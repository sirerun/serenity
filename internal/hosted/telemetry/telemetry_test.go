package telemetry

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

type blockingSink struct{}

func (blockingSink) Write([]byte) (int, error) { select {} }

func validEvent() Event {
	return Event{Metric: "requests_total", Unit: "count", Operation: "gateway.recall", Outcome: contracts.TelemetryOutcomeSuccess, Value: 1}
}

func TestRedactRecursiveSentinels(t *testing.T) {
	var out bytes.Buffer
	l, err := NewLogger(&out, 4)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := l.Log(ctx, "error", "provider failed", map[string]any{"url": "https://x.test/callback?token=sentinel-token&code=sentinel-code", "nested": []any{"Authorization: Bearer sentinel-bearer", errors.New("email jane@example.com key sk-TESTFAKEKEY1234567890ABCDEFGHIJKL")}}); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, sentinel := range []string{"sentinel-token", "sentinel-code", "sentinel-bearer", "jane@example.com", "sk-TESTFAKEKEY1234567890ABCDEFGHIJKL"} {
		if strings.Contains(got, sentinel) {
			t.Fatalf("log leaked %q: %s", sentinel, got)
		}
	}
	if !strings.Contains(got, "REDACTED") {
		t.Fatal("redaction marker missing")
	}
}

func TestQueueIsBoundedAndNonBlocking(t *testing.T) {
	l, err := NewLogger(blockingSink{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	for i := 0; i < 100; i++ {
		err = l.Emit(ctx, validEvent())
		if errors.Is(err, ErrQueueFull) {
			break
		}
	}
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("queue never filled: %v", err)
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Fatal("full queue blocked caller")
	}
	if l.Dropped() == 0 {
		t.Fatal("queue-full emission was not counted")
	}
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer closeCancel()
	_ = l.Close(closeCtx) // blocked sink is a shutdown limitation, not request work
}

func TestMetricContractRejectsUnboundedFields(t *testing.T) {
	if err := ValidateEvent(Event{Metric: "request_for_alice", Unit: "count", Operation: "gateway.recall", Outcome: contracts.TelemetryOutcomeSuccess, Value: 1}); err == nil {
		t.Fatal("accepted unbounded metric")
	}
	if err := ValidateEvent(Event{Metric: "requests_total", Unit: "count", Operation: "tenant-alice", Outcome: contracts.TelemetryOutcomeSuccess, Value: 1}); err == nil {
		t.Fatal("accepted unbounded operation")
	}
}

func TestCloseAndEmitDoNotRaceQueueClose(t *testing.T) {
	var out bytes.Buffer
	l, err := NewLogger(&out, 8)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			_ = l.Emit(context.Background(), validEvent())
		}
	}()
	closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = l.Close(closeCtx)
	<-done
}
