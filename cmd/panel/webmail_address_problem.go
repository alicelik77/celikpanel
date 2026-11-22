package main

import (
	"context"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Webmail's address in its earlier form (alpha.144, review of PR #355,
// finding 2). The Agent brings every webmail address file in CelikPanel's
// earlier form to the current one at its start; one it could not (the owner
// changed the file: CelikPanel's text lies beside it; or nginx's
// configuration test failed because of another file) still gives out
// non-public files of Roundcube's plugin folders and runs their PHP. It is
// named in Problems now, with why and the step, until the file is in the
// current form (read from the Agent each time: it goes by itself). The
// Agent tries the rewrite again at each start.
//
// Önceki biçimdeki web postası adresi Şimdiki sorunlar'da nedeni ve adımıyla
// gösterilir; dosya yeni biçime geçince kendiliğinden kalkar.

func (p *Panel) webmailAddressProblems(ctx context.Context) []monitoringProblem {
	var resp transport.WebmailAddressFormsResponse
	if err := p.callAgentContext(ctx, "Agent.WebmailAddressFormsV1", &transport.WebmailAddressFormsRequest{}, &resp); err != nil {
		return nil
	}
	var problems []monitoringProblem
	// alpha.145: webmail's own configuration still hands mail to this
	// server's submission service without encryption, so nothing can be sent
	// from webmail. The Agent corrects CelikPanel's own text at its start; an
	// owner's file is kept (CelikPanel's text beside it, Name). The line goes
	// by itself once the file sends with encryption.
	if resp.Sending == "cannot_send" {
		problems = append(problems, monitoringProblem{
			Key: "webmail_cannot_send", Kind: "webmail_cannot_send", Severity: "warning", Actor: "server_owner",
			Name:   resp.SendingFile,
			Action: monitoringProblemAction{Kind: "open", To: "/components/roundcube"},
		})
	}
	for _, open := range resp.Open {
		kind := "webmail_address_kept"
		actor := "server_owner"
		if open.State == "test_failed" {
			kind = "webmail_address_refused"
		}
		problems = append(problems, monitoringProblem{
			Key: kind + ":" + open.Host, Kind: kind, Severity: "warning", Actor: actor,
			Subject: open.Host, Name: open.File,
			Action: monitoringProblemAction{Kind: "open", To: "/components/roundcube"},
		})
	}
	return problems
}
