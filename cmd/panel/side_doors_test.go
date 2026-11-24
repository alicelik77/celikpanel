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

// S4 R1 fix round: a tool that does not answer, an owner's tool pool that
// runs as the web server's account, and every stock pool state that needs
// someone are Problems lines with who acts; the owner's undo offers the
// explicit "turn it off again".
func TestSideDoorsFixRoundProblems(t *testing.T) {
	state := transport.SideDoorsState{
		CheckedAt: "2026-11-21T12:00:00Z",
		Tools: []transport.SideDoorsTool{
			{Tool: "webmail", Version: "8.4", State: transport.SideDoorsToolDown},
			{Tool: "phppgadmin", Version: "8.4", State: transport.SideDoorsToolOwnerConfig, Path: "/etc/php/8.4/fpm/pool.d/celikpanel-phppgadmin.conf"},
		},
		StockPools: []transport.SideDoorsStockPool{
			{Version: "8.1", Path: "/etc/php/8.1/fpm/pool.d/www.conf", State: transport.SideDoorsStockFailed},
			{Version: "8.2", Path: "/etc/php/8.2/fpm/pool.d/www.conf", State: transport.SideDoorsStockNotPutBack},
			{Version: "8.3", Path: "/etc/php/8.3/fpm/pool.d/www.conf", State: transport.SideDoorsStockUnknown},
			{Version: "8.4", Path: "/etc/php/8.4/fpm/pool.d/www.conf", State: transport.SideDoorsStockOwnerRestored},
		},
	}
	raw, _ := json.Marshal(state)
	var got []string
	for _, p := range sideDoorsProblems(string(raw)) {
		got = append(got, p.Kind+"|"+p.Severity+"|"+p.Actor+"|"+p.Subject+"|"+p.Action.Kind+":"+p.Action.To)
	}
	want := []string{
		"tool_down|critical|administrator|8.4|open:/components/roundcube",
		"tool_account_owner_config|warning|server_owner|/etc/php/8.4/fpm/pool.d/celikpanel-phppgadmin.conf|open:/components/phppgadmin",
		"stock_pool_failed|warning|server_owner|/etc/php/8.1/fpm/pool.d/www.conf|open:/components/php-fpm",
		"stock_pool_not_put_back|critical|server_owner|/etc/php/8.2/fpm/pool.d/www.conf|open:/components/php-fpm",
		"stock_pool_unknown|warning|server_owner|/etc/php/8.3/fpm/pool.d/www.conf|open:/components/php-fpm",
		"stock_pool_owner_restored|warning|administrator|/etc/php/8.4/fpm/pool.d/www.conf|reclose:stock_pool",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, kind := range []string{"tool_down", "stock_pool_failed", "stock_pool_not_put_back", "stock_pool_unknown", "stock_pool_owner_restored"} {
		if _, ordered := problemKindOrder[kind]; !ordered {
			t.Fatalf("%s has no place in the problem order", kind)
		}
		if en, tr := alertLine(pendingAlertMail{kind: kind, name: "8.4", subject: "8.4"}); strings.Contains(en, "A monitored problem lasts") || tr == "" {
			t.Fatalf("%s has no alert sentence", kind)
		}
	}
	// The explicit action closes only what the step says is closed.
	closed := &transport.SideDoorsState{StockPools: []transport.SideDoorsStockPool{{Version: "8.4", State: transport.SideDoorsStockTurnedOff}}, At: &transport.SideDoorsAt{State: transport.SideDoorsAtOwnerRemoved}}
	if !sideDoorClosed(closed, transport.SideDoorsReCloseStockPool, "8.4") || sideDoorClosed(closed, transport.SideDoorsReCloseAt, "") {
		t.Fatal("sideDoorClosed")
	}
}
