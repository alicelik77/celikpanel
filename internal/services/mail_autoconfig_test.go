package services

import (
	"encoding/xml"
	"strings"
	"testing"
)

func mailAutoconfigTestData(project string, ssl bool) VhostData {
	data := VhostData{
		SiteID:            41,
		Domain:            "example.test",
		ServerNames:       []string{"example.test", "www.example.test"},
		ACMEChallengeRoot: testACMEChallengeRoot,
		DocumentRoot:      "/srv/example.test/public_html",
		ProjectType:       project,
		PHPSocket:         "/run/php/php8.3-fpm-site41.sock",
		SSLType:           "none",
		ForwardTo:         "https://other.example",
	}
	if ssl {
		data.SSLType = "custom"
		data.SSLCert = "/etc/celikpanel/certs/example.test/cert.pem"
		data.SSLKey = "/etc/celikpanel/certs/example.test/key.pem"
	}
	return data
}

func renderTest(t *testing.T, data VhostData) string {
	t.Helper()
	ng, err := NewNginxGenerator()
	if err != nil {
		t.Fatal(err)
	}
	out, err := ng.Render(data)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(out, "\r\n", "\n")
}

// A site whose domain has no mail here renders exactly as before.
func TestMailAutoconfigAbsentLeavesTheSiteFileAsItWas(t *testing.T) {
	for _, project := range []string{"php", "static", "forwarding"} {
		for _, ssl := range []bool{false, true} {
			out := renderTest(t, mailAutoconfigTestData(project, ssl))
			if strings.Contains(out, "autoconfig") || strings.Contains(out, "Autodiscover") {
				t.Fatalf("%s ssl=%v: mail set-up text in a site without it\n%s", project, ssl, out)
			}
		}
	}
}

func TestMailAutoconfigAnswersThunderbirdAndOutlookWithoutSecrets(t *testing.T) {
	for _, project := range []string{"php", "static", "forwarding"} {
		data := mailAutoconfigTestData(project, true)
		data.MailAutoconfig = &MailAutoconfig{MailHost: "Mail.Server.Example.", TLS: true}
		out := renderTest(t, data)
		if got := strings.Count(out, "server_name autoconfig.example.test autodiscover.example.test;"); got != 2 {
			t.Fatalf("%s: want the HTTP and HTTPS mail set-up servers, got %d\n%s", project, got, out)
		}
		// The site's own blocks answer /.well-known/autoconfig (Thunderbird's
		// second address) and the set-up servers answer all three.
		if got := strings.Count(out, "location = /.well-known/autoconfig/mail/config-v1.1.xml {"); got != 4 {
			t.Fatalf("%s: .well-known answers = %d\n%s", project, got, out)
		}
		if got := strings.Count(out, `location ~* ^/autodiscover/autodiscover\.xml$ {`); got != 2 {
			t.Fatalf("%s: autodiscover answers = %d", project, got)
		}
		for _, forbidden := range []string{"password>", "LoginName", "$", "<EMailAddress>"} {
			for _, line := range strings.Split(out, "\n") {
				if strings.Contains(line, "return 200 '") && strings.Contains(line, forbidden) {
					t.Fatalf("%s: answer carries %q: %s", project, forbidden, line)
				}
			}
		}
		if !strings.Contains(out, "<hostname>mail.server.example</hostname><port>993</port><socketType>SSL</socketType>") ||
			!strings.Contains(out, "<port>587</port><socketType>STARTTLS</socketType>") ||
			!strings.Contains(out, "<username>%EMAILADDRESS%</username>") {
			t.Fatalf("%s: Thunderbird answer\n%s", project, out)
		}
		if !strings.Contains(out, "<Type>IMAP</Type><Server>mail.server.example</Server><Port>993</Port>") {
			t.Fatalf("%s: Outlook answer\n%s", project, out)
		}
		// The set-up servers keep certificate validation and serve nothing else.
		start := strings.Index(out, "server_name autoconfig.example.test")
		block := out[start : start+strings.Index(out[start:], "\n}\n")]
		if !strings.Contains(block, "include ") || strings.Contains(block, "root /srv/") || strings.Contains(block, "fastcgi_pass") || strings.Contains(block, "proxy_pass") {
			t.Fatalf("%s: set-up server block\n%s", project, block)
		}
	}
}

func TestMailAutoconfigAnswersAreWellFormedXML(t *testing.T) {
	data := mailAutoconfigTestData("static", false)
	data.MailAutoconfig = &MailAutoconfig{MailHost: "mail.example.test"}
	prepared, err := prepareVhostData(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{prepared.MailAutoconfig.ThunderbirdXML, prepared.MailAutoconfig.AutodiscoverXML} {
		decoder := xml.NewDecoder(strings.NewReader(text))
		for {
			if _, err := decoder.Token(); err != nil {
				if err.Error() == "EOF" {
					break
				}
				t.Fatalf("not well-formed: %v\n%s", err, text)
			}
		}
	}
}

func TestMailAutoconfigWithoutCertificateServesHTTPOnly(t *testing.T) {
	data := mailAutoconfigTestData("php", false)
	data.MailAutoconfig = &MailAutoconfig{MailHost: "mail.example.test", TLS: true}
	out := renderTest(t, data)
	if got := strings.Count(out, "server_name autoconfig.example.test autodiscover.example.test;"); got != 1 {
		t.Fatalf("want only the HTTP set-up server, got %d\n%s", got, out)
	}
	data.Suspended = true
	if out := renderTest(t, data); strings.Contains(out, "autoconfig.example.test") {
		t.Fatalf("a suspended site answers mail set-up\n%s", out)
	}
}

func TestMailAutoconfigRefusesUnsafeInput(t *testing.T) {
	for name, mutate := range map[string]func(*VhostData){
		"host quote":    func(d *VhostData) { d.MailAutoconfig.MailHost = "mail'.example.test" },
		"host variable": func(d *VhostData) { d.MailAutoconfig.MailHost = "$host" },
		"host short":    func(d *VhostData) { d.MailAutoconfig.MailHost = "mail" },
		"name taken":    func(d *VhostData) { d.ServerNames = append(d.ServerNames, "autoconfig.example.test") },
		"acme taken":    func(d *VhostData) { d.ACMEChallengeNames = []string{"autodiscover.example.test"} },
	} {
		data := mailAutoconfigTestData("php", true)
		data.MailAutoconfig = &MailAutoconfig{MailHost: "mail.example.test", TLS: true}
		mutate(&data)
		if _, err := prepareVhostData(data); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
}
