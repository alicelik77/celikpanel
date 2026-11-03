package main

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/alicelik/celikpanel/internal/appcatalog"
)

// The database password inside a catalog application's own configuration
// file (alpha.118), for a transfer's credential rotation and a database
// user's "New password". Each format names the database, the user and the
// password exactly once, in the form the application's own installer writes;
// anything else changes nothing (the site keeps working and the screen says
// so). Comment lines are ignored, so Drupal's commented examples in
// settings.php are not mistaken for the configuration.
//
// Katalog uygulamasının kendi yapılandırma dosyasındaki veritabanı parolası.
// Her biçim veritabanını, kullanıcıyı ve parolayı tam bir kez, uygulamanın
// kendi kurucusunun yazdığı biçimde adlandırır; aksi hâlde hiçbir şey değişmez.

type appConfigFormat struct {
	// file is the configuration file, relative to the document root.
	file string
	// pattern captures (key, value); keys maps a key to "name", "user" or
	// "password".
	pattern *regexp.Regexp
	keys    map[string]string
	// doubleQuoted values (MediaWiki) are PHP "..." strings.
	doubleQuoted bool
}

const singleQuotedPHP = `'((?:[^'\\]|\\.)*)'`

var appConfigFormats = map[string]appConfigFormat{
	"nextcloud": {
		pattern: regexp.MustCompile(`'(dbname|dbuser|dbpassword)'\s*=>\s*` + singleQuotedPHP),
		keys:    map[string]string{"dbname": "name", "dbuser": "user", "dbpassword": "password"},
	},
	"drupal": {
		pattern: regexp.MustCompile(`'(database|username|password)'\s*=>\s*` + singleQuotedPHP),
		keys:    map[string]string{"database": "name", "username": "user", "password": "password"},
	},
	"joomla": {
		pattern: regexp.MustCompile(`public\s+\$(db|user|password)\s*=\s*` + singleQuotedPHP + `\s*;`),
		keys:    map[string]string{"db": "name", "user": "user", "password": "password"},
	},
	"prestashop": {
		pattern: regexp.MustCompile(`'(database_name|database_user|database_password)'\s*=>\s*` + singleQuotedPHP),
		keys:    map[string]string{"database_name": "name", "database_user": "user", "database_password": "password"},
	},
	"mediawiki": {
		pattern:      regexp.MustCompile(`\$(wgDBname|wgDBuser|wgDBpassword)\s*=\s*"((?:[^"\\$]|\\.)*)"\s*;`),
		keys:         map[string]string{"wgDBname": "name", "wgDBuser": "user", "wgDBpassword": "password"},
		doubleQuoted: true,
	},
	"phpbb": {
		pattern: regexp.MustCompile(`\$(dbname|dbuser|dbpasswd)\s*=\s*` + singleQuotedPHP + `\s*;`),
		keys:    map[string]string{"dbname": "name", "dbuser": "user", "dbpasswd": "password"},
	},
}

// appConfigFormatFor returns the format of an application kind, with the
// file from the catalog. WordPress keeps its own (wordpressDBDefines).
func appConfigFormatFor(kind string) (appConfigFormat, bool) {
	format, ok := appConfigFormats[kind]
	if !ok {
		return appConfigFormat{}, false
	}
	app, found := appcatalog.Lookup(kind)
	if !found || app.ConfigFile == "" {
		return appConfigFormat{}, false
	}
	format.file = app.ConfigFile
	return format, true
}

func commentLine(content []byte, offset int) bool {
	start := strings.LastIndexByte(string(content[:offset]), '\n') + 1
	line := strings.TrimSpace(string(content[start:offset]))
	return strings.HasPrefix(line, "*") || strings.HasPrefix(line, "#") ||
		strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*")
}

// defines reads the database name, user and the span of the password's
// quoted value. Each must appear exactly once outside comments.
func (f appConfigFormat) defines(content []byte) (name, user string, passStart, passEnd int, err error) {
	seen := map[string]int{}
	for _, m := range f.pattern.FindAllSubmatchIndex(content, -1) {
		if commentLine(content, m[0]) {
			continue
		}
		role := f.keys[string(content[m[2]:m[3]])]
		seen[role]++
		raw := string(content[m[4]:m[5]])
		switch role {
		case "name":
			name = f.unquote(raw)
		case "user":
			user = f.unquote(raw)
		case "password":
			passStart, passEnd = m[4]-1, m[5]+1
		}
	}
	base := path.Base(f.file)
	for _, role := range []string{"name", "user", "password"} {
		if seen[role] != 1 {
			return "", "", 0, 0, fmt.Errorf("%s does not name the database %s exactly once in the form the installer wrote", base, role)
		}
	}
	return name, user, passStart, passEnd, nil
}

func (f appConfigFormat) unquote(value string) string {
	if !f.doubleQuoted {
		return phpUnquote(value)
	}
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) && strings.ContainsRune(`\"$`, rune(value[i+1])) {
			i++
		}
		b.WriteByte(value[i])
	}
	return b.String()
}

func (f appConfigFormat) quote(value string) string {
	if !f.doubleQuoted {
		return phpQuote(value)
	}
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`)
	return `"` + replacer.Replace(value) + `"`
}

// matches says why a file is not the one to update ("" when it is).
func (f appConfigFormat) matches(content []byte, dbName, dbUser string) string {
	name, user, _, _, err := f.defines(content)
	switch {
	case err != nil:
		return err.Error()
	case name != dbName || user != dbUser:
		return path.Base(f.file) + " no longer uses database " + dbName + " with user " + dbUser
	}
	return ""
}

// replacePassword writes password as the password's value and changes
// nothing else.
func (f appConfigFormat) replacePassword(content []byte, password string) ([]byte, error) {
	_, _, start, end, err := f.defines(content)
	if err != nil {
		return nil, err
	}
	quoted := f.quote(password)
	out := make([]byte, 0, len(content)+len(quoted))
	out = append(out, content[:start]...)
	out = append(out, quoted...)
	out = append(out, content[end:]...)
	return out, nil
}
