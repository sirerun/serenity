package contracts_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

const (
	hexA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hexB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	hexC = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	oid1 = "1111111111111111111111111111111111111111"
	oid2 = "2222222222222222222222222222222222222222"
)

// validManifest returns a fresh, fully populated manifest with two non-empty
// brains and one empty brain. Every call returns independent slices, so a case
// can mutate it freely.
func validManifest() contracts.ManifestV2 {
	return contracts.ManifestV2{
		Version:   2,
		Source:    contracts.SourceRef{BuildSHA: "0123456789abcdef0123456789abcdef01234567", SchemaVersion: 3},
		CreatedAt: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC),
		ControlDB: contracts.ArtifactRef{RelativePath: "control.db", LengthBytes: 4096, SHA256: hexA},
		Brains: []contracts.BrainArtifact{
			{
				ID:          "brain-a",
				ArtifactRef: contracts.ArtifactRef{RelativePath: "brain-a.bundle", LengthBytes: 100, SHA256: hexB},
				Heads: []contracts.BundleHead{
					{Ref: "HEAD", ObjectID: oid1},
					{Ref: "refs/heads/main", ObjectID: oid1},
				},
			},
			{ID: "brain-b", Empty: true},
			{
				ID:          "brain-c",
				ArtifactRef: contracts.ArtifactRef{RelativePath: "brain-c.bundle", LengthBytes: 7, SHA256: hexC},
				Heads:       []contracts.BundleHead{{Ref: "refs/heads/main", ObjectID: oid2}},
			},
		},
		JournalWatermark: contracts.DeletionWatermark{Generation: 1, SequenceID: 4, EntryHash: hexA},
	}
}

func TestManifestV2ValidateAcceptsACompleteManifest(t *testing.T) {
	if err := validManifest().Validate(); err != nil {
		t.Fatalf("a complete manifest must validate: %v", err)
	}
	empty := validManifest()
	empty.Brains = nil
	empty.JournalWatermark = contracts.DeletionWatermark{}
	if err := empty.Validate(); err != nil {
		t.Fatalf("a manifest with no brains and an empty journal must validate: %v", err)
	}
	sha256Heads := validManifest()
	sha256Heads.Brains[0].Heads[0].ObjectID = strings.Repeat("d", 64)
	if err := sha256Heads.Validate(); err != nil {
		t.Fatalf("a 64-character object id must validate: %v", err)
	}
}

// TestManifestV2ValidateRejectsEveryDefect is the executable statement of what
// makes a manifest unusable. Each case starts from the valid manifest, applies
// one defect, and must fail with the named error class.
func TestManifestV2ValidateRejectsEveryDefect(t *testing.T) {
	invalid := contracts.ErrManifestInvalid
	cases := []struct {
		name   string
		mutate func(*contracts.ManifestV2)
		want   error
	}{
		{"version 1", func(m *contracts.ManifestV2) { m.Version = 1 }, contracts.ErrManifestVersion},
		{"version 3", func(m *contracts.ManifestV2) { m.Version = 3 }, contracts.ErrManifestVersion},
		{"version unset", func(m *contracts.ManifestV2) { m.Version = 0 }, contracts.ErrManifestVersion},
		{"build unset", func(m *contracts.ManifestV2) { m.Source.BuildSHA = "" }, invalid},
		{"build with a space", func(m *contracts.ManifestV2) { m.Source.BuildSHA = "abc def" }, invalid},
		{"schema zero", func(m *contracts.ManifestV2) { m.Source.SchemaVersion = 0 }, invalid},
		{"created unset", func(m *contracts.ManifestV2) { m.CreatedAt = time.Time{} }, invalid},
		{"created not UTC", func(m *contracts.ManifestV2) {
			m.CreatedAt = m.CreatedAt.In(time.FixedZone("EAT", 3*3600))
		}, invalid},

		{"control db missing", func(m *contracts.ManifestV2) { m.ControlDB = contracts.ArtifactRef{} }, invalid},
		{"control db path unset", func(m *contracts.ManifestV2) { m.ControlDB.RelativePath = "" }, invalid},
		{"control db length zero", func(m *contracts.ManifestV2) { m.ControlDB.LengthBytes = 0 }, invalid},
		{"control db length negative", func(m *contracts.ManifestV2) { m.ControlDB.LengthBytes = -1 }, invalid},
		{"control db checksum unset", func(m *contracts.ManifestV2) { m.ControlDB.SHA256 = "" }, invalid},
		{"control db checksum short", func(m *contracts.ManifestV2) { m.ControlDB.SHA256 = hexA[:63] }, invalid},
		{"control db checksum uppercase", func(m *contracts.ManifestV2) { m.ControlDB.SHA256 = strings.ToUpper(hexA) }, invalid},
		{"control db checksum not hex", func(m *contracts.ManifestV2) { m.ControlDB.SHA256 = strings.Repeat("g", 64) }, invalid},
		{"control db path absolute", func(m *contracts.ManifestV2) { m.ControlDB.RelativePath = "/etc/control.db" }, invalid},
		{"control db path traverses", func(m *contracts.ManifestV2) { m.ControlDB.RelativePath = "../control.db" }, invalid},
		{"control db path is dot dot", func(m *contracts.ManifestV2) { m.ControlDB.RelativePath = ".." }, invalid},
		{"control db path nested", func(m *contracts.ManifestV2) { m.ControlDB.RelativePath = "sub/control.db" }, invalid},
		{"control db path backslash", func(m *contracts.ManifestV2) { m.ControlDB.RelativePath = `sub\control.db` }, invalid},
		{"control db path takes the manifest name", func(m *contracts.ManifestV2) { m.ControlDB.RelativePath = "manifest.json" }, invalid},

		{"brain id unset", func(m *contracts.ManifestV2) { m.Brains[0].ID = "" }, invalid},
		{"brain id traverses", func(m *contracts.ManifestV2) { m.Brains[0].ID = "../x" }, invalid},
		{"brains unsorted", func(m *contracts.ManifestV2) { m.Brains[0], m.Brains[2] = m.Brains[2], m.Brains[0] }, invalid},
		{"brain duplicated out of order", func(m *contracts.ManifestV2) {
			m.Brains[2] = m.Brains[0]
		}, invalid},
		{"brain duplicated adjacently", func(m *contracts.ManifestV2) {
			m.Brains[1] = contracts.BrainArtifact{ID: m.Brains[0].ID, Empty: true}
		}, invalid},

		{"bundle path unset", func(m *contracts.ManifestV2) { m.Brains[0].RelativePath = "" }, invalid},
		{"bundle path traverses", func(m *contracts.ManifestV2) { m.Brains[0].RelativePath = "../brain-a.bundle" }, invalid},
		{"bundle path absolute", func(m *contracts.ManifestV2) { m.Brains[0].RelativePath = "/tmp/brain-a.bundle" }, invalid},
		{"bundle length zero", func(m *contracts.ManifestV2) { m.Brains[0].LengthBytes = 0 }, invalid},
		{"bundle checksum unset", func(m *contracts.ManifestV2) { m.Brains[0].SHA256 = "" }, invalid},
		{"bundle checksum uppercase", func(m *contracts.ManifestV2) { m.Brains[0].SHA256 = strings.ToUpper(hexB) }, invalid},
		{"bundle duplicates the control db path", func(m *contracts.ManifestV2) { m.Brains[0].RelativePath = "control.db" }, invalid},
		{"two bundles share a path", func(m *contracts.ManifestV2) { m.Brains[2].RelativePath = "brain-a.bundle" }, invalid},
		{"two bundles differ only by case", func(m *contracts.ManifestV2) { m.Brains[2].RelativePath = "BRAIN-A.bundle" }, invalid},
		{"bundle takes the manifest name", func(m *contracts.ManifestV2) { m.Brains[0].RelativePath = "Manifest.json" }, invalid},

		{"non-empty brain without heads", func(m *contracts.ManifestV2) { m.Brains[0].Heads = nil }, invalid},
		{"head ref unset", func(m *contracts.ManifestV2) { m.Brains[0].Heads[0].Ref = "" }, invalid},
		{"head ref is a bare branch name", func(m *contracts.ManifestV2) { m.Brains[0].Heads[1].Ref = "main" }, invalid},
		{"head ref is only the refs prefix", func(m *contracts.ManifestV2) { m.Brains[0].Heads[1].Ref = "refs/" }, invalid},
		{"head ref has a space", func(m *contracts.ManifestV2) { m.Brains[0].Heads[1].Ref = "refs/heads/a b" }, invalid},
		{"heads unsorted", func(m *contracts.ManifestV2) {
			h := m.Brains[0].Heads
			h[0], h[1] = h[1], h[0]
		}, invalid},
		{"heads duplicated", func(m *contracts.ManifestV2) { m.Brains[0].Heads[1] = m.Brains[0].Heads[0] }, invalid},
		{"head object id unset", func(m *contracts.ManifestV2) { m.Brains[0].Heads[0].ObjectID = "" }, invalid},
		{"head object id wrong length", func(m *contracts.ManifestV2) { m.Brains[0].Heads[0].ObjectID = oid1[:39] }, invalid},
		{"head object id uppercase", func(m *contracts.ManifestV2) { m.Brains[0].Heads[0].ObjectID = strings.Repeat("A", 40) }, invalid},
		{"head object id not hex", func(m *contracts.ManifestV2) { m.Brains[0].Heads[0].ObjectID = strings.Repeat("z", 40) }, invalid},

		{"empty brain with a bundle", func(m *contracts.ManifestV2) {
			m.Brains[1].ArtifactRef = contracts.ArtifactRef{RelativePath: "brain-b.bundle", LengthBytes: 1, SHA256: hexA}
		}, invalid},
		{"empty brain with a stray length", func(m *contracts.ManifestV2) { m.Brains[1].LengthBytes = 9 }, invalid},
		{"empty brain with heads", func(m *contracts.ManifestV2) {
			m.Brains[1].Heads = []contracts.BundleHead{{Ref: "HEAD", ObjectID: oid1}}
		}, invalid},

		{"watermark generation only", func(m *contracts.ManifestV2) {
			m.JournalWatermark = contracts.DeletionWatermark{Generation: 1}
		}, invalid},
		{"watermark hash only", func(m *contracts.ManifestV2) {
			m.JournalWatermark = contracts.DeletionWatermark{EntryHash: hexA}
		}, invalid},
		{"watermark sequence zero", func(m *contracts.ManifestV2) { m.JournalWatermark.SequenceID = 0 }, invalid},
		{"watermark hash malformed", func(m *contracts.ManifestV2) { m.JournalWatermark.EntryHash = "h" }, invalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := validManifest()
			tc.mutate(&m)
			err := m.Validate()
			if err == nil {
				t.Fatalf("a manifest with %q validated", tc.name)
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want an error wrapping %v", err, tc.want)
			}
		})
	}
}

// TestManifestV2VersionErrorIsDistinct: a legacy or future manifest is a
// support-policy question for task49, not a "malformed" one, so the two error
// classes must not overlap.
func TestManifestV2VersionErrorIsDistinct(t *testing.T) {
	m := validManifest()
	m.Version = 1
	err := m.Validate()
	if errors.Is(err, contracts.ErrManifestInvalid) {
		t.Fatalf("a version error must not also be ErrManifestInvalid: %v", err)
	}
	if !errors.Is(err, contracts.ErrManifestVersion) {
		t.Fatalf("got %v, want ErrManifestVersion", err)
	}
}

// TestManifestV2JSONRoundTrip pins the wire form: snake_case keys, the bundle
// artifact flattened into its brain, and no reserved field for an incremental
// scheme. A round trip must reproduce the manifest exactly and still validate.
func TestManifestV2JSONRoundTrip(t *testing.T) {
	want := validManifest()
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got contracts.ManifestV2
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip changed the manifest\n got %+v\nwant %+v", got, want)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("a round-tripped manifest must validate: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	wantKeys := map[string]bool{"version": true, "source": true, "created_at": true, "control_db": true, "brains": true, "journal_watermark": true}
	if len(keys) != len(wantKeys) {
		t.Fatalf("top-level keys = %v, want exactly %v", keys, wantKeys)
	}
	for _, k := range keys {
		if !wantKeys[k] {
			t.Fatalf("unexpected top-level key %q: the manifest reserves no field for a future scheme", k)
		}
	}

	var brains []map[string]json.RawMessage
	if err := json.Unmarshal(raw["brains"], &brains); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "relative_path", "length_bytes", "sha256", "empty", "heads"} {
		if _, ok := brains[0][key]; !ok {
			t.Fatalf("a non-empty brain's JSON lacks %q: %s", key, raw["brains"])
		}
	}
	if _, ok := brains[1]["heads"]; ok {
		t.Fatalf("an empty brain's JSON must omit heads: %s", raw["brains"])
	}
}
