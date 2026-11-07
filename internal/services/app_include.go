package services

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Application rules in the site's CelikPanel include directory (alpha.118,
// D-031): /etc/nginx/celikpanel-managed.d/<domain>/app-<id>.conf, and the
// setup lock app-setup-lock.conf that answers every address of the site with
// "being set up" while an application's own installer has not finished (so
// nobody else can complete that installer in a browser). Each file carries
// CelikPanel's render header; a file the owner edited is never overwritten
// or removed, and the refusal says so. Every write is checked with nginx -t
// and put back when nginx refuses it.
//
// Uygulama kuralları ve kurulum kilidi sitenin CelikPanel ekleme dizinindedir.
// Sahibin düzenlediği dosyanın üzerine yazılmaz, silinmez; her yazım nginx -t
// ile denetlenir, reddedilirse geri alınır.

// AppSetupLockFileName is the setup lock's file name.
const AppSetupLockFileName = "app-setup-lock.conf"

var appIncludeName = regexp.MustCompile(`^app-[a-z][a-z0-9-]{1,40}\.conf$`)

// ErrAppIncludeKept: the file is not CelikPanel's unchanged text (the owner
// edited it, or it is someone else's); it was left as it is.
var ErrAppIncludeKept = errors.New("the file is not CelikPanel's unchanged text and was left as it is")

// AppIncludePath is the path of one application include file of a site.
func AppIncludePath(domain, fileName string) (string, error) {
	if !appIncludeName.MatchString(fileName) {
		return "", fmt.Errorf("invalid application include name %q", fileName)
	}
	if domain == "" || strings.ContainsAny(domain, "/\\\x00") || domain == "." || domain == ".." {
		return "", fmt.Errorf("invalid domain %q", domain)
	}
	return filepath.Join(PanelManagedDir(domain), fileName), nil
}

// RenderAppSetupLock is the body of the setup lock of one site.
func RenderAppSetupLock(domain, appName string) string {
	return "# " + appName + " is being set up on " + domain + " by CelikPanel. Until its setup\n" +
		"# finishes every address of the site answers \"being set up\", so nobody can\n" +
		"# complete the application's own installer in a browser. CelikPanel removes\n" +
		"# this file when the setup finishes or the application is removed.\n" +
		"# Kurulum kilidi: kurulum bitene kadar site \"kuruluyor\" yanıtı verir.\n" +
		"location ~ ^/ {\n" +
		"    default_type text/html;\n" +
		"    charset utf-8;\n" +
		"    add_header Retry-After 300 always;\n" +
		"    add_header Cache-Control \"no-store\" always;\n" +
		"    return 503 '<!doctype html><html><head><meta charset=\"utf-8\"><meta name=\"robots\" content=\"noindex\"><title>Being set up</title></head><body style=\"font-family:sans-serif;max-width:40em;margin:4em auto;padding:0 1em\"><h1>This site is being set up</h1><p>Bu site kuruluyor.</p><p>Please try again in a few minutes. / Lütfen birkaç dakika sonra yeniden deneyin.</p></body></html>';\n" +
		"}\n"
}

// AppIncludeState reads one include file: "absent", "managed_unchanged"
// (CelikPanel's text, whatever its body), or the kept states.
func AppIncludeState(domain, fileName string) (string, error) {
	path, err := AppIncludePath(domain, fileName)
	if err != nil {
		return "", err
	}
	nginxMutationMu.Lock()
	defer nginxMutationMu.Unlock()
	return inspectSiteFile(path, nil, "").state, nil
}

// PublishAppInclude writes an application include file (body without the
// render header), checks nginx and reloads it. A file the owner changed is
// kept (ErrAppIncludeKept); identical text is left and nothing is reloaded.
func (ng *NginxGenerator) PublishAppInclude(domain, fileName, body string) error {
	path, err := AppIncludePath(domain, fileName)
	if err != nil {
		return err
	}
	nginxMutationMu.Lock()
	defer nginxMutationMu.Unlock()
	// alpha.128: when the site's settings own the upload size, the
	// application's include is written without its own line (nginx refuses
	// the directive twice).
	if fileName != AppSetupLockFileName && siteSettingsOwnsUpload(domain) {
		body, _ = withoutAppUploadLines(body)
	}
	sealed := []byte(SealManagedText(body))
	inspected := inspectSiteFile(path, nil, "")
	switch inspected.state {
	case transport.SiteFileAbsent:
	case transport.SiteFileManagedUnchanged:
		if bytes.Equal(inspected.content, sealed) {
			return nil
		}
	default:
		return fmt.Errorf("%s: %w", path, ErrAppIncludeKept)
	}
	if err := ensurePanelManagedDir(domain); err != nil {
		return fmt.Errorf("prepare %s: %w", PanelManagedDir(domain), err)
	}
	if err := writeManagedSiteFile(path, sealed, inspected); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	written := []plannedSiteFileWrite{{domain: domain, available: path, sealed: sealed, inspected: inspected}}
	if err := ng.ValidateNginx(); err != nil {
		detail := err.Error()
		var refused *NginxConfigRefusedError
		if errors.As(err, &refused) {
			detail = refused.FirstLine()
		}
		rollbackErr := restorePlannedSiteFiles(written)
		return fmt.Errorf("nginx refused %s, the file was put back: %w", filepath.Base(path),
			ng.finishManagedRollback(errors.New(detail), rollbackErr))
	}
	if err := ng.ReloadNginx(); err != nil {
		rollbackErr := restorePlannedSiteFiles(written)
		return fmt.Errorf("nginx did not reload with %s, the file was put back: %w", filepath.Base(path),
			ng.finishManagedRollback(err, rollbackErr))
	}
	return nil
}

// RemoveAppInclude removes an application include file when it is
// CelikPanel's unchanged text, then checks nginx and reloads it. An absent
// file is done; a file the owner changed is kept (ErrAppIncludeKept).
func (ng *NginxGenerator) RemoveAppInclude(domain, fileName string) error {
	path, err := AppIncludePath(domain, fileName)
	if err != nil {
		return err
	}
	nginxMutationMu.Lock()
	defer nginxMutationMu.Unlock()
	inspected := inspectSiteFile(path, nil, "")
	switch inspected.state {
	case transport.SiteFileAbsent:
		return nil
	case transport.SiteFileManagedUnchanged:
	default:
		return fmt.Errorf("%s: %w", path, ErrAppIncludeKept)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	removed := []plannedSiteFileWrite{{domain: domain, available: path, inspected: inspected}}
	if err := ng.ValidateNginx(); err != nil {
		rollbackErr := restorePlannedSiteFiles(removed)
		return fmt.Errorf("nginx refused the configuration without %s, the file was put back: %w", filepath.Base(path),
			ng.finishManagedRollback(err, rollbackErr))
	}
	if err := ng.ReloadNginx(); err != nil {
		return fmt.Errorf("%s was removed but nginx did not reload: %w", filepath.Base(path), err)
	}
	return nil
}

// removeUnchangedAppIncludes removes the application include files of a
// deleted site that are still CelikPanel's unchanged text. The caller holds
// nginxMutationMu.
func removeUnchangedAppIncludes(domain string) {
	entries, err := os.ReadDir(PanelManagedDir(domain))
	if err != nil {
		return
	}
	for _, entry := range entries {
		// alpha.128: the site's settings file goes with the site too.
		if !appIncludeName.MatchString(entry.Name()) && entry.Name() != SiteSettingsFileName {
			continue
		}
		path := filepath.Join(PanelManagedDir(domain), entry.Name())
		if inspectSiteFile(path, nil, "").state == transport.SiteFileManagedUnchanged {
			_ = os.Remove(path)
		}
	}
}
