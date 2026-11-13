package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// handleDomainUsage returns one domain's disk use and this month's traffic.
// Called by the domain detail page in the background after render — lists
// never trigger measurements, they read the cached columns.
//
// alpha.127: reading the page never walks the site's files. The disk is the
// last measurement with its time; when it is older than siteDiskFresh one
// measurement starts behind the screen and the answer says `rechecking`.
// Only a site never measured waits for its first measurement. The usage
// worker also measures every site on its schedule (measureDueSiteDisks).
//
// handleDomainUsage bir alan adinin disk kullanimini ve bu ayin trafigini
// dondurur. Sayfa okumasi site dosyalarini taramaz: son olcum zamaniyla
// gelir; eskiyse arkada tek bir olcum baslar.
func (p *Panel) handleDomainUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	domainID, err := strconv.Atoi(parts[4])
	if err != nil {
		http.Error(w, "invalid domain ID", http.StatusBadRequest)
		return
	}

	var domainName, docroot string
	var storedDisk sql.NullInt64
	var storedAt sql.NullString
	err = p.db.GetDB().QueryRowContext(r.Context(), `
		SELECT d.name, s.document_root, s.disk_usage_bytes, s.usage_updated_at
		FROM sites s JOIN domains d ON d.id = s.domain_id
		WHERE s.domain_id = ?`, domainID).Scan(&domainName, &docroot, &storedDisk, &storedAt)
	if err != nil {
		http.Error(w, "site not found", http.StatusNotFound)
		return
	}

	measuredAt, measured := parseSiteUsageTime(storedAt.String)
	measured = measured && storedDisk.Valid
	disk := storedDisk.Int64
	rechecking := false
	if !measured {
		run := p.siteDiskMeasurements.start(p, domainID, filepath.Dir(docroot), domainName)
		select {
		case <-run.wait:
		case <-r.Context().Done():
			return
		}
		if run.err != nil {
			if errors.Is(run.err, errSiteDiskUnmeasured) {
				writeClientError(w, http.StatusConflict, run.err.Error())
				return
			}
			writeServerError(w, run.err)
			return
		}
		disk, measuredAt = run.disk, run.at
	} else if usageNow().Sub(measuredAt) >= siteDiskFresh {
		p.siteDiskMeasurements.start(p, domainID, filepath.Dir(docroot), domainName)
		rechecking = true
	}

	// alpha.118 (D-044): this month's traffic is the collected number
	// (traffic_daily), not a re-read of a log the distribution rotates daily.
	traffic, err := p.siteMonthTraffic(r.Context(), domainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	var sourceState, sourceReason sql.NullString
	_ = p.db.GetDB().QueryRowContext(r.Context(), `SELECT state, reason FROM traffic_sources WHERE domain_id = ?`, domainID).
		Scan(&sourceState, &sourceReason)
	state := sourceState.String
	if state == "" {
		state = "unknown"
		sourceReason.String = "not_collected_yet"
	}

	json.NewEncoder(w).Encode(map[string]any{
		"disk_usage":     disk,
		"bandwidth":      traffic,
		"traffic_state":  state,
		"traffic_reason": sourceReason.String,
		"measured_at":    measuredAt.UTC().Format(time.RFC3339),
		"rechecking":     rechecking,
	})
}

// siteDiskFresh: a page visit later than this after the last measurement
// starts one measurement behind the screen. A variable for tests.
var siteDiskFresh = 15 * time.Minute

const siteDiskMeasureLimit = 10 * time.Minute

// errSiteDiskUnmeasured: the Agent answered that the disk could not be
// measured (a refusal, not an unreachable Agent).
var errSiteDiskUnmeasured = errors.New("the site's disk use could not be measured")

func parseSiteUsageTime(value string) (time.Time, bool) {
	at, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	return at, err == nil
}

func (p *Panel) siteMonthTraffic(ctx context.Context, domainID int) (int64, error) {
	from, to := trafficMonthBounds(usageNow())
	var traffic int64
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT COALESCE(SUM(bytes_out), 0) FROM traffic_daily WHERE domain_id = ? AND day >= ? AND day < ?`,
		domainID, from, to).Scan(&traffic)
	return traffic, err
}

// siteDiskMeasurement is one site's disk measurement, run once however many
// readers ask for it.
type siteDiskMeasurement struct {
	wait chan struct{}
	disk int64
	at   time.Time
	err  error
}

type siteDiskMeasurements struct {
	mu      sync.Mutex
	running map[int]*siteDiskMeasurement
}

// start joins the measurement of this site that is running, or starts one.
// It does not belong to the request that asked: a closed page does not stop
// it, and its result is stored on the site row for the next read.
func (m *siteDiskMeasurements) start(p *Panel, domainID int, siteHome, domain string) *siteDiskMeasurement {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running == nil {
		m.running = map[int]*siteDiskMeasurement{}
	}
	if running := m.running[domainID]; running != nil {
		return running
	}
	run := &siteDiskMeasurement{wait: make(chan struct{})}
	m.running[domainID] = run
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), siteDiskMeasureLimit)
		run.disk, run.at, run.err = p.measureSiteDisk(ctx, domainID, siteHome, domain)
		cancel()
		m.mu.Lock()
		delete(m.running, domainID)
		m.mu.Unlock()
		close(run.wait)
	}()
	return run
}

// measureSiteDisk measures one site's files and stores the number with its
// time. A measurement that failed stores nothing: the last one stays, with
// its own time.
func (p *Panel) measureSiteDisk(ctx context.Context, domainID int, siteHome, domain string) (int64, time.Time, error) {
	var resp transport.SiteUsageResponse
	req := transport.SiteUsageRequest{SiteHome: siteHome, Domain: domain, DiskOnly: true}
	if err := p.callAgentContext(ctx, "Agent.SiteUsage", &req, &resp); err != nil {
		return 0, time.Time{}, err
	}
	if resp.Error != "" {
		return 0, time.Time{}, fmt.Errorf("%w: %s", errSiteDiskUnmeasured, resp.Error)
	}
	at := usageNow()
	traffic, err := p.siteMonthTraffic(ctx, domainID)
	if err != nil {
		return 0, time.Time{}, err
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE sites SET disk_usage_bytes = ?, traffic_month_bytes = ?, usage_updated_at = ?
		WHERE domain_id = ?`,
		resp.DiskBytes, traffic, at.UTC().Format(time.RFC3339), domainID); err != nil {
		return 0, time.Time{}, err
	}
	return resp.DiskBytes, at, nil
}

// measureDueSiteDisks is the usage worker's share (alpha.127): the sites not
// measured for diskMeasureInterval, oldest first, within the round's budget,
// so a domain page mostly finds a recent number.
func (p *Panel) measureDueSiteDisks(ctx context.Context) error {
	cutoff := usageNow().Add(-diskMeasureInterval).UTC().Format(time.RFC3339)
	// A site whose domain is being deleted is not measured: its files are
	// removed on purpose, and asking every round only logged "could not be
	// measured" for it (alpha.133 lab, sil7.test, every five minutes).
	// Silinmekte olan alan adinin sitesi olculmez.
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT s.domain_id, s.document_root, d.name
		FROM sites s JOIN domains d ON d.id = s.domain_id
		WHERE COALESCE(s.document_root, '') <> '' AND (s.usage_updated_at IS NULL OR s.usage_updated_at < ?)
		  AND NOT EXISTS (SELECT 1 FROM domain_deletion_operations op WHERE op.domain_id = s.domain_id)
		ORDER BY COALESCE(s.usage_updated_at, ''), s.domain_id
		LIMIT ?`, cutoff, diskMeasurePerTick)
	if err != nil {
		return err
	}
	type due struct {
		domainID      int
		docroot, name string
	}
	var sites []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.domainID, &d.docroot, &d.name); err != nil {
			rows.Close()
			return err
		}
		sites = append(sites, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	start := usageNow()
	var errs []error
	for _, site := range sites {
		if usageNow().Sub(start) > diskMeasureBudget {
			break
		}
		run := p.siteDiskMeasurements.start(p, site.domainID, filepath.Dir(site.docroot), site.name)
		select {
		case <-run.wait:
		case <-ctx.Done():
			return ctx.Err()
		}
		if run.err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", site.name, run.err))
		}
	}
	return errors.Join(errs...)
}
