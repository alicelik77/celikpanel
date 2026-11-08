package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
	gossh "golang.org/x/crypto/ssh"
)

// SSH password sign-in (alpha.130, owner approval of 8 Nov 2026).
//
// Both servers had password sign-in on although the owner believed it off:
// cloud-init's /etc/ssh/sshd_config.d/50-cloud-init.conf says
// `PasswordAuthentication yes`, and sshd uses the FIRST value it reads, so
// that file beat 60-cloudimg-settings.conf's `no`.
//
// Turning it off writes one file, /etc/ssh/sshd_config.d/00-celikpanel.conf,
// which sorts before the other drop-ins. Before writing, the configuration is
// read the way sshd reads it (Include in place, globs in lexical order, first
// value wins) with the drop-in added, and the change is refused when the
// drop-in would not decide (an Include after sshd_config's own setting, an
// owner's drop-in sorting earlier, a Match block). After writing: `sshd -t`
// must accept it, `sshd -T` must say both password methods are off, and only
// then is ssh/sshd reloaded (never restarted: open sessions stay). Any failure
// removes the drop-in again. Turning it back on removes only that drop-in,
// and only when its text is exactly CelikPanel's.
//
// Lock-out guard: refused unless at least one account can sign in with a key
// sshd would accept: root (when PermitRootLogin lets root in with a key) or a
// member of sudo/wheel/admin, with a login shell, allowed by AllowUsers/
// DenyUsers/AllowGroups/DenyGroups, whose authorized_keys file passes sshd's
// StrictModes ownership rules and holds a parsable key.
//
// Without CelikPanel: delete the drop-in and run `systemctl reload ssh`
// (Arch: sshd). Nothing here is a CelikPanel service (D-022).
//
// SSH parola ile giriş: kapatma tek bir ek dosya yazar; sshd onu ilk okur.
// Yazmadan önce sshd'nin okuma sırası taklit edilir; sonra sshd -t, sshd -T
// ve yeniden yükleme (yeniden başlatma değil). Her hatada ek dosya kaldırılır.
// Anahtarla girebilen bir yönetici hesabı yoksa reddedilir.

const (
	sshPasswordLoginDropInName = "00-celikpanel.conf"
	sshPasswordLoginKeyFileMax = 1 << 20
	sshPasswordLoginDetailMax  = 300
	sshPasswordLoginTimeout    = 20 * time.Second
	sshConfigIncludeDepth      = 8
	sshConfigFileCountMax      = 64
	sshConfigFileMax           = 256 << 10
)

// sshPasswordLoginDropInText is exactly what CelikPanel writes. A drop-in
// with any other text is the owner's and is never replaced or removed.
const sshPasswordLoginDropInText = "# Written by CelikPanel at the administrator's request: SSH password sign-in is off.\n" +
	"# sshd uses the first value it reads; this file is read before the others in this folder.\n" +
	"# To allow passwords again without CelikPanel: delete this file, then run\n" +
	"#   systemctl reload ssh      (Arch Linux: systemctl reload sshd)\n" +
	"PasswordAuthentication no\n" +
	"KbdInteractiveAuthentication no\n"

var sshPasswordSettings = []string{"passwordauthentication", "kbdinteractiveauthentication"}

// sshPasswordLoginEnv is everything the feature touches; tests replace it.
type sshPasswordLoginEnv struct {
	root string // /etc/ssh
	// readConfig reads one sshd configuration file (root-owned, bounded).
	readConfig func(path string) ([]byte, error)
	glob       func(pattern string) ([]string, error)
	dirExists  func(path string) bool
	// sshd runs the verified sshd binary (-T, -t).
	sshd func(ctx context.Context, args ...string) ([]byte, error)
	// systemctl runs the verified systemctl.
	systemctl func(ctx context.Context, args ...string) ([]byte, error)
	unit      string
	passwd    func() ([]byte, error)
	group     func() ([]byte, error)
	// keyFile reads an authorized_keys file and says whether sshd's
	// StrictModes would accept it for uid (owner and modes of the file, its
	// directory and the home directory).
	keyFile func(file, home string, uid int, strict bool) ([]byte, bool)
	// write publishes the drop-in without replacing an existing file;
	// remove takes CelikPanel's drop-in away and returns how to put it back.
	write  func(path string, content []byte) error
	read   func(path string) ([]byte, bool, error)
	remove func(path string) (restore func() error, commit func(), err error)
}

func (env sshPasswordLoginEnv) mainConfig() string { return filepath.Join(env.root, "sshd_config") }
func (env sshPasswordLoginEnv) dropInDir() string  { return filepath.Join(env.root, "sshd_config.d") }
func (env sshPasswordLoginEnv) dropInPath() string {
	return filepath.Join(env.dropInDir(), sshPasswordLoginDropInName)
}

// --- Reading the configuration the way sshd does ------------------------------

type sshdDirective struct {
	file    string
	line    int
	key     string
	value   string
	inMatch bool
}

// walkSSHDConfig lists every directive in the order sshd reads them. virtual
// adds files that do not exist yet (the drop-in about to be written).
func walkSSHDConfig(env sshPasswordLoginEnv, virtual map[string][]byte) ([]sshdDirective, error) {
	var out []sshdDirective
	active := map[string]bool{}
	count := 0
	var walk func(file string, depth int, inMatch bool) (bool, error)
	walk = func(file string, depth int, inMatch bool) (bool, error) {
		if depth > sshConfigIncludeDepth {
			return inMatch, errors.New("SSH include depth exceeds the fixed limit")
		}
		if active[file] {
			return inMatch, errors.New("SSH include cycle")
		}
		count++
		if count > sshConfigFileCountMax {
			return inMatch, errors.New("SSH include count exceeds the fixed limit")
		}
		raw, ok := virtual[file]
		if !ok {
			var err error
			if raw, err = env.readConfig(file); err != nil {
				return inMatch, fmt.Errorf("%s could not be read safely: %w", file, err)
			}
		}
		if bytes.IndexByte(raw, 0) >= 0 {
			return inMatch, fmt.Errorf("%s contains NUL", file)
		}
		active[file] = true
		defer delete(active, file)
		scanner := bufio.NewScanner(bytes.NewReader(raw))
		scanner.Buffer(make([]byte, 64<<10), sshConfigFileMax)
		number := 0
		for scanner.Scan() {
			number++
			key, values := splitSSHDLine(scanner.Text())
			if key == "" {
				continue
			}
			switch key {
			case "match":
				inMatch = true
				continue
			case "include":
				for _, include := range values {
					pattern := include
					if !filepath.IsAbs(pattern) {
						pattern = filepath.Join(env.root, pattern)
					}
					pattern = filepath.Clean(pattern)
					matches, err := env.glob(pattern)
					if err != nil {
						return inMatch, fmt.Errorf("SSH include pattern %q is invalid", include)
					}
					for name := range virtual {
						if ok, _ := path.Match(pattern, name); ok && !containsString(matches, name) {
							matches = append(matches, name)
						}
					}
					sort.Strings(matches)
					for _, match := range matches {
						var err error
						if inMatch, err = walk(match, depth+1, inMatch); err != nil {
							return inMatch, err
						}
					}
				}
				continue
			}
			value := ""
			if len(values) > 0 {
				value = strings.ToLower(values[0])
			}
			out = append(out, sshdDirective{file: file, line: number, key: key, value: value, inMatch: inMatch})
		}
		if err := scanner.Err(); err != nil {
			return inMatch, fmt.Errorf("%s could not be read: %w", file, err)
		}
		return inMatch, nil
	}
	if _, err := walk(env.mainConfig(), 0, false); err != nil {
		return nil, err
	}
	return out, nil
}

// splitSSHDLine reads "Keyword value…" or "Keyword=value"; comments and blank
// lines give "".
func splitSSHDLine(line string) (string, []string) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", nil
	}
	if i := strings.Index(line, " #"); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	keyEnd := strings.IndexAny(line, " \t=")
	if keyEnd < 0 {
		return strings.ToLower(line), nil
	}
	key := strings.ToLower(line[:keyEnd])
	rest := strings.TrimLeft(line[keyEnd:], " \t")
	rest = strings.TrimLeft(strings.TrimPrefix(rest, "="), " \t")
	var values []string
	for _, field := range strings.Fields(rest) {
		values = append(values, strings.Trim(field, `"`))
	}
	return key, values
}

// sshdDeciders returns, per setting, the line sshd uses (the first outside a
// Match block), and whether a Match block sets it.
func sshdDeciders(directives []sshdDirective, settings []string) (map[string]*sshdDirective, map[string]*sshdDirective) {
	first := map[string]*sshdDirective{}
	match := map[string]*sshdDirective{}
	for i := range directives {
		d := &directives[i]
		if !containsString(settings, d.key) {
			continue
		}
		if d.inMatch {
			if match[d.key] == nil {
				match[d.key] = d
			}
			continue
		}
		if first[d.key] == nil {
			first[d.key] = d
		}
	}
	return first, match
}

// --- sshd -T --------------------------------------------------------------------

// parseSSHDEffective reads `sshd -T`: one "keyword value…" per line; a
// keyword sshd prints more than once (allowusers) keeps every value.
func parseSSHDEffective(raw []byte) map[string][]string {
	values := map[string][]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.ToLower(fields[0])
		values[key] = append(values[key], fields[1:]...)
	}
	return values
}

func effectiveOne(values map[string][]string, key string) string {
	if list := values[key]; len(list) > 0 {
		return strings.ToLower(list[0])
	}
	return "unknown"
}

// --- Who can still sign in with a key -----------------------------------------

type sshAccount struct {
	name  string
	uid   int
	gid   int
	home  string
	shell string
}

func parsePasswd(raw []byte) []sshAccount {
	var accounts []sshAccount
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Split(strings.TrimSpace(line), ":")
		if len(fields) < 7 || fields[0] == "" || strings.HasPrefix(fields[0], "#") {
			continue
		}
		uid, uerr := strconv.Atoi(fields[2])
		gid, gerr := strconv.Atoi(fields[3])
		if uerr != nil || gerr != nil {
			continue
		}
		accounts = append(accounts, sshAccount{name: fields[0], uid: uid, gid: gid, home: fields[5], shell: fields[6]})
	}
	return accounts
}

// parseGroups returns, per group name, its id and members.
func parseGroups(raw []byte) (map[string]int, map[string][]string) {
	ids, members := map[string]int{}, map[string][]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Split(strings.TrimSpace(line), ":")
		if len(fields) < 4 || fields[0] == "" {
			continue
		}
		if id, err := strconv.Atoi(fields[2]); err == nil {
			ids[fields[0]] = id
		}
		for _, member := range strings.Split(fields[3], ",") {
			if member = strings.TrimSpace(member); member != "" {
				members[fields[0]] = append(members[fields[0]], member)
			}
		}
	}
	return ids, members
}

var sshAdminGroups = []string{"sudo", "wheel", "admin"}

func sshLoginShell(shell string) bool {
	base := path.Base(shell)
	return shell != "" && base != "nologin" && base != "false" && base != "sync" && base != "halt" && base != "shutdown"
}

func accountGroups(account sshAccount, ids map[string]int, members map[string][]string) []string {
	var groups []string
	for name, id := range ids {
		if id == account.gid || containsString(members[name], account.name) {
			groups = append(groups, name)
		}
	}
	return groups
}

// sshdAllows applies AllowUsers/DenyUsers/AllowGroups/DenyGroups. A pattern
// with a host part (user@host) cannot be judged here, so such an account is
// not counted (the guard never claims more than it can prove).
func sshdAllows(account sshAccount, groups []string, effective map[string][]string) bool {
	matchAny := func(patterns []string, name string) (bool, bool) {
		for _, pattern := range patterns {
			if strings.Contains(pattern, "@") {
				return false, false
			}
			if ok, _ := path.Match(pattern, name); ok {
				return true, true
			}
		}
		return false, true
	}
	if hit, known := matchAny(effective["denyusers"], account.name); !known || hit {
		return false
	}
	if allow := effective["allowusers"]; len(allow) > 0 {
		if hit, known := matchAny(allow, account.name); !known || !hit {
			return false
		}
	}
	for _, group := range groups {
		if hit, known := matchAny(effective["denygroups"], group); !known || hit {
			return false
		}
	}
	if allow := effective["allowgroups"]; len(allow) > 0 {
		any := false
		for _, group := range groups {
			if hit, known := matchAny(allow, group); !known {
				return false
			} else if hit {
				any = true
			}
		}
		if !any {
			return false
		}
	}
	return true
}

// authorizedKeyPaths expands AuthorizedKeysFile for one account. A token
// this code does not know makes that path unknown (skipped).
func authorizedKeyPaths(patterns []string, account sshAccount) []string {
	if len(patterns) == 0 {
		patterns = []string{".ssh/authorized_keys", ".ssh/authorized_keys2"}
	}
	var out []string
	for _, pattern := range patterns {
		if strings.EqualFold(pattern, "none") {
			continue
		}
		var b strings.Builder
		ok := true
		for i := 0; i < len(pattern); i++ {
			if pattern[i] != '%' {
				b.WriteByte(pattern[i])
				continue
			}
			if i+1 >= len(pattern) {
				ok = false
				break
			}
			i++
			switch pattern[i] {
			case '%':
				b.WriteByte('%')
			case 'h':
				b.WriteString(account.home)
			case 'u':
				b.WriteString(account.name)
			case 'U':
				b.WriteString(strconv.Itoa(account.uid))
			default:
				ok = false
			}
		}
		if !ok {
			continue
		}
		file := b.String()
		if !filepath.IsAbs(file) {
			file = filepath.Join(account.home, file)
		}
		out = append(out, filepath.Clean(file))
	}
	return out
}

// countAuthorizedKeys counts the lines sshd would read as keys. Options and
// comments are allowed; a line with restrict/command= still signs in, but
// only for that command, so it is not counted as a way back in.
func countAuthorizedKeys(raw []byte) int {
	count := 0
	rest := raw
	for len(rest) > 0 {
		key, _, options, next, err := gossh.ParseAuthorizedKey(rest)
		if err != nil {
			break
		}
		rest = next
		if key == nil {
			continue
		}
		forced := false
		for _, option := range options {
			lower := strings.ToLower(option)
			if strings.HasPrefix(lower, "command=") || lower == "restrict" {
				forced = true
			}
		}
		if !forced {
			count++
		}
	}
	return count
}

// sshKeyAccounts lists the accounts that keep a way in once passwords are off.
func sshKeyAccounts(env sshPasswordLoginEnv, effective map[string][]string) ([]transport.SSHKeyAccount, error) {
	passwdRaw, err := env.passwd()
	if err != nil {
		return nil, fmt.Errorf("the account list could not be read: %w", err)
	}
	groupRaw, err := env.group()
	if err != nil {
		return nil, fmt.Errorf("the group list could not be read: %w", err)
	}
	ids, members := parseGroups(groupRaw)
	rootLogin := effectiveOne(effective, "permitrootlogin")
	strict := effectiveOne(effective, "strictmodes") != "no"
	var out []transport.SSHKeyAccount
	for _, account := range parsePasswd(passwdRaw) {
		groups := accountGroups(account, ids, members)
		root := account.uid == 0
		admin := false
		for _, group := range groups {
			if containsString(sshAdminGroups, group) {
				admin = true
			}
		}
		if root {
			// forced-commands-only lets root run one command, not sign in.
			if rootLogin != "yes" && rootLogin != "prohibit-password" && rootLogin != "without-password" {
				continue
			}
		} else if !admin {
			continue
		}
		if !sshLoginShell(account.shell) || !filepath.IsAbs(account.home) || !sshdAllows(account, groups, effective) {
			continue
		}
		keys := 0
		for _, file := range authorizedKeyPaths(effective["authorizedkeysfile"], account) {
			raw, ok := env.keyFile(file, account.home, account.uid, strict)
			if ok {
				keys += countAuthorizedKeys(raw)
			}
		}
		if keys == 0 {
			continue
		}
		out = append(out, transport.SSHKeyAccount{Name: account.name, Root: root, Keys: keys})
		if len(out) == transport.SSHKeyAccountsMax {
			break
		}
	}
	return out, nil
}

// --- Status -----------------------------------------------------------------------

func boundedSSHDetail(text string) string {
	text = strings.TrimSpace(text)
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	if len(text) > sshPasswordLoginDetailMax {
		text = text[:sshPasswordLoginDetailMax]
	}
	return text
}

func directiveDetail(d *sshdDirective) string {
	if d == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d", d.file, d.line)
}

// sshPasswordLoginStatus reads everything and decides what may be done.
func sshPasswordLoginStatus(ctx context.Context, env sshPasswordLoginEnv) transport.SSHPasswordLoginStatusResponse {
	status := transport.SSHPasswordLoginStatusResponse{
		State: transport.SSHPasswordLoginStateRead, Unit: env.unit,
		PasswordAuthentication: "unknown", KbdInteractiveAuthentication: "unknown",
		PermitRootLogin: "unknown", PubkeyAuthentication: "unknown",
		Deciders: []transport.SSHSettingDecider{}, KeyAccounts: []transport.SSHKeyAccount{},
		DropIn: transport.SSHDropIn{Path: env.dropInPath(), State: transport.SSHDropInAbsent},
	}
	if env.unit != "" && env.systemctl != nil {
		if out, err := env.systemctl(ctx, "show", "-p", "ActiveState", "--value", env.unit+".service"); err == nil {
			status.UnitActive = strings.TrimSpace(string(out)) == "active"
		}
	}
	raw, err := env.sshd(ctx, "-T")
	if err != nil {
		status.State, status.Reason, status.Detail = transport.SSHPasswordLoginStateUnreadable, "sshd_unreadable", boundedSSHDetail(string(raw))
		return status
	}
	effective := parseSSHDEffective(raw)
	status.PasswordAuthentication = effectiveOne(effective, "passwordauthentication")
	status.KbdInteractiveAuthentication = effectiveOne(effective, "kbdinteractiveauthentication")
	status.PermitRootLogin = effectiveOne(effective, "permitrootlogin")
	status.PubkeyAuthentication = effectiveOne(effective, "pubkeyauthentication")

	if content, exists, err := env.read(env.dropInPath()); err != nil {
		status.DropIn.State = transport.SSHDropInEdited
	} else if exists {
		status.DropIn.SHA256 = sha256Hex(content)
		if string(content) == sshPasswordLoginDropInText {
			status.DropIn.State = transport.SSHDropInPanel
		} else {
			status.DropIn.State = transport.SSHDropInEdited
		}
	}
	status.CanTurnOn = status.DropIn.State == transport.SSHDropInPanel

	directives, err := walkSSHDConfig(env, nil)
	if err != nil {
		status.State, status.Reason, status.Detail = transport.SSHPasswordLoginStateUnreadable, "config_unreadable", boundedSSHDetail(err.Error())
		return status
	}
	first, _ := sshdDeciders(directives, append([]string{"permitrootlogin"}, sshPasswordSettings...))
	for _, setting := range append(append([]string{}, sshPasswordSettings...), "permitrootlogin") {
		d := transport.SSHSettingDecider{Setting: setting, Value: effectiveOne(effective, setting)}
		if line := first[setting]; line != nil {
			d.File, d.Line = line.file, line.line
		} else {
			d.Default = true
		}
		status.Deciders = append(status.Deciders, d)
	}
	if accounts, err := sshKeyAccounts(env, effective); err == nil {
		status.KeyAccounts = accounts
	} else {
		status.Detail = boundedSSHDetail(err.Error())
	}
	status.CanTurnOff, status.Reason, status.Detail = sshTurnOffAllowed(env, status, effective)
	return status
}

// sshTurnOffAllowed decides whether the drop-in can be written now.
func sshTurnOffAllowed(env sshPasswordLoginEnv, status transport.SSHPasswordLoginStatusResponse, effective map[string][]string) (bool, string, string) {
	if status.PasswordAuthentication == "no" && status.KbdInteractiveAuthentication == "no" {
		return false, "already_off", ""
	}
	switch status.DropIn.State {
	case transport.SSHDropInEdited:
		return false, "drop_in_edited", status.DropIn.Path
	case transport.SSHDropInPanel:
		// CelikPanel's own drop-in is there, yet passwords are on: something
		// else decides (said by include_order below on a fresh read).
	}
	if env.dirExists != nil && !env.dirExists(env.dropInDir()) {
		return false, "drop_in_dir_missing", env.dropInDir()
	}
	if status.PubkeyAuthentication != "yes" {
		return false, "pubkey_off", ""
	}
	virtual := map[string][]byte{}
	if status.DropIn.State == transport.SSHDropInAbsent {
		virtual[env.dropInPath()] = []byte(sshPasswordLoginDropInText)
	}
	directives, err := walkSSHDConfig(env, virtual)
	if err != nil {
		return false, "config_unreadable", boundedSSHDetail(err.Error())
	}
	first, _ := sshdDeciders(directives, sshPasswordSettings)
	for _, setting := range sshPasswordSettings {
		if line := enablingMatchDirective(directives, setting); line != nil {
			return false, "match_block", directiveDetail(line)
		}
		if line := first[setting]; line == nil || line.file != env.dropInPath() {
			return false, "include_order", directiveDetail(line)
		}
	}
	if len(status.KeyAccounts) == 0 {
		return false, "no_key_account", ""
	}
	return true, "", ""
}

// enablingMatchDirective is the first Match-block line that can turn the
// setting on for some connections. A Match block that only says "no" narrows
// and never opens: on every BIND secondary CelikPanel's own peer-inspector
// account (/etc/bind-peer-inspector/sshd-match.conf, "Match User celikpeer",
// PasswordAuthentication no) is such a block, and an owner's restriction is
// another. Neither blocks the change (lab, alpha.130: a joined Ubuntu 24.04
// server answered match_block and could never turn passwords off).
//
// Yalnız "no" diyen bir Match bloğu daraltır, açmaz; değişikliği engellemez.
func enablingMatchDirective(directives []sshdDirective, setting string) *sshdDirective {
	for i := range directives {
		d := &directives[i]
		if d.inMatch && d.key == setting && d.value != "no" {
			return d
		}
	}
	return nil
}

// --- The change -------------------------------------------------------------------

// setSSHPasswordLogin runs one confirmed change. Every step is checked; any
// failure takes CelikPanel's change back.
func setSSHPasswordLogin(ctx context.Context, env sshPasswordLoginEnv, req *transport.SetSSHPasswordLoginRequest) transport.SetSSHPasswordLoginResponse {
	resp := transport.SetSSHPasswordLoginResponse{Unit: env.unit}
	refuse := func(code, message, detail string) transport.SetSSHPasswordLoginResponse {
		resp.Outcome, resp.Code, resp.Error, resp.Detail = transport.SSHPasswordLoginRefused, code, message, boundedSSHDetail(detail)
		return resp
	}
	status := sshPasswordLoginStatus(ctx, env)
	if status.State != transport.SSHPasswordLoginStateRead {
		return refuse(status.Reason, "the SSH configuration could not be read, so nothing was changed", status.Detail)
	}
	if status.DropIn.SHA256 != req.ExpectedDropInSHA256 {
		return refuse("changed", status.DropIn.Path+" changed after the page showed it; nothing was changed", "")
	}
	switch req.Action {
	case transport.SSHPasswordLoginActionOff:
		if !status.CanTurnOff {
			if status.Reason == "already_off" {
				resp.Outcome = transport.SSHPasswordLoginUnchanged
				resp.PasswordAuthentication, resp.KbdInteractiveAuthentication = status.PasswordAuthentication, status.KbdInteractiveAuthentication
				return resp
			}
			return refuse(status.Reason, "password sign-in was not turned off; nothing was changed", status.Detail)
		}
		return turnSSHPasswordsOff(ctx, env, status, resp)
	case transport.SSHPasswordLoginActionOn:
		if status.DropIn.State == transport.SSHDropInAbsent {
			resp.Outcome = transport.SSHPasswordLoginUnchanged
			resp.PasswordAuthentication, resp.KbdInteractiveAuthentication = status.PasswordAuthentication, status.KbdInteractiveAuthentication
			return resp
		}
		if !status.CanTurnOn {
			return refuse("drop_in_edited", status.DropIn.Path+" is not CelikPanel's text any more, so CelikPanel does not remove it", status.DropIn.Path)
		}
		return turnSSHPasswordsOn(ctx, env, resp)
	}
	return refuse("invalid", "the action must be turn_off or turn_on", "")
}

func sshValidate(ctx context.Context, env sshPasswordLoginEnv) error {
	if out, err := env.sshd(ctx, "-t"); err != nil {
		detail := boundedSSHDetail(string(out))
		if detail == "" {
			detail = err.Error()
		}
		return errors.New(detail)
	}
	return nil
}

// sshReload asks sshd to read its configuration again. An inactive unit
// (socket activation) reads it when the next connection starts it.
func sshReload(ctx context.Context, env sshPasswordLoginEnv) (reloaded, nextStart bool, err error) {
	out, err := env.systemctl(ctx, "show", "-p", "ActiveState", "--value", env.unit+".service")
	if err != nil {
		return false, false, fmt.Errorf("the state of %s could not be read", env.unit)
	}
	if strings.TrimSpace(string(out)) != "active" {
		return false, true, nil
	}
	if out, err := env.systemctl(ctx, "reload", env.unit+".service"); err != nil {
		detail := boundedSSHDetail(string(out))
		if detail == "" {
			detail = err.Error()
		}
		return false, false, fmt.Errorf("reloading %s failed: %s", env.unit, detail)
	}
	return true, false, nil
}

func turnSSHPasswordsOff(ctx context.Context, env sshPasswordLoginEnv, status transport.SSHPasswordLoginStatusResponse, resp transport.SetSSHPasswordLoginResponse) transport.SetSSHPasswordLoginResponse {
	if err := sshValidate(ctx, env); err != nil {
		resp.Outcome, resp.Code, resp.Error, resp.Detail = transport.SSHPasswordLoginRefused, "config_invalid",
			"sshd does not accept its current configuration, so CelikPanel did not add to it; nothing was changed", boundedSSHDetail(err.Error())
		return resp
	}
	path := env.dropInPath()
	if status.DropIn.State == transport.SSHDropInAbsent {
		if err := env.write(path, []byte(sshPasswordLoginDropInText)); err != nil {
			resp.Outcome, resp.Code, resp.Error, resp.Detail = transport.SSHPasswordLoginRefused, "write_failed",
				"the setting file could not be written; nothing was changed", boundedSSHDetail(err.Error())
			return resp
		}
	}
	rollback := func(code, message, detail string) transport.SetSSHPasswordLoginResponse {
		resp.Code, resp.Error, resp.Detail = code, message, boundedSSHDetail(detail)
		restoreErr := sshRemovePanelDropIn(env, path)
		if restoreErr == nil {
			restoreErr = sshValidate(ctx, env)
		}
		if restoreErr == nil && resp.Reloaded {
			_, _, restoreErr = sshReload(ctx, env)
		}
		if restoreErr != nil {
			resp.Outcome = transport.SSHPasswordLoginRollbackFailed
			resp.Detail = boundedSSHDetail(resp.Detail + "; taking it back failed: " + restoreErr.Error())
			return resp
		}
		resp.Outcome, resp.Reloaded = transport.SSHPasswordLoginRolledBack, false
		return resp
	}
	if err := sshValidate(ctx, env); err != nil {
		return rollback("validate_failed", "sshd did not accept the setting, so CelikPanel removed it again", err.Error())
	}
	raw, err := env.sshd(ctx, "-T")
	if err != nil {
		return rollback("confirm_failed", "sshd's effective settings could not be read, so CelikPanel removed the setting again", string(raw))
	}
	effective := parseSSHDEffective(raw)
	resp.PasswordAuthentication = effectiveOne(effective, "passwordauthentication")
	resp.KbdInteractiveAuthentication = effectiveOne(effective, "kbdinteractiveauthentication")
	if resp.PasswordAuthentication != "no" || resp.KbdInteractiveAuthentication != "no" {
		return rollback("not_effective", "sshd still allows passwords with the setting in place (another file decides), so CelikPanel removed it again", "")
	}
	reloaded, nextStart, err := sshReload(ctx, env)
	resp.Reloaded, resp.AppliesAtNextStart = reloaded, nextStart
	if err != nil {
		resp.Reloaded = true // the reload may have half-happened: reload again after removing
		return rollback("reload_failed", "sshd could not be reloaded, so CelikPanel removed the setting again", err.Error())
	}
	resp.Outcome = transport.SSHPasswordLoginTurnedOff
	return resp
}

// sshRemovePanelDropIn removes the drop-in only while its text is exactly
// CelikPanel's.
func sshRemovePanelDropIn(env sshPasswordLoginEnv, path string) error {
	content, exists, err := env.read(path)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if string(content) != sshPasswordLoginDropInText {
		return fmt.Errorf("%s changed meanwhile and was left as it is", path)
	}
	_, commit, err := env.remove(path)
	if err != nil {
		return err
	}
	commit()
	return nil
}

func turnSSHPasswordsOn(ctx context.Context, env sshPasswordLoginEnv, resp transport.SetSSHPasswordLoginResponse) transport.SetSSHPasswordLoginResponse {
	path := env.dropInPath()
	restore, commit, err := env.remove(path)
	if err != nil {
		resp.Outcome, resp.Code, resp.Error, resp.Detail = transport.SSHPasswordLoginRefused, "remove_failed",
			"the setting file could not be removed; nothing was changed", boundedSSHDetail(err.Error())
		return resp
	}
	rollback := func(code, message, detail string, reloadAgain bool) transport.SetSSHPasswordLoginResponse {
		resp.Code, resp.Error, resp.Detail = code, message, boundedSSHDetail(detail)
		restoreErr := restore()
		if restoreErr == nil && reloadAgain {
			_, _, restoreErr = sshReload(ctx, env)
		}
		if restoreErr != nil {
			resp.Outcome = transport.SSHPasswordLoginRollbackFailed
			resp.Detail = boundedSSHDetail(resp.Detail + "; putting it back failed: " + restoreErr.Error())
			return resp
		}
		resp.Outcome, resp.Reloaded = transport.SSHPasswordLoginRolledBack, false
		return resp
	}
	if err := sshValidate(ctx, env); err != nil {
		return rollback("validate_failed", "without CelikPanel's setting sshd does not accept the configuration, so the setting was put back", err.Error(), false)
	}
	reloaded, nextStart, err := sshReload(ctx, env)
	if err != nil {
		return rollback("reload_failed", "sshd could not be reloaded, so CelikPanel's setting was put back", err.Error(), true)
	}
	commit()
	resp.Reloaded, resp.AppliesAtNextStart = reloaded, nextStart
	resp.Outcome = transport.SSHPasswordLoginTurnedOn
	if raw, err := env.sshd(ctx, "-T"); err == nil {
		effective := parseSSHDEffective(raw)
		resp.PasswordAuthentication = effectiveOne(effective, "passwordauthentication")
		resp.KbdInteractiveAuthentication = effectiveOne(effective, "kbdinteractiveauthentication")
		if resp.PasswordAuthentication == "no" {
			if directives, err := walkSSHDConfig(env, nil); err == nil {
				first, _ := sshdDeciders(directives, sshPasswordSettings)
				resp.StillOffBy = directiveDetail(first["passwordauthentication"])
			}
		}
	}
	return resp
}

// --- RPC ---------------------------------------------------------------------------

// SSHPasswordLoginStatusV1 reads; it changes nothing.
func (a *Agent) SSHPasswordLoginStatusV1(_ *transport.Empty, resp *transport.SSHPasswordLoginStatusResponse) error {
	env, err := productionSSHPasswordLoginEnv()
	if err != nil {
		*resp = transport.SSHPasswordLoginStatusResponse{
			State: transport.SSHPasswordLoginStateUnsupported, Reason: "unsupported", Detail: boundedSSHDetail(err.Error()),
			PasswordAuthentication: "unknown", KbdInteractiveAuthentication: "unknown", PermitRootLogin: "unknown", PubkeyAuthentication: "unknown",
			Deciders: []transport.SSHSettingDecider{}, KeyAccounts: []transport.SSHKeyAccount{},
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), sshPasswordLoginTimeout)
	defer cancel()
	*resp = sshPasswordLoginStatus(ctx, env)
	return nil
}

// SetSSHPasswordLoginV1 is the administrator's confirmed change.
func (a *Agent) SetSSHPasswordLoginV1(req *transport.SetSSHPasswordLoginRequest, resp *transport.SetSSHPasswordLoginResponse) error {
	*resp = transport.SetSSHPasswordLoginResponse{}
	if req == nil || (req.Action != transport.SSHPasswordLoginActionOff && req.Action != transport.SSHPasswordLoginActionOn) ||
		(req.ExpectedDropInSHA256 != "" && !validSHA256Hex(req.ExpectedDropInSHA256)) {
		resp.Outcome, resp.Code, resp.Error = transport.SSHPasswordLoginRefused, "invalid", "the request is not valid; nothing was changed"
		return nil
	}
	ctx, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepSSHPasswordLogin, sshPasswordLoginTarget, "", req.Action),
	)
	if err != nil {
		resp.Outcome, resp.Code, resp.Error = transport.SSHPasswordLoginRefused, "MUTATION_REFUSED", err.Error()
		return nil
	}
	defer finishStep()
	env, err := productionSSHPasswordLoginEnv()
	if err != nil {
		resp.Outcome, resp.Code, resp.Error = transport.SSHPasswordLoginRefused, "unsupported", boundedSSHDetail(err.Error())
		return nil
	}
	*resp = setSSHPasswordLogin(ctx, env, req)
	return nil
}

const sshPasswordLoginTarget = "openssh"

func validSHA256Hex(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
