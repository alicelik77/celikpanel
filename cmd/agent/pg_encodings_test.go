package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// One read answers every running cluster's databases with their encodings; a
// cluster that is down is "not known", never "fine".
func TestPGEncodingsAreReadPerRunningCluster(t *testing.T) {
	f := newFakePGHost(t)
	f.answers = func(owner string, port int, database, statement string) (string, error) {
		if database != "postgres" || statement != pgEncodingsStatement {
			t.Fatalf("asked %s on %d: %q", database, port, statement)
		}
		switch port {
		case 5432:
			return "port=5432\nenc=postgres\x1fSQL_ASCII\nenc=shop\x1fSQL_ASCII\nenc=blog\x1fUTF8\n", nil
		case 5433:
			return "port=5433\nenc=fresh\x1fUTF8\nenc=bad name\x1fUTF8\n", nil
		}
		return "", errors.New("no such cluster")
	}
	resp := pgEncodingsRead(context.Background(), f.host())
	if resp.Single || resp.Error != "" || len(resp.Clusters) != 3 {
		t.Fatalf("resp = %+v", resp)
	}
	byPort := map[int]map[string]string{}
	for _, cluster := range resp.Clusters {
		if cluster.Version == "16" {
			if cluster.Known || cluster.Error != "the cluster is not running" {
				t.Fatalf("down cluster = %+v", cluster)
			}
			continue
		}
		if !cluster.Known {
			t.Fatalf("cluster = %+v", cluster)
		}
		byPort[cluster.Port] = cluster.Encodings
	}
	want := map[int]map[string]string{
		5432: {"postgres": "SQL_ASCII", "shop": "SQL_ASCII", "blog": "UTF8"},
		5433: {"fresh": "UTF8"},
	}
	if !reflect.DeepEqual(byPort, want) {
		t.Fatalf("encodings = %v", byPort)
	}
}

// psql's aligned output (the statement runner without --tuples-only, as on a
// server with one PostgreSQL) reads the same.
func TestPGEncodingsReadFromAlignedOutputOnASingleServer(t *testing.T) {
	previous := pgSingleServerEncodings
	pgSingleServerEncodings = func(context.Context) (string, error) {
		return "  ?column?  \n------------\n port=5432\n(1 row)\n\n     ?column?      \n-------------------\n enc=shop\x1fSQL_ASCII \n enc=app\x1fUTF8\n(2 rows)\n", nil
	}
	t.Cleanup(func() { pgSingleServerEncodings = previous })
	host := newFakePGHost(t).host()
	host.family = func() string { return "pacman" }
	resp := pgEncodingsRead(context.Background(), host)
	if !resp.Single || len(resp.Clusters) != 1 || !resp.Clusters[0].Known || resp.Clusters[0].Port != 5432 ||
		!reflect.DeepEqual(resp.Clusters[0].Encodings, map[string]string{"shop": "SQL_ASCII", "app": "UTF8"}) {
		t.Fatalf("resp = %+v", resp)
	}

	pgSingleServerEncodings = func(context.Context) (string, error) { return "", errors.New("connection refused") }
	resp = pgEncodingsRead(context.Background(), host)
	if !resp.Single || resp.Clusters[0].Known || resp.Clusters[0].Error == "" {
		t.Fatalf("unanswered = %+v", resp)
	}
}
