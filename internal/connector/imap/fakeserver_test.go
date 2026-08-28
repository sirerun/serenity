package imap_test

// Test-only in-process fake IMAP server (RFC 0001 §10.1 T1.4 acc: "an
// in-process fake IMAP server"). Built entirely from the same
// github.com/emersion/go-imap/v2 dependency ADR 003 already approves for
// this connector -- imapserver/imapmemserver is its in-memory backend, and
// imapserver.New speaks the real wire protocol over a loopback TCP
// listener, so tests exercise the real client/server IMAP round trip with
// no network access and no real Gmail account.

import (
	"net"
	"os"
	"path/filepath"
	"sort"
	"testing"

	imapapi "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
)

const (
	testAccount  = "alice@example.com"
	testPassword = "an-app-password"
	testMailbox  = "INBOX"
)

// fakeServer is a running in-process IMAP server plus the memstore handle
// tests use to mutate mailbox state directly (append fixtures, force a
// UIDVALIDITY change, expunge a message out from under a poll in flight).
type fakeServer struct {
	addr string
	user *imapmemserver.User
}

// startFakeServer boots an in-memory IMAP server on loopback with one user
// and an empty INBOX, and stops it when the test ends.
func startFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	return startFakeServerWithSessionWrap(t, nil)
}

// startFakeServerWithSessionWrap is startFakeServer, but every new session
// the memstore backend creates is passed through wrap first. Tests that
// need to interpose on a specific IMAP command (e.g. to expunge a message
// deterministically between SEARCH and the connector's next FETCH) supply
// a wrap that overrides that one method and embeds the rest.
func startFakeServerWithSessionWrap(t *testing.T, wrap func(imapserver.Session) imapserver.Session) *fakeServer {
	t.Helper()

	mem := imapmemserver.New()
	user := imapmemserver.NewUser(testAccount, testPassword)
	if err := user.Create(testMailbox, nil); err != nil {
		t.Fatalf("create INBOX: %v", err)
	}
	mem.AddUser(user)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			sess := mem.NewSession()
			if wrap != nil {
				sess = wrap(sess)
			}
			return sess, nil, nil
		},
		Caps: imapapi.CapSet{
			imapapi.CapIMAP4rev1: {},
			imapapi.CapIMAP4rev2: {},
		},
		InsecureAuth: true, // loopback test server: no TLS certificate to present
	})
	t.Cleanup(func() { _ = srv.Close() })
	go func() { _ = srv.Serve(ln) }()

	return &fakeServer{addr: ln.Addr().String(), user: user}
}

// dial returns a Connector.Dial-shaped closure authenticating as account
// against this fake server in cleartext (the test analogue of
// NewGmail's DialTLS + Login).
func (f *fakeServer) dial(account, password string) func() (*imapclient.Client, error) {
	return func() (*imapclient.Client, error) {
		client, err := imapclient.DialInsecure(f.addr, &imapclient.Options{})
		if err != nil {
			return nil, err
		}
		if err := client.Login(account, password).Wait(); err != nil {
			_ = client.Close()
			return nil, err
		}
		return client, nil
	}
}

// rawClient opens a second, independent authenticated connection -- tests
// use this to mutate mailbox state (expunge, recreate) concurrently with
// or ahead of the connector's own Poll connection.
func (f *fakeServer) rawClient(t *testing.T) *imapclient.Client {
	t.Helper()
	client, err := imapclient.DialInsecure(f.addr, &imapclient.Options{})
	if err != nil {
		t.Fatalf("dial raw client: %v", err)
	}
	if err := client.Login(testAccount, testPassword).Wait(); err != nil {
		t.Fatalf("login raw client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// recreateMailbox deletes and re-creates name, which bumps its
// UIDVALIDITY (imapmemserver.User.Create's own invariant) -- the fixture
// for "a changed UIDVALIDITY resets the cursor".
func (f *fakeServer) recreateMailbox(t *testing.T, name string) {
	t.Helper()
	if err := f.user.Delete(name); err != nil {
		t.Fatalf("delete %s: %v", name, err)
	}
	if err := f.user.Create(name, nil); err != nil {
		t.Fatalf("recreate %s: %v", name, err)
	}
}

// bytesLiteral adapts a []byte to imap.LiteralReader (io.Reader + Size)
// for User.Append.
type bytesLiteral struct {
	*os.File
	size int64
}

func (b bytesLiteral) Size() int64 { return b.size }

// appendFixture appends the raw bytes at path into mailbox and returns the
// UID the server assigned.
func (f *fakeServer) appendFixture(t *testing.T, mailbox, path string) imapapi.UID {
	t.Helper()
	fh, err := os.Open(path)
	if err != nil {
		t.Fatalf("open fixture %s: %v", path, err)
	}
	defer func() { _ = fh.Close() }()
	fi, err := fh.Stat()
	if err != nil {
		t.Fatalf("stat fixture %s: %v", path, err)
	}
	// imapmemserver's appendBytes dereferences options unconditionally (no
	// nil-options fast path), unlike most of this package's other methods
	// -- pass an explicit zero value rather than nil.
	data, err := f.user.Append(mailbox, bytesLiteral{File: fh, size: fi.Size()}, &imapapi.AppendOptions{})
	if err != nil {
		t.Fatalf("append fixture %s: %v", path, err)
	}
	return data.UID
}

// loadFixtures returns the sorted testdata/imap/*.eml fixture paths.
func loadFixtures(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("testdata", "imap", "*.eml"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no testdata/imap/*.eml fixtures found")
	}
	sort.Strings(matches)
	return matches
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// searchHookSession wraps a session and runs afterSearch synchronously
// right after the real SEARCH result is computed but before it is sent
// back to the client -- the deterministic seam for placing a concurrent
// mailbox mutation exactly between SEARCH and the client's next command
// (the "EXPUNGE mid-fetch" pitfall). Every other method is the embedded
// one, unmodified. Embeds the richer SessionIMAP4rev2 (not just Session):
// the server negotiated IMAP4rev2 (imapserver.Options.Caps) and panics if
// a session's dynamic type doesn't also carry SessionNamespace/SessionMove
// -- embedding the base Session interface alone would silently drop those
// promoted methods even though imapmemserver's concrete session has them.
type searchHookSession struct {
	imapserver.SessionIMAP4rev2
	afterSearch func()
}

func (s *searchHookSession) Search(kind imapserver.NumKind, criteria *imapapi.SearchCriteria, options *imapapi.SearchOptions) (*imapapi.SearchData, error) {
	data, err := s.SessionIMAP4rev2.Search(kind, criteria, options)
	if s.afterSearch != nil {
		s.afterSearch()
	}
	return data, err
}
