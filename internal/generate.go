/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"errors"
	"fmt"
	"log"
	"time"

	"math/rand"

	"gopkg.in/yaml.v2"
)

type IPInfo struct {
	Status  string `yaml:"status"`
	Cluster string `yaml:"cluster"`
}

type IPs map[string]IPInfo

func GenerateIPs(ipList map[string]IPs, requestedIPs int, networkKey string) (randomValues []string, err error) {

	var availableAddresses []string

	if ipList, ok := ipList[networkKey]; ok {
		fmt.Println("KEY EXISTS", ipList)

		for ip, adressStatus := range ipList {

			address := networkKey + "." + ip

			fmt.Println("IP-Address:", address)

			fmt.Println("ClusterName:", adressStatus.Cluster)
			fmt.Println("Status:", adressStatus.Status)

			switch adressStatus.Status {
			case "PENDING", "ASSIGNED":
				// DO NOTHING
			default:
				availableAddresses = append(availableAddresses, address)
			}
		}

		fmt.Printf("AVAILABLE ADDRESSES: %v\n", availableAddresses)

		randomValues = pickRandomValues(availableAddresses, requestedIPs)

		fmt.Printf("PICKED IPs %v\n", randomValues)

	} else {
		fmt.Println("KEY DOES NOT EXIST")
		err = errors.New("KEY DOES NOT EXIST")
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
