package direction

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/router"
)

type discardLedger struct{}

func (discardLedger) Record(context.Context, router.SpendEntry) error { return nil }

// TestCheckPlanProviderErrorMapsToStatusWithoutBody is AI-L02: when the
// classification provider fails, check_plan answers provider_error with
// the provider's HTTP status and never echoes the provider's response
// body (which can carry request ids, account hints or prompt fragments).
func TestCheckPlanProviderErrorMapsToStatusWithoutBody(t *testing.T) {
	const sentinel = "provider-body-SENTINEL-do-not-echo"
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(529)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"` + sentinel + `"}}`))
	}))
	t.Cleanup(provider.Close)
	rtr := router.New(map[router.Tier]router.Provider{
		router.TierLocalCheap: &router.AnthropicProvider{BaseURL: provider.URL, APIKey: "test", Model: "m", Version: "v"},
	}, discardLedger{})

	env := newTestEnv(t, WithRouter(rtr))
	seedConstraint(t, env.ledger, "cst-0001", "fixture spend ceiling", "spend_over", "{amount: {gte: 200}}",
		`Unbounded spend risk: "no ceiling" was rejected outright.`, "quarterly budget review")
	base, token := startTestServer(t, env.handlers)

	resp := postJSON(t, base, token, "/direction/check_plan", map[string]any{"plan_text": "spend 500 dollars"})
	body := readBody(t, resp)
	if strings.Contains(string(body), sentinel) {
		t.Fatalf("check_plan echoed the provider body: %s", body)
	}
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body %s", resp.StatusCode, body)
	}
	var pe struct {
		Code           string `json:"error"`
		ProviderStatus int    `json:"provider_status"`
	}
	if err := json.Unmarshal(body, &pe); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if pe.Code != "provider_error" || pe.ProviderStatus != 529 {
		t.Fatalf("error envelope = %+v, want provider_error with provider_status 529", pe)
	}
}
