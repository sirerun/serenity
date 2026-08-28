package cli

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/connector"
	fileconn "github.com/sirerun/serenity/internal/connector/file"
	"github.com/sirerun/serenity/internal/connector/gitrepo"
	imapconn "github.com/sirerun/serenity/internal/connector/imap"
)

// buildConnectors constructs one connector.Connector per entry under
// serenity.yml's `connectors:` map (T1.15 -- the config schema below is new;
// `serenity connectors auth imap` (T1.4) already writes the `imap` shape).
// Connectors with no configured entry are simply absent from the result --
// a brain repo with nothing configured yet polls nothing, not an error.
//
// file: a single watched directory (RFC section 10.1's file-watcher
// connector). Always constructed in poll mode (fileconn.NewPoll), never
// watch mode: `serenity sync` is a one-shot CLI invocation, and watch
// mode's fsnotify goroutine only accumulates change events while it runs
// in the background -- started moments before Poll is called, it would
// see none. Poll mode's "rescan the whole tree on every Poll call" is
// exactly the one-shot model sync uses; watch mode remains available to a
// long-running caller (a future daemon) via the package directly. Only one
// directory is supported today: internal/connector/file.Connector.Name()
// always returns the constant "file" (no per-root distinguishing suffix),
// so two configured roots would collide on one cursor/job-history slot --
// disclosed limitation, not silently wrong. Multiple file roots is
// unblocked follow-up work (mirroring gitrepo's own path-derived Name()).
//
// git_repo: a list of repositories to crawl (RFC section 10.1's "5 repos"
// M1 acceptance criterion) -- gitrepo.Connector.Name() already derives a
// distinct name per repo root, so this is the one connector kind that
// supports multiple instances out of the box.
//
// imap: exactly one mailbox, matching `serenity connectors auth imap`'s
// existing `{account: ...}` shape; the app password is read from the OS
// keychain by imapconn.NewGmail, never from serenity.yml.
func buildConnectors(root string, cfg *config.Config) ([]connector.Connector, error) {
	var cs []connector.Connector

	if raw, ok := cfg.Connectors["imap"]; ok {
		var c struct {
			Account string `yaml:"account"`
		}
		if err := decodeConnectorConfig(raw, &c); err != nil {
			return nil, fmt.Errorf("connectors.imap: %w", err)
		}
		if c.Account == "" {
			return nil, fmt.Errorf("connectors.imap: account is required")
		}
		cs = append(cs, imapconn.NewGmail(c.Account))
	}

	if raw, ok := cfg.Connectors["file"]; ok {
		var c struct {
			Path string `yaml:"path"`
		}
		if err := decodeConnectorConfig(raw, &c); err != nil {
			return nil, fmt.Errorf("connectors.file: %w", err)
		}
		if c.Path == "" {
			return nil, fmt.Errorf("connectors.file: path is required")
		}
		cs = append(cs, fileconn.NewPoll(c.Path))
	}

	if raw, ok := cfg.Connectors["git_repo"]; ok {
		var list []struct {
			Path string `yaml:"path"`
		}
		if err := decodeConnectorConfig(raw, &list); err != nil {
			return nil, fmt.Errorf("connectors.git_repo: %w", err)
		}
		for i, c := range list {
			if c.Path == "" {
				return nil, fmt.Errorf("connectors.git_repo[%d]: path is required", i)
			}
			cs = append(cs, gitrepo.New(gitrepo.Config{RepoRoot: c.Path, BrainRoot: root}))
		}
	}

	return cs, nil
}

// decodeConnectorConfig re-decodes one `connectors.<name>` entry (loaded by
// config.Load as `any`, since Config.Connectors is intentionally untyped
// -- serenity.yml's vocabulary of connector kinds grows independently of
// internal/config) into a typed shape, via a YAML round-trip rather than a
// reflection-based mapstructure dependency: yaml.v3 already unmarshals
// mapping nodes into map[string]any (string keys, not v2's
// map[interface{}]interface{}), so re-marshaling that and unmarshaling
// into out is a small, dependency-free, already-imported-package way to
// get a typed struct back out of an any.
func decodeConnectorConfig(raw any, out any) error {
	b, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, out)
}
