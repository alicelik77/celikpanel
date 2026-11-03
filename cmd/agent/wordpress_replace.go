package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strconv"
)

// The address change of a WordPress copy (alpha.118). A copy of a site
// carries the old address in its database: in plain text, in JSON and in PHP
// serialized values (options, widgets, theme settings). PHP's serialized
// format stores the byte length of every string ("s:20:\"https://a.example\""),
// so a plain text replace that changes a length breaks the value and
// WordPress silently drops it. The replace below parses serialized values,
// changes only the string contents and writes every length again. A value
// that looks serialized but cannot be parsed is left untouched (counted as
// skipped) rather than corrupted.
//
// The replace works on the database dump itself: mysqldump writes every
// value as an SQL string literal, so the literal is unescaped, changed and
// escaped again. No database driver and no site code is needed.
//
// Bir WordPress kopyasının adres değişimi: serileştirilmiş değerlerde
// uzunluklar yeniden yazılır; çözülemeyen değer bozulmaz, olduğu gibi kalır.

// addressPair is one search/replace pair.
type addressPair struct {
	from, to []byte
}

// wordpressAddressPairs: the protocol-relative forms of the address, in the
// order they are tried (the www form first, so it maps to the target too).
// A bare "example.com" is not replaced: e-mail addresses and text that only
// mention the name stay as they are.
func wordpressAddressPairs(source, target string) []addressPair {
	return []addressPair{
		{[]byte("//www." + source), []byte("//" + target)},
		{[]byte("//" + source), []byte("//" + target)},
		{[]byte(`\/\/www.` + source), []byte(`\/\/` + target)},
		{[]byte(`\/\/` + source), []byte(`\/\/` + target)},
		{[]byte("%2F%2Fwww." + source), []byte("%2F%2F" + target)},
		{[]byte("%2F%2F" + source), []byte("%2F%2F" + target)},
	}
}

func hostByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-'
}

// replaceAddress replaces every pair in plain bytes. A match is replaced only
// when the address ends there: "//example.com" does not match inside
// "//example.com.au" or "//example.community".
func replaceAddress(value []byte, pairs []addressPair) ([]byte, bool) {
	changed := false
	for _, pair := range pairs {
		if len(pair.from) == 0 || !bytes.Contains(value, pair.from) {
			continue
		}
		var out []byte
		rest := value
		for {
			index := bytes.Index(rest, pair.from)
			if index < 0 {
				out = append(out, rest...)
				break
			}
			end := index + len(pair.from)
			boundary := end >= len(rest) || !hostByte(rest[end]) &&
				!(rest[end] == '.' && end+1 < len(rest) && hostByte(rest[end+1]))
			if boundary {
				out = append(out, rest[:index]...)
				out = append(out, pair.to...)
				changed = true
			} else {
				out = append(out, rest[:end]...)
			}
			rest = rest[end:]
		}
		value = out
	}
	return value, changed
}

var errNotSerialized = errors.New("not a serialized value")

// looksSerialized: the value starts like a PHP serialized value.
func looksSerialized(value []byte) bool {
	if len(value) < 2 {
		return false
	}
	if bytes.Equal(value, []byte("N;")) {
		return true
	}
	if value[1] != ':' {
		return false
	}
	switch value[0] {
	case 'a', 'O', 's', 'i', 'd', 'b', 'C', 'E':
		return true
	}
	return false
}

// serializedReplacer walks one serialized value and writes it again.
type serializedReplacer struct {
	in      []byte
	pos     int
	out     []byte
	pairs   []addressPair
	changed bool
	depth   int
}

const maxSerializedDepth = 64

func (r *serializedReplacer) expect(c byte) error {
	if r.pos >= len(r.in) || r.in[r.pos] != c {
		return errNotSerialized
	}
	r.pos++
	return nil
}

// readNumber reads digits (with an optional sign) up to stop.
func (r *serializedReplacer) readUntil(stop byte) ([]byte, error) {
	start := r.pos
	for r.pos < len(r.in) && r.in[r.pos] != stop {
		r.pos++
	}
	if r.pos >= len(r.in) {
		return nil, errNotSerialized
	}
	token := r.in[start:r.pos]
	r.pos++
	return token, nil
}

func (r *serializedReplacer) readLength() (int, error) {
	token, err := r.readUntil(':')
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(string(token))
	if err != nil || n < 0 || n > len(r.in) {
		return 0, errNotSerialized
	}
	return n, nil
}

// quoted reads `"<n bytes>"`.
func (r *serializedReplacer) quoted(n int) ([]byte, error) {
	if err := r.expect('"'); err != nil {
		return nil, err
	}
	if r.pos+n > len(r.in) {
		return nil, errNotSerialized
	}
	content := r.in[r.pos : r.pos+n]
	r.pos += n
	if err := r.expect('"'); err != nil {
		return nil, err
	}
	return content, nil
}

func (r *serializedReplacer) value() error {
	r.depth++
	defer func() { r.depth-- }()
	if r.depth > maxSerializedDepth || r.pos+1 >= len(r.in) {
		return errNotSerialized
	}
	kind := r.in[r.pos]
	if kind == 'N' {
		if err := r.expect('N'); err != nil {
			return err
		}
		if err := r.expect(';'); err != nil {
			return err
		}
		r.out = append(r.out, 'N', ';')
		return nil
	}
	r.pos++
	if err := r.expect(':'); err != nil {
		return err
	}
	switch kind {
	case 'b', 'i', 'd', 'r', 'R':
		token, err := r.readUntil(';')
		if err != nil {
			return err
		}
		r.out = append(r.out, kind, ':')
		r.out = append(r.out, token...)
		r.out = append(r.out, ';')
		return nil
	case 's':
		n, err := r.readLength()
		if err != nil {
			return err
		}
		content, err := r.quoted(n)
		if err != nil {
			return err
		}
		if err := r.expect(';'); err != nil {
			return err
		}
		r.writeString(content)
		return nil
	case 'E':
		n, err := r.readLength()
		if err != nil {
			return err
		}
		content, err := r.quoted(n)
		if err != nil {
			return err
		}
		if err := r.expect(';'); err != nil {
			return err
		}
		r.out = append(r.out, 'E', ':')
		r.out = strconv.AppendInt(r.out, int64(len(content)), 10)
		r.out = append(r.out, ':', '"')
		r.out = append(r.out, content...)
		r.out = append(r.out, '"', ';')
		return nil
	case 'a':
		count, err := r.readLength()
		if err != nil {
			return err
		}
		r.out = append(r.out, 'a', ':')
		r.out = strconv.AppendInt(r.out, int64(count), 10)
		r.out = append(r.out, ':')
		return r.members(count)
	case 'O':
		n, err := r.readLength()
		if err != nil {
			return err
		}
		class, err := r.quoted(n)
		if err != nil {
			return err
		}
		if err := r.expect(':'); err != nil {
			return err
		}
		count, err := r.readLength()
		if err != nil {
			return err
		}
		r.out = append(r.out, 'O', ':')
		r.out = strconv.AppendInt(r.out, int64(len(class)), 10)
		r.out = append(r.out, ':', '"')
		r.out = append(r.out, class...)
		r.out = append(r.out, '"', ':')
		r.out = strconv.AppendInt(r.out, int64(count), 10)
		r.out = append(r.out, ':')
		return r.members(count)
	case 'C':
		// A class's own serialization: its payload is opaque, so it is
		// copied as it is (its inner lengths are the class's business).
		n, err := r.readLength()
		if err != nil {
			return err
		}
		class, err := r.quoted(n)
		if err != nil {
			return err
		}
		if err := r.expect(':'); err != nil {
			return err
		}
		size, err := r.readLength()
		if err != nil {
			return err
		}
		if err := r.expect('{'); err != nil {
			return err
		}
		if r.pos+size > len(r.in) {
			return errNotSerialized
		}
		payload := r.in[r.pos : r.pos+size]
		r.pos += size
		if err := r.expect('}'); err != nil {
			return err
		}
		r.out = append(r.out, 'C', ':')
		r.out = strconv.AppendInt(r.out, int64(len(class)), 10)
		r.out = append(r.out, ':', '"')
		r.out = append(r.out, class...)
		r.out = append(r.out, '"', ':')
		r.out = strconv.AppendInt(r.out, int64(len(payload)), 10)
		r.out = append(r.out, ':', '{')
		r.out = append(r.out, payload...)
		r.out = append(r.out, '}')
		return nil
	}
	return errNotSerialized
}

func (r *serializedReplacer) members(count int) error {
	if err := r.expect('{'); err != nil {
		return err
	}
	r.out = append(r.out, '{')
	for i := 0; i < count*2; i++ {
		if err := r.value(); err != nil {
			return err
		}
	}
	if err := r.expect('}'); err != nil {
		return err
	}
	r.out = append(r.out, '}')
	return nil
}

// writeString writes a string member, replacing inside it. A string that is
// itself a serialized value (WordPress stores those) is walked in turn.
func (r *serializedReplacer) writeString(content []byte) {
	replaced, changed, _ := replaceInValue(content, r.pairs)
	if changed {
		r.changed = true
		content = replaced
	}
	r.out = append(r.out, 's', ':')
	r.out = strconv.AppendInt(r.out, int64(len(content)), 10)
	r.out = append(r.out, ':', '"')
	r.out = append(r.out, content...)
	r.out = append(r.out, '"', ';')
}

// replaceInValue changes the address in one stored value. skipped reports a
// value that looks serialized, holds the address, but cannot be parsed: it
// is returned unchanged.
func replaceInValue(value []byte, pairs []addressPair) (out []byte, changed bool, skipped bool) {
	if !containsAnyPair(value, pairs) {
		return value, false, false
	}
	if looksSerialized(value) {
		r := &serializedReplacer{in: value, pairs: pairs}
		if err := r.value(); err == nil && r.pos == len(value) {
			if !r.changed {
				return value, false, false
			}
			return r.out, true, false
		}
		return value, false, true
	}
	out, changed = replaceAddress(value, pairs)
	return out, changed, false
}

func containsAnyPair(value []byte, pairs []addressPair) bool {
	for _, pair := range pairs {
		if bytes.Contains(value, pair.from) {
			return true
		}
	}
	return false
}

// replaceStats counts what the dump transform did.
type replaceStats struct {
	Replaced int
	Skipped  int
}

// transformSQLDump copies a mysqldump stream and changes the address inside
// every single-quoted string literal. Identifiers (`...`), double-quoted
// text and "-- " comment lines are copied as they are.
func transformSQLDump(src io.Reader, dst io.Writer, pairs []addressPair) (replaceStats, error) {
	var stats replaceStats
	in := bufio.NewReaderSize(src, 1<<20)
	out := bufio.NewWriterSize(dst, 1<<20)
	const (
		plain = iota
		backtick
		double
	)
	state := plain
	lineStart := true
	var literal []byte
	for {
		c, err := in.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return stats, err
		}
		switch state {
		case backtick:
			if c == '`' {
				state = plain
			}
			if err := out.WriteByte(c); err != nil {
				return stats, err
			}
			lineStart = false
			continue
		case double:
			if err := out.WriteByte(c); err != nil {
				return stats, err
			}
			if c == '\\' {
				next, err := in.ReadByte()
				if err != nil {
					return stats, errors.New("dump ends inside a quoted text")
				}
				if err := out.WriteByte(next); err != nil {
					return stats, err
				}
			} else if c == '"' {
				state = plain
			}
			continue
		}
		// plain
		if lineStart && c == '-' {
			if peek, _ := in.Peek(2); len(peek) == 2 && peek[0] == '-' && (peek[1] == ' ' || peek[1] == '\n') {
				line, err := in.ReadBytes('\n')
				if err := out.WriteByte(c); err != nil {
					return stats, err
				}
				if _, werr := out.Write(line); werr != nil {
					return stats, werr
				}
				if err == io.EOF {
					break
				}
				if err != nil {
					return stats, err
				}
				lineStart = true
				continue
			}
		}
		switch c {
		case '`':
			state = backtick
		case '"':
			state = double
		case '\'':
			literal = literal[:0]
			closed := false
			for !closed {
				b, err := in.ReadByte()
				if err != nil {
					return stats, errors.New("dump ends inside a text value")
				}
				switch b {
				case '\\':
					escaped, err := in.ReadByte()
					if err != nil {
						return stats, errors.New("dump ends inside a text value")
					}
					literal = append(literal, unescapeSQL(escaped))
				case '\'':
					if next, _ := in.Peek(1); len(next) == 1 && next[0] == '\'' {
						_, _ = in.ReadByte()
						literal = append(literal, '\'')
					} else {
						closed = true
					}
				default:
					literal = append(literal, b)
				}
			}
			value, changed, skipped := replaceInValue(literal, pairs)
			if changed {
				stats.Replaced++
			}
			if skipped {
				stats.Skipped++
			}
			if err := writeSQLLiteral(out, value); err != nil {
				return stats, err
			}
			lineStart = false
			continue
		}
		if err := out.WriteByte(c); err != nil {
			return stats, err
		}
		lineStart = c == '\n'
	}
	if state != plain {
		return stats, errors.New("dump ends inside a quoted name or text")
	}
	return stats, out.Flush()
}

func unescapeSQL(c byte) byte {
	switch c {
	case '0':
		return 0
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 'Z':
		return 0x1a
	case 't':
		return '\t'
	case 'b':
		return '\b'
	}
	return c
}

// writeSQLLiteral writes the value the way mysqldump escapes it.
func writeSQLLiteral(out *bufio.Writer, value []byte) error {
	if err := out.WriteByte('\''); err != nil {
		return err
	}
	for _, c := range value {
		var escaped string
		switch c {
		case 0:
			escaped = `\0`
		case '\n':
			escaped = `\n`
		case '\r':
			escaped = `\r`
		case 0x1a:
			escaped = `\Z`
		case '\\':
			escaped = `\\`
		case '\'':
			escaped = `\'`
		case '"':
			escaped = `\"`
		}
		if escaped != "" {
			if _, err := out.WriteString(escaped); err != nil {
				return err
			}
			continue
		}
		if err := out.WriteByte(c); err != nil {
			return err
		}
	}
	return out.WriteByte('\'')
}
