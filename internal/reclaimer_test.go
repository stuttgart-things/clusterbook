package internal

import (
	"testing"
	"time"
)

func TestFindExpiredLeases(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	ipList := map[string]IPs{
		"10.31.103": {
			"4": {Status: "ASSIGNED", Cluster: "a", LeaseExpiresAt: 0},                    // no lease
			"5": {Status: "ASSIGNED", Cluster: "b", LeaseExpiresAt: now.Unix() - 10},      // expired
			"6": {Status: "ASSIGNED:DNS", Cluster: "c", LeaseExpiresAt: now.Unix() - 100}, // expired w/ DNS
			"7": {Status: "ASSIGNED", Cluster: "d", LeaseExpiresAt: now.Unix() + 100},     // still valid
		},
	}

	expired := FindExpiredLeases(ipList, now)

	if len(expired) != 2 {
		t.Fatalf("expected 2 expired, got %d", len(expired))
	}

	got := map[string]ExpiredLease{}
	for _, e := range expired {
		got[e.IPDigit] = e
	}
	if e, ok := got["5"]; !ok || e.HadDNS {
		t.Errorf("digit 5: want expired without DNS, got %+v ok=%v", e, ok)
	}
	if e, ok := got["6"]; !ok || !e.HadDNS {
		t.Errorf("digit 6: want expired with DNS, got %+v ok=%v", e, ok)
	}
}

func TestReclaimExpiredLeasesClearsEntry(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	ipList := map[string]IPs{
		"10.31.103": {
			"5": {Status: "ASSIGNED", Cluster: "b", LeaseExpiresAt: now.Unix() - 10},
			"7": {Status: "ASSIGNED", Cluster: "d", LeaseExpiresAt: now.Unix() + 100},
		},
	}

	reclaimed := ReclaimExpiredLeases(ipList, now)
	if len(reclaimed) != 1 {
		t.Fatalf("expected 1 reclaimed, got %d", len(reclaimed))
	}

	cleared := ipList["10.31.103"]["5"]
	if cleared.Status != "" || cleared.Cluster != "" || cleared.LeaseExpiresAt != 0 {
		t.Errorf("digit 5: want fully cleared, got %+v", cleared)
	}

	untouched := ipList["10.31.103"]["7"]
	if untouched.Status != "ASSIGNED" || untouched.Cluster != "d" {
		t.Errorf("digit 7: want untouched, got %+v", untouched)
	}
}

const reclaimConfigYAML = `
10.31.103:
  "6":
    status: "ASSIGNED:DNS"
    cluster: c
    lease_expires_at: 1699999995
  "7":
    status: "ASSIGNED"
    cluster: d
    lease_expires_at: 1699999995
`

func TestReclaimOnce_SavesThenRemovesDNS(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	dir, name := setupTestConfig(t, reclaimConfigYAML)

	srv, err := NewFakeDDWRTServer("root", "testpass")
	if err != nil {
		t.Fatalf("fake ssh server: %v", err)
	}
	defer srv.Close()
	srv.NvramSet("dnsmasq_options", "address=/c.sthings.lab/10.31.103.6")
	ddwrt := NewDDWRTClient("true", srv.Addr, "root", "testpass", "sthings.lab")

	reclaimed, err := reclaimOnce("disk", dir, name, now, nil, ddwrt)
	if err != nil {
		t.Fatalf("reclaimOnce: %v", err)
	}
	if len(reclaimed) != 2 {
		t.Fatalf("expected 2 reclaimed, got %d", len(reclaimed))
	}

	ipList, err := LoadProfile("disk", dir, name)
	if err != nil {
		t.Fatal(err)
	}
	for _, digit := range []string{"6", "7"} {
		if got := ipList["10.31.103"][digit]; !isFree(got) || got.Cluster != "" {
			t.Errorf("digit %s: want cleared in the saved config, got %+v", digit, got)
		}
	}

	if opts := srv.NvramGet("dnsmasq_options"); opts != "" {
		t.Errorf("expected DNS entry removed, still have: %q", opts)
	}
}

// Issue #200: the reclaimer removed DNS before saving and ignored the save error,
// so a failed write left an address assigned in the ledger with its record gone.
func TestReclaimOnce_SaveFailureKeepsDNS(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	dir, name := readOnlyConfig(t, reclaimConfigYAML)

	exec := newFakeExecutor()
	exec.nvram["dnsmasq_options"] = "address=/c.sthings.lab/10.31.103.6"
	ddwrt := newDDWRTClientWithExecutor("sthings.lab", exec)

	reclaimed, err := reclaimOnce("disk", dir, name, now, nil, ddwrt)
	if err == nil {
		t.Fatal("expected the save error to be returned")
	}
	if len(reclaimed) != 0 {
		t.Errorf("nothing may count as reclaimed when the save failed, got %v", reclaimed)
	}
	if len(exec.calls) != 0 {
		t.Errorf("DNS was touched for an unsaved reclaim: %v", exec.calls)
	}
}
