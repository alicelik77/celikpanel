package main

import (
	"bufio"
	"bytes"
	"io"
	"regexp"
)

// A dump of a SQL_ASCII database, loaded into a UTF8 one (20 Nov 2026).
//
// pg_dump writes the source database's encoding as the client encoding:
// SET client_encoding = 'SQL_ASCII'. Under that client encoding PostgreSQL
// does not check the bytes of COPY data, so loading such a dump into a UTF8
// database stored a Latin-1 byte there as it was (seen on PostgreSQL 17.12:
// "caf\xe9" kept as 63 61 66 e9, upper() then read e9 20 61 as one
// character, and a later UTF8 dump of it would not load anywhere). That is
// exactly the way out the database card names for a SQL_ASCII database
// (export, a new UTF8 database, import; or Copy), and the same holds for a
// backup restored into a database made again as UTF8.
//
// The dump's statement is therefore replaced by one that keeps SQL_ASCII
// wherever the target database is not UTF8 (a SQL_ASCII database's own backup
// restores as before, whatever bytes it holds) and makes the client encoding
// UTF8 in a UTF8 database: text that is valid UTF-8 loads unchanged, any
// other byte stops the load with PostgreSQL's own "invalid byte sequence for
// encoding "UTF8"", and the load's transaction leaves the database as it was.
//
// SQL_ASCII bir veritabanının dökümü UTF8 bir veritabanına yüklenirken her
// bayt denetlenir; UTF-8 olmayan bayt yüklemeyi durdurur.

// pgCheckedClientEncodingStatement keeps SQL_ASCII except in a UTF8 database.
const pgCheckedClientEncodingStatement = "SELECT pg_catalog.set_config('client_encoding', CASE WHEN pg_catalog.current_setting('server_encoding') = 'UTF8' THEN 'UTF8' ELSE 'SQL_ASCII' END, false)"

var pgSQLASCIIClientEncodingRe = regexp.MustCompile(`(?i)^\s*set\s+(session\s+)?client_encoding\s*(=|\s+to\s+|\s)\s*'?sql_ascii'?\s*;?\s*$`)

// pgCheckedClientEncoding is the statement to send for one statement of a
// loaded file (the import), and whether it was replaced.
func pgCheckedClientEncoding(statement string) (string, bool) {
	if pgSQLASCIIClientEncodingRe.MatchString(statement) {
		return pgCheckedClientEncodingStatement, true
	}
	return statement, false
}

// pgDumpHeaderLineRe: the lines of a plain pg_dump before its first object -
// blank lines, comments, SET and set_config lines, \restrict.
var pgDumpHeaderLineRe = regexp.MustCompile(`(?i)^\s*($|--|set\s|select\s+pg_catalog\.set_config\(|\\restrict\s|\\unrestrict\s)`)

// newPGDumpEncodingFilter passes a plain dump through for psql (the copy and
// a backup restore), with the header's SET client_encoding = 'SQL_ASCII'
// replaced; it stops looking at the first line past the header, so no data
// line is ever changed.
func newPGDumpEncodingFilter(r io.Reader) io.Reader {
	return &pgDumpEncodingFilter{r: bufio.NewReaderSize(r, 64<<10)}
}

type pgDumpEncodingFilter struct {
	r       *bufio.Reader
	pending []byte
	through bool
}

func (f *pgDumpEncodingFilter) Read(p []byte) (int, error) {
	for len(f.pending) == 0 {
		if f.through {
			return f.r.Read(p)
		}
		line, err := f.r.ReadBytes('\n')
		if len(line) > 0 {
			body := bytes.TrimRight(line, "\r\n")
			ending := line[len(body):]
			switch {
			case pgSQLASCIIClientEncodingRe.Match(body):
				line = append([]byte(pgCheckedClientEncodingStatement+";"), ending...)
				f.through = true
			case !pgDumpHeaderLineRe.Match(body):
				f.through = true
			}
			f.pending = line
		}
		if err != nil {
			if len(f.pending) == 0 {
				return 0, err
			}
			f.through = true
		}
	}
	n := copy(p, f.pending)
	f.pending = f.pending[n:]
	return n, nil
}
