# Müşteriler arası ayrım: dosyalar, bağlantılar ve adlar

*20 Kasım 2026 · Kaynakta var, bileşen testlerinden geçti. Bağlantı kuralı Debian 13'ün kendi
nginx 1.26.3 programıyla, ayrı bir deneme klasöründe de denendi. Debian 13 ve Ubuntu 24.04
üzerinde iki müşteriyle laboratuvar turu hâlâ açık (adımlar sonda).*

İngilizcesi: [TENANT-ISOLATION.md](TENANT-ISOLATION.md).

Sahibin 20 Kasım 2026 kuralı: root yalnız yöneticidedir. Bir bayi ya da müşteri başka bir
müşterinin ya da sunucunun hiçbir şeyini okuyamaz, değiştiremez, listeleyemez. Bu sayfa, bir
müşterinin web sunucusu üzerinden başka bir müşterinin dosyasını okumasına ve bir site hesabının
sunucuda başka hangi sitelerin olduğunu öğrenmesine karşı yapılanları anlatır.

## 1. Sitenin klasöründeki bağlantılar

**Sorun.** nginx'in işçi süreci her sitenin dosyasını web sunucusunun grubuyla (`www-data`,
`nginx` ya da `http`) okur: her site ev dizini `<site hesabı>:<web grubu> 0750`'dir. Bu
değişiklikten önce, müşterinin kendi klasörüne koyduğu bir bağlantı (zamanlanmış görevle, SFTP
ile, PHP'nin `symlink()` işleviyle ya da bir Node.js sitesiyle) nginx tarafından bu grubun
okuyabildiği her dosyaya izleniyordu: başka bir sitenin veritabanı parolalı ayar dosyası, web
postasının ve veritabanı araçlarının gizli bilgileri. nginx'in varsayılanı `disable_symlinks off`'tur.

**CelikPanel'in artık yazdığı.** Bir sitenin dosyalarını sunan her sunucu bloğunda (PHP ve statik
siteler, Node.js sitesinin `public/` klasörü; içlerindeki parolalı klasörler, hata sayfaları ve
sertifika doğrulama konumu dahil):

```nginx
disable_symlinks if_not_owner;
```

nginx bir bağlantıyı yalnız bağlantının ve gösterdiği dosya ya da klasörün sahibi aynıysa izler.
`from=` bilerek yok: `from=<belge kökü>` ile nginx belge kökünün kendisini denetlemez; site hesabı
kendi ev dizininin sahibidir, `public_html`'i başka bir sitenin klasörüne giden bir bağlantıyla
değiştirebilir. `from=` olmadan yolun tamamı denetlenir.

Kaynak: nginx belgeleri, `disable_symlinks` (ngx_http_core_module,
<https://nginx.org/en/docs/http/ngx_http_core_module.html#disable_symlinks>, 20 Kasım 2026'da
okundu). Yönerge nginx 1.1.15'ten beri var; Debian 13 (1.26) ve Ubuntu 24.04 (1.24) ikisinde de
bulunur, dağıtım yapılandırma matrisi üretilen dosyayı her sürümün kendi `nginx -t`'siyle denetler.
Belgeye göre `on` ve `if_not_owner` ek işlem yükü getirir: yolun her parçası açılır ve sahibi
karşılaştırılır, istek başına birkaç sistem çağrısı.

Debian 13'ün nginx 1.26.3 programıyla, iki ayrı hesaba ait sitelerden oluşan ayrı bir deneme
ağacında ölçüldü:

| İstek | Önce | Şimdi |
|---|---|---|
| A sitesinde B'nin dosyasına giden bağlantı | 200, B'nin dosyası | 404 (`try_files` yoksa 403) |
| A sitesinde root'a ait ortak bir dosyaya giden bağlantı | 200 | 404 |
| A sitesinde A'nın kendi dosyasına giden bağlantı | 200 | 200 |
| A'nın `public_html`'i B'nin klasörüne giden bağlantıyla değiştirildi | 200 | 404 (403); belge kökünün yolunu `from=` ile verince 200 |
| A'nın `public_html`'i A'nın başka bir klasörüne giden bağlantı (yayın düzeni) | 200 | 200 |

**Gerçek bir sitede değişen.** Sitenin kendi dosyasına ya da klasörüne giden bağlantı çalışmaya
devam eder. Başka bir hesaba ya da root'a ait dosyaya giden bağlantı (örneğin `/usr/share`
altındaki ortak bir kütüphane) artık sunulmaz: dosyanın bir kopyasını siteye koyun. Yedekten geri
yüklenen bağlantılar çalışır: geri yükleme bağlantıları sitenin hesabına verir.

**Bağlantının neden çalışmadığı nasıl öğrenilir (D-024).** nginx nedeni sitenin hata günlüğüne
yazar: `openat() "<yol>" failed (40: Too many levels of symbolic links)`. Barındırma → Günlükler →
Hata günlüğü bu satırların üstünde satırın ne demek olduğunu ve ne yapılacağını tek düz cümleyle
söyler.

**Sahibin kendi kuralı korunur (D-022).** Sunucu sahibi bir site için `disable_symlinks`'i kendisi
koyduysa (`/etc/nginx/celikpanel-sites.d/<alan adı>/` içinde bir dosya, değiştirdiği bir site
ayar dosyası ya da yöneticinin ek yönergeleri), CelikPanel kendi satırını yazmaz: nginx aynı
blokta yönergeyi iki kez kabul etmez ve sahibin seçimi geçerli kalır.

**Var olan siteler.** Güncellemeden sonra Panel açılırken her sitenin sanal konak dosyası yeniden
üretilir. Hâlâ CelikPanel'in değişmemiş metni olan dosya yeni satırı alır. Sahibin değiştirdiği
dosya olduğu gibi korunur; CelikPanel'in yeni metni yanında `<dosya>.celikpanel-pending` olarak
durur ve site Barındırma → Yapılandırma'da listelenir; ne zaman alınacağına sahip karar verir
(D-031).

Web postasının ve veritabanı araçlarının sanal konakları değişmedi: klasörleri yalnız root'un
değiştirebildiği paket dosyalarıdır, müşteri oraya bağlantı koyamaz. Yalnız doğrulama yapan bloklar
(`mail.<alan adı>`, posta ayar adları) root'a ait bir klasörden yalnız sertifika doğrulamasını sunar.

## 2. Bayi ve müşterilerin ek nginx yönergeleri

Yalnız yönetici sunduysa ve plan içeriyorsa. Bayi ya da müşteri metninde artık reddedilenler
(yöneticinin metni sınırlanmaz):

| Yönerge | Neden |
|---|---|
| `disable_symlinks` | yukarıdaki bağlantı kuralını bir konumda kaldırırdı |
| `autoindex`, `random_index` | nginx belgesine göre `disable_symlinks`'i dikkate almazlar; bir bağlantı üzerinden başka sitenin klasörünü listelerlerdi |
| `xslt_stylesheet` | dosyasını nginx'in ana süreci yapılandırmayı her yüklediğinde root olarak okur |
| `fastcgi_param` / `scgi_param` / `uwsgi_param` `PHP_VALUE` ya da `PHP_ADMIN_VALUE` | php-fpm bunları istek için php.ini ayarı olarak uygular; planın PHP sınırlarını kaldırabilirlerdi ([php.net, FPM yapılandırması](https://www.php.net/manual/en/install.fpm.configuration.php); php-src `sapi/fpm/fpm/fpm_main.c` tam olarak bu ikisini okur) |

`auth_basic_user_file`, sitenin ev dizinindeki bir dosya için izinli kalır. Agent artık metni
yazmadan önce dosyanın `/`'den hiçbir bağlantı izlenmeden ulaşılan düz bir dosya olduğunu da
kanıtlar (openat2, RESOLVE_NO_SYMLINKS); önceden yalnız yolun harfleri denetleniyordu. Sonradan
bağlantıya dönüşen dosya sitenin bir sonraki üretiminde reddedilir. Parolalı klasörler özelliği
bunların hiçbirine gerek duymaz.

Güncellemeden önce kaydedilmiş ve yeni kuralların reddettiği bir metin artık kullanılmaz: bir
sonraki üretimde CelikPanel kendi kopyasını kaldırır (sahibin değiştirdiği kopya korunur), Web
sunucusu ekranı hangi satırın artık izinli olmadığını söyler. Sitenin başka hiçbir şeyi değişmez.

## 3. Bir site hesabının başka siteler hakkında öğrenebildikleri

- **Site başına yapılandırma dosyaları.** Sanal konak dosyaları, CelikPanel'in her site için
  yazdığı ekleme dosyaları (`/etc/nginx/celikpanel-managed.d/<alan adı>/`) ve PHP-FPM havuzları
  artık `0644` değil `0640` yazılır (root ve Agent'ın grubu `celikpanel`). nginx'in ve PHP-FPM'in
  ana süreçleri yapılandırmayı root olarak okur. systemd birimleri `0644` kalır: systemd bir
  birimin ayarlarını D-Bus üzerinden her yerel kullanıcıya zaten söyler.
- **Barındırma kökü.** `/var/www/celikpanel` ve içindeki `subscriptions` klasörü `0711`'dir:
  her hesap içinden geçer, yalnız root listeler.
- **Ek girişler (FTP/SFTP).** Kullanımdaki bir ada, kimde olursa olsun (başka aboneliğin girişi,
  bir sistem hesabı ya da grubu) tek cevap verilir: "the name … cannot be used on this server".

**Var olan sunucular.** Agent güncellemeden sonraki ilk açılışında, hiçbir iş almadan önce kipleri
bir kez düzeltir: hâlâ `0644` olan CelikPanel'in kendi değişmemiş dosyaları `0640`, `0755` olan
barındırma kökü ve abonelikler klasörü `0711` olur. Başka her kip sahibin seçimidir ve korunur;
Agent'ın günlüğü bunu yazar (`tenant file modes: kept …`). Bir makbuz
(`/var/lib/celikpanel-agent-private/tenant-file-modes-v1.done`) düzeltmenin yapıldığını kaydeder;
sahibin sonradan koyduğu kip asla geri çevrilmez. Eski kipleri isteyen sahip için geri alma:
dosyalara `chmod 0644`, iki klasöre `chmod 0755`; CelikPanel sonraki her yazmada dosyanın kipini korur.

**Hâlâ görünenler ve nedeni.** `/etc/passwd` her site hesabını listeler ve hesap adı alan adından
yapılır; `/etc/nginx` ile `/var/log/nginx` içindeki dosya adları ve `/proc`'ta her süreç komut
satırıyla görünür. Bunlar 4. bölümdeki değişiklikleri gerektirir.

## 4. Açık kalanlar (öneri, yapılmadı)

- Her ev dizininde web sunucusunun grubu yerine site başına bir grup; böylece siteler arasında
  bağlantı kuralından başka engel de olur.
- `/proc`'un `hidepid` ile, izleme için bir grupla bağlanması.
- Her sitenin PHP-FPM havuzunun ve zamanlanmış görevlerinin programların aldığı systemd
  korumasında çalışması (site başına dilim, gizli barındırma ağacı, giden posta engeli, sınırlar).
- Site hesaplarının alan adıyla değil numarayla adlandırılması.
- Postfix tabloları (`/etc/postfix/vmailbox`, `virtual`, …) hâlâ `0644`: her posta kutusunu ve
  yönlendirmeyi listeler. Postfix'te okuyan süreçler ve tablo türü sürüme göre değişir; kendi
  laboratuvar kanıtı gerekir.

## 5. Laboratuvar adımları (Debian 13 ve Ubuntu 24.04, iki müşteri A ve B)

1. Güncellemeden önce: A bir zamanlanmış görev ekler:
   `ln -s <B'nin ev dizini>/public_html/<bir dosya> <A'nın belge kökü>/x.txt`;
   `curl https://a.test/x.txt` B'nin dosyasını sunar. `/etc/nginx/sites-available/*.conf`,
   `/etc/php/*/fpm/pool.d/*.conf`, `/var/www/celikpanel` ve `/var/www/celikpanel/subscriptions`
   için `stat -c '%a %n'` çıktısını not edin.
2. Sahip paneli kendi güncelleme ekranından günceller.
3. `curl https://a.test/x.txt` reddedilir (404); A'nın hata günlüğünde satır, Günlükler ekranında
   cümle görünür. A'nın kendi dosyasına giden bağlantı sunulur.
4. A, `public_html`'ini B'nin `public_html`'ine giden bir bağlantıyla değiştirir: reddedilir.
5. Siteler, PHP, web postası, phpMyAdmin/phpPgAdmin, parolalı bir klasör, özel bir hata sayfası ve
   `certbot renew --dry-run` çalışmaya devam eder.
6. 1. adımdaki kipler artık `640`/`711`; sahibin güncellemeden önce değiştirdiği kip aynı kalır;
   Agent'ın günlüğü ikisini de listeler.
7. Yönerge hakkı olan A: `location /x/ { disable_symlinks off; }`, `autoindex on;`,
   `fastcgi_param PHP_ADMIN_VALUE "memory_limit=-1";` ve bağlantı olan bir
   `auth_basic_user_file` her biri düz bir nedenle reddedilir.
8. A, B'ninkiyle aynı adlı bir ek giriş ekler: cevap bu adın kimde olduğunu söylemez.
