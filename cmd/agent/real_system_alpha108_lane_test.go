//go:build linux && realsystem

package main

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Real-system evidence for alpha.108's vendor lane (D-038 amendment "two
// lanes"), in the disposable Debian 13 VM (build tag realsystem,
// CELIKPANEL_REAL_SYSTEM_A108=1). Through the Agent's own code: without a
// vendor repository the server uses Debian's packages only; the Sury lane is
// planned (source and key, confinement, automatic-updates origin, each with
// its digest) and written as ONE confirmed operation, after which the source,
// the confinement and the origin are all in place and unattended-upgrade's
// dry run lists the origin; a version only Sury ships is installed through
// the install preview with its vendor-shared package confirmed; the key's
// expiry is read from its newest self-signature; disabling the repository
// removes nothing and marks what came from it as no longer updated.
// Never run it on an installed server.
//
// alpha.108 satıcı yolu gerçek sistem kanıtı; yalnız atılabilir Debian 13
// sanal makinesinde. Kurulu sunucuda asla koşturulmaz.

// a108Lane plans and writes a vendor repository's lane, as the lane dialog
// does: the plan read, then the confirmed plan in its own vendor_lane job.
func a108Lane(t *testing.T, a *Agent, id string) (transport.VendorLanePlan, transport.VendorLaneResponse) {
	t.Helper()
	var plan transport.VendorLanePlan
	if err := a.VendorLanePlanV1(&EnableRepoRequest{RepoID: id}, &plan); err != nil || plan.Error != "" {
		t.Fatalf("lane plan %s: %v %+v", id, err, plan)
	}
	logJSON(t, "lane plan "+id, plan)
	if !plan.Ready || plan.SHA256 == "" {
		t.Fatalf("the %s lane is not ready: %+v", id, plan)
	}
	var resp transport.VendorLaneResponse
	a108Job(t, "vendor_lane", id, "", func(binding ServiceMutationBinding) bool {
		if err := a.EnableVendorLaneV1(&EnableRepoRequest{ServiceMutationBinding: binding, RepoID: id, ExpectedSHA256: plan.SHA256}, &resp); err != nil {
			t.Fatal(err)
		}
		return resp.ErrorCode == ""
	})
	logJSON(t, "lane written "+id, resp)
	if resp.ErrorCode != "" {
		t.Fatalf("lane %s: %+v", id, resp)
	}
	return plan, resp
}

// a108AutomaticUpdates makes sure the operating system's automatic updates
// are installed and on, as the owner would before choosing a vendor lane
// (the lane refuses otherwise; that refusal is checked first).
func a108AutomaticUpdates(t *testing.T, a *Agent) {
	t.Helper()
	a107Out(t, "apt-get", "install", "-y", "-q", "--no-install-recommends", "unattended-upgrades")
	var plan transport.VendorLanePlan
	if err := a.VendorLanePlanV1(&EnableRepoRequest{RepoID: "sury"}, &plan); err != nil {
		t.Fatal(err)
	}
	if step := laneStepOf(plan, laneStepUpdates); step.State == laneStateBlocked && step.Reason == "automatic_off" {
		logJSON(t, "lane refused while automatic updates are off", plan)
		if plan.Ready {
			t.Fatal("a fresh lane was ready with automatic updates off")
		}
		if err := os.WriteFile("/etc/apt/apt.conf.d/20auto-upgrades", []byte("APT::Periodic::Update-Package-Lists \"1\";\nAPT::Periodic::Unattended-Upgrade \"1\";\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRealSystemA108VendorLane(t *testing.T) {
	requireRealSystemA108(t)
	a := &Agent{}
	// The base image's own Sury source is the owner's earlier choice; the
	// default lane starts without it.
	if err := os.Remove("/etc/apt/sources.list.d/php.list"); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	a107Out(t, "apt-get", "update", "-q")
	a108AutomaticUpdates(t, a)

	// 1. Without the vendor repository, PHP 8.5 has no candidate: the default lane.
	if _, candidate := a107Policy(t, "php8.5-fpm"); candidate != "" && candidate != "(none)" {
		t.Fatalf("php8.5-fpm has a candidate without the vendor repository: %s", candidate)
	}

	// 2. The Sury lane, planned and written as one confirmed operation.
	plan, written := a108Lane(t, a, "sury")
	for _, step := range plan.Steps {
		if step.State != laneStateTodo || step.SHA256 == "" || step.Text == "" {
			t.Fatalf("a fresh lane step is not shown in full: %+v", step)
		}
	}
	if strings.Join(written.Completed, ",") != "repository,confinement,updates" || !written.Enabled || !written.Effective {
		t.Fatalf("written = %+v", written)
	}
	for _, path := range []string{repoSourcePath("sury"), repoPinPath("sury"), vendorOriginsDropInPath} {
		a107Out(t, "cat", path)
	}
	if pin, _ := os.ReadFile(repoPinPath("sury")); sha256Hex(pin) != laneStepOf(plan, laneStepConfinement).SHA256 {
		t.Fatal("the confinement written is not the one confirmed")
	}
	if dropIn, _ := os.ReadFile(vendorOriginsDropInPath); sha256Hex(dropIn) != laneStepOf(plan, laneStepUpdates).SHA256 {
		t.Fatal("the drop-in written is not the one confirmed")
	}
	allowed := ""
	for _, line := range strings.Split(a107Out(t, "unattended-upgrade", "--dry-run", "--debug"), "\n") {
		if strings.Contains(line, "Allowed origins are:") {
			allowed = line
		}
	}
	if !strings.Contains(allowed, "origin=deb.sury.org") {
		t.Fatalf("unattended-upgrade does not list Sury: %q", allowed)
	}
	after := vendorLanePlan(t.Context(), repoFromCatalogue("sury"))
	if after.Ready || after.SHA256 != "" {
		t.Fatalf("after the lane something is still missing: %+v", after)
	}
	// A second confirmation of the same plan writes nothing (the plan changed).
	var again transport.VendorLaneResponse
	a108Job(t, "vendor_lane", "sury", "", func(binding ServiceMutationBinding) bool {
		_ = a.EnableVendorLaneV1(&EnableRepoRequest{ServiceMutationBinding: binding, RepoID: "sury", ExpectedSHA256: plan.SHA256}, &again)
		return again.ErrorCode == ""
	})
	if again.ErrorCode != "LANE_PLAN_CHANGED" {
		t.Fatalf("a repeated confirmation = %+v", again)
	}

	// 3. The key's expiry is its newest self-signature's (owner finding, Boston).
	var status RepoStatusResponse
	if err := a.RepoStatus(&EnableRepoRequest{RepoID: "sury"}, &status); err != nil {
		t.Fatal(err)
	}
	logJSON(t, "sury status", status)
	if status.Key == nil || status.Key.State != "valid" {
		t.Fatalf("key = %+v", status.Key)
	}
	if expires, err := time.Parse(time.RFC3339, status.Key.ExpiresAt); err == nil && expires.Year() < 2026 {
		t.Fatalf("the key reads as expiring in %d", expires.Year())
	}

	// 4. A version only Sury ships, from the version list: the preview names
	// its vendor-shared package, the install waits for it, then installs.
	var preview transport.ServiceInstallPreviewResponse
	if err := a.ServiceInstallPreviewV1(&transport.ServiceInstallPreviewRequest{Steps: []transport.ServiceInstallPreviewStep{{ID: "php-fpm", Package: "php8.5-fpm"}}}, &preview); err != nil {
		t.Fatal(err)
	}
	logJSON(t, "install preview php8.5-fpm", preview)
	if preview.State != "checked" || !slices.ContainsFunc(preview.Notable, func(item transport.NotableInstall) bool { return item.Reason == notableReasonVendorShared }) {
		t.Fatalf("the preview does not name the vendor-shared package: %+v", preview)
	}
	if resp := a105Install(t, a, "php-fpm", "php8.5-fpm", "", nil); resp.ErrorCode != "INSTALL_NEEDS_CONFIRMATION" {
		t.Fatalf("unconfirmed php8.5-fpm = %+v", resp)
	}
	if resp := a107Install(t, a, "php-fpm", "php8.5-fpm"); resp.Error != "" {
		t.Fatalf("confirmed php8.5-fpm: %s", resp.Error)
	}
	if a107Version("php8.5-fpm") == "" {
		t.Fatal("php8.5-fpm was not installed")
	}

	// 5. Disabling removes nothing; what came from it is no longer updated.
	var disabled RepoStatusResponse
	a108Job(t, "repo_disable", "sury", "", func(binding ServiceMutationBinding) bool {
		if err := a.DisableRepo(&EnableRepoRequest{ServiceMutationBinding: binding, RepoID: "sury"}, &disabled); err != nil {
			t.Fatal(err)
		}
		return disabled.Error == ""
	})
	logJSON(t, "disabled", disabled)
	if disabled.Error != "" || a107Version("php8.5-fpm") == "" || !slices.Contains(disabled.NoLongerUpdated, "php8.5-fpm") {
		t.Fatalf("disable = %+v (php8.5-fpm %q)", disabled, a107Version("php8.5-fpm"))
	}
	var later RepoStatusResponse
	if err := a.RepoStatus(&EnableRepoRequest{RepoID: "sury"}, &later); err != nil || later.Enabled || !slices.Contains(later.NoLongerUpdated, "php8.5-fpm") {
		t.Fatalf("status after disable = %+v %v", later, err)
	}
	a107Out(t, "systemctl", "is-active", "php8.5-fpm")
}
