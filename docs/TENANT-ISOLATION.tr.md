# Müşteriler arası ayrım: dosyalar, bağlantılar, adlar ve posta imzası

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

## 6. Posta yalnız göndericinin kendi alan adıyla imzalanır

*20 Kasım 2026, sürüm adayının bağımsız incelemesinden sonra 21 Kasım 2026'da düzeltildi ·
Kaynakta var, bileşen testlerinden geçti; dağıtım yapılandırma matrisi CelikPanel'in
`opendkim.conf` dosyasını her sürümün kendi OpenDKIM'iyle denetler, `miltertest` ile (Debian ve
Ubuntu) her deneme iletisini hangi alan adıyla imzaladığını sorar ve CelikPanel'in kendi
kararlarını her sürümün gerçek `postconf` ve `opendkim -n` komutlarıyla çalıştırır. Laboratuvar
turu açık (adımlar aşağıda).*

**Sorun.** OpenDKIM, istemci kimliğini doğrulamışsa ya da sunucunun kendisinden bağlanmışsa,
`From:` satırında barındırılan bir alan adı geçen her iletiyi imzalıyordu ve 25. kapı dahil her
smtpd üzerindeydi. Bir posta kutusu `From:` satırına başka bir müşterinin adresini yazarak posta
gönderebiliyor, bir web sitesinin kodu da iletiyi geri döngü üzerinden 25. kapıya verebiliyordu;
ikisi de bu sunucudan o müşterinin DKIM imzasıyla çıkıyor, alıcılar onu gerçek sanıyordu.

**CelikPanel'in artık koyduğu.**

- `opendkim.conf` içinde `SenderMacro {auth_authen}`: OpenDKIM imza anahtarını `From:` satırına
  göre değil, SASL girişine göre seçer. Kaynaklar: opendkim.conf(5) "SenderMacro"; `opendkim.c`
  (2.11.0, `mlfi_eoh`): boş olmayan makro değeri imzalama tablosunda aranan adrestir, boş değer
  `From:` satırına döner; Postfix `{auth_authen}`'i MAIL FROM ile gönderir (`postconf -d
  milter_mail_macros`). Submission (587/465) giriş ister ve bu sunucuda giriş posta kutusunun tam
  adresidir; gönderilen posta girişin kendi alan adıyla imzalanır ya da hiç imzalanmaz. Zarf
  göndericisi (`{mail_addr}`) kullanılmadı: Postfix boş gönderici için
  `reject_sender_login_mismatch` denetimini atlar (`smtpd_check.c`) ve boş değer `From:`'a döner.
- 25. kapıdan posta alan smtpd (`smtp/inet`, ya da postscreen arkasında `smtpd/pass`) kendi
  `-o smtpd_milters=` ayarını alır: panelin her açılışında `main.cf`'teki liste, sunucu sahibinin
  kendi süzgeçleri dahil, yalnız OpenDKIM çıkarılarak (virgülle, boşluksuz: `master.cf` orada
  boşluk kabul etmez). 25. kapıya gelen hiçbir şey, dışarıdan ya da bir siteden geri döngüyle, bu
  sunucuca imzalanmaz. OpenDKIM artık kurulu değilken (birimi ya da `opendkim.conf` dosyası
  gitmişse) ya da `main.cf` listesinde OpenDKIM kalmadığında ayar, sonraki panel açılışında ya da
  sonraki posta süzgeci adımında (bir bileşenin kurulması ya da kaldırılması) yeniden silinir;
  OpenDKIM'i kendi başına kaldıran bir bileşen ya da düğme yoktur. CelikPanel kendi değerini
  yazdıklarının kaydından tanır (`/var/lib/celikpanel-agent-private/mail-port25-milters.json`);
  kayıt yoksa (22 Kasım 2026) içeriğinden: `main.cf` listesinin OpenDKIM'siz hâline eşit olan ya da
  yalnız bir CelikPanel sürümünün yazdığı girdilerden oluşan değer. Daha önceki bir liste için
  yazılmış değer o zaman sahibinki sayılıp korunur (OpenDKIM'i dışarıda bırakır; sahip onu
  `postconf -PX smtp/inet/smtpd_milters` ile silip paneli yeniden başlatana dek sonradan eklenen
  süzgeç 25. kapıya ulaşmaz). `main.cf` OpenDKIM'i tutar; submission, smtps ve sahibin eklediği
  smtpd'ler onu korur.
- OpenDKIM'in adresi `main.cf` listesinde, Postfix'in aynı uç nokta için kabul ettiği her yazılışla
  tanınır (22 Kasım 2026). Postfix `inet` taşıma adını birebir karşılaştırır (`src/milter/milter8.c`,
  `milter8_connect`) ve gerisini `host_port(buf, &host, "localhost", &port, NULL)` ile ayrıştırır
  (`src/util/inet_connect.c`, `src/util/host_port.c`): `[host]:port` (host `IPv6:adres` olabilir),
  son iki noktadan bölünen `host:port`, boş host `localhost` demektir, ya da yalnız port; adlar ve
  adresler çözülür. Kabul edilenler: `inet:127.0.0.1:8891`, `inet:[127.0.0.1]:8891`,
  `inet:localhost:8891`, `inet:[localhost]:8891`, `inet::8891`, `inet:8891`,
  `inet:[IPv6:::ffff:127.0.0.1]:8891`, `inet:::ffff:127.0.0.1:8891`, host adının her harf biçimi,
  portun baştaki sıfırları. OpenDKIM sayılmayanlar: başka bir adres (`::1` başka bir uç noktadır),
  numara yerine hizmet adı, bir `unix:` soketi.
- `main.cf` listesi süzgeçlerini bir `$değişken` ya da `{ }` grubuyla adlandırıyorsa CelikPanel onu
  tek tek okuyamaz; bu yüzden 25. kapıda hiçbir şeyi değiştirmez (22 Kasım 2026). Bir 25. kapı
  girdisinde kendi önceki değeri varsa o değer geçerli kalır: OpenDKIM'i dışarıda bırakır, ama
  listeye sonradan eklenen her süzgeci de; silinmez, çünkü o zaman 25. kapı OpenDKIM'i
  adlandırıyor olabilecek listeye uyardı. Durum `port25_variable_own`'dur. Böyle bir değer yoksa
  25. kapı listeye uyar ve hâlâ imzalanabilir: `port25_variable`. Şu anki sorunlar her birini
  adımıyla söyler (süzgeçleri `smtpd_milters`'a adlarıyla yazın ve paneli yeniden başlatın), elle
  değiştirilmiş bir girdinin sözleriyle asla söylemez.

**İmzalanmaya devam edenler ve hangi kimlikle.**

| Posta | Kim teslim eder | Hangi alan adıyla imzalanır |
|---|---|---|
| 587/465 üzerinden posta programı ya da web postası; posta kutusu girişiyle gönderen site kodu ya da eklentisi | SASL girişi | girişin kendi alan adı |
| Ofis dışında yanıtları | Dovecot'un Sieve'i, `vmail` olarak, `sendmail` ile | `From:` satırı: kutunun kendi adresi ya da aynı hesabın bir alan adından kutuya yönlendirilmiş ek adres; yanıt kaydedilirken, her açılışta, her saat ve bir adın kimde olduğunu ya da bir adresin nereye gittiğini değiştiren her işlemden hemen sonra denetlenir (aşağıda); böyle bir değişiklikle yeniden yazma arasındaki saniyelerde ya da yeniden yazma yapılamadıkça (Şu anki sorunlar) önceki yanıt hâlâ verilebilir |
| Panelin bildirimleri (davet, parola postası) | Agent, `root` olarak, `sendmail` ile | sahibin ana alan adındaki `no-reply@` |
| Sunucunun cron ve sistem postası | `root`, `sendmail` ile | `From:` alan adı (sunucunun kendi adı) |
| Yönlendirmeler, tümünü yakalama, "kopya tut ve yönlendir" | Postfix'in sanal takma adları, 25. kapıdan gelen iletinin kuyruğu içinde | imzalanmaz; özgün imza dokunulmadan geçer |
| Bir sitenin `mail()` ya da `sendmail` çağrısı | reddedilir (`authorized_submit_users = root, vmail`, alpha.125) | — |
| Bir sitenin girişsiz 25. kapıya verdiği posta | — | imzalanmaz |

Sunucuda girişsiz teslim edilen postanın girişi yoktur; OpenDKIM onu `From:` satırına göre
imzalar. Sunucuya bunu kim yaptırabilir (21 Kasım 2026):

- Müşteriler kendi Sieve betiğini veremez: ManageSieve dinleyicisi kurulmaz (Debian ve Ubuntu'nun
  `dovecot-managesieved` paketi CelikPanel'in kurduklarından değildir), web postasının süzgeç eki
  yoktur, posta klasörleri `vmail`'indir ve yanıt ayarları Agent'ın betiğe çevirdiği alanlardır,
  hiçbir zaman betik metni değil. CelikPanel'in yazdığı tek betik ofis dışında yanıtıdır (Sieve
  `vacation`, `date`, `relational`).
- Yanıtın `From:` satırı kutunun adresi ya da gelen iletinin adlandırdığı ek adrestir
  (Pigeonhole'un `vacation :addresses`'i). Bu düzeltmeden beri ek adres, aynı hesabın bir alan
  adından kutuya yönlendirilmiş olmalıdır; Panel bunu kendi kayıtlarından, Agent da sunucunun kendi
  yönlendirme tablosundan yeniden denetler; ret yalnız adresin kutunun kendi adresi olmadığını
  söyler. Pigeonhole'un `sieve_vacation_use_original_recipient` ayarı kapalı kalır; yönlendirilmiş
  bir kopyanın özgün alıcısı asla `From:` olmaz. Önceden kaydedilmiş yanıtlar güncellemeden sonraki
  ilk açılışta başka adresler olmadan yeniden yazılır; yanıt penceresi hangi adreslerin
  çıkarıldığını bir kez söyler ve denetim kaydına yazılır.
- 22 Kasım 2026 (ikinci inceleme): aynı denetim ve yeniden yazma, bir adın kimde olduğunu ya da bir
  adresin nereye teslim edildiğini değiştiren her işlemin hemen ardından ve içinde çalışır: abonelik
  devri, alan adı silme (posta adımından sonra), alan adı ekleme (içe aktarmayla da), yönlendirme ya
  da catch-all ekleme, değiştirme ya da kaldırma (içe aktarmayla da), posta kutusu ekleme (içe
  aktarmayla da); ayrıca başka her şeye karşı ağ olarak her saat. Her kutu, satırı yeniden okunarak
  Panel'in posta kilidi altında denetlenir; böylece müşterinin aynı yanıtı kaydetmesiyle yeniden
  yazma birbirinin üzerine yazmaz. Agent'ın yapamadığı bir yeniden yazma (yanıt yok, başka sürüm,
  Dovecot durmuş, bir ret), uygulanabildiği yerde yanıtı kapatır (yanıtı kapatmak ne Dovecot'un
  çalışmasını ne adreslerin denetlenmesini ister); pencere o zaman yanıtın kapatıldığını ve
  kaydetmenin onu yeniden açacağını söyler. Bu da uygulanamazsa yabancı adres etkin kalır: Şu anki
  sorunlar kutu başına bir satır gösterir (`mail_autoresponder_foreign`, yönetici, adımıyla: Agent
  yanıt vermiyor, başka sürümde ya da reddetti), pencere bunu söyler ve iş bitene dek her açılışta
  ve her saat yeniden denenir. Olağan bir güncellemeden sonra Agent, Panel'in ilk açılışında asla
  önceki sürüm değildir: `update.sh` iki ikiliyi de kurup doğrular, Agent'ı başlatır ve Panel'i
  başlatmadan önce onu bekler.
- Yönlendirmeler `sendmail`'den ya da OpenDKIM'den yeniden geçmez: `From:` satırı sahte olan ve
  yönlendirme adresine gönderilen dış ileti burada imzalanmaz.
- `authorized_submit_users` yalnız `root` ve `vmail` ile sınırlı değilse (sahibin kendi değeri ya
  da geri alınmış değişiklik), sitelerin programları barındırılan her alan adı adına imzalı posta
  gönderebilir. İmza o zaman yerinde gösterilmez: Şu anki sorunlar bunu yazılacak satırla söyler;
  sahibin dosyasında hiçbir şey değiştirilmez.

Bir posta kutusu bugün takma alan adı adına gönderemez (zarf göndericisi kendi adresi olmalıdır);
bu yüzden orada bir şey değişmez.

**Gelen posta.** OpenDKIM hâlâ imzala ve doğrula kipinde çalışır ama artık 25. kapıda değildir;
gelen postanın imzalarını denetlemez ve ona kendi `Authentication-Results` satırını eklemez.
Rspamd kuruluysa gelen postanın DKIM (ve DMARC) denetimini spam puanı için sürdürür;
spamass-milter imzaları yalnız SpamAssassin'in DKIM eki çalışıyorsa puanlar; spam süzgeci yoksa bu
sunucuda onları hiçbir şey denetlemez. CelikPanel'in hiçbir parçası `Authentication-Results`
satırını okumaz.

**Seçeneği olmayan OpenDKIM.** `SenderMacro` yalnız dağıtım açtığında derlenir. 20 Kasım 2026'da
paketlerde ölçüldü: Debian 11, 12, 13 ve Ubuntu'nun 2.11.0 paketlerinde (24.04:
`2.11.0~beta2-9build4`) var; Arch Linux'un `opendkim 2.11.0beta2-2` paketinde ne bu seçenek ne Lua
var. Agent önce kurulu OpenDKIM'e sorar (yalnız o satırı içeren bir dosyayla `opendkim -n`);
bilmiyorsa CelikPanel önceki yapılandırmayı korur, 25. kapı yine imzaya kapalıdır ve Şu anki
sorunlar orada posta kutusu parolasıyla gönderilen postanın hâlâ `From:` satırına göre
imzalandığını söyler (bilinen sınır). Böyle bir sistemde sahibin `opendkim.conf` dosyası korunur ve
yanına hiçbir şey konmaz: sunucu sahibine OpenDKIM'inin reddettiği bir satırı eklemesi asla
söylenmez.

**Var olan sunucular.** Panelin her açılışında (Dovecot eki ve submission chroot'u ile aynı posta
onarımı adımında), şu sırayla:

1. Süzgeç listeleri: önce `main.cf` (listeler CelikPanel'inse onlarla birlikte
   `milter_default_action = accept`), sonra 25. kapı girdileri (`postconf -P`, ya da silmek için
   `-PX`), ardından `postfix check` ve Postfix'in doğrulanmış biçimde alması gereken bir yeniden
   yükleme. Almazsa yazılan her ayar birebir geri konur (Postfix'in varsayılanında olan ayar
   yazılmaz, yeniden silinir) ve Postfix öncekilerle yeniden yüklenir.
2. `opendkim.conf`: dosya önceki bir sürümün yazdığıyla birebir aynıysa CelikPanel'in yeni metni
   yanına yazılır ve `opendkim -n` ile denetlenir; yeniden başlatma borç olarak kaydedilir; ancak
   sonra metin dosyanın yerine konur, OpenDKIM yeniden başlatılır ve doğrulanır. OpenDKIM onunla
   çalışmazsa önceki dosya geri konur ve OpenDKIM onunla başlatılır. Yarıda kesilen bir açılışın
   borç bıraktığı yeniden başlatma, dosya zaten CelikPanel'inki olsa da sonraki açılışta yapılır;
   OpenDKIM'in çalıştığı doğrulanana dek imza yerinde gösterilmez. Bir alan adının imzasını kurmak
   (`ConfigureDKIMSigning`) 22 Kasım 2026'dan beri aynı sırayı izler: önce tablolar, sonra yeni
   metin dosyanın yanına ve `opendkim -n`, borçlu yeniden başlatma, yer değiştirme ve OpenDKIM onunla
   çalışmazsa önceki metni geri koyan bir yeniden başlatma. OpenDKIM yeniden başlarken iki liste de
   CelikPanel'inse Postfix postayı imzasız geçirir (`milter_default_action = accept`, 1. adım).
   Listelerden biri sahibinkiyse `accept` yazılmaz; onun `milter_default_action` değeri, o da yoksa
   Postfix'in varsayılanı `tempfail` geçerlidir ve panelin notu bunun ne yaptığını söyler: gönderen
   bir posta sunucusu geçici hata alır ve kendiliğinden yeniden dener; 587'deki bir posta programı
   kullanıcısına hata gösterir ve kişi yeniden gönderir.

**Sahibin kendi ayarları (D-022).** Sahibin değiştirdiği `opendkim.conf` yazdığı gibi korunur;
CelikPanel'in metni yanına `opendkim.conf.celikpanel-pending` olarak konur. OpenDKIM'in okuduğu gibi
okunduğunda (satır başına bir `Ad değer`, adlarda büyük/küçük harf fark etmez, `#` açıklamadır)
`SenderMacro {auth_authen}` içeren, başka bir `SenderMacro` değeri ve `Include` içermeyen bir sahip
dosyası yerindedir: imza bağlı sayılır, diğer durumlar olağan biçimde değerlendirilir ve bekleyen metin
silinir (alpha.143 laboratuvarı, 22 Kasım 2026). Sahibin `main.cf`'te
yazdığı süzgeç listesi korunur ve 25. kapı onu OpenDKIM'siz izler. Bir 25. kapı girdisindeki kendi
`-o smtpd_milters` ayarı korunur; OpenDKIM'i adlandırıyorsa bildirilir. OpenDKIM'i ya da süzgeçleri
değiştirebilen her adım (açılış, bir bileşenin kurulması ya da kaldırılması, posta kurulumu, DKIM
imzalama) imza durumunu kaydeder; gelmeyen bir yanıt "bilinmiyor" olarak kaydedilir. Şu anki
sorunlar dosyayı ya da ayarı ve sunucu sahibinin somut adımını söyler; sonraki bir adım her şeyi
yerinde bulunca satır kalkar.

**Laboratuvar adımları** (ikilinin ana sunucusu postayı barındırır, Debian 13'te ve Ubuntu
24.04'te; laboratuvardaki bir alıcı her iletinin `From:` ve `DKIM-Signature: d=` değerini
gösterir):

1. Güncellemeden önce, a yolu: `anna@a.test` posta kutusuyla
   `swaks --server <ana>:587 --tls --auth-user anna@a.test --from anna@a.test --header 'From: ceo@b.test' --to <alıcı>`:
   alıcı `d=b.test` gösterir. b yolu: `a.test`'in bir sitesinden 127.0.0.1:25'e bağlanıp
   `MAIL FROM:<x@b.test>`, `From: x@b.test` ile `a.test`'in alıcıya yönlendiren bir adresine
   gönderin: `d=b.test`. `anna@a.test` üzerinde ek adresi `ceo@b.test` (B müşterisinin alan adı)
   olan ofis dışında yanıtı: dışarıdan `To: ceo@b.test` ile ileti gönderin: yanıt
   `From: ceo@b.test`, `d=b.test` gösterir.
2. Sahip paneli kendi güncelleme ekranından günceller.
3. `/etc/opendkim.conf` SenderMacro satırını içerir; `postconf -P smtp/inet/smtpd_milters` yalnız
   spam süzgecini gösterir (ya da hiçbir şeyi); `ls /var/lib/celikpanel-agent-private/opendkim-restart-owed`
   hiçbir şey bulmaz.
4. a yolu yeniden: `d=a.test`. DKIM anahtarı olmayan bir alan adının girişi: imza yok (`From:`
   alan adı değil). Girişli boş gönderici: `d=a.test`. b yolu yeniden, 127.0.0.1:25'e ve genel
   adrese: imza yok.
5. Başka bir alan adının kimliğiyle web postası: girişin alan adının `d=` değeri. Panel bildirimi:
   sahibin ana alan adının `d=` değeri. Dışarıdan, barındırılan bir alan adının sahte `From:`
   satırıyla yönlendirilen ileti: teslim edilir, imza eklenmez.
6. 1. adımdaki yanıt: yanıt penceresi `ceo@b.test`'in çıkarıldığını bir kez söyler; denetim
   kaydında satırı vardır; aynı ileti yeniden `anna@a.test`'ten yanıt alır (kutuyu adlandırmıyorsa
   hiç almaz). `ceo@b.test`'i yeniden kaydetmek tarafsız cümleyle reddedilir.
7. `postconf -e 'authorized_submit_users = static:anyone'`, panel yeniden başlatılır: Şu anki
   sorunlar yazılacak satırı gösterir; bir siteden `From: ceo@b.test` ile `mail()` `d=b.test` ile
   çıkar (satırın söylediği). `root, vmail`'e geri alınır, panel yeniden başlatılır: satır kalkar.
8. Sahip `main.cf` `smtpd_milters`'a bir süzgeç ekler (örneğin `inet:127.0.0.1:7777`), panel
   yeniden başlatılır: `postconf -P smtp/inet/smtpd_milters` spam süzgecini ve o süzgeci gösterir,
   OpenDKIM'i değil; Şu anki sorunlar 25. kapı satırı göstermez. OpenDKIM'i kaldıran bir bileşen
   yoktur: root olarak `systemctl disable --now opendkim` çalıştırılır ve `/etc/opendkim.conf`
   kenara alınır (`mv /etc/opendkim.conf /root/`), sonra panel yeniden başlatılır: CelikPanel'in
   hiçbir `-o smtpd_milters` ayarı kalmaz. Sonraki adımlardan önce dosya geri konur ve panel
   yeniden başlatılır.
9. Öldürme denemesi: açılış sürerken `opendkim.conf` yer değiştirmesiyle yeniden başlatma arasında
   Agent durdurulur (örneğin `strace` ya da bir kesme noktasıyla, `systemctl kill -s KILL
   celikpanel-agent`), panel yeniden başlatılır: OpenDKIM yeniden başlatılır, SenderMacro ile çalışır
   (`/proc/<pid>` başlangıç zamanı dosyanın değişme zamanından sonra), kayıt silinmiştir.
10. Sahibin tuttuğu bir süzgeç listesi ve `milter_default_action` olmadan, uzlaştırma yeniden
    başlatması sırasında 587 üzerinden posta gönderilir: posta programı geçici hata alır ve kişi
    yeniden gönderir; panelin o açılıştaki günlüğü bunu söyler.
11. Ana sunucunun bir kopyasında: `/etc/opendkim.conf` elle değiştirilir, panel yeniden
    başlatılır: dosya değişmez, yanında `opendkim -n -x` ile başlayan
    `opendkim.conf.celikpanel-pending` vardır, Şu anki sorunlar dosyayı ve satırı söyler, posta
    akar. Sahibin dosyasına `SenderMacro {auth_authen}` eklenir, `systemctl restart opendkim`, panel
    yeniden başlatılır: satır kalkar, `opendkim.conf.celikpanel-pending` silinmiştir, sahibin dosyası
    değişmemiştir (`sha256sum`) ve `authorized_submit_users = static:anyone` ayarlıyken Şu anki
    sorunlar bunun yerine o durumu gösterir.

22 Kasım 2026'da eklendi (alpha.143'ün ikinci incelemesi):

12. A müşterisi `a.test` ve `a2.test` alan adlarını iki abonelikte tutar; `x@a2.test` →
    `anna@a.test` yönlendirmesi; `anna@a.test` üzerinde ek adresi `x@a2.test` olan yanıt.
    `a2.test`'in aboneliği B müşterisine devredilir: devir yanıt vermeden önce yanıt penceresi
    `x@a2.test`'in çıkarıldığını söyler ve denetim kaydında `subscription_transfer` ile satır
    vardır; dışarıdan `anna@a.test`'e `To: x@a2.test` ile gelen ileti `anna@a.test`'ten yanıt alır,
    asla `From: x@a2.test` değil.
13. Aynısı, bunun yerine `a2.test` silinerek (`domain_deleted` satırı) ve yönlendirme kaldırılarak
    (`forwarding_changed` satırı).
14. Dovecot durdurulur (`systemctl stop dovecot`), `anna@a.test` üzerindeki yanıt veritabanında
    (`email_autoresponders`) yeniden `x@a2.test`'i adlandıracak biçimde ayarlanır, panel yeniden
    başlatılır: yanıt kapatılır, penceresi bunu ve kaydetmenin onu yeniden açacağını söyler. Sonra
    aynı satırla Agent durdurulur (`systemctl stop celikpanel-agent`), panel yeniden başlatılır:
    Şu anki sorunlar "anna@a.test: otomatik yanıt hâlâ yabancı bir adresten gidiyor" satırını
    Agent adımıyla gösterir; yanıt penceresi `x@a2.test`'i adlandırır. Agent ve Dovecot başlatılır,
    saatlik tur beklenir (ya da panel yeniden başlatılır): satır kalkar.
15. `postconf -e 'owner_milters = inet:127.0.0.1:8891, inet:127.0.0.1:11332, inet:127.0.0.1:7777'`
    ve `postconf -e 'smtpd_milters = $owner_milters'`, panel yeniden başlatılır:
    `postconf -P smtp/inet/smtpd_milters` hâlâ CelikPanel'in önceki değerini gösterir; Şu anki
    sorunlar "25. kapı, main.cf’e sonradan eklenen filtreleri atlıyor" der, asla "elle
    değiştirildi" demez. Önceki değeri olmayan bir kopyada: 25. kapı için hiçbir şey yazılmaz ve
    Şu anki sorunlar gelen postanın hâlâ imzalanabileceğini söyler. Liste adlarıyla geri yazılır,
    panel yeniden başlatılır: satır kalkar.
16. `postconf -e 'smtpd_milters = inet:[127.0.0.1]:8891, inet:127.0.0.1:7777'`, panel yeniden
    başlatılır: `postconf -P smtp/inet/smtpd_milters` yalnız `inet:127.0.0.1:7777`'yi gösterir;
    b yolunun 25. kapı postası imzalanmaz.
17. `sha256sum /etc/opendkim.conf`; 8891 numaralı port tutulur (`systemctl stop opendkim`, sonra
    `python3 -m http.server 8891 --bind 127.0.0.1`) ve DKIM anahtarı alan bir alan adı eklenir:
    alan adının imza adımı OpenDKIM'in çalışmadığını söyler, `sha256sum` değişmez, Şu anki sorunlar
    imza hatası satırını gösterir. Program durdurulur, panel yeniden başlatılır: satır kalkar.

alpha.141'den önce açılan PostgreSQL veritabanları (`docs/DATABASE-USERS.md`, düzeltme turunun
ikinci incelemesi), aynı sunucularda, A müşterisinin alpha.140 ya da öncesiyle açılmış ve bir gün
izlenmiş eski veritabanı `a_shop` ile (sahibi `a_owner`, kayıtlı okuyucu `a_reader`):

18. A veritabanının satırını tutar: `psql -U a_owner a_shop`, sonra `BEGIN; GRANT CONNECT ON
    DATABASE a_shop TO a_reader;` ve oturumu açık bırakır. Yönetici Kapat'a basar: yanıt yaklaşık
    30 saniye içinde gelir ve veritabanı sunucusunun kapatmadığını ya da sonucun bilinmediğini
    söyler; sayfa onu asla Geri al düğmesi olmadan kapalı göstermez ve oturum açıkken hiçbir şey
    kapatılmaz (`SELECT datacl FROM pg_database WHERE datname = 'a_shop'` değişmez). Yanıttan
    sonra `ps -ef | grep celikpanel-pgpublic` hiçbir istemci göstermez. A oturumu bitirir
    (`ROLLBACK;`); Yeniden oku: sayfa durumu söyler (açık, hatası ve zamanıyla); yeniden Kapat:
    kapalı, Geri al ile.
19. Geri al, sonra 18. adım panelin kendi kapatmasıyla tekrarlanır: oturum açıkken saatlik denetim
    beklenir. PostgreSQL sayfası veritabanını "kapatma gönderildi, sonucu henüz doğrulanmadı" ile
    açık ya da başarısız kapatma olarak gösterir, asla yeniden izleniyor diye göstermez ve sonraki
    denetimde ikinci bir kapatma gönderilmez (`journalctl -u celikpanel` bir tane gösterir).
20. `a_owner` olarak: `ALTER DATABASE a_shop SET default_transaction_read_only = on; ALTER DATABASE
    a_shop SET role = 'a_owner'; ALTER DATABASE a_shop SET lock_timeout = '1ms';`. Yeniden oku:
    veritabanı okunur; Kapat: kapalı; Geri al: yeniden açık. Alan adı sayfasından `a_shop`
    yedeklenip geri yüklenebilir.
21. Küme dört saat durdurulur (ya da bir kopyada `a_shop`, `ALTER DATABASE a_shop ALLOW_CONNECTIONS
    false` ile her bağlantıyı reddeder, sonra `true` ile geri alınır): sayfa nedeniyle "Şu an
    okunamadı" gösterir;
    dördüncü saatlik denetimden sonra Şu anki sorunlar ve uyarı postası onu nedeni ve adımıyla
    okunamadı diye listeler. Küme başlatılır, Yeniden oku: satır kalkar.
22. Kaydı olan bir veritabanı silinir (`SELECT key FROM panel_settings WHERE key LIKE
    'postgresql_public_v1:%'` öncesi ve sonrası, python3'ün sqlite3 modülüyle okunur):
    kaydı onunla birlikte gider.
23. (alpha.143 laboratuvar düzeltmeleri) Bir denetim `a_shop`'u kendiliğinden kapattıktan sonra
    PostgreSQL sayfası Yeniden oku'ya basmadan açılır: kapatmayı gösterir (CelikPanel, zamanı, Geri
    al), veritabanını asla açık göstermez ve hiçbir bulgu boş bir iki noktayla bitmez.
24. Yönetici olarak `a_reader` elle silinir (`a_shop` içinde `REASSIGN OWNED`/`DROP OWNED`
    sonrasında `DROP ROLE a_reader`), sonra Türkçe sayfada Kapat'a basılır: pencere hiçbir şeyin
    değişmediğini, nedenini Türkçe ve veritabanı sunucusunun kendi iletisini tırnak içinde söyler;
    asla "bilinmiyor" demez; listedeki başarısız satırı da aynısını söyler.
25. Bir kapatma geri alınır: `datacl` yazılmış varsayılandır (sırası değişebilir
    `{=Tc/a_owner,a_owner=CTc/a_owner}`); veritabanı "CelikPanel dışında değiştirilmiş" demeden "Bir
    yöneticinin yeniden açtıkları" altında listelenir.
26. Tek yabancı adres adlandıran yanıtın notu tekil, iki adres adlandıranınki çoğul söyler (Türkçe ve
    İngilizce).

## 7. Yan kapılar: veritabanı araçları, web postası, varsayılan PHP-FPM havuzu, `at` ve site crontab'ları

*23 Kasım 2026 (kum havuzu dilimi R1, #348 numaralı istek, dört bağımsız incelemeden sonra) ·
Kaynakta var, bileşen testlerinden geçti; web postası ağacındaki erişim listesi, deneme
sunucusunun ext4 dosya sisteminde gerçek dosyalar, sahipler ve erişim denetimleriyle denendi.
Debian 13 ve Ubuntu 24.04 laboratuvar turu açık (adımlar istekte).*

**Sorun.** phpMyAdmin, phpPgAdmin ve web postası (Roundcube), dağıtımın varsayılan PHP-FPM
havuzunda, grubu her sitenin evini okuyan web sunucusu hesabıyla çalışıyordu; birindeki bir hata
her siteyi okuyordu. Varsayılan havuz, hiçbir şey kullanmasa da açık kalıyordu. Bir sitenin kendi
kodu, sınırlarının dışında çalışan `at` işleri ve crontab satırları kurabiliyor, hiçbir şey
bunları göstermiyordu.

**CelikPanel her Panel başlangıcından sonra bir kez ne yapar (yan kapılar adımı); sonra
durumları her saat ve bir aracın Yönet penceresindeki her taşımadan sonra, hiçbir şeyi
değiştirmeden yeniden okur:**

1. *Her araç kendi hesabında.* `celikpanel-webmail`, `celikpanel-phpmyadmin` ve
   `celikpanel-phppgadmin` (girişi, evi yok, başka hiçbir grupta değil); her birinin bağlı olduğu
   PHP sürümünde kendi havuzu var (`/etc/php/<s>/fpm/pool.d/celikpanel-<araç>.conf`; soketi web
   sunucusu hesabının, 0660). Taşıma, Yönet penceresinin doğrulanan adımıdır: kendi havuzu,
   nginx denetlenip yeniden yüklenir, araç kendisi olarak yanıt vermelidir; vermezse her şey
   geri gelir. Sahibin değiştirdiği yapılandırma korunur (D-022).
2. *Web postası ağacı.* `/var/lib/celikpanel-webmail` kökü, kurulumun bıraktığı gibi
   `root:<web grubu> 0750` kalır, başkalarına kapalıdır. İçinden yalnız web postasının hesabı
   geçer: tek bir erişim listesi girdisiyle, `user:celikpanel-webmail:--x`. Girdi, hiçbir
   bağlantı izlenmeden açılan bir tanıtıcı üzerinde `fsetxattr` ile yazılır (`acl` paketine gerek
   yok). Web sunucusu `public_html`'e (içindeki `plugins`, `skins` ve `program/js` ağacın içine
   bağlantıdır) kökün grubuyla ulaşır. `db/`, `temp/` ve `logs/` hesabın olur; `config/`
   `root:<hesap> 0750` olur; web grubunun okuduğu eklenti yapılandırması hesabın grubunu alır.
   Hiçbir site hesabı dosya sisteminde ağaca giremez; web postasının yerel sunucusu da yalnız
   CelikPanel'in kendi denetimlerine yanıt verir (aşağıya bakın). Erişim listesini kabul etmeyen dosya sisteminde taşıma
   hiçbir şey değişmeden durur: web postası web sunucusu hesabında kalır, Şu anki sorunlar
   nedenini ve sahibin ne yapabileceğini söyler. Web postası güncellemeleri girdiyi yeni ağaca,
   yayımlanmadan önce koyar.
3. *Varsayılan havuz* (`www.conf`): paketin dokunulmamış dosyasıysa (dpkg'nin kendi MD5'i),
   yüklenen hiçbir web sunucusu yapılandırması soketini anmıyorsa ve başka bir havuz kalıyorsa
   kapatılır. "Yüklenen", nginx'in geçerli yapılandırmasıdır (`nginx -T`) ve yalnız Apache
   programı kuruluysa Apache'nin yüklediği dosyalardır: `/etc/apache2` altında `*-available`
   klasörleri dışındaki her dosya, etkin bağlantılar ve bunların içe aldığı her şey. PHP-FPM
   paketinin kendi `/etc/apache2/conf-available/php<s>-fpm.conf` dosyası, standart sunucuda onu
   hiçbir şey etkinleştirmediği için kullanıcı sayılmaz; izlenemeyen bir Apache içe alması (bir
   değişken, sonraki satıra süren bir satır) kendisi kullanıcı sayılır ve satırıyla anılır. Yüklenen yapılandırmada soketi anan, tam satır yorum olmayan her satır;
   aynı sokete başka bir yazılışla ya da bağlantıyla ulaşan (aygıt ve düğüm numarasıyla
   karşılaştırılır) ya da hiç izlenemeyen bir `unix:` yolu; ya da adresi değişken olan bir
   `*_pass` havuzu açık tutar. Tek ayrık, CelikPanel'in mührünü taşıyan ve değişmemiş bir site
   dosyasındaki kendi ters vekil satırıdır, `proxy_pass $celikpanel_upstream;`: bu değişken
   yalnız `http(s)://ad[:port]` tutar. Havuz şu durumlarda da, birim ya da satır anılarak açık
   tutulur: etkin ya da çalışan bir Apache (`apache2` ya da `/etc/apache2-<ad>` klasörü aynı
   biçimde okunan bir `apache2@<ad>` örneği) yüklediği yapılandırmada `.htaccess` dosyalarının bir
   şeyi değiştirmesine izin veriyorsa (`None` dışında bir `AllowOverride` ya da
   `AllowOverrideList`); bir `httpd`, `lighttpd`, Caddy ya da OpenLiteSpeed birimi etkin ya da
   çalışıyorsa (yapılandırmaları okunmaz); birimler listelenemiyorsa. Bakılmayanlar: sokete
   doğrudan bağlanan programlar (işlem kodu önbelleğini sıfırlayan bir cron işi, havuzun durum
   sayfasını okuyan bir izleme aracı), systemd dışında başlatılan bir web sunucusu ve `nginx -T`'nin
   yazdırmadığı kod (bir Lua dosyası). Bunlardan biri havuzu kullanıyorsa sahip havuzu eski adına
   döndürür (aşağıya bakın); CelikPanel onu sonra açık bırakır. Yani yalnız nginx kullanan
   standart bir Debian 13 ya da Ubuntu 24.04 sunucusunda havuz, okunan hiçbir şey onu
   göstermiyorsa ve yanında bir site ya da araç havuzu çalışıyorsa kapatılır. Kapatma, adını `www.conf.off-by-celikpanel` yapmaktır; `php-fpm -t` ve yeniden yükleme ile
   denetlenir, reddedilirse geri konur. Bir sürüm hiç havuzsuz bırakılmaz: son diğer havuzu
   gitmeden önce (site silinir ya da taşınır, araç taşınır ya da kaldırılır) varsayılan havuz geri
   konur. Bir sürümün havuzlarındaki her tutma, yazma, silme, deneme ve yeniden yükleme tek bir
   sunucu kilidi altındadır.
4. *`at`* site hesaplarına kapatılır: at paketi kuruluysa ve `/etc/at.allow` yoksa CelikPanel,
   bugün at kullanabilen her hesabı, site hesapları (ve bir site hesabının kimliğini paylaşan her
   hesap) dışında listeleyen bir dosya yazar. Site hesaplarının bekleyen at işleri listelenir
   (numara, hesap, zaman), asla silinmez.
5. *Site crontab'ları.* CelikPanel'de yapılmamış satırlar sitenin Zamanlanmış görevler
   listesinde işaretlenir (yalnız Sil; yönetici Kalsın diyebilir) ve Şu anki sorunlar'da
   sayılır. Agent böyle bir satırı, tutulmadan değiştirmez ya da etkinleştirmez.

**Diskte tutulanlar** (hepsi root'un, `/var/lib/celikpanel-agent-private/` içinde):

| Dosya | Ne tutar | Ne zaman silinir |
|---|---|---|
| `php-tool-accounts.json` | bir aracın başarısız taşınması (sürüm, neden) | taşıma başarılı olunca |
| `php-tool-files-<araç>.json` | değişen her yolun sahibi, grubu, kipi (özel bitlerle), kökün erişim listesi ve CelikPanel'in koyduğu | araç kendi havuzunda yanıt verince ya da geri alma bitince |
| `stock-pools-off.json` | CelikPanel'in kapattığı sürümler ve başlayıp bitmemiş bir kapatma | dosya olarak hiçbir zaman; bir sürümün satırı sürüm geri konunca gider |
| `at-allow.sha256` | CelikPanel'in `/etc/at.allow` dosyasının özeti, yazarken `pending <yeni> <önceki>` | hiçbir zaman (sahibin silmesi onunla anlaşılır) |
| `site-crontabs/<hesap>.json`, `site-crontabs/baseline-v1` | CelikPanel'in açıkladığı crontab satırlarının özetleri | site hesabı silinince |

Panel kendi kayıtlarını kendi veritabanında tutar: yan kapıların son yanıtını ve yöneticinin
son "Yeniden kapat" işini, zamanıyla (Panel'in yeniden başlamasıyla kesilen iş "bilinmiyor" diye
söylenir).

**Her adım nasıl toparlanır.** Ters sıranın elektrik kesintisini sahibin işi gibi göstereceği her
yerde kayıt, anlattığı değişiklikten önce yazılır: aracın dosyaları değişmeden önce, varsayılan
havuzun "kapatılıyor" işareti adı değişmeden önce, `at.allow` dosyasının bekleyen kaydı dosyadan
önce. Arada elektrik kesilirse bir sonraki başlangıç bunu CelikPanel'in bitmemiş adımı olarak
okur. Bitmiş kayıtlar, doğruladıkları değişiklikten sonra gelir (kapalı diye kaydedilen havuz,
CelikPanel'in diye kaydedilen `at.allow`); satır ekleyen crontab değişikliği de bilerek önce
crontab'ı, sonra kaydını yazar: arada elektrik kesilirse yeni satır CelikPanel'de yapılmamış diye
görünür, hiçbir satır gizlenmez. nginx değiştirilmeden kesilen araç taşıması yeniden denenir ve başarısız olursa ilk denemenin
kaydettiği geri konur; sonra kesilen bir kez yeniden yüklenir ve yeniden sorulur; yine yanıt
vermeyen araç Şu anki sorunlar'da bir satırdır (kim, ne yapar). Geri alma bir yolu yalnız
CelikPanel'in koyduğu gibi duruyorsa geri koyar; taşımanın kendi oluşturduğu dosya (phpMyAdmin'in
`conf.d/celikpanel-own-account.php` dosyası) taşımanın yazdığını hâlâ tutuyorsa silinir; zaten
gitmiş bir yolun geri konacak bir şeyi yoktur ve böyle söylenir; o arada değişen korunur ve adı
söylenir. "Kapatılıyor" diye işaretli varsayılan havuz bitirilir (deneme ve yeniden yükleme) ya da
yeniden karar verilir; reddedilen bir kapatmada havuz, işaret kalkmadan önce eski adına döner;
CelikPanel'in adıyla ama kaydı olmadan yeniden adlandırılmış bulunan havuz, kapalı diye
kaydedilmeden önce denetlenir (`php-fpm -t` ve yeniden yükleme). Kaydı bekleyen `at.allow`
bitirilir ya da yeniden yazılır. Satır kaldıran crontab değişikliği önce o satırsız kaydı yazar.
Saatlik okuma adımın kilidini alır, daha yeni bir yanıtın yerine asla geçmez ve bir aracı ancak
art arda iki deneme başarısız olunca çalışmıyor sayar.

**Sahip her birini panel olmadan nasıl geri alır (D-022).**

- Varsayılan havuz: `www.conf.off-by-celikpanel` yeniden `www.conf` yapılır, PHP-FPM yeniden
  yüklenir. CelikPanel açık bırakır ve listeler; yalnız yöneticinin açıkça "Yeniden kapat" demesi
  (yalnız o havuz) yeniden kapatır.
- `at`: `/etc/at.allow` silinir; yeniden `/etc/at.deny` geçerli olur. CelikPanel dosyayı
  kendiliğinden yeniden yazmaz; "Yeniden kapat" (yalnız o dosya) yeniden yazar.
- Crontab satırı: crontab root olarak düzenlenir. CelikPanel'in yazmadığı satırlar listelenir,
  asla onun tarafından silinmez.
- Yalnız web postasının girdisi: `setfacl -x u:celikpanel-webmail /var/lib/celikpanel-webmail`
  (okumak için `getfacl`). Web postası o zaman dosyalarını okuyamaz; CelikPanel girdiyi
  kendiliğinden geri koymaz ve bunu söyler; web postasının Yönet penceresinde PHP sürümünü seçmek
  girdiyi geri koyar.
- Araçları yeniden web sunucusu hesabına almak. Panelde bunun bir eylemi yoktur (bir aracın PHP
  sürümünü seçmek kendi hesabını korur); elle, bu sırayla, root olarak (`www-data`, Debian ve
  Ubuntu'da web sunucusunun grubudur; `<s>`, bir aracın çalıştığı her PHP sürümüdür):
  1. Önce kapatılmış her varsayılan havuz geri konur, böylece araçların döneceği soket var olur:
     `mv /etc/php/<s>/fpm/pool.d/www.conf.off-by-celikpanel /etc/php/<s>/fpm/pool.d/www.conf && systemctl reload php<s>-fpm`.
  2. Dosyalar, kurulumun koyduğu gibi web sunucusunun grubuna geri verilir
     (`applyRoundcubePermissions`; phpMyAdmin'in iki paket dosyası paketin bıraktığı gibi):
     `chown -R root:www-data /var/lib/celikpanel-webmail`,
     `chmod 0750 /var/lib/celikpanel-webmail`,
     `chmod 0770 /var/lib/celikpanel-webmail/db /var/lib/celikpanel-webmail/temp /var/lib/celikpanel-webmail/logs`,
     `chmod -R g+rwX /var/lib/celikpanel-webmail/db /var/lib/celikpanel-webmail/temp /var/lib/celikpanel-webmail/logs`,
     `chmod 0640 /var/lib/celikpanel-webmail/config/config.inc.php`,
     `setfacl -x u:celikpanel-webmail /var/lib/celikpanel-webmail`;
     `rm /etc/phpmyadmin/conf.d/celikpanel-own-account.php`,
     `chgrp www-data /etc/phpmyadmin/config-db.php /var/lib/phpmyadmin/blowfish_secret.inc.php`.
  3. Web sunucusu yeniden varsayılan soketlere çevrilir, sonra denetlenip yeniden yüklenir:
     `sed -i 's#php\([0-9.]*\)-fpm-celikpanel-[a-z]*\.sock#php\1-fpm.sock#g' /etc/nginx/conf.d/celikpanel-webmail.conf /etc/nginx/conf.d/celikpanel-dbtools.conf /etc/nginx/conf.d/celikpanel-webmail-*.conf`,
     sonra `nginx -t && systemctl reload nginx`.
  4. Araçların kendi havuzları kaldırılır: `rm /etc/php/<s>/fpm/pool.d/celikpanel-*.conf && systemctl reload php<s>-fpm`.
  5. En son hesaplar ve veri klasörleri:
     `userdel celikpanel-webmail; userdel celikpanel-phpmyadmin; userdel celikpanel-phppgadmin; rm -r /var/lib/celikpanel-php-tools`.
- **Panel gidince artık güncel tutulmayanlar:** `/etc/at.allow` son yazıldığı gibi kalır; sonradan
  açılan bir hesap, sahip onu eklemeden `at` kullanamaz. Bir PHP sürümünün son havuzunu kaybetmesini
  de artık hiçbir şey önlemez: varsayılan havuz kapalıyken bir sürümün son diğer havuzunu kaldırmak
  PHP-FPM'in başlamamasına yol açar (önce varsayılan havuzu geri koyun).

**Önceki sürüme dönmek (alpha.146).** Başarısız bir güncellemenin otomatik geri dönüşü yalnız
yeni panel başlatılmadan önce olur (yeni panelin açılış denetimi de o ana kadardır); o sırada R1'in
değişikliklerinin hiçbiri yoktur, çünkü yan kapılar adımı yeni panelin açılışından yaklaşık üç
dakika sonra çalışır. Yeni panel başlatıldıktan sonra güncelleme sonuna kadar götürülür, geri
alınmaz. Tamamlanmış bir güncellemeyi ürün geri almaz: güncelleme yedeği için kurtarma verisi
yazar ve sürümün `rollback.sh` dosyası böyle bir yedeği hiçbir şeyi durdurmadan, adımdan önce de
sonra da, şu satırla reddeder:
`!! a new rollback cannot reuse an earlier transaction's recovery material; preserve the snapshot and use a supported recovery plan`.
`sudo /usr/libexec/celikpanel/recovery recover` yalnız yarıda kalmış bir işlemi tamamlar.
Yöneticinin elinde bunun yerine bu sürümün kendi geri alması vardır: kapatılmış bir varsayılan
havuz adı geri verilerek açılır (yukarıda; CelikPanel onu sonra açık bırakır) ve `at`,
`/etc/at.allow` silinerek yeniden açılır; araçları web sunucusu hesabına geri koyan bir panel işlemi
yoktur (yukarıdaki elle adımlar laboratuvarda denenmedi).

Yalnız eksiksiz olsun diye, alpha.146'nın (alpha.144 ve alpha.145 gibi) bu değişikliğin çalıştığı
bir sunucuda çalışsaydı neyi korumayacağı: bir aracın kendi soketini tanımaz; bu yüzden aracın
çalıştığı PHP sürümünün kaldırılmasını reddetmez ve araçların Yönet pencerelerinde PHP sürümü
görünmez; web postasının kendi güncellemesi başarısız olur ve eski ağacı geri koyar (yeni ağaç
hesabın girdisini ve klasörlerini almaz); varsayılan havuzu kapalı bir PHP sürümünün son sitesini
silmek ya da taşımak reddedilir (alpha.146 varsayılan havuzu geri koymaz); yan kapıların Şu anki
sorunlar satırları görünmez.

**Bir site hesabının hâlâ ulaşabildiği.** Dosya sisteminde web postası ağacının altında hiçbir
şey. Araçların soketleri web sunucusunundur (0660) ve site hesabı o grupta değildir; müşteriler
hiçbir `*_pass` yönergesi yazamaz. Varsayılan soketi zararsız bir yönergede anan müşteri o havuzu
açık tutar ve bundan bir şey kazanmaz.

Web postasının panelin yerel soketindeki sunucusu (`/run/celikpanel-webmail.sock`; nginx Unix
dinleme soketini 0666 kipiyle oluşturur, bu yüzden her yerel hesap bağlanabilir) alpha.145'ten
beri yalnız CelikPanel'in kendi sorduklarına yanıt verir: `/webmail/` (Roundcube'un giriş
noktası, orada çalışan tek PHP), `/webmail/skins/` altındaki stil dosyaları ve CelikPanel'in PHP
denetimi; geri kalan her şey bulunamaz. Bu değişiklikle o PHP web postasının kendi havuzunda,
kendi hesabıyla çalışır. Veritabanı araçlarının `127.0.0.1:8306` üzerindeki sunucusuna
her yerel hesap aynı biçimde, panelin `/dbtool/` yolundan da oturum açmış her panel kullanıcısı
ulaşır: paketlerin kendi dosyalarını (kod, belgeler, örnek SQL; gizli bilgi yok) verir ve `.php`
dosyalarını araçların kendi hesaplarıyla çalıştırır; veritabanlarını araçların kendi girişleri
korur.
