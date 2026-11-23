package membership

// PHP behaviour the member service depends on, ported so the Go service accepts,
// refuses and prints exactly what portal-membership/ (PHP 8.3) does.
//
// Sources read on 2026-11-23 from php-src (tags php-8.3.6, the Ubuntu 24.04 build the
// CI job runs, and php-8.3.28): ext/filter/logical_filters.c (FILTER_VALIDATE_EMAIL,
// FILTER_VALIDATE_IP with FILTER_FLAG_NO_PRIV_RANGE|FILTER_FLAG_NO_RES_RANGE) and
// ext/standard/html.c (htmlspecialchars, ENT_QUOTES|ENT_SUBSTITUTE, UTF-8); glibc's
// inet_ntop for the text PHP's inet_ntop() returns; main/php_variables.c for how
// $_GET/$_POST are filled. Every function here is checked against the PHP binary by
// the comparison tests (php_parity_test.go) when CELIKPANEL_MEMBERSHIP_PHP is set.
//
// PHP-davranisi: Go hizmeti PHP 8.3 ile ayni seyi kabul eder, reddeder ve yazar.

import (
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

// phpTrim is PHP's trim() with its default characters " \t\n\r\0\x0B".
func phpTrim(s string) string { return strings.Trim(s, " \t\n\r\x00\x0B") }

// phpLower is PHP 8's strtolower(): ASCII only, whatever the locale.
func phpLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// htmlspecialchars($v, ENT_QUOTES|ENT_SUBSTITUTE, 'UTF-8')

// htmlEscape replaces & " ' < > like htmlspecialchars with ENT_QUOTES (HTML 4.01:
// the apostrophe is &#039;) and replaces each invalid UTF-8 sequence by U+FFFD the
// way html.c's get_next_char counts it (UTR #36 strategy 2).
func htmlEscape(s string) string {
	var out strings.Builder
	out.Grow(len(s) + 16)
	n := len(s)
	lead := func(c byte) bool { return c < 0x80 || (c >= 0xC2 && c <= 0xF4) }
	trail := func(c byte) bool { return c >= 0x80 && c <= 0xBF }
	for pos := 0; pos < n; {
		c := s[pos]
		switch {
		case c < 0x80:
			switch c {
			case '&':
				out.WriteString("&amp;")
			case '"':
				out.WriteString("&quot;")
			case '\'':
				out.WriteString("&#039;")
			case '<':
				out.WriteString("&lt;")
			case '>':
				out.WriteString("&gt;")
			default:
				out.WriteByte(c)
			}
			pos++
			continue
		case c < 0xC2:
			out.WriteString("�")
			pos++
			continue
		case c < 0xE0:
			if n-pos < 2 {
				out.WriteString("�")
				pos++
				continue
			}
			if !trail(s[pos+1]) {
				out.WriteString("�")
				if lead(s[pos+1]) {
					pos++
				} else {
					pos += 2
				}
				continue
			}
			// c >= 0xC2 makes the value at least 0x80: never non-shortest.
			out.WriteString(s[pos : pos+2])
			pos += 2
			continue
		case c < 0xF0:
			avail := n - pos
			if avail < 3 || !trail(s[pos+1]) || !trail(s[pos+2]) {
				out.WriteString("�")
				switch {
				case avail < 2 || lead(s[pos+1]):
					pos++
				case avail < 3 || lead(s[pos+2]):
					pos += 2
				default:
					pos += 3
				}
				continue
			}
			v := rune(c&0x0F)<<12 | rune(s[pos+1]&0x3F)<<6 | rune(s[pos+2]&0x3F)
			if v < 0x800 || (v >= 0xD800 && v <= 0xDFFF) {
				out.WriteString("�")
				pos += 3
				continue
			}
			out.WriteString(s[pos : pos+3])
			pos += 3
			continue
		case c < 0xF5:
			avail := n - pos
			if avail < 4 || !trail(s[pos+1]) || !trail(s[pos+2]) || !trail(s[pos+3]) {
				out.WriteString("�")
				switch {
				case avail < 2 || lead(s[pos+1]):
					pos++
				case avail < 3 || lead(s[pos+2]):
					pos += 2
				case avail < 4 || lead(s[pos+3]):
					pos += 3
				default:
					pos += 4
				}
				continue
			}
			v := rune(c&0x07)<<18 | rune(s[pos+1]&0x3F)<<12 | rune(s[pos+2]&0x3F)<<6 | rune(s[pos+3]&0x3F)
			if v < 0x10000 || v > 0x10FFFF {
				out.WriteString("�")
				pos += 4
				continue
			}
			out.WriteString(s[pos : pos+4])
			pos += 4
			continue
		default:
			out.WriteString("�")
			pos++
		}
	}
	return out.String()
}

// ---------------------------------------------------------------------------
// filter_var($v, FILTER_VALIDATE_EMAIL) without FILTER_FLAG_EMAIL_UNICODE.
//
// PHP matches one PCRE pattern (regexp1 in logical_filters.c, flags /iD). Go's RE2
// has no look-ahead, so the three look-aheads are computed here by hand and the rest
// is split into a local part (everything before the last "@"; the domain alternatives
// cannot contain "@") and a domain.

const (
	emailAtom   = `[\x21\x23-\x27\x2A\x2B\x2D\x2F-\x39\x3D\x3F\x41-\x5A\x5E-\x7E]+`
	emailQuoted = `"(?:[\x01-\x08\x0B\x0C\x0E-\x1F\x21\x23-\x5B\x5D-\x7F]|\\[\x00-\x7F])*"`
	emailHex    = `[a-fA-F0-9]{1,4}`
	emailIPv4   = `(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])(?:\.(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])){3}`
)

var (
	emailLocal = regexp.MustCompile(`^(?:` + emailAtom + `|` + emailQuoted + `)(?:\.(?:` + emailAtom + `|` + emailQuoted + `))*$`)
	// Labels and the last label, case-insensitive in PHP (/i): A-Z written out, never
	// Go's (?i), which would also fold the Kelvin sign and the long s.
	emailHost    = regexp.MustCompile(`^(?:[a-zA-Z0-9]+(?:-+[a-zA-Z0-9]+)*\.)+[a-zA-Z][a-zA-Z0-9]*(?:-+[a-zA-Z0-9]+)*$`)
	emailV6Full  = regexp.MustCompile(`^` + emailHex + `(?::` + emailHex + `){7}$`)
	emailV6Comp  = regexp.MustCompile(`^(?:` + emailHex + `(?::` + emailHex + `){0,5})?::(?:` + emailHex + `(?::` + emailHex + `){0,5})?$`)
	emailV6Six   = regexp.MustCompile(`^` + emailHex + `(?::` + emailHex + `){5}:` + emailIPv4 + `$`)
	emailV6Comp4 = regexp.MustCompile(`^(?:` + emailHex + `(?::` + emailHex + `){0,3})?::(?:` + emailHex + `(?::` + emailHex + `){0,3}:)?` + emailIPv4 + `$`)
	emailV4Only  = regexp.MustCompile(`^` + emailIPv4 + `$`)
)

// emailUnits answers, for every prefix s[:p], the largest number of the look-ahead's
// units that cover it exactly (-1: none). A unit is `"?\\[\x00-\x7E]"?` or
// `"?[^\\"]"?` (bytes, as PCRE without /u reads them).
func emailUnits(s string) []int {
	best := make([]int, len(s)+1)
	for i := range best {
		best[i] = -1
	}
	best[0] = 0
	for i := 0; i < len(s); i++ {
		if best[i] < 0 {
			continue
		}
		for lead := 0; lead <= 1; lead++ {
			j := i
			if lead == 1 {
				if s[j] != '"' {
					continue
				}
				j++
			}
			var ends []int
			if j+1 < len(s) && s[j] == '\\' && s[j+1] <= 0x7E {
				ends = append(ends, j+2)
			}
			if j < len(s) && s[j] != '\\' && s[j] != '"' {
				ends = append(ends, j+1)
			}
			for _, e := range ends {
				for _, end := range []int{e, e + 1} {
					if end > e && (e >= len(s) || s[e] != '"') {
						continue
					}
					if best[i]+1 > best[end] {
						best[end] = best[i] + 1
					}
				}
			}
		}
	}
	return best
}

// countPairs counts the non-overlapping places in s where a hex digit is followed
// by one of the bytes in next (the `(?:.*[a-f0-9][:\]]){n,}` look-aheads).
func countPairs(s string, next string) int {
	n := 0
	for i := 0; i+1 < len(s); i++ {
		c := s[i]
		if ((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) && strings.IndexByte(next, s[i+1]) >= 0 {
			n++
			i++
		}
	}
	return n
}

func hasIPv6Prefix(s string) bool {
	return len(s) >= 5 && (s[0]|0x20) == 'i' && (s[1]|0x20) == 'p' && (s[2]|0x20) == 'v' && s[3] == '6' && s[4] == ':'
}

// validEmail is filter_var($s, FILTER_VALIDATE_EMAIL) !== false.
func validEmail(s string) bool {
	if len(s) > 320 {
		return false
	}
	units := emailUnits(s)
	for p, n := range units {
		if n >= 255 {
			return false // (?!(?:unit){255,})
		}
		if p < len(s) && s[p] == '@' && n >= 65 {
			return false // (?!(?:unit){65,}@)
		}
	}
	at := strings.LastIndexByte(s, '@')
	if at < 0 || !emailLocal.MatchString(s[:at]) {
		return false
	}
	domain := s[at+1:]
	if hostnameDomain(domain) {
		return true
	}
	if len(domain) < 2 || domain[0] != '[' || domain[len(domain)-1] != ']' {
		return false
	}
	inner := domain[1 : len(domain)-1]
	if hasIPv6Prefix(inner) {
		rest := inner[5:]
		after := domain[6:] // the look-aheads run to the end of the subject, "]" included
		if emailV6Full.MatchString(rest) {
			return true
		}
		if countPairs(after, ":]") < 7 && emailV6Comp.MatchString(rest) {
			return true
		}
		if emailV6Six.MatchString(rest) {
			return true
		}
		if countPairs(after, ":") < 5 && emailV6Comp4.MatchString(rest) {
			return true
		}
		return false
	}
	return emailV4Only.MatchString(inner)
}

// hostnameDomain is the domain-name alternative with its (?!.*[^.]{64,}) look-ahead.
func hostnameDomain(d string) bool {
	run := 0
	for i := 0; i < len(d); i++ {
		if d[i] == '\n' {
			break // `.*` stops at a line feed; the pattern below refuses it anyway
		}
		if d[i] == '.' {
			run = 0
			continue
		}
		run++
		if run >= 64 {
			return false
		}
	}
	return emailHost.MatchString(d)
}

// ---------------------------------------------------------------------------
// The panel's address: Service.php publicIP().
//
// PHP: inet_pton (glibc), IPv4-mapped IPv6 reduced to IPv4, inet_ntop (glibc), then
// filter_var(FILTER_VALIDATE_IP, FILTER_FLAG_NO_PRIV_RANGE|FILTER_FLAG_NO_RES_RANGE).

// publicIP returns the address text PHP would store, or invalid_server_ip.
func publicIP(observed string) (string, error) {
	addr, err := netip.ParseAddr(observed)
	if err != nil || addr.Zone() != "" {
		return "", ErrInvalidServerIP
	}
	if addr.Is4In6() {
		addr = addr.Unmap()
	}
	if !publicRange(addr) {
		return "", ErrInvalidServerIP
	}
	return inetNtop(addr), nil
}

// publicRange applies FILTER_FLAG_NO_PRIV_RANGE|FILTER_FLAG_NO_RES_RANGE as PHP 8.3.6
// does. PHP 8.3.28 (RFC 6890 tables) gives the same IPv4 answers; for IPv6 it no
// longer refuses 005f::/16, 2001:db8::/32, 2001:10::/28 and 3ff3::/16. None of these
// is routable on the internet; this port keeps refusing them (the stricter answer and
// the one the CI's PHP gives), recorded as a known difference in the PR.
func publicRange(addr netip.Addr) bool {
	if addr.Is4() {
		ip := addr.As4()
		switch {
		case ip[0] == 10, ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31, ip[0] == 192 && ip[1] == 168:
			return false
		case ip[0] == 0, ip[0] >= 240, ip[0] == 127, ip[0] == 169 && ip[1] == 254:
			return false
		}
		return true
	}
	b := addr.As16()
	var w [8]int
	for i := range w {
		w[i] = int(b[2*i])<<8 | int(b[2*i+1])
	}
	if w[0] >= 0xfc00 && w[0] <= 0xfdff {
		return false
	}
	zero6 := w[0] == 0 && w[1] == 0 && w[2] == 0 && w[3] == 0 && w[4] == 0 && w[5] == 0 && w[6] == 0
	if (zero6 && (w[7] == 0 || w[7] == 1)) || w[0] == 0x5f || (w[0] >= 0xfe80 && w[0] <= 0xfebf) ||
		(w[0] == 0x2001 && (w[1] == 0x0db8 || (w[1] >= 0x0010 && w[1] <= 0x001f))) || w[0] == 0x3ff3 {
		return false
	}
	return true
}

// inetNtop is glibc's inet_ntop: dotted IPv4; for IPv6 the first longest run of two
// or more zero words becomes "::", and ::a.b.c.d / ::ffff:a.b.c.d keep a dotted tail.
// Go's netip prints ::a.b.c.d as ::102:304, so it cannot be used for IPv6.
func inetNtop(addr netip.Addr) string {
	if addr.Is4() {
		return addr.String()
	}
	b := addr.As16()
	var w [8]int
	for i := range w {
		w[i] = int(b[2*i])<<8 | int(b[2*i+1])
	}
	bestBase, bestLen, curBase, curLen := -1, 0, -1, 0
	for i := 0; i < 8; i++ {
		if w[i] == 0 {
			if curBase == -1 {
				curBase, curLen = i, 1
			} else {
				curLen++
			}
			continue
		}
		if curBase != -1 {
			if bestBase == -1 || curLen > bestLen {
				bestBase, bestLen = curBase, curLen
			}
			curBase = -1
		}
	}
	if curBase != -1 && (bestBase == -1 || curLen > bestLen) {
		bestBase, bestLen = curBase, curLen
	}
	if bestBase != -1 && bestLen < 2 {
		bestBase = -1
	}
	var out strings.Builder
	for i := 0; i < 8; i++ {
		if bestBase != -1 && i >= bestBase && i < bestBase+bestLen {
			if i == bestBase {
				out.WriteByte(':')
			}
			continue
		}
		if i != 0 {
			out.WriteByte(':')
		}
		if i == 6 && bestBase == 0 && (bestLen == 6 || (bestLen == 5 && w[5] == 0xffff)) {
			out.WriteString(strconv.Itoa(int(b[12])) + "." + strconv.Itoa(int(b[13])) + "." +
				strconv.Itoa(int(b[14])) + "." + strconv.Itoa(int(b[15])))
			return out.String()
		}
		out.WriteString(strconv.FormatInt(int64(w[i]), 16))
	}
	if bestBase != -1 && bestBase+bestLen == 8 {
		out.WriteByte(':')
	}
	return out.String()
}

// ---------------------------------------------------------------------------
// $_GET and $_POST.

// phpValue is one entry of $_GET/$_POST: a string, or an array (PHP builds one for
// a name with brackets, "id[]=x"). The member pages only ever read strings; an array
// is carried so the same refusals and the same "Array" text follow.
type phpValue struct {
	array bool
	str   string
}

type phpVars map[string]phpValue

// str is $vars[name] with the empty string when absent; the caller already refused arrays.
func (v phpVars) str(name string) string { return v[name].str }

// text is (string)$value: an array reads as "Array".
func (v phpValue) text() string {
	if v.array {
		return "Array"
	}
	return v.str
}

// phpURLDecode is PHP's urldecode: "+" is a space, "%XX" a byte, a lone "%" stays.
func phpURLDecode(s string) string {
	if !strings.ContainsAny(s, "+%") {
		return s
	}
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '+':
			out = append(out, ' ')
		case c == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]):
			out = append(out, unhex(s[i+1])<<4|unhex(s[i+2]))
			i += 2
		default:
			out = append(out, c)
		}
	}
	return string(out)
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func unhex(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	}
	return c - 'A' + 10
}

// parsePHPQuery fills a $_GET/$_POST-like map from an urlencoded string the way
// php_default_treat_data and php_register_variable_ex do for the names the member
// pages use: pairs split on "&", the last plain value wins, a name is cut at its
// first NUL, leading spaces dropped, a name starting with "[" dropped, " " and "."
// before the first "[" become "_", and "name[...]" makes an array; after an
// unmatched "[" that bracket and every later " ", "." and "[" become "_".
func parsePHPQuery(raw string, into phpVars) {
	for _, pair := range strings.Split(raw, "&") {
		if pair == "" {
			continue
		}
		name, value, _ := strings.Cut(pair, "=")
		name = phpURLDecode(name)
		if i := strings.IndexByte(name, 0); i >= 0 {
			name = name[:i]
		}
		name = strings.TrimLeft(name, " ")
		if name == "" {
			continue
		}
		into.set(name, phpURLDecode(value))
	}
}

func (v phpVars) set(name, value string) {
	b := []byte(name)
	array := false
	cut := len(b)
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c == ' ' || c == '.' {
			b[i] = '_'
			continue
		}
		if c == '[' {
			if i == 0 {
				return // an empty name before the bracket: PHP drops the pair
			}
			if strings.IndexByte(name[i+1:], ']') >= 0 {
				array, cut = true, i
				break
			}
			// Not an index: this bracket and every later " ", "." and "[" become "_".
			for j := i; j < len(b); j++ {
				if b[j] == ' ' || b[j] == '.' || b[j] == '[' {
					b[j] = '_'
				}
			}
			break
		}
	}
	base := string(b[:cut])
	if base == "" {
		return
	}
	if array {
		v[base] = phpValue{array: true}
		return
	}
	v[base] = phpValue{str: value}
}
