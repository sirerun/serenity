package episodeerase

import (
	"bytes"
	"context"
	"errors"
	"github.com/sirerun/serenity/internal/gitrun"
	"github.com/sirerun/serenity/internal/hosted/provision"
	hostedstore "github.com/sirerun/serenity/internal/hosted/store"
	"github.com/sirerun/serenity/internal/store"
	"os"
	"path/filepath"
	"testing"
)

func hostedFixture(t *testing.T) (*fixture, HostedRequest) {
	f := newFixture(t)
	write := func(path, text string) {
		t.Helper()
		full := filepath.Join(f.root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitignore", ".serenity/\n")
	f.commit()
	write("serenity.yml", "version: 1\n")
	write("brain/entities/independent.yaml", "name: independent\n")
	write(".dira/entries/independent.md", "Independent external evidence\n")
	r := appendRecord("delete", "delete-command", "HOSTED-ERASE-SECRET", false)
	f.put(r)
	f.commit()
	write(".serenity/runtime.db", "ignored-runtime-fixture")
	req := HostedRequest{Request: f.request(), IndependentPaths: []PathPin{{".gitignore", rawDigest([]byte(".serenity/\n"))}, {"serenity.yml", rawDigest([]byte("version: 1\n"))}, {"brain/entities/independent.yaml", rawDigest([]byte("name: independent\n"))}, {".dira/entries/independent.md", rawDigest([]byte("Independent external evidence\n"))}}}
	return f, req
}
func TestHostedBaselineAndCanonicalPins(t *testing.T) {
	f, req := hostedFixture(t)
	if _, err := Build(context.Background(), req.Request); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("legacy accepted hosted: %v", err)
	}
	result, err := BuildHosted(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Schema != "serenity.hosted-episode-erasure-candidate/v1" || result.Activation != "not_activated" || result.PhysicalPurge != "unsupported" {
		t.Fatalf("manifest %+v", result)
	}
	runner := gitrun.CanonicalReadOnly(filepath.Join(req.Destination, "repository.git"))
	budget := &budget{}
	objects, err := allObjects(context.Background(), budget, runner)
	if err != nil {
		t.Fatal(err)
	}
	for id, kind := range objects {
		data, err := budget.git(context.Background(), runner, nil, "cat-file", kind, id)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("HOSTED-ERASE-SECRET")) || bytes.Contains(data, []byte("ignored-runtime-fixture")) {
			t.Fatal("excluded/runtime bytes copied")
		}
	}
	// Generic fixture projection helper is source-only; inspect hosted Git directly.
	for _, pin := range req.IndependentPaths {
		got, err := os.ReadFile(filepath.Join(f.root, pin.Path))
		if err != nil {
			t.Fatal(err)
		}
		out := candidateGit(t, req.Request, "show", "main:"+pin.Path)
		if string(out) != string(got) {
			t.Fatalf("changed %s", pin.Path)
		}
	}
	if _, err := os.Stat(filepath.Join(req.Destination, "repository.git", ".serenity")); !os.IsNotExist(err) {
		t.Fatal("runtime copied")
	}
	req.Destination = filepath.Join(f.parent, "retry-hosted")
	for i, j := 0, len(req.IndependentPaths)-1; i < j; i, j = i+1, j-1 {
		req.IndependentPaths[i], req.IndependentPaths[j] = req.IndependentPaths[j], req.IndependentPaths[i]
	}
	retry, err := BuildHosted(context.Background(), req)
	if err != nil || retry.RetryID != result.RetryID || retry.CandidateHead != result.CandidateHead {
		t.Fatalf("retry %v", err)
	}
}
func TestHostedRefusesUnknownAndUnsafeMaterial(t *testing.T) {
	for _, name := range []string{"unlisted", "duplicate", "unused", "drift", "unsafe_path", "runtime_link", "runtime_exec", "ignored_other", "missing_config", "historical_drift", "tracked_exec", "tracked_link", "config_pin_ordinary", "unknown_config"} {
		t.Run(name, func(t *testing.T) {
			f, req := hostedFixture(t)
			switch name {
			case "historical_drift":
				if err := os.WriteFile(filepath.Join(f.root, "serenity.yml"), []byte("changed config\n"), 0600); err != nil {
					t.Fatal(err)
				}
				f.commit()
				req.IndependentPaths[1].SHA256 = rawDigest([]byte("changed config\n"))
			case "tracked_exec":
				if err := os.Chmod(filepath.Join(f.root, "brain/entities/independent.yaml"), 0700); err != nil {
					t.Fatal(err)
				}
				f.commit()
			case "tracked_link":
				if err := os.Symlink("independent.yaml", filepath.Join(f.root, "brain/entities/link")); err != nil {
					t.Fatal(err)
				}
				f.commit()
			case "config_pin_ordinary":
				req.IndependentPaths = append(req.IndependentPaths, PathPin{".serenity/config", rawDigest(nil)})
			case "unknown_config":
				path := filepath.Join(f.root, ".git/config")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, append(data, []byte("\n[core]\n hooksPath = /tmp/unsafe\n")...), 0600); err != nil {
					t.Fatal(err)
				}
			case "unlisted":
				req.IndependentPaths = req.IndependentPaths[:3]
			case "duplicate":
				req.IndependentPaths = append(req.IndependentPaths, req.IndependentPaths[0])
			case "unused":
				req.IndependentPaths = append(req.IndependentPaths, PathPin{"brain/claims/unused", rawDigest([]byte("unused"))})
			case "drift":
				req.IndependentPaths[1].SHA256 = rawDigest([]byte("different"))
			case "unsafe_path":
				req.IndependentPaths = append(req.IndependentPaths, PathPin{"brain/claims/../escape", rawDigest(nil)})
			case "runtime_link":
				if err := os.Symlink("runtime.db", filepath.Join(f.root, ".serenity/link")); err != nil {
					t.Fatal(err)
				}
			case "runtime_exec":
				if err := os.Chmod(filepath.Join(f.root, ".serenity/runtime.db"), 0700); err != nil {
					t.Fatal(err)
				}
			case "ignored_other":
				if err := os.WriteFile(filepath.Join(f.root, "untracked"), []byte("x"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing_config":
				if err := os.Remove(filepath.Join(f.root, "serenity.yml")); err != nil {
					t.Fatal(err)
				}
				f.commit()
			}
			if _, err := BuildHosted(context.Background(), req); err == nil {
				t.Fatal("unsafe accepted")
			}
			if _, err := os.Stat(req.Destination); !os.IsNotExist(err) {
				t.Fatal("failure published")
			}
		})
	}
}

func candidateGit(t *testing.T, req Request, args ...string) []byte {
	t.Helper()
	out, err := gitrun.CanonicalReadOnly(filepath.Join(req.Destination, "repository.git")).Output(context.Background(), args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestHostedActualProvisionedBaseline(t *testing.T) {
	ctx := context.Background()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	control, err := hostedstore.Open(filepath.Join(parent, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := control.Close(); err != nil {
			t.Error(err)
		}
	}()
	account, err := control.CreateAccount(ctx, "synthetic@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	p := &provision.Provisioner{Store: control, BrainsRoot: filepath.Join(parent, "brains")}
	brain, err := p.Provision(ctx, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(p.BrainsRoot, brain.PathKey)
	f := &fixture{t: t, root: root, parent: parent, s: store.NewSourceStore(root)}
	config := []byte("version: 1\n")
	if err := os.WriteFile(filepath.Join(root, "serenity.yml"), config, 0600); err != nil {
		t.Fatal(err)
	}
	rec := appendRecord("delete", "delete-command", "PROVISIONED-SECRET", false)
	rec.BrainID = brain.ID
	f.put(rec)
	f.commit()
	req := HostedRequest{Request: f.request(), IndependentPaths: []PathPin{{".gitignore", rawDigest([]byte(".serenity/\n"))}, {"serenity.yml", rawDigest(config)}}}
	req.BrainID = brain.ID
	req.CurrentFences[0].BrainID = brain.ID
	result, err := BuildHosted(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Commits != 2 || result.Activation != "not_activated" {
		t.Fatalf("unexpected result %+v", result)
	}
	if string(candidateGit(t, req.Request, "show", "main~1:.gitignore")) != ".serenity/\n" {
		t.Fatal("provisioning baseline changed")
	}
}
