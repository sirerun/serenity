//go:build linux || darwin

package episodeclient

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"
)

const (
	MaxQueueItems    = 1024
	MaxQueueBytes    = 32 << 20
	maxMetadataBytes = 1 << 20
	maxScopes        = 1024
	maxFences        = 4096
)

// Options requires an existing trusted parent directory. OpenQueue creates only
// the final directory. Key must be 32 caller-owned bytes; no credential store is
// read. Use one dedicated directory/key/binding. Lost keys cannot be recovered.
type Options struct {
	Directory string
	Key       []byte
	Policy    Policy
	MaxItems  int
	MaxBytes  int
}
type Queue struct {
	mu                 sync.Mutex
	root               *os.Root
	lock               *os.File
	aead               cipher.AEAD
	aad                []byte
	policy             Policy
	maxItems, maxBytes int
	closed             bool
}
type Scope struct {
	Workspace    string `json:"workspace,omitempty"`
	Conversation string `json:"conversation,omitempty"`
}

func (s Scope) valid() bool {
	return (s.Workspace == "" || validID(s.Workspace)) && (s.Conversation == "" || validID(s.Conversation))
}
func (s Scope) covers(r Record) bool {
	return (s.Workspace == "" || s.Workspace == r.Workspace) && (s.Conversation == "" || s.Conversation == r.Conversation)
}

type queueState struct {
	Version         int             `json:"version"`
	Items           []Record        `json:"items"`
	Paused          []Scope         `json:"paused"`
	Generation      uint64          `json:"generation"`
	GenerationKnown bool            `json:"generation_known"`
	Fenced          map[string]bool `json:"fenced,omitempty"`
}

// QueueStats does not reveal text, identifiers, filenames or attachment bytes.
type QueueStats struct{ Items, Bytes, PausedScopes int }

func OpenQueue(o Options) (*Queue, error) {
	if len(o.Key) != 32 || !o.Policy.Binding.valid() || o.MaxItems < 1 || o.MaxItems > MaxQueueItems || o.MaxBytes < 1 || o.MaxBytes > MaxQueueBytes || len(o.Policy.Mappings) > maxScopes {
		return nil, ErrInvalid
	}
	for _, m := range o.Policy.Mappings {
		if !validID(m.Workspace) || !m.Binding.valid() {
			return nil, ErrInvalid
		}
	}
	if !filepath.IsAbs(o.Directory) || filepath.Clean(o.Directory) != o.Directory {
		return nil, ErrStorage
	}
	if err := checkParents(filepath.Dir(o.Directory)); err != nil {
		return nil, err
	}
	if err := os.Mkdir(o.Directory, 0700); err != nil && !os.IsExist(err) {
		return nil, ErrStorage
	}
	st, err := os.Lstat(o.Directory)
	if err != nil || !safeInfo(st, true) {
		return nil, ErrStorage
	}
	root, err := os.OpenRoot(o.Directory)
	if err != nil {
		return nil, ErrStorage
	}
	opened, err := root.Stat(".")
	if err != nil || !safeInfo(opened, true) || !os.SameFile(st, opened) {
		_ = root.Close()
		return nil, ErrStorage
	}
	// Persist creation of the queue directory itself, not just its contents.
	parent, err := os.Open(filepath.Dir(o.Directory))
	if err != nil {
		_ = root.Close()
		return nil, ErrStorage
	}
	syncErr := parent.Sync()
	closeErr := parent.Close()
	if syncErr != nil || closeErr != nil {
		_ = root.Close()
		return nil, ErrStorage
	}
	lock, err := root.OpenFile("queue.lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		_ = root.Close()
		return nil, ErrStorage
	}
	st, err = lock.Stat()
	if err != nil || !safeInfo(st, false) {
		_ = lock.Close()
		_ = root.Close()
		return nil, ErrStorage
	}
	block, err := aes.NewCipher(o.Key)
	if err != nil {
		_ = lock.Close()
		_ = root.Close()
		return nil, ErrInvalid
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		_ = lock.Close()
		_ = root.Close()
		return nil, ErrInvalid
	}
	aad, _ := json.Marshal(struct {
		Format  string
		Binding Binding
	}{"episodeclient.queue.v1", o.Policy.Binding})
	p := o.Policy
	p.Mappings = append([]Mapping(nil), p.Mappings...)
	q := &Queue{root: root, lock: lock, aead: aead, aad: aad, policy: p, maxItems: o.MaxItems, maxBytes: o.MaxBytes}
	err = q.withLock(func() error {
		// A stale pending file is encrypted crash residue, never admitted state.
		if st, e := root.Lstat("queue.pending"); e == nil {
			if !safeInfo(st, false) {
				return ErrStorage
			}
			if e = root.Remove("queue.pending"); e != nil {
				return ErrStorage
			}
		} else if !os.IsNotExist(e) {
			return ErrStorage
		}
		_, e := q.read()
		if e != nil {
			return e
		}
		return q.syncDir()
	})
	if err != nil {
		_ = q.Close()
		return nil, err
	}
	return q, nil
}
func checkParents(path string) error {
	for {
		st, e := os.Lstat(path)
		if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return ErrStorage
		}
		owner, ok := st.Sys().(*syscall.Stat_t)
		if !ok || (owner.Uid != 0 && owner.Uid != uint32(os.Geteuid())) {
			return ErrStorage
		}
		// A sticky temporary parent is permitted; a replaceable nonsticky ancestor is not.
		if st.Mode().Perm()&0022 != 0 && st.Mode()&os.ModeSticky == 0 {
			return ErrStorage
		}
		if path == filepath.Dir(path) {
			return nil
		}
		path = filepath.Dir(path)
	}
}
func safeInfo(st os.FileInfo, directory bool) bool {
	if st == nil {
		return false
	}
	s, ok := st.Sys().(*syscall.Stat_t)
	if !ok || s.Uid != uint32(os.Geteuid()) {
		return false
	}
	if directory {
		return st.IsDir() && st.Mode().Perm() == 0700 && st.Mode()&os.ModeSymlink == 0
	}
	return st.Mode().IsRegular() && st.Mode().Perm() == 0600 && s.Nlink == 1
}
func (q *Queue) withLock(fn func() error) (result error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return ErrStorage
	}
	if e := syscall.Flock(int(q.lock.Fd()), syscall.LOCK_EX); e != nil {
		return ErrStorage
	}
	defer func() {
		if syscall.Flock(int(q.lock.Fd()), syscall.LOCK_UN) != nil {
			result = ErrStorage
		}
	}()
	// Detect replacement of the lock inode and permission changes before touching data.
	held, e := q.lock.Stat()
	if e != nil || !safeInfo(held, false) {
		return ErrStorage
	}
	named, e := q.root.Lstat("queue.lock")
	if e != nil || !safeInfo(named, false) || !os.SameFile(held, named) {
		return ErrStorage
	}
	dir, e := q.root.Stat(".")
	if e != nil || !safeInfo(dir, true) {
		return ErrStorage
	}
	return fn()
}
func (q *Queue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return nil
	}
	q.closed = true
	a := q.lock.Close()
	b := q.root.Close()
	if a != nil || b != nil {
		return ErrStorage
	}
	return nil
}
func (q *Queue) syncDir() error {
	f, e := q.root.Open(".")
	if e != nil {
		return ErrStorage
	}
	syncErr := f.Sync()
	closeErr := f.Close()
	if syncErr != nil || closeErr != nil {
		return ErrStorage
	}
	return nil
}
func (q *Queue) read() (queueState, error) {
	s := queueState{Version: 1}
	f, e := q.root.OpenFile("queue.bin", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if os.IsNotExist(e) {
		return s, nil
	}
	if e != nil {
		return s, ErrStorage
	}
	defer func() { _ = f.Close() }()
	st, e := f.Stat()
	if e != nil || !safeInfo(st, false) || st.Size() > int64(q.maxBytes+maxMetadataBytes) {
		return s, ErrStorage
	}
	b, e := io.ReadAll(io.LimitReader(f, int64(q.maxBytes+maxMetadataBytes+1)))
	if e != nil || len(b) > q.maxBytes+maxMetadataBytes || len(b) < q.aead.NonceSize()+q.aead.Overhead() {
		return s, ErrStorage
	}
	plain, e := q.aead.Open(nil, b[:q.aead.NonceSize()], b[q.aead.NonceSize():], q.aad)
	if e != nil {
		return s, ErrStorage
	}
	if json.Unmarshal(plain, &s) != nil || s.Version != 1 || len(s.Items) > q.maxItems || len(s.Paused) > maxScopes || len(s.Fenced) > maxFences {
		return s, ErrStorage
	}
	for id, fenced := range s.Fenced {
		if !validFenceKey(id) || !fenced || !s.GenerationKnown {
			return s, ErrStorage
		}
	}
	total := 0
	identities := map[string]bool{}
	sequences := map[string]map[uint64]bool{}
	for _, r := range s.Items {
		if r.Binding != q.policy.Binding || !validID(r.Workspace) || !validID(r.Conversation) || !validID(r.Event) || r.Sequence == 0 {
			return s, ErrStorage
		}
		k := r.Conversation + "/" + r.Event
		if identities[k] {
			return s, ErrStorage
		}
		identities[k] = true
		if sequences[r.Conversation] == nil {
			sequences[r.Conversation] = map[uint64]bool{}
		}
		if sequences[r.Conversation][r.Sequence] {
			return s, ErrStorage
		}
		sequences[r.Conversation][r.Sequence] = true
		raw, _ := json.Marshal(r)
		if len(raw) > MaxEventBytes {
			return s, ErrStorage
		}
		total += len(raw)
	}
	if total > q.maxBytes {
		return s, ErrStorage
	}
	for _, p := range s.Paused {
		if !p.valid() {
			return s, ErrStorage
		}
	}
	return s, nil
}
func (q *Queue) write(s queueState) error {
	plain, e := json.Marshal(s)
	if e != nil || len(plain)+q.aead.NonceSize()+q.aead.Overhead() > q.maxBytes+maxMetadataBytes {
		return ErrCapacity
	}
	nonce := make([]byte, q.aead.NonceSize())
	if _, e = io.ReadFull(rand.Reader, nonce); e != nil {
		return ErrStorage
	}
	b := q.aead.Seal(nonce, nonce, plain, q.aad)
	// A previous failed write may have left a pending file in this same handle.
	if st, e := q.root.Lstat("queue.pending"); e == nil {
		if !safeInfo(st, false) {
			return ErrStorage
		}
		if q.root.Remove("queue.pending") != nil {
			return ErrStorage
		}
	} else if !os.IsNotExist(e) {
		return ErrStorage
	}
	f, e := q.root.OpenFile("queue.pending", os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return ErrStorage
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil || closeErr != nil {
		return ErrStorage
	}
	if e = q.root.Rename("queue.pending", "queue.bin"); e != nil {
		return ErrStorage
	}
	return q.syncDir()
}
func stats(s queueState) QueueStats {
	n := 0
	for _, r := range s.Items {
		b, _ := json.Marshal(r)
		n += len(b)
	}
	return QueueStats{len(s.Items), n, len(s.Paused)}
}
func (q *Queue) Stats() (QueueStats, error) {
	var out QueueStats
	err := q.withLock(func() error {
		s, e := q.read()
		if e == nil {
			out = stats(s)
		}
		return e
	})
	return out, err
}
func paused(s queueState, r Record) bool {
	for _, p := range s.Paused {
		if p.covers(r) {
			return true
		}
	}
	return false
}

// Capture filters before any durable write. It copies admitted bytes; later
// caller mutation cannot change queued content. Identical pending retries are
// no-ops. Identity conflicts never overwrite an earlier record.
func (q *Queue) Capture(in Input) (Decision, error) {
	var d Decision
	err := q.withLock(func() error {
		r, decision, e := admit(q.policy, in)
		d = decision
		if e != nil {
			return e
		}
		s, e := q.read()
		if e != nil {
			return e
		}
		if s.Fenced[ConversationFenceKey(r.Binding.Producer, r.Conversation)] {
			return ErrBlocked
		}
		if paused(s, r) {
			d.Admitted = false
			return ErrPaused
		}
		for _, old := range s.Items {
			if old.Conversation == r.Conversation {
				if old.Event == r.Event {
					if recordDigest(old) == recordDigest(r) {
						return nil
					}
					d.Admitted = false
					return ErrConflict
				}
				if old.Sequence == r.Sequence {
					d.Admitted = false
					return ErrConflict
				}
			}
		}
		raw, _ := json.Marshal(r)
		st := stats(s)
		if st.Items >= q.maxItems || len(raw) > q.maxBytes-st.Bytes {
			d.Admitted = false
			return ErrCapacity
		}
		s.Items = append(s.Items, r)
		sort.SliceStable(s.Items, func(i, j int) bool {
			a, b := s.Items[i], s.Items[j]
			if a.Conversation == b.Conversation {
				return a.Sequence < b.Sequence
			}
			return a.Conversation < b.Conversation
		})
		return q.write(s)
	})
	if err != nil {
		d.Admitted = false
	}
	return d, err
}

// Pause atomically persists the scope and removes matching unpublished records.
// It does not delete server history or guarantee physical erasure of disk blocks.
// A publication already in progress completes before Pause acquires the lock.
func (q *Queue) Pause(scope Scope) (int, error) {
	if !scope.valid() {
		return 0, ErrInvalid
	}
	removed := 0
	err := q.withLock(func() error {
		s, e := q.read()
		if e != nil {
			return e
		}
		found := false
		for _, p := range s.Paused {
			if p == scope {
				found = true
			}
		}
		if !found {
			if len(s.Paused) >= maxScopes {
				return ErrCapacity
			}
			s.Paused = append(s.Paused, scope)
		}
		keep := s.Items[:0]
		for _, r := range s.Items {
			if scope.covers(r) {
				removed++
			} else {
				keep = append(keep, r)
			}
		}
		s.Items = keep
		return q.write(s)
	})
	return removed, err
}

// Resume removes only this exact local pause. It cannot change a binding, grant,
// policy or server fence, and is never called automatically by Capture or Drain.
func (q *Queue) Resume(scope Scope) error {
	if !scope.valid() {
		return ErrInvalid
	}
	return q.withLock(func() error {
		s, e := q.read()
		if e != nil {
			return e
		}
		keep := s.Paused[:0]
		for _, p := range s.Paused {
			if p != scope {
				keep = append(keep, p)
			}
		}
		s.Paused = keep
		return q.write(s)
	})
}

// ReplayKey is metadata for a trusted current-authority/fence adapter. The
// adapter MUST resolve workspace enrollment and private mode from trusted
// current state, verify the authenticated binding, then fetch complete fences.
type ReplayKey struct {
	Binding                        Binding
	Workspace, Conversation, Event string
}
type Verification struct {
	Binding             Binding
	Workspace           string
	Allowed             bool
	Full                bool
	GenerationKnown     bool
	Generation          uint64
	SyncedAt, ExpiresAt time.Time
	// Conversations is the complete server fence set, keyed by
	// ConversationFenceKey, including streams belonging to other producers.
	Conversations map[string]bool
}
type Reconciler interface {
	Reconcile(context.Context, ReplayKey) (Verification, error)
}

// Publisher is deliberately not implemented here. An integration must send the
// generation and enforce it again at server admission AND canonical publication.
// Retry the SAME identity on ambiguous outcomes; absence of status is unknown.
type Publisher interface {
	Publish(context.Context, Record, uint64) (Receipt, error)
}
type Receipt struct {
	Key      ReplayKey
	Digest   string
	Complete bool
}
type DrainResult struct {
	Published int
	Remaining int
}

func key(r Record) ReplayKey { return ReplayKey{r.Binding, r.Workspace, r.Conversation, r.Event} }
func verificationOK(v Verification, r Record, s queueState, now time.Time) bool {
	if !v.Allowed || v.Binding != r.Binding || v.Workspace != r.Workspace || !v.Full || !v.GenerationKnown || len(v.Conversations) > maxFences {
		return false
	}
	if v.SyncedAt.IsZero() || v.SyncedAt.After(now) || now.Sub(v.SyncedAt) > time.Minute || !v.ExpiresAt.After(now) || v.ExpiresAt.Sub(v.SyncedAt) > time.Minute {
		return false
	}
	if s.GenerationKnown && v.Generation < s.Generation {
		return false
	}
	for id, fenced := range v.Conversations {
		if !validFenceKey(id) || !fenced {
			return false
		}
	}
	// Deletion fences cannot disappear through a normal refresh. Intentional
	// recapture needs a separately specified owner action, which is not supplied.
	for id := range s.Fenced {
		if !v.Conversations[id] {
			return false
		}
	}
	if s.GenerationKnown && v.Generation == s.Generation && len(v.Conversations) != len(s.Fenced) {
		return false
	}
	return true
}

// Drain holds the process/file lock through reconciliation, publication and
// durable acknowledgment. Callbacks must honor context deadlines and never call
// back into this queue. max is bounded; no background goroutines are launched.
func (q *Queue) Drain(ctx context.Context, g Reconciler, p Publisher, max int) (DrainResult, error) {
	var out DrainResult
	if g == nil || p == nil || max < 1 || max > 64 {
		return out, ErrInvalid
	}
	err := q.withLock(func() error {
		s, e := q.read()
		if e != nil {
			return e
		}
		out.Remaining = len(s.Items)
		for len(s.Items) > 0 && out.Published < max {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			r := s.Items[0]
			if paused(s, r) {
				return ErrPaused
			}
			if q.policy.resolve(r.Workspace) != nil {
				return ErrBinding
			}
			v, e := g.Reconcile(ctx, key(r))
			if e != nil {
				return ErrFence
			}
			if !verificationOK(v, r, s, time.Now()) {
				return ErrFence
			}
			// Persist the highest observed generation even when an item is fenced.
			s.Generation = v.Generation
			s.GenerationKnown = true
			s.Fenced = make(map[string]bool, len(v.Conversations))
			for id := range v.Conversations {
				s.Fenced[id] = true
			}
			if e = q.write(s); e != nil {
				return e
			}
			if v.Conversations[ConversationFenceKey(r.Binding.Producer, r.Conversation)] {
				return ErrBlocked
			}
			if !verificationOK(v, r, s, time.Now()) {
				return ErrFence
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			expectedDigest := recordDigest(r)
			receipt, e := p.Publish(ctx, r, v.Generation)
			if e != nil {
				return ErrBlocked
			}
			if !receipt.Complete || receipt.Key != key(r) || receipt.Digest != expectedDigest {
				return ErrBlocked
			}
			s.Items = s.Items[1:]
			if e = q.write(s); e != nil {
				return e
			}
			out.Published++
			out.Remaining = len(s.Items)
		}
		return nil
	})
	return out, err
}

// DigestRecord lets a trusted synthetic publisher produce exact-content receipts.
// It is not a server receipt verifier or a signature.
func DigestRecord(r Record) string { return recordDigest(r) }
