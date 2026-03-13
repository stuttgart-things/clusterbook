/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// CIDRToNetworks parses a CIDR notation string and returns a map of network prefix keys
// (first 3 octets) to slices of last-octet strings. Network and broadcast addresses
// are automatically excluded. Additional IPs can be excluded via the reserved parameter
// (as last-octet strings, e.g. "1" for the gateway).
func CIDRToNetworks(cidr string, reserved []string) (map[string][]string, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR notation: %w", err)
	}

	ip4 := ip.To4()
	if ip4 == nil {
		return nil, fmt.Errorf("only IPv4 CIDR is supported")
	}

	// Build reserved set for quick lookup (keyed by full IP string)
	reservedSet := make(map[string]bool)
	for _, r := range reserved {
		reservedSet[r] = true
	}

	// Calculate network and broadcast addresses
	maskLen, _ := ipNet.Mask.Size()
	networkIP := ipNet.IP.To4()
	networkInt := binary.BigEndian.Uint32(networkIP)
	hostBits := uint(32 - maskLen)

	// For /31 and /32, no broadcast/network exclusion per RFC 3021
	excludeNetBroadcast := hostBits > 1
	var broadcastInt uint32
	if excludeNetBroadcast {
		broadcastInt = networkInt | (0xFFFFFFFF >> uint(maskLen))
	}

	result := make(map[string][]string)

	// Iterate through all IPs in the CIDR range
	for current := networkInt; ipNet.Contains(uint32ToIP(current)); current++ {
		// Skip network address
		if excludeNetBroadcast && current == networkInt {
			continue
		}
		// Skip broadcast address
		if excludeNetBroadcast && current == broadcastInt {
			continue
		}

		currentIP := uint32ToIP(current)
		octets := strings.Split(currentIP.String(), ".")
		if len(octets) != 4 {
			continue
		}

		networkKey := strings.Join(octets[:3], ".")
		lastOctet := octets[3]

		// Skip reserved IPs
		if reservedSet[lastOctet] {
			continue
		}

		result[networkKey] = append(result[networkKey], lastOctet)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("CIDR %s produced no usable IPs", cidr)
	}

	return result, nil
}

// ValidateCIDR checks if a string is valid CIDR notation
func ValidateCIDR(cidr string) error {
	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR notation: %w", err)
	}

	ip, _, _ := net.ParseCIDR(cidr)
	if ip.To4() == nil {
		return fmt.Errorf("only IPv4 CIDR is supported")
	}

	return nil
}

// CIDRToIPList returns all usable host IPs in a CIDR range as full IP address strings
func CIDRToIPList(cidr string, reserved []string) ([]string, error) {
	networks, err := CIDRToNetworks(cidr, reserved)
	if err != nil {
		return nil, err
	}

	var ips []string
	// Sort network keys for deterministic output
	keys := sortedKeys(networks)
	for _, key := range keys {
		for _, octet := range networks[key] {
			ips = append(ips, key+"."+octet)
		}
	}

	return ips, nil
}

// CIDRNetworkKey returns the 3-octet network prefix for a /24 or smaller CIDR
func CIDRNetworkKey(cidr string) (string, error) {
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR notation: %w", err)
	}

	ip4 := ip.To4()
	if ip4 == nil {
		return "", fmt.Errorf("only IPv4 CIDR is supported")
	}

	return fmt.Sprintf("%d.%d.%d", ip4[0], ip4[1], ip4[2]), nil
}

func uint32ToIP(n uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, n)
	return ip
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Sort by parsing octets numerically
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if compareNetworkKeys(keys[i], keys[j]) > 0 {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

func compareNetworkKeys(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	for i := 0; i < 3 && i < len(aParts) && i < len(bParts); i++ {
		aNum, _ := strconv.Atoi(aParts[i])
		bNum, _ := strconv.Atoi(bParts[i])
		if aNum != bNum {
			return aNum - bNum
		}
	}
	return 0
}
