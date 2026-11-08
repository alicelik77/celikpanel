package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Site settings within limits (alpha.128, docs/SITE-LIMITS.md, migration 066).
//
// A limited setting has a key ("php.memory_limit_mb") and follows the rights
// chain of D-042: server ⊇ reseller ⊇ plan or Custom ⊇ site. Each level may
// hold a ceiling for the key (setting_policy for the server, setting_ceilings
// for the others); no ceiling at a level is "up to the level above". The
// effective ceiling is the tightest of the levels, with the level that set it
// (set_by), so a refusal can name who can raise it.
//
// The model is generic: a workstream with its own limited setting registers
// its key with registerSettingKey (in an init function), inserts the server
// row in its own migration, and reads subscriptionSettingLimits. Plans,
// reseller plans and Custom sets carry every registered key in their
// "ceilings" object without any further change.
//
// Most keys are ceilings (a site may choose up to the value). A Floor key is
// the opposite: the smallest value a site may choose (the minimum minutes
// between two runs of a scheduled task); its tightest level is the largest.
//
// Sınırlar içinde site ayarları: her anahtar hak zincirini izler (sunucu ⊇
// bayi ⊇ plan ya da Özel ⊇ site). Etkin tavan en sıkı düzeydir; onu koyan
// düzey (set_by) ret iletisinde adıyla geçer.

// settingKeyDef describes one limited setting.
type settingKeyDef struct {
	Key string
	// Group is the screen group ("php", "db", "cron", "mail" ...).
	Group string
	// Unit is how a value is read: "mb", "s", "count", "min".
	Unit string
	// Floor: the value is a minimum (higher is stricter).
	Floor bool
	// Min and Max bound every value of the key (ceilings and site values).
	Min, Max int
	// Presets are the screen's quick choices, smallest first.
	Presets []int
	// CustomerValue (alpha.130): a site's owner chooses a value of its own
	// within the limit (the PHP settings), so the administrator may fix it
	// ("customers may not change it"). The other keys are limits only:
	// nothing for a customer to change, so they offer no such choice.
	CustomerValue bool
}

var (
	settingKeyOrder []string
	settingKeyDefs  = map[string]settingKeyDef{}
)

// registerSettingKey adds a key to the closed set. It panics on a duplicate
// or malformed key: keys are code, never input.
func registerSettingKey(def settingKeyDef) {
	if _, exists := settingKeyDefs[def.Key]; exists || !strings.Contains(def.Key, ".") || def.Min < 0 || def.Max < def.Min {
		panic("invalid setting key " + def.Key)
	}
	settingKeyDefs[def.Key] = def
	settingKeyOrder = append(settingKeyOrder, def.Key)
}

// The keys of this release.
const (
	settingPHPMemoryMB        = "php.memory_limit_mb"
	settingPHPTimeS           = "php.max_execution_time_s"
	settingPHPUploadMB        = "php.upload_max_mb"
	settingPHPInputVars       = "php.max_input_vars"
	settingPHPProcesses       = "php.max_children"
	settingDBConnections      = "db.max_user_connections"
	settingCronJobs           = "cron.max_jobs"
	settingCronMinIntervalMin = "cron.min_interval_minutes"
)

func init() {
	registerSettingKey(settingKeyDef{Key: settingPHPMemoryMB, Group: "php", CustomerValue: true, Unit: "mb", Min: 16, Max: 16384, Presets: []int{64, 128, 256, 512}})
	registerSettingKey(settingKeyDef{Key: settingPHPTimeS, Group: "php", CustomerValue: true, Unit: "s", Min: 5, Max: 3600, Presets: []int{30, 60, 120, 300}})
	registerSettingKey(settingKeyDef{Key: settingPHPUploadMB, Group: "php", CustomerValue: true, Unit: "mb", Min: 1, Max: 10240, Presets: []int{2, 8, 32, 64, 128, 256}})
	registerSettingKey(settingKeyDef{Key: settingPHPInputVars, Group: "php", CustomerValue: true, Unit: "count", Min: 100, Max: 100000, Presets: []int{1000, 3000, 5000, 10000}})
	registerSettingKey(settingKeyDef{Key: settingPHPProcesses, Group: "php", CustomerValue: true, Unit: "count", Min: 1, Max: 200, Presets: []int{2, 5, 10, 20}})
	registerSettingKey(settingKeyDef{Key: settingDBConnections, Group: "db", Unit: "count", Min: 1, Max: 10000, Presets: []int{10, 25, 50, 100}})
	registerSettingKey(settingKeyDef{Key: settingCronJobs, Group: "cron", Unit: "count", Min: 0, Max: 1000, Presets: []int{5, 10, 25, 50}})
	registerSettingKey(settingKeyDef{Key: settingCronMinIntervalMin, Group: "cron", Unit: "min", Floor: true, Min: 1, Max: 1440, Presets: []int{1, 5, 15, 60}})
}

// Levels a limit can come from, in chain order.
const (
	settingSetByServer       = "server"
	settingSetByReseller     = "reseller"
	settingSetByPlan         = "plan"
	settingSetBySubscription = "subscription"
)

// settingLimit is the effective limit of one key for one account.
type settingLimit struct {
	Key   string `json:"key"`
	Group string `json:"group"`
	Unit  string `json:"unit"`
	Floor bool   `json:"floor,omitempty"`
	// Ceiling is the tightest level's value; nil: no level limits the key.
	Ceiling *int `json:"ceiling"`
	// SetBy is the level of Ceiling ("" when nil).
	SetBy string `json:"set_by,omitempty"`
	// SetByName names it for a sentence (the plan's or reseller's name).
	SetByName string `json:"set_by_name,omitempty"`
	// Default is the server default kept within Ceiling; nil: the native
	// default (php.ini, the pool, the database server) applies.
	Default *int `json:"default,omitempty"`
	// Fixed: the administrator fixed the key server-wide; sites show it
	// read-only.
	Fixed   bool  `json:"fixed,omitempty"`
	Min     int   `json:"min"`
	Max     int   `json:"max"`
	Presets []int `json:"presets"`
}

// settingLimits maps a key to its effective limit.
type settingLimits map[string]settingLimit

// allows reports whether value is within the key's effective limit.
func (l settingLimit) allows(value int) bool {
	if value < l.Min || value > l.Max {
		return false
	}
	if l.Ceiling == nil {
		return true
	}
	if l.Floor {
		return value >= *l.Ceiling
	}
	return value <= *l.Ceiling
}

// clamp brings value within the effective limit.
func (l settingLimit) clamp(value int) int {
	if l.Ceiling != nil {
		if l.Floor && value < *l.Ceiling {
			value = *l.Ceiling
		}
		if !l.Floor && value > *l.Ceiling {
			value = *l.Ceiling
		}
	}
	return value
}

// tighten applies one level's value; a looser value is ignored.
func (l *settingLimit) tighten(value int, setBy, name string) {
	if l.Ceiling != nil {
		if l.Floor && value <= *l.Ceiling {
			return
		}
		if !l.Floor && value >= *l.Ceiling {
			return
		}
	}
	v := value
	l.Ceiling, l.SetBy, l.SetByName = &v, setBy, name
}

// ceilingValue is the effective ceiling as a plain number (0: none).
func (l settingLimits) ceilingValue(key string) int {
	if limit, ok := l[key]; ok && limit.Ceiling != nil {
		return *limit.Ceiling
	}
	return 0
}

// settingPolicyRow is the server level of one key.
type settingPolicyRow struct {
	Key              string `json:"key"`
	ServerCeiling    *int   `json:"server_ceiling"`
	ServerDefault    *int   `json:"server_default"`
	CustomerEditable bool   `json:"customer_editable"`
	// CustomerChoice: the key has a value of the customer's own, so
	// CustomerEditable means something (settingKeyDef.CustomerValue).
	CustomerChoice bool `json:"customer_choice"`
}

func nullIntPtr(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int64)
	return &n
}

// loadSettingPolicy reads the server level of every registered key. A key
// without a row has no server ceiling and is customer editable.
func loadSettingPolicy(ctx context.Context, q rowQueryer) (map[string]settingPolicyRow, error) {
	out := map[string]settingPolicyRow{}
	for _, key := range settingKeyOrder {
		out[key] = settingPolicyRow{Key: key, CustomerEditable: true, CustomerChoice: settingKeyDefs[key].CustomerValue}
	}
	rows, err := q.QueryContext(ctx, `SELECT key, server_ceiling, server_default, customer_editable FROM setting_policy`)
	if err != nil {
		return nil, fmt.Errorf("read setting policy: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var ceiling, def sql.NullInt64
		var editable int
		if err := rows.Scan(&key, &ceiling, &def, &editable); err != nil {
			return nil, err
		}
		if _, known := settingKeyDefs[key]; !known {
			continue // a key this release does not know limits nothing
		}
		choice := settingKeyDefs[key].CustomerValue
		// A limit-only key has nothing to fix: it reads as editable whatever
		// an earlier release stored.
		out[key] = settingPolicyRow{Key: key, ServerCeiling: nullIntPtr(ceiling), ServerDefault: nullIntPtr(def),
			CustomerEditable: editable == 1 || !choice, CustomerChoice: choice}
	}
	return out, rows.Err()
}

// loadSettingCeilings reads one owner's ceilings (known keys only).
func loadSettingCeilings(ctx context.Context, q rowQueryer, ownerKind string, ownerID int) (map[string]int, error) {
	out := map[string]int{}
	if ownerID <= 0 {
		return out, nil
	}
	rows, err := q.QueryContext(ctx, `SELECT key, value FROM setting_ceilings WHERE owner_kind = ? AND owner_id = ?`, ownerKind, ownerID)
	if err != nil {
		return nil, fmt.Errorf("read setting ceilings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var value int
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		if _, known := settingKeyDefs[key]; known {
			out[key] = value
		}
	}
	return out, rows.Err()
}

// saveSettingCeilings replaces one owner's ceilings.
func saveSettingCeilings(ctx context.Context, tx sqlExecer, ownerKind string, ownerID int, ceilings map[string]int) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM setting_ceilings WHERE owner_kind = ? AND owner_id = ?`, ownerKind, ownerID); err != nil {
		return fmt.Errorf("replace setting ceilings: %w", err)
	}
	keys := make([]string, 0, len(ceilings))
	for key := range ceilings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, err := tx.ExecContext(ctx, `INSERT INTO setting_ceilings (owner_kind, owner_id, key, value) VALUES (?, ?, ?, ?)`,
			ownerKind, ownerID, key, ceilings[key]); err != nil {
			return fmt.Errorf("save setting ceiling %s: %w", key, err)
		}
	}
	return nil
}

func deleteSettingCeilings(ctx context.Context, tx sqlExecer, ownerKind string, ownerID int) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM setting_ceilings WHERE owner_kind = ? AND owner_id = ?`, ownerKind, ownerID)
	return err
}

// serverSettingLimits is the server level alone (the administrator's field).
func serverSettingLimits(ctx context.Context, q rowQueryer) (settingLimits, error) {
	policy, err := loadSettingPolicy(ctx, q)
	if err != nil {
		return nil, err
	}
	out := settingLimits{}
	for _, key := range settingKeyOrder {
		def := settingKeyDefs[key]
		row := policy[key]
		limit := settingLimit{Key: key, Group: def.Group, Unit: def.Unit, Floor: def.Floor, Min: def.Min, Max: def.Max,
			Presets: append([]int(nil), def.Presets...), Fixed: !row.CustomerEditable}
		if row.ServerCeiling != nil {
			limit.tighten(*row.ServerCeiling, settingSetByServer, "")
		}
		if row.ServerDefault != nil {
			d := *row.ServerDefault
			limit.Default = &d
		}
		out[key] = limit
	}
	return out, nil
}

// withLevel tightens every key by one level's ceilings.
func (l settingLimits) withLevel(ceilings map[string]int, setBy, name string) settingLimits {
	out := settingLimits{}
	for key, limit := range l {
		if value, ok := ceilings[key]; ok {
			limit.tighten(value, setBy, name)
		}
		out[key] = limit
	}
	return out
}

// settled keeps every Default within its Ceiling and clips the presets.
func (l settingLimits) settled() settingLimits {
	out := settingLimits{}
	for key, limit := range l {
		if limit.Default != nil {
			d := limit.clamp(*limit.Default)
			limit.Default = &d
		}
		presets := []int{}
		for _, preset := range limit.Presets {
			if limit.allows(preset) {
				presets = append(presets, preset)
			}
		}
		limit.Presets = presets
		out[key] = limit
	}
	return out
}

// resellerCeilingLayer reads a reseller's own level: its reseller plan's
// ceilings, its Custom ceilings, or none (Unlimited reseller).
func resellerCeilingLayer(ctx context.Context, q rowQueryer, resellerID int) (map[string]int, string, error) {
	state, err := resellerRightsState(ctx, q, resellerID)
	if err != nil {
		return nil, "", err
	}
	switch state.State {
	case rightsStatePlan:
		c, err := loadSettingCeilings(ctx, q, rightsOwnerResellerPlan, state.PlanID)
		return c, state.PlanName, err
	case rightsStateCustom:
		c, err := loadSettingCeilings(ctx, q, rightsOwnerReseller, resellerID)
		return c, "", err
	}
	return map[string]int{}, "", nil
}

// resellerSettingLimits: server ∩ the reseller's own level. It is the field
// a reseller's plans and Custom sets must stay inside.
func resellerSettingLimits(ctx context.Context, q rowQueryer, resellerID int) (settingLimits, error) {
	limits, err := serverSettingLimits(ctx, q)
	if err != nil {
		return nil, err
	}
	layer, _, err := resellerCeilingLayer(ctx, q, resellerID)
	if err != nil {
		return nil, err
	}
	var name string
	_ = q.QueryRowContext(ctx, `SELECT username FROM users WHERE id = ?`, resellerID).Scan(&name)
	return limits.withLevel(layer, settingSetByReseller, name).settled(), nil
}

// subscriptionSettingLimits is the effective limit of every key for the
// sites of one subscription. The administrator's own workspace has the
// server level only.
func subscriptionSettingLimits(ctx context.Context, q rowQueryer, subID int) (settingLimits, error) {
	limits, err := serverSettingLimits(ctx, q)
	if err != nil {
		return nil, err
	}
	own, err := subscriptionRightsState(ctx, q, subID)
	if err != nil {
		return nil, err
	}
	if own.AdminWorkspace {
		return limits.settled(), nil
	}
	_, resellerID, err := subscriptionOwnerChain(ctx, q, subID)
	if err != nil {
		return nil, err
	}
	if resellerID != 0 {
		layer, _, err := resellerCeilingLayer(ctx, q, resellerID)
		if err != nil {
			return nil, err
		}
		var name string
		_ = q.QueryRowContext(ctx, `SELECT username FROM users WHERE id = ?`, resellerID).Scan(&name)
		limits = limits.withLevel(layer, settingSetByReseller, name)
	}
	switch own.State {
	case rightsStatePlan:
		layer, err := loadSettingCeilings(ctx, q, rightsOwnerPlan, own.PlanID)
		if err != nil {
			return nil, err
		}
		limits = limits.withLevel(layer, settingSetByPlan, own.PlanName)
	case rightsStateCustom:
		layer, err := loadSettingCeilings(ctx, q, rightsOwnerSubscription, subID)
		if err != nil {
			return nil, err
		}
		limits = limits.withLevel(layer, settingSetBySubscription, "")
	}
	return limits.settled(), nil
}

// domainSettingLimits: the limits of the subscription a domain is in.
func (p *Panel) domainSettingLimits(ctx context.Context, domainID int) (settingLimits, int, error) {
	subID, err := p.domainSubscriptionID(ctx, domainID)
	if err != nil {
		return nil, 0, err
	}
	limits, err := subscriptionSettingLimits(ctx, p.db.GetDB(), subID)
	return limits, subID, err
}

// errCodeSettingAboveLimit: a value above the effective ceiling (or below
// a floor). Vars: key, value, limit, set_by, set_by_name.
const errCodeSettingAboveLimit = "SETTING_ABOVE_LIMIT"

// errCodeSettingUnknown: a key this release does not know, or a value
// outside the key's fixed range.
const errCodeSettingUnknown = "SETTING_UNKNOWN"

// settingRefusal is one value a level may not take.
type settingRefusal struct {
	Key       string
	Value     int
	Limit     int
	SetBy     string
	SetByName string
}

func (r settingRefusal) line() string {
	return fmt.Sprintf("%s: %d (limit %d, set by %s)", r.Key, r.Value, r.Limit, settingLevelWords(r.SetBy, r.SetByName))
}

func (r settingRefusal) vars() map[string]string {
	return map[string]string{"key": r.Key, "value": strconv.Itoa(r.Value), "limit": strconv.Itoa(r.Limit),
		"set_by": r.SetBy, "set_by_name": r.SetByName}
}

// settingLevelWords names a level in a sentence.
func settingLevelWords(setBy, name string) string {
	switch setBy {
	case settingSetByServer:
		return "the server administrator"
	case settingSetByReseller:
		if name != "" {
			return "your provider (" + name + ")"
		}
		return "your provider"
	case settingSetByPlan:
		if name != "" {
			return "the plan \"" + name + "\""
		}
		return "the plan"
	case settingSetBySubscription:
		return "this subscription's own limits"
	}
	return "nobody"
}

var errSettingUnknown = errors.New("unknown setting")

// validateCeilings checks one level's ceilings against the level above.
// Unknown keys or values outside a key's range are an error; values the
// level above does not allow are returned as refusals.
func validateCeilings(input map[string]int, above settingLimits) (map[string]int, []settingRefusal, error) {
	clean := map[string]int{}
	refusals := []settingRefusal{}
	if len(input) > len(settingKeyOrder) {
		return nil, nil, fmt.Errorf("%w: too many keys", errSettingUnknown)
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := input[key]
		def, known := settingKeyDefs[key]
		if !known {
			return nil, nil, fmt.Errorf("%w: %q", errSettingUnknown, key)
		}
		if value < def.Min || value > def.Max {
			return nil, nil, fmt.Errorf("%w: %s must be between %d and %d", errSettingUnknown, key, def.Min, def.Max)
		}
		limit, ok := above[key]
		if ok && limit.Ceiling != nil && !limit.allows(value) {
			refusals = append(refusals, settingRefusal{Key: key, Value: value, Limit: *limit.Ceiling, SetBy: limit.SetBy, SetByName: limit.SetByName})
			continue
		}
		clean[key] = value
	}
	return clean, refusals, nil
}

// writeSettingAboveLimit answers the first refusal with its vars and every
// refusal as a detail line.
func writeSettingAboveLimit(w http.ResponseWriter, refusals []settingRefusal) {
	first := refusals[0]
	details := make([]string, 0, len(refusals))
	for _, r := range refusals {
		details = append(details, r.line())
	}
	message := fmt.Sprintf("Nothing was changed: %s is above the limit %d set by %s. Choose a value up to the limit, or ask them to raise it.",
		first.Key, first.Limit, settingLevelWords(first.SetBy, first.SetByName))
	if settingKeyDefs[first.Key].Floor {
		message = fmt.Sprintf("Nothing was changed: %s is below the minimum %d set by %s. Choose a value of at least the minimum, or ask them to lower it.",
			first.Key, first.Limit, settingLevelWords(first.SetBy, first.SetByName))
	}
	writeCodedErrorVars(w, http.StatusConflict, errCodeSettingAboveLimit, message, "", details, first.vars())
}

// ceilingsPayload is the JSON form of one level's ceilings: key -> value.
// Absent keys are "up to the level above".
type ceilingsPayload map[string]int
