package router

import "github.com/sirerun/serenity/internal/redact"

// SetRedaction installs the redaction options Complete applies at the
// provider-egress chokepoint (ADR 021). The built-in pattern table always
// runs; opts can only add to it (Options.Patterns, from serenity.yml
// `redact.patterns`) or switch on the on-request email rule. It is
// wiring, called once when the router is built (internal/providers),
// not a per-call setting: set it before the router is shared across
// goroutines.
func (r *Router) SetRedaction(opts redact.Options) { r.redact = opts }
