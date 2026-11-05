//go:build linux

package bindroot

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/alicelik/celikpanel/internal/hostplatform"
)

// Ubuntu 22.04 ships its own bind9 unit (Type=forking, named without -f); its
// contract is exactly those bytes, and only there (alpha.123 lab, u22 guest:
// dpkg md5sums and sha256 of /usr/lib/systemd/system/named.service).
func TestCertifiedVendorContractUbuntu2204UsesItsOwnUnit(t *testing.T) {
	apt := func(id, version string) hostplatform.Profile {
		return hostplatform.Profile{
			DistroFamily: hostplatform.DistroFamilyDebian, PackageManager: hostplatform.PackageManagerAPT,
			ServiceManager: hostplatform.ServiceManagerSystemd, ID: id, Version: version,
		}
	}
	jammy, err := CertifiedVendorContract(apt("ubuntu", "22.04"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(jammy.UnitBytes)
	if got := hex.EncodeToString(sum[:]); got != "26250537dafeaf30b0a07fd2ff2678fc3f9d5ad52c27e6ea370d2d5bf8ab3a38" {
		t.Fatalf("Ubuntu 22.04 unit sha256 = %s", got)
	}
	md := md5.Sum(jammy.UnitBytes)
	if got := hex.EncodeToString(md[:]); got != "2ea058809196551201d8d88748e46f6f" {
		t.Fatalf("Ubuntu 22.04 unit md5 = %s (dpkg md5sums)", got)
	}
	if string(jammy.EnvironmentBytes) != CertifiedAPTBINDVendorEnvironment {
		t.Fatal("Ubuntu 22.04 environment contract changed")
	}
	for _, p := range []hostplatform.Profile{apt("debian", "12"), apt("debian", "13"), apt("ubuntu", "24.04")} {
		c, err := CertifiedVendorContract(p)
		if err != nil {
			t.Fatal(err)
		}
		if string(c.UnitBytes) != CertifiedAPTBINDVendorUnit {
			t.Fatalf("%s %s: not the foreground vendor unit", p.ID, p.Version)
		}
	}
	// Debian 12's own unit (lab, d12 guest) is the foreground contract.
	md = md5.Sum([]byte(CertifiedAPTBINDVendorUnit))
	if got := hex.EncodeToString(md[:]); got != "8b951de04daa9470d24c08a38fa9845d" {
		t.Fatalf("foreground unit md5 = %s", got)
	}
}
