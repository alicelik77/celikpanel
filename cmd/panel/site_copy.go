package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/mariadbgrant"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Website copying (alpha.128, PLESK-GAP G8).
//
//	GET  /api/v1/domains/{id}/copy   the sites this person may copy to, and the source's MariaDB databases
//	POST /api/v1/domains/{id}/copy   copy: {target_domain_id, database_id (0: files only), overwrite}
//
// Only into a site the same caller may manage (canAccessDomain on both), whose
// subscription is not suspended and whose rights hold the file manager (and
// databases, for a database copy); the target's disk and database limits
// apply. A target whose web folder has content is refused (an unmet
// prerequisite) unless the caller confirms overwrite: a full backup of the
// target is made first and named in the answer, so the previous site comes
// back from Backups. A database copy goes into a NEW database of the target
// (the source is never changed); its password is in this answer only (the
// route keeps no copy of it) and the application's own configuration still
// names the source's database until the owner changes it — the answer says so.
//
// Steps: check → backup (overwrite only) → database (when asked) → files.
//
// Site kopyalama: yalnız aynı kişinin yönettiği bir siteye; içerikli hedefin
// önce yedeği alınır; veritabanı kopyası yeni bir veritabanına gider.

const (
	errCodeSiteCopyTarget   = "SITE_COPY_TARGET"
	errCodeSiteCopyNotEmpty = "SITE_COPY_TARGET_NOT_EMPTY"
	errCodeSiteCopyFailed   = "SITE_COPY_FAILED"
	errCodeSiteCopyUnknown  = "SITE_COPY_UNKNOWN"
	errCodeSiteCopyDatabase = "SITE_COPY_DATABASE"
)

var requestIdentityRouteSiteCopy = requestIdentityRoute{
	pattern: "/api/v1/domains/{id}/copy", optional: true, secret: true,
	timeout: agentRPCSiteLongTimeout + agentRPCBulkImportTimeout + 5*time.Minute,
}

// Seams (tests replace them).
var (
	siteCopyAgent = func(p *Panel, ctx context.Context, method string, req, resp any) error {
		return p.callAgentContext(ctx, method, req, resp)
	}
)

type siteCopyTarget struct {
	DomainID    int    `json:"domain_id"`
	Domain      string `json:"domain"`
	ProjectType string `json:"project_type"`
}

type siteCopyDatabase struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func siteCopyable(projectType string) bool {
	switch projectType {
	case "php", "static", "node":
		return true
	}
	return false
}

func (p *Panel) siteCopyTargets(ctx context.Context, caller *Caller, sourceID int) ([]siteCopyTarget, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT d.id, d.name, COALESCE(s.project_type, 'php') FROM domains d JOIN sites s ON s.domain_id = d.id
		WHERE d.id <> ? GROUP BY d.id ORDER BY d.name`, sourceID)
	if err != nil {
		return nil, err
	}
	var all []siteCopyTarget
	for rows.Next() {
		var t siteCopyTarget
		if rows.Scan(&t.DomainID, &t.Domain, &t.ProjectType) == nil && siteCopyable(t.ProjectType) {
			all = append(all, t)
		}
	}
	rows.Close()
	out := []siteCopyTarget{}
	for _, t := range all {
		if p.canAccessDomain(ctx, caller, t.DomainID) == nil {
			out = append(out, t)
		}
	}
	return out, nil
}

func (p *Panel) siteCopyDatabases(ctx context.Context, domainID int) ([]siteCopyDatabase, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT dv.id, dv.name FROM databases_v2 dv
		JOIN database_servers ds ON ds.id = dv.server_id JOIN database_server_types dst ON dst.id = ds.type_id
		WHERE dv.domain_id = ? AND dst.name = 'mariadb' ORDER BY dv.name`, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []siteCopyDatabase{}
	for rows.Next() {
		var d siteCopyDatabase
		if rows.Scan(&d.ID, &d.Name) == nil {
			out = append(out, d)
		}
	}
	return out, rows.Err()
}

func (p *Panel) handleDomainSiteCopy(w http.ResponseWriter, r *http.Request, domainID int) {
	ctx := r.Context()
	caller := currentCaller(r)
	source, err := p.resolveWordPressSite(ctx, domainID)
	if err != nil {
		writeClientError(w, http.StatusNotFound, "site not found")
		return
	}
	if r.Method == http.MethodGet {
		targets, err := p.siteCopyTargets(ctx, caller, domainID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		databases, err := p.siteCopyDatabases(ctx, domainID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"copyable": siteCopyable(source.ProjectType), "targets": targets, "databases": databases})
		return
	}
	var body struct {
		TargetDomainID int  `json:"target_domain_id"`
		DatabaseID     int  `json:"database_id"`
		Overwrite      bool `json:"overwrite"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil || body.TargetDomainID <= 0 || body.TargetDomainID == domainID {
		writeClientError(w, http.StatusBadRequest, "target_domain_id is required")
		return
	}
	plan := []string{"check"}
	if body.Overwrite {
		plan = append(plan, "backup")
	}
	if body.DatabaseID > 0 {
		plan = append(plan, "database")
	}
	reportOperationPlan(ctx, append(plan, "files")...)
	reportOperationStep(ctx, "check")

	refuse := func(status int, code, message string) {
		writeCodedError(w, status, code, message, "")
	}
	if !siteCopyable(source.ProjectType) {
		refuse(http.StatusConflict, errCodeSiteCopyTarget, "This site serves no files of its own, so there is nothing to copy. Nothing was changed.")
		return
	}
	if p.canAccessDomain(ctx, caller, body.TargetDomainID) != nil {
		refuse(http.StatusNotFound, errCodeSiteCopyTarget, "The target site is not one you manage. Choose one from the list; nothing was changed.")
		return
	}
	target, err := p.resolveWordPressSite(ctx, body.TargetDomainID)
	if err != nil || !siteCopyable(target.ProjectType) {
		refuse(http.StatusConflict, errCodeSiteCopyTarget, "The target has no website folder (forwarding or DNS only). Choose another site; nothing was changed.")
		return
	}
	if caller != nil && !caller.hasAccountRole(roleAdmin) {
		if suspended, err := p.subscriptionSuspended(ctx, target.SubscriptionID); err != nil || suspended {
			refuse(http.StatusForbidden, errCodeSubscriptionSuspended, "The target's subscription is suspended; nothing was changed.")
			return
		}
	}
	if !p.requireDomainRight(w, r, target.DomainID, rightFileManager) {
		return
	}
	if err := p.checkSubscriptionQuota(ctx, target.SubscriptionID, quotaDisk); err != nil {
		writeQuotaRefusal(w, err)
		return
	}
	var sourceDB string
	if body.DatabaseID > 0 {
		if !p.requireDomainRight(w, r, target.DomainID, rightDatabases) {
			return
		}
		if err := p.db.GetDB().QueryRowContext(ctx, `
			SELECT dv.name FROM databases_v2 dv JOIN database_servers ds ON ds.id = dv.server_id
			JOIN database_server_types dst ON dst.id = ds.type_id
			WHERE dv.id = ? AND dv.domain_id = ? AND dst.name = 'mariadb'`, body.DatabaseID, domainID).Scan(&sourceDB); err != nil {
			refuse(http.StatusBadRequest, errCodeSiteCopyDatabase, "That database is not a MariaDB database of this site; nothing was changed.")
			return
		}
		if err := p.ensureInstalledDBServers(ctx, target.SubscriptionID); err != nil {
			writeServerError(w, err)
			return
		}
		if err := p.checkSubscriptionQuota(ctx, target.SubscriptionID, quotaDBs); err != nil {
			writeQuotaRefusal(w, err)
			return
		}
	}
	operationID, err := randomHexToken(16)
	if err != nil {
		writeServerError(w, err)
		return
	}
	opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), agentRPCSiteLongTimeout+agentRPCBulkImportTimeout)
	defer cancel()
	result := map[string]any{"copied": false, "target": target.Domain}

	if body.Overwrite {
		reportOperationStep(ctx, "backup")
		name, err := p.siteCopySafetyBackup(opCtx, target.DomainID, operationID)
		if err != nil {
			refuse(http.StatusBadGateway, errCodeSiteCopyFailed, "The safety backup of "+target.Domain+" could not be made, so nothing was copied: "+firstLineOf(err.Error()))
			return
		}
		result["backup"] = name
	}

	var created *siteCopyNewDatabase
	if sourceDB != "" {
		reportOperationStep(ctx, "database")
		created, err = p.siteCopyCreateDatabase(opCtx, target, operationID)
		if err != nil {
			refuse(http.StatusConflict, errCodeSiteCopyDatabase, "The copy's database could not be created, so nothing was copied: "+firstLineOf(err.Error()))
			return
		}
	}

	reportOperationStep(ctx, "files")
	request := transport.SiteCopyRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), OperationID: operationID,
		Source: source.site(), Target: target.site(), Overwrite: body.Overwrite, SourceDatabase: sourceDB,
	}
	if created != nil {
		request.TargetDatabase = created.Name
	}
	var answer transport.SiteCopyResponse
	if err := siteCopyAgent(p, opCtx, "Agent.SiteCopyV1", &request, &answer); err != nil {
		refuse(http.StatusBadGateway, errCodeSiteCopyUnknown,
			"The copy's answer was lost, so its result is not known. Open "+target.Domain+" to check; start the copy again only if it is missing.")
		return
	}
	if !answer.Copied {
		leftover := ""
		if created != nil && answer.CompensationSafe && !p.siteCopyRemoveDatabase(context.WithoutCancel(ctx), created) {
			leftover = siteCopyLeftoverNote(created.Name, target.Domain)
		}
		if answer.Code == transport.SiteCopyTargetNotEmpty {
			refuse(http.StatusConflict, errCodeSiteCopyNotEmpty,
				target.Domain+" already has content in its web folder, so nothing was copied. To replace it, confirm overwriting: a backup of it is made first."+leftover)
			return
		}
		message := "The copy failed and nothing was kept: " + boundedAgentDiagnostic(answer.Error)
		if leftover != "" {
			message = "The copy failed: " + boundedAgentDiagnostic(answer.Error) + "." + leftover
		}
		if !answer.CompensationSafe {
			message = "The copy stopped after it began publishing: " + boundedAgentDiagnostic(answer.Error) + ". Open " + target.Domain + " to check."
		}
		refuse(http.StatusBadGateway, errCodeSiteCopyFailed, message)
		return
	}
	details := map[string]any{"to": target.DomainID, "overwrite": body.Overwrite, "database": sourceDB != ""}
	p.auditDetailed(r, "domain.copy", "domain", domainID, details)
	result["copied"] = true
	if created != nil {
		doNotRetainRequestAnswer(ctx)
		result["database"] = map[string]string{"name": created.Name, "user": created.User, "password": created.Password}
		result["note"] = "The copied site's own configuration still names the source's database. Change it to this database, user and password; the password is shown only now."
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// siteCopySafetyBackup makes a full backup of the target (its files and
// linked databases) before it is overwritten; the answer names it.
func (p *Panel) siteCopySafetyBackup(ctx context.Context, domainID int, operationID string) (string, error) {
	d, err := p.loadBackupDomain(ctx, domainID)
	if err != nil {
		return "", err
	}
	databases, err := p.domainDatabaseIdentities(ctx, d)
	if err != nil {
		return "", err
	}
	req := backupspec.CreateRequest{
		ProtocolVersion: backupspec.ProtocolVersion, SubscriptionID: d.SubscriptionID, DomainID: d.ID,
		DomainName: d.Name, Type: backupspec.TypeFiles, Origin: backupspec.OriginPreUpdate,
		JobKey: "sitecopy:" + operationID, SourceDir: d.DocumentRoot,
	}
	if len(databases) > 0 {
		req.Type, req.Databases = backupspec.TypeFull, databases
	}
	var created backupspec.CreateResponse
	if err := siteCopyAgent(p, ctx, "Agent.CreateBackup", &req, &created); err != nil {
		return "", err
	}
	if !created.Success || created.Error != "" {
		return "", errors.New(boundedAgentDiagnostic(created.Error))
	}
	return created.Backup.Name, nil
}

type siteCopyNewDatabase struct {
	ServerID   int
	DatabaseID int64
	UserID     int64
	Name       string
	User       string
	Password   string
	Operation  string
	CleanupKey string
}

// siteCopyCreateDatabase makes the target's new, empty MariaDB database and
// its user, owned by this operation so a failed copy removes exactly it.
func (p *Panel) siteCopyCreateDatabase(ctx context.Context, target wordpressSiteTarget, operationID string) (*siteCopyNewDatabase, error) {
	db := p.db.GetDB()
	var serverID int
	if err := db.QueryRowContext(ctx, `
		SELECT ds.id FROM database_servers ds JOIN database_server_types dst ON dst.id = ds.type_id
		WHERE ds.subscription_id = ? AND dst.name = 'mariadb' AND COALESCE(ds.status, 'active') <> 'retired'
		ORDER BY ds.is_default DESC, ds.id LIMIT 1`, target.SubscriptionID).Scan(&serverID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("no MariaDB server is installed for the target's subscription; the administrator installs MariaDB from Components")
		}
		return nil, err
	}
	base := sanitizeDBIdent(sanitizeName(target.Domain))
	if len(base) > 40 {
		base = base[:40]
	}
	suffix, err := randomHexToken(2)
	if err != nil {
		return nil, err
	}
	// alpha.134 follow-up: the copy's name always fits a right by its exact
	// name (each `_` counts twice), so the copy never stops on its length
	// after the safety backup was made.
	base = mariadbgrant.FitBase(base, "_copy_"+suffix)
	created := &siteCopyNewDatabase{ServerID: serverID, Name: base + "_copy_" + suffix, Operation: operationID}
	created.User = created.Name
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM databases_v2 WHERE server_id = ? AND name = ?)
		OR EXISTS(SELECT 1 FROM database_users WHERE server_id = ? AND username = ?)`,
		serverID, created.Name, serverID, created.User).Scan(&exists); err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("a database named " + created.Name + " already exists; start the copy again")
	}
	if created.Password, err = randomDBPassword(); err != nil {
		return nil, err
	}
	if created.CleanupKey, err = randomHexToken(32); err != nil {
		return nil, err
	}
	sealed, err := p.secrets.Encrypt(created.Password)
	if err != nil {
		return nil, err
	}
	var answer transport.CreateDatabaseResponse
	if err := siteCopyAgent(p, ctx, "Agent.CreateDatabase", transport.CreateDatabaseRequest{
		Type: "mysql", Name: created.Name, User: created.User, Password: created.Password,
		OperationID: operationID, CleanupToken: created.CleanupKey,
	}, &answer); err != nil {
		return nil, err
	}
	if !answer.Success || answer.Error != "" {
		return nil, errors.New(boundedAgentDiagnostic(answer.Error))
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO databases_v2 (server_id, subscription_id, domain_id, name) VALUES (?, ?, ?, ?)`,
		serverID, target.SubscriptionID, target.DomainID, created.Name)
	if err == nil {
		created.DatabaseID, err = res.LastInsertId()
	}
	if err == nil {
		res, err = tx.ExecContext(ctx, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (?, ?, ?, ?)`,
			serverID, target.SubscriptionID, created.User, sealed)
	}
	if err == nil {
		created.UserID, err = res.LastInsertId()
	}
	if err == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO database_user_grants (database_id, user_id, privileges) VALUES (?, ?, 'ALL')`, created.DatabaseID, created.UserID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		if !p.siteCopyRemoveDatabase(context.WithoutCancel(ctx), created) {
			return nil, fmt.Errorf("%w; the new database %s could not be removed from MariaDB, so it stays on the server: remove it there (DROP DATABASE and DROP USER %s) or ask the administrator", err, created.Name, created.User)
		}
		return nil, err
	}
	return created, nil
}

// siteCopyLeftoverNote tells where a database the copy made, and could not
// remove, stays (D-024: the problem and its fix together).
func siteCopyLeftoverNote(name, target string) string {
	return " The copy's new database " + name + " could not be removed, so it stays under " + target +
		" → Databases; delete it there when you no longer need it (deleting it again also retries the removal)."
}

// siteCopyRemoveDatabase removes a database this operation made (the Agent
// checks the operation's ownership proof first). The panel's records go only
// when the Agent proved the removal: a refused, failed or unanswered delete
// leaves a database in MariaDB, and its records keep it visible on the
// target's Databases page, where the owner can delete it (alpha.129 lab,
// Arch: the records went and the database stayed, unseen). It reports
// whether the database is gone.
func (p *Panel) siteCopyRemoveDatabase(ctx context.Context, created *siteCopyNewDatabase) bool {
	var answer transport.DeleteDatabaseResponse
	err := siteCopyAgent(p, ctx, "Agent.DeleteDatabase", transport.DeleteDatabaseRequest{
		Type: "mysql", Name: created.Name, User: created.User, RequireUserCleanup: true,
		RequireOwnershipProof: true, OperationID: created.Operation, CleanupToken: created.CleanupKey,
	}, &answer)
	if err != nil || !answer.Success || answer.Error != "" {
		log.Printf("[site-copy] the copy database %s was not removed (%s); its records are kept so it stays visible", created.Name,
			boundedAgentDiagnostic(firstNonEmpty(answer.Error, siteCopyErrText(err))))
		return false
	}
	db := p.db.GetDB()
	if created.DatabaseID > 0 {
		_, _ = db.ExecContext(ctx, `DELETE FROM databases_v2 WHERE id = ?`, created.DatabaseID)
	}
	if created.UserID > 0 {
		_, _ = db.ExecContext(ctx, `DELETE FROM database_users WHERE id = ?`, created.UserID)
	}
	return true
}

func siteCopyErrText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
