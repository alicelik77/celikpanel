package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// WordPress tools (alpha.118, docs/WORDPRESS-TOOLS.md): status, updates with
// a backup first and a restore when they fail, a per-site switch for minor
// and security core releases, and a copy (staging) to another site of the
// same subscription. The right is "wordpress_tools" (D-042); the server
// enforces it, the screen only hides.
//
// Updates and copies run in the background as operations with a durable
// status; reading the status never starts anything (D-024). A restart in the
// middle leaves the operation "needs review" with the backup's name: the
// result is unknown and nothing is guessed (D-025).
//
// WordPress araçları: durum, önce yedekli güncelleme ve başarısızlıkta geri
// yükleme, küçük sürümler için site anahtarı ve aynı aboneliğe kopya.

const wordpressToolsPath = "/api/v1/wordpress/"

// Answer codes the screen turns into guidance (who acts, what to do).
const (
	errCodeWordPressNotDetected   = "WORDPRESS_NOT_DETECTED"
	errCodeWordPressBusy          = "WORDPRESS_OPERATION_RUNNING"
	errCodeWordPressNotPHP        = "WORDPRESS_NOT_PHP_SITE"
	errCodeWordPressCloneTarget   = "WORDPRESS_CLONE_TARGET_INVALID"
	errCodeWordPressDBNotLinked   = "WORDPRESS_DATABASE_NOT_LINKED"
	errCodeWordPressNoMariaDB     = "WORDPRESS_NO_MARIADB"
	errCodeWordPressBackupFailed  = "WORDPRESS_BACKUP_FAILED"
	errCodeWordPressResultUnknown = "WORDPRESS_RESULT_UNKNOWN"
	errCodeWordPressRestoreFailed = "WORDPRESS_RESTORE_FAILED"
	errCodeWordPressInterrupted   = "WORDPRESS_INTERRUPTED"
)

var wordpressOpenStatuses = []string{"queued", "backing_up", "updating", "restoring", "cloning"}

// wordpressOperationSlots runs one WordPress operation at a time on the
// server (each one backs up, downloads and copies whole sites).
var wordpressOperationSlots = make(chan struct{}, 1)

type wordpressSiteTarget struct {
	appInstallTarget
	DomainID   int
	PHPVersion string
}

func (t wordpressSiteTarget) site() transport.WordPressSite {
	return transport.WordPressSite{
		SiteID: t.SiteID, SubscriptionID: t.SubscriptionID, DomainID: t.DomainID,
		Domain: t.Domain, Username: services.SiteUsername(t.Domain), PHPVersion: t.PHPVersion,
	}
}

func (p *Panel) resolveWordPressSite(ctx context.Context, domainID int) (wordpressSiteTarget, error) {
	target, err := p.resolveAppInstallTarget(ctx, domainID)
	if err != nil {
		return wordpressSiteTarget{}, err
	}
	out := wordpressSiteTarget{appInstallTarget: target, DomainID: domainID}
	_ = p.db.GetDB().QueryRowContext(ctx, `SELECT COALESCE(php_version, '') FROM sites WHERE id = ?`, target.SiteID).Scan(&out.PHPVersion)
	if offeredVersionLine("php", out.PHPVersion) != "" {
		out.PHPVersion = offeredVersionLine("php", out.PHPVersion)
	}
	return out, nil
}

type wordpressOperationView struct {
	OperationID    string   `json:"operation_id"`
	Kind           string   `json:"kind"`
	Scope          string   `json:"scope,omitempty"`
	Items          []string `json:"items"`
	CoreVersion    string   `json:"core_version,omitempty"`
	TargetDomainID int      `json:"target_domain_id,omitempty"`
	TargetDomain   string   `json:"target_domain,omitempty"`
	Origin         string   `json:"origin"`
	Status         string   `json:"status"`
	BackupName     string   `json:"backup_name,omitempty"`
	Code           string   `json:"code,omitempty"`
	Detail         string   `json:"detail,omitempty"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
	// Open: the operation is still running; the screen keeps reading.
	Open bool `json:"open"`
}

func wordpressStatusOpen(status string) bool {
	for _, open := range wordpressOpenStatuses {
		if status == open {
			return true
		}
	}
	return false
}

// handleWordPressTools routes /api/v1/wordpress/{domainID}[/...].
func (p *Panel) handleWordPressTools(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, wordpressToolsPath), "/")
	segments := strings.Split(rest, "/")
	domainID, ok := strictPositiveID(segments[0])
	if !ok {
		http.NotFound(w, r)
		return
	}
	action := strings.Join(segments[1:], "/")
	var allowed []string
	switch {
	case action == "":
		allowed = []string{http.MethodGet}
	case action == "update", action == "clone":
		allowed = []string{http.MethodPost}
	case action == "clone-targets":
		allowed = []string{http.MethodGet}
	case action == "auto-update":
		allowed = []string{http.MethodPut}
	case len(segments) == 3 && segments[1] == "operations":
		allowed = []string{http.MethodGet}
	default:
		http.NotFound(w, r)
		return
	}
	if !routeAllows(r.Method, allowed) {
		rejectRouteMethod(w, allowed)
		return
	}
	// Ownership, suspension and team-member rules of every domain route;
	// additional users are not mapped to these tools.
	if !p.authorizeDomainSubroute(w, r, domainID, "wordpress-tools") {
		return
	}
	if !p.requireDomainRight(w, r, domainID, rightWordPressTools) {
		return
	}
	switch {
	case action == "":
		p.handleWordPressStatus(w, r, domainID)
	case action == "update":
		p.handleWordPressUpdate(w, r, domainID)
	case action == "clone":
		p.handleWordPressClone(w, r, domainID)
	case action == "clone-targets":
		p.handleWordPressCloneTargets(w, r, domainID)
	case action == "auto-update":
		p.handleWordPressAutoUpdate(w, r, domainID)
	default:
		p.handleWordPressOperation(w, r, domainID, segments[2])
	}
}

func (p *Panel) handleWordPressStatus(w http.ResponseWriter, r *http.Request, domainID int) {
	target, err := p.resolveWordPressSite(r.Context(), domainID)
	if errors.Is(err, sql.ErrNoRows) {
		writeClientError(w, http.StatusNotFound, "site not found")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	operations, err := p.wordpressOperations(r.Context(), domainID, 10)
	if err != nil {
		writeServerError(w, err)
		return
	}
	autoMinor := false
	_ = p.db.GetDB().QueryRowContext(r.Context(), `SELECT auto_minor_core = 1 FROM wordpress_site_settings WHERE domain_id = ?`, domainID).Scan(&autoMinor)
	answer := map[string]any{
		"domain_id":       domainID,
		"domain":          target.Domain,
		"project_type":    target.ProjectType,
		"auto_minor_core": autoMinor,
		"operations":      operations,
	}
	if target.ProjectType != "php" {
		answer["status"] = transport.WordPressStatusResponse{Code: errCodeWordPressNotPHP, Plugins: []transport.WordPressItem{}, Themes: []transport.WordPressItem{}}
		_ = json.NewEncoder(w).Encode(answer)
		return
	}
	var status transport.WordPressStatusResponse
	request := transport.WordPressStatusRequest{Site: target.site(), CheckUpdates: r.URL.Query().Get("check") != "0"}
	if err := p.callAgentContext(r.Context(), "Agent.WordPressStatusV1", &request, &status); err != nil {
		// Unknown, not a verified failure: the screen says the status could
		// not be read and offers to read again.
		writeAgentError(w, err, status.Error)
		return
	}
	if status.Plugins == nil {
		status.Plugins = []transport.WordPressItem{}
	}
	if status.Themes == nil {
		status.Themes = []transport.WordPressItem{}
	}
	if status.Detected {
		linked, _ := p.wordpressDatabaseLinked(r.Context(), target, status.DatabaseName)
		answer["database_linked"] = linked
	}
	// The database name is not shown to the screen (it is not needed there).
	status.DatabaseName, status.TablePrefix = "", ""
	answer["status"] = status
	_ = json.NewEncoder(w).Encode(answer)
}

// wordpressDatabaseLinked: the site's database is one of this domain's
// databases, so the domain's full backup includes it.
func (p *Panel) wordpressDatabaseLinked(ctx context.Context, target wordpressSiteTarget, name string) (bool, error) {
	if strings.TrimSpace(name) == "" {
		return false, nil
	}
	var count int
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM databases_v2 db
		JOIN database_servers ds ON ds.id = db.server_id
		JOIN database_server_types t ON t.id = ds.type_id
		WHERE db.name = ? AND db.domain_id = ? AND db.subscription_id = ? AND t.name = 'mariadb'`,
		name, target.DomainID, target.SubscriptionID).Scan(&count)
	return count > 0, err
}

func (p *Panel) wordpressOperations(ctx context.Context, domainID, limit int) ([]wordpressOperationView, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT o.operation_id, o.kind, o.scope, o.items, o.core_version, COALESCE(o.target_domain_id, 0),
		       COALESCE(d.name, ''), o.origin, o.status, COALESCE(o.backup_name, ''), COALESCE(o.code, ''),
		       COALESCE(o.detail, ''), o.created_at, o.updated_at
		FROM wordpress_operations o LEFT JOIN domains d ON d.id = o.target_domain_id
		WHERE o.domain_id = ? OR o.target_domain_id = ?
		ORDER BY o.id DESC LIMIT ?`, domainID, domainID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []wordpressOperationView{}
	for rows.Next() {
		var v wordpressOperationView
		var items string
		if err := rows.Scan(&v.OperationID, &v.Kind, &v.Scope, &items, &v.CoreVersion, &v.TargetDomainID,
			&v.TargetDomain, &v.Origin, &v.Status, &v.BackupName, &v.Code, &v.Detail, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		v.Items = []string{}
		_ = json.Unmarshal([]byte(items), &v.Items)
		v.Open = wordpressStatusOpen(v.Status)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (p *Panel) handleWordPressOperation(w http.ResponseWriter, r *http.Request, domainID int, operationID string) {
	if !wordpressOperationIDValid(operationID) {
		writeClientError(w, http.StatusNotFound, "operation not found")
		return
	}
	operations, err := p.wordpressOperations(r.Context(), domainID, 50)
	if err != nil {
		writeServerError(w, err)
		return
	}
	for _, operation := range operations {
		if operation.OperationID == operationID {
			_ = json.NewEncoder(w).Encode(operation)
			return
		}
	}
	writeClientError(w, http.StatusNotFound, "operation not found")
}

// insertWordPressOperation records a new operation; the open-operation index
// refuses a second one for the same site (a repeated click never starts two).
func (p *Panel) insertWordPressOperation(ctx context.Context, op wordpressOperationView, domainID, subscriptionID, requestedBy int) error {
	items, _ := json.Marshal(op.Items)
	var target any
	if op.TargetDomainID > 0 {
		target = op.TargetDomainID
	}
	var by any
	if requestedBy > 0 {
		by = requestedBy
	}
	_, err := p.db.GetDB().ExecContext(ctx, `
		INSERT INTO wordpress_operations (operation_id, domain_id, subscription_id, kind, scope, items,
			core_version, target_domain_id, origin, status, requested_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'queued', ?)`,
		op.OperationID, domainID, subscriptionID, op.Kind, op.Scope, string(items), op.CoreVersion, target, op.Origin, by)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique") {
		return errWordPressBusy
	}
	return err
}

var errWordPressBusy = errors.New("another WordPress operation is running for this site")

func (p *Panel) setWordPressOperation(operationID, status, code, detail, backupName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if len(detail) > 1000 {
		detail = detail[:1000]
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE wordpress_operations SET status = ?, code = NULLIF(?, ''), detail = NULLIF(?, ''),
			backup_name = COALESCE(NULLIF(?, ''), backup_name), updated_at = datetime('now')
		WHERE operation_id = ?`, status, code, detail, backupName, operationID); err != nil {
		log.Printf("[wordpress] operation %s status %s: %v", operationID, status, err)
	}
}

func writeWordPressBusy(w http.ResponseWriter) {
	writeCodedError(w, http.StatusConflict, errCodeWordPressBusy,
		"Another WordPress update or copy is running for this site, so nothing new was started. Wait for it to finish; this screen shows its result.", "")
}

func (p *Panel) handleWordPressUpdate(w http.ResponseWriter, r *http.Request, domainID int) {
	var body struct {
		Kind        string   `json:"kind"`
		Items       []string `json:"items"`
		CoreVersion string   `json:"core_version"`
	}
	if err := decodeBackupJSON(w, r, &body); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	switch body.Kind {
	case "core":
		body.Items = []string{}
		if strings.TrimSpace(body.CoreVersion) == "" {
			writeClientError(w, http.StatusBadRequest, "core_version is required")
			return
		}
	case "plugin":
		if len(body.Items) == 0 || len(body.Items) > 50 {
			writeClientError(w, http.StatusBadRequest, "choose between 1 and 50 plugins")
			return
		}
		body.CoreVersion = ""
	case "theme":
		// wordpress.org publishes no checksum for theme packages: refused
		// before anything is recorded, with the native path to use instead.
		writeCodedError(w, http.StatusConflict, transport.WordPressCodeNotVerifiable,
			"Nothing was changed: CelikPanel installs only packages it can verify, and wordpress.org publishes no checksum for themes. Update the theme from WordPress's own Dashboard > Updates page; make a backup first under Advanced > Backups on the domain's page.", "")
		return
	default:
		writeClientError(w, http.StatusBadRequest, "kind must be core, plugin or theme")
		return
	}
	target, err := p.resolveWordPressSite(r.Context(), domainID)
	if errors.Is(err, sql.ErrNoRows) {
		writeClientError(w, http.StatusNotFound, "site not found")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	if target.ProjectType != "php" {
		writeCodedError(w, http.StatusConflict, errCodeWordPressNotPHP, "WordPress tools work on PHP sites only.", "")
		return
	}
	operationID, err := randomHexToken(16)
	if err != nil {
		writeServerError(w, err)
		return
	}
	op := wordpressOperationView{OperationID: operationID, Kind: "update", Scope: body.Kind, Items: body.Items,
		CoreVersion: body.CoreVersion, Origin: "manual", Status: "queued"}
	if err := p.insertWordPressOperation(r.Context(), op, domainID, target.SubscriptionID, currentUserID(r)); err != nil {
		if errors.Is(err, errWordPressBusy) {
			writeWordPressBusy(w)
			return
		}
		writeServerError(w, err)
		return
	}
	p.audit(r, "wordpress.update:"+body.Kind, "domain", domainID)
	go p.runWordPressOperation(operationID, func(ctx context.Context) {
		p.runWordPressUpdate(ctx, operationID, target, body.Kind, body.Items, body.CoreVersion, false)
	})
	op.Open = true
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(op)
}

// runWordPressOperation waits for the server's operation slot, then runs.
func (p *Panel) runWordPressOperation(operationID string, run func(ctx context.Context)) {
	wordpressOperationSlots <- struct{}{}
	defer func() { <-wordpressOperationSlots }()
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[wordpress] operation %s stopped: %v", operationID, recovered)
			p.setWordPressOperation(operationID, "needs_review", errCodeWordPressResultUnknown,
				"the operation stopped unexpectedly; check the site", "")
		}
	}()
	run(context.Background())
}

// runWordPressUpdate: backup first, then the update, then — when files
// changed and the update failed — the backup is restored.
func (p *Panel) runWordPressUpdate(ctx context.Context, operationID string, target wordpressSiteTarget, kind string, items []string, coreVersion string, minorOnly bool) {
	d, err := p.loadBackupDomain(ctx, target.DomainID)
	if err != nil {
		p.setWordPressOperation(operationID, "failed", errCodeWordPressBackupFailed, "the site could not be read: "+err.Error(), "")
		return
	}
	databases, err := p.domainDatabaseIdentities(ctx, d)
	if err != nil {
		p.setWordPressOperation(operationID, "failed", errCodeWordPressBackupFailed, "the site's databases could not be read", "")
		return
	}
	backupType := backupspec.TypeFull
	if len(databases) == 0 {
		if kind == "core" {
			// A core update changes the database; without it in the backup
			// there is no way back, so nothing starts.
			p.setWordPressOperation(operationID, "failed", errCodeWordPressDBNotLinked,
				"the site's database is not linked to this domain, so the backup would not include it; nothing was changed", "")
			return
		}
		backupType = backupspec.TypeFiles
	}
	p.setWordPressOperation(operationID, "backing_up", "", "", "")
	createReq := backupspec.CreateRequest{
		ProtocolVersion: backupspec.ProtocolVersion, SubscriptionID: d.SubscriptionID, DomainID: d.ID,
		DomainName: d.Name, Type: backupType, Origin: backupspec.OriginPreUpdate,
		JobKey: "wordpress:" + operationID, SourceDir: d.DocumentRoot,
	}
	if backupType == backupspec.TypeFull {
		createReq.Databases = databases
	}
	var created backupspec.CreateResponse
	if err := p.callAgentContext(ctx, "Agent.CreateBackup", &createReq, &created); err != nil || !created.Success || created.Error != "" {
		detail := created.Error
		if err != nil {
			detail = err.Error()
		}
		p.setWordPressOperation(operationID, "failed", errCodeWordPressBackupFailed,
			"the backup before the update could not be made, so nothing was changed: "+boundedAgentDiagnostic(detail), "")
		return
	}
	backupName := created.Backup.Name
	p.setWordPressOperation(operationID, "updating", "", "", backupName)

	request := transport.WordPressUpdateRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), OperationID: operationID, Site: target.site(),
		Kind: kind, Items: items, CoreVersion: coreVersion, MinorOnly: minorOnly,
	}
	var answer transport.WordPressUpdateResponse
	if err := p.callAgentContext(ctx, "Agent.WordPressUpdateV1", &request, &answer); err != nil {
		// The answer was lost: the update may or may not have run.
		p.setWordPressOperation(operationID, "needs_review", errCodeWordPressResultUnknown,
			"the update's answer was lost, so its result is not known; open the site, and restore the backup "+backupName+" from Backups if it does not work", backupName)
		p.auditBackupSystem(ctx, "wordpress.update.unknown", target.DomainID)
		return
	}
	if answer.Error == "" {
		p.setWordPressOperation(operationID, "applied", "", answer.Detail, backupName)
		p.auditBackupSystem(ctx, "wordpress.update.applied:"+kind, target.DomainID)
		return
	}
	detail := boundedAgentDiagnostic(answer.Error)
	switch {
	case !answer.Changed:
		p.setWordPressOperation(operationID, "failed", answer.Code, "nothing was changed: "+detail, backupName)
	case answer.RolledBack:
		p.setWordPressOperation(operationID, "rolled_back", answer.Code, "the previous files were put back: "+detail, backupName)
	default:
		p.setWordPressOperation(operationID, "restoring", answer.Code, detail, backupName)
		if err := p.restoreDomainBackup(ctx, d, backupName); err != nil {
			code := errCodeWordPressRestoreFailed
			// alpha.127: a PostgreSQL prerequisite says which one.
			var refusal *pgClusterRefusalError
			if errors.As(err, &refusal) {
				code = "WORDPRESS_RESTORE_" + pgClusterRefusalCodeSuffix(refusal.refusal.Reason)
			}
			p.setWordPressOperation(operationID, "needs_review", code,
				"the update failed ("+detail+") and restoring the backup "+backupName+" failed too: "+boundedAgentDiagnostic(err.Error()), backupName)
			p.auditBackupSystem(ctx, "wordpress.update.restore_failed", target.DomainID)
			return
		}
		p.setWordPressOperation(operationID, "rolled_back", answer.Code, "the backup made before the update was restored: "+detail, backupName)
	}
	p.auditBackupSystem(ctx, "wordpress.update.failed:"+kind, target.DomainID)
}

// restoreDomainBackup restores one backup of a domain the way the Backups
// tab does: inspected, its databases authorized against the domain again.
func (p *Panel) restoreDomainBackup(ctx context.Context, d backupDomain, name string) error {
	inspectReq := backupspec.InspectRequest{
		ProtocolVersion: backupspec.ProtocolVersion, SubscriptionID: d.SubscriptionID,
		DomainID: d.ID, DomainName: d.Name, BackupName: name,
	}
	var inspection backupspec.InspectResponse
	if err := p.callAgentContext(ctx, "Agent.InspectBackup", &inspectReq, &inspection); err != nil {
		return err
	}
	if !inspection.Success || inspection.Error != "" {
		return errors.New(inspection.Error)
	}
	if !inspection.Backup.Restorable {
		return errors.New("the backup cannot be restored safely")
	}
	restoreReq := backupspec.RestoreRequest{
		ProtocolVersion: backupspec.ProtocolVersion, SubscriptionID: d.SubscriptionID,
		DomainID: d.ID, DomainName: d.Name, BackupName: name, TargetDir: d.DocumentRoot,
	}
	if err := p.authorizeRestoreDatabases(ctx, d, inspection, &restoreReq); err != nil {
		return err
	}
	var resp backupspec.RestoreResponse
	if err := p.callAgentContext(ctx, "Agent.RestoreBackup", &restoreReq, &resp); err != nil {
		return err
	}
	if resp.Refusal != nil {
		// alpha.127: a PostgreSQL prerequisite; nothing was written.
		return &pgClusterRefusalError{refusal: transport.DatabaseClusterRefusal(*resp.Refusal)}
	}
	if !resp.Success || resp.Error != "" {
		return errors.New(resp.Error)
	}
	return nil
}

func (p *Panel) handleWordPressAutoUpdate(w http.ResponseWriter, r *http.Request, domainID int) {
	var body struct {
		MinorCore bool `json:"minor_core"`
	}
	if err := decodeBackupJSON(w, r, &body); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	value := 0
	if body.MinorCore {
		value = 1
	}
	if _, err := p.db.GetDB().ExecContext(r.Context(), `
		INSERT INTO wordpress_site_settings (domain_id, auto_minor_core, updated_at) VALUES (?, ?, datetime('now'))
		ON CONFLICT(domain_id) DO UPDATE SET auto_minor_core = excluded.auto_minor_core, updated_at = datetime('now')`,
		domainID, value); err != nil {
		writeServerError(w, err)
		return
	}
	p.audit(r, fmt.Sprintf("wordpress.auto_minor_core:%t", body.MinorCore), "domain", domainID)
	_ = json.NewEncoder(w).Encode(map[string]bool{"auto_minor_core": body.MinorCore})
}

// handleWordPressCloneTargets lists the other PHP sites of the subscription.
// Whether a target is empty is checked by the Agent when the copy starts.
func (p *Panel) handleWordPressCloneTargets(w http.ResponseWriter, r *http.Request, domainID int) {
	source, err := p.resolveWordPressSite(r.Context(), domainID)
	if err != nil {
		writeClientError(w, http.StatusNotFound, "site not found")
		return
	}
	rows, err := p.db.GetDB().QueryContext(r.Context(), `
		SELECT d.id, d.name FROM domains d JOIN sites s ON s.domain_id = d.id
		WHERE d.subscription_id = ? AND d.id <> ? AND COALESCE(s.project_type, 'php') = 'php'
		GROUP BY d.id ORDER BY d.name`, source.SubscriptionID, domainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer rows.Close()
	type candidate struct {
		DomainID int    `json:"domain_id"`
		Domain   string `json:"domain"`
	}
	out := []candidate{}
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.DomainID, &c.Domain); err != nil {
			writeServerError(w, err)
			return
		}
		out = append(out, c)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"targets": out})
}

func (p *Panel) handleWordPressClone(w http.ResponseWriter, r *http.Request, domainID int) {
	var body struct {
		TargetDomainID int `json:"target_domain_id"`
	}
	if err := decodeBackupJSON(w, r, &body); err != nil || body.TargetDomainID <= 0 || body.TargetDomainID == domainID {
		writeClientError(w, http.StatusBadRequest, "target_domain_id is required")
		return
	}
	source, err := p.resolveWordPressSite(r.Context(), domainID)
	if err != nil {
		writeClientError(w, http.StatusNotFound, "site not found")
		return
	}
	target, err := p.resolveWordPressSite(r.Context(), body.TargetDomainID)
	if err != nil || target.SubscriptionID != source.SubscriptionID {
		writeCodedError(w, http.StatusConflict, errCodeWordPressCloneTarget,
			"A copy goes to another site of the same subscription. Choose one from the list; nothing was changed.", "")
		return
	}
	if source.ProjectType != "php" || target.ProjectType != "php" {
		writeCodedError(w, http.StatusConflict, errCodeWordPressNotPHP, "Both sites must be PHP sites; nothing was changed.", "")
		return
	}
	// The target must be one the caller may also manage with these tools.
	if !p.authorizeDomainSubroute(w, r, target.DomainID, "wordpress-tools") || !p.requireDomainRight(w, r, target.DomainID, rightWordPressTools) {
		return
	}
	// The source's database, read from its wp-config.php.
	var status transport.WordPressStatusResponse
	statusReq := transport.WordPressStatusRequest{Site: source.site()}
	if err := p.callAgentContext(r.Context(), "Agent.WordPressStatusV1", &statusReq, &status); err != nil {
		writeAgentError(w, err, status.Error)
		return
	}
	if !status.Detected {
		writeCodedError(w, http.StatusConflict, errCodeWordPressNotDetected, "WordPress was not found on this site; nothing was changed.", "")
		return
	}
	if linked, err := p.wordpressDatabaseLinked(r.Context(), source, status.DatabaseName); err != nil {
		writeServerError(w, err)
		return
	} else if !linked {
		writeCodedError(w, http.StatusConflict, errCodeWordPressDBNotLinked,
			"The site's database is not one of this domain's databases in the panel, so it cannot be copied safely; nothing was changed.", "")
		return
	}
	// Limits: the copy is a new database and as much disk as the source.
	if err := p.ensureInstalledDBServers(r.Context(), source.SubscriptionID); err != nil {
		writeServerError(w, err)
		return
	}
	if err := p.checkSubscriptionQuota(r.Context(), source.SubscriptionID, quotaDBs); err != nil {
		writeQuotaRefusal(w, err)
		return
	}
	if err := p.checkSubscriptionQuota(r.Context(), source.SubscriptionID, quotaDisk); err != nil {
		writeQuotaRefusal(w, err)
		return
	}
	base := sanitizeDBIdent(services.SiteUsername(target.Domain))
	databaseName, databaseUser := base+"_wp", base+"_wp"
	serverID, err := p.preflightAppDatabase(r.Context(), source.SubscriptionID, databaseName, databaseUser)
	if errors.Is(err, sql.ErrNoRows) {
		writeCodedError(w, http.StatusConflict, errCodeWordPressNoMariaDB,
			"No MariaDB server is installed for this subscription. The administrator installs MariaDB from Components; then start the copy again.", adminInstallAction(r, "mariadb"))
		return
	}
	if errors.Is(err, errAppDatabaseExists) {
		writeCodedError(w, http.StatusConflict, errCodeWordPressCloneTarget,
			"The target site already has a WordPress database; choose an empty site. Nothing was changed.", "")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	operationID, err := randomHexToken(16)
	if err != nil {
		writeServerError(w, err)
		return
	}
	op := wordpressOperationView{OperationID: operationID, Kind: "clone", Items: []string{}, TargetDomainID: target.DomainID,
		TargetDomain: target.Domain, Origin: "manual", Status: "queued"}
	if err := p.insertWordPressOperation(r.Context(), op, domainID, source.SubscriptionID, currentUserID(r)); err != nil {
		if errors.Is(err, errWordPressBusy) {
			writeWordPressBusy(w)
			return
		}
		writeServerError(w, err)
		return
	}
	p.audit(r, fmt.Sprintf("wordpress.clone:to:%d", target.DomainID), "domain", domainID)
	go p.runWordPressOperation(operationID, func(ctx context.Context) {
		p.runWordPressClone(ctx, operationID, source, target, status.DatabaseName, serverID, databaseName, databaseUser)
	})
	op.Open = true
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(op)
}

// runWordPressClone creates the copy's database (recorded like an app
// install, with an ownership token so a failed copy can remove exactly that
// database), then asks the Agent to copy.
func (p *Panel) runWordPressClone(ctx context.Context, operationID string, source, target wordpressSiteTarget,
	sourceDatabase string, serverID int, databaseName, databaseUser string) {
	p.setWordPressOperation(operationID, "cloning", "", "", "")
	fail := func(status, code, detail string) {
		p.setWordPressOperation(operationID, status, code, detail, "")
		p.auditBackupSystem(ctx, "wordpress.clone."+status, source.DomainID)
	}
	password, err := randomDBPassword()
	if err != nil {
		fail("failed", "", "nothing was changed: "+err.Error())
		return
	}
	cleanupToken, err := randomHexToken(32)
	if err != nil {
		fail("failed", "", "nothing was changed: "+err.Error())
		return
	}
	sealedPassword, err := p.secrets.Encrypt(password)
	if err != nil {
		fail("failed", "", "nothing was changed: "+err.Error())
		return
	}
	sealedToken, err := p.secrets.Encrypt(cleanupToken)
	if err != nil {
		fail("failed", "", "nothing was changed: "+err.Error())
		return
	}
	ledger, err := p.storeAppDatabase(ctx, serverID, target.SubscriptionID, target.DomainID, target.SiteID,
		operationID, databaseName, databaseUser, sealedPassword, sealedToken)
	if err != nil {
		fail("failed", "", "nothing was changed: "+err.Error())
		return
	}
	_ = p.setAppInstallStatus(operationID, "database_creating", "")
	var created transport.CreateDatabaseResponse
	if err := p.callAgentContext(ctx, "Agent.CreateDatabase", transport.CreateDatabaseRequest{
		Type: "mysql", Name: databaseName, User: databaseUser, Password: password,
		OperationID: operationID, CleanupToken: cleanupToken,
	}, &created); err != nil {
		_ = p.persistAppInstallOutcome(operationID, "needs_review", err.Error(), err)
		fail("needs_review", errCodeWordPressResultUnknown, "creating the copy's database gave no answer; check Databases for "+databaseName)
		return
	}
	if !created.Success || created.Error != "" || !created.OwnedByOperation {
		status := "failed"
		if created.CleanupIncomplete || (created.Success && !created.OwnedByOperation) {
			status = "needs_review"
		} else if cleanupErr := p.removeAppDatabaseLedger(&ledger); cleanupErr != nil {
			status = "needs_review"
		}
		_ = p.persistAppInstallOutcome(operationID, status, created.Error, nil)
		fail(status, "", "the copy's database could not be created: "+boundedAgentDiagnostic(created.Error))
		return
	}
	_ = p.setAppInstallStatus(operationID, "database_ready", "")
	_ = p.setAppInstallStatus(operationID, "files_installing", "")
	request := transport.WordPressCloneRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), OperationID: operationID,
		Source: source.site(), Target: target.site(), SourceDatabase: sourceDatabase,
		TargetDatabase: databaseName, TargetDBUser: databaseUser, TargetDBPass: password,
	}
	var answer transport.WordPressCloneResponse
	if err := p.callAgentContext(ctx, "Agent.WordPressCloneV1", &request, &answer); err != nil {
		_ = p.persistAppInstallOutcome(operationID, "needs_review", err.Error(), err)
		fail("needs_review", errCodeWordPressResultUnknown, "the copy's answer was lost, so its result is not known; open "+target.Domain+" to check")
		return
	}
	if !answer.Cloned {
		detail := boundedAgentDiagnostic(answer.Error)
		if !answer.CompensationSafe {
			_ = p.persistAppInstallOutcome(operationID, "needs_review", answer.Error, nil)
			fail("needs_review", answer.Code, "the copy stopped after it began publishing: "+detail)
			return
		}
		if cleanupErr := p.compensateAppDatabase(&ledger, databaseName, databaseUser, operationID, cleanupToken); cleanupErr != nil {
			_ = p.persistAppInstallOutcome(operationID, "needs_review", cleanupErr.Error(), cleanupErr)
			fail("needs_review", answer.Code, "the copy failed ("+detail+") and its new database "+databaseName+" could not be removed: "+boundedAgentDiagnostic(cleanupErr.Error()))
			return
		}
		_ = p.persistAppInstallOutcome(operationID, "failed", answer.Error, nil)
		fail("failed", answer.Code, "nothing was kept: "+detail)
		return
	}
	_ = p.setAppInstallStatus(operationID, "applied", "")
	p.setWordPressOperation(operationID, "applied", "", fmt.Sprintf("copied to %s; the address was changed in %d values", target.Domain, answer.ReplacedValues), "")
	p.auditBackupSystem(ctx, "wordpress.clone.applied", source.DomainID)
}

// recoverInterruptedWordPressOperations never guesses: an operation a
// restart interrupted is marked for review with what is known.
func (p *Panel) recoverInterruptedWordPressOperations(ctx context.Context) (int64, error) {
	result, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE wordpress_operations
		SET status = CASE WHEN status = 'queued' THEN 'failed' ELSE 'needs_review' END,
		    code = ?,
		    detail = CASE WHEN status = 'queued'
		        THEN 'the panel restarted before the operation began; nothing was changed. Start it again.'
		        ELSE 'the panel restarted during the operation, so its result is not known; open the site, and restore the backup from Backups if it does not work' END,
		    updated_at = datetime('now')
		WHERE status IN ('queued', 'backing_up', 'updating', 'restoring', 'cloning')`, errCodeWordPressInterrupted)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// --- scheduled minor and security core releases ---------------------------

const wordpressAutoUpdateInterval = time.Hour

func (p *Panel) startWordPressAutoUpdater() {
	go func() {
		ticker := time.NewTicker(wordpressAutoUpdateInterval)
		defer ticker.Stop()
		for range ticker.C {
			p.runDueWordPressAutoUpdates(context.Background())
		}
	}()
}

// runDueWordPressAutoUpdates looks once a day at every site whose switch is
// on and installs a newer release of the same branch, with a backup first.
// A site whose subscription is suspended or no longer holds the right is
// skipped (what runs keeps running; nothing new starts).
func (p *Panel) runDueWordPressAutoUpdates(ctx context.Context) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT domain_id FROM wordpress_site_settings
		WHERE auto_minor_core = 1
		  AND (last_auto_check IS NULL OR last_auto_check <= datetime('now', '-20 hours'))
		ORDER BY domain_id LIMIT 200`)
	if err != nil {
		log.Printf("[wordpress] automatic updates: %v", err)
		return
	}
	var due []int
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil {
			due = append(due, id)
		}
	}
	rows.Close()
	for _, domainID := range due {
		_, _ = p.db.GetDB().ExecContext(ctx, `UPDATE wordpress_site_settings SET last_auto_check = datetime('now') WHERE domain_id = ?`, domainID)
		p.autoUpdateWordPressSite(ctx, domainID)
	}
}

func (p *Panel) autoUpdateWordPressSite(ctx context.Context, domainID int) {
	target, err := p.resolveWordPressSite(ctx, domainID)
	if err != nil || target.ProjectType != "php" {
		return
	}
	if suspended, err := p.subscriptionSuspended(ctx, target.SubscriptionID); err != nil || suspended {
		return
	}
	if ok, err := p.subscriptionHasRight(ctx, int64(target.SubscriptionID), rightWordPressTools); err != nil || !ok {
		return
	}
	var status transport.WordPressStatusResponse
	request := transport.WordPressStatusRequest{Site: target.site(), CheckUpdates: true}
	if err := p.callAgentContext(ctx, "Agent.WordPressStatusV1", &request, &status); err != nil || !status.Detected || !status.UpdatesChecked || status.CoreMinor == "" {
		return
	}
	operationID, err := randomHexToken(16)
	if err != nil {
		return
	}
	op := wordpressOperationView{OperationID: operationID, Kind: "update", Scope: "core", Items: []string{},
		CoreVersion: status.CoreMinor, Origin: "scheduled", Status: "queued"}
	if err := p.insertWordPressOperation(ctx, op, domainID, target.SubscriptionID, 0); err != nil {
		return
	}
	p.auditBackupSystem(ctx, "wordpress.update.scheduled:core", domainID)
	p.runWordPressOperation(operationID, func(ctx context.Context) {
		p.runWordPressUpdate(ctx, operationID, target, "core", nil, status.CoreMinor, true)
	})
}

// wordpressOperationIDValid: the 32 lowercase hex characters of an id.
func wordpressOperationIDValid(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := strconv.ParseUint(id[:16], 16, 64)
	return err == nil && strings.ToLower(id) == id
}
