package supersede

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// CompactionResult reports the approved pass, not a new sweep on every retry.
type CompactionResult struct {
	Archived        int  `json:"archived"`
	Entities        int  `json:"entities"`
	AlreadyComplete bool `json:"already_complete,omitempty"`
}

type compactPublication struct {
	Version  int                 `json:"version"`
	Decision string              `json:"decision"`
	Complete bool                `json:"complete"`
	Result   CompactionResult    `json:"result"`
	Changes  []writer.FileChange `json:"changes"`
}

func compactDecision(item disposition.Item) (string, error) {
	if item.Kind != disposition.KindCompact || item.State != disposition.StateDisposed || (item.Verdict != disposition.VerdictAccept && item.Verdict != disposition.VerdictEditAccept) || !strings.HasPrefix(item.Actor, "human:") || item.Actor == "human:" || item.DisposedAt.IsZero() {
		return "", fmt.Errorf("compact: accepted human compact decision required")
	}
	payload := item.Payload
	if len(item.EditedPayload) > 0 {
		payload = item.EditedPayload
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(payload, &object) != nil || object == nil || len(object) != 0 {
		return "", fmt.Errorf("compact: only the approved empty global scope is supported")
	}
	raw, err := json.Marshal(struct {
		ID              string
		Verdict         disposition.Verdict
		Actor           string
		At              time.Time
		Payload, Edited json.RawMessage
	}{item.ID, item.Verdict, item.Actor, item.DisposedAt, item.Payload, item.EditedPayload})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// ApplyAndCommitCompaction persists exact before/after bytes before publishing
// an approved global pass. A retry finishes that pass and never sweeps new data.
func ApplyAndCommitCompaction(ctx context.Context, root string, ds *disposition.Store, item disposition.Item, now time.Time) (CompactionResult, error) {
	var result CompactionResult
	if _, err := compactDecision(item); err != nil {
		return result, err
	}
	runtime, err := openPublicationRuntime(root)
	if err != nil {
		return result, err
	}
	defer func() { _ = runtime.Close() }()
	lock, err := lockPublication(runtime)
	if err != nil {
		return result, err
	}
	defer func() { _ = lock.Close() }()
	item, err = ds.Get(ctx, item.ID)
	if err != nil {
		return result, err
	}
	decision, err := compactDecision(item)
	if err != nil {
		return result, err
	}
	if item.AppliedPublicationID != "" && item.AppliedPublicationID != decision {
		return result, fmt.Errorf("compact: completion marker differs from recorded approval")
	}
	sum := sha256.Sum256([]byte(item.ID))
	name := "compact-" + hex.EncodeToString(sum[:]) + ".json"
	if info, err := runtime.Lstat(name); err == nil && !info.Mode().IsRegular() {
		return result, fmt.Errorf("compact: unsafe receipt")
	} else if err != nil && !os.IsNotExist(err) {
		return result, err
	}
	var receipt compactPublication
	raw, err := runtime.ReadFile(name)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &receipt); err != nil {
			return result, fmt.Errorf("compact: corrupt receipt: %w", err)
		}
		if receipt.Version != 1 || receipt.Decision != decision {
			return result, fmt.Errorf("compact: receipt differs from recorded approval")
		}
	case errors.Is(err, fs.ErrNotExist):
		if item.AppliedPublicationID != "" {
			return result, fmt.Errorf("compact: completed receipt missing; do not repeat an old approval")
		}
		changes, planned, err := planCompaction(root)
		if err != nil {
			return result, err
		}
		receipt = compactPublication{Version: 1, Decision: decision, Result: planned, Changes: changes}
		if err := savePublication(runtime, name, receipt); err != nil {
			return result, err
		}
	default:
		return result, err
	}
	result = receipt.Result
	result.AlreadyComplete = receipt.Complete
	if !receipt.Complete {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		q := writer.NewQueue(nil)
		defer q.Close()
		if err := writer.PublishCompactionFiles(q, root, receipt.Changes); err != nil {
			return result, err
		}
		if _, err := writer.Flush(q, root); err != nil {
			return result, fmt.Errorf("compact: publication awaits commit; retry compact --item %s: %w", item.ID, err)
		}
		receipt.Complete = true
		if err := savePublication(runtime, name, receipt); err != nil {
			return result, err
		}
	}
	return result, ds.RecordCompactionID(ctx, item.ID, decision, now)
}

func planCompaction(root string) ([]writer.FileChange, CompactionResult, error) {
	var result CompactionResult
	dir, err := os.OpenRoot(root)
	if err != nil {
		return nil, result, err
	}
	defer func() { _ = dir.Close() }()
	for _, path := range []string{"brain", "brain/claims"} {
		info, err := dir.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, result, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, result, fmt.Errorf("compact: unsafe claims directory")
		}
	}
	var paths []string
	base := filepath.Join(root, "brain", "claims")
	err = filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
		if os.IsNotExist(walkErr) && path == base {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("compact: symlink in claims tree")
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, result, err
	}
	snapshot, err := writer.SnapshotFiles(root, paths)
	if err != nil {
		return nil, result, err
	}
	changes, err := writer.PlanCompactionFiles(root, snapshot, func(preview string) error {
		ss := store.NewShardStore(preview)
		slugs, err := ss.Slugs()
		if err != nil {
			return err
		}
		result.Entities = len(slugs)
		for _, slug := range slugs {
			families, err := ss.Families(slug)
			if err != nil {
				return err
			}
			for _, family := range families {
				moved, err := ss.Compact(slug, family)
				if err != nil {
					return fmt.Errorf("compact %s/%s: %w", slug, family, err)
				}
				result.Archived += moved
			}
		}
		return nil
	})
	return changes, result, err
}
