package service_test

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/service"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func TestEpisodeServiceDefaultOffAndSyntheticConfig(t *testing.T) {
	cfg := service.Config{Bind: "127.0.0.1:8090", PublicOrigin: "http://127.0.0.1", DataDir: t.TempDir(), SecretsDir: t.TempDir(), EmbeddingModel: "test", EmbeddingVersion: "v1"}
	if err := cfg.Validate(true); err != nil {
		t.Fatal(err)
	}
	if cfg.EpisodeArchiveEnabled {
		t.Fatal("archive defaults on")
	}
	cfg.EpisodeArchiveEnabled = true
	if err := cfg.Validate(true); err == nil || !strings.Contains(err.Error(), "synthetic brain") {
		t.Fatalf("missing allowlist accepted %v", err)
	}
	cfg.EpisodeSyntheticBrains = []string{"../foreign"}
	if err := cfg.Validate(true); err == nil {
		t.Fatal("unsafe allowlist")
	}
	cfg.EpisodeSyntheticBrains = []string{"synthetic-brain"}
	if err := cfg.Validate(true); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			dir := t.TempDir()
			db, err := store.Open(filepath.Join(dir, "control.db"))
			if err != nil {
				t.Fatal(err)
			}
			config := service.Config{DataDir: dir, MaxOpen: 1, MaxInFlight: 1, PublicOrigin: "http://127.0.0.1", AccountCap: 100, EpisodeArchiveEnabled: enabled, EpisodeSyntheticBrains: []string{"synthetic-brain"}}
			svc, err := assembleForTest(t, config, true, db, &sender{}, embedding{})
			if err != nil {
				_ = db.Close()
				t.Fatal(err)
			}
			defer func() { _ = svc.Close() }()
			if svc.Gateway.EpisodeArchiveEnabled != enabled || len(svc.Gateway.EpisodeSyntheticBrains) != 1 {
				t.Fatal("config not wired")
			}
			req := httptest.NewRequest("POST", "/episodes/v1", strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			svc.Handler.ServeHTTP(w, req)
			want := 404
			if enabled {
				want = 401
			}
			if w.Code != want || !strings.Contains(w.Body.String(), "serenity.episodes/v1") {
				t.Fatalf("route %d %s", w.Code, w.Body.String())
			}
		})
	}
}
