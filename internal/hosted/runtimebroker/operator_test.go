package runtimebroker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func ownerFiles(t *testing.T) (string, string) {
	t.Helper()
	parent := os.Getenv("SERENITY_RECOVERY_TEST_TMPDIR")
	if parent == "" {
		parent = t.TempDir()
	}
	dir, err := os.MkdirTemp(parent, "owner-config-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("cleanup owner fixture: %v", err)
		}
	})
	credentials := filepath.Join(dir, "credentials")
	if err := os.WriteFile(credentials, []byte("[owner]\naws_access_key_id=fixture\naws_secret_access_key=fixture-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "owner.json")
	data, err := json.Marshal(operatorConfig{Endpoint: "https://0123456789.execute-api.us-west-2.amazonaws.com/prod/owner", Region: "us-west-2", Profile: "owner", CredentialsFile: credentials})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, data, 0600); err != nil {
		t.Fatal(err)
	}
	return config, credentials
}

func TestOwnerCurrentUIDPrivateFiles(t *testing.T) {
	config, _ := ownerFiles(t)
	client, closeClient, err := OpenOwner(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer closeClient()
	if client == nil {
		t.Fatal("owner client missing")
	}
	if os.Geteuid() != 0 {
		if _, _, err := OpenOperator(context.Background(), config); err == nil {
			t.Fatal("non-root restore operator accepted")
		}
	}
}

func TestOpenClosedMaintenanceUsesAuthenticatedOwnerFactory(t *testing.T) {
	config, _ := ownerFiles(t)
	source, closeSource, err := OpenClosedMaintenance(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer closeSource()
	if source == nil || closeSource == nil {
		t.Fatal("trusted CLOSED maintenance source was not constructed")
	}
}

func TestOwnerRejectsUnsafeFiles(t *testing.T) {
	for _, target := range []string{"config", "credentials"} {
		for _, hazard := range []string{"mode", "symlink", "directory"} {
			t.Run(target+"/"+hazard, func(t *testing.T) {
				config, credentials := ownerFiles(t)
				path := config
				if target == "credentials" {
					path = credentials
				}
				switch hazard {
				case "mode":
					if err := os.Chmod(path, 0644); err != nil {
						t.Fatal(err)
					}
				case "directory":
					if err := os.Chmod(filepath.Dir(path), 0755); err != nil {
						t.Fatal(err)
					}
				case "symlink":
					saved := path + ".saved"
					if err := os.Rename(path, saved); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(saved, path); err != nil {
						t.Fatal(err)
					}
				}
				if _, _, err := OpenOwner(context.Background(), config); err == nil {
					t.Fatal("unsafe owner file accepted")
				}
			})
		}
	}
}

func TestOwnerRejectsForeignUID(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("ownership mutation requires root")
	}
	for _, target := range []string{"config", "credentials"} {
		t.Run(target, func(t *testing.T) {
			config, credentials := ownerFiles(t)
			path := config
			if target == "credentials" {
				path = credentials
			}
			if err := os.Chown(path, 65534, -1); err != nil {
				t.Fatal(err)
			}
			if _, _, err := OpenOwner(context.Background(), config); err == nil {
				t.Fatal("foreign owner accepted")
			}
		})
	}
}
