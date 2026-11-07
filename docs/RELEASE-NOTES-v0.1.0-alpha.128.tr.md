# v0.1.0-alpha.128

[English](RELEASE-NOTES-v0.1.0-alpha.128.md)

> **Yayımlandı**, 7 Kasım 2026 (UTC): sürüm commit'i `818a03e36`, GitHub ön sürümü 10:48:24Z, indirme portalı yaklaşık 10:56Z (dış denetim 27/27).

Her site kendi PHP ayarlarını alıyor; bu ayarlar sunucunun, bayinin ve
planın koyduğu sınırların içinde kalıyor. Planlar artık bir sınır aşılınca
siteyi kapatmak yerine yalnız uyarabiliyor. Giden posta posta kutusu, alan
adı ve barındırma hesabı başına sınırlanabiliyor. Birden her zamankinden çok
daha fazla gönderen bir posta kutusu, parolası değiştirilene dek
durduruluyor. Her site web sunucusu ayarları, hata sayfaları, sayfa
önbelleği ve günlük saklama süresi alıyor. Node.js sitelerine ortam
değişkenleri ve Git ile dağıtım ekleniyor; bir web sitesi bütünüyle
kopyalanabiliyor. Bir site, bir çalışma arkadaşına yalnız tek bir klasörü
gören bir FTP/SFTP girişi verebiliyor. Veritabanları dışa aktarma, içe aktarma, kopyalama,
denetleme ve onarma araçları alıyor. Siteler ziyaretçi istatistikleri
kazanıyor. Kendi programlarınız, jetonlu bir uzak API ile bir hesap üzerinde
çalışabiliyor. Her kişinin, işlem bekleyenleri en üstte gösteren tek bir
bildirim listesi var. Hesaplar oturum açık tarayıcılarını görüp
kapatabiliyor; sunucunun bakım sayfası sunucuyu yeniden başlatabiliyor.
Yeni gelen her şey biri açana dek kapalı ya da değişmeden
kalıyor; güncellenen bir sunucu tam eskisi gibi çalışıyor. **Kurulu
panelleri yalnız sahibi, panelin kendi güncelleme ekranından günceller.**

## Sınırlar içinde site ayarları

- **Site başına PHP:** bellek, çalışma süresi, en büyük yükleme, form alanı
  sayısı ve PHP işlemi sayısı sitenin PHP sayfasından seçiliyor. Bir site
  yalnız sunucunun, bayinin ve planın koyduğu sınırlar içinde değer
  seçebiliyor. Uygulamalar da bu sınırları yükseltemiyor. Sayfa, sınırı
  hangi düzeyin koyduğunu ve kimin yükseltebileceğini söylüyor.
- **Sahibin sunucu sınırları:** 512 MB bellek, 300 saniye, 256 MB yükleme ve
  20 PHP işlemi. Yönetici bunları sunucunun sınırlar sayfasından
  değiştirebiliyor. Yöneticinin kendi sitelerinde sınır yok.
- **Uygulamalar:** sınırdan fazlasını isteyen bir uygulama sınırı alıyor;
  sitenin PHP sayfası uygulamanın ne istediğini gösteriyor. Bir uygulama
  kurulduktan sonra sitenin yükleme boyutu, sınır içinde kalarak
  uygulamanın boyutunu izliyor.
- **Bir sınır düşürüldüğünde:** sınırın üstündeki siteler yeni sınırla
  çalışmayı sürdürüyor. Müşteri, bayisi ve yöneticiler, neyin değiştiğini
  ve sonra ne yapılacağını söyleyen birer bildirim alıyor.
- **Zamanlanmış görevler:** planlar bir sitenin kaç görevi olacağını ve bir
  görevin ne sıklıkla çalışabileceğini sınırlayabiliyor (yeni görevler için
  varsayılan en az 5 dakika ara).
- **Veritabanı bağlantıları:** planlar bir veritabanı kullanıcısının aynı
  anda kaç bağlantı tutabileceğini sınırlayabiliyor. Sınır MariaDB ya da
  PostgreSQL'in içinde ayarlandığı için panel olmadan da çalışıyor.
  PostgreSQL'de kullanıcının kendi çalışan kümesine yazılıyor.
- **Kim neyi değiştirebilir:** yeni izinler "PHP ayarları", "PHP sürümü" ve
  "Ek nginx yönergeleri". Daha önce PHP'yi değiştirebilen herkes bu hakkı
  koruyor. Bir planın nginx yönergeleri verebilmesi için önce yöneticinin
  bunları sunması gerekiyor.
- **Parola düzeyi:** yönetici parola kuralını herkes için sıkılaştırabiliyor:
  rakamlı 10 karakter ya da rakamlı 14 karakter.

## Planlar

- **Yalnız uyar:** bir site bir sınırı aşınca plan, siteyi kapatmak ya da
  hesabı askıya almak yerine uyarabiliyor. Sunucuyu korumak için dolu bir
  disk yine yüklemeyi durduruyor. Uyarı düzeyi (%50–95, varsayılan %80)
  plan başına ayarlanabiliyor.
- **Ön izleme:** bir planı kaydetmeden önce hangi sınırların düştüğünü ve
  kaç sitenin yeni bir sınırdan fazlasını kullandığını gösteriyor.
- **Planı kopyala:** formu "<ad> (kopya)" adıyla, dolu olarak açıyor.
- **Havuzdan fazlasını dağıtmak:** bir bayi planı, havuzunda kalandan
  fazlasının dağıtılmasını yasaklayabiliyor. Varsayılan, eskisi gibi izin
  vermek.
- **Özel hesaplar:** Özel bir aboneliğin ya da Özel bir bayinin, haklar
  formunda kendi sınırları var.

## Giden posta sınırları

- **Yönetici açana dek kapalı.** Güncellemeden sonra her sunucu postayı
  tam eskisi gibi gönderiyor. Bileşenler'den "Giden posta sınırları
  (postfwd)" kurulduktan sonra posta sunucusu sayfasından açılıyor.
  postfwd her sistemin kendi paketinden geliyor.
- **Sahibin sınırları:** saatte posta kutusu başına 500, alan adı başına 750,
  barındırma hesabı başına 1000 alıcı. Yeni bir posta kutusu ilk gününde
  saatte 50 gönderebiliyor. Planlar daha düşük sınırlar koyabiliyor. Bir
  sınıra ulaşılınca gönderene daha sonra yeniden denemesi ve gönderimin ne
  zaman açılacağı söyleniyor. Hiçbir şey kaybolmuyor.
- **Güvenlik durdurması:** birden her zamankinden çok daha fazla gönderen
  bir posta kutusu durduruluyor ve yöneticiye hemen bildiriliyor. Yalnız
  parola değişikliği onu yeniden açıyor.
- **postfwd durursa posta akmaya devam ediyor.** Yalnız kimliği doğrulanmış
  gönderim sayılıyor; 25 numaralı porttan gelen posta sayılmıyor.

## Posta işleri

- **Toplu işlemler:** posta kutularını seçip gönderim sınırlarını
  değiştirmek, boşaltmak ya da silmek, tek onayla.
- **Posta kutusunu boşaltma** ve **her posta kutusunun altında takma
  adları**, "Takma ad ekle" ile birlikte.
- **Plan başına en büyük posta kutusu.**
- **Alan adı başına seçenekler:** "yalnız gönder" (alan adı başka
  sunuculardan gelen postayı reddeder) ve bilinmeyen adresler için kendi
  yanıt metniniz.
- **Teslim sorgusu:** son 7 günde bir iletiye ne olduğu, yalnız o alan adı
  için.
- **Posta sunucusu:** gönderen bir sunucunun aynı anda tutabileceği
  bağlantı sayısına sınır.

## Her site için web sunucusu ayarları

- **Ayarlar ve ek nginx yönergeleri:** yönetici her zaman yönerge
  ekleyebiliyor. Bayiler ve müşteriler yalnız yönetici bunları sunduysa ve
  planları izin veriyorsa ekleyebiliyor. Yasak listesindeki yönergeler
  reddediliyor. Her değişiklik `nginx -t` ile denetleniyor; nginx
  reddederse geri alınıyor. Sunucuda kendi yaptığınız düzenlemeler
  korunuyor.
- Site başına **özel hata sayfaları**.
- Site başına **sayfa önbelleği**, "Önbelleği temizle" ile.
- **Günlük saklama:** plan ya da hesap başına. Yalnız sitenin eski, döndürülmüş
  günlük dosyaları siliniyor; sistemin kendi günlük döndürmesi
  değiştirilmiyor.
- **Node.js:** ortam değişkenleri (gizli değerler gösterilmiyor) ve paket
  betikleri.
- **Bir web sitesini kopyalama:** başka bir siteye, isteğe bağlı bir MariaDB
  veritabanıyla. İçeriği olan bir site önce yedekleniyor.
- **Git ile dağıtım:** HTTPS ya da SSH deposundan, sunucuda üretilen bir
  dağıtım anahtarıyla (açık yarısı bir kez gösteriliyor). Derlemeler site
  korumasının içinde çalışıyor.

## Site başına daha fazla FTP/SFTP girişi

- **Tek klasörlük giriş:** sitenin sahibi bir tasarımcıya ya da çalışma
  arkadaşına, sitenin yalnız tek bir klasörünü gören kendi kullanıcı adını
  ve parolasını verebiliyor. SFTP ile, vsftpd CelikPanel'inse TLS'li FTP ile
  çalışıyor. Bir giriş asla komut satırı açamıyor ve yalnız okunur
  yapılabiliyor.
- **Dosyalar sitenin kalıyor:** yüklenenler sitenin oluyor; PHP onlarla
  eskisi gibi çalışabiliyor.
- **Plan sınırı:** varsayılan olarak barındırma hesabı başına 10 giriş. Bir
  plan, bayi planı ya da Özel hesap bunu düşürebiliyor; 0 hiç izin vermiyor.
- **Kaçış denemeleri:** desteklenen beş sistemin hepsinde 64 denemenin 64'ü geçti.
  Denemeler başka klasörleri, başka siteleri, bağlantıları, yeri değiştirilen
  bir klasörü, yeniden başlatmaları, yönlendirmeyi ve düz FTP'yi kapsadı.
- **Panel olmadan:** her giriş sıradan sistem yapılandırmasıdır (bir systemd
  birimi, bir sshd bloğu, bir vsftpd kaydı) ve çalışmayı sürdürür; rehber
  birinin elle nasıl kaldırılacağını gösterir (docs/FTP-LOGINS.md).

## Veritabanları ve istatistikler

- **Veritabanı araçları:** MariaDB ve PostgreSQL için dışa aktarma, içe
  aktarma (önce bir güvenlik kopyası alınıyor), kopyalama, denetleme ve
  onarma, boyut ve tablolar. Yüklenen bir dosya yönetici olarak değil,
  her zaman veritabanının kendi kullanıcısıyla yükleniyor.
- **Geri yükleme retleri** artık genel bir hata yerine veritabanı motorunun
  kendi nedenini veriyor.
- Site başına, sitenin kendi erişim günlüğünden **ziyaretçi
  istatistikleri**. Biten günler saklanıyor; ziyaretçilerin adresleri asla
  saklanmıyor.
- **IP adresleri:** yönetici sunucunun adreslerini görebiliyor (yalnız
  okuma).

## Bildirimler, oturumlar ve sunucu bakımı

- **Kişi başına tek bildirim listesi** (Pano ve Hesabım): işlem yapmanızı
  bekleyenler önce, bilgi amaçlılar sonra geliyor. Her bildirimde düz dille
  nedeni, düzeltmeye götüren bir düğme ve "Görüldü olarak işaretle" var.
  Kullanım bildirimleri, "düşürülen bir sınır bir sitenin PHP ayarını aşağı
  çekti" bildirimleri ve giden posta bildirimleri hep bu listede; bakılacak
  tek yer var. Planım, Haklar ve posta sunucusu sayfasındaki ayrı bloklar
  kaldırıldı. Her bildirim eskisi gibi yalnız bir kez postalanıyor. Sitenin
  PHP sayfası, düşürülen bir sınırın indirdiği değeri yine işaretliyor.
- **E-posta tercihiniz** (Hesabım): her bildirim, yalnız işlem bekleyenler
  ya da hiçbiri; ayrıca bir ek adres. Yönetici her olay türü için bir ek
  adres ekleyebiliyor.
- **Abonelik bitiş tarihi:** hesap, bitiş tarihinden belirli gün önce ve son
  gün yeniden uyarılıyor.
- **Açık oturumlar** (Hesabım): oturum açık her tarayıcı, adresi ve
  tarayıcısıyla; birini ya da diğerlerinin hepsini kapatabiliyorsunuz.
  Yönetici her hesabın oturumlarını görüyor (Güvenlik); bayi de
  müşterilerinin oturumlarını görüp kapatabiliyor.
- **Boşta kalma süresi:** 60 dakika kullanılmayan bir oturum (15 ile 480
  arası, Güvenlik, Panel erişimi) bir sonraki isteğinde kapanıyor.
- **Yönetici adresleri:** yönetici adres listesi girerse, yönetici yalnız bu
  adreslerden giriş yapabiliyor ve API jetonu kullanabiliyor. Sunucunun
  kendisine her zaman izin var, yani SSH tüneli her zaman çalışıyor; yerel bir
  dosya erişimi yeniden açıyor (docs/PANEL-ACCESS.tr.md). Müşteriler ve
  bayiler hiç etkilenmiyor.
- **Kendi etkinliğiniz:** her hesap kendi işlem kaydını okuyabiliyor; bayiler
  müşterilerininkini de görüyor.
- **Alan adı sağlığı:** alan adının Genel bakış sayfasındaki tek bir kart, web
  sitesinin yanıt verip vermediğini, sertifikayı, DNS'i, postayı, disk
  kullanımını ve son yedeği, her birinin düzeltmesiyle gösteriyor.
- **Sunucu bakımı:** sunucuyu yeniden başlatma (başka bir değişiklik
  sürerken reddediliyor), sunucunun saati ve en yoğun işlemleri.
- **Bu sunucudaki sertifikalar:** her sertifikanın, kimin kullandığı, kalan
  gün ve kendiliğinden yenilenip yenilenmediğiyle salt okunur tek listesi.
  Satın aldığınız bir sertifika için sertifika isteği (CSR) de
  oluşturabiliyorsunuz; anahtarı sunucuda kalıyor ve asla gösterilmiyor.
  Sertifika yetkilisi sertifikayı gönderince onu ait olduğu siteye
  kurabiliyorsunuz: panel önce isteğin anahtarına uyduğunu, şu an geçerli
  olduğunu ve her adı kapsadığını denetliyor, sonra yüklenen her sertifika
  gibi kuruyor; anahtar sunucudan hiç çıkmıyor. Panelin kendi sertifikası ve
  posta sunucusunun sertifikası Let's Encrypt ile yenilendiği için satın
  alınan bir sertifika orada nedeniyle reddediliyor; siteye kurun, alan
  adının postası da onu kullanır.

## Uzak API

- **Yönetici açana dek kapalı** (Ayarlar → Panel → Uzak API). Planın da yeni
  "Uzak API" iznini içermesi gerekiyor.
- **Jetonlar** Hesabım → API jetonları sayfasında oluşturuluyor: yalnız okuma
  ya da okuma ve değiştirme; 7, 30, 90 ya da 365 gün sonra bitiyor. Jeton
  oluşturmak parolanızı (ve iki adımlı doğrulama kodunuzu) yeniden istiyor.
  Jeton bir kez gösteriliyor; CelikPanel yalnız özetini saklıyor.
- **Bir jetonun ulaştıkları:** hesabın alan adları, DNS kayıtları, posta
  kutuları, veritabanları, yedekleri, kullanımı ve ziyaretçi istatistikleri;
  başka hiçbir şey. Bir jeton yalnız hesabın kendisinin, planı içinde
  ulaşabildiğine ulaşıyor. Bir alan adını silmek, onay olarak alan adının
  kendisini istiyor.
- **Güvenlik:** jeton başına dakikada 120 istek; bir adresten tekrarlanan
  yanlış jetonlar yavaşlatılıyor; parola değişikliği hesabın jetonlarını
  bitiriyor; ekip üyeleri jeton tutamıyor. Bir jetonla yapılan her değişiklik
  denetim kaydında.
- Rehber: docs/REMOTE-API.tr.md.

## Laboratuvar sonuçları

Birleştirilmiş bir sunucu çiftinde iki laboratuvar turu yapıldı: PowerDNS
ve postalı bir Debian 13 ana sunucu ile BIND'li bir Ubuntu 24.04 katılan
sunucu. Ayrıca sıfırdan bir Debian 13 sunucu kuruldu. Her sunucu sahibin
kurulum komutuyla kuruldu, her güncellemeye panelde bir kez basıldı.
Laboratuvar kendi sertifika yetkilisini ve Git sunucusunu kullandı; gerçek
hiçbir sunucuya dokunulmadı.

- **127 → 128 güncellemesi:** her sunucuda tek basış, 68–93 saniye. Hiçbir
  hizmet bozulmadı. Var olan sitelerin ve postanın sistem yapılandırma
  dosyaları bayt bayt aynı kaldı (450 dosya karşılaştırıldı).
- **Sıfırdan kurulum:** 71 saniye, bozulan hizmet yok.
- **Bozuk güncelleme:** zorla bozulan bir güncelleme kendiliğinden 128'e
  geri alındı; sonraki basış çalıştı.
- **Site ayarları:** sınırın üstündeki değerler, sınırı kimin koyduğu
  söylenerek reddediliyor. PHP 8.4 ve 8.3'te `ini_set`, `set_time_limit` ve
  `.user.ini` bir sınırı yükseltemedi. 140 saniyelik bir betik 130 saniyelik
  sınırda durdu. Plan düşürülünce site planın sınırıyla çalışıyor ve kendi
  değeri saklanıyor: plan yeniden yükselince kendi değeri geri geliyor.
- **Yalnız bildir:** disk dolu ve süre bitimi bildirimleri 10 saniye içinde
  geldi; site açık kaldı, yüklemeler durdu.
- **Web ayarları:** hata sayfaları, önbellek, başlıklar, adres engelleri, ek
  nginx yönergeleri (reddedilenler `nginx -t` sonrası geri alınıyor), sayfa
  önbelleği, Node.js değişkenleri ve HTTPS ile SSH üzerinden Git ile dağıtım.
- **Gerçek Postfix ile giden posta:** sınırını aşan bir posta kutusu,
  sınırı adıyla söyleyen geçici bir ret aldı. Olağandışı bir patlama posta
  kutusunu kalıcı bir retle durdurdu ve yöneticiye bildirdi. Parola
  değişikliği gönderimi 6 saniye içinde yeniden açtı. postfwd durmuşken
  posta akmayı sürdürdü. "Yalnız gönder" ve bilinmeyen adres için kendi
  yanıt çalışıyor.
- **Veritabanları:** tek seferlik indirmeli dışa aktarma, güvenlik kopyalı
  içe aktarma (başarısız içe aktarma geri alınıyor), kopyalama, denetim ve
  MariaDB onarımı; PostgreSQL 16 ve MariaDB'de. Ziyaretçi istatistikleri
  çalışıyor.
- **Uzak API:** varsayılan olarak kapalı; anahtar oluşturmak parola istiyor;
  kapsamlar, başka hesapların alan adları ve kapatma düğmesi uygulanıyor.
- **Oturumlar ve erişim:** boşta kalma süresi oturumu bitiriyor; yöneticiyi
  dışarıda bırakacak bir panel adres listesi reddediliyor; kurtarma dosyası
  erişimi açıyor, silinince yeniden kapatıyor.
- **Sunucu bakımı:** katılan sunucu yeniden başlatıldı ve 29 saniyede
  döndü; iki DNS sunucusu da yanıt veriyordu.
- **Ek SFTP girişleri:** Debian 13 ve Ubuntu 24.04'te oluşturuldu ve
  silindi. Bir giriş yalnız kendi klasörünü görüyor, yüklemeler sitenin
  oluyor, komut satırı açamıyor ve `/etc`'ye ulaşamıyor.
- **Yayından önce bulunup düzeltilen hatalar (14):** sayfa önbelleği
  açıkken her PHP sayfası hata veriyordu; giden posta sınırları
  Debian/Ubuntu'da açılamıyordu, açılınca da hiçbir sınır uygulanmıyordu;
  MariaDB kopyalama ve onarma çalışmıyordu; PostgreSQL içe aktarma yanlış
  satırı gösteriyordu; düşürülen plan sitenin kendi değerini siliyordu;
  parola değişikliğinden sonra gönderim yaklaşık 2 dakika kapalı kalıyordu;
  değiştirilen bir sınır bir saate kadar bekliyordu; yeni bir durdurma
  sınır yazısını gösteriyordu; postfwd öz denetimi bir liste okumuyordu;
  git Bileşenler'den kurulamıyordu; toplu sınır kayıtları belirsiz yanıt
  veriyordu; değişmeyen Postfix eşlemeleri her güncellemede yeniden
  oluşturuluyordu; yalnız bildir hesabına askıya alma tarihi
  gösteriliyordu.

## Bilinen sınırlar

- **Sınırlar içinde site ayarları:** uygulamaların bir sınırı (`ini_set`,
  `.user.ini` ya da `set_time_limit` ile) yükseltemediği PHP 8.4 ve 8.3'te
  laboratuvarda kanıtlandı; diğer PHP sürümleri aynı havuz ayarını
  kullanıyor ama denenmedi. Tarayıcıdaki parola
  denetimi hâlâ sahibin kuralını kullanıyor; sunucunun yanıtı daha sıkı
  düzeyi adıyla söylüyor. Gerçek kullanımdan sınır önerileri alpha.131 için
  planlandı.
- **Giden posta sınırları:** gerçek Postfix ile yalnız Debian 13'te
  denendi. postfwd yeniden başlatılınca saatin sayımı sıfırdan başlıyor.
  Yeniden açılan bir posta kutusu aynı saat içinde tekrar durdurulmuyor;
  olağan sınırı yine uygulanıyor.
- **Web sitesi kopyalama:** bilinen bir uygulamanın yapılandırması henüz
  yeniden yazılmıyor; kopya hâlâ kaynak veritabanını gösteriyor. PostgreSQL
  veritabanları sunulmuyor.
- **Git ile dağıtım:** henüz sürüm ya da geri alma klasörü ve webhook yok.
  Düz ("dumb") HTTP ile sunulan depolar dağıtılamıyor; akıllı HTTP ve SSH
  çalışıyor.
- **FTP/SFTP girişleri:** sshd yalnız anahtar kabul ediyorsa (bulut
  sunucularda yaygın) girişler TLS'li FTP kullanıyor ve ekran bunu
  söylüyor. TLS'li FTP de kurulu değilse ekran hiçbir girişin neden
  bağlanamadığını söylüyor ve Ekle'yi gizliyor; yönetici Bileşenler'deki FTP
  sunucusuna yönlendiriliyor. CelikPanel sunucunun SSH ayarlarında parolayı
  hiçbir zaman açmıyor. Bu uyarı laboratuvarda değil, otomatik testlerle
  denetlendi. Arch'ta sistemin kendi giriş kuralı bu girişler için
  parolalı SFTP'yi reddettiğinden girişler TLS'li FTP kullanıyor. Arch'ta
  vsftpd'nin ek seccomp korumalı alanı kapalı, çünkü TLS veri bağlantılarını
  bozuyor (alpha.117'den beri her FTP hesabını etkiliyordu); chroot ve
  ayrıcalık ayrımı açık kalıyor. Sahip kendi vsftpd'sini çalıştırıyorsa girişler yalnız SFTP
  kullanıyor. Bir giriş sitenin kullanıcı kimliğini paylaştığı için sistemin
  kendi araçları onu site olarak gösteriyor.
- **Günlük saklama:** yalnız sistemin zaten tuttuğunu kısaltıyor. Günlükleri
  daha uzun tutmak için günlük klasörünü taşımak gerekiyor; bu yapılmadı.
- **Laboratuvar kapsamı:** yeni özellikler Debian 13 ve Ubuntu 24.04'te
  denendi. Debian 12, Ubuntu 22.04 ve Arch Linux yalnız otomatik testlerle
  kapsandı. Günlük saklama ve büyük bir sitenin kopyalanması laboratuvarda
  denenmedi.
- **Uzak API:** DNS yayını hâlâ beklenen bir alan adını anahtarla silmek
  yalnız birim testiyle denendi.
- **Yapılmadı, yalnız tasarım notu:**
  - **Posta otomatik yapılandırması** (Thunderbird, Outlook): bu sürümün de
    değiştirdiği site şablonunu değiştiriyor ve sertifika işi gerektiriyor.
    alpha.129'daki şablon değişikliğiyle planlandı.
  - **Posta kutusu kurtarma adresi ve sıfırlama bağlantısı:** parolasız bir
    giriş akışı; kendi güvenlik incelemesini bekliyor.
  - **Daha fazla posta sunucusu ayarı** (akıllı aktarıcı, Dovecot sınırları,
    alt adresleme, DKIM anahtar değişimi, virüs taraması, izin ve engel
    listeleri): yalnız bağlantı sınırı geldi.
  - **Site başına ayrılmış IP:** bir sitenin adresini değiştirmek, o adresteki
    her site için nginx'in varsayılan sitesini değiştiriyor. Önce oluşturma
    testleri ve bir laboratuvar turu gerekiyor.
  - **Web uygulaması güvenlik duvarı (ModSecurity):** Arch'ta resmî paketi
    yok, Ubuntu'da yalnız topluluk deposunda. Sahibin kararını bekliyor.
- Önceki sınırlar sürüyor (alpha.127'ye bakın).

## Kanıt

Geçişler (hepsi eklemeli; planlar ve aboneliklerdeki yeni sütunların
varsayılanları dışında var olan hiçbir şey değişmiyor):

- **066 `site_setting_limits`:** `setting_policy`, `setting_ceilings`.
  Planlar ve aboneliklerde: `overuse_policy` ('close'),
  `notify_percent_disk` ve `notify_percent_traffic` (80). Bayi planları ve
  havuzlarında: `overselling` ('allowed'). `php_settings` ve `php_version`
  izinleri listelenen her hak kümesine ekleniyor.
- **067 `mail_outgoing`:** `mail_outgoing_settings` (tek satır, kapalı),
  `mailbox_outgoing`, `mail_plan_limits`, `mail_outgoing_hourly`,
  `mail_outgoing_notices`, `domain_mail_options` ve `setting_policy`
  tablosunda dört posta satırı (sınırsız).
- **068 `site_web_settings`:** `site_web_settings`, `node_site_env`,
  `log_retention_plans`, `log_retention_subscriptions`,
  `git_deploy_settings`.
- **069 `site_visitor_stats`:** `site_visitor_stats_daily`.
- **070 `api_tokens`:** `api_tokens` (her jetonun yalnız özeti saklanır).
- **071 `ftp_logins`:** `ftp_logins` (parola saklanmaz) ve `setting_policy`
  tablosunda `ftp.max_logins` satırı (10).
- **072 `panel_operations`:** `notification_preferences`, `account_notices`,
  `certificate_requests`; `sessions` tablosunda üç yeni boş sütun (son
  kullanım, adres, tarayıcı); `audit_logs` üzerinde iki dizin.

Her geçişte olduğu gibi alpha.127 bu geçişleri içeren bir veritabanını
reddeder; geri dönmek, güncelleme öncesi alınan anlık görüntüyü geri
yüklemektir. Sürüm sırası 127 → 128.
