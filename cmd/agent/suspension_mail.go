package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// A suspended mail domain is out of Postfix's virtual mailbox domain map, so
// Postfix refuses its mail (relay denied), and each mailbox's Dovecot
// passwd-file line carries nologin=y as its first extra field, so Dovecot
// refuses IMAP, POP3 and SMTP AUTH (Postfix authenticates through Dovecot
// SASL). Mailboxes and maildirs are never deleted. The token goes first in the
// extra fields because UpdateMailQuota rewrites from "userdb_quota_rule=" to
// the end of the line; a token in front of it survives a quota change.
//
// Askıya alınmış posta alan adı Postfix'in sanal posta kutusu alan adı
// haritasından çıkar (posta reddedilir); her posta kutusunun Dovecot
// satırının ilk ek alanı nologin=y olur (IMAP, POP3 ve SMTP girişi
// reddedilir). Hiçbir posta kutusu silinmez.

const suspensionNologinToken = "nologin=y"

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

// mailDomainSuspensionState reads the state of one domain from the two maps'
// contents. Duplicate lines make the state unknown.
func mailDomainSuspensionState(domainsRaw, usersRaw []byte, domain string, mailboxes []string) (transport.MailDomainState, error) {
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

// mailSuspensionPlan is the complete change for one domain; it is written as
// one transaction or not at all.
type mailSuspensionPlan struct {
	domains        []byte
	domainsChanged bool
	users          []byte
	usersChanged   bool
	conflicts      []string
}

// planMailDomainSuspension computes the compare-and-set change. Restore
// re-inserts a missing domain line by appending it to the map. A mailbox the
// before-image does not name (created after inspection) is suspended too, and
// on restore is treated as not refused before: only the token suspension put
// in front of its extra fields is removed, so it cannot stay refused.
func planMailDomainSuspension(
	domainsRaw, usersRaw []byte,
	domain string,
	mailboxes []string,
	suspend bool,
	before transport.MailDomainState,
) (mailSuspensionPlan, error) {
	plan := mailSuspensionPlan{domains: domainsRaw, users: usersRaw}

	domainLines := splitMailFile(domainsRaw)
	indexes, err := suspensionDomainLineIndexes(domainLines, domain)
	if err != nil {
		return mailSuspensionPlan{}, fmt.Errorf("the virtual mailbox domain map: %w", err)
	}
	switch {
	case len(indexes) > 1:
		plan.conflicts = append(plan.conflicts, fmt.Sprintf("the virtual mailbox domain map lists %s %d times", domain, len(indexes)))
	case suspend && len(indexes) == 1:
		line := domainLines[indexes[0]]
		if before.DomainListed && line == before.DomainLine {
			domainLines = append(domainLines[:indexes[0]], domainLines[indexes[0]+1:]...)
			plan.domainsChanged = true
		} else {
			plan.conflicts = append(plan.conflicts, fmt.Sprintf(
				"the virtual mailbox domain map has %q, but the line recorded before suspension was %s",
				line, describeRecordedDomainLine(before)))
		}
	case !suspend && before.DomainListed && len(indexes) == 0:
		domainLines = append(domainLines, before.DomainLine)
		plan.domainsChanged = true
	case !suspend && len(indexes) == 1:
		line := domainLines[indexes[0]]
		if !before.DomainListed || line != before.DomainLine {
			plan.conflicts = append(plan.conflicts, fmt.Sprintf(
				"the virtual mailbox domain map has %q while the domain is suspended, but the line recorded before suspension was %s",
				line, describeRecordedDomainLine(before)))
		}
	}
	if plan.domainsChanged {
		plan.domains = renderMailFile(domainLines)
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
	if plan.domainsChanged {
		plan.domainsChanged = !bytes.Equal(plan.domains, domainsRaw)
	}
	return plan, nil
}

func describeRecordedDomainLine(before transport.MailDomainState) string {
	if !before.DomainListed {
		return "none"
	}
	return fmt.Sprintf("%q", before.DomainLine)
}

// mailDomainStateMeetsTarget: suspended means unlisted and every mailbox with
// a line refused; restored means the recorded line (or none) and each
// mailbox refused exactly as recorded (a mailbox not recorded: not refused).
func mailDomainStateMeetsTarget(state transport.MailDomainState, suspend bool, before transport.MailDomainState) bool {
	if suspend {
		if state.DomainListed {
			return false
		}
		for _, refused := range state.LoginRefused {
			if !refused {
				return false
			}
		}
		return true
	}
	if state.DomainListed != before.DomainListed || (before.DomainListed && state.DomainLine != before.DomainLine) {
		return false
	}
	for mailbox, refused := range state.LoginRefused {
		if refused != before.LoginRefused[mailbox] {
			return false
		}
	}
	return true
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

// readSuspensionMailMaps reads the domain map (missing = empty) and the
// Dovecot users file (missing = empty; an existing one must pass its strict
// metadata contract).
func readSuspensionMailMaps() (domainsRaw []byte, domainsExist bool, usersRaw []byte, usersExist bool, err error) {
	domainsRaw, err = secureReadConfig(postfixDomainsPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		domainsRaw, err = []byte{}, nil
	case err != nil:
		return nil, false, nil, false, fmt.Errorf("the virtual mailbox domain map could not be read: %w", err)
	default:
		domainsExist = true
	}
	usersRaw, usersExist, err = readDovecotUsersFileForMutation(dovecotUsersPath, false)
	if err != nil {
		return nil, false, nil, false, fmt.Errorf("the Dovecot users file could not be read: %w", err)
	}
	return domainsRaw, domainsExist, usersRaw, usersExist, nil
}

func inspectSuspensionMailDomains(domains []transport.SuspensionMailDomain) []transport.MailDomainObservation {
	observations := make([]transport.MailDomainObservation, 0, len(domains))
	if len(domains) == 0 {
		return observations
	}
	domainsRaw, _, usersRaw, _, readErr := readSuspensionMailMaps()
	for _, requested := range domains {
		observation := transport.MailDomainObservation{Domain: requested}
		if readErr != nil {
			observation.Error = readErr.Error()
			observations = append(observations, observation)
			continue
		}
		domain, mailboxes, err := canonicalSuspensionMailDomain(requested)
		if err == nil {
			observation.State, err = mailDomainSuspensionState(domainsRaw, usersRaw, domain, mailboxes)
		}
		if err != nil {
			observation.Error = err.Error()
		}
		observations = append(observations, observation)
	}
	return observations
}

// SetMailDomainSuspensionV1 suspends or restores one mail domain and its
// mailboxes in Postfix's and Dovecot's own maps, in one transaction.
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
	if !mailMutex.TryLock() {
		resp.Error = transport.MailConfigurationBusy
		return nil
	}
	defer mailMutex.Unlock()

	domainsRaw, domainsExist, usersRaw, _, err := readSuspensionMailMaps()
	if err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return nil
	}
	if current, stateErr := mailDomainSuspensionState(domainsRaw, usersRaw, domain, mailboxes); stateErr == nil {
		resp.State = current
	}
	plan, err := planMailDomainSuspension(domainsRaw, usersRaw, domain, mailboxes, req.Suspend, before)
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

	var writes []mailFileWrite
	if plan.domainsChanged {
		writes = append(writes, mailFileWrite{path: postfixDomainsPath, content: plan.domains, mode: 0o644})
	}
	if plan.usersChanged {
		writes = append(writes, mailFileWrite{path: dovecotUsersPath, content: plan.users, mode: 0o640})
	}
	// The domain map is compiled again even when its text is already right:
	// an earlier call may have stopped after writing the text and before
	// postmap, and Postfix reads the compiled index.
	var postmaps []string
	if domainsExist || plan.domainsChanged {
		postmaps = append(postmaps, postfixDomainsPath)
	}
	if len(writes) > 0 || len(postmaps) > 0 {
		ctx, cancel := newMailMutationContext()
		defer cancel()
		if err := applyMailFileMutation(ctx, writes, postmaps, nil); err != nil {
			resp.Error = "the mail maps could not be changed; what was written is put back unless this detail says the rollback failed too: " + err.Error()
			if afterDomains, _, afterUsers, _, readErr := readSuspensionMailMaps(); readErr == nil {
				if after, stateErr := mailDomainSuspensionState(afterDomains, afterUsers, domain, mailboxes); stateErr == nil {
					resp.State = after
				}
			}
			return nil
		}
	}
	resp.Changed = len(writes) > 0

	afterDomains, _, afterUsers, _, err := readSuspensionMailMaps()
	if err != nil {
		resp.Error = "the change finished but the mail maps could not be read back, so the result is unknown: " + err.Error()
		return nil
	}
	after, err := mailDomainSuspensionState(afterDomains, afterUsers, domain, mailboxes)
	if err != nil {
		resp.Error = "the change finished but the mail maps could not be read back, so the result is unknown: " + err.Error()
		return nil
	}
	resp.State = after
	if !mailDomainStateMeetsTarget(after, req.Suspend, before) {
		resp.Error = fmt.Sprintf("the mail maps of %s do not read as the wanted state after the change", domain)
		return nil
	}
	resp.Applied = true
	return nil
}
