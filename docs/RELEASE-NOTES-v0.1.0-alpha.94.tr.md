# v0.1.0-alpha.94 (aday, yayımlanmadı)

[English](RELEASE-NOTES-v0.1.0-alpha.94.md)

Bu sürüm v0.1.0-alpha.93'ün ardından gelir. Numarayı, sahibin 2026-10-13
tarihli kalıcı talimatıyla sürümü hazırlayan seçti. 2026-10-17'de,
`feat/alpha94-ready-bundles` dalında, `main` üzerindeki alpha.93 sürüm
kaydından (`0897ee9238bdfee295149adfe8b16968dd6fe0de`) yazıldı.

## Ne değişti

**Hazır paketler (sahibin 2026-10-17 kararı, D-034).** Bileşenler'in en
üstündeki üç "Tek tıkla posta paketleri" kartı kalktı. Yerlerinde her kategorinin
başlığı ile satırları arasında "Hazır paketler" adlı tek bir satır var:

- **Posta:** Temel posta, Web postalı posta, Spam korumalı posta. Mevcut posta
  kurulum penceresi üzerinden, eskisi gibi kurulurlar.
- **Veritabanları:** bir PostgreSQL paketi ve bir MariaDB paketi; biri
  kurulunca tarayıcı aracı (phpPgAdmin, phpMyAdmin) "phpPgAdmin ekle" ve
  "phpMyAdmin ekle" olarak sunulur.
- **Web:** Nginx + PHP-FPM.

Paket, altında sıralanan bileşenlerin üzerinde bir kısayoldur, ayrı bir ürün
değildir: bir veritabanı ya da web paketi eksik bileşenlerini olağan bileşen
kurulumuyla art arda kurar, her biri kendi ilerlemesiyle ("2/3 · PHP-FPM
kuruluyor"). Biri başarısız olursa kendi yönlendirmesi gösterilir ve gerisi
bekler; paket o zaman Devam et düğmesini sunar, bu yalnız eksik olanı kurar.
Kurulu bir paket tek satıra daralır: "… kurulu ve doğrulandı" (posta için küçük
bir bağlantı olarak Onar) ya da Doğrula düğmeli "Kurulu, doğrulanmadı". Panonun
Posta paketleri kartı, neyin kurulu olduğunu sıralayan ve Bileşenler'deki
paketleri açan tek bir sakin "Hazır paketler" kartıyla değişti.

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

- Boston (DNS motorundan başka bir şey kurulu değil): Web sunucuları,
  Veritabanları ve E-posta kategorilerinin her biri paketleriyle başlar.
- Frankfurt: "Web postalı posta kurulu ve doğrulandı · Onar" ve "Spam koruması
  ekle"; "MariaDB paketi kurulu ve doğrulandı" ve "phpMyAdmin ekle"; güvenlik
  duvarı tek satır olarak; Panoda Hazır paketler kartı.

## Sınırlar

- Sahibin 2026-10-16 talimatıyla (hızlı sürümler) gerçek sistem kaydı yok.
  Yalnız Go, bileşen, bağlanmış bileşen ve taklit tarayıcı denetimleri; kurulu
  bir sunucuda hiçbir şey ölçülmedi; paket sırası gerçek bir Agent'a karşı
  çalışmadı.
- Şema 44, geçiş yok; yeni Agent iş türü yok; kurtarma davranışı değişmedi.
  Yönetilen hizmetler yanıtı eklemeli bir `bundles` alanı kazanır; eski bir Panel
  göndermezse sayfa şerit göstermez.
- P0.1-P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez; yayım kabul
  değildir.

## Kanıt

Go `./cmd/panel/` ve `./internal/...` (66 paket): vet ve gofmt temiz; 2843 üst düzey test geçer, 42 atlanır, 4278 alt test geçer, hiçbiri başarısız değil; `./cmd/panel/` içinde 1593 üst düzey test geçer, 6 atlanır (sekiz yeni hazır paket testi; posta profili testleri değişmedi). Web: `npm test` 1575/1575; `tsc --noEmit` ve paket
bütçeli üretim derlemesi geçer. Tarayıcı turu 19 (yalnız taklit): Boston benzeri
ve Frankfurt benzeri taklitlerde Bileşenler, 2/3 adımında bir veritabanı paketi
ve Pano kartı; 1440 ve 390 piksel genişlikte, iki temada, Türkçe; bir düzeltme
turu (kurulu bir paketin yanındaki ipucu satırı, telefonda PHP-FPM'nin bütün
kalması). Sürüm sözleşmeleri geçer: sıra politikası (93 -> 94), indirme portalı,
imzalı bildirim.

## Yayım kaydı

Yayımlanmadı.

Bu sürümü yalnız CelikPanel'in güncelleme arayüzünden kurun. Yayımlamak kurulu
sunucuları güncellemez; bir sunucunun güncellemesini yalnız o sunucunun sahibi,
panelin kendi güncelleme ekranından başlatır.
