package episodemaintenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/sirerun/serenity/internal/hosted/authority"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/deletion"
)

// Minimal in-memory AtomicStore/WitnessStore adapted from authority's existing
// transaction tests. These exercise the real Kernel, Server, Gateway and Client;
// they make no AWS persistence, transport or IAM qualification claim.
type memoryAtomic struct {
	mu           sync.Mutex
	r            authority.Record
	exists       bool
	items        map[string][]byte
	ops          map[string]authority.Receipt
	lostCommit   bool
	lostComplete bool
}

func newMemoryAtomic() *memoryAtomic {
	return &memoryAtomic{items: map[string][]byte{}, ops: map[string]authority.Receipt{}}
}
func clone[T any](x T) T { b, _ := json.Marshal(x); var y T; _ = json.Unmarshal(b, &y); return y }
func (m *memoryAtomic) Load(context.Context) (authority.Record, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return clone(m.r), m.exists, nil
}
func (m *memoryAtomic) Receipt(_ context.Context, id string) (authority.Receipt, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.ops[id]
	return clone(r), ok, nil
}
func (m *memoryAtomic) Commit(_ context.Context, rev uint64, r authority.Record, items []authority.ImmutableItem, op authority.Receipt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.r.Revision != rev || m.r.Pending != nil {
		return authority.ErrConflict
	}
	if _, ok := m.ops[op.OperationID]; ok {
		return authority.ErrConflict
	}
	for _, it := range items {
		if _, ok := m.items[it.Key]; ok {
			return authority.ErrConflict
		}
	}
	for _, it := range items {
		m.items[it.Key] = append([]byte(nil), it.Body...)
	}
	m.ops[op.OperationID] = clone(op)
	m.r = clone(r)
	m.exists = true
	if m.lostCommit {
		m.lostCommit = false
		return authority.ErrUnavailable
	}
	return nil
}
func (m *memoryAtomic) Complete(_ context.Context, r authority.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.r.Pending == nil || r.Pending == nil || m.r.Revision != r.Revision || m.r.Pending.OperationID != r.Pending.OperationID {
		return authority.ErrConflict
	}
	op := m.ops[r.Pending.OperationID]
	op.Complete = true
	m.ops[op.OperationID] = op
	m.r.Tip = r.Pending.Next
	m.r.Pending = nil
	if m.lostComplete {
		m.lostComplete = false
		return authority.ErrUnavailable
	}
	return nil
}
func (m *memoryAtomic) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.items[key]
	return append([]byte(nil), b...), ok, nil
}
func (m *memoryAtomic) List(_ context.Context, prefix, after string, n int) ([]string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var keys []string
	for k := range m.items {
		if strings.HasPrefix(k, prefix) && k > after {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	more := len(keys) > n
	if more {
		keys = keys[:n]
	}
	return keys, more, nil
}

type memoryWitness struct {
	mu    sync.Mutex
	tip   authority.WitnessTip
	items map[string][]byte
	fail  bool
}

func (m *memoryWitness) Tip(context.Context) (authority.WitnessTip, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tip, m.tip.Sequence > 0, nil
}
func (m *memoryWitness) Advance(_ context.Context, p authority.Pending) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return authority.ErrUnavailable
	}
	if m.tip == p.Next {
		return nil
	}
	if m.tip != p.Previous {
		return authority.ErrConflict
	}
	m.items[p.Next.Key] = append([]byte(nil), p.Witness...)
	m.tip = p.Next
	return nil
}
func (m *memoryWitness) Read(_ context.Context, t authority.WitnessTip) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.items[t.Key]
	if !ok || digest(b) != t.SHA256 {
		return nil, authority.ErrConflict
	}
	return append([]byte(nil), b...), nil
}

func (m *memoryAtomic) Import(_ context.Context, items []authority.ImmutableItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, it := range items {
		if old, ok := m.items[it.Key]; ok && string(old) != string(it.Body) {
			return authority.ErrConflict
		}
		if strings.HasPrefix(it.Key, "OP#") {
			var receipt authority.Receipt
			if err := json.Unmarshal(it.Body, &receipt); err != nil {
				return err
			}
			m.ops[receipt.OperationID] = receipt
		} else {
			m.items[it.Key] = append([]byte(nil), it.Body...)
		}
	}
	return nil
}

func (m *memoryAtomic) ListObjects(_ context.Context, prefix, after string, n int) ([]authority.ImmutableItem, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var keys []string
	for k := range m.items {
		if strings.HasPrefix(k, prefix) && k > after {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	more := len(keys) > n
	if more {
		keys = keys[:n]
	}
	out := make([]authority.ImmutableItem, len(keys))
	for i, key := range keys {
		out[i] = authority.ImmutableItem{Key: key, Body: append([]byte(nil), m.items[key]...)}
	}
	return out, more, nil
}

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

type gatewayTransport struct {
	gateway     *authority.Gateway
	binding     authority.GatewayBinding
	owner       bool
	measurement authority.Measurement
}

func (t gatewayTransport) Invoke(ctx context.Context, q authority.Request) (authority.Response, error) {
	if q.Action == "prepare" || q.Action == "renew" {
		m := t.measurement
		q.Measurement = &m
	}
	path, arn, caller := t.binding.RuntimePath, t.binding.RuntimeARN, t.binding.RuntimeCallerID
	if t.owner {
		path, arn, caller = t.binding.OwnerPath, t.binding.OwnerARN, t.binding.OwnerCallerID
	}
	body, err := json.Marshal(q)
	if err != nil {
		return authority.Response{}, err
	}
	response, err := t.gateway.Handle(ctx, events.APIGatewayProxyRequest{
		Resource: path, Path: path, HTTPMethod: "POST", Body: string(body),
		RequestContext: events.APIGatewayProxyRequestContext{APIID: t.binding.APIID, AccountID: t.binding.AccountID, Stage: t.binding.Stage, DomainName: t.binding.DomainName, HTTPMethod: "POST", RequestID: "synthetic-request", ResourcePath: path, Identity: events.APIGatewayRequestIdentity{UserArn: arn, Caller: caller}},
	})
	if err != nil || response.StatusCode != 200 {
		return authority.Response{}, authority.ErrUnavailable
	}
	var out authority.Response
	if err = json.Unmarshal([]byte(response.Body), &out); err != nil {
		return authority.Response{}, err
	}
	return out, nil
}

func operationID(t *testing.T) string {
	t.Helper()
	id, err := authority.NewOperationID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

type authorityFixture struct {
	client  *authority.Client
	owner   *authority.Client
	server  *authority.Server
	lease   authority.Lease
	now     time.Time
	store   *memoryAtomic
	witness *memoryWitness
}

func newAuthorityFixture(t *testing.T) *authorityFixture {
	t.Helper()
	f := &authorityFixture{now: time.Now().UTC().Truncate(time.Second), store: newMemoryAtomic(), witness: &memoryWitness{items: map[string][]byte{}}}
	kernel := &authority.Kernel{Store: f.store, Witness: f.witness, Lineage: "synthetic-lineage", TableARN: "arn:aws:dynamodb:eu-west-1:123456789012:table/synthetic"}
	measurement := authority.Measurement{ExecutableSHA256: digest([]byte("synthetic-binary")), ProcessLifetime: "synthetic-pid-start"}
	binding := authority.GatewayBinding{APIID: "synthetic-api", AccountID: "123456789012", Stage: "synthetic", DomainName: "synthetic.execute-api.test", RuntimePath: "/runtime", OwnerPath: "/owner", RuntimeARN: "arn:aws:iam::123456789012:role/runtime", RuntimeCallerID: "runtime-caller", OwnerARN: "arn:aws:iam::123456789012:role/owner", OwnerCallerID: "owner-caller"}
	server, err := authority.NewServer(kernel, authority.ServerConfig{RuntimePrincipal: binding.RuntimeARN, OwnerPrincipal: binding.OwnerARN, ApprovedBuilds: map[string]string{measurement.ExecutableSHA256: strings.Repeat("a", 40)}, LeaseLifetime: time.Hour, ChallengeLifetime: time.Minute, GenesisLifetime: time.Hour, Now: func() time.Time { return f.now }})
	if err != nil {
		t.Fatal(err)
	}
	f.server = server
	gateway, err := authority.NewGateway(server, binding)
	if err != nil {
		t.Fatal(err)
	}
	ownerTransport := gatewayTransport{gateway: gateway, binding: binding, owner: true}
	if _, err = ownerTransport.Invoke(context.Background(), authority.Request{Action: "enroll", OperationID: operationID(t)}); err != nil {
		t.Fatal(err)
	}
	f.owner, err = authority.NewClient(ownerTransport)
	if err != nil {
		t.Fatal(err)
	}
	f.client, err = authority.NewClient(gatewayTransport{gateway: gateway, binding: binding, measurement: measurement})
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := f.client.Challenge(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := f.client.Prepare(context.Background(), challenge)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := deletion.NewJournalReader(f.client.Reader(prepared), prepared.Position)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := reader.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	f.lease, err = f.client.Activate(context.Background(), prepared, observation.HistorySHA256, operationID(t))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestClosedSourceUsesCompleteCurrentHistoryWithoutCut(t *testing.T) {
	f := newAuthorityFixture(t)
	f.append(t, 1)
	cut, err := f.client.Cut(context.Background(), f.lease, operationID(t), operationID(t))
	if err != nil {
		t.Fatal(err)
	}
	f.append(t, 2)
	if err = f.owner.Close(context.Background(), f.lease, operationID(t)); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(2 * time.Hour) // historical cuts stay readable to their owner after expiry.
	closed, err := f.owner.ObserveClosed(context.Background())
	if err != nil || closed.Position.LastObjectInGeneration.SequenceID != 2 {
		t.Fatalf("current CLOSED position missing newer history: %+v %v", closed, err)
	}
	if _, err = f.client.ObserveClosed(context.Background()); err == nil {
		t.Fatal("runtime principal used owner-only CLOSED route")
	}
	source, err := NewClosedSource(f.owner)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := source.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Recheck == nil || snapshot.Position != closed || snapshot.Observation.Position != closed.Position || len(snapshot.Observation.Read.Entries) != 2 {
		t.Fatalf("wrong complete CLOSED history: %+v", snapshot)
	}
	if err = snapshot.Recheck(context.Background()); err != nil {
		t.Fatal(err)
	}
	oldReader, err := deletion.NewJournalReader(f.owner.CutReader(cut), cut.Position)
	if err != nil {
		t.Fatal(err)
	}
	old, err := oldReader.Observe(context.Background())
	if err != nil || len(old.Read.Entries) != 1 {
		t.Fatalf("expired historical cut lost its original scope: %+v %v", old, err)
	}
}

func TestClosedSourceRejectsInvalidPositionAndDetachesRecheckPin(t *testing.T) {
	f := newAuthorityFixture(t)
	if err := f.owner.Close(context.Background(), f.lease, operationID(t)); err != nil {
		t.Fatal(err)
	}
	source, err := NewClosedSource(f.owner)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := source.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Position.Epoch++
	if err = snapshot.Recheck(context.Background()); err != nil {
		t.Fatalf("public snapshot mutation changed private recheck pin: %v", err)
	}
	if _, err = source.Observe(nil); !errors.Is(err, ErrAuthorityUnavailable) {
		t.Fatalf("nil context accepted: %v", err)
	}
	if _, err = NewClosedSource((*authority.Client)(nil)); !errors.Is(err, ErrAuthorityUnavailable) {
		t.Fatalf("typed nil owner accepted: %v", err)
	}
}

func TestClosedSourceTraversesSealedGenerationAndEmptySuccessor(t *testing.T) {
	f := newAuthorityFixture(t)
	f.append(t, 1)
	current, err := f.client.Current(context.Background(), f.lease)
	if err != nil {
		t.Fatal(err)
	}
	seal, err := (contracts.JournalObject{Kind: contracts.JournalKindSeal, Generation: 1, SequenceID: 2, WriterID: f.lease.WriterID, PrevHash: current.Position.LastObjectInGeneration.EntryHash, RecordedAt: f.now}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err = f.owner.Seal(context.Background(), f.lease, contracts.JournalKey(1, 2), seal, operationID(t)); err != nil {
		t.Fatal(err)
	}
	position, err := f.owner.ObserveClosed(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	reader, err := deletion.NewJournalReader(f.owner.ClosedReader(position), position.Position)
	if err != nil {
		t.Fatal(err)
	}
	if observation, observeErr := reader.Observe(context.Background()); observeErr != nil || len(observation.Read.Entries) != 1 {
		t.Fatalf("journal observer rejected sealed generation and empty successor: %+v %v", observation, observeErr)
	}
	source, err := NewClosedSource(f.owner)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := source.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Position.Position.ActiveGeneration != 2 || snapshot.Position.Position.PredecessorSeal.SequenceID != 2 || !snapshot.Position.Position.LastObjectInGeneration.IsZero() || len(snapshot.Observation.Read.Entries) != 1 || !snapshot.Observation.Read.Sealed {
		t.Fatalf("sealed generation or empty successor misread: %+v", snapshot)
	}
}

func TestClosedSourceAcceptsPreFixSuccessorStateWithExactWitnessAnchor(t *testing.T) {
	f := newAuthorityFixture(t)
	f.append(t, 1)
	var prior struct {
		Position contracts.JournalPosition `json:"position"`
	}
	if err := json.Unmarshal(f.store.r.State, &prior); err != nil {
		t.Fatal(err)
	}
	legacyGenesis := prior.Position.GenesisIssuanceID
	current, err := f.client.Current(context.Background(), f.lease)
	if err != nil {
		t.Fatal(err)
	}
	seal, err := (contracts.JournalObject{Kind: contracts.JournalKindSeal, Generation: 1, SequenceID: 2, WriterID: f.lease.WriterID, PrevHash: current.Position.LastObjectInGeneration.EntryHash, RecordedAt: f.now}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err = f.owner.Seal(context.Background(), f.lease, contracts.JournalKey(1, 2), seal, operationID(t)); err != nil {
		t.Fatal(err)
	}

	// Recreate the durable shape produced by the old seal transition, including
	// a fresh witness whose exact state bytes and anchor agree with META.
	var state map[string]json.RawMessage
	if err = json.Unmarshal(f.store.r.State, &state); err != nil {
		t.Fatal(err)
	}
	var position contracts.JournalPosition
	if err = json.Unmarshal(state["position"], &position); err != nil {
		t.Fatal(err)
	}
	position.GenesisIssuanceID = legacyGenesis
	state["position"], err = json.Marshal(position)
	if err != nil {
		t.Fatal(err)
	}
	legacyState, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	type witnessEnvelope struct {
		Version  int                       `json:"version"`
		Previous authority.WitnessTip      `json:"previous"`
		Sequence uint64                    `json:"sequence"`
		Lineage  string                    `json:"lineage"`
		TableARN string                    `json:"table_arn"`
		Revision uint64                    `json:"revision"`
		State    json.RawMessage           `json:"state"`
		Items    []authority.ImmutableItem `json:"items"`
		Receipt  authority.Receipt         `json:"receipt"`
	}
	previous := f.store.r.Tip
	var old witnessEnvelope
	oldBody := f.witness.items[previous.Key]
	if err = json.Unmarshal(oldBody, &old); err != nil {
		t.Fatal(err)
	}
	record := old
	record.State = legacyState
	legacyBody, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	tip := authority.WitnessTip{Lineage: previous.Lineage, TableARN: previous.TableARN, Sequence: record.Sequence, TotalBytes: record.Previous.TotalBytes + int64(len(legacyBody)), SHA256: digest(legacyBody)}
	tip.Key = fmt.Sprintf("%020d/%s.json", tip.Sequence, tip.SHA256)
	f.store.mu.Lock()
	f.store.r.State = legacyState
	f.store.r.Tip = tip
	f.store.mu.Unlock()
	f.witness.mu.Lock()
	f.witness.items[tip.Key] = legacyBody
	f.witness.tip = tip
	f.witness.mu.Unlock()

	closed, err := f.owner.ObserveClosed(context.Background())
	if err != nil {
		t.Fatalf("pre-fix successor state was rejected: %v", err)
	}
	if closed.Position.ActiveGeneration != 2 || closed.Position.GenesisIssuanceID != "" || closed.Anchor != tip {
		t.Fatalf("legacy projection lost its exact anchor: %+v", closed)
	}
	source, err := NewClosedSource(f.owner)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := source.Observe(context.Background())
	if err != nil || len(snapshot.Observation.Read.Entries) != 1 || !snapshot.Observation.Read.Sealed {
		t.Fatalf("legacy CLOSED history was not readable: %+v %v", snapshot, err)
	}
}

func TestClosedSourceReadsAcrossBoundedPages(t *testing.T) {
	f := newAuthorityFixture(t)
	for sequence := int64(1); sequence <= 300; sequence++ {
		f.append(t, sequence)
	}
	if err := f.owner.Close(context.Background(), f.lease, operationID(t)); err != nil {
		t.Fatal(err)
	}
	source, err := NewClosedSource(f.owner)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := source.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Observation.Read.Entries) != 300 || snapshot.Observation.Position.LastObjectInGeneration.SequenceID != 300 || snapshot.Recheck == nil {
		t.Fatalf("bounded page continuation lost current history: entries=%d pos=%+v", len(snapshot.Observation.Read.Entries), snapshot.Observation.Position)
	}
}

func TestClosedSourceRefusesCanceledObservation(t *testing.T) {
	f := newAuthorityFixture(t)
	if err := f.owner.Close(context.Background(), f.lease, operationID(t)); err != nil {
		t.Fatal(err)
	}
	source, err := NewClosedSource(f.owner)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if snapshot, err := source.Observe(ctx); !errors.Is(err, ErrAuthorityUnavailable) || snapshot.Recheck != nil {
		t.Fatalf("canceled source returned usable authority: %+v %v", snapshot, err)
	}
}

func TestClosedSourceRejectsMissingMalformedOversizedAndExtraTailObjects(t *testing.T) {
	for _, scenario := range []string{"missing", "malformed", "oversized", "extra_tail"} {
		t.Run(scenario, func(t *testing.T) {
			f := newAuthorityFixture(t)
			f.append(t, 1)
			if err := f.owner.Close(context.Background(), f.lease, operationID(t)); err != nil {
				t.Fatal(err)
			}
			key := "OBJ#" + contracts.JournalKey(1, 1)
			f.store.mu.Lock()
			switch scenario {
			case "missing":
				delete(f.store.items, key)
			case "malformed":
				f.store.items[key] = []byte("not-json")
			case "oversized":
				f.store.items[key] = make([]byte, authority.MaxObjectBytes+1)
			case "extra_tail":
				f.store.items["OBJ#"+contracts.JournalKey(1, 2)] = []byte("unadmitted tail")
			}
			f.store.mu.Unlock()
			source, err := NewClosedSource(f.owner)
			if err != nil {
				t.Fatal(err)
			}
			if snapshot, err := source.Observe(context.Background()); !errors.Is(err, ErrAuthorityUnavailable) || snapshot.Recheck != nil || len(snapshot.Observation.Read.Entries) != 0 {
				t.Fatalf("invalid current history returned a usable snapshot: %+v %v", snapshot, err)
			}
		})
	}
}

func (f *authorityFixture) source(t *testing.T, a Authority) *source {
	t.Helper()
	s, err := NewSource(a, func() authority.Lease { return f.lease })
	if err != nil {
		t.Fatal(err)
	}
	actual := s.(*source)
	actual.now = func() time.Time { return f.now }
	return actual
}
func (f *authorityFixture) append(t *testing.T, sequence int64) {
	t.Helper()
	identity := contracts.EpisodeDeletionIdentity{BrainHash: contracts.EpisodeDeletionBrainHash("synthetic-brain"), ConversationHash: strings.Repeat("b", 64), CommandHash: strings.Repeat("c", 64), RequestDigest: strings.Repeat("d", 64)}
	object := contracts.JournalObject{Kind: contracts.JournalKindEntry, Generation: f.lease.Position.ActiveGeneration, SequenceID: sequence, WriterID: f.lease.WriterID, SubjectType: contracts.DeletionSubjectEpisodeConversation, SubjectID: identity.String(), Outcome: contracts.DeletionIntentRequested, RecordedAt: f.now}
	if sequence > 1 {
		current, err := f.client.Current(context.Background(), f.lease)
		if err != nil {
			t.Fatal(err)
		}
		object.PrevHash = current.Position.LastObjectInGeneration.EntryHash
	}
	body, err := object.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.client.Store(f.lease).PutIfAbsent(context.Background(), contracts.JournalKey(object.Generation, sequence), body); err != nil {
		t.Fatal(err)
	}
}

func TestAuthoritySourceRealServerClientCutAndRecheck(t *testing.T) {
	f := newAuthorityFixture(t)
	f.append(t, 1)
	s := f.source(t, f.client)
	snapshot, err := s.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CutID == "" || snapshot.Recheck == nil || len(snapshot.Observation.Read.Entries) != 1 || snapshot.Observation.Position.LastObjectInGeneration.SequenceID != 1 || snapshot.Observation.Read.Sealed {
		t.Fatalf("wrong active-prefix observation: %+v", snapshot)
	}
	if err = snapshot.Recheck(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Renew the existing issuance; local factory callback and authenticated
	// Current must agree. Renewal does not change journal position or identity.
	f.now = f.now.Add(time.Minute)
	f.lease, err = f.client.Renew(context.Background(), f.lease, operationID(t))
	if err != nil {
		t.Fatal(err)
	}
	if err = snapshot.Recheck(context.Background()); err != nil {
		t.Fatal("same-issuance renewal refused", err)
	}
	f.append(t, 2)
	if err = snapshot.Recheck(context.Background()); !errors.Is(err, ErrAuthorityUnavailable) {
		t.Fatalf("advanced authoritative history admitted: %v", err)
	}
}

type faultAuthority struct {
	Authority
	mutateCut     func(*authority.Cut)
	mutateCurrent func(*authority.Current)
	cutErr        error
	nilReader     bool
}

func (a *faultAuthority) Cut(ctx context.Context, l authority.Lease, attempt, op string) (authority.Cut, error) {
	if a.cutErr != nil {
		return authority.Cut{}, a.cutErr
	}
	cut, err := a.Authority.Cut(ctx, l, attempt, op)
	if err == nil && a.mutateCut != nil {
		a.mutateCut(&cut)
	}
	return cut, err
}
func (a *faultAuthority) Current(ctx context.Context, l authority.Lease) (authority.Current, error) {
	current, err := a.Authority.Current(ctx, l)
	if err == nil && a.mutateCurrent != nil {
		a.mutateCurrent(&current)
	}
	return current, err
}
func (a *faultAuthority) CutReader(c authority.Cut) contracts.JournalObjectReader {
	if a.nilReader {
		return nil
	}
	return a.Authority.CutReader(c)
}

func TestAuthoritySourceRefusesInvalidAuthenticatedResponses(t *testing.T) {
	for _, scenario := range []string{"cut_unknown", "cut_id", "lease_id", "attempt_id", "cut_expiry", "generation", "genesis", "anchor_missing", "anchor_key", "anchor_hash", "nil_reader", "current_epoch", "current_position", "current_expiry", "missing_history"} {
		t.Run(scenario, func(t *testing.T) {
			f := newAuthorityFixture(t)
			f.append(t, 1)
			a := &faultAuthority{Authority: f.client}
			switch scenario {
			case "cut_unknown":
				a.cutErr = errors.New("untrusted detail must not escape")
			case "cut_id":
				a.mutateCut = func(c *authority.Cut) { c.ID = "bad" }
			case "lease_id":
				a.mutateCut = func(c *authority.Cut) { c.LeaseID = strings.Repeat("e", 64) }
			case "attempt_id":
				a.mutateCut = func(c *authority.Cut) { c.SnapshotAttemptID = strings.Repeat("e", 64) }
			case "cut_expiry":
				a.mutateCut = func(c *authority.Cut) { c.ExpiresAt = f.now }
			case "generation":
				a.mutateCut = func(c *authority.Cut) { c.Position.ActiveGeneration++ }
			case "genesis":
				a.mutateCut = func(c *authority.Cut) { c.Position.GenesisIssuanceID = strings.Repeat("e", 64) }
			case "anchor_missing":
				a.mutateCut = func(c *authority.Cut) { c.Anchor = authority.WitnessTip{} }
			case "anchor_key":
				a.mutateCut = func(c *authority.Cut) { c.Anchor.Key = "../foreign" }
			case "anchor_hash":
				a.mutateCut = func(c *authority.Cut) { c.Anchor.SHA256 = strings.Repeat("A", 64) }
			case "nil_reader":
				a.nilReader = true
			case "current_epoch":
				a.mutateCurrent = func(c *authority.Current) { c.Epoch++ }
			case "current_position":
				a.mutateCurrent = func(c *authority.Current) { c.Position.LastObjectInGeneration = contracts.DeletionWatermark{} }
			case "current_expiry":
				a.mutateCurrent = func(c *authority.Current) { c.ExpiresAt = f.now }
			case "missing_history":
				f.store.mu.Lock()
				delete(f.store.items, "OBJ#"+contracts.JournalKey(1, 1))
				f.store.mu.Unlock()
			}
			snapshot, err := f.source(t, a).Observe(context.Background())
			if !errors.Is(err, ErrAuthorityUnavailable) || snapshot.Recheck != nil || snapshot.CutID != "" {
				t.Fatalf("unqualified response admitted: %v", err)
			}
			if strings.Contains(err.Error(), "untrusted detail") {
				t.Fatal("authority content escaped")
			}
		})
	}
}

func TestAuthoritySourceLeaseIdentityAndExpiryFailClosed(t *testing.T) {
	for _, scenario := range []string{"id", "reservation", "writer", "epoch", "build", "expiry", "cut_expiry", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			f := newAuthorityFixture(t)
			snapshot, err := f.source(t, f.client).Observe(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			switch scenario {
			case "id":
				f.lease.ID = strings.Repeat("e", 64)
			case "reservation":
				f.lease.ReservationID = strings.Repeat("e", 64)
			case "writer":
				f.lease.WriterID = strings.Repeat("e", 64)
			case "epoch":
				f.lease.Epoch++
			case "build":
				f.lease.BuildSHA = strings.Repeat("e", 40)
			case "expiry":
				f.lease.ExpiresAt = f.now
			case "cut_expiry":
				f.now = f.now.Add(2 * time.Hour)
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if err = snapshot.Recheck(ctx); !errors.Is(err, ErrAuthorityUnavailable) {
				t.Fatalf("invalid maintenance issuance admitted: %v", err)
			}
		})
	}
}

func TestAuthoritySourceConstructorAndInvalidLease(t *testing.T) {
	var typedNil *faultAuthority
	for _, a := range []Authority{nil, typedNil} {
		if s, err := NewSource(a, func() authority.Lease { return authority.Lease{} }); s != nil || !errors.Is(err, ErrAuthorityUnavailable) {
			t.Fatal("nil authority admitted")
		}
	}
	f := newAuthorityFixture(t)
	if s, err := NewSource(f.client, nil); s != nil || err == nil {
		t.Fatal("nil lease callback admitted")
	}
	f.lease.ID = "plaintext"
	if snapshot, err := f.source(t, f.client).Observe(context.Background()); snapshot.Recheck != nil || !errors.Is(err, ErrAuthorityUnavailable) {
		t.Fatal("invalid lease admitted")
	}
}
