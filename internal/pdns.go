/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
)

// PDNSClient manages PowerDNS API interactions
type PDNSClient struct {
	URL    string
	Token  string
	Zone   string
	client *http.Client
}

// NewPDNSClient creates a new PowerDNS client from config.
// Returns nil if PDNS is not enabled.
func NewPDNSClient(enabled, url, token, zone string) *PDNSClient {
	if enabled != "true" || url == "" || token == "" || zone == "" {
		return nil
	}

	// Ensure zone ends with a dot (FQDN)
	if !strings.HasSuffix(zone, ".") {
		zone += "."
	}

	log.Printf("PDNS INTEGRATION ENABLED (zone: %s)", zone)
	return &PDNSClient{
		URL:   url,
		Token: token,
		Zone:  zone,
		client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec G402 — matches Ansible role validate_certs: false
			},
		},
	}
}

// rrset represents a PowerDNS RRSet for the PATCH API
type rrset struct {
	Name       string      `json:"name"`
	Type       string      `json:"type"`
	TTL        int         `json:"ttl"`
	ChangeType string      `json:"changetype"`
	Records    []pdRecord  `json:"records,omitempty"`
	Comments   []pdComment `json:"comments,omitempty"`
}

type pdRecord struct {
	Content  string `json:"content"`
	Disabled bool   `json:"disabled"`
}

type pdComment struct {
	Account string `json:"account"`
	Content string `json:"content"`
}

type rrsetPayload struct {
	RRSets []rrset `json:"rrsets"`
}

// CreateRecord creates a wildcard A record: *.{cluster}.{zone} → ip
func (c *PDNSClient) CreateRecord(cluster, ip string) {
	if c == nil || cluster == "" {
		return
	}

	fqdn := fmt.Sprintf("*.%s.%s", cluster, c.Zone)

	payload := rrsetPayload{
		RRSets: []rrset{
			{
				Name:       fqdn,
				Type:       "A",
				TTL:        60,
				ChangeType: "REPLACE",
				Records:    []pdRecord{{Content: ip, Disabled: false}},
				Comments:   []pdComment{{Account: "", Content: "managed by clusterbook"}},
			},
		},
	}

	c.patchZone(payload, "CREATE", fqdn, ip)
}

// DeleteRecord deletes the wildcard A record for a cluster
func (c *PDNSClient) DeleteRecord(cluster string) {
	if c == nil || cluster == "" {
		return
	}

	fqdn := fmt.Sprintf("*.%s.%s", cluster, c.Zone)

	payload := rrsetPayload{
		RRSets: []rrset{
			{
				Name:       fqdn,
				Type:       "A",
				ChangeType: "DELETE",
			},
		},
	}

	c.patchZone(payload, "DELETE", fqdn, "")
}

// TestDNS resolves test.{cluster}.{zone} and checks if it matches the expected IP.
// Returns (resolvedIP, match, error).
func (c *PDNSClient) TestDNS(cluster, expectedIP string) (string, bool, error) {
	if c == nil {
		return "", false, fmt.Errorf("PDNS not enabled")
	}

	zone := strings.TrimSuffix(c.Zone, ".")
	fqdn := fmt.Sprintf("test.%s.%s", cluster, zone)

	ips, err := net.LookupHost(fqdn)
	if err != nil {
		return "", false, fmt.Errorf("lookup %s: %w", fqdn, err)
	}

	for _, ip := range ips {
		if ip == expectedIP {
			return ip, true, nil
		}
	}

	return strings.Join(ips, ","), false, nil
}

// patchZone sends the PATCH request to PowerDNS
func (c *PDNSClient) patchZone(payload rrsetPayload, action, fqdn, ip string) {
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("PDNS %s ERROR (marshal): %v", action, err)
		return
	}

	url := fmt.Sprintf("%s/api/v1/servers/localhost/zones/%s", c.URL, c.Zone)
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("PDNS %s ERROR (request): %v", action, err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.Token)

	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("PDNS %s ERROR (http): %v", action, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		log.Printf("PDNS %s FAILED: %s → HTTP %d", action, fqdn, resp.StatusCode)
		return
	}

	if ip != "" {
		log.Printf("PDNS %s OK: %s → %s", action, fqdn, ip)
	} else {
		log.Printf("PDNS %s OK: %s", action, fqdn)
	}
}
