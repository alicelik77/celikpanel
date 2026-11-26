package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"sync"

	"github.com/alicelik/celikpanel/internal/services"
)

// The panel never resets a database user's setting it did not set (S4 R2a,
// review finding 2; AGENTS.md: "Detect owner configuration changes rather
// than silently overwrite them").
//
// For every user and every time or temporary-file key the panel records what
// it wrote itself (panel_settings db_role_limits_v1; no schema change): the
// plan's value and the engine's own text of it, read back after the write.
// Before writing a key it reads the user's current values:
//
//   - the current value is what the panel wrote (or nothing, and the panel
//     never wrote one): the panel may change it to the plan's value, and
//     writes only when that value differs (so a change of another key, such
//     as db.connections, writes nothing here);
//   - anything else is the server owner's (changed by hand, or set before
//     the panel wrote anything): it is kept, recorded as the owner's and shown
//     on the site's card for the administrator, and never written over.
//
// A restore or copy never touches a user's settings (database_load_limits.go
// in the Agent).
//
// Panel yalnız kendi yazdığı veritabanı kullanıcı ayarını değiştirir;
// sunucu sahibinin elle koyduğu değer korunur ve sitenin kartında gösterilir.

const settingDBRoleLimitsRecord = "db_role_limits_v1"

type dbRoleWritten struct {
	Value int    `json:"value"`
	Raw   string `json:"raw"`
	// Unknown (alpha.149, review of PR #370 finding 4): the write may have
	// landed but its answer was lost. The value stays the panel's either way:
	// Raw (landed) or Prev (the panel's earlier text, not landed) is taken as
	// the panel's, and the next round writes again until a read-back confirms
	// Raw. An older panel reads only Value and Raw.
	Unknown bool   `json:"unknown,omitempty"`
	Prev    string `json:"prev,omitempty"`
}

// dbRoleKeyConnections (alpha.149, finding 3) records the PostgreSQL
// connection limit the panel wrote, under the same rule as the role keys;
// its text is pg_roles.rolconnlimit ("-1": none). MariaDB's connection limit
// keeps its rule of alpha.128 and is not recorded. An older panel reads only
// services.RoleKeys and passes this key by.
const dbRoleKeyConnections = "connections"

// dbConnectionNone is rolconnlimit's text for no limit.
const dbConnectionNone = "-1"

type dbRoleUserRecord struct {
	// Panel: what the panel wrote, per key.
	Panel map[string]dbRoleWritten `json:"panel,omitempty"`
	// Owner: values the panel found and did not write (the owner's), per key.
	Owner map[string]string `json:"owner,omitempty"`
}

type dbRoleRecord map[string]*dbRoleUserRecord

var dbRoleRecordMu sync.Mutex

func dbRoleRecordKey(serverID int, username string) string {
	return strconv.Itoa(serverID) + "/" + username
}

func (p *Panel) loadDBRoleRecord(ctx context.Context) dbRoleRecord {
	record := dbRoleRecord{}
	if raw := p.setting(ctx, settingDBRoleLimitsRecord); raw != "" {
		_ = json.Unmarshal([]byte(raw), &record)
	}
	return record
}

func (p *Panel) saveDBRoleRecord(ctx context.Context, record dbRoleRecord) {
	for key, user := range record {
		if len(user.Panel) == 0 && len(user.Owner) == 0 {
			delete(record, key)
		}
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return
	}
	if err := p.setSetting(ctx, settingDBRoleLimitsRecord, string(raw)); err != nil {
		log.Printf("database time limits: what the panel wrote was not recorded: %v", err)
	}
}

// planDBRoleWrites decides, from the current values and the record, which
// keys the panel writes (key -> value) and which are the owner's (key ->
// current value). It changes nothing.
func planDBRoleWrites(keys []string, current services.RoleSettings, user *dbRoleUserRecord, desired services.RoleLimits) (map[string]int, map[string]string) {
	writes, owner := map[string]int{}, map[string]string{}
	for _, key := range keys {
		write, ownerValue, isOwner := planDBRecordedKey(current[key], "", user.Panel[key], hasKey(user.Panel, key), desired.Value(key))
		switch {
		case isOwner:
			owner[key] = ownerValue
		case write:
			writes[key] = desired.Value(key)
		}
	}
	return writes, owner
}

func hasKey(m map[string]dbRoleWritten, key string) bool {
	_, ok := m[key]
	return ok
}

// planDBRecordedKey is the rule for one recorded key: value is the engine's
// text now, none its text for "not set". It answers whether the panel
// writes want, or whether the value is someone else's (kept).
func planDBRecordedKey(value, none string, written dbRoleWritten, recorded bool, want int) (write bool, ownerValue string, isOwner bool) {
	landed := recorded && value == written.Raw
	panels := landed || (recorded && written.Unknown && value == written.Prev)
	switch {
	case recorded && !panels, !recorded && value != none:
		return false, value, true
	}
	if (landed && written.Value == want) || (!recorded && want == 0) {
		return false, "", false
	}
	return true, "", false
}

// confirmDBRecordedKeys clears the unknown mark of keys whose written text a
// read now shows (a read-back confirmed them); true when one changed.
func confirmDBRecordedKeys(user *dbRoleUserRecord, current map[string]string) bool {
	changed := false
	for key, written := range user.Panel {
		if written.Unknown && current[key] == written.Raw {
			written.Unknown, written.Prev = false, ""
			user.Panel[key] = written
			changed = true
		}
	}
	return changed
}

// recordDBUnknownWrite keeps a key whose write's answer was lost as the
// panel's: the text it wrote, and the earlier text (alpha.149, finding 4).
func recordDBUnknownWrite(user *dbRoleUserRecord, key string, value int, text string) {
	previous, recorded := user.Panel[key]
	prev := previous.Raw
	if previous.Unknown {
		prev = previous.Prev
	}
	if !recorded {
		prev = ""
		if key == dbRoleKeyConnections {
			prev = dbConnectionNone
		}
	}
	user.Panel[key] = dbRoleWritten{Value: value, Raw: text, Unknown: true, Prev: prev}
}

// applyDBRoleLimits writes the plan's role limits for one user, key by key,
// under the rule above. fresh: the user was just created (an earlier record
// of the same name is forgotten).
func (p *Panel) applyDBRoleLimits(ctx context.Context, limiter services.RoleLimiter, serverID int, username string, desired services.RoleLimits, fresh bool) error {
	current, err := limiter.ReadUserRoleSettings(username)
	if err != nil {
		return err
	}
	return p.applyDBRoleLimitsFrom(ctx, limiter, serverID, username, desired, fresh, current, nil)
}

// dbConnectionStep is a PostgreSQL role's connection limit under the record
// rule (alpha.149, review of PR #370 finding 3): the panel records what it
// wrote; a value that differs is the owner's and is kept; the limit is
// written only when the plan's value changed, or for a user without one
// when the plan sets one (a new user among them). A value equal to the
// plan's that no record names (alpha.128 wrote it on PostgreSQL 15 or older)
// is taken as the panel's without a write.
type dbConnectionStep struct {
	current int // rolconnlimit read before anything is written (-1: none)
	want    int // the plan's limit (0: none)
	// set writes limit and answers the value read back in the same call.
	set func(limit int) (int, error)
}

func dbConnectionText(limit int) string {
	if limit <= 0 {
		return dbConnectionNone
	}
	return strconv.Itoa(limit)
}

// applyDBRoleLimitsFrom is applyDBRoleLimits with the user's current values
// already read (alpha.149: read before anything is written, so a state that
// cannot be read writes nothing at all). conn, when not nil, is the
// PostgreSQL connection limit under the same rule.
func (p *Panel) applyDBRoleLimitsFrom(ctx context.Context, limiter services.RoleLimiter, serverID int, username string, desired services.RoleLimits, fresh bool, current services.RoleSettings, conn *dbConnectionStep) error {
	dbRoleRecordMu.Lock()
	defer dbRoleRecordMu.Unlock()
	record := p.loadDBRoleRecord(ctx)
	key := dbRoleRecordKey(serverID, username)
	if fresh {
		delete(record, key)
	}
	user := record[key]
	if user == nil {
		user = &dbRoleUserRecord{}
		record[key] = user
	}
	if user.Panel == nil {
		user.Panel = map[string]dbRoleWritten{}
	}
	seen := map[string]string{}
	for k, v := range current {
		seen[k] = v
	}
	if conn != nil {
		seen[dbRoleKeyConnections] = strconv.Itoa(conn.current)
	}
	changed := confirmDBRecordedKeys(user, seen)
	keys := limiter.RoleKeysSupported()
	writes, owner := planDBRoleWrites(keys, current, user, desired)
	var firstErr error
	if conn != nil {
		text := strconv.Itoa(conn.current)
		written, recorded := user.Panel[dbRoleKeyConnections]
		write, ownerValue, isOwner := planDBRecordedKey(text, dbConnectionNone, written, recorded, conn.want)
		switch {
		case !recorded && conn.want > 0 && conn.current == conn.want:
			user.Panel[dbRoleKeyConnections] = dbRoleWritten{Value: conn.want, Raw: text}
			changed = true
		case isOwner:
			owner[dbRoleKeyConnections] = ownerValue
		case write:
			readBack, err := conn.set(conn.want)
			switch {
			case err == nil && conn.want == 0:
				delete(user.Panel, dbRoleKeyConnections)
			case err == nil:
				user.Panel[dbRoleKeyConnections] = dbRoleWritten{Value: conn.want, Raw: strconv.Itoa(readBack)}
			case dbLimitStateOf(err) == dbLimitUnknown:
				recordDBUnknownWrite(user, dbRoleKeyConnections, conn.want, dbConnectionText(conn.want))
			}
			if err != nil {
				firstErr = fmt.Errorf("connection limit: %w", err)
			}
			changed = true
		}
	}
	ownerChanged := len(owner) != len(user.Owner)
	for k, v := range owner {
		ownerChanged = ownerChanged || user.Owner[k] != v
	}
	changed = changed || ownerChanged
	user.Owner = owner
	if len(writes) > 0 {
		names := make([]string, 0, len(writes))
		for k := range writes {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			if err := limiter.SetUserRoleSetting(username, k, writes[k]); err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("%s: %w", k, err)
				}
				if dbLimitStateOf(err) == dbLimitUnknown {
					// alpha.149 (finding 4): the write may have landed; the
					// key stays the panel's with the text it wrote, so the
					// next round reaches it and the line stays until a
					// read-back confirms.
					text := ""
					if writes[k] != 0 {
						text = dbRoleWrittenText(k, writes[k])
					}
					recordDBUnknownWrite(user, k, writes[k], text)
				}
				delete(writes, k)
			}
		}
		after, err := limiter.ReadUserRoleSettings(username)
		if err != nil && firstErr == nil {
			// alpha.149: the writes were accepted; what the engine holds now
			// was not read, so the result is not known.
			firstErr = &dbLimitError{state: dbLimitUnknown, err: fmt.Errorf("read back: %w", err)}
		}
		for k, v := range writes {
			if v == 0 {
				delete(user.Panel, k)
				continue
			}
			raw := after[k]
			if err != nil {
				// alpha.149: never an empty text for a value the engine
				// accepted, which the next round would take for the owner's;
				// the engines keep it as written ("<n>s", "<n>MB").
				raw = dbRoleWrittenText(k, v)
			}
			user.Panel[k] = dbRoleWritten{Value: v, Raw: raw}
		}
		changed = true
	}
	if changed || fresh {
		p.saveDBRoleRecord(ctx, record)
	}
	return firstErr
}

// forgetDBRoleRecord forgets what the panel recorded for a user name on a
// server (alpha.149, finding 5): a new user of an earlier user's name starts
// with no record.
func (p *Panel) forgetDBRoleRecord(ctx context.Context, serverID int, username string) {
	dbRoleRecordMu.Lock()
	defer dbRoleRecordMu.Unlock()
	record := p.loadDBRoleRecord(ctx)
	if _, ok := record[dbRoleRecordKey(serverID, username)]; !ok {
		return
	}
	delete(record, dbRoleRecordKey(serverID, username))
	p.saveDBRoleRecord(ctx, record)
}

// dbOwnerSetting is one value of a database user the panel did not write.
type dbOwnerSetting struct {
	User  string `json:"user"`
	Key   string `json:"key"`
	Value string `json:"value"`
	// Origin (alpha.149, review of PR #370 finding 13) says what is known
	// about who set it: "owner" for a real limit only an administrator
	// account can set (temporary files, the connection limit); "on_user" for
	// a default the database user's own session may change on itself
	// (statement time, idle transaction time) - it was changed on the user
	// itself, by the owner or by the user, which the panel cannot tell
	// apart. It is never imposed again: by design it is a default the user
	// may change (docs/SITE-LIMITS.md).
	Origin string `json:"origin"`
}

// dbOwnerSettingOrigin is the Origin of a recorded key.
func dbOwnerSettingOrigin(key string) string {
	if key == services.RoleKeyStatementSeconds || key == services.RoleKeyIdleInTransactionSeconds {
		return "on_user"
	}
	return "owner"
}

// dbOwnerSettingsOf lists the owner's values of a subscription's database
// users (from the record; no engine is asked).
func (p *Panel) dbOwnerSettingsOf(ctx context.Context, subscriptionID int) []dbOwnerSetting {
	out := []dbOwnerSetting{}
	if p.db == nil {
		return out
	}
	dbRoleRecordMu.Lock()
	record := p.loadDBRoleRecord(ctx)
	dbRoleRecordMu.Unlock()
	if len(record) == 0 {
		return out
	}
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT server_id, username FROM database_users WHERE subscription_id = ? ORDER BY username`, subscriptionID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var serverID int
		var username string
		if rows.Scan(&serverID, &username) != nil {
			continue
		}
		user := record[dbRoleRecordKey(serverID, username)]
		if user == nil {
			continue
		}
		for _, key := range services.RoleKeys {
			if value, ok := user.Owner[key]; ok {
				out = append(out, dbOwnerSetting{User: username, Key: "db." + key, Value: value, Origin: dbOwnerSettingOrigin(key)})
			}
		}
		// alpha.149 (finding 3): the owner's PostgreSQL connection limit.
		if value, ok := user.Owner[dbRoleKeyConnections]; ok {
			out = append(out, dbOwnerSetting{User: username, Key: settingDBConnections, Value: value, Origin: "owner"})
		}
	}
	return out
}
