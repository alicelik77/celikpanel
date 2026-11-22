# v0.1.0-alpha.144

[English](RELEASE-NOTES-v0.1.0-alpha.144.md)

**Küçük bir sürüm.** Web postasının kendi adresindeki bir güvenlik açığını
kapatır, alpha.143 laboratuvarının istediği altı düzeltmeyi ve trafik
rakamlarının bir düzeltmesini getirir ve küçük bir şey ekler: aynı hesabın
bir alan adının içindeki takma ad artık DNS'te yanıt verir. Başka yenilik
yoktur ve panelin veritabanı yapısı değişmez.
**Kurulu panelleri yalnız sahibi, panelin kendi güncelleme ekranından
günceller.**

## alpha.143'ten güncelleme

1. Her zamanki gibi panelin kendi güncelleme ekranından güncelleyin.
2. **Panelin veritabanı yapısı değişmez** (şema 81'de kalır). alpha.143'e
   dönmek olağan geri dönüştür; bir anlık görüntüyü geri yüklemek gerekmez.
3. Aracı güncellemeden sonraki ilk açılışında kendiliğinden, bir kez, iki şey
   yapar: web postası adresi olan bir sunucuda o adresin web sunucusu
   dosyasını yeni biçimde yazar ve web sunucusunu yeniden yükler (aşağıda
   "Güvenlik: kapanan yollar"), ve CelikPanel'in günlük klasörünü web
   sunucusunun çalışan süreçleri için geçilebilir yapar (aşağıda "Gece günlük
   döndürmesinden sonra trafik rakamları"). Basmanız gereken bir şey yok.
   Yapamadığı şey **Şu anki sorunlar**da adımıyla birlikte tek satırdır.

## Güvenlik: kapanan yollar

- **Web postasının kendi adresi sayfalarına ait olmayan dosyaları veriyordu.**
  alpha.110'dan beri web postasının kendi adresi (Roundcube'un ayrı web
  adresi), bir Roundcube eklentisinin klasöründeki PHP olmayan her dosyayı,
  adresi açabilen herkese olduğu gibi veriyordu: örneğin bir eklentinin
  yapılandırmasının bir kopyası (`config.inc.php.bak`, doldurulmuş bir
  `config.inc.php.dist`) ya da bir eklentinin `.sql`, `.log`, `.ini`, `.yml`
  veya `composer.json` dosyası. Ayrıca herhangi bir eklentinin PHP dosyasını
  doğrudan çalıştırıyordu. Roundcube'un kendi program klasörlerine
  ulaşılamıyordu. Artık adres yalnız web postasının kendi giriş noktasını
  (`index.php`) çalıştırır ve yalnız web postası sayfalarının yüklediği durağan
  dosyaları verir (betikler, stil dosyaları, resimler, yazı tipleri, yeni posta
  sesleri, tarayıcı denetiminin PDF ve TIFF dosyaları ve temaların sayfaları);
  geri kalan her şey "bulunamadı" yanıtı verir ve yedek, döküm, günlük ya da
  ayar dosyasına benzeyen bir ad her yerde reddedilir.
- **Var olan sunucular kendiliğinden düzeltilir:** aracının güncellemeden
  sonraki ilk açılışında yeni dosya yazılır, `nginx -t` ile denetlenir ve web
  sunucusu yeniden yüklenir. Denetim ya da yeniden yükleme başarısız olursa
  önceki dosya aynen geri konur; web postası ve bütün siteler hizmet vermeyi
  sürdürür.
- **Elle değiştirdiğiniz bir web postası adresi dosyası korunur;**
  CelikPanel'in metni yanına `<dosya>.celikpanel-pending` olarak konur (web
  sunucusu onu okumaz). **Şu anki sorunlar** bunun üzerine tek satırda web
  postasının adresinin hâlâ eklenti dosyalarını verdiğini, sunucu sahibinin
  işlem yapacağını ve adımı söyler: o metni adlandırır ya da CelikPanel onu
  dosyanızın yanına yazamadıysa dosyanızı kaldırıp Roundcube sayfasında adres
  için **Yeniden kur**'a basmanızı söyler. Web sunucusunun denetimi başka bir
  dosyada başarısız olduğu için yeni metin geri alındığında da aynısını
  yapar ve o dosyayı adlandırır. Aracı her açılışta yeniden dener; dosya yeni
  biçime girince satır kendiliğinden kalkar.
- Panelin kendi `/webmail/` adresi tarayıcıyı yalnız o adrese yönlendirir
  (alpha.110'dan beri) ve değişmez. Yalnız yerel bir soket üzerinden dinleyen
  web postası sunucusuna ağdan ulaşılamaz; önceki kurallarını korur.

## Sizin için ne değişir

### alpha.143 laboratuvarının düzeltmeleri

- **Siz sayfadan ayrılırken oluşturulan posta kutusu** eksiksiz oluşturulur:
  sayfadan ayrılsanız ya da bağlantı kesilse bile panel oluşturmayı ve
  kaydını bitirir; aynı basış iki kez ulaşırsa kutu bir kez oluşturulur. Bir
  otomatik yanıtı kaydetmek için de aynısı geçerlidir.
- **Sunucuda kaydı olmadan bulunan posta kutusu.** Sunucunun panelde kaydı
  olmadan zaten tuttuğu bir adla posta kutusu oluşturmak artık iç hatayla
  yanıtlanmaz ve hiçbir şey değişmez. Bir müşteri ya da bayi, posta kutusunun
  sunucuda var olduğunu okur ve yöneticiye başvurması ya da başka bir ad
  seçmesi istenir. Yöneticiye **Bu alan adına al** sunulur: yalnız içinde
  posta olmayan ve CelikPanel'in yazdığı biçimde olan bir kutu için çalışır ve
  her zaman alma penceresine yazılan yeni parolayı koyar; böylece önceki
  hiçbir parola çalışmaz. İçinde posta olan ya da klasörü denetlenemeyen bir
  kutu alınmaz: ne olacağına sunucunun sahibi sunucuda karar verir. Alınan
  kutu bundan sonra sıradan bir kutudur: sunucuda bulunan boyut sınırını
  korur, alan adının imza anahtarı ve ilk gün gönderim sınırı ona da
  uygulanır.
- **Sayfası bırakılan bir PostgreSQL kapatması** artık "gönderildi, henüz
  doğrulanmadı" diye kaydedilmez: kapatma ve kaydı biter, sayfaya
  dönüldüğünde kapatma gerçek sonucunu gösterir. Geri al ve Yeniden oku için
  de aynısı geçerlidir.
- **Başarısız bir kapatmanın penceresi** "Sunucuda çalışılıyor…" adımında
  kırmızı bir işaretle değil, "Yapılmadı — hiçbir şey değiştirilmedi" adımında
  (sonuç bilinmediği sürece "Sonuç henüz doğrulanmadı — yeniden gönderilmez")
  biter. Nedeni önce sayfanın dilinde nedenin türünü söyler (veritabanı
  kullanımda, süre doldu, bağlantı kurulamadı, bir hak eksik, bir rol ya da
  veritabanı yok, veritabanı sunucusu reddetti), ardından veritabanı
  sunucusunun kendi iletisini tırnak içinde verir.
- **Retler sayfanın dilinde.** Türkçe sayfada İngilizce çıkan takma ad ve
  sertifika retleri sayfanın dilinde söylenir: takma ad için yeniden
  verilemeyen ya da onu kapsamayan bir sertifika, geçersiz bir takma ad,
  kullanımda olan bir ad, bulunamayan bir takma ad, HSTS'nin engellediği bir
  kaldırma, kaldırılacak etkin sertifikanın olmaması. Yanıtların kendisi
  değişmez.
- **"1 PostgreSQL veritabanı":** tek veritabanı **Şu anki sorunlar**da ve
  uyarı postasında tekil söylenir (Türkçe satırda zamirler de buna uyar).
- **Gece günlük döndürmesinden sonra trafik rakamları.** Trafik rakamları,
  her gecenin günlük döndürmesinden sonra web sunucusu bir dahaki sefer
  yeniden yüklenene dek eksik sayılıyordu: web sunucusunun çalışan süreçleri
  CelikPanel'in trafik günlüklerini yeniden açamıyordu (her gece günlük başına
  bir "Permission denied" hata satırı) ve döndürülen dosyaya yazmayı
  sürdürüyordu; o dosyayı bir sonraki gecenin döndürmesi siliyordu.
  Güncellemeden sonraki ilk açılışta CelikPanel'in günlük klasörü
  `/var/log/celikpanel` geçilebilir olur (diğer hesaplar içinden geçebilir,
  listeleyemez) ve `traffic` klasörüne yalnız web sunucusunun çalışan
  süreçleri geçebilir (0710; sahibi root, grubu web sunucusunun grubu):
  hiçbir sitenin hesabı başka bir sitenin trafik günlüğünü açamaz (web
  sunucusunun kendi hesabıyla çalışan programlar için "Bilinen sınırlar"a
  bakın). Diğer hesapların okuyabildiği bir trafik günlüğü (ilkini web
  sunucusu herkese okunur yapabiliyordu) onlara kapatılır; bir sitenin ilk
  günlüğü ve gece döndürmesinin her yeni günlüğü artık web sunucusunun
  hesabı için kapalı olarak oluşturulur. Kendi koyduğunuz bir klasör kipi
  sahibi ve grubuyla korunur; grubu ne web sunucusunun ne de CelikPanel'in
  verdiği bir grup (root, celikpanel) olan 0710 kipli bir `traffic` klasörü
  de korunur; yaptığınız bir bağlantı da korunur. Her biri aracının
  günlüğünde adlandırılır; sertifika aracının günlük klasörü kapalı kalır.
  Yeniden yükleme gerekmez: güncellemeden sonraki ilk geceden
  itibaren böyle bir hata satırı yazılmaz ve trafik, bir yeniden yükleme
  beklemeden yeniden eksiksiz sayılır. Yeni kurulumlar klasörleri ve
  günlükleri baştan böyle oluşturur.

### Yeni, küçük: takma ad DNS'te yanıt verir

- Aynı hesabın, DNS'ini bu sunucunun tuttuğu bir alan adının içindeki takma
  ad artık o alan adının bölgesinde sitenin adres kayıtlarını alır; kayıtlar
  bölgeyle birlikte yayımlanır (yedek DNS sunucusu onları bölgenin olağan
  aktarımıyla alır). Önceki hiçbir sürüm bunu yapmıyordu: takma ad yalnız
  sitenin web sunucusu dosyasında ve sertifikasında bir addı. Takma ad için
  sertifika yeniden verilirken kayıt, sertifika yetkilisi adı denetlemeden
  önce yayımlanır ve takma ad eklenmezse geri çekilir.
- Kayıt takma adla birlikte gider; takma adın alan adı silindiğinde de,
  aboneliği başka bir hesaba geçtikten sonra da. Yalnız CelikPanel'in
  eklediği kaldırılır, o da yalnız bölge takma adın alan adının hesabına
  aitken: artık başka bir hesaba ait bir bölgede hiçbir şey kaldırılmaz ve
  panelin günlüğü bakılacak kaydı adlandırır.
- Yayın bitemezse (yedek DNS sunucusu bekliyorsa, DNS yayımlayıcısı hazır
  değilse) takma ad satırı "DNS kaydı yayımlanmayı bekliyor — her saat yeniden
  denenir, bir şey yapmanız gerekmez" der ve panelin saatlik turu onu yayımlar.
  Bir alan adının silinmesini asla durdurmaz.
- O adda sizin oluşturduğunuz bir adres kaydı asla değiştirilmez ya da
  kaldırılmaz ve yanına bir şey eklenmez: takma ad "korundu" yanıtını verir ve
  sayfa, takma ad bu siteye ulaşacaksa adı DNS kayıtlarından bu sunucuya
  yönlendirmenizi söyler.
- Yapmadığı şey: kendi başına bir alan adı olan takma ad (burada tutulan aynı
  hesabın bir alan adının içinde olmayan) hâlâ bölge almaz; bu sürümden önce
  oluşturulan bir takma ad kaydını yalnız kaldırılıp yeniden eklendiğinde
  alır.

## Bilinen sınırlar

- Başarısız bir PostgreSQL kapatmasının neden türü, veritabanı sunucusunun
  İngilizce iletisinden tanınır. PostgreSQL iletileri başka bir dilde olan
  bir sunucuda her başarısızlık, tırnak içindeki iletisiyle birlikte,
  "veritabanı sunucusu reddetti" diye okunur.
- Başka değişiklikler hâlâ tarayıcının isteğiyle çalışır: sunucu çalışırken
  sayfadan ayrılırsanız böyle bir değişiklik sunucuda panelin kaydı olmadan
  bitebilir ya da yalnız yanıtı kaybolur. Bunlar arasında zamanlanmış
  görevler, posta yönlendiricileri ve her şeyi yakalayan adres, bir posta
  kutusunun parolası, boyut sınırı ve silinmesi, DKIM, bir sitenin SSL ve PHP
  ayarları, dosya erişimi, DNS kurulumu ve DNSSEC, VPN, bir yedeği silmek, bir
  katalog uygulamasını kurmak ya da kaldırmak ve veritabanı içe aktarma
  vardır; tam liste sonraki bir sürüm için bu sürümün çekme isteğindedir
  (#355).
- Kendi başına bir alan adı olan takma ad bölge almaz (olması gerekip
  gerekmediğine sahip karar verir) ve bu sürümden önce oluşturulan takma
  adlar kaydını yalnız yeniden eklendiğinde alır (yukarıda).
- Güncellemeden sonraki ilk gece döndürmesine dek web sunucusunun çalışan
  süreçleri önceden yazdıkları yere yazar; sayım o geceden itibaren
  eksiksizdir.
- Hâlâ web sunucusunun kendi ortak hesabıyla çalışan programlar trafik
  günlüklerini okuyabilir: henüz kendi hesabına taşınmamış Node.js siteleri
  ve açıksa dağıtımın varsayılan PHP havuzu. Bu günlükler yalnız isteklerin
  zamanını ve bayt sayılarını tutar, adres tutmaz; o programlar her sitenin
  sunulan dosyalarını zaten okuyabilir.
- Yalnız yerel bir soket üzerinden dinleyen web postası sunucusu önceki
  kurallarını korur; ona ağdan ulaşılamaz.
- alpha.143'ün notlarının hâlâ açık diye listelediği her şey açık kalır
  ([alpha.143 sürüm notları](RELEASE-NOTES-v0.1.0-alpha.143.tr.md)).
- **Web postası posta gönderemiyor** (posta okumak, ekler ve ayarlar
  çalışıyor); alpha.145'e dek bir posta programından gönderin. Laboratuvar
  bunu ve iki küçük görünüm bulgusunu, her biri alpha.145'e dek ne
  yapılacağıyla birlikte buldu: aşağıda "Laboratuvarda bulunan, alpha.145'e
  planlananlar".

## Laboratuvarda denetlenenler

Laboratuvar denemesi bitti: bir test sunucusu çiftinde bir deneme, PowerDNS'li
Debian 13 ana DNS sunucusu ve BIND'li Ubuntu 24.04 yedek DNS sunucusu
(22 Kasım 2026, UTC 19:02'den toparlamayla birlikte yaklaşık 20:30'a).
alpha.143 durumundan başladı ve sahibin yolunu izledi: güncellemeden önce
sahip ana sunucuda web postasına sayfadan kendi adresini verdi ve iki sunucu
da panelin kendi güncelleme sayfasından sürümün koduna güncellendi
(`6ed1f94a0`, yalnız laboratuvara ait v0.1.0-alpha.152 etiketiyle). İki
tarafı, aynı bayinin ayrı hesaplardaki iki müşterisi oynadı: alice ve sitesinin
kendi hesabıyla onun trafik günlüğünü okumaya çalışan bob. Laboratuvarın
yaptığı yazılanlar dışında her şey sayfanın kendi düğmeleriyle yapıldı. Ağır
hata yok.

- **Güncellemeden önce web postasının adresi açıktı** (alpha.143'ün kodu):
  bir eklentinin yapılandırmasının yedek kopyası
  (`plugins/archive/config.inc.php.bak`) ve laboratuvarın o eklentinin
  klasörüne koyduğu bir metin dosyası içerikleriyle verildi, bir eklentinin
  `composer.json` dosyası verildi ve doğrudan istenen bir eklenti PHP dosyası
  (`plugins/archive/archive.php`) PHP'ye verilip çalıştırıldı (bir PHP
  hatasıyla yanıt verdi).
- **İki sunucu da panelin kendi güncelleme sayfasından kendiliğinden
  güncellendi**, her biri yaklaşık bir buçuk dakikada. Şema adımı yok (81'de
  kalır); her sunucuda bir panel ve bir aracı çalıştı; yeni hatalı hizmet
  yok. Web postası adresinin bulunduğu ana sunucuda web sunucusu bir kez
  yeniden yüklendi; dışarıdan her saniye yapılan bir denetim, güncelleme
  boyunca bir sitenin ve web postasının yanıt verdiğini gördü. Yedek
  sunucunun web sunucusu dosyaları değişmedi ve yeniden yüklenmedi.
- **Güncellemeden sonra web postasının adresi** (güvenlik düzeltmesi):
  - konan yedek kopya ve metin dosyası, eklentinin PHP dosyası, bir
    `config.inc.php.dist`, `composer.json` ve `.sql`, `.log` ve `.md`
    dosyaları "bulunamadı" yanıtı verdi; nokta ile başlayan dosyalar
    (`.htaccess`, `.env`, `.git/config`) reddedildi; betikler, stil
    dosyaları, resimler, tarayıcı denetiminin dosyası (`blank.tiff`), bir
    temanın sayfası ve giriş noktası (`index.php`) yanıt vermeyi sürdürdü;
  - alice web postasını bir müşteri olarak tarayıcıda baştan sona kullandı:
    oturum açtı, gelen kutusunu listeledi, bir PDF ve bir metin eki olan bir
    iletiyi açıp iki eki de indirdi, düzenleyiciyi açtı, bir ayarı kaydetti,
    bir eklentinin ayarlarını ve kişileri açtı; hiçbir istek başarısız olmadı
    ve konsolda hata yazılmadı. Gönderme başarısız oldu: bkz.
    "Laboratuvarda bulunan", 1;
  - sahibin değiştirdiği bir adres dosyası birebir korundu; panelin metni
    yanında, **Şu anki sorunlar**da tek satır (Türkçe ve İngilizce). Sahip
    panelin metnini alıp web sunucusunu yeniden yükleyince satır
    kendiliğinden kalktı;
  - başka bir web sunucusu dosyası bilerek bozulmuşken önceki adres dosyası
    aynen geri kondu, siteler ve web postası yanıt vermeyi sürdürdü ve
    **Şu anki sorunlar** satırı bozuk dosyayı adlandırdı. O dosya
    kaldırılınca aracının bir sonraki açılışı yeni biçimi yazdı ve satır
    kalktı.
- **Posta.**
  - Sayfadan hemen ayrılırken oluşturulan posta kutusunun kaydı var ve
    oturum açıyor.
  - Sunucunun kaydı olmadan tuttuğu bir posta kutusunu oluşturmak müşteriye
    açıkça söylendi: kutu sunucuda var, hiçbir şey değiştirilmedi,
    yöneticiye başvurun ya da başka bir ad seçin.
  - Yönetici onu yeni bir parolayla aldı: yeni parola oturum açıyor, eskisi
    artık açmıyor.
  - İçinde bir ileti olan posta kutusunun alınması reddedildi; parolası
    değiştirilmedi ve iletisi korundu.
  - Başka bir hesabın alan adı üzerinden posta kutusu almak "bulunamadı"
    (404) ile, o alan adı hakkında hiçbir şey söylenmeden reddedildi; başka
    bir alan adının adresi de reddedildi.
  - Sayfadan hemen ayrılırken kaydedilen otomatik yanıt saklandı.
- **DNS'te takma adlar.**
  - alice'in kendi alan adının içindeki bir takma ad ilk bakışta iki
    sunucudan da yanıt verdi ve kaldırılınca ikisinde de yanıt vermez oldu.
  - Onun zaten kendi adres kaydı olan bir ad olduğu gibi bırakıldı, yanına
    hiçbir şey eklenmedi ve sayfa bunu söyledi.
  - Geçersiz ve yinelenen bir takma ad, sayfanın dilinde (Türkçe)
    reddedildi.
  - Yedek sunucuya ulaşılamazken bir takma ad kaydedildi ve satırı
    "yayımlanmayı bekliyor" dedi; yedek sunucu dönünce kısa süre sonra kaydı
    yayımlandı ve iki sunucudan da yanıt verdi (satırın kendisi için bkz.
    "Laboratuvarda bulunan", 2).
- **PostgreSQL.**
  - Sayfası hemen bırakılan bir kapatma, dönüldüğünde gerçek sonucunu
    gösterdi.
  - Veritabanının sahibi bir kilit tutarken yapılan ve sayfası sunucu yanıt
    vermeden bırakılan bir kapatma, dönüldüğünde yapılmadığını, hiçbir şeyin
    değiştirilmediğini ve nedenini (veritabanı kullanımdaydı ve kilit zamanında
    gelmedi) sunucunun iletisiyle gösterdi; yeniden gönderilmedi.
  - Sayfadan Geri al önceki hakları geri verdi.
  - Başarısız bir kapatma (kayıtlı bir kullanıcının rolü laboratuvarca
    silinmişken) "Yapılmadı — hiçbir şey değiştirilmedi" adımında bitti,
    nedenin türünü söyledi (adı geçen bir şey veritabanı sunucusunda yok) ve
    sunucuyu tırnak içinde aktardı, Türkçe ve İngilizce (başlığı için bkz.
    "Laboratuvarda bulunan", 3).
- **Trafik günlükleri.**
  - Güncellemeden sonra `/var/log/celikpanel` 0750'den 0751'e (diğer
    hesaplar içinden geçebilir, listeleyemez), `traffic` klasörü 0755'ten
    root ve web sunucusunun grubu için 0710'a geçti, sertifika aracının
    klasörü kapalı kaldı (0700); hiçbir trafik günlüğü diğer hesaplarca
    okunamıyordu; aracının günlüğü bunu bir kez söyledi; döndürme dosyası
    bir kez yeniden yazıldı.
  - Yedek sunucuda, grubunu laboratuvarın elle koyduğu bir `traffic` klasörü
    korundu ve aracının günlüğünde adlandırıldı.
  - Yeni bir sitenin ilk günlüğü kapalı oluşturuldu (0640, web sunucusunun
    hesabı).
  - bob'un sitesinin kendi hesabı, zorla yapılan bir döndürmeden önce de
    sonra da ne alice'in sitesinin trafik günlüğünü okuyabildi ne de klasörü
    listeleyebildi.
  - Zorla yapılan döndürmeden sonra "Permission denied" satırı yazılmadı ve
    yeni istekler yeni günlüğe düştü.
- İki sunucuda da olağan sayfalar hatasız açıldı.

**Laboratuvarın sayfanın yolundan ayrıldığı yerler.** Laboratuvar,
laboratuvar sunucularında root olarak, bir web postası eklentisinin
klasörüne bir yedek kopya ve bir metin dosyası koydu; web postasının adres
dosyasını bir sahibin yapacağı gibi değiştirdi (bir yorum satırı), aracıyı
yeniden başlattı, sonra panelin metnini o dosyanın yerine alıp web
sunucusunu yeniden yükledi; başka bir web sunucusu dosyasını bilerek bozdu,
aracıyı yeniden başlattı ve dosyayı yeniden kaldırdı; deneme iletilerini
sunucuda teslim etti; yedek sunucuda `traffic` klasörünün grubunu elle
koydu; başka bir sitenin trafik günlüğünü okumak için bob'un sitesine küçük
bir betik koydu; zorla bir günlük döndürmesi yaptı; yedek sunucuyu ana
sunucudan ulaşılamaz yapıp yeniden ulaşılır yaptı ve sertifika aracını
laboratuvarın kendi sertifika yetkilisine kaydetti. Bir kez, panel
durmuşken, sunucunun kaydı olmadan tuttuğu posta kutuları oluşturmak için
laboratuvar panelinin veritabanına yazdı ve iki posta kutusunun panel
satırlarını sildi (alpha.143'ün kodunda sayfadan ayrılarak yapılan iki
deneme, satırı olmayan bir posta kutusu bırakmamıştı). Veritabanı yöneticisi
olarak bir veritabanını alpha.141'den önce açılanların durumuna getirdi ve
kayıtlı bir kullanıcının rolünü sildi; o veritabanının sahibi olarak bir
kilit tuttu. Hesaplar ve alan adları arasındaki iki alma reddi doğrudan
panelin API'sine gönderildi.

**Denetlenmeyenler:**

- Açık tek PostgreSQL veritabanı için **Şu anki sorunlar** satırının tekil
  ifadesi: o duruma ulaşılamadı, çünkü izlenen veritabanları açık
  sayılmıyor; testlerle kapsanıyor.
- Sayfası bırakılan Geri al ve Yeniden oku: sayfası bırakılarak yalnız
  kapatma denendi; testlerle kapsanıyor.
- Döndürmeden sonra bir sitenin sayfasındaki trafik rakamı: süre.
- Yedek sunucuya ulaşılamazken bir alan adını silmek: süre; testlerle
  kapsanıyor.
- Aboneliği başka bir hesaba geçtikten sonra bir takma adı kaldırmak: sayfa
  taşımayı yalnız bir istek ve onay olarak sunuyor; testlerle kapsanıyor.
- Sertifikalı bir takma ad (takma ad için yeniden verilen ya da reddedilen)
  ve Türkçe sayfanın sertifika ve HSTS retleri: bu denemenin parçası değildi;
  testlerle kapsanıyor.
- Olağan sayfaların gezintisinde **Günlükler** sayfası: laboratuvarın
  kullandığı adres kendi başına bir sayfa değil, bu yüzden sayfa açılmadı.
- Yedek sunucuda web postası: laboratuvarda onun web postası adresi yok.
- Veritabanı web araçları: laboratuvar çiftinde kurulu değil (alpha.142'den
  taşındı, hâlâ açık).
- Laboratuvar sunucusunun 17'si dışındaki PostgreSQL sürümleri, PostgreSQL
  iletileri İngilizce olmayan bir sunucu ve şablonları SQL_ASCII olan bir
  sunucu.

### Laboratuvarda bulunan, alpha.145'e planlananlar

Hiçbiri bir güvenlik ya da veri kaybı sorunu değil.

1. **Web postası posta gönderemiyor.** Web postasından göndermek bir kimlik
   doğrulama hatasıyla yanıtlanıyor: panelin web postası yapılandırması
   postayı, bağlantıyı önce şifrelemeye geçirmeden posta sunucusunun
   gönderim portuna veriyor; posta sunucusu ise girişi yalnız şifreli bir
   bağlantıda kabul ediyor. Posta okumak, ekler ve ayarlar çalışıyor. Bu, bu
   sürümden eski: aynı yapılandırma metni, sahibin sunucularının çalıştırdığı
   alpha.143'te de var; web postası adresli ilk laboratuvar denemesi bu
   olduğu için şimdi bulundu. Web postasının hangi sürümden beri
   gönderemediği belirlenmedi. alpha.145'e dek: bir posta programından
   gönderin (587 ya da 465 numaralı port, posta kutusunun tam adresi ve
   parolasıyla).
2. Yedek sunucu dönünce bir takma adın satırı, kaydı iki sunucudan da yanıt
   verdiği hâlde bir sonraki saatlik tura dek (burada yaklaşık yarım saat)
   "yayımlanmayı bekliyor" demeyi sürdürdü; bu arada bölgenin kendi
   bildirimi değişikliği yeniden yapmayı söylerken takma ad satırı bir şey
   yapmak gerekmediğini söylüyordu. Yapılacak bir şey yok: saatlik tur
   satırı temizler.
3. Başarısız bir PostgreSQL kapatmasının penceresi, "Tamamlanmadı"nın
   üstünde sürmekte olan başlığını ("… başka hesaplara kapatılıyor") tutuyor.
   Altındaki adımlar ve neden doğru.

## Güncellemeden sonra ne beklenir

- Güncelleme laboratuvarda sunucu başına iki dakikadan kısa sürdü ve
  bittiğini kendisi söyler.
- Şema adımı yok: yapı şema 81'de kalır ve alpha.143'e dönmek olağan geri
  dönüştür.
- Web postası adresi olan bir sunucuda web sunucusu ilk açılışta,
  yapılandırma denetimi geçtikten sonra bir kez yeniden yüklenir; bundan
  sonra web postasının adresi yalnız sayfalarının yüklediğini verir.
- Web postası hâlâ posta gönderemez (bilinen sınır; "Laboratuvarda bulunan",
  1): alpha.145'e dek bir posta programından gönderin.
- Günlük klasörlerinin kipleri ilk açılışta bir kez değişir:
  `/var/log/celikpanel` 0700'den 0711'e ya da 0750'den 0751'e; `traffic`
  klasörü 0755'ten 0710'a, sahibi root ve grubu web sunucusunun grubu
  (Debian ve Ubuntu'da `www-data`, Arch Linux'ta `http`); diğer hesapların
  okuyabildiği trafik günlükleri 0644'ten 0640'a. Kendi koyduğunuz bir kip
  (sahibi ve grubuyla), kendi grubunuzla 0710 kipli bir `traffic` klasörü ya
  da yaptığınız bir bağlantı korunur. CelikPanel'in döndürme dosyası
  `/etc/logrotate.d/celikpanel-traffic`, onu değiştirmediyseniz, her yeni
  trafik günlüğü web sunucusunun hesabı için 0640 oluşturulsun diye bir kez
  yeniden yazılır; değiştirdiğiniz dosya korunur. Güncellemeden sonraki ilk
  geceden itibaren trafik rakamları eksiksizdir.
- Sunucunun panel kaydı olmadan tuttuğu bir posta kutusu açıkça söylenir;
  onu yalnız yönetici, yalnız içinde posta yokken ve yeni bir parolayla
  alabilir.
- alpha.143'e döndükten sonra: alpha.143 yeni web postası adresi dosyasını
  tanımaz ve onu elle değiştirmişsiniz gibi korur; böylece web postası hizmet
  vermeyi sürdürür ve hiçbir şey yeniden açılmaz; web postası adresi
  değişiklikleri ise yeniden güncelleyene dek elle düzenlenmiş diye reddedilir.
  alpha.144'ün eklediği bir takma ad kaydı, takma ad alpha.143 altında
  kaldırıldığında kalır; onu DNS kayıtlarından kaldırın.

## Kanıt

Şema değişikliği yok: `internal/db/migrations/` altında hiçbir dosya
değişmez; yapı 81'de kalır. Yeni, hepsi ekleme: derlemeye bağlı, salt-okur
aracı çağrısı `MailboxOnServerV1`; salt-okur aracı çağrısı
`WebmailAddressFormsV1`; Şu anki sorunlar türleri `webmail_address_kept` ve
`webmail_address_refused`; ret kodları `MAIL_ACCOUNT_ON_SERVER_ONLY`,
`MAIL_ACCOUNT_ADOPT_REFUSED`, `MAIL_ACCOUNT_ADOPT_RECORD_NOT_WRITTEN`,
`ALIAS_INVALID`, `ALIAS_CONFLICT`,
`ALIAS_NOT_FOUND`, `ALIAS_CERTIFICATE_REISSUE_FAILED`,
`ALIAS_CERTIFICATE_NOT_COVERING`, `HSTS_REMOVAL_BLOCKED` ve
`SSL_NO_ACTIVE_CERTIFICATE` (durum kodları değişmedi); panel ayar satırları
`alias_dns_v1:<takma ad>`; denetim kaydı eylemi `mail.account.adopt`; aracının
bir kezlik makbuzu `log-root-modes-v2.done`. Dört yol, istek kimliği
korumasının işlem adımlarına katılır (PostgreSQL kapatma, geri alma ve
Yeniden oku; posta kutusu oluşturma; otomatik yanıt kaydı; alma); saklanan
özetleri asla parola tutmaz.

Maddeye göre testler:

- Web postasının adresi: `TestWebmailAddressServesOnlyRoundcubesStaticFilesAndEntryPoint`
  (oluşturulan dosyayı nginx'in konum seçtiği gibi okur: 21 ad reddedilir;
  bir betik, bir stil dosyası, bir resim, bir yazı tipi, bir temanın boş
  sayfası, bir ses ve `index.php` verilir), `TestWebmailAddressEarlierFormServedPluginFiles`
  (önceki biçim `config.inc.php.bak` dosyasını veriyor ve bir eklentinin
  PHP'sini çalıştırıyordu),
  `TestWebmailAddressEarlierFormIsRewrittenAtStartAndAnOwnersFileKept`,
  `TestWebmailAddressFormsNameWhatStillServesPluginFiles`,
  `TestA144WebmailAddressInItsEarlierFormIsAProblemUntilItIsNot`; dağıtım
  yapılandırma matrisi yeni dosyayı oluşturur (`TestRenderDistroConfigMatrix`,
  CI'da nginx ile denetlenir).
- DNS'te takma ad: `TestA144AliasInsideItsAccountsZoneAnswersAndGoesAgain`,
  `TestA144AliasNeverTakesTheOwnersRecords`,
  `TestA144AliasRecordsSurviveAFailedPublicationAndArePublishedLater`,
  `TestA144AliasAddsNothingBesideAnOwnersAddressRecord`,
  `TestA144AliasPublicationThatWaitsIsRetriedAndNeverStopsADeletion`,
  `TestA144DeletingADomainWithExternalDNSRemovesItsAliasRecordsHere`,
  `TestA144AliasRecordsStayInAZoneNowHeldByAnotherAccount`,
  `TestA144AliasRecordsGoWithTheAliasAfterASubscriptionTransfer`,
  `TestA144AWaitingZoneWhoseModeCannotBeReadKeepsItsWait`.
- Sayfası bırakılan kapatma: `TestA144PGPublicCloseIsNotLostWhenThePageIsLeft`
  (yol korunmadığında laboratuvarın kaydını yeniden üretir).
- Başarısız kapatma: `TestA144PGFailureKindNamesTheCause`,
  `TestA144FailedCloseAnswersTheKindOfCause`.
- Sayfanın dilinde retler: `TestA144AliasRefusalsAreCoded`,
  `TestA144AliasHandlersRefuseAnInvalidNameCoded`.
- Tekil satır: `TestA144OnePostgreSQLDatabaseIsSaidInTheSingular` ve web
  takımının sorun anahtarı testleri.
- Sayfası bırakılan posta kutusu ve sunucuda bulunan posta kutusu:
  `TestA144MailboxCreatedWhileThePageIsLeftHasItsRow`,
  `TestA144AutoresponderSavedWhileThePageIsLeftHasItsCopy` (ikisi de koruma
  kaldırılınca başarısız olur), `TestA144MailboxOnTheServerWithoutARowIsSaidPlainly`,
  `TestA144AdministratorAdoptsAMailboxFoundOnTheServer` (klasörde posta
  varken ve klasör sayılamazken reddedilir; parola, parola adımıyla bir kez
  değiştirilir),
  `TestA144SecondAdoptionOfTheSameAddressNeverReplacesThePassword`,
  `TestMailboxOnServerReadsOnlyCelikPanelsOwnForm`,
  `TestMailboxMessageCountReadsCurAndNewOnly`.
- Trafik günlükleri: `TestLogRootIsMadePassableWhateverTheUmask`,
  `TestASitesFirstTrafficLogIsMadeClosed`,
  `TestALogFolderThatIsTheOwnersLinkIsUsedAsBefore`,
  `TestLogRootBecomesPassableOnceAndKeepsTheOwnersModes`,
  `TestLogRootKeepsTheOwnersLinks`, `TestCertbotStorageGoesOnThroughTheOwnersLogLink`,
  `TestTrafficLogsReachOnlyTheWebServerWorkers` (web sunucusunun hesabı
  günlüğüne döndürmeden önce ve sonra ulaşır; ne root ne de web sunucusunun
  grubunda olan bir hesap hiçbir trafik günlüğünü adıyla açamaz ve hiçbir
  şeyi listeleyemez); düzeltmeden önceki kodda başarısız olurlar. İkinci
  incelemenin düzeltmeleriyle: `TestTrafficFolderAt0710WithTheOwnersGroupIsKept`,
  `TestTrafficLogNeedsTheWebServerAccountOnlyToMakeIt`,
  `TestTrafficSettingStaysWhenTheWebServerAccountCannotBeRead`,
  `TestTrafficLogrotateMakesEachNewLogClosed`.

Go modülünün ve web'in tam test takımları düzeltme şeridinde temel
(alpha.143'ün kodu) ve düzeltmeler üzerinde çalıştı; sürüm çekme isteğinin
her denetimi CI'da geçti. Düzeltmelerin bağımsız, salt-okur bir incelemesi
sürümü engelleyen bir şey bulmadı; bulguları bu sürümde düzeltildi, yukarıda
bilinen sınır olarak bırakılan biri dışında (neden türünün veritabanı
sunucusunun İngilizce iletisinden okunması).
