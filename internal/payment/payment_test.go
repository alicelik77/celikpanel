// Source: bioexamine, backend/internal/payment/payment_test.go, read at commit 181fccf6
// (last changed there in 602dbeeb, 2026-11-04); copied into CelikPanel on 2026-11-23 under
// the shared catalogue's entry A (same language: copy with a source note). CelikPanel's
// changes after the copy are marked "CelikPanel:" and are in this repository's history.

package payment

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestManualGoesThroughTheProviderInterface(t *testing.T) {
	var p Provider = Manual{}
	registry := DefaultRegistry()
	if got, ok := registry.Get(ManualCode); !ok || got.Code() != ManualCode {
		t.Fatalf("the default registry must hold the manual provider: %v %v", got, ok)
	}
	// 000298: the integrated providers are in the product's registry, each with key fields.
	// CelikPanel: Stripe only (iyzico and PayTR were not copied).
	if codes := registry.Codes(); len(codes) != 2 || codes[0] != ManualCode || codes[1] != StripeCode {
		t.Fatalf("the default registry: %v", codes)
	}
	for _, code := range []string{StripeCode} {
		provider, _ := registry.Get(code)
		configurable, ok := provider.(Configurable)
		if !ok || configurable.ConfigSchema().Provider != code || configurable.ConfigSchema().Last4Field == "" {
			t.Fatalf("%s has no key fields", code)
		}
	}
	if _, ok := Provider(Manual{}).(Configurable); ok {
		t.Fatal("the manual provider has no key")
	}
	start, err := p.StartCheckout(context.Background(), "", Checkout{Period: PeriodYearly})
	if err != nil || start.Instructions != InstructionOperatorRecords {
		t.Fatalf("manual checkout: %+v %v", start, err)
	}
	if _, err := p.StartCheckout(context.Background(), "", Checkout{Period: "monthly"}); !errors.Is(err, ErrPeriodNotYearly) {
		t.Fatalf("a monthly checkout must be refused: %v", err)
	}
}

// CelikPanel: the entry names a licence (license_id) instead of a membership; review of #361, 9:
// the status must be stated (never read as paid by default) and the licence id must be one.
func TestManualEntryIsChecked(t *testing.T) {
	l := `"license_id":"` + licenseID + `"`
	ok := `{` + l + `,"reference":"dekont-1","status":"succeeded","amount":"99.00","currency":"USD"}`
	fact, err := Manual{}.ParseEntry(Notification{Body: []byte(ok)})
	if err != nil || fact.Outcome != OutcomePaid || fact.PaidMinor != 9900 || fact.LicenseID != licenseID || fact.Currency != "USD" {
		t.Fatalf("a stated payment is a paid year: %+v %v", fact, err)
	}
	if event, err := (Manual{}).ParseNotification(context.Background(), "", Notification{Body: []byte(ok)}); err != nil ||
		event.Provider != ManualCode || event.LicenseID != licenseID || event.Reference != "dekont-1" {
		t.Fatalf("the entry's identifiers: %+v %v", event, err)
	}
	for body, want := range map[string]error{
		`{` + l + `,"reference":"r","status":"succeeded","amount":"1","currency":"TRY","period":"monthly"}`:                      ErrPeriodNotYearly,
		`{` + l + `,"reference":"r","amount":"1","currency":"TRY"}`:                                                              ErrInvalidNotification, // no status
		`{` + l + `,"reference":"r","status":"","amount":"1","currency":"TRY"}`:                                                  ErrInvalidNotification,
		`{"license_id":"m","reference":"r","status":"succeeded","amount":"1","currency":"TRY"}`:                                  ErrInvalidNotification,
		`{"license_id":"` + strings.ToUpper(licenseID) + `","reference":"r","status":"succeeded","amount":"1","currency":"TRY"}`: ErrInvalidNotification,
		`{` + l + `,"reference":"","status":"succeeded","amount":"1","currency":"TRY"}`:                                          ErrInvalidNotification,
		`{` + l + `,"reference":"r","status":"succeeded","amount":"-1","currency":"TRY"}`:                                        ErrInvalidNotification,
		`{` + l + `,"reference":"r","status":"succeeded","amount":"1","currency":"try"}`:                                         ErrInvalidNotification,
		`{` + l + `,"reference":"r","status":"maybe","amount":"1","currency":"TRY"}`:                                             ErrInvalidNotification,
		`{` + l + `,"reference":"r","status":"succeeded","amount":"1","currency":"TRY","extra":1}`:                               ErrInvalidNotification,
	} {
		if _, err := (Manual{}).ParseEntry(Notification{Body: []byte(body)}); !errors.Is(err, want) {
			t.Errorf("%s: got %v, want %v", body, err, want)
		}
	}
}
