package runtimebroker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/sirerun/serenity/internal/hosted/authority"
)

const MaxInventoryEntries = 100000
const MaxInventoryBytes int64 = 2 * 1024 * 1024 * 1024

type inventoryRecord struct {
	Path   string `json:"path"`
	Mode   uint32 `json:"mode"`
	UID    uint32 `json:"uid"`
	GID    uint32 `json:"gid"`
	Kind   string `json:"kind"`
	Bytes  int64  `json:"bytes,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

type inventoryWalker struct {
	ctx     context.Context
	encode  *json.Encoder
	entries int
	bytes   int64
}

// InventorySHA256 describes the complete stopped data tree. The caller must
// hold its local ownership lock and prevent concurrent writers. Root-confined
// descriptors, nofollow opens and before/after identity checks reject redirected
// or changing files. Only the named genuine admin socket and an empty regular
// ownership lock are ephemeral; every other entry is bound or rejected.
func InventorySHA256(ctx context.Context, dir string) (string, error) {
	if ctx == nil || !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return "", errors.New("broker: canonical inventory directory required")
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("broker: real inventory directory required")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer func() { _ = root.Close() }()
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		return "", errors.Join(err, errors.New("broker: inventory root changed"))
	}
	h := sha256.New()
	w := inventoryWalker{ctx: ctx, encode: json.NewEncoder(h)}
	if err := w.walk(root, "", 0); err != nil {
		return "", err
	}
	after, err := os.Lstat(dir)
	if err != nil || !sameInventoryInfo(info, after) {
		return "", errors.Join(err, errors.New("broker: inventory root changed"))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (w *inventoryWalker) record(r inventoryRecord, info fs.FileInfo) error {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("broker: inventory custody unavailable")
	}
	r.UID, r.GID = st.Uid, st.Gid
	w.entries++
	if w.entries > MaxInventoryEntries {
		return errors.New("broker: inventory entry limit exceeded")
	}
	return w.encode.Encode(r)
}

func sameInventoryInfo(a, b fs.FileInfo) bool {
	return a != nil && b != nil && os.SameFile(a, b) && a.Mode() == b.Mode() &&
		a.Size() == b.Size() && a.ModTime().Equal(b.ModTime()) && sameInventoryCustody(a, b)
}

func (w *inventoryWalker) walk(root *os.Root, prefix string, depth int) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if depth > 64 {
		return errors.New("broker: inventory depth limit exceeded")
	}
	before, err := root.Stat(".")
	if err != nil {
		return err
	}
	if err := w.record(inventoryRecord{Path: prefix, Mode: uint32(before.Mode()), Kind: "directory"}, before); err != nil {
		return err
	}
	directory, err := root.OpenFile(".", os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	remaining := MaxInventoryEntries - w.entries
	entries, readErr := directory.ReadDir(remaining + 1)
	closeErr := directory.Close()
	if (readErr != nil && !errors.Is(readErr, io.EOF)) || closeErr != nil {
		return errors.Join(readErr, closeErr)
	}
	if len(entries) > remaining {
		return errors.New("broker: inventory entry limit exceeded")
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if err := w.ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		path := name
		if prefix != "" {
			path = prefix + "/" + name
		}
		info, err := root.Lstat(name)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("broker: symlink in inventory: %s", path)
		}
		if path == ".hosted-admin.sock" && info.Mode()&os.ModeSocket != 0 {
			continue
		}
		if path == ".serenity/writer.lock" && info.Mode().IsRegular() && info.Size() == 0 {
			f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
			if err != nil {
				return err
			}
			opened, statErr := f.Stat()
			closeErr := f.Close()
			if statErr != nil || closeErr != nil || !sameInventoryInfo(info, opened) {
				return errors.Join(statErr, closeErr, errors.New("broker: ownership lock changed"))
			}
			continue
		}
		if info.IsDir() {
			child, err := root.OpenRoot(name)
			if err != nil {
				return err
			}
			opened, statErr := child.Stat(".")
			if statErr != nil || !sameInventoryInfo(info, opened) {
				return errors.Join(statErr, child.Close(), errors.New("broker: inventory directory redirected"))
			}
			walkErr := w.walk(child, path, depth+1)
			closeErr := child.Close()
			after, statErr := root.Lstat(name)
			if walkErr != nil || closeErr != nil || statErr != nil || !sameInventoryInfo(info, after) {
				return errors.Join(walkErr, closeErr, statErr, errors.New("broker: inventory directory changed"))
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("broker: special file in inventory: %s", path)
		}
		if st, ok := info.Sys().(*syscall.Stat_t); !ok || st.Nlink != 1 {
			return fmt.Errorf("broker: hardlink in inventory: %s", path)
		}
		if info.Size() < 0 || info.Size() > MaxInventoryBytes-w.bytes {
			return errors.New("broker: inventory byte limit exceeded")
		}
		f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
		if err != nil {
			return err
		}
		opened, statErr := f.Stat()
		if statErr != nil || !sameInventoryInfo(info, opened) {
			return errors.Join(statErr, f.Close(), errors.New("broker: inventory file redirected"))
		}
		hash := sha256.New()
		n, readErr := io.Copy(hash, io.LimitReader(inventoryContextReader{ctx: w.ctx, r: f}, info.Size()+1))
		after, statErr := f.Stat()
		closeErr := f.Close()
		pathAfter, pathErr := root.Lstat(name)
		if readErr != nil || statErr != nil || closeErr != nil || pathErr != nil || n != info.Size() ||
			!sameInventoryInfo(info, after) || !sameInventoryInfo(info, pathAfter) {
			return errors.Join(readErr, statErr, closeErr, pathErr, errors.New("broker: inventory file changed"))
		}
		w.bytes += n
		if err := w.record(inventoryRecord{Path: path, Mode: uint32(info.Mode()), Kind: "file", Bytes: n, SHA256: hex.EncodeToString(hash.Sum(nil))}, info); err != nil {
			return err
		}
	}
	after, err := root.Stat(".")
	if err != nil || !sameInventoryInfo(before, after) {
		return errors.Join(err, errors.New("broker: inventory directory changed"))
	}
	return nil
}

func requireInitialData(ctx context.Context, dir string, a *authority.Adoption) error {
	if a == nil {
		return requireEmptyData(dir)
	}
	if err := a.Validate(); err != nil {
		return err
	}
	actual, err := InventorySHA256(ctx, dir)
	if err != nil {
		return err
	}
	if actual != a.InventorySHA256 {
		return errors.New("broker: data differs from owner adoption checkpoint")
	}
	return nil
}

type inventoryContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r inventoryContextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(b)
}

func brokerInitialInventory(ctx context.Context, c Config, a *authority.Adoption) (string, error) {
	if a != nil {
		digest, err := a.SHA256()
		if err != nil || c.AdoptionSHA256 == "" || digest != c.AdoptionSHA256 {
			return "", errors.New("broker: owner adoption binding mismatch")
		}
	}
	if err := requireInitialData(ctx, c.DataDir, a); err != nil {
		return "", err
	}
	if a != nil {
		return a.InventorySHA256, nil
	}
	return "", nil
}

func rejectRuntimeIdentity(q authority.Request) error {
	if q.Measurement != nil || q.Adoption != nil || q.DataInventorySHA256 != "" {
		return authority.ErrForbidden
	}
	return nil
}
