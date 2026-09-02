package providers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/router"
)

// Package providers is the one place a brain repo's serenity.yml pins are
// turned into live handles: the derived SQLite index (OpenIndex), the spend
// ledger over it (IndexSpendLedger), and a *router.Router per pinned model
// (BuildExtractionRouter, BuildEmbeddingRouter, BuildComposerRouter). It
// was internal/cli/providers.go until T4.18 moved it here unchanged so the
// read-only facade (pkg/serenity, ADR 012) can call exactly the wiring the
// CLI calls without importing the CLI package and its cobra dependency;
// internal/cli is its other consumer.
//
// This file closes the "construct a real router.Provider from serenity.yml"
// gap T1.15 found open: internal/router (T1.7) never grew a production
// wiring path from a pinned "<model>@<version>" string to a live provider
// -- every existing caller (extract_test.go, embed_test.go, router_test.go)
// builds a fake router.Provider by hand. serenity.yml's Models struct (§7.5)
// pins a model identifier only, with no separate provider field or
// credential storage, so provider selection here is deliberately minimal:
// inferred from the model name, credentials read from the environment
// (not the OS keychain internal/connector/imap uses -- there is no
// keychain schema for router credentials yet, and inventing one is out of
// this task's scope). This is disclosed, not silent: a missing credential
// or an unpinned model returns ok=false with a human-readable reason,
// exactly like the pre-T1.15 `serenity extract` stub's "none@v0 skipped"
// message, never a crash or a silently-skipped call.
//
// One further gap this closes: RouterEmbedder's documented interop
// convention (internal/embed/embed.go) requires a provider whose Send
// returns a JSON-array-of-floats Response.Text -- neither
// AnthropicProvider nor OpenAICompatibleProvider can produce that for a
// real embeddings endpoint (both speak chat-completions shapes). The new
// OpenAIEmbeddingsProvider (internal/router/openai_embeddings.go) is a
// real /embeddings adapter for exactly this reason.
//
// Both a pinned extraction model and a pinned embedding model resolve to
// router.TierLocalCheap (RFC section 9's task-class table), but they are
// two different models with two different credentials -- router.Router
// holds one provider per tier, so this deliberately builds two independent
// *router.Router values rather than trying to cram two models under one
// tier key. Nothing about router.Router itself changes.

// IndexSpendLedger satisfies router.SpendLedger by writing through
// index.SQLite.RecordSpend -- the concrete backing ledger.go's own doc
// comment calls for ("wired by whichever subsystem first holds a live
// index.Engine handle in production"). That subsystem is this one:
// T1.17 shipped RecordSpend/SpendRows with spend-to-date always reading
// back zero because nothing yet called Router.Complete for real; this is
// the first production call site.
type IndexSpendLedger struct {
	Eng *index.SQLite
}

// Record implements router.SpendLedger.
func (a *IndexSpendLedger) Record(ctx context.Context, e router.SpendEntry) error {
	return a.Eng.RecordSpend(ctx, index.SpendRow{
		ID:           e.ID,
		TaskClass:    string(e.TaskClass),
		Tier:         string(e.Tier),
		Provider:     e.Provider,
		ModelVersion: e.ModelVersion,
		InputTokens:  e.InputTokens,
		OutputTokens: e.OutputTokens,
		CostUSD:      e.CostUSD,
		OccurredAt:   e.OccurredAt,
	})
}

// splitPin splits a serenity.yml "<model>@<version>" pin into its two
// parts. ok is false when pin carries no "@" at all.
func splitPin(pin string) (model, version string, ok bool) {
	model, version, ok = strings.Cut(pin, "@")
	return model, version, ok
}

// unpinned reports whether pin is the install-time "no model configured"
// sentinel (config.Default()'s Models.Embedding/Extraction).
func unpinned(pin string) bool {
	return pin == "" || pin == "none@v0"
}

// BuildExtractionRouter constructs a *router.Router whose local-cheap
// provider is serenity.yml's pinned extraction model. ok is false --
// extraction must be explicitly skipped, never silently no-op'd -- when no
// model is pinned or its credential is not configured; note explains why.
func BuildExtractionRouter(cfg *config.Config, ledger router.SpendLedger) (r *router.Router, ok bool, note string) {
	pin := cfg.Models.Extraction
	if unpinned(pin) {
		return nil, false, "no extraction model pinned (models.extraction: none@v0); extraction skipped"
	}
	model, version, split := splitPin(pin)
	if !split {
		return nil, false, fmt.Sprintf("models.extraction %q is not shaped <model>@<version>; extraction skipped", pin)
	}

	var p router.Provider
	if strings.Contains(strings.ToLower(model), "claude") {
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return nil, false, "models.extraction is pinned to a Claude model but ANTHROPIC_API_KEY is not set; extraction skipped"
		}
		p = &router.AnthropicProvider{APIKey: key, Model: model, Version: version}
	} else {
		key := os.Getenv("OPENAI_API_KEY")
		baseURL := os.Getenv("OPENAI_BASE_URL")
		if key == "" && baseURL == "" {
			return nil, false, "models.extraction is pinned but neither OPENAI_API_KEY nor OPENAI_BASE_URL (local server) is set; extraction skipped"
		}
		p = &router.OpenAICompatibleProvider{APIKey: key, BaseURL: baseURL, Model: model, Version: version}
	}
	return router.New(map[router.Tier]router.Provider{router.TierLocalCheap: p}, ledger), true, ""
}

// BuildEmbeddingRouter constructs a *router.Router whose local-cheap
// provider is serenity.yml's pinned embedding model, via the real
// OpenAIEmbeddingsProvider adapter. ok is false under the same
// explicit-skip contract as BuildExtractionRouter.
func BuildEmbeddingRouter(cfg *config.Config, ledger router.SpendLedger) (r *router.Router, ok bool, note string) {
	pin := cfg.Models.Embedding
	if unpinned(pin) {
		return nil, false, "no embedding model pinned (models.embedding: none@v0); embedding skipped"
	}
	model, version, split := splitPin(pin)
	if !split {
		return nil, false, fmt.Sprintf("models.embedding %q is not shaped <model>@<version>; embedding skipped", pin)
	}

	key := os.Getenv("OPENAI_API_KEY")
	baseURL := os.Getenv("OPENAI_EMBEDDINGS_BASE_URL")
	if key == "" && baseURL == "" {
		return nil, false, "models.embedding is pinned but neither OPENAI_API_KEY nor OPENAI_EMBEDDINGS_BASE_URL (local server) is set; embedding skipped"
	}
	p := &router.OpenAIEmbeddingsProvider{APIKey: key, BaseURL: baseURL, Model: model, Version: version}
	return router.New(map[router.Tier]router.Provider{router.TierLocalCheap: p}, ledger), true, ""
}

// BuildComposerRouter constructs a *router.Router whose judgment-tier
// provider is serenity.yml's pinned composer model (T1.12, RFC §11) --
// TaskClassComposerSynthesis resolves to router.TierJudgment (router.go's
// closed task-class table), unlike extraction/embedding's local-cheap
// pin, so this wires TierJudgment rather than reusing
// BuildExtractionRouter's tier. Same credential-from-env inference and
// explicit-skip contract as BuildExtractionRouter/BuildEmbeddingRouter.
func BuildComposerRouter(cfg *config.Config, ledger router.SpendLedger) (r *router.Router, ok bool, note string) {
	pin := cfg.Models.Composer
	if unpinned(pin) {
		return nil, false, "no composer model pinned (models.composer: none@v0); ask skipped"
	}
	model, version, split := splitPin(pin)
	if !split {
		return nil, false, fmt.Sprintf("models.composer %q is not shaped <model>@<version>; ask skipped", pin)
	}

	var p router.Provider
	if strings.Contains(strings.ToLower(model), "claude") {
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return nil, false, "models.composer is pinned to a Claude model but ANTHROPIC_API_KEY is not set; ask skipped"
		}
		p = &router.AnthropicProvider{APIKey: key, Model: model, Version: version}
	} else {
		key := os.Getenv("OPENAI_API_KEY")
		baseURL := os.Getenv("OPENAI_BASE_URL")
		if key == "" && baseURL == "" {
			return nil, false, "models.composer is pinned but neither OPENAI_API_KEY nor OPENAI_BASE_URL (local server) is set; ask skipped"
		}
		p = &router.OpenAICompatibleProvider{APIKey: key, BaseURL: baseURL, Model: model, Version: version}
	}
	return router.New(map[router.Tier]router.Provider{router.TierJudgment: p}, ledger), true, ""
}

// OpenIndex opens (creating if needed) root's derived SQLite index at
// root/.serenity/index.db -- derived state, never canonical (RFC §7.5),
// so creating the directory is not a brain write.
func OpenIndex(root string) (*index.SQLite, error) {
	dir := filepath.Join(root, ".serenity")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return index.Open(filepath.Join(dir, "index.db"))
}
