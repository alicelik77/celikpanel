package services

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

// Additional nginx directives of one site (alpha.128, PLESK-GAP G3; owner
// decision 4 of 2026-11-07). The text is written to the site's
// directives.site, read only by the server blocks that serve the site's
// content, never by the block that only redirects to HTTPS.
//
// The administrator's text is checked for size and syntax only; nginx -t
// decides the rest. A reseller's or customer's text (plan permission
// nginx_directives, offered by the administrator first) is also checked
// against the forbidden list below, in this order:
//
//  1. size and syntax;
//  2. the forbidden list: context and identity (include, server, listen,
//     root, alias, ssl_* ...), upstreams and code (every *_pass, mirror,
//     resolver, perl, js_*, lua), paths nginx would write or log to
//     (access_log, error_log, *_temp_path, *_store, dav_*), real-IP rewriting;
//  3. directives a field owns, refused for everybody: index, expires, types,
//     client_max_body_size, fastcgi_read_timeout, the page cache and the
//     HSTS header — "use the field" instead;
//  4. a path argument must lie inside the site's home;
//  5. add_header only at the top level (inside a location it would drop
//     the site's security headers, an nginx inheritance rule).
//
// Then the Agent writes the file, runs nginx -t and reloads; when nginx
// refuses, the previous file is put back.
//
// Ek nginx yönergeleri: yönetici metni yalnız boyut ve sözdizimi denetiminden
// geçer; bayi ve müşteri metni ayrıca yasak listesinden, alan sahipliğinden ve
// yol kuralından geçer. Sonra nginx -t; reddederse önceki dosya geri konur.

// MaxSiteDirectivesBytes bounds the text.
const MaxSiteDirectivesBytes = 16 << 10

const maxDirectiveDepth = 8

// siteDirective is one statement of the text.
type siteDirective struct {
	name  string
	args  []string
	block []siteDirective
	// hasBlock: the statement ends with { ... } rather than ";".
	hasBlock bool
	line     int
}

type directiveToken struct {
	text   string
	quoted bool
	line   int
}

// tokenizeDirectives splits text like nginx's own lexer: words, quoted
// strings (with backslash escapes), ";", "{", "}", and comments from "#" to
// the end of the line. "${name}" stays one word.
func tokenizeDirectives(text string) ([]directiveToken, error) {
	var tokens []directiveToken
	line := 1
	i := 0
	for i < len(text) {
		c := text[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '#':
			for i < len(text) && text[i] != '\n' {
				i++
			}
		case c == ';' || c == '{' || c == '}':
			tokens = append(tokens, directiveToken{text: string(c), line: line})
			i++
		case c == '"' || c == '\'':
			quote := c
			start := line
			i++
			var b strings.Builder
			closed := false
			for i < len(text) {
				ch := text[i]
				if ch == '\\' && i+1 < len(text) {
					b.WriteByte(ch)
					b.WriteByte(text[i+1])
					if text[i+1] == '\n' {
						line++
					}
					i += 2
					continue
				}
				if ch == quote {
					closed = true
					i++
					break
				}
				if ch == '\n' {
					line++
				}
				b.WriteByte(ch)
				i++
			}
			if !closed {
				return nil, fmt.Errorf("line %d: a quoted text is not closed", start)
			}
			tokens = append(tokens, directiveToken{text: b.String(), quoted: true, line: start})
		default:
			start := i
			for i < len(text) {
				ch := text[i]
				if ch == '$' && i+1 < len(text) && text[i+1] == '{' {
					end := strings.IndexByte(text[i:], '}')
					if end < 0 {
						return nil, fmt.Errorf("line %d: a ${...} variable is not closed", line)
					}
					i += end + 1
					continue
				}
				if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' || ch == ';' || ch == '{' || ch == '}' || ch == '"' || ch == '\'' {
					break
				}
				if ch < 0x20 || ch == 0x7f {
					return nil, fmt.Errorf("line %d: a control character is not allowed", line)
				}
				i++
			}
			tokens = append(tokens, directiveToken{text: text[start:i], line: line})
		}
	}
	return tokens, nil
}

// parseSiteDirectives parses the text into statements.
func parseSiteDirectives(text string) ([]siteDirective, error) {
	if len(text) > MaxSiteDirectivesBytes {
		return nil, fmt.Errorf("the text is longer than %d KB", MaxSiteDirectivesBytes>>10)
	}
	if strings.ContainsRune(text, 0) {
		return nil, errors.New("the text holds a NUL character")
	}
	tokens, err := tokenizeDirectives(text)
	if err != nil {
		return nil, err
	}
	pos := 0
	var parse func(depth int) ([]siteDirective, error)
	parse = func(depth int) ([]siteDirective, error) {
		if depth > maxDirectiveDepth {
			return nil, fmt.Errorf("blocks are nested more than %d deep", maxDirectiveDepth)
		}
		var out []siteDirective
		for pos < len(tokens) {
			tok := tokens[pos]
			if !tok.quoted && tok.text == "}" {
				if depth == 0 {
					return nil, fmt.Errorf("line %d: \"}\" without an open block", tok.line)
				}
				pos++
				return out, nil
			}
			if !tok.quoted && (tok.text == ";" || tok.text == "{") {
				return nil, fmt.Errorf("line %d: %q without a directive name", tok.line, tok.text)
			}
			d := siteDirective{name: tok.text, line: tok.line}
			if tok.quoted {
				return nil, fmt.Errorf("line %d: a directive name cannot be quoted", tok.line)
			}
			pos++
			ended := false
			for pos < len(tokens) {
				t := tokens[pos]
				if !t.quoted && t.text == ";" {
					pos++
					ended = true
					break
				}
				if !t.quoted && t.text == "{" {
					pos++
					block, err := parse(depth + 1)
					if err != nil {
						return nil, err
					}
					d.block, d.hasBlock, ended = block, true, true
					break
				}
				if !t.quoted && t.text == "}" {
					return nil, fmt.Errorf("line %d: %s is missing its \";\"", d.line, d.name)
				}
				d.args = append(d.args, t.text)
				pos++
			}
			if !ended {
				return nil, fmt.Errorf("line %d: %s is missing its \";\"", d.line, d.name)
			}
			out = append(out, d)
		}
		if depth > 0 {
			return nil, errors.New("a block is not closed with \"}\"")
		}
		return out, nil
	}
	return parse(0)
}

// Names refused for a reseller's or customer's text (lower case).
var siteDirectiveForbidden = map[string]string{
	"include": "context", "load_module": "context", "server": "context", "http": "context",
	"events": "context", "stream": "context", "mail": "context", "upstream": "context",
	"listen": "context", "server_name": "context", "user": "context", "pid": "context",
	"env": "context", "daemon": "context", "master_process": "context", "lock_file": "context",
	"root": "context", "alias": "context", "ssl": "context", "http2": "context", "http3": "context",
	"quic": "context", "thread_pool": "context", "working_directory": "context",
	"mirror": "upstream", "resolver": "upstream", "resolver_timeout": "upstream",
	"perl": "code", "perl_modules": "code", "perl_require": "code", "perl_set": "code",
	"access_log": "path", "error_log": "path", "open_log_file_cache": "path",
	"set_real_ip_from": "realip", "real_ip_header": "realip", "real_ip_recursive": "realip",
	"create_full_put_path": "path", "min_delete_depth": "path",
	"uwsgi_cache": "field", "scgi_cache": "field",
}

// Directives whose arguments name files on the server.
var siteDirectivePathArgs = map[string]bool{
	"auth_basic_user_file": true, "xslt_stylesheet": true, "disable_symlinks": true,
}

// fieldOwnedDirectives are refused for everybody at the top level: a second
// one would append to, replace or duplicate what the field writes.
var fieldOwnedDirectives = map[string]string{
	"index":                "index files",
	"expires":              "browser caching",
	"types":                "extra MIME types",
	"client_max_body_size": "largest upload (PHP settings)",
	"fastcgi_read_timeout": "maximum run time (PHP settings)",
	"fastcgi_cache":        "page cache",
	"proxy_cache":          "page cache",
}

// SiteDirectiveRefusal is a refusal of the text the screen can name.
type SiteDirectiveRefusal struct {
	// Kind: syntax, forbidden, field, path, header.
	Kind      string
	Directive string
	Line      int
	Field     string
	Message   string
}

func (e *SiteDirectiveRefusal) Error() string { return e.Message }

// ValidateSiteDirectives checks a site's additional directives. trusted: the
// administrator's text (syntax and size only). siteHome is the site's home
// (the parent of its document root); a path argument must lie inside it.
func ValidateSiteDirectives(text string, trusted bool, siteHome string) error {
	directives, err := parseSiteDirectives(text)
	if err != nil {
		return &SiteDirectiveRefusal{Kind: "syntax", Message: err.Error()}
	}
	var walk func(list []siteDirective, depth int) error
	walk = func(list []siteDirective, depth int) error {
		for _, d := range list {
			name := strings.ToLower(d.name)
			// A field's directive inside a location is that location's own
			// value (a legitimate override); at the top level it would
			// append to, replace or duplicate what the field writes.
			if field, ok := fieldOwnedDirectives[name]; ok && depth == 0 {
				return &SiteDirectiveRefusal{Kind: "field", Directive: d.name, Line: d.line, Field: field,
					Message: fmt.Sprintf("line %d: %s is set by the field \"%s\" on this screen; use the field instead", d.line, d.name, field)}
			}
			if name == "add_header" && len(d.args) > 0 && strings.EqualFold(d.args[0], "Strict-Transport-Security") {
				return &SiteDirectiveRefusal{Kind: "field", Directive: d.name, Line: d.line, Field: "HSTS (SSL/TLS settings)",
					Message: fmt.Sprintf("line %d: the Strict-Transport-Security header is set in the SSL/TLS settings; use them instead", d.line)}
			}
			if !trusted {
				// `access_log off;` writes nothing: allowed.
				offLog := name == "access_log" && len(d.args) == 1 && d.args[0] == "off"
				if why := forbiddenSiteDirective(name); why != "" && !offLog {
					return &SiteDirectiveRefusal{Kind: "forbidden", Directive: d.name, Line: d.line,
						Message: fmt.Sprintf("line %d: %s is not allowed here (%s); only the server administrator can set it", d.line, d.name, why)}
				}
				if name == "add_header" && depth > 0 {
					return &SiteDirectiveRefusal{Kind: "header", Directive: d.name, Line: d.line,
						Message: fmt.Sprintf("line %d: add_header inside a block would drop the site's security headers there; use the Headers field, or put it at the top level", d.line)}
				}
				if siteDirectivePathArgs[name] {
					for _, arg := range d.args {
						value := arg
						if rest, ok := strings.CutPrefix(arg, "from="); ok {
							value = rest
						}
						if !strings.HasPrefix(value, "/") {
							continue
						}
						if !pathInside(value, siteHome) {
							return &SiteDirectiveRefusal{Kind: "path", Directive: d.name, Line: d.line,
								Message: fmt.Sprintf("line %d: %s names %s, which is outside this site's folder %s", d.line, d.name, value, siteHome)}
						}
					}
				}
			}
			if err := walk(d.block, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(directives, 0)
}

// forbiddenSiteDirective says why a name is refused for a reseller's or
// customer's text ("" when it is allowed).
func forbiddenSiteDirective(name string) string {
	if kind, ok := siteDirectiveForbidden[name]; ok {
		switch kind {
		case "context":
			return "it changes the server's structure or files"
		case "upstream":
			return "it sends requests to other servers"
		case "code":
			return "it runs code inside nginx"
		case "path":
			return "it writes or logs to a file on the server"
		case "realip":
			return "it changes the visitor address other sites and security rules see"
		case "field":
			return "a field on this screen sets it"
		}
	}
	switch {
	case strings.HasSuffix(name, "_pass"):
		return "it sends requests to another program or server"
	case strings.HasPrefix(name, "ssl_"):
		return "certificates and TLS are set in the SSL/TLS settings"
	case strings.HasPrefix(name, "worker_"):
		return "it changes the whole server"
	case strings.HasPrefix(name, "js_"), strings.HasPrefix(name, "lua"), strings.Contains(name, "_by_lua"), strings.HasPrefix(name, "perl"):
		return "it runs code inside nginx"
	case strings.HasSuffix(name, "_temp_path"), strings.HasSuffix(name, "_cache_path"),
		strings.HasSuffix(name, "_store"), strings.HasSuffix(name, "_store_access"), strings.HasPrefix(name, "dav_"):
		return "it writes files on the server"
	}
	return ""
}

// pathInside: the cleaned absolute path, without variables, lies inside dir.
func pathInside(p, dir string) bool {
	if dir == "" || strings.ContainsAny(p, "$") {
		return false
	}
	clean := path.Clean(p)
	return clean == dir || strings.HasPrefix(clean, strings.TrimSuffix(dir, "/")+"/")
}
