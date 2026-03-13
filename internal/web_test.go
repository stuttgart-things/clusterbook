package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
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

		handleAPIClusterInfo(w, req, "disk", dir, name)

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

		handleAPIClusterInfo(w, req, "disk", dir, name)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})
}
