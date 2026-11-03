package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Subscription disk usage, Panel side (alpha.118, D-044). The Agent measures
// one subscription at a time at low priority (Agent.SubscriptionDiskUsageV1):
// every 6 hours, every hour while the hosting file system has less than 10%
// free, and soon after a restore, an import or an app install asks for it.
// The last measurement is in subscription_disk_usage; quota checks, the
// reseller pool and transfers read it.
//
// Abonelik disk kullanımı: Agent her seferinde bir aboneliği düşük
// öncelikle ölçer; 6 saatte bir, boş alan %10'un altındayken saatte bir.

const (
	diskMeasureInterval    = 6 * time.Hour
	diskMeasureLowInterval = time.Hour
	diskLowFreePercent     = 10
	// diskMeasurePerTick bounds the measurements of one worker round.
	diskMeasurePerTick = 20
	diskMeasureBudget  = 4 * time.Minute
)

// diskMeasurer is what the worker asks the Agent (a test seam).
type diskMeasurer interface {
	measure(ctx context.Context, req transport.SubscriptionDiskUsageRequest) (transport.SubscriptionDiskUsageResponse, error)
}

type agentDiskMeasurer struct{ p *Panel }

func (m agentDiskMeasurer) measure(ctx context.Context, req transport.SubscriptionDiskUsageRequest) (transport.SubscriptionDiskUsageResponse, error) {
	var resp transport.SubscriptionDiskUsageResponse
	err := m.p.callAgentContext(ctx, "Agent.SubscriptionDiskUsageV1", &req, &resp)
	return resp, err
}

func (p *Panel) diskMeasurerOps() diskMeasurer {
	if p.diskMeasurer != nil {
		return p.diskMeasurer
	}
	return agentDiskMeasurer{p: p}
}

// requestDiskMeasurement asks for a measurement soon (after a restore, an
// import or an app install). It never fails the caller.
func (p *Panel) requestDiskMeasurement(ctx context.Context, subscriptionID int) {
	if subscriptionID <= 0 {
		return
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `
		INSERT INTO subscription_disk_usage (subscription_id, requested_at) VALUES (?, datetime('now'))
		ON CONFLICT(subscription_id) DO UPDATE SET requested_at = excluded.requested_at`, subscriptionID); err != nil {
		log.Printf("[disk] request a measurement of subscription %d: %v", subscriptionID, err)
		return
	}
	p.kickUsageWorker()
}

// diskMeasurementRequest builds the Agent request: the subscription's mail
// domains and its databases.
func diskMeasurementRequest(ctx context.Context, q sqlQueryer, subscriptionID int) (transport.SubscriptionDiskUsageRequest, error) {
	req := transport.SubscriptionDiskUsageRequest{SubscriptionID: subscriptionID, MailDomains: []string{}, Databases: []transport.SubscriptionDiskDatabase{}}
	rows, err := q.QueryContext(ctx, `
		SELECT DISTINCT d.name FROM domains d
		WHERE d.subscription_id = ? AND EXISTS (SELECT 1 FROM email_accounts e WHERE e.domain_id = d.id)
		ORDER BY d.name`, subscriptionID)
	if err != nil {
		return req, err
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return req, err
		}
		req.MailDomains = append(req.MailDomains, name)
	}
	rows.Close()
	rows, err = q.QueryContext(ctx, `
		SELECT db.name, LOWER(COALESCE(t.name, '')), COALESCE(ds.host, ''), COALESCE(ds.port, 0)
		FROM databases_v2 db
		JOIN database_servers ds ON ds.id = db.server_id
		JOIN database_server_types t ON t.id = ds.type_id
		WHERE db.subscription_id = ? ORDER BY db.name`, subscriptionID)
	if err != nil {
		return req, err
	}
	defer rows.Close()
	for rows.Next() {
		var name, engine, host string
		var port int
		if err := rows.Scan(&name, &engine, &host, &port); err != nil {
			return req, err
		}
		if !localDatabaseHost(host) {
			continue
		}
		switch engine {
		case "mariadb", "mysql":
			req.Databases = append(req.Databases, transport.SubscriptionDiskDatabase{Engine: "mysql", Name: name})
		case "postgresql", "postgres":
			req.Databases = append(req.Databases, transport.SubscriptionDiskDatabase{Engine: "postgresql", Name: name, Port: port})
		}
	}
	return req, rows.Err()
}

// dueDiskMeasurements lists the subscriptions to measure now, oldest first.
func dueDiskMeasurements(ctx context.Context, q sqlQueryer, now time.Time, lowSpace bool) ([]int, error) {
	interval := diskMeasureInterval
	if lowSpace {
		interval = diskMeasureLowInterval
	}
	cutoff := now.Add(-interval).UTC().Format("2006-01-02 15:04:05")
	rows, err := q.QueryContext(ctx, `
		SELECT s.id FROM subscriptions s
		LEFT JOIN subscription_disk_usage u ON u.subscription_id = s.id
		WHERE u.subscription_id IS NULL OR u.attempted_at IS NULL OR u.attempted_at <= ?
		   OR (u.requested_at IS NOT NULL AND u.requested_at >= u.attempted_at)
		ORDER BY CASE WHEN u.requested_at IS NOT NULL AND (u.attempted_at IS NULL OR u.requested_at >= u.attempted_at) THEN 0 ELSE 1 END,
		         COALESCE(u.attempted_at, ''), s.id
		LIMIT ?`, cutoff, diskMeasurePerTick)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// measureDueDisks measures what is due, one at a time, within a budget.
// It returns the lowest free space seen (-1 when none was read).
func (p *Panel) measureDueDisks(ctx context.Context) (int, error) {
	db := p.db.GetDB()
	start := usageNow()
	ids, err := dueDiskMeasurements(ctx, db, start, p.usageWorker.lowSpace())
	if err != nil {
		return -1, err
	}
	lowest := -1
	var errs []error
	for _, id := range ids {
		if time.Since(start) > diskMeasureBudget {
			break
		}
		free, err := p.measureSubscriptionDisk(ctx, id)
		if err != nil {
			errs = append(errs, fmt.Errorf("subscription %d: %w", id, err))
		}
		if free >= 0 && (lowest < 0 || free < lowest) {
			lowest = free
		}
	}
	if lowest >= 0 {
		p.usageWorker.setLowSpace(lowest < diskLowFreePercent)
	}
	return lowest, errors.Join(errs...)
}

// measureSubscriptionDisk measures one subscription and stores the result.
// An unreachable Agent or an unreadable part stores "unknown"/"partial" with
// the reason, never a 0 that would read as "empty".
func (p *Panel) measureSubscriptionDisk(ctx context.Context, subscriptionID int) (int, error) {
	db := p.db.GetDB()
	req, err := diskMeasurementRequest(ctx, db, subscriptionID)
	if err != nil {
		return -1, err
	}
	resp, callErr := p.diskMeasurerOps().measure(ctx, req)
	if callErr == nil && resp.Error != "" {
		callErr = errors.New(resp.Error)
	}
	if callErr != nil {
		_, err := db.ExecContext(ctx, `
			INSERT INTO subscription_disk_usage (subscription_id, state, detail, attempted_at) VALUES (?, 'unknown', ?, datetime('now'))
			ON CONFLICT(subscription_id) DO UPDATE SET
			  state = CASE WHEN subscription_disk_usage.measured_at IS NULL THEN 'unknown' ELSE subscription_disk_usage.state END,
			  detail = excluded.detail, attempted_at = excluded.attempted_at`,
			subscriptionID, boundedAgentDiagnostic("the server could not measure it: "+callErr.Error()))
		return -1, errors.Join(callErr, err)
	}
	state, detail := "measured", ""
	if len(resp.Unknown) > 0 {
		state = "partial"
		detail = "not measured: " + strings.Join(resp.Unknown, ", ")
	}
	if resp.ProjectQuota {
		if detail != "" {
			detail += "; "
		}
		detail += "the file system has project quotas; CelikPanel measures by walking the folders"
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_disk_usage (subscription_id, web_bytes, mail_bytes, db_bytes, method, state, detail, measured_at, attempted_at)
		VALUES (?, ?, ?, ?, 'walk', ?, ?, datetime('now'), datetime('now'))
		ON CONFLICT(subscription_id) DO UPDATE SET
		  web_bytes = excluded.web_bytes, mail_bytes = excluded.mail_bytes, db_bytes = excluded.db_bytes,
		  method = excluded.method, state = excluded.state, detail = excluded.detail,
		  measured_at = excluded.measured_at, attempted_at = excluded.attempted_at`,
		subscriptionID, maxInt64(resp.WebBytes, 0), maxInt64(resp.MailBytes, 0), maxInt64(resp.DBBytes, 0), state, nullIfEmpty(detail))
	return resp.FreePercent, err
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// restartDiskCountdown starts the 7-day period again from now (an
// operator's resume of a disk suspension).
func restartDiskCountdown(ctx context.Context, q sqlQueryer, subscriptionID int) error {
	_, err := q.ExecContext(ctx, `
		UPDATE subscription_disk_usage SET over_since = CASE WHEN over_since IS NULL THEN NULL ELSE ? END
		WHERE subscription_id = ?`, usageNow().UTC().Format("2006-01-02 15:04:05"), subscriptionID)
	return err
}

// diskRow is one subscription's measured disk against its limit.
type diskRow struct {
	ID        int
	LimitMB   int64
	UsedBytes int64
	State     string
	OverSince sql.NullString
	CauseDisk bool
	Admin     bool
	Uploads   bool // upload_blocked
}

// evaluateDiskLimits applies the disk rules to the stored measurements:
// notices at 80% and 100%, the countdown, the 7-day suspension and its end,
// and the upload block. It only reads what was measured; it never measures.
func (p *Panel) evaluateDiskLimits(ctx context.Context) error {
	db := p.db.GetDB()
	now := usageNow()
	rows, err := db.QueryContext(ctx, `
		SELECT s.id, COALESCE(s.disk_quota_mb, 0), u.web_bytes + u.mail_bytes + u.db_bytes, u.state, u.over_since,
		       COALESCE(ss.cause_disk, 0),
		       CASE WHEN o.role = 'admin' AND s.plan_id IS NULL AND s.name = 'Admin Subscription' THEN 1 ELSE 0 END,
		       u.upload_blocked
		FROM subscriptions s
		JOIN users o ON o.id = s.owner_id
		JOIN subscription_disk_usage u ON u.subscription_id = s.id
		LEFT JOIN subscription_suspensions ss ON ss.subscription_id = s.id
		WHERE u.measured_at IS NOT NULL`)
	if err != nil {
		return err
	}
	var items []diskRow
	for rows.Next() {
		var row diskRow
		if err := rows.Scan(&row.ID, &row.LimitMB, &row.UsedBytes, &row.State, &row.OverSince, &row.CauseDisk, &row.Admin, &row.Uploads); err != nil {
			rows.Close()
			return err
		}
		items = append(items, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	var errs []error
	for _, row := range items {
		if err := p.evaluateDiskRow(ctx, row, now); err != nil {
			errs = append(errs, fmt.Errorf("subscription %d: %w", row.ID, err))
		}
	}
	return errors.Join(errs...)
}

// parseUsageTime reads a stored UTC time (SQLite datetime() or RFC 3339).
func parseUsageTime(value string) (time.Time, bool) {
	t := parseDBTime(value)
	return t, !t.IsZero()
}

func (p *Panel) evaluateDiskRow(ctx context.Context, row diskRow, now time.Time) error {
	db := p.db.GetDB()
	limit := row.LimitMB * mib
	over := !row.Admin && limit > 0 && row.UsedBytes >= limit
	if !over {
		// Under the limit (or unlimited): the episode ends.
		if row.OverSince.Valid {
			if _, err := db.ExecContext(ctx, `UPDATE subscription_disk_usage SET over_since = NULL WHERE subscription_id = ?`, row.ID); err != nil {
				return err
			}
		}
		if row.Uploads {
			if _, err := db.ExecContext(ctx, `UPDATE subscription_disk_usage SET upload_blocked = 0 WHERE subscription_id = ?`, row.ID); err != nil {
				return err
			}
		}
		if row.CauseDisk {
			no := false
			if _, err := p.acceptSuspensionChange(ctx, row.ID, suspensionCauseChange{Disk: &no, Trigger: suspensionTriggerManual}, false); err != nil {
				return err
			}
		}
		if !row.Admin && limit > 0 && usageLevel(row.UsedBytes, limit) == 80 {
			name, owner, reseller := subscriptionNoticeNames(ctx, db, row.ID)
			recipients, err := usageRecipientsFor(ctx, db, row.ID)
			if err != nil {
				return err
			}
			p.storeUsageNotices(ctx, db, recipients, noticeDisk80, row.ID, 0, trafficPeriod(now),
				usageNoticeBody{Subscription: name, Owner: owner, Reseller: reseller, UsedBytes: row.UsedBytes, LimitBytes: limit, Period: trafficPeriod(now)})
		}
		return nil
	}
	since := now.UTC()
	if row.OverSince.Valid {
		if parsed, ok := parseUsageTime(row.OverSince.String); ok {
			since = parsed
		}
	} else if _, err := db.ExecContext(ctx, `UPDATE subscription_disk_usage SET over_since = ? WHERE subscription_id = ?`,
		since.Format("2006-01-02 15:04:05"), row.ID); err != nil {
		return err
	}
	if !row.Uploads {
		if _, err := db.ExecContext(ctx, `UPDATE subscription_disk_usage SET upload_blocked = 1 WHERE subscription_id = ?`, row.ID); err != nil {
			return err
		}
	}
	suspendOn := since.Add(diskSuspendAfter)
	episode := since.Format("2006-01-02 15:04:05")
	name, owner, reseller := subscriptionNoticeNames(ctx, db, row.ID)
	body := usageNoticeBody{Subscription: name, Owner: owner, Reseller: reseller, UsedBytes: row.UsedBytes, LimitBytes: limit,
		Period: episode, SuspendOn: suspendOn.In(time.Local).Format("2006-01-02")}
	recipients, err := usageRecipientsFor(ctx, db, row.ID)
	if err != nil {
		return err
	}
	p.storeUsageNotices(ctx, db, recipients, noticeDisk100, row.ID, 0, episode, body)
	switch {
	case !now.Before(suspendOn):
		if !row.CauseDisk {
			yes := true
			if _, err := p.acceptSuspensionChange(ctx, row.ID, suspensionCauseChange{
				Disk: &yes, Trigger: suspensionTriggerManual,
				Reason: fmt.Sprintf("Disk usage (%s) has been over the limit (%s) for 7 days.", formatMB(row.UsedBytes), formatMB(limit)),
			}, false); err != nil {
				return err
			}
			p.storeUsageNotices(ctx, db, recipients, noticeDiskSuspended, row.ID, 0, episode, body)
		}
	case !now.Before(suspendOn.Add(-diskCountdownNotice)):
		p.storeUsageNotices(ctx, db, recipients, noticeDiskCountdown, row.ID, 0, episode, body)
	}
	return nil
}

// subscriptionUploadsBlocked: the subscription is at or over its disk limit,
// so the panel's file manager refuses uploads.
func subscriptionUploadsBlocked(ctx context.Context, q sqlQueryer, subscriptionID int) (bool, error) {
	var blocked int
	err := q.QueryRowContext(ctx, `SELECT upload_blocked FROM subscription_disk_usage WHERE subscription_id = ?`, subscriptionID).Scan(&blocked)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return blocked == 1, err
}

// subscriptionDiskView is what the screens show of one subscription's disk.
type subscriptionDiskView struct {
	WebBytes   int64  `json:"web_bytes"`
	MailBytes  int64  `json:"mail_bytes"`
	DBBytes    int64  `json:"db_bytes"`
	State      string `json:"state"` // measured | partial | unknown | not_measured_yet
	Detail     string `json:"detail,omitempty"`
	MeasuredAt string `json:"measured_at,omitempty"`
	// OverSince and SuspendOn: the 7-day countdown, when over the limit.
	OverSince      string `json:"over_since,omitempty"`
	SuspendOn      string `json:"suspend_on,omitempty"`
	UploadsBlocked bool   `json:"uploads_blocked"`
	// UploadsApplied: FTP/SFTP and PHP were brought to UploadsBlocked.
	// UploadsNote: codes the screen explains (ftp_kept, php_kept, ssh_open,
	// or "error:<detail>" while it is retried).
	UploadsApplied bool   `json:"uploads_applied"`
	UploadsNote    string `json:"uploads_note,omitempty"`
}

func loadSubscriptionDiskView(ctx context.Context, q rowQueryer, subscriptionID int) (subscriptionDiskView, error) {
	view := subscriptionDiskView{State: "not_measured_yet"}
	var state, detail, measured, over, note sql.NullString
	var blocked, applied int
	err := q.QueryRowContext(ctx, `
		SELECT web_bytes, mail_bytes, db_bytes, state, detail, measured_at, over_since, upload_blocked, upload_applied, upload_error
		FROM subscription_disk_usage WHERE subscription_id = ?`, subscriptionID).
		Scan(&view.WebBytes, &view.MailBytes, &view.DBBytes, &state, &detail, &measured, &over, &blocked, &applied, &note)
	if errors.Is(err, sql.ErrNoRows) {
		return view, nil
	}
	if err != nil {
		return view, err
	}
	if measured.Valid {
		view.State = state.String
	} else if detail.Valid {
		view.State = "unknown"
	}
	view.Detail, view.MeasuredAt, view.OverSince = detail.String, measured.String, over.String
	if t, ok := parseUsageTime(over.String); ok {
		view.SuspendOn = t.Add(diskSuspendAfter).UTC().Format(time.RFC3339)
	}
	view.UploadsBlocked = blocked == 1
	view.UploadsApplied = applied == blocked
	view.UploadsNote = note.String
	return view, nil
}

// uploadBlockNative brings one subscription's FTP/SFTP and PHP uploads to
// the wanted state and returns a note for the screen (an owner's file kept,
// command-line accounts). Production asks the Agent; tests replace it.
func (p *Panel) uploadBlockOps() func(ctx context.Context, subscriptionID int, blocked bool) (string, error) {
	if p.uploadBlockNative != nil {
		return p.uploadBlockNative
	}
	return p.applyUploadBlockNative
}

func (p *Panel) applyUploadBlockNative(ctx context.Context, subscriptionID int, blocked bool) (string, error) {
	var notes []string
	req := transport.SetFileAccessReadOnlyRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit), SubscriptionID: subscriptionID, ReadOnly: blocked}
	var resp transport.SetFileAccessReadOnlyResponse
	if err := p.callAgentContext(ctx, "Agent.SetFileAccessReadOnlyV1", &req, &resp); err != nil {
		return "", fmt.Errorf("file access: %w", err)
	}
	if resp.Error != "" {
		return "", fmt.Errorf("file access: %s", resp.Error)
	}
	if resp.Kept {
		notes = append(notes, "ftp_kept")
	}
	if blocked && len(resp.SSHAccounts) > 0 {
		notes = append(notes, "ssh_open")
	}
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT d.id FROM domains d JOIN sites s ON s.domain_id = d.id
		WHERE d.subscription_id = ? AND COALESCE(s.project_type, 'php') = 'php' ORDER BY d.id`, subscriptionID)
	if err != nil {
		return "", err
	}
	var domains []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return "", err
		}
		domains = append(domains, id)
	}
	rows.Close()
	for _, domainID := range domains {
		site, err := p.loadSiteFileSite(ctx, domainID)
		if err != nil || site.PHPVersion == "" {
			continue
		}
		request := p.sitePoolRequest(ctx, site, transport.SitePoolOperationApply)
		request.Uploads = transport.SitePoolUploadsFor(blocked)
		if _, err := p.applySitePool(ctx, site, request); err != nil {
			var held *siteFileHeldError
			if errors.As(err, &held) {
				notes = append(notes, "php_kept")
				continue
			}
			return "", fmt.Errorf("PHP pool of %s: %w", site.Domain, err)
		}
	}
	return strings.Join(notes, ","), nil
}

// applyUploadBlocks brings every subscription whose disk state changed to
// it: uploads closed over FTP/SFTP and in PHP while full, open again when
// under the limit. A failure is kept and retried next round.
func (p *Panel) applyUploadBlocks(ctx context.Context) error {
	db := p.db.GetDB()
	rows, err := db.QueryContext(ctx, `SELECT subscription_id, upload_blocked FROM subscription_disk_usage WHERE upload_blocked <> upload_applied OR (upload_blocked = 1 AND upload_error LIKE 'error:%')`)
	if err != nil {
		return err
	}
	type item struct {
		id      int
		blocked bool
	}
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.blocked); err != nil {
			rows.Close()
			return err
		}
		items = append(items, it)
	}
	rows.Close()
	var errs []error
	for _, it := range items {
		note, err := p.uploadBlockOps()(ctx, it.id, it.blocked)
		if err != nil {
			_, _ = db.ExecContext(ctx, `UPDATE subscription_disk_usage SET upload_error = ? WHERE subscription_id = ?`,
				"error:"+boundedAgentDiagnostic(err.Error()), it.id)
			errs = append(errs, fmt.Errorf("subscription %d: %w", it.id, err))
			continue
		}
		if _, err := db.ExecContext(ctx, `UPDATE subscription_disk_usage SET upload_applied = ?, upload_error = ? WHERE subscription_id = ? AND upload_blocked = ?`,
			it.blocked, nullIfEmpty(note), it.id, it.blocked); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
