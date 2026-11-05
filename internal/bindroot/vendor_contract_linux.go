//go:build linux

package bindroot

import (
	"errors"

	"github.com/alicelik/celikpanel/internal/hostplatform"
)

type VendorContract struct {
	UnitPath         string
	EnvironmentPath  string
	UnitBytes        []byte
	EnvironmentBytes []byte
}

const CertifiedAPTBINDVendorUnit = "[Unit]\n" +
	"Description=BIND Domain Name Server\n" +
	"Documentation=man:named(8)\n" +
	"After=network.target\nWants=nss-lookup.target\nBefore=nss-lookup.target\n\n" +
	"[Service]\nType=notify\nEnvironmentFile=-/etc/default/named\n" +
	"ExecStart=/usr/sbin/named -f $OPTIONS\n" +
	"ExecReload=/usr/sbin/rndc reload\nExecStop=/usr/sbin/rndc stop\n" +
	"Restart=on-failure\n\n[Install]\nWantedBy=multi-user.target\nAlias=bind9.service\n"

// CertifiedAPTBINDVendorUnitUbuntu2204 is Ubuntu 22.04's own bind9 unit
// (1:9.18.39-0ubuntu0.22.04.8, dpkg md5 2ea058809196551201d8d88748e46f6f,
// sha256 26250537dafeaf30b0a07fd2ff2678fc3f9d5ad52c27e6ea370d2d5bf8ab3a38):
// Type=forking and named without -f. It is the contract only on Ubuntu 22.04
// (alpha.123 lab, 2026-11-05).
const CertifiedAPTBINDVendorUnitUbuntu2204 = "[Unit]\n" +
	"Description=BIND Domain Name Server\n" +
	"Documentation=man:named(8)\n" +
	"After=network.target\nWants=nss-lookup.target\nBefore=nss-lookup.target\n\n" +
	"[Service]\nType=forking\nEnvironmentFile=-/etc/default/named\n" +
	"ExecStart=/usr/sbin/named $OPTIONS\n" +
	"ExecReload=/usr/sbin/rndc reload\nExecStop=/usr/sbin/rndc stop\n" +
	"Restart=on-failure\n\n[Install]\nWantedBy=multi-user.target\nAlias=bind9.service\n"

const CertifiedAPTBINDVendorEnvironment = "#\n# run resolvconf?\n" +
	"RESOLVCONF=no\n\n# startup options for the server\nOPTIONS=\"-u bind\"\n"

const CertifiedPacmanBINDVendorUnit = "[Unit]\n" +
	"Description=Internet domain name server\nAfter=network.target\n\n" +
	"[Service]\nExecStart=/usr/bin/named -f -u named\n" +
	"ExecReload=/usr/bin/kill -HUP $MAINPID\n\n" +
	"[Install]\nWantedBy=multi-user.target\n"

func CertifiedVendorContract(profile hostplatform.Profile) (VendorContract, error) {
	if profile.ServiceManager != hostplatform.ServiceManagerSystemd {
		return VendorContract{}, errors.New("BIND vendor proof requires systemd")
	}
	switch {
	case profile.PackageManager == hostplatform.PackageManagerAPT &&
		profile.DistroFamily == hostplatform.DistroFamilyDebian:
		unit := CertifiedAPTBINDVendorUnit
		if profile.ID == "ubuntu" && profile.Version == "22.04" {
			unit = CertifiedAPTBINDVendorUnitUbuntu2204
		}
		return VendorContract{
			UnitPath:         "/usr/lib/systemd/system/named.service",
			EnvironmentPath:  "/etc/default/named",
			UnitBytes:        []byte(unit),
			EnvironmentBytes: []byte(CertifiedAPTBINDVendorEnvironment),
		}, nil
	case profile.PackageManager == hostplatform.PackageManagerPacman &&
		profile.DistroFamily == hostplatform.DistroFamilyArch:
		return VendorContract{
			UnitPath:  "/usr/lib/systemd/system/named.service",
			UnitBytes: []byte(CertifiedPacmanBINDVendorUnit),
		}, nil
	default:
		return VendorContract{}, errors.New("BIND vendor unit proof is unsupported on this host profile")
	}
}
