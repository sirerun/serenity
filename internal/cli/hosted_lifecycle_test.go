package cli

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/service"
)

func TestHostedSnapshotCommandsRequireAdmissionBeforeIO(t *testing.T) {
	for _, action := range []string{"backup", "restore"} {
		t.Run(action, func(t *testing.T) {
			dir := t.TempDir()
			data := filepath.Join(dir, "data")
			snapshot := filepath.Join(dir, "snapshot")
			cmd := newHostedCmd()
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs([]string{action, "--data-dir", data, "--snapshot", snapshot})
			if err := cmd.Execute(); !errors.Is(err, service.ErrStartupUnavailable) {
				t.Fatalf("expected admission unavailable, got %v", err)
			}
			for _, path := range []string{data, snapshot} {
				if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("unadmitted command touched %s: %v", path, err)
				}
			}
		})
	}
}

func TestHostedServeRequiresAdmissionBeforeConfigIO(t *testing.T) {
	config := filepath.Join(t.TempDir(), "missing-config.json")
	cmd := newHostedCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"serve", "--config", config})
	if err := cmd.Execute(); !errors.Is(err, service.ErrStartupUnavailable) {
		t.Fatalf("expected admission unavailable before config read, got %v", err)
	}
	if _, err := os.Lstat(config); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unadmitted serve touched config: %v", err)
	}
}
