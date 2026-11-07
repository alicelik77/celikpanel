package main

import (
	"net/http"
	"strings"
	"time"
)

// The changes whose progress window follows the server's own steps
// (alpha.127, owner's order of 6 Nov 2026: "always, for every operation").
// Each is guarded by its request identity when the page sends one, so its
// handler reports its steps under that identity (operation_steps.go), the
// change outlives a reload, and a replay is answered from the stored row
// instead of running twice (D-029). A request without the header runs as
// before (optional): clients other than the CelikPanel page are not refused.
//
// Sayfanın kimlik gönderdiği değişiklikler korunur; işleyici adımlarını o
// kimlikle bildirir. Başlıksız istek eskisi gibi çalışır.

// operationStepRouteTimeout bounds every one of these handlers once the
// connection can no longer cancel it; it covers the longest Agent call any of
// them makes (a hosting type change installs a runtime's pool and unit).
const operationStepRouteTimeout = agentRPCDeploymentTimeout + 5*time.Minute

func operationStepRoute(pattern string) requestIdentityRoute {
	return requestIdentityRoute{pattern: pattern, timeout: operationStepRouteTimeout, optional: true}
}

var (
	requestIdentityRouteSSLUpload = requestIdentityRoute{
		pattern: "/api/v1/domains/{id}/ssl/upload", timeout: operationStepRouteTimeout, optional: true,
		// The certificate, key and chain upload is larger than the default.
		maxBody: maxCustomCertificateUploadBytes,
	}
	requestIdentityRouteSSLRemove      = operationStepRoute("/api/v1/domains/{id}/ssl")
	requestIdentityRouteMailCert       = operationStepRoute("/api/v1/domains/{id}/ssl/mail")
	requestIdentityRouteDNSRecords     = operationStepRoute("/api/v1/domains/{id}/dns/records")
	requestIdentityRouteHosting        = operationStepRoute("/api/v1/domains/{id}/hosting")
	requestIdentityRouteDomainDBDelete = operationStepRoute("/api/v1/domains/{id}/databases/{db}")
	requestIdentityRoutePDNSRepair     = operationStepRoute("/api/v1/pdns/enable")
	requestIdentityRoutePHPIni         = operationStepRoute("/api/v1/php/extended-config")
	requestIdentityRouteServiceAction  = operationStepRoute("/api/v1/service/action")
	requestIdentityRouteMalwareAction  = operationStepRoute("/api/v1/malware/findings/{id}/{action}")
	requestIdentityRouteDatabaseDelete = operationStepRoute("/api/v1/databases/{id}")
	requestIdentityRouteDBUserDelete   = operationStepRoute("/api/v1/database-users/{id}")
	requestIdentityRouteAdminRemove    = operationStepRoute("/api/v1/database-servers/{id}/admin-account")
	requestIdentityRouteUserDelete     = operationStepRoute("/api/v1/users/{id}")
	requestIdentityRouteTeamDelete     = operationStepRoute("/api/v1/team-members/{id}")
	// alpha.128: the site's PHP values (pool, then nginx file).
	requestIdentityRoutePHPSettings = operationStepRoute("/api/v1/domains/{id}/php/settings")
	// alpha.128: additional FTP/SFTP logins; the stored digest leaves the
	// password out.
	requestIdentityRouteFTPLoginAdd = requestIdentityRoute{
		pattern: "/api/v1/domains/{id}/access/logins", timeout: operationStepRouteTimeout, optional: true,
		digest: requestIdentityFTPLoginDigest,
	}
	// alpha.129: remote access to databases (database_remote_handlers.go).
	requestIdentityRouteDatabaseRemoteAdd    = operationStepRoute("/api/v1/database-remote/databases/{id}/rules")
	requestIdentityRouteDatabaseRemoteRemove = operationStepRoute("/api/v1/database-remote/rules/{id}")
	requestIdentityRouteDatabaseRemoteServer = operationStepRoute("/api/v1/database-remote/server")
	requestIdentityRouteFTPLogin             = requestIdentityRoute{
		pattern: "/api/v1/domains/{id}/access/logins/{login}", timeout: operationStepRouteTimeout, optional: true,
		digest: requestIdentityFTPLoginDigest,
	}
)

// operationStepRouteFor names the optional route a request would reach, using
// the dispatchers' own strict matchers where they exist.
func operationStepRouteFor(r *http.Request) (requestIdentityRoute, bool) {
	path := r.URL.Path
	if strings.HasPrefix(path, databaseRemotePath) {
		if route, ok := matchDatabaseRemoteRoute(r); ok && routeAllows(r.Method, route.methods) {
			switch {
			case route.kind == "rules" && r.Method == http.MethodPost:
				return requestIdentityRouteDatabaseRemoteAdd, true
			case route.kind == "rule" && r.Method == http.MethodDelete:
				return requestIdentityRouteDatabaseRemoteRemove, true
			case route.kind == "server" && r.Method == http.MethodPut:
				return requestIdentityRouteDatabaseRemoteServer, true
			}
		}
		return requestIdentityRoute{}, false
	}
	switch r.Method {
	case http.MethodPost:
		switch path {
		case "/api/v1/pdns/enable":
			return requestIdentityRoutePDNSRepair, true
		case "/api/v1/php/extended-config":
			return requestIdentityRoutePHPIni, true
		case "/api/v1/service/action":
			return requestIdentityRouteServiceAction, true
		case serverRebootPath: // alpha.128 ops: G29, always with an identity
			return requestIdentityRouteServerReboot, true
		}
		// alpha.128 ops: G31, installing a request's issued certificate.
		if rest, ok := strings.CutPrefix(path, certificateRequestsPath+"/"); ok {
			if idText, action, found := strings.Cut(rest, "/"); found && action == "install" {
				if _, valid := strictPositiveID(idText); valid {
					return requestIdentityRouteCertificateRequestInstall, true
				}
			}
		}
		if rest, ok := strings.CutPrefix(path, malwarePath+"findings/"); ok {
			segments := strings.Split(rest, "/")
			if len(segments) == 2 {
				if _, valid := strictPositiveID(segments[0]); valid {
					switch segments[1] {
					case "quarantine", "restore", "delete":
						return requestIdentityRouteMalwareAction, true
					}
				}
			}
		}
	case http.MethodDelete:
		if rest, ok := strings.CutPrefix(path, "/api/v1/users/"); ok {
			if _, valid := strictPositiveID(rest); valid {
				return requestIdentityRouteUserDelete, true
			}
		}
		if rest, ok := strings.CutPrefix(path, "/api/v1/team-members/"); ok {
			if _, valid := strictPositiveID(rest); valid {
				return requestIdentityRouteTeamDelete, true
			}
		}
		for _, prefix := range []string{"/api/v1/databases/", "/api/v1/database-users/", "/api/v1/database-servers/"} {
			if !strings.HasPrefix(path, prefix) {
				continue
			}
			match, ok := matchDatabaseSubroute(r, prefix)
			if !ok {
				break
			}
			switch match.kind {
			case "database-delete":
				return requestIdentityRouteDatabaseDelete, true
			case "user-delete":
				return requestIdentityRouteDBUserDelete, true
			case "server-admin-account":
				return requestIdentityRouteAdminRemove, true
			}
		}
	}
	if !strings.HasPrefix(path, "/api/v1/domains/") {
		return requestIdentityRoute{}, false
	}
	match, ok := matchDomainSubroute(r)
	if !ok {
		return requestIdentityRoute{}, false
	}
	switch {
	case match.kind == "ssl-upload" && r.Method == http.MethodPost:
		return requestIdentityRouteSSLUpload, true
	case match.kind == "ssl" && r.Method == http.MethodDelete:
		return requestIdentityRouteSSLRemove, true
	case match.kind == "ssl-mail" && r.Method == http.MethodPut:
		return requestIdentityRouteMailCert, true
	case match.kind == "hosting" && r.Method == http.MethodPut:
		return requestIdentityRouteHosting, true
	case match.kind == "php-settings" && r.Method == http.MethodPut:
		return requestIdentityRoutePHPSettings, true
	case match.kind == "database-delete" && r.Method == http.MethodDelete:
		return requestIdentityRouteDomainDBDelete, true
	case match.kind == "ftp-logins" && r.Method == http.MethodPost:
		return requestIdentityRouteFTPLoginAdd, true
	case match.kind == "ftp-logins" && (r.Method == http.MethodPut || r.Method == http.MethodDelete):
		return requestIdentityRouteFTPLogin, true
	case match.kind == "dns" && strings.HasSuffix(path, "/records") &&
		(r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete):
		return requestIdentityRouteDNSRecords, true
	}
	return requestIdentityRoute{}, false
}
