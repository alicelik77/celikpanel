package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// fakeMainCF answers postconf -n -h / -x -h / -e / -X like Postfix for the
// two settings, from an explicit map and the stock defaults.
type fakeMainCF struct {
	set      map[string]string
	defaults map[string]string
	calls    []string
	failOn   string
}

func newFakeMainCF(set map[string]string) *fakeMainCF {
	return &fakeMainCF{set: set, defaults: map[string]string{
		passwordlessSubmitSetting: "static:anyone",
		passwordlessRelaySetting:  "permit_mynetworks permit_sasl_authenticated defer_unauth_destination",
	}}
}

func (f *fakeMainCF) run(name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	if name != "postconf" {
		return nil, errors.New("unexpected " + name)
	}
	if f.failOn != "" && strings.Contains(call, f.failOn) {
		return []byte("postconf: fatal: simulated"), errors.New("exit status 1")
	}
	switch {
	case len(args) == 3 && args[0] == "-n" && args[1] == "-h":
		if value, ok := f.set[args[2]]; ok {
			return []byte(value + "\n"), nil
		}
		return nil, nil
	case len(args) == 3 && args[0] == "-x" && args[1] == "-h":
		if value, ok := f.set[args[2]]; ok {
			return []byte(value + "\n"), nil
		}
		return []byte(f.defaults[args[2]] + "\n"), nil
	case len(args) == 2 && args[0] == "-e":
		kv := strings.SplitN(args[1], "=", 2)
		f.set[kv[0]] = kv[1]
		return nil, nil
	case len(args) == 2 && args[0] == "-X":
		delete(f.set, args[1])
		return nil, nil
	case len(args) == 2 && args[0] == "-h" && args[1] == "mail_version":
		return []byte("3.10.1\n"), nil
	}
	return nil, errors.New("unexpected " + call)
}

func withPasswordlessRecord(t *testing.T) string {
	t.Helper()
	saved := passwordlessRecordPath
	passwordlessRecordPath = filepath.Join(t.TempDir(), "private", "mail-passwordless.json")
	t.Cleanup(func() { passwordlessRecordPath = saved })
	return passwordlessRecordPath
}

func TestRelayWithoutMynetworks(t *testing.T) {
	for _, tc := range []struct {
		in, want    string
		changed, ok bool
	}{
		{"permit_mynetworks permit_sasl_authenticated defer_unauth_destination", "permit_sasl_authenticated, defer_unauth_destination", true, true},
		{"permit_mynetworks, permit_sasl_authenticated, reject_unauth_destination", "permit_sasl_authenticated, reject_unauth_destination", true, true},
		{"permit_sasl_authenticated defer_unauth_destination", "permit_sasl_authenticated defer_unauth_destination", false, true},
		{"", passwordlessRelayFallback, true, true},
		{"permit_mynetworks", passwordlessRelayFallback, true, true},
		{"$my_relay_list", "", false, false},
		{"permit_mynetworks, { check_client_access cidr:/x }", "", false, false},
	} {
		got, changed, ok := relayWithoutMynetworks(tc.in)
		if got != tc.want || changed != tc.changed || ok != tc.ok {
			t.Errorf("%q: got %q %v %v", tc.in, got, changed, ok)
		}
	}
}

func TestPasswordlessCloseOnStockDebianThenIdempotent(t *testing.T) {
	path := withPasswordlessRecord(t)
	cf := newFakeMainCF(map[string]string{
		passwordlessRelaySetting: "permit_mynetworks permit_sasl_authenticated defer_unauth_destination",
	})
	applied := 0
	now := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	note, err := reconcilePasswordlessMail(cf.run, func() error { applied++; return nil }, now)
	if err != nil || applied != 1 || !strings.Contains(note, "is closed") {
		t.Fatalf("note=%q err=%v applied=%d", note, err, applied)
	}
	if cf.set[passwordlessSubmitSetting] != "root, vmail" ||
		cf.set[passwordlessRelaySetting] != "permit_sasl_authenticated, defer_unauth_destination" {
		t.Fatalf("main.cf %v", cf.set)
	}
	record, err := readPasswordlessRecord()
	if err != nil || record == nil || record.ChangedAt != "2026-11-20T08:00:00Z" ||
		!reflect.DeepEqual(record.SubmitBefore, &passwordlessSetting{Value: "static:anyone"}) ||
		!record.RelayBefore.Set || record.RelayBefore.Value != "permit_mynetworks permit_sasl_authenticated defer_unauth_destination" {
		t.Fatalf("record %+v %v", record, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("record file %v %v", info, err)
	}
	// The next start changes nothing and reloads nothing.
	note, err = reconcilePasswordlessMail(cf.run, func() error { applied++; return nil }, now.Add(time.Hour))
	if err != nil || note != "" || applied != 1 {
		t.Fatalf("second: note=%q err=%v applied=%d", note, err, applied)
	}
	if state := passwordlessEnforcement(cf.run); state.State != passwordlessStateClosed || state.ChangedAt != "2026-11-20T08:00:00Z" {
		t.Fatalf("state %+v", state)
	}
}

func TestPasswordlessCloseKeepsOwnerValues(t *testing.T) {
	withPasswordlessRecord(t)
	cf := newFakeMainCF(map[string]string{
		passwordlessSubmitSetting: "root, www-data",
		passwordlessRelaySetting:  "permit_mynetworks permit_sasl_authenticated defer_unauth_destination",
	})
	note, err := reconcilePasswordlessMail(cf.run, func() error { return nil }, time.Now())
	if err != nil || !strings.Contains(note, "authorized_submit_users = root, www-data (CelikPanel's form: root, vmail)") {
		t.Fatalf("note=%q err=%v", note, err)
	}
	// The relay half is still closed; the owner's list is untouched.
	if cf.set[passwordlessSubmitSetting] != "root, www-data" || strings.Contains(cf.set[passwordlessRelaySetting], "permit_mynetworks") {
		t.Fatalf("main.cf %v", cf.set)
	}
	// Later the owner reopens the relay: reported, not overwritten.
	cf.set[passwordlessRelaySetting] = "permit_mynetworks, permit_sasl_authenticated, defer_unauth_destination"
	note, err = reconcilePasswordlessMail(cf.run, func() error { t.Fatal("no reload expected"); return nil }, time.Now())
	if err != nil || !strings.Contains(note, "smtpd_relay_restrictions = permit_mynetworks") {
		t.Fatalf("note=%q err=%v", note, err)
	}
	if cf.set[passwordlessRelaySetting] != "permit_mynetworks, permit_sasl_authenticated, defer_unauth_destination" {
		t.Fatalf("overwritten %v", cf.set)
	}
	if state := passwordlessEnforcement(cf.run); state.State != passwordlessStateOwner || state.Detail == "" ||
		state.ExpectedRelayRestrictions != "permit_sasl_authenticated, defer_unauth_destination" {
		t.Fatalf("state %+v", state)
	}
}

func TestPasswordlessCloseRollsBackWhenPostfixRefuses(t *testing.T) {
	withPasswordlessRecord(t)
	cf := newFakeMainCF(map[string]string{}) // Arch: neither setting in main.cf
	if state := passwordlessEnforcement(cf.run); state.State != passwordlessStateOpen {
		t.Fatalf("before: %+v", state)
	}
	applies := 0
	_, err := reconcilePasswordlessMail(cf.run, func() error {
		applies++
		if applies == 1 {
			return errors.New("postfix check: fatal: simulated")
		}
		return nil
	}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "earlier Postfix settings were restored") || applies != 2 {
		t.Fatalf("err=%v applies=%d", err, applies)
	}
	if len(cf.set) != 0 {
		t.Fatalf("not restored to defaults: %v", cf.set)
	}
	state := passwordlessEnforcement(cf.run)
	if state.State != passwordlessStateFailed || !strings.Contains(state.Detail, "simulated") {
		t.Fatalf("state %+v", state)
	}
}

func TestPasswordlessCloseUnreadableSettingChangesNothing(t *testing.T) {
	withPasswordlessRecord(t)
	cf := newFakeMainCF(map[string]string{})
	cf.failOn = "-n -h " + passwordlessSubmitSetting
	note, err := reconcilePasswordlessMail(cf.run, func() error { t.Fatal("no reload"); return nil }, time.Now())
	if err != nil || !strings.Contains(note, "was not closed") {
		t.Fatalf("note=%q err=%v", note, err)
	}
	for _, call := range cf.calls {
		if strings.Contains(call, " -e ") || strings.Contains(call, " -X ") {
			t.Fatalf("wrote %q", call)
		}
	}
}

func TestSendAccountMailHandsOverThroughSendmail(t *testing.T) {
	saved, savedPaths := runAccountSendmail, accountMailSendmailPaths
	t.Cleanup(func() { runAccountSendmail, accountMailSendmailPaths = saved, savedPaths })
	fake := filepath.Join(t.TempDir(), "sendmail")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	accountMailSendmailPaths = []string{fake}
	var gotArgs []string
	var gotMessage string
	runAccountSendmail = func(_ context.Context, path string, args []string, message []byte) ([]byte, error) {
		gotArgs, gotMessage = append([]string{path}, args...), string(message)
		return nil, nil
	}
	var resp transport.SendAccountMailResponse
	_ = (&Agent{}).SendAccountMailV1(&transport.SendAccountMailRequest{From: "no-reply@panel.example", To: "user@example.org", Message: []byte("Subject: x\r\n\r\nhi\r\n")}, &resp)
	if !resp.Sent || !reflect.DeepEqual(gotArgs, []string{fake, "-i", "-f", "no-reply@panel.example", "--", "user@example.org"}) || !strings.Contains(gotMessage, "hi") {
		t.Fatalf("resp=%+v args=%v", resp, gotArgs)
	}
	for _, bad := range []transport.SendAccountMailRequest{
		{From: "-oQ/tmp@x.example", To: "user@example.org", Message: []byte("x")},
		{From: "no-reply@panel.example", To: "a@b.example, c@d.example", Message: []byte("x")},
		{From: "no-reply@panel.example", To: "user@example.org"},
	} {
		resp = transport.SendAccountMailResponse{}
		_ = (&Agent{}).SendAccountMailV1(&bad, &resp)
		if resp.Sent || resp.Error == "" {
			t.Fatalf("accepted %+v", bad)
		}
	}
	runAccountSendmail = func(context.Context, string, []string, []byte) ([]byte, error) {
		return []byte("sendmail: fatal: User celikpanel(998) is not allowed to submit mail"), errors.New("exit status 77")
	}
	resp = transport.SendAccountMailResponse{}
	_ = (&Agent{}).SendAccountMailV1(&transport.SendAccountMailRequest{From: "no-reply@panel.example", To: "user@example.org", Message: []byte("x")}, &resp)
	if resp.Sent || !strings.Contains(resp.Error, "not allowed to submit mail") {
		t.Fatalf("%+v", resp)
	}
}
