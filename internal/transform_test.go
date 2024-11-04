/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"reflect"
	"testing"
)

func TestConvertToCRFormat(t *testing.T) {
	// Testdaten
	info := map[string]IPs{
		"10.31.103": {
			"3": {Status: "assigned", Cluster: "sandiego"},
			"4": {Status: "pending", Cluster: ""},
			"5": {Status: "", Cluster: ""},
		},
		"10.31.104": {
			"4": {Status: "pending", Cluster: "losangeles"},
			"5": {Status: "", Cluster: ""},
		},
	}

	expected := map[string][]string{
		"10.31.103": {
			"3:assigned:sandiego",
			"4",
			"5",
		},
		"10.31.104": {
			"4:pending:losangeles",
			"5",
		},
	}

	result := ConvertToCRFormat(info)

	// Vergleiche die Ergebnisse
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected %v, but got %v", expected, result)
	}
}

func TestGetLastIPDigit(t *testing.T) {
	tests := []struct {
		ip        string
		expected  string
		shouldErr bool
	}{
		{"10.31.104.6", "6", false},             // Valid IP
		{"192.168.1.1", "1", false},             // Valid IP
		{"172.16.0.255", "255", false},          // Valid IP
		{"10.31.104", "", true},                 // Invalid: Only 3 segments
		{"10.31.104.6.7", "7", false},           // Valid: Extra segment
		{"256.100.50.25", "25", false},          // Valid but out of range
		{"10.31.104.a", "a", false},             // Valid but non-numeric segment
		{"", "", true},                          // Invalid: Empty string
		{"10.31.104.6:extra", "6:extra", false}, // Valid with extra data
	}

	for _, test := range tests {
		result, err := GetLastIPDigit(test.ip)

		if test.shouldErr {
			if err == nil {
				t.Errorf("Expected an error for IP %s, but got none", test.ip)
			}
		} else {
			if err != nil {
				t.Errorf("Unexpected error for IP %s: %v", test.ip, err)
			}
			if result != test.expected {
				t.Errorf("For IP %s, expected %s but got %s", test.ip, test.expected, result)
			}
		}
	}
}

// Test für die Funktion TruncateIP
func TestTruncateIP(t *testing.T) {
	tests := []struct {
		ip        string
		expected  string
		shouldErr bool
	}{
		{"10.31.104.6", "10.31.104", false},
		{"192.168.1.1", "192.168.1", false},
		{"172.16.0.255", "172.16.0", false},
		{"", "", true}, // Invalid: Empty string
	}

	for _, test := range tests {
		result, err := TruncateIP(test.ip)

		if test.shouldErr {
			if err == nil {
				t.Errorf("Expected an error for IP %s, but got none", test.ip)
			}
		} else {
			if err != nil {
				t.Errorf("Unexpected error for IP %s: %v", test.ip, err)
			}
			if result != test.expected {
				t.Errorf("For IP %s, expected %s but got %s", test.ip, test.expected, result)
			}
		}
	}
}
