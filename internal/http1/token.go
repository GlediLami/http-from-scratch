package http1

var tchar [256]bool

// method names and header fields must be tokens. 256 bool is O(1) lookup per byte and better than switch.
// fills once per startup on init
func init() {
	const special = "!#$%&'*+-.^_`|~"

	for c := 0; c < 256; c++ {
		if c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' {
			tchar[c] = true
		}
	}

	for i := 0; i < len(special); i++ {
		tchar[special[i]] = true
	}
}

// rejects emoty method and anything with non token byte so host (with space) fails.
func isToken(s string) bool {
	if s == "" {
		return false
	}

	for i := 0; i < len(s); i++ {
		if !tchar[s[i]] {
			return false
		}
	}

	return true
}

// rejects cr, lf and nuk in headers.
func validFieldValue(v string) bool {
	for i := 0; i < len(v); i++ {
		if c := v[i]; c == 0 || c == '\r' || c == '\n' {
			return false
		}
	}
	return true
}
