# v0.1.0-alpha.97 (aday, 21 Ekim 2026)

[English](RELEASE-NOTES-v0.1.0-alpha.97.md)

Bu, v0.1.0-alpha.96'nın ardından gelen adaydır. Henüz yayımlanmadı ve hiçbir kurulu
sunucu ona güncellenmedi. Numarayı, sahibin 2026-10-13 tarihli kalıcı talimatıyla
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
aşağıdaki sunucu satırıyla bunu denetler; …") ve sunucunun ilk satırı bunun
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
- **Kritik açılış yolunda bütçeden yaklaşık 1 KiB kaldı.** Güncelleme penceresi
  sayfaların üstünde bağlandığından yeni metinleri açılış metnidir. Sınır
  yükseltilmedi.
- **Boston'un alpha.95 güncellemesinin sonucu** bildirilmedi.
- Masaüstünde bileşen sayfası başlığı sürüm satırıyla biraz daha yüksektir, bu
  yüzden eylem düğmeleri başlık satırının biraz altında durur; telefonda uzun bir
  korunan cümle üç satır tutar. Okunabilir; değiştirilmedi.
- P0.1-P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez; aday kabul
  değildir.

## Kanıt

Aşağıdakilerin hepsi 2026-10-21'de aday ağacında bir geliştirme ortamında çalıştırıldı;
hiçbiri kurulu bir sunucuda çalıştırılmadı.

- Go: `./cmd/panel/` ve `./internal/...` 7153 geçti, 0 başarısız, 44 atlandı; Agent
  `./cmd/agent/` 4652 geçti, 0 başarısız, 29 atlandı, önceki Agent çalıştırmalarında
  olduğu gibi `CelikPanel-S2-Debian` WSL konuğunda temiz bir bağlama ad alanında
  (boş Agent durumu, `celikpanel` grubu yok) çalıştırıldı; `go vet` ve gofmt temiz
  (`-timeout 45m`). Yeni Agent testleri dpkg ve pacman ayrıştırıcılarını (normal,
  eksik paket, kaldırılmış ve yarım kurulmuş paketler, birkaç mimari, tuhaf
  karakterler, aşırı uzun metin, epoch), katalog eşlemesini (üst paket ile sürümlü
  paket, birkaç paket, bulunmayan bileşenler), tek sabit sorguyu, zaman aşımını,
  meşgul ve okunamayan yoklamayı (sorgu yapılmaz), kilit tablosu okumasını ve çıktı
  taşımayan hataları kapsar. Yeni Panel testleri RPC politikasını (her ailede bir
  okuma; benzer hiçbir değiştirme kabul edilmez), kısa biçimi, birden çok sürümü,
  bilinmeyeni, meşgul koruma durumunu, eski önbellek satırlarını ve sürümün tarama,
  önbellek ve iki yönetilen hizmet yanıtından geçişini; ve sınırlı, temizlenmiş
  güncelleme tanısını kapsar.
- Web: `npm test` 1671 geçti, 0 başarısız; `npm run build` bütçeler içinde başarılı
  oldu: en büyük Türkçe ekran parçası (setup) 80 KiB'nin 55,84'ü, components 64
  KiB'nin 44,26'sı, settings 44 KiB'nin 31,24'ü; kritik açılış yolu 361 KiB'nin
  359,95'i. Bağlanmış testler sürüm sütununun yalnız bir sürüm biliniyorsa
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
