package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Webmail sends again (alpha.145, alpha.144 lab item D1). From the first
// release CelikPanel wrote Roundcube's configuration with
// smtp_host 'localhost:587', while its own submission service on 587 takes a
// login only on an encrypted connection (smtpd_tls_security_level=encrypt,
// mail_submission_rpc.go). Roundcube 1.6 starts STARTTLS only for a host
// written with the tls:// prefix (program/lib/Roundcube/rcube_smtp.php,
// connect(): $use_tls = $scheme === 'tls'; it then calls Net_SMTP::auth with
// its own STARTTLS switched off), so it logged in without encryption, found
// no AUTH in the service's answer and every message failed with
// "authentication failed".
//
// Mail is now handed to the same service with STARTTLS and the mailbox's own
// login, on 127.0.0.1: the connection never leaves this server. PHP checks a
// certificate's chain and name by default, and the name it would check is
// 127.0.0.1, which no mail certificate carries; the service presents the mail
// host's certificate or, before one exists, the distribution's or
// CelikPanel's self-signed one, and sending must work in both states. So for
// 127.0.0.1 ONLY, the certificate's chain and name are not checked: Roundcube
// applies smtp_conn_options given per host name only to that host
// (rcube_utils::parse_socket_options), and any other host keeps PHP's full
// check. A peer name equal to the certificate's name would keep the check but
// cannot work in both states (the self-signed certificate's chain never
// verifies, and its name is the machine's, not always the mail host's).
//
// Web postası yeniden gönderebilir: posta bu sunucunun gönderim hizmetine
// 127.0.0.1 üzerinden şifreli ve kutunun kendi girişiyle verilir; bağlantı
// sunucudan çıkmadığı için yalnız 127.0.0.1'de sertifikanın adı ve zinciri
// denetlenmez. Başka her ad PHP'nin tam denetimini korur.

// roundcubeSMTPSettings are CelikPanel's sending lines of Roundcube's
// configuration (the reason is written into the file with them).
const roundcubeSMTPSettings = `// Mail goes to this server's own submission service with STARTTLS and the
// mailbox's own login. The connection is to 127.0.0.1 and never leaves this
// server; the service may present the mail host's certificate or, before one
// exists, a self-signed one, so for 127.0.0.1 only the certificate's name and
// chain are not checked. Any other host keeps the full check.
$config['smtp_host'] = 'tls://127.0.0.1:587';
$config['smtp_user'] = '%u';
$config['smtp_pass'] = '%p';
$config['smtp_conn_options'] = ['127.0.0.1' => ['ssl' => ['verify_peer' => false, 'verify_peer_name' => false]]];
`

// legacyRenderRoundcubeConfig is the text alpha.1 to alpha.144 wrote (still
// CelikPanel's own: corrected at the Agent's first start after the update).
func legacyRenderRoundcubeConfig(desKey string) string {
	dbPath := filepath.Join(webmailBaseDir, "db", "roundcube.sqlite3")
	return fmt.Sprintf(`<?php
// Managed by CelikPanel. Roundcube installed from the official tarball (D-004:
// one path on every Linux). SQLite store, localhost IMAP/SMTP.
$config = [];
$config['db_dsnw'] = 'sqlite:///%s?mode=0664';
$config['imap_host'] = 'localhost:143';
$config['smtp_host'] = 'localhost:587';
$config['smtp_user'] = '%%u';
$config['smtp_pass'] = '%%p';
$config['support_url'] = '';
$config['des_key'] = '%s';
$config['product_name'] = 'CelikPanel Webmail';
$config['plugins'] = ['archive', 'zipdownload'];
$config['skin'] = 'elastic';
$config['enable_installer'] = false;
`, dbPath, desKey)
}

func roundcubeConfigPath() string { return filepath.Join(webmailBaseDir, "config", "config.inc.php") }

// roundcubeConfigPendingSuffix names CelikPanel's text put beside a
// configuration the owner changed (Roundcube reads only config.inc.php).
const roundcubeConfigPendingSuffix = ".celikpanel-pending"

// roundcubeConfigForm is one of CelikPanel's texts of the configuration.
type roundcubeConfigForm struct {
	desKey      string
	withAddress bool
	// current: the text is already the current one (not the earlier one).
	current bool
}

func (form roundcubeConfigForm) render() string {
	text := renderRoundcubeConfig(form.desKey)
	if form.withAddress {
		text += roundcubeAddressSettings
	}
	return text
}

// parseRoundcubeConfigForm recognises CelikPanel's texts, current or earlier,
// with or without the address lines. Anything else is the owner's (desKey is
// still set when a des_key line in CelikPanel's form is in it).
func parseRoundcubeConfigForm(content string) (roundcubeConfigForm, bool) {
	match := roundcubeDesKeyLine.FindStringSubmatch(content)
	if match == nil {
		return roundcubeConfigForm{}, false
	}
	key := match[1]
	for _, base := range []struct {
		text    string
		current bool
	}{{renderRoundcubeConfig(key), true}, {legacyRenderRoundcubeConfig(key), false}} {
		switch content {
		case base.text:
			return roundcubeConfigForm{desKey: key, current: base.current}, true
		case base.text + roundcubeAddressSettings:
			return roundcubeConfigForm{desKey: key, withAddress: true, current: base.current}, true
		}
	}
	return roundcubeConfigForm{desKey: key}, false
}

// roundcubeSMTPHostLine is an uncommented smtp_host assignment;
// roundcubeConnOptionsLine an uncommented smtp_conn_options assignment.
var (
	roundcubeSMTPHostLine    = regexp.MustCompile(`(?m)^[ \t]*\$config\[\s*['"]smtp_host['"]\s*\][ \t]*=[ \t]*(.*?)[ \t]*$`)
	roundcubeConnOptionsLine = regexp.MustCompile(`(?m)^[ \t]*\$config\[\s*['"]smtp_conn_options['"]\s*\][ \t]*=`)
	roundcubeQuotedValue     = regexp.MustCompile(`^(?:'([^'\\]*)'|"([^"\\$]*)")[ \t]*;[ \t]*(?:(?://|#).*)?$`)
	roundcubeLoopbackHosts   = map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true}
)

// roundcubeCannotSend reads, from a configuration's text, whether webmail
// cannot send through this server's submission service because of it
// (review of PR #357, findings 1 and 7). The last uncommented smtp_host
// assignment decides; with none, Roundcube's default 'localhost:587'.
//   - A loopback host (localhost, 127.0.0.1, [::1]) without tls:// or ssl://,
//     on 587 or no port: the service refuses a login without encryption.
//   - A loopback host WITH tls:// or ssl://, on any port, while the file has
//     no uncommented smtp_conn_options assignment at all: PHP then checks the
//     certificate against the loopback name, which no certificate of this
//     server carries. Any smtp_conn_options assignment is the owner's own
//     setting (its PHP array is not read): nothing is said.
//
// known is false (nothing is said) for a value that is not one quoted string
// (an array, an expression, another file), for a file that mentions the
// legacy key smtp_server, and for an smtp_host that appears only on a line
// that is not a whole-line comment and not an assignment. CRLF line endings
// are read as LF. Known gap: an assignment inside a /* … */ block comment is
// read as active.
func roundcubeCannotSend(content string) (cannot, known bool) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if strings.Contains(content, "smtp_server") {
		return false, false
	}
	matches := roundcubeSMTPHostLine.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") {
				continue
			}
			if strings.Contains(line, "smtp_host") {
				return false, false
			}
		}
		return true, true
	}
	value := roundcubeQuotedValue.FindStringSubmatch(matches[len(matches)-1][1])
	if value == nil {
		return false, false
	}
	host := strings.ToLower(strings.TrimSpace(value[1] + value[2]))
	encrypted := false
	for _, scheme := range []string{"tls://", "ssl://"} {
		if rest, ok := strings.CutPrefix(host, scheme); ok {
			host, encrypted = rest, true
		}
	}
	if strings.Contains(host, "://") {
		return false, true
	}
	name, port := host, ""
	switch {
	case strings.HasPrefix(host, "["):
		end := strings.Index(host, "]")
		if end < 0 {
			return false, true
		}
		name, port = host[1:end], strings.TrimPrefix(host[end+1:], ":")
	case strings.Count(host, ":") == 1:
		name, port, _ = strings.Cut(host, ":")
	}
	if !roundcubeLoopbackHosts[name] {
		return false, true
	}
	if !encrypted {
		return port == "" || port == "587", true
	}
	return !roundcubeConnOptionsLine.MatchString(content), true
}

// reconcileRoundcubeConfigAtStartup brings CelikPanel's earlier text of
// Roundcube's configuration to the current one once after an update (the
// same write as the address lines: mode and owner kept). A configuration the
// owner changed is kept as it is; when it still sends without encryption,
// CelikPanel's current text (with its des_key) is put beside it as
// config.inc.php.celikpanel-pending, and Problems now names it. It answers
// what it did, for the journal.
func reconcileRoundcubeConfigAtStartup() []string {
	path := roundcubeConfigPath()
	pending := path + roundcubeConfigPendingSuffix
	snapshot, err := snapshotMailFile(path)
	if err != nil {
		return []string{fmt.Sprintf("could not be read: %v", err)}
	}
	if !snapshot.exists {
		return nil
	}
	content := string(snapshot.content)
	form, managed := parseRoundcubeConfigForm(content)
	if managed {
		if form.current {
			// Also an owner who took CelikPanel's text: it is no longer needed beside it.
			removeRoundcubePendingText(pending)
			return nil
		}
		if err := writeRoundcubeConfigKeeping(snapshot, form.render()); err != nil {
			return []string{fmt.Sprintf("kept in its earlier form, in which webmail cannot send (%v); CelikPanel tries again at its next start", err)}
		}
		removeRoundcubePendingText(pending)
		return []string{"rewritten in the current form: webmail sends through this server's submission service with encryption"}
	}
	if cannot, known := roundcubeCannotSend(content); !known || !cannot {
		// The owner's own sending setting: nothing of CelikPanel's is needed.
		removeRoundcubePendingText(pending)
		return nil
	}
	if form.desKey == "" {
		return []string{fmt.Sprintf("%s was changed by the owner and kept; webmail cannot send with it; CelikPanel's text is not put beside it because the file has no des_key line of CelikPanel's form", path)}
	}
	text := renderRoundcubeConfig(form.desKey)
	if strings.Contains(content, roundcubeAddressSettings) {
		text += roundcubeAddressSettings
	}
	if existing, err := os.ReadFile(pending); err == nil && string(existing) == text {
		return nil
	}
	// The text holds the des_key: readable by root only.
	if err := secureWriteConfig(pending, []byte(text), 0o600); err != nil {
		return []string{fmt.Sprintf("%s was changed by the owner and kept; webmail cannot send with it; CelikPanel's text could not be put beside it: %v", path, err)}
	}
	return []string{fmt.Sprintf("%s was changed by the owner and kept as it is; webmail cannot send with it; CelikPanel's current text is in %s", path, pending)}
}

// removeRoundcubePendingText removes CelikPanel's own text beside the
// configuration once it is no longer needed (only a regular file).
func removeRoundcubePendingText(pending string) {
	if info, err := os.Lstat(pending); err == nil && info.Mode().IsRegular() {
		if err := os.Remove(pending); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("[webmail] configuration: %s could not be removed: %v", pending, err)
		}
	}
}

// roundcubeSendingForm reads, read only, whether webmail cannot send because
// of its configuration: "cannot_send" with the file to name (CelikPanel's
// text beside it when it lies there, otherwise the configuration itself), or
// "" when it can or when it is not known. reason "rewrite_failed" (review of
// PR #357, finding 2): the file is still CelikPanel's own earlier text, whose
// rewrite at the Agent's start did not happen; the Agent rewrites it at its
// next start, so the owner must not edit it by hand.
func roundcubeSendingForm() (state, reason, file string) {
	path := roundcubeConfigPath()
	content, err := os.ReadFile(path)
	if err != nil {
		return "", "", ""
	}
	if form, managed := parseRoundcubeConfigForm(string(content)); managed {
		if form.current {
			return "", "", ""
		}
		return "cannot_send", "rewrite_failed", path
	}
	if cannot, known := roundcubeCannotSend(string(content)); !known || !cannot {
		return "", "", ""
	}
	pending := path + roundcubeConfigPendingSuffix
	if info, err := os.Lstat(pending); err == nil && info.Mode().IsRegular() {
		return "cannot_send", "", pending
	}
	return "cannot_send", "", path
}
