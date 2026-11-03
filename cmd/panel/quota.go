package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
)

// Quota enforcement: subscription limits are checked at every resource
// creation point (the API layer — the UI merely mirrors what the API would
// reject). The checks run for everyone; the admin's seed subscription simply
// carries large values.
//
// One meaning of 0, end to end (alpha.114): a limit of 0 (or below) is
// "unlimited" for domains, databases, mailboxes and disk alike. The plan form
// says so, the plan validation accepts it, these checks honour it and the
// usage answers report it, so a 0 can no longer read as "unlimited" on one
// screen and refuse as "none allowed" on another.
//
// Kota uygulaması: abonelik sınırları her kaynak oluşturma noktasında
// denetlenir. 0'ın tek anlamı vardır (alpha.114): domain, veritabanı, posta
// kutusu ve disk için 0 (ya da altı) "sınırsız" demektir.

type quotaKind string

const (
	quotaDomains quotaKind = "domains"
	quotaDBs     quotaKind = "databases"
	quotaMail    quotaKind = "email accounts"
	quotaDisk    quotaKind = "disk"
)

// Coded refusals for each quota (the web translates err.<CODE>). The action
// is the page that shows the plan, its usage and whom to ask for more.
// Her kota için kodlu retler; eylem, planı, kullanımı ve kimden daha fazla
// isteneceğini gösteren sayfadır.
const (
	errCodeQuotaDatabases = "QUOTA_DATABASES_EXCEEDED"
	errCodeQuotaMailboxes = "QUOTA_MAILBOXES_EXCEEDED"
	myPlanPagePath        = "/account" // My account holds the "My plan" card
)

// quotaUnlimited is the single reading of a stored limit.
// quotaUnlimited, kayıtlı bir sınırın tek okunuşudur.
func quotaUnlimited(limit int64) bool { return limit <= 0 }

// quotaExceededError is a refusal with its numbers. Used and Limit are counts,
// or megabytes for disk.
// quotaExceededError, sayılarıyla birlikte bir rettir.
type quotaExceededError struct {
	Kind  quotaKind
	Used  int64
	Limit int64
}

func (e *quotaExceededError) Error() string {
	if e.Kind == quotaDisk {
		return fmt.Sprintf("disk quota reached: %d of %d MB in use. Free some space, or ask your provider for a larger plan (see My plan).", e.Used, e.Limit)
	}
	return fmt.Sprintf("subscription limit reached: %d of %d %s in use. Remove one, or ask your provider for a larger plan (see My plan).", e.Used, e.Limit, e.Kind)
}

func (e *quotaExceededError) code() string {
	switch e.Kind {
	case quotaDomains:
		return errCodeQuotaDomains
	case quotaDBs:
		return errCodeQuotaDatabases
	case quotaMail:
		return errCodeQuotaMailboxes
	default:
		return errCodeQuotaDisk
	}
}

// errSubscriptionSuspendedCreate: the subscription is suspended (alpha.117).
var errSubscriptionSuspendedCreate = errors.New("subscription is suspended")

// errQuotaSubscriptionMissing: the subscription a creation names does not exist.
var errQuotaSubscriptionMissing = errors.New("subscription not found")

// administratorWorkspace tells whether a subscription is the one the Panel
// made for an administrator who hosts sites without a plan (it was seeded with
// the stored value 999). That workspace has no limit: the old stored 999 would
// have refused the 1000th domain, and the screens showed it as "2 of 999".
// An administrator's subscription with a plan is metered like any other.
// administratorWorkspace, planı olmayan yöneticinin kendi çalışma alanıdır;
// sınırsızdır. Planı olan yönetici aboneliği diğerleri gibi sınırlanır.
func (p *Panel) administratorWorkspace(ctx context.Context, subID int) bool {
	var n int
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM subscriptions s JOIN users u ON u.id = s.owner_id
		WHERE s.id = ? AND u.role = 'admin' AND s.plan_id IS NULL AND s.name = 'Admin Subscription'`, subID).Scan(&n)
	return err == nil && n > 0
}

// checkSubscriptionQuota returns nil when one more resource of the given
// kind fits in the subscription, a *quotaExceededError when the limit is
// reached, errQuotaSubscriptionMissing when there is no such subscription,
// or the read error.
// checkSubscriptionQuota, verilen türden bir kaynak daha aboneliğe
// sığıyorsa nil; sınıra ulaşıldıysa *quotaExceededError döndürür.
func (p *Panel) checkSubscriptionQuota(ctx context.Context, subID int, kind quotaKind) error {
	var limit, used int64
	var err error

	// alpha.117: a suspended subscription takes nothing new (a new site or
	// mailbox would serve, a new database would be open) until resumed.
	if suspended, suspErr := p.subscriptionSuspended(ctx, subID); suspErr == nil && suspended {
		return errSubscriptionSuspendedCreate
	} else if suspErr != nil && !errors.Is(suspErr, errSuspensionNotFound) {
		return suspErr
	}

	// The administrator's own workspace is not metered (see below).
	if p.administratorWorkspace(ctx, subID) {
		return nil
	}

	switch kind {
	case quotaDomains:
		err = p.db.GetDB().QueryRowContext(ctx, `
			SELECT s.max_domains, (SELECT COUNT(*) FROM domains d WHERE d.subscription_id = s.id)
			FROM subscriptions s WHERE s.id = ?`, subID).Scan(&limit, &used)
	case quotaDBs:
		// databases_v2 is the live table.
		// databases_v2 canlı tablodur.
		err = p.db.GetDB().QueryRowContext(ctx, `
			SELECT s.max_databases,
			       (SELECT COUNT(*) FROM databases_v2 db WHERE db.subscription_id = s.id)
			FROM subscriptions s WHERE s.id = ?`, subID).Scan(&limit, &used)
	case quotaMail:
		err = p.db.GetDB().QueryRowContext(ctx, `
			SELECT s.max_email_accounts,
			       (SELECT COUNT(*) FROM email_accounts ea JOIN domains d ON ea.domain_id = d.id WHERE d.subscription_id = s.id)
			FROM subscriptions s WHERE s.id = ?`, subID).Scan(&limit, &used)
	case quotaDisk:
		// Disk is a byte comparison, not a count: the gate refuses new
		// resources once the subscription is already at/over its measured
		// usage (alpha.118: site files, mail and databases, measured by the
		// Agent; before the first measurement the per-site cached numbers).
		// Disk sayı değil bayt karşılaştırmasıdır.
		var usedBytes int64
		err = p.db.GetDB().QueryRowContext(ctx, `
			SELECT s.disk_quota_mb, `+subscriptionDiskBytesSQL+`
			FROM subscriptions s WHERE s.id = ?`, subID).Scan(&limit, &usedBytes)
		if err != nil {
			return quotaReadError(err)
		}
		if quotaUnlimited(limit) || usedBytes < limit*1024*1024 {
			return p.checkResellerPoolUsage(ctx, subID, kind)
		}
		return &quotaExceededError{Kind: quotaDisk, Used: usedBytes / (1024 * 1024), Limit: limit}
	default:
		return nil
	}
	if err != nil {
		return quotaReadError(err)
	}
	if quotaUnlimited(limit) || used < limit {
		// alpha.118 (D-044): a full disk takes nothing new that grows the
		// subscription (a domain, a mailbox, a database).
		if err := p.checkSubscriptionQuota(ctx, subID, quotaDisk); err != nil {
			return err
		}
		// alpha.117 (K2): the reseller's pool limits the tree's real use.
		return p.checkResellerPoolUsage(ctx, subID, kind)
	}
	return &quotaExceededError{Kind: kind, Used: used, Limit: limit}
}

func quotaReadError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return errQuotaSubscriptionMissing
	}
	return fmt.Errorf("read subscription quota: %w", err)
}

// writeQuotaRefusal answers a failed quota check: a coded 409 naming the
// limit and the next step (free space or ask the provider, from My plan), a
// coded 409 when the subscription is missing, or a server error when the
// numbers could not be read (nothing was created in any case).
// writeQuotaRefusal, başarısız bir kota denetimini yanıtlar.
func writeQuotaRefusal(w http.ResponseWriter, err error) {
	var exceeded *quotaExceededError
	var pool *resellerPoolExceededError
	switch {
	case errors.As(err, &exceeded):
		writeCodedError(w, http.StatusConflict, exceeded.code(), exceeded.Error(), myPlanPagePath)
	case errors.As(err, &pool):
		writeCodedErrorDetails(w, http.StatusConflict, errCodeResellerPoolExceeded, pool.Error(), myPlanPagePath,
			[]string{fmt.Sprintf("%s: %d of %d", pool.Kind, pool.Used, pool.Limit)})
	case errors.Is(err, errSubscriptionSuspendedCreate):
		writeCodedError(w, http.StatusConflict, errCodeSubscriptionSuspended,
			"this subscription is suspended, so nothing new can be added to it; nothing was changed. The administrator or its reseller resumes the subscription first.", "")
	case errors.Is(err, errQuotaSubscriptionMissing):
		writeCodedError(w, http.StatusConflict, errCodeNoSubscription,
			"this account has no subscription yet; ask your provider to assign a plan", myPlanPagePath)
	default:
		writeServerError(w, err)
	}
}

// subscriptionUsage is the real, aggregate resource picture for one
// subscription: measured disk against the plan limit plus the counted
// resources. Every limit of 0 means unlimited.
// subscriptionUsage, bir aboneliğin gerçek, toplu kaynak tablosudur. Her 0
// sınırı sınırsız demektir.
type subscriptionUsage struct {
	DiskUsedBytes  int64 `json:"disk_used_bytes"`
	DiskLimitBytes int64 `json:"disk_limit_bytes"` // 0 = unlimited
	Domains        int   `json:"domains"`
	DomainsLimit   int   `json:"domains_limit"` // 0 = unlimited
	Databases      int   `json:"databases"`
	DatabasesLimit int   `json:"databases_limit"` // 0 = unlimited
	MailAccounts   int   `json:"mail_accounts"`
	MailLimit      int   `json:"mail_limit"` // 0 = unlimited
	// alpha.118 (D-044): this month's website traffic and its limit, how
	// sure the number is, and the disk measurement in parts.
	TrafficUsedBytes  int64                `json:"traffic_used_bytes"`
	TrafficLimitBytes int64                `json:"traffic_limit_bytes"` // 0 = unlimited
	TrafficPeriod     string               `json:"traffic_period"`
	TrafficMeasure    trafficMeasureState  `json:"traffic_measure"`
	TrafficLimited    bool                 `json:"traffic_limited"`
	Disk              subscriptionDiskView `json:"disk"`
}

func (p *Panel) subscriptionUsageFor(ctx context.Context, subID int) (*subscriptionUsage, error) {
	var u subscriptionUsage
	var diskLimitMB, trafficLimitMB int64
	now := usageNow()
	from, to := trafficMonthBounds(now)
	db := p.db.GetDB()
	err := db.QueryRowContext(ctx, `
		SELECT
		  s.max_domains, s.max_databases, s.max_email_accounts, s.disk_quota_mb, COALESCE(s.bandwidth_quota_mb, 0),
		  (SELECT COUNT(*) FROM domains d WHERE d.subscription_id = s.id),
		  (SELECT COUNT(*) FROM databases_v2 db WHERE db.subscription_id = s.id),
		  (SELECT COUNT(*) FROM email_accounts ea JOIN domains d ON ea.domain_id = d.id WHERE d.subscription_id = s.id),
		  `+subscriptionDiskBytesSQL+`,
		  `+subscriptionTrafficSQL+`
		FROM subscriptions s WHERE s.id = ?`, from, to, subID).Scan(
		&u.DomainsLimit, &u.DatabasesLimit, &u.MailLimit, &diskLimitMB, &trafficLimitMB,
		&u.Domains, &u.Databases, &u.MailAccounts, &u.DiskUsedBytes, &u.TrafficUsedBytes)
	if err != nil {
		return nil, err
	}
	if trafficLimitMB > 0 {
		u.TrafficLimitBytes = trafficLimitMB * mib
	}
	u.TrafficPeriod = trafficPeriod(now)
	if u.TrafficMeasure, err = subscriptionTrafficMeasure(ctx, db, subID); err != nil {
		return nil, err
	}
	if u.TrafficLimited, err = subscriptionTrafficLimited(ctx, db, subID); err != nil {
		return nil, err
	}
	if u.Disk, err = loadSubscriptionDiskView(ctx, db, subID); err != nil {
		return nil, err
	}
	// Negative stored values are unlimited too; the answer says 0.
	// Negatif kayıtlı değerler de sınırsızdır; yanıt 0 der.
	if u.DomainsLimit < 0 {
		u.DomainsLimit = 0
	}
	if u.DatabasesLimit < 0 {
		u.DatabasesLimit = 0
	}
	if u.MailLimit < 0 {
		u.MailLimit = 0
	}
	if diskLimitMB > 0 {
		u.DiskLimitBytes = diskLimitMB * 1024 * 1024
	}
	// The administrator's own workspace has no limit; the answer says 0, not 999.
	// Yoneticinin kendi calisma alani sinirsizdir; yanit 999 degil 0 der.
	if p.administratorWorkspace(ctx, subID) {
		u.DomainsLimit, u.DatabasesLimit, u.MailLimit, u.DiskLimitBytes, u.TrafficLimitBytes = 0, 0, 0, 0, 0
	}
	return &u, nil
}

// overLimits lists the kinds whose current usage is above (not at) the
// limit: what a downgrade below usage leaves frozen. Nothing is deleted;
// new creations of these kinds are refused until usage is back under.
// overLimits, mevcut kullanımın sınırın üstünde olduğu türleri listeler.
func (u *subscriptionUsage) overLimits() []string {
	over := []string{}
	if u.DomainsLimit > 0 && u.Domains > u.DomainsLimit {
		over = append(over, fmt.Sprintf("domains: %d in use, limit %d", u.Domains, u.DomainsLimit))
	}
	if u.DatabasesLimit > 0 && u.Databases > u.DatabasesLimit {
		over = append(over, fmt.Sprintf("databases: %d in use, limit %d", u.Databases, u.DatabasesLimit))
	}
	if u.MailLimit > 0 && u.MailAccounts > u.MailLimit {
		over = append(over, fmt.Sprintf("mailboxes: %d in use, limit %d", u.MailAccounts, u.MailLimit))
	}
	if u.DiskLimitBytes > 0 && u.DiskUsedBytes > u.DiskLimitBytes {
		over = append(over, fmt.Sprintf("disk: %d MB in use, limit %d MB", u.DiskUsedBytes/(1024*1024), u.DiskLimitBytes/(1024*1024)))
	}
	if u.TrafficLimitBytes > 0 && u.TrafficUsedBytes > u.TrafficLimitBytes {
		over = append(over, fmt.Sprintf("traffic: %d MB in use, limit %d MB", u.TrafficUsedBytes/mib, u.TrafficLimitBytes/mib))
	}
	return over
}

// domainSubscriptionID resolves the owning subscription of a domain.
// domainSubscriptionID, bir domain'in bağlı olduğu aboneliği çözer.
func (p *Panel) domainSubscriptionID(ctx context.Context, domainID int) (int, error) {
	var subID int
	err := p.db.GetDB().QueryRowContext(ctx,
		`SELECT subscription_id FROM domains WHERE id = ?`, domainID).Scan(&subID)
	return subID, err
}

// enforceDomainQuota is the HTTP-flavoured guard for per-domain resources:
// resolves the subscription and writes a coded 409 when the limit is hit.
// enforceDomainQuota, domain-başına kaynaklar için HTTP korumasıdır.
func (p *Panel) enforceDomainQuota(w http.ResponseWriter, r *http.Request, domainID int, kind quotaKind) bool {
	subID, err := p.domainSubscriptionID(r.Context(), domainID)
	if err != nil {
		writeClientError(w, http.StatusNotFound, "domain not found")
		return false
	}
	if err := p.checkSubscriptionQuota(r.Context(), subID, kind); err != nil {
		writeQuotaRefusal(w, err)
		return false
	}
	return true
}
