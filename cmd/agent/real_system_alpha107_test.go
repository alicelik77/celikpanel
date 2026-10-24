//go:build linux && realsystem

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Real-system evidence for alpha.107 (D-038), run only inside a disposable
// Debian 13 virtual machine booted from the base image (nginx, the vendor PHP
// repository's own source, no PHP; build tag realsystem, plus
// CELIKPANEL_REAL_SYSTEM_A107=1). Through the Agent's own code: the Sury and
// PGDG repositories are enabled and confined; plain installs take the
// distribution's exact default; a distribution package a vendor repository
// would replace is refused by name; the vendor origin joins the operating
// system's automatic updates and unattended-upgrade's dry run lists it;
// PostgreSQL removed keeps its cluster and a reinstall uses it; webmail is
// updated with a forced rollback first, then for real; restarts, reboot and
// the release facts are read. Never run it on an installed server.
//
// alpha.107 gerçek sistem kanıtı; yalnız atılabilir bir Debian 13 sanal
// makinesinde koşar. Kurulu bir sunucuda asla koşturulmaz.

func requireRealSystemA107(t *testing.T) {
	t.Helper()
	if os.Getenv("CELIKPANEL_REAL_SYSTEM_A107") != "1" {
		t.Skip("real-system evidence runs only in the disposable alpha.107 VM")
	}
}

// a107Out runs a command and returns its output; the evidence keeps it.
func a107Out(t *testing.T, name string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "DEBIAN_FRONTEND=noninteractive")
	out, err := cmd.CombinedOutput()
	t.Logf("$ %s %s (err=%v)\n%s", name, strings.Join(args, " "), err, lastLines(string(out), 40))
	return string(out)
}

func a107Version(pkg string) string {
	out, err := exec.Command("dpkg-query", "-W", "-f", "${Status}\t${Version}", pkg).Output()
	if err != nil {
		return ""
	}
	status, version, _ := strings.Cut(string(out), "\t")
	if !strings.HasSuffix(status, "install ok installed") {
		return ""
	}
	return version
}

func a107Policy(t *testing.T, pkg string) (installed, candidate string) {
	for _, line := range strings.Split(a107Out(t, "apt-cache", "policy", pkg), "\n") {
		line = strings.TrimSpace(line)
		if value, ok := strings.CutPrefix(line, "Installed: "); ok {
			installed = value
		}
		if value, ok := strings.CutPrefix(line, "Candidate: "); ok {
			candidate = value
		}
	}
	return installed, candidate
}

func TestRealSystemA107RepositoriesAreConfinedAndTheDefaultIsExact(t *testing.T) {
	requireRealSystemA107(t)
	a := &Agent{}
	if err := os.Remove("/etc/apt/sources.list.d/php.list"); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, id := range []string{"sury", "pgdg"} {
		var repo RepoStatusResponse
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		err := a.enableRepoWith(ctx, &EnableRepoRequest{RepoID: id}, &repo)
		cancel()
		logJSON(t, "enable "+id, repo)
		if err != nil || repo.Error != "" || !repo.Enabled || repo.Confinement == nil || repo.Confinement.State != repoConfinementConfined {
			t.Fatalf("enable %s: %v %+v", id, err, repo)
		}
		pin, err := os.ReadFile(repoPinPath(id))
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s:\n%s", repoPinPath(id), pin)
	}
	// Confined: the distribution's PHP 8.4 and PostgreSQL stay the candidates.
	if _, candidate := a107Policy(t, "php8.4-fpm"); candidate == "" || hostVendorVersionLookup()("php8.4-fpm", candidate) != "" {
		t.Fatalf("php8.4-fpm's candidate %q comes from a vendor repository", candidate)
	}
	a107Out(t, "apt-cache", "policy", "postgresql", "php-common")

	// A plain install takes the distribution's exact default.
	if resp := a105Install(t, a, "php-fpm", "", "", nil); resp.Error != "" {
		t.Fatalf("php-fpm: %s", resp.Error)
	}
	if v := a107Version("php8.4-fpm"); v == "" || hostVendorVersionLookup()("php8.4-fpm", v) != "" {
		t.Fatalf("php8.4-fpm %q is not the distribution's", v)
	}
	if resp := a105Install(t, a, "postgresql", "", "", nil); resp.Error != "" {
		t.Fatalf("postgresql: %s", resp.Error)
	}
	if a107Version("postgresql-17") == "" || a107Version("postgresql-18") != "" {
		t.Fatalf("postgresql-17 %q, postgresql-18 %q", a107Version("postgresql-17"), a107Version("postgresql-18"))
	}
	t.Logf("postgresql metapackage: %q (exact install: empty)", a107Version("postgresql"))
	// A version only the vendor repository has still installs from it.
	if resp := a105Install(t, a, "php-fpm", "php8.3-fpm", "", nil); resp.Error != "" {
		t.Fatalf("php8.3-fpm from the vendor repository under confinement: %s", resp.Error)
	}
	t.Logf("php8.3-fpm %s, php-common %s", a107Version("php8.3-fpm"), a107Version("php-common"))

	// Without the confinement (as on a server whose owner removed it), a
	// distribution package the vendor would replace is refused by name.
	pin := repoPinPath("sury")
	if err := os.Rename(pin, pin+".a107"); err != nil {
		t.Fatal(err)
	}
	restored := false
	restore := func() {
		if !restored {
			restored = true
			if err := os.Rename(pin+".a107", pin); err != nil {
				t.Errorf("restore the pin: %v", err)
			}
		}
	}
	defer restore()
	installed, candidate := a107Policy(t, "php8.4-common")
	if installed == candidate || hostVendorVersionLookup()("php8.4-common", candidate) == "" {
		t.Logf("the vendor repository offers no newer php8.4-common (%s / %s); the replacement refusal is covered by component tests only", installed, candidate)
	} else {
		before := a105DpkgSet(t)
		classification := newInstallClassification("apt", []string{"php8.4-gd"}, []string{"php-fpm"}, []string{"8.4"}, a.installedServiceSet())
		classification.vendorOf = hostVendorVersionLookup()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		out, err := installPackagesGuardedContext(ctx, "apt", []string{"php8.4-gd"}, "", notableInstallGuard(classification, []string{"php8.4-gd"}, []string{"php8.4-common"}))
		cancel()
		var refusal *packageInstallVendorRefusal
		if !errors.As(err, &refusal) || !strings.Contains(err.Error(), "php8.4-common") || out != "" {
			t.Fatalf("vendor replacement: out=%q err=%v", out, err)
		}
		t.Logf("refused, even confirmed: %v", err)
		if after := a105DpkgSet(t); len(after) != len(before) || a107Version("php8.4-common") != installed {
			t.Fatalf("a refused install changed the installed set")
		}
	}
	restore()
}

func TestRealSystemA107VendorOriginsJoinAutomaticUpdates(t *testing.T) {
	requireRealSystemA107(t)
	a := &Agent{}
	a107Out(t, "apt-get", "install", "-y", "-q", "--no-install-recommends", "unattended-upgrades")
	var status transport.OSUpdatesStatusResponse
	if err := a.OSUpdatesStatusV1(&transport.Empty{}, &status); err != nil {
		t.Fatal(err)
	}
	logJSON(t, "updates before", status)
	if status.DropIn == nil || status.DropIn.ProposedSHA256 == "" {
		t.Fatalf("no drop-in proposed: %+v", status)
	}
	dryRunOrigins := func() string {
		for _, line := range strings.Split(a107Out(t, "unattended-upgrade", "--dry-run", "--debug"), "\n") {
			if strings.Contains(line, "Allowed origins are:") {
				return line
			}
		}
		return ""
	}
	if line := dryRunOrigins(); strings.Contains(line, "deb.sury.org") {
		t.Fatalf("the vendor origin was already allowed: %s", line)
	}
	manager, _ := newMutationTestManager(t)
	installGlobalMutationTestManager(t, manager)
	beginMutationTestJobWithIdentity(t, manager, "os_updates_origins", "unattended-upgrades", "")
	var written transport.WriteVendorUpdateOriginsResponse
	if err := a.WriteVendorUpdateOriginsV1(&transport.WriteVendorUpdateOriginsRequest{
		ServiceMutationBinding: ServiceMutationBinding{MutationRequestID: testMutationRequestID, MutationOwnerID: testMutationOwnerID},
		ExpectedSHA256:         status.DropIn.ProposedSHA256,
	}, &written); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.finish(&ServiceMutationFinishRequest{RequestID: testMutationRequestID, OwnerID: testMutationOwnerID, Success: written.Written}); err != nil {
		t.Fatal(err)
	}
	logJSON(t, "drop-in written", written)
	if !written.Written || !written.Effective {
		t.Fatalf("written = %+v", written)
	}
	a107Out(t, "cat", vendorOriginsDropInPath)
	if line := dryRunOrigins(); !strings.Contains(line, "deb.sury.org") {
		t.Fatalf("unattended-upgrade does not list the vendor origin: %q", line)
	}
	var after transport.OSUpdatesStatusResponse
	if err := a.OSUpdatesStatusV1(&transport.Empty{}, &after); err != nil {
		t.Fatal(err)
	}
	logJSON(t, "updates after", after)
	for _, vendor := range after.Vendors {
		if vendor.RepoID == "sury" && !vendor.Covered {
			t.Fatalf("sury still not covered: %+v", vendor)
		}
	}
}

func TestRealSystemA107PostgreSQLRemovalKeepsTheCluster(t *testing.T) {
	requireRealSystemA107(t)
	if a107Version("postgresql-17") == "" {
		t.Skip("runs after the repository test")
	}
	a := &Agent{}
	a107Out(t, "runuser", "-u", "postgres", "--", "psql", "-c", "CREATE DATABASE a107evidence")
	var preview transport.ServiceRemovalPreviewResponse
	if err := a.ServiceRemovalPreviewV1(&transport.ServiceRemovalPreviewRequest{ID: "postgresql"}, &preview); err != nil {
		t.Fatal(err)
	}
	logJSON(t, "removal preview", preview)
	if preview.State != "checked" || !preview.KeepsData || !slices.ContainsFunc(preview.DataPaths, func(path string) bool { return strings.HasPrefix(path, "/var/lib/postgresql") }) {
		t.Fatalf("preview = %+v", preview)
	}
	// What a purge would do, simulated only: CelikPanel no longer purges an engine.
	a107Out(t, "apt-get", "-s", "purge", "postgresql-17")
	ops, err := defaultServiceUninstallOps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var removed UninstallServiceResponse
	if err := a.uninstallServiceWithOps(&InstallServiceRequest{ID: "postgresql"}, &removed, ops); err != nil {
		t.Fatal(err)
	}
	logJSON(t, "removal", removed)
	if removed.Error != "" || !removed.Removed {
		t.Fatalf("removal = %+v", removed)
	}
	for _, path := range []string{"/var/lib/postgresql/17/main/PG_VERSION", "/etc/postgresql/17/main/postgresql.conf"} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s did not stay: %v", path, err)
		}
	}
	a107Out(t, "dpkg", "-l", "postgresql-17")
	var install transport.ServiceInstallPreviewResponse
	if err := a.ServiceInstallPreviewV1(&transport.ServiceInstallPreviewRequest{Steps: []transport.ServiceInstallPreviewStep{{ID: "postgresql"}}}, &install); err != nil {
		t.Fatal(err)
	}
	logJSON(t, "install preview with the kept cluster", install)
	if len(install.ExistingData) == 0 {
		t.Fatalf("the install preview does not name the kept data: %+v", install)
	}
	if resp := a105Install(t, a, "postgresql", "", "", nil); resp.Error != "" {
		t.Fatalf("reinstall: %s", resp.Error)
	}
	if out := a107Out(t, "runuser", "-u", "postgres", "--", "psql", "-tAc", "SELECT count(*) FROM pg_database WHERE datname = 'a107evidence'"); strings.TrimSpace(out) != "1" {
		t.Fatalf("the kept cluster's database is not back: %q", out)
	}
	a107Out(t, "pg_lsclusters")
}

func a107Fetch(t *testing.T, url string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("%s: HTTP %d", url, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, roundcubeMaxSize+1))
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestRealSystemA107WebmailUpdateRollsBackThenUpdates(t *testing.T) {
	requireRealSystemA107(t)
	if a107Version("php8.4-fpm") == "" {
		t.Skip("runs after the repository test")
	}
	a := &Agent{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	// An older release on the 1.6 line, verified with Roundcube's signing
	// key before its digest is trusted for the install.
	const older = "1.6.15"
	archive := a107Fetch(t, roundcubeReleaseURL(older, ""))
	signature := a107Fetch(t, roundcubeReleaseURL(older, ".asc"))
	if err := verifyRoundcubeSignature(strings.NewReader(string(archive)), signature, roundcubeSigningKey, roundcubeSigningPrimary); err != nil {
		t.Fatalf("Roundcube %s's signature: %v", older, err)
	}
	sum := sha256.Sum256(archive)
	previousVersion, previousSHA := roundcubeVersion, roundcubeSHA256
	roundcubeVersion, roundcubeSHA256 = older, hex.EncodeToString(sum[:])
	t.Cleanup(func() { roundcubeVersion, roundcubeSHA256 = previousVersion, previousSHA })
	var installed InstallRoundcubeResponse
	a.installRoundcubeWith(ctx, &installed)
	logJSON(t, "install "+older, installed)
	if installed.Error != "" {
		t.Fatalf("install: %s", installed.Error)
	}
	var configured ConfigureWebmailResponse
	a.configureWebmailWith(ctx, &configured)
	logJSON(t, "configure", configured)
	if configured.Error != "" {
		t.Fatalf("configure: %s", configured.Error)
	}
	host := hostRoundcubeUpdateOps()
	if err := host.prove(ctx); err != nil {
		t.Fatalf("webmail %s does not answer: %v", older, err)
	}
	configPath := filepath.Join(webmailBaseDir, "config", "config.inc.php")
	configBefore := roundcubeFileSHA256(configPath)
	check := roundcubeUpdateCheck(ctx, true)
	logJSON(t, "update check", check)
	if !check.Installed || check.Version != older || check.LatestOnLine == "" || !roundcubeVersionLess(older, check.LatestOnLine) {
		t.Fatalf("check = %+v", check)
	}
	target := check.LatestOnLine

	// 1. The new tree is made not to answer: the previous tree is put back.
	forced := host
	calls := 0
	forced.prove = func(ctx context.Context) error {
		calls++
		if calls == 1 {
			return fmt.Errorf("injected for the evidence: Roundcube %s did not answer", target)
		}
		return host.prove(ctx)
	}
	back := updateRoundcubeTo(ctx, target, forced)
	logJSON(t, "update with a forced failed proof", back)
	if back.Updated || !back.RolledBack || back.ErrorCode != "PROOF_FAILED" || back.Backup == "" {
		t.Fatalf("rollback = %+v", back)
	}
	if got := roundcubeTreeVersion(webmailBaseDir); got != older {
		t.Fatalf("after the rollback the tree is %q", got)
	}
	if err := host.prove(ctx); err != nil {
		t.Fatalf("webmail does not answer after the rollback: %v", err)
	}
	if roundcubeFileSHA256(configPath) != configBefore {
		t.Fatalf("the configuration changed")
	}

	// 2. The real update.
	updated := updateRoundcubeTo(ctx, target, host)
	logJSON(t, "update", updated)
	if !updated.Updated || !updated.Proved || updated.Error != "" {
		t.Fatalf("update = %+v", updated)
	}
	if got := roundcubeTreeVersion(webmailBaseDir); got != target {
		t.Fatalf("the tree is %q", got)
	}
	if roundcubeFileSHA256(configPath) != configBefore {
		t.Fatalf("the configuration was not carried")
	}
	a107Out(t, "ls", "-la", roundcubeBackupDir)
}

func TestRealSystemA107RestartsAndReleaseAreRead(t *testing.T) {
	requireRealSystemA107(t)
	a107Out(t, "apt-get", "install", "-y", "-q", "--no-install-recommends", "needrestart")
	ctx, cancel := context.WithTimeout(context.Background(), restartNeedsBound)
	defer cancel()
	needs := restartNeeds(ctx)
	logJSON(t, "restarts and reboot", needs)
	if !needs.ServicesKnown || !needs.RebootKnown || needs.Source != "needrestart" {
		t.Fatalf("needs = %+v", needs)
	}
	facts := osReleaseFacts()
	logJSON(t, "release facts", facts)
	if facts.ID != "debian" || facts.Codename != "trixie" || facts.RepoSuites["sury"] != "trixie" || !strings.HasPrefix(facts.RepoSuites["pgdg"], "trixie") {
		t.Fatalf("facts = %+v", facts)
	}
	if a107Version("php8.4-fpm") != "" && !slices.Contains(facts.PHPVersions, "8.4") {
		t.Fatalf("PHP lines = %v", facts.PHPVersions)
	}
	if a107Version("postgresql-17") != "" && !slices.Contains(facts.PostgreSQLClusters, "17/main") {
		t.Fatalf("clusters = %v", facts.PostgreSQLClusters)
	}
}
