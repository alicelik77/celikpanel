package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/services"
)

// Security release (20 November 2026): two domain names can derive the same
// system account (services.SiteUsername). A site whose creation was refused
// because "its" account already exists belongs to another customer's site;
// deleting the refused site must not touch that account, its scheduled
// tasks or its FTP logins.

// accountProofOps records every account-keyed step of a deletion.
func accountProofOps(fake *fakeSiteLifecycle, keyed *[]string) *siteLifecycleOps {
	ops := fake.operations()
	ops.removeCrontab = func(name string) error { *keyed = append(*keyed, "crontab:"+name); return nil }
	ops.forgetLogins = func(name string) error { *keyed = append(*keyed, "ftp-logins:"+name); return nil }
	ops.forgetAccess = func(name string) { *keyed = append(*keyed, "file-access:"+name) }
	return ops
}

func TestDeleteSiteLeavesAnotherSitesAccountCrontabAndLoginsAlone(t *testing.T) {
	withLifecycleTestBuild(t)
	// Customer A's site ab.cd.test (subscription 3, domain 30) owns the
	// account ab_cd_test. Customer B's refused site is ab-cd.test
	// (subscription 7, domain 11): same account name.
	otherHome, err := hostingpath.SiteHome(3, 30)
	if err != nil {
		t.Fatal(err)
	}
	home, err := hostingpath.SiteHome(7, 11)
	if err != nil {
		t.Fatal(err)
	}
	if services.SiteUsername("ab-cd.test") != services.SiteUsername("ab.cd.test") {
		t.Fatal("the two names no longer share an account; the test lost its premise")
	}
	req := DeleteSiteRequest{
		ExpectedBuildCommit: "lifecycle-test", SiteID: 13, SubscriptionID: 7, DomainID: 11,
		Domain: "ab-cd.test", Username: services.SiteUsername("ab-cd.test"), PHPVersion: "8.3", SiteHome: home,
	}
	fake := &fakeSiteLifecycle{home: home, userExists: true, userHome: otherHome, failures: map[string]error{}}
	var keyed []string
	agent := lifecycleTestAgent(t, fake)
	agent.siteOps = accountProofOps(fake, &keyed)
	resp := &DeleteSiteResponse{}
	if err := agent.DeleteSite(&req, resp); err != nil {
		t.Fatal(err)
	}
	if len(keyed) != 0 {
		t.Fatalf("the other site's account was acted on by name: %v", keyed)
	}
	joined := strings.Join(fake.calls, ",")
	for _, forbidden := range []string{"kill-user", "delete-user"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("the other site's account was touched (%s): %v", forbidden, fake.calls)
		}
	}
	if !resp.Success {
		t.Fatalf("the refused site's own parts are removed and nothing of it remains; got %#v", resp)
	}
	if !strings.Contains(joined, "remove-all") || !strings.Contains(joined, "remove-vhost") {
		t.Fatalf("the site's own parts were not removed: %v", fake.calls)
	}
}
