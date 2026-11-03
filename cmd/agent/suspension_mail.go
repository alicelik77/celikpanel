package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/alicelik/celikpanel/internal/hostcmd"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A suspended mail domain stays in Postfix's virtual mailbox domain map and
// gets one line in CelikPanel's suspended recipient map,
// "<domain> 450 4.7.1 Mailbox temporarily unavailable". smtpd_recipient_
// restrictions starts with "check_recipient_access hash:<that map>" while the
// map has a line, so Postfix answers the domain's mail with a temporary 450:
// senders keep it queued and retry, and nothing is lost when the subscription
// resumes within days. Each mailbox's Dovecot passwd-file line carries
// nologin=y as its first extra field, so Dovecot refuses IMAP, POP3 and SMTP
// AUTH. Mailboxes and maildirs are never deleted. The token goes first in the
// extra fields because UpdateMailQuota rewrites from "userdb_quota_rule=" to
// the end of the line; a token in front of it survives a quota change.
//
// A resume removes the map line; when the map has no line left, the managed
// restriction element is removed again and main.cf is back to the owner's
// value. A restriction element the owner moved is left alone (with an empty
// map it changes nothing).
//
// Askıya alınmış posta alan adı haritada kalır; CelikPanel'in askı haritasına
// "450 4.7.1" satırı eklenir, Postfix postayı geçici olarak reddeder ve
// gönderen yeniden dener. Her posta kutusunun Dovecot satırının ilk ek alanı
// nologin=y olur. Hiçbir posta kutusu silinmez.

const (
	suspensionNologinToken      = "nologin=y"
	suspensionMailDeferReply    = "450 4.7.1 Mailbox temporarily unavailable"
	suspensionRecipientAccess   = "check_recipient_access"
	suspensionRestrictionsParam = "smtpd_recipient_restrictions"
)

// postfixSuspendedRecipientsPath is the suspended recipient map, compiled
// with this server's indexed map type like CelikPanel's other maps.
var (
	postfixSuspendedRecipientsPath = "/etc/postfix/celikpanel_suspended_recipients"
	suspensionMapType              = func() string { return postfixMapTypeContext(context.Background()) }
)

func suspensionDeferLine(domain string) string {
	return domain + " " + suspensionMailDeferReply
}

// suspensionRestrictionPrefix is the managed element; mapType is Postfix's
// indexed map type on this server (hash, lmdb, ...).
func suspensionRestrictionPrefix(mapType string) string {
	return suspensionRecipientAccess + " " + mapType + ":" + postfixSuspendedRecipientsPath
}

// suspensionMapOperand: a restriction operand that names our map, any type.
func suspensionMapOperand(text string) bool {
	index := strings.IndexByte(text, ':')
	return index > 0 && text[index+1:] == postfixSuspendedRecipientsPath
}

// dovecotUsersExtraIndex returns where the extra fields of a passwd-file line
// start (just after the 7th ':'; user:pass:uid:gid:gecos:home:shell:extra),
// or -1 when the line has fewer fields.
func dovecotUsersExtraIndex(line string) int {
	colons := 0
	for index := 0; index < len(line); index++ {
		if line[index] != ':' {
			continue
		}
		colons++
		if colons == 7 {
			return index + 1
		}
	}
	return -1
}

// dovecotUsersLineRefusesLogin: the line's extra fields carry a nologin field
// (anywhere, in any form; Dovecot honours them all).
func dovecotUsersLineRefusesLogin(line string) bool {
	index := dovecotUsersExtraIndex(line)
	if index < 0 {
		return false
	}
	for _, field := range strings.Fields(line[index:]) {
		key, _, _ := strings.Cut(field, "=")
		if key == "nologin" {
			return true
		}
	}
	return false
}

// addSuspensionNologin inserts the token at the start of the extra fields.
func addSuspensionNologin(line string) (string, error) {
	index := dovecotUsersExtraIndex(line)
	if index < 0 {
		return "", errors.New("unrecognised passwd-file line")
	}
	extra := line[index:]
	if extra == "" {
		return line + suspensionNologinToken, nil
	}
	return line[:index] + suspensionNologinToken + " " + extra, nil
}

// removeSuspensionNologin is the exact inverse of addSuspensionNologin. It
// removes the token only where suspension puts it; removed=false otherwise.
func removeSuspensionNologin(line string) (string, bool, error) {
	index := dovecotUsersExtraIndex(line)
	if index < 0 {
		return "", false, errors.New("unrecognised passwd-file line")
	}
	extra := line[index:]
	if extra == suspensionNologinToken {
		return line[:index], true, nil
	}
	if strings.HasPrefix(extra, suspensionNologinToken+" ") {
		return line[:index] + extra[len(suspensionNologinToken)+1:], true, nil
	}
	return line, false, nil
}

// suspensionDomainLineIndexes lists the domain map lines whose key is domain.
func suspensionDomainLineIndexes(lines []string, domain string) ([]int, error) {
	var indexes []int
	for index, line := range lines {
		belongs, err := mailDomainLineBelongsTo(line, domain, mailDomainPostfixDomains)
		if err != nil {
			return nil, err
		}
		if belongs {
			indexes = append(indexes, index)
		}
	}
	return indexes, nil
}

// suspensionUsersLineIndexes lists the passwd-file lines of one mailbox.
func suspensionUsersLineIndexes(lines []string, mailbox string) []int {
	var indexes []int
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, _, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		email, _, _, err := canonicalAgentMailbox(key)
		if err == nil && email == mailbox {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

// mailDomainSuspensionState reads the state of one domain from the domain
// map, the suspended recipient map and the users file. Duplicate lines make
// the state unknown.
func mailDomainSuspensionState(domainsRaw, deferRaw, usersRaw []byte, domain string, mailboxes []string) (transport.MailDomainState, error) {
	state := transport.MailDomainState{LoginRefused: map[string]bool{}}
	domainLines := splitMailFile(domainsRaw)
	indexes, err := suspensionDomainLineIndexes(domainLines, domain)
	if err != nil {
		return transport.MailDomainState{}, fmt.Errorf("the virtual mailbox domain map: %w", err)
	}
	if len(indexes) > 1 {
		return transport.MailDomainState{}, fmt.Errorf("the virtual mailbox domain map lists %s %d times", domain, len(indexes))
	}
	if len(indexes) == 1 {
		state.DomainListed = true
		state.DomainLine = domainLines[indexes[0]]
	}
	deferIndexes, err := suspensionDomainLineIndexes(splitMailFile(deferRaw), domain)
	if err != nil {
		return transport.MailDomainState{}, fmt.Errorf("the suspended recipient map: %w", err)
	}
	if len(deferIndexes) > 1 {
		return transport.MailDomainState{}, fmt.Errorf("the suspended recipient map lists %s %d times", domain, len(deferIndexes))
	}
	state.MailDeferred = len(deferIndexes) == 1
	userLines := splitMailFile(usersRaw)
	for _, mailbox := range mailboxes {
		lineIndexes := suspensionUsersLineIndexes(userLines, mailbox)
		if len(lineIndexes) > 1 {
			return transport.MailDomainState{}, fmt.Errorf("the Dovecot users file lists %s %d times", mailbox, len(lineIndexes))
		}
		if len(lineIndexes) == 1 {
			state.LoginRefused[mailbox] = dovecotUsersLineRefusesLogin(userLines[lineIndexes[0]])
		}
	}
	return state, nil
}

// mailSuspensionPlan is the complete file change for one domain; it is
// written as one transaction or not at all. The domain map is not changed.
type mailSuspensionPlan struct {
	deferral        []byte
	deferralChanged bool
	users           []byte
	usersChanged    bool
	conflicts       []string
}

// planMailDomainSuspension computes the compare-and-set change. Suspend adds
// the domain's 450 line to the suspended recipient map; restore removes only
// that exact line. A mailbox the before-image does not name (created after
// inspection) is suspended too, and on restore is treated as not refused
// before: only the token suspension put in front of its extra fields is
// removed, so it cannot stay refused.
func planMailDomainSuspension(
	deferRaw, usersRaw []byte,
	domain string,
	mailboxes []string,
	suspend bool,
	before transport.MailDomainState,
) (mailSuspensionPlan, error) {
	plan := mailSuspensionPlan{deferral: deferRaw, users: usersRaw}

	deferLines := splitMailFile(deferRaw)
	indexes, err := suspensionDomainLineIndexes(deferLines, domain)
	if err != nil {
		return mailSuspensionPlan{}, fmt.Errorf("the suspended recipient map: %w", err)
	}
	ours := suspensionDeferLine(domain)
	switch {
	case len(indexes) > 1:
		plan.conflicts = append(plan.conflicts, fmt.Sprintf("the suspended recipient map lists %s %d times", domain, len(indexes)))
	case len(indexes) == 1 && deferLines[indexes[0]] != ours:
		plan.conflicts = append(plan.conflicts, fmt.Sprintf(
			"the suspended recipient map has %q, not the line CelikPanel writes (%q)", deferLines[indexes[0]], ours))
	case suspend && len(indexes) == 0:
		deferLines = append(deferLines, ours)
		plan.deferralChanged = true
	case !suspend && !before.MailDeferred && len(indexes) == 1:
		deferLines = append(deferLines[:indexes[0]], deferLines[indexes[0]+1:]...)
		plan.deferralChanged = true
	}
	if plan.deferralChanged {
		plan.deferral = renderMailFile(deferLines)
		plan.deferralChanged = !bytes.Equal(plan.deferral, deferRaw)
	}

	userLines := splitMailFile(usersRaw)
	usersChanged := false
	for _, mailbox := range mailboxes {
		lineIndexes := suspensionUsersLineIndexes(userLines, mailbox)
		if len(lineIndexes) == 0 {
			continue // no users line: nothing can log in as it
		}
		if len(lineIndexes) > 1 {
			plan.conflicts = append(plan.conflicts, fmt.Sprintf("the Dovecot users file lists %s %d times", mailbox, len(lineIndexes)))
			continue
		}
		index := lineIndexes[0]
		line := userLines[index]
		refused := dovecotUsersLineRefusesLogin(line)
		wasRefused := before.LoginRefused[mailbox]
		if suspend {
			if refused {
				continue
			}
			if wasRefused {
				plan.conflicts = append(plan.conflicts, fmt.Sprintf(
					"%s no longer carries the nologin field it had before suspension", mailbox))
				continue
			}
			next, addErr := addSuspensionNologin(line)
			if addErr != nil {
				plan.conflicts = append(plan.conflicts, fmt.Sprintf("the Dovecot users line of %s is not in the expected form", mailbox))
				continue
			}
			userLines[index] = next
			usersChanged = true
			continue
		}
		if refused == wasRefused {
			continue
		}
		if wasRefused {
			plan.conflicts = append(plan.conflicts, fmt.Sprintf(
				"%s had the nologin field before suspension but no longer carries it", mailbox))
			continue
		}
		next, removed, removeErr := removeSuspensionNologin(line)
		if removeErr != nil || !removed || dovecotUsersLineRefusesLogin(next) {
			plan.conflicts = append(plan.conflicts, fmt.Sprintf(
				"%s carries a nologin field that suspension did not put there", mailbox))
			continue
		}
		userLines[index] = next
		usersChanged = true
	}
	if usersChanged {
		plan.users = renderMailFile(userLines)
		plan.usersChanged = !bytes.Equal(plan.users, usersRaw)
	}
	return plan, nil
}

// mailDomainStateMeetsTarget: suspended means the 450 line is present and
// every mailbox with a line is refused; restored means the line is as
// recorded (normally absent) and each mailbox refused exactly as recorded (a
// mailbox not recorded: not refused). The domain map is not part of either.
func mailDomainStateMeetsTarget(state transport.MailDomainState, suspend bool, before transport.MailDomainState) bool {
	if suspend {
		if !state.MailDeferred {
			return false
		}
		for _, refused := range state.LoginRefused {
			if !refused {
				return false
			}
		}
		return true
	}
	if state.MailDeferred != before.MailDeferred {
		return false
	}
	for mailbox, refused := range state.LoginRefused {
		if refused != before.LoginRefused[mailbox] {
			return false
		}
	}
	return true
}

// suspensionRestrictionPlan is the main.cf change for the managed
// check_recipient_access element: set (postconf -e) or unset (postconf -X).
type suspensionRestrictionPlan struct {
	change bool
	unset  bool
	value  string
}

// planSuspensionRestriction keeps the managed element at the front of
// smtpd_recipient_restrictions while the map has a line (want) and takes
// exactly that prefix off again when it has none. The owner's own elements
// are never reordered or rewritten. An element the owner moved elsewhere is
// left as it is.
func planSuspensionRestriction(current string, want bool, mapType string) (suspensionRestrictionPlan, error) {
	elements, ok := splitRecipientRestrictions(current)
	if !ok {
		return suspensionRestrictionPlan{}, errors.New("smtpd_recipient_restrictions has unbalanced braces")
	}
	present := false
	for index := 0; index+1 < len(elements); index++ {
		if elements[index].text == suspensionRecipientAccess && suspensionMapOperand(elements[index+1].text) {
			present = true
		}
	}
	trimmed := strings.TrimSpace(current)
	leads := len(elements) >= 2 && elements[0].text == suspensionRecipientAccess && suspensionMapOperand(elements[1].text)
	switch {
	case want && !present && trimmed == "":
		return suspensionRestrictionPlan{change: true, value: suspensionRestrictionPrefix(mapType)}, nil
	case want && !present:
		return suspensionRestrictionPlan{change: true, value: suspensionRestrictionPrefix(mapType) + ", " + trimmed}, nil
	case !want && leads && len(elements) == 2:
		// The default value is empty; unsetting leaves main.cf as before.
		return suspensionRestrictionPlan{change: true, unset: true}, nil
	case !want && leads:
		// Only the two leading elements go; the owner's text after them stays.
		at := strings.Index(trimmed, elements[1].text) + len(elements[1].text)
		return suspensionRestrictionPlan{change: true, value: strings.TrimLeft(trimmed[at:], ", \t\r\n")}, nil
	}
	return suspensionRestrictionPlan{}, nil
}

func suspensionMapHasLines(raw []byte) bool {
	for _, line := range splitMailFile(raw) {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return true
		}
	}
	return false
}

func applySuspensionRestriction(plan suspensionRestrictionPlan) error {
	if !plan.change {
		return nil
	}
	args := []string{"-e", suspensionRestrictionsParam + "=" + plan.value}
	if plan.unset {
		args = []string{"-X", suspensionRestrictionsParam}
	}
	if out, err := mailPolicyPostconf(args...); err != nil {
		return fmt.Errorf("postconf could not change %s: %s", suspensionRestrictionsParam, hostcmd.Diagnostic(out, err))
	}
	return nil
}

// canonicalSuspensionMailDomain canonicalizes the domain and its mailboxes;
// every mailbox must belong to the domain. Mailboxes come back sorted, once.
func canonicalSuspensionMailDomain(raw transport.SuspensionMailDomain) (string, []string, error) {
	domain, err := canonicalAgentDomain(raw.Domain)
	if err != nil {
		return "", nil, fmt.Errorf("invalid mail domain: %w", err)
	}
	seen := make(map[string]struct{}, len(raw.Mailboxes))
	mailboxes := make([]string, 0, len(raw.Mailboxes))
	for _, rawMailbox := range raw.Mailboxes {
		email, _, mailboxDomain, err := canonicalAgentMailbox(rawMailbox)
		if err != nil {
			return "", nil, fmt.Errorf("invalid mailbox: %w", err)
		}
		if mailboxDomain != domain {
			return "", nil, fmt.Errorf("mailbox %s does not belong to %s", email, domain)
		}
		if _, duplicate := seen[email]; duplicate {
			continue
		}
		seen[email] = struct{}{}
		mailboxes = append(mailboxes, email)
	}
	sort.Strings(mailboxes)
	return domain, mailboxes, nil
}

// canonicalSuspensionMailBefore checks the before-image: the recorded domain
// line must be one line of this domain (it may be written back), and every
// recorded mailbox must belong to it.
func canonicalSuspensionMailBefore(domain string, before transport.MailDomainState) (transport.MailDomainState, error) {
	out := transport.MailDomainState{
		DomainListed: before.DomainListed,
		DomainLine:   before.DomainLine,
		LoginRefused: make(map[string]bool, len(before.LoginRefused)),
	}
	if before.DomainListed {
		if strings.ContainsAny(before.DomainLine, "\r\n\x00") {
			return transport.MailDomainState{}, errors.New("the recorded domain map line is not one line")
		}
		belongs, err := mailDomainLineBelongsTo(before.DomainLine, domain, mailDomainPostfixDomains)
		if err != nil || !belongs {
			return transport.MailDomainState{}, fmt.Errorf("the recorded domain map line is not a line of %s", domain)
		}
	} else if before.DomainLine != "" {
		return transport.MailDomainState{}, errors.New("the recorded state has a domain map line but says the domain was not listed")
	}
	for rawMailbox, refused := range before.LoginRefused {
		email, _, mailboxDomain, err := canonicalAgentMailbox(rawMailbox)
		if err != nil || mailboxDomain != domain {
			return transport.MailDomainState{}, fmt.Errorf("the recorded state names a mailbox that is not of %s", domain)
		}
		out.LoginRefused[email] = refused
	}
	return out, nil
}

// suspensionMailMaps is what one call reads: the domain map, the suspended
// recipient map (missing = empty) and the Dovecot users file (missing =
// empty; an existing one must pass its strict metadata contract).
type suspensionMailMaps struct {
	domains, deferral, users []byte
	deferralExists           bool
}

func readSuspensionMailMaps() (suspensionMailMaps, error) {
	var maps suspensionMailMaps
	var err error
	maps.domains, err = secureReadConfig(postfixDomainsPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		maps.domains = []byte{}
	case err != nil:
		return maps, fmt.Errorf("the virtual mailbox domain map could not be read: %w", err)
	}
	maps.deferral, err = secureReadConfig(postfixSuspendedRecipientsPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		maps.deferral = []byte{}
	case err != nil:
		return maps, fmt.Errorf("the suspended recipient map could not be read: %w", err)
	default:
		maps.deferralExists = true
	}
	maps.users, _, err = readDovecotUsersFileForMutation(dovecotUsersPath, false)
	if err != nil {
		return maps, fmt.Errorf("the Dovecot users file could not be read: %w", err)
	}
	return maps, nil
}

func (m suspensionMailMaps) state(domain string, mailboxes []string) (transport.MailDomainState, error) {
	return mailDomainSuspensionState(m.domains, m.deferral, m.users, domain, mailboxes)
}

func inspectSuspensionMailDomains(domains []transport.SuspensionMailDomain) []transport.MailDomainObservation {
	observations := make([]transport.MailDomainObservation, 0, len(domains))
	if len(domains) == 0 {
		return observations
	}
	maps, readErr := readSuspensionMailMaps()
	for _, requested := range domains {
		observation := transport.MailDomainObservation{Domain: requested}
		if readErr != nil {
			observation.Error = readErr.Error()
			observations = append(observations, observation)
			continue
		}
		domain, mailboxes, err := canonicalSuspensionMailDomain(requested)
		if err == nil {
			observation.State, err = maps.state(domain, mailboxes)
		}
		if err != nil {
			observation.Error = err.Error()
		}
		observations = append(observations, observation)
	}
	return observations
}

// SetMailDomainSuspensionV1 suspends or restores one mail domain and its
// mailboxes in Postfix's and Dovecot's own maps, in one transaction, and
// keeps the managed recipient restriction in main.cf in step with the map.
//
// SetMailDomainSuspensionV1 bir posta alan adını ve posta kutularını
// Postfix ve Dovecot'un kendi haritalarında tek işlemle askıya alır ya da
// geri verir.
func (a *Agent) SetMailDomainSuspensionV1(
	req *transport.SetMailDomainSuspensionRequest,
	resp *transport.SetMailDomainSuspensionResponse,
) error {
	if req == nil || resp == nil {
		return fmt.Errorf("mail domain suspension request and response are required")
	}
	*resp = transport.SetMailDomainSuspensionResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "suspending or restoring a mail domain"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	domain, mailboxes, err := canonicalSuspensionMailDomain(req.Domain)
	if err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return nil
	}
	before, err := canonicalSuspensionMailBefore(domain, req.Before)
	if err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return nil
	}

	// TryLock, like UpdateMailPassword: a call must not queue behind another
	// mail change and apply after its Panel caller gave up. The Panel retries.
	// main.cf is shared with the mail policy, so its lock is taken too.
	if !mailMutex.TryLock() {
		resp.Error = transport.MailConfigurationBusy
		return nil
	}
	defer mailMutex.Unlock()
	if !mailPolicyMu.TryLock() {
		resp.Error = transport.MailConfigurationBusy
		return nil
	}
	defer mailPolicyMu.Unlock()

	maps, err := readSuspensionMailMaps()
	if err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return nil
	}
	if current, stateErr := maps.state(domain, mailboxes); stateErr == nil {
		resp.State = current
	}
	plan, err := planMailDomainSuspension(maps.deferral, maps.users, domain, mailboxes, req.Suspend, before)
	if err != nil {
		resp.Conflict = true
		resp.Error = err.Error() + "; nothing was changed. The server owner corrects the map, then the operation can be retried"
		return nil
	}
	if len(plan.conflicts) > 0 {
		resp.Conflict = true
		resp.Error = fmt.Sprintf(
			"mail of %s was changed outside CelikPanel: %s; nothing was changed. The server owner decides which state is right, "+
				"then the operation can be retried",
			domain, strings.Join(plan.conflicts, "; "),
		)
		return nil
	}

	// main.cf is read before anything is written: a write is never built
	// from a failed read.
	if _, err := mailPolicyLookPath("postconf"); err != nil {
		resp.Error = "Postfix's postconf was not found, so the mail refusal cannot be set; nothing was changed. " +
			"The administrator installs or repairs Postfix, then the operation can be retried"
		return nil
	}
	currentRestrictions, err := mailPolicyPostconf("-h", suspensionRestrictionsParam)
	if err != nil {
		resp.Error = "smtpd_recipient_restrictions could not be read: " + hostcmd.Diagnostic(currentRestrictions, err) + "; nothing was changed"
		return nil
	}
	restriction, err := planSuspensionRestriction(strings.TrimSpace(string(currentRestrictions)), suspensionMapHasLines(plan.deferral), suspensionMapType())
	if err != nil {
		resp.Conflict = true
		resp.Error = err.Error() + "; nothing was changed. The server owner corrects main.cf, then the operation can be retried"
		return nil
	}

	var writes []mailFileWrite
	if plan.deferralChanged {
		writes = append(writes, mailFileWrite{path: postfixSuspendedRecipientsPath, content: plan.deferral, mode: 0o644})
	}
	if plan.usersChanged {
		writes = append(writes, mailFileWrite{path: dovecotUsersPath, content: plan.users, mode: 0o640})
	}
	// The map is compiled again even when its text is already right: an
	// earlier call may have stopped after writing the text and before
	// postmap, and Postfix reads the compiled index.
	var postmaps []string
	if maps.deferralExists || plan.deferralChanged {
		postmaps = append(postmaps, postfixSuspendedRecipientsPath)
	}
	// The main.cf change comes after the maps, inside the same transaction:
	// if postconf fails, the map and users files are put back.
	var afterWrite func() (func() error, error)
	if restriction.change {
		afterWrite = func() (func() error, error) {
			return nil, applySuspensionRestriction(restriction)
		}
	}
	if len(writes) > 0 || len(postmaps) > 0 || afterWrite != nil {
		ctx, cancel := newMailMutationContext()
		defer cancel()
		if err := applyMailFileMutation(ctx, writes, postmaps, afterWrite); err != nil {
			resp.Error = "the mail maps could not be changed; what was written is put back unless this detail says the rollback failed too: " + err.Error()
			if after, readErr := readSuspensionMailMaps(); readErr == nil {
				if state, stateErr := after.state(domain, mailboxes); stateErr == nil {
					resp.State = state
				}
			}
			return nil
		}
	}
	resp.Changed = len(writes) > 0 || restriction.change

	// A verified reload makes a running Postfix take main.cf. It runs on every
	// call, so a retry after a lost reload does not skip it; a map-only
	// change alone would need none, Postfix rereads hash maps by itself. A
	// stopped Postfix stays stopped.
	if _, err := mailPolicyReload(); err != nil {
		log.Printf("mail suspension of %s: postfix reload failed: %v", domain, err)
		resp.Error = "the mail maps of " + domain + " are written, but Postfix could not be reloaded, so it may not refuse or accept the domain's mail yet: " +
			hostcmd.Bounded(strings.Join(strings.Fields(err.Error()), " "), 300) +
			". The administrator checks Postfix (postfix check), then the operation is retried"
		return nil
	}

	after, err := readSuspensionMailMaps()
	if err != nil {
		resp.Error = "the change finished but the mail maps could not be read back, so the result is unknown: " + err.Error()
		return nil
	}
	state, err := after.state(domain, mailboxes)
	if err != nil {
		resp.Error = "the change finished but the mail maps could not be read back, so the result is unknown: " + err.Error()
		return nil
	}
	resp.State = state
	if !mailDomainStateMeetsTarget(state, req.Suspend, before) {
		resp.Error = fmt.Sprintf("the mail maps of %s do not read as the wanted state after the change", domain)
		return nil
	}
	resp.Applied = true
	return nil
}
