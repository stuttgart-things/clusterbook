/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"errors"
	"sort"
	"testing"

	"gopkg.in/yaml.v2"
)

// generateFixtureYAML carries every status shape production writes, not just the
// bare ones. The old fixture only had "ASSIGNED", which hid issue #196.
const generateFixtureYAML = `
10.31.103:
  3:
    status: ""
    cluster: ""
  4:
    status: PENDING
    cluster: losangeles
  5:
    status: ASSIGNED
    cluster: skyami
  6:
    status: ""
    cluster: ""
  7:
    status: ASSIGNED:DNS
    cluster: cicd-test4
  8:
    status: PENDING:DNS
    cluster: sthings-platform
  9:
    status: assigned
    cluster: set-over-grpc
  10:
    status: ""
    cluster: ""
10.100.136:
  224:
    status: ASSIGNED:DNS
    cluster: cicd-test4
  225:
    status: ASSIGNED:DNS
    cluster: sthings-platform
  227:
    status: ASSIGNED:DNS
    cluster: cicd-machinery-test5
`

func loadGenerateFixture(t *testing.T) map[string]IPs {
	t.Helper()
	var data map[string]IPs
	if err := yaml.Unmarshal([]byte(generateFixtureYAML), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	return data
}

func TestIsFree(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"", true},
		{"ASSIGNED", false},
		{"ASSIGNED:DNS", false},
		{"PENDING", false},
		{"PENDING:DNS", false},
		// Unknown statuses are busy: handing out an address someone marked in a
		// way we do not recognise is the failure, refusing it is not.
		{"assigned", false},
		{"MAINTENANCE", false},
	}

	for _, tt := range tests {
		if got := isFree(IPInfo{Status: tt.status}); got != tt.want {
			t.Errorf("isFree(%q) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestGenerateIPs_OffersOnlyFreeAddresses(t *testing.T) {
	data := loadGenerateFixture(t)

	// Ask for exactly as many as are free, so every free address comes back.
	got, err := GenerateIPs(data, 3, "10.31.103")
	if err != nil {
		t.Fatalf("GenerateIPs: %v", err)
	}

	sort.Strings(got)
	want := []string{"10.31.103.10", "10.31.103.3", "10.31.103.6"}
	if len(got) != len(want) {
		t.Fatalf("GenerateIPs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("GenerateIPs = %v, want %v", got, want)
		}
	}
}

// TestGenerateIPs_AssignedDNSIsNotAvailable reproduces the LabDA measurement from
// issue #196: three running clusters, all ASSIGNED:DNS, all offered as free.
func TestGenerateIPs_AssignedDNSIsNotAvailable(t *testing.T) {
	data := loadGenerateFixture(t)

	got, err := GenerateIPs(data, 1, "10.100.136")
	if err != nil {
		t.Fatalf("GenerateIPs: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("GenerateIPs offered in-use addresses: %v", got)
	}
}

func TestGenerateIPs_NotEnoughAddresses(t *testing.T) {
	data := loadGenerateFixture(t)

	got, err := GenerateIPs(data, 4, "10.31.103")
	if !errors.Is(err, ErrNotEnoughAddresses) {
		t.Fatalf("err = %v, want ErrNotEnoughAddresses", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no addresses alongside the error, got %v", got)
	}
}

func TestGenerateIPs_UnknownNetwork(t *testing.T) {
	data := loadGenerateFixture(t)

	if _, err := GenerateIPs(data, 1, "10.99.99"); !errors.Is(err, ErrNetworkNotFound) {
		t.Fatalf("err = %v, want ErrNetworkNotFound", err)
	}
}

// TestFreeAddressesAgreeAcrossCallers guards against the three answers to "is
// this address free?" drifting apart again: what the UI counts as Available must
// be exactly what GenerateIPs is willing to hand out.
func TestFreeAddressesAgreeAcrossCallers(t *testing.T) {
	data := loadGenerateFixture(t)

	for _, pool := range getPoolInfos(data) {
		if pool.Available == 0 {
			if got, err := GenerateIPs(data, 1, pool.NetworkKey); err != nil || len(got) != 0 {
				t.Errorf("%s: pool shows 0 available, GenerateIPs returned %v, %v", pool.NetworkKey, got, err)
			}
			continue
		}

		got, err := GenerateIPs(data, pool.Available, pool.NetworkKey)
		if err != nil {
			t.Errorf("%s: pool shows %d available, GenerateIPs failed: %v", pool.NetworkKey, pool.Available, err)
			continue
		}
		if len(got) != pool.Available {
			t.Errorf("%s: pool shows %d available, GenerateIPs offered %d", pool.NetworkKey, pool.Available, len(got))
		}
		if _, err := GenerateIPs(data, pool.Available+1, pool.NetworkKey); !errors.Is(err, ErrNotEnoughAddresses) {
			t.Errorf("%s: GenerateIPs can offer more than the %d the pool shows", pool.NetworkKey, pool.Available)
		}
	}
}

func TestPickRandomValues(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		n      int
	}{
		{
			name:   "Pick 3 values from a list of 5",
			values: []string{"a", "b", "c", "d", "e"},
			n:      3,
		},
		{
			name:   "Pick 0 values from a list of 5",
			values: []string{"a", "b", "c", "d", "e"},
			n:      0,
		},
		{
			name:   "Pick more values than available in the list",
			values: []string{"a", "b", "c"},
			n:      5,
		},
		{
			name:   "Pick values from an empty list",
			values: []string{},
			n:      3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pickRandomValues(tt.values, tt.n)
			if len(got) != tt.n && tt.n <= len(tt.values) {
				t.Errorf("pickRandomValues() = %v, want %v elements", got, tt.n)
			}
			if tt.n > len(tt.values) && len(got) != len(tt.values) {
				t.Errorf("pickRandomValues() = %v, want %v elements", got, len(tt.values))
			}
		})
	}
}
