package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R1 (21 Nov 2026): crontab lines not made in CelikPanel are ONE line for
// the administrator naming the sites and how many lines; a site that is gone
// is left out. at: the website accounts' waiting jobs, an owner's at.allow
// that names them, and at left open are each a line with who acts; what the
// step did by itself is not.
func TestSiteJobsProblems(t *testing.T) {
	outside := siteCrontabsOutside{CheckedAt: "2026-11-21T10:00:00Z", Sites: map[string]int{"7": 2, "9": 1, "40": 5}}
	problems := siteCrontabOutsideProblem(outside, map[int]string{7: "shop.example", 9: "blog.example"})
	if len(problems) != 1 {
		t.Fatalf("problems %+v", problems)
	}
	p := problems[0]
	if p.Kind != "site_crontab_outside" || p.Value != 3 || p.Subject != "blog.example (1), shop.example (2)" || p.Actor != "administrator" ||
		p.Action.Kind != "open" || p.Action.To != "/domains/blog.example?tab=cron" {
		t.Fatalf("problem %+v", p)
	}
	if siteCrontabOutsideProblem(siteCrontabsOutside{Sites: map[string]int{"40": 5}}, map[int]string{}) != nil {
		t.Fatal("a site that is gone is not a problem")
	}

	state := transport.SideDoorsState{CheckedAt: "2026-11-21T10:00:00Z", At: &transport.SideDoorsAt{
		State: transport.SideDoorsAtOwnerFile, Path: "/etc/at.allow", SiteAccounts: []string{"shop_example"},
		Jobs: []transport.SideDoorsAtJob{{ID: "3", Account: "shop_example", When: "Sat Nov 22 10:00:00 2026"}},
	}}
	var got []string
	for _, p := range siteAtProblems(state) {
		got = append(got, p.Kind+"|"+p.Subject+"|"+p.Actor)
	}
	want := "site_at_jobs|#3 shop_example, Sat Nov 22 10:00:00 2026|administrator\nat_allow_lists_sites|shop_example|server_owner"
	if strings.Join(got, "\n") != want {
		t.Fatalf("at problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), want)
	}
	for atState, kind := range map[string]string{
		transport.SideDoorsAtDirectory:    "at_directory_accounts",
		transport.SideDoorsAtFailed:       "at_not_closed",
		transport.SideDoorsAtOwnerRemoved: "at_owner_removed",
		transport.SideDoorsAtWritten:      "",
		transport.SideDoorsAtCurrent:      "",
		transport.SideDoorsAtOnlyRoot:     "",
		transport.SideDoorsAtNotInstalled: "",
	} {
		problems := siteAtProblems(transport.SideDoorsState{At: &transport.SideDoorsAt{State: atState, Path: "/etc/at.allow"}})
		if (kind == "") != (len(problems) == 0) || (kind != "" && problems[0].Kind != kind) {
			t.Fatalf("%s: %+v", atState, problems)
		}
	}
	if siteAtProblems(transport.SideDoorsState{}) != nil {
		t.Fatal("an older Agent's answer has no at part")
	}
	for _, kind := range []string{"site_crontab_outside", "site_at_jobs", "at_allow_lists_sites", "at_directory_accounts", "at_not_closed"} {
		if _, ordered := problemKindOrder[kind]; !ordered {
			t.Fatalf("%s has no place in the problem order", kind)
		}
		if en, tr := alertLine(pendingAlertMail{kind: kind, name: "x", subject: "x", value: 2}); strings.Contains(en, "A monitored problem lasts") || tr == "" {
			t.Fatalf("%s has no alert sentence", kind)
		}
	}
}

// S4 R1 fix round: the owner's deletion of at.allow is one line with the
// administrator's explicit "close it again"; an unreadable crontab is named
// once with why.
func TestSiteJobsFixRoundProblems(t *testing.T) {
	problems := siteAtProblems(transport.SideDoorsState{At: &transport.SideDoorsAt{State: transport.SideDoorsAtOwnerRemoved, Path: "/etc/at.allow"}})
	if len(problems) != 1 || problems[0].Actor != "administrator" || problems[0].Severity != "warning" ||
		problems[0].Action.Kind != "reclose" || problems[0].Action.To != transport.SideDoorsReCloseAt {
		t.Fatalf("%+v", problems)
	}
	unreadable := siteCrontabUnreadableProblem(siteCrontabsOutside{Unreadable: map[string]string{"shop_example": "crontab: user listed in cron.deny", "blog_example": "permission denied"}})
	if len(unreadable) != 1 || unreadable[0].Subject != "blog_example, shop_example" || unreadable[0].Name != "permission denied" || unreadable[0].Value != 2 {
		t.Fatalf("%+v", unreadable)
	}
	for _, kind := range []string{"at_owner_removed", "site_crontab_unreadable"} {
		if _, ordered := problemKindOrder[kind]; !ordered {
			t.Fatalf("%s has no place in the problem order", kind)
		}
		if en, tr := alertLine(pendingAlertMail{kind: kind, name: "x", subject: "x"}); strings.Contains(en, "A monitored problem lasts") || tr == "" {
			t.Fatalf("%s has no alert sentence", kind)
		}
	}
}
