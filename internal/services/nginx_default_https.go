package services

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"text/template"

	"github.com/alicelik/celikpanel/internal/transport"
)

// CelikPanel's default HTTPS server (2026-10-30).
//
// nginx answers an HTTPS visitor whose name no server block carries (a name
// that points here but has no site, the server's bare IP address, a client
// without SNI) from the default server of port 443. Without one of its own,
// that is the first HTTPS server nginx read, so the visitor was handed
// another site's certificate: it names that site, and a browser shows a
// certificate warning for a site the visitor never asked for. CelikPanel's
// default server refuses the TLS handshake instead (`ssl_reject_handshake`,
// nginx >= 1.19.4), so no certificate is shown to a name it does not belong
// to.
//
// The file is plain nginx configuration in nginx's own drop-in directory,
// /etc/nginx/conf.d/celikpanel-default-https.conf, and keeps working when the
// Panel is stopped or removed (D-022). It is CelikPanel's own file with the
// render header (D-031): CelikPanel writes it only while it is absent or
// still CelikPanel's unchanged text, and never writes over or removes a file
// the owner changed. It is there only while nginx serves HTTPS anyway (some
// other server block listens on port 443), so it never makes nginx take a
// port that something else may hold; and only while no other server block
// already is the default server of port 443 (the owner's own choice wins,
// and two default servers would make nginx refuse its configuration).
//
// CelikPanel'in varsayılan HTTPS sunucusu: sertifikası olmayan bir adla gelen
// ziyaretçiye başka bir sitenin sertifikası gösterilmez, TLS el sıkışması
// reddedilir. Düz nginx yapılandırmasıdır; panel olmadan da çalışır. Sahibin
// değiştirdiği dosyaya dokunulmaz; sahibin kendi varsayılan 443 sunucusu
// varsa bu dosya yazılmaz.

const defaultHTTPSServerFileName = "celikpanel-default-https.conf"

// DefaultHTTPSServerPath is the file of CelikPanel's default HTTPS server.
func DefaultHTTPSServerPath() string {
	return filepath.Join(nginxBase(), "conf.d", defaultHTTPSServerFileName)
}

// Outcomes of ReconcileDefaultHTTPSServer.
const (
	DefaultHTTPSWritten     = "written"
	DefaultHTTPSUnchanged   = "unchanged"
	DefaultHTTPSRemoved     = "removed"
	DefaultHTTPSNotNeeded   = "not_needed"    // nothing serves port 443
	DefaultHTTPSOwnerHasOne = "owner_default" // another block is the default of 443
	DefaultHTTPSKept        = "kept"          // the file is not CelikPanel's unchanged text
	DefaultHTTPSUnsupported = "unsupported"   // nginx older than 1.19.4, or its version unknown
	DefaultHTTPSUnknown     = "unknown"       // nginx -T could not be read
	DefaultHTTPSFailed      = "failed"        // nginx refused; the previous state was put back
)

// DefaultHTTPSServerResult says what the reconciliation found and did.
type DefaultHTTPSServerResult struct {
	Outcome string
	Path    string
	// Detail is the reason in a sentence for the Agent's journal.
	Detail string
}

const defaultHTTPSServerTemplate = `# CelikPanel's default HTTPS server, written by CelikPanel.
# A visitor who asks this server over HTTPS for a name that has no site here
# (or for its bare IP address) has the TLS handshake refused, instead of being
# shown another site's certificate. Plain nginx configuration: it keeps
# working without CelikPanel. CelikPanel writes this file only while it is
# CelikPanel's unchanged text, and only while no other server block is the
# default server of port 443. To use your own default HTTPS server, give it
# "default_server" and delete this file. To turn this off, replace this
# file's contents with a comment of your own: CelikPanel never writes over a
# file it did not write.
# CelikPanel'in varsayılan HTTPS sunucusu: burada sitesi olmayan bir adla
# gelen ziyaretçiye başka sitenin sertifikası gösterilmez, el sıkışma
# reddedilir. Kapatmak için içeriğini kendi açıklamanızla değiştirin.
server {
{{- if eq .HTTP2 "listen"}}
    listen 443 ssl http2 default_server;
    listen [::]:443 ssl http2 default_server;
{{- else}}
    listen 443 ssl default_server;
    listen [::]:443 ssl default_server;
{{- end}}
{{- if eq .HTTP2 "directive"}}
    http2 on;
{{- end}}
    server_name _;

    ssl_reject_handshake on;
    # nginx negotiates with the default server's settings before it knows the
    # name asked for; these are the sites' own (Mozilla intermediate).
    ssl_session_cache shared:SSL:10m;
{{.TLSPolicy}}
}
`

var (
	defaultHTTPSServerParsed = template.Must(template.New("default-https").Parse(defaultHTTPSServerTemplate))

	tlsPolicyOnce sync.Once
	tlsPolicyText string
	tlsPolicyErr  error
)

// NginxTLSPolicy is the vhost template's "tls-policy" lines (protocols,
// ciphers, order, session timeout and tickets), for the other HTTPS server
// blocks CelikPanel writes: one policy, one source.
func NginxTLSPolicy() (string, error) {
	tlsPolicyOnce.Do(func() {
		parsed, err := template.New("vhost").Parse(vhostTemplate)
		if err != nil {
			tlsPolicyErr = err
			return
		}
		var buf bytes.Buffer
		if err := parsed.ExecuteTemplate(&buf, "tls-policy", nil); err != nil {
			tlsPolicyErr = err
			return
		}
		tlsPolicyText = strings.TrimPrefix(buf.String(), "\n")
	})
	return tlsPolicyText, tlsPolicyErr
}

// RenderDefaultHTTPSServer is the body of DefaultHTTPSServerPath (without the
// render header) for this nginx.
func RenderDefaultHTTPSServer(features NginxFeatures) (string, error) {
	policy, err := NginxTLSPolicy()
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = defaultHTTPSServerParsed.Execute(&buf, struct {
		HTTP2     string
		TLSPolicy string
	}{features.HTTP2Form(), policy})
	return buf.String(), err
}

var nginxDumpFileHeader = regexp.MustCompile(`(?m)^# configuration file (\S+):$`)

// httpsListenFacts reads `nginx -T` outside ownPath: whether some server
// block serves HTTPS on port 443 (`listen … 443 … ssl`; a stream proxy's
// plain `listen 443;` does not count), and the first one that makes itself
// the default server of port 443 on a wildcard address (the address
// CelikPanel's default server listens on).
func httpsListenFacts(dump, ownPath string) (serves443 bool, ownerDefault string) {
	indexes := nginxDumpFileHeader.FindAllStringSubmatchIndex(dump, -1)
	for i, index := range indexes {
		file := dump[index[2]:index[3]]
		end := len(dump)
		if i+1 < len(indexes) {
			end = indexes[i+1][0]
		}
		if filepath.Clean(file) == filepath.Clean(ownPath) {
			continue
		}
		for _, block := range nginxServerBlocks(dump[index[1]:end]) {
			for _, listen := range block.listens {
				if len(listen) == 0 {
					continue
				}
				address, port := splitNginxListen(listen[0])
				if port != "443" {
					continue
				}
				wildcard := address == "" || address == "*" || address == "0.0.0.0" || address == "::"
				for _, parameter := range listen[1:] {
					switch parameter {
					case "ssl":
						serves443 = true
					case "default_server", "default":
						if wildcard && ownerDefault == "" {
							ownerDefault = file
						}
					}
				}
			}
		}
	}
	return serves443, ownerDefault
}

// splitNginxListen splits a listen directive's first argument into its
// address ("" for a bare port; "::" for [::]) and port ("80" when absent;
// "" for a unix socket).
func splitNginxListen(value string) (address, port string) {
	switch {
	case strings.HasPrefix(value, "unix:"):
		return "", ""
	case strings.HasPrefix(value, "["):
		end := strings.IndexByte(value, ']')
		if end < 0 {
			return "", ""
		}
		address, port = value[1:end], "80"
		if strings.HasPrefix(value[end+1:], ":") {
			port = value[end+2:]
		}
		return address, port
	case strings.Trim(value, "0123456789") == "":
		return "", value
	case strings.Contains(value, ":"):
		cut := strings.LastIndexByte(value, ':')
		return value[:cut], value[cut+1:]
	default:
		return value, "80"
	}
}

func (ng *NginxGenerator) dumpNginx() (string, error) {
	if ng.nginxDump != nil {
		return ng.nginxDump()
	}
	if nginxDevMode() {
		return "", errors.New("development mode: the files are not part of the running nginx")
	}
	output, err := runNginxCommand("nginx", "-T")
	if err != nil {
		return "", nginxValidationError(output, err)
	}
	return string(output), nil
}

// ReconcileDefaultHTTPSServer puts CelikPanel's default HTTPS server in place
// when nginx serves port 443 and nothing else is its default, and removes
// CelikPanel's own unchanged file when it is no longer wanted. It reads the
// configuration nginx runs (`nginx -T`); a change is tested (`nginx -t`) and
// reloaded once, and a refusal puts the previous file back. It never touches
// a file that is not CelikPanel's unchanged text. Its failure never undoes
// the operation that called it: the answer says what happened.
//
// Varsayılan HTTPS sunucusunu gerektiğinde yazar, gerekmediğinde kendi
// değişmemiş dosyasını kaldırır; sahibin dosyasına dokunmaz.
func (ng *NginxGenerator) ReconcileDefaultHTTPSServer() DefaultHTTPSServerResult {
	nginxMutationMu.Lock()
	defer nginxMutationMu.Unlock()

	path := DefaultHTTPSServerPath()
	result := DefaultHTTPSServerResult{Path: path}
	inspected := inspectSiteFile(path, nil, "")
	switch inspected.state {
	case transport.SiteFileAbsent, transport.SiteFileManagedUnchanged:
	default:
		result.Outcome = DefaultHTTPSKept
		result.Detail = fmt.Sprintf("%s is not CelikPanel's unchanged text (%s); it is left as it is", path, inspected.state)
		return result
	}

	features := ng.features()
	if !features.RejectHandshake() {
		result.Outcome = DefaultHTTPSUnsupported
		if features.Known {
			result.Detail = fmt.Sprintf("nginx %s has no ssl_reject_handshake (1.19.4 and later)", features.Version)
		} else {
			result.Detail = "the nginx version could not be read"
		}
		return result
	}

	dump, err := ng.dumpNginx()
	if err != nil {
		result.Outcome = DefaultHTTPSUnknown
		result.Detail = boundedSiteFileDetail("nginx -T: " + err.Error())
		return result
	}
	serves443, ownerDefault := httpsListenFacts(dump, path)

	want := serves443 && ownerDefault == ""
	var next []byte
	if want {
		body, err := RenderDefaultHTTPSServer(features)
		if err != nil {
			result.Outcome = DefaultHTTPSFailed
			result.Detail = err.Error()
			return result
		}
		next = []byte(SealManagedText(body))
		if inspected.exists && bytes.Equal(inspected.content, next) {
			result.Outcome = DefaultHTTPSUnchanged
			return result
		}
	} else if !inspected.exists {
		result.Outcome = DefaultHTTPSNotNeeded
		if ownerDefault != "" {
			result.Outcome = DefaultHTTPSOwnerHasOne
			result.Detail = ownerDefault + " is the default server of port 443"
		}
		return result
	}

	restore := func() error {
		if inspected.exists {
			return atomicWriteSiteFile(path, inspected.content, inspected.mode, inspected.ownership)
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if want {
		if err := writeManagedSiteFile(path, next, inspected); err != nil {
			result.Outcome = DefaultHTTPSFailed
			result.Detail = boundedSiteFileDetail("write: " + err.Error())
			return result
		}
		result.Outcome = DefaultHTTPSWritten
	} else {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			result.Outcome = DefaultHTTPSFailed
			result.Detail = boundedSiteFileDetail("remove: " + err.Error())
			return result
		}
		result.Outcome = DefaultHTTPSRemoved
		if ownerDefault != "" {
			result.Detail = ownerDefault + " is the default server of port 443"
		}
	}
	if err := ng.ValidateNginx(); err != nil {
		restoreErr := restore()
		result.Outcome = DefaultHTTPSFailed
		result.Detail = boundedSiteFileDetail(errors.Join(fmt.Errorf("nginx -t refused it, the previous state was put back: %w", err), restoreErr).Error())
		return result
	}
	if err := ng.ReloadNginx(); err != nil {
		restoreErr := restore()
		if restoreErr == nil {
			if validateErr := ng.ValidateNginx(); validateErr == nil {
				restoreErr = ng.ReloadNginx()
			} else {
				restoreErr = validateErr
			}
		}
		result.Outcome = DefaultHTTPSFailed
		result.Detail = boundedSiteFileDetail(errors.Join(fmt.Errorf("nginx reload failed, the previous state was put back: %w", err), restoreErr).Error())
		return result
	}
	return result
}
