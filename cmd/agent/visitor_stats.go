package main

import (
	"bufio"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Visitor statistics per site (alpha.128, G24). The site's own nginx access
// log (/var/log/nginx/<domain>-access.log, combined format, rotated by the
// system's logrotate) is read on request, newest file first, back to the
// first day the Panel does not hold yet. Only totals leave the Agent:
// visitor addresses are counted in memory and never kept or sent, page
// paths lose their query string, and a referrer is reduced to its host, so
// no token in a URL reaches a screen or the Panel's database.
//
// Bounds: one call reads at most visitorStatsMaxBytes and runs for at most
// visitorStatsMaxTime; a plain log larger than visitorStatsTailBytes is read
// from its last visitorStatsTailBytes. A read that stops early says so
// (Partial, Oldest), and the Panel then keeps only the days after Oldest.
//
// Ziyaretçi istatistikleri: sitenin kendi erişim günlüğü istek üzerine okunur;
// yalnız toplamlar çıkar, adresler saklanmaz, yollar sorgu dizgesiz,
// yönlendiren yalnız alan adıyla.

const (
	visitorStatsMaxBytes     = int64(1) << 30
	visitorStatsTailBytes    = int64(256) << 20
	visitorStatsMaxTime      = 25 * time.Second
	visitorStatsMaxFiles     = 15
	visitorStatsMaxDays      = 62
	visitorStatsTop          = 10
	visitorStatsMaxPaths     = 5000
	visitorStatsMaxVisitors  = 200000
	visitorStatsMaxPathBytes = 200
)

var (
	visitorStatsLogDir      = "/var/log/nginx"
	visitorStatsNow         = time.Now
	visitorStatsDomainRe    = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)
	visitorStatsStaticExt   = map[string]bool{".css": true, ".js": true, ".mjs": true, ".map": true, ".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true, ".ico": true, ".webp": true, ".avif": true, ".woff": true, ".woff2": true, ".ttf": true, ".eot": true, ".otf": true, ".mp4": true, ".webm": true, ".mp3": true, ".json": true, ".xml": true, ".txt": true}
	visitorStatsBotMarkers  = []string{"bot", "crawl", "spider", "slurp", "facebookexternalhit", "headless", "curl/", "wget/", "python-requests", "go-http-client"}
	errVisitorStatsLineSkip = errors.New("not a combined log line")
)

type visitorDay struct {
	totals    transport.VisitorStatsDay
	visitors  map[string]struct{}
	pages     map[string]int64
	referrers map[string]int64
}

type visitorLine struct {
	addr, day, method, path, referrer, agent string
	status                                   int
	bytes                                    int64
}

// parseCombinedLine reads one line of nginx's combined format:
// addr - user [02/Jan/2006:15:04:05 -0700] "GET /path HTTP/1.1" 200 123 "ref" "agent"
// nginx escapes a quote inside a field (\x22), so a quoted field has none.
func parseCombinedLine(line string) (visitorLine, error) {
	var out visitorLine
	space := strings.IndexByte(line, ' ')
	open := strings.IndexByte(line, '[')
	if space <= 0 || open < space {
		return out, errVisitorStatsLineSkip
	}
	out.addr = line[:space]
	closing := strings.IndexByte(line[open:], ']')
	if closing < 12 {
		return out, errVisitorStatsLineSkip
	}
	stamp := line[open+1 : open+closing]
	day, err := time.Parse("02/Jan/2006", stamp[:min(11, len(stamp))])
	if err != nil {
		return out, errVisitorStatsLineSkip
	}
	out.day = day.Format("2006-01-02")
	rest := line[open+closing+1:]
	fields := []string{}
	for len(fields) < 5 {
		rest = strings.TrimLeft(rest, " ")
		if rest == "" {
			break
		}
		if rest[0] == '"' {
			end := strings.IndexByte(rest[1:], '"')
			if end < 0 {
				return out, errVisitorStatsLineSkip
			}
			fields = append(fields, rest[1:1+end])
			rest = rest[end+2:]
			continue
		}
		end := strings.IndexByte(rest, ' ')
		if end < 0 {
			end = len(rest)
		}
		fields = append(fields, rest[:end])
		rest = rest[end:]
	}
	if len(fields) < 3 {
		return out, errVisitorStatsLineSkip
	}
	request := strings.Fields(fields[0])
	if len(request) >= 2 {
		out.method, out.path = request[0], request[1]
	}
	if out.status, err = strconv.Atoi(fields[1]); err != nil || out.status < 100 || out.status > 599 {
		return out, errVisitorStatsLineSkip
	}
	out.bytes, _ = strconv.ParseInt(fields[2], 10, 64)
	if len(fields) > 3 {
		out.referrer = fields[3]
	}
	if len(fields) > 4 {
		out.agent = fields[4]
	}
	return out, nil
}

func visitorIsBot(agent string) bool {
	lowered := strings.ToLower(agent)
	if lowered == "" || lowered == "-" {
		return true
	}
	for _, marker := range visitorStatsBotMarkers {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return false
}

// visitorPagePath is the path of a page view, without its query; "" when the
// request is not a page (an asset, or not a path).
func visitorPagePath(path string) string {
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	if path == "" || path[0] != '/' {
		return ""
	}
	last := path[strings.LastIndexByte(path, '/')+1:]
	if dot := strings.LastIndexByte(last, '.'); dot >= 0 && visitorStatsStaticExt[strings.ToLower(last[dot:])] {
		return ""
	}
	if len(path) > visitorStatsMaxPathBytes {
		path = path[:visitorStatsMaxPathBytes]
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, path)
}

// visitorReferrerHost is the referring site's host, "" for none or the site
// itself.
func visitorReferrerHost(referrer, domain string) string {
	lowered := strings.ToLower(referrer)
	for _, scheme := range []string{"https://", "http://"} {
		if strings.HasPrefix(lowered, scheme) {
			host := lowered[len(scheme):]
			if i := strings.IndexAny(host, "/?#:"); i >= 0 {
				host = host[:i]
			}
			host = strings.TrimPrefix(host, "www.")
			if host == "" || host == domain || strings.HasSuffix(host, "."+domain) || !visitorStatsDomainRe.MatchString(host) {
				return ""
			}
			return host
		}
	}
	return ""
}

func (d *visitorDay) add(line visitorLine, domain string) {
	t := &d.totals
	t.Hits++
	t.BytesSent += line.bytes
	switch line.status / 100 {
	case 2:
		t.Status2xx++
	case 3:
		t.Status3xx++
	case 4:
		t.Status4xx++
	case 5:
		t.Status5xx++
	}
	if visitorIsBot(line.agent) {
		t.Bots++
		return
	}
	if line.method != "GET" || line.status < 200 || line.status > 299 {
		return
	}
	path := visitorPagePath(line.path)
	if path == "" {
		return
	}
	t.PageViews++
	if len(d.visitors) < visitorStatsMaxVisitors {
		d.visitors[line.addr] = struct{}{}
	}
	if _, ok := d.pages[path]; ok || len(d.pages) < visitorStatsMaxPaths {
		d.pages[path]++
	}
	if host := visitorReferrerHost(line.referrer, domain); host != "" {
		if _, ok := d.referrers[host]; ok || len(d.referrers) < visitorStatsMaxPaths {
			d.referrers[host]++
		}
	}
}

func topCounts(counts map[string]int64) []transport.VisitorStatsCount {
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
	if len(list) > visitorStatsTop {
		list = list[:visitorStatsTop]
	}
	return list
}

// visitorStatsFiles is the log and its rotations, newest first.
func visitorStatsFiles(domain string) []string {
	base := visitorStatsLogDir + "/" + domain + "-access.log"
	files := []string{base, base + ".1"}
	for i := 2; i < visitorStatsMaxFiles; i++ {
		files = append(files, base+"."+strconv.Itoa(i)+".gz")
	}
	return files
}

// SiteVisitorStatsV1 reads the days from req.Since to today.
func (a *Agent) SiteVisitorStatsV1(req transport.VisitorStatsRequest, resp *transport.VisitorStatsResponse) error {
	*resp = collectVisitorStats(req)
	return nil
}

func collectVisitorStats(req transport.VisitorStatsRequest) transport.VisitorStatsResponse {
	now := visitorStatsNow()
	resp := transport.VisitorStatsResponse{Today: now.Format("2006-01-02"), Days: []transport.VisitorStatsDay{}}
	domain := strings.ToLower(strings.TrimSpace(req.Domain))
	if !visitorStatsDomainRe.MatchString(domain) {
		resp.Error = "invalid domain"
		return resp
	}
	floor := now.AddDate(0, 0, -visitorStatsMaxDays).Format("2006-01-02")
	since := req.Since
	if _, err := time.Parse("2006-01-02", since); err != nil || since < floor {
		since = floor
	}
	days := map[string]*visitorDay{}
	started := time.Now()
	var read int64
	for _, path := range visitorStatsFiles(domain) {
		if read >= visitorStatsMaxBytes || time.Since(started) > visitorStatsMaxTime {
			resp.Partial = true
			break
		}
		oldest, newest, n, cut, err := readVisitorLog(path, domain, since, days, visitorStatsMaxBytes-read, started)
		read += n
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if cut {
			// Days up to this file's newest may be incomplete.
			resp.Partial = true
			resp.Oldest = newest
			break
		}
		if err != nil {
			// A damaged rotation: what came before it is complete.
			resp.Partial = true
			resp.Oldest = newest
			break
		}
		if oldest != "" && oldest < since {
			break
		}
	}
	names := make([]string, 0, len(days))
	for day := range days {
		names = append(names, day)
	}
	sort.Strings(names)
	for _, name := range names {
		d := days[name]
		d.totals.Day = name
		d.totals.Visitors = int64(len(d.visitors))
		d.totals.TopPages = topCounts(d.pages)
		d.totals.TopReferrers = topCounts(d.referrers)
		resp.Days = append(resp.Days, d.totals)
	}
	return resp
}

// readVisitorLog adds one file's lines on or after since. It answers the
// oldest and newest day the file holds, the bytes read, and cut when the
// file was not read to its end (a bound) or only its tail was read.
func readVisitorLog(path, domain, since string, days map[string]*visitorDay, budget int64, started time.Time) (string, string, int64, bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", "", 0, false, err
	}
	if !info.Mode().IsRegular() {
		return "", "", 0, false, os.ErrNotExist
	}
	file, err := os.Open(path)
	if err != nil {
		return "", "", 0, false, err
	}
	defer file.Close()
	var reader io.Reader = file
	cut := false
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(file)
		if err != nil {
			return "", "", 0, false, err
		}
		defer gz.Close()
		reader = gz
	} else if info.Size() > visitorStatsTailBytes {
		if _, err := file.Seek(info.Size()-visitorStatsTailBytes, io.SeekStart); err != nil {
			return "", "", 0, false, err
		}
		cut = true
	}
	counter := &countingReader{r: io.LimitReader(reader, budget)}
	scanner := bufio.NewScanner(counter)
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	oldest, newest := "", ""
	first := true
	lines := 0
	for scanner.Scan() {
		if first && cut {
			first = false
			continue // the partial line after the seek
		}
		first = false
		lines++
		if lines%4096 == 0 && time.Since(started) > visitorStatsMaxTime {
			return oldest, newest, counter.n, true, nil
		}
		line, err := parseCombinedLine(scanner.Text())
		if err != nil {
			continue
		}
		if oldest == "" || line.day < oldest {
			oldest = line.day
		}
		if line.day > newest {
			newest = line.day
		}
		if line.day < since {
			continue
		}
		d := days[line.day]
		if d == nil {
			if len(days) > visitorStatsMaxDays+2 {
				continue
			}
			d = &visitorDay{visitors: map[string]struct{}{}, pages: map[string]int64{}, referrers: map[string]int64{}}
			days[line.day] = d
		}
		d.add(line, domain)
	}
	if err := scanner.Err(); err != nil {
		return oldest, newest, counter.n, true, nil
	}
	if counter.n >= budget {
		cut = true
	}
	if cut && oldest != "" {
		// Only the tail was read: its first day is incomplete.
		return oldest, oldest, counter.n, true, nil
	}
	return oldest, newest, counter.n, false, nil
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
