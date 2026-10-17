# v0.1.0-alpha.94 (17 Ekim 2026'da yayımlandı)

[English](RELEASE-NOTES-v0.1.0-alpha.94.md)

Bu sürüm v0.1.0-alpha.93'ün ardından gelir. Numarayı, sahibin 2026-10-13
tarihli kalıcı talimatıyla sürümü hazırlayan seçti. 2026-10-17'de,
`feat/alpha94-ready-bundles` dalında, `main` üzerindeki alpha.93 sürüm
kaydından (`0897ee9238bdfee295149adfe8b16968dd6fe0de`) yazıldı.

## Ne değişti

**Hazır paketler (sahibin 2026-10-17 kararı, D-034).** Bileşenler'in en
üstündeki üç "Tek tıkla posta paketleri" kartı kalktı. Yerlerinde her kategorinin
başlığı ile satırları arasında "Hazır paketler" adlı tek bir satır var:

- **Posta:** Temel Posta, Webmail, Spam Korumalı Posta (ürünün zaten kullandığı
  adlar). Mevcut posta kurulum penceresi üzerinden, eskisi gibi kurulurlar.
- **Veritabanları:** bir PostgreSQL paketi ve bir MariaDB paketi; biri
  kurulunca tarayıcı aracı (phpPgAdmin, phpMyAdmin) "phpPgAdmin ekle" ve
  "phpMyAdmin ekle" olarak sunulur.
- **Web:** Nginx + PHP-FPM.

Paket, altında sıralanan bileşenlerin üzerinde bir kısayoldur, ayrı bir ürün
değildir: bir veritabanı ya da web paketi eksik bileşenlerini olağan bileşen
kurulumuyla art arda kurar, her biri kendi ilerlemesiyle ("2/3 · PHP-FPM
kuruluyor") ve ancak öncekinin, yani kendi işleminin bitmesinden sonra. Biri
başarısız olursa kendi yönlendirmesi gösterilir ve gerisi bekler; paket o zaman
Devam et düğmesini sunar, bu yalnız eksik olanı kurar. Bunun yerine başka bir
işlem çalışırsa ya da sunucuda değişiklikler duraklatılmışsa durur ve bunu
söyler. Yeniden yüklemeden sonra hiçbir şey kendiliğinden sürmez: paket yeniden
sunulur ve yalnız eksik olanı kurar. Kurulu bir paket tek satıra daralır: "…
kurulu ve doğrulandı" (posta için küçük bir bağlantı olarak Onar) ya da "kurulu,
doğrulanmadı"; posta için Onar (eksik olanı tamamlar ve kurulumu doğrular;
kurulu hiçbir şey kaldırılmaz), veritabanı ya da web paketi için Doğrula
(bileşenleri yalnız yeniden okur). Bir posta paketinin son başarısız ya da süren
denemesi altında yeniden söylenir. Panonun Posta paketleri kartı, neyin kurulu
olduğunu ve başarısız ya da süren her posta denemesini nedeniyle birlikte
sıralayan ve Bileşenler'deki paketleri açan tek bir sakin "Hazır paketler"
kartıyla değişti.

**Daha sakin Bileşenler sayfası** (16 Ekim 2026 tasarım incelemesi):

- Bir satırdaki Yeniden başlat, Durdur ve Onar simgeleri, üzerine gelene kadar
  gridir; Durdur'un kırmızısı onay penceresinde kalır.
- Durmuş bir hizmetin noktası kırmızıdır ve "Durdu" yazar; "Kurulu değil" içi boş
  bir halkadır.
- Satırlardaki Kur düğmeleri çerçevelidir; sayfada yalnız bir dolu düğme kalır.
- Güvenlik duvarı açık ve sağlıklıyken tek satırdır: "Güvenlik duvarı açık ·
  Açık port: 4 · Ayarlar". Tam kutu, kapalı, okunamayan ya da ilgi isteyen
  durumlar için kalır.
- Bileşen simgeleri emoji yerine tek bir haritadan çizgi simgeleridir.
- Sürümler ve durumlar sütunlarda hizalanır; telefonda satır düğmeleri 44 piksel
  yüksekliğinde ve adları yazılıdır, sürüm açıklamanın altında gösterilir.

## alpha.94'e güncelledikten sonra ne göreceksiniz

- Boston (Nginx kurulu bir PowerDNS ikincil sunucusu): Web sunucuları, yalnız
  PHP-FPM'i kuracak olan Nginx + PHP-FPM paketini sunar; Veritabanları
  paketlerini sunar. DNS kimliği hazır değilse E-posta, posta paketleri yerine
  bunu söyleyen tek bir satır gösterir (ikincil sunucuda: posta kaydının nerede
  oluşturulduğu); hazırsa E-posta Temel Posta, Webmail ve Spam Korumalı Posta'yı
  sunar.
- Frankfurt: "Webmail kurulu ve doğrulandı · Onar" ve "Spam koruması ekle";
  "MariaDB paketi kurulu ve doğrulandı" ve "phpMyAdmin ekle" (Nginx ve PHP-FPM'i
  zaten kurulu olarak sıralar ve phpMyAdmin'i kurar); güvenlik duvarı tek satır
  olarak; Panoda Hazır paketler kartı.

## Sınırlar

- Sahibin 2026-10-16 talimatıyla (hızlı sürümler) yerel kayıt yok. Yalnız Go,
  bileşen, bağlanmış bileşen ve taklit tarayıcı denetimleri; kurulu bir sunucuda
  hiçbir şey ölçülmedi; paket sırası gerçek bir Agent'a karşı çalışmadı.
- Şema 44, geçiş yok; yeni Agent iş türü yok; kurtarma davranışı değişmedi.
  Yönetilen hizmetler yanıtı eklemeli bir `bundles` alanı kazanır; eski bir Panel
  göndermezse sayfa şerit göstermez.
- P0.1-P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez; yayım kabul
  değildir.

## Kanıt

Go `./cmd/panel/` ve `./internal/...` (66 paket): vet ve gofmt temiz; 2844 üst düzey test geçer, 42 atlanır, 4278 alt test geçer, hiçbiri başarısız değil; `./cmd/panel/` içinde 1594 üst düzey test geçer, 6 atlanır (dokuz yeni hazır paket testi; posta profili görünüm testleri değişmeden geçer). Web: `npm test` 1582 / 1582; `tsc --noEmit` ve paket
bütçeli üretim derlemesi geçer. Tarayıcı turu 19 (yalnız taklit): DNS kimliği
hazır ve hazır olmayan Boston benzeri taklitlerde ve Frankfurt benzeri bir
taklitte Bileşenler, 2/3 adımında bir veritabanı paketi ve Pano kartı; 1440 ve
390 piksel genişlikte, iki temada, Türkçe; bir düzeltme turu (kurulu bir paketin
yanındaki ipucu satırı, telefonda PHP-FPM'nin bütün kalması). Sürüm sözleşmeleri
geçer: sıra politikası (93 -> 94), indirme portalı, imzalı bildirim.

## Yayım kaydı

Aşağıdaki her şey 17 Ekim 2026 tarihlidir (saat dilimi belirtilmedikçe UTC).
Birleştirmeyi, etiketi, denetimleri, portal yayımını ve genel sitenin okunmasını
sürümü hazırlayan kişi yaptı, önceki sürümlerde olduğu gibi sahibin kalıcı
talimatı uyarınca; kurulu panel güncellemelerini yalnız sahip, panelin kendi
güncelleme ekranından başlatır. Yayım için sahibin talimatıyla gerçek sistem
kaydı yok. Bu girdi, aşağıdaki sonuçların hiçbirini depoda bir dosya olarak
saklamaz.

**Ne oldu.**

1. "Release v0.1.0-alpha.94: ready bundles at the head of each Components
   category, one Dashboard card, and calmer Components colours" başlıklı #226
   numaralı çekme isteği `main`'e `d506a05e8c003dffee8de2e1268402865520bd6b`
   commit'i olarak birleştirildi. Sürüm sırası değişkeni etiket yapılmadan önce
   93'ten 94'e geçirildi. `main` koşusu ve etiket koşusu başarılı oldu; sürüm
   zinciri temiz çalıştı (alpha.93'ün aksine olay yok).
2. `v0.1.0-alpha.94` açıklamalı etiketi (nesne
   `d17aa39f13568a6254eeb5f1ab271eccff5eb5be`, ağaç
   `7c1fd69a4dd1c43953037a344f7850dc150922b6`) sürüm commit'ini gösterir. Sürüm
   bir ön sürümdür (pre-release); 2026-10-17T00:30:13Z'de altı dosyayla
   yayımlandı.
3. 17 Ekim 2026'da `CelikPanel-S2-Debian` WSL konuğunda bir indirmeden
   denetlendi (hazırlayan kişi; burada bir dosyada kayıtlı değil): arşivler bayt
   bayt aynı, her biri 66.259.323 bayt, SHA-256
   `bec3c015317fecc226d0a282ecf7e30547e305a621e7fa70943947c6ef0a6788`; manifest
   sıra 94, `published_at` 2026-10-16T23:46:25Z der (SHA-256
   `690d6aa03ee71010206b9f966ec7ac5098ba3b3e36ad539091797b02d9fecc83`); imzası
   (SHA-256 `724c9d440cd4960e6586e5b378fa3f9139ce17bdf0007916ee3fe334102aca1f`),
   imza anahtarına (SHA-256
   `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`, öncekiyle
   aynı anahtar) karşı `openssl pkeyutl -verify -rawin -pubin` ile doğrulanır;
   iki içerik koruması da 0 döndürdü; derleme Go go1.26.5, CGO kapalı,
   `-trimpath` ile yapıldı. `libexec/get.sh` 94'ü sabitler (SHA-256
   `aeb5ac9f97e252c1a08f37a5dbc44cc9615e40839b55d8d16daf6d15109062d1`); içindeki
   politika şimdiki 94, önceki 93, önceki commit
   `0897ee9238bdfee295149adfe8b16968dd6fe0de` der. Arşivde 243 üye var. Arayüz
   paketi `web/dist/assets/app-C9mMpV6A.js`, 249.028 bayt, SHA-256
   `7023aad0b5e28df141d7ffd9ef40311bdcbab594d5c3090adc514cdce2d7bf00`.
4. Portal paketi: `portal.tar.gz`, 132.891.376 bayt, SHA-256
   `b493b72b09e49301fdce059a52ad2d0b92f892dd0e82c1ce1f1550c4b6dcf215`, 54 girdi;
   etiketin `deploy/build-download-portal.sh` betiğiyle (önceden imzalı kip) iki
   kez bayt bayt aynı derlendi. Sözleşme testi 0 döndürdü; genel doğrulayıcının
   17 testi ve terfi aracının 14 testi geçti.
5. Yaklaşık 00:48Z'de hazırlayan kişi onu kendi bilgisayarından
   `deploy/publish-download-portal.ps1` ile yayımladı: önceki sürüm
   v0.1.0-alpha.93, hedef v0.1.0-alpha.94; durum `committed`, genel doğrulama 26
   istekten 26'sı, `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`.
6. Önceki sitenin yedeği
   `/var/www/vhosts/celikpanel.net/portal-backups/httpdocs.before-v0.1.0-alpha.94.20261017T004816Z.630222.f2befd2a7e699711`;
   yüklenen paket
   `/var/www/vhosts/celikpanel.net/.upload-portal-20261017T004735Z-v0.1.0-alpha.94-71537ab34f991196`
   altında saklandı.
7. Yayımdan sonra dışarıdan okuma: `https://celikpanel.net/releases/latest.txt`
   v0.1.0-alpha.94 der; `latest.json` sıra 94'ü, yukarıdaki commit ve SHA-256'yı
   gösterir.

**Sahibin bildirdikleri.** Henüz yok. Bu yazılırken Boston ve Frankfurt hâlâ
alpha.92'deydi. Sahip ikisini de alpha.94'e güncellememişti ve alpha.93'e
güncellediğini de bildirmemişti. Burada kurulu bir sunucudan hiçbir gözlem
kaydedilmez ve çıkarılmamalıdır. Hazır paketler, Pano kartı ve daha sakin
Bileşenler renkleri ilk kez sahip panelin kendi güncelleme ekranından
güncelledikten sonra görülür. Sunucular hâlâ alpha.92'de olduğundan o güncelleme
alpha.93'ün altı düzeltmesini de getirir.

**Yapılmayanlar ve yayımlamanın değiştirmedikleri.**

- Kurulu sunucuları sürümü hazırlayan kişi güncellemedi.
- İncelemede bulunan üç küçük iş bilerek sonraki bir sürüme bırakıldı ve
  açıktır:
  1. Bir paketin kurulum adımları arasında sayfanın yaptığı denetim, sunucunun
     durumunun doğrulanamadığını bildiren bir işareti bir ekran yenilemesi
     boyunca kaçırabilir. Sunucunun hazır olma durumu her adımdan önce yine taze
     okunur ve sunucu kabul etmemesi gereken değişikliği reddeder.
  2. Bileşenler sayfasında Doğrula (bileşenleri yalnız yeniden okur) bir kurulum
     sürerken basılabilir; o zaman kullanılamaz olmak yerine "meşgul" reddi alır.
  3. Sayfa değişiklik kapısını okurken şerit, henüz hiçbir şey gönderilmeden
     "kuruluyor" der (kısa süre).
- Bildirilmeyen ve kaydedilmeyenler: Boston ya da Frankfurt'tan alpha.93 veya
  alpha.94'e dair her şey ve alpha.92 ile alpha.93 notlarının bildirilmedi diye
  saydığı her şey. Her biri "kaydedilmedi"dir; "çalıştı" da değildir,
  "başarısız" da. Paket sırası gerçek bir Agent'a karşı çalışmadı.
- Önceki yayımlama kayıtlarının sınırları ([alpha.88
  notları](RELEASE-NOTES-v0.1.0-alpha.88.tr.md)) kalkmış olarak bildirilmedi.
- Yayımlanma kabul değildir. P0.1–P0.5 kısmi kalır; hiçbir P0 işi kapanmaz ya da
  ilerlemez.

Bu sürümü yalnız CelikPanel'in güncelleme arayüzünden kurun. Yayımlamak kurulu
sunucuları güncellemez; bir sunucunun güncellemesini yalnız o sunucunun sahibi,
panelin kendi güncelleme ekranından başlatır.
