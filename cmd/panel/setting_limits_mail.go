package main

import (
	"context"
	"database/sql"
	"log"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Mail keys in the limits model (alpha.128 integration, docs/SITE-LIMITS.md
// §1.1). The mail workstream keeps its plan mail settings in
// mail_plan_limits (the administrator's "Mail limits of hosting plans" on the
// Mail server page). The same values are also limited settings of the rights
// chain, so a reseller plan, a Custom reseller, a plan form or a Custom
// subscription can hold them like any other key:
//
//	mail.mailbox_size_mb           the largest mailbox (MB)
//	mail.outgoing.mailbox          recipients per hour, one mailbox (ceiling)
//	mail.outgoing.domain           recipients per hour, one domain
//	mail.outgoing.subscription     recipients per hour, one hosting account
//
// Every mail value a server enforces or a screen shows is the tighter of the
// two: the mail row (or, without one, the Mail server page's value) and the
// chain's effective ceiling. Nothing is copied between the two stores. The
// server row has no ceiling and no default (the Mail server page holds the
// defaults), so no limit changes when the update arrives.
//
// Posta anahtarları sınır modelinde: uygulanan değer, posta satırı (ya da
// posta sunucusu sayfasının değeri) ile hak zincirinin etkin tavanının daha
// sıkı olanıdır.

const (
	settingMailboxSizeMB            = "mail.mailbox_size_mb"
	settingMailOutgoingMailbox      = "mail.outgoing.mailbox"
	settingMailOutgoingDomain       = "mail.outgoing.domain"
	settingMailOutgoingSubscription = "mail.outgoing.subscription"
)

func init() {
	registerSettingKey(settingKeyDef{Key: settingMailboxSizeMB, Group: "mail", Unit: "mb", Min: 1, Max: transport.MaxMailboxQuotaMB, Presets: []int{1024, 5120, 10240, 51200}})
	registerSettingKey(settingKeyDef{Key: settingMailOutgoingMailbox, Group: "mail", Unit: "count", Min: 1, Max: transport.OutgoingMailMaxPerHour, Presets: []int{100, 250, 500, 1000}})
	registerSettingKey(settingKeyDef{Key: settingMailOutgoingDomain, Group: "mail", Unit: "count", Min: 1, Max: transport.OutgoingMailMaxPerHour, Presets: []int{250, 500, 750, 1500}})
	registerSettingKey(settingKeyDef{Key: settingMailOutgoingSubscription, Group: "mail", Unit: "count", Min: 1, Max: transport.OutgoingMailMaxPerHour, Presets: []int{500, 1000, 2000, 5000}})
}

// mailLimitChains reads the chain's effective limits per subscription once.
type mailLimitChains struct {
	ctx   context.Context
	q     rowQueryer
	cache map[int]settingLimits
}

func newMailLimitChains(ctx context.Context, q rowQueryer) *mailLimitChains {
	return &mailLimitChains{ctx: ctx, q: q, cache: map[int]settingLimits{}}
}

// of is the subscription's limits; a read that fails is logged and limits
// nothing beyond the mail row (the mail row still applies).
func (c *mailLimitChains) of(subID int) settingLimits {
	if c == nil {
		return nil
	}
	if limits, ok := c.cache[subID]; ok {
		return limits
	}
	limits, err := subscriptionSettingLimits(c.ctx, c.q, subID)
	if err != nil {
		log.Printf("mail limits of subscription %d: the rights chain could not be read: %v", subID, err)
		limits = nil
	}
	c.cache[subID] = limits
	return limits
}

// mailRowFor is the subscription's mail row with the chain folded in.
func (c *mailLimitChains) mailRowFor(limits map[mailPlanLimitKey]mailPlanLimitRow, settings outgoingSettings, subID int, planID sql.NullInt64) mailPlanLimitRow {
	return foldMailChain(planLimitsFor(limits, subID, planID), settings, c.of(subID))
}

// foldMailChain makes each value the tighter of the row's (or, without one,
// the Mail server page's) and the chain's ceiling. 0 means no limit, as in
// the mail values.
func foldMailChain(row mailPlanLimitRow, settings outgoingSettings, chain settingLimits) mailPlanLimitRow {
	fold := func(value *int, fallback int, key string) *int {
		ceiling := chain.ceilingValue(key)
		if ceiling <= 0 {
			return value
		}
		base := fallback
		if value != nil {
			base = *value
		}
		folded := lowerLimit(base, ceiling)
		return &folded
	}
	row.MailboxSizeMB = fold(row.MailboxSizeMB, 0, settingMailboxSizeMB)
	row.OutgoingMailbox = fold(row.OutgoingMailbox, settings.MailboxPerHour, settingMailOutgoingMailbox)
	row.OutgoingDomain = fold(row.OutgoingDomain, settings.DomainPerHour, settingMailOutgoingDomain)
	row.OutgoingSubscription = fold(row.OutgoingSubscription, settings.SubscriptionPerHour, settingMailOutgoingSubscription)
	return row
}
