package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/deletion"
	"github.com/sirerun/serenity/internal/hosted/gateway"
	"github.com/sirerun/serenity/internal/hosted/provision"
	hoststore "github.com/sirerun/serenity/internal/hosted/store"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

type episodeReplayRequest struct {
	Schema        string              `json:"schema"`
	ProjectID     string              `json:"project_id"`
	Operation     string              `json:"operation"`
	Qualification string              `json:"qualification"`
	Generation    *uint64             `json:"generation,omitempty"`
	Mutation      *store.EpisodeInput `json:"mutation,omitempty"`
	CommandKey    string              `json:"command_key,omitempty"`
}
type episodeReplayResponse struct {
	Status        string                       `json:"status"`
	Generation    uint64                       `json:"generation"`
	Receipt       *store.EpisodeReceipt        `json:"receipt"`
	Events        []store.ArchivedEpisodeEvent `json:"events"`
	PhysicalPurge string                       `json:"physical_purge"`
}

func episodeReplayServiceFixture(t *testing.T) (*hoststore.Store, Config, string, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := hoststore.Open(filepath.Join(dir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	account, err := db.CreateAccount(ctx, "episode-replay@example.test")
	if err != nil {
		t.Fatal(err)
	}
	p := &provision.Provisioner{Store: db, BrainsRoot: filepath.Join(dir, "brains")}
	brain, err := p.Provision(ctx, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{DataDir: dir, MaxOpen: 2, MaxInFlight: 4, PublicOrigin: "http://127.0.0.1", AccountCap: 100, EpisodeArchiveEnabled: true, EpisodeSyntheticBrains: []string{brain.ID}}
	return db, cfg, account.ID, brain.ID
}
func episodeReplayAppend(t *testing.T, brain string) episodeReplayRequest {
	t.Helper()
	raw, err := os.ReadFile("../../../testdata/conformance/episodes/append.json")
	if err != nil {
		t.Fatal(err)
	}
	var req episodeReplayRequest
	if err = json.Unmarshal(raw, &req); err != nil {
		t.Fatal(err)
	}
	req.ProjectID = brain
	return req
}
func episodeReplayCall(t *testing.T, s *Service, token string, req episodeReplayRequest) (int, episodeReplayResponse, string) {
	t.Helper()
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/episodes/v1", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.Handler.ServeHTTP(w, r)
	var res episodeReplayResponse
	if w.Code == 200 {
		if err = json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
	}
	return w.Code, res, w.Body.String()
}
func episodeReplayGit(t *testing.T, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		t.Fatalf("git fixture: %v %s", err, out)
	}
}
func episodeReplayEntry(brain string) contracts.DeletionEntry {
	in := store.EpisodeInput{Operation: "exclude", ProducerID: "p", ConversationID: "c", CommandID: "d"}
	id := contracts.EpisodeDeletionIdentity{BrainHash: contracts.EpisodeDeletionBrainHash(brain), ConversationHash: store.EpisodeConversationKey(in.ProducerID, in.ConversationID), CommandHash: store.EpisodeCommandKey(in.ProducerID, in.CommandID), RequestDigest: store.EpisodeHash(in)}
	return contracts.DeletionEntry{SubjectType: contracts.DeletionSubjectEpisodeConversation, SubjectID: id.String(), Outcome: contracts.DeletionIntentRequested, RecordedAt: time.Now().UTC()}
}

func TestEpisodeReplayStartupOlderSnapshotFullHistoryAndSiblingIsolation(t *testing.T) {
	ctx := context.Background()
	db, cfg, account, brain := episodeReplayServiceFixture(t)
	provisioner := &provision.Provisioner{Store: db, BrainsRoot: filepath.Join(cfg.DataDir, "brains")}
	sibling, err := provisioner.Additional(ctx, account, 10)
	if err != nil {
		t.Fatal(err)
	}
	cfg.EpisodeSyntheticBrains = append(cfg.EpisodeSyntheticBrains, sibling.ID)
	journalRoot := t.TempDir()
	genesis, err := deletion.NewFilesystemJournal(journalRoot, "genesis", 1, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	cut, err := genesis.Seal(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	active, err := deletion.NewFilesystemJournal(journalRoot, "active", 2, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	deps := LifecycleDependencies{Journal: active, BuildSHA: testLifecycleBuildSHA, Recovery: testRecoveryAdmission{journal: active, from: cut}}
	svc, err := AssembleWithDependencies(ctx, cfg, true, db, nil, deletionEmbedding{}, deps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	token, err := svc.Gateway.Issuer.Issue(ctx, brain, account, []string{"memory:read", "memory:write"})
	if err != nil {
		t.Fatal(err)
	}
	siblingToken, err := svc.Gateway.Issuer.Issue(ctx, sibling.ID, account, []string{"memory:read", "memory:write"})
	if err != nil {
		t.Fatal(err)
	}
	original := episodeReplayAppend(t, brain)
	code, appended, body := episodeReplayCall(t, svc, token, original)
	if code != 200 || appended.Receipt == nil {
		t.Fatalf("append %d %s", code, body)
	}
	siblingRequest := episodeReplayAppend(t, sibling.ID)
	if code, _, body = episodeReplayCall(t, svc, siblingToken, siblingRequest); code != 200 {
		t.Fatalf("sibling %d %s", code, body)
	}
	cp := episodeReplayAppend(t, brain)
	cp.Operation = "checkpoint"
	cp.Mutation.Operation = "checkpoint"
	cp.Mutation.CommandID = "checkpoint"
	cp.Mutation.ConversationID = "derived-conversation"
	cp.Mutation.Events = nil
	cp.Mutation.Checkpoint = &store.EpisodeCheckpoint{Text: "dependent original", Model: "fixture", CreatedAt: original.Mutation.Events[0].ObservedAt, Sources: appended.Receipt.EventRefs}
	if code, _, body = episodeReplayCall(t, svc, token, cp); code != 200 {
		t.Fatalf("checkpoint %d %s", code, body)
	}
	// Real canonical snapshot BEFORE exclusion, including the derived checkpoint.
	brainRoot := filepath.Join(cfg.DataDir, "brains", brain)
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	episodeReplayGit(t, "clone", "--quiet", "--no-hardlinks", brainRoot, snapshot)
	episodeReplayGit(t, "-C", snapshot, "config", "user.name", "Replay fixture")
	episodeReplayGit(t, "-C", snapshot, "config", "user.email", "replay@example.test")
	exclude := episodeReplayAppend(t, brain)
	exclude.Operation = "exclude"
	exclude.Mutation = &store.EpisodeInput{Operation: "exclude", ProducerID: original.Mutation.ProducerID, ConversationID: original.Mutation.ConversationID, CommandID: "delete"}
	code, excluded, body := episodeReplayCall(t, svc, token, exclude)
	if code != 200 || excluded.Status != "excluded" {
		t.Fatalf("exclude %d %s", code, body)
	}
	history, err := active.ReadThrough(ctx, contracts.DeletionWatermark{})
	if err != nil || len(history.Entries) != 1 {
		t.Fatalf("journal %v %+v", err, history)
	}
	intent := history.Entries[0]
	if _, err = active.Seal(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if err = svc.Close(); err != nil {
		t.Fatal(err)
	}
	// Replace only this disposable fixture brain with its older canonical copy.
	if err = os.Rename(brainRoot, filepath.Join(t.TempDir(), "post-delete")); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(snapshot, brainRoot); err != nil {
		t.Fatal(err)
	}
	restored, err := hoststore.Open(filepath.Join(cfg.DataDir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restored.Close() })
	adopted, err := deletion.NewFilesystemJournal(journalRoot, "adopted", 3, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	// The admitted cut itself names the intent, so a tail-only replay loses it.
	deps = LifecycleDependencies{Journal: adopted, BuildSHA: testLifecycleBuildSHA, Recovery: testRecoveryAdmission{journal: adopted, from: intent.Watermark}}
	resumed, err := AssembleWithDependencies(ctx, cfg, true, restored, nil, deletionEmbedding{}, deps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resumed.Close() })
	for _, in := range []*store.EpisodeInput{original.Mutation, cp.Mutation} {
		read := episodeReplayRequest{Schema: store.EpisodeSchema, ProjectID: brain, Operation: "read", Qualification: "synthetic", CommandKey: store.EpisodeCommandKey(in.ProducerID, in.CommandID)}
		if code, _, body = episodeReplayCall(t, resumed, token, read); code != 404 || !strings.Contains(body, "record_unavailable") {
			t.Fatalf("restored stale original/derivative %d %s", code, body)
		}
	}
	siblingRead := episodeReplayRequest{Schema: store.EpisodeSchema, ProjectID: sibling.ID, Operation: "read", Qualification: "synthetic", CommandKey: store.EpisodeCommandKey(siblingRequest.Mutation.ProducerID, siblingRequest.Mutation.CommandID)}
	if code, res, body := episodeReplayCall(t, resumed, siblingToken, siblingRead); code != 200 || len(res.Events) != 1 {
		t.Fatalf("sibling changed %d %s", code, body)
	}
	if code, _, body = episodeReplayCall(t, resumed, token, original); code != 409 || !strings.Contains(body, "generation_mismatch") {
		t.Fatalf("stale generation %d %s", code, body)
	}
	*original.Generation = 1
	if code, _, body = episodeReplayCall(t, resumed, token, original); code != 410 || !strings.Contains(body, `"error":"excluded"`) {
		t.Fatalf("stale queue %d %s", code, body)
	}
	*exclude.Generation = 1
	if code, res, body := episodeReplayCall(t, resumed, token, exclude); code != 200 || !reflect.DeepEqual(res.Receipt, excluded.Receipt) || res.PhysicalPurge != "unsupported" {
		t.Fatalf("replayed receipt %d %s", code, body)
	}
}

func TestEpisodeReplayStartupRejectsUnknownMalformedAndUnprovenHistory(t *testing.T) {
	for _, mode := range []string{"unknown-brain", "malformed", "purged", "unsealed", "wrong-watermark", "canceled", "unverified-observation"} {
		t.Run(mode, func(t *testing.T) {
			db, cfg, _, brain := episodeReplayServiceFixture(t)
			ctx := context.Background()
			entry := episodeReplayEntry(brain)
			if mode == "unknown-brain" {
				entry = episodeReplayEntry("unknown-brain")
			}
			var deps LifecycleDependencies
			if mode == "malformed" || mode == "purged" || mode == "unverified-observation" {
				d, p := startupDeps(t)
				deps = d
				entry.Watermark = contracts.DeletionWatermark{Generation: 1, SequenceID: 1, EntryHash: strings.Repeat("a", 64)}
				if mode == "malformed" {
					entry.SubjectID = "v2:plaintext/path"
				}
				if mode == "purged" {
					entry.Outcome = contracts.DeletionOutcomePurged
				}
				if mode == "unverified-observation" {
					p.verifyErr = errors.New("observation is not admitted")
				}
				p.observation.Read.Entries = []contracts.DeletionEntry{entry}
				deps.JournalReader = startupReaderTest{p.observation}
			} else {
				j, err := deletion.NewFilesystemJournal(t.TempDir(), "prior", 1, time.Now)
				if err != nil {
					t.Fatal(err)
				}
				appended, err := j.AppendDeletion(ctx, entry)
				if err != nil {
					t.Fatal(err)
				}
				if mode != "unsealed" {
					if _, err = j.Seal(ctx, 1); err != nil {
						t.Fatal(err)
					}
				}
				from := contracts.DeletionWatermark{}
				if mode == "wrong-watermark" {
					from = appended.Watermark
					from.EntryHash = strings.Repeat("f", 64)
				}
				deps = LifecycleDependencies{Journal: j, BuildSHA: testLifecycleBuildSHA, Recovery: testRecoveryAdmission{journal: j, from: from}}
			}
			if mode == "canceled" {
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			}
			svc, err := AssembleWithDependencies(ctx, cfg, true, db, nil, deletionEmbedding{}, deps)
			if err == nil || svc != nil {
				if svc != nil {
					_ = svc.Close()
				}
				t.Fatalf("unsafe startup admitted: %v", err)
			}
		})
	}
}

func TestEpisodeReplayStartupFoundationFenceRequiresReconciliation(t *testing.T) {
	ctx := context.Background()
	db, cfg, _, brain := episodeReplayServiceFixture(t)
	svc, err := AssembleWithDependencies(ctx, cfg, true, db, nil, deletionEmbedding{}, testLifecycleDependencies(t))
	if err != nil {
		t.Fatal(err)
	}
	runtime, release, err := svc.Pool.AcquireCanonical(ctx, brain)
	if err != nil {
		t.Fatal(err)
	}
	in := store.EpisodeInput{Operation: "exclude", ProducerID: "p", ConversationID: "c", CommandID: "foundation-exclude"}
	_, err = writer.Episode(ctx, runtime.Queue, runtime.Sources, runtime.Root, brain, 0, in, nil)
	release()
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := hoststore.Open(filepath.Join(cfg.DataDir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	resumed, err := AssembleWithDependencies(ctx, cfg, true, reopened, nil, deletionEmbedding{}, testLifecycleDependencies(t))
	if !errors.Is(err, gateway.ErrEpisodeReplayPending) || resumed != nil {
		t.Fatalf("foundation falsely gained restore protection: %v", err)
	}
}

func TestEpisodeReplayOrdinaryStartupUsesAuthorityAdmittedFullObservation(t *testing.T) {
	ctx := context.Background()
	db, cfg, _, brain := episodeReplayServiceFixture(t)
	entry := episodeReplayEntry(brain)
	j, err := deletion.NewFilesystemJournal(t.TempDir(), "ordinary", 1, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = j.AppendDeletion(ctx, entry); err != nil {
		t.Fatal(err)
	}
	read, err := j.ReadThrough(ctx, contracts.DeletionWatermark{})
	if err != nil {
		t.Fatal(err)
	}
	d, p := startupDeps(t)
	d.Journal = j
	p.observation.Read = read
	d.JournalReader = startupReaderTest{p.observation}
	svc, err := AssembleWithDependencies(ctx, cfg, true, db, nil, deletionEmbedding{}, d)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	projection, err := store.LoadEpisodes(ctx, store.NewSourceStore(filepath.Join(cfg.DataDir, "brains", brain)))
	if err != nil || len(projection.Fences) != 1 {
		t.Fatalf("admitted ordinary full history omitted %v", err)
	}
}

func TestEpisodeReplayStartupHonorsWholeBrainDeletionDisposition(t *testing.T) {
	for _, absent := range []bool{false, true} {
		t.Run(map[bool]string{false: "retained-row", true: "absent-row"}[absent], func(t *testing.T) {
			ctx := context.Background()
			db, cfg, account, brain := episodeReplayServiceFixture(t)
			if absent {
				if _, err := db.DB().Exec("DELETE FROM brains WHERE id=?", brain); err != nil {
					t.Fatal(err)
				}
				// A retained orphan directory must still be purged by legacy replay.
			}
			entries := []contracts.DeletionEntry{
				episodeReplayEntry(brain),
				{SubjectType: contracts.DeletionSubjectBrain, SubjectID: brain, Outcome: contracts.DeletionIntentRequested},
				{SubjectType: contracts.DeletionSubjectBrain, SubjectID: brain, Outcome: contracts.DeletionOutcomePurged},
			}
			deps := testLifecycleDependenciesWithEntries(t, entries...)
			svc, err := AssembleWithDependencies(ctx, cfg, true, db, nil, deletionEmbedding{}, deps)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = svc.Close() })
			if _, err = os.Stat(filepath.Join(cfg.DataDir, "brains", brain)); !os.IsNotExist(err) {
				t.Fatalf("whole-brain replay left old directory %v", err)
			}
			var status string
			if err = db.DB().QueryRow("SELECT status FROM accounts WHERE id=?", account).Scan(&status); err != nil || status != "active" {
				t.Fatalf("unrelated account changed %q %v", status, err)
			}
		})
	}
}

// This also runs in the episode_archive_disabled safety build: replay must not
// depend on serving or enabling the archive API.
func TestEpisodeReplaySafetyBuildStartup(t *testing.T) {
	ctx := context.Background()
	db, cfg, _, brain := episodeReplayServiceFixture(t)
	cfg.EpisodeArchiveEnabled = false
	entry := episodeReplayEntry(brain)
	svc, err := AssembleWithDependencies(ctx, cfg, true, db, nil, deletionEmbedding{}, testLifecycleDependenciesWithEntries(t, entry))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := svc.Close(); err != nil {
			t.Error(err)
		}
	})
	runtime, release, err := svc.Pool.AcquireCanonical(ctx, brain)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	p, err := store.LoadEpisodes(ctx, runtime.Sources)
	if err != nil {
		t.Fatal(err)
	}
	id, err := contracts.ParseEpisodeDeletionIdentity(entry.SubjectID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Fences[id.ConversationHash] == 0 {
		t.Fatal("safety startup lost external fence")
	}
	w := httptest.NewRecorder()
	svc.Handler.ServeHTTP(w, httptest.NewRequest("POST", "/episodes/v1", strings.NewReader(`{}`)))
	if w.Code != 404 || !strings.Contains(w.Body.String(), `"error":"archive_disabled"`) {
		t.Fatalf("disabled archive %d %s", w.Code, w.Body.String())
	}
}

// Run this same regression normally and with -tags episode_archive_disabled.
// The inventory is real canonical ordinary data, not synthetic empty filenames.
// It is built once and reused across startup, migration refusal and two replay
// publications so every discovery path encounters more than 65,536 entries.
func TestEpisodeReplayLargeOrdinaryInventoryStartup(t *testing.T) {
	ctx := context.Background()
	db, cfg, _, brain := episodeReplayServiceFixture(t)
	cfg.EpisodeArchiveEnabled = false
	cfg.EpisodeSyntheticBrains = nil // An unenrolled brain still needs discovery.
	root := filepath.Join(cfg.DataDir, "brains", brain)
	sources := store.NewSourceStore(root)
	for i := 0; i < 22000; i++ {
		data := []byte(fmt.Sprintf("ordinary retained source %05d", i))
		if _, err := sources.Write(data, domain.Source{Kind: "file", URI: "fixture:ordinary", OccurredAt: time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)}); err != nil {
			t.Fatal(err)
		}
	}
	episodeReplayGit(t, "-C", root, "add", "--", "brain/sources")
	episodeReplayGit(t, "-C", root, "commit", "-m", "ordinary inventory fixture")
	count := 0
	if err := filepath.WalkDir(filepath.Join(root, "brain", "sources"), func(_ string, _ fs.DirEntry, err error) error { count++; return err }); err != nil {
		t.Fatal(err)
	}
	if count <= 65536 {
		t.Fatalf("fixture has only %d entries", count)
	}
	// The interactive budget is unchanged even though maintenance must complete.
	if _, err := store.LoadEpisodes(ctx, sources); !errors.Is(err, store.ErrEpisodeLimit) {
		t.Fatalf("interactive budget changed: %v", err)
	}
	svc, err := AssembleWithDependencies(ctx, cfg, true, db, nil, deletionEmbedding{}, testLifecycleDependencies(t))
	if err != nil {
		t.Fatalf("archive-off ordinary startup: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if svc.Handler == nil {
		t.Fatal("startup did not install handlers")
	}
	disabled := httptest.NewRecorder()
	svc.Handler.ServeHTTP(disabled, httptest.NewRequest("POST", "/episodes/v1", strings.NewReader(`{}`)))
	if disabled.Code != 404 || !strings.Contains(disabled.Body.String(), "archive_disabled") {
		t.Fatalf("archive enabled: %d %s", disabled.Code, disabled.Body.String())
	}
	if err = svc.Close(); err != nil {
		t.Fatal(err)
	}
	reopen := func() *hoststore.Store {
		t.Helper()
		reopened, err := hoststore.Open(filepath.Join(cfg.DataDir, "control.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = reopened.Close() })
		return reopened
	}
	// A foundation-only fence must be found despite the ordinary inventory and
	// the archive being disabled. It may not be hidden by ignoring scan limits.
	entry := episodeReplayEntry(brain)
	id, err := contracts.ParseEpisodeDeletionIdentity(entry.SubjectID)
	if err != nil {
		t.Fatal(err)
	}
	fence := store.EpisodeRecord{Schema: store.EpisodeSchema, BrainID: brain, Kind: "exclude", CommandKey: id.CommandHash, ConversationKey: id.ConversationHash, RequestDigest: id.RequestDigest, Generation: 1, RecordedAt: entry.RecordedAt}
	if _, err = sources.PutEpisode(fence); err != nil {
		t.Fatal(err)
	}
	episodeReplayGit(t, "-C", root, "add", "--", "brain/sources")
	episodeReplayGit(t, "-C", root, "commit", "-m", "foundation fence fixture")
	foundationDB := reopen()
	blocked, err := AssembleWithDependencies(ctx, cfg, true, foundationDB, nil, deletionEmbedding{}, testLifecycleDependencies(t))
	if blocked != nil {
		_ = blocked.Close()
		t.Fatal("unjournaled foundation fence admitted")
	}
	if !errors.Is(err, gateway.ErrEpisodeReplayPending) {
		t.Fatalf("foundation discovery: %v", err)
	}
	if err = foundationDB.Close(); err != nil {
		t.Fatal(err)
	}
	// The first admitted fence already exists. The second must be installed.
	// Both the writer's pre/post loads and the gateway's final verification must
	// use complete discovery, not just the initial startup scan.
	second := entry
	secondID := id
	secondID.CommandHash = store.EpisodeCommandKey("p", "second-delete")
	secondID.ConversationHash = store.EpisodeConversationKey("p", "second-conversation")
	secondID.RequestDigest = store.EpisodeHash(store.EpisodeInput{Operation: "exclude", ProducerID: "p", CommandID: "second-delete", ConversationID: "second-conversation"})
	second.SubjectID = secondID.String()
	for attempt := 0; attempt < 2; attempt++ {
		replayDB := reopen()
		resumed, err := AssembleWithDependencies(ctx, cfg, true, replayDB, nil, deletionEmbedding{}, testLifecycleDependenciesWithEntries(t, entry, second))
		if err != nil {
			t.Fatalf("large-brain replay %d: %v", attempt, err)
		}
		t.Cleanup(func() { _ = resumed.Close() })
		projection, err := store.LoadEpisodesForReplay(ctx, sources)
		if err != nil {
			_ = resumed.Close()
			t.Fatal(err)
		}
		if len(projection.Fences) != 2 || projection.Fences[id.ConversationHash] != 1 || projection.Fences[secondID.ConversationHash] != 2 {
			_ = resumed.Close()
			t.Fatalf("fences missing after replay: %+v", projection.Fences)
		}
		if _, err = store.LoadEpisodes(ctx, sources); !errors.Is(err, store.ErrEpisodeLimit) {
			_ = resumed.Close()
			t.Fatalf("replay relaxed interactive limit: %v", err)
		}
		if err = resumed.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
