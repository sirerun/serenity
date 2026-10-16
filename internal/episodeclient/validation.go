package episodeclient

import "unicode/utf8"

// ValidJSON rejects malformed UTF-8, duplicate/case-aliased keys, excessive
// nesting and invalid surrogate escapes before classifying imported evidence.
// It performs no policy admission and does not make its payload trusted.
func ValidJSON(raw []byte) bool { return utf8.Valid(raw) && strictJSON(raw) }
