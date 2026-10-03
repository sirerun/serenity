package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func journalCapacityStore(t *testing.T) *SnapshotLeaseStore {
	t.Helper()
	root := filepath.Join(privateTempDir(t), "journal-capacity")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	return testLeaseStoreOptions(t, SnapshotLeaseStoreOptions{LeaseRoot: root, MaxArtifactBytesPerLease: 1 << 30, MaxMetadataBytesPerLease: 1 << 20, MaxRestoreScratchBytes: 1 << 30, MaxRetainedArtifactBytes: 2 << 30, MaxRetainedMetadataBytes: 16 << 20, MaxLeases: 8}, newLeaseTestAuthority())
}

func journalCapacityNames(t *testing.T, path string) []string {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names
}

func TestSnapshotReleaseJournalOverflowRefusesBeforeEntryInspection(t *testing.T) {
	s := journalCapacityStore(t)
	journal := filepath.Join(s.root, ".releases")
	// Deliberately unknown names prove overflow is counted before filtering or
	// inspecting individual entries. Every entry remains untouched on refusal.
	for i := 0; i <= maxReleaseJournalEntries; i++ {
		if err := os.Mkdir(filepath.Join(journal, fmt.Sprintf("unknown-%04d", i)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	before := journalCapacityNames(t, journal)
	for attempt := 0; attempt < 2; attempt++ {
		entries, err := s.readReleaseJournalEntries()
		if !errors.Is(err, ErrSnapshotLeaseLimit) || len(entries) != 0 {
			t.Fatalf("overflow scan = %d entries, %v", len(entries), err)
		}
	}
	if after := journalCapacityNames(t, journal); !reflect.DeepEqual(before, after) {
		t.Fatal("overflow refusal altered journal inventory")
	}
}

func TestSnapshotReleaseJournalUnknownEntriesRefuseWithoutCleanup(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 1)
	options, _ := inspectionOptions(t, snapshot)
	for _, kind := range []string{"hidden-directory", "unknown-directory", "canonical-file", "canonical-symlink"} {
		t.Run(kind, func(t *testing.T) {
			s := journalCapacityStore(t)
			journal := filepath.Join(s.root, ".releases")
			name := strings.Repeat("a", 64)
			victim := filepath.Join(privateTempDir(t), "victim")
			expected := []byte("retained outside bytes")
			if err := os.WriteFile(victim, expected, 0600); err != nil {
				t.Fatal(err)
			}
			var err error
			switch kind {
			case "hidden-directory":
				name = ".unknown"
				err = os.Mkdir(filepath.Join(journal, name), 0700)
			case "unknown-directory":
				name = "unknown"
				err = os.Mkdir(filepath.Join(journal, name), 0700)
			case "canonical-file":
				err = os.WriteFile(filepath.Join(journal, name), expected, 0600)
			case "canonical-symlink":
				err = os.Symlink(victim, filepath.Join(journal, name))
			}
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(journal, name)
			beforeInfo, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			rootBefore := journalCapacityNames(t, s.root)
			if entries, err := s.readReleaseJournalEntries(); !errors.Is(err, ErrSnapshotLeaseInvalid) || len(entries) != 0 {
				t.Fatalf("unknown journal scan = %d, %v", len(entries), err)
			}
			lease, err := s.Stage(context.Background(), snapshot, options)
			if !errors.Is(err, ErrSnapshotLeaseInvalid) || lease != nil {
				t.Fatalf("Stage with unknown journal = %v, %v", lease, err)
			}
			afterInfo, err := os.Lstat(path)
			if err != nil || !os.SameFile(beforeInfo, afterInfo) {
				t.Fatalf("refusal changed entry identity: %v", err)
			}
			if after := journalCapacityNames(t, s.root); !reflect.DeepEqual(rootBefore, after) {
				t.Fatal("refusal created or removed root entries")
			}
			if got, err := os.ReadFile(victim); err != nil || string(got) != string(expected) {
				t.Fatalf("refusal touched outside bytes: %v", err)
			}
		})
	}
}

func TestSnapshotReleaseJournalTerminalCapacityChargesMetadataAndRefusesStage(t *testing.T) {
	s := journalCapacityStore(t)
	journal := filepath.Join(s.root, ".releases")
	var metadata int64
	// These are local capacity fixtures emitted by the actual receipt encoder.
	// They assert storage bounds, not owner authority or production acceptance.
	for i := 0; i < maxReleaseJournalEntries; i++ {
		id := fmt.Sprintf("%064x", i+1)
		record := leaseDiskRecord{ID: id, PinID: strings.Repeat("b", 64), PlanRef: strings.Repeat("c", 64), ManifestSHA256: strings.Repeat("d", 64), ReservationVersion: 1, AttemptVersion: 1, Disposition: PinAbandonedBeforeEffects, RecordVersion: 3}
		_, raw, err := encodeReleaseTombstone(record)
		if err != nil {
			t.Fatal(err)
		}
		directory := filepath.Join(journal, id)
		if err = os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(directory, releaseRecordName), raw, 0600); err != nil {
			t.Fatal(err)
		}
		metadata += int64(len(raw))
	}
	usage, err := s.usageLocked()
	if err != nil {
		t.Fatal(err)
	}
	if usage.leases != 0 || usage.releaseEntries != maxReleaseJournalEntries || usage.metadata != metadata {
		t.Fatalf("terminal usage = %+v, want zero live leases, %d receipts and %d metadata bytes", usage, maxReleaseJournalEntries, metadata)
	}
	rootBefore := journalCapacityNames(t, s.root)
	journalBefore := journalCapacityNames(t, journal)
	_, snapshot, _ := freshPrivateSnapshot(t, 1)
	options, _ := inspectionOptions(t, snapshot)
	lease, err := s.Stage(context.Background(), snapshot, options)
	if !errors.Is(err, ErrSnapshotLeaseLimit) || lease != nil {
		t.Fatalf("Stage at terminal capacity = %v, %v", lease, err)
	}
	if after := journalCapacityNames(t, s.root); !reflect.DeepEqual(rootBefore, after) {
		t.Fatal("Stage at capacity changed root inventory")
	}
	if after := journalCapacityNames(t, journal); !reflect.DeepEqual(journalBefore, after) {
		t.Fatal("Stage at capacity changed retained receipt inventory")
	}
	if after, err := s.usageLocked(); err != nil || after != usage {
		t.Fatalf("Stage at capacity changed charged usage: %+v, %v", after, err)
	}
}
