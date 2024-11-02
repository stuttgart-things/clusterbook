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
