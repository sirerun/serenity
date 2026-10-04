package pool

import (
	"context"
	"errors"
	"testing"
)

func TestAcquireExistingForReadDistinguishesColdWarmAndCapacity(t *testing.T) {
	p := &Pool{cfg: Config{MaxInFlight: 1}, open: map[string]*Runtime{}}
	if _, _, err := p.AcquireExistingForRead(context.Background(), "cold-brain"); !errors.Is(err, ErrNotOpen) {
		t.Fatalf("cold brain error = %v, want ErrNotOpen", err)
	}

	warm := &Runtime{}
	p.open["warm-brain"] = warm
	got, release, err := p.AcquireExistingForRead(context.Background(), "warm-brain")
	if err != nil {
		t.Fatal(err)
	}
	if got != warm || warm.users != 1 || p.inFlight != 1 {
		t.Fatalf("warm lease state runtime=%p users=%d inFlight=%d", got, warm.users, p.inFlight)
	}
	if _, _, err := p.AcquireExistingForRead(context.Background(), "cold-brain"); !errors.Is(err, ErrCapacity) {
		t.Fatalf("saturated pool error = %v, want ErrCapacity", err)
	}
	release()
	release()
	if warm.users != 0 || p.inFlight != 0 {
		t.Fatalf("release state users=%d inFlight=%d", warm.users, p.inFlight)
	}
}

func TestAcquireExistingForReadDoesNotOpenColdRuntime(t *testing.T) {
	p := &Pool{cfg: Config{BrainsRoot: t.TempDir(), MaxInFlight: 1}, open: map[string]*Runtime{}}
	if _, _, err := p.AcquireExistingForRead(context.Background(), "1234567890abcdef"); !errors.Is(err, ErrNotOpen) {
		t.Fatalf("cold brain error = %v, want ErrNotOpen", err)
	}
	if len(p.open) != 0 || p.inFlight != 0 {
		t.Fatalf("cold read changed pool: open=%d inFlight=%d", len(p.open), p.inFlight)
	}
}
