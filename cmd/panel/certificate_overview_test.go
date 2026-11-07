package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 ops, G31: the certificates overview and certificate requests.

func stubCertificateAgent(t *testing.T) (created *transport.CertificateRequestCreateRequest, discarded *[]string) {
	t.Helper()
	previousDefault, previousMail := readDefaultHTTPSForOverview, readMailHostCertificateForOverview
	previousCreate, previousDiscard := callCertificateRequestCreate, callCertificateRequestDiscard
	readDefaultHTTPSForOverview = func(*Panel, context.Context) (core.NginxDefaultHTTPS, error) {
		return core.NginxDefaultHTTPS{State: core.NginxDefaultHTTPSActive, Mode: "reject"}, nil
	}
	readMailHostCertificateForOverview = func(*Panel, context.Context, string) (transport.MailHostCertificateStatusResponse, error) {
		return transport.MailHostCertificateStatusResponse{}, errors.New("agent unreachable")
	}
	created = &transport.CertificateRequestCreateRequest{}
	list := []string{}
	discarded = &list
	callCertificateRequestCreate = func(_ *Panel, _ context.Context, request *transport.CertificateRequestCreateRequest) (transport.CertificateRequestCreateResponse, error) {
		*created = *request
		return transport.CertificateRequestCreateResponse{
			CSRPEM:          "-----BEGIN CERTIFICATE REQUEST-----\n" + strings.Repeat("A", 120) + "\n-----END CERTIFICATE REQUEST-----\n",
			PublicKeySHA256: strings.Repeat("f", 64),
			KeyFile:         "/var/lib/celikpanel-agent-private/certificate-requests/" + request.Token + ".key",
		}, nil
	}
	callCertificateRequestDiscard = func(_ *Panel, _ context.Context, request *transport.CertificateRequestDiscardRequest) (transport.CertificateRequestDiscardResponse, error) {
		list = append(list, request.Token)
		return transport.CertificateRequestDiscardResponse{Removed: true}, nil
	}
	certificateReadsCache = certificateAgentReads{}
	t.Cleanup(func() {
		readDefaultHTTPSForOverview, readMailHostCertificateForOverview = previousDefault, previousMail
		callCertificateRequestCreate, callCertificateRequestDiscard = previousCreate, previousDiscard
		certificateReadsCache = certificateAgentReads{}
	})
	return created, discarded
}

func TestCertificatesOverviewListsEveryUseWithRenewal(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	stubCertificateAgent(t)
	db := database.GetDB()
	result, err := db.Exec(`INSERT INTO subscriptions (owner_id, name, status) VALUES (?, 'cert-test', 'active')`, userID)
	if err != nil {
		t.Fatal(err)
	}
	subscriptionID, _ := result.LastInsertId()
	expires := time.Now().Add(10 * 24 * time.Hour).UTC().Format(time.RFC3339)
	for i, row := range []struct{ name, kind, renew, mail string }{
		{"auto.example.test", "letsencrypt", "1", "1"},
		{"upload.example.test", "custom", "0", "0"},
		{"bad.example.test", "letsencrypt", "'not-a-bool'", "0"},
	} {
		if _, err := db.Exec(`INSERT INTO domains (subscription_id, name, status) VALUES (?, ?, 'active')`, subscriptionID, row.name); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO ssl_certificates (domain_id, type, cert_path, key_path, expires_at, auto_renew, secure_mail, issuer, status)
			SELECT id, ?, '/x/'||?, '/y/'||?, ?, `+row.renew+`, `+row.mail+`, 'R11', 'active' FROM domains WHERE name = ?`,
			row.kind, strconv.Itoa(i), strconv.Itoa(i), expires, row.name); err != nil {
			t.Fatal(err)
		}
	}
	panel.setSetting(context.Background(), settingMailHostname, "mail.example.test")

	recorder := httptest.NewRecorder()
	panel.handleCertificatesOverview(recorder, serviceStateAtomicRequest(http.MethodGet, certificatesOverviewPath, "", userID))
	if recorder.Code != http.StatusOK {
		t.Fatalf("overview: %d %s", recorder.Code, recorder.Body.String())
	}
	var overview certificatesOverview
	if err := json.Unmarshal(recorder.Body.Bytes(), &overview); err != nil {
		t.Fatal(err)
	}
	byName := map[string]certificateView{}
	for _, item := range overview.Certificates {
		byName[item.Kind+":"+item.Name] = item
	}
	if overview.Certificates[0].Kind != "panel" {
		t.Fatalf("first: %+v", overview.Certificates[0])
	}
	if item := byName["default_https:default"]; item.State != "in_use" || item.Source != "none" || item.Renewal != "none" {
		t.Fatalf("default https: %+v", item)
	}
	if item := byName["mail_host:mail.example.test"]; item.State != "unknown" {
		t.Fatalf("mail host unknown: %+v", item)
	}
	auto := byName["site:auto.example.test"]
	if auto.Renewal != "automatic" || auto.Source != "letsencrypt" || len(auto.UsedBy) != 2 || auto.DaysLeft == nil || *auto.DaysLeft < 9 {
		t.Fatalf("auto: %+v", auto)
	}
	if item := byName["site:upload.example.test"]; item.Renewal != "manual" || item.Source != "uploaded" {
		t.Fatalf("upload: %+v", item)
	}
	if item := byName["site:bad.example.test"]; item.Renewal != "off" {
		t.Fatalf("a stored non-boolean renewal reads as off: %+v", item)
	}
	if overview.Requests == nil || len(overview.Requests) != 0 {
		t.Fatalf("requests: %+v", overview.Requests)
	}
	// The Agent reads are kept: a second page load does not ask again.
	calls := 0
	readDefaultHTTPSForOverview = func(*Panel, context.Context) (core.NginxDefaultHTTPS, error) {
		calls++
		return core.NginxDefaultHTTPS{}, nil
	}
	recorder = httptest.NewRecorder()
	panel.handleCertificatesOverview(recorder, serviceStateAtomicRequest(http.MethodGet, certificatesOverviewPath, "", userID))
	if calls != 0 || recorder.Code != http.StatusOK {
		t.Fatalf("cached: calls=%d code=%d", calls, recorder.Code)
	}
}

func TestCertificateRequestIsMadeListedAndDiscarded(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	created, discarded := stubCertificateAgent(t)

	for _, body := range []string{
		`{"names":[],"key_type":"ecdsa-p256"}`,
		`{"names":["localhost"]}`,
		`{"names":["a.example.test"],"key_type":"dsa"}`,
		`{"names":["a.example.test"],"private_key":"x"}`,
	} {
		recorder := httptest.NewRecorder()
		panel.handleCertificateRequests(recorder, serviceStateAtomicRequest(http.MethodPost, certificateRequestsPath, body, userID))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", body, recorder.Code, recorder.Body.String())
		}
	}
	if created.Token != "" {
		t.Fatal("the Agent was asked for a refused request")
	}

	recorder := httptest.NewRecorder()
	panel.handleCertificateRequests(recorder, serviceStateAtomicRequest(http.MethodPost, certificateRequestsPath,
		`{"names":[" Shop.Example.Test ","*.example.test","shop.example.test"]}`, userID))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", recorder.Code, recorder.Body.String())
	}
	if created.KeyType != transport.CertificateKeyECDSAP256 || created.CommonName != "shop.example.test" ||
		strings.Join(created.Names, ",") != "shop.example.test,*.example.test" || len(created.Token) != 32 {
		t.Fatalf("agent request: %+v", created)
	}
	var view certificateRequestView
	_ = json.Unmarshal(recorder.Body.Bytes(), &view)
	if view.ID == 0 || !strings.Contains(view.CSRPEM, "CERTIFICATE REQUEST") || strings.Contains(recorder.Body.String(), "PRIVATE") {
		t.Fatalf("answer: %s", recorder.Body.String())
	}
	var audited int
	_ = database.GetDB().QueryRow(`SELECT count(*) FROM audit_logs WHERE action = 'certificate.request.create'`).Scan(&audited)
	if audited != 1 {
		t.Fatalf("audit: %d", audited)
	}

	recorder = httptest.NewRecorder()
	panel.handleCertificatesOverview(recorder, serviceStateAtomicRequest(http.MethodGet, certificatesOverviewPath, "", userID))
	if !strings.Contains(recorder.Body.String(), `"common_name":"shop.example.test"`) {
		t.Fatalf("listed: %s", recorder.Body.String())
	}

	path := certificateRequestsPath + "/" + strconv.Itoa(view.ID) + "/discard"
	recorder = httptest.NewRecorder()
	panel.handleCertificateRequestItem(recorder, serviceStateAtomicRequest(http.MethodPost, path, "", userID))
	if recorder.Code != http.StatusOK || len(*discarded) != 1 || (*discarded)[0] != created.Token {
		t.Fatalf("discard: %d %s %v", recorder.Code, recorder.Body.String(), *discarded)
	}
	var state string
	_ = database.GetDB().QueryRow(`SELECT state FROM certificate_requests WHERE id = ?`, view.ID).Scan(&state)
	if state != "discarded" {
		t.Fatalf("state: %s", state)
	}
	recorder = httptest.NewRecorder()
	panel.handleCertificateRequestItem(recorder, serviceStateAtomicRequest(http.MethodPost, path, "", userID))
	if recorder.Code != http.StatusNotFound || len(*discarded) != 1 {
		t.Fatalf("second discard: %d", recorder.Code)
	}

	// The Agent does not answer: not known, nothing listed.
	callCertificateRequestCreate = func(*Panel, context.Context, *transport.CertificateRequestCreateRequest) (transport.CertificateRequestCreateResponse, error) {
		return transport.CertificateRequestCreateResponse{}, errors.New("reset")
	}
	recorder = httptest.NewRecorder()
	panel.handleCertificateRequests(recorder, serviceStateAtomicRequest(http.MethodPost, certificateRequestsPath, `{"names":["b.example.test"]}`, userID))
	if recorder.Code != http.StatusBadGateway || !strings.Contains(recorder.Body.String(), errCodeCertificateRequestUnknown) {
		t.Fatalf("unknown: %d %s", recorder.Code, recorder.Body.String())
	}
	var waiting int
	_ = database.GetDB().QueryRow(`SELECT count(*) FROM certificate_requests WHERE state = 'waiting'`).Scan(&waiting)
	if waiting != 0 {
		t.Fatalf("waiting: %d", waiting)
	}

	// Only the administrator.
	for _, handle := range []func(http.ResponseWriter, *http.Request){panel.handleCertificatesOverview, panel.handleCertificateRequests, panel.handleCertificateRequestItem} {
		recorder = httptest.NewRecorder()
		handle(recorder, maintenanceCustomerRequest(http.MethodPost, path, `{}`))
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("customer: %d", recorder.Code)
		}
	}
}
