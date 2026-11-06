# v0.1.0-alpha.126

[English](RELEASE-NOTES-v0.1.0-alpha.126.md)

Panel artık sorularını kendi penceresinde soruyor ve onaylanan her değişikliği
çalışırken adım adım gösteriyor. Bağlı bir sunucuda sonsuza dek "siliniyor"
durumunda kalan alan adı silme sorunu düzeltildi; takılı kalan silme
güncellemeden sonra kendiliğinden tamamlanır. İzleme dürüst işlemci
grafikleri, 30 günlük geçmiş ve "Şu anki sorunlar" listesi gösteriyor;
kurtarma bekçisi boşta duran sunucuda çok daha sessiz. **Kurulu panelleri
yalnız sahibi, panelin kendi güncelleme ekranından günceller.**

## Panelin kendi onay ve ilerleme penceresi

- **Tarayıcı açılır pencereleri kalktı.** Her "Emin misiniz?" sorusu artık
  panelin kendi penceresinde ve panelin dilinde soruluyor (tarayıcının
  "…:1923 diyor ki" penceresi hiçbir yerde çıkmıyor). Esc ya da pencerenin
  dışına tıklamak vazgeçer; ikinci basış değişikliği iki kez göndermez.
- **Onaylanan her değişiklik için ilerleme penceresi.** Onayladıktan sonra
  aynı pencere, güncelleme ekranındaki gibi, ne yapıldığını adım adım ve
  geçen süreyle gösterir. İş sürerken sayfa kilitlidir; hiçbir şeye iki kez
  basılamaz. Sonunda **Tamamlandı** der ya da neden durduğunu, kimin ne
  yapması gerektiğini ve sonraki adımı söyler (varsa çözüm düğmesiyle).
  Listeler kendiliğinden güncellenir; sayfayı yenilemek gerekmez.
- **Alan adı silme gerçek adımlarını olurken gösterir:** posta ayarları →
  web sitesi → DNS → sertifikalar → bitiriliyor. DNS adımı başka bir DNS
  sunucusunu beklerken (yedek sunucunun onayı, yedeğin yetişmesi ya da ana
  DNS sunucusunun yanıt vermemesi) pencere neyi beklediğini söyler. Sayfa en
  fazla 10 dakika kilitli kalır; sonra pencere silmenin **kendiliğinden
  süreceğini** söyler, kapatılabilir ve satır "Siliniyor" demeye devam eder.
- Aynı bildirim ekrandayken artık ikinci kez gösterilmiyor.

## Bağlı sunucuda takılı kalan alan adı silme

Ana DNS sunucusu PowerDNS, yedeği BIND olan bağlı bir sunucuda (denemelerimizde
Frankfurt ve Boston) bir alan adını silmek onu DNS'ten kaldırıyordu, ama ana
sunucunun son denetimi hiç bitmiyor, alan adı sonsuza dek "siliniyor"
kalıyordu. İki hata vardı: PowerDNS bölge listesini yeniden damgaladıktan
sonra yedeğin denetimi her yeni denemeyi reddediyordu; yedeğin DNS denetim
aracı da aynı adreste birden çok kez dinleyen bir DNS sunucusunu kabul
etmiyordu. İkisi de düzeltildi.

- **Takılı kalan silme güncellemeden sonra kendiliğinden biter.** Yedeğin
  denetim aracı CelikPanel'in kendi programıdır ve şimdiye dek hiçbir
  güncelleme onu yenilemiyordu. Ajan artık bir güncellemeden sonra
  başlarken güncel kopyayı yerine koyar (yalnız kurulu dosya CelikPanel'in
  dağıttığı bir dosyaysa; sizin değiştirdiğiniz dosya korunur ve DNS
  sayfası onu değiştirecek tek komutu gösterir). Önce bağlı sunucuyu
  güncelleyin: yeniden başlarken kaldırma isteğini tekrar gönderir. Ana DNS
  sunucusu daha sonra güncellenirse silme, o güncellemeden sonra en geç beş
  dakika içinde biter. Kimsenin bir şeye basması gerekmez.
- **Sile yeniden basmak hiçbir şey başlatmaz.** Bir alan adı zaten
  silinirken yeni bir basış "Bu alan adı zaten siliniyor. Kimsenin bir şey
  yapması gerekmiyor" der ve hiçbir şeyi yeniden çalıştırmaz. Önceden her
  basış bütün silmeyi baştan çalıştırıyor ve kaldırma isteğini ana sunucuya
  yeniden gönderiyordu.
- **Plan sınırı uyarısı web sitesinin adını veriyor.** Bir alan adı
  silinirken Kullanıcılar sayfası "1 web sitesi sayılmıyor" diye
  uyarabiliyordu. Silinmekte olan alan adı artık ölçülmüyor; uyarı da
  trafiği sayılmayan her web sitesinin adını veriyor, bunun ne anlama
  geldiğini söylüyor ve **Aç** düğmesiyle tek adım gösteriyor. Dosya yolları
  yalnız teknik ayrıntılarda.
- **Plan yazılımları yalnız bir hesabın seçebileceklerini listeliyor:** PHP
  ve Node.js sürümleri, Python, MariaDB, PostgreSQL, sertifika sağlayıcıları,
  uygulamalar ve kendi programlar. Sunucu parçaları (nginx, BIND, Postfix,
  Dovecot, certbot, cron ve diğerleri) artık sunulmuyor ve sayılmıyor. Bunları
  içeren kayıtlı bir plan çalışmaya devam eder; plan bir sonraki kayıtta
  onlardan arınır.
- **Hizmet planları:** bir plan formu açıkken **Plan ekle** gizlenir ve diğer
  satırlar düzenlenemez.

## İzleme, 1. aşama

- **Dürüst işlemci grafikleri.** Her nokta son 30 saniyenin ortalamasıdır;
  soluk bir şerit kısa tepeleri gösterir. Önceden 0,2 saniyelik anlık bir
  ölçüm, boşta duran bir sunucuyu %50–100 sıçramalarla çizebiliyordu.
  İşlemci kartı sanal çekirdek sayısını söylüyor.
- **1 saat, 6 saat, 24 saat, 7 gün ya da 30 gün**; her biri en fazla 360
  noktayla çizilir. Geçmiş 30 gün saklanır. Bir aralığın bir güne kadar
  fazlasını göstermesine yol açan hata düzeltildi.
- **Şu anki sorunlar**, İzleme sayfasının en üstünde ve Ana sayfada: durmuş
  bir hizmet (**Başlat** düğmesiyle), %85 ya da daha dolu bir disk, 10 dakika
  boyunca %90 ya da üstünde işlemci veya bellek, uzun bir posta kuyruğu,
  süresi yakında dolacak ya da yenilenemeyen bir sertifika, bekleyen güvenlik
  güncellemeleri. Her biri kimin ne yapacağını ve sonraki adımı söyler;
  denetlenemeyen şey "sorun yok" diye değil, denetlenemedi diye gösterilir.
  Liste kendini dakikada bir yeniler (sekme gizliyken durur).

## Daha sessiz kurtarma bekçisi

Kurtarma bekçisi yaklaşık 30 saniyede bir çalışır. Bekleyen bir şey yoksa
artık 0,3–0,9 saniye yerine yaklaşık 11–15 milisaniye işlemci süresinde
biter; bu yüzden işlemci sıçraması olarak görünmüyor. Kurtarma dosyalarının
tam denetimi yine her açılışta ve en az saatte bir çalışır. Bu denetim
başarısız olursa günlük satırı artık hiçbir kurtarma adımının başlatılmadığını,
sahibin yalnız bu bir güncelleme dışında tekrarlanırsa bir şey yapacağını,
çalıştırılacak komutu
(`sudo /usr/libexec/celikpanel/recovery runtime-status --lang tr`) ve
hatanın kendiliğinden kalkacağını söyler.

## Laboratuvar sonuçları

_Laboratuvar turundan sonra doldurulacak._

## Bilinen sınırlar

- İlerleme penceresi gerçek adımları alan adı silmede gösteriyor. SSL
  sertifikası alma, yedekten geri yükleme, uygulama kurma, PHP sürümünü
  değiştirme ve WordPress araçları hâlâ geçen süreyle birlikte tek bir
  "Sunucuda çalışılıyor…" adımı gösteriyor; kendi adımları alpha.127'de
  gelecek.
- Güncellemenin son veritabanı denetimi hâlâ çalışan veritabanının bir
  kopyasını alıyor. Bunun yerine çalışan panele sormak, alpha.127 için
  planlanan yapısal bir sadeleştirmedir.
- alpha.125'e ya da daha eskisine geri dönmek, yedek DNS sunucusundaki yeni
  DNS denetim aracını yerinde bırakır (aynı istek biçimini konuşur; eski
  program `/var/lib/celikpanel-agent-private/dns-check-tool/` altında
  saklanır).
- Ubuntu 22.04 üzerinde güncelleme alpha.125 laboratuvarında denenmedi.
- Önceki sınırlar sürüyor: yedek sunucusu olan PowerDNS ana sunucusunda DNSSEC
  imzalama yok; site cron çıktısı postayla gönderilmiyor ve saklanmıyor;
  kurulum makinenin kendi adı için AAAA (IPv6) kaydı eklemiyor.

## Kanıt

Geçiş 064 (`metrics_history`): işlemci kısa tepeleri için yeni bir sütun ve
5 dakikalık ile 1 saatlik izleme özetleri için yeni bir tablo; var olan hiçbir
şey değişmiyor. Her geçişte olduğu gibi alpha.125 bu geçişi içeren bir
veritabanını reddeder; geri dönüş, güncelleme öncesi anlık görüntüden geri
yüklemedir. Sürüm sırası 125 → 126. Go testleri (bütün modül) ve web
testleri geçiyor, web paketleri sınırları içinde.
