package direction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/writer"
)

type ledgerPublication struct {
	Version      int               `json:"version"`
	Decision     string            `json:"decision"`
	EntryID      string            `json:"entry_id"`
	Entry        []byte            `json:"entry"`
	Dependencies map[string][]byte `json:"dependencies,omitempty"`
	Complete     bool              `json:"complete"`
}

// PreviewDisposition checks a proposed acceptance without disposing it or
// changing the ledger. The real lifecycle runs only in an isolated preview.
func (s *Store) PreviewDisposition(ctx context.Context, item disposition.Item, actor string, now time.Time) error {
	if err := s.writable("preview publication"); err != nil {
		return err
	}
	if item.State == disposition.StateDisposed {
		return fmt.Errorf("direction: decision already disposed")
	}
	item.State = disposition.StateDisposed
	item.Verdict = disposition.VerdictAccept
	item.Actor = actor
	item.DisposedAt = now
	_, err := s.preparePublication(ctx, item)
	return err
}

func acceptedLedgerDecision(item disposition.Item) error {
	if (item.Kind != disposition.KindPreceptDraft && item.Kind != disposition.KindDecompose) || item.State != disposition.StateDisposed || (item.Verdict != disposition.VerdictAccept && item.Verdict != disposition.VerdictEditAccept) {
		return fmt.Errorf("direction: accepted precept draft or child intent required")
	}
	if !strings.HasPrefix(item.Actor, "human:") || item.Actor == "human:" || item.DisposedAt.IsZero() {
		return fmt.Errorf("direction: recorded human actor and decision time required")
	}
	return nil
}

// ApplyAndCommitDisposition saves an exact entry and its dependencies before
// canonical publication. It retries the recorded decision without allocating a
// second entry, and marks the inbox only after Git commit and receipt completion.
func (s *Store) ApplyAndCommitDisposition(ctx context.Context, ds *disposition.Store, item disposition.Item, now time.Time) (*ledger.Entry, error) {
	if err := s.writable("publish disposition"); err != nil {
		return nil, err
	}
	runtime, err := openDirectories(s.root, []string{".serenity", ".serenity/direction"}, true)
	if err != nil {
		return nil, err
	}
	defer func() { _ = runtime.Close() }()
	lock, err := lockPublication(runtime)
	if err != nil {
		return nil, err
	}
	defer func() { _ = lock.Close() }()
	item, err = ds.Get(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	if err := acceptedLedgerDecision(item); err != nil {
		return nil, err
	}
	decision, err := json.Marshal(struct {
		ID              string
		Kind            disposition.Kind
		Verdict         disposition.Verdict
		Actor           string
		At              time.Time
		Payload, Edited json.RawMessage
	}{item.ID, item.Kind, item.Verdict, item.Actor, item.DisposedAt, item.Payload, item.EditedPayload})
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(decision)
	fingerprint := hex.EncodeToString(sum[:])
	idSum := sha256.Sum256([]byte(item.ID))
	name := hex.EncodeToString(idSum[:]) + ".json"
	if _, err := regularEntry(runtime, name, true); err != nil {
		return nil, err
	}
	raw, err := runtime.ReadFile(name)
	var plan ledgerPublication
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &plan); err != nil {
			return nil, fmt.Errorf("direction: corrupt publication receipt: %w", err)
		}
		if plan.Version != 1 || plan.Decision != fingerprint || !ledger.ValidID(plan.EntryID) {
			return nil, fmt.Errorf("direction: publication receipt does not match decision")
		}
	case os.IsNotExist(err):
		if item.AppliedEntryID != "" {
			return nil, fmt.Errorf("direction: completed entry marker exists but its publication receipt is missing")
		}
		if !item.LedgerEffectPending {
			return nil, fmt.Errorf("direction: legacy ledger acceptance has no recoverable publication intent; inspect existing entries before resolving it")
		}
		plan, err = s.preparePublication(ctx, item)
		if err != nil {
			return nil, err
		}
		plan.Version = 1
		plan.Decision = fingerprint
		if err := saveLedgerPublication(runtime, name, plan); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}
	entry, err := ledger.Decode(plan.Entry)
	if err != nil {
		return nil, err
	}
	if entry.ID != plan.EntryID || (item.AppliedEntryID != "" && item.AppliedEntryID != entry.ID) {
		return nil, fmt.Errorf("direction: publication entry identity mismatch")
	}
	if !plan.Complete {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		q := writer.NewQueue(nil)
		defer q.Close()
		result := q.Submit(writer.Job{Path: s.PathFor(entry.ID), Render: func() ([]byte, error) {
			entries, err := s.openEntries(true)
			if err != nil {
				return nil, err
			}
			defer func() { _ = entries.Close() }()
			for id, before := range plan.Dependencies {
				name, err := entryName(id)
				if err != nil {
					return nil, err
				}
				current, _, err := readEntry(entries, name)
				if err != nil {
					return nil, err
				}
				if !bytes.Equal(current, before) {
					return nil, fmt.Errorf("direction: parent %s changed since approval was prepared", id)
				}
			}
			name, err := entryName(entry.ID)
			if err != nil {
				return nil, err
			}
			info, err := regularEntry(entries, name, true)
			if err != nil {
				return nil, err
			}
			if info != nil {
				current, _, err := readEntry(entries, name)
				if err != nil {
					return nil, err
				}
				if !bytes.Equal(current, plan.Entry) {
					return nil, fmt.Errorf("direction: reserved entry %s contains newer or unrelated content", entry.ID)
				}
			} else if err := writeEntry(entries, name, plan.Entry, true); err != nil {
				return nil, err
			}
			return plan.Entry, nil
		}})
		if result.Err != nil {
			return nil, result.Err
		}
		if _, err := writer.Flush(q, s.root); err != nil {
			return nil, fmt.Errorf("direction: ledger publication awaits commit: %w", err)
		}
		plan.Complete = true
		if err := saveLedgerPublication(runtime, name, plan); err != nil {
			return nil, err
		}
	}
	if err := ds.RecordEntryID(ctx, item.ID, entry.ID, now); err != nil {
		return nil, err
	}
	return entry, nil
}

func saveLedgerPublication(root *os.Root, name string, plan ledgerPublication) error {
	raw, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	return writeLedgerFile(root, name, raw, false, 0600)
}

func (s *Store) preparePublication(ctx context.Context, item disposition.Item) (ledgerPublication, error) {
	if err := acceptedLedgerDecision(item); err != nil {
		return ledgerPublication{}, err
	}
	// Verify Git even for a first precept with no existing ledger dependencies.
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = s.root
	output, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(output)) != "true" {
		return ledgerPublication{}, fmt.Errorf("direction: publication requires a Git brain repository")
	}
	infos, err := s.List(ctx)
	if err != nil {
		return ledgerPublication{}, err
	}
	snapshot := map[string][]byte{}
	if len(infos) > 0 {
		entries, err := s.openEntries(false)
		if err != nil {
			return ledgerPublication{}, err
		}
		defer func() { _ = entries.Close() }()
		for _, info := range infos {
			name, err := entryName(info.ID)
			if err != nil {
				return ledgerPublication{}, err
			}
			raw, _, err := readEntry(entries, name)
			if err != nil {
				return ledgerPublication{}, err
			}
			e, err := ledger.Decode(raw)
			if err != nil {
				return ledgerPublication{}, err
			}
			if e.ID != info.ID {
				return ledgerPublication{}, fmt.Errorf("direction: ledger filename/identity mismatch")
			}
			snapshot[info.ID] = raw
		}
	}
	dependencies := map[string][]byte{}
	if item.Kind == disposition.KindDecompose {
		raw := item.Payload
		if item.Verdict == disposition.VerdictEditAccept && len(item.EditedPayload) > 0 {
			raw = item.EditedPayload
		}
		var payload DecomposePayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			return ledgerPublication{}, err
		}
		if !ledger.ValidID(payload.ParentID) || snapshot[payload.ParentID] == nil {
			return ledgerPublication{}, fmt.Errorf("direction: child intent requires an existing local parent")
		}
		parent, err := ledger.Decode(snapshot[payload.ParentID])
		if err != nil {
			return ledgerPublication{}, err
		}
		if parent.Kind != ledger.KindIntent || parent.State != ledger.StateActive {
			return ledgerPublication{}, fmt.Errorf("direction: child intent requires an active parent intent")
		}
		dependencies[parent.ID] = snapshot[parent.ID]
	}
	runtime, err := openDirectories(s.root, []string{".serenity", ".serenity/direction"}, true)
	if err != nil {
		return ledgerPublication{}, err
	}
	defer func() { _ = runtime.Close() }()
	preview, err := os.MkdirTemp(runtime.Name(), "plan-")
	if err != nil {
		return ledgerPublication{}, err
	}
	defer func() { _ = os.RemoveAll(preview) }()
	entriesDir := filepath.Join(preview, ".dira", "entries")
	if err := os.MkdirAll(entriesDir, 0700); err != nil {
		return ledgerPublication{}, err
	}
	for id, raw := range snapshot {
		if err := os.WriteFile(filepath.Join(entriesDir, id+".md"), raw, 0600); err != nil {
			return ledgerPublication{}, err
		}
	}
	q := writer.NewQueue(nil)
	defer q.Close()
	planned := NewStore(preview, q)
	var entry *ledger.Entry
	if item.Kind == disposition.KindPreceptDraft {
		entry, err = planned.ApplyDisposedPreceptDraft(ctx, item, item.DisposedAt)
	} else {
		entry, err = planned.ApplyDisposedDecompose(ctx, item, item.DisposedAt)
	}
	if err != nil {
		return ledgerPublication{}, err
	}
	entry.Created = item.DisposedAt.UTC().Format(time.RFC3339Nano)
	if entry.Updated != "" {
		entry.Updated = entry.Created
	}
	raw, err := ledger.Encode(entry)
	if err != nil {
		return ledgerPublication{}, err
	}
	for id, before := range dependencies {
		entries, err := s.openEntries(false)
		if err != nil {
			return ledgerPublication{}, err
		}
		name := id + ".md"
		current, _, readErr := readEntry(entries, name)
		_ = entries.Close()
		if readErr != nil {
			return ledgerPublication{}, readErr
		}
		if !bytes.Equal(current, before) {
			return ledgerPublication{}, fmt.Errorf("direction: parent changed during publication planning")
		}
		cmd := exec.CommandContext(ctx, "git", "--literal-pathspecs", "status", "--porcelain", "--", filepath.Join(".dira", "entries", name))
		cmd.Dir = s.root
		status, err := cmd.Output()
		if err != nil {
			return ledgerPublication{}, err
		}
		if len(bytes.TrimSpace(status)) != 0 {
			return ledgerPublication{}, fmt.Errorf("direction: parent %s has uncommitted human changes", id)
		}
	}
	return ledgerPublication{EntryID: entry.ID, Entry: raw, Dependencies: dependencies}, nil
}
