package main

import (
	"errors"
	"os/user"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The account proof of DeleteSite and CreateSite (security release,
// 20 November 2026): what still happens to a site's own account.

func TestDeleteSiteStillRemovesItsOwnAccountCrontabAndLogins(t *testing.T) {
	withLifecycleTestBuild(t)
	req := validLifecycleDeleteRequest(t)
	fake := &fakeSiteLifecycle{home: req.SiteHome, userExists: true, failures: map[string]error{}}
	var keyed []string
	agent := lifecycleTestAgent(t, fake)
	agent.siteOps = accountProofOps(fake, &keyed)
	resp := &DeleteSiteResponse{}
	if err := agent.DeleteSite(&req, resp); err != nil {
		t.Fatal(err)
	}
	want := "crontab:example_test,ftp-logins:example_test,file-access:example_test"
	if strings.Join(keyed, ",") != want || !resp.Success {
		t.Fatalf("keyed=%v resp=%#v", keyed, resp)
	}
	if !strings.Contains(strings.Join(fake.calls, ","), "delete-user") {
		t.Fatalf("the site's own account was kept: %v", fake.calls)
	}
}

func TestDeleteSiteOfAGoneAccountStillClearsItsLeftovers(t *testing.T) {
	withLifecycleTestBuild(t)
	req := validLifecycleDeleteRequest(t)
	fake := &fakeSiteLifecycle{home: req.SiteHome, failures: map[string]error{}}
	var keyed []string
	agent := lifecycleTestAgent(t, fake)
	agent.siteOps = accountProofOps(fake, &keyed)
	resp := &DeleteSiteResponse{}
	if err := agent.DeleteSite(&req, resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Success || !strings.Contains(strings.Join(keyed, ","), "crontab:example_test") {
		t.Fatalf("keyed=%v resp=%#v", keyed, resp)
	}
}

func TestDeleteSiteStopsAccountStepsWhenTheAccountCannotBeRead(t *testing.T) {
	withLifecycleTestBuild(t)
	req := validLifecycleDeleteRequest(t)
	fake := &fakeSiteLifecycle{home: req.SiteHome, failures: map[string]error{"lookup-user": errors.New("nss unavailable")}}
	var keyed []string
	agent := lifecycleTestAgent(t, fake)
	agent.siteOps = accountProofOps(fake, &keyed)
	resp := &DeleteSiteResponse{}
	if err := agent.DeleteSite(&req, resp); err != nil {
		t.Fatal(err)
	}
	if resp.Success || len(keyed) != 0 {
		t.Fatalf("keyed=%v resp=%#v", keyed, resp)
	}
}

func TestCreateSiteSaysTheIdentityExistsSoNothingIsCompensated(t *testing.T) {
	withLifecycleTestBuild(t)
	req := validLifecycleCreateRequest(t)
	otherHome, _ := hostingpath.SiteHome(3, 30)
	fake := &fakeSiteLifecycle{userExists: true, userHome: otherHome, failures: map[string]error{}}
	reply := &transport.CreateSiteResponse{}
	if err := lifecycleTestAgent(t, fake).CreateSite(req, reply); err != nil {
		t.Fatal(err)
	}
	if reply.Success || reply.ErrorCode != transport.SiteIdentityExists {
		t.Fatalf("reply = %#v", reply)
	}
}

func TestClassifySiteAccount(t *testing.T) {
	home, _ := hostingpath.SiteHome(7, 11)
	other, _ := hostingpath.SiteHome(3, 30)
	for _, tc := range []struct {
		name string
		u    *user.User
		err  error
		want siteAccountState
	}{
		{"absent", nil, user.UnknownUserError("x"), siteAccountAbsent},
		{"this site", &user.User{Uid: "2001", HomeDir: home + "/"}, nil, siteAccountThisSite},
		{"another site", &user.User{Uid: "2002", HomeDir: other}, nil, siteAccountOtherSite},
		{"system", &user.User{Uid: "33", HomeDir: home}, nil, siteAccountForeign},
		{"outside", &user.User{Uid: "2003", HomeDir: "/srv/x"}, nil, siteAccountForeign},
	} {
		ops := siteLifecycleOps{lookupUser: func(string) (*user.User, error) { return tc.u, tc.err }}
		got, err := classifySiteAccount(ops, "x", home)
		if err != nil || got != tc.want {
			t.Fatalf("%s: got %v %v", tc.name, got, err)
		}
	}
}
