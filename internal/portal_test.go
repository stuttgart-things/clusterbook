package internal

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestParseBasePath(t *testing.T) {
	for in, want := range map[string]string{
		"": "", "/": "", "/clusterbook": "/clusterbook", "/clusterbook/": "/clusterbook", " /lab/ipam": "/lab/ipam",
	} {
		got, err := ParseBasePath(in)
		if err != nil || got != want {
			t.Errorf("ParseBasePath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"clusterbook", "/a//b", "/a/../b", "/a b", "/a?x", "/a#x", "/a%2f", `/a"b`} {
		if got, err := ParseBasePath(in); err == nil {
			t.Errorf("ParseBasePath(%q) = %q, want an error", in, got)
		}
	}
}

func serveWeb(h http.Handler, method, path string, form url.Values, header ...string) *httptest.ResponseRecorder {
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// absoluteURL finds URLs the pages hand to the browser that ignore <base href>:
// an absolute path in href/src/hx-* (the portal's token stylesheet aside).
var absoluteURL = regexp.MustCompile(`(?:href|src|hx-post|hx-get|hx-delete|hx-put)="(/[^"]*)"`)

func TestWebHandlerWithoutBasePath(t *testing.T) {
	dir, name := setupTestConfig(t, testConfigYAML)
	h := NewWebHandler("", "disk", dir, name, nil, nil)

	for _, path := range []string{"/", "/network/10.31.103", "/health", "/api/v1/networks"} {
		if w := serveWeb(h, "GET", path, nil); w.Code != http.StatusOK {
			t.Errorf("GET %s: got %d", path, w.Code)
		}
	}
	body := serveWeb(h, "GET", "/", nil).Body.String()
	if !strings.Contains(body, `<base href="/">`) {
		t.Error(`dashboard: expected <base href="/">`)
	}
	if !strings.Contains(body, `href="network/10.31.103"`) {
		t.Error("dashboard: expected a relative network link")
	}
}

func TestWebHandlerBasePath(t *testing.T) {
	dir, name := setupTestConfig(t, testConfigYAML)
	h := NewWebHandler("/clusterbook", "disk", dir, name, nil, nil)

	for _, path := range []string{
		"/clusterbook/", "/clusterbook/network/10.31.103", "/clusterbook/health",
		"/clusterbook/api/v1/networks", "/health", "/api/v1/networks", "/api/v1/clusters",
	} {
		if w := serveWeb(h, "GET", path, nil); w.Code != http.StatusOK {
			t.Errorf("GET %s: got %d", path, w.Code)
		}
	}
	for _, path := range []string{"/", "/network/10.31.103"} {
		if w := serveWeb(h, "GET", path, nil); w.Code != http.StatusNotFound {
			t.Errorf("GET %s outside the prefix: got %d, want 404", path, w.Code)
		}
	}
	if w := serveWeb(h, "GET", "/clusterbook", nil); w.Code/100 != 3 || w.Header().Get("Location") != "/clusterbook/" {
		t.Errorf("GET /clusterbook: got %d %q, want a redirect to /clusterbook/", w.Code, w.Header().Get("Location"))
	}

	for _, path := range []string{"/clusterbook/", "/clusterbook/network/10.31.103"} {
		body := serveWeb(h, "GET", path, nil).Body.String()
		if !strings.Contains(body, `<base href="/clusterbook/">`) {
			t.Errorf("%s: expected <base href=\"/clusterbook/\">", path)
		}
		withoutBase := strings.Replace(body, `<base href="/clusterbook/">`, "", 1)
		if m := absoluteURL.FindAllStringSubmatch(withoutBase, -1); len(m) > 0 {
			t.Errorf("%s: absolute URLs bypass <base href>: %v", path, m)
		}
		if strings.Contains(body, "homerun2-tokens.css") {
			t.Errorf("%s: links the portal tokens without X-Forwarded-Prefix", path)
		}
	}

	// A table fragment (swapped into the page) uses relative URLs too.
	w := serveWeb(h, "POST", "/clusterbook/htmx/add-ip", url.Values{"network_key": {"10.31.103"}, "ip": {"20"}})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `hx-post="htmx/assign"`) {
		t.Errorf("add-ip fragment: got %d, want relative hx-post", w.Code)
	}
	if m := absoluteURL.FindAllStringSubmatch(w.Body.String(), -1); len(m) > 0 {
		t.Errorf("add-ip fragment: absolute URLs: %v", m)
	}

	// HX-Redirect targets carry the prefix.
	w = serveWeb(h, "POST", "/clusterbook/htmx/add-network", url.Values{"network": {"10.31.105"}, "ip_from": {"1"}, "ip_to": {"3"}})
	if got := w.Header().Get("HX-Redirect"); got != "/clusterbook/network/10.31.105" {
		t.Errorf("add-network HX-Redirect = %q (status %d)", got, w.Code)
	}
	w = serveWeb(h, "POST", "/clusterbook/htmx/delete-network", url.Values{"network_key": {"10.31.105"}})
	if got := w.Header().Get("HX-Redirect"); got != "/clusterbook/" {
		t.Errorf("delete-network HX-Redirect = %q (status %d)", got, w.Code)
	}
}

func TestWebPortalEmbedding(t *testing.T) {
	dir, name := setupTestConfig(t, testConfigYAML)
	h := NewWebHandler("/clusterbook", "disk", dir, name, nil, nil)

	for _, path := range []string{"/clusterbook/", "/clusterbook/network/10.31.103"} {
		body := serveWeb(h, "GET", path, nil, "X-Forwarded-Prefix", "/clusterbook").Body.String()
		for _, want := range []string{
			`<link rel="stylesheet" href="/static/homerun2-tokens.css">`,
			`classList.add('hr-embedded')`,
			`class="embedded-bar"`,
			`:where(:root)`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: expected %q", path, want)
			}
		}
	}
	if body := serveWeb(h, "GET", "/clusterbook/", nil).Body.String(); !strings.Contains(body, "2 networks") {
		t.Error("dashboard embedded bar: expected the network count")
	}
}
