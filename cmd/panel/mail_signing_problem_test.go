package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The Agent's signing state becomes a problem only when mail can still be
// signed for another domain, or a change was refused.
func TestMailSigningStateBecomesAProblem(t *testing.T) {
	raw := func(state string) string {
		b, _ := json.Marshal(transport.MailSigningState{State: state, Detail: "x"})
		return string(b)
	}
	kind := "mail_signing"
	for _, state := range []string{"conf_kept", "port25_kept", "failed", "unsupported", "submit_open", "unknown"} {
		problems := mailSigningProblems(raw(state))
		if len(problems) != 1 || problems[0].Kind != kind || problems[0].Name != state || problems[0].Actor != "server_owner" ||
			problems[0].Action.Kind != "open" || problems[0].Action.To == "" {
			t.Errorf("%s: %+v", state, problems)
		}
		if _, ordered := problemKindOrder[kind]; !ordered {
			t.Errorf("%s has no place in the list's order", kind)
		}
		if en, tr := alertLine(pendingAlertMail{kind: kind}); en == "" || tr == "" || en == "A monitored problem lasts. Open Monitoring for details." {
			t.Errorf("%s has no alert words", kind)
		}
	}
	for _, quiet := range []string{"", raw("bound"), raw("no_dkim"), "not json"} {
		if problems := mailSigningProblems(quiet); len(problems) != 0 {
			t.Errorf("%q: %+v", quiet, problems)
		}
	}
	// Review finding 10: the step names its file or master.cf entry.
	b, _ := json.Marshal(transport.MailSigningState{State: "conf_kept", Subject: "/etc/opendkim/opendkim.conf"})
	if problems := mailSigningProblems(string(b)); len(problems) != 1 || problems[0].Subject != "/etc/opendkim/opendkim.conf" {
		t.Fatalf("subject: %+v", problems)
	}
}

// Review finding 5: every step that can change signing is recorded, and an
// answer that did not arrive is unknown, never an earlier "in place".
func TestMailSigningUnknownReplacesAnEarlierInPlace(t *testing.T) {
	database, err := openTemplatePanelDatabase(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	panel := &Panel{db: database}
	ctx := context.Background()
	panel.recordMailSigningCall(ctx, &transport.MailSigningState{State: "bound"}, nil)
	panel.recordMailSigningCall(ctx, nil, errors.New("connection reset"))
	problems := mailSigningProblems(panel.setting(ctx, settingMailSigningState))
	if len(problems) != 1 || problems[0].Name != "unknown" {
		t.Fatalf("after a lost answer: %+v", problems)
	}
	// The Agent refused before changing anything: the record stays.
	panel.recordMailSigningCall(ctx, &transport.MailSigningState{State: "bound"}, nil)
	panel.recordMailSigningCall(ctx, nil, nil)
	if problems := mailSigningProblems(panel.setting(ctx, settingMailSigningState)); len(problems) != 0 {
		t.Fatalf("a refused step changed the record: %+v", problems)
	}
	// Every Panel call site of a step that changes signing records it.
	for _, file := range []string{"main.go", "service_install_handler.go", "component_remove_together.go", "domain_dkim_automatic.go",
		"domain_mail_extra_handlers.go", "domain_mailauth_handlers.go", "mail_profiles.go", "server_setup_mail_dns.go"} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		text := string(source)
		calls := strings.Count(text, `"Agent.WireMailFilters"`) + strings.Count(text, `"Agent.ConfigureDKIMSigning"`) + strings.Count(text, `"Agent.ConfigureMailStack"`)
		if records := strings.Count(text, "p.recordMailSigningCall("); records != calls {
			t.Errorf("%s: %d calls that change signing, %d record it", file, calls, records)
		}
	}
}
