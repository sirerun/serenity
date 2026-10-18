package gateway

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/deletion"
	"github.com/sirerun/serenity/internal/hosted/pool"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

type episodeFaultJournal struct {
	contracts.DeletionJournal
	mode  string
	calls int
}

func (j *episodeFaultJournal) AppendDeletion(ctx context.Context, e contracts.DeletionEntry) (contracts.DeletionEntry, error) {
	j.calls++
	if j.mode == "fail" {
		return contracts.DeletionEntry{}, errors.New("journal unavailable")
	}
	out, err := j.DeletionJournal.AppendDeletion(ctx, e)
	if err != nil {
		return out, err
	}
	if j.mode == "ambiguous" {
		return contracts.DeletionEntry{}, errors.New("response lost after append")
	}
	if j.mode == "wrong-receipt" {
		out.SubjectID = "wrong"
	}
	return out, nil
}
func episodeReplayHTTPSetup(t *testing.T) (*Gateway, *pool.Pool, string, string, *episodeFaultJournal) {
	t.Helper()
	g, p, brain, token := episodeHTTPSetup(t)
	journal, err := deletion.NewFilesystemJournal(t.TempDir(), "episode-replay-http", 1, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	j := &episodeFaultJournal{DeletionJournal: journal}
	g.Journal = j
	return g, p, brain, token, j
}
func episodeExcludeRequest(brain string, in store.EpisodeInput) episodeRequest {
	gen := uint64(0)
	req := episodeReadRequest(brain, "exclude")
	req.Generation = &gen
	req.Mutation = &store.EpisodeInput{Operation: "exclude", ProducerID: in.ProducerID, ConversationID: in.ConversationID, CommandID: "external-delete"}
	return req
}
func TestEpisodeReplayHTTPSuccessRequiresExternalIntentAndStableRetry(t *testing.T) {
	g, _, brain, token, j := episodeReplayHTTPSetup(t)
	req := episodeHTTPFixture(t, brain)
	if code, _, body := episodeHTTPCall(t, g, token, req); code != 200 {
		t.Fatalf("append %d %s", code, body)
	}
	exclude := episodeExcludeRequest(brain, *req.Mutation)
	code, res, body := episodeHTTPCall(t, g, token, exclude)
	if code != 200 || res.Status != "excluded" || res.PhysicalPurge != "unsupported" || res.ManagedRestore != "unsupported" || res.Receipt == nil {
		t.Fatalf("exclude %d %s", code, body)
	}
	history, err := j.ReadThrough(context.Background(), contracts.DeletionWatermark{})
	if err != nil || len(history.Entries) != 1 {
		t.Fatalf("no independent intent %v %+v", err, history)
	}
	e := history.Entries[0]
	id, err := contracts.ParseEpisodeDeletionIdentity(e.SubjectID)
	if err != nil || e.SubjectType != contracts.DeletionSubjectEpisodeConversation || id.BrainHash != contracts.EpisodeDeletionBrainHash(brain) || id.CommandHash != res.Receipt.CommandKey || id.RequestDigest != res.Receipt.Digest {
		t.Fatalf("incorrect independent identity %v %+v", err, id)
	}
	if strings.Contains(e.SubjectID, req.Mutation.ConversationID) || strings.Contains(e.SubjectID, brain) {
		t.Fatal("journal contains original identity")
	}
	*exclude.Generation = res.Generation
	code, retry, body := episodeHTTPCall(t, g, token, exclude)
	if code != 200 || !reflect.DeepEqual(retry.Receipt, res.Receipt) || j.calls != 1 {
		t.Fatalf("unstable retry %d %s calls=%d", code, body, j.calls)
	}
	read := episodeReadRequest(brain, "read")
	read.CommandKey = store.EpisodeCommandKey(req.Mutation.ProducerID, req.Mutation.CommandID)
	if code, _, body = episodeHTTPCall(t, g, token, read); code != 404 || !strings.Contains(body, `"error":"record_unavailable"`) {
		t.Fatalf("read exclusion %d %s", code, body)
	}
	*req.Generation = res.Generation
	if code, _, body = episodeHTTPCall(t, g, token, req); code != 410 || !strings.Contains(body, `"error":"excluded"`) {
		t.Fatalf("stale queue %d %s", code, body)
	}
}
func TestEpisodeReplayHTTPJournalFailuresDenyFollowupAndReconcileRetry(t *testing.T) {
	for _, mode := range []string{"fail", "ambiguous", "wrong-receipt"} {
		t.Run(mode, func(t *testing.T) {
			g, p, brain, token, j := episodeReplayHTTPSetup(t)
			req := episodeHTTPFixture(t, brain)
			if code, _, body := episodeHTTPCall(t, g, token, req); code != 200 {
				t.Fatalf("append %d %s", code, body)
			}
			exclude := episodeExcludeRequest(brain, *req.Mutation)
			j.mode = mode
			if code, _, body := episodeHTTPCall(t, g, token, exclude); code != 503 || !strings.Contains(body, `"error"`) {
				t.Fatalf("failed intent acknowledged %d %s", code, body)
			}
			read := episodeReadRequest(brain, "read")
			read.CommandKey = store.EpisodeCommandKey(req.Mutation.ProducerID, req.Mutation.CommandID)
			for _, follow := range []episodeRequest{read, req, episodeReadRequest(brain, "fences"), episodeReadRequest(brain, "context")} {
				if code, _, body := episodeHTTPCall(t, g, token, follow); code != 503 || !strings.Contains(body, "exclusion_replay_pending") {
					t.Fatalf("followup not closed %d %s", code, body)
				}
			}
			// Quarantine exists independently of the in-memory attempted-intent map.
			projection, err := store.LoadEpisodes(context.Background(), store.NewSourceStore(filepath.Join(p.BrainsRoot(), brain)))
			if err != nil || len(projection.Fences) != 1 {
				t.Fatalf("no persistent quarantine %v", err)
			}
			if err = writer.EpisodesPublished(context.Background(), filepath.Join(p.BrainsRoot(), brain), projection); err != nil {
				t.Fatal(err)
			}
			g.mu.Lock()
			g.episodeReplay = nil
			g.mu.Unlock()
			if code, _, body := episodeHTTPCall(t, g, token, read); code != 503 || !strings.Contains(body, "exclusion_replay_pending") {
				t.Fatalf("memory reset reopened brain %d %s", code, body)
			}
			j.mode = ""
			if code, res, body := episodeHTTPCall(t, g, token, exclude); code != 200 || res.Status != "excluded" {
				t.Fatalf("identical repair retry %d %s", code, body)
			}
			if code, _, body := episodeHTTPCall(t, g, token, read); code != 404 || !strings.Contains(body, "record_unavailable") {
				t.Fatalf("repaired read %d %s", code, body)
			}
		})
	}
}
func TestEpisodeReplayHTTPCanonicalFailureHasExternalIntent(t *testing.T) {
	g, p, brain, token, j := episodeReplayHTTPSetup(t)
	req := episodeHTTPFixture(t, brain)
	if code, _, body := episodeHTTPCall(t, g, token, req); code != 200 {
		t.Fatalf("append %d %s", code, body)
	}
	root := filepath.Join(p.BrainsRoot(), brain)
	hook := filepath.Join(root, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	exclude := episodeExcludeRequest(brain, *req.Mutation)
	if code, _, body := episodeHTTPCall(t, g, token, exclude); code != 503 {
		t.Fatalf("canonical failure acknowledged %d %s", code, body)
	}
	read, err := j.ReadThrough(context.Background(), contracts.DeletionWatermark{})
	if err != nil || len(read.Entries) != 1 {
		t.Fatalf("canonical failure lacks external safety %v", err)
	}
	if code, _, body := episodeHTTPCall(t, g, token, episodeReadRequest(brain, "context")); code != 503 || !strings.Contains(body, "publication_pending") {
		t.Fatalf("unpublished access %d %s", code, body)
	}
	if err = os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	if code, res, body := episodeHTTPCall(t, g, token, exclude); code != 200 || res.Status != "excluded" {
		t.Fatalf("canonical retry %d %s", code, body)
	}
	if j.calls != 1 {
		t.Fatal("canonical retry duplicated confirmed intent")
	}
}
func TestEpisodeReplayHTTPMissingJournalAndFoundationFenceFailClosed(t *testing.T) {
	g, p, brain, token, j := episodeReplayHTTPSetup(t)
	req := episodeHTTPFixture(t, brain)
	g.Journal = nil
	if code, _, body := episodeHTTPCall(t, g, token, req); code != 503 || !strings.Contains(body, "exclusion_replay_pending") {
		t.Fatalf("missing journal %d %s", code, body)
	}
	g.Journal = j
	r, release, err := p.AcquireCanonical(context.Background(), brain)
	if err != nil {
		t.Fatal(err)
	}
	exclusion := episodeExcludeRequest(brain, *req.Mutation)
	_, err = writer.Episode(context.Background(), r.Queue, r.Sources, r.Root, brain, 0, *exclusion.Mutation, nil)
	release()
	if err != nil {
		t.Fatal(err)
	}
	if code, _, body := episodeHTTPCall(t, g, token, episodeReadRequest(brain, "fences")); code != 503 || !strings.Contains(body, "exclusion_replay_pending") {
		t.Fatalf("foundation fence falsely protected %d %s", code, body)
	}
	if code, res, body := episodeHTTPCall(t, g, token, exclusion); code != 200 || res.Status != "excluded" {
		t.Fatalf("foundation repair %d %s", code, body)
	}
}
func TestEpisodeReplayHTTPExclusionAtQuotaAndAuthorityBoundaries(t *testing.T) {
	g, _, brain, token, j := episodeReplayHTTPSetup(t)
	req := episodeHTTPFixture(t, brain)
	exclusion := episodeExcludeRequest(brain, *req.Mutation)
	binding, err := g.Issuer.Verify(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	entitlement, _, err := g.Meter.Subject(context.Background(), binding.AccountID, binding.PartnerID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = g.Issuer.Store.DB().Exec("INSERT INTO usage_windows(account_id,window_key,metric,committed) VALUES(?,?,?,?)", binding.AccountID, entitlement.Window, "writes", entitlement.Plan.Writes); err != nil {
		t.Fatal(err)
	}
	if code, _, body := episodeHTTPCall(t, g, token, req); code != 429 {
		t.Fatalf("quota %d %s", code, body)
	}
	foreign := exclusion
	foreign.ProjectID = "foreign-brain"
	if code, _, body := episodeHTTPCall(t, g, token, foreign); code != 403 || !strings.Contains(body, "binding_mismatch") {
		t.Fatalf("foreign exclusion %d %s", code, body)
	}
	readOnly, err := g.Issuer.Issue(context.Background(), brain, binding.AccountID, []string{"memory:read"})
	if err != nil {
		t.Fatal(err)
	}
	if code, _, body := episodeHTTPCall(t, g, readOnly, exclusion); code != 403 || !strings.Contains(body, "scope_denied") {
		t.Fatalf("scope %d %s", code, body)
	}
	if j.calls != 0 {
		t.Fatal("unauthorized request reached journal")
	}
	if code, res, body := episodeHTTPCall(t, g, token, exclusion); code != 200 || res.Status != "excluded" || res.PhysicalPurge != "unsupported" {
		t.Fatalf("quota exclusion %d %s", code, body)
	}
	if _, err = g.Issuer.Store.DB().Exec("UPDATE client_credentials SET revoked_at=? WHERE id=?", time.Now().UTC().Format(time.RFC3339Nano), binding.CredentialID); err != nil {
		t.Fatal(err)
	}
	if code, _, body := episodeHTTPCall(t, g, token, exclusion); code != 401 || !strings.Contains(body, "unauthorized") {
		t.Fatalf("revoked %d %s", code, body)
	}
	if j.calls != 1 {
		t.Fatal("revoked retry reached journal")
	}
}

func TestEpisodeReplayCanonicalViewRefusesMaintenanceAndMissingGate(t *testing.T) {
	g, _, _, account, brain, root := newLifecycleExportFixture(t)
	journal, err := deletion.NewFilesystemJournal(t.TempDir(), "view-replay", 1, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	g.Journal = journal
	if _, err = g.ReplayEpisodeDeletions(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	id := writer.MaintenanceIdentity{Schema: "serenity.canonical-maintenance/v1", OperationID: strings.Repeat("a", 64), BrainID: brain, SourceHead: strings.Repeat("b", 40), FenceDigest: strings.Repeat("c", 64)}
	maintenance, err := writer.BeginBrainMaintenance(root, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = g.ReplayEpisodeDeletions(context.Background(), nil); !errors.Is(err, writer.ErrCanonicalViewUnavailable) {
		t.Fatalf("live maintenance replay: %v", err)
	}
	if err = maintenance.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = g.ReplayEpisodeDeletions(context.Background(), nil); !errors.Is(err, writer.ErrMaintenancePending) {
		t.Fatalf("persisted maintenance replay: %v", err)
	}
	maintenance, err = writer.ResumeBrainMaintenance(root, id)
	if err != nil {
		t.Fatal(err)
	}
	if err = maintenance.Finish(id); err != nil {
		t.Fatal(err)
	}
	if err = maintenance.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(root, ".serenity", "canonical-view.lock")); err != nil {
		t.Fatal(err)
	}
	if _, err = g.ReplayEpisodeDeletions(context.Background(), nil); err != nil {
		t.Fatalf("admitted recovery writer bootstrap: %v", err)
	}
	if _, err = os.Stat(filepath.Join(root, ".serenity", "canonical-view.lock")); err != nil {
		t.Fatal("recovery writer did not initialize its view lock", err)
	}
	// Non-ready, absent roots without episode intents require no canonical read.
	absent := "AbsentBrain123456"
	if err = g.replayEpisodeCanonicalView(context.Background(), account, absent, "allocating", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(filepath.Dir(root), absent)); !os.IsNotExist(err) {
		t.Fatal("absent replay created a brain")
	}
}

func TestEpisodeReplayViewBootstrapCannotBypassWriterOrMarker(t *testing.T) {
	for _, pending := range []bool{false, true} {
		t.Run(map[bool]string{false: "writer", true: "marker"}[pending], func(t *testing.T) {
			g, _, _, _, brain, root := newLifecycleExportFixture(t)
			journal, err := deletion.NewFilesystemJournal(t.TempDir(), "replay-bootstrap", 1, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			g.Journal = journal
			owner, err := writer.AcquireBrain(root)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = owner.Close() }()
			if err = os.Remove(filepath.Join(root, ".serenity", "canonical-view.lock")); err != nil {
				t.Fatal(err)
			}
			want := writer.ErrBrainOwned
			if pending {
				if err = owner.Close(); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(root, ".serenity", "canonical-maintenance.json"), []byte("interrupted"), 0600); err != nil {
					t.Fatal(err)
				}
				want = writer.ErrMaintenancePending
			}
			if _, err = g.ReplayEpisodeDeletions(context.Background(), nil); !errors.Is(err, want) {
				t.Fatalf("bootstrap %s: %v", brain, err)
			}
			if _, err = os.Stat(filepath.Join(root, ".serenity", "canonical-view.lock")); !os.IsNotExist(err) {
				t.Fatal("failed bootstrap repaired view lock")
			}
		})
	}
}
