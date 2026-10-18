package episodeinstall

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/writer"
)

type transactionAdmission struct {
	want AdmissionPins
	fail bool
}

func (a transactionAdmission) Check(_ context.Context, got AdmissionPins) error {
	if a.fail || a.want != got {
		return ErrUnqualified
	}
	return nil
}
func transactionAdmissionFor(t *testing.T, ref PreparedRef) transactionAdmission {
	t.Helper()
	h, err := loadHeader(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	return transactionAdmission{want: AdmissionPins{Source: ref.Source, AccountID: h.AccountID, BrainID: h.Identity.BrainID, OperationID: h.Identity.OperationID, SourceHead: h.Identity.SourceHead, CandidateHead: h.Candidate.CandidateHead, FenceDigest: h.Identity.FenceDigest, HeaderSHA256: ref.HeaderSHA256}}
}
func transactionPrepared(t *testing.T) PreparedRef {
	t.Helper()
	ref, err := Prepare(context.Background(), prepareFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	return ref
}
func transactionOwner(t *testing.T, ref PreparedRef) *writer.BrainMaintenance {
	t.Helper()
	m, err := writer.BeginBrainMaintenance(ref.Source, ref.Identity)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func TestTransactionInstallsForwardAndKeepsMarker(t *testing.T) {
	ref := transactionPrepared(t)
	m := transactionOwner(t, ref)
	a := transactionAdmissionFor(t, ref)
	result, err := Install(context.Background(), ref, m, a)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "installed-marker-held" || !result.MarkerHeld || !result.ResidualCopies {
		t.Fatalf("wrong boundary %+v", result)
	}
	if view, err := writer.AcquireCanonicalRead(ref.Source); err == nil {
		_ = view.Close()
		t.Fatal("installation removed maintenance gate")
	}
	if _, err := Install(context.Background(), ref, m, a); err == nil {
		t.Fatal("Install adopted existing intent")
	}
	if _, err := Resume(context.Background(), ref, m, a); err != nil {
		t.Fatalf("completed resume %v", err)
	}
	if _, err := os.Stat(filepath.Join(ref.Directory, "original", ".git")); err != nil {
		t.Fatal("original proof removed")
	}
}
func TestTransactionRejectsChangedHandoffAndUnknownSlots(t *testing.T) {
	for _, name := range []string{"changed_source", "unknown_inspection", "unknown_stage", "bad_admission", "wrong_header", "partial_intent", "partial_completion"} {
		t.Run(name, func(t *testing.T) {
			ref := transactionPrepared(t)
			m := transactionOwner(t, ref)
			a := transactionAdmissionFor(t, ref)
			switch name {
			case "changed_source":
				path := filepath.Join(ref.Source, "serenity.yml")
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, append(raw, []byte("# changed handoff\n")...), 0600); err != nil {
					t.Fatal(err)
				}
			case "unknown_inspection":
				if err := os.WriteFile(filepath.Join(ref.Directory, "inspection", "unexpected"), []byte("private"), 0600); err != nil {
					t.Fatal(err)
				}
			case "unknown_stage":
				if err := os.WriteFile(filepath.Join(ref.Directory, "replacement", "unexpected"), []byte("private"), 0600); err != nil {
					t.Fatal(err)
				}
			case "bad_admission":
				a.fail = true
			case "wrong_header":
				ref.HeaderSHA256 = strings.Repeat("0", 64)
			case "partial_intent":
				if err := os.WriteFile(filepath.Join(ref.Directory, progressName), []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			case "partial_completion":
				if err := os.WriteFile(filepath.Join(ref.Directory, installedName), []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := Install(context.Background(), ref, m, a)
			if err == nil {
				t.Fatal("unsafe accepted")
			}
			if strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), ref.Source) {
				t.Fatal("error disclosed payload or routing")
			}
			if view, err := writer.AcquireCanonicalRead(ref.Source); err == nil {
				_ = view.Close()
				t.Fatal("failure removed maintenance gate")
			}
		})
	}
}

// Every subprocess dies without running defers. Resume reacquires the retained
// persistent marker and stable lock inodes, then reconciles actual files rather
// than replaying an old phase flag or rolling excluded history back.
func TestTransactionProcessDeathForwardRecovery(t *testing.T) {
	prototype := transactionPrepared(t)
	h, err := loadHeader(context.Background(), prototype)
	if err != nil {
		t.Fatal(err)
	}
	points := []string{"before-record:" + progressName, "after-record-file-sync:" + progressName, "after-record-directory-sync:" + progressName, "before-record:" + installedName, "after-record-file-sync:" + installedName, "after-record-directory-sync:" + installedName}
	for _, c := range h.Components {
		for _, action := range []string{"retain", "publish"} {
			if action == "retain" && !present(c.Old) || action == "publish" && !present(c.New) {
				continue
			}
			label := action + ":" + c.Name
			points = append(points, "before-rename:"+label, "after-rename:"+label)
			for _, parent := range []string{"source", "destination"} {
				points = append(points, "before-parent-sync:"+label+":"+parent, "after-parent-sync:"+label+":"+parent)
			}
		}
	}
	for _, point := range points {
		t.Run(strings.ReplaceAll(point, "/", "_"), func(t *testing.T) {
			ref := transactionPrepared(t)
			identity, err := json.Marshal(ref.Identity)
			if err != nil {
				t.Fatal(err)
			}
			command := exec.Command(os.Args[0], "-test.run=^TestTransactionCrashChild$")
			command.Env = append(os.Environ(), "SERENITY_INSTALL_TEST_CHILD=1", "SERENITY_INSTALL_TEST_SOURCE="+ref.Source, "SERENITY_INSTALL_TEST_OPERATION="+ref.Directory, "SERENITY_INSTALL_TEST_HEADER="+ref.HeaderSHA256, "SERENITY_INSTALL_TEST_IDENTITY="+string(identity), "SERENITY_INSTALL_TEST_POINT="+point)
			output, err := command.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 23 {
				t.Fatalf("child missed boundary %s: %v %s", point, err, output)
			}
			m, err := writer.ResumeBrainMaintenance(ref.Source, ref.Identity)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = m.Close() }()
			result, err := Resume(context.Background(), ref, m, transactionAdmissionFor(t, ref))
			if err != nil {
				t.Fatalf("forward recovery: %v", err)
			}
			if result.State != "installed-marker-held" || !result.MarkerHeld || !result.ResidualCopies {
				t.Fatal("recovery inflated completion")
			}
			header, err := loadHeader(context.Background(), ref)
			if err != nil || validateInstalled(context.Background(), ref.Source, header) != nil {
				t.Fatal("installed source invalid")
			}
		})
	}
}
func TestTransactionCrashChild(t *testing.T) {
	if os.Getenv("SERENITY_INSTALL_TEST_CHILD") != "1" {
		t.Skip("subprocess only")
	}
	ref := PreparedRef{Source: os.Getenv("SERENITY_INSTALL_TEST_SOURCE"), Directory: os.Getenv("SERENITY_INSTALL_TEST_OPERATION"), HeaderSHA256: os.Getenv("SERENITY_INSTALL_TEST_HEADER"), ResidualCopies: true}
	if err := json.Unmarshal([]byte(os.Getenv("SERENITY_INSTALL_TEST_IDENTITY")), &ref.Identity); err != nil {
		t.Fatal(err)
	}
	m, err := writer.BeginBrainMaintenance(ref.Source, ref.Identity)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	point := os.Getenv("SERENITY_INSTALL_TEST_POINT")
	_, err = transact(context.Background(), ref, m, transactionAdmissionFor(t, ref), false, transactionHooks{checkpoint: func(at string) error {
		if at == point {
			os.Exit(23)
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Fatal("requested death boundary was not reached")
}

type transactionFlakyAdmission struct {
	want   AdmissionPins
	calls  int
	failAt int
}

func (a *transactionFlakyAdmission) Check(_ context.Context, p AdmissionPins) error {
	a.calls++
	if p != a.want || a.failAt > 0 && a.calls >= a.failAt {
		return ErrUnqualified
	}
	return nil
}
func TestTransactionAuthorityLossParksAndFreshAdmissionResumes(t *testing.T) {
	ref := transactionPrepared(t)
	m := transactionOwner(t, ref)
	a := &transactionFlakyAdmission{want: transactionAdmissionFor(t, ref).want, failAt: 4}
	result, err := Install(context.Background(), ref, m, a)
	if err == nil || !result.MarkerHeld || !result.ResidualCopies {
		t.Fatal("authority loss did not retain gated state")
	}
	if _, err := Resume(context.Background(), ref, m, transactionAdmissionFor(t, ref)); err != nil {
		t.Fatalf("fresh independently checked admission failed forward recovery: %v", err)
	}
}
func TestTransactionNeverReplacesUnexpectedRenameDestination(t *testing.T) {
	ref := transactionPrepared(t)
	m := transactionOwner(t, ref)
	sentinel := []byte("unexpected-retained-data")
	target := filepath.Join(ref.Directory, "retained", ".git")
	result, err := transact(context.Background(), ref, m, transactionAdmissionFor(t, ref), false, transactionHooks{checkpoint: func(point string) error {
		if point == "before-rename:retain:.git" {
			return os.WriteFile(target, sentinel, 0600)
		}
		return nil
	}})
	if err == nil || !result.MarkerHeld {
		t.Fatal("unexpected destination was accepted")
	}
	actual, err := os.ReadFile(target)
	if err != nil || string(actual) != string(sentinel) {
		t.Fatal("unexpected destination overwritten")
	}
	if _, err := os.Stat(filepath.Join(ref.Source, ".git")); err != nil {
		t.Fatal("old canonical repository moved despite conflict")
	}
}
