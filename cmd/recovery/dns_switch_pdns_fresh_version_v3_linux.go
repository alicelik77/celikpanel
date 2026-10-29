//go:build linux

package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/alicelik/celikpanel/internal/debversion"
	"github.com/alicelik/celikpanel/internal/dnsengineartifact"
	"github.com/alicelik/celikpanel/internal/hostplatform"
)

// verifyInstalledFreshPDNSNativeVersionV3 is independent, read-only recovery
// attestation. Unknown package or executable provenance leaves the serving
// target pending; it never grants an inverse or silently normalizes SQL. It
// accepts what the Agent accepts: both packages at one release of the tested
// PowerDNS series and the daemon the installed package's own file.
func verifyInstalledFreshPDNSNativeVersionV3(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil {
		return errors.New("v3 PowerDNS version check requires a live context")
	}
	profile, err := hostplatform.Detect()
	if err != nil {
		return err
	}
	if profile.ID != "debian" || profile.Version != "13" || profile.Arch != "amd64" ||
		profile.PackageManager != hostplatform.PackageManagerAPT || profile.ServiceManager != hostplatform.ServiceManagerSystemd {
		return errors.New("v3 native PowerDNS transform is measured only on Debian 13 amd64")
	}
	query := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "/usr/bin/dpkg-query", args...)
		cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C"}
		raw, err := cmd.Output()
		return string(raw), err
	}
	version := ""
	for _, name := range dnsengineartifact.PDNSFreshPrimaryPackages {
		raw, err := query("-W", "-f", "${Status}\t${Version}", "--", name)
		if err != nil {
			return fmt.Errorf("v3 PowerDNS package %s could not be verified: %w", name, err)
		}
		installed, ok := freshPDNSInstalledVersionV3(raw, version)
		if !ok {
			return fmt.Errorf("v3 PowerDNS package %s is not the tested PowerDNS %s build", name, dnsengineartifact.PDNSFreshPrimaryTestedSeries)
		}
		version = installed
	}
	path, err := query("--control-path", dnsengineartifact.PDNSFreshPrimaryPackages[0], "md5sums")
	if err != nil {
		return fmt.Errorf("v3 PowerDNS package file digests could not be read: %w", err)
	}
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.Dir(path) != "/var/lib/dpkg/info" {
		return errors.New("v3 PowerDNS package file digests are outside the package database")
	}
	sums, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	digest, ok := dnsengineartifact.PDNSPackageFileMD5(sums, dnsengineartifact.PDNSFreshPrimaryBinary)
	if !ok {
		return errors.New("v3 PowerDNS package does not record its daemon")
	}
	if err := verifyFreshPDNSExecutableV3(dnsengineartifact.PDNSFreshPrimaryBinary, digest); err != nil {
		return err
	}
	return ctx.Err()
}

// freshPDNSInstalledVersionV3 reads one dpkg-query "${Status}\t${Version}"
// record: accepted only for Debian's archive build of the tested series (no
// epoch, no backport, no PowerDNS repository or vendor build) at the same
// version as the other package (prior, "" for the first).
func freshPDNSInstalledVersionV3(raw, prior string) (string, bool) {
	installed, ok := strings.CutPrefix(raw, "install ok installed\t")
	if !ok || !debversion.Valid(installed) || !dnsengineartifact.PDNSFreshPrimaryVersionInTestedSeries(installed) || (prior != "" && installed != prior) {
		return "", false
	}
	return installed, true
}

// The caller supplies only the fixed vendor path and the digest dpkg recorded
// for it. Keeping the filesystem proof separate allows real-file regression
// checks without installing or changing a native DNS service.
func verifyFreshPDNSExecutableV3(binary, packageMD5 string) error {
	info, err := os.Lstat(binary)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0o755 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || st.Uid != 0 || st.Gid != 0 || st.Nlink != 1 {
		return errors.New("v3 PowerDNS executable identity changed")
	}
	file, err := os.Open(binary)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != packageMD5 {
		return errors.New("v3 PowerDNS executable differs from the installed package's own file")
	}
	again, err := file.Stat()
	if err != nil {
		return err
	}
	ast, aok := again.Sys().(*syscall.Stat_t)
	if !aok || ast.Dev != st.Dev || ast.Ino != st.Ino || ast.Size != st.Size || ast.Mtim != st.Mtim || ast.Ctim != st.Ctim {
		return errors.New("v3 PowerDNS executable changed while hashing")
	}
	return nil
}
