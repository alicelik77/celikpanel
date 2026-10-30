package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/alicelik/celikpanel/internal/mailtlsconfig"
)

// Dovecot config comes in two dialects. 2.3 (Ubuntu 24.04) uses mail_location,
// passdb{driver=…,args=…} and ssl_cert=<file; 2.4 (Debian 13 "trixie") removed
// all of those: mail_driver/mail_path, named passdb blocks with explicit
// settings, ssl_server_cert_file, and a new %{user | domain} variable syntax.
// A 2.3-dialect file makes 2.4 refuse to start — caught live on the Debian 13
// golden path. So every writer builds the dialect the installed Dovecot
// actually speaks, and applyDovecotConf validates the result with Dovecot's
// own parser before the service is ever restarted (the nginx -t pattern):
// a wrong config becomes an honest error, never a dead mail server.
//
// Dovecot yapılandırması iki lehçede gelir. 2.3 (Ubuntu 24.04) mail_location,
// passdb{driver=…,args=…} ve ssl_cert=<dosya kullanır; 2.4 (Debian 13
// "trixie") bunların hepsini kaldırdı: mail_driver/mail_path, açık ayarlı
// adlandırılmış passdb blokları, ssl_server_cert_file ve yeni
// %{user | domain} değişken sözdizimi. 2.3 lehçesinde bir dosya 2.4'ü
// başlamaz eder — Debian 13 golden path'inde canlı yakalandı. Bu yüzden her
// yazıcı, kurulu Dovecot'un gerçekten konuştuğu lehçeyi üretir ve
// applyDovecotConf sonucu servis yeniden başlatılmadan ÖNCE Dovecot'un kendi
// ayrıştırıcısıyla doğrular (nginx -t deseni): yanlış yapılandırma dürüst bir
// hataya dönüşür, asla ölü bir posta sunucusuna değil.

// dovecotIs24 requires a successful observation of an implemented dialect.
// Unknown does not establish 2.4 compatibility or permission to change files.
func dovecotIs24() (bool, error) {
	return dovecotIs24WithRunner(runMailTLSCommand)
}

// buildDovecotVirtualConf renders the virtual-mailbox override (auth against
// /etc/dovecot/users, maildirs under mailRootDir) in the requested dialect.
// buildDovecotVirtualConf, sanal-posta-kutusu ekini (auth /etc/dovecot/users'a
// karşı, maildir'ler mailRootDir altında) istenen lehçede üretir.
func buildDovecotVirtualConf(is24 bool) string {
	return buildDovecotVirtualConfAt(is24, mailRootDir)
}

// buildDovecotVirtualConfAt renders the override for an explicit, already
// resolved mail root (see managedMailRootPath). The first line carries the
// SHA-256 of everything after it, so a later release can tell its own,
// unchanged file from one the server administrator edited
// (classifyDovecotVirtualDropIn).
// buildDovecotVirtualConfAt, açıkça verilmiş ve çözülmüş posta kökü için eki
// üretir. İlk satır geri kalanın SHA-256 özetini taşır; sonraki bir sürüm
// kendi değişmemiş dosyasını yöneticinin düzenlediğinden ayırabilir.
func buildDovecotVirtualConfAt(is24 bool, root string) string {
	body := dovecotVirtualBody(is24, root)
	sum := sha256.Sum256([]byte(body))
	return dovecotVirtualHeaderStart + hex.EncodeToString(sum[:]) + "\n" + body
}

// dovecotVirtualHeaderStart opens the managed drop-in's first line; the
// digest of the body follows it.
const dovecotVirtualHeaderStart = "# Managed by CelikPanel: Dovecot virtual mailboxes. sha256="

// dovecotVirtualPreamble is the drop-in's own explanation for the person who
// opens it: what CelikPanel does with the file and where their settings go.
const dovecotVirtualPreamble = `# CelikPanel rewrites this file only while it is exactly as CelikPanel wrote
# it. Once you edit it, CelikPanel keeps your version and tells you so.
# Put your own settings in /etc/dovecot/local.conf, which Dovecot reads after
# conf.d. Dovecot runs from this file with or without CelikPanel.
`

func dovecotVirtualBody(is24 bool, root string) string {
	if !is24 {
		return dovecotVirtualPreamble + fmt.Sprintf(`mail_location = maildir:%s/%%d/%%n
mail_uid = %s
mail_gid = %s
first_valid_uid = %s
passdb {
  driver = passwd-file
  args = scheme=CRYPT username_format=%%u %s
}
userdb {
  driver = passwd-file
  args = username_format=%%u %s
  default_fields = uid=%s gid=%s home=%s/%%d/%%n
}
`, root, vmailUID, vmailGID, vmailUID,
			dovecotUsersPath, dovecotUsersPath, vmailUID, vmailGID, root)
	}
	// 2.4: mail_location → mail_driver+mail_path, %d/%n → %{user | domain} /
	// %{user | username}, args → explicit settings, default_fields → fields
	// with :default. Our stored hashes carry explicit {SCHEME} prefixes, so
	// default_password_scheme is only the fallback (mirrors 2.3's scheme=CRYPT).
	// 2.4: mail_location → mail_driver+mail_path, %d/%n → %{user | domain} /
	// %{user | username}, args → açık ayarlar, default_fields → :default'lu
	// fields. Kayıtlı özetlerimiz açık {ŞEMA} öneki taşır; default_password_
	// scheme yalnız yedektir (2.3'teki scheme=CRYPT'in aynası).
	//
	// 2.4 split the 2.3 mail_location string into separate settings, and a
	// later file only replaces the settings it names. Debian 13's stock
	// conf.d/10-mail.conf names three for its system users: mail_home =
	// /home/%{user | username}, mail_path = %{home}/mail and mail_inbox_path
	// = /var/mail/%{user}. With only mail_driver and mail_path here, the
	// virtual users inherited Debian's INBOX in /var/mail, which they cannot
	// create ("Failed to autocreate mailbox: Permission denied", final
	// alpha.113 acceptance, 30 Oct 2026). All three are therefore set: INBOX
	// is the maildir root Postfix's virtual(8) delivers into
	// (<root>/<domain>/<user>/new). Leaving mail_inbox_path empty is not the
	// same: measured on Debian's 2.4.1 it puts INBOX in a ".INBOX" folder
	// beside the delivered mail, so delivered mail is silently not shown.
	// mail_privileged_group (Debian: mail) only matters for an mbox INBOX in
	// a shared spool and is left as the distribution set it; the stock inbox
	// namespace (inbox = yes, special-use folders) is what the virtual users
	// need and is not repeated here.
	// 2.4, 2.3'ün mail_location dizgesini ayrı ayarlara böldü; sonraki dosya
	// yalnız adını verdiği ayarları değiştirir. Debian 13'ün 10-mail.conf'u
	// sistem kullanıcıları için mail_home, mail_path ve mail_inbox_path
	// (/var/mail/%{user}) verir; yalnız mail_driver ve mail_path yazınca sanal
	// kullanıcılar Debian'ın /var/mail INBOX'unu devralıyordu. Üçü de burada:
	// INBOX, Postfix'in teslim ettiği maildir köküdür. mail_inbox_path'i boş
	// bırakmak aynı şey değildir: Debian 2.4.1'de INBOX'u ".INBOX" klasörüne
	// koyar ve teslim edilen posta görünmez.
	userDir := "%{user | domain}/%{user | username}"
	return dovecotVirtualPreamble + fmt.Sprintf(`mail_driver = maildir
mail_home = %[1]s/%[2]s
mail_path = %[1]s/%[2]s
mail_inbox_path = %[1]s/%[2]s
mail_uid = %[3]s
mail_gid = %[4]s
first_valid_uid = %[3]s
passdb passwd-file {
  default_password_scheme = CRYPT
  passwd_file_path = %[5]s
}
userdb passwd-file {
  passwd_file_path = %[5]s
  fields {
    uid:default = %[3]s
    gid:default = %[4]s
    home:default = %[1]s/%[2]s
  }
}
`, root, userDir, vmailUID, vmailGID, dovecotUsersPath)
}

// buildDovecotTLSConf renders the TLS drop-in (default certificate + one
// local_name block per SNI name) in the requested dialect.
// buildDovecotTLSConf, TLS ekini (varsayılan sertifika + SNI adı başına bir
// local_name bloğu) istenen lehçede üretir.
func buildDovecotTLSConf(is24 bool, certPath, keyPath string, sni []MailSNIEntry) string {
	return mailtlsconfig.Dovecot(is24, certPath, keyPath, sni)
}

// applyDovecotConf writes a drop-in, then validates the WHOLE resulting config
// with Dovecot's own parser. On failure the previous content is restored (or
// the file removed if it did not exist) and the parser's first complaint is
// returned — configuration can fail loudly, the mail service cannot be broken.
// applyDovecotConf bir ek yazar, sonra ortaya çıkan yapılandırmanın TAMAMINI
// Dovecot'un kendi ayrıştırıcısıyla doğrular. Hata durumunda önceki içerik
// geri gelir (dosya yoktuysa silinir) ve ayrıştırıcının ilk şikâyeti döner —
// yapılandırma gürültüyle başarısız olabilir, posta servisi bozulamaz.
func applyDovecotConf(path string, content string) error {
	snapshot, err := snapshotMailFile(path)
	if err != nil {
		return fmt.Errorf("snapshot dovecot configuration: %w", err)
	}
	if err := secureWriteConfig(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write dovecot configuration: %w", err)
	}
	out, err := runMailTLSCommand("doveconf", "-n")
	if err != nil {
		if rollbackErr := restoreMailFile(snapshot); rollbackErr != nil {
			return fmt.Errorf(
				"dovecot rejected the configuration and rollback failed: %s: %v",
				dovecotFirstError(string(out)),
				rollbackErr,
			)
		}
		detail := dovecotFirstError(string(out))
		if strings.TrimSpace(string(out)) == "" {
			detail = err.Error()
		}
		return fmt.Errorf("dovecot rejected the configuration: %s", detail)
	}
	return nil
}

// dovecotFirstError picks the first line that looks like the actual problem,
// so the operator sees "unknown setting X" rather than a wall of output.
// dovecotFirstError, gerçek soruna benzeyen ilk satırı seçer; operatör çıktı
// duvarı yerine "bilinmeyen ayar X" görür.
func dovecotFirstError(out string) string {
	for _, line := range strings.Split(out, "\n") {
		l := strings.TrimSpace(line)
		if strings.Contains(l, "Error") || strings.Contains(l, "Fatal") || strings.Contains(l, "Invalid") {
			return l
		}
	}
	return firstLine(out)
}
