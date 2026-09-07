package direction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/index"
)

// RevisitCondition is an opt-in deterministic alternative revisit condition.
type RevisitCondition = index.RevisitCondition

// ParseRevisitCondition leaves ordinary dira prose unsupported, not guessed.
func ParseRevisitCondition(raw string) (RevisitCondition, bool, error) {
	var c RevisitCondition
	switch {
	case strings.HasPrefix(raw, "after:"):
		at, err := time.Parse(time.RFC3339, strings.TrimPrefix(raw, "after:"))
		if err != nil {
			return c, true, fmt.Errorf("revisit after: %w", err)
		}
		c.After = at
		c.Timed = true
	case strings.HasPrefix(raw, "claim_state_changed:"):
		parts := strings.Split(strings.TrimPrefix(raw, "claim_state_changed:"), "/")
		if len(parts) != 3 {
			return c, true, fmt.Errorf("revisit claim key requires subject/predicate/object_key")
		}
		for _, part := range parts {
			v, err := url.PathUnescape(part)
			if err != nil || v == "" {
				return c, true, fmt.Errorf("revisit invalid claim key component %q", part)
			}
			c.Key = append(c.Key, v)
		}
	default:
		return c, false, nil
	}
	return c, true, nil
}

// RevisitRequest carries immutable source text and one evaluated condition.
type RevisitRequest = index.RevisitRequest

// RevisitBackend atomically observes claims and checkpoints a review delivery.
type RevisitBackend interface {
	Revisit(context.Context, RevisitRequest) (bool, error)
}

// RevisitResult reports unsupported prose explicitly to callers.
type RevisitResult struct {
	Created     int `json:"created"`
	Unsupported int `json:"unsupported"`
}

// SweepRevisit reads accepted/active entries without ever mutating the ledger.
func SweepRevisit(ctx context.Context, store ledger.Store, backend RevisitBackend, now time.Time) (RevisitResult, error) {
	var result RevisitResult
	entries, err := store.List(ctx)
	if err != nil {
		return result, fmt.Errorf("revisit list: %w", err)
	}
	var requests []RevisitRequest
	for _, info := range entries {
		entry, err := store.Get(ctx, info.ID)
		if err != nil {
			return result, fmt.Errorf("revisit read %s: %w", info.ID, err)
		}
		if entry.State != ledger.StateActive && entry.State != ledger.StateAccepted {
			continue
		}
		for i, alt := range entry.Alternatives {
			if alt.RevisitIf == "" {
				continue
			}
			condition, supported, err := ParseRevisitCondition(alt.RevisitIf)
			if err != nil {
				return result, fmt.Errorf("revisit %s alternative %d: %w", entry.ID, i, err)
			}
			if !supported {
				result.Unsupported++
				continue
			}
			key := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s", entry.ID, i, alt.RevisitIf)))
			payload, err := json.Marshal(struct {
				Action      string             `json:"action"`
				EntryID     string             `json:"entry_id"`
				Title       string             `json:"title"`
				Body        string             `json:"body"`
				Alternative ledger.Alternative `json:"alternative"`
			}{"review", entry.ID, entry.Title, entry.Body, alt})
			if err != nil {
				return result, fmt.Errorf("revisit payload: %w", err)
			}
			requests = append(requests, RevisitRequest{ID: "revisit:" + hex.EncodeToString(key[:]), Condition: condition, Now: now.UTC(),
				NewItem: func(id string, at time.Time) ([]byte, error) {
					return json.Marshal(disposition.Item{ID: id, Kind: disposition.KindPreceptDraft, State: disposition.StatePending, Payload: payload, CreatedAt: at, UpdatedAt: at})
				},
				Outstanding: func(data []byte) (bool, error) {
					var item disposition.Item
					if err := json.Unmarshal(data, &item); err != nil {
						return false, err
					}
					return item.State != disposition.StateDisposed, nil
				}})
		}
	}
	// Parse all conditions first so a malformed condition cannot partially sweep.
	for _, request := range requests {
		created, err := backend.Revisit(ctx, request)
		if err != nil {
			return result, fmt.Errorf("revisit %s: %w", request.ID, err)
		}
		if created {
			result.Created++
		}
	}
	return result, nil
}
