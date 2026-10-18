package writer

import "errors"

// ErrCanonicalViewUnavailable refuses canonical disclosure when the stable
// reader/maintenance exclusion cannot be established. It is never permission to
// fall back to an unfenced file read.
var ErrCanonicalViewUnavailable = errors.New("writer: canonical view unavailable")

const canonicalViewLockName = "canonical-view.lock"
