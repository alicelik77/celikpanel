package appcatalog

// The pinned releases (checked 2026-11-03). To move an application to a new
// release: download the upstream's official archive, compare its SHA-256 with
// the value the upstream publishes (where it publishes one), put the address
// and the digest here, update Version and the PHP range from the release's
// own requirement file, and run the tests. Nothing else changes.
//
// Sabitlenmiş sürümler. Yeni sürüme geçmek: resmi arşivi indir, SHA-256'yı
// yayımlanan değerle karşılaştır, adresi ve özeti buraya yaz.

const mib = int64(1 << 20)

var catalog = []App{
	{
		ID: "wordpress", Kind: KindPHPSite, Name: "WordPress", Version: "latest",
		License: "GPL-2.0-or-later", Homepage: "https://wordpress.org",
		Summary: "Blogs, company sites and shops (WooCommerce).",
		Archive: Archive{
			URL: "https://wordpress.org/latest.tar.gz", Format: FormatTarGz,
			Upstream: "the newest release, checked against the SHA-1 WordPress.org publishes beside it (the installer of earlier releases, kept unchanged)",
		},
		PHPMin: "7.4", Extensions: []string{"mysqli"}, Databases: []string{"mariadb"},
		MinDiskMB: 100, Setup: SetupWordPressLegacy, AdminPath: "/wp-admin/",
		ConfigFile: "wp-config.php",
		Risk:       "Widely used and widely attacked: keep WordPress, its plugins and themes updated.",
	},
	{
		ID: "prestashop", Kind: KindPHPSite, Name: "PrestaShop", Version: "9.2.0",
		License: "OSL-3.0 (core), AFL-3.0 (modules)", Homepage: "https://prestashop.com",
		Summary: "An online shop: catalogue, cart, orders and payments.",
		Archive: Archive{
			URL:      "https://assets.prestashop3.com/dst/edition/corporate/9.2.0-6.0/prestashop_edition_classic_version_9.2.0-6.0.zip",
			SHA256:   "81e1719181c4ebe29d54c7e914bc571f1883dd4cfd216f5b28bd30af5757105f",
			Format:   FormatZip,
			Inner:    "prestashop.zip",
			Markers:  []string{"index.php", "install/index_cli.php", "admin/", "app/", "vendor/"},
			Upstream: "PrestaShop publishes no checksum for this archive; the digest was taken from the official download (prestashop.com/versions, Classic edition 9.2.0-6.0) on 2026-11-03",
			// The release zip is 134 MB; the inner tree is 456 MB in 52,423 entries.
			MaxDownloadBytes: 200 * mib, MaxExtractedBytes: 900 * mib, MaxEntries: 70_000,
		},
		PHPMin: "8.1", PHPMax: "8.5",
		Extensions: []string{"curl", "dom", "fileinfo", "gd", "iconv", "intl", "json", "mbstring", "openssl", "pdo_mysql", "simplexml", "zip", "zlib"},
		Databases:  []string{"mariadb"}, MinDiskMB: 800, Setup: SetupPrestaShop,
		AdminUser: "the e-mail address shown", ConfigFile: "app/config/parameters.php",
		Nginx: "prestashop", UploadMB: 64,
		Risk: "The Classic edition includes PrestaShop SA modules (PrestaShop Checkout, Account, CloudSync) that connect to PrestaShop's services once enabled. Shops handle personal and payment data: keep PrestaShop and its modules updated.",
	},
	{
		ID: "nextcloud", Kind: KindPHPSite, Name: "Nextcloud", Version: "35.0.2",
		License: "AGPL-3.0-or-later", Homepage: "https://nextcloud.com",
		Summary: "Files, calendars and contacts in your own cloud.",
		Archive: Archive{
			URL:         "https://download.nextcloud.com/server/releases/nextcloud-35.0.2.tar.bz2",
			SHA256:      "dfc7955eaec5621de90e031d10f76006173b3f3e31d5d6e174821d1da14f162d",
			Format:      FormatTarBz2,
			StripPrefix: "nextcloud",
			Markers:     []string{"occ", "version.php", "index.php", "lib/", "core/"},
			Upstream:    "equal to nextcloud-35.0.2.tar.bz2.sha256 published by Nextcloud",
			// 250 MB archive; 760 MB in 31,421 entries.
			MaxDownloadBytes: 320 * mib, MaxExtractedBytes: 1400 * mib, MaxEntries: 45_000,
		},
		PHPMin: "8.3", PHPMax: "8.5",
		Extensions: []string{"ctype", "curl", "dom", "fileinfo", "filter", "gd", "mbstring", "openssl", "pdo_mysql", "posix", "session", "simplexml", "xmlreader", "xmlwriter", "zip", "zlib"},
		Databases:  []string{"mariadb"}, MinDiskMB: 1100, Setup: SetupNextcloud,
		AdminPath: "/", AdminUser: "admin", DataDir: "nextcloud-data", ConfigFile: "config/config.php",
		Nginx: "nextcloud", UploadMB: 512,
		Risk: "Holds people's files: the data folder is outside the site's public folder. Set up background jobs (cron) and keep Nextcloud updated with its own updater.",
	},
	{
		ID: "drupal", Kind: KindPHPSite, Name: "Drupal", Version: "11.4.11",
		License: "GPL-2.0-or-later", Homepage: "https://www.drupal.org",
		Summary: "A content management system for structured sites.",
		Archive: Archive{
			URL:         "https://ftp.drupal.org/files/projects/drupal-11.4.11.tar.gz",
			SHA256:      "f6935da6ddfb2b3bf5c79c5118018be91021f7d5e12901eed1a71de140708e13",
			Format:      FormatTarGz,
			StripPrefix: "drupal-11.4.11",
			Markers:     []string{"index.php", "autoload.php", "core/includes/install.core.inc", "vendor/"},
			Upstream:    "drupal.org publishes an MD5 (7be4f59239b421684573764a8809cfb1) for this file, which the downloaded archive matched; the SHA-256 was taken from that file on 2026-11-03",
			// 23 MB archive; 93 MB in 31,132 entries.
			MaxDownloadBytes: 64 * mib, MaxExtractedBytes: 300 * mib, MaxEntries: 45_000,
		},
		PHPMin:     "8.3",
		Extensions: []string{"dom", "gd", "mbstring", "pdo_mysql", "simplexml", "tokenizer", "xml"},
		Databases:  []string{"mariadb"}, MinDiskMB: 250, Setup: SetupDrupal,
		AdminPath: "/user/login", AdminUser: "admin", ConfigFile: "sites/default/settings.php",
		Nginx: "drupal", UploadMB: 32,
		Risk: "Keep Drupal core and its modules updated; Drupal publishes security releases on Wednesdays.",
	},
	{
		ID: "joomla", Kind: KindPHPSite, Name: "Joomla", Version: "5.4.10",
		License: "GPL-2.0-or-later", Homepage: "https://www.joomla.org",
		Summary: "A content management system for sites and portals.",
		Archive: Archive{
			URL:      "https://github.com/joomla/joomla-cms/releases/download/5.4.10/Joomla_5.4.10-Stable-Full_Package.tar.gz",
			SHA256:   "aa72c425105cf775312f9610598b9f1c8866c7f3219a729b59d29a56ff7f76c4",
			Format:   FormatTarGz,
			Markers:  []string{"index.php", "installation/joomla.php", "libraries/", "administrator/"},
			Upstream: "equal to the SHA-256 digest GitHub records for the Joomla 5.4.10 release asset",
			// 28 MB archive; 84 MB in 13,654 entries.
			MaxDownloadBytes: 64 * mib, MaxExtractedBytes: 250 * mib, MaxEntries: 25_000,
		},
		PHPMin:     "8.1",
		Extensions: []string{"dom", "gd", "json", "mbstring", "mysqli", "simplexml", "zip", "zlib"},
		Databases:  []string{"mariadb"}, MinDiskMB: 200, Setup: SetupJoomla,
		AdminPath: "/administrator/", AdminUser: "admin", ConfigFile: "configuration.php",
		Nginx: "joomla", UploadMB: 32,
		Risk: "Keep Joomla and its extensions updated; install extensions only from the Joomla Extensions Directory.",
	},
	{
		ID: "mediawiki", Kind: KindPHPSite, Name: "MediaWiki", Version: "1.43.11",
		License: "GPL-2.0-or-later", Homepage: "https://www.mediawiki.org",
		Summary: "The wiki software behind Wikipedia (long-term support release).",
		Archive: Archive{
			URL:         "https://releases.wikimedia.org/mediawiki/1.43/mediawiki-1.43.11.tar.gz",
			SHA256:      "86a5b8e2fb7ca10228d66c8b2659a0a692958e5c520fd023c78bd713a68142f0",
			Format:      FormatTarGz,
			StripPrefix: "mediawiki-1.43.11",
			Markers:     []string{"index.php", "maintenance/install.php", "includes/", "vendor/"},
			Upstream:    "Wikimedia publishes a GPG signature (.sig), no checksum; the SHA-256 was taken from the official download on 2026-11-03",
			// 93 MB archive; 351 MB in 31,725 entries.
			MaxDownloadBytes: 160 * mib, MaxExtractedBytes: 600 * mib, MaxEntries: 45_000,
		},
		PHPMin:     "8.1",
		Extensions: []string{"ctype", "dom", "fileinfo", "iconv", "intl", "json", "mbstring", "mysqli", "xml"},
		Databases:  []string{"mariadb"}, MinDiskMB: 500, Setup: SetupMediaWiki,
		AdminPath: "/index.php?title=Special:UserLogin", AdminUser: "Admin", ConfigFile: "LocalSettings.php",
		Nginx: "mediawiki", UploadMB: 32,
		Risk: "A wiki is open for anyone to edit by default; decide who may edit in LocalSettings.php before announcing it.",
	},
	{
		// alpha.118 lab: download.phpbb.com answers 403 to the panel's own
		// User-Agent. CelikPanel does not pretend to be another program, so
		// phpBB waits until its download works for the panel.
		Withheld: "phpBB's download server refuses the panel's downloads",
		ID:       "phpbb", Kind: KindPHPSite, Name: "phpBB", Version: "3.3.19",
		License: "GPL-2.0-only", Homepage: "https://www.phpbb.com",
		Summary: "A discussion forum.",
		Archive: Archive{
			URL:         "https://download.phpbb.com/pub/release/3.3/3.3.19/phpBB-3.3.19.tar.bz2",
			SHA256:      "66f35fa3d9bf3b90269205bbb281e1156727a0014eea344fe15f9c7376b78c66",
			Format:      FormatTarBz2,
			StripPrefix: "phpBB3",
			Markers:     []string{"index.php", "install/phpbbcli.php", "includes/", "vendor/"},
			Upstream:    "equal to phpBB-3.3.19.tar.bz2.sha256 published by phpBB",
			// 4 MB archive; 22 MB in 4,532 entries.
			MaxDownloadBytes: 32 * mib, MaxExtractedBytes: 100 * mib, MaxEntries: 10_000,
		},
		PHPMin:     "7.2",
		Extensions: []string{"dom", "json", "mbstring", "mysqli", "xml"},
		Databases:  []string{"mariadb"}, MinDiskMB: 64, Setup: SetupPhpBB,
		AdminPath: "/adm/", AdminUser: "admin", ConfigFile: "config.php",
		Nginx: "phpbb", UploadMB: 32,
		Risk: "Forums attract spam: turn on registration questions or e-mail activation before opening registration.",
	},
}
