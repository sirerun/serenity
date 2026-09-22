// Package telemetry provides bounded, fixed-cardinality telemetry and redacted
// structured logs for the hosted service. A full queue never blocks a request.
package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/redact"
)

var (
	ErrQueueFull = errors.New("telemetry queue full")
	ErrClosed    = errors.New("telemetry closed")
	ErrInvalid   = errors.New("invalid telemetry event")
)

// Fixed operation names prevent tenant, brain, account or request identifiers
// from becoming metric dimensions. Producers must choose one of these names.
var allowedOperations = map[string]struct{}{
	"gateway.remember": {}, "gateway.recall": {}, "gateway.forget": {},
	"gateway.admission": {}, "gateway.quota": {}, "gateway.readiness": {},
	"provision.create": {}, "provision.recover": {}, "provider.embedding": {},
	"billing.webhook": {}, "identity.mail": {}, "backup.snapshot": {},
}

// Metric definitions use stable units and bounded names. Values are never
// interpreted as customer data.
var metricUnits = map[string]string{
	"requests_total": "count", "errors_total": "count", "readiness_total": "count",
	"provision_duration_ms": "milliseconds", "remember_duration_ms": "milliseconds",
	"recall_duration_ms": "milliseconds", "quota_denials_total": "count",
	"admission_denials_total": "count", "pool_occupancy": "count",
	"provider_calls_total": "count", "provider_input_tokens_total": "count",
	"provider_cost_usd_total": "usd", "disk_utilization_ratio": "ratio",
	"backup_age_seconds": "seconds", "telemetry_dropped_total": "count",
}

// Event is the metric wire shape emitted as one JSON line.
type Event struct {
	Metric    string                     `json:"metric"`
	Unit      string                     `json:"unit"`
	Operation string                     `json:"operation"`
	Outcome   contracts.TelemetryOutcome `json:"outcome"`
	Value     float64                    `json:"value"`
	At        time.Time                  `json:"at"`
}

func ValidateEvent(e Event) error {
	unit, ok := metricUnits[e.Metric]
	if !ok || unit != e.Unit {
		return fmt.Errorf("%w: metric/unit", ErrInvalid)
	}
	if _, ok := allowedOperations[e.Operation]; !ok {
		return fmt.Errorf("%w: operation", ErrInvalid)
	}
	if e.Outcome != contracts.TelemetryOutcomeSuccess && e.Outcome != contracts.TelemetryOutcomeFailure && e.Outcome != contracts.TelemetryOutcomeDeferred {
		return fmt.Errorf("%w: outcome", ErrInvalid)
	}
	if e.Value < 0 || e.Value != e.Value { // NaN
		return fmt.Errorf("%w: value", ErrInvalid)
	}
	return nil
}

type sink interface{ Write([]byte) (int, error) }

// Logger is a bounded asynchronous structured logger. Log never waits on the
// sink; a blocked/full sink produces an error and increments Dropped.
type Logger struct {
	queue   chan []byte
	sink    sink
	closed  chan struct{}
	done    chan struct{}
	closing sync.Once
	dropped atomic.Uint64
}

func NewLogger(w io.Writer, queueSize int) (*Logger, error) {
	if w == nil || queueSize <= 0 {
		return nil, fmt.Errorf("%w: logger configuration", ErrInvalid)
	}
	l := &Logger{queue: make(chan []byte, queueSize), sink: w, closed: make(chan struct{}), done: make(chan struct{})}
	go l.run()
	return l, nil
}

func (l *Logger) run() {
	defer close(l.done)
	for line := range l.queue {
		_, _ = l.sink.Write(line)
	}
}

func (l *Logger) Dropped() uint64 { return l.dropped.Load() }

func (l *Logger) Emit(ctx context.Context, e Event) error {
	if err := ValidateEvent(e); err != nil {
		return err
	}
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("%w: encode", ErrInvalid)
	}
	line = append(line, '\n')
	select {
	case <-l.closed:
		return ErrClosed
	default:
	}
	select {
	case l.queue <- line:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		l.dropped.Add(1)
		return ErrQueueFull
	}
}

// Close drains queued lines unless ctx expires. A sink that blocks cannot hold
// a request; the caller gets the context error and can continue shutdown.
func (l *Logger) Close(ctx context.Context) error {
	l.closing.Do(func() { close(l.closed); close(l.queue) })
	select {
	case <-l.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

var sensitiveQuery = regexp.MustCompile(`(?i)([?&](?:token|code|secret|key|session|authorization|credential|magic_link)=)[^&#\s]*`)
var sensitiveHeader = regexp.MustCompile(`(?i)((?:authorization|cookie|set-cookie)\s*:\s*)[^\r\n]*`)

func sanitizeText(s string) string {
	s = sensitiveHeader.ReplaceAllString(s, `${1}[REDACTED]`)
	s = sensitiveQuery.ReplaceAllString(s, `${1}[REDACTED]`)
	// Decode only for redaction matching; return the original shape with common
	// secret patterns removed, never the decoded value.
	if decoded, err := url.QueryUnescape(s); err == nil && decoded != s {
		s = redact.Apply(s, redact.Options{RedactEmails: true})
	}
	return redact.Apply(s, redact.Options{RedactEmails: true})
}

// Redact recursively sanitizes values before JSON encoding. Unknown values are
// rendered through fmt.Sprint and therefore cannot leak an error object.
func Redact(v any) any {
	switch x := v.(type) {
	case string:
		return sanitizeText(x)
	case error:
		return sanitizeText(x.Error())
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, value := range x {
			out[sanitizeText(k)] = Redact(value)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, value := range x {
			out[i] = Redact(value)
		}
		return out
	default:
		return sanitizeText(fmt.Sprint(v))
	}
}

// Log writes a single bounded JSON record. It intentionally has no tenant or
// request-id argument, so labels cannot accidentally become high-cardinality.
func (l *Logger) Log(ctx context.Context, level, message string, fields map[string]any) error {
	record := map[string]any{"at": time.Now().UTC(), "level": sanitizeText(level), "message": sanitizeText(message), "fields": Redact(fields)}
	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("%w: log encode", ErrInvalid)
	}
	line = append(line, '\n')
	select {
	case <-l.closed:
		return ErrClosed
	default:
	}
	select {
	case l.queue <- line:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		l.dropped.Add(1)
		return ErrQueueFull
	}
}
