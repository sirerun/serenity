package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirerun/serenity/internal/config"
)

// TestConfigSetModelRewritesExactPin proves runConfigSetModel rewrites
// exactly the named pin and leaves the other two untouched (plan T1.25
// acc: "running the command against a temp brain rewrites exactly the
// named pin in serenity.yml and leaves the other two untouched").
func TestConfigSetModelRewritesExactPin(t *testing.T) {
	const (
		startExtraction = "start-extraction@v1"
		startEmbedding  = "start-embedding@v1"
		startComposer   = "start-composer@v1"
		newModel        = "new-model@v1"
	)

	tests := []struct {
		purpose string
	}{
		{"extraction"},
		{"embedding"},
		{"composer"},
	}

	for _, tt := range tests {
		t.Run(tt.purpose, func(t *testing.T) {
			root := t.TempDir()
			cfgPath := filepath.Join(root, config.FileName)

			cfg := config.Default()
			cfg.Models.Extraction = startExtraction
			cfg.Models.Embedding = startEmbedding
			cfg.Models.Composer = startComposer
			if err := cfg.Save(cfgPath); err != nil {
				t.Fatalf("seed Save: %v", err)
			}

			if err := runConfigSetModel(root, tt.purpose, newModel, io.Discard); err != nil {
				t.Fatalf("runConfigSetModel(%q): %v", tt.purpose, err)
			}

			got, err := config.Load(cfgPath)
			if err != nil {
				t.Fatalf("Load after runConfigSetModel: %v", err)
			}

			want := map[string]string{
				"extraction": startExtraction,
				"embedding":  startEmbedding,
				"composer":   startComposer,
			}
			want[tt.purpose] = newModel

			if got.Models.Extraction != want["extraction"] {
				t.Fatalf("Models.Extraction = %q, want %q", got.Models.Extraction, want["extraction"])
			}
			if got.Models.Embedding != want["embedding"] {
				t.Fatalf("Models.Embedding = %q, want %q", got.Models.Embedding, want["embedding"])
			}
			if got.Models.Composer != want["composer"] {
				t.Fatalf("Models.Composer = %q, want %q", got.Models.Composer, want["composer"])
			}
		})
	}
}

// TestConfigSetModelUnknownPurposeErrors proves an unknown purpose name
// errors without writing anything -- the file must come back
// byte-identical to before the call (plan T1.25 acc: "an unknown purpose
// name errors without writing").
func TestConfigSetModelUnknownPurposeErrors(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, config.FileName)

	if err := config.Default().Save(cfgPath); err != nil {
		t.Fatalf("seed Save: %v", err)
	}

	before, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read before snapshot: %v", err)
	}

	if err := runConfigSetModel(root, "reticulation", "whatever@v1", io.Discard); err == nil {
		t.Fatal("runConfigSetModel with an unknown purpose returned nil error, want non-nil")
	}

	after, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read after snapshot: %v", err)
	}

	if !bytes.Equal(before, after) {
		t.Fatalf("serenity.yml was modified by a failed set-model call:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}
