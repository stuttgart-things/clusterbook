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
