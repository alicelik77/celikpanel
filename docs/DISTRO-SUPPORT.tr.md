# Hangi paket ekosisteminde ne sunuluyor?

<!-- BU DOSYA ÜRETİLİR — elle düzenlemeyin. -->
<!-- Kaynak: internal/core/managed_services.go · Üretmek için: make distro-matrix -->
<!-- Bekçi: internal/core/distro_matrix_test.go (katalog değişip bu dosya üretilmezse test düşer) -->

Bu liste elle yazılmaz; bileşen kataloğundan üretilir. Panelin Bileşenler
sayfası aynı kataloğu okur — yani bu belge ile ekran birbirinden ayrı düşemez.

Kurallar:

- **Dolu hücre bir paket eşlemesidir, bütün ekosistem için kurulum garantisi
  değildir.** Mutasyon ayrıca doğrulanmış paket yöneticisi/systemd, servise özgü
  yaşam döngüsü yetkisi ve varsa kesin vendor artifact veya depo tarifi kanıtını
  gerektirir. Bu son kapılar dağıtım adı değil, gerçek host baytları/yetenekleridir.
- **Boş hücre (—) bir eksik değil, bir sözdür:** katalog o paket yöneticisi
  için güvenli bir eşleme sunmuyorsa satır bilerek kapalıdır. Yarım kurulum kırıktır.
- **Koltuk:** aynı koltuktaki bileşenler aynı işi yapar; aynı anda yalnız biri
  kurulabilir (ör. iki web sunucusu 80 portunu paylaşamaz). Diğerini seçmek
  ekleme değil, değiştirmedir.
- **Gerekenler** ürünü değil ROLÜ adlandırır: "SMTP sunucusu" diyen bir satırı,
  o koltuğun kurulu herhangi bir üyesi tatmin eder.

## Dağıtım paketiyle kurulanlar

### Web

| Bileşen | Tür | APT paket eşlemesi | pacman paket eşlemesi | Koltuk | Gerekenler |
|---|---|---|---|---|---|
| 🐘 PHP-FPM | Çalışma ortamı | `php-fpm` · sürüm seçimi: Sury PHP (packages.sury.org) | `php-fpm` | — | — |
| 🔄 Nginx | Servis | `nginx` | `nginx` | web sunucusu | — |
| 🪶 Apache | Servis | — | — | web sunucusu | — |

### Veritabanı

| Bileşen | Tür | APT paket eşlemesi | pacman paket eşlemesi | Koltuk | Gerekenler |
|---|---|---|---|---|---|
| 🐘 PostgreSQL | Servis | `postgresql` · sürüm seçimi: PostgreSQL Global Development Group (PGDG) | `postgresql` | — | — |
| 🦭 MariaDB | Servis | `mariadb-server` | `mariadb` | — | — |
| 🐬 phpMyAdmin | Araç | `phpmyadmin` | `phpmyadmin` | — | MariaDB, web sunucusu, PHP-FPM |
| 🐘 phpPgAdmin | Araç | `phppgadmin` | — | — | PostgreSQL, web sunucusu, PHP-FPM |

### E-posta

| Bileşen | Tür | APT paket eşlemesi | pacman paket eşlemesi | Koltuk | Gerekenler |
|---|---|---|---|---|---|
| 📧 Postfix | Servis | `postfix` | `postfix` | SMTP sunucusu | — |
| 📮 Exim | Servis | — | — | SMTP sunucusu | — |
| 🧹 Rspamd | Servis | `rspamd` | `rspamd` | spam filtresi | SMTP sunucusu |
| 📬 Dovecot | Servis | `dovecot-imapd` `dovecot-pop3d` `dovecot-lmtpd` | `dovecot` | IMAP sunucusu | — |
| 🛡️ SpamAssassin | Servis | `spamassassin` `spamd` `spamass-milter` | — | spam filtresi | SMTP sunucusu |

### Güvenlik

| Bileşen | Tür | APT paket eşlemesi | pacman paket eşlemesi | Koltuk | Gerekenler |
|---|---|---|---|---|---|
| 🔐 WireGuard VPN | Servis | `wireguard` | `wireguard-tools` | — | — |
| 🔐 Let's Encrypt client (certbot) | Araç | `certbot` | `certbot` | — | — |
| 🚫 Fail2ban | Servis | `fail2ban` | `fail2ban` | — | — |
| 🧱 Firewall engine | Araç | `nftables` | `nftables` | — | — |
| 🦠 ClamAV | Servis | `clamav` `clamav-daemon` | `clamav` | — | — |

### DNS

| Bileşen | Tür | APT paket eşlemesi | pacman paket eşlemesi | Koltuk | Gerekenler |
|---|---|---|---|---|---|
| 🌐 BIND | Servis | `bind9` | `bind` | DNS sunucusu | — |
| ⚡ PowerDNS | Servis | `pdns-server` `pdns-backend-sqlite3` | — | DNS sunucusu | — |

### FTP

| Bileşen | Tür | APT paket eşlemesi | pacman paket eşlemesi | Koltuk | Gerekenler |
|---|---|---|---|---|---|
| 📂 vsftpd | Servis | — | — | — | — |

### Önbellek

| Bileşen | Tür | APT paket eşlemesi | pacman paket eşlemesi | Koltuk | Gerekenler |
|---|---|---|---|---|---|
| ⚡ Redis | Servis | `redis-server` | — | kv-store | — |
| 🗝️ Valkey | Servis | `valkey-server` | `valkey` | kv-store | — |
| 💾 Memcached | Servis | `memcached` | `memcached` | — | — |

### İzleme

| Bileşen | Tür | APT paket eşlemesi | pacman paket eşlemesi | Koltuk | Gerekenler |
|---|---|---|---|---|---|
| 📈 Netdata | Servis | `netdata` · sürüm seçimi: Netdata (repository.netdata.cloud) | `netdata` | — | — |

### Sistem

| Bileşen | Tür | APT paket eşlemesi | pacman paket eşlemesi | Koltuk | Gerekenler |
|---|---|---|---|---|---|
| ⏰ Scheduled tasks (cron) | Servis | `cron` | `cronie` | — | — |

## Kapalı hücreler ve gerekçeleri

Bileşenler sayfası ve Agent aynı gerekçeyi verir (İngilizce metin, ekranda çevrilir).

| Bileşen | Ekosistem | Gerekçe |
|---|---|---|
| 🪶 Apache | apt | Apache virtual-host integration is not available in CelikPanel yet; use Nginx. |
| 🪶 Apache | pacman | Apache virtual-host integration is not available in CelikPanel yet; use Nginx. |
| 🐘 phpPgAdmin | pacman | phpPgAdmin is not offered on Arch Linux: it is not in the official repositories (only in the AUR, which CelikPanel does not use). The server administrator manages PostgreSQL with psql on the server, or with a desktop client through an SSH tunnel. |
| 📮 Exim | apt | Exim mailbox, TLS and submission integration is not available in CelikPanel yet; use Postfix. |
| 📮 Exim | pacman | Exim mailbox, TLS and submission integration is not available in CelikPanel yet; use Postfix. |
| 🛡️ SpamAssassin | pacman | SpamAssassin is not offered on Arch Linux: its Postfix bridge, spamass-milter, is not in the official repositories (only in the AUR, which CelikPanel does not use), so SpamAssassin would filter no mail. The server administrator installs Rspamd, the spam filter in the same seat, from the Components page. |
| ⚡ PowerDNS | pacman | the managed service lifecycle is not supported in this package ecosystem yet |
| 📂 vsftpd | apt | vsftpd passive-mode ports, TLS and per-subscription access are not integrated in CelikPanel yet. |
| 📂 vsftpd | pacman | vsftpd passive-mode ports, TLS and per-subscription access are not integrated in CelikPanel yet. |
| ⚡ Redis | pacman | Redis is not offered on Arch Linux: its official repositories ship Valkey instead (extra/valkey, which provides redis), and CelikPanel installs official distribution packages only. The server administrator installs Valkey, the compatible cache in the same seat, from the Components page. |

## Resmi sürümden kurulanlar (her dağıtımda aynı)

Bu bileşenler dağıtım paketine hiç bağlanmaz: panel, üreticinin yayınladığı
sürümü SHA-256 doğrulamasıyla indirir ve her Linux'ta aynı yoldan kurar
(D-018). Dağıtım sütunu yoktur çünkü cevap her sütunda aynıdır: evet.

| Bileşen | Tür | Koltuk | Gerekenler |
|---|---|---|---|
| 🟩 Node.js | Çalışma ortamı | — | web sunucusu |
| ✉️ Roundcube | Araç | — | SMTP sunucusu, IMAP sunucusu, web sunucusu, PHP-FPM |

