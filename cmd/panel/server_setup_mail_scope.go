package main

import (
	"context"
	"database/sql"
	"fmt"
)

// A mail-only extension (alpha.114, owner decision 2026-10-31).
//
// Installing a mail profile from Components (or making the main domain's
// missing DKIM key from the DNS page) used to send the owner into the setup
// wizard, at its Names step, with questions that had nothing to do with mail.
// The same setup-extension planner and runner now take it in place: the
// extension is begun with scope "mail", and while its draft differs from the
// completed setup only by the mail profile's components and the mail host
// name, the plan holds the mail steps alone. Names, the DNS software, the
// panel's own records and certificate and the infrastructure zone are not
// planned, so nothing else is asked or changed. On a joined server the main
// domain's mail records stay with the first server: a mail step for the main
// domain is kept only when this server already received that mail before the
// extension (a repair), never when mail is being added here.
//
// The scope is one panel setting next to the extension base; an edit in the
// wizard that changes anything else simply turns the plan back into the full
// extension review. Completing or stopping the extension removes it.
//
// Yalniz posta genisletmesi: plan yalniz posta adimlarini tutar; adlar, DNS
// yazilimi ve ana alan adinin posta kayitlari (katilan sunucuda) degismez.

const (
	settingServerSetupExtensionScope = "server_setup_extension_scope"
	serverSetupExtensionScopeMail    = "mail"
)

func saveServerSetupExtensionScope(ctx context.Context, tx *sql.Tx, scope string) error {
	if scope == "" {
		_, err := tx.ExecContext(ctx, `DELETE FROM panel_settings WHERE key=?`, settingServerSetupExtensionScope)
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO panel_settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, settingServerSetupExtensionScope, scope)
	return err
}

// serverSetupExtensionScope is "mail" only while the extension still is one.
func (p *Panel) serverSetupExtensionScope(ctx context.Context, s serverSetupState) string {
	if !s.Extension || s.ExtensionBase == nil || !serverSetupDraftHasMailProfile(s.Draft) {
		return ""
	}
	var value string
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key=?`, settingServerSetupExtensionScope).Scan(&value); err != nil || value != serverSetupExtensionScopeMail {
		return ""
	}
	if !serverSetupMailOnlyChange(*s.ExtensionBase, s.Draft) {
		return ""
	}
	return serverSetupExtensionScopeMail
}

// serverSetupDraftHasMailProfile: a mail profile with its host name, whether
// or not the draft names a main domain (a setup made before alpha.111 has none).
func serverSetupDraftHasMailProfile(d serverSetupDraft) bool {
	return d.MailHostname != "" && len(serverSetupMailProfileIDs(d)) > 0
}

func serverSetupMailComponent(id string) bool {
	return stringIn(id, "postfix", "dovecot", "rspamd", "roundcube") || serverSetupMailDependent(id)
}

func serverSetupComponentSet(d serverSetupDraft) map[string]bool {
	ids := serverSetupPresetComponents(d)
	if d.Customization != nil {
		ids = d.Customization.Components
	}
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

// serverSetupMailOnlyChange: the draft keeps every component of the base,
// adds only mail components, and differs in nothing else than the purpose
// label that follows a customization, the components and the mail host name.
func serverSetupMailOnlyChange(base, draft serverSetupDraft) bool {
	before, after := serverSetupComponentSet(base), serverSetupComponentSet(draft)
	for id := range before {
		if !after[id] {
			return false
		}
	}
	for id := range after {
		if !before[id] && !serverSetupMailComponent(id) {
			return false
		}
	}
	probe := draft
	probe.Purpose, probe.Customization, probe.MailHostname = base.Purpose, base.Customization, base.MailHostname
	return serverSetupDraftsEqual(probe, base)
}

// serverSetupMailScopeSteps keeps the mail steps of a full plan, in order:
// the mail profile and the services that follow it, the firewall (the mail
// ports), the mail host name's address record (the existing access-DNS step),
// its certificate, the independent renewal and the final check.
func serverSetupMailScopeSteps(draft serverSetupDraft, steps []serverSetupPlanStep, joined bool, base *serverSetupDraft) []serverSetupPlanStep {
	keepMainDomainMail := !joined || (base != nil && serverSetupHasMail(*base))
	kept := []serverSetupPlanStep{}
	for _, step := range steps {
		switch {
		case step.Kind == "mail_profile", step.Kind == "mail_certificate", step.Kind == "mail_enrollment", step.Kind == "firewall", step.Kind == "verify":
		case step.Kind == "service" && serverSetupMailComponent(step.Target):
		case step.Kind == "access_dns" && step.Target == draft.MailHostname:
		case step.Kind == "mail_dns" && keepMainDomainMail:
		default:
			continue
		}
		kept = append(kept, step)
	}
	for index := range kept {
		kept[index].ID = fmt.Sprintf("%02d-%s", index+1, kept[index].Kind)
	}
	return kept
}
