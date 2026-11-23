package membership

// The store's parts of the member's pages (S2). They appear only when the store exists
// (`membership migrate-store`); a file without it renders exactly the PHP service's pages.
// Every value goes through htmlEscape; EN and TR say the same thing.

import (
	"strconv"
	"strings"
	"time"
)

// storePage is what the pages know about the store for the signed-in member.
type storePage struct {
	charging ChargingMode
	ready    bool // the payment provider can be used (charging test, keys set)
	orders   []Order
	revoked  map[string]bool
	order    *Order // the order page's order
	quotes   map[string]Quote
	buy      *License // the page before payment: its licence and quote
	quote    Quote
	paidAt   map[string]int64 // order id -> first confirmation of its payment
	// charged: this account buys its years (test mode: flagged; S2 review, 2 and 3). Everyone
	// else sees the beta pages.
	charged      bool
	trialID      string // the account's trial licence (" when none)
	trialEnds    int64
	trialUsed    bool
	awaiting     map[string]bool // licences waiting for their first year to be paid
	trialRefused bool            // the trial licence was refused on a server that had a trial
	// The order page's cause when it did not become paid normally (Service.Cause).
	causeKind, causeDetail string
	causeOpen, settled     bool
	receipt                *Order // the receipt page's order
}

func money(minor int64, currency string) string {
	return strconv.FormatInt(minor/100, 10) + "." + leftPad(strconv.FormatInt(minor%100, 10), 2) + " " + currency
}

func leftPad(s string, n int) string {
	for len(s) < n {
		s = "0" + s
	}
	return s
}

func day(at int64) string { return time.Unix(at, 0).UTC().Format("2006-01-02") }

func moment(at int64) string { return time.Unix(at, 0).UTC().Format("2006-01-02 15:04") + " UTC" }

// storeSummary is the line under the free licence button: free during the beta, and what
// test mode means.
func (p *page) storeSummary(b *strings.Builder) {
	if p.store == nil {
		return
	}
	switch {
	case p.store.charged && p.store.charging == ChargingTest:
		b.WriteString(`<p class="member-notice" role="status">` + p.t(
			"Deneme kipi: satın almalar ödeme sağlayıcısının deneme kipiyle yapılır; gerçek para alınmaz.",
			"Test mode: purchases use the payment provider's test mode; no real money is taken.") + `</p>` + "\n")
	default:
		b.WriteString(`<p class="member-notice" role="status">` + p.t(
			"Beta süresince CelikPanel ücretsizdir. Ücretlendirme 1.0 sürümüyle başlar.",
			"CelikPanel is free during the beta. Charging starts with version 1.0.") + `</p>` + "\n")
	}
}

// chargedSummary replaces "Create a free license" for an account that buys its years: the
// first licence is the 30-day trial; a later one waits for its first year to be paid.
func (p *page) chargedSummary(b *strings.Builder) {
	text, button := p.t("İlk lisansınız 30 günlük ücretsiz denemedir; ödeme bilgisi istenmez. Panelin açık kalması için deneme bitmeden bir yıl satın alın.",
		"Your first license is a free 30-day trial; no payment details are asked. Buy a year before it ends to keep the panel open."),
		p.t("30 günlük denemeyi başlat", "Start the 30-day trial")
	if p.store.trialUsed {
		text, button = p.t("Yeni bir lisans, ilk yılı ödendiğinde kullanılabilir.", "A new license can be used once its first year is paid."),
			p.t("Lisans oluştur", "Create a license")
	}
	b.WriteString(`<section class="member-summary"><div><h2>CelikPanel</h2><p>` + text + `</p></div>`)
	p.formStart(b, "issue")
	b.WriteString(`<button type="submit">` + button + `</button></form></section>` + "\n")
}

// licenceStore is the store's part of one licence card for an account that buys its years:
// its state in plain words (trial / awaiting payment / ends in N days / ended: the owner,
// 23 November 2026 -- no grace; without a licence the panel is closed and the server keeps
// running), and "Buy a year" or "Renew for a year".
func (p *page) licenceStore(b *strings.Builder, l License) {
	if p.store == nil {
		return
	}
	if p.store.revoked[l.ID] {
		b.WriteString(`<p class="member-state">` + p.t("Bu lisansın ödemesi geri alındı; lisans sona erdi.", "This license's payment was reversed; the license has ended.") + `</p>`)
		return
	}
	if !p.store.charged {
		return
	}
	q, quoted := p.store.quotes[l.ID]
	switch end := l.Expires.Int64; {
	case p.store.awaiting[l.ID]:
		b.WriteString(`<p class="member-state">` + p.t("Ödeme bekleniyor: bu lisans ilk yılı ödendiğinde kullanılabilir.", "Awaiting payment: this license can be used once its first year is paid.") + `</p>`)
	case l.ID == p.store.trialID && end == p.store.trialEnds && end > p.now:
		// The trial line only while the licence is still in its trial (no paid year, no gift).
		b.WriteString(`<p class="member-state">` + p.t("Ücretsiz deneme "+day(end)+" tarihine kadar. Panelin açık kalması için o tarihten önce bir yıl satın alın.",
			"Free trial until "+day(end)+". Buy a year before then to keep the panel open.") + `</p>`)
		if p.store.trialRefused {
			b.WriteString(`<p class="member-state">` + p.t("Bu sunucu daha önce bir deneme kullandı; deneme orada başlamaz. Lisansı orada kullanmak için bir yıl satın alın.",
				"That server already used a trial, so the trial does not start there. Buy a year to use this license on it.") + `</p>`)
		}
	case end <= p.now:
		b.WriteString(`<p class="member-state">` + p.t(day(end)+" tarihinde sona erdi. O sunucudaki panel, yeni bir yıl alana kadar kapalı kalır; sunucu, siteler, e-posta ve DNS çalışmaya devam eder.",
			"Ended on "+day(end)+". The panel on that server stays closed until you buy a new year; the server and its sites, mail and DNS keep running.") + `</p>`)
	case end-p.now < 30*86400:
		left := strconv.FormatInt((end-p.now)/86400, 10)
		if quoted && q.Locked {
			b.WriteString(`<p class="member-state">` + p.t(left+" gün sonra sona eriyor ("+day(end)+"). Panelin açık kalması ve fiyatınızın aynı kalması için o tarihten önce yenileyin.",
				"Ends in "+left+" days ("+day(end)+"). Renew before then to keep the panel open and your price unchanged.") + `</p>`)
		} else {
			b.WriteString(`<p class="member-state">` + p.t(left+" gün sonra sona eriyor ("+day(end)+"). Panelin açık kalması için o tarihten önce bir yıl alın.",
				"Ends in "+left+" days ("+day(end)+"). Buy a year before then to keep the panel open.") + `</p>`)
		}
	}
	if !quoted || !p.store.ready {
		return // the state is said above; only buying needs the payment provider
	}
	if q.WaitUntilEnd {
		b.WriteString(`<p>` + p.t("Yeni yıl, "+day(l.Expires.Int64)+" bitişinden sonra satın alınabilir.", "The new year can be bought after the end on "+day(l.Expires.Int64)+".") + `</p>`)
		return
	}
	label := p.t("Bir yıl satın al", "Buy a year")
	if q.Kind == KindRenewal {
		label = p.t("Bir yıl yenile", "Renew for a year")
	}
	b.WriteString(`<p><a href="?action=buy&amp;id=` + htmlEscape(l.ID) + `&amp;lang=` + htmlEscape(p.lang) + `">` + label + ` · ` + htmlEscape(money(q.AmountMinor, q.Currency)) + `</a></p>`)
}

// buyPage is the page before payment: what the member buys, from when to when, the price,
// and -- plainly, before paying -- that payments are not refunded and that the trial is for
// trying first (the owner, 23 November 2026). The legal wording of the terms is the owner's
// lawyer's matter, not this page's.
func (p *page) buyPage(b *strings.Builder) {
	l, q := p.store.buy, p.store.quote
	name := p.t("Aktivasyon bekliyor", "Awaiting activation")
	if nullTruthy(l.Hostname.Valid, l.Hostname.String) {
		name = l.Hostname.String
	}
	b.WriteString(`<dl><dt>` + p.t("Lisans", "License") + `</dt><dd>` + htmlEscape(name) + ` <code>…` + htmlEscape(l.KeySuffix) + `</code></dd>`)
	b.WriteString(`<dt>` + p.t("Süre (UTC)", "Term (UTC)") + `</dt><dd>` + day(q.From) + ` – ` + day(q.To) + `</dd>`)
	b.WriteString(`<dt>` + p.t("Tutar", "Amount") + `</dt><dd>` + htmlEscape(money(q.AmountMinor, q.Currency)) + `</dd></dl>` + "\n")
	if q.From > p.now {
		b.WriteString(`<p>` + p.t("Yeni yıl, şimdiki bitiş tarihinizden sonra başlar; kalan günleriniz kaybolmaz.", "The new year starts after your current end date; no remaining days are lost.") + `</p>`)
	} else {
		b.WriteString(`<p>` + p.t("Yıl, ödemeniz onaylandığında başlar.", "The year starts when your payment is confirmed.") + `</p>`)
	}
	if q.Locked {
		b.WriteString(`<p>` + p.t("Bitiş tarihinden önce yenilediğiniz için fiyatınız aynı kalır. Bu ödeme "+moment(q.Expires)+" tarihine kadar yapılabilir.", "Because you renew before the end date, your price stays the same. This payment can be made until "+moment(q.Expires)+".") + `</p>`)
	}
	b.WriteString(`<p class="member-notice" role="note"><strong>` + p.t("Ödemeler iade edilmez.", "Payments are not refunded.") + `</strong> ` +
		p.trialNote() + `</p>` + "\n")
	if p.store.charging == ChargingTest {
		b.WriteString(`<p>` + p.t("Deneme kipi: gerçek para alınmaz.", "Test mode: no real money is taken.") + `</p>`)
	}
	p.formStart(b, "buy")
	b.WriteString(`<input type="hidden" name="id" value="` + htmlEscape(l.ID) + `"><button type="submit">` + p.t("Ödemeye geç", "Continue to payment") + `</button></form>` + "\n")
	b.WriteString(`<a href="/account/">` + p.t("Lisanslarıma dön", "Back to licenses") + `</a>` + "\n")
}

// orderPage is the page the member returns to from the provider, and every order's page.
// It says the state, who acts next and how it ends; it never leaves "pending" without an
// end.
func (p *page) orderPage(b *strings.Builder) {
	o := p.store.order
	b.WriteString(`<dl><dt>` + p.t("Lisans", "License") + `</dt><dd><code>…` + htmlEscape(o.LicenseID[len(o.LicenseID)-8:]) + `</code></dd>`)
	b.WriteString(`<dt>` + p.t("Tutar", "Amount") + `</dt><dd>` + htmlEscape(money(o.AmountMinor, o.Currency)))
	if !o.Live {
		b.WriteString(` (` + p.t("deneme kipi", "test mode") + `)`)
	}
	b.WriteString(`</dd><dt>` + p.t("Oluşturuldu (UTC)", "Created (UTC)") + `</dt><dd>` + day(o.Created) + `</dd></dl>` + "\n")
	var state, next string
	switch o.State {
	case OrderCreated, OrderPending:
		state = p.t("Ödemeniz doğrulanıyor.", "Your payment is being confirmed.")
		next = p.t("Sizin bir şey yapmanız gerekmiyor. Ödeme sağlayıcısı ödemeyi onayladığında lisansınız uzar; bu sayfayı yenileyerek tekrar bakabilirsiniz. Ödeme yapmadan ayrıldıysanız sipariş bir gün içinde kendiliğinden kapanır ve hiçbir ücret alınmaz.",
			"You do not need to do anything. When the payment provider confirms the payment, your license is extended; reload this page to check again. If you left without paying, the order closes by itself within a day and nothing is charged.")
	case OrderPaid:
		state = p.t("Ödendi.", "Paid.")
		next = p.t("Lisansınız "+day(o.GrantedTo)+" tarihine kadar geçerli. Sunucunuz yeni tarihi bir sonraki lisans kontrolünde alır.",
			"Your license now runs until "+day(o.GrantedTo)+". Your server receives the new date at its next license check.")
	case OrderFailed:
		state = p.t("Ödeme gerçekleşmedi.", "The payment did not go through.")
		next = p.t("Hiçbir ücret alınmadı. Lisanslarınızdan yeniden deneyebilirsiniz.", "Nothing was charged. You can try again from your licenses.")
	case OrderExpired:
		state = p.t("Sipariş ödenmeden kapandı.", "The order closed unpaid.")
		next = p.t("Hiçbir ücret alınmadı. Lisanslarınızdan yeniden deneyebilirsiniz.", "Nothing was charged. You can try again from your licenses.")
	case OrderMismatch:
		state, next = p.mismatchText()
	case OrderRefunded:
		state = p.t("Ödeme geri alındı.", "The payment was reversed.")
		// What really happened to the licence (S2 third look, F7).
		switch {
		case o.GrantedFrom == 0:
			next = p.t("Ödeme size geri döndü; lisansınızda hiçbir şey değişmedi.", "The payment went back to you; nothing changed on your license.")
		case p.store.revoked[o.LicenseID]:
			next = p.t("Ödeme size geri döndü ve aldığı süre geri alındı; lisans sona erdi.", "The payment went back to you and the time it bought was taken back; the license has ended.")
		default:
			end := ""
			for _, l := range p.licenses {
				if l.ID == o.LicenseID && l.Expires.Valid {
					end = day(l.Expires.Int64)
				}
			}
			next = p.t("Ödeme size geri döndü ve aldığı süre geri alındı; lisansınız artık "+end+" tarihine kadar geçerli.", "The payment went back to you and the time it bought was taken back; your license now runs until "+end+".")
		}
	}
	b.WriteString(`<p class="member-notice" role="status"><strong>` + state + `</strong> ` + htmlEscape(next) + `</p>` + "\n")
	b.WriteString(`<a href="/account/">` + p.t("Lisanslarıma dön", "Back to licenses") + `</a>` + "\n")
}

// payments is the member's list of orders on the home page: what was paid, when, for which
// licence and term, and a receipt for each payment; an order still being confirmed links to
// its page. Orders that closed unpaid are not listed (nothing was charged).
func (p *page) payments(b *strings.Builder) {
	if p.store == nil {
		return
	}
	var rows []Order
	for _, o := range p.store.orders {
		if o.State != OrderExpired && o.State != OrderFailed {
			rows = append(rows, o)
		}
	}
	if len(rows) == 0 {
		return
	}
	b.WriteString(`<section class="member-payments" aria-labelledby="payments"><h2 id="payments">` + p.t("Ödemeleriniz", "Your payments") + `</h2><ul>`)
	for _, o := range rows {
		b.WriteString(`<li>` + htmlEscape(money(o.AmountMinor, o.Currency)) + ` · ` + p.t("lisans", "license") + ` <code>…` + htmlEscape(o.LicenseID[len(o.LicenseID)-8:]) + `</code> · `)
		switch o.State {
		case OrderPaid:
			b.WriteString(p.t("ödendi ", "paid ") + day(p.store.paidAt[o.ID]) + ` · ` + day(o.GrantedFrom) + ` – ` + day(o.GrantedTo) +
				` · <a href="?action=receipt&amp;id=` + htmlEscape(o.ID) + `&amp;lang=` + htmlEscape(p.lang) + `">` + p.t("Makbuz", "Receipt") + `</a>`)
		case OrderRefunded:
			b.WriteString(p.t("ödeme geri alındı", "payment reversed"))
			if p.store.paidAt[o.ID] != 0 {
				b.WriteString(` · <a href="?action=receipt&amp;id=` + htmlEscape(o.ID) + `&amp;lang=` + htmlEscape(p.lang) + `">` + p.t("Makbuz", "Receipt") + `</a>`)
			}
		case OrderMismatch:
			b.WriteString(p.t("inceliyoruz; size e-postayla yazacağız", "we are looking into it and will write to you by email"))
		default:
			b.WriteString(`<a href="?action=order&amp;id=` + htmlEscape(o.ID) + `&amp;lang=` + htmlEscape(p.lang) + `">` + p.t("doğrulanıyor", "being confirmed") + `</a>`)
		}
		if !o.Live {
			b.WriteString(` (` + p.t("deneme kipi", "test mode") + `)`)
		}
		b.WriteString(`</li>`)
	}
	b.WriteString(`</ul></section>` + "\n")
}

// receiptPage shows what was paid, when, for which licence and term. It is not a tax
// invoice, and says so.
func (p *page) receiptPage(b *strings.Builder) {
	o := p.store.receipt
	b.WriteString(`<dl><dt>` + p.t("Sipariş", "Order") + `</dt><dd><code>` + htmlEscape(o.ID) + `</code></dd>`)
	b.WriteString(`<dt>` + p.t("Ödeme tarihi (UTC)", "Paid on (UTC)") + `</dt><dd>` + day(p.store.paidAt[o.ID]) + `</dd>`)
	b.WriteString(`<dt>` + p.t("Tutar", "Amount") + `</dt><dd>` + htmlEscape(money(o.AmountMinor, o.Currency)) + `</dd>`)
	b.WriteString(`<dt>` + p.t("Lisans", "License") + `</dt><dd><code>…` + htmlEscape(o.LicenseID[len(o.LicenseID)-8:]) + `</code></dd>`)
	what := p.t("İlk yıl", "First year")
	if o.Kind == KindRenewal {
		what = p.t("Yenileme", "Renewal")
	}
	b.WriteString(`<dt>` + p.t("Ne için", "For") + `</dt><dd>` + what)
	if o.GrantedFrom != 0 {
		b.WriteString(`, ` + day(o.GrantedFrom) + ` – ` + day(o.GrantedTo))
	}
	b.WriteString(`</dd>`)
	if o.State == OrderRefunded {
		b.WriteString(`<dt>` + p.t("Durum", "Status") + `</dt><dd>` + p.t("Ödeme geri alındı", "Payment reversed") + `</dd>`)
	}
	b.WriteString(`</dl>` + "\n")
	if !o.Live {
		b.WriteString(`<p>` + p.t("Deneme kipi: gerçek para alınmadı.", "Test mode: no real money was taken.") + `</p>`)
	}
	b.WriteString(`<p class="member-notice" role="note">` + p.t("Bu makbuz bir vergi faturası değildir.", "This receipt is not a tax invoice.") + `</p>` + "\n")
	b.WriteString(`<a href="/account/">` + p.t("Lisanslarıma dön", "Back to licenses") + `</a>` + "\n")
}

// openOrder says whether a licence has an unpaid order (nil-safe: no store, no orders).
func (s *storePage) openOrder(licenseID string) bool {
	if s == nil {
		return false
	}
	for _, o := range s.orders {
		if o.LicenseID == licenseID && (o.State == OrderCreated || o.State == OrderPending) {
			return true
		}
	}
	return false
}

// mismatchText says, for each cause, what is true about an order that did not become paid
// normally -- and that the payment went back once that is settled (S2 review 2, N8).
func (p *page) mismatchText() (string, string) {
	// "We will write to you" only while the matter is open for the operator (S2 third look, F7).
	contact := ""
	if p.store.causeOpen {
		contact = p.t("Size e-postayla yazacağız; sizin bir şey yapmanız gerekmiyor.", "We will write to you by email; you do not need to do anything.")
	}
	if p.store.settled {
		return p.t("Ödeme size geri döndü.", "The payment went back to you."), p.t("Lisansınızda hiçbir şey değişmedi.", "Nothing changed on your license.")
	}
	switch {
	case p.store.causeKind == AttentionNotGranted && p.store.causeDetail == NotGrantedRemoved:
		return p.t("Ödemeniz ulaştı, ancak ilgili lisans artık yok; bu yüzden yıl eklenemedi.", "Your payment arrived, but the license it was for no longer exists, so no year could be added."), contact
	case p.store.causeKind == AttentionNotGranted && p.store.causeDetail == NotGrantedRevoked:
		return p.t("Ödemeniz ulaştı, ancak ilgili lisans sona ermişti; bu yüzden yıl eklenemedi.", "Your payment arrived, but the license it was for had ended, so no year could be added."), contact
	case p.store.causeKind == AttentionNotGranted && p.store.causeDetail == NotGrantedUnflagged:
		return p.t("Ödemeniz ulaştı, ancak bu hesap artık deneme kipinde satın almıyor; bu yüzden yıl eklenemedi.", "Your payment arrived, but this account no longer buys in test mode, so no year could be added."), contact
	}
	return p.t("Bu ödemeyi siparişle eşleştiremedik.", "We could not match this payment to the order."),
		strings.TrimSpace(p.t("Lisansınızda hiçbir şey değişmedi. ", "Nothing changed on your license. ") + contact)
}

// trialNote is the buy page's trial sentence, only for an account that can still have the
// trial (S2 third look, F7).
func (p *page) trialNote() string {
	if p.store.trialUsed {
		return ""
	}
	return p.t("İlk seferde 30 günlük ücretsiz deneme kullanılabilir.", "A free 30-day trial is available the first time.")
}
