package episodeinstall

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/gitrun"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

const installedName = "installed.json"

type transactionHooks struct{ checkpoint func(string) error }

func (h transactionHooks) at(point string) error {
	if h.checkpoint != nil {
		return h.checkpoint(point)
	}
	return nil
}

type installationRecord struct {
	Schema       string                     `json:"schema"`
	HeaderSHA256 string                     `json:"header_sha256"`
	Identity     writer.MaintenanceIdentity `json:"identity"`
	Components   []string                   `json:"components"`
}

// Install publishes forward only under an existing qualified maintenance owner.
// It neither authenticates production cold custody nor finishes that owner.
func Install(ctx context.Context, ref PreparedRef, owner *writer.BrainMaintenance, admission ColdAdmission) (Result, error) {
	return transact(ctx, ref, owner, admission, false, transactionHooks{})
}

// Resume reconciles actual fixed-slot inventories with the durable prepared
// header. It never rolls old excluded sources back into the active namespace.
func Resume(ctx context.Context, ref PreparedRef, owner *writer.BrainMaintenance, admission ColdAdmission) (Result, error) {
	return transact(ctx, ref, owner, admission, true, transactionHooks{})
}

func transact(ctx context.Context, ref PreparedRef, owner *writer.BrainMaintenance, admission ColdAdmission, resume bool, hooks transactionHooks) (out Result, err error) {
	out = Result{HeaderSHA256: ref.HeaderSHA256, ResidualCopies: true, State: "unqualified"}
	if ctx == nil || owner == nil || nilAdmission(admission) {
		return out, ErrUnqualified
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	h, err := loadHeader(ctx, ref)
	if err != nil {
		return out, ErrUnqualified
	}
	if owner.ValidateFor(ref.Source, h.Identity) != nil {
		return out, ErrUnqualified
	}
	out.MarkerHeld = true
	pins := AdmissionPins{Source: ref.Source, AccountID: h.AccountID, BrainID: h.Identity.BrainID, OperationID: h.Identity.OperationID, SourceHead: h.Identity.SourceHead, CandidateHead: h.Candidate.CandidateHead, FenceDigest: h.Identity.FenceDigest, HeaderSHA256: ref.HeaderSHA256}
	check := func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if owner.ValidateFor(ref.Source, h.Identity) != nil || admission.Check(ctx, pins) != nil {
			return ErrUnqualified
		}
		return nil
	}
	if check() != nil {
		return out, ErrUnqualified
	}
	if verifyInventory(ctx, filepath.Join(ref.Directory, "inspection"), h.Inspection) != nil || verifyInventory(ctx, filepath.Join(ref.Directory, "original"), h.Original) != nil || verifyInventory(ctx, filepath.Join(ref.Directory, "old-index"), h.OldIndex) != nil || verifyInventory(ctx, filepath.Join(ref.Directory, "candidate"), h.CandidateFiles) != nil {
		return out, ErrUnqualified
	}
	if validateRuntime(ctx, ref.Source, true) != nil || validateCanonicalLayout(ctx, ref.Source) != nil {
		return out, ErrUnqualified
	}
	if err = transactionLayout(ctx, ref); err != nil {
		return out, ErrUnqualified
	}
	intent, err := transactionRecord(ref, h, false)
	if err != nil {
		return out, ErrUnqualified
	}
	completion, err := transactionRecord(ref, h, true)
	if err != nil {
		return out, ErrUnqualified
	}
	intentExists, err := readTransactionRecord(filepath.Join(ref.Directory, progressName), intent)
	if err != nil {
		return out, ErrUnqualified
	}
	completeExists, err := readTransactionRecord(filepath.Join(ref.Directory, installedName), completion)
	if err != nil || completeExists && !intentExists {
		return out, ErrUnqualified
	}
	intentPending, _, pendingErr := readPendingRecord(filepath.Join(ref.Directory, progressName+".pending"), intent)
	if pendingErr != nil || intentPending && intentExists {
		return out, ErrUnqualified
	}
	completePending, _, pendingErr := readPendingRecord(filepath.Join(ref.Directory, installedName+".pending"), completion)
	if pendingErr != nil || completePending && completeExists || (completePending || completeExists) && !intentExists {
		return out, ErrUnqualified
	}
	if !resume && (intentExists || intentPending || completePending || completeExists) {
		return out, ErrUnqualified
	}
	states, err := transactionStates(ctx, ref, h)
	if err != nil {
		return out, ErrUnqualified
	}
	if completePending && !allFinal(states) {
		return out, ErrUnqualified
	}
	if !intentExists {
		for _, state := range states {
			if state != slotInitial && state != slotAbsent {
				return out, ErrUnqualified
			}
		}
		// Entire original source inventory must still match before first mutation.
		for _, c := range h.Components {
			if verifyInventory(ctx, filepath.Join(ref.Source, filepath.FromSlash(c.Name)), c.Old) != nil {
				return out, ErrUnqualified
			}
		}
		if check() != nil {
			return out, ErrUnqualified
		}
		if err = writeTransactionRecord(ref.Directory, progressName, intent, hooks); err != nil {
			out.State = "incomplete"
			return out, ErrIncomplete
		}
	} else if err = resyncTransactionParents(ref, h, hooks); err != nil {
		out.State = "incomplete"
		return out, ErrIncomplete
	}
	out.State = "incomplete"
	if completeExists {
		if !allFinal(states) || validateInstalled(ctx, ref.Source, h) != nil || check() != nil {
			return out, ErrIncomplete
		}
		if syncDirectory(ref.Directory) != nil {
			return out, ErrIncomplete
		}
		out.State = "installed-marker-held"
		return out, nil
	}
	for i, c := range h.Components {
		if err = check(); err != nil {
			return out, ErrIncomplete
		}
		state := states[i]
		if state == slotFinal || state == slotAbsent {
			continue
		}
		canonical := filepath.Join(ref.Source, filepath.FromSlash(c.Name))
		retained := filepath.Join(ref.Directory, "retained", filepath.FromSlash(c.Name))
		staged := componentStage(ref, c.Name)
		if state == slotInitial && present(c.Old) {
			if err = renameTransaction(ctx, canonical, retained, "retain:"+c.Name, c.Old, hooks); err != nil {
				return out, ErrIncomplete
			}
		}
		if err = check(); err != nil {
			return out, ErrIncomplete
		}
		if present(c.New) {
			if err = renameTransaction(ctx, staged, canonical, "publish:"+c.Name, c.New, hooks); err != nil {
				return out, ErrIncomplete
			}
		}
		if err = check(); err != nil {
			return out, ErrIncomplete
		}
		actual, e := transactionStates(ctx, ref, h)
		if e != nil || actual[i] != slotFinal && actual[i] != slotAbsent {
			return out, ErrIncomplete
		}
		states = actual
	}
	states, err = transactionStates(ctx, ref, h)
	if err != nil || !allFinal(states) || validateInstalled(ctx, ref.Source, h) != nil || check() != nil {
		return out, ErrIncomplete
	}
	if err = writeTransactionRecord(ref.Directory, installedName, completion, hooks); err != nil {
		return out, ErrIncomplete
	}
	out.State = "installed-marker-held"
	return out, nil
}
func nilAdmission(a ColdAdmission) bool {
	if a == nil {
		return true
	}
	v := reflect.ValueOf(a)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
func present(in inventory) bool { return len(in) > 0 && in[0].Kind != "absent" }

const (
	slotInitial = iota
	slotBetween
	slotFinal
	slotAbsent
)

func allFinal(states []int) bool {
	for _, s := range states {
		if s != slotFinal && s != slotAbsent {
			return false
		}
	}
	return true
}
func componentStage(ref PreparedRef, name string) string {
	if strings.HasPrefix(name, ".serenity/") {
		return filepath.Join(ref.Directory, "new-index", filepath.Base(name))
	}
	return filepath.Join(ref.Directory, "replacement", name)
}
func absentAt(ctx context.Context, path string) bool {
	got, err := inventoryPath(ctx, path)
	return err == nil && len(got) == 1 && got[0].Kind == "absent"
}
func transactionStates(ctx context.Context, ref PreparedRef, h preparedHeader) ([]int, error) {
	states := make([]int, len(h.Components))
	unfinished := false
	for i, c := range h.Components {
		canonical := filepath.Join(ref.Source, filepath.FromSlash(c.Name))
		retained := filepath.Join(ref.Directory, "retained", filepath.FromSlash(c.Name))
		staged := componentStage(ref, c.Name)
		ca, ra, sa := absentAt(ctx, canonical), absentAt(ctx, retained), absentAt(ctx, staged)
		oldPresent, newPresent := present(c.Old), present(c.New)
		isInitial := verifyInventory(ctx, canonical, c.Old) == nil && ra && verifyInventory(ctx, staged, c.New) == nil
		isBetween := ca && verifyInventory(ctx, retained, c.Old) == nil && verifyInventory(ctx, staged, c.New) == nil
		isFinal := verifyInventory(ctx, canonical, c.New) == nil && verifyInventory(ctx, retained, c.Old) == nil && sa
		switch {
		case !oldPresent && !newPresent && ca && ra && sa:
			states[i] = slotAbsent
		case isFinal:
			states[i] = slotFinal
		case isInitial:
			states[i] = slotInitial
		case isBetween:
			states[i] = slotBetween
		default:
			return nil, ErrUnqualified
		}
		// A forward transaction can have one partial component followed by initial
		// components; later finalized components cannot jump over unfinished work.
		if states[i] == slotFinal && unfinished {
			return nil, ErrUnqualified
		}
		if states[i] == slotInitial || states[i] == slotBetween {
			unfinished = true
		}
	}
	return states, nil
}
func transactionRecord(ref PreparedRef, h preparedHeader, complete bool) ([]byte, error) {
	schema := "serenity.episode-install-intent/v1"
	if complete {
		schema = "serenity.episode-install-complete/v1"
	}
	raw, err := json.Marshal(installationRecord{schema, ref.HeaderSHA256, h.Identity, append([]string(nil), componentNames[:]...)})
	return append(raw, '\n'), err
}
func readTransactionRecord(path string, want []byte) (bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || !ownedInfo(info) || !singleLink(info) || info.Size() != int64(len(want)) {
		return false, ErrUnqualified
	}
	f, err := os.Open(path)
	if err != nil {
		return false, ErrUnqualified
	}
	opened, statErr := f.Stat()
	if statErr != nil || !os.SameFile(info, opened) {
		_ = f.Close()
		return false, ErrUnqualified
	}
	actual, err := io.ReadAll(io.LimitReader(f, int64(len(want))+1))
	closeErr := f.Close()
	if err != nil || closeErr != nil || !bytes.Equal(actual, want) {
		return false, ErrUnqualified
	}
	return true, nil
}

// Pending records contain only a deterministic opaque record. A strict byte
// prefix can be completed append-only; no unknown bytes are truncated or erased.
func readPendingRecord(path string, want []byte) (bool, []byte, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || !ownedInfo(info) || !singleLink(info) || info.Size() < 0 || info.Size() > int64(len(want)) {
		return false, nil, ErrUnqualified
	}
	f, err := os.Open(path)
	if err != nil {
		return false, nil, ErrUnqualified
	}
	opened, statErr := f.Stat()
	if statErr != nil || !os.SameFile(info, opened) {
		_ = f.Close()
		return false, nil, ErrUnqualified
	}
	raw, readErr := io.ReadAll(io.LimitReader(f, int64(len(want))+1))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil || !bytes.HasPrefix(want, raw) {
		return false, nil, ErrUnqualified
	}
	return true, raw, nil
}
func writeTransactionRecord(directory, name string, raw []byte, hooks transactionHooks) error {
	if hooks.at("before-record:"+name) != nil || canonicalDirectory(directory, true) != nil {
		return ErrIncomplete
	}
	final := filepath.Join(directory, name)
	pending := final + ".pending"
	if _, err := os.Lstat(final); !os.IsNotExist(err) {
		return ErrIncomplete
	}
	exists, prefix, err := readPendingRecord(pending, raw)
	if err != nil {
		return ErrIncomplete
	}
	var f *os.File
	if !exists {
		f, err = os.OpenFile(pending, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return ErrIncomplete
		}
		if err = hooks.at("after-record-create:" + name); err != nil {
			_ = f.Close()
			return ErrIncomplete
		}
	} else {
		before, statErr := os.Lstat(pending)
		if statErr != nil {
			return ErrIncomplete
		}
		f, err = os.OpenFile(pending, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			return ErrIncomplete
		}
		opened, statErr := f.Stat()
		if statErr != nil || !os.SameFile(before, opened) {
			_ = f.Close()
			return ErrIncomplete
		}
	}
	midpoint := len(raw) / 2
	if len(prefix) < midpoint {
		if _, err = f.Write(raw[len(prefix):midpoint]); err != nil {
			_ = f.Close()
			return ErrIncomplete
		}
		prefix = raw[:midpoint]
	}
	if hooks.at("after-record-midwrite:"+name) != nil {
		_ = f.Close()
		return ErrIncomplete
	}
	if _, err = f.Write(raw[len(prefix):]); err != nil {
		_ = f.Close()
		return ErrIncomplete
	}
	if hooks.at("before-record-file-sync:"+name) != nil {
		_ = f.Close()
		return ErrIncomplete
	}
	syncErr := f.Sync()
	closeErr := f.Close()
	if syncErr != nil || closeErr != nil {
		return ErrIncomplete
	}
	if hooks.at("after-record-file-sync:"+name) != nil {
		return ErrIncomplete
	}
	valid, persisted, err := readPendingRecord(pending, raw)
	if err != nil || !valid || !bytes.Equal(persisted, raw) {
		return ErrIncomplete
	}
	if hooks.at("before-record-rename:"+name) != nil {
		return ErrIncomplete
	}
	if err = renameExclusive(pending, final); err != nil {
		return ErrIncomplete
	}
	if hooks.at("after-record-rename:"+name) != nil {
		return ErrIncomplete
	}
	if hooks.at("before-record-directory-sync:"+name) != nil || syncDirectory(directory) != nil {
		return ErrIncomplete
	}
	return hooks.at("after-record-directory-sync:" + name)
}
func renameTransaction(ctx context.Context, from, to, label string, want inventory, hooks transactionHooks) error {
	if verifyInventory(ctx, from, want) != nil || !absentAt(ctx, to) {
		return ErrIncomplete
	}
	if hooks.at("before-rename:"+label) != nil {
		return ErrIncomplete
	}
	if canonicalDirectory(filepath.Dir(from), false) != nil || canonicalDirectory(filepath.Dir(to), false) != nil || verifyInventory(ctx, from, want) != nil || !absentAt(ctx, to) {
		return ErrIncomplete
	}
	if err := renameExclusive(from, to); err != nil {
		return ErrIncomplete
	}
	if hooks.at("after-rename:"+label) != nil {
		return ErrIncomplete
	}
	for i, parent := range []string{filepath.Dir(from), filepath.Dir(to)} {
		suffix := "source"
		if i == 1 {
			suffix = "destination"
		}
		if hooks.at("before-parent-sync:"+label+":"+suffix) != nil {
			return ErrIncomplete
		}
		if syncDirectory(parent) != nil {
			return ErrIncomplete
		}
		if hooks.at("after-parent-sync:"+label+":"+suffix) != nil {
			return ErrIncomplete
		}
	}
	return nil
}
func transactionLayout(ctx context.Context, ref PreparedRef) error {
	if checkChildren(ctx, ref.Directory, map[string]bool{"original": true, "old-index": true, "inspection": true, "candidate": true, "replacement": true, "new-index": true, "retained": true, headerName: true, progressName: true, installedName: true, progressName + ".pending": true, installedName + ".pending": true}) != nil {
		return ErrUnqualified
	}
	for _, dir := range []string{"retained", "retained/.serenity"} {
		path := filepath.Join(ref.Directory, filepath.FromSlash(dir))
		if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
			return ErrUnqualified
		}
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
			return ErrUnqualified
		}
		if syncDirectory(filepath.Dir(path)) != nil {
			return ErrUnqualified
		}
	}
	canonical := map[string]bool{".git": true, "brain": true, ".dira": true, "serenity.yml": true, ".gitignore": true}
	retained := map[string]bool{".git": true, "brain": true, ".dira": true, "serenity.yml": true, ".gitignore": true, ".serenity": true}
	family := map[string]bool{"index.db": true, "index.db-wal": true, "index.db-shm": true}
	if checkChildren(ctx, filepath.Join(ref.Directory, "replacement"), canonical) != nil || checkChildren(ctx, filepath.Join(ref.Directory, "retained"), retained) != nil || checkChildren(ctx, filepath.Join(ref.Directory, "new-index"), family) != nil || checkChildren(ctx, filepath.Join(ref.Directory, "retained/.serenity"), family) != nil {
		return ErrUnqualified
	}
	return nil
}
func checkChildren(ctx context.Context, path string, allowed map[string]bool) error {
	if canonicalDirectory(path, true) != nil {
		return ErrUnqualified
	}
	entries, err := boundedDirectoryEntries(ctx, path, len(allowed))
	if err != nil {
		return ErrUnqualified
	}
	for _, e := range entries {
		if !allowed[e.Name()] {
			return ErrUnqualified
		}
	}
	return nil
}
func resyncTransactionParents(ref PreparedRef, h preparedHeader, hooks transactionHooks) error {
	if hooks.at("before-recovery-operation-sync") != nil || syncDirectory(ref.Directory) != nil || hooks.at("after-recovery-operation-sync") != nil {
		return ErrIncomplete
	}
	seen := map[string]bool{}
	for _, c := range h.Components {
		for _, path := range []string{filepath.Dir(filepath.Join(ref.Source, c.Name)), filepath.Dir(filepath.Join(ref.Directory, "retained", c.Name)), filepath.Dir(componentStage(ref, c.Name))} {
			if seen[path] {
				continue
			}
			seen[path] = true
			if hooks.at("before-recovery-parent-sync") != nil || syncDirectory(path) != nil || hooks.at("after-recovery-parent-sync") != nil {
				return ErrIncomplete
			}
		}
	}
	return nil
}
func validateInstalled(ctx context.Context, source string, h preparedHeader) error {
	if validateCanonicalLayout(ctx, source) != nil || validateRuntime(ctx, source, true) != nil {
		return ErrUnqualified
	}
	r := gitrun.CanonicalReadOnly(source)
	head, err := r.Output(ctx, "rev-parse", "--verify", "HEAD")
	if err != nil || strings.TrimSpace(string(head)) != h.Candidate.CandidateHead {
		return ErrUnqualified
	}
	status, err := r.Output(ctx, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil || len(status) != 0 {
		return ErrUnqualified
	}
	p, err := store.LoadEpisodes(ctx, store.NewSourceStore(source))
	if err != nil || p.BrainID != "" && p.BrainID != h.Identity.BrainID || p.Generation != uint64(len(h.CurrentFences)) {
		return ErrUnqualified
	}
	fences := map[string]string{}
	for _, f := range h.CurrentFences {
		fences[f.CommandKey] = store.EpisodeHash(f)
	}
	count := 0
	for _, record := range p.Records {
		if record.Kind != "exclude" {
			continue
		}
		if fences[record.CommandKey] != store.EpisodeHash(record) {
			return ErrUnqualified
		}
		count++
	}
	if count != len(fences) {
		return ErrUnqualified
	}
	return nil
}
