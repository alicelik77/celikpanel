package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A database deletion drops the database it was asked to delete, once
// (alpha.140, carried item 12).
//
// Before: every press of Delete sent DROP DATABASE IF EXISTS <name>. The
// engine's drop and CelikPanel's records are two steps, and the record stays
// listed whenever the second does not follow the first: the drop's answer was
// lost (a timeout, a restart of the Agent or of the panel, a closed page), the
// database's users or the rights naming it were not all removed yet, or the
// records could not be written. Meanwhile a database of the same name can be
// made on the engine - by the customer's own user, which keeps its rights on
// that name until the deletion finishes; by the owner by hand; by a restore or
// an installer - and the next press dropped THAT database, with everything in
// it.
//
// Now:
//  1. Before the first drop the deletion reads the database's identity on the
//     engine (PostgreSQL: its oid; MariaDB on this machine: its directory's
//     device, inode, inode generation and birth time; see internal/transport/
//     database_drop_once.go) and keeps it, durably, in a marker of the
//     database record (panel_settings "database_delete_v1:<id>", state
//     "sent"). Nothing is dropped unless that marker is written.
//  2. The drop takes the database only while the engine shows that identity.
//     Dropped, absent, or a database of the name that is another one: the
//     marker says "dropped", and no later press drops anything again; it only
//     finishes the users and the records.
//  3. A later press after a lost answer sends the same identity. Without an
//     identity (a filesystem without birth times, a MariaDB reached with a
//     record's own credential) a database of the name found then is never
//     dropped: whether it is the one asked for cannot be told, and the answer
//     says so (DATABASE_DELETE_UNVERIFIED) with who acts.
//  4. The marker goes with the database's records, in their transaction.
//
// A status read never drops anything: only a person's Delete (or a domain
// deletion that was asked to delete the domain's databases) runs this.
//
// Silme, silinmesi istenen veritabanını bir kez siler: ilk silmeden önce
// veritabanının motordaki kimliği okunur ve kalıcı olarak saklanır; sonraki
// basışlar yalnız o kimliği taşıyan veritabanını siler, bu arada aynı adla
// açılmış veritabanını asla.

const (
	databaseDeleteMarkerPrefix = "database_delete_v1:"
	// databaseDeleteSent: the drop was sent (or is about to be); its outcome
	// is not known yet.
	databaseDeleteSent = "sent"
	// databaseDeleteDropped: the engine no longer holds the database asked
	// for (dropped now or earlier, or the name holds another database).
	databaseDeleteDropped = "dropped"

	// errCodeDatabaseDeleteUnverified: see databaseDeleteError.unverified.
	errCodeDatabaseDeleteUnverified = "DATABASE_DELETE_UNVERIFIED"
)

// databaseDeleteMarker is a deletion's durable memory of the engine step.
type databaseDeleteMarker struct {
	Version int    `json:"v"`
	Name    string `json:"name"`
	// Engine is databaseServerEngineIdentity of the record when it was
	// written: a marker of another engine is not this deletion's.
	Engine string `json:"engine"`
	// Identity: the database's identity read before the first drop; "" when
	// the engine could not tell one.
	Identity string `json:"identity,omitempty"`
	State    string `json:"state"`
	At       string `json:"at"`
}

func databaseDeleteMarkerKey(databaseID int) string {
	return databaseDeleteMarkerPrefix + strconv.Itoa(databaseID)
}

// readDatabaseDeleteMarker reads the marker of a database record; nil when
// there is none. A marker that cannot be read is an error: the deletion
// stops before the engine rather than guess.
func (p *Panel) readDatabaseDeleteMarker(ctx context.Context, databaseID int) (*databaseDeleteMarker, error) {
	var raw string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`, databaseDeleteMarkerKey(databaseID)).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the deletion marker of database %d: %w", databaseID, err)
	}
	var marker databaseDeleteMarker
	if err := json.Unmarshal([]byte(raw), &marker); err != nil || marker.Version != 1 ||
		(marker.State != databaseDeleteSent && marker.State != databaseDeleteDropped) {
		return nil, fmt.Errorf("the deletion marker of database %d cannot be read", databaseID)
	}
	return &marker, nil
}

// writeDatabaseDeleteMarker stores the marker of a database record.
func (p *Panel) writeDatabaseDeleteMarker(ctx context.Context, databaseID int, marker *databaseDeleteMarker) error {
	marker.Version = 1
	marker.At = time.Now().UTC().Format(time.RFC3339)
	raw, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	_, err = p.db.GetDB().ExecContext(ctx, `INSERT INTO panel_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`, databaseDeleteMarkerKey(databaseID), string(raw))
	return err
}

// clearDatabaseDeleteMarker removes a marker this press wrote when nothing
// was dropped (a verified refusal or failure, an Agent that was not reached).
func (p *Panel) clearDatabaseDeleteMarker(ctx context.Context, databaseID int) {
	if _, err := p.db.GetDB().ExecContext(context.WithoutCancel(ctx), `DELETE FROM panel_settings WHERE key = ?`, databaseDeleteMarkerKey(databaseID)); err != nil {
		// Kept, it binds the next press to the same identity: still safe.
		log.Printf("database deletion %d: the marker of a drop that did not run stays: %v", databaseID, err)
	}
}

// databaseDropAnswer is what one bound drop did.
type databaseDropAnswer struct {
	dropped, absent, replaced bool
}

// databaseDropReach reaches one engine for the deletion: identify reads
// (exists, identity), drop drops only the identified database (or, byName,
// the one of that name now). Their errors are *databaseDeleteError.
type databaseDropReach struct {
	identify func(ctx context.Context) (bool, string, error)
	drop     func(ctx context.Context, identity string, byName bool) (databaseDropAnswer, error)
	// driver: the record's own credential (nil through the Agent).
	driver services.DatabaseDriver
}

// databaseDropThroughAgent reaches this machine's engine through the Agent.
func (p *Panel) databaseDropThroughAgent(server *core.DatabaseServer, name string) databaseDropReach {
	engine, port := databaseAgentEngine(server)
	commit := strings.TrimSpace(buildCommit)
	return databaseDropReach{
		identify: func(ctx context.Context) (bool, string, error) {
			var resp transport.DatabaseIdentityResponse
			err := databaseIdentityAgent(p, ctx, transport.DatabaseIdentityRequest{ExpectedBuildCommit: commit, Type: engine, Name: name, Port: port}, &resp)
			switch {
			case err != nil && agentUnreachableAlone(err):
				// Nothing ran; said as the Agent not being reached.
				return false, "", &databaseDeleteError{unknown: true, cause: fmt.Errorf("read database %s: %w", name, err)}
			case err != nil:
				// Only a read was asked: nothing was dropped.
				log.Printf("[409][database] %s %s: its identity could not be read: %v", errCodeDatabaseDeleteFailed, name, err)
				return false, "", &databaseDeleteError{cause: fmt.Errorf("read database %s: %w", name, err)}
			case resp.Refusal != nil:
				return false, "", &databaseDeleteError{refusal: resp.Refusal, cause: fmt.Errorf("read database %s: its PostgreSQL cluster cannot answer", name)}
			case resp.Error != "":
				log.Printf("[409][database] %s %s: its identity could not be read: %s", errCodeDatabaseDeleteFailed, name, boundedAgentDiagnostic(resp.Error))
				return false, "", &databaseDeleteError{cause: fmt.Errorf("read database %s: the engine did not answer", name)}
			}
			return resp.Exists, resp.Identity, nil
		},
		drop: func(ctx context.Context, identity string, byName bool) (databaseDropAnswer, error) {
			var resp transport.DropDatabaseOnceResponse
			err := databaseDropOnceAgent(p, ctx, transport.DropDatabaseOnceRequest{ExpectedBuildCommit: commit, Type: engine, Name: name, Port: port,
				Identity: identity, ByName: byName}, &resp)
			switch {
			case err != nil:
				return databaseDropAnswer{}, &databaseDeleteError{unknown: true, cause: fmt.Errorf("drop database %s: %w", name, err)}
			case resp.Refusal != nil:
				return databaseDropAnswer{}, &databaseDeleteError{refusal: resp.Refusal, cause: fmt.Errorf("drop database %s: its PostgreSQL cluster cannot take it", name)}
			case resp.Error != "" || !(resp.Dropped || resp.Absent || resp.Replaced):
				log.Printf("[409][database] %s %s: %s", errCodeDatabaseDeleteFailed, name, boundedAgentDiagnostic(resp.Error))
				return databaseDropAnswer{}, &databaseDeleteError{cause: fmt.Errorf("drop database %s: the engine did not drop it", name)}
			}
			return databaseDropAnswer{dropped: resp.Dropped, absent: resp.Absent, replaced: resp.Replaced}, nil
		},
	}
}

// databaseIdentityReader is a driver that tells a database's identity
// (internal/services: PostgreSQL its oid, MariaDB none).
type databaseIdentityReader interface {
	DatabaseIdentity(name string) (exists bool, identity string, err error)
}

// databaseDropWithRecord reaches an engine with the record's own credential
// (another machine, or an administrator's own record on this one).
func (p *Panel) databaseDropWithRecord(server *core.DatabaseServer, name string) (databaseDropReach, error) {
	driver, err := p.dbDriverFor(server)
	if err != nil {
		return databaseDropReach{}, fmt.Errorf("open the credential of database server %d: %w", server.ID, err)
	}
	engineErr := func(err error) error {
		return &databaseDeleteError{engine: err, server: server, cause: fmt.Errorf("drop database %s: %w", name, err)}
	}
	identify := func() (bool, string, error) {
		reader, ok := driver.(databaseIdentityReader)
		if !ok {
			// A driver that cannot tell: the database is taken to be there,
			// its identity not known.
			return true, "", nil
		}
		exists, identity, err := reader.DatabaseIdentity(name)
		if err != nil {
			return false, "", engineErr(err)
		}
		return exists, identity, nil
	}
	return databaseDropReach{
		driver:   driver,
		identify: func(context.Context) (bool, string, error) { return identify() },
		drop: func(_ context.Context, identity string, byName bool) (databaseDropAnswer, error) {
			exists, now, err := identify()
			switch {
			case err != nil:
				return databaseDropAnswer{}, err
			case !exists:
				return databaseDropAnswer{absent: true}, nil
			case identity == "" && !byName, identity != "" && now != identity:
				return databaseDropAnswer{replaced: true}, nil
			}
			if err := driver.DeleteDatabase(name); err != nil {
				// A refused credential or a connection that was never made ran
				// nothing; anything else (a reset, a timeout) may have: the
				// marker then stays and binds the next press.
				return databaseDropAnswer{}, &databaseDeleteError{engine: err, server: server,
					unknown: !databaseEngineFailedBeforeStatement(err),
					cause:   fmt.Errorf("drop database %s: %w", name, err)}
			}
			return databaseDropAnswer{dropped: true}, nil
		},
	}, nil
}

// databaseEngineFailedBeforeStatement: an engine reached with a record's
// credential failed in a way that comes before any statement runs - the
// credential was refused, or no connection was made. A reset connection or
// a timeout is not among them: the statement may have run.
func databaseEngineFailedBeforeStatement(err error) bool {
	if services.ClassifyDatabaseEngineRefusal(err) == services.DatabaseEngineRefusalCredential {
		return true
	}
	text := strings.ToLower(err.Error())
	for _, phrase := range []string{"can't connect to", "cannot connect to", "error 2002", "error 2003", "could not connect",
		"connection refused", "host is unreachable", "network is unreachable", "no route to host"} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

// dropDatabaseOnce makes the engine not hold the database the deletion of
// record databaseID was asked for, dropping it at most once (see the top of
// this file). nil: the users and the records may follow.
func (p *Panel) dropDatabaseOnce(ctx context.Context, databaseID int, engine string, deletion *databaseDeletion, reach databaseDropReach) error {
	name := deletion.Name
	marker, err := p.readDatabaseDeleteMarker(ctx, databaseID)
	if err != nil {
		return err
	}
	if marker != nil && (marker.Name != name || marker.Engine != engine) {
		// Written for another name or engine of this record (its server was
		// changed since): not this deletion's; this press starts afresh.
		log.Printf("database deletion %s: a marker of %s on %s is not this deletion's; it is replaced", name, marker.Name, marker.Engine)
		marker = nil
	}
	if marker != nil && marker.State == databaseDeleteDropped {
		return nil
	}
	fresh := marker == nil
	if fresh {
		exists, identity, err := reach.identify(ctx)
		if err != nil {
			return err
		}
		if !exists {
			// Nothing of that name to drop: the users and records follow.
			if err := p.writeDatabaseDeleteMarker(ctx, databaseID, &databaseDeleteMarker{Name: name, Engine: engine, State: databaseDeleteDropped}); err != nil {
				return fmt.Errorf("record the deletion of database %s: %w", name, err)
			}
			return nil
		}
		marker = &databaseDeleteMarker{Name: name, Engine: engine, Identity: identity, State: databaseDeleteSent}
		// Durable before the drop: a lost answer leaves the identity behind.
		if err := p.writeDatabaseDeleteMarker(ctx, databaseID, marker); err != nil {
			return fmt.Errorf("record the deletion of database %s: %w", name, err)
		}
	}
	var answer databaseDropAnswer
	if !fresh && marker.Identity == "" {
		// A later press without an identity drops nothing: it only reads
		// whether a database of the name is there.
		exists, _, readErr := reach.identify(ctx)
		if readErr != nil {
			return readErr
		}
		answer = databaseDropAnswer{absent: !exists, replaced: exists}
	} else {
		answer, err = reach.drop(ctx, marker.Identity, fresh && marker.Identity == "")
	}
	if err != nil {
		var failed *databaseDeleteError
		if fresh && errors.As(err, &failed) && (!failed.unknown || agentUnreachableAlone(errors.Unwrap(failed.cause))) {
			// Verified: nothing was dropped by this press (a refusal, the
			// engine's failure of the statement, or an Agent not reached).
			// An unknown result keeps the marker and its identity.
			p.clearDatabaseDeleteMarker(ctx, databaseID)
		}
		return err
	}
	if answer.replaced && marker.Identity == "" {
		// The database there now may be the one asked for or another: it is
		// never dropped without knowing. The marker stays as it is.
		return &databaseDeleteError{unverified: true, cause: fmt.Errorf("database %s: not dropped, its identity is not known", name)}
	}
	if answer.replaced {
		log.Printf("database deletion %s: the database of that name on the engine now is another one; it is not dropped", name)
	}
	marker.State = databaseDeleteDropped
	if err := p.writeDatabaseDeleteMarker(ctx, databaseID, marker); err != nil {
		// The identity in the "sent" marker still binds a later press.
		log.Printf("database deletion %s: the engine step is done, its marker was not updated: %v", name, err)
	}
	return nil
}
