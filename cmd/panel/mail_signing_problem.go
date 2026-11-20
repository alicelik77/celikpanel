package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/alicelik/celikpanel/internal/transport"
)

// DKIM signing bound to the sender (20 Nov 2026, tenant isolation; the Agent's
// cmd/agent/mail_signing_bind.go). Agent.WireMailFilters, at every Panel
// start, says whether outgoing mail is signed only for the sender's own
// domain. When it is not (the server owner's opendkim.conf or port-25 milter
// setting was kept, or a change was refused and undone), the state is kept in
// panel_settings and shown in Monitoring → Problems now with who acts and
// what to do (D-024). Reading it never starts a change.
//
// İmzanın göndericiye bağlanmadığı durum İzleme → Şu anki sorunlar'da gösterilir.

const settingMailSigningState = "mail_signing_state_v1"

// recordMailSigningState keeps the Agent's latest state; nil (an older Agent)
// leaves the record as it was.
func (p *Panel) recordMailSigningState(ctx context.Context, state *transport.MailSigningState) {
	if state == nil {
		return
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return
	}
	if err := p.setSetting(ctx, settingMailSigningState, string(raw)); err != nil {
		log.Printf("mail signing state not recorded: %v", err)
	}
}

// mailSigningProblemKinds maps a state to its problem kind.
var mailSigningProblemKinds = map[string]string{
	"conf_kept":   "mail_signing_conf_kept",
	"port25_kept": "mail_signing_port25_kept",
	"failed":      "mail_signing_failed",
	"unsupported": "mail_signing_unsupported",
}

// mailSigningProblems turns the recorded state into Problems now.
func mailSigningProblems(raw string) []monitoringProblem {
	if raw == "" {
		return nil
	}
	var state transport.MailSigningState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return nil
	}
	kind, ok := mailSigningProblemKinds[state.State]
	if !ok {
		return nil
	}
	return []monitoringProblem{{
		Key: "mail_signing", Kind: kind, Severity: "warning", Actor: "server_owner",
		Action: monitoringProblemAction{Kind: "open", To: "/components/postfix"},
	}}
}
