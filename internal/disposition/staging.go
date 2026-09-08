package disposition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// CreateOnce stages an immutable producer input once. The caller's key identifies
// the semantic proposal, not the attempt time. A replay returns the stored item
// with its current review state; it never resets a rejection or adds history.
func (s *Store) CreateOnce(ctx context.Context, kind Kind, payload json.RawMessage, groupID, key string, now time.Time) (Item, bool, error) {
	if key == "" {
		return Item{}, false, fmt.Errorf("disposition: staging requires a nonempty input key")
	}
	identity, err := json.Marshal([]string{"serenity-stage-v1", string(kind), groupID, key})
	if err != nil {
		return Item{}, false, err
	}
	sum := sha256.Sum256(identity)
	id := hex.EncodeToString(sum[:16])
	item := Item{ID: id, Kind: kind, State: StatePending, GroupID: groupID, Payload: payload, CreatedAt: now.UTC(), UpdatedAt: now.UTC()}
	raw, err := json.Marshal(item)
	if err != nil {
		return Item{}, false, err
	}
	inserted, err := s.backend.InsertDispositionItem(ctx, id, raw)
	if err != nil {
		return Item{}, false, err
	}
	if inserted {
		return item, true, nil
	}
	prior, err := s.Get(ctx, id)
	if err != nil {
		return Item{}, false, err
	}
	if prior.Kind != kind || prior.GroupID != groupID {
		return Item{}, false, fmt.Errorf("disposition: staging identity does not match existing item")
	}
	return prior, false, nil
}
