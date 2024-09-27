/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"fmt"
	"time"

	"math/rand"
)

func GenerateIPs(ipList map[string]map[string]string, requestedIPs int) {

	var availableAddresses []string

	for address, clusterInformation := range ipList {
		fmt.Printf("Address: %s\n", address)

		for clusterName, adressStatus := range clusterInformation {
			fmt.Printf("  ClusterName: %s, Address Status: %s\n", clusterName, adressStatus)

			switch adressStatus {
			case "PENDING", "ASSIGNED":
				// DO NOTHING
			default:
				availableAddresses = append(availableAddresses, address)
			}

		}
	}

	fmt.Printf("AVAILABLE ADDRESSES: %v\n", availableAddresses)

	randomValues := pickRandomValues(availableAddresses, requestedIPs)

	fmt.Printf("PICKED IPs %v\n", randomValues)

	// innerMap, exists := ipList["outerKey1"]
	// if exists {
	// 	innerValue, innerExists := innerMap["innerKey1"]
	// 	if innerExists {
	// 		fmt.Printf("The value for 'outerKey1' -> 'innerKey1' is: %s\n", innerValue)
	// 	} else {
	// 		fmt.Println("The inner key 'innerKey1' does not exist.")
	// 	}
	// } else {
	// 	fmt.Println("The outer key 'outerKey1' does not exist.")
	// }

}

// Funktion zum zufälligen Auswählen von Werten aus einer String-Slice
func pickRandomValues(slice []string, count int) []string {

	source := rand.NewSource(time.Now().UnixNano())
	rng := rand.New(source)

	if count >= len(slice) {
		return slice // Wenn die Anzahl der gewünschten Werte größer oder gleich der Länge der Slice ist, geben Sie die gesamte Slice zurück
	}

	picked := make([]string, 0, count)
	indices := rng.Perm(len(slice)) // Zufällige Permutation der Indizes

	for i := 0; i < count; i++ {
		picked = append(picked, slice[indices[i]])
	}

	return picked
}
