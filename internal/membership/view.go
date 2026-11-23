package membership

// Port of portal-membership/view.php and terms.php (origin/main 2584a566e): the
// same Turkish and English texts and the same bytes for the same state. Every value
// that came from a member, the database or the request goes through htmlEscape
// (PHP's h()); texts are written as they stand in the PHP source. The line comments
// name view.php's lines; where PHP's "?>" swallows the newline after it, no newline
// is written here either.

import (
	"strings"
	"time"
)

type page struct {
	lang     string
	action   string
	member   *Member
	err      string
	notice   string
	issued   *Issued
	licenses []License
	now      int64
	csrf     string
	get      phpVars
	post     phpVars
	// store is set only when the store exists and a member is signed in (S2).
	store *storePage
}

func (p *page) t(tr, en string) string {
	if p.lang == "en" {
		return en
	}
	return tr
}

var messages = map[string][2]string{
	"invalid_email":             {"Geçerli bir e-posta adresi girin.", "Enter a valid email address."},
	"password_length":           {"Parolanız 12–128 bayt uzunluğunda olmalı.", "Use a password between 12 and 128 bytes."},
	"invalid_login":             {"E-posta veya parola doğru değil.", "The email or password is incorrect."},
	"verify_email":              {"Giriş yapmadan önce e-postanızı doğrulayın. Aşağıdan yeni doğrulama e-postası isteyebilirsiniz.", "Verify your email before signing in. You can request another email below."},
	"invalid_token":             {"Bağlantı kullanılmış veya süresi dolmuş. Yeni bir e-posta isteyin.", "This link has been used or has expired. Request another email."},
	"rate_limited":              {"Çok fazla deneme yapıldı. Lütfen daha sonra tekrar deneyin.", "Too many attempts. Please try again later."},
	"service_busy":              {"Hizmet şu anda yoğun. Lütfen biraz sonra tekrar deneyin.", "The service is busy. Please try again in a moment."},
	"session_changed":           {"Oturumunuz yenilendi. Sayfayı yenileyip tekrar deneyin.", "Your session changed. Reload this page and try again."},
	"invalid_request":           {"İstek tamamlanamadı. Sayfayı yenileyip tekrar deneyin.", "The request could not be completed. Reload and try again."},
	"authentication_required":   {"Devam etmek için giriş yapın.", "Sign in to continue."},
	"accept_terms":              {"Devam etmek için kullanım koşullarını kabul edin.", "Accept the terms to continue."},
	"mail_unavailable":          {"E-posta şu anda gönderilemiyor. Daha sonra yeni bir doğrulama veya parola yenileme e-postası isteyin.", "Email is temporarily unavailable. Request another verification or reset email later."},
	"key_not_saved":             {"Anahtar görüntülenemiyor. Lisanslarınızdan yeni anahtar oluşturabilirsiniz; bitiş tarihi değişmez.", "The key cannot be displayed. You can create a replacement from your licenses; the expiry date stays the same."},
	"invalid_license":           {"Anahtar bu lisansla eşleşmiyor. Oluştururken aldığınız anahtarı girin.", "This key does not match this license. Enter the key you received when it was created."},
	"server_released":           {"Sunucu eşlemesi kaldırıldı. Aynı anahtarı yeni sunucuda kullanabilirsiniz; bitiş tarihi değişmedi.", "The server binding was released. Use the same key on the new server; the expiry date is unchanged."},
	"license_revoked":           {"Bu lisansın ödemesi geri alındı; lisans sona erdi.", "This license's payment was reversed; the license has ended."},
	"charging_off":              {"Beta süresince satın alma yok; CelikPanel ücretsizdir. Ücretlendirme 1.0 sürümüyle başlar.", "There is nothing to buy during the beta; CelikPanel is free. Charging starts with version 1.0."},
	"charging_live_not_enabled": {"Gerçek ödemeler bu sürümde açık değil. Sizin bir şey yapmanız gerekmiyor.", "Real payments are not enabled in this release. You do not need to do anything."},
	"not_a_test_account":        {"Deneme kipinde yalnız deneme hesapları satın alabilir. Bu hesap bir deneme hesabı değil; hiçbir ücret alınmadı.", "In test mode only test accounts can buy. This account is not a test account; nothing was charged."},
	"order_not_found":           {"Sipariş bulunamadı.", "Order not found."},
	"payment_not_ready":         {"Ödeme şu anda alınamıyor; durumu biz düzeltiyoruz. Hiçbir ücret alınmadı; daha sonra tekrar deneyin.", "Payments cannot be taken right now; we are fixing it. Nothing was charged; please try again later."},
	"payment_unavailable":       {"Ödeme sağlayıcısına şu anda ulaşılamıyor. Hiçbir ücret alınmadı; birkaç dakika sonra tekrar deneyin.", "The payment provider cannot be reached right now. Nothing was charged; please try again in a few minutes."},
	"renewal_is_bought":         {"Bu lisans, üzerinde bir yıl satın alınarak yenilenir.", "This license is renewed by buying a year on it."},
	"too_many_awaiting":         {"İlk yılını bekleyen 3 lisansınız var. Yenisini oluşturmadan önce birinin yılını satın alın ya da birini kaldırın.", "You already have 3 licenses waiting for their first year. Buy a year for one of them, or remove one, before creating another."},
	"buy_after_end":             {"Lisansınızın bitişine çok az kaldı; yeni yıl bitişten sonra satın alınabilir.", "Your license ends very soon; the new year can be bought after it ends."},
	"test_time_remains":         {"Bu hesabın deneme ödemesiyle alınmış süresi devam ediyor.", "This account still has time bought with test payments."},
	"trial_started":             {"Ücretsiz 30 günlük denemeniz başladı. Panelin açık kalması için deneme bitmeden bir yıl satın alın.", "Your free 30-day trial has started. Buy a year before it ends to keep the panel open."},
	"license_awaiting_payment":  {"Yeni lisansınız, ilk yılı ödendiğinde kullanılabilir.", "Your new license can be used once its first year is paid."},
	"license_not_found":         {"Lisans bulunamadı.", "License not found."},
	"renewal_not_due":           {"Lisansınızı bitiş tarihinden önceki son 30 günde veya süresi dolduktan sonra yenileyebilirsiniz.", "Renew during the last 30 days of the license or after it expires."},
	"mail_sent":                 {"Bu adres için işlem yapılabiliyorsa e-posta gönderildi. Gelen kutunuzu ve spam klasörünü kontrol edin.", "If this address is eligible, an email has been sent. Check your inbox and spam folder."},
	"email_verified":            {"E-postanız doğrulandı. Şimdi giriş yapabilirsiniz.", "Your email is verified. You can now sign in."},
	"password_reset":            {"Parolanız değiştirildi. Yeni parolanızla giriş yapın.", "Your password has changed. Sign in with your new password."},
	"license_removed":           {"Lisans kaldırıldı. Anahtarı artık hiçbir sunucuda kullanılamaz.", "The license was removed. Its key no longer works on any server."},
	"renewed":                   {"Lisans bir yıl uzatıldı. Sunucu sonraki lisans kontrolünde yeni tarihi alır.", "The license was extended by one year. Your server receives the new date on its next license check."},
}

func (p *page) message(code string) string {
	m, ok := messages[code]
	if !ok {
		m = messages["invalid_request"]
	}
	if p.lang == "en" {
		return m[1]
	}
	return m[0]
}

func (p *page) titles() map[string]string {
	titles := map[string]string{
		"terms":    p.t("Kullanım koşulları ve veri açıklaması", "Terms and data notice"),
		"login":    p.t("Hesabınıza giriş yapın", "Sign in to your account"),
		"register": p.t("CelikPanel hesabınızı oluşturun", "Create your CelikPanel account"),
		"forgot":   p.t("Parolanızı yenileyin", "Reset your password"),
		"resend":   p.t("Doğrulama e-postası isteyin", "Request a verification email"),
		"verify":   p.t("E-postanızı doğrulayın", "Verify your email"),
		"reset":    p.t("Yeni parolanızı belirleyin", "Choose your new password"),
		"home":     p.t("Lisanslarınız", "Your licenses"),
		"release":  p.t("Sunucu / IP değiştirin", "Change server / IP"),
		"reveal":   p.t("Lisans anahtarınız", "Your license key"),
		"rotate":   p.t("Lisans anahtarını değiştirin", "Replace your license key"),
		"renew":    p.t("Lisansınızı uzatın", "Renew your license"),
		"remove":   p.t("Lisansı kaldırın", "Remove the license"),
	}
	if p.store != nil && p.store.order != nil {
		titles["order"] = p.t("Siparişiniz", "Your order")
	}
	if p.store != nil && p.store.receipt != nil {
		titles["receipt"] = p.t("Makbuz", "Receipt")
	}
	if p.store != nil && p.store.buy != nil {
		titles["buy"] = p.t("Bir yıl satın alın", "Buy a year")
		if p.store.quote.Kind == KindRenewal {
			titles["buy"] = p.t("Bir yıl yenileyin", "Renew for a year")
		}
	}
	return titles
}

func (p *page) formStart(b *strings.Builder, action string) {
	b.WriteString(`<form method="post" action="/account/?action=` + htmlEscape(action) + `&amp;lang=` + htmlEscape(p.lang) + `">`)
	b.WriteString(`<input type="hidden" name="csrf" value="` + htmlEscape(p.csrf) + `">`)
}

func (p *page) passwordField(b *strings.Builder, autocomplete string) {
	b.WriteString(`<label>` + p.t("Parola", "Password") + `<input type="password" name="password" required minlength="12" maxlength="128" autocomplete="` + htmlEscape(autocomplete) + `"></label>`)
}

// requestID is $_POST['id'] ?? $_GET['id'] (absent: the empty string).
func (p *page) requestID() (phpValue, bool) {
	if v, ok := p.post["id"]; ok {
		return v, true
	}
	if v, ok := p.get["id"]; ok {
		return v, true
	}
	return phpValue{}, false
}

// nullTruthy is PHP's truthiness of a nullable text column.
func nullTruthy(valid bool, s string) bool { return valid && phpTruthyString(s) }

// render returns the page; the caller has already refused an array action.
func (p *page) render() string {
	var b strings.Builder
	titles := p.titles()
	if _, ok := titles[p.action]; !ok { // view.php:41
		if p.member != nil {
			p.action = "home"
		} else {
			p.action = "login"
		}
	}
	action := p.action
	other := "en"
	if p.lang == "en" {
		other = "tr"
	}
	// view.php:44-49
	b.WriteString(`<!doctype html><html lang="` + htmlEscape(p.lang) + `"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">` + "\n")
	b.WriteString(`<title>` + htmlEscape(titles[action]) + ` · CelikPanel</title><link rel="icon" href="/assets/favicon-v2.svg"><link rel="stylesheet" href="/assets/site.css"><link rel="stylesheet" href="/account/member.css"><script src="/account/member.js" defer></script></head>` + "\n")
	b.WriteString(`<body class="member-page"><a class="member-skip" href="#main">` + p.t("İçeriğe geç", "Skip to content") + `</a>` + "\n")
	b.WriteString(`<header class="member-header"><a href="/" class="member-brand"><img src="/assets/favicon-v2.svg" alt="" width="28" height="28">CelikPanel</a><nav aria-label="` +
		p.t("Hesap menüsü", "Account navigation") + `"><a href="?action=` + htmlEscape(action) + `&amp;lang=` + other)
	if id, ok := p.requestID(); ok {
		b.WriteString(`&amp;id=` + htmlEscape(id.text()))
	}
	language := "English"
	if p.lang == "en" {
		language = "Türkçe"
	}
	b.WriteString(`" lang="` + other + `">` + language + `</a>`)
	if p.member != nil {
		p.formStart(&b, "logout")
		b.WriteString(`<button class="member-link" type="submit">` + p.t("Çıkış yap", "Sign out") + `</button></form>`)
	}
	b.WriteString(`</nav></header>` + "\n")
	wide := ""
	if action == "home" {
		wide = "member-wide"
	}
	b.WriteString(`<main id="main" class="member-main ` + wide + `">` + "\n")
	b.WriteString(`<h1>` + htmlEscape(titles[action]) + `</h1>` + "\n")
	// view.php:50-51 (the newline after each endif is swallowed)
	if phpTruthyString(p.err) {
		b.WriteString(`<p class="member-notice member-error" role="alert">` + htmlEscape(p.message(p.err)) + `</p>`)
	}
	if phpTruthyString(p.notice) {
		b.WriteString(`<p class="member-notice" role="status">` + htmlEscape(p.message(p.notice)) + `</p>`)
	}
	switch {
	case action == "terms":
		b.WriteString(p.terms())
	case action == "home" && p.member != nil:
		p.home(&b)
	case action == "order" && p.member != nil && p.store != nil && p.store.order != nil:
		p.orderPage(&b)
	case action == "buy" && p.member != nil && p.store != nil && p.store.buy != nil:
		p.buyPage(&b)
	case action == "receipt" && p.member != nil && p.store != nil && p.store.receipt != nil:
		p.receiptPage(&b)
	case (action == "release" || action == "renew" || action == "reveal" || action == "rotate" || action == "remove") && p.member != nil:
		p.confirm(&b, action)
	default:
		p.form(&b, action)
	}
	b.WriteString(`</main><footer class="member-footer">CelikPanel · <a href="/">` + p.t("Websitesine dön", "Back to website") + `</a></footer></body></html>` + "\n")
	return b.String()
}

// home is view.php:54-62.
func (p *page) home(b *strings.Builder) {
	b.WriteString(`<p class="member-intro">` + htmlEscape(p.member.Email) + ` · ` + p.t("Her lisans bir aktif sunucu içindir.", "Each license is for one active server.") + `</p>` + "\n")
	if p.issued != nil {
		b.WriteString(`<section class="member-key" aria-labelledby="new-key"><h2 id="new-key">` + p.t("Lisans anahtarınız", "Your license key") + `</h2><p>` +
			p.t("Kurulumdan sonra sunucunuzdaki panele yönetici olarak giriş yapın ve bu anahtarı Lisans ekranına yapıştırın. Anahtarı daha sonra hesabınızdan tekrar görüntüleyebilirsiniz.",
				"After installation, sign in to your server panel as administrator and paste this key on the License screen. You can view the key again from your account.") +
			`</p><code id="license-key">` + htmlEscape(p.issued.Key) + `</code><button type="button" data-copy="license-key">` + p.t("Anahtarı kopyala", "Copy key") +
			`</button><span id="copy-status" role="status"></span></section>`)
	}
	if p.store != nil && p.store.charged {
		p.chargedSummary(b)
	} else {
		b.WriteString(`<section class="member-summary"><div><h2>CelikPanel</h2><p>` + p.t("Şu an ücretsiz. Oluşturulduğu tarihten itibaren bir yıl geçerli.", "Currently free. Valid for one year from creation.") + `</p></div>`)
		p.formStart(b, "issue")
		b.WriteString(`<button type="submit">` + p.t("Ücretsiz lisans oluştur", "Create a free license") + `</button></form></section>` + "\n")
	}
	p.storeSummary(b)
	if len(p.licenses) == 0 {
		b.WriteString(`<p class="member-empty">` + p.t("Henüz lisansınız yok. İlk lisansınızı oluşturup sunucunuza CelikPanel kurabilirsiniz.", "You have no licenses yet. Create your first license to install CelikPanel on your server.") + `</p>`)
	}
	b.WriteString(`<div class="member-licenses">`)
	for _, l := range p.licenses {
		expires := l.Expires.Valid && l.Expires.Int64 != 0
		expired := expires && l.Expires.Int64 <= p.now
		hostname := p.t("Aktivasyon bekliyor", "Awaiting activation")
		if nullTruthy(l.Hostname.Valid, l.Hostname.String) {
			hostname = l.Hostname.String
		}
		state := p.t("Sunucuya bağlanmadı", "Not bound to a server")
		if expired {
			state = p.t("Süresi doldu", "Expired")
		} else if nullTruthy(l.ServerID.Valid, l.ServerID.String) {
			state = p.t("Aktif", "Active")
		}
		b.WriteString(`<article class="member-license"><div><h2>` + htmlEscape(hostname) + `</h2><p><span class="member-state">` + state + `</span> <code>…` + htmlEscape(l.KeySuffix) + `</code></p>`)
		if nullTruthy(l.ServerIP.Valid, l.ServerIP.String) {
			b.WriteString(`<p>` + p.t("Eşleşen IP", "Bound IP") + `: <code>` + htmlEscape(l.ServerIP.String) + `</code></p>`)
		}
		date := p.t("Tarih belirlenemedi", "Date unavailable")
		if expires {
			date = htmlEscape(time.Unix(l.Expires.Int64, 0).UTC().Format("2006-01-02 15:04"))
		}
		b.WriteString(`</div><dl><dt>` + p.t("Bitiş tarihi (UTC)", "Expires (UTC)") + `</dt><dd>` + date + `</dd></dl><div class="member-actions">`)
		ended := p.store != nil && p.store.revoked[l.ID] // a revoked licence offers removal only
		if ended {
			b.WriteString(`<a href="?action=remove&amp;id=` + htmlEscape(l.ID) + `">` + p.t("Lisansı kaldır", "Remove license") + `</a></div>`)
			p.licenceStore(b, l)
			b.WriteString(`</article>`)
			continue
		}
		if l.KeyAvailable != 0 {
			b.WriteString(`<a href="?action=reveal&amp;id=` + htmlEscape(l.ID) + `">` + p.t("Anahtarı göster", "Show key") + `</a>`)
		} else {
			b.WriteString(`<span>` + p.t("Bu eski lisansın anahtarı görüntülenemiyor.", "This older license key cannot be displayed.") + `</span>`)
		}
		if nullTruthy(l.ServerID.Valid, l.ServerID.String) {
			b.WriteString(`<a href="?action=release&amp;id=` + htmlEscape(l.ID) + `">` + p.t("Sunucu / IP değiştir", "Change server / IP") + `</a>`)
		}
		replace := p.t("Yeni anahtar oluştur", "Create replacement key")
		if l.KeyAvailable != 0 {
			replace = p.t("Anahtarı değiştir", "Replace key")
		}
		b.WriteString(`<a href="?action=rotate&amp;id=` + htmlEscape(l.ID) + `">` + replace + `</a>`)
		if expires && l.Expires.Int64 <= p.now+30*86400 && !(p.store != nil && p.store.charged) {
			b.WriteString(`<a href="?action=renew&amp;id=` + htmlEscape(l.ID) + `">` + p.t("Ücretsiz yenile", "Renew for free") + `</a>`)
		}
		b.WriteString(`<a href="?action=remove&amp;id=` + htmlEscape(l.ID) + `">` + p.t("Lisansı kaldır", "Remove license") + `</a></div>`)
		p.licenceStore(b, l)
		b.WriteString(`</article>`)
	}
	b.WriteString(`</div>` + "\n")
	p.payments(b)
	b.WriteString(`<section class="member-install"><h2>` + p.t("Sunucunuza kurun", "Install on your server") + `</h2><p>` +
		p.t("Desteklenen temiz bir Linux sunucusunda çalıştırın. Terminalde yönetici hesabınızı oluşturun; kurulum bitince panele giriş yapıp lisans anahtarınızı girin. Yönetici parolası bu websitesiyle paylaşılmaz.",
			"Run on a supported clean Linux server. Create your administrator account in the terminal, then sign in to the panel and enter your license key. The administrator password is not shared with this website.") +
		`</p><pre><code id="install-command">timedatectl set-ntp true` + "\n" +
		`curl -fL --proto '=https' -o celikpanel-install.sh https://celikpanel.net/get.sh &amp;&amp;` + "\n" +
		`sh celikpanel-install.sh</code></pre><button type="button" data-copy="install-command">` + p.t("Komutu kopyala", "Copy command") + `</button><p>` +
		p.t("Yarım kalan kurulumda aynı komutu yeniden çalıştırın. Lisans süresi yeniden başlamaz.", "If installation is interrupted, run the same command again. The license term does not restart.") +
		`</p></section>` + "\n")
}

// confirm is view.php:64-84: the password step of release, renew, reveal, rotate
// and remove.
func (p *page) confirm(b *strings.Builder, action string) {
	id, hasID := p.requestID()
	var target *License
	for i := range p.licenses {
		if hasID && !id.array && p.licenses[i].ID == id.str {
			target = &p.licenses[i]
		}
	}
	bound := target != nil && nullTruthy(target.ServerID.Valid, target.ServerID.String)
	where := ""
	if bound {
		name := p.t("bir sunucu", "a server")
		if nullTruthy(target.Hostname.Valid, target.Hostname.String) {
			name = target.Hostname.String
		}
		where = htmlEscape(name)
		if nullTruthy(target.ServerIP.Valid, target.ServerIP.String) {
			where += " (" + htmlEscape(target.ServerIP.String) + ")"
		}
	}
	var description string
	switch action {
	case "remove":
		if bound {
			description = p.t("Bu lisansı "+where+" kullanıyor. Kaldırırsanız oradaki panel bunu bir sonraki lisans kontrolünde (panel kullanılıyorsa genellikle bir saat içinde) öğrenir ve yönetim ekranını kapatır; siteler, e-posta, DNS ve zamanlanmış görevler çalışmaya devam eder. Lisans hizmetine ulaşamayan bir sunucu, son başarılı kontrolünden sonra en çok 7 gün açık kalabilir. Anahtar geçersiz olur; bu işlem geri alınamaz. O sunucuya daha sonra yeni bir lisans girebilirsiniz.",
				"This license is used by "+where+". If you remove it, the panel there learns of it at its next license check (usually within an hour while the panel is in use) and closes its management screens; sites, mail, DNS and scheduled tasks keep running. A server that cannot reach the license service can stay open for up to 7 days after its last successful check. The key stops working; this cannot be undone. You can enter a new license on that server later.")
		} else {
			description = p.t("Bu lisans bir sunucuya bağlı değil. Kaldırırsanız anahtarı geçersiz olur ve listenizden çıkar. Bu işlem geri alınamaz.",
				"This license is not bound to a server. If you remove it, its key stops working and it leaves your list. This cannot be undone.")
		}
	case "reveal":
		description = htmlEscape(p.t("Anahtarınızı görüntülemek ve kopyalamak için hesap parolanızı girin. Aynı anahtarı sunucunuzun yeniden kurulumunda kullanabilirsiniz.",
			"Enter your account password to view and copy your key. You can reuse the same key when reinstalling your server."))
	case "release":
		description = htmlEscape(p.t("Sunucu eşlemesi kaldırılacak. Aynı anahtarı yeni sunucuda kullanın; bitiş tarihi değişmez. Güncel panel sürümlerinde eski sunucu bunu bir sonraki lisans kontrolünde (kullanımdayken genellikle bir saat içinde) öğrenir ve erişimi kapanır; lisans hizmetine ulaşamayan bir sunucu en çok 7 gün açık kalabilir. Eski sürümleri önce güncelleyin.",
			"The server binding will be released. Use the same key on the new server; its expiry date stays the same. On current panel versions, the old server learns of it at its next license check (usually within an hour while in use) and loses access; a server that cannot reach the license service can stay open for up to 7 days. Update older versions first."))
	case "rotate":
		description = htmlEscape(p.t("Mevcut anahtar iptal edilip yenisi oluşturulacak. Sunucu eşlemesi ve bitiş tarihi korunur. Yeni anahtarı panelin Lisans ayarlarına girin. Güncel panel sürümlerinde eski doğrulama bir sonraki lisans kontrolünde (kullanımdayken genellikle bir saat içinde) geçersiz olur; lisans hizmetine ulaşamayan bir sunucu en çok 7 gün açık kalabilir. Eski sürümleri önce güncelleyin.",
			"Your current key will be replaced. The server binding and expiry date stay the same. Enter the new key in the panel’s License settings. On current panel versions, the previous verification becomes invalid at the panel’s next license check (usually within an hour while in use); a server that cannot reach the license service can stay open for up to 7 days. Update older versions first."))
	default: // renew
		description = htmlEscape(p.t("Şu an yenileme ücretsizdir. Lisans bitişi bir yıl uzatılır.", "Renewal is currently free. The license expiry is extended by one year."))
	}
	if action == "remove" && target != nil && p.store.openOrder(target.ID) {
		description += " " + p.t("Bu lisansın ödenmemiş siparişi de kapanır; hiçbir ücret alınmaz.", "This license's unpaid order closes with it; nothing is charged.")
	}
	b.WriteString(`<p>` + description + `</p>` + "\n")
	p.formStart(b, action)
	value := ""
	if hasID {
		value = id.text()
	}
	b.WriteString(`<input type="hidden" name="id" value="` + htmlEscape(value) + `">` + "\n")
	p.passwordField(b, "current-password")
	var label string
	switch action {
	case "release":
		label = p.t("Sunucu eşlemesini kaldır", "Release server binding")
	case "reveal":
		label = p.t("Anahtarı göster", "Show key")
	case "rotate":
		label = p.t("Anahtarı değiştir", "Replace key")
	case "remove":
		label = p.t("Lisansı kaldır", "Remove license")
	default:
		label = p.t("Bir yıl ücretsiz uzat", "Extend free for one year")
	}
	b.WriteString(`<button type="submit">` + label + `</button></form>` + "\n")
	b.WriteString(`<a href="/account/">` + p.t("Lisanslarıma dön", "Back to licenses") + `</a>` + "\n")
}

// form is view.php:86-93: sign-in, registration, verification and reset forms.
func (p *page) form(b *strings.Builder, action string) {
	if action == "register" {
		b.WriteString(`<p class="member-intro">` + p.t("Bir hesapla sunucularınızın lisanslarını yönetin. Şu an ücretsiz; ödeme bilgisi gerekmez.", "Manage your server licenses from one account. Currently free; no payment details required.") + `</p>`)
	}
	p.formStart(b, action)
	postText := func(name string) string {
		if v, ok := p.post[name]; ok {
			return v.text()
		}
		return ""
	}
	if action != "verify" && action != "reset" {
		b.WriteString(`<label>` + p.t("E-posta adresi", "Email address") + `<input name="email" type="email" required maxlength="254" autocomplete="email" value="` + htmlEscape(postText("email")) + `"></label>`)
	}
	switch action {
	case "login":
		p.passwordField(b, "current-password")
	case "verify", "reset":
		p.passwordField(b, "new-password")
	}
	if action == "verify" || action == "reset" {
		b.WriteString(`<label>` + p.t("E-postanızdaki doğrulama kodu", "Code from your email") + `<input name="token" id="email-token" required pattern="[a-f0-9]{64}" maxlength="64" autocomplete="off" value="` +
			htmlEscape(postText("token")) + `"></label><p>` + p.t("E-postadaki bağlantıyla geldiğinizde bu alan otomatik doldurulur.", "This field is filled automatically when you follow the email link.") + `</p>`)
	}
	if action == "register" {
		b.WriteString(`<p>` + p.t("E-postanızı doğrularken bu hesaba özel bir parola belirleyeceksiniz.", "You will choose a unique password when verifying your email.") +
			`</p><label class="member-check"><input type="checkbox" name="terms" value="yes" required> <span><a href="/account/?action=terms" target="_blank" rel="noopener">` +
			p.t("Kullanım koşulları ve gizlilik açıklamasını", "Terms and privacy notice") + `</a> ` + p.t("okudum ve kabul ediyorum.", "— I have read and accept.") + `</span></label>`)
	}
	var label string
	switch action {
	case "register":
		label = p.t("Hesap oluştur", "Create account")
	case "verify":
		label = p.t("E-postamı doğrula", "Verify my email")
	case "reset":
		label = p.t("Parolayı değiştir", "Change password")
	case "forgot", "resend":
		label = p.t("E-posta gönder", "Send email")
	default:
		label = p.t("Giriş yap", "Sign in")
	}
	b.WriteString(`<button type="submit">` + label + `</button></form>` + "\n")
	first, firstText := "register", p.t("Hesap oluştur", "Create an account")
	if action == "register" {
		first, firstText = "login", p.t("Zaten hesabım var", "I already have an account")
	}
	b.WriteString(`<nav class="member-secondary" aria-label="` + p.t("Hesap yardımı", "Account help") + `"><a href="?action=` + first + `">` + firstText +
		`</a><a href="?action=forgot">` + p.t("Parolamı unuttum", "Forgot password") + `</a><a href="?action=resend">` + p.t("Doğrulama e-postasını yeniden gönder", "Resend verification email") + `</a></nav>` + "\n")
}

// terms is terms.php lines 2-14.
func (p *page) terms() string {
	lines := []string{
		`<section class="member-terms">`,
		`<p>` + p.t("Geçerli metin: 30 Ekim 2026.", "Effective text: 30 October 2026.") + `</p>`,
		`<h2>` + p.t("Ürün ve ücretsiz lisans", "Product and free license") + `</h2>`,
		`<p>` + p.t("CelikPanel, CELIKBROS tarafından sunulan sunucu yönetim yazılımıdır. Şu an bir üyelik hesabından ücretsiz lisans alabilirsiniz. Her lisans bir aktif sunucuda kullanım içindir; oluşturulduğu tarihten itibaren bir takvim yılı geçerlidir. Kendi sitelerinizi barındırabilir veya müşterilerinize ve bayilerinize hosting hizmeti verebilirsiniz.",
			"CelikPanel is server management software provided by CELIKBROS. You can currently obtain free licenses from a member account. Each license covers one active server for one calendar year from creation. You may host your own sites or provide hosting to your customers and resellers.") + `</p>`,
		`<p>` + p.t("Bu kullanım izni yazılımın mülkiyetini veya kaynak kodunu yeniden dağıtma hakkını devretmez. Üçüncü taraf bileşenlerin kendi lisansları geçerlidir. CelikPanel şu anda alfa aşamasındadır; ilk denemenizi temiz bir test sunucusunda yapın.",
			"This permission does not transfer ownership of the software or grant source redistribution rights. Third-party components retain their own licenses. CelikPanel is currently in alpha; begin on a clean test server.") + `</p>`,
		`<h2>` + p.t("Yenileme ve sunucu değişimi", "Renewal and server transfer") + `</h2>`,
		`<p>` + p.t("Yenileme şu anda ücretsizdir; son 30 günde veya süre dolduktan sonra hesabınızdan yapılabilir. Ücretli satış eklenirse fiyat ve koşullar satın alma öncesinde gösterilir; otomatik ödeme alınmaz. Lisans, ilk bağlantıda doğrulanan genel IP adresiyle eşleşir. Aynı IP üzerinde yeniden kurulumda aynı anahtar kullanılabilir. IP değişirse hesabınızdan sunucu eşlemesini kaldırıp aynı anahtarla tekrar etkinleştirin. Bu işlemler mevcut lisansın süresini yeniden başlatmaz.",
			"Renewal is currently free and available from your account during the last 30 days or after expiry. If paid sales are introduced, prices and terms will be shown before purchase; no automatic charges are made. The license binds to the public IP verified on first connection. Reinstalling at the same IP can reuse the same key. If the IP changes, release the server binding from your account and activate with the same key. These operations do not restart the license term.") + `</p>`,
		`<p>` + p.t("Aktif lisans yoksa veya süresi dolmuşsa tüm kullanıcıların panel ve yönetim API erişimi kapanır. Yönetici lisansı etkinleştirebilir veya yenileyebilir ve imzalı CelikPanel güncellemelerini kurabilir. Güncelleme lisansı etkinleştirmez veya süresini uzatmaz; giriş, çıkış ve kendi hesabını kurtarma işlemleri erişilebilir kalır. Çalışan siteler, e-posta, veritabanları ve mevcut zamanlanmış görevler durdurulmaz. Panel veya API kullanılırken panel, tüm kullanıcılar için ortak lisans kontrolünü yaklaşık saatte bir yeniler. Kullanım yokken otomatik merkezi kontrol yapılmaz. Lisans hizmetine ulaşılamazsa son başarılı kontrol panel erişimini en çok 7 gün, hiçbir zaman lisans bitiş tarihinden sonra olmamak üzere açık tutar; kesinti daha uzun sürerse bir kontrol başarılı olana kadar panel erişimi kapalı kalır. Merkez lisansı reddederse (örneğin lisans kaldırıldığında veya başka bir sunucuya taşındığında) panel erişimi bir sonraki kontrolde kapanır. Geri alınan sunucu saati bu süreyi uzatmaz.",
			"Without an active license, or after expiry, panel and management API access is locked for all users. The administrator can activate or renew the license and install signed CelikPanel updates. Updating does not activate or extend the license; sign-in, sign-out and own-account recovery remain available. Running websites, mail, databases and existing scheduled tasks are not stopped. During panel or API use, the panel renews one shared license check for all users about once an hour. Idle servers do not make automatic central checks. If the license service cannot be reached, the last successful check keeps panel access open for up to 7 days, never beyond the license expiry date; if the outage lasts longer, panel access stays locked until a check succeeds. An explicit license rejection (for example after the license is removed or moved to another server) locks panel access at the next check. Setting the server clock back does not extend this period.") + `</p>`,
		`<h2>` + p.t("Hesap ve lisans verileri", "Account and license data") + `</h2>`,
		`<p>` + p.t("CELIKBROS üyelik hizmeti e-posta adresinizi, parolanızın tek yönlü özetini, e-posta doğrulama durumunu, kabul ettiğiniz metnin sürümünü, lisans tarihlerini, anahtar özetlerini ve şifrelenmiş anahtarları, eşleşen genel IP adresini, sunucunun türetilmiş kimliğini ve adını, aktivasyon ve yenileme kayıtlarını saklar. Bu bilgiler hesabı çalıştırmak, lisansları yönetmek ve kötüye kullanımı sınırlamak için kullanılır. IP adresiniz istek sınırlamasında özetlenerek işlenir; web sunucusu erişim günlükleri ayrıca IP adresi içerebilir.",
			"The CELIKBROS membership service stores your email, a one-way password hash, verification status, the accepted text version, license dates, key hashes and encrypted keys, the bound public IP address, a derived server identity and hostname, and activation and renewal records. These support account operation, license management and abuse prevention. Request limits use a hash derived from your IP address; web server access logs may separately contain IP addresses.") + `</p>`,
		`<p>` + p.t("Sunucunuzun root parolası, panel yöneticisi parolası, müşteri hesapları, site dosyaları ve e-postalar lisans hizmetine gönderilmez. Giriş oturumu için gerekli bir çerez kullanılır. Doğrulama ve parola yenileme e-postaları mevcut posta hizmeti üzerinden gönderilir; bu üyelik akışı reklam veya analiz çerezi eklemez.",
			"Your server root password, panel administrator password, customer accounts, site files and emails are not sent to the licensing service. An essential session cookie is used for sign-in. Verification and password reset emails use the existing mail service; this membership flow adds no advertising or analytics cookies.") + `</p>`,
		`<p>` + p.t("Hesap ve lisans kayıtları hizmeti sağlamak için saklanır. Hesap, veri veya lisansla ilgili talepleriniz için websitesinin Destek bölümündeki iletişim yolunu kullanın.",
			"Account and license records are retained to provide the service. For account, data or license requests, use the contact route in the website’s Support section.") + ` <a href="/#destek">` + p.t("Destek", "Support") + `</a></p>`,
		`</section>`,
	}
	return strings.Join(lines, "\n") + "\n"
}
