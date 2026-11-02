package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"unicode"

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

const errCodeDatabasePasswordInvalid = "DATABASE_PASSWORD_INVALID"

func validDatabaseUserPassword(password string) bool {
	if len(password) < transport.MinDatabaseUserPasswordBytes || len(password) > transport.MaxDatabaseUserPasswordBytes {
		return false
	}
	for _, r := range password {
		if r == unicode.ReplacementChar || unicode.IsControl(r) {
			return false
		}
	}
	return true
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
	var request struct {
		Password string `json:"password"`
	}
	if err := decodeStrictJSON(w, r, &request); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	minted := request.Password == ""
	if !minted && !validDatabaseUserPassword(request.Password) {
		writeCodedError(w, http.StatusBadRequest, errCodeDatabasePasswordInvalid,
			"the password must be 12 to 128 characters of printable text on one line; nothing was changed", "")
		return
	}
	user, err := repositories.NewPostgresDatabaseUserRepository(p.db.GetDB()).GetByID(ctx, userID)
	if err != nil {
		writeClientError(w, http.StatusNotFound, "database user not found")
		return
	}
	if err := p.canAccessDBServer(ctx, currentCaller(r), user.ServerID); err != nil {
		writeClientError(w, http.StatusNotFound, "database user not found")
		return
	}
	server, err := repositories.NewPostgresDatabaseServerRepository(p.db.GetDB()).GetByID(ctx, user.ServerID)
	if err != nil {
		writeClientError(w, http.StatusNotFound, "database user not found")
		return
	}
	if p.secrets == nil {
		writeServerError(w, errors.New("secret storage is unavailable"))
		return
	}
	password := request.Password
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
	if localDatabaseHost(server.Host) {
		engine, port := "mysql", 0
		if dbDriverTypeFor(server) == "postgresql" {
			engine, port = "postgresql", server.Port
		}
		req := transport.RotateDatabaseUserPasswordRequest{
			User:        transport.SuspensionDatabaseUser{Engine: engine, Port: port, Username: user.Username},
			NewPassword: password,
		}
		var appDomainID int
		var appDomain, appDB string
		_ = p.db.GetDB().QueryRowContext(ctx, `
			SELECT a.domain_id, d.name, a.database_name FROM application_install_operations a JOIN domains d ON d.id = a.domain_id
			WHERE a.status = 'applied' AND a.database_server_id = ? AND a.database_user = ? LIMIT 1`,
			user.ServerID, user.Username).Scan(&appDomainID, &appDomain, &appDB)
		if appDomainID > 0 {
			req.App = &transport.DatabaseUserApp{Kind: "wordpress", SubscriptionID: user.SubscriptionID, DomainID: appDomainID,
				SiteUsername: services.SiteUsername(appDomain), DBName: appDB}
		}
		resp, err := p.rotationNativeOps().rotateDatabaseUser(ctx, req)
		if err != nil {
			release()
			writeServerError(w, errors.New("the Agent did not confirm the database password change; reload to see the current state"))
			return
		}
		if resp.PasswordChanged {
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
		if err := driver.ChangePassword(user.Username, password); err != nil {
			release()
			writeDatabaseEngineError(w, server, err)
			return
		}
		if _, err := p.db.GetDB().ExecContext(ctx, `UPDATE database_users SET password = ?, updated_at = datetime('now') WHERE id = ?`, sealed, user.ID); err != nil {
			writeServerError(w, err)
			return
		}
		answer["sessions_ended"] = false
	}
	p.auditDetailed(r, "database.user.password:"+user.Username, "database_user", user.ID,
		map[string]any{"made_up": minted, "app_updated": answer["app_updated"] == true})
	if minted {
		// Shown once; the request identity keeps no copy (secret route).
		doNotRetainRequestAnswer(ctx)
		answer["password"] = password
	}
	_ = json.NewEncoder(w).Encode(answer)
}
