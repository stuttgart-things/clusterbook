/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"testing"
)

func TestGenerateIPs(t *testing.T) {

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

	GenerateIPs(ipList, requestedIPs)

}
