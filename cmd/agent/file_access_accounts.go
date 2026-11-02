package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/fileaccess"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Local account files. Hosting accounts are always local (useradd), so the
// files are read directly instead of through NSS.
var (
	fileAccessPasswdPath = "/etc/passwd"
	fileAccessGroupPath  = "/etc/group"
	fileAccessShadowPath = "/etc/shadow"
)

type passwdLine struct {
	name     string
	uid, gid int
	home     string
	shell    string
}

func readPasswdLines() ([]passwdLine, error) {
	content, err := os.ReadFile(fileAccessPasswdPath)
	if err != nil {
		return nil, err
	}
	var lines []passwdLine
	for _, raw := range strings.Split(string(content), "\n") {
		fields := strings.Split(raw, ":")
		if len(fields) != 7 {
			continue
		}
		uid, err1 := strconv.Atoi(fields[2])
		gid, err2 := strconv.Atoi(fields[3])
		if err1 != nil || err2 != nil {
			continue
		}
		lines = append(lines, passwdLine{name: fields[0], uid: uid, gid: gid, home: fields[5], shell: fields[6]})
	}
	return lines, nil
}

// readGroupFile returns gid -> name and the members of the site group.
func readGroupFile() (map[int]string, map[string]bool, error) {
	content, err := os.ReadFile(fileAccessGroupPath)
	if err != nil {
		return nil, nil, err
	}
	names := map[int]string{}
	members := map[string]bool{}
	for _, raw := range strings.Split(string(content), "\n") {
		fields := strings.Split(raw, ":")
		if len(fields) != 4 {
			continue
		}
		if gid, err := strconv.Atoi(fields[2]); err == nil {
			names[gid] = fields[0]
		}
		if fields[0] == fileaccess.SiteGroup {
			for _, member := range strings.Split(fields[3], ",") {
				if member = strings.TrimSpace(member); member != "" {
					members[member] = true
				}
			}
		}
	}
	return names, members, nil
}

// readShadowStates returns each account's password state, never the hash.
func readShadowStates() (map[string]fileaccess.PasswordState, error) {
	content, err := os.ReadFile(fileAccessShadowPath)
	if err != nil {
		return nil, err
	}
	states := map[string]fileaccess.PasswordState{}
	for _, raw := range strings.Split(string(content), "\n") {
		name, rest, ok := strings.Cut(raw, ":")
		if !ok {
			continue
		}
		field, _, _ := strings.Cut(rest, ":")
		states[name] = fileaccess.ClassifyShadowPassword(field)
	}
	return states, nil
}

func lookupFileAccessAccount(name string) (fileAccessAccount, error) {
	lines, err := readPasswdLines()
	if err != nil {
		return fileAccessAccount{}, err
	}
	for _, line := range lines {
		if line.name != name {
			continue
		}
		groups, members, err := readGroupFile()
		if err != nil {
			return fileAccessAccount{}, err
		}
		states, err := readShadowStates()
		if err != nil {
			return fileAccessAccount{}, err
		}
		state, ok := states[name]
		if !ok {
			state = fileaccess.PasswordNone
		}
		return fileAccessAccount{
			Name: line.name, UID: line.uid, GID: line.gid, Group: groups[line.gid],
			Home: line.home, Shell: line.shell, Password: state, InSiteGroup: members[name],
		}, nil
	}
	return fileAccessAccount{}, errFileAccessNoAccount
}

// listFileAccessSiteAccounts lists every hosting account: uid >= 1000 and a
// home under the subscriptions root.
func listFileAccessSiteAccounts() ([]fileAccessAccount, error) {
	lines, err := readPasswdLines()
	if err != nil {
		return nil, err
	}
	_, members, err := readGroupFile()
	if err != nil {
		return nil, err
	}
	var accounts []fileAccessAccount
	for _, line := range lines {
		if line.uid < 1000 || !isFileAccessSiteHome(line.home) {
			continue
		}
		accounts = append(accounts, fileAccessAccount{
			Name: line.name, UID: line.uid, GID: line.gid, Home: line.home, Shell: line.shell, InSiteGroup: members[line.name],
		})
	}
	return accounts, nil
}

// ---------------------------------------------------------------------------
// FTP certificate

var (
	fileAccessMailHostCert = fileaccess.MailHostCert
	fileAccessMailHostKey  = fileaccess.MailHostKey
	fileAccessSelfCert     = fileaccess.SelfSignedCert
	fileAccessSelfKey      = fileaccess.SelfSignedKey
	fileAccessNow          = time.Now
)

// chooseFileAccessCertificate picks the FTP certificate: the server's
// trusted mail host certificate when it is there and valid, otherwise a
// self-signed one (made when create is true). It returns the kind and the
// paths; empty paths mean none is available.
func chooseFileAccessCertificate(create bool) (string, string, string) {
	if certificateUsable(fileAccessMailHostCert, fileAccessMailHostKey) {
		return transport.FileAccessCertMailHost, fileAccessMailHostCert, fileAccessMailHostKey
	}
	if certificateUsable(fileAccessSelfCert, fileAccessSelfKey) {
		return transport.FileAccessCertSelfSigned, fileAccessSelfCert, fileAccessSelfKey
	}
	if !create {
		return transport.FileAccessCertNone, "", ""
	}
	if err := writeFileAccessSelfSigned(); err != nil {
		return transport.FileAccessCertNone, "", ""
	}
	return transport.FileAccessCertSelfSigned, fileAccessSelfCert, fileAccessSelfKey
}

func readLeafCertificate(path string) (*x509.Certificate, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(content)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("no PEM certificate")
	}
	return x509.ParseCertificate(block.Bytes)
}

func certificateUsable(certPath, keyPath string) bool {
	leaf, err := readLeafCertificate(certPath)
	if err != nil {
		return false
	}
	if info, err := os.Stat(keyPath); err != nil || !info.Mode().IsRegular() {
		return false
	}
	now := fileAccessNow()
	return now.After(leaf.NotBefore) && now.Before(leaf.NotAfter)
}

// describeFileAccessCertificate returns the name an FTP client must connect
// to for the certificate to match, and its SHA-256 fingerprint for comparing
// with what the client shows.
func describeFileAccessCertificate(path string) (string, string) {
	leaf, err := readLeafCertificate(path)
	if err != nil {
		return "", ""
	}
	sum := sha256.Sum256(leaf.Raw)
	name := leaf.Subject.CommonName
	if len(leaf.DNSNames) > 0 {
		name = leaf.DNSNames[0]
	}
	return name, strings.ToUpper(hex.EncodeToString(sum[:]))
}

func writeFileAccessSelfSigned() error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "localhost"
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		return err
	}
	now := fileAccessNow()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     []string{host},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.AddDate(3, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	if err := fileAccessEnsureDir(fileaccess.SelfSignedDir, 0o700); err != nil {
		return err
	}
	if err := fileAccessWriteFile(fileAccessSelfKey, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return fmt.Errorf("write the FTP key: %w", err)
	}
	return fileAccessWriteFile(fileAccessSelfCert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
}
