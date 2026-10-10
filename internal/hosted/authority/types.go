// Package authority implements the hosted journal authority protocol. Runtime
// callers hold only opaque server-issued capabilities; authentication belongs
// to the trusted transport, never to fields asserted by the runtime.
package authority

import (
	"context"
	"errors"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

const MaxBodyBytes = 96 * 1024
const MaxObjectBytes = 64 * 1024
const MaxPageKeys = 256

var (
	ErrInvalid     = errors.New("authority: invalid request")
	ErrForbidden   = errors.New("authority: forbidden")
	ErrConflict    = errors.New("authority: conflicting or stale operation")
	ErrExpired     = errors.New("authority: capability expired")
	ErrUnavailable = errors.New("authority: outcome unknown; retry identical operation")
)

// Measurement is inserted by the privileged broker after authenticating and
// measuring its local peer. It is not proof when supplied by an ordinary caller.
type Measurement struct {
	ExecutableSHA256 string `json:"executable_sha256"`
	ProcessLifetime  string `json:"process_lifetime"`
}

type Challenge struct {
	ID          string    `json:"id"`
	Nonce       string    `json:"nonce"`
	OperationID string    `json:"operation_id"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type Prepared struct {
	RequireEmptyData bool                      `json:"require_empty_data"`
	ReservationID    string                    `json:"reservation_id"`
	WriterID         string                    `json:"writer_id"`
	Epoch            uint64                    `json:"epoch"`
	Position         contracts.JournalPosition `json:"position"`
	ExpiresAt        time.Time                 `json:"expires_at"`
	BuildSHA         string                    `json:"build_sha"`
}

type Lease struct {
	ID            string                    `json:"id"`
	ReservationID string                    `json:"reservation_id"`
	WriterID      string                    `json:"writer_id"`
	Epoch         uint64                    `json:"epoch"`
	Position      contracts.JournalPosition `json:"position"`
	ExpiresAt     time.Time                 `json:"expires_at"`
	BuildSHA      string                    `json:"build_sha"`
}

type Current struct {
	Position  contracts.JournalPosition `json:"position"`
	Epoch     uint64                    `json:"epoch"`
	ExpiresAt time.Time                 `json:"expires_at"`
}

type Cut struct {
	ID                string                    `json:"id"`
	LeaseID           string                    `json:"lease_id"`
	Position          contracts.JournalPosition `json:"position"`
	SnapshotAttemptID string                    `json:"snapshot_attempt_id"`
	ExpiresAt         time.Time                 `json:"expires_at"`
	Anchor            WitnessTip                `json:"anchor"`
}

type FinalizedCut struct {
	Cut            Cut    `json:"cut"`
	ManifestSHA256 string `json:"manifest_sha256"`
	OperationID    string `json:"operation_id"`
}

// Request has no authenticated principal, enrollment, account, or namespace.
// The transport binds those independently to its owner-controlled endpoint.
type Request struct {
	Action            string       `json:"action"`
	OperationID       string       `json:"operation_id,omitempty"`
	ChallengeID       string       `json:"challenge_id,omitempty"`
	Nonce             string       `json:"nonce,omitempty"`
	Measurement       *Measurement `json:"measurement,omitempty"`
	ReservationID     string       `json:"reservation_id,omitempty"`
	LeaseID           string       `json:"lease_id,omitempty"`
	CutID             string       `json:"cut_id,omitempty"`
	HistorySHA256     string       `json:"history_sha256,omitempty"`
	ManifestSHA256    string       `json:"manifest_sha256,omitempty"`
	Key               string       `json:"key,omitempty"`
	Body              []byte       `json:"body,omitempty"`
	Prefix            string       `json:"prefix,omitempty"`
	StartAfter        string       `json:"start_after,omitempty"`
	Limit             int          `json:"limit,omitempty"`
	SnapshotAttemptID string       `json:"snapshot_attempt_id,omitempty"`
}

type Response struct {
	Challenge    *Challenge    `json:"challenge,omitempty"`
	Prepared     *Prepared     `json:"prepared,omitempty"`
	Lease        *Lease        `json:"lease,omitempty"`
	Current      *Current      `json:"current,omitempty"`
	Cut          *Cut          `json:"cut,omitempty"`
	FinalizedCut *FinalizedCut `json:"finalized_cut,omitempty"`
	Body         []byte        `json:"body,omitempty"`
	Keys         []string      `json:"keys,omitempty"`
	Found        bool          `json:"found,omitempty"`
	More         bool          `json:"more,omitempty"`
	Created      bool          `json:"created,omitempty"`
}

// Transport must authenticate the caller independently of Request. The
// production runtime supplies a local privileged-broker transport.
type Transport interface {
	Invoke(context.Context, Request) (Response, error)
}
