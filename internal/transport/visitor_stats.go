package transport

// Visitor statistics per site (alpha.128, G24): daily totals read from the
// site's own nginx access log (combined format), which nginx already
// writes. No analytics package, no tracking script, no new log. The Panel
// keeps each finished day as one row, so the history outlives log rotation.
//
// Site başına ziyaretçi istatistikleri: nginx'in zaten yazdığı erişim
// günlüğünden günlük toplamlar; ek paket ya da izleme betiği yok.

// VisitorStatsRequest asks for the days from Since (YYYY-MM-DD, the server's
// local date) up to today.
type VisitorStatsRequest struct {
	Domain string `json:"domain"`
	Since  string `json:"since"`
}

// VisitorStatsCount is one ranked item (a page path or a referring host).
type VisitorStatsCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// VisitorStatsDay is one day's totals. Visitors are distinct addresses among
// page views of browsers (bots apart); addresses are never kept or sent.
type VisitorStatsDay struct {
	Day          string              `json:"day"`
	Hits         int64               `json:"hits"`
	PageViews    int64               `json:"page_views"`
	Visitors     int64               `json:"visitors"`
	Bots         int64               `json:"bots"`
	BytesSent    int64               `json:"bytes_sent"`
	Status2xx    int64               `json:"status_2xx"`
	Status3xx    int64               `json:"status_3xx"`
	Status4xx    int64               `json:"status_4xx"`
	Status5xx    int64               `json:"status_5xx"`
	TopPages     []VisitorStatsCount `json:"top_pages"`
	TopReferrers []VisitorStatsCount `json:"top_referrers"`
}

type VisitorStatsResponse struct {
	// Today is the server's local date: days before it are complete.
	Today string            `json:"today"`
	Days  []VisitorStatsDay `json:"days"`
	// Partial: the read stopped at its size or time bound, so the oldest
	// day may be incomplete (the Panel then does not keep it).
	Partial bool   `json:"partial,omitempty"`
	Oldest  string `json:"oldest,omitempty"`
	Error   string `json:"error,omitempty"`
}
