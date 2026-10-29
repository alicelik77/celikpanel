package main

import (
	"context"
	"errors"

	"github.com/alicelik/celikpanel/internal/transport"
)

// A first install that failed before it placed any package (Beta-1 re-run
// P6, 2026-10-29). The VM case: APT could not show what installing bind9
// would do ("no installation candidate"), so nothing was installed. The
// install receipt for that exact request had been written (it is written
// before the package transaction) and the install guard had restored the
// unit state it found: no masks, no units, nothing running. The sealed-target
// proof needs the guard's persistent masks, which only a placed package
// keeps, so it could never pass, and the setup step stayed paused at
// server_setup_dns_agent_failed. Only a panel restart released it, through
// startup recovery's weaker runtime check.
//
// This is the same claim the sealed proof makes, for the host the failed
// install actually left: none of the packages that this exact request found
// missing is installed now, none of the target's units is active, failed or
// masked, and nothing serves public port 53. Read twice around the unit and
// port observation; a package that appears in between is not "never placed".
// Everything else (journal, engine state, ownership, the exact install
// receipt) is still checked first by the caller.
//
// Hiçbir paket yerleştirmeden başarısız olan ilk kurulum: bu isteğin eksik
// bulduğu paketlerin hiçbiri kurulu değil, hedefin birimleri etkin, hatalı ya
// da maskeli değil ve genel 53 numaralı bağlantı noktasında hizmet veren yok.

type dnsEngineNeverPlacedOps struct {
	packageInstalled func(name string) (bool, error)
	targetUnits      func() ([]bindInstallUnitState, error)
	port53Conflict   func() (bool, error)
}

func verifyDNSEngineInstallNeverPlacedWithOps(
	install dnsEngineInstallOwnershipReceipt,
	ops dnsEngineNeverPlacedOps,
) (bool, error) {
	if ops.packageInstalled == nil || ops.targetUnits == nil || ops.port53Conflict == nil {
		return false, errors.New("never-placed DNS install proof operations are incomplete")
	}
	if install.AdoptedPresent || len(install.MissingBefore) == 0 {
		return false, nil
	}
	absent := func() (bool, error) {
		for _, name := range install.MissingBefore {
			installed, err := ops.packageInstalled(name)
			if err != nil {
				return false, err
			}
			if installed {
				return false, nil
			}
		}
		return true, nil
	}
	if ok, err := absent(); err != nil || !ok {
		return false, err
	}
	units, err := ops.targetUnits()
	if err != nil {
		return false, err
	}
	for _, unit := range units {
		if unit.active() || unit.activeState == "failed" || unit.activeState == "activating" ||
			unit.activeState == "deactivating" || unit.activeState == "reloading" || unit.masked() {
			return false, nil
		}
	}
	conflict, err := ops.port53Conflict()
	if err != nil || conflict {
		return false, err
	}
	return absent()
}

func verifyDNSEngineInstallNeverPlaced(
	ctx context.Context,
	target transport.DNSEngine,
	install dnsEngineInstallOwnershipReceipt,
	host dnsEngineRollbackTargetHost,
) (bool, error) {
	if host.Systemctl == "" || install.Engine != target ||
		install.PackageManager != string(host.PackageManager) {
		return false, nil
	}
	profile, err := verifiedHostProfileForAnyFamily()
	if err != nil {
		return false, err
	}
	if profile.PackageManager != host.PackageManager {
		return false, errors.New("never-placed DNS install proof host profile changed")
	}
	unitNames := []string{"named.service", "bind9.service"}
	if target == transport.DNSEnginePowerDNS {
		unitNames = []string{"pdns.service"}
	}
	proofCtx, cancel := context.WithTimeout(ctx, dnsRuntimeInspectionTimeout)
	defer cancel()
	guard := dnsSystemdStateGuard(host.Systemctl)
	return verifyDNSEngineInstallNeverPlacedWithOps(install, dnsEngineNeverPlacedOps{
		packageInstalled: func(name string) (bool, error) {
			return exactDNSEnginePackageInstalled(proofCtx, profile, name)
		},
		targetUnits: func() ([]bindInstallUnitState, error) {
			units := make([]bindInstallUnitState, 0, len(unitNames))
			for _, name := range unitNames {
				state, err := guard.inspect(proofCtx, name)
				if err != nil {
					return nil, err
				}
				units = append(units, state)
			}
			return units, nil
		},
		port53Conflict: func() (bool, error) {
			return dnsPort53ConflictCheck(proofCtx, false, false)
		},
	})
}
