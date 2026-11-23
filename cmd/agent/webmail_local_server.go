package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Webmail's local server (alpha.145, security): the earlier forms of its
// nginx file gave out every file of the Roundcube tree and ran every .php in
// it to any account of this server (renderWebmailNginxConfig says why). At
// the Agent's first start after the update a file in one of CelikPanel's
// earlier forms gets the current one through applyWebmailNginxMutation (the
// configuration test, one reload, the previous file put back on any
// failure; a failure never stops the start). A file the owner changed is
// kept: CelikPanel's current text is put beside it as
// <file>.celikpanel-pending (nginx reads only *.conf) and Problems now names
// it, as for webmail's address files.
//
// Web postasının yerel sunucusu artık yalnız CelikPanel'in sorduğunu yanıtlar;
// önceki biçim açılışta yenilenir, sahibin dosyası korunur.

// webmailLocalServerHost is the "host" under which the local server's file
// is listed among the address forms (it has no name).
const webmailLocalServerHost = ""

// webmailLocalServerParts reads the listen socket, Roundcube root and PHP-FPM
// socket a webmail server file names, with today's defaults for what it does
// not name (so CelikPanel's text can be put beside an owner's file).
func webmailLocalServerParts(content string) (listen, root, fpm string, err error) {
	if match := webmailListenLine.FindStringSubmatch(content); match != nil {
		listen = match[1]
	} else if listen, err = webmailSocketPathForNginx(); err != nil {
		return "", "", "", err
	}
	root = roundcubeRootDir()
	if match := webmailAliasLine.FindStringSubmatch(content); match != nil {
		root = match[1]
	}
	if fpm = parseWebmailFPMSocket(content); fpm == "" {
		if fpm, err = phpToolSocket(phpToolWebmail, phpToolStateFor(phpToolWebmail).Socket); err != nil {
			return "", "", "", err
		}
	}
	return listen, root, fpm, nil
}

// webmailLocalServerCurrent: the file is CelikPanel's current form.
func webmailLocalServerCurrent(content string) bool {
	listen := webmailListenLine.FindStringSubmatch(content)
	alias := webmailAliasLine.FindStringSubmatch(content)
	if listen == nil || alias == nil || webmailLoopbackListenLine.MatchString(content) {
		return false
	}
	return content == renderWebmailNginxConfig(listen[1], alias[1], parseWebmailFPMSocket(content))
}

// reconcileWebmailLocalServerAtStartup brings the local server's file in an
// earlier CelikPanel form to the current one, or puts CelikPanel's text
// beside an owner's file. It answers what it did, for the journal.
func reconcileWebmailLocalServerAtStartup(ctx context.Context, run webmailCommandRunner) []string {
	path := phpToolWebmailConfPath
	pending := path + webmailAddressPendingSuffix
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Webmail is not served here (any more): CelikPanel's text beside a
		// file that is gone is not needed (review of PR #357 fixes, finding 4).
		removeWebmailLocalServerPending()
		return nil
	}
	if err != nil {
		return []string{fmt.Sprintf("%s could not be read: %v", path, err)}
	}
	content := string(data)
	if strings.TrimSpace(content) == "" {
		return nil
	}
	if managedWebmailConfig(content) {
		if webmailLocalServerCurrent(content) {
			removeRoundcubePendingText(pending)
			return nil
		}
		next, err := currentFormPHPToolConfig(phpToolWebmail, content)
		if err == nil {
			err = ensurePHPToolCheckScript()
		}
		if err == nil {
			err = applyWebmailNginxMutation(ctx, path, []byte(next), true, run)
		}
		if err != nil {
			noteWebmailAddressRewriteRefused(webmailLocalServerHost, err)
			// Only another file nginx's test named is the owner's to correct;
			// CelikPanel's own file, a reload, the PHP check script or the
			// start's time limit are said without a file (review of PR #357
			// fixes, findings 1 and 6: the journal and a restart of the Agent).
			webmailAddressRewriteRefusalsMu.Lock()
			if webmailAddressRewriteRefusals[webmailLocalServerHost] == path {
				webmailAddressRewriteRefusals[webmailLocalServerHost] = ""
			}
			webmailAddressRewriteRefusalsMu.Unlock()
			return []string{fmt.Sprintf("kept in its earlier form, which gives out Roundcube's files to every account of this server (%v); CelikPanel tries again at its next start", err)}
		}
		webmailAddressRewriteRefusalsMu.Lock()
		delete(webmailAddressRewriteRefusals, webmailLocalServerHost)
		webmailAddressRewriteRefusalsMu.Unlock()
		removeRoundcubePendingText(pending)
		return []string{"rewritten in the current form: it answers only CelikPanel's own checks"}
	}
	if strings.Contains(content, webmailLocalServerMarker) {
		// The owner took CelikPanel's text (and added to it).
		removeRoundcubePendingText(pending)
		return nil
	}
	listen, root, fpm, err := webmailLocalServerParts(content)
	if err != nil {
		return []string{fmt.Sprintf("%s was changed by the owner and kept; CelikPanel's text could not be prepared beside it: %v", path, err)}
	}
	text := renderWebmailNginxConfig(listen, root, fpm)
	if existing, err := os.ReadFile(pending); err == nil && string(existing) == text {
		return nil
	}
	if err := secureWriteConfig(pending, []byte(text), 0o600); err != nil {
		return []string{fmt.Sprintf("%s was changed by the owner and kept; CelikPanel's text could not be put beside it: %v", path, err)}
	}
	return []string{fmt.Sprintf("%s was changed by the owner and kept as it is; CelikPanel's current text, which answers only CelikPanel's own checks, is in %s", path, pending)}
}

// webmailLocalServerForm lists, read only, the local server's file when it
// still serves Roundcube's tree as alpha.144 and earlier did: "test_failed"
// (CelikPanel's earlier text whose rewrite was put back; File is the file
// nginx's test named, "" when none) or "owner_edited" (File is CelikPanel's
// text beside it when it lies there, otherwise the file itself).
func webmailLocalServerForm() (transport.WebmailAddressForm, bool) {
	path := phpToolWebmailConfPath
	data, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return transport.WebmailAddressForm{}, false
	}
	content := string(data)
	if managedWebmailConfig(content) {
		if webmailLocalServerCurrent(content) {
			return transport.WebmailAddressForm{}, false
		}
		webmailAddressRewriteRefusalsMu.Lock()
		file := webmailAddressRewriteRefusals[webmailLocalServerHost]
		webmailAddressRewriteRefusalsMu.Unlock()
		return transport.WebmailAddressForm{Host: webmailLocalServerHost, State: "test_failed", File: file}, true
	}
	if strings.Contains(content, webmailLocalServerMarker) {
		return transport.WebmailAddressForm{}, false
	}
	file := path + webmailAddressPendingSuffix
	if info, err := os.Lstat(file); err != nil || !info.Mode().IsRegular() {
		file = path
	}
	return transport.WebmailAddressForm{Host: webmailLocalServerHost, State: "owner_edited", File: file}, true
}

// removeWebmailLocalServerPending removes, by name, CelikPanel's text beside
// the local server's file (a regular file only): when webmail goes, and when
// the file it was for is gone.
func removeWebmailLocalServerPending() {
	removeRoundcubePendingText(phpToolWebmailConfPath + webmailAddressPendingSuffix)
}
