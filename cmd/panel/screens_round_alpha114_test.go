package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// alpha.114, the lab screenshot review: two answers the screens read.

// The administrator's own workspace (the subscription the Panel makes for an
// administrator who hosts sites without a plan, stored with 999) has no limit:
// the screens used to show "2/999" and the 1000th domain was refused. An
// administrator's subscription that has a plan, and every other account's, is
// still metered.
func TestAdministratorWorkspaceIsNotMetered(t *testing.T) {
	f := newAccountsFixture(t)
	ctx := context.Background()
	f.exec(t, `INSERT INTO subscriptions (id, owner_id, name, max_domains, max_databases, status) VALUES (9810, 9701, 'Admin Subscription', 1, 1, 'active')`)
	f.exec(t, `INSERT INTO domains (id, subscription_id, name, status) VALUES (9910, 9810, 'admin-own.matrix.test', 'active')`)
	for _, kind := range []quotaKind{quotaDomains, quotaDBs, quotaMail, quotaDisk} {
		if err := f.panel.checkSubscriptionQuota(ctx, 9810, kind); err != nil {
			t.Fatalf("%s in the administrator's own workspace: %v", kind, err)
		}
	}
	usage, err := f.panel.subscriptionUsageFor(ctx, 9810)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Domains != 1 || usage.DomainsLimit != 0 || usage.DatabasesLimit != 0 || usage.MailLimit != 0 || usage.DiskLimitBytes != 0 {
		t.Fatalf("usage = %+v, want one domain and no limits", usage)
	}

	// The same name under a plan, or owned by a customer, keeps its limit.
	f.exec(t, `INSERT INTO service_plans (id, owner_id, name, max_domains) VALUES (9820, 9701, 'tiny', 1)`)
	f.exec(t, `UPDATE subscriptions SET plan_id = 9820 WHERE id = 9810`)
	if err := f.panel.checkSubscriptionQuota(ctx, 9810, quotaDomains); err == nil {
		t.Fatal("an administrator's subscription with a plan is metered like any other")
	}
	f.exec(t, `UPDATE subscriptions SET plan_id = NULL, owner_id = 9703 WHERE id = 9810`)
	if err := f.panel.checkSubscriptionQuota(ctx, 9810, quotaDomains); err == nil {
		t.Fatal("a customer's subscription with that name is metered")
	}
}

// The Users list tells an additional user from a customer: it carries
// account_type, and the screen writes "Additional user of <owner>".
func TestUsersListCarriesTheAccountType(t *testing.T) {
	f := newAccountsFixture(t)
	recorder := f.as(t, 9701, http.MethodGet, "/api/v1/users", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Users []struct {
			ID          int    `json:"id"`
			AccountType string `json:"account_type"`
			ParentName  string `json:"parent_name"`
		} `json:"users"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	seen := map[int]string{}
	parent := ""
	for _, user := range body.Users {
		seen[user.ID] = user.AccountType
		if user.ID == authzMatrixAdditionalUserID {
			parent = user.ParentName
		}
	}
	if seen[authzMatrixAdditionalUserID] != "additional_user" {
		t.Fatalf("the additional user's account_type = %q", seen[authzMatrixAdditionalUserID])
	}
	if parent != "matrix-customer" {
		t.Fatalf("the additional user's owner = %q", parent)
	}
	if seen[9703] == "additional_user" {
		t.Fatal("a customer is not an additional user")
	}
}
