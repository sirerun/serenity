package authority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// WitnessTip is independently stored in S3. The exact table identity is part
// of the fence: restoring an old DynamoDB table cannot restore write authority.
type WitnessTip struct {
	TotalBytes int64  `json:"total_bytes"`
	Lineage    string `json:"lineage"`
	TableARN   string `json:"table_arn"`
	Sequence   uint64 `json:"sequence"`
	Key        string `json:"key"`
	SHA256     string `json:"sha256"`
}

type ImmutableItem struct {
	Key  string `json:"key"`
	Body []byte `json:"body"`
}

type Pending struct {
	OwnerRebind   bool       `json:"owner_rebind"`
	OperationID   string     `json:"operation_id"`
	RequestSHA256 string     `json:"request_sha256"`
	Principal     string     `json:"principal"`
	Previous      WitnessTip `json:"previous"`
	Next          WitnessTip `json:"next"`
	Witness       []byte     `json:"witness"`
	Response      Response   `json:"response"`
	Bootstrap     bool       `json:"bootstrap"`
}

// Record stores the durable protocol state and at most one unfinished witness.
// Revision increments once per logical transaction, not for witness completion.
type Record struct {
	Revision uint64          `json:"revision"`
	State    json.RawMessage `json:"state"`
	Tip      WitnessTip      `json:"tip"`
	Pending  *Pending        `json:"pending,omitempty"`
}

type Receipt struct {
	OperationID   string   `json:"operation_id"`
	RequestSHA256 string   `json:"request_sha256"`
	Principal     string   `json:"principal"`
	Response      Response `json:"response"`
	Complete      bool     `json:"complete"`
}

// AtomicStore must atomically CAS the state and create all immutable items and
// the operation receipt. A provider timeout is never a negative existence proof.
type AtomicStore interface {
	ListObjects(context.Context, string, string, int) ([]ImmutableItem, bool, error)
	Import(context.Context, []ImmutableItem) error
	Load(context.Context) (Record, bool, error)
	Receipt(context.Context, string) (Receipt, bool, error)
	Commit(context.Context, uint64, Record, []ImmutableItem, Receipt) error
	Complete(context.Context, Record) error
	Get(context.Context, string) ([]byte, bool, error)
	List(context.Context, string, string, int) ([]string, bool, error)
}

// WitnessStore implements immutable witness PUT plus independent TIP CAS.
// Advance must compare the exact previous tip, not just whatever ETag is latest.
type WitnessStore interface {
	Tip(context.Context) (WitnessTip, bool, error)
	Advance(context.Context, Pending) error
	Read(context.Context, WitnessTip) ([]byte, error)
}

type Kernel struct {
	Store    AtomicStore
	Witness  WitnessStore
	Lineage  string
	TableARN string
}

type witnessRecord struct {
	Version  int             `json:"version"`
	Previous WitnessTip      `json:"previous"`
	Sequence uint64          `json:"sequence"`
	Lineage  string          `json:"lineage"`
	TableARN string          `json:"table_arn"`
	Revision uint64          `json:"revision"`
	State    json.RawMessage `json:"state"`
	Items    []ImmutableItem `json:"items"`
	Receipt  Receipt         `json:"receipt"`
}

func digest(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

func (k *Kernel) validate() error {
	if k.Store == nil || k.Witness == nil || k.Lineage == "" || k.TableARN == "" {
		return ErrInvalid
	}
	return nil
}

// Resolve completes a committed transaction before any subsequent transition.
// It never permits deletion side effects before the independent witness CAS.
func (k *Kernel) Resolve(ctx context.Context, r Record) (Record, error) {
	if err := k.validate(); err != nil {
		return Record{}, err
	}
	if r.Pending == nil {
		return r, nil
	}
	p := *r.Pending
	if p.Next.Lineage != k.Lineage || p.Next.TableARN != k.TableARN || digest(p.Witness) != p.Next.SHA256 {
		return Record{}, ErrConflict
	}
	if err := k.Witness.Advance(ctx, p); err != nil {
		return Record{}, fmt.Errorf("witness advance: %w", err)
	}
	if err := k.Store.Complete(ctx, r); err != nil {
		// A lost acknowledgement can already have completed. Strongly read it back.
		receipt, found, readErr := k.Store.Receipt(ctx, p.OperationID)
		if readErr != nil || !found || !receipt.Complete || receipt.RequestSHA256 != p.RequestSHA256 || receipt.Principal != p.Principal {
			return Record{}, fmt.Errorf("witness completion: %w", ErrUnavailable)
		}
	}
	latest, found, err := k.Store.Load(ctx)
	if err != nil || !found {
		return Record{}, ErrUnavailable
	}
	return latest, nil
}

// Current proves the local state matches the independent effective tip. It
// detects restored older tables even when there is no pending mutation.
func (k *Kernel) Current(ctx context.Context) (Record, error) {
	if err := k.validate(); err != nil {
		return Record{}, err
	}
	r, found, err := k.Store.Load(ctx)
	if err != nil {
		return Record{}, err
	}
	if !found {
		return Record{}, ErrForbidden
	}
	for attempt := 0; r.Pending != nil && attempt < 4; attempt++ {
		r, err = k.Resolve(ctx, r)
		if err != nil {
			return Record{}, err
		}
	}
	if r.Pending != nil {
		return Record{}, ErrUnavailable
	}
	tip, exists, err := k.Witness.Tip(ctx)
	if err != nil {
		return Record{}, err
	}
	if !exists || tip != r.Tip || tip.Lineage != k.Lineage || tip.TableARN != k.TableARN {
		return Record{}, ErrConflict
	}
	return r, nil
}

// Apply executes one validated state transition. Validation and authorization
// are the handler responsibility; serialization and durability are enforced here.
func (k *Kernel) Apply(ctx context.Context, old Record, state json.RawMessage, items []ImmutableItem, receipt Receipt, bootstrap bool) (Response, error) {
	if err := k.validate(); err != nil {
		return Response{}, err
	}
	if !validID(receipt.OperationID) || receipt.Principal == "" || !validID(receipt.RequestSHA256) || !json.Valid(state) || old.Pending != nil {
		return Response{}, ErrInvalid
	}
	prior, found, err := k.Store.Receipt(ctx, receipt.OperationID)
	if err != nil {
		return Response{}, err
	}
	if found {
		if prior.RequestSHA256 != receipt.RequestSHA256 || prior.Principal != receipt.Principal {
			return Response{}, ErrConflict
		}
		if !prior.Complete {
			r, ok, e := k.Store.Load(ctx)
			if e != nil || !ok {
				return Response{}, ErrUnavailable
			}
			if _, e = k.Resolve(ctx, r); e != nil {
				return Response{}, e
			}
			prior, found, err = k.Store.Receipt(ctx, receipt.OperationID)
			if err != nil || !found || !prior.Complete {
				return Response{}, ErrUnavailable
			}
		}
		return prior.Response, nil
	}
	if bootstrap {
		if old.Revision != 0 || old.Tip != (WitnessTip{}) {
			return Response{}, ErrConflict
		}
	} else {
		tip, ok, e := k.Witness.Tip(ctx)
		if e != nil {
			return Response{}, e
		}
		if !ok || tip != old.Tip || tip.Lineage != k.Lineage || tip.TableARN != k.TableARN {
			return Response{}, ErrConflict
		}
	}
	if len(state) > MaxObjectBytes || len(items) > 90 {
		return Response{}, ErrInvalid
	}
	for _, item := range items {
		if item.Key == "" || len(item.Key) > 512 || len(item.Body) > MaxObjectBytes {
			return Response{}, ErrInvalid
		}
	}
	receipt.Complete = false
	w := witnessRecord{Version: 1, Previous: old.Tip, Sequence: old.Tip.Sequence + 1, Lineage: k.Lineage, TableARN: k.TableARN, Revision: old.Revision + 1, State: state, Items: items, Receipt: receipt}
	body, err := json.Marshal(w)
	if err != nil {
		return Response{}, err
	}
	if len(body) > 4*MaxBodyBytes {
		return Response{}, ErrInvalid
	}
	tip := WitnessTip{Lineage: k.Lineage, TableARN: k.TableARN, Sequence: w.Sequence, SHA256: digest(body)}
	tip.TotalBytes = old.Tip.TotalBytes + int64(len(body))
	if tip.Sequence > 99000 || tip.TotalBytes > 63*1024*1024 {
		return Response{}, ErrInvalid
	}
	tip.Key = fmt.Sprintf("%020d/%s.json", tip.Sequence, tip.SHA256)
	next := Record{Revision: old.Revision + 1, State: bytes.Clone(state), Tip: old.Tip, Pending: &Pending{OperationID: receipt.OperationID, RequestSHA256: receipt.RequestSHA256, Principal: receipt.Principal, Previous: old.Tip, Next: tip, Witness: body, Response: receipt.Response, Bootstrap: bootstrap}}
	if err = k.Store.Commit(ctx, old.Revision, next, items, receipt); err != nil {
		// Cancellation/conflict/timeout is ambiguous until the exact durable receipt
		// is read. A different request never borrows another transaction success.
		p, ok, e := k.Store.Receipt(ctx, receipt.OperationID)
		if e != nil || !ok {
			return Response{}, fmt.Errorf("commit unresolved: %w", err)
		}
		if p.RequestSHA256 != receipt.RequestSHA256 || p.Principal != receipt.Principal {
			return Response{}, ErrConflict
		}
		if p.Complete {
			return p.Response, nil
		}
		next, ok, e = k.Store.Load(ctx)
		if e != nil || !ok {
			return Response{}, ErrUnavailable
		}
	}
	if _, err = k.Resolve(ctx, next); err != nil {
		return Response{}, err
	}
	return receipt.Response, nil
}

func validID(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
