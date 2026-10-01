package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/writer"
)

// Request runs an offline backup directly (holding dataDir's exclusive local
// lock itself) when no live service is listening on its admin socket, or
// else asks the running service to run one in its own coordinated
// maintenance window. buildSHA and journal are used only for the offline
// path; the online path's in-process Create call is the live service's own
// responsibility to supply them for (see internal/hosted/service).
func Request(ctx context.Context, dataDir, destination, buildSHA string, journal contracts.DeletionJournal) error {
	socket := filepath.Join(dataDir, ".hosted-admin.sock")
	if _, err := os.Stat(socket); errors.Is(err, os.ErrNotExist) {
		owner, e := writer.AcquireBrain(dataDir)
		if e != nil {
			return e
		}
		backupErr := Create(ctx, dataDir, destination, buildSHA, journal)
		return errors.Join(backupErr, owner.Close())
	} else if err != nil {
		return err
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 10 * time.Minute}
	payload, err := json.Marshal(map[string]string{"destination": destination})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost/backup", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("hosted backup coordinator unavailable")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		return errors.New("hosted backup did not complete")
	}
	return nil
}
