package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.105 (D-034 amendment): the install read names what the package
// manager would bring, the notable packages among them and a chosen PHP
// version that cannot serve its tool; a failed read says "unknown".

func stubInstallPreview(t *testing.T, answer func(transport.ServiceInstallPreviewRequest) (transport.ServiceInstallPreviewResponse, error)) *[]transport.ServiceInstallPreviewRequest {
	t.Helper()
	restore := installPackagePreview
	t.Cleanup(func() { installPackagePreview = restore })
	var asked []transport.ServiceInstallPreviewRequest
	installPackagePreview = func(_ *Panel, _ context.Context, request transport.ServiceInstallPreviewRequest) (transport.ServiceInstallPreviewResponse, error) {
		asked = append(asked, request)
		return answer(request)
	}
	return &asked
}

func installPreviewRequest(panel *Panel, method, query string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	panel.handleServiceInstallPreview(recorder, httptest.NewRequest(method, serviceInstallPreviewPath+query, nil))
	return recorder
}

func TestInstallPreviewCarriesTheSimulationAndItsNotablePackages(t *testing.T) {
	panel := newManagedServiceCachePanel(t)
	asked := stubInstallPreview(t, func(transport.ServiceInstallPreviewRequest) (transport.ServiceInstallPreviewResponse, error) {
		return transport.ServiceInstallPreviewResponse{
			State:     "known",
			Requested: []string{"php8.3-fpm", "phpmyadmin"},
			Installs:  []string{"php8.3-fpm", "phpmyadmin", "apache2"},
			Notable:   []transport.NotableInstall{{Package: "apache2", Reason: "apache", Component: "apache"}},
		}, nil
	})
	recorder := installPreviewRequest(panel, http.MethodGet, "?step=php-fpm:php8.3-fpm&step=phpmyadmin::8.3")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	want := []transport.ServiceInstallPreviewStep{{ID: "php-fpm", Package: "php8.3-fpm"}, {ID: "phpmyadmin", PHPVersion: "8.3"}}
	if len(*asked) != 1 || !reflect.DeepEqual((*asked)[0].Steps, want) {
		t.Fatalf("asked = %+v", *asked)
	}
	var answer serviceInstallPreviewAnswer
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.State != "known" || len(answer.Notable) != 1 || answer.Notable[0].Package != "apache2" || answer.Removes == nil || answer.Refusals == nil {
		t.Fatalf("answer = %+v", answer)
	}
}

func TestInstallPreviewSaysUnknownWhenTheReadFails(t *testing.T) {
	panel := newManagedServiceCachePanel(t)
	stubInstallPreview(t, func(transport.ServiceInstallPreviewRequest) (transport.ServiceInstallPreviewResponse, error) {
		return transport.ServiceInstallPreviewResponse{}, errors.New("agent timeout")
	})
	recorder := installPreviewRequest(panel, http.MethodGet, "?step=nginx")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"state":"unknown"`) {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	// The Agent's own refusal of a PHP version stands with a failed simulation.
	stubInstallPreview(t, func(transport.ServiceInstallPreviewRequest) (transport.ServiceInstallPreviewResponse, error) {
		return transport.ServiceInstallPreviewResponse{State: "failed", Error: "APT simulation failed",
			Refusals: []transport.InstallPreviewRefusal{{ID: "phpmyadmin", Code: "php_extensions_unavailable", Version: "8.6", Packages: []string{"php8.6-mysql"}}}}, nil
	})
	recorder = installPreviewRequest(panel, http.MethodGet, "?step=phpmyadmin::8.6")
	var answer serviceInstallPreviewAnswer
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.State != "unknown" || len(answer.Refusals) != 1 || answer.Refusals[0].Packages[0] != "php8.6-mysql" {
		t.Fatalf("answer = %+v", answer)
	}
}

func TestInstallPreviewRefusesStepsItCannotName(t *testing.T) {
	panel := newManagedServiceCachePanel(t)
	asked := stubInstallPreview(t, func(transport.ServiceInstallPreviewRequest) (transport.ServiceInstallPreviewResponse, error) {
		return transport.ServiceInstallPreviewResponse{State: "known"}, nil
	})
	for _, query := range []string{
		"", "?step=nope", "?step=php-fpm:nginx", "?step=nginx::8.3", "?step=phpmyadmin::8", "?step=bind",
		"?step=a&step=b&step=c&step=d&step=e&step=f&step=g&step=h&step=i", "?step=php-fpm:php8.3-fpm:8.3:x",
	} {
		if recorder := installPreviewRequest(panel, http.MethodGet, query); recorder.Code != http.StatusBadRequest {
			t.Fatalf("%q: status = %d", query, recorder.Code)
		}
	}
	if recorder := installPreviewRequest(panel, http.MethodPost, "?step=nginx"); recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d", recorder.Code)
	}
	if recorder := installPreviewRequest(panel, http.MethodGet, "?step=postgresql:postgresql-17"); recorder.Code != http.StatusOK {
		t.Fatalf("a PostgreSQL version pick: status = %d %s", recorder.Code, recorder.Body.String())
	}
	if len(*asked) != 1 {
		t.Fatalf("asked = %+v", *asked)
	}
}

func TestInstallChoicesAreValidated(t *testing.T) {
	for _, tc := range []struct {
		req serviceInstallRequest
		ok  bool
	}{
		{serviceInstallRequest{ServiceID: "phpmyadmin", PHPVersion: "8.3", CheckInstalls: true, ConfirmedInstalls: []string{"apache2", "libapache2-mod-php8.5"}}, true},
		{serviceInstallRequest{ServiceID: "nginx", CheckInstalls: true}, true},
		{serviceInstallRequest{ServiceID: "nginx", PHPVersion: "8.3"}, false},
		{serviceInstallRequest{ServiceID: "phppgadmin", PHPVersion: "8"}, false},
		{serviceInstallRequest{ServiceID: "nginx", ConfirmedInstalls: []string{"-o"}}, false},
		{serviceInstallRequest{ServiceID: "nginx", ConfirmedInstalls: make([]string, maxConfirmedInstalls+1)}, false},
	} {
		if got := validServiceInstallChoices(tc.req) == ""; got != tc.ok {
			t.Fatalf("%+v: ok = %v", tc.req, got)
		}
	}
}

func TestARefusedInstallSaysWhatItWouldBringInOneLine(t *testing.T) {
	detail := notableRefusalDetail([]transport.NotableInstall{
		{Package: "php8.4-common", Reason: "new_php_version", Version: "8.4"},
		{Package: "php8.4-mbstring", Reason: "new_php_version", Version: "8.4"},
		{Package: "php8.4-mysql", Reason: "new_php_version", Version: "8.4"},
		{Package: "php8.4-xml", Reason: "new_php_version", Version: "8.4"},
	})
	if detail != "Not installed: it would also bring php8.4-common, php8.4-mbstring (+2 more). Install it from Components to see and confirm them." {
		t.Fatalf("detail = %q", detail)
	}
	if len([]rune(detail)) > serviceFailureDetailLimit {
		t.Fatalf("detail is longer than Server setup shows: %d", len(detail))
	}
}

func TestAResumedInstallKeepsItsChoices(t *testing.T) {
	req := serviceInstallRequest{ServiceID: "phpmyadmin", PHPVersion: "8.3", ConfirmedInstalls: []string{"php8.4-common"}}
	raw := encodeServiceInstallOperationData(req)
	resumed := serviceInstallRequest{ServiceID: "phpmyadmin"}
	restoreServiceInstallChoices(&resumed, raw)
	if resumed.PHPVersion != "8.3" || len(resumed.ConfirmedInstalls) != 1 || resumed.ConfirmedInstalls[0] != "php8.4-common" {
		t.Fatalf("resumed = %+v", resumed)
	}
	if encodeServiceInstallOperationData(serviceInstallRequest{ServiceID: "nginx"}) != "" {
		t.Fatal("an install with no choices recorded data")
	}
	// A record that does not validate is dropped: the check stays, nothing confirmed.
	other := serviceInstallRequest{ServiceID: "nginx"}
	restoreServiceInstallChoices(&other, raw)
	if other.PHPVersion != "" || len(other.ConfirmedInstalls) != 0 {
		t.Fatalf("other = %+v", other)
	}
	restoreServiceInstallChoices(&other, `{"setup_mail_hostname":"mail.example.net"}`)
	if other.PHPVersion != "" {
		t.Fatalf("other = %+v", other)
	}
}

// alpha.109: the confirmation names whose version a vendor package replaces
// ("Ubuntu’s version"), so the answer carries this server's distribution;
// without os-release it is left out and the page says "the distribution".
func TestInstallPreviewNamesTheServersDistribution(t *testing.T) {
	panel := newManagedServiceCachePanel(t)
	stubInstallPreview(t, func(transport.ServiceInstallPreviewRequest) (transport.ServiceInstallPreviewResponse, error) {
		return transport.ServiceInstallPreviewResponse{State: "known"}, nil
	})
	withSupportServer(t, "ID=ubuntu\nVERSION_ID=\"24.04\"\nVERSION_CODENAME=noble\n")
	var answer serviceInstallPreviewAnswer
	if err := json.Unmarshal(installPreviewRequest(panel, http.MethodGet, "?step=mariadb").Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Distribution != "ubuntu" {
		t.Fatalf("distribution = %q", answer.Distribution)
	}
	osReleasePath = filepath.Join(t.TempDir(), "missing")
	body := installPreviewRequest(panel, http.MethodGet, "?step=mariadb").Body.String()
	if strings.Contains(body, "distribution") {
		t.Fatalf("an unread os-release still named a distribution: %s", body)
	}
}
