package episodeerase

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/gitrun"
)

func rawSource(t *testing.T, f *fixture, kind, text string) SourcePin {
	t.Helper()
	src, err := f.s.Write([]byte(text), domain.Source{Kind: kind, URI: "fixture://independent", OccurredAt: testTime})
	if err != nil {
		t.Fatal(err)
	}
	meta, err := os.ReadFile(filepath.Join(f.s.DirFor(src.SHA256), "meta.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return SourcePin{src.SHA256, rawDigest(meta)}
}

func TestMixedPreservesIndependentHistoryAndErasesClosure(t *testing.T) {
	f := newFixture(t)
	firstPin := rawSource(t, f, "file", "independent first revision")
	deleted := appendRecord("delete", "delete-command", "MIXED-UNIQUE-SECRET", false)
	removed := f.put(deleted)
	f.commit()
	secondPin := rawSource(t, f, "email", "independent second revision")
	checkpoint := f.put(checkpointRecord(deleted))
	sibling := f.put(appendRecord("sibling", "sibling-command", "surviving episode", false))
	f.commit()
	req := f.request()
	if _, err := Build(context.Background(), req); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("legacy mixed refusal: %v", err)
	}
	r, err := BuildMixed(context.Background(), MixedRequest{req, []SourcePin{secondPin, firstPin}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Schema != "serenity.mixed-episode-erasure-candidate/v1" || r.Activation != "not_activated" || r.PhysicalPurge != "unsupported" || len(r.RetainedSources) != 2 {
		t.Fatalf("bad manifest: %+v", r)
	}
	if !contains(r.RemovedSources, removed) || !contains(r.RemovedSources, checkpoint) {
		t.Fatal("missing removal closure")
	}
	for _, revision := range []string{"refs/heads/main~1", "refs/heads/main"} {
		got := candidateFiles(t, req, revision)
		for _, pin := range []SourcePin{firstPin, secondPin} {
			originalRevision := "HEAD"
			if strings.HasSuffix(revision, "~1") {
				originalRevision = "HEAD~1"
				if pin == secondPin {
					if _, ok := got[sourceBase(pin.SHA256)+"bytes"]; ok {
						t.Fatal("introduced future source into old history")
					}
					continue
				}
			}
			for _, name := range []string{"bytes", "meta.yaml"} {
				original := f.git("show", originalRevision+":"+sourceBase(pin.SHA256)+name)
				if !bytes.Equal(got[sourceBase(pin.SHA256)+name], original) {
					t.Fatal("raw source identity changed")
				}
			}
		}
		p := projection(t, got)
		if p.Generation != 1 || p.Fences[deleted.ConversationKey] != 1 {
			t.Fatal("fence missing")
		}
		if _, ok := got[sourceBase(removed)+"bytes"]; ok {
			t.Fatal("excluded episode retained")
		}
	}
	if _, ok := candidateFiles(t, req, "refs/heads/main")[sourceBase(sibling)+"bytes"]; !ok {
		t.Fatal("sibling missing")
	}
	runner := gitrun.CanonicalReadOnly(filepath.Join(req.Destination, "repository.git"))
	b := &budget{}
	objects, err := allObjects(context.Background(), b, runner)
	if err != nil {
		t.Fatal(err)
	}
	for id, kind := range objects {
		raw, err := b.git(context.Background(), runner, nil, "cat-file", kind, id)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"MIXED-UNIQUE-SECRET", "DERIVED-SECRET", "MAY-CONTAIN-DELETED-TEXT"} {
			if bytes.Contains(raw, []byte(secret)) {
				t.Fatalf("secret retained in %s", id)
			}
		}
	}
	// Pin ordering is irrelevant to deterministic retry and candidate identities.
	req.Destination = filepath.Join(f.parent, "retry")
	retry, err := BuildMixed(context.Background(), MixedRequest{req, []SourcePin{firstPin, secondPin}})
	if err != nil {
		t.Fatal(err)
	}
	if retry.RetryID != r.RetryID || retry.CandidateHead != r.CandidateHead {
		t.Fatal("pin ordering changed identity")
	}
	if _, err := BuildMixed(context.Background(), MixedRequest{req, []SourcePin{firstPin, secondPin}}); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("existing destination: %v", err)
	}
}

func TestMixedRejectsUnqualifiedPinsAndMetadata(t *testing.T) {
	for _, test := range []string{"empty", "duplicate", "bad_sha", "bad_metadata_sha", "mismatch", "unused", "unlisted", "unknown_kind", "dependency", "unknown_field", "index_only", "mutated_bytes", "historical_metadata", "foreign_brain", "episode_pin", "too_many"} {
		t.Run(test, func(t *testing.T) {
			f := newFixture(t)
			pin := rawSource(t, f, "voice", "independent source")
			episode := appendRecord("delete", "delete-command", "excluded", false)
			if test == "foreign_brain" {
				episode.BrainID = "foreign"
			}
			episodeSHA := f.put(episode)
			pins := []SourcePin{pin}
			metaPath := filepath.Join(f.s.DirFor(pin.SHA256), "meta.yaml")
			mutateMeta := func(transform func(string) string) {
				data, err := os.ReadFile(metaPath)
				if err != nil {
					t.Fatal(err)
				}
				data = []byte(transform(string(data)))
				if err := os.WriteFile(metaPath, data, 0600); err != nil {
					t.Fatal(err)
				}
				pins[0].MetadataSHA256 = rawDigest(data)
			}
			switch test {
			case "empty":
				pins = nil
			case "duplicate":
				pins = append(pins, pin)
			case "bad_sha":
				pins[0].SHA256 = "BAD"
			case "bad_metadata_sha":
				pins[0].MetadataSHA256 = "BAD"
			case "mismatch":
				pins[0].MetadataSHA256 = strings.Repeat("0", 64)
			case "unused":
				pins = append(pins, SourcePin{strings.Repeat("1", 64), strings.Repeat("2", 64)})
			case "unlisted":
				rawSource(t, f, "file", "unlisted raw evidence")
			case "unknown_kind":
				mutateMeta(func(s string) string { return strings.Replace(s, "kind: voice", "kind: generated_summary", 1) })
			case "dependency":
				mutateMeta(func(s string) string { return s + "meta:\n    source: erased-event\n" })
			case "unknown_field":
				mutateMeta(func(s string) string { return s + "depends_on: erased-event\n" })
			case "index_only":
				mutateMeta(func(s string) string { return s + "index_only: true\n" })
			case "mutated_bytes":
				if err := os.WriteFile(filepath.Join(f.s.DirFor(pin.SHA256), "bytes"), []byte("drift"), 0600); err != nil {
					t.Fatal(err)
				}
			case "historical_metadata":
				f.commit()
				mutateMeta(func(s string) string { return strings.Replace(s, "fixture://independent", "fixture://changed", 1) })
			case "episode_pin":
				data, err := os.ReadFile(filepath.Join(f.s.DirFor(episodeSHA), "meta.yaml"))
				if err != nil {
					t.Fatal(err)
				}
				pins = append(pins, SourcePin{episodeSHA, rawDigest(data)})
			case "too_many":
				pins = make([]SourcePin, 2049)
			}
			f.commit()
			req := f.request()
			if _, err := BuildMixed(context.Background(), MixedRequest{req, pins}); err == nil {
				t.Fatal("unsafe candidate accepted")
			}
			if _, err := os.Stat(req.Destination); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("failure published candidate")
			}
		})
	}
}
