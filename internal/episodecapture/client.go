// Package episodecapture connects protected queue primitives to the gated
// synthetic episode protocol. It is not a production capture enrollment path.
package episodecapture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/episodeclient"
	"github.com/sirerun/serenity/internal/store"
)

var ErrUnavailable = errors.New("episode_protocol_unavailable")
var ErrReceipt = errors.New("episode_receipt_not_verified")

// Options must come from trusted enrollment, never a repository or transcript.
// Token stays in caller custody and is obtained afresh for each request.
// Authorize rechecks current workspace mapping, policy and private-session state.
// Synthetic must be explicit: real-history upload is intentionally unavailable.
type Options struct {
	Endpoint   string
	HTTPClient *http.Client
	Binding    episodeclient.Binding
	Synthetic  bool
	Token      func(context.Context) (string, error)
	Authorize  func(context.Context, episodeclient.ReplayKey) bool
}
type Client struct {
	options Options
	http    *http.Client
}

func New(o Options) (*Client, error) {
	u, err := url.Parse(o.Endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/episodes/v1" || !o.Synthetic || o.Token == nil || o.Authorize == nil || !store.ValidEpisodeID(o.Binding.Brain) || !store.ValidEpisodeID(o.Binding.Policy) || !store.ValidEpisodeID(o.Binding.Producer) {
		return nil, ErrUnavailable
	}
	h := http.Client{Timeout: 10 * time.Second}
	if o.HTTPClient != nil {
		h = *o.HTTPClient
	}
	// Redirects could disclose a bearer credential to another destination. Never
	// follow them, even if the supplied HTTP client has a permissive policy.
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if h.Timeout <= 0 || h.Timeout > 10*time.Second {
		h.Timeout = 10 * time.Second
	}
	return &Client{o, &h}, nil
}

type request struct {
	Schema        string              `json:"schema"`
	ProjectID     string              `json:"project_id"`
	Operation     string              `json:"operation"`
	Qualification string              `json:"qualification"`
	Generation    *uint64             `json:"generation,omitempty"`
	Mutation      *store.EpisodeInput `json:"mutation,omitempty"`
}
type response struct {
	Schema     string                `json:"schema"`
	ProjectID  string                `json:"project_id"`
	Status     string                `json:"status"`
	Generation *uint64               `json:"generation"`
	Revision   string                `json:"revision"`
	Fences     map[string]uint64     `json:"fences"`
	Receipt    *store.EpisodeReceipt `json:"receipt,omitempty"`
}

func (c *Client) call(ctx context.Context, operation string, generation *uint64, input *store.EpisodeInput) (response, error) {
	var out response
	token, err := c.options.Token(ctx)
	if err != nil || token == "" || strings.ContainsAny(token, "\r\n") {
		return out, ErrUnavailable
	}
	raw, err := json.Marshal(request{store.EpisodeSchema, c.options.Binding.Brain, operation, "synthetic", generation, input})
	if err != nil || len(raw) > store.EpisodeMaxRequestBytes {
		return out, ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.options.Endpoint, bytes.NewReader(raw))
	if err != nil {
		return out, ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return out, ErrUnavailable
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return out, ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, store.EpisodeMaxResponseBytes+1))
	if err != nil || len(body) > store.EpisodeMaxResponseBytes || !episodeclient.ValidJSON(body) || json.Unmarshal(body, &out) != nil {
		return response{}, ErrUnavailable
	}
	if out.Schema != store.EpisodeSchema || out.ProjectID != c.options.Binding.Brain || out.Status != "completed" || out.Generation == nil || !store.ValidSourceSHA(out.Revision) {
		return response{}, ErrReceipt
	}
	return out, nil
}
func (c *Client) allowed(ctx context.Context, key episodeclient.ReplayKey) bool {
	return key.Binding == c.options.Binding && c.options.Authorize(ctx, key)
}

// Reconcile authenticates to the exact brain and downloads its complete bounded
// fence set. Opaque keys preserve producer identity across restarts and
// reconciliation for unrelated conversations.
func (c *Client) Reconcile(ctx context.Context, key episodeclient.ReplayKey) (episodeclient.Verification, error) {
	var out episodeclient.Verification
	if !c.allowed(ctx, key) {
		return out, ErrUnavailable
	}
	res, err := c.call(ctx, "fences", nil, nil)
	if err != nil {
		return out, err
	}
	if res.Fences == nil || len(res.Fences) > store.EpisodeMaxFences {
		return out, ErrReceipt
	}
	for hash, g := range res.Fences {
		if !store.ValidSourceSHA(hash) || g == 0 || g > *res.Generation {
			return out, ErrReceipt
		}
	}
	now := time.Now()
	out = episodeclient.Verification{Binding: key.Binding, Workspace: key.Workspace, Allowed: true, Full: true, GenerationKnown: true, Generation: *res.Generation, SyncedAt: now, ExpiresAt: now.Add(30 * time.Second), Conversations: map[string]bool{}}
	for hash := range res.Fences {
		out.Conversations[hash] = true
	}
	return out, nil
}
func mutation(r episodeclient.Record) (store.EpisodeInput, error) {
	if r.Sequence > math.MaxInt64 {
		return store.EpisodeInput{}, ErrUnavailable
	}
	kind := r.Kind
	if kind == "tool_request" {
		kind = "tool_call"
	}
	event := store.EpisodeEvent{ID: r.Event, Sequence: int64(r.Sequence), Role: r.Role, Type: kind, OccurredAt: r.SourceTime, ObservedAt: r.ObservedTime, Attribution: "producer_reported_visible_evidence", Text: r.Text, Attachments: []store.EpisodeAttachment{}, Gaps: []string{}}
	if r.Tool != "" || r.Kind == "tool_request" || r.Kind == "tool_result" {
		event.Gaps = append(event.Gaps, "tool_identity_and_pairing_not_archived")
	}
	for _, a := range r.Attachments {
		event.Attachments = append(event.Attachments, store.EpisodeAttachment{ID: a.ID, MediaType: a.MediaType, Data: a.Bytes, SHA256: a.SHA256})
	}
	for _, g := range r.Gaps {
		event.Gaps = append(event.Gaps, g.Reason)
	}
	// A command identifies an occurrence, not its content, so a changed retry
	// conflicts instead of silently becoming another accepted command.
	command := "queue-" + store.EpisodeEventKey(r.Binding.Producer, r.Conversation, r.Event)
	in := store.EpisodeInput{Operation: "append", ProducerID: r.Binding.Producer, ConversationID: r.Conversation, CommandID: command, Host: "normalized-visible", HostVersion: "1", WorkspaceLabel: r.Workspace, PolicyVersion: r.Binding.Policy, Events: []store.EpisodeEvent{event}, Gaps: []string{"synthetic_qualification_only", "host_coverage_not_verified"}}
	if err := store.ValidateEpisodeInput(in); err != nil {
		return store.EpisodeInput{}, ErrUnavailable
	}
	return in, nil
}

// Publish acknowledges the local queue only after checking the server's stable
// command/content receipt and exact event reference. Ambiguous responses leave
// the queued item intact for an identical retry; no new identity is minted.
func (c *Client) Publish(ctx context.Context, r episodeclient.Record, generation uint64) (episodeclient.Receipt, error) {
	key := episodeclient.ReplayKey{Binding: r.Binding, Workspace: r.Workspace, Conversation: r.Conversation, Event: r.Event}
	if !c.allowed(ctx, key) {
		return episodeclient.Receipt{}, ErrUnavailable
	}
	in, err := mutation(r)
	if err != nil {
		return episodeclient.Receipt{}, err
	}
	out, err := c.call(ctx, "append", &generation, &in)
	if err != nil {
		return episodeclient.Receipt{}, err
	}
	receipt := out.Receipt
	if *out.Generation != generation || receipt == nil || receipt.CommandKey != store.EpisodeCommandKey(in.ProducerID, in.CommandID) || receipt.ConversationKey != store.EpisodeConversationKey(in.ProducerID, in.ConversationID) || receipt.Digest != store.EpisodeHash(in) || !store.ValidSourceSHA(receipt.SourceSHA256) || receipt.RecordedAt.IsZero() || len(receipt.EventRefs) != 1 || receipt.EventRefs[0].EventKey != store.EpisodeEventKey(in.ProducerID, in.ConversationID, r.Event) || receipt.EventRefs[0].Digest != store.EpisodeHash(in.Events[0]) {
		return episodeclient.Receipt{}, ErrReceipt
	}
	return episodeclient.Receipt{Key: key, Digest: episodeclient.DigestRecord(r), Complete: true}, nil
}
