package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// Client is a REST client for the clusterbook IPAM API.
type Client struct {
	endpoint   string
	httpClient *http.Client
}

// NewClient creates a new clusterbook REST client.
func NewClient(endpoint string) *Client {
	return &Client{
		endpoint:   strings.TrimRight(endpoint, "/"),
		httpClient: &http.Client{},
	}
}

// NetworkPool holds summary info for a network pool (matches NetworkPoolInfo).
type NetworkPool struct {
	NetworkKey string `json:"NetworkKey"`
	Total      int    `json:"Total"`
	Assigned   int    `json:"Assigned"`
	Pending    int    `json:"Pending"`
	Available  int    `json:"Available"`
}

// IPEntry holds a single IP entry (matches IPEntry from web.go).
type IPEntry struct {
	IP      string `json:"IP"`
	Digit   string `json:"Digit"`
	Status  string `json:"Status"`
	Cluster string `json:"Cluster"`
}

// ListNetworks returns all network pools.
func (c *Client) ListNetworks() ([]NetworkPool, error) {
	resp, err := c.httpClient.Get(c.endpoint + "/api/v1/networks")
	if err != nil {
		return nil, fmt.Errorf("listing networks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.readError(resp)
	}

	var pools []NetworkPool
	if err := json.NewDecoder(resp.Body).Decode(&pools); err != nil {
		return nil, fmt.Errorf("decoding networks: %w", err)
	}
	return pools, nil
}

// GetNetworkIPs returns all IP entries for a network.
func (c *Client) GetNetworkIPs(networkKey string) ([]IPEntry, error) {
	resp, err := c.httpClient.Get(c.endpoint + "/api/v1/networks/" + networkKey + "/ips")
	if err != nil {
		return nil, fmt.Errorf("getting network IPs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, c.readError(resp)
	}

	var entries []IPEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decoding network IPs: %w", err)
	}
	return entries, nil
}

// CreateNetwork creates a network with a flat list of IP suffixes.
func (c *Client) CreateNetwork(networkKey string, ips []string) error {
	body := map[string]interface{}{
		"network": networkKey,
		"ips":     ips,
	}
	return c.postJSON("/api/v1/networks", body, http.StatusCreated)
}

// CreateNetworkFromCIDR creates networks from a CIDR notation.
func (c *Client) CreateNetworkFromCIDR(cidr string, reserved []string) error {
	body := map[string]interface{}{
		"cidr":     cidr,
		"reserved": reserved,
	}
	return c.postJSON("/api/v1/networks/cidr", body, http.StatusCreated)
}

// DeleteNetwork deletes a network.
func (c *Client) DeleteNetwork(networkKey string) error {
	req, err := http.NewRequest(http.MethodDelete, c.endpoint+"/api/v1/networks/"+networkKey, nil)
	if err != nil {
		return fmt.Errorf("creating delete request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("deleting network: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.readError(resp)
	}
	return nil
}

// AssignIP assigns an IP address to a cluster.
func (c *Client) AssignIP(networkKey, ip, cluster, status string, createDNS bool) error {
	body := map[string]interface{}{
		"ip":         ip,
		"cluster":    cluster,
		"status":     status,
		"create_dns": createDNS,
	}
	return c.postJSON("/api/v1/networks/"+networkKey+"/assign", body, http.StatusOK)
}

// ReleaseIP releases an IP address.
func (c *Client) ReleaseIP(networkKey, ip string) error {
	body := map[string]interface{}{
		"ip": ip,
	}
	return c.postJSON("/api/v1/networks/"+networkKey+"/release", body, http.StatusOK)
}

// EditIP updates an existing IP assignment.
func (c *Client) EditIP(networkKey, ipDigit, cluster, status string, createDNS bool) error {
	body := map[string]interface{}{
		"cluster":    cluster,
		"status":     status,
		"create_dns": createDNS,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling edit request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPut, c.endpoint+"/api/v1/networks/"+networkKey+"/ips/"+ipDigit, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating edit request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("editing IP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.readError(resp)
	}
	return nil
}

// AddIPs adds IP suffixes to an existing network.
func (c *Client) AddIPs(networkKey string, ips []string) error {
	body := map[string]interface{}{
		"ips": ips,
	}
	return c.postJSON("/api/v1/networks/"+networkKey+"/ips/add", body, http.StatusCreated)
}

// FindAvailableIPs returns up to count available (unassigned) IPs from a network, sorted.
func (c *Client) FindAvailableIPs(networkKey string, count int) ([]IPEntry, error) {
	entries, err := c.GetNetworkIPs(networkKey)
	if err != nil {
		return nil, err
	}
	if entries == nil {
		return nil, fmt.Errorf("network %s not found", networkKey)
	}

	var available []IPEntry
	for _, e := range entries {
		if e.Status == "" {
			available = append(available, e)
		}
	}

	sort.Slice(available, func(i, j int) bool {
		return available[i].IP < available[j].IP
	})

	if count > len(available) {
		return nil, fmt.Errorf("not enough available IPs: need %d, have %d", count, len(available))
	}

	return available[:count], nil
}

// NetworkExists checks if a network exists.
func (c *Client) NetworkExists(networkKey string) (bool, *NetworkPool, error) {
	pools, err := c.ListNetworks()
	if err != nil {
		return false, nil, err
	}
	for _, p := range pools {
		if p.NetworkKey == networkKey {
			return true, &p, nil
		}
	}
	return false, nil, nil
}

func (c *Client) postJSON(path string, body interface{}, expectedStatus int) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	resp, err := c.httpClient.Post(c.endpoint+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != expectedStatus {
		return c.readError(resp)
	}
	return nil
}

func (c *Client) readError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
}
