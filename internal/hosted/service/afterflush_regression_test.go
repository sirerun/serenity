package service_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/provision"
	"github.com/sirerun/serenity/internal/hosted/service"
	hoststore "github.com/sirerun/serenity/internal/hosted/store"
	brainstore "github.com/sirerun/serenity/internal/store"
)

type headCheckingFailingEmbedder struct {
	root         string
	calls        int
	observedHead bool
	checkErr     error
}

func (*headCheckingFailingEmbedder) ModelVersion() string { return "test@v1" }

func (e *headCheckingFailingEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	e.calls++
	sources := brainstore.NewSourceStore(e.root)
	projection, err := brainstore.LoadMemoryProjection(sources)
	if err != nil {
		e.checkErr = err
		return nil, err
	}
	for _, record := range projection.All() {
		if record.Payload.Fact != text {
			continue
		}
		rel, err := filepath.Rel(e.root, sources.DirFor(record.SHA256))
		if err != nil {
			e.checkErr = err
			return nil, err
		}
		cmd := exec.Command("git", "-C", e.root, "show", "HEAD:"+filepath.ToSlash(filepath.Join(rel, "bytes")))
		data, err := cmd.CombinedOutput()
		if err != nil {
			e.checkErr = errors.Join(err, errors.New(string(data)))
			return nil, e.checkErr
		}
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != record.SHA256 {
			e.checkErr = errors.New("Git HEAD source does not match the fact identity")
			return nil, e.checkErr
		}
		payload, err := brainstore.DecodeMemoryFact(data)
		if err != nil {
			e.checkErr = err
			return nil, err
		}
		if payload.Fact != text || payload.CanonicalOperationID == "" {
			e.checkErr = errors.New("provider started before the matching canonical fact was committed")
			return nil, e.checkErr
		}
		e.observedHead = true
		break
	}
	if !e.observedHead {
		e.checkErr = errors.New("provider started without matching fact in Git HEAD")
		return nil, e.checkErr
	}
	return nil, errors.New("injected embedding failure after durable source write")
}

func TestHostedRememberCommitsBeforeProviderFailureAndReplays(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := hoststore.Open(filepath.Join(dir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	embedding := &headCheckingFailingEmbedder{}
	svc, err := service.Assemble(service.Config{DataDir: dir, PublicOrigin: "http://127.0.0.1", MaxOpen: 2, MaxInFlight: 4, AccountCap: 100}, true, db, &sender{}, embedding)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := svc.Close(); err != nil {
			t.Error(err)
		}
	})
	account, err := db.CreateAccount(ctx, "afterflush-regression@example.test")
	if err != nil {
		t.Fatal(err)
	}
	brain, err := (&provision.Provisioner{Store: db, BrainsRoot: filepath.Join(dir, "brains")}).Provision(ctx, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	embedding.root = filepath.Join(dir, "brains", brain.ID)
	args := json.RawMessage(`{"fact":"Durable before provider","provenance":"after-flush regression","operation_key":"provider-failure"}`)
	first, err := svc.Gateway.CallForAccount(ctx, account.ID, brain.ID, "remember", args)
	if err != nil || first.IsError {
		t.Fatalf("remember with degraded index: result=%+v err=%v", first, err)
	}
	if !embedding.observedHead || embedding.checkErr != nil || embedding.calls != 1 {
		t.Fatalf("provider observation: inHead=%v calls=%d err=%v", embedding.observedHead, embedding.calls, embedding.checkErr)
	}
	var firstBody struct {
		ID string `json:"id"`
	}
	if len(first.Content) == 0 || json.Unmarshal([]byte(first.Content[0].Text), &firstBody) != nil || firstBody.ID == "" {
		t.Fatalf("remember response lacks fact ID: %+v", first)
	}
	var phase string
	if err := db.DB().QueryRowContext(ctx, `SELECT phase FROM operations WHERE account_id=? AND client_key='provider-failure'`, account.ID).Scan(&phase); err != nil {
		t.Fatal(err)
	}
	if phase != "committed" {
		t.Fatalf("operation phase after provider failure=%q, want committed", phase)
	}
	second, err := svc.Gateway.CallForAccount(ctx, account.ID, brain.ID, "remember", args)
	if err != nil || second.IsError {
		t.Fatalf("committed retry: result=%+v err=%v", second, err)
	}
	var secondBody struct {
		ID string `json:"id"`
	}
	if len(second.Content) == 0 || json.Unmarshal([]byte(second.Content[0].Text), &secondBody) != nil || secondBody.ID != firstBody.ID {
		t.Fatalf("retry identity changed: first=%q retry=%+v", firstBody.ID, second)
	}
	if embedding.calls != 1 {
		t.Fatalf("committed retry re-entered provider; calls=%d", embedding.calls)
	}
	projection, err := brainstore.LoadMemoryProjection(brainstore.NewSourceStore(embedding.root))
	if err != nil || len(projection.All()) != 1 {
		t.Fatalf("canonical source count=%d err=%v, want one", len(projection.All()), err)
	}
	var writes int
	if err := db.DB().QueryRowContext(ctx, `SELECT COALESCE(sum(committed),0) FROM usage_windows WHERE account_id=? AND metric='writes'`, account.ID).Scan(&writes); err != nil || writes != 1 {
		t.Fatalf("committed writes=%d err=%v, want one", writes, err)
	}
}
