package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// A Postfix and Dovecot double for the settings: main.cf parameters and the
// files the settings write, in memory. It prints what the real postconf
// prints (nothing for an unset setting with -n).
type settingsHost struct {
	main      map[string]string
	files     map[string][]byte
	dovecot   string
	noDovecot bool
	calls     []string
	failCmd   string
	reloadErr error
	reloads   int
}

func newSettingsHost() *settingsHost {
	return &settingsHost{main: map[string]string{}, files: map[string][]byte{}, dovecot: "+"}
}

func (h *settingsHost) run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	h.calls = append(h.calls, call)
	if h.failCmd != "" && strings.HasPrefix(call, h.failCmd) {
		return []byte("refused"), errors.New("exit status 1")
	}
	switch name {
	case "doveconf":
		return []byte(h.dovecot + "\n"), nil
	case "postmap":
		_, path, _ := strings.Cut(args[0], ":")
		h.files[path+".db"] = []byte("index")
		return nil, nil
	case "postconf":
	default:
		return nil, errors.New("unexpected " + name)
	}
	switch args[0] {
	case "-n":
		value, set := h.main[args[2]]
		if !set {
			return nil, nil
		}
		return []byte(value + "\n"), nil
	case "-e":
		key, value, _ := strings.Cut(args[1], "=")
		h.main[key] = value
	case "-X":
		delete(h.main, args[1])
	}
	return nil, nil
}

func (h *settingsHost) snapshot(path string) (mailFileSnapshot, error) {
	if path == "/etc/postfix/main.cf" {
		raw, _ := json.Marshal(h.main)
		return mailFileSnapshot{path: path, exists: true, content: raw}, nil
	}
	content, ok := h.files[path]
	return mailFileSnapshot{path: path, exists: ok, content: append([]byte(nil), content...)}, nil
}

func (h *settingsHost) restore(s mailFileSnapshot) error {
	if s.path == "/etc/postfix/main.cf" {
		h.main = map[string]string{}
		return json.Unmarshal(s.content, &h.main)
	}
	if s.exists {
		h.files[s.path] = s.content
	} else {
		delete(h.files, s.path)
	}
	return nil
}

func useSettingsHost(t *testing.T, h *settingsHost) {
	t.Helper()
	run, look, read, write, remove, snap, rest, mapType, reload := mailSettingsRun, mailSettingsLookPath, mailSettingsRead, mailSettingsWrite, mailSettingsRemove, mailSettingsSnapshot, mailSettingsRestore, mailSettingsMapType, mailSettingsReload
	t.Cleanup(func() {
		mailSettingsRun, mailSettingsLookPath, mailSettingsRead, mailSettingsWrite, mailSettingsRemove = run, look, read, write, remove
		mailSettingsSnapshot, mailSettingsRestore, mailSettingsMapType, mailSettingsReload = snap, rest, mapType, reload
	})
	mailSettingsRun = h.run
	mailSettingsLookPath = func(name string) (string, error) {
		if name == "doveconf" && h.noDovecot {
			return "", errors.New("not found")
		}
		return "/usr/sbin/" + name, nil
	}
	mailSettingsRead = func(path string) ([]byte, bool, error) {
		raw, ok := h.files[path]
		return raw, ok, nil
	}
	mailSettingsWrite = func(path string, content []byte) error {
		h.files[path] = append([]byte(nil), content...)
		return nil
	}
	mailSettingsRemove = func(path string) error { delete(h.files, path); return nil }
	mailSettingsSnapshot = h.snapshot
	mailSettingsRestore = h.restore
	mailSettingsMapType = func(context.Context) string { return "hash" }
	mailSettingsReload = func() error {
		h.reloads++
		if h.reloadErr != nil && h.reloads == 1 {
			return h.reloadErr
		}
		return nil
	}
}

func subaddressing(enabled bool) transport.MailSettingApplyResponse {
	var resp transport.MailSettingApplyResponse
	applySubaddressing(context.Background(), enabled, &resp)
	return resp
}

func TestSubaddressingTurnsOnAndOffLeavingNothing(t *testing.T) {
	h := newSettingsHost()
	h.main["myhostname"] = "mail.example.test"
	useSettingsHost(t, h)
	if resp := subaddressing(true); !resp.Applied || resp.State != transport.MailSettingOn || h.main["recipient_delimiter"] != "+" {
		t.Fatalf("on = %+v main=%v", resp, h.main)
	}
	if _, ok := h.files[mailSubaddressingMarker]; !ok {
		t.Fatal("no marker")
	}
	if status := readMailServerSettingsStatus(context.Background()); status.Subaddressing != transport.MailSettingOn || status.DovecotDelimiter != "+" {
		t.Fatalf("status = %+v", status)
	}
	// Again: nothing changes, no reload.
	reloads := h.reloads
	if resp := subaddressing(true); !resp.Applied || resp.Changed || h.reloads != reloads {
		t.Fatalf("again = %+v", resp)
	}
	if resp := subaddressing(false); !resp.Applied || resp.State != transport.MailSettingOff {
		t.Fatalf("off = %+v", resp)
	}
	if len(h.files) != 0 || len(h.main) != 1 || h.main["myhostname"] == "" {
		t.Fatalf("leftovers: files=%v main=%v", h.files, h.main)
	}
}

func TestSubaddressingNeverOverwritesTheOwnersDelimiter(t *testing.T) {
	// Debian's and Ubuntu's main.cf carry `recipient_delimiter = +`: that is
	// already on. A wish the owner's value contradicts changes nothing and
	// says so.
	for _, value := range []string{"+", "-", "+-"} {
		h := newSettingsHost()
		h.main["recipient_delimiter"] = value
		useSettingsHost(t, h)
		hasPlus := strings.Contains(value, "+")
		for _, enabled := range []bool{true, false} {
			resp := subaddressing(enabled)
			agrees := enabled == hasPlus
			if resp.Applied != agrees || resp.Changed || (!agrees && resp.Code != transport.MailSettingOwnerConfig) {
				t.Fatalf("%q enabled=%v = %+v", value, enabled, resp)
			}
			if h.main["recipient_delimiter"] != value || h.reloads != 0 || len(h.files) != 0 {
				t.Fatalf("%q enabled=%v changed the server: %v %v", value, enabled, h.main, h.files)
			}
		}
		if status := readMailServerSettingsStatus(context.Background()); status.Subaddressing != transport.MailSettingOwner || status.RecipientDelimiter != value {
			t.Fatalf("%q status = %+v", value, status)
		}
	}
	// CelikPanel's line changed by the owner: kept.
	h := newSettingsHost()
	useSettingsHost(t, h)
	subaddressing(true)
	h.main["recipient_delimiter"] = "-"
	for _, enabled := range []bool{true, false} {
		if resp := subaddressing(enabled); resp.Code != transport.MailSettingEditedConfig || h.main["recipient_delimiter"] != "-" {
			t.Fatalf("edited %v = %+v", enabled, resp)
		}
	}
}

func TestSubaddressingNeedsTheSameDovecotDelimiter(t *testing.T) {
	h := newSettingsHost()
	h.dovecot = "-"
	useSettingsHost(t, h)
	if resp := subaddressing(true); resp.Code != transport.MailSettingDovecot || !strings.Contains(resp.Error, "never changes Dovecot") {
		t.Fatalf("dovecot = %+v", resp)
	}
	h.noDovecot = true
	if resp := subaddressing(true); resp.Code != transport.MailSettingDovecot {
		t.Fatalf("no dovecot = %+v", resp)
	}
	if len(h.files) != 0 || len(h.main) != 0 {
		t.Fatalf("something was written: %v %v", h.files, h.main)
	}
}

func TestSubaddressingPutsBackWhenPostfixRefuses(t *testing.T) {
	h := newSettingsHost()
	useSettingsHost(t, h)
	h.reloadErr = errors.New("postfix check failed")
	resp := subaddressing(true)
	if resp.Applied || resp.Code != transport.MailSettingPostfixFailed || len(h.files) != 0 || len(h.main) != 0 {
		t.Fatalf("resp = %+v files=%v main=%v", resp, h.files, h.main)
	}
}

func senderList(rules ...transport.MailSenderRule) transport.MailSettingApplyResponse {
	var resp transport.MailSettingApplyResponse
	applyMailSenderList(context.Background(), rules, &resp)
	return resp
}

func TestSenderListBlocksAndAllowsWithoutEverSayingOK(t *testing.T) {
	h := newSettingsHost()
	h.main["smtpd_sender_restrictions"] = "permit_mynetworks, reject_unknown_sender_domain"
	useSettingsHost(t, h)
	resp := senderList(
		transport.MailSenderRule{Value: "spam.example", Action: transport.MailSenderBlock},
		transport.MailSenderRule{Value: "boss@spam.example", Action: transport.MailSenderAllow},
	)
	if !resp.Applied || resp.State != transport.MailSettingOn {
		t.Fatalf("apply = %+v", resp)
	}
	content := string(h.files[mailSenderMapPath])
	if !strings.Contains(content, "boss@spam.example DUNNO\n") || !strings.Contains(content, "spam.example 550 5.7.1 ") {
		t.Fatalf("map = %q", content)
	}
	for _, line := range strings.Split(content, "\n") {
		if strings.HasSuffix(line, " OK") || strings.Contains(line, " PERMIT") {
			t.Fatalf("the map can open a relay: %q", line)
		}
	}
	if _, ok := h.files[mailSenderMapPath+".db"]; !ok {
		t.Fatal("postmap was not run")
	}
	if got := h.main["smtpd_sender_restrictions"]; got != "check_sender_access hash:"+mailSenderMapPath+", permit_mynetworks, reject_unknown_sender_domain" {
		t.Fatalf("restrictions = %q", got)
	}
	if status := readMailServerSettingsStatus(context.Background()); status.SenderList != transport.MailSettingOn {
		t.Fatalf("status = %+v", status)
	}
	// Emptying the list removes exactly what was written.
	if resp := senderList(); !resp.Applied || resp.State != transport.MailSettingOff {
		t.Fatalf("empty = %+v", resp)
	}
	if len(h.files) != 0 || h.main["smtpd_sender_restrictions"] != "permit_mynetworks, reject_unknown_sender_domain" {
		t.Fatalf("leftovers: files=%v main=%v", h.files, h.main)
	}
}

func TestSenderListUnsetsTheSettingItAlone(t *testing.T) {
	h := newSettingsHost()
	useSettingsHost(t, h)
	senderList(transport.MailSenderRule{Value: "x@spam.example", Action: transport.MailSenderBlock})
	senderList()
	if _, set := h.main["smtpd_sender_restrictions"]; set || len(h.files) != 0 {
		t.Fatalf("leftovers: %v %v", h.main, h.files)
	}
}

func TestSenderListKeepsTheOwnersFiles(t *testing.T) {
	h := newSettingsHost()
	useSettingsHost(t, h)
	h.files[mailSenderMapPath] = []byte("spam.example REJECT\n")
	if resp := senderList(transport.MailSenderRule{Value: "a.example", Action: transport.MailSenderBlock}); resp.Code != transport.MailSettingEditedConfig {
		t.Fatalf("edited map = %+v", resp)
	}
	delete(h.files, mailSenderMapPath)
	h.main["smtpd_sender_restrictions"] = "$my_sender_checks"
	if resp := senderList(transport.MailSenderRule{Value: "a.example", Action: transport.MailSenderBlock}); resp.Code != transport.MailSettingOwnerConfig {
		t.Fatalf("variable = %+v", resp)
	}
	if len(h.files) != 0 || h.main["smtpd_sender_restrictions"] != "$my_sender_checks" {
		t.Fatal("something was written")
	}
}

func TestSenderListRefusesBadEntriesAndPutsBack(t *testing.T) {
	h := newSettingsHost()
	useSettingsHost(t, h)
	for _, rules := range [][]transport.MailSenderRule{
		{{Value: "Spam.Example", Action: transport.MailSenderBlock}},
		{{Value: "spam.example OK", Action: transport.MailSenderBlock}},
		{{Value: "spam.example", Action: "permit"}},
		{{Value: "a.example", Action: transport.MailSenderBlock}, {Value: "a.example", Action: transport.MailSenderAllow}},
	} {
		if resp := senderList(rules...); resp.Code != transport.MailSettingInvalid {
			t.Fatalf("%v = %+v", rules, resp)
		}
	}
	h.reloadErr = errors.New("postfix check failed")
	resp := senderList(transport.MailSenderRule{Value: "a.example", Action: transport.MailSenderBlock})
	if resp.Code != transport.MailSettingPostfixFailed || len(h.files) != 0 || len(h.main) != 0 {
		t.Fatalf("put back = %+v files=%v main=%v", resp, h.files, h.main)
	}
}

func TestCanonicalMailSenderValue(t *testing.T) {
	for in, want := range map[string]string{"User@Spam.Example": "user@spam.example", "@spam.example": "spam.example", "Spam.Example.": "spam.example"} {
		if got, err := transport.CanonicalMailSenderValue(in); err != nil || got != want {
			t.Fatalf("%q = %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "spam", "a b@x.example", "x@", "spam.example OK", "*.example"} {
		if _, err := transport.CanonicalMailSenderValue(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}
