package services

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
)

const testACMEChallengeRoot = "/var/lib/celikpanel-agent/acme-http-01/subscriptions/7/domains/19"

// The vhost template is the panel's security boundary for anything sitting in
// a document root, so these are not style assertions — each one is a leak that
// reached production once (see AUTOPSY A6) or was found on the way there.
//
// vhost şablonu, doküman kökündeki her şey için panelin güvenlik sınırıdır;
// bu yüzden buradakiler biçim iddiası değildir — her biri ya üretime ulaşmış
// (bkz. AUTOPSY A6) ya da oraya giderken yakalanmış bir sızıntıdır.
func TestVhostServingRules(t *testing.T) {
	ng, err := NewNginxGenerator()
	if err != nil {
		t.Fatal(err)
	}

	render := func(t *testing.T, d VhostData) string {
		t.Helper()
		d.ACMEChallengeRoot = testACMEChallengeRoot
		out, err := ng.Render(d)
		if err != nil {
			t.Fatalf("render %s: %v", d.ProjectType, err)
		}
		return out
	}

	static := render(t, VhostData{SiteID: 1, Domain: "ornek.com", DocumentRoot: "/var/www/ornek", ProjectType: "static"})
	php := render(t, VhostData{SiteID: 2, Domain: "ornek.com", DocumentRoot: "/var/www/ornek", ProjectType: "php", PHPSocket: "/run/php/php8.3-fpm.sock"})

	// A static site has no PHP handler. Without an explicit deny, nginx hands
	// the file out as application/octet-stream — the browser downloads the
	// SOURCE, credentials included. Verified against live nginx: 403, and the
	// body is nginx's error page, not the file.
	// Statik sitenin PHP işleyicisi yoktur. Açık bir ret olmadan nginx dosyayı
	// application/octet-stream olarak verir — tarayıcı KAYNAĞI indirir, içindeki
	// bilgilerle birlikte. Canlı nginx'te doğrulandı: 403 ve gövde dosya değil,
	// nginx'in hata sayfası.
	if !strings.Contains(static, `location ~ \.(php|phtml|phps|php[0-9])$`) {
		t.Error("static vhost does not deny PHP files — the source would be served as a download")
	}
	// The PHP vhost must NOT carry that deny: there the files are meant to run.
	// PHP vhost'u o reddi TAŞIMAMALI: orada dosyaların çalışması beklenir.
	if strings.Contains(php, `location ~ \.(php|phtml|phps|php[0-9])$`) {
		t.Error("php vhost denies PHP files — the site could never run")
	}
	if !strings.Contains(php, "fastcgi_pass unix:/run/php/php8.3-fpm.sock") {
		t.Error("php vhost does not pass to the FPM socket")
	}

	// A6, both halves: dotfiles are denied so .env/.git stay secret, but
	// .well-known is exempt or certbot --webroot can never validate — that
	// broke every issuance and renewal for static sites.
	// A6'nın iki yarısı: .env/.git gizli kalsın diye nokta dosyalar reddedilir,
	// ama .well-known muaftır; yoksa certbot --webroot hiç doğrulayamaz — bu,
	// statik sitelerin her sertifika alma ve yenilemesini kırmıştı.
	for name, conf := range map[string]string{"static": static, "php": php} {
		if !strings.Contains(conf, `location ~ /\.(?!well-known).*`) {
			t.Errorf("%s vhost: dotfile rule missing or not ACME-safe", name)
		}
	}
}

// A php vhost with no FPM socket renders `unix:` with no path and nginx
// refuses to load it — taking the whole web server down on reload, not just
// this site. Refusing to render is the honest failure.
// FPM soketi olmayan bir php vhost'u yolsuz `unix:` üretir ve nginx onu
// yüklemeyi reddeder — reload'da yalnız bu siteyi değil, tüm web sunucusunu
// düşürür. Üretmeyi reddetmek dürüst olan başarısızlıktır.
func TestPHPVhostRefusesWithoutSocket(t *testing.T) {
	ng, err := NewNginxGenerator()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ng.Render(VhostData{
		SiteID: 1, Domain: "ornek.com", ProjectType: "php",
		ACMEChallengeRoot: testACMEChallengeRoot,
	}); err == nil {
		t.Error("rendered a php vhost with no FPM socket — nginx would reject the config")
	}
}

// Asking for SSL is not the same as HAVING a certificate, and conflating the
// two took the panel down on a plain "add a domain" click: with the Let's
// Encrypt box ticked, GenerateVhost emitted the HTTPS server block while no
// certificate existed yet (it cannot — the domain has to resolve here first),
// so the template rendered `ssl_certificate ;` with no argument, `nginx -t`
// refused the WHOLE config and the operator got "internal server error"
// (biovision.health on Boston, 25 Jul).
//
// The second half of the trap is subtler and would have survived a naive fix:
// the same branch turned port 80 into a 301 to HTTPS, which redirects the ACME
// http-01 challenge away — so the certificate could never be obtained and the
// site would never leave this state on its own.
//
// SSL istemek, sertifikaya SAHİP olmakla aynı şey değildir; ikisini birbirine
// karıştırmak paneli düpedüz bir "alan adı ekle" tıklamasında düşürdü: Let's
// Encrypt kutusu işaretliyken GenerateVhost, henüz sertifika yokken (olamaz da
// — alan adının önce buraya çözülmesi gerekir) HTTPS sunucu bloğunu yazıyordu;
// şablon argümansız `ssl_certificate ;` üretiyor, `nginx -t` TÜM yapılandırmayı
// reddediyor ve operatör "sunucu hatası" alıyordu (Boston'da biovision.health,
// 25 Tem).
//
// Tuzağın ikinci yarısı daha sinsi ve naif bir düzeltmeden sağ çıkardı: aynı
// dal 80 portunu HTTPS'e 301 yapıyordu, bu da ACME http-01 doğrulamasını başka
// yere yönlendirir — yani sertifika hiçbir zaman alınamaz ve site bu durumdan
// kendi başına asla çıkamazdı.
func TestSSLRequestedWithoutCertificateStaysHTTP(t *testing.T) {
	ng, err := NewNginxGenerator()
	if err != nil {
		t.Fatal(err)
	}
	domain := &core.Domain{ID: 19, SubscriptionID: 7, Name: "biovision.health"}
	site := &core.Site{
		ID:           10,
		DocumentRoot: "/var/www/biovision.health",
		ProjectType:  "static",
		SSLEnabled:   true, // the operator ticked the box
		// SSLCertPath / SSLKeyPath are nil: nothing has been issued yet.
	}

	out, err := ng.GenerateVhost(site, domain, "")
	if err != nil {
		t.Fatalf("a domain must be creatable before its certificate exists: %v", err)
	}
	if strings.Contains(out, "ssl_certificate") {
		t.Error("no certificate exists yet — writing ssl_certificate makes nginx -t refuse the whole config")
	}
	if strings.Contains(out, "return 301 https://") {
		t.Error("redirecting to HTTPS before a certificate exists takes the site offline AND blocks the ACME challenge")
	}
	if !strings.Contains(out, "listen 80;") {
		t.Error("the site must still answer on port 80 — that is where the ACME challenge arrives")
	}

	// Once the certificate is on disk, the same site regenerates WITH HTTPS.
	// Sertifika diske indiğinde aynı site HTTPS İLE yeniden üretilir.
	cert, key := "/etc/letsencrypt/live/biovision.health/fullchain.pem", "/etc/letsencrypt/live/biovision.health/privkey.pem"
	site.SSLCertPath, site.SSLKeyPath = &cert, &key
	out, err = ng.GenerateVhost(site, domain, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ssl_certificate "+cert+";") {
		t.Error("with a certificate present the HTTPS block must be written")
	}
	if !strings.Contains(out, "return 301 https://") {
		t.Error("with HTTPS able to answer, port 80 should redirect to it")
	}
}

// The HTTP/2 form follows the installed nginx (2026-10-30). nginx >= 1.25.1
// deprecates `listen ... http2` (a warning on every test and reload) and has
// `http2 on;`; nginx 1.24, Ubuntu 24.04's, refuses `http2 on;` ([emerg]
// unknown directive "http2"), which left a certificate "activation pending"
// on a real server. An unknown version writes neither: every nginx accepts
// that.
// HTTP/2 biçimi kurulu nginx'e göre: 1.25.1 ve sonrası `http2 on;`, 1.24
// `listen ... ssl http2;`, bilinmeyen sürüm HTTP/2'siz.
func TestHTTPSListenFollowsTheInstalledNginx(t *testing.T) {
	data := VhostData{
		SiteID: 10, Domain: "biovision.health", DocumentRoot: "/var/www/biovision.health",
		ProjectType: "static", ACMEChallengeRoot: testACMEChallengeRoot, SSLType: "custom",
		SSLCert: "/etc/letsencrypt/live/biovision.health/fullchain.pem",
		SSLKey:  "/etc/letsencrypt/live/biovision.health/privkey.pem",
	}
	forwarding := data
	forwarding.ProjectType, forwarding.ForwardTo = "forwarding", "https://example.org"
	for _, tc := range []struct {
		name      string
		features  NginxFeatures
		directive bool
		listen    bool
	}{
		{"debian 13 nginx 1.26.3", NginxFeatures{Known: true, Version: NginxVersion{1, 26, 3}, HTTP2Module: true}, true, false},
		{"nginx 1.25.1", NginxFeatures{Known: true, Version: NginxVersion{1, 25, 1}, HTTP2Module: true}, true, false},
		{"nginx 1.25.0", NginxFeatures{Known: true, Version: NginxVersion{1, 25, 0}, HTTP2Module: true}, false, true},
		{"ubuntu 24.04 nginx 1.24.0", NginxFeatures{Known: true, Version: NginxVersion{1, 24, 0}, HTTP2Module: true}, false, true},
		{"built without http_v2", NginxFeatures{Known: true, Version: NginxVersion{1, 26, 3}}, false, false},
		{"version unknown", NginxFeatures{}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ng, err := NewNginxGenerator()
			if err != nil {
				t.Fatal(err)
			}
			ng.SetNginxFeatures(tc.features)
			for _, d := range []VhostData{data, forwarding} {
				out, err := ng.Render(d)
				if err != nil {
					t.Fatal(err)
				}
				if got := strings.Count(out, "\n    http2 on;\n"); (got == 1) != tc.directive || got > 1 {
					t.Errorf("%s: `http2 on;` %d times, want directive=%v:\n%s", d.ProjectType, got, tc.directive, out)
				}
				hasListen := strings.Contains(out, "\n    listen 443 ssl http2;\n    listen [::]:443 ssl http2;\n")
				if hasListen != tc.listen {
					t.Errorf("%s: listen http2 = %v, want %v", d.ProjectType, hasListen, tc.listen)
				}
				if !tc.listen && !strings.Contains(out, "\n    listen 443 ssl;\n    listen [::]:443 ssl;\n") {
					t.Errorf("%s: the plain HTTPS listen lines are missing", d.ProjectType)
				}
				if strings.Contains(out, "http2") && !tc.directive && !tc.listen {
					t.Errorf("%s: HTTP/2 written for %s", d.ProjectType, tc.name)
				}
			}
		})
	}
}

// Mozilla intermediate (guidelines 6.0) on every HTTPS server block; nothing
// the two supported OpenSSL series (3.0 on Ubuntu 24.04, 3.5 on Debian 13)
// would refuse, and no OCSP stapling (Let's Encrypt ended OCSP in 2025).
func TestHTTPSServerCarriesTheIntermediateTLSPolicy(t *testing.T) {
	ng, err := NewNginxGenerator()
	if err != nil {
		t.Fatal(err)
	}
	ng.SetNginxFeatures(NginxFeatures{Known: true, Version: NginxVersion{1, 24, 0}, HTTP2Module: true})
	out, err := ng.Render(VhostData{
		SiteID: 10, Domain: "a.example", DocumentRoot: "/var/www/a", ProjectType: "static",
		ACMEChallengeRoot: testACMEChallengeRoot, SSLType: "custom",
		SSLCert: "/etc/ssl/a.crt", SSLKey: "/etc/ssl/a.key",
	})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := NginxTLSPolicy()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "    ssl_session_cache shared:SSL:10m;\n"+policy+"\n") {
		t.Fatalf("the HTTPS block does not carry the policy lines as one piece:\n%s", out)
	}
	for _, want := range []string{
		"    ssl_protocols TLSv1.2 TLSv1.3;",
		"    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305;",
		"    ssl_prefer_server_ciphers off;",
		"    ssl_session_timeout 1d;",
		"    ssl_session_tickets off;",
	} {
		if !strings.Contains(policy, want) {
			t.Errorf("policy lacks %q", want)
		}
	}
	for _, refused := range []string{"HIGH:", ":DHE-", " DHE-", "ssl_stapling", "ssl_dhparam", "ssl_ecdh_curve", "TLSv1.1", "MLKEM"} {
		if strings.Contains(out, refused) {
			t.Errorf("the HTTPS block carries %q", refused)
		}
	}
}

// An empty string is as fatal as a nil pointer: `ssl_certificate ;` is what
// nginx actually refused. A path that exists in the row but is blank must be
// treated as "no certificate".
// Boş dize, nil işaretçi kadar ölümcüldür: nginx'in gerçekten reddettiği şey
// `ssl_certificate ;` idi. Satırda var olan ama boş olan yol, "sertifika yok"
// sayılmalıdır.
func TestBlankCertificatePathCountsAsNoCertificate(t *testing.T) {
	ng, err := NewNginxGenerator()
	if err != nil {
		t.Fatal(err)
	}
	blank := ""
	site := &core.Site{
		ID: 11, DocumentRoot: "/var/www/x", ProjectType: "static",
		SSLEnabled: true, SSLCertPath: &blank, SSLKeyPath: &blank,
	}
	out, err := ng.GenerateVhost(site, &core.Domain{
		ID: 19, SubscriptionID: 7, Name: "x.example",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "ssl_certificate") {
		t.Error("a blank certificate path must not produce an ssl_certificate directive")
	}
}

// expandPanelIncludes is the vhost as nginx reads it once the Panel's include
// directory holds its challenge file (D-031 step 1b): every include line of
// that directory replaced by the challenge location. Structural tests of the
// ACME exception read this text.
func expandPanelIncludes(out string, data VhostData) string {
	location := RenderACMEChallengeFile(data.Domain, data.ACMEChallengeRoot)
	return strings.ReplaceAll(out, PanelManagedIncludeLine(data.Domain), location)
}
