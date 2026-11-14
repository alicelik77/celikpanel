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

	"github.com/alicelik/celikpanel/internal/mariadbgrant"
	"github.com/alicelik/celikpanel/internal/repositories"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Domain Database Management Handlers
//
// These endpoints keep the v1 URL and payload shape the domain-detail page
// uses, but store metadata in the same tables as the v2 flow (databases_v2 +
// database_users + database_user_grants) so both screens see one truth.
// Engine-side work stays on the privileged agent RPC.
//
// Bu uç noktalar domain-detay sayfasının kullandığı v1 URL ve gövde biçimini
// korur; ama meta veriyi v2 akışıyla aynı tablolara yazar (databases_v2 +
// database_users + database_user_grants), böylece iki ekran tek gerçeği
// görür. Motor tarafı işler yetkili agent RPC'sinde kalır.

// DatabaseInfo represents a database associated with a domain
type DatabaseInfo struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"` // mysql or postgresql
	User      string `json:"user"`
	CreatedAt string `json:"created_at"`
	Size      string `json:"size,omitempty"`
	// alpha.129: the engine the server runs (mariadb, mysql or postgresql)
	// and its version as the server reported it (major only), and every
	// user of the database with its access.
	Engine        string               `json:"engine"`
	EngineVersion string               `json:"engine_version,omitempty"`
	Users         []domainDatabaseUser `json:"users"`
}

// CreateDatabaseRequest represents a request to create a database
type CreateDatabaseRequest struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // mysql or postgresql
	Password string `json:"password"`
}

// serverTypeNameFor maps the API's database type to the
// database_server_types.name the v2 tables use.
// serverTypeNameFor, API'nin veritabanı tipini v2 tablolarının kullandığı
// database_server_types.name değerine eşler.
func serverTypeNameFor(apiType string) string {
	if apiType == "mysql" {
		return "mariadb"
	}
	return apiType
}

// apiTypeNameFor is the reverse mapping, for responses and agent calls.
// apiTypeNameFor, yanıtlar ve agent çağrıları için ters eşlemedir.
func apiTypeNameFor(serverType string) string {
	if serverType == "mariadb" {
		return "mysql"
	}
	return serverType
}

// availableDatabaseTypes returns only the active engines registered for one
// subscription. The domain dispatcher has already authorized the domain; this
// query deliberately stays on that domain's tenant boundary and never asks the
// global host capability or agent surfaces.
func availableDatabaseTypes(ctx context.Context, pool *sql.DB, subscriptionID int) ([]string, error) {
	rows, err := pool.QueryContext(ctx, `
		SELECT DISTINCT dst.name
		FROM database_servers ds
		JOIN database_server_types dst ON dst.id = ds.type_id
		WHERE ds.subscription_id = ?
		  AND ds.status = 'active'
		  AND dst.name IN ('mariadb', 'postgresql')
		ORDER BY dst.name
	`, subscriptionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	types := make([]string, 0, 2)
	for rows.Next() {
		var serverType string
		if err := rows.Scan(&serverType); err != nil {
			return nil, err
		}
		types = append(types, apiTypeNameFor(serverType))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return types, nil
}

// databaseEngineLabel names the engine a server record runs, for the
// database cards (alpha.129): a "mariadb" record is MariaDB unless its
// version is one only MySQL has (8 and later, without "MariaDB"; MariaDB's
// own majors are 5.5, 10, 11 and 12, and a stored version may be the bare
// "10.11"). The version is
// cut to its major: "11.8" for MariaDB/MySQL, "17" for PostgreSQL. An
// unknown version is left out, never guessed.
//
// databaseEngineLabel, kartlar için motorun adı ve ana sürümü.
func databaseEngineLabel(serverType, version string) (string, string) {
	version = strings.TrimSpace(version)
	switch serverType {
	case "postgresql":
		return "postgresql", leadingVersion(version, 1)
	case "mariadb", "mysql":
		major := leadingVersion(version, 2)
		if !strings.Contains(strings.ToLower(version), "mariadb") && mysqlOnlyMajor(major) {
			return "mysql", major
		}
		return "mariadb", major
	}
	return serverType, ""
}

// mysqlOnlyMajor: MySQL 8.x and 9.x; MariaDB never used those numbers.
func mysqlOnlyMajor(major string) bool {
	return strings.HasPrefix(major, "8.") || strings.HasPrefix(major, "9.") || major == "8" || major == "9"
}

// leadingVersion keeps the first `parts` numeric parts of a version ("11.8.6-
// MariaDB" with 2 is "11.8"; "17.11 (Debian…)" with 1 is "17").
func leadingVersion(version string, parts int) string {
	end := 0
	seen := 0
	for end < len(version) {
		c := version[end]
		if c >= '0' && c <= '9' {
			end++
			continue
		}
		if c == '.' && end > 0 && seen+1 < parts {
			seen++
			end++
			continue
		}
		break
	}
	out := strings.TrimSuffix(version[:end], ".")
	return out
}

// handleDomainDatabases handles GET/POST for domain databases
func (p *Panel) handleDomainDatabases(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract domain ID
	domainID, err := extractDomainID(r.URL.Path, "/api/v1/domains/", "/databases")
	if err != nil {
		http.Error(w, "Invalid domain ID", http.StatusBadRequest)
		return
	}

	if r.Method == "GET" {
		p.handleGetDomainDatabases(w, r, domainID)
	} else if r.Method == "POST" {
		p.handleCreateDatabase(w, r, domainID)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (p *Panel) handleGetDomainDatabases(w http.ResponseWriter, r *http.Request, domainID int) {
	ctx := r.Context()
	pool := p.db.GetDB()

	// Get domain info
	domainRepo := repositories.NewPostgresDomainRepository(pool)
	domain, err := domainRepo.GetByID(ctx, domainID)
	if err != nil {
		http.Error(w, "Domain not found", http.StatusNotFound)
		return
	}

	availableTypes, err := availableDatabaseTypes(ctx, pool, domain.SubscriptionID)
	if err != nil {
		http.Error(w, "Failed to load databases", http.StatusInternalServerError)
		return
	}
	// alpha.117 (D-042): only the engines the subscription may choose.
	if !callerIsAdministrator(r) {
		rights, err := p.effectiveSubscriptionRights(ctx, domain.SubscriptionID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		open := []string{}
		for _, engine := range availableTypes {
			if rights.hasPermission(rightDatabases) && rights.anyVersionRefusal(databaseEngineOffering(engine)) == "" {
				open = append(open, engine)
			}
		}
		availableTypes = open
	}

	// The first granted user is the one this endpoint auto-created with the
	// database, so it matches what create returned.
	// İlk yetkilendirilen kullanıcı bu ucun veritabanıyla birlikte otomatik
	// oluşturduğudur; create'in döndürdüğüyle eşleşir.
	rows, err := pool.QueryContext(ctx, `
		SELECT d.id, d.name, dst.name,
		       COALESCE((SELECT u.username
		                 FROM database_user_grants g
		                 JOIN database_users u ON u.id = g.user_id
		                 WHERE g.database_id = d.id
		                 ORDER BY g.id LIMIT 1), ''),
		       d.created_at, COALESCE(ds.version, '')
		FROM databases_v2 d
		JOIN database_servers ds ON ds.id = d.server_id
		JOIN database_server_types dst ON dst.id = ds.type_id
		WHERE d.domain_id = ?
		ORDER BY d.created_at DESC, d.id DESC
	`, domainID)
	if err != nil {
		http.Error(w, "Failed to load databases", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	databases := make([]DatabaseInfo, 0)
	for rows.Next() {
		var db DatabaseInfo
		var serverType, version string
		if err := rows.Scan(&db.ID, &db.Name, &serverType, &db.User, &db.CreatedAt, &version); err != nil {
			http.Error(w, "Failed to load databases", http.StatusInternalServerError)
			return
		}
		db.Type = apiTypeNameFor(serverType)
		db.Engine, db.EngineVersion = databaseEngineLabel(serverType, version)
		databases = append(databases, db)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Failed to load databases", http.StatusInternalServerError)
		return
	}

	users, err := p.domainDatabaseUsers(ctx, domainID)
	if err != nil {
		http.Error(w, "Failed to load databases", http.StatusInternalServerError)
		return
	}
	ids := make([]int, 0, len(databases))
	for _, db := range databases {
		ids = append(ids, db.ID)
	}
	// alpha.133 round 5: what deleting each database leaves of its users.
	// A failure to tell is "unknown" per user; the list still loads.
	fates := p.databaseUserFates(ctx, ids)
	for i := range databases {
		databases[i].Users = users[databases[i].ID]
		if databases[i].Users == nil {
			databases[i].Users = []domainDatabaseUser{}
		}
		for j := range databases[i].Users {
			databases[i].Users[j].Stays = databaseUserStays(fates, databases[i].ID, databases[i].Users[j].ID)
		}
	}

	response := map[string]interface{}{
		"domain_id":       domain.ID,
		"domain_name":     domain.Name,
		"databases":       databases,
		"available_types": availableTypes,
	}

	json.NewEncoder(w).Encode(response)
}

func (p *Panel) handleCreateDatabase(w http.ResponseWriter, r *http.Request, domainID int) {
	var req CreateDatabaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate input
	if req.Name == "" || req.Type == "" || req.Password == "" {
		http.Error(w, "Name, type, and password are required", http.StatusBadRequest)
		return
	}

	if req.Type != "mysql" && req.Type != "postgresql" {
		http.Error(w, "Type must be 'mysql' or 'postgresql'", http.StatusBadRequest)
		return
	}
	// The database's user gets this password: the database user rule.
	if refusal := checkDatabaseUserPassword(req.Password); refusal != nil {
		writePasswordRefusal(w, refusal)
		return
	}

	ctx := r.Context()
	pool := p.db.GetDB()

	// Get domain info
	domainRepo := repositories.NewPostgresDomainRepository(pool)
	domain, err := domainRepo.GetByID(ctx, domainID)
	if err != nil {
		http.Error(w, "Domain not found", http.StatusNotFound)
		return
	}

	// alpha.117 (D-042): the engine must be open to the subscription (the
	// databases permission was checked by the domain dispatcher).
	if !p.requireSubscriptionOffering(w, r, domain.SubscriptionID, databaseEngineOffering(req.Type), req.Type, true) {
		return
	}

	// Generate database name (prefix with domain for organization)
	dbName := fmt.Sprintf("%s_%s", sanitizeName(domain.Name), req.Name)
	dbUser := dbName + "_user"

	// Databases live on the subscription's registered engine of the requested
	// type; auto-registration keeps this working without manual server setup.
	// Veritabanları aboneliğin istenen tipteki kayıtlı motorunda yaşar;
	// otomatik kayıt, elle sunucu eklemeye gerek bırakmaz.
	if err := p.ensureInstalledDBServers(ctx, domain.SubscriptionID); err != nil {
		writeServerError(w, err)
		return
	}

	// alpha.108: a PostgreSQL server record is one cluster on its port; a
	// record whose cluster was emptied and removed (status 'retired') takes
	// no new database. alpha.127: a PostgreSQL database goes to a cluster
	// that runs, never to one whose program was removed or that is stopped
	// (postgresql_placement.go); with none running, nothing runs and the
	// refusal says why.
	var serverID, serverPort int
	if serverTypeNameFor(req.Type) == "postgresql" {
		var refusal *transport.DatabaseClusterRefusal
		serverID, serverPort, refusal, err = p.pgPlaceNewDatabase(ctx, domain.SubscriptionID, p.readPGClustersForPlacement(ctx))
		if refusal != nil {
			writeDatabaseClusterRefusal(w, r, refusal)
			return
		}
	} else {
		err = pool.QueryRowContext(ctx, `
			SELECT ds.id, ds.port
			FROM database_servers ds
			JOIN database_server_types dst ON dst.id = ds.type_id
			WHERE ds.subscription_id = ? AND dst.name = ?
			  AND COALESCE(ds.status, 'active') <> 'retired'
			ORDER BY ds.is_default DESC, ds.id
			LIMIT 1
		`, domain.SubscriptionID, serverTypeNameFor(req.Type)).Scan(&serverID, &serverPort)
	}
	if err == sql.ErrNoRows {
		writeClientError(w, http.StatusServiceUnavailable,
			fmt.Sprintf("no %s server is installed on this host", req.Type))
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}

	// alpha.133 round 5 (review W-C): when this subscription already has a
	// user record of that name, the record is given to the new database below;
	// that step waits at most databaseEngineLinkWait for a deletion on the
	// same engine and holds the gate until the records are written. A new
	// name needs no gate.
	var existingUser int
	if err := pool.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_users WHERE server_id = ? AND username = ?`, serverID, dbUser).Scan(&existingUser); err != nil {
		writeServerError(w, err)
		return
	}
	if existingUser > 0 {
		engineServer, err := repositories.NewPostgresDatabaseServerRepository(pool).GetByID(ctx, serverID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		release, err := acquireDatabaseEngineGate(ctx, engineServer, databaseEngineLinkWait)
		if errors.Is(err, errDatabaseEngineBusy) {
			writeDatabaseEngineBusy(w)
			return
		}
		if err != nil {
			writeServerError(w, err)
			return
		}
		defer release()
	}

	// alpha.129: the database's and its user's names must be free on this
	// engine under every account (database_names.go); this subscription's
	// own user of that name is reused below, as before.
	if what, err := p.databaseNameTakenOnEngine(ctx, serverID, dbName, dbUser, true); err != nil {
		writeServerError(w, err)
		return
	} else if what != "" {
		writeDatabaseNameTaken(w, what, map[string]string{"database": dbName, "user": dbUser}[what])
		return
	}

	// alpha.134 review: on MariaDB a name whose exact form does not fit a
	// right is refused before anything is made.
	if serverTypeNameFor(req.Type) != "postgresql" && !mariadbgrant.ExactFits(dbName) {
		writeDatabaseNameTooLong(w, dbName)
		return
	}

	// The subscription's database limit applies on this path too, with the
	// same refusal as the Databases page (0 = unlimited).
	// Abonelik veritabanı sınırı bu yolda da geçerlidir.
	if err := p.checkSubscriptionQuota(ctx, domain.SubscriptionID, quotaDBs); err != nil {
		writeQuotaRefusal(w, err)
		return
	}

	// Create database via agent RPC
	var agentResp transport.CreateDatabaseResponse

	sealedPassword, err := p.secrets.Encrypt(req.Password)
	if err != nil {
		writeServerError(w, err)
		return
	}

	agentReq := transport.CreateDatabaseRequest{
		Type:     req.Type,
		Name:     dbName,
		User:     dbUser,
		Password: req.Password,
	}
	if serverTypeNameFor(req.Type) == "postgresql" {
		agentReq.Port = serverPort
	}

	err = p.callAgent("Agent.CreateDatabase", agentReq, &agentResp)
	if err == nil && agentResp.Refusal != nil {
		// The Agent found the cluster unable to answer before running
		// anything (it changed since it was read).
		writeDatabaseClusterRefusal(w, r, agentResp.Refusal)
		return
	}
	if err == nil && agentResp.NameTooLong {
		writeDatabaseNameTooLong(w, dbName)
		return
	}
	if err == nil && agentResp.NameTaken != "" {
		// alpha.129: the engine has the name already (made outside
		// CelikPanel); nothing was changed or taken over.
		writeDatabaseNameTaken(w, agentResp.NameTaken, map[string]string{"database": dbName, "user": dbUser}[agentResp.NameTaken])
		return
	}
	if err != nil || !agentResp.Success {
		writeAgentError(w, err, agentResp.Error)
		return
	}

	// Record metadata in one transaction so the three v2 rows stay consistent.
	// Meta veriyi tek işlemde kaydet; üç v2 satırı tutarlı kalsın.
	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		INSERT INTO databases_v2 (server_id, subscription_id, domain_id, name)
		VALUES (?, ?, ?, ?)
	`, serverID, domain.SubscriptionID, domainID, dbName)
	if err != nil {
		http.Error(w, "Failed to store database info", http.StatusInternalServerError)
		return
	}
	dbID, err := res.LastInsertId()
	if err != nil {
		writeServerError(w, err)
		return
	}

	// Reuse the metadata row if this user already exists on the server
	// (UNIQUE(server_id, username)); the agent create is idempotent too.
	// Kullanıcı sunucuda zaten kayıtlıysa meta veri satırını yeniden kullan
	// (UNIQUE(server_id, username)); agent tarafı da idempotenttir.
	var userID int64
	err = tx.QueryRowContext(ctx,
		"SELECT id FROM database_users WHERE server_id = ? AND username = ?",
		serverID, dbUser).Scan(&userID)
	if err == sql.ErrNoRows {
		res, err = tx.ExecContext(ctx, `
			INSERT INTO database_users (server_id, subscription_id, username, password)
			VALUES (?, ?, ?, ?)
		`, serverID, domain.SubscriptionID, dbUser, sealedPassword)
		if err == nil {
			userID, err = res.LastInsertId()
		}
	}
	if err != nil {
		http.Error(w, "Failed to store database user", http.StatusInternalServerError)
		return
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO database_user_grants (database_id, user_id, privileges)
		VALUES (?, ?, 'ALL')
	`, dbID, userID)
	if err != nil {
		http.Error(w, "Failed to store database grant", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		writeServerError(w, err)
		return
	}
	// alpha.128 (G5): the user created here gets its subscription's
	// connection limit too, on its own server (for PostgreSQL its own
	// running cluster), through the same path as a changed limit. No limit
	// writes nothing.
	if limit, err := p.dbConnectionLimit(ctx, domain.SubscriptionID); err != nil {
		log.Printf("database connection limit of %s: %v", dbUser, err)
	} else if limit > 0 {
		p.reapplyDBConnectionLimits([]int{domain.SubscriptionID})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"id":     dbID,
		"name":   dbName,
		"user":   dbUser,
		"type":   req.Type,
	})
}

// handleDeleteDatabase handles DELETE for a specific database
func (p *Panel) handleDeleteDatabase(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != "DELETE" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// alpha.127: the progress window follows these steps (operation_steps.go).
	reportOperationPlan(r.Context(), "check", "delete", "records")
	reportOperationStep(r.Context(), "check")
	// Extract domain ID and database ID from URL
	// /api/v1/domains/:domain_id/databases/:db_id
	domainID, err := extractDomainID(r.URL.Path, "/api/v1/domains/", "/databases/")
	if err != nil {
		http.Error(w, "Invalid domain ID", http.StatusBadRequest)
		return
	}

	dbID, err := getIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid database ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	pool := p.db.GetDB()

	// Scope by domain so a database can only be deleted through its own
	// domain (the dispatcher already verified the caller owns that domain).
	// Domain'e göre süz; bir veritabanı yalnızca kendi domain'i üzerinden
	// silinebilsin (yönlendirici, çağıranın o domain'in sahibi olduğunu
	// zaten doğruladı).
	var dbName string
	err = pool.QueryRowContext(ctx, `
		SELECT d.name FROM databases_v2 d WHERE d.id = ? AND d.domain_id = ?
	`, dbID, domainID).Scan(&dbName)
	if err != nil {
		http.Error(w, "Database not found", http.StatusNotFound)
		return
	}

	// alpha.133 round 5: one deletion for both screens (database_delete.go).
	// It used to drop the first user on the engine even when another
	// database still used it, and to leave every other user behind.
	// İki ekran için tek silme: başka veritabanının kullandığı kullanıcı
	// kalır, yalnız bu veritabanının kullanıcıları gider.
	deleted, err := p.deleteDatabaseRecord(ctx, dbID)
	if err != nil {
		p.writeDatabaseDeleteError(w, r, dbName, err)
		return
	}
	// alpha.133 round 5: recorded like the Databases page's delete.
	p.auditDatabaseDeletion(r, deleted, "deleted")
	json.NewEncoder(w).Encode(databaseDeletionAnswerFor(r, "success", deleted))
}

// sanitizeName turns a domain into the leading fragment of a generated
// database name. Dropping and folding characters cannot fix the first one, so
// the finished fragment goes through the one place that decides that (R-051):
// a domain such as 1and1.com or 360.com is legal, and without this the name it
// produced began with a digit and the agent's validator refused it, so no
// database could be created for such a domain at all.
//
// sanitizeName bir domain'i üretilmiş veritabanı adının baştaki parçasına
// çevirir. Karakter atmak ilk karakteri düzeltemez; bu yüzden tamamlanmış
// parça, bu kararın verildiği tek yerden geçer (R-051). 1and1.com ya da
// 360.com meşru domain'lerdir ve bu olmadan ürettikleri ad rakamla başlıyor,
// agent'ın doğrulayıcısı reddediyordu.
func sanitizeName(name string) string {
	// Remove dots and hyphens, replace with underscores
	result := ""
	for _, char := range name {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			result += string(char)
		} else if char == '.' || char == '-' {
			result += "_"
		}
	}
	return services.EnsureIdentifierLeader(result)
}
