package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.134 lab run 6, C.3c: the owner added `renew_before_expiry = 20 days`
// to a replaced lineage's renewal file and the lineage was removed. A removal
// now needs a file that holds nothing but what certbot writes for
// CelikPanel's request; the page's warning rule is unchanged.
func TestRetireKeepsAReplacedLineageWhoseRenewalFileHoldsAnythingElse(t *testing.T) {
	const active = "celikpanel-site-42-ffffffffffffffffffffffff"
	labCase := strings.Replace(testRenewalConf, "version = 4.0.0\n", "version = 4.0.0\nrenew_before_expiry = 20 days\n", 1)
	if labCase == testRenewalConf {
		t.Fatal("the lab's edit was not applied to the fixture")
	}
	withParam := func(line string) string {
		return strings.Replace(testRenewalConf, "[renewalparams]\n", "[renewalparams]\n"+line+"\n", 1)
	}
	for _, tc := range []struct {
		name, conf, activeConf, want string
	}{
		{"a clean file certbot wrote", testRenewalConf, testRenewalConf, transport.SiteLineageRetired},
		{"CelikPanel's own autorenew = False (alpha.113-133)", withParam("autorenew = False"), testRenewalConf, transport.SiteLineageRetired},
		{"the lab's case: renew_before_expiry added", labCase, testRenewalConf, transport.SiteLineageOwnerChanged},
		{"a hook", withParam("renew_hook = /usr/local/bin/notify"), testRenewalConf, transport.SiteLineageOwnerChanged},
		{"a hook also in the active file (a global setting) is still a hook", withParam("deploy_hook = /usr/local/bin/notify"), withParam("deploy_hook = /usr/local/bin/notify"), transport.SiteLineageOwnerChanged},
		{"an unknown key", withParam("foo = bar"), testRenewalConf, transport.SiteLineageOwnerChanged},
		{"a global certbot setting, the same in the active file", withParam("no_verify_ssl = True"), withParam("no_verify_ssl = True"), transport.SiteLineageRetired},
		{"another key type", withParam("key_type = ed25519"), testRenewalConf, transport.SiteLineageOwnerChanged},
		{"another section", testRenewalConf + "[owner]\nnote = mine\n", testRenewalConf, transport.SiteLineageOwnerChanged},
		{"live files moved", strings.Replace(testRenewalConf, "/live/celikpanel-site-42-00112233445566778899aabb/cert.pem", "/live/other/cert.pem", 1), testRenewalConf, transport.SiteLineageOwnerChanged},
		// When in doubt, keep: a comment certbot does not write is an edit.
		{"an owner's comment", "# kept by me\n" + testRenewalConf, testRenewalConf, transport.SiteLineageOwnerChanged},
	} {
		t.Run(tc.name, func(t *testing.T) {
			retireTestStore(t)
			const replaced = "celikpanel-site-42-aaaaaaaaaaaaaaaaaaaaaaaa"
			retireTestLineage(t, replaced, tc.conf)
			retireTestLineage(t, active, tc.activeConf)
			var resp transport.RetireSiteCertificateLineagesResponse
			_ = (&Agent{}).RetireSiteCertificateLineagesV1(&transport.RetireSiteCertificateLineagesRequest{
				DomainID: 42, SubscriptionID: 7, ActiveLineage: active,
				Lineages: []transport.RetireSiteLineage{{LineageName: replaced}},
			}, &resp)
			if len(resp.Results) != 1 || resp.Results[0].Outcome != tc.want {
				t.Fatalf("resp = %+v", resp)
			}
			_, err := os.Stat(nativeSiteRenewalConfPath(replaced))
			if removed := os.IsNotExist(err); removed != (tc.want == transport.SiteLineageRetired) {
				t.Fatalf("removed = %v, want %v", removed, tc.want == transport.SiteLineageRetired)
			}
		})
	}
	// The page's warning rule is unchanged: an edit that does not change
	// how it renews is not reported as "changed outside CelikPanel".
	conf, err := parseNativeSiteRenewalConf([]byte(labCase))
	if err != nil {
		t.Fatal(err)
	}
	if nativeSiteConfOwnerChanged(conf, "/var/lib/celikpanel-agent/acme-http-01/subscriptions/7/domains/42", letsEncryptDefaultDirectory) {
		t.Fatal("the page now warns about a harmless edit")
	}
}

// alpha.134 lab run 6: certbot printed only "Account <url> not found." for
// the authority's accountDoesNotExist (the urn is in its log), and "Please
// choose an account" when it held two accounts for one authority; both were
// "the reason could not be established". Both are the account reason, from
// the printed output, and from this run's log when the output names nothing.
func TestCertbotAccountProblemsAreReadAsTheAccountReason(t *testing.T) {
	stale := "Saving debug log to /var/log/letsencrypt/letsencrypt.log\n" +
		"An unexpected error occurred:\n" +
		"Account https://192.0.2.60:14000/my-account/54808237512c1c19 not found.\n" +
		"Ask for help or search for solutions at https://community.letsencrypt.org. See the logfile /var/log/letsencrypt/letsencrypt.log or re-run Certbot with -v for more details.\n"
	choose := "Saving debug log to /var/log/letsencrypt/letsencrypt.log\n" +
		"Missing command line flag or config entry for this setting:\n" +
		"Please choose an account\n" +
		"Choices: ['host1.dr.test@2026-11-12T12:20:00Z (39fd)', 'host1.dr.test@2026-11-14T07:40:00Z (88c5)']\n\n" +
		"(You can set this with the --account flag)\n"
	for name, output := range map[string]string{"stale account": stale, "two accounts": choose} {
		if kind := certbotFailureKind(output); kind != transport.CertificateFailureAccountUnknown {
			t.Fatalf("%s: kind = %q", name, kind)
		}
	}

	// Only this run's log is read, and only when the output names nothing.
	nativeSiteTestStore(t)
	if err := os.MkdirAll(nativeSiteLogDir, 0o700); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(nativeSiteLogDir, "letsencrypt.log")
	if err := os.WriteFile(record, []byte("2026-11-14 12:36:14,470:DEBUG:certbot._internal.log:Exiting abnormally:\n"+
		"acme.messages.Error: urn:ietf:params:acme:error:accountDoesNotExist :: The request specified an account that does not exist :: No account exists with the given key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	vague := "An unexpected error occurred:\nThe authority said no.\n"
	started := time.Now().Add(-time.Minute)
	if kind := certbotFailureKindWithLog(vague, started); kind != transport.CertificateFailureAccountUnknown {
		t.Fatalf("with this run's log: kind = %q", kind)
	}
	if kind := certbotFailureKindWithLog(vague, time.Now().Add(time.Hour)); kind != transport.CertificateFailureTool {
		t.Fatalf("an earlier run's log was read: kind = %q", kind)
	}
	if kind := certbotFailureKind(vague); kind != transport.CertificateFailureTool {
		t.Fatalf("output alone: kind = %q", kind)
	}
}
