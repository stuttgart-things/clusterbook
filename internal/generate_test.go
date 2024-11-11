/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"fmt"
	"log"
	"testing"

	"gopkg.in/yaml.v2"
)

func TestGenerateIPs(t *testing.T) {
	yamlData := `
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
    status: ""
    cluster: ""
  8:
    status: ""
    cluster: ""
  9:
    status: PENDING
    cluster: cicd
  10:
    status: ""
    cluster: ""
10.31.104:
  4:
    status: PENDING
    cluster: losangeles
  5:
    status: ASSIGNED
    cluster: miami
  6:
    status: ""
    cluster: ""
  7:
    status: ""
    cluster: ""
`

	// READ YAML FILE
	var data map[string]IPs
	err := yaml.Unmarshal([]byte(yamlData), &data)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	fmt.Println(data)

	requestedIPs := 8

	// GenerateIPs(ipList, requestedIPs)
	ips, err := GenerateIPs(data, requestedIPs, "10.31.103")
	fmt.Println(ips, err)
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
