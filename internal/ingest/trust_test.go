package ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

func trustSource(t *testing.T, w *Writer, kind, uri, text string) string {
	t.Helper()
	src, err := store.NewSourceStore(w.Fence.Root).Write([]byte(text), domain.Source{Kind: kind, URI: uri, OccurredAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return src.SHA256
}

func claimByID(t *testing.T, w *Writer, subject, family, id string) []domain.Claim {
	t.Helper()
	var out []domain.Claim
	if w.Config.TierOf(family) == domain.TierShard {
		lines, err := w.Shard.Lines(subject, family)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range lines {
			if c.ID == id {
				out = append(out, c)
			}
		}
		return out
	}
	page, err := w.Fence.ParseEntity(w.Fence.PathFor(DefaultEntityType, subject))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range page.Claims {
		if c.ID == id {
			out = append(out, c)
		}
	}
	return out
}

// TestUntrustedFirstSeenClaimWaitsInInbox is T24.16's ingest acceptance
// (AI-01): a first-seen, non-conflicting machine claim from an imap or
// git_repo source is written pending with actor machine and surfaces as a
// claim_candidate item; the same kind of claim from a file source is active
// exactly as before.
func TestUntrustedFirstSeenClaimWaitsInInbox(t *testing.T) {
	for _, tc := range []struct {
		kind, connector, family, object string
		pending                         bool
	}{
		{"email", "imap", "has_balance", "$0", true},
		{"git_repo", "git_repo", "works_at", "Planted Corp", true},
		{"file", "file", "has_balance", "$0", false},
		{"file", "file", "works_at", "Planted Corp", false},
	} {
		t.Run(tc.kind+"/"+tc.family, func(t *testing.T) {
			w, closeQ := newTestWriter(t)
			defer closeQ()
			ctx := context.Background()
			now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
			sha := trustSource(t, w, tc.kind, "fixture:"+tc.kind, "Ava's balance is $0")
			w.Trust = StoreTrust(w.Config, store.NewSourceStore(w.Fence.Root))
			ds := reviewStore(t, w)
			o := obs("ava-standardo", tc.family, tc.object, sha, "0-19", .9, now)
			plan, err := w.ReviewObservations(ctx, ds, []domain.Observation{o}, now)
			if err != nil {
				t.Fatal(err)
			}
			if tc.pending && (len(plan.Pending) != 1 || len(plan.Ready) != 0) {
				t.Fatalf("untrusted first-seen claim not held: %+v", plan)
			}
			if !tc.pending && (len(plan.Pending) != 0 || len(plan.Ready) != 1) {
				t.Fatalf("trusted claim held: %+v", plan)
			}
			stats, err := w.WriteReviewed(plan)
			if err != nil {
				t.Fatal(err)
			}
			if stats.Written != 1 || (tc.pending && stats.Pending != 1) {
				t.Fatalf("stats=%+v", stats)
			}
			if _, err := writer.Flush(w.Queue, w.Fence.Root); err != nil {
				t.Fatal(err)
			}
			id := ClaimFromObservation(o).ID
			rows := claimByID(t, w, "ava-standardo", tc.family, id)
			if len(rows) != 1 {
				t.Fatalf("rows=%+v", rows)
			}
			got := rows[0]
			if got.Provenance.Actor != "machine" {
				t.Fatalf("actor=%q", got.Provenance.Actor)
			}
			wantState := domain.StateActive
			if tc.pending {
				wantState = domain.StatePending
			}
			if got.State != wantState {
				t.Fatalf("state=%q want %q", got.State, wantState)
			}
			if tc.pending && (got.Provenance.Meta[MetaTrust] != string(config.TrustUntrusted) || got.Provenance.Meta[MetaConnector] != tc.connector) {
				t.Fatalf("pending claim trust metadata: %+v", got.Provenance.Meta)
			}
			if !tc.pending && len(got.Provenance.Meta) != 0 {
				t.Fatalf("trusted claim changed shape: %+v", got.Provenance.Meta)
			}

			created, existing, err := w.StageCandidates(ctx, ds, plan.Pending, now)
			if err != nil {
				t.Fatal(err)
			}
			items, err := ds.List(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.pending {
				if created != 0 || len(items) != 0 {
					t.Fatalf("trusted claim staged: %+v", items)
				}
				return
			}
			if created != 1 || existing != 0 || len(items) != 1 || items[0].Kind != disposition.KindClaimCandidate {
				t.Fatalf("candidate staging: created=%d existing=%d items=%+v", created, existing, items)
			}
			payload, ok, err := ClaimCandidate(items[0])
			if err != nil || !ok {
				t.Fatalf("decode candidate: ok=%v err=%v", ok, err)
			}
			if payload.Claim.ID != id || payload.Connector != tc.connector || payload.SourceKind != tc.kind || payload.SourceURI != "fixture:"+tc.kind || payload.Trust != config.TrustUntrusted {
				t.Fatalf("payload=%+v", payload)
			}

			// A repeat extraction neither re-stages nor duplicates the claim.
			repeat, err := w.ReviewObservations(ctx, ds, []domain.Observation{o}, now.Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			if repeat.AlreadyPresent != 1 || len(repeat.Pending) != 0 || len(repeat.Ready) != 0 {
				t.Fatalf("repeat=%+v", repeat)
			}

			// Pending claims are never disclosure-eligible.
			proj, err := store.LoadMemoryProjection(store.NewSourceStore(w.Fence.Root))
			if err != nil {
				t.Fatal(err)
			}
			if index.ClaimDisclosureEligible(proj, got, true, true, now) || index.ClaimDisclosureEligible(proj, got, false, false, now) {
				t.Fatal("pending claim disclosure-eligible")
			}

			// Accepting activates the same claim identity with the human actor.
			changes, err := w.PlanActivateCandidate(ctx, payload.Claim, "human:ada", now)
			if err != nil {
				t.Fatal(err)
			}
			if err := writer.PublishFiles(w.Queue, w.Fence.Root, changes); err != nil {
				t.Fatal(err)
			}
			if _, err := writer.Flush(w.Queue, w.Fence.Root); err != nil {
				t.Fatal(err)
			}
			var active []domain.Claim
			for _, c := range claimByID(t, w, "ava-standardo", tc.family, id) {
				if c.State == domain.StateActive {
					active = append(active, c)
				}
			}
			if len(active) != 1 || active[0].Provenance.Actor != "human:ada" || active[0].Provenance.SourceSHA256 != sha || active[0].Provenance.Meta[MetaTrust] != string(config.TrustUntrusted) {
				t.Fatalf("activation: %+v", active)
			}
			if !index.ClaimDisclosureEligible(proj, active[0], true, true, now) {
				t.Fatal("activated claim not eligible")
			}
			if _, err := w.PlanActivateCandidate(ctx, payload.Claim, "human:ada", now); err == nil {
				t.Fatal("re-activated an already active claim")
			}
			if _, err := w.PlanActivateCandidate(ctx, payload.Claim, "machine", now); err == nil {
				t.Fatal("activation accepted a non-human actor")
			}
		})
	}
}

// TestUntrustedClaimReviewPathsUnchanged: an untrusted conflicting claim still
// goes to supersession review (out of scope for T24.16), and an untrusted
// claim that agrees with an existing active claim is not first-seen, so it is
// written active -- but marked untrusted for the composer.
func TestUntrustedClaimReviewPathsUnchanged(t *testing.T) {
	w, closeQ := newTestWriter(t)
	defer closeQ()
	ctx := context.Background()
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	fileSHA := trustSource(t, w, "file", "fixture:notes", "Ava works at Acme.")
	mailSHA := trustSource(t, w, "email", "fixture:mail", "Ava works at Acme; Ava works at Globex.")
	w.Trust = StoreTrust(w.Config, store.NewSourceStore(w.Fence.Root))
	ds := reviewStore(t, w)
	prior := obs("ava-standardo", "works_at", "Acme", fileSHA, "0-10", .9, now)
	plan, err := w.ReviewObservations(ctx, ds, []domain.Observation{prior}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteReviewed(plan); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Flush(w.Queue, w.Fence.Root); err != nil {
		t.Fatal(err)
	}
	agree := obs("ava-standardo", "works_at", "Acme", mailSHA, "0-10", .9, now)
	conflict := obs("ava-standardo", "works_at", "Globex", mailSHA, "11-30", .9, now)
	plan, err = w.ReviewObservations(ctx, ds, []domain.Observation{agree, conflict}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Proposals) != 1 || plan.Proposals[0].A.Object != "Globex" || len(plan.Pending) != 0 || len(plan.Ready) != 1 {
		t.Fatalf("plan=%+v", plan)
	}
	if _, err := w.WriteReviewed(plan); err != nil {
		t.Fatal(err)
	}
	rows := claimByID(t, w, "ava-standardo", "works_at", ClaimFromObservation(agree).ID)
	if len(rows) != 1 || rows[0].State != domain.StateActive || rows[0].Provenance.Meta[MetaTrust] != string(config.TrustUntrusted) {
		t.Fatalf("agreeing untrusted claim: %+v", rows)
	}
	raw, err := json.Marshal(plan.Proposals[0].A.Provenance)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Proposals[0].A.Provenance.Meta[MetaTrust] != string(config.TrustUntrusted) {
		t.Fatalf("conflict proposal lost trust: %s", raw)
	}
}

// TestNilTrustKeepsLegacyBehavior: a Writer built without a trust resolver
// (every caller before T24.16) treats observations as trusted.
func TestNilTrustKeepsLegacyBehavior(t *testing.T) {
	w, closeQ := newTestWriter(t)
	defer closeQ()
	ds := reviewStore(t, w)
	now := time.Now()
	plan, err := w.ReviewObservations(context.Background(), ds, []domain.Observation{obs("demo", "works_at", "Acme", "unknown-source", "0-1", .9, now)}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Ready) != 1 || len(plan.Pending) != 0 {
		t.Fatalf("plan=%+v", plan)
	}
}

// TestStoreTrustFailsClosed: a source the store does not hold is untrusted.
func TestStoreTrustFailsClosed(t *testing.T) {
	w, closeQ := newTestWriter(t)
	defer closeQ()
	class := StoreTrust(w.Config, store.NewSourceStore(w.Fence.Root))("0000000000000000000000000000000000000000000000000000000000000000")
	if class.Trust != config.TrustUntrusted {
		t.Fatalf("class=%+v", class)
	}
}
