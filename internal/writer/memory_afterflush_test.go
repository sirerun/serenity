package writer

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/store"
)

func TestHostedRememberAfterFlushSeesFactInHeadAfterQueueGuards(t *testing.T) {
	root, _ := gitRepoFixture(t)
	sources := store.NewSourceStore(root)
	q := NewQueue(nil)
	defer q.Close()
	w := MemoryFact{Queue: q, Sources: sources}
	opID := "fedcba9876543210"
	var callbackErr error
	var entered bool
	var callbackID, callbackFact string
	ctx := WithCanonicalOperation(context.Background(), CanonicalOperation{
		ID: opID,
		BeforeCommit: func(_ context.Context, got string) error {
			if got != opID {
				return errors.New("unexpected operation ID")
			}
			all, err := sources.All()
			if err != nil {
				return err
			}
			if len(all) != 0 {
				return errors.New("source write preceded canonical entry")
			}
			entered = true
			return nil
		},
		AfterFlush: func(callbackCtx context.Context, gotOperationID, factID string) {
			callbackID, callbackFact = gotOperationID, factID
			// Reacquiring the exclusive guard proves SubmitAndFlush released
			// both queue guards before invoking this trusted callback.
			checkErr, acquireErr := q.WithCommitFence(callbackCtx, func(context.Context) error {
				rel, err := filepath.Rel(root, sources.DirFor(factID))
				if err != nil {
					return err
				}
				cmd := exec.Command("git", "-C", root, "show", "HEAD:"+filepath.ToSlash(filepath.Join(rel, "bytes")))
				data, err := cmd.CombinedOutput()
				if err != nil {
					return errors.Join(err, errors.New(string(data)))
				}
				payload, err := store.DecodeMemoryFact(data)
				if err != nil {
					return err
				}
				if payload.CanonicalOperationID != opID {
					return errors.New("HEAD fact has a different canonical operation ID")
				}
				return nil
			})
			callbackErr = errors.Join(checkErr, acquireErr)
		},
	})
	result, err := w.RememberContext(ctx, RememberInput{
		OperationKey: opID, Fact: "inline durable fact", Provenance: "test",
		Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld,
	}, time.Now().UTC())
	if err != nil || !entered {
		t.Fatalf("RememberContext entered=%v err=%v", entered, err)
	}
	if result.Record.Payload.CanonicalOperationID != opID || callbackID != opID || callbackFact != result.Record.SHA256 {
		t.Fatalf("durable callback identity: result=%+v callback=(%q,%q)", result.Record.Payload, callbackID, callbackFact)
	}
	if callbackErr != nil {
		t.Fatalf("after-flush callback: %v", callbackErr)
	}
}

func TestHostedRememberAfterFlushDoesNotPromoteDuplicatesOrRefusals(t *testing.T) {
	root, _ := gitRepoFixture(t)
	sources := store.NewSourceStore(root)
	q := NewQueue(nil)
	defer q.Close()
	w := MemoryFact{Queue: q, Sources: sources}
	var beforeCalls, afterCalls int
	newContext := func(refuse bool) context.Context {
		return WithCanonicalOperation(context.Background(), CanonicalOperation{
			ID: "fedcba9876543210",
			BeforeCommit: func(context.Context, string) error {
				beforeCalls++
				if refuse {
					return errors.New("ledger refused")
				}
				return nil
			},
			AfterFlush: func(context.Context, string, string) { afterCalls++ },
		})
	}
	input := RememberInput{OperationKey: "fedcba9876543210", Fact: "same fact", Provenance: "test", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld}
	if _, err := w.RememberContext(newContext(false), input, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := w.RememberContext(newContext(false), input, time.Now().UTC()); err != nil {
		t.Fatalf("exact retry: %v", err)
	}
	if beforeCalls != 1 || afterCalls != 1 {
		t.Fatalf("exact retry synthesized callbacks: before=%d after=%d", beforeCalls, afterCalls)
	}
	input.OperationKey = "different-trusted-key"
	if _, err := w.RememberContext(newContext(false), input, time.Now().UTC()); err == nil {
		t.Fatal("mismatched operation key unexpectedly entered canonical writer")
	}
	if beforeCalls != 1 || afterCalls != 1 {
		t.Fatalf("mismatched key synthesized callbacks: before=%d after=%d", beforeCalls, afterCalls)
	}
	input.OperationKey = "1111111111111111"
	if _, err := w.RememberContext(newContext(true), input, time.Now().UTC()); err == nil {
		t.Fatal("refused canonical entry unexpectedly succeeded")
	}
	if beforeCalls != 2 || afterCalls != 1 {
		t.Fatalf("refusal synthesized durable callback: before=%d after=%d", beforeCalls, afterCalls)
	}
}
