//go:build linux

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// TestLabFTPLoginStep drives the real Agent code on a disposable lab guest
// (alpha.128 escape matrix, docs/FTP-LOGINS.md). It never runs in ordinary
// test runs: it needs CELIKPANEL_LAB_FTP=1, root, and a guest made for it.
// One step per run (CP_LAB_STEP); the step's values come from the
// environment, and a password from standard input (never an argument).
//
// Laboratuvar konuğunda gerçek Agent kodunu adım adım çalıştırır; olağan
// test çalıştırmalarında çalışmaz.
func TestLabFTPLoginStep(t *testing.T) {
	if os.Getenv("CELIKPANEL_LAB_FTP") != "1" || os.Geteuid() != 0 {
		t.Skip("lab only: CELIKPANEL_LAB_FTP=1 as root on a lab guest")
	}
	// TestMain points the sandbox's native files at a throwaway folder; the
	// lab guest uses the production places, as the Agent does.
	sandboxEgressDir = sitesandbox.EgressDir
	sandboxLedgerPath = filepath.Join(hostingpath.ServiceMutationStateRoot(), "sandbox", "native-files.json")
	sandboxUnitDir = unitDir
	sandboxDaemonReload = hostDaemonReload
	sandboxHelperSelfSum = ""
	if helper := os.Getenv("CP_LAB_HELPER"); helper != "" {
		sandboxHelperSource = helper
	}
	env := func(name string) string { return strings.TrimSpace(os.Getenv(name)) }
	atoi := func(name string) int {
		var n int
		_, _ = fmt.Sscan(env(name), &n)
		return n
	}
	password := ""
	if env("CP_LAB_PASSWORD_STDIN") == "1" {
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		password = strings.TrimRight(line, "\r\n")
	}
	site := transport.FileAccessSite{SubscriptionID: atoi("CP_LAB_SUB"), DomainID: atoi("CP_LAB_DOMAIN"), Username: env("CP_LAB_SITE")}
	agent := &Agent{}
	var out any
	switch step := env("CP_LAB_STEP"); step {
	case "site-access":
		var resp transport.ApplyFileAccessResponse
		err := agent.ApplyFileAccessV1(&transport.ApplyFileAccessRequest{Site: site, Enabled: true, Password: password}, &resp)
		if err != nil {
			t.Fatal(err)
		}
		out = resp
	case "add", "update", "remove":
		var resp transport.ApplyFTPLoginResponse
		err := agent.ApplyFTPLoginV1(&transport.ApplyFTPLoginRequest{Site: site, Action: step, Login: env("CP_LAB_LOGIN"),
			Folder: env("CP_LAB_FOLDER"), Password: password, ReadOnly: env("CP_LAB_READONLY") == "1"}, &resp)
		if err != nil {
			t.Fatal(err)
		}
		out = resp
	case "state":
		var resp transport.FTPLoginsStateResponse
		if err := agent.FTPLoginsStateV1(&transport.FTPLoginsStateRequest{Site: site}, &resp); err != nil {
			t.Fatal(err)
		}
		out = resp
	case "readonly-on", "readonly-off":
		var resp transport.SetFileAccessReadOnlyResponse
		err := agent.SetFileAccessReadOnlyV1(&transport.SetFileAccessReadOnlyRequest{SubscriptionID: site.SubscriptionID, ReadOnly: step == "readonly-on"}, &resp)
		if err != nil {
			t.Fatal(err)
		}
		out = resp
	case "suspend", "restore":
		// The site account's own suspension is chage -E (done by the lab
		// script); this is the Agent's follow-up for the logins.
		expiry := ""
		if step == "suspend" {
			expiry = transport.SuspensionSiteAccountExpiry
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		err := syncFTPLoginsExpiry(ctx, site.Username, expiry)
		out = map[string]any{"error": fmt.Sprint(err)}
	case "revoke":
		var resp transport.ApplyFileAccessResponse
		err := agent.ApplyFileAccessV1(&transport.ApplyFileAccessRequest{Site: site, Enabled: false, RevokeCredentials: true}, &resp)
		if err != nil {
			t.Fatal(err)
		}
		out = resp
	case "forget-site":
		out = map[string]any{"error": fmt.Sprint(forgetFTPLoginsOfSite(site.Username))}
	default:
		t.Fatalf("unknown step %q", step)
	}
	encoded, _ := json.Marshal(out)
	fmt.Printf("LABRESULT %s\n", encoded)
}
