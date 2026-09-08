package importbudget_test

// All synthetic server/model implementations in this benchmark are test-only.
// The measured path uses the production IMAP connector and storage pipeline.
import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"

	imapapi "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	imapconnector "github.com/sirerun/serenity/internal/connector/imap"
	"github.com/sirerun/serenity/internal/eval/messages"
)

type literal struct{ *bytes.Reader }

func (l literal) Size() int64 { return int64(l.Len()) }
func mailbox(t *testing.T, dir string, manifest messages.Manifest) *imapconnector.Connector {
	t.Helper()
	mem := imapmemserver.New()
	user := imapmemserver.NewUser("benchmark@example.invalid", "synthetic-fixture")
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	mem.AddUser(user)
	count := 0
	for _, file := range manifest.Files {
		raw, err := os.ReadFile(filepath.Join(dir, file.Path))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != file.SHA256 {
			t.Fatal("corpus checksum mismatch")
		}
		scanner := bufio.NewScanner(bytes.NewReader(raw))
		var msg bytes.Buffer
		fileCount := 0
		appendMessage := func() {
			if msg.Len() == 0 {
				return
			}
			if _, err := mail.ReadMessage(bytes.NewReader(msg.Bytes())); err != nil {
				t.Fatal(err)
			}
			if _, err := user.Append("INBOX", literal{bytes.NewReader(msg.Bytes())}, &imapapi.AppendOptions{}); err != nil {
				t.Fatal(err)
			}
			fileCount++
			msg.Reset()
		}
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "From ") {
				appendMessage()
				continue
			}
			if strings.HasPrefix(line, ">") && strings.HasPrefix(strings.TrimLeft(line, ">"), "From ") {
				line = line[1:]
			}
			fmt.Fprintln(&msg, line)
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		appendMessage()
		if fileCount != file.Messages {
			t.Fatalf("mbox messages=%d want %d", fileCount, file.Messages)
		}
		count += fileCount
	}
	if count != manifest.Count {
		t.Fatal("incomplete corpus")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := imapserver.New(&imapserver.Options{NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
		return mem.NewSession(), nil, nil
	}, Caps: imapapi.CapSet{imapapi.CapIMAP4rev1: {}, imapapi.CapIMAP4rev2: {}}, InsecureAuth: true})
	t.Cleanup(func() { _ = server.Close() })
	go func() { _ = server.Serve(listener) }()
	return &imapconnector.Connector{Account: "benchmark@example.invalid", Dial: func() (*imapclient.Client, error) {
		client, err := imapclient.DialInsecure(listener.Addr().String(), nil)
		if err != nil {
			return nil, err
		}
		if err := client.Login("benchmark@example.invalid", "synthetic-fixture").Wait(); err != nil {
			_ = client.Close()
			return nil, err
		}
		return client, nil
	}}
}
