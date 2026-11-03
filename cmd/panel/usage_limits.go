package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

// Plan limits made real (alpha.118, D-044; owner decisions of 3 Nov 2026).
//
// Traffic: the bytes every website sends to its visitors, per calendar month
// (server time), measured from the small traffic logs nginx writes
// (traffic_collect.go). At 100% of a subscription's limit, or when the real
// total of its reseller's customers fills the reseller's pool (K2), only the
// subscription's websites show "traffic limit reached"; mail, databases and
// file access keep working. The sites open again by themselves on the 1st or
// as soon as the limit is raised. Collection runs every few minutes, so a
// site can go over its limit by what it sends in one interval; the screens
// say so.
//
// Disk: the real size of the subscription (site files, mail, databases),
// measured by the Agent at low priority (disk_usage.go). At 100% nothing new
// that grows the subscription can be created and uploads stop; the sites keep
// running. Seven days over the limit suspend the subscription with the
// alpha.117 suspension, cause "disk"; it resumes by itself when a
// measurement is under the limit, and never clears another cause.
//
// Notices at 80% and 100% (and on the disk countdown) go to the customer, its
// reseller and the administrator, in the panel and by e-mail when the
// panel's account mail is available.
//
// Trafik ve disk sınırları gerçek olur. Trafik dolunca yalnız web siteleri
// kapanır; ayın 1'inde ya da sınır yükselince kendiliğinden açılır. Disk
// dolunca yeni oluşturma ve yükleme durur; 7 gün aşımda abonelik askıya
// alınır, sınırın altına inince kendiliğinden açılır.

const (
	trafficCauseSubscription = "subscription"
	trafficCausePool         = "pool"

	noticeTraffic80      = "traffic_80"
	noticeTraffic100     = "traffic_100"
	noticePoolTraffic80  = "pool_traffic_80"
	noticePoolTraffic100 = "pool_traffic_100"
	noticeDisk80         = "disk_80"
	noticeDisk100        = "disk_100"
	noticeDiskCountdown  = "disk_countdown"
	noticeDiskSuspended  = "disk_suspended"

	audienceCustomer      = "customer"
	audienceReseller      = "reseller"
	audienceAdministrator = "administrator"

	// diskSuspendAfter: over the disk limit this long suspends.
	diskSuspendAfter = 7 * 24 * time.Hour
	// diskCountdownNotice: the countdown notice goes this long before.
	diskCountdownNotice = 2 * 24 * time.Hour

	mib = int64(1024 * 1024)
)

// usageNow is the clock of the limits (a test seam).
var usageNow = time.Now

// trafficPeriod is the calendar month of t in server time ("2006-01").
func trafficPeriod(t time.Time) string { return t.In(time.Local).Format("2006-01") }

// trafficMonthBounds: the first day of t's month and of the next one.
func trafficMonthBounds(t time.Time) (string, string) {
	local := t.In(time.Local)
	first := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, time.Local)
	return first.Format("2006-01-02"), first.AddDate(0, 1, 0).Format("2006-01-02")
}

// subscriptionDiskBytesSQL is the real disk use of subscription s: the last
// measurement, or (before the first one) the per-site numbers the domain
// pages cached, or 0.
const subscriptionDiskBytesSQL = `COALESCE(
	(SELECT u.web_bytes + u.mail_bytes + u.db_bytes FROM subscription_disk_usage u
	  WHERE u.subscription_id = s.id AND u.measured_at IS NOT NULL),
	(SELECT SUM(si.disk_usage_bytes) FROM sites si JOIN domains d ON si.domain_id = d.id WHERE d.subscription_id = s.id),
	0)`

// subscriptionTrafficSQL is subscription s's bytes this month (two
// arguments: the month's first day and the next month's first day).
const subscriptionTrafficSQL = `COALESCE((SELECT SUM(t.bytes_out) FROM traffic_daily t
	WHERE t.subscription_id = s.id AND t.day >= ? AND t.day < ?), 0)`

// subscriptionTrafficLimited: its websites show the traffic limit page.
func subscriptionTrafficLimited(ctx context.Context, q rowQueryer, subscriptionID int) (bool, error) {
	var wanted int
	err := q.QueryRowContext(ctx, `SELECT wanted FROM traffic_web_blocks WHERE subscription_id = ? AND period = ?`,
		subscriptionID, trafficPeriod(usageNow())).Scan(&wanted)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return wanted == 1, err
}

// subscriptionTrafficMonth is the subscription's bytes this month.
func subscriptionTrafficMonth(ctx context.Context, q rowQueryer, subscriptionID int, now time.Time) (int64, error) {
	from, to := trafficMonthBounds(now)
	var used int64
	err := q.QueryRowContext(ctx, `SELECT `+subscriptionTrafficSQL+` FROM subscriptions s WHERE s.id = ?`, from, to, subscriptionID).Scan(&used)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return used, err
}

// treeTrafficMonth is the real traffic of a reseller's whole tree this month.
func treeTrafficMonth(ctx context.Context, q rowQueryer, resellerID int, now time.Time) (int64, error) {
	from, to := trafficMonthBounds(now)
	var used int64
	err := q.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(t.bytes_out), 0) FROM traffic_daily t
		WHERE t.day >= ? AND t.day < ? AND t.subscription_id IN (`+treeSubscriptionsSQL+`)`,
		from, to, resellerID, resellerID).Scan(&used)
	return used, err
}

// trafficMeasureState is what the screens say about the measurement of a
// subscription's traffic: "measuring", "unknown" (with a reason code) or
// "none" (no website).
type trafficMeasureState struct {
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
	// LastCollected is the last successful collection (UTC), "" when none.
	LastCollected string `json:"last_collected,omitempty"`
	// CollectError is the last collection's failure, "" when it worked.
	CollectError string `json:"collect_error,omitempty"`
}

func subscriptionTrafficMeasure(ctx context.Context, q rowQueryer, subscriptionID int) (trafficMeasureState, error) {
	var state trafficMeasureState
	var last, lastErr sql.NullString
	if err := q.QueryRowContext(ctx, `SELECT last_success_at, last_error FROM traffic_collect_state WHERE id = 1`).Scan(&last, &lastErr); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return state, err
	}
	state.LastCollected, state.CollectError = last.String, lastErr.String
	rows, err := q.QueryContext(ctx, `
		SELECT d.id, COALESCE(ts.state, ''), COALESCE(ts.reason, '')
		FROM domains d JOIN sites si ON si.domain_id = d.id
		LEFT JOIN traffic_sources ts ON ts.domain_id = d.id
		WHERE d.subscription_id = ?`, subscriptionID)
	if err != nil {
		return state, err
	}
	defer rows.Close()
	sites, measuring := 0, 0
	for rows.Next() {
		var id int
		var srcState, reason string
		if err := rows.Scan(&id, &srcState, &reason); err != nil {
			return state, err
		}
		sites++
		switch srcState {
		case "measuring", "not_counted":
			measuring++
		case "":
			if state.Reason == "" {
				state.Reason = "not_collected_yet"
			}
		default:
			if state.Reason == "" || state.Reason == "not_collected_yet" {
				state.Reason = reason
			}
		}
	}
	if err := rows.Err(); err != nil {
		return state, err
	}
	switch {
	case sites == 0:
		state.State = "none"
	case measuring == sites && state.CollectError == "":
		state.State = "measuring"
	default:
		state.State = "unknown"
		if state.Reason == "" {
			state.Reason = "collection_failed"
		}
	}
	return state, nil
}

// usageRecipients: who is told about one subscription: its owner (customer
// or reseller), the owner's reseller, and every administrator.
type usageRecipient struct {
	UserID   int
	Audience string
}

func usageRecipientsFor(ctx context.Context, q sqlQueryer, subscriptionID int) ([]usageRecipient, error) {
	ownerID, resellerID, err := subscriptionOwnerChain(ctx, q, subscriptionID)
	if err != nil {
		return nil, err
	}
	var out []usageRecipient
	var ownerRole string
	_ = q.QueryRowContext(ctx, `SELECT role FROM users WHERE id = ?`, ownerID).Scan(&ownerRole)
	if ownerRole != roleAdmin {
		audience := audienceCustomer
		if ownerID == resellerID {
			audience = audienceReseller
		}
		out = append(out, usageRecipient{ownerID, audience})
	}
	if resellerID > 0 && resellerID != ownerID {
		out = append(out, usageRecipient{resellerID, audienceReseller})
	}
	admins, err := administratorIDs(ctx, q)
	if err != nil {
		return nil, err
	}
	for _, id := range admins {
		out = append(out, usageRecipient{id, audienceAdministrator})
	}
	return out, nil
}

func administratorIDs(ctx context.Context, q sqlQueryer) ([]int, error) {
	rows, err := q.QueryContext(ctx, `SELECT id FROM users WHERE role = 'admin' AND COALESCE(NULLIF(account_type, ''), 'account') = 'account' ORDER BY id`)
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

// usageNoticeBody is the facts of one notice (no secret).
type usageNoticeBody struct {
	Subscription string `json:"subscription,omitempty"`
	Owner        string `json:"owner,omitempty"`
	Reseller     string `json:"reseller,omitempty"`
	UsedBytes    int64  `json:"used_bytes"`
	LimitBytes   int64  `json:"limit_bytes"`
	Period       string `json:"period"`
	// Cause: "subscription" or "pool" (traffic).
	Cause string `json:"cause,omitempty"`
	// SuspendOn: the day a disk suspension happens unless usage goes down.
	SuspendOn string `json:"suspend_on,omitempty"`
}

// storeUsageNotices writes one notice per recipient; one already stored for
// the same kind, subscription, reseller and period is left as it is.
func (p *Panel) storeUsageNotices(ctx context.Context, q sqlQueryer, recipients []usageRecipient, kind string, subscriptionID, resellerID int, period string, body usageNoticeBody) {
	raw, err := json.Marshal(body)
	if err != nil {
		return
	}
	for _, recipient := range recipients {
		if recipient.UserID <= 0 {
			continue
		}
		if _, err := q.ExecContext(ctx, `
			INSERT INTO usage_notices (user_id, audience, kind, subscription_id, reseller_id, period, body)
			VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
			recipient.UserID, recipient.Audience, kind, subscriptionID, resellerID, period, string(raw)); err != nil {
			log.Printf("[usage] store notice %s for user %d: %v", kind, recipient.UserID, err)
		}
	}
}

// subscriptionNoticeNames: the subscription's name, its owner and reseller.
func subscriptionNoticeNames(ctx context.Context, q sqlQueryer, subscriptionID int) (name, owner, reseller string) {
	_ = q.QueryRowContext(ctx, `
		SELECT COALESCE(s.name, ''), COALESCE(u.username, ''), COALESCE(r.username, '')
		FROM subscriptions s JOIN users u ON u.id = s.owner_id
		LEFT JOIN users r ON r.id = u.parent_id AND r.role = 'reseller'
		WHERE s.id = ?`, subscriptionID).Scan(&name, &owner, &reseller)
	return
}

// usageFraction: used/limit in tenths of a percent band: 0 under 80%, 80, 100.
func usageLevel(used, limit int64) int {
	if limit <= 0 {
		return 0
	}
	switch {
	case used >= limit:
		return 100
	case used*10 >= limit*8:
		return 80
	}
	return 0
}

// formatMB is bytes as whole megabytes for a sentence.
func formatMB(bytes int64) string {
	return fmt.Sprintf("%d MB", bytes/mib)
}

// usageNoticeSubject is a notice's mail subject and body (English and
// Turkish, plain words: what happened, what keeps working, what to do).
func usageNoticeMail(kind, audience string, body usageNoticeBody, baseURL string) (string, string) {
	who := body.Subscription
	if body.Owner != "" && audience != audienceCustomer {
		who = body.Subscription + " (" + body.Owner + ")"
	}
	used, limit := formatMB(body.UsedBytes), formatMB(body.LimitBytes)
	var subject, en, tr string
	switch kind {
	case noticeTraffic80:
		subject = "Traffic 80% used / Trafik %80 kullanıldı: " + who
		en = fmt.Sprintf("The websites of %s have used %s of their %s monthly traffic. At 100%% only the websites show \"traffic limit reached\" until the 1st of next month; mail and databases keep working. To avoid it, ask for a larger plan.", who, used, limit)
		tr = fmt.Sprintf("%s web sitelerinin aylık %s trafiğinin %s kadarı kullanıldı. %%100 olunca gelecek ayın 1'ine dek yalnız web siteleri \"trafik sınırı doldu\" gösterir; posta ve veritabanları çalışır. Önlemek için daha büyük bir plan isteyin.", who, limit, used)
	case noticeTraffic100:
		subject = "Traffic limit reached / Trafik sınırı doldu: " + who
		cause := "its own monthly traffic"
		causeTR := "kendi aylık trafiği"
		if body.Cause == trafficCausePool {
			cause = "the shared traffic of its provider (reseller pool)"
			causeTR = "sağlayıcısının ortak trafiği (bayi havuzu)"
		}
		en = fmt.Sprintf("The websites of %s now show \"traffic limit reached\": %s is used up (%s of %s). Mail, databases and file access keep working. The websites open again by themselves on the 1st of next month, or within minutes after the limit is raised. Who acts: the customer asks the provider for a larger plan; the provider or administrator raises the limit.", who, cause, used, limit)
		tr = fmt.Sprintf("%s web siteleri artık \"trafik sınırı doldu\" gösteriyor: %s doldu (%s / %s). Posta, veritabanları ve dosya erişimi çalışır. Siteler gelecek ayın 1'inde ya da sınır yükseltildikten birkaç dakika sonra kendiliğinden açılır. Kim yapar: müşteri sağlayıcısından daha büyük plan ister; sağlayıcı ya da yönetici sınırı yükseltir.", who, causeTR, used, limit)
	case noticePoolTraffic80:
		subject = "Reseller traffic pool 80% used / Bayi trafik havuzu %80: " + body.Reseller
		en = fmt.Sprintf("The customers of %s have used %s of the %s monthly traffic pool. At 100%% the websites of all of its customers show \"traffic limit reached\" until the 1st. The administrator can raise the pool.", body.Reseller, used, limit)
		tr = fmt.Sprintf("%s müşterileri aylık %s trafik havuzunun %s kadarını kullandı. %%100 olunca 1'ine dek bütün müşterilerinin web siteleri \"trafik sınırı doldu\" gösterir. Yönetici havuzu yükseltebilir.", body.Reseller, limit, used)
	case noticePoolTraffic100:
		subject = "Reseller traffic pool used up / Bayi trafik havuzu doldu: " + body.Reseller
		en = fmt.Sprintf("The customers of %s have used the whole %s monthly traffic pool (%s). Their websites show \"traffic limit reached\" until the 1st of next month or until the administrator raises the pool; mail and databases keep working.", body.Reseller, limit, used)
		tr = fmt.Sprintf("%s müşterileri aylık %s trafik havuzunun tamamını kullandı (%s). Web siteleri gelecek ayın 1'ine ya da yönetici havuzu yükseltene dek \"trafik sınırı doldu\" gösterir; posta ve veritabanları çalışır.", body.Reseller, limit, used)
	case noticeDisk80:
		subject = "Disk 80% used / Disk %80 dolu: " + who
		en = fmt.Sprintf("%s uses %s of its %s disk space (site files, mail and databases). At 100%% nothing new can be created or uploaded. Delete what is no longer needed, or ask for a larger plan.", who, used, limit)
		tr = fmt.Sprintf("%s, %s disk alanının %s kadarını kullanıyor (site dosyaları, posta ve veritabanları). %%100 olunca yeni bir şey oluşturulamaz ve yüklenemez. Gerekmeyenleri silin ya da daha büyük plan isteyin.", who, limit, used)
	case noticeDisk100:
		subject = "Disk full / Disk doldu: " + who
		en = fmt.Sprintf("%s uses %s of its %s disk space. Nothing new can be created and uploads are refused; the websites keep running. If it stays over the limit, the subscription is suspended on %s. Delete files, mail or databases you no longer need, or ask the provider for a larger plan; it is measured again within hours.", who, used, limit, body.SuspendOn)
		tr = fmt.Sprintf("%s, %s disk alanının %s kadarını kullanıyor. Yeni bir şey oluşturulamaz, yükleme reddedilir; web siteleri çalışmaya devam eder. Sınırın üstünde kalırsa abonelik %s tarihinde askıya alınır. Gerekmeyen dosya, posta ya da veritabanlarını silin veya sağlayıcıdan daha büyük plan isteyin; birkaç saat içinde yeniden ölçülür.", who, limit, used, body.SuspendOn)
	case noticeDiskCountdown:
		subject = "Suspension in 2 days: disk full / 2 gün sonra askı: disk dolu: " + who
		en = fmt.Sprintf("%s has been over its %s disk space for 5 days (%s used). It is suspended on %s unless usage goes under the limit or the limit is raised.", who, limit, used, body.SuspendOn)
		tr = fmt.Sprintf("%s beş gündür %s disk alanının üstünde (%s kullanılıyor). Kullanım sınırın altına inmez ya da sınır yükseltilmezse %s tarihinde askıya alınır.", who, limit, used, body.SuspendOn)
	case noticeDiskSuspended:
		subject = "Suspended: disk over the limit for 7 days / Askıya alındı: disk 7 gün sınır üstünde: " + who
		en = fmt.Sprintf("%s was suspended because it used more than its %s disk space for 7 days (%s). Nothing was deleted. It resumes by itself when a measurement is under the limit: the provider raises the limit, or resumes it to give 7 more days to free space.", who, limit, used)
		tr = fmt.Sprintf("%s, 7 gün boyunca %s disk alanını aştığı için askıya alındı (%s). Hiçbir şey silinmedi. Ölçüm sınırın altına inince kendiliğinden açılır: sağlayıcı sınırı yükseltir ya da yer açmak için 7 gün daha vermek üzere aboneliği açar.", who, limit, used)
	default:
		return "", ""
	}
	text := strings.Join([]string{en, "", tr, "", baseURL}, "\n")
	return subject, text
}

// sendPendingUsageNotices hands pending notices to the mail system, or
// records why it could not.
func (p *Panel) sendPendingUsageNotices(ctx context.Context) error {
	db := p.db.GetDB()
	rows, err := db.QueryContext(ctx, `
		SELECT n.id, n.kind, n.audience, n.body, COALESCE(u.email, '')
		FROM usage_notices n LEFT JOIN users u ON u.id = n.user_id
		WHERE n.mail_state = 'pending' ORDER BY n.id LIMIT 200`)
	if err != nil {
		return err
	}
	type pending struct {
		id                          int
		kind, audience, body, email string
	}
	var items []pending
	for rows.Next() {
		var item pending
		if err := rows.Scan(&item.id, &item.kind, &item.audience, &item.body, &item.email); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(items) == 0 {
		return err
	}
	setup := p.accountMailSetup(ctx)
	mark := func(id int, state, detail string) {
		if _, err := db.ExecContext(ctx, `UPDATE usage_notices SET mail_state = ?, mail_detail = ? WHERE id = ?`, state, nullIfEmpty(detail), id); err != nil {
			log.Printf("[usage] notice %d mail state: %v", id, err)
		}
	}
	for _, item := range items {
		if !setup.Available {
			mark(item.id, "not_available", setup.Reason)
			continue
		}
		if strings.TrimSpace(item.email) == "" {
			mark(item.id, "no_address", "")
			continue
		}
		var body usageNoticeBody
		if err := json.Unmarshal([]byte(item.body), &body); err != nil {
			mark(item.id, "failed", "the notice could not be read")
			continue
		}
		subject, text := usageNoticeMail(item.kind, item.audience, body, setup.baseURL)
		if subject == "" {
			mark(item.id, "failed", "unknown notice kind")
			continue
		}
		if err := p.sendAccountMail(ctx, setup, strings.TrimSpace(item.email), accountMailContent{kind: accountMailUsageNotice, subject: subject, body: text}); err != nil {
			mark(item.id, "failed", boundedAgentDiagnostic(err.Error()))
			continue
		}
		mark(item.id, "sent", "")
	}
	return nil
}
