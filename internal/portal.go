package internal

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Behind sthings-portal (stuttgart-things/sthings-portal#23) clusterbook runs
// under a prefix and inside the portal's frame:
//
//   - the portal forwards /<prefix>/... unchanged, so the pages answer under
//     BASE_PATH and resolve their relative URLs against <base href>;
//   - a request through the portal carries X-Forwarded-Prefix; the page then
//     links the portal's design tokens, which win over its own copy;
//   - inside the portal's frame the page swaps its banner for one compact
//     line (the portal already shows the service name).

// ParseBasePath checks BASE_PATH: "" (or "/") means no prefix, otherwise it
// is "/" plus path segments, without a trailing slash.
func ParseBasePath(v string) (string, error) {
	v = strings.TrimSuffix(strings.TrimSpace(v), "/")
	if v == "" {
		return "", nil
	}
	if !strings.HasPrefix(v, "/") {
		return "", fmt.Errorf("BASE_PATH %q must start with /", v)
	}
	for _, seg := range strings.Split(v[1:], "/") {
		if seg == "" || seg == "." || seg == ".." || strings.ContainsAny(seg, "?#%{}\\ \t\"'<>") {
			return "", fmt.Errorf("BASE_PATH %q is not a clean path", v)
		}
	}
	return v, nil
}

type basePathKey struct{}

// withBasePath records the prefix the request came in under, for the pages'
// <base href> and the HX-Redirect targets.
func withBasePath(basePath string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), basePathKey{}, basePath)))
	})
}

// basePathFrom returns the prefix of the request ("" without one).
func basePathFrom(r *http.Request) string {
	p, _ := r.Context().Value(basePathKey{}).(string)
	return p
}

// page is what every full HTML page gets.
type page struct {
	// Base is the prefix for <base href>; "" serves at the root.
	Base string
	// PortalTheme links the portal's /static/homerun2-tokens.css: the
	// request came through the portal.
	PortalTheme bool
}

func newPage(r *http.Request) page {
	return page{Base: basePathFrom(r), PortalTheme: r.Header.Get("X-Forwarded-Prefix") != ""}
}

// pageHead goes at the top of every page's <head>: the base URL, the
// embedded class before the first paint, and the design tokens. The copy
// under :where(:root) keeps the standalone look; the portal's :root wins.
const pageHead = `<base href="{{.Base}}/">
    <script>if (window.self !== window.top) document.documentElement.classList.add('hr-embedded')</script>
    <style>
        :where(:root) {
            --hr-bg: #0f172a; --hr-surface: #111827; --hr-card: #1e293b; --hr-border: #334155;
            --hr-text: #e2e8f0; --hr-muted: #94a3b8; --hr-accent: #6366f1;
            --hr-font: -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
        }
        .embedded-bar { display: none; }
        .hr-embedded .banner, .hr-embedded .footer { display: none; }
        .hr-embedded .embedded-bar { display: flex; align-items: center; justify-content: space-between; gap: 1rem; min-height: 44px; padding: 6px 16px; font-family: var(--hr-font); background: var(--hr-surface); border-bottom: 1px solid var(--hr-border); }
        .hr-embedded .container { padding-top: 1rem; }
        .embedded-sub { font-size: 13px; color: var(--hr-muted); }
        .embedded-stats { display: flex; gap: 0.5rem; font-size: 12px; }
        .embedded-stats span { padding: 0.1rem 0.5rem; border: 1px solid var(--hr-border); border-radius: 9999px; color: var(--hr-muted); }
        .embedded-stats a { color: var(--hr-accent); text-decoration: none; }
    </style>
    {{if .PortalTheme}}<link rel="stylesheet" href="/static/homerun2-tokens.css">{{end}}`
