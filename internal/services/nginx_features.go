package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The installed nginx decides which form of a directive the Panel writes
// (2026-10-30). The distributions CelikPanel supports ship different nginx
// series from their own archives (owner rule: distribution packages only):
// Ubuntu 24.04 has 1.24.0, Debian 13 has 1.26.x, Arch follows nginx's own
// releases. `http2 on;` exists only from nginx 1.25.1; 1.24 refuses it
// (`[emerg] unknown directive "http2"`), which on a real Ubuntu 24.04 server
// left a site's certificate "activation pending": nginx -t refused the
// HTTPS vhost and the previous file was put back. Before 1.25.1 HTTP/2 is the
// `http2` parameter of `listen`; from 1.25.1 that parameter still works but
// is deprecated (a warning on every test and reload).
//
// Kurulu nginx, Panel'in hangi yönerge biçimini yazacağını belirler. Ubuntu
// 24.04 nginx 1.24.0 getirir ve `http2 on;` yönergesini tanımaz; Debian 13
// 1.26 getirir. 1.25.1 öncesinde HTTP/2, `listen` satırının `http2`
// parametresidir.

// HTTP/2 forms of an HTTPS server block.
const (
	// NginxHTTP2Directive: `http2 on;` in the server block (nginx >= 1.25.1).
	NginxHTTP2Directive = "directive"
	// NginxHTTP2Listen: `listen 443 ssl http2;` (nginx 1.9.5 to 1.25.0).
	NginxHTTP2Listen = "listen"
	// NginxHTTP2Off: no HTTP/2 (version unknown, or nginx built without
	// ngx_http_v2_module). Every nginx accepts it; visitors get HTTP/1.1.
	NginxHTTP2Off = ""
)

// NginxVersion is nginx's own release number (major.minor.patch).
type NginxVersion struct {
	Major, Minor, Patch int
}

func (v NginxVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// AtLeast compares release numbers.
func (v NginxVersion) AtLeast(major, minor, patch int) bool {
	if v.Major != major {
		return v.Major > major
	}
	if v.Minor != minor {
		return v.Minor > minor
	}
	return v.Patch >= patch
}

// NginxFeatures is what the Panel measured of the installed nginx.
type NginxFeatures struct {
	// Known is false when the version could not be read (nginx missing, the
	// command failed, or a server that does not name nginx's own release
	// number); every version-gated choice then takes its safe side.
	Known   bool
	Version NginxVersion
	// HTTP2Module: `nginx -V` lists --with-http_v2_module.
	HTTP2Module bool
}

// HTTP2Form is the HTTP/2 form an HTTPS server block of this nginx carries.
//
// Unknown version: no HTTP/2 at all. Both version-specific forms are refused
// or warned about by some nginx (`http2 on;` is an error before 1.25.1, the
// listen parameter is deprecated from 1.25.1 and may go), and a refused
// configuration takes the site's HTTPS away; HTTP/1.1 is accepted by every
// nginx and serves the site fully. The next render, once the version is
// readable, writes the HTTP/2 form.
//
// Sürüm bilinmiyorsa HTTP/2 yazılmaz: her nginx bunu kabul eder.
func (f NginxFeatures) HTTP2Form() string {
	if !f.Known || !f.HTTP2Module {
		return NginxHTTP2Off
	}
	if f.Version.AtLeast(1, 25, 1) {
		return NginxHTTP2Directive
	}
	if f.Version.AtLeast(1, 9, 5) {
		return NginxHTTP2Listen
	}
	return NginxHTTP2Off
}

// RejectHandshake says whether `ssl_reject_handshake` exists (nginx >=
// 1.19.4). Unknown: no.
func (f NginxFeatures) RejectHandshake() bool {
	return f.Known && f.Version.AtLeast(1, 19, 4)
}

// nginxVersionLine reads `nginx version: nginx/1.24.0 (Ubuntu)`. freenginx
// keeps nginx's numbering and directives; OpenResty's first three numbers are
// the nginx core it bundles (1.25.3.2 is nginx 1.25.3). Any other server name
// (Angie numbers its own releases) is not read as an nginx version.
var nginxVersionLine = regexp.MustCompile(`(?m)^nginx version: (?:nginx|freenginx|openresty)/(\d{1,4})\.(\d{1,4})\.(\d{1,4})\b`)

// ParseNginxFeatures reads the output of `nginx -V` (or `nginx -v`, which has
// only the version line: then no module list is known and HTTP/2 counts as
// built in, as in every distribution package).
func ParseNginxFeatures(output string) NginxFeatures {
	output = strings.ReplaceAll(output, "\r\n", "\n")
	match := nginxVersionLine.FindStringSubmatch(output)
	if match == nil {
		return NginxFeatures{}
	}
	numbers := make([]int, 3)
	for i := range numbers {
		value, err := strconv.Atoi(match[i+1])
		if err != nil {
			return NginxFeatures{}
		}
		numbers[i] = value
	}
	features := NginxFeatures{
		Known:   true,
		Version: NginxVersion{Major: numbers[0], Minor: numbers[1], Patch: numbers[2]},
	}
	if strings.Contains(output, "configure arguments:") {
		features.HTTP2Module = strings.Contains(output, "--with-http_v2_module")
	} else {
		features.HTTP2Module = true
	}
	return features
}

// The measurement is kept while the nginx binary is the same file (path,
// size, modification time): a package upgrade replaces the file and the next
// render measures again. A failed measurement is not kept.
var (
	nginxFeaturesMu    sync.Mutex
	nginxFeaturesKey   string
	nginxFeaturesCache NginxFeatures

	nginxFeaturesLookPath = exec.LookPath
	// The package's nginx launcher (nginx_command.go); `nginx -V` only
	// prints the version and build options, changes nothing.
	nginxFeaturesRun = func(ctx context.Context, binary string) ([]byte, error) {
		return executeNginxCommand(ctx, binary, "-V")
	}
)

const nginxFeaturesTimeout = 10 * time.Second

// HostNginxFeatures measures the installed nginx (`nginx -V`, root not
// needed). It never fails: an unreadable answer is NginxFeatures{} (unknown).
func HostNginxFeatures() NginxFeatures {
	binary, err := nginxFeaturesLookPath("nginx")
	if err != nil {
		for _, candidate := range []string{"/usr/sbin/nginx", "/usr/bin/nginx"} {
			if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() {
				binary, err = candidate, nil
				break
			}
		}
		if err != nil {
			return NginxFeatures{}
		}
	}
	info, err := os.Stat(binary)
	if err != nil {
		return NginxFeatures{}
	}
	key := fmt.Sprintf("%s|%d|%d", binary, info.Size(), info.ModTime().UnixNano())
	nginxFeaturesMu.Lock()
	defer nginxFeaturesMu.Unlock()
	if key == nginxFeaturesKey {
		return nginxFeaturesCache
	}
	ctx, cancel := context.WithTimeout(context.Background(), nginxFeaturesTimeout)
	defer cancel()
	output, err := nginxFeaturesRun(ctx, binary)
	if err != nil {
		return NginxFeatures{}
	}
	features := ParseNginxFeatures(string(output))
	if features.Known {
		nginxFeaturesKey, nginxFeaturesCache = key, features
	}
	return features
}

// features is the generator's measurement of the installed nginx (a test
// seam: nginxFeatures).
func (ng *NginxGenerator) features() NginxFeatures {
	if ng.nginxFeatures != nil {
		return ng.nginxFeatures()
	}
	return HostNginxFeatures()
}

// SetNginxFeatures fixes the generator's view of the installed nginx (tests
// and tools that render for another host).
func (ng *NginxGenerator) SetNginxFeatures(features NginxFeatures) {
	ng.nginxFeatures = func() NginxFeatures { return features }
}
