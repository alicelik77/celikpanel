package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Which DKIM keys and table lines CelikPanel made (30 Oct 2026).
//
// Every domain that can send mail from this server now gets its DKIM key
// automatically, and a deleted domain's key goes with it. Removal must touch
// only what CelikPanel made (D-022, invariant 1): a key the owner placed in
// the store, or a line the owner added to the OpenDKIM tables, is kept and
// named. So a key CelikPanel generates carries an origin record next to it,
// <selector>.celikpanel, naming the SHA-256 of its public key. A key without
// a matching record (the owner's, or one generated before this release) is
// never removed by CelikPanel.
//
// The key and signing tables are regenerated from the key store; a line in
// them that is not exactly the form CelikPanel writes is the owner's and is
// carried over unchanged after CelikPanel's own lines.
//
// Hangi DKIM anahtarlarını ve tablo satırlarını CelikPanel üretti. Üretilen
// her anahtarın yanında, genel anahtarın SHA-256'sını taşıyan bir köken
// kaydı durur; eşleşen kaydı olmayan anahtar (sahibinki ya da bu sürümden
// önce üretilmiş olan) CelikPanel tarafından asla silinmez. Tablolarda
// CelikPanel'in yazdığı biçimde olmayan satır sahibindir ve korunur.

const dkimOriginPrefix = "celikpanel-dkim-key v1 sha256:"

func dkimOriginPath(keyPath string) string {
	return strings.TrimSuffix(keyPath, ".private") + ".celikpanel"
}

func dkimPublicDigest(publicKeyB64 string) (string, error) {
	der, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:]), nil
}

func dkimOriginContent(publicKeyB64 string) ([]byte, error) {
	digest, err := dkimPublicDigest(publicKeyB64)
	if err != nil {
		return nil, err
	}
	return []byte(dkimOriginPrefix + digest + "\n"), nil
}

// dkimKeyMadeByCelikPanel reports whether the origin record next to the key
// names exactly this public key.
func dkimKeyMadeByCelikPanel(keyPath, publicKeyB64 string) (bool, error) {
	want, err := dkimOriginContent(publicKeyB64)
	if err != nil {
		return false, err
	}
	got, err := secureReadConfig(dkimOriginPath(keyPath))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return string(got) == string(want), nil
}

// dkimTableLines are the key-table and signing-table lines CelikPanel writes
// for one domain.
func dkimTableLines(domain string) (keyLine, signingLine string, err error) {
	key, err := dkimKeyPath(domain, signingSelector)
	if err != nil {
		return "", "", err
	}
	keyLine = fmt.Sprintf("%s._domainkey.%s %s:%s:%s", signingSelector, domain, domain, signingSelector, key)
	signingLine = fmt.Sprintf("*@%s %s._domainkey.%s", domain, signingSelector, domain)
	return keyLine, signingLine, nil
}

// dkimLineIsCelikPanels: a line exactly in the form CelikPanel writes for
// some valid domain (whitespace between the two fields is normalised).
func dkimLineIsCelikPanels(line string, signing bool) bool {
	fields := strings.Fields(line)
	if len(fields) != 2 {
		return false
	}
	var domain string
	if signing {
		if !strings.HasPrefix(fields[0], "*@") {
			return false
		}
		domain = strings.TrimPrefix(fields[0], "*@")
	} else {
		prefix := signingSelector + "._domainkey."
		if !strings.HasPrefix(fields[0], prefix) {
			return false
		}
		domain = strings.TrimPrefix(fields[0], prefix)
	}
	keyLine, signingLine, err := dkimTableLines(domain)
	if err != nil {
		return false
	}
	normalised := fields[0] + " " + fields[1]
	if signing {
		return normalised == signingLine
	}
	return normalised == keyLine
}

// dkimOwnerLines returns, in order, the lines of a table that CelikPanel did
// not write (blank lines dropped).
func dkimOwnerLines(content []byte, signing bool) []string {
	var owner []string
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || dkimLineIsCelikPanels(trimmed, signing) {
			continue
		}
		owner = append(owner, strings.TrimRight(line, "\r"))
	}
	return owner
}

// renderDKIMTables renders both tables: CelikPanel's lines for every domain
// with a key, then the owner's lines carried over unchanged. Ours come first
// so a domain CelikPanel signs keeps its signature; an owner line for another
// domain or key is untouched.
func renderDKIMTables(existingKeyTable, existingSigningTable []byte, domains []string) (keyTable, signingTable []byte, ownerLines int, err error) {
	var kt, st strings.Builder
	for _, domain := range domains {
		keyLine, signingLine, lineErr := dkimTableLines(domain)
		if lineErr != nil {
			return nil, nil, 0, fmt.Errorf("invalid DKIM domain %q: %w", domain, lineErr)
		}
		kt.WriteString(keyLine + "\n")
		st.WriteString(signingLine + "\n")
	}
	ownerKeys := dkimOwnerLines(existingKeyTable, false)
	ownerSigning := dkimOwnerLines(existingSigningTable, true)
	for _, line := range ownerKeys {
		kt.WriteString(line + "\n")
	}
	for _, line := range ownerSigning {
		st.WriteString(line + "\n")
	}
	return []byte(kt.String()), []byte(st.String()), len(ownerKeys) + len(ownerSigning), nil
}

// dkimLineNamesDomain: an owner line that mentions the domain as a whole
// name (signing pattern, key name, or key table value).
func dkimLineNamesDomain(line, domain string) bool {
	for _, field := range strings.Fields(line) {
		field = strings.ToLower(field)
		for _, cut := range []string{"*@", "@"} {
			field = strings.TrimPrefix(field, cut)
		}
		if i := strings.IndexByte(field, ':'); i >= 0 {
			field = field[:i]
		}
		if field == domain || strings.HasSuffix(field, "._domainkey."+domain) {
			return true
		}
	}
	return false
}

func readOptionalDKIMTable(path string) ([]byte, bool, error) {
	content, err := secureReadConfig(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read OpenDKIM table %s: %w", path, err)
	}
	return content, true, nil
}

// dkimDomainSigned reports whether CelikPanel's signing and key tables list
// the domain in CelikPanel's own form.
func dkimDomainSigned(domain string) (bool, error) {
	keyLine, signingLine, err := dkimTableLines(domain)
	if err != nil {
		return false, err
	}
	signing, _, err := readOptionalDKIMTable(filepath.Join(opendkimTablesDir, "signingtable"))
	if err != nil {
		return false, err
	}
	keys, _, err := readOptionalDKIMTable(filepath.Join(opendkimTablesDir, "keytable"))
	if err != nil {
		return false, err
	}
	return dkimTableHasLine(signing, signingLine) && dkimTableHasLine(keys, keyLine), nil
}

func dkimTableHasLine(content []byte, want string) bool {
	for _, line := range strings.Split(string(content), "\n") {
		if strings.Join(strings.Fields(line), " ") == want {
			return true
		}
	}
	return false
}

// RemoveDKIMKey removes the key CelikPanel made for a domain being deleted
// and its origin record. A key without a matching origin record is kept and
// named, and so are owner lines in the tables that name the domain. The
// tables themselves are regenerated by ConfigureDKIMSigning, which the panel
// runs next under its own lease when SigningConfigured is true.
// RemoveDKIMKey, silinen alan adı için CelikPanel'in ürettiği anahtarı ve
// köken kaydını kaldırır; eşleşmeyen anahtar ve sahibin tablo satırları
// korunur ve bildirilir.
func (a *Agent) RemoveDKIMKey(req *transport.DKIMRemoveRequest, resp *transport.DKIMRemoveResponse) error {
	if req == nil || resp == nil {
		return fmt.Errorf("DKIM key removal request and response are required")
	}
	*resp = transport.DKIMRemoveResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "removing a domain's DKIM key"); err != nil {
		return err
	}
	keyPath, err := dkimKeyPath(req.Domain, req.Selector)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}

	dkimKeyMu.Lock()
	defer dkimKeyMu.Unlock()

	if err := ensureDKIMStorageMigrated(); err != nil {
		resp.Error = err.Error()
		return nil
	}
	originPath := dkimOriginPath(keyPath)
	key, readErr := readDKIMKey(keyPath)
	switch {
	case errors.Is(readErr, os.ErrNotExist):
		resp.Absent = true
		// A stray origin record of ours (the key write never finished).
		if err := secureRemoveConfig(originPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			resp.Error = fmt.Sprintf("remove the DKIM origin record: %v", err)
			return nil
		}
	case readErr != nil:
		// Unreadable or malformed: never removed blind.
		resp.KeptKey = keyPath
	default:
		pub, err := dkimPublicB64(key)
		if err != nil {
			resp.Error = err.Error()
			return nil
		}
		ours, err := dkimKeyMadeByCelikPanel(keyPath, pub)
		if err != nil {
			resp.Error = fmt.Sprintf("read the DKIM origin record: %v", err)
			return nil
		}
		if !ours {
			resp.KeptKey = keyPath
			break
		}
		if err := secureRemoveConfig(keyPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			resp.Error = fmt.Sprintf("remove the DKIM key: %v", err)
			return nil
		}
		if err := secureRemoveConfig(originPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			resp.Error = fmt.Sprintf("remove the DKIM origin record: %v", err)
			return nil
		}
		resp.Removed = true
	}
	if resp.KeptKey == "" {
		// The domain's directory goes when nothing else is in it; anything
		// the owner put there stays (and keeps the directory).
		dir := filepath.Dir(keyPath)
		if err := rejectSymlinkPath(dir); err == nil {
			_ = os.Remove(dir)
		}
	}

	domain := strings.ToLower(req.Domain)
	for _, table := range []struct {
		name    string
		signing bool
	}{{"keytable", false}, {"signingtable", true}} {
		content, exists, err := readOptionalDKIMTable(filepath.Join(opendkimTablesDir, table.name))
		if err != nil {
			resp.Error = err.Error()
			return nil
		}
		if table.signing {
			resp.SigningConfigured = exists
		}
		for _, line := range dkimOwnerLines(content, table.signing) {
			if dkimLineNamesDomain(line, domain) {
				resp.OwnerTableLines = append(resp.OwnerTableLines, table.name+": "+strings.TrimSpace(line))
			}
		}
	}
	return nil
}
