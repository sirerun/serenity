package runtimebroker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirerun/serenity/internal/hosted/authority"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/deletion"
)

// Session can only be constructed through the root broker. Its immutable
// observation proof is distinct from its renewing mutable writer lease.
type Session struct {
	transport  *localTransport
	client     *authority.Client
	prepared   authority.Prepared
	observed   contracts.JournalObservation
	reader     *deletion.ReadOnlyJournal
	journal    *deletion.Journal
	mu         sync.Mutex
	lease      authority.Lease
	renewalErr error
	cancel     context.CancelFunc
	done       chan struct{}
	closeOnce  sync.Once
	closeErr   error
}

func OpenSession(ctx context.Context, socket, dataDir string) (*Session, error) {
	t, err := newLocalTransport(ctx, socket)
	if err != nil {
		return nil, err
	}
	return openSession(ctx, t, dataDir)
}
func openSession(ctx context.Context, t *localTransport, dataDir string) (result *Session, resultErr error) {
	c, err := authority.NewClient(t)
	if err != nil {
		return nil, errors.Join(err, t.Close())
	}
	var prepared authority.Prepared
	var lease authority.Lease
	defer func() {
		if resultErr == nil {
			return
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		op, err := authority.NewOperationID()
		if err == nil {
			if lease.ID != "" {
				err = c.Close(cleanup, lease, op)
			} else if prepared.ReservationID != "" {
				err = c.ClosePrepared(cleanup, prepared, op)
			}
		}
		resultErr = errors.Join(resultErr, err, t.Close())
	}()
	ch, err := c.Challenge(ctx)
	if err != nil {
		return nil, err
	}
	prepared, err = c.Prepare(ctx, ch)
	if err != nil {
		return nil, err
	}
	reader, err := deletion.NewJournalReader(c.Reader(prepared), prepared.Position)
	if err != nil {
		return nil, err
	}
	observed, err := reader.Observe(ctx)
	if err != nil {
		return nil, err
	}
	if prepared.RequireEmptyData {
		if err := requireEmptyData(dataDir); err != nil {
			return nil, err
		}
	}
	op, err := authority.NewOperationID()
	if err != nil {
		return nil, err
	}
	lease, err = c.Activate(ctx, prepared, observed.HistorySHA256, op)
	if err != nil {
		return nil, err
	}
	if lease.ReservationID != prepared.ReservationID || lease.WriterID != prepared.WriterID || lease.Epoch != prepared.Epoch || lease.Position != prepared.Position || lease.BuildSHA != prepared.BuildSHA || lease.BuildSHA == "" {
		return nil, errors.New("broker: activation binding mismatch")
	}
	journal, err := deletion.NewJournalAt(c.Store(lease), lease.WriterID, lease.Position, time.Now)
	if err != nil {
		return nil, err
	}
	lifetime, cancel := context.WithCancel(context.Background())
	s := &Session{transport: t, client: c, prepared: prepared, observed: observed, reader: reader, journal: journal, lease: lease, cancel: cancel, done: make(chan struct{})}
	if err = s.CheckReadiness(ctx); err != nil {
		cancel()
		return nil, err
	}
	go s.renew(lifetime)
	return s, nil
}
func (s *Session) Position() contracts.JournalPosition     { return s.prepared.Position }
func (s *Session) BuildSHA() string                        { return s.prepared.BuildSHA }
func (s *Session) Journal() contracts.DeletionJournal      { return s.journal }
func (s *Session) Reader() contracts.DeletionJournalReader { return s.reader }
func (s *Session) Authority() *authority.Client            { return s.client }
func (s *Session) Lease() authority.Lease                  { s.mu.Lock(); defer s.mu.Unlock(); return s.lease }
func (s *Session) VerifyObservation(ctx context.Context, o contracts.JournalObservation) error {
	if ctx == nil {
		return authority.ErrInvalid
	}
	if o.Position != s.observed.Position || o.HistorySHA256 != s.observed.HistorySHA256 || o.Read.To != s.observed.Read.To || o.Read.Sealed != s.observed.Read.Sealed {
		return errors.New("broker: observation does not match activated history")
	}
	return s.CheckReadiness(ctx)
}
func (s *Session) CheckReadiness(ctx context.Context) error {
	s.mu.Lock()
	lease := s.lease
	renewErr := s.renewalErr
	s.mu.Unlock()
	if renewErr != nil {
		return renewErr
	}
	current, err := s.client.Current(ctx, lease)
	if err != nil {
		return err
	}
	if current.Epoch != lease.Epoch || !current.ExpiresAt.After(time.Now()) {
		return authority.ErrExpired
	}
	return nil
}
func (s *Session) renew(ctx context.Context) {
	defer close(s.done)
	for {
		l := s.Lease()
		wait := time.Until(l.ExpiresAt) / 3
		if wait <= 0 {
			s.setRenewalError(authority.ErrExpired)
			return
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		op, err := authority.NewOperationID()
		if err != nil {
			s.setRenewalError(err)
			return
		}
		renewalCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		next, err := s.client.Renew(renewalCtx, l, op)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.setRenewalError(fmt.Errorf("broker: lease renewal unavailable: %w", err))
			return
		}
		if next.ID != l.ID || next.Epoch != l.Epoch || next.WriterID != l.WriterID || next.BuildSHA != l.BuildSHA || !next.ExpiresAt.After(l.ExpiresAt) {
			s.setRenewalError(authority.ErrConflict)
			return
		}
		s.mu.Lock()
		s.lease = next
		s.mu.Unlock()
	}
}
func (s *Session) setRenewalError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.renewalErr = errors.Join(err, s.transport.Close())
}
func (s *Session) Close(ctx context.Context) error {
	if ctx == nil {
		return authority.ErrInvalid
	}
	s.closeOnce.Do(func() {
		s.cancel()
		<-s.done
		op, err := authority.NewOperationID()
		if err == nil {
			err = s.client.Close(ctx, s.Lease(), op)
		}
		s.closeErr = errors.Join(err, s.transport.Close())
	})
	return s.closeErr
}

// First authority enrollment cannot silently adopt preexisting control data.
// Caller holds the local data ownership lock across this check and assembly.
func requireEmptyData(dir string) error {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return errors.New("broker: canonical fresh data directory required")
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("broker: real data directory required")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() != ".serenity" || !e.IsDir() || e.Type()&os.ModeSymlink != 0 {
			return errors.New("broker: existing data requires owner adoption or recovery")
		}
		state, err := os.ReadDir(filepath.Join(dir, ".serenity"))
		if err != nil {
			return err
		}
		for _, f := range state {
			if f.Name() != "writer.lock" || f.Type()&os.ModeSymlink != 0 || f.IsDir() {
				return errors.New("broker: existing runtime state requires owner adoption or recovery")
			}
			i, err := f.Info()
			if err != nil {
				return err
			}
			if !i.Mode().IsRegular() || i.Size() != 0 {
				return errors.New("broker: unexpected ownership lock content")
			}
		}
	}
	return nil
}

// Done closes when the renewal lifetime ends; callers must fence and drain
// handlers before closing service workers/storage and the exact lease.
func (s *Session) Done() <-chan struct{} { return s.done }
func (s *Session) Err() error            { s.mu.Lock(); defer s.mu.Unlock(); return s.renewalErr }
