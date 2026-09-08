package client

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// encodeJSON writes a JSON body from inside a test server handler. Handlers run
// on their own goroutine, so failures are reported with Errorf, not Fatalf.
func encodeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

// decodeBody reads a JSON request body from inside a test server handler.
func decodeBody(t *testing.T, r *http.Request, v any) {
	t.Helper()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		t.Errorf("decode request body: %v", err)
	}
}

func TestListNetworks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/networks" || r.Method != http.MethodGet {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		encodeJSON(t, w, []NetworkPool{
			{NetworkKey: "10.31.103", Total: 3, Assigned: 1, Available: 2},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	pools, err := c.ListNetworks()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pools) != 1 {
		t.Fatalf("expected 1 pool, got %d", len(pools))
	}
	if pools[0].NetworkKey != "10.31.103" {
		t.Errorf("expected network key 10.31.103, got %s", pools[0].NetworkKey)
	}
	if pools[0].Available != 2 {
		t.Errorf("expected 2 available, got %d", pools[0].Available)
	}
}

func TestGetNetworkIPs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/networks/10.31.103/ips" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		encodeJSON(t, w, []IPEntry{
			{IP: "10.31.103.5", Digit: "5", Status: "ASSIGNED", Cluster: "mycluster"},
			{IP: "10.31.103.6", Digit: "6", Status: "", Cluster: ""},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	entries, err := c.GetNetworkIPs("10.31.103")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestGetNetworkIPs_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"network not found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	entries, err := c.GetNetworkIPs("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entries != nil {
		t.Fatalf("expected nil entries, got %v", entries)
	}
}

func TestCreateNetwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/networks" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req map[string]interface{}
		decodeBody(t, r, &req)
		if req["network"] != "10.31.105" {
			t.Errorf("expected network 10.31.105, got %v", req["network"])
		}
		w.WriteHeader(http.StatusCreated)
		encodeJSON(t, w, map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.CreateNetwork("10.31.105", []string{"1", "2", "3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteNetwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/networks/10.31.105" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		encodeJSON(t, w, map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.DeleteNetwork("10.31.105")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAssignIP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/networks/10.31.103/assign" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req map[string]interface{}
		decodeBody(t, r, &req)
		if req["ip"] != "10.31.103.5" {
			t.Errorf("expected IP 10.31.103.5, got %v", req["ip"])
		}
		if req["cluster"] != "mycluster" {
			t.Errorf("expected cluster mycluster, got %v", req["cluster"])
		}
		encodeJSON(t, w, map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.AssignIP("10.31.103", "10.31.103.5", "mycluster", "ASSIGNED", false, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReleaseIP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/networks/10.31.103/release" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		encodeJSON(t, w, map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.ReleaseIP("10.31.103", "10.31.103.5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFindAvailableIPs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		encodeJSON(t, w, []IPEntry{
			{IP: "10.31.103.5", Digit: "5", Status: "ASSIGNED", Cluster: "mycluster"},
			{IP: "10.31.103.6", Digit: "6", Status: "", Cluster: ""},
			{IP: "10.31.103.7", Digit: "7", Status: "", Cluster: ""},
			{IP: "10.31.103.8", Digit: "8", Status: "PENDING", Cluster: "other"},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	available, err := c.FindAvailableIPs("10.31.103", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(available) != 2 {
		t.Fatalf("expected 2 available, got %d", len(available))
	}
	if available[0].IP != "10.31.103.6" {
		t.Errorf("expected first IP 10.31.103.6, got %s", available[0].IP)
	}
}

func TestFindAvailableIPs_NotEnough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		encodeJSON(t, w, []IPEntry{
			{IP: "10.31.103.5", Digit: "5", Status: "ASSIGNED", Cluster: "mycluster"},
			{IP: "10.31.103.6", Digit: "6", Status: "", Cluster: ""},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.FindAvailableIPs("10.31.103", 5)
	if err == nil {
		t.Fatal("expected error for not enough IPs")
	}
}

func TestNetworkExists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		encodeJSON(t, w, []NetworkPool{
			{NetworkKey: "10.31.103", Total: 3, Assigned: 1, Available: 2},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)

	exists, pool, err := c.NetworkExists("10.31.103")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected network to exist")
	}
	if pool.Total != 3 {
		t.Errorf("expected 3 total, got %d", pool.Total)
	}

	exists, _, err = c.NetworkExists("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected network to not exist")
	}
}

func TestEditIP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/networks/10.31.103/ips/5" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req map[string]interface{}
		decodeBody(t, r, &req)
		if req["cluster"] != "newcluster" {
			t.Errorf("expected cluster newcluster, got %v", req["cluster"])
		}
		encodeJSON(t, w, map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.EditIP("10.31.103", "5", "newcluster", "ASSIGNED", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddIPs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/networks/10.31.103/ips/add" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		encodeJSON(t, w, map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.AddIPs("10.31.103", []string{"10", "11"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateNetworkFromCIDR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/networks/cidr" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req map[string]interface{}
		decodeBody(t, r, &req)
		if req["cidr"] != "10.31.103.0/24" {
			t.Errorf("expected cidr 10.31.103.0/24, got %v", req["cidr"])
		}
		w.WriteHeader(http.StatusCreated)
		encodeJSON(t, w, map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.CreateNetworkFromCIDR("10.31.103.0/24", []string{"0", "255"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestAssignIPDNSFailure covers issue #187: clusterbook answers 200 even when
// the DNS record was not written, so a client that only checks the status code
// reports success for a half-completed operation.
func TestAssignIPDNSFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encodeJSON(t, w, map[string]string{
			"status":    "ok",
			"dns":       "failed",
			"dns_error": "ddwrt reload dnsmasq: no working reload command",
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.AssignIP("10.31.103", "10.31.103.5", "mycluster", "ASSIGNED", true, 0)
	if err == nil {
		t.Fatal("expected an error when the server reports dns: failed")
	}

	var dnsErr *DNSError
	if !errors.As(err, &dnsErr) {
		t.Fatalf("expected a *DNSError, got %T: %v", err, err)
	}
	if !strings.Contains(dnsErr.Error(), "no working reload command") {
		t.Errorf("error should carry the server detail, got: %v", dnsErr)
	}
}

// TestAssignIPDNSOK and the "skipped" case must not be mistaken for failures.
func TestAssignIPDNSNonFailureVerdicts(t *testing.T) {
	for _, verdict := range []string{"ok", "skipped", ""} {
		t.Run("dns="+verdict, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := map[string]string{"status": "ok"}
				if verdict != "" {
					body["dns"] = verdict
				}
				encodeJSON(t, w, body)
			}))
			defer srv.Close()

			c := NewClient(srv.URL)
			if err := c.AssignIP("10.31.103", "10.31.103.5", "mycluster", "ASSIGNED", true, 0); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestReleaseIPDNSFailure covers the release half of issue #187: a stale record
// that was never withdrawn must not be reported as released.
func TestReleaseIPDNSFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encodeJSON(t, w, map[string]string{
			"status":    "ok",
			"message":   "IP 10.31.103.5 released",
			"dns":       "failed",
			"dns_error": "ddwrt nvram commit: Process exited with status 127",
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	if err := c.ReleaseIP("10.31.103", "10.31.103.5"); err == nil {
		t.Fatal("expected an error when the DNS record was not withdrawn")
	}
}

// TestRenewLease checks the lease heartbeat endpoint and payload.
func TestRenewLease(t *testing.T) {
	var got map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/networks/10.31.103/ips/10.31.103.5/renew" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		decodeBody(t, r, &got)
		encodeJSON(t, w, map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	if err := c.RenewLease("10.31.103", "10.31.103.5", 3600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["lease_duration_seconds"] != float64(3600) {
		t.Errorf("expected lease_duration_seconds 3600, got %v", got["lease_duration_seconds"])
	}
}
