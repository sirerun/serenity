package releaseauth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Paths are operator supplied; neither an archive nor manifest selects trust files.
type Paths struct {
	Archive, Manifest, Signature, Policy, Approval, State, Tag string
	LockFD                                                     int
}
type Transaction struct {
	paths     Paths
	lock      *os.File
	closeLock bool
}
type Result struct {
	ManifestSHA256 string `json:"manifest_sha256"`
	ArchiveSHA256  string `json:"archive_sha256"`
	Sequence       uint64 `json:"sequence"`
}

type verified struct {
	manifest   Manifest
	canonical  []byte
	approval   Approval
	state      State
	stateBytes []byte
	next       State
}

func rootInfo(info os.FileInfo, directory bool) error {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != 0 || info.Mode().Perm()&0022 != 0 || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("path must be root owned and not group/other writable")
	}
	if directory && !info.IsDir() || !directory && !info.Mode().IsRegular() {
		return fmt.Errorf("unexpected file type")
	}
	return nil
}

func safeParents(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("trust path must be clean and absolute")
	}
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		info, err := os.Lstat(dir)
		if err != nil {
			return fmt.Errorf("inspecting trust parent: %w", err)
		}
		if err = rootInfo(info, true); err != nil {
			return err
		}
		if dir == "/" {
			break
		}
	}
	return nil
}

func safeOpen(path string, flags int) (*os.File, error) {
	if err := safeParents(path); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(path); err == nil {
		if err = rootInfo(info, false); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	f, err := os.OpenFile(path, flags|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening trusted file: %w", err)
	}
	info, err := f.Stat()
	if err == nil {
		err = rootInfo(info, false)
	}
	if err == nil {
		var current os.FileInfo
		current, err = os.Lstat(path)
		if err == nil && !os.SameFile(info, current) {
			err = fmt.Errorf("trusted file changed during open")
		}
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func boundedRead(f *os.File, max int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("input exceeds size limit")
	}
	return b, nil
}
func ReadBounded(path string, max int64) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("input must be regular file")
	}
	return boundedRead(f, max)
}
func ReadTrusted(path string) ([]byte, error) {
	f, err := safeOpen(path, os.O_RDONLY)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return boundedRead(f, MaxDocument)
}

// OpenTransaction acquires or validates the deploy lock. The inherited FD is
// independently flocked, so merely naming an open FD never asserts ownership.
func OpenTransaction(p Paths) (*Transaction, error) {
	if os.Geteuid() != 0 {
		return nil, fmt.Errorf("production verification requires root")
	}
	if p.State == "" {
		return nil, fmt.Errorf("state path required")
	}
	f, err := safeOpen(p.State+".lock", os.O_CREATE|os.O_RDWR)
	if err != nil {
		return nil, err
	}
	closeLock := true
	if p.LockFD >= 0 {
		dup, e := syscall.Dup(p.LockFD)
		if e != nil {
			_ = f.Close()
			return nil, fmt.Errorf("duplicating lock descriptor: %w", e)
		}
		inherited := os.NewFile(uintptr(dup), "release-lock-"+strconv.Itoa(p.LockFD))
		info, e := inherited.Stat()
		other, oe := f.Stat()
		if e != nil || oe != nil || !os.SameFile(info, other) {
			_ = f.Close()
			_ = inherited.Close()
			return nil, fmt.Errorf("inherited lock does not match state lock")
		}
		if e = rootInfo(info, false); e != nil {
			_ = f.Close()
			_ = inherited.Close()
			return nil, e
		}
		if e = f.Close(); e != nil {
			_ = inherited.Close()
			return nil, e
		}
		f = inherited
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if closeLock {
			_ = f.Close()
		}
		return nil, fmt.Errorf("deployment lock unavailable: %w", err)
	}
	return &Transaction{paths: p, lock: f, closeLock: closeLock}, nil
}

func (t *Transaction) Close() error {
	if t.closeLock {
		return t.lock.Close()
	}
	return nil
}
func (t *Transaction) result(v verified) Result {
	return Result{Digest(v.canonical), v.manifest.ArchiveSHA256, v.manifest.Sequence}
}
func (t *Transaction) hasPending() error {
	_, err := os.Lstat(t.paths.State + ".pending")
	if err == nil {
		return fmt.Errorf("pending deployment requires exact recovery")
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

type recoveryContext struct {
	pendingSHA256           string
	installedManifestSHA256 string
}

func (t *Transaction) load(now time.Time, previous []byte, recovery *recoveryContext) (verified, error) {
	var v verified
	policyBytes, err := ReadTrusted(t.paths.Policy)
	if err != nil {
		return v, err
	}
	var p Policy
	if err = DecodeCanonical(policyBytes, &p); err != nil {
		return v, err
	}
	approvalBytes, err := ReadTrusted(t.paths.Approval)
	if err != nil {
		return v, err
	}
	explicitRecovery := false
	if err = DecodeCanonical(approvalBytes, &v.approval); err != nil {
		if recovery == nil {
			return v, err
		}
		var authorization RecoveryApproval
		if err = DecodeCanonical(approvalBytes, &authorization); err != nil {
			return v, err
		}
		if (authorization.Mode != "recover-promote" && authorization.Mode != "recover-rollback") ||
			authorization.PendingSHA256 != recovery.pendingSHA256 ||
			authorization.InstalledManifestSHA256 != recovery.installedManifestSHA256 ||
			authorization.InstalledManifestSHA256 != authorization.ManifestSHA256 {
			return v, fmt.Errorf("recovery approval does not bind exact pending and installed artifact")
		}
		if err = validWindow(authorization.IssuedAt, authorization.ExpiresAt, now); err != nil {
			return v, err
		}
		v.approval = authorization.Approval
		v.approval.Mode = strings.TrimPrefix(v.approval.Mode, "recover-")
		explicitRecovery = true
	}
	v.canonical, err = ReadBounded(t.paths.Manifest, MaxDocument)
	if err != nil {
		return v, err
	}
	if err = DecodeCanonical(v.canonical, &v.manifest); err != nil {
		return v, err
	}
	signature, err := ReadBounded(t.paths.Signature, MaxSignature)
	if err != nil {
		return v, err
	}
	manifestTime := now
	if explicitRecovery {
		// Only elapsed expiry is waived: the original manifest must have had a
		// valid bounded window at issuance and must never have been future-issued.
		issued, e := time.Parse(time.RFC3339, v.manifest.IssuedAt)
		if e != nil || issued.After(now) {
			return v, fmt.Errorf("invalid recovery manifest issuance")
		}
		manifestTime = issued
	}
	if err = VerifySignature(v.manifest, v.canonical, signature, p, manifestTime); err != nil {
		return v, err
	}
	archive, err := os.OpenFile(t.paths.Archive, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return v, err
	}
	defer func() { _ = archive.Close() }()
	info, err := archive.Stat()
	if err != nil {
		return v, err
	}
	if !info.Mode().IsRegular() {
		return v, fmt.Errorf("archive is not regular file")
	}
	if err = VerifyArchive(archive, v.manifest); err != nil {
		return v, err
	}
	v.stateBytes = previous
	if previous == nil {
		v.stateBytes, err = ReadTrusted(t.paths.State)
		if err != nil {
			return v, err
		}
	}
	if err = DecodeCanonical(v.stateBytes, &v.state); err != nil {
		return v, err
	}
	v.next, err = nextStateAt(v.manifest, v.canonical, v.approval, v.state, v.stateBytes, t.paths.Tag, now, manifestTime)
	return v, err
}

func (t *Transaction) Verify(now time.Time) (Result, error) {
	if err := t.hasPending(); err != nil {
		return Result{}, err
	}
	v, err := t.load(now, nil, nil)
	if err != nil {
		return Result{}, err
	}
	return t.result(v), nil
}

func syncDir(path string) error {
	f, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.Sync()
}
func writeExclusive(path string, data []byte) error {
	f, err := safeOpen(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return syncDir(path)
}
func replaceState(path string, data []byte) (resultErr error) {
	if err := safeParents(path); err != nil {
		return err
	}
	old, err := safeOpen(path, os.O_RDONLY)
	if err != nil {
		return err
	}
	if err = old.Close(); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".release-state-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if err := os.Remove(tmp); err != nil && !errors.Is(err, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, fmt.Errorf("removing temporary state: %w", err))
		}
	}()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	return syncDir(path)
}

func (t *Transaction) Prepare(now time.Time) (Result, error) {
	if err := t.hasPending(); err != nil {
		return Result{}, err
	}
	v, err := t.load(now, nil, nil)
	if err != nil {
		return Result{}, err
	}
	pending := Pending{PreviousState: v.state, Version: 1, PreviousStateSHA256: Digest(v.stateBytes), ManifestSHA256: Digest(v.canonical)}
	data, err := json.Marshal(pending)
	if err != nil {
		return Result{}, err
	}
	if err = writeExclusive(t.paths.State+".pending", data); err != nil {
		return Result{}, fmt.Errorf("persisting pending deployment: %w", err)
	}
	return t.result(v), nil
}

func (t *Transaction) finish(now time.Time, installedPin string, recovery bool) (Result, error) {
	pendingBytes, err := ReadTrusted(t.paths.State + ".pending")
	if err != nil {
		return Result{}, err
	}
	var pending Pending
	if err = DecodeCanonical(pendingBytes, &pending); err != nil {
		return Result{}, err
	}
	previous, err := json.Marshal(pending.PreviousState)
	if err != nil {
		return Result{}, err
	}
	if pending.Version != 1 || pending.PreviousStateSHA256 != Digest(previous) || !hex64.MatchString(pending.ManifestSHA256) {
		return Result{}, fmt.Errorf("invalid pending deployment")
	}
	if recovery && installedPin != pending.ManifestSHA256 {
		return Result{}, fmt.Errorf("recovery requires exact installed manifest pin")
	}
	var authorization *recoveryContext
	if recovery {
		authorization = &recoveryContext{pendingSHA256: Digest(pendingBytes), installedManifestSHA256: installedPin}
	}
	v, err := t.load(now, previous, authorization)
	if err != nil {
		return Result{}, err
	}
	if pending.ManifestSHA256 != Digest(v.canonical) {
		return Result{}, fmt.Errorf("pending manifest mismatch")
	}
	current, err := ReadTrusted(t.paths.State)
	if err != nil {
		return Result{}, err
	}
	next, err := json.Marshal(v.next)
	if err != nil {
		return Result{}, err
	}
	if !bytes.Equal(current, previous) {
		if !recovery || !bytes.Equal(current, next) {
			return Result{}, fmt.Errorf("state changed since pending deployment")
		}
	} else if err = replaceState(t.paths.State, next); err != nil {
		return Result{}, err
	}
	if err = os.Remove(t.paths.State + ".pending"); err != nil {
		return Result{}, err
	}
	if err = syncDir(t.paths.State); err != nil {
		return Result{}, err
	}
	return t.result(v), nil
}
func (t *Transaction) Commit(now time.Time) (Result, error) { return t.finish(now, "", false) }
func (t *Transaction) Recover(now time.Time, installedPin string) (Result, error) {
	return t.finish(now, installedPin, true)
}
