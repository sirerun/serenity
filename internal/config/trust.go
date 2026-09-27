package config

import "fmt"

// Trust is a connector's trust class (ADR 022, T24.16). Content from an
// untrusted connector can carry planted text, so a first-seen machine claim
// extracted from it waits for a human accept before it can be cited.
type Trust string

const (
	TrustTrusted   Trust = "trusted"
	TrustUntrusted Trust = "untrusted"
)

// defaultConnectorTrust is the per-kind default when serenity.yml sets no
// `trust:` for a connector. Mail and crawled repositories carry text written
// by other people; a watched file or voice-note directory is the owner's own.
// Any connector name not listed here is untrusted.
var defaultConnectorTrust = map[string]Trust{
	"imap":     TrustUntrusted,
	"git_repo": TrustUntrusted,
	"file":     TrustTrusted,
	"voice":    TrustTrusted,
}

// sourceKindConnector maps a domain.Source.Kind to the `connectors:` key
// that produces it.
var sourceKindConnector = map[string]string{
	"email":    "imap",
	"git_repo": "git_repo",
	"file":     "file",
	"voice":    "voice",
}

// ConnectorForSourceKind names the `connectors:` key that produces sources
// of kind, or "" for a kind no connector owns.
func ConnectorForSourceKind(kind string) string { return sourceKindConnector[kind] }

// ConnectorTrust resolves connectors.<name>.trust, falling back to the
// per-kind default. A list-valued entry (git_repo) is trusted only when every
// element says trusted. A value Load would reject resolves untrusted.
func (c *Config) ConnectorTrust(name string) Trust {
	def, ok := defaultConnectorTrust[name]
	if !ok {
		def = TrustUntrusted
	}
	raw, ok := c.Connectors[name]
	if !ok {
		return def
	}
	trust, err := entryTrust(raw, def)
	if err != nil {
		return TrustUntrusted
	}
	return trust
}

// SourceTrust is the trust class of the connector that produces sources of
// kind. A kind no connector owns is untrusted (fail closed).
func (c *Config) SourceTrust(kind string) Trust {
	name := sourceKindConnector[kind]
	if name == "" {
		return TrustUntrusted
	}
	return c.ConnectorTrust(name)
}

func validateConnectorTrust(c *Config) error {
	for name, raw := range c.Connectors {
		if _, err := entryTrust(raw, TrustUntrusted); err != nil {
			return fmt.Errorf("connectors.%s: %w", name, err)
		}
	}
	return nil
}

func entryTrust(raw any, def Trust) (Trust, error) {
	switch v := raw.(type) {
	case map[string]any:
		return fieldTrust(v, def)
	case []any:
		if len(v) == 0 {
			return def, nil
		}
		all := TrustTrusted
		for i, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				if def == TrustUntrusted {
					all = TrustUntrusted
				}
				continue
			}
			t, err := fieldTrust(m, def)
			if err != nil {
				return "", fmt.Errorf("[%d]: %w", i, err)
			}
			if t != TrustTrusted {
				all = TrustUntrusted
			}
		}
		return all, nil
	default:
		return def, nil
	}
}

func fieldTrust(m map[string]any, def Trust) (Trust, error) {
	raw, ok := m["trust"]
	if !ok {
		return def, nil
	}
	s, ok := raw.(string)
	switch {
	case ok && Trust(s) == TrustTrusted:
		return TrustTrusted, nil
	case ok && Trust(s) == TrustUntrusted:
		return TrustUntrusted, nil
	}
	return "", fmt.Errorf("trust must be %q or %q, got %v", TrustTrusted, TrustUntrusted, raw)
}
