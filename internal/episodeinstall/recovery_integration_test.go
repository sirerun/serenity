//go:build darwin || linux

package episodeinstall

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/pool"
	"github.com/sirerun/serenity/internal/writer"
)

// This is a synthetic trusted issuer, not production authentication. Its
// ordinary admission checks every prepared pin independently of recovery phases.
type recoveryIntegrationAdmission struct {
	transactionAdmission
	phases []string
	reject string
}

func (a *recoveryIntegrationAdmission) CheckInitialRecovery(_ context.Context, p InitialRecoveryPins) error {
	if p.Purpose != "serenity.initial-maintenance-reconciliation/v1" {
		return ErrUnqualified
	}
	switch p.Phase {
	case "precreate":
		if p.RecordSHA256 != "" || p.Original.Present || p.Current.Present {
			return ErrUnqualified
		}
	case "record", "append", "return":
		if len(p.RecordSHA256) != 64 || !p.Original.Present || !p.Current.Present {
			return ErrUnqualified
		}
	default:
		return ErrUnqualified
	}
	a.phases = append(a.phases, string(p.Phase))
	if string(p.Phase) == a.reject {
		return ErrUnqualified
	}
	return nil
}

func TestInitialRecoveryRealPoolAndInstaller(t *testing.T) {
	for _, stage := range []string{"absent", "zero", "prefix", "full"} {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			ref := transactionPrepared(t)
			h, err := loadHeader(ctx, ref)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(ref.Identity)
			if err != nil {
				t.Fatal(err)
			}
			raw = append(raw, '\n')
			marker := filepath.Join(ref.Source, ".serenity", "canonical-maintenance.json")
			var original os.FileInfo
			if stage != "absent" {
				initial := raw
				if stage == "zero" {
					initial = raw[:0]
				}
				if stage == "prefix" {
					initial = raw[:len(raw)/2]
				}
				if err = os.WriteFile(marker, initial, 0600); err != nil {
					t.Fatal(err)
				}
				original, err = os.Stat(marker)
				if err != nil {
					t.Fatal(err)
				}
			}
			locks := map[string]os.FileInfo{}
			for _, name := range []string{"writer.lock", "canonical-view.lock"} {
				locks[name], err = os.Stat(filepath.Join(ref.Source, ".serenity", name))
				if err != nil {
					t.Fatal(err)
				}
			}
			issuer := &recoveryIntegrationAdmission{transactionAdmission: transactionAdmissionFor(t, ref)}
			owner, result, err := RecoverInitialMaintenance(ctx, ref, issuer)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = owner.Close() }()
			if result.State != "initial-gate-reconciled" || !result.MarkerHeld || !result.ResidualCopies {
				t.Fatalf("incorrect recovery boundary: %+v", result)
			}
			for _, c := range h.Components {
				if verifyInventory(ctx, filepath.Join(ref.Source, filepath.FromSlash(c.Name)), c.Old) != nil {
					t.Fatal("recovery changed canonical component")
				}
			}
			now, err := os.Stat(marker)
			if err != nil {
				t.Fatal(err)
			}
			if original != nil && !os.SameFile(original, now) {
				t.Fatal("recovery replaced existing marker inode")
			}
			for name, before := range locks {
				after, err := os.Stat(filepath.Join(ref.Source, ".serenity", name))
				if err != nil || !os.SameFile(before, after) {
					t.Fatal("stable ownership inode changed")
				}
			}
			expected := []string{"record", "append", "return"}
			if stage == "absent" {
				expected = append([]string{"precreate"}, expected...)
			}
			// Fresh checks may repeat a phase around durability boundaries.
			var observed []string
			for _, phase := range issuer.phases {
				if len(observed) == 0 || observed[len(observed)-1] != phase {
					observed = append(observed, phase)
				}
			}
			if !reflect.DeepEqual(observed, expected) {
				t.Fatalf("unexpected admission phases %v", issuer.phases)
			}
			p, err := pool.New(pool.Config{BrainsRoot: filepath.Dir(ref.Source), MaxOpen: 1, MaxInFlight: 1, Embedder: prepareEmbedding{}})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := p.Close(); err != nil {
					t.Error(err)
				}
			}()
			if _, release, err := p.AcquireCanonical(ctx, ref.Identity.BrainID); err == nil {
				release()
				t.Fatal("pool crossed recovered maintenance gate")
			}
			if _, err = Install(ctx, ref, owner, issuer); err != nil {
				t.Fatal(err)
			}
			if _, err = Resume(ctx, ref, owner, issuer); err != nil {
				t.Fatal(err)
			}
			if err = owner.Close(); err != nil {
				t.Fatal(err)
			}
			if other, err := writer.AcquireBrain(ref.Source); err == nil {
				_ = other.Close()
				t.Fatal("closing owner removed persistent gate")
			}
			// Only this synthetic test owns closeout. Recovery itself must never Finish.
			owner, err = writer.ResumeBrainMaintenance(ref.Source, ref.Identity)
			if err != nil {
				t.Fatal(err)
			}
			if err = owner.Finish(ref.Identity); err != nil {
				_ = owner.Close()
				t.Fatal(err)
			}
			if err = owner.Close(); err != nil {
				t.Fatal(err)
			}
			runtime, release, err := p.AcquireCanonical(ctx, ref.Identity.BrainID)
			if err != nil {
				t.Fatal(err)
			}
			if runtime == nil {
				release()
				t.Fatal("installed runtime absent")
			}
			release()
		})
	}
}

// Abrupt exit skips all defers. Each continuation must reacquire actual locks,
// qualify immutable copies again and obtain fresh initial-recovery admission.
func TestInitialRecoveryProcessDeath(t *testing.T) {
	points := []string{"initial:before-gate", "initial:after-gate", "initial:before-gate-sync", "initial:after-gate-sync", "initial:before-complete", "initial:after-complete"}
	for _, event := range []string{"before-record:", "after-record-create:", "after-record-midwrite:", "before-record-file-sync:", "after-record-file-sync:", "before-record-rename:", "after-record-rename:", "before-record-directory-sync:", "after-record-directory-sync:"} {
		points = append(points, event+"initial-recovery.json")
	}
	for _, point := range points {
		t.Run(strings.ReplaceAll(point, ":", "_"), func(t *testing.T) {
			ref := transactionPrepared(t)
			raw, err := json.Marshal(ref.Identity)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestInitialRecoveryDeathChild$")
			cmd.Env = append(os.Environ(), "SERENITY_INITIAL_DEATH_CHILD=1", "SERENITY_INITIAL_SOURCE="+ref.Source, "SERENITY_INITIAL_DIRECTORY="+ref.Directory, "SERENITY_INITIAL_HEADER="+ref.HeaderSHA256, "SERENITY_INITIAL_IDENTITY="+string(raw), "SERENITY_INITIAL_POINT="+point)
			output, err := cmd.CombinedOutput()
			exit, ok := err.(*exec.ExitError)
			if !ok || exit.ExitCode() != 23 {
				t.Fatalf("death boundary not reached: %v %s", err, output)
			}
			// Before-gate dies before persistent creation; every other boundary must
			// retain a gate that ordinary cooperating readers and writers refuse.
			if point != "initial:before-gate" {
				if owner, err := writer.AcquireBrain(ref.Source); err == nil {
					_ = owner.Close()
					t.Fatal("death removed persistent writer fence")
				}
				if view, err := writer.AcquireCanonicalRead(ref.Source); err == nil {
					_ = view.Close()
					t.Fatal("death removed persistent reader fence")
				}
			}
			admission := &recoveryIntegrationAdmission{transactionAdmission: transactionAdmissionFor(t, ref)}
			owner, result, err := RecoverInitialMaintenance(context.Background(), ref, admission)
			if err != nil {
				t.Fatalf("fresh recovery refused legitimate death boundary: %v", err)
			}
			defer func() { _ = owner.Close() }()
			if !result.MarkerHeld || !result.ResidualCopies {
				t.Fatal("recovery overstated completion")
			}
			if _, err = Install(context.Background(), ref, owner, admission); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestInitialRecoveryDeathChild(t *testing.T) {
	if os.Getenv("SERENITY_INITIAL_DEATH_CHILD") != "1" {
		t.Skip("subprocess only")
	}
	ref := PreparedRef{Source: os.Getenv("SERENITY_INITIAL_SOURCE"), Directory: os.Getenv("SERENITY_INITIAL_DIRECTORY"), HeaderSHA256: os.Getenv("SERENITY_INITIAL_HEADER"), ResidualCopies: true}
	if err := json.Unmarshal([]byte(os.Getenv("SERENITY_INITIAL_IDENTITY")), &ref.Identity); err != nil {
		t.Fatal(err)
	}
	admission := &recoveryIntegrationAdmission{transactionAdmission: transactionAdmissionFor(t, ref)}
	_, _, err := recoverInitialMaintenance(context.Background(), ref, admission, transactionHooks{checkpoint: func(point string) error {
		if point == os.Getenv("SERENITY_INITIAL_POINT") {
			os.Exit(23)
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Fatal("death boundary was not reached")
}

func TestInitialRecoveryUnattributedGateRequiresDistinctAdmission(t *testing.T) {
	ctx := context.Background()
	req := prepareFixture(t)
	first, err := Prepare(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	req.OperationID = hashBytes([]byte("second independently prepared operation"))
	req.OperationDirectory = filepath.Join(filepath.Dir(req.OperationDirectory), req.OperationID)
	second, err := Prepare(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Identity.SourceHead != second.Identity.SourceHead || first.Identity.FenceDigest != second.Identity.FenceDigest {
		t.Fatal("fixture did not preserve ambiguity")
	}
	marker := filepath.Join(req.Source, ".serenity", "canonical-maintenance.json")
	// A lost creator left no identity bytes. Matching B's source proves no
	// historical attribution; the distinct role must authorize reconciliation now.
	if err = os.WriteFile(marker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	ordinary := transactionAdmissionFor(t, second)
	if _, ok := any(ordinary).(InitialRecoveryAdmission); ok {
		t.Fatal("ordinary cold role implicitly grants initial recovery")
	}
	denied := &recoveryIntegrationAdmission{transactionAdmission: ordinary, reject: "record"}
	owner, _, err := RecoverInitialMaintenance(ctx, second, denied)
	if owner != nil || err == nil {
		t.Fatal("distinct role rejection was bypassed")
	}
	raw, err := os.ReadFile(marker)
	if err != nil || len(raw) != 0 {
		t.Fatal("rejected initial role changed gate")
	}
	if _, err = os.Stat(filepath.Join(second.Directory, "initial-recovery.json")); !os.IsNotExist(err) {
		t.Fatal("rejection published recovery evidence")
	}
	admitted := &recoveryIntegrationAdmission{transactionAdmission: ordinary}
	owner, result, err := RecoverInitialMaintenance(ctx, second, admitted)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close() }()
	if !result.ResidualCopies || !result.MarkerHeld {
		t.Fatal("initial reconciliation claimed predecessor completion")
	}
	evidence, err := os.ReadFile(filepath.Join(second.Directory, "initial-recovery.json"))
	if err != nil || !bytes.Contains(evidence, []byte("unattributed")) {
		t.Fatal("evidence asserted historical intent")
	}
	if _, err = os.Stat(filepath.Join(first.Directory, headerName)); err != nil {
		t.Fatal("other prepared operation was disposed")
	}
}

func TestInitialRecoveryFreshAuthorityAfterReturnRejection(t *testing.T) {
	ctx := context.Background()
	ref := transactionPrepared(t)
	rejected := &recoveryIntegrationAdmission{transactionAdmission: transactionAdmissionFor(t, ref), reject: "return"}
	owner, _, err := RecoverInitialMaintenance(ctx, ref, rejected)
	if err == nil || owner != nil {
		t.Fatal("expired final authority returned usable owner")
	}
	record := filepath.Join(ref.Directory, "initial-recovery.json")
	before, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if view, err := writer.AcquireCanonicalRead(ref.Source); err == nil {
		_ = view.Close()
		t.Fatal("failed return removed gate")
	}
	fresh := &recoveryIntegrationAdmission{transactionAdmission: transactionAdmissionFor(t, ref)}
	owner, result, err := RecoverInitialMaintenance(ctx, ref, fresh)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close() }()
	after, err := os.ReadFile(record)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("retry reinterpreted current longer marker as original")
	}
	if !result.MarkerHeld || !result.ResidualCopies {
		t.Fatal("retry overstated completion")
	}
}
