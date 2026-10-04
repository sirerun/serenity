package gateway

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/hosted/pool"
)

// InspectorReadView is a short-lived, fenced view of one already-owned brain.
// The callback must use it only for bounded, read-only canonical-file reads.
type InspectorReadView struct {
	Root string
	Now  time.Time
}

var inspectorReadSlots = make(chan struct{}, 4)

var ErrInspectorBrainUnavailable = errors.New("hosted: inspector brain is unavailable")
var ErrInspectorReadCapacity = errors.New("hosted: inspector read capacity reached")

// WithInspectorRead holds the same maintenance, account, and runtime mutation
// fences used by hosted writes and deletion while a read-only inspector query
// runs. A cold brain is read directly from its validated canonical directory;
// it is never opened through Pool.Acquire, which can initialize or recover it.
func (g *Gateway) WithInspectorRead(ctx context.Context, accountID, brainID string, read func(InspectorReadView) error) error {
	if g == nil || g.Issuer == nil || g.Issuer.Store == nil || read == nil {
		return errors.New("hosted: inspector read is unavailable")
	}
	select {
	case inspectorReadSlots <- struct{}{}:
		defer func() { <-inspectorReadSlots }()
	case <-ctx.Done():
		return ctx.Err()
	default:
		return ErrInspectorReadCapacity
	}
	g.Maintenance.RLock()
	defer g.Maintenance.RUnlock()

	h := sha256.Sum256([]byte(accountID))
	accountLock := &g.accountLocks[int(h[0])%len(g.accountLocks)]
	accountLock.Lock()
	defer accountLock.Unlock()

	var status string
	if err := g.Issuer.Store.DB().QueryRowContext(ctx, `SELECT status FROM accounts WHERE id=?`, accountID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInspectorBrainUnavailable
		}
		return err
	}
	if status != "active" {
		return ErrInspectorBrainUnavailable
	}

	var storedID, storedAccount, state, pathKey string
	err := g.Issuer.Store.DB().QueryRowContext(ctx, `SELECT id,account_id,state,path_key FROM brains WHERE id=? AND account_id=? AND deleted_at IS NULL`, brainID, accountID).Scan(&storedID, &storedAccount, &state, &pathKey)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInspectorBrainUnavailable
	}
	if err != nil {
		return err
	}
	if storedID != brainID || storedAccount != accountID || state != "ready" || pathKey != brainID || !validInspectorBrainID(pathKey) {
		return ErrInspectorBrainUnavailable
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if g.Pool == nil {
		return errors.New("hosted: inspector runtime pool is unavailable")
	}
	root, err := canonicalInspectorBrainRoot(g.Pool.BrainsRoot(), pathKey)
	if err != nil {
		return err
	}

	runtime, release, acquireErr := g.Pool.AcquireExistingForRead(ctx, brainID)
	if acquireErr == nil {
		defer release()
		if runtime == nil || !sameInspectorPath(runtime.Root, root) {
			return errors.New("hosted: inspector runtime path does not match its owner")
		}
		runtime.Mutations.Lock()
		defer runtime.Mutations.Unlock()
	} else if !errors.Is(acquireErr, pool.ErrNotOpen) {
		return fmt.Errorf("hosted: inspector runtime unavailable: %w", acquireErr)
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	return read(InspectorReadView{Root: root, Now: time.Now().UTC()})
}

func canonicalInspectorBrainRoot(brainsRoot, pathKey string) (string, error) {
	if !validInspectorBrainID(pathKey) || strings.TrimSpace(brainsRoot) == "" {
		return "", errors.New("hosted: inspector brain path is invalid")
	}
	rootAbs, err := filepath.Abs(brainsRoot)
	if err != nil {
		return "", err
	}
	rootInfo, err := os.Lstat(rootAbs)
	if err != nil {
		return "", err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("hosted: inspector brain root is unsafe")
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", err
	}
	brainPath := filepath.Join(rootReal, pathKey)
	if filepath.Dir(brainPath) != rootReal {
		return "", errors.New("hosted: inspector brain path escapes its root")
	}
	brainInfo, err := os.Lstat(brainPath)
	if err != nil {
		return "", err
	}
	if !brainInfo.IsDir() || brainInfo.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("hosted: inspector brain directory is unsafe")
	}
	brainReal, err := filepath.EvalSymlinks(brainPath)
	if err != nil {
		return "", err
	}
	if filepath.Dir(brainReal) != rootReal || brainReal != brainPath {
		return "", errors.New("hosted: inspector brain directory is not canonical")
	}
	return brainReal, nil
}

func sameInspectorPath(runtimeRoot, canonicalRoot string) bool {
	resolved, err := filepath.EvalSymlinks(runtimeRoot)
	return err == nil && resolved == canonicalRoot
}

func validInspectorBrainID(id string) bool {
	if len(id) < 16 || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}
