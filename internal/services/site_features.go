package services

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/alicelik/celikpanel/internal/hostname"
)

// Per-site password-protected folders and free-form redirects (alpha.114).
//
// Both are rendered into the site's own nginx vhost (templates/nginx/
// vhost.conf.tmpl), so `nginx -t` checks them with the rest of the
// configuration before nginx is reloaded, and a vhost the owner changed is
// kept and named like any other (D-031). Every value that reaches the template
// is validated here, by the one function the Panel (for usability) and the
// Agent (for safety) both call: a value that is not on the short allow-lists
// below is refused, never escaped, so no input can become another nginx
// directive.
//
// Siteye özel parolalı klasörler ve serbest yönlendirmeler (alpha.114). İkisi de
// sitenin kendi nginx vhost'una yazılır; `nginx -t` yeniden yüklemeden önce
// bunları da denetler. Şablona giren her değer burada doğrulanır; kısa izin
// listesinde olmayan değer kaçışlanmaz, reddedilir.

const (
	// MaxSiteProtectedDirs, MaxSiteProtectedUsers and MaxSiteRedirects bound
	// what one site can carry, so a vhost stays readable and nginx -t fast.
	MaxSiteProtectedDirs  = 20
	MaxSiteProtectedUsers = 50
	MaxSiteRedirects      = 100

	// RedirectMatchExact is the one path; RedirectMatchPrefix is the folder
	// and everything below it, with the rest of the address carried over.
	RedirectMatchExact  = "exact"
	RedirectMatchPrefix = "prefix"

	// SiteRealmDefault is the title of the sign-in box when none was given.
	SiteRealmDefault = "Restricted area"

	maxSitePathLen      = 255
	maxSitePathSegments = 16
	maxRedirectTarget   = 2048
	maxSiteRealmLen     = 64
	maxSiteUsernameLen  = 64

	siteFeatureProjectPHP    = "php"
	siteFeatureProjectStatic = "static"
)

var (
	sitePathSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9_~-][A-Za-z0-9._~-]*$`)
	siteRealmPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]*$`)
	siteUsernamePattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	// redirectTargetPattern is every character a redirect target may carry:
	// URL characters that mean nothing to nginx's configuration syntax. No
	// space, quote, semicolon, brace, dollar, backslash, hash or control
	// character; a fragment (#) is not supported.
	redirectTargetPattern = regexp.MustCompile(`^[A-Za-z0-9._~:/?&=%@+,!*()\[\]-]+$`)
)

// SiteProtectedDir is one password-protected folder of a site. ID, Path and
// Realm come from the caller; AuthFile is derived by Render.
type SiteProtectedDir struct {
	ID       int
	Path     string
	Realm    string
	AuthFile string
}

// SiteRedirect is one redirect of a site. Match, Source, Target, Code and
// KeepQuery come from the caller; the rest is derived by Render.
type SiteRedirect struct {
	Match     string
	Source    string
	Target    string
	Code      int
	KeepQuery bool
	// SourceRegex is Source as the text of a regular expression, TargetBase
	// the target without a trailing slash: derived, for the template.
	SourceRegex string
	TargetBase  string
}

// NormalizeSitePath returns the canonical form of a folder or path of a site
// ("/private", no trailing slash) or an error saying what is wrong. "/" alone
// is accepted (asRoot) only where the caller allows it.
func NormalizeSitePath(raw string, allowRoot bool) (string, error) {
	if raw == "/" && allowRoot {
		return "/", nil
	}
	path := strings.TrimSuffix(raw, "/")
	if path == "" || path[0] != '/' {
		return "", fmt.Errorf("the path must start with a slash, for example /private")
	}
	if len(path) > maxSitePathLen {
		return "", fmt.Errorf("the path is too long")
	}
	segments := strings.Split(path[1:], "/")
	if len(segments) > maxSitePathSegments {
		return "", fmt.Errorf("the path has too many levels")
	}
	for _, segment := range segments {
		if segment == "" {
			return "", fmt.Errorf("the path must not contain an empty level (//)")
		}
		if !sitePathSegmentPattern.MatchString(segment) {
			return "", fmt.Errorf("the path may contain only letters, digits, dot, dash, underscore and tilde, and a level must not start with a dot")
		}
	}
	return path, nil
}

func siteReservedPath(path string) bool {
	return path == "/.well-known" || strings.HasPrefix(path, "/.well-known/")
}

// NormalizeSiteRealm checks the title of the sign-in box.
func NormalizeSiteRealm(raw string) (string, error) {
	realm := strings.TrimSpace(raw)
	if realm == "" {
		return SiteRealmDefault, nil
	}
	if len(realm) > maxSiteRealmLen || !siteRealmPattern.MatchString(realm) {
		return "", fmt.Errorf("the sign-in title may contain only letters, digits, space, dot, dash and underscore (at most %d characters)", maxSiteRealmLen)
	}
	return realm, nil
}

// ValidateSiteProtectedUsername checks a user name of a protected folder. A
// colon would split the password file's line, so it is not allowed.
func ValidateSiteProtectedUsername(username string) error {
	if username == "" || len(username) > maxSiteUsernameLen || !siteUsernamePattern.MatchString(username) {
		return fmt.Errorf("a user name may contain only letters, digits, dot, dash and underscore (at most %d characters)", maxSiteUsernameLen)
	}
	return nil
}

// NormalizeRedirect validates one redirect and returns it in canonical form.
// serverNames are the names the site answers to; a target that leads back
// into the source on one of them is refused (a loop).
func NormalizeRedirect(in SiteRedirect, serverNames []string) (SiteRedirect, error) {
	out := in
	switch in.Match {
	case RedirectMatchExact:
		source, err := NormalizeSitePath(in.Source, true)
		if err != nil {
			return SiteRedirect{}, err
		}
		out.Source = source
	case RedirectMatchPrefix:
		source, err := NormalizeSitePath(in.Source, false)
		if err != nil {
			return SiteRedirect{}, err
		}
		out.Source = source
	default:
		return SiteRedirect{}, fmt.Errorf("the match must be exact or prefix")
	}
	if siteReservedPath(out.Source) {
		return SiteRedirect{}, fmt.Errorf("the path /.well-known is reserved for certificate validation")
	}
	switch in.Code {
	case 301, 302, 307, 308:
	default:
		return SiteRedirect{}, fmt.Errorf("the status code must be 301, 302, 307 or 308")
	}
	target, err := normalizeRedirectTarget(in.Target)
	if err != nil {
		return SiteRedirect{}, err
	}
	out.Target = target
	hasQuery := strings.Contains(target, "?")
	if in.Match == RedirectMatchPrefix && hasQuery {
		return SiteRedirect{}, fmt.Errorf("the target of a folder redirect must not contain a query string")
	}
	if in.KeepQuery && hasQuery {
		return SiteRedirect{}, fmt.Errorf("choose either keeping the visitor's query string or a query string in the target, not both")
	}
	targetPath, targetHost := redirectTargetParts(target)
	if in.Match == RedirectMatchPrefix {
		out.TargetBase = strings.TrimSuffix(target, "/")
		if out.TargetBase == "" {
			return SiteRedirect{}, fmt.Errorf("the target of a folder redirect must not be the site root; use a folder or another address")
		}
	}
	if targetHost == "" || containsFold(serverNames, targetHost) {
		if redirectLoops(in.Match, out.Source, targetPath) {
			return SiteRedirect{}, fmt.Errorf("the target leads back into the redirected path and would loop")
		}
	}
	out.SourceRegex = strings.ReplaceAll(out.Source, ".", `\.`)
	return out, nil
}

func normalizeRedirectTarget(raw string) (string, error) {
	if raw == "" || raw != strings.TrimSpace(raw) {
		return "", fmt.Errorf("a target address is required")
	}
	if len(raw) > maxRedirectTarget {
		return "", fmt.Errorf("the target address is too long")
	}
	if !redirectTargetPattern.MatchString(raw) {
		return "", fmt.Errorf("the target address contains a character that is not allowed (no spaces, quotes, semicolons, braces, dollar signs or #)")
	}
	if strings.HasPrefix(raw, "/") {
		if strings.HasPrefix(raw, "//") {
			return "", fmt.Errorf("a target that starts with // is not allowed; use a full http:// or https:// address")
		}
		return raw, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || parsed.User != nil || parsed.Host == "" || parsed.Fragment != "" {
		return "", fmt.Errorf("the target must be a path starting with / or a full http:// or https:// address")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("the target must use http or https")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" {
		return "", fmt.Errorf("the target host name is invalid")
	}
	if net.ParseIP(host) == nil {
		if err := hostname.Validate(host); err != nil {
			return "", fmt.Errorf("the target host name is invalid")
		}
	}
	if port := parsed.Port(); port != "" {
		var number int
		if _, err := fmt.Sscanf(port, "%d", &number); err != nil || number <= 0 || number > 65535 {
			return "", fmt.Errorf("the target port is invalid")
		}
	}
	// The scheme and host are written in lower case; the rest is the text
	// the person gave, which the allow-list above has already checked.
	rest := raw[strings.Index(raw, "://")+3:]
	if slash := strings.IndexAny(rest, "/?"); slash >= 0 {
		rest = rest[slash:]
	} else {
		rest = ""
	}
	authority := strings.ToLower(parsed.Host)
	return parsed.Scheme + "://" + authority + rest, nil
}

// redirectTargetParts splits a normalized target into its path (without any
// query string) and the lower-case host name ("" for a path-only target).
func redirectTargetParts(target string) (path, host string) {
	if strings.HasPrefix(target, "/") {
		path = target
	} else {
		rest := target[strings.Index(target, "://")+3:]
		authority := rest
		if slash := strings.IndexAny(rest, "/?"); slash >= 0 {
			authority, path = rest[:slash], rest[slash:]
		}
		if parsed, err := url.Parse("//" + authority); err == nil {
			host = strings.ToLower(parsed.Hostname())
		}
	}
	if question := strings.Index(path, "?"); question >= 0 {
		path = path[:question]
	}
	if path == "" {
		path = "/"
	}
	return path, host
}

func redirectLoops(match, source, targetPath string) bool {
	if match == RedirectMatchExact {
		return strings.TrimSuffix(targetPath, "/") == strings.TrimSuffix(source, "/")
	}
	return targetPath == source || strings.HasPrefix(targetPath, source+"/")
}

func containsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}

// PrepareSiteFeatures validates the protected folders and redirects of one
// vhost render, refuses combinations nginx would reject or that would mean
// something other than was asked, and fills the derived fields. The result is
// sorted, so the same input always renders the same text.
func PrepareSiteFeatures(projectType string, serverNames []string, dirs []SiteProtectedDir, redirects []SiteRedirect, authFile func(id int) string) ([]SiteProtectedDir, []SiteRedirect, error) {
	if len(dirs) == 0 && len(redirects) == 0 {
		return nil, nil, nil
	}
	if projectType == "forwarding" {
		return nil, nil, fmt.Errorf("a forwarding site sends every address to its target, so it has no folders to protect and no redirects of its own")
	}
	if len(dirs) > 0 && projectType != siteFeatureProjectPHP && projectType != siteFeatureProjectStatic {
		return nil, nil, fmt.Errorf("password-protected folders are available for PHP and static sites")
	}
	if len(dirs) > MaxSiteProtectedDirs {
		return nil, nil, fmt.Errorf("a site can have at most %d protected folders", MaxSiteProtectedDirs)
	}
	if len(redirects) > MaxSiteRedirects {
		return nil, nil, fmt.Errorf("a site can have at most %d redirects", MaxSiteRedirects)
	}
	protected := make(map[string]bool, len(dirs))
	outDirs := make([]SiteProtectedDir, 0, len(dirs))
	for _, dir := range dirs {
		path, err := NormalizeSitePath(dir.Path, false)
		if err != nil {
			return nil, nil, fmt.Errorf("protected folder %q: %w", dir.Path, err)
		}
		if siteReservedPath(path) {
			return nil, nil, fmt.Errorf("protected folder %q: the path /.well-known is reserved for certificate validation", dir.Path)
		}
		if protected[path] {
			return nil, nil, fmt.Errorf("protected folder %q is listed twice", path)
		}
		protected[path] = true
		realm, err := NormalizeSiteRealm(dir.Realm)
		if err != nil {
			return nil, nil, err
		}
		if dir.ID <= 0 {
			return nil, nil, fmt.Errorf("protected folder %q has no identity", path)
		}
		outDirs = append(outDirs, SiteProtectedDir{ID: dir.ID, Path: path, Realm: realm, AuthFile: authFile(dir.ID)})
	}
	sort.Slice(outDirs, func(i, j int) bool { return outDirs[i].Path < outDirs[j].Path })

	seen := make(map[string]bool, len(redirects))
	outRedirects := make([]SiteRedirect, 0, len(redirects))
	for _, redirect := range redirects {
		normalized, err := NormalizeRedirect(redirect, serverNames)
		if err != nil {
			return nil, nil, fmt.Errorf("redirect %q: %w", redirect.Source, err)
		}
		key := normalized.Match + " " + normalized.Source
		if seen[key] {
			return nil, nil, fmt.Errorf("redirect %q is listed twice", normalized.Source)
		}
		seen[key] = true
		// A redirect and a protected folder on the same path would be two
		// nginx locations of the same name; nginx refuses that.
		if protected[normalized.Source] {
			return nil, nil, fmt.Errorf("redirect %q is the same path as a protected folder", normalized.Source)
		}
		// A folder redirect also owns "<source>/..." and "= <source>"; an exact
		// redirect of the same path would duplicate the second.
		if normalized.Match == RedirectMatchPrefix && seen[RedirectMatchExact+" "+normalized.Source] {
			return nil, nil, fmt.Errorf("redirect %q is both an exact and a folder redirect", normalized.Source)
		}
		if normalized.Match == RedirectMatchExact && seen[RedirectMatchPrefix+" "+normalized.Source] {
			return nil, nil, fmt.Errorf("redirect %q is both an exact and a folder redirect", normalized.Source)
		}
		outRedirects = append(outRedirects, normalized)
	}
	sort.Slice(outRedirects, func(i, j int) bool {
		if outRedirects[i].Source != outRedirects[j].Source {
			return outRedirects[i].Source < outRedirects[j].Source
		}
		return outRedirects[i].Match < outRedirects[j].Match
	})
	return outDirs, outRedirects, nil
}
