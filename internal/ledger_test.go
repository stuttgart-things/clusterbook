package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

// ── Issue #199: in-process serialization ─────────────────────────────────────

// poolYAML returns one network with n free addresses, digits 1..n.
func poolYAML(network string, n int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s:\n", network)
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "  \"%d\":\n    status: \"\"\n    cluster: \"\"\n", i)
	}
	return b.String()
}

// reserveConcurrently fires n reserve requests at once and returns the IP each
// one was given ("" when the request did not succeed).
func reserveConcurrently(t *testing.T, n int, body func(i int) string, loadFrom, dir, name string, ddwrt *DDWRTClient) []string {
	t.Helper()
	ips := make([]string, n)
	codes := make([]int, n)

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			req := httptest.NewRequest("POST", "/api/v1/networks/10.50.1/reserve", strings.NewReader(body(i)))
			req.SetPathValue("key", "10.50.1")
			w := httptest.NewRecorder()
			handleAPIReserve(w, req, loadFrom, dir, name, nil, ddwrt)

			codes[i] = w.Code
			var resp struct {
				IP string `json:"ip"`
			}
			if w.Code == http.StatusOK && json.NewDecoder(w.Body).Decode(&resp) == nil {
				ips[i] = resp.IP
			}
		}(i)
	}
	close(start)
	wg.Wait()

	for i, code := range codes {
		if code != http.StatusOK {
			t.Errorf("request %d: code %d", i, code)
		}
	}
	return ips
}

// Without a lock, overlapping reserves each loaded the same snapshot, handed out
// the same free address, and the later save erased the earlier assignment.
func TestConcurrentReserve_NoAddressGivenTwice(t *testing.T) {
	const n = 25
	dir, name := setupTestConfig(t, poolYAML("10.50.1", n))

	ips := reserveConcurrently(t, n, func(i int) string {
		return fmt.Sprintf(`{"cluster":"c%d"}`, i)
	}, "disk", dir, name, nil)

	seen := map[string]int{}
	for i, ip := range ips {
		if prev, dup := seen[ip]; dup && ip != "" {
			t.Errorf("%s was given to request %d and request %d", ip, prev, i)
		}
		seen[ip] = i
	}

	ipList, err := LoadProfile("disk", dir, name)
	if err != nil {
		t.Fatal(err)
	}
	clusters := map[string]bool{}
	for digit, info := range ipList["10.50.1"] {
		if isFree(info) {
			t.Errorf("10.50.1.%s is free in the ledger although %d reserves succeeded", digit, n)
			continue
		}
		clusters[info.Cluster] = true
	}
	if len(clusters) != n {
		t.Errorf("ledger holds %d distinct clusters, want %d (lost updates)", len(clusters), n)
	}
}

// DD-WRT records are a get → merge → set on nvram. Concurrent reserves with DNS
// dropped each other's entries unless the write lock also spans the DNS calls.
func TestConcurrentReserve_KeepsEveryDNSRecord(t *testing.T) {
	const n = 8
	dir, name := setupTestConfig(t, poolYAML("10.50.1", n))

	srv, err := NewFakeDDWRTServer("root", "testpass")
	if err != nil {
		t.Fatalf("fake ssh server: %v", err)
	}
	defer srv.Close()
	ddwrt := NewDDWRTClient("true", srv.Addr, "root", "testpass", "sthings.lab")

	reserveConcurrently(t, n, func(i int) string {
		return fmt.Sprintf(`{"cluster":"dns%d","create_dns":true}`, i)
	}, "disk", dir, name, ddwrt)

	opts := srv.NvramGet("dnsmasq_options")
	for i := 0; i < n; i++ {
		if fqdn := fmt.Sprintf("/dns%d.sthings.lab/", i); !strings.Contains(opts, fqdn) {
			t.Errorf("record for dns%d missing from nvram: %q", i, opts)
		}
	}
}

// blockingExecutor parks the first SSH command until released, so a test can
// observe what other writers do while DNS work is in flight.
type blockingExecutor struct {
	*fakeExecutor
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (b *blockingExecutor) Run(cmd string) (string, error) {
	b.once.Do(func() {
		close(b.entered)
		<-b.release
	})
	return b.fakeExecutor.Run(cmd)
}

func TestReclaimOnce_HoldsLedgerAcrossDNS(t *testing.T) {
	dir, name := setupTestConfig(t, reclaimConfigYAML)

	exec := &blockingExecutor{fakeExecutor: newFakeExecutor(), entered: make(chan struct{}), release: make(chan struct{})}
	ddwrt := newDDWRTClientWithExecutor("sthings.lab", exec)

	reclaimDone := make(chan error, 1)
	go func() {
		_, err := reclaimOnce("disk", dir, name, time.Unix(1_700_000_000, 0), nil, ddwrt)
		reclaimDone <- err
	}()
	<-exec.entered // saved, now removing the DNS record

	reserveDone := make(chan int, 1)
	go func() {
		req := httptest.NewRequest("POST", "/", strings.NewReader(`{"cluster":"late"}`))
		req.SetPathValue("key", "10.31.103")
		w := httptest.NewRecorder()
		handleAPIReserve(w, req, "disk", dir, name, nil, nil)
		reserveDone <- w.Code
	}()

	select {
	case code := <-reserveDone:
		t.Fatalf("reserve finished (code %d) while the reclaimer was still inside its ledger write", code)
	case <-time.After(150 * time.Millisecond):
	}

	close(exec.release)
	if err := <-reclaimDone; err != nil {
		t.Fatalf("reclaimOnce: %v", err)
	}
	if code := <-reserveDone; code != http.StatusOK {
		t.Fatalf("reserve after the reclaimer: code %d", code)
	}

	ipList, err := LoadProfile("disk", dir, name)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, info := range ipList["10.31.103"] {
		found = found || info.Cluster == "late"
	}
	if !found {
		t.Errorf("the reservation made after the reclaimer is missing from the ledger: %+v", ipList)
	}
}

func TestLedgerWrite_Misuse(t *testing.T) {
	dir, name := setupTestConfig(t, testConfigYAML)

	tx := BeginLedgerWrite("disk", dir, name)
	if err := tx.Save(map[string]IPs{}); err == nil {
		t.Error("Save without Load must fail")
	}
	tx.End()
	tx.End() // harmless

	tx = BeginLedgerWrite("disk", dir, name) // would deadlock if End had not released
	if _, err := tx.Load(); err != nil {
		t.Fatal(err)
	}
	tx.End()
	if err := tx.Save(map[string]IPs{}); err == nil {
		t.Error("Save after End must fail")
	}
}

// ── Issue #199: cross-writer conflicts in cr mode ────────────────────────────

// fakeAPIServer answers get/create/update for the NetworkConfig the way the
// apiserver does, including resourceVersion checks — the client-go fake tracker
// does not enforce those.
type fakeAPIServer struct {
	mu      sync.Mutex
	obj     *unstructured.Unstructured
	rv      int
	updates int

	// writeBeforeNextUpdate simulates another writer landing between the Get
	// and the Update of a save.
	writeBeforeNextUpdate bool
}

func installFakeAPIServer(t *testing.T) *fakeAPIServer {
	t.Helper()
	api := &fakeAPIServer{}
	gvr := groupVersion.WithResource(resource)
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "NetworkConfigList"})
	client.PrependReactor("*", resource, api.react)

	prev := newDynamicClient
	newDynamicClient = func() (dynamic.Interface, error) { return client, nil }
	t.Cleanup(func() { newDynamicClient = prev })
	return api
}

func (a *fakeAPIServer) react(action k8stesting.Action) (bool, runtime.Object, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	gr := groupVersion.WithResource(resource).GroupResource()

	switch action.GetVerb() {
	case "get":
		name := action.(k8stesting.GetAction).GetName()
		if a.obj == nil {
			return true, nil, apierrors.NewNotFound(gr, name)
		}
		return true, a.obj.DeepCopy(), nil

	case "create":
		obj := action.(k8stesting.CreateAction).GetObject().(*unstructured.Unstructured).DeepCopy()
		if a.obj != nil {
			return true, nil, apierrors.NewAlreadyExists(gr, obj.GetName())
		}
		a.store(obj)
		return true, obj.DeepCopy(), nil

	case "update":
		obj := action.(k8stesting.UpdateAction).GetObject().(*unstructured.Unstructured).DeepCopy()
		if a.obj == nil {
			return true, nil, apierrors.NewNotFound(gr, obj.GetName())
		}
		if a.writeBeforeNextUpdate {
			a.writeBeforeNextUpdate = false
			a.store(a.obj.DeepCopy())
		}
		if obj.GetResourceVersion() != a.obj.GetResourceVersion() {
			return true, nil, apierrors.NewConflict(gr, obj.GetName(), errors.New("the object has been modified"))
		}
		a.store(obj)
		a.updates++
		return true, obj.DeepCopy(), nil
	}
	return false, nil, nil
}

func (a *fakeAPIServer) store(obj *unstructured.Unstructured) {
	a.rv++
	obj.SetResourceVersion(strconv.Itoa(a.rv))
	a.obj = obj
}

// externalEdit is another replica, or kubectl edit, writing the CR.
func (a *fakeAPIServer) externalEdit() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.store(a.obj.DeepCopy())
}

const (
	crNamespace = "clusterbook"
	crName      = "networks"
)

func seedCR(t *testing.T, ipList map[string]IPs) {
	t.Helper()
	tx := BeginLedgerWrite("cr", crNamespace, crName)
	defer tx.End()
	if _, err := tx.Load(); err != nil {
		t.Fatalf("seed load: %v", err)
	}
	if err := tx.Save(ipList); err != nil {
		t.Fatalf("seed save: %v", err)
	}
}

func crFixture() map[string]IPs {
	return map[string]IPs{"10.31.103": {"6": {}, "7": {Status: "ASSIGNED", Cluster: "keep"}}}
}

func TestLedgerWrite_CR_SavesAtLoadedVersion(t *testing.T) {
	api := installFakeAPIServer(t)
	seedCR(t, crFixture())

	tx := BeginLedgerWrite("cr", crNamespace, crName)
	ipList, err := tx.Load()
	if err != nil {
		t.Fatal(err)
	}
	ipList["10.31.103"]["6"] = IPInfo{Status: "ASSIGNED", Cluster: "probe"}
	if err := tx.Save(ipList); err != nil {
		t.Fatalf("Save: %v", err)
	}
	tx.End()

	if api.updates != 1 {
		t.Errorf("updates = %d, want 1", api.updates)
	}
	got, err := LoadProfile("cr", crNamespace, crName)
	if err != nil {
		t.Fatal(err)
	}
	if got["10.31.103"]["6"].Cluster != "probe" {
		t.Errorf("saved config = %+v", got)
	}
}

func TestLedgerWrite_CR_Conflicts(t *testing.T) {
	tests := []struct {
		name  string
		seed  bool
		after func(api *fakeAPIServer) // runs between Load and Save
	}{
		{"edited since load", true, func(api *fakeAPIServer) { api.externalEdit() }},
		{"written between get and update", true, func(api *fakeAPIServer) { api.writeBeforeNextUpdate = true }},
		{"deleted since load", true, func(api *fakeAPIServer) { api.obj = nil }},
		{"created since load", false, func(api *fakeAPIServer) {
			obj := &unstructured.Unstructured{}
			obj.SetAPIVersion(groupVersion.String())
			obj.SetKind("NetworkConfig")
			obj.SetName(crName)
			obj.SetNamespace(crNamespace)
			api.store(obj)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := installFakeAPIServer(t)
			if tt.seed {
				seedCR(t, crFixture())
			}

			tx := BeginLedgerWrite("cr", crNamespace, crName)
			defer tx.End()
			ipList, err := tx.Load()
			if err != nil {
				t.Fatal(err)
			}
			if ipList["10.31.103"] == nil {
				ipList["10.31.103"] = IPs{}
			}
			ipList["10.31.103"]["6"] = IPInfo{Status: "ASSIGNED", Cluster: "stale-writer"}

			tt.after(api)
			updatesBefore := api.updates

			if err := tx.Save(ipList); !errors.Is(err, ErrLedgerConflict) {
				t.Fatalf("Save err = %v, want ErrLedgerConflict", err)
			}
			if api.updates != updatesBefore {
				t.Error("the stale write reached the API server")
			}
		})
	}
}

// The HTTP layer turns a conflict into 409, and does not create DNS for it.
func TestHandleAPIReserve_CRConflictAnswers409(t *testing.T) {
	api := installFakeAPIServer(t)
	seedCR(t, crFixture())
	api.writeBeforeNextUpdate = true

	exec := newFakeExecutor()
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"cluster":"probe","create_dns":true}`))
	req.SetPathValue("key", "10.31.103")
	w := httptest.NewRecorder()
	handleAPIReserve(w, req, "cr", crNamespace, crName, nil, newDDWRTClientWithExecutor("sthings.lab", exec))

	if w.Code != http.StatusConflict {
		t.Fatalf("code = %d, want 409; body %s", w.Code, w.Body.String())
	}
	if len(exec.calls) != 0 {
		t.Errorf("DNS was touched for a refused write: %v", exec.calls)
	}
}
