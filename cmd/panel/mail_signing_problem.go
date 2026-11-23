package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/alicelik/celikpanel/internal/transport"
)

// DKIM signing bound to the sender (20 Nov 2026, tenant isolation; the Agent's
// cmd/agent/mail_signing_bind.go). Every Agent step that can change it (the
// start's Agent.WireMailFilters, the wiring after a component install or
// removal, Agent.ConfigureDKIMSigning and Agent.ConfigureMailStack) says
// whether outgoing mail is signed only for the sender's own domain; since 21
// Nov 2026 (independent review of alpha.143, finding 5) every one of them is
// recorded, and an answer that was not received is recorded as unknown, so an
// earlier "in place" never stands for a result nobody knows. When it is not in
// place the state is kept in panel_settings and shown in Monitoring → Problems
// now with who acts and the concrete step (D-024, finding 10). Reading it
// never starts a change.
//
// İmzanın göndericiye bağlanmadığı durum İzleme → Şu anki sorunlar'da somut
// adımıyla gösterilir; onu değiştirebilen her adım durumu kaydeder.

const settingMailSigningState = "mail_signing_state_v1"

// mailSigningUnknownState is what the Panel records when a step's answer did
// not arrive.
const mailSigningUnknownState = "unknown"

// recordMailSigningState keeps the Agent's latest state; nil (the Agent
// refused the step before it changed anything) leaves the record as it was.
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

// recordMailSigningCall records the outcome of one Agent call that can change
// signing: the state it answered, or unknown when the answer did not arrive
// (the step may or may not have run).
func (p *Panel) recordMailSigningCall(ctx context.Context, state *transport.MailSigningState, callErr error) {
	if callErr != nil && state == nil {
		state = &transport.MailSigningState{State: mailSigningUnknownState,
			Detail: "the Agent's answer to a mail signing step did not arrive: " + callErr.Error()}
	}
	p.recordMailSigningState(ctx, state)
}

// mailSigningProblemStates are the states shown as the problem kind
// mail_signing; the screen words each one with its own concrete step.
var mailSigningProblemStates = map[string]bool{
	"conf_kept": true, "port25_kept": true, "failed": true, "unsupported": true,
	"submit_open": true, mailSigningUnknownState: true,
	// 22 Nov 2026 (second review of alpha.143, B4): a main.cf list read
	// through a variable or a { } group; port 25 follows it, or CelikPanel's
	// own earlier value is still in force.
	"port25_variable": true, "port25_variable_own": true,
}

// mailSigningProblems turns the recorded state into Problems now. Name is the
// state (the screen's words for it), Subject the file or master.cf entry its
// step names.
func mailSigningProblems(raw string) []monitoringProblem {
	if raw == "" {
		return nil
	}
	var state transport.MailSigningState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return nil
	}
	if !mailSigningProblemStates[state.State] {
		return nil
	}
	return []monitoringProblem{{
		Key: "mail_signing", Kind: "mail_signing", Severity: "warning", Actor: "server_owner", Name: state.State,
		Subject: mailSigningSubject(state),
		Action:  monitoringProblemAction{Kind: "open", To: "/components/postfix"},
	}}
}

// mailSigningSubject is the path or entry the step names; fixed defaults when
// an older record has none.
func mailSigningSubject(state transport.MailSigningState) string {
	if state.Subject != "" {
		return state.Subject
	}
	switch state.State {
	case "conf_kept":
		return "/etc/opendkim.conf"
	case "port25_kept":
		return "smtp/inet"
	}
	return ""
}
