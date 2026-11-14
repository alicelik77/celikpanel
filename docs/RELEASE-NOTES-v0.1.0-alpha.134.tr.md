# v0.1.0-alpha.134

[English](RELEASE-NOTES-v0.1.0-alpha.134.md)

> **Aday.** Henüz yayımlanmadı; laboratuvar denetimi henüz yapılmadı.

Üç işlik küçük bir sürüm. Sitenin sertifikası artık neyi koruduğunu
söylüyor ve yeni sertifika alan adının posta adını da kapsıyor; site
yedekleri artık sitenin programını, Python uygulamasını ve hizmet
uygulamalarının verisini içeriyor; veritabanı hakları tam olarak tek bir
veritabanının adına veriliyor.
**Kurulu panelleri yalnız sahibi, panelin kendi güncelleme ekranından
günceller.**

## alpha.133'ten güncellerken

1. **Veritabanı hakları kendiliğinden daraltılır; basmanız gereken bir şey
   yok.** Güncelleme doğrulandıktan sonra CelikPanel, yönettiği her MariaDB
   sunucusunda daha önce verdiği veritabanı haklarını, her biri yalnız kendi
   veritabanına erişecek biçimde yeniden yazar. Ardından **Bileşenler →
   MariaDB** sayfasındaki **Veritabanı hakları tam adla** bölümü kaç hakkın
   daraltıldığını, bunlardan hangilerinin önceden başka veritabanlarına da
   eriştiğini (ve hangilerine), kaçının olduğu gibi bırakıldığını ve
   nedenini, kaçının hâlâ beklediğini söyler. Olduğu gibi bırakılan bir hak
   varsa yöneticiler ana sayfada **tek** bir bildirim görür.
   - **"Olduğu gibi bırakıldı"**, CelikPanel'in o hakkı kendisinin verdiğini
     kanıtlayamadığı (elle açılmış bir hesap ya da CelikPanel'in artık
     kaydını tutmadığı bir veritabanı) ya da hakkın sunucu sahibine ait
     olduğu (başkasına hak verme yetkisi ya da CelikPanel'in hiç vermediği
     bir yetki taşıdığı) anlamına gelir; CelikPanel ona dokunmaz. Bölüm her
     birini şu an eriştiği veritabanlarıyla listeler; **Ayrıntılar** altında
     onu tek bir veritabanına daraltan ya da kaldıran komut yazar. Bir hesap
     yalnız kendi veritabanına erişmeliyse o komutu çalıştırın; bilerek
     birden çok veritabanına erişiyorsa olduğu gibi bırakın.
   - **Daraltılan bir kullanıcı, adı eski hakkına rastlantıyla uyan başka
     veritabanlarına artık erişemez.** Bir uygulama buna dayanıyorsa hakkı
     bilerek verin (bölüm, daraltılan her hakkın önceden eriştiği
     veritabanlarını adlarıyla gösterir).
2. Güncellemeyi her zamanki gibi panelin kendi güncelleme ekranından
   başlatın. Güncelleme panelin kendi veritabanına her alan adı için yeni bir
   ayar ve daraltılmadan önce her veritabanı hakkının bir kaydını ekler.
   Güncelleme başarısız olursa ya da sonradan alpha.133'e dönerseniz, panelin
   veritabanı güncellemeden hemen önce alınan kopyasına geri döner: alpha.133
   bu yeni kayıtları okuyamaz. Daraltılmış haklar veritabanı sunucusunda
   daraltılmış olarak kalır ve çalışmayı sürdürür (bkz. Bilinen sınırlar).
3. **Güncellemeden sonra program, Python uygulaması ya da hizmet uygulaması
   çalıştıran her sitenin yeni bir yedeğini alın** (alan adının
   **Yedekler** bölümünde dosya ya da tam yedek). Bu sürümden önce alınan
   yedekler programın klasörünü ve verisini içermez; yedek listesi bunu her
   birinin yanında söyler ("Bu yedek programı içermiyor (eski biçim)").
4. **Her alan adının sertifikasına bir kez bakın** (alan adının **Güvenlik**
   bölümü). Yeni "Neler korunuyor" listesi, sertifikanın neyi koruduğunu ad
   ad söyler. Posta adı eksikse orada "Korunmuyor" ve tek bir düğme görünür:
   **Posta adını da kapsa**. Bir basış, sertifikayı bu adla yeniden ister.
   Kendiliğinden hiçbir şey yeniden istenmez.
5. alpha.133'ten sonra yapmadıysanız: CelikPanel'in veritabanı hesabına her
   veritabanı sunucusunda bir kez yeni parola verin (**Bileşenler →
   MariaDB**, "CelikPanel'in bu hizmete bağlandığı hesap", **Diğer → Yeni
   parola ver…**; aynısı **Bileşenler → PostgreSQL** sayfasında). Nedeni
   alpha.133 notlarında yazıyor.

## Sizin için ne değişiyor

### Sertifika neyi koruduğunu söylüyor

- Sitenin sertifikası artık neyi koruduğunu söylüyor: sitenin ve postasının
  her adı, ne zamana kadar korunduğu ya da neden korunmadığı. **Neler
  korunuyor** listesi sertifikanın bilgilerinin hemen altında, ayarlarından
  ve düğmelerinden önce durur: web sitesi (`www` ile ve `www` olmadan) ve
  diğer adları, web postası, posta programlarına söylenen ad, posta adı
  `mail.<alan adı>` (MX kaydı ve elle yazılan ayarlar) ve posta
  programlarının kendilerini ayarlarken kullandığı adlar.
- **Korunmayan adlar en başta gelir**, nedenleri ve tek düğmeleriyle;
  ardından korunan adlar. Tek liste, hep açık.
- Korunan bir ad hangi sertifikayla ve ne zamana kadar korunduğunu söyler
  ("bu alan adının sertifikası, 30 Oca 2027 tarihine kadar" ya da sunucunun
  posta sertifikası). Denetlenemeyen bir ad "Bilinmiyor" der, asla
  "Korunmuyor" demez.
- **Yeni sertifika, `mail.<alan adı>` bu sunucuyu gösteriyorsa ve alan
  adının postası bu sunucudaysa bu adı da kapsar.** İstek formunda ad
  işaretli gelir; isterseniz dışarıda bırakabilirsiniz. Başka yeri gösteren,
  DNS'te adresi olmayan ya da denetlenemeyen bir ad istenmez; istek o ad
  olmadan sürer. Böylece `mail.<alan adı>` adına bağlanan posta programları
  uyarı yerine bu adın sertifikasını görür; sunucunun posta adıyla
  ayarlanmış programlar eskiden ne görüyorsa onu görür ve programlara
  söylenen ad değişmez.
- **Posta adını içermeyen mevcut bir sertifika eksiği ve tek basışlık
  çözümü gösterir:** **Posta adını da kapsa**, sertifikayı bu adla yeniden
  ister; ilerleme penceresinde gerçek adımlarıyla. Ad artık bu sunucuyu
  göstermiyorsa, hiçbir sertifika kuruluşuna sorulmadan reddedilir.
  Sertifika kuruluşu reddederse eski sertifika ve posta olduğu gibi kalır.
  Posta yeni sertifikayı kullanmaya başlayamazsa posta eski hâline döner ve
  liste **Postada da kullan** düğmesini gösterir.
- **Başka yeri gösteren bir ad, hangi DNS kaydını kimin değiştireceğini
  söyler:** yanında "Başka yeri gösteriyor" ya da "DNS'te adresi yok" yazar
  (bu bir hata değil, beklenen bir koşuldur) ve tek bir cümle, adın A
  kaydını bu sunucunun adresine nerede çevireceğinizi (bu sunucunun DNS
  kayıtlarında ya da alan adının DNS'inin yönetildiği yerde) ve ardından
  listede ne çıkacağını söyler. **Yeniden denetle** yalnızca genel DNS'e
  yeniden bakar, hiçbir şeyi değiştirmez.
- Posta ekranı da sunucu adının altında aynı şeyi söyler ("Bu adı güvenilir
  bir sertifika koruyor (…, … tarihine kadar)").
- Yenileme değişmedi: sertifikayı işletim sisteminin certbot'u yeniler;
  CelikPanel durdurulsa ya da kaldırılsa da. Yenilenen sertifika posta adını
  korur.

### Site yedekleri programı ve verisini içeriyor

- Site yedekleri artık sitenin programını, Python uygulamasını ve hizmet
  uygulamalarının verisini içeriyor. Dosya ya da tam yedek, web klasörünü
  eskisi gibi tutar; ayrıca bir programın ya da Python uygulamasının
  çalıştığı klasörü (programın kendisi, ayarları ve verisi) ve her hizmet
  uygulamasının verisini (örneğin n8n, Open WebUI) de tutar.
- Alan adının **Yedekler** bölümü, yedeği almadan önce neleri içereceğini ve
  her parçanın ne kadar yer tuttuğunu söyler ("Site dosyaları (12 MB),
  Program dosyaları ve verisi (1,1 GB)", tam yedekte de veritabanları);
  neyin neden dışarıda kaldığını da söyler: indirme önbellekleri ve yarım
  kalmış derleme klasörleri, hizmet uygulamalarının kurulu program dosyaları
  (panel bunları yeniden kurar), programın başlarken yeniden oluşturduğu
  özel dosyalar ve sitenin hesabına ait olmayan dosyalar (karar
  verebilmeniz için adlarıyla). 1 GB ve daha büyük bir yedek önce sorar;
  yedek diskinde yer olmayabilecekse bunu hiçbir şey başlamadan söyler.
- Listedeki her yedek neleri içerdiğini ve programı içermeyen eski biçimde
  alınıp alınmadığını söyler.
- **Programı içeren bir yedeği geri yüklemek**, çalışan programı durdurur,
  klasörünü yerine koyar ve programı yeniden başlatır; sonuç programın
  başlayıp yanıt verdiğini ya da neden başlamadığını ve **Programlar**
  sekmesinde ne yapılacağını söyler. Sizin durdurduğunuz bir program durmuş
  kalır. Dosyaları yerine koymak başarısız olursa önceki dosyalar geri konur.
- Site yedeğinin geri yüklenmesi programın dosyalarını sitenin kendi
  hesabına ve kendi grubuna verir; web sunucusunun grubu programın
  klasörüne ulaşmaz.
- Bu siteye ait olmayan, artık sunucuda bulunmayan, bozuk ya da diske
  sığmayan bir yedek artık "iç hata" yerine nedeni ve yapılacakla söylenir.
  Hiçbir şey geri yüklenmez ve site olduğu gibi kalır.
- Programın gizli ayarları hiçbir zaman yedeğe yazılmaz. Aynı sunucuda geri
  yüklerken sunucudaki ayarlar olduğu gibi kalır; yeni bir sunucuda gizli
  değerleri **Programlar** sekmesinden yeniden girin.
- Eski yedekler eskisi gibi geri yüklenir; program klasörü içermedikleri
  için program olduğu gibi kalır.

### Tek bir veritabanı için veritabanı hakları

- **Bu neden bir güvenlik açığıydı.** MariaDB, bir veritabanının tamamına
  verilen haktaki veritabanı adını bir kalıp olarak okur: `_` herhangi bir
  karakter yerine geçer. CelikPanel'in açtığı her adda `_` vardır; bu yüzden
  bir müşterinin veritabanına verilen hak, adı yalnız o noktada ayrılan başka
  bir müşterinin veritabanına da uyabiliyordu. alpha.133 laboratuvarında
  MariaDB 11.8.6 ve 10.11.14 üzerinde ölçüldü: hakkı `labd_1__wp` adını
  taşıyan bir kullanıcı `labd_12_wp` veritabanında tablo açabildi.
- **CelikPanel'in verdiği her hak artık tam olarak tek bir veritabanı adına
  yazılır** (veritabanı ya da kullanıcı açarken, bir kullanıcıyı başka bir
  veritabanına verirken, dışarıdan bağlantı kopyalarında) ve yazıldıktan
  sonra sunucudan geri okunur: saklanan hak tam olarak o veritabanını
  anmalıdır.
- **CelikPanel'in önceden verdiği haklar, güncelleme doğrulandıktan sonra
  kendiliğinden daraltılır** (yukarıdaki güncelleme adımlarına bakın).
  CelikPanel her hak için önce onu olduğu gibi kaydeder, sonra aynı
  yetkileri tam adla verir ve denetler, ardından eski kalıp hakkını kaldırır
  ve yeniden denetler; böylece arada erişim hiç kesilmez, yarıda kalan bir
  tur sunucunun durumundan devam eder. Yalnız CelikPanel'in birlikte
  kaydettiği kullanıcı ve veritabanı ele alınır; kendisine ait olduğunu
  kanıtlayamadığı haklar listelenir, asla değiştirilmez. Panelin saatlik
  turunda çalışır; açılıştan sonraki ilk dakikalarda ve sunucu meşgulken
  çalışmaz; çalışabilene dek sayfa beklediğini ve nedenini söyler. Üç kez
  başarısız olan bir hak bir gün bekler ve elle daraltma komutuyla
  listelenir.
- **Bir veritabanını silmek, onu anan her hakkı her hesaptan ve rolden
  kaldırır;** herhangi bir adla elle açılmış hesaplardan da. Hesaplar ve
  roller kalır. **alpha.133'ten farkı:** silinen bir veritabanını anan
  haklar artık aynı adlı bir rolden de kaldırılır. Adı yalnız kalıp olarak
  karşılayan ve var olan başka bir veritabanına da erişen bir hak, silinen
  veritabanı için kaydı olmayan bir hesapta bırakılır ve yanıt onu
  listeler.
- **Veritabanı adı, tam adlı hakkı sığacak biçimde sınırlanır.** MariaDB bir
  haktaki veritabanı adını en çok 64 karakter olarak saklar; tam biçimde
  her `_` iki karakter sayılır. MariaDB 11.8.6 üzerinde ölçüldü: daha uzun
  bir tam ada verilen hak kabul edilir ve tam gösterilir, ama 64 karaktere
  kesilerek saklanır; bir sonraki yeniden yüklemeden sonra bu, başka ve
  daha kısa bir ada verilmiş bir haktır. Bu yüzden tam adı sığmayacak yeni
  bir veritabanı, hiçbir şey açılmadan, adın ne kadar kısalması gerektiği
  söylenerek reddedilir. CelikPanel'in kendi verdiği adlar (uygulama ve
  WordPress kurulumları, kopyalar) sığana dek kısaltılır. cPanel'den içe
  aktarma böyle veritabanlarını dışarıda bırakır ve adlarını söyler. Böyle
  bir adla zaten var olan bir veritabanı her zamanki kalıp biçimiyle
  çalışmayı sürdürür ve listelenir.
- **Veritabanı oluştururken veritabanı ya da kullanıcı adı motor için fazla
  uzun olacak bir ad artık hiçbir şey yapılmadan reddediliyor**
  (PostgreSQL'de iç hata çıkıyordu); form gerçek tam adı gösteriyor ve
  böyle bir adı yazarken durduruyor.
- **Başarısız olan bir veritabanı oluşturma, RELOAD yetkisi olmayan bir
  MariaDB hesabında artık kullanıcısını geride bırakmıyor;** temizlik
  bitemezse panel geride neyin kaldığını adıyla söylüyor.
- Hizmet sayfaları ve giriş sayfasındaki demo hesapları artık açıldıklarında
  yüklenir; böylece panelin ilk açılışı boyut sınırının içinde kalır.

## Bilinen sınırlar

### Sertifikalar

- **Yeni sertifika istekleri gerçek bir sertifika kuruluşuna karşı
  denenmedi.** Bileşen testleriyle denetlendi; deneme amaçlı bir sertifika
  kuruluşuyla yapılacak laboratuvar denetimi henüz yapılmadı.
- Sertifika alındıktan sonra bu sunucuyu göstermeye başlayan bir ad (yeni
  bir diğer ad ya da sonradan açılan `mail.<alan adı>`) "Korunmuyor" ve
  **Bu adla yeniden al** düğmesiyle gösterilir; kendiliğinden eklenmez.
- Bütün alt adları kapsayan bir sertifika (joker sertifika) yok.
- Sertifika isteği şu durumlarda hâlâ genel "iç hata" cümlesini gösterir:
  panel kendi kayıtlarını yazamazsa ya da sunucu isteği certbot çalışmadan
  önce reddederse (örneğin adın kanıtı için klasör hazırlanamazsa).

### Site yedekleri

- **Programların yedeklenmesi ve geri yüklenmesi yalnızca bileşen
  testleriyle denetlendi;** gerçek bir sunucuda henüz çalıştırılmadı.
- **Bir siteyi silip ardından yedeğini geri yüklemek henüz çalışmıyor.**
  Bir yedek kendi sitesine ve aboneliğine bağlıdır; bu yüzden panelden yeni
  bir siteye geri yüklenemez. Bu, sonraki bir sürümde gelecek. Yedeğine
  ihtiyaç duyacağınız bir siteyi silecekseniz önce yedeği indirin (alan
  adının **Yedekler** bölümü). Yeni sitede elle geri koymak için: paketi
  açın (`tar -xzf <yedek>.cpbak -C restore`); `restore/payload/files.tar.gz`
  içindeki site dosyalarını yeni sitenin web klasörüne açın;
  `restore/payload/home.tar.gz` dosyasını yeni sitenin ev klasörüne
  (`public_html` klasörünü içeren klasör) açın ve dosyaları sitenin
  hesabına ve o hesabın kendi grubuna verin; ev klasörünün grubunu asla
  vermeyin, o grup web sunucusunundur
  (`chown -hR "$(stat -c %U <site ev klasörü>):" <site ev klasörü>/app`;
  iki nokta hesabın kendi grubunu alır); her veritabanını
  `restore/payload/databases/` içinden
  yükleyin. Örnekli adımların tamamı docs/REMOTE-BACKUPS.tr.md belgesinde,
  "CelikPanel olmadan geri yükleme" bölümünün 4–6. adımlarında. Panelden
  indirilen yedek şifreli değildir; 1–3. adımlar ona uygulanmaz.
- **Hizmet uygulamasının verisi yedekte durur, ancak panel bu sürümde onu
  geri yüklemez;** geri yükleme sonucu bu uygulamaların adını söyler. Elle:
  uygulamayı durdurun, `payload/home.tar.gz` içinden `apps/<uygulama>/data`
  klasörünü alın, uygulamayı yeniden başlatın.
- Sitenin ev klasöründe web klasörü, program klasörü ve hizmet uygulaması
  klasörleri dışında doğrudan duran dosyalar yedeğe girmez.
- Sunucunun kendisi kaybedilirse programın gizli değerleri, hizmet
  uygulamalarının anahtarları ve n8n'in şifreleme anahtarı yeniden
  girilmelidir (bunlar hiçbir zaman yedekte olmaz).
- Geri yükleme sırasında panel, program durdurulduktan sonra ve yeniden
  başlatılmadan önce durursa program durmuş kalır; onu **Programlar**
  sekmesinden başlatın. Geri yüklemenin denetiminden hemen sonraki birkaç
  anda başlatılan bir program işi henüz bekletilmiyor.
- `node_modules` klasöründe bağlantı bulunan bir Node.js sitesi hâlâ
  yedeklenemiyor (değişmedi).
- Yedekler planın disk alanından sayılmaz (değişmedi).
- alpha.133'e dönüldükten sonra, alpha.134'ün aldığı ve program içeren
  yedekler orada listelenmez ve geri yüklenemez; sunucuda kalırlar ve
  alpha.134'e yeniden güncellenince yeniden görünürler.
- Geri yükleme şu durumlarda hâlâ genel "iç hata" cümlesini gösterir:
  sunucu denetim ya da geri yükleme sırasında bir dosyayı okuyamaz ya da
  yazamazsa (disk dolu olması dışında), program durdurulmadan yapılan bir
  geri yüklemenin ortasında sunucu yanıt vermeyi keserse ya da panel
  yedeğin veritabanlarıyla ilgili kendi kayıtlarını okuyamazsa.

### Veritabanı hakları

- **MySQL 8 sunucuları dönüştürülmez, yalnız listelenir.** Oradaki yeni
  haklar tam adlıdır, ama önceden verilen haklar yalnız daraltma komutuyla
  listelenir: MySQL 8'in bu hakları nasıl sakladığı ve
  gösterdiği doğrulanmadı.
- **Listelenen bir hakkı tek basışla daraltmak henüz yok.** **Bileşenler →
  MariaDB** sayfasında **Ayrıntılar** altındaki komutu kullanın.
- **Hesabı hak tablolarını okuyamayan, sonradan eklenmiş bir veritabanı
  sunucusunda haklar doğrulanamaz.** Oradaki haklar ad uzunluğu kuralına
  dayanır, hiçbir şey reddedilmez; MariaDB sayfası oradaki hakların
  doğrulanamadığını ve hesabın neye ihtiyacı olduğunu söyler (`mysql.db`
  üzerinde `SELECT`). Önceki hakların kendiliğinden daraltılması için
  hesabın, CelikPanel'in kendi hesabı gibi her hesabın haklarını okuyup
  değiştirebilmesi gerekir (`ALL PRIVILEGES ON *.* WITH GRANT OPTION`).
- **alpha.133'e dönüş** panelin veritabanını güncelleme öncesi hâline
  getirir. Daraltılmış haklar veritabanı sunucusunda kalır ve veritabanı
  kullanıcıları çalışmayı sürdürür: bir veritabanına eklenmiş bir
  kullanıcıyı çıkarmak orada da başarılı olur (laboratuvarda denetlendi) ve
  bu sunucunun MariaDB'sinde bir veritabanını silmek haklarını da kaldırır.
  alpha.133'te tam ad biçimine yalnız şunlar takılır: kullanıcısı
  çıkarılırken alpha.133'ün bıraktığı bir hesap (başka yerde hakları olan
  ya da CelikPanel'in açmadığı bir adresteki hesap) o veritabanındaki
  daraltılmış hakkını korur ve yanıt hesabın bırakıldığını söyler; API'nin
  hak kaldırma çağrısıyla (hiçbir ekran kullanmaz) bir hakkı kaldırmak
  "böyle bir hak yok" yanıtı alır ve hak yerinde kalır; böyle bir
  kullanıcının dışarıdan bağlantı kopyaları yenilenmez. Sonradan eklenmiş
  bir MariaDB sunucusunda alpha.133 yalnız veritabanını siler, onu anan
  haklar kalır (alpha.133'ün kendi davranışı). alpha.134'e yeniden güncellemek
  sunucunun durumundan devam eder.
- **Eski bir kusur sürüyor:** sonradan eklenmiş bir veritabanı sunucusu "bu
  işlem için yetkiniz yok" yanıtı verdiğinde panel bunu hâlâ "sunucu kimlik
  bilgisini reddetti" diye gösterir.
- Kullanıcının hesaplarını bırakan bir silmeden sonra, o hesaplar
  kaldırılana dek aynı adla veritabanı hâlâ açılamaz ve ret bunu henüz
  söylemez (alpha.135).
- **Motorun yanıt vermediği bir veritabanı oluşturma** (bağlantı koptu)
  kendiliğinden geri alınmaz: veritabanının ya da kullanıcısının açılıp
  açılmadığı bilinmez ve o adla yapılan sonraki deneme "ad kullanılıyor"
  diye reddedilir. Başka bir ad seçin; açılmış olanı sunucu yöneticisi
  veritabanı sunucusundan kaldırır.
- **Hesabında RELOAD yetkisi olmayan, sonradan eklenmiş bir MariaDB
  sunucusunda veritabanı oluşturma hâlâ tamamlanamıyor.** Artık geride hiçbir
  şey bırakmıyor.

### alpha.135'e taşınanlar

Alan adı silme ve geride bıraktıkları (veritabanları; postası ve
yedeklerinin 30 gün tutulması); bir veritabanının zaten silinmiş olduğunun
hatırlanması (ikinci basış arada aynı adla açılan veritabanını silmez);
CelikPanel'in veritabanı hesabının geride kalmış kopyalarının kendiliğinden
onarılması; bir veritabanı kullanıcısının adını neyin boşa çıkaracağını
söyleyen ret; PostgreSQL'de veritabanının kendi kullanıcısına ait
tablolardaki haklar; hesap kalırken verilen "kullanıcı kaldırıldı" sonucu;
her posta kutusu için web postası bağlantısı; gönderim sınırlarının daha
sade gösterimi; eş DNS sunucusundan kayıt eklemesini isteyen **Ekle**
düğmesi; Veritabanları listesinde boyutlar. alpha.133'ün bunlarla ilgili
bilinen sınırları sürüyor.

### Diğer

- Önceki sınırlar sürüyor (bkz. alpha.133); bu sürümün düzelttikleri
  dışında: site yedekleri artık programı içeriyor, sertifika ekranı posta
  adının kapsanıp kapsanmadığını söylüyor, MariaDB hakları tam adla
  veriliyor ve silinen bir veritabanının hakları her hesaptan ve rolden
  kaldırılıyor.

## Laboratuvarda denetlenenler

**Bu sürümün laboratuvar denetimi henüz yapılmadı.** Bu bölüm, denetimden
sonra neyin denetlendiği ve neyin denetlenmediğiyle doldurulur.

## Kanıt

Bu sürüm panelin veritabanına iki değişiklik ekler (77 → 79). 078: her alan adı
için, posta programlarına alan adının kendi posta adının mı söylendiğini
tutan yeni bir ayar. Daha önce postayı sertifikasıyla koruyan her alan
adında bu ayar açık başlar; böylece güncellemeden sonra hiçbir programa
başka bir ad söylenmez. 079: daraltılmadan önce her veritabanı hakkının kaydı
(hesap, adres, saklanan ad, yetkiler ve durumu) ve her veritabanı sunucusu
için turun durumu (haklarının doğrulanamadığı ve beklediği zamandan beri
bilgisi dâhil). İki değişiklik de yalnızca ekler; alpha.133 bunları içeren
bir veritabanını okumayı reddeder, bu yüzden alpha.133'e dönüş güncelleme
öncesi kopyanın geri yüklenmesidir. MariaDB'de verilen her hak, ortadan
kalkmış bir hesabı parolasız yeniden açmayacak bir oturum ayarıyla yazılır
ve geri okunur; geri okuması başarısız olan bir hak "bilinmiyor" sayılır,
hiçbir yanıta yorulmaz. Veritabanı hakları testleri geliştirme ortamında
gerçek bir MariaDB 11.8.6'ya karşı çalıştı (yukarıdaki ölçümler); MariaDB
10.11, sonradan eklenmiş ikinci bir sunucu ve iki müşterinin
veritabanlarıyla yapılan denetim laboratuvara kaldı. İki yeni salt-okur sorgu
eklendi: bir alan adının sertifikasının neyi koruduğu ve bir sitenin
yedeğinin neleri içereceği. Sertifika isteği eskisi gibi önce hazırlanır,
sonra devreye alınır; posta, alan adının sertifikasını yalnızca
`mail.<alan adı>` adı için sunar. Panel yokken yenileme işletim sisteminin
certbot'u ve kendi kancasıyla yapılır, değişmedi. Yedek paketinin içerik
listesi, yalnızca program ya da uygulama klasörü varsa yeni biçimde (sürüm
3) yazılır; yoksa paket eskisiyle aynıdır. Eski bir sürüm yeni biçimdeki
paketi hiçbir şey yazmadan reddeder. Bileşen testleri ve tarayıcı
bileşenlerinin testleri (listenin sırası dâhil) geçti; adları İngilizce
notta yazılı. Gerçek bir sertifika kuruluşu ve gerçek bir sunucu denemesi
yok. **Temel dayanıklılık işlerinden hiçbiri kapanmadı.** Sürüm sırası 133 →
134.
