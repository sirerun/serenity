package direction

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/kazi-org/dira/engine"
	"github.com/kazi-org/dira/ledger"
)

type lexicalFixture struct {
	entries []*ledger.Entry
	listErr error
	getErr  error
	listed  int
	got     int
	cancel  context.CancelFunc
}

func (s *lexicalFixture) List(context.Context) ([]ledger.EntryInfo, error) {
	s.listed++
	if s.listErr != nil {
		return nil, s.listErr
	}
	out := make([]ledger.EntryInfo, len(s.entries))
	for i, e := range s.entries {
		out[i].ID = e.ID
	}
	return out, nil
}

func (s *lexicalFixture) Get(_ context.Context, id string) (*ledger.Entry, error) {
	s.got++
	if s.cancel != nil {
		s.cancel()
	}
	if s.getErr != nil {
		return nil, s.getErr
	}
	for _, e := range s.entries {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, nil
}

type canonicalLexicalEntries []*ledger.Entry

func (e canonicalLexicalEntries) Entries(context.Context) ([]*ledger.Entry, error) { return e, nil }

func TestLexicalAdapterUsesCanonicalEngine(t *testing.T) {
	entries := []*ledger.Entry{{ID: "dec-0001", Kind: ledger.KindDecision,
		State: ledger.StateAccepted, Title: "Keep the tool foreground only", Private: true,
		Alternatives: []ledger.Alternative{{Option: "background daemon", WhyNot: "private rationale", RevisitIf: "private trigger"}},
	}}
	plan := "add a background daemon to track run state"
	want, err := engine.Check(context.Background(), canonicalLexicalEntries(entries), plan)
	if err != nil {
		t.Fatal(err)
	}
	reader := &lexicalFixture{entries: entries}
	got, err := CheckLexical(context.Background(), reader, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) || reader.listed != 1 || reader.got != 1 {
		t.Fatalf("canonical result or ledger read contract changed: got=%+v want=%+v", got, want)
	}
	if got.ExitCode != engine.ExitConflict || len(got.Conflicts) == 0 {
		t.Fatal("fixture must exercise a real conflict")
	}
	for _, c := range got.Conflicts {
		if !c.Private || c.Title != "" || c.Alternative != "" || c.WhyNot != "" || c.RevisitIf != nil {
			t.Fatal("private citation text escaped canonical rendering")
		}
	}
}

func TestLexicalAdapterErrorsNeverBecomeCleanVerdicts(t *testing.T) {
	sentinel := errors.New("synthetic read failure")
	for _, reader := range []*lexicalFixture{
		{listErr: sentinel},
		{entries: []*ledger.Entry{{ID: "dec-0001"}}, getErr: sentinel},
	} {
		got, err := CheckLexical(context.Background(), reader, "add daemon")
		if got != nil || !errors.Is(err, sentinel) {
			t.Fatal("read failure lost or converted to verdict")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	reader := &lexicalFixture{entries: []*ledger.Entry{{ID: "dec-0001"}}, cancel: cancel}
	got, err := CheckLexical(ctx, reader, "add daemon")
	if got != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("late cancellation converted to verdict")
	}
	reader = &lexicalFixture{}
	if got, err := CheckLexical(context.Background(), reader, " \t"); got != nil || err == nil || reader.listed != 0 {
		t.Fatal("blank plan read ledger or produced verdict")
	}
}
