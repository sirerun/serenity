package runtimebroker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/sirerun/serenity/internal/hosted/authority"
)

func validTestConfig() Config {
	return Config{DataDir: "/var/lib/serenity", Socket: "/run/serenity-broker/runtime.sock", ServiceUID: 991, ServiceGID: 991, Unit: "serenity-hosted.service", ExecutableSHA256: strings.Repeat("a", 64), BuildSHA: strings.Repeat("b", 40), Endpoint: "https://0123456789.execute-api.us-west-2.amazonaws.com/prod/runtime", Region: "us-west-2", CredentialSource: "imds-v2-instance-role"}
}
func TestConfigRejectsAuthoritySubstitution(t *testing.T) {
	for _, name := range []string{"http", "user-info", "query", "region", "custom-host", "default-credentials", "root-peer", "unit-option", "bad-digest", "socket-traversal"} {
		t.Run(name, func(t *testing.T) {
			c := validTestConfig()
			switch name {
			case "http":
				c.Endpoint = strings.Replace(c.Endpoint, "https:", "http:", 1)
			case "user-info":
				c.Endpoint = strings.Replace(c.Endpoint, "https://", "https://secret@", 1)
			case "query":
				c.Endpoint += "?role=admin"
			case "region":
				c.Region = "us-east-1"
			case "custom-host":
				c.Endpoint = "https://evil.example/runtime"
			case "default-credentials":
				c.CredentialSource = "default"
			case "root-peer":
				c.ServiceUID = 0
			case "unit-option":
				c.Unit = "--help.service"
			case "bad-digest":
				c.ExecutableSHA256 = "abc"
			case "socket-traversal":
				c.Socket = "/run/../tmp/sock"
			}
			if c.validate() == nil {
				t.Fatal("unsafe config accepted")
			}
		})
	}
}
func TestValidConfig(t *testing.T) {
	if err := validTestConfig().validate(); err != nil {
		t.Fatal(err)
	}
}
func TestStrictJSONRejectsAmbiguity(t *testing.T) {
	for _, body := range []string{`null`, `{"action":"get","action":"append"}`, `{"action":"get","identity":"root"}`, `{"action":"get"} {}`, `{"body":` + strings.Repeat("[", 70) + strings.Repeat("]", 70) + `}`} {
		var q authority.Request
		if strictJSON([]byte(body), &q) == nil {
			t.Fatalf("ambiguous JSON accepted: %.100s", body)
		}
	}
}
func TestSelectedProfileNeverFallsBack(t *testing.T) {
	data := []byte("[default]\naws_access_key_id=not-selected\naws_secret_access_key=not-selected\n[operator]\naws_access_key_id=chosen\naws_secret_access_key=chosen-secret\n")
	a, s, _, err := selectedProfile(data, "operator")
	if err != nil || a != "chosen" || s != "chosen-secret" {
		t.Fatal("explicit profile not selected")
	}
	if _, _, _, err = selectedProfile(data, "missing"); err == nil {
		t.Fatal("missing profile fell back")
	}
	if _, _, _, err = selectedProfile(append(data, []byte("aws_access_key_id=duplicate\n")...), "operator"); err == nil {
		t.Fatal("duplicate profile field accepted")
	}
}

type roundTripTest func(*http.Request) (*http.Response, error)

func (f roundTripTest) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestRemoteSignsExactScopedBodyAndBoundsResponse(t *testing.T) {
	signed := false
	r := &remote{endpoint: validTestConfig().Endpoint, region: "us-west-2", credentials: credentials.NewStaticCredentialsProvider("test-key", "test-secret", ""), signer: v4.NewSigner()}
	r.client = &http.Client{Transport: roundTripTest(func(req *http.Request) (*http.Response, error) {
		b, e := io.ReadAll(req.Body)
		if e != nil {
			return nil, e
		}
		if !strings.Contains(req.Header.Get("Authorization"), "/us-west-2/execute-api/aws4_request") || !strings.Contains(string(b), `"action":"current"`) {
			t.Error("incorrect SigV4 scope or body")
		}
		signed = true
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", authority.MaxBodyBytes+1))), Header: http.Header{}}, nil
	})}
	if _, err := r.Invoke(context.Background(), authority.Request{Action: "current", LeaseID: "test"}); !errors.Is(err, authority.ErrUnavailable) || !signed {
		t.Fatal("oversized response accepted or request unsigned")
	}
}
func TestOperatorTransportCannotMutate(t *testing.T) {
	for _, action := range []string{"prepare", "append", "activate", "renew", "close", "cut"} {
		if _, err := (operatorTransport{}).Invoke(context.Background(), authority.Request{Action: action}); !errors.Is(err, authority.ErrForbidden) {
			t.Fatalf("operator mutation %s accepted", action)
		}
	}
}
func TestTransportCloseInterruptsInflightRequest(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = server.Close() }()
	tr := &localTransport{conn: client, reader: bufio.NewReaderSize(client, authority.MaxBodyBytes+2)}
	read := make(chan struct{})
	go func() { _, _ = bufio.NewReader(server).ReadBytes('\n'); close(read) }()
	done := make(chan error, 1)
	go func() { _, err := tr.Invoke(context.Background(), authority.Request{Action: "current"}); done <- err }()
	<-read
	if err := tr.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("closed request succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("close waited behind blocked request")
	}
}
func TestLocalTransportRejectsForgedResponseShape(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = server.Close() }()
	tr := &localTransport{conn: client, reader: bufio.NewReaderSize(client, authority.MaxBodyBytes+2)}
	defer func() { _ = tr.Close() }()
	go func() {
		var q authority.Request
		_ = json.NewDecoder(server).Decode(&q)
		_, _ = io.WriteString(server, "{\"result\":{},\"result\":{}}\n")
	}()
	if _, err := tr.Invoke(context.Background(), authority.Request{Action: "current"}); err == nil {
		t.Fatal("duplicate response accepted")
	}
}

func TestFreshEnrollmentRejectsEveryPreexistingDataKind(t *testing.T) {
	for _, name := range []string{"control.db", "brains", "snapshot", ".serenity"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			var err error
			if name == "brains" || name == ".serenity" {
				err = os.Mkdir(filepath.Join(dir, name), 0700)
				if err == nil {
					err = os.WriteFile(filepath.Join(dir, name, "legacy"), []byte("x"), 0600)
				}
			} else {
				err = os.WriteFile(filepath.Join(dir, name), []byte("x"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if requireEmptyData(dir) == nil {
				t.Fatal("existing data silently adopted")
			}
		})
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".serenity"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".serenity", "writer.lock"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := requireEmptyData(dir); err != nil {
		t.Fatal(err)
	}
}
