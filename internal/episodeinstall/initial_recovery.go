package episodeinstall

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/sirerun/serenity/internal/writer"
)

const initialRecoveryPurpose = "serenity.initial-maintenance-reconciliation/v1"
const initialRecoveryName = "initial-recovery.json"

type initialObservationRecord struct {
	Present bool   `json:"present"`
	Device  uint64 `json:"device"`
	Inode   uint64 `json:"inode"`
	UID     uint32 `json:"uid"`
	GID     uint32 `json:"gid"`
	Mode    uint32 `json:"mode"`
	Links   uint64 `json:"links"`
	Bytes   int64  `json:"bytes"`
	SHA256  string `json:"sha256"`
	Prefix  []byte `json:"prefix"`
}

func privateObservation(o writer.InitialMaintenanceObservation) initialObservationRecord {
	return initialObservationRecord{o.Present, o.Device, o.Inode, o.UID, o.GID, o.Mode, o.Links, o.Bytes, o.SHA256, append([]byte{}, o.Prefix...)}
}
func (r initialObservationRecord) observation() writer.InitialMaintenanceObservation {
	return writer.InitialMaintenanceObservation{Present: r.Present, Device: r.Device, Inode: r.Inode, UID: r.UID, GID: r.GID, Mode: r.Mode, Links: r.Links, Bytes: r.Bytes, SHA256: r.SHA256, Prefix: append([]byte{}, r.Prefix...)}
}

type initialRecoveryRecord struct {
	Schema               string                     `json:"schema"`
	Purpose              string                     `json:"purpose"`
	HeaderSHA256         string                     `json:"header_sha256"`
	Identity             writer.MaintenanceIdentity `json:"identity"`
	ExpectedMarkerSHA256 string                     `json:"expected_marker_sha256"`
	Original             initialObservationRecord   `json:"original"`
	PriorIntent          string                     `json:"prior_intent"`
}

func expectedInitialMarker(id writer.MaintenanceIdentity) []byte {
	raw, _ := json.Marshal(id)
	return append(raw, '\n')
}
func initialRecord(ref PreparedRef, o writer.InitialMaintenanceObservation) initialRecoveryRecord {
	return initialRecoveryRecord{initialRecoveryPurpose, initialRecoveryPurpose, ref.HeaderSHA256, ref.Identity, hashBytes(expectedInitialMarker(ref.Identity)), privateObservation(o), "unattributed"}
}
func initialRecordBytes(r initialRecoveryRecord) ([]byte, error) {
	raw, err := json.Marshal(r)
	if err != nil || r.Original.Prefix == nil || len(raw)+1 > 4096 {
		return nil, ErrUnqualified
	}
	return append(raw, '\n'), nil
}
func initialObservationExtends(original, current writer.InitialMaintenanceObservation, expected []byte) bool {
	if !original.Present || !current.Present || original.Device != current.Device || original.Inode != current.Inode || original.UID != current.UID || original.GID != current.GID || original.Mode != 0600 || current.Mode != 0600 || original.Links != 1 || current.Links != 1 || original.Bytes != int64(len(original.Prefix)) || current.Bytes != int64(len(current.Prefix)) || original.SHA256 != hashBytes(original.Prefix) || current.SHA256 != hashBytes(current.Prefix) {
		return false
	}
	return bytes.HasPrefix(expected, current.Prefix) && bytes.HasPrefix(current.Prefix, original.Prefix)
}
func loadInitialRecord(ref PreparedRef) (initialRecoveryRecord, bool, error) {
	var record initialRecoveryRecord
	p := filepath.Join(ref.Directory, initialRecoveryName)
	info, err := os.Lstat(p)
	if os.IsNotExist(err) {
		return record, false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || !ownedInfo(info) || !singleLink(info) || info.Size() > 4096 {
		return record, false, ErrUnqualified
	}
	f, err := os.Open(p)
	if err != nil {
		return record, false, ErrUnqualified
	}
	opened, statErr := f.Stat()
	raw, readErr := io.ReadAll(io.LimitReader(f, 4097))
	closeErr := f.Close()
	if statErr != nil || !os.SameFile(info, opened) || readErr != nil || closeErr != nil || json.Unmarshal(raw, &record) != nil {
		return record, false, ErrUnqualified
	}
	canonical, err := initialRecordBytes(record)
	if err != nil || !bytes.Equal(raw, canonical) || record.Schema != initialRecoveryPurpose || record.Purpose != initialRecoveryPurpose || record.PriorIntent != "unattributed" || record.HeaderSHA256 != ref.HeaderSHA256 || record.Identity != ref.Identity || record.ExpectedMarkerSHA256 != hashBytes(expectedInitialMarker(ref.Identity)) || !initialObservationExtends(record.Original.observation(), record.Original.observation(), expectedInitialMarker(ref.Identity)) {
		return record, false, ErrUnqualified
	}
	return record, true, nil
}

// RecoverInitialMaintenance reconciles a CURRENT initial gate under a distinct
// fresh owner admission. It does not identify the prior actor or authorize Install.
func RecoverInitialMaintenance(ctx context.Context, ref PreparedRef, admission InitialRecoveryAdmission) (*writer.BrainMaintenance, Result, error) {
	return recoverInitialMaintenance(ctx, ref, admission, transactionHooks{})
}
func recoverInitialMaintenance(ctx context.Context, ref PreparedRef, admission InitialRecoveryAdmission, hooks transactionHooks) (owner *writer.BrainMaintenance, out Result, err error) {
	out = Result{HeaderSHA256: ref.HeaderSHA256, ResidualCopies: true, State: "unqualified"}
	if ctx == nil || nilAdmission(admission) {
		return nil, out, ErrUnqualified
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	h, err := loadHeader(ctx, ref)
	if err != nil {
		return nil, out, ErrUnqualified
	}
	pending, err := writer.OpenInitialBrainMaintenance(ref.Source, ref.Identity)
	if err != nil {
		return nil, out, ErrUnqualified
	}
	defer func() {
		if owner == nil {
			_ = pending.Close()
		}
	}()
	current, err := pending.Observation()
	if err != nil {
		return nil, out, ErrUnqualified
	}
	out.MarkerHeld = current.Present
	pins := AdmissionPins{Source: ref.Source, AccountID: h.AccountID, BrainID: h.Identity.BrainID, OperationID: h.Identity.OperationID, SourceHead: h.Identity.SourceHead, CandidateHead: h.Candidate.CandidateHead, FenceDigest: h.Identity.FenceDigest, HeaderSHA256: ref.HeaderSHA256}
	qualify := func() error { return qualifyInitialSource(ctx, ref, h) }
	check := func(phase, recordHash string, original, observed writer.InitialMaintenanceObservation) error {
		if ctx.Err() != nil || qualify() != nil || admission.Check(ctx, pins) != nil {
			return ErrUnqualified
		}
		if admission.CheckInitialRecovery(ctx, InitialRecoveryPins{AdmissionPins: pins, Purpose: initialRecoveryPurpose, Phase: phase, RecordSHA256: recordHash, Original: privateObservation(original).observation(), Current: privateObservation(observed).observation()}) != nil {
			return ErrUnqualified
		}
		return nil
	}
	record, published, err := loadInitialRecord(ref)
	if err != nil {
		return nil, out, ErrUnqualified
	}
	if !current.Present {
		if published || !absentAt(ctx, filepath.Join(ref.Directory, initialRecoveryName+".pending")) || hooks.at("initial:before-gate") != nil {
			return nil, out, ErrUnqualified
		}
		// Creation can succeed before a later verifier/sync/cancel failure.
		// On errors this conservative flag means a gate MAY remain; only a
		// fresh locked observation can establish actual presence or absence.
		out.MarkerHeld = true
		out.State = "incomplete"
		if err = pending.EnsureGate(ctx, func(_ context.Context, o writer.InitialMaintenanceObservation) error {
			if o.Present {
				return ErrUnqualified
			}
			return check("precreate", "", o, o)
		}); err != nil {
			return nil, out, ErrUnqualified
		}
		out.MarkerHeld = true
		if hooks.at("initial:after-gate") != nil {
			return nil, out, ErrIncomplete
		}
		current, err = pending.Observation()
		if err != nil {
			return nil, out, ErrUnqualified
		}
	}
	if !published {
		record = initialRecord(ref, current)
	}
	original := record.Original.observation()
	expected := expectedInitialMarker(ref.Identity)
	if !initialObservationExtends(original, current, expected) || qualify() != nil {
		return nil, out, ErrUnqualified
	}
	recordRaw, err := initialRecordBytes(record)
	if err != nil {
		return nil, out, ErrUnqualified
	}
	recordHash := hashBytes(recordRaw)
	if published {
		if !absentAt(ctx, filepath.Join(ref.Directory, initialRecoveryName+".pending")) {
			return nil, out, ErrUnqualified
		}
	} else {
		// Any interrupted pending record must match the SAME unchanged original
		// observation. Published evidence alone permits a later extended marker.
		if !reflect.DeepEqual(privateObservation(original), privateObservation(current)) {
			return nil, out, ErrUnqualified
		}
		if _, _, err = readPendingRecord(filepath.Join(ref.Directory, initialRecoveryName+".pending"), recordRaw); err != nil {
			return nil, out, ErrUnqualified
		}
	}
	// An inode observation alone is not durable evidence: even a full marker
	// may be the unsynced product of an interrupted Begin. Stabilize every
	// actual gate before creating/completing evidence that binds its inode.
	if hooks.at("initial:before-gate-sync") != nil {
		return nil, out, ErrIncomplete
	}
	beforeSync := privateObservation(current)
	if pending.SyncGate(ctx, func(_ context.Context, o writer.InitialMaintenanceObservation) error {
		if !reflect.DeepEqual(beforeSync, privateObservation(o)) || !initialObservationExtends(original, o, expected) {
			return ErrUnqualified
		}
		return check("record", recordHash, original, o)
	}) != nil {
		return nil, out, ErrIncomplete
	}
	if hooks.at("initial:after-gate-sync") != nil {
		return nil, out, ErrIncomplete
	}
	current, err = pending.Observation()
	if err != nil || !reflect.DeepEqual(beforeSync, privateObservation(current)) {
		return nil, out, ErrUnqualified
	}
	if check("record", recordHash, original, current) != nil {
		return nil, out, ErrUnqualified
	}
	if !published {
		if writeTransactionRecord(ref.Directory, initialRecoveryName, recordRaw, hooks) != nil {
			return nil, out, ErrIncomplete
		}
	} else if syncDirectory(ref.Directory) != nil {
		return nil, out, ErrIncomplete
	}
	if hooks.at("initial:before-complete") != nil {
		return nil, out, ErrIncomplete
	}
	calls := 0
	owner, err = pending.Complete(ctx, func(_ context.Context, o writer.InitialMaintenanceObservation) error {
		calls++
		phase := "append"
		if calls == 2 {
			phase = "return"
		} else if calls != 1 {
			return ErrUnqualified
		}
		if !initialObservationExtends(original, o, expected) {
			return ErrUnqualified
		}
		stored, ok, loadErr := loadInitialRecord(ref)
		if loadErr != nil || !ok || !reflect.DeepEqual(stored, record) {
			return ErrUnqualified
		}
		return check(phase, recordHash, original, o)
	})
	if err != nil || calls != 2 {
		if owner != nil {
			_ = owner.Close()
			owner = nil
		}
		return nil, out, ErrIncomplete
	}
	if hooks.at("initial:after-complete") != nil {
		_ = owner.Close()
		owner = nil
		return nil, out, ErrIncomplete
	}
	out.State = "initial-gate-reconciled"
	out.MarkerHeld = true
	return owner, out, nil
}

// Pure qualification: no path creation, SQLite opens, payload copies or marker
// mutation. Every canonical and staged component must remain in its OLD slot.
func qualifyInitialSource(ctx context.Context, ref PreparedRef, h preparedHeader) error {
	if validateCanonicalLayout(ctx, ref.Source) != nil || validateRuntime(ctx, ref.Source, true) != nil {
		return ErrUnqualified
	}
	allowed := map[string]bool{"original": true, "old-index": true, "inspection": true, "candidate": true, "replacement": true, "new-index": true, headerName: true, initialRecoveryName: true, initialRecoveryName + ".pending": true}
	if checkChildren(ctx, ref.Directory, allowed) != nil {
		return ErrUnqualified
	}
	for _, p := range []string{"retained", progressName, progressName + ".pending", installedName, installedName + ".pending"} {
		if !absentAt(ctx, filepath.Join(ref.Directory, p)) {
			return ErrUnqualified
		}
	}
	for _, item := range []struct {
		name string
		want inventory
	}{{"original", h.Original}, {"old-index", h.OldIndex}, {"inspection", h.Inspection}, {"candidate", h.CandidateFiles}} {
		if verifyInventory(ctx, filepath.Join(ref.Directory, item.name), item.want) != nil {
			return ErrUnqualified
		}
	}
	canonical := map[string]bool{".git": true, "brain": true, ".dira": true, "serenity.yml": true, ".gitignore": true}
	family := map[string]bool{"index.db": true, "index.db-wal": true, "index.db-shm": true}
	if checkChildren(ctx, filepath.Join(ref.Directory, "replacement"), canonical) != nil || checkChildren(ctx, filepath.Join(ref.Directory, "new-index"), family) != nil {
		return ErrUnqualified
	}
	for _, c := range h.Components {
		if verifyInventory(ctx, filepath.Join(ref.Source, filepath.FromSlash(c.Name)), c.Old) != nil || verifyInventory(ctx, componentStage(ref, c.Name), c.New) != nil {
			return ErrUnqualified
		}
	}
	g := &prepareGit{}
	if prepareHead(ctx, g, ref.Source) != h.Identity.SourceHead || !prepareClean(ctx, g, ref.Source) {
		return ErrUnqualified
	}
	return nil
}

func validateInitialRecordForInstall(ctx context.Context, ref PreparedRef) error {
	if !absentAt(ctx, filepath.Join(ref.Directory, initialRecoveryName+".pending")) {
		return ErrUnqualified
	}
	record, present, err := loadInitialRecord(ref)
	if err != nil {
		return ErrUnqualified
	}
	if !present {
		return nil
	}
	expected := expectedInitialMarker(ref.Identity)
	if !bytes.HasPrefix(expected, record.Original.Prefix) {
		return ErrUnqualified
	}
	// The normal validated owner separately pins marker inode; bind it to the
	// immutable original evidence here before ordinary installation admits it.
	path := filepath.Join(ref.Source, ".serenity", "canonical-maintenance.json")
	info, err := os.Lstat(path)
	if err != nil {
		return ErrUnqualified
	}
	return validateInitialInstalledMarker(info, path, record, expected)
}
