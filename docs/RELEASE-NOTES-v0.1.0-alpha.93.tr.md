# v0.1.0-alpha.93 (16 Ekim 2026'da yayımlandı)

[English](RELEASE-NOTES-v0.1.0-alpha.93.md)

Bu sürüm v0.1.0-alpha.92'nin ardından gelir. Numarayı, sahibin 2026-10-13
tarihli kalıcı talimatıyla sürümü hazırlayan seçti. 2026-10-16'da (UTC),
`fix/alpha93-small-truths` dalında, `main` üzerindeki alpha.92 sürüm kaydından
(`961f2cb2fcaeb48c08fc58828d15fe0b231e1e1a`) yazıldı.

## Ne değişti

Sahibin alpha.92 çalıştıran iki kurulu sunucudan aldığı ekran görüntülerinden
(2026-10-16) altı küçük düzeltme; her birine sahip aynı gün karar verdi:

1. **Posta doğrulanmışken Temel Posta artık Onar istemez.** Bir profil, onun
   bütün bileşenlerini içeren doğrulanmış daha büyük bir profil (Webmail ya da
   Spam Korumalı Posta) aynı taramada aynı uzlaştırmayı kanıtlıyorsa doğrulanmış
   sayılır. Yalnız bileşen durumu yine kanıt değildir; süren ya da başarısız bir
   Temel Posta denemesi kendi durumunu korur.
2. **"Web postayı onar" olağan bir düğmedir**, Yönet gibi çizilir. Yalnız sunucu
   bir webmail sorunu bildirdiğinde sarıdır.
3. **Pano doğrulanmış postayı düz sözle söyler:** "Posta bu sunucuda kurulu ve
   doğrulanmış."
4. **Posta bileşeni yoksa "Kısmen kurulu" da yok.** Yalnız Nginx (ya da PHP-FPM)
   varken Posta paketleri kartı nötr tonda "Kurulu değil" der; Bileşenler'deki
   Webmail kartı "Bazı bileşenler zaten kurulu…" notunu soluk tonda tutar. Tek
   bir posta bileşeni (Postfix, Dovecot, Rspamd, Roundcube) profili yine kısmen
   kurulu yapar.
5. **Son etkinlik posta profillerini adıyla anar:** "kurulum sırasında
   protected-mail bileşenini hazırladı" yerine "kurulum sırasında Spam Korumalı
   Posta profilini hazırladı".
6. **Sayılar doğru söylenir:** "1 öğe", "3 öğe" (İngilizcede "1 item",
   "3 items").

## alpha.93'e güncelledikten sonra ne göreceksiniz

- Frankfurt: Temel Posta kartı sakindir, sarı Onar cümlesi yoktur; Panodaki
  Posta paketleri kartı "Posta bu sunucuda kurulu ve doğrulanmış." der.
- Boston: Posta paketleri kartı nötr tonda "Kurulu değil" der.
- İkisinde de: Roundcube'un yanındaki "Web postayı onar" sade bir düğmedir; Son
  etkinlik profil adlarını gösterir; Alan adları "1 öğe" der.

## Sınırlar

- Sahibin 2026-10-16 talimatıyla (hızlı sürümler) gerçek sistem kaydı yok.
  Yalnız Go, bileşen ve bağlanmış bileşen testleri; kurulu bir sunucuda hiçbir
  şey ölçülmedi.
- Şema 44, geçiş yok; kurtarma davranışı değişmedi. API değişikliği yalnız bir
  kural değişikliğidir: bir posta profili artık kendi son denemesi `none`
  iken `verified` olabilir.
- P0.1-P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez; yayım kabul
  değildir.

## Yayım kaydı

Aşağıdaki her şey 16 Ekim 2026 tarihlidir (saat dilimi belirtilmedikçe UTC).
Birleştirmeyi, etiketi, denetimleri, portal yayımını ve genel sitenin okunmasını
sürümü hazırlayan kişi yaptı, önceki sürümlerde olduğu gibi sahibin kalıcı
talimatı uyarınca; kurulu panel güncellemelerini yalnız sahip, panelin kendi
güncelleme ekranından başlatır. Yayım için sahibin talimatıyla gerçek sistem
kaydı yok. Bu girdi, aşağıdaki sonuçların hiçbirini depoda bir dosya olarak
saklamaz.

**Ne oldu.**

1. "Release v0.1.0-alpha.93: six small corrections from the installed servers"
   başlıklı #224 numaralı çekme isteği `main`'e
   `0897ee9238bdfee295149adfe8b16968dd6fe0de` commit'i olarak birleştirildi.
2. **İlk deneme yanlış commit'i etiketledi; hiçbir şey yayımlanmadı.** Sürüm
   zincirinin ilk koşusu çekme isteğini birleştirmeye çalıştı; çekme isteğinde
   çakışma olduğu için birleştirme reddedildi, betik yine de devam etti ve sürüm
   commit'i olmayan bir commit'i etiketledi. Etiket gönderildi, CI'ın politika
   koruması onu reddetti ("tag does not match policy version"). Hiçbir sürüm
   oluşmadı ve hiçbir dosya yayımlanmadı; dışarıda hiçbir şey değişmedi. Etiket
   silindi ve zincir betiği şöyle sertleştirildi: çekme isteği birleştirilmemişse,
   `main` onun birleştirme commit'ine eşit değilse ya da `main`'in politika
   sürümü etiketle aynı değilse betik durur. Bu korumayla yapılan ikinci koşu bu
   sürümü üretti.
3. `v0.1.0-alpha.93` açıklamalı etiketi (nesne
   `c64b60021c3fc6b6bb506e99bdaf6b1354f531cf`, ağaç
   `7b18e5252045eef74264a946d41efcbbc927b545`) sürüm commit'ini gösterir. Sürüm
   bir ön sürümdür (pre-release); 2026-10-16T22:04:09Z'de altı dosyayla
   yayımlandı.
4. Etiketle aynı akşam, `CelikPanel-S2-Debian` WSL konuğunda bir indirmeden
   denetlendi (hazırlayan kişi; burada bir dosyada kayıtlı değil): arşivler bayt
   bayt aynı, 66.240.206 bayt, SHA-256
   `887fbe30a29c6fa364bfafc44ec676414c32e41758417738ff4cecc287f9fc39`; manifest
   sıra 93, `published_at` 2026-10-16T21:21:23Z der (SHA-256
   `e28f451221825d6244d1a34c50bb5ecba366fafc74e9c0342563f7068df7f625`); imzası
   (SHA-256 `f0991824d4e746a5726e3a46255a824ff332f9799797ee67772c72fb993bde8a`),
   imza anahtarına (SHA-256
   `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`, öncekiyle
   aynı anahtar) karşı `openssl pkeyutl -verify -rawin -pubin` ile doğrulanır;
   iki içerik koruması da 0 döndürdü; derleme Go go1.26.5, CGO kapalı,
   `-trimpath` ile yapıldı. `libexec/get.sh` 93'ü sabitler (SHA-256
   `b3f1ce91e127203350b5c73e509f49b62433ee0190b9e9de6c8bcd72bb20cf11`); içindeki
   politika şimdiki 93, önceki 92, önceki commit
   `961f2cb2fcaeb48c08fc58828d15fe0b231e1e1a` der. Arayüz paketi
   `web/dist/assets/app-BskYbaLr.js`, 248.904 bayt, SHA-256
   `5f0db2b45ac3aa0165b367e3a6fac224a6670ac0b86a3d6f4df4ac4c111af46b`.
5. Portal paketi: `portal.tar.gz`, 132.853.434 bayt, SHA-256
   `b27504acc26d7c291d7e0ec3ebfb051ac04996b27af65e78e99c3a0ea934f83c`, 54 girdi;
   etiketin `deploy/build-download-portal.sh` betiğiyle (önceden imzalı kip) iki
   kez bayt bayt aynı derlendi. Sözleşme testi 0 döndürdü; genel doğrulayıcının
   17 testi ve terfi aracının 14 testi geçti.
6. Hazırlayan kişi onu kendi bilgisayarından `deploy/publish-download-portal.ps1`
   ile yayımladı: önceki sürüm v0.1.0-alpha.92, hedef v0.1.0-alpha.93; durum
   `committed`, genel doğrulama 26 istekten 26'sı, evre full,
   `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`. Önceki sitenin yedeği
   `/var/www/vhosts/celikpanel.net/portal-backups/httpdocs.before-v0.1.0-alpha.93.20261016T221039Z.596810.a34abffc5937f8c4`;
   yüklenen paket
   `/var/www/vhosts/celikpanel.net/.upload-portal-20261016T220956Z-v0.1.0-alpha.93-9fc5ae34dd76f014`
   altında saklandı.
7. Yayımdan sonra dışarıdan okuma: `https://celikpanel.net/releases/latest.txt`
   v0.1.0-alpha.93 der; `latest.json` sıra 93'ü, yukarıdaki commit ve SHA-256'yı
   gösterir.

**Sahibin bildirdikleri.** Henüz yok. Bu yazılırken Boston ve Frankfurt hâlâ
alpha.92'deydi; sahip ikisini de alpha.93'e güncellememişti. Burada kurulu bir
sunucudan hiçbir gözlem kaydedilmez ve çıkarılmamalıdır. Altı düzeltme
(Frankfurt'taki Temel Posta kartı, Boston'daki "Kurulu değil", sakin Web postayı
onar düğmesi, Pano cümlesi, Son etkinlikteki profil adları, çoğul sayılar) ilk
kez sahip panelin kendi güncelleme ekranından güncelledikten sonra görülür. Bu
güncelleme alpha.92'**den** başlayan ilk güncelleme olduğu için sakin
"Güncelleme tamamlanıyor" görünümü de ilk kez burada görülebilir.

**Yapılmayanlar ve yayımlamanın değiştirmedikleri.**

- Kurulu sunucuları sürümü hazırlayan kişi güncellemedi.
- Bildirilmeyen ve kaydedilmeyenler: Boston ya da Frankfurt'tan alpha.93'e dair
  her şey ve alpha.92 notlarının bildirilmedi diye saydığı her şey. Her biri
  "kaydedilmedi"dir; "çalıştı" da değildir, "başarısız" da.
- Önceki yayımlama kayıtlarının sınırları ([alpha.88
  notları](RELEASE-NOTES-v0.1.0-alpha.88.tr.md)) kalkmış olarak bildirilmedi.
- Yayımlanma kabul değildir. P0.1–P0.5 kısmi kalır; hiçbir P0 işi kapanmaz ya da
  ilerlemez.

Bu sürümü yalnız CelikPanel'in güncelleme arayüzünden kurun. Yayımlamak kurulu
sunucuları güncellemez; bir sunucunun güncellemesini yalnız o sunucunun sahibi,
panelin kendi güncelleme ekranından başlatır.
