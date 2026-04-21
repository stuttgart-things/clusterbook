package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupTestConfig(t *testing.T, yaml string) (configDir, configName string) {
	t.Helper()
	dir := t.TempDir()
	name := "config.yaml"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	return dir, name
}

const testConfigYAML = `
10.31.103:
  "5":
    status: "ASSIGNED:DNS"
    cluster: mycluster
  "6":
    status: "ASSIGNED"
    cluster: mycluster
  "7":
    status: ""
    cluster: ""
10.31.104:
  "10":
    status: "PENDING"
    cluster: othercluster
  "11":
    status: ""
    cluster: ""
`

func TestHandleAPIClusters(t *testing.T) {
	dir, name := setupTestConfig(t, testConfigYAML)

	req := httptest.NewRequest("GET", "/api/v1/clusters", nil)
	w := httptest.NewRecorder()

	handleAPIClusters(w, req, "disk", dir, name)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []struct {
		Cluster string `json:"cluster"`
		IPCount int    `json:"ip_count"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	clusters := map[string]int{}
	for _, c := range result {
		clusters[c.Cluster] = c.IPCount
	}

	if clusters["mycluster"] != 2 {
		t.Errorf("expected mycluster to have 2 IPs, got %d", clusters["mycluster"])
	}
	if clusters["othercluster"] != 1 {
		t.Errorf("expected othercluster to have 1 IP, got %d", clusters["othercluster"])
	}
}

func TestHandleAPIClusterInfo(t *testing.T) {
	dir, name := setupTestConfig(t, testConfigYAML)

	t.Run("existing cluster", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/clusters/mycluster", nil)
		req.SetPathValue("name", "mycluster")
		w := httptest.NewRecorder()

		handleAPIClusterInfo(w, req, "disk", dir, name, nil, nil)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}

		var result struct {
			Cluster string `json:"cluster"`
			IPs     []struct {
				Network string `json:"network"`
				IP      string `json:"ip"`
				Status  string `json:"status"`
			} `json:"ips"`
		}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode error: %v", err)
		}

		if result.Cluster != "mycluster" {
			t.Errorf("expected cluster mycluster, got %s", result.Cluster)
		}
		if len(result.IPs) != 2 {
			t.Errorf("expected 2 IPs, got %d", len(result.IPs))
		}
	})

	t.Run("nonexistent cluster returns 404", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/clusters/doesnotexist", nil)
		req.SetPathValue("name", "doesnotexist")
		w := httptest.NewRecorder()

		handleAPIClusterInfo(w, req, "disk", dir, name, nil, nil)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})

	t.Run("includes fqdn and zone with PDNS provider", func(t *testing.T) {
		pdns := &PDNSClient{Zone: "sthings-vsphere.labul.sva.de."}
		req := httptest.NewRequest("GET", "/api/v1/clusters/mycluster", nil)
		req.SetPathValue("name", "mycluster")
		w := httptest.NewRecorder()

		handleAPIClusterInfo(w, req, "disk", dir, name, pdns, nil)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}

		var result struct {
			Cluster string `json:"cluster"`
			FQDN    string `json:"fqdn"`
			Zone    string `json:"zone"`
		}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode error: %v", err)
		}

		expectedFQDN := "*.mycluster.sthings-vsphere.labul.sva.de"
		if result.FQDN != expectedFQDN {
			t.Errorf("expected fqdn %q, got %q", expectedFQDN, result.FQDN)
		}
		if result.Zone != "sthings-vsphere.labul.sva.de" {
			t.Errorf("expected zone sthings-vsphere.labul.sva.de, got %q", result.Zone)
		}
	})

	t.Run("no fqdn without DNS status", func(t *testing.T) {
		pdns := &PDNSClient{Zone: "sthings.lab."}
		req := httptest.NewRequest("GET", "/api/v1/clusters/othercluster", nil)
		req.SetPathValue("name", "othercluster")
		w := httptest.NewRecorder()

		handleAPIClusterInfo(w, req, "disk", dir, name, pdns, nil)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode error: %v", err)
		}

		if _, ok := result["fqdn"]; ok {
			t.Errorf("expected no fqdn field for cluster without DNS, got %v", result["fqdn"])
		}
	})
}

func TestHandleAPINetworkIPsWithFQDN(t *testing.T) {
	dir, name := setupTestConfig(t, testConfigYAML)
	pdns := &PDNSClient{Zone: "sthings-vsphere.labul.sva.de."}

	req := httptest.NewRequest("GET", "/api/v1/networks/10.31.103/ips", nil)
	req.SetPathValue("key", "10.31.103")
	w := httptest.NewRecorder()

	handleAPINetworkIPs(w, req, "disk", dir, name, pdns, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var entries []IPEntry
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	fqdnByDigit := map[string]string{}
	for _, e := range entries {
		fqdnByDigit[e.Digit] = e.FQDN
	}

	// digit "5" has ASSIGNED:DNS → should have FQDN
	expected := "*.mycluster.sthings-vsphere.labul.sva.de"
	if fqdnByDigit["5"] != expected {
		t.Errorf("digit 5: expected fqdn %q, got %q", expected, fqdnByDigit["5"])
	}

	// digit "6" has ASSIGNED (no DNS) → no FQDN
	if fqdnByDigit["6"] != "" {
		t.Errorf("digit 6: expected empty fqdn, got %q", fqdnByDigit["6"])
	}

	// digit "7" is unassigned → no FQDN
	if fqdnByDigit["7"] != "" {
		t.Errorf("digit 7: expected empty fqdn, got %q", fqdnByDigit["7"])
	}
}

func TestHandleAPIZone(t *testing.T) {
	t.Run("with PDNS provider", func(t *testing.T) {
		pdns := &PDNSClient{Zone: "sthings-vsphere.labul.sva.de."}
		req := httptest.NewRequest("GET", "/api/v1/zone", nil)
		w := httptest.NewRecorder()

		handleAPIZone(w, req, pdns, nil)

		var result map[string]struct {
			Enabled bool   `json:"enabled"`
			Zone    string `json:"zone"`
		}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode error: %v", err)
		}

		if !result["pdns"].Enabled {
			t.Error("expected pdns enabled")
		}
		if result["pdns"].Zone != "sthings-vsphere.labul.sva.de" {
			t.Errorf("expected zone sthings-vsphere.labul.sva.de, got %q", result["pdns"].Zone)
		}
		if result["ddwrt"].Enabled {
			t.Error("expected ddwrt disabled")
		}
	})

	t.Run("no providers", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/zone", nil)
		w := httptest.NewRecorder()

		handleAPIZone(w, req, nil, nil)

		var result map[string]struct {
			Enabled bool   `json:"enabled"`
			Zone    string `json:"zone"`
		}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("decode error: %v", err)
		}

		if result["pdns"].Enabled || result["ddwrt"].Enabled {
			t.Error("expected both providers disabled")
		}
	})
}

func TestHandleAPIAssignWithLease(t *testing.T) {
	dir, name := setupTestConfig(t, testConfigYAML)

	body := `{"ip":"10.31.103.7","cluster":"newcluster","lease_duration_seconds":3600}`
	req := httptest.NewRequest("POST", "/api/v1/networks/10.31.103/assign", strings.NewReader(body))
	req.SetPathValue("key", "10.31.103")
	w := httptest.NewRecorder()

	before := time.Now().Unix()
	handleAPIAssign(w, req, "disk", dir, name, nil, nil)
	after := time.Now().Unix()

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	ipList := LoadProfile("disk", dir, name)
	entry := ipList["10.31.103"]["7"]
	if entry.Status != "ASSIGNED" || entry.Cluster != "newcluster" {
		t.Errorf("entry not assigned: %+v", entry)
	}
	if entry.LeaseExpiresAt < before+3600 || entry.LeaseExpiresAt > after+3600 {
		t.Errorf("unexpected LeaseExpiresAt: got %d, want within [%d, %d]",
			entry.LeaseExpiresAt, before+3600, after+3600)
	}
}

// Regression tests for #150 — reserve with createDNS=true:
// - camelCase createDNS is accepted (operator dialect, not just create_dns)
// - response contains an "ips" array so clients expecting that shape can parse it
// - follow-up edit doesn't double-append ":DNS" to the status
// - list-ips carries an fqdn field on the new entry end-to-end
func TestHandleAPIReserveAcceptsCamelCaseCreateDNS(t *testing.T) {
	dir, name := setupTestConfig(t, testConfigYAML)

	body := `{"cluster":"smoke-alloc","count":1,"createDNS":true}`
	req := httptest.NewRequest("POST", "/api/v1/networks/10.31.103/reserve", strings.NewReader(body))
	req.SetPathValue("key", "10.31.103")
	w := httptest.NewRecorder()

	handleAPIReserve(w, req, "disk", dir, name, nil, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		IP      string   `json:"ip"`
		IPs     []string `json:"ips"`
		Status  string   `json:"status"`
		Cluster string   `json:"cluster"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if resp.Cluster != "smoke-alloc" {
		t.Errorf("cluster in response: got %q, want %q", resp.Cluster, "smoke-alloc")
	}
	if resp.Status != "ASSIGNED:DNS" {
		t.Errorf("status in response: got %q, want %q", resp.Status, "ASSIGNED:DNS")
	}
	if resp.IP == "" {
		t.Errorf("ip field missing")
	}
	if len(resp.IPs) != 1 || resp.IPs[0] != resp.IP {
		t.Errorf("ips field: got %v, want [%q]", resp.IPs, resp.IP)
	}

	// Persisted entry should match — cluster kept intact, :DNS suffix set.
	ipList := LoadProfile("disk", dir, name)
	digit := strings.TrimPrefix(resp.IP, "10.31.103.")
	entry := ipList["10.31.103"][digit]
	if entry.Cluster != "smoke-alloc" {
		t.Errorf("persisted cluster: got %q, want %q", entry.Cluster, "smoke-alloc")
	}
	if entry.Status != "ASSIGNED:DNS" {
		t.Errorf("persisted status: got %q, want %q", entry.Status, "ASSIGNED:DNS")
	}
}

func TestHandleAPIAssignAcceptsCamelCaseCreateDNS(t *testing.T) {
	dir, name := setupTestConfig(t, testConfigYAML)

	body := `{"ip":"10.31.103.7","cluster":"smoke-assign","status":"ASSIGNED","createDNS":true}`
	req := httptest.NewRequest("POST", "/api/v1/networks/10.31.103/assign", strings.NewReader(body))
	req.SetPathValue("key", "10.31.103")
	w := httptest.NewRecorder()

	handleAPIAssign(w, req, "disk", dir, name, nil, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	entry := LoadProfile("disk", dir, name)["10.31.103"]["7"]
	if entry.Cluster != "smoke-assign" {
		t.Errorf("cluster: got %q, want %q", entry.Cluster, "smoke-assign")
	}
	if entry.Status != "ASSIGNED:DNS" {
		t.Errorf("status: got %q, want %q", entry.Status, "ASSIGNED:DNS")
	}
}

func TestHandleAPIEditIPDoesNotDoubleSuffixDNS(t *testing.T) {
	dir, name := setupTestConfig(t, testConfigYAML)

	// Mirrors the operator's drift-reconcile body: pre-suffixed status + createDNS=true.
	body := `{"cluster":"mycluster","status":"ASSIGNED:DNS","createDNS":true}`
	req := httptest.NewRequest("PUT", "/api/v1/networks/10.31.103/ips/6", strings.NewReader(body))
	req.SetPathValue("key", "10.31.103")
	req.SetPathValue("ip", "6")
	w := httptest.NewRecorder()

	handleAPIEditIP(w, req, "disk", dir, name, nil, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	entry := LoadProfile("disk", dir, name)["10.31.103"]["6"]
	if entry.Status != "ASSIGNED:DNS" {
		t.Errorf("status: got %q, want %q (no double :DNS)", entry.Status, "ASSIGNED:DNS")
	}
}

func TestHandleAPIReserveThenListIPsIncludesFQDN(t *testing.T) {
	dir, name := setupTestConfig(t, testConfigYAML)
	// DDWRT with a fake executor gives us a zone without touching SSH or HTTP.
	ddwrt := newDDWRTClientWithExecutor("sthings.lab", newFakeExecutor())

	reserveBody := `{"cluster":"e2e-alloc","createDNS":true}`
	reserveReq := httptest.NewRequest("POST", "/api/v1/networks/10.31.104/reserve", strings.NewReader(reserveBody))
	reserveReq.SetPathValue("key", "10.31.104")
	reserveW := httptest.NewRecorder()
	handleAPIReserve(reserveW, reserveReq, "disk", dir, name, nil, ddwrt)
	if reserveW.Code != http.StatusOK {
		t.Fatalf("reserve: expected 200, got %d: %s", reserveW.Code, reserveW.Body.String())
	}

	listReq := httptest.NewRequest("GET", "/api/v1/networks/10.31.104/ips", nil)
	listReq.SetPathValue("key", "10.31.104")
	listW := httptest.NewRecorder()
	handleAPINetworkIPs(listW, listReq, "disk", dir, name, nil, ddwrt)
	if listW.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", listW.Code, listW.Body.String())
	}

	var entries []IPEntry
	if err := json.NewDecoder(listW.Body).Decode(&entries); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	var found *IPEntry
	for i, e := range entries {
		if e.Cluster == "e2e-alloc" {
			found = &entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("reserved entry not in listing; entries=%+v", entries)
	}
	if found.Status != "ASSIGNED:DNS" {
		t.Errorf("listing status: got %q, want ASSIGNED:DNS", found.Status)
	}
	if found.FQDN != "*.e2e-alloc.sthings.lab" {
		t.Errorf("listing fqdn: got %q, want *.e2e-alloc.sthings.lab", found.FQDN)
	}
}

func TestHandleAPIRenewLease(t *testing.T) {
	dir, name := setupTestConfig(t, testConfigYAML)

	body := `{"lease_duration_seconds":7200}`
	req := httptest.NewRequest("POST", "/api/v1/networks/10.31.103/ips/10.31.103.6/renew", strings.NewReader(body))
	req.SetPathValue("key", "10.31.103")
	req.SetPathValue("ip", "10.31.103.6")
	w := httptest.NewRecorder()

	before := time.Now().Unix()
	handleAPIRenewLease(w, req, "disk", dir, name)
	after := time.Now().Unix()

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	ipList := LoadProfile("disk", dir, name)
	entry := ipList["10.31.103"]["6"]
	if entry.LeaseExpiresAt < before+7200 || entry.LeaseExpiresAt > after+7200 {
		t.Errorf("unexpected LeaseExpiresAt after renew: %d", entry.LeaseExpiresAt)
	}
}

func TestHandleAPIRenewLeaseRejectsUnassigned(t *testing.T) {
	dir, name := setupTestConfig(t, testConfigYAML)

	body := `{"lease_duration_seconds":3600}`
	req := httptest.NewRequest("POST", "/api/v1/networks/10.31.103/ips/10.31.103.7/renew", strings.NewReader(body))
	req.SetPathValue("key", "10.31.103")
	req.SetPathValue("ip", "10.31.103.7")
	w := httptest.NewRecorder()

	handleAPIRenewLease(w, req, "disk", dir, name)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 for unassigned IP, got %d", w.Code)
	}
}

func TestHandleAPIReleaseClearsLease(t *testing.T) {
	dir, name := setupTestConfig(t, testConfigYAML)

	// First assign with lease
	assignBody := `{"ip":"10.31.103.7","cluster":"temp","lease_duration_seconds":3600}`
	assignReq := httptest.NewRequest("POST", "/api/v1/networks/10.31.103/assign", strings.NewReader(assignBody))
	assignReq.SetPathValue("key", "10.31.103")
	handleAPIAssign(httptest.NewRecorder(), assignReq, "disk", dir, name, nil, nil)

	// Then release
	releaseBody := `{"ip":"10.31.103.7"}`
	releaseReq := httptest.NewRequest("POST", "/api/v1/networks/10.31.103/release", strings.NewReader(releaseBody))
	releaseReq.SetPathValue("key", "10.31.103")
	w := httptest.NewRecorder()
	handleAPIRelease(w, releaseReq, "disk", dir, name, nil, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	ipList := LoadProfile("disk", dir, name)
	entry := ipList["10.31.103"]["7"]
	if entry.LeaseExpiresAt != 0 {
		t.Errorf("expected lease cleared after release, got %d", entry.LeaseExpiresAt)
	}
}

func TestBuildFQDN(t *testing.T) {
	tests := []struct {
		cluster, zone, want string
	}{
		{"myapp", "sthings.lab", "*.myapp.sthings.lab"},
		{"", "sthings.lab", ""},
		{"myapp", "", ""},
	}
	for _, tt := range tests {
		got := buildFQDN(tt.cluster, tt.zone)
		if got != tt.want {
			t.Errorf("buildFQDN(%q, %q) = %q, want %q", tt.cluster, tt.zone, got, tt.want)
		}
	}
}

func TestDnsZone(t *testing.T) {
	t.Run("prefers PDNS zone", func(t *testing.T) {
		pdns := &PDNSClient{Zone: "pdns.zone."}
		ddwrt := &DDWRTClient{Zone: "ddwrt.zone"}
		if got := dnsZone(pdns, ddwrt); got != "pdns.zone" {
			t.Errorf("expected pdns.zone, got %q", got)
		}
	})

	t.Run("falls back to DDWRT", func(t *testing.T) {
		ddwrt := &DDWRTClient{Zone: "ddwrt.zone"}
		if got := dnsZone(nil, ddwrt); got != "ddwrt.zone" {
			t.Errorf("expected ddwrt.zone, got %q", got)
		}
	})

	t.Run("returns empty when both nil", func(t *testing.T) {
		if got := dnsZone(nil, nil); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})
}
