package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
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
// foreign addresses at the first start after the update; the mailbox's reply
// window says once which addresses were removed and why, and it is audited.
//
// Otomatik yanıt yalnız kutunun kendi adreslerini adlandırır; önceden
// kaydedilmiş yabancı adresler güncellemeden sonraki ilk açılışta çıkarılır,
// kutunun penceresi bunu bir kez söyler ve denetim kaydına yazılır.

// autoresponderRemovedSettingPrefix keys the one-time notice per mailbox
// (panel_settings "mail_autoresponder_removed_v1:<account id>"). It is
// removed when the mailbox's reply is saved again or the mailbox is gone.
const autoresponderRemovedSettingPrefix = "mail_autoresponder_removed_v1:"

// autoresponderRemovedNotice is what the reply window shows once.
type autoresponderRemovedNotice struct {
	Addresses []string `json:"addresses"`
	At        string   `json:"at"`
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

// clearAutoresponderRemovedNotice removes the notice once the reply is saved
// again (the person has seen and answered it).
func (p *Panel) clearAutoresponderRemovedNotice(ctx context.Context, accountID int) {
	if _, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key = ?`,
		autoresponderRemovedSettingPrefix+strconv.Itoa(accountID)); err != nil {
		log.Printf("[mail] autoresponder %d: the removed-address notice could not be cleared: %v", accountID, err)
	}
}

// storedAutoresponderRow is one saved reply that names additional addresses.
type storedAutoresponderRow struct {
	accountID, domainID int
	address, domain     string
	setting             transport.MailAutoresponder
}

// reconcileAutoresponderAddresses runs after every Panel start (it changes
// something only once): a saved reply that names an address that is not the
// mailbox's own is rendered again without it. A reply the Agent could not
// rewrite now (Dovecot stopped, the Agent unreachable) keeps its row and is
// tried again at the next start; nothing is retried in a loop.
func (p *Panel) reconcileAutoresponderAddresses(ctx context.Context) {
	// Notices of mailboxes that are gone go with them (no leftovers).
	if _, err := p.db.GetDB().ExecContext(ctx, `
		DELETE FROM panel_settings WHERE key LIKE ? AND
		CAST(substr(key, ?) AS INTEGER) NOT IN (SELECT id FROM email_accounts)`,
		autoresponderRemovedSettingPrefix+"%", len(autoresponderRemovedSettingPrefix)+1); err != nil {
		log.Printf("[mail] old out-of-office notices could not be removed: %v", err)
	}
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT r.account_id, a.domain_id, a.address, d.name, r.enabled, r.subject, r.message,
		       COALESCE(r.start_date, ''), COALESCE(r.end_date, ''), r.interval_days, r.addresses
		FROM email_autoresponders r
		JOIN email_accounts a ON a.id = r.account_id
		JOIN domains d ON d.id = a.domain_id
		WHERE r.addresses <> '[]'
		ORDER BY r.account_id`)
	if err != nil {
		log.Printf("[mail] out-of-office replies could not be read for the address check: %v", err)
		return
	}
	var stored []storedAutoresponderRow
	for rows.Next() {
		var row storedAutoresponderRow
		var enabled int
		var addresses string
		if err := rows.Scan(&row.accountID, &row.domainID, &row.address, &row.domain, &enabled, &row.setting.Subject, &row.setting.Message,
			&row.setting.StartDate, &row.setting.EndDate, &row.setting.IntervalDays, &addresses); err != nil {
			log.Printf("[mail] out-of-office reply row: %v", err)
			continue
		}
		row.setting.Enabled = enabled == 1
		if err := json.Unmarshal([]byte(addresses), &row.setting.Addresses); err != nil {
			log.Printf("[mail] out-of-office reply %d: addresses unreadable: %v", row.accountID, err)
			continue
		}
		stored = append(stored, row)
	}
	_ = rows.Close()
	for _, row := range stored {
		p.reconcileOneAutoresponder(ctx, row)
	}
}

func (p *Panel) reconcileOneAutoresponder(ctx context.Context, row storedAutoresponderRow) {
	mailbox, err := transport.CanonicalMailboxForDomain(row.address, row.domain)
	if err != nil {
		return
	}
	domains, foreign, err := p.foreignAutoresponderAddresses(ctx, row.domainID, mailbox, row.setting.Addresses)
	if err != nil {
		log.Printf("[mail] out-of-office reply %d: addresses could not be checked: %v", row.accountID, err)
		return
	}
	if len(foreign) == 0 {
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
		log.Printf("[mail] out-of-office reply %d: %v", row.accountID, err)
		return
	}
	disable := false
	if normalized.Enabled {
		p.mailMutationMu.Lock()
		var response transport.MailAutoresponderResponse
		callErr := p.callAgentContext(ctx, "Agent.SetMailAutoresponder", &transport.SetMailAutoresponderRequest{
			ExpectedBuildCommit: strings.TrimSpace(buildCommit),
			Email:               mailbox,
			Autoresponder:       normalized,
			AccountDomains:      domains,
		}, &response)
		p.mailMutationMu.Unlock()
		switch {
		case callErr != nil:
			log.Printf("[mail] out-of-office reply of %s names %s, which is not the mailbox's own; it could not be rewritten now (%v) and is tried again at the next start",
				mailbox, strings.Join(foreign, ", "), callErr)
			return
		case response.Applied:
		case response.Refusal == transport.AutoresponderRefusedLMTPRejected:
			// The Agent took the reply off the server (as on a save).
			disable = true
		case response.Refusal == transport.AutoresponderRefusedOwnerScript,
			response.Refusal == transport.AutoresponderRefusedNoMailbox,
			response.Refusal == transport.AutoresponderRefusedMailNotInstalled:
			// CelikPanel's script is not what answers this mailbox.
		default:
			log.Printf("[mail] out-of-office reply of %s names %s, which is not the mailbox's own; it could not be rewritten now (%s: %s) and is tried again at the next start",
				mailbox, strings.Join(foreign, ", "), response.Refusal, response.Detail)
			return
		}
	}
	addresses, err := json.Marshal(append([]string{}, normalized.Addresses...))
	if err != nil {
		return
	}
	query := `UPDATE email_autoresponders SET addresses = ?, updated_at = datetime('now') WHERE account_id = ?`
	if disable {
		query = `UPDATE email_autoresponders SET addresses = ?, enabled = 0, updated_at = datetime('now') WHERE account_id = ?`
	}
	if _, err := p.db.GetDB().ExecContext(ctx, query, string(addresses), row.accountID); err != nil {
		log.Printf("[mail] out-of-office reply %d: the stored addresses could not be updated: %v", row.accountID, err)
		return
	}
	notice, _ := json.Marshal(autoresponderRemovedNotice{Addresses: foreign, At: time.Now().UTC().Format(time.RFC3339)})
	if err := p.setSetting(ctx, autoresponderRemovedSettingPrefix+strconv.Itoa(row.accountID), string(notice)); err != nil {
		log.Printf("[mail] out-of-office reply %d: the notice could not be stored: %v", row.accountID, err)
	}
	details, _ := json.Marshal(map[string]any{"mailbox": mailbox, "removed": foreign, "reason": "not_own_address"})
	p.recordAudit(auditRecord{Action: "mail.autoresponder.addresses_removed", ResourceType: "email_account", ResourceID: row.accountID, Details: string(details)})
	log.Printf("[mail] out-of-office reply of %s: removed %s, which is not one of the mailbox's own addresses", mailbox, strings.Join(foreign, ", "))
}

// startAutoresponderAddressReconcile runs the check once, beside the start.
func (p *Panel) startAutoresponderAddressReconcile() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		p.reconcileAutoresponderAddresses(ctx)
	}()
}
