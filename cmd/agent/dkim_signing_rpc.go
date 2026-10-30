package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// DKIM signing — the missing half of DKIM. The panel has long generated keys
// and published the DNS records, but nothing ever SIGNED outgoing mail, so
// receivers saw a key in DNS and no signature on the message: a fail, not a
// pass. OpenDKIM runs as a milter on localhost; its key and signing tables
// are generated from the panel's key directory, so every domain that has a
// key signs automatically. milter_default_action=accept means a dead filter
// degrades to unsigned mail, never to lost mail.
//
// DKIM imzalama — DKIM'in eksik yarısı. Panel uzun süredir anahtar üretiyor
// ve DNS kayıtlarını yayımlıyordu ama giden postayı hiçbir şey İMZALAMIYORDU;
// alıcılar DNS'te anahtar görüp iletide imza bulamıyordu: geçer değil, kalır.
// OpenDKIM, localhost'ta milter olarak koşar; anahtar ve imzalama tabloları
// panelin anahtar dizininden üretilir; anahtarı olan her domain otomatik
// imzalar. milter_default_action=accept: ölü filtre imzasız postaya düşer,
// asla kayıp postaya değil.

const signingSelector = "celik"

const (
	opendkimAPTConfPath    = "/etc/opendkim.conf"
	opendkimPacmanConfPath = "/etc/opendkim/opendkim.conf"
	opendkimTablesDir      = dkimStoreRoot + "/tables"
	// Numeric loopback on both sides (D3, final alpha.113 acceptance run 2,
	// 30 Oct 2026): a chrooted Postfix service resolves "localhost" through
	// the files inside its chroot and Arch ships none, so submission (587)
	// and smtps (465) logged "host or service localhost:8891 not found" and
	// sent every message unsigned under milter_default_action=accept. A
	// literal address needs no resolver, hosts file or nsswitch anywhere,
	// and never depends on whether "localhost" means 127.0.0.1 or ::1.
	// İki tarafta da sayısal geri döngü adresi: chroot'lu bir Postfix
	// hizmeti "localhost"u chroot içindeki dosyalarla çözer ve Arch hiçbirini
	// getirmez. Sayısal adres hiçbir çözücüye ihtiyaç duymaz.
	opendkimSocket = "inet:8891@127.0.0.1"
	opendkimMilter = "inet:127.0.0.1:8891"
	// What releases up to alpha.113 wrote; the startup repair recognises
	// them as the panel's own (mail_milter_repair.go).
	// alpha.113'e kadarki sürümlerin yazdığı; başlangıç onarımı bunları
	// panelin kendi yazdığı sayar.
	legacyOpenDKIMSocket = "inet:8891@localhost"
	legacyOpenDKIMMilter = "inet:localhost:8891"
)

// opendkimLayout is where the distribution's own opendkim unit reads its
// configuration, and which packages bring the daemon. Debian/Ubuntu: the
// unit reads /etc/opendkim.conf (opendkim + opendkim-tools, root anchor from
// dns-root-data). Arch: the official extra/opendkim unit runs
// `opendkim -x /etc/opendkim/opendkim.conf`, its sysusers file creates the
// opendkim user and group, and no DNSSEC root anchor file is installed.
// opendkimLayout, dağıtımın kendi opendkim unit'inin yapılandırmayı nereden
// okuduğunu ve hangi paketlerin daemon'u getirdiğini söyler.
type opendkimLayout struct {
	confPath    string
	packages    []string
	trustAnchor string
}

func opendkimLayoutFor(family string) (opendkimLayout, bool) {
	switch family {
	case "apt":
		return opendkimLayout{
			confPath:    opendkimAPTConfPath,
			packages:    []string{"opendkim", "opendkim-tools"},
			trustAnchor: "/usr/share/dns/root.key",
		}, true
	case "pacman":
		return opendkimLayout{
			confPath: opendkimPacmanConfPath,
			packages: []string{"opendkim"},
		}, true
	default:
		return opendkimLayout{}, false
	}
}

// opendkimConfPath is the configuration path on this host, apt's when the
// host family cannot be verified (the historical behaviour).
func opendkimConfPath() string {
	if layout, ok := opendkimLayoutFor(detectPkgFamily()); ok {
		return layout.confPath
	}
	return opendkimAPTConfPath
}

// renderOpenDKIMConf renders CelikPanel's OpenDKIM configuration.
func renderOpenDKIMConf(layout opendkimLayout) string {
	return renderOpenDKIMConfWithSocket(layout, opendkimSocket)
}

// renderOpenDKIMConfWithSocket renders the file around one listening socket,
// so the startup repair can recognise the exact text an earlier release wrote.
// renderOpenDKIMConfWithSocket dosyayı tek bir dinleme soketiyle üretir;
// başlangıç onarımı önceki sürümün yazdığı metni böylece birebir tanır.
func renderOpenDKIMConfWithSocket(layout opendkimLayout, socket string) string {
	conf := fmt.Sprintf(`# Managed by CelikPanel — DKIM signing for hosted domains. Do not edit by hand.
Syslog			yes
UMask			007
Mode			sv
Canonicalization	relaxed/simple
OversignHeaders		From
Socket			%s
PidFile			/run/opendkim/opendkim.pid
UserID			opendkim
KeyTable		%s/keytable
SigningTable		refile:%s/signingtable
InternalHosts		%s/trustedhosts
`, socket, opendkimTablesDir, opendkimTablesDir, opendkimTablesDir)
	if layout.trustAnchor != "" {
		conf += "TrustAnchorFile\t\t" + layout.trustAnchor + "\n"
	}
	return conf
}

type ConfigureDKIMSigningResponse = transport.ConfigureDKIMSigningResponse

// ConfigureDKIMSigning installs OpenDKIM if needed, regenerates the tables
// from the key directory and wires the milter into Postfix. Idempotent —
// also the resync path whenever a new domain key is created.
// ConfigureDKIMSigning, gerekirse OpenDKIM'i kurar, tabloları anahtar
// dizininden yeniden üretir ve milter'ı Postfix'e bağlar. Idempotent —
// yeni bir domain anahtarı üretildiğinde de aynı yol yeniden senkronlar.
func (a *Agent) ConfigureDKIMSigning(req *ServiceMutationRequest, resp *ConfigureDKIMSigningResponse) error {
	*resp = ConfigureDKIMSigningResponse{}
	if req == nil {
		return fmt.Errorf("DKIM signing configuration request is required")
	}
	ctx, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepConfigureDKIMSigning, "opendkim", "", "configure"),
	)
	if err != nil {
		*resp = ConfigureDKIMSigningResponse{Error: err.Error()}
		return nil
	}
	defer finishStep()
	// CELIKPANEL_MAIL_DIR marks a non-root dev agent (CELIKPANEL_DKIM_DIR
	// alone does not: installed units set it to the production default).
	// CELIKPANEL_MAIL_DIR root olmayan dev agent'ı işaretler (tek başına
	// CELIKPANEL_DKIM_DIR etmez: kurulu unit'ler onu üretim varsayılanına
	// ayarlar).
	if os.Getenv("CELIKPANEL_MAIL_DIR") != "" {
		resp.Error = "DKIM signing is a production action; not available with CELIKPANEL_MAIL_DIR set"
		return nil
	}
	if _, err := exec.LookPath("postconf"); err != nil {
		resp.Error = "postfix is not installed"
		return nil
	}
	family := detectPkgFamily()
	layout, supported := opendkimLayoutFor(family)
	if !supported {
		resp.Error = "opendkim cannot be installed automatically on this distro yet"
		return nil
	}
	if _, err := exec.LookPath("opendkim"); err != nil {
		if _, err := installPackagesContext(ctx, family, layout.packages); err != nil {
			resp.Error = fmt.Sprintf("opendkim install: %v", err)
			return nil
		}
	}

	domains, err := dkimSignedDomains()
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	if err := writeDKIMTables(ctx, domains, layout.confPath, renderOpenDKIMConf(layout)); err != nil {
		resp.Error = err.Error()
		return nil
	}

	if out, err := serviceMutationCommand(ctx, "systemctl", "enable", "opendkim").CombinedOutput(); err != nil {
		resp.Error = fmt.Sprintf("opendkim enable: %s", firstLine(string(out)))
		return nil
	}
	if out, err := serviceMutationCommand(ctx, "systemctl", "restart", "opendkim").CombinedOutput(); err != nil {
		resp.Error = fmt.Sprintf("opendkim restart: %s", firstLine(string(out)))
		return nil
	}

	// Wire the milters through the ONE composer: writing smtpd_milters here
	// directly used to erase any spam filter already wired (last writer wins,
	// silently). applyMilterChain composes every installed filter instead.
	// Milter'ları TEK besteciden bağla: burada doğrudan smtpd_milters yazmak,
	// önceden bağlanmış bir spam filtresini siliyordu (son yazan kazanır,
	// sessizce). applyMilterChain bunun yerine kurulu her filtreyi birleştirir.
	milterNote, err := applyMilterChain(ctx)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}

	resp.Configured = true
	resp.Domains = len(domains)
	resp.Detail = joinNotes(fmt.Sprintf("DKIM signing active for %d domain(s)", len(domains)), milterNote)
	return nil
}

// dkimSignedDomains lists the domains that have a private key on disk.
// dkimSignedDomains, diskte özel anahtarı olan domain'leri listeler.
func dkimSignedDomains() ([]string, error) {
	if err := ensureDKIMStorageMigrated(); err != nil {
		return nil, err
	}
	base := dkimBaseDir
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var domains []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		key, err := dkimKeyPath(e.Name(), signingSelector)
		if err != nil {
			return nil, fmt.Errorf("invalid DKIM key directory %q: %w", e.Name(), err)
		}
		exists, err := secureMailFileExists(key)
		if err != nil {
			return nil, fmt.Errorf("inspect DKIM private key for %s: %w", e.Name(), err)
		}
		if exists {
			domains = append(domains, e.Name())
		}
	}
	sort.Strings(domains)
	return domains, nil
}

// writeDKIMTables generates the OpenDKIM key/signing/trust tables and makes
// the private keys readable by the opendkim group (root keeps ownership).
// writeDKIMTables, OpenDKIM anahtar/imzalama/güven tablolarını üretir ve
// özel anahtarları opendkim grubunca okunur yapar (sahiplik root'ta kalır).
func writeDKIMTables(ctx context.Context, domains []string, confPath, conf string) error {
	if err := ensureDKIMStorageMigrated(); err != nil {
		return err
	}
	if err := secureMkdirAll(dkimStoreRoot, 0o750); err != nil {
		return fmt.Errorf("create DKIM store: %w", err)
	}
	if err := secureMkdirAll(opendkimTablesDir, 0o750); err != nil {
		return fmt.Errorf("create OpenDKIM table directory: %w", err)
	}
	if err := secureMkdirAll(dkimBaseDir, 0o750); err != nil {
		return fmt.Errorf("create DKIM key directory: %w", err)
	}
	// The whole directory chain must be traversable by opendkim, not just the
	// key file — a parent it cannot enter blocks the key silently. The store
	// root is root:opendkim, and everything above it (/var, /var/lib) is
	// world-traversable, so opendkim gets there through its own group and the
	// celikpanel group membership the old layout required is now removed.
	// Tüm dizin zinciri opendkim'ce geçilebilir olmalı, yalnız anahtar dosyası
	// değil — giremediği bir üst dizin anahtarı sessizce keser. Depo kökü
	// root:opendkim'dir ve üstündeki her şey (/var, /var/lib) herkesçe
	// geçilebilirdir; dolayısıyla opendkim oraya kendi grubu üzerinden ulaşır ve
	// eski düzenin gerektirdiği celikpanel grup üyeliği artık kaldırılır.
	dropOpenDKIMFromPanelGroup(ctx)
	opendkimGroup, err := user.LookupGroup("opendkim")
	if err != nil {
		return fmt.Errorf("look up opendkim group: %w", err)
	}
	opendkimGID, err := strconv.Atoi(opendkimGroup.Gid)
	if err != nil {
		return fmt.Errorf("parse opendkim group id %q: %w", opendkimGroup.Gid, err)
	}
	if err := secureSetMailDirectoryMetadata(dkimStoreRoot, 0o750, 0, opendkimGID); err != nil {
		return err
	}
	if err := secureSetMailDirectoryMetadata(dkimBaseDir, 0o750, 0, opendkimGID); err != nil {
		return err
	}
	// The tables name every key path and every signing domain. They are not
	// secrets, but they are not the world's business either, and the store root
	// is already group-scoped — so they follow the same rule rather than
	// staying 0755 out of habit.
	// Tablolar her anahtar yolunu ve her imzalama alan adını adlandırır. Sır
	// değildirler ama herkesin işi de değildir; depo kökü zaten grup kapsamlıdır
	// — bu yüzden alışkanlıktan 0755 kalmak yerine aynı kurala uyarlar.
	if err := secureSetMailDirectoryMetadata(opendkimTablesDir, 0o750, 0, opendkimGID); err != nil {
		return err
	}
	var kt, st strings.Builder
	for _, d := range domains {
		key, err := dkimKeyPath(d, signingSelector)
		if err != nil {
			return fmt.Errorf("invalid DKIM domain %q: %w", d, err)
		}
		fmt.Fprintf(&kt, "%s._domainkey.%s %s:%s:%s\n", signingSelector, d, d, signingSelector, key)
		fmt.Fprintf(&st, "*@%s %s._domainkey.%s\n", d, signingSelector, d)
		// OpenDKIM drops to its own user; give the group read access.
		// OpenDKIM kendi kullanıcısına düşer; gruba okuma izni ver.
		if err := secureSetMailFileMetadata(key, 0o640, 0, opendkimGID); err != nil {
			return fmt.Errorf("secure DKIM private key for %s: %w", d, err)
		}
		if err := secureSetMailDirectoryMetadata(filepath.Dir(key), 0o750, 0, opendkimGID); err != nil {
			return fmt.Errorf("secure DKIM key directory for %s: %w", d, err)
		}
	}
	trusted := "127.0.0.1\n::1\nlocalhost\n"
	tables := []string{
		filepath.Join(opendkimTablesDir, "keytable"),
		filepath.Join(opendkimTablesDir, "signingtable"),
		filepath.Join(opendkimTablesDir, "trustedhosts"),
	}
	// The tables belong to the opendkim group, like the keys: Arch's unit
	// starts opendkim as its own user (Debian's starts it as root and drops
	// privileges after reading them), so root:celikpanel 0640 tables stopped
	// it with "dkimf_db_open(): Permission denied" (Arch VM, 2026-10-30).
	// Tablolar, anahtarlar gibi opendkim grubuna aittir.
	tablesToGroup := func() (func() error, error) {
		for _, path := range tables {
			if err := secureSetMailFileMetadata(path, 0o640, 0, opendkimGID); err != nil {
				return nil, fmt.Errorf("give the OpenDKIM table %s to the opendkim group: %w", path, err)
			}
		}
		return nil, nil
	}
	return applyMailFileMutation(ctx, []mailFileWrite{
		{path: tables[0], content: []byte(kt.String()), mode: 0o640},
		{path: tables[1], content: []byte(st.String()), mode: 0o640},
		{path: tables[2], content: []byte(trusted), mode: 0o640},
		{path: confPath, content: []byte(conf), mode: 0o644},
	}, nil, tablesToGroup)
}
