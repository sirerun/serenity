package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// EpisodeDeletionIdentity contains only domain-separated opaque brain identity
// and the existing content-free archive identities. It never contains input IDs.
// Command and digest make replay preserve the original exclusion receipt.
type EpisodeDeletionIdentity struct {
	BrainHash        string
	ConversationHash string
	CommandHash      string
	RequestDigest    string
}

func EpisodeDeletionBrainHash(brain string) string {
	sum := sha256.Sum256([]byte("serenity:episode-deletion:brain:v1\x00" + brain))
	return hex.EncodeToString(sum[:])
}

func (id EpisodeDeletionIdentity) String() string {
	return "v1:" + id.BrainHash + ":" + id.ConversationHash + ":" + id.CommandHash + ":" + id.RequestDigest
}

// ParseEpisodeDeletionIdentity rejects alternate spellings, versions, paths,
// plaintext IDs and partial identities. SubjectID's grammar is wire-frozen.
func ParseEpisodeDeletionIdentity(raw string) (EpisodeDeletionIdentity, error) {
	if len(raw) != 262 {
		return EpisodeDeletionIdentity{}, fmt.Errorf("%w: episode identity length", ErrDeletionEntryInvalid)
	}
	parts := strings.Split(raw, ":")
	if len(parts) != 5 || parts[0] != "v1" {
		return EpisodeDeletionIdentity{}, fmt.Errorf("%w: episode identity version or shape", ErrDeletionEntryInvalid)
	}
	for _, part := range parts[1:] {
		b, err := hex.DecodeString(part)
		if err != nil || len(b) != sha256.Size || hex.EncodeToString(b) != part {
			return EpisodeDeletionIdentity{}, fmt.Errorf("%w: episode identity must contain canonical SHA-256 hashes", ErrDeletionEntryInvalid)
		}
	}
	return EpisodeDeletionIdentity{parts[1], parts[2], parts[3], parts[4]}, nil
}
