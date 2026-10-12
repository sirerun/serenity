package meter_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/meter"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func TestDurableAdmissionSurvivesRestartExpiryAndQuota(t *testing.T) {
	for _, released := range []bool{false, true} {
		t.Run(map[bool]string{false: "expired_open", true: "legacy_released"}[released], func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "meter.db")
			s, err := store.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			account, err := s.CreateAccount(ctx, "durable@example.com")
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			m := &meter.Meter{Store: s, Clock: func() time.Time { return now }}
			key := "governed:brain:original"
			r, err := m.ReserveDurable(ctx, account.ID, "writes", 1, 1, "original-window", now.Add(time.Hour), key)
			if err != nil {
				t.Fatal(err)
			}
			if released {
				if _, err = s.DB().Exec("UPDATE reservations SET status='released' WHERE id=?", r.ID); err != nil {
					t.Fatal(err)
				}
			}
			if err = s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = store.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := s.Close(); err != nil {
					t.Error(err)
				}
			}()
			now = now.Add(48 * time.Hour)
			m = &meter.Meter{Store: s, Clock: func() time.Time { return now }}
			// Exercise ordinary admission cleanup after both lease and retention TTL.
			if _, err = m.Reserve(ctx, account.ID, "recalls", 0, 1, "later-window", now.Add(time.Hour), "cleanup"); err != nil {
				t.Fatal(err)
			}
			var count int
			if err = s.DB().QueryRow("SELECT count(*) FROM reservations WHERE id=?", r.ID).Scan(&count); err != nil || count != 1 {
				t.Fatalf("lost original admission: %d %v", count, err)
			}
			// Current quota denial cannot prevent settlement of already admitted work.
			if _, err = m.ReserveDurable(ctx, account.ID, "writes", 1, 0, "later-window", now.Add(time.Hour), "governed:brain:denied"); err == nil {
				t.Fatal("quota denial missing")
			} else {
				var limit *meter.LimitError
				if !errors.As(err, &limit) {
					t.Fatal(err)
				}
			}
			for range 2 {
				if err = m.CommitDurableOperation(ctx, account.ID, "writes", key); err != nil {
					t.Fatal(err)
				}
			}
			var used int64
			if err = s.DB().QueryRow("SELECT committed FROM usage_windows WHERE account_id=? AND window_key='original-window' AND metric='writes'", account.ID).Scan(&used); err != nil || used != 1 {
				t.Fatalf("original charge %d %v", used, err)
			}
			if err = s.DB().QueryRow("SELECT count(*) FROM reservations WHERE operation_key='governed:brain:denied'").Scan(&count); err != nil || count != 0 {
				t.Fatalf("denied operation charged: %d %v", count, err)
			}
			if err = m.CommitDurableOperation(ctx, account.ID, "writes", "governed:brain:missing"); err == nil {
				t.Fatal("missing admission accepted")
			}
		})
	}
}
