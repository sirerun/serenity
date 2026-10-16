package episodeclient

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"unicode/utf8"
)

// ValidJSON rejects malformed UTF-8, duplicate/case-aliased keys, excessive
// nesting and invalid surrogate escapes before classifying imported evidence.
// It performs no policy admission and does not make its payload trusted.
func ValidJSON(raw []byte) bool { return utf8.Valid(raw) && strictJSON(raw) }

// ConversationFenceKey is the content-free protocol identity for a stream.
// The complete server fence set can be retained without reversing its hashes.
func ConversationFenceKey(producer, conversation string) string {
	raw, _ := json.Marshal([]string{producer, conversation})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func validFenceKey(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
