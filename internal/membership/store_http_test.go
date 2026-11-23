package membership

// S2, step 2: "Buy a year" through the pages, the return page settled by Confirm, the
// webhook route and reconciliation -- against a fake Stripe only (no test reaches the real
// provider: the payment package refuses any stripe.com address while testing).

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/payment"
)

// fakeStripe answers the few Stripe calls the store makes, from its own state.
type fakeStripe struct {
	mu       sync.Mutex
	server   *httptest.Server
	sessions map[string]*fakeSession // by session id
	byKey    map[string]string       // idempotency key -> session id
	down     bool                    // answer 500 to every request
	posts    int
	refunds  int
}

type fakeSession struct {
	id, order, licence string
	amount             int64
	status             string // open, complete, expired
	paid               bool
	refunded           int64
	disputed           bool
	dispute            string // the dispute's status when disputed
	livemode           bool
	expiresAt          string // the expires_at the store sent
}

func newFakeStripe(t *testing.T) *fakeStripe {
	f := &fakeStripe{sessions: map[string]*fakeSession{}, byKey: map[string]string{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeStripe) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		w.WriteHeader(500)
		io.WriteString(w, `{"error":{}}`)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer sk_test_") {
		w.WriteHeader(401)
		return
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/checkout/sessions":
		f.posts++
		r.ParseForm()
		key := r.Header.Get("Idempotency-Key")
		id, ok := f.byKey[key]
		if !ok {
			id = "cs_test_" + strconv.Itoa(len(f.sessions)+1)
			amount, _ := strconv.ParseInt(r.PostForm.Get("line_items[0][price_data][unit_amount]"), 10, 64)
			f.sessions[id] = &fakeSession{id: id, order: r.PostForm.Get("client_reference_id"), licence: r.PostForm.Get("metadata[license_id]"), amount: amount, status: "open",
				expiresAt: r.PostForm.Get("expires_at")}
			f.byKey[key] = id
		}
		io.WriteString(w, `{"id":"`+id+`","url":"https://checkout.example/`+id+`"}`)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/expire") && strings.HasPrefix(r.URL.Path, "/v1/checkout/sessions/"):
		s := f.sessions[strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/checkout/sessions/"), "/expire")]
		if s == nil {
			w.WriteHeader(404)
			return
		}
		s.status = "expired"
		io.WriteString(w, f.sessionJSON(s))
	case r.Method == http.MethodGet && r.URL.Path == "/v1/disputes":
		for _, s := range f.sessions {
			if "ch_"+s.id == r.URL.Query().Get("charge") && s.disputed {
				io.WriteString(w, `{"data":[{"id":"dp_`+s.id+`","status":"`+s.dispute+`"}]}`)
				return
			}
		}
		io.WriteString(w, `{"data":[]}`)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/checkout/sessions/"):
		s := f.sessions[strings.TrimPrefix(r.URL.Path, "/v1/checkout/sessions/")]
		if s == nil {
			w.WriteHeader(404)
			return
		}
		io.WriteString(w, f.sessionJSON(s))
	case r.Method == http.MethodPost && r.URL.Path == "/v1/refunds":
		f.refunds++
		r.ParseForm()
		for _, s := range f.sessions {
			if "pi_"+s.id == r.PostForm.Get("payment_intent") && s.paid {
				s.refunded = s.amount
				io.WriteString(w, `{"id":"re_`+s.id+`","status":"succeeded"}`)
				return
			}
		}
		w.WriteHeader(400)
		io.WriteString(w, `{"error":{"code":"resource_missing"}}`)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/payment_intents/"):
		for _, s := range f.sessions {
			if "pi_"+s.id == strings.TrimPrefix(r.URL.Path, "/v1/payment_intents/") {
				io.WriteString(w, f.intentJSON(s))
				return
			}
		}
		w.WriteHeader(404)
	default:
		w.WriteHeader(404)
	}
}

func (f *fakeStripe) intentJSON(s *fakeSession) string {
	status, received := "requires_payment_method", int64(0)
	if s.paid {
		status, received = "succeeded", s.amount
	}
	return `{"id":"pi_` + s.id + `","status":"` + status + `","amount":` + strconv.FormatInt(s.amount, 10) + `,"amount_received":` + strconv.FormatInt(received, 10) +
		`,"currency":"usd","livemode":` + strconv.FormatBool(s.livemode) + `,"metadata":{"order":"` + s.order + `","license_id":"` + s.licence + `"},` +
		`"latest_charge":{"id":"ch_` + s.id + `","payment_intent":"pi_` + s.id + `","paid":` + strconv.FormatBool(s.paid) + `,"amount":` + strconv.FormatInt(s.amount, 10) +
		`,"amount_captured":` + strconv.FormatInt(received, 10) + `,"amount_refunded":` + strconv.FormatInt(s.refunded, 10) + `,"refunded":` + strconv.FormatBool(s.paid && s.refunded == s.amount) +
		`,"disputed":` + strconv.FormatBool(s.disputed) + `,"currency":"usd"}}`
}

func (f *fakeStripe) sessionJSON(s *fakeSession) string {
	paymentStatus, intent := "unpaid", "null"
	if s.paid {
		paymentStatus, intent = "paid", f.intentJSON(s)
	}
	return `{"id":"` + s.id + `","url":"https://checkout.example/` + s.id + `","mode":"payment","status":"` + s.status + `","payment_status":"` + paymentStatus + `","livemode":` + strconv.FormatBool(s.livemode) +
		`,"client_reference_id":"` + s.order + `","metadata":{"license_id":"` + s.licence + `"},"payment_intent":` + intent + `}`
}

// pay marks a session paid, as the payer finishing on Stripe's page does.
func (f *fakeStripe) pay(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions[id].paid, f.sessions[id].status = true, "complete"
}

func (f *fakeStripe) session(id string) *fakeSession {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sessions[id]
}

// storeSite is the account site over TLS with a store, a test-mode Stripe pointed at the
// fake, a test account with one licence, and a signed-in browser.
type storeSite struct {
	t       *testing.T
	dir     string
	server  *httptest.Server
	handler *Handler
	stripe  *fakeStripe
	client  *http.Client
	csrf    string
	webhook string // the webhook signing secret (throwaway)
	s       *Service
	member  int64
	licence Issued
	logged  []string
	mails   []sentMail
	mu      sync.Mutex
}

var csrfField = regexp.MustCompile(`name="csrf" value="([a-f0-9]{64})"`)

func newStoreSite(t *testing.T, charging string, keyMode string) *storeSite {
	t.Helper()
	site := &storeSite{t: t, dir: t.TempDir(), stripe: newFakeStripe(t), webhook: "whsec_" + randomHex(16)}
	site.handler = &Handler{Private: site.dir, RemoteAddr: PeerAddress,
		Payments: payment.NewRegistry(payment.Manual{}, &payment.Stripe{BaseURL: site.stripe.server.URL}),
		Log:      func(line string) { site.mu.Lock(); site.logged = append(site.logged, line); site.mu.Unlock() }}
	site.handler.sendMail = func(to, subject, body string) error {
		site.mu.Lock()
		defer site.mu.Unlock()
		site.mails = append(site.mails, sentMail{Email: to, Purpose: subject, Token: body})
		return nil
	}
	site.server = httptest.NewUnstartedServer(site.handler)
	site.server.StartTLS()
	t.Cleanup(site.server.Close)
	site.handler.AllowTestKey = writeFixture(t, site.dir, "https://account.example")
	secret := "sk_" + keyMode + "_" + randomHex(12)
	site.setConfig(map[string]any{"charging": charging,
		"payment": map[string]string{"provider": "stripe", "mode": keyMode, "secret_key": secret, "webhook_secret": site.webhook}})
	config, err := LoadConfig(site.dir)
	if err != nil {
		t.Fatal(err)
	}
	var token string
	site.s, err = Open(filepath.Join(site.dir, "members.sqlite"), config.SigningKey, func(_, _, tk string) error { token = tk; return nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { site.s.Close() })
	if err := site.s.Register("buyer@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := site.s.ConsumeToken(token, "verify", "buyer-password-1234"); err != nil {
		t.Fatal(err)
	}
	site.member = must(site.s.Login("buyer@example.com", "buyer-password-1234")).ID
	site.licence = must(site.s.Issue(site.member))
	if err := site.s.MigrateStore("operator:test"); err != nil {
		t.Fatal(err)
	}
	if err := site.s.SetTestAccount("operator:test", "store tests", site.member, true); err != nil {
		t.Fatal(err)
	}
	site.client = site.server.Client()
	site.client.Jar, _ = cookiejar.New(nil)
	site.client.Timeout = 30 * time.Second
	site.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	site.page(http.MethodGet, "action=login&lang=en", nil)
	if status, _, _ := site.page(http.MethodPost, "action=login&lang=en", url.Values{"email": {"buyer@example.com"}, "password": {"buyer-password-1234"}}); status != 303 {
		t.Fatal("sign-in failed")
	}
	site.page(http.MethodGet, "lang=en", nil) // the session's new CSRF value
	return site
}

// setConfig adds fields to the private config.json.
func (site *storeSite) setConfig(fields map[string]any) {
	site.t.Helper()
	path := filepath.Join(site.dir, "config.json")
	var config map[string]any
	raw, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(raw, &config) != nil {
		site.t.Fatal("config.json")
	}
	for k, v := range fields {
		config[k] = v
	}
	raw, _ = json.Marshal(config)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		site.t.Fatal(err)
	}
}

// page sends a page request; a POST carries the current CSRF value.
func (site *storeSite) page(method, query string, form url.Values) (int, http.Header, string) {
	site.t.Helper()
	var response *http.Response
	var err error
	target := site.server.URL + "/account/?" + query
	if method == http.MethodPost {
		if form == nil {
			form = url.Values{}
		}
		form.Set("csrf", site.csrf)
		response, err = site.client.PostForm(target, form)
	} else {
		response, err = site.client.Get(target)
	}
	if err != nil {
		site.t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	if m := csrfField.FindStringSubmatch(string(raw)); m != nil {
		site.csrf = m[1]
	}
	return response.StatusCode, response.Header, string(raw)
}

// event posts a signed (or forged) notification to the webhook route.
func (site *storeSite) event(body string, secret string) (int, string) {
	site.t.Helper()
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + body))
	request, _ := http.NewRequest(http.MethodPost, site.server.URL+"/account/?action=webhook&provider=stripe", strings.NewReader(body))
	request.Header.Set("Stripe-Signature", "t="+ts+",v1="+hex.EncodeToString(mac.Sum(nil)))
	request.Header.Set("Content-Type", "application/json")
	response, err := site.server.Client().Do(request)
	if err != nil {
		site.t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(raw)
}

func completed(eventID, session, order, licence string) string {
	return `{"id":"` + eventID + `","type":"checkout.session.completed","data":{"object":{"id":"` + session + `","mode":"payment","status":"complete","payment_status":"paid","client_reference_id":"` +
		order + `","metadata":{"license_id":"` + licence + `"}}}}`
}

func (site *storeSite) expires() int64 {
	var at int64
	if err := site.s.DB().QueryRow("SELECT expires FROM licenses WHERE id=?", site.licence.ID).Scan(&at); err != nil {
		site.t.Fatal(err)
	}
	return at
}

// buy presses "Buy a year" and answers the provider's address the member is sent to.
func (site *storeSite) buy() string {
	site.t.Helper()
	status, header, body := site.page(http.MethodPost, "action=buy&lang=en", url.Values{"id": {site.licence.ID}})
	if status != 303 || !strings.HasPrefix(header.Get("Location"), "https://checkout.example/cs_test_") {
		site.t.Fatalf("buy: %d %q %s", status, header.Get("Location"), body)
	}
	return strings.TrimPrefix(header.Get("Location"), "https://checkout.example/")
}

func TestBuyAYearThroughThePages(t *testing.T) {
	site := newStoreSite(t, "test", "test")
	_, _, home := site.page(http.MethodGet, "lang=en", nil)
	if !strings.Contains(home, "Buy a year · 99.00 USD") || !strings.Contains(home, "Test mode: purchases use the payment provider") {
		t.Fatalf("home: %s", home)
	}
	before := site.expires()
	for lang, words := range map[string][]string{
		"en": {"<h1>Buy a year</h1>", "The new year starts after your current end date", "Payments are not refunded.", "a free 30-day trial, to try CelikPanel before paying", "Test mode: no real money is taken.", "99.00 USD"},
		"tr": {"<h1>Bir yıl satın alın</h1>", "Ödemeler iade edilmez.", "30 günlük ücretsiz deneme", "Ödemeye geç"},
	} {
		_, _, page := site.page(http.MethodGet, "action=buy&id="+site.licence.ID+"&lang="+lang, nil)
		for _, w := range words {
			if !strings.Contains(page, w) {
				t.Fatalf("%s: the page before payment lacks %q:\n%s", lang, w, page)
			}
		}
	}
	if orders := must(site.s.Orders(0)); len(orders) != 0 {
		t.Fatal("looking at the page made an order")
	}
	session := site.buy()
	// A double click: the same order and the same checkout (the provider answers once per key).
	if again := site.buy(); again != session || site.stripe.posts != 1 || len(site.stripe.sessions) != 1 {
		t.Fatalf("a double click made another checkout: %s %d", again, len(site.stripe.sessions))
	}
	order := must(site.s.OrderByProviderRef("stripe", session))
	// The member returns before paying: being confirmed, nothing changed.
	_, _, page := site.page(http.MethodGet, "action=order&id="+order.ID+"&lang=en", nil)
	if !strings.Contains(page, "Your payment is being confirmed.") || !strings.Contains(page, "closes by itself within a day") || site.expires() != before {
		t.Fatalf("before paying: %s", page)
	}
	// The member pays; the return page settles it by asking the provider (never from the URL).
	site.stripe.pay(session)
	_, _, page = site.page(http.MethodGet, "action=order&id="+order.ID+"&lang=en&paid=1&amount=1", nil)
	want := anniversary(before)
	if !strings.Contains(page, "Paid.") || !strings.Contains(page, day(want)) || site.expires() != want {
		t.Fatalf("after paying: %s (expires %d, want %d)", page, site.expires(), want)
	}
	// One receipt mail, to the verified address, with nothing secret in it.
	site.mu.Lock()
	mails := append([]sentMail(nil), site.mails...)
	site.mu.Unlock()
	if len(mails) != 1 || mails[0].Email != "buyer@example.com" || !strings.Contains(mails[0].Purpose, order.ID) ||
		!strings.Contains(mails[0].Token, "This receipt is not a tax invoice.") || !strings.Contains(mails[0].Token, "99.00 USD") {
		t.Fatalf("the receipt mail: %+v", mails)
	}
	for _, secret := range []string{"sk_test_", "whsec_", "cs_test_", "pi_"} {
		if strings.Contains(mails[0].Token+mails[0].Purpose, secret) {
			t.Fatalf("the receipt mail carries %q", secret)
		}
	}
	// The webhook that arrives afterwards changes nothing.
	if status, _ := site.event(completed("evt_1", session, order.ID, site.licence.ID), site.webhook); status != 200 || site.expires() != want {
		t.Fatalf("the late webhook: %d", status)
	}
	site.mu.Lock()
	if len(site.mails) != 1 {
		t.Fatalf("a second receipt: %d mails", len(site.mails))
	}
	site.mu.Unlock()
	// The licence now has a paid year: "Renew for a year", from the new end, at the same price.
	_, _, home = site.page(http.MethodGet, "lang=en", nil)
	if strings.Contains(home, ">Buy a year") || !strings.Contains(home, "Renew for a year · 99.00 USD") {
		t.Fatalf("after the first year: %s", home)
	}
	_, _, renew := site.page(http.MethodGet, "action=buy&id="+site.licence.ID+"&lang=en", nil)
	if !strings.Contains(renew, "<h1>Renew for a year</h1>") || !strings.Contains(renew, day(want)+" – "+day(anniversary(want))) ||
		!strings.Contains(renew, "Payments are not refunded.") || !strings.Contains(renew, "Continue to payment") {
		t.Fatalf("the renewal page: %s", renew)
	}
	// The payments list and the receipt (not a tax invoice), in both languages.
	if !strings.Contains(home, "Your payments") || !strings.Contains(home, "paid "+day(order.Created)) || !strings.Contains(home, "action=receipt&amp;id="+order.ID) {
		t.Fatalf("the payments list: %s", home)
	}
	_, _, receipt := site.page(http.MethodGet, "action=receipt&id="+order.ID+"&lang=en", nil)
	for _, w := range []string{"<h1>Receipt</h1>", order.ID, "Paid on (UTC)", "99.00 USD", "First year, " + day(before) + " – " + day(want), "This receipt is not a tax invoice.", "Test mode: no real money was taken."} {
		if !strings.Contains(receipt, w) {
			t.Fatalf("the receipt lacks %q: %s", w, receipt)
		}
	}
	if _, _, tr := site.page(http.MethodGet, "action=receipt&id="+order.ID+"&lang=tr", nil); !strings.Contains(tr, "Bu makbuz bir vergi faturası değildir.") {
		t.Fatalf("the receipt in Turkish: %s", tr)
	}
	// Another member's order is not shown.
	if _, _, body := site.page(http.MethodGet, "action=receipt&id=CPC"+strings.Repeat("0", 44)+"&lang=en", nil); !strings.Contains(body, "Order not found.") {
		t.Fatalf("an unknown receipt: %s", body)
	}
	if _, _, body := site.page(http.MethodGet, "action=order&id=CPC"+strings.Repeat("0", 44)+"&lang=en", nil); !strings.Contains(body, "Order not found.") {
		t.Fatalf("an unknown order: %s", body)
	}
}

func TestTheWebhookRoute(t *testing.T) {
	site := newStoreSite(t, "test", "test")
	session := site.buy()
	order := must(site.s.OrderByProviderRef("stripe", session))
	before := site.expires()
	body := completed("evt_1", session, order.ID, site.licence.ID)
	// Forged, wrong method, unknown or manual provider.
	if status, _ := site.event(body, "whsec_forged"); status != 400 {
		t.Fatalf("forged: %d", status)
	}
	for _, path := range []string{"provider=manual", "provider=nobody"} {
		response, err := site.server.Client().Post(site.server.URL+"/account/?action=webhook&"+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 404 {
			t.Fatalf("%s: %d", path, response.StatusCode)
		}
	}
	response, err := site.server.Client().Get(site.server.URL + "/account/?action=webhook&provider=stripe")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 405 {
		t.Fatalf("GET: %d", response.StatusCode)
	}
	// A "paid" body while the provider says open: nothing granted (the body is only a hint).
	if status, _ := site.event(body, site.webhook); status != 200 || site.expires() != before {
		t.Fatalf("a body that says paid: %d", status)
	}
	// The provider is down: 503 and not recorded, so the provider's retry is processed.
	site.stripe.pay(session)
	site.stripe.mu.Lock()
	site.stripe.down = true
	site.stripe.mu.Unlock()
	if status, _ := site.event(completed("evt_2", session, order.ID, site.licence.ID), site.webhook); status != 503 || site.expires() != before {
		t.Fatalf("provider down: %d", status)
	}
	site.stripe.mu.Lock()
	site.stripe.down = false
	site.stripe.mu.Unlock()
	// Its retry (the same event id) is processed: the year is granted.
	if status, _ := site.event(completed("evt_2", session, order.ID, site.licence.ID), site.webhook); status != 200 || site.expires() != anniversary(before) {
		t.Fatalf("the retry after the provider came back: %d, expires %d", status, site.expires())
	}
	// Two webhooks at once for another paid order: one year, once.
	other := must(site.s.Issue(site.member))
	_, header, _ := site.page(http.MethodPost, "action=buy&lang=en", url.Values{"id": {other.ID}})
	otherSession := strings.TrimPrefix(header.Get("Location"), "https://checkout.example/")
	otherOrder := must(site.s.OrderByProviderRef("stripe", otherSession))
	var otherEnd int64
	if err := site.s.DB().QueryRow("SELECT expires FROM licenses WHERE id=?", other.ID).Scan(&otherEnd); err != nil {
		t.Fatal(err)
	}
	site.stripe.pay(otherSession)
	var wg sync.WaitGroup
	for _, id := range []string{"evt_3", "evt_4"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			site.event(completed(id, otherSession, otherOrder.ID, other.ID), site.webhook)
		}(id)
	}
	wg.Wait()
	var otherAfter int64
	if err := site.s.DB().QueryRow("SELECT expires FROM licenses WHERE id=?", other.ID).Scan(&otherAfter); err != nil {
		t.Fatal(err)
	}
	if otherAfter != anniversary(otherEnd) {
		t.Fatalf("two webhooks at once: %d, want %d", otherAfter, anniversary(otherEnd))
	} // A replay and an event for no order of ours: 200, nothing else.
	if status, _ := site.event(completed("evt_3", session, order.ID, site.licence.ID), site.webhook); status != 200 {
		t.Fatalf("replay: %d", status)
	}
	if status, _ := site.event(completed("evt_9", "cs_test_99", "CPC"+strings.Repeat("e", 44), strings.Repeat("e", 32)), site.webhook); status != 200 {
		t.Fatalf("no order of ours: %d", status)
	}
	// A refund made at the provider (there are none by policy; the operator's exceptional act
	// or a chargeback): the hint is confirmed, the first year revoked.
	site.stripe.mu.Lock()
	site.stripe.sessions[session].refunded = site.stripe.sessions[session].amount
	site.stripe.mu.Unlock()
	refund := `{"id":"evt_r","type":"charge.refunded","data":{"object":{"id":"ch_` + session + `","payment_intent":"pi_` + session + `","amount":9900,"amount_refunded":9900,"refunded":true}}}`
	if status, _ := site.event(refund, site.webhook); status != 200 {
		t.Fatalf("refund hint: %d", status)
	}
	if revoked, why, _ := site.s.LicenceRevocation(site.licence.ID); !revoked || why != "refunded" {
		t.Fatal("the refunded first year was not revoked")
	}
	_, _, home := site.page(http.MethodGet, "lang=en", nil)
	if !strings.Contains(home, "This license's payment was reversed; the license has ended.") {
		t.Fatalf("home after the refund: %s", home)
	}
}

func TestChargingModesOnThePages(t *testing.T) {
	for _, c := range []struct {
		charging, keys string
		buy            bool
		says, refusal  string
	}{
		{"off", "test", false, "CelikPanel is free during the beta. Charging starts with version 1.0.", "There is nothing to buy during the beta"},
		{"", "test", false, "CelikPanel is free during the beta.", "There is nothing to buy during the beta"},
		{"live", "live", false, "CelikPanel is free during the beta.", "Real payments are not enabled in this release."},
		{"test", "live", false, "Test mode", "Payments cannot be taken right now"},
	} {
		site := newStoreSite(t, c.charging, c.keys)
		_, _, home := site.page(http.MethodGet, "lang=en", nil)
		if strings.Contains(home, "Buy a year · ") != c.buy || !strings.Contains(home, c.says) {
			t.Fatalf("%s/%s: home: %s", c.charging, c.keys, home)
		}
		if _, _, body := site.page(http.MethodPost, "action=buy&lang=en", url.Values{"id": {site.licence.ID}}); !strings.Contains(body, c.refusal) {
			t.Fatalf("%s/%s: buy: %s", c.charging, c.keys, body)
		}
		if orders := must(site.s.Orders(0)); len(orders) != 0 || site.stripe.posts != 0 {
			t.Fatalf("%s/%s: an order or a checkout was made", c.charging, c.keys)
		}
	}
	// Test mode on an account that is not a test account.
	site := newStoreSite(t, "test", "test")
	if err := site.s.SetTestAccount("operator:test", "store tests", site.member, false); err != nil {
		t.Fatal(err)
	}
	if _, _, body := site.page(http.MethodPost, "action=buy&lang=tr", url.Values{"id": {site.licence.ID}}); !strings.Contains(body, "Deneme kipinde yalnız deneme hesapları satın alabilir.") {
		t.Fatalf("a real account in test mode: %s", body)
	}
}

func TestPagesWithoutTheStoreAreUnchanged(t *testing.T) {
	site := newStoreSite(t, "test", "test")
	_, _, with := site.page(http.MethodGet, "lang=en", nil)
	if !strings.Contains(with, "Buy a year") {
		t.Fatal("no store parts with the store")
	}
	// A file without the store: none of the store's words appear.
	plain := signIn(t)
	_, home := plain.page(t, http.MethodGet, "lang=en", nil)
	for _, word := range []string{"Buy a year", "free during the beta", "Test mode"} {
		if strings.Contains(home, word) {
			t.Fatalf("%q on a page without the store", word)
		}
	}
}

func TestReconciliation(t *testing.T) {
	site := newStoreSite(t, "test", "test")
	config := must(LoadConfig(site.dir))
	registry := site.handler.Payments
	clock := time.Now().Unix()
	site.s.clock = func() int64 { return clock }
	session := site.buy()
	order := must(site.s.OrderByProviderRef("stripe", session))
	before := site.expires()
	// Younger than ten minutes: left alone.
	report := must(site.s.Reconcile(t.Context(), registry, config, "reconcile:test"))
	if report.Confirmed != 0 {
		t.Fatalf("a young order was asked about: %+v", report)
	}
	// Paid while the webhook was lost: reconciliation fulfils it.
	site.stripe.pay(session)
	clock += 601
	report = must(site.s.Reconcile(t.Context(), registry, config, "reconcile:test"))
	if report.Paid != 1 || site.expires() != anniversary(before) || must(site.s.GetOrder(order.ID)).State != OrderPaid {
		t.Fatalf("a lost webhook: %+v", report)
	}
	// An unpaid order past its day closes; a mismatch is listed and never fulfilled.
	l2 := must(site.s.Issue(site.member))
	o2 := must(site.s.CreateOrder(site.member, l2.ID, ChargingTest))
	if err := site.s.AttachCheckout(o2.ID, "stripe", "cs_none", "https://checkout.example/cs_none"); err != nil {
		t.Fatal(err)
	}
	clock += OrderLifetime + 1
	site.stripe.mu.Lock()
	site.stripe.sessions["cs_none"] = &fakeSession{id: "cs_none", order: o2.ID, licence: l2.ID, amount: 9900, status: "expired"}
	site.stripe.mu.Unlock()
	l3 := must(site.s.Issue(site.member))
	o3 := must(site.s.CreateOrder(site.member, l3.ID, ChargingTest))
	if err := site.s.AttachCheckout(o3.ID, "stripe", "cs_wrong", "https://checkout.example/cs_wrong"); err != nil {
		t.Fatal(err)
	}
	site.stripe.mu.Lock()
	site.stripe.sessions["cs_wrong"] = &fakeSession{id: "cs_wrong", order: o3.ID, licence: l3.ID, amount: 100, status: "complete", paid: true}
	site.stripe.mu.Unlock()
	clock += 601
	report = must(site.s.Reconcile(t.Context(), registry, config, "reconcile:test"))
	if report.Expired != 1 || len(report.Attention) != 1 || report.Attention[0].ID != o3.ID {
		t.Fatalf("expiry and mismatch: %+v", report)
	}
	if must(site.s.GetOrder(o2.ID)).State != OrderExpired {
		t.Fatal("the unpaid order did not close")
	}
}

// The operator's exceptional refund (there are no refunds by policy): a reason is required, the
// provider is asked with confirm-first, and the confirmed refund revokes the first year.
func TestOperatorRefund(t *testing.T) {
	site := newStoreSite(t, "test", "test")
	session := site.buy()
	site.stripe.pay(session)
	order := must(site.s.OrderByProviderRef("stripe", session))
	config := must(LoadConfig(site.dir))
	if _, _, page := site.page(http.MethodGet, "action=order&id="+order.ID+"&lang=en", nil); !strings.Contains(page, "Paid.") {
		t.Fatal("not paid")
	}
	if _, _, err := site.s.OperatorRefund(t.Context(), site.handler.Payments, config, "operator:test", order.ID, ""); err != ErrInvalidRequest {
		t.Fatalf("without a reason: %v", err)
	}
	r, o, err := site.s.OperatorRefund(t.Context(), site.handler.Payments, config, "operator:test", order.ID, "an exceptional act for a test")
	if err != nil || r.State != "done" || o.State != OrderRefunded || site.stripe.refunds != 1 {
		t.Fatalf("the refund: %+v %+v %v (%d refund calls)", r, o, err, site.stripe.refunds)
	}
	if revoked, _, _ := site.s.LicenceRevocation(site.licence.ID); !revoked {
		t.Fatal("the refunded first year was not revoked")
	}
	if _, _, err := site.s.OperatorRefund(t.Context(), site.handler.Payments, config, "operator:test", order.ID, "again"); err != ErrRefundNotPossible || site.stripe.refunds != 1 {
		t.Fatalf("a second refund: %v", err)
	}
	// Nothing on the member's pages offers a refund.
	_, _, home := site.page(http.MethodGet, "lang=en", nil)
	_, _, receipt := site.page(http.MethodGet, "action=receipt&id="+order.ID+"&lang=en", nil)
	for _, page := range []string{home, receipt} {
		if strings.Contains(strings.ToLower(page), "refund") && !strings.Contains(page, "Payments are not refunded") {
			t.Fatalf("a page speaks of refunds: %s", page)
		}
	}
	if !strings.Contains(receipt, "Payment reversed") {
		t.Fatalf("the receipt of a reversed payment: %s", receipt)
	}
}
