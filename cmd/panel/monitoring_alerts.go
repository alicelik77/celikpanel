package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/mutationpayload"
)

// Alerts (alpha.127, monitoring stage 3; owner decisions of 5 Nov 2026).
//
// An alert is a "Problems now" item that lasts. Once a minute the sampler
// computes the same list the screens show and keeps one row per problem key
// (monitoring_alerts, migration 065):
//
//   - pending: seen, not yet past the owner's delay (default 10 minutes); a
//     problem that goes away before then never becomes an alert. A problem
//     that already says since when it is there (sustained load, a site down
//     since its first failed check) counts from then.
//   - open: past the delay. While the problem lasts it stays this one row
//     (the key de-duplicates); a problem that comes back within the clear
//     delay is the same alert (hysteresis), and the disk alert clears only
//     under 80 % (5 points under its level).
//   - resolved: the problem has been gone for the clear delay (10 minutes).
//
// A source that could not be checked (Agent unreachable, a reading that
// failed) neither opens nor closes anything: a known alert is kept while it
// is unknown whether it still holds (D-024).
//
// Every alert is shown in the panel (Monitoring, and a line on the
// Dashboard). Email goes only to administrators whose account address is
// confirmed (the account's own confirmation link, My account), through the
// panel's existing mail path (Agent → the server's own Postfix); nothing is
// emailed before an address is confirmed. One email per minute's new alerts,
// at most four an hour, and the same problem again within six hours is not
// emailed twice. A hand-over that fails is recorded on the alert and said in
// the panel; it is never tried again in a loop.
//
// Alerts are optional panel automation (D-022): with the panel stopped there
// are none, so the Monitoring page recommends an outside uptime checker.
//
// Uyarılar: "Şu anki sorunlar"daki kalıcı bir kalem. Gecikme (varsayılan 10
// dakika) geçmeden kalkan sorun uyarı olmaz; sorun sürdükçe tek satır kalır;
// sorun 10 dakika yok olunca kapanır. Kontrol edilemeyen kaynak hiçbir uyarıyı
// açmaz ya da kapatmaz. E-posta yalnız adresi onaylı yöneticilere, panelin
// mevcut posta yoluyla gider; başarısız teslim uyarıda yazılır ve döngüyle
// yeniden denenmez. Panel dururken uyarı yoktur.

const (
	alertDelayDefault   = 10
	alertClearDelay     = 10 * time.Minute
	alertRepeatQuiet    = 6 * time.Hour
	alertMailsPerHour   = 4
	alertDiskClearLevel = problemDiskWarning - 5

	settingAlertDelay      = "monitoring.alerts.delay_minutes"
	settingAlertEmail      = "monitoring.alerts.email"
	settingAlertSiteChecks = "monitoring.sites.check"
)

var alertDelayChoices = []int{5, 10, 15, 30, 60}

type alertSettings struct {
	DelayMinutes int  `json:"delay_minutes"`
	Email        bool `json:"email"`
	SiteChecks   bool `json:"site_checks"`
}

func (p *Panel) alertSettings(ctx context.Context) alertSettings {
	settings := alertSettings{DelayMinutes: alertDelayDefault, Email: true, SiteChecks: true}
	if p == nil || p.db == nil {
		return settings
	}
	if n, err := strconv.Atoi(p.setting(ctx, settingAlertDelay)); err == nil && validAlertDelay(n) {
		settings.DelayMinutes = n
	}
	settings.Email = p.setting(ctx, settingAlertEmail) != "off"
	settings.SiteChecks = p.setting(ctx, settingAlertSiteChecks) != "off"
	return settings
}

func validAlertDelay(n int) bool {
	for _, choice := range alertDelayChoices {
		if n == choice {
			return true
		}
	}
	return false
}

// problemSource is the source whose reading decides a kind.
var problemSource = map[string]string{
	"service_stopped":            "services",
	"service_restarting":         "services",
	"disk_full":                  "disk",
	"cpu_sustained":              "metrics",
	"memory_sustained":           "metrics",
	"certificate_expired":        "certificates",
	"certificate_renewal_failed": "certificates",
	"certificate_expiring":       "certificates",
	"site_down":                  "sites",
	"site_certificate":           "sites",
	"mail_queue_deferred":        "mail_queue",
	"security_updates":           "security_updates",
	"name_overlap":               "names",
	"name_server_reserved":       "names",
	"node_port_reserved":         "names",
}

type activeAlert struct {
	id         int64
	key, kind  string
	state      string
	firstSeen  time.Time
	clearSince sql.NullString
}

func parseAlertTime(raw string) (time.Time, bool) {
	at, err := time.Parse(time.RFC3339, raw)
	return at, err == nil
}

// evaluateMonitoringAlerts is the once-a-minute step.
func (p *Panel) evaluateMonitoringAlerts(ctx context.Context, now time.Time) {
	report := p.collectMonitoringProblemsFor(ctx, now, true)
	settings := p.alertSettings(ctx)
	diskPercent := func() (float64, bool) {
		used, total := monitoringReadDisk("/")
		if total == 0 {
			return 0, false
		}
		return float64(used) / float64(total) * 100, true
	}
	if err := applyAlertProblems(ctx, p.db.GetDB(), report, settings, now, diskPercent); err != nil {
		log.Printf("monitoring: alerts could not be updated: %v", err)
		return
	}
	p.sendPendingAlertMail(ctx, now)
}

// applyAlertProblems moves every alert one step with the current list.
func applyAlertProblems(ctx context.Context, db *sql.DB, report monitoringProblemsReport, settings alertSettings,
	now time.Time, diskPercent func() (float64, bool)) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	at := metricsTime(now)
	delay := time.Duration(settings.DelayMinutes) * time.Minute

	active := map[string]activeAlert{}
	rows, err := tx.QueryContext(ctx, `SELECT id, key, kind, state, first_seen, clear_since FROM monitoring_alerts WHERE state IN ('pending', 'open')`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var a activeAlert
		var first string
		if err := rows.Scan(&a.id, &a.key, &a.kind, &a.state, &first, &a.clearSince); err != nil {
			rows.Close()
			return err
		}
		a.firstSeen, _ = parseAlertTime(first)
		active[a.key] = a
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	unknown := map[string]bool{}
	for _, u := range report.Unknown {
		unknown[u.Source] = true
	}

	open := func(id int64, key string) error {
		mail := "none"
		if settings.Email {
			mail = "pending"
			var recent int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM monitoring_alerts WHERE key = ? AND mail_state = 'sent' AND opened_at >= ?`,
				key, metricsTime(now.Add(-alertRepeatQuiet))).Scan(&recent); err != nil {
				return err
			}
			if recent > 0 {
				mail = "suppressed"
			}
		}
		detail := any(nil)
		if mail == "suppressed" {
			detail = "repeat"
		}
		_, err := tx.ExecContext(ctx, `UPDATE monitoring_alerts SET state = 'open', opened_at = ?, mail_state = ?, mail_detail = ? WHERE id = ?`,
			at, mail, detail, id)
		return err
	}

	present := map[string]bool{}
	for _, problem := range report.Problems {
		if present[problem.Key] {
			continue
		}
		present[problem.Key] = true
		first := now
		if since, ok := parseAlertTime(problem.Since); ok && since.Before(now) {
			first = since
		}
		existing, ok := active[problem.Key]
		if !ok {
			result, err := tx.ExecContext(ctx, `
				INSERT INTO monitoring_alerts (key, kind, severity, subject, name, value, state, first_seen, last_seen)
				VALUES (?, ?, ?, ?, ?, ?, 'pending', ?, ?)`,
				problem.Key, problem.Kind, problem.Severity, problem.Subject, problem.Name, problem.Value, metricsTime(first), at)
			if err != nil {
				return err
			}
			id, err := result.LastInsertId()
			if err != nil {
				return err
			}
			existing = activeAlert{id: id, key: problem.Key, kind: problem.Kind, state: "pending", firstSeen: first}
		} else if _, err := tx.ExecContext(ctx, `
			UPDATE monitoring_alerts SET severity = ?, name = ?, value = ?, last_seen = ?, clear_since = NULL WHERE id = ?`,
			problem.Severity, problem.Name, problem.Value, at, existing.id); err != nil {
			return err
		}
		if existing.state == "pending" && now.Sub(existing.firstSeen) >= delay {
			if err := open(existing.id, existing.key); err != nil {
				return err
			}
		}
	}

	for key, alert := range active {
		if present[key] || unknown[problemSource[alert.kind]] {
			// Still there, or not known whether it is: kept as it is.
			continue
		}
		if alert.state == "pending" {
			if _, err := tx.ExecContext(ctx, `DELETE FROM monitoring_alerts WHERE id = ?`, alert.id); err != nil {
				return err
			}
			continue
		}
		if alert.kind == "disk_full" {
			if percent, ok := diskPercent(); ok && percent >= alertDiskClearLevel {
				// Value hysteresis: 84 % after 86 % is the same problem.
				if _, err := tx.ExecContext(ctx, `UPDATE monitoring_alerts SET value = ?, last_seen = ?, clear_since = NULL WHERE id = ?`,
					round1(percent), at, alert.id); err != nil {
					return err
				}
				continue
			}
		}
		if !alert.clearSince.Valid {
			if _, err := tx.ExecContext(ctx, `UPDATE monitoring_alerts SET clear_since = ? WHERE id = ?`, at, alert.id); err != nil {
				return err
			}
			continue
		}
		if since, ok := parseAlertTime(alert.clearSince.String); ok && now.Sub(since) >= alertClearDelay {
			if _, err := tx.ExecContext(ctx, `UPDATE monitoring_alerts SET state = 'resolved', resolved_at = ? WHERE id = ?`, at, alert.id); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// backgroundSecurityUpdates is the alert check's view of the waiting updates:
// the cached reading, asked again from the Agent at most once an hour.
var (
	backgroundOSUpdatesMu   sync.Mutex
	backgroundOSUpdatesLast time.Time
)

var backgroundSecurityUpdates = func(p *Panel, now time.Time) osUpdatesSummaryView {
	backgroundOSUpdatesMu.Lock()
	due := backgroundOSUpdatesLast.IsZero() || now.Sub(backgroundOSUpdatesLast) >= time.Hour
	if due {
		backgroundOSUpdatesLast = now
	}
	backgroundOSUpdatesMu.Unlock()
	if due {
		return monitoringSecurityUpdates(p)
	}
	return osUpdatesSummary.peek()
}

// --- Email -------------------------------------------------------------------

// alertMailLog bounds the emails of the last hour in this process.
var alertMailLog struct {
	mu   sync.Mutex
	sent []time.Time
}

func alertMailAllowed(now time.Time) bool {
	alertMailLog.mu.Lock()
	defer alertMailLog.mu.Unlock()
	kept := alertMailLog.sent[:0]
	for _, at := range alertMailLog.sent {
		if now.Sub(at) < time.Hour {
			kept = append(kept, at)
		}
	}
	alertMailLog.sent = kept
	return len(kept) < alertMailsPerHour
}

func noteAlertMail(now time.Time) {
	alertMailLog.mu.Lock()
	alertMailLog.sent = append(alertMailLog.sent, now)
	alertMailLog.mu.Unlock()
}

// confirmedAdministratorAddresses: active administrators whose address is
// confirmed. Nobody else is emailed.
func confirmedAdministratorAddresses(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT trim(email) FROM users
		WHERE role = 'admin' AND COALESCE(NULLIF(account_type, ''), 'account') = 'account'
		  AND COALESCE(NULLIF(status, ''), 'active') = 'active'
		  AND email_verified_at IS NOT NULL AND trim(COALESCE(email, '')) <> ''
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var addresses []string
	seen := map[string]bool{}
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, err
		}
		canonical, err := mutationpayload.CanonicalContactEmail(email)
		if err != nil || seen[strings.ToLower(canonical)] {
			continue
		}
		seen[strings.ToLower(canonical)] = true
		addresses = append(addresses, canonical)
	}
	return addresses, rows.Err()
}

// alertMailRecipients (alpha.128, G18): the confirmed administrators who
// want email (an alert always needs the administrator's action, so only
// "nothing" leaves one out), each one's extra address, and the
// administrator's extra address for monitoring alerts; without repeats.
// confirmed counts the confirmed administrators before their choice.
func (p *Panel) alertMailRecipients(ctx context.Context, db *sql.DB) ([]string, int, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, trim(email) FROM users
		WHERE role = 'admin' AND COALESCE(NULLIF(account_type, ''), 'account') = 'account'
		  AND COALESCE(NULLIF(status, ''), 'active') = 'active'
		  AND email_verified_at IS NOT NULL AND trim(COALESCE(email, '')) <> ''
		ORDER BY id`)
	if err != nil {
		return nil, 0, err
	}
	type admin struct {
		id    int
		email string
	}
	var admins []admin
	for rows.Next() {
		var a admin
		if err := rows.Scan(&a.id, &a.email); err != nil {
			rows.Close()
			return nil, 0, err
		}
		admins = append(admins, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var addresses []string
	seen := map[string]bool{}
	add := func(raw string) {
		canonical, err := mutationpayload.CanonicalContactEmail(raw)
		if err != nil || seen[strings.ToLower(canonical)] {
			return
		}
		seen[strings.ToLower(canonical)] = true
		addresses = append(addresses, canonical)
	}
	confirmed := 0
	for _, a := range admins {
		if _, err := mutationpayload.CanonicalContactEmail(a.email); err != nil {
			continue
		}
		confirmed++
		pref := notificationPreferenceFor(ctx, db, a.id)
		if !pref.mailWanted(true) {
			continue
		}
		add(a.email)
		if pref.ExtraEmail != "" {
			add(pref.ExtraEmail)
		}
	}
	if extra := p.adminGroupExtraEmail(ctx, notifyGroupMonitoring); extra != "" {
		add(extra)
	}
	return addresses, confirmed, nil
}

type pendingAlertMail struct {
	id       int64
	kind     string
	severity string
	subject  string
	name     string
	value    float64
}

// sendPendingAlertMail hands this minute's new alerts over as one email.
func (p *Panel) sendPendingAlertMail(ctx context.Context, now time.Time) {
	db := p.db.GetDB()
	rows, err := db.QueryContext(ctx, `
		SELECT id, kind, severity, subject, name, value FROM monitoring_alerts
		WHERE mail_state = 'pending' ORDER BY severity = 'critical' DESC, id LIMIT 50`)
	if err != nil {
		log.Printf("monitoring: pending alert mail could not be read: %v", err)
		return
	}
	var pending []pendingAlertMail
	for rows.Next() {
		var item pendingAlertMail
		if err := rows.Scan(&item.id, &item.kind, &item.severity, &item.subject, &item.name, &item.value); err != nil {
			rows.Close()
			return
		}
		pending = append(pending, item)
	}
	rows.Close()
	if len(pending) == 0 {
		return
	}
	mark := func(state string, detail string) {
		ids := make([]any, 0, len(pending)+2)
		ids = append(ids, state, nullIfEmpty(detail))
		for _, item := range pending {
			ids = append(ids, item.id)
		}
		if _, err := db.ExecContext(ctx, `UPDATE monitoring_alerts SET mail_state = ?, mail_detail = ? WHERE id IN (?`+
			strings.Repeat(`, ?`, len(pending)-1)+`)`, ids...); err != nil {
			log.Printf("monitoring: alert mail state could not be recorded: %v", err)
		}
	}
	recipients, confirmed, err := p.alertMailRecipients(ctx, db)
	if err != nil {
		return
	}
	if len(recipients) == 0 {
		if confirmed > 0 {
			// alpha.128 (G18): every confirmed administrator chose not to
			// receive email; the alert is still in the panel.
			mark("suppressed", "not_wanted")
			return
		}
		mark("not_confirmed", "")
		return
	}
	setup := p.accountMailSetup(ctx)
	if !setup.Available {
		mark("not_available", setup.Reason)
		return
	}
	if !alertMailAllowed(now) {
		mark("suppressed", "hourly_cap")
		return
	}
	subject, body := alertMailText(pending, setup.host, setup.baseURL)
	sent, failure := 0, ""
	for _, to := range recipients {
		if err := p.sendAccountMail(ctx, setup, to, accountMailContent{kind: accountMailAlert, subject: subject, body: body}); err != nil {
			failure = boundedAgentDiagnostic(err.Error())
			continue
		}
		sent++
	}
	noteAlertMail(now)
	if sent == 0 {
		mark("failed", failure)
		return
	}
	mark("sent", "")
}

// alertLine is one alert in plain English and Turkish: what is wrong and the
// one thing to do.
func alertLine(item pendingAlertMail) (string, string) {
	name, subject := item.name, item.subject
	if name == "" {
		name = subject
	}
	n := strconv.FormatFloat(item.value, 'f', -1, 64)
	switch item.kind {
	case "service_stopped":
		return fmt.Sprintf("The service %s is not running. Open Monitoring and press Start; if it stops again, its log on the component page says why.", name),
			fmt.Sprintf("%s hizmeti çalışmıyor. İzleme'yi açıp Başlat'a basın; yine durursa bileşen sayfasındaki günlük nedenini söyler.", name)
	case "service_restarting":
		return fmt.Sprintf("The service %s was restarted %s times in the last hour: it keeps failing and coming back. Read its log on the component page.", name, n),
			fmt.Sprintf("%s hizmeti son bir saatte %s kez yeniden başlatıldı: durup yeniden açılıyor. Bileşen sayfasındaki günlüğünü okuyun.", name, n)
	case "site_down":
		return fmt.Sprintf("The website %s does not answer from this server. Check that the web server and PHP are running (Monitoring), then open the site.", subject),
			fmt.Sprintf("%s web sitesi bu sunucudan yanıt vermiyor. Web sunucusu ve PHP'nin çalıştığını İzleme'de denetleyin, sonra siteyi açın.", subject)
	case "site_certificate":
		if item.name == "invalid" {
			return fmt.Sprintf("The certificate the website %s serves is not valid for its name or has expired. Open the domain's SSL tab and issue or renew it.", subject),
				fmt.Sprintf("%s web sitesinin sunduğu sertifika bu ad için geçerli değil ya da süresi dolmuş. Alan adının SSL sekmesinden yeniden alın ya da yenileyin.", subject)
		}
		return fmt.Sprintf("The certificate the website %s serves expires in %s days. Open the domain's SSL tab and renew it.", subject, n),
			fmt.Sprintf("%s web sitesinin sunduğu sertifikanın süresi %s gün içinde doluyor. Alan adının SSL sekmesinden yenileyin.", subject, n)
	case "disk_full":
		return fmt.Sprintf("The disk is %s%% full. Remove old backups, logs or files that are no longer needed; Maintenance → What deletions left shows what deleted domains and mailboxes left, which can be deleted now.", n),
			fmt.Sprintf("Disk %%%s dolu. Gerekmeyen eski yedekleri, günlükleri ya da dosyaları silin; Bakım → Silmelerden kalanlar, silinen alan adlarından ve posta kutularından kalanları gösterir, bunlar şimdi silinebilir.", n)
	case "certificate_expired":
		return fmt.Sprintf("The certificate of %s has expired; visitors see a warning. Open the domain's SSL tab and renew it.", subject),
			fmt.Sprintf("%s sertifikasının süresi doldu; ziyaretçiler uyarı görüyor. Alan adının SSL sekmesinden yenileyin.", subject)
	case "certificate_renewal_failed":
		return fmt.Sprintf("The certificate of %s could not be renewed. Open the domain's SSL tab: it says why and how to renew it.", subject),
			fmt.Sprintf("%s sertifikası yenilenemedi. Alan adının SSL sekmesi nedenini ve nasıl yenileneceğini söyler.", subject)
	case "certificate_expiring":
		return fmt.Sprintf("The certificate of %s expires in %s days. Open the domain's SSL tab and renew it.", subject, n),
			fmt.Sprintf("%s sertifikasının süresi %s gün içinde doluyor. Alan adının SSL sekmesinden yenileyin.", subject, n)
	case "memory_sustained":
		return fmt.Sprintf("Memory use has been at %s%% or more for over 10 minutes. Monitoring shows the busiest processes at that time.", n),
			fmt.Sprintf("Bellek kullanımı 10 dakikadan uzun süredir %%%s ya da üstünde. İzleme o andaki en yoğun işlemleri gösterir.", n)
	case "cpu_sustained":
		return fmt.Sprintf("The processor has been at %s%% or more for over 10 minutes. Monitoring shows the busiest processes at that time.", n),
			fmt.Sprintf("İşlemci 10 dakikadan uzun süredir %%%s ya da üstünde. İzleme o andaki en yoğun işlemleri gösterir.", n)
	case "mail_queue_deferred":
		return fmt.Sprintf("%s messages are waiting in the mail queue. Open the mail queue to see why they are deferred.", n),
			fmt.Sprintf("Posta kuyruğunda bekleyen %s ileti var. Neden ertelendiklerini posta kuyruğunda görün.", n)
	case "security_updates":
		return fmt.Sprintf("%s security updates are waiting. Install them under System updates.", n),
			fmt.Sprintf("%s güvenlik güncellemesi bekliyor. Sistem güncellemeleri'nden kurun.", n)
	case "name_overlap":
		return fmt.Sprintf("The domain %s meets %s, a name of another account: remove one of them, or transfer the subscription that holds %s to the account that holds %s. Open the domain in Domains.", subject, name, subject, name),
			fmt.Sprintf("%s alan adı, başka bir hesabın adı olan %s ile çakışıyor: birini kaldırın ya da %s adını tutan aboneliği %s adını tutan hesaba devredin. Alan adını Alan adları'nda açın.", subject, name, subject, name)
	case "name_server_reserved":
		return fmt.Sprintf("The domain %s meets %s, one of this server's own names, and belongs to a customer or reseller: only an administrator's own subscription holds it. Unless that account truly may, remove it. Open the domain in Domains.", subject, name),
			fmt.Sprintf("%s alan adı, bu sunucunun kendi adlarından %s ile çakışıyor ve bir müşteriye ya da bayiye ait: bu adı yalnız bir yöneticinin kendi aboneliği tutar. O hesabın tutması gerçekten gerekmiyorsa kaldırın. Alan adını Alan adları'nda açın.", subject, name)
	case "node_port_reserved":
		return fmt.Sprintf("The Node.js site %s sends its visitors to port %s, which a service of this server uses. Open the domain and save its Node.js settings again: the site gets a free port, and its program must listen on that port.", subject, n),
			fmt.Sprintf("%s Node.js sitesi ziyaretçilerini bu sunucunun bir hizmetinin kullandığı %s numaralı porta gönderiyor. Alan adını açıp Node.js ayarlarını yeniden kaydedin: site boş bir port alır; programı o portu dinlemelidir.", subject, n)
	// S4 R1 (side_doors.go).
	case "tool_account_failed":
		return fmt.Sprintf("%s still runs under the web server's account, which can read every site. Open it under Components and choose its PHP version again to move it to its own account.", name),
			fmt.Sprintf("%s hâlâ her siteyi okuyabilen web sunucusu hesabıyla çalışıyor. Bileşenler'de açıp PHP sürümünü yeniden seçerek kendi hesabına taşıyın.", name)
	case "tool_account_owner_config":
		return fmt.Sprintf("%s still runs under the web server's account because %s was changed on the server; CelikPanel keeps it.", name, subject),
			fmt.Sprintf("%s, %s sunucuda değiştirildiği için hâlâ web sunucusu hesabıyla çalışıyor; CelikPanel dosyayı korur.", name, subject)
	case "stock_pool_in_use", "stock_pool_owner_changed":
		return fmt.Sprintf("The distribution's PHP %s pool, which runs as the web server's account, is kept on: Monitoring says why.", name),
			fmt.Sprintf("Web sunucusu hesabıyla çalışan dağıtımın PHP %s havuzu açık bırakıldı; nedeni İzleme'de.", name)
	// S4 R1 (side_doors_jobs.go).
	case "site_crontab_outside":
		return fmt.Sprintf("%s scheduled task lines were added outside CelikPanel (%s); the sites' task limits were not checked for them. Each site's Scheduled tasks list marks them.", n, subject),
			fmt.Sprintf("CelikPanel dışında %s zamanlanmış görev satırı eklendi (%s); sitelerin görev sınırları onlar için denetlenmedi. Her sitenin Zamanlanmış görevler listesi onları işaretler.", n, subject)
	case "site_at_jobs":
		return fmt.Sprintf("%s at jobs of website accounts, made outside CelikPanel, are waiting to run: %s. Monitoring says how to remove them.", n, subject),
			fmt.Sprintf("Web sitesi hesaplarının CelikPanel dışında kurulmuş %s at işi çalışmayı bekliyor: %s. Nasıl kaldırılacağı İzleme'de.", n, subject)
	case "at_allow_lists_sites":
		return fmt.Sprintf("/etc/at.allow on this server lets website accounts use at: %s.", subject),
			fmt.Sprintf("Bu sunucudaki /etc/at.allow web sitesi hesaplarının at kullanmasına izin veriyor: %s.", subject)
	case "at_directory_accounts":
		return "Website accounts can still use at on this server: its accounts come from a directory service, so CelikPanel did not write /etc/at.allow.",
			"Bu sunucuda web sitesi hesapları hâlâ at kullanabiliyor: hesaplar bir dizin hizmetinden geldiği için CelikPanel /etc/at.allow dosyasını yazmadı."
	// S4 R1 fix round (side_doors.go, side_doors_jobs.go).
	case "tool_down":
		return fmt.Sprintf("%s does not answer on PHP %s. Open it under Components and choose its PHP version again; Monitoring says why.", name, subject),
			fmt.Sprintf("%s PHP %s üzerinde yanıt vermiyor. Bileşenler'de açıp PHP sürümünü yeniden seçin; nedeni İzleme'de.", name, subject)
	case "stock_pool_not_put_back":
		return fmt.Sprintf("The distribution's PHP %s pool could not be put back after turning it off failed. Monitoring says how to put it back.", name),
			fmt.Sprintf("Dağıtımın PHP %s havuzu kapatılamadı ve geri de konamadı. Nasıl geri konacağı İzleme'de.", name)
	case "stock_pool_unknown":
		// Second fix round (B10): it may not be readable at all; "still on"
		// is not said.
		return fmt.Sprintf("What the distribution's PHP %s pool is could not be read, so CelikPanel changed nothing: Monitoring says why.", name),
			fmt.Sprintf("Dağıtımın PHP %s havuzunun durumu okunamadı; CelikPanel hiçbir şeyi değiştirmedi. Nedeni İzleme'de.", name)
	case "stock_pool_failed":
		return fmt.Sprintf("The distribution's PHP %s pool, which runs as the web server's account, is still on: Monitoring says why.", name),
			fmt.Sprintf("Web sunucusu hesabıyla çalışan dağıtımın PHP %s havuzu hâlâ açık; nedeni İzleme'de.", name)
	case "stock_pool_owner_restored":
		return fmt.Sprintf("The distribution's PHP %s pool was put back on the server and runs as the web server's account. Monitoring offers to turn it off again.", name),
			fmt.Sprintf("Dağıtımın PHP %s havuzu sunucuda geri açıldı ve web sunucusu hesabıyla çalışıyor. İzleme yeniden kapatmayı önerir.", name)
	case "at_owner_removed":
		return "/etc/at.allow was deleted on this server, so website accounts can use at again. Monitoring offers to close it again.",
			"Bu sunucuda /etc/at.allow silindi; web sitesi hesapları yeniden at kullanabiliyor. İzleme yeniden kapatmayı önerir."
	case "site_crontab_unreadable":
		return fmt.Sprintf("The crontab of %s could not be read, so lines added to it outside CelikPanel cannot be seen. Monitoring says why.", subject),
			fmt.Sprintf("%s crontab'ı okunamadı; CelikPanel dışında eklenen satırları görülemiyor. Nedeni İzleme'de.", subject)
	case "at_not_closed":
		return "Website accounts can still use at on this server: CelikPanel could not write /etc/at.allow. It tries again at the panel's next start.",
			"Bu sunucuda web sitesi hesapları hâlâ at kullanabiliyor: CelikPanel /etc/at.allow dosyasını yazamadı. Panelin bir sonraki başlangıcında yeniden dener."
	case "postgresql_databases_open":
		return fmt.Sprintf("%s PostgreSQL databases made before alpha.141 are still open to this server's other accounts (%s): every account may connect to them, or make temporary tables where it may already connect. Open the PostgreSQL page, read what was found and close each one; it can be undone.", n, name),
			fmt.Sprintf("alpha.141'den önce açılan %s PostgreSQL veritabanı hâlâ bu sunucunun diğer hesaplarına açık (%s): her hesap bunlara bağlanabilir ya da zaten bağlanabildiği yerde geçici tablo açabilir. PostgreSQL sayfasını açın, bulunanı okuyun ve her birini kapatın; geri alınabilir.", n, name)
	case "postgresql_databases_unreadable":
		// 22 Nov 2026 (B5): item.name names each database with its reason.
		return fmt.Sprintf("Whether other accounts can enter these PostgreSQL databases is not known: they could not be read at the last hourly checks (%s). Open the PostgreSQL page, correct what the reason names, then press Read again.", name),
			fmt.Sprintf("Bu PostgreSQL veritabanlarına başka hesapların girip giremediği bilinmiyor: son saatlik denetimlerde okunamadılar (%s). PostgreSQL sayfasını açın, nedenin söylediğini düzeltin, sonra Yeniden oku'ya basın.", name)
	case "mail_signing":
		if item.name == "port25_variable_own" {
			// 22 Nov 2026 (B4): nothing is signed here; the owner's filters
			// do not reach port 25.
			return "Mail that arrives on port 25 passes CelikPanel's earlier filter list, not the filters added to main.cf since. Monitoring says what the server owner does.",
				"25. kapıya gelen posta, main.cf'e sonradan eklenen filtrelerden değil, CelikPanel'in önceki filtre listesinden geçiyor. İzleme sunucu sahibinin ne yapacağını söyler."
		}
		return "This server may still sign mail for a hosted domain other than the sender's own. Monitoring says which setting and what the server owner does.",
			"Bu sunucu postayı hâlâ göndericininkinden başka bir alan adı adına imzalayabilir. İzleme hangi ayar olduğunu ve sunucu sahibinin ne yapacağını söyler."
	case "mail_autoresponder_foreign":
		// 22 Nov 2026 (B3): item.name is the mailbox.
		return fmt.Sprintf("The out-of-office reply of %s still answers from an address that is no longer this mailbox's own, and CelikPanel could not change it on the server. Open Monitoring: it says the step; CelikPanel tries again every hour.", item.name),
			fmt.Sprintf("%s kutusunun otomatik yanıtı hâlâ artık bu kutunun olmayan bir adresten yanıt veriyor ve CelikPanel bunu sunucuda değiştiremedi. İzleme'yi açın: yapılacak adımı söyler; CelikPanel her saat yeniden dener.", item.name)
	// S4 R2a (site_limits_view.go).
	case "site_tasks_repeated":
		return fmt.Sprintf("The programs of %s keep reaching their process limit (%s times in a day). The site's page says what its plan allows.", subject, n),
			fmt.Sprintf("%s sitesinin programları işlem sınırına tekrar tekrar ulaşıyor (bir günde %s kez). Sitenin sayfası planın neye izin verdiğini söyler.", subject, n)
	case "site_memory_repeated":
		return fmt.Sprintf("The programs of %s keep using all their memory (%s times in a day). The site's page says what its plan allows.", subject, n),
			fmt.Sprintf("%s sitesinin programları belleğin tamamını tekrar tekrar kullanıyor (bir günde %s kez). Sitenin sayfası planın neye izin verdiğini söyler.", subject, n)
	case "site_programs_outside":
		return fmt.Sprintf("The programs of %s sites still run outside their limits (%s). \"Move into the limits\" on each site's Overview restarts them once inside.", n, subject),
			fmt.Sprintf("%s sitenin programları hâlâ sınırlarının dışında çalışıyor (%s). Her sitenin Genel Bakış sayfasındaki \"Sınırların içine taşı\" onları bir kez yeniden başlatıp içeri alır.", n, subject)
	case "site_limits_unsupported":
		return "This server cannot enforce the limits of sites' programs (missing: " + subject + "). Monitoring says what that means.",
			"Bu sunucu sitelerin programlarının sınırlarını uygulayamıyor (eksik: " + subject + "). Ne anlama geldiği İzleme'de."
	case "site_limits_unapplied":
		return fmt.Sprintf("A change of limits (%s) did not reach %s sites yet; they keep their earlier limits. CelikPanel tries again every hour. Monitoring says why.", subject, n),
			fmt.Sprintf("Bir sınır değişikliği (%s) henüz %s siteye ulaşmadı; o siteler önceki sınırlarını koruyor. CelikPanel her saat yeniden dener. Nedeni İzleme'de.", subject, n)
	case "site_move_kept":
		return fmt.Sprintf("A move of %s into its limits did not finish: a program may be stopped, and no site is moved until it is settled. The site's page names the program and has Try again.", subject),
			fmt.Sprintf("%s sitesinin sınırlarına taşınması bitmedi: bir program durmuş olabilir; bu çözülene kadar hiçbir site taşınmaz. Sitenin sayfası programı söyler ve Tekrar dene düğmesini sunar.", subject)
	case "site_memory_default":
		return "The server's memory changed: the default memory limit of a site is still the one set for the earlier memory. Monitoring offers the new default after a preview.",
			"Sunucunun belleği değişti: bir sitenin varsayılan bellek sınırı hâlâ önceki belleğe göre konan sınır. İzleme yeni varsayılanı önizlemeyle sunar."
	case "disk_limits_counted":
		return "Disk limits are counted, not enforced: the hosting file system has no project quotas. Monitoring says what that means.",
			"Disk sınırları sayılıyor ama dosya sistemi tarafından uygulanmıyor: barındırma dosya sisteminde proje kotası yok. Ne anlama geldiği İzleme'de."
	default:
		return "A monitored problem lasts. Open Monitoring for details.", "Bir izleme sorunu sürüyor. Ayrıntı için İzleme'yi açın."
	}
}

func alertMailText(items []pendingAlertMail, host, baseURL string) (string, string) {
	en, _ := alertLine(items[0])
	subject := "CelikPanel alert on " + host + " / uyarı: " + strings.TrimSuffix(en, ".")
	if len(subject) > 160 {
		subject = subject[:157] + "..."
	}
	if len(items) > 1 {
		subject = fmt.Sprintf("CelikPanel: %d alerts on %s / %d uyarı", len(items), host, len(items))
	}
	var b strings.Builder
	for _, item := range items {
		en, _ := alertLine(item)
		b.WriteString("- " + en + "\n")
	}
	b.WriteString("\nOpen Monitoring: " + baseURL + "/monitoring\n")
	b.WriteString("This email was sent by CelikPanel on " + host + " because the problem lasted longer than your alert delay. " +
		"While the panel is stopped no alerts are sent; an outside uptime checker covers that. " +
		"Change or turn off these emails under Monitoring → Alert settings.\n\n")
	for _, item := range items {
		_, tr := alertLine(item)
		b.WriteString("- " + tr + "\n")
	}
	b.WriteString("\nİzleme: " + baseURL + "/monitoring\n")
	b.WriteString("Bu e-postayı " + host + " üzerindeki CelikPanel, sorun uyarı gecikmenizden uzun sürdüğü için gönderdi. " +
		"Panel dururken uyarı gönderilmez; bunu dış bir çalışma süresi denetleyicisi karşılar. " +
		"Bu e-postaları İzleme → Uyarı ayarları'ndan değiştirin ya da kapatın.\n")
	return subject, b.String()
}

// --- Handlers ----------------------------------------------------------------

type alertView struct {
	ID         int64   `json:"id"`
	Key        string  `json:"key"`
	Kind       string  `json:"kind"`
	Severity   string  `json:"severity"`
	Subject    string  `json:"subject,omitempty"`
	Name       string  `json:"name,omitempty"`
	Value      float64 `json:"value"`
	State      string  `json:"state"`
	FirstSeen  string  `json:"first_seen"`
	OpenedAt   string  `json:"opened_at"`
	LastSeen   string  `json:"last_seen"`
	ResolvedAt string  `json:"resolved_at,omitempty"`
	MailState  string  `json:"mail_state"`
	MailDetail string  `json:"mail_detail,omitempty"`
	Seen       bool    `json:"seen"`
}

type alertEmailState struct {
	// Confirmed is how many administrators would receive an email.
	Confirmed int `json:"confirmed"`
	// The caller's own address.
	OwnSet      bool              `json:"own_set"`
	OwnVerified bool              `json:"own_verified"`
	Mail        accountMailStatus `json:"mail"`
}

type alertsReport struct {
	Settings alertSettings   `json:"settings"`
	Choices  []int           `json:"delay_choices"`
	Email    alertEmailState `json:"email"`
	Alerts   []alertView     `json:"alerts"`
	Unseen   int             `json:"unseen"`
}

func readAlerts(ctx context.Context, db *sql.DB) ([]alertView, int, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, key, kind, severity, subject, name, value, state, first_seen, opened_at, last_seen,
		       COALESCE(resolved_at, ''), mail_state, COALESCE(mail_detail, ''), seen_at IS NOT NULL
		FROM monitoring_alerts WHERE state IN ('open', 'resolved')
		ORDER BY state = 'resolved', opened_at DESC, id DESC LIMIT 50`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	alerts := []alertView{}
	unseen := 0
	for rows.Next() {
		var a alertView
		if err := rows.Scan(&a.ID, &a.Key, &a.Kind, &a.Severity, &a.Subject, &a.Name, &a.Value, &a.State, &a.FirstSeen,
			&a.OpenedAt, &a.LastSeen, &a.ResolvedAt, &a.MailState, &a.MailDetail, &a.Seen); err != nil {
			return nil, 0, err
		}
		if !a.Seen {
			unseen++
		}
		alerts = append(alerts, a)
	}
	return alerts, unseen, rows.Err()
}

func (p *Panel) handleMonitoringAlerts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	db := p.db.GetDB()
	alerts, unseen, err := readAlerts(r.Context(), db)
	if err != nil {
		writeServerError(w, err)
		return
	}
	recipients, err := confirmedAdministratorAddresses(r.Context(), db)
	if err != nil {
		writeServerError(w, err)
		return
	}
	email := alertEmailState{Confirmed: len(recipients), Mail: p.accountMailSetup(r.Context()).accountMailStatus}
	if caller := currentCaller(r); caller != nil && caller.ID > 0 {
		var address string
		var verified sql.NullString
		if err := db.QueryRowContext(r.Context(), `SELECT COALESCE(email, ''), email_verified_at FROM users WHERE id = ?`, caller.ID).
			Scan(&address, &verified); err == nil {
			email.OwnSet, email.OwnVerified = strings.TrimSpace(address) != "", verified.Valid
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(alertsReport{
		Settings: p.alertSettings(r.Context()), Choices: alertDelayChoices, Email: email, Alerts: alerts, Unseen: unseen,
	})
}

const (
	errCodeAlertSettingsInvalid = "monitoring_alert_settings_invalid"
	alertSettingsInvalidMessage = "These alert settings could not be read, so nothing was changed. Reload the page and choose again."
)

// handleMonitoringAlertSettings: PUT the three settings; the answer is the
// stored settings. Saving the same values again changes nothing.
func (p *Panel) handleMonitoringAlertSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPut)
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		DelayMinutes *int  `json:"delay_minutes"`
		Email        *bool `json:"email"`
		SiteChecks   *bool `json:"site_checks"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || req.DelayMinutes == nil || req.Email == nil || req.SiteChecks == nil ||
		!validAlertDelay(*req.DelayMinutes) {
		writeCodedError(w, http.StatusBadRequest, errCodeAlertSettingsInvalid, alertSettingsInvalidMessage, "")
		return
	}
	onOff := func(v bool) string {
		if v {
			return "on"
		}
		return "off"
	}
	for key, value := range map[string]string{
		settingAlertDelay:      strconv.Itoa(*req.DelayMinutes),
		settingAlertEmail:      onOff(*req.Email),
		settingAlertSiteChecks: onOff(*req.SiteChecks),
	} {
		if err := p.setSetting(r.Context(), key, value); err != nil {
			writeServerError(w, err)
			return
		}
	}
	if !*req.SiteChecks {
		// No checks, no stale results that would read as current.
		_, _ = p.db.GetDB().ExecContext(r.Context(), `DELETE FROM site_uptime`)
	}
	p.audit(r, "monitoring.alerts.settings", "settings", 0)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p.alertSettings(r.Context()))
}

// handleMonitoringAlertsSeen: POST marks every listed alert as seen (the
// list's own "Mark as seen"). Repeating it changes nothing.
func (p *Panel) handleMonitoringAlertsSeen(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if _, err := p.db.GetDB().ExecContext(r.Context(),
		`UPDATE monitoring_alerts SET seen_at = ? WHERE seen_at IS NULL AND state IN ('open', 'resolved')`, metricsTime(time.Now())); err != nil {
		writeServerError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"seen": true})
}
