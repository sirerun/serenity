package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initializeWarmCanonicalGitFixture prepares only a positive runtime fixture.
// The general deletion fixture stays Git-free for cold/refusal controls.
func initializeWarmCanonicalGitFixture(t *testing.T, root string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.name", "Serenity Hosted"},
		{"config", "user.email", "hosted@serenity.sire.run"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("initialize owned canonical Git fixture: %v: %s", err, out)
		}
	}
	f, err := os.OpenFile(filepath.Join(root, ".gitignore"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString(".serenity/\n"); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}
