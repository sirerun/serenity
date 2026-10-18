//go:build darwin || linux

package episodeinstall

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/episodeerase"
	"github.com/sirerun/serenity/internal/gitrun"
	"github.com/sirerun/serenity/internal/hosted/pool"
	"github.com/sirerun/serenity/internal/hosted/provision"
	hoststore "github.com/sirerun/serenity/internal/hosted/store"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

type prepareEmbedding struct{}

func (prepareEmbedding) ModelVersion() string { return "episode-install-fixture@v1" }
func (prepareEmbedding) Embed(context.Context, string) ([]float32, error) {
	panic("prepare fixture must never invoke embedding")
}

func prepareFixtureGit(t *testing.T, root string, args ...string) []byte {
	t.Helper()
	raw, err := gitrun.Quarantine(root).Output(context.Background(), args...)
	if err != nil {
		t.Fatalf("synthetic fixture Git %s: %v", args[0], err)
	}
	return raw
}
func prepareTestEpisode(conversation, command, text string) store.EpisodeRecord {
	at := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	event := store.EpisodeEvent{ID: "event", Role: "user", Type: "message", OccurredAt: at, ObservedAt: at, Attribution: "synthetic", Text: text}
	input := store.EpisodeInput{Operation: "append", ProducerID: "producer", ConversationID: conversation, CommandID: command, Host: "fixture", PolicyVersion: "v1", Events: []store.EpisodeEvent{event}, Gaps: []string{}}
	return store.EpisodeRecord{Schema: store.EpisodeSchema, BrainID: "brain", Kind: "append", CommandKey: store.EpisodeCommandKey(input.ProducerID, command), ConversationKey: store.EpisodeConversationKey(input.ProducerID, conversation), RequestDigest: store.EpisodeHash(input), RecordedAt: at, Input: &input, Refs: []store.EpisodeRef{{EventKey: store.EpisodeEventKey(input.ProducerID, conversation, event.ID), Digest: store.EpisodeHash(event)}}}
}

// prepareFixture uses actual hosted provisioning, canonical pool opening/drop,
// the episode writer and source/entity writers. All data and custody are local
// synthetic fixtures; there is no external authority or provider qualification.
func prepareFixture(t *testing.T) Request {
	t.Helper()
	ctx := context.Background()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	control, err := hoststore.Open(filepath.Join(parent, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := control.Close(); err != nil {
			t.Error(err)
		}
	})
	account, err := control.CreateAccount(ctx, "episode-install-fixture@example.test")
	if err != nil {
		t.Fatal(err)
	}
	brains := filepath.Join(parent, "brains")
	if err = os.Mkdir(brains, 0700); err != nil {
		t.Fatal(err)
	}
	provisioner := &provision.Provisioner{Store: control, BrainsRoot: brains}
	brain, err := provisioner.Provision(ctx, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(brains, brain.ID)
	runtimePool, err := pool.New(pool.Config{BrainsRoot: brains, MaxOpen: 1, MaxInFlight: 1, Embedder: prepareEmbedding{}})
	if err != nil {
		t.Fatal(err)
	}
	_, release, err := runtimePool.AcquireCanonical(ctx, brain.ID)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if err = runtimePool.DropContext(ctx, brain.ID); err != nil {
		t.Fatal(err)
	}
	if err = runtimePool.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.db", "index.db-wal", "index.db-shm"} {
		path := filepath.Join(root, ".serenity", name)
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			continue
		}
		if err = os.Chmod(path, 0600); err != nil {
			t.Fatal(err)
		}
	}
	owner, err := writer.AcquireBrain(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close() }()
	q := writer.NewQueue(nil)
	defer q.Close()
	sources := store.NewSourceStore(root)
	var independent domain.Source
	result := q.Submit(writer.Job{Render: func() ([]byte, error) {
		var err error
		independent, err = sources.Write([]byte("independent lexical ember source"), domain.Source{Kind: "file", URI: "fixture://independent", OccurredAt: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)})
		if err == nil {
			q.MarkTouched(filepath.Join(sources.DirFor(independent.SHA256), "bytes"))
			q.MarkTouched(filepath.Join(sources.DirFor(independent.SHA256), "meta.yaml"))
		}
		return nil, err
	}})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	page := store.NewEntityPage(domain.Entity{Type: "project", Slug: "retained"})
	page.Summary = "independent lexical ember summary"
	page.Claims = []domain.Claim{{ID: "independent", SubjectSlug: "retained", Predicate: "has_role", Object: "independent lexical ember", Family: "has_role", SourceRef: "independent", Confidence: 1, State: domain.StateActive}}
	entityPath, _, err := writer.Fence(q, store.NewFenceWriter(root), page)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = writer.FlushContext(ctx, q, root); err != nil {
		t.Fatal(err)
	}
	deleted := prepareTestEpisode("delete", "original-command", "INSTALL-EXCLUDED-SECRET")
	receipt, err := writer.Episode(ctx, q, sources, root, brain.ID, 0, *deleted.Input, nil)
	if err != nil {
		t.Fatal(err)
	}
	sibling := prepareTestEpisode("sibling", "sibling-command", "independent sibling episode")
	if _, err = writer.Episode(ctx, q, sources, root, brain.ID, 0, *sibling.Input, nil); err != nil {
		t.Fatal(err)
	}
	checkpoint := store.EpisodeInput{Operation: "checkpoint", ProducerID: "producer", ConversationID: "derived", CommandID: "checkpoint-command", Host: "fixture", PolicyVersion: "v1", Gaps: []string{}, Checkpoint: &store.EpisodeCheckpoint{Text: "INSTALL-DEPENDENT-SECRET", Sources: receipt.EventRefs, Model: "synthetic", CreatedAt: deleted.RecordedAt}}
	if _, err = writer.Episode(ctx, q, sources, root, brain.ID, 0, checkpoint, nil); err != nil {
		t.Fatal(err)
	}
	exclusion := store.EpisodeInput{Operation: "exclude", ProducerID: "producer", ConversationID: "delete", CommandID: "exclude-command"}
	if _, err = writer.Episode(ctx, q, sources, root, brain.ID, 0, exclusion, nil); err != nil {
		t.Fatal(err)
	}
	projection, err := store.LoadEpisodes(ctx, sources)
	if err != nil {
		t.Fatal(err)
	}
	fences := []store.EpisodeRecord{}
	for _, record := range projection.Records {
		if record.Kind == "exclude" {
			fences = append(fences, record)
		}
	}
	metadata, err := os.ReadFile(filepath.Join(sources.DirFor(independent.SHA256), "meta.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	pins := []episodeerase.PathPin{}
	for _, name := range []string{".gitignore", "serenity.yml", filepath.ToSlash(strings.TrimPrefix(entityPath, root+string(os.PathSeparator)))} {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		pins = append(pins, episodeerase.PathPin{Path: name, SHA256: hashBytes(raw)})
	}
	operationID := hashBytes([]byte("synthetic-episode-install-operation"))
	return Request{Source: root, OperationDirectory: filepath.Join(parent, operationID), AccountID: account.ID, BrainID: brain.ID, OperationID: operationID, SourceHead: strings.TrimSpace(string(prepareFixtureGit(t, root, "rev-parse", "HEAD"))), CurrentFences: fences, IndependentSources: []episodeerase.SourcePin{{SHA256: independent.SHA256, MetadataSHA256: hashBytes(metadata)}}, IndependentPaths: pins}
}

func TestPrepareDurableInactiveCandidatePreservesOriginalAndLexicalData(t *testing.T) {
	req := prepareFixture(t)
	ctx := context.Background()
	before := map[string]inventory{}
	for _, name := range componentNames {
		inv, err := inventoryPath(ctx, filepath.Join(req.Source, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		before[name] = inv
	}
	ref, err := Prepare(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !ref.ResidualCopies || ref.HeaderSHA256 == "" {
		t.Fatalf("missing durable preparation: %+v", ref)
	}
	header, err := loadHeader(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(header.Components) != 8 || header.Candidate.Activation != "not_activated" || header.Candidate.PhysicalPurge != "unsupported" {
		t.Fatal("preparation claimed installation or purge")
	}
	for _, name := range componentNames {
		if verifyInventory(ctx, filepath.Join(req.Source, filepath.FromSlash(name)), before[name]) != nil {
			t.Fatal("preparation changed source", name)
		}
	}
	for _, root := range []string{"replacement", "candidate/repository.git"} {
		repo := filepath.Join(ref.Directory, filepath.FromSlash(root))
		raw := prepareFixtureGit(t, repo, "cat-file", "--batch-all-objects", "--batch-check=%(objectname)")
		for _, object := range strings.Fields(string(raw)) {
			bytes := prepareFixtureGit(t, repo, "cat-file", "-p", object)
			if strings.Contains(string(bytes), "INSTALL-EXCLUDED-SECRET") || strings.Contains(string(bytes), "INSTALL-DEPENDENT-SECRET") {
				t.Fatal("excluded content in new Git object")
			}
		}
	}
	report, err := index.OpenReport(filepath.Join(ref.Directory, "new-index", "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = report.Close() }()
	hits, err := report.SearchFTS(ctx, "ember", 10)
	if err != nil || len(hits) == 0 {
		t.Fatalf("lexical replacement lost independent data: %v", err)
	}
	public, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(public, []byte(req.Source)) || bytes.Contains(public, []byte(req.OperationDirectory)) {
		t.Fatal("prepared reference exported private routes")
	}
}

func TestPrepareRefusesUnsupportedRuntimeAndAttributesOwnedFailure(t *testing.T) {
	t.Run("unknown-runtime", func(t *testing.T) {
		req := prepareFixture(t)
		if err := os.WriteFile(filepath.Join(req.Source, ".serenity", "unknown"), []byte("private"), 0600); err != nil {
			t.Fatal(err)
		}
		ref, err := Prepare(context.Background(), req)
		if !errors.Is(err, ErrUnqualified) || ref.ResidualCopies || ref.HeaderSHA256 != "" {
			t.Fatalf("unknown runtime admitted: %+v %v", ref, err)
		}
	})
	t.Run("cancel-after-copy", func(t *testing.T) {
		req := prepareFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ref, err := prepare(ctx, req, func(stage string) {
			if stage == "original-copied" {
				cancel()
			}
		})
		if !errors.Is(err, ErrUnqualified) || !ref.ResidualCopies || ref.HeaderSHA256 != "" {
			t.Fatalf("copy failure lost custody: %+v %v", ref, err)
		}
		if _, err = os.Stat(filepath.Join(req.OperationDirectory, "original", ".git")); err != nil {
			t.Fatal("owned proof copy disappeared")
		}
	})
}

func TestPrepareNoOldIndexAndQuarantinedPageRefusal(t *testing.T) {
	t.Run("no-old-index", func(t *testing.T) {
		req := prepareFixture(t)
		for _, name := range []string{"index.db", "index.db-wal", "index.db-shm"} {
			if err := os.Remove(filepath.Join(req.Source, ".serenity", name)); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
		}
		ref, err := Prepare(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		header, err := loadHeader(context.Background(), ref)
		if err != nil {
			t.Fatal(err)
		}
		for i := 5; i < 8; i++ {
			if header.Components[i].Old[0].Kind != "absent" {
				t.Fatal("missing old family not explicit")
			}
		}
	})
	t.Run("quarantined-page", func(t *testing.T) {
		req := prepareFixture(t)
		path := filepath.Join(req.Source, "brain", "entities", "project", "bad.md")
		if err := os.WriteFile(path, []byte("not a canonical entity page"), 0600); err != nil {
			t.Fatal(err)
		}
		prepareFixtureGit(t, req.Source, "add", "--", filepath.ToSlash(strings.TrimPrefix(path, req.Source+string(os.PathSeparator))))
		prepareFixtureGit(t, req.Source, "commit", "-m", "synthetic malformed page")
		req.SourceHead = strings.TrimSpace(string(prepareFixtureGit(t, req.Source, "rev-parse", "HEAD")))
		req.IndependentPaths = append(req.IndependentPaths, episodeerase.PathPin{Path: "brain/entities/project/bad.md", SHA256: hashBytes([]byte("not a canonical entity page"))})
		ref, err := Prepare(context.Background(), req)
		if !errors.Is(err, ErrUnqualified) || !ref.ResidualCopies || ref.HeaderSHA256 != "" {
			t.Fatalf("quarantined page admitted: %+v %v", ref, err)
		}
	})
}
