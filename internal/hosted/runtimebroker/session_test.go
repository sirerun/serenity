package runtimebroker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/authority"
	"github.com/sirerun/serenity/internal/hosted/contracts"
)

// The protocol fixture is test-only; separate subprocess tests exercise real
// Unix credentials/pidfd/executable measurement rather than this transport.
func sessionProtocol(t *testing.T, requireEmpty, badBinding bool) (*localTransport, *atomic.Bool) {
	t.Helper()
	client, server := net.Pipe()
	tr := &localTransport{conn: client, reader: bufio.NewReaderSize(client, maxWireBytes+2)}
	activated := &atomic.Bool{}
	done := make(chan struct{})
	t.Cleanup(func() { _ = tr.Close(); _ = server.Close(); <-done })
	p := authority.Prepared{ReservationID: "reservation", WriterID: "writer", Epoch: 1, Position: contracts.JournalPosition{ActiveGeneration: 1, GenesisIssuanceID: "genesis"}, ExpiresAt: time.Now().Add(time.Hour), BuildSHA: strings.Repeat("a", 40), RequireEmptyData: requireEmpty}
	go func() {
		defer close(done)
		reader := bufio.NewReader(server)
		for {
			frame, err := reader.ReadBytes('\n')
			if err != nil {
				return
			}
			var q authority.Request
			if err = json.Unmarshal(frame, &q); err != nil {
				return
			}
			var response authority.Response
			switch q.Action {
			case "challenge":
				response.Challenge = &authority.Challenge{ID: "challenge", Nonce: "nonce", OperationID: "operation", ExpiresAt: p.ExpiresAt}
			case "prepare":
				response.Prepared = &p
			case "list":
				response.Keys = []string{}
			case "activate":
				activated.Store(true)
				l := authority.Lease{ID: "lease", ReservationID: p.ReservationID, WriterID: p.WriterID, Epoch: p.Epoch, Position: p.Position, ExpiresAt: p.ExpiresAt, BuildSHA: p.BuildSHA}
				if badBinding {
					l.WriterID = "forged"
				}
				response.Lease = &l
			case "current":
				response.Current = &authority.Current{Position: p.Position, Epoch: p.Epoch, ExpiresAt: p.ExpiresAt}
			case "close":
			default:
				return
			}
			body, err := json.Marshal(wireResponse{Result: response})
			if err != nil {
				return
			}
			if _, err = server.Write(append(body, '\n')); err != nil {
				return
			}
		}
	}()
	return tr, activated
}
func TestStartupProtocolRejectsLegacyDataBeforeActivation(t *testing.T) {
	tr, activated := sessionProtocol(t, true, false)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "control.db"), []byte("legacy"), 0600); err != nil {
		t.Fatal(err)
	}
	if s, err := openSession(context.Background(), tr, dir); err == nil || s != nil {
		t.Fatal("legacy database admitted")
	}
	if activated.Load() {
		t.Fatal("authority activated before fresh-data proof")
	}
}
func TestStartupProtocolActivatesObservedHistoryAndJoinsClose(t *testing.T) {
	tr, activated := sessionProtocol(t, true, false)
	s, err := openSession(context.Background(), tr, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !activated.Load() {
		t.Fatal("authority was not activated")
	}
	observation, err := s.Reader().Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.VerifyObservation(context.Background(), observation); err != nil {
		t.Fatal(err)
	}
	if observation.Read.Sealed {
		t.Fatal("ordinary startup forged a recovery seal")
	}
	if err = s.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.Done():
	default:
		t.Fatal("renewal monitor not joined")
	}
}
func TestStartupProtocolRejectsSubstitutedActivationBinding(t *testing.T) {
	tr, _ := sessionProtocol(t, true, true)
	if s, err := openSession(context.Background(), tr, t.TempDir()); err == nil || s != nil {
		t.Fatal("substituted writer accepted")
	}
}
func TestRenewalFailureClosesTransportAndAdmission(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = server.Close() }()
	s := &Session{transport: &localTransport{conn: client, reader: bufio.NewReader(client)}}
	s.setRenewalError(authority.ErrExpired)
	if !errors.Is(s.Err(), authority.ErrExpired) {
		t.Fatal("expiry not retained")
	}
	if err := s.CheckReadiness(context.Background()); !errors.Is(err, authority.ErrExpired) {
		t.Fatal("expired session remained ready")
	}
	if _, err := server.Write([]byte("x")); err == nil {
		t.Fatal("expired transport remained open")
	}
}
