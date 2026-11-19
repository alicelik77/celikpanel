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
2. Düzeltmelerin çoğu alpha.137'yi kuran güncellemenin kendisindedir; bu
   güncellemede de işe yarar: nerede beklediği ve meşgul sunucunun yeniden
   başlatılabilen bir duruşla bitmesi. Bir kısmı sonraki güncellemeden
   itibaren etkilidir: günlük denetimin ilk adımını tanıyan paket denetimi
   kurulu panelinkidir, bu güncelleme sırasında alpha.136'nın denetimi
   cevap verir. Güncelleme yine "sunucunun paket yöneticisi meşguldü"
   diyerek durursa sunucu gerçekten daha uzun süre paket kuruyor ya da
   denetliyordu: o iş bitince **Güncelle**'ye yeniden basın.
3. **Şu anki sürümünüzün güncelleme sayfası takılı kaldıysa** (güncellemenin
   başarısız olduğunu ve sunucu sahibinin güncelleme günlüğünü incelemesi
   gerektiğini söylüyor, sayfa kilitli kalıyor ve güncelleme düğmesi yok),
   güncellemeden önce aşağıdaki "Önceki bir güncelleme sayfayı kilitli
   bıraktıysa" bölümüne bakın (bir kısmı laboratuvarda doğrulanacak).
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
- **Sağlamlaştırma:** yalnız paket yöneticisinin adını taşıyan ve sıradan
  bir hesabın çalıştırdığı bir süreç artık güncellemeyi bekletmez. Bir
  barındırma sunucusunda her hesap `apt` ya da `dpkg` adlı bir program
  başlatabilir; artık yalnız root olarak çalışan süreç (sistemin paketlerini
  değiştirebilen tek kullanıcı) sayılır. Sahibi okunamayan süreç yine
  sayılır.
- Böyle bir duruştan sonraki yeniden deneme artık kurtarma kitinin bir
  kopyasını geride bırakmaz.

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

Kurtarma aracının hazırlığı başka bir nedenle durursa pencere artık kilitli
kalmaz; bunu kapatılabilir bir duruş olarak gösterir ve şunu söyler:

> "Neden: sunucunun kurtarma aracı yeni sürüm için kendi hazırlığını
> doğrulayamadı; bu yüzden güncelleme hiçbir hizmeti durdurmadan durdu.
> Kurulu sürüm ve verileri değiştirilmedi; kurtarma aracının yeni sürüm
> için kendi hazırlığı yarım kalmış olabilir. Sunucu sahibi aşağıdaki
> komutu sunucuda bir kez çalıştırır ve yanıtının söylediğini yapar. Hiçbir
> işlem kendiliğinden başlamaz."
>
> `sudo /usr/libexec/celikpanel/recovery runtime-status --lang tr`
>
> "Bu komut yalnız okur; hiçbir şeyi değiştirmez."
>
> "Kaydedilmiş kurtarma kiti geçişi olmadığını, yeni kitin hazır olduğunu
> ya da geçişin tamamlandığının doğrulandığını söylüyorsa v0.1.0-alpha.137
> güncellemesini yeniden başlatın; güncelleme önce aynı denetimi yineler.
> Yeni başlatıcının kurulduğunu ya da yeni kitin seçildiğini söylüyorsa
> söylediği gibi sudo /usr/libexec/celikpanel/recovery recover komutunu
> çalıştırın, sonra güncellemeyi yeniden başlatın. Geçiş kanıtlarının
> doğrulanamadığını söylüyorsa kurtarma dosyalarını ve bu güncellemenin
> günlüğünü değiştirmeyin ve güncellemeyi başlatmayın; önce yanıtın sonunda
> parantez içinde adı geçen kanıt sorunu giderilir. O zamana dek sunucu
> eskisi gibi şu anki sürümünü çalıştırır."

İngilizce sayfada komut `--lang en` ile aynıdır ve araç İngilizce yanıt
verir. Pencere kapatıldıktan sonra güncelleme kartı da o girişim için aynı
şeyi söyler ("kurulu sürümü ya da verilerini değiştirmeden durdu", aynı
komutla); "tamamlanmadı" demez.

## Önceki bir güncelleme sayfayı kilitli bıraktıysa

Bu, alpha.136 ve öncesinde, güncellemeye basıldıktan hemen sonra bir paket
kurulumu başladığında olabilir. Kurulu sürüm ve verileri değiştirilmedi;
kurtarma aracının kendi hazırlığı yarım kalmış olabilir (2. adım geriye ne
kaldığını söyler).

<!-- LAB-CONFIRM a137-second-browser: replace after the alpha.137 lab run -->
**Laboratuvarda doğrulanacak.** Panelin kodu bu kilidi sunucuda değil,
güncellemeyi izleyen tarayıcıda (işlemin orada saklanan kaydında) tutar.
Laboratuvar bunu doğrularsa:

1. Paneli başka bir tarayıcıda ya da gizli pencerede açıp oturum açın.
   Güncelleme kartı önceki girişimi gösterir ve güncellemeyi yeniden sunar.
   (Aynı tarayıcıda panel sitesinin saklanan verilerini silmek de aynı işi
   görmelidir.)
<!-- /LAB-CONFIRM a137-second-browser -->

2. Güncellemeden önce sunucuda
   `sudo /usr/libexec/celikpanel/recovery runtime-status --lang tr`
   komutunu bir kez çalıştırın ve yukarıda pencere için anlatıldığı gibi
   yanıtının söylediğini yapın: kaydedilmiş geçiş yok, yeni kit hazır ya da
   geçişin tamamlandığı doğrulandı: panelden güncelleyin; yeni başlatıcı
   kuruldu ya da yeni kit seçildi: önce
   `sudo /usr/libexec/celikpanel/recovery recover` komutunu çalıştırın;
   kanıtlar doğrulanamadı: dosyaları ve güncelleme günlüğünü değiştirmeyin,
   henüz güncellemeyin.
3. Başka bir şey gerekmez. alpha.137'ye güncelleme başarılı olunca panel o
   önceki girişimin hizmetindeki "başarısız" işaretini kendisi kaldırır.

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
- **Önceki bir sürümde zaten kilitli kalmış sayfalar.** <!-- LAB-CONFIRM a137-second-browser -->
  Yukarıda anlatıldığı gibi başka bir tarayıcıdan açıldıkları laboratuvarda
  doğrulanacak; sunucu bir tarayıcının kilidini kaldıramaz.
- **Önceki yeniden denemelerin bıraktığı kopyalar.** Kurtarma aracının
  hazırlığı durduktan sonra aynı sürümün önceki her yeniden denemesi,
  `/usr/libexec/celikpanel/recovery-runtimes/v1/` altında gizli bir
  `.promote-…` klasöründe kurtarma kitinin bir kopyasını bırakabiliyordu.
  alpha.137 artık bırakmaz; zaten var olanları henüz hiçbir şey silmez
  (alpha.139 için listelendi). Etkisizdirler: onları hiçbir şey seçmez ya da
  çalıştırmaz.

## Bu sürümde ayrıca

- **Disk kullanımı işçisi tek saat kullanır.** Ölçümlerini bir saatle
  damgalıyor, neyin sırası geldiğine başka bir saatle karar veriyor ve süre
  bütçesini de bu ikinci saate göre ölçüyordu. Bu yüzden CelikPanel'in kendi
  testlerinden beşi 15 Kasım 2026 12:00'den itibaren her makinede başarısız
  olmaya başladı (PR #328). Testleri artık takvim tarihine bağlı değil.
  Sunucuda hiçbir şey değişmez: orada iki saat aynıdır.
- **CelikPanel'in kendi yarış testlerinin yine payı var.** Bu testlerin bir
  grubu, hiçbir test başarısız olmadan, yavaş test makinelerinde sekiz
  dakikalık sınırına ulaşıyordu; gruplar ölçülen sürelere göre yeniden
  bölündü ve bir denetim her panel testini listeleyip her birinin tam bir
  grupta çalıştığını doğrulamazsa başarısız olur (PR #329'da ölçüldü,
  buraya birleştirildi). Sunucuda hiçbir şey değişmez.

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
temizleme kuralı; laboratuvarın tam kaydıyla iki dilde güncelleme penceresi
ve pencere kapatıldıktan sonraki güncelleme kartı; yalnız root'a ait paket
süreçlerini sayan paket taraması (sıradan bir hesabın aynı adlı süreci
sayılmaz, sahibi okunamayan sayılır); iki kez meşgul sunucuyla karşılaşan
hazırlık kit kopyası bırakmaz; tek bir sözleşme "preparation is unconfirmed"
satırını, Agent'ın ve pencerenin onu okuyan kurallarını ve kurtarma aracının
tipli bildirimlerini tek metin olarak tutar
(`deploy/test-update-preparation-stop-contract.sh`).
Tam Go test paketleri alpha.136 ile karşılaştırıldı: yeni hata yok.
**Hiçbir P0 işi kapanmadı.**
