package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/rpc"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Domain deletion: databases, mail lists and what stays (alpha.134).
//
// The owner's two rules meet here: nothing a deletion made obsolete stays
// behind, and data the user created is never destroyed unless the person
// chose it.
//
//   - Databases are a choice. "keep" (the default) unbinds them from the
//     domain: they stay in their engine and in the panel, listed on the
//     Databases page as bound to no domain, with their users, passwords and
//     remote access rules unchanged. "delete" deletes each database through
//     the one database deletion of the Databases page and the domain's
//     Databases section (deleteDatabaseRecord, database_delete.go): the same
//     per-user fates, the same rights removal after the drop, the same audit
//     rows and the same partial results, one database at a time, so pressing
//     the deletion again continues with what is left. There is no second
//     implementation here. Until alpha.133 the domain row's foreign key
//     (databases_v2.domain_id ON DELETE CASCADE) removed the panel's records
//     while nothing dropped the databases: they stayed in the engine,
//     invisible and unmanageable.
//   - The choice and the databases it covers are the accepted plan, saved
//     with the deletion marker as a versioned panel_settings record (no
//     schema change): a retry, a reload or CelikPanel finishing the deletion
//     by itself repeats the same plan; a retry that names the other choice is
//     refused before anything changes.
//   - The server's sender-limit lists and the per-domain mail options map
//     are made from the panel's records; a domain being deleted leaves them
//     at the mail step (loadOutgoingMailboxes and domainOptionsEntries skip a
//     marked domain), and that new state is applied as a step.
//   - What stays on purpose (stored mail moved aside, the backups, unbound
//     databases) is named in the deletion's answer with where and how large.
//
// No schema change: an older release reading this database sees a plan
// record it does not know (it ignores unknown panel_settings keys) and
// databases_v2 rows with domain_id NULL, which alpha.133 already lists as
// bound to no domain. An older release finishing a deletion this release
// started cascades the databases it did not unbind yet, as it always did.
//
// Silmede veritabanları bir seçimdir; varsayılan tutmaktır. Seçim, silme
// işaretiyle birlikte kaydedilir ve yeniden denemede aynen uygulanır.

const (
	domainDeletionStageMailLists  = "mail_lists_cleanup"
	domainDeletionStageSite       = "site_cleanup"
	domainDeletionStageDatabases  = "database_cleanup"
	domainDeletionReasonMailLists = "mail_lists_cleanup_failed"
	domainDeletionReasonSite      = "site_cleanup_failed"
	domainDeletionReasonDatabases = "database_cleanup_failed"

	domainDeletionDatabasesKeep   = "keep"
	domainDeletionDatabasesDelete = "delete"

	domainDeletionPlanKeyPrefix = "domain_deletion_plan_v1:"
	domainDeletionPlanVersion   = 1
	// The plan names at most this many databases (a domain has a handful).
	domainDeletionPlanDatabaseMax = 500

	errCodeDomainDeletionPlanFixed = "DOMAIN_DELETION_PLAN_FIXED"
)

// domainDeletionStageFailureKinds: each stage that keeps a verified failure
// with the marker, its typed reason and the English sentence of who acts.
var domainDeletionStageFailureKinds = map[string]struct{ reason, english string }{
	domainDeletionStageMailRuntime: {domainDeletionReasonMailCleanupFailed, domainMailCleanupFailedEnglish},
	domainDeletionStageMailLists: {domainDeletionReasonMailLists,
		"Deletion stopped while taking this domain out of the server's sender-limit lists. The website, the DNS zone and the databases are unchanged. " +
			"The server administrator opens Mail > Sending limits on this server, fixes what it reports, then retries this same deletion. Nothing retries by itself."},
	domainDeletionStageSite: {domainDeletionReasonSite,
		"Deletion stopped while removing this domain's website from this server; error_line names the part that is still there. The DNS zone and the databases are unchanged. " +
			"The server administrator checks that part on this server, then retries this same deletion, which repeats the removal and continues. Nothing retries by itself."},
	domainDeletionStageDatabases: {domainDeletionReasonDatabases,
		"Deletion stopped while deleting this domain's databases; error_line says which database and what is left of it. The website is already removed; the DNS zone is unchanged. " +
			"Retry this same deletion: it deletes what is left and continues. If it stops here again, the server administrator makes sure the database engine runs (Services). Nothing retries by itself."},
}

func domainDeletionStageFailureEnglish(stage string) string {
	if kind, ok := domainDeletionStageFailureKinds[stage]; ok {
		return kind.english
	}
	return domainMailCleanupFailedEnglish
}

// domainDeletionPlanDatabase is one database the plan covers.
type domainDeletionPlanDatabase struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Engine string `json:"engine"`
}

type domainDeletionPlan struct {
	Version   int                          `json:"version"`
	DomainID  int                          `json:"domain_id"`
	Domain    string                       `json:"domain"`
	Databases string                       `json:"databases"`
	Covered   []domainDeletionPlanDatabase `json:"covered"`
}

func domainDeletionPlanKey(domainID int) string {
	return domainDeletionPlanKeyPrefix + strconv.Itoa(domainID)
}

var errDomainDeletionPlanFixed = errors.New("this deletion already runs with the other database choice")

// requestedDomainDeletionDatabases reads the person's choice: "" (not said),
// "keep" or "delete". Anything else is refused.
func requestedDomainDeletionDatabases(r *http.Request) (string, bool) {
	values := r.URL.Query()["databases"]
	if len(values) == 0 {
		return "", true
	}
	if len(values) != 1 {
		return "", false
	}
	switch values[0] {
	case domainDeletionDatabasesKeep, domainDeletionDatabasesDelete:
		return values[0], true
	}
	return "", false
}

// readDomainDeletionPlan is read only. A missing, malformed or foreign record
// is absent.
func readDomainDeletionPlan(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, domainID int, domain string) (domainDeletionPlan, bool) {
	var raw string
	if err := q.QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`,
		domainDeletionPlanKey(domainID)).Scan(&raw); err != nil {
		return domainDeletionPlan{}, false
	}
	var plan domainDeletionPlan
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&plan) != nil || plan.Version != domainDeletionPlanVersion || plan.DomainID != domainID ||
		plan.Domain != strings.ToLower(strings.TrimSpace(domain)) ||
		(plan.Databases != domainDeletionDatabasesKeep && plan.Databases != domainDeletionDatabasesDelete) ||
		len(plan.Covered) > domainDeletionPlanDatabaseMax {
		return domainDeletionPlan{}, false
	}
	return plan, true
}

// checkDomainDeletionPlan refuses, before anything changes, a retry that
// names the other choice than the saved plan. Read only.
func (p *Panel) checkDomainDeletionPlan(ctx context.Context, domainID int, domain, requested string) error {
	if requested == "" {
		return nil
	}
	plan, ok := readDomainDeletionPlan(ctx, p.db.GetDB(), domainID, domain)
	if ok && plan.Databases != requested {
		return errDomainDeletionPlanFixed
	}
	return nil
}

// acceptDomainDeletionPlan saves the plan once, while the deletion marker
// exists, and answers the plan in force: the saved one on a retry.
func (p *Panel) acceptDomainDeletionPlan(ctx context.Context, domainID int, domain, requested string) (domainDeletionPlan, error) {
	db := p.db.GetDB()
	if plan, ok := readDomainDeletionPlan(ctx, db, domainID, domain); ok {
		if requested != "" && plan.Databases != requested {
			return plan, errDomainDeletionPlanFixed
		}
		return plan, nil
	}
	choice := requested
	if choice == "" {
		// Data is not destroyed unless asked (an older release's pending
		// deletion, an API call that does not say).
		choice = domainDeletionDatabasesKeep
	}
	covered, err := domainBoundDatabases(ctx, db, domainID)
	if err != nil {
		return domainDeletionPlan{}, err
	}
	plan := domainDeletionPlan{
		Version: domainDeletionPlanVersion, DomainID: domainID,
		Domain: strings.ToLower(strings.TrimSpace(domain)), Databases: choice, Covered: covered,
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		return plan, err
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO panel_settings (key, value, updated_at)
		SELECT ?, ?, CURRENT_TIMESTAMP
		WHERE EXISTS (SELECT 1 FROM domain_deletion_operations WHERE domain_id = ?)
		ON CONFLICT(key) DO NOTHING`, domainDeletionPlanKey(domainID), string(raw), domainID); err != nil {
		return plan, fmt.Errorf("save domain deletion plan: %w", err)
	}
	saved, ok := readDomainDeletionPlan(ctx, db, domainID, domain)
	if !ok {
		return plan, errors.New("save domain deletion plan: the saved plan could not be read back")
	}
	if requested != "" && saved.Databases != requested {
		return saved, errDomainDeletionPlanFixed
	}
	return saved, nil
}

func clearDomainDeletionPlan(ctx context.Context, q interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, domainID int) error {
	_, err := q.ExecContext(ctx, `DELETE FROM panel_settings WHERE key = ?`, domainDeletionPlanKey(domainID))
	return err
}

// domainBoundDatabases lists the databases bound to the domain. Read only.
func domainBoundDatabases(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, domainID int) ([]domainDeletionPlanDatabase, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT d.id, d.name, dst.name
		FROM databases_v2 d
		JOIN database_servers ds ON ds.id = d.server_id
		JOIN database_server_types dst ON dst.id = ds.type_id
		WHERE d.domain_id = ?
		ORDER BY d.name, d.id`, domainID)
	if err != nil {
		return nil, fmt.Errorf("read the domain's databases: %w", err)
	}
	defer rows.Close()
	out := []domainDeletionPlanDatabase{}
	for rows.Next() {
		var item domainDeletionPlanDatabase
		if err := rows.Scan(&item.ID, &item.Name, &item.Engine); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > domainDeletionPlanDatabaseMax {
		return nil, fmt.Errorf("the domain has more than %d databases", domainDeletionPlanDatabaseMax)
	}
	return out, nil
}

// domainDeletionKept is what the deletion's answer says stays, which
// databases it deleted and, when a database deletion did not finish, that
// deletion's own answer.
type domainDeletionKept struct {
	// Choice is the accepted plan's choice for the databases ("keep" or
	// "delete"), so a retry's question can say it.
	Choice    string                          `json:"choice,omitempty"`
	Databases []domainDeletionPlanDatabase    `json:"databases,omitempty"`
	Deleted   []domainDeletionDeletedDatabase `json:"deleted_databases,omitempty"`
	// DatabaseFailure: the database whose deletion did not finish, with the
	// answer the Databases page would have given for it (the same code,
	// reason and facts, so the screen uses the same sentences).
	DatabaseFailure *domainDeletionDatabaseFailure `json:"database_failure,omitempty"`
	Mail            *domainDeletionKeptMail        `json:"mail,omitempty"`
	Backups         *transport.KeptBackups         `json:"backups,omitempty"`
}

// domainDeletionDeletedDatabase is one deleted database. The user lists are
// the database deletion's own answer (databaseDeletionAnswer) for a database
// this press deleted; one an earlier press deleted carries none.
type domainDeletionDeletedDatabase struct {
	domainDeletionPlanDatabase
	UsersRemoved       []string `json:"users_removed,omitempty"`
	UsersKept          []string `json:"users_kept,omitempty"`
	UsersKeptOnEngine  []string `json:"users_kept_on_engine,omitempty"`
	UsersKeptUnchecked []string `json:"users_kept_unchecked,omitempty"`
	UsersKeptOnServer  []string `json:"users_kept_on_server,omitempty"`
}

type domainDeletionDatabaseFailure struct {
	Name   string          `json:"name"`
	Status int             `json:"status"`
	Answer json.RawMessage `json:"answer"`
}

type domainDeletionKeptMail struct {
	Path string `json:"path"`
	// Bytes is -1 when the size could not be read.
	Bytes int64 `json:"bytes"`
}

func (k *domainDeletionKept) empty() bool {
	return len(k.Databases) == 0 && len(k.Deleted) == 0 && k.DatabaseFailure == nil && k.Mail == nil && k.Backups == nil
}

// planDatabases names what the plan does with the databases before its
// database step runs: kept ones stay whatever happens; deleted ones are
// named as each is deleted (runDomainDeletionDatabaseStage).
func (k *domainDeletionKept) planDatabases(plan domainDeletionPlan) {
	k.Choice = plan.Databases
	k.Deleted = nil
	k.Databases = nil
	if plan.Databases != domainDeletionDatabasesDelete {
		k.Databases = plan.Covered
	}
}

// runDomainDeletionMailListsStage applies the sender-limit lists and the
// per-domain mail options without the domain being deleted (both skip a
// marked domain). A refusal that leaves those lists out of CelikPanel's hands
// (the limits are not installed, the owner's own wiring is kept, another mail
// change is running) does not stop the deletion: the next apply, which runs
// every five minutes, writes them without the domain. A failure the server
// answered is a verified stage failure.
func (p *Panel) runDomainDeletionMailListsStage(ctx context.Context) error {
	resp, asked, err := p.applyOutgoingMail(ctx, false)
	if err == nil && asked && !resp.Applied {
		switch resp.Code {
		case transport.OutgoingMailCheckFailed, transport.OutgoingMailServiceFailed,
			transport.OutgoingMailPostfixFailed, transport.OutgoingMailSelfTestFailed:
			err = fmt.Errorf("sender limits: %s", strings.TrimSpace(resp.Code+" "+resp.Error))
		default:
			log.Printf("domain deletion: the sender limits were not applied now (%s); the next round applies them", resp.Code)
		}
	}
	if err == nil {
		if optionsResp, optionsAsked, optionsErr := p.applyDomainMailOptions(ctx, false); optionsErr != nil {
			err = fmt.Errorf("per-domain mail options: %w", optionsErr)
		} else if optionsAsked && !optionsResp.Applied {
			log.Printf("domain deletion: the per-domain mail options were not applied now; the next round applies them")
		}
	}
	return err
}

// classifyDomainDeletionAgentFailure returns a typed failure when the Agent
// answered (or the panel's own records failed), and false for a lost answer,
// whose outcome is unknown.
func classifyDomainDeletionAgentFailure(stage string, err error, verified bool) (*domainDeletionStageFailure, bool) {
	var serverErr rpc.ServerError
	if !verified && !errors.As(err, &serverErr) {
		return nil, false
	}
	kind := domainDeletionStageFailureKinds[stage]
	return &domainDeletionStageFailure{
		Stage: stage, Reason: kind.reason,
		ErrorLine: boundedDeletionErrorLine(err.Error()), Cause: err,
	}, true
}

// recordStageFailure keeps a verified failure with the marker (an unknown
// outcome keeps nothing; the mail stage of the same attempt cleared earlier
// records).
func (p *Panel) recordDomainDeletionStageFailure(ctx context.Context, domainID int, domain, stage string, err error, verified bool) error {
	failure, ok := classifyDomainDeletionAgentFailure(stage, err, verified)
	if !ok {
		return err
	}
	if recordErr := p.recordDomainDeletionFailure(ctx, domainID, domain, failure); recordErr != nil {
		failure.Cause = errors.Join(err, recordErr)
	}
	return failure
}

// runDomainDeletionDatabaseStage carries out the plan's database choice.
// keep: the plan's databases are unbound from the domain now (the final
// step unbinds any other too). delete: each database of the plan still bound
// to the domain is deleted by deleteDatabaseRecord, the one database
// deletion of the Databases page and the domain's Databases section, and
// recorded in the audit log the same way; one database at a time, so a retry
// continues with the ones that remain. A database deletion that does not
// finish stops this step: its own answer goes into kept.DatabaseFailure, and
// the database stays listed (bound to the domain) until a retry finishes it.
func (p *Panel) runDomainDeletionDatabaseStage(ctx context.Context, r *http.Request, domainID int, domain string,
	plan domainDeletionPlan, kept *domainDeletionKept) error {
	db := p.db.GetDB()
	if plan.Databases != domainDeletionDatabasesDelete {
		if _, err := db.ExecContext(ctx, `
			UPDATE databases_v2 SET domain_id = NULL, updated_at = datetime('now')
			WHERE domain_id = ?`, domainID); err != nil {
			return p.recordDomainDeletionStageFailure(ctx, domainID, domain, domainDeletionStageDatabases,
				fmt.Errorf("unbind the databases from the domain: %w", err), true)
		}
		return nil
	}
	for _, item := range plan.Covered {
		var bound int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM databases_v2 WHERE id = ? AND domain_id = ?`,
			item.ID, domainID).Scan(&bound); err != nil {
			return p.recordDomainDeletionStageFailure(ctx, domainID, domain, domainDeletionStageDatabases,
				fmt.Errorf("database %s: read its record: %w", item.Name, err), true)
		}
		if bound == 0 {
			// Deleted by an earlier press of this deletion, or by its owner
			// meanwhile: gone either way.
			kept.Deleted = append(kept.Deleted, domainDeletionDeletedDatabase{domainDeletionPlanDatabase: item})
			continue
		}
		deleted, err := p.deleteDatabaseRecord(ctx, item.ID)
		if err != nil {
			return p.domainDeletionDatabaseFailed(ctx, r, domainID, domain, item, err, kept)
		}
		p.auditDatabaseDeletion(r, deleted, "deleted")
		kept.Deleted = append(kept.Deleted, domainDeletionDeletedDatabase{
			domainDeletionPlanDatabase: item,
			UsersRemoved:               deleted.UsersRemoved, UsersKept: deleted.UsersKept,
			UsersKeptOnEngine: deleted.UsersKeptOnEngine, UsersKeptUnchecked: deleted.UsersKeptUnchecked,
			UsersKeptOnServer: deleted.UsersKeptOnServer,
		})
	}
	return nil
}

// domainDeletionDatabaseFailed stops the database step at a database whose
// deletion did not finish. The answer and audit row are the ones the
// Databases page gives (databaseDeleteFailureAnswer); the domain deletion
// keeps a verified failure with its marker unless nothing at all is known
// (no answer before the database was dropped), which reads as unknown.
func (p *Panel) domainDeletionDatabaseFailed(ctx context.Context, r *http.Request, domainID int, domain string,
	item domainDeletionPlanDatabase, err error, kept *domainDeletionKept) error {
	status, answer := p.databaseDeleteFailureAnswer(r, item.Name, err)
	kept.DatabaseFailure = &domainDeletionDatabaseFailure{Name: item.Name, Status: status, Answer: answer}
	line := err.Error()
	var body struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(answer, &body) == nil && strings.TrimSpace(body.Error) != "" {
		line = body.Error
	}
	var failed *databaseDeleteError
	unknown := errors.As(err, &failed) && failed.unknown && !failed.databaseGone && !agentUnreachableAlone(errors.Unwrap(failed.cause))
	return p.recordDomainDeletionStageFailure(ctx, domainID, domain, domainDeletionStageDatabases,
		fmt.Errorf("database %s: %s", item.Name, line), !unknown)
}

// writeDomainDeletionPlanFixed answers a retry that names the other database
// choice than the deletion already runs with. Nothing was changed.
func writeDomainDeletionPlanFixed(w http.ResponseWriter, err error) {
	writeCodedError(w, http.StatusConflict, errCodeDomainDeletionPlanFixed,
		"This deletion already runs with the other choice for the domain's databases, so nothing was changed. "+
			"Retry it without naming databases (or with the same choice); it continues as it was accepted.", "")
}
