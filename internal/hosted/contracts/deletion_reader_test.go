package contracts_test

import (
	"context"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

var (
	_ contracts.JournalObjectReader   = fakeJournalObjectStore{}
	_ contracts.DeletionJournalReader = fakeDeletionJournalReader{}
)

type fakeDeletionJournalReader struct{}

func (fakeDeletionJournalReader) Observe(context.Context) (contracts.JournalObservation, error) {
	return contracts.JournalObservation{}, nil
}
