package pdnsvendor

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/dnsunitidentity"
)

// measuredArch515UnitSHA256 is the digest Arch's powerdns 5.1.5-1 package
// recorded for /usr/lib/systemd/system/pdns.service (mtree, 2026-10-30).
const measuredArch515UnitSHA256 = "4bc3129b997154908bb65f81c021ea965037178367b6f319c68865bb3097d420"

func TestCertifiedArch51UnitIsTheMeasuredPackageFile(t *testing.T) {
	sum := sha256.Sum256([]byte(CertifiedArch51Unit))
	if got := hex.EncodeToString(sum[:]); got != measuredArch515UnitSHA256 {
		t.Fatalf("certified Arch unit digest %s differs from the measured package file", got)
	}
	if err := VerifyPacmanUnit([]byte(CertifiedArch51Unit), measuredArch515UnitSHA256); err != nil {
		t.Fatal(err)
	}
	// The Debian unit is never accepted as Arch's, and the Arch unit never
	// passes the APT byte proof.
	if VerifyBytes([]byte(CertifiedArch51Unit)) == nil {
		t.Fatal("Arch unit passed the APT byte proof")
	}
	debianSum := sha256.Sum256([]byte(CertifiedDebian13Unit))
	if VerifyPacmanUnit([]byte(CertifiedDebian13Unit), hex.EncodeToString(debianSum[:])) == nil {
		t.Fatal("the Debian unit was accepted on Arch")
	}
}

func TestVerifyPacmanUnitAcceptsOnlyThePackageFile(t *testing.T) {
	// A later package release may change the unit; it is accepted when the
	// installed package records exactly that file and the command and account
	// stay the certified ones.
	updated := strings.Replace(CertifiedArch51Unit, "RestartSec=1\n", "RestartSec=2\n", 1)
	updatedSum := sha256.Sum256([]byte(updated))
	if err := VerifyPacmanUnit([]byte(updated), hex.EncodeToString(updatedSum[:])); err != nil {
		t.Fatalf("a package-recorded unit update was refused: %v", err)
	}
	// An edit that the package does not record is refused.
	if VerifyPacmanUnit([]byte(updated), measuredArch515UnitSHA256) == nil {
		t.Fatal("an unrecorded unit edit was accepted")
	}
	for name, unit := range map[string]string{
		"other daemon":  strings.Replace(CertifiedArch51Unit, ArchExecArgv, "/usr/bin/pdns_server --daemon=no", 1),
		"root account":  strings.Replace(CertifiedArch51Unit, "User=powerdns\n", "User=root\n", 1),
		"extra command": strings.Replace(CertifiedArch51Unit, "SyslogIdentifier", "ExecStartPre=/bin/true\nSyslogIdentifier", 1),
	} {
		sum := sha256.Sum256([]byte(unit))
		if VerifyPacmanUnit([]byte(unit), hex.EncodeToString(sum[:])) == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if VerifyPacmanUnit([]byte(CertifiedArch51Unit), "") == nil {
		t.Fatal("a unit without a package record was accepted")
	}
}

func TestPacmanUnitIdentity(t *testing.T) {
	identity := dnsunitidentity.Identity{
		ID: "pdns.service", Names: []string{"pdns.service"},
		FragmentPath: UnitPath, Transient: "no",
		ExecStartPath: "/usr/bin/pdns_server", ExecStartArgv: ArchExecArgv,
	}
	if err := dnsunitidentity.ValidatePDNSVendorIdentityFor("pacman", identity); err != nil {
		t.Fatal(err)
	}
	if dnsunitidentity.ValidatePDNSVendorIdentityFor("apt", identity) == nil {
		t.Fatal("the Arch identity passed on an APT host")
	}
	debian := identity
	debian.ExecStartPath, debian.ExecStartArgv = "/usr/sbin/pdns_server", ExecArgv
	if dnsunitidentity.ValidatePDNSVendorIdentityFor("pacman", debian) == nil {
		t.Fatal("the Debian identity passed on a pacman host")
	}
	if err := dnsunitidentity.ValidatePDNSVendorIdentityFor("apt", debian); err != nil {
		t.Fatal(err)
	}
	if dnsunitidentity.ValidatePDNSVendorIdentityFor("dnf", identity) == nil {
		t.Fatal("an uncertified family was accepted")
	}
}
