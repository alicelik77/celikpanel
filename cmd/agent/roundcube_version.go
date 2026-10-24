package main

import (
	"os"
	"path/filepath"
	"regexp"
)

// roundcubeVersionDefine is Roundcube's own version line in
// program/include/iniset.php: define('RCMAIL_VERSION', '1.6.15');
var roundcubeVersionDefine = regexp.MustCompile(`define\(\s*'RCMAIL_VERSION'\s*,\s*'([0-9]+\.[0-9]+(?:\.[0-9]+)?(?:-[a-z0-9]+)?)'\s*\)`)

// roundcubeInstalledVersion is the version the installed tree says it is
// (alpha.107, D-036: a version shown is the tree's own answer), or "".
func roundcubeInstalledVersion() string {
	return roundcubeTreeVersion(webmailBaseDir)
}

func roundcubeTreeVersion(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "program", "include", "iniset.php"))
	if err != nil || len(data) > 1<<20 {
		return ""
	}
	if match := roundcubeVersionDefine.FindSubmatch(data); match != nil {
		return string(match[1])
	}
	return ""
}
