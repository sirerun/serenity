package oauth

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ajent-social/go/mcpoauth"
)

// binding reports a snapshot for this token only; it grants no future authority.
func (h *Hosted) binding(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Pragma", "no-cache")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		http.Error(w, "query parameters are not supported", http.StatusBadRequest)
		return
	}
	raw, ok := mcpoauth.BearerToken(r)
	if !ok {
		h.Server.Challenge(w, false)
		return
	}
	ident, epoch, err := h.verifyIdentity(r.Context(), raw)
	if err != nil {
		h.Server.Challenge(w, true)
		return
	}
	brain, _, _ := strings.Cut(ident.Binding, ".")
	response := struct {
		Schema          string    `json:"schema"`
		Issuer          string    `json:"issuer"`
		Resource        string    `json:"resource"`
		AccountID       string    `json:"account_id"`
		AccountState    string    `json:"account_state"`
		ProjectID       string    `json:"project_id"`
		Scopes          []string  `json:"scopes"`
		GrantID         string    `json:"grant_id"`
		ProjectState    string    `json:"project_state"`
		RevocationEpoch int       `json:"revocation_epoch"`
		ObservedAt      time.Time `json:"observed_at"`
	}{"serenity.oauth-binding/v1", h.Origin, h.Origin + "/mcp", ident.Subject, "active", brain, ident.Scopes, ident.GrantID, "ready", epoch, time.Now().UTC()}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return
	}
}
