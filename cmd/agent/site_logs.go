package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/hostname"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Site log retention (alpha.128, PLESK-GAP G27; transport/site_logs.go). The
// distribution's logrotate rule for /var/log/nginx/*.log stays the only rule
// for these files. This removes, for one site at a time, the rotated files
// (<domain>-access.log.1, .2.gz, -20261107.gz ...) whose last change is older
// than the site's limit. It never removes the live log, never follows a link,
// and never touches another site's file or a file of another name.
//
// Site günlükleri: yalnız sitenin döndürülmüş, sınırdan eski dosyaları silinir.

// Seams (tests replace them).
var (
	siteLogDir         = "/var/log/nginx"
	siteLogRotateRules = []string{"/etc/logrotate.d/nginx", "/etc/logrotate.d/nginx-common"}
	siteLogNow         = time.Now
)

const maxSiteLogRetentionDays = 3650

// siteLogRotatedPattern matches only the rotated files of one site.
func siteLogRotatedPattern(domain string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(domain) + `-(access|error)\.log[.-][0-9]{1,8}(\.(gz|xz|zst|bz2))?$`)
}

func siteLogLiveName(domain, name string) bool {
	return name == domain+"-access.log" || name == domain+"-error.log"
}

// SiteLogRetentionV1 removes the rotated files older than each site's limit.
func (a *Agent) SiteLogRetentionV1(req *transport.SiteLogRetentionRequest, resp *transport.SiteLogRetentionResponse) error {
	if req == nil {
		resp.Error = "a request is required"
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "applying site log retention"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	resp.Removed = map[string]int{}
	entries, err := os.ReadDir(siteLogDir)
	if err != nil {
		resp.Error = "read " + siteLogDir + ": " + err.Error()
		return nil
	}
	var problems []string
	for _, item := range req.Items {
		domain, err := hostname.CanonicalFQDN(item.Domain)
		if err != nil || item.KeepDays < 1 || item.KeepDays > maxSiteLogRetentionDays {
			problems = append(problems, "skipped an invalid item")
			continue
		}
		pattern := siteLogRotatedPattern(domain)
		cutoff := siteLogNow().Add(-time.Duration(item.KeepDays) * 24 * time.Hour)
		for _, entry := range entries {
			if !pattern.MatchString(entry.Name()) {
				continue
			}
			path := filepath.Join(siteLogDir, entry.Name())
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || !info.ModTime().Before(cutoff) {
				continue
			}
			if err := os.Remove(path); err != nil {
				problems = append(problems, entry.Name()+": "+err.Error())
				continue
			}
			resp.Removed[domain]++
		}
	}
	if len(problems) > 0 {
		resp.Error = boundedText(strings.Join(problems, "; "), 300)
	}
	return nil
}

// SiteLogStatusV1 lists the site's log files and the system's rule (reads only).
func (a *Agent) SiteLogStatusV1(req *transport.SiteLogStatusRequest, resp *transport.SiteLogStatusResponse) error {
	resp.Files = []transport.SiteLogFile{}
	resp.System = readSiteLogSystemRule()
	if req == nil {
		resp.Error = "a request is required"
		return nil
	}
	domain, err := hostname.CanonicalFQDN(req.Domain)
	if err != nil {
		resp.Error = "a valid domain is required"
		return nil
	}
	entries, err := os.ReadDir(siteLogDir)
	if err != nil {
		resp.Error = "read " + siteLogDir + ": " + err.Error()
		return nil
	}
	pattern := siteLogRotatedPattern(domain)
	for _, entry := range entries {
		live := siteLogLiveName(domain, entry.Name())
		if !live && !pattern.MatchString(entry.Name()) {
			continue
		}
		info, err := os.Lstat(filepath.Join(siteLogDir, entry.Name()))
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		resp.Files = append(resp.Files, transport.SiteLogFile{Name: entry.Name(), Bytes: info.Size(),
			ModTime: info.ModTime().UTC().Format(time.RFC3339), Live: live})
	}
	sort.Slice(resp.Files, func(i, j int) bool { return resp.Files[i].ModTime > resp.Files[j].ModTime })
	return nil
}

// readSiteLogSystemRule reads the distribution's logrotate rule for nginx:
// how often it rotates and how many files it keeps (the block that names
// /var/log/nginx/*.log).
func readSiteLogSystemRule() transport.SiteLogSystemRule {
	for _, path := range siteLogRotateRules {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		rule := transport.SiteLogSystemRule{Path: path}
		inBlock := false
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "#") || line == "" {
				continue
			}
			if !inBlock {
				if strings.Contains(line, "/nginx/") && strings.HasSuffix(line, "{") {
					inBlock = true
				}
				continue
			}
			if line == "}" {
				break
			}
			fields := strings.Fields(line)
			switch fields[0] {
			case "daily", "weekly", "monthly", "yearly":
				rule.Frequency = fields[0]
			case "rotate":
				if len(fields) > 1 {
					rule.Rotate, _ = strconv.Atoi(fields[1])
				}
			}
		}
		f.Close()
		if inBlock {
			rule.Found = true
			return rule
		}
	}
	return transport.SiteLogSystemRule{}
}
