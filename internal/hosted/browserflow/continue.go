// Package browserflow completes actions before navigating outside a form policy.
package browserflow

import (
	"html/template"
	"net/http"
	"net/url"
)

// Continue renders a fresh document so callback redirects are not a form
// submission. target must be a validated OAuth or trusted provider response,
// never an unvalidated request parameter.
func Continue(w http.ResponseWriter, target string) {
	u, err := url.Parse(target)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		http.Error(w, "Connection destination unavailable", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; form-action 'none'; frame-ancestors 'none'; base-uri 'none'")
	_ = page.Execute(w, target)
}

var page = template.Must(template.New("continue").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="refresh" content="0;url={{.}}"><title>Continue · Serenity</title></head><body><main><h1>Returning you to your app.</h1><p>If you are not redirected, <a href="{{.}}" rel="noreferrer">continue to your app</a>.</p></main></body></html>`))
