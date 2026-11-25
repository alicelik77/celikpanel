# v0.1.0-alpha.148

[English](RELEASE-NOTES-v0.1.0-alpha.148.md)

> **Yayımlandı**, 25 Kasım 2026 (UTC): sürüm commit'i `0c07d0281`, GitHub ön sürümü 21:34:14Z, indirme portalı yaklaşık 21:38Z (dış denetim 27/27).

**Bir güvenlik sürümü.** Bir müşteri sitesinin programları (yüklenen
programlar, Python uygulamaları, Node.js uygulaması, hizmet uygulamaları ve
bunların kurulum, derleme ve dağıtım adımları) artık kendi sınırlarının
içinde çalışıyor: hepsi birlikte yalnız sitesine verilen belleği, işlem
sayısını ve işlemci süresini kullanabilir; böylece bir sitenin programları
artık sunucunun tamamını öteki müşterilerden alamaz. Bir sitenin PHP'si
eskisi gibi site başına kendi sınırlarını korur ve sitenin sınırlarının
içinde değildir; zamanlanmış görevleri bu sürümde bu sınırların içinde
değildir; güncellemeden önce çalışan programlar yönetici siteyi taşıyınca
içeri girer. Panelin giriş çerezlerini
aynı alan adı altında başka bir ana makine adındaki bir web sitesi artık
koyamaz, gölgeleyemez ya da değiştiremez; "Farklı giriş yap"tan sonra çıkış
yapmak artık yöneticinin kendi girişini tarayıcıda bırakmaz. Panelin
veritabanı düzeni değişmez.
**Kurulu panelleri yalnız sahibi, panelin kendi güncelleme ekranından
günceller.**

## alpha.147'den güncelleme

1. Panelin kendi güncelleme ekranından, eskisi gibi güncelleyin.
2. **Panelin veritabanı düzeni değişmez** (şema 81 olarak kalır). Yeni panel
   açılmadan önce başarısız olan bir güncelleme kendiliğinden alpha.147'ye
   döner. Tamamlanmış bir güncelleme geri alınmaz; aşağıdaki "alpha.147'ye
   dönmek" bölümüne bakın.
3. **Güncelleme bir müşterinin hiçbir şeyini taşımaz ve yeniden başlatmaz.**
   Çalışan her program, yönetici siteyi taşıyana kadar çalıştığı yerde,
   sitesinin sınırlarının dışında çalışmayı sürdürür. Panel ilk açılışında bir
   sitenin varsayılan bellek sınırını bu sunucunun belleğine göre sabitler
   (aşağıya bakın). Yaklaşık beş dakika içinde **Şu anki sorunlar** yöneticiye
   tek bir satır gösterir: "Programları sınırlarının dışında çalışan siteler";
   böyle her sitenin Genel Bakış sayfası **Sınırların içine taşı** işlemini
   sunar. Bu taşıma yöneticinin adımıdır; site site, sitenin işine uygun bir
   zamanda yapılır: sitenin çalışan programlarını bir kez yeniden başlatır
   (bkz. "Güncellemeden sonra ne beklenir").
4. **Güncellemeyi kendi sayfasında izleyen yönetici oturumunu korur.**
   Güncelleme sırasında oturumu açık olan bayiler, müşteriler ve ekip
   üyeleri bir kez yeniden giriş yapar (bkz. "Güvenlik: başka sitelerin
   koyamayacağı giriş çerezleri").

## Güvenlik: bir sitenin programları kendi sınırlarının içinde

- **Şimdiye kadar bir sitenin programları yalnız tek tek sınırlanıyordu.**
  Bir müşterinin yüklediği programlar, Python uygulamaları, Node.js
  uygulaması ve hizmet uygulamaları (n8n gibi) ile bunların kurulum, derleme
  ve dağıtım adımlarının her birinin kendi sınırı vardı, ama hiçbir şey
  onları birlikte sınırlamıyordu. Birkaç programı olan ya da durmadan yeni
  program başlatan bir site sunucunun bütün belleğini, işlem sayısını ya da
  işlemci süresini kullanabiliyordu; öteki müşterilerin siteleri ve sunucunun
  kendisi beklemek zorunda kalıyor ya da bellek yetmediği için
  durduruluyordu.
- **Artık bir sitenin programları o sitenin kendi sınırlarında çalışır**:
  yüklenen programları, Python uygulamaları, Node.js uygulaması ve hizmet
  uygulamaları ile bunların kurulum, derleme ve dağıtım adımları yerel bir
  systemd diliminde, `celiksite_<site kimliği>.slice` içinde çalışır; hepsi
  için birlikte bir bellek sınırı (takas yok), bir işlem sınırı ve bir
  işlemci sınırı. Programlar bellek sınırına ulaşınca sistem içlerinden
  birini durdurur; işlem sınırında yeni işlem başlamaz, çalışanlar sürer;
  işlemci sınırında yavaşlatılırlar. Öteki siteler ve sunucu kendilerinin
  olanı korur. Bir programın kendi sınırları olduğu gibi kalır; sitenin
  sınırları dış sınırdır. Bir sitenin PHP-FPM havuzu eskisi gibi kendi PHP
  sınırlarını korur ve sitenin sınırlarının içinde değildir; zamanlanmış
  görevleri bu sürümde bu sınırların içinde değildir.
- **Bir program nereye gider.** Güncellemeden sonra dağıtılan, kurulan ya da
  derlenen bir program sitesinin sınırlarının içinde çalışır; her kurulum,
  derleme ya da dağıtım adımı da öyle. Güncellemeden önce durdurulmuş olup
  sayfasından yeniden başlatılan bir program, yeniden dağıtılana ya da site
  taşınana kadar bu sınırların dışında çalışır ve sitesi taşınacak siteler
  arasında sayılır. Çalışan bir program sıradan bir kaydetmede ya da yeniden
  başlatmada çalıştığı yerde kalır; böylece kimseden habersiz hiçbir şey
  yeniden başlatılmaz. Bir sitenin çalışan programlarını yöneticinin
  **Sınırların içine taşı** işlemi içeri alır. Sınırlarının içindeki bir
  program, sistem o an nerede çalıştığını söyleyemese de her kaydetmede
  içeride kalır.
- **Sınırlar ve onları kim koyar.** Üç plan anahtarı: bir sitenin
  programlarının birlikte belleği, işlem sayısı ve işlemcisi. Hiçbir ayar
  yoksa bir site 512 işlem, bir işlemci çekirdeğinin %200'ü ve bellek olarak
  3072 MB ile sunucu belleğinin %75'inden küçük olanını alır; böylece
  varsayılan olarak bir sitenin programları sunucunun bütün belleğini hiçbir
  zaman almaz. Bu bellek varsayılanı
  panelin ilk açılışında sabitlenir; sunucunun belleği sonradan değişirse
  **Şu anki sorunlar** bunu söyler ve yeni varsayılanı önizlemeyle sunar.
  Yönetici anahtarları planlarda, bayi planlarında, bir hesabın kendi
  sınırlarında ve sunucunun sınırlarında koyar; yönetici bir bayiye daha
  yüksek bir sınır vermedikçe bayi sunucunun varsayılanının içinde kalır.
- **Bir sınırı sitenin kullandığının altına düşürmek önce sorar.** Bir planın,
  bir bayi planının ya da bir hesabın kendi sınırlarının önizlemesi,
  programları yeni değerden fazlasını kullanan siteleri listeler ve ne
  olacağını söyler: bellek sınırını aşan bir program hemen durdurulur; işlem
  sınırında yeni işlemler reddedilir, çalışanlar sürer. Bunu sunucu uygular:
  böyle bir değişiklik ancak o liste onaylanırsa kaydedilir.
- **Veritabanı kullanıcıları.** İsteğe bağlı üç anahtar: en uzun veritabanı
  komutu (kullanıcının kendi oturumunun değiştirebildiği bir varsayılan),
  boşta açık işlemin en uzun süresi (PostgreSQL, bu da bir varsayılan) ve tek
  bir PostgreSQL oturumunun geçici dosyaları. Planın veritabanı süre
  sınırları, bir sitenin sınır konduğunda var olan MariaDB kullanıcılarına
  ulaşır; sonradan oluşturulan bir kullanıcı onları planın veritabanı
  sınırlarının bir sonraki değişikliğinde alır. PostgreSQL'de süre sınırları
  ve geçici dosya sınırı bu sürümde sitelerin veritabanı kullanıcılarına
  ulaşmaz.
- **İnsanların gördüğü.** Sitenin Genel Bakış sayfası sitenin sınırlarını,
  uygulanıp uygulanmadıklarını, programların şu an ne kullandığını ve sade
  sözlerle bir sınıra ne zaman ulaştıklarını gösterir; müşteri, sağlayıcısının
  sınırı yükseltebileceğini okur. Yönetici ayrıca her programın nerede
  çalıştığını ve taşımayı görür. **Şu anki sorunlar** yöneticiye taşınacak
  siteleri, sınırlarına tekrar tekrar ulaşan bir siteyi, henüz her siteye
  ulaşmamış bir sınır değişikliğini (panel o siteleri her saat yeniden yazar),
  kaydı saklanan bir taşımayı (bkz. "Bilinen sınırlar") ve sınırları
  tutamayan bir sunucuyu söyler.
- **Koşullar.** Sınırlar için sistemin birleşik denetim gruplarının (cgroup
  v2) bellek, işlem ve işlemci denetleyicileriyle birlikte olması gerekir;
  Debian 13 ve Ubuntu 24.04'te vardır. Bunlar olmayan bir sunucuda hiçbir şey
  sınırlanmaz ve **Şu anki sorunlar** neyin eksik olduğunu söyler. Sınırlar
  yerel systemd dosyalarıdır: panel durdurulsa, kaldırılsa ya da lisanssız
  kalsa da, yeniden başlatmalardan sonra da çalışmayı sürdürür. Sunucu
  sahibinin değiştirdiği bir dilim dosyası ve sahibin kendi eklemeleri
  (`/etc/systemd/system/celiksite_<site kimliği>.slice.d/*.conf`) site var
  oldukça hiç değiştirilmez; sitenin kartı onları adlarıyla gösterir. Site
  silinince ya da yeni bir site, dosyalarını CelikPanel'in bilmediği önceki
  bir sitenin kimliğini alınca, bunlar aynı klasörde systemd'nin yüklemediği
  adlarla kenara konur
  (`celiksite_<site kimliği>.slice.celikpanel-aside-<UTC zamanı>` ve
  `celiksite_<site kimliği>.slice.d.celikpanel-aside-<UTC zamanı>`); silme
  işlemi onları etkinlik listesinde adlarıyla gösterir.

## Güvenlik: başka sitelerin koyamayacağı giriş çerezleri

- **Şimdiye kadar panelin giriş çerezlerinin adları sıradandı.** Tarayıcı,
  aynı alan adı altında başka bir ana makine adındaki bir web sitesinin
  bütün alan adı için koyduğu bir çerezi de panele gönderir (örneğin panel
  `panel.example.com` adresindeyken `shop.example.com` adresindeki bir
  site). Böyle bir web sitesi, bir ziyaretçinin tarayıcısına kendi seçtiği
  bir girişi koyabiliyordu; panel bunu kendi girişinden önce okuyordu ve
  ziyaretçi farkına varmadan başka bir hesapta çalışıyordu. Ayrıca **Hesabıma
  dön** düğmesinin arkasına bir giriş koyabiliyordu.
- **Artık panelin giriş çerezleri, tarayıcının yalnız panelin kendi ana
  makine adı için sakladığı adları taşır**; yalnız HTTPS üzerinden ve hiçbir
  zaman bütün bir alan adı için değil (`__Host-celikpanel_session` ve "Farklı
  giriş yap" sırasında `__Host-celikpanel_impersonator`). Aynı alan adı
  altında başka bir ana makine adındaki bir web sitesi panelin girişini artık
  koyamaz, gölgeleyemez ya da değiştiremez. Bir tarayıcı girişi yine de iki
  kez gönderirse panel ikisini de kullanmaz ve giriş sayfası nedenini
  söyler. Başka bir web sitesinin bütün alan adı için koyduğu bir çerez
  tarayıcıda kalır: kişi yeniden giriş yaptıktan sonra panel onu yok sayar,
  çıkış yaptıktan sonra giriş sayfası onu yeniden anabilir. Oturumu açık bir
  tarayıcıdan panele gelen ve
  başka bir web sitesinden ya da aynı sitenin başka bir adresinden çıkan
  istek reddedilir; panelin kendi sayfaları hiçbir zaman böyle istek
  göndermez.
- **"Farklı giriş yap", sonra "Çıkış yap".** Şimdiye kadar "Farklı giriş
  yap" oturumundan çıkış yapmak yalnız o oturumu kapatıyordu: tarayıcı
  yöneticinin kendi girişini **Hesabıma dön** için saklıyordu ve o
  tarayıcıda, tarayıcı kapanmadan sonra giriş yapan kişi **Hesabıma dön**
  düğmesine basıp yönetici olarak devam edebiliyordu. Artık "Farklı giriş
  yap" sırasında **Çıkış yap** yöneticinin o tarayıcıdaki kendi oturumunu da
  kapatır; **Hesabıma dön** yalnız aynı yöneticinin bir "Farklı giriş yap"
  oturumunda sunulur ve çalışır; her giriş, tarayıcıda kalmış bir yönetici
  girişini siler.
- **Bir "Farklı giriş yap" oturumu başka bir yolla sona erdiğinde**
  (oturumlar listesinden kapatıldığında, hesap askıya alındığında ya da
  silindiğinde, oturum uzun süre kullanılmadığında ya da yönetici hesabı
  artık yönetemediğinde) tarayıcı yöneticinin kendi girişini artık saklamaz
  ve giriş sayfası açılır; şimdiye kadar panel orada her isteği, **Çıkış
  yap** dahil, reddediyordu. Oturum oturumlar listesinden ya da hesap askıya
  alınarak veya silinerek kapatıldıysa giriş sayfası genel "oturumunuz sona
  erdi" metnini gösterir.
- **Veritabanı araçları artık panelin girişini almaz.** Panelin kendi
  adresi üzerinden bir veritabanı aracına ilettiği hiçbir şey panelin giriş
  çerezlerini ya da bir `Authorization` başlığını taşımaz; bir araç panelin
  adlarıyla çerez koyamaz ve panelin çerezlerini silemez.
- **İnsanların fark ettiği.** Güncelleme sırasında oturumu açık olan yönetici
  oturumunu korur: güncelleme sayfası güncellemeyi eskisi gibi kendiliğinden
  sonuna kadar izler. Güncelleme sırasında oturumu açık olan bayiler,
  müşteriler ve ekip üyeleri ile o sırada "Farklı giriş yap" içinde olan bir
  yönetici bir kez yeniden giriş yapar; güncellemeden 24 saatten sonra geri
  gelen herkes de öyle. Güncelleme sırasında bir panel sayfası açık olan
  kişi genel "oturumunuz sona erdi" metnini görür. Giriş sayfası nedeni ve ne
  yapılacağını şu durumlarda söyler: bir sayfa güncellemeden önceki bir
  girişle yeni açıldığında, tarayıcı girişi iki kez gönderdiğinde ve
  tarayıcı az önce verilen girişi saklamadığında (örneğin panelin önündeki
  bir vekil sunucu çerezlerini değiştirdiği için).
- **Panel için tek bir ana makine adı.** Panelin kendi ana makine adındaki
  bütün hizmetler, bağlantı noktası ne olursa olsun, onun çerezlerini
  paylaşır; bu nedenle bu ana makine adı yalnız panel için kullanılmalıdır.

## alpha.147'ye dönmek

- **Başarısız olan bir güncelleme kendiliğinden alpha.147'ye döner.** Yeni
  panelin açılış denetimi yeni panel başlatılmadan önce çalışır; bu denetim
  başarısız olursa ya da güncelleme o açılıştan önce herhangi bir yerde
  durursa, güncelleme alpha.147'yi ve veritabanını kendisi geri koyar. O
  sırada bu sürümün değişikliklerinin hiçbiri yoktur: veritabanı adımı yoktur,
  güncelleme hiçbir programın birimini yeniden yazmaz ve panelin ajanı bir
  sitenin sınırlarını ve kendi yardımcısını ancak açıldıktan sonra yazar.
- **Tamamlanmış bir güncelleme geri alınmaz.** Yeni panel başlatıldığı andan
  itibaren güncelleme, bu açılış başarısız olsa da sonuna kadar götürülür.
  Panel önceki sürüme dönüş sunmaz; sürümün `rollback.sh` dosyası da
  tamamlanmış bir güncellemeyi hiçbir şeyi durdurmadan reddeder ve şunu
  yazar:
  `!! a new rollback cannot reuse an earlier transaction's recovery material; preserve the snapshot and use a supported recovery plan`.
  Güncelleme kendi yedeğini saklar. `sudo /usr/libexec/celikpanel/recovery recover`
  yarıda kalmış bir güncellemeyi ya da geri dönüşü tamamlar; tamamlanmış bir
  güncellemeden sonra yapacağı bir şey yoktur.
- **Bu sürümün yaptıkları alpha.148'de geri alınır:**
  - Bir sitenin sınırları: planda ya da hesapta bir anahtarı boşaltmak siteyi
    varsayılana döndürür. Onları panel olmadan kaldırmak ya da değiştirmek
    için sunucu sahibi kendi dosyasını ekler, örneğin
    `/etc/systemd/system/celiksite_<site kimliği>.slice.d/owner.conf`
    içinde `[Slice]` ve `MemoryMax=infinity`, sonra
    `sudo systemctl daemon-reload` çalıştırır. `sudo systemctl daemon-reload`
    sonrasında sahibin değişikliği çalışan programlara Ubuntu 24.04'te hemen,
    Debian 13'te hiçbir program başlamadan birkaç saniye ile yaklaşık bir
    buçuk dakika arasında ulaşır; bir programın başlaması onu hemen uygular.
  - Sitesinin sınırlarının içindeki bir program: panelde onu dışarı çıkaran
    bir işlem yoktur. Sunucu sahibi programın biriminin yanına kendi
    dosyasını ekler; Debian 13'te ve Ubuntu 24.04'te aynıdır:
    `/etc/systemd/system/<birim>.service.d/slice.conf` içinde iki satır,
    `[Service]` ve `Slice=system.slice`, sonra
    `sudo systemctl daemon-reload` ve `sudo systemctl restart <birim>`
    (`<birim>` programın kartındaki adıdır, örneğin `celikrt-12`). Panel
    programı yönetmeyi sürdürür: sitenin kartı yöneticiye programı sunucu
    sahibinin kendi eklemesiyle yerleştirdiğini söyler, taşıma onu olduğu
    yerde bırakır, panelden kaydetmek ya da güncellemek o dosyayı hiçbir
    zaman değiştirmez. Dosyayı silip aynı iki komutu çalıştırmak programı
    yeniden içeri alır. Bunun için CelikPanel'in kendi birim dosyasını
    değiştirmeyin: değişen bir birim dosyası sunucu sahibininki olarak
    korunur ve panel o programın birimini artık yazmaz (bir hizmet
    uygulamasının bir sonraki güncellemesi, değişiklik böyle bir dosyaya
    taşınana kadar reddedilir).
  - Bir veritabanı kullanıcısının sınırları: anahtarı boşaltmak panelin kendi
    yazdığı değeri kaldırmasını sağlar.
- **Giriş çerezlerinde geri alınacak bir şey yoktur.** Kendiliğinden
  dönüşte o bölümden hiçbir şey yoktur: yeni çerez adlarını ve onları ilk ne
  zaman kullandığını yalnız açılmış yeni panel koyar ve kaydeder. Elle geri
  alınacak bir şey yoktur. Önceki bir sürümün paneli yeni adları okumaz;
  orada insanlar yalnızca yeniden giriş yapar.

## Bilinen sınırlar

- **Tamamlanmış bir güncelleme geri alınmaz** (bkz. "alpha.147'ye dönmek");
  bu sürümün değişiklikleri alpha.148'de geri alınır.
- **Güncellemeden önce çalışan programlar, yönetici siteyi taşıyana kadar
  sitesinin sınırlarının dışında kalır;** böyle bir program taşınana kadar
  sunucunun her yeniden başlatılmasından sonra yine dışarıda başlar.
  Güncellemeden önce durdurulmuş olup sayfasından yeniden başlatılan bir
  program da, yeniden dağıtılana ya da site taşınana kadar öyledir.
- **Bir sitenin sınırlarının dışında kalanlar:** PHP'si (kendi PHP
  sınırlarını koruyan, her sitenin PHP-FPM havuzu), zamanlanmış görevleri ve
  bir site hesabının açık tuttuğu kullanıcı yöneticileri.
- **Taşıma sitenin çalışan her programını bir kez yeniden başlatır** ve onun
  başlama süresi içinde (program başkasını koymadıkça 90 saniye, en az 20, en
  çok 300) sınırların içinde çalışıp yeniden yanıt vermesini bekler; vermeyen
  program önceki dosyasıyla çalıştığı yere geri konur ve orada başlatılır.
  Bir seferde tek bir site taşınır, bir taşımada en çok 64 program; bir site
  taşınırken o sitenin değişiklikleri bekler ya da "değiştiriliyor" yanıtını
  alır. Dosyasını sunucu sahibinin değiştirdiği programlar, önceki bir
  korumadaki programlar ve sahibin bir dosyasının başka yere koyduğu
  programlar taşınmaz; kart onları ve kimin ne yapacağını söyler.
- **Geri konan bir program çalıştığı yerde de başlayamazsa,** taşımanın
  penceresi yine onun geri konduğunu söyler ve sitenin kartı yaklaşık yarım
  dakika boyunca programın sunucu sahibinin koyduğu yerde olduğunu
  söyleyebilir. Tamamlanmayan bir taşımadan sonra sitenin programlarının
  çalıştığını denetleyin. Taşıma kaydını saklar ve sitenin kartı **Tekrar
  dene** düğmesini yalnız sistem geri konan bir programı başlatmayı
  reddettiğinde ya da ajanın açılışında onu başlatmaya zaman kalmadığında
  sunar.
- **Panelin ajanının durmasıyla yarıda kalan bir taşıma,** ajan yeniden
  açılınca tamamlanır ya da geri konur. Ajanın açılışındaki geri koyma
  başlatmalarına hepsi için birlikte birkaç saniye verilir; sığmayan
  **Tekrar dene** için saklanır.
- **Bir site sınırlarının içine taşınırken panelin güncellemesini
  başlatmayın;** taşımanın penceresi bitene kadar bekleyin: panelin ajanı bir
  taşımanın ortasında durdurulunca açılması birkaç saniye daha uzun sürer ve
  bir güncelleme onu yalnız on iki saniye kadar bekler. Barındırılan siteler
  her iki durumda da etkilenmez.
- **Bir taşıma tamamlanmadığında penceresi sistemin nedenini İngilizce
  verebilir.**
- **Debian 13'te sahibin elle değiştirdiği bir dilim dosyası çalışan
  programlara hemen ulaşmayabilir** (panelin kendi değişiklikleri aynı adımda
  ulaşır). `sudo systemctl daemon-reload` sonrasında sahibin değişikliği
  çalışan programlara Ubuntu 24.04'te hemen, Debian 13'te hiçbir program
  başlamadan birkaç saniye ile yaklaşık bir buçuk dakika arasında ulaşır; bir
  programın başlaması onu hemen uygular.
- **Ubuntu 24.04'te sitenin kartı, sitenin programlarının işlem sınırına
  ulaştığını söylemeyebilir;** sınırın kendisi tutar.
- **Panelin ajanı bir sınır değişikliğini kaydedemezse Şu anki sorunlar bir
  saate kadar sitelerin yeni sınırlara sahip olmadığını söyleyebilir,**
  sahip olsalar bile.
- **Sunucunun kendi sınırlar ekranı**, düşürülen bir site sınırını
  önizlemez: sunucunun bir site anahtarındaki tavanını ya da varsayılanını
  düşürmek, durduracağı sitelerin listesi gösterilmeden uygulanır.
- **Planın veritabanı süre sınırları, bir sitenin sınır konduğunda var olan
  MariaDB kullanıcılarına ulaşır;** sonradan oluşturulan bir kullanıcı onları
  planın veritabanı sınırlarının bir sonraki değişikliğinde alır.
  PostgreSQL'de süre sınırları ve geçici dosya sınırı bu sürümde sitelerin
  veritabanı kullanıcılarına ulaşmaz.
- **En uzun veritabanı komutu bir varsayılandır:** kullanıcının kendi oturumu
  onu değiştirebilir. MariaDB'de geçici dosya ya da boşta işlem anahtarı
  yoktur.
- **Disk sınırları sayılır ama dosya sistemince uygulanmaz**; barındırma
  dosya sisteminde proje kotası yoksa panel sayar ve davranır (yüklemeler
  durur), dosya sistemi bir yazmayı kendisi reddetmez. **Şu anki sorunlar**
  bunu bir kez söyler.
- **Panelin kendi ana makine adını her bağlantı noktası paylaşır.** Bu ana
  makine adındaki bir web sitesi ya da hizmet, hangi bağlantı noktasında
  olursa olsun, panelin giriş çerezlerini alır ve onları koyabilir; yeni
  adlar bağlantı noktalarını ayırmaz. Bu ana makine adı yalnız panel için
  kullanılmalıdır.
- **Güncellemeden sonraki 24 saat boyunca** bir yöneticinin güncellemeden
  önceki girişi önceki çerez adıyla da kabul edilir ve yeni ada taşınır;
  güncellemeden önceki öteki her giriş yeniden giriş yapmaya çağrılır.
- **Bir "Farklı giriş yap" oturumu oturumlar listesinden ya da hesap askıya
  alınarak veya silinerek kapatıldığında** tarayıcı yöneticinin kendi
  girişini artık saklamaz, ama bu giriş kendiliğinden sona erene kadar
  (yönetici giriş yaptıktan en çok 24 saat sonra) sunucuda geçerli kalır;
  onu artık hiçbir tarayıcı tutmaz.
- **Aynı alan adı altındaki bir web sitesi tarayıcının o alan adı için
  çerezlerini yine doldurabilir** (bu, panele giden istekleri fazla büyütebilir
  ya da çerezleri dışarı itebilir); panelin girişini artık koyamaz ya da
  değiştiremez.
- **Güncelleme sırasında panel sayfası açık olan bayiler, müşteriler ve ekip
  üyeleri nedeni değil, genel "oturumunuz sona erdi" metnini görür** ve bir
  kez yeniden giriş yapar. Hesap askıya alınarak veya silinerek ya da
  oturumlar listesinden kapatılan bir "Farklı giriş yap" oturumu ve uzun
  süre kullanılmadığı için kapanan bir oturum da genel metni gösterir.
- **Aynı alan adı altındaki başka bir web sitesinin koyduğu bir giriş çerezi
  tarayıcıda kalır;** panel onu yok sayar ve çıkış yaptıktan sonra giriş
  sayfası onu yeniden anabilir.
- alpha.147'nin açık listesi açık kalır
  ([alpha.147 sürüm notları](RELEASE-NOTES-v0.1.0-alpha.147.tr.md)).

## Laboratuvarda denenenler

Laboratuvar denemesi bitti: deneme sunucusu çiftinde, posta ve web
postası olan bir Debian 13 sunucusu ile bir Ubuntu 24.04 sunucusunda tek
bir çalışma (25 Kasım 2026, UTC 16:20'den temizlikle birlikte yaklaşık
18:20'ye kadar, ayrıca 18:55'ten 19:12'ye kısa bir ikinci deneme). İki
sunucu da alpha.147'nin koduyla başladı ve panelin kendi güncelleme
sayfasından bu sürümün koduna (`83d6e043c`, yalnız laboratuvarda kullanılan
v0.1.0-alpha.156 etiketiyle) güncellendi. Her sunucuda bir müşterinin
sitesi, panelin kendi sayfalarından dağıtılan bir deneme programı çalıştırdı
ve karşı tarafı müşterinin site hesabı oynadı. Laboratuvarın yaptığı
söylenenler dışında her şey panelin kendi sayfalarından yapıldı.
Laboratuvarın karşılaştıkları "Bilinen sınırlar"dadır.

- **İki sunucu da panelin kendi güncelleme sayfasından kendiliğinden
  güncellendi;** yönetici oturumu açık olarak güncellemeyi o sayfada izledi:
  Debian 13'te yaklaşık bir buçuk dakika, Ubuntu 24.04'te iki dakika. Giriş
  sayfası hiç çıkmadı; sonra tarayıcı girişi yalnız yeni adıyla tutuyordu.
  Şema adımı yok (81 olarak kalır); her sunucuda bir panel ve bir ajan;
  yeni başarısız hizmet yok. Bir müşterinin hiçbir şeyi yeniden
  başlatılmadı: her program işlemini ve yerini korudu, saniyede bir yapılan
  bir denetim sitelerin ve web postasının iki güncelleme boyunca yanıt
  verdiğini gördü.
- **Güncellemeden hemen sonra,** hiçbir taşımadan önce: iki sunucuda da her
  program çalıştığı yerde çalışıyordu; sitenin kartı yöneticiye sınırların
  henüz uygulanmadığını söyledi ve taşımayı sundu, müşteri bununla
  sunucunun yöneticisinin ilgilendiğini okudu; beş dakika içinde **Şu anki
  sorunlar** programları sınırlarının dışında çalışan siteleri listeledi.
  Bir plan değişikliği hiçbir şeyi yeniden başlatmadı.
- **Sınırların içine taşı,** iki sunucuda da sitenin kartından: adımlarıyla
  kilitli bir pencere, sonra "tamamlandı"; program sitesinin diliminde tam
  kartın değerleriyle çalıştı (256 MB, 64 işlem, bir çekirdeğin %50'si),
  posta engeli onunla gitti (içeriden 25 numaralı bağlantı noktası
  reddedildi) ve program Debian 13'te 1,2 saniye, Ubuntu 24.04'te 0,2 saniye
  durdu; bu sırada başka bir site yanıt vermeyi sürdürdü. Posta engelinin
  yardımcısı Ubuntu 24.04'te de programın kendi yerini, sistem diliminde ve
  sitenin diliminde okur.
- **Sınırlar müşterinin kendi programına karşı tutuyor,** iki sunucuda da:
  işlem başlatan bir döngü sitenin 64 işleminde durduruldu; 512 MB tutan bir
  programı sistem sitenin 256 MB'ı içinde durdurdu ve program kendiliğinden
  yeniden başladı (yaklaşık 6 ile 7 saniye durdu); meşgul dört iş parçacığı
  yaklaşık yarım çekirdekte tutuldu. Başka bir site ve panel bu süre boyunca
  her saniye yanıt verdi. Kart bellek sınırını iki sunucuda, işlem sınırını
  Debian 13'te, işlemci sınırını Ubuntu 24.04'te adlandırdı; müşteri,
  sağlayıcının sınırı yükseltebileceğini okudu. Bir müşteri ya da bir bayi
  olarak "Farklı giriş yap" içindeki yönetici kartın yalnız o hesabın
  görebileceği kısmını gördü (sınırlar, kullanım ve o hesaba öneriyle
  olaylar; programların nerede çalıştığının listesi yok, taşıma yok) ve
  taşıma o oturum için reddedildi.
- **Panel olmadan ve yeniden başlatmadan sonra:** panel ve ajanı
  durdurulmuşken (Ubuntu 24.04) program sınırlarının içinde kaldı ve işlem
  sınırı tuttu. Debian 13 sunucusu yeniden başlatılınca taşınmış program
  aynı sınırlar ve posta engeliyle diliminde geri geldi; taşınmamış bir
  program dışarıda geri geldi ve taşınacaklar arasında sayılmayı sürdürdü.
  Güvenlik duvarının kuralları boşaltıldıktan sonra diliminde olan bir
  programın posta engeli yaklaşık 12 saniye içinde geri geldi.
- **Taşınmış bir programı kaydetmek,** systemd tekrar tekrar yeniden
  yüklenip yeniden çalıştırılırken de, onu sitesinin sınırlarında tuttu;
  sonraki her başlangıç orada açıldı.
- **Sınırları değiştirmek:** birlikte değiştirilen bellek, işlem ve işlemci
  çalışan programa Debian 13'te 2 saniye içinde ulaştı; üstünde site
  olmayan daha düşük bir plan belleği Debian 13'te 2 saniye içinde, Ubuntu
  24.04'te hemen ulaştı ve kart her okumada çekirdeğin tuttuğunu gösterdi.
  Sunucu, bir sitenin kullandığının altına inen ve onaylanmış liste olmadan
  gönderilen bir değişikliği iki sunucuda da reddetti.
- **Bir sınırı sitenin kullandığının altına, planın sayfasından düşürmek**
  (Debian 13): bir sitenin programları 213 MB kullanıyordu ve planın site
  belleği 128 MB yapıldı; sayfa siteyi kullandığı 213 MB ve yeni 128 MB
  sınırıyla listeledi ve onay istedi. Onaydan sonra kayıt kabul edildi, çekirdek
  yeni sınırı hemen aldı, sınırı aşan program durduruldu ve yaklaşık 8
  saniye sonra sitenin sınırlarının içinde kendiliğinden yeniden başladı.
- **Panelin ajanı bir sınır değişikliğini kaydedemediğinde** (Debian 13)
  çekirdek yeni sınırı yine aldı ve kart onu gösterdi; **Şu anki sorunlar**
  sitelerin onu henüz almadığını söyledi.
- **Sunucu sahibinin kendi komutları, iki sunucuda da:** `MemoryMax=infinity`
  içeren `owner.conf` ve `sudo systemctl daemon-reload` bellek sınırını hiçbir
  program başlamadan kaldırdı, Ubuntu 24.04'te hemen, Debian 13'te 15 saniye
  içinde; dosyayı silmek sınırı geri getirdi. "alpha.147'ye dönmek"
  bölümündeki `slice.conf` dosyası iki komutuyla taşınmış bir programı
  sitesinin sınırlarının dışına aldı: kart programı sunucu sahibinin
  yerleştirdiğini söyledi, panelden kaydetmek o dosyayı ve programın birim
  dosyasını olduğu gibi bıraktı, taşıma programı yeniden başlatmadan
  bıraktı; dosyayı silip aynı iki komutu çalıştırmak onu yeniden içeri aldı.
- **Tamamlanmayan bir taşıma** (Debian 13, laboratuvarın hiçbir yerde
  başlayamaz yaptığı bir program): pencere taşımanın tamamlanmadığını ve
  programın geri konduğunu söyledi; program çalışmıyordu. Böyle bir
  taşımanın 30. saniyesinde ajan yeniden başlatıldı: program geri kondu,
  taşıma çözüldü olarak bildirildi ve ajan 9 saniye sonra yanıt verdi.
- **Ajanın açılışı** (Debian 13, yeniden başlatılmasından yanıt vermesine
  kadar): her şey sağlıklıyken 3 ile 6 saniye; bir sitenin programı
  taşınmış olsun ya da olmasın başlayamamayı sürdürürken 5 ile 7 saniye;
  systemd o programdan vazgeçtikten sonra 2 ile 3 saniye; bir taşımanın
  ortasında yeniden başlatıldığında 9 saniye. Ubuntu 24.04'te başlayamayan
  bir programla 2 saniye. Tamamlanmayan bir taşımadan kısa süre sonra bir
  açılış 14 saniye sürdü; bu yeniden olmadı.
- **Giriş,** Debian 13'te: (1) güncelleme sırasında panel sayfası açık olan
  bir bayi ve bir müşteri genel "oturumunuz sona erdi" metnini, sonra yalın
  giriş formunu gördü ve hemen yeniden giriş yaptı; (2) bir müşteri olarak
  "Farklı giriş yap", sonra **Hesabıma dön** çalıştı; yeniden "Farklı giriş
  yap", sonra **Çıkış yap**: hiçbir yönetici girişi kalmadı, o tarayıcıda
  sonra giriş yapan müşterinin yöneticiye dönecek bir yolu yoktu ve
  yöneticinin önceki girişi tarayıcıya elle geri konunca reddedildi; (3)
  panelin alan adı altında başka bir ana makine adındaki bir müşteri web
  sitesi bütün alan adı için giriş çerezleri koydu, biri başka bir
  müşterinin geçerli girişini taşıyordu: tarayıcı yeni adı bütün alan adı
  için saklamadı, oturumu açık yönetici yönetici olarak kaldı, konulan giriş
  tek başına hiç kullanılmadı (giriş sayfası nedenini söyledi: iki kez
  gönderildi ya da güncellemeden önceden) ve web sitesinin panele isteği
  engellendi; (4) panel üzerinden ulaşılan veritabanı araçları panelin giriş
  çerezlerinin hiçbirini ve hiçbir `Authorization` başlığını almadı; (5)
  hesabı başka bir tarayıcıdan askıya alınan bir "Farklı giriş yap" oturumu
  genel metinle giriş sayfasına gitti ve hiçbir yönetici girişi bırakmadı;
  yönetici orada yeniden giriş yaptı.
- **Veritabanı sınırları** (Debian 13): planın bir saniyelik en uzun komut
  süresi sitenin var olan MariaDB kullanıcılarına ulaştı; hemen ardından
  oluşturulan bir kullanıcı onu almadı; sitenin PostgreSQL kullanıcıları süre
  sınırlarını almadı. Anahtarı boşaltmak panelin yazdığını kaldırdı.
- **Geri dönmek:** sürümün `rollback.sh` dosyası taşımalardan sonra Ubuntu
  24.04 sunucusunda "alpha.147'ye dönmek" bölümündeki satırla reddedildi;
  hiçbir şey durdurulmadı ya da değiştirilmedi.
- Güncellemeden sonra her iki sunucuda olağan sayfalar açıldı; bir posta
  kutusu web postasının kendi adresinde giriş yapıp bir ileti gönderdi.

**Laboratuvarın sayfanın yolundan ayrıldığı yerler.** Laboratuvar deneme
sunucularında root olarak deneme programını sitelerin klasörlerine kopyaladı
(sahibin bir yüklemesinin yapacağı gibi), Ubuntu 24.04 sunucusunda yüklenen
programları sundu, bir müşterinin sitesine çerez koyan bir sayfa koydu, bir
programı başlayamaz yaptı, panelin ajanını ve paneli yeniden başlattı ve
durdurdu, systemd'yi yeniden yükledi ve yeniden çalıştırdı, ajanın durum
klasörünü salt okunur yaptı, güvenlik duvarının kurallarını boşalttı, Debian
13 sunucusunu yeniden başlattı, sunucu sahibinin dosyalarını yazdı,
`rollback.sh` dosyasını çalıştırdı ve sonunda temizledi. Kendi sertifika
yetkilisine çalışma boyunca güvenildi.

**Gerçek sunucularda denenmeyenler:**

- Yeni panelin başarısız bir açılış denetimi üzerinden kendiliğinden dönüş.
- systemd yeniden yüklenirken tekrar tekrar yeniden başlatılan bir program:
  bir an bile posta engeli olmadan çalışıp çalışmadığı.
- alpha.147'nin yardımcısıyla sitesinin sınırlarının içindeki bir program ve
  bu sürümün bilmediği anahtarların plan sınırları.
- Panelin ajanının bir taşımanın her noktasında ya da diski doluyken
  durdurulması ve Ubuntu 24.04'te yarıda kalan bir taşıma.
- Ubuntu 24.04'te bir sınırı sitenin kullandığının altına sayfadan
  düşürmek.
- Bir programın kendi daha küçük sınırlarına ulaşması ve bütün sunucunun
  belleğinin yetmemesi.
- Dilim dosyasını sunucu sahibinin değiştirdiği bir siteyi silmek; yaklaşık
  1000 site; sunucunun belleğinin değişmesi; sitenin varsayılan belleğinde
  n8n.
- PostgreSQL'in geçici dosya sınırı, veritabanı sınırları altında geri
  yüklemeler ve yüklemeler ve sunucu sahibinin elle koyduğu bir veritabanı
  değeri.
- Oturumlar listesinden kapatılan bir "Farklı giriş yap" oturumu ve panelin
  çerezlerini değiştiren bir vekil sunucunun arkasındaki giriş sayfası.
- Arch Linux'ta hiçbir şey denenmedi.

## Güncellemeden sonra ne beklenir

- Şema adımı yok: düzen şema 81 olarak kalır. Tamamlanmış bir güncelleme geri
  alınmaz; bu sürümün değişiklikleri alpha.148'de geri alınır (bkz.
  "alpha.147'ye dönmek").
- **Bu sürüm, güncellemede ya da panelin ilk açılışında bir müşterinin hiçbir
  şeyini yeniden başlatmaz ve yeniden yüklemez:** ne bir programı, ne bir PHP
  havuzunu, ne web sunucusunu. Programlar çalıştıkları yerde, sitelerinin
  sınırlarının dışında çalışmayı sürdürür. (alpha.147'nin her açılıştan
  yaklaşık üç dakika sonraki adımı yine çalışır; işi yapılmışsa hiçbir şeyi
  taşımaz.)
- **Bir sınırı koymak ya da değiştirmek hiçbir şeyi yeniden başlatmaz.** Zaten
  sitelerinin sınırlarının içinde olan programlar yeni değeri yeniden
  başlatılmadan hemen alır. Bir sınırı yükseltmek hiçbir şeyi kesmez. Bir
  sınırı sitenin şu an kullandığının altına düşürmek, önizleme o siteyi
  listeledikten ve değişiklik onaylandıktan sonra, yeni bellek sınırını aşan
  bir programı hemen durdurur (laboratuvarda durdurulan program yaklaşık 8
  saniye sonra kendiliğinden yeniden başladı); düşürülen işlem sınırında
  yalnız yeni işlemler reddedilir.
- **Sınırların içine taşı, o sitenin çalışan her programını bir kez, birbiri
  ardına yeniden başlatır:** her program kendi yeniden başlatması boyunca
  çalışmaz; laboratuvarda başlayan bir program yaklaşık bir saniye durdu.
  Başlama süresi içinde sınırların içinde çalışmayan bir program çalıştığı
  yere geri konur ve orada başlatılır (bkz. "Bilinen sınırlar"). Sitenin
  PHP'sinden, havuzundan ya da web sunucusundan hiçbir şey yeniden
  başlatılmaz ya da yeniden yüklenmez. Anı seçmek için bir siteyi onun için
  sakin bir saatte taşıyın.
- Güncellemeden sonra dağıtılan, kurulan ya da derlenen bir program ve her
  kurulum, derleme ya da dağıtım adımı artık sitenin sınırlarının içinde
  çalışır: sitenin sahip olduğundan fazla bellek isteyen bir adımı
  (varsayılan olarak 3072 MB ya da sunucu belleğinin %75'i) sistem durdurur
  ve kart hangi sınıra takıldığını söyler. Güncellemeden önce durdurulmuş
  olup sayfasından yeniden başlatılan bir program, yeniden dağıtılana ya da
  site taşınana kadar bu sınırların dışında çalışır.
- **Şu anki sorunlar**, her site taşınana kadar yöneticiye "Programları
  sınırlarının dışında çalışan siteler" satırını gösterir.
- **Giriş.** Güncelleme sırasında oturumu açık olan yönetici oturumunu
  korur ve güncelleme sayfası güncellemenin sonunu kendiliğinden gösterir.
  O sırada oturumu açık olan bayiler, müşteriler ve ekip üyeleri ile "Farklı
  giriş yap" içinde olan bir yönetici bir kez yeniden giriş yapar:
  güncelleme sırasında açık olan bir panel sayfası genel "oturumunuz sona
  erdi" metnini gösterir, yeni açılan bir sayfa ise girişin güncellemeden
  önce açıldığı nedenini verir. Bu, bir müşterinin sitelerine, postasına ya
  da veritabanlarına dokunmaz.

## Kanıt

Şema değişikliği yok: `internal/db/migrations/` altında hiçbir dosya
değişmez; düzen 81 olarak kalır. Yeni ve ekleyici: ajan çağrıları
`SiteSlicesStatusV1`, `ApplySiteSlicesV1` ve `MoveSiteSliceV1`; yollar
`GET /api/v1/domains/{id}/site-limits`,
`POST /api/v1/domains/{id}/site-limits/move` (yönetici) ve
`POST /api/v1/rights/site-memory-default` (yönetici); plan anahtarları
`site.memory_mb`, `site.tasks`, `site.cpu_percent`, `db.statement_seconds`,
`db.idle_in_transaction_seconds`, `db.temp_file_mb`; denetim kaydı
işlemleri `site.limits_move` ve `site.limits_file_aside`; Şu anki sorunlar
türleri 39 ile 46 arası. Giriş çerezleri için: HTTPS üzerinden çerez
adları `__Host-celikpanel_session` ve `__Host-celikpanel_impersonator`;
bir kez yazılan tek bir panel ayarı, `auth.signin_cookie_names_since`;
yanıtlar `SIGNIN_COOKIES_DUPLICATE`, `SIGNIN_FROM_BEFORE_UPDATE` ve
`LOG_IN_AS_ENDED` (401) ile `CROSS_SITE_REQUEST_REFUSED` (403).

Bölüm bölüm testler:

- Bir sitenin sınırlarının nereden geldiği ve neyin onlara ulaşmak
  sayıldığı: `TestSliceCountersTellTheSitesLimitApart` (gerçek bir Debian 13
  ve Ubuntu 24.04 çekirdeğinin yazdığı sayaç dosyaları),
  `TestOnlyTheSitesOwnLimitBlamesThePlan`, `TestTheStatusReadsTheKernel`,
  `TestTheCardShowsWhatTheKernelHolds`, `TestTheMemoryDefaultIsBoundToTheHost`,
  `TestTheDefaultDoesNotMoveByItself`,
  `TestAResellersSiteLimitsAreBoundedByDefault`,
  `TestAResellerCannotSetASiteLimitAboveItsOwn`.
- Taşıma ve kaydı: `TestMoveSiteSliceMovesRunningUnitsOnce`,
  `TestAMoveSurvivesAnAgentStopAtEveryStep`,
  `TestTheMoveIsRecordedBeforeEveryWrite`,
  `TestAFullDiskAtAnyMoveRecordStopsTheMove`,
  `TestARecoveryThatCannotRestartKeepsItsRecord`,
  `TestAnUnknownPlaceOrAFailedReloadRestartsNothing`,
  `TestTheMovesRestartIsBoundedByItsStartWait`,
  `TestAMoveTakesEachProgramsLockAndLosesNothing`,
  `TestAMoveReadsDropInsAndWaitsAsTheUnitDoes`,
  `TestAKeptMoveIsOneLineAndCanBeTriedAgain`,
  `TestAKeptMoveNeverHoldsUpTheAgentsStart`.
- systemd bir programın nerede çalıştığını söyleyemezken kaydedilen
  programın gittiği yer:
  `TestARuntimeAppSavedWhileItsPlaceIsUnknownKeepsItsLimits`,
  `TestANodeSiteSavedWhileItsPlaceIsUnknownKeepsItsLimits`,
  `TestAServiceAppSavedWhileItsPlaceIsUnknownKeepsItsLimits`.
- Sınırları uygulamak: `TestALoweredSiteLimitAsksFirst`,
  `TestALoweringWithoutTheListIsRefused`,
  `TestARunningSliceTakesTheNewLimitAtOnce`,
  `TestAPushThatDoesNotTakeIsAFailedOutcome`,
  `TestALimitChangeThatDidNotReachEverySiteIsALine`,
  `TestFiveThousandSitesAreReadAndWritten`,
  `TestNothingIsWrittenOnAHostWithoutLimits`, `TestADeletedSitesSliceGoes`.
- Posta engeli ve bir programın yeri: `TestEgressPlaceOrder`,
  `TestEgressFollowsTheSitesSlice`, `TestProofAcceptsTheSitesSlice`,
  `TestTheAgentsPlaceReadingIsTheHelpers`,
  `TestGuardReaddsUnitsInTheirSitesSlice`.
- Veritabanı kullanıcıları: `TestAPostgreSQLLoadNeverLiftsTheTemporaryFileLimit`,
  `TestTheCustomersPostgreSQLImportFollowsTheSameRule`,
  `TestADatabaseUsersOwnSettingsSurvive`, motor testleri
  `TestR2aPostgreSQLTemporaryFileLimitHoldsDuringARestore` ve
  `TestR2aMariaDBStatementTimeDoesNotCutARestore`; web
  `site-limits-r2a.test.mjs`, `site-limits-round2.test.mjs`,
  `site-limits-round3.test.mjs`, `site-limits-a148.test.mjs`.
- Giriş çerezleri: `TestSigninCookiesUseHostPrefixOverHTTPS`,
  `TestSigninDuplicateSessionCookiesAreRefusedAndCleared`,
  `TestSigninPlainCookieBesideTheNewNameIsIgnoredAndCleared`,
  `TestSigninFetchMetadataGate`, `TestCSRFBlocksOriginThatDiffersOnlyByPort`.
- Güncellemedeki giriş: `TestSigninTransitionCarriesAPreUpdateAdministratorSessionOnce`,
  `TestSigninTransitionCarriesOverOnlyOnAPIAndNeverCached`,
  `TestSigninTransitionRefusals`,
  `TestSigninCookieNamesSinceIsRecordedOnceAndKeptAcrossRestarts`.
- "Farklı giriş yap": `TestSigninLogInAsAndReturnUseTheNewNames`,
  `TestSigninLogInAsThenSignOutLeavesNoWayToTheOperator`,
  `TestSigninReturnCookieBesideAnOrdinarySessionLeadsNowhere`,
  `TestSigninLogInAsEndedAnyOtherWayLeavesNoUsableTokenInTheBrowser`,
  `TestSigninRefusalsAndTheReturnCookie`.
- Veritabanı araçları: `TestDBToolProxyStripsPanelCredentialsBothWays`,
  `TestDBToolProxyKeepsThePanelsSecurityHeadersOverPlainHTTP`; giriş
  sayfasının nedenleri: web `access-hold-runtime.test.mjs`.
