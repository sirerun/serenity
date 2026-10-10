package authority

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/sirerun/serenity/internal/hosted/deletion"
)

// restore is an owner-only table migration. It reads the independent effective
// tip itself. A runtime-provided snapshot, watermark or PITR time cannot choose
// which deletion history survives. The destination remains unavailable until
// the exact old tip is atomically rebound to the new table and acknowledged.
func (s *Server) restore(ctx context.Context, principal string, q Request, requestHash string) (Response, error) {
	if principal != s.config.OwnerPrincipal {
		return Response{}, ErrForbidden
	}
	_, exists, err := s.kernel.Store.Load(ctx)
	if err != nil {
		return Response{}, err
	}
	if exists {
		return Response{}, ErrConflict
	}
	previous, found, err := s.kernel.Witness.Tip(ctx)
	if err != nil {
		return Response{}, err
	}
	if !found || previous.Lineage != s.kernel.Lineage || previous.TableARN == s.kernel.TableARN {
		return Response{}, ErrConflict
	}
	history, err := History(ctx, s.kernel.Witness, previous, 100000, 64*1024*1024)
	if err != nil {
		return Response{}, err
	}
	if len(history) == 0 {
		return Response{}, ErrConflict
	}
	imported := map[string][]byte{}
	var latest witnessRecord
	for _, body := range history {
		var w witnessRecord
		if err = strictDecode(body, &w); err != nil {
			return Response{}, err
		}
		for _, item := range w.Items {
			if item.Key == "META" || len(item.Key) > 512 || len(item.Body) > MaxObjectBytes {
				return Response{}, ErrConflict
			}
			if existing, ok := imported[item.Key]; ok && string(existing) != string(item.Body) {
				return Response{}, ErrConflict
			}
			imported[item.Key] = item.Body
		}
		receipt := w.Receipt
		receipt.Complete = true
		if !validID(receipt.OperationID) || !validID(receipt.RequestSHA256) {
			return Response{}, ErrConflict
		}
		encoded, e := json.Marshal(receipt)
		if e != nil {
			return Response{}, e
		}
		key := "OP#" + receipt.OperationID
		if existing, ok := imported[key]; ok && string(existing) != string(encoded) {
			return Response{}, ErrConflict
		}
		imported[key] = encoded
		latest = w
	}
	var st protocolState
	if err = strictDecode(latest.State, &st); err != nil {
		return Response{}, err
	}
	st.Phase = "CLOSED"
	st.Epoch++
	st.Prepared = nil
	st.Lease = nil
	st.Measurement = Measurement{}
	keys := make([]string, 0, len(imported))
	for key := range imported {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	items := make([]ImmutableItem, 0, len(keys))
	for _, key := range keys {
		items = append(items, ImmutableItem{Key: key, Body: imported[key]})
	}
	if err = s.kernel.Store.Import(ctx, items); err != nil {
		return Response{}, err
	}
	reader, err := deletion.NewJournalReader(&boundedReader{store: s.kernel.Store, position: st.Position, strict: true}, st.Position)
	if err != nil {
		return Response{}, err
	}
	observation, err := reader.Observe(ctx)
	if err != nil {
		return Response{}, err
	}
	for _, use := range observation.WriterUses {
		b, found, e := s.kernel.Store.Get(ctx, "ISS#"+use.WriterID)
		if e != nil {
			return Response{}, e
		}
		if !found {
			return Response{}, ErrConflict
		}
		var issued issuance
		if e = strictDecode(b, &issued); e != nil || issued.WriterID != use.WriterID || issued.Generation != use.Generation {
			return Response{}, ErrConflict
		}
	}
	state, err := json.Marshal(st)
	if err != nil {
		return Response{}, err
	}
	receipt := Receipt{OperationID: q.OperationID, RequestSHA256: requestHash, Principal: principal}
	w := witnessRecord{Version: 1, Previous: previous, Sequence: previous.Sequence + 1, Lineage: s.kernel.Lineage, TableARN: s.kernel.TableARN, Revision: latest.Revision + 1, State: state, Receipt: receipt}
	body, err := json.Marshal(w)
	if err != nil {
		return Response{}, err
	}
	next := WitnessTip{Lineage: s.kernel.Lineage, TableARN: s.kernel.TableARN, Sequence: w.Sequence, SHA256: digest(body)}
	next.TotalBytes = previous.TotalBytes + int64(len(body))
	if next.Sequence > 100000 || next.TotalBytes > 64*1024*1024 {
		return Response{}, ErrInvalid
	}
	next.Key = fmt.Sprintf("%020d/%s.json", next.Sequence, next.SHA256)
	record := Record{Revision: w.Revision, State: state, Tip: previous, Pending: &Pending{OperationID: q.OperationID, RequestSHA256: requestHash, Principal: principal, Previous: previous, Next: next, Witness: body, OwnerRebind: true}}
	if err = s.kernel.Store.Commit(ctx, 0, record, nil, receipt); err != nil {
		r, ok, e := s.kernel.Store.Receipt(ctx, q.OperationID)
		if e != nil || !ok || r.Principal != principal || r.RequestSHA256 != requestHash {
			return Response{}, ErrUnavailable
		}
		record, ok, e = s.kernel.Store.Load(ctx)
		if e != nil || !ok {
			return Response{}, ErrUnavailable
		}
	}
	if _, err = s.kernel.Resolve(ctx, record); err != nil {
		return Response{}, err
	}
	return Response{}, nil
}

// Enroll creates a first-ever authority lineage through the separately
// authenticated owner endpoint. Empty journal storage alone is insufficient.
func (c *Client) Enroll(ctx context.Context, op string) error {
	_, err := c.invoke(ctx, Request{Action: "enroll", OperationID: op})
	return err
}

// RestoreAuthority reconstructs a new table from the independent witness tip.
// The client must use the owner transport; the runtime endpoint rejects it.
func (c *Client) RestoreAuthority(ctx context.Context, op string) error {
	_, err := c.invoke(ctx, Request{Action: "restore", OperationID: op})
	return err
}
