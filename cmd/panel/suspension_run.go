package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The suspension operation (D-025). Affected resources are planned when the
// operation is created (the same plan the preview shows); each resource is a
// checkpoint row in suspension_effects:
//
//	pending -> captured (before-image stored, read-only inspection) -> done
//	                                  \-> kept (owner changed it; left, D-022)
//	                                  \-> failed (verified failure or unknown)
//
// A crash leaves the operation 'running'; the worker continues it at start
// from the first effect that is not finished, and every Agent call is
// compare-and-set, so a repeated step changes nothing. Automatic retries are
// bounded (suspensionAutoAttempts); after that an operator continues it.
//
// İşlem: etkilenen kaynaklar işlem oluşturulurken planlanır; her kaynak bir
// kontrol noktası satırıdır. Çökme sonrası aynı işlem kaldığı yerden sürer.

const (
	suspensionAutoAttempts = 3
	suspensionWorkerTick   = time.Minute
)

// Effect kinds in the order they run.
const (
	suspensionEffectWeb         = "web"
	suspensionEffectApp         = "app"
	suspensionEffectSiteAccount = "site_account"
	suspensionEffectMail        = "mail"
	suspensionEffectDatabase    = "database"
	suspensionEffectVPN         = "vpn"
)

const (
	effectPending  = "pending"
	effectCaptured = "captured"
	effectDone     = "done"
	effectKept     = "kept"
	effectFailed   = "failed"
	effectManual   = "manual"
)

// suspensionNative is what an operation does on the server. Production uses
// the Agent (panelSuspensionNative); tests replace it.
type suspensionNative interface {
	renderSite(ctx context.Context, domainID int) error
	inspect(ctx context.Context, req transport.SuspensionInspectRequest) (transport.SuspensionInspectResponse, error)
	setSiteAccount(ctx context.Context, req transport.SetSiteAccountSuspensionRequest) (transport.SetSiteAccountSuspensionResponse, error)
	setMailDomain(ctx context.Context, req transport.SetMailDomainSuspensionRequest) (transport.SetMailDomainSuspensionResponse, error)
	setDatabaseUser(ctx context.Context, req transport.SetDatabaseUserSuspensionRequest) (transport.SetDatabaseUserSuspensionResponse, error)
	syncVPN(ctx context.Context) error
	setAppUnit(ctx context.Context, req transport.SetAppUnitSuspensionRequest) (transport.SetAppUnitSuspensionResponse, error)
}

type panelSuspensionNative struct{ p *Panel }

func (n panelSuspensionNative) renderSite(ctx context.Context, domainID int) error {
	return n.p.applyVhostForDomain(ctx, domainID)
}

func (n panelSuspensionNative) inspect(ctx context.Context, req transport.SuspensionInspectRequest) (transport.SuspensionInspectResponse, error) {
	var resp transport.SuspensionInspectResponse
	err := n.p.callAgentContext(ctx, "Agent.SuspensionInspectV1", &req, &resp)
	return resp, err
}

func (n panelSuspensionNative) setSiteAccount(ctx context.Context, req transport.SetSiteAccountSuspensionRequest) (transport.SetSiteAccountSuspensionResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.SetSiteAccountSuspensionResponse
	err := n.p.callAgentContext(ctx, "Agent.SetSiteAccountSuspensionV1", &req, &resp)
	return resp, err
}

func (n panelSuspensionNative) setMailDomain(ctx context.Context, req transport.SetMailDomainSuspensionRequest) (transport.SetMailDomainSuspensionResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.SetMailDomainSuspensionResponse
	err := n.p.callAgentContext(ctx, "Agent.SetMailDomainSuspensionV1", &req, &resp)
	return resp, err
}

func (n panelSuspensionNative) setDatabaseUser(ctx context.Context, req transport.SetDatabaseUserSuspensionRequest) (transport.SetDatabaseUserSuspensionResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.SetDatabaseUserSuspensionResponse
	err := n.p.callAgentContext(ctx, "Agent.SetDatabaseUserSuspensionV1", &req, &resp)
	return resp, err
}

func (n panelSuspensionNative) setAppUnit(ctx context.Context, req transport.SetAppUnitSuspensionRequest) (transport.SetAppUnitSuspensionResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.SetAppUnitSuspensionResponse
	err := n.p.callAgentContext(ctx, "Agent.SetAppUnitSuspensionV1", &req, &resp)
	return resp, err
}

// syncVPN advances the WireGuard generation and synchronizes: the peer query
// leaves out peers of suspended subscriptions (vpn_handlers.go), so the live
// configuration drops them while their rows stay. Terminal proof: the applied
// generation reached the one this step asked for.
func (n panelSuspensionNative) syncVPN(ctx context.Context) error {
	db := n.p.db.GetDB()
	if _, err := db.ExecContext(ctx, `
		UPDATE vpn_sync_state SET desired_generation = desired_generation + 1,
		       status = 'pending', last_error = NULL, updated_at = datetime('now')
		WHERE id = 1`); err != nil {
		return err
	}
	var wanted int64
	if err := db.QueryRowContext(ctx, `SELECT desired_generation FROM vpn_sync_state WHERE id = 1`).Scan(&wanted); err != nil {
		return err
	}
	if err := n.p.syncVPNPeers(ctx); err != nil {
		return err
	}
	var applied int64
	if err := db.QueryRowContext(ctx, `SELECT applied_generation FROM vpn_sync_state WHERE id = 1`).Scan(&applied); err != nil {
		return err
	}
	if applied < wanted {
		return fmt.Errorf("WireGuard applied generation %d, wanted %d", applied, wanted)
	}
	return nil
}

func (p *Panel) suspensionNativeOps() suspensionNative {
	if p.suspensionNative != nil {
		return p.suspensionNative
	}
	return panelSuspensionNative{p: p}
}

// suspensionEffectRef is the identity stored with an effect.
type suspensionEffectRef struct {
	SubscriptionID int    `json:"subscription_id,omitempty"`
	DomainID       int    `json:"domain_id,omitempty"`
	SiteID         int    `json:"site_id,omitempty"`
	Domain         string `json:"domain,omitempty"`
	Engine         string `json:"engine,omitempty"`
	Port           int    `json:"port,omitempty"`
	Username       string `json:"username,omitempty"`
	Host           string `json:"host,omitempty"`
	Local          bool   `json:"local,omitempty"`
	// InstallID (alpha.118): the effect is a service app's unit
	// (celiksvc-<id>) rather than the site's Node.js app unit.
	InstallID int `json:"install_id,omitempty"`
}

type suspensionPlanItem struct {
	Kind   string
	Target string
	Ref    suspensionEffectRef
}

// suspensionPreview is what an operation will change (read-only).
type suspensionPreview struct {
	Action        string   `json:"action"`
	Sites         []string `json:"sites"`
	Apps          []string `json:"apps"`
	MailDomains   []string `json:"mail_domains"`
	Mailboxes     int      `json:"mailboxes"`
	DatabaseUsers []string `json:"database_users"`
	VPNPeers      int      `json:"vpn_peers"`
	SiteAccounts  []string `json:"site_accounts"`
	Notes         []string `json:"notes"`
}

func localDatabaseHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "", "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// planSuspension reads the subscription's resources: the affected-resource
// list of the operation and the preview. Read-only.
func planSuspension(ctx context.Context, q sqlQueryer, subscriptionID int, action string) ([]suspensionPlanItem, suspensionPreview, error) {
	preview := suspensionPreview{Action: action, Sites: []string{}, Apps: []string{}, MailDomains: []string{}, DatabaseUsers: []string{}, SiteAccounts: []string{}, Notes: []string{}}
	var web, apps, accounts, mail, dbs, vpn []suspensionPlanItem

	rows, err := q.QueryContext(ctx, `
		SELECT d.id, d.name,
		       EXISTS (SELECT 1 FROM sites s WHERE s.domain_id = d.id),
		       COALESCE((SELECT s.id FROM sites s WHERE s.domain_id = d.id AND s.project_type = 'node'), 0),
		       (SELECT COUNT(*) FROM email_accounts e WHERE e.domain_id = d.id),
		       EXISTS (SELECT 1 FROM email_forwardings f WHERE f.domain_id = d.id)
		FROM domains d WHERE d.subscription_id = ? ORDER BY d.name`, subscriptionID)
	if err != nil {
		return nil, preview, err
	}
	for rows.Next() {
		var id, nodeSite, mailboxes int
		var name string
		var hasSite, hasForward bool
		if err := rows.Scan(&id, &name, &hasSite, &nodeSite, &mailboxes, &hasForward); err != nil {
			rows.Close()
			return nil, preview, err
		}
		ref := suspensionEffectRef{SubscriptionID: subscriptionID, DomainID: id, Domain: name}
		if hasSite {
			web = append(web, suspensionPlanItem{suspensionEffectWeb, name, ref})
			accounts = append(accounts, suspensionPlanItem{suspensionEffectSiteAccount, name, ref})
			preview.Sites = append(preview.Sites, name)
			preview.SiteAccounts = append(preview.SiteAccounts, name)
		}
		if nodeSite > 0 {
			appRef := ref
			appRef.SiteID = nodeSite
			apps = append(apps, suspensionPlanItem{suspensionEffectApp, name, appRef})
			preview.Apps = append(preview.Apps, name)
		}
		if mailboxes > 0 || hasForward {
			mail = append(mail, suspensionPlanItem{suspensionEffectMail, name, ref})
			preview.MailDomains = append(preview.MailDomains, name)
			preview.Mailboxes += mailboxes
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, preview, err
	}

	rows, err = q.QueryContext(ctx, `
		SELECT du.username, LOWER(t.name), COALESCE(ds.host, ''), ds.port
		FROM database_users du
		JOIN database_servers ds ON ds.id = du.server_id
		JOIN database_server_types t ON t.id = ds.type_id
		WHERE du.subscription_id = ?
		UNION
		SELECT d.db_user, LOWER(d.db_type), COALESCE(d.host, ''), COALESCE(d.port, 0)
		FROM databases d WHERE d.subscription_id = ?
		ORDER BY 1`, subscriptionID, subscriptionID)
	if err != nil {
		return nil, preview, err
	}
	seen := map[string]bool{}
	for rows.Next() {
		var username, engine, host string
		var port int
		if err := rows.Scan(&username, &engine, &host, &port); err != nil {
			rows.Close()
			return nil, preview, err
		}
		switch engine {
		case "mariadb", "mysql":
			engine = "mysql"
			port = 0
		case "postgresql", "postgres":
			engine = "postgresql"
		default:
			continue
		}
		key := fmt.Sprintf("%s:%s@%s:%d", engine, username, host, port)
		if seen[key] {
			continue
		}
		seen[key] = true
		ref := suspensionEffectRef{SubscriptionID: subscriptionID, Engine: engine, Port: port, Username: username, Host: host, Local: localDatabaseHost(host)}
		dbs = append(dbs, suspensionPlanItem{suspensionEffectDatabase, key, ref})
		preview.DatabaseUsers = append(preview.DatabaseUsers, username)
		if !ref.Local {
			preview.Notes = append(preview.Notes, fmt.Sprintf("Database user %s is on another server (%s); CelikPanel cannot lock it there. The administrator locks it on that server.", username, host))
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, preview, err
	}

	if err := q.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM vpn_peers WHERE subscription_id = ? AND desired_state = 'active'`,
		subscriptionID).Scan(&preview.VPNPeers); err != nil {
		return nil, preview, err
	}
	if preview.VPNPeers > 0 {
		vpn = append(vpn, suspensionPlanItem{suspensionEffectVPN, "wireguard", suspensionEffectRef{SubscriptionID: subscriptionID}})
	}

	// alpha.118: the subscription's service apps stop like its Node.js apps.
	serviceApps, err := planServiceAppSuspension(ctx, q, subscriptionID, &preview)
	if err != nil {
		return nil, preview, err
	}
	apps = append(apps, serviceApps...)

	if action == suspensionKindSuspend {
		preview.Notes = append(preview.Notes,
			"Nothing is deleted: files, mailboxes, databases and settings stay and come back exactly when the subscription is resumed.",
			"Mail sent to these domains is refused temporarily while suspended (senders keep it and retry for several days); mail already stored is kept.",
			"Node.js apps are stopped and do not start at boot; on resume only the apps that were running are started again.")
	} else {
		preview.Notes = append(preview.Notes,
			"Everything CelikPanel changed when suspending is put back as it was; anything changed on the server since then by its owner is kept.")
	}
	plan := append(append(append(append(append(web, apps...), accounts...), mail...), dbs...), vpn...)
	return plan, preview, nil
}

// createSuspensionOperation inserts an operation and its effect checkpoints.
func (p *Panel) createSuspensionOperation(ctx context.Context, tx sqlQueryer, subscriptionID int, kind, trigger string, actorID int) (string, error) {
	id, err := newServiceOperationID()
	if err != nil {
		return "", err
	}
	plan, _, err := planSuspension(ctx, tx, subscriptionID, kind)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO suspension_operations (id, subscription_id, kind, trigger, actor_id)
		VALUES (?, ?, ?, ?, ?)`, id, subscriptionID, kind, trigger, nullablePositiveInt(actorID)); err != nil {
		return "", err
	}
	for seq, item := range plan {
		ref, err := json.Marshal(item.Ref)
		if err != nil {
			return "", err
		}
		state := effectPending
		detail := ""
		if item.Kind == suspensionEffectDatabase && !item.Ref.Local {
			state = effectManual
			detail = "on another server (" + item.Ref.Host + "); the administrator locks or unlocks this user there"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO suspension_effects (operation_id, seq, kind, target, ref, state, detail)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, id, seq, item.Kind, item.Target, string(ref), state, nullIfEmpty(detail)); err != nil {
			return "", err
		}
	}
	return id, nil
}

type suspensionOperationRow struct {
	ID             string
	SubscriptionID int
	Kind           string
	Trigger        string
	State          string
	Attempts       int
	ErrorKind      string
	LastError      string
	CreatedAt      string
	FinishedAt     string
}

type suspensionEffectRow struct {
	Seq    int
	Kind   string
	Target string
	Ref    suspensionEffectRef
	State  string
	Detail string
}

func loadSuspensionOperation(ctx context.Context, q sqlQueryer, id string) (suspensionOperationRow, error) {
	var op suspensionOperationRow
	var errKind, lastErr, finished sql.NullString
	err := q.QueryRowContext(ctx, `
		SELECT id, subscription_id, kind, trigger, state, attempts, error_kind, last_error, created_at, finished_at
		FROM suspension_operations WHERE id = ?`, id).
		Scan(&op.ID, &op.SubscriptionID, &op.Kind, &op.Trigger, &op.State, &op.Attempts, &errKind, &lastErr, &op.CreatedAt, &finished)
	op.ErrorKind, op.LastError, op.FinishedAt = errKind.String, lastErr.String, finished.String
	return op, err
}

func loadSuspensionEffects(ctx context.Context, q sqlQueryer, id string) ([]suspensionEffectRow, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT seq, kind, target, ref, state, COALESCE(detail, '')
		FROM suspension_effects WHERE operation_id = ? ORDER BY seq`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var effects []suspensionEffectRow
	for rows.Next() {
		var e suspensionEffectRow
		var ref string
		if err := rows.Scan(&e.Seq, &e.Kind, &e.Target, &ref, &e.State, &e.Detail); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(ref), &e.Ref); err != nil {
			return nil, fmt.Errorf("effect %d reference: %w", e.Seq, err)
		}
		effects = append(effects, e)
	}
	return effects, rows.Err()
}

func effectFinished(state string) bool {
	return state == effectDone || state == effectKept || state == effectManual
}

// suspensionStepError classifies a failed step (D-024).
type suspensionStepError struct {
	kind string // verified_failure | prerequisite | unknown
	err  error
}

func (e *suspensionStepError) Error() string { return e.err.Error() }

func verifiedFailure(format string, args ...any) error {
	return &suspensionStepError{kind: "verified_failure", err: fmt.Errorf(format, args...)}
}

func unknownResult(err error) error {
	return &suspensionStepError{kind: "unknown", err: err}
}

func stepErrorKind(err error) string {
	var step *suspensionStepError
	if errors.As(err, &step) {
		return step.kind
	}
	var held *siteFileHeldError
	if errors.As(err, &held) {
		return "prerequisite"
	}
	return "unknown"
}

func (p *Panel) setEffectState(ctx context.Context, opID string, seq int, state, detail string) error {
	_, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE suspension_effects SET state = ?, detail = ?, updated_at = datetime('now')
		WHERE operation_id = ? AND seq = ?`, state, nullIfEmpty(detail), opID, seq)
	return err
}

// runSuspensionOperation continues one running operation from its first
// unfinished checkpoint. It returns when the operation finished, failed, or
// must wait for a later automatic attempt.
func (p *Panel) runSuspensionOperation(ctx context.Context, opID string) error {
	db := p.db.GetDB()
	op, err := loadSuspensionOperation(ctx, db, opID)
	if err != nil {
		return err
	}
	if op.State != "running" {
		return nil
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE suspension_operations SET attempts = attempts + 1, updated_at = datetime('now')
		WHERE id = ? AND state = 'running'`, opID); err != nil {
		return err
	}
	op.Attempts++
	effects, err := loadSuspensionEffects(ctx, db, opID)
	if err != nil {
		return err
	}
	native := p.suspensionNativeOps()
	for _, effect := range effects {
		if effectFinished(effect.State) {
			continue
		}
		state, detail, stepErr := p.runSuspensionEffect(ctx, native, op, effect)
		if stepErr != nil {
			kind := stepErrorKind(stepErr)
			message := fmt.Sprintf("%s %s: %v", effect.Kind, effect.Target, stepErr)
			if err := p.setEffectState(ctx, opID, effect.Seq, effectFailed, stepErr.Error()); err != nil {
				return err
			}
			// An unknown result is tried again automatically, a bounded number
			// of times; a verified failure or unmet prerequisite waits for a
			// person, because repeating it changes nothing.
			if kind == "unknown" && op.Attempts < suspensionAutoAttempts {
				_, err := db.ExecContext(ctx, `
					UPDATE suspension_operations SET error_kind = ?, last_error = ?, updated_at = datetime('now')
					WHERE id = ?`, kind, message, opID)
				return err
			}
			_, err := db.ExecContext(ctx, `
				UPDATE suspension_operations SET state = 'failed', error_kind = ?, last_error = ?,
				       updated_at = datetime('now'), finished_at = datetime('now')
				WHERE id = ? AND state = 'running'`, kind, message, opID)
			return err
		}
		if err := p.setEffectState(ctx, opID, effect.Seq, state, detail); err != nil {
			return err
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		UPDATE suspension_operations SET state = 'completed', error_kind = NULL, last_error = NULL,
		       updated_at = datetime('now'), finished_at = datetime('now')
		WHERE id = ? AND state = 'running'`, opID); err != nil {
		return err
	}
	// A completed resume has put everything back: the before-images are spent.
	if op.Kind == suspensionKindResume {
		if _, err := tx.ExecContext(ctx, `DELETE FROM suspension_before_images WHERE subscription_id = ?`, op.SubscriptionID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// beforeImage loads or captures the before-image of one resource. When none
// is stored, capture runs (only for a suspension): the current state is read
// and stored before anything changes, so a crash after the change still knows
// what to restore. A stored image is never replaced: it is the state before
// the first suspension.
func (p *Panel) beforeImage(ctx context.Context, subscriptionID int, kind, target string, capture func() (any, error), into any) (bool, error) {
	db := p.db.GetDB()
	var raw string
	err := db.QueryRowContext(ctx, `
		SELECT before FROM suspension_before_images
		WHERE subscription_id = ? AND kind = ? AND target = ?`, subscriptionID, kind, target).Scan(&raw)
	if err == nil {
		return true, json.Unmarshal([]byte(raw), into)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if capture == nil {
		return false, nil
	}
	value, err := capture()
	if err != nil {
		return false, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	if _, err := db.ExecContext(ctx, `
		INSERT OR IGNORE INTO suspension_before_images (subscription_id, kind, target, before)
		VALUES (?, ?, ?, ?)`, subscriptionID, kind, target, string(encoded)); err != nil {
		return false, err
	}
	return p.beforeImage(ctx, subscriptionID, kind, target, nil, into)
}

func absentError(message string) bool {
	return strings.HasPrefix(message, "site account is absent") || strings.HasPrefix(message, "database user is absent") ||
		strings.HasPrefix(message, "app unit is absent")
}

// applyResult turns the Agent's compare-and-set answer into a checkpoint.
func applyResult(r transport.SuspensionApplyResult, changedText, sameText string) (string, string, error) {
	switch {
	case r.Conflict:
		// D-022: someone else changed it; it is left as they made it.
		return effectKept, "left unchanged: " + r.Error, nil
	case r.Applied && r.Changed:
		return effectDone, changedText, nil
	case r.Applied:
		return effectDone, sameText, nil
	case absentError(r.Error):
		return effectDone, "not on the server; nothing to change", nil
	case r.Error == transport.MailConfigurationBusy:
		return "", "", unknownResult(errors.New("the mail configuration is busy with another change; tried again shortly"))
	case r.Error != "":
		return "", "", verifiedFailure("%s", r.Error)
	}
	return "", "", unknownResult(errors.New("the Agent confirmed nothing"))
}

func (p *Panel) runSuspensionEffect(ctx context.Context, native suspensionNative, op suspensionOperationRow, e suspensionEffectRow) (string, string, error) {
	suspend := op.Kind == suspensionKindSuspend
	switch e.Kind {
	case suspensionEffectWeb:
		// The render reads subscriptions.status (buildVhostRequest), so it
		// writes the suspended page or the site, then nginx -t and reload.
		if err := native.renderSite(ctx, e.Ref.DomainID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return effectDone, "the site no longer exists", nil
			}
			var held *siteFileHeldError
			if errors.As(err, &held) {
				return "", "", &suspensionStepError{kind: "prerequisite", err: fmt.Errorf(
					"the site's nginx file was changed on the server, so CelikPanel kept it and did not write the %s; the administrator reviews the file on the domain's Site configuration screen, then continues this operation",
					map[bool]string{true: "suspended page", false: "site again"}[suspend])}
			}
			return "", "", unknownResult(err)
		}
		if suspend {
			return effectDone, "serves the suspended page (503); nginx validated and reloaded", nil
		}
		return effectDone, "serves the site again; nginx validated and reloaded", nil

	case suspensionEffectApp:
		if e.Ref.InstallID > 0 {
			return p.runServiceAppSuspensionEffect(ctx, op, e)
		}
		var before transport.AppUnitState
		var capture func() (any, error)
		if suspend {
			capture = func() (any, error) {
				resp, err := native.inspect(ctx, transport.SuspensionInspectRequest{AppSites: []int{e.Ref.SiteID}})
				if err != nil {
					return nil, unknownResult(err)
				}
				if resp.Error != "" || len(resp.AppUnits) != 1 {
					return nil, unknownResult(fmt.Errorf("inspection: %s", resp.Error))
				}
				if resp.AppUnits[0].Error != "" {
					return nil, unknownResult(errors.New(resp.AppUnits[0].Error))
				}
				return resp.AppUnits[0].State, nil
			}
		}
		found, err := p.beforeImage(ctx, op.SubscriptionID, e.Kind, e.Target, capture, &before)
		if err != nil {
			return "", "", err
		}
		if !found {
			return effectDone, "nothing was changed when suspending; nothing to restore", nil
		}
		if suspend {
			_ = p.setEffectState(ctx, op.ID, e.Seq, effectCaptured, "")
		}
		if !before.Present {
			return effectDone, "the site has no Node.js app unit; nothing to change", nil
		}
		// D-043: a security emergency still holds this app; it stays
		// stopped and the emergency restores the earliest state it knows.
		if !suspend {
			if handed, err := p.handSuspensionAppToEmergency(ctx, op.SubscriptionID, e.Ref.DomainID, e.Target); err != nil {
				return "", "", err
			} else if handed {
				return effectDone, "kept stopped: a security emergency holds this Node.js app; it starts again when the emergency is re-enabled", nil
			}
		}
		resp, err := native.setAppUnit(ctx, transport.SetAppUnitSuspensionRequest{SiteID: e.Ref.SiteID, Suspend: suspend, Before: before})
		if err != nil {
			return "", "", unknownResult(err)
		}
		if suspend {
			return applyResult(resp.SuspensionApplyResult, "Node.js app stopped and kept from starting at boot", "already stopped")
		}
		if !before.Active {
			return applyResult(resp.SuspensionApplyResult, "boot setting put back; the app was not running before, so it was not started", "already as it was")
		}
		return applyResult(resp.SuspensionApplyResult, "Node.js app started again", "already running")

	case suspensionEffectSiteAccount:
		account := transport.SuspensionSiteAccount{SubscriptionID: e.Ref.SubscriptionID, DomainID: e.Ref.DomainID, Domain: e.Ref.Domain}
		var before transport.SiteAccountState
		var capture func() (any, error)
		if suspend {
			capture = func() (any, error) {
				resp, err := native.inspect(ctx, transport.SuspensionInspectRequest{SiteAccounts: []transport.SuspensionSiteAccount{account}})
				if err != nil {
					return nil, unknownResult(err)
				}
				if resp.Error != "" || len(resp.SiteAccounts) != 1 {
					return nil, unknownResult(fmt.Errorf("inspection: %s", resp.Error))
				}
				if resp.SiteAccounts[0].Error != "" {
					return nil, unknownResult(errors.New(resp.SiteAccounts[0].Error))
				}
				return resp.SiteAccounts[0].State, nil
			}
		}
		found, err := p.beforeImage(ctx, op.SubscriptionID, e.Kind, e.Target, capture, &before)
		if err != nil {
			return "", "", err
		}
		if !found {
			return effectDone, "nothing was changed when suspending; nothing to restore", nil
		}
		if suspend {
			_ = p.setEffectState(ctx, op.ID, e.Seq, effectCaptured, "")
		}
		if !before.Present {
			return effectDone, "the site has no system account; nothing to change", nil
		}
		resp, err := native.setSiteAccount(ctx, transport.SetSiteAccountSuspensionRequest{Account: account, Suspend: suspend, Before: before})
		if err != nil {
			return "", "", unknownResult(err)
		}
		if suspend {
			return applyResult(resp.SuspensionApplyResult, "account expired: cron jobs stop and SSH/SFTP/FTP sign-in is refused", "already expired")
		}
		return applyResult(resp.SuspensionApplyResult, "account expiry put back as it was", "already as it was")

	case suspensionEffectMail:
		mailboxes, err := p.suspensionMailboxes(ctx, e.Ref.DomainID)
		if err != nil {
			return "", "", err
		}
		domain := transport.SuspensionMailDomain{Domain: e.Ref.Domain, Mailboxes: mailboxes}
		var before transport.MailDomainState
		var capture func() (any, error)
		if suspend {
			capture = func() (any, error) {
				resp, err := native.inspect(ctx, transport.SuspensionInspectRequest{MailDomains: []transport.SuspensionMailDomain{domain}})
				if err != nil {
					return nil, unknownResult(err)
				}
				if resp.Error != "" || len(resp.MailDomains) != 1 {
					return nil, unknownResult(fmt.Errorf("inspection: %s", resp.Error))
				}
				if resp.MailDomains[0].Error != "" {
					return nil, unknownResult(errors.New(resp.MailDomains[0].Error))
				}
				return resp.MailDomains[0].State, nil
			}
		}
		found, err := p.beforeImage(ctx, op.SubscriptionID, e.Kind, e.Target, capture, &before)
		if err != nil {
			return "", "", err
		}
		if !found {
			return effectDone, "nothing was changed when suspending; nothing to restore", nil
		}
		if suspend {
			_ = p.setEffectState(ctx, op.ID, e.Seq, effectCaptured, "")
		}
		if before.LoginRefused == nil {
			before.LoginRefused = map[string]bool{}
		}
		resp, err := native.setMailDomain(ctx, transport.SetMailDomainSuspensionRequest{Domain: domain, Suspend: suspend, Before: before})
		if err != nil {
			return "", "", unknownResult(err)
		}
		if suspend {
			return applyResult(resp.SuspensionApplyResult, "incoming mail refused temporarily (450, senders retry); mailbox sign-in refused", "already refused")
		}
		return applyResult(resp.SuspensionApplyResult, "mail accepted and sign-in allowed again", "already as it was")

	case suspensionEffectDatabase:
		user := transport.SuspensionDatabaseUser{Engine: e.Ref.Engine, Port: e.Ref.Port, Username: e.Ref.Username}
		var before transport.DatabaseUserState
		var capture func() (any, error)
		if suspend {
			capture = func() (any, error) {
				resp, err := native.inspect(ctx, transport.SuspensionInspectRequest{DatabaseUsers: []transport.SuspensionDatabaseUser{user}})
				if err != nil {
					return nil, unknownResult(err)
				}
				if resp.Error != "" || len(resp.DatabaseUsers) != 1 {
					return nil, unknownResult(fmt.Errorf("inspection: %s", resp.Error))
				}
				if resp.DatabaseUsers[0].Error != "" {
					return nil, unknownResult(errors.New(resp.DatabaseUsers[0].Error))
				}
				return resp.DatabaseUsers[0].State, nil
			}
		}
		found, err := p.beforeImage(ctx, op.SubscriptionID, e.Kind, e.Target, capture, &before)
		if err != nil {
			return "", "", err
		}
		if !found {
			return effectDone, "nothing was changed when suspending; nothing to restore", nil
		}
		if suspend {
			_ = p.setEffectState(ctx, op.ID, e.Seq, effectCaptured, "")
		}
		if !before.Present {
			return effectDone, "the database user does not exist on the engine; nothing to change", nil
		}
		resp, err := native.setDatabaseUser(ctx, transport.SetDatabaseUserSuspensionRequest{User: user, Suspend: suspend, Before: before})
		if err != nil {
			return "", "", unknownResult(err)
		}
		if suspend {
			return applyResult(resp.SuspensionApplyResult, "account locked and its connections ended", "already locked; its connections ended")
		}
		if before.Locked {
			return applyResult(resp.SuspensionApplyResult, "kept locked: it was locked before the suspension", "kept locked: it was locked before the suspension")
		}
		return applyResult(resp.SuspensionApplyResult, "account unlocked", "already unlocked")

	case suspensionEffectVPN:
		if err := native.syncVPN(ctx); err != nil {
			if errors.Is(err, errVPNNotSetUp) {
				return effectDone, "WireGuard is not set up on this server; nothing to change", nil
			}
			return "", "", unknownResult(err)
		}
		if suspend {
			return effectDone, "peers removed from the live WireGuard configuration (kept in CelikPanel)", nil
		}
		return effectDone, "peers back in the live WireGuard configuration", nil
	}
	return "", "", verifiedFailure("unknown effect kind %q", e.Kind)
}

func (p *Panel) suspensionMailboxes(ctx context.Context, domainID int) ([]string, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT address FROM email_accounts WHERE domain_id = ? ORDER BY address`, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var address string
		if err := rows.Scan(&address); err != nil {
			return nil, err
		}
		out = append(out, strings.ToLower(strings.TrimSpace(address)))
	}
	return out, rows.Err()
}

// runPendingSuspensionOperations runs every running operation once, oldest
// first, then starts any operation a later cause change needs.
func (p *Panel) runPendingSuspensionOperations(ctx context.Context) error {
	p.suspensionWorker.mu.Lock()
	defer p.suspensionWorker.mu.Unlock()
	var errs []error
	for round := 0; round < 3; round++ {
		rows, err := p.db.GetDB().QueryContext(ctx, `
			SELECT id FROM suspension_operations WHERE state = 'running' ORDER BY created_at, rowid`)
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
		for _, id := range ids {
			if err := p.runSuspensionOperation(ctx, id); err != nil {
				errs = append(errs, fmt.Errorf("suspension operation %s: %w", id, err))
			}
		}
		before, _ := p.countRunningSuspensionOperations(ctx)
		if err := p.reconcileSuspensionOperations(ctx); err != nil {
			errs = append(errs, err)
		}
		after, _ := p.countRunningSuspensionOperations(ctx)
		// Run the follow-up operations the reconcile just started; an
		// operation waiting for its next automatic attempt waits a tick.
		if after <= before {
			break
		}
	}
	return errors.Join(errs...)
}

func (p *Panel) countRunningSuspensionOperations(ctx context.Context) (int, error) {
	var n int
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM suspension_operations WHERE state = 'running'`).Scan(&n)
	return n, err
}

// kickSuspensionWorker asks the worker to run now (non-blocking).
func (p *Panel) kickSuspensionWorker() {
	if p.suspensionWorker.kick == nil {
		return
	}
	select {
	case p.suspensionWorker.kick <- struct{}{}:
	default:
	}
}

// startSuspensionWorker runs operations on every kick and every minute, and
// checks end dates. At start it continues what a crash left running.
func (p *Panel) startSuspensionWorker() {
	p.suspensionWorker.once.Do(func() {
		p.suspensionWorker.kick = make(chan struct{}, 1)
		go func() {
			ticker := time.NewTicker(suspensionWorkerTick)
			defer ticker.Stop()
			for {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
				if err := p.checkSubscriptionExpiry(ctx); err != nil {
					log.Printf("[suspension] end-date check: %v", err)
				}
				if err := p.runPendingSuspensionOperations(ctx); err != nil {
					log.Printf("[suspension] %v", err)
				}
				// Transfer credential rotation shares this worker and its lock.
				if err := p.runPendingTransferRotations(ctx); err != nil {
					log.Printf("[transfer rotation] %v", err)
				}
				cancel()
				select {
				case <-ticker.C:
				case <-p.suspensionWorker.kick:
				}
			}
		}()
	})
}
