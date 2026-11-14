package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The answer to a certificate request that certbot ran and did not fulfil
// (11 Oct 2026; D-024).
//
// It used to be `500 INTERNAL` "internal server error" for every such run,
// with the cause only in the server's log. The Agent now says which of five
// kinds the run was (transport.CertificateFailure*), read from certbot's own
// output, and the answer is `502 CERTIFICATE_ISSUE_FAILED` with that kind as
// `reason`. Each sentence says only what that kind establishes, what is in
// place now, who acts and that nothing asks again by itself.
//
// `vars`: `domain`; `kept` (`previous` or `none`); `detail`, one bounded line
// of certbot's own words, and `record`, the file where certbot keeps its
// record, for an administrator only (they can name paths and hosts of this
// server); `check` for reason issued_check.
//
// What stays in place is the same for every kind: the request added no
// certificate, and a certificate the site already had keeps serving. The
// handler removes what certbot left of the unfinished request before it
// answers.
//
// certbot'un çalışıp yerine getirmediği bir sertifika isteğinin yanıtı. Eskiden
// her durumda `500` "internal server error" idi. Yanıt artık nedenin türünü,
// şu an neyin yerinde olduğunu, kimin ne yapacağını ve hiçbir şeyin
// kendiliğinden yeniden denemediğini söyler.
const errCodeCertificateIssueFailed = "CERTIFICATE_ISSUE_FAILED"

const (
	certificateIssueKept   = " The certificate this site already had is still in place and keeps serving."
	certificateIssueNone   = " The site has no certificate from this request and is served as before."
	certificateIssueResume = " Nothing asks again automatically."
)

// alpha.134 lab: the sentences no longer send anyone to a file on the server
// (a customer or reseller has none). Each says what is established, who acts
// and the next action; certbot's own line (and where certbot keeps its
// record) is `vars.detail` for an administrator only. The page words them
// for the reader's role.
var certificateIssueFailedMessages = map[string]string{
	transport.CertificateFailureAuthorityUnreachable: "No certificate was issued: this server could not reach the certificate authority, so nothing was asked of it. " +
		"The server administrator checks that this server can open HTTPS connections to the internet (name resolution, outbound port 443, the system clock), then requests the certificate here again.",
	transport.CertificateFailureValidation: "No certificate was issued: the certificate authority could not confirm that one of the site's names points to this server. " +
		"Point each name of the site to this server in the domain's DNS records and wait until public DNS shows it; if they already point here, the server administrator checks that port 80 is open from the internet. " +
		"Then request the certificate here again.",
	transport.CertificateFailureRateLimited: "No certificate was issued: the certificate authority refused for now because one of its request limits was reached. " +
		"A request before the limit resets is refused the same way and counts against it; request the certificate here again later.",
	transport.CertificateFailureAccountUnknown: "No certificate was issued: the certificate authority does not know the account this server uses with it. " +
		"The server administrator replaces certbot's account for this authority on the server (sudo certbot show_account shows it), then requests the certificate here again.",
	transport.CertificateFailureTimeout: "No certificate was issued: the request to the certificate authority did not finish in time and was stopped; why is not known. " +
		"Request the certificate here again later; if it stops the same way, the server administrator reads certbot's record on the server.",
	transport.CertificateFailureTool: "No certificate was issued, and the reason could not be established from certbot's answer. " +
		"Request the certificate here again later; if it fails the same way, the server administrator reads certbot's record on the server.",
	transport.CertificateFailureIssuedCheck: issuedCheckMessages[issuedCheckUnusable],
}

// The checks a just-issued certificate must pass before it is installed
// (alpha.134 lab: a chain this server does not trust answered a bare 500).
// vars.check names which one failed.
const (
	issuedCheckUntrusted = "untrusted"
	issuedCheckNames     = "names"
	issuedCheckUnusable  = "unusable"
)

var issuedCheckMessages = map[string]string{
	issuedCheckUntrusted: "The certificate authority issued a certificate, but this server does not trust the authority's certificate chain, so CelikPanel did not install it and removed it again. " +
		"The server administrator updates this server's trusted certificate authorities (the ca-certificates package), then requests the certificate here again.",
	issuedCheckNames: "The certificate authority issued a certificate that does not carry exactly the names that were asked for, so CelikPanel did not install it and removed it again. " +
		"Request the certificate here again; if it happens again, the server administrator reads the Panel's log (sudo journalctl -u celikpanel-panel), which names the difference.",
	issuedCheckUnusable: "The certificate authority issued a certificate, but it did not pass CelikPanel's check before use, so CelikPanel did not install it and removed it again. " +
		"Request the certificate here again; if it happens again, the server administrator reads the Panel's log (sudo journalctl -u celikpanel-panel), which names the check.",
}

// certbotRecordPath is where certbot keeps its own record of a run into the
// standard store (D-040): named to an administrator, never to a customer.
const certbotRecordPath = "/var/log/letsencrypt/letsencrypt.log"

// writeIssuedCertificateCheckFailure answers a certificate the authority
// issued that did not pass the check before use: 409 CERTIFICATE_ISSUE_FAILED
// with reason issued_check and vars.check, never a bare 500. The caller
// removes the certificate (the deferred cleanup of the request).
func writeIssuedCertificateCheckFailure(w http.ResponseWriter, caller *Caller, domain string, hadCertificate bool, check, detail string) {
	sentence, known := issuedCheckMessages[check]
	if !known {
		check, sentence = issuedCheckUnusable, issuedCheckMessages[issuedCheckUnusable]
	}
	if hadCertificate {
		sentence += certificateIssueKept
	} else {
		sentence += certificateIssueNone
	}
	sentence += certificateIssueResume
	vars := map[string]string{"domain": domain, "kept": "none", "check": check}
	if hadCertificate {
		vars["kept"] = "previous"
	}
	if caller != nil && caller.Role == roleAdmin {
		if line := boundedAgentDiagnostic(detail); line != "" {
			vars["detail"] = line
		}
	}
	log.Printf("[409][certificate] %s: %s %s %s: %s", domain, errCodeCertificateIssueFailed, transport.CertificateFailureIssuedCheck, check, boundedAgentDiagnostic(detail))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(apiErrorBody{Error: sentence, Code: errCodeCertificateIssueFailed, Reason: transport.CertificateFailureIssuedCheck, Vars: vars})
}

// certificateClockFailureMessage is the sentence of a certificate the
// authority issued and this server's clock does not accept (alpha.113).
const certificateClockFailureMessage = "The certificate authority issued a certificate, but this server's clock does not accept it: " +
	"it is valid from %s and this server's time was %s. CelikPanel did not install it and removed it again, so no certificate from this request is in place." +
	" The server administrator corrects this server's clock: run timedatectl and check that \"System clock synchronized\" says yes " +
	"(sudo timedatectl set-ntp true turns time synchronisation on), then requests the certificate here again."

// writeCertificateClockFailure answers a just-issued certificate that is not
// yet valid (or already expired) by this server's clock beyond the wait
// inspectIssuedCertificate allows: 409 CERTIFICATE_ISSUE_FAILED with reason
// server_clock, never a bare 500. The caller removes the certificate.
func writeCertificateClockFailure(w http.ResponseWriter, caller *Caller, domain string, hadCertificate bool, info installedCertificateInfo) {
	validFrom := info.IssuedAt.UTC().Format(time.RFC3339)
	if info.Expired {
		validFrom = info.IssuedAt.UTC().Format(time.RFC3339) + " until " + info.ExpiresAt.UTC().Format(time.RFC3339)
	}
	serverTime := info.CheckedAt.UTC().Format(time.RFC3339)
	sentence := fmt.Sprintf(certificateClockFailureMessage, validFrom, serverTime)
	if hadCertificate {
		sentence += certificateIssueKept
	} else {
		sentence += certificateIssueNone
	}
	sentence += certificateIssueResume
	vars := map[string]string{
		"domain": domain, "kept": "none",
		"valid_from": info.IssuedAt.UTC().Format(time.RFC3339), "valid_until": info.ExpiresAt.UTC().Format(time.RFC3339),
		"server_time": serverTime, "command": "timedatectl",
	}
	if hadCertificate {
		vars["kept"] = "previous"
	}
	if caller != nil && caller.Role == roleAdmin {
		if detail := boundedAgentDiagnostic(info.Error); detail != "" {
			vars["detail"] = detail
		}
	}
	log.Printf("[409][certificate] %s: %s %s: %s", domain, errCodeCertificateIssueFailed, transport.CertificateFailureServerClock, boundedAgentDiagnostic(info.Error))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(apiErrorBody{Error: sentence, Code: errCodeCertificateIssueFailed, Reason: transport.CertificateFailureServerClock, Vars: vars})
}

// writeCertificateIssueFailure answers a certbot run the Agent classified. It
// reports whether it wrote the answer; anything the Agent did not classify
// (an older Agent, a failure before certbot ran) is left to the caller.
func writeCertificateIssueFailure(w http.ResponseWriter, caller *Caller, domain string, hadCertificate bool, reply *transport.IssueLetsEncryptResponse) bool {
	if reply == nil || reply.Success {
		return false
	}
	sentence, known := certificateIssueFailedMessages[reply.Failure]
	if !known {
		return false
	}
	if hadCertificate {
		sentence += certificateIssueKept
	} else {
		sentence += certificateIssueNone
	}
	sentence += certificateIssueResume
	vars := map[string]string{"domain": domain, "kept": "none"}
	if hadCertificate {
		vars["kept"] = "previous"
	}
	status := http.StatusBadGateway
	if reply.Failure == transport.CertificateFailureIssuedCheck {
		// Issued, and its files did not pass the check: the same answer as
		// the Panel's own check of an issued certificate.
		status = http.StatusConflict
		vars["check"] = issuedCheckUnusable
	}
	if caller != nil && caller.Role == roleAdmin {
		if detail := boundedAgentDiagnostic(reply.FailureDetail); detail != "" {
			vars["detail"] = detail
		}
		if reply.Failure != transport.CertificateFailureIssuedCheck {
			vars["record"] = certbotRecordPath
		}
	}
	log.Printf("[%d][certificate] %s: %s %s: %s", status, domain, errCodeCertificateIssueFailed, reply.Failure, boundedAgentDiagnostic(reply.Error))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiErrorBody{Error: sentence, Code: errCodeCertificateIssueFailed, Reason: reply.Failure, Vars: vars})
	return true
}
