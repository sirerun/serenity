package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/embed"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/deletion"
	"github.com/sirerun/serenity/internal/hosted/identity"
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

func testLifecycleDependencies(t *testing.T) LifecycleDependencies {
	return testLifecycleDependenciesWithEntries(t)
}

func testLifecycleDependenciesWithEntries(t *testing.T, entries ...contracts.DeletionEntry) LifecycleDependencies {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	prior, err := deletion.NewFilesystemJournal(root, "test-prior-writer", 1, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if _, err := prior.AppendDeletion(ctx, entry); err != nil {
			t.Fatalf("append explicit test recovery entry: %v", err)
		}
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
	return LifecycleDependencies{
		Journal:  active,
		BuildSHA: testLifecycleBuildSHA,
		Recovery: testRecoveryAdmission{journal: active, from: from},
	}
}

func assembleForTest(t *testing.T, cfg Config, dev bool, db *store.Store, sender identity.Sender, embedding embed.Embedder) (*Service, error) {
	t.Helper()
	svc, err := AssembleWithDependencies(context.Background(), cfg, dev, db, sender, embedding, testLifecycleDependencies(t))
	if svc != nil {
		t.Cleanup(func() { _ = svc.Close() })
	}
	return svc, err
}

func TestLegacyConstructorsFailClosedBeforeSideEffects(t *testing.T) {
	if _, err := New(Config{}, true, nil); !errors.Is(err, ErrStartupUnavailable) {
		t.Fatalf("legacy New error=%v, want ErrStartupUnavailable", err)
	}
	if _, err := Assemble(Config{}, true, nil, nil, nil); !errors.Is(err, ErrStartupUnavailable) {
		t.Fatalf("legacy Assemble error=%v, want ErrStartupUnavailable", err)
	}
}

func TestAssembleRequiresAllLifecycleDependenciesBeforeCreatingResources(t *testing.T) {
	base := testLifecycleDependencies(t)
	typedNilJournal := (*deletion.Journal)(nil)
	cases := []struct {
		name string
		edit func(*LifecycleDependencies)
	}{
		{name: "missing journal", edit: func(d *LifecycleDependencies) { d.Journal = nil }},
		{name: "typed nil journal", edit: func(d *LifecycleDependencies) { d.Journal = typedNilJournal }},
		{name: "missing build identity", edit: func(d *LifecycleDependencies) { d.BuildSHA = "" }},
		{name: "missing recovery admission", edit: func(d *LifecycleDependencies) { d.Recovery = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := base
			tc.edit(&deps)
			_, err := AssembleWithDependencies(context.Background(), Config{}, true, nil, nil, nil, deps)
			if !errors.Is(err, ErrStartupUnavailable) {
				t.Fatalf("AssembleWithDependencies error=%v, want ErrStartupUnavailable", err)
			}
		})
	}
}
