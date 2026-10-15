package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every mailProfileTextCode constant declared anywhere in this package is in
// mailProfileTextCodes, which the web test reads to require an English and a
// Turkish sentence for each (2026-10-14). A code added without the list fails
// here; a listed code without words fails there.
// Paketteki her mailProfileTextCode sabiti listededir.
func TestMailProfileTextCodesAreListed(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]string{}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, name, source, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				value := spec.(*ast.ValueSpec)
				typed, ok := value.Type.(*ast.Ident)
				if !ok || typed.Name != "mailProfileTextCode" {
					continue
				}
				for i, ident := range value.Names {
					literal := value.Values[i].(*ast.BasicLit)
					declared[ident.Name] = strings.Trim(literal.Value, `"`)
				}
			}
		}
	}
	listed := map[string]bool{}
	for _, code := range mailProfileTextCodes {
		if code == "" || listed[string(code)] {
			t.Fatalf("code %q is empty or listed twice", code)
		}
		listed[string(code)] = true
	}
	if len(declared) != len(listed) {
		t.Fatalf("%d codes declared, %d listed: %v", len(declared), len(listed), declared)
	}
	for name, code := range declared {
		if !listed[code] {
			t.Fatalf("%s (%q) is not in mailProfileTextCodes", name, code)
		}
	}
}

func TestMailProfileViewTypesItsSentences(t *testing.T) {
	type want struct {
		blocked, warning mailProfileTextCode
		vars             map[string]string
	}
	check := func(name string, profile MailProfileResponse, expected want) {
		t.Helper()
		if profile.BlockedCode != expected.blocked || profile.WarningCode != expected.warning {
			t.Fatalf("%s: blocked_code=%q warning_code=%q, want %q/%q (%+v)", name, profile.BlockedCode, profile.WarningCode, expected.blocked, expected.warning, profile)
		}
		if expected.blocked != "" && profile.BlockedReason == "" || expected.warning != "" && profile.Warning == "" {
			t.Fatalf("%s: the English text is gone for older pages: %+v", name, profile)
		}
		for key, value := range expected.vars {
			if profile.BlockedVars[key] != value {
				t.Fatalf("%s: blocked_vars[%s]=%q, want %q (%v)", name, key, profile.BlockedVars[key], value, profile.BlockedVars)
			}
		}
		for _, text := range []string{profile.BlockedReason, profile.Warning} {
			if strings.Contains(strings.ToLower(text), "scan") {
				t.Fatalf("%s: the sentence asks for a scan (D-032): %q", name, text)
			}
		}
	}
	available := catalogView(nil, "apt")
	check("unverified", profileViewByID(t, mailProfilesView(nil, false, "apt", "", false, false, nil), "core-mail"), want{blocked: mailProfileBlockedStateUnverified})
	check("hostname unknown", profileViewByID(t, mailProfilesView(available, true, "apt", mailProfileServerHostnameUnknown, false, true, nil), "core-mail"), want{blocked: mailProfileBlockedHostnameUnknown})
	check("dns identity", profileViewByID(t, mailProfilesView(available, true, "apt", mailProfileCatalogBlockedReason("", false), false, true, nil), "core-mail"), want{blocked: mailProfileBlockedDNSIdentity})
	check("incomplete", profileViewByID(t, mailProfilesView(available[:1], true, "apt", "", false, true, nil), "core-mail"), want{blocked: mailProfileBlockedStateIncomplete})
	check("platform not verified", profileViewByID(t, mailProfilesView(catalogView(nil, ""), true, "", "", false, true, nil), "core-mail"), want{
		blocked: mailProfileBlockedNotInstallable, vars: map[string]string{"component": "Postfix", "kind": "distribution"},
	})
	// A verified platform is named; an unverified one is not (2026-10-15).
	if platform, ok := profileViewByID(t, mailProfilesView(catalogView(nil, ""), true, "", "", false, true, nil), "core-mail").BlockedVars["platform"]; ok {
		t.Fatalf("an unverified platform is named: %q", platform)
	}
	check("pacman layout", profileViewByID(t, mailProfilesView(catalogView(nil, "pacman"), true, "pacman", "", false, true, nil), "core-mail"), want{
		blocked: mailProfileBlockedNotInstallable, vars: map[string]string{"component": "Dovecot", "kind": "distribution", "platform": "Arch Linux (pacman)"},
	})
	check("dnf preview", profileViewByID(t, mailProfilesView(catalogView(nil, "dnf"), true, "dnf", "", false, true, nil), "core-mail"), want{
		blocked: mailProfileBlockedNotInstallable, vars: map[string]string{"component": "Postfix", "platform": "Fedora/RHEL (dnf)"},
	})
	conflict := catalogView([]serviceObservation{{ID: "exim", IsInstalled: true, Status: "active (running)"}}, "apt")
	conflicted := profileViewByID(t, mailProfilesView(conflict, true, "apt", "", false, true, nil), "core-mail")
	check("conflict", conflicted, want{blocked: mailProfileBlockedConflict, vars: map[string]string{"component": "Postfix"}})
	if conflicted.BlockedVars["conflicting"] == "" {
		t.Fatalf("conflict names no installed component: %v", conflicted.BlockedVars)
	}

	partial := catalogView([]serviceObservation{{ID: "postfix", IsInstalled: true, Status: "active (running)"}}, "apt")
	check("partial", profileViewByID(t, mailProfilesView(partial, true, "apt", "", false, true, nil), "core-mail"), want{warning: mailProfileWarningPartlyPresent})
	complete := catalogView([]serviceObservation{
		{ID: "postfix", IsInstalled: true, Status: "active (running)"},
		{ID: "dovecot", IsInstalled: true, Status: "active (running)"},
	}, "apt")
	check("complete, not verified", profileViewByID(t, mailProfilesView(complete, true, "apt", "", false, true, nil), "core-mail"), want{warning: mailProfileWarningRepairToVerify})
	check("verified", profileViewByID(t, mailProfilesView(complete, true, "apt", "", false, true, map[string]mailProfileAttemptProof{
		"core-mail": {Status: mailProfileAttemptSucceeded, Verified: true},
	}), "core-mail"), want{})

	webmail := catalogView([]serviceObservation{
		{ID: "postfix", IsInstalled: true, Status: "active (running)"},
		{ID: "dovecot", IsInstalled: true, Status: "active (running)"},
		{ID: "nginx", IsInstalled: true, Status: "active (running)"},
		{ID: "php-fpm", IsInstalled: true, Status: "active (running)"},
		{ID: "roundcube", IsInstalled: true, Status: "installed"},
	}, "apt")
	check("webmail unproven", profileViewByID(t, mailProfilesView(webmail, true, "apt", "", false, false, nil), "webmail"), want{warning: mailProfileWarningWebmailUnverified})
	check("webmail dead socket", profileViewByID(t, mailProfilesView(webmail, true, "apt", "", false, true, nil), "webmail"), want{warning: mailProfileWarningRoundcubeNotReady})

	// On the wire: the typed fields beside the text, absent when there is none.
	body, err := json.Marshal(profileViewByID(t, mailProfilesView(conflict, true, "apt", "", false, true, nil), "core-mail"))
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"blocked_reason", "blocked_code", "blocked_vars"} {
		if _, ok := wire[key]; !ok {
			t.Fatalf("%s missing from %s", key, body)
		}
	}
	if _, ok := wire["warning_code"]; ok {
		t.Fatalf("warning_code present without a warning: %s", body)
	}
}
