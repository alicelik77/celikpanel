package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/appcatalog"
	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/hostname"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Catalog applications (alpha.118, docs/APP-CATALOG.md). Three privileged
// steps, each a separate RPC the Panel records before and after:
//
//  1. InstallCatalogAppV1: the pinned release is downloaded into a
//     root-private folder on the hosting file system, its SHA-256 is compared
//     with the catalog's (a mismatch stops everything), it is extracted with
//     the same limits and path rules as WordPress, given to the site's user,
//     and published into the empty document root by the same atomic exchange
//     as WordPress. Before publication the site's setup lock is written so
//     nobody can run the application's own web installer in a browser.
//  2. SetupCatalogAppV1: the application's own non-interactive installer runs
//     as the site's user, with the site's PHP, its secrets in a file only that
//     user can read (never on a command line, never in a log). Then the
//     application's nginx rules are written and the setup lock removed.
//     Running it again after a failure is safe: a finished setup is detected
//     and only CelikPanel's part is done.
//  3. RemoveCatalogAppV1: the application's files are removed as the site's
//     user, the placeholder page is put back and its nginx rules removed. Its
//     private data folder only with explicit confirmation.
//
// Katalog uygulamaları: sabit sürüm indirilir, SHA-256 karşılaştırılır
// (uyuşmazsa hiçbir şey değişmez), çıkarılır ve boş belge köküne atomik
// olarak konur; kurulumu sitenin kullanıcısı olarak uygulamanın kendi
// kurucusu yapar; gizli bilgiler komut satırında ya da günlükte yer almaz.

type (
	AppPrerequisitesRequest   = transport.AppPrerequisitesRequest
	AppPrerequisitesResponse  = transport.AppPrerequisitesResponse
	InstallCatalogAppRequest  = transport.InstallCatalogAppRequest
	InstallCatalogAppResponse = transport.InstallCatalogAppResponse
	SetupCatalogAppRequest    = transport.SetupCatalogAppRequest
	SetupCatalogAppResponse   = transport.SetupCatalogAppResponse
	RemoveCatalogAppRequest   = transport.RemoveCatalogAppRequest
	RemoveCatalogAppResponse  = transport.RemoveCatalogAppResponse
)

// Seams for tests.
var (
	catalogAppLookup     = appcatalog.Lookup
	catalogDownload      = downloadPinnedArchive
	catalogPHPExtensions = probePHPExtensions
	catalogPkgFamily     = detectPkgFamily
	catalogRunAsSiteUser = runAsSiteUser
	catalogFreeDiskMB    = freeDiskMB
	catalogNginx         = func(a *Agent) appIncludeWriter {
		if a == nil || a.nginxGen == nil {
			return nil
		}
		return a.nginxGen
	}
	catalogSetupTimeout    = 15 * time.Minute
	catalogDownloadTimeout = 20 * time.Minute
	// catalogSitePaths derives the document root and the site home from the
	// immutable identity (a test points it at a temporary tree).
	catalogSitePaths = func(subscriptionID, domainID int) (string, string, error) {
		docRoot, err := hostingpath.DocumentRoot(subscriptionID, domainID)
		if err != nil {
			return "", "", err
		}
		home, err := hostingpath.SiteHome(subscriptionID, domainID)
		return docRoot, home, err
	}
)

// appIncludeWriter is the part of the nginx generator the catalog uses.
type appIncludeWriter interface {
	PublishAppInclude(domain, fileName, body string) error
	RemoveAppInclude(domain, fileName string) error
}

// catalogSiteLocks serializes the steps of one site (the Panel also holds
// its own per-domain lock).
var catalogSiteLocks sync.Map

func catalogSiteLock(siteID int) *sync.Mutex {
	lock, _ := catalogSiteLocks.LoadOrStore(siteID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

type catalogSite struct {
	app      appcatalog.App
	domain   string
	docRoot  string
	siteHome string
	sitesDir string
	username string
	phpLine  string
}

// resolveCatalogSite validates the request's identity against the catalog
// and derives every path.
func resolveCatalogSite(appID string, site transport.AppSiteIdentity) (catalogSite, error) {
	app, ok := catalogAppLookup(appID)
	if !ok || app.Kind != appcatalog.KindPHPSite || app.Legacy() {
		return catalogSite{}, fmt.Errorf("%q is not an application this agent can install", appID)
	}
	domain, err := hostname.CanonicalFQDN(site.Domain)
	if err != nil || domain != site.Domain || site.SiteID <= 0 || site.SubscriptionID <= 0 || site.DomainID <= 0 {
		return catalogSite{}, errors.New("site, subscription and domain identities are required")
	}
	username := strings.TrimSpace(site.Username)
	if username == "" || username != services.SiteUsername(domain) {
		return catalogSite{}, errors.New("the site user does not belong to this domain")
	}
	docRoot, home, err := catalogSitePaths(site.SubscriptionID, site.DomainID)
	if err != nil {
		return catalogSite{}, errors.New("invalid hosting identity")
	}
	return catalogSite{
		app: app, domain: domain, docRoot: docRoot, siteHome: home,
		sitesDir: filepath.Dir(home), username: username,
		phpLine: appcatalog.PHPLine(site.PHPVersion),
	}, nil
}

// ---- prerequisites (read only) ----

// AppPrerequisitesV1 answers what stands between this site and the
// application, changing nothing.
func (a *Agent) AppPrerequisitesV1(req *AppPrerequisitesRequest, resp *AppPrerequisitesResponse) error {
	if req == nil || resp == nil {
		return errors.New("request and response are required")
	}
	*resp = AppPrerequisitesResponse{}
	site, err := resolveCatalogSite(req.AppID, req.Site)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	resp.Family = catalogPkgFamily()
	resp.PHPLine = site.phpLine
	resp.PHPOK, resp.PHPReason = site.app.PHPSupported(req.Site.PHPVersion)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	missing, err := missingCatalogExtensions(ctx, site.app, site.phpLine, resp.Family)
	if err != nil {
		resp.ExtensionsUnknown = true
	} else {
		resp.Missing = missing
	}
	resp.DocRoot = catalogDocRootState(site)
	resp.FreeDiskMB = catalogFreeDiskMB(site.sitesDir)
	return nil
}

func missingCatalogExtensions(ctx context.Context, app appcatalog.App, line, family string) ([]transport.AppMissingExtension, error) {
	if len(app.Extensions) == 0 {
		return nil, nil
	}
	missing, err := catalogPHPExtensions(ctx, line, app.Extensions)
	if err != nil {
		return nil, err
	}
	out := []transport.AppMissingExtension{}
	seen := map[string]bool{}
	for _, ext := range missing {
		source := appcatalog.ExtensionSource(family, line, ext)
		key := source.Package + "|" + source.Extension
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, transport.AppMissingExtension{Extension: source.Extension, Package: source.Package, Enable: source.Enable})
	}
	return out, nil
}

var phpExtensionName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,40}$`)

// probePHPExtensions asks the PHP of one version line which of the
// extensions it does not load.
func probePHPExtensions(ctx context.Context, line string, extensions []string) ([]string, error) {
	quoted := make([]string, 0, len(extensions))
	for _, ext := range extensions {
		if !phpExtensionName.MatchString(ext) {
			return nil, fmt.Errorf("invalid extension name %q", ext)
		}
		quoted = append(quoted, "'"+ext+"'")
	}
	code := "$m=[];foreach([" + strings.Join(quoted, ",") + "] as $e){if(!extension_loaded($e)){$m[]=$e;}}echo 'missing:'.implode(',',$m);"
	// Debian and Ubuntu enable an extension package for every PHP interface
	// at once (phpenmod), so the command-line PHP answers for the site's pool.
	out, err := serviceMutationCommand(ctx, phpCLIForVersion(line), "-r", code).Output()
	if err != nil {
		return nil, fmt.Errorf("PHP %s could not be asked for its extensions: %w", line, err)
	}
	result := strings.TrimSpace(string(out))
	list, ok := strings.CutPrefix(result, "missing:")
	if !ok {
		return nil, fmt.Errorf("PHP %s answered the extension check unexpectedly", line)
	}
	return splitExtensions(list), nil
}

func catalogDocRootState(site catalogSite) string {
	snapshot, err := inspectInstallableDocumentRoot(site.docRoot, site.domain)
	if err != nil {
		if requireCanonicalDirectory(site.docRoot) != nil {
			return transport.AppDocRootUnavailable
		}
		return transport.AppDocRootContent
	}
	if snapshot.placeholderName != "" {
		return transport.AppDocRootPlaceholder
	}
	return transport.AppDocRootEmpty
}

// ---- step 1: files ----

func (a *Agent) InstallCatalogAppV1(req *InstallCatalogAppRequest, resp *InstallCatalogAppResponse) error {
	if req == nil || resp == nil {
		return errors.New("request and response are required")
	}
	*resp = InstallCatalogAppResponse{CompensationSafe: true}
	fail := func(kind, format string, args ...any) error {
		resp.ErrorKind = kind
		resp.Error = fmt.Sprintf(format, args...)
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "application installation"); err != nil {
		return fail(transport.AppFailInternal, "%s", err.Error())
	}
	if !validWordPressOperationID(req.OperationID) {
		return fail(transport.AppFailInternal, "a valid operation id is required")
	}
	site, err := resolveCatalogSite(req.AppID, req.Site)
	if err != nil {
		return fail(transport.AppFailInternal, "%s", err.Error())
	}
	app := site.app
	if req.Version != app.Version {
		return fail(transport.AppFailInternal, "the Panel asked for %s %s but this agent's catalog pins %s; nothing was changed. Update the Panel and the agent together", app.Name, req.Version, app.Version)
	}
	if ok, reason := app.PHPSupported(req.Site.PHPVersion); !ok {
		return fail(transport.AppFailPrerequisite, "%s; nothing was changed", reason)
	}

	lock := catalogSiteLock(req.Site.SiteID)
	lock.Lock()
	defer lock.Unlock()
	mutationMu := siteMutationMutex(req.Site.SiteID)
	mutationMu.Lock()
	defer mutationMu.Unlock()

	if _, err := inspectInstallableDocumentRoot(site.docRoot, site.domain); err != nil {
		return fail(transport.AppFailContent, "%s", err.Error())
	}
	if err := requireCanonicalDirectory(site.sitesDir); err != nil {
		return fail(transport.AppFailPublish, "invalid hosting layout: %v", err)
	}
	if err := requireWordPressStagingParent(site.sitesDir); err != nil {
		return fail(transport.AppFailPublish, "unsafe hosting layout: %v", err)
	}
	needMB := app.Archive.MaxDownloadBytes/(1<<20) + app.MinDiskMB*2
	if free := catalogFreeDiskMB(site.sitesDir); free > 0 && free < needMB {
		return fail(transport.AppFailPrerequisite, "the server's disk has %d MB free where sites live; installing %s needs about %d MB while it unpacks. Nothing was changed. The administrator frees disk space, then you install again", free, app.Name, needMB)
	}

	// The download lives on the hosting file system in a root-private folder
	// (not /tmp, which is a small memory file system on some systems).
	downloadDir, err := os.MkdirTemp(site.sitesDir, ".celikpanel-app-download-")
	if err != nil {
		return fail(transport.AppFailPublish, "prepare download: %v", err)
	}
	defer os.RemoveAll(downloadDir)
	if err := os.Chmod(downloadDir, 0o700); err != nil {
		return fail(transport.AppFailPublish, "protect download folder: %v", err)
	}
	archive, kind, err := fetchVerifiedRelease(app, downloadDir)
	if err != nil {
		return fail(kind, "%s", err.Error())
	}

	stageDir, err := os.MkdirTemp(site.sitesDir, fmt.Sprintf(".app-stage-%d-", req.Site.DomainID))
	if err != nil {
		return fail(transport.AppFailPublish, "prepare staging directory: %v", err)
	}
	removeStage := true
	defer func() {
		if removeStage {
			_ = os.RemoveAll(stageDir)
		}
	}()
	if err := os.Chmod(stageDir, 0o700); err != nil {
		return fail(transport.AppFailPublish, "protect staging directory: %v", err)
	}
	if err := extractCatalogArchive(app, archive, stageDir, downloadDir); err != nil {
		return fail(transport.AppFailUnpack, "the %s release could not be unpacked safely (%v); nothing was changed on the site. Try again later; if it repeats, report it to CelikPanel with this message", app.Name, err)
	}
	_ = os.Remove(archive)
	if err := applyWordPressOwnership(stageDir, site.username); err != nil {
		return fail(transport.AppFailPublish, "apply site ownership: %v", err)
	}

	// The setup lock goes in before the files are visible: until the
	// application's own setup finishes, nobody can finish it in a browser.
	if nginx := catalogNginx(a); nginx != nil {
		if err := nginx.PublishAppInclude(site.domain, services.AppSetupLockFileName, services.RenderAppSetupLock(site.domain, app.Name)); err != nil {
			return fail(transport.AppFailNginx, "the site could not be closed for setup (%v); nothing was changed", err)
		}
	} else {
		return fail(transport.AppFailInternal, "the nginx configuration is not available on this agent; nothing was changed")
	}
	removeLock := true
	defer func() {
		if removeLock {
			if nginx := catalogNginx(a); nginx != nil {
				_ = nginx.RemoveAppInclude(site.domain, services.AppSetupLockFileName)
			}
		}
	}()

	snapshot, err := inspectInstallableDocumentRoot(site.docRoot, site.domain)
	if err != nil {
		return fail(transport.AppFailContent, "%s", err.Error())
	}
	warning, preserveOldRoot, compensationSafe, err := publishWordPressStage(
		stageDir, site.docRoot, site.sitesDir, site.domain, snapshot,
		func(exchange wordpressPathExchange) error { return exchange.FinalizePublishedRoot(site.username) },
	)
	if err != nil {
		resp.CompensationSafe = compensationSafe
		removeLock = compensationSafe
		return fail(transport.AppFailPublish, "publish failed: %v", err)
	}
	removeStage = !preserveOldRoot
	removeLock = false
	resp.CompensationSafe = false
	resp.Installed = true
	resp.Detail = fmt.Sprintf("%s %s downloaded, verified (SHA-256) and put in place; setup comes next", app.Name, app.Version)
	if warning != "" {
		resp.Detail += "; " + strings.ReplaceAll(warning, "WordPress", app.Name)
	}
	return nil
}

// fetchVerifiedRelease downloads the pinned release into dir and compares
// its SHA-256 with the catalog's. A file that does not match is deleted and
// the error says so; nothing else is touched.
func fetchVerifiedRelease(app appcatalog.App, dir string) (string, string, error) {
	archive := filepath.Join(dir, "release")
	ctx, cancel := context.WithTimeout(context.Background(), catalogDownloadTimeout)
	defer cancel()
	if err := catalogDownload(ctx, app.Archive.URL, archive, app.Archive.MaxDownloadBytes); err != nil {
		_ = os.Remove(archive)
		return "", transport.AppFailDownload, fmt.Errorf("the %s %s release could not be downloaded from %s (%v); nothing was changed. Check that this server can reach that address, then install again", app.Name, app.Version, app.Archive.URL, err)
	}
	if err := verifyPinnedSHA256(archive, app.Archive.SHA256); err != nil {
		_ = os.Remove(archive)
		return "", transport.AppFailVerification, fmt.Errorf("the downloaded %s %s release is not the file CelikPanel pinned (%v). It was deleted and nothing was changed. Do not install it by hand; this can be a damaged download or a changed file on the download server. Try again later; if it repeats, report it to CelikPanel", app.Name, app.Version, err)
	}
	return archive, "", nil
}

// downloadPinnedArchive fetches url over HTTPS (redirects only to HTTPS) into
// destination, at most limit bytes.
func downloadPinnedArchive(ctx context.Context, url, destination string, limit int64) (resultErr error) {
	if !strings.HasPrefix(url, "https://") {
		return errors.New("only https addresses are downloaded")
	}
	request, err := newGetRequest(ctx, url)
	if err != nil {
		return err
	}
	response, err := wpHTTPClient(catalogDownloadTimeout).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	f, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if resultErr != nil {
			_ = os.Remove(destination)
		}
	}()
	if limit <= 0 {
		limit = 64 << 20
	}
	written, copyErr := io.Copy(f, io.LimitReader(response.Body, limit+1))
	if copyErr == nil && written > limit {
		copyErr = fmt.Errorf("the download is larger than %d bytes", limit)
	}
	if copyErr == nil {
		copyErr = f.Sync()
	}
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// verifyPinnedSHA256 compares a file with the catalog's digest.
func verifyPinnedSHA256(path, want string) error {
	want = strings.ToLower(strings.TrimSpace(want))
	if len(want) != sha256.Size*2 {
		return errors.New("the catalog has no valid SHA-256 for this release")
	}
	if _, err := hex.DecodeString(want); err != nil {
		return errors.New("the catalog has no valid SHA-256 for this release")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return err
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if got != want {
		return fmt.Errorf("SHA-256 %s, expected %s", got, want)
	}
	return nil
}

// ---- step 2: setup ----

func (a *Agent) SetupCatalogAppV1(req *SetupCatalogAppRequest, resp *SetupCatalogAppResponse) error {
	if req == nil || resp == nil {
		return errors.New("request and response are required")
	}
	*resp = SetupCatalogAppResponse{}
	fail := func(kind, format string, args ...any) error {
		resp.ErrorKind = kind
		resp.Error = fmt.Sprintf(format, args...)
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "application setup"); err != nil {
		return fail(transport.AppFailInternal, "%s", err.Error())
	}
	if !validWordPressOperationID(req.OperationID) {
		return fail(transport.AppFailInternal, "a valid operation id is required")
	}
	site, err := resolveCatalogSite(req.AppID, req.Site)
	if err != nil {
		return fail(transport.AppFailInternal, "%s", err.Error())
	}
	if req.DBName == "" || req.DBUser == "" || req.DBPass == "" || len(req.AdminPass) < 12 || !strings.Contains(req.AdminEmail, "@") {
		return fail(transport.AppFailInternal, "database details, an administrator password and an e-mail address are required")
	}
	lock := catalogSiteLock(req.Site.SiteID)
	lock.Lock()
	defer lock.Unlock()

	if !catalogMarkersPresent(site) {
		return fail(transport.AppFailContent, "the site's document root does not hold %s any more; nothing was changed", site.app.Name)
	}
	plan, err := buildSetupPlan(site, req)
	if err != nil {
		return fail(transport.AppFailInternal, "%v", err)
	}
	secrets := []string{req.DBPass, req.AdminPass}
	ctx, cancel := context.WithTimeout(context.Background(), catalogSetupTimeout)
	defer cancel()
	if !plan.alreadyDone() {
		for _, step := range plan.steps {
			output, runErr := runSetupStep(ctx, site, step)
			resp.Output = scrubSecrets(output, secrets)
			if runErr != nil {
				return fail(transport.AppFailSetup, "%s's own installer did not finish (%s: %v). The site stays closed with a \"being set up\" page. Retry the setup; if it fails again, remove the application and install it again", site.app.Name, step.name, runErr)
			}
		}
		if !plan.alreadyDone() {
			return fail(transport.AppFailSetup, "%s's installer ended without writing %s. The site stays closed with a \"being set up\" page. Retry the setup", site.app.Name, site.app.ConfigFile)
		}
	} else {
		resp.AlreadyConfigured = true
	}
	for _, step := range plan.after {
		output, runErr := runSetupStep(ctx, site, step)
		if runErr != nil {
			resp.Output = scrubSecrets(output, secrets)
			return fail(transport.AppFailSetup, "%s is set up, but finishing it failed (%s: %v). The site stays closed; retry the setup", site.app.Name, step.name, runErr)
		}
	}
	resp.AdminPath = site.app.AdminPath
	if plan.adminPath != nil {
		path, err := plan.adminPath(site)
		if err != nil {
			return fail(transport.AppFailSetup, "%s is set up, but its administration folder could not be found (%v); retry the setup", site.app.Name, err)
		}
		resp.AdminPath = path
	}

	nginx := catalogNginx(a)
	if nginx == nil {
		return fail(transport.AppFailInternal, "the nginx configuration is not available on this agent; the site stays closed. Retry the setup")
	}
	if site.app.HasNginxRules() {
		err := nginx.PublishAppInclude(site.domain, appcatalog.IncludeFileName(site.app.ID), site.app.RenderNginx(site.domain))
		switch {
		case errors.Is(err, services.ErrAppIncludeKept):
			resp.RulesKept = true
		case err != nil:
			return fail(transport.AppFailNginx, "%s is set up, but its nginx rules were not accepted (%v). The site stays closed; retry the setup, or ask the administrator to look at nginx", site.app.Name, err)
		}
	}
	if err := nginx.RemoveAppInclude(site.domain, services.AppSetupLockFileName); err != nil {
		return fail(transport.AppFailNginx, "%s is set up, but the \"being set up\" page could not be taken down (%v); retry the setup", site.app.Name, err)
	}
	resp.Configured = true
	resp.Detail = site.app.Name + " " + site.app.Version + " is set up"
	if resp.RulesKept {
		resp.Detail += "; its nginx rules file was changed by the owner and was kept as it is"
	}
	return nil
}

// catalogMarkersPresent: the document root still holds the application.
func catalogMarkersPresent(site catalogSite) bool {
	for _, marker := range site.app.Archive.Markers {
		switch marker {
		case "install/index_cli.php", "installation/joomla.php", "install/phpbbcli.php", "admin/":
			continue // removed or renamed by a finished setup
		}
		path := filepath.Join(site.docRoot, filepath.FromSlash(strings.TrimSuffix(marker, "/")))
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || info.IsDir() != strings.HasSuffix(marker, "/") {
			return false
		}
	}
	return true
}

// scrubSecrets removes every secret from installer output and keeps its end.
func scrubSecrets(output string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			output = strings.ReplaceAll(output, secret, "********")
		}
	}
	output = strings.TrimSpace(output)
	const keep = 2000
	if len(output) > keep {
		output = "…" + output[len(output)-keep:]
	}
	return output
}

// runSetupStep runs one step as the site's user (see apps_catalog_setup.go).
func runSetupStep(ctx context.Context, site catalogSite, step setupStep) (string, error) {
	args, stdin, err := step.command(site)
	if err != nil {
		return "", err
	}
	return catalogRunAsSiteUser(ctx, site.username, site.docRoot, args, stdin)
}

// ---- step 3: removal ----

func (a *Agent) RemoveCatalogAppV1(req *RemoveCatalogAppRequest, resp *RemoveCatalogAppResponse) error {
	if req == nil || resp == nil {
		return errors.New("request and response are required")
	}
	*resp = RemoveCatalogAppResponse{}
	fail := func(kind, format string, args ...any) error {
		resp.ErrorKind = kind
		resp.Error = fmt.Sprintf(format, args...)
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "application removal"); err != nil {
		return fail(transport.AppFailInternal, "%s", err.Error())
	}
	site, err := resolveCatalogSite(req.AppID, req.Site)
	if err != nil {
		return fail(transport.AppFailInternal, "%s", err.Error())
	}
	lock := catalogSiteLock(req.Site.SiteID)
	lock.Lock()
	defer lock.Unlock()
	mutationMu := siteMutationMutex(req.Site.SiteID)
	mutationMu.Lock()
	defer mutationMu.Unlock()

	nginx := catalogNginx(a)
	if nginx == nil {
		return fail(transport.AppFailInternal, "the nginx configuration is not available on this agent; nothing was removed")
	}
	state := catalogDocRootState(site)
	record, removalStarted := loadCatalogRemoval(req.Site.SiteID, site)
	placeholderName, placeholderContent := celikPanelSitePlaceholder(site.domain, "php")
	switch {
	case state == transport.AppDocRootUnavailable:
		return fail(transport.AppFailContent, "the site's document root could not be opened; nothing was removed")
	case state == transport.AppDocRootContent && !removalStarted && !catalogMarkersPresent(site):
		resp.NotThisApp = true
		return fail(transport.AppFailContent, "the site's document root does not hold %s any more (something else is there now); nothing was removed. Remove those files with the file manager if you want them gone", site.app.Name)
	case state == transport.AppDocRootContent:
		// The removal is recorded before the first file goes, with the names
		// of the document root's top-level entries at that moment (names
		// only). A retry after a partial removal continues it although the
		// application's own identifying files may be gone, and it deletes
		// only those recorded entries: anything the owner put there after
		// the removal started is the owner's and stays (D-022).
		// Kayıt yalnız başlangıçtaki üst düzey adları tutar; sonradan eklenen
		// her şey sahibinindir ve silinmez.
		if !removalStarted {
			names, err := catalogTopLevelNames(site.docRoot)
			if err != nil {
				return fail(transport.AppFailContent, "the site's document root could not be listed (%v); nothing was removed", err)
			}
			record = catalogRemovalRecordFor(req.Site.SiteID, site)
			record.Entries = names
			if err := markCatalogRemoval(req.Site.SiteID, site, record); err != nil {
				return fail(transport.AppFailInternal, "the removal could not be recorded (%v); nothing was removed. Retry the removal", err)
			}
		}
		targets := catalogRemovalTargets(site.docRoot, record.Entries)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if len(targets) > 0 {
			if err := removeCatalogRemovalTargets(ctx, site, targets); err != nil {
				// What could not be removed stays; the placeholder page goes back
				// beside it (never over a file) so visitors see the site's page
				// instead of an error, and the record says the removal continues.
				// Kaldırılamayanlar yerinde kalır; ziyaretçi hata yerine yer tutucu
				// sayfayı görsün diye yer tutucu (bir dosyanın üzerine yazmadan) konur.
				placeholderPath := filepath.Join(site.docRoot, placeholderName)
				_, existed := os.Lstat(placeholderPath)
				if _, err := catalogRunAsSiteUser(ctx, site.username, site.docRoot,
					[]string{"sh", "-c", `set -C; umask 027; cat > "$1"`, "sh", placeholderPath}, placeholderContent); err == nil && existed != nil {
					// The placeholder now under that name is CelikPanel's own,
					// not the application's: a continued removal leaves it
					// (and whatever the owner may put there instead).
					record.Entries = slices.DeleteFunc(record.Entries, func(name string) bool { return name == placeholderName })
					if len(record.Entries) > 0 {
						_ = markCatalogRemoval(req.Site.SiteID, site, record)
					}
				}
				return fail(transport.AppFailRemove, "removing %s stopped part way: some of its files could not be deleted (%s). What remains is in the site's document root, and the site shows its placeholder page where it could be put back. Press Remove again: it continues where it stopped. If it stops again, the administrator checks who owns those files (they must belong to the site's user)", site.app.Name, boundedText(err.Error(), 400))
			}
		}
		resp.KeptEntries = catalogKeptEntries(site.docRoot, record.Entries, placeholderName)
		if _, err := catalogRunAsSiteUser(ctx, site.username, site.docRoot,
			[]string{"sh", "-c", `set -C; umask 027; [ -e "$1" ] || cat > "$1"`, "sh", filepath.Join(site.docRoot, placeholderName)}, placeholderContent); err != nil {
			resp.Detail = "the placeholder page could not be put back"
		}
		if len(resp.KeptEntries) > 0 {
			resp.Detail = strings.TrimPrefix(resp.Detail+"; ", "; ") + fmt.Sprintf(
				"%d item(s) were added to the site's document root after the removal started and were kept as they are: %s. They are yours; remove them with the file manager if you do not want them",
				len(resp.KeptEntries), strings.Join(resp.KeptEntries, ", "))
		}
	}
	resp.FilesRemoved = true
	clearCatalogRemoval(req.Site.SiteID, site)

	for _, file := range []string{appcatalog.IncludeFileName(site.app.ID), services.AppSetupLockFileName} {
		err := nginx.RemoveAppInclude(site.domain, file)
		switch {
		case errors.Is(err, services.ErrAppIncludeKept):
			resp.RulesKept = true
		case err != nil:
			return fail(transport.AppFailNginx, "%s's files were removed, but its nginx rules could not be removed (%v); retry the removal", site.app.Name, err)
		}
	}
	resp.RulesRemoved = !resp.RulesKept

	if site.app.DataDir != "" {
		dataDir := filepath.Join(site.siteHome, site.app.DataDir)
		if _, err := os.Lstat(dataDir); err == nil {
			if req.RemoveData {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
				defer cancel()
				if _, err := catalogRunAsSiteUser(ctx, site.username, site.siteHome,
					[]string{"rm", "-rf", "--one-file-system", "--", dataDir}, nil); err != nil {
					return fail(transport.AppFailPublish, "%s was removed, but its data folder %s could not be removed (%v); retry the removal", site.app.Name, dataDir, err)
				}
				resp.DataRemoved = true
			} else {
				resp.DataKept = dataDir
			}
		}
	}
	return nil
}

func randomLowerAlnum(n int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	random := make([]byte, n)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	for i := range random {
		random[i] = charset[int(random[i])%len(charset)]
	}
	return string(random), nil
}
