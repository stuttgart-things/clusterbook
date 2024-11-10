/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"fmt"
	"strings"
)

func TruncateIP(ip string) (string, error) {
	segments := strings.Split(ip, ".")

	// Check if there are exactly 4 segments in the IP address
	if len(segments) != 4 {
		return "", fmt.Errorf("INVALID IP FORMAT: %s", ip)
	}

	// Join the first three segments with dots
	return strings.Join(segments[:3], "."), nil
}

func GetLastIPDigit(ip string) (string, error) {
	segments := strings.Split(ip, ".")

	// Check if the IP address has at least one segment
	if len(segments) < 4 {
		return "", fmt.Errorf("INVALID IP FORMAT: %s", ip)
	}

	// Return the last segment
	return segments[len(segments)-1], nil
}

func ConvertToCRFormat(info map[string]IPs) map[string][]string {
	networks := make(map[string][]string)

	// Iterate over the info map to populate the networks map
	for ip, ipDetails := range info {
		for ipDigit, details := range ipDetails {
			if details.Status != "" && details.Cluster != "" {
				// Format: "ipDigit:status:cluster"
				networks[ip] = append(networks[ip], fmt.Sprintf("%s:%s:%s", ipDigit, details.Status, details.Cluster))
			} else {
				// Just add the ipDigit if status or cluster is empty
				networks[ip] = append(networks[ip], ipDigit)
			}
		}
	}

	return networks
}

func ConvertFromCRFormat(data map[string][]string) map[string]IPs {
	result := make(map[string]IPs)

	for ip, entries := range data {
		ipMap := make(IPs)

		for _, entry := range entries {
			parts := strings.Split(entry, ":")
			ipDigit := parts[0]

			info := IPInfo{}
			if len(parts) > 1 {
				info.Status = parts[1]
				if len(parts) > 2 {
					info.Cluster = parts[2]
				}
			}

			ipMap[ipDigit] = info
		}

		result[ip] = ipMap
	}

	return result
}
