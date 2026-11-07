package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/netip"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/dbremote"
)

// The host side of remote access: the address filter (nft file, table and
// boot unit) and the engines' certificate.
//
// Uzak erişimin makine tarafı: adres süzgeci ve motorların sertifikası.

func parsePrefix(s string) (netip.Prefix, error) { return netip.ParsePrefix(s) }

// databaseRemoteRun runs one host command with its standard input; swapped
// by tests.
var databaseRemoteRun = func(name string, args []string, stdin string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	return cmd.CombinedOutput()
}

// databaseRemoteFilterFiles are the filter's files; swapped by tests.
var databaseRemoteFilterFiles = struct {
	nftPath, unitPath, guardPath, timerPath string
}{dbremote.NFTPath, dbremote.NFTUnitPath, dbremote.NFTGuardPath, dbremote.NFTTimerPath}

func databaseRemoteTablePresent(nftPath string) (bool, error) {
	out, err := databaseRemoteRun(nftPath, []string{"list", "tables"}, "")
	if err != nil {
		return false, fmt.Errorf("nft list tables: %s", firstLine(string(out)))
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "table" && fields[1] == "inet" && fields[2] == dbremote.NFTTable {
			return true, nil
		}
	}
	return false, nil
}

// databaseRemoteFilterLoaded reads (only) whether the filter's chain is in
// the kernel with its drop rules (review L2): the panel opens a port only
// while this holds.
func databaseRemoteFilterLoaded(nftPath string) bool {
	out, err := databaseRemoteRun(nftPath, []string{"list", "chain", "inet", dbremote.NFTTable, "input"}, "")
	return err == nil && strings.Contains(string(out), " drop")
}

// writeIfChanged writes one of the filter's files when its text differs.
func writeIfChanged(path, text string, mode os.FileMode) (bool, error) {
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, []byte(text)) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, secureWriteConfig(path, []byte(text), mode)
}

// writeDatabaseRemoteFilter makes the filter hold exactly ports: the file is
// checked by nft, written, loaded as one transaction; its guard script, boot
// unit and 30-second timer are written and enabled. No ports removes the
// table, the timer, the unit, the guard and the file.
func writeDatabaseRemoteFilter(nftPath string, ports []dbremote.NFTPort) error {
	text, err := dbremote.NFTFile(ports)
	if err != nil {
		return err
	}
	present, err := databaseRemoteTablePresent(nftPath)
	if err != nil {
		return err
	}
	files := databaseRemoteFilterFiles
	// The guard takes the same lock (re-review LOW 2): a guard run never
	// interleaves with a write or a removal.
	unlock, err := databaseRemoteLockFilter()
	if err != nil {
		return fmt.Errorf("the filter's lock could not be taken: %w", err)
	}
	defer unlock()
	if text == "" {
		// Closing: the rule file goes first, so a guard that runs after
		// this point loads nothing; then the timer, then the table.
		if err := os.Remove(files.nftPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		_, _ = databaseRemoteRun("systemctl", []string{"disable", "--now", dbremote.NFTTimerName}, "")
		if present {
			if out, err := databaseRemoteRun(nftPath, []string{"delete", "table", "inet", dbremote.NFTTable}, ""); err != nil {
				return fmt.Errorf("nft delete table: %s", firstLine(string(out)))
			}
		}
		reload := false
		for _, path := range []string{files.timerPath, files.unitPath} {
			if _, err := os.Stat(path); err == nil {
				if path == files.unitPath {
					_, _ = databaseRemoteRun("systemctl", []string{"disable", dbremote.NFTUnitName}, "")
				}
				if err := os.Remove(path); err != nil {
					return err
				}
				reload = true
			}
		}
		if reload {
			_, _ = databaseRemoteRun("systemctl", []string{"daemon-reload"}, "")
		}
		if err := os.Remove(files.guardPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	current, readErr := os.ReadFile(files.nftPath)
	if readErr != nil || string(current) != text || !present || !databaseRemoteFilterLoaded(nftPath) {
		if out, err := databaseRemoteRun(nftPath, []string{"--check", "-f", "-"}, text); err != nil {
			return fmt.Errorf("nft refused the filter: %s", firstLine(string(out)))
		}
		if err := os.MkdirAll(filepath.Dir(files.nftPath), 0o755); err != nil {
			return err
		}
		if err := secureWriteConfig(files.nftPath, []byte(text), 0o600); err != nil {
			return err
		}
		if out, err := databaseRemoteRun(nftPath, []string{"-f", files.nftPath}, ""); err != nil {
			return fmt.Errorf("nft did not load the filter: %s", firstLine(string(out)))
		}
	}
	guard, err := dbremote.NFTGuardScript(nftPath)
	if err != nil {
		return err
	}
	unit, err := dbremote.NFTUnit(nftPath)
	if err != nil {
		return err
	}
	changed := false
	for _, f := range []struct {
		path, text string
		mode       os.FileMode
	}{{files.guardPath, guard, 0o700}, {files.unitPath, unit, 0o644}, {files.timerPath, dbremote.NFTTimer(), 0o644}} {
		wrote, err := writeIfChanged(f.path, f.text, f.mode)
		if err != nil {
			return err
		}
		changed = changed || wrote
	}
	if changed {
		if out, err := databaseRemoteRun("systemctl", []string{"daemon-reload"}, ""); err != nil {
			return fmt.Errorf("systemctl daemon-reload: %s", firstLine(string(out)))
		}
	}
	if out, err := databaseRemoteRun("systemctl", []string{"enable", dbremote.NFTUnitName}, ""); err != nil {
		return fmt.Errorf("the filter is loaded but its boot unit could not be enabled: %s", firstLine(string(out)))
	}
	if out, err := databaseRemoteRun("systemctl", []string{"enable", "--now", dbremote.NFTTimerName}, ""); err != nil {
		return fmt.Errorf("the filter is loaded but its guard timer could not be started: %s", firstLine(string(out)))
	}
	if !databaseRemoteFilterLoaded(nftPath) {
		return errors.New("the filter was loaded but nft does not list it")
	}
	return nil
}

// databaseRemoteCertRoot holds the engines' certificates; swapped by tests.
var databaseRemoteCertRoot = "/etc/ssl/celikpanel-database"

var certHostName = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)

// ensureDatabaseRemoteCertificate answers the engine's certificate and key,
// making them once: ECDSA P-256, self-signed, ten years. A pair already
// there (CelikPanel's, or the owner's replacement) is never rewritten. The
// key is readable by root and the engine's group only.
func ensureDatabaseRemoteCertificate(engine, group, hostName string) (cert, key string, err error) {
	dir := filepath.Join(databaseRemoteCertRoot, engine)
	cert, key = filepath.Join(dir, "server.crt"), filepath.Join(dir, "server.key")
	_, certErr := os.Stat(cert)
	_, keyErr := os.Stat(key)
	if certErr == nil && keyErr == nil {
		return cert, key, nil
	}
	if certErr == nil || keyErr == nil {
		return "", "", fmt.Errorf("only one of %s and %s exists; CelikPanel does not replace half of a certificate pair", cert, key)
	}
	g, err := user.LookupGroup(group)
	if err != nil {
		return "", "", fmt.Errorf("the %s group does not exist, so the key could not be made readable by the engine", group)
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return "", "", err
	}
	if !certHostName.MatchString(hostName) {
		hostName = "localhost"
	}
	if err := os.MkdirAll(databaseRemoteCertRoot, 0o755); err != nil {
		return "", "", err
	}
	if err := os.Mkdir(dir, 0o750); err != nil && !errors.Is(err, os.ErrExist) {
		return "", "", err
	}
	if err := os.Chown(dir, 0, gid); err != nil {
		return "", "", err
	}
	if err := os.Chmod(dir, 0o750); err != nil {
		return "", "", err
	}
	certPEM, keyPEM, err := newDatabaseRemoteCertificate(hostName, time.Now())
	if err != nil {
		return "", "", err
	}
	if err := secureWriteConfigOwnedBy(key, keyPEM, 0o640, 0, uint32(gid)); err != nil {
		return "", "", fmt.Errorf("write %s: %w", key, err)
	}
	if err := secureWriteConfigOwnedBy(cert, certPEM, 0o644, 0, 0); err != nil {
		_ = os.Remove(key)
		return "", "", fmt.Errorf("write %s: %w", cert, err)
	}
	return cert, key, nil
}

// newDatabaseRemoteCertificate makes the PEM pair.
func newDatabaseRemoteCertificate(hostName string, now time.Time) (certPEM, keyPEM []byte, err error) {
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return nil, nil, err
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: hostName, Organization: []string{"CelikPanel database TLS"}},
		DNSNames:              []string{hostName},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &private.PublicKey, private)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), nil
}

// databaseRemoteFilterLockPath is the lock the guard script shares
// (dbremote.NFTLockPath); swapped by tests.
var databaseRemoteFilterLockPath = dbremote.NFTLockPath

// databaseRemoteLockFilter takes that lock and answers its release. Set per
// platform (flock on Linux); swapped by tests.
var databaseRemoteLockFilter = func() (func(), error) { return func() {}, nil }
