// Source: bioexamine, backend/internal/payment/config.go, read at commit 181fccf6
// (last changed there in 898b3288, 2026-11-04); copied into CelikPanel on 2026-11-23 under
// the shared catalogue's entry A (same language: copy with a source note). CelikPanel's
// changes after the copy are marked "CelikPanel:" and are in this repository's history.

package payment

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// The operator's key fields.
//
// A provider's credentials are one JSON object in bx_billing.payment_providers.credentials. Each
// provider says which fields it has (ConfigSchema), so the operator screen draws them without
// knowing any provider: a secret field is a password input that is never filled back in, a select
// offers its options. Mode is always present: "test" until the founder switches a provider to
// "live" with live keys.

// Field kinds the screen draws.
const (
	FieldText   = "text"
	FieldSecret = "secret"
	FieldSelect = "select"
)

// Modes. Every provider has both.
const (
	ModeTest = "test"
	ModeLive = "live"
)

// ConfigField is one credential field.
type ConfigField struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Kind     string   `json:"kind"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
	// Hint is a sentence for the operator: where the value is found in the provider's panel.
	Hint string `json:"hint,omitempty"`
}

// ConfigSchema is what a provider needs and offers.
type ConfigSchema struct {
	Provider string        `json:"provider"`
	Fields   []ConfigField `json:"fields"`
	// Last4Field is the secret whose last four characters the screen shows.
	Last4Field string `json:"last4_field"`
	// DocsURL is the provider's official documentation the module follows.
	DocsURL string `json:"docs_url"`
	// Webhook says whether the provider calls the public door, and BrowserReturn whether it sends
	// the payer's browser back with a result to confirm.
	Webhook       bool `json:"webhook"`
	BrowserReturn bool `json:"browser_return"`
	// Renewal says how a year renews: "provider_subscription", "stored_card".
	Renewal string `json:"renewal"`
	// Refund and CancelAtProvider: what the operator's and the payer's buttons reach.
	Refund           bool `json:"refund"`
	CancelAtProvider bool `json:"cancel_at_provider"`
	// Currencies the provider takes, in our codes.
	Currencies []string `json:"currencies"`
}

// Configurable is a provider with key fields (every provider but Manual).
type Configurable interface {
	ConfigSchema() ConfigSchema
}

// Config is a parsed credentials object.
type Config map[string]string

// Mode is "test" or "live" ("test" when unset: a provider never charges for real by default).
func (c Config) Mode() string {
	if c["mode"] == ModeLive {
		return ModeLive
	}
	return ModeTest
}

// ParseConfig reads stored credentials. Empty credentials are ErrNotConfigured; anything that is
// not a JSON object of strings is too (never echoing what was stored).
func ParseConfig(credentials Credentials) (Config, error) {
	raw := strings.TrimSpace(string(credentials))
	if raw == "" {
		return nil, ErrNotConfigured
	}
	var values map[string]any
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, ErrNotConfigured
	}
	config := Config{}
	for name, value := range values {
		switch v := value.(type) {
		case string:
			config[name] = strings.TrimSpace(v)
		case bool:
			config[name] = strconv.FormatBool(v)
		default:
			return nil, ErrNotConfigured
		}
	}
	return config, nil
}

// requireConfig parses credentials and checks every required field of the schema is present.
func requireConfig(credentials Credentials, schema ConfigSchema) (Config, error) {
	config, err := ParseConfig(credentials)
	if err != nil {
		return nil, err
	}
	for _, field := range schema.Fields {
		if field.Required && config[field.Name] == "" {
			return nil, ErrNotConfigured
		}
	}
	return config, nil
}

// Why a credential field was refused (CredentialError.Code); the operator screen says each.
const (
	CredentialUnknownField          = "unknown_field"
	CredentialNotUsable             = "not_usable"
	CredentialNotAnOption           = "not_an_option"
	CredentialRequired              = "required"
	CredentialTooShort              = "too_short"
	CredentialLiveKeyRequired       = "live_key_required"
	CredentialTestKeyRequired       = "test_key_required"
	CredentialSigningSecretRequired = "signing_secret_required"
)

// CredentialError says which field of an operator's entry is wrong, by name only -- never its
// value. Code is one of the Credential* reasons.
type CredentialError struct {
	Field  string
	Code   string
	Reason string
}

func (e *CredentialError) Error() string {
	return "payment credentials: " + e.Field + ": " + e.Reason
}

// PrepareCredentials checks an operator's entry against the provider's schema and merges it over
// what is stored (a secret field left empty keeps its stored value, so the screen never has to
// hold a secret to change the mode). It answers the JSON to store and the last four characters of
// the main secret. Unknown fields are refused; values are never part of an error.
func PrepareCredentials(schema ConfigSchema, entered map[string]string, stored Credentials, check func(Config) error) (string, string, error) {
	known := map[string]ConfigField{}
	for _, field := range schema.Fields {
		known[field.Name] = field
	}
	merged := Config{}
	if previous, err := ParseConfig(stored); err == nil {
		for name, value := range previous {
			if _, ok := known[name]; ok {
				merged[name] = value
			}
		}
	}
	names := make([]string, 0, len(entered))
	for name := range entered {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		field, ok := known[name]
		if !ok {
			return "", "", &CredentialError{Field: name, Code: CredentialUnknownField, Reason: "no such field"}
		}
		value := strings.TrimSpace(entered[name])
		if value == "" {
			if field.Kind == FieldSecret {
				continue // keep the stored secret
			}
			delete(merged, name)
			continue
		}
		if len(value) > 2000 || !utf8.ValidString(value) || strings.ContainsAny(value, "\r\n\t") {
			return "", "", &CredentialError{Field: name, Code: CredentialNotUsable, Reason: "not a usable value"}
		}
		if field.Kind == FieldSelect && !contains(field.Options, value) {
			return "", "", &CredentialError{Field: name, Code: CredentialNotAnOption, Reason: "not one of the options"}
		}
		merged[name] = value
	}
	for _, field := range schema.Fields {
		if field.Required && merged[field.Name] == "" {
			return "", "", &CredentialError{Field: field.Name, Code: CredentialRequired, Reason: "required"}
		}
	}
	if check != nil {
		if err := check(merged); err != nil {
			return "", "", err
		}
	}
	secret := merged[schema.Last4Field]
	if len(secret) < 4 {
		return "", "", &CredentialError{Field: schema.Last4Field, Code: CredentialTooShort, Reason: "too short"}
	}
	encoded, err := json.Marshal(merged)
	if err != nil {
		return "", "", err
	}
	return string(encoded), secret[len(secret)-4:], nil
}

// PublicValues are the non-secret fields of stored credentials, for the operator screen.
func PublicValues(schema ConfigSchema, credentials Credentials) map[string]string {
	out := map[string]string{}
	config, err := ParseConfig(credentials)
	if err != nil {
		return out
	}
	for _, field := range schema.Fields {
		if field.Kind != FieldSecret && config[field.Name] != "" {
			out[field.Name] = config[field.Name]
		}
	}
	return out
}

// Complete reports whether stored credentials have every required field.
func Complete(schema ConfigSchema, credentials Credentials) bool {
	_, err := requireConfig(credentials, schema)
	return err == nil
}

func modeField() ConfigField {
	return ConfigField{Name: "mode", Label: "Çalışma kipi", Kind: FieldSelect, Required: true,
		Options: []string{ModeTest, ModeLive}, Hint: "Önce test anahtarlarıyla 'test'; canlı anahtarlar girildiğinde 'live'."}
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Amounts. Our amounts are decimal strings with at most two decimals (numeric(12,2)); providers
// take minor units (Stripe, PayTR iFrame) or decimals (iyzico, PayTR Direct and refund).

// zeroDecimal are the currencies whose minor unit is the unit itself, per Stripe's list
// (https://docs.stripe.com/currencies#zero-decimal). None is a region's currency today; the list
// is here so a future region in one of them is not charged a hundred times over.
var zeroDecimal = map[string]bool{
	"BIF": true, "CLP": true, "DJF": true, "GNF": true, "JPY": true, "KMF": true, "KRW": true,
	"MGA": true, "PYG": true, "RWF": true, "UGX": true, "VND": true, "VUV": true, "XAF": true,
	"XOF": true, "XPF": true,
}

// threeDecimal currencies cannot be held by numeric(12,2) and are refused
// (https://docs.stripe.com/currencies#three-decimal).
var threeDecimal = map[string]bool{"BHD": true, "JOD": true, "KWD": true, "OMR": true, "TND": true}

var decimalPattern = regexp.MustCompile(`^([0-9]{1,10})(?:\.([0-9]{1,2}))?$`)

// ToMinor turns "2490.00" TRY into 249000 (kuruş), "99" EUR into 9900 (cent), "500" JPY into 500.
func ToMinor(amount, currency string) (int64, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if threeDecimal[currency] || !currencyPattern.MatchString(currency) {
		return 0, ErrNotSupported
	}
	m := decimalPattern.FindStringSubmatch(strings.TrimSpace(amount))
	if m == nil {
		return 0, ErrCheckoutIncomplete
	}
	whole, _ := strconv.ParseInt(m[1], 10, 64)
	fraction := m[2]
	if zeroDecimal[currency] {
		if strings.Trim(fraction, "0") != "" {
			return 0, ErrCheckoutIncomplete
		}
		return whole, nil
	}
	for len(fraction) < 2 {
		fraction += "0"
	}
	cents, _ := strconv.ParseInt(fraction, 10, 64)
	return whole*100 + cents, nil
}

// FromMinor is ToMinor's inverse, always with two decimals for a two-decimal currency.
func FromMinor(minor int64, currency string) (string, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if minor < 0 || threeDecimal[currency] || !currencyPattern.MatchString(currency) {
		return "", ErrInvalidNotification
	}
	if zeroDecimal[currency] {
		return strconv.FormatInt(minor, 10), nil
	}
	return fmt.Sprintf("%d.%02d", minor/100, minor%100), nil
}

// ---------------------------------------------------------------------------
// References.
//
// Our reference for a checkout or a renewal is letters and digits only (PayTR's merchant_oid must
// be alphanumeric, at most 64, which keeps a later provider open): CelikPanel: "CP", a kind letter
// (C one-time checkout, S subscription checkout, R renewal charge), the licence id (32 hex), and 12
// characters that make it unique. The licence can be read back from it, so a provider that echoes
// only our reference still names the licence.

var referencePattern = regexp.MustCompile(`^CP[CSR]([0-9a-f]{32})[0-9a-zA-Z]{12}$`)

var licensePattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// NewOrderReference is a fresh reference for one checkout attempt: kind C for a one-time payment,
// S when the member asked for automatic renewal.
func NewOrderReference(licenseID string, automaticRenewal bool) (string, error) {
	if !licensePattern.MatchString(licenseID) {
		return "", ErrCheckoutIncomplete
	}
	var random [6]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	kind := "C"
	if automaticRenewal {
		kind = "S"
	}
	return "CP" + kind + licenseID + hex.EncodeToString(random[:]), nil
}

// RenewalReference is the reference of the charge for the year that ends at periodEnd: the same
// every time it is computed, so a second attempt is the same order to the provider.
func RenewalReference(licenseID string, periodEnd time.Time) (string, error) {
	if !licensePattern.MatchString(licenseID) {
		return "", ErrCheckoutIncomplete
	}
	return "CPR" + licenseID + periodEnd.UTC().Format("200601021504"), nil
}

// LicenseFromReference reads the licence id back from one of our references ("" if the
// reference is not ours).
func LicenseFromReference(reference string) string {
	m := referencePattern.FindStringSubmatch(reference)
	if m == nil {
		return ""
	}
	return m[1]
}

// ---------------------------------------------------------------------------
// HTTP. One client shape for every provider: a bounded timeout, a bounded answer, and errors
// that never carry a request (which holds a key) or a response body.

const (
	providerTimeout   = 20 * time.Second
	maxProviderAnswer = 1 << 20
)

func httpClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return &http.Client{Timeout: providerTimeout}
}

// send performs a request and answers the status and at most maxProviderAnswer bytes of body.
// A transport failure is ErrProviderUnavailable with no detail (the URL may carry a key).
func send(client *http.Client, request *http.Request) (int, []byte, error) {
	response, err := httpClient(client).Do(request)
	if err != nil {
		return 0, nil, ErrProviderUnavailable
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxProviderAnswer+1))
	if err != nil || len(body) > maxProviderAnswer {
		return response.StatusCode, nil, ErrProviderUnavailable
	}
	return response.StatusCode, body, nil
}

// header reads a header case-insensitively from a notification.
func (n Notification) header(name string) string {
	for key, values := range n.Headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func languageOr(language string) string {
	if strings.HasPrefix(strings.ToLower(language), "en") {
		return "en"
	}
	return "tr"
}

// productName is what the payer sees on the provider's page. CelikPanel: one licence, one year.
func productName(language string) string {
	if languageOr(language) == "en" {
		return "CelikPanel licence, one server, one year"
	}
	return "CelikPanel lisansı, bir sunucu, bir yıl"
}
