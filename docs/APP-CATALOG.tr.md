# Uygulama kataloğu

*[English](APP-CATALOG.md) · alpha.118 · Kod: `internal/appcatalog`, `cmd/agent/apps_catalog_*.go`,
`cmd/panel/app_catalog_*.go`, migration 058. Haklar: [PLANS-AND-RIGHTS.tr.md](PLANS-AND-RIGHTS.tr.md) 4.5.*

Bir PHP sitesinin Uygulamalar sekmesi, sitenin boş belge köküne hazır bir web uygulaması kurar:
WordPress (eski kurucusu, değişmedi), PrestaShop, Nextcloud, Drupal, Joomla ve MediaWiki.
Her biri tek bir incelenmiş dosyada sabitlenmiş tek bir sürümdür. phpBB de sabitlenmiştir ama
alpha.118'de sunulmaz: download.phpbb.com panelin kendi User-Agent'ına 403 verir ve panel kendini
başka bir program gibi göstermez. Bu değişene dek listelenmez, sunulmaz, kurulmaz
(internal/appcatalog/pinned.go içinde `Withheld`).

Bu belgedeki kodun yalnız bileşen testleri vardır. Her uygulamanın Debian, Ubuntu ve Arch'ta
gerçek kurulumu sürümün laboratuvar turunda kanıtlanır; o zamana kadar kurucular "yazıldı, gerçek
sunucuda henüz kanıtlanmadı" durumundadır.

## Sabitle ve doğrula kuralı

- `internal/appcatalog/pinned.go` her uygulama için şunları tutar: resmi sürüm arşivinin
  adresi, **SHA-256** özeti, arşiv biçimi ve üst klasörü, boyut sınırları, her sürümde bulunan
  dosyalar, PHP aralığı, PHP uzantıları, veritabanı motoru, disk alanı, kurulum tarifi, yönetim
  adresi, veritabanı parolasını tutan yapılandırma dosyası, nginx kuralları ve kısa bir risk notu.
- Panel uygulamayı yalnız kimliğiyle adlandırır. Agent aynı derlenmiş listeyi okur; Panel ona
  adres ya da özet gönderemez.
- Agent HTTPS ile indirir (yönlendirme yalnız HTTPS'e), SHA-256'yı karşılaştırır ve **uyuşmazsa
  reddeder**: dosya silinir, sitede hiçbir şey değişmez, ekran elle kurulmamasını söyler.
  Kurulum sırasında composer, npm ya da başka bir indirme kullanılmaz: sabitlenen her arşiv
  `vendor` klasörünü zaten içerir.
- Üretici sağlama toplamı yayımlıyorsa sabitlenen özet onunla karşılaştırıldı (Nextcloud ve
  phpBB `.sha256`, Joomla için GitHub'ın varlık özeti, Drupal için drupal.org'un MD5'i).
  PrestaShop ve MediaWiki yayımlamaz (MediaWiki GPG imzası yayımlar); özetleri 3 Kasım 2026'da
  resmi indirmeden alındı. Her girişin `Upstream` alanı hangisi olduğunu söyler.

### Uygulama eklemek ya da güncellemek

1. Resmi arşivi indirin; SHA-256'sını üreticinin yayımladığıyla karşılaştırın.
2. PHP aralığını sürümün kendi gereksinim dosyasından okuyun.
3. Arşivi listeleyin: üst klasör, giriş sayısı, açılmış boyut, sembolik bağlantılar (yalnız aynı
   klasördeki bir dosyayı gösteren bağlantı kabul edilir; kopya olarak yazılır).
4. `pinned.go`'daki girişi düzenleyin. Yeni bir uygulama ayrıca bir kurulum tarifi, nginx
   kuralları, parola döndürme için bir yapılandırma biçimi ve `web/src/lib/rights.ts`'te adını ister.
5. `go test ./internal/appcatalog/ ./cmd/agent/ -run Catalog` ve laboratuvar kurulumunu çalıştırın.

## Kur'a basınca ne olur

1. **Kapılar** (Panel): Uygulama kurucu izni, `app:<kimlik>` sunumu (yönetici ∩ bayi ∩
   abonelik), `component:mariadb`, veritabanı sayısı ve disk sınırı, alan adı başına tek uygulama.
2. **Önkoşullar** (Agent, salt okunur): sitenin PHP sürümü; eksik her PHP uzantısı **onu
   getiren dağıtım paketiyle** (Debian ve Ubuntu'da `php8.3-intl`, Arch'ta `php-intl` ya da
   "php.ini'de açın"); boş belge kökü; sitelerin durduğu yerde boş disk. Kurucu paket kurmaz:
   yönetici Bileşenler'den ya da sistemin paket yöneticisinden kurar. Debian ve Ubuntu'da kart
   ve ret, yöneticiye **PHP <sürüm> uzantılarını Bileşenler'den kur** yolunu sunar: Bileşenler,
   sitenin PHP sürümünün PHP-FPM kurulum onayıyla açılır; önizleme standart uzantı paketlerini
   listeler; yönetici onaylamadan hiçbir şey başlamaz. Dağıtımın kendi PHP sürümü (Debian 12'de
   8.2, Debian 13'te 8.4, Ubuntu 22.04'te 8.1, Ubuntu 24.04'te 8.3) Sury deposu **olmadan**
   dağıtımın arşivinden kurulur (alpha.118); diğer sürümler depoyu ister. Arch tek PHP getirir:
   ret, paketleri ya da `/etc/php/conf.d` içinde açılacak uzantıları adlandırır; bunun için
   panelde henüz düğme yok. WordPress aynı denetimi (en az `mysqli`) veritabanı oluşturulmadan
   önce yapar (alpha.118; lab 2. turuna dek Agent bu denetimi WordPress için reddediyordu, bu
   yüzden hep "sorulamadı" çıkıyordu). Denetim sunucu kilidi almaz, hiçbir şeyi değiştirmez.
   Yanıt alınamazsa nedenini söyler (`APP_PREREQUISITES_UNKNOWN` ve bir neden): **meşgul** (40
   saniyede yanıt yok, genellikle sunucuda başka bir iş sürüyor: kimse bir şey yapmaz, bir iki
   dakika sonra yeniden kurulur), **reddedildi** (Agent isteği reddetti: yeniden denenir,
   yinelenirse ileti bildirilir) ya da Agent'a ulaşılamadı (yönetici çalıştığını denetler).
3. **Veritabanı** ve kullanıcısı, her veritabanı gibi kaydedilir ve sayılır.
4. **Dosyalar**: indirme, doğrulama, WordPress sınırlarıyla kök kullanıcıya özel hazırlık
   klasörüne açma, sitenin kullanıcısına verme, **kurulum kilidi** (her adres "kuruluyor" yanıtı
   verir; uygulamanın web kurucusunu kimse önce bitiremez), WordPress'teki atomik değişimle yayın.
5. **Kurulum**: uygulamanın kendi etkileşimsiz kurucusu **sitenin kullanıcısıyla**, sitenin
   PHP'siyle çalışır. Gizli bilgiler komut satırında yer almaz: komut satırında
   `@@CELIKPANEL:AD@@` belirteçleri olur, önce yüklenen sabit bir PHP dosyası değerleri standart
   girişten okur. Kurucunun çıktısı gizli bilgileri silinmiş olarak döner, günlüğe yazılmaz;
   son satırları başarısız kurulumun yanında gösterilir. Yardımcı klasör açıkça `0755` yapılır,
   çünkü agent `UMask=0027` ile çalışır ve sitenin kullanıcısı dosyayı okuyabilmelidir
   (alpha.118: Joomla kurucusu "Failed opening required … prepend.php" ile 255 koduyla durdu).
   İndirmeler User-Agent'ta CelikPanel'i adlandırır; bazı sunucular Go'nun varsayılanını reddeder.
   Açma, sürüm ağacının içinde kalan göreli bağlantıları (başka bağlantı üzerinden de) kopya
   olarak yazar; mutlak ya da ağaçtan çıkan bağlantı reddedilir.
6. **Kurallar**: uygulamanın nginx kuralları sitenin CelikPanel ekleme dizinine yazılır, sonra
   kurulum kilidi kaldırılır.

Üretilen yönetici parolası bir kez gösterilir; sahip kaydettiğini onaylayana kadar (ya da yanıt
kaybolduysa bir kez gösterilene kadar) yalnız mühürlü saklanır.

### Hata, yeniden deneme ve kurtarma

- 3. adımdan önceki ret hiçbir şeyi değiştirmez ve kimin ne yapacağını söyler.
- 3. adımdan sonraki doğrulama ya da indirme hatası işlemin veritabanını (yalnız işlemin sahiplik
  kanıtıyla) kaldırır ve `failed` kaydeder.
- Başarısız kurulum dosyaları ve kurulum kilidini bırakır: `setup_failed`; **Kurulumu yeniden
  dene** yalnız 5–6. adımları aynı gizli bilgilerle yeniden çalıştırır; bitmiş kurulum anlaşılır ve
  tekrarlanmaz.
- Kaybolan yanıt `needs_review` olur (kurulumda `setup_failed`; kurulum güvenle tekrarlanır).
  Panel yeniden başladığında yarıda kalan kurulum `setup_failed`, diğer yarıda kalan adımlar
  `needs_review` olur. Hiçbir şey tahmin edilmez; durum okumak hiçbir şeyi değiştirmez.
- Bir alan adında tek canlı uygulama olur; ikinci kurulumu Panel ve kısmi tekil dizin reddeder.

## Kaldırma

Onay için alan adı yazılır. Agent belge kökünün uygulamayı hâlâ tuttuğunu denetler (yoksa hiçbir
şey kaldırılmaz), dosyaları **sitenin kullanıcısıyla** siler, karşılama sayfasını geri koyar,
nginx kurallarını ve kurulum kilidini kaldırır (sahibin düzenlediği dosya korunur ve adı söylenir).
Veritabanı yalnız işaretlenirse, Nextcloud'un veri klasörü yalnız o da işaretlenirse silinir.

Uygulamalar klasörleri salt okunur yapar (Drupal'ın `sites/default`'u): sitenin kullanıcısı
önce kendi klasörlerinde `u+rwx` iznini geri verir (bağlantı izlenmez). Kaldırma, ilk dosya
silinmeden önce köke özel bir kayda yazılır. Silme yine yarıda kalırsa yer tutucu sayfa kalanların
yanına (hiçbir dosyanın üzerine yazmadan) konur, satır "… kaldırılırken yarıda kaldı" der
(`needs_review`, tür `remove`, kod `APP_REMOVE_INCOMPLETE`) ve silinemeyen yolu gösterir;
**Kaldır**'a yeniden basmak, uygulamayı tanıtan dosyalar gitmiş olsa da işi sürdürür. Kayıt,
kaldırma başladığında belge kökündeki üst düzey öğelerin yalnız **adlarını** tutar (içerik yok);
sürdürülen kaldırma yalnız bunlardan hâlâ var olanları siler. Kaldırma başladıktan sonra sahibin
koyduğu her şey sahibinindir ve kalır: kaldırma yine biter (`removed`) ve korunan öğeleri
adlandırır. Bu kayıt yoksa uygulamayı artık tutmayan belge kökü yine reddedilir.

## nginx kuralları (yerel, panel olmadan okunur)

`/etc/nginx/celikpanel-managed.d/<alan>/app-<kimlik>.conf`. Vhost bu dizini sitenin askı, bakım
ve yalnız yönlendiren blokları dahil her sunucu bloğunda okur; bu yüzden kurallar kendi başına
hiçbir şey sunmaz: `root` yok, `fastcgi_pass` yok; yalnız ret, yönlendirme ya da vhost'un kendi
PHP konumuna iç yeniden yazım; yalnız düzenli ifade ya da tam eşleşme konumları. Her değişiklikten
önce nginx denetlenir, reddederse dosya geri konur.

## Askı, güvenlik acil durumu, devir

- **Askı** ve **güvenlik acil durumu** siteye uygulanır; uygulamanın kuralları bunu aşamaz,
  kurulum kilidi de 503 verir. Askıdaki abonelik kurulum yapamaz.
- Acil durumun **toplu taşıması** başka PHP sürümüne geçerken uygulamanın PHP aralığını henüz
  denetlemez (açık iş).
- **Devir** sonrası parola döndürme ve veritabanı kullanıcısının **Yeni parola**'sı artık
  uygulamanın kendi yapılandırma dosyasındaki parolayı da, kurucunun yazdığı biçimde, yeniler;
  başka biçimde hiçbir şey değişmez ve bu söylenir.

## Uygulamaya özel notlar

| Uygulama | Sürüm | PHP | Not |
|---|---|---|---|
| PrestaShop | 9.2.0 (Classic 9.2.0-6.0) | 8.1–8.5 | Classic sürüm, açıldığında PrestaShop hizmetlerine bağlanan PrestaShop SA modülleri içerir. Yönetim klasörü `admin<rastgele>` olur; install klasörü silinir. Örnek ürün yok. Dostça adresler kapalı (bu sürümün yazmadığı görsel yeniden yazım kuralları gerekir). |
| Nextcloud | 35.0.2 | 8.3–8.5 | Veri `<site evi>/nextcloud-data` içinde, belge kökü dışında. Arka plan işleri sahip cron ekleyene kadar AJAX'ta. |
| Drupal | 11.4.11 | 8.3+ | Standart profil; `trusted_host_patterns` alan adına ayarlanır. |
| Joomla | 5.4.10 | 8.1+ | Joomla 6 PHP 8.3 ister, Debian 12'de çalışmaz; 5.4 desteklenir. |
| MediaWiki | 1.43.11 (LTS) | 8.1+ | Kurulumdan sonra web kurucusu (`mw-config`) kapalıdır. Sahip `$wgGroupPermissions`'ı değiştirene kadar herkes düzenleyebilir. |
| phpBB | 3.3.19 | 7.2+ | Kurulumdan sonra install klasörü silinir. |
| WordPress | en yeni | 7.4+ | Kurucu değişmedi. |

Bu sürümde yok: **Matomo** (çekirdeğinde desteklenen komut satırı kurucusu yok; web kurucusu
siteyi ilk bitirene açık bırakırdı) ve **Moodle** (sitenin PHP'sinde `max_input_vars` ≥ 5000 ve
bir cron işi ister; bu sürüm bu site ayarlarını yapmaz). Yarım çalışan uygulama sunulmaz.

## Hizmet uygulamaları (ileride)

Modelde `kind` alanı vardır; buradaki her giriş `php-site`'dır. Uzun süre çalışan Node/Python
uygulamaları (`service`) aynı `app:<kimlik>` sunum alanını paylaşır ve ayrı bir iş akışıdır.
