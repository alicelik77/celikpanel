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

1. Güncellemeyi her zamanki gibi panelin kendi güncelleme ekranından
   başlatın. Güncelleme, panelin kendi veritabanına her alan adı için yeni
   bir ayar ekler. Güncelleme başarısız olursa ya da sonradan alpha.133'e
   dönerseniz, panelin veritabanı güncellemeden hemen önce alınan kopyasına
   geri döner: alpha.133 bu yeni ayarı okuyamaz.
2. **Güncellemeden sonra program, Python uygulaması ya da hizmet uygulaması
   çalıştıran her sitenin yeni bir yedeğini alın** (alan adının
   **Yedekler** bölümünde dosya ya da tam yedek). Bu sürümden önce alınan
   yedekler programın klasörünü ve verisini içermez; yedek listesi bunu her
   birinin yanında söyler ("Bu yedek programı içermiyor (eski biçim)").
3. **Her alan adının sertifikasına bir kez bakın** (alan adının **Güvenlik**
   bölümü). Yeni "Neler korunuyor" listesi, sertifikanın neyi koruduğunu ad
   ad söyler. Posta adı eksikse orada "Korunmuyor" ve tek bir düğme görünür:
   **Posta adını da kapsa**. Bir basış, sertifikayı bu adla yeniden ister.
   Kendiliğinden hiçbir şey yeniden istenmez.
4. alpha.133'ten sonra yapmadıysanız: CelikPanel'in veritabanı hesabına her
   veritabanı sunucusunda bir kez yeni parola verin (**Bileşenler →
   MariaDB**, "CelikPanel'in bu hizmete bağlandığı hesap", **Diğer → Yeni
   parola ver…**; aynısı **Bileşenler → PostgreSQL** sayfasında). Nedeni
   alpha.133 notlarında yazıyor.
5. *(Veritabanı hakları: bu işin güncelleme adımları, iş sürüme
   katıldığında buraya eklenir.)*

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
  klasörünü sitenin hesabına ait olarak yerine koyar ve programı yeniden
  başlatır; sonuç programın başlayıp yanıt verdiğini ya da neden başlamadığını
  ve **Programlar** sekmesinde ne yapılacağını söyler. Sizin durdurduğunuz
  bir program durmuş kalır. Dosyaları yerine koymak başarısız olursa önceki
  dosyalar geri konur.
- Programın gizli ayarları hiçbir zaman yedeğe yazılmaz. Aynı sunucuda geri
  yüklerken sunucudaki ayarlar olduğu gibi kalır; yeni bir sunucuda gizli
  değerleri **Programlar** sekmesinden yeniden girin.
- Eski yedekler eskisi gibi geri yüklenir; program klasörü içermedikleri
  için program olduğu gibi kalır.

### Tek bir veritabanı için veritabanı hakları

*(Bu bölüm, veritabanı hakları işi incelemesinden sonra bu sürüme
katıldığında yazılır: hakların tam olarak tek bir veritabanı adına
verilmesi, önceki hakların kendiliğinden dönüştürülmesi ve silinen bir
veritabanının haklarının her hesaptan ve rolden kaldırılması.)*

## Bilinen sınırlar

### Sertifikalar

- **Yeni sertifika istekleri gerçek bir sertifika kuruluşuna karşı
  denenmedi.** Bileşen testleriyle denetlendi; deneme amaçlı bir sertifika
  kuruluşuyla yapılacak laboratuvar denetimi henüz yapılmadı.
- Sertifika alındıktan sonra bu sunucuyu göstermeye başlayan bir ad (yeni
  bir diğer ad ya da sonradan açılan `mail.<alan adı>`) "Korunmuyor" ve
  **Bu adla yeniden al** düğmesiyle gösterilir; kendiliğinden eklenmez.
- Bütün alt adları kapsayan bir sertifika (joker sertifika) yok.

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
  hesabına verin (`chown -hR --reference=<site ev klasörü> <site ev
  klasörü>/app`); her veritabanını `restore/payload/databases/` içinden
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

### Veritabanı hakları

*(Veritabanı hakları işi bu sürüme katıldığında doldurulur.)*

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

- Önceki sınırlar sürüyor (bkz. alpha.133); bu sürümün düzelttiği iki sınır
  dışında: site yedekleri artık programı içeriyor ve sertifika ekranı artık
  posta adının kapsanıp kapsanmadığını söylüyor.

## Laboratuvarda denetlenenler

**Bu sürümün laboratuvar denetimi henüz yapılmadı.** Bu bölüm, denetimden
sonra neyin denetlendiği ve neyin denetlenmediğiyle doldurulur.

## Kanıt

Panelin veritabanına 078 numaralı değişiklik eklendi (77 → 78): her alan adı
için, posta programlarına alan adının kendi posta adının mı söylendiğini
tutan yeni bir ayar. Daha önce postayı sertifikasıyla koruyan her alan
adında bu ayar açık başlar; böylece güncellemeden sonra hiçbir programa
başka bir ad söylenmez. Değişiklik yalnızca ekler; alpha.133 bu değişikliği
içeren bir veritabanını okumayı reddeder, bu yüzden alpha.133'e dönüş
güncelleme öncesi kopyanın geri yüklenmesidir. İki yeni salt-okur sorgu
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
