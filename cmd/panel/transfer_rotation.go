package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Rotation of a transferred subscription's passwords and keys (owner
// decision, 2 November 2026; docs/PLANS-AND-RIGHTS.md section 8).
//
// Order. The owner change commits first, in one transaction with the
// transfer row: the old owner loses panel access at once, and in the same
// transaction the subscription's VPN peers are marked revoked and this
// operation with its checkpoints is written. Then the worker changes the
// server. Doing it the other way round would leave a half-rotated
// subscription with its old owner still able to set passwords in the panel.
//
// Steps, in this order, one checkpoint row each:
//
//	site_account   file access closed, password removed, every SSH key removed
//	               (first: a shell could read the new passwords below)
//	vpn            the revoked peers leave the live WireGuard configuration
//	mailbox        a random password nobody knows; open IMAP/POP3 sessions end
//	database_user  a random password on the engine, its sessions end, and the
//	               wp-config.php of a WordPress CelikPanel installed gets it
//	protected_dir  every password-protected folder user gets a random password
//
// A step is pending -> running -> done | failed | manual | replaced
// (replaced: the new owner set that credential first). A crash leaves the
// operation running; the worker continues it from the first unfinished step.
// A GET only reads. No password is stored: each run makes a new one, and a
// database user's is kept sealed in database_users once the engine has it.
//
// Devirden sonra parola ve anahtar yenileme. Önce sahiplik değişir; sonra
// kalıcı işlem sunucuyu değiştirir. Parola saklanmaz.

const (
	rotationEffectSiteAccount  = "site_account"
	rotationEffectVPN          = "vpn"
	rotationEffectMailbox      = "mailbox"
	rotationEffectDatabaseUser = "database_user"
	rotationEffectProtected    = "protected_dir"

	rotationStepPending  = "pending"
	rotationStepRunning  = "running"
	rotationStepDone     = "done"
	rotationStepFailed   = "failed"
	rotationStepManual   = "manual"
	rotationStepReplaced = "replaced"

	rotationAutoAttempts = 3

	errCodeTransferRotationPending = "TRANSFER_ROTATION_PENDING"
	errCodeTransferRotationRunning = "TRANSFER_ROTATION_RUNNING"
	errCodeTransferRotationNothing = "TRANSFER_ROTATION_NOTHING_TO_RETRY"
)

// rotationRef identifies one step's resource. It carries no secret.
type rotationRef struct {
	SubscriptionID int    `json:"subscription_id,omitempty"`
	DomainID       int    `json:"domain_id,omitempty"`
	SiteID         int    `json:"site_id,omitempty"`
	Domain         string `json:"domain,omitempty"`
	MailboxID      int    `json:"mailbox_id,omitempty"`
	DBUserID       int    `json:"db_user_id,omitempty"`
	Engine         string `json:"engine,omitempty"`
	Host           string `json:"host,omitempty"`
	Port           int    `json:"port,omitempty"`
	Username       string `json:"username,omitempty"`
	// A WordPress CelikPanel installed reads this database user's password.
	AppDomainID int    `json:"app_domain_id,omitempty"`
	AppDomain   string `json:"app_domain,omitempty"`
	AppDBName   string `json:"app_db_name,omitempty"`
}

type rotationPlanItem struct {
	Kind   string
	Target string
	Ref    rotationRef
	Before map[string]any
	// Manual: CelikPanel cannot change it here; Detail says where.
	Manual bool
	Detail string
}

// rotationDBUserLine is one database user in the preview.
type rotationDBUserLine struct {
	Username string `json:"username"`
	// AppDomain: a WordPress CelikPanel installed uses it; its
	// wp-config.php is updated too.
	AppDomain string `json:"app_domain,omitempty"`
	// Host: on another server; CelikPanel cannot change it there.
	Host   string `json:"host,omitempty"`
	Legacy bool   `json:"legacy,omitempty"`
}

type rotationProtectedLine struct {
	Domain string `json:"domain"`
	Users  int    `json:"users"`
}

// transferRotationPreview lists every credential the transfer changes, and
// what the new owner does after.
type transferRotationPreview struct {
	SiteAccounts   []string                `json:"site_accounts"`
	VPNDevices     []string                `json:"vpn_devices"`
	Mailboxes      []string                `json:"mailboxes"`
	DatabaseUsers  []rotationDBUserLine    `json:"database_users"`
	ProtectedUsers []rotationProtectedLine `json:"protected_users"`
	// LegacyFTPAccounts: rows an older CelikPanel recorded; not changed.
	LegacyFTPAccounts int `json:"legacy_ftp_accounts"`
	Total             int `json:"total"`
	// After: what the new owner does (codes; the screen says them).
	After []string `json:"after"`
	// NotCovered: credentials CelikPanel does not hold (codes).
	NotCovered []string `json:"not_covered"`
}

// planTransferRotation reads the subscription's credentials (read only).
func planTransferRotation(ctx context.Context, q rowQueryer, subID int) ([]rotationPlanItem, transferRotationPreview, error) {
	preview := transferRotationPreview{SiteAccounts: []string{}, VPNDevices: []string{}, Mailboxes: []string{},
		DatabaseUsers: []rotationDBUserLine{}, ProtectedUsers: []rotationProtectedLine{}, After: []string{},
		NotCovered: []string{"app_admin_accounts", "own_app_secrets"}}
	var accounts, vpn, mail, dbs, protected []rotationPlanItem

	// Site (file access) accounts: one per site with files.
	rows, err := q.QueryContext(ctx, `
		SELECT d.id, d.name, MIN(si.id)
		FROM domains d JOIN sites si ON si.domain_id = d.id
		WHERE d.subscription_id = ? AND COALESCE(si.project_type, 'php') <> 'dnsonly'
		GROUP BY d.id, d.name ORDER BY d.name`, subID)
	if err != nil {
		return nil, preview, err
	}
	for rows.Next() {
		var domainID, siteID int
		var name string
		if err := rows.Scan(&domainID, &name, &siteID); err != nil {
			rows.Close()
			return nil, preview, err
		}
		accounts = append(accounts, rotationPlanItem{Kind: rotationEffectSiteAccount, Target: name,
			Ref: rotationRef{SubscriptionID: subID, DomainID: domainID, SiteID: siteID, Domain: name, Username: services.SiteUsername(name)}})
		preview.SiteAccounts = append(preview.SiteAccounts, services.SiteUsername(name))
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, preview, err
	}

	// VPN peers that are live or being issued.
	rows, err = q.QueryContext(ctx, `
		SELECT name FROM vpn_peers WHERE subscription_id = ? AND desired_state = 'active' ORDER BY id`, subID)
	if err != nil {
		return nil, preview, err
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, preview, err
		}
		preview.VPNDevices = append(preview.VPNDevices, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, preview, err
	}
	if len(preview.VPNDevices) > 0 {
		vpn = append(vpn, rotationPlanItem{Kind: rotationEffectVPN, Target: "wireguard",
			Ref: rotationRef{SubscriptionID: subID}, Before: map[string]any{"peers": append([]string{}, preview.VPNDevices...)}})
	}

	rows, err = q.QueryContext(ctx, `
		SELECT ea.id, ea.address, d.id FROM email_accounts ea JOIN domains d ON d.id = ea.domain_id
		WHERE d.subscription_id = ? ORDER BY ea.address`, subID)
	if err != nil {
		return nil, preview, err
	}
	for rows.Next() {
		var id, domainID int
		var address string
		if err := rows.Scan(&id, &address, &domainID); err != nil {
			rows.Close()
			return nil, preview, err
		}
		address = strings.ToLower(strings.TrimSpace(address))
		mail = append(mail, rotationPlanItem{Kind: rotationEffectMailbox, Target: address,
			Ref: rotationRef{SubscriptionID: subID, DomainID: domainID, MailboxID: id}})
		preview.Mailboxes = append(preview.Mailboxes, address)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, preview, err
	}

	// Database users, with the WordPress CelikPanel installed for each.
	rows, err = q.QueryContext(ctx, `
		SELECT du.id, du.server_id, du.username, LOWER(t.name), COALESCE(ds.host, ''), COALESCE(ds.port, 0),
		       COALESCE(app.domain_id, 0), COALESCE(app.domain_name, ''), COALESCE(app.database_name, '')
		FROM database_users du
		JOIN database_servers ds ON ds.id = du.server_id
		JOIN database_server_types t ON t.id = ds.type_id
		LEFT JOIN (
			SELECT a.database_server_id, a.database_user, a.database_name, a.domain_id, d.name AS domain_name
			FROM application_install_operations a JOIN domains d ON d.id = a.domain_id
			WHERE a.status = 'applied' AND a.subscription_id = ?1
		) app ON app.database_server_id = du.server_id AND app.database_user = du.username
		WHERE du.subscription_id = ?1
		ORDER BY du.username, du.id`, subID)
	if err != nil {
		return nil, preview, err
	}
	for rows.Next() {
		var id, serverID, port, appDomainID int
		var username, engine, host, appDomain, appDB string
		if err := rows.Scan(&id, &serverID, &username, &engine, &host, &port, &appDomainID, &appDomain, &appDB); err != nil {
			rows.Close()
			return nil, preview, err
		}
		switch engine {
		case "mariadb", "mysql":
			engine, port = "mysql", 0
		case "postgresql", "postgres":
			engine = "postgresql"
		default:
			continue
		}
		ref := rotationRef{SubscriptionID: subID, DBUserID: id, Engine: engine, Host: host, Port: port, Username: username,
			AppDomainID: appDomainID, AppDomain: appDomain, AppDBName: appDB}
		item := rotationPlanItem{Kind: rotationEffectDatabaseUser, Target: strconv.Itoa(serverID) + ":" + username, Ref: ref}
		line := rotationDBUserLine{Username: username, AppDomain: appDomain}
		if !localDatabaseHost(host) {
			item.Manual = true
			item.Detail = "on another server (" + host + "); its administrator changes this user's password there"
			line.Host = host
		}
		dbs = append(dbs, item)
		preview.DatabaseUsers = append(preview.DatabaseUsers, line)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, preview, err
	}
	// Rows an older CelikPanel recorded in the legacy table.
	rows, err = q.QueryContext(ctx, `SELECT db_user, COALESCE(host, '') FROM databases WHERE subscription_id = ? ORDER BY db_user`, subID)
	if err != nil {
		return nil, preview, err
	}
	for rows.Next() {
		var username, host string
		if err := rows.Scan(&username, &host); err != nil {
			rows.Close()
			return nil, preview, err
		}
		dbs = append(dbs, rotationPlanItem{Kind: rotationEffectDatabaseUser, Target: "legacy:" + username, Manual: true,
			Ref:    rotationRef{SubscriptionID: subID, Username: username, Host: host},
			Detail: "recorded by an older CelikPanel version; the administrator changes this user's password on the database server"})
		preview.DatabaseUsers = append(preview.DatabaseUsers, rotationDBUserLine{Username: username, Legacy: true})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, preview, err
	}

	rows, err = q.QueryContext(ctx, `
		SELECT d.id, d.name, si.id, COUNT(u.id)
		FROM site_protected_users u
		JOIN site_protected_dirs pd ON pd.id = u.dir_id
		JOIN sites si ON si.id = pd.site_id
		JOIN domains d ON d.id = si.domain_id
		WHERE d.subscription_id = ?
		GROUP BY d.id, d.name, si.id ORDER BY d.name`, subID)
	if err != nil {
		return nil, preview, err
	}
	for rows.Next() {
		var domainID, siteID, users int
		var name string
		if err := rows.Scan(&domainID, &name, &siteID, &users); err != nil {
			rows.Close()
			return nil, preview, err
		}
		protected = append(protected, rotationPlanItem{Kind: rotationEffectProtected, Target: name,
			Ref: rotationRef{SubscriptionID: subID, DomainID: domainID, SiteID: siteID, Domain: name}, Before: map[string]any{"users": users}})
		preview.ProtectedUsers = append(preview.ProtectedUsers, rotationProtectedLine{Domain: name, Users: users})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, preview, err
	}
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM ftp_accounts WHERE subscription_id = ?`, subID).Scan(&preview.LegacyFTPAccounts); err != nil {
		return nil, preview, err
	}
	if preview.LegacyFTPAccounts > 0 {
		preview.NotCovered = append(preview.NotCovered, "legacy_ftp_accounts")
	}

	plan := append(append(append(append(accounts, vpn...), mail...), dbs...), protected...)
	preview.Total = len(preview.SiteAccounts) + len(preview.VPNDevices) + len(preview.Mailboxes) + len(preview.DatabaseUsers)
	for _, line := range preview.ProtectedUsers {
		preview.Total += line.Users
	}
	if len(preview.Mailboxes) > 0 {
		preview.After = append(preview.After, "mail_clients")
	}
	for _, line := range preview.DatabaseUsers {
		if line.AppDomain == "" && line.Host == "" && !line.Legacy {
			preview.After = append(preview.After, "database_apps")
			break
		}
	}
	if len(preview.SiteAccounts) > 0 {
		preview.After = append(preview.After, "file_access")
	}
	if len(preview.VPNDevices) > 0 {
		preview.After = append(preview.After, "vpn_devices")
	}
	if len(preview.ProtectedUsers) > 0 {
		preview.After = append(preview.After, "protected_folders")
	}
	return plan, preview, nil
}

// createTransferRotation writes the operation and its checkpoints inside the
// transfer's transaction and marks the VPN peers revoked there. It returns ""
// when the subscription holds no credential.
func createTransferRotation(ctx context.Context, tx *sql.Tx, transferID, subID, actorID int) (string, error) {
	plan, _, err := planTransferRotation(ctx, tx, subID)
	if err != nil {
		return "", err
	}
	if len(plan) == 0 {
		return "", nil
	}
	id, err := newServiceOperationID()
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO transfer_rotations (id, transfer_id, subscription_id, actor_id) VALUES (?, ?, ?, ?)`,
		id, transferID, subID, nullablePositiveInt(actorID)); err != nil {
		return "", err
	}
	for seq, item := range plan {
		ref, err := json.Marshal(item.Ref)
		if err != nil {
			return "", err
		}
		before := item.Before
		if before == nil {
			before = map[string]any{}
		}
		beforeJSON, err := json.Marshal(before)
		if err != nil {
			return "", err
		}
		state := rotationStepPending
		if item.Manual {
			state = rotationStepManual
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO transfer_rotation_effects (operation_id, seq, kind, target, ref, before, state, detail)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, seq, item.Kind, item.Target, string(ref), string(beforeJSON), state, nullIfEmpty(item.Detail)); err != nil {
			return "", err
		}
	}
	// The peers leave the desired state now; the vpn step proves the live
	// configuration followed.
	if _, err := tx.ExecContext(ctx, `
		UPDATE vpn_peers SET desired_state = 'revoked', sync_state = 'pending', sync_error = NULL, updated_at = datetime('now')
		WHERE subscription_id = ? AND desired_state = 'active'`, subID); err != nil {
		return "", err
	}
	return id, nil
}

// ---- running ----

// rotationNative is what a rotation does on the server; tests replace it.
type rotationNative interface {
	fileAccessState(ctx context.Context, site transport.FileAccessSite) (transport.FileAccessStateResponse, error)
	revokeFileAccess(ctx context.Context, req transport.ApplyFileAccessRequest) (transport.ApplyFileAccessResponse, error)
	setMailboxPassword(ctx context.Context, email, password string) (bool, error)
	endMailSessions(ctx context.Context, email string) (transport.EndMailSessionsResponse, error)
	rotateDatabaseUser(ctx context.Context, req transport.RotateDatabaseUserPasswordRequest) (transport.RotateDatabaseUserPasswordResponse, error)
	syncVPN(ctx context.Context) error
}

type panelRotationNative struct{ p *Panel }

func (n panelRotationNative) fileAccessState(ctx context.Context, site transport.FileAccessSite) (transport.FileAccessStateResponse, error) {
	if err := n.p.requireFileAccessAgent(ctx); err != nil {
		return transport.FileAccessStateResponse{}, errFileAccessAgentUpgrade
	}
	var state transport.FileAccessStateResponse
	err := n.p.callAgentContext(ctx, "Agent.FileAccessStateV1", &transport.FileAccessStateRequest{Site: site}, &state)
	return state, err
}

func (n panelRotationNative) revokeFileAccess(ctx context.Context, req transport.ApplyFileAccessRequest) (transport.ApplyFileAccessResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.ApplyFileAccessResponse
	err := n.p.callAgentContext(ctx, "Agent.ApplyFileAccessV1", &req, &resp)
	return resp, err
}

func (n panelRotationNative) setMailboxPassword(ctx context.Context, email, password string) (bool, error) {
	var resp transport.MailMutationResponse
	err := n.p.callAgentContext(ctx, "Agent.UpdateMailPassword", &transport.UpdateMailPasswordRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), Email: email, NewPassword: password,
	}, &resp)
	if err != nil {
		if agentAnsweredExactly(err, transport.MailConfigurationBusy) {
			return false, errors.New(transport.MailConfigurationBusy)
		}
		// An Agent's message is not passed on: it must not echo a password.
		return false, errors.New("the Agent did not confirm the mailbox password change")
	}
	return resp.Applied, nil
}

func (n panelRotationNative) endMailSessions(ctx context.Context, email string) (transport.EndMailSessionsResponse, error) {
	var resp transport.EndMailSessionsResponse
	err := n.p.callAgentContext(ctx, "Agent.EndMailSessionsV1", &transport.EndMailSessionsRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), Email: email,
	}, &resp)
	return resp, err
}

func (n panelRotationNative) rotateDatabaseUser(ctx context.Context, req transport.RotateDatabaseUserPasswordRequest) (transport.RotateDatabaseUserPasswordResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.RotateDatabaseUserPasswordResponse
	err := n.p.callAgentContext(ctx, "Agent.RotateDatabaseUserPasswordV1", &req, &resp)
	return resp, err
}

func (n panelRotationNative) syncVPN(ctx context.Context) error {
	return panelSuspensionNative(n).syncVPN(ctx)
}

func (p *Panel) rotationNativeOps() rotationNative {
	if p.rotationNative != nil {
		return p.rotationNative
	}
	return panelRotationNative{p: p}
}

type rotationOperationRow struct {
	ID             string
	TransferID     int
	SubscriptionID int
	ActorID        int
	State          string
	Attempts       int
	ErrorKind      string
	LastError      string
	CreatedAt      string
	FinishedAt     string
}

type rotationEffectRow struct {
	Seq    int
	Kind   string
	Target string
	Ref    rotationRef
	Before string
	State  string
	Detail string
}

func loadRotationOperation(ctx context.Context, q rowQueryer, where string, args ...any) (rotationOperationRow, error) {
	var op rotationOperationRow
	var actor sql.NullInt64
	var errKind, lastErr, finished sql.NullString
	err := q.QueryRowContext(ctx, `
		SELECT id, transfer_id, subscription_id, actor_id, state, attempts, error_kind, last_error, created_at, finished_at
		FROM transfer_rotations WHERE `+where, args...).
		Scan(&op.ID, &op.TransferID, &op.SubscriptionID, &actor, &op.State, &op.Attempts, &errKind, &lastErr, &op.CreatedAt, &finished)
	op.ActorID = int(actor.Int64)
	op.ErrorKind, op.LastError, op.FinishedAt = errKind.String, lastErr.String, finished.String
	return op, err
}

func loadRotationEffects(ctx context.Context, q rowQueryer, opID string) ([]rotationEffectRow, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT seq, kind, target, ref, before, state, COALESCE(detail, '')
		FROM transfer_rotation_effects WHERE operation_id = ? ORDER BY seq`, opID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var effects []rotationEffectRow
	for rows.Next() {
		var e rotationEffectRow
		var ref string
		if err := rows.Scan(&e.Seq, &e.Kind, &e.Target, &ref, &e.Before, &e.State, &e.Detail); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(ref), &e.Ref); err != nil {
			return nil, fmt.Errorf("rotation step %d reference: %w", e.Seq, err)
		}
		effects = append(effects, e)
	}
	return effects, rows.Err()
}

func rotationStepFinished(state string) bool {
	return state == rotationStepDone || state == rotationStepManual || state == rotationStepReplaced
}

// claimRotationStep marks a step running; false when the new owner replaced
// it meanwhile.
func (p *Panel) claimRotationStep(ctx context.Context, opID string, seq int) (bool, error) {
	res, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE transfer_rotation_effects SET state = 'running', updated_at = datetime('now')
		WHERE operation_id = ? AND seq = ? AND state IN ('pending', 'running', 'failed')`, opID, seq)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (p *Panel) finishRotationStep(ctx context.Context, opID string, seq int, state, detail, before string) error {
	query := `UPDATE transfer_rotation_effects SET state = ?, detail = ?, updated_at = datetime('now')`
	args := []any{state, nullIfEmpty(detail)}
	if before != "" {
		query += `, before = ?`
		args = append(args, before)
	}
	query += ` WHERE operation_id = ? AND seq = ? AND state = 'running'`
	args = append(args, opID, seq)
	_, err := p.db.GetDB().ExecContext(ctx, query, args...)
	return err
}

// runTransferRotation continues one running operation. A failed first step
// (file access) stops the run: the later steps write new passwords that an
// open shell could read. Other failures do not stop the steps after them.
func (p *Panel) runTransferRotation(ctx context.Context, opID string) error {
	db := p.db.GetDB()
	op, err := loadRotationOperation(ctx, db, `id = ?`, opID)
	if err != nil {
		return err
	}
	if op.State != "running" {
		return nil
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE transfer_rotations SET attempts = attempts + 1, updated_at = datetime('now') WHERE id = ? AND state = 'running'`, opID); err != nil {
		return err
	}
	op.Attempts++
	effects, err := loadRotationEffects(ctx, db, opID)
	if err != nil {
		return err
	}
	native := p.rotationNativeOps()
	var firstErr error
	var firstStep rotationEffectRow
	barrier := false
	for _, effect := range effects {
		if rotationStepFinished(effect.State) {
			continue
		}
		if barrier {
			continue
		}
		claimed, err := p.claimRotationStep(ctx, opID, effect.Seq)
		if err != nil {
			return err
		}
		if !claimed {
			continue
		}
		state, detail, before, stepErr := p.runRotationStep(ctx, native, op, effect)
		if stepErr != nil {
			if err := p.finishRotationStep(ctx, opID, effect.Seq, rotationStepFailed, stepErr.Error(), before); err != nil {
				return err
			}
			if firstErr == nil {
				firstErr, firstStep = stepErr, effect
			}
			if effect.Kind == rotationEffectSiteAccount {
				barrier = true
			}
			continue
		}
		if err := p.finishRotationStep(ctx, opID, effect.Seq, state, detail, before); err != nil {
			return err
		}
	}
	if firstErr != nil {
		kind := stepErrorKind(firstErr)
		message := fmt.Sprintf("%s %s: %v", firstStep.Kind, firstStep.Target, firstErr)
		// An unknown result is tried again by itself a bounded number of
		// times; a verified failure or an unmet prerequisite waits for a person.
		if kind == "unknown" && op.Attempts < rotationAutoAttempts {
			_, err := db.ExecContext(ctx, `
				UPDATE transfer_rotations SET error_kind = ?, last_error = ?, updated_at = datetime('now') WHERE id = ?`, kind, message, opID)
			return err
		}
		if _, err := db.ExecContext(ctx, `
			UPDATE transfer_rotations SET state = 'failed', error_kind = ?, last_error = ?, updated_at = datetime('now'), finished_at = datetime('now')
			WHERE id = ? AND state = 'running'`, kind, message, opID); err != nil {
			return err
		}
		p.auditRotationOutcome(ctx, op, "failed")
		return nil
	}
	// Terminal proof: every step reads done, manual or replaced.
	var open int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM transfer_rotation_effects WHERE operation_id = ? AND state NOT IN ('done', 'manual', 'replaced')`, opID).Scan(&open); err != nil {
		return err
	}
	if open > 0 {
		return nil
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE transfer_rotations SET state = 'completed', error_kind = NULL, last_error = NULL,
		       updated_at = datetime('now'), finished_at = datetime('now')
		WHERE id = ? AND state = 'running'`, opID); err != nil {
		return err
	}
	p.auditRotationOutcome(ctx, op, "completed")
	return nil
}

// newRotationSecret: 24 random letters and digits (crypto/rand) that nobody
// is shown.
func newRotationSecret() (string, error) { return randomDBPassword() }

// runRotationStep changes one resource. before is the native state found
// (no secret), stored with the step.
func (p *Panel) runRotationStep(ctx context.Context, native rotationNative, op rotationOperationRow, e rotationEffectRow) (string, string, string, error) {
	switch e.Kind {
	case rotationEffectSiteAccount:
		site := transport.FileAccessSite{SubscriptionID: e.Ref.SubscriptionID, DomainID: e.Ref.DomainID, Username: e.Ref.Username}
		state, err := native.fileAccessState(ctx, site)
		if errors.Is(err, errFileAccessAgentUpgrade) {
			return "", "", "", &suspensionStepError{kind: "prerequisite", err: errors.New("the server's Agent does not offer file access yet; finish the panel update, then retry")}
		}
		if err != nil {
			return "", "", "", unknownResult(err)
		}
		if !state.Account.Exists {
			return rotationStepDone, "the site has no system account; nothing to change", "", nil
		}
		before, _ := json.Marshal(map[string]any{"enabled": state.Account.Enabled, "ssh": state.Account.SSH,
			"password": state.Account.Password, "keys": len(state.Account.Keys)})
		reconcile, err := p.fileAccessReconcileSites(ctx, fileAccessSite{DomainID: e.Ref.DomainID, SubscriptionID: e.Ref.SubscriptionID})
		if err != nil {
			return "", "", string(before), err
		}
		resp, err := native.revokeFileAccess(ctx, transport.ApplyFileAccessRequest{Site: site, Enabled: false, RevokeCredentials: true, Reconcile: reconcile})
		if err != nil {
			return "", "", string(before), unknownResult(err)
		}
		if resp.Refusal != "" {
			if resp.Refusal == transport.FileAccessRefusedNoAccount {
				return rotationStepDone, "the site has no system account; nothing to change", string(before), nil
			}
			return "", "", string(before), &suspensionStepError{kind: "prerequisite", err: fmt.Errorf("%s (%s)", resp.Detail, resp.Refusal)}
		}
		after := resp.State.Account
		if !resp.Applied || after.Enabled || len(after.Keys) > 0 || after.Password == "set" || after.Password == "locked" || after.Password == "empty" {
			return "", "", string(before), verifiedFailure("the account still reads enabled=%t, keys=%d, password=%s after the change", after.Enabled, len(after.Keys), after.Password)
		}
		return rotationStepDone, fmt.Sprintf("file access closed, password removed, %d SSH key(s) removed", len(state.Account.Keys)), string(before), nil

	case rotationEffectVPN:
		if err := native.syncVPN(ctx); err != nil {
			if errors.Is(err, errVPNNotSetUp) {
				return rotationStepDone, "WireGuard is not set up on this server; the devices are revoked in CelikPanel", "", nil
			}
			return "", "", "", unknownResult(err)
		}
		return rotationStepDone, "the devices were removed from the live WireGuard configuration", "", nil

	case rotationEffectMailbox:
		// The address exactly as the mail files have it (the Agent refuses
		// another spelling), from the row, not from the step's target.
		var stored, domain string
		err := p.db.GetDB().QueryRowContext(ctx, `
			SELECT a.address, d.name FROM email_accounts a JOIN domains d ON d.id = a.domain_id WHERE a.id = ?`,
			e.Ref.MailboxID).Scan(&stored, &domain)
		if errors.Is(err, sql.ErrNoRows) {
			return rotationStepDone, "the mailbox no longer exists", "", nil
		}
		if err != nil {
			return "", "", "", err
		}
		address, err := transport.CanonicalMailboxForDomain(stored, domain)
		if err != nil {
			return "", "", "", verifiedFailure("the stored mailbox address is not valid; it was not changed")
		}
		secret, err := newRotationSecret()
		if err != nil {
			return "", "", "", err
		}
		// One mail change at a time, as the Mail screen does.
		p.mailMutationMu.Lock()
		applied, err := native.setMailboxPassword(ctx, address, secret)
		p.mailMutationMu.Unlock()
		if err != nil {
			return "", "", "", unknownResult(err)
		}
		if !applied {
			return "", "", "", unknownResult(errors.New("the Agent did not confirm the password change"))
		}
		ended, err := native.endMailSessions(ctx, address)
		if err != nil {
			return "", "", "", unknownResult(fmt.Errorf("password changed; ending open sessions: %w", err))
		}
		if !ended.Applied {
			return "", "", "", verifiedFailure("password changed, but open mail sessions were not ended: %s", ended.Error)
		}
		return rotationStepDone, "new password set (not shown to anyone); open mail sessions ended", "", nil

	case rotationEffectDatabaseUser:
		return p.runDatabaseUserRotation(ctx, native, e)

	case rotationEffectProtected:
		site, err := p.siteFeatureSite(ctx, e.Ref.DomainID)
		if errors.Is(err, sql.ErrNoRows) {
			return rotationStepDone, "the site no longer exists", "", nil
		}
		if err != nil {
			return "", "", "", err
		}
		changed := 0
		_, err = p.changeSiteFeatures(ctx, e.Ref.DomainID, site, func(state *siteFeatureState) (string, error) {
			for d := range state.Dirs {
				for u := range state.Dirs[d].Users {
					secret, err := newRotationSecret()
					if err != nil {
						return "", err
					}
					hash, err := bcrypt.GenerateFromPassword([]byte(secret), siteProtectBcryptCost)
					if err != nil {
						return "", err
					}
					state.Dirs[d].Users[u].Hash = "$2y$" + strings.TrimPrefix(string(hash), "$2a$")
					changed++
				}
			}
			return "", nil
		})
		if err != nil {
			var held *siteFileHeldError
			if errors.As(err, &held) {
				return "", "", "", &suspensionStepError{kind: "prerequisite", err: errors.New("the site's nginx file was changed on the server, so CelikPanel kept it; review it on the domain's Site configuration screen, then retry")}
			}
			return "", "", "", unknownResult(err)
		}
		return rotationStepDone, fmt.Sprintf("%d folder user(s) got a new password (not shown to anyone)", changed), "", nil
	}
	return "", "", "", verifiedFailure("unknown step kind %q", e.Kind)
}

func (p *Panel) runDatabaseUserRotation(ctx context.Context, native rotationNative, e rotationEffectRow) (string, string, string, error) {
	var exists int
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM database_users WHERE id = ?`, e.Ref.DBUserID).Scan(&exists); err != nil {
		return "", "", "", err
	}
	if exists == 0 {
		return rotationStepDone, "the database user no longer exists", "", nil
	}
	if p.secrets == nil {
		return "", "", "", &suspensionStepError{kind: "prerequisite", err: errors.New("the panel's secret storage is unavailable")}
	}
	secret, err := newRotationSecret()
	if err != nil {
		return "", "", "", err
	}
	sealed, err := p.secrets.Encrypt(secret)
	if err != nil {
		return "", "", "", err
	}
	req := transport.RotateDatabaseUserPasswordRequest{
		User:        transport.SuspensionDatabaseUser{Engine: e.Ref.Engine, Port: e.Ref.Port, Username: e.Ref.Username},
		NewPassword: secret,
	}
	if e.Ref.AppDomainID > 0 {
		req.App = &transport.DatabaseUserApp{Kind: "wordpress", SubscriptionID: e.Ref.SubscriptionID, DomainID: e.Ref.AppDomainID,
			SiteUsername: services.SiteUsername(e.Ref.AppDomain), DBName: e.Ref.AppDBName, Required: true}
	}
	resp, err := native.rotateDatabaseUser(ctx, req)
	if err != nil {
		return "", "", "", unknownResult(err)
	}
	// The engine has the new password: keep it sealed, whatever came after.
	if resp.PasswordChanged {
		if _, err := p.db.GetDB().ExecContext(ctx, `UPDATE database_users SET password = ?, updated_at = datetime('now') WHERE id = ?`, sealed, e.Ref.DBUserID); err != nil {
			return "", "", "", err
		}
	}
	switch {
	case resp.Absent:
		return rotationStepDone, "the user does not exist on the database engine; nothing to change", "", nil
	case resp.AppConflict:
		return "", "", "", &suspensionStepError{kind: "prerequisite", err: fmt.Errorf(
			"WordPress on %s: %s. Its database password was not changed so the site keeps working; the new owner sets a new password for %s under Databases and puts it into the site's configuration",
			e.Ref.AppDomain, resp.AppDetail, e.Ref.Username)}
	case resp.Error != "":
		return "", "", "", verifiedFailure("%s", resp.Error)
	case !resp.Applied:
		return "", "", "", unknownResult(errors.New("the Agent confirmed nothing"))
	}
	if resp.AppUpdated {
		return rotationStepDone, "new password set on the engine (not shown to anyone), open sessions ended, WordPress on " + e.Ref.AppDomain + " updated", "", nil
	}
	return rotationStepDone, "new password set on the engine (not shown to anyone); open sessions ended", "", nil
}

// auditRotationOutcome writes subscription.transfer_rotate with counts only.
func (p *Panel) auditRotationOutcome(ctx context.Context, op rotationOperationRow, result string) {
	counts := map[string]any{}
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT kind, state, COUNT(*) FROM transfer_rotation_effects WHERE operation_id = ? GROUP BY kind, state`, op.ID)
	if err == nil {
		for rows.Next() {
			var kind, state string
			var n int
			if rows.Scan(&kind, &state, &n) == nil {
				counts[kind+"_"+state] = n
			}
		}
		rows.Close()
	}
	details, _ := json.Marshal(map[string]any{"transfer_id": op.TransferID, "operation": op.ID, "result": result, "counts": counts})
	if _, err := p.db.GetDB().ExecContext(ctx, `
		INSERT INTO audit_logs (user_id, action, resource_type, resource_id, details)
		VALUES (?, ?, 'subscription', ?, ?)`,
		nullablePositiveInt(op.ActorID), "subscription.transfer_rotate", op.SubscriptionID, string(details)); err != nil {
		log.Printf("audit write failed (subscription.transfer_rotate): %v", err)
	}
}

// runPendingTransferRotations runs every running rotation once, oldest
// first. It shares the suspension worker's lock: both change the same
// accounts, so they never run at the same time.
func (p *Panel) runPendingTransferRotations(ctx context.Context) error {
	p.suspensionWorker.mu.Lock()
	defer p.suspensionWorker.mu.Unlock()
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT id FROM transfer_rotations WHERE state = 'running' ORDER BY created_at, rowid`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	var errs []error
	for _, id := range ids {
		if err := p.runTransferRotation(ctx, id); err != nil {
			errs = append(errs, fmt.Errorf("transfer rotation %s: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

// ---- view, retry and the new owner's own changes ----

type rotationEffectView struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

type rotationCounts struct {
	Total    int `json:"total"`
	Done     int `json:"done"`
	Pending  int `json:"pending"`
	Failed   int `json:"failed"`
	Manual   int `json:"manual"`
	Replaced int `json:"replaced"`
}

type transferRotationView struct {
	ID         string               `json:"id"`
	State      string               `json:"state"`
	Attempts   int                  `json:"attempts"`
	CreatedAt  string               `json:"created_at"`
	FinishedAt string               `json:"finished_at,omitempty"`
	Guidance   suspensionGuidance   `json:"guidance"`
	Counts     rotationCounts       `json:"counts"`
	Effects    []rotationEffectView `json:"effects"`
}

func rotationGuidance(op rotationOperationRow, counts rotationCounts) suspensionGuidance {
	switch op.State {
	case "running":
		g := suspensionGuidance{Result: "in_progress", Reason: "Changing the subscription's passwords and keys on this server.",
			WhoActs: "none", NextAction: "Nothing to do; this page follows the operation.",
			Resumes: "If the panel restarts, the same operation continues where it stopped."}
		if op.LastError != "" {
			g.Reason = "Paused after a result that could not be confirmed: " + op.LastError
			g.WhoActs = "panel"
			g.NextAction = "CelikPanel tries again by itself within a minute (at most " + strconv.Itoa(rotationAutoAttempts) + " attempts)."
		}
		return g
	case "failed":
		g := suspensionGuidance{Reason: op.LastError, WhoActs: "approver_or_new_owner",
			NextAction: "Fix the cause named here, then press Retry. Until then the steps marked still to do keep the previous owner's credential.",
			Resumes:    "Retry runs the same operation from the steps that are not finished; finished steps are not repeated."}
		switch op.ErrorKind {
		case "prerequisite":
			g.Result = "prerequisite"
		case "verified_failure":
			g.Result = "verified_failure"
		default:
			g.Result = "unknown"
			g.NextAction = "The result of the last step is not known. Press Retry; each step reads the server before it changes anything."
		}
		return g
	}
	g := suspensionGuidance{Result: "done", Reason: "Every password and key the previous owner knew was changed or removed.",
		WhoActs: "new_owner", NextAction: "Set new passwords for the mailboxes, database users, file access and devices you use.", Resumes: ""}
	if counts.Manual > 0 {
		g.Reason = "Every credential on this server was changed; " + strconv.Itoa(counts.Manual) + " item(s) live elsewhere."
		g.WhoActs = "administrator"
		g.NextAction = "Change the items marked manual on their own server, then the new owner sets new passwords."
	}
	return g
}

func (p *Panel) transferRotationView(ctx context.Context, transferID int) (*transferRotationView, error) {
	db := p.db.GetDB()
	op, err := loadRotationOperation(ctx, db, `transfer_id = ?`, transferID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	effects, err := loadRotationEffects(ctx, db, op.ID)
	if err != nil {
		return nil, err
	}
	view := &transferRotationView{ID: op.ID, State: op.State, Attempts: op.Attempts, CreatedAt: op.CreatedAt,
		FinishedAt: op.FinishedAt, Effects: []rotationEffectView{}}
	for _, e := range effects {
		view.Effects = append(view.Effects, rotationEffectView{Kind: e.Kind, Target: rotationEffectLabel(e), State: e.State, Detail: e.Detail})
		view.Counts.Total++
		switch e.State {
		case rotationStepDone:
			view.Counts.Done++
		case rotationStepFailed:
			view.Counts.Failed++
		case rotationStepManual:
			view.Counts.Manual++
		case rotationStepReplaced:
			view.Counts.Replaced++
		default:
			view.Counts.Pending++
		}
	}
	view.Guidance = rotationGuidance(op, view.Counts)
	return view, nil
}

// rotationEffectLabel is what the screen names: an account, an address, a
// user (its server id stays out).
func rotationEffectLabel(e rotationEffectRow) string {
	switch e.Kind {
	case rotationEffectSiteAccount:
		return e.Ref.Username
	case rotationEffectDatabaseUser:
		return e.Ref.Username
	}
	return e.Target
}

var errRotationNothingToRetry = errors.New("nothing to retry")

// retryTransferRotation continues a failed rotation: the same operation from
// its unfinished steps. A running one is left alone (polling and repeated
// presses never start a second run).
func (p *Panel) retryTransferRotation(ctx context.Context, transferID int) (bool, error) {
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var id, state string
	err = tx.QueryRowContext(ctx, `SELECT id, state FROM transfer_rotations WHERE transfer_id = ?`, transferID).Scan(&id, &state)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && state == "completed") {
		return false, errRotationNothingToRetry
	}
	if err != nil {
		return false, err
	}
	if state == "running" {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE transfer_rotations SET state = 'running', attempts = 0, error_kind = NULL, last_error = NULL, finished_at = NULL,
		       updated_at = datetime('now') WHERE id = ? AND state = 'failed'`, id); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE transfer_rotation_effects SET state = 'pending', updated_at = datetime('now')
		WHERE operation_id = ? AND state IN ('failed', 'running')`, id); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	p.kickSuspensionWorker()
	return true, nil
}

var errRotationStepRunning = errors.New("the transfer's change of this credential is running now")

// claimReplacedCredential: the new owner is about to set this credential
// themselves; an unfinished rotation step for it is marked replaced first so
// the worker cannot overwrite the new password. found says a step was
// claimed (release it if the owner's change fails).
func (p *Panel) claimReplacedCredential(ctx context.Context, kind, target string) (bool, error) {
	db := p.db.GetDB()
	var running int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM transfer_rotation_effects e JOIN transfer_rotations o ON o.id = e.operation_id
		WHERE e.kind = ? AND e.target = ? AND e.state = 'running' AND o.state IN ('running', 'failed')`, kind, target).Scan(&running); err != nil {
		return false, err
	}
	if running > 0 {
		return false, errRotationStepRunning
	}
	res, err := db.ExecContext(ctx, `
		UPDATE transfer_rotation_effects SET state = 'replaced', detail = 'the new owner set a new one', updated_at = datetime('now')
		WHERE kind = ? AND target = ? AND state IN ('pending', 'failed')
		  AND operation_id IN (SELECT id FROM transfer_rotations WHERE state IN ('running', 'failed'))`, kind, target)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return false, err
	}
	// A failed operation whose last open step this was is finished now.
	p.completeSatisfiedRotations(ctx)
	return true, nil
}

// releaseReplacedCredential undoes a claim when the owner's change failed.
func (p *Panel) releaseReplacedCredential(ctx context.Context, kind, target string) {
	if _, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE transfer_rotation_effects SET state = 'pending', detail = NULL, updated_at = datetime('now')
		WHERE kind = ? AND target = ? AND state = 'replaced'
		  AND operation_id IN (SELECT id FROM transfer_rotations WHERE state IN ('running', 'failed'))`, kind, target); err != nil {
		log.Printf("[transfer rotation] release %s step: %v", kind, err)
	}
	p.kickSuspensionWorker()
}

// completeSatisfiedRotations ends failed operations with no open step.
func (p *Panel) completeSatisfiedRotations(ctx context.Context) {
	db := p.db.GetDB()
	rows, err := db.QueryContext(ctx, `
		SELECT o.id FROM transfer_rotations o WHERE o.state = 'failed' AND NOT EXISTS (
			SELECT 1 FROM transfer_rotation_effects e WHERE e.operation_id = o.id AND e.state NOT IN ('done', 'manual', 'replaced'))`)
	if err != nil {
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		res, err := db.ExecContext(ctx, `
			UPDATE transfer_rotations SET state = 'completed', error_kind = NULL, last_error = NULL, updated_at = datetime('now'), finished_at = datetime('now')
			WHERE id = ? AND state = 'failed'`, id)
		if err != nil {
			continue
		}
		if n, _ := res.RowsAffected(); n == 1 {
			if op, err := loadRotationOperation(ctx, db, `id = ?`, id); err == nil {
				p.auditRotationOutcome(ctx, op, "completed")
			}
		}
	}
}

// siteAccountRotationOpen: the site's file access step is not finished, so
// opening access must wait (the old owner's password or keys may still be
// on the account).
func (p *Panel) siteAccountRotationOpen(ctx context.Context, domainID int) (bool, error) {
	var n int
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM transfer_rotation_effects e JOIN transfer_rotations o ON o.id = e.operation_id
		WHERE e.kind = 'site_account' AND json_extract(e.ref, '$.domain_id') = ?
		  AND e.state IN ('pending', 'running', 'failed') AND o.state IN ('running', 'failed')`, domainID).Scan(&n)
	return n > 0, err
}
