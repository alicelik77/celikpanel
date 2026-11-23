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
	receipt  *Order           // the receipt page's order
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

// storeSummary is the line under the free licence button: free during the beta, and what
// test mode means.
func (p *page) storeSummary(b *strings.Builder) {
	if p.store == nil {
		return
	}
	switch p.store.charging {
	case ChargingTest:
		b.WriteString(`<p class="member-notice" role="status">` + p.t(
			"Deneme kipi: satın almalar ödeme sağlayıcısının deneme kipiyle yapılır; gerçek para alınmaz.",
			"Test mode: purchases use the payment provider's test mode; no real money is taken.") + `</p>` + "\n")
	default:
		b.WriteString(`<p class="member-notice" role="status">` + p.t(
			"Beta süresince CelikPanel ücretsizdir. Ücretlendirme 1.0 sürümüyle başlar.",
			"CelikPanel is free during the beta. Charging starts with version 1.0.") + `</p>` + "\n")
	}
}

// licenceStore is the store's part of one licence card: its state in plain words (active /
// ends in N days / ended: the owner, 23 November 2026 -- no grace; without a licence the
// panel is closed and the server keeps running), and "Buy a year" or "Renew for a year".
func (p *page) licenceStore(b *strings.Builder, l License) {
	if p.store == nil {
		return
	}
	if p.store.revoked[l.ID] {
		b.WriteString(`<p class="member-state">` + p.t("Bu lisansın ödemesi geri alındı; lisans sona erdi.", "This license's payment was reversed; the license has ended.") + `</p>`)
		return
	}
	if p.store.charging != ChargingTest || !p.store.ready {
		return
	}
	if l.Expires.Valid && l.Expires.Int64 != 0 {
		end := day(l.Expires.Int64)
		switch left := (l.Expires.Int64 - p.now) / 86400; {
		case l.Expires.Int64 <= p.now:
			b.WriteString(`<p class="member-state">` + p.t(end+" tarihinde sona erdi. O sunucudaki panel, yeni bir yıl alana kadar kapalı kalır; sunucu, siteler, e-posta ve DNS çalışmaya devam eder.",
				"Ended on "+end+". The panel on that server stays closed until you buy a new year; the server and its sites, mail and DNS keep running.") + `</p>`)
		case left < 30:
			b.WriteString(`<p class="member-state">` + p.t(strconv.FormatInt(left, 10)+" gün sonra sona eriyor ("+end+"). Panelin açık kalması ve fiyatınızın aynı kalması için o tarihten önce yenileyin.",
				"Ends in "+strconv.FormatInt(left, 10)+" days ("+end+"). Renew before then to keep the panel open and your price unchanged.") + `</p>`)
		}
	}
	q, ok := p.store.quotes[l.ID]
	if !ok {
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
		b.WriteString(`<p>` + p.t("Bitiş tarihinden önce yenilediğiniz için fiyatınız aynı kalır.", "Because you renew before the end date, your price stays the same.") + `</p>`)
	}
	b.WriteString(`<p class="member-notice" role="note"><strong>` + p.t("Ödemeler iade edilmez.", "Payments are not refunded.") + `</strong> ` +
		p.t("Önce denemek isterseniz ilk seferde 30 günlük ücretsiz deneme kullanabilirsiniz.", "If you want to try first, the free 30-day trial is there for your first time.") + `</p>` + "\n")
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
		state = p.t("Bu ödemeyi siparişle eşleştiremedik.", "We could not match this payment to the order.")
		next = p.t("Lisansınızda hiçbir şey değişmedi. Durumu biz inceliyoruz ve size e-postayla yazacağız; sizin bir şey yapmanız gerekmiyor.",
			"Nothing changed on your license. We are looking into it and will write to you by email; you do not need to do anything.")
	case OrderRefunded:
		state = p.t("Ödeme geri alındı.", "The payment was reversed.")
		next = p.t("Ödeme size geri döndü ve bu lisans sona erdi.", "The payment went back to you and this license has ended.")
		if o.Kind == KindRenewal {
			next = p.t("Ödeme size geri döndü ve bu ödemenin aldığı yıl geri alındı.", "The payment went back to you and the year it bought was taken back.")
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
