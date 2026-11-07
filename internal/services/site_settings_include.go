package services

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The site's nginx settings file (alpha.128, docs/SITE-LIMITS.md):
// /etc/nginx/celikpanel-managed.d/<domain>/site-settings.conf, in CelikPanel's
// include directory, so it is read in every server block of the site at
// server level. It holds two values of the site's PHP settings:
//
//	client_max_body_size <largest upload>m;   nginx answers 413 before PHP
//	fastcgi_read_timeout <run time + 10>s;    nginx waits for a long PHP run
//
// Both are harmless in the HTTP->HTTPS redirect block. A directive that
// another file of the site already sets (an application's include such as
// app-nextcloud.conf, the owner's celikpanel-sites.d file) is not written,
// since nginx refuses the same directive twice in one block; that file's
// value applies and the answer names it. The file carries CelikPanel's render
// header; an owner-edited file is kept and nothing is written. Every write is
// checked with nginx -t and put back when nginx refuses it.
//
// Sitenin nginx ayar dosyası: yükleme sınırı ve PHP okuma süresi. Başka bir
// dosya aynı yönergeyi koyuyorsa yazılmaz; sahibin düzenlemesi korunur; her
// yazım nginx -t ile denetlenir, reddedilirse geri alınır.

// SiteSettingsFileName is the file's name in the site's include directory.
const SiteSettingsFileName = "site-settings.conf"

// SiteSettingsIncludePath is the file of one site.
func SiteSettingsIncludePath(domain string) (string, error) {
	if domain == "" || strings.ContainsAny(domain, "/\\\x00 ") || domain == "." || domain == ".." || len(domain) > 253 {
		return "", fmt.Errorf("invalid domain %q", domain)
	}
	return filepath.Join(PanelManagedDir(domain), SiteSettingsFileName), nil
}

// RenderSiteSettingsInclude is the body of the file (without the header);
// "" when neither value is written.
func RenderSiteSettingsInclude(domain string, uploadMB, timeoutS int) string {
	if uploadMB <= 0 && timeoutS <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Site settings of " + domain + ", written by CelikPanel from the site's PHP settings.\n")
	b.WriteString("# Change them on that page. A line you change here is kept, and CelikPanel stops writing this file.\n")
	b.WriteString("# Sitenin ayarları; PHP ayarları sayfasında değiştirin. Burada değiştirilen satır korunur.\n")
	if uploadMB > 0 {
		fmt.Fprintf(&b, "client_max_body_size %dm;\n", uploadMB)
	}
	if timeoutS > 0 {
		fmt.Fprintf(&b, "fastcgi_read_timeout %ds;\n", timeoutS)
	}
	return b.String()
}

// SiteSettingsIncludeState reads the file's D-031 state ("" when the domain
// is not a valid name).
func SiteSettingsIncludeState(domain string) string {
	path, err := SiteSettingsIncludePath(domain)
	if err != nil {
		return ""
	}
	nginxMutationMu.Lock()
	defer nginxMutationMu.Unlock()
	return inspectSiteFile(path, nil, "").state
}

type siteDirectiveOwners struct {
	uploadFile  string
	uploadMB    int
	timeoutFile string
	timeoutS    int
}

// siteSettingsDirectiveOwners finds the other files of the site that set
// client_max_body_size or fastcgi_read_timeout (anywhere in the file: a
// conservative reading that never writes a second one).
func siteSettingsDirectiveOwners(domain string) siteDirectiveOwners {
	var owners siteDirectiveOwners
	dirs := []string{PanelManagedDir(domain), OwnerIncludeDir(domain)}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		names := []string{}
		for _, entry := range entries {
			if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".conf") && entry.Name() != SiteSettingsFileName {
				names = append(names, entry.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			path := filepath.Join(dir, name)
			content, err := readManagedConfig(path)
			if err != nil || len(content) > maxManagedSiteFileBytes {
				continue
			}
			if value, ok := nginxDirectiveValue(content, "client_max_body_size"); ok && owners.uploadFile == "" {
				owners.uploadFile, owners.uploadMB = path, nginxSizeMB(value)
			}
			if value, ok := nginxDirectiveValue(content, "fastcgi_read_timeout"); ok && owners.timeoutFile == "" {
				owners.timeoutFile, owners.timeoutS = path, nginxSeconds(value)
			}
		}
	}
	return owners
}

// nginxDirectiveValue returns the argument of the first line that starts
// with the directive (comments ignored).
func nginxDirectiveValue(content []byte, directive string) (string, bool) {
	for _, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(raw)
		if index := strings.IndexByte(line, '#'); index >= 0 {
			line = strings.TrimSpace(line[:index])
		}
		fields := strings.Fields(strings.TrimSuffix(line, ";"))
		if len(fields) >= 2 && fields[0] == directive {
			return fields[1], true
		}
	}
	return "", false
}

func nginxSizeMB(value string) int {
	value = strings.ToLower(strings.TrimSpace(value))
	scale := 1.0 / (1024 * 1024)
	switch {
	case strings.HasSuffix(value, "k"):
		value, scale = strings.TrimSuffix(value, "k"), 1.0/1024
	case strings.HasSuffix(value, "m"):
		value, scale = strings.TrimSuffix(value, "m"), 1
	case strings.HasSuffix(value, "g"):
		value, scale = strings.TrimSuffix(value, "g"), 1024
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0
	}
	return int(float64(n) * scale)
}

func nginxSeconds(value string) int {
	value = strings.ToLower(strings.TrimSpace(value))
	scale := 1
	switch {
	case strings.HasSuffix(value, "ms"):
		return 0
	case strings.HasSuffix(value, "s"):
		value = strings.TrimSuffix(value, "s")
	case strings.HasSuffix(value, "m"):
		value, scale = strings.TrimSuffix(value, "m"), 60
	case strings.HasSuffix(value, "h"):
		value, scale = strings.TrimSuffix(value, "h"), 3600
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0
	}
	return n * scale
}

// ApplySiteSettingsInclude writes, keeps or removes the site's settings file.
// It never reports an applied change that nginx did not take.
func (ng *NginxGenerator) ApplySiteSettingsInclude(request transport.SiteSettingsIncludeRequest) transport.SiteSettingsIncludeResponse {
	var response transport.SiteSettingsIncludeResponse
	path, err := SiteSettingsIncludePath(request.Domain)
	if err != nil {
		response.Outcome, response.Error = transport.SiteSettingsIncludeRefused, err.Error()
		return response
	}
	if request.UploadMB < 0 || request.UploadMB > 10240 || request.TimeoutS < 0 || request.TimeoutS > 3700 {
		response.Outcome, response.Error = transport.SiteSettingsIncludeRefused, "values out of range"
		return response
	}
	response.Path = path
	nginxMutationMu.Lock()
	defer nginxMutationMu.Unlock()
	owners := siteSettingsDirectiveOwners(request.Domain)
	uploadMB, timeoutS := request.UploadMB, request.TimeoutS
	// alpha.128: the upload size belongs to the site setting. An
	// application's include that is CelikPanel's unchanged text gives its
	// line up (it is written again without it, checked with this file); an
	// edited one keeps it, and the answer names the file.
	var takeOver *plannedSiteFileWrite
	if owners.uploadFile != "" && uploadMB > 0 {
		takeOver = appIncludeTakeOver(request.Domain, owners.uploadFile)
		if takeOver != nil {
			response.TookOverFrom = owners.uploadFile
			owners.uploadFile = ""
		}
	}
	if owners.uploadFile != "" {
		response.UploadOwner, uploadMB = owners.uploadFile, 0
	}
	if owners.timeoutFile != "" {
		response.TimeoutOwner, timeoutS = owners.timeoutFile, 0
	}
	body := RenderSiteSettingsInclude(request.Domain, uploadMB, timeoutS)
	inspected := inspectSiteFile(path, nil, "")
	switch inspected.state {
	case transport.SiteFileAbsent:
		if body == "" && takeOver == nil {
			response.Outcome = transport.SiteSettingsIncludeUnchanged
			return response
		}
	case transport.SiteFileManagedUnchanged:
		if body != "" && takeOver == nil && bytes.Equal(inspected.content, []byte(SealManagedText(body))) {
			response.Outcome = transport.SiteSettingsIncludeUnchanged
			return response
		}
	default:
		response.Outcome = transport.SiteSettingsIncludeKept
		response.Detail = inspected.state
		return response
	}
	// An unmet prerequisite, not this file's fault: nginx already refuses the
	// server's configuration, so no check of this change could pass.
	if err := ng.ValidateNginx(); err != nil {
		response.Outcome = transport.SiteSettingsIncludeRefused
		response.Error = "the server's nginx configuration already fails its check; nothing was written"
		response.Detail = SiteFileCheckLine(err)
		return response
	}
	var written []plannedSiteFileWrite
	if body == "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			response.Outcome, response.Error = transport.SiteSettingsIncludeRefused, err.Error()
			return response
		}
		written = []plannedSiteFileWrite{{domain: request.Domain, available: path, inspected: inspected}}
		response.Outcome = transport.SiteSettingsIncludeRemoved
	} else {
		if err := ensurePanelManagedDir(request.Domain); err != nil {
			response.Outcome, response.Error = transport.SiteSettingsIncludeRefused, fmt.Sprintf("prepare %s: %v", PanelManagedDir(request.Domain), err)
			return response
		}
		sealed := []byte(SealManagedText(body))
		if err := writeManagedSiteFile(path, sealed, inspected); err != nil {
			response.Outcome, response.Error = transport.SiteSettingsIncludeRefused, fmt.Sprintf("write %s: %v", path, err)
			return response
		}
		written = []plannedSiteFileWrite{{domain: request.Domain, available: path, sealed: sealed, inspected: inspected}}
		response.Outcome = transport.SiteSettingsIncludeWritten
	}
	if takeOver != nil {
		if err := writeManagedSiteFile(takeOver.available, takeOver.sealed, takeOver.inspected); err != nil {
			rollbackErr := restorePlannedSiteFiles(written)
			response.Outcome = transport.SiteSettingsIncludeRefused
			response.Restored = rollbackErr == nil
			response.TookOverFrom = ""
			response.Error = fmt.Sprintf("write %s without its upload line: %v", filepath.Base(takeOver.available), err)
			return response
		}
		written = append(written, *takeOver)
	}
	if err := ng.ValidateNginx(); err != nil {
		rollbackErr := restorePlannedSiteFiles(written)
		response.Outcome = transport.SiteSettingsIncludeRefused
		response.Detail = SiteFileCheckLine(err)
		response.Restored = rollbackErr == nil
		response.Error = fmt.Sprintf("nginx refused %s, the previous file was put back", SiteSettingsFileName)
		if rollbackErr != nil {
			response.Error = fmt.Sprintf("nginx refused %s and putting the previous file back failed: %v", SiteSettingsFileName, rollbackErr)
		}
		return response
	}
	if err := ng.ReloadNginx(); err != nil {
		rollbackErr := restorePlannedSiteFiles(written)
		response.Outcome = transport.SiteSettingsIncludeRefused
		response.Restored = rollbackErr == nil
		response.Detail = boundedSiteFileDetail(err.Error())
		response.Error = fmt.Sprintf("nginx did not reload with %s, the previous file was put back", SiteSettingsFileName)
		if rollbackErr == nil {
			_ = ng.ReloadNginx()
		}
		return response
	}
	return response
}

// appUploadLines are the two lines an application's include carries for its
// upload size (appcatalog.App.RenderNginx).
func withoutAppUploadLines(body string) (string, bool) {
	lines := strings.SplitAfter(body, "\n")
	out := make([]string, 0, len(lines))
	removed := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "client_max_body_size ") || strings.HasPrefix(trimmed, "# Uploads up to ") {
			removed = true
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, ""), removed
}

// appIncludeTakeOver plans the rewrite of an application's include without
// its upload line: only for CelikPanel's unchanged text of an app-<id>.conf
// in the site's own directory; nil keeps the file as it is. The caller holds
// nginxMutationMu.
func appIncludeTakeOver(domain, path string) *plannedSiteFileWrite {
	if filepath.Dir(path) != PanelManagedDir(domain) || !appIncludeName.MatchString(filepath.Base(path)) {
		return nil
	}
	inspected := inspectSiteFile(path, nil, "")
	if inspected.state != transport.SiteFileManagedUnchanged {
		return nil
	}
	_, body, ok := splitManagedHeader(inspected.content)
	if !ok {
		return nil
	}
	stripped, removed := withoutAppUploadLines(string(body))
	if !removed {
		return nil
	}
	return &plannedSiteFileWrite{domain: domain, available: path, sealed: []byte(SealManagedText(stripped)), inspected: inspected}
}

// siteSettingsOwnsUpload reports whether the site's settings file sets the
// upload size, so an application's include is written without its own line.
// The caller holds nginxMutationMu.
func siteSettingsOwnsUpload(domain string) bool {
	path, err := SiteSettingsIncludePath(domain)
	if err != nil {
		return false
	}
	content, err := readManagedConfig(path)
	if err != nil {
		return false
	}
	_, ok := nginxDirectiveValue(content, "client_max_body_size")
	return ok
}
