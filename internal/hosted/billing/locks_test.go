package billing

import (
	"context"
	"testing"
	"time"
)

func TestKeyedLocksCancellationRetainsHeldEntryUntilLastReference(t *testing.T) {
	var locks keyedLocks
	firstRelease, err := locks.acquire(context.Background(), "account:a")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	waiting := make(chan error, 1)
	go func() {
		_, acquireErr := locks.acquire(ctx, "account:a")
		waiting <- acquireErr
	}()
	deadline := time.Now().Add(time.Second)
	for {
		locks.mu.Lock()
		refs := locks.entries["account:a"].refs
		locks.mu.Unlock()
		if refs == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("waiter did not register its reference")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err = <-waiting:
		if err != context.Canceled {
			t.Fatalf("wait returned %v, want cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled lock wait did not return")
	}
	locks.mu.Lock()
	entry := locks.entries["account:a"]
	if entry == nil || entry.refs != 1 {
		locks.mu.Unlock()
		t.Fatal("canceled waiter removed or corrupted held lock entry")
	}
	locks.mu.Unlock()
	firstRelease()
	locks.mu.Lock()
	_, remains := locks.entries["account:a"]
	locks.mu.Unlock()
	if remains {
		t.Fatal("idle lock entry was not reclaimed")
	}
}
