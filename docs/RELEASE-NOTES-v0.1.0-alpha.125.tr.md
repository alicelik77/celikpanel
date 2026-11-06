# v0.1.0-alpha.125

[English](RELEASE-NOTES-v0.1.0-alpha.125.md)

Parolasız posta artık kapalı: siteler postayı yalnız bir posta kutusunun
kullanıcı adı ve parolasıyla gönderir. Panel daha hızlı, güncelleme son
veritabanı denetiminde bir zamanlama çakışması yüzünden artık başarısız
olmuyor; alpha.124 laboratuvarında bulunan üç sorun ve bu sürümün
laboratuvarında bulunan iki güncelleme hatası düzeltildi (biri Debian 12'de
her güncellemeyi durduruyordu). **Kurulu
panelleri yalnız sahibi, panelin kendi güncelleme ekranından günceller.**

## Parolasız posta kapatıldı

**Ne kapandı.** Bu güncellemeden sonra panel ilk kez başladığında sunucu,
parolasız posta göndermenin iki yolunu bütün siteler, PHP sürümleri ve
betikler için birden kapatır:

- **PHP `mail()` ve `sendmail` komutu** (site cron görevleri ve betikler de
  bunu kullanır). Yalnız `root` (sistem bildirimleri: cron, otomatik
  güncellemeler, Fail2ban) ve `vmail` (posta kutusu otomatik yanıtları) bunu
  kullanmaya devam edebilir. Bir sitede PHP `mail()` artık false döner ve
  posta günlüğü `User <site kullanıcısı> is not allowed to submit mail` yazar.
- **Sunucunun kendisine parolasız SMTP** (127.0.0.1 üzerinde 25. bağlantı
  noktası). Bu sunucudaki posta kutularına gelen posta yine teslim edilir;
  başka sunuculara giden posta için bir posta kutusunun kullanıcı adı ve
  parolası gerekir ("Relay access denied").

Bu, `/etc/postfix/main.cf` içindeki iki sıradan Postfix ayarıyla yapılır
(`authorized_submit_users = root, vmail` ve `permit_mynetworks` içermeyen
`smtpd_relay_restrictions`); bu yüzden CelikPanel olmadan da çalışmaya devam
eder. Postfix denetlenir ve yeniden yüklenir; değişikliği kabul etmezse önceki
ayarlar geri yazılır, posta eskisi gibi çalışır, Postfix sayfası bunu söyler ve
CelikPanel bir sonraki panel başlangıcında yeniden dener. Bu iki ayardan birini
kendiniz ayarladıysanız CelikPanel sizin değerinizi korur ve Postfix sayfası
onu CelikPanel'in değerinin yanında gösterir: karar sizindir.

**Site cron görevleri.** Bir sitenin cron çıktısı artık postayla
gönderilemez (bunun için kapatılan `sendmail` komutu kullanılıyordu). Bu
yüzden site crontab'larının başına `MAILTO=""` eklenir: var olan crontab'lara
bir kez, panel bir crontab yazdığında da her seferinde. Kendi `MAILTO`'sunu
ayarlayan crontab olduğu gibi korunur ve Postfix sayfasında adıyla gösterilir;
root'un crontab'ına hiç dokunulmaz. Çıktısı önemli olan bir görev çıktısını
bir dosyaya yazar.

**Panelin kendi postası** (parola sıfırlama, davetler, bildirimler) Agent
tarafından Postfix'e verilir ve çalışmaya devam eder.

**Bir site hâlâ parolasız göndermek isterse ne yapılır.** Postfix sayfası,
alpha.124'teki gibi, son 30 günde parolasız posta gönderen ya da göndermeye
çalışan her siteyi (Postfix'in kendi günlüğünden) listeler. Listedeki her site
için:

1. **Gönderim posta kutusu oluştur**'a basın. `noreply@<alan adı>` oluşturulur
   ve sunucu, bağlantı noktası (STARTTLS ile 587 ya da SSL/TLS ile 465),
   kullanıcı adı, gönderen adresi ve parola bir kez gösterilir.
2. Bunları sitenin posta ayarlarına girin (örneğin SMTP eklentisine). Site bu
   adresten göndermelidir.

Bu paneldeki sitelerden olmayan gönderenler ayrıca listelenir; sunucu sahibi
onları parolalı bir posta kutusuna taşır. İki deneme sunucusunda da (Boston ve
Frankfurt) son 30 günde parolasız posta gönderen site yoktu; orada bir şey
değiştirmek gerekmiyor.

## Güncellemeler

- Güncelleme, son veritabanı denetiminde (yeni panel başladıktan sonra) yeni
  panel aynı anda veritabanına yazdığında başarısız olabiliyordu; sürüm
  kurtarma güncellemeyi yine de tamamlıyordu ama ekran nedeni olmayan bir hata
  gösteriyordu. Bu denetim artık yalnız okuma sırasında veritabanına
  yazıldıysa yeniden okur; başka her sorunda eskisi gibi hemen durur.
  Reddedilen denetim artık nedenini gösterir. Geri almada da aynı düzeltme var.
- Otomatik geri almadan hemen sonra **Güncelle**'ye basmak bir kez başarısız
  olabiliyordu (yeniden basınca çalışıyordu): panelin açılıştaki posta işi
  sunucuda hâlâ değişiklik yapıyordu. Güncellemenin ilk sunucu denetimi artık
  bitmek üzere olan bir değişikliği bir dakikaya kadar bekler; adım beklediğini
  ve kimsenin bir şey yapması gerekmediğini söyler. Panelin açılıştaki posta
  işi de kabul ettiği bir güncelleme sırada ya da sürerken bekler.
- Agent başladıktan sonra sürüm kurtarmanın tamamladığı kendini güncelleme
  denemesinin "başarısız" işareti de artık temizlenir; `systemctl --failed`
  altında kalmaz.
- Sunucunun kendi paket işi sürerken (örneğin Debian'ın gece yarısından hemen
  sonraki paket görevleri) basılan güncelleme "independent recovery runtime
  preparation is unconfirmed" iletisiyle hemen başarısız olabiliyordu (hiçbir
  şey değişmemişti; sonra yeniden basınca çalışıyordu). Yeni kurtarma
  programının hazırlığı artık güncellemenin diğer salt-okur denetimleri gibi o
  işi bir dakikaya kadar bekler.
- Debian 12'de (ve aynı eski systemd'ye sahip Ubuntu 22.04'te) her güncelleme,
  panel durdurulduktan sonra Certbot yenileme zamanlayıcılarını okurken
  duruyordu; sürüm kurtarma da geri alamıyordu ve panel kapalı kalıyordu. Bu
  sistemlerde olmayan zamanlayıcı artık doğru okunuyor ve bu adımdaki bir ret
  artık kurtarmanın kabul etmediği bir kayıt bırakamıyor. Hata alpha.92'den
  beri vardı; Debian 13 ve Ubuntu 24.04 hiç etkilenmedi.

## Daha hızlı panel

- Bileşenler sayfası, barındırma olanakları ve sunucu kurulumu sunucunun
  paketlerini ve hizmetlerini düzinelerce komut yerine birkaç toplu komutla
  okur ve sunucuda bir şey değişmediği sürece sonucu kısa süre saklar (panel
  dışında yapılan bir değişiklik hemen görülür). Laboratuvarda Bileşenler
  denetimi yaklaşık 1,1 saniyeden yaklaşık 0,15 saniyeye indi.
- Bileşenler sayfası, denetim arka planda sürerken kullanılabilir kalır. Bir
  bileşeni başlatmak ya da durdurmak yeni durumu hemen gösterir.
- Panelin dosyaları ve büyük yanıtları tarayıcıya sıkıştırılmış (gzip)
  gönderilir. Parola, anahtar ya da tek kullanımlık kod taşıyabilecek yanıtlar
  hiç sıkıştırılmaz. Panel her dosyayı ilk istendiğinde kendisi sıkıştırır;
  böylece sürüm paketi her derleme makinesinde aynı kalır.
- Kendini yenileyen sayfalar (Ana sayfa, Bileşenler, Güvenlik, İzleme, alan
  adı günlüğü) tarayıcı sekmesi gizliyken durur ve sekme yeniden açılınca bir
  kez okur.

## Daha açık retler

- Zaten var olan bir posta kutusunu oluşturmak artık iç hata yerine bunu,
  hiçbir şeyin değiştirilmediğini ve ne yapılacağını söyler (var olanı
  kullanın, listeden yeni parola belirleyin ya da başka bir ad seçin).
- PHP çalıştırmayan bir sitenin (örneğin kurulumun oluşturduğu statik ana
  site) PHP sürümünü ya da PHP havuz ayarlarını değiştirmek artık sitenin
  PHP'si olmadığını, hiçbir şeyin değiştirilmediğini söyler ve barındırma
  tipinin değiştirildiği Barındırma sekmesine götürür. Önceden iç hataydı.

## Laboratuvar sonuçları

6 Kasım 2026 laboratuvar turu, yeni kurulmuş sanal sunucularla (PowerDNS ve
postalı Debian 13 ana sunucu, Ubuntu 24.04 yedek DNS sunucusu, bir Debian 12
sunucu ve ikinci bir posta sunucusu); her biri **Güncelle** düğmesiyle
güncellendi:

- **124 → 125 güncellemesi:** Ubuntu yedek sunucu ilk basışta güncellendi.
  Debian ana sunucu, Debian'ın gece yarısı paket görevi sürdüğü için bir kez
  başarısız oldu (yukarıdaki hata); düzeltmeden sonra, o anda bilerek
  başlatılan bir paket göreviyle bile ilk basışta güncellendi. Eski başarısız
  işaretleri temizlendi; `systemctl --failed` boştu.
- **Geri almadan hemen sonra güncelleme:** geri almadan bir saniye sonra
  basılan güncelleme açılıştaki posta işini bekledi ve başarılı oldu; ikinci
  bir işlem başlamadı. Debian 12'de yukarıdaki Certbot zamanlayıcı hatası
  bulundu; düzeltmeden sonra Debian 12 ilk basışta güncellendi.
- **Daha hızlı panel:** laboratuvar sunucularında Bileşenler denetimi
  1,8–2,2 sn'den 0,6–0,9 sn'ye, sunucu kurulumunun bileşen yanıtı 0,8–1,3 sn'den
  0,1 sn'nin altına indi; sonuçlar aynı. Panel dışında `apt` ile kurulan paket
  1,4 sn sonra göründü. Dosyalar ve yanıtlar sıkıştırıldı; gizli bilgi
  taşıyabilen yanıtlar sıkıştırılmadı.
- **Parolasız posta:** her madde denetlendi; ikinci bir sunucuya posta kutusu
  parolasıyla teslim de dahil.
- **Daha açık retler:** iki ret de anlatıldığı gibi, İngilizce ve Türkçe
  döndü.

## Bilinen sınırlar

- Site cron çıktısı artık postayla gönderilmiyor ve panel onu başka bir yerde
  de saklamıyor.
- Yukarıdaki Certbot zamanlayıcı hatası yüzünden paneli zaten kapalı kalmış
  bir Debian 12 ya da Ubuntu 22.04 sunucusunu bu sürüm onaramaz, çünkü ona
  ulaşamaz; sahibinin kurtarma yolu gerekir. Ubuntu 22.04'te güncellemenin
  kendisi laboratuvarda denenmedi.
- Güncellemenin son veritabanı denetimi hâlâ çalışan veritabanının bir
  kopyasını alıyor. Bunun yerine çalışan panele sormak, alpha.126 için
  planlanan yapısal bir sadeleştirmedir.
- İzlemenin ilk aşaması ve hiçbir şey olmazken kurtarma bekçisinin hızlı yolu
  alpha.126 için planlandı.
- Node.js siteleri için yalıtım alpha.127'ye kaldı.
- Önceki sınırlar sürüyor: yedek sunucusu olan PowerDNS ana sunucusunda DNSSEC
  imzalama yok; bir işlemin durumunu kaydeden her yer henüz meşgul veritabanını
  beklemiyor; kurulum makinenin kendi adı için AAAA (IPv6) kaydı eklemiyor.

## Kanıt

Şema değişmedi, geçiş yok. Sürüm sırası 124 → 125.
Go testleri (bütün modül) ve web testleri geçiyor, web paketleri sınırları
içinde. Laboratuvar kanıtı: laboratuvar dalında
`deploy/e2e/dns-pair-acceptance/evidence/a125`.
