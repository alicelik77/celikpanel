# v0.1.0-alpha.145

[English](RELEASE-NOTES-v0.1.0-alpha.145.md)

> **Yayımlandı**, 23 Kasım 2026 (UTC): sürüm commit'i `2584a566e`, GitHub ön sürümü 08:56:20Z, indirme portalı yaklaşık 09:00Z (dış denetim 27/27).

**Küçük bir sürüm.** Web postasının yerel kapısındaki bir güvenlik açığını
kapatır, web postasının posta gönderebilmesini sağlar (daha önce hiç
gönderemiyordu) ve alpha.144 laboratuvarının istediği iki küçük görünüm
düzeltmesini getirir. Başka yeni bir şey yoktur; panelin veritabanı düzeni
değişmez.
**Kurulu panelleri yalnız sahip, panelin kendi güncelleme ekranından
günceller.**

## alpha.144'ten güncelleme

1. Panelin kendi güncelleme ekranından, önceki gibi güncelleyin.
2. **Panelin veritabanı düzeni değişmez** (şema 81 kalır). alpha.144'e geri
   dönmek olağan geri almadır; hiçbir anlık görüntünün geri yüklenmesi
   gerekmez.
3. Güncellemeden sonraki ilk açılışında Agent kendiliğinden, bir kez iki şey
   yapar: web postasının yerel kapısının web sunucusu dosyasını yeni
   biçimiyle yazar ve web sunucusunu yeniden yükler ("Güvenlik: kapatılan"
   bölümüne bakın); web postasının kendi yapılandırmasını gönderebilecek
   biçimde yeniden yazar ("Web postası gönderir" bölümüne bakın). Basılacak
   bir şey yoktur. Yapamadığı şey **Şu anki sorunlar**'da adımıyla birlikte
   tek satırdır.

## Güvenlik: kapatılan

- **Web postasının yerel kapısı sunucudaki her hesaba yanıt veriyordu.**
  Panel web postasını yerel bir kapıdan denetler: sunucunun kendisindeki bir
  soket (`/run/celikpanel-webmail.sock`). Web sunucusu bu soketi sunucudaki
  her hesaba açar; bu yüzden oradaki her hesap — bir müşterinin site kodu ya
  da zamanlanmış görevi — onu oturum açmadan kullanabiliyordu. Onun üzerinden
  sunucudaki herkes web postasının herkese açık klasöründeki her dosyayı
  olduğu gibi okuyabiliyordu; Roundcube'un eklenti ve tema klasörleri de
  dahil: eklenti ayarlarının artık kopyaları (örneğin bir eklentinin
  `config.inc.php.bak` dosyası), nokta dosyaları, `composer.json`, `.dist`,
  `.inc` ve SQL dosyaları; eklenti betiklerini de doğrudan
  çalıştırabiliyordu. Ağdan **erişilemiyordu**; web postasının kendi
  yapılandırması, veritabanı ve posta kutularının parolaları bu yoldan hiçbir
  zaman erişilebilir değildi. Bu durum alpha.13'ten beri vardı (alpha.1'den
  beri aynı ağaç sunucunun iç adresi 127.0.0.1 üzerinde açıktı).
- **Kapı artık yalnız CelikPanel'in kendi denetimlerinin sorduğunu yanıtlar:**
  web postasının oturum açma sayfası (`/webmail/`, Roundcube'un giriş
  noktası, orada çalışan tek PHP), stil dosyaları ve CelikPanel'in PHP
  denetimi. Gerisi "bulunamadı" yanıtı alır. Soketin kendisi taşınmaz, kipi
  değişmez.
- **Kurulu sunucular kendiliğinden düzeltilir:** güncellemeden sonraki ilk
  açılışta yeni dosya yazılır, `nginx -t` ile denetlenir ve web sunucusu
  yeniden yüklenir. Denetim ya da yeniden yükleme başarısız olursa önceki
  dosya tam olarak geri konur, Agent açılmayı sürdürür ve **Şu anki
  sorunlar** nedenini ve adımı söyler (denetimin gösterdiğini ya da Agent
  günlüğündeki nedeni düzeltin, sonra Agent'ı yeniden başlatın: CelikPanel
  yeni dosyayı o açılışta yazar).
- **Elle değiştirdiğiniz yerel kapı dosyası korunur;** CelikPanel'in metni
  yanına `celikpanel-webmail.conf.celikpanel-pending` olarak konur (web
  sunucusu onu okumaz) ve **Şu anki sorunlar** tek satırda yerel kapının hâlâ
  Roundcube dosyalarını verdiğini, sunucu sahibinin yapacağını ve adımı
  söyler. Dosyada CelikPanel'in kuralları olunca satır kendiliğinden kalkar.

## Sizin için ne değişiyor

### Web postası gönderir

- **Web postasından gönderim önceki hiçbir sürümde çalışmadı** (posta okuma,
  ekler ve ayarlar çalışıyordu). Web postası bu sunucunun kendi gönderim
  hizmetiyle şifresiz konuşuyordu; o hizmet ise girişi yalnız şifreli
  bağlantıda kabul eder; bu yüzden her ileti "kimlik doğrulanamadı" ile
  bitiyordu.
- **Web postası artık şifrelemeyi açar** ve bir posta programı gibi posta
  kutusunun kendi adresi ve parolasıyla giriş yapar. Bağlantı sunucunun
  içinde kalır (`127.0.0.1`); bu yüzden yalnız bu iç adreste sertifikanın adı
  ve zinciri denetlenmez: gönderim hizmeti posta adının sertifikasını ya da o
  yokken kendi imzalı bir sertifikayı gösterebilir ve gönderim iki durumda da
  çalışmalıdır. Başka hiçbir adres için hiçbir şey gevşetilmez.
- Web postasından giden posta, o kutunun diğer postaları gibi göndericinin
  kendi alan adı adına imzalanır (alpha.143'ün kuralı, değişmedi). Posta
  programları için 465 ve 587 kapıları değişmedi.
- **Kurulu sunucular kendiliğinden düzeltilir:** güncellemeden sonraki ilk
  açılışta web postasının yapılandırması bir kez, kipi, sahibi ve gizli
  anahtarı korunarak yeniden yazılır.
- **Elle değiştirdiğiniz web postası yapılandırması korunur;** CelikPanel'in
  metni yanına `config.inc.php.celikpanel-pending` olarak konur (yalnız root
  okuyabilir; Roundcube onu okumaz) ve **Şu anki sorunlar'da tek satır** web
  postasının gönderemediğini, nedenini ve adımı söyler: o metni (ya da iki
  gönderim satırını) dosyanıza kopyalayın; yanına metin konamadıysa satır
  yazılacak iki satırı verir. CelikPanel kendi önceki metnini yeniden
  yazamadıysa (salt okunur dosya sistemi, değiştirilemez işareti, dolu disk)
  satır bu nedeni giderip Agent'ı yeniden başlatmayı söyler — dosyayı elle
  düzenlemeyi değil. Dosya şifreli gönderince satır kendiliğinden kalkar.
- Web postası gönderemediği sürece Webmail kartı "kurulu ve doğrulanmış"
  demez.

### Küçük düzeltmeler

- Bir takma adın "yayımlanmayı bekliyor" durumu, bölgesinin bekletilen
  değişikliği yayımlanınca ya da bölgenin yedek sunucuyu bekleyen kendi yayını
  tamamlanınca biter — bir saat sonra değil. Beklerken bölgenin uyarısı artık
  değişikliği yeniden yapmayı istemez; takma ad kayıtlarının kendiliğinden
  yayımlanacağını söyler.
- PostgreSQL kapatma ya da geri alma penceresi sonucu söyleyen bir başlıkla
  biter (kapatıldı, kapatılamadı, bilinmiyor); sürmekte olan "… kapatılıyor"
  başlığıyla bitmez.

## Bilinen sınırlar

- **Sunucudaki her hesabın yerel kapıdan hâlâ erişebildikleri:** web
  postasının oturum açma sayfası — herkese açık adresinin gösterdiği aynı
  sayfa; yani Roundcube'un kendisi, bir girişle; istemci adresine göre
  çalışan bir deneme sınırı bunu görmez — ve PHP sürümü (CelikPanel'in PHP
  denetimi söyler). root'un Roundcube'un `skins` klasörünün içinde yaptığı bir
  bağlantı üzerinden erişilen stil dosyası verilir.
- **Sahibin o sokete istek ileten kendi web sunucusu bloğu** (örneğin web
  postasını bir sitenin yolunda tutmak için) artık yalnız oturum açma
  sayfasını ve stil dosyalarını alır; bunun yerine web postasının kendi
  adresini kullanın.
- **"Gönderemiyor" denetimi yalnız açık durumları okur.** Sahibin kendi
  sertifika seçenekleriyle elle yazdığı bir gönderim ayarı sahibin seçimi
  sayılır ve denetlenmez; `/* */` yorumunun içindeki bir `smtp_host` etkin
  okunur. Şifrelemeyi adla açıp sertifika ayarı olmayan bir yapılandırma ise
  adlandırılır.
- **Web postası gönderemezken Webmail kartı Onar düğmesini yine sunar;**
  Onar yapılandırmayı düzeltmez. Adım Şu anki sorunlar'dadır.
- **alpha.144'e geri dönülürse:** web postası göndermeyi sürdürür ve yerel
  kapı kapalı kalır; ama alpha.144 yeni metinleri tanımaz: web postasının
  yapılandırmasını ve yerel kapının dosyasını sahibin değiştirdiği dosyalar
  sayar; bu yüzden sunucu yeniden güncellenene dek web postasının adres
  satırları, web postasının onarımı ve web postasını başka bir PHP sürümüne
  taşımak elle değiştirilmiş diye reddedilir. (Bu iletinin dediği gibi yerel
  kapının dosyasını silerseniz alpha.144 önceki, açık biçimini yeniden
  yazar.)
- **Uzun bir değişiklik sunucuda bittikten sonra panelde "sürüyor"
  kalabilir.** Uzun bir değişiklik sürerken — web postası güncellemesi,
  bileşen kurulumu, posta profili kurulumu — panel birkaç saniyede bir
  Agent'la bağını yoklar. Bu bağ bir an koparsa değişiklik sunucuda yine
  biter; ama panel onu sürüyor gösterebilir ve başka sunucu değişikliklerini
  "başka bir işlem çalışıyor" diye reddedebilir (bu ileti o işlem bitince
  yeniden basmayı söyler; burada işlem kendiliğinden bitmez). Sunucuda
  hiçbir şey zarar görmez: siteler, posta ve web postası çalışmayı sürdürür,
  yapılan değişiklik kalır. Bir değişiklik beş dakikadan uzun süredir
  sürüyor görünüyorsa (bir web postası güncellemesi normalde saniyeler
  sürer) sunucuda `sudo systemctl restart celikpanel-panel` komutunu
  çalıştırın. Bu yalnız panelin web sürecini yeniden başlatır; panel
  açılırken sunucuda gerçekte ne yapıldığını okur ve sonucu kaydeder. Web
  postası güncellemesi yalnız okunur, hiçbir şey yeniden çalıştırılmaz;
  yarıda kalan bir kurulum sunucuda kaldığı yerden sürdürülüp tamamlanır.
  Sayfanın yenilenmesi ya da yeniden oturum açılması gerekebilir. Bu önceki
  sürümlerde de böyleydi: o yolun kodu bu sürümde değişmedi.
  Laboratuvarda bu sürümde, hazırlığı alışılmadık biçimde yavaş geçen bir
  Roundcube güncellemesinde bir kez oldu (sayfa yaklaşık kırk dakika
  "güncelleniyor" dedi, bir panel yeniden başlatması güncellemeyi
  tamamlandı diye kaydedene dek); aynı güncelleme alpha.144'ün koduyla
  denendiğinde olmadı. Gelecek bir sürümde düzeltilecek.
- **Bir bölgenin genel uyarısı bir yayının gerisinde kalabilir.**
  Laboratuvarda bekletilen bir DNS değişikliği yayımlandıktan yaklaşık
  yirmi saniye sonra bölgenin genel uyarısı hâlâ yeni değişikliklerin
  beklediğini ve değişikliği yeniden yapmayı söylüyordu; takma ad satırı ise
  artık beklediğini söylemiyordu. Yayın tamamlanmıştı; bir yayından hemen
  sonra bu uyarıyı görürseniz bir değişikliği yeniden yapmadan önce biraz
  sonra yeniden bakın.
- alpha.144'ün açık listesi açık kalır
  ([alpha.144 sürüm notları](RELEASE-NOTES-v0.1.0-alpha.144.tr.md)); aralarında
  tarayıcının isteğiyle çalışan diğer değişiklikler ve web sunucusunun ortak
  hesabında kalan programların trafik günlüklerini okuyabilmesi var.

## Laboratuvarda denetlenenler

Laboratuvar denetimi bitti: deneme sunucusu çiftinde tek deneme; PowerDNS,
posta ve kendi adresinde web postası olan bir Debian 13 ana DNS sunucusu ve
web postasının yerel kapısı olan ama posta hizmeti olmayan, BIND'li bir
Ubuntu 24.04 yedek DNS sunucusu (23 Kasım 2026, temizlikle birlikte UTC
05:21 ile yaklaşık 06:50 arası). İki sunucu da alpha.144'ün koduyla
başladı ve panelin kendi güncelleme sayfasından bu sürümün koduna
(`e10c2ddb6`, yalnız laboratuvara özel v0.1.0-alpha.153 etiketiyle)
güncellendi. Ayrı hesaplardaki iki müşteri iki tarafı oynadı: posta kutusu
postayı gönderen alice ve sitesinin kendi hesabı yerel kapıyı deneyen bob.
Laboratuvarın yaptığı yazan yerler dışında her şey sayfanın kendi
düğmeleriyle yapıldı. Ağır hata yok; bu sürümden eski bir sınırla
karşılaşıldı ("Bilinen sınırlar"a bakın).

- **Güncellemeden önce yerel kapı açıktı** (alpha.144'ün kodu), iki
  sunucuda da: bob'un site hesabı olarak — sitesinin PHP'si üzerinden ve
  doğrudan soketten — laboratuvarın koyduğu bir eklenti ayarı yedek kopyası
  ve bir nokta dosyası içerikleriyle verildi; bir eklenti klasörüne konan
  PHP dosyası, `skins/…/../..` yoluyla istendiğinde de çalıştırıldı. Ana
  sunucuda web postasından gönderim "kimlik doğrulanamadı" ile bitti; posta
  günlüğünde yalnız bir bağlantı vardı, ne şifreleme ne giriş.
- **İki sunucu da panelin kendi güncelleme sayfasından kendiliğinden
  güncellendi**, yaklaşık dört buçuk ve yaklaşık dört dakikada. Şema adımı
  yok (81'de kalır); her sunucuda bir panel ve bir Agent çalıştı; yeni
  başarısız hizmet yok. İlk açılışta Agent yerel kapının dosyasını ve web
  postasının yapılandırmasını yeniden yazdığını bir kez söyledi; web
  sunucusu her sunucuda denetimi geçtikten sonra bir kez yeniden yüklendi;
  PHP-FPM yeniden başlatılmadı. Web postasının yapılandırması kipini,
  sahibini ve gizli anahtarını, ana sunucuda da web postasının kendi
  adresinin satırlarını korudu. Dışarıdan saniyede bir yapılan denetim, ana
  sunucuda güncelleme boyunca bir sitenin ve web postasının yanıt verdiğini
  gördü (483 istek, hepsi yanıtlandı); bu denetim yeniden yüklemeden 13
  saniye önce bitti, ikisi de yüklemeden sonra yine yanıt verdi.
- **Güncellemeden sonra yerel kapı** (güvenlik düzeltmesi), iki sunucuda,
  bob'un site hesabı olarak iki yoldan da: konan kopya ve nokta dosyası,
  konan PHP ile Roundcube'un kendi eklenti PHP'si, bir eklentinin
  `composer.json` dosyası, `index.php/x`, iki `..` yolu, bir betik dosyası,
  web postasının yapılandırması ve onun `.celikpanel-pending` kopyası hep
  "bulunamadı" yanıtı aldı; oturum açma sayfası, bir stil dosyası ve
  CelikPanel'in PHP denetimi yanıt verdi. Web postasının kendi adresi
  alpha.144'ün kurallarını korudu.
- **Web postası gönderiyor** (ana sunucu, posta adının sertifikasıyla):
  alice web postasının kendi adresinde oturum açtı ve üç ileti gönderdi:
  kendine, başka bir posta kutusuna ve biri ekli. Her birinden sonra sayfa
  hatasız gelen kutusuna döndü; üçü de Gönderilenler'de. Posta günlüğünde
  her biri için gönderim hizmetine 127.0.0.1'den gelen bağlantının
  şifrelemeye geçtiği (TLS 1.3), posta kutusunun kendi adresiyle giriş
  yapıldığı ve iletinin teslim edildiği görülüyor; içeriden 25. kapıya
  hiçbir şey gitmedi. Ulaşan kopyalar göndericinin kendi alan adı adına
  imzalı ve imza doğrulandı. Dışarıdan gönderilen bir yanıt listelendi ve
  açıldı. Başka bir hesabın adresli bir kimlikle gönderim, gönderim
  hizmetince reddedildi ("Sender address rejected: not owned by user …").
  Dışarıdan alice'in girişiyle posta programları: 587. kapı STARTTLS ile ve
  465. kapı iletiyi kabul etti; başka bir hesabın gönderici adresi
  reddedildi (553).
- **Yeniden başlatma hiçbir şeyi değiştirmez:** laboratuvar Agent'ı ve
  paneli yeniden başlattıktan sonra iki dosya da içeriğini ve zamanını
  korudu, web sunucusu yeniden yüklenmedi ve web postası yine gönderdi.
- **Başka bir sunucu adı için hiçbir şey gevşetilmez:** laboratuvar web
  postasını `127.0.0.1` yerine `localhost`'a yönlendirince (sertifika posta
  sunucusunun adına) gönderim şifreleme adımında durdu; yani ad denetimi
  başka her ad için açık kalıyor. Laboratuvar sonra CelikPanel'in metnini
  geri koydu.
- **Sahibin değiştirdiği yerel kapı dosyası:**
  - CelikPanel'in güncel metnine eklenen bir yorum satırı korundu, satır
    çıkmadı ve kapı kapalı kaldı;
  - alpha.144'ün açık metni bir yorum satırıyla bayt bayt korundu,
    CelikPanel'in metni yanına kondu; bob'un site hesabı konan dosyaları
    yine okuyabildi ve konan PHP'yi çalıştırabildi (onları sahibin dosyası
    veriyor); **Şu anki sorunlar** Türkçe ve İngilizce tek satırda yerel
    kapının Roundcube dosyalarını hâlâ her hesaba verdiğini, sunucu
    sahibinin yapacağını ve adımı söyledi. Sahip CelikPanel'in metnini
    dosyanın üzerine taşıyıp web sunucusunu yeniden yükleyince satır
    sonraki okumada gitmişti.
- **Sahibin değiştirdiği web postası yapılandırması** (alpha.144'ün metni,
  bir yorum satırıyla) bayt bayt korundu; CelikPanel'in metni, yalnız
  root'un okuyabileceği biçimde ve aynı gizli anahtarla yanına kondu. **Şu
  anki sorunlar** Türkçe ve İngilizce tek satır gösterdi: "Web postası posta
  gönderemiyor", nedeni ve adımı; sorun listesini isteyen bir müşteri ve
  bir bayi reddedildi. Sahip CelikPanel'in metnini dosyaya kopyalayınca
  satır, yeniden başlatma olmadan sonraki okumada gitmişti; yanındaki metin
  Agent'ın sonraki açılışında kaldırıldı ve web postası yine gönderdi.
- **Çeşitlemeler:**
  - yalnız gönderim adresi şifreli biçime çevrilip sertifika satırı
    eklenmeyince dosya korundu ve satır, yanındaki metni anarak kaldı; sahip
    sertifika satırını ekleyince satır saniyeler içinde, yeniden başlatma
    olmadan kendiliğinden gitti;
  - laboratuvarın değiştirilemez yaptığı (immutable işareti) bir dosyada
    CelikPanel'in kendi önceki metni: Agent önceki biçimi koruduğunu ve
    sonraki açılışında yeniden deneyeceğini söyledi, satır dosyayı yeniden
    yazılamayan dosya olarak andı; işaret kaldırılıp Agent yeniden
    başlatılınca dosya yeniden yazıldı ve satır gitti;
  - sahibin başka bir sunucuya giden kendi gönderim ayarı anılmadı;
  - dosyanın kipi, sahibi ve gizli anahtarı hiç değişmedi.
- **Bekletilen yolda takma ad beklemesi:** yedek sunucuya ulaşılamazken
  alice bir takma ad ekledi; satırı "DNS kaydı yayımlanmayı bekliyor — her
  saat yeniden denenir, bir şey yapmanız gerekmez" dedi; bölgenin uyarısı,
  değişikliği yeniden yapmayı istemeden takma ad kayıtlarının
  kendiliğinden yayımlanacağını söyledi (Türkçe ve İngilizce). Yedek sunucu
  döndükten yaklaşık altı saniye sonra bekletilen değişiklik yayımlandı,
  takma ad yaklaşık on üç saniye içinde iki sunucudan da yanıt verdi ve
  satır artık beklediğini söylemiyordu. Satırdan kaldırılınca takma ad iki
  sunucuda da yanıt vermez oldu.
- **PostgreSQL penceresinin başlıkları** sonucu söylüyor, Türkçe ve
  İngilizce: bir kapatma "… başka hesaplara kapatıldı", bir Geri al "…
  yeniden her hesaba açık", başarısız bir kapatma (laboratuvar kayıtlı bir
  kullanıcının rolünü silmişti) "… başka hesaplara kapatılamadı" başlığıyla,
  "Yapılmadı — hiçbir şey değiştirilmedi", nedenin türü ve veritabanı
  sunucusunun tırnak içindeki iletisiyle bitti. Hiçbiri "… kapatılıyor"
  ile bitmedi.
- **Sayfasından bir Roundcube güncellemesi** (1.6.20'den 1.6.21'e, ana
  sunucu): güncelleme uygulandı, yapılandırma korundu, web postası 1.6.21
  ile çalışmayı ve göndermeyi sürdürdü; ama panel güncellemeyi bir panel
  yeniden başlatması onu tamamlandı diye kaydedene dek sürüyor gösterdi
  ("Bilinen sınırlar"a bakın).
- İki sunucuda olağan sayfalar betik hatası olmadan açıldı.

**Laboratuvarın sayfanın yolundan ayrıldığı yerler.** Laboratuvar
sunucularında root olarak laboratuvar bir web postası eklentisinin
klasörüne bir yedek kopya, bir nokta dosyası ve bir PHP dosyası koydu ve
yerel kapıya sormak için bob'un sitesine küçük bir betik koydu; yerel
kapının dosyasını ve web postasının yapılandırmasını bir sahibin yapacağı
gibi değiştirdi, Agent'ı yeniden başlattı, sonra CelikPanel'in metnini
onların üzerine aldı; yapılandırmayı değiştirilemez yapıp yeniden
değiştirilebilir yaptı; web postasını başka gönderim sunucularına
yönlendirip CelikPanel'in metnini geri koydu; Agent'ı ve paneli yeniden
başlattı; dışarıdaki bir makineden bir yanıt gönderdi ve posta programlarını
kullandı; başka bir hesabın adresli kimliği web postasının ayarlarına kendi
kestirme yoluyla ekledi; yedek sunucuyu ana sunucudan ulaşılamaz yapıp
yeniden ulaşılır yaptı; veritabanı yöneticisi olarak bir veritabanını
alpha.141'den önce açılanların durumuna getirdi ve kayıtlı bir kullanıcının
rolünü sildi; web postası güncellemesini sonuçlandırmak için paneli bir kez
yeniden başlattı. Deneme boyunca kendi sertifika yetkilisi iki sunucuda da
güvenilir sayıldı.

**Denetlenmeyenler:**

- Gönderim hizmeti kendi imzalı sertifikasını gösterirken (posta adının
  sertifikası henüz yokken) gönderim: o duruma ulaşmak posta adının
  sertifikasını kaldırmayı gerektirirdi; testlerle kapsanıyor.
- Başka bir sunucu adı için sertifika zinciri reddi: laboratuvar makineleri
  laboratuvarın kendi sertifika yetkilisine güvendiği için sertifikası ona
  bağlanan bir ad kabul edildi; yalnız ad denetimi gösterildi.
- Ubuntu 24.04 sunucusunda web postasından gönderim: laboratuvarda orada
  posta hizmeti yok (yapılandırması ana sunucudaki gibi yeniden yazıldı).
- Web postası gönderemezken Webmail kartı: kart takılı kalan güncellemenin
  yazısını gösterdi ("Bilinen sınırlar"a bakın), posta paketinin kartı
  listelenmedi; kanıtlanmadı.
- Yavaş yedek yolunda takma ad beklemesi (bekletilmeyen, bölgenin kendi
  yayını bekliyor): süre içinde oluşturulamadı; testlerle kapsanıyor.
- "Yeniden yazılamadı" satırının ekranı: sorun listesinden okundu, sayfada
  bakılmadı.
- Web postasının yeniden kurulum kanıtı ve PostgreSQL Geri al penceresinin
  "bilinmiyor" sonucu: süre; testlerle kapsanıyor.
- Arch Linux'ta hiçbir şey denenmedi.

## Güncellemeden sonra ne beklenir

- Şema adımı yoktur: düzen şema 81 kalır; alpha.144'e geri dönmek olağan
  geri almadır.
- Web sunucusu ilk açılışta, düzeltilen her dosya için bir kez (yerel
  kapının dosyası; bir web postası adres dosyası yalnız hâlâ önceki
  biçimindeyse), her biri yapılandırma denetimi geçtikten sonra yeniden
  yüklenir.
- Web postasının yapılandırması ilk açılışta bir kez, kipi, sahibi ve gizli
  anahtarı korunarak yeniden yazılır; web postası o andan sonra gönderebilir.
  Elle yapılacak bir şey yoktur.
- **Şu anki sorunlar** bir web postası satırı gösterirse adım o satırdadır.

## Kanıt

Şema değişikliği yoktur: `internal/db/migrations/` altında hiçbir dosya
değişmez; düzen 81 kalır. Yeni olanların hepsi eklemedir: salt okur Agent
çağrısı `WebmailAddressFormsV1`'in `sending`, `sending_file` ve
`sending_reason` alanları (ve yerel kapı için adressiz bir kayıt); bölge
yanıtının `aliases_waiting` alanı; bileşen kaydının `webmail_cannot_send`
alanı; Şu anki sorunlar türü `webmail_cannot_send` ve posta profili kodu
`webmail_cannot_send`. Yeni Agent çağrısı, yol ya da denetim kaydı eylemi
yoktur.

Maddeye göre testler:

- Yerel kapı: `TestA145LocalServerAnswersOnlyCelikPanelsOwnChecks` (işlenmiş
  dosyayı nginx'in konum seçtiği gibi okur: oturum açma sayfası, giriş
  noktası, stil dosyaları ve PHP denetimi yanıt verir; bir eklenti kopyası,
  bir eklentinin PHP'si, nokta dosyaları, `composer.json`,
  `index.php/extra` ve bir `..` yolu bulunamaz; önceki biçim bunları veriyor
  ve çalıştırıyordu), `TestA145ProofAndReadinessStillAnswerOnTheLocalServer`,
  `TestA145EarlierLocalServerIsRewrittenAtStartWithOneReload`,
  `TestA145OwnersLocalServerFileIsKeptWithTheTextBesideIt`,
  `TestA145LocalServerRewriteFailuresThatAreNotAnotherFileAreUnnamed`,
  `TestA145LocalServerPendingTextGoesWithTheFile`,
  `TestA145LocalServerInItsEarlierFormIsAProblemUntilItIsNot`,
  `TestA145LocalServerAlertMailFollowsTheScreensBranches`; dağıtım
  yapılandırma matrisi yeni dosyayı işler (`TestRenderDistroConfigMatrix`, CI'da
  nginx ile denetlenir). Bağımsız bir inceleme yeni dosyayı gerçek bir nginx
  üzerinde de çalıştırdı (Debian 13): yalnız listelenen istekler, hangi
  kodlamayla gelirse gelsin, yanıt alır.
- Web postası gönderir: `TestA145WebmailSendsWithEncryptionAndRelaxesTheCheckOnLoopbackOnly`,
  `TestA145SendingSettingIsReadFromTheLastAssignment`,
  `TestA145EarlierConfigurationIsCorrectedAtTheFirstStart`,
  `TestA145AddressLinesOnTheEarlierTextWriteTheCurrentOne`,
  `TestA145OwnerEditedConfigurationIsKeptWithTheTextBesideIt`,
  `TestA145OwnerSendingSettingIsNotNamed`,
  `TestA145FailedRewriteOfCelikPanelsTextIsItsOwnState`,
  `TestA145RewrittenConfigurationIsPublishedWithItsFinalOwner`,
  `TestA145WebmailThatCannotSendIsAProblemUntilItCan`,
  `TestA145FailedRewriteIsNamedWithTheRestartNotAHandEdit`,
  `TestA145TheOwnerIsGivenCelikPanelsTwoSendingLines`,
  `TestA145WebmailProfileIsNotCompleteWhileItCannotSend`; var olan gönderim,
  imzalama ve web postası adres testleri değişmeden geçer.
- Takma ad beklemesi: `TestA145AliasWaitEndsWhenTheHeldPublicationCompletes`,
  `TestA145AliasWaitIsSettledWhenThePendingPublicationCompletes`,
  `TestA145AliasWaitSettledByTheFollowUpPublicationWithoutASecondOne`.
- Metinler, PostgreSQL penceresinin başlıkları ve bölge uyarısı:
  `web/tests/a145-webmail-send-lab.test.mjs`.

Değişen paketlerin Go testleri ve web testleri temel üzerinde (alpha.144'ün
kodu) ve bu sürümün kodu üzerinde, adıyla aynı başarısız kümeyle çalıştı
(test makinesinde root'a ait geçici klasör gerektiren iki test) ve sürüm
çekme isteğinin her denetimi CI'da geçti. Çekme isteğinin iki bağımsız, salt
okur incelemesi sürümü durduran bir şey bulmadı; bulguları bu sürümde
düzeltildi ya da yukarıda listelendi.
