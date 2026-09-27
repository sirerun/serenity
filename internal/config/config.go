// Package config loads and writes serenity.yml: connectors, the pinned
// model set (RFC §7.5), the index engine, and the predicate-family
// vocabulary with storage tiers (§7.2, §7.2a). The vocabulary is
// extensible only via this file plus migration — never ad hoc by workers.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/ladder"
	"github.com/sirerun/serenity/internal/redact"
)

// FileName is the canonical config file name at the brain repo root.
const FileName = "serenity.yml"

// Models is the pinned model set (§7.5): exact provider+model+version
// identifiers. The byte-identical rebuild invariant is asserted only
// within an unchanged pinned set; changing a pin is a migration.
type Models struct {
	Embedding  string `yaml:"embedding"`
	Extraction string `yaml:"extraction"`
	// Composer is the judgment-tier model `serenity ask` (T1.12, RFC
	// §11) routes TaskClassComposerSynthesis calls through. Same
	// "<model>@<version>" / "none@v0" convention as the other two pins.
	Composer string `yaml:"composer"`
	// Provider explicitly selects which adapter internal/providers'
	// Build*Router functions build for the Extraction and Composer pins:
	// "openrouter" | "anthropic" | "openai" (ADR 013). Empty -- the zero
	// value for every brain created before ADR 013 -- falls back to the
	// pre-existing "claude" substring inference on the model name, so
	// existing brains are unaffected. BuildEmbeddingRouter never reads
	// this field: OpenRouter has no embeddings endpoint.
	Provider string `yaml:"provider,omitempty"`
	// DisableThinking, when true, asks an OpenAI-compatible chat endpoint
	// to skip a reasoning-capable model's default "thinking" pass
	// (SGLang/vLLM's chat_template_kwargs.enable_thinking, sent verbatim
	// via router.OpenAICompatibleProvider.ExtraBody) for the Extraction
	// and Composer pins (T1.31; docs/devlog.md 2026-09-06 measures a
	// 6-15x per-call wall-clock reduction against a live qwen3.8-27b
	// endpoint, with equivalent extraction quality on the sampled
	// chunks). Default false so existing brains and real OpenAI/
	// OpenRouter calls are unaffected -- a real OpenAI/OpenRouter
	// endpoint does not recognize this field and some reject unknown
	// top-level fields outright, so this must stay opt-in, never a
	// default. BuildEmbeddingRouter never reads this field.
	DisableThinking bool `yaml:"disable_thinking,omitempty"`
}

// Family declares one predicate family: its storage tier and the
// confidence-decay half-life used by ranking (§10.2).
type Family struct {
	Tier         domain.Tier `yaml:"tier"`
	HalfLifeDays int         `yaml:"half_life_days"`
}

// Index selects the derived-index engine. SQLite is the default;
// Postgres+pgvector is the documented scale profile (§7.5).
type Index struct {
	Engine string `yaml:"engine"`
}

// Server configures the daemon's HTTP transport (RFC §14: "binds
// localhost with a bearer token required by default; LAN/Tailscale
// exposure is explicit config with token + optional mTLS"). The zero
// value is the secure default: loopback only, no mTLS.
type Server struct {
	// Bind is the listen address ("host:port"). Empty selects the
	// transport's own loopback default.
	Bind string `yaml:"bind,omitempty"`
	// AllowLAN is the explicit, separately-named opt-in RFC §14 requires
	// before Bind may resolve to anything but a loopback address.
	AllowLAN bool `yaml:"allow_lan,omitempty"`
	// ClientCAFile, set together with ServerCertFile/ServerKeyFile, turns
	// on mTLS: connections must present a certificate signed by this CA.
	// Only meaningful when AllowLAN is true.
	ClientCAFile string `yaml:"client_ca_file,omitempty"`
	// ServerCertFile and ServerKeyFile are the daemon's own TLS identity,
	// required alongside ClientCAFile for mTLS.
	ServerCertFile string `yaml:"server_cert_file,omitempty"`
	ServerKeyFile  string `yaml:"server_key_file,omitempty"`
	// MaxInFlightCalls bounds concurrent MCP tool calls for this daemon's
	// HTTP handler. Zero uses the transport default.
	MaxInFlightCalls int `yaml:"max_in_flight_calls,omitempty"`
}

// Connectors is serenity.yml's typed `connectors:` section (ADR 018
// decision 3, SEC-H05). It used to be an untyped map so connector kinds
// could grow without touching this package; that also meant a synced
// serenity.yml could carry any key and any shape unchecked. Every kind
// the CLI can build is now named here, so an unknown kind fails Load
// with the key named instead of being silently ignored, and the shape
// each kind decodes into is fixed at the schema rather than re-decoded
// at build time.
type Connectors struct {
	// Roots extends the allowlist of directories a connector `path` may
	// resolve under. The user's home directory is always allowed; each
	// entry here must be an absolute path and is cleaned before use. A
	// connector path (file.path, git_repo[].path) whose cleaned absolute
	// form lies outside every root fails connector build naming the path,
	// so a serenity.yml delivered through the brain remote cannot point
	// the process at an arbitrary repository.
	Roots  []string `yaml:"roots,omitempty"`
	Redact Redact   `yaml:"redact,omitempty"`
	// IMAP is the single Gmail mailbox `serenity connectors auth imap`
	// writes; the app password lives in the OS keychain, never here.
	IMAP *IMAPConnector `yaml:"imap,omitempty"`
	// File is the single watched directory (docs/connectors/file.md).
	File *FileConnector `yaml:"file,omitempty"`
	// GitRepo lists repositories to crawl, one entry each
	// (docs/connectors/gitrepo.md).
	GitRepo []GitRepoConnector `yaml:"git_repo,omitempty"`
}

// IMAPConnector is the `connectors.imap` entry.
type IMAPConnector struct {
	Account string `yaml:"account"`
}

// FileConnector is the `connectors.file` entry.
type FileConnector struct {
	Path string `yaml:"path"`
}

// GitRepoConnector is one `connectors.git_repo[]` entry.
type GitRepoConnector struct {
	Path string `yaml:"path"`
}

// Redact configures the redaction pass internal/router applies to every
// provider egress (ADR 021). The built-in table (API-key shapes, card
// and account numbers) always runs; this section can only extend it.
// There is deliberately no key that disables a built-in rule.
type Redact struct {
	// Patterns are operator-defined rules, applied after the built-in
	// table. A match is replaced by "[REDACTED:<NAME>]" (name
	// upper-cased). Load rejects an invalid regex or a missing name.
	Patterns []RedactPattern `yaml:"patterns,omitempty"`
}

// RedactPattern is one entry of `redact.patterns`.
type RedactPattern struct {
	// Name labels the placeholder; letters, digits, "_" and "-" only.
	Name string `yaml:"name"`
	// Regex is a Go (RE2) regular expression.
	Regex string `yaml:"regex"`
}

// Compile turns the configured entries into redact.Pattern values. Load
// calls it to fail fast on a bad rule; internal/providers calls it to
// wire the router. An error names the entry's index and name.
func (r Redact) Compile() ([]redact.Pattern, error) {
	patterns := make([]redact.Pattern, 0, len(r.Patterns))
	for i, p := range r.Patterns {
		compiled, err := redact.NewPattern(p.Name, p.Regex)
		if err != nil {
			return nil, fmt.Errorf("redact.patterns[%d]: %w", i, err)
		}
		patterns = append(patterns, compiled)
	}
	return patterns, nil
}

type Config struct {
	Version    int               `yaml:"version"`
	Models     Models            `yaml:"models"`
	Index      Index             `yaml:"index"`
	Server     Server            `yaml:"server,omitempty"`
	Families   map[string]Family `yaml:"families"`
	Connectors Connectors        `yaml:"connectors,omitempty"`
	Redact     Redact            `yaml:"redact,omitempty"`
	// Ladder is the earned-automation ladder policy object (RFC §10.3,
	// T2.10). config.Default seeds the RFC's published priors; T2.11's
	// calibration sweep replaces them with evidence-backed defaults before
	// launch. A brain repo created before T2.10 has no "ladder:" key in its
	// serenity.yml at all, so Load leaves this at its Go zero value on
	// those brains -- every field 0/empty, which internal/ladder's mandatory
	// correlation-guard validation (ParseConfig) never runs against here,
	// since plain per-field YAML decoding (not ParseConfig) is what Load
	// uses. A zero-value Ladder is inert: T2.2's reconcile engine (not yet
	// built) is expected to treat a config whose correlation guards are
	// both 0 as "ladder not configured for this brain" rather than feeding
	// it to ladder.NewEngine, the same disclosed-scope pattern ADR 013 used
	// for the Provider field's empty-string fallback.
	Ladder ladder.Config `yaml:"ladder,omitempty"`
}

// Default returns the install-time seed: the controlled predicate
// vocabulary of §7.2 with the tier assignments of §7.2a (balances,
// costs, and transaction-shaped families are shard-tier).
func Default() *Config {
	return &Config{
		Version: 1,
		Models: Models{
			// No models pinned at init. Extraction and embedding are
			// configured (and pinned) when the user connects a provider;
			// "none@v0" keeps the rebuild-identity assertion honest.
			Embedding:  "none@v0",
			Extraction: "none@v0",
			Composer:   "none@v0",
			// ADR 013: new brains default to OpenRouter for extraction/
			// composer once a model is pinned; this does not un-skip the
			// "no model pinned" default above.
			Provider: "openrouter",
		},
		Index: Index{Engine: "sqlite"},
		Families: map[string]Family{
			"works_at":           {Tier: domain.TierFence, HalfLifeDays: 90},
			"has_role":           {Tier: domain.TierFence, HalfLifeDays: 90},
			"owns_account":       {Tier: domain.TierFence, HalfLifeDays: 365},
			"has_balance":        {Tier: domain.TierShard, HalfLifeDays: 1},
			"has_condition":      {Tier: domain.TierFence, HalfLifeDays: 365},
			"takes_medication":   {Tier: domain.TierFence, HalfLifeDays: 90},
			"prefers":            {Tier: domain.TierFence, HalfLifeDays: 365},
			"committed_to":       {Tier: domain.TierFence, HalfLifeDays: 30},
			"deadline_on":        {Tier: domain.TierFence, HalfLifeDays: 30},
			"relates_to":         {Tier: domain.TierFence, HalfLifeDays: 365},
			"belongs_to_project": {Tier: domain.TierFence, HalfLifeDays: 180},
			"said":               {Tier: domain.TierFence, HalfLifeDays: 365},
			"costs":              {Tier: domain.TierShard, HalfLifeDays: 30},
		},
		Ladder: *ladder.DefaultConfig(),
	}
}

// Load reads and strictly decodes a serenity.yml. serenity.yml is committed
// into the brain and synced through the brain remote, so it is not
// first-party input (ADR 018, SEC-H05): any key the schema does not know,
// at any nesting level, fails Load with an error of the form
// `unknown key <dotted.path>` naming the first offender and its line.
// Strictness is enforced twice on purpose: unknownKeys walks the parsed
// document against the Config type to produce the named-path error, and
// the decoder's own KnownFields(true) backstops anything the walk does
// not model (a field with a custom unmarshaler, say) so a miss can never
// degrade to silent acceptance.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := unknownKeys(&doc, reflect.TypeFor[Config](), ""); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	// A bad redaction rule is a config error at load time, never a
	// silently dropped rule on the egress path (ADR 021).
	if _, err := c.Redact.Compile(); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := validateConnectorTrust(&c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &c, nil
}

// yamlUnmarshalerType is the interface yaml.v3 consults before its
// reflective decode; a type implementing it owns its own key vocabulary,
// so the strict walk stops at it and leaves KnownFields to the decoder.
var yamlUnmarshalerType = reflect.TypeFor[yaml.Unmarshaler]()

// unknownKeys walks node against t and returns the first mapping key that
// has no yaml-tagged field to land in, as `unknown key <path> (line N)`.
// Structs are checked key by key; maps recurse into their values with the
// key appended to the path; sequences recurse with the index appended.
// Aliases are followed and `<<` merge keys are validated against the
// same type as the mapping they merge into. Scalar mismatches are not
// this walk's job -- the decoder reports those.
func unknownKeys(node *yaml.Node, t reflect.Type, path string) error {
	if node == nil {
		return nil
	}
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return nil
		}
		return unknownKeys(node.Content[0], t, path)
	case yaml.AliasNode:
		return unknownKeys(node.Alias, t, path)
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if reflect.PointerTo(t).Implements(yamlUnmarshalerType) {
		return nil
	}
	switch t.Kind() {
	case reflect.Struct:
		if node.Kind != yaml.MappingNode {
			return nil
		}
		fields := yamlFields(t)
		return walkMapping(node, path, func(key string) (reflect.Type, bool) {
			ft, ok := fields[key]
			return ft, ok
		})
	case reflect.Map:
		if node.Kind != yaml.MappingNode {
			return nil
		}
		elem := t.Elem()
		return walkMapping(node, path, func(string) (reflect.Type, bool) { return elem, true })
	case reflect.Slice, reflect.Array:
		if node.Kind != yaml.SequenceNode {
			return nil
		}
		for i, item := range node.Content {
			if err := unknownKeys(item, t.Elem(), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		return nil
	default:
		return nil
	}
}

// walkMapping checks each key of a mapping node with lookup and recurses
// into each value with the type lookup returns. Merge keys (`<<`) are
// expanded against the same lookup so a merged mapping cannot smuggle a
// key past the check.
func walkMapping(node *yaml.Node, path string, lookup func(key string) (reflect.Type, bool)) error {
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode, valNode := node.Content[i], node.Content[i+1]
		if keyNode.Tag == "!!merge" || keyNode.Value == "<<" {
			if err := walkMerge(valNode, path, lookup); err != nil {
				return err
			}
			continue
		}
		key := keyNode.Value
		ft, ok := lookup(key)
		if !ok {
			return fmt.Errorf("unknown key %s (line %d)", joinPath(path, key), keyNode.Line)
		}
		if err := unknownKeys(valNode, ft, joinPath(path, key)); err != nil {
			return err
		}
	}
	return nil
}

func walkMerge(val *yaml.Node, path string, lookup func(key string) (reflect.Type, bool)) error {
	if val.Kind == yaml.AliasNode {
		val = val.Alias
	}
	switch val.Kind {
	case yaml.MappingNode:
		return walkMapping(val, path, lookup)
	case yaml.SequenceNode:
		for _, item := range val.Content {
			if err := walkMerge(item, path, lookup); err != nil {
				return err
			}
		}
	}
	return nil
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// yamlFields maps each yaml key a struct type accepts to the field type
// it decodes into, following yaml.v3's tag rules: the tag name wins, `-`
// skips, an untagged exported field is its lowercased name, and `inline`
// hoists an embedded struct's keys to this level.
func yamlFields(t reflect.Type) map[string]reflect.Type {
	out := map[string]reflect.Type{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("yaml")
		name, opts, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}
		if strings.Contains(","+opts+",", ",inline,") {
			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				for k, v := range yamlFields(ft) {
					out[k] = v
				}
			}
			continue
		}
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		out[name] = f.Type
	}
	return out
}

func (c *Config) Save(path string) error {
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// TierOf returns the storage tier for a predicate family. Unknown
// families default to fence-tier (the conservative, human-readable home).
func (c *Config) TierOf(family string) domain.Tier {
	if f, ok := c.Families[family]; ok && f.Tier == domain.TierShard {
		return domain.TierShard
	}
	return domain.TierFence
}

// FamilyNames returns the vocabulary in sorted order (deterministic
// iteration for writers and reports).
func (c *Config) FamilyNames() []string {
	names := make([]string, 0, len(c.Families))
	for n := range c.Families {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
