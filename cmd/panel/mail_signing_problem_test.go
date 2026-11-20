package main

import (
	"encoding/json"
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
	for state, kind := range map[string]string{
		"conf_kept":   "mail_signing_conf_kept",
		"port25_kept": "mail_signing_port25_kept",
		"failed":      "mail_signing_failed",
		"unsupported": "mail_signing_unsupported",
	} {
		problems := mailSigningProblems(raw(state))
		if len(problems) != 1 || problems[0].Kind != kind || problems[0].Actor != "server_owner" ||
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
}
