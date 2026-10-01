package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/embed"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/deletion"
	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/service"
	"github.com/sirerun/serenity/internal/hosted/store"
)

const testLifecycleBuildSHA = "0123456789abcdef0123456789abcdef01234567"

type testRecoveryAdmission struct {
	journal contracts.DeletionJournal
	from    contracts.DeletionWatermark
}

func (a testRecoveryAdmission) Admit(_ context.Context, journal contracts.DeletionJournal) (contracts.DeletionWatermark, error) {
	if journal != a.journal {
		return contracts.DeletionWatermark{}, errors.New("test recovery admission received another journal instance")
	}
	return a.from, nil
}

func testLifecycleDependencies(t *testing.T) service.LifecycleDependencies {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	prior, err := deletion.NewFilesystemJournal(root, "test-prior-writer", 1, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	seal, err := prior.Seal(ctx, 1)
	if err != nil {
		t.Fatalf("seal explicit test genesis: %v", err)
	}
	if seal.Generation != 1 || seal.SequenceID < 1 || seal.EntryHash == "" {
		t.Fatalf("test genesis seal is invalid: %+v", seal)
	}
	active, err := deletion.NewFilesystemJournal(root, "test-active-writer", 2, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	// This is the explicitly admitted local-fixture genesis, not a production
	// default or an inferred missing-manifest watermark.
	from := contracts.DeletionWatermark{}
	return service.LifecycleDependencies{
		Journal:  active,
		BuildSHA: testLifecycleBuildSHA,
		Recovery: testRecoveryAdmission{journal: active, from: from},
	}
}

func assembleForTest(t *testing.T, cfg service.Config, dev bool, db *store.Store, sender identity.Sender, embedding embed.Embedder) (*service.Service, error) {
	t.Helper()
	return service.AssembleWithDependencies(context.Background(), cfg, dev, db, sender, embedding, testLifecycleDependencies(t))
}
