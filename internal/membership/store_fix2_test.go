package membership

// The S2 second fix round (REVIEW-S2-FIXES.md, S2-FIX-ROUND2.md): each test fails without its fix.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/payment"
)

// A payment that could not be granted is honoured by the operator's grant -- after asking the
// provider (F2); test money of an unflagged account is never granted; a payment given back at
// the provider is settled, not granted; the attention entry closes.
func TestTheOperatorGrantsAPaymentThatCouldNotBeGranted(t *testing.T) {
	site := newStoreSite(t, "test", "test")
	config := must(LoadConfig(site.dir))
	session := site.buy()
	order := must(site.s.OrderByProviderRef("stripe", session))
	if err := site.s.SetTestAccount("operator:test", "unflag", site.member, false); err != nil {
		t.Fatal(err)
	}
	site.stripe.pay(session)
	clock := order.Created + 700
	site.s.clock = func() int64 { return clock }
	must(site.s.Reconcile(t.Context(), site.handler.Payments, config, "reconcile:test"))
	if o := must(site.s.GetOrder(order.ID)); o.State != OrderMismatch || len(must(site.s.AttentionOrders())) != 1 {
		t.Fatalf("not granted: %+v", o)
	}
	if _, err := site.s.GrantOrder(t.Context(), site.handler.Payments, config, "operator:test", order.ID, "honour it"); err == nil || !strings.Contains(err.Error(), "no longer a test account") {
		t.Fatalf("test money for an unflagged account: %v", err)
	}
	if err := site.s.SetTestAccount("operator:test", "flag again", site.member, true); err != nil {
		t.Fatal(err)
	}
	if _, err := site.s.GrantOrder(t.Context(), site.handler.Payments, config, "operator:test", order.ID, ""); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("a grant without a reason: %v", err)
	}
	before := site.expires()
	g, err := site.s.GrantOrder(t.Context(), site.handler.Payments, config, "operator:test", order.ID, "the member paid; honour it")
	if err != nil || g.State != OrderPaid || site.expires() != anniversary(before) || len(must(site.s.AttentionOrders())) != 0 || !g.ReceiptMine {
		t.Fatalf("the operator's grant: %+v %v", g, err)
	}
	// A payment given back at the provider (its webhook lost): settled, never granted.
	l2 := must(site.s.Issue(site.member))
	site.licence = l2
	session2 := site.buy()
	order2 := must(site.s.OrderByProviderRef("stripe", session2))
	if err := site.s.SetTestAccount("operator:test", "unflag", site.member, false); err != nil && !errors.Is(err, ErrTestTimeRemains) {
		t.Fatal(err)
	}
	if _, err := site.s.DB().Exec("INSERT INTO store_attention(order_id,kind,detail,state,opened) VALUES(?,?,?,'open',1)", order2.ID, AttentionNotGranted, NotGrantedRemoved); err != nil {
		t.Fatal(err)
	}
	site.stripe.pay(session2)
	o2 := must(site.s.GetOrder(order2.ID))
	fact := payment.Fact{Outcome: payment.OutcomePaid, PaymentID: "pi_" + session2, Order: o2.ID, LicenseID: o2.LicenseID, PaidMinor: o2.AmountMinor, Currency: "USD"}
	if _, err := site.s.DB().Exec("UPDATE store_orders SET state='mismatch' WHERE id=?", o2.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := site.s.DB().Exec("INSERT INTO store_payments(provider,payment_id,order_id,outcome,paid_minor,refunded_minor,currency,live,disputed,confirmed,updated) VALUES('stripe',?,?,'paid',?,0,'USD',0,0,?,?)",
		fact.PaymentID, o2.ID, o2.AmountMinor, clock, clock); err != nil {
		t.Fatal(err)
	}
	site.stripe.mu.Lock()
	site.stripe.sessions[session2].refunded = site.stripe.sessions[session2].amount
	site.stripe.mu.Unlock()
	if _, err := site.s.GrantOrder(t.Context(), site.handler.Payments, config, "operator:test", o2.ID, "try"); err == nil || !strings.Contains(err.Error(), "no longer shows this payment as paid") {
		t.Fatalf("a refunded payment was granted: %v", err)
	}
	if _, _, _, settled, _ := site.s.Cause(o2.ID); !settled {
		t.Fatal("the refunded payment was not settled")
	}
}

// A lost dispute takes back exactly its payment's time; later paid years stay. When nothing
// remains, the licence ends, and reinstate undoes it.
func TestALostDisputeTakesBackOnlyItsOwnTime(t *testing.T) {
	f := newStore(t)
	s := f.s
	f.h.now = f.expires(f.licence.ID) + 86400 // the free year ended
	first := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	first = must(s.ApplyFact(first.ID, "stripe", "provider:stripe", paid(first, "pi_1")))
	second := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	second = must(s.ApplyFact(second.ID, "stripe", "provider:stripe", paid(second, "pi_2")))
	end := f.expires(f.licence.ID)
	lost := paid(first, "pi_1")
	lost.Outcome, lost.Disputed, lost.DisputeStatus = payment.OutcomeReversed, true, "lost"
	must(s.ApplyFact(first.ID, "stripe", "provider:stripe", lost))
	if revoked, _, _ := s.LicenceRevocation(f.licence.ID); revoked || f.expires(f.licence.ID) != end-(first.GrantedTo-first.GrantedFrom) {
		t.Fatalf("a lost dispute on year 1 ended the later year: expires %d", f.expires(f.licence.ID))
	}
	// The second year lost too: nothing remains; the licence ends.
	lost2 := paid(second, "pi_2")
	lost2.Outcome, lost2.Disputed, lost2.DisputeStatus = payment.OutcomeReversed, true, "lost"
	f.h.now += 86400
	must(s.ApplyFact(second.ID, "stripe", "provider:stripe", lost2))
	if revoked, _, _ := s.LicenceRevocation(f.licence.ID); !revoked {
		t.Fatal("nothing remained, yet the licence runs")
	}
	// Reinstate (a revocation by mistake): the end it had comes back, the generation is raised.
	var generation int64
	if err := s.DB().QueryRow("SELECT generation FROM licenses WHERE id=?", f.licence.ID).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	expiresBefore := int64(0)
	if err := s.DB().QueryRow("SELECT expires_before FROM store_revocations WHERE license_id=?", f.licence.ID).Scan(&expiresBefore); err != nil {
		t.Fatal(err)
	}
	if err := s.Reinstate("operator:test", f.licence.ID, "accepted by mistake"); err != nil {
		t.Fatal(err)
	}
	var after int64
	if err := s.DB().QueryRow("SELECT generation FROM licenses WHERE id=?", f.licence.ID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	var takenBack int64
	if err := s.DB().QueryRow("SELECT taken_back FROM store_revocations WHERE license_id=?", f.licence.ID).Scan(&takenBack); err != nil {
		t.Fatal(err)
	}
	want := expiresBefore - takenBack
	if want < f.h.now {
		want = f.h.now
	}
	if revoked, _, _ := s.LicenceRevocation(f.licence.ID); revoked || f.expires(f.licence.ID) != want || after != generation+1 {
		t.Fatalf("reinstate: generation %d -> %d, expires %d want %d (no time given back)", generation, after, f.expires(f.licence.ID), want)
	}
	if err := s.Reinstate("operator:test", f.licence.ID, "again"); err == nil {
		t.Fatal("a second reinstate of the same revocation")
	}
}

// Disputes are re-checked until they close, however old the payment; a failing order never
// stops reconciliation; the cap leaves the rest for the next run and says so; a receipt that
// could not be mailed stays owed.
func TestReconciliationDisputesFailuresAndOwedReceipts(t *testing.T) {
	site := newStoreSite(t, "test", "test")
	config := must(LoadConfig(site.dir))
	session := site.buy()
	site.stripe.pay(session)
	order := must(site.s.OrderByProviderRef("stripe", session))
	clock := order.Created + 700
	site.s.clock = func() int64 { return clock }
	must(site.s.Reconcile(t.Context(), site.handler.Payments, config, "reconcile:test"))
	// 200 days later a dispute opens: still asked about every day until it closes.
	site.stripe.mu.Lock()
	site.stripe.sessions[session].disputed, site.stripe.sessions[session].dispute = true, "needs_response"
	site.stripe.mu.Unlock()
	clock += 200 * 86400
	if r := must(site.s.Reconcile(t.Context(), site.handler.Payments, config, "reconcile:test")); r.Confirmed != 0 {
		t.Fatal("an old payment without a recorded dispute was asked about") // the webhook brings the dispute
	}
	if _, err := site.s.DB().Exec("UPDATE store_payments SET disputed=1,dispute_status='needs_response'"); err != nil {
		t.Fatal(err)
	}
	if r := must(site.s.Reconcile(t.Context(), site.handler.Payments, config, "reconcile:test")); r.Confirmed != 1 {
		t.Fatalf("an open dispute after 200 days was not asked about: %+v", r)
	}
	clock += 86400
	site.stripe.mu.Lock()
	site.stripe.sessions[session].dispute = "won"
	site.stripe.mu.Unlock()
	if r := must(site.s.Reconcile(t.Context(), site.handler.Payments, config, "reconcile:test")); r.Confirmed != 1 || len(r.Attention) != 0 {
		t.Fatalf("a won dispute: %+v", r)
	}
	clock += 86400
	if r := must(site.s.Reconcile(t.Context(), site.handler.Payments, config, "reconcile:test")); r.Confirmed != 0 {
		t.Fatal("a closed dispute is still asked about")
	}
	// An order whose provider object cannot be confirmed: listed, the run goes on.
	l2 := must(site.s.Issue(site.member))
	o2 := must(site.s.CreateOrder(site.member, l2.ID, ChargingTest))
	if err := site.s.AttachCheckout(o2.ID, "stripe", "cs_gone"); err != nil {
		t.Fatal(err)
	}
	clock += 700
	r := must(site.s.Reconcile(t.Context(), site.handler.Payments, config, "reconcile:test"))
	if len(r.Failed) != 1 || !strings.HasPrefix(r.Failed[0], o2.ID) || r.Unreachable != 0 || len(r.Attention) != 1 || r.Attention[0].Kind != AttentionUnconfirmed {
		t.Fatalf("a failing order: %+v", r)
	}
	// A receipt whose mail failed stays owed and is listed for the next run.
	if _, err := site.s.DB().Exec("UPDATE store_orders SET receipt_sent=NULL WHERE id=?", order.ID); err != nil {
		t.Fatal(err)
	}
	if r := must(site.s.Reconcile(t.Context(), site.handler.Payments, config, "reconcile:test")); len(r.ReceiptsOwed) != 1 || r.ReceiptsOwed[0].ID != order.ID {
		t.Fatalf("an owed receipt: %+v", r.ReceiptsOwed)
	}
}

// The webhook's answer is complete (its length set) before any mail is tried.
func TestTheWebhookAnswerIsCompleteBeforeTheMail(t *testing.T) {
	site := newStoreSite(t, "test", "test")
	session := site.buy()
	order := must(site.s.OrderByProviderRef("stripe", session))
	site.stripe.pay(session)
	mailed := make(chan struct{})
	release := make(chan struct{})
	site.handler.sendMail = func(to, subject, body string) error {
		close(mailed)
		<-release // the mail is slow
		return nil
	}
	body := completed("evt_1", session, order.ID, site.licence.ID)
	ts, signature := signFor(site.webhook, body)
	request, _ := http.NewRequest(http.MethodPost, site.server.URL+"/account/?action=webhook&provider=stripe", strings.NewReader(body))
	request.Header.Set("Stripe-Signature", "t="+ts+",v1="+signature)
	response, err := site.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(response.Body) // reads to the end only if the answer is complete
	response.Body.Close()
	close(release)
	<-mailed
	if err != nil || response.StatusCode != 200 || string(raw) != `{"received":true}` || response.ContentLength != int64(len(raw)) {
		t.Fatalf("the answer: %d %q %v (length %d)", response.StatusCode, raw, err, response.ContentLength)
	}
}

// What a member reads: the trial line goes once a year is bought; a licence awaiting payment
// says so also when payments cannot be taken; "payable until" names the time; the mismatch
// text is true for its cause and after settlement.
func TestMemberTextsPerState(t *testing.T) {
	site := buildStoreSite(t, "test", "test", false)
	site.page(http.MethodPost, "action=issue&lang=en", nil)
	trialID, _, _ := must3(site.s.Trial(site.member, ChargingTest))
	site.licence = Issued{ID: trialID}
	session := site.buy()
	site.stripe.pay(session)
	order := must(site.s.OrderByProviderRef("stripe", session))
	_, _, orderPage := site.page(http.MethodGet, "action=order&id="+order.ID+"&lang=en", nil)
	if !strings.Contains(orderPage, "Paid.") {
		t.Fatalf("not paid: %s", orderPage)
	}
	_, _, home := site.page(http.MethodGet, "lang=en", nil)
	if strings.Contains(home, "Free trial until") || !strings.Contains(home, "Renew for a year") {
		t.Fatalf("the card after the year was bought: %s", home)
	}
	// Payments cannot be taken (live keys in test mode): a licence awaiting payment still says so.
	site.s.clock = func() int64 { return time.Now().Unix() + 30 } // not the same click
	waiting, _, err := site.s.IssueCharged(site.member, ChargingTest)
	if err != nil || waiting.ID == "" {
		t.Fatalf("a licence awaiting payment: %v", err)
	}
	site.setConfig(map[string]any{"payment": map[string]string{"provider": "stripe", "mode": "live", "secret_key": "sk_live_" + randomHex(12), "webhook_secret": site.webhook}})
	_, _, home = site.page(http.MethodGet, "lang=en", nil)
	if !strings.Contains(home, "Awaiting payment: this license can be used once its first year is paid.") || strings.Contains(home, "Buy a year · ") {
		t.Fatalf("awaiting payment without a provider: %s", home)
	}
	// The mismatch texts: a removed licence, then settled at the provider.
	p := &page{lang: "en", store: &storePage{causeKind: AttentionNotGranted, causeDetail: NotGrantedRemoved, causeOpen: true}}
	if state, next := p.mismatchText(); !strings.Contains(state, "the license it was for no longer exists") || !strings.Contains(next, "We will write to you") {
		t.Fatalf("removed: %s", state)
	}
	p.store.causeOpen = false // the operator closed the matter: no promise of a mail
	if _, next := p.mismatchText(); strings.Contains(next, "We will write to you") {
		t.Fatalf("a closed matter still promises a mail: %s", next)
	}
	p.store.settled = true
	if state, next := p.mismatchText(); state != "The payment went back to you." || next != "Nothing changed on your license." {
		t.Fatalf("settled: %s %s", state, next)
	}
	if !strings.Contains(moment(1700000000), "2023-11-14 22:13 UTC") {
		t.Fatalf("the time is not shown: %s", moment(1700000000))
	}
}

// price -from today is now; a past day is refused.
func TestAPriceFromToday(t *testing.T) {
	f := newStore(t)
	today := f.h.now - f.h.now%86400
	if err := f.s.SetPrice("operator:test", "from today", "USD", 12900, today); err != nil {
		t.Fatalf("a price from today: %v", err)
	}
	if p := must(f.s.CurrentPrice("USD")); p.AmountMinor != 12900 || p.ValidFrom != f.h.now {
		t.Fatalf("today's price: %+v", p)
	}
	if err := f.s.SetPrice("operator:test", "default now", "USD", 13900, 0); err != nil {
		t.Fatalf("now: %v", err)
	}
	if err := f.s.SetPrice("operator:test", "yesterday", "USD", 12900, today-86400); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("yesterday: %v", err)
	}
}

// A file made by the earlier S2 commits (store version 1) is brought to version 2.
func TestTheStoreMigratesFromVersion1(t *testing.T) {
	h := newHarness(t)
	s := h.s
	for _, statement := range []string{
		"CREATE TABLE store_meta (\n key TEXT PRIMARY KEY, value TEXT NOT NULL\n)",
		"INSERT INTO store_meta(key,value) VALUES('version','1')",
		"CREATE TABLE store_orders (\n id TEXT PRIMARY KEY, member_id INTEGER NOT NULL, license_id TEXT NOT NULL, kind TEXT NOT NULL, amount_minor INTEGER NOT NULL, currency TEXT NOT NULL, live INTEGER NOT NULL,\n state TEXT NOT NULL, provider TEXT, provider_ref TEXT, granted_from INTEGER, granted_to INTEGER, previous_expires INTEGER,\n created INTEGER NOT NULL, expires INTEGER NOT NULL, updated INTEGER NOT NULL\n)",
		"CREATE TABLE store_payments (\n provider TEXT NOT NULL, payment_id TEXT NOT NULL, order_id TEXT NOT NULL, outcome TEXT NOT NULL, paid_minor INTEGER NOT NULL, refunded_minor INTEGER NOT NULL, currency TEXT NOT NULL,\n live INTEGER NOT NULL, disputed INTEGER NOT NULL, confirmed INTEGER NOT NULL, updated INTEGER NOT NULL, PRIMARY KEY(provider, payment_id)\n)",
		"INSERT INTO members(email,password,created) VALUES('v1@example.com','x',1)",
		"INSERT INTO licenses(id,member_id,key_hash,key_suffix,created,expires) VALUES('aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',1,'h','s',1,2)",
		"CREATE TABLE store_trials (\n member_id INTEGER PRIMARY KEY, license_id TEXT NOT NULL, started INTEGER NOT NULL, ends INTEGER NOT NULL\n)",
		"INSERT INTO store_trials VALUES(1,'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',1,2)",
	} {
		if _, err := s.DB().Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	if s.StoreReady() {
		t.Fatal("a version 1 store is read as ready")
	}
	if err := s.MigrateStore("operator:test"); err != nil {
		t.Fatal(err)
	}
	if !s.StoreReady() {
		t.Fatal("not version 2 after the migration")
	}
	var mode string
	if err := s.DB().QueryRow("SELECT mode FROM store_trials WHERE member_id=1").Scan(&mode); err != nil || mode != "test" {
		t.Fatalf("the version 1 trial: %q %v", mode, err)
	}
	for table, column := range map[string]string{"store_orders": "receipt_owed", "store_payments": "dispute_status"} {
		columns := must(tableColumns(t.Context(), s.DB(), table))
		if !columns[column] {
			t.Fatalf("%s.%s missing", table, column)
		}
	}
}

func signFor(secret, body string) (string, string) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + body))
	return ts, hex.EncodeToString(mac.Sum(nil))
}

// An operator's gift after a paid year has ended does not revive the old locked price.
func TestAGiftAfterTheEndDoesNotReviveTheLock(t *testing.T) {
	f := newStore(t)
	s := f.s
	f.h.now = f.expires(f.licence.ID) + 86400
	o := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	must(s.ApplyFact(o.ID, "stripe", "provider:stripe", paid(o, "pi_1")))
	if err := s.SetPrice("operator:test", "rise", "USD", 12900, 0); err != nil {
		t.Fatal(err)
	}
	if q := must(s.QuoteFor(f.member, f.licence.ID)); !q.Locked || q.AmountMinor != 9900 {
		t.Fatalf("inside the paid year: %+v", q)
	}
	f.h.now = f.expires(f.licence.ID) + 86400 // the paid year ended
	if err := s.ExtendByHand("operator:test", "a gift after the end", f.licence.ID, 30); err != nil {
		t.Fatal(err)
	}
	if q := must(s.QuoteFor(f.member, f.licence.ID)); q.Locked || q.AmountMinor != 12900 {
		t.Fatalf("a gift after the end revived the lock: %+v", q)
	}
}
