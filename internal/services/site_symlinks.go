package services

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Links in a site's folder (20 Nov 2026, tenant isolation). The vhost writes
// `disable_symlinks if_not_owner;` in every server block that serves the
// site's files (the "symlinks" template), unless the owner already set
// disable_symlinks for the site in server context himself: in his include
// directory (D-031) or in the administrator's additional directives. nginx
// refuses the directive twice in one block ("is duplicate"), and the owner's
// rule is his to keep (D-022), so the Panel's line is then left out.
//
// Sitenin klasöründeki bağlantılar: sahip site için disable_symlinks'i
// kendisi koyduysa Panel'in satırı yazılmaz; onunki korunur.

// maxOwnerIncludeScanBytes bounds what is read of one owner include file.
const maxOwnerIncludeScanBytes = 1 << 20

// OwnerSetsSymlinks says whether the owner set disable_symlinks in server
// context for the site: in a *.conf file of its owner include directory, in a
// site setting file of CelikPanel's that he edited, or at the top level of the
// administrator's directives (adminDirectives; "" when there are none or they
// are a customer's). A file that cannot be read or
// parsed is read line by line: any disable_symlinks line then counts, on the
// side of never making nginx refuse the owner's configuration.
func OwnerSetsSymlinks(domain, adminDirectives string) bool {
	if strings.TrimSpace(adminDirectives) != "" && serverLevelSymlinks(adminDirectives) {
		return true
	}
	// The site's web server setting files the owner edited are his too
	// (CelikPanel keeps them); CelikPanel's own text never sets the rule.
	for _, name := range []string{SiteSettingsPartName, SiteDirectivesPartName} {
		inspected := inspectSiteFile(SitePartPath(domain, name), nil, "")
		if siteFileKeptState(inspected.state) && serverLevelSymlinks(string(inspected.content)) {
			return true
		}
	}
	matches, err := filepath.Glob(filepath.Join(OwnerIncludeDir(domain), "*.conf"))
	if err != nil {
		return false
	}
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		content, err := readBounded(path, maxOwnerIncludeScanBytes)
		if err != nil {
			continue
		}
		if serverLevelSymlinks(string(content)) {
			return true
		}
	}
	return false
}

// serverLevelSymlinks: the text sets disable_symlinks at its top level. Text
// the directive parser does not take is scanned line by line instead.
func serverLevelSymlinks(text string) bool {
	directives, err := parseSiteDirectives(text)
	if err != nil {
		for _, line := range strings.Split(text, "\n") {
			fields := strings.Fields(normalizedDirectiveLine(line))
			if len(fields) > 0 && strings.EqualFold(strings.TrimSuffix(fields[0], ";"), "disable_symlinks") {
				return true
			}
		}
		return false
	}
	for _, d := range directives {
		if strings.EqualFold(d.name, "disable_symlinks") {
			return true
		}
	}
	return false
}

func readBounded(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, limit))
}
