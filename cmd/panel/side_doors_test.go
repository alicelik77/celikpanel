package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R1: what the side doors step left for someone becomes Problems now;
// what it did by itself (a move, a pool turned off) does not.
func TestSideDoorsProblems(t *testing.T) {
	state := transport.SideDoorsState{
		CheckedAt: "2026-11-20T12:00:00Z",
		Tools: []transport.SideDoorsTool{
			{Tool: "phpmyadmin", Version: "8.4", State: transport.SideDoorsToolMoved, Account: "celikpanel-phpmyadmin"},
			{Tool: "webmail", Version: "8.4", State: transport.SideDoorsToolFailed, Detail: "it did not answer"},
			{Tool: "phppgadmin", Version: "8.4", State: transport.SideDoorsToolOwnerConfig},
			{Tool: "phpmyadmin", Version: "8.4", State: transport.SideDoorsToolUnsupported},
		},
		StockPools: []transport.SideDoorsStockPool{
			{Version: "8.3", Path: "/etc/php/8.3/fpm/pool.d/www.conf", State: transport.SideDoorsStockTurnedOff},
			{Version: "8.4", Path: "/etc/php/8.4/fpm/pool.d/www.conf", State: transport.SideDoorsStockInUse, UsedBy: []string{"/etc/nginx/sites-enabled/a", "/etc/nginx/sites-enabled/b"}},
			{Version: "8.5", Path: "/etc/php/8.5/fpm/pool.d/www.conf", State: transport.SideDoorsStockOwnerChanged},
			{Version: "8.2", Path: "/etc/php/8.2/fpm/pool.d/www.conf", State: transport.SideDoorsStockOnlyPool},
		},
	}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	problems := sideDoorsProblems(string(raw))
	var got []string
	for _, p := range problems {
		got = append(got, p.Kind+"|"+p.Name+"|"+p.Subject+"|"+p.Actor+"|"+p.Action.To)
		if p.Key == "" || p.Severity != "warning" || p.Action.Kind != "open" {
			t.Fatalf("problem %+v", p)
		}
	}
	want := []string{
		"tool_account_failed|Webmail (Roundcube)|8.4|administrator|/components/roundcube",
		"tool_account_owner_config|phpPgAdmin|/etc/nginx/conf.d/celikpanel-dbtools.conf|server_owner|/components/phppgadmin",
		"stock_pool_in_use|8.4|/etc/nginx/sites-enabled/a, /etc/nginx/sites-enabled/b|server_owner|/components/php-fpm",
		"stock_pool_owner_changed|8.5|/etc/php/8.5/fpm/pool.d/www.conf|server_owner|/components/php-fpm",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, kind := range []string{"tool_account_failed", "tool_account_owner_config", "stock_pool_in_use", "stock_pool_owner_changed"} {
		if _, ordered := problemKindOrder[kind]; !ordered {
			t.Fatalf("%s has no place in the problem order", kind)
		}
		if en, tr := alertLine(pendingAlertMail{kind: kind, name: "phpMyAdmin", subject: "/x"}); strings.Contains(en, "A monitored problem lasts") || tr == "" {
			t.Fatalf("%s has no alert sentence", kind)
		}
	}
	if sideDoorsProblems("") != nil || sideDoorsProblems("{") != nil {
		t.Fatal("no state, or an unreadable one, is not a problem")
	}
}
