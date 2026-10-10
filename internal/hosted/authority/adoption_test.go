package authority

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/deletion"
)

func adoptionFixture() Adoption {
	return Adoption{Version: 1, HistoryOrigin: LegacyHistoryOrigin, CheckpointSHA256: strings.Repeat("c", 64), InventorySHA256: strings.Repeat("d", 64), PriorBuildSHA: strings.Repeat("e", 40)}
}

func TestAdoptionValidationAndStrictParsing(t *testing.T) {
	a := adoptionFixture()
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseAdoption(b)
	if err != nil || got != a {
		t.Fatalf("valid receipt: %v", err)
	}
	for _, name := range []string{"origin", "version", "checkpoint", "inventory", "build"} {
		t.Run(name, func(t *testing.T) {
			bad := a
			switch name {
			case "origin":
				bad.HistoryOrigin = "complete-history"
			case "version":
				bad.Version = 0
			case "checkpoint":
				bad.CheckpointSHA256 = ""
			case "inventory":
				bad.InventorySHA256 = strings.Repeat("D", 64)
			case "build":
				bad.PriorBuildSHA = "legacy"
			}
			if bad.Validate() == nil {
				t.Fatal("invalid adoption accepted")
			}
		})
	}
	for _, body := range []string{string(b[:len(b)-1]) + `,"version":1}`, string(b[:len(b)-1]) + `,"extra":true}`, string(b) + " {}", strings.Repeat(" ", 4097)} {
		if _, err := ParseAdoption([]byte(body)); err == nil {
			t.Fatal("ambiguous receipt accepted")
		}
	}
}

func adoptionServer(t *testing.T) (*Server, *Client, *Client, *memoryAtomic, *memoryWitness) {
	t.Helper()
	now := time.Unix(1700000000, 0).UTC()
	store := newMemoryAtomic()
	w := &memoryWitness{items: map[string][]byte{}}
	k := &Kernel{Store: store, Witness: w, Lineage: "synthetic-adoption-lineage", TableARN: "arn:aws:dynamodb:us-west-2:000000000000:table/adoption"}
	s, err := NewServer(k, ServerConfig{RuntimePrincipal: "runtime", OwnerPrincipal: "owner", ApprovedBuilds: map[string]string{strings.Repeat("a", 64): strings.Repeat("b", 40)}, Now: func() time.Time { return now }, LeaseLifetime: time.Hour, ChallengeLifetime: time.Minute, GenesisLifetime: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := NewClient(localTransport{s: s, principal: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := NewClient(localTransport{s: s, principal: "runtime", measurement: Measurement{ExecutableSHA256: strings.Repeat("a", 64), ProcessLifetime: "process"}})
	if err != nil {
		t.Fatal(err)
	}
	return s, owner, runtime, store, w
}

func prepareAdoption(t *testing.T, c *Client) Prepared {
	t.Helper()
	ch, err := c.Challenge(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p, err := c.Prepare(context.Background(), ch)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func adoptionObservation(t *testing.T, c *Client, p Prepared) string {
	t.Helper()
	reader, err := deletion.NewJournalReader(c.Reader(p), p.Position)
	if err != nil {
		t.Fatal(err)
	}
	o, err := reader.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return o.HistorySHA256
}

func TestOwnerAdoptionPersistsFailedActivationAndCloseUntilInitialized(t *testing.T) {
	ctx := context.Background()
	s, owner, c, store, _ := adoptionServer(t)
	a := adoptionFixture()
	if err := c.EnrollWithAdoption(ctx, a, opID(t)); !errors.Is(err, ErrForbidden) {
		t.Fatalf("runtime enroll: %v", err)
	}
	if err := owner.EnrollWithAdoption(ctx, a, opID(t)); err != nil {
		t.Fatal(err)
	}
	hash, err := a.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	if b, found, err := store.Get(ctx, "ADOPTION#"+hash); err != nil || !found || len(b) == 0 {
		t.Fatal("immutable adoption missing")
	}
	p := prepareAdoption(t, c)
	if !p.RequireEmptyData || p.Adoption == nil || *p.Adoption != a {
		t.Fatal("owner adoption not bound to first preparation")
	}
	history := adoptionObservation(t, c, p)
	for _, inventory := range []string{"", strings.Repeat("f", 64)} {
		_, err := s.execute(ctx, "runtime", Request{Action: "activate", OperationID: opID(t), ReservationID: p.ReservationID, HistorySHA256: history, DataInventorySHA256: inventory})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("unbound adoption activated: %v", err)
		}
	}
	if err := c.ClosePrepared(ctx, p, opID(t)); err != nil {
		t.Fatal(err)
	}
	p = prepareAdoption(t, c)
	if !p.RequireEmptyData || p.Adoption == nil || *p.Adoption != a {
		t.Fatal("close bypassed initial adoption")
	}
	history = adoptionObservation(t, c, p)
	result, err := s.execute(ctx, "runtime", Request{Action: "activate", OperationID: opID(t), ReservationID: p.ReservationID, HistorySHA256: history, DataInventorySHA256: a.InventorySHA256})
	if err != nil || result.Lease == nil {
		t.Fatalf("bound activation: %v", err)
	}
	if err := c.Close(ctx, *result.Lease, opID(t)); err != nil {
		t.Fatal(err)
	}
	p = prepareAdoption(t, c)
	if p.RequireEmptyData || p.Adoption != nil {
		t.Fatal("ordinary restart incorrectly pins legacy bytes")
	}
	inspected, err := s.execute(ctx, "owner", Request{Action: "inspect"})
	if err != nil || inspected.Adoption == nil || *inspected.Adoption != a {
		t.Fatal("durable provenance lost after initialization")
	}
}

func TestAuthorityRestorePreservesUninitializedAdoption(t *testing.T) {
	ctx := context.Background()
	s, owner, c, _, w := adoptionServer(t)
	a := adoptionFixture()
	if err := owner.EnrollWithAdoption(ctx, a, opID(t)); err != nil {
		t.Fatal(err)
	}
	oldPrepared := prepareAdoption(t, c)
	if err := c.ClosePrepared(ctx, oldPrepared, opID(t)); err != nil {
		t.Fatal(err)
	}
	replacement := newMemoryAtomic()
	k := &Kernel{Store: replacement, Witness: w, Lineage: s.kernel.Lineage, TableARN: s.kernel.TableARN + "-replacement"}
	restored, err := NewServer(k, s.config)
	if err != nil {
		t.Fatal(err)
	}
	newOwner, err := NewClient(localTransport{s: restored, principal: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if err := newOwner.RestoreAuthority(ctx, opID(t)); err != nil {
		t.Fatal(err)
	}
	inspected, err := restored.execute(ctx, "owner", Request{Action: "inspect"})
	if err != nil || inspected.Adoption == nil || *inspected.Adoption != a {
		t.Fatal("restored adoption changed")
	}
	hash, err := a.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := replacement.Get(ctx, "ADOPTION#"+hash); err != nil || !found {
		t.Fatal("restored adoption receipt missing")
	}
	runtime, err := NewClient(localTransport{s: restored, principal: "runtime", measurement: Measurement{ExecutableSHA256: strings.Repeat("a", 64), ProcessLifetime: "replacement"}})
	if err != nil {
		t.Fatal(err)
	}
	p := prepareAdoption(t, runtime)
	if !p.RequireEmptyData || p.Adoption == nil || *p.Adoption != a {
		t.Fatal("restored pending adoption bypassed")
	}
}
