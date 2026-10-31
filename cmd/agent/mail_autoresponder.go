package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Out-of-office reply per mailbox (alpha.114, Plesk parity; D-022, D-024).
//
// The reply is Dovecot Pigeonhole's Sieve `vacation`, executed by Dovecot's
// own LMTP delivery. Nothing of CelikPanel runs when mail arrives, so the
// reply keeps working with the Panel stopped or removed. Three native pieces
// make it work, each one an ordinary file the owner can read and change:
//
//  1. /etc/dovecot/conf.d/96-celikpanel-sieve.conf: Dovecot's LMTP listener
//     inside Postfix's spool (private/dovecot-lmtp) and the Sieve plugin for
//     LMTP, with the standard personal script location (~/sieve,
//     active ~/.dovecot.sieve). First line: the SHA-256 of the rest, so an
//     administrator's edit is detected and kept (never overwritten).
//  2. /etc/postfix/celikpanel_transport (transport_maps): only the mailboxes
//     with a reply are delivered through Dovecot LMTP; every other mailbox
//     keeps Postfix's virtual(8) delivery exactly as before. A transport_maps
//     the administrator set to another table is kept; the reply is refused
//     with guidance instead.
//  3. The mailbox's own Sieve script <maildir>/sieve/celikpanel-autoresponder.sieve
//     and the active link <maildir>/.dovecot.sieve. The start and end dates
//     are Sieve `currentdate` tests, so the reply starts and stops by itself.
//     An active script that is not CelikPanel's (for example one a mail
//     client wrote through ManageSieve), or CelikPanel's script edited by
//     hand, is the mailbox owner's: it is reported, never replaced.
//
// Posta kutusu başına "ofis dışında" yanıtı: Dovecot'un kendi LMTP
// teslimindeki Sieve `vacation`. Panel durduğunda da çalışır. Üç yerel
// parça: Dovecot Sieve eki (özetli; yöneticinin düzenlemesi korunur),
// yalnız yanıtı olan kutuları LMTP'ye veren Postfix transport tablosu ve
// kutunun kendi Sieve betiği. Sahibin betiği asla değiştirilmez.

const (
	autoresponderScriptName   = "celikpanel-autoresponder.sieve"
	autoresponderCompiledName = "celikpanel-autoresponder.svbin"
	autoresponderLinkName     = ".dovecot.sieve"
	autoresponderLinkTarget   = "sieve/" + autoresponderScriptName
	autoresponderScriptHeader = "# Managed by CelikPanel: out-of-office reply. sha256="
	autoresponderDovecotHead  = "# Managed by CelikPanel: Dovecot Sieve for out-of-office replies. sha256="
	autoresponderLMTPSocket   = "/var/spool/postfix/private/dovecot-lmtp"
	autoresponderLMTPNexthop  = "lmtp:unix:private/dovecot-lmtp"
)

// Seams: production values, replaced by component tests.
var (
	autoresponderDovecotConfPath = dovecotConfDirPath + "/96-celikpanel-sieve.conf"
	autoresponderTransportPath   = "/etc/postfix/celikpanel_transport"
	autoresponderSievePlugin     = []string{
		"/usr/lib/dovecot/modules/lib90_sieve_plugin.so",
		"/usr/lib64/dovecot/modules/lib90_sieve_plugin.so",
	}
	autoresponderLMTPBinary = []string{"/usr/lib/dovecot/lmtp", "/usr/libexec/dovecot/lmtp"}
	autoresponderFileExists = func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}
	autoresponderPkgFamily       = detectPkgFamily
	autoresponderInstallPackages = func(ctx context.Context, family string, packages []string) error {
		_, err := installPackagesContext(ctx, family, packages)
		return err
	}
	autoresponderDovecotIs24     = dovecotIs24
	autoresponderApplyDovecotCfg = applyDovecotConf
	autoresponderReloadDovecot   = func(ctx context.Context) (string, error) {
		return applyDovecotVerified(mailServiceLeaseRunner(ctx), mailServiceReload)
	}
	autoresponderReloadPostfix = func(ctx context.Context) (string, error) {
		return applyPostfixVerified(mailServiceLeaseRunner(ctx), mailServiceReload)
	}
	autoresponderPostconfGet = func(ctx context.Context, key string) (string, error) {
		out, err := runMailTLSMutationCommand(ctx, "postconf", "-h", key)
		return strings.TrimSpace(string(out)), err
	}
	autoresponderPostconfSet = func(ctx context.Context, key, value string) error {
		out, err := runMailTLSMutationCommand(ctx, "postconf", "-e", key+"="+value)
		if err != nil {
			return fmt.Errorf("postconf %s: %s", key, firstLine(string(out)))
		}
		return nil
	}
	autoresponderMapType     = postfixMapTypeContext
	autoresponderSocketWait  = 10 * time.Second
	autoresponderSocketReady = func() bool {
		info, err := os.Stat(autoresponderLMTPSocket)
		return err == nil && info.Mode()&os.ModeSocket != 0
	}
	autoresponderSleep = time.Sleep
	autoresponderStore = mailboxSieveStore(nativeMailboxSieveStore{})
)

// mailboxSieveState is what one mailbox's directory holds.
type mailboxSieveState struct {
	// LinkKind: "none", "link" (a symbolic link) or "file" (a regular
	// .dovecot.sieve, which is always the owner's).
	LinkKind      string
	LinkTarget    string
	ScriptExists  bool
	ScriptContent []byte
}

// mailboxSieveStore reads and writes the mailbox's Sieve files as the mail
// user, refusing every symbolic link on the way (nativeMailboxSieveStore).
type mailboxSieveStore interface {
	Read(domain, local string) (mailboxSieveState, error)
	Write(domain, local string, script []byte) error
	Remove(domain, local string) error
}

// renderAutoresponderSieve renders the mailbox's script. The first line
// carries the SHA-256 of everything after it (classifyMailboxSieve).
func renderAutoresponderSieve(setting transport.MailAutoresponder) string {
	var body strings.Builder
	body.WriteString("# CelikPanel writes this script only while it is exactly as CelikPanel wrote\n")
	body.WriteString("# it. If you change it, CelikPanel keeps your version and says so.\n")
	body.WriteString("# Dovecot runs it when mail arrives, with or without CelikPanel.\n")
	requires := []string{`"vacation"`}
	var tests []string
	if setting.StartDate != "" {
		tests = append(tests, `currentdate :value "ge" "date" `+sieveQuote(setting.StartDate))
	}
	if setting.EndDate != "" {
		tests = append(tests, `currentdate :value "le" "date" `+sieveQuote(setting.EndDate))
	}
	if len(tests) > 0 {
		requires = append(requires, `"date"`, `"relational"`)
	}
	body.WriteString("require [" + strings.Join(requires, ", ") + "];\n")
	action := fmt.Sprintf("vacation :days %d :subject %s", setting.IntervalDays, sieveQuote(setting.Subject))
	if len(setting.Addresses) > 0 {
		quoted := make([]string, 0, len(setting.Addresses))
		for _, address := range setting.Addresses {
			quoted = append(quoted, sieveQuote(address))
		}
		action += " :addresses [" + strings.Join(quoted, ", ") + "]"
	}
	action += " " + sieveQuote(setting.Message) + ";\n"
	switch len(tests) {
	case 0:
		body.WriteString(action)
	case 1:
		body.WriteString("if " + tests[0] + " {\n  " + action + "}\n")
	default:
		body.WriteString("if allof (" + strings.Join(tests, ", ") + ") {\n  " + action + "}\n")
	}
	text := body.String()
	sum := sha256.Sum256([]byte(text))
	return autoresponderScriptHeader + hex.EncodeToString(sum[:]) + "\n" + text
}

// sieveQuote is an RFC 5228 quoted string: backslash and double quote are
// escaped; line breaks are allowed inside it.
func sieveQuote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

// digestHeaderIntact reports whether content starts with header + the
// SHA-256 of everything after the first line.
func digestHeaderIntact(content []byte, header string) bool {
	text := string(content)
	newline := strings.IndexByte(text, '\n')
	if newline < 0 || !strings.HasPrefix(text, header) {
		return false
	}
	sum := sha256.Sum256([]byte(text[newline+1:]))
	return strings.TrimPrefix(text[:newline], header) == hex.EncodeToString(sum[:])
}

// classifyMailboxSieve decides whose the mailbox's Sieve state is.
func classifyMailboxSieve(state mailboxSieveState) string {
	switch state.LinkKind {
	case "file":
		return transport.AutoresponderScriptOwner
	case "link":
		if state.LinkTarget != autoresponderLinkTarget {
			return transport.AutoresponderScriptOwner
		}
		if !state.ScriptExists {
			// A dangling link to CelikPanel's own name: nothing answers.
			return transport.AutoresponderScriptNone
		}
		if digestHeaderIntact(state.ScriptContent, autoresponderScriptHeader) {
			return transport.AutoresponderScriptManaged
		}
		return transport.AutoresponderScriptOwner
	default:
		if state.ScriptExists && !digestHeaderIntact(state.ScriptContent, autoresponderScriptHeader) {
			// CelikPanel's file name, changed by hand, not active: still
			// the owner's text, never overwritten.
			return transport.AutoresponderScriptOwner
		}
		return transport.AutoresponderScriptNone
	}
}

// renderAutoresponderDovecotConf renders the Sieve drop-in for the dialect.
func renderAutoresponderDovecotConf(is24 bool) string {
	listener := `service lmtp {
  unix_listener ` + autoresponderLMTPSocket + ` {
    mode = 0600
    user = postfix
    group = postfix
  }
}
`
	preamble := `# CelikPanel writes this file only while it is exactly as CelikPanel wrote it.
# Once you edit it, CelikPanel keeps your version and tells you so.
# It lets Postfix deliver the mailboxes that have an out-of-office reply
# through Dovecot LMTP, where the Sieve plugin sends the reply. Dovecot runs
# from this file with or without CelikPanel.
`
	var body string
	if is24 {
		body = preamble + `protocol lmtp {
  mail_plugins {
    sieve = yes
  }
}
` + listener + `sieve_script personal {
  driver = file
  path = ~/sieve
  active_path = ~/.dovecot.sieve
}
`
	} else {
		body = preamble + `protocol lmtp {
  mail_plugins = $mail_plugins sieve
}
` + listener + `plugin {
  sieve = file:~/sieve;active=~/.dovecot.sieve
}
`
	}
	sum := sha256.Sum256([]byte(body))
	return autoresponderDovecotHead + hex.EncodeToString(sum[:]) + "\n" + body
}

// autoresponderSievePackages are the official packages that provide the
// Sieve plugin and the LMTP server on each package family.
func autoresponderSievePackages(family string) ([]string, bool) {
	switch family {
	case "apt":
		return []string{"dovecot-sieve", "dovecot-lmtpd"}, true
	case "pacman":
		// Arch's dovecot package already contains LMTP.
		return []string{"pigeonhole"}, true
	default:
		return nil, false
	}
}

func autoresponderAnyExists(paths []string) bool {
	for _, path := range paths {
		if autoresponderFileExists(path) {
			return true
		}
	}
	return false
}

func autoresponderSieveAvailable() bool {
	return autoresponderAnyExists(autoresponderSievePlugin) && autoresponderAnyExists(autoresponderLMTPBinary)
}

// autoresponderTransportValue is transport_maps naming CelikPanel's table.
func autoresponderTransportValue(ctx context.Context) string {
	return autoresponderMapType(ctx) + ":" + autoresponderTransportPath
}

// transportMapsIncludesOurs reports whether a transport_maps value already
// names CelikPanel's table (alone or in the administrator's own list).
func transportMapsIncludesOurs(value string) bool {
	for _, field := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		if index := strings.IndexByte(field, ':'); index > 0 && field[index+1:] == autoresponderTransportPath {
			return true
		}
	}
	return false
}

type autoresponderRefusal struct {
	code, detail string
}

func (r *autoresponderRefusal) Error() string { return r.detail }

func refuseAutoresponder(code, detail string) error {
	return &autoresponderRefusal{code: code, detail: detail}
}

// Owner-facing D-024 texts: what was found, that nothing changed, who acts.
const (
	autoresponderOwnerScriptDetail    = "This mailbox already has its own mail filter (Sieve) script, made outside CelikPanel (for example in a mail program). CelikPanel did not change it. The mailbox owner adds the out-of-office reply to that script, or removes the script so CelikPanel can write its own."
	autoresponderOwnerDovecotDetail   = "/etc/dovecot/conf.d/96-celikpanel-sieve.conf was changed outside CelikPanel, so CelikPanel left it as it is and changed nothing. The server administrator either keeps it (it must enable Sieve for LMTP and the private/dovecot-lmtp listener) or moves it aside, for example `sudo mv /etc/dovecot/conf.d/96-celikpanel-sieve.conf /root/`, and saves the reply again."
	autoresponderOwnerTransportDetail = "Postfix's transport_maps is set to a table that is not CelikPanel's, so CelikPanel changed nothing. The server administrator adds `%s` to transport_maps (`sudo postconf -e 'transport_maps = <yours>, %s'`) and saves the reply again."
)

// ensureAutoresponderInfrastructure installs the Sieve package when it is
// missing, writes the Dovecot drop-in (validated by doveconf, rolled back on
// refusal), reloads Dovecot and waits for the LMTP socket. It returns a
// warning sentence when Dovecot is stopped (the reply works once it starts).
func ensureAutoresponderInfrastructure(ctx context.Context) (string, error) {
	if !autoresponderSieveAvailable() {
		family := autoresponderPkgFamily()
		packages, ok := autoresponderSievePackages(family)
		if !ok {
			return "", refuseAutoresponder(transport.AutoresponderRefusedSievePackage,
				"CelikPanel cannot install Dovecot's Sieve plugin on this operating system. Nothing was changed.")
		}
		if err := autoresponderInstallPackages(ctx, family, packages); err != nil {
			return "", refuseAutoresponder(transport.AutoresponderRefusedSievePackage,
				fmt.Sprintf("The Dovecot Sieve package (%s) could not be installed: %s. Nothing else was changed; save the reply again when the package manager works (another update may be running).",
					strings.Join(packages, ", "), boundedNativeDetail(err.Error())))
		}
		if !autoresponderSieveAvailable() {
			return "", refuseAutoresponder(transport.AutoresponderRefusedSievePackage,
				"The Dovecot Sieve package was installed, but its plugin or Dovecot's LMTP server is still missing. Nothing else was changed.")
		}
	}
	is24, err := autoresponderDovecotIs24()
	if err != nil {
		return "", refuseAutoresponder(transport.AutoresponderRefusedDovecotRejected,
			"The installed Dovecot version could not be read, so CelikPanel changed nothing: "+boundedNativeDetail(err.Error()))
	}
	want := renderAutoresponderDovecotConf(is24)
	snapshot, err := snapshotMailFile(autoresponderDovecotConfPath)
	if err != nil {
		return "", err
	}
	changed := false
	switch {
	case snapshot.exists && string(snapshot.content) == want:
	case snapshot.exists && !digestHeaderIntact(snapshot.content, autoresponderDovecotHead):
		return "", refuseAutoresponder(transport.AutoresponderRefusedOwnerDovecotConf, autoresponderOwnerDovecotDetail)
	default:
		if err := autoresponderApplyDovecotCfg(autoresponderDovecotConfPath, want); err != nil {
			return "", refuseAutoresponder(transport.AutoresponderRefusedDovecotRejected,
				"Dovecot refused the Sieve settings, so the previous configuration was kept and no reply was set: "+boundedNativeDetail(err.Error()))
		}
		changed = true
	}
	warning := ""
	if changed || !autoresponderSocketReady() {
		state, err := autoresponderReloadDovecot(ctx)
		if err != nil {
			return "", refuseAutoresponder(transport.AutoresponderRefusedDovecotRejected,
				"Dovecot did not load the Sieve settings ("+boundedNativeDetail(err.Error())+"). No mailbox was moved to the new delivery. The server administrator reads `journalctl -u dovecot`.")
		}
		if state == mailServiceNotRunning {
			warning = "Dovecot is stopped. The reply starts working when Dovecot is started."
			return warning, nil
		}
	}
	deadline := time.Now().Add(autoresponderSocketWait)
	for !autoresponderSocketReady() {
		if time.Now().After(deadline) {
			return "", refuseAutoresponder(transport.AutoresponderRefusedDovecotRejected,
				"Dovecot runs, but its LMTP listener "+autoresponderLMTPSocket+" did not appear, so no mailbox was moved to Dovecot delivery and mail keeps arriving as before. The server administrator checks that `protocols` includes lmtp (`doveconf protocols`).")
		}
		autoresponderSleep(200 * time.Millisecond)
	}
	return warning, nil
}

// setAutoresponderTransport adds or removes one mailbox's LMTP route. It
// sets transport_maps only when it is empty or already CelikPanel's.
func setAutoresponderTransport(ctx context.Context, email string, enabled bool) (string, error) {
	current, err := readMailFile(autoresponderTransportPath)
	if err != nil {
		return "", err
	}
	var next []byte
	if enabled {
		next = upsertMailLine(current, email, email+" "+autoresponderLMTPNexthop)
	} else {
		var removed bool
		next, removed = removeMailLine(current, email)
		if !removed {
			return "", nil
		}
	}
	value, err := autoresponderPostconfGet(ctx, "transport_maps")
	if err != nil {
		return "", fmt.Errorf("read Postfix transport_maps: %w", err)
	}
	setMaps := false
	switch {
	case transportMapsIncludesOurs(value):
	case strings.TrimSpace(value) == "":
		setMaps = enabled
	default:
		if enabled {
			ours := autoresponderTransportValue(ctx)
			return "", refuseAutoresponder(transport.AutoresponderRefusedOwnerTransport,
				fmt.Sprintf(autoresponderOwnerTransportDetail, ours, ours))
		}
	}
	if err := applyMailFileMutation(ctx, []mailFileWrite{
		{path: autoresponderTransportPath, content: next, mode: 0o644},
	}, []string{autoresponderTransportPath}, nil); err != nil {
		return "", err
	}
	if !setMaps {
		return "", nil
	}
	if err := autoresponderPostconfSet(ctx, "transport_maps", autoresponderTransportValue(ctx)); err != nil {
		// The table is written but unused: mail keeps its old delivery.
		return "", err
	}
	if _, err := autoresponderReloadPostfix(ctx); err != nil {
		return "Postfix did not confirm the reload (" + boundedNativeDetail(err.Error()) + "); the reply starts with Postfix's next reload (`sudo systemctl reload postfix`).", nil
	}
	return "", nil
}

// mailboxExistsInVBox reports whether the mailbox is one Postfix delivers.
func mailboxExistsInVBox(email string) (bool, error) {
	content, err := readMailFile(postfixVBoxPath)
	if err != nil {
		return false, err
	}
	return mailFileHasKey(content, email), nil
}

func fillAutoresponderRefusal(resp *transport.MailAutoresponderResponse, err error) error {
	var refusal *autoresponderRefusal
	if errors.As(err, &refusal) {
		resp.Applied = false
		resp.Refusal = refusal.code
		resp.Detail = refusal.detail
		return nil
	}
	return err
}

// SetMailAutoresponder brings one mailbox's out-of-office reply to the
// requested setting. A refusal is an answer (Refusal + Detail), not an RPC
// error, so the page can say who acts.
func (a *Agent) SetMailAutoresponder(req *transport.SetMailAutoresponderRequest, resp *transport.MailAutoresponderResponse) error {
	if req == nil || resp == nil {
		return fmt.Errorf("autoresponder request and response are required")
	}
	*resp = transport.MailAutoresponderResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "setting an out-of-office reply"); err != nil {
		return err
	}
	email, local, domain, err := canonicalAgentMailbox(req.Email)
	if err != nil {
		return err
	}
	setting, err := transport.NormalizeMailAutoresponder(req.Autoresponder, email)
	if err != nil {
		return err
	}
	mailMutex.Lock()
	defer mailMutex.Unlock()
	ctx, cancel := newMailMutationContext()
	defer cancel()
	return fillAutoresponderRefusal(resp, applyMailAutoresponder(ctx, email, local, domain, setting, resp))
}

func applyMailAutoresponder(ctx context.Context, email, local, domain string, setting transport.MailAutoresponder, resp *transport.MailAutoresponderResponse) error {
	if !dovecotInstalledForRepair() || !autoresponderFileExists("/etc/postfix") {
		return refuseAutoresponder(transport.AutoresponderRefusedMailNotInstalled,
			"Mail is not set up on this server (Postfix and Dovecot), so there is nothing to answer mail. Install the mail service first.")
	}
	exists, err := mailboxExistsInVBox(email)
	if err != nil {
		return err
	}
	if !exists && setting.Enabled {
		return refuseAutoresponder(transport.AutoresponderRefusedNoMailbox,
			"This mailbox is not in the server's mail configuration. Save the mailbox again, then the reply.")
	}
	state, err := autoresponderStore.Read(domain, local)
	if err != nil {
		return fmt.Errorf("read the mailbox's Sieve script: %w", err)
	}
	resp.Script = classifyMailboxSieve(state)
	if resp.Script == transport.AutoresponderScriptOwner {
		return refuseAutoresponder(transport.AutoresponderRefusedOwnerScript, autoresponderOwnerScriptDetail)
	}
	if !setting.Enabled {
		// Delivery first: once the route is gone no script runs.
		warning, err := setAutoresponderTransport(ctx, email, false)
		if err != nil {
			return err
		}
		if resp.Script == transport.AutoresponderScriptManaged || state.ScriptExists || state.LinkKind == "link" {
			if err := autoresponderStore.Remove(domain, local); err != nil {
				return fmt.Errorf("remove CelikPanel's Sieve script: %w", err)
			}
		}
		resp.Script = transport.AutoresponderScriptNone
		resp.SieveAvailable = autoresponderSieveAvailable()
		resp.Warning = warning
		resp.Applied = true
		return nil
	}
	warning, err := ensureAutoresponderInfrastructure(ctx)
	if err != nil {
		return err
	}
	resp.SieveAvailable = true
	if err := autoresponderStore.Write(domain, local, []byte(renderAutoresponderSieve(setting))); err != nil {
		return fmt.Errorf("write the mailbox's Sieve script: %w", err)
	}
	transportWarning, err := setAutoresponderTransport(ctx, email, true)
	if err != nil {
		// Without the route the script is never run; leave no half state.
		if removeErr := autoresponderStore.Remove(domain, local); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("remove the new Sieve script: %w", removeErr))
		}
		return err
	}
	resp.Script = transport.AutoresponderScriptManaged
	resp.Warning = joinWarning(warning, transportWarning)
	resp.Applied = true
	return nil
}

// MailAutoresponderState reads one mailbox's Sieve state (read only).
func (a *Agent) MailAutoresponderState(req *transport.MailAutoresponderStateRequest, resp *transport.MailAutoresponderResponse) error {
	if req == nil || resp == nil {
		return fmt.Errorf("autoresponder state request and response are required")
	}
	*resp = transport.MailAutoresponderResponse{}
	_, local, domain, err := canonicalAgentMailbox(req.Email)
	if err != nil {
		return err
	}
	resp.SieveAvailable = autoresponderSieveAvailable()
	state, err := autoresponderStore.Read(domain, local)
	if err != nil {
		return fmt.Errorf("read the mailbox's Sieve script: %w", err)
	}
	resp.Script = classifyMailboxSieve(state)
	if snapshot, err := snapshotMailFile(autoresponderDovecotConfPath); err == nil && snapshot.exists &&
		!digestHeaderIntact(snapshot.content, autoresponderDovecotHead) {
		resp.Refusal = transport.AutoresponderRefusedOwnerDovecotConf
		resp.Detail = autoresponderOwnerDovecotDetail
	}
	if resp.Script == transport.AutoresponderScriptOwner {
		resp.Refusal = transport.AutoresponderRefusedOwnerScript
		resp.Detail = autoresponderOwnerScriptDetail
	}
	return nil
}
