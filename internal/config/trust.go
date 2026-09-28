package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

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

// ConnectorTrust resolves connectors.<name>.trust against the typed
// schema, falling back to the per-kind default. A git_repo list is trusted
// only when every entry says trusted. A name no typed connector owns is
// untrusted (fail closed).
func (c *Config) ConnectorTrust(name string) Trust {
	def, ok := defaultConnectorTrust[name]
	if !ok {
		def = TrustUntrusted
	}
	switch name {
	case "imap":
		if c.Connectors.IMAP == nil {
			return def
		}
		return parseTrust(string(c.Connectors.IMAP.Trust), def)
	case "file":
		if c.Connectors.File == nil {
			return def
		}
		return parseTrust(string(c.Connectors.File.Trust), def)
	case "git_repo":
		entries := c.Connectors.GitRepo
		if len(entries) == 0 {
			return def
		}
		for _, e := range entries {
			if parseTrust(string(e.Trust), def) != TrustTrusted {
				return TrustUntrusted
			}
		}
		return TrustTrusted
	default:
		return def
	}
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

// parseTrust resolves one trust value: empty falls back to def; anything
// else must be a Trust constant (Load already rejected other spellings via
// TrustField, so this only defends programmatic callers).
func parseTrust(raw string, def Trust) Trust {
	if raw == "" {
		return def
	}
	switch t := Trust(raw); t {
	case TrustTrusted, TrustUntrusted:
		return t
	default:
		return TrustUntrusted
	}
}

// validateConnectorTrust re-checks every configured connector's trust value
// against the vocabulary (the same strict-twice discipline T24.8 uses for
// unknown keys: the reflective walk and a named validation). Load calls it.
func validateConnectorTrust(c *Config) error {
	if c.Connectors.IMAP != nil {
		if err := checkTrustValue(string(c.Connectors.IMAP.Trust), "connectors.imap"); err != nil {
			return err
		}
	}
	if c.Connectors.File != nil {
		if err := checkTrustValue(string(c.Connectors.File.Trust), "connectors.file"); err != nil {
			return err
		}
	}
	for i, e := range c.Connectors.GitRepo {
		if err := checkTrustValue(string(e.Trust), fmt.Sprintf("connectors.git_repo[%d]", i)); err != nil {
			return err
		}
	}
	return nil
}

func checkTrustValue(raw, path string) error {
	switch raw {
	case "", string(TrustTrusted), string(TrustUntrusted):
		return nil
	}
	return fmt.Errorf("%s: trust must be %q or %q, got %q", path, TrustTrusted, TrustUntrusted, raw)
}

// TrustField decodes and validates a per-connector `trust:` value at load
// time, so a typo can never silently fall back to a default in either
// direction (ADR 022). The empty value means "use the per-kind default".
type TrustField string

func (t *TrustField) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("trust must be %q or %q, not a %s", TrustTrusted, TrustUntrusted, value.ShortTag())
	}
	var s string
	if err := value.Decode(&s); err != nil {
		return fmt.Errorf("trust: %w", err)
	}
	if err := checkTrustValue(s, "trust"); err != nil {
		return err
	}
	*t = TrustField(s)
	return nil
}
