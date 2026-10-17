# v0.1.0-alpha.95 (aday, yayımlanmadı)

[English](RELEASE-NOTES-v0.1.0-alpha.95.md)

Bu sürüm v0.1.0-alpha.94'ün ardından gelir. Numarayı, sahibin 2026-10-13
tarihli kalıcı talimatıyla sürümü hazırlayan seçti. 2026-10-17'de,
`fix/alpha95-bundle-consistency` dalında, `main` üzerindeki alpha.94 sürüm
kaydından (`d506a05e8c003dffee8de2e1268402865520bd6b`) yazıldı.

## Kim güncellemeli ve neden

alpha.94'ü çalıştıran herkes. alpha.94'e güncelledikten sonra sahip Boston ve
Frankfurt'ta Bileşenler ve Pano'ya baktı ve sorunların yeniden değerlendirilip
düzeltilmesini istedi (sahibin 2026-10-17 kararı, D-034 değişikliği). Bu sürüm o
düzeltmedir. Bir sunucuyu yalnız sahibi, panelin kendi güncelleme ekranından
günceller.

## Ekranda ne değişir

**Her sunucuda aynı paket listesi.** Her Bileşenler kategorisi (E-posta,
Veritabanları, Web sunucuları) her sunucuda aynı hazır paketleri aynı sırayla
gösterir. Yalnız her çipin durumu değişir. Bu, alpha.94'ün "kurulu paket ve en çok
bir öneri daha" kuralını geri çeker; o kural sürümü hazırlayanın hatasıydı:
Boston'da kurulabilir olan MariaDB paketini gizliyordu (Pano ise onu sayıyordu) ve
Temel Posta Boston'da bir çip, Frankfurt'ta yoktu.

Bir çip durumunu sözle ve biçimle söyler, asla yalnız renkle:

- onay işareti ve ad: kurulu ve doğrulandı;
- "Temel Posta · içinde": doğrulanmış bir posta paketi onu içerir ve içerilen
  paketin kendisi de kurulu ve doğrulanmıştır, kendi başarısız ya da süren bir
  denemesi yoktur (aksi halde nedeniyle "doğrulanmadı" der);
- gri nokta ve "doğrulanmadı": kurulu, henüz kanıtlanmadı;
- çerçeveli "Ad · Kur" (eklentilerde "· Ekle"): kurulabilir;
- "n/m bileşen kurulu · Devam et": bir posta kurulumu başlatıldı ve tamamlanmadı.
  "Kur" diyen bir posta çipi (kurulu bir Webmail'in yanında Spam Korumalı Posta;
  Panel onu yalnız parçalar paylaşıldığı için kısmi sayar) posta penceresini "devam
  et" değil, kurulum biçiminde açar;
- kısa bir nedenle soluk metin ("phpMyAdmin · MariaDB gerekli"), sessiz bir
  düğmede: basınca (klavye ya da dokunmayla) tam neden çiplerin altındaki bir
  satırda görünür (ipucu kalır ama tek yol değildir). Henüz kurulamaz; eklentiler
  her zaman görünür;
- bir paket kurulurken hareket eden bir göstergeyle "2/3 · PHP-FPM kuruluyor";
  bu sırada diğer çipler devre dışıdır.

Kurulu bir çipe basmak bir pencere açar: posta paketi için **Onar** (eksik olanı
tamamlar ve kurulumu doğrular; kurulu hiçbir şey kaldırılmaz), bileşen paketi için
**Doğrula** (bileşenleri yeniden okur ve hiçbir şeyi değiştirmez). Pencere paketi
betimler ("Sitelerin veritabanları için PostgreSQL."), emir kipinde değildir.
Şeritte Onar bağlantısı ve ipucu cümlesi yoktur. Bileşenler yakın zamanda
denetlenmediyse şerit tek bir soluk satırdır ve sayfa kendiliğinden denetler.
Çiplerin altında şerit, nedeniyle yalnız başarısız ya da süren denemeleri ve
doğrulanmamış ya da kısmen kurulu çiplerdeki tipli uyarıları söyler; "bazı parçalar
zaten var" notu orada asla görünmez.

**Pano kartı** tek bir özet satırdır ("7 kurulu ve doğrulandı · 1 kurulabilir"),
sonra yalnız sorun satırları (nedeniyle başarısız ya da süren bir posta denemesi,
kurulu ama doğrulanmamış bir paket), sonra "Bileşenler'de aç". Çiplerin çizildiği
gibi sayar: eklentiler dahil her paket; "kurulabilir" kurulabilir ya da kısmi
demektir, engelli sayılmaz; "içinde" kurulu ve doğrulanmış sayılır, ama kendi son
denemesi başarısız olan ya da süren bir paket asla kurulu ve doğrulanmış sayılmaz.
Doğrulanmamış ve ayrıca başarısız ya da süren denemesi olan paket, nedenle tek
satırdır. Hiç denetlenmedi, denetleniyor, tutuluyor, denetlenemedi ve yakın zamanda
değil cümleleri korunur.

**alpha.94'ün üç inceleme maddesi kapandı.**

1. Bir paketin kurulum adımları arasındaki denetim, "durum doğrulanamadı"
   bayrağını değiştiği anda okur; böylece bir ekran yenilemesi onu kaçıramaz.
2. Bir bileşen paketinin Doğrula'sı, sunucuda bir bileşen işlemi sürerken nedeni
   gösterilerek tutulur; işlem bitince yeniden basarsınız. Değişiklikler başka
   nedenlerle tutulurken (meşgul bir ana makine kilidi ya da çalışan bir paket yöneticisi
   dahil) yalnız okuduğu için kullanılabilir kalır: yeniden tarama ucu yalnız
   çalışan bir sunucu kurulumunda, etkin bir hizmet işleminde ya da etkin bir Agent
   değişikliğinde reddeder.
3. Sayfa bir kurulumdan önce sunucuyu denetlerken ve istek henüz kabul
   edilmemişken çip "X kurulmadan önce sunucu denetleniyor…" der; "X kuruluyor"
   yalnız kurulum kabul edildikten sonra söylenir.

**Bileşenler satırları.**

- Durdur simgesi daire içinde bir dur işaretidir (boş kare bir onay kutusu gibi
  okunuyordu). Yeniden başlat, durdur, başlat, onar ve sil tek bir çerçeveli simge
  düğmesini paylaşır; dinlenirken gridir, yalnız imleç ya da klavye odağı altında
  renklenir. Aynı Durdur simgesi bileşen sayfasında ve barındırma türü panelinde de
  kullanılır.
- Çizgilerden oluşan masaüstü sürüm sütunu gitti. Sürüm, tarama onu bildiği yerde
  (bugün PHP-FPM ve Node.js) açıklamanın altında küçük tek aralıklı metin olarak
  görünür.
- "PHP-FPM" önkoşul rozetlerinde ve çiplerde tire yerinden bölünmez.
- "Webmail'i onar" Roundcube satırında yalnız sunucu bir webmail sorunu
  bildirdiğinde durur; aksi halde onarım Webmail çipinin penceresindedir.
- **Roundcube kalıntıları.** Roundcube satırında soluk bir not ("Önceki bir
  Roundcube kurulumu tamamlanmadı; dosyaları bu sunucuda kalmış olabilir." ya da
  "… kaldırması tamamlanmadı; …") ve nötr bir "Kalıntıları temizle…" düğmesi yalnız
  Panel'in kendi kaydı yarım kalmış bir Roundcube kurulumu ya da kaldırması
  gösterdiğinde durur: Panel denetim kaydındaki en yeni Roundcube kurulum ya da
  kaldırma girdisi başarısız bir kurulum ya da kısmi ya da başarısız bir kaldırmadır,
  ya da webmail profilinin son denemesi başarısız olmuştur. Sonraki temiz bir
  kurulum ya da kaldırma sunumu bitirir; kayıt yoksa satır bir şey söylemez (böyle
  bir kaydı olmayan Boston'da satırda kalıntı sunumu görünmez). Roundcube'un kendi
  bileşen sayfasında aynı onayı Bileşenler sayfasında açan bir "Kalıntıları
  temizle…" bağlantısı her zaman vardır. Onay penceresi
  Agent'ın neyi sildiğini sıralar: `/var/lib/celikpanel-webmail` (Roundcube
  dosyaları, kendi ayarları ve adres defteri veritabanı),
  `/var/lib/.celikpanel-webmail.stage`, `/var/lib/.celikpanel-webmail.retired*`,
  `/var/lib/..celikpanel-webmail.stage.retired*` (yarım kalan kurulum ya da
  kaldırmadan kalan kopyalar), `/etc/nginx/conf.d/celikpanel-webmail.conf` (sonra
  Nginx yeniden yüklenir) ve `/run/celikpanel-webmail.sock`; ayrıca posta
  kutularına ve Maildir verisine, diğer sitelere, Nginx'e, PHP-FPM'e ve paketlere
  dokunulmadığını söyler. Kırmızı tehlike biçimi yalnız pencerenin onay düğmesindedir.

**Sayılar.** Bileşenler ve Alan adları için kenar çubuğu sayıları dolu hap yerine
sade soluk sayılardır (bilinmeyen bir sayı yine çizgidir, asla 0 değil). Pano'nun
Alan adları kutusu ve kenar çubuğundaki Alan adları rozeti artık tek bir sayı
kullanır: alan adları artı Alan adları sayfasının listelediği yalnız DNS altyapı
bölgeleri (siteye dönüşen bir bölge, o site olarak bir kez sayılır); böylece kutu,
rozet ve sayfa uyuşur (Frankfurt'ta Pano 0 diyordu, sayfa ise bölgeyi
listeliyordu). Sayı yalnız iki okuma da yanıtlandığında görünür; biri okunurken
kutu yükleme işaretini gösterir; biri okunamadığında bilinmeyendir, asla "alan
adları + 0" değil (önceki sayı yalnız iki okumanın da sayısı varsa korunur); bölge
okuması olmayan yönetici olmayanlar yalnız alan adlarını sayar. Hiçbir şey
dikkat gerektirmediğinde Pano cümlesi artık "Dikkat gerektiren bir şey yok.
CelikPanel sertifikaları, güvenlik duvarını ve kurulu bileşenleri denetledi."
şeklindedir ve eskisi gibi yalnız her okuma yanıtlandığında ve bileşen taraması
tazeyken görünür.

## Ne değişmez

- Bir veritabanı ya da web paketini kurmak: yine her seferinde tek olağan bileşen
  kurulumu, her biri ancak öncekinin taze taramayla bitmesinden sonra; ilk hata,
  başka bir işlem ya da kapalı bir kapı nedeniyle durdurur; Devam et yalnız eksik
  olanı kurar; yeniden yüklemeden sonra hiçbir şey kendiliğinden sürmez.
- Bir posta paketi yine mevcut posta kurulum penceresini açar ve aynı yolu
  çalıştırır.
- Şema 44, geçiş yok; Agent değişikliği yok; yeni Agent iş türü yok; güncelleyici,
  geri alma ve kurtarma alpha.94'teki gibi davranır. Güncelleyici değişmediği için
  (ve sahibin 2026-10-16 talimatıyla) gerçek sistem kaydı yok.
- Kurulu panelleri yalnız sahipleri, panelin kendi güncelleme ekranından günceller.

## Bilinen açıklar

- **Sürümler her bileşen için okunmuyor.** Panel bir sürümü yalnız PHP-FPM ve
  Node.js için alır. Agent ayrıca her hizmet birimi için bir ikili yoklar; ama bu
  bir paket sürümü değildir, birkaç birim için güvenilmezdir (Apache, `named`,
  PostgreSQL) ve birimi olmayan araçları (Certbot) kapsamaz. Gerçek paket sürümleri
  yeni bir Agent okuması ister; bu sürüm eklemez. Açık yol haritası işi: her bileşen
  için kurulu paket sürümünü okumak.
- **Roundcube kalıntıları diskte görülmez.** Panel, dosyaların sunucuda gerçekten
  kalıp kalmadığını hâlâ göremez (bunun için bir Agent okuması yok); yarım kalmış
  kurulum ya da kaldırmaya dair kendi kaydına dayanır. Hiçbir Panel kaydının
  göstermediği bir kalıntıya yalnız Roundcube'un kendi sayfasındaki bağlantıyla
  ulaşılır.
- Kurulu bir sunucuda hiçbir şey ölçülmedi; paket sırası gerçek bir Agent'a karşı
  çalışmadı. P0.1-P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez; aday
  kabul değildir.

## Adayın bağımsız incelemesi (17 Ekim 2026)

Adayın bağımsız incelemesi on sorun buldu. Hepsi düzeltildi ve yukarıdaki metin düzeltilmiş davranışı anlatır:

1. Roundcube kalıntıları Bileşenler satırında yalnız Panel'in kendi kaydı yarım
   kalmış bir kurulum ya da kaldırma gösterdiğinde (ek yük alanı `webmail_cleanup`,
   var olan denetim kaydından okunur) ya da webmail denemesi başarısız olduğunda
   sunulur; Roundcube'un kendi sayfasında bağlantı her zaman vardır
   (`/services?cleanup=roundcube`).
2. Kurulu paket penceresi "… kurar" yerine paketi betimler.
3. Çiplerin altındaki satırlar: yalnız başarısız ya da süren denemeler ve
   doğrulanmamış ya da kısmen kurulu çiplerdeki tipli uyarılar; asla "parçalar zaten
   var".
4. "içinde" yalnız, kendisi de kurulu ve doğrulanmış olan ve başarısız ya da süren
   denemesi bulunmayan içerilen paket için; aksi halde nedeniyle "doğrulanmadı" ve
   Pano onu saymaz.
5. Pano kutusu ve kenar çubuğu rozeti için tek Alan adları sayısı (`domainsCount`).
6. Doğrulanmamış ve başarısız ya da süren denemesi olan paket için nedenle tek Pano
   satırı.
7. Engelli çip sessiz bir düğmedir; basınca tam neden çiplerin altında görünür.
8. Doğrula, meşgul ana makine kilidinde ya da çalışan paket yöneticisinde
   kullanılabilir kalır.
9. İfade: EN "2/3 · installing PHP-FPM"; "n of m installed" (Türkçe "2/3 bileşen
   kurulu").
10. "Kur" diyen bir posta çipi posta penceresini kurulum biçiminde açar.

## Kanıt

Web: değişen ve yeni sözleşme ve bağlanmış testler (Boston ve Frankfurt biçimli
yüklerde ready-bundles-ui-contract ve ready-bundles-mounted; kalıntılar için kayıt,
kayıtsız ve doğrudan bağlantı, engelli neden ve kurulu paket açıklaması dahil;
installed-alpha94-corrections, Alan adları sayısı durumları ve kalıntı kuralı dahil;
Pano kartı, satır düzeni, uzak durum, posta profili ve hizmet kullanılabilirliği
sözleşmeleri); Go: yeni webmail_cleanup_record_test, bundles testi (ana paketinden
önce bir eklenti MariaDB ya da PostgreSQL ister) ve posta profili ön yüz
sözleşmesi. Sürüm sözleşmeleri (sıra politikası 94 -> 95, indirme portalı, imzalı
manifest) önceki ağaçta geçti ve girdileri değişmedi. Son aday ağacındaki kesin
sayılar (17 Ekim 2026): web `npm test` 1599 geçti, 0 başarısız, `npm run build`
başarılı; Go `./cmd/panel/` ve `./internal/...` 7123 geçti, 0 başarısız, 44 atlandı;
`go vet ./cmd/panel/` ve gofmt temiz. Tarayıcı 20. grubu (Boston ve Frankfurt
benzeri taklitler, Türkçe ve İngilizce, açık ve koyu, 1440 ve 390 piksel) inceleme
düzeltmelerinden önce iki kez incelendi; ilk bakış iki pencerede kısa çizgisinden
bölünen PHP-FPM'i, telefonda 44 pikselden küçük Bileşenler denetimlerini ve başıboş
bir ayraç noktasını buldu; hepsi düzeltildi. İncelemeden sonra Boston senaryosu
kalıntı onayını `/services?cleanup=roundcube` ile açar; **20. grup inceleme
düzeltmelerinden sonra yeniden çalıştırılmadı.** Yalnız tarayıcı taklidi; kurulu bir
sunucuda hiçbir şey yok. Şema değişikliği yok (44; yeni yük alanı eklemeli JSON),
Agent değişikliği yok, gerçek sistem kaydı yok.

Bu sürümü yalnız CelikPanel'in güncelleme arayüzü üzerinden kurun. Yayımlamak kurulu
sunucuları güncellemez; bir sunucunun güncellemesini yalnız sahibi, panelin kendi
güncelleme ekranından başlatır.
