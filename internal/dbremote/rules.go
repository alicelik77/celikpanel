package dbremote

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
)

// Rule lets one database user reach one database from one address.
// Kural: bir veritabanı kullanıcısı, bir veritabanına, bir adresten.
type Rule struct {
	Database string `json:"database"`
	User     string `json:"user"`
	Address  string `json:"address"` // canonical prefix, e.g. 203.0.113.7/32
}

// Target is one engine instance: MariaDB, or one PostgreSQL cluster by port,
// with every rule it should carry.
type Target struct {
	Engine string `json:"engine"`
	Port   int    `json:"port"`
	Rules  []Rule `json:"rules"`
}

// Key names a target in state and answers ("postgresql:5432").
func (t Target) Key() string { return TargetKey(t.Engine, t.Port) }

// TargetKey is Target.Key for loose values.
func TargetKey(engine string, port int) string { return engine + ":" + strconv.Itoa(port) }

// reservedNames are never a rule's user or database: engine superusers,
// pg_hba keywords and the panel's own accounts.
var reservedNames = map[string]bool{
	"all": true, "sameuser": true, "samerole": true, "samegroup": true, "replication": true,
	"postgres": true, "root": true, "mysql": true, "mariadb.sys": true, "debian-sys-maint": true,
	"template0": true, "template1": true, "information_schema": true, "performance_schema": true, "sys": true,
}

// PanelAccountPrefix: the panel's own engine accounts (R-057) start with it.
const PanelAccountPrefix = "celikpanel"

// validName is the panel's identifier rule (services.ValidateSQLIdentifier),
// repeated here so this package depends on nothing, plus the reserved names.
func validName(name string) error {
	if name == "" || len(name) > 63 {
		return errors.New("a name of 1 to 63 characters is required")
	}
	for i, r := range name {
		letter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		digit := r >= '0' && r <= '9'
		if i == 0 && !letter && r != '_' {
			return fmt.Errorf("%q must start with a letter or underscore", name)
		}
		if !letter && !digit && r != '_' {
			return fmt.Errorf("%q holds a character other than a letter, digit or underscore", name)
		}
	}
	lower := strings.ToLower(name)
	if reservedNames[lower] || strings.HasPrefix(lower, PanelAccountPrefix) || strings.HasPrefix(lower, "pg_") {
		return fmt.Errorf("%q is an engine or panel account and is never reachable from outside", name)
	}
	return nil
}

// Normalize checks every rule of a target and answers them sorted, without
// duplicates, with canonical addresses. It enforces the per-user address
// limit. An unknown engine or a bad port is refused.
func (t Target) Normalize() (Target, error) {
	if t.Engine != EngineMySQL && t.Engine != EnginePostgreSQL {
		return Target{}, fmt.Errorf("unknown engine %q", t.Engine)
	}
	if t.Port < 1 || t.Port > 65535 {
		return Target{}, fmt.Errorf("invalid port %d", t.Port)
	}
	out := Target{Engine: t.Engine, Port: t.Port}
	seen := map[Rule]bool{}
	perUser := map[string]map[string]bool{}
	for _, rule := range t.Rules {
		if err := validName(rule.Database); err != nil {
			return Target{}, fmt.Errorf("database: %w", err)
		}
		if err := validName(rule.User); err != nil {
			return Target{}, fmt.Errorf("user: %w", err)
		}
		address, err := CanonicalAddress(t.Engine, rule.Address)
		if err != nil {
			return Target{}, err
		}
		rule.Address = address
		if seen[rule] {
			continue
		}
		seen[rule] = true
		if perUser[rule.User] == nil {
			perUser[rule.User] = map[string]bool{}
		}
		perUser[rule.User][address] = true
		if len(perUser[rule.User]) > MaxAddressesPerUser {
			return Target{}, fmt.Errorf("database user %s would have more than %d addresses", rule.User, MaxAddressesPerUser)
		}
		out.Rules = append(out.Rules, rule)
	}
	sort.Slice(out.Rules, func(i, j int) bool {
		a, b := out.Rules[i], out.Rules[j]
		if a.User != b.User {
			return a.User < b.User
		}
		if a.Database != b.Database {
			return a.Database < b.Database
		}
		return a.Address < b.Address
	})
	return out, nil
}

// Addresses answers a target's distinct addresses, IPv4 first, sorted.
func (t Target) Addresses() (v4, v6 []netip.Prefix) {
	seen := map[netip.Prefix]bool{}
	for _, rule := range t.Rules {
		prefix, err := netip.ParsePrefix(rule.Address)
		if err != nil || seen[prefix] {
			continue
		}
		seen[prefix] = true
		if prefix.Addr().Is4() {
			v4 = append(v4, prefix)
		} else {
			v6 = append(v6, prefix)
		}
	}
	less := func(list []netip.Prefix) func(i, j int) bool {
		return func(i, j int) bool {
			if c := list[i].Addr().Compare(list[j].Addr()); c != 0 {
				return c < 0
			}
			return list[i].Bits() < list[j].Bits()
		}
	}
	sort.Slice(v4, less(v4))
	sort.Slice(v6, less(v6))
	return v4, v6
}

func digest(lines []string) string {
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])[:16]
}
