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
	price    int64
	currency string
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

// licenceStore is the store's part of one licence card: refunded, or "Buy a year".
func (p *page) licenceStore(b *strings.Builder, l License) {
	if p.store == nil {
		return
	}
	if p.store.revoked[l.ID] {
		b.WriteString(`<p class="member-state">` + p.t("Ödemesi iade edildi; bu lisans sona erdi.", "Its payment was refunded; this license has ended.") + `</p>`)
		return
	}
	if p.store.charging != ChargingTest || !p.store.ready {
		return
	}
	for _, o := range p.store.orders {
		if o.LicenseID == l.ID && (o.State == OrderPaid || o.State == OrderRefunded) {
			return // a paid year exists: "Renew" waits for the owner's answers
		}
	}
	p.formStart(b, "buy")
	b.WriteString(`<input type="hidden" name="id" value="` + htmlEscape(l.ID) + `"><button type="submit">` +
		p.t("Bir yıl satın al", "Buy a year") + ` · ` + htmlEscape(money(p.store.price, p.store.currency)) + `</button></form>`)
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
		state = p.t("İade edildi.", "Refunded.")
		next = p.t("Ödemeniz iade edildi ve bu lisans sona erdi.", "Your payment was refunded and this license has ended.")
	}
	b.WriteString(`<p class="member-notice" role="status"><strong>` + state + `</strong> ` + htmlEscape(next) + `</p>` + "\n")
	b.WriteString(`<a href="/account/">` + p.t("Lisanslarıma dön", "Back to licenses") + `</a>` + "\n")
}
