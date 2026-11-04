package main

import "testing"

// alpha.123 (D-045): one remove-together lease covers its target and the
// members the dependency map says require it, and nothing else.
func TestRemoveTogetherLeaseAuthorizesOnlyItsMembers(t *testing.T) {
	job := &ServiceMutationJob{Kind: "service_remove_together", Target: "postfix", PackageName: "roundcube,rspamd"}
	allowed := []serviceMutationStepClaim{
		newServiceMutationStepClaim(serviceMutationStepUninstallService, "postfix", "", "uninstall"),
		newServiceMutationStepClaim(serviceMutationStepUninstallService, "rspamd", "", "uninstall"),
		newServiceMutationStepClaim(serviceMutationStepRemoveRoundcube, "roundcube", "", "remove"),
		newServiceMutationStepClaim(serviceMutationStepConfigureWebmail, "roundcube", "", "configure"),
		newServiceMutationStepClaim(serviceMutationStepWireMailFilters, "mail-filters", "", "wire"),
	}
	for _, claim := range allowed {
		if err := authorizeServiceMutationStep(job, claim); err != nil {
			t.Errorf("%+v refused: %v", claim, err)
		}
	}
	refused := []serviceMutationStepClaim{
		newServiceMutationStepClaim(serviceMutationStepUninstallService, "dovecot", "", "uninstall"),
		newServiceMutationStepClaim(serviceMutationStepUninstallService, "rspamd", "pkg", "uninstall"),
		newServiceMutationStepClaim(serviceMutationStepInstallService, "rspamd", "", "install"),
		newServiceMutationStepClaim(serviceMutationStepConfigureDBTools, "dbtools", "", "configure"),
	}
	for _, claim := range refused {
		if authorizeServiceMutationStep(job, claim) == nil {
			t.Errorf("%+v allowed", claim)
		}
	}
	for _, bad := range []*ServiceMutationJob{
		{Kind: "service_remove_together", Target: "postfix", PackageName: "rspamd,roundcube"}, // not canonical
		{Kind: "service_remove_together", Target: "postfix", PackageName: "phpmyadmin"},       // does not require postfix
		{Kind: "service_remove_together", Target: "nftables", PackageName: "fail2ban"},        // never together
		{Kind: "service_remove_together", Target: "pdns", PackageName: ""},                    // DNS engine workflow
	} {
		if authorizeServiceMutationStep(bad, newServiceMutationStepClaim(serviceMutationStepUninstallService, bad.Target, "", "uninstall")) == nil {
			t.Errorf("lease %+v authorized its target", bad)
		}
	}
	db := &ServiceMutationJob{Kind: "service_remove_together", Target: "postgresql", PackageName: "phppgadmin"}
	if err := authorizeServiceMutationStep(db, newServiceMutationStepClaim(serviceMutationStepConfigureDBTools, "dbtools", "", "configure")); err != nil {
		t.Errorf("db tools refused: %v", err)
	}
}
