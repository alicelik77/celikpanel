package fileaccess

import (
	"sort"
	"strings"
)

// The official distribution package of every command-line tool of the jail
// (alpha.118, owner: "if something must be installed for SSH, make sure it
// is installed before SSH is turned on").
//
// The mapping was read from the packages themselves, not guessed: dpkg -S on
// Debian 13 and the .deb contents of zip, unzip, rsync, less, nano, vim-tiny
// and openssh-client (Ubuntu 24.04 ships the same package names; its
// vim-tiny also installs /usr/bin/vi through update-alternatives), and
// pacman -Qo plus the package archives on Arch. On Arch /usr/bin/vi is a
// small bash script in ex-vi-compat that runs /usr/bin/vim, so the jail
// carries vim beside vi there (JailCompanions).
//
// Jail araçlarının resmi dağıtım paketleri. Eşleme paketlerin içeriğinden
// okundu; tahmin değildir.

// JailCommandPackages maps a package-manager family to the package that
// provides each command of JailCommands.
var JailCommandPackages = map[string]map[string]string{
	"apt":    jailPackages("dash", "mawk", "vim-tiny", "ncurses-bin", "openssh-client"),
	"pacman": jailPackages("bash", "gawk", "ex-vi-compat", "ncurses", "openssh"),
}

func jailPackages(sh, awk, vi, clear, scp string) map[string]string {
	packages := map[string]string{
		"bash": "bash", "sh": sh, "awk": awk, "vi": vi, "clear": clear, "scp": scp,
		"less": "less", "grep": "grep", "sed": "sed", "find": "findutils", "xargs": "findutils",
		"diff": "diffutils", "tar": "tar", "gzip": "gzip", "gunzip": "gzip", "zcat": "gzip",
		"unzip": "unzip", "zip": "zip", "nano": "nano", "rsync": "rsync",
	}
	for _, command := range JailCommands {
		if _, ok := packages[command]; !ok {
			packages[command] = "coreutils"
		}
	}
	return packages
}

// JailInstallPackages are the packages install.sh brings with its small
// prerequisites, so a fresh server has every jail tool: the ones a minimal
// image of the family may lack. The rest (coreutils, bash, grep, sed, the
// awk, findutils, tar, gzip, ncurses) belong to every installation of the
// family; scp comes with the OpenSSH server.
var JailInstallPackages = map[string][]string{
	"apt":    {"less", "zip", "unzip", "nano", "vim-tiny", "rsync"},
	"pacman": {"less", "zip", "unzip", "nano", "ex-vi-compat", "rsync", "diffutils"},
}

// JailCompanions are programs a jail command runs and that are copied with
// it when the host has them, without being reported missing on their own.
var JailCompanions = map[string][]string{
	"vi": {"vim"},
}

// JailPackagesFor returns the sorted, distinct packages that provide the
// missing commands on this family, and the commands it has no package for
// (an unknown family names every command).
func JailPackagesFor(family string, missing []string) (packages []string, unknown []string) {
	table := JailCommandPackages[family]
	seen := map[string]bool{}
	for _, command := range missing {
		pkg, ok := table[command]
		if !ok {
			unknown = append(unknown, command)
			continue
		}
		if !seen[pkg] {
			seen[pkg] = true
			packages = append(packages, pkg)
		}
	}
	sort.Strings(packages)
	return packages, unknown
}

// JailToolsQualifier is the stable record of one install of jail packages
// (the sorted names, comma separated): the durable job and its step carry
// it, so a job taken for one set cannot install another.
func JailToolsQualifier(packages []string) string {
	sorted := append([]string(nil), packages...)
	sort.Strings(sorted)
	return strings.Join(sorted, ",")
}

// IsJailPackage reports whether name is a package of the family's mapping.
func IsJailPackage(family, name string) bool {
	for _, pkg := range JailCommandPackages[family] {
		if pkg == name {
			return true
		}
	}
	return false
}
