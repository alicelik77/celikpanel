package main

import (
	"bufio"
	"bytes"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// Is a replaced lineage's renewal file still exactly what certbot wrote for
// CelikPanel's own request? (alpha.134 lab, run 6: a replaced lineage whose
// renewal file the owner had edited, `renew_before_expiry = 20 days`, was
// removed.) Removal cannot be taken back, so this rule is narrower than the
// page's "changed outside CelikPanel" (nativeSiteConfOwnerChanged, which keeps
// warning only about what changes how it renews): every line must be one that
// certbot writes for `certbot certonly --webroot -w <root>
// --preferred-challenges http --cert-name <l> [--server <s>]` (the Agent's
// command line, buildNativeSiteIssueArgs), with a value that request can
// produce. Anything else keeps the lineage.
//
// What certbot writes (certbot/_internal/storage.py, renewal.py; measured on
// Debian 13 certbot 4.0, the fixture testRenewalConf): at the top `version`,
// `archive_dir`, `cert`, `privkey`, `chain`, `fullchain` and the comment
// `# renew_before_expiry = …`; in [renewalparams] `account`, `authenticator`,
// `webroot_path`, `server`, `key_type`, `pref_challs` (and `installer = None`
// on older certbot); one [[webroot_map]] line per name.
//
// `autorenew` is accepted (True or False): alpha.113–133's own pass wrote
// `autorenew = False` into every replaced lineage, and the page's switch
// writes it too. An owner's `autorenew = False` reads the same and cannot be
// told apart; such a lineage is kept only when something else differs.
//
// A key certbot stored because of a global certbot setting (a cli.ini) is
// written the same into the domain's active lineage, which certbot wrote for
// CelikPanel's newest request: the same key with the same value there is
// accepted. A hook never is: CelikPanel's own deploy hook lives in certbot's
// renewal-hooks/deploy directory, never in a renewal file.
//
// Yenileme dosyası yalnız certbot'un CelikPanel isteği için yazdığını mı
// taşıyor? Başka her satır, kanca ya da değer lineage'ı korur.

var (
	certbotAccountID   = regexp.MustCompile(`^[0-9a-f]{32}$`)
	certbotHookKey     = regexp.MustCompile(`(^|_)hook$|hooks?$`)
	certbotConfVersion = regexp.MustCompile(`^[0-9][0-9A-Za-z.+-]{0,31}$`)
	certbotComment     = regexp.MustCompile(`^# (?:renew_before_expiry = [0-9]+ days?|Options used in the renewal process)$`)
)

type renewalLine struct {
	section, key, value string
}

// parseRenewalLines reads a renewal file line by line: section headers,
// `key = value` pairs, comments and blank lines. A line that is none of these
// is an error.
func parseRenewalLines(content []byte) ([]renewalLine, error) {
	var lines []renewalLine
	section := ""
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 0, 4096), 64<<10)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		switch {
		case text == "":
			continue
		case strings.HasPrefix(text, "#"):
			// certbot writes two comments; any other is an edit.
			if !certbotComment.MatchString(text) {
				return nil, fmt.Errorf("a comment certbot does not write: %q", boundedNativeDetail(text))
			}
			continue
		case strings.HasPrefix(text, "["):
			section = strings.ToLower(strings.Trim(text, "[] "))
			continue
		}
		key, value, ok := strings.Cut(text, "=")
		if !ok {
			return nil, fmt.Errorf("a line is not a setting: %q", boundedNativeDetail(text))
		}
		lines = append(lines, renewalLine{section: section, key: strings.ToLower(strings.TrimSpace(key)), value: strings.Trim(strings.TrimSpace(value), `"'`)})
	}
	return lines, scanner.Err()
}

// nativeSiteConfOnlyCertbots answers "" when every line of the replaced
// lineage's renewal file is one certbot writes for CelikPanel's request, else
// the first line that is not (for the log). reference is the domain's active
// lineage's renewal file (nil when there is none).
func nativeSiteConfOnlyCertbots(content, reference []byte, lineage, expectWebroot string, servers ...string) string {
	lines, err := parseRenewalLines(content)
	if err != nil {
		return err.Error()
	}
	inReference := map[renewalLine]bool{}
	if reference != nil {
		if referenceLines, err := parseRenewalLines(reference); err == nil {
			for _, line := range referenceLines {
				inReference[line] = true
			}
		}
	}
	livePath := func(store, file string) []string {
		out := []string{}
		for _, dir := range []string{store, "/etc/letsencrypt"} {
			out = append(out, filepath.ToSlash(filepath.Join(dir, "live", lineage, file)), path.Join(dir, "live", lineage, file))
		}
		return out
	}
	oneOf := func(value string, allowed ...string) bool {
		for _, candidate := range allowed {
			if value == candidate {
				return true
			}
		}
		return false
	}
	store := filepath.ToSlash(nativeSiteConfigDir)
	for _, line := range lines {
		ok := false
		switch line.section + "." + line.key {
		case ".version":
			ok = certbotConfVersion.MatchString(line.value)
		case ".archive_dir":
			ok = oneOf(line.value, path.Join(store, "archive", lineage), path.Join("/etc/letsencrypt", "archive", lineage))
		case ".cert", ".privkey", ".chain", ".fullchain":
			ok = oneOf(line.value, livePath(store, line.key+".pem")...)
		case "renewalparams.account":
			ok = certbotAccountID.MatchString(line.value)
		case "renewalparams.authenticator":
			ok = line.value == "webroot"
		case "renewalparams.webroot_path":
			ok = true
			for _, part := range strings.Split(line.value, ",") {
				if part = strings.TrimSpace(part); part != "" && filepath.Clean(part) != expectWebroot {
					ok = false
				}
			}
		case "renewalparams.server":
			for _, server := range servers {
				if server != "" && strings.EqualFold(strings.TrimRight(line.value, "/"), strings.TrimRight(server, "/")) {
					ok = true
				}
			}
		case "renewalparams.key_type":
			ok = oneOf(line.value, "ecdsa", "rsa")
		case "renewalparams.pref_challs":
			ok = oneOf(line.value, "http-01,", "http-01")
		case "renewalparams.installer":
			ok = line.value == "None"
		case "renewalparams.autorenew":
			ok = oneOf(strings.ToLower(line.value), "true", "false")
		default:
			if line.section == "webroot_map" {
				ok = filepath.Clean(line.value) == expectWebroot
			} else if line.section == "renewalparams" && !certbotHookKey.MatchString(line.key) {
				// Stored the same for CelikPanel's newest request: a global
				// certbot setting, not an edit of this file.
				ok = inReference[line]
			}
		}
		if !ok {
			where := line.section
			if where == "" {
				where = "top"
			}
			return fmt.Sprintf("%s: %s = %s", where, line.key, boundedNativeDetail(line.value))
		}
	}
	return ""
}
