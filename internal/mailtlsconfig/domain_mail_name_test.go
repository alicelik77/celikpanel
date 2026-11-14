package mailtlsconfig

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.134 (docs/CERTIFICATE-COVERAGE.md §3, §5): serving a domain's
// certificate for mail.<domain> adds one name to Postfix's SNI map and one
// Dovecot local_name block, both naming certbot's stable live files of the
// site's lineage. The default certificate of every mail port, which the
// server's own mail name uses, is unchanged; and the live directory is what
// the native deploy hook looks for after a renewal, so renewal reaches mail
// with no CelikPanel program.
// Posta adı için sertifika sunmak varsayılan sertifikayı değiştirmez; yenileme
// Panel olmadan postaya ulaşır.
func TestDomainMailNameBlocksLeaveTheServerNameAlone(t *testing.T) {
	lineage := "celikpanel-site-42-" + strings.Repeat("ab", 12)
	fullchain, privkey, _ := transport.NativeSiteLineagePaths(lineage)
	live := "/etc/letsencrypt/live/" + lineage + "/"
	sni := []transport.MailSNIEntry{{Names: []string{"mail.example.test", "example.test"}, CertPath: fullchain, KeyPath: privkey}}
	for _, modern := range []bool{false, true} {
		without := Dovecot(modern, fixtureCert, fixtureKey, nil)
		with := Dovecot(modern, fixtureCert, fixtureKey, sni)
		if !strings.HasPrefix(with, without) {
			t.Fatalf("the default part changed (modern=%t):\n%s\n---\n%s", modern, without, with)
		}
		added := strings.TrimPrefix(with, without)
		if strings.Count(added, "local_name ") != 2 || !strings.Contains(added, "local_name mail.example.test {") ||
			strings.Contains(added, "mail.panel.test") || strings.Count(added, live) != 4 {
			t.Fatalf("per-name blocks (modern=%t):\n%s", modern, added)
		}
		for _, line := range strings.Split(added, "\n") {
			if strings.Contains(line, "/") && !strings.Contains(line, live) {
				t.Fatalf("a per-name block names a file outside the lineage: %q", line)
			}
		}
	}
	for _, setting := range PostfixSettings("mail.panel.test", fixtureCert, fixtureKey) {
		if strings.Contains(setting[1], "letsencrypt") || strings.Contains(setting[1], "example.test") {
			t.Fatalf("a default Postfix setting follows the domain: %v", setting)
		}
	}
	mapLines := strings.Split(strings.TrimSpace(string(PostfixSNI(sni))), "\n")
	if len(mapLines) != 3 || mapLines[1] != "mail.example.test "+privkey+" "+fullchain ||
		mapLines[2] != "example.test "+privkey+" "+fullchain {
		t.Fatalf("SNI map = %q", mapLines)
	}
	if strings.Contains(string(PostfixSNI(sni)), "mail.panel.test") {
		t.Fatal("the server's mail name entered the SNI map")
	}
}
