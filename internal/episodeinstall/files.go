package episodeinstall

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var gitPattern = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
var brainPattern = regexp.MustCompile(`^[A-Za-z0-9]{16,64}$`)
var accountPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func hashBytes(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func safeRelative(p string) bool {
	return p == "." || (p != "" && len(p) <= 512 && path.Clean(p) == p && !strings.ContainsAny(p, "\\\x00") && !strings.HasPrefix(p, "/") && p != ".." && !strings.HasPrefix(p, "../"))
}
func canonicalDirectory(p string, private bool) error {
	if !filepath.IsAbs(p) || filepath.Clean(p) != p {
		return ErrUnqualified
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil || resolved != p {
		return ErrUnqualified
	}
	info, err := os.Lstat(p)
	if err != nil || !info.IsDir() || !ownedInfo(info) || info.Mode().Perm()&0022 != 0 {
		return ErrUnqualified
	}
	if private && info.Mode().Perm() != 0700 {
		return ErrUnqualified
	}
	return nil
}
func validateRoutes(source, operation, operationID string) error {
	if !digestPattern.MatchString(operationID) || filepath.Base(operation) != operationID || !filepath.IsAbs(operation) || filepath.Clean(operation) != operation {
		return ErrUnqualified
	}
	if err := canonicalDirectory(source, false); err != nil {
		return err
	}
	if err := canonicalDirectory(filepath.Dir(operation), true); err != nil {
		return err
	}
	// Neither route may be a descendant of the other.
	for _, pair := range [][2]string{{source, operation}, {operation, source}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err != nil || rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..") {
			return ErrUnqualified
		}
	}
	return nil
}

func inventoryPath(ctx context.Context, name string) (inventory, error) {
	return inventoryPathBounded(ctx, name, maxSourceEntries, func(f *os.File, n int) ([]os.DirEntry, error) { return f.ReadDir(n) })
}
func inventoryPathBounded(ctx context.Context, name string, entryLimit int, readDir func(*os.File, int) ([]os.DirEntry, error)) (inventory, error) {
	return inventoryPathLimits(ctx, name, entryLimit, maxSourceBytes, readDir)
}
func inventoryPathLimits(ctx context.Context, name string, entryLimit int, byteLimit int64, readDir func(*os.File, int) ([]os.DirEntry, error)) (inventory, error) {
	if entryLimit < 1 || entryLimit > maxOperationEntries || byteLimit < 0 || byteLimit > maxOperationBytes || readDir == nil {
		return nil, ErrUnqualified
	}
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrUnqualified
	}
	info, err := os.Lstat(name)
	if os.IsNotExist(err) {
		return inventory{{Path: ".", Kind: "absent"}}, nil
	}
	if err != nil {
		return nil, ErrUnqualified
	}
	if !info.IsDir() {
		entry, err := inventoryFile(ctx, name, ".", info)
		if err != nil {
			return nil, err
		}
		return inventory{entry}, nil
	}
	if info.Mode()&os.ModeSymlink != 0 || !ownedInfo(info) || info.Mode().Perm()&0022 != 0 {
		return nil, ErrUnqualified
	}
	root, err := os.OpenRoot(name)
	if err != nil {
		return nil, ErrUnqualified
	}
	defer func() { _ = root.Close() }()
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		return nil, ErrUnqualified
	}
	var entries inventory
	var total int64
	var walk func(string) error
	walk = func(p string) error {
		if ctx.Err() != nil || !safeRelative(p) || len(entries) >= entryLimit {
			return ErrUnqualified
		}
		fi, err := root.Lstat(p)
		if err != nil || !ownedInfo(fi) || fi.Mode().Perm()&0022 != 0 || fi.Mode()&os.ModeSymlink != 0 {
			return ErrUnqualified
		}
		if !fi.IsDir() {
			if !fi.Mode().IsRegular() || !singleLink(fi) || fi.Size() < 0 || fi.Size() > byteLimit-total {
				return ErrUnqualified
			}
			f, err := root.Open(p)
			if err != nil {
				return ErrUnqualified
			}
			entry, err := inventoryOpened(ctx, f, p, fi)
			closeErr := f.Close()
			if err != nil || closeErr != nil {
				return ErrUnqualified
			}
			total += entry.Bytes
			entries = append(entries, entry)
			return nil
		}
		entries = append(entries, inventoryEntry{Path: p, Kind: "directory", Mode: uint32(fi.Mode().Perm())})
		directory, err := root.Open(p)
		if err != nil {
			return ErrUnqualified
		}
		defer func() { _ = directory.Close() }()
		opened, err := directory.Stat()
		if err != nil || !os.SameFile(fi, opened) {
			return ErrUnqualified
		}
		for {
			if ctx.Err() != nil {
				return ErrUnqualified
			}
			// Bounded batches avoid allocating an unbounded directory listing
			// before discovering the global source entry limit.
			batch := min(256, entryLimit-len(entries)+1)
			children, readErr := readDir(directory, batch)
			for _, child := range children {
				if err := walk(path.Join(p, child.Name())); err != nil {
					return err
				}
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				return ErrUnqualified
			}
		}
		after, err := root.Lstat(p)
		if err != nil || !os.SameFile(fi, after) || after.Mode() != fi.Mode() || !after.ModTime().Equal(fi.ModTime()) {
			return ErrUnqualified
		}
		return nil
	}
	err = walk(".")
	if err != nil {
		return nil, ErrUnqualified
	}
	after, err := os.Lstat(name)
	if err != nil || !os.SameFile(info, after) {
		return nil, ErrUnqualified
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}
func inventoryFile(ctx context.Context, name, p string, info os.FileInfo) (inventoryEntry, error) {
	if !info.Mode().IsRegular() || !ownedInfo(info) || !singleLink(info) || info.Mode().Perm()&0022 != 0 || info.Size() < 0 || info.Size() > maxSourceBytes {
		return inventoryEntry{}, ErrUnqualified
	}
	f, err := os.Open(name)
	if err != nil {
		return inventoryEntry{}, ErrUnqualified
	}
	defer func() { _ = f.Close() }()
	entry, err := inventoryOpened(ctx, f, p, info)
	if err != nil {
		return inventoryEntry{}, err
	}
	after, err := os.Lstat(name)
	if err != nil || !os.SameFile(info, after) {
		return inventoryEntry{}, ErrUnqualified
	}
	return entry, nil
}
func inventoryOpened(ctx context.Context, f *os.File, p string, before os.FileInfo) (inventoryEntry, error) {
	opened, err := f.Stat()
	if err != nil || !os.SameFile(before, opened) || !opened.Mode().IsRegular() {
		return inventoryEntry{}, ErrUnqualified
	}
	h := sha256.New()
	buf := make([]byte, 64<<10)
	var n int64
	for {
		if ctx.Err() != nil {
			return inventoryEntry{}, ErrUnqualified
		}
		nr, re := f.Read(buf)
		if nr > 0 {
			n += int64(nr)
			if n > before.Size() {
				return inventoryEntry{}, ErrUnqualified
			}
			_, _ = h.Write(buf[:nr])
		}
		if re == io.EOF {
			break
		}
		if re != nil {
			return inventoryEntry{}, ErrUnqualified
		}
	}
	after, err := f.Stat()
	if err != nil || n != before.Size() || after.Size() != before.Size() || after.Mode() != before.Mode() || !after.ModTime().Equal(before.ModTime()) || !singleLink(after) {
		return inventoryEntry{}, ErrUnqualified
	}
	return inventoryEntry{Path: p, Kind: "file", Mode: uint32(before.Mode().Perm()), Bytes: n, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}
func validInventory(inv inventory) bool {
	if len(inv) == 0 || len(inv) > maxSourceEntries || inv[0].Path != "." {
		return false
	}
	var total int64
	directories := make(map[string]bool, len(inv))
	for i, e := range inv {
		if !safeRelative(e.Path) || (i > 0 && e.Path <= inv[i-1].Path) || e.Mode > 0777 || e.Mode&0022 != 0 {
			return false
		}
		switch e.Kind {
		case "absent":
			if len(inv) != 1 || e.Mode != 0 || e.Bytes != 0 || e.SHA256 != "" {
				return false
			}
		case "directory":
			if e.Bytes != 0 || e.SHA256 != "" {
				return false
			}
		case "file":
			if e.Bytes < 0 || e.Bytes > maxSourceBytes-total || !digestPattern.MatchString(e.SHA256) {
				return false
			}
			total += e.Bytes
		default:
			return false
		}
		if i > 0 && !directories[path.Dir(e.Path)] {
			return false
		}
		if e.Kind == "directory" {
			directories[e.Path] = true
		}
	}
	return true
}
func verifyInventory(ctx context.Context, p string, want inventory) error {
	if !validInventory(want) {
		return ErrUnqualified
	}
	got, err := inventoryPath(ctx, p)
	if err != nil || !reflect.DeepEqual(got, want) {
		return ErrUnqualified
	}
	return nil
}
func copyInventory(ctx context.Context, source, destination string, want inventory) error {
	if !validInventory(want) || verifyInventory(ctx, source, want) != nil {
		return ErrUnqualified
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		return ErrUnqualified
	}
	if want[0].Kind == "absent" {
		return nil
	}
	sourceBase := source
	if want[0].Kind == "file" {
		sourceBase = filepath.Dir(source)
	}
	sourceRoot, rootErr := os.OpenRoot(sourceBase)
	if rootErr != nil {
		return ErrUnqualified
	}
	defer func() { _ = sourceRoot.Close() }()
	for _, e := range want {
		if ctx.Err() != nil {
			return ErrUnqualified
		}
		src := source
		dst := destination
		if e.Path != "." {
			src = filepath.Join(source, filepath.FromSlash(e.Path))
			dst = filepath.Join(destination, filepath.FromSlash(e.Path))
		}
		if e.Kind == "directory" {
			if os.Mkdir(dst, os.FileMode(e.Mode)|0700) != nil {
				return ErrUnqualified
			}
			continue
		}
		relative := e.Path
		if want[0].Kind == "file" {
			relative = filepath.Base(src)
		}
		before, statErr := sourceRoot.Lstat(relative)
		if statErr != nil || !before.Mode().IsRegular() || !ownedInfo(before) || !singleLink(before) || before.Size() != e.Bytes || uint32(before.Mode().Perm()) != e.Mode {
			return ErrUnqualified
		}
		in, err := sourceRoot.Open(relative)
		if err != nil {
			return ErrUnqualified
		}
		opened, statErr := in.Stat()
		if statErr != nil || !os.SameFile(before, opened) {
			_ = in.Close()
			return ErrUnqualified
		}
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			_ = in.Close()
			return ErrUnqualified
		}
		_, copyErr := io.Copy(out, io.LimitReader(contextReader{ctx: ctx, reader: in}, e.Bytes+1))
		syncErr := out.Sync()
		modeErr := out.Chmod(os.FileMode(e.Mode))
		closeErr := out.Close()
		inErr := in.Close()
		if copyErr != nil || syncErr != nil || modeErr != nil || closeErr != nil || inErr != nil {
			return ErrUnqualified
		}
	}
	// Restore directory modes after construction; preserve even empty directories.
	for i := len(want) - 1; i >= 0; i-- {
		e := want[i]
		if e.Kind == "directory" {
			dst := destination
			if e.Path != "." {
				dst = filepath.Join(destination, filepath.FromSlash(e.Path))
			}
			if os.Chmod(dst, os.FileMode(e.Mode)) != nil {
				return ErrUnqualified
			}
		}
	}
	if verifyInventory(ctx, source, want) != nil || verifyInventory(ctx, destination, want) != nil {
		return ErrUnqualified
	}
	return syncTree(ctx, destination)
}
func syncDirectory(p string) error {
	f, err := os.Open(p)
	if err != nil {
		return ErrUnqualified
	}
	err = f.Sync()
	ce := f.Close()
	if err != nil || ce != nil {
		return ErrUnqualified
	}
	return nil
}
func syncTree(ctx context.Context, p string) error {
	inv, err := inventoryPathLimits(ctx, p, maxOperationEntries, maxOperationBytes, func(f *os.File, n int) ([]os.DirEntry, error) { return f.ReadDir(n) })
	if err != nil {
		return err
	}
	if inv[0].Kind == "absent" {
		return ErrUnqualified
	}
	for i := len(inv) - 1; i >= 0; i-- {
		if ctx.Err() != nil {
			return ErrUnqualified
		}
		e := inv[i]
		name := p
		if e.Path != "." {
			name = filepath.Join(p, filepath.FromSlash(e.Path))
		}
		f, err := os.Open(name)
		if err != nil {
			return ErrUnqualified
		}
		se := f.Sync()
		ce := f.Close()
		if se != nil || ce != nil {
			return ErrUnqualified
		}
	}
	return syncDirectory(filepath.Dir(p))
}
func validateCanonicalLayout(ctx context.Context, source string) error {
	if ctx == nil || ctx.Err() != nil || canonicalDirectory(source, false) != nil {
		return ErrUnqualified
	}
	entries, err := boundedDirectoryEntries(ctx, source, 6)
	if err != nil {
		return ErrUnqualified
	}
	allowed := map[string]bool{".git": true, "brain": true, ".dira": true, "serenity.yml": true, ".gitignore": true, ".serenity": true}
	for _, e := range entries {
		if !allowed[e.Name()] || e.Type()&os.ModeSymlink != 0 {
			return ErrUnqualified
		}
	}
	return nil
}
func validateRuntime(ctx context.Context, source string, maintenance bool) error {
	if ctx == nil || ctx.Err() != nil {
		return ErrUnqualified
	}
	root := filepath.Join(source, ".serenity")
	if canonicalDirectory(root, false) != nil {
		return ErrUnqualified
	}
	entries, err := boundedDirectoryEntries(ctx, root, 6)
	if err != nil {
		return ErrUnqualified
	}
	allowed := map[string]bool{"writer.lock": true, "canonical-view.lock": true, "index.db": true, "index.db-wal": true, "index.db-shm": true}
	if maintenance {
		allowed["canonical-maintenance.json"] = true
	}
	for _, e := range entries {
		if !allowed[e.Name()] {
			return ErrUnqualified
		}
		fi, err := os.Lstat(filepath.Join(root, e.Name()))
		if err != nil || !fi.Mode().IsRegular() || !ownedInfo(fi) || !singleLink(fi) || fi.Mode().Perm() != 0600 {
			return ErrUnqualified
		}
	}
	return nil
}
func writeHeader(ctx context.Context, directory string, h preparedHeader) (string, error) {
	if ctx == nil || ctx.Err() != nil || canonicalDirectory(directory, true) != nil {
		return "", ErrUnqualified
	}
	raw, err := json.Marshal(h)
	if err != nil || len(raw)+1 > maxHeaderBytes {
		return "", ErrUnqualified
	}
	raw = append(raw, '\n')
	f, err := os.OpenFile(filepath.Join(directory, headerName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", ErrUnqualified
	}
	_, we := f.Write(raw)
	se := f.Sync()
	ce := f.Close()
	if we != nil || se != nil || ce != nil || syncDirectory(directory) != nil {
		return "", ErrUnqualified
	}
	return hashBytes(raw), nil
}
func loadHeader(ctx context.Context, ref PreparedRef) (preparedHeader, error) {
	var h preparedHeader
	if ctx == nil || ctx.Err() != nil || !digestPattern.MatchString(ref.HeaderSHA256) || validateRoutes(ref.Source, ref.Directory, ref.Identity.OperationID) != nil || canonicalDirectory(ref.Directory, true) != nil {
		return h, ErrUnqualified
	}
	name := filepath.Join(ref.Directory, headerName)
	info, err := os.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || !ownedInfo(info) || !singleLink(info) || info.Size() > maxHeaderBytes {
		return h, ErrUnqualified
	}
	f, err := os.Open(name)
	if err != nil {
		return h, ErrUnqualified
	}
	opened, statErr := f.Stat()
	if statErr != nil || !os.SameFile(info, opened) {
		_ = f.Close()
		return h, ErrUnqualified
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxHeaderBytes+1))
	after, afterErr := f.Stat()
	closeErr := f.Close()
	if err != nil || afterErr != nil || closeErr != nil || !os.SameFile(info, after) || after.Size() != info.Size() || after.Mode() != info.Mode() || !singleLink(after) || len(raw) > maxHeaderBytes || hashBytes(raw) != ref.HeaderSHA256 {
		return h, ErrUnqualified
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&h) != nil {
		return preparedHeader{}, ErrUnqualified
	}
	canonical, err := json.Marshal(h)
	canonical = append(canonical, '\n')
	if err != nil || !bytes.Equal(canonical, raw) {
		return preparedHeader{}, ErrUnqualified
	}
	if h.Schema != headerSchema || h.Identity != ref.Identity || h.SourceRouteSHA256 != hashBytes([]byte(ref.Source)) || h.OperationRouteSHA256 != hashBytes([]byte(ref.Directory)) || !accountPattern.MatchString(h.AccountID) || !brainPattern.MatchString(h.Identity.BrainID) || !gitPattern.MatchString(h.Identity.SourceHead) || !digestPattern.MatchString(h.Identity.FenceDigest) || h.Identity.Schema != "serenity.canonical-maintenance/v1" || h.Candidate.SourceHead != h.Identity.SourceHead || h.Candidate.BrainID != h.Identity.BrainID || h.Candidate.FenceDigest != h.Identity.FenceDigest || !gitPattern.MatchString(h.Candidate.CandidateHead) || len(h.Components) != len(componentNames) {
		return preparedHeader{}, ErrUnqualified
	}
	for i, c := range h.Components {
		if c.Name != componentNames[i] || !validInventory(c.Old) || !validInventory(c.New) {
			return preparedHeader{}, ErrUnqualified
		}
	}
	for _, inv := range []inventory{h.Original, h.OldIndex, h.Inspection, h.CandidateFiles} {
		if !validInventory(inv) {
			return preparedHeader{}, ErrUnqualified
		}
	}
	return h, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if r.ctx.Err() != nil {
		return 0, ErrUnqualified
	}
	return r.reader.Read(p)
}

func boundedDirectoryEntries(ctx context.Context, p string, limit int) ([]os.DirEntry, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrUnqualified
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, ErrUnqualified
	}
	defer func() { _ = f.Close() }()
	entries, err := f.ReadDir(limit + 1)
	if ctx.Err() != nil || (err != nil && err != io.EOF) || len(entries) > limit {
		return nil, ErrUnqualified
	}
	return entries, nil
}

func renameParent(p string) (*os.File, error) {
	if canonicalDirectory(p, false) != nil {
		return nil, ErrIncomplete
	}
	before, err := os.Lstat(p)
	if err != nil {
		return nil, ErrIncomplete
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, ErrIncomplete
	}
	opened, err := f.Stat()
	if err != nil || !opened.IsDir() || !os.SameFile(before, opened) {
		_ = f.Close()
		return nil, ErrIncomplete
	}
	return f, nil
}
