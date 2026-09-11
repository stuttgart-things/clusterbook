/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"

	"gopkg.in/yaml.v2"
)

type IPInfo struct {
	Status         string `yaml:"status"`
	Cluster        string `yaml:"cluster"`
	LeaseExpiresAt int64  `yaml:"lease_expires_at,omitempty"`
}

type IPs map[string]IPInfo

var (
	ErrNotEnoughAddresses = errors.New("NOT ENOUGH AVAILABLE ADDRESSES")
	ErrNetworkNotFound    = errors.New("KEY DOES NOT EXIST")
)

// isFree reports whether an address may be handed out. Only an empty status is
// free: release and the lease reclaimer both reset to "", and nothing writes any
// other "free" marker. Matching on known busy statuses instead is what issue #196
// was — "ASSIGNED:DNS" did not equal "ASSIGNED", so in-use addresses were offered.
// Every allocator and the pool counts must ask this, never test Status themselves.
func isFree(info IPInfo) bool {
	return info.Status == ""
}

func GenerateIPs(ipList map[string]IPs, requestedIPs int, networkKey string) (randomValues []string, err error) {
	var availableAddresses []string

	if ipList, ok := ipList[networkKey]; ok {
		fmt.Println("KEY EXISTS", ipList)

		for ip, adressStatus := range ipList {

			address := networkKey + "." + ip

			fmt.Println("IP-Address:", address)
			fmt.Println("ClusterName:", adressStatus.Cluster)
			fmt.Println("Status:", adressStatus.Status)

			if isFree(adressStatus) {
				availableAddresses = append(availableAddresses, address)
			}
		}

		if len(availableAddresses) == 0 {
			fmt.Println("NO AVAILABLE ADDRESSES")
		} else if len(availableAddresses) >= requestedIPs {
			randomValues = pickRandomValues(availableAddresses, requestedIPs)
			fmt.Printf("PICKED IPs %v\n", randomValues)

		} else {
			fmt.Println("NOT ENOUGH AVAILABLE ADDRESSES")
			err = ErrNotEnoughAddresses
		}

	} else {
		fmt.Println("KEY DOES NOT EXIST")
		err = ErrNetworkNotFound
	}

	return
}

func pickRandomValues(slice []string, count int) []string {
	source := rand.NewSource(time.Now().UnixNano())
	rng := rand.New(source)

	if count >= len(slice) {
		return slice
	}

	picked := make([]string, 0, count)
	indices := rng.Perm(len(slice))

	for i := 0; i < count; i++ {
		picked = append(picked, slice[indices[i]])
	}

	return picked
}

func LoadYAMLStructure(yamlData []byte) (ipList map[string]IPs) {
	err := yaml.Unmarshal([]byte(yamlData), &ipList)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	return
}
