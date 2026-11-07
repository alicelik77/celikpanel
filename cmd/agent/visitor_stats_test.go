package main

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

func TestParseCombinedLine(t *testing.T) {
	line := `203.0.113.9 - - [06/Nov/2026:21:05:01 +0300] "GET /shop/item?id=5&token=SECRET HTTP/1.1" 200 5120 "https://www.google.com/search?q=x" "Mozilla/5.0 (X11; Linux x86_64)"`
	got, err := parseCombinedLine(line)
	if err != nil || got.day != "2026-11-06" || got.status != 200 || got.bytes != 5120 || got.method != "GET" || got.addr != "203.0.113.9" {
		t.Fatalf("parsed %+v %v", got, err)
	}
	if path := visitorPagePath(got.path); path != "/shop/item" {
		t.Fatalf("page path %q (the query must go)", path)
	}
	if host := visitorReferrerHost(got.referrer, "example.com"); host != "google.com" {
		t.Fatalf("referrer host %q", host)
	}
	if host := visitorReferrerHost("https://shop.example.com/a", "example.com"); host != "" {
		t.Fatalf("the site's own host counted as a referrer: %q", host)
	}
	if visitorPagePath("/static/app.js") != "" || visitorPagePath("/logo.PNG") != "" {
		t.Fatal("an asset counted as a page")
	}
	if _, err := parseCombinedLine("garbage"); err == nil {
		t.Fatal("garbage parsed")
	}
}

func TestCollectVisitorStatsReadsRotationsAndKeepsNoAddresses(t *testing.T) {
	dir := t.TempDir()
	oldDir, oldNow := visitorStatsLogDir, visitorStatsNow
	visitorStatsLogDir = dir
	visitorStatsNow = func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.Local) }
	t.Cleanup(func() { visitorStatsLogDir, visitorStatsNow = oldDir, oldNow })
	browser := `"Mozilla/5.0"`
	line := func(addr, day, path string, status int, agent string) string {
		return addr + ` - - [` + day + `:10:00:00 +0000] "GET ` + path + ` HTTP/1.1" ` + strconv.Itoa(status) + ` 100 "-" ` + agent + "\n"
	}
	current := line("198.51.100.1", "07/Nov/2026", "/", 200, browser) +
		line("198.51.100.2", "07/Nov/2026", "/about", 200, browser) +
		line("198.51.100.1", "07/Nov/2026", "/style.css", 200, browser) +
		line("198.51.100.3", "07/Nov/2026", "/", 200, `"Googlebot/2.1"`)
	previous := line("198.51.100.1", "06/Nov/2026", "/", 200, browser) + line("198.51.100.1", "06/Nov/2026", "/x", 404, browser)
	older := line("198.51.100.9", "05/Nov/2026", "/old", 200, browser)
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("example.com-access.log", current)
	write("example.com-access.log.1", previous)
	f, _ := os.Create(filepath.Join(dir, "example.com-access.log.2.gz"))
	gz := gzip.NewWriter(f)
	_, _ = gz.Write([]byte(older))
	_ = gz.Close()
	_ = f.Close()

	resp := collectVisitorStats(transport.VisitorStatsRequest{Domain: "example.com", Since: "2026-11-05"})
	if resp.Error != "" || resp.Today != "2026-11-07" || len(resp.Days) != 3 {
		t.Fatalf("resp = %+v", resp)
	}
	today := resp.Days[2]
	if today.Hits != 4 || today.PageViews != 2 || today.Visitors != 2 || today.Bots != 1 {
		t.Fatalf("today = %+v", today)
	}
	if resp.Days[1].Status4xx != 1 || resp.Days[0].TopPages[0].Name != "/old" {
		t.Fatalf("days = %+v", resp.Days)
	}
	for _, day := range resp.Days {
		for _, item := range append(day.TopPages, day.TopReferrers...) {
			if strings.Contains(item.Name, "198.51.100") {
				t.Fatal("an address left the Agent")
			}
		}
	}
	// Days already held by the Panel are not read again.
	resp = collectVisitorStats(transport.VisitorStatsRequest{Domain: "example.com", Since: "2026-11-07"})
	if len(resp.Days) != 1 {
		t.Fatalf("since today: %+v", resp.Days)
	}
	if bad := collectVisitorStats(transport.VisitorStatsRequest{Domain: "../etc"}); bad.Error == "" {
		t.Fatal("a path-like domain was accepted")
	}
}
