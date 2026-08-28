package ledger

import (
	"context"
	"sort"
	"sync"
)

// memoryStore is a minimal in-memory Store used only by this package's own
// tests, to exercise the vendored reader/writer contract (Add, Get, List,
// Put, Delete, ReadOnly) without a filesystem. It is not vendored from
// upstream -- dira's own filesystem-backed implementation
// (internal/ledger/local) is deliberately not part of this vendor: T3.3
// wires a Store implementation through Serenity's own writer queue instead
// (docs/adr/008-precepts-on-dira-applies-when-in-body.md).
type memoryStore struct {
	mu      sync.Mutex
	entries map[string]*Entry
	version int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{entries: make(map[string]*Entry)}
}

func (s *memoryStore) nextVersion() string {
	s.version++
	return string(rune('a' + s.version))
}

func (s *memoryStore) Get(_ context.Context, id string) (*Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *e
	return &cp, nil
}

func (s *memoryStore) List(_ context.Context) ([]EntryInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	infos := make([]EntryInfo, 0, len(s.entries))
	for id, e := range s.entries {
		infos = append(infos, EntryInfo{ID: id, Version: e.version})
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].ID < infos[j].ID })
	return infos, nil
}

func (s *memoryStore) Create(_ context.Context, e *Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.entries[e.ID]; ok {
		return ErrExists
	}
	cp := *e
	cp.version = s.nextVersion()
	s.entries[e.ID] = &cp
	return nil
}

func (s *memoryStore) Put(_ context.Context, e *Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *e
	cp.version = s.nextVersion()
	s.entries[e.ID] = &cp
	return nil
}

func (s *memoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.entries[id]; !ok {
		return ErrNotFound
	}
	delete(s.entries, id)
	return nil
}
