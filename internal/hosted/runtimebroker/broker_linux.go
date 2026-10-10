//go:build linux

package runtimebroker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirerun/serenity/internal/hosted/authority"
	"golang.org/x/sys/unix"
)

// Serve runs the privileged broker until cancellation, then drains every
// connection and attempts exact owned reservation/lease revocation.
func Serve(ctx context.Context, c Config) error {
	if ctx == nil || os.Geteuid() != 0 {
		return errors.New("broker: root and context required")
	}
	if err := c.validate(); err != nil {
		return err
	}
	if err := trustedPath(filepath.Dir(c.DataDir), false); err != nil {
		return err
	}
	var data unix.Stat_t
	if err := unix.Lstat(c.DataDir, &data); err != nil {
		return err
	}
	if data.Uid != c.ServiceUID || data.Mode&unix.S_IFMT != unix.S_IFDIR || data.Mode&0077 != 0 {
		return errors.New("broker: pinned service-owned private data directory required")
	}
	dir := filepath.Dir(c.Socket)
	if err := trustedPath(dir, false); err != nil {
		return err
	}
	var parent unix.Stat_t
	if err := unix.Lstat(dir, &parent); err != nil {
		return err
	}
	if parent.Gid != c.ServiceGID || parent.Mode&0077 != 0050 {
		return errors.New("broker: root-owned service-group socket directory mode 0750 required")
	}
	if _, err := os.Lstat(c.Socket); !errors.Is(err, os.ErrNotExist) {
		return errors.New("broker: socket already exists or unavailable")
	}
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: c.Socket, Net: "unix"})
	if err != nil {
		return err
	}
	l.SetUnlinkOnClose(false)
	info, err := os.Lstat(c.Socket)
	if err != nil {
		return errors.Join(err, l.Close())
	}
	clean := func() error {
		now, err := os.Lstat(c.Socket)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !os.SameFile(info, now) {
			return errors.New("broker: socket identity changed")
		}
		return os.Remove(c.Socket)
	}
	if err = os.Chown(c.Socket, 0, int(c.ServiceGID)); err == nil {
		err = os.Chmod(c.Socket, 0660)
	}
	if err != nil {
		return errors.Join(err, l.Close(), clean())
	}
	r, err := newRemote(c)
	if err != nil {
		return errors.Join(err, l.Close(), clean())
	}
	defer r.Close()
	var wg sync.WaitGroup
	var mu sync.Mutex
	connections := map[*net.UnixConn]struct{}{}
	var failures []error
	stopped := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = l.Close()
			mu.Lock()
			for conn := range connections {
				_ = conn.Close()
			}
			mu.Unlock()
		case <-stopped:
		}
	}()
	for {
		conn, acceptErr := l.AcceptUnix()
		if acceptErr != nil {
			if ctx.Err() == nil {
				mu.Lock()
				failures = append(failures, acceptErr)
				mu.Unlock()
			}
			break
		}
		mu.Lock()
		if len(connections) >= 4 {
			mu.Unlock()
			_ = conn.Close()
			continue
		}
		connections[conn] = struct{}{}
		mu.Unlock()
		wg.Add(1)
		go func() {
			defer wg.Done()
			e := servePeer(ctx, c, r, conn)
			mu.Lock()
			delete(connections, conn)
			if e != nil {
				failures = append(failures, e)
			}
			mu.Unlock()
		}()
	}
	close(stopped)
	closeErr := l.Close()
	if errors.Is(closeErr, net.ErrClosed) {
		closeErr = nil
	}
	wg.Wait()
	return errors.Join(append(failures, closeErr, clean())...)
}
func servePeer(ctx context.Context, c Config, r *remote, conn *net.UnixConn) (resultErr error) {
	defer func() {
		e := conn.Close()
		if !errors.Is(e, net.ErrClosed) {
			resultErr = errors.Join(resultErr, e)
		}
	}()
	measured, err := bindPeer(ctx, conn, c.ServiceUID, c.Unit, c.ExecutableSHA256)
	if err != nil {
		return nil
	}
	defer func() { resultErr = errors.Join(resultErr, measured.Close()) }()
	var reservation, lease, challenge string
	var requireEmpty bool
	var adoption *authority.Adoption
	cuts := map[string]bool{}
	defer func() {
		if reservation == "" && lease == "" {
			return
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		op, err := authority.NewOperationID()
		if err != nil {
			resultErr = errors.Join(resultErr, err)
			return
		}
		q := authority.Request{Action: "close", OperationID: op, LeaseID: lease}
		if lease == "" {
			q.ReservationID = reservation
		}
		_, err = r.Invoke(cleanup, q)
		resultErr = errors.Join(resultErr, err)
	}()
	reader := bufio.NewReaderSize(conn, authority.MaxBodyBytes+2)
	for {
		frame, err := reader.ReadSlice('\n')
		if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
			return nil
		}
		if err != nil || len(frame) > authority.MaxBodyBytes+1 {
			return nil
		}
		var q authority.Request
		if err = strictJSON(frame, &q); err != nil {
			return nil
		}
		if err := rejectRuntimeIdentity(q); err != nil {
			return nil
		}
		if q.ReservationID != "" && q.ReservationID != reservation {
			return nil
		}
		if q.LeaseID != "" && q.LeaseID != lease {
			return nil
		}
		if q.CutID != "" && !cuts[q.CutID] {
			return nil
		}
		switch q.Action {
		case "challenge":
			if reservation != "" || lease != "" {
				return nil
			}
		case "prepare":
			if q.ChallengeID != challenge || challenge == "" {
				return nil
			}
			q.Measurement = &authority.Measurement{ExecutableSHA256: measured.digest, ProcessLifetime: measured.lifetime}
		case "renew":
			q.Measurement = &authority.Measurement{ExecutableSHA256: measured.digest, ProcessLifetime: measured.lifetime}
		case "activate":
			if requireEmpty {
				inventory, err := brokerInitialInventory(ctx, c, adoption)
				if err != nil {
					return err
				}
				q.DataInventorySHA256 = inventory
			}
		case "get", "list", "append", "current", "close", "cut", "finalize_cut":
		default:
			return nil
		}
		if err = measured.Check(ctx); err != nil {
			return nil
		}
		requestCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		response, invokeErr := r.Invoke(requestCtx, q)
		// An unknown outcome retries only the identical request while its measured
		// process remains valid. Never invent a replacement operation/issuance.
		if errors.Is(invokeErr, authority.ErrUnavailable) && measured.Check(requestCtx) == nil {
			response, invokeErr = r.Invoke(requestCtx, q)
		}
		cancel()
		if invokeErr == nil {
			if response.Challenge != nil {
				challenge = response.Challenge.ID
			}
			if response.Prepared != nil {
				if response.Prepared.Adoption != nil && !response.Prepared.RequireEmptyData {
					return errors.New("broker: unexpected adoption after initialization")
				}
				reservation = response.Prepared.ReservationID
				requireEmpty = response.Prepared.RequireEmptyData
				adoption = response.Prepared.Adoption
				if response.Prepared.BuildSHA != c.BuildSHA {
					return errors.New("broker: prepared release mismatch")
				}
			}
			if response.Lease != nil {
				lease = response.Lease.ID
				reservation = response.Lease.ReservationID
				if response.Lease.BuildSHA != c.BuildSHA {
					return errors.New("broker: active release mismatch")
				}
			}
			if response.Cut != nil {
				cuts[response.Cut.ID] = true
			}
			if q.Action == "close" {
				reservation = ""
				lease = ""
			}
		}
		if err := measured.Check(ctx); err != nil {
			return err
		}
		body, err := json.Marshal(wireResponse{Result: response, Error: code(invokeErr)})
		if err != nil || len(body) > maxWireBytes {
			return errors.Join(err, authority.ErrUnavailable)
		}
		if err = conn.SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil {
			return err
		}
		if _, err = conn.Write(append(body, '\n')); err != nil {
			return nil
		}
	}
}
