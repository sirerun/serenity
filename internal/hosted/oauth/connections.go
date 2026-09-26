package oauth

import (
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/ajent-social/go/mcpoauth"
)

type connection struct{ ID, Name, Access, Expires string }

func (h *Hosted) connections(w http.ResponseWriter, r *http.Request) {
	s, err := h.session(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	rows, err := h.Store.DB.DB().QueryContext(r.Context(), "SELECT record FROM oauth_grants WHERE json_extract(record,'$.subject')=? ORDER BY expires_at DESC LIMIT 100", s.AccountID)
	if err != nil {
		http.Error(w, "Connections unavailable", http.StatusServiceUnavailable)
		return
	}
	var records []mcpoauth.GrantRecord
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			break
		}
		var v mcpoauth.GrantRecord
		if err = json.Unmarshal([]byte(raw), &v); err != nil {
			break
		}
		records = append(records, v)
	}
	scanErr := rows.Err()
	_ = rows.Close()
	if err != nil || scanErr != nil {
		http.Error(w, "Connections unavailable", http.StatusServiceUnavailable)
		return
	}
	var connections []connection
	for _, v := range records {
		if !v.RevokedAt.IsZero() || !time.Now().Before(v.ExpiresAt) {
			continue
		}
		c, err := h.Store.Client(r.Context(), v.ClientID)
		if errors.Is(err, mcpoauth.ErrNotFound) {
			c.Name = "Previously registered agent"
		} else if err != nil {
			http.Error(w, "Connections unavailable", http.StatusServiceUnavailable)
			return
		}
		connections = append(connections, connection{v.ID, c.Name, strings.Join(v.Scopes, ", "), v.ExpiresAt.UTC().Format("2 Jan 2006 15:04 UTC")})
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = connectionsPage.Execute(w, struct {
		CSRF        string
		Connections []connection
	}{s.CSRF, connections})
}
func (h *Hosted) disconnect(w http.ResponseWriter, r *http.Request) {
	s, ok := h.mutation(w, r)
	if !ok {
		return
	}
	id := r.PostForm.Get("id")
	v, err := h.Store.Grant(r.Context(), id)
	if err != nil || v.Subject != s.AccountID {
		http.Error(w, "Connection unavailable", 404)
		return
	}
	if err = h.Store.RevokeGrant(r.Context(), id, time.Now()); err != nil {
		http.Error(w, "Could not disconnect. Try again.", http.StatusServiceUnavailable)
		return
	}
	http.Redirect(w, r, "/oauth/connections", http.StatusSeeOther)
}

var connectionsPage = template.Must(template.New("connections").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Connected agents · Serenity</title><link rel="stylesheet" href="/assets/site.css"><link rel="stylesheet" href="/assets/hosted.css"></head><body><header class="site-nav"><a class="brand" href="/"><img src="/assets/brand.svg" alt="">Serenity</a><nav class="nav-links" aria-label="Main navigation"><a href="/docs/">Docs</a><a href="/dashboard">Your memory</a></nav></header><main class="hosted-main"><p><a href="/dashboard/connections">Back to connections</a></p><h1>Connected agents</h1><p>OAuth connections are listed below. Connection names are supplied by each app. Manual bearer tokens are managed in account settings.</p>{{range .Connections}}<section class="panel"><h2>{{.Name}}</h2><p>{{.Access}}</p><p>Connection expires {{.Expires}}</p><form method="post" action="/oauth/disconnect"><input type="hidden" name="csrf" value="{{$.CSRF}}"><input type="hidden" name="id" value="{{.ID}}"><button class="secondary">Disconnect agent</button></form></section>{{else}}<p>No active OAuth connections. Start from your agent’s MCP connection settings using Serenity’s server URL.</p>{{end}}<p><a href="/docs/connections/">Connection guides</a></p></main></body></html>`))
