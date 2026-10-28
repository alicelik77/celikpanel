package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Setup wizard v2, phase 2 (alpha.112): the main domain's existing public
// records the owner imports into the setup's zone, and the main domain's
// mail records. Imported and mail records are the owner's records in the
// zone (editable in Domains, never locked); only the setup's own server
// addresses stay locked. MX is written only when no old MX is imported or the
// owner chose to move mail now; SPF is merged with an imported one; DMARC is
// kept or added as the owner chose. DKIM is a step after the mail profile
// (the key exists only once mail is installed).
// Iceri alinan genel kayitlar ve ana alan adinin posta kayitlari.

const serverSetupMaxImportedRecords = 64

type serverSetupImportedRecord struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	Priority int    `json:"priority,omitempty"`
	Decision string `json:"decision"`
}

// serverSetupMailDNS is the owner's choice for the main domain's mail
// records: MX "now" moves incoming mail to this server at once, "later"
// keeps an imported old MX; DMARC "keep" keeps an imported policy, "add"
// writes the recommended one. SPF is always merged.
type serverSetupMailDNS struct {
	MX    string `json:"mx"`
	DMARC string `json:"dmarc"`
}

var serverSetupRecordLabel = regexp.MustCompile(`^[a-z0-9_](?:[a-z0-9_-]{0,61}[a-z0-9_])?$`)

func serverSetupRecordName(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
	if value == "" || len(value) > 253 {
		return "", false
	}
	for _, label := range strings.Split(value, ".") {
		if !serverSetupRecordLabel.MatchString(label) {
			return "", false
		}
	}
	return value, true
}

func canonicalServerSetupRecordContent(recordType, content string) (string, bool) {
	content = strings.TrimSpace(content)
	switch recordType {
	case "A":
		ip, ok := canonicalIPv4(content)
		return ip, ok
	case "AAAA":
		ip := net.ParseIP(content)
		if ip == nil || ip.To4() != nil {
			return "", false
		}
		return ip.String(), true
	case "CNAME", "MX":
		host, ok := serverSetupRecordName(content)
		return host, ok
	case "TXT":
		if content == "" || len(content) > 2048 || strings.ContainsAny(content, "\x00\r\n") {
			return "", false
		}
		return content, true
	case "CAA":
		fields := strings.Fields(content)
		if len(fields) < 3 || len(content) > 512 || !stringIn(fields[1], "issue", "issuewild", "iodef") {
			return "", false
		}
		return content, true
	}
	return "", false
}

// canonicalServerSetupMailAnswers checks the imported records and the mail
// DNS choice. Records outside the main domain, NS/SOA records and an apex
// CNAME are refused: they are not the owner's ordinary records to import.
func canonicalServerSetupMailAnswers(d *serverSetupDraft) error {
	if len(d.ImportedRecords) > serverSetupMaxImportedRecords {
		return errors.New("too many imported records")
	}
	seen := map[string]bool{}
	records := []serverSetupImportedRecord{}
	for _, record := range d.ImportedRecords {
		record.Type = strings.ToUpper(strings.TrimSpace(record.Type))
		name, ok := serverSetupRecordName(record.Name)
		if !ok || d.MainDomain == "" || !serverSetupDNSNameWithin(name, d.MainDomain) {
			return errors.New("an imported record must be a name under the main domain")
		}
		if !stringIn(record.Type, "A", "AAAA", "CNAME", "MX", "TXT", "CAA") || (record.Type == "CNAME" && name == d.MainDomain) {
			return errors.New("unsupported imported record")
		}
		content, ok := canonicalServerSetupRecordContent(record.Type, record.Content)
		if !ok || record.Priority < 0 || record.Priority > 65535 || (record.Type != "MX" && record.Priority != 0) {
			return errors.New("invalid imported record content")
		}
		if !stringIn(record.Decision, "import", "skip") {
			return errors.New("each imported record needs a decision")
		}
		record.Name, record.Content = name, content
		key := record.Name + "\x00" + record.Type + "\x00" + record.Content
		if seen[key] {
			continue
		}
		seen[key] = true
		records = append(records, record)
	}
	slices.SortFunc(records, func(a, b serverSetupImportedRecord) int {
		return strings.Compare(a.Name+"\x00"+a.Type+"\x00"+a.Content, b.Name+"\x00"+b.Type+"\x00"+b.Content)
	})
	d.ImportedRecords = records
	if len(records) == 0 {
		d.ImportedRecords = nil
	}
	if d.MailDNS != nil {
		d.MailDNS.MX, d.MailDNS.DMARC = strings.TrimSpace(d.MailDNS.MX), strings.TrimSpace(d.MailDNS.DMARC)
		if !stringIn(d.MailDNS.MX, "", "now", "later") || !stringIn(d.MailDNS.DMARC, "", "keep", "add") {
			return errors.New("invalid mail DNS choice")
		}
		if d.MailDNS.MX == "" && d.MailDNS.DMARC == "" {
			d.MailDNS = nil
		}
	}
	return nil
}

func serverSetupImported(d serverSetupDraft) []serverSetupImportedRecord {
	out := []serverSetupImportedRecord{}
	for _, record := range d.ImportedRecords {
		if record.Decision == "import" {
			out = append(out, record)
		}
	}
	return out
}

func isSPF(record serverSetupImportedRecord, domain string) bool {
	return record.Type == "TXT" && record.Name == domain && strings.HasPrefix(strings.ToLower(record.Content), "v=spf1")
}

func isDMARC(record serverSetupImportedRecord, domain string) bool {
	return record.Type == "TXT" && record.Name == "_dmarc."+domain && strings.HasPrefix(strings.ToUpper(record.Content), "V=DMARC1")
}

// mergeServerSetupSPF adds this mail server to an SPF policy: the mechanism
// a:<mail host> before the final "all" (or "redirect"), keeping every other
// mechanism. Without an old policy it is "v=spf1 mx a:<mail host> ~all".
func mergeServerSetupSPF(old, mailHost string) string {
	mechanism := "a:" + mailHost
	old = strings.TrimSpace(old)
	if old == "" {
		return "v=spf1 mx " + mechanism + " ~all"
	}
	fields := strings.Fields(old)
	for _, field := range fields {
		if strings.EqualFold(field, mechanism) {
			return strings.Join(fields, " ")
		}
	}
	insert := len(fields)
	for index, field := range fields {
		lower := strings.ToLower(field)
		if strings.HasSuffix(lower, "all") && len(lower) <= 4 || strings.HasPrefix(lower, "redirect=") {
			insert = index
			break
		}
	}
	merged := append(append(append([]string{}, fields[:insert]...), mechanism), fields[insert:]...)
	return strings.Join(merged, " ")
}

func serverSetupHasMail(d serverSetupDraft) bool {
	return d.MainDomain != "" && d.MailHostname != "" && len(serverSetupMailProfileIDs(d)) > 0
}

// serverSetupMailDNSValues are the main domain's mail records the setup
// proposes, as plain values (TXT unquoted): MX unless an old MX is kept, the
// merged SPF, and DMARC unless an old one is kept. DKIM follows in its step.
type serverSetupMailRecord struct {
	Name, Type, Value string
	Priority          int
}

func serverSetupMailDNSValues(d serverSetupDraft, oldSPF string, oldMX, oldDMARC bool) []serverSetupMailRecord {
	if !serverSetupHasMail(d) {
		return nil
	}
	choice := serverSetupMailDNS{}
	if d.MailDNS != nil {
		choice = *d.MailDNS
	}
	records := []serverSetupMailRecord{}
	if !oldMX || choice.MX == "now" {
		records = append(records, serverSetupMailRecord{Name: d.MainDomain, Type: "MX", Value: d.MailHostname, Priority: 10})
	}
	records = append(records, serverSetupMailRecord{Name: d.MainDomain, Type: "TXT", Value: mergeServerSetupSPF(oldSPF, d.MailHostname)})
	if !oldDMARC || choice.DMARC == "add" {
		records = append(records, serverSetupMailRecord{Name: "_dmarc." + d.MainDomain, Type: "TXT", Value: dmarcRecommended(d.MainDomain, "none")})
	}
	return records
}

// serverSetupImportedAndMailDesired adds, to the setup zone, the imported
// records the owner kept and the mail records. Its records carry an origin,
// so they are never locked as the setup's own addresses.
func serverSetupImportedAndMailDesired(d serverSetupDraft, own map[string]bool) ([]serverSetupInfrastructureDNSRecord, error) {
	domain := d.MainDomain
	imported := serverSetupImported(d)
	choice := serverSetupMailDNS{}
	if d.MailDNS != nil {
		choice = *d.MailDNS
	}
	mail := serverSetupHasMail(d)
	oldSPF, oldMX, oldDMARC := "", false, false
	records := []serverSetupInfrastructureDNSRecord{}
	for _, record := range imported {
		if own[record.Name] && stringIn(record.Type, "A", "AAAA", "CNAME") {
			return nil, &serverSetupInfrastructureDNSPlanError{Code: "infrastructure_dns_imported_clash", Message: fmt.Sprintf("the imported %s record of %s uses a name this setup gives to a server; choose another name or skip the record", record.Type, record.Name)}
		}
		if mail && isSPF(record, domain) {
			oldSPF = record.Content
			continue
		}
		if mail && record.Type == "MX" && record.Name == domain {
			oldMX = true
			if choice.MX == "now" {
				continue
			}
		}
		if mail && isDMARC(record, domain) {
			oldDMARC = true
			if choice.DMARC == "add" {
				continue
			}
		}
		content := record.Content
		if record.Type == "TXT" {
			content = splitTXTContent(content)
		}
		records = append(records, serverSetupInfrastructureDNSRecord{Name: record.Name, Type: record.Type, Content: content, Priority: record.Priority, TTL: 3600, Action: "add", Origin: "imported"})
	}
	for _, record := range serverSetupMailDNSValues(d, oldSPF, oldMX, oldDMARC) {
		content := record.Value
		if record.Type == "TXT" {
			content = splitTXTContent(content)
		}
		records = append(records, serverSetupInfrastructureDNSRecord{Name: record.Name, Type: record.Type, Content: content, Priority: record.Priority, TTL: 3600, Action: "add", Origin: "mail"})
	}
	return records, nil
}

// The mail_dns step (after the mail profile): the main domain's DKIM key,
// signing for it, and, when this server keeps the zone, its DKIM TXT record
// published like any other record of the zone. Every part is idempotent:
// the key is reused, signing is rewritten as it is, the TXT is replaced by
// the same value, so a retried step never makes a second, different change.
// DKIM adimi: anahtar, imzalama ve bolgede TXT; her parca tekrar edilebilir.
func (p *Panel) runServerSetupMailDNS(ctx context.Context, plan serverSetupPlan, step serverSetupExecutionStep) (bool, error) {
	draft := plan.Draft
	if step.Target != draft.MainDomain || !serverSetupHasMail(draft) {
		return false, errors.New("mail DNS step does not match its reviewed plan")
	}
	if err := p.requireServerSetupAdmission(ctx); err != nil {
		return false, err
	}
	var key transport.DKIMEnsureResponse
	if err := p.callAgentContext(ctx, "Agent.EnsureDKIMKey", &transport.DKIMEnsureRequest{Domain: draft.MainDomain, Selector: dkimSelector}, &key); err != nil {
		return false, err
	}
	if key.Error != "" || key.PublicKeyB64 == "" {
		return false, &serverSetupChildFailure{Code: "server_setup_mail_dns_dkim_failed", Message: "The DKIM key for the main domain could not be prepared: " + boundedSetupHostReason(key.Error)}
	}
	// The signing change has one identity per step (D-024): a lost reply is
	// reconciled by reading that exact mutation, never repeated under a new
	// identity. / Imzalama degisikliginin adim basina tek kimligi vardir.
	signing := serverSetupMailDNSSigningIdentity(step)
	job, err := p.statusAgentMutation(ctx, signing.RequestID)
	if err != nil {
		return false, errServiceOperationBusy
	}
	switch {
	case job != nil && agentMutationActive(job.Status):
		return false, errServiceOperationBusy
	case job != nil && job.Status == agentMutationFailed:
		return false, &serverSetupChildFailure{Code: "server_setup_mail_dns_signing_failed", Message: "DKIM signing for the main domain could not be configured; its recorded operation failed. Review the plan again to retry."}
	case job != nil && job.Status == agentMutationSucceeded:
		// Already configured by this step's own operation.
	default:
		if !p.serviceMutationMu.TryLock() {
			return false, errServiceOperationBusy
		}
		var sign transport.ConfigureDKIMSigningResponse
		signErr := p.withStandaloneAgentMutationIdentity(ctx, signing.op, signing.OwnerID, func(callCtx context.Context, binding agentMutationBinding) error {
			request := transport.ServiceMutationRequest{ServiceMutationBinding: binding}
			if err := p.callAgentContext(callCtx, "Agent.ConfigureDKIMSigning", &request, &sign); err != nil {
				return err
			}
			if sign.Error != "" {
				return errors.New(sign.Error)
			}
			return nil
		})
		p.serviceMutationMu.Unlock()
		if signErr != nil {
			if sign.Error == "" {
				// No answer from the Agent: the outcome is read again next pass.
				return false, errServiceOperationBusy
			}
			return false, &serverSetupChildFailure{Code: "server_setup_mail_dns_signing_failed", Message: "DKIM signing for the main domain could not be configured: " + boundedSetupHostReason(sign.Error)}
		}
	}
	if draft.DNSMode != setupDNSModeLocal || draft.InfrastructureDNS == nil {
		return true, nil
	}
	var zoneID int
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT id FROM pdns_domains WHERE name=?`, draft.InfrastructureDNS.Zone).Scan(&zoneID); err != nil {
		return false, err
	}
	if err := p.upsertTXT(ctx, zoneID, dkimSelector+"._domainkey."+draft.MainDomain, dkimRecommended(key.PublicKeyB64)); err != nil {
		return false, err
	}
	if err := p.syncZoneToDNS(ctx, draft.InfrastructureDNS.Zone, false); err != nil {
		return false, err
	}
	return true, nil
}

type serverSetupMailDNSSigning struct {
	RequestID, OwnerID string
	op                 serviceOperation
}

// serverSetupMailDNSSigningIdentity derives the DKIM signing mutation's
// identity from the step's own (itself derived from execution and step).
func serverSetupMailDNSSigningIdentity(step serverSetupExecutionStep) serverSetupMailDNSSigning {
	identity := serverSetupMailDNSSigning{RequestID: serverSetupID(step.RequestID, "dkim-signing", "request"), OwnerID: serverSetupID(step.OwnerID, "dkim-signing", "owner")}
	identity.op = serviceOperation{RequestID: identity.RequestID, Kind: "dkim_signing_configure", ServiceID: "opendkim"}
	return identity
}

func serverSetupNeedsMailDNSStep(d serverSetupDraft) bool {
	return serverSetupHasMail(d) && ((d.DNSMode == setupDNSModeLocal && d.DNSRole == transport.DNSPairRolePrimary && d.InfrastructureDNS != nil) || d.DNSMode == setupDNSModeExternal)
}

// serverSetupMXPriority parses "10 mail.example.com".
func serverSetupMXValue(value string) (int, string) {
	fields := strings.Fields(value)
	if len(fields) != 2 {
		return 0, ""
	}
	priority, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, ""
	}
	return priority, strings.ToLower(strings.TrimSuffix(fields[1], "."))
}
