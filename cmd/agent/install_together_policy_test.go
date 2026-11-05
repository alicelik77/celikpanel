package main

import "testing"

// alpha.124 (D-045 follow-up): one install-together lease covers its target
// and the plain requirements named in its qualifier, and nothing else.
func TestInstallTogetherLeaseAuthorizesOnlyItsMembers(t *testing.T) {
	job := &ServiceMutationJob{Kind: "service_install_together", Target: "phpmyadmin", PackageName: "mariadb,nginx,php-fpm"}
	allowed := []serviceMutationStepClaim{
		newServiceMutationStepClaim(serviceMutationStepInstallService, "mariadb", "", "install"),
		newServiceMutationStepClaim(serviceMutationStepInstallService, "nginx", "", "install"),
		newServiceMutationStepClaim(serviceMutationStepInstallService, "php-fpm", "", "install"),
		newServiceMutationStepClaim(serviceMutationStepInstallService, "phpmyadmin", "", "install"),
		newServiceMutationStepClaim(serviceMutationStepEnsureNginxReady, "nginx", "", "ready"),
		newServiceMutationStepClaim(serviceMutationStepConfigureDBTools, "dbtools", "", "configure"),
		newServiceMutationStepClaim(serviceMutationStepProvePHPTool, "phpmyadmin", "", "prove"),
	}
	for _, claim := range allowed {
		if err := authorizeServiceMutationStep(job, claim); err != nil {
			t.Errorf("%+v refused: %v", claim, err)
		}
	}
	refused := []serviceMutationStepClaim{
		newServiceMutationStepClaim(serviceMutationStepInstallService, "postgresql", "", "install"),
		newServiceMutationStepClaim(serviceMutationStepInstallService, "php-fpm", "php8.5-fpm", "install"),
		newServiceMutationStepClaim(serviceMutationStepUninstallService, "nginx", "", "uninstall"),
		newServiceMutationStepClaim(serviceMutationStepInstallRoundcube, "roundcube", "", "install"),
		newServiceMutationStepClaim(serviceMutationStepBindPHPTool, "phpmyadmin", "8.4", "bind"),
		newServiceMutationStepClaim(serviceMutationStepProvePHPTool, "webmail", "", "prove"),
	}
	for _, claim := range refused {
		if authorizeServiceMutationStep(job, claim) == nil {
			t.Errorf("%+v allowed", claim)
		}
	}
	for _, bad := range []*ServiceMutationJob{
		{Kind: "service_install_together", Target: "phpmyadmin", PackageName: "php-fpm,nginx"}, // not canonical
		{Kind: "service_install_together", Target: "phpmyadmin", PackageName: "postgresql"},    // not its requirement
		{Kind: "service_install_together", Target: "roundcube", PackageName: "postfix"},        // mail setup only
		{Kind: "service_install_together", Target: "pdns", PackageName: "nginx"},               // DNS engine workflow
	} {
		if authorizeServiceMutationStep(bad, newServiceMutationStepClaim(serviceMutationStepInstallService, bad.Target, "", "install")) == nil {
			t.Errorf("lease %+v authorized its target", bad)
		}
	}
	webmail := &ServiceMutationJob{Kind: "service_install_together", Target: "roundcube", PackageName: "nginx,php-fpm"}
	for _, claim := range []serviceMutationStepClaim{
		newServiceMutationStepClaim(serviceMutationStepInstallRoundcube, "roundcube", "", "install"),
		newServiceMutationStepClaim(serviceMutationStepConfigureWebmail, "roundcube", "", "configure"),
		newServiceMutationStepClaim(serviceMutationStepBindPHPTool, "webmail", "8.4", "bind"),
		newServiceMutationStepClaim(serviceMutationStepProvePHPTool, "webmail", "", "prove"),
	} {
		if err := authorizeServiceMutationStep(webmail, claim); err != nil {
			t.Errorf("webmail %+v refused: %v", claim, err)
		}
	}
	fail2ban := &ServiceMutationJob{Kind: "service_install_together", Target: "fail2ban", PackageName: "nftables"}
	if err := authorizeServiceMutationStep(fail2ban, newServiceMutationStepClaim(serviceMutationStepInstallService, "nftables", "", "install")); err != nil {
		t.Errorf("nftables refused: %v", err)
	}
}

// alpha.124 (P1c): webmail's own install binds it to the chosen PHP version;
// another tool's install does not.
func TestRoundcubeInstallLeaseBindsWebmailOnly(t *testing.T) {
	job := &ServiceMutationJob{Kind: "service_install", Target: "roundcube"}
	if err := authorizeServiceMutationStep(job, newServiceMutationStepClaim(serviceMutationStepBindPHPTool, "webmail", "8.3", "bind")); err != nil {
		t.Errorf("webmail bind refused: %v", err)
	}
	if authorizeServiceMutationStep(job, newServiceMutationStepClaim(serviceMutationStepBindPHPTool, "phpmyadmin", "8.3", "bind")) == nil {
		t.Error("phpMyAdmin bind allowed under webmail's install")
	}
	other := &ServiceMutationJob{Kind: "service_install", Target: "phpmyadmin"}
	if authorizeServiceMutationStep(other, newServiceMutationStepClaim(serviceMutationStepBindPHPTool, "webmail", "8.3", "bind")) == nil {
		t.Error("webmail bind allowed under phpMyAdmin's install")
	}
}
