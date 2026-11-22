package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/rpc"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// An out-of-office reply names only the mailbox's own addresses (21 Nov 2026,
// independent review of alpha.143, finding 1).
//
// Pigeonhole writes the reply's From: from the additional address the
// incoming message named, and mail handed over on the server is signed by
// its From:. An additional address in another account's domain (or the
// owner's) therefore had the reply, with the mailbox owner's text, signed for
// that domain. Now an additional address must be forwarded to the mailbox
// from a domain of the same account (transport.ForeignAutoresponderAddresses),
// checked here before anything is stored and again by the Agent from the
// server's own files. Settings saved before are rendered again without the
// foreign addresses; the mailbox's reply window says once which addresses
// were removed and why, and it is audited.
//
// 22 Nov 2026 (second review of alpha.143, B2, B3, B9):
//   - The same check-and-rewrite runs right after every operation that changes
//     who holds a name or where an address delivers, inside that operation's
//     flow (reconcileAutoresponderAddressesAfter / ...Locked): a subscription
//     changing owner, a domain deleted or added, a forwarder or catch-all
//     added, changed or removed, a mailbox added; and at every start and every
//     hour as the safety net.
//   - A rewrite the Agent cannot make (no answer, another build, Dovecot
//     stopped, a refusal) takes the reply off for that mailbox when that can be
//     applied, and the reply window says why and that saving the reply again
//     turns it back on. When even that cannot be applied the foreign address
//     stays active: the administrator gets one Problems now line per mailbox
//     (mail_autoresponder_foreign) naming the step, the reply window says so,
//     and it is tried again at each start and each hourly round until done.
//   - Each mailbox is checked and rewritten under p.mailMutationMu, with its
//     row read again under the lock, so a customer's save of the same reply
//     (which holds the same lock) and the reconcile never overwrite each
//     other.
//
// Otomatik yanıt yalnız kutunun kendi adreslerini adlandırır. Bir adın kimde
// olduğunu ya da bir adresin nereye teslim edildiğini değiştiren her işlemden
// hemen sonra, her açılışta ve her saat denetlenir. Sunucuda yeniden
// yazılamayan yanıt kapatılır; kapatılamıyorsa yönetici "Şu anki sorunlar"da
// görür ve her saat yeniden denenir.

// autoresponderRemovedSettingPrefix keys the one-time notice per mailbox
// (panel_settings "mail_autoresponder_removed_v1:<account id>"). It is
// removed when the mailbox's reply is saved again or the mailbox is gone.
const autoresponderRemovedSettingPrefix = "mail_autoresponder_removed_v1:"

// autoresponderForeignSettingPrefix keys a reply whose foreign addresses could
// neither be rewritten nor taken off the server (panel_settings
// "mail_autoresponder_foreign_v1:<account id>"). It is shown in Problems now
// and in the reply window, and removed once the reply names no foreign
// address any more, when the reply is saved, or when the mailbox is gone.
const autoresponderForeignSettingPrefix = "mail_autoresponder_foreign_v1:"

// The step a still-active foreign address waits for (Problems now Subject).
const (
	// The Agent did not answer, or the call could not be made.
	autoresponderStepAgentUnreachable = "agent_unreachable"
	// The Agent is another build than the Panel (an update not finished).
	autoresponderStepAgentBuild = "agent_build"
	// The Agent answered, but could neither rewrite nor turn off the reply.
	autoresponderStepAgentFailed = "agent_failed"
)

// autoresponderRemovedNotice is what the reply window shows once.
type autoresponderRemovedNotice struct {
	Addresses []string `json:"addresses"`
	At        string   `json:"at"`
	// TakenOff: the rewrite could not be made, so the reply was turned off;
	// saving it again turns it back on.
	TakenOff bool `json:"taken_off,omitempty"`
}

// autoresponderForeignRecord is a reply that still answers from an address
// that is not the mailbox's own.
type autoresponderForeignRecord struct {
	Mailbox   string   `json:"mailbox"`
	Domain    string   `json:"domain"`
	Addresses []string `json:"addresses"`
	Step      string   `json:"step"`
	Since     string   `json:"since"`
}

// mailboxAccountDomains lists every domain of the account that owns the
// mailbox's domain (the subscriptions of the same owner).
func (p *Panel) mailboxAccountDomains(ctx context.Context, domainID int) ([]string, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT DISTINCT other.name
		FROM domains own
		JOIN subscriptions own_sub ON own_sub.id = own.subscription_id
		JOIN subscriptions other_sub ON other_sub.owner_id = own_sub.owner_id
		JOIN domains other ON other.subscription_id = other_sub.id
		WHERE own.id = ?
		ORDER BY other.name`, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var domains []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		domains = append(domains, strings.ToLower(name))
	}
	return domains, rows.Err()
}

// foreignAutoresponderAddresses returns the account's domains and the
// additional addresses that are not the mailbox's own, from the Panel's
// records (forwardings, catch-alls, mailboxes).
func (p *Panel) foreignAutoresponderAddresses(ctx context.Context, domainID int, mailbox string, addresses []string) ([]string, []string, error) {
	domains, err := p.mailboxAccountDomains(ctx, domainID)
	if err != nil {
		return nil, nil, err
	}
	if len(addresses) == 0 {
		return domains, nil, nil
	}
	forwardings, err := loadAllForwardings(ctx, p.db.GetDB())
	if err != nil {
		return nil, nil, err
	}
	var lookupErr error
	isMailbox := func(address string) bool {
		var one int
		err := p.db.GetDB().QueryRowContext(ctx, `SELECT 1 FROM email_accounts WHERE lower(address) = ? LIMIT 1`, address).Scan(&one)
		if err != nil && err != sql.ErrNoRows && lookupErr == nil {
			lookupErr = err
		}
		return err == nil
	}
	foreign := transport.ForeignAutoresponderAddresses(mailbox, addresses, domains, forwardings, isMailbox)
	if lookupErr != nil {
		return nil, nil, lookupErr
	}
	return domains, foreign, nil
}

// autoresponderForeignError is the neutral refusal's text: the addresses are
// not the mailbox's own; whose they are is never said.
func autoresponderForeignError(foreign []string) string {
	return fmt.Sprintf("Not one of this mailbox's own addresses: %s. An additional address must be forwarded to this mailbox from a domain of the same account. Nothing was changed.",
		strings.Join(foreign, ", "))
}

// autoresponderRemovedNoticeFor reads the one-time notice of a mailbox.
func (p *Panel) autoresponderRemovedNoticeFor(ctx context.Context, accountID int) *autoresponderRemovedNotice {
	raw := p.setting(ctx, autoresponderRemovedSettingPrefix+strconv.Itoa(accountID))
	if raw == "" {
		return nil
	}
	var notice autoresponderRemovedNotice
	if err := json.Unmarshal([]byte(raw), &notice); err != nil || len(notice.Addresses) == 0 {
		return nil
	}
	return &notice
}

// autoresponderForeignRecordFor reads the still-active foreign addresses of a
// mailbox, nil when there are none.
func (p *Panel) autoresponderForeignRecordFor(ctx context.Context, accountID int) *autoresponderForeignRecord {
	raw := p.setting(ctx, autoresponderForeignSettingPrefix+strconv.Itoa(accountID))
	if raw == "" {
		return nil
	}
	var record autoresponderForeignRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil || len(record.Addresses) == 0 {
		return nil
	}
	return &record
}

// clearAutoresponderRemovedNotice removes the notice once the reply is saved
// again (the person has seen and answered it), and the record of foreign
// addresses still active: the save just applied only the mailbox's own.
func (p *Panel) clearAutoresponderRemovedNotice(ctx context.Context, accountID int) {
	if _, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key IN (?, ?)`,
		autoresponderRemovedSettingPrefix+strconv.Itoa(accountID), autoresponderForeignSettingPrefix+strconv.Itoa(accountID)); err != nil {
		log.Printf("[mail] autoresponder %d: the removed-address notice could not be cleared: %v", accountID, err)
	}
}

// clearAutoresponderForeign removes the record of foreign addresses still
// active (absent is fine).
func (p *Panel) clearAutoresponderForeign(ctx context.Context, accountID int) {
	if _, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key = ?`,
		autoresponderForeignSettingPrefix+strconv.Itoa(accountID)); err != nil {
		log.Printf("[mail] autoresponder %d: the record of foreign addresses could not be cleared: %v", accountID, err)
	}
}

// storedAutoresponderRow is one saved reply.
type storedAutoresponderRow struct {
	accountID, domainID int
	address, domain     string
	setting             transport.MailAutoresponder
}

// readAutoresponderRow reads one mailbox's saved reply with its mailbox and
// domain; ok=false when there is none.
func (p *Panel) readAutoresponderRow(ctx context.Context, accountID int) (storedAutoresponderRow, bool, error) {
	row := storedAutoresponderRow{accountID: accountID}
	var enabled int
	var addresses string
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT a.domain_id, a.address, d.name, r.enabled, r.subject, r.message,
		       COALESCE(r.start_date, ''), COALESCE(r.end_date, ''), r.interval_days, r.addresses
		FROM email_autoresponders r
		JOIN email_accounts a ON a.id = r.account_id
		JOIN domains d ON d.id = a.domain_id
		WHERE r.account_id = ?`, accountID).Scan(&row.domainID, &row.address, &row.domain, &enabled,
		&row.setting.Subject, &row.setting.Message, &row.setting.StartDate, &row.setting.EndDate, &row.setting.IntervalDays, &addresses)
	if errors.Is(err, sql.ErrNoRows) {
		return row, false, nil
	}
	if err != nil {
		return row, false, err
	}
	row.setting.Enabled = enabled == 1
	if err := json.Unmarshal([]byte(addresses), &row.setting.Addresses); err != nil {
		return row, false, fmt.Errorf("addresses unreadable: %w", err)
	}
	return row, true, nil
}

// autoresponderReconcileCandidates lists the mailboxes whose reply names
// additional addresses, or whose foreign addresses are recorded as still
// active. Mailboxes of a domain whose deletion is running are left out (their
// mail is being removed with the domain).
func (p *Panel) autoresponderReconcileCandidates(ctx context.Context) ([]int, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT r.account_id
		FROM email_autoresponders r
		JOIN email_accounts a ON a.id = r.account_id
		LEFT JOIN domain_deletion_operations op ON op.domain_id = a.domain_id
		WHERE op.domain_id IS NULL
		  AND (r.addresses <> '[]'
		       OR EXISTS (SELECT 1 FROM panel_settings s WHERE s.key = ? || r.account_id))
		ORDER BY r.account_id`, autoresponderForeignSettingPrefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// removeGoneAutoresponderRecords removes the notices and records of mailboxes
// that are gone (no leftovers), and records of mailboxes without a reply.
func (p *Panel) removeGoneAutoresponderRecords(ctx context.Context) {
	for _, prefix := range []string{autoresponderRemovedSettingPrefix, autoresponderForeignSettingPrefix} {
		if _, err := p.db.GetDB().ExecContext(ctx, `
			DELETE FROM panel_settings WHERE key LIKE ? AND
			CAST(substr(key, ?) AS INTEGER) NOT IN (SELECT id FROM email_accounts)`,
			prefix+"%", len(prefix)+1); err != nil {
			log.Printf("[mail] old out-of-office records (%s) could not be removed: %v", prefix, err)
		}
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `
		DELETE FROM panel_settings WHERE key LIKE ? AND
		CAST(substr(key, ?) AS INTEGER) NOT IN (SELECT account_id FROM email_autoresponders)`,
		autoresponderForeignSettingPrefix+"%", len(autoresponderForeignSettingPrefix)+1); err != nil {
		log.Printf("[mail] old out-of-office records could not be removed: %v", err)
	}
}

// reconcileAutoresponderAddresses is the start's run (and the hourly
// round's): every saved reply that names an address that is not the
// mailbox's own is rendered again without it.
func (p *Panel) reconcileAutoresponderAddresses(ctx context.Context) {
	p.reconcileAutoresponderAddressesAfter(ctx, "start")
}

// reconcileAutoresponderAddressesAfter runs the check after an operation that
// does not hold p.mailMutationMu; each mailbox takes the lock for its own
// check and rewrite. cause names the operation in the audit row.
func (p *Panel) reconcileAutoresponderAddressesAfter(ctx context.Context, cause string) {
	p.removeGoneAutoresponderRecords(ctx)
	ids, err := p.autoresponderReconcileCandidates(ctx)
	if err != nil {
		log.Printf("[mail] out-of-office replies could not be read for the address check (after %s): %v", cause, err)
		return
	}
	for _, id := range ids {
		p.mailMutationMu.Lock()
		p.reconcileOneAutoresponderLocked(ctx, id, cause)
		p.mailMutationMu.Unlock()
	}
}

// reconcileAutoresponderAddressesLocked is the same check for an operation
// that holds p.mailMutationMu (the caller keeps it held).
func (p *Panel) reconcileAutoresponderAddressesLocked(ctx context.Context, cause string) {
	p.removeGoneAutoresponderRecords(ctx)
	ids, err := p.autoresponderReconcileCandidates(ctx)
	if err != nil {
		log.Printf("[mail] out-of-office replies could not be read for the address check (after %s): %v", cause, err)
		return
	}
	for _, id := range ids {
		p.reconcileOneAutoresponderLocked(ctx, id, cause)
	}
}

// setAutoresponderOnAgent sends one reply setting to the Agent.
func (p *Panel) setAutoresponderOnAgent(ctx context.Context, mailbox string, setting transport.MailAutoresponder, domains []string) (transport.MailAutoresponderResponse, error) {
	var response transport.MailAutoresponderResponse
	err := p.callAgentContext(ctx, "Agent.SetMailAutoresponder", &transport.SetMailAutoresponderRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit),
		Email:               mailbox,
		Autoresponder:       setting,
		AccountDomains:      domains,
	}, &response)
	return response, err
}

// autoresponderNotOurs reports a refusal that means CelikPanel's script is
// not what answers this mailbox, so no foreign address of its is active.
func autoresponderNotOurs(refusal string) bool {
	switch refusal {
	case transport.AutoresponderRefusedOwnerScript, transport.AutoresponderRefusedNoMailbox,
		transport.AutoresponderRefusedMailNotInstalled:
		return true
	}
	return false
}

// autoresponderStepFor names what the administrator does when neither the
// rewrite nor turning the reply off could be applied.
func autoresponderStepFor(err error) string {
	var serverErr rpc.ServerError
	if errors.As(err, &serverErr) {
		if strings.Contains(string(serverErr), "build mismatch") {
			return autoresponderStepAgentBuild
		}
		return autoresponderStepAgentFailed
	}
	if err != nil {
		return autoresponderStepAgentUnreachable
	}
	return autoresponderStepAgentFailed
}

// reconcileOneAutoresponderLocked checks and rewrites one mailbox's reply. It
// requires p.mailMutationMu and reads the row again under it.
func (p *Panel) reconcileOneAutoresponderLocked(ctx context.Context, accountID int, cause string) {
	row, ok, err := p.readAutoresponderRow(ctx, accountID)
	if err != nil {
		log.Printf("[mail] out-of-office reply %d: %v", accountID, err)
		return
	}
	if !ok {
		p.clearAutoresponderForeign(ctx, accountID)
		return
	}
	mailbox, err := transport.CanonicalMailboxForDomain(row.address, row.domain)
	if err != nil {
		return
	}
	domains, foreign, err := p.foreignAutoresponderAddresses(ctx, row.domainID, mailbox, row.setting.Addresses)
	if err != nil {
		log.Printf("[mail] out-of-office reply %d: addresses could not be checked: %v", accountID, err)
		return
	}
	if len(foreign) == 0 {
		p.clearAutoresponderForeign(ctx, accountID)
		return
	}
	drop := map[string]bool{}
	for _, address := range foreign {
		drop[address] = true
	}
	next := row.setting
	next.Addresses = nil
	for _, address := range row.setting.Addresses {
		if !drop[strings.ToLower(address)] {
			next.Addresses = append(next.Addresses, address)
		}
	}
	normalized, err := transport.NormalizeMailAutoresponder(next, mailbox)
	if err != nil {
		log.Printf("[mail] out-of-office reply %d: %v", accountID, err)
		return
	}
	takenOff := false
	if normalized.Enabled {
		response, callErr := p.setAutoresponderOnAgent(ctx, mailbox, normalized, domains)
		switch {
		case callErr == nil && response.Applied:
		case callErr == nil && response.Refusal == transport.AutoresponderRefusedLMTPRejected:
			// The Agent took the reply off the server (as on a save).
			takenOff = true
		case callErr == nil && autoresponderNotOurs(response.Refusal):
			// CelikPanel's script is not what answers this mailbox.
		default:
			// B3: the rewrite could not be made. The reply is taken off
			// (turning a reply off needs neither Dovecot running nor the
			// addresses checked); when even that cannot be applied, the
			// foreign address stays active and the administrator is told.
			off := normalized
			off.Enabled = false
			offResponse, offErr := p.setAutoresponderOnAgent(ctx, mailbox, off, domains)
			if offErr == nil && (offResponse.Applied || autoresponderNotOurs(offResponse.Refusal)) {
				takenOff = true
				why := response.Refusal
				if callErr != nil {
					why = callErr.Error()
				}
				log.Printf("[mail] out-of-office reply of %s names %s, which is not the mailbox's own; it could not be rewritten (%s), so it was turned off",
					mailbox, strings.Join(foreign, ", "), why)
				break
			}
			step := autoresponderStepFor(offErr)
			p.recordAutoresponderForeign(ctx, accountID, autoresponderForeignRecord{
				Mailbox: mailbox, Domain: row.domain, Addresses: foreign, Step: step,
			})
			log.Printf("[mail] out-of-office reply of %s names %s, which is not the mailbox's own; it could be neither rewritten (%v %s) nor turned off (%v %s); tried again within the hour",
				mailbox, strings.Join(foreign, ", "), callErr, response.Refusal, offErr, offResponse.Refusal)
			return
		}
	}
	addresses, err := json.Marshal(append([]string{}, normalized.Addresses...))
	if err != nil {
		return
	}
	query := `UPDATE email_autoresponders SET addresses = ?, updated_at = datetime('now') WHERE account_id = ?`
	if takenOff {
		query = `UPDATE email_autoresponders SET addresses = ?, enabled = 0, updated_at = datetime('now') WHERE account_id = ?`
	}
	if _, err := p.db.GetDB().ExecContext(ctx, query, string(addresses), accountID); err != nil {
		// The server already answers without the foreign addresses; the next
		// round stores it (the rewrite is the same).
		log.Printf("[mail] out-of-office reply %d: the stored addresses could not be updated: %v", accountID, err)
		return
	}
	p.clearAutoresponderForeign(ctx, accountID)
	notice, _ := json.Marshal(autoresponderRemovedNotice{Addresses: foreign, At: time.Now().UTC().Format(time.RFC3339), TakenOff: takenOff})
	if err := p.setSetting(ctx, autoresponderRemovedSettingPrefix+strconv.Itoa(accountID), string(notice)); err != nil {
		log.Printf("[mail] out-of-office reply %d: the notice could not be stored: %v", accountID, err)
	}
	details, _ := json.Marshal(map[string]any{"mailbox": mailbox, "removed": foreign, "reason": "not_own_address", "after": cause, "turned_off": takenOff})
	p.recordAudit(auditRecord{Action: "mail.autoresponder.addresses_removed", ResourceType: "email_account", ResourceID: accountID, Details: string(details)})
	log.Printf("[mail] out-of-office reply of %s: removed %s, which is not one of the mailbox's own addresses (after %s)", mailbox, strings.Join(foreign, ", "), cause)
}

// recordAutoresponderForeign stores (or refreshes) the record of foreign
// addresses still active; Since stays the first time it was recorded.
func (p *Panel) recordAutoresponderForeign(ctx context.Context, accountID int, record autoresponderForeignRecord) {
	record.Since = time.Now().UTC().Format(time.RFC3339)
	if earlier := p.autoresponderForeignRecordFor(ctx, accountID); earlier != nil && earlier.Since != "" {
		record.Since = earlier.Since
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return
	}
	if err := p.setSetting(ctx, autoresponderForeignSettingPrefix+strconv.Itoa(accountID), string(raw)); err != nil {
		log.Printf("[mail] out-of-office reply %d: the record of foreign addresses could not be stored: %v", accountID, err)
	}
}

// autoresponderForeignProblems is the Problems now line per mailbox whose
// reply still answers from an address that is not its own (administrator;
// the action opens the mailbox's domain mail page).
func autoresponderForeignProblems(ctx context.Context, db *sql.DB) []monitoringProblem {
	rows, err := db.QueryContext(ctx, `SELECT key, value FROM panel_settings WHERE key LIKE ? ORDER BY key`,
		autoresponderForeignSettingPrefix+"%")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var problems []monitoringProblem
	for rows.Next() {
		var key, raw string
		if rows.Scan(&key, &raw) != nil {
			continue
		}
		var record autoresponderForeignRecord
		if json.Unmarshal([]byte(raw), &record) != nil || record.Mailbox == "" || record.Domain == "" {
			continue
		}
		step := record.Step
		switch step {
		case autoresponderStepAgentUnreachable, autoresponderStepAgentBuild, autoresponderStepAgentFailed:
		default:
			step = autoresponderStepAgentFailed
		}
		problems = append(problems, monitoringProblem{
			Key:  "mail_autoresponder_foreign:" + strings.TrimPrefix(key, autoresponderForeignSettingPrefix),
			Kind: "mail_autoresponder_foreign", Severity: "warning", Actor: "administrator",
			Name: record.Mailbox, Subject: step, Since: record.Since,
			Action: monitoringProblemAction{Kind: "open", To: "/domains/" + url.PathEscape(record.Domain) + "?tab=mail"},
		})
	}
	return problems
}

// autoresponderRoundInterval is the hourly round (a var for tests).
var autoresponderRoundInterval = time.Hour

// startAutoresponderAddressReconcile runs the check beside the start, then
// every hour: a rewrite that could not be made is tried again until done, and
// a change no operation reported is caught within the hour.
func (p *Panel) startAutoresponderAddressReconcile() {
	go func() {
		run := func(cause string) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			p.reconcileAutoresponderAddressesAfter(ctx, cause)
		}
		run("start")
		ticker := time.NewTicker(autoresponderRoundInterval)
		defer ticker.Stop()
		for range ticker.C {
			run("hourly")
		}
	}()
}
