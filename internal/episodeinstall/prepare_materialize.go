package episodeinstall

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	"github.com/sirerun/serenity/internal/gitrun"
)

type prepareOutput struct {
	buffer bytes.Buffer
	limit  int64
}

func (b *prepareOutput) Write(raw []byte) (int, error) {
	if int64(len(raw)) > b.limit-int64(b.buffer.Len()) {
		return 0, ErrUnqualified
	}
	return b.buffer.Write(raw)
}

type prepareGit struct{ total int64 }

func (g *prepareGit) read(ctx context.Context, r *gitrun.Runner, input []byte, limit int64, args ...string) ([]byte, error) {
	if ctx.Err() != nil || limit < 0 || limit > maxSourceBytes {
		return nil, ErrUnqualified
	}
	cmd, err := r.Command(ctx, args...)
	if err != nil {
		return nil, ErrUnqualified
	}
	output := &prepareOutput{limit: limit}
	stderr := &prepareOutput{limit: 4096}
	cmd.Stdout = output
	cmd.Stderr = stderr
	cmd.Stdin = bytes.NewReader(input)
	if cmd.Run() != nil {
		return nil, ErrUnqualified
	}
	g.total += int64(output.buffer.Len())
	if g.total > maxSourceBytes {
		return nil, ErrUnqualified
	}
	return output.buffer.Bytes(), nil
}

func prepareMaterialize(ctx context.Context, g *prepareGit, original, candidate, replacement, head string) error {
	if os.Mkdir(replacement, 0700) != nil {
		return ErrUnqualified
	}
	runner := gitrun.Quarantine(replacement)
	if _, err := g.read(ctx, runner, nil, 4096, "init", "--object-format=sha1", "--initial-branch=main", "--template="); err != nil {
		return ErrUnqualified
	}
	bare := filepath.Join(candidate, "repository.git")
	pack, err := g.read(ctx, gitrun.Quarantine(bare), nil, maxSourceBytes, "pack-objects", "--stdout", "--all")
	if err != nil {
		return ErrUnqualified
	}
	if _, err = g.read(ctx, runner, pack, 4096, "index-pack", "--stdin"); err != nil {
		return ErrUnqualified
	}
	if _, err = g.read(ctx, runner, nil, 4096, "update-ref", "refs/heads/main", head); err != nil {
		return ErrUnqualified
	}
	if _, err = g.read(ctx, runner, nil, 4096, "read-tree", "--reset", "-u", head); err != nil {
		return ErrUnqualified
	}
	// BuildHosted already qualified the exact owned hosted config. Keep its
	// eligible identity/options, rather than adding fetch/remotes/borrowed state.
	configPath := filepath.Join(original, ".git", "config")
	inv, err := inventoryPath(ctx, configPath)
	if err != nil || len(inv) != 1 || inv[0].Kind != "file" || inv[0].Bytes > 64<<10 {
		return ErrUnqualified
	}
	raw, err := os.ReadFile(configPath)
	if err != nil || int64(len(raw)) != inv[0].Bytes || hashBytes(raw) != inv[0].SHA256 {
		return ErrUnqualified
	}
	target := filepath.Join(replacement, ".git", "config")
	info, err := os.Lstat(target)
	if err != nil || !info.Mode().IsRegular() || !ownedInfo(info) || !singleLink(info) {
		return ErrUnqualified
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return ErrUnqualified
	}
	opened, statErr := file.Stat()
	if statErr != nil || !os.SameFile(info, opened) {
		_ = file.Close()
		return ErrUnqualified
	}
	_, writeErr := file.Write(raw)
	modeErr := file.Chmod(os.FileMode(inv[0].Mode))
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || modeErr != nil || syncErr != nil || closeErr != nil {
		return ErrUnqualified
	}
	candidateObjects, err := g.read(ctx, gitrun.CanonicalReadOnly(bare), nil, 16<<20, "cat-file", "--batch-all-objects", "--batch-check=%(objectname) %(objecttype)", "--unordered")
	if err != nil {
		return ErrUnqualified
	}
	replacementObjects, err := g.read(ctx, gitrun.CanonicalReadOnly(replacement), nil, 16<<20, "cat-file", "--batch-all-objects", "--batch-check=%(objectname) %(objecttype)", "--unordered")
	if err != nil || !prepareSameObjectSet(candidateObjects, replacementObjects) {
		return ErrUnqualified
	}
	return syncTree(ctx, replacement)
}

func prepareSameObjectSet(a, b []byte) bool {
	// Object enumeration order may differ between loose and packed stores.
	left, right := bytes.FieldsFunc(a, func(r rune) bool { return r == '\n' }), bytes.FieldsFunc(b, func(r rune) bool { return r == '\n' })
	if len(left) != len(right) {
		return false
	}
	objects := make(map[string]bool, len(left))
	for _, item := range left {
		key := string(item)
		if objects[key] {
			return false
		}
		objects[key] = true
	}
	for _, item := range right {
		key := string(item)
		if !objects[key] {
			return false
		}
		delete(objects, key)
	}
	return len(objects) == 0
}

func preparePrivateFamily(ctx context.Context, directory string) error {
	entries, err := boundedDirectoryEntries(ctx, directory, 3)
	if err != nil || len(entries) > 3 {
		return ErrUnqualified
	}
	for _, entry := range entries {
		if entry.Name() != "index.db" && entry.Name() != "index.db-wal" && entry.Name() != "index.db-shm" {
			return ErrUnqualified
		}
		path := filepath.Join(directory, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || !ownedInfo(info) || !singleLink(info) {
			return ErrUnqualified
		}
		if os.Chmod(path, 0600) != nil {
			return ErrUnqualified
		}
	}
	return nil
}

func prepareOperationBounds(ctx context.Context, root string) error {
	_, err := inventoryPathLimits(ctx, root, maxOperationEntries, maxOperationBytes, func(f *os.File, n int) ([]os.DirEntry, error) { return f.ReadDir(n) })
	if err != nil {
		return ErrUnqualified
	}
	return nil
}
