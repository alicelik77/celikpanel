package core

import (
	"slices"
	"testing"
)

func TestComponentRulesShape(t *testing.T) {
	if err := ValidateComponentRules(); err != nil {
		t.Fatal(err)
	}
}

func TestComponentRulesFillCatalogue(t *testing.T) {
	cases := map[string]struct {
		requires []string
		seat     string
	}{
		"phpmyadmin":        {[]string{"mariadb", SeatWebServer, "php-fpm"}, ""},
		"roundcube":         {[]string{SeatSMTPServer, SeatIMAPServer, SeatWebServer, "php-fpm"}, ""},
		"nginx":             {nil, SeatWebServer},
		"rspamd":            {[]string{SeatSMTPServer}, SeatSpamFilter},
		"fail2ban":          {[]string{"nftables"}, ""},
		"valkey":            {nil, SeatKVStore},
		"memcached":         {nil, ""},
		"node":              {[]string{SeatWebServer}, ""},
		"pdns":              {nil, SeatDNSServer},
		NativeCronServiceID: {nil, ""},
	}
	for id, want := range cases {
		svc := GetManagedServiceByID(id)
		if svc == nil {
			t.Fatalf("%s missing", id)
		}
		if !slices.Equal(svc.Requires, want.requires) || svc.ConflictGroup != want.seat {
			t.Errorf("%s: requires %v seat %q, want %v %q", id, svc.Requires, svc.ConflictGroup, want.requires, want.seat)
		}
	}
}

func TestComponentRulesDetectBrokenMaps(t *testing.T) {
	saved := ComponentRules
	defer func() { ComponentRules = saved }()
	clone := func() map[string]ComponentRule {
		out := map[string]ComponentRule{}
		for k, v := range saved {
			out[k] = v
		}
		return out
	}

	ComponentRules = clone()
	delete(ComponentRules, "netdata")
	if ValidateComponentRules() == nil {
		t.Error("a component without a rule passed")
	}

	ComponentRules = clone()
	r := ComponentRules["nginx"]
	r.Conflicts = nil
	ComponentRules["nginx"] = r
	if ValidateComponentRules() == nil {
		t.Error("an asymmetric conflict passed")
	}

	ComponentRules = clone()
	r = ComponentRules["nftables"]
	r.Requires = []string{"fail2ban"}
	ComponentRules["nftables"] = r
	if err := ValidateComponentRules(); err == nil {
		t.Error("a requirement cycle passed")
	}
}

func TestRequiringComponents(t *testing.T) {
	installed := map[string]bool{"nginx": true, "php-fpm": true, "mariadb": true, "phpmyadmin": true, "node": true,
		"postfix": true, "dovecot": true, "roundcube": true, "rspamd": true, "nftables": true, "fail2ban": true}
	if got := RequiringComponents("nginx", installed); !slices.Equal(got, []string{"node", "phpmyadmin", "roundcube"}) {
		t.Errorf("nginx: %v", got)
	}
	if got := RequiringComponents("postfix", installed); !slices.Equal(got, []string{"rspamd", "roundcube"}) {
		t.Errorf("postfix: %v", got)
	}
	if got := RequiringComponents("nftables", installed); !slices.Equal(got, []string{"fail2ban"}) {
		t.Errorf("nftables: %v", got)
	}
	// Another provider of the seat keeps the seat filled.
	installed["apache"] = true
	if got := RequiringComponents("nginx", installed); len(got) != 0 {
		t.Errorf("nginx with apache: %v", got)
	}
	if !RemovableTogether("mariadb", "phpmyadmin", installed) || RemovableTogether("nftables", "fail2ban", installed) ||
		RemovableTogether("apache", "node", installed) {
		t.Error("together rule wrong")
	}
}

func TestRemoveTogetherQualifier(t *testing.T) {
	q := RemoveTogetherQualifier([]string{"roundcube", "rspamd", "roundcube"})
	if q != "roundcube,rspamd" {
		t.Fatalf("qualifier %q", q)
	}
	if members, err := ParseRemoveTogetherQualifier(q); err != nil || len(members) != 2 {
		t.Fatalf("parse %v %v", members, err)
	}
	for _, bad := range []string{"", "rspamd,roundcube", "nginx", "phpmyadmin,unknown"} {
		if _, err := ParseRemoveTogetherQualifier(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestMayBeRequired(t *testing.T) {
	for id, want := range map[string]bool{"nginx": true, "postgresql": true, "nftables": true, "postfix": true, "php-fpm": true,
		"roundcube": false, "redis": false, "rspamd": false, "certbot": false, "vsftpd": false} {
		if MayBeRequired(id) != want {
			t.Errorf("%s: MayBeRequired = %v", id, !want)
		}
	}
}
