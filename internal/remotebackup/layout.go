package remotebackup

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Object layout under a destination's base (bucket prefix or SFTP folder):
//
//	<server>/sites/<domain>-d<domain id>/manual/<backup name>.gpg
//	<server>/sites/<domain>-d<domain id>/schedule-<policy id>/<backup name>.gpg
//
// and beside each object "<object>.json", the side file: plain metadata a
// person reads without CelikPanel (which site, when, sizes, SHA-256 of the
// encrypted and of the decrypted file, how to decrypt). <server> is this
// server's stable tag, so several servers can share one bucket.
//
// Nesne düzeni: sunucu/sites/alanadı-dKİMLİK/(manual|schedule-N)/yedek.gpg
// ve yanında okunabilir .json yan dosyası.

const (
	// SidecarSuffix ends the side file's key.
	SidecarSuffix = ".json"
	// GroupManual holds backups a person sent.
	GroupManual = "manual"
	// schedulePrefix + policy id holds a schedule's backups (its retention
	// only ever looks there).
	schedulePrefix = "schedule-"
)

var (
	serverTagPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,62}$`)
	domainNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,252}$`)
	backupNamePattern = regexp.MustCompile(`^(files|full)-[0-9]{8}T[0-9]{6}\.[0-9]{9}Z-[0-9a-f]{16}\.cpbak$|^database-[1-9][0-9]*-[0-9]{8}T[0-9]{6}\.[0-9]{9}Z-[0-9a-f]{16}\.cpbak$`)
	siteDirPattern    = regexp.MustCompile(`^([a-z0-9][a-z0-9.-]*)-d([1-9][0-9]{0,17})$`)
)

// ServerTag turns a host name into the server's folder name.
func ServerTag(hostname string) string {
	tag := strings.ToLower(strings.TrimSpace(hostname))
	var b strings.Builder
	for _, r := range tag {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	tag = strings.Trim(b.String(), ".-")
	if len(tag) > 63 {
		tag = strings.Trim(tag[:63], ".-")
	}
	if tag == "" {
		tag = "server"
	}
	return tag
}

// ValidBackupName: a name the backup engine generates (immutable package).
func ValidBackupName(name string) bool { return backupNamePattern.MatchString(name) }

// GroupForPolicy names the folder of one schedule's backups.
func GroupForPolicy(policyID int64) string { return schedulePrefix + strconv.FormatInt(policyID, 10) }

// SiteFolder is the folder of one site's backups on a destination.
func SiteFolder(serverTag, domain string, domainID int) (string, error) {
	if !serverTagPattern.MatchString(serverTag) {
		return "", errors.New("invalid server tag")
	}
	domain = strings.ToLower(domain)
	if !domainNamePattern.MatchString(domain) || domainID < 1 {
		return "", errors.New("invalid site")
	}
	return serverTag + "/sites/" + domain + "-d" + strconv.Itoa(domainID), nil
}

// ObjectKey is the key of one encrypted backup.
func ObjectKey(serverTag, domain string, domainID int, group, backupName string) (string, error) {
	folder, err := SiteFolder(serverTag, domain, domainID)
	if err != nil {
		return "", err
	}
	if group != GroupManual && !strings.HasPrefix(group, schedulePrefix) {
		return "", errors.New("invalid group")
	}
	if strings.HasPrefix(group, schedulePrefix) {
		if id, err := strconv.ParseInt(strings.TrimPrefix(group, schedulePrefix), 10, 64); err != nil || id < 1 ||
			GroupForPolicy(id) != group {
			return "", errors.New("invalid group")
		}
	}
	if !ValidBackupName(backupName) {
		return "", errors.New("invalid backup name")
	}
	return folder + "/" + group + "/" + backupName + EncryptedSuffix, nil
}

// ParsedKey is what an object key says.
type ParsedKey struct {
	ServerTag  string
	Domain     string
	DomainID   int
	Group      string
	PolicyID   int64 // 0 for GroupManual
	BackupName string
}

// ParseObjectKey reads a key written by ObjectKey; anything else is refused.
func ParseObjectKey(key string) (ParsedKey, error) {
	parts := strings.Split(key, "/")
	if len(parts) != 5 || parts[1] != "sites" || !strings.HasSuffix(parts[4], EncryptedSuffix) {
		return ParsedKey{}, errors.New("not a backup object")
	}
	m := siteDirPattern.FindStringSubmatch(parts[2])
	if m == nil {
		return ParsedKey{}, errors.New("not a backup object")
	}
	id, err := strconv.Atoi(m[2])
	if err != nil || id < 1 {
		return ParsedKey{}, errors.New("not a backup object")
	}
	out := ParsedKey{ServerTag: parts[0], Domain: m[1], DomainID: id, Group: parts[3],
		BackupName: strings.TrimSuffix(parts[4], EncryptedSuffix)}
	if out.Group != GroupManual {
		pid, err := strconv.ParseInt(strings.TrimPrefix(out.Group, schedulePrefix), 10, 64)
		if !strings.HasPrefix(out.Group, schedulePrefix) || err != nil || pid < 1 {
			return ParsedKey{}, errors.New("not a backup object")
		}
		out.PolicyID = pid
	}
	if again, err := ObjectKey(out.ServerTag, out.Domain, out.DomainID, out.Group, out.BackupName); err != nil || again != key {
		return ParsedKey{}, errors.New("not a backup object")
	}
	return out, nil
}

// BackupCreatedAt reads the creation time the engine wrote into the name.
func BackupCreatedAt(name string) (time.Time, bool) {
	i := strings.Index(name, "T")
	if i < 8 || len(name) < i+18 {
		return time.Time{}, false
	}
	stamp := name[i-8 : i+18] // 20060102T150405.000000000Z
	t, err := time.Parse("20060102T150405.000000000Z", stamp)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// Sidecar is the plain metadata beside an encrypted backup. It holds no
// secret: the passphrase check is a short non-reversible tag.
type Sidecar struct {
	Format          string    `json:"format"`
	DecryptWith     string    `json:"decrypt_with"`
	Server          string    `json:"server"`
	Domain          string    `json:"domain"`
	DomainID        int       `json:"domain_id"`
	SubscriptionID  int       `json:"subscription_id"`
	BackupName      string    `json:"backup_name"`
	Type            string    `json:"type"`
	Origin          string    `json:"origin"`
	CreatedAt       time.Time `json:"created_at"`
	UploadedAt      time.Time `json:"uploaded_at"`
	PlainSize       int64     `json:"plain_size"`
	PlainSHA256     string    `json:"plain_sha256"`
	EncryptedSize   int64     `json:"encrypted_size"`
	EncryptedSHA256 string    `json:"encrypted_sha256"`
	PassphraseCheck string    `json:"passphrase_check"`
	PanelVersion    string    `json:"panel_version,omitempty"`
}

// DecryptCommand is the command the side file names.
const DecryptCommand = "gpg --batch --pinentry-mode loopback --passphrase-file PASSFILE --output BACKUP.cpbak --decrypt BACKUP.cpbak.gpg; then: tar -xzf BACKUP.cpbak (manifest.json, payload/files.tar.gz, payload/databases/ID.sql.gz)"

// MarshalSidecar encodes a side file (indented: people read it).
func MarshalSidecar(s Sidecar) ([]byte, error) {
	s.Format = FormatName
	s.DecryptWith = DecryptCommand
	return json.MarshalIndent(s, "", "  ")
}

// ParseSidecar decodes a side file, bounded.
func ParseSidecar(data []byte) (Sidecar, error) {
	var s Sidecar
	if len(data) > 64<<10 {
		return s, errors.New("side file too large")
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("side file: %w", err)
	}
	if s.Format != FormatName {
		return s, errors.New("side file names an unknown format")
	}
	return s, nil
}
