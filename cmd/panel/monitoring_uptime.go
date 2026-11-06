package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Site checks (alpha.127, monitoring stage 3; owner decision 5 Nov 2026:
// "the server may check its own sites every five minutes"). Every five
// minutes the panel asks each hosted site for its home page from the server
// itself: HTTPS on the loopback address with the site's name when the site
// has an active certificate, otherwise HTTP. A site that does not answer
// within 10 seconds, refuses the connection or answers with a server error
// (5xx) is down; any other answer (a redirect, 403, 404) means the web server
// is serving it. A failed check is repeated once after three seconds before
// it counts, so one dropped connection is not an outage.
//
// The certificate the site actually serves is read on the same connection:
// its last day, and whether it is valid for the name now. That catches a
// renewed certificate the web server never loaded.
//
// What this cannot see: whether the site is reachable from outside (DNS, a
// firewall in front, the network). That is the job of an outside uptime
// checker, which the Monitoring page recommends; with the panel stopped there
// are no checks and no alerts at all (D-022: optional panel automation). A
// paired DNS server could ask from outside, but no cheap authenticated path
// for an HTTP check exists between the two panels yet (open item).
//
// Site denetimi: panel beş dakikada bir her sitenin ana sayfasını sunucunun
// kendisinden, sitenin adıyla geri döngü adresine sorar. 10 saniyede yanıt
// vermeyen, bağlantıyı reddeden ya da 5xx dönen site kapalıdır; başarısız
// denetim üç saniye sonra bir kez yinelenir. Sitenin sunduğu sertifika aynı
// bağlantıda okunur. Dışarıdan erişilebilirliği göremez; bunu dış bir
// çalışma süresi denetleyicisi yapar. Panel dururken denetim ve uyarı yoktur.

const (
	siteCheckEvery       = 5 * time.Minute
	siteCheckTimeout     = 10 * time.Second
	siteCheckRetryAfter  = 3 * time.Second
	siteCheckConcurrency = 4
	// A reading older than three rounds is not the current state.
	siteCheckStale = 3*siteCheckEvery + time.Minute
)

// Replaced by tests: where the checks connect and how long the retry waits.
var (
	siteCheckAddress = func(scheme string) string {
		if scheme == "https" {
			return "127.0.0.1:443"
		}
		return "127.0.0.1:80"
	}
	siteCheckRetryWait = func() { time.Sleep(siteCheckRetryAfter) }
)

type hostedSite struct {
	domain string
	https  bool
}

// hostedSitesToCheck: active domains with a website, in an active
// subscription, not being deleted.
func hostedSitesToCheck(ctx context.Context, db *sql.DB) ([]hostedSite, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT d.name, EXISTS (SELECT 1 FROM ssl_certificates c WHERE c.domain_id = d.id AND c.status = 'active')
		FROM domains d
		LEFT JOIN subscriptions sub ON sub.id = d.subscription_id
		WHERE COALESCE(d.status, 'active') = 'active'
		  AND COALESCE(sub.status, 'active') = 'active'
		  AND EXISTS (SELECT 1 FROM sites s WHERE s.domain_id = d.id)
		  AND NOT EXISTS (SELECT 1 FROM domain_deletion_operations o WHERE o.domain_id = d.id)
		ORDER BY d.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sites []hostedSite
	for rows.Next() {
		var site hostedSite
		if err := rows.Scan(&site.domain, &site.https); err != nil {
			return nil, err
		}
		sites = append(sites, site)
	}
	return sites, rows.Err()
}

type siteCheckResult struct {
	domain       string
	scheme       string
	up           bool
	status       int
	responseMS   int64
	reason       string
	certNotAfter time.Time
	certValid    *bool
}

// checkSite asks one site once.
func checkSite(ctx context.Context, site hostedSite, now func() time.Time) siteCheckResult {
	result := siteCheckResult{domain: site.domain, scheme: "http"}
	if site.https {
		result.scheme = "https"
	}
	var peer *x509.Certificate
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: siteCheckTimeout}).DialContext(ctx, "tcp", siteCheckAddress(result.scheme))
		},
		TLSClientConfig: &tls.Config{
			ServerName: site.domain,
			// The chain is not judged here (a private CA is the owner's
			// choice); the certificate's dates and name are, below.
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS12,
		},
		TLSHandshakeTimeout:   siteCheckTimeout,
		ResponseHeaderTimeout: siteCheckTimeout,
		DisableKeepAlives:     true,
		Proxy:                 nil,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport:     transport,
		Timeout:       siteCheckTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	target := (&url.URL{Scheme: result.scheme, Host: site.domain, Path: "/"}).String()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		result.reason = "error"
		return result
	}
	request.Header.Set("User-Agent", "CelikPanel-site-check/1 (from this server)")
	started := now()
	response, err := client.Do(request)
	result.responseMS = now().Sub(started).Milliseconds()
	if err != nil {
		result.reason = siteCheckReason(err)
		return result
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	response.Body.Close()
	result.status = response.StatusCode
	if response.TLS != nil && len(response.TLS.PeerCertificates) > 0 {
		peer = response.TLS.PeerCertificates[0]
		result.certNotAfter = peer.NotAfter.UTC()
		valid := peer.VerifyHostname(site.domain) == nil && now().Before(peer.NotAfter) && now().After(peer.NotBefore)
		result.certValid = &valid
	}
	if response.StatusCode >= 500 {
		result.reason = "http_5xx"
		return result
	}
	result.up = true
	return result
}

func siteCheckReason(err error) string {
	var netErr net.Error
	var recordErr tls.RecordHeaderError
	var certErr *tls.CertificateVerificationError
	switch {
	case errors.As(err, &netErr) && netErr.Timeout(), errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.As(err, &recordErr), errors.As(err, &certErr), strings.Contains(err.Error(), "tls:"):
		return "tls"
	case strings.Contains(err.Error(), "connection refused"):
		return "refused"
	case strings.Contains(err.Error(), "connection reset"), errors.Is(err, io.EOF), strings.Contains(err.Error(), "EOF"):
		return "closed"
	default:
		return "error"
	}
}

// checkSites runs one round and stores it.
func (p *Panel) checkSites(ctx context.Context, now time.Time) {
	db := p.db.GetDB()
	if !p.alertSettings(ctx).SiteChecks {
		return
	}
	sites, err := hostedSitesToCheck(ctx, db)
	if err != nil {
		log.Printf("monitoring: the sites to check could not be read: %v", err)
		return
	}
	results := runSiteChecks(ctx, sites)
	if err := storeSiteChecks(ctx, db, results, now); err != nil {
		log.Printf("monitoring: site checks could not be stored: %v", err)
	}
}

func runSiteChecks(ctx context.Context, sites []hostedSite) []siteCheckResult {
	results := make([]siteCheckResult, len(sites))
	slots := make(chan struct{}, siteCheckConcurrency)
	var wg sync.WaitGroup
	for i, site := range sites {
		wg.Add(1)
		slots <- struct{}{}
		go func(i int, site hostedSite) {
			defer wg.Done()
			defer func() { <-slots }()
			result := checkSite(ctx, site, time.Now)
			if !result.up {
				siteCheckRetryWait()
				result = checkSite(ctx, site, time.Now)
			}
			results[i] = result
		}(i, site)
	}
	wg.Wait()
	return results
}

// storeSiteChecks writes the round: down_since stays the first failed check
// of the current failure; a site no longer hosted loses its row.
func storeSiteChecks(ctx context.Context, db *sql.DB, results []siteCheckResult, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	at := metricsTime(now)
	keep := make([]any, 0, len(results))
	for _, result := range results {
		keep = append(keep, result.domain)
		state := "up"
		var downSince any
		if !result.up {
			state, downSince = "down", at
		}
		var notAfter, valid any
		if !result.certNotAfter.IsZero() {
			notAfter = metricsTime(result.certNotAfter)
		}
		if result.certValid != nil {
			valid = 0
			if *result.certValid {
				valid = 1
			}
		}
		var status any
		if result.status > 0 {
			status = result.status
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO site_uptime (domain, checked_at, state, scheme, http_status, response_ms, reason, cert_not_after, cert_valid, down_since)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(domain) DO UPDATE SET
				checked_at = excluded.checked_at, state = excluded.state, scheme = excluded.scheme,
				http_status = excluded.http_status, response_ms = excluded.response_ms, reason = excluded.reason,
				cert_not_after = excluded.cert_not_after, cert_valid = excluded.cert_valid,
				down_since = CASE WHEN excluded.state = 'down' THEN COALESCE(site_uptime.down_since, excluded.down_since) ELSE NULL END`,
			result.domain, at, state, result.scheme, status, result.responseMS, result.reason, notAfter, valid, downSince); err != nil {
			return err
		}
	}
	query := `DELETE FROM site_uptime`
	if len(keep) > 0 {
		query += ` WHERE domain NOT IN (?` + strings.Repeat(`, ?`, len(keep)-1) + `)`
	}
	if _, err := tx.ExecContext(ctx, query, keep...); err != nil {
		return err
	}
	return tx.Commit()
}

type siteUptimeView struct {
	Domain       string `json:"domain"`
	CheckedAt    string `json:"checked_at"`
	State        string `json:"state"`
	Scheme       string `json:"scheme"`
	HTTPStatus   *int64 `json:"http_status,omitempty"`
	ResponseMS   *int64 `json:"response_ms,omitempty"`
	Reason       string `json:"reason,omitempty"`
	CertNotAfter string `json:"cert_not_after,omitempty"`
	CertDays     *int   `json:"cert_days,omitempty"`
	CertValid    *bool  `json:"cert_valid,omitempty"`
	DownSince    string `json:"down_since,omitempty"`
	Stale        bool   `json:"stale,omitempty"`
}

func readSiteUptime(ctx context.Context, db *sql.DB, now time.Time) ([]siteUptimeView, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT domain, checked_at, state, scheme, http_status, response_ms, reason, cert_not_after, cert_valid, down_since
		FROM site_uptime ORDER BY state = 'up', domain`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	views := []siteUptimeView{}
	for rows.Next() {
		var view siteUptimeView
		var status, ms sql.NullInt64
		var notAfter, downSince sql.NullString
		var valid sql.NullInt64
		if err := rows.Scan(&view.Domain, &view.CheckedAt, &view.State, &view.Scheme, &status, &ms, &view.Reason, &notAfter, &valid, &downSince); err != nil {
			return nil, err
		}
		if status.Valid {
			view.HTTPStatus = &status.Int64
		}
		if ms.Valid {
			view.ResponseMS = &ms.Int64
		}
		if notAfter.Valid {
			view.CertNotAfter = notAfter.String
			if at, err := time.Parse(time.RFC3339, notAfter.String); err == nil {
				days := int(at.Sub(now).Hours() / 24)
				if at.Before(now) {
					days = -1
				}
				view.CertDays = &days
			}
		}
		if valid.Valid {
			ok := valid.Int64 == 1
			view.CertValid = &ok
		}
		view.DownSince = downSince.String
		if at, err := time.Parse(time.RFC3339, view.CheckedAt); err != nil || now.Sub(at) > siteCheckStale {
			view.Stale = true
		}
		views = append(views, view)
	}
	return views, rows.Err()
}

// siteProblems: a site down at its last check, and a served certificate
// that expires within 14 days, has expired or does not carry the name, when
// the certificate records do not already say so.
func siteProblems(ctx context.Context, db *sql.DB, now time.Time, certificateProblems []monitoringProblem) ([]monitoringProblem, *monitoringUnknown) {
	views, err := readSiteUptime(ctx, db, now)
	if err != nil {
		return nil, &monitoringUnknown{Source: "sites", Reason: "unreadable"}
	}
	known := map[string]bool{}
	for _, problem := range certificateProblems {
		known[problem.Subject] = true
	}
	var problems []monitoringProblem
	stale := false
	for _, view := range views {
		if view.Stale {
			stale = true
			continue
		}
		open := monitoringProblemAction{Kind: "open", To: "/domains/" + url.PathEscape(view.Domain)}
		if view.State == "down" {
			problems = append(problems, monitoringProblem{
				Key: "site:" + view.Domain, Kind: "site_down", Severity: "critical", Actor: "administrator",
				Subject: view.Domain, Name: view.Reason, Since: view.DownSince, ObservedAt: view.CheckedAt, Action: open,
			})
			continue
		}
		if view.CertDays == nil || known[view.Domain] {
			continue
		}
		invalid := view.CertValid != nil && !*view.CertValid
		if !invalid && *view.CertDays >= problemCertificateDays {
			continue
		}
		severity := "warning"
		if *view.CertDays <= problemCertificateCrit || invalid {
			severity = "critical"
		}
		reason := "expiring"
		if invalid {
			reason = "invalid"
		}
		problems = append(problems, monitoringProblem{
			Key: "site-certificate:" + view.Domain, Kind: "site_certificate", Severity: severity, Actor: "administrator",
			Subject: view.Domain, Name: reason, Value: float64(*view.CertDays), ObservedAt: view.CheckedAt,
			Action: monitoringProblemAction{Kind: "open", To: "/domains/" + url.PathEscape(view.Domain) + "?tab=ssl"},
		})
	}
	if stale {
		return problems, &monitoringUnknown{Source: "sites", Reason: "checking"}
	}
	return problems, nil
}

// handleMonitoringSites: GET /api/v1/metrics/sites, the stored results.
func (p *Panel) handleMonitoringSites(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	views, err := readSiteUptime(r.Context(), p.db.GetDB(), time.Now())
	if err != nil {
		writeServerError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"enabled":       p.alertSettings(r.Context()).SiteChecks,
		"every_seconds": int(siteCheckEvery / time.Second),
		"sites":         views,
	})
}
