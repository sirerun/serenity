package entities

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// newID returns a random 32-hex-char id -- the same shape and generation
// method internal/index's newJobID and internal/disposition's newEventID
// use (crypto/rand keeps this package stdlib-only per ADR 003). Merge
// event ids are not content-derived (unlike claim ids, ADR 004): two
// merges of the same pair at different times are different events, each
// independently undoable.
func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read failing means the OS entropy source is broken --
		// every other ID generator in this codebase (disposition.newEventID,
		// index.newJobID) treats this the same way: a panic, not a
		// swallowed error returning a zero-value id that would silently
		// collide.
		panic(fmt.Sprintf("entities: crypto/rand: %v", err))
	}
	return hex.EncodeToString(b)
}
