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
}

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
		value := current[key]
		written, recorded := user.Panel[key]
		switch {
		case recorded && value != written.Raw, !recorded && value != "":
			owner[key] = value
			continue
		}
		want := desired.Value(key)
		if (recorded && written.Value == want) || (!recorded && want == 0) {
			continue
		}
		writes[key] = want
	}
	return writes, owner
}

// applyDBRoleLimits writes the plan's role limits for one user, key by key,
// under the rule above. fresh: the user was just created (an earlier record
// of the same name is forgotten).
func (p *Panel) applyDBRoleLimits(ctx context.Context, limiter services.RoleLimiter, serverID int, username string, desired services.RoleLimits, fresh bool) error {
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
	keys := limiter.RoleKeysSupported()
	current, err := limiter.ReadUserRoleSettings(username)
	if err != nil {
		return err
	}
	writes, owner := planDBRoleWrites(keys, current, user, desired)
	changed := len(owner) != len(user.Owner)
	for k, v := range owner {
		changed = changed || user.Owner[k] != v
	}
	user.Owner = owner
	var firstErr error
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
				delete(writes, k)
			}
		}
		after, err := limiter.ReadUserRoleSettings(username)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		for k, v := range writes {
			if v == 0 {
				delete(user.Panel, k)
			} else {
				user.Panel[k] = dbRoleWritten{Value: v, Raw: after[k]}
			}
		}
		changed = true
	}
	if changed || fresh {
		p.saveDBRoleRecord(ctx, record)
	}
	return firstErr
}

// dbOwnerSetting is one value of a database user the server owner set.
type dbOwnerSetting struct {
	User  string `json:"user"`
	Key   string `json:"key"`
	Value string `json:"value"`
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
				out = append(out, dbOwnerSetting{User: username, Key: "db." + key, Value: value})
			}
		}
	}
	return out
}
