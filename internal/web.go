/*
Copyright © 2024 Patrick Hermann patrick.hermann@sva.de
*/

package internal

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sort"
	"strconv"
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
	IP      string
	Digit   string
	Status  string
	Cluster string
}

// StartWebServer starts the HTTP server for HTMX frontend and REST API
func StartWebServer(httpPort, loadFrom, configLoc, configNm string) {
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
		handleAPINetworkIPs(w, r, loadFrom, configLoc, configNm)
	})
	mux.HandleFunc("POST /api/v1/networks/{key}/assign", func(w http.ResponseWriter, r *http.Request) {
		handleAPIAssign(w, r, loadFrom, configLoc, configNm)
	})
	mux.HandleFunc("POST /api/v1/networks/{key}/release", func(w http.ResponseWriter, r *http.Request) {
		handleAPIRelease(w, r, loadFrom, configLoc, configNm)
	})

	// HTMX partial routes
	mux.HandleFunc("POST /htmx/assign", func(w http.ResponseWriter, r *http.Request) {
		handleHTMXAssign(w, r, loadFrom, configLoc, configNm)
	})
	mux.HandleFunc("POST /htmx/release", func(w http.ResponseWriter, r *http.Request) {
		handleHTMXRelease(w, r, loadFrom, configLoc, configNm)
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
			switch ipInfo.Status {
			case "ASSIGNED":
				info.Assigned++
			case "PENDING":
				info.Pending++
			default:
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
			IP:      networkKey + "." + digit,
			Digit:   digit,
			Status:  info.Status,
			Cluster: info.Cluster,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		a, _ := strconv.Atoi(entries[i].Digit)
		b, _ := strconv.Atoi(entries[j].Digit)
		return a < b
	})
	return entries
}

// --- HTMX Frontend Handlers ---

func handleDashboard(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string) {
	ipList := LoadProfile(loadFrom, configLoc, configNm)
	pools := getPoolInfos(ipList)

	tmpl := template.Must(template.New("dashboard").Funcs(TemplateFuncs()).Parse(dashboardTemplate))
	if err := tmpl.Execute(w, pools); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleNetworkDetail(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string) {
	networkKey := r.PathValue("key")
	ipList := LoadProfile(loadFrom, configLoc, configNm)

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

	tmpl := template.Must(template.New("network").Parse(networkDetailTemplate))
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleHTMXAssign(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ip := r.FormValue("ip")
	cluster := r.FormValue("cluster")
	status := r.FormValue("status")
	networkKey := r.FormValue("network_key")

	if ip == "" || cluster == "" || status == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	ipList := LoadProfile(loadFrom, configLoc, configNm)

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
	entry.Status = status
	entry.Cluster = cluster
	ipList[ipKey][ipDigit] = entry

	saveConfig(ipList, loadFrom, configLoc, configNm)

	// Re-render the network detail table
	ips := ipList[networkKey]
	entries := getIPEntries(ips, networkKey)
	tmpl := template.Must(template.New("table").Parse(ipTablePartial))
	tmpl.Execute(w, struct {
		NetworkKey string
		Entries    []IPEntry
	}{networkKey, entries})
}

func handleHTMXRelease(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ip := r.FormValue("ip")
	networkKey := r.FormValue("network_key")

	ipList := LoadProfile(loadFrom, configLoc, configNm)

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
	entry.Status = ""
	entry.Cluster = ""
	ipList[ipKey][ipDigit] = entry

	saveConfig(ipList, loadFrom, configLoc, configNm)

	// Re-render the network detail table
	ips := ipList[networkKey]
	entries := getIPEntries(ips, networkKey)
	tmpl := template.Must(template.New("table").Parse(ipTablePartial))
	tmpl.Execute(w, struct {
		NetworkKey string
		Entries    []IPEntry
	}{networkKey, entries})
}

// --- REST API Handlers ---

func handleAPINetworks(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string) {
	ipList := LoadProfile(loadFrom, configLoc, configNm)
	pools := getPoolInfos(ipList)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pools)
}

func handleAPINetworkIPs(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string) {
	networkKey := r.PathValue("key")
	ipList := LoadProfile(loadFrom, configLoc, configNm)

	ips, ok := ipList[networkKey]
	if !ok {
		http.Error(w, `{"error":"network not found"}`, http.StatusNotFound)
		return
	}

	entries := getIPEntries(ips, networkKey)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func handleAPIAssign(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string) {
	networkKey := r.PathValue("key")

	var req struct {
		IP      string `json:"ip"`
		Cluster string `json:"cluster"`
		Status  string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.IP == "" || req.Cluster == "" {
		http.Error(w, `{"error":"ip and cluster are required"}`, http.StatusBadRequest)
		return
	}

	if req.Status == "" {
		req.Status = "ASSIGNED"
	}

	ipList := LoadProfile(loadFrom, configLoc, configNm)

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
	entry.Status = req.Status
	entry.Cluster = req.Cluster
	ipList[networkKey][ipDigit] = entry

	saveConfig(ipList, loadFrom, configLoc, configNm)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": fmt.Sprintf("IP %s assigned to cluster %s", req.IP, req.Cluster),
	})
}

func handleAPIRelease(w http.ResponseWriter, r *http.Request, loadFrom, configLoc, configNm string) {
	networkKey := r.PathValue("key")

	var req struct {
		IP string `json:"ip"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	ipList := LoadProfile(loadFrom, configLoc, configNm)

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
	entry.Status = ""
	entry.Cluster = ""
	ipList[networkKey][ipDigit] = entry

	saveConfig(ipList, loadFrom, configLoc, configNm)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": fmt.Sprintf("IP %s released", req.IP),
	})
}

// saveConfig persists the IP list based on the configured backend
func saveConfig(ipList map[string]IPs, loadFrom, configLoc, configNm string) {
	switch loadFrom {
	case "disk":
		SaveYAMLToDisk(ipList, configLoc+"/"+configNm)
	case "cr":
		ipListCR := ConvertToCRFormat(ipList)
		if err := CreateOrUpdateNetworkConfig(ipListCR, configNm, configLoc); err != nil {
			log.Printf("ERROR SAVING CR: %v", err)
		}
	default:
		log.Printf("INVALID LOAD_CONFIG_FROM VALUE: %s", loadFrom)
	}
}

// --- HTML Templates ---

const dashboardTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Clusterbook</title>
    <script src="https://unpkg.com/htmx.org@2.0.4"></script>
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
    </style>
</head>
<body>
    <div class="container">
        <h1>Clusterbook</h1>
        <p class="subtitle">IP Address Management Dashboard</p>
        <div class="grid">
            {{range .}}
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
    <script src="https://unpkg.com/htmx.org@2.0.4"></script>
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
    </style>
</head>
<body>
    <div class="container">
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
            <div id="ip-table">` + ipTablePartial + `</div>
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
                {{if eq .Status "ASSIGNED"}}<span class="badge badge-assigned">ASSIGNED</span>
                {{else if eq .Status "PENDING"}}<span class="badge badge-pending">PENDING</span>
                {{else}}<span class="badge badge-available">AVAILABLE</span>
                {{end}}
            </td>
            <td>{{if .Cluster}}{{.Cluster}}{{else}}<span style="color: #475569;">—</span>{{end}}</td>
            <td>
                {{if or (eq .Status "ASSIGNED") (eq .Status "PENDING")}}
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
                    <button type="submit" class="btn btn-assign">Assign</button>
                </form>
                {{end}}
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
	}
}
