// Package debversion reads and orders Debian package versions
// ([epoch:]upstream[-revision]) the way dpkg --compare-versions does, without
// running anything. It is shared by the Agent, the Panel and the recovery
// tool so they apply one rule.
//
// debversion, Debian paket sürümlerini dpkg --compare-versions ile aynı
// biçimde okur ve sıralar; hiçbir şey çalıştırmaz.
package debversion

import "strings"

// Version is a parsed Debian version.
type Version struct {
	Epoch    string // "" when absent
	Upstream string
	Revision string // "" when absent
}

// Valid reports whether value is a canonical Debian version string: a
// leading digit, only the characters dpkg allows, at most 128 bytes, a
// numeric epoch when there is one and an upstream part that is not empty.
func Valid(value string) bool {
	if value == "" || len(value) > 128 || value[0] < '0' || value[0] > '9' {
		return false
	}
	for _, r := range value {
		alnum := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
		if !alnum && r != '.' && r != '+' && r != '~' && r != ':' && r != '-' {
			return false
		}
	}
	v, ok := Parse(value)
	return ok && v.Upstream != "" && (v.Upstream[0] >= '0' && v.Upstream[0] <= '9')
}

// Parse splits a version into epoch, upstream and revision.
func Parse(value string) (Version, bool) {
	var v Version
	rest := value
	if colon := strings.IndexByte(rest, ':'); colon >= 0 {
		v.Epoch, rest = rest[:colon], rest[colon+1:]
		if v.Epoch == "" || !digits(v.Epoch) {
			return Version{}, false
		}
	}
	if dash := strings.LastIndexByte(rest, '-'); dash >= 0 {
		v.Upstream, v.Revision = rest[:dash], rest[dash+1:]
		if v.Revision == "" {
			return Version{}, false
		}
	} else {
		v.Upstream = rest
	}
	if v.Upstream == "" || strings.ContainsRune(v.Upstream, ':') {
		return Version{}, false
	}
	return v, true
}

// Compare orders two valid versions: -1, 0 or 1, as dpkg does.
func Compare(a, b string) int {
	va, okA := Parse(a)
	vb, okB := Parse(b)
	if !okA || !okB {
		return strings.Compare(a, b)
	}
	if c := compareNumber(orZero(va.Epoch), orZero(vb.Epoch)); c != 0 {
		return c
	}
	if c := compareFragment(va.Upstream, vb.Upstream); c != 0 {
		return c
	}
	return compareFragment(orZero(va.Revision), orZero(vb.Revision))
}

func orZero(value string) string {
	if value == "" {
		return "0"
	}
	return value
}

func digits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func compareNumber(a, b string) int {
	a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

// order is dpkg's character weight: '~' sorts before everything, even the
// end of the part; letters before other characters.
func order(c byte) int {
	switch {
	case c == '~':
		return -1
	case c >= '0' && c <= '9':
		return 0
	case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
		return int(c)
	default:
		return int(c) + 256
	}
}

func compareFragment(a, b string) int {
	for a != "" || b != "" {
		// Non-digit prefix.
		for (a != "" && (a[0] < '0' || a[0] > '9')) || (b != "" && (b[0] < '0' || b[0] > '9')) {
			ac, bc := 0, 0
			if a != "" && (a[0] < '0' || a[0] > '9') {
				ac = order(a[0])
			}
			if b != "" && (b[0] < '0' || b[0] > '9') {
				bc = order(b[0])
			}
			if ac != bc {
				if ac < bc {
					return -1
				}
				return 1
			}
			if a != "" && (a[0] < '0' || a[0] > '9') {
				a = a[1:]
			}
			if b != "" && (b[0] < '0' || b[0] > '9') {
				b = b[1:]
			}
		}
		// Digit run.
		i := 0
		for i < len(a) && a[i] >= '0' && a[i] <= '9' {
			i++
		}
		j := 0
		for j < len(b) && b[j] >= '0' && b[j] <= '9' {
			j++
		}
		if c := compareNumber(orZero(a[:i]), orZero(b[:j])); c != 0 {
			return c
		}
		a, b = a[i:], b[j:]
	}
	return 0
}
