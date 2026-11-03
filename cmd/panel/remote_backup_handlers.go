package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/remotebackup"
)

// HTTP surface of remote backups (alpha.118):
//
//	GET    /api/v1/remote-backup/destinations            destinations the caller manages or may choose; whether it may add one
//	POST   /api/v1/remote-backup/destinations            add (runs the connection test first; refuses on failure)
//	POST   /api/v1/remote-backup/destinations/test       connection test: write, read and delete a small object
//	PUT    /api/v1/remote-backup/destinations/{id}       change (blank secrets keep the stored ones; the passphrase never changes)
//	DELETE /api/v1/remote-backup/destinations/{id}       remove from the panel (what is stored there is not touched)
//	GET    /api/v1/remote-backup/policies                administrator: every-site schedules
//	PUT    /api/v1/remote-backup/policies                administrator: set the every-site schedule of a destination
//	DELETE /api/v1/remote-backup/policies?destination_id=
//
//	GET    /api/v1/domains/{id}/backups/remote           destinations, schedules and recent operations of a site
//	PUT    /api/v1/domains/{id}/backups/remote/policy    the site's schedule for a destination
//	DELETE /api/v1/domains/{id}/backups/remote/policy?destination_id=
//	GET    /api/v1/domains/{id}/backups/remote/objects?destination_id=   the site's backups on a destination
//	POST   /api/v1/domains/{id}/backups/remote/send      {backup_name, destination_id}: queue an upload
//	POST   /api/v1/domains/{id}/backups/remote/restore   {destination_id, object_key, confirm}: queue a restore
//	GET    /api/v1/domains/{id}/backups/remote/operation?id=   one operation (read only)
//
// No answer carries a secret or the passphrase.

const remoteBackupPathPrefix = "/api/v1/remote-backup/"

const maxRemoteBackupBody = 64 << 10

// ----- guidance -----

// remoteGuidance follows D-024: the reason, who acts, the next action and how
// the work resumes. Code names the sentence the screen shows in its language;
// the English text is for anything that reads the API directly.
type remoteGuidance struct {
	Result     string `json:"result"` // waiting | in_progress | retrying | done | prerequisite | verified_failure | unknown
	Code       string `json:"code,omitempty"`
	Reason     string `json:"reason"`
	WhoActs    string `json:"who_acts"` // none | panel | destination_owner | administrator | account_owner
	NextAction string `json:"next_action"`
	Resumes    string `json:"resumes"`
}

type remoteOperationView struct {
	ID          string         `json:"id"`
	Kind        string         `json:"kind"`
	Destination int64          `json:"destination_id"`
	BackupName  string         `json:"backup_name,omitempty"`
	BackupType  string         `json:"backup_type,omitempty"`
	ObjectKey   string         `json:"object_key,omitempty"`
	Scheduled   bool           `json:"scheduled"`
	State       string         `json:"state"`
	Phase       string         `json:"phase"`
	BytesDone   int64          `json:"bytes_done"`
	BytesTotal  int64          `json:"bytes_total"`
	Attempts    int            `json:"attempts"`
	NextAttempt string         `json:"next_attempt_at,omitempty"`
	Verified    string         `json:"verified,omitempty"`
	Detail      string         `json:"detail,omitempty"`
	CreatedAt   string         `json:"created_at"`
	FinishedAt  string         `json:"finished_at,omitempty"`
	Guidance    remoteGuidance `json:"guidance"`
}

func remoteFailureWho(code string, serverWide bool) string {
	switch code {
	case "REMOTE_CREDENTIALS", "REMOTE_PERMISSION", "REMOTE_NOT_FOUND", "REMOTE_HOST_KEY", "REMOTE_FULL",
		"REMOTE_SETTINGS", "DECRYPT_FAILED", "DESTINATION_REMOVED":
		if serverWide {
			return "administrator"
		}
		return "destination_owner"
	case "SERVER_CLOCK", "LOCAL_BACKUP_FAILED", "LOCAL_READ_FAILED", "LOCAL_IMPORT_FAILED", "LOCAL_BACKUP_UNREADABLE",
		"PANEL_RESTARTED", "REMOTE_UNREACHABLE", "REMOTE_TIMED_OUT", "REMOTE_INTEGRITY":
		return "administrator"
	case "DISK_QUOTA", "DATABASE_GONE", "LOCAL_BACKUP_MISSING", "DESTINATION_NOT_AVAILABLE", "LEGACY_BACKUP",
		"SUBSCRIPTION_SUSPENDED", "RESTORE_IN_PROGRESS":
		return "account_owner"
	}
	return "administrator"
}

func remoteNextAction(code string) string {
	switch code {
	case "REMOTE_CREDENTIALS":
		return "Enter a valid access key and secret (or SFTP password or key) in the destination, test it, then send again."
	case "REMOTE_PERMISSION":
		return "Allow this key to write, read, list and delete in the bucket or folder at the storage provider, then send again."
	case "REMOTE_NOT_FOUND":
		return "Create the bucket or folder named in the destination, or correct its name, then send again."
	case "REMOTE_HOST_KEY":
		return "Ask the SFTP server's owner whether its key changed. Only if it did, confirm the new fingerprint in the destination."
	case "REMOTE_FULL":
		return "Free space at the storage provider or raise its limit, then send again."
	case "SERVER_CLOCK":
		return "Correct this server's clock (time synchronisation), then send again."
	case "REMOTE_SETTINGS":
		return "Correct the destination's settings and run its connection test."
	case "REMOTE_UNREACHABLE", "REMOTE_TIMED_OUT", "PANEL_RESTARTED", "LOCAL_READ_FAILED", "LOCAL_IMPORT_FAILED", "LOCAL_BACKUP_UNREADABLE":
		return "Nothing to do now: CelikPanel tries again by itself. If the last attempt also fails, check that the storage is reachable from this server, then send again."
	case "REMOTE_INTEGRITY":
		return "CelikPanel sends again by itself. If it keeps failing, test the destination and check the storage provider's status."
	case "DECRYPT_FAILED":
		return "The destination's passphrase does not open this backup: it was made with another passphrase, or the file is damaged. Use the passphrase recorded for it (see its .json side file) with gpg."
	case "DISK_QUOTA":
		return "Free space in the subscription or ask your provider for a larger plan, then restore again."
	case "DATABASE_GONE":
		return "Create the database again for this site, then restore again."
	case "LOCAL_BACKUP_MISSING":
		return "Make a new backup and send that one."
	case "LEGACY_BACKUP":
		return "Make a new backup and send that one."
	case "DESTINATION_NOT_AVAILABLE", "DESTINATION_REMOVED":
		return "Choose another destination for this site."
	case "RESTORE_IN_PROGRESS":
		return "Wait for the other restore to finish, check the site, and restore again only if still needed."
	case "SUBSCRIPTION_SUSPENDED":
		return "Ask the provider to resume the subscription, then restore again."
	}
	return "Read the reason, correct it, then start the operation again."
}

func remoteOperationGuidance(op remoteOperation, serverWide bool) remoteGuidance {
	switch op.State {
	case remoteOpQueued:
		if op.FailureCode != "" {
			return remoteGuidance{Result: "retrying", Code: op.FailureCode,
				Reason: "The last attempt did not finish: " + op.FailureDetail, WhoActs: "panel",
				NextAction: "Nothing to do now; CelikPanel tries again by itself (attempt " + strconv.Itoa(op.Attempts+1) + " of " + strconv.Itoa(remoteAutoAttempts) + ").",
				Resumes:    "The same operation continues; nothing is sent twice under another name."}
		}
		return remoteGuidance{Result: "waiting", Reason: "Waiting for its turn: remote operations run one at a time.",
			WhoActs: "none", NextAction: "Nothing to do; this page follows the operation.", Resumes: "It starts by itself."}
	case remoteOpRunning:
		return remoteGuidance{Result: "in_progress", Reason: "Running: " + op.Phase + ".", WhoActs: "none",
			NextAction: "Nothing to do; this page follows the operation.",
			Resumes:    "If the panel restarts, the same operation is checked and continued."}
	case remoteOpSucceeded:
		reason := "Sent, encrypted, and checked at the destination."
		if op.Kind == "restore" {
			reason = "Downloaded, checked, decrypted and restored. A safety backup of the state before the restore was kept on this server."
		}
		return remoteGuidance{Result: "done", Reason: reason, WhoActs: "none", NextAction: "Nothing to do.", Resumes: ""}
	}
	class := op.FailureClass
	if class == "" {
		class = remoteFailureUnknown
	}
	return remoteGuidance{Result: class, Code: op.FailureCode, Reason: op.FailureDetail,
		WhoActs: remoteFailureWho(op.FailureCode, serverWide), NextAction: remoteNextAction(op.FailureCode),
		Resumes: "Nothing runs again by itself. Start the operation again after the cause is corrected; the next scheduled run also tries again."}
}

func (p *Panel) remoteOperationViewFor(ctx context.Context, op remoteOperation) remoteOperationView {
	serverWide := false
	if d, err := p.loadRemoteDestination(ctx, op.DestinationID); err == nil {
		serverWide = d.serverWide()
	}
	return remoteOperationView{
		ID: op.ID, Kind: op.Kind, Destination: op.DestinationID, BackupName: op.BackupName, BackupType: op.BackupType,
		ObjectKey: op.ObjectKey, Scheduled: op.PolicyID > 0, State: op.State, Phase: op.Phase,
		BytesDone: op.BytesDone, BytesTotal: op.BytesTotal, Attempts: op.Attempts, NextAttempt: op.NextAttemptAt,
		Verified: op.Verified, Detail: op.FailureDetail, CreatedAt: op.CreatedAt, FinishedAt: op.FinishedAt,
		Guidance: remoteOperationGuidance(op, serverWide),
	}
}

// ----- destinations -----

type remoteDestinationBody struct {
	Name               string                  `json:"name"`
	Kind               string                  `json:"kind"`
	Config             remoteDestinationConfig `json:"config"`
	Secret             remoteDestinationSecret `json:"secret"`
	Passphrase         string                  `json:"passphrase,omitempty"`
	HostKeyFingerprint string                  `json:"host_key_fingerprint,omitempty"`
	OfferedToAccounts  bool                    `json:"offered_to_accounts"`
	DestinationID      int64                   `json:"destination_id,omitempty"` // test: an existing destination's stored secrets
}

type remoteTestStep struct {
	Step string `json:"step"` // connect | write | read | delete
	OK   bool   `json:"ok"`
}

type remoteTestResult struct {
	OK       bool                      `json:"ok"`
	Steps    []remoteTestStep          `json:"steps"`
	HostKey  *remotebackup.SFTPHostKey `json:"host_key,omitempty"` // a key to confirm (SFTP)
	Guidance *remoteGuidance           `json:"guidance,omitempty"`
	pinned   string
}

func decodeRemoteJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRemoteBackupBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return errors.New("multiple JSON values")
	}
	return nil
}

func (p *Panel) handleRemoteBackupRoutes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	c := currentCaller(r)
	if callerAccountRole(c) == "" {
		writeClientError(w, http.StatusForbidden, "remote backup destinations are managed by the account's owner")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, remoteBackupPathPrefix)
	switch {
	case rest == "destinations":
		switch r.Method {
		case http.MethodGet:
			p.handleListRemoteDestinations(w, r)
		case http.MethodPost:
			p.handleSaveRemoteDestination(w, r, 0)
		default:
			rejectRouteMethod(w, []string{http.MethodGet, http.MethodPost})
		}
	case rest == "destinations/test":
		if r.Method != http.MethodPost {
			rejectRouteMethod(w, []string{http.MethodPost})
			return
		}
		p.handleTestRemoteDestination(w, r)
	case strings.HasPrefix(rest, "destinations/"):
		id, ok := strictPositiveID(strings.TrimPrefix(rest, "destinations/"))
		if !ok {
			writeClientError(w, http.StatusNotFound, "destination not found")
			return
		}
		switch r.Method {
		case http.MethodPut:
			p.handleSaveRemoteDestination(w, r, int64(id))
		case http.MethodDelete:
			p.handleDeleteRemoteDestination(w, r, int64(id))
		default:
			rejectRouteMethod(w, []string{http.MethodPut, http.MethodDelete})
		}
	case rest == "policies":
		if !c.hasAccountRole(roleAdmin) {
			writeClientError(w, http.StatusForbidden, "only the administrator sets every-site schedules")
			return
		}
		switch r.Method {
		case http.MethodGet:
			p.handleListServerPolicies(w, r)
		case http.MethodPut:
			p.handleSaveRemotePolicy(w, r, nil)
		case http.MethodDelete:
			p.handleDeleteRemotePolicy(w, r, nil)
		default:
			rejectRouteMethod(w, []string{http.MethodGet, http.MethodPut, http.MethodDelete})
		}
	default:
		writeClientError(w, http.StatusNotFound, "not found")
	}
}

func (p *Panel) handleListRemoteDestinations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := currentCaller(r)
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT `+remoteDestinationColumns+`
		FROM remote_backup_destinations WHERE owner_user_id IS NULL OR owner_user_id = ?
		ORDER BY owner_user_id IS NOT NULL, name, id`, c.ID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	views := []remoteDestinationView{}
	for rows.Next() {
		d, err := scanRemoteDestination(rows)
		if err != nil {
			rows.Close()
			writeServerError(w, err)
			return
		}
		if destinationVisible(c, d) {
			views = append(views, remoteDestinationViewFor(c, d, int64(c.ID)))
		}
	}
	rows.Close()
	refusal, err := p.ownDestinationRefusal(ctx, c)
	if err != nil {
		writeServerError(w, err)
		return
	}
	scope := "own"
	if c.hasAccountRole(roleAdmin) {
		scope = "server"
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"destinations": views, "can_add": refusal == "", "add_refused_by": refusal, "add_scope": scope,
		"min_passphrase": remotebackup.MinPassphraseLength,
	})
}

// testDestination runs the connection test: connect (SFTP: the key a person
// confirmed), write a small object, read it back, delete it.
func (p *Panel) testDestination(ctx context.Context, kind string, cfg remoteDestinationConfig, secret remoteDestinationSecret,
	storedHostKey, confirmedFingerprint string) remoteTestResult {
	result := remoteTestResult{Steps: []remoteTestStep{}}
	fail := func(err error) remoteTestResult {
		f := classifyRemoteError(err)
		g := remoteGuidance{Result: f.class, Code: f.code, Reason: f.detail, WhoActs: "destination_owner",
			NextAction: remoteNextAction(f.code), Resumes: "Run the test again after the correction; nothing was saved."}
		result.Guidance = &g
		return result
	}
	hostKey := storedHostKey
	if kind == remoteDestinationKindSFTP {
		presented, err := probeRemoteHostKey(ctx, cfg.Host, cfg.Port)
		if err != nil {
			result.Steps = append(result.Steps, remoteTestStep{Step: "connect"})
			return fail(err)
		}
		storedFingerprint := remotebackup.HostKeyFingerprint(storedHostKey)
		switch {
		case confirmedFingerprint != "" && confirmedFingerprint == presented.Fingerprint:
			hostKey = presented.Key
		case storedHostKey != "" && storedFingerprint == presented.Fingerprint:
			hostKey = storedHostKey
		default:
			// Not confirmed yet, or the server now shows another key: a person
			// compares the fingerprint with the server's own before anything
			// is sent.
			result.HostKey = &presented
			code := "REMOTE_HOST_KEY_CONFIRM"
			reason := "Confirm the SFTP server's key fingerprint before the first connection."
			if storedHostKey != "" {
				code = "REMOTE_HOST_KEY"
				reason = "The SFTP server shows a different key than the one confirmed before. Nothing was sent."
			}
			g := remoteGuidance{Result: remoteFailurePrerequisite, Code: code, Reason: reason, WhoActs: "destination_owner",
				NextAction: "Compare the fingerprint with the one the server's owner gives you (ssh-keygen -lf on the server); confirm it only if they match.",
				Resumes:    "After you confirm, the test runs again with that key pinned."}
			result.Guidance = &g
			return result
		}
	}
	result.pinned = hostKey
	store, err := openRemoteBackupStore(ctx, kind, cfg, secret, hostKey)
	if err != nil {
		result.Steps = append(result.Steps, remoteTestStep{Step: "connect"})
		return fail(err)
	}
	defer store.Close()
	result.Steps = append(result.Steps, remoteTestStep{Step: "connect", OK: true})
	random := make([]byte, 8)
	_, _ = rand.Read(random)
	payload := make([]byte, 1024)
	_, _ = rand.Read(payload)
	key := "celikpanel-connection-test-" + hex.EncodeToString(random) + ".bin"
	if _, err := store.Put(ctx, key, bytes.NewReader(payload)); err != nil {
		result.Steps = append(result.Steps, remoteTestStep{Step: "write"})
		return fail(err)
	}
	result.Steps = append(result.Steps, remoteTestStep{Step: "write", OK: true})
	rc, err := store.Get(ctx, key)
	if err == nil {
		var got []byte
		got, err = io.ReadAll(io.LimitReader(rc, 4096))
		rc.Close()
		if err == nil && !bytes.Equal(got, payload) {
			err = &remotebackup.Error{Kind: remotebackup.KindIntegrity, Op: "read", Message: "the object read back differs from what was written"}
		}
	}
	if err != nil {
		result.Steps = append(result.Steps, remoteTestStep{Step: "read"})
		_ = store.Delete(ctx, key)
		return fail(err)
	}
	result.Steps = append(result.Steps, remoteTestStep{Step: "read", OK: true})
	if err := store.Delete(ctx, key); err != nil {
		result.Steps = append(result.Steps, remoteTestStep{Step: "delete"})
		return fail(err)
	}
	result.Steps = append(result.Steps, remoteTestStep{Step: "delete", OK: true})
	result.OK = true
	return result
}

// resolveDestinationBody validates a body and fills blank secrets from the
// stored destination (an update or a test of a saved one).
func (p *Panel) resolveDestinationBody(body *remoteDestinationBody, stored *remoteDestination) (remoteDestinationSecret, string, error) {
	body.Name = strings.TrimSpace(body.Name)
	if stored != nil {
		body.Kind = stored.Kind
		if body.Config.AccessKeyID == "" || strings.Contains(body.Config.AccessKeyID, "…") {
			body.Config.AccessKeyID = stored.Config.AccessKeyID
		}
	}
	cfg, err := normalizeRemoteConfig(body.Kind, body.Config)
	if err != nil {
		return remoteDestinationSecret{}, "", err
	}
	body.Config = cfg
	secret := body.Secret
	hostKey := ""
	if stored != nil {
		old, _, err := p.remoteDestinationSecrets(*stored)
		if err != nil {
			return remoteDestinationSecret{}, "", err
		}
		if secret.SecretAccessKey == "" {
			secret.SecretAccessKey = old.SecretAccessKey
		}
		if secret.Password == "" && secret.PrivateKey == "" {
			secret.Password, secret.PrivateKey = old.Password, old.PrivateKey
		}
		// A stored host key stays pinned only for the same server.
		if stored.Config.Host == cfg.Host && stored.Config.Port == cfg.Port {
			hostKey = stored.HostKey
		}
	}
	switch body.Kind {
	case remoteDestinationKindS3:
		secret = remoteDestinationSecret{SecretAccessKey: secret.SecretAccessKey}
		if secret.SecretAccessKey == "" {
			return secret, "", errors.New("the secret access key is required")
		}
	case remoteDestinationKindSFTP:
		secret = remoteDestinationSecret{Password: secret.Password, PrivateKey: strings.TrimSpace(secret.PrivateKey)}
		if secret.Password == "" && secret.PrivateKey == "" {
			return secret, "", errors.New("a password or a private key is required")
		}
	}
	return secret, hostKey, nil
}

func (p *Panel) handleTestRemoteDestination(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := currentCaller(r)
	var body remoteDestinationBody
	if err := decodeRemoteJSON(w, r, &body); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var stored *remoteDestination
	if body.DestinationID > 0 {
		d, err := p.loadRemoteDestination(ctx, body.DestinationID)
		if err != nil || !destinationManageable(c, d) {
			writeClientError(w, http.StatusNotFound, "destination not found")
			return
		}
		stored = &d
	} else if refusal, err := p.ownDestinationRefusal(ctx, c); err != nil {
		writeServerError(w, err)
		return
	} else if refusal != "" {
		writeRemoteRightRefusal(w, refusal)
		return
	}
	secret, hostKey, err := p.resolveDestinationBody(&body, stored)
	if err != nil {
		writeCodedError(w, http.StatusBadRequest, "REMOTE_SETTINGS", err.Error(), "")
		return
	}
	testCtx, cancel := context.WithTimeout(ctx, remoteBackupTestTimeout)
	defer cancel()
	result := p.testDestination(testCtx, body.Kind, body.Config, secret, hostKey, body.HostKeyFingerprint)
	if stored != nil {
		outcome := "ok"
		if !result.OK {
			outcome = "failed"
			if result.Guidance != nil {
				outcome = result.Guidance.Code
			}
		}
		_, _ = p.db.GetDB().ExecContext(ctx, `UPDATE remote_backup_destinations SET last_test_at = datetime('now'), last_test_result = ? WHERE id = ?`, outcome, stored.ID)
	}
	_ = json.NewEncoder(w).Encode(result)
}

func writeRemoteRightRefusal(w http.ResponseWriter, layer string) {
	if layer == "role" {
		writeClientError(w, http.StatusForbidden, "remote backup destinations are managed by the account's owner")
		return
	}
	who := "the administrator"
	why := "the administrator does not offer it on this server"
	switch layer {
	case rightsLayerReseller:
		why = "it is not in the reseller's rights"
	case rightsLayerSubscription:
		why = "it is not in this account's rights"
		who = "your provider"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(apiErrorBody{
		Error: "Nothing was changed: own remote backup destinations are not available here because " + why + ". " +
			"Ask " + who + " to include \"Own remote backup destinations\", or use a destination the administrator offers.",
		Code: errCodeRightNotGranted, Reason: layer, Action: myPlanPagePath,
		Vars: map[string]string{"right": rightRemoteBackupDestinations, "name": rightPermissionNames[rightRemoteBackupDestinations], "grantor": who},
	})
}

func (p *Panel) handleSaveRemoteDestination(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	c := currentCaller(r)
	var body remoteDestinationBody
	if err := decodeRemoteJSON(w, r, &body); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var stored *remoteDestination
	if id > 0 {
		d, err := p.loadRemoteDestination(ctx, id)
		if err != nil || !destinationManageable(c, d) {
			writeClientError(w, http.StatusNotFound, "destination not found")
			return
		}
		stored = &d
		if body.Passphrase != "" {
			writeCodedError(w, http.StatusBadRequest, "REMOTE_PASSPHRASE_FIXED",
				"The passphrase of a destination cannot be changed: the backups already there need it. Add a new destination for a new passphrase.", "")
			return
		}
	} else {
		refusal, err := p.ownDestinationRefusal(ctx, c)
		if err != nil {
			writeServerError(w, err)
			return
		}
		if refusal != "" {
			writeRemoteRightRefusal(w, refusal)
			return
		}
		if err := remotebackup.ValidPassphrase(body.Passphrase); err != nil {
			writeCodedError(w, http.StatusBadRequest, "REMOTE_PASSPHRASE", err.Error(), "")
			return
		}
	}
	if body.Name == "" && stored != nil {
		body.Name = stored.Name
	}
	if l := len(strings.TrimSpace(body.Name)); l < 1 || l > 100 {
		writeCodedError(w, http.StatusBadRequest, "REMOTE_SETTINGS", "the name must be 1 to 100 characters", "")
		return
	}
	secret, hostKey, err := p.resolveDestinationBody(&body, stored)
	if err != nil {
		writeCodedError(w, http.StatusBadRequest, "REMOTE_SETTINGS", err.Error(), "")
		return
	}
	if p.secrets == nil {
		writeServerError(w, errors.New("the panel's secret key is not available"))
		return
	}
	testCtx, cancel := context.WithTimeout(ctx, remoteBackupTestTimeout)
	defer cancel()
	result := p.testDestination(testCtx, body.Kind, body.Config, secret, hostKey, body.HostKeyFingerprint)
	if !result.OK {
		// Nothing is saved until the destination works (write, read, delete).
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"saved": false, "test": result})
		return
	}
	secretJSON, _ := json.Marshal(secret)
	sealedSecret, err := p.secrets.Encrypt(string(secretJSON))
	if err != nil {
		writeServerError(w, err)
		return
	}
	configJSON, _ := json.Marshal(body.Config)
	offered := 0
	if body.OfferedToAccounts && c.hasAccountRole(roleAdmin) {
		offered = 1
	}
	db := p.db.GetDB()
	if stored == nil {
		salt := make([]byte, 8)
		_, _ = rand.Read(salt)
		check := remotebackup.PassphraseCheck(hex.EncodeToString(salt), body.Passphrase)
		sealedPassphrase, err := p.secrets.Encrypt(body.Passphrase)
		if err != nil {
			writeServerError(w, err)
			return
		}
		var owner any
		if !c.hasAccountRole(roleAdmin) {
			owner = c.ID
		}
		res, err := db.ExecContext(ctx, `
			INSERT INTO remote_backup_destinations
			    (owner_user_id, name, kind, config_json, secret_sealed, passphrase_sealed, passphrase_check, host_key,
			     offered_to_accounts, last_test_at, last_test_result, created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), 'ok', ?)`,
			owner, body.Name, body.Kind, string(configJSON), sealedSecret, sealedPassphrase, check, result.pinned, offered, c.ID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		id, _ = res.LastInsertId()
		p.audit(r, "backup.remote.destination.add:"+body.Kind, "remote_backup_destination", int(id))
	} else {
		if !c.hasAccountRole(roleAdmin) {
			offered = 0
		}
		if _, err := db.ExecContext(ctx, `
			UPDATE remote_backup_destinations
			SET name = ?, config_json = ?, secret_sealed = ?, host_key = ?, offered_to_accounts = ?,
			    last_test_at = datetime('now'), last_test_result = 'ok', updated_at = datetime('now')
			WHERE id = ?`, body.Name, string(configJSON), sealedSecret, result.pinned, offered, id); err != nil {
			writeServerError(w, err)
			return
		}
		p.audit(r, "backup.remote.destination.change", "remote_backup_destination", int(id))
	}
	d, err := p.loadRemoteDestination(ctx, id)
	if err != nil {
		writeServerError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"saved": true, "test": result, "destination": remoteDestinationViewFor(c, d, int64(c.ID))})
}

func (p *Panel) handleDeleteRemoteDestination(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	c := currentCaller(r)
	d, err := p.loadRemoteDestination(ctx, id)
	if err != nil || !destinationManageable(c, d) {
		writeClientError(w, http.StatusNotFound, "destination not found")
		return
	}
	var active int
	_ = p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM remote_backup_operations WHERE destination_id = ? AND state IN ('queued','running')`, id).Scan(&active)
	if active > 0 {
		writeCodedError(w, http.StatusConflict, "REMOTE_DESTINATION_BUSY",
			"Nothing was changed: an upload or restore with this destination is still waiting or running. Remove it after that finishes.", "")
		return
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM remote_backup_destinations WHERE id = ?`, id); err != nil {
		writeServerError(w, err)
		return
	}
	p.audit(r, "backup.remote.destination.remove", "remote_backup_destination", int(id))
	_ = json.NewEncoder(w).Encode(map[string]any{"removed": true,
		"note": "The destination and its schedules were removed from CelikPanel. The backups stored there were not touched."})
}

// ----- schedules -----

type remotePolicyBody struct {
	DestinationID int64  `json:"destination_id"`
	Frequency     string `json:"frequency"`
	BackupType    string `json:"backup_type"`
	KeepDaily     int    `json:"keep_daily"`
	KeepWeekly    int    `json:"keep_weekly"`
	Enabled       *bool  `json:"enabled,omitempty"`
}

type remotePolicyView struct {
	ID            int64  `json:"id"`
	DestinationID int64  `json:"destination_id"`
	Destination   string `json:"destination_name"`
	EverySite     bool   `json:"every_site"`
	Frequency     string `json:"frequency"`
	BackupType    string `json:"backup_type"`
	KeepDaily     int    `json:"keep_daily"`
	KeepWeekly    int    `json:"keep_weekly"`
	Enabled       bool   `json:"enabled"`
	Editable      bool   `json:"editable"`
}

func validRemotePolicy(b remotePolicyBody) string {
	switch {
	case b.Frequency != "daily" && b.Frequency != "weekly":
		return "frequency must be daily or weekly"
	case b.BackupType != backupspec.TypeFiles && b.BackupType != backupspec.TypeFull:
		return "backup_type must be files or full"
	case b.KeepDaily < 0 || b.KeepDaily > remotebackup.MaxKeepDaily:
		return "keep_daily must be between 0 and 90"
	case b.KeepWeekly < 0 || b.KeepWeekly > remotebackup.MaxKeepWeekly:
		return "keep_weekly must be between 0 and 104"
	case b.KeepDaily+b.KeepWeekly == 0:
		return "keep at least one daily or weekly copy"
	}
	return ""
}

// handleSaveRemotePolicy sets a schedule: site != nil for one site, nil for
// the administrator's every-site schedule.
func (p *Panel) handleSaveRemotePolicy(w http.ResponseWriter, r *http.Request, site *remoteDomain) {
	ctx := r.Context()
	c := currentCaller(r)
	var body remotePolicyBody
	if err := decodeRemoteJSON(w, r, &body); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := validRemotePolicy(body); msg != "" {
		writeCodedError(w, http.StatusBadRequest, "REMOTE_SCHEDULE", msg, "")
		return
	}
	d, err := p.loadRemoteDestination(ctx, body.DestinationID)
	if err != nil {
		writeClientError(w, http.StatusNotFound, "destination not found")
		return
	}
	enabled := 1
	if body.Enabled != nil && !*body.Enabled {
		enabled = 0
	}
	db := p.db.GetDB()
	var res sql.Result
	if site == nil {
		if !d.serverWide() {
			writeClientError(w, http.StatusBadRequest, "an every-site schedule needs a server-wide destination")
			return
		}
		res, err = db.ExecContext(ctx, `
			INSERT INTO remote_backup_policies (destination_id, domain_id, frequency, backup_type, keep_daily, keep_weekly, enabled, created_by)
			VALUES (?, NULL, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (destination_id) WHERE domain_id IS NULL DO UPDATE SET
			    frequency = excluded.frequency, backup_type = excluded.backup_type, keep_daily = excluded.keep_daily,
			    keep_weekly = excluded.keep_weekly, enabled = excluded.enabled, updated_at = datetime('now')`,
			d.ID, body.Frequency, body.BackupType, body.KeepDaily, body.KeepWeekly, enabled, c.ID)
	} else {
		if p.policyHeldByAdministrator(ctx, c, d.ID, site.ID) {
			writeRemotePolicyHeld(w)
			return
		}
		refusal, err := p.destinationChoiceRefusal(ctx, c, d, *site)
		if err != nil {
			writeServerError(w, err)
			return
		}
		if refusal != "" {
			writeRemoteChoiceRefusal(w, refusal)
			return
		}
		res, err = db.ExecContext(ctx, `
			INSERT INTO remote_backup_policies (destination_id, domain_id, frequency, backup_type, keep_daily, keep_weekly, enabled, created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (destination_id, domain_id) WHERE domain_id IS NOT NULL DO UPDATE SET
			    frequency = excluded.frequency, backup_type = excluded.backup_type, keep_daily = excluded.keep_daily,
			    keep_weekly = excluded.keep_weekly, enabled = excluded.enabled, created_by = excluded.created_by,
			    updated_at = datetime('now')`,
			d.ID, site.ID, body.Frequency, body.BackupType, body.KeepDaily, body.KeepWeekly, enabled, c.ID)
		if err != nil {
			writeServerError(w, err)
			return
		}
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	_ = res
	resource := 0
	if site != nil {
		resource = site.ID
	}
	p.audit(r, "backup.remote.schedule:"+body.Frequency, "domain", resource)
	wakeRemoteBackupWorker()
	_ = json.NewEncoder(w).Encode(map[string]any{"saved": true})
}

// policyHeldByAdministrator: a site's schedule the administrator set is
// changed only by the administrator.
func (p *Panel) policyHeldByAdministrator(ctx context.Context, c *Caller, destinationID int64, domainID int) bool {
	if c.hasAccountRole(roleAdmin) {
		return false
	}
	var createdBy int64
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT COALESCE(created_by, 0) FROM remote_backup_policies
		WHERE destination_id = ? AND domain_id = ?`, destinationID, domainID).Scan(&createdBy)
	return err == nil && p.userIsAdministrator(ctx, createdBy)
}

func writeRemotePolicyHeld(w http.ResponseWriter) {
	writeCodedError(w, http.StatusForbidden, "REMOTE_SCHEDULE_ADMINISTRATOR",
		"Nothing was changed: the administrator set this schedule; only the administrator can change or turn it off.", "")
}

func writeRemoteChoiceRefusal(w http.ResponseWriter, code string) {
	if code == errCodeRightNotGranted {
		writeRemoteRightRefusal(w, rightsLayerSubscription)
		return
	}
	writeCodedError(w, http.StatusConflict, "DESTINATION_NOT_AVAILABLE",
		"Nothing was changed: this destination is not available for this site. Choose another one.", "")
}

func (p *Panel) handleDeleteRemotePolicy(w http.ResponseWriter, r *http.Request, site *remoteDomain) {
	ctx := r.Context()
	c := currentCaller(r)
	destID, err := strconv.ParseInt(r.URL.Query().Get("destination_id"), 10, 64)
	if err != nil || destID < 1 {
		writeClientError(w, http.StatusBadRequest, "destination_id is required")
		return
	}
	db := p.db.GetDB()
	if site == nil {
		_, err = db.ExecContext(ctx, `DELETE FROM remote_backup_policies WHERE destination_id = ? AND domain_id IS NULL`, destID)
	} else {
		// Turning an own schedule off is always allowed (it stops new use
		// only); one the administrator set stays the administrator's.
		if p.policyHeldByAdministrator(ctx, c, destID, site.ID) {
			writeRemotePolicyHeld(w)
			return
		}
		_, err = db.ExecContext(ctx, `DELETE FROM remote_backup_policies WHERE destination_id = ? AND domain_id = ?`, destID, site.ID)
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	resource := 0
	if site != nil {
		resource = site.ID
	}
	p.audit(r, "backup.remote.schedule.off", "domain", resource)
	_ = json.NewEncoder(w).Encode(map[string]any{"removed": true,
		"note": "The schedule was turned off. Backups already sent stay at the destination."})
}

func (p *Panel) listPolicies(ctx context.Context, where string, args ...any) ([]remotePolicy, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT `+remotePolicyColumns+` FROM remote_backup_policies WHERE `+where+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []remotePolicy
	for rows.Next() {
		pol, err := scanRemotePolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, pol)
	}
	return out, rows.Err()
}

func (p *Panel) policyViews(ctx context.Context, c *Caller, policies []remotePolicy) []remotePolicyView {
	out := []remotePolicyView{}
	for _, pol := range policies {
		v := remotePolicyView{ID: pol.ID, DestinationID: pol.DestinationID, EverySite: pol.DomainID == 0,
			Frequency: pol.Frequency, BackupType: pol.BackupType, KeepDaily: pol.KeepDaily, KeepWeekly: pol.KeepWeekly,
			Enabled: pol.Enabled}
		if d, err := p.loadRemoteDestination(ctx, pol.DestinationID); err == nil {
			v.Destination = d.Name
		}
		v.Editable = c.hasAccountRole(roleAdmin) || (pol.DomainID != 0 && !p.userIsAdministrator(ctx, pol.CreatedBy))
		out = append(out, v)
	}
	return out
}

func (p *Panel) handleListServerPolicies(w http.ResponseWriter, r *http.Request) {
	policies, err := p.listPolicies(r.Context(), `domain_id IS NULL`)
	if err != nil {
		writeServerError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"policies": p.policyViews(r.Context(), currentCaller(r), policies)})
}

// ----- per site -----

func (p *Panel) handleDomainRemoteBackup(w http.ResponseWriter, r *http.Request, domainID int, kind string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	ctx := r.Context()
	c := currentCaller(r)
	site, err := p.loadRemoteDomain(ctx, domainID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeClientError(w, http.StatusNotFound, "domain not found")
		} else {
			writeServerError(w, err)
		}
		return
	}
	switch kind {
	case "backup-remote":
		p.handleSiteRemoteOverview(w, r, site)
	case "backup-remote-policy":
		if r.Method == http.MethodPut {
			p.handleSaveRemotePolicy(w, r, &site)
		} else {
			p.handleDeleteRemotePolicy(w, r, &site)
		}
	case "backup-remote-objects":
		p.handleSiteRemoteObjects(w, r, site)
	case "backup-remote-send":
		p.handleSiteRemoteSend(w, r, site)
	case "backup-remote-restore":
		p.handleSiteRemoteRestore(w, r, site)
	case "backup-remote-operation":
		op, err := p.loadRemoteOperation(ctx, r.URL.Query().Get("id"))
		if err != nil || op.DomainID != site.ID {
			writeClientError(w, http.StatusNotFound, "operation not found")
			return
		}
		_ = json.NewEncoder(w).Encode(p.remoteOperationViewFor(ctx, op))
	default:
		_ = c
		writeClientError(w, http.StatusNotFound, "not found")
	}
}

func (p *Panel) handleSiteRemoteOverview(w http.ResponseWriter, r *http.Request, site remoteDomain) {
	ctx := r.Context()
	c := currentCaller(r)
	destinations, err := p.destinationsForSite(ctx, c, site)
	if err != nil {
		writeServerError(w, err)
		return
	}
	views := []remoteDestinationView{}
	for _, d := range destinations {
		views = append(views, remoteDestinationViewFor(c, d, site.OwnerID))
	}
	policies, err := p.listPolicies(ctx, `domain_id = ? OR domain_id IS NULL`, site.ID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT `+remoteOperationColumns+`
		FROM remote_backup_operations WHERE domain_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`, site.ID, remoteRecentOperations)
	if err != nil {
		writeServerError(w, err)
		return
	}
	ops := []remoteOperationView{}
	var raw []remoteOperation
	for rows.Next() {
		op, err := scanRemoteOperation(rows)
		if err != nil {
			rows.Close()
			writeServerError(w, err)
			return
		}
		raw = append(raw, op)
	}
	rows.Close()
	for _, op := range raw {
		ops = append(ops, p.remoteOperationViewFor(ctx, op))
	}
	refusal, err := p.ownDestinationRefusal(ctx, c)
	if err != nil {
		writeServerError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"destinations": views, "policies": p.policyViews(ctx, c, policies), "operations": ops,
		"can_add_destination": refusal == "", "add_refused_by": refusal,
	})
}

type remoteObjectView struct {
	ObjectKey   string    `json:"object_key"`
	BackupName  string    `json:"backup_name"`
	Type        string    `json:"type"`
	Group       string    `json:"group"` // manual | schedule
	CreatedAt   time.Time `json:"created_at"`
	Size        int64     `json:"size"`
	HasSidecar  bool      `json:"has_sidecar"`
	OtherServer bool      `json:"other_server,omitempty"`
}

func (p *Panel) siteDestination(w http.ResponseWriter, r *http.Request, site remoteDomain, destID int64) (remoteDestination, bool) {
	ctx := r.Context()
	d, err := p.loadRemoteDestination(ctx, destID)
	if err != nil {
		writeClientError(w, http.StatusNotFound, "destination not found")
		return d, false
	}
	refusal, err := p.destinationChoiceRefusal(ctx, currentCaller(r), d, site)
	if err != nil {
		writeServerError(w, err)
		return d, false
	}
	if refusal != "" {
		writeRemoteChoiceRefusal(w, refusal)
		return d, false
	}
	return d, true
}

func writeRemoteStoreFailure(w http.ResponseWriter, err error) {
	f := classifyRemoteError(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	_ = json.NewEncoder(w).Encode(apiErrorBody{Error: f.detail + ". " + remoteNextAction(f.code), Code: f.code, Reason: f.class})
}

func (p *Panel) handleSiteRemoteObjects(w http.ResponseWriter, r *http.Request, site remoteDomain) {
	ctx := r.Context()
	destID, _ := strconv.ParseInt(r.URL.Query().Get("destination_id"), 10, 64)
	d, ok := p.siteDestination(w, r, site, destID)
	if !ok {
		return
	}
	tag, err := p.remoteServerTag(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	listCtx, cancel := context.WithTimeout(ctx, remoteBackupTestTimeout)
	defer cancel()
	store, _, err := p.openDestination(listCtx, d)
	if err != nil {
		writeRemoteStoreFailure(w, err)
		return
	}
	defer store.Close()
	folder, err := remotebackup.SiteFolder(tag, site.Name, site.ID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	objects, err := store.List(listCtx, folder+"/")
	if err != nil {
		writeRemoteStoreFailure(w, err)
		return
	}
	sidecars := map[string]bool{}
	for _, obj := range objects {
		if strings.HasSuffix(obj.Key, remotebackup.EncryptedSuffix+remotebackup.SidecarSuffix) {
			sidecars[strings.TrimSuffix(obj.Key, remotebackup.SidecarSuffix)] = true
		}
	}
	out := []remoteObjectView{}
	for _, obj := range objects {
		parsed, err := remotebackup.ParseObjectKey(obj.Key)
		if err != nil || parsed.DomainID != site.ID {
			continue
		}
		created, _ := remotebackup.BackupCreatedAt(parsed.BackupName)
		group := "manual"
		if parsed.PolicyID > 0 {
			group = "schedule"
		}
		typ := strings.SplitN(parsed.BackupName, "-", 2)[0]
		out = append(out, remoteObjectView{ObjectKey: obj.Key, BackupName: parsed.BackupName, Type: typ, Group: group,
			CreatedAt: created, Size: obj.Size, HasSidecar: sidecars[obj.Key], OtherServer: parsed.ServerTag != tag})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	_ = json.NewEncoder(w).Encode(map[string]any{"objects": out})
}

func (p *Panel) handleSiteRemoteSend(w http.ResponseWriter, r *http.Request, site remoteDomain) {
	ctx := r.Context()
	c := currentCaller(r)
	var body struct {
		BackupName    string `json:"backup_name"`
		DestinationID int64  `json:"destination_id"`
	}
	if err := decodeRemoteJSON(w, r, &body); err != nil || !remotebackup.ValidBackupName(body.BackupName) {
		writeClientError(w, http.StatusBadRequest, "invalid request body or backup name")
		return
	}
	d, ok := p.siteDestination(w, r, site, body.DestinationID)
	if !ok {
		return
	}
	id, created, err := p.queueRemoteOperation(ctx, remoteOperationRequest{
		Kind: "upload", DestinationID: d.ID, Site: site, BackupName: body.BackupName,
		RequestedBy: int64(c.ID), RequestIdentity: requestIdentityFromContext(ctx),
	})
	if err != nil {
		writeServerError(w, err)
		return
	}
	if created {
		p.audit(r, "backup.remote.send", "domain", site.ID)
	}
	op, err := p.loadRemoteOperation(ctx, id)
	if err != nil {
		writeServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"operation": p.remoteOperationViewFor(ctx, op), "created": created})
}

func (p *Panel) handleSiteRemoteRestore(w http.ResponseWriter, r *http.Request, site remoteDomain) {
	ctx := r.Context()
	c := currentCaller(r)
	var body struct {
		DestinationID int64  `json:"destination_id"`
		ObjectKey     string `json:"object_key"`
		Confirm       bool   `json:"confirm"`
	}
	if err := decodeRemoteJSON(w, r, &body); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !body.Confirm {
		writeCodedError(w, http.StatusBadRequest, "CONFIRM_REQUIRED",
			"A restore replaces the site's files (and databases of a full backup); confirm it first. A safety backup of the current state is kept.", "")
		return
	}
	parsed, err := remotebackup.ParseObjectKey(body.ObjectKey)
	if err != nil || parsed.DomainID != site.ID {
		writeClientError(w, http.StatusBadRequest, "this remote backup does not belong to this site")
		return
	}
	d, ok := p.siteDestination(w, r, site, body.DestinationID)
	if !ok {
		return
	}
	if err := p.checkSubscriptionQuota(ctx, site.SubscriptionID, quotaDisk); err != nil {
		writeQuotaRefusal(w, err)
		return
	}
	id, created, err := p.queueRemoteOperation(ctx, remoteOperationRequest{
		Kind: "restore", DestinationID: d.ID, Site: site, BackupName: parsed.BackupName,
		BackupType: strings.SplitN(parsed.BackupName, "-", 2)[0], ObjectKey: body.ObjectKey,
		RequestedBy: int64(c.ID), RequestIdentity: requestIdentityFromContext(ctx),
	})
	if err != nil {
		writeServerError(w, err)
		return
	}
	if created {
		p.audit(r, "backup.remote.restore", "domain", site.ID)
	}
	op, err := p.loadRemoteOperation(ctx, id)
	if err != nil {
		writeServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"operation": p.remoteOperationViewFor(ctx, op), "created": created})
}
