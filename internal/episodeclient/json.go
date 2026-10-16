package episodeclient

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
)

// strictJSON rejects duplicate keys (including case aliases accepted by Go's
// struct decoder), excessive nesting, and unpaired UTF-16 escapes. It prevents
// silent replacement or last-key-wins normalization in imported evidence.
func strictJSON(b []byte) bool {
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
			} // Normalize ASCII case; schema keys are ASCII.
			lower := []byte(k)
			for i, c := range lower {
				if c >= 'A' && c <= 'Z' {
					lower[i] = c + ('a' - 'A')
				}
			}
			k = string(lower)
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
