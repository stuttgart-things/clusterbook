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

	reclaimed := ReclaimExpiredLeases(ipList, now, nil, nil)
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

func TestReclaimExpiredLeasesCallsDDWRTDelete(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	ipList := map[string]IPs{
		"10.31.103": {
			"6": {Status: "ASSIGNED:DNS", Cluster: "c", LeaseExpiresAt: now.Unix() - 5},
			"7": {Status: "ASSIGNED", Cluster: "d", LeaseExpiresAt: now.Unix() - 5}, // no DNS suffix
		},
	}

	srv, err := NewFakeDDWRTServer("root", "testpass")
	if err != nil {
		t.Fatalf("fake ssh server: %v", err)
	}
	defer srv.Close()

	srv.NvramSet("dnsmasq_options", "address=/c.sthings.lab/10.31.103.6")

	ddwrt := NewDDWRTClient("true", srv.Addr, "root", "testpass", "sthings.lab")
	ReclaimExpiredLeases(ipList, now, nil, ddwrt)

	opts := srv.NvramGet("dnsmasq_options")
	if opts != "" {
		t.Errorf("expected DNS entry removed, still have: %q", opts)
	}
}
