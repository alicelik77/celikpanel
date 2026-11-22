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
