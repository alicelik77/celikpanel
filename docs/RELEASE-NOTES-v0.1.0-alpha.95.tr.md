# v0.1.0-alpha.95 (17 Ekim 2026'da yayımlandı)

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
başarılı; Go `./cmd/panel/` ve `./internal/...` 7124 geçti, 0 başarısız, 44 atlandı;
`go vet ./cmd/panel/` ve gofmt temiz. Tarayıcı 20. grubu (Boston ve Frankfurt
benzeri taklitler, Türkçe ve İngilizce, açık ve koyu, 1440 ve 390 piksel) inceleme
düzeltmelerinden önce iki kez incelendi; ilk bakış iki pencerede kısa çizgisinden
bölünen PHP-FPM'i, telefonda 44 pikselden küçük Bileşenler denetimlerini ve başıboş
bir ayraç noktasını buldu; hepsi düzeltildi. İncelemeden sonra Boston senaryosu
kalıntı onayını `/services?cleanup=roundcube` ile açar; **20. grup inceleme
düzeltmelerinden sonra yeniden çalıştırılmadı.** Yalnız tarayıcı taklidi; kurulu bir
sunucuda hiçbir şey yok. Şema değişikliği yok (44; yeni yük alanı eklemeli JSON),
Agent değişikliği yok, gerçek sistem kaydı yok.

## Yayım kaydı

Aşağıdaki her şey 17 Ekim 2026 tarihlidir (saat dilimi belirtilmedikçe UTC).
Birleştirmeyi, etiketi, denetimleri, portal yayımını ve genel sitenin okunmasını
sürümü hazırlayan kişi yaptı, önceki sürümlerde olduğu gibi sahibin kalıcı
talimatı uyarınca; kurulu panel güncellemelerini yalnız sahip, panelin kendi
güncelleme ekranından başlatır. Yayım için sahibin talimatıyla gerçek sistem
kaydı yok. Bu girdi, aşağıdaki sonuçların hiçbirini depoda bir dosya olarak
saklamaz.

**Ne oldu.**

1. Sürümden önce iki bağımsız inceleme yapıldı. Birincisi on sorun buldu (hepsi
   düzeltildi, yukarıda sıralı); ikincisi sürümü yayıma hazır buldu. Sürümü
   hazırlayan kişi ayrıca yeni Roundcube kaydı okumasını denetim kaydının en yeni
   20.000 girdisiyle sınırladı.
2. "Release v0.1.0-alpha.95: the same ready bundle list on every server, and the
   owner's alpha.94 screen findings" başlıklı #228 numaralı çekme isteği
   `main`'e `5f3cb1a2fa336c529740a20e9a543b8ac086dc8d` commit'i olarak
   birleştirildi. Sürüm sırası değişkeni etiket yapılmadan önce 94'ten 95'e
   geçirildi. `main` koşusu ve etiket koşusu başarılı oldu; sürüm zinciri temiz
   çalıştı (olay yok).
3. `v0.1.0-alpha.95` açıklamalı etiketi (nesne
   `44764f09aa74546bb5b5704fabde8bf1930fc803`, ağaç
   `0e9d3c486cae7250c4d4c4d525d50fd37c56920a`) sürüm commit'ini gösterir. Sürüm
   bir ön sürümdür (pre-release); 2026-10-17T11:01:22Z'de altı dosyayla
   yayımlandı.
4. 17 Ekim 2026'da `CelikPanel-S2-Debian` WSL konuğunda bir indirmeden
   denetlendi (hazırlayan kişi; burada bir dosyada kayıtlı değil): arşivler bayt
   bayt aynı, her biri 66.259.462 bayt, SHA-256
   `9d9116f3199f453c508a605403cf4e6c63c099145dbef0b98cc402abc0852ef5`; manifest
   sıra 95, `published_at` 2026-10-17T10:22:04Z der (SHA-256
   `a3720afcf1230eb4f7ed76c514fd1a81fa65bd0e3033951bd24835bb45ec5b10`); imzası
   (SHA-256 `026029ba235720f708999d79b2700f20873c6cc3b9de20d008b78fc30872e69b`),
   imza anahtarına (SHA-256
   `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`, öncekiyle
   aynı anahtar) karşı `openssl pkeyutl -verify -rawin -pubin` ile doğrulanır;
   iki içerik koruması da 0 döndürdü; derleme Go go1.26.5, CGO kapalı,
   `-trimpath` ile yapıldı. `libexec/get.sh` 95'i sabitler (SHA-256
   `f1846dacedbee5ab860d7765242dc6a979481be95c25ba69f4f48a1d78ce1a02`); içindeki
   politika şimdiki 95, önceki 94, önceki commit
   `d506a05e8c003dffee8de2e1268402865520bd6b` der. Arşivde 243 üye var. Arayüz
   paketi `web/dist/assets/app-DU840jYm.js`, 250.651 bayt, SHA-256
   `f48f3c54869b6c45e7160c1fe49fd6a92c2f90799385f3a76024bbe86a61acf0`.
5. Portal paketi: `portal.tar.gz`, 132.890.919 bayt, SHA-256
   `4f11dc01376626e6db9160f4716b65fa044b4084bd9b24991bced6b5a9985ca1`, 54 girdi;
   etiketin `deploy/build-download-portal.sh` betiğiyle (önceden imzalı kip) iki
   kez bayt bayt aynı derlendi. Sözleşme testi 0 döndürdü; genel doğrulayıcının
   17 testi ve terfi aracının 14 testi geçti.
6. Yaklaşık 11:09Z'de hazırlayan kişi onu kendi bilgisayarından
   `deploy/publish-download-portal.ps1` ile yayımladı: önceki sürüm
   v0.1.0-alpha.94, hedef v0.1.0-alpha.95; durum `committed`, genel doğrulama 26
   istekten 26'sı, `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`.
7. Önceki sitenin yedeği
   `/var/www/vhosts/celikpanel.net/portal-backups/httpdocs.before-v0.1.0-alpha.95.20261017T110941Z.763371.3e1141ace6970b30`;
   yüklenen paket
   `/var/www/vhosts/celikpanel.net/.upload-portal-20261017T110857Z-v0.1.0-alpha.95-c4ef6772d072445a`
   altında saklandı.
8. Yayımdan sonra dışarıdan okuma: `https://celikpanel.net/releases/latest.txt`
   v0.1.0-alpha.95 der; `latest.json` sıra 95'i, yukarıdaki commit ve SHA-256'yı
   gösterir.

**Sahibin bildirdikleri.** 17 Ekim 2026'da sahip Boston ve Frankfurt'u panelin
kendi güncelleme ekranından alpha.94'e güncelledi ve Pano ile Bileşenler ekran
görüntülerini gönderdi; ikisi de `d506a05e8c00` yapısını gösteriyordu. Bu sürümün
düzelttiği şey bu ekran görüntüleridir (yukarıdaki "Kim güncellemeli ve neden" ve
"Ekranda ne değişir" bölümlerine bakın; burada yinelenmez). Boston'un Bileşenler
sayfasının bir ekran görüntüsü, başlıkları tekrarlanan, birleştirilmiş tam sayfa
bir yakalamaydı; normal ekran görüntülerinde sayfa sağlamdı, bu yüzden sayfa
kusuru olarak kaydedilmedi. Kurulu bir sunucuda alpha.95'ten henüz hiçbir şey
bildirilmedi ve çıkarılmamalıdır.

**Yapılmayanlar ve yayımlamanın değiştirmedikleri.**

- Kurulu sunucuları sürümü hazırlayan kişi alpha.95'e güncellemedi. Boston ve
  Frankfurt, sahip güncelleyene kadar alpha.94'tedir.
- İncelemede bulunan küçük işler bilerek sonraki bir sürüme bırakıldı ve
  açıktır:
  1. Doğrula (bileşenleri yalnız yeniden okur), ana makinenin paket yöneticisi ya
     da ana makine kilidi meşgulken hâlâ "meşgul" yanıtı alabilir.
  2. Başarısız bir Webmail denemesi, sonradan temiz bir kaldırma yapılsa bile
     Roundcube temizleme önerisini satırda tutar.
  3. Roundcube'un kendi sayfasındaki temizleme bağlantısı, sayfa yeniden
     yüklenirse onayı yeniden açar.
  4. Telefonda pencerelerdeki düğmeler 34 piksel yüksekliğindedir.
  5. Tarayıcı ekran denetimi (20. grup) son inceleme düzeltmelerinden sonra
     yeniden çalıştırılmadı.
- Her bileşen için kurulu paket sürümlerinin okunması açıktır ("Bilinen
  açıklar"a bakın).
- Bildirilmeyen ve kaydedilmeyenler: Boston ya da Frankfurt'tan alpha.95'e dair
  her şey ve önceki notların bildirilmedi diye saydığı her şey. Her biri
  "kaydedilmedi"dir; "çalıştı" da değildir, "başarısız" da. Paket sırası gerçek
  bir Agent'a karşı çalışmadı.
- Önceki yayımlama kayıtlarının sınırları ([alpha.88
  notları](RELEASE-NOTES-v0.1.0-alpha.88.tr.md)) kalkmış olarak bildirilmedi.
- Yayımlanma kabul değildir. P0.1–P0.5 kısmi kalır; hiçbir P0 işi kapanmaz ya da
  ilerlemez.

Bu sürümü yalnız CelikPanel'in güncelleme arayüzü üzerinden kurun. Yayımlamak kurulu
sunucuları güncellemez; bir sunucunun güncellemesini yalnız sahibi, panelin kendi
güncelleme ekranından başlatır.
