# v0.1.0-alpha.137

[English](RELEASE-NOTES-v0.1.0-alpha.137.md)

alpha.136 laboratuvarında bulunan iki düzeltmeden oluşan küçük bir sürüm.
Ubuntu'da panel güncellemesinin ilk basışı, hiçbir şey gerçekte paket
değiştirmezken "sunucunun paket yöneticisi meşguldü" diyerek durabiliyordu.
Artık güncelleme kısa bir paket işini en çok bir dakika bekler, neyi
beklediğini söyler ve kendiliğinden sürer.
**Kurulu panelleri yalnız sahip, panelin kendi güncelleme ekranından
günceller.**

## alpha.136'dan güncelleme

1. Her zamanki gibi panelin kendi güncelleme ekranından güncelleyin. Bu
   sürüm panelin veritabanı yapısını değiştirmez (alpha.136'daki gibi
   kalır); sonradan alpha.136'ya dönmek veritabanı geri yüklemesi
   gerektirmez.
2. Düzeltme, alpha.137'yi kuran güncellemenin kendisindedir; bu güncellemede
   de işe yarar. Güncelleme yine "sunucunun paket yöneticisi meşguldü"
   diyerek durursa sunucu gerçekten bir dakikadan uzun süre paket kuruyor ya
   da denetliyordu: o iş bitince **Güncelle**'ye yeniden basın.
3. Güncellemeden sonra basılacak ya da ayarlanacak bir şey yok.

## Sizin için ne değişiyor

### Sorun neydi

- **Ubuntu'da ilk basış.** Güncelleme hazırlanırken sunucunun hizmet
  tanımlarını (systemd) birkaç kez yeniden yükler. Ubuntu'da ve Debian'da
  böyle bir yeniden yükleme sistemin günlük paket denetimini hemen
  başlatabilir (`apt-daily`: paket listelerini yeniler ve Ubuntu'da Ubuntu
  Pro'nun iki küçük yardımcısını, haber ve ESM önbellek hizmetlerini
  başlatır). Güncellemenin paket etkinliği denetimi günlük denetimin ilk
  adımlarını tanımıyordu, yalnız birkaç saniye sonra çalıştırdığı
  `apt-get`'i tanıyordu. Güncelleme bir paket işini bir dakikaya kadar
  bekler, ama bu bekleme aynı soruyu soran ve hiç beklemeyen başka bir
  denetimden sonra geliyordu. Bu yüzden güncelleme hiçbir şeyi
  değiştirmeden hemen duruyor, birkaç dakika sonraki ikinci basış
  çalışıyordu.
- **Bin denetimde bir kadar.** Bir program, paket denetimi tam adını okurken
  sona erdiğinde denetim cevap vermek yerine reddediyordu ("no such
  process") ve güncelleme bu yüzden hemen durabiliyordu. Hiçbir şeyi
  değiştirmeden, güvenle duruyordu.

### Ne değişiyor

- Güncellemenin bekleyen denetimi, hiç beklemeyen her denetimden önce gelir
  ve iletisi neyi beklediğini söyler: sunucunun paket yöneticisi ya da
  panelin başka bir sunucu işlemi.
- Günlük paket denetimi ilk adımından tanınır (`apt.systemd.daily` ve
  `apt-helper`); güncelleme işin ortasında karşılaşmak yerine bütün işi
  bekler.
- Paketler gerçekten kurulurken ya da paket yöneticisi kilidini tutarken
  güncelleme yine hiç ilerlemez; otomatik güvenlik güncellemeleri de
  dahil. Güncelleme bir şey değiştirmeden önceki sonraki her denetim paket
  etkinliğini yine reddeder; yalnız ilki bekler.
- Paket denetimi okurken sona eren bir program gitmiş sayılır, ret nedeni
  sayılmaz.
- Ubuntu Pro'nun haber ve ESM önbellek hizmetleri paket etkinliği sayılmaz:
  sistemin paket kilitlerinden hiçbirini almazlar.

### Ne görürsünüz

Güncelleme penceresinde **Kurmadan önce sunucuyu denetleme** adımının
altındaki not artık paket yöneticisini anar:

- TR: "Başka bir sunucu değişikliği ya da sunucunun paket yöneticisi
  (örneğin otomatik güncellemeler) hâlâ çalışıyorsa güncelleme onu bekler
  (en çok bir dakika), sonra kendiliğinden sürer ya da hiçbir şeyi
  değiştirmeden durur. Bir şey yapmanız gerekmiyor."
- EN: "If another server change or the server's package manager (for
  example automatic updates) is still working, the update waits for it (up
  to a minute), then continues by itself or stops without changing
  anything. Nothing needs to be done."

Güncellemenin kendi günlüğü (sunucudaki işçi günlüğü), paket yöneticisini
ne zaman beklediğini ve ne zaman sürdüğünü söyler:

- "==> Sunucunun paket yöneticisi çalışıyor (örneğin otomatik
  güncellemeler); güncelleme devam etmeden önce en çok 60 sn bekleniyor
  (bir şey yapmanız gerekmiyor)" / "==> The server's package manager is
  working (for example automatic updates); waiting up to 60 s for it
  before the update continues (nothing needs to be done)"
- "==> Paket yöneticisi işini yaklaşık N sn sonra bitirdi; güncelleme
  sürüyor" / "==> The package manager finished after about N s; the update
  continues"

Paket yöneticisi bir dakikanın sonunda hâlâ çalışıyorsa güncelleme önceki
sözlerle durur ("sunucunun paket yöneticisi meşguldü, örneğin otomatik
güvenlik güncellemeleri çalışıyordu. Hiçbir şey değiştirilmedi."), sürüm
değişmez; o iş bitince güncellemeyi yeniden başlatırsınız.

## Bu sürümde ayrıca

- **Disk kullanımı işçisi tek saat kullanır.** Ölçümlerini bir saatle
  damgalıyor, neyin sırası geldiğine başka bir saatle karar veriyor ve süre
  bütçesini de bu ikinci saate göre ölçüyordu. Bu yüzden CelikPanel'in kendi
  testlerinden beşi 15 Kasım 2026 12:00'den itibaren her makinede başarısız
  olmaya başladı (PR #328). Testleri artık takvim tarihine bağlı değil.
  Sunucuda hiçbir şey değişmez: orada iki saat aynıdır.

## Bilinen sınırlar

- **Son saniyelerde başlayan bir paket işi.** Günlük paket denetimi (ya da
  herhangi bir paket işi) bekleme ile güncellemenin son denetimi arasındaki
  birkaç saniyede başlarsa güncelleme yine beklemeden, değişiklik yapmadan
  durur. O iş bitince **Güncelle**'ye yeniden basın.
- **Pencerede canlı bir "… bekleniyor" satırı yok.** Pencere, denetim adımı
  boyunca yukarıdaki duran notu gösterir; güncellemenin tam olarak ne zaman
  ve neyi beklediği yalnız güncellemenin günlüğündedir. Canlı bir satır için
  güncellemeden panele yeni bir kanal gerekir.
- **Ubuntu hakkında belirlenemeyenler.** Zincir (bir yeniden yükleme
  `apt-daily`'yi başlatır, onun `apt-get update`'i Ubuntu Pro'nun haber ve
  ESM önbellek hizmetlerini başlatır) systemd'nin, apt'nin ve Ubuntu Pro'nun
  kaynaklarından belirlendi, bir sunucuda ölçülmedi. alpha.136
  laboratuvarındaki ilk basışı güncellemenin hangi denetiminin durdurduğu
  bilinmiyor (duruş yalnız "paket yöneticisi meşgul" der); o işi
  `apt-daily`'nin mi yoksa laboratuvarın saat sıçramasının mı başlattığı da
  bilinmiyor. Günlük denetimin programlarının Ubuntu 24.04'te taşıdığı
  adlar bir sunucuda okunmadı.

## Laboratuvarda denendi

*Sürüm adayının laboratuvar denetiminden sonra doldurulacak.*

## Kanıt

Şema değişikliği yok: şema 079, alpha.136'daki gibi. Kalıcı kayıt
değişmez. Değişiklik `cmd/agent/service_mutation_lock_linux.go` (paket
taraması), `update.sh` (beklemenin sırası ve sözleri),
`cmd/panel/disk_usage.go` (tek saat, PR #328) ve güncelleme penceresinin
notundadır (`web/src/i18n`). Bileşen testleri: sahte `/proc` ile paket
taraması (okuma sırasında sona eren süreç, günlük denetimin ilk adımları,
Ubuntu Pro'nun okuyucularının sayılmaması, başka okuma hatalarının yine
reddetmesi); güncellemenin ilk denetiminin kabuk sözleşme testi (paket işi
kendi satırlarıyla en çok 60 sn beklenir, bekleme BIND denetiminden önce
gelir); saat bugünden bir yıl önce ve bir yıl sonrayken disk kullanımı
testleri. Tam Go test paketleri alpha.136 ile karşılaştırıldı: yeni hata
yok. Henüz gerçek sistem güncelleme denemesi yok. **Hiçbir P0 işi
kapanmadı.**
