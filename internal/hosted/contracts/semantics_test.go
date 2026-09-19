package contracts_test

import (
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

type legalTransition struct {
	from, to contracts.OperationPhase
	actor    contracts.ResolutionActor
	kind     contracts.EvidenceKind
	hasRef   bool
}

// legal is the reviewed, independently written statement of the operation
// state machine. TestValidateTransitionIsExhaustive proves ValidateTransition
// accepts exactly these tuples and rejects every other combination of phase,
// actor, evidence kind and reference presence.
var legal = []legalTransition{
	{contracts.OperationReserved, contracts.OperationCommitted, contracts.ActorCaller, contracts.EvidenceCommitted, true},
	{contracts.OperationReserved, contracts.OperationCommitted, contracts.ActorReconciler, contracts.EvidenceCommitted, true},
	{contracts.OperationReserved, contracts.OperationReleased, contracts.ActorCaller, contracts.EvidenceNoCanonicalAttempt, false},
	{contracts.OperationReserved, contracts.OperationReleased, contracts.ActorReconciler, contracts.EvidenceAbsent, true},
	{contracts.OperationReserved, contracts.OperationPendingReview, contracts.ActorCaller, contracts.EvidenceUnknown, true},
	{contracts.OperationReserved, contracts.OperationPendingReview, contracts.ActorReconciler, contracts.EvidenceUnknown, true},
	{contracts.OperationPendingReview, contracts.OperationCommitted, contracts.ActorOperator, contracts.EvidenceOperatorReview, true},
	{contracts.OperationPendingReview, contracts.OperationReleased, contracts.ActorOperator, contracts.EvidenceOperatorReview, true},
}

func TestValidateTransitionIsExhaustive(t *testing.T) {
	phases := []contracts.OperationPhase{contracts.OperationReserved, contracts.OperationCommitted, contracts.OperationReleased, contracts.OperationPendingReview, "bogus"}
	actors := []contracts.ResolutionActor{contracts.ActorCaller, contracts.ActorReconciler, contracts.ActorOperator, "stranger"}
	kinds := []contracts.EvidenceKind{"", contracts.EvidenceCommitted, contracts.EvidenceAbsent, contracts.EvidenceNoCanonicalAttempt, contracts.EvidenceUnknown, contracts.EvidenceOperatorReview}
	want := map[legalTransition]bool{}
	for _, l := range legal {
		want[l] = true
	}
	checked, accepted := 0, 0
	for _, from := range phases {
		for _, to := range phases {
			for _, actor := range actors {
				for _, kind := range kinds {
					for _, hasRef := range []bool{false, true} {
						ref := ""
						if hasRef {
							ref = "ref-1"
						}
						err := contracts.ValidateTransition(from, to, actor, contracts.Evidence{Kind: kind, Ref: ref})
						key := legalTransition{from, to, actor, kind, hasRef}
						if want[key] != (err == nil) {
							t.Errorf("%s→%s by %s with %q ref=%v: legal=%v but err=%v", from, to, actor, kind, hasRef, want[key], err)
						}
						checked++
						if err == nil {
							accepted++
						}
					}
				}
			}
		}
	}
	if accepted != len(legal) {
		t.Fatalf("accepted %d combinations, want exactly %d", accepted, len(legal))
	}
	if checked != len(phases)*len(phases)*len(actors)*len(kinds)*2 {
		t.Fatalf("checked %d combinations, loop did not cover the space", checked)
	}
}

func TestValidateTransitionErrorClasses(t *testing.T) {
	// A final phase has no exit: the error is a transition error, not an evidence error.
	err := contracts.ValidateTransition(contracts.OperationCommitted, contracts.OperationReleased, contracts.ActorOperator, contracts.Evidence{Kind: contracts.EvidenceOperatorReview, Ref: "r"})
	if !errors.Is(err, contracts.ErrOperationTransition) {
		t.Fatalf("committed→released: %v, want ErrOperationTransition", err)
	}
	// The caller can never resolve pending_review; only an operator can.
	err = contracts.ValidateTransition(contracts.OperationPendingReview, contracts.OperationReleased, contracts.ActorCaller, contracts.Evidence{Kind: contracts.EvidenceAbsent, Ref: "h"})
	if !errors.Is(err, contracts.ErrOperationTransition) {
		t.Fatalf("caller resolving pending_review: %v, want ErrOperationTransition", err)
	}
	// A caller can never release on its own absence check: only the fenced
	// reconciler path may present EvidenceAbsent.
	err = contracts.ValidateTransition(contracts.OperationReserved, contracts.OperationReleased, contracts.ActorCaller, contracts.Evidence{Kind: contracts.EvidenceAbsent, Ref: "head-1"})
	if !errors.Is(err, contracts.ErrOperationEvidence) {
		t.Fatalf("caller releasing on absence: %v, want ErrOperationEvidence", err)
	}
	// The reconciler can never release on "unknown": absence needs proof.
	err = contracts.ValidateTransition(contracts.OperationReserved, contracts.OperationReleased, contracts.ActorReconciler, contracts.Evidence{Kind: contracts.EvidenceUnknown, Ref: "io"})
	if !errors.Is(err, contracts.ErrOperationEvidence) {
		t.Fatalf("reconciler releasing on unknown: %v, want ErrOperationEvidence", err)
	}
}

func TestPhaseClassification(t *testing.T) {
	for phase, want := range map[contracts.OperationPhase][2]bool{ // {final, holds capacity}
		contracts.OperationReserved:      {false, true},
		contracts.OperationPendingReview: {false, true},
		contracts.OperationCommitted:     {true, false},
		contracts.OperationReleased:      {true, false},
	} {
		if phase.Final() != want[0] || phase.HoldsCapacity() != want[1] {
			t.Errorf("%s: Final=%v HoldsCapacity=%v, want %v", phase, phase.Final(), phase.HoldsCapacity(), want)
		}
	}
}

func TestReserveRequestValidate(t *testing.T) {
	ok := contracts.ReserveRequest{AccountID: "a", BrainID: "b", QuotaPeriod: "2026-09", Source: "gateway.remember", LeaseFor: time.Minute,
		Deltas: []contracts.ReserveDelta{{Metric: "writes", Units: 1, Limit: 60}, {Metric: "input_tokens", Units: 12, Limit: 1000}}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	for name, mutate := range map[string]func(*contracts.ReserveRequest){
		"no account":              func(r *contracts.ReserveRequest) { r.AccountID = "" },
		"no period":               func(r *contracts.ReserveRequest) { r.QuotaPeriod = "" },
		"no lease":                func(r *contracts.ReserveRequest) { r.LeaseFor = 0 },
		"key without fingerprint": func(r *contracts.ReserveRequest) { r.ClientKey = "k"; r.Fingerprint = "" },
		"no deltas":               func(r *contracts.ReserveRequest) { r.Deltas = nil },
		"negative units": func(r *contracts.ReserveRequest) {
			r.Deltas = []contracts.ReserveDelta{{Metric: "writes", Units: -1, Limit: 1}}
		},
		"duplicate metric": func(r *contracts.ReserveRequest) {
			r.Deltas = []contracts.ReserveDelta{{Metric: "writes", Units: 1, Limit: 9}, {Metric: "writes", Units: 1, Limit: 9}}
		},
	} {
		bad := ok
		mutate(&bad)
		if err := bad.Validate(); !errors.Is(err, contracts.ErrOperationInvalid) {
			t.Errorf("%s: err=%v, want ErrOperationInvalid", name, err)
		}
	}
}

func TestFenceReceiptRequiresEveryFact(t *testing.T) {
	full := contracts.FenceReceipt{Generation: 2, JournalSeal: contracts.DeletionWatermark{Generation: 2, SequenceID: 7, EntryHash: "h"},
		OldInstanceStopped: true, OldCredentialsRevoked: true, VerifiedAt: time.Unix(1, 0)}
	if err := full.Sufficient(); err != nil {
		t.Fatalf("complete receipt rejected: %v", err)
	}
	for name, mutate := range map[string]func(*contracts.FenceReceipt){
		"no seal":              func(f *contracts.FenceReceipt) { f.JournalSeal = contracts.DeletionWatermark{} },
		"seal of other gen":    func(f *contracts.FenceReceipt) { f.JournalSeal.Generation = 1 },
		"seal without hash":    func(f *contracts.FenceReceipt) { f.JournalSeal.EntryHash = "" },
		"instance not stopped": func(f *contracts.FenceReceipt) { f.OldInstanceStopped = false },
		"credentials live":     func(f *contracts.FenceReceipt) { f.OldCredentialsRevoked = false },
		"unverified":           func(f *contracts.FenceReceipt) { f.VerifiedAt = time.Time{} },
	} {
		f := full
		mutate(&f)
		if err := f.Sufficient(); !errors.Is(err, contracts.ErrFenceInsufficient) {
			t.Errorf("%s: err=%v, want ErrFenceInsufficient", name, err)
		}
	}
	// Every missing fact is reported, not just the first.
	err := contracts.FenceReceipt{}.Sufficient()
	for _, part := range []string{"journal seal", "instance stop", "credential"} {
		if err == nil || !strings.Contains(err.Error(), part) {
			t.Errorf("empty receipt error %v does not mention %q", err, part)
		}
	}
}

func TestRecoveryResultConsistency(t *testing.T) {
	sealed := contracts.FenceReceipt{Generation: 1, JournalSeal: contracts.DeletionWatermark{Generation: 1, SequenceID: 1, EntryHash: "h"},
		OldInstanceStopped: true, OldCredentialsRevoked: true, VerifiedAt: time.Unix(1, 0)}
	if err := (contracts.RecoveryApplyResult{AccountID: "a", Unfrozen: true, Fence: sealed}).Consistent(); err != nil {
		t.Errorf("fenced unfreeze rejected: %v", err)
	}
	if err := (contracts.RecoveryApplyResult{AccountID: "a", Unfrozen: true}).Consistent(); !errors.Is(err, contracts.ErrFenceInsufficient) {
		t.Errorf("unfreeze without a fence: %v, want ErrFenceInsufficient", err)
	}
	if err := (contracts.RecoveryApplyResult{AccountID: "a"}).Consistent(); err == nil {
		t.Error("refusal without a reason accepted")
	}
	if err := (contracts.RecoveryApplyResult{AccountID: "a", Reason: "journal incomplete"}).Consistent(); err != nil {
		t.Errorf("reasoned refusal rejected: %v", err)
	}
}

func TestDeletionEntryValidateRejectsPersonalData(t *testing.T) {
	good := contracts.DeletionEntry{SubjectType: contracts.DeletionSubjectAccount, SubjectID: "acc_01HZ", Outcome: contracts.DeletionIntentRequested}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid entry rejected: %v", err)
	}
	for _, id := range []string{"", "person@example.com", "has space", "a/b"} {
		bad := good
		bad.SubjectID = id
		if err := bad.Validate(); !errors.Is(err, contracts.ErrDeletionEntryInvalid) {
			t.Errorf("subject %q: %v, want ErrDeletionEntryInvalid", id, err)
		}
	}
}

func TestJournalKeysSortInAppendOrder(t *testing.T) {
	var keys []string
	for _, gen := range []int64{1, 2, 10} {
		for _, seq := range []int64{1, 2, 9, 10, 11, 100, 1000} {
			keys = append(keys, contracts.JournalKey(gen, seq))
		}
	}
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	for i := range keys {
		if keys[i] != sorted[i] {
			t.Fatalf("key %q is out of append order (lexicographic position %q)", keys[i], sorted[i])
		}
	}
}

func TestStageMeterEnforcesCeiling(t *testing.T) {
	m := contracts.NewStageMeter(contracts.StageTicket{CeilingBytes: 100})
	if err := m.Add(60); err != nil {
		t.Fatal(err)
	}
	if err := m.Add(40); err != nil {
		t.Fatalf("exactly reaching the ceiling must be allowed: %v", err)
	}
	if err := m.Add(1); !errors.Is(err, contracts.ErrStageCeilingExceeded) {
		t.Fatalf("one byte past the ceiling: %v, want ErrStageCeilingExceeded", err)
	}
	if m.Used() != 100 {
		t.Fatalf("a refused Add must record nothing; used=%d", m.Used())
	}
	if err := m.Add(-1); err == nil {
		t.Fatal("negative bytes accepted")
	}
}

func TestRequestFingerprintBindsPayloadIdentity(t *testing.T) {
	fp := func(kind string, fields ...contracts.FingerprintField) string {
		t.Helper()
		v, err := contracts.RequestFingerprint(kind, fields)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	fact := func(text string) contracts.FingerprintField {
		return contracts.FingerprintField{Name: "fact", Value: []byte(text)}
	}
	brain := contracts.FingerprintField{Name: "brain", Value: []byte("b1")}

	base := fp("remember", fact("the sky is blue"), brain)
	if base != fp("remember", brain, fact("the sky is blue")) {
		t.Error("field order changed the fingerprint")
	}
	// Same operation kind, same brain and the same character count, different fact.
	for name, other := range map[string]string{
		"different fact, equal length": fp("remember", fact("the sky is bleu"), brain),
		"different kind":               fp("forget", fact("the sky is blue"), brain),
		"different brain":              fp("remember", fact("the sky is blue"), contracts.FingerprintField{Name: "brain", Value: []byte("b2")}),
		// Length prefixes stop "ab"+"c" colliding with "a"+"bc" across fields.
		"boundary shift": fp("remember", contracts.FingerprintField{Name: "fact", Value: []byte("the sky is blu")}, contracts.FingerprintField{Name: "brain", Value: []byte("eb1")}),
	} {
		if other == base {
			t.Errorf("%s produced the same fingerprint", name)
		}
	}
	if _, err := contracts.RequestFingerprint("", nil); !errors.Is(err, contracts.ErrOperationInvalid) {
		t.Errorf("empty kind: %v", err)
	}
	if _, err := contracts.RequestFingerprint("remember", []contracts.FingerprintField{fact("a"), fact("b")}); !errors.Is(err, contracts.ErrOperationInvalid) {
		t.Errorf("duplicate field: %v", err)
	}
}
