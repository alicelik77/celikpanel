package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Visitor statistics per site (alpha.128, G24). GET
// /api/v1/domains/{id}/stats/visitors answers the last 30 days: finished days
// from site_visitor_stats_daily, today read live. When the screen asks and
// the last read is older than five minutes, the Agent reads the site's own
// access log from the first day not stored yet (cmd/agent/visitor_stats.go)
// and the finished days are kept, so the history outlives log rotation.
// Reading never changes the site; a failed read leaves the stored days and
// says so.
//
// Site başına ziyaretçi istatistikleri: biten günler tablodan, bugün canlı.

const (
	visitorStatsFresh  = 5 * time.Minute
	visitorStatsWindow = 30
)

func init() {
	teamMemberDomainRequirements[teamMemberRouteKey{kind: "visitor-stats", method: http.MethodGet}] =
		teamMemberDomainRequirement{capability: core.TeamCapabilityStatistics, mode: core.TeamPermissionView}
}

var visitorStatsCache = struct {
	sync.Mutex
	entries map[int]visitorStatsCacheEntry
}{entries: map[int]visitorStatsCacheEntry{}}

type visitorStatsCacheEntry struct {
	at      time.Time
	today   *transport.VisitorStatsDay
	partial bool
	failed  bool
}

type visitorStatsDayJSON struct {
	Day       string `json:"day"`
	Hits      int64  `json:"hits"`
	PageViews int64  `json:"page_views"`
	Visitors  int64  `json:"visitors"`
	Bots      int64  `json:"bots"`
	BytesSent int64  `json:"bytes_sent"`
	Status4xx int64  `json:"status_4xx"`
	Status5xx int64  `json:"status_5xx"`
}

func (p *Panel) handleVisitorStats(w http.ResponseWriter, r *http.Request, domainID int) {
	w.Header().Set("Content-Type", "application/json")
	ctx := r.Context()
	var domainName string
	if err := p.db.GetDB().QueryRowContext(ctx, "SELECT name FROM domains WHERE id = ?", domainID).Scan(&domainName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeClientError(w, http.StatusNotFound, "domain not found")
		} else {
			writeServerError(w, err)
		}
		return
	}
	now := time.Now()
	todayKey := now.Format("2006-01-02")
	from := now.AddDate(0, 0, -visitorStatsWindow).Format("2006-01-02")

	visitorStatsCache.Lock()
	entry, cached := visitorStatsCache.entries[domainID]
	visitorStatsCache.Unlock()
	if !cached || time.Since(entry.at) > visitorStatsFresh {
		entry = p.refreshVisitorStats(ctx, domainID, domainName, from)
		visitorStatsCache.Lock()
		visitorStatsCache.entries[domainID] = entry
		visitorStatsCache.Unlock()
	}

	days, pages, referrers, err := p.storedVisitorStats(ctx, domainID, from, todayKey)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if entry.today != nil && entry.today.Day == todayKey {
		days = append(days, visitorStatsDayJSON{Day: todayKey, Hits: entry.today.Hits, PageViews: entry.today.PageViews,
			Visitors: entry.today.Visitors, Bots: entry.today.Bots, BytesSent: entry.today.BytesSent,
			Status4xx: entry.today.Status4xx, Status5xx: entry.today.Status5xx})
		weekStart := now.AddDate(0, 0, -6).Format("2006-01-02")
		if todayKey >= weekStart {
			for _, item := range entry.today.TopPages {
				pages[item.Name] += item.Count
			}
			for _, item := range entry.today.TopReferrers {
				referrers[item.Name] += item.Count
			}
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"today":         todayKey,
		"days":          days,
		"top_pages":     rankVisitorCounts(pages),
		"top_referrers": rankVisitorCounts(referrers),
		"read_at":       entry.at.UTC().Format(time.RFC3339),
		"read_failed":   entry.failed,
		"partial":       entry.partial,
	})
}

// refreshVisitorStats asks the Agent for the days not stored yet and keeps
// the finished ones.
func (p *Panel) refreshVisitorStats(ctx context.Context, domainID int, domainName, from string) visitorStatsCacheEntry {
	entry := visitorStatsCacheEntry{at: time.Now()}
	since := from
	var last sql.NullString
	if err := p.db.GetDB().QueryRowContext(ctx, "SELECT MAX(day) FROM site_visitor_stats_daily WHERE domain_id = ?", domainID).Scan(&last); err == nil && last.Valid {
		if day, err := time.Parse("2006-01-02", last.String); err == nil {
			if next := day.AddDate(0, 0, 1).Format("2006-01-02"); next > since {
				since = next
			}
		}
	}
	var resp transport.VisitorStatsResponse
	if err := p.callAgentContext(ctx, "Agent.SiteVisitorStatsV1", transport.VisitorStatsRequest{Domain: domainName, Since: since}, &resp); err != nil || resp.Error != "" {
		entry.failed = true
		return entry
	}
	entry.partial = resp.Partial
	stamp := time.Now().UTC().Format(time.RFC3339)
	for i := range resp.Days {
		day := resp.Days[i]
		if day.Day == resp.Today {
			entry.today = &resp.Days[i]
			continue
		}
		if day.Day > resp.Today || day.Day < since || (resp.Partial && day.Day <= resp.Oldest) {
			continue
		}
		pages, _ := json.Marshal(day.TopPages)
		referrers, _ := json.Marshal(day.TopReferrers)
		if _, err := p.db.GetDB().ExecContext(ctx, `
			INSERT INTO site_visitor_stats_daily (domain_id, day, hits, page_views, visitors, bots, bytes_sent,
				status_2xx, status_3xx, status_4xx, status_5xx, top_pages, top_referrers, computed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (domain_id, day) DO NOTHING`,
			domainID, day.Day, day.Hits, day.PageViews, day.Visitors, day.Bots, day.BytesSent,
			day.Status2xx, day.Status3xx, day.Status4xx, day.Status5xx, string(pages), string(referrers), stamp); err != nil {
			log.Printf("visitor statistics: keep %s of domain %d: %v", day.Day, domainID, err)
		}
	}
	if entry.today == nil {
		entry.today = &transport.VisitorStatsDay{Day: resp.Today}
	}
	return entry
}

// storedVisitorStats reads the finished days and the last seven days' ranked
// pages and referrers.
func (p *Panel) storedVisitorStats(ctx context.Context, domainID int, from, today string) ([]visitorStatsDayJSON, map[string]int64, map[string]int64, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT day, hits, page_views, visitors, bots, bytes_sent, status_4xx, status_5xx, top_pages, top_referrers
		FROM site_visitor_stats_daily WHERE domain_id = ? AND day >= ? AND day < ? ORDER BY day`, domainID, from, today)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	weekStart := ""
	if t, err := time.Parse("2006-01-02", today); err == nil {
		weekStart = t.AddDate(0, 0, -6).Format("2006-01-02")
	}
	days := []visitorStatsDayJSON{}
	pages, referrers := map[string]int64{}, map[string]int64{}
	for rows.Next() {
		var d visitorStatsDayJSON
		var topPages, topReferrers string
		if err := rows.Scan(&d.Day, &d.Hits, &d.PageViews, &d.Visitors, &d.Bots, &d.BytesSent, &d.Status4xx, &d.Status5xx, &topPages, &topReferrers); err != nil {
			return nil, nil, nil, err
		}
		days = append(days, d)
		if d.Day >= weekStart {
			for _, pair := range []struct {
				raw  string
				into map[string]int64
			}{{topPages, pages}, {topReferrers, referrers}} {
				var items []transport.VisitorStatsCount
				if json.Unmarshal([]byte(pair.raw), &items) == nil {
					for _, item := range items {
						pair.into[item.Name] += item.Count
					}
				}
			}
		}
	}
	return days, pages, referrers, rows.Err()
}

func rankVisitorCounts(counts map[string]int64) []transport.VisitorStatsCount {
	list := make([]transport.VisitorStatsCount, 0, len(counts))
	for name, count := range counts {
		list = append(list, transport.VisitorStatsCount{Name: name, Count: count})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Count != list[j].Count {
			return list[i].Count > list[j].Count
		}
		return list[i].Name < list[j].Name
	})
	if len(list) > 10 {
		list = list[:10]
	}
	return list
}
