# v0.1.0-alpha.103 (23 Ekim 2026'da yayımlandı)

[English](RELEASE-NOTES-v0.1.0-alpha.103.md)

Bu sürüm v0.1.0-alpha.102'nin ardından gelir. 23 Ekim 2026'da yayımlandı (sonda
"Yayım kaydı"na bakın); henüz hiçbir parçası bir kurulu sunucuda değil.
23 Ekim 2026'da, aday olarak, `fix/alpha103-fresh-update-check` dalında, alpha.102'nin yayım
kaydından sonraki `main`'den yazıldı (alpha.102 sürüm commit'i
`085cc400131072f3bcda81b560bfcf98dad47f03`).

## Kim güncellemeli ve neden

Yayımlandığında, CelikPanel'i kendi güncelleme ekranından güncelleyen herkes.
Sahibin 23 Ekim 2026'da, iki sunucu da alpha.100'deyken gördüğü iki şey:

- **Frankfurt.** Kenar çubuğu "Update available · v0.1.0-alpha.101" diyordu.
  Açılan güncelleme ekranı, alpha.102 yaklaşık 10:02Z'den beri celikpanel.net'te
  olduğu hâlde "Update to v0.1.0-alpha.101" sundu; alpha.102'yi yalnız "Check for
  updates" gösterdi. Sahibin sözleriyle: "güncellemede her zaman en son versiyonu
  göstermedi … sol alt kısımda 101 nolu güncellemeyi gösterdiğinde ve onu
  tıkladığımda güncelleme ekranında en son güncelleme görünmesi gerekmez miydi".
  Ardından Frankfurt'u kendisi alpha.102'ye güncelledi.
- **Boston.** alpha.102 güncellemesi, sunucunun paket yöneticisi meşgul olduğu
  için hiçbir şeyi değiştirmeden durdu. Bu bildirim açıkken "CelikPanel
  güncellemeleri" kartı yalnız kurulu sürümü, "Teknik ayrıntılar"ı ve otomatik
  denetim anahtarını gösterdi; kenar çubuğu ve Ayarlar öğesi hâlâ "Güncelleme var
  · v0.1.0-alpha.102" diyordu ve bildirim, kartın paket yöneticisinin ne zaman
  boşaldığını söyleyeceğini yazıyordu.

Bir sunucuyu yalnız sahibi, panelin kendi güncelleme ekranından günceller.

## Güncelleme ekranında değişenler

**Ekran açılınca en yeni sürüm denetlenir**: bilinen sonuç on dakikadan eskiyse ve
otomatik denetim açıksa (D-035 değişikliği). Denetim sürerken kart hareketli
"celikpanel.net'te en yeni sürüm denetleniyor…", "Bu denetimden önce bilinen:
v0.1.0-alpha.101 sürümü var (13 saat önce bulundu)." ve "Güncelleme düğmesi bu
yanıtı bekler, en çok bir dakika kadar. Hiçbir işlem kendiliğinden başlamaz." der;
bilinen teklif görünür kalır ve güncelleme düğmesi bekler. Ardından en yeni sürüm
sunulur; kenar çubuğu bağlantısı, Ayarlar öğesi ve Pano satırı aynı sürümü
gösterir.

- On dakika: ekranı bir oturumda birkaç kez açmak her seferinde celikpanel.net'e
  sormasın (ekran açılışları on dakikada en çok bir tamamlanmış sürüm okuması
  yapar; atlanan ya da bitmeyen denetim hiçbir şey saklamaz, bu yüzden sonraki
  açılış yeniden deneyebilir), ama daha yeni bir sürüm çıktığında ve denetim
  çalışabildiğinde ekran 12 saatlik bir sonucu sunmasın diye. Tamamlanan denetim,
  tarayıcı yanıt gelmeden ayrılsa da saklanır.
- "Güncellemeleri denetle" ve zamanlamayla aynı denetimdir, onlarla aynı anda
  değil sırayla çalışır: on dakika içinde tamamlanan bir denetimi bekleyen açılış
  o sonucu kullanır, zamanlama da başka bir denetimin hemen ardından yeniden
  okumaz (düğmenin okuması artık zamanlamanınki gibi 40 saniyeyle sınırlı). Aynı
  kurallar geçerlidir: bu sunucuda son bir saatte bir
  güncelleme başlatıldıysa, sunucu süren bir değişiklik bildiriyorsa, sunucu
  kurulumu sunucuyu değiştiriyorsa ya da Agent yanıt vermiyorsa denetim yapılmaz.
  Meşgul paket yöneticisi onu durdurmaz.
- Denetleyemezse teklif yaşı ve nedeniyle kalır: başarısız bir denetim kendi
  satırını korur ("Son otomatik denetim: az önce — sürüm sunucusundan
  (celikpanel.net) doğrulanmış bir yanıt alınamadı."), karşılanmayan bir koşul bunu
  söyler ("Açılışta yeniden denetlenmedi, çünkü bu sunucuda bir saatten kısa süre
  önce bir güncelleme başlatıldı."; "Açılışta yeniden denetlenmedi: sunucuda başka
  bir değişiklik sürüyordu."), zamanında yanıt gelmezse "Açılışta yapılan denetim
  zamanında yanıt vermedi; bu yüzden sonucu bilinmiyor." der, yanıtı okunamayan denetim (panel bir hatayla ya da okunamayan bir yanıtla döndü) "Açılışta yapılan denetim okunamadı: panel bir hatayla ya da okunamayan bir yanıtla döndü." der; her birinin ardından
  "Gösterilen, son denetimin sonucudur (13 saat önce); şimdi de
  denetleyebilirsiniz." gelir.
- **Otomatik denetim kapalıyken ekranın açılması hiçbir yere sormaz.** Teklif "Son
  denetim: 13 saat önce." der, sonraki satır "Otomatik denetim kapalı. Şimdi bakmak
  için "Güncellemeleri denetle"ye basın." der.

**Durdurulan bir güncelleme kartı boş bırakmaz.** Hiçbir şeyi değiştirmeden duran
(ya da doğrulanarak geri dönen) bir güncellemenin bildirimi açıkken kart teklifi,
"Bu sürüme yapılan önceki güncelleme kurulu sürümü değiştirmeden durdu" başlığını
"Sunucunun paket yöneticisi meşguldü, örneğin otomatik güvenlik güncellemeleri
çalışıyordu. Hiçbir şey değiştirilmedi; …" ile, kartın kendi okumasından paket
yöneticisinin durumunu ("Paket yöneticisi hâlâ meşgul. Bu kart kendiliğinden
yeniden denetler ve güncellemenin ne zaman başlatılabileceğini söyler; hiçbir işlem
kendiliğinden başlamaz.", ardından "Paket yöneticisi işini bitirdi. Güncelleme
yeniden başlatılabilir.") ve kartın köşe bildiriminin örtmediği en üstünde şunu
gösterir: "Önce durdurulan güncellemeyle ilgili bildirimi kapatın ("Bildirimi
kapat"); ardından bu düğme v0.1.0-alpha.102 güncellemesini başlatır. Hiçbir işlem
kendiliğinden başlamaz." Güncelleme düğmesi bildirim kapanana kadar bekler.

Telefonda hazırlık kutusunun "Tekrar dene" düğmesi artık 44 px yüksekliğinde.

## Değişmeyenler

- Hiçbir şey kendiliğinden kurulmaz; ekran, zamanlama ve her yeniden okuma yalnız
  okur. Başlat, bir şey kurulmadan önce tam hedefi yine yeniden denetler ve
  değişmiş bir hedefi reddeder.
- Agent, güncelleyici (`update.sh`, sistem güncelleme işçisi, geri alma ve
  kurtarma), tarayıcıdaki güncelleme kilidi, roller, şema (44, geçiş yok). Sahibin
  16 Ekim 2026 talimatı gereği gerçek sistem kaydı yapılmadı.
- 12 saatlik zamanlama, geri çekilmesi ve anahtarı.

## Bilinen eksikler

- Kurulu bir sunucuda hiçbir şey ölçülmedi. Açılış denetimi gerçek bir Agent'a ya
  da gerçek sürüm sunucusuna karşı çalışmadı; Go testleri Agent'ı taklit eder,
  tarayıcı grubu yerel bir sahte sunucu kullanır.
- Durdurulan güncellemenin bildirimi açıkken köşe bildirimi geniş ekranda kartın
  bir kısmını hâlâ örter; ne yapılacağını söyleyen cümle kartın en üstündedir ve
  görünür kalır.
- Durdurulan bir güncellemenin bildirimi açıkken açılışta denetim yapılmaz;
  bildirim kapanınca, bilinen sonuç on dakikadan eskiyse kart denetler.
  Başlatmadan sonraki bir saat içinde Panel bu denetimi atlanmış olarak yanıtlar
  ("bu sunucuda bir saatten kısa süre önce bir güncelleme başlatıldı");
  "Güncellemeleri denetle" sürümü hemen okur.
- Bildirimi bilinmeyen, kurtarma ya da dikkat durumunda olan başarısız bir
  güncellemede kart hâlâ yalnız sürümü ve anahtarı gösterir, kenar çubuğu ise
  "Güncelleme var" der. Orada Başlat'ı tutmak doğrudur; kartın hiçbir şey
  söylememesi sonraki bir sürüme kalan bir eksiktir.
- Açılış denetimi sürümü okuyan ve saklanan sonucu yazan bir GET'tir. Oturumu
  açık bir yöneticinin tarayıcısı başka bir siteden üst düzey bir bağlantıyla ona
  gönderilebilir (oturum çerezi SameSite=Lax); on dakikada bir tamamlanmış
  okumayla sınırlıdır, mevcut "Güncellemeleri denetle" okumasıyla aynı açıklık.

## Kanıt

Aday ağaçta:

- Go: açılış denetimi (eski sonuç yeniden denetlenir, on dakika içindeki taze sonuç denetlenmez, otomatik denetim kapalıyken Agent çağrısı yapılmaz, bir güncelleme kabul edilmişken ya da sunucu kilidi tutulurken denetim yapılmaz, meşgul paket yöneticisi onu tutmaz, başarısız denetim teklifi korur, yanıt vermeyen Agent atlama sayılır, kapıyı beklerken biten istek hiçbir şey saklamaz, tarayıcı ayrılsa da tamamlanan açılış denetimi saklanır, düğme, açılışlar ve zamanlama arasında aynı anda tek sürüm denetimi, bekleyen zamanlanmış denetim koşullarını yeniden okur (tamamlanmış bir denetimin hemen ardından okuma yok, bu arada kabul edilen başlatmada atlama), düğmenin okuması 40 saniyeyle sınırlı, yalnız yöneticiler, lisans kilidinde okunabilir); son Go ağacında `CelikPanel-S2-Debian` konuğunda geçti (Panel ve iç paketler, 45 dakika sınırı, CGO kapalı): 7.229 geçti, 44 atlandı, 0 başarısız (2.973 üst düzey test), başarısız paket yok; gofmt ve go vet temiz
- Web: 1.757 test geçti, 0 başarısız; aralarında eski bir saklı teklif ve daha
  yeni bir sürüm (bekleme, ardından en yeni sürümün sunulması ve kenar çubuğunun
  izlemesi), taze bir sonuç (denetim yok), otomatik denetim kapalı (istek yok, yaş
  gösterilir), başarısız, atlanan, yanıtsız ya da okunamayan denetim (teklif
  nedeniyle kalır; HTTP hatası ya da okunamayan yanıt, zamanında gelmeyen yanıttan
  ayrı söylenir), bir güncelleme izlenirken denetim yapılmaması, Boston'ın
  durdurulmuş hâli (teklif, nedeniyle önceki deneme, "hâlâ meşgul" ve "işini
  bitirdi", bildirimi bekleyen Başlat, aynı sürümü söyleyen kenar çubuğu),
  durdurulan güncellemenin bildirimi kapanınca denetimin çalışması (sonuç yeniyse
  çalışmaması) ve açık bildirimden çizilen denemenin kaydedilen nedeni taşıması
  için bağlanmış testler var;
  üretim derlemesi paket bütçeleri içinde geçti (hiçbir sınır yükseltilmedi).
- Tarayıcı incelemesi, 26. grup (yerel sahte sunucu; Frankfurt'un eski teklifi ve
  Boston'ın meşgul duruşu; İngilizce ve Türkçe, açık ve koyu, 1440 ve 390 px; 40
  durum, incelemeden sonra da 1440 EN açık ve 390 TR koyu kapanmış bildirim hâli:
  açılış denetimi çalıştı, Başlat açıldı): bir kez bakıldı, düzeltildi ("önce bildirimi kapatın" cümlesi köşe
  bildirimi onu örttüğü için kartın en üstüne taşındı, içindeki sürüm bölünmez
  tutuldu, telefonda Tekrar dene 44 px), yeniden bakıldı; ham anahtar yok, Türkçe
  sayfada İngilizce yok, yana kaydırma yok, telefonda 44 px'ten küçük hedef yok.
- Kabuk: `archlinux` konuğunda temiz bir kopyada çevrimdışı sürüm kurtarma
  düzeneği geçti (1.109 test, 1 atlandı; yansıttığı sonuç metinleri ve mantığı
  değişmediği için içinde güncellenecek bir şey yoktu; inceleme turundan önce çalıştırıldı, o tur da bunların hiçbirini değiştirmedi), ve sürüm sırası politikası
  ("102 -> 103; historical bootstrap sequence 102"), indirme portalı ve imzalı
  sürüm manifesti sözleşmeleri geçti.

## Nasıl güncellenir

Yayımlandığında: sunucunun sahibi tarafından, panelin kendi güncelleme ekranından
(Ayarlar → CelikPanel güncellemeleri). Öncesinde ya da sonrasında yapılacak bir şey
yok.

## Yayım kaydı

Aşağıdaki her şey 23 Ekim 2026 tarihlidir (saat dilimi belirtilmedikçe UTC). Sürüm
commit'i 2026-10-23T16:23:04+03:00 taşır; bu, 13:23:04Z ile aynı andır.
Birleştirmeyi, denetimleri, portal yayımını ve genel sitenin okunmasını sürümü
hazırlayan kişi yaptı, önceki sürümlerde olduğu gibi sahibin kalıcı talimatı
uyarınca; kurulu panel güncellemelerini yalnız sahip, panelin kendi güncelleme
ekranından başlatır. Yayım için sahibin talimatıyla gerçek sistem kaydı yok. Bu
girdi, aşağıdaki sonuçların hiçbirini depoda bir dosya olarak saklamaz.

**Ne oldu.**

1. Çekme isteğinden önce adayın bağımsız bir incelemesi bir zorunlu düzeltme (açık
   bir durdurma bildirimi, sayfa yenilemeleri boyunca açılış denetimini devre dışı
   bırakıyordu) ve daha küçük bulgular buldu; hepsi düzeltildi. Yukarıdaki "Bilinen
   eksikler" altında sayılan eksikler kayıtlı kalır.
2. "Release v0.1.0-alpha.103: the update screen checks for the newest release, and
   a stopped update keeps the card" başlıklı #244 numaralı çekme isteği `main`'e
   `f780d5f08d0f295fbf7d249a853d9c130cd28779` commit'i olarak birleştirildi;
   yazarı Mehmet Ömer Efe Çelik. Manifestin `published_at` değeri
   2026-10-23T13:23:04Z.
3. Sürüm bir ön sürümdür (pre-release), `v0.1.0-alpha.103`; GitHub'da
   2026-10-23T14:08:46Z'de yayımlandı.
4. Hazırlayan kişi yayımdan önce denetledi (burada bir dosyada kayıtlı değil):
   arşivler 66.351.430 bayt, SHA-256
   `8a065499b59764cbefbdbe1dc16cfe754b240d865f00191484f75ec2e4ae9a00`; sağlama
   toplamları, manifest ve arşiv üyeleri denetlendi; manifestin imzası yayın
   anahtarıyla (SHA-256 `7eadeb0b…c68bb`) doğrulanır; içerik korumaları
   geçti; `libexec/get.sh` 103'ü sabitler; `install.sh` etiketinkidir; içindeki
   politika 102'den sonra 103 der; Panel ikilisi yeni açılış denetimi yolunu
   taşır; arayüz paketi `app-DVVjTxvw.js`.
5. Portal paketi iki kez derlendi ve bayt bayt aynı çıktı; terfi aracının kendi
   açma ve normalleştirme adımı onu kabul etti.
6. Yaklaşık 14:17Z'de hazırlayan kişi portalı yayımladı: durum `committed`, genel
   doğrulama 26 istekten 26'sı tamam.
7. Yayımdan sonra dışarıdan okuma: `https://celikpanel.net/releases/latest.txt`
   v0.1.0-alpha.103 der; `latest.json` sıra 103'ü ve `f780d5f…` commit'ini
   gösterir.

**Sahibin bildirdikleri.** Sahibin 23 Ekim 2026 tarihli ekran görüntülerine göre:
Frankfurt'u sahip alpha.100'den alpha.102'ye başarıyla güncelledi (bu sürüme yol
açan eski alpha.101 teklifinden sonra). Boston'ın alpha.102 güncellemesi paket
yöneticisi meşgul olduğu için hiçbir şey değişmeden durdu; Boston bilindiği kadarıyla
hâlâ alpha.100'de. alpha.103'ü çalıştırdığı bilinen bir kurulu sunucu yok ve bu
yazılırken alpha.103'ten hiçbir şey bildirilmemişti.

**Yapılmayanlar ve yayımlamanın değiştirmedikleri.**

- Kurulu sunucuları sürümü hazırlayan kişi alpha.103'e güncellemedi. Onları yalnız
  sahip, panelin kendi güncelleme ekranından günceller. alpha.103 henüz hiçbir
  kurulu sunucuda değil.
- alpha.103'ten hiçbir şey kurulu bir sunucuda ya da gerçek sürüm sunucusuna karşı
  ölçülmedi. Sahibin güncelledikten sonraki gözlemi, Boston'ın yeniden denemesi
  dahil, henüz gelmedi.
- Yukarıdaki "Bilinen eksikler" altında sayılan eksikler sonraki bir sürüm için açık kalır.
- **Yaklaşan kabul, bir iddia değil:** Beta 1'de sahip temiz ilk kurulum denemesi
  için iki sunucuyu da silip yeniden kuracak.
- Paketlerin bir veri kataloğuna taşınıp taşınmayacağı sahibe bırakıldı
  ([alpha.101 notları](RELEASE-NOTES-v0.1.0-alpha.101.tr.md), "Bilinen açıklar").
- Bildirilmeyen ve kaydedilmeyenler: Boston ya da Frankfurt'tan alpha.103'e dair her
  şey ve önceki notların bildirilmedi diye saydığı her şey. Her biri
  "kaydedilmedi"dir; "çalıştı" da değildir, "başarısız" da. alpha.102 kaydının açık
  bıraktıkları açık kalır.
- Önceki yayımlama kayıtlarının sınırları ([alpha.88
  notları](RELEASE-NOTES-v0.1.0-alpha.88.tr.md)) kalkmış olarak bildirilmedi.
- Bu kayıtta kod, şema (44) ya da kurtarma davranışı değişmedi.
- Yayımlanma kabul değildir. P0.1–P0.5 kısmi kalır; hiçbir P0 işi kapanmaz ya da
  ilerlemez.
