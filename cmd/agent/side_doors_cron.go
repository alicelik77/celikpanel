package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The last doors for a site's own jobs (S4 R2b, 22 Nov 2026):
//
//  1. /etc/cron.allow keeps the accounts of moved sites (whose tasks run as
//     timers in their limits) from installing their own crontab, by R1's
//     at.allow rules: written only when absent and only once a site is
//     moved; every account that may use crontab today keeps it, minus the
//     moved sites' accounts, CelikPanel's tool accounts and every login that
//     shares a site's id (crontab decides by the first passwd name of the
//     caller's uid); root is listed first. A site not yet moved stays
//     allowed. The owner's file is kept byte for byte and a moved account it
//     names is a Problems now line; the owner's deletion of CelikPanel's file
//     stands (shown, with one explicit action to close it again); accounts
//     from a directory service: nothing is written, and it is said.
//     Measured (Stage 1 F1, Debian 13 and Ubuntu 24.04): an account the file
//     leaves out keeps running the crontab it already has, and root's
//     `crontab -u <account> -l/-r/<file>` is refused for it; root still reads
//     and removes the spool file, and cron notices without a signal. So the
//     move empties the crontab itself, and a crontab that comes back for a
//     moved account (the owner's) is read from the spool and reported.
//  2. Lingering (`loginctl enable-linger`): measured on both systems, a site
//     account with no session (a site's PHP) may switch it on for itself
//     with polkitd as shipped (org.freedesktop.login1.set-self-linger is
//     "yes" for anyone), and then runs its own user services outside its
//     slice. Without polkitd systemd-logind refuses every account but root.
//     Where polkitd is installed, a rules file asks for an administrator's
//     password instead, unless the caller is in the system's administrator
//     group (sudo, admin or wheel, those that exist). Accounts that already
//     linger keep lingering: they are named in Problems now with the command
//     that ends it; nothing is removed by itself.
//
// Sitelerin kendi işleri için son kapılar: taşınmış sitelerin hesapları
// crontab kuramaz; site hesapları oturum dışında program çalışır tutamaz.

var (
	cronAllowPath       = "/etc/cron.allow"
	cronDenyPath        = "/etc/cron.deny"
	cronAllowRecordPath = filepath.Join(hostingpath.ServiceMutationStateRoot(), "cron-allow.sha256")
)

// cronAllowMark is the first line of CelikPanel's cron.allow.
const cronAllowMark = "# Written by CelikPanel to keep the accounts of sites whose tasks run in their limits from using cron."

const cronAllowHeader = cronAllowMark + `
# Only the accounts listed below may use crontab. They are every account that could use it
# before (not named in /etc/cron.deny), except the accounts of sites whose scheduled tasks
# run as timers in the site's own limits, CelikPanel's tool accounts and extra logins that
# share a site's id. A crontab such an account already had keeps running (CelikPanel moved
# the site's tasks out of it). While this file is as CelikPanel wrote it, CelikPanel brings it
# up to date at its next start and after each move: an account you create later may use
# crontab from the panel's next start; until then (or without the panel) add its name here or
# delete this file. Once you change it, it is yours and CelikPanel leaves it as it is.
# To undo: delete this file. /etc/cron.deny then applies again, as before, and CelikPanel
# does not write it again by itself: its Problems now list says cron is open to those
# accounts, and closing it again there writes it anew.
`

// cronAllowMu serializes CelikPanel's writes of cron.allow.
var cronAllowMu sync.Mutex

// movedSiteAccounts are the accounts of moved sites (from their task files,
// or their units when a task file is gone), except the site exceptSite.
func movedSiteAccounts(exceptSite int) map[string]bool {
	out, _ := movedSiteAccountsChecked(exceptSite)
	return out
}

// movedSiteAccountsChecked is movedSiteAccounts with an error naming the
// moved sites whose account could not be read at all (second fix round):
// cron.allow is then left as it is rather than rendered without them.
func movedSiteAccountsChecked(exceptSite int) (map[string]bool, error) {
	ids, accounts, unknown := movedSiteTaskSitesAll()
	out := map[string]bool{}
	for _, id := range ids {
		if id == exceptSite {
			continue
		}
		if account := accounts[id]; account != "" {
			out[account] = true
		}
	}
	var missing []string
	for _, id := range unknown {
		if id != exceptSite {
			missing = append(missing, strconv.Itoa(id))
		}
	}
	if len(missing) > 0 {
		return out, fmt.Errorf("the account of moved site %s could be read neither from its task file nor from its timers' units", strings.Join(missing, ", "))
	}
	return out, nil
}

// renderCronAllow: root, then every account cron.deny does not name, in
// passwd order, minus the moved sites' accounts, the tool accounts and every
// login sharing a site's (or tool's) id.
func renderCronAllow(entries []passwdEntry, deny string, moved map[string]bool) string {
	denied := map[string]bool{}
	for _, name := range nameLines(deny) {
		denied[name] = true
	}
	websiteUIDs := atWebsiteUIDs(entries)
	seen := map[string]bool{"root": true}
	var b strings.Builder
	b.WriteString(cronAllowHeader)
	b.WriteString("root\n")
	for _, entry := range entries {
		site := hostingpath.IsSiteHome(entry.Home)
		switch {
		case seen[entry.Name], denied[entry.Name], len(entry.Name) > 255, moved[entry.Name]:
			continue
		case !site && atWebsiteAccount(entry): // a tool account
			continue
		case !site && websiteUIDs[entry.UID]: // a login sharing a site's id
			continue
		}
		seen[entry.Name] = true
		b.WriteString(entry.Name + "\n")
	}
	return b.String()
}

type siteCronOps struct {
	installed func() bool
	readFile  func(string) ([]byte, error)
	writeLike func(path, like string, data []byte) error
	writeNew  func(path string, data []byte) error
}

func hostSiteCronOps() siteCronOps {
	return siteCronOps{
		installed: func() bool { return requireCronInstalled() == nil },
		readFile:  os.ReadFile,
		writeLike: writeFileLike,
		writeNew: func(path string, data []byte) error {
			tmp, err := os.CreateTemp(filepath.Dir(path), ".celikpanel-"+filepath.Base(path)+"-*")
			if err != nil {
				return err
			}
			name := tmp.Name()
			defer os.Remove(name)
			_, err = tmp.Write(data)
			if err == nil {
				err = tmp.Chmod(0o644)
			}
			if err == nil {
				err = tmp.Sync()
			}
			if closeErr := tmp.Close(); err == nil {
				err = closeErr
			}
			if err == nil {
				err = os.Rename(name, path)
			}
			return err
		},
	}
}

// cronAllowOurs: the file is CelikPanel's as it last wrote it.
func cronAllowOurs(ops siteCronOps, current []byte) bool {
	if !strings.HasPrefix(string(current), cronAllowMark+"\n") {
		return false
	}
	recorded, err := ops.readFile(cronAllowRecordPath)
	return err == nil && strings.TrimSpace(string(recorded)) == atDigest(current)
}

// closeSiteCronWith decides and, where allowed, writes /etc/cron.allow;
// exceptSite is a site being put back into cron (its account is allowed).
func closeSiteCronWith(ops siteCronOps, exceptSite int) *transport.SideDoorsCron {
	if !ops.installed() {
		return &transport.SideDoorsCron{State: transport.SideDoorsAtNotInstalled}
	}
	out := &transport.SideDoorsCron{Path: cronAllowPath}
	moved, unknownErr := movedSiteAccountsChecked(exceptSite)
	defer func() {
		// After the file is decided: a crontab found for a moved account, and
		// whether the account could have installed it itself (fix round: it is
		// called the server owner's only when it could not).
		pruneCronClosed()
		for account := range moved {
			if n := len(spoolCrontabLines(account)); n > 0 {
				if out.Reappeared == nil {
					out.Reappeared, out.ReappearedBySite = map[string]int{}, map[string]bool{}
				}
				out.Reappeared[account] = n
				files, _ := accountSpoolFiles(account)
				if reappearedBySite(account, files) {
					out.ReappearedBySite[account] = true
				}
			}
		}
	}()
	entries, nisLines, passwdErr := readPasswd(sideDoorsPasswd)
	if passwdErr != nil {
		out.State, out.Detail = transport.SideDoorsAtFailed, "the account list could not be read: "+passwdErr.Error()
		return out
	}
	// Names that stand for a moved site: its account and logins sharing its id.
	movedUIDs := map[int]bool{}
	for _, entry := range entries {
		if moved[entry.Name] && entry.UID != 0 {
			movedUIDs[entry.UID] = true
		}
	}
	standsForMoved := func(name string) bool {
		if moved[name] {
			return true
		}
		for _, entry := range entries {
			if entry.Name == name && movedUIDs[entry.UID] {
				return true
			}
		}
		return false
	}
	deny, denyErr := ops.readFile(cronDenyPath)
	if denyErr != nil && !errors.Is(denyErr, fs.ErrNotExist) {
		out.State, out.Detail = transport.SideDoorsAtFailed, denyErr.Error()
		return out
	}
	current, err := ops.readFile(cronAllowPath)
	switch {
	case err == nil:
		if !cronAllowOurs(ops, current) || nisLines {
			out.State = transport.SideDoorsAtOwnerFile
			for _, name := range nameLines(string(current)) {
				if standsForMoved(name) {
					out.SiteAccounts = append(out.SiteAccounts, name)
				}
			}
			sort.Strings(out.SiteAccounts)
			return out
		}
		if unknownErr != nil {
			// A moved site whose account is unknown must not come back into
			// cron by a render without it: the file stays as it is.
			log.Printf("site tasks: %s is left as it is this time: %v", cronAllowPath, unknownErr)
			out.State, out.Detail = transport.SideDoorsAtCurrent, unknownErr.Error()+"; the file was left as it is"
			return out
		}
		desired := renderCronAllow(entries, string(deny), moved)
		if desired == string(current) {
			out.State = transport.SideDoorsAtCurrent
			return out
		}
		return writeCronAllow(ops, out, desired)
	case !errors.Is(err, fs.ErrNotExist):
		out.State, out.Detail = transport.SideDoorsAtFailed, err.Error()
		return out
	}
	// The owner's undo stands (D-022).
	if _, recordErr := ops.readFile(cronAllowRecordPath); recordErr == nil {
		out.State = transport.SideDoorsAtOwnerRemoved
		return out
	}
	if unknownErr != nil {
		log.Printf("site tasks: %s is not written this time: %v", cronAllowPath, unknownErr)
		out.State, out.Detail = transport.SideDoorsAtFailed, unknownErr.Error()+"; the file was not written"
		return out
	}
	if len(moved) == 0 {
		out.State = transport.SideDoorsCronNotNeeded
		return out
	}
	nss, nssErr := ops.readFile(sideDoorsNSSwitch)
	if nisLines || (nssErr == nil && nssDirectoryAccounts(string(nss))) {
		out.State = transport.SideDoorsAtDirectory
		out.Detail = "accounts come from a directory service (" + sideDoorsNSSwitch + " or + lines in " + sideDoorsPasswd + "), so who may use cron today cannot be listed; nothing was written"
		return out
	}
	return writeCronAllow(ops, out, renderCronAllow(entries, string(deny), moved))
}

func writeCronAllow(ops siteCronOps, out *transport.SideDoorsCron, content string) *transport.SideDoorsCron {
	var err error
	if _, statErr := os.Stat(cronDenyPath); statErr == nil {
		err = ops.writeLike(cronAllowPath, cronDenyPath, []byte(content))
	} else {
		err = ops.writeNew(cronAllowPath, []byte(content))
	}
	if err != nil {
		out.State, out.Detail = transport.SideDoorsAtFailed, err.Error()
		return out
	}
	out.State = transport.SideDoorsAtWritten
	if err := writePrivateFileAtomic(cronAllowRecordPath, []byte(atDigest([]byte(content))+"\n"), ".cron-allow-*"); err != nil {
		out.Detail = "CelikPanel's record of the file could not be written, so it will not bring it up to date: " + err.Error()
	}
	return out
}

// refreshCronAllow brings CelikPanel's cron.allow up to date after a move,
// an undo or a new site (an absent file is written once a site is moved).
func refreshCronAllow() {
	cronAllowMu.Lock()
	defer cronAllowMu.Unlock()
	if out := closeSiteCronWith(hostSiteCronOps(), 0); out.State == transport.SideDoorsAtFailed {
		log.Printf("site tasks: /etc/cron.allow: %s", out.Detail)
	}
}

// cronAllowRefuses says why the account could not use cron after an undo
// ("" when it can, or when CelikPanel's own file decides): the server
// owner's cron.allow that does not list it, or cron.deny that names it.
func cronAllowRefuses(account string, siteID int) string {
	ops := hostSiteCronOps()
	if current, err := ops.readFile(cronAllowPath); err == nil {
		if cronAllowOurs(ops, current) {
			return ""
		}
		for _, name := range nameLines(string(current)) {
			if name == account {
				return ""
			}
		}
		return "the server owner's " + cronAllowPath + " does not list " + account + ", so the account could not use its crontab; nothing was changed. The server owner adds the account to that file (or deletes the file), then this can be pressed again"
	} else if !errors.Is(err, fs.ErrNotExist) {
		return cronAllowPath + " could not be read (" + err.Error() + "); nothing was changed"
	}
	if exists, listed := cronAccessLists(cronDenyPath, account); exists && listed {
		return cronDenyPath + " names " + account + ", so the account could not use its crontab; nothing was changed. The server owner removes it from that file, then this can be pressed again"
	}
	return ""
}

// allowCronFor lets the account use cron again in CelikPanel's own file
// (the undo, before its crontab is written).
func allowCronFor(account string, siteID int) error {
	cronAllowMu.Lock()
	defer cronAllowMu.Unlock()
	out := closeSiteCronWith(hostSiteCronOps(), siteID)
	switch out.State {
	case transport.SideDoorsAtFailed:
		return fmt.Errorf("%s could not be brought up to date (%s); nothing was changed", cronAllowPath, out.Detail)
	case transport.SideDoorsAtOwnerFile:
		if why := cronAllowRefuses(account, siteID); why != "" {
			return errors.New(why)
		}
	}
	return nil
}

// --- Lingering -------------------------------------------------------------------

var (
	polkitRulesDir    = "/etc/polkit-1/rules.d"
	polkitDaemonPaths = []string{"/usr/lib/polkit-1/polkitd", "/usr/libexec/polkitd", "/usr/libexec/polkit-1/polkitd"}
	lingerDir         = "/var/lib/systemd/linger"
	sideDoorsGroup    = "/etc/group"
)

const (
	lingerRuleName  = "49-celikpanel-linger.rules"
	lingerRuleKind  = "polkit_linger_rule"
	polkitRulesHead = "// celikpanel-render v2 sha256="
)

// lingerAdminGroups are the administrator groups the rule lets switch
// lingering on for themselves, those that exist on the host.
var lingerAdminGroups = []string{"sudo", "admin", "wheel"}

func existingGroups(content string, names []string) []string {
	have := map[string]bool{}
	for _, line := range strings.Split(content, "\n") {
		if name, _, ok := strings.Cut(line, ":"); ok {
			have[name] = true
		}
	}
	var out []string
	for _, name := range names {
		if have[name] {
			out = append(out, name)
		}
	}
	return out
}

// lingerRuleText is the rules file (without CelikPanel's render header).
func lingerRuleText(groups []string) string {
	who := "root only"
	condition := `action.id == "org.freedesktop.login1.set-self-linger"`
	if len(groups) > 0 {
		who = "root and the members of " + strings.Join(groups, ", ")
		for _, g := range groups {
			condition += ` &&
        !subject.isInGroup("` + g + `")`
		}
	}
	return `// CelikPanel (docs/SITE-SANDBOX.md section 9.5): a website's account may not keep its own
// programs running after its work is done ("lingering", loginctl enable-linger). Switching
// lingering on for oneself is left to ` + who + `; any other account is asked
// for an administrator's password, which a website's code cannot give. Only the groups
// sudo, admin and wheel count, as /etc/group names them: administrators in another group
// (from LDAP or SSSD, for example) run "sudo loginctl enable-linger <name>", or the server
// owner adds their group below (the file is then the owner's and CelikPanel keeps it).
// Accounts that already linger keep doing so (CelikPanel names them in its Problems now list).
// To undo: delete this file; polkit reads the change at once. CelikPanel does not write it
// again by itself; its Problems now list then says lingering is open.
//
// CelikPanel: site hesaplari kendi programlarini is bittikten sonra calisir tutamaz.
polkit.addRule(function(action, subject) {
    if (` + condition + `) {
        return polkit.Result.AUTH_ADMIN;
    }
});
`
}

type siteLingerOps struct {
	installed func() bool
	// legacy: polkit 0.105 (Ubuntu 22.04), which reads .pkla files and no
	// rules files, is installed.
	legacy   func() bool
	readFile func(string) ([]byte, error)
}

func hostSiteLingerOps() siteLingerOps {
	return siteLingerOps{
		installed: func() bool {
			for _, path := range polkitDaemonPaths {
				if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
					return true
				}
			}
			return false
		},
		legacy: func() bool {
			info, err := os.Stat("/usr/lib/policykit-1/polkitd")
			return err == nil && info.Mode().IsRegular()
		},
		readFile: os.ReadFile,
	}
}

// closeSiteLingerWith writes the rules file where polkitd is installed and
// lists the site accounts that linger now.
func closeSiteLingerWith(ops siteLingerOps) *transport.SideDoorsLinger {
	out := &transport.SideDoorsLinger{}
	entries, _, _ := readPasswd(sideDoorsPasswd)
	websiteUIDs := map[int]bool{}
	for _, entry := range entries {
		if hostingpath.IsSiteHome(entry.Home) && entry.UID != 0 {
			websiteUIDs[entry.UID] = true
		}
	}
	if names, err := os.ReadDir(lingerDir); err == nil {
		for _, e := range names {
			for _, entry := range entries {
				if entry.Name == e.Name() && websiteUIDs[entry.UID] {
					out.Accounts = append(out.Accounts, entry.Name)
					break
				}
			}
		}
		sort.Strings(out.Accounts)
	}
	if !ops.installed() {
		out.State = transport.SideDoorsAtNotInstalled
		if ops.legacy != nil && ops.legacy() {
			out.State, out.Detail = transport.SideDoorsAtFailed, "polkit 0.105 is installed; it reads no rules files, so lingering stays open to site accounts on this system"
		}
		return out
	}
	path := filepath.Join(polkitRulesDir, lingerRuleName)
	out.Path = path
	groupText, _ := ops.readFile(sideDoorsGroup)
	out.Groups = existingGroups(string(groupText), lingerAdminGroups)
	ledger := readSandboxLedger()
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		if _, known := ledger[path]; known {
			out.State = transport.SideDoorsAtOwnerRemoved
			return out
		}
	}
	if info, err := os.Stat(polkitRulesDir); err != nil || !info.IsDir() {
		out.State, out.Detail = transport.SideDoorsAtFailed, polkitRulesDir+" is not there"
		return out
	}
	body := lingerRuleText(out.Groups)
	result, err := services.ApplyManagedSiteFile(services.ManagedSiteFile{Kind: lingerRuleKind, Path: path, HeaderPrefix: polkitRulesHead,
		Body: body, Trigger: transport.SiteFileTriggerCreate, RecordedSHA256: ledger[path].RenderSHA256}, nil, nil)
	switch {
	case err != nil:
		out.State, out.Detail = transport.SideDoorsAtFailed, err.Error()
	case result.Outcome == transport.SiteFileOutcomeKept:
		out.State = transport.SideDoorsAtOwnerFile
	case result.Outcome == transport.SiteFileOutcomeUnchanged:
		out.State = transport.SideDoorsAtCurrent
	case result.Outcome == transport.SiteFileOutcomeWritten || result.Outcome == transport.SiteFileOutcomeRecreated:
		out.State = transport.SideDoorsAtWritten
		// polkitd reads its rules as the polkitd account: the file must be
		// readable by it (0644, as the distribution's own rules).
		_ = os.Chmod(path, 0o644)
	default:
		out.State, out.Detail = transport.SideDoorsAtFailed, result.Outcome+" "+result.Reason+" "+result.Detail
	}
	if out.State == transport.SideDoorsAtWritten || out.State == transport.SideDoorsAtCurrent {
		if err := setSandboxLedgerEntry(path, lingerRuleKind, result.RenderSHA256); err != nil {
			log.Printf("side doors: record %s: %v", path, err)
		}
	}
	return out
}

// forgetLingerRule is the explicit re-close of lingering: the record of the
// rules file goes, so a missing file is written as at a first start.
func forgetLingerRule() error {
	path := filepath.Join(polkitRulesDir, lingerRuleName)
	return updateSandboxLedger(func(l map[string]sandboxLedgerEntry) bool {
		if _, ok := l[path]; !ok {
			return false
		}
		delete(l, path)
		return true
	})
}
