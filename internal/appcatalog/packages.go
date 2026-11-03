package appcatalog

import "strings"

// Which distribution package brings a PHP extension, so a refusal can name
// what the administrator installs. The application installer itself never
// installs a package (security before freedom: packages are the
// administrator's decision, from Components or the system's package manager).
//
// Bir PHP uzantısını hangi dağıtım paketinin getirdiği; ret iletisi
// yöneticinin kuracağı paketi adlandırır. Uygulama kurucusu paket kurmaz.

// aptExtensionPackage maps an extension to the suffix of Debian's and
// Ubuntu's php<version>-<suffix> package. Extensions of php<version>-common
// are listed with "common".
var aptExtensionPackage = map[string]string{
	"mysqli": "mysql", "pdo_mysql": "mysql", "mysqlnd": "mysql",
	"pgsql": "pgsql", "pdo_pgsql": "pgsql",
	"sqlite3": "sqlite3", "pdo_sqlite": "sqlite3",
	"gd": "gd", "intl": "intl", "mbstring": "mbstring", "zip": "zip", "curl": "curl",
	"bcmath": "bcmath", "gmp": "gmp", "soap": "soap", "ldap": "ldap", "imap": "imap",
	"apcu": "apcu", "imagick": "imagick", "redis": "redis", "memcached": "memcached",
	"opcache": "opcache", "readline": "readline",
	"dom": "xml", "xml": "xml", "simplexml": "xml", "xmlreader": "xml", "xmlwriter": "xml", "xsl": "xsl",
	"ctype": "common", "fileinfo": "common", "iconv": "common", "exif": "common", "posix": "common",
	"tokenizer": "common", "sockets": "common", "ftp": "common", "gettext": "common", "pdo": "common",
	"phar": "common", "shmop": "common", "sodium": "common", "calendar": "common",
	"sysvmsg": "common", "sysvsem": "common", "sysvshm": "common", "ffi": "common",
}

// builtIn extensions are compiled into PHP on every supported system.
var builtIn = map[string]bool{
	"json": true, "openssl": true, "session": true, "filter": true, "hash": true,
	"pcre": true, "spl": true, "standard": true, "zlib": true, "date": true, "random": true,
	"libxml": true, "reflection": true,
}

// pacmanExtensionPackage: Arch Linux packages a few extensions separately;
// the rest ship in the php package and are switched on in /etc/php/php.ini.
var pacmanExtensionPackage = map[string]string{
	"gd": "php-gd", "intl": "php-intl", "sodium": "php-sodium", "pgsql": "php-pgsql",
	"pdo_pgsql": "php-pgsql", "sqlite3": "php-sqlite", "pdo_sqlite": "php-sqlite",
	"apcu": "php-apcu", "imagick": "php-imagick", "redis": "php-redis", "memcached": "php-memcached",
	"xsl": "php-xsl", "snmp": "php-snmp", "tidy": "php-tidy",
}

// MissingExtension is one extension a site's PHP lacks and what brings it.
type MissingExtension struct {
	Extension string `json:"extension"`
	// Package is the distribution package that brings it ("php8.3-intl");
	// empty when the extension ships with PHP and is only switched off.
	Package string `json:"package,omitempty"`
	// Enable is set when the extension ships with PHP and is switched on in
	// the PHP configuration (Arch Linux's php.ini).
	Enable bool `json:"enable,omitempty"`
}

// ExtensionSource names what brings ext for a PHP line on a package family
// ("apt", "pacman").
func ExtensionSource(family, phpLine, ext string) MissingExtension {
	ext = strings.ToLower(strings.TrimSpace(ext))
	out := MissingExtension{Extension: ext}
	if builtIn[ext] {
		out.Enable = true
		return out
	}
	switch family {
	case "apt":
		suffix, ok := aptExtensionPackage[ext]
		if !ok {
			suffix = ext
		}
		if phpLine != "" {
			out.Package = "php" + phpLine + "-" + suffix
		} else {
			out.Package = "php-" + suffix
		}
	case "pacman":
		if pkg, ok := pacmanExtensionPackage[ext]; ok {
			out.Package = pkg
		} else {
			out.Enable = true
		}
	default:
		out.Package = "php-" + ext
	}
	return out
}
