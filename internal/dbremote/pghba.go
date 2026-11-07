package dbremote

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
)

// The managed block at the end of a cluster's pg_hba.conf. Its END line
// carries a short digest of the lines between the markers, so a block edited
// by hand is recognised and left alone (D-022).
const (
	HBABeginMarker = "# BEGIN CELIKPANEL REMOTE DATABASE ACCESS"
	HBAEndMarker   = "# END CELIKPANEL REMOTE DATABASE ACCESS"
	hbaBeginLine   = HBABeginMarker + " (managed by CelikPanel; your own lines go outside this block)"
)

// ErrHBABlockEdited: the managed block's lines no longer match their digest,
// or the file holds more than one block or a marker without its pair.
var ErrHBABlockEdited = errors.New("the CelikPanel block in pg_hba.conf was changed by hand; CelikPanel leaves it as it is")

// HBALines renders a PostgreSQL target's rules: TLS only (hostssl), one
// database, one user, one address, SCRAM passwords. Names are quoted so no
// name is read as a keyword.
func HBALines(t Target) ([]string, error) {
	norm, err := t.Normalize()
	if err != nil {
		return nil, err
	}
	if norm.Engine != EnginePostgreSQL {
		return nil, fmt.Errorf("pg_hba lines are for PostgreSQL, not %s", norm.Engine)
	}
	lines := make([]string, 0, len(norm.Rules))
	for _, rule := range norm.Rules {
		prefix, err := netip.ParsePrefix(rule.Address)
		if err != nil {
			return nil, err
		}
		lines = append(lines, fmt.Sprintf("hostssl %q %q %s scram-sha-256", rule.Database, rule.User, prefix.String()))
	}
	return lines, nil
}

// hbaBlock finds the managed block: the index of its BEGIN and END lines,
// or -1, -1 when there is none.
func hbaBlock(lines []string) (begin, end int, err error) {
	begin, end = -1, -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, HBABeginMarker):
			if begin >= 0 {
				return -1, -1, ErrHBABlockEdited
			}
			begin = i
		case strings.HasPrefix(trimmed, HBAEndMarker):
			if end >= 0 || begin < 0 {
				return -1, -1, ErrHBABlockEdited
			}
			end = i
		}
	}
	if (begin >= 0) != (end >= 0) {
		return -1, -1, ErrHBABlockEdited
	}
	return begin, end, nil
}

// CurrentHBALines answers the lines of the managed block as they are in the
// file (nil when there is no block), after checking the block's digest.
func CurrentHBALines(content string) ([]string, error) {
	lines := strings.Split(content, "\n")
	begin, end, err := hbaBlock(lines)
	if err != nil || begin < 0 {
		return nil, err
	}
	inner := append([]string(nil), lines[begin+1:end]...)
	if strings.TrimSpace(lines[end]) != HBAEndMarker+" sha256="+digest(inner) {
		return nil, ErrHBABlockEdited
	}
	return inner, nil
}

// SpliceHBA answers the file with its managed block holding exactly lines:
// replaced in place, appended at the end when there is none, removed when
// lines is empty. A hand-edited block is refused. The answer equals the
// input when nothing changes.
func SpliceHBA(content string, lines []string) (string, error) {
	all := strings.Split(content, "\n")
	begin, end, err := hbaBlock(all)
	if err != nil {
		return "", err
	}
	if begin >= 0 {
		if _, err := CurrentHBALines(content); err != nil {
			return "", err
		}
	}
	var block []string
	if len(lines) > 0 {
		block = append(block, hbaBeginLine)
		block = append(block, lines...)
		block = append(block, HBAEndMarker+" sha256="+digest(lines))
	}
	var out []string
	if begin >= 0 {
		out = append(out, all[:begin]...)
		out = append(out, block...)
		out = append(out, all[end+1:]...)
		if len(block) == 0 {
			// Removing the block also removes the empty line put before it.
			if begin > 0 && begin <= len(out) && strings.TrimSpace(out[begin-1]) == "" && begin-1 < len(out) {
				out = append(out[:begin-1], out[begin:]...)
			}
		}
	} else {
		if len(block) == 0 {
			return content, nil
		}
		out = append(out, all...)
		// The file ends with a newline: its last element is "".
		if n := len(out); n > 0 && out[n-1] == "" {
			out = out[:n-1]
		}
		out = append(out, "")
		out = append(out, block...)
		out = append(out, "")
	}
	return strings.Join(out, "\n"), nil
}
