package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

const testManagedMailCert = managedMailHostTLSDir + "/current/fullchain.pem"
const testManagedMailKey = managedMailHostTLSDir + "/current/privkey.pem"

func TestDecidePostfixServerCertificate(t *testing.T) {
	for name, tc := range map[string]struct {
		certFile, chainFiles, managed string
		want                          postfixCertAction
	}{
		"debian snakeoil, managed current":   {debianSnakeoilCert, "", testManagedMailCert, postfixCertManaged},
		"none named, managed current":        {"", "", testManagedMailCert, postfixCertManaged},
		"celikpanel default, managed":        {postfixDefaultCertPath, "", testManagedMailCert, postfixCertManaged},
		"mail tls default, managed":          {defaultMailCert, "", testManagedMailCert, postfixCertManaged},
		"already managed":                    {testManagedMailCert, "", testManagedMailCert, postfixCertKeep},
		"owner certificate, managed current": {"/etc/ssl/owner/mail.pem", "", testManagedMailCert, postfixCertKeep},
		"owner chain files, managed current": {"", "/etc/ssl/owner/chain.pem", testManagedMailCert, postfixCertKeep},
		"none named, no managed (Arch)":      {"", "", "", postfixCertDefault},
		"snakeoil, no managed":               {debianSnakeoilCert, "", "", postfixCertKeep},
		"chain files, no managed":            {"", "/etc/x/chain.pem", "", postfixCertKeep},
	} {
		if got := decidePostfixServerCertificate(tc.certFile, tc.chainFiles, tc.managed); got != tc.want {
			t.Errorf("%s: %v, want %v", name, got, tc.want)
		}
	}
}

func fakePostconf(t *testing.T, settings map[string]string) postfixCommandRunner {
	return func(name string, args ...string) ([]byte, error) {
		switch {
		case name == "postconf" && args[0] == "-h" && len(args) == 2:
			return []byte(settings[args[1]] + "\n"), nil
		case name == "postconf" && args[0] == "-h":
			var out strings.Builder
			for _, key := range args[1:] {
				out.WriteString(settings[key] + "\n")
			}
			return []byte(out.String()), nil
		case name == "postconf" && args[0] == "-e":
			key, value, _ := strings.Cut(args[1], "=")
			settings[key] = value
			return nil, nil
		}
		t.Fatalf("unexpected command %s %v", name, args)
		return nil, nil
	}
}

func TestEnsurePostfixServerCertificateUsesManagedPairBeforeFirstStart(t *testing.T) {
	ctx := context.Background()
	// Debian: the snakeoil pair is replaced by the trusted managed pair.
	settings := map[string]string{"smtpd_tls_cert_file": debianSnakeoilCert, "smtpd_tls_key_file": "/etc/ssl/private/ssl-cert-snakeoil.key"}
	managed, err := ensurePostfixServerCertificate(ctx, fakePostconf(t, settings), testManagedMailCert, testManagedMailKey)
	if err != nil || !managed || settings["smtpd_tls_cert_file"] != testManagedMailCert ||
		settings["smtpd_tls_key_file"] != testManagedMailKey || settings["smtpd_tls_security_level"] != "may" {
		t.Fatalf("debian: managed=%v err=%v settings=%v", managed, err, settings)
	}
	// Re-run: already managed, nothing written.
	written := 0
	run := fakePostconf(t, settings)
	counting := func(name string, args ...string) ([]byte, error) {
		if args[0] == "-e" {
			written++
		}
		return run(name, args...)
	}
	if managed, err := ensurePostfixServerCertificate(ctx, counting, testManagedMailCert, testManagedMailKey); err != nil || !managed || written != 0 {
		t.Fatalf("rerun: managed=%v err=%v written=%d", managed, err, written)
	}
	// The owner's certificate is never touched.
	owner := map[string]string{"smtpd_tls_cert_file": "/etc/ssl/owner/mail.pem"}
	if managed, err := ensurePostfixServerCertificate(ctx, fakePostconf(t, owner), testManagedMailCert, testManagedMailKey); err != nil || managed || owner["smtpd_tls_cert_file"] != "/etc/ssl/owner/mail.pem" {
		t.Fatalf("owner: managed=%v err=%v settings=%v", managed, err, owner)
	}
	// Half a pair is refused.
	if _, err := ensurePostfixServerCertificate(ctx, fakePostconf(t, map[string]string{}), testManagedMailCert, ""); err == nil {
		t.Fatal("half a pair accepted")
	}
	// A cancelled lease changes nothing.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	fresh := map[string]string{}
	if _, err := ensurePostfixServerCertificate(cancelled, fakePostconf(t, fresh), testManagedMailCert, testManagedMailKey); err == nil || len(fresh) != 0 {
		t.Fatalf("cancelled: err=%v settings=%v", err, fresh)
	}
}

// Without a managed pair the earlier behavior stays: Arch gets the default.
func TestEnsurePostfixServerCertificateDefaultWithoutManagedPair(t *testing.T) {
	dir := t.TempDir()
	oldCert, oldKey, oldCreate := postfixDefaultCertPath, postfixDefaultKeyPath, postfixDefaultPairCreate
	t.Cleanup(func() {
		postfixDefaultCertPath, postfixDefaultKeyPath, postfixDefaultPairCreate = oldCert, oldKey, oldCreate
	})
	postfixDefaultCertPath, postfixDefaultKeyPath = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	postfixDefaultPairCreate = func(cert, key, _ string) error {
		for _, path := range []string{cert, key} {
			if err := os.WriteFile(path, []byte("pem"), 0o600); err != nil {
				return err
			}
		}
		return nil
	}
	settings := map[string]string{}
	managed, err := ensurePostfixServerCertificate(context.Background(), fakePostconf(t, settings), "", "")
	if err != nil || managed || settings["smtpd_tls_cert_file"] != postfixDefaultCertPath {
		t.Fatalf("arch: managed=%v err=%v settings=%v", managed, err, settings)
	}
	debian := map[string]string{"smtpd_tls_cert_file": debianSnakeoilCert}
	if managed, err := ensurePostfixServerCertificate(context.Background(), fakePostconf(t, debian), "", ""); err != nil || managed || debian["smtpd_tls_cert_file"] != debianSnakeoilCert {
		t.Fatalf("debian without managed: managed=%v err=%v settings=%v", managed, err, debian)
	}
}

func TestMailStackHostCertificateSelection(t *testing.T) {
	old := mailStackSelectHostCertificate
	t.Cleanup(func() { mailStackSelectHostCertificate = old })
	for name, tc := range map[string]struct {
		name       string
		cert, key  string
		err        error
		wantCert   string
		wantNoteIn string
	}{
		"no name":            {name: ""},
		"not canonical":      {name: "Mail.Example.Test", wantNoteIn: "not a canonical name"},
		"none published":     {name: "mail.example.test", cert: defaultMailCert, key: defaultMailKey},
		"published, current": {name: "mail.example.test", cert: testManagedMailCert, key: testManagedMailKey, wantCert: testManagedMailCert},
		"other identity":     {name: "mail.example.test", err: errors.New("configured host certificate belongs to another mail identity"), wantNoteIn: "another mail identity"},
	} {
		mailStackSelectHostCertificate = func(string) (string, string, error) { return tc.cert, tc.key, tc.err }
		cert, key, note := mailStackHostCertificate(tc.name)
		if cert != tc.wantCert || (cert == "") != (key == "") {
			t.Errorf("%s: cert=%q key=%q", name, cert, key)
		}
		if tc.wantNoteIn == "" && note != "" || tc.wantNoteIn != "" && !strings.Contains(note, tc.wantNoteIn) {
			t.Errorf("%s: note %q", name, note)
		}
	}
}

func TestMailStackHostCertificateState(t *testing.T) {
	const name = "mail.example.test"
	for label, tc := range map[string]struct {
		name, managed                    string
		postfix, pfManaged, dovecot, dvM bool
		want                             string
	}{
		"no name given":        {"", testManagedMailCert, true, true, true, true, ""},
		"no managed pair":      {name, "", true, false, true, false, transport.MailHostCertificateDefault},
		"both managed":         {name, testManagedMailCert, true, true, true, true, transport.MailHostCertificateManaged},
		"postfix only managed": {name, testManagedMailCert, true, true, false, false, transport.MailHostCertificateManaged},
		"owner postfix cert":   {name, testManagedMailCert, true, false, true, true, transport.MailHostCertificateDefault},
		"dovecot not managed":  {name, testManagedMailCert, true, true, true, false, transport.MailHostCertificateDefault},
	} {
		if got := mailStackHostCertificateState(tc.name, tc.managed, tc.postfix, tc.pfManaged, tc.dovecot, tc.dvM); got != tc.want {
			t.Errorf("%s: %q, want %q", label, got, tc.want)
		}
	}
}

func TestMailTLSHostCertificateState(t *testing.T) {
	old := mailStackSelectHostCertificate
	t.Cleanup(func() { mailStackSelectHostCertificate = old })
	for label, tc := range map[string]struct {
		cert string
		err  error
		want string
	}{
		"managed":    {cert: testManagedMailCert, want: transport.MailHostCertificateManaged},
		"default":    {cert: defaultMailCert, want: transport.MailHostCertificateDefault},
		"unreadable": {err: errors.New("unreadable"), want: ""},
	} {
		mailStackSelectHostCertificate = func(string) (string, string, error) { return tc.cert, "k", tc.err }
		if got := mailTLSHostCertificateState("mail.example.test"); got != tc.want {
			t.Errorf("%s: %q, want %q", label, got, tc.want)
		}
	}
}

func TestEnsureDovecotHostCertificateWithoutManagedPairDoesNothing(t *testing.T) {
	managed, err := ensureDovecotHostCertificate("mail.example.test", "", func(string, ...string) ([]byte, error) {
		t.Fatal("no command expected")
		return nil, nil
	})
	if err != nil || managed {
		t.Fatalf("managed=%v err=%v", managed, err)
	}
}
