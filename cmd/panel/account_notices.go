package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/mutationpayload"
)

// Account notices and notifications by recipient (alpha.128, G11 + G18;
// migration 072's account_notices and notification_preferences).
//
// An account notice is a notice for one person about one thing that is not a
// usage notice (057) or a monitoring alert (065): today a subscription's end
// date coming (expiry_soon) and reached within a day (expiry_today); other
// features raise their own kinds through raiseAccountNotice. One row per
// person, kind, subject and period: a stage is never repeated, and a changed
// subject period (a moved end date) gives fresh stages. needs_action says
// whether this person is the one who acts; fix_path is the panel page where
// the fix is.
//
// Every notice is shown in the panel to its recipient only. Email follows the
// person's own choice (notification_preferences: everything, only what needs
// my action, nothing; plus one extra address) and the administrator's extra
// address per event group (panel_settings notify.extra.<group>), which gets one
// copy per event, not one per administrator. Mail goes through the panel's
// existing account mail path (Agent → the server's own Postfix); a hand-over
// that fails is recorded on the row and never retried in a loop.
//
// Notices are optional panel automation (D-022): with the panel stopped
// nothing is raised, and nothing a hosted service reads changes here.
// Polling a screen never raises or mails anything (D-024); the usage worker
// does, once per round, idempotently.
//
// Hesap bildirimleri: kullanım bildirimi ya da izleme uyarısı olmayan, bir
// kişiye bir konu hakkındaki bildirim (bugün aboneliğin bitiş tarihi).
// Kişi-tür-konu-dönem başına bir satır; aşama tekrarlanmaz. Panelde yalnız
// alıcısına görünür; e-posta kişinin kendi seçimine ve yöneticinin olay
// grubu başına ek adresine göre gider. Ekran sorgusu hiçbir şey başlatmaz.

const (
	noticeExpirySoon  = "expiry_soon"
	noticeExpiryToday = "expiry_today"

	// Notification preference modes (069's CHECK).
	notifyModeAll    = "all"
	notifyModeAction = "action"
	notifyModeNone   = "none"

	// Event groups for the administrator's extra addresses.
	notifyGroupMonitoring = "monitoring"
	notifyGroupAccount    = "account"
	notifyGroupSecurity   = "security"

	settingExpiryDaysBefore = "notify.expiry.days_before"
	settingNotifyExtraFmt   = "notify.extra.%s"
	expiryDaysDefault       = 10

	// accountNoticeMailBatch bounds one round's hand-overs.
	accountNoticeMailBatch = 200
)

// expiryDaysChoices is the administrator's fixed choice list.
var expiryDaysChoices = []int{3, 7, 10, 14, 30}

// notifyGroups lists the administrator's event groups, in screen order.
var notifyGroups = []string{notifyGroupMonitoring, notifyGroupAccount, notifyGroupSecurity}

func validExpiryDays(n int) bool {
	for _, choice := range expiryDaysChoices {
		if n == choice {
			return true
		}
	}
	return false
}

// expiryDaysBefore is the administrator's choice, the default when unset or
// not one of the choices.
func (p *Panel) expiryDaysBefore(ctx context.Context) int {
	if n, err := strconv.Atoi(p.setting(ctx, settingExpiryDaysBefore)); err == nil && validExpiryDays(n) {
		return n
	}
	return expiryDaysDefault
}

// adminGroupExtraEmail is the administrator's extra address for one event
// group ("" when none or unreadable). It is checked again on every read.
func (p *Panel) adminGroupExtraEmail(ctx context.Context, group string) string {
	raw := strings.TrimSpace(p.setting(ctx, fmt.Sprintf(settingNotifyExtraFmt, group)))
	if raw == "" {
		return ""
	}
	canonical, err := mutationpayload.CanonicalContactEmail(raw)
	if err != nil {
		return ""
	}
	return canonical
}

// notificationPreference is one person's email choice.
type notificationPreference struct {
	Mode       string
	ExtraEmail string
}

// notificationPreferenceFor reads a person's choice; no row (or an
// unreadable one) is the default, everything.
func notificationPreferenceFor(ctx context.Context, q rowQueryer, userID int) notificationPreference {
	pref := notificationPreference{Mode: notifyModeAll}
	var mode string
	var extra sql.NullString
	if err := q.QueryRowContext(ctx, `SELECT mode, extra_email FROM notification_preferences WHERE user_id = ?`, userID).Scan(&mode, &extra); err != nil {
		return pref
	}
	switch mode {
	case notifyModeAction, notifyModeNone:
		pref.Mode = mode
	}
	if extra.Valid {
		if canonical, err := mutationpayload.CanonicalContactEmail(extra.String); err == nil {
			pref.ExtraEmail = canonical
		}
	}
	return pref
}

// mailWanted: does this person want this notice emailed?
func (pref notificationPreference) mailWanted(needsAction bool) bool {
	switch pref.Mode {
	case notifyModeNone:
		return false
	case notifyModeAction:
		return needsAction
	}
	return true
}

// mailAddresses: the person's own address (when set) and the extra address,
// without repeats.
func (pref notificationPreference) mailAddresses(own string) []string {
	var out []string
	seen := map[string]bool{}
	for _, raw := range []string{own, pref.ExtraEmail} {
		raw = strings.TrimSpace(raw)
		if raw == "" || seen[strings.ToLower(raw)] {
			continue
		}
		seen[strings.ToLower(raw)] = true
		out = append(out, raw)
	}
	return out
}

// accountNoticeWords is one notice in plain words, English and Turkish: the
// title says what happens, the text says when, who acts and how it resumes.
type accountNoticeWords struct {
	TitleEN string `json:"title_en"`
	TextEN  string `json:"text_en"`
	TitleTR string `json:"title_tr"`
	TextTR  string `json:"text_tr"`
}

// accountNoticeKind describes one kind: its event group (which
// administrator extra address gets a copy) and its words for an audience.
type accountNoticeKind struct {
	Group string
	Words func(audience string, body map[string]any) accountNoticeWords
}

// accountNoticeKinds is the text registry, keyed by kind. Another feature
// adds its kind with registerAccountNoticeKind from its own file's init; a
// kind without an entry still gets the safe default words.
var accountNoticeKinds = map[string]accountNoticeKind{
	noticeExpirySoon: {Group: notifyGroupAccount, Words: func(audience string, body map[string]any) accountNoticeWords {
		return expiryNoticeWords(audience, body, false)
	}},
	noticeExpiryToday: {Group: notifyGroupAccount, Words: func(audience string, body map[string]any) accountNoticeWords {
		return expiryNoticeWords(audience, body, true)
	}},
}

// registerAccountNoticeKind adds a kind's words (from an init function).
func registerAccountNoticeKind(kind string, spec accountNoticeKind) {
	if kind == "" || spec.Words == nil {
		panic("account notice kind needs a name and words")
	}
	if spec.Group == "" {
		spec.Group = notifyGroupAccount
	}
	accountNoticeKinds[kind] = spec
}

func accountNoticeGroup(kind string) string {
	if spec, ok := accountNoticeKinds[kind]; ok && spec.Group != "" {
		return spec.Group
	}
	return notifyGroupAccount
}

// accountNoticeWordsFor: the registered words, or the safe default.
func accountNoticeWordsFor(kind, audience string, body map[string]any) accountNoticeWords {
	if spec, ok := accountNoticeKinds[kind]; ok && spec.Words != nil {
		if words := spec.Words(audience, body); words.TitleEN != "" && words.TitleTR != "" {
			return words
		}
	}
	return accountNoticeWords{
		TitleEN: "A notice about your account",
		TextEN:  "Something about your account needs a look. Open the page with the button to see what happened and what to do.",
		TitleTR: "Hesabınızla ilgili bir bildirim",
		TextTR:  "Hesabınızla ilgili bir konuya bakılması gerekiyor. Ne olduğunu ve ne yapılacağını görmek için düğmeyle sayfayı açın.",
	}
}

func bodyString(body map[string]any, key string) string {
	if v, ok := body[key].(string); ok {
		return v
	}
	return ""
}

func bodyBool(body map[string]any, key string) bool {
	v, _ := body[key].(bool)
	return v
}

// validNoticeFixPath keeps only a local panel path (069's CHECK).
func validNoticeFixPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || len(path) > 300 || path[0] != '/' || (len(path) > 1 && (path[1] == '/' || path[1] == '\\')) ||
		strings.ContainsAny(path, "\r\n\t") {
		return ""
	}
	return path
}

// raiseAccountNotice stores one notice per recipient; one already stored
// for the same person, kind, subject and period is left as it is (a stage is
// never repeated). needsAction and fixPath apply to every recipient given:
// a caller whose audiences differ (the customer is told whom to contact, the
// reseller acts) calls it once per group. body holds only facts for the
// words (no secret; at most 8 KiB as JSON). Mail is handed over by the usage
// worker's next round. Errors are logged, never returned: a notice must not
// fail the change that raised it.
//
// raiseAccountNotice her alıcıya bir bildirim yazar; aynı kişi, tür, konu ve
// dönem için yazılmış olan olduğu gibi kalır. E-posta işçinin sonraki
// turunda gider.
func (p *Panel) raiseAccountNotice(ctx context.Context, q sqlQueryer, recipients []usageRecipient, kind string, needsAction bool, subjectKey, period string, body map[string]any, fixPath string) {
	if kind == "" || len(kind) > 64 || period == "" || len(period) > 64 || len(subjectKey) > 200 {
		log.Printf("[notices] %q: kind, subject or period out of bounds; not stored", kind)
		return
	}
	if body == nil {
		body = map[string]any{}
	}
	raw, err := json.Marshal(body)
	if err != nil || len(raw) > 8192 {
		log.Printf("[notices] %s: body not stored as given (%v, %d bytes)", kind, err, len(raw))
		raw = []byte("{}")
	}
	action := 0
	if needsAction {
		action = 1
	}
	fixPath = validNoticeFixPath(fixPath)
	for _, recipient := range recipients {
		if recipient.UserID <= 0 {
			continue
		}
		audience := recipient.Audience
		switch audience {
		case audienceCustomer, audienceReseller, audienceAdministrator:
		default:
			audience = audienceCustomer
		}
		if _, err := q.ExecContext(ctx, `
			INSERT INTO account_notices (user_id, audience, kind, needs_action, subject_key, period, body, fix_path)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
			recipient.UserID, audience, kind, action, subjectKey, period, string(raw), fixPath); err != nil {
			log.Printf("[notices] store %s for user %d: %v", kind, recipient.UserID, err)
		}
	}
}

// accountNoticeMailText is one notice as an email: English, then Turkish,
// then the page to open.
func accountNoticeMailText(words accountNoticeWords, baseURL, fixPath string) (string, string) {
	subject := words.TitleEN + " / " + words.TitleTR
	link := baseURL
	if fixPath != "" && baseURL != "" {
		link = strings.TrimRight(baseURL, "/") + fixPath
	}
	return subject, strings.Join([]string{words.TextEN, "", words.TextTR, "", link}, "\n")
}

// sendPendingAccountNotices hands pending notices to the mail system, or
// records why not: not_wanted (the person's choice), not_available (no mail
// system), no_address, failed. The administrator's extra address for the
// kind's group gets one copy of each event (the event's first row).
func (p *Panel) sendPendingAccountNotices(ctx context.Context) error {
	db := p.db.GetDB()
	rows, err := db.QueryContext(ctx, `
		SELECT n.id, n.user_id, n.kind, n.audience, n.needs_action, n.body, n.fix_path, COALESCE(u.email, ''),
		       n.id = (SELECT MIN(m.id) FROM account_notices m WHERE m.kind = n.kind AND m.subject_key = n.subject_key AND m.period = n.period)
		FROM account_notices n LEFT JOIN users u ON u.id = n.user_id
		WHERE n.mail_state = 'pending' ORDER BY n.id LIMIT ?`, accountNoticeMailBatch)
	if err != nil {
		return err
	}
	type pending struct {
		id, userID                           int
		kind, audience, body, fixPath, email string
		needsAction, firstOfEvent            bool
	}
	var items []pending
	for rows.Next() {
		var item pending
		if err := rows.Scan(&item.id, &item.userID, &item.kind, &item.audience, &item.needsAction, &item.body, &item.fixPath, &item.email, &item.firstOfEvent); err != nil {
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
		if _, err := db.ExecContext(ctx, `UPDATE account_notices SET mail_state = ?, mail_detail = ? WHERE id = ?`, state, nullIfEmpty(detail), id); err != nil {
			log.Printf("[notices] notice %d mail state: %v", id, err)
		}
	}
	groupCopies := map[string]string{}
	for _, item := range items {
		var body map[string]any
		if err := json.Unmarshal([]byte(item.body), &body); err != nil {
			mark(item.id, "failed", "the notice could not be read")
			continue
		}
		// One copy of the event to the administrator's group address,
		// whatever the people's own choices.
		if item.firstOfEvent && setup.Available {
			group := accountNoticeGroup(item.kind)
			to, ok := groupCopies[group]
			if !ok {
				to = p.adminGroupExtraEmail(ctx, group)
				groupCopies[group] = to
			}
			if to != "" {
				subject, text := accountNoticeMailText(accountNoticeWordsFor(item.kind, audienceAdministrator, body), setup.baseURL, "")
				if err := p.sendAccountMail(ctx, setup, to, accountMailContent{kind: accountMailAccountNotice, subject: subject, body: text}); err != nil {
					log.Printf("[notices] group copy of notice %d: %s", item.id, boundedAgentDiagnostic(err.Error()))
				}
			}
		}
		pref := notificationPreferenceFor(ctx, db, item.userID)
		if !pref.mailWanted(item.needsAction) {
			mark(item.id, "not_wanted", "")
			continue
		}
		if !setup.Available {
			mark(item.id, "not_available", setup.Reason)
			continue
		}
		addresses := pref.mailAddresses(item.email)
		if len(addresses) == 0 {
			mark(item.id, "no_address", "")
			continue
		}
		subject, text := accountNoticeMailText(accountNoticeWordsFor(item.kind, item.audience, body), setup.baseURL, item.fixPath)
		sent, failure := 0, ""
		for _, to := range addresses {
			if err := p.sendAccountMail(ctx, setup, to, accountMailContent{kind: accountMailAccountNotice, subject: subject, body: text}); err != nil {
				failure = boundedAgentDiagnostic(err.Error())
				continue
			}
			sent++
		}
		if sent == 0 {
			mark(item.id, "failed", failure)
			continue
		}
		mark(item.id, "sent", "")
	}
	return nil
}

// --- G11: subscription expiry ------------------------------------------------

// endsAtLayout is how 052's ends_at is stored (UTC).
const endsAtLayout = "2006-01-02 15:04:05"

// expiryStage: which stage an end date is in at now: expiry_today within
// the last 24 hours before it, expiry_soon within the administrator's days,
// "" otherwise (further away, or passed: the suspension takes over).
func expiryStage(endsAt, now time.Time, daysBefore int) string {
	left := endsAt.Sub(now)
	switch {
	case left <= 0:
		return ""
	case left <= 24*time.Hour:
		return noticeExpiryToday
	case left <= time.Duration(daysBefore)*24*time.Hour:
		return noticeExpirySoon
	}
	return ""
}

// raiseExpiryNotices is the usage worker's step: a notice to the
// subscription's owner, its reseller and the administrators when its end
// date is near, and again within its last day. A notice of an end date that
// changed, was cleared or has passed is marked as seen (it no longer holds).
// Cheap: one read of the subscriptions whose end date is in the window (a
// small table; the retire step looks each notice's subscription up by its
// key), and the recipients are read only for a stage not stored yet.
//
// Bitiş tarihi yaklaşınca ve son gününde sahibine, bayisine ve yöneticilere
// bildirim; tarihi değişen, kaldırılan ya da geçen bildirim görüldü sayılır.
func (p *Panel) raiseExpiryNotices(ctx context.Context, now time.Time) error {
	db := p.db.GetDB()
	nowText := now.UTC().Format(endsAtLayout)
	if _, err := db.ExecContext(ctx, `
		UPDATE account_notices SET seen_at = datetime('now')
		WHERE kind IN (?, ?) AND seen_at IS NULL AND NOT EXISTS (
			SELECT 1 FROM subscriptions s
			WHERE s.id = CAST(substr(account_notices.subject_key, 14) AS INTEGER)
			  AND 'subscription:' || s.id = account_notices.subject_key AND s.ends_at = account_notices.period
			  AND julianday(s.ends_at) > julianday(?))`, noticeExpirySoon, noticeExpiryToday, nowText); err != nil {
		return err
	}
	days := p.expiryDaysBefore(ctx)
	rows, err := db.QueryContext(ctx, `
		SELECT id, ends_at FROM subscriptions
		WHERE ends_at IS NOT NULL AND julianday(ends_at) > julianday(?) AND julianday(ends_at) <= julianday(?)
		ORDER BY id LIMIT 1000`, nowText, now.UTC().Add(time.Duration(days)*24*time.Hour).Format(endsAtLayout))
	if err != nil {
		return err
	}
	type due struct {
		id     int
		endsAt string
		stage  string
	}
	var items []due
	for rows.Next() {
		var id int
		var endsAt string
		if err := rows.Scan(&id, &endsAt); err != nil {
			rows.Close()
			return err
		}
		at, err := time.Parse(endsAtLayout, endsAt)
		if err != nil {
			continue
		}
		if stage := expiryStage(at, now, days); stage != "" {
			items = append(items, due{id, endsAt, stage})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, it := range items {
		subjectKey := "subscription:" + strconv.Itoa(it.id)
		var stored int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM account_notices WHERE kind = ? AND subject_key = ? AND period = ?`,
			it.stage, subjectKey, it.endsAt).Scan(&stored); err != nil {
			return err
		}
		if stored > 0 {
			continue
		}
		if err := p.raiseExpiryNotice(ctx, db, it.id, it.endsAt, it.stage); err != nil {
			log.Printf("[notices] expiry of subscription %d: %v", it.id, err)
		}
	}
	return nil
}

// raiseExpiryNotice writes one stage for one subscription. The owner is
// told whom to contact (needs_action 0, their own plan page); the reseller
// who manages it acts (the account's rights, where the end date is); the
// administrators act when no reseller is between them and the owner, and
// are informed otherwise.
func (p *Panel) raiseExpiryNotice(ctx context.Context, db *sql.DB, subscriptionID int, endsAt, stage string) error {
	recipients, err := usageRecipientsFor(ctx, db, subscriptionID)
	if err != nil {
		return err
	}
	ownerID, resellerID, err := subscriptionOwnerChain(ctx, db, subscriptionID)
	if err != nil {
		return err
	}
	name, owner, reseller := subscriptionNoticeNames(ctx, db, subscriptionID)
	ownerIsReseller := ownerID == resellerID && resellerID > 0
	if ownerIsReseller {
		reseller = ""
	}
	body := map[string]any{
		"subscription":      name,
		"owner":             owner,
		"reseller":          reseller,
		"ends_at":           endsAt,
		"owner_is_reseller": ownerIsReseller,
	}
	subjectKey := "subscription:" + strconv.Itoa(subscriptionID)
	manage := "/users?rights=" + url.QueryEscape(owner)
	if owner == "" {
		manage = "/users"
	}
	var ownerSide, resellerSide, adminSide []usageRecipient
	for _, r := range recipients {
		switch {
		case r.UserID == ownerID && r.Audience != audienceAdministrator:
			ownerSide = append(ownerSide, r)
		case r.Audience == audienceAdministrator:
			adminSide = append(adminSide, r)
		default:
			resellerSide = append(resellerSide, r)
		}
	}
	p.raiseAccountNotice(ctx, db, ownerSide, stage, false, subjectKey, endsAt, body, "/account")
	p.raiseAccountNotice(ctx, db, resellerSide, stage, true, subjectKey, endsAt, body, manage)
	p.raiseAccountNotice(ctx, db, adminSide, stage, len(resellerSide) == 0, subjectKey, endsAt, body, manage)
	return nil
}

// expiryNoticeWords: what happens on the end date (every service of the
// subscription is suspended, nothing is deleted), who acts and how it
// resumes, for the owner, the reseller who manages it and the administrator.
// today is the last day's stage.
func expiryNoticeWords(audience string, body map[string]any, today bool) accountNoticeWords {
	name := bodyString(body, "subscription")
	owner := bodyString(body, "owner")
	reseller := bodyString(body, "reseller")
	when := bodyString(body, "ends_at")
	if at, err := time.Parse(endsAtLayout, when); err == nil {
		when = at.Format("2006-01-02 15:04") + " UTC"
	}
	ownerIsReseller := bodyBool(body, "owner_is_reseller")
	isOwner := audience == audienceCustomer || (audience == audienceReseller && ownerIsReseller)
	what := name
	if owner != "" && !isOwner {
		what = name + " (" + owner + ")"
	}
	happensEN := "On that date every service of it is suspended: its websites show \"This site has been suspended\"; mail, databases, file access and scheduled tasks stop. Nothing is deleted."
	happensTR := "O tarihte bütün hizmetleri askıya alınır: web siteleri \"Bu site askıya alınmıştır\" gösterir; posta, veritabanları, dosya erişimi ve zamanlanmış görevler durur. Hiçbir şey silinmez."
	if ownerIsReseller {
		happensEN += " The subscriptions of the reseller's customers are suspended too."
		happensTR += " Bayinin müşterilerinin abonelikleri de askıya alınır."
	}
	var w accountNoticeWords
	if isOwner {
		contactEN, contactTR := "the administrator of this server", "bu sunucunun yöneticisi"
		if reseller != "" && !ownerIsReseller {
			contactEN, contactTR = "your provider ("+reseller+")", "sağlayıcınız ("+reseller+")"
		}
		w.TitleEN, w.TitleTR = "Your subscription ends on "+when+": "+name, "Aboneliğiniz "+when+" tarihinde bitiyor: "+name
		if today {
			w.TitleEN, w.TitleTR = "Your subscription ends within a day ("+when+"): "+name, "Aboneliğiniz bir gün içinde bitiyor ("+when+"): "+name
		}
		w.TextEN = fmt.Sprintf("Your subscription %s ends on %s. %s Who acts: %s extends the end date; ask them before then. If it has been suspended, everything comes back as it was once the end date is moved later.", name, when, happensEN, contactEN)
		w.TextTR = fmt.Sprintf("%s aboneliğiniz %s tarihinde bitiyor. %s Kim yapar: bitiş tarihini %s uzatır; o zamandan önce isteyin. Askıya alındıysa bitiş tarihi ileri alınınca her şey olduğu gibi geri gelir.", name, when, happensTR, contactTR)
		return w
	}
	w.TitleEN, w.TitleTR = "Subscription ends on "+when+": "+what, "Abonelik "+when+" tarihinde bitiyor: "+what
	if today {
		w.TitleEN, w.TitleTR = "Subscription ends within a day ("+when+"): "+what, "Abonelik bir gün içinde bitiyor ("+when+"): "+what
	}
	if audience == audienceAdministrator && reseller != "" {
		w.TextEN = fmt.Sprintf("For your information: the subscription %s ends on %s. %s Its reseller %s manages the end date and was told; you can also change it in the account's rights.", what, when, happensEN, reseller)
		w.TextTR = fmt.Sprintf("Bilginize: %s aboneliği %s tarihinde bitiyor. %s Bitiş tarihini bayisi %s yönetir ve bilgilendirildi; siz de hesabın haklarından değiştirebilirsiniz.", what, when, happensTR, reseller)
		return w
	}
	w.TextEN = fmt.Sprintf("The subscription %s ends on %s. %s Who acts: you. To keep it running, open the account's rights and change or clear its end date; the owner was told to contact you. If it has been suspended, moving the date later resumes it.", what, when, happensEN)
	w.TextTR = fmt.Sprintf("%s aboneliği %s tarihinde bitiyor. %s Kim yapar: siz. Çalışmaya devam etmesi için hesabın haklarını açıp bitiş tarihini değiştirin ya da kaldırın; sahibine sizinle iletişime geçmesi söylendi. Askıya alındıysa tarihi ileri almak onu yeniden açar.", what, when, happensTR)
	return w
}
