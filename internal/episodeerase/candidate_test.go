package episodeerase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/gitrun"
	"github.com/sirerun/serenity/internal/store"
)

var testTime = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

type fixture struct {
	t      *testing.T
	root   string
	parent string
	s      *store.SourceStore
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "source")
	if err = os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	template := filepath.Join(parent, "template")
	if err = os.Mkdir(template, 0700); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, root: root, parent: parent, s: store.NewSourceStore(root)}
	f.git("init", "--initial-branch=main", "--object-format=sha1", "--template="+template)
	return f
}
func (f *fixture) git(args ...string) []byte {
	f.t.Helper()
	out, e := gitrun.Quarantine(f.root).Output(context.Background(), args...)
	if e != nil {
		f.t.Fatalf("git %s: %v", args[0], e)
	}
	return out
}
func (f *fixture) commit() string {
	f.t.Helper()
	f.git("add", "--all")
	tree := strings.TrimSpace(string(f.git("write-tree")))
	return f.commitTree(tree)
}

// Publish a fixture tree without checking it out or changing the clean index.
func (f *fixture) commitTree(tree string) string {
	f.t.Helper()
	raw := "tree " + tree + "\n"
	old, e := gitrun.CanonicalReadOnly(f.root).Output(context.Background(), "rev-parse", "--verify", "HEAD")
	if e == nil {
		raw += "parent " + strings.TrimSpace(string(old)) + "\n"
	}
	raw += "author Fixture <fixture@invalid> 946684800 +0000\ncommitter Fixture <fixture@invalid> 946684800 +0000\n\nsource metadata MAY-CONTAIN-DELETED-TEXT\n"
	id, e := (&budget{}).git(context.Background(), gitrun.Quarantine(f.root), []byte(raw), "hash-object", "-t", "commit", "-w", "--stdin")
	if e != nil {
		f.t.Fatal(e)
	}
	head := strings.TrimSpace(string(id))
	f.git("update-ref", "refs/heads/main", head)
	return head
}
func (f *fixture) put(r store.EpisodeRecord) string {
	f.t.Helper()
	src, e := f.s.PutEpisode(r)
	if e != nil {
		f.t.Fatal(e)
	}
	return src.SHA256
}
func appendRecord(conversation, command, text string, attachment bool) store.EpisodeRecord {
	ev := store.EpisodeEvent{ID: "event", Sequence: 0, Role: "user", Type: "message", OccurredAt: testTime, ObservedAt: testTime, Attribution: "synthetic", Text: text}
	if attachment {
		data := []byte("independently shared attachment")
		h := sha256.Sum256(data)
		ev.Attachments = []store.EpisodeAttachment{{ID: "attachment", MediaType: "text/plain", Data: data, SHA256: hex.EncodeToString(h[:])}}
	}
	in := store.EpisodeInput{Operation: "append", ProducerID: "producer", ConversationID: conversation, CommandID: command, Host: "fixture", PolicyVersion: "v1", Events: []store.EpisodeEvent{ev}}
	return store.EpisodeRecord{Schema: store.EpisodeSchema, BrainID: "brain", Kind: "append", CommandKey: store.EpisodeCommandKey(in.ProducerID, command), ConversationKey: store.EpisodeConversationKey(in.ProducerID, conversation), RequestDigest: store.EpisodeHash(in), RecordedAt: testTime, Input: &in, Refs: []store.EpisodeRef{{EventKey: store.EpisodeEventKey(in.ProducerID, conversation, ev.ID), Digest: store.EpisodeHash(ev)}}}
}
func checkpointRecord(source store.EpisodeRecord) store.EpisodeRecord {
	in := store.EpisodeInput{Operation: "checkpoint", ProducerID: "producer", ConversationID: "checkpoint-conversation", CommandID: "checkpoint-command", Host: "fixture", PolicyVersion: "v1", Checkpoint: &store.EpisodeCheckpoint{Text: "DERIVED-SECRET", Sources: source.Refs, Model: "fixture", CreatedAt: testTime}}
	return store.EpisodeRecord{Schema: store.EpisodeSchema, BrainID: "brain", Kind: "checkpoint", CommandKey: store.EpisodeCommandKey(in.ProducerID, in.CommandID), ConversationKey: store.EpisodeConversationKey(in.ProducerID, in.ConversationID), RequestDigest: store.EpisodeHash(in), RecordedAt: testTime, Input: &in}
}
func fence(conversation string, generation uint64) store.EpisodeRecord {
	in := store.EpisodeInput{Operation: "exclude", ProducerID: "producer", ConversationID: conversation, CommandID: "exclude-" + conversation}
	return store.EpisodeRecord{Schema: store.EpisodeSchema, BrainID: "brain", Kind: "exclude", CommandKey: store.EpisodeCommandKey(in.ProducerID, in.CommandID), ConversationKey: store.EpisodeConversationKey(in.ProducerID, conversation), RequestDigest: store.EpisodeHash(in), Generation: generation, RecordedAt: testTime}
}
func (f *fixture) request() Request {
	return Request{Source: f.root, Destination: filepath.Join(f.parent, "candidate"), BrainID: "brain", CurrentFences: []store.EpisodeRecord{fence("delete", 1)}}
}
func mustBuild(t *testing.T, req Request) Result {
	t.Helper()
	r, e := Build(context.Background(), req)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func candidateFiles(t *testing.T, req Request, revision string) map[string][]byte {
	t.Helper()
	files, e := snapshot(context.Background(), &budget{}, gitrun.CanonicalReadOnly(filepath.Join(req.Destination, "repository.git")), revision)
	if e != nil {
		t.Fatal(e)
	}
	return files
}
func projection(t *testing.T, files map[string][]byte) *store.EpisodeProjection {
	t.Helper()
	root := t.TempDir()
	if e := materialize(root, files); e != nil {
		t.Fatal(e)
	}
	p, e := store.LoadEpisodes(context.Background(), store.NewSourceStore(root))
	if e != nil {
		t.Fatal(e)
	}
	return p
}

func TestCandidateErasesHistoryAndDependentCheckpointsPreservesSiblings(t *testing.T) {
	f := newFixture(t)
	deleted := appendRecord("delete", "deleted-command", "UNIQUE-ORIGINAL-SECRET", true)
	deletedSHA := f.put(deleted)
	first := f.commit()
	sibling := appendRecord("sibling", "sibling-command", "sibling exact text", true)
	siblingSHA := f.put(sibling)
	checkpointSHA := f.put(checkpointRecord(deleted))
	head := f.commit()
	original, _ := os.ReadFile(filepath.Join(f.s.DirFor(siblingSHA), "bytes"))
	req := f.request()
	r := mustBuild(t, req)
	if r.SourceHead != head || r.Commits != 2 || r.Activation != "not_activated" || r.PhysicalPurge != "unsupported" {
		t.Fatalf("bad result: %+v", r)
	}
	if len(r.RemovedSources) != 2 || !contains(r.RemovedSources, deletedSHA) || !contains(r.RemovedSources, checkpointSHA) {
		t.Fatalf("closure: %v", r.RemovedSources)
	}
	if len(r.RetainedAttachmentDigests) != 1 || r.RetainedAttachmentDigests[0] != sibling.Input.Events[0].Attachments[0].SHA256 {
		t.Fatal("shared attachment disposition missing")
	}
	files := candidateFiles(t, req, "refs/heads/main")
	if !bytes.Equal(files[sourceBase(siblingSHA)+"bytes"], original) {
		t.Fatal("sibling changed")
	}
	p := projection(t, files)
	if len(p.Events) != 1 || len(p.Records) != 2 || p.Fences[deleted.ConversationKey] != 1 {
		t.Fatalf("projection: %+v", p)
	}
	// Snapshot-before-delete has no local exclusion. Explicit current input is
	// inserted even into the candidate's oldest revision, preventing restore replay.
	old := projection(t, candidateFiles(t, req, "refs/heads/main~1"))
	if len(old.Events) != 0 || old.Generation != 1 || old.Fences[deleted.ConversationKey] != 1 {
		t.Fatal("old snapshot bypasses current fence")
	}
	if string(f.git("rev-parse", "HEAD")) != head+"\n" {
		t.Fatal("source changed")
	}
	if _, e := os.Stat(filepath.Join(f.s.DirFor(deletedSHA), "bytes")); e != nil {
		t.Fatal("source was destructively changed")
	}
	if len(f.git("show", first+":"+sourceBase(deletedSHA)+"bytes")) == 0 {
		t.Fatal("source historical bytes changed")
	}
	// Inspect ALL candidate objects, including unreachable ones (which are also
	// forbidden), rather than only the worktree or current branch.
	runner := gitrun.CanonicalReadOnly(filepath.Join(req.Destination, "repository.git"))
	b := &budget{}
	objects, e := allObjects(context.Background(), b, runner)
	if e != nil {
		t.Fatal(e)
	}
	for id, kind := range objects {
		raw, e := b.git(context.Background(), runner, nil, "cat-file", kind, id)
		if e != nil {
			t.Fatal(e)
		}
		for _, secret := range []string{"UNIQUE-ORIGINAL-SECRET", "DERIVED-SECRET", "MAY-CONTAIN-DELETED-TEXT"} {
			if bytes.Contains(raw, []byte(secret)) {
				t.Fatalf("old bytes in %s %s", kind, id)
			}
		}
	}
	if _, ok := objects[blobID(mustJSON(t, deleted))]; ok {
		t.Fatal("deleted source blob survived")
	}
	manifest, e := os.ReadFile(filepath.Join(req.Destination, "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	for _, secret := range []string{"UNIQUE-ORIGINAL-SECRET", "DERIVED-SECRET", f.root, "deleted-command"} {
		if bytes.Contains(manifest, []byte(secret)) {
			t.Fatal("manifest retained content or path")
		}
	}
}
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func TestHistoricalOnlyOriginalIsRemoved(t *testing.T) {
	f := newFixture(t)
	sha := f.put(appendRecord("delete", "command", "HISTORICAL-ONLY", false))
	f.commit()
	if e := os.RemoveAll(f.s.DirFor(sha)); e != nil {
		t.Fatal(e)
	}
	f.commit()
	req := f.request()
	r := mustBuild(t, req)
	if !contains(r.RemovedSources, sha) {
		t.Fatal("missed historical source")
	}
	for _, rev := range []string{"main", "main~1"} {
		p := projection(t, candidateFiles(t, req, rev))
		if len(p.Events) != 0 || p.Generation != 1 {
			t.Fatal("historical resurrection")
		}
	}
}

func TestRetryIsDeterministicAndNeverAdoptsExistingState(t *testing.T) {
	f := newFixture(t)
	f.put(appendRecord("delete", "command", "secret", false))
	f.commit()
	req := f.request()
	// An interrupted earlier process may leave an inert partial stage. A retry
	// neither adopts its unverified content nor deletes another operation's files.
	abandoned := filepath.Join(f.parent, ".episode-erase-stage-abandoned")
	if e := os.Mkdir(abandoned, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(abandoned, "partial"), []byte("untrusted partial"), 0600); e != nil {
		t.Fatal(e)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := Build(cancelled, req); !errors.Is(e, context.Canceled) {
		t.Fatalf("cancellation: %v", e)
	}
	if _, e := os.Stat(req.Destination); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("cancelled candidate published")
	}
	one := mustBuild(t, req)
	if _, e := Build(context.Background(), req); !errors.Is(e, ErrUnsafe) {
		t.Fatalf("existing destination accepted: %v", e)
	}
	req.Destination = filepath.Join(f.parent, "retry")
	two := mustBuild(t, req)
	if one.RetryID != two.RetryID || one.CandidateHead != two.CandidateHead {
		t.Fatal("retry changed identity or candidate")
	}
	if _, e := os.Stat(filepath.Join(abandoned, "partial")); e != nil {
		t.Fatal("foreign stage removed")
	}
}

func TestCurrentFenceRequiredAndCannotRollbackSourceFence(t *testing.T) {
	for _, test := range []string{"missing", "foreign-brain", "gap", "altered", "stale"} {
		t.Run(test, func(t *testing.T) {
			f := newFixture(t)
			f.put(appendRecord("delete", "command", "secret", false))
			f.put(fence("delete", 1))
			f.commit()
			req := f.request()
			switch test {
			case "missing":
				req.CurrentFences = nil
			case "foreign-brain":
				req.BrainID = "foreign"
			case "gap":
				req.CurrentFences[0].Generation = 2
			case "altered":
				req.CurrentFences[0].RecordedAt = testTime.Add(time.Second)
			case "stale":
				f.put(fence("other", 2))
				f.commit()
			}
			if _, e := Build(context.Background(), req); !errors.Is(e, ErrFence) {
				t.Fatalf("wanted fence rejection: %v", e)
			}
			if _, e := os.Stat(req.Destination); !errors.Is(e, os.ErrNotExist) {
				t.Fatal("invalid candidate published")
			}
		})
	}
}

func TestAllCurrentFencesApplyAndDeduplicatedReceiptsDisappear(t *testing.T) {
	f := newFixture(t)
	r := appendRecord("delete", "original", "secret", false)
	f.put(r)
	copied := *r.Input
	copied.CommandID = "reimport"
	r.CommandKey = store.EpisodeCommandKey(copied.ProducerID, copied.CommandID)
	r.RequestDigest = store.EpisodeHash(copied)
	copied.Events = nil
	r.Input = &copied
	f.put(r)
	f.put(appendRecord("also-delete", "second", "second secret", false))
	f.commit()
	req := f.request()
	req.CurrentFences = append(req.CurrentFences, fence("also-delete", 2))
	result := mustBuild(t, req)
	p := projection(t, candidateFiles(t, req, "main"))
	if len(result.RemovedSources) != 3 || len(p.Events) != 0 || len(p.Records) != 2 || p.Generation != 2 {
		t.Fatal("incomplete multi-fence closure")
	}
}

func TestUnsafeRepositoriesFailClosed(t *testing.T) {
	cases := map[string]func(*fixture){
		"dirty": func(f *fixture) { f.put(appendRecord("other", "dirty", "dirty", false)) },
		"unknown-derivative": func(f *fixture) {
			mustWrite(f.t, filepath.Join(f.root, "derived.db"), []byte("derived secret"))
			f.commit()
		},
		"ignored-derivative": func(f *fixture) {
			mustWrite(f.t, filepath.Join(f.root, ".git", "info", "exclude"), []byte("cache.db\n"))
			mustWrite(f.t, filepath.Join(f.root, "cache.db"), []byte("stale cache"))
		},
		"extra-ref": func(f *fixture) { f.git("branch", "other") },
		"tag":       func(f *fixture) { f.git("tag", "snapshot") },
		"replace-ref": func(f *fixture) {
			id := strings.TrimSpace(string(f.git("rev-parse", "HEAD")))
			f.git("update-ref", "refs/replace/"+id, id)
		},
		"alternates": func(f *fixture) {
			mustWrite(f.t, filepath.Join(f.root, ".git", "objects", "info", "alternates"), []byte("/not-read\n"))
		},
		"shallow": func(f *fixture) { mustWrite(f.t, filepath.Join(f.root, ".git", "shallow"), f.git("rev-parse", "HEAD")) },
		"config-include": func(f *fixture) {
			file := filepath.Join(f.root, ".git", "config")
			raw, e := os.ReadFile(file)
			if e != nil {
				f.t.Fatal(e)
			}
			mustWrite(f.t, file, append(raw, []byte("[include]\npath = /not-read\n")...))
		},
		"foreign-brain": func(f *fixture) {
			r := appendRecord("foreign", "foreign", "foreign", false)
			r.BrainID = "foreign-brain"
			f.put(r)
			f.commit()
		},
		"symlink": func(f *fixture) {
			if e := os.Symlink("/not-read", filepath.Join(f.root, "link")); e != nil {
				f.t.Fatal(e)
			}
		},
		"git-symlink": func(f *fixture) {
			if e := os.Symlink("/not-read", filepath.Join(f.root, ".git", "suspicious")); e != nil {
				f.t.Fatal(e)
			}
		},
		"unreachable-object": func(f *fixture) {
			cmd, e := gitrun.Quarantine(f.root).Command(context.Background(), "hash-object", "-w", "--stdin")
			if e != nil {
				f.t.Fatal(e)
			}
			cmd.Stdin = strings.NewReader("unreachable secret")
			if e = cmd.Run(); e != nil {
				f.t.Fatal(e)
			}
		},
		"malformed-record": func(f *fixture) {
			sha := strings.Repeat("a", 64)
			base := filepath.Join(f.root, filepath.FromSlash(sourceBase(sha)))
			mustWrite(f.t, filepath.Join(base, "bytes"), []byte("invalid JSON"))
			mustWrite(f.t, filepath.Join(base, "meta.yaml"), []byte("kind: memory_episode\nuri: memory://episode\noccurred_at: 2026-10-16T12:00:00Z\n"))
			f.commit()
		},
		"gitlink": func(f *fixture) {
			head := strings.TrimSpace(string(f.git("rev-parse", "HEAD")))
			f.git("update-index", "--add", "--cacheinfo", "160000,"+head+",submodule")
			cmd, e := gitrun.Quarantine(f.root).Command(context.Background(), "write-tree")
			if e != nil {
				f.t.Fatal(e)
			}
			if _, e = cmd.Output(); e != nil {
				f.t.Fatal(e)
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.put(appendRecord("delete", "command", "secret", false))
			f.commit()
			mutate(f)
			req := f.request()
			if _, e := Build(context.Background(), req); e == nil {
				t.Fatal("unsafe repository accepted")
			}
			if _, e := os.Stat(req.Destination); !errors.Is(e, os.ErrNotExist) {
				t.Fatal("candidate published on failure")
			}
		})
	}
}
func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, data, 0600); e != nil {
		t.Fatal(e)
	}
}

func TestHistoricalUnknownDerivativeRefusedEvenIfHeadClean(t *testing.T) {
	f := newFixture(t)
	f.put(appendRecord("delete", "command", "secret", false))
	mustWrite(t, filepath.Join(f.root, "summary.txt"), []byte("derived old secret"))
	f.commit()
	if e := os.Remove(filepath.Join(f.root, "summary.txt")); e != nil {
		t.Fatal(e)
	}
	f.commit()
	if _, e := Build(context.Background(), f.request()); !errors.Is(e, ErrUnsafe) {
		t.Fatalf("historical derivative accepted: %v", e)
	}
}

func TestSymlinkedRootAndDestinationParentRefused(t *testing.T) {
	f := newFixture(t)
	f.put(appendRecord("delete", "command", "secret", false))
	f.commit()
	link := filepath.Join(f.parent, "linked")
	if e := os.Symlink(f.root, link); e != nil {
		t.Fatal(e)
	}
	req := f.request()
	req.Source = link
	if _, e := Build(context.Background(), req); !errors.Is(e, ErrUnsafe) {
		t.Fatalf("symlink source: %v", e)
	}
	req = f.request()
	req.Destination = filepath.Join(link, "candidate")
	if _, e := Build(context.Background(), req); !errors.Is(e, ErrUnsafe) {
		t.Fatalf("symlink destination: %v", e)
	}
}

func TestResponseAndCommitLimits(t *testing.T) {
	var buf limitedBuffer
	buf.limit = 3
	if _, e := buf.Write([]byte("four")); !errors.Is(e, ErrLimit) {
		t.Fatal("response bound missing")
	}
	// os/exec copies pipes through io.Copy. ReaderFrom promotion must never
	// bypass the cap, even when the reader has no WriteTo fast path.
	if _, e := io.Copy(&buf, &io.LimitedReader{R: strings.NewReader("four"), N: 4}); !errors.Is(e, ErrLimit) {
		t.Fatal("streaming response bound bypassed")
	}
	f := newFixture(t)
	f.put(appendRecord("delete", "command", "secret", false))
	for i := 0; i <= maxCommits; i++ {
		f.commit()
	}
	if _, e := Build(context.Background(), f.request()); !errors.Is(e, ErrLimit) {
		t.Fatalf("history bound: %v", e)
	}
}

func TestUnknownCanonicalFieldsRejected(t *testing.T) {
	for _, kind := range []string{"json", "metadata"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			r := appendRecord("sibling", "command", "survivor", false)
			sha := f.put(r)
			if kind == "metadata" {
				path := filepath.Join(f.s.DirFor(sha), "meta.yaml")
				raw, e := os.ReadFile(path)
				if e != nil {
					t.Fatal(e)
				}
				mustWrite(t, path, append(raw, []byte("unknown_derived_text: hidden derivative\n")...))
			} else {
				raw := mustJSON(t, r)
				raw = append(raw[:len(raw)-1], []byte(",\"unknown_derived_text\":\"hidden derivative\"}")...)
				sum := sha256.Sum256(raw)
				newSHA := hex.EncodeToString(sum[:])
				meta, e := os.ReadFile(filepath.Join(f.s.DirFor(sha), "meta.yaml"))
				if e != nil {
					t.Fatal(e)
				}
				if e = os.RemoveAll(f.s.DirFor(sha)); e != nil {
					t.Fatal(e)
				}
				mustWrite(t, filepath.Join(f.s.DirFor(newSHA), "bytes"), raw)
				mustWrite(t, filepath.Join(f.s.DirFor(newSHA), "meta.yaml"), meta)
			}
			f.commit()
			if _, e := Build(context.Background(), f.request()); !errors.Is(e, ErrUnsafe) {
				t.Fatalf("unknown fields accepted: %v", e)
			}
		})
	}
}

// Cancel after candidate staging has actually started. Err is queried at every
// bounded operation; no timing or arbitrary sleeps are needed in this fixture.
type stageCancelContext struct {
	context.Context
	parent string
	cancel context.CancelFunc
}

func (c *stageCancelContext) Err() error {
	entries, _ := os.ReadDir(c.parent)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".episode-erase-stage-") {
			c.cancel()
			break
		}
	}
	return c.Context.Err()
}
func TestInterruptedStageIsRemovedAndRetrySucceeds(t *testing.T) {
	f := newFixture(t)
	f.put(appendRecord("delete", "command", "secret", false))
	f.commit()
	req := f.request()
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &stageCancelContext{Context: base, parent: f.parent, cancel: cancel}
	if _, e := Build(ctx, req); !errors.Is(e, context.Canceled) {
		t.Fatalf("interruption: %v", e)
	}
	entries, e := os.ReadDir(f.parent)
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".episode-erase-stage-") {
			t.Fatal("owned interrupted stage remains")
		}
	}
	if _, e = os.Stat(req.Destination); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("interrupted operation activated candidate")
	}
	mustBuild(t, req)
}

func (f *fixture) gitInput(input []byte, args ...string) string {
	f.t.Helper()
	out, err := (&budget{}).git(context.Background(), gitrun.Quarantine(f.root), input, args...)
	if err != nil {
		f.t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

// Include Git files as well as canonical sources: a rejected build must leave
// refs, objects, reflogs, index and working bytes untouched.
func sourceFileDigests(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	result := map[string][32]byte{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[path] = sha256.Sum256(raw)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func assertHistoricalRejection(t *testing.T, f *fixture, malformed string) {
	t.Helper()
	req := f.request()
	before := sourceFileDigests(t, f.root)
	// Prove clean HEAD/reachability qualification is not what rejects the case.
	if _, err := qualifyGit(context.Background(), &budget{}, gitrun.CanonicalReadOnly(f.root), f.root); err != nil {
		t.Fatalf("fixture failed initial qualification: %v", err)
	}
	if _, err := snapshot(context.Background(), &budget{}, gitrun.CanonicalReadOnly(f.root), malformed); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("malformed historical snapshot: %v", err)
	}
	if _, err := Build(context.Background(), req); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("historical build: %v", err)
	}
	if _, err := os.Lstat(req.Destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination published: %v", err)
	}
	after := sourceFileDigests(t, f.root)
	if len(before) != len(after) {
		t.Fatal("source file set changed")
	}
	for path, digest := range before {
		if got, ok := after[path]; !ok || got != digest {
			t.Fatalf("source changed: %s", path)
		}
	}
}

func TestDuplicateHistoricalPathsRejected(t *testing.T) {
	for _, name := range []string{"bytes", "meta.yaml"} {
		for _, identical := range []bool{false, true} {
			variant := "different"
			if identical {
				variant = "identical"
			}
			t.Run(name+"/"+variant, func(t *testing.T) {
				f := newFixture(t)
				sha := f.put(appendRecord("delete", "command", "synthetic secret", false))
				clean := f.commit()
				cleanTree := strings.TrimSpace(string(f.git("rev-parse", clean+"^{tree}")))
				ids := map[string]string{}
				for _, file := range []string{"bytes", "meta.yaml"} {
					ids[file] = strings.TrimSpace(string(f.git("rev-parse", clean+":"+sourceBase(sha)+file)))
				}
				hidden := ids[name]
				if !identical {
					hidden = f.gitInput([]byte("unknown historical payload"), "hash-object", "-w", "--stdin")
				}
				// Raw tree encoding deliberately preserves a duplicate entry. Ordinary
				// index/write-tree construction cannot represent this malformed shape.
				var leaf []byte
				add := func(file, id string) {
					oid, err := hex.DecodeString(id)
					if err != nil {
						t.Fatal(err)
					}
					leaf = append(leaf, []byte("100644 "+file+"\x00")...)
					leaf = append(leaf, oid...)
				}
				for _, file := range []string{"bytes", "meta.yaml"} {
					if file == name {
						add(file, hidden)
					}
					add(file, ids[file]) // Canonical entry last: reproduces the overwrite.
				}
				tree := f.gitInput(leaf, "hash-object", "--literally", "-t", "tree", "-w", "--stdin")
				for _, dir := range []string{sha, sha[:2], "sources", "brain"} {
					tree = f.gitInput([]byte("040000 tree "+tree+"\t"+dir+"\x00"), "mktree", "-z")
				}
				malformed := f.commitTree(tree)
				f.commitTree(cleanTree) // Restore clean ordinary HEAD without checkout.
				listing := f.git("ls-tree", "-rz", "--full-tree", malformed)
				repeated := []byte("\t" + sourceBase(sha) + name + "\x00")
				if bytes.Count(listing, repeated) != 2 {
					t.Fatal("fixture did not retain both full paths")
				}
				if status := f.git("status", "--porcelain=v1", "--untracked-files=all", "--ignored"); len(status) != 0 {
					t.Fatal("fixture HEAD is not clean")
				}
				assertHistoricalRejection(t, f, malformed)
			})
		}
	}
}

func TestHistoricalGitlinkRejectedWithCleanHead(t *testing.T) {
	f := newFixture(t)
	f.put(appendRecord("delete", "command", "synthetic secret", false))
	clean := f.commit()
	cleanTree := strings.TrimSpace(string(f.git("rev-parse", clean+"^{tree}")))
	entries := f.git("ls-tree", "-z", clean)
	entries = append(entries, []byte("160000 commit "+clean+"\tsubmodule\x00")...)
	tree := f.gitInput(entries, "mktree", "-z")
	malformed := f.commitTree(tree)
	f.commitTree(cleanTree)
	if status := f.git("status", "--porcelain=v1", "--untracked-files=all", "--ignored"); len(status) != 0 {
		t.Fatal("fixture HEAD is not clean")
	}
	assertHistoricalRejection(t, f, malformed)
}
