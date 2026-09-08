package gbrain

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
)

type pageCheckpoint struct {
	SourceSHA256 string            `json:"source_sha256"`
	Target       string            `json:"target"`
	TargetSHA256 string            `json:"target_sha256"`
	Rows         map[string]string `json:"rows"`
}
type checkpoint struct {
	Version int                       `json:"version"`
	Pages   map[string]pageCheckpoint `json:"pages"`
}

// Observers are an internal seam for subprocess crash tests, not an environment
// variable or kill switch in the shipped CLI.
type importObserver func(stage, page string) error

func observe(fn importObserver, stage, page string) error {
	if fn != nil {
		return fn(stage, page)
	}
	return nil
}
func contentHash(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

// openRuntime confines checkpoint operations below .serenity/import. Symlink
// components are rejected, even if their destinations would stay in the brain.
func openRuntime(target string) (*os.Root, error) {
	root, err := os.OpenRoot(target)
	if err != nil {
		return nil, err
	}
	for _, part := range []string{".serenity", "import"} {
		if err := root.Mkdir(part, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			_ = root.Close()
			return nil, err
		}
		info, err := root.Lstat(part)
		if err != nil {
			_ = root.Close()
			return nil, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			_ = root.Close()
			return nil, fmt.Errorf("gbrain checkpoint: unsafe directory %s", part)
		}
		next, err := root.OpenRoot(part)
		_ = root.Close()
		if err != nil {
			return nil, err
		}
		root = next
	}
	return root, nil
}

func lockRuntime(root *os.Root) (*os.File, error) {
	if info, err := root.Lstat("gbrain.lock"); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("gbrain checkpoint: lock is not a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	f, err := root.OpenFile("gbrain.lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockImport(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	// A previous SIGKILL may leave only an unpublished runtime temp file.
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".gbrain-checkpoint-") {
			if !entry.Type().IsRegular() {
				_ = f.Close()
				return nil, fmt.Errorf("gbrain checkpoint: unsafe temporary entry")
			}
			if err := root.Remove(entry.Name()); err != nil {
				_ = f.Close()
				return nil, err
			}
		}
	}
	return f, nil
}

func loadCheckpoint(root *os.Root) (checkpoint, error) {
	cp := checkpoint{Version: 1, Pages: map[string]pageCheckpoint{}}
	info, err := root.Lstat("gbrain.json")
	if errors.Is(err, os.ErrNotExist) {
		return cp, nil
	}
	if err != nil {
		return cp, err
	}
	if !info.Mode().IsRegular() {
		return cp, fmt.Errorf("gbrain checkpoint: state is not a regular file")
	}
	raw, err := root.ReadFile("gbrain.json")
	if err != nil {
		return cp, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cp); err != nil {
		return cp, fmt.Errorf("gbrain checkpoint: decode: %w", err)
	}
	if dec.Decode(new(any)) != io.EOF || cp.Version != 1 || cp.Pages == nil {
		return cp, fmt.Errorf("gbrain checkpoint: invalid version or shape")
	}
	return cp, nil
}

func saveCheckpoint(ctx context.Context, root *os.Root, cp checkpoint, observer importObserver, page string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	name := ".gbrain-checkpoint-" + rand.Text()
	f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(name) }()
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := observe(observer, "checkpoint_staged", page); err != nil {
		return err
	}
	if err := root.Rename(name, "gbrain.json"); err != nil {
		return fmt.Errorf("gbrain checkpoint: publish: %w", err)
	}
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	err = dir.Sync()
	closeErr := dir.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return observe(observer, "checkpoint_published", page)
}

// Completed rows are advisory until canonical bytes AND the committed git
// version match. A forged or stale checkpoint cannot hide undurable work.
func checkpointComplete(ctx context.Context, target string, root *os.Root, actual, expected pageCheckpoint) (bool, error) {
	if !reflect.DeepEqual(actual, expected) {
		return false, fmt.Errorf("gbrain checkpoint: source or mapping changed for %s; resume the original snapshot or use a fresh target", expected.Target)
	}
	raw, err := root.ReadFile(expected.Target)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if contentHash(raw) != expected.TargetSHA256 {
		return false, fmt.Errorf("gbrain checkpoint: canonical page changed: %s", expected.Target)
	}
	cmd := exec.CommandContext(ctx, "git", "show", "HEAD:"+filepath.ToSlash(expected.Target))
	cmd.Dir = target
	committed, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, nil
	}
	return bytes.Equal(raw, committed), nil
}
