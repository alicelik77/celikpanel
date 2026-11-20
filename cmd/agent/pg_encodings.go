package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// PostgreSQLDatabaseEncodingsV1 (20 Nov 2026) answers, read only and
// bounded, the encoding of every database on this server's running
// PostgreSQL clusters, so a database list says each database's encoding in
// one call. A database made on a cluster with SQL_ASCII templates before this
// release is SQL_ASCII: its text is stored unchecked, and the list says so
// next to it. Nothing is changed here.
//
// Sunucudaki her PostgreSQL veritabanının kodlaması tek çağrıda, salt okunur
// olarak okunur.

// pgEncodingsStatement names each database and its encoding on lines of their
// own ("enc=<name><US><encoding>"), readable in psql's aligned and unaligned
// output alike; "port=" names the cluster's port.
const pgEncodingsStatement = "SELECT 'port=' || current_setting('port');\n" +
	"SELECT 'enc=' || datname || chr(31) || pg_catalog.pg_encoding_to_char(encoding) FROM pg_catalog.pg_database WHERE NOT datistemplate ORDER BY datname;"

// parsePGEncodings reads pgEncodingsStatement's answer. Names outside the
// panel's own rule for a PostgreSQL database name are left out.
func parsePGEncodings(out string) (map[string]string, int) {
	encodings := map[string]string{}
	port := 0
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "port="); ok {
			if n := parsePositiveInt(rest); n > 0 && n < 65536 {
				port = n
			}
			continue
		}
		rest, ok := strings.CutPrefix(line, "enc=")
		if !ok {
			continue
		}
		name, encoding, found := strings.Cut(rest, "\x1f")
		encoding = strings.TrimSpace(encoding)
		if !found || !pgDatabaseNameRe.MatchString(name) || encoding == "" || len(encodings) >= pgDatabaseNamesCap {
			continue
		}
		encodings[name] = encoding
	}
	return encodings, port
}

func parsePositiveInt(text string) int {
	n := 0
	for _, r := range strings.TrimSpace(text) {
		if r < '0' || r > '9' || n > 1<<20 {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// pgSingleServerEncodings is the read where PostgreSQL is one server without
// postgresql-common (a variable for tests).
// It is the Agent's own statement runner, the one every other PostgreSQL
// statement on such a server goes through (no deadline of its own, as for
// them; the Panel bounds its wait for the answer).
var pgSingleServerEncodings = func(context.Context) (string, error) {
	run, err := postgreSQLStatementFor(0)
	if err != nil {
		return "", err
	}
	out, err := run(pgEncodingsStatement)
	if err != nil {
		if text := strings.TrimSpace(string(out)); text != "" {
			return "", errors.New(boundedText(text, 200))
		}
		return "", err
	}
	return string(out), nil
}

func pgEncodingsRead(ctx context.Context, host pgClusterHost) transport.PostgreSQLEncodingsResponse {
	resp := transport.PostgreSQLEncodingsResponse{Clusters: []transport.PostgreSQLClusterEncodings{}}
	if host.family() != "apt" {
		resp.Single = true
		entry := transport.PostgreSQLClusterEncodings{Online: true, Encodings: map[string]string{}}
		readCtx, cancel := context.WithTimeout(ctx, pgReadBound)
		out, err := pgSingleServerEncodings(readCtx)
		cancel()
		if err != nil {
			entry.Online = false
			entry.Error = "PostgreSQL did not answer: " + boundedText(err.Error(), 200)
		} else {
			entry.Encodings, entry.Port = parsePGEncodings(out)
			entry.Known = true
		}
		resp.Clusters = append(resp.Clusters, entry)
		return resp
	}
	readCtx, cancel := context.WithTimeout(ctx, pgReadBound)
	out, err := host.lsclusters(readCtx)
	cancel()
	if err != nil {
		resp.Error = "pg_lsclusters could not be read: " + boundedText(err.Error(), 200)
		return resp
	}
	for _, cluster := range parsePGLsClusters(out) {
		entry := transport.PostgreSQLClusterEncodings{Version: cluster.Version, Name: cluster.Name, Port: cluster.Port, Online: cluster.Online, Encodings: map[string]string{}}
		switch {
		case !cluster.Online:
			entry.Error = "the cluster is not running"
		case !pgOwnerRe.MatchString(cluster.Owner):
			entry.Error = "the cluster's owner is not an account CelikPanel can ask as"
		default:
			queryCtx, cancel := context.WithTimeout(ctx, pgReadBound)
			answer, err := host.query(queryCtx, cluster.Owner, cluster.Port, "postgres", pgEncodingsStatement)
			cancel()
			if err != nil {
				entry.Error = "the cluster did not answer: " + boundedText(err.Error(), 200)
				break
			}
			entry.Encodings, _ = parsePGEncodings(answer)
			entry.Known = true
		}
		resp.Clusters = append(resp.Clusters, entry)
	}
	return resp
}

// PostgreSQLDatabaseEncodingsV1 is the read; it changes nothing.
func (a *Agent) PostgreSQLDatabaseEncodingsV1(_ *transport.Empty, resp *transport.PostgreSQLEncodingsResponse) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*pgReadBound+10*time.Second)
	defer cancel()
	*resp = pgEncodingsRead(ctx, pgClusterHostActive())
	return nil
}
