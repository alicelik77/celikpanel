package main

import (
	"context"
	"errors"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// debian13Stock10Mail is every non-comment line of Debian 13's
// /etc/dovecot/conf.d/10-mail.conf (dovecot-core 2.4.1-4), measured in a
// clean debian:13 container on 30 Oct 2026.
const debian13Stock10Mail = `mail_driver = mbox
mail_home = /home/%{user | username}
mail_path = %{home}/mail
mail_inbox_path = /var/mail/%{user}
namespace inbox {
  inbox = yes
}
mail_privileged_group = mail
protocol !indexer-worker {
}
`

var dovecotTopLevelSetting = regexp.MustCompile(`^([a-z_]+)\s*=\s*(.*)$`)

// effectiveDovecotTopLevel models Dovecot 2.4's rule for global settings in
// files read in order: the later value wins. Lines inside blocks are skipped.
func effectiveDovecotTopLevel(files ...string) map[string]string {
	settings := map[string]string{}
	for _, file := range files {
		depth := 0
		for _, line := range strings.Split(file, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!") {
				continue
			}
			if strings.HasSuffix(trimmed, "{") {
				depth++
				continue
			}
			if trimmed == "}" {
				depth--
				continue
			}
			if depth != 0 {
				continue
			}
			if match := dovecotTopLevelSetting.FindStringSubmatch(trimmed); match != nil {
				settings[match[1]] = strings.TrimSpace(match[2])
			}
		}
	}
	return settings
}

// D1: on Debian 13 the virtual users must get INBOX = the maildir Postfix
// delivers into, not Debian's /var/mail spool; the same on Arch's main file.
func TestDovecot24DropInOverridesStockMailLocation(t *testing.T) {
	for _, root := range []string{"/var/mail/vhosts", "/var/spool/mail/vhosts"} {
		want := root + "/%{user | domain}/%{user | username}"
		dropIn := buildDovecotVirtualConfAt(true, root)
		layouts := map[string][]string{
			"debian-13 stock conf.d": {debian13Stock10Mail, dropIn},
			"arch main file":         {string(renderArchDovecotMain("2.4.0", "2.4.0")), dropIn},
		}
		for layout, files := range layouts {
			got := effectiveDovecotTopLevel(files...)
			for _, key := range []string{"mail_home", "mail_path", "mail_inbox_path"} {
				if got[key] != want {
					t.Errorf("%s, root %s: effective %s = %q, want %q", layout, root, key, got[key], want)
				}
			}
			if got["mail_driver"] != "maildir" {
				t.Errorf("%s: effective mail_driver = %q", layout, got["mail_driver"])
			}
			if got["mail_uid"] != vmailUID || got["mail_gid"] != vmailGID || got["first_valid_uid"] != vmailUID {
				t.Errorf("%s: effective ids = %q/%q/%q", layout, got["mail_uid"], got["mail_gid"], got["first_valid_uid"])
			}
		}
	}
	// The text every release up to alpha.112 wrote reproduces the defect in
	// this model: Debian's spool INBOX leaked through.
	earlier := earlierDovecotVirtualTexts("/var/mail/vhosts")[1]
	if got := effectiveDovecotTopLevel(debian13Stock10Mail, earlier)["mail_inbox_path"]; got != "/var/mail/%{user}" {
		t.Fatalf("model does not reproduce the alpha.112 defect: mail_inbox_path = %q", got)
	}
}

func TestDovecotVirtualDropInEachMailSettingOnce(t *testing.T) {
	conf := buildDovecotVirtualConfAt(true, "/var/mail/vhosts")
	for _, key := range []string{"mail_driver", "mail_home", "mail_path", "mail_inbox_path", "mail_uid", "mail_gid", "first_valid_uid"} {
		if n := len(regexp.MustCompile(`(?m)^`+key+` = `).FindAllString(conf, -1)); n != 1 {
			t.Errorf("%s appears %d times:\n%s", key, n, conf)
		}
	}
	// The 2.3 dialect keeps its single mail_location (INBOX is part of it)
	// and gains no 2.4 setting.
	v23 := buildDovecotVirtualConfAt(false, "/var/mail/vhosts")
	if !strings.Contains(v23, "\nmail_location = maildir:/var/mail/vhosts/%d/%n\n") {
		t.Fatalf("2.3 mail_location changed:\n%s", v23)
	}
	for _, forbid := range []string{"mail_inbox_path", "mail_home", "mail_path"} {
		if strings.Contains(v23, forbid) {
			t.Fatalf("2.3 dialect must not contain %q:\n%s", forbid, v23)
		}
	}
}

func TestDovecotVirtualDropInCarriesItsDigest(t *testing.T) {
	for _, modern := range []bool{false, true} {
		conf := buildDovecotVirtualConfAt(modern, "/var/mail/vhosts")
		if !strings.HasPrefix(conf, dovecotVirtualHeaderStart) || !dovecotVirtualManagedIntact([]byte(conf)) {
			t.Fatalf("modern=%v: header digest does not match:\n%s", modern, conf)
		}
		edited := strings.Replace(conf, "mail_uid = 5000", "mail_uid = 5001", 1)
		if dovecotVirtualManagedIntact([]byte(edited)) {
			t.Fatalf("modern=%v: an edited body still counts as CelikPanel's", modern)
		}
	}
}

// The texts earlier releases wrote are frozen byte for byte; the 2.4 one is
// the 504-byte file the final alpha.113 acceptance listed on Debian 13.
func TestEarlierDovecotVirtualTextsAreFrozen(t *testing.T) {
	texts := earlierDovecotVirtualTexts("/var/mail/vhosts")
	if len(texts) != 2 {
		t.Fatalf("texts = %d", len(texts))
	}
	if got := len(texts[1]); got != 504 {
		t.Fatalf("earlier 2.4 text is %d bytes, the acceptance listed 504", got)
	}
	if !strings.HasPrefix(texts[0], "# Managed by CelikPanel — do not edit by hand / elle düzenlemeyin\nmail_location = maildir:/var/mail/vhosts/%d/%n\n") {
		t.Fatalf("earlier 2.3 text changed:\n%s", texts[0])
	}
	if !strings.HasPrefix(texts[1], "# Managed by CelikPanel — do not edit by hand / elle düzenlemeyin\nmail_driver = maildir\nmail_path = /var/mail/vhosts/%{user | domain}/%{user | username}\nmail_uid = 5000\n") {
		t.Fatalf("earlier 2.4 text changed:\n%s", texts[1])
	}
}

func TestClassifyDovecotVirtualDropIn(t *testing.T) {
	root := "/var/spool/mail/vhosts"
	roots := dovecotVirtualEarlierRoots(root)
	current := buildDovecotVirtualConfAt(true, root)
	cases := []struct {
		name    string
		content string
		exists  bool
		want    dovecotVirtualDropInState
	}{
		{"absent", "", false, dovecotVirtualDropInAbsent},
		{"current 2.4", current, true, dovecotVirtualDropInManaged},
		{"current 2.3", buildDovecotVirtualConfAt(false, root), true, dovecotVirtualDropInManaged},
		{"earlier 2.4, resolved root", earlierDovecotVirtualTexts(root)[1], true, dovecotVirtualDropInEarlier},
		{"earlier 2.3, resolved root", earlierDovecotVirtualTexts(root)[0], true, dovecotVirtualDropInEarlier},
		{"earlier 2.4, product root", earlierDovecotVirtualTexts(mailRootDir)[1], true, dovecotVirtualDropInEarlier},
		{"earlier 2.4 plus an owner line", earlierDovecotVirtualTexts(root)[1] + "mail_inbox_path = /srv/inbox\n", true, dovecotVirtualDropInOwnerEdited},
		{"earlier 2.4 for another root", earlierDovecotVirtualTexts("/srv/mail")[1], true, dovecotVirtualDropInOwnerEdited},
		{"current with an edited body", strings.Replace(current, "first_valid_uid = 5000", "first_valid_uid = 1000", 1), true, dovecotVirtualDropInOwnerEdited},
		{"owner's own file", "mail_driver = sdbox\nmail_path = ~/mail\n", true, dovecotVirtualDropInOwnerEdited},
		{"empty file", "", true, dovecotVirtualDropInOwnerEdited},
	}
	for _, tc := range cases {
		if got := classifyDovecotVirtualDropIn([]byte(tc.content), tc.exists, roots); got != tc.want {
			t.Errorf("%s: state = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestDovecotVirtualOwnerEditedReasonNamesActorAndResume(t *testing.T) {
	for _, want := range []string{
		"left /etc/dovecot/conf.d/99-celikpanel.conf unchanged",
		"server administrator",
		"runs the same mail setup again",
		"sudo mv /etc/dovecot/conf.d/99-celikpanel.conf /etc/dovecot/conf.d/99-celikpanel.conf.mine",
		"/etc/dovecot/local.conf",
	} {
		if !strings.Contains(dovecotVirtualOwnerEditedReason, want) {
			t.Errorf("reason lacks %q", want)
		}
	}
}

// ------------------------------------------------- Postfix map-type probe ---

// `postconf -m` output measured on 30 Oct 2026 in clean containers: Debian 13
// (postfix 3.10.13, no postfix-lmdb) and Arch (postfix 3.11.7, lmdb).
const (
	debian13PostconfM = `btree
cidr
environ
fail
hash
inline
internal
memcache
nis
pipemap
proxy
randmap
regexp
socketmap
static
tcp
texthash
unionmap
unix
`
	archPostconfM = `btree
cidr
debug
environ
fail
hash
inline
internal
lmdb
memcache
nis
pipemap
proxy
randmap
regexp
socketmap
static
tcp
texthash
unionmap
unix
`
)

func withMapTypeProbe(t *testing.T, list func(context.Context) ([]byte, error), usable map[string]bool) *[]string {
	t.Helper()
	previousList, previousUsable := postfixMapTypeList, postfixMapTypeUsable
	tried := []string{}
	postfixMapTypeList = list
	postfixMapTypeUsable = func(_ context.Context, mapType string) bool {
		tried = append(tried, mapType)
		return usable[mapType]
	}
	mapTypeOnce = sync.Once{}
	mapTypeCached = ""
	t.Cleanup(func() {
		postfixMapTypeList, postfixMapTypeUsable = previousList, previousUsable
		mapTypeOnce = sync.Once{}
		mapTypeCached = ""
	})
	return &tried
}

// The error-level journal line on Debian came from trying lmdb with postmap;
// a type Postfix does not list is never tried.
func TestPostfixMapTypeNeverTriesATypePostfixDoesNotList(t *testing.T) {
	tried := withMapTypeProbe(t, func(context.Context) ([]byte, error) { return []byte(debian13PostconfM), nil },
		map[string]bool{"lmdb": true, "hash": true})
	if got := postfixMapTypeContext(context.Background()); got != "hash" {
		t.Fatalf("map type = %q, want hash", got)
	}
	if strings.Join(*tried, ",") != "hash" {
		t.Fatalf("postmap tried %v, want only hash", *tried)
	}
}

// A listed type is still tried for real: Arch lists hash without a usable
// Berkeley DB, and lmdb stays first where it works.
func TestPostfixMapTypeStillTriesListedTypes(t *testing.T) {
	tried := withMapTypeProbe(t, func(context.Context) ([]byte, error) { return []byte(archPostconfM), nil },
		map[string]bool{"lmdb": true})
	if got := postfixMapTypeContext(context.Background()); got != "lmdb" || strings.Join(*tried, ",") != "lmdb" {
		t.Fatalf("map type = %q after %v, want lmdb after lmdb", got, *tried)
	}

	tried = withMapTypeProbe(t, func(context.Context) ([]byte, error) { return []byte(archPostconfM), nil },
		map[string]bool{"btree": true})
	if got := postfixMapTypeContext(context.Background()); got != "btree" || strings.Join(*tried, ",") != "lmdb,hash,btree" {
		t.Fatalf("map type = %q after %v, want btree after lmdb,hash,btree", got, *tried)
	}
}

func TestPostfixMapTypeUnreadableListTriesEveryTypeAsBefore(t *testing.T) {
	tried := withMapTypeProbe(t, func(context.Context) ([]byte, error) { return nil, errors.New("postconf: not found") },
		map[string]bool{})
	if got := postfixMapTypeContext(context.Background()); got != "texthash" {
		t.Fatalf("map type = %q, want texthash", got)
	}
	if strings.Join(*tried, ",") != "lmdb,hash,btree" {
		t.Fatalf("tried %v, want every indexed type", *tried)
	}
	// Nothing indexed listed at all: texthash with no postmap trial.
	tried = withMapTypeProbe(t, func(context.Context) ([]byte, error) { return []byte("cidr\nstatic\ntexthash\n"), nil },
		map[string]bool{"hash": true})
	if got := postfixMapTypeContext(context.Background()); got != "texthash" || len(*tried) != 0 {
		t.Fatalf("map type = %q after %v, want texthash with no trial", got, *tried)
	}
}

func TestPostfixTLSMapProbeNeverTriesATypePostfixDoesNotList(t *testing.T) {
	var postmaps []string
	run := func(name string, args ...string) ([]byte, error) {
		switch name {
		case "postconf":
			if strings.Join(args, " ") != "-m" {
				t.Fatalf("unexpected postconf %v", args)
			}
			return []byte(debian13PostconfM), nil
		case "postmap":
			mapType, probe, _ := strings.Cut(args[len(args)-1], ":")
			postmaps = append(postmaps, mapType)
			if mapType == "hash" {
				return nil, os.WriteFile(probe+".db", []byte("index"), 0o600)
			}
			return []byte("postmap: fatal: unsupported map type: " + mapType), errors.New("exit status 1")
		}
		t.Fatalf("unexpected command %s %v", name, args)
		return nil, nil
	}
	mapType, err := probePostfixTLSMapType(run)
	if err != nil || mapType != "hash" {
		t.Fatalf("map type = %q, err = %v", mapType, err)
	}
	if strings.Join(postmaps, ",") != "hash" {
		t.Fatalf("postmap tried %v, want only hash", postmaps)
	}
}
