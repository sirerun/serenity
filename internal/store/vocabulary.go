package store

import (
	"errors"

	"github.com/sirerun/serenity/internal/config"
)

// ErrUnknownPredicate is returned by the writers (FenceWriter.RenderEntity,
// ShardStore.Append) when a claim's predicate/family is not in the
// controlled vocabulary (RFC §7.2). Wrapped with the offending predicate;
// callers detect this failure class with errors.Is.
var ErrUnknownPredicate = errors.New("store: predicate not in controlled vocabulary (serenity.yml)")

// defaultVocabulary is the controlled predicate vocabulary seeded at
// install (RFC §7.2, T0.8): the floor every writer enforces when a caller
// does not supply a project-specific Vocabulary loaded from the repo's
// serenity.yml. The vocabulary is extensible only via serenity.yml +
// migration — never ad hoc from inside a writer.
var defaultVocabulary = vocabularyOf(config.Default())

// vocabularyOf turns a config's declared predicate families into a
// membership set for writer-side enforcement.
func vocabularyOf(cfg *config.Config) map[string]bool {
	out := make(map[string]bool, len(cfg.Families))
	for name := range cfg.Families {
		out[name] = true
	}
	return out
}
