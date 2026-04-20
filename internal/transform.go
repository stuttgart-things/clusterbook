/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"fmt"
	"strconv"
	"strings"
)

const crLeasePrefix = "exp="

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
			var entry string
			if details.Status != "" && details.Cluster != "" {
				// Format: "ipDigit:status:cluster"
				entry = fmt.Sprintf("%s:%s:%s", ipDigit, details.Status, details.Cluster)
			} else {
				// Just add the ipDigit if status or cluster is empty
				entry = ipDigit
			}
			if details.LeaseExpiresAt != 0 {
				entry = fmt.Sprintf("%s:%s%d", entry, crLeasePrefix, details.LeaseExpiresAt)
			}
			networks[ip] = append(networks[ip], entry)
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

			info := IPInfo{}
			if len(parts) > 0 && strings.HasPrefix(parts[len(parts)-1], crLeasePrefix) {
				raw := strings.TrimPrefix(parts[len(parts)-1], crLeasePrefix)
				if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
					info.LeaseExpiresAt = v
				}
				parts = parts[:len(parts)-1]
			}

			ipDigit := parts[0]

			if len(parts) > 1 {
				info.Status = parts[1]
				if len(parts) > 2 {
					// Handle status suffix like "ASSIGNED:DNS" where DNS is part of the status
					// CR format: "digit:STATUS:DNS:cluster" (4 parts) or "digit:STATUS:cluster" (3 parts)
					if len(parts) == 4 {
						info.Status = parts[1] + ":" + parts[2]
						info.Cluster = parts[3]
					} else {
						info.Cluster = parts[2]
					}
				}
			}

			ipMap[ipDigit] = info
		}

		result[ip] = ipMap
	}

	return result
}
