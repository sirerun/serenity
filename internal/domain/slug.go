package domain

import "regexp"

// slugRE is the canonical slug grammar: lowercase ASCII alphanumerics and
// hyphens, 1-64 characters, never starting or ending with a hyphen. It is
// the only shape an entity slug or entity type may take on disk, because
// both become a path segment under brain/entities/ and brain/claims/ and a
// YAML frontmatter value (RFC 0001 §7.1, §7.2; deep review SEC-H03).
var slugRE = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)

// ValidSlug reports whether s is a canonical entity slug or entity type.
// Every path from a model-emitted subject to a file name or frontmatter
// key checks it: the extraction candidate filter, the ingest path builder
// and the fence writer. Nothing escapes or repairs a value that fails --
// it is refused, so a newline, colon, dot-segment, backslash or non-ASCII
// character can never become a directory name or a YAML key.
func ValidSlug(s string) bool { return slugRE.MatchString(s) }
