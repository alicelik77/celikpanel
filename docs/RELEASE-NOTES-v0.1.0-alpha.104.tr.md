# v0.1.0-alpha.104 (aday)

[English](RELEASE-NOTES-v0.1.0-alpha.104.md)

Bu, v0.1.0-alpha.103'ü izleyen adaydır. **Yayımlanmadı.** 23 Ekim 2026'da aday
olarak, alpha.103'ün yayın kaydından sonraki `main` üzerinden
`fix/alpha104-safe-removal` dalında yazıldı (alpha.103 sürüm commit'i
`f780d5f08d0f295fbf7d249a853d9c130cd28779`).

## Kim güncellemeli ve neden

Bileşenler'den bileşen ya da PHP sürümü kaldıran herkes ve Roundcube web postası,
phpMyAdmin ya da phpPgAdmin kullanan herkes. 23 Ekim 2026'da Frankfurt'ta (Debian
13, alpha.103) ne oldu: PHP-FPM 8.6, 8.5 ve 8.4 kuruluydu (8.4 Debian 13'ün kendi
PHP'si), phpMyAdmin, phpPgAdmin ve Roundcube ile. Sahip PHP-FPM 8.4'ü kaldırdı.
Pencere yalnızca sitelerin denetleneceğini söylemişti. İki dakika sonra phpMyAdmin
"Kurulu değil", web postası "doğrulanmadı — Roundcube kurulu, ama Unix soketi
çalışma ortamı hazır değil" durumundaydı. Sunucunun apt geçmişi, kaldırmanın
(`apt-get purge -y --auto-remove`) phpMyAdmin'i, dağıtımın PHP paketlerini ve
yaklaşık 35 kütüphaneyi de sildiğini ve üç aracın da PHP 8.4 üzerinden
sunulduğunu gösterdi. Sahip her düzeltmenin bu sürümde çıkmasına karar verdi ("Tüm
düzeltmeleri 104'te yap. 105'e bir şey bırakma.").

Bir sunucuyu yalnız sahibi, panelin kendi güncelleme ekranından günceller.
Frankfurt'un nasıl geri getirileceği [işlem
rehberindedir](OPERATION-GUIDANCE.tr.md).

## Bir kaldırma adını söylemediği hiçbir şeyi kaldırmaz

Herhangi bir paket kaldırmadan önce (bir bileşen, tek bir PHP ya da PostgreSQL
sürümü; apt ve pacman) Agent, hiçbir şeyi değiştirmeden paket yöneticisine
kaldırmanın ne yapacağını sorar. Kaldırabilecekleri: adını söylediği paketler; aynı
sürümün paketleri (`php8.4-…`); ve yalnızca bu kaldırmanın kullanılmaz hâle
getirdiği, otomatik kurulmuş paketler. Başka her şey — başka bir bileşenin paketi
(phpMyAdmin), bileşenin kendi dağıtım paketi (sürümün sistemin varsayılanı olduğu
anlamına gelir), kaldırılana bağlı başka her paket ya da kuracağı her şey —
kaldırmayı adıyla, hiçbir hizmet durdurulmadan reddeder. Daha önce de kullanılmayan
paketlere (eski bir çekirdek) dokunulmaz. Gerçek kaldırma, benzetimi yapılan tam
listeyi, aynı benzetim bir kez daha yapıldıktan sonra ve tek paket kilidi altında
çalıştırır; arada yanıt değiştiyse reddedilir.

Kaldırma pencereleri bunu düğmeden önce söyler: "Paket yöneticisi bu kaldırmayı
denetledi: tam olarak şu N paketi kaldırır."; ya da "Paket yöneticisi bu kaldırmanın
adını söylemediği paketleri de kaldıracaktı; bu yüzden CelikPanel kaldırmayı
reddeder ve hiçbir şey değişmez:" ve her biri için ne yapılacağı ("phpmyadmin,
phpMyAdmin bileşenine ait: önce phpMyAdmin bileşenini kaldırın ya da PHP-FPM 8.4
bileşenini tutun."; "php-fpm — dağıtımın kendi paketi — buna bağlı: PHP-FPM 8.4, bu
sistemin varsayılanı. …"); o zaman Kaldır sunulmaz. Denetim çalışamadığında: "Başka
bir paket işlemi sürüyor; bu yüzden paket yöneticisine şimdi sorulamadı. Yine de
Kaldır'a basabilirsiniz: o anda yeniden denetlenir ve adını söylemediği bir şeyi
kaldıracaksa reddedilir." Başarısız bir okuma kaldırmayı asla tutmaz.

## Her PHP aracı sizin seçtiğiniz PHP sürümünde çalışır

- **Aracın Yönet penceresi** (Roundcube, phpMyAdmin ve phpPgAdmin'in artık bir
  Yönet penceresi var) "PHP sürümü" bölümünü gösterir: hangi sürümde çalıştığı ve
  diğer kurulu PHP-FPM sürümlerinden bir seçim. "PHP 8.6 kullan" aracı taşır:
  eksikse PHP eklentileri o sürüm için kurulur, web sunucusunun yapılandırması
  yazılır, denetlenir ve yeniden yüklenir, sonra araç kendi soketi üzerinden
  kendisi olarak yanıt vermelidir (bir CelikPanel denetimi o PHP-FPM'deki PHP
  sürümünü ve aracın eklentilerini doğrular, sonra aracın kendi sayfası); olmazsa
  önceki sürümüne geri konur ve kart nedenini söyler. Bekleme gösterilir; bu arada
  bir şey yapmanız gerekmez.
- **Artık hiçbir şey bir aracı kendiliğinden taşımaz.** Önceden her yeniden
  yapılandırma alfabetik olarak son PHP soketini seçiyordu (8.9'u da 8.10'dan sonraya
  koyuyordu); bu yüzden daha yeni bir PHP kurup sonra phpMyAdmin'i yeniden kurmak ya
  da web postasını onarmak araçları ona taşıyordu. Artık bir araç, sürümü kurulu
  kaldıkça o sürümde kalır; henüz bağı olmayan araç sistemin varsayılan PHP'sini
  alır.
- **PHP-FPM sürüm listesi** her sürümü kimin kullandığını söyler: "Kullanan: 2 site,
  phpPgAdmin, Roundcube" ya da "Hiçbir site ya da araç kullanmıyor".
- **Araçların çalıştığı bir PHP sürümünü kaldırmak** onları adlandırır ve "PHP
  [sürüm] sürümüne taşı, sonra kaldır" seçeneğini sunar: her araç taşınır ve
  denenir, sonra sürüm kaldırılır; bir taşıma başarısız olursa hiçbir şey
  kaldırılmaz ve pencere neyin taşındığını, neyin geri konduğunu ve sürümün
  kaldığını söyler. Sürümdeki siteler "Alan Adları'nı aç" ile adlandırılır (önceki
  gibi her sitenin ayarlarından taşınırlar).
- **PHP'si eksik bir araç** "Kurulu" değil, nedeni ve çaresiyle "Hazır değil" der.

## Küçükler

- Debian 13'teki phpMyAdmin (`4:5.2.2-really+dfsg-1+deb13u1`) "5.2.2-really" değil
  **5.2.2** okunur (alpha.101 kuralı "really"den sonra bir sürüm bekliyordu).

## Değişmeyenler

Siteler PHP sürümlerini hâlâ kendi ayarlarından seçer. Güncelleyici, şema (44) ve
kurulu her sunucu değişmez; yalnız sahip günceller.

## Aday üzerindeki kanıt

- Agent: paketin tamamı temiz bir bağlama ad alanında, 4.702 test geçti (1.857 üst
  düzey), 29 atlandı, 0 başarısız; APT benzetim çıktısı (Purg, Remv, Inst, yabancı
  mimariler), Frankfurt reddi, sürüm paketleri ve yeni öksüzlerin kabulü, eski
  öksüzlere dokunulmaması, hiçbir unit durmadan ret, son benzetim ile kaldırmada
  aynı argümanlar, tutulan paket kilidi, değişen yanıtın ve başarısız benzetimin
  reddi, pacman çıktısı ve "breaks dependency" reddi, bağlı araçların sürüm
  kaldırmayı reddetmesi, kilidi beklemeyen salt-okur önizleme, bağ kuralları
  (korunur, gitti, varsayılanlar, sayısal sıra), doğrulanan taşıma ile geri alması
  ve türlü retleri, PHP denetimi ve adım politikası için yeni testler.
- Panel ve iç paketler (45 dakika sınırı, CGO kapalı): 7.238 geçti, 44 atlandı, 0
  başarısız, 64 paket; gofmt ve go vet temiz.
- Web: 1.772 test geçti (sürüm kaldırma penceresi, ek kaybı adlandıran bileşen
  kaldırma penceresi, sürüm listesi, hazır olmayan satır, phpMyAdmin sürüm hücresi
  ve aracın PHP sürümü kartı için İngilizce ve Türkçe yeni bağlanmış testler);
  derleme her bütçenin içinde.
- Gerçek sistem: atılabilir bir Debian 13 sanal makinesi (QEMU/KVM, koşudan sonra
  yok edildi): Frankfurt kaldırması apt'nin kendi benzetimiyle phpmyadmin ve
  phppgadmin adıyla reddedildi ve hiçbir şey değişmedi; temiz bir kaldırma tam
  olarak benzetim listesini kaldırdı; phpPgAdmin PHP 8.5'e taşındı ve kanıtlandı,
  kanıtı başarısız olan bir taşıma bayt bayt geri alındı
  ([kanıt](../deploy/e2e/removal-safety/alpha104-debian13-20261023/README.md),
  arşiv SHA-256 `fb17d18c…ef50`). Önceki iki koşu başarısız oldu ve orada kayıtlı
  iki düzeltmeye yol açtı.
- Tarayıcı incelemesi 27. grup: 8 yapılandırmanın her birinde 11 durum (EN/TR,
  açık/koyu, 1440/390); bakıldı, düzeltildi, yeniden bakıldı.
- `archlinux` konuğunda temiz bir klonda: sürüm sırası (103 → 104), indirme portalı
  ve imzalı bildirim sözleşmeleri ile çevrimdışı sürüm kurtarma düzeneği (1.109 test,
  1 atlandı) geçti.

## Bilinen eksikler

- Kurulu bir sunucuda hiçbir şey ölçülmedi.
- pacman kaldırma yolu yalnız bileşen testleriyle kapsanır; DNF erişilmez kalır.
- Roundcube'un taşınması sanal makinede koşturulmadı (aynı kod, kendi soketi).
- Bileşenler satırı bir aracın PHP soketi yoksa "Hazır değil" der; aracın
  eklentileri olmadan çalışan bir PHP taramayla değil, taşımanın kanıtıyla bulunur.
- Kaldırma isteği bir istek kimliğiyle korunmaz (önceki gibi); yinelenen "taşı, sonra
  kaldır" doğrulanmış taşımaları yineler ve kaldıracak bir şey bulmaz.
- Türkçede başarısız bir taşımanın nedeni Agent'ın söylediği gibi (İngilizce)
  aktarılır.
- Bileşen sayfasının geri bağlantısı, Yardım ve günlük Yenile düğmesi telefonda 44
  pikselden küçüktür (bu sürümden önce de öyleydi).
- P0.1–P0.5 kısmi kalır; hiçbir P0 işi kapatılmaz ya da ilerletilmez.
