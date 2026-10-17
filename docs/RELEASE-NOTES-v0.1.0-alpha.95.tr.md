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
- "Temel Posta · içinde": doğrulanmış bir posta paketi onu içerir;
- gri nokta ve "doğrulanmadı": kurulu, henüz kanıtlanmadı;
- çerçeveli "Ad · Kur" (eklentilerde "· Ekle"): kurulabilir;
- "m bileşenden n · Devam et": bir posta kurulumu başlatıldı ve tamamlanmadı;
- kısa bir nedenle soluk metin ve düğme yok ("phpMyAdmin · MariaDB gerekli"):
  henüz kurulamaz; eklentiler her zaman görünür;
- bir paket kurulurken hareket eden bir göstergeyle "2/3 · PHP-FPM kuruluyor";
  bu sırada diğer çipler devre dışıdır.

Kurulu bir çipe basmak bir pencere açar: posta paketi için **Onar** (eksik olanı
tamamlar ve kurulumu doğrular; kurulu hiçbir şey kaldırılmaz), bileşen paketi için
**Doğrula** (bileşenleri yeniden okur ve hiçbir şeyi değiştirmez). Şeritte Onar
bağlantısı ve ipucu cümlesi yoktur. Bileşenler yakın zamanda denetlenmediyse şerit
tek bir soluk satırdır ve sayfa kendiliğinden denetler.

**Pano kartı** tek bir özet satırdır ("7 kurulu ve doğrulandı · 1 kurulabilir"),
sonra yalnız sorun satırları (nedeniyle başarısız ya da süren bir posta denemesi,
kurulu ama doğrulanmamış bir paket), sonra "Bileşenler'de aç". Çiplerin çizildiği
gibi sayar: eklentiler dahil her paket; "kurulabilir" kurulabilir ya da kısmi
demektir, engelli sayılmaz; "içinde" kurulu ve doğrulanmış sayılır. Hiç
denetlenmedi, denetleniyor, tutuluyor, denetlenemedi ve yakın zamanda değil
cümleleri korunur.

**alpha.94'ün üç inceleme maddesi kapandı.**

1. Bir paketin kurulum adımları arasındaki denetim, "durum doğrulanamadı"
   bayrağını değiştiği anda okur; böylece bir ekran yenilemesi onu kaçıramaz.
2. Bir bileşen paketinin Doğrula'sı, sunucuda bir bileşen işlemi sürerken nedeni
   gösterilerek tutulur; işlem bitince yeniden basarsınız. Değişiklikler başka
   nedenlerle tutulurken yalnız okuduğu için kullanılabilir kalır.
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
- **Roundcube kalıntıları.** Panel, yarıda kalmış bir Roundcube kurulumunun ya da
  kaldırmasının dosyalarının sunucuda olup olmadığını söyleyemez; bu yüzden satırda
  soluk bir not ve nötr bir "Kalıntıları temizle…" düğmesi durur. Onay penceresi
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
Alan adları sayısı artık Alan adları sayfasının listelediği altyapı DNS bölgesini
içerir (Frankfurt'ta Pano 0 diyordu, sayfa ise bölgeyi listeliyordu). Hiçbir şey
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
- **Kenar çubuğundaki Alan adları rozeti** hâlâ yalnız alan adlarını sayar; Pano ve
  Alan adları sayfası ise artık altyapı DNS bölgesini içerir. Küçük bir kalan fark
  (rozet 0 için sayı göstermez).
- **Roundcube kalıntıları saptanamaz.** Panel'de kalıntıların var olup olmadığını
  söyleyen bir alan yok; bu yüzden düğme Roundcube satırında her zaman sunulur ve not
  "kalmış olabilir" der.
- Kurulu bir sunucuda hiçbir şey ölçülmedi; paket sırası gerçek bir Agent'a karşı
  çalışmadı. P0.1-P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez; aday
  kabul değildir.

## Kanıt

Web: değişen ve yeni sözleşme ve bağlanmış testler (Boston ve Frankfurt biçimli
yüklerde ready-bundles-ui-contract ve ready-bundles-mounted; installed-alpha94-
corrections; Pano kartı, satır düzeni, uzak durum, posta profili ve hizmet
kullanılabilirliği sözleşmeleri); Go: bundles testi (ana paketinden önce bir eklenti
MariaDB ya da PostgreSQL ister) ve posta profili ön yüz sözleşmesi. Sürüm
sözleşmeleri (sıra politikası 94 -> 95, indirme portalı, imzalı manifest) geçer.
Aday ağacındaki kesin sayılar (17 Ekim 2026): web `npm test` 1594 geçti, 0 başarısız,
`npm run build` başarılı; Go `./cmd/panel/` ve `./internal/...` 7122 geçti, 0
başarısız, 44 atlandı. Tarayıcı 20. grubu (Boston ve Frankfurt benzeri taklitler,
Türkçe ve İngilizce, açık ve koyu, 1440 ve 390 piksel) iki kez incelendi; ilk bakış
iki pencerede kısa çizgisinden bölünen PHP-FPM'i, telefonda 44 pikselden küçük
Bileşenler denetimlerini ve başıboş bir ayraç noktasını buldu; hepsi düzeltildi.
Yalnız tarayıcı taklidi; kurulu bir sunucuda hiçbir şey yok.

Bu sürümü yalnız CelikPanel'in güncelleme arayüzü üzerinden kurun. Yayımlamak kurulu
sunucuları güncellemez; bir sunucunun güncellemesini yalnız sahibi, panelin kendi
güncelleme ekranından başlatır.
