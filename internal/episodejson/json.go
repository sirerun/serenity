// Package episodejson validates bounded episode JSON without ambiguous normalization.
package episodejson

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"unicode/utf8"
)

// Valid rejects duplicate keys (including case aliases accepted by Go's
// struct decoder), excessive nesting, and unpaired UTF-16 escapes. It prevents
// silent replacement or last-key-wins normalization in imported evidence.
func Valid(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	for i := 0; i < len(b); i++ {
		if b[i] != '"' {
			continue
		}
		i++
		for i < len(b) && b[i] != '"' {
			if b[i] == '\\' {
				i++
				if i >= len(b) {
					return false
				}
				if b[i] == 'u' {
					if i+4 >= len(b) {
						return false
					}
					n, e := strconv.ParseUint(string(b[i+1:i+5]), 16, 16)
					if e != nil {
						return false
					}
					i += 4
					if n >= 0xDC00 && n <= 0xDFFF {
						return false
					}
					if n >= 0xD800 && n <= 0xDBFF {
						if i+6 >= len(b) || b[i+1] != '\\' || b[i+2] != 'u' {
							return false
						}
						low, e := strconv.ParseUint(string(b[i+3:i+7]), 16, 16)
						if e != nil || low < 0xDC00 || low > 0xDFFF {
							return false
						}
						i += 6
					}
				}
			}
			i++
		}
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if !jsonValue(dec, 0) {
		return false
	}
	_, e := dec.Token()
	return e == io.EOF
}
func jsonValue(dec *json.Decoder, depth int) bool {
	if depth > 16 {
		return false
	}
	tok, e := dec.Token()
	if e != nil {
		return false
	}
	d, ok := tok.(json.Delim)
	if !ok {
		return true
	}
	switch d {
	case '{':
		seen := map[string]bool{}
		for dec.More() {
			t, e := dec.Token()
			if e != nil {
				return false
			}
			k, ok := t.(string)
			if !ok {
				return false
			}
			// Normalized schema keys are lowercase ASCII. Reject Go's
			// case-folding aliases, including non-ASCII long-s/Kelvin aliases.
			if k == "" {
				return false
			}
			for _, c := range k {
				if !(c >= 'a' && c <= 'z' || c == '_') {
					return false
				}
			}

			if seen[k] {
				return false
			}
			seen[k] = true
			if !jsonValue(dec, depth+1) {
				return false
			}
		}
		end, e := dec.Token()
		return e == nil && end == json.Delim('}')
	case '[':
		for dec.More() {
			if !jsonValue(dec, depth+1) {
				return false
			}
		}
		end, e := dec.Token()
		return e == nil && end == json.Delim(']')
	default:
		return false
	}
}
