# v0.1.0-alpha.137

[English](RELEASE-NOTES-v0.1.0-alpha.137.md)

Tek konulu küçük bir sürüm: sunucunun paket yöneticisi çalışırken yapılan
güncelleme. Ubuntu'da panel güncellemesinin ilk basışı, hiçbir şey gerçekte
paket değiştirmezken "sunucunun paket yöneticisi meşguldü" diyerek
durabiliyordu; basıştan hemen sonra gerçek bir paket kurulumu başladığında
ise güncelleme, güncelleme sayfasını kilitli ve yeniden başlatma yolu
olmadan bırakan bir biçimde duruyordu. Artık güncelleme bir paket işini her
denetiminde en çok bir dakika bekler; iş hâlâ sürüyorsa güncelleme hiçbir
şeyi değiştirmeden durur ve hemen yeniden başlatılabilir.
**Kurulu panelleri yalnız sahip, panelin kendi güncelleme ekranından
günceller.**

## alpha.136'dan güncelleme

1. Her zamanki gibi panelin kendi güncelleme ekranından güncelleyin. Bu
   sürüm panelin veritabanı yapısını değiştirmez (alpha.136'daki gibi
   kalır); sonradan alpha.136'ya dönmek veritabanı geri yüklemesi
   gerektirmez.
2. Düzeltmeler, alpha.137'yi kuran güncellemenin kendisindedir; bu
   güncellemede de işe yarar. Güncelleme yine "sunucunun paket yöneticisi
   meşguldü" diyerek durursa sunucu gerçekten daha uzun süre paket kuruyor ya
   da denetliyordu: o iş bitince **Güncelle**'ye yeniden basın.
3. **Şu anki sürümünüzün güncelleme sayfası takılı kaldıysa** (güncellemenin
   başarısız olduğunu ve sunucu sahibinin güncelleme günlüğünü incelemesi
   gerektiğini söylüyor, sayfa kilitli kalıyor ve güncelleme düğmesi yok),
   güncellemeden önce aşağıdaki "Önceki bir güncelleme sayfayı kilitli
   bıraktıysa" bölümüne bakın.
4. Güncellemeden sonra basılacak ya da ayarlanacak bir şey yok.

## Sizin için ne değişiyor

### Sorun neydi

- **Ubuntu'da ilk basış.** Güncelleme hazırlanırken sunucunun hizmet
  tanımlarını (systemd) birkaç kez yeniden yükler. Ubuntu'da ve Debian'da
  böyle bir yeniden yükleme sistemin günlük paket denetimini hemen
  başlatabilir (`apt-daily`: paket listelerini yeniler ve Ubuntu'da Ubuntu
  Pro'nun iki küçük yardımcısını, haber ve ESM önbellek hizmetlerini
  başlatır). Güncellemenin paket etkinliği denetimi günlük denetimin ilk
  adımlarını tanımıyordu, yalnız birkaç saniye sonra çalıştırdığı
  `apt-get`'i tanıyordu; güncellemenin bir dakikalık beklemesi de aynı
  soruyu soran ve hiç beklemeyen başka bir denetimden sonra geliyordu. Bu
  yüzden güncelleme hiçbir şeyi değiştirmeden hemen duruyor, birkaç dakika
  sonraki ikinci basış çalışıyordu.
- **Basıştan hemen sonra başlayan bir paket kurulumu** (alpha.137
  laboratuvarında, Debian 13'te bulundu). Güncelleme bir şey durdurmadan
  önce sunucunun kurtarma aracı kendi yeni sürümünü hazırlar ve sunucunun
  boşta olduğunu denetler. Bu denetim paket yöneticisini bir dakika bekledi,
  sonra güncelleme her zamanki "paket yöneticisi meşguldü" yerine
  "independent recovery runtime preparation is unconfirmed" diyerek durdu.
  Hiçbir şey değiştirilmemişti; ama güncelleme sayfası bunu sunucunun
  kurtarmasının hâlâ ele alacağı bir hata sandı: kilitli kaldı, güncelleme
  düğmesi göstermedi ve girişimin hizmeti başarısız olarak işaretli kaldı.
- **Bin denetimde bir kadar.** Bir program, paket denetimi tam adını okurken
  sona erdiğinde denetim cevap vermek yerine reddediyordu ("no such
  process") ve güncelleme hemen durabiliyordu. Hiçbir şeyi değiştirmeden,
  güvenle duruyordu.

### Ne değişiyor

- Güncelleme artık bir şey hazırlamadan önce bekler: paket yöneticisi ya da
  panelin başka bir sunucu işlemi çalışıyorsa onu en çok bir dakika bekler,
  sonra sürer. Hizmet tanımlarını yeniden yükledikten sonra, diğer
  denetimlerinden önce bir kez daha bekler.
- Kurtarma aracının kendi denetimi, kendi beklemesinden sonra ve hiçbir şeyi
  seçmeden önce sunucuyu hâlâ meşgul bulursa güncelleme artık her zamanki
  "paket yöneticisi meşguldü" (ya da "başka bir sunucu işlemi hâlâ
  sürüyordu") ile durur: hiçbir şey değişmez, güncelleme hemen yeniden
  başlatılabilir ve girişimin hizmeti başarısız olarak işaretli kalmaz. O
  hazırlığın başka her sorunu mevcut, dikkatli bildirimini korur.
- Günlük paket denetimi ilk adımından tanınır (`apt.systemd.daily` ve
  `apt-helper`); güncelleme işin ortasında karşılaşmak yerine bütün işi
  bekler.
- Paketler gerçekten kurulurken ya da paket yöneticisi kilidini tutarken
  güncelleme yine hiç ilerlemez; otomatik güvenlik güncellemeleri de dahil.
  Güncelleme bir şey değiştirmeden önceki sonraki her denetim paket
  etkinliğini yine reddeder; yalnız ilkler bekler.
- Paket denetimi okurken sona eren bir program gitmiş sayılır, ret nedeni
  sayılmaz.
- Ubuntu Pro'nun haber ve ESM önbellek hizmetleri paket etkinliği sayılmaz:
  sistemin paket kilitlerinden hiçbirini almazlar.

### Ne görürsünüz

Güncelleme penceresinde **Kurmadan önce sunucuyu denetleme** adımının
altındaki not artık paket yöneticisini anar:

- TR: "Başka bir sunucu değişikliği ya da sunucunun paket yöneticisi
  (örneğin otomatik güncellemeler) hâlâ çalışıyorsa güncelleme onu bekler
  (her denetimde en çok bir dakika), sonra kendiliğinden sürer ya da hiçbir
  şeyi değiştirmeden durur. Bir şey yapmanız gerekmiyor."
- EN: "If another server change or the server's package manager (for
  example automatic updates) is still working, the update waits for it (up
  to a minute at each check), then continues by itself or stops without
  changing anything. Nothing needs to be done."

Bu not denetim adımı boyunca gösterilir. Pencere, güncellemenin beklemeye
başladığı ya da beklemeyi bitirdiği anı göstermez: bugün bunu ne pencere ne
de sunucunun günlükleri söyler (bkz. "Bilinen sınırlar").

Paket yöneticisi beklemeden sonra hâlâ çalışıyorsa güncelleme önceki
sözlerle durur ("sunucunun paket yöneticisi meşguldü, örneğin otomatik
güvenlik güncellemeleri çalışıyordu. Hiçbir şey değiştirilmedi."), sürüm
değişmez, güncelleme düğmesi yeniden görünür; o iş bitince güncellemeyi
yeniden başlatırsınız.

Kurtarma aracının hazırlığı başka bir nedenle durursa pencere bunu artık
kapatılabilir bir duruş olarak söyler: "sunucunun kurtarma aracı yeni sürüm
için kendi hazırlığını doğrulayamadı; bu yüzden güncelleme hiçbir hizmeti
durdurmadan durdu. Hiçbir şey değiştirilmedi.", yanında yalnız okuyan tek
bir komutla: `sudo /usr/libexec/celikpanel/recovery runtime-status`; komut o
hazırlıktan geriye bir şey kalıp kalmadığını ve ne yapılacağını söyler.
Önceden bu sayfa kilitli kalıyordu.

## Önceki bir güncelleme sayfayı kilitli bıraktıysa

Bu, alpha.136 ve öncesinde, güncellemeye basıldıktan hemen sonra bir paket
kurulumu başladığında olabilir. Kurulu sürüm ve veriler değiştirilmedi.
Kilidi sunucu değil, güncellemeyi izleyen tarayıcı tutar:

1. Paneli başka bir tarayıcıda ya da gizli pencerede açıp oturum açın.
   Güncelleme kartı önceki girişimi başarısız olarak gösterir ve
   güncellemeyi yeniden sunar. (Aynı tarayıcıda panel sitesinin saklanan
   verilerini silmek de aynı işi görür.)
2. Önce emin olmak isterseniz sunucuda
   `sudo /usr/libexec/celikpanel/recovery runtime-status` komutunu
   çalıştırın; komut yalnız okur. "Kaydedilmiş kurtarma kiti geçişi yok"
   derse geriye bir şey kalmamıştır: panelden güncelleyin. Yeni kitin hazır
   olduğunu söylerse panelden yeniden güncellemek o denetimi bitirir.
   Kurulmuş bir başlatıcıdan ya da seçilmiş bir kitten söz ederse önce,
   söylediği gibi, `sudo /usr/libexec/celikpanel/recovery recover`
   komutunu çalıştırın.
3. İsteğe bağlı: `sudo systemctl reset-failed 'celikpanel-self-update-*'`
   o girişimin hizmetindeki "başarısız" işaretini kaldırır. Başka hiçbir
   şeyi değiştirmez; alpha.137 bunu böyle girişimler için kendisi yapar.

## Bilinen sınırlar

- **Son saniyelerde başlayan bir paket işi.** Bir paket işi güncellemenin
  beklemeleri ile son denetimleri arasındaki birkaç saniyede (ya da kurtarma
  aracı yeni sürümüne geçerken) başlarsa güncelleme yine beklemeden durur;
  hiçbir şeyi değiştirmeden durur ve yeniden başlatılabilir.
- **Canlı bir "… bekleniyor" satırı yok.** Güncellemenin kendi iletileri,
  paket yöneticisini beklerken yazdığı satırlar da dahil, sahibin
  okuyabileceği hiçbir yerde tutulmaz: panel bir hatanın yalnız son satırını
  saklar. Canlı bir bekleme satırı için güncellemenin panele ne yaptığını
  söylemesinin yeni bir yolu gerekir; bu sürümde yok; pencere yukarıdaki
  duran notu korur.
- **Ubuntu hakkında belirlenemeyenler.** Zincir (bir yeniden yükleme
  `apt-daily`'yi başlatır, onun `apt-get update`'i Ubuntu Pro'nun haber ve
  ESM önbellek hizmetlerini başlatır) systemd'nin, apt'nin ve Ubuntu Pro'nun
  kaynaklarından belirlendi (laboratuvarın gördükleri "Laboratuvarda
  denendi" bölümündedir). alpha.136 laboratuvarındaki ilk basışı
  güncellemenin hangi denetiminin durdurduğu bilinmiyor.
- **Önceki bir sürümde zaten kilitli kalmış sayfalar** yukarıda anlatıldığı
  gibi başka bir tarayıcıdan açılır; sunucu bir tarayıcının kilidini
  kaldıramaz.

## Bu sürümde ayrıca

- **Disk kullanımı işçisi tek saat kullanır.** Ölçümlerini bir saatle
  damgalıyor, neyin sırası geldiğine başka bir saatle karar veriyor ve süre
  bütçesini de bu ikinci saate göre ölçüyordu. Bu yüzden CelikPanel'in kendi
  testlerinden beşi 15 Kasım 2026 12:00'den itibaren her makinede başarısız
  olmaya başladı (PR #328). Testleri artık takvim tarihine bağlı değil.
  Sunucuda hiçbir şey değişmez: orada iki saat aynıdır.

## Laboratuvarda denendi

*Sürüm adayının laboratuvar denetiminden sonra doldurulacak.*

## Kanıt

Şema değişikliği yok: şema 079, alpha.136'daki gibi. Kalıcı kayıt
değişmez. Değişiklikler `update.sh`'ta (nerede ve nasıl beklediği, çıkış
işleyicisi kurulmadan önceki tipli duruş), kurtarma aracında
(`internal/recoveryruntime`, `cmd/recovery`: herhangi bir geçişten önce
meşgul sunucu adıyla söylenir), Agent'ın paket taramasında ve hiçbir şeyi
değiştirmemiş girişimin başarısız işaretini temizleme kuralında
(`cmd/agent`), güncelleme penceresinin metinlerinde ve doğrulanamayan
hazırlık duruşunu okumasında (`web/src`) ve `cmd/panel/disk_usage.go`'dadır
(tek saat, PR #328). Bileşen testleri: sahte `/proc` ile paket taraması;
kurtarma aracının denetleyicisi ve hazırlığı, niyetten önce ve sonra meşgul
sunucuyla; güncellemenin ilk denetimleri (paket işi beklenir, süren iş tipli
satırıyla değişiklik yapmadan durur; hazırlıktan ve BIND denetiminden
önce); kurtarma ortamı kabuk sözleşmesi (meşgul hazırlık tipli duruştur,
başka her hata bildirimini korur); laboratuvarın tam kaydıyla Agent'ın
temizleme kuralı; laboratuvarın tam kaydıyla iki dilde güncelleme penceresi.
Tam Go test paketleri alpha.136 ile karşılaştırıldı: yeni hata yok.
**Hiçbir P0 işi kapanmadı.**
