package ipassignment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stuttgart-things/clusterbook/provider/apis/v1alpha1"
	"github.com/stuttgart-things/clusterbook/provider/internal/client"
)

const network = "10.31.103"

// fakeClusterbook serves the endpoints the IPAssignment controller calls, with
// clusterbook's semantics: reserve picks and records a free address in one
// request, assign overwrites whoever holds the address.
type fakeClusterbook struct {
	t *testing.T

	mu      sync.Mutex
	entries map[string]client.IPEntry // digit → entry

	reserves, assigns, edits int

	// afterList runs once, right after a GET /ips was answered: another writer
	// acting between the controller's read and its write.
	afterList func(f *fakeClusterbook)
	// failReserve maps the n-th reserve call (1-based) to an error status.
	failReserve map[int]int
}

func newFakeClusterbook(t *testing.T, digits ...int) (*fakeClusterbook, *external) {
	t.Helper()
	f := &fakeClusterbook{t: t, entries: map[string]client.IPEntry{}, failReserve: map[int]int{}}
	for _, d := range digits {
		digit := strconv.Itoa(d)
		f.entries[digit] = client.IPEntry{IP: network + "." + digit, Digit: digit}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/networks/{key}/ips", f.list)
	mux.HandleFunc("POST /api/v1/networks/{key}/reserve", f.reserve)
	mux.HandleFunc("POST /api/v1/networks/{key}/assign", f.assign)
	mux.HandleFunc("PUT /api/v1/networks/{key}/ips/{ip}", f.edit)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return f, &external{client: client.NewClient(srv.URL)}
}

func (f *fakeClusterbook) set(digit, cluster, status string) {
	f.entries[digit] = client.IPEntry{IP: network + "." + digit, Digit: digit, Cluster: cluster, Status: status}
}

func (f *fakeClusterbook) sorted() []client.IPEntry {
	out := make([]client.IPEntry, 0, len(f.entries))
	for _, e := range f.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := strconv.Atoi(out[i].Digit)
		b, _ := strconv.Atoi(out[j].Digit)
		return a < b
	})
	return out
}

func (f *fakeClusterbook) heldBy(cluster string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var ips []string
	for _, e := range f.sorted() {
		if e.Cluster == cluster && e.Status != "" {
			ips = append(ips, e.IP)
		}
	}
	return ips
}

func (f *fakeClusterbook) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		f.t.Errorf("encode: %v", err)
	}
}

func (f *fakeClusterbook) list(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writeJSON(w, f.sorted())
	if f.afterList != nil {
		hook := f.afterList
		f.afterList = nil
		hook(f)
	}
}

type writeRequest struct {
	IP        string `json:"ip"`
	Cluster   string `json:"cluster"`
	Status    string `json:"status"`
	CreateDNS bool   `json:"create_dns"`
}

func (f *fakeClusterbook) decode(r *http.Request) writeRequest {
	var req writeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		f.t.Errorf("decode request: %v", err)
	}
	if req.CreateDNS {
		req.Status = strings.TrimSuffix(req.Status, ":DNS") + ":DNS"
	}
	return req
}

func (f *fakeClusterbook) reserve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reserves++
	req := f.decode(r)

	if code, fail := f.failReserve[f.reserves]; fail {
		http.Error(w, "injected failure", code)
		return
	}

	for _, e := range f.sorted() {
		if e.Status == "" {
			f.set(e.Digit, req.Cluster, req.Status)
			f.writeJSON(w, map[string]any{"ip": e.IP, "digit": e.Digit, "status": req.Status, "cluster": req.Cluster, "dns": "ok"})
			return
		}
	}
	http.Error(w, `{"error":"no available IPs in network"}`, http.StatusConflict)
}

func (f *fakeClusterbook) assign(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.assigns++
	req := f.decode(r)
	digit := req.IP[strings.LastIndex(req.IP, ".")+1:]
	f.set(digit, req.Cluster, req.Status)
	f.writeJSON(w, map[string]any{"status": "ok", "dns": "skipped"})
}

func (f *fakeClusterbook) edit(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.edits++
	f.writeJSON(w, map[string]any{"status": "ok", "dns": "ok"})
}

func newIPAssignment(cluster string, count int, createDNS bool) *v1alpha1.IPAssignment {
	cr := &v1alpha1.IPAssignment{}
	cr.Spec.ForProvider = v1alpha1.IPAssignmentParameters{
		NetworkKey: network,
		Cluster:    cluster,
		CountIPs:   count,
		Status:     "ASSIGNED",
		CreateDNS:  createDNS,
	}
	return cr
}

// Issue #205: the controller listed free addresses, then assigned the lowest one.
// A writer that took that address in between lost it — and, since assign
// withdraws the previous holder's DNS record, its record too.
func TestCreate_DoesNotTakeAnAddressAnotherWriterGrabbed(t *testing.T) {
	f, ext := newFakeClusterbook(t, 1, 2)
	f.afterList = func(f *fakeClusterbook) { f.set("1", "other", "ASSIGNED:DNS") }

	cr := newIPAssignment("mine", 1, false)
	if _, err := ext.Create(context.Background(), cr); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if got := f.heldBy("other"); len(got) != 1 || got[0] != network+".1" {
		t.Errorf("the other writer's address was taken: other now holds %v", got)
	}
	if got := cr.Status.AtProvider.IPAddresses; len(got) != 1 || got[0] != network+".2" {
		t.Errorf("status IPAddresses = %v, want [%s.2]", got, network)
	}
	if f.assigns != 0 {
		t.Errorf("Create used assign %d time(s); it must only reserve", f.assigns)
	}
}

// A Create that fails after reserving part of countIPs must not, on retry, take a
// full new set and strand what it already reserved.
func TestCreate_RetryAfterPartialFailureDoesNotLeak(t *testing.T) {
	f, ext := newFakeClusterbook(t, 1, 2, 3, 4)
	f.failReserve[2] = http.StatusInternalServerError

	cr := newIPAssignment("mine", 2, false)
	if _, err := ext.Create(context.Background(), cr); err == nil {
		t.Fatal("first Create: expected the injected failure")
	}
	if len(cr.Status.AtProvider.IPAddresses) != 0 {
		t.Errorf("a failed Create recorded a partial status: %v", cr.Status.AtProvider.IPAddresses)
	}

	if _, err := ext.Create(context.Background(), cr); err != nil {
		t.Fatalf("second Create: %v", err)
	}

	held := f.heldBy("mine")
	if len(held) != 2 {
		t.Fatalf("cluster holds %v, want exactly 2 addresses", held)
	}
	got := append([]string(nil), cr.Status.AtProvider.IPAddresses...)
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(held, ",") {
		t.Errorf("status IPAddresses %v do not match what the cluster holds %v", got, held)
	}
}

func TestCreate_NotEnoughFreeReservesNothing(t *testing.T) {
	f, ext := newFakeClusterbook(t, 1)

	if _, err := ext.Create(context.Background(), newIPAssignment("mine", 2, false)); err == nil {
		t.Fatal("expected an error for a pool with 1 free address and countIPs 2")
	}
	if f.reserves != 0 {
		t.Errorf("reserved %d address(es) for a request that could not be met", f.reserves)
	}
}

func TestCreate_CountsAddressesAlreadyHeld(t *testing.T) {
	f, ext := newFakeClusterbook(t, 1, 2, 3)
	f.set("3", "mine", "ASSIGNED")

	cr := newIPAssignment("mine", 2, false)
	if _, err := ext.Create(context.Background(), cr); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if f.reserves != 1 {
		t.Errorf("reserves = %d, want 1 (one address was already held)", f.reserves)
	}
	if got := f.heldBy("mine"); len(got) != 2 {
		t.Errorf("cluster holds %v, want 2", got)
	}
}

// Adoption after a Create that reserved but failed on DNS must not settle as up
// to date, or the record is never retried.
func TestObserve_AdoptionReDrivesDNS(t *testing.T) {
	tests := []struct {
		createDNS    bool
		wantUpToDate bool
	}{
		{createDNS: true, wantUpToDate: false},
		{createDNS: false, wantUpToDate: true},
	}

	for _, tt := range tests {
		f, ext := newFakeClusterbook(t, 1, 2)
		status := "ASSIGNED"
		if tt.createDNS {
			status = "ASSIGNED:DNS"
		}
		f.set("1", "mine", status)

		cr := newIPAssignment("mine", 1, tt.createDNS)
		obs, err := ext.Observe(context.Background(), cr)
		if err != nil {
			t.Fatalf("Observe: %v", err)
		}
		if !obs.ResourceExists {
			t.Fatalf("createDNS=%v: held address not adopted", tt.createDNS)
		}
		if obs.ResourceUpToDate != tt.wantUpToDate {
			t.Errorf("createDNS=%v: ResourceUpToDate = %v, want %v", tt.createDNS, obs.ResourceUpToDate, tt.wantUpToDate)
		}
	}
}
