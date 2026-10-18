//go:build darwin || linux

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFinalRevokeChecksExactPrivateAndCredentialInventory(t *testing.T) {
	for _, damage := range []string{"none", "unknown_file", "live_credential", "unfinished", "retained_key"} {
		t.Run(damage, func(t *testing.T) {
			e := fixture(t)
			mustOperate(t, e, "prepare")
			m := mustOperate(t, e, "revoke")
			switch damage {
			case "unknown_file":
				if err := os.WriteFile(filepath.Join(e.privateDir, "unrecognized"), []byte("private"), 0600); err != nil {
					t.Fatal(err)
				}
			case "live_credential":
				if _, err := e.store.DB().Exec("UPDATE client_credentials SET revoked_at=NULL WHERE id=?", m.Accounts[0].CredentialIDs[0]); err != nil {
					t.Fatal(err)
				}
			case "unfinished":
				m.Accounts[0].Pending = "revoke"
				save(t, e, m)
			case "retained_key":
				m.Accounts[0].Key = "not-a-qualified-key"
				save(t, e, m)
			}
			err := e.verifyFinalRevoke(context.Background())
			if damage == "none" && err != nil {
				t.Fatal(err)
			}
			if damage != "none" && err == nil {
				t.Fatalf("accepted %s", damage)
			}
		})
	}
}
