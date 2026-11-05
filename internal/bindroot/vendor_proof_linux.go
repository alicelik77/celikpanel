//go:build linux

package bindroot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alicelik/celikpanel/internal/dnsunitidentity"
	"github.com/alicelik/celikpanel/internal/hostplatform"
	"golang.org/x/sys/unix"
)

type VendorFilesIdentity struct {
	Unit        FileIdentity
	Environment FileIdentity
}

const (
	aptVendorUnitPath         = dnsunitidentity.VendorUnitDir + "named.service"
	aptSplitUsrVendorUnitPath = dnsunitidentity.SplitUsrVendorUnitDir + "named.service"
	aptVendorEnvironmentPath  = "/etc/default/named"
)

// VerifyAPTVendorOwner accepts only the certified bind9-owned vendor files:
// the unit under the name dpkg records for it (/usr/lib on Debian 13 and
// Ubuntu 24.04, /lib on Debian 12 and Ubuntu 22.04) and its environment file.
func VerifyAPTVendorOwner(path string, output []byte, commandErr error) error {
	if commandErr != nil {
		return fmt.Errorf("verify BIND vendor package ownership for %s: %w", path, commandErr)
	}
	switch path {
	case aptVendorUnitPath, aptSplitUsrVendorUnitPath, aptVendorEnvironmentPath:
		if string(output) != "bind9: "+path+"\n" {
			return fmt.Errorf("%s is not owned by the exact bind9 package", path)
		}
		return nil
	default:
		return errors.New("unsupported APT BIND vendor file")
	}
}

// APTVendorOwnerLookup runs dpkg-query -S for one path.
type APTVendorOwnerLookup func(context.Context, string) ([]byte, error)

// VerifyAPTVendorPackageOwnership proves that the bind9 package owns the
// vendor unit and /etc/default/named. The unit is asked for under its /usr/lib
// name first. Packages built before the usrmerge path move (Debian 12, Ubuntu
// 22.04) record it as /lib/systemd/system/named.service, which dpkg does not
// map to /usr/lib; that name is asked for only when libIsUsrLibAlias proves
// /lib is the merged-/usr symlink, so the dpkg-owned file is the same file
// whose bytes are proven at /usr/lib and that systemd loads.
func VerifyAPTVendorPackageOwnership(
	ctx context.Context,
	lookup APTVendorOwnerLookup,
	libIsUsrLibAlias bool,
) error {
	if ctx == nil || lookup == nil {
		return errors.New("invalid APT BIND vendor package ownership proof")
	}
	output, err := lookup(ctx, aptVendorUnitPath)
	if unitErr := VerifyAPTVendorOwner(aptVendorUnitPath, output, err); unitErr != nil {
		if !libIsUsrLibAlias {
			return unitErr
		}
		output, err = lookup(ctx, aptSplitUsrVendorUnitPath)
		if splitErr := VerifyAPTVendorOwner(aptSplitUsrVendorUnitPath, output, err); splitErr != nil {
			return fmt.Errorf("%v; under the merged-/usr name: %w", unitErr, splitErr)
		}
	}
	output, err = lookup(ctx, aptVendorEnvironmentPath)
	return VerifyAPTVendorOwner(aptVendorEnvironmentPath, output, err)
}

func VerifyPacmanVendorOwner(path string, output []byte, commandErr error) error {
	if commandErr != nil {
		return fmt.Errorf("verify BIND vendor package ownership for %s: %w", path, commandErr)
	}
	if path != "/usr/lib/systemd/system/named.service" || string(output) != "bind\n" {
		return errors.New("BIND vendor unit is not owned by the exact bind package")
	}
	return nil
}

func proveInstalledVendorPackage(ctx context.Context, profile hostplatform.Profile) error {
	switch profile.PackageManager {
	case hostplatform.PackageManagerAPT:
		lookup := func(lookupCtx context.Context, path string) ([]byte, error) {
			return runTrusted(lookupCtx, []string{"/usr/bin/dpkg-query", "/usr/sbin/dpkg-query"}, "-S", "--", path)
		}
		if err := VerifyAPTVendorPackageOwnership(ctx, lookup, dnsunitidentity.LibIsUsrLibAlias()); err != nil {
			return err
		}
	case hostplatform.PackageManagerPacman:
		const path = "/usr/lib/systemd/system/named.service"
		output, err := runTrusted(ctx, []string{"/usr/bin/pacman", "/usr/sbin/pacman"}, "--query", "--quiet", "--owns", "--", path)
		if err := VerifyPacmanVendorOwner(path, output, err); err != nil {
			return err
		}
	default:
		return errors.New("unsupported BIND vendor package proof")
	}
	return nil
}

// InspectInstalledVendor proves the fixed package-owned vendor unit and, on
// APT, its effective environment across two no-follow descriptor reads. It
// does not prove systemd loaded these bytes or that named serves the selected
// generation.
func InspectInstalledVendor(ctx context.Context, profile hostplatform.Profile) (VendorFilesIdentity, error) {
	if ctx == nil {
		return VendorFilesIdentity{}, errors.New("BIND vendor observation requires a context")
	}
	contract, err := CertifiedVendorContract(profile)
	if err != nil {
		return VendorFilesIdentity{}, err
	}
	proofCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	rootFD, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return VendorFilesIdentity{}, fmt.Errorf("open BIND vendor proof root: %w", err)
	}
	defer unix.Close(rootFD)
	if err := proveInstalledVendorPackage(proofCtx, profile); err != nil {
		return VendorFilesIdentity{}, err
	}
	identity, err := inspectVendorAt(rootFD, contract, nil)
	if err != nil {
		return VendorFilesIdentity{}, err
	}
	if err := proveInstalledVendorPackage(proofCtx, profile); err != nil {
		return VendorFilesIdentity{}, err
	}
	return identity, nil
}

func inspectVendorAt(rootFD int, contract VendorContract, between func()) (VendorFilesIdentity, error) {
	first, err := vendorSnapshotAt(rootFD, contract)
	if err != nil {
		return VendorFilesIdentity{}, err
	}
	if between != nil {
		between()
	}
	second, err := vendorSnapshotAt(rootFD, contract)
	if err != nil {
		return VendorFilesIdentity{}, err
	}
	if first != second {
		return VendorFilesIdentity{}, errors.New("BIND vendor unit files changed during exact verification")
	}
	return second, nil
}

func vendorSnapshotAt(rootFD int, contract VendorContract) (VendorFilesIdentity, error) {
	unit, unitIdentity, err := ReadExactRootOwnedFileAt(rootFD, contract.UnitPath, "BIND vendor unit")
	if err != nil {
		return VendorFilesIdentity{}, err
	}
	if !bytes.Equal(unit, contract.UnitBytes) {
		return VendorFilesIdentity{}, errors.New("BIND vendor unit bytes differ from the certified package unit")
	}
	identity := VendorFilesIdentity{Unit: unitIdentity}
	if contract.EnvironmentPath == "" {
		return identity, nil
	}
	environment, environmentIdentity, err := ReadExactRootOwnedFileAt(rootFD, contract.EnvironmentPath, "BIND vendor environment")
	if err != nil {
		return VendorFilesIdentity{}, err
	}
	if !bytes.Equal(environment, contract.EnvironmentBytes) {
		return VendorFilesIdentity{}, errors.New("BIND vendor environment bytes differ from the certified safe options")
	}
	identity.Environment = environmentIdentity
	return identity, nil
}
