package main

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// S1b (20 November 2026): a move loads every table's rows with its owner's
// rights; what cannot be loaded that way is refused before anything changes.

func TestPGMoveVersionsAreTheSecurityUpdatesAMoveNeeds(t *testing.T) {
	clients := map[string]bool{
		"pg_dump (PostgreSQL) 17.12 (Debian 17.12-0+deb13u1)":        true,
		"pg_dump (PostgreSQL) 17.5":                                  false,
		"pg_dump (PostgreSQL) 16.10 (Ubuntu 16.10-0ubuntu0.24.04.1)": true,
		"pg_dump (PostgreSQL) 16.9":                                  false,
		"pg_dump (PostgreSQL) 18.0":                                  true,
		"pg_dump (PostgreSQL) 12.22":                                 false,
		"not a version":                                              false,
	}
	for text, want := range clients {
		if _, got := pgClientVersionSafe(text); got != want {
			t.Errorf("client %q = %v, want %v", text, got, want)
		}
	}
	servers := map[string]bool{"170012": true, "150000": true, "140005": true, "140004": false, "130008": true, "130007": false, "120022": false, "x": false}
	for text, want := range servers {
		if _, got := pgServerVersionSafe(text); got != want {
			t.Errorf("server %q = %v, want %v", text, got, want)
		}
	}
}

func TestPGMoveListGivesEachOwnerItsOwnTablesRows(t *testing.T) {
	list := ";\n; Archive created\n" +
		"215; 1259 16390 TABLE public orders shop_user\n" +
		"3310; 0 16390 TABLE DATA public orders shop_user\n" +
		"3311; 0 16395 TABLE DATA app notes shop_editor\n" +
		"3312; 0 16399 TABLE DATA public spatial_ref_sys postgres\n" +
		"3320; 0 0 SEQUENCE SET public orders_id_seq shop_user\n" +
		"3330; 2613 16500 BLOBS - BLOBS \n"
	byOwner, rest, err := pgMoveLists(list, map[string]bool{"postgres": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(byOwner) != 2 || len(byOwner["shop_user"]) != 1 || len(byOwner["shop_editor"]) != 1 {
		t.Fatalf("by owner = %v", byOwner)
	}
	joined := strings.Join(rest, "\n")
	if !strings.Contains(joined, "spatial_ref_sys postgres") || !strings.Contains(joined, "SEQUENCE SET") || !strings.Contains(joined, "BLOBS") || strings.Contains(joined, "TABLE DATA public orders") {
		t.Fatalf("rest = %v", rest)
	}
	if _, _, err := pgMoveLists("3310; 0 16390 TABLE DATA public my orders shop_user\n", nil); err == nil {
		t.Fatal("a table name with white space was read")
	}
}

// A move is refused before anything is changed when a table belongs to an
// administrator role, when the client programs are older than the security
// updates, and when the target does not let the move role sign in.
func TestPGMoveRefusesWhatCannotBeLoadedWithTheOwnersRights(t *testing.T) {
	f, _ := copyHost(t, "public\x1forders\x1f10\npublic\x1fusers\x1f3\n")
	base := f.answers
	f.answers = func(owner string, port int, database, statement string) (string, error) {
		if statement == pgAdministratorObjectsStatement {
			return "public.audit\n", nil
		}
		return base(owner, port, database, statement)
	}
	var resp transport.PostgreSQLCopyResponse
	copyPGDatabase(context.Background(), f.host(), &transport.PostgreSQLCopyRequest{Database: "shop", SourcePort: 5432, TargetPort: 5433}, &resp)
	if resp.Copied || resp.ErrorCode != "ADMINISTRATOR_OBJECTS" || !strings.Contains(resp.Error, "public.audit") {
		t.Fatalf("administrator's table = %+v", resp)
	}
	for _, s := range f.statement {
		if strings.Contains(s, "CREATE ROLE") || strings.HasPrefix(s, "BEGIN;") {
			t.Fatalf("something was changed before the refusal: %q", s)
		}
	}

	f, _ = copyHost(t, "public\x1forders\x1f10\npublic\x1fusers\x1f3\n")
	stream := f.streamFn
	f.streamFn = func(client string, stdin io.Reader, stdout io.Writer, args []string) error {
		if client == "pg_dump" && len(args) == 1 && args[0] == "--version" {
			_, err := io.WriteString(stdout, "pg_dump (PostgreSQL) 16.4\n")
			return err
		}
		return stream(client, stdin, stdout, args)
	}
	resp = transport.PostgreSQLCopyResponse{}
	copyPGDatabase(context.Background(), f.host(), &transport.PostgreSQLCopyRequest{Database: "shop", SourcePort: 5432, TargetPort: 5433}, &resp)
	if resp.ErrorCode != "CLIENT_OUTDATED" {
		t.Fatalf("old client = %+v", resp)
	}

	f, _ = copyHost(t, "public\x1forders\x1f10\npublic\x1fusers\x1f3\n")
	stream = f.streamFn
	f.streamFn = func(client string, stdin io.Reader, stdout io.Writer, args []string) error {
		if client == "psql" {
			return io.ErrUnexpectedEOF
		}
		return stream(client, stdin, stdout, args)
	}
	resp = transport.PostgreSQLCopyResponse{}
	copyPGDatabase(context.Background(), f.host(), &transport.PostgreSQLCopyRequest{Database: "shop", SourcePort: 5432, TargetPort: 5433}, &resp)
	if resp.Copied || resp.ErrorCode != "LOGIN_REFUSED" || !resp.Cleaned {
		t.Fatalf("no sign-in = %+v", resp)
	}
	for _, s := range f.streams {
		if strings.Contains(s, "pre-data") {
			t.Fatalf("restored although the move role could not sign in: %v", f.streams)
		}
	}
	dropped := false
	for _, s := range f.statement {
		dropped = dropped || strings.HasPrefix(s, `DROP ROLE IF EXISTS "celikpanel_move_`)
	}
	if !dropped {
		t.Fatalf("the move role was not removed: %v", f.statement)
	}
}
