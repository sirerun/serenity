// Package service assembles the hosted service without self-hosted daemon credentials.
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirerun/serenity/internal/embed"
	"github.com/sirerun/serenity/internal/hosted/backup"
	"github.com/sirerun/serenity/internal/hosted/billing"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/dashboard"
	"github.com/sirerun/serenity/internal/hosted/gateway"
	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/meter"
	"github.com/sirerun/serenity/internal/hosted/operation"
	"github.com/sirerun/serenity/internal/hosted/pool"
	"github.com/sirerun/serenity/internal/hosted/provision"
	"github.com/sirerun/serenity/internal/hosted/store"
	"github.com/sirerun/serenity/internal/router"
)

type Config struct {
	BillingEnabled   bool   `json:"billing_enabled"`
	BuilderPrice     string `json:"builder_price"`
	ScalePrice       string `json:"scale_price"`
	billingConfig    *billing.Config
	Bind             string                     `json:"bind"`
	DataDir          string                     `json:"data_dir"`
	SecretsDir       string                     `json:"secrets_dir"`
	PublicOrigin     string                     `json:"public_origin"`
	EmbeddingModel   string                     `json:"embedding_model"`
	EmbeddingVersion string                     `json:"embedding_version"`
	EmbeddingBaseURL string                     `json:"embedding_base_url"`
	Sender           string                     `json:"sender"`
	MaxOpen          int                        `json:"max_open"`
	MaxInFlight      int                        `json:"max_in_flight"`
	AccountCap       int                        `json:"account_cap"`
	RegistrationMode contracts.RegistrationMode `json:"registration_mode"`
	InviteAllowlist  []string                   `json:"invite_allowlist"`
}

func Load(path string) (Config, error) {
	var c Config
	b, e := os.ReadFile(path)
	if e != nil {
		return c, e
	}
	decoder := json.NewDecoder(strings.NewReader(string(b)))
	decoder.DisallowUnknownFields()
	if e = decoder.Decode(&c); e != nil {
		return c, e
	}
	if e = decoder.Decode(new(any)); e != io.EOF {
		return c, errors.New("configuration must contain one JSON object")
	}
	return c, nil
}
func (c *Config) Validate(dev bool) error {
	if c.RegistrationMode == "" {
		c.RegistrationMode = contracts.RegistrationPublic
	}
	if c.RegistrationMode != contracts.RegistrationPublic && c.RegistrationMode != contracts.RegistrationInviteOnly {
		return errors.New("registration_mode must be public or invite_only")
	}
	for i, raw := range c.InviteAllowlist {
		email := strings.ToLower(strings.TrimSpace(raw))
		address, e := mail.ParseAddress(email)
		if e != nil || address.Address != email || len(email) > 254 {
			return fmt.Errorf("invite_allowlist[%d] must be an exact email address", i)
		}
		c.InviteAllowlist[i] = email
	}
	if c.RegistrationMode == contracts.RegistrationInviteOnly && len(c.InviteAllowlist) == 0 {
		return errors.New("invite_only registration requires invite_allowlist")
	}
	host, _, err := net.SplitHostPort(c.Bind)
	if err != nil {
		return errors.New("bind must be a loopback IP and port")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("hosted bind must be loopback")
	}
	u, err := url.Parse(c.PublicOrigin)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return errors.New("public_origin must be an origin without path, credentials or query")
	}
	if dev {
		if u.Scheme != "http" || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() {
			return errors.New("development origin must be HTTP loopback")
		}
	} else if u.Scheme != "https" {
		return errors.New("public_origin must use HTTPS")
	}
	if !filepath.IsAbs(c.DataDir) || !filepath.IsAbs(c.SecretsDir) {
		return errors.New("data_dir and secrets_dir must be absolute")
	}
	if c.EmbeddingModel == "" || c.EmbeddingVersion == "" {
		return errors.New("an explicit embedding model and version are required")
	}
	if c.EmbeddingBaseURL != "" {
		v, e := url.Parse(c.EmbeddingBaseURL)
		if e != nil || v.User != nil || v.Host == "" || v.RawQuery != "" || v.Fragment != "" {
			return errors.New("invalid embedding provider URL")
		}
		if v.Scheme != "https" && (!dev || v.Scheme != "http" || net.ParseIP(v.Hostname()) == nil || !net.ParseIP(v.Hostname()).IsLoopback()) {
			return errors.New("embedding provider must use HTTPS")
		}
	}
	if c.MaxOpen == 0 {
		c.MaxOpen = 8
	}
	if c.MaxInFlight == 0 {
		c.MaxInFlight = 16
	}
	if c.AccountCap == 0 {
		c.AccountCap = 100
	}
	if c.MaxOpen < 1 || c.MaxInFlight < 1 || c.AccountCap < 1 {
		return errors.New("capacity values must be positive")
	}
	if c.Sender == "" {
		c.Sender = "login@serenity.sire.run"
	}
	return nil
}
func readSecret(dir, name string) (string, error) {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return "", errors.New("secrets_dir must be a real directory with mode 0700")
	}
	path := filepath.Join(dir, name)
	info, err = os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("required secret %s is missing", name)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return "", fmt.Errorf("secret %s must be a private regular file", name)
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read secret %s", name)
	}
	out := strings.TrimSpace(string(value))
	if out == "" || out == "UNCONFIGURED" {
		return "", fmt.Errorf("required secret %s is empty", name)
	}
	return out, nil
}

type Service struct {
	Store    *store.Store
	Pool     *pool.Pool
	Gateway  *gateway.Gateway
	Handler  http.Handler
	cfg      Config
	embedder embed.Embedder
	readyMu  sync.Mutex
	readyAt  time.Time
	ready    bool
}
type ledger struct{ db *store.Store }

func (l ledger) Record(ctx context.Context, e router.SpendEntry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return l.db.Transaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO audit_log(actor,action,created_at,detail) VALUES('provider','embedding',?,?)`, store.Stamp(e.OccurredAt), string(data))
		return err
	})
}
func New(cfg Config, dev bool, devOutput io.Writer) (*Service, error) {
	if err := cfg.Validate(dev); err != nil {
		return nil, err
	}
	embeddingKey, err := readSecret(cfg.SecretsDir, "EMBEDDINGS_API_KEY")
	if err != nil {
		return nil, err
	}
	if cfg.BillingEnabled {
		key, e := readSecret(cfg.SecretsDir, "STRIPE_SECRET_KEY")
		if e != nil {
			return nil, e
		}
		webhook, e := readSecret(cfg.SecretsDir, "STRIPE_WEBHOOK_SECRET")
		if e != nil {
			return nil, e
		}
		if cfg.BuilderPrice == "" || cfg.ScalePrice == "" {
			return nil, errors.New("billing requires configured Builder and Scale price IDs")
		}
		cfg.billingConfig = &billing.Config{SecretKey: key, WebhookSecret: webhook, BuilderPrice: cfg.BuilderPrice, ScalePrice: cfg.ScalePrice, Origin: cfg.PublicOrigin}
	}
	var sender identity.Sender
	if dev {
		sender = &identity.DevSender{Writer: devOutput}
	} else {
		key, e := readSecret(cfg.SecretsDir, "RESEND_API_KEY")
		if e != nil {
			return nil, e
		}
		sender = &identity.Resend{APIKey: key, From: cfg.Sender}
	}
	if err = os.MkdirAll(filepath.Join(cfg.DataDir, "brains"), 0700); err != nil {
		return nil, err
	}
	db, err := store.Open(filepath.Join(cfg.DataDir, "control.db"))
	if err != nil {
		return nil, err
	}
	provider := &router.OpenAIEmbeddingsProvider{APIKey: embeddingKey, BaseURL: cfg.EmbeddingBaseURL, Model: cfg.EmbeddingModel, Version: cfg.EmbeddingVersion, HTTPClient: &http.Client{Timeout: 15 * time.Second}}
	embedder := &embed.RouterEmbedder{Router: router.New(map[router.Tier]router.Provider{router.TierLocalCheap: provider}, ledger{db}), Pin: provider.ModelVersion()}
	s, err := Assemble(cfg, dev, db, sender, embedder)
	if err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return s, nil
}

// Assemble supplies the same production handlers to integration tests with explicit provider adapters.
func Assemble(cfg Config, dev bool, db *store.Store, sender identity.Sender, embedding embed.Embedder) (*Service, error) {
	p, err := pool.New(pool.Config{MaxOpen: cfg.MaxOpen, MaxInFlight: cfg.MaxInFlight, IdleTimeout: 10 * time.Minute, BrainsRoot: filepath.Join(cfg.DataDir, "brains"), Embedder: embedding})
	if err != nil {
		return nil, err
	}
	issuer := &credential.Issuer{Store: db}
	metering := &meter.Meter{Store: db}
	g := &gateway.Gateway{Issuer: issuer, Pool: p, Meter: metering, Operations: &operation.Ledger{Store: db}}
	if err = g.RecoverDeletions(context.Background(), filepath.Join(cfg.DataDir, "brains")); err != nil {
		return nil, errors.Join(err, p.Close())
	}
	allowlist := make(map[string]struct{}, len(cfg.InviteAllowlist))
	for _, email := range cfg.InviteAllowlist {
		allowlist[email] = struct{}{}
	}
	id := &identity.Service{Store: db, Sender: sender, Origin: cfg.PublicOrigin, AccountCap: cfg.AccountCap, RegistrationMode: cfg.RegistrationMode, InviteAllowlist: allowlist}
	provisioner := &provision.Provisioner{Store: db, BrainsRoot: filepath.Join(cfg.DataDir, "brains")}
	if err = provisioner.Recover(context.Background()); err != nil {
		return nil, errors.Join(err, p.Close())
	}
	dash := &dashboard.Dashboard{Gateway: g, Identity: id, Provision: provisioner, Issuer: issuer, Meter: metering, Origin: cfg.PublicOrigin, Dev: dev, Billing: cfg.BillingEnabled}
	s := &Service{Store: db, Pool: p, Gateway: g, cfg: cfg, embedder: embedding}
	mux := http.NewServeMux()
	mux.Handle("/mcp", g)
	if cfg.billingConfig != nil {
		dash.BillingService = &billing.Service{Store: db, Identity: id, Config: *cfg.billingConfig}
		mux.Handle("/billing/", dash.BillingService)
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("GET /readyz", s.readiness)
	mux.Handle("/", dash.Handler())
	s.Handler = mux
	return s, nil
}
func (s *Service) readiness(w http.ResponseWriter, r *http.Request) {
	s.readyMu.Lock()
	defer s.readyMu.Unlock()
	if time.Since(s.readyAt) > time.Minute {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		s.ready = s.Store.DB().PingContext(ctx) == nil
		if s.ready {
			v, err := s.embedder.Embed(ctx, "Serenity readiness probe")
			s.ready = err == nil && len(v) > 0
		}
		if s.ready {
			f, err := os.CreateTemp(filepath.Join(s.cfg.DataDir, "brains"), ".ready-")
			if err != nil {
				s.ready = false
			} else {
				path := f.Name()
				closeErr := f.Close()
				removeErr := os.Remove(path)
				s.ready = closeErr == nil && removeErr == nil
			}
		}
		s.readyAt = time.Now()
	}
	w.Header().Set("Cache-Control", "no-store")
	if !s.ready {
		http.Error(w, "Not ready", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(200)
}
func (s *Service) Close() error {
	s.Gateway.Close()
	return errors.Join(s.Pool.Close(), s.Store.Close())
}

func (s *Service) Backup(ctx context.Context, destination string) error {
	s.Gateway.Maintenance.Lock()
	defer s.Gateway.Maintenance.Unlock()
	if err := s.Pool.FlushAll(); err != nil {
		return err
	}
	return backup.Create(ctx, s.cfg.DataDir, destination)
}
func (s *Service) AdminHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/backup" {
			http.NotFound(w, r)
			return
		}
		var request struct {
			Destination string `json:"destination"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request); err != nil || !filepath.IsAbs(request.Destination) {
			http.Error(w, "Invalid snapshot destination", http.StatusBadRequest)
			return
		}
		if err := s.Backup(r.Context(), request.Destination); err != nil {
			http.Error(w, "Backup failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
