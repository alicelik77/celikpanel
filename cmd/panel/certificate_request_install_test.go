package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// alpha.128 ops, G31 follow-up: installing the certificate issued for a
// request. Everything that can be refused is refused before the Agent is
// asked and before anything changes.

type issuedFixture struct {
	key  *ecdsa.PrivateKey
	sha  string
	cert func(names []string, notBefore, notAfter time.Time) string
}

func newIssuedFixture(t *testing.T) issuedFixture {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKIXPublicKey(key.Public())
	sum := sha256.Sum256(der)
	return issuedFixture{key: key, sha: hex.EncodeToString(sum[:]), cert: func(names []string, notBefore, notAfter time.Time) string {
		template := &x509.Certificate{SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: names[0]}, DNSNames: names, NotBefore: notBefore, NotAfter: notAfter}
		raw, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
		if err != nil {
			t.Fatal(err)
		}
		return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw}))
	}}
}

func TestIssuedCertificateIsRefusedBeforeAnythingChanges(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	db := database.GetDB()
	fixture := newIssuedFixture(t)
	other := newIssuedFixture(t)
	now := time.Now()
	result, err := db.Exec(`INSERT INTO certificate_requests (common_name, alt_names, key_type, csr_pem, public_key_sha256, key_file)
		VALUES ('shop.example.test', '["shop.example.test","www.shop.example.test"]', 'ecdsa-p256', ?, ?, '/var/lib/celikpanel-agent-private/certificate-requests/`+strings.Repeat("ab", 16)+`.key')`,
		strings.Repeat("A", 120), fixture.sha)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	path := certificateRequestsPath + "/" + strconv.FormatInt(id, 10) + "/install"
	post := func(body any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		recorder := httptest.NewRecorder()
		panel.handleCertificateRequestItem(recorder, serviceStateAtomicRequest(http.MethodPost, path, string(raw), userID))
		return recorder
	}
	both := []string{"shop.example.test", "www.shop.example.test"}
	keyBlock, _ := x509.MarshalPKCS8PrivateKey(fixture.key)
	for name, testCase := range map[string]struct {
		body map[string]any
		code string
	}{
		"another key":   {map[string]any{"target": "site", "domain_id": 1, "certificate": other.cert(both, now.Add(-time.Hour), now.Add(time.Hour))}, errCodeCertificateRequestMismatch},
		"expired":       {map[string]any{"target": "site", "domain_id": 1, "certificate": fixture.cert(both, now.Add(-48*time.Hour), now.Add(-time.Hour))}, errCodeCertificateRequestExpired},
		"not yet valid": {map[string]any{"target": "site", "domain_id": 1, "certificate": fixture.cert(both, now.Add(time.Hour), now.Add(48*time.Hour))}, errCodeCertificateRequestExpired},
		"names missing": {map[string]any{"target": "site", "domain_id": 1, "certificate": fixture.cert([]string{"shop.example.test"}, now.Add(-time.Hour), now.Add(time.Hour))}, errCodeCertificateRequestNames},
		"panel target":  {map[string]any{"target": "panel", "certificate": fixture.cert(both, now.Add(-time.Hour), now.Add(time.Hour))}, errCodeCertificateRequestTarget},
		"mail target":   {map[string]any{"target": "mail", "certificate": fixture.cert(both, now.Add(-time.Hour), now.Add(time.Hour))}, errCodeCertificateRequestTarget},
		"a private key": {map[string]any{"target": "site", "domain_id": 1, "certificate": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBlock}))}, errCodeCertificateRequestInvalid},
		"unknown field": {map[string]any{"target": "site", "domain_id": 1, "certificate": "x", "private_key": "y"}, errCodeCertificateRequestInvalid},
		"no site":       {map[string]any{"target": "site", "domain_id": 999999, "certificate": fixture.cert(both, now.Add(-time.Hour), now.Add(time.Hour))}, errCodeCertificateRequestInvalid},
	} {
		got := post(testCase.body)
		if got.Code < 400 || !strings.Contains(got.Body.String(), testCase.code) {
			t.Fatalf("%s: %d %s", name, got.Code, got.Body.String())
		}
		if strings.Contains(got.Body.String(), "PRIVATE KEY") {
			t.Fatalf("%s: key material in the answer", name)
		}
	}
	var state string
	_ = db.QueryRow(`SELECT state FROM certificate_requests WHERE id = ?`, id).Scan(&state)
	if state != "waiting" {
		t.Fatalf("a refused install changed the request: %s", state)
	}

	// An installed or discarded request is not installed again.
	_, _ = db.Exec(`UPDATE certificate_requests SET state = 'installed', installed_at = 'x', installed_for = 'site:shop.example.test' WHERE id = ?`, id)
	if got := post(map[string]any{"target": "site", "domain_id": 1, "certificate": fixture.cert(both, now.Add(-time.Hour), now.Add(time.Hour))}); got.Code != http.StatusConflict ||
		!strings.Contains(got.Body.String(), errCodeCertificateRequestInstalled) {
		t.Fatalf("installed: %d %s", got.Code, got.Body.String())
	}
	_, _ = db.Exec(`UPDATE certificate_requests SET state = 'discarded', installed_at = NULL, installed_for = NULL WHERE id = ?`, id)
	if got := post(map[string]any{"target": "site", "domain_id": 1, "certificate": "x"}); got.Code != http.StatusNotFound {
		t.Fatalf("discarded: %d", got.Code)
	}
}

func TestIssuedCertificateSplitAndNames(t *testing.T) {
	fixture := newIssuedFixture(t)
	now := time.Now()
	leafPEM := fixture.cert([]string{"*.example.test", "example.test"}, now.Add(-time.Hour), now.Add(time.Hour))
	intermediate := newIssuedFixture(t).cert([]string{"ca.example.test"}, now.Add(-time.Hour), now.Add(time.Hour))
	leaf, leafOut, chain, err := splitIssuedCertificate(leafPEM+intermediate, "")
	if err != nil || !strings.Contains(chain, "CERTIFICATE") || strings.Count(leafOut, "BEGIN") != 1 {
		t.Fatalf("split: %v", err)
	}
	if issuedPublicKeySHA256(leaf) != fixture.sha {
		t.Fatal("fingerprint of the issued certificate")
	}
	if missing := missingRequestedNames(leaf, []string{"example.test", "*.example.test"}); len(missing) != 0 {
		t.Fatalf("covered names reported missing: %v", missing)
	}
	if missing := missingRequestedNames(leaf, []string{"shop.other.test"}); len(missing) != 1 {
		t.Fatal("an uncovered name was not reported")
	}
	if _, _, _, err := splitIssuedCertificate("not pem", ""); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestCertificateRequestInstallHasItsProgressRoute(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, certificateRequestsPath+"/12/install", nil)
	route, ok := operationStepRouteFor(request)
	if !ok || route.pattern != requestIdentityRouteCertificateRequestInstall.pattern {
		t.Fatalf("route: %v %+v", ok, route)
	}
	for _, path := range []string{certificateRequestsPath + "/x/install", certificateRequestsPath + "/12/discard"} {
		if _, ok := operationStepRouteFor(httptest.NewRequest(http.MethodPost, path, nil)); ok {
			t.Fatalf("%s matched the install route", path)
		}
	}
}
