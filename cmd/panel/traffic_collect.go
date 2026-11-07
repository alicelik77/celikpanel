package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Traffic collection, Panel side (alpha.118, D-044). Every few minutes the
// Panel asks the Agent for the traffic batches it has not confirmed
// (Agent.TrafficCollectV1, cmd/agent/traffic_collect.go), adds them to
// traffic_daily and records the batch sequence in one transaction, so a
// batch is counted exactly once whatever is lost or repeated. Then it
// decides which subscriptions' websites show the traffic limit page.
//
// Trafik toplama: Panel her birkaç dakikada onaylanmamış paketleri alır,
// tek işlemde traffic_daily'ye ekler ve sırayı kaydeder; paket tam bir kez
// sayılır.

// trafficCollector is what the collector asks the Agent (a test seam).
type trafficCollector interface {
	collect(ctx context.Context, req transport.TrafficCollectRequest) (transport.TrafficCollectResponse, error)
}

type agentTrafficCollector struct{ p *Panel }

func (c agentTrafficCollector) collect(ctx context.Context, req transport.TrafficCollectRequest) (transport.TrafficCollectResponse, error) {
	var resp transport.TrafficCollectResponse
	err := c.p.callAgentContext(ctx, "Agent.TrafficCollectV1", &req, &resp)
	return resp, err
}

func (p *Panel) trafficCollectorOps() trafficCollector {
	if p.trafficCollector != nil {
		return p.trafficCollector
	}
	return agentTrafficCollector{p: p}
}

// trafficDomains lists the websites to measure and whether each shows a
// CelikPanel page now (suspended, maintenance or traffic limit). A domain
// whose deletion is accepted is not measured: its site files are removed
// first and its rows go when the deletion ends; until 6 Nov 2026 it was
// reported as a website "not counted" for as long as the deletion waited.
func (p *Panel) trafficDomains(ctx context.Context) ([]transport.TrafficDomain, error) {
	period := trafficPeriod(usageNow())
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT d.id, d.name,
		       CASE WHEN s.status IN ('suspended', 'expired') THEN 1
		            WHEN EXISTS (SELECT 1 FROM traffic_web_blocks b WHERE b.subscription_id = s.id AND b.period = ? AND b.wanted = 1) THEN 1
		            ELSE 0 END
		FROM domains d JOIN sites si ON si.domain_id = d.id
		JOIN subscriptions s ON s.id = d.subscription_id
		WHERE NOT EXISTS (SELECT 1 FROM domain_deletion_operations op WHERE op.domain_id = d.id)
		ORDER BY d.id`, period)
	if err != nil {
		return nil, err
	}
	var out []transport.TrafficDomain
	for rows.Next() {
		var item transport.TrafficDomain
		if err := rows.Scan(&item.DomainID, &item.Domain, &item.Blocked); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Blocked {
			continue
		}
		maintenance, err := siteUnderEmergencyMaintenance(ctx, p.db.GetDB(), out[i].DomainID)
		if err == nil && maintenance {
			out[i].Blocked = true
		}
	}
	return out, nil
}

// collectTraffic runs one collection and commits it.
func (p *Panel) collectTraffic(ctx context.Context) error {
	db := p.db.GetDB()
	var epoch string
	var lastSeq int64
	if err := db.QueryRowContext(ctx, `SELECT agent_epoch, last_seq FROM traffic_collect_state WHERE id = 1`).Scan(&epoch, &lastSeq); err != nil {
		return fmt.Errorf("read the traffic collection state: %w", err)
	}
	domains, err := p.trafficDomains(ctx)
	if err != nil {
		return err
	}
	_, _ = db.ExecContext(ctx, `UPDATE traffic_collect_state SET last_attempt_at = datetime('now') WHERE id = 1`)
	resp, err := p.trafficCollectorOps().collect(ctx, transport.TrafficCollectRequest{Epoch: epoch, AfterSeq: lastSeq, Domains: domains})
	if err == nil && resp.Error != "" {
		err = errors.New(resp.Error)
	}
	if err != nil {
		_, _ = db.ExecContext(ctx, `UPDATE traffic_collect_state SET last_error = ? WHERE id = 1`, boundedAgentDiagnostic(err.Error()))
		return fmt.Errorf("collect traffic: %w", err)
	}
	if err := p.commitTrafficBatches(ctx, epoch, lastSeq, resp); err != nil {
		_, _ = db.ExecContext(ctx, `UPDATE traffic_collect_state SET last_error = ? WHERE id = 1`, boundedAgentDiagnostic(err.Error()))
		return err
	}
	for _, source := range resp.Sources {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO traffic_sources (domain_id, state, reason, checked_at) VALUES (?, ?, ?, datetime('now'))
			ON CONFLICT(domain_id) DO UPDATE SET state = excluded.state, reason = excluded.reason, checked_at = excluded.checked_at`,
			source.DomainID, source.State, nullIfEmpty(source.Reason)); err != nil {
			return err
		}
	}
	_, _ = db.ExecContext(ctx, `DELETE FROM traffic_sources WHERE domain_id NOT IN (SELECT domain_id FROM sites)
		OR domain_id IN (SELECT domain_id FROM domain_deletion_operations)`)
	return nil
}

// commitTrafficBatches adds every batch after the committed sequence, in
// one transaction with the new sequence. A different Agent epoch is a new
// store: its sequence starts again from 0.
func (p *Panel) commitTrafficBatches(ctx context.Context, epoch string, lastSeq int64, resp transport.TrafficCollectResponse) error {
	if resp.Epoch == "" {
		return errors.New("the Agent's traffic answer has no epoch")
	}
	if resp.Epoch != epoch {
		lastSeq = 0
	}
	batches := append([]transport.TrafficBatch(nil), resp.Batches...)
	sort.Slice(batches, func(i, j int) bool { return batches[i].Seq < batches[j].Seq })
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	subscriptionOf := map[int]int{}
	for _, batch := range batches {
		if batch.Seq <= lastSeq {
			continue
		}
		for _, item := range batch.Items {
			if item.DomainID <= 0 || item.BytesOut < 0 || item.BytesIn < 0 || item.Requests < 0 || !validTrafficDay(item.Day) {
				continue
			}
			subID, ok := subscriptionOf[item.DomainID]
			if !ok {
				err := tx.QueryRowContext(ctx, `SELECT subscription_id FROM domains WHERE id = ?`, item.DomainID).Scan(&subID)
				if errors.Is(err, sql.ErrNoRows) {
					subID = 0
				} else if err != nil {
					return err
				}
				subscriptionOf[item.DomainID] = subID
			}
			if subID <= 0 {
				// The website was deleted before its last bytes were read.
				continue
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO traffic_daily (domain_id, day, source, subscription_id, bytes_out, bytes_in, requests, updated_at)
				VALUES (?, ?, 'web', ?, ?, ?, ?, datetime('now'))
				ON CONFLICT(domain_id, day, source) DO UPDATE SET
				  bytes_out = bytes_out + excluded.bytes_out, bytes_in = bytes_in + excluded.bytes_in,
				  requests = requests + excluded.requests, subscription_id = excluded.subscription_id,
				  updated_at = excluded.updated_at`,
				item.DomainID, item.Day, subID, item.BytesOut, item.BytesIn, item.Requests); err != nil {
				return err
			}
		}
		lastSeq = batch.Seq
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE traffic_collect_state SET agent_epoch = ?, last_seq = ?, last_success_at = datetime('now'),
		       last_error = NULL, logrotate = ? WHERE id = 1`, resp.Epoch, lastSeq, nullIfEmpty(resp.Logrotate)); err != nil {
		return err
	}
	return tx.Commit()
}

func validTrafficDay(day string) bool {
	_, err := time.Parse("2006-01-02", day)
	return err == nil
}

// trafficSubscriptionRow is one subscription's traffic picture.
type trafficSubscriptionRow struct {
	ID        int
	OwnerID   int
	LimitMB   int64
	UsedBytes int64
	Admin     bool
}

// evaluateTrafficLimits decides, from the committed numbers, which
// subscriptions' websites show the traffic limit page this month, stores the
// 80%/100% notices and renders what changed. A limit raised, a pool raised
// or a new month opens the websites again.
func (p *Panel) evaluateTrafficLimits(ctx context.Context) error {
	db := p.db.GetDB()
	now := usageNow()
	period := trafficPeriod(now)
	from, to := trafficMonthBounds(now)
	rows, err := db.QueryContext(ctx, `
		SELECT s.id, s.owner_id, COALESCE(s.bandwidth_quota_mb, 0), `+subscriptionTrafficSQL+`,
		       CASE WHEN u.role = 'admin' AND s.plan_id IS NULL AND s.name = 'Admin Subscription' THEN 1 ELSE 0 END
		FROM subscriptions s JOIN users u ON u.id = s.owner_id`, from, to)
	if err != nil {
		return err
	}
	var subs []trafficSubscriptionRow
	for rows.Next() {
		var row trafficSubscriptionRow
		if err := rows.Scan(&row.ID, &row.OwnerID, &row.LimitMB, &row.UsedBytes, &row.Admin); err != nil {
			rows.Close()
			return err
		}
		subs = append(subs, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	want := map[int]string{}
	for _, sub := range subs {
		if sub.Admin || sub.LimitMB <= 0 {
			continue
		}
		limit := sub.LimitMB * mib
		// alpha.128 (G10): the plan's notice level and overuse policy; a
		// "notify only" plan never closes the websites.
		policy, _, notifyPct, err := subscriptionOverusePolicy(ctx, db, sub.ID)
		if err != nil {
			return err
		}
		level := usageLevelAt(sub.UsedBytes, limit, notifyPct)
		if level == 0 {
			continue
		}
		name, owner, reseller := subscriptionNoticeNames(ctx, db, sub.ID)
		body := usageNoticeBody{Subscription: name, Owner: owner, Reseller: reseller, UsedBytes: sub.UsedBytes, LimitBytes: limit, Period: period, Cause: trafficCauseSubscription}
		recipients, err := usageRecipientsFor(ctx, db, sub.ID)
		if err != nil {
			return err
		}
		kind := noticeTraffic80
		if level == 100 {
			kind = noticeTraffic100
			if policy != overusePolicyNotify {
				want[sub.ID] = trafficCauseSubscription
			} else {
				body.Cause = trafficCauseNotifyOnly
			}
		}
		p.storeUsageNotices(ctx, db, recipients, kind, sub.ID, 0, period, body)
	}

	// K2: the reseller pool limits the real total of its tree.
	poolRows, err := db.QueryContext(ctx, `SELECT reseller_id, traffic_quota_mb FROM reseller_pools WHERE traffic_quota_mb > 0`)
	if err != nil {
		return err
	}
	type poolRow struct {
		reseller int
		limitMB  int64
	}
	var pools []poolRow
	for poolRows.Next() {
		var row poolRow
		if err := poolRows.Scan(&row.reseller, &row.limitMB); err != nil {
			poolRows.Close()
			return err
		}
		pools = append(pools, row)
	}
	poolRows.Close()
	if err := poolRows.Err(); err != nil {
		return err
	}
	for _, pool := range pools {
		used, err := treeTrafficMonth(ctx, db, pool.reseller, now)
		if err != nil {
			return err
		}
		limit := pool.limitMB * mib
		level := usageLevel(used, limit)
		if level == 0 {
			continue
		}
		var resellerName string
		_ = db.QueryRowContext(ctx, `SELECT username FROM users WHERE id = ?`, pool.reseller).Scan(&resellerName)
		body := usageNoticeBody{Reseller: resellerName, UsedBytes: used, LimitBytes: limit, Period: period, Cause: trafficCausePool}
		recipients := []usageRecipient{{pool.reseller, audienceReseller}}
		admins, err := administratorIDs(ctx, db)
		if err != nil {
			return err
		}
		for _, id := range admins {
			recipients = append(recipients, usageRecipient{id, audienceAdministrator})
		}
		if level == 80 {
			p.storeUsageNotices(ctx, db, recipients, noticePoolTraffic80, 0, pool.reseller, period, body)
			continue
		}
		p.storeUsageNotices(ctx, db, recipients, noticePoolTraffic100, 0, pool.reseller, period, body)
		treeRows, err := db.QueryContext(ctx, treeSubscriptionsSQL, pool.reseller, pool.reseller)
		if err != nil {
			return err
		}
		var tree []int
		for treeRows.Next() {
			var id int
			if err := treeRows.Scan(&id); err != nil {
				treeRows.Close()
				return err
			}
			tree = append(tree, id)
		}
		treeRows.Close()
		for _, id := range tree {
			if _, already := want[id]; already {
				continue
			}
			want[id] = trafficCausePool
			name, owner, _ := subscriptionNoticeNames(ctx, db, id)
			customer, err := usageRecipientsFor(ctx, db, id)
			if err != nil {
				return err
			}
			// The customer is told once; the reseller and the administrator
			// already have the pool notice.
			var only []usageRecipient
			for _, r := range customer {
				if r.Audience == audienceCustomer {
					only = append(only, r)
				}
			}
			p.storeUsageNotices(ctx, db, only, noticeTraffic100, id, pool.reseller, period,
				usageNoticeBody{Subscription: name, Owner: owner, Reseller: resellerName, UsedBytes: used, LimitBytes: limit, Period: period, Cause: trafficCausePool})
		}
	}

	if err := storeTrafficBlocks(ctx, db, period, want); err != nil {
		return err
	}
	return p.applyTrafficBlocks(ctx)
}

// storeTrafficBlocks records the wanted state: rows in want are wanted for
// this period; every other row (another subscription, an earlier month) is
// not wanted any more.
func storeTrafficBlocks(ctx context.Context, db *sql.DB, period string, want map[int]string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for id, cause := range want {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO traffic_web_blocks (subscription_id, period, cause, wanted, applied, since, updated_at)
			VALUES (?, ?, ?, 1, 0, datetime('now'), datetime('now'))
			ON CONFLICT(subscription_id) DO UPDATE SET
			  since = CASE WHEN traffic_web_blocks.wanted = 1 AND traffic_web_blocks.period = excluded.period THEN traffic_web_blocks.since ELSE excluded.since END,
			  period = excluded.period, cause = excluded.cause, wanted = 1, updated_at = excluded.updated_at`,
			id, period, cause); err != nil {
			return err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT subscription_id, period, wanted FROM traffic_web_blocks`)
	if err != nil {
		return err
	}
	var release []int
	for rows.Next() {
		var id, wanted int
		var rowPeriod string
		if err := rows.Scan(&id, &rowPeriod, &wanted); err != nil {
			rows.Close()
			return err
		}
		if _, ok := want[id]; (!ok || rowPeriod != period) && wanted == 1 {
			release = append(release, id)
		}
	}
	rows.Close()
	for _, id := range release {
		if _, err := tx.ExecContext(ctx, `UPDATE traffic_web_blocks SET wanted = 0, updated_at = datetime('now') WHERE subscription_id = ?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// applyTrafficBlocks renders the websites of every subscription whose wanted
// state differs from what was rendered. A failure keeps the row for the next
// run and is shown; a released row is removed once its sites are rendered.
func (p *Panel) applyTrafficBlocks(ctx context.Context) error {
	db := p.db.GetDB()
	// A row never rendered and no longer wanted needs no render.
	if _, err := db.ExecContext(ctx, `DELETE FROM traffic_web_blocks WHERE wanted = 0 AND applied = 0`); err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, `SELECT subscription_id, wanted FROM traffic_web_blocks WHERE wanted <> applied`)
	if err != nil {
		return err
	}
	type item struct{ id, wanted int }
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.wanted); err != nil {
			rows.Close()
			return err
		}
		items = append(items, it)
	}
	rows.Close()
	var errs []error
	for _, it := range items {
		if err := p.renderSubscriptionSites(ctx, it.id); err != nil {
			_, _ = db.ExecContext(ctx, `UPDATE traffic_web_blocks SET last_error = ?, updated_at = datetime('now') WHERE subscription_id = ?`,
				boundedAgentDiagnostic(err.Error()), it.id)
			errs = append(errs, fmt.Errorf("subscription %d: %w", it.id, err))
			continue
		}
		if it.wanted == 1 {
			_, err = db.ExecContext(ctx, `UPDATE traffic_web_blocks SET applied = 1, last_error = NULL WHERE subscription_id = ? AND wanted = 1`, it.id)
		} else {
			_, err = db.ExecContext(ctx, `DELETE FROM traffic_web_blocks WHERE subscription_id = ? AND wanted = 0`, it.id)
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// renderSubscriptionSites renders every website of a subscription.
func (p *Panel) renderSubscriptionSites(ctx context.Context, subscriptionID int) error {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT d.id FROM domains d JOIN sites si ON si.domain_id = d.id WHERE d.subscription_id = ? ORDER BY d.id`, subscriptionID)
	if err != nil {
		return err
	}
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	var errs []error
	for _, id := range ids {
		if err := p.trafficRenderSite(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// trafficRenderSite renders one site (a test seam).
func (p *Panel) trafficRenderSite(ctx context.Context, domainID int) error {
	if p.trafficRender != nil {
		return p.trafficRender(ctx, domainID)
	}
	return p.applyVhostForDomain(ctx, domainID)
}

// trafficReasonWords is the English sentence of an unknown measurement.
func trafficReasonWords(reason string) string {
	switch reason {
	case transport.TrafficReasonFormatKept:
		return "the traffic log format file /etc/nginx/conf.d/celikpanel-traffic.conf was changed outside CelikPanel"
	case transport.TrafficReasonIncludeMissing:
		return "the site's traffic log setting is not in place yet"
	case transport.TrafficReasonVhostKept:
		return "the site's nginx file is the owner's and does not read CelikPanel's directory of the site"
	case transport.TrafficReasonLogUnreadable:
		return "the site's traffic log could not be read"
	case "not_collected_yet":
		return "the first collection has not run yet"
	case "collection_failed":
		return "the last collection from the server failed"
	}
	return strings.TrimSpace(reason)
}
