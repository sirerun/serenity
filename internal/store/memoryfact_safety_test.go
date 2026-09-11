package store_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

func TestMemorySourcePublicationRejectsPartialFinalAndIgnoresStaging(t *testing.T) {
	root := t.TempDir()
	s := store.NewSourceStore(root)
	p := independentMemoryPayload("durable fact", 10)
	sha := independentPublishMemoryFixture(t, root, p)
	if err := os.Remove(filepath.Join(s.DirFor(sha), "meta.yaml")); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(s.DirFor(sha), "bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteMemoryFact(p); err == nil {
		t.Fatal("incomplete final record was silently repaired/overwritten")
	}
	after, err := os.ReadFile(filepath.Join(s.DirFor(sha), "bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("corrupt final record was changed")
	}
	if _, err := s.All(); err == nil {
		t.Fatal("partial published source was omitted without an error")
	}
	if err := os.RemoveAll(s.DirFor(sha)); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(root, "brain", "sources", sha[:2], ".source-stage-interrupted")
	if err := os.Mkdir(stage, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "bytes"), before, 0644); err != nil {
		t.Fatal(err)
	}
	records, err := s.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatal("unpublished staging file entered source enumeration")
	}
	got, err := s.WriteMemoryFact(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.SHA256 != sha {
		t.Fatal("publication changed content identity")
	}
	if _, _, err := s.Read(sha); err != nil {
		t.Fatal(err)
	}
}

func TestMemorySourceReservedKindsAndMetadataAreValidated(t *testing.T) {
	root := t.TempDir()
	s := store.NewSourceStore(root)
	p := independentMemoryPayload("attributed text", 11)
	data, err := store.EncodeMemoryFact(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{store.SourceKindMemoryFact, store.SourceKindMemoryExpiry, "memory_unknown"} {
		if _, err := s.Write(data, domain.Source{Kind: kind}); err == nil {
			t.Fatalf("generic import accepted reserved kind %s", kind)
		}
	}
	ordinary, err := s.Write(data, domain.Source{Kind: "file"})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := store.LoadMemoryProjection(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.All()) != 0 {
		t.Fatal("ordinary imported JSON activated memory semantics")
	}
	if _, err := s.WriteMemoryFact(p); err == nil {
		t.Fatal("typed write silently relabeled immutable generic source")
	}
	// Corrupted reserved metadata must fail closed rather than become generic.
	meta := []byte("kind: memory_unknown\nuri: memory://fact\n")
	if err := os.WriteFile(filepath.Join(s.DirFor(ordinary.SHA256), "meta.yaml"), meta, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadMemoryProjection(s); err == nil {
		t.Fatal("unknown reserved kind accepted")
	}
}

func TestMemoryCodecRejectsCorruptCanonicalFields(t *testing.T) {
	base := independentMemoryPayload("valid", 12)
	cases := map[string]func(*store.MemoryFactPayload){
		"zero id":          func(p *store.MemoryFactPayload) { p.LegacyID = 0 },
		"negative id":      func(p *store.MemoryFactPayload) { p.LegacyID = -1 },
		"unsafe id":        func(p *store.MemoryFactPayload) { p.LegacyID = 1 << 53 },
		"blank provenance": func(p *store.MemoryFactPayload) { p.Provenance = "  " },
		"long provenance":  func(p *store.MemoryFactPayload) { p.Provenance = strings.Repeat("x", 501) },
		"blank fact":       func(p *store.MemoryFactPayload) { p.Fact = " " },
		"missing time":     func(p *store.MemoryFactPayload) { p.CreatedAt = time.Time{} },
		"version":          func(p *store.MemoryFactPayload) { p.FormatVersion = 2 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := base
			mutate(&p)
			if _, err := store.EncodeMemoryFact(p); err == nil {
				t.Fatal("encoder accepted corrupt canonical fact")
			}
			raw, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.DecodeMemoryFact(raw); err == nil {
				t.Fatal("decoder accepted corrupt canonical fact")
			}
		})
	}
	for _, target := range []string{"", "short", strings.Repeat("Z", 64)} {
		p := store.MemoryExpiryPayload{FormatVersion: 1, RecordType: store.SourceKindMemoryExpiry, TargetSHA256: target, ExpiredAt: base.CreatedAt}
		if _, err := store.EncodeMemoryExpiry(p); err == nil {
			t.Fatal("invalid expiry target accepted")
		}
	}
}

func TestMemoryProjectionExcludesLifecycleAndRetainsIndexOnlyPolicy(t *testing.T) {
	root := t.TempDir()
	s := store.NewSourceStore(root)
	p := independentMemoryPayload("public", 13)
	fact, err := s.WriteMemoryFact(p)
	if err != nil {
		t.Fatal(err)
	}
	expiry, err := s.WriteMemoryExpiry(store.MemoryExpiryPayload{FormatVersion: 1, RecordType: store.SourceKindMemoryExpiry, TargetSHA256: fact.SHA256, ExpiredAt: p.CreatedAt})
	if err != nil {
		t.Fatal(err)
	}
	local, err := s.Write([]byte("local-only source"), domain.Source{Kind: "file", IndexOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := store.LoadMemoryProjection(s)
	if err != nil {
		t.Fatal(err)
	}
	if !projection.SourceIndexOnly(local.SHA256) {
		t.Fatal("lost source egress policy")
	}
	if store.MemoryEligible(projection, expiry.SHA256, false, p.CreatedAt) || store.MemoryEligible(projection, fact.SHA256, true, p.CreatedAt) {
		t.Fatal("expired fact or lifecycle event allowed as evidence")
	}
}

func TestMemorySourceConcurrentPublicationIsWhole(t *testing.T) {
	root := t.TempDir()
	s := store.NewSourceStore(root)
	started, done := make(chan struct{}), make(chan error, 1)
	go func() {
		<-started
		for id := int64(1); id <= 20; id++ {
			if _, err := s.WriteMemoryFact(independentMemoryPayload("concurrently observed fact", id)); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	close(started)
	for {
		if _, err := s.All(); err != nil {
			t.Fatalf("reader saw incomplete published record: %v", err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
			records, err := s.All()
			if err != nil {
				t.Fatal(err)
			}
			if len(records) != 20 {
				t.Fatalf("published %d records, want 20", len(records))
			}
			return
		default:
		}
	}
}

func TestSourceMetadataOnlyIndexOnlyCloneRemainsReadable(t *testing.T) {
	root := t.TempDir()
	s := store.NewSourceStore(root)
	data := []byte("excluded original")
	src, err := s.Write(data, domain.Source{Kind: "file", IndexOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(s.DirFor(src.SHA256), "bytes")); err != nil {
		t.Fatal(err)
	}
	records, err := s.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || !s.Exists(src.SHA256) {
		t.Fatal("metadata-only ordinary import was lost")
	}
	again, err := s.Write(data, domain.Source{Kind: "file", IndexOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if again.SHA256 != src.SHA256 {
		t.Fatal("immutable source identity changed")
	}
}

func TestMemoryProjectionRejectsDuplicateOperationKeys(t *testing.T) {
	root := t.TempDir()
	first := independentMemoryPayload("first operation", 101)
	first.OperationKey = "merged-key"
	independentPublishMemoryFixture(t, root, first)
	second := independentMemoryPayload("other branch", 102)
	second.OperationKey = first.OperationKey
	independentPublishMemoryFixture(t, root, second)
	if _, err := store.LoadMemoryProjection(store.NewSourceStore(root)); err == nil {
		t.Fatal("conflicting merged operation keys were accepted")
	}
}

func TestMemoryOperationKeyEncodingValidation(t *testing.T) {
	for _, key := range []string{"contains space", "../slash", "雪", strings.Repeat("a", 129)} {
		p := independentMemoryPayload("fact", 103)
		p.OperationKey = key
		if _, err := store.EncodeMemoryFact(p); err == nil {
			t.Fatalf("invalid key accepted: %q", key)
		}
	}
	p := independentMemoryPayload("fact", 104)
	p.OperationKey = "ajent:post-1.rev_2"
	data, err := store.EncodeMemoryFact(p)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := store.DecodeMemoryFact(data)
	if err != nil || decoded.OperationKey != p.OperationKey {
		t.Fatal("operation identity lost")
	}
}
