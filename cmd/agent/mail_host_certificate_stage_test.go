package main

import (
	"context"
	"errors"
	"io/fs"
	"os/exec"
	"strings"
	"testing"
)

func withMailCommandLookup(t *testing.T, found map[string]error) {
	t.Helper()
	previous := lookupMailTLSCommand
	t.Cleanup(func() { lookupMailTLSCommand = previous })
	lookupMailTLSCommand = func(name string) (string, error) {
		if err, ok := found[name]; ok {
			if err != nil {
				return "", err
			}
			return "/usr/sbin/" + name, nil
		}
		return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
	}
}

// Setup obtains the mail host certificate before installing mail (2 Nov
// 2026). Only a host with neither Postfix nor Dovecot counts as absent; any
// other lookup failure is unknown and stops the issue.
func TestMailHostStackAbsentOnlyWhenNeitherServiceIsFound(t *testing.T) {
	for name, tc := range map[string]struct {
		found   map[string]error
		absent  bool
		failure bool
	}{
		"neither installed": {found: map[string]error{}, absent: true},
		"postfix installed": {found: map[string]error{"postconf": nil}},
		"dovecot installed": {found: map[string]error{"doveconf": nil}},
		"both installed":    {found: map[string]error{"postconf": nil, "doveconf": nil}},
		"postconf unreadable": {
			found:   map[string]error{"postconf": &exec.Error{Name: "postconf", Err: fs.ErrPermission}},
			failure: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			withMailCommandLookup(t, tc.found)
			absent, err := mailHostStackAbsent()
			if (err != nil) != tc.failure || absent != tc.absent {
				t.Fatalf("absent=%v err=%v, want absent=%v failure=%v", absent, err, tc.absent, tc.failure)
			}
		})
	}
}

// Staging never serves a renewal (a renewal replaces a certificate that mail
// already uses) and stops, without publishing, when a mail service appeared
// after the issue began.
func TestStageMailHostCertificateSourceRefusesRenewalAndPresentMail(t *testing.T) {
	if _, err := stageMailHostCertificateSource(context.Background(), "mail.example.test", "request", "qualifier", "leaf"); err == nil || !strings.Contains(err.Error(), "renewal") {
		t.Fatalf("renewal staged: %v", err)
	}
	previousLock := panelCertWithPublishLock
	t.Cleanup(func() { panelCertWithPublishLock = previousLock })
	locked := false
	panelCertWithPublishLock = func(action func() error) error {
		locked = true
		return action()
	}
	withMailCommandLookup(t, map[string]error{"postconf": nil})
	_, err := stageMailHostCertificateSource(context.Background(), "mail.example.test", "request", "qualifier", "")
	if !locked || err == nil || !strings.Contains(err.Error(), "mail services were installed") {
		t.Fatalf("staged with mail present: locked=%v err=%v", locked, err)
	}
	withMailCommandLookup(t, map[string]error{"postconf": errors.New("lookup failed")})
	if _, err := stageMailHostCertificateSource(context.Background(), "mail.example.test", "request", "qualifier", ""); err == nil || !strings.Contains(err.Error(), "inspect installed mail services") {
		t.Fatalf("staged with unknown mail state: %v", err)
	}
}
