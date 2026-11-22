//go:build linux

package main

import (
	"os"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.145 (alpha.144 lab D1): webmail could not send. CelikPanel's
// configuration handed mail to the submission service on 587 without
// encryption (smtp_host 'localhost:587'), and the service takes a login only
// with encryption.

func useWebmailBaseDir(t *testing.T) string {
	t.Helper()
	saved := webmailBaseDir
	t.Cleanup(func() { webmailBaseDir = saved })
	webmailBaseDir = t.TempDir()
	return roundcubeConfigPath()
}

func webmailSendingState(t *testing.T) transport.WebmailAddressFormsResponse {
	t.Helper()
	var resp transport.WebmailAddressFormsResponse
	if err := (&Agent{}).WebmailAddressFormsV1(&transport.WebmailAddressFormsRequest{}, &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestA145WebmailSendsWithEncryptionAndRelaxesTheCheckOnLoopbackOnly(t *testing.T) {
	text := renderRoundcubeConfig(strings.Repeat("ab", 18))
	for _, want := range []string{
		"$config['smtp_host'] = 'tls://127.0.0.1:587';",
		"$config['smtp_user'] = '%u';",
		"$config['smtp_pass'] = '%p';",
		"$config['smtp_conn_options'] = ['127.0.0.1' => ['ssl' => ['verify_peer' => false, 'verify_peer_name' => false]]];",
		"$config['imap_host'] = 'localhost:143';",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "'localhost:587'") {
		t.Fatalf("still sends without encryption:\n%s", text)
	}
	// The check is relaxed in exactly one place, keyed by the loopback
	// address Roundcube connects to: Roundcube applies per-host options only
	// to that host, every other host keeps PHP's full check.
	if strings.Count(text, "verify_peer") != 2 || strings.Count(text, "smtp_conn_options") != 1 ||
		!strings.Contains(text, "$config['smtp_conn_options'] = ['127.0.0.1' => [") {
		t.Fatalf("the certificate check is relaxed elsewhere than for 127.0.0.1:\n%s", text)
	}
	if strings.Contains(text, "imap_conn_options") || strings.Contains(text, "allow_self_signed") {
		t.Fatalf("more than the SMTP loopback check changed:\n%s", text)
	}
	if plain, known := roundcubeSendsWithoutEncryption(text); plain || !known {
		t.Fatalf("current text read as plain=%v known=%v", plain, known)
	}
	if plain, known := roundcubeSendsWithoutEncryption(legacyRenderRoundcubeConfig(strings.Repeat("ab", 18))); !plain || !known {
		t.Fatalf("earlier text read as plain=%v known=%v", plain, known)
	}
}

func TestA145SendingSettingIsReadFromTheLastAssignment(t *testing.T) {
	for _, c := range []struct {
		content      string
		plain, known bool
	}{
		{"<?php\n$config = [];\n", true, true},
		{"<?php\n$config['smtp_host'] = 'localhost:587';\n", true, true},
		{"<?php\n$config['smtp_host'] = \"127.0.0.1\"; // own\n", true, true},
		{"<?php\n$config['smtp_host'] = 'localhost:587';\n$config['smtp_host'] = 'ssl://mail.example.test:465';\n", false, true},
		{"<?php\n// $config['smtp_host'] = 'tls://127.0.0.1:587';\n$config['smtp_host'] = 'localhost';\n", true, true},
		{"<?php\n$config['smtp_host'] = 'tls://localhost:587';\n", false, true},
		{"<?php\n$config['smtp_host'] = 'relay.example.test:25';\n", false, true},
		{"<?php\n$config['smtp_host'] = ['imap.example.test' => 'smtp.example.test'];\n", false, false},
		{"<?php\n$config = ['smtp_host' => 'localhost:587'];\n", false, false},
	} {
		if plain, known := roundcubeSendsWithoutEncryption(c.content); plain != c.plain || known != c.known {
			t.Fatalf("%q: plain=%v known=%v, want %v %v", c.content, plain, known, c.plain, c.known)
		}
	}
}

func TestA145EarlierConfigurationIsCorrectedAtTheFirstStart(t *testing.T) {
	path := useWebmailBaseDir(t)
	key := strings.Repeat("cd", 18)
	for _, withAddress := range []bool{false, true} {
		earlier := legacyRenderRoundcubeConfig(key)
		want := renderRoundcubeConfig(key)
		if withAddress {
			earlier += roundcubeAddressSettings
			want += roundcubeAddressSettings
		}
		mustWriteTestFile(t, path, []byte(earlier), 0o640)
		if resp := webmailSendingState(t); resp.Sending != "cannot_send" || resp.SendingFile != path {
			t.Fatalf("before the start: %+v", resp)
		}
		said := reconcileRoundcubeConfigAtStartup()
		if len(said) != 1 || !strings.Contains(said[0], "rewritten in the current form") {
			t.Fatalf("said %q", said)
		}
		content, _ := os.ReadFile(path)
		if string(content) != want {
			t.Fatalf("address=%v: not the current text:\n%s", withAddress, content)
		}
		if info, _ := os.Stat(path); info.Mode().Perm() != 0o640 {
			t.Fatalf("mode = %v", info.Mode())
		}
		if again := reconcileRoundcubeConfigAtStartup(); len(again) != 0 {
			t.Fatalf("a second start changed it again: %q", again)
		}
		if resp := webmailSendingState(t); resp.Sending != "" || resp.SendingFile != "" {
			t.Fatalf("after the start: %+v", resp)
		}
	}
	// A new installation writes the current text.
	if err := writeRoundcubeConfig(webmailBaseDir, ""); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(path)
	if form, managed := parseRoundcubeConfigForm(string(content)); !managed || !form.current || form.withAddress {
		t.Fatalf("new install: %+v %v\n%s", form, managed, content)
	}
}

func TestA145AddressLinesOnTheEarlierTextWriteTheCurrentOne(t *testing.T) {
	path := useWebmailBaseDir(t)
	key := strings.Repeat("ef", 18)
	mustWriteTestFile(t, path, []byte(legacyRenderRoundcubeConfig(key)), 0o640)
	if state, err := setRoundcubeAddressSettings(true); err != nil || state != "applied" {
		t.Fatalf("enable: %q %v", state, err)
	}
	if content, _ := os.ReadFile(path); string(content) != renderRoundcubeConfig(key)+roundcubeAddressSettings {
		t.Fatalf("not the current text with the address lines:\n%s", content)
	}
	mustWriteTestFile(t, path, []byte(legacyRenderRoundcubeConfig(key)+roundcubeAddressSettings), 0o640)
	if state, err := setRoundcubeAddressSettings(false); err != nil || state != "unchanged" {
		t.Fatalf("disable: %q %v", state, err)
	}
	if content, _ := os.ReadFile(path); string(content) != renderRoundcubeConfig(key) {
		t.Fatalf("not the current text:\n%s", content)
	}
}

func TestA145OwnerEditedConfigurationIsKeptWithTheTextBesideIt(t *testing.T) {
	path := useWebmailBaseDir(t)
	key := strings.Repeat("12", 18)
	owner := legacyRenderRoundcubeConfig(key) + roundcubeAddressSettings + "$config['skin'] = 'larry';\n"
	mustWriteTestFile(t, path, []byte(owner), 0o640)
	said := reconcileRoundcubeConfigAtStartup()
	if len(said) != 1 || !strings.Contains(said[0], "kept as it is") || !strings.Contains(said[0], path+roundcubeConfigPendingSuffix) {
		t.Fatalf("said %q", said)
	}
	if content, _ := os.ReadFile(path); string(content) != owner {
		t.Fatal("the owner's configuration changed")
	}
	pending := path + roundcubeConfigPendingSuffix
	text, err := os.ReadFile(pending)
	if err != nil || string(text) != renderRoundcubeConfig(key)+roundcubeAddressSettings {
		t.Fatalf("CelikPanel's text beside it: %v\n%s", err, text)
	}
	if info, _ := os.Stat(pending); info.Mode().Perm() != 0o600 {
		t.Fatalf("the text holds the des_key; mode = %v", info.Mode())
	}
	if resp := webmailSendingState(t); resp.Sending != "cannot_send" || resp.SendingFile != pending {
		t.Fatalf("named: %+v", resp)
	}
	if again := reconcileRoundcubeConfigAtStartup(); len(again) != 0 {
		t.Fatalf("the same text written again: %q", again)
	}
	// The owner corrects the file their own way: the line goes by itself and
	// the text beside it is no longer needed.
	fixed := strings.Replace(owner, "'localhost:587'", "'tls://127.0.0.1:587'", 1)
	mustWriteTestFile(t, path, []byte(fixed), 0o640)
	if resp := webmailSendingState(t); resp.Sending != "" {
		t.Fatalf("still named after the owner's correction: %+v", resp)
	}
	if said := reconcileRoundcubeConfigAtStartup(); len(said) != 0 {
		t.Fatalf("said %q", said)
	}
	if _, err := os.Stat(pending); !os.IsNotExist(err) {
		t.Fatalf("the text beside it stayed: %v", err)
	}
	if content, _ := os.ReadFile(path); string(content) != fixed {
		t.Fatal("the owner's configuration changed")
	}
}

func TestA145OwnerSendingSettingIsNotNamed(t *testing.T) {
	path := useWebmailBaseDir(t)
	for _, owner := range []string{
		"<?php\n$config = [];\n$config['smtp_host'] = 'ssl://mail.example.test:465';\n",
		"<?php\n$config = [];\n$config['smtp_host'] = ['imap.example.test' => 'smtp.example.test'];\n",
	} {
		mustWriteTestFile(t, path, []byte(owner), 0o640)
		if said := reconcileRoundcubeConfigAtStartup(); len(said) != 0 {
			t.Fatalf("said %q", said)
		}
		if _, err := os.Stat(path + roundcubeConfigPendingSuffix); !os.IsNotExist(err) {
			t.Fatalf("a text beside the owner's own sending setting: %v", err)
		}
		if resp := webmailSendingState(t); resp.Sending != "" {
			t.Fatalf("named: %+v", resp)
		}
	}
	// An owner's file with no des_key line of CelikPanel's form that still
	// sends without encryption: named, with the file itself.
	mustWriteTestFile(t, path, []byte("<?php\n$config = [];\n$config['smtp_host'] = 'localhost:587';\n"), 0o640)
	if said := reconcileRoundcubeConfigAtStartup(); len(said) != 1 || !strings.Contains(said[0], "no des_key line") {
		t.Fatalf("said %q", said)
	}
	if resp := webmailSendingState(t); resp.Sending != "cannot_send" || resp.SendingFile != path {
		t.Fatalf("named: %+v", resp)
	}
	// Webmail not installed: nothing.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if said := reconcileRoundcubeConfigAtStartup(); len(said) != 0 {
		t.Fatalf("said %q", said)
	}
	if resp := webmailSendingState(t); resp.Sending != "" {
		t.Fatalf("named: %+v", resp)
	}
}
