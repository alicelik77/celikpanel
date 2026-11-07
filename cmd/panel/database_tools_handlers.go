package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Database tools per database (alpha.128, G7): size and tables, export
// (download a dump), import (upload a dump; a safety copy is taken first and
// a load that stops puts the database back), copy into a new database, and
// check/repair. Every route sits under the domain, so ownership, team
// capability, suspension and the plan's databases permission are decided
// by the domain dispatcher before any of this runs; the database must also
// belong to that domain and subscription (databaseIdentity).
//
// Steps go to the progress window through the request identity
// (operation_steps.go). The engine work is the Agent's
// (cmd/agent/database_tools.go); an uploaded file is loaded as the
// database's own user, never as the engine's administrator.
//
// Veritabanı araçları: boyut/tablolar, dışa aktarma, güvenlik kopyalı içe
// aktarma, yeni veritabanına kopya, denetim/onarım. Hepsi alan adının altında;
// yetki ve sahiplik yönlendiricide karar verilir.

const (
	dbToolsInfoFresh       = 30 * time.Second
	dbToolsUploadMaxChunk  = 4 << 20
	errCodeDBImportAuth    = "DB_IMPORT_AUTH"
	errCodeDBImportClient  = "DB_IMPORT_CLIENT_UNSAFE"
	errCodeDBImportFile    = "DB_IMPORT_FILE_REFUSED"
	errCodeDBImportLoad    = "DB_IMPORT_LOAD_FAILED"
	errCodeDBImportUnwound = "DB_IMPORT_ROLLBACK_FAILED"
	errCodeDBImportUpload  = "DB_IMPORT_UPLOAD_MISSING"
	errCodeDBImportAccount = "DB_IMPORT_NO_SITE_ACCOUNT"
	errCodeDBToolsBusy     = "DB_TOOLS_RESTORE_RUNNING"
	errCodeDBCopyFailed    = "DB_COPY_FAILED"
	errCodeDBCheckFailed   = "DB_CHECK_FAILED"
)

var dbToolsHexID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// databaseToolsKinds are the dispatcher kinds of the tools routes.
var databaseToolsKinds = map[string][]string{
	"tools":         {http.MethodGet},
	"export":        {http.MethodGet, http.MethodPost},
	"import":        {http.MethodPost},
	"import/upload": {http.MethodPost},
	"copy":          {http.MethodPost},
	"check":         {http.MethodPost},
}

// matchDatabaseToolsRoute matches databases/{id}/<tool> under a domain.
func matchDatabaseToolsRoute(tail []string) (string, []string, bool) {
	if len(tail) < 3 || len(tail) > 4 || tail[0] != "databases" {
		return "", nil, false
	}
	if _, ok := strictPositiveID(tail[1]); !ok {
		return "", nil, false
	}
	tool := strings.Join(tail[2:], "/")
	methods, ok := databaseToolsKinds[tool]
	if !ok {
		return "", nil, false
	}
	return "database-" + strings.ReplaceAll(tool, "/", "-"), methods, true
}

func init() {
	// D-042: the plan's databases permission; an export download is the use
	// itself, so it is gated on reads too.
	for _, kind := range []string{"database-tools", "database-import", "database-import-upload", "database-copy", "database-check"} {
		domainSubrouteRights[kind] = domainSubrouteRight{right: rightDatabases}
	}
	domainSubrouteRights["database-export"] = domainSubrouteRight{right: rightDatabases, gateReads: true}
	// Team members: seeing the numbers is viewing; taking the data out or
	// changing it is managing.
	view := teamMemberDomainRequirement{capability: core.TeamCapabilityDatabases, mode: core.TeamPermissionView}
	manage := teamMemberDomainRequirement{capability: core.TeamCapabilityDatabases, mode: core.TeamPermissionManage}
	teamMemberDomainRequirements[teamMemberRouteKey{kind: "database-tools", method: http.MethodGet}] = view
	for _, key := range []teamMemberRouteKey{
		{"database-export", http.MethodGet}, {"database-export", http.MethodPost},
		{"database-import", http.MethodPost}, {"database-import-upload", http.MethodPost},
		{"database-copy", http.MethodPost}, {"database-check", http.MethodPost},
	} {
		teamMemberDomainRequirements[key] = manage
	}
}

// Guarded when the page sends an identity: never run twice, outlive a
// reload, and the progress window follows the steps.
var (
	requestIdentityRouteDBExport = requestIdentityRoute{pattern: "/api/v1/domains/{id}/databases/{id}/export", optional: true, timeout: agentRPCSiteLongTimeout + 5*time.Minute}
	requestIdentityRouteDBImport = requestIdentityRoute{pattern: "/api/v1/domains/{id}/databases/{id}/import", optional: true, timeout: agentRPCBulkImportTimeout + agentRPCSiteLongTimeout + 10*time.Minute}
	requestIdentityRouteDBCopy   = requestIdentityRoute{pattern: "/api/v1/domains/{id}/databases/{id}/copy", optional: true, timeout: agentRPCSiteLongTimeout + agentRPCDatabaseTimeout + 5*time.Minute,
		digest: requestIdentityDatabaseCopyDigest}
	requestIdentityRouteDBCheck = requestIdentityRoute{pattern: "/api/v1/domains/{id}/databases/{id}/check", optional: true, timeout: agentRPCSiteLongTimeout + 5*time.Minute}
)

// requestIdentityDatabaseCopyDigest leaves the new user's password out.
func requestIdentityDatabaseCopyDigest(body []byte) []byte {
	var req struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(body, &req)
	return []byte(req.Name)
}

func databaseToolsIdentityRoute(kind string) (requestIdentityRoute, bool) {
	switch kind {
	case "database-export":
		return requestIdentityRouteDBExport, true
	case "database-import":
		return requestIdentityRouteDBImport, true
	case "database-copy":
		return requestIdentityRouteDBCopy, true
	case "database-check":
		return requestIdentityRouteDBCheck, true
	}
	return requestIdentityRoute{}, false
}

type databaseToolsTarget struct {
	domain     backupDomain
	identity   backupspec.DatabaseIdentity
	serverID   int
	serverPort int
	status     string
	user       string
	sealed     string
}

func (t databaseToolsTarget) agentTarget() transport.DBToolsTarget {
	return transport.DBToolsTarget{SubscriptionID: t.domain.SubscriptionID, DomainID: t.domain.ID, Domain: t.domain.Name, Database: t.identity}
}

// loadDatabaseToolsTarget resolves the database of a tools route: it must
// belong to the domain and its subscription.
func (p *Panel) loadDatabaseToolsTarget(ctx context.Context, r *http.Request, domainID int) (databaseToolsTarget, error) {
	segments, ok := strictRouteSegments(r, "/api/v1/domains/")
	if !ok || len(segments) < 3 {
		return databaseToolsTarget{}, sql.ErrNoRows
	}
	dbID, ok := strictPositiveID(segments[2])
	if !ok {
		return databaseToolsTarget{}, sql.ErrNoRows
	}
	var target databaseToolsTarget
	var err error
	if target.domain, err = p.loadBackupDomain(ctx, domainID); err != nil {
		return target, err
	}
	if target.identity, err = p.databaseIdentity(ctx, target.domain, dbID); err != nil {
		return target, err
	}
	err = p.db.GetDB().QueryRowContext(ctx, `
		SELECT ds.id, ds.port, COALESCE(ds.status, 'active'),
		       COALESCE((SELECT u.username FROM database_user_grants g JOIN database_users u ON u.id = g.user_id
		                 WHERE g.database_id = d.id ORDER BY g.id LIMIT 1), ''),
		       COALESCE((SELECT u.password FROM database_user_grants g JOIN database_users u ON u.id = g.user_id
		                 WHERE g.database_id = d.id ORDER BY g.id LIMIT 1), '')
		FROM databases_v2 d JOIN database_servers ds ON ds.id = d.server_id
		WHERE d.id = ?`, dbID).Scan(&target.serverID, &target.serverPort, &target.status, &target.user, &target.sealed)
	return target, err
}

func (p *Panel) writeDatabaseToolsLoadError(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeClientError(w, http.StatusNotFound, "database not found")
		return
	}
	writeServerError(w, err)
}

// handleDatabaseTools dispatches the tools routes.
func (p *Panel) handleDatabaseTools(w http.ResponseWriter, r *http.Request, domainID int, kind string) {
	w.Header().Set("Content-Type", "application/json")
	switch kind {
	case "database-tools":
		p.handleDatabaseToolsInfo(w, r, domainID)
	case "database-export":
		if r.Method == http.MethodGet {
			p.handleDatabaseExportDownload(w, r, domainID)
		} else {
			p.handleDatabaseExport(w, r, domainID)
		}
	case "database-import-upload":
		p.handleDatabaseImportUpload(w, r, domainID)
	case "database-import":
		p.handleDatabaseImport(w, r, domainID)
	case "database-copy":
		p.handleDatabaseCopy(w, r, domainID)
	case "database-check":
		p.handleDatabaseCheck(w, r, domainID)
	default:
		http.NotFound(w, r)
	}
}

// ---- size, tables, connection facts (cached 30 s, never blocks) ----

type dbToolsCacheEntry struct {
	at   time.Time
	info transport.DBToolsInfoResponse
}

var dbToolsInfoCache = struct {
	sync.Mutex
	entries map[int]dbToolsCacheEntry
}{entries: map[int]dbToolsCacheEntry{}}

func forgetDatabaseToolsInfo(dbID int) {
	dbToolsInfoCache.Lock()
	delete(dbToolsInfoCache.entries, dbID)
	dbToolsInfoCache.Unlock()
}

func (p *Panel) handleDatabaseToolsInfo(w http.ResponseWriter, r *http.Request, domainID int) {
	target, err := p.loadDatabaseToolsTarget(r.Context(), r, domainID)
	if err != nil {
		p.writeDatabaseToolsLoadError(w, err)
		return
	}
	dbToolsInfoCache.Lock()
	entry, cached := dbToolsInfoCache.entries[target.identity.ID]
	dbToolsInfoCache.Unlock()
	if !cached || time.Since(entry.at) > dbToolsInfoFresh {
		var info transport.DBToolsInfoResponse
		if callErr := p.callAgentContext(r.Context(), "Agent.DatabaseToolsInfoV1", target.agentTarget(), &info); callErr != nil {
			info = transport.DBToolsInfoResponse{Error: "the database server could not be asked"}
		}
		entry = dbToolsCacheEntry{at: time.Now(), info: info}
		if info.Error == "" && info.Refusal == nil {
			dbToolsInfoCache.Lock()
			dbToolsInfoCache.entries[target.identity.ID] = entry
			dbToolsInfoCache.Unlock()
		}
	}
	info := entry.info
	port := target.serverPort
	if target.identity.Type == "postgresql" && info.ClusterPort > 0 {
		port = info.ClusterPort
	}
	if port <= 0 {
		port = map[string]int{"mysql": 3306, "mariadb": 3306, "postgresql": 5432}[target.identity.Type]
	}
	body := map[string]any{
		"id":          target.identity.ID,
		"name":        target.identity.Name,
		"type":        target.identity.Type,
		"host":        "localhost",
		"port":        port,
		"user":        target.user,
		"can_repair":  apiTypeNameFor(target.identity.Type) == "mysql",
		"read_at":     entry.at.UTC().Format(time.RFC3339),
		"known":       info.Error == "" && info.Refusal == nil,
		"size_bytes":  info.SizeBytes,
		"tables":      info.Tables,
		"connections": info.Connections,
	}
	if info.ClusterVersion != "" {
		body["cluster_version"] = info.ClusterVersion
	}
	if info.Refusal != nil {
		body["refusal"] = info.Refusal
	}
	switch {
	case info.ImportBlocker != "":
		body["import_blocker"] = "client_unsafe"
	case target.user == "" || target.sealed == "":
		body["import_blocker"] = "no_user"
	}
	_ = json.NewEncoder(w).Encode(body)
}

// ---- export ----

func newDBToolsID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func (p *Panel) handleDatabaseExport(w http.ResponseWriter, r *http.Request, domainID int) {
	ctx := r.Context()
	reportOperationPlan(ctx, "check", "dump", "ready")
	reportOperationStep(ctx, "check")
	target, err := p.loadDatabaseToolsTarget(ctx, r, domainID)
	if err != nil {
		p.writeDatabaseToolsLoadError(w, err)
		return
	}
	id, err := newDBToolsID()
	if err != nil {
		writeServerError(w, err)
		return
	}
	reportOperationStep(ctx, "dump")
	var resp transport.DBToolsExportResponse
	if err := p.callAgentContext(ctx, "Agent.DatabaseExportV1", transport.DBToolsExportRequest{Target: target.agentTarget(), ExportID: id}, &resp); err != nil {
		writeServerError(w, err)
		return
	}
	if resp.Refusal != nil {
		writeDatabaseClusterRefusal(w, r, resp.Refusal)
		return
	}
	if resp.Error != "" {
		writeAgentError(w, nil, resp.Error)
		return
	}
	reportOperationStep(ctx, "ready")
	p.audit(r, "database.export:"+target.identity.Name, "domain", domainID)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"export_id": id, "size": resp.Size, "filename": databaseExportFilename(target.identity.Name, target.identity.Type, time.Now()),
	})
}

// databaseExportFilename names a download: the database, its engine and the
// minute (alpha.129). Two databases of different engines may share a name,
// and their exports must never overwrite each other in a downloads folder.
// The engine is the server record's ("mariadb" or "postgresql").
func databaseExportFilename(name, engine string, at time.Time) string {
	if engine == "mysql" {
		engine = "mariadb"
	}
	return name + "-" + engine + "-" + at.UTC().Format("20060102-1504") + ".sql.gz"
}

func (p *Panel) handleDatabaseExportDownload(w http.ResponseWriter, r *http.Request, domainID int) {
	target, err := p.loadDatabaseToolsTarget(r.Context(), r, domainID)
	if err != nil {
		p.writeDatabaseToolsLoadError(w, err)
		return
	}
	id := r.URL.Query().Get("id")
	if !dbToolsHexID.MatchString(id) {
		writeClientError(w, http.StatusBadRequest, "invalid export identity")
		return
	}
	read := func(offset int64) (transport.DBToolsReadExportResponse, error) {
		var chunk transport.DBToolsReadExportResponse
		err := p.callAgentContext(r.Context(), "Agent.DatabaseExportReadV1", transport.DBToolsReadExportRequest{
			Target: target.agentTarget(), ExportID: id, Offset: offset, MaxBytes: backupspec.MaxChunkBytes,
		}, &chunk)
		if err == nil && chunk.Error != "" {
			err = errors.New(chunk.Error)
		}
		return chunk, err
	}
	chunk, err := read(0)
	if err != nil {
		writeCodedError(w, http.StatusGone, "DB_EXPORT_GONE",
			"This export file is no longer on the server (it is removed after one download, or after six hours). Nothing was changed. Make a new export.", "")
		return
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": databaseExportFilename(target.identity.Name, target.identity.Type, time.Now())}))
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Length", strconv.FormatInt(chunk.Size, 10))
	w.Header().Set("Cache-Control", "no-store")
	size := chunk.Size
	for {
		if len(chunk.Data) > 0 {
			if _, err := w.Write(chunk.Data); err != nil {
				return
			}
		}
		if chunk.EOF {
			return
		}
		if len(chunk.Data) == 0 || chunk.Size != size {
			log.Printf("database export download stopped at %d", chunk.Offset)
			return
		}
		if chunk, err = read(chunk.Offset); err != nil {
			log.Printf("database export download stopped: %v", err)
			return
		}
	}
}

// ---- import ----

func (p *Panel) handleDatabaseImportUpload(w http.ResponseWriter, r *http.Request, domainID int) {
	target, err := p.loadDatabaseToolsTarget(r.Context(), r, domainID)
	if err != nil {
		p.writeDatabaseToolsLoadError(w, err)
		return
	}
	id := r.URL.Query().Get("id")
	offset, offsetErr := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	if !dbToolsHexID.MatchString(id) || offsetErr != nil || offset < 0 {
		writeClientError(w, http.StatusBadRequest, "invalid upload identity or offset")
		return
	}
	if offset == 0 {
		// D-044: a full disk takes nothing new.
		if err := p.checkSubscriptionQuota(r.Context(), target.domain.SubscriptionID, quotaDisk); err != nil {
			writeQuotaRefusal(w, err)
			return
		}
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, dbToolsUploadMaxChunk))
	if err != nil {
		writeClientError(w, http.StatusRequestEntityTooLarge, "an upload chunk is at most 4 MiB")
		return
	}
	var resp transport.DBToolsUploadChunkResponse
	if err := p.callAgentContext(r.Context(), "Agent.DatabaseImportUploadV1", transport.DBToolsUploadChunkRequest{
		Target: target.agentTarget(), ImportID: id, Offset: offset, Data: data,
	}, &resp); err != nil {
		writeServerError(w, err)
		return
	}
	if resp.Error != "" {
		// The size the server holds lets the page continue from there.
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "the upload could not continue: " + boundedAgentDiagnostic(resp.Error), "size": resp.Size})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"size": resp.Size})
}

func (p *Panel) handleDatabaseImport(w http.ResponseWriter, r *http.Request, domainID int) {
	ctx := r.Context()
	reportOperationPlan(ctx, "check", "safety", "load", "finish")
	reportOperationStep(ctx, "check")
	var body struct {
		UploadID   string `json:"upload_id"`
		Size       int64  `json:"size"`
		EmptyFirst bool   `json:"empty_first"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil || !dbToolsHexID.MatchString(body.UploadID) || body.Size <= 0 {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	target, err := p.loadDatabaseToolsTarget(ctx, r, domainID)
	if err != nil {
		p.writeDatabaseToolsLoadError(w, err)
		return
	}
	if err := p.checkSubscriptionQuota(ctx, target.domain.SubscriptionID, quotaDisk); err != nil {
		writeQuotaRefusal(w, err)
		return
	}
	if target.user == "" || target.sealed == "" {
		writeCodedError(w, http.StatusConflict, errCodeDBImportAuth,
			"This database has no user with a stored password, so there is no account to load the file as. Nothing was changed. Add a user to the database on the Databases page, then import again.", "/databases")
		return
	}
	password, err := p.secrets.Decrypt(target.sealed)
	if err != nil {
		writeServerError(w, err)
		return
	}
	reportOperationStep(ctx, "safety")
	var safety transport.DBToolsSafetyResponse
	if err := p.callAgentContext(ctx, "Agent.DatabaseImportSafetyV1", transport.DBToolsSafetyRequest{Target: target.agentTarget(), ImportID: body.UploadID}, &safety); err != nil {
		writeServerError(w, err)
		return
	}
	if !p.writeDatabaseToolsRefusal(w, r, safety.Refusal, safety.Error) {
		return
	}
	reportOperationStep(ctx, "load")
	var resp transport.DBToolsImportResponse
	err = p.callAgentContext(ctx, "Agent.DatabaseImportV1", transport.DBToolsImportRequest{
		Target: target.agentTarget(), ImportID: body.UploadID, Size: body.Size, SafetyBackup: safety.SafetyBackup,
		EmptyFirst: body.EmptyFirst, User: target.user, Password: password,
	}, &resp)
	forgetDatabaseToolsInfo(target.identity.ID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !resp.Success {
		p.audit(r, "database.import.failed:"+target.identity.Name+" — "+auditReason(resp.Code), "domain", domainID)
		p.writeDatabaseImportFailure(w, r, resp, safety.SafetyBackup)
		return
	}
	reportOperationStep(ctx, "finish")
	p.audit(r, "database.import:"+target.identity.Name, "domain", domainID)
	p.requestDiskMeasurement(ctx, target.domain.SubscriptionID)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "safety_backup": safety.SafetyBackup})
}

// writeDatabaseToolsRefusal answers an Agent refusal; true when there was none.
func (p *Panel) writeDatabaseToolsRefusal(w http.ResponseWriter, r *http.Request, refusal *transport.DatabaseClusterRefusal, agentError string) bool {
	switch {
	case refusal != nil:
		writeDatabaseClusterRefusal(w, r, refusal)
	case agentError == backupspec.RestoreInProgress:
		writeCodedError(w, http.StatusConflict, errCodeDBToolsBusy,
			"A restore or import of this site is still running. Nothing was started. Wait until it ends, then try again.", "")
	case agentError != "":
		writeAgentError(w, nil, agentError)
	default:
		return true
	}
	return false
}

func (p *Panel) writeDatabaseImportFailure(w http.ResponseWriter, r *http.Request, resp transport.DBToolsImportResponse, safetyBackup string) {
	detail := []string{}
	if resp.Error != "" {
		detail = append(detail, boundedAgentDiagnostic(resp.Error))
	}
	if resp.Refusal != nil {
		writeDatabaseClusterRefusal(w, r, resp.Refusal)
		return
	}
	switch resp.Code {
	case transport.DBImportCodeAuth:
		writeCodedErrorDetails(w, http.StatusConflict, errCodeDBImportAuth,
			"The database server did not accept the password CelikPanel has stored for this database's user (it was probably changed outside the panel). Nothing was loaded. Set a new password for the user on the Databases page, then import again.", "/databases", detail)
	case transport.DBImportCodeClientUnsafe:
		writeCodedErrorDetails(w, http.StatusConflict, errCodeDBImportClient,
			"This server's MariaDB client is too old to load a file safely (it lacks sandbox mode). Nothing was loaded. Update the server's MariaDB packages, then import again.", "", detail)
	case transport.DBImportCodeNoSiteAccount:
		writeCodedErrorDetails(w, http.StatusConflict, errCodeDBImportAccount,
			"This domain has no hosting account to load the file under. Nothing was loaded.", "", detail)
	case transport.DBImportCodeNotFound, transport.DBImportCodeUploadIncorrect:
		writeCodedErrorDetails(w, http.StatusConflict, errCodeDBImportUpload,
			"The uploaded file is missing or incomplete on the server. Nothing was loaded. Choose the file and import again.", "", detail)
	case transport.DBImportCodeClientCommand, transport.DBImportCodeTransaction:
		writeCodedErrorDetails(w, http.StatusUnprocessableEntity, errCodeDBImportFile,
			"The file contains a command CelikPanel does not run, so it was not loaded and the database is unchanged. Remove that line from the file, then import again.", "", detail)
	case transport.DBImportCodeRollbackFailed:
		writeCodedErrorDetails(w, http.StatusInternalServerError, errCodeDBImportUnwound,
			fmt.Sprintf("Loading the file stopped, and putting the database back did not finish. The safety copy %s is kept: restore it from the Backups tab.", safetyBackup), "", detail)
	case transport.DBImportCodeLoadFailed:
		writeCodedErrorDetails(w, http.StatusUnprocessableEntity, errCodeDBImportLoad,
			"Loading the file stopped at an error, and the database was put back as it was before. Fix the line named below in the file, then import again.", "", detail)
	default:
		if resp.Error == backupspec.RestoreInProgress {
			p.writeDatabaseToolsRefusal(w, r, nil, resp.Error)
			return
		}
		writeAgentError(w, nil, resp.Error)
	}
}

// ---- copy ----

func (p *Panel) handleDatabaseCopy(w http.ResponseWriter, r *http.Request, domainID int) {
	ctx := r.Context()
	reportOperationPlan(ctx, "check", "create", "copy", "records")
	reportOperationStep(ctx, "check")
	var body struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeClientError(w, http.StatusBadRequest, "a name and a password are required")
		return
	}
	if refusal := checkDatabaseUserPassword(body.Password); refusal != nil {
		writePasswordRefusal(w, refusal)
		return
	}
	source, err := p.loadDatabaseToolsTarget(ctx, r, domainID)
	if err != nil {
		p.writeDatabaseToolsLoadError(w, err)
		return
	}
	if !p.requireSubscriptionOffering(w, r, source.domain.SubscriptionID, databaseEngineOffering(source.identity.Type), source.identity.Type, true) {
		return
	}
	if source.status == "retired" {
		writeClientError(w, http.StatusConflict, "this database's server takes no new database")
		return
	}
	if err := p.checkSubscriptionQuota(ctx, source.domain.SubscriptionID, quotaDBs); err != nil {
		writeQuotaRefusal(w, err)
		return
	}
	if err := p.checkSubscriptionQuota(ctx, source.domain.SubscriptionID, quotaDisk); err != nil {
		writeQuotaRefusal(w, err)
		return
	}
	dbName := fmt.Sprintf("%s_%s", sanitizeName(source.domain.Name), body.Name)
	dbUser := dbName + "_user"
	pool := p.db.GetDB()
	var exists bool
	if err := pool.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM databases_v2 WHERE server_id = ? AND name = ?)", source.serverID, dbName).Scan(&exists); err != nil {
		writeServerError(w, err)
		return
	}
	if exists {
		writeClientError(w, http.StatusConflict, "a database with this name already exists")
		return
	}
	sealed, err := p.secrets.Encrypt(body.Password)
	if err != nil {
		writeServerError(w, err)
		return
	}
	reportOperationStep(ctx, "create")
	// The engine record names MariaDB "mariadb"; the Agent's create and
	// delete take the API name "mysql" (alpha.128 lab: the copy of a MariaDB
	// database answered 500 "invalid database type").
	createReq := transport.CreateDatabaseRequest{Type: apiTypeNameFor(source.identity.Type), Name: dbName, User: dbUser, Password: body.Password}
	if source.identity.Type == "postgresql" {
		createReq.Port = source.serverPort
	}
	var created transport.CreateDatabaseResponse
	err = p.callAgentContext(ctx, "Agent.CreateDatabase", createReq, &created)
	if err == nil && created.Refusal != nil {
		writeDatabaseClusterRefusal(w, r, created.Refusal)
		return
	}
	if err == nil && created.NameTaken != "" {
		// alpha.129: the copy's name is taken on the engine; nothing changed.
		writeDatabaseNameTaken(w, created.NameTaken, map[string]string{"database": dbName, "user": dbUser}[created.NameTaken])
		return
	}
	if err != nil || !created.Success {
		writeAgentError(w, err, created.Error)
		return
	}
	newID, err := p.insertDatabaseToolsCopyRecords(ctx, source, dbName, dbUser, sealed)
	if err != nil {
		p.dropDatabaseToolsCopy(source, dbName, dbUser, 0)
		writeServerError(w, err)
		return
	}
	reportOperationStep(ctx, "copy")
	targetIdentity := withBackupCluster(backupspec.DatabaseIdentity{ID: int(newID), Name: dbName, Type: source.identity.Type}, source.serverPort)
	var copied transport.DBToolsCopyResponse
	err = p.callAgentContext(ctx, "Agent.DatabaseCopyV1", transport.DBToolsCopyRequest{Source: source.agentTarget(), Target: targetIdentity}, &copied)
	if err != nil || !copied.Success {
		// Nothing half-copied is left behind: the new database goes again.
		p.dropDatabaseToolsCopy(source, dbName, dbUser, int(newID))
		if err == nil && copied.Refusal != nil {
			writeDatabaseClusterRefusal(w, r, copied.Refusal)
			return
		}
		detail := []string{}
		if err == nil {
			detail = append(detail, boundedAgentDiagnostic(copied.Error))
		}
		p.audit(r, "database.copy.failed:"+source.identity.Name, "domain", domainID)
		writeCodedErrorDetails(w, http.StatusBadGateway, errCodeDBCopyFailed,
			"The copy could not be made, so the new database was removed again. The original is unchanged. Try again; if it fails again, the reason is below.", "", detail)
		return
	}
	reportOperationStep(ctx, "records")
	p.audit(r, "database.copy:"+source.identity.Name+"->"+dbName, "domain", domainID)
	p.requestDiskMeasurement(ctx, source.domain.SubscriptionID)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "id": newID, "name": dbName, "user": dbUser, "type": source.identity.Type})
}

func (p *Panel) insertDatabaseToolsCopyRecords(ctx context.Context, source databaseToolsTarget, dbName, dbUser, sealed string) (int64, error) {
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO databases_v2 (server_id, subscription_id, domain_id, name) VALUES (?, ?, ?, ?)`,
		source.serverID, source.domain.SubscriptionID, source.domain.ID, dbName)
	if err != nil {
		return 0, err
	}
	dbID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	var userID int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM database_users WHERE server_id = ? AND username = ?", source.serverID, dbUser).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		res, err = tx.ExecContext(ctx, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (?, ?, ?, ?)`,
			source.serverID, source.domain.SubscriptionID, dbUser, sealed)
		if err == nil {
			userID, err = res.LastInsertId()
		}
	}
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO database_user_grants (database_id, user_id, privileges) VALUES (?, ?, 'ALL')`, dbID, userID); err != nil {
		return 0, err
	}
	return dbID, tx.Commit()
}

// dropDatabaseToolsCopy removes a copy that did not finish (engine and records).
func (p *Panel) dropDatabaseToolsCopy(source databaseToolsTarget, dbName, dbUser string, dbID int) {
	ctx, cancel := context.WithTimeout(context.Background(), agentRPCDatabaseTimeout)
	defer cancel()
	req := transport.DeleteDatabaseRequest{Type: apiTypeNameFor(source.identity.Type), Name: dbName, User: dbUser}
	if source.identity.Type == "postgresql" {
		req.Port = source.serverPort
	}
	var resp transport.DeleteDatabaseResponse
	if err := p.callAgentContext(ctx, "Agent.DeleteDatabase", req, &resp); err != nil || !resp.Success {
		log.Printf("database copy cleanup: %s on the engine was not removed: %v %s", dbName, err, boundedAgentDiagnostic(resp.Error))
		return
	}
	if dbID == 0 {
		return
	}
	pool := p.db.GetDB()
	_, _ = pool.ExecContext(ctx, "DELETE FROM databases_v2 WHERE id = ?", dbID)
	_, _ = pool.ExecContext(ctx, `DELETE FROM database_users WHERE server_id = ? AND username = ?
		AND NOT EXISTS (SELECT 1 FROM database_user_grants g WHERE g.user_id = database_users.id)`, source.serverID, dbUser)
}

// ---- check and repair ----

func (p *Panel) handleDatabaseCheck(w http.ResponseWriter, r *http.Request, domainID int) {
	ctx := r.Context()
	var body struct {
		Repair bool `json:"repair"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	step := "check_tables"
	if body.Repair {
		step = "repair_tables"
	}
	reportOperationPlan(ctx, "check", step)
	reportOperationStep(ctx, "check")
	target, err := p.loadDatabaseToolsTarget(ctx, r, domainID)
	if err != nil {
		p.writeDatabaseToolsLoadError(w, err)
		return
	}
	if body.Repair && apiTypeNameFor(target.identity.Type) != "mysql" {
		writeClientError(w, http.StatusBadRequest, "PostgreSQL keeps its tables consistent itself; it has no repair step")
		return
	}
	reportOperationStep(ctx, step)
	var resp transport.DBToolsCheckResponse
	if err := p.callAgentContext(ctx, "Agent.DatabaseCheckV1", transport.DBToolsCheckRequest{Target: target.agentTarget(), Repair: body.Repair}, &resp); err != nil {
		writeServerError(w, err)
		return
	}
	if resp.Refusal != nil {
		writeDatabaseClusterRefusal(w, r, resp.Refusal)
		return
	}
	if resp.Error != "" {
		writeCodedErrorDetails(w, http.StatusBadGateway, errCodeDBCheckFailed,
			"The database server could not check this database. Nothing was changed. The server's answer is below.", "", []string{boundedAgentDiagnostic(resp.Error)})
		return
	}
	action := "database.check:"
	if body.Repair {
		action = "database.repair:"
	}
	p.audit(r, action+target.identity.Name, "domain", domainID)
	forgetDatabaseToolsInfo(target.identity.ID)
	if resp.Problems == nil {
		resp.Problems = []transport.DBToolsTableProblem{}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"checked": resp.Checked, "problems": resp.Problems, "self_repairing": resp.SelfRepairing, "repair": body.Repair,
	})
}
