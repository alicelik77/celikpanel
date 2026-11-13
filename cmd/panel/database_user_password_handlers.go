package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/passwordpolicy"
	"github.com/alicelik/celikpanel/internal/repositories"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// POST /api/v1/database-users/{id}/password {password?}
//
// A database user gets a new password: the one sent, or one CelikPanel makes
// up and shows once in this answer. On this server's engine the Agent sets
// it, ends the user's open sessions and, for a WordPress CelikPanel
// installed, writes it into wp-config.php when that file still names this
// database and user. On another server the panel's own engine account sets
// it. This is how a new owner sets the passwords a transfer replaced
// (transfer_rotation.go). The password is stored sealed; it is never logged.
//
// Veritabanı kullanıcısına yeni parola: gönderilen ya da bir kez gösterilen.

// DATABASE_PASSWORD_INVALID now means only that the engine side refused the
// change; a password that breaks the rule is PASSWORD_WEAK (6 Nov 2026).
// DATABASE_PASSWORD_INVALID artik yalniz motor tarafinin reddidir.
const errCodeDatabasePasswordInvalid = "DATABASE_PASSWORD_INVALID"

// checkDatabaseUserPassword applies the owner's password rule to a database
// user's new password: 12 to 128 bytes (transport's wire bounds, which the
// Agent repeats), an uppercase and a lowercase letter and a special character.
// checkDatabaseUserPassword, veritabani kullanicisinin yeni parolasina kurali
// uygular.
func checkDatabaseUserPassword(password string) *passwordpolicy.Error {
	return passwordpolicy.CheckField(password, "database_user")
}

// requestIdentityDatabaseUserPasswordDigest leaves the password out of the
// stored request digest.
func requestIdentityDatabaseUserPasswordDigest([]byte) []byte {
	return []byte("database-user-password")
}

func (p *Panel) handleSetDatabaseUserPassword(w http.ResponseWriter, r *http.Request, userID int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	ctx := r.Context()
	reportOperationPlan(ctx, "check", "password", "records")
	reportOperationStep(ctx, "check")
	password, ok := readDatabaseUserPasswordRequest(w, r)
	if !ok {
		return
	}
	user, err := repositories.NewPostgresDatabaseUserRepository(p.db.GetDB()).GetByID(ctx, userID)
	if err != nil {
		writeClientError(w, http.StatusNotFound, "database user not found")
		return
	}
	// alpha.133: authorized by the visibility of the user's owner, never by
	// the caller's first subscription; missing and foreign answer the same.
	if _, err := p.visibleDatabaseRecordOwner(ctx, currentCaller(r), user.SubscriptionID, user.ServerID); errors.Is(err, errNotFound) {
		writeClientError(w, http.StatusNotFound, "database user not found")
		return
	} else if err != nil {
		writeServerError(w, err)
		return
	}
	p.applyDatabaseUserPassword(w, r, user, password)
}

// readDatabaseUserPasswordRequest reads {password?} and applies the rule to
// a sent password before anything is looked up; "" asks CelikPanel to make
// one up.
func readDatabaseUserPasswordRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	var request struct {
		Password string `json:"password"`
	}
	if err := decodeStrictJSON(w, r, &request); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return "", false
	}
	if request.Password != "" {
		if refusal := checkDatabaseUserPassword(request.Password); refusal != nil {
			writePasswordRefusal(w, refusal)
			return "", false
		}
	}
	return request.Password, true
}

// applyDatabaseUserPassword is the change itself, for a password the route
// has read and checked (readDatabaseUserPasswordRequest) and a user it has
// resolved and authorized: the account page's
// /api/v1/database-users/{id}/password and, alpha.129, the domain's
// /api/v1/domains/{id}/databases/{db}/users/{user}/password (the domain
// dispatcher decided ownership, team capability, suspension and the plan's
// databases permission; the user holds a grant on that domain's database).
// Steps: check, password (the engine; for a known application also its
// configuration file), records.
//
// applyDatabaseUserPassword: rota kullanıcıyı çözüp yetkilendirdikten sonra
// değişikliğin kendisi.
func (p *Panel) applyDatabaseUserPassword(w http.ResponseWriter, r *http.Request, user *core.DatabaseUser, sent string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	ctx := r.Context()
	minted := sent == ""
	server, err := repositories.NewPostgresDatabaseServerRepository(p.db.GetDB()).GetByID(ctx, user.ServerID)
	if err != nil {
		writeClientError(w, http.StatusNotFound, "database user not found")
		return
	}
	if p.secrets == nil {
		writeServerError(w, errors.New("secret storage is unavailable"))
		return
	}
	password := sent
	if minted {
		if password, err = newRotationSecret(); err != nil {
			writeServerError(w, err)
			return
		}
	}
	sealed, err := p.secrets.Encrypt(password)
	if err != nil {
		writeServerError(w, err)
		return
	}

	// A transfer's open step for this user is the new owner's now.
	target := strconv.Itoa(user.ServerID) + ":" + user.Username
	claimed, err := p.claimReplacedCredential(ctx, rotationEffectDatabaseUser, target)
	if errors.Is(err, errRotationStepRunning) {
		writeCodedError(w, http.StatusConflict, errCodeTransferRotationRunning,
			"the transfer's change of this database user's password is running right now; nothing was changed. Try again in a minute.", "")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	release := func() {
		if claimed {
			p.releaseReplacedCredential(ctx, rotationEffectDatabaseUser, target)
		}
	}

	answer := map[string]any{"success": true}
	// alpha.133 round 5: the Agent's door only for CelikPanel's own record of
	// the machine's engine (databaseRecordIsOurLocalEngine).
	if databaseRecordIsOurLocalEngine(server) {
		engine, port := "mysql", 0
		if dbDriverTypeFor(server) == "postgresql" {
			engine, port = "postgresql", server.Port
		}
		req := transport.RotateDatabaseUserPasswordRequest{
			User:        transport.SuspensionDatabaseUser{Engine: engine, Port: port, Username: user.Username},
			NewPassword: password,
		}
		var appDomainID int
		var appDomain, appDB, appKind string
		// WordPress (028) or a catalog application (alpha.118, 058).
		_ = p.db.GetDB().QueryRowContext(ctx, `
			SELECT a.domain_id, d.name, a.database_name, 'wordpress' FROM application_install_operations a JOIN domains d ON d.id = a.domain_id
			WHERE a.status = 'applied' AND a.database_server_id = ?1 AND a.database_user = ?2
			UNION ALL
			SELECT c.domain_id, d.name, c.database_name, c.app_id FROM app_catalog_operations c JOIN domains d ON d.id = c.domain_id
			WHERE c.status IN ('applied', 'setup_failed') AND c.database_server_id = ?1 AND c.database_user = ?2
			LIMIT 1`,
			user.ServerID, user.Username).Scan(&appDomainID, &appDomain, &appDB, &appKind)
		if appDomainID > 0 {
			req.App = &transport.DatabaseUserApp{Kind: appKind, SubscriptionID: user.SubscriptionID, DomainID: appDomainID,
				SiteUsername: services.SiteUsername(appDomain), DBName: appDB}
		}
		reportOperationStep(ctx, "password")
		resp, err := p.rotationNativeOps().rotateDatabaseUser(ctx, req)
		if err != nil {
			release()
			writeServerError(w, errors.New("the Agent did not confirm the database password change; reload to see the current state"))
			return
		}
		if resp.PasswordChanged {
			reportOperationStep(ctx, "records")
			if _, err := p.db.GetDB().ExecContext(ctx, `UPDATE database_users SET password = ?, updated_at = datetime('now') WHERE id = ?`, sealed, user.ID); err != nil {
				writeServerError(w, err)
				return
			}
		}
		if !resp.Applied {
			if !resp.PasswordChanged {
				release()
			}
			writeCodedError(w, http.StatusConflict, errCodeDatabasePasswordInvalid, resp.Error, "")
			return
		}
		answer["sessions_ended"] = resp.SessionsEnded
		if appDomainID > 0 {
			answer["app_domain"] = appDomain
			answer["app_kind"] = appKind
			answer["app_updated"] = resp.AppUpdated
			if !resp.AppUpdated {
				answer["app_detail"] = resp.AppDetail
			}
		}
	} else {
		driver, err := p.dbDriverFor(server)
		if err != nil {
			release()
			writeServerError(w, err)
			return
		}
		reportOperationStep(ctx, "password")
		if err := driver.ChangePassword(user.Username, password); err != nil {
			release()
			writeDatabaseEngineError(w, server, err)
			return
		}
		reportOperationStep(ctx, "records")
		if _, err := p.db.GetDB().ExecContext(ctx, `UPDATE database_users SET password = ?, updated_at = datetime('now') WHERE id = ?`, sealed, user.ID); err != nil {
			writeServerError(w, err)
			return
		}
		answer["sessions_ended"] = false
	}
	details := map[string]any{"made_up": minted, "app_updated": answer["app_updated"] == true}
	// alpha.133: the row names whose user it is as well as who acted.
	if owner, ok := p.databaseSubscriptionOwner(ctx, user.SubscriptionID); ok {
		details["owner"], details["owner_id"] = owner.Username, owner.ID
	}
	p.auditDetailed(r, "database.user.password:"+user.Username, "database_user", user.ID, details)
	if minted {
		// Shown once; the request identity keeps no copy (secret route).
		doNotRetainRequestAnswer(ctx)
		answer["password"] = password
	}
	_ = json.NewEncoder(w).Encode(answer)
}
