package services

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Website traffic measurement (alpha.118, D-044).
//
// nginx writes a second, small access log per website for CelikPanel; the
// owner's own logs (/var/log/nginx/<domain>-access.log) are not touched. Two
// plain nginx files make it, both CelikPanel's own with the render header
// (D-031), written only while absent or still CelikPanel's unchanged text:
//
//   - /etc/nginx/conf.d/celikpanel-traffic.conf (http context): the log
//     format "celikpanel_traffic" (time, bytes sent to the visitor, bytes
//     received) and a map that leaves certificate validation requests out;
//   - /etc/nginx/celikpanel-managed.d/<domain>/access-log.traffic (server
//     context, through the vhost's `include .../*.traffic;` line in the
//     blocks that log to the site's own access log, so a block that only
//     redirects to HTTPS keeps logging where it did): the access_log line
//     of /var/log/celikpanel/traffic/<domain id>.log. While
//     the site shows a CelikPanel page (suspended, maintenance, traffic limit)
//     it holds only a comment, so those pages are not counted.
//
// The logs are rotated by the operating system's logrotate
// (/etc/logrotate.d/celikpanel-traffic, written by the Agent), so they stay
// small with or without CelikPanel. Without the Panel nothing reads them and
// nothing is limited; the sites keep working (D-022).
//
// Web sitesi trafiği: nginx her site için CelikPanel'e ikinci, küçük bir
// erişim günlüğü yazar; sahibin günlüklerine dokunulmaz. Günlükler işletim
// sisteminin logrotate'i ile döndürülür.

const (
	trafficHTTPConfName = "celikpanel-traffic.conf"
	// TrafficIncludeFileName is the site's traffic log setting in its
	// Panel-owned include directory.
	TrafficIncludeFileName = "access-log.traffic"
	// TrafficLogFormat is the log format's name.
	TrafficLogFormat = "celikpanel_traffic"
	// trafficCountedVariable leaves ACME HTTP-01 validation out.
	trafficCountedVariable = "$celikpanel_traffic_counted"
)

// trafficLogDirectory is where nginx writes the traffic logs (a seam: tests
// and the development mode use a directory under nginxDir).
var trafficLogDirectory = "/var/log/celikpanel/traffic"

// TrafficLogDir is the directory of the traffic logs.
func TrafficLogDir() string {
	if nginxDevMode() {
		return filepath.Join(nginxDir, "celikpanel-traffic-logs")
	}
	return trafficLogDirectory
}

// TrafficLogPath is one website's traffic log.
func TrafficLogPath(domainID int) string {
	return filepath.Join(TrafficLogDir(), strconv.Itoa(domainID)+".log")
}

// TrafficHTTPConfPath is the http-context file with the log format.
func TrafficHTTPConfPath() string {
	return filepath.Join(nginxBase(), "conf.d", trafficHTTPConfName)
}

// TrafficIncludePath is one site's traffic log setting.
func TrafficIncludePath(domain string) string {
	return PanelManagedDir(domain) + "/" + TrafficIncludeFileName
}

// RenderTrafficHTTPConf is the body of TrafficHTTPConfPath.
func RenderTrafficHTTPConf() string {
	return "# CelikPanel's traffic measurement: the log format of the small per-site\n" +
		"# traffic logs in " + TrafficLogDir() + ". Plain nginx configuration;\n" +
		"# your own logs are not changed. CelikPanel writes this file only while it\n" +
		"# is CelikPanel's unchanged text. Without it the sites keep working and\n" +
		"# their traffic is shown as unknown.\n" +
		"# CelikPanel trafik ölçümü: site başına küçük trafik günlüklerinin biçimi.\n" +
		"log_format " + TrafficLogFormat + " '$msec $bytes_sent $request_length';\n" +
		"map $uri " + trafficCountedVariable + " {\n" +
		"    ~^/\\.well-known/acme-challenge/ 0;\n" +
		"    default 1;\n" +
		"}\n"
}

// RenderTrafficInclude is the body of TrafficIncludePath. counted=false
// (the site shows a CelikPanel page) writes no log line.
func RenderTrafficInclude(domain string, domainID int, counted bool) string {
	head := "# The traffic log of " + domain + " for CelikPanel's monthly traffic limit.\n" +
		"# This directory is CelikPanel's; put your own additions in " + OwnerIncludeDir(domain) + "/*.conf.\n" +
		"# Trafik ölçümü; bu dizin CelikPanel'indir.\n"
	if !counted {
		return head + "# The site shows a CelikPanel page now; that page is not counted.\n"
	}
	return head + "access_log " + TrafficLogPath(domainID) + " " + TrafficLogFormat +
		" buffer=64k flush=1m if=" + trafficCountedVariable + ";\n"
}

// TrafficLogItem asks ApplyManagedVhosts to keep a site's traffic log
// setting (written with the vhost, validated by the same nginx test).
type TrafficLogItem struct {
	DomainID int
	// Counted: the site serves its content (not a CelikPanel page).
	Counted bool
}

// Traffic log outcomes in SiteFileResult.TrafficLog besides the challenge
// words (written, unchanged, kept, failed).
const (
	// TrafficLogFormatKept: the log format file is not CelikPanel's
	// unchanged text, so the site's setting is written without a log line.
	TrafficLogFormatKept = "format_kept"
)

type plannedTrafficFile struct {
	path      string
	sealed    []byte
	inspected siteFileInspection
}

// managedPartState classifies one of CelikPanel's own small files: absent,
// CelikPanel's unchanged text (the same or other text), or kept.
func managedPartState(path string, sealed []byte) (siteFileInspection, string) {
	inspected := inspectSiteFile(path, nil, "")
	switch inspected.state {
	case transport.SiteFileAbsent:
		return inspected, transport.SiteFileChallengeAbsent
	case transport.SiteFileManagedUnchanged:
		if bytes.Equal(inspected.content, sealed) {
			return inspected, transport.SiteFileChallengeUnchanged
		}
		return inspected, transport.SiteFileChallengeDiffers
	}
	return inspected, transport.SiteFileChallengeKept
}

// TrafficHTTPConfState is the log format file's state, read only.
func TrafficHTTPConfState() string {
	_, state := managedPartState(TrafficHTTPConfPath(), []byte(SealManagedText(RenderTrafficHTTPConf())))
	return state
}

// TrafficIncludeState is a site's traffic setting's state, read only:
// "counting" (CelikPanel's text with a log line), "not_counting", "absent"
// or "kept".
func TrafficIncludeState(domain string, domainID int) string {
	path := TrafficIncludePath(domain)
	inspected := inspectSiteFile(path, nil, "")
	switch inspected.state {
	case transport.SiteFileAbsent:
		return "absent"
	case transport.SiteFileManagedUnchanged:
		if bytes.Equal(inspected.content, []byte(SealManagedText(RenderTrafficInclude(domain, domainID, true)))) {
			return "counting"
		}
		return "not_counting"
	}
	return "kept"
}

// planTrafficFormat writes the log format file when absent or CelikPanel's
// other text. ok=false: it is kept (the owner's), so no site may name the
// format.
func planTrafficFormat() (*plannedTrafficFile, bool, error) {
	sealed := []byte(SealManagedText(RenderTrafficHTTPConf()))
	path := TrafficHTTPConfPath()
	inspected, state := managedPartState(path, sealed)
	switch state {
	case transport.SiteFileChallengeUnchanged:
		return nil, true, nil
	case transport.SiteFileChallengeKept:
		return nil, false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false, err
	}
	if err := writeManagedSiteFile(path, sealed, inspected); err != nil {
		return nil, false, err
	}
	return &plannedTrafficFile{path: path, sealed: sealed, inspected: inspected}, true, nil
}

// ensureTrafficLogDir makes the log directory (0755: nginx's workers reopen
// the logs after a rotation). nginx -t opens every log, so it must exist
// before a site names its log.
func ensureTrafficLogDir() error {
	dir := TrafficLogDir()
	if info, err := os.Lstat(dir); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s exists and is not a directory; it was left alone", dir)
		}
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.Chmod(dir, 0o755)
}

// planTrafficInclude writes one site's setting when absent or CelikPanel's
// other text; a kept file is left alone.
func planTrafficInclude(domain string, item TrafficLogItem, formatOK bool) (*plannedTrafficFile, string, error) {
	counted := item.Counted && formatOK
	if counted {
		if err := ensureTrafficLogDir(); err != nil {
			counted = false
		}
	}
	sealed := []byte(SealManagedText(RenderTrafficInclude(domain, item.DomainID, counted)))
	path := TrafficIncludePath(domain)
	inspected, state := managedPartState(path, sealed)
	outcome := state
	switch state {
	case transport.SiteFileChallengeUnchanged:
		if !formatOK {
			outcome = TrafficLogFormatKept
		}
		return nil, outcome, nil
	case transport.SiteFileChallengeKept:
		return nil, state, nil
	}
	if err := ensurePanelManagedDir(domain); err != nil {
		return nil, transport.SiteFileChallengeFailed, err
	}
	if err := writeManagedSiteFile(path, sealed, inspected); err != nil {
		return nil, transport.SiteFileChallengeFailed, err
	}
	outcome = transport.SiteFileChallengeWritten
	if !formatOK {
		outcome = TrafficLogFormatKept
	}
	return &plannedTrafficFile{path: path, sealed: sealed, inspected: inspected}, outcome, nil
}

func restorePlannedTrafficFiles(files []*plannedTrafficFile) error {
	var errs []error
	for index := len(files) - 1; index >= 0; index-- {
		file := files[index]
		if file.inspected.exists {
			if err := atomicWriteSiteFile(file.path, file.inspected.content, file.inspected.mode, file.inspected.ownership); err != nil {
				errs = append(errs, err)
			}
		} else if info, err := os.Lstat(file.path); err == nil && info.Mode().IsRegular() {
			if err := os.Remove(file.path); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// removeTrafficInclude removes a deleted site's traffic setting when it is
// still CelikPanel's unchanged text (called before the directory itself).
func removeTrafficInclude(domain string) {
	path := TrafficIncludePath(domain)
	if inspected := inspectSiteFile(path, nil, ""); inspected.state == transport.SiteFileManagedUnchanged {
		_ = os.Remove(path)
	}
}

// TrafficIncludeLine is the line a vhost needs to read the traffic log.
func TrafficIncludeLine(domain string) string {
	return "include " + PanelManagedDir(domain) + "/*.traffic;"
}

// SiteReadsTrafficInclude reports whether the site's vhost file has the
// line that reads its traffic log setting (read only). A missing or
// unreadable file reads as false.
func SiteReadsTrafficInclude(domain string) bool {
	available, _ := vhostPaths(domain)
	content, err := readManagedConfig(available)
	if err != nil {
		return false
	}
	return fileHasDirective(content, TrafficIncludeLine(domain))
}

// ReconcileManagedFile writes CelikPanel's own small file (render header,
// D-031) when it is absent or still CelikPanel's other text, and leaves any
// other file alone. It answers "written", "unchanged" or "kept".
func ReconcileManagedFile(path, body string) (string, error) {
	sealed := []byte(SealManagedText(body))
	inspected, state := managedPartState(path, sealed)
	switch state {
	case transport.SiteFileChallengeUnchanged:
		return transport.SiteFileChallengeUnchanged, nil
	case transport.SiteFileChallengeKept:
		return transport.SiteFileChallengeKept, nil
	}
	if err := writeManagedSiteFile(path, sealed, inspected); err != nil {
		return transport.SiteFileChallengeFailed, err
	}
	return transport.SiteFileChallengeWritten, nil
}
