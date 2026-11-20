package main

import (
	"context"
	"log"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// A PostgreSQL database's encoding on its card (20 Nov 2026). A database made
// on a cluster with SQL_ASCII templates before this release is SQL_ASCII:
// PostgreSQL stores its text as unchecked bytes. The domain's database list
// reads every PostgreSQL database's encoding from the Agent in one call (not
// one per row) and puts it on each database it can place: a database on this
// machine whose record names a cluster the read answered for. Anything else
// (another machine's server, a cluster down or not answering, the Agent not
// answering in time) carries no encoding, and the card then says nothing
// about it rather than guess. Nothing is changed.
//
// Veritabanı listesi her PostgreSQL veritabanının kodlamasını tek çağrıda
// okur; bilinmeyen kodlama için kart bir şey söylemez.

// pgEncodingsListBound keeps the list quick when the Agent is slow: the list
// is drawn without encodings rather than late.
const pgEncodingsListBound = 8 * time.Second

// readPGEncodingsFromAgent is the one read (a variable for tests).
var readPGEncodingsFromAgent = func(p *Panel, ctx context.Context) (transport.PostgreSQLEncodingsResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, pgEncodingsListBound)
	defer cancel()
	var resp transport.PostgreSQLEncodingsResponse
	err := p.callAgentContext(callCtx, "Agent.PostgreSQLDatabaseEncodingsV1", &transport.Empty{}, &resp)
	return resp, err
}

// pgEncodingWanted is one listed PostgreSQL database whose encoding is asked.
type pgEncodingWanted struct {
	index  int
	name   string
	record pgServerRecord
}

// pgEncodingOf is the encoding of database name on the record's cluster, or
// "" when the read does not say it.
func pgEncodingOf(read transport.PostgreSQLEncodingsResponse, record pgServerRecord, name string) string {
	if read.Single {
		if len(read.Clusters) != 1 || !read.Clusters[0].Known {
			return ""
		}
		only := read.Clusters[0]
		if record.Port > 0 && only.Port > 0 && record.Port != only.Port {
			return ""
		}
		return only.Encodings[name]
	}
	clusters := make([]transport.PostgreSQLCluster, 0, len(read.Clusters))
	for _, cluster := range read.Clusters {
		clusters = append(clusters, transport.PostgreSQLCluster{Version: cluster.Version, Name: cluster.Name, Port: cluster.Port, Online: cluster.Online})
	}
	placed, ok := pgClusterOfRecord(record, clusters)
	if !ok {
		return ""
	}
	for _, cluster := range read.Clusters {
		if cluster.Version == placed.Version && cluster.Name == placed.Name && cluster.Known {
			return cluster.Encodings[name]
		}
	}
	return ""
}

// fillPGEncodings sets Encoding on the listed databases it can place, with
// one Agent read; a failed read is logged and leaves them unset.
func (p *Panel) fillPGEncodings(ctx context.Context, databases []DatabaseInfo, wanted []pgEncodingWanted) {
	if len(wanted) == 0 {
		return
	}
	read, err := readPGEncodingsFromAgent(p, ctx)
	if err != nil {
		log.Printf("database list: the PostgreSQL encodings could not be read: %v", err)
		return
	}
	for _, want := range wanted {
		databases[want.index].Encoding = pgEncodingOf(read, want.record, want.name)
	}
}
