/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// NetworkPoolInfo holds summary info for a network pool
type NetworkPoolInfo struct {
	NetworkKey string
	Total      int
	Assigned   int
	Pending    int
	Available  int
}

// IPEntry holds a single IP entry for display
type IPEntry struct {
	IP             string `json:"ip"`
	Digit          string `json:"digit"`
	Status         string `json:"status"`
	Cluster        string `json:"cluster"`
	FQDN           string `json:"fqdn,omitempty"`
	LeaseExpiresAt int64  `json:"lease_expires_at,omitempty"`
}

// dnsResult records the outcome of the DNS half of an IP operation.
//
// Both providers log their own failures, but before issue #187 every call site
// discarded the returned error, so the HTTP response looked identical whether
// the record was written or not. Handlers now thread a dnsResult through and
// report it, giving an automated caller something to react to.
type dnsResult struct {
	attempted bool
	errs      []string
}

// add records a provider error; nil errors (including the no-op returned by a
// disabled provider) are ignored.
func (r *dnsResult) add(err error) {
	if err != nil {
		r.errs = append(r.errs, err.Error())
	}
}

// create writes the record on every enabled provider, collecting failures.
func (r *dnsResult) create(pdns *PDNSClient, ddwrt *DDWRTClient, cluster, ip string) {
	r.attempted = true
	r.add(pdns.CreateRecord(cluster, ip)) // nil-receiver safe
	if ddwrt != nil {
		r.add(ddwrt.CreateRecord(cluster, ip))
	}
}

// remove deletes the record on every enabled provider, collecting failures.
func (r *dnsResult) remove(pdns *PDNSClient, ddwrt *DDWRTClient, cluster string) {
	r.attempted = true
	r.add(pdns.DeleteRecord(cluster)) // nil-receiver safe
	if ddwrt != nil {
		r.add(ddwrt.DeleteRecord(cluster))
	}
}

func (r dnsResult) failed() bool { return len(r.errs) > 0 }

// status is "skipped" when no DNS work was requested, "failed" when any
// provider errored, "ok" otherwise.
func (r dnsResult) status() string {
	switch {
	case !r.attempted:
		return "skipped"
	case r.failed():
		return "failed"
	default:
		return "ok"
	}
}

// message joins every provider error into one line.
func (r dnsResult) message() string { return strings.Join(r.errs, "; ") }

// annotate adds the DNS outcome to a JSON response body. The IP operation
// itself has already been persisted, so the status code stays 200 and the
// caller inspects "dns" to learn whether the DNS half went through.
func (r dnsResult) annotate(resp map[string]any) map[string]any {
	resp["dns"] = r.status()
	if r.failed() {
		resp["dns_error"] = r.message()
	}
	return resp
}

// banner renders an HTMX-visible warning, or "" when nothing failed. It is
// written ahead of the re-rendered table so the UI shows the same failure the
// REST API reports.
func (r dnsResult) banner() string {
	if !r.failed() {
		return ""
	}
	return `<div style="background:#4c1d1d;border:1px solid #b91c1c;color:#fecaca;padding:0.5rem 0.75rem;border-radius:4px;margin-bottom:0.75rem;font-size:0.8rem;">` +
		`<strong>DNS operation failed.</strong> The IP entry was saved, but the DNS record was not. ` +
		template.HTMLEscapeString(r.message()) + `</div>`
}

// withDNSSuffix marks a status as DNS-backed without doubling the marker when
// the caller already supplied one (issue #187: "ASSIGNED:DNS" became
// "ASSIGNED:DNS:DNS").
func withDNSSuffix(status string) string {
	return strings.TrimSuffix(status, ":DNS") + ":DNS"
}

// wantsDNS reports whether DNS records were requested, either through the
// create_dns flag or through a status that already carries the ":DNS" marker.
func wantsDNS(createDNS bool, status string) bool {
	return createDNS || strings.HasSuffix(status, ":DNS")
}

// dnsZone returns the configured DNS zone (without trailing dot) from PDNS or DDWRT.
// Returns empty string if neither is configured.
func dnsZone(pdns *PDNSClient, ddwrt *DDWRTClient) string {
	if pdns != nil {
		return strings.TrimSuffix(pdns.Zone, ".")
	}
	if ddwrt != nil {
		return ddwrt.Zone
	}
	return ""
}

// buildFQDN returns the wildcard FQDN for a cluster given a DNS zone.
func buildFQDN(cluster, zone string) string {
	if cluster == "" || zone == "" {
		return ""
	}
	return fmt.Sprintf("*.%s.%s", cluster, zone)
}

// hostDigit returns the host part of ip within networkKey. ip may be the full
// address ("10.31.103.6") or the host part alone ("6"). An address from another
// network is an error: only the last octet used to be read, so "10.99.99.7"
// sent to 10.31.103 silently wrote digit 7 there (issue #201).
func hostDigit(ip, networkKey string) (string, error) {
	digit := ip
	if strings.Contains(ip, ".") {
		key, err := TruncateIP(ip)
		if err != nil {
			return "", err
		}
		if key != networkKey {
			return "", fmt.Errorf("ip %s is not in network %s", ip, networkKey)
		}
		digit = ip[strings.LastIndex(ip, ".")+1:]
	}

	// Pool keys are plain octets; "06" or "256" would create a digit no address has.
	if n, err := strconv.Atoi(digit); err != nil || n < 0 || n > 255 || strconv.Itoa(n) != digit {
		return "", fmt.Errorf("invalid host part %q in ip %q", digit, ip)
	}
	return digit, nil
}

// loadProfileHTTP loads the network config for an HTTP request. On failure it
// logs the error, writes a 500 response, and returns ok=false so the handler
// can return immediately. This keeps a missing/unreachable config from taking
// down the whole server — only the affected request fails.
func loadProfileHTTP(w http.ResponseWriter, loadFrom, configLoc, configNm string) (map[string]IPs, bool) {
	ipList, err := LoadProfile(loadFrom, configLoc, configNm)
	if err != nil {
		log.Printf("FAILED TO LOAD NETWORK CONFIG: %v", err)
		http.Error(w, "failed to load network config", http.StatusInternalServerError)
		return nil, false
	}
	return ipList, true
}

// StartWebServer starts the HTTP server for HTMX frontend and REST API
func StartWebServer(httpPort, loadFrom, configLoc, configNm string, pdns *PDNSClient, ddwrt *DDWRTClient) {
	mux := http.NewServeMux()

	// HTMX frontend routes
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		handleDashboard(w, r, loadFrom, configLoc, configNm)
	})
	mux.HandleFunc("GET /network/{key}", func(w http.ResponseWriter, r *http.Request) {
		handleNetworkDetail(w, r, loadFrom, configLoc, configNm)
	})

	// REST API routes
	mux.HandleFunc("GET /api/v1/networks", func(w http.ResponseWriter, r *http.Request) {
		handleAPINetworks(w, r, loadFrom, configLoc, configNm)
	})
	mux.HandleFunc("GET /api/v1/networks/{key}/ips", func(w http.ResponseWriter, r *http.Request) {
		handleAPINetworkIPs(w, r, loadFrom, configLoc, configNm, pdns, ddwrt)
	})
	mux.HandleFunc("POST /api/v1/networks/{key}/assign", func(w http.ResponseWriter, r *http.Request) {
		handleAPIAssign(w, r, loadFrom, configLoc, configNm, pdns, ddwrt)
	})
	mux.HandleFunc("POST /api/v1/networks/{key}/reserve", func(w http.ResponseWriter, r *http.Request) {
		handleAPIReserve(w, r, loadFrom, configLoc, configNm, pdns, ddwrt)
	})
	mux.HandleFunc("POST /api/v1/networks/{key}/release", func(w http.ResponseWriter, r *http.Request) {
		handleAPIRelease(w, r, loadFrom, configLoc, configNm, pdns, ddwrt)
	})
	mux.HandleFunc("POST /api/v1/networks/{key}/ips/{ip}/renew", func(w http.ResponseWriter, r *http.Request) {
		handleAPIRenewLease(w, r, loadFrom, configLoc, configNm)
	})

	// REST API CRUD routes
	mux.HandleFunc("POST /api/v1/networks", func(w http.ResponseWriter, r *http.Request) {
		handleAPICreateNetwork(w, r, loadFrom, configLoc, configNm)
	})
	mux.HandleFunc("POST /api/v1/networks/cidr", func(w http.ResponseWriter, r *http.Request) {
		handleAPICreateNetworkFromCIDR(w, r, loadFrom, configLoc, configNm)
	})
	mux.HandleFunc("DELETE /api/v1/networks/{key}", func(w http.ResponseWriter, r *http.Request) {
		handleAPIDeleteNetwork(w, r, loadFrom, configLoc, configNm)
	})
	mux.HandleFunc("POST /api/v1/networks/{key}/ips/add", func(w http.ResponseWriter, r *http.Request) {
		handleAPIAddIP(w, r, loadFrom, configLoc, configNm)
	})
	mux.HandleFunc("DELETE /api/v1/networks/{key}/ips/{ip}", func(w http.ResponseWriter, r *http.Request) {
		handleAPIDeleteIP(w, r, loadFrom, configLoc, configNm, pdns, ddwrt)
	})

	// Edit (update) existing assignment
	mux.HandleFunc("PUT /api/v1/networks/{key}/ips/{ip}", func(w http.ResponseWriter, r *http.Request) {
		handleAPIEditIP(w, r, loadFrom, configLoc, configNm, pdns, ddwrt)
	})

	// Cluster info routes
	mux.HandleFunc("GET /api/v1/clusters", func(w http.ResponseWriter, r *http.Request) {
		handleAPIClusters(w, r, loadFrom, configLoc, configNm)
	})
	mux.HandleFunc("GET /api/v1/clusters/{name}", func(w http.ResponseWriter, r *http.Request) {
		handleAPIClusterInfo(w, r, loadFrom, configLoc, configNm, pdns, ddwrt)
	})

	// Zone info endpoint
	mux.HandleFunc("GET /api/v1/zone", func(w http.ResponseWriter, r *http.Request) {
		handleAPIZone(w, r, pdns, ddwrt)
	})

	// HTMX partial routes
	mux.HandleFunc("POST /htmx/assign", func(w http.ResponseWriter, r *http.Request) {
		handleHTMXAssign(w, r, loadFrom, configLoc, configNm, pdns, ddwrt)
	})
	mux.HandleFunc("POST /htmx/release", func(w http.ResponseWriter, r *http.Request) {
		handleHTMXRelease(w, r, loadFrom, configLoc, configNm, pdns, ddwrt)
	})
	mux.HandleFunc("POST /htmx/add-network", func(w http.ResponseWriter, r *http.Request) {
		handleHTMXAddNetwork(w, r, loadFrom, configLoc, configNm)
	})
	mux.HandleFunc("POST /htmx/add-ip", func(w http.ResponseWriter, r *http.Request) {
		handleHTMXAddIP(w, r, loadFrom, configLoc, configNm)
	})
	mux.HandleFunc("POST /htmx/delete-ip", func(w http.ResponseWriter, r *http.Request) {
		handleHTMXDeleteIP(w, r, loadFrom, configLoc, configNm, pdns, ddwrt)
	})
	mux.HandleFunc("POST /htmx/edit", func(w http.ResponseWriter, r *http.Request) {
		handleHTMXEdit(w, r, loadFrom, configLoc, configNm, pdns, ddwrt)
	})
	mux.HandleFunc("POST /htmx/delete-network", func(w http.ResponseWriter, r *http.Request) {
		handleHTMXDeleteNetwork(w, r, loadFrom, configLoc, configNm)
	})
	mux.HandleFunc("POST /htmx/test-dns", func(w http.ResponseWriter, r *http.Request) {
		handleHTMXTestDNS(w, r, pdns, ddwrt)
	})

	log.Printf("HTTP/HTMX SERVER LISTENING AT :%s", httpPort)
	if err := http.ListenAndServe(":"+httpPort, mux); err != nil {
		log.Fatalf("FAILED TO START HTTP SERVER: %v", err)
	}
}

// getPoolInfos returns summary info for all network pools
func getPoolInfos(ipList map[string]IPs) []NetworkPoolInfo {
	var pools []NetworkPoolInfo
	for key, ips := range ipList {
		info := NetworkPoolInfo{NetworkKey: key, Total: len(ips)}
		for _, ipInfo := range ips {
			switch {
			case strings.HasPrefix(ipInfo.Status, "ASSIGNED"):
				info.Assigned++
			case strings.HasPrefix(ipInfo.Status, "PENDING"):
				info.Pending++
			case isFree(ipInfo):
				// Must match what the allocators hand out (issue #196), so an
				// unrecognised status is counted in Total but never as Available.
				info.Available++
			}
		}
		pools = append(pools, info)
	}
	sort.Slice(pools, func(i, j int) bool {
		return pools[i].NetworkKey < pools[j].NetworkKey
	})
	return pools
}

// getIPEntries returns sorted IP entries for a network key
func getIPEntries(ips IPs, networkKey string) []IPEntry {
	var entries []IPEntry
	for digit, info := range ips {
		entries = append(entries, IPEntry{
			IP:             networkKey + "." + digit,
			Digit:          digit,
			Status:         info.Status,
			Cluster:        info.Cluster,
			LeaseExpiresAt: info.LeaseExpiresAt,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		a, _ := strconv.Atoi(entries[i].Digit)
		b, _ := strconv.Atoi(entries[j].Digit)
		return a < b
	})
	return entries
}

// writeJSON writes a 200 JSON response body with the correct content type.
func writeJSON(w http.ResponseWriter, body any) {
	writeJSONStatus(w, http.StatusOK, body)
}

// writeJSONStatus writes a JSON response body under an explicit status code.
func writeJSONStatus(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("encode response: %v", err)
	}
}

// renderIPTable re-renders the IP table for an HTMX swap, prefixed with a
// warning banner when the DNS half of the operation failed — the UI counterpart
// of the "dns" field in the REST responses.
func renderIPTable(w http.ResponseWriter, networkKey string, entries []IPEntry, dns dnsResult) {
	if banner := dns.banner(); banner != "" {
		if _, err := io.WriteString(w, banner); err != nil {
			log.Printf("write dns banner: %v", err)
			return
		}
	}

	tmpl := template.Must(template.New("table").Funcs(TemplateFuncs()).Parse(ipTablePartial))
	if err := tmpl.Execute(w, struct {
		NetworkKey string
		Entries    []IPEntry
	}{networkKey, entries}); err != nil {
		log.Printf("render ip table: %v", err)
	}
}

// --- HTMX Frontend Handlers ---

type dashboardData struct {
	Pools     []NetworkPoolInfo
	Version   string
	Commit    string
	StartTime string
}

func handleDashboard(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string) {
	ipList, ok := loadProfileHTTP(w, loadFrom, configLoc, configNm)
	if !ok {
		return
	}
	pools := getPoolInfos(ipList)

	data := dashboardData{
		Pools:     pools,
		Version:   version,
		Commit:    commit,
		StartTime: date,
	}

	tmpl := template.Must(template.New("dashboard").Funcs(TemplateFuncs()).Parse(dashboardTemplate))
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleNetworkDetail(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string) {
	networkKey := r.PathValue("key")
	ipList, ok := loadProfileHTTP(w, loadFrom, configLoc, configNm)
	if !ok {
		return
	}

	ips, ok := ipList[networkKey]
	if !ok {
		http.Error(w, "Network not found", http.StatusNotFound)
		return
	}

	entries := getIPEntries(ips, networkKey)
	pools := getPoolInfos(ipList)

	data := struct {
		NetworkKey string
		Entries    []IPEntry
		Pools      []NetworkPoolInfo
	}{networkKey, entries, pools}

	tmpl := template.Must(template.New("network").Funcs(TemplateFuncs()).Parse(networkDetailTemplate))
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleHTMXAssign(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string, pdns *PDNSClient, ddwrt *DDWRTClient) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ip := r.FormValue("ip")
	cluster := r.FormValue("cluster")
	status := r.FormValue("status")
	networkKey := r.FormValue("network_key")
	createDNS := r.FormValue("create_dns") == "on"

	if ip == "" || cluster == "" || status == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	tx := BeginLedgerWrite(loadFrom, configLoc, configNm)
	defer tx.End()

	ipList, ok := loadForWriteHTTP(w, tx)
	if !ok {
		return
	}

	ipKey, err := TruncateIP(ip)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ipDigit, err := GetLastIPDigit(ip)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	createDNS = wantsDNS(createDNS, status)

	entry := ipList[ipKey][ipDigit]
	entry.Status = status
	if createDNS {
		entry.Status = withDNSSuffix(status)
	}
	entry.Cluster = cluster
	ipList[ipKey][ipDigit] = entry

	if !saveConfigHTTP(w, tx, ipList) {
		return
	}

	var dns dnsResult
	if createDNS {
		dns.create(pdns, ddwrt, cluster, ipKey+"."+ipDigit)
	}

	// Re-render the network detail table
	ips := ipList[networkKey]
	entries := getIPEntries(ips, networkKey)
	renderIPTable(w, networkKey, entries, dns)
}

func handleHTMXRelease(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string, pdns *PDNSClient, ddwrt *DDWRTClient) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ip := r.FormValue("ip")
	networkKey := r.FormValue("network_key")

	tx := BeginLedgerWrite(loadFrom, configLoc, configNm)
	defer tx.End()

	ipList, ok := loadForWriteHTTP(w, tx)
	if !ok {
		return
	}

	ipKey, err := TruncateIP(ip)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ipDigit, err := GetLastIPDigit(ip)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	entry := ipList[ipKey][ipDigit]
	prevCluster := entry.Cluster
	hadDNS := strings.HasSuffix(entry.Status, ":DNS")
	entry.Status = ""
	entry.Cluster = ""
	ipList[ipKey][ipDigit] = entry

	if !saveConfigHTTP(w, tx, ipList) {
		return
	}

	var dns dnsResult
	if hadDNS {
		dns.remove(pdns, ddwrt, prevCluster)
	}

	// Re-render the network detail table
	ips := ipList[networkKey]
	entries := getIPEntries(ips, networkKey)
	renderIPTable(w, networkKey, entries, dns)
}

// --- REST API Handlers ---

func handleAPINetworks(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string) {
	ipList, ok := loadProfileHTTP(w, loadFrom, configLoc, configNm)
	if !ok {
		return
	}
	pools := getPoolInfos(ipList)

	writeJSON(w, pools)
}

func handleAPINetworkIPs(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string, pdns *PDNSClient, ddwrt *DDWRTClient) {
	networkKey := r.PathValue("key")
	ipList, ok := loadProfileHTTP(w, loadFrom, configLoc, configNm)
	if !ok {
		return
	}

	ips, ok := ipList[networkKey]
	if !ok {
		http.Error(w, `{"error":"network not found"}`, http.StatusNotFound)
		return
	}

	zone := dnsZone(pdns, ddwrt)
	entries := getIPEntries(ips, networkKey)
	for i := range entries {
		if strings.Contains(entries[i].Status, ":DNS") && entries[i].Cluster != "" {
			entries[i].FQDN = buildFQDN(entries[i].Cluster, zone)
		}
	}

	writeJSON(w, entries)
}

func handleAPIAssign(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string, pdns *PDNSClient, ddwrt *DDWRTClient) {
	networkKey := r.PathValue("key")

	var req struct {
		IP                   string `json:"ip"`
		Cluster              string `json:"cluster"`
		Status               string `json:"status"`
		CreateDNS            bool   `json:"create_dns"`
		CreateDNSAlt         bool   `json:"createDNS"`
		LeaseDurationSeconds int64  `json:"lease_duration_seconds"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.CreateDNSAlt {
		req.CreateDNS = true
	}

	if req.IP == "" || req.Cluster == "" {
		http.Error(w, `{"error":"ip and cluster are required"}`, http.StatusBadRequest)
		return
	}

	if req.Status == "" {
		req.Status = "ASSIGNED"
	}

	tx := BeginLedgerWrite(loadFrom, configLoc, configNm)
	defer tx.End()

	ipList, ok := loadForWriteHTTP(w, tx)
	if !ok {
		return
	}

	if _, ok := ipList[networkKey]; !ok {
		http.Error(w, `{"error":"network not found"}`, http.StatusNotFound)
		return
	}

	ipDigit, err := hostDigit(req.IP, networkKey)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	req.CreateDNS = wantsDNS(req.CreateDNS, req.Status)

	// assign overwrites by design (the Crossplane provider relies on it), but
	// never silently: the previous holder is reported and its record withdrawn.
	// Claiming an address only if it is free is reserve with "ip" (issue #201).
	entry, inPool := ipList[networkKey][ipDigit]
	previous := entry
	if !isFree(previous) {
		log.Printf("ASSIGN OVERWRITES %s.%s: was %s/%s, now %s", networkKey, ipDigit, previous.Cluster, previous.Status, req.Cluster)
	}

	entry.Status = req.Status
	if req.CreateDNS {
		entry.Status = withDNSSuffix(req.Status)
	}
	entry.Cluster = req.Cluster
	if req.LeaseDurationSeconds > 0 {
		entry.LeaseExpiresAt = time.Now().Unix() + req.LeaseDurationSeconds
	} else {
		entry.LeaseExpiresAt = 0
	}
	ipList[networkKey][ipDigit] = entry

	if !saveConfigHTTP(w, tx, ipList) {
		return
	}

	// Same rule as edit: withdraw the old record when DNS is turned off or the
	// address changes hands, otherwise it keeps resolving to an address that is
	// no longer that cluster's.
	var dns dnsResult
	hadDNS := strings.HasSuffix(previous.Status, ":DNS") && previous.Cluster != ""
	if hadDNS && (!req.CreateDNS || previous.Cluster != req.Cluster) {
		dns.remove(pdns, ddwrt, previous.Cluster)
	}
	if req.CreateDNS {
		dns.create(pdns, ddwrt, req.Cluster, networkKey+"."+ipDigit)
	}

	resp := map[string]any{
		"status":  "ok",
		"message": fmt.Sprintf("IP %s assigned to cluster %s", networkKey+"."+ipDigit, req.Cluster),
	}
	if !isFree(previous) {
		resp["previous_cluster"] = previous.Cluster
		resp["previous_status"] = previous.Status
	}
	if !inPool {
		resp["added_to_pool"] = true
	}
	writeJSON(w, dns.annotate(resp))
}

// handleAPIReserve finds an available IP in the network, assigns it to the cluster, and returns the full IP.
func handleAPIReserve(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string, pdns *PDNSClient, ddwrt *DDWRTClient) {
	networkKey := r.PathValue("key")

	var req struct {
		Cluster              string `json:"cluster"`
		Status               string `json:"status"`
		CreateDNS            bool   `json:"create_dns"`
		CreateDNSAlt         bool   `json:"createDNS"`
		LeaseDurationSeconds int64  `json:"lease_duration_seconds"`
		// IP asks for one specific address instead of any free one (issue #201).
		// Unlike assign it never overwrites: a taken address answers 409.
		IP string `json:"ip"`
		// AddToPool lets IP name a host part the pool does not list yet.
		AddToPool bool `json:"add_to_pool"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.CreateDNSAlt {
		req.CreateDNS = true
	}

	if req.Cluster == "" {
		http.Error(w, `{"error":"cluster is required"}`, http.StatusBadRequest)
		return
	}

	if req.Status == "" {
		req.Status = "ASSIGNED"
	}

	tx := BeginLedgerWrite(loadFrom, configLoc, configNm)
	defer tx.End()

	ipList, ok := loadForWriteHTTP(w, tx)
	if !ok {
		return
	}

	networkIPs, ok := ipList[networkKey]
	if !ok {
		http.Error(w, `{"error":"network not found"}`, http.StatusNotFound)
		return
	}

	var foundDigit string
	addedToPool := false

	if req.IP != "" {
		// A specific address: it must belong here, exist (or be added on
		// request) and be free — checked inside the ledger write, so the answer
		// cannot go stale before the save.
		digit, err := hostDigit(req.IP, networkKey)
		if err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}

		current, inPool := networkIPs[digit]
		switch {
		case !inPool && !req.AddToPool:
			writeJSONStatus(w, http.StatusNotFound, map[string]any{
				"error": fmt.Sprintf("ip %s.%s is not in the pool; set add_to_pool to add it", networkKey, digit),
			})
			return
		case inPool && !isFree(current):
			writeJSONStatus(w, http.StatusConflict, map[string]any{
				"error":   fmt.Sprintf("ip %s.%s is not free", networkKey, digit),
				"ip":      networkKey + "." + digit,
				"cluster": current.Cluster,
				"status":  current.Status,
			})
			return
		}
		foundDigit = digit
		addedToPool = !inPool
	} else {
		// Find an available IP
		for digit, info := range networkIPs {
			if isFree(info) {
				foundDigit = digit
				break
			}
		}

		if foundDigit == "" {
			http.Error(w, `{"error":"no available IPs in network"}`, http.StatusConflict)
			return
		}
	}

	fullIP := networkKey + "." + foundDigit

	// Assign the IP
	req.CreateDNS = wantsDNS(req.CreateDNS, req.Status)

	entry := networkIPs[foundDigit]
	entry.Status = req.Status
	if req.CreateDNS {
		entry.Status = withDNSSuffix(req.Status)
	}
	entry.Cluster = req.Cluster
	if req.LeaseDurationSeconds > 0 {
		entry.LeaseExpiresAt = time.Now().Unix() + req.LeaseDurationSeconds
	} else {
		entry.LeaseExpiresAt = 0
	}
	ipList[networkKey][foundDigit] = entry

	if !saveConfigHTTP(w, tx, ipList) {
		return
	}

	var dns dnsResult
	if req.CreateDNS {
		dns.create(pdns, ddwrt, req.Cluster, fullIP)
	}

	resp := map[string]any{
		"ip":      fullIP,
		"ips":     []string{fullIP},
		"digit":   foundDigit,
		"status":  entry.Status,
		"cluster": req.Cluster,
	}
	if addedToPool {
		resp["added_to_pool"] = true
	}
	writeJSON(w, dns.annotate(resp))
}

func handleAPIRelease(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string, pdns *PDNSClient, ddwrt *DDWRTClient) {
	networkKey := r.PathValue("key")

	var req struct {
		IP string `json:"ip"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	tx := BeginLedgerWrite(loadFrom, configLoc, configNm)
	defer tx.End()

	ipList, ok := loadForWriteHTTP(w, tx)
	if !ok {
		return
	}

	if _, ok := ipList[networkKey]; !ok {
		http.Error(w, `{"error":"network not found"}`, http.StatusNotFound)
		return
	}

	ipDigit, err := GetLastIPDigit(req.IP)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	entry := ipList[networkKey][ipDigit]
	prevCluster := entry.Cluster
	hadDNS := strings.HasSuffix(entry.Status, ":DNS")
	entry.Status = ""
	entry.Cluster = ""
	entry.LeaseExpiresAt = 0
	ipList[networkKey][ipDigit] = entry

	if !saveConfigHTTP(w, tx, ipList) {
		return
	}

	var dns dnsResult
	if hadDNS {
		dns.remove(pdns, ddwrt, prevCluster)
	}

	writeJSON(w, dns.annotate(map[string]any{
		"status":  "ok",
		"message": fmt.Sprintf("IP %s released", req.IP),
	}))
}

func handleAPIRenewLease(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string) {
	networkKey := r.PathValue("key")
	ip := r.PathValue("ip")

	var req struct {
		LeaseDurationSeconds int64 `json:"lease_duration_seconds"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.LeaseDurationSeconds <= 0 {
		http.Error(w, `{"error":"lease_duration_seconds must be > 0"}`, http.StatusBadRequest)
		return
	}

	tx := BeginLedgerWrite(loadFrom, configLoc, configNm)
	defer tx.End()

	ipList, ok := loadForWriteHTTP(w, tx)
	if !ok {
		return
	}

	if _, ok := ipList[networkKey]; !ok {
		http.Error(w, `{"error":"network not found"}`, http.StatusNotFound)
		return
	}

	ipDigit, err := GetLastIPDigit(ip)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	entry, ok := ipList[networkKey][ipDigit]
	if !ok {
		http.Error(w, `{"error":"ip not found"}`, http.StatusNotFound)
		return
	}
	if entry.Status == "" {
		http.Error(w, `{"error":"ip is not assigned"}`, http.StatusConflict)
		return
	}

	entry.LeaseExpiresAt = time.Now().Unix() + req.LeaseDurationSeconds
	ipList[networkKey][ipDigit] = entry

	if !saveConfigHTTP(w, tx, ipList) {
		return
	}

	writeJSON(w, map[string]any{
		"status":           "ok",
		"ip":               ip,
		"lease_expires_at": entry.LeaseExpiresAt,
	})
}

// --- REST API CRUD Handlers ---

func handleAPICreateNetwork(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string) {
	var req struct {
		Network  string   `json:"network"`
		IPs      []string `json:"ips"`
		CIDR     string   `json:"cidr"`
		Reserved []string `json:"reserved"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	tx := BeginLedgerWrite(loadFrom, configLoc, configNm)
	defer tx.End()

	ipList, ok := loadForWriteHTTP(w, tx)
	if !ok {
		return
	}

	// CIDR mode: expand CIDR notation into networks
	if req.CIDR != "" {
		networks, err := CIDRToNetworks(req.CIDR, req.Reserved)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		createdKeys := []string{}
		totalIPs := 0

		for networkKey, octets := range networks {
			if _, exists := ipList[networkKey]; exists {
				http.Error(w, fmt.Sprintf(`{"error":"network %s already exists"}`, networkKey), http.StatusConflict)
				return
			}

			ipList[networkKey] = make(IPs)
			for _, octet := range octets {
				ipList[networkKey][octet] = IPInfo{}
			}

			createdKeys = append(createdKeys, networkKey)
			totalIPs += len(octets)
		}

		if !saveConfigHTTP(w, tx, ipList) {
			return
		}

		writeJSONStatus(w, http.StatusCreated, map[string]any{
			"status":   "ok",
			"message":  fmt.Sprintf("Created %d network(s) with %d IPs from CIDR %s", len(createdKeys), totalIPs, req.CIDR),
			"networks": createdKeys,
		})
		return
	}

	// Flat list mode: existing behavior
	if req.Network == "" || len(req.IPs) == 0 {
		http.Error(w, `{"error":"network and ips are required, or provide cidr"}`, http.StatusBadRequest)
		return
	}

	if _, exists := ipList[req.Network]; exists {
		http.Error(w, `{"error":"network already exists"}`, http.StatusConflict)
		return
	}

	ipList[req.Network] = make(IPs)
	for _, ip := range req.IPs {
		ipList[req.Network][ip] = IPInfo{}
	}

	if !saveConfigHTTP(w, tx, ipList) {
		return
	}

	writeJSONStatus(w, http.StatusCreated, map[string]any{
		"status":  "ok",
		"message": fmt.Sprintf("Network %s created with %d IPs", req.Network, len(req.IPs)),
	})
}

func handleAPICreateNetworkFromCIDR(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string) {
	var req struct {
		CIDR     string   `json:"cidr"`
		Reserved []string `json:"reserved"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.CIDR == "" {
		http.Error(w, `{"error":"cidr is required"}`, http.StatusBadRequest)
		return
	}

	networks, err := CIDRToNetworks(req.CIDR, req.Reserved)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	tx := BeginLedgerWrite(loadFrom, configLoc, configNm)
	defer tx.End()

	ipList, ok := loadForWriteHTTP(w, tx)
	if !ok {
		return
	}

	createdKeys := []string{}
	totalIPs := 0

	for networkKey, octets := range networks {
		if _, exists := ipList[networkKey]; exists {
			http.Error(w, fmt.Sprintf(`{"error":"network %s already exists"}`, networkKey), http.StatusConflict)
			return
		}

		ipList[networkKey] = make(IPs)
		for _, octet := range octets {
			ipList[networkKey][octet] = IPInfo{}
		}

		createdKeys = append(createdKeys, networkKey)
		totalIPs += len(octets)
	}

	if !saveConfigHTTP(w, tx, ipList) {
		return
	}

	writeJSONStatus(w, http.StatusCreated, map[string]any{
		"status":   "ok",
		"message":  fmt.Sprintf("Created %d network(s) with %d IPs from CIDR %s", len(createdKeys), totalIPs, req.CIDR),
		"networks": createdKeys,
	})
}

func handleAPIDeleteNetwork(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string) {
	networkKey := r.PathValue("key")
	tx := BeginLedgerWrite(loadFrom, configLoc, configNm)
	defer tx.End()

	ipList, ok := loadForWriteHTTP(w, tx)
	if !ok {
		return
	}

	if _, ok := ipList[networkKey]; !ok {
		http.Error(w, `{"error":"network not found"}`, http.StatusNotFound)
		return
	}

	delete(ipList, networkKey)
	if !saveConfigHTTP(w, tx, ipList) {
		return
	}

	writeJSON(w, map[string]any{
		"status":  "ok",
		"message": fmt.Sprintf("Network %s deleted", networkKey),
	})
}

func handleAPIAddIP(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string) {
	networkKey := r.PathValue("key")

	var req struct {
		IPs []string `json:"ips"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if len(req.IPs) == 0 {
		http.Error(w, `{"error":"ips array is required"}`, http.StatusBadRequest)
		return
	}

	tx := BeginLedgerWrite(loadFrom, configLoc, configNm)
	defer tx.End()

	ipList, ok := loadForWriteHTTP(w, tx)
	if !ok {
		return
	}

	if _, ok := ipList[networkKey]; !ok {
		http.Error(w, `{"error":"network not found"}`, http.StatusNotFound)
		return
	}

	added := 0
	for _, ip := range req.IPs {
		if _, exists := ipList[networkKey][ip]; !exists {
			ipList[networkKey][ip] = IPInfo{}
			added++
		}
	}

	if !saveConfigHTTP(w, tx, ipList) {
		return
	}

	writeJSONStatus(w, http.StatusCreated, map[string]any{
		"status":  "ok",
		"message": fmt.Sprintf("Added %d IPs to network %s", added, networkKey),
	})
}

func handleAPIDeleteIP(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string, pdns *PDNSClient, ddwrt *DDWRTClient) {
	networkKey := r.PathValue("key")
	ip := r.PathValue("ip")
	// Support both full IP (10.31.104.5) and host-part only (5)
	if strings.HasPrefix(ip, networkKey+".") {
		ip = strings.TrimPrefix(ip, networkKey+".")
	}

	tx := BeginLedgerWrite(loadFrom, configLoc, configNm)
	defer tx.End()

	ipList, ok := loadForWriteHTTP(w, tx)
	if !ok {
		return
	}

	network, ok := ipList[networkKey]
	if !ok {
		http.Error(w, `{"error":"network not found"}`, http.StatusNotFound)
		return
	}

	entry, exists := network[ip]
	if !exists {
		http.Error(w, `{"error":"ip not found"}`, http.StatusNotFound)
		return
	}

	// Delete DNS record if one was created for this IP
	hadDNS := strings.HasSuffix(entry.Status, ":DNS")
	prevCluster := entry.Cluster

	delete(ipList[networkKey], ip)
	if !saveConfigHTTP(w, tx, ipList) {
		return
	}

	var dns dnsResult
	if hadDNS {
		dns.remove(pdns, ddwrt, prevCluster)
	}

	writeJSON(w, dns.annotate(map[string]any{
		"status":  "ok",
		"message": fmt.Sprintf("IP %s.%s deleted", networkKey, ip),
	}))
}

func handleAPIEditIP(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string, pdns *PDNSClient, ddwrt *DDWRTClient) {
	networkKey := r.PathValue("key")
	ipDigit := r.PathValue("ip")
	// Support both full IP (10.31.104.5) and host-part only (5)
	if strings.HasPrefix(ipDigit, networkKey+".") {
		ipDigit = strings.TrimPrefix(ipDigit, networkKey+".")
	}

	var req struct {
		Cluster      string `json:"cluster"`
		Status       string `json:"status"`
		CreateDNS    bool   `json:"create_dns"`
		CreateDNSAlt bool   `json:"createDNS"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Accept both "create_dns" and "createDNS" field names
	if req.CreateDNSAlt {
		req.CreateDNS = true
	}

	if req.Cluster == "" || req.Status == "" {
		http.Error(w, `{"error":"cluster and status are required"}`, http.StatusBadRequest)
		return
	}

	tx := BeginLedgerWrite(loadFrom, configLoc, configNm)
	defer tx.End()

	ipList, ok := loadForWriteHTTP(w, tx)
	if !ok {
		return
	}

	network, ok := ipList[networkKey]
	if !ok {
		http.Error(w, `{"error":"network not found"}`, http.StatusNotFound)
		return
	}

	entry, exists := network[ipDigit]
	if !exists {
		http.Error(w, `{"error":"ip not found"}`, http.StatusNotFound)
		return
	}

	prevCluster := entry.Cluster
	hadDNS := strings.HasSuffix(entry.Status, ":DNS")

	req.CreateDNS = wantsDNS(req.CreateDNS, req.Status)

	baseStatus := strings.TrimSuffix(req.Status, ":DNS")
	entry.Status = baseStatus
	if req.CreateDNS {
		entry.Status = withDNSSuffix(baseStatus)
	}
	entry.Cluster = req.Cluster
	ipList[networkKey][ipDigit] = entry

	if !saveConfigHTTP(w, tx, ipList) {
		return
	}

	// Handle DNS changes. Remove the old record when DNS is turned off or the
	// cluster changed. When DNS is on, always (re)create — both providers are
	// idempotent, so re-saving reconciles a record that drifted from the router.
	var dns dnsResult
	if hadDNS && (!req.CreateDNS || prevCluster != req.Cluster) {
		dns.remove(pdns, ddwrt, prevCluster)
	}
	if req.CreateDNS {
		dns.create(pdns, ddwrt, req.Cluster, networkKey+"."+ipDigit)
	}

	writeJSON(w, dns.annotate(map[string]any{
		"status":  "ok",
		"message": fmt.Sprintf("IP %s.%s updated: cluster=%s status=%s", networkKey, ipDigit, req.Cluster, entry.Status),
	}))
}

// --- Cluster Info Handlers ---

func handleAPIClusters(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string) {
	ipList, ok := loadProfileHTTP(w, loadFrom, configLoc, configNm)
	if !ok {
		return
	}

	clusters := map[string][]map[string]string{}
	for networkKey, ips := range ipList {
		for ipDigit, entry := range ips {
			if entry.Cluster != "" {
				clusters[entry.Cluster] = append(clusters[entry.Cluster], map[string]string{
					"network": networkKey,
					"ip":      networkKey + "." + ipDigit,
					"status":  entry.Status,
				})
			}
		}
	}

	type clusterSummary struct {
		Cluster string `json:"cluster"`
		IPCount int    `json:"ip_count"`
	}

	result := []clusterSummary{}
	for name, ips := range clusters {
		result = append(result, clusterSummary{Cluster: name, IPCount: len(ips)})
	}

	writeJSON(w, result)
}

func handleAPIClusterInfo(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string, pdns *PDNSClient, ddwrt *DDWRTClient) {
	clusterName := r.PathValue("name")
	ipList, ok := loadProfileHTTP(w, loadFrom, configLoc, configNm)
	if !ok {
		return
	}

	type ipInfo struct {
		Network string `json:"network"`
		IP      string `json:"ip"`
		Status  string `json:"status"`
	}

	var ips []ipInfo
	hasDNS := false
	for networkKey, network := range ipList {
		for ipDigit, entry := range network {
			if entry.Cluster == clusterName {
				ips = append(ips, ipInfo{
					Network: networkKey,
					IP:      networkKey + "." + ipDigit,
					Status:  entry.Status,
				})
				if strings.Contains(entry.Status, ":DNS") {
					hasDNS = true
				}
			}
		}
	}

	if len(ips) == 0 {
		http.Error(w, `{"error":"cluster not found"}`, http.StatusNotFound)
		return
	}

	zone := dnsZone(pdns, ddwrt)
	result := map[string]interface{}{
		"cluster": clusterName,
		"ips":     ips,
	}
	if hasDNS && zone != "" {
		result["fqdn"] = buildFQDN(clusterName, zone)
		result["zone"] = zone
	}

	writeJSON(w, result)
}

func handleAPIZone(w http.ResponseWriter, r *http.Request, pdns *PDNSClient, ddwrt *DDWRTClient) {
	type providerInfo struct {
		Enabled bool   `json:"enabled"`
		Zone    string `json:"zone,omitempty"`
	}

	result := map[string]providerInfo{
		"pdns": {
			Enabled: pdns != nil,
		},
		"ddwrt": {
			Enabled: ddwrt != nil,
		},
	}

	if pdns != nil {
		result["pdns"] = providerInfo{
			Enabled: true,
			Zone:    strings.TrimSuffix(pdns.Zone, "."),
		}
	}
	if ddwrt != nil {
		result["ddwrt"] = providerInfo{
			Enabled: true,
			Zone:    ddwrt.Zone,
		}
	}

	writeJSON(w, result)
}

// --- HTMX CRUD Handlers ---

func handleHTMXAddNetwork(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	network := r.FormValue("network")
	ipFrom := r.FormValue("ip_from")
	ipTo := r.FormValue("ip_to")

	if network == "" || ipFrom == "" || ipTo == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	from, err := strconv.Atoi(ipFrom)
	if err != nil {
		http.Error(w, "Invalid IP range start", http.StatusBadRequest)
		return
	}
	to, err := strconv.Atoi(ipTo)
	if err != nil {
		http.Error(w, "Invalid IP range end", http.StatusBadRequest)
		return
	}

	tx := BeginLedgerWrite(loadFrom, configLoc, configNm)
	defer tx.End()

	ipList, ok := loadForWriteHTTP(w, tx)
	if !ok {
		return
	}

	if _, exists := ipList[network]; exists {
		http.Error(w, "Network already exists", http.StatusConflict)
		return
	}

	ipList[network] = make(IPs)
	for i := from; i <= to; i++ {
		ipList[network][strconv.Itoa(i)] = IPInfo{}
	}

	if !saveConfigHTTP(w, tx, ipList) {
		return
	}

	// Redirect to the new network's detail page
	w.Header().Set("HX-Redirect", "/network/"+network)
	w.WriteHeader(http.StatusOK)
}

func handleHTMXAddIP(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	networkKey := r.FormValue("network_key")
	ip := r.FormValue("ip")

	if networkKey == "" || ip == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	tx := BeginLedgerWrite(loadFrom, configLoc, configNm)
	defer tx.End()

	ipList, ok := loadForWriteHTTP(w, tx)
	if !ok {
		return
	}

	if _, ok := ipList[networkKey]; !ok {
		http.Error(w, "Network not found", http.StatusNotFound)
		return
	}

	if _, exists := ipList[networkKey][ip]; exists {
		http.Error(w, "IP already exists", http.StatusConflict)
		return
	}

	ipList[networkKey][ip] = IPInfo{}
	if !saveConfigHTTP(w, tx, ipList) {
		return
	}

	// Re-render the IP table
	entries := getIPEntries(ipList[networkKey], networkKey)
	renderIPTable(w, networkKey, entries, dnsResult{})
}

func handleHTMXDeleteIP(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string, pdns *PDNSClient, ddwrt *DDWRTClient) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	networkKey := r.FormValue("network_key")
	ip := r.FormValue("ip")

	if networkKey == "" || ip == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	tx := BeginLedgerWrite(loadFrom, configLoc, configNm)
	defer tx.End()

	ipList, ok := loadForWriteHTTP(w, tx)
	if !ok {
		return
	}

	ipDigit, err := GetLastIPDigit(ip)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Delete DNS record if one was created for this IP
	entry := ipList[networkKey][ipDigit]
	hadDNS := strings.HasSuffix(entry.Status, ":DNS")
	prevCluster := entry.Cluster

	delete(ipList[networkKey], ipDigit)
	if !saveConfigHTTP(w, tx, ipList) {
		return
	}

	var dns dnsResult
	if hadDNS {
		dns.remove(pdns, ddwrt, prevCluster)
	}

	// Re-render the IP table
	entries := getIPEntries(ipList[networkKey], networkKey)
	renderIPTable(w, networkKey, entries, dns)
}

func handleHTMXEdit(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string, pdns *PDNSClient, ddwrt *DDWRTClient) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ip := r.FormValue("ip")
	cluster := r.FormValue("cluster")
	status := r.FormValue("status")
	networkKey := r.FormValue("network_key")
	createDNS := r.FormValue("create_dns") == "on"

	if ip == "" || cluster == "" || status == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	tx := BeginLedgerWrite(loadFrom, configLoc, configNm)
	defer tx.End()

	ipList, ok := loadForWriteHTTP(w, tx)
	if !ok {
		return
	}

	ipKey, err := TruncateIP(ip)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ipDigit, err := GetLastIPDigit(ip)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	entry := ipList[ipKey][ipDigit]
	prevCluster := entry.Cluster
	hadDNS := strings.HasSuffix(entry.Status, ":DNS")

	// Update entry
	createDNS = wantsDNS(createDNS, status)

	entry.Status = status
	if createDNS {
		entry.Status = withDNSSuffix(status)
	}
	entry.Cluster = cluster
	ipList[ipKey][ipDigit] = entry

	if !saveConfigHTTP(w, tx, ipList) {
		return
	}

	// Handle DNS changes. Remove the old record when DNS is turned off or the
	// cluster changed. When DNS is on, always (re)create — both providers are
	// idempotent, so re-saving reconciles a record that drifted from the router.
	var dns dnsResult
	if hadDNS && (!createDNS || prevCluster != cluster) {
		dns.remove(pdns, ddwrt, prevCluster)
	}
	if createDNS {
		dns.create(pdns, ddwrt, cluster, ipKey+"."+ipDigit)
	}

	// Re-render the network detail table
	ips := ipList[networkKey]
	entries := getIPEntries(ips, networkKey)
	renderIPTable(w, networkKey, entries, dns)
}

func handleHTMXTestDNS(w http.ResponseWriter, r *http.Request, pdns *PDNSClient, ddwrt *DDWRTClient) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cluster := r.FormValue("cluster")
	expectedIP := r.FormValue("expected_ip")

	if cluster == "" || expectedIP == "" {
		fmt.Fprintf(w, `<span style="color:#f97316;font-size:0.75rem;">missing params</span>`)
		return
	}

	// Test against whichever DNS provider is enabled. PDNS takes precedence;
	// fall back to DD-WRT so DD-WRT-only deployments can verify records too.
	var (
		fqdn, resolved string
		match          bool
		err            error
	)
	switch {
	case pdns != nil:
		fqdn, resolved, match, err = pdns.TestDNS(cluster, expectedIP)
	case ddwrt != nil:
		fqdn, resolved, match, err = ddwrt.TestDNS(cluster, expectedIP)
	default:
		fmt.Fprintf(w, `<span style="color:#f97316;font-size:0.75rem;">no DNS provider enabled</span>`)
		return
	}

	if err != nil {
		fmt.Fprintf(w, `<span style="color:#ef4444;font-size:0.75rem;" title="%s">DNS FAIL</span>`, err.Error())
		return
	}

	if match {
		fmt.Fprintf(w, `<span style="color:#4ade80;font-size:0.75rem;">%s → %s</span>`, fqdn, resolved)
	} else {
		fmt.Fprintf(w, `<span style="color:#ef4444;font-size:0.75rem;">%s → %s (expected %s)</span>`, fqdn, resolved, expectedIP)
	}
}

func handleHTMXDeleteNetwork(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	networkKey := r.FormValue("network_key")

	tx := BeginLedgerWrite(loadFrom, configLoc, configNm)
	defer tx.End()

	ipList, ok := loadForWriteHTTP(w, tx)
	if !ok {
		return
	}
	delete(ipList, networkKey)
	if !saveConfigHTTP(w, tx, ipList) {
		return
	}

	// Redirect to dashboard
	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}

// loadForWriteHTTP loads the ledger inside a write, the write-side twin of
// loadProfileHTTP.
func loadForWriteHTTP(w http.ResponseWriter, tx *LedgerWrite) (map[string]IPs, bool) {
	ipList, err := tx.Load()
	if err != nil {
		log.Printf("FAILED TO LOAD NETWORK CONFIG: %v", err)
		http.Error(w, "failed to load network config", http.StatusInternalServerError)
		return nil, false
	}
	return ipList, true
}

// saveConfigHTTP persists the IP list for an HTTP request. On failure it logs,
// writes the error and returns false; the handler must return immediately —
// before any DNS call, so a record is never created or removed for a change the
// ledger does not hold (issue #200). A concurrent change answers 409 so the
// caller can retry (issue #199).
func saveConfigHTTP(w http.ResponseWriter, tx *LedgerWrite, ipList map[string]IPs) bool {
	err := tx.Save(ipList)
	switch {
	case err == nil:
		return true
	case errors.Is(err, ErrLedgerConflict):
		log.Printf("NETWORK CONFIG CHANGED CONCURRENTLY: %v", err)
		http.Error(w, "network config changed concurrently, retry the request", http.StatusConflict)
	default:
		log.Printf("FAILED TO SAVE NETWORK CONFIG: %v", err)
		http.Error(w, "failed to save network config", http.StatusInternalServerError)
	}
	return false
}

// --- HTML Templates ---

const dashboardTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Clusterbook</title>
    <link rel="icon" type="image/png" href="https://raw.githubusercontent.com/stuttgart-things/docs/main/hugo/sthings-argo.png">
    <script src="https://unpkg.com/htmx.org@2.0.4"></script>
    <link href="https://fonts.googleapis.com/css2?family=Press+Start+2P&display=swap" rel="stylesheet">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif; background: #0f172a; color: #e2e8f0; min-height: 100vh; }
        .container { max-width: 1200px; margin: 0 auto; padding: 2rem; }
        h1 { font-size: 2rem; margin-bottom: 0.5rem; color: #f8fafc; }
        .subtitle { color: #94a3b8; margin-bottom: 2rem; }
        .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(340px, 1fr)); gap: 1.5rem; }
        .card { background: #1e293b; border-radius: 12px; padding: 1.5rem; border: 1px solid #334155; transition: border-color 0.2s; }
        .card:hover { border-color: #6366f1; }
        .card a { color: inherit; text-decoration: none; display: block; }
        .card-title { font-size: 1.25rem; font-weight: 600; margin-bottom: 1rem; color: #f8fafc; }
        .stats { display: grid; grid-template-columns: repeat(3, 1fr); gap: 0.75rem; margin-bottom: 1rem; }
        .stat { text-align: center; }
        .stat-value { font-size: 1.5rem; font-weight: 700; }
        .stat-label { font-size: 0.75rem; color: #94a3b8; text-transform: uppercase; letter-spacing: 0.05em; }
        .available { color: #4ade80; }
        .assigned { color: #f97316; }
        .pending { color: #facc15; }
        .bar { height: 8px; background: #334155; border-radius: 4px; overflow: hidden; display: flex; }
        .bar-assigned { background: #f97316; }
        .bar-pending { background: #facc15; }
        .bar-available { background: #4ade80; }
        .total-badge { display: inline-block; background: #334155; color: #94a3b8; padding: 0.25rem 0.75rem; border-radius: 9999px; font-size: 0.875rem; margin-top: 0.5rem; }
        .add-card { background: #1e293b; border-radius: 12px; padding: 1.5rem; border: 2px dashed #334155; display: flex; align-items: center; justify-content: center; min-height: 180px; cursor: pointer; transition: border-color 0.2s; }
        .add-card:hover { border-color: #6366f1; }
        .add-form { width: 100%; }
        .add-form input { background: #0f172a; border: 1px solid #334155; color: #e2e8f0; padding: 0.5rem 0.75rem; border-radius: 6px; font-size: 0.875rem; width: 100%; margin-bottom: 0.5rem; }
        .add-form .row { display: flex; gap: 0.5rem; }
        .add-form .row input { width: 50%; }
        .btn-add { background: #4f46e5; color: white; padding: 0.5rem 1rem; border-radius: 6px; border: none; cursor: pointer; font-size: 0.875rem; font-weight: 600; width: 100%; }
        .btn-add:hover { background: #6366f1; }
        .add-label { color: #94a3b8; font-size: 0.75rem; margin-bottom: 0.25rem; }
        .banner { text-align: center; margin-bottom: 2rem; padding: 1.5rem 0; }
        .banner img { height: 140px; filter: drop-shadow(0 0 12px rgba(99, 102, 241, 0.3)); }
        .banner-title { font-family: 'Press Start 2P', cursive; font-size: 1.8rem; color: #818cf8; margin-top: 0.75rem; letter-spacing: 0.1em; text-transform: uppercase; text-shadow: 3px 3px 0px #312e81, -1px -1px 0px #4f46e5; }
        .banner-sub { font-family: 'Press Start 2P', cursive; color: #f97316; font-size: 0.65rem; margin-top: 0.5rem; letter-spacing: 0.15em; text-transform: uppercase; }
        .footer { margin-top: 2rem; padding: 1rem; border-top: 1px solid #334155; display: flex; justify-content: center; gap: 2rem; font-size: 0.75rem; color: #64748b; }
        .footer-item { display: flex; align-items: center; gap: 0.35rem; }
        .footer-label { color: #475569; }
        .footer-value { color: #94a3b8; font-family: monospace; }
    </style>
</head>
<body>
    <div class="container">
        <div class="banner">
            <img src="https://raw.githubusercontent.com/stuttgart-things/docs/main/hugo/sthings-argo.png" alt="sthings">
            <div class="banner-title">Clusterbook</div>
            <div class="banner-sub">IP Address Management</div>
        </div>
        <div class="grid">
            {{range .Pools}}
            <div class="card">
                <a href="/network/{{.NetworkKey}}">
                    <div class="card-title">{{.NetworkKey}}.x</div>
                    <div class="stats">
                        <div class="stat">
                            <div class="stat-value available">{{.Available}}</div>
                            <div class="stat-label">Available</div>
                        </div>
                        <div class="stat">
                            <div class="stat-value assigned">{{.Assigned}}</div>
                            <div class="stat-label">Assigned</div>
                        </div>
                        <div class="stat">
                            <div class="stat-value pending">{{.Pending}}</div>
                            <div class="stat-label">Pending</div>
                        </div>
                    </div>
                    <div class="bar">
                        {{if .Assigned}}<div class="bar-assigned" style="width: {{printf "%.0f" (mul (div .Assigned .Total) 100.0)}}%"></div>{{end}}
                        {{if .Pending}}<div class="bar-pending" style="width: {{printf "%.0f" (mul (div .Pending .Total) 100.0)}}%"></div>{{end}}
                        {{if .Available}}<div class="bar-available" style="width: {{printf "%.0f" (mul (div .Available .Total) 100.0)}}%"></div>{{end}}
                    </div>
                    <div class="total-badge">{{.Total}} total IPs</div>
                </a>
            </div>
            {{end}}
            <div class="add-card">
                <form class="add-form" hx-post="/htmx/add-network" hx-swap="none">
                    <div class="card-title" style="text-align: center; margin-bottom: 1rem;">+ Add Network</div>
                    <div class="add-label">Subnet prefix (e.g. 10.31.105)</div>
                    <input type="text" name="network" placeholder="10.31.105" required>
                    <div class="add-label">IP range (last octet)</div>
                    <div class="row">
                        <input type="number" name="ip_from" placeholder="From" min="1" max="254" required>
                        <input type="number" name="ip_to" placeholder="To" min="1" max="254" required>
                    </div>
                    <button type="submit" class="btn-add">Create Network</button>
                </form>
            </div>
        </div>
        <div class="footer">
            <div class="footer-item"><span class="footer-label">version</span> <span class="footer-value">{{.Version}}</span></div>
            <div class="footer-item"><span class="footer-label">commit</span> <span class="footer-value">{{if gt (len .Commit) 7}}{{slice .Commit 0 7}}{{else}}{{.Commit}}{{end}}</span></div>
            <div class="footer-item"><span class="footer-label">built</span> <span class="footer-value">{{.StartTime}}</span></div>
            <div class="footer-item" style="margin-left:auto"><span class="footer-label">a</span> <a href="https://github.com/stuttgart-things" target="_blank" style="color:#818cf8;text-decoration:none">stuttgart-things</a> <span class="footer-label">project</span> <img src="https://raw.githubusercontent.com/stuttgart-things/docs/main/hugo/sthings-logo.png" alt="sthings" style="height:24px;vertical-align:middle"></div>
        </div>
    </div>
</body>
</html>`

const networkDetailTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Clusterbook - {{.NetworkKey}}</title>
    <link rel="icon" type="image/png" href="https://raw.githubusercontent.com/stuttgart-things/docs/main/hugo/sthings-argo.png">
    <script src="https://unpkg.com/htmx.org@2.0.4"></script>
    <link href="https://fonts.googleapis.com/css2?family=Press+Start+2P&display=swap" rel="stylesheet">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif; background: #0f172a; color: #e2e8f0; min-height: 100vh; }
        .container { max-width: 1200px; margin: 0 auto; padding: 2rem; }
        .header { display: flex; align-items: center; gap: 1rem; margin-bottom: 2rem; }
        .back { color: #6366f1; text-decoration: none; font-size: 1.5rem; }
        h1 { font-size: 2rem; color: #f8fafc; }
        .layout { display: grid; grid-template-columns: 200px 1fr; gap: 2rem; }
        .sidebar a { display: block; padding: 0.5rem 1rem; color: #94a3b8; text-decoration: none; border-radius: 6px; margin-bottom: 0.25rem; font-size: 0.875rem; }
        .sidebar a:hover, .sidebar a.active { background: #1e293b; color: #f8fafc; }
        table { width: 100%; border-collapse: collapse; background: #1e293b; border-radius: 12px; overflow: hidden; }
        th { background: #334155; text-align: left; padding: 0.75rem 1rem; font-size: 0.75rem; text-transform: uppercase; letter-spacing: 0.05em; color: #94a3b8; }
        td { padding: 0.75rem 1rem; border-bottom: 1px solid #334155; }
        tr:last-child td { border-bottom: none; }
        .badge { display: inline-block; padding: 0.2rem 0.6rem; border-radius: 9999px; font-size: 0.75rem; font-weight: 600; }
        .badge-available { background: #065f46; color: #4ade80; }
        .badge-assigned { background: #7c2d12; color: #f97316; }
        .badge-pending { background: #713f12; color: #facc15; }
        .btn { padding: 0.4rem 0.8rem; border-radius: 6px; border: none; cursor: pointer; font-size: 0.75rem; font-weight: 600; }
        .btn-assign { background: #4f46e5; color: white; }
        .btn-assign:hover { background: #6366f1; }
        .btn-release { background: #991b1b; color: white; }
        .btn-release:hover { background: #b91c1c; }
        .form-inline { display: flex; gap: 0.5rem; align-items: center; }
        .form-inline input { background: #0f172a; border: 1px solid #334155; color: #e2e8f0; padding: 0.4rem 0.6rem; border-radius: 6px; font-size: 0.75rem; width: 120px; }
        .form-inline select { background: #0f172a; border: 1px solid #334155; color: #e2e8f0; padding: 0.4rem 0.6rem; border-radius: 6px; font-size: 0.75rem; }
        .htmx-indicator { display: none; }
        .htmx-request .htmx-indicator { display: inline; }
        .toolbar { display: flex; gap: 0.75rem; align-items: center; margin-bottom: 1rem; flex-wrap: wrap; }
        .toolbar input { background: #0f172a; border: 1px solid #334155; color: #e2e8f0; padding: 0.4rem 0.6rem; border-radius: 6px; font-size: 0.75rem; width: 80px; }
        .btn-add-ip { background: #4f46e5; color: white; padding: 0.4rem 0.8rem; border-radius: 6px; border: none; cursor: pointer; font-size: 0.75rem; font-weight: 600; }
        .btn-add-ip:hover { background: #6366f1; }
        .btn-danger { background: #991b1b; color: white; padding: 0.4rem 0.8rem; border-radius: 6px; border: none; cursor: pointer; font-size: 0.75rem; font-weight: 600; margin-left: auto; }
        .btn-danger:hover { background: #b91c1c; }
        .banner { text-align: center; margin-bottom: 1rem; padding: 0.75rem 0; }
        .banner img { height: 100px; filter: drop-shadow(0 0 10px rgba(99, 102, 241, 0.3)); }
        .banner a { text-decoration: none; }
        .banner-title { font-family: 'Press Start 2P', cursive; font-size: 1.0rem; color: #818cf8; margin-top: 0.5rem; letter-spacing: 0.1em; text-transform: uppercase; text-shadow: 2px 2px 0px #312e81; }
    </style>
</head>
<body>
    <div class="container">
        <div class="banner">
            <a href="/"><img src="https://raw.githubusercontent.com/stuttgart-things/docs/main/hugo/sthings-argo.png" alt="sthings"></a>
            <div class="banner-title">Clusterbook</div>
        </div>
        <div class="header">
            <a href="/" class="back">&larr;</a>
            <h1>{{.NetworkKey}}.x</h1>
        </div>
        <div class="layout">
            <div class="sidebar">
                {{range .Pools}}
                <a href="/network/{{.NetworkKey}}" {{if eq $.NetworkKey .NetworkKey}}class="active"{{end}}>{{.NetworkKey}}.x</a>
                {{end}}
            </div>
            <div>
                <div class="toolbar">
                    <form class="form-inline" hx-post="/htmx/add-ip" hx-target="#ip-table" hx-swap="innerHTML">
                        <input type="hidden" name="network_key" value="{{.NetworkKey}}">
                        <input type="text" name="ip" placeholder="Last octet" required style="width: 90px;">
                        <button type="submit" class="btn-add-ip">+ Add IP</button>
                    </form>
                    <form hx-post="/htmx/delete-network" hx-swap="none" hx-confirm="Delete network {{.NetworkKey}}? This cannot be undone.">
                        <input type="hidden" name="network_key" value="{{.NetworkKey}}">
                        <button type="submit" class="btn-danger">Delete Network</button>
                    </form>
                </div>
                <div id="ip-table">` + ipTablePartial + `</div>
            </div>
        </div>
    </div>
</body>
</html>`

const ipTablePartial = `<table>
    <thead>
        <tr>
            <th>IP Address</th>
            <th>Status</th>
            <th>Cluster</th>
            <th>Actions</th>
        </tr>
    </thead>
    <tbody>
        {{range .Entries}}
        <tr>
            <td style="font-family: monospace;">{{.IP}}</td>
            <td>
                {{if hasPrefix .Status "ASSIGNED"}}<span class="badge badge-assigned">ASSIGNED</span>{{if hasSuffix .Status ":DNS"}} <span class="badge" style="background: #1e3a5f; color: #60a5fa; font-size: 0.65rem;">DNS</span> <span id="dns-test-{{.Digit}}" style="display:inline;"><button class="btn" style="background:#334155;color:#60a5fa;font-size:0.6rem;padding:0.2rem 0.4rem;" hx-post="/htmx/test-dns" hx-vals='{"cluster":"{{.Cluster}}","expected_ip":"{{.IP}}"}' hx-target="#dns-test-{{.Digit}}" hx-swap="innerHTML">Test</button></span>{{end}}
                {{else if hasPrefix .Status "PENDING"}}<span class="badge badge-pending">PENDING</span>{{if hasSuffix .Status ":DNS"}} <span class="badge" style="background: #1e3a5f; color: #60a5fa; font-size: 0.65rem;">DNS</span> <span id="dns-test-{{.Digit}}" style="display:inline;"><button class="btn" style="background:#334155;color:#60a5fa;font-size:0.6rem;padding:0.2rem 0.4rem;" hx-post="/htmx/test-dns" hx-vals='{"cluster":"{{.Cluster}}","expected_ip":"{{.IP}}"}' hx-target="#dns-test-{{.Digit}}" hx-swap="innerHTML">Test</button></span>{{end}}
                {{else}}<span class="badge badge-available">AVAILABLE</span>
                {{end}}
            </td>
            <td>{{if .Cluster}}{{.Cluster}}{{else}}<span style="color: #475569;">—</span>{{end}}</td>
            <td>
                <div style="display: flex; gap: 0.5rem; align-items: center;">
                {{if or (hasPrefix .Status "ASSIGNED") (hasPrefix .Status "PENDING")}}
                <form class="form-inline" hx-post="/htmx/edit" hx-target="#ip-table" hx-swap="innerHTML">
                    <input type="hidden" name="ip" value="{{.IP}}">
                    <input type="hidden" name="network_key" value="{{$.NetworkKey}}">
                    <input type="text" name="cluster" value="{{.Cluster}}" placeholder="Cluster name" required>
                    <select name="status">
                        <option value="ASSIGNED" {{if hasPrefix .Status "ASSIGNED"}}selected{{end}}>ASSIGNED</option>
                        <option value="PENDING" {{if hasPrefix .Status "PENDING"}}selected{{end}}>PENDING</option>
                    </select>
                    <label style="display: flex; align-items: center; gap: 0.15rem; font-size: 0.75rem; color: #94a3b8; cursor: pointer; white-space: nowrap;"><input type="checkbox" name="create_dns" {{if hasSuffix .Status ":DNS"}}checked{{end}} style="accent-color: #6366f1; margin-right: 0.15rem;"> DNS</label>
                    <button type="submit" class="btn btn-assign">Save</button>
                </form>
                <form class="form-inline" hx-post="/htmx/release" hx-target="#ip-table" hx-swap="innerHTML">
                    <input type="hidden" name="ip" value="{{.IP}}">
                    <input type="hidden" name="network_key" value="{{$.NetworkKey}}">
                    <button type="submit" class="btn btn-release">Release</button>
                </form>
                {{else}}
                <form class="form-inline" hx-post="/htmx/assign" hx-target="#ip-table" hx-swap="innerHTML">
                    <input type="hidden" name="ip" value="{{.IP}}">
                    <input type="hidden" name="network_key" value="{{$.NetworkKey}}">
                    <input type="text" name="cluster" placeholder="Cluster name" required>
                    <select name="status">
                        <option value="ASSIGNED">ASSIGNED</option>
                        <option value="PENDING">PENDING</option>
                    </select>
                    <label style="display: flex; align-items: center; gap: 0.15rem; font-size: 0.75rem; color: #94a3b8; cursor: pointer; white-space: nowrap;"><input type="checkbox" name="create_dns" style="accent-color: #6366f1; margin-right: 0.15rem;"> DNS</label>
                    <button type="submit" class="btn btn-assign">Assign</button>
                </form>
                {{end}}
                <form hx-post="/htmx/delete-ip" hx-target="#ip-table" hx-swap="innerHTML" hx-confirm="Remove {{.IP}} from this network?">
                    <input type="hidden" name="ip" value="{{.IP}}">
                    <input type="hidden" name="network_key" value="{{$.NetworkKey}}">
                    <button type="submit" class="btn btn-release" style="font-size: 0.65rem; padding: 0.3rem 0.5rem;">&#x2715;</button>
                </form>
                </div>
            </td>
        </tr>
        {{end}}
    </tbody>
</table>`

// helper functions for templates are not supported in raw template strings,
// so we use a FuncMap approach
func init() {
	// Override the template parsing to include helper functions
}

// TemplateFuncs returns template helper functions
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"div": func(a, b int) float64 {
			if b == 0 {
				return 0
			}
			return float64(a) / float64(b)
		},
		"mul": func(a float64, b float64) float64 {
			return a * b
		},
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,
	}
}
