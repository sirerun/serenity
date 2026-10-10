package releaseauth

import (
	"os"
	"strings"
	"testing"
	"time"
)

func recoveryApprovalFixture(t *testing.T, p Paths, m Manifest, now time.Time, mode string) RecoveryApproval {
	t.Helper()
	pendingBytes, err := os.ReadFile(p.State + ".pending")
	if err != nil {
		t.Fatal(err)
	}
	var pending Pending
	if err = DecodeCanonical(pendingBytes, &pending); err != nil {
		t.Fatal(err)
	}
	a := approvalFixture(t, m, pending.PreviousState)
	a.Mode = mode
	a.IssuedAt = now.Add(-time.Minute).Format(time.RFC3339)
	a.ExpiresAt = now.Add(time.Hour).Format(time.RFC3339)
	return RecoveryApproval{Approval: a, PendingSHA256: Digest(pendingBytes), InstalledManifestSHA256: Digest(encode(t, m))}
}

func TestRootExpiredPendingRequiresFreshExactRecovery(t *testing.T) {
	for _, mode := range []string{"recover-promote", "recover-rollback"} {
		t.Run(mode, func(t *testing.T) {
			p, m := rootFixture(t)
			if mode == "recover-rollback" {
				s := State{Version: 1, HighestSequence: 10, InstalledSequence: 8, ManifestSHA256: strings.Repeat("a", 64)}
				a := approvalFixture(t, m, s)
				a.Mode = "rollback"
				writeTest(t, p.State, encode(t, s))
				writeTest(t, p.Approval, encode(t, a))
			}
			tx, err := OpenTransaction(p)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := tx.Close(); err != nil {
					t.Error(err)
				}
			})
			if _, err = tx.Prepare(testNow); err != nil {
				t.Fatal(err)
			}
			later := testNow.Add(10 * 24 * time.Hour)
			pin := Digest(encode(t, m))
			if _, err = tx.Recover(later, pin); err == nil {
				t.Fatal("expired old approval accepted")
			}
			fresh := recoveryApprovalFixture(t, p, m, later, mode)
			normal := fresh.Approval
			normal.Mode = strings.TrimPrefix(normal.Mode, "recover-")
			writeTest(t, p.Approval, encode(t, normal))
			if _, err = tx.Recover(later, pin); err == nil {
				t.Fatal("ordinary approval waived manifest expiry")
			}
			writeTest(t, p.Approval, encode(t, fresh))
			if _, err = tx.Commit(later); err == nil {
				t.Fatal("ordinary commit accepted recovery approval")
			}
			if _, err = tx.Recover(later, pin); err != nil {
				t.Fatal(err)
			}
			var state State
			raw, err := ReadTrusted(p.State)
			if err != nil {
				t.Fatal(err)
			}
			if err = DecodeCanonical(raw, &state); err != nil {
				t.Fatal(err)
			}
			expectedHigh := uint64(1)
			if mode == "recover-rollback" {
				expectedHigh = 10
			}
			if state.HighestSequence != expectedHigh || state.InstalledSequence != 1 {
				t.Fatalf("wrong recovery state %+v", state)
			}
			if _, err = tx.Verify(later); err == nil {
				t.Fatal("recovery approval accepted for ordinary replay")
			}
		})
	}
}

func TestRootRecoveryApprovalNegatives(t *testing.T) {
	cases := map[string]func(*RecoveryApproval){
		"pending":          func(a *RecoveryApproval) { a.PendingSHA256 = strings.Repeat("f", 64) },
		"installed":        func(a *RecoveryApproval) { a.InstalledManifestSHA256 = strings.Repeat("f", 64) },
		"manifest":         func(a *RecoveryApproval) { a.ManifestSHA256 = strings.Repeat("f", 64) },
		"previous-state":   func(a *RecoveryApproval) { a.PreviousStateSHA256 = strings.Repeat("f", 64) },
		"commit":           func(a *RecoveryApproval) { a.Commit = strings.Repeat("f", 40) },
		"archive":          func(a *RecoveryApproval) { a.ArchiveSHA256 = strings.Repeat("f", 64) },
		"sequence":         func(a *RecoveryApproval) { a.Sequence++ },
		"expired-approval": func(a *RecoveryApproval) { a.ExpiresAt = testNow.Add(9 * 24 * time.Hour).Format(time.RFC3339) },
		"future-approval":  func(a *RecoveryApproval) { a.IssuedAt = testNow.Add(11 * 24 * time.Hour).Format(time.RFC3339) },
		"wrong-transition": func(a *RecoveryApproval) { a.Mode = "recover-rollback" },
		"ordinary-mode":    func(a *RecoveryApproval) { a.Mode = "promote" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			p, m := rootFixture(t)
			tx, err := OpenTransaction(p)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := tx.Close(); err != nil {
					t.Error(err)
				}
			})
			if _, err = tx.Prepare(testNow); err != nil {
				t.Fatal(err)
			}
			later := testNow.Add(10 * 24 * time.Hour)
			a := recoveryApprovalFixture(t, p, m, later, "recover-promote")
			change(&a)
			writeTest(t, p.Approval, encode(t, a))
			if _, err = tx.Recover(later, Digest(encode(t, m))); err == nil {
				t.Fatal("unsafe recovery approval accepted")
			}
		})
	}
}

func TestRootRecoveryRetainsCurrentTrustAndChronology(t *testing.T) {
	for _, name := range []string{"signature", "archive", "revoked-key", "future-manifest", "invalid-original-window", "already-written"} {
		t.Run(name, func(t *testing.T) {
			p, m := rootFixture(t)
			tx, err := OpenTransaction(p)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := tx.Close(); err != nil {
					t.Error(err)
				}
			})
			if _, err = tx.Prepare(testNow); err != nil {
				t.Fatal(err)
			}
			later := testNow.Add(10 * 24 * time.Hour)
			a := recoveryApprovalFixture(t, p, m, later, "recover-promote")
			switch name {
			case "signature":
				writeTest(t, p.Signature, []byte("not a signature"))
			case "archive":
				writeTest(t, p.Archive, []byte("not the archive"))
			case "revoked-key":
				raw, err := ReadTrusted(p.Policy)
				if err != nil {
					t.Fatal(err)
				}
				var policy Policy
				if err = DecodeCanonical(raw, &policy); err != nil {
					t.Fatal(err)
				}
				policy.KeyID = strings.Replace(policy.KeyID, "123456789012", "999999999999", 1)
				writeTest(t, p.Policy, encode(t, policy))
			case "future-manifest", "invalid-original-window":
				// An independently signed replacement still cannot alter the exact pending
				// artifact or bypass chronology. Model a hostile root-file fixture to reach
				// chronology checks without trusting the fixture as an issued transaction.
				changed, policy, archive, _, key := fixture(t)
				m = changed
				if name == "future-manifest" {
					m.IssuedAt = later.Add(time.Hour).Format(time.RFC3339)
					m.ExpiresAt = later.Add(2 * time.Hour).Format(time.RFC3339)
				} else {
					m.ExpiresAt = m.IssuedAt
				}
				raw := encode(t, m)
				writeTest(t, p.Manifest, raw)
				writeTest(t, p.Signature, signFixture(t, key, raw))
				writeTest(t, p.Policy, encode(t, policy))
				writeTest(t, p.Archive, archive)
				pending := Pending{PreviousState: genesis(), Version: 1, PreviousStateSHA256: Digest(encode(t, genesis())), ManifestSHA256: Digest(raw)}
				writeTest(t, p.State+".pending", encode(t, pending))
				a = recoveryApprovalFixture(t, p, m, later, "recover-promote")
			case "already-written":
				writeTest(t, p.State, encode(t, State{Version: 1, HighestSequence: 1, InstalledSequence: 1, ManifestSHA256: Digest(encode(t, m))}))
			}
			writeTest(t, p.Approval, encode(t, a))
			_, err = tx.Recover(later, Digest(encode(t, m)))
			if name == "already-written" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("recovery weakened artifact trust or chronology")
			}
		})
	}
}
