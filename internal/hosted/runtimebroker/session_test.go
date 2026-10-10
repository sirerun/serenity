package runtimebroker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	return sessionProtocolWithAdoption(t, requireEmpty, badBinding, nil)
}
func sessionProtocolWithAdoption(t *testing.T, requireEmpty, badBinding bool, a *authority.Adoption) (*localTransport, *atomic.Bool) {
	t.Helper()
	client, server := net.Pipe()
	tr := &localTransport{conn: client, reader: bufio.NewReaderSize(client, maxWireBytes+2)}
	activated := &atomic.Bool{}
	done := make(chan struct{})
	t.Cleanup(func() { _ = tr.Close(); _ = server.Close(); <-done })
	p := authority.Prepared{Adoption: a, ReservationID: "reservation", WriterID: "writer", Epoch: 1, Position: contracts.JournalPosition{ActiveGeneration: 1, GenesisIssuanceID: "genesis"}, ExpiresAt: time.Now().Add(time.Hour), BuildSHA: strings.Repeat("a", 40), RequireEmptyData: requireEmpty}
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

func TestStartupAdmitsOnlyExactOwnerLegacyCheckpoint(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		t.Run(fmt.Sprintf("mismatch=%v", mismatch), func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "control.db"), []byte("legacy"), 0600); err != nil {
				t.Fatal(err)
			}
			inventory, err := InventorySHA256(context.Background(), dir)
			if err != nil {
				t.Fatal(err)
			}
			if mismatch {
				inventory = strings.Repeat("f", 64)
			}
			a := authority.Adoption{Version: 1, HistoryOrigin: authority.LegacyHistoryOrigin, CheckpointSHA256: strings.Repeat("c", 64), InventorySHA256: inventory, PriorBuildSHA: strings.Repeat("e", 40)}
			tr, activated := sessionProtocolWithAdoption(t, true, false, &a)
			s, err := openSession(context.Background(), tr, dir)
			if mismatch {
				if err == nil {
					t.Fatal("different checkpoint admitted")
				}
				if activated.Load() {
					t.Fatal("mismatched data reached activation")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if !activated.Load() {
					t.Fatal("bound data did not activate")
				}
				if err := s.Close(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
