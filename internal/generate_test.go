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

	ipList := make(map[string]map[string]string)

	ipList["10.31.103.3"] = map[string]string{
		"": "",
	}

	ipList["10.31.103.4"] = map[string]string{
		"losangeles": "PENDING",
	}

	ipList["10.31.103.5"] = map[string]string{
		"skyami": "ASSIGNED",
	}

	ipList["10.31.103.7"] = map[string]string{
		"": "",
	}

	ipList["10.31.103.8"] = map[string]string{
		"": "",
	}

	ipList["10.31.103.9"] = map[string]string{
		"cicd": "PENDING",
	}

	requestedIPs := 4

	// GenerateIPs(ipList, requestedIPs)
	GenerateIPs(data, requestedIPs)
}
