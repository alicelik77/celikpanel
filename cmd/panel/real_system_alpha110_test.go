//go:build linux && realsystem

package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Real-system evidence for alpha.110, run in the disposable Debian 13 VM
// after the Agent's TestRealSystemA110WebmailAddressEnable (CELIKPANEL_REAL_
// SYSTEM_A110=1): the panel's /webmail/ (behind the panel's own security
// headers) answers only a redirect to webmail's own address, and following it
// reaches Roundcube served by the VM's nginx, verified against the VM's
// trust store. The panel never serves Roundcube's content.
func TestRealSystemA110PanelRedirectsToWebmailAddress(t *testing.T) {
	if os.Getenv("CELIKPANEL_REAL_SYSTEM_A110") != "1" {
		t.Skip("real-system evidence runs only in the disposable alpha.110 VM")
	}
	panel, _, _, _ := newServiceStateAtomicPanel(t)
	if err := panel.saveWebmailAddress(context.Background(), "mail.a110.test"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(securityHeaders(true, http.HandlerFunc(panel.handleWebmailEntry)))
	defer server.Close()
	noFollow := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := noFollow.Get(server.URL + "/webmail/?_task=mail")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	t.Logf("panel /webmail/: HTTP %d Location %q, %d bytes", response.StatusCode, response.Header.Get("Location"), len(body))
	if response.StatusCode != http.StatusFound || response.Header.Get("Location") != "https://mail.a110.test/" || strings.Contains(string(body), "rcmail") {
		t.Fatal("the panel's /webmail/ is not a redirect to webmail's own address")
	}
	followed, err := (&http.Client{Timeout: 20 * time.Second}).Get(server.URL + "/webmail/")
	if err != nil {
		t.Fatalf("following the redirect: %v", err)
	}
	page, _ := io.ReadAll(followed.Body)
	followed.Body.Close()
	t.Logf("followed to %s: HTTP %d, Roundcube login=%v, CSP %q", followed.Request.URL, followed.StatusCode, strings.Contains(string(page), "rcmloginuser"), followed.Header.Get("Content-Security-Policy"))
	if followed.StatusCode != http.StatusOK || followed.Request.URL.Host != "mail.a110.test" || !strings.Contains(string(page), "rcmloginuser") {
		t.Fatal("the redirect does not reach Roundcube on its own address")
	}
	if strings.Contains(followed.Header.Get("Content-Security-Policy"), "script-src 'self'") {
		t.Fatal("webmail's own origin carries the panel's policy")
	}
}
