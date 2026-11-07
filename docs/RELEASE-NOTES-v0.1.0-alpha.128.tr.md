# v0.1.0-alpha.128

[English](RELEASE-NOTES-v0.1.0-alpha.128.md)

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

_Laboratuvar turundan sonra doldurulacak._

## Bilinen sınırlar

- **Sınırlar içinde site ayarları:** desteklenen her PHP sürümü için,
  uygulamaların bir sınırı (`ini_set`, `.user.ini` ya da `set_time_limit`
  ile) yükseltemediğinin laboratuvar kanıtı henüz yok. Tarayıcıdaki parola
  denetimi hâlâ sahibin kuralını kullanıyor; sunucunun yanıtı daha sıkı
  düzeyi adıyla söylüyor. Gerçek kullanımdan sınır önerileri alpha.131 için
  planlandı.
- **Giden posta sınırları:** beş sistemde gerçek bir Postfix ile baştan sona
  henüz denenmedi: kurulum, bir sınıra ulaşma, panel durmuşken, postfwd
  durmuşken. postfwd yeniden başlatılınca saatin sayımı sıfırdan başlıyor.
- **Posta:** "yalnız gönder" ve bilinmeyen adres yanıtı yalnız benzetilmiş bir
  Postfix'e karşı denendi; gerçek Postfix denemesi laboratuvar turunun
  parçası.
- **Web sitesi kopyalama:** bilinen bir uygulamanın yapılandırması henüz
  yeniden yazılmıyor; kopya hâlâ kaynak veritabanını gösteriyor. PostgreSQL
  veritabanları sunulmuyor.
- **Git ile dağıtım:** henüz sürüm ya da geri alma klasörü ve webhook yok.
- **FTP/SFTP girişleri:** sshd yalnız anahtar kabul ediyorsa girişler TLS'li
  FTP kullanıyor. Arch'ta sistemin kendi giriş kuralı bu girişler için
  parolalı SFTP'yi reddettiğinden girişler TLS'li FTP kullanıyor. Arch'ta
  vsftpd'nin ek seccomp korumalı alanı kapalı, çünkü TLS veri bağlantılarını
  bozuyor (alpha.117'den beri her FTP hesabını etkiliyordu); chroot ve
  ayrıcalık ayrımı açık kalıyor. Sahip kendi vsftpd'sini çalıştırıyorsa girişler yalnız SFTP
  kullanıyor. Bir giriş sitenin kullanıcı kimliğini paylaştığı için sistemin
  kendi araçları onu site olarak gösteriyor.
- **Günlük saklama:** yalnız sistemin zaten tuttuğunu kısaltıyor. Günlükleri
  daha uzun tutmak için günlük klasörünü taşımak gerekiyor; bu yapılmadı.
- Günlük saklama, büyük bir sitenin kopyalanması, Git ile dağıtım, web
  ayarları ve veritabanı araçları için beş sistemdeki laboratuvar turu
  hâlâ açık.
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
