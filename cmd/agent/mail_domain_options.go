package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/alicelik/celikpanel/internal/hostcmd"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Per-domain mail service options (alpha.128, gap G25).
//
// One recipient access map, /etc/postfix/celikpanel_domain_mail_options,
// rendered whole from the panel's state and named by "check_recipient_access
// <type>:<map>" at the front of main.cf's smtpd_recipient_restrictions (port
// 25; submission has its own restrictions, so a domain's own mailboxes keep
// sending). Lines:
//
//   - "<domain> 550 5.7.1 …": the domain receives no mail from other servers
//     here (send only);
//   - "<address> DUNNO" for each known address of a domain with its own
//     unknown-address text, then "<domain> 550 5.1.1 <text>": DUNNO stops
//     the lookup for a known address (access(5)), so only unknown addresses
//     get the text.
//
// The map can only refuse or pass on; it never says OK, so it cannot open a
// relay. A map changed on the server (its marker or digest no longer match)
// is kept and reported (D-031); main.cf's element is added at the front only
// while the map has lines and taken off again when it has none, the owner's
// own elements untouched. Every change is one mail file transaction (put back
// when postconf fails) followed by Postfix's verified reload.
//
// Alan adı başına posta seçenekleri: tek alıcı erişim haritası; yalnız
// reddeder ya da geçer, asla OK demez. Sunucuda değişen harita korunur.

var postfixDomainOptionsPath = "/etc/postfix/celikpanel_domain_mail_options"

const domainOptionsMarker = "# Managed by CelikPanel: per-domain mail service options (send only, unknown-address replies)."
const domainOptionsDigestPrefix = "# celikpanel-body-sha256: "

var domainOptionsReplyRe = regexp.MustCompile(`^550 5\.[0-9]\.[0-9] [ -~]{1,200}$`)

func validateDomainOptionsEntry(entry transport.MailDomainOptionsEntry) error {
	key := strings.ToLower(strings.TrimSpace(entry.Key))
	if key != entry.Key || key == "" {
		return errors.New("a map key is not in canonical form")
	}
	if strings.Contains(key, "@") {
		if _, err := transport.CanonicalMailAddress(key); err != nil {
			return errors.New("a map address is not valid")
		}
	} else if _, err := canonicalAgentDomain(key); err != nil {
		return errors.New("a map domain is not valid")
	}
	switch {
	case entry.Action == "DUNNO":
		if !strings.Contains(key, "@") {
			return errors.New("DUNNO is only for a known address")
		}
	case domainOptionsReplyRe.MatchString(entry.Action):
		if strings.Contains(key, "@") {
			return errors.New("a refusal is only for a whole domain")
		}
		if strings.ContainsAny(entry.Action, "\r\n\t") {
			return errors.New("a refusal text holds a control character")
		}
	default:
		return errors.New("an action is neither DUNNO nor a 550 refusal")
	}
	return nil
}

func renderDomainOptionsMap(entries []transport.MailDomainOptionsEntry) []byte {
	sorted := append([]transport.MailDomainOptionsEntry(nil), entries...)
	// Addresses before their domain only matters to a reader: Postfix looks
	// up the full address first whatever the order.
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Key < sorted[j].Key })
	var body strings.Builder
	for _, entry := range sorted {
		body.WriteString(entry.Key + " " + entry.Action + "\n")
	}
	if body.Len() == 0 {
		return []byte(domainOptionsMarker + "\n")
	}
	sum := sha256.Sum256([]byte(body.String()))
	return []byte(domainOptionsMarker + "\n" + domainOptionsDigestPrefix + hex.EncodeToString(sum[:]) + "\n" + body.String())
}

// domainOptionsMapIsPanels reports whether an existing map is CelikPanel's
// unchanged text (or absent).
func domainOptionsMapIsPanels(raw []byte, exists bool) bool {
	if !exists {
		return true
	}
	text := string(raw)
	if text == domainOptionsMarker+"\n" {
		return true
	}
	rest, ok := strings.CutPrefix(text, domainOptionsMarker+"\n"+domainOptionsDigestPrefix)
	if !ok {
		return false
	}
	digest, body, found := strings.Cut(rest, "\n")
	if !found {
		return false
	}
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:]) == digest
}

func domainOptionsMapOperand(text string) bool {
	index := strings.IndexByte(text, ':')
	return index > 0 && text[index+1:] == postfixDomainOptionsPath
}

// planDomainOptionsRestriction keeps "check_recipient_access <type>:<map>"
// at the front of smtpd_recipient_restrictions while the map has lines and
// takes exactly those two elements off when it has none.
func planDomainOptionsRestriction(current string, want bool, mapType string) (suspensionRestrictionPlan, error) {
	elements, ok := splitRecipientRestrictions(current)
	if !ok {
		return suspensionRestrictionPlan{}, errors.New("smtpd_recipient_restrictions has unbalanced braces")
	}
	present := false
	for index := 0; index+1 < len(elements); index++ {
		if elements[index].text == suspensionRecipientAccess && domainOptionsMapOperand(elements[index+1].text) {
			present = true
		}
	}
	element := suspensionRecipientAccess + " " + mapType + ":" + postfixDomainOptionsPath
	trimmed := strings.TrimSpace(current)
	leads := len(elements) >= 2 && elements[0].text == suspensionRecipientAccess && domainOptionsMapOperand(elements[1].text)
	switch {
	case want && !present && trimmed == "":
		return suspensionRestrictionPlan{change: true, value: element}, nil
	case want && !present:
		return suspensionRestrictionPlan{change: true, value: element + ", " + trimmed}, nil
	case !want && leads && len(elements) == 2:
		return suspensionRestrictionPlan{change: true, unset: true}, nil
	case !want && leads:
		at := strings.Index(trimmed, elements[1].text) + len(elements[1].text)
		return suspensionRestrictionPlan{change: true, value: strings.TrimLeft(trimmed[at:], ", \t\r\n")}, nil
	}
	return suspensionRestrictionPlan{}, nil
}

// SetMailDomainOptionsV1 makes Postfix hold the per-domain mail options.
// SetMailDomainOptionsV1 alan adı başına posta seçeneklerini Postfix'e uygular.
func (a *Agent) SetMailDomainOptionsV1(req *transport.SetMailDomainOptionsRequest, resp *transport.SetMailDomainOptionsResponse) error {
	if req == nil || resp == nil {
		return fmt.Errorf("mail domain options request and response are required")
	}
	*resp = transport.SetMailDomainOptionsResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "changing per-domain mail options"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	seen := map[string]bool{}
	for _, entry := range req.Entries {
		if err := validateDomainOptionsEntry(entry); err != nil {
			resp.Error = err.Error() + "; nothing was changed"
			return nil
		}
		if seen[entry.Key] {
			resp.Error = "a map key appears twice; nothing was changed"
			return nil
		}
		seen[entry.Key] = true
	}
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

	if _, err := mailPolicyLookPath("postconf"); err != nil {
		resp.Error = "Postfix is not installed; nothing was changed"
		return nil
	}
	var raw []byte
	var err error
	exists := false
	if _, statErr := os.Lstat(postfixDomainOptionsPath); statErr == nil {
		exists = true
		if raw, err = secureReadConfig(postfixDomainOptionsPath); err != nil {
			resp.Error = "the mail options map could not be read: " + hostcmd.Bounded(err.Error(), 200) + "; nothing was changed"
			return nil
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		resp.Error = "the mail options map could not be read: " + hostcmd.Bounded(statErr.Error(), 200) + "; nothing was changed"
		return nil
	}
	if !domainOptionsMapIsPanels(raw, exists) {
		resp.Owner = true
		resp.Error = postfixDomainOptionsPath + " was changed on the server, so CelikPanel keeps it and does not write the mail options. " +
			"To let CelikPanel write it, the server owner moves the own lines elsewhere, deletes the file and saves the options again"
		return nil
	}
	want := renderDomainOptionsMap(req.Entries)
	current, err := mailPolicyPostconf("-h", suspensionRestrictionsParam)
	if err != nil {
		resp.Error = "smtpd_recipient_restrictions could not be read: " + hostcmd.Diagnostic(current, err) + "; nothing was changed"
		return nil
	}
	restriction, err := planDomainOptionsRestriction(strings.TrimSpace(string(current)), len(req.Entries) > 0, suspensionMapType())
	if err != nil {
		resp.Error = err.Error() + "; nothing was changed. The server owner corrects main.cf, then the options are saved again"
		return nil
	}
	changed := (exists && string(raw) != string(want)) || (!exists && len(req.Entries) > 0)
	var writes []mailFileWrite
	if changed {
		writes = append(writes, mailFileWrite{path: postfixDomainOptionsPath, content: want, mode: 0o644})
	}
	var afterWrite func() (func() error, error)
	if restriction.change {
		afterWrite = func() (func() error, error) { return nil, applySuspensionRestriction(restriction) }
	}
	ctx, cancel := newMailMutationContext()
	defer cancel()
	// The map is compiled every time: an earlier call may have stopped after
	// writing the text and before postmap.
	var postmaps []string
	if exists || changed {
		postmaps = []string{postfixDomainOptionsPath}
	}
	if err := applyMailFileMutation(ctx, writes, postmaps, afterWrite); err != nil {
		resp.Error = "the mail options could not be written; what was written is put back unless this says the rollback failed too: " + err.Error()
		return nil
	}
	resp.Changed = changed || restriction.change
	if _, err := mailPolicyReload(); err != nil {
		log.Printf("mail domain options: postfix reload failed: %v", err)
		resp.Error = "the mail options are written, but Postfix could not be reloaded, so it may not apply them yet: " +
			hostcmd.Bounded(strings.Join(strings.Fields(err.Error()), " "), 300) + ". The administrator checks Postfix (postfix check), then the options are saved again"
		return nil
	}
	resp.Applied = true
	return nil
}
