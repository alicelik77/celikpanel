# v0.1.0-alpha.97 (21 Ekim 2026'da yayımlandı)

[English](RELEASE-NOTES-v0.1.0-alpha.97.md)

Bu sürüm v0.1.0-alpha.96'nın ardından gelir. 21 Ekim 2026'da yayımlandı (sonda
"Yayım kaydı"na bakın); okuduğu sürümler henüz bir kurulu sunucuda görülmedi.
Numarayı, sahibin 2026-10-13 tarihli kalıcı talimatıyla
sürümü hazırlayan seçti. 2026-10-21'de, `feat/alpha97-component-versions`
dalında, alpha.96'nın yayım kaydından sonraki `main` üzerinden yazıldı (alpha.96
sürüm kaydı `08faa91e5a8c6622072b40f0f87c9c388035c9a3`).

## Kim güncellemeli ve neden

Bu sürüm yayımlandığında alpha.96'yı ya da daha eski bir sürümü çalıştıran herkes.
Sahibin 2026-10-17 tarihli alpha.94 ekranlarında Bileşenler sayfası, tek sürümlü her
bileşenin sürümü olarak bir çizgi gösteriyordu; alpha.95 o sütunu kaldırdı ve "her
bileşen için kurulu paket sürümlerini oku"yu açık bıraktı. Bu sürüm onları okur
(D-036). Ayrıca Frankfurt'un alpha.95 güncellemesinin neden durduğunu açıklar ve
sayfanın bunu nasıl söylediğini düzeltir: sunucunun paket yöneticisi Debian'ın
otomatik paket güncellemeleriyle meşguldü. Bir sunucuyu yalnız sahibi, panelin kendi
güncelleme ekranından günceller.

## Ekranda ne değişir

**Bileşenler'de gerçek sürümler.** Kurulu bir bileşenin satırı, paket veritabanının
bildirdiği sürümü kısa biçimiyle, mono yazı tipinde söyler: `1.26.3-3+deb13u1`
için Nginx `1.26.3`, `1:11.8.3-0+deb13u1` için MariaDB `11.8.3`. İpucunda tam sürüm
ve "sistemin paket veritabanından" vardır. Masaüstünde sürümler kendi sütunlarında
durur; bu sütun yalnız sayfadaki kurulu bileşenlerden en az birinin sürümü
biliniyorsa çizilir; hiçbiri bilinmiyorsa sayfa alpha.95'teki gibi görünür,
çizgilerden oluşan bir sütun olmadan. Telefonda sürüm açıklamanın altındadır.
PHP-FPM ve Node.js kurulu kopyalarının sürümlerini korur.

**Her bileşen sayfasında tam sürüm.** Bileşenin adının ve durumunun altında: "Kurulu
sürüm: 1:11.8.3-0+deb13u1" ve "Sistemin paket veritabanından okundu." Bu, genel
bileşen sayfasında da özel olanlarda da (Nginx, Postfix, Fail2ban, PowerDNS,
MariaDB, PostgreSQL…) aynıdır.

**Bilinmeyen, bilinmeyen olarak kalır.** Kurulu olmayan ya da sürümü okunamayan bir bileşen
hiçbir şey söylemez: çizgi yok, "varsayılan" yok, tahmin yok. Paketleri farklı
sürümler bildiren bir bileşen (örneğin SpamAssassin ve milter'i) satırında "birden
çok sürüm" der ve sayfası her paketi sürümüyle adlandırır. Sunucunun paket yöneticisi
son bileşen denetiminde meşgulse, önceki okuma gösterilir ve öyle söylenir
("Sistemin paket veritabanından {time} tarihinde okundu. Son denetimde paket
yöneticisi meşguldü, bu yüzden bu önceki okuma gösteriliyor; sonraki denetim yeniden
okur.").

**Bir güncellemenin neden durduğu, sade sözlerle.** Frankfurt'un alpha.95
güncellemesi 2026-10-17'de sunucunun paket yöneticisi meşgul olduğu için durdu
(Debian'ın otomatik paket güncellemeleri çalışıyordu). Durdurma penceresi artık bunu
söyler: "Neden: sunucunun paket yöneticisi meşguldü, örneğin otomatik güvenlik
güncellemeleri çalışıyordu. Hiçbir şey değiştirilmedi." ve "Paket yöneticisi işini
bitirince {target} güncellemesini yeniden başlatın; Ayarlar'daki güncelleme kartı
paket yöneticisinin ne zaman boşaldığını söyler. Hiçbir işlem kendiliğinden
başlamaz." Güncelleme kartının böyle bir önceki deneme hakkındaki bildirimi aynı
şeyi sakin bir dille söyler ve sunucu meşgulken zaten sessizce yeniden okuyan hazır
olma denetimi, paket yöneticisi boşaldığında "Paket yöneticisi işini bitirdi.
Güncelleme yeniden başlatılabilir." der. Sunucunun kurtarma aracının başlatılamadığı
ya da doğrulanamadığı bir durmanın artık kendi nedeni vardır ("Neden: sunucunun
kurtarma aracı başlatılamadı ya da kurulumu doğrulanamadı. Sunucu yöneticisi
aşağıdaki sunucu iletisine bakarak bunu denetlemeli; …") ve sunucunun ilk satırı bunun
altındadır. Uzun bir tanı satırı artık atılmaz: panel ilk satırını, en çok 240
karakter olarak, ürünün kendi yollarının dışındaki yollar, adresler, atama değerleri
ya da belirteç benzeri diziler olmadan tutar. Bir adımın sözleri hâlâ yüklenirken
neden satırı genel neden yerine "Neden okunuyor…" der.

## Altta ne değişir

- **Yeni bir salt-okur Agent yeteneği.** `Agent.InstalledPackageVersionsV1`, bileşen
  taraması başına bir kez, tüm katalog için tek sorguyla sistem paket veritabanına
  sorar: Debian ve Ubuntu'da sabit biçimli `dpkg-query -W`, Arch'ta `pacman -Q`.
  Sabit argümanlar, kabuk yok, ağ yok, 10 saniyelik zaman aşımı ve 8 MiB çıktı
  sınırı. Sorulan paketler Agent'ın kendi kataloğudur. Paket yöneticisini bir paket
  işlemi tutarken asla çalışmaz: önce süreç tablosunu ve çekirdeğin kilit tablosunu
  okur, kendisi kilit almaz ve paket yöneticisi kullanımdayken ya da durumu
  okunamıyorsa "meşgul" yanıtı verir.
- **Panel** yöntemi Agent RPC politikasında bir okuma olarak kabul eder (diğer tarama
  okumaları gibi), tarama başına bir kez çağırır ve taramayı asla başarısız kılmaz,
  yanıtı mevcut bileşen tarama önbelleğiyle birlikte eklemeli bir JSON alanı olarak
  saklar. alpha.97'den önce yazılmış bir önbellek satırı bilinmeyen olarak okunur.
  Şema değişikliği yok.
- **Ekran kataloğu bölündü.** İki Türkçe ekran kataloğu dosyası boyut sınırlarının
  0,29 ve 1,00 KiB yakınındaydı. Ekran metinleri artık her dilde ekran grupları
  boyunca dokuz parçadır (setup, sites, siteConfig, dnsCertificates, components,
  mailDatabases, security, dashboard, settings), her birinin en az %25 pay bırakılarak
  belirlenmiş kendi sınırı var; yeni bir metnin nereye gideceği ve bir parçanın
  yeniden nasıl bölüneceğine ilişkin kural `web/scripts/check-bundle-budget.mjs`
  başlığındadır. Her parça bir ekran çizilmeden önce yine yüklenir ve bölünmede
  hiçbir metin değişmedi.
- **Tarayıcı taklitleri** sürümleri yalnız yeni alanların bildireceği biçimde
  bildirir; Frankfurt biçimli veriler için örnek Debian 13 metinleri, Boston biçimli
  veriler için Ubuntu 24.04 metinleri kullanılır ve örnek veri olarak etiketlenir.

## Bağımsız incelemenin düzelttikleri (2026-10-21)

- dpkg'nin kurulu dediği her paket sayılır (mevcut durum "i", hata bayrağı yok),
  tutulan ya da kaldırma için işaretlenen de; ilk biçim yalnız "ii" sayıyordu.
  Yeniden kurulması gereken bir paketin sürümü yine gösterilmez.
- Sürüm sorgusu sabit bir ortamla (`LC_ALL=C`, `LANG=C`, sabit bir `PATH`) kendi
  süreç grubunda çalışır; 8 MiB tavanını aşan bir yanıt sorguyu hemen durdurur ve
  "okuma tavanından büyük" diye bildirilir; artık sorgu sonuna dek çalıştıktan sonra
  genel bir hata olarak bildirilmez.
- Meşguliyet denetimi çekirdeğin kilit tablosundan en çok 1 MiB okur ve inode'u
  olmayan kilitleri atlar; öldürülmüş bir pacman'in bıraktığı kilit dosyası, çalışan
  pacman yoksa sonraki her denetimi artık "meşgul" tutmaz.
- Bir günden uzun korunan okuma şunu ekler: "Paket yöneticisi bir günden uzun
  süredir her denetimde meşgul. Sunucuda bir kurulum ya da güncelleme çalışmıyorsa
  sunucu yöneticisi takılı kalmış bir paket işlemi olup olmadığına bakmalı."
- Bileşenler sayfasının kendi taraması sürümleri panelin değişiklik kirasını almadan
  önce okur; paket veritabanı yanıt verirken kira tutulmaz.
- Durma penceresi sonsuza dek beklemez: yüklü kopyada olan neden hemen gösterilir,
  diğerleri en çok 10 saniye "Neden okunuyor…" der, sonra açılış kopyasının kendi
  cümlesini gösterir. Bu sürümün yeni neden metinleri açılış kopyasından ayarlar
  ekran parçasına taşındı (kritik açılış yolu 361 KiB'ın 359,20'si).
- Sunucunun satırı `:`, `,`, `;`, `>`, `|`, ters tırnak ya da `@` sonrasındaki yolu
  da yol sayar, `/` ile bölünmüş base64 benzeri bir değeri gizler (`/` ile başlayan
  bir dizi yalnız ürünün kendi dizinlerindeyse korunur) ve `mode=0775`'i okunur
  bırakır.
- Türkçe: `compatibility` nedeni "… aşağıdaki sunucu iletisine bakarak bunu
  denetlemeli" der; sürümleri farklı paketler "Paketlerinin sürümleri farklı:" diye
  okunur.

## Ne değişmez

- Güncelleyicinin değiştirme yolu: `update.sh`, `get.sh`'ın davranışı, Agent'ın
  sistem güncelleme işçisi, geri alma ve kurtarma. Agent değişikliği salt-okur bir
  yetenek ve RPC politikasıdır. Bu yüzden sahibin 2026-10-16 talimatıyla gerçek
  sistem kaydı yapılmaz.
- Şema 44, geçiş yok. Yeni Agent iş türü yok. Sürüm okuması hiçbir paketi kurmaz,
  yükseltmez ya da yenilemez ve bir sürümden güncelleme önerisi çıkarılmaz.
- Barındırılan siteler, e-posta, DNS, veritabanları, zamanlanmış işler ve sertifika
  yenileme sürüm okumasına bağlı değildir.
- Kurulu paneller yalnız sahipleri tarafından, panelin kendi güncelleme ekranından
  güncellenir.

## Bilinen açıklar

- **Kurulu bir sunucuda hiçbir şey ölçülmedi.** Sürüm okuması Go testlerinde sahte
  bir çalıştırıcıya ve kilit tablosu düzenek dosyalarına karşı, tarayıcı grubunda
  bir taklit sunucuya karşı çalıştı; Boston ve Frankfurt'ta gerçek
  `dpkg-query`/`pacman` yanıtı henüz görülmedi. "Her bileşen için kurulu paket
  sürümlerini oku" açık işi ancak sahip sürümleri orada gördüğünde kapanır.
- **Kritik açılış yolunda bütçeden 1,8 KiB kaldı** (361 KiB'ın 359,20'si; bu sürümden
  önce 2,99 KiB kalmıştı). Yalnız bir nedeni bekleyen satır yeni açılış metnidir;
  nedenler ekran metnidir. Sınır yükseltilmedi.
- **Boston'un alpha.95 güncellemesinin sonucu** bildirilmedi.
- Masaüstünde bileşen sayfası başlığı sürüm satırıyla biraz daha yüksektir, bu
  yüzden eylem düğmeleri başlık satırının biraz altında durur; telefonda uzun bir
  korunan cümle üç satır tutar. Okunabilir; değiştirilmedi.
- **İki bileşen, tasarım gereği diğerlerinden farklı okunur.** SpamAssassin her
  zaman "birden çok sürüm" der: katalog paketleri arasında başka bir kaynaktan gelen
  ve kendi sürümü olan `spamass-milter` vardır. WireGuard satırı Debian ve Ubuntu'da
  `wireguard` üst paketinin (Arch'ta `wireguard-tools`) sürümünü gösterir, çekirdek
  modülününkini değil.
- **Veritabanı anlık görüntüsü olmadan alpha.97'den alpha.96'ya dönüş:** alpha.96'nın
  Bileşenler sayfası, Pano'su ve bileşen sayfaları önbellek satırını okur (yeni
  alanları yok sayar), ama alpha.96'nın Mağazası bilinmeyen alanları reddeder ve
  alpha.96'nın sonraki bileşen taraması satırı bu alanlar olmadan yeniden yazana dek
  "Bu sunucunun bileşenleri için saklanan kayıt okunamadı" der (o tarama bir yönetici
  Pano'yu, Bileşenler'i ya da bir bileşen sayfasını açık tuttuğunda kendiliğinden
  gelir). Hiçbir şey kaybolmaz; o taramayla düzelir. Bir Go testi alpha.96'nın iki
  çözücüsünü de yansıtır.
- P0.1-P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez; aday kabul
  değildir.

## Kanıt

Aşağıdakilerin hepsi 2026-10-21'de aday ağacında bir geliştirme ortamında çalıştırıldı;
hiçbiri kurulu bir sunucuda çalıştırılmadı.

- Go: `./cmd/panel/` ve `./internal/...` 7156 geçti, 0 başarısız, 44 atlandı; Agent
  `./cmd/agent/` 4657 geçti, 0 başarısız, 29 atlandı, önceki Agent çalıştırmalarında
  olduğu gibi `CelikPanel-S2-Debian` WSL konuğunda temiz bir bağlama ad alanında
  (boş Agent durumu, `celikpanel` grubu yok) çalıştırıldı; `go vet` ve gofmt temiz
  (`-timeout 45m`). Yeni Agent testleri dpkg ve pacman ayrıştırıcılarını (normal,
  eksik paket, kaldırılmış ve yarım kurulmuş paketler, birkaç mimari, tuhaf
  karakterler, aşırı uzun metin, epoch), katalog eşlemesini (üst paket ile sürümlü
  paket, birkaç paket, bulunmayan bileşenler), tek sabit sorguyu, zaman aşımını,
  meşgul ve okunamayan yoklamayı (sorgu yapılmaz), kilit tablosu okumasını ve çıktı
  taşımayan hataları kapsar; incelemeden sonra ayrıca tutulan, kaldırma için
  işaretlenen ve yeniden kurulum gerektiren dpkg durumlarını, küçük bir çıktı
  tavanının ve küçük bir zaman aşımının ötesine giden gerçek bir süreci (süreç grubu
  kalmaz, yanıt "çok büyük"tür), sabit ortamı, inode'u olmayan bir kilidi ve aşırı
  uzun bir kilit tablosunu kapsar. Yeni Panel testleri RPC politikasını (her ailede bir
  okuma; benzer hiçbir değiştirme kabul edilmez), kısa biçimi, birden çok sürümü,
  bilinmeyeni, meşgul koruma durumunu, eski önbellek satırlarını ve sürümün tarama,
  önbellek ve iki yönetilen hizmet yanıtından geçişini; ve sınırlı, temizlenmiş
  güncelleme tanısını kapsar.
- Web: `npm test` 1675 geçti, 0 başarısız; `npm run build` bütçeler içinde başarılı
  oldu: en büyük Türkçe ekran parçası (setup) 80 KiB'nin 55,84'ü, components 64
  KiB'nin 44,49'u, settings 44 KiB'nin 31,93'ü; kritik açılış yolu 361 KiB'nin
  359,20'si. Bağlanmış testler sürüm sütununun yalnız bir sürüm biliniyorsa
  görünmesini, kısa ve tam biçimleri, birden çok sürümü, bilinmeyenin hiçbir şey
  göstermemesini, bileşen sayfası başlığını, meşgul paket yöneticisi durmasını
  (Frankfurt'un sahip olduğu kayıt biçimiyle), `compatibility` adımını, sözlerini
  bekleyen nedeni, güncelleme kartının bildirimini ve "boş"/"hâlâ meşgul" satırını ve
  her ekran parçasının her anahtarının İngilizce ve Türkçe gerçek sağlayıcıdan
  çözülmesini kapsar.
- Sürüm-kurtarma düzeneği (güncelleme kartının metinlerini ve sonuç mantığını
  yansıtır): `archlinux` WSL konuğunda temiz bir klonda
  `python3 -m unittest discover -s deploy/e2e/release-recovery -p 'test_*.py'`: 1109
  test, OK (1 atlandı). Meşgul paket yöneticisi durmasının yansıması sakin duruma
  güncellendi.
- Tarayıcı 22. grubu (taklit sunucu): Boston biçimli ve Frankfurt biçimli veriyle
  Bileşenler, bilinen ve bilinmeyen sürümlerle, ve bileşen sayfaları (Nginx, birden
  çok sürümlü SpamAssassin, korunan okumalı BIND, hiç sürümü olmayan phpMyAdmin),
  İngilizce ve Türkçe, açık ve koyu, 1440 ve 390 px'te. İlk bakış, özel bileşen
  sayfalarının (Nginx, Postfix, Fail2ban, PowerDNS) sürümü hiç göstermediğini buldu,
  çünkü sürüm genel sayfanın özet kartındaydı; her bileşen sayfasının paylaştığı
  başlığa taşındı. İkinci bakış: her sayfada sürüm satırı, ham anahtar yok, Türkçe
  sayfada İngilizce yok, 390 px'te yatay taşma yok, sürümde sinyal rengi yok, hiçbir
  sürüm bilinmiyorsa sütun ve çizgi yok.
- Sürüm sözleşmeleri (sıra politikası 96'dan sonra 97, indirme portalı, imzalı
  bildirim): sıra politikası (Git Bash) "96 -> 97" geçti; indirme portalı ve imzalı
  bildirim (WSL konuğu) geçti.

Bileşen, bağlanmış ve taklit tarayıcı testleri gerçek sistem davranışını kanıtlamaz.

## Yayım kaydı

Aşağıdaki her şey 21 Ekim 2026 tarihlidir (saat dilimi belirtilmedikçe UTC).
Birleştirmeyi, etiketi, denetimleri, portal yayımını ve genel sitenin okunmasını
sürümü hazırlayan kişi yaptı, önceki sürümlerde olduğu gibi sahibin kalıcı
talimatı uyarınca; kurulu panel güncellemelerini yalnız sahip, panelin kendi
güncelleme ekranından başlatır. Yayım için sahibin talimatıyla gerçek sistem
kaydı yok. Bu girdi, aşağıdaki sonuçların hiçbirini depoda bir dosya olarak
saklamaz.

**Ne oldu.**

1. Sürümden önce üç bağımsız inceleme turu yapıldı. Bulguları düzeltildi: bir
   paketin tutulduğunu, kaldırıldığını ya da yarım kurulduğunu gösteren dpkg
   durumları artık sayılıyor; çıktı sınırının etiketi ve küçük bir sınırı aşan
   gerçek bir süreci çalıştıran testler; sonsuza dek bekleyebilen bir neden
   satırı; temizleyicinin yolları işlemesindeki boşluklar; ve alpha.96'nın
   Mağazasının bir alpha.97 kaydını nasıl okuduğuna dair not (yukarıda
   anlatıldı).
2. "Release v0.1.0-alpha.97: real versions for every component, a readable
   reason when an update stops, and room in the catalogues" başlıklı #232
   numaralı çekme isteği `main`'e `0c1a8917ef3f8141702fa41ae085f64d62a713f0`
   commit'i olarak birleştirildi. Sürüm sırası değişkeni etiket yapılmadan önce
   96'dan 97'ye geçirildi. `main` koşusu ve etiket koşusu başarılı oldu; sürüm
   zinciri hiçbir olay olmadan çalıştı.
3. `v0.1.0-alpha.97` açıklamalı etiketi (nesne
   `e5c64bcc3569513050ebfc89c01185a69cabbfb0`, ağaç
   `67db021959466f61a2defc232194d897c1eb9cb7`) sürüm commit'ini gösterir. Sürüm
   bir ön sürümdür (pre-release); 2026-10-21T20:24:20Z'de altı dosyayla
   yayımlandı.
4. 21 Ekim 2026'da `CelikPanel-S2-Debian` WSL konuğunda bir indirmeden
   denetlendi (hazırlayan kişi; burada bir dosyada kayıtlı değil): arşivler bayt
   bayt aynı, her biri 66.312.385 bayt, SHA-256
   `3784fa392b016a9fbd18ffc372f7017c8c2aab5d3643bf319c25e53514800aa3`; manifest
   sıra 97, `published_at` 2026-10-21T19:40:33Z der (SHA-256
   `276a972a492beed16524f65f3eb29e2fe590fcbc32a3a9c996ec7063b974c027`); imzası
   (SHA-256 `5acfb875718e99791bfdf5811a3371ade1f6d1ca92aeac8d63d809a62cf2927d`),
   imza anahtarına (SHA-256
   `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`, öncekiyle
   aynı anahtar) karşı `openssl pkeyutl -verify -rawin -pubin` ile doğrulanır;
   iki içerik koruması da 0 döndürdü; derleme Go go1.26.5, CGO kapalı,
   `-trimpath` ile yapıldı. `libexec/get.sh` 97'yi sabitler (SHA-256
   `08ece9d28b3caba0cb94c8bd3659de138fd357352b8b2a5df82f6c31da6cf5bc`); içindeki
   politika şimdiki 97, önceki 96, önceki commit
   `08faa91e5a8c6622072b40f0f87c9c388035c9a3` der. Arşivde 257 üye var (233
   dosya, 24 dizin). Arayüz paketi `web/dist/assets/app-Ckmq5jcD.js`, 256.924
   bayt, SHA-256
   `f94433074b4451430875fda814d0dd371df99eae938c6111a41fb609ff719861`.
   `bin/agent` içinde `InstalledPackageVersionsV1` var.
5. Portal paketi: `portal.tar.gz`, 132.999.066 bayt, SHA-256
   `61ade15eee9ecd7e136d0eee599534a96c628d6bb38ae2365c0c74f56e339197`, 54 girdi;
   etiketin `deploy/build-download-portal.sh` betiğiyle (önceden imzalı kip) iki
   kez bayt bayt aynı derlendi. Sözleşme testi 0 döndürdü; genel doğrulayıcının
   17 testi ve terfi aracının 14 testi geçti.
6. Yaklaşık 20:36Z'de hazırlayan kişi onu kendi bilgisayarından
   `deploy/publish-download-portal.ps1` ile yayımladı: önceki sürüm
   v0.1.0-alpha.96, hedef v0.1.0-alpha.97; durum `committed`, genel doğrulama 26
   istekten 26'sı, `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`.
7. Önceki sitenin yedeği
   `/var/www/vhosts/celikpanel.net/portal-backups/httpdocs.before-v0.1.0-alpha.97.20261021T203644Z.2234083.ade8d257d372e677`;
   yüklenen paket
   `/var/www/vhosts/celikpanel.net/.upload-portal-20261021T203602Z-v0.1.0-alpha.97-69cbd3f06d952c28`
   altında saklandı.
8. Yayımdan sonra dışarıdan okuma: `https://celikpanel.net/releases/latest.txt`
   v0.1.0-alpha.97 der; `latest.json` sıra 97'yi, yukarıdaki commit ve
   SHA-256'yı gösterir.

**Sahibin bildirdikleri.** 21 Ekim 2026'da sahip, Boston'un v0.1.0-alpha.96'da
(yapı 08faa91e5a8c) olduğunu gösteren bir Ayarlar ekran görüntüsü gönderdi.
Frankfurt'un alpha.95 güncellemesinin durması için sahip, 21 Ekim 2026'da
kendisine verilen iki salt-okur komutu çalıştırdı; saklanan satır `!!
CELIKPANEL_UPDATE_FAILURE code=package_manager_busy state=unchanged reason=the
host package manager is active detail=` idi (2026-10-17T12:25:23Z). Debian'ın
otomatik paket güncellemeleri çalışıyordu; sunucuda hiçbir şey değişmedi.
alpha.97 ekranın bunu söylemesini sağlar (yukarıdaki "Ekranda ne değişir"
bölümüne bakın; burada yinelenmez). Frankfurt'un bundan sonraki güncellemesi bu
yazılırken bildirilmemişti. Sahip ayrıca GitHub hesabının adını değiştirdi
(celikbros → alicelik77). Depo artık `alicelik77/celikpanel`; eski
`celikbros/celikpanel` adı GitHub tarafından emekliye ayrıldı ve kalıcı olarak
yönlendirilir. Commit'ler sahibin seçtiği kimlikle yapılır.

**Yapılmayanlar ve yayımlamanın değiştirmedikleri.**

- Kurulu sunucuları sürümü hazırlayan kişi alpha.97'ye güncellemedi. Onları
  yalnız sahip, panelin kendi güncelleme ekranından günceller.
- Bu sürümün okuduğu sürümler henüz bir kurulu sunucuda görülmedi. "Her bileşen
  için kurulu paket sürümlerini oku" açık işi ancak sahip bunları Boston ve
  Frankfurt'ta gördüğünde kapanır.
- Kritik açılış yolunda bütçenin 1,8 KiB'ı kaldı.
- Genel portal hâlâ `celikbros/...` GitHub adreslerine bağlanıyor.
  Yönlendirilirler ve portal bir sonraki değiştiğinde güncellenecekler.
- Ayarlar için menü düzeni önerisi (içeriği) sahibin kararını bekliyor.
- Bildirilmeyen ve kaydedilmeyenler: Frankfurt'un alpha.95 durmasından sonraki
  güncellemesi, Boston'un alpha.96'ya nasıl geçtiği, Boston ya da Frankfurt'tan
  alpha.97'ye dair her şey ve önceki notların bildirilmedi diye saydığı her şey.
  Her biri "kaydedilmedi"dir; "çalıştı" da değildir, "başarısız" da.
- Önceki yayımlama kayıtlarının sınırları ([alpha.88
  notları](RELEASE-NOTES-v0.1.0-alpha.88.tr.md)) kalkmış olarak bildirilmedi.
- Yayımlanma kabul değildir. P0.1–P0.5 kısmi kalır; hiçbir P0 işi kapanmaz ya da
  ilerlemez.

Bu sürümü yalnız CelikPanel'in güncelleme arayüzü üzerinden kurun. Yayımlamak kurulu
sunucuları güncellemez; bir sunucunun güncellemesini yalnız sahibi, panelin kendi
güncelleme ekranından başlatır.
