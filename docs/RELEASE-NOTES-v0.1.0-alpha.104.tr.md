# v0.1.0-alpha.104

[English](RELEASE-NOTES-v0.1.0-alpha.104.md)

Bu, v0.1.0-alpha.103'ü izleyen adaydır. **23 Ekim 2026'da (UTC) yayımlandı: sürüm commit'i `98c4750b4`, GitHub ön sürümü 22:03:19Z, yapıtlar ve imza doğrulanıp dış denetim geçtikten (26/26) sonra indirme portalı yaklaşık 22:10Z.** Hiçbir parçası henüz kurulu bir sunucuda değil; kurulu panelleri yalnız sahip günceller. 23 Ekim 2026'da aday
olarak, alpha.103'ün yayın kaydından sonraki `main` üzerinden
`fix/alpha104-safe-removal` dalında yazıldı (alpha.103 sürüm commit'i
`f780d5f08d0f295fbf7d249a853d9c130cd28779`).

## Kim güncellemeli ve neden

Bileşenler'den bileşen ya da PHP sürümü kaldıran herkes ve Roundcube web postası,
phpMyAdmin ya da phpPgAdmin kullanan herkes. 23 Ekim 2026'da Frankfurt'ta (Debian
13, alpha.103) ne oldu: PHP-FPM 8.6, 8.5 ve 8.4 kuruluydu (8.4 Debian 13'ün kendi
PHP'si), phpMyAdmin, phpPgAdmin ve Roundcube ile. Sahip PHP-FPM 8.4'ü kaldırdı.
Pencere yalnızca sitelerin denetleneceğini söylemişti. İki dakika sonra phpMyAdmin
"Kurulu değil", web postası "doğrulanmadı — Roundcube kurulu, ama Unix soketi
çalışma ortamı hazır değil" durumundaydı. Sunucunun apt geçmişi, kaldırmanın
(`apt-get purge -y --auto-remove`) phpMyAdmin'i, dağıtımın PHP paketlerini ve
yaklaşık 35 kütüphaneyi de sildiğini ve üç aracın da PHP 8.4 üzerinden
sunulduğunu gösterdi. Sahip her düzeltmenin bu sürümde çıkmasına karar verdi ("Tüm
düzeltmeleri 104'te yap. 105'e bir şey bırakma.").

Bir sunucuyu yalnız sahibi, panelin kendi güncelleme ekranından günceller.
Frankfurt'un nasıl geri getirileceği [işlem
rehberindedir](OPERATION-GUIDANCE.tr.md).

## Bir kaldırma adını söylemediği hiçbir şeyi kaldırmaz

Herhangi bir paket kaldırmadan önce (bir bileşen, tek bir PHP ya da PostgreSQL
sürümü; apt ve pacman) Agent, hiçbir şeyi değiştirmeden paket yöneticisine
kaldırmanın ne yapacağını sorar. Kaldırabilecekleri: adını söylediği paketler; aynı
sürümün paketleri (`php8.4-…`); ve yalnızca bu kaldırmanın kullanılmaz hâle
getirdiği, otomatik kurulmuş paketler. Başka her şey — başka bir bileşenin paketi
(phpMyAdmin), bileşenin kendi dağıtım paketi (sürümün sistemin varsayılanı olduğu
anlamına gelir), kaldırılana bağlı başka her paket ya da kuracağı her şey —
kaldırmayı adıyla, hiçbir hizmet durdurulmadan reddeder. Daha önce de kullanılmayan
paketlere (eski bir çekirdek) dokunulmaz. Gerçek kaldırma, benzetimi yapılan tam
listeyi, aynı benzetim bir kez daha yapıldıktan sonra ve tek paket kilidi altında
çalıştırır; arada yanıt değiştiyse reddedilir.

Kaldırma pencereleri bunu düğmeden önce söyler: "Paket yöneticisi bu kaldırmayı
denetledi: tam olarak şu N paketi kaldırır."; ya da "Paket yöneticisi bu kaldırmanın
adını söylemediği paketleri de kaldıracaktı; bu yüzden CelikPanel kaldırmayı
reddeder ve hiçbir şey değişmez:" ve her biri için ne yapılacağı ("phpmyadmin,
phpMyAdmin bileşenine ait: önce phpMyAdmin bileşenini kaldırın ya da PHP-FPM 8.4
bileşenini tutun."; "php-fpm — dağıtımın kendi paketi — buna bağlı: PHP-FPM 8.4, bu
sistemin varsayılanı. …"); o zaman Kaldır sunulmaz. Denetim çalışamadığında: "Başka
bir paket işlemi sürüyor; bu yüzden paket yöneticisine şimdi sorulamadı. Yine de
Kaldır'a basabilirsiniz: o anda yeniden denetlenir ve adını söylemediği bir şeyi
kaldıracaksa reddedilir." Başarısız bir okuma kaldırmayı asla tutmaz.

## Her PHP aracı sizin seçtiğiniz PHP sürümünde çalışır

- **Aracın Yönet penceresi** (Roundcube, phpMyAdmin ve phpPgAdmin'in artık bir
  Yönet penceresi var) "PHP sürümü" bölümünü gösterir: hangi sürümde çalıştığı ve
  diğer kurulu PHP-FPM sürümlerinden bir seçim. "PHP 8.6 kullan" aracı taşır:
  eksikse PHP eklentileri o sürüm için kurulur, web sunucusunun yapılandırması
  yazılır, denetlenir ve yeniden yüklenir, sonra araç kendi soketi üzerinden
  kendisi olarak yanıt vermelidir (bir CelikPanel denetimi o PHP-FPM'deki PHP
  sürümünü ve aracın eklentilerini doğrular, sonra aracın kendi sayfası); olmazsa
  önceki sürümüne geri konur ve kart nedenini söyler. Bekleme gösterilir; bu arada
  bir şey yapmanız gerekmez.
- **Artık hiçbir şey bir aracı kendiliğinden taşımaz.** Önceden her yeniden
  yapılandırma alfabetik olarak son PHP soketini seçiyordu (8.9'u da 8.10'dan sonraya
  koyuyordu); bu yüzden daha yeni bir PHP kurup sonra phpMyAdmin'i yeniden kurmak ya
  da web postasını onarmak araçları ona taşıyordu. Artık bir araç, sürümü kurulu
  kaldıkça o sürümde kalır; henüz bağı olmayan araç sistemin varsayılan PHP'sini
  alır.
- **PHP-FPM sürüm listesi** her sürümü kimin kullandığını söyler: "Kullanan: 2 site,
  phpPgAdmin, Roundcube" ya da "Hiçbir site ya da araç kullanmıyor".
- **Araçların çalıştığı bir PHP sürümünü kaldırmak** onları adlandırır ve "PHP
  [sürüm] sürümüne taşı, sonra kaldır" seçeneğini sunar: her araç taşınır ve
  denenir, sonra sürüm kaldırılır; bir taşıma başarısız olursa hiçbir şey
  kaldırılmaz ve pencere neyin taşındığını, neyin geri konduğunu ve sürümün
  kaldığını söyler. Sürümdeki siteler "Alan Adları'nı aç" ile adlandırılır (önceki
  gibi her sitenin ayarlarından taşınırlar).
- **PHP'si eksik bir araç** "Kurulu" değil, nedeni ve çaresiyle "Hazır değil" der.

## Küçükler

- Debian 13'teki phpMyAdmin (`4:5.2.2-really+dfsg-1+deb13u1`) "5.2.2-really" değil
  **5.2.2** okunur (alpha.101 kuralı "really"den sonra bir sürüm bekliyordu).

## Bağımsız incelemeden sonraki düzeltmeler

Bu adayın bağımsız incelemesi eksikler buldu; sahibin kuralı hepsinin alpha.104'te
çıkmasıydı. Her biri testleriyle birlikte kaynakta:

- **Paket yöneticisi kaldırmayı kabul etmeden hiçbir araç taşınmaz.** "Onları …'e
  taşı, sonra kaldır" önce aynı sürüm için paket yöneticisine kaldırmanın neyi
  alacağını sorar; yanıt gelmedikçe ve başka bir şey almıyorsa hiçbir şeyi taşımaz
  ("Hiçbir araç taşınmadı, hiçbir şey kaldırılmadı: … şu an paket yöneticisine
  sorulamadı …"). Taşıma yalnız pencerenin kendi denetimi yanıt aldığında sunulur.
- **Taşımalar ve ardından gelen ret ya da kaybolan yanıt birlikte söylenir.**
  Pencere taşınan araçları, sonra reddi, sonra sürümün kaldırılmadığını — ya da
  kaldırmanın yanıtı kaybolduysa kaldırılıp kaldırılmadığının bilinmediğini —
  söyler; sürümleri, denetimi ve bileşenleri yeniden okur. Kaldırma isteği bir istek
  kimliği taşır: kaybolan yanıt bir kez yeniden sorulur, asla iki kez çalışmaz.
- **"Başka PHP sürümü kurulu değil"** yalnız kurulu sürümler okunduğunda söylenir;
  aksi hâlde "şu an okunamadı" ve Yeniden dene.
- **Başarısız taşımanın nedeni Türkçe ya da İngilizce söylenir**; olgularından
  (eksik eklentiler, yanıt veren PHP sürümü, HTTP durumu, nginx'in reddetmesi ya da
  yeniden yüklenmemesi). Ekranın bilmediği bir neden "sunucunun söylediği: …"
  diye alıntılanır. Önceki yapılandırma tam geri konamadığında cümle hangi adımın
  başarısız olduğunu söyler. Yanıtın yerine gelen geçit sayfası "bilinmiyor"dur.
- **PHP eklenti paketleri sürümü için eksik olan araç "Hazır değil"** der; "Yönet'te
  PHP x.y'yi yeniden seçin: taşıma onları kurar". Okunan kurulu paketlerdir,
  PHP'nin onları yüklediği değil; taşımanın kanıtı yüklenen eklentileri denetler.
- **Başka bir aracın PHP sorunu artık kurulumu düşürmez.** Başka bir aracın PHP'si
  ilgi isterken phpMyAdmin ya da phpPgAdmin kurmak onu kurar ve o aracı ve çaresini
  söyleyen bir uyarı gösterir.
- **PHP sürümü kaldırıldıktan sonra yeniden kurulan araç** bir zamanlar bağlı olduğu
  kaldırılmış sürümü değil, varsayılan PHP'yi alır.
- **Sahibin düzenlemesinin üzerine yazılmaz (D-022).** CelikPanel dışında
  değiştirilmiş (CelikPanel'in alpha.1'den beri yazdığı biçimlerden biri ya da boş
  bir dosya olmayan) bir veritabanı araçları ya da web postası yapılandırması yeniden
  üretilmez; yanıt dosyayı ve sonraki adımı söyler. Veritabanı araçları
  yapılandırması atomik yazılır; nginx reddederse kipi ve sahibiyle geri konur.
- **Bir kaldırma**, başka PHP sürümlerinin eklenti paketlerine (siteler
  kullanıyor olabilir) ve zaten kullanılmayan paketlere dokunmaz (eski bir öksüz
  yanlış ret doğurmaz). Hizmetleri durdurulmadan önce ve sonra yeniden denetlenir;
  yanıt değiştiyse her hizmet sahibin bıraktığı gibi geri konur (yalnız öyleyse
  etkin ya da çalışır) ve hiçbir şey kaldırılmaz.
- **Bir kurulum asla paket kaldırmaz.** Bileşen kurulumu ve bir aracın eklenti
  kurulumu önce benzetilir; herhangi bir paketi kaldıracaksa paketleri adlandırarak
  reddedilir.
- **Sözcükler.** "Denetlenemedi" artık pacman sistemlerinde apt adını anmaz;
  kaldırılana bağlı paketler için ret artık "ait oldukları bileşeni kaldırın"
  demez.
- **Telefon hedefleri.** Bileşen sayfasının geri bağlantısı, Yardım ve günlüğün
  süzgeci ile Yenile düğmesi telefonda 44 piksel yüksekliğindedir.

## Değişmeyenler

Siteler PHP sürümlerini hâlâ kendi ayarlarından seçer. Güncelleyici, şema (44) ve
kurulu her sunucu değişmez; yalnız sahip günceller.

## Aday üzerindeki kanıt

- Agent: paketin tamamı temiz bir bağlama ad alanında, 4.709 test geçti (1.864 üst
  düzey), 29 atlandı, 0 başarısız; ilk turda sayılan testlerin yanında başka
  sürümlerin eklenti paketlerine ve eski öksüzlere dokunulmaması, durdurmadan sonra
  yeniden denetim ve hizmetlerin yeniden başlatılması, paket kaldıracak bir
  kurulumun reddi, eskimiş kayıt, sahibin düzenlediği yapılandırmaların korunması
  (CelikPanel'in alpha.1'den beri yazdığı her biçim ve boş dosya tanınır), tam geri koyma adımı,
  yapılandırılmış neden ve eklenti paketi varlığı için yeni testler.
- Panel ve iç paketler (45 dakika sınırı, CGO kapalı): 7.239 geçti, 44 atlandı, 0
  başarısız, 64 paket; gofmt ve go vet temiz (gerçek sistem derleme etiketiyle de).
- Web: 1.783 test geçti (yeni: denetim yanıt verdikten sonra sunulan taşıma,
  taşımalar sonra ret, taşımalar sonra kaybolan yanıt, Yeniden dene ile okunamayan
  sürümler, eksik eklenti satırı ve kartı, İngilizce ve Türkçe neden, "bilinmiyor"
  sayılan geçit sayfası, kurulum uyarısı, yeniden sorulan kaldırma yolu, telefon
  hedefleri); derleme her bütçenin içinde.
- Gerçek sistem, iki atılabilir Debian 13 sanal makinesi (QEMU/KVM, her koşudan
  sonra yok edildi). İlk koşu: Frankfurt kaldırması apt'nin kendi benzetimiyle
  phpmyadmin ve phppgadmin adıyla reddedildi ve hiçbir şey değişmedi; temiz bir
  kaldırma tam olarak benzetim listesini kaldırdı; phpPgAdmin PHP 8.5'e taşındı ve
  kanıtlandı, kanıtı başarısız olan bir taşıma bayt bayt geri alındı
  ([kanıt](../deploy/e2e/removal-safety/alpha104-debian13-20261023/README.md),
  arşiv SHA-256 `61714759…2ec1`; önceki iki koşu başarısız oldu ve orada kayıtlı iki
  düzeltmeye yol açtı). İkinci koşu, incelemeden sonra, son Agent koduyla: aynı iki
  kaldırma; Roundcube Agent'ın kendi koduyla kuruldu ve PHP 8.4'te yanıt verdi; web
  postası PHP 8.5'e taşındı ve kanıtlandı, geri taşındı, kanıtı başarısız olan bir
  taşıma (8.5'in FPM'inde intl kapalı) `missing_extensions`/intl nedeniyle
  reddedildi ve bayt bayt geri alındı; Agent'ın PHP-FPM 8.4'ü gerçek kaldırması
  phpmyadmin, phppgadmin ve webmail adıyla REMOVAL_TOOLS_BOUND ile reddedildi, paket
  kümesi değişmedi ve php8.4-fpm çalışır kaldı
  ([kanıt](../deploy/e2e/removal-safety/alpha104-review-debian13-20261023/README.md),
  arşiv SHA-256 `8f9dca04…2a44ce2`).
- Tarayıcı incelemesi 27. grup: 8 yapılandırmanın her birinde 17 durum (EN/TR,
  açık/koyu, 1440/390), inceleme durumları dahil; ham anahtar yok, Türkçe sayfada
  İngilizce yok, telefonda 44 pikselden küçük denetim yok, yana taşma yok; bakıldı,
  düzeltildi (taşımalardan sonraki ret telefonda artık onların yanında; eklentisi
  eksik kart kendi sürümünü seçili getirir), yeniden bakıldı.
- `archlinux` konuğunda temiz bir klonda: sürüm sırası (103 → 104), indirme portalı
  ve imzalı bildirim sözleşmeleri ile çevrimdışı sürüm kurtarma düzeneği (1.109 test,
  1 atlandı) geçti.

## Bilinen eksikler

- Kurulu bir sunucuda hiçbir şey ölçülmedi.
- pacman kaldırma ve kurulum koruması yolları yalnız bileşen testleriyle kapsanır;
  DNF yolu erişilmez kalır.
- Panel'in "taşı, sonra kaldır" isteği, tarayıcı ve Agent RPC'leri sanal makine
  koşularının parçası değildi (bileşen ve bağlanmış testler kapsar).
- "Hazır değil · eksik eklentiler" kurulu paketleri okur; paketleri olup bir
  eklentiyi yüklemeyen bir PHP taramayla değil, taşımanın kanıtıyla bulunur.
- P0.1–P0.5 kısmi kalır; hiçbir P0 işi kapatılmaz ya da ilerletilmez.
