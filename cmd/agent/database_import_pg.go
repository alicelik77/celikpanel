package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// Loading an uploaded SQL file into a PostgreSQL database (alpha.128, G7).
//
// The file is the customer's, so it is never handed to psql and never run as
// the engine's administrator. psql would obey its meta-commands (\! runs a
// shell, \copy and \i read local files, \c changes database), and the
// postgres role could COPY ... TO PROGRAM. Instead the Agent connects as the
// database's own role, with its password, and sends the file's statements
// over the protocol itself: what the file can do is what that role can do in
// its own database, and a psql meta-command is refused, not run.
//
// The reader below splits the file at the statement ends PostgreSQL itself
// sees (outside quotes, dollar quotes and comments), so pg_dump's plain
// format loads as psql would load it: COPY ... FROM stdin blocks are sent
// through the copy protocol, pg_dump's \restrict/\unrestrict guard lines are
// skipped, ALTER ... OWNER TO and SET ROLE/SESSION AUTHORIZATION (another
// server's roles) are left out so the objects belong to this database's
// role, and BEGIN/COMMIT are left out because the whole load already runs in
// one transaction. ROLLBACK, SAVEPOINT and PREPARE TRANSACTION are refused:
// a file that steers the transaction could commit half of itself.
//
// Yüklenen SQL dosyası psql'e verilmez ve yönetici rolüyle çalıştırılmaz:
// Agent veritabanının kendi rolüyle bağlanır ve ifadeleri protokolle gönderir.
// psql komutları reddedilir; COPY blokları kopya protokolüyle gider.

// pgLoadExecutor is the protocol side, a seam for tests.
type pgLoadExecutor interface {
	// Exec runs one batch of complete statements (simple protocol).
	Exec(ctx context.Context, sql string) error
	// CopyFrom runs COPY ... FROM STDIN with the given data.
	CopyFrom(ctx context.Context, sql string, data io.Reader) error
}

// pgLoadRefusal is a file the loader will not run; Code is a transport
// DBImportCode*.
type pgLoadRefusal struct {
	Code string
	Line int
	What string
}

func (e *pgLoadRefusal) Error() string {
	return fmt.Sprintf("line %d: %s", e.Line, e.What)
}

// pgLoadStatementError is a statement the server refused.
type pgLoadStatementError struct {
	Line int
	Err  error
}

func (e *pgLoadStatementError) Error() string {
	return fmt.Sprintf("statement near line %d: %v", e.Line, e.Err)
}

func (e *pgLoadStatementError) Unwrap() error { return e.Err }

const pgLoadBatchBytes = 1 << 20

var (
	pgCopyFromStdinRe   = regexp.MustCompile(`(?is)^copy\s.+\sfrom\s+stdin\b`)
	pgAlterOwnerRe      = regexp.MustCompile(`(?is)^alter\s.+\sowner\s+to\s`)
	pgSkippedSessionRe  = regexp.MustCompile(`(?is)^(set|reset)\s+(session\s+authorization|role)\b`)
	pgSkippedTxRe       = regexp.MustCompile(`(?is)^(begin|start\s+transaction|commit|end)(\s+(work|transaction))?\s*(isolation\s+level\s+.*|read\s+(only|write).*|and\s+(no\s+)?chain)?$`)
	pgRefusedTxRe       = regexp.MustCompile(`(?is)^(rollback|abort|savepoint|release|prepare\s+transaction|commit\s+prepared|rollback\s+prepared)\b`)
	pgAllowedMetaRe     = regexp.MustCompile(`^\\(restrict|unrestrict)(\s|$)`)
	pgMetaCommandNameRe = regexp.MustCompile(`^\\[A-Za-z!.?]*`)
	pgRoutineHeadRe     = regexp.MustCompile(`(?is)^create\s+(or\s+replace\s+)?(function|procedure)\s`)
	pgBeginAtomicRe     = regexp.MustCompile(`(?is)\bbegin\s+atomic\b`)
	pgEndsWithEndRe     = regexp.MustCompile(`(?is)\bend\s*$`)
)

// pgAtomicBodyOpen: a CREATE FUNCTION/PROCEDURE with a BEGIN ATOMIC body whose
// END has not come yet. An approximation of psql's own rule: a CASE ... END
// inside the body can end it early; the server then refuses the statement
// and the load is rolled back - never a wrong success.
func pgAtomicBodyOpen(text string) bool {
	return pgRoutineHeadRe.MatchString(text) && pgBeginAtomicRe.MatchString(text) && !pgEndsWithEndRe.MatchString(text)
}

// pgSplitter reads statements from a SQL file.
type pgSplitter struct {
	r    *bufio.Reader
	line int // current line (1-based)
	// atLineStart: only spaces/tabs since the last newline, outside quotes.
	atLineStart bool
}

func newPGSplitter(r io.Reader) *pgSplitter {
	return &pgSplitter{r: bufio.NewReaderSize(r, 256<<10), line: 1, atLineStart: true}
}

func isPGIdentByte(c byte) bool {
	return c == '_' || c == '$' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

// next returns the next statement (with its first line), without its
// terminating semicolon, or a meta-command line (meta true). io.EOF when the
// file has ended and nothing is left.
func (s *pgSplitter) next() (text string, startLine int, meta bool, err error) {
	var buf bytes.Buffer
	startLine = 0
	const (
		normal = iota
		squote
		equote
		dquote
		dollar
		lineComment
		blockComment
	)
	state := normal
	depth := 0
	var tag []byte
	mark := func() {
		if startLine == 0 {
			startLine = s.line
		}
	}
	for {
		c, readErr := s.r.ReadByte()
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				rest := strings.TrimSpace(buf.String())
				if rest == "" || startLine == 0 {
					return "", 0, false, io.EOF
				}
				return rest, startLine, false, nil
			}
			return "", 0, false, readErr
		}
		if c == '\n' {
			s.line++
		}
		switch state {
		case normal:
			if c == '\\' && s.atLineStart {
				// A psql meta-command: the rest of the line.
				lineText, _ := s.r.ReadString('\n')
				if strings.HasSuffix(lineText, "\n") {
					s.line++
				}
				s.atLineStart = true
				// Inside a statement too: psql would run it there as well.
				return "\\" + strings.TrimRight(lineText, "\r\n"), s.line - 1, true, nil
			}
			switch {
			case c == '\n':
				s.atLineStart = true
			case c == ' ' || c == '\t' || c == '\r':
			default:
				s.atLineStart = false
			}
			switch c {
			case ';':
				text := strings.TrimSpace(buf.String())
				if text == "" || startLine == 0 {
					buf.Reset()
					startLine = 0
					continue
				}
				// A SQL-standard function body (BEGIN ATOMIC ... END) holds
				// semicolons of its own; the statement ends after its END.
				if pgAtomicBodyOpen(text) {
					buf.WriteByte(';')
					continue
				}
				return text, startLine, false, nil
			case '\'':
				mark()
				prev := buf.Bytes()
				escape := false
				if n := len(prev); n > 0 && (prev[n-1] == 'E' || prev[n-1] == 'e') && (n == 1 || !isPGIdentByte(prev[n-2])) {
					escape = true
				}
				buf.WriteByte(c)
				if escape {
					state = equote
				} else {
					state = squote
				}
				continue
			case '"':
				mark()
				buf.WriteByte(c)
				state = dquote
				continue
			case '-':
				if p, _ := s.r.Peek(1); len(p) == 1 && p[0] == '-' {
					_, _ = s.r.ReadByte()
					state = lineComment
					continue
				}
			case '/':
				if p, _ := s.r.Peek(1); len(p) == 1 && p[0] == '*' {
					_, _ = s.r.ReadByte()
					state = blockComment
					depth = 1
					continue
				}
			case '$':
				prev := buf.Bytes()
				if n := len(prev); n == 0 || !isPGIdentByte(prev[n-1]) {
					if t, ok := s.peekDollarTag(); ok {
						mark()
						buf.WriteByte('$')
						buf.Write(t[1:])
						_, _ = s.r.Discard(len(t) - 1)
						tag = t
						state = dollar
						continue
					}
				}
			}
			if !(c == ' ' || c == '\t' || c == '\r' || c == '\n') {
				mark()
			}
			buf.WriteByte(c)
		case squote, dquote:
			buf.WriteByte(c)
			quote := byte('\'')
			if state == dquote {
				quote = '"'
			}
			if c == quote {
				if p, _ := s.r.Peek(1); len(p) == 1 && p[0] == quote {
					b, _ := s.r.ReadByte()
					buf.WriteByte(b)
					continue
				}
				state = normal
				s.atLineStart = false
			}
		case equote:
			buf.WriteByte(c)
			if c == '\\' {
				b, e := s.r.ReadByte()
				if e == nil {
					if b == '\n' {
						s.line++
					}
					buf.WriteByte(b)
				}
				continue
			}
			if c == '\'' {
				if p, _ := s.r.Peek(1); len(p) == 1 && p[0] == '\'' {
					b, _ := s.r.ReadByte()
					buf.WriteByte(b)
					continue
				}
				state = normal
				s.atLineStart = false
			}
		case dollar:
			buf.WriteByte(c)
			if c == '$' {
				rest := tag[1:]
				if p, _ := s.r.Peek(len(rest)); bytes.Equal(p, rest) {
					buf.Write(rest)
					_, _ = s.r.Discard(len(rest))
					state = normal
					s.atLineStart = false
				}
			}
		case lineComment:
			if c == '\n' {
				state = normal
				s.atLineStart = true
				buf.WriteByte('\n')
			}
		case blockComment:
			if c == '*' {
				if p, _ := s.r.Peek(1); len(p) == 1 && p[0] == '/' {
					_, _ = s.r.ReadByte()
					depth--
					if depth == 0 {
						state = normal
						buf.WriteByte(' ')
					}
				}
			} else if c == '/' {
				if p, _ := s.r.Peek(1); len(p) == 1 && p[0] == '*' {
					_, _ = s.r.ReadByte()
					depth++
				}
			}
		}
	}
}

// peekDollarTag reads (without consuming the first '$', which was already
// read) a dollar-quote opening: "$$" or "$tag$". It returns the whole tag
// including both dollars.
func (s *pgSplitter) peekDollarTag() ([]byte, bool) {
	for n := 1; n <= 64; n++ {
		p, err := s.r.Peek(n)
		if len(p) < n {
			_ = err
			return nil, false
		}
		c := p[n-1]
		if c == '$' {
			return append([]byte{'$'}, p...), true
		}
		if n == 1 && c >= '0' && c <= '9' {
			return nil, false // $1 is a parameter, not a quote
		}
		if !(c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80) {
			return nil, false
		}
	}
	return nil, false
}

// copyBlockReader yields the data lines of a COPY ... FROM stdin block up to
// its "\." line (or the end of the file).
type copyBlockReader struct {
	s    *pgSplitter
	done bool
	pend []byte
}

func (c *copyBlockReader) Read(p []byte) (int, error) {
	for len(c.pend) == 0 {
		if c.done {
			return 0, io.EOF
		}
		line, err := c.s.r.ReadBytes('\n')
		if len(line) > 0 && line[len(line)-1] == '\n' {
			c.s.line++
		}
		trimmed := bytes.TrimRight(line, "\r\n")
		if bytes.Equal(trimmed, []byte(`\.`)) {
			c.done = true
			c.s.atLineStart = true
			return 0, io.EOF
		}
		c.pend = line
		if err != nil {
			c.done = true
			if len(line) == 0 {
				return 0, io.EOF
			}
		}
	}
	n := copy(p, c.pend)
	c.pend = c.pend[n:]
	return n, nil
}

// drain reads what is left of a block the server did not read.
func (c *copyBlockReader) drain() {
	_, _ = io.Copy(io.Discard, c)
}

// firstWords is the statement without leading comments, for matching.
func pgStatementHead(text string) string {
	return strings.TrimSpace(text)
}

// loadPGStatements runs a SQL file through exec. The caller wraps it in one
// transaction.
func loadPGStatements(ctx context.Context, exec pgLoadExecutor, r io.Reader) error {
	s := newPGSplitter(r)
	var batch strings.Builder
	batchLine := 0
	flush := func() error {
		if batch.Len() == 0 {
			return nil
		}
		sql := batch.String()
		line := batchLine
		batch.Reset()
		batchLine = 0
		if err := exec.Exec(ctx, sql); err != nil {
			return &pgLoadStatementError{Line: line, Err: err}
		}
		return nil
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		text, line, meta, err := s.next()
		if errors.Is(err, io.EOF) {
			return flush()
		}
		if err != nil {
			return err
		}
		if meta {
			if pgAllowedMetaRe.MatchString(text) {
				continue
			}
			name := pgMetaCommandNameRe.FindString(text)
			if name == "" || len(name) > 20 {
				name = `\`
			}
			return &pgLoadRefusal{Code: "client_command", Line: line, What: "the file contains the psql command " + name + ", which CelikPanel does not run; only SQL statements are loaded"}
		}
		head := pgStatementHead(text)
		switch {
		case pgRefusedTxRe.MatchString(head):
			word := strings.ToUpper(strings.Fields(head)[0])
			return &pgLoadRefusal{Code: "transaction_control", Line: line, What: "the file contains " + word + "; the whole file is loaded as one transaction, so it may not end or split it"}
		case pgSkippedTxRe.MatchString(head), pgAlterOwnerRe.MatchString(head), pgSkippedSessionRe.MatchString(head):
			continue
		case pgCopyFromStdinRe.MatchString(head):
			if err := flush(); err != nil {
				return err
			}
			// The rest of the COPY line (normally only its newline).
			if rest, _ := s.r.ReadString('\n'); strings.HasSuffix(rest, "\n") {
				s.line++
			}
			block := &copyBlockReader{s: s}
			copyErr := exec.CopyFrom(ctx, head, block)
			block.drain()
			if copyErr != nil {
				return &pgLoadStatementError{Line: line, Err: copyErr}
			}
		default:
			if batchLine == 0 {
				batchLine = line
			}
			batch.WriteString(text)
			batch.WriteString(";\n")
			if batch.Len() >= pgLoadBatchBytes {
				if err := flush(); err != nil {
					return err
				}
			}
		}
	}
}
