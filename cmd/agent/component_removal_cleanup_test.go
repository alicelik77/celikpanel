package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// alpha.130 (no leftovers): a mail component's removal takes the files
// CelikPanel wrote for it, by the same ownership rule each writer uses, and
// keeps (and names) the ones the owner changed.

func TestMailRemovalCleanupNamesCelikPanelsOwnFiles(t *testing.T) {
	paths := func(c componentRemovalCleanup) []string {
		var out []string
		for _, file := range c.files {
			out = append(out, file.path)
		}
		return append(out, c.dirs...)
	}
	postfix := strings.Join(paths(removalCleanupFor("postfix", "apt")), " ")
	for _, want := range []string{
		"/etc/postfix/celikpanel_transport", "/etc/postfix/celikpanel_transport.lmdb",
		"/etc/postfix/celikpanel_domain_mail_options.db", "/etc/postfix/celikpanel_login_map",
		"/etc/postfix/celikpanel_sni", "/etc/postfix/celikpanel_sender_access.lmdb",
		"/etc/postfix/celikpanel-relay-sasl", "/etc/postfix/celikpanel-relay-sasl.db",
		"/etc/postfix/celikpanel-subaddressing",
	} {
		if !strings.Contains(postfix, want) {
			t.Errorf("postfix cleanup misses %s", want)
		}
	}
	if strings.Contains(postfix, "/etc/postfix/vmailbox") || strings.Contains(postfix, "/etc/postfix/virtual") || strings.Contains(postfix, "/etc/postfix/main.cf") {
		t.Errorf("postfix cleanup names a file that is not CelikPanel's alone: %s", postfix)
	}
	dovecot := removalCleanupFor("dovecot", "apt")
	if got := strings.Join(paths(dovecot), " "); !strings.Contains(got, "celikpanel-quota-warning.timer") ||
		!strings.Contains(got, "99-celikpanel.conf") || !strings.Contains(got, "96-celikpanel-sieve.conf") ||
		!strings.Contains(got, "98-celikpanel-tls.conf") || !strings.Contains(got, "/var/lib/celikpanel-quota-warning") ||
		!strings.Contains(got, "/etc/dovecot/users") {
		t.Errorf("dovecot cleanup = %s", got)
	}
	if dovecot.files[0].unit != "celikpanel-quota-warning.timer" {
		t.Error("the timer is turned off first")
	}
	apt := strings.Join(paths(removalCleanupFor("postfwd", "apt")), " ")
	pacman := strings.Join(paths(removalCleanupFor("postfwd", "pacman")), " ")
	if !strings.Contains(apt, "postfwd.service.d/celikpanel.conf") || !strings.Contains(apt, "/var/lib/celikpanel-postfwd") || !strings.Contains(apt, "/etc/postfix/celikpanel-postfwd") ||
		!strings.Contains(pacman, "/etc/postfwd/celikpanel") || strings.Contains(pacman, "/var/lib/celikpanel-postfwd") {
		t.Errorf("postfwd cleanup apt=%s pacman=%s", apt, pacman)
	}
	if c := removalCleanupFor("redis", "apt"); len(c.files)+len(c.dirs) != 0 {
		t.Error("a component without CelikPanel files has nothing to clean")
	}
}

func TestRemovalCleanupRemovesOnlyWhatIsStillCelikPanels(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	timer := write("systemd/celikpanel-quota-warning.timer", renderQuotaWarningTimer())
	service := write("systemd/celikpanel-quota-warning.service", renderQuotaWarningService()+"# owner's line\n")
	sender := write("postfix/celikpanel_sender_access", removalPostfixSenderMarker+"\n# celikpanel-body-sha256: x\nbad@example.com 550\n")
	senderIndex := write("postfix/celikpanel_sender_access.lmdb", "index")
	ownMap := write("postfix/celikpanel_domain_mail_options", "example.com OK\n")
	ownIndex := write("postfix/celikpanel_domain_mail_options.db", "index")
	orphanIndex := write("postfix/celikpanel_transport.lmdb", "index of a map that is gone")
	tls := write("dovecot/conf.d/98-celikpanel-tls.conf", "# whatever CelikPanel wrote last\n")
	state := filepath.Join(root, "state")
	os.MkdirAll(filepath.Join(state, "markers"), 0o700)
	write("state/markers/a@example.com", "")
	cleanup := componentRemovalCleanup{
		files: []ownedRemovalFile{
			{path: timer, owned: quotaWarningIntact, unit: "celikpanel-quota-warning.timer"},
			{path: service, owned: quotaWarningIntact, unit: "celikpanel-quota-warning.service"},
			{path: filepath.Join(root, "absent.conf"), owned: nil},
			{path: tls},
		},
		dirs:    []string{state, filepath.Join(root, "no-such-dir")},
		parents: []string{filepath.Join(root, "dovecot", "conf.d"), filepath.Join(root, "dovecot")},
	}
	cleanup.files = append(cleanup.files, postfixMapAt(filepath.Join(root, "postfix", "celikpanel_sender_access"), startsWithLine(removalPostfixSenderMarker))...)
	cleanup.files = append(cleanup.files, postfixMapAt(filepath.Join(root, "postfix", "celikpanel_domain_mail_options"), startsWithLine(domainOptionsMarker))...)
	cleanup.files = append(cleanup.files, postfixMapAt(filepath.Join(root, "postfix", "celikpanel_transport"), nil)...)
	var calls []string
	removed, kept, notes := runRemovalCleanup(context.Background(), cleanup, removalCleanupOps{systemctl: func(_ context.Context, args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		return nil
	}})
	if len(notes) != 0 {
		t.Fatalf("notes = %v", notes)
	}
	for _, gone := range []string{timer, tls, sender, senderIndex, orphanIndex, state, filepath.Join(root, "dovecot", "conf.d"), filepath.Join(root, "dovecot")} {
		if _, err := os.Lstat(gone); !os.IsNotExist(err) {
			t.Errorf("%s stayed", gone)
		}
	}
	if !reflect.DeepEqual(kept, []string{ownMap, ownIndex, service}) {
		t.Errorf("kept = %v", kept)
	}
	for _, path := range kept {
		if _, err := os.Lstat(path); err != nil {
			t.Errorf("owner's %s was removed", path)
		}
	}
	if !reflect.DeepEqual(calls, []string{"disable --now celikpanel-quota-warning.timer", "daemon-reload", "reset-failed celikpanel-quota-warning.timer"}) {
		t.Errorf("systemctl = %v", calls)
	}
	if detail := removalCleanupDetail(removed, kept, notes); !strings.Contains(detail, "kept because they were changed outside CelikPanel: "+ownMap) {
		t.Errorf("detail = %q", detail)
	}
}

func postfixMapAt(path string, owned func([]byte) bool) []ownedRemovalFile {
	return postfixMap(path, owned)
}

func TestPostfwdPackageConfigIsLeftToThePackageManager(t *testing.T) {
	layout, _ := outgoingLayoutFor("apt")
	cleanup := removalCleanupFor("postfwd", "apt")
	var config ownedRemovalFile
	for _, file := range cleanup.files {
		if file.path == layout.config {
			config = file
		}
	}
	owner := []byte("# my own rules\nid=R1; action=DUNNO\n")
	if config.owned(owner) || config.packages(owner) {
		t.Fatal("an owner's rule file is the owner's")
	}
}
