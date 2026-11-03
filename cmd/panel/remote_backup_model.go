package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/remotebackup"
)

// Remote backups (alpha.118, owner request "uzak yedek", docs/REMOTE-BACKUPS.md).
//
// A backup on the server's own disk does not survive losing the server. A
// destination is storage the owner controls elsewhere: S3-compatible object
// storage or an SFTP server. Every object is encrypted before it leaves the
// server with a passphrase the owner keeps (OpenPGP, opened by the
// distribution's gpg without CelikPanel). Sending on a schedule is Panel
// automation; the local backups and their schedules work exactly as before,
// and what was sent stays usable if the Panel is removed.
//
// Who (D-042): the administrator defines server-wide destinations and may use
// them for every site; a server-wide destination may be offered to accounts.
// A reseller or customer may add an own destination only with the permission
// "remote_backup_destinations". The server enforces every rule here.
//
// Uzak yedek: hedef, sahibin başka yerdeki depolamasıdır. Her nesne gönderilmeden
// sahibin tuttuğu parolayla şifrelenir. Yerel yedekler değişmez.

const (
	rightRemoteBackupDestinations = "remote_backup_destinations"

	remoteDestinationKindS3   = "s3"
	remoteDestinationKindSFTP = "sftp"

	settingRemoteBackupServerTag = "remote_backup_server_tag"

	remoteBackupTestTimeout = 90 * time.Second
)

// remoteDestinationConfig holds the settings that are not secret.
type remoteDestinationConfig struct {
	Endpoint      string `json:"endpoint,omitempty"`
	Region        string `json:"region,omitempty"`
	Bucket        string `json:"bucket,omitempty"`
	Prefix        string `json:"prefix,omitempty"`
	AccessKeyID   string `json:"access_key_id,omitempty"`
	VirtualHosted bool   `json:"virtual_hosted,omitempty"`
	Host          string `json:"host,omitempty"`
	Port          int    `json:"port,omitempty"`
	User          string `json:"user,omitempty"`
	Path          string `json:"path,omitempty"`
}

// remoteDestinationSecret is sealed as one value; never sent to a screen.
type remoteDestinationSecret struct {
	SecretAccessKey string `json:"secret_access_key,omitempty"`
	Password        string `json:"password,omitempty"`
	PrivateKey      string `json:"private_key,omitempty"`
}

type remoteDestination struct {
	ID                int64
	OwnerUserID       int64 // 0 = server-wide (administrator)
	Name              string
	Kind              string
	Config            remoteDestinationConfig
	secretSealed      string
	passphraseSealed  string
	PassphraseCheck   string
	HostKey           string
	OfferedToAccounts bool
	LastTestAt        string
	LastTestResult    string
	CreatedAt         string
}

func (d remoteDestination) serverWide() bool { return d.OwnerUserID == 0 }

const remoteDestinationColumns = `id, COALESCE(owner_user_id, 0), name, kind, config_json, secret_sealed,
	passphrase_sealed, passphrase_check, host_key, offered_to_accounts, COALESCE(last_test_at, ''),
	last_test_result, created_at`

func scanRemoteDestination(row interface{ Scan(...any) error }) (remoteDestination, error) {
	var d remoteDestination
	var configJSON string
	var offered int
	if err := row.Scan(&d.ID, &d.OwnerUserID, &d.Name, &d.Kind, &configJSON, &d.secretSealed,
		&d.passphraseSealed, &d.PassphraseCheck, &d.HostKey, &offered, &d.LastTestAt,
		&d.LastTestResult, &d.CreatedAt); err != nil {
		return d, err
	}
	if err := json.Unmarshal([]byte(configJSON), &d.Config); err != nil {
		return d, fmt.Errorf("remote backup destination %d settings are unreadable", d.ID)
	}
	d.OfferedToAccounts = offered == 1
	return d, nil
}

func (p *Panel) loadRemoteDestination(ctx context.Context, id int64) (remoteDestination, error) {
	return scanRemoteDestination(p.db.GetDB().QueryRowContext(ctx,
		`SELECT `+remoteDestinationColumns+` FROM remote_backup_destinations WHERE id = ?`, id))
}

func (p *Panel) remoteDestinationSecrets(d remoteDestination) (remoteDestinationSecret, string, error) {
	if p.secrets == nil {
		return remoteDestinationSecret{}, "", errors.New("the panel's secret key is not available")
	}
	var secret remoteDestinationSecret
	if d.secretSealed != "" {
		plain, err := p.secrets.Decrypt(d.secretSealed)
		if err != nil {
			return remoteDestinationSecret{}, "", errors.New("the destination's stored credentials cannot be opened with this panel's key")
		}
		if err := json.Unmarshal([]byte(plain), &secret); err != nil {
			return remoteDestinationSecret{}, "", errors.New("the destination's stored credentials are unreadable")
		}
	}
	passphrase, err := p.secrets.Decrypt(d.passphraseSealed)
	if err != nil {
		return remoteDestinationSecret{}, "", errors.New("the destination's stored passphrase cannot be opened with this panel's key")
	}
	return secret, passphrase, nil
}

// openRemoteBackupStore connects to a destination. Tests replace it.
var openRemoteBackupStore = func(ctx context.Context, kind string, cfg remoteDestinationConfig, secret remoteDestinationSecret, hostKey string) (remotebackup.Store, error) {
	switch kind {
	case remoteDestinationKindS3:
		return remotebackup.NewS3(remotebackup.S3Config{
			Endpoint: cfg.Endpoint, Region: cfg.Region, Bucket: cfg.Bucket, Prefix: cfg.Prefix,
			AccessKeyID: cfg.AccessKeyID, SecretAccessKey: secret.SecretAccessKey, VirtualHosted: cfg.VirtualHosted,
		})
	case remoteDestinationKindSFTP:
		return remotebackup.DialSFTP(ctx, remotebackup.SFTPConfig{
			Host: cfg.Host, Port: cfg.Port, User: cfg.User, Password: secret.Password,
			PrivateKey: []byte(secret.PrivateKey), BasePath: cfg.Path, HostKey: hostKey,
		})
	}
	return nil, &remotebackup.Error{Kind: remotebackup.KindConfig, Op: "settings", Message: "unknown destination type"}
}

// probeRemoteHostKey learns an SFTP server's key. Tests replace it.
var probeRemoteHostKey = func(ctx context.Context, host string, port int) (remotebackup.SFTPHostKey, error) {
	return remotebackup.ProbeSFTPHostKey(ctx, host, port, 20*time.Second)
}

func (p *Panel) openDestination(ctx context.Context, d remoteDestination) (remotebackup.Store, string, error) {
	secret, passphrase, err := p.remoteDestinationSecrets(d)
	if err != nil {
		return nil, "", &remotebackup.Error{Kind: remotebackup.KindConfig, Op: "settings", Message: err.Error()}
	}
	store, err := openRemoteBackupStore(ctx, d.Kind, d.Config, secret, d.HostKey)
	if err != nil {
		return nil, "", err
	}
	return store, passphrase, nil
}

// normalizeRemoteConfig trims and checks the settings of one kind and drops
// the other kind's fields. The store constructors check them again.
func normalizeRemoteConfig(kind string, in remoteDestinationConfig) (remoteDestinationConfig, error) {
	switch kind {
	case remoteDestinationKindS3:
		out := remoteDestinationConfig{
			Endpoint: strings.TrimRight(strings.TrimSpace(in.Endpoint), "/"), Region: strings.TrimSpace(in.Region),
			Bucket: strings.TrimSpace(in.Bucket), Prefix: strings.Trim(strings.TrimSpace(in.Prefix), "/"),
			AccessKeyID: strings.TrimSpace(in.AccessKeyID), VirtualHosted: in.VirtualHosted,
		}
		if out.Endpoint == "" || out.Bucket == "" || out.AccessKeyID == "" {
			return out, errors.New("endpoint, bucket and access key ID are required")
		}
		return out, nil
	case remoteDestinationKindSFTP:
		out := remoteDestinationConfig{
			Host: strings.TrimSpace(in.Host), Port: in.Port, User: strings.TrimSpace(in.User),
			Path: strings.TrimSpace(in.Path),
		}
		if out.Port == 0 {
			out.Port = 22
		}
		if out.Host == "" || out.User == "" || out.Path == "" {
			return out, errors.New("host, user and folder are required")
		}
		return out, nil
	}
	return remoteDestinationConfig{}, errors.New("type must be s3 or sftp")
}

// remoteServerTag is this server's folder on every destination. It is fixed
// the first time it is needed, so a later host name change does not move
// where backups are found.
func (p *Panel) remoteServerTag(ctx context.Context) (string, error) {
	if tag := p.setting(ctx, settingRemoteBackupServerTag); tag != "" {
		return tag, nil
	}
	host, _ := os.Hostname()
	tag := remotebackup.ServerTag(host)
	if err := p.setSetting(ctx, settingRemoteBackupServerTag, tag); err != nil {
		return "", err
	}
	return tag, nil
}

func newRemoteBackupOperationID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "rb-" + hex.EncodeToString(b), nil
}

// ----- who may do what -----

// callerAccountRole: admin, reseller, customer, or "" (additional users and
// anything else never manage destinations).
func callerAccountRole(c *Caller) string {
	switch {
	case c.hasAccountRole(roleAdmin):
		return roleAdmin
	case c.hasAccountRole(roleReseller):
		return roleReseller
	case c.hasAccountRole(roleCustomer):
		return roleCustomer
	}
	return ""
}

// ownDestinationRefusal: "" when the caller may add and use an own
// destination; else the layer that refuses ("administrator", "reseller",
// "subscription", "role").
func (p *Panel) ownDestinationRefusal(ctx context.Context, c *Caller) (string, error) {
	switch callerAccountRole(c) {
	case roleAdmin:
		return "", nil
	case roleReseller:
		admin, err := p.loadAdminOfferLayer(ctx)
		if err != nil {
			return "", err
		}
		if !admin.permits(rightRemoteBackupDestinations) {
			return rightsLayerAdministrator, nil
		}
		state, err := resellerRightsState(ctx, p.db.GetDB(), c.ID)
		if err != nil {
			return "", err
		}
		if !state.Set.permits(rightRemoteBackupDestinations) {
			return rightsLayerReseller, nil
		}
		return "", nil
	case roleCustomer:
		rows, err := p.db.GetDB().QueryContext(ctx, `SELECT id FROM subscriptions WHERE owner_id = ? ORDER BY id`, c.ID)
		if err != nil {
			return "", err
		}
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return "", err
			}
			ids = append(ids, id)
		}
		rows.Close()
		refusal := rightsLayerSubscription
		for _, id := range ids {
			e, err := p.effectiveSubscriptionRights(ctx, int(id))
			if err != nil {
				return "", err
			}
			layer := e.permissionRefusal(rightRemoteBackupDestinations)
			if layer == "" {
				return "", nil
			}
			refusal = layer
		}
		return refusal, nil
	}
	return "role", nil
}

// destinationManageable: the caller may change or remove this destination.
func destinationManageable(c *Caller, d remoteDestination) bool {
	if d.serverWide() {
		return c.hasAccountRole(roleAdmin)
	}
	role := callerAccountRole(c)
	return role != "" && role != roleAdmin && int64(c.ID) == d.OwnerUserID
}

// destinationVisible: the caller may see this destination in a list (name and
// type for an offered one; settings only when manageable).
func destinationVisible(c *Caller, d remoteDestination) bool {
	if destinationManageable(c, d) {
		return true
	}
	return d.serverWide() && d.OfferedToAccounts && callerAccountRole(c) != ""
}

// remoteDomain is what the remote backup code needs of a site.
type remoteDomain struct {
	backupDomain
	OwnerID    int64
	ResellerID int64
}

func (p *Panel) loadRemoteDomain(ctx context.Context, domainID int) (remoteDomain, error) {
	d, err := p.loadBackupDomain(ctx, domainID)
	if err != nil {
		return remoteDomain{}, err
	}
	out := remoteDomain{backupDomain: d}
	var parentRole sql.NullString
	var parentID sql.NullInt64
	err = p.db.GetDB().QueryRowContext(ctx, `
		SELECT s.owner_id, u.parent_id, pu.role
		FROM subscriptions s JOIN users u ON u.id = s.owner_id
		LEFT JOIN users pu ON pu.id = u.parent_id
		WHERE s.id = ?`, d.SubscriptionID).Scan(&out.OwnerID, &parentID, &parentRole)
	if err != nil {
		return remoteDomain{}, err
	}
	if parentID.Valid && parentRole.String == roleReseller {
		out.ResellerID = parentID.Int64
	}
	return out, nil
}

// destinationUsableForSite answers whether a destination may hold this
// site's backups at all: a server-wide one (offered, or chosen by the
// administrator), the site owner's own, or its reseller's own. After a
// subscription transfer a previous owner's destination stops being usable,
// so the new owner's data never goes to someone else's storage.
func destinationUsableForSite(d remoteDestination, site remoteDomain, chosenByAdministrator bool) bool {
	switch {
	case d.serverWide():
		return d.OfferedToAccounts || chosenByAdministrator
	case d.OwnerUserID == site.OwnerID:
		return true
	case site.ResellerID != 0 && d.OwnerUserID == site.ResellerID:
		return true
	}
	return false
}

// destinationChoiceRefusal: "" when the caller may newly choose this
// destination for the site (a send, a schedule, a restore); else a reason
// code for the screen.
func (p *Panel) destinationChoiceRefusal(ctx context.Context, c *Caller, d remoteDestination, site remoteDomain) (string, error) {
	isAdmin := c.hasAccountRole(roleAdmin)
	if !destinationUsableForSite(d, site, isAdmin) {
		return "DESTINATION_NOT_AVAILABLE", nil
	}
	if d.serverWide() || isAdmin {
		return "", nil
	}
	// An own destination: the owner's right is checked for new use (D-042
	// P5: a right removed later does not stop schedules that run).
	if d.OwnerUserID == site.OwnerID {
		e, err := p.effectiveSubscriptionRights(ctx, site.SubscriptionID)
		if err != nil {
			return "", err
		}
		if e.permissionRefusal(rightRemoteBackupDestinations) != "" {
			return errCodeRightNotGranted, nil
		}
		return "", nil
	}
	refusal, err := p.ownDestinationRefusal(ctx, &Caller{ID: int(d.OwnerUserID), Role: roleReseller})
	if err != nil {
		return "", err
	}
	if refusal != "" {
		return errCodeRightNotGranted, nil
	}
	// A reseller's destination is chosen by that reseller.
	if int64(c.ID) != d.OwnerUserID {
		return "DESTINATION_NOT_AVAILABLE", nil
	}
	return "", nil
}

// destinationsForSite lists the destinations the caller may choose for a
// site, server-wide first.
func (p *Panel) destinationsForSite(ctx context.Context, c *Caller, site remoteDomain) ([]remoteDestination, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT `+remoteDestinationColumns+`
		FROM remote_backup_destinations
		WHERE owner_user_id IS NULL OR owner_user_id IN (?, ?)
		ORDER BY owner_user_id IS NOT NULL, name, id`, site.OwnerID, site.ResellerID)
	if err != nil {
		return nil, err
	}
	var all []remoteDestination
	for rows.Next() {
		d, err := scanRemoteDestination(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, d)
	}
	rows.Close()
	out := []remoteDestination{}
	for _, d := range all {
		refusal, err := p.destinationChoiceRefusal(ctx, c, d, site)
		if err != nil {
			return nil, err
		}
		if refusal == "" {
			out = append(out, d)
		}
	}
	return out, nil
}

// ----- the screen's view of a destination (no secret) -----

type remoteDestinationView struct {
	ID                 int64                    `json:"id"`
	Name               string                   `json:"name"`
	Kind               string                   `json:"kind"`
	Scope              string                   `json:"scope"` // server | own | reseller
	Manageable         bool                     `json:"manageable"`
	Config             *remoteDestinationConfig `json:"config,omitempty"`
	HasSecret          bool                     `json:"has_secret"`
	PassphraseCheck    string                   `json:"passphrase_check"`
	HostKeyFingerprint string                   `json:"host_key_fingerprint,omitempty"`
	OfferedToAccounts  bool                     `json:"offered_to_accounts"`
	LastTestAt         string                   `json:"last_test_at,omitempty"`
	LastTestResult     string                   `json:"last_test_result,omitempty"`
	Location           string                   `json:"location"`
}

func destinationLocation(d remoteDestination) string {
	switch d.Kind {
	case remoteDestinationKindS3:
		loc := d.Config.Bucket
		if d.Config.Prefix != "" {
			loc += "/" + d.Config.Prefix
		}
		return "s3://" + loc
	case remoteDestinationKindSFTP:
		return "sftp://" + d.Config.User + "@" + d.Config.Host + d.Config.Path
	}
	return ""
}

func remoteDestinationViewFor(c *Caller, d remoteDestination, siteOwnerID int64) remoteDestinationView {
	v := remoteDestinationView{
		ID: d.ID, Name: d.Name, Kind: d.Kind, Manageable: destinationManageable(c, d),
		PassphraseCheck: d.PassphraseCheck, OfferedToAccounts: d.OfferedToAccounts,
		HasSecret: d.secretSealed != "",
	}
	switch {
	case d.serverWide():
		v.Scope = "server"
	case d.OwnerUserID == siteOwnerID || d.OwnerUserID == int64(c.ID):
		v.Scope = "own"
	default:
		v.Scope = "reseller"
	}
	if v.Manageable {
		cfg := d.Config
		cfg.AccessKeyID = maskKeyID(cfg.AccessKeyID)
		v.Config = &cfg
		v.Location = destinationLocation(d)
		v.LastTestAt, v.LastTestResult = d.LastTestAt, d.LastTestResult
		if d.HostKey != "" {
			v.HostKeyFingerprint = remotebackup.HostKeyFingerprint(d.HostKey)
		}
	}
	return v
}

// maskKeyID shows the start and end of an access key ID only.
func maskKeyID(id string) string {
	if len(id) <= 8 {
		return "…"
	}
	return id[:4] + "…" + id[len(id)-4:]
}
