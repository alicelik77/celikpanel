# v0.1.0-alpha.143

[English](RELEASE-NOTES-v0.1.0-alpha.143.md)

**Bir güvenlik sürümü: lütfen yakında güncelleyin.** Bir müşterinin başka
bir müşteri gibi davranabildiği iki yolu kapatır: başka bir müşterinin alan
adıyla imzalanan giden posta ve alpha.141'den önce açılan, sunucudaki her
hesabın hâlâ girebildiği PostgreSQL veritabanları. Ayrıca
alpha.142 laboratuvarının istediği dört düzeltmeyi getirir.
**Kurulu panelleri yalnız sahibi, panelin kendi güncelleme ekranından
günceller.**

## alpha.142'den güncelleme

1. Her zamanki gibi panelin kendi güncelleme ekranından güncelleyin.
2. **Panelin veritabanı yapısı değişir** (şema 81): her yeni adı denetleyen
   altı kural, tek bir istisna eklenmiş aynı kurallarla değiştirilir
   (aşağıda "Kendi alan adınızın içinde bir alt alan adı"); hiçbir tablo,
   sütun ya da satır değişmez. Sonradan alpha.142'ye dönmek, yapıyı
   değiştiren her sürümde olduğu gibi, güncellemenin aldığı anlık görüntüyü
   geri yüklemek demektir.
3. Panel güncellemeden sonraki ilk açılışında posta imzası ayarlarını yapar
   (aşağıda "Giden posta ve imzası"). Sahibin elle değiştirdiği bir ayar
   korunur ve **Şu anki sorunlar**da adlandırılır. Kendisinin olmayan bir
   adresten yanıt veren otomatik yanıt o adres olmadan yeniden ayarlanır ve
   penceresi bunu bir kez söyler. Basmanız gereken bir şey yok. Sunucu onu
   yeniden ayarlayamazsa (örneğin posta sunucusu durmuşsa) yanıt kapatılır
   ve penceresi yeniden kaydetmeyi söyler; bu da yapılamazsa **Şu anki
   sorunlar** posta kutusunu ve adımı adlandırır, panel her saat yeniden
   dener.
4. Açılıştan birkaç dakika sonra, güncelleme doğrulanmış ve sunucu sakinken
   panel, alpha.141'den önce açılan ve kendi başına kapatabileceği
   PostgreSQL veritabanlarını izlemeye başlar; her birini, başka kimsenin
   kullandığının görülmediği tam bir günden sonra kapatır ve her saat bakar
   (aşağıda "alpha.141'den önce açılan PostgreSQL veritabanları").
5. **alpha.142'ye dönmek kapatılan yolları yeniden açar.**

## Güvenlik: kapanan yollar

- **Başka bir müşterinin alan adıyla imzalanan posta.** Posta sunucusu,
  "Kimden" satırında burada barındırılan bir alan adı geçen her iletiyi
  imzalıyordu: hem girişini kullanarak gönderen bir posta kutusu için hem
  de sunucuya girişsiz posta bırakan bir web sitesi için. Ayrıca bir
  otomatik yanıt, başka bir müşterinin adresinden yanıt verecek biçimde
  ayarlanabiliyordu. Böylece başka bir müşterinin adresini taşıyan posta, o
  müşterinin geçerli imzasıyla bu sunucudan çıkabiliyordu. Artık posta
  kutusu girişiyle gönderilen posta yalnız o posta kutusunun kendi alan adı
  için imzalanır, 25. porta gelen posta hiç imzalanmaz ve otomatik yanıt
  yalnız kutunun kendi adreslerinden verilir.
- **Her hesaba açık eski PostgreSQL veritabanları.** alpha.141'den önce
  açılan veritabanları PostgreSQL'in varsayılanını koruyordu: sunucudaki
  her veritabanı kullanıcısı her veritabanına bağlanabilir ve içinde geçici
  tablo açabilir. Bir müşterinin veritabanı kullanıcısı başka bir müşterinin
  veritabanına girip yapısını görebiliyordu. Artık bu veritabanları başka
  hesaplara kapatılır: başka kimsenin kullandığının görülmediği bir günden
  sonra panelin kendisi, yöneticinin karar vermesi gereken yerde tek
  düğmeyle.

## Sizin için ne değişiyor

### Giden posta ve imzası

- **Posta kutusu girişiyle gönderilen posta** (bir posta programı, web
  postası, 587 ya da 465 numaralı portta giriş yapan bir web sitesi),
  "Kimden" satırı ne derse desin, o posta kutusunun kendi alan adı için
  imzalanır. "Kimden" satırında başka bir müşterinin alan adı geçen posta
  artık o müşterinin anahtarıyla imzalanmaz.
- **Postayı 25. porta girişsiz bırakan bir web sitesi** artık imzalanmaz.
  Postasının imzalı kalması için sitenin postasını, kendi posta
  kutularından biriyle 587 numaralı portta giriş yapacak biçimde ayarlayın.
- **Otomatik yanıtlar** kutunun kendi adresinden ya da aynı hesabın bir
  alan adından kutuya yönlendirilmiş bir ek adresten verilir. Başka bir
  adres, yanıt kaydedilirken reddedilir. Önceden kaydedilmiş ve başka bir
  adresi adlandıran yanıt, güncellemeden sonraki ilk açılışta o adres
  olmadan yeniden ayarlanır. Penceresi hangi adreslerin çıkarıldığını bir
  kez söyler; denetim kaydında satırı vardır. Aynı denetim, bir adın kimde
  olduğunu ya da bir adresin nereye gittiğini değiştiren her işlemden hemen
  sonra çalışır: aboneliğin başka sahibe geçmesi, bir alan adının silinmesi
  ya da eklenmesi, bir yönlendirmenin ya da catch-all adresinin
  eklenmesi, değişmesi ya da kaldırılması, bir posta kutusunun eklenmesi;
  ayrıca her saat.
- **Sunucunun kendisinde oluşan posta** eskisi gibi "Kimden" alan adı için
  imzalanır: panelin bildirimleri (sahibin ana alan adı), sistemin kendi
  zamanlanmış işleri (root'unkiler) ve otomatik yanıtlar. Postayı bu yolla
  yalnız sistemin root ve posta hesapları teslim edebilir.
- **Gelen posta:** posta imzalayıcısı 25. porta gelen postanın imzalarını
  artık denetlemez ve ona "Authentication-Results" satırını eklemez. Rspamd
  kuruluysa onları spam puanı için yine denetler. SpamAssassin'de yalnız
  DKIM denetimi çalışıyorsa sayılır. Spam süzgeci yoksa onları hiçbir şey
  denetlemez. CelikPanel'de bu satırı kullanan hiçbir şey yoktu.
- Web postası, posta programları ve yönlendirmeler bunun dışında eskisi
  gibi çalışır.
- **Sahip için:**
  - Değişiklik, panel durdurulsa ya da kaldırılsa da çalışan olağan posta
    sunucusu ayarlarıdır.
  - CelikPanel yeni bir imza dosyasını kullanmadan önce, panelin
    açılışında da bir alan adının imzasını kurarken de, posta imzalayıcısı
    onu denetler. İmzalayıcı onunla başlamazsa önceki dosya birebir geri
    konur ve imzalayıcı onunla yeniden başlatılır.
  - Bir kesintinin yarım bıraktığı imzalayıcı yeniden başlatması sonraki
    açılışta yapılır.
  - Sahibin posta sunucusunun listesine eklediği süzgeç 25. portta da
    kalır. İmzalayıcıyı kaldırmak için ayrı bir adım yoktur: CelikPanel'in
    kendi 25. port ayarı, imzalayıcı artık kurulu değilken ya da listede
    yokken, sonraki panel açılışında ya da sonraki posta süzgeci adımında
    (bir bileşenin kurulması ya da kaldırılması) silinir.
  - Liste süzgeçlerini bir `$değişken` ya da `{ }` grubuyla adlandırıyorsa
    CelikPanel onu tek tek okuyamaz. Önceden yazdığı kendi 25. port ayarı
    varsa geçerli kalır: imzalayıcıyı dışarıda bırakır, ama listeye
    sonradan eklenen her süzgeci de. Yazmadıysa 25. port listeye uyar ve
    gelen posta hâlâ imzalanabilir. **Şu anki sorunlar** bu ikisinden
    hangisi olduğunu adımıyla söyler: süzgeçleri listeye adlarıyla yazın ve
    paneli yeniden başlatın.
  - CelikPanel kendi 25. port ayarını bir kayıt dosyasından tanır. Bu dosya
    kaybolursa, imzalayıcısız listeye eşit olan ya da yalnız kendi
    süzgeçlerinden oluşan ayarı yine tanır; daha önceki bir liste için
    yazılmış ayar o zaman sahibinkiymiş gibi korunur.
  - İmzalayıcı, Postfix adresinin nasıl yazılmasına izin veriyorsa o
    biçimde sahibin listesinde tanınır (örneğin `inet:[127.0.0.1]:8891`,
    `inet:localhost:8891` ya da `inet::8891`).
  - Sahibin elle değiştirdiği imza dosyası ya da 25. port süzgeç ayarı
    olduğu gibi kalır (CelikPanel'in metni yanında
    `opendkim.conf.celikpanel-pending` olarak durur). **Şu anki sorunlar**
    dosyayı ya da ayarı ve sahibin somut adımını söyler. Sahibin kendi imza
    dosyasında `SenderMacro {auth_authen}` satırı olduğunda (başka bir
    `SenderMacro` ve `Include` olmadan) yerinde sayılır: **Şu anki
    sorunlar**daki satır sonraki panel açılışında kalkar, panelin yanındaki
    metni silinir ve dosya sahibinin kalır.
  - **Şu anki sorunlar** bunu, girişsiz posta teslim edebilecek hesapları
    belirleyen ayar panelinki (`authorized_submit_users = root, vmail`)
    olmadığında da söyler; çünkü o zaman sitelerin programları buradaki her
    alan adı adına imzalı posta gönderebilir.
  - İmzalayıcı yeniden başlarken (birkaç saniye) iki süzgeç listesi de
    CelikPanel'inkiyse posta imzasız geçer: o zaman
    `milter_default_action = accept` yazar. Listelerden biri sahibinkiyse
    onu yazmaz; sahibin `milter_default_action` değeri, o da yoksa
    Postfix'in varsayılanı (`tempfail`) geçerlidir: gönderen bir posta
    sunucusu sonra kendiliğinden yeniden dener, 587 numaralı porttaki bir
    posta programı ise kullanıcısına hata gösterir ve kişi yeniden gönderir.

### alpha.141'den önce açılan PostgreSQL veritabanları

- Panel böyle bir veritabanını kendi başına yalnız onu başka kimsenin
  kullandığı görülmüyorsa kapatır: hakları hâlâ PostgreSQL'in dokunulmamış
  varsayılanıdır; panelin onun için kaydetmediği hiçbir hesabın içinde bir
  nesnesi, bir hakkı (tek bir sütun üzerinde bile), bir ayarı, bir ilkesi ya
  da bir kullanıcı eşlemesi yoktur; panelin onun için kaydettiği her
  kullanıcının hesabı vardır; veritabanı sunucusunda izleme ya da çoğaltma
  hesabı yoktur. Sahibi ve panelin onun için kaydettiği kullanıcılar girmeye
  devam eder; diğer her hesap artık ona bağlanamaz ve içinde geçici tablo
  açamaz.
- Böyle bir veritabanı önce izlenir: izleme başladıktan en az 24 saat
  sonraki ilk saatlik denetimde, en az 24 saatlik denetim onu okumuşsa
  kapatılır. Haklarının, içindeki hesapların, sahibinin ya da kayıtlı
  kullanıcılarının değişmesi izlemeyi yeniden başlatır. Panelin onun için
  kaydetmediği bir hesabın ona bir kez bile bağlı görülmesi izlemeyi bitirir:
  karar yöneticinindir ve hesap adıyla söylenir; kapatmanın kendisinin son
  anda bağlı bulduğu bir hesap da öyle. Bağlantılar saatlik denetimlerde
  görülür: yalnız iki denetim arasında bağlanan bir hesap görülmez.
  **PostgreSQL** sayfası her veritabanının ne zamandan beri izlendiğini ve
  bu arada saatlik bir denetimde başka bir hesabın bağlı olduğu görülmezse
  ne zaman kendiliğinden kapanacağını gösterir; yönetici onu istediği an
  kapatabilir.
- Panelin okuyabildiği diğer her böyle veritabanı olduğu gibi bırakılır ve
  **Şu anki sorunlar**da ve **PostgreSQL** sayfasında, neyin bulunduğu, her
  hesabın orada şu an ne yapabildiği (bağlanmak ya da yalnız geçici tablo
  açmak), kapatmanın neyi değiştireceği ve son okumada bağlı olan hesapları
  adlandıran bir onaydan sonra kapatan tek düğmeyle listelenir. Panelin
  okuyamadığı bir veritabanı sayfada nedeniyle birlikte okunamadı diye
  gösterilir; üst üste üçten fazla saatlik denetimde okunamazsa **Şu anki
  sorunlar**da ve uyarı postasında da nedeni ve atılacak adımla okunamadı
  diye listelenir. Süresi biten bir okuma, diğer veritabanlarından
  okuduğunu korur.
- Bir kapatma, veritabanını tutan başka bir oturumu en çok 30 saniye bekler
  (ve hiçbir komutu 60 saniyeden uzun sürmez); veritabanının sahibi onun
  için ne ayarlamış olursa olsun. Ajan, 3 dakika sonra hâlâ çalışan bir
  kapatmayı ya da geri almayı sonlandırır; yanıtı o zaman sonlandırıldığı
  zamanla birlikte "bilinmiyor" olur. Yanıtı kaybolan ya da bilinmeyen bir
  kapatma veya geri alma hiçbir zaman "hiçbir şey değişmedi" sayılmaz: panel
  kaydını korur, hiçbir şeyi yeniden göndermez ve veritabanını ancak ajan o
  kapatmanın ya da geri almanın hiçbir parçasının sunucuda hâlâ çalışmadığını
  doğruladıktan sonra okur; böylece veritabanı kapalı olabildiği sürece bir
  kapatmanın kaydı asla silinmez ve veritabanı kapalıyken **Geri al** her
  zaman vardır. Başarısız bir kapatma nedeni, zamanı ve atılacak adımla
  gösterilir; her saat yeniden denenmez, yalnız veritabanında bir şey
  değişince ya da yönetici Kapat'a basınca denenir. Aynı veritabanının iki
  kapatması asla aynı anda çalışmaz; bir veritabanının kapatması başka bir
  veritabanının kapatmasını da **Yeniden oku**yu da bekletmez.
- Yöneticinin bir müşterinin veritabanı içindeki oturumları, veritabanının
  sahibi onun için ne ayarlamış olursa olsun PostgreSQL'in kendi
  varsayılanlarını korur (zaman sınırları, salt okunur kip, oturumun hangi
  rolle çalıştığı, bir hatada oturumun kapanması); bu alpha.141'in geri
  yükleme ve kopyalama işleri için de geçerlidir.
- Her kapatma ne zaman ve kim tarafından yapıldığını, **Geri al** düğmesini
  (ve panel olmadan geri almak için düz komutları) gösterir; geri alma,
  kapatmanın değiştirdiğini tam olarak geri verir. Yöneticinin yeniden
  açtığı bir veritabanı sayfada yöneticinin tercihi olarak kalır, **Şu anki
  sorunlar**dan ve uyarı postasından çıkar ve bir daha kendiliğinden
  kapatılmaz.
- Sayfa son okumayı zamanıyla gösterir; **Yeniden oku** bir veritabanı
  sunucusunun bütün veritabanlarını tek bağlantıda okur.
- Panel bir veritabanını kendiliğinden kapattıktan hemen sonra sayfa,
  **Yeniden oku** gerekmeden o kapatmayı gösterir (panel, zamanı, **Geri
  al**). Başarısız bir kapatma yalnız hiçbir şeyin değişmediğini, nedenini
  sayfanın dilinde ve veritabanı sunucusunun kendi iletisini tırnak içinde
  söyler; "bilinmiyor" yalnız gerçekten bilinmediğinde söylenir. Sahibi için
  yazılmış varsayılan haklar (geri almanın bıraktığı gibi) her yerde
  varsayılan sayılır; böylece yeniden açılan bir veritabanına "CelikPanel
  dışında değiştirilmiş" denmez.
- MariaDB'de böyle bir varsayılan yok: orada hiçbir şey değişmez. Başka
  makinelerde kayıtlı PostgreSQL sunucularına dokunulmaz.

### alpha.142 laboratuvarından düzeltmeler

- **Kendi alan adınızın içinde bir alt alan adı.** Alan adı sunucunun kendi
  adlarından birinin ya da yöneticinin tuttuğu bir adın (örneğin ana alan
  adı) altında olan bir müşteri, o alan adının içinde yeniden alt alan adı
  ve takma ad ekleyebilir. Ad kuralının reddettiği diğer her şey reddedilmeye
  devam eder: sunucunun ya da yöneticinin adının kendisi, onun üstündeki bir
  ad, müşterinin kendi alan adının dışında onun altındaki bir ad ve
  müşterinin alan adının içindeki bir sunucu adı (örneğin sunucunun posta
  adresi). Başka müşterilerin adlarına bu yolla asla ulaşılmaz. Böyle bir
  alan adı ve altında açılan bir alt alan adı, **Şu anki sorunlar**da hâlâ
  sunucunun adlarından biriyle çakışıyor diye listelenir; listelenmeleri
  gerekip gerekmediği sahibe sorulmuş açık bir sorudur.
- **Yöneticiye en yakın sahip söylenir.** Yönetici başka bir hesaba ad
  yerleştirmekten alıkonduğunda cümle, sunucunun uzak bir adını değil, o
  hesabın en yakın adını ve sahibini söyler. Bir müşterinin ya da bayinin
  cümlesi değişmez ve her neden için aynıdır.
- **Uygulaması hata veren bir Node.js sitesi** tek basışta silinir. Önceden
  silme "tamamlanmadı" diyordu ve root olarak `systemctl reset-failed`
  çalıştırılana kadar alan adı "silinmeyi bekliyor" durumunda kalıyordu.
- **Korunan dosya kipleri adlandırılır.** alpha.142'nin tek seferlik kip
  düzeltmesi artık sahibin seçip kendisinin koruduğu her dosya kipini
  aracının günlüğünde adlandırır ve sayar (önceden daha sıkı bir kip için
  "0 korundu" diyordu). Önceden değiştirmediği hiçbir kipi değiştirmez.

## Bilinen sınırlar

- Arch Linux'un OpenDKIM paketi imzayı girişe bağlayamaz. Orada girişle
  gönderilen posta, "Kimden" satırında geçen barındırılan her alan adı için
  hâlâ imzalanır. 25. port orada da imzaya kapalıdır. **Şu anki sorunlar**
  bunu söyler ve oradaki OpenDKIM'in reddettiği bir ayarı asla önermez.
- Otomatik yanıtın ek adresi, panelin bildiği bir yönlendirme olmalıdır.
  Sunucu sahibinin posta sunucusunun tablolarına elle yazdığı takma ad
  reddedilir.
- Yanıtlar panelin kayıtlarından düzeltilir. Panel kaydı olmayan bir
  CelikPanel yanıt betiği yeniden yazılmaz.
- Otomatik yanıt, kutusundan bir adresi alan değişiklikten (abonelik
  taşındı, alan adı silindi, yönlendirme kaldırıldı) hemen sonra yeniden
  ayarlanır: o değişiklikle yanıtın yeniden ayarlanması arasındaki birkaç
  saniye boyunca önceki yanıt hâlâ verilebilir. Sunucu yanıtı yeniden
  ayarlayamaz ya da kapatamazsa (Agent yanıt vermiyor ya da başka bir
  sürümde), başarılı olan sonraki saatlik denemeye dek yanıt verilmeye
  devam eder; bu arada **Şu anki sorunlar** posta kutusunu ve adımı
  adlandırır.
- Posta sunucusunun süzgeç listesi süzgeçlerini bir `$değişken` ya da
  `{ }` grubuyla adlandırıyorsa, CelikPanel'in önceki 25. port ayarı
  geçerli kaldıkça listeye sonradan eklenen bir süzgeç 25. porta gelen
  postayı görmez (yukarıda "Sahip için").
- **Bir veritabanını başka bir PostgreSQL sürümüne taşımak** (yönetici
  işlemi: yalnız yönetici başlatabilir), alpha.141'de olduğu gibi, taşıma
  sırasında içindeki müşterilerin veritabanı kodunu hâlâ yöneticinin
  haklarıyla çalıştırır. Sonraki bir sürüm bunu düzeltene dek sahibine
  güvenmediğiniz bir veritabanını taşımayın.
- Panelin içinde kendi başına karar vermediği bir şey bulduğu bir PostgreSQL
  veritabanı, yönetici onu PostgreSQL sayfasında kapatana kadar başka
  hesaplara açık kalır; kendi başına kapatabileceği bir veritabanı izlendiği
  gün boyunca açık kalır. Panelin kaydetmediği veritabanlarına (örneğin elle
  açılanlar) dokunulmaz. Kapatma yalnız PostgreSQL 17'de denendi.
- Oturumlar saatlik denetimlerde görülür: izlenen bir veritabanına yalnız
  iki denetim arasında bağlanan bir hesap görülmez ve veritabanı günü
  dolunca kendiliğinden kapatılabilir. Bir veritabanı sahibinin
  ayarlayabileceği boşta oturum ve işlem zaman sınırları (PostgreSQL 14 ve
  17) panelin kapatma oturumlarında bağlantı seçeneği olarak değil, bağlanır
  bağlanmaz sıfırlanır (eski bir sunucu tanımadığı bir seçeneği reddederdi).
- Bir müşterinin kendi alan adının içinde, bir sunucu adının altında açılan
  alt alan adı, üst alan adı gibi **Şu anki sorunlar**da listelenir
  (yukarıda). Yöneticinin bir müşteriye sunucu adının altında yeni bir ad
  vermesi bu sürümün kapsamında değil.
- Şablonları SQL_ASCII olan bir PostgreSQL sunucusu hâlâ laboratuvarda
  denenmedi.
- alpha.142'den açık kalanlar: web sunucusunun grubu hâlâ her sitenin
  klasörünü okuyabilir (her site kendi grubunu alana kadar siteler
  arasındaki engel bağlantı kuralıdır); her sitenin hesabı sistemin hesap
  listesinde görünür ve adı alan adından türetilir; postanın adres
  eşlemeleri her hesapça okunabilir; PHP ve zamanlanmış görevler site
  başına kum havuzunun ve sınırlarının dışında çalışır; çalışan süreçler
  her hesaba görünür; bir Git deposundan yayımlanan site sunucunun kendi
  adreslerinden birinden kopyalayabilir; uzak yedek hedefini denemek
  sunucunun ağı içindeki adreslere ulaşabilir.
- Laboratuvar altı sınır daha buldu; her biri için alpha.144'e dek
  yapılacak olan aşağıda, "Laboratuvarda bulunan, alpha.144'e planlananlar"
  altında.

## Laboratuvarda denenenler

Laboratuvar denemesi bitti: bir test sunucusu çiftinde iki deneme,
PowerDNS'li Debian 13 ana DNS sunucusu ve BIND'li Ubuntu 24.04 yedek DNS
sunucusu. Her deneme alpha.142 durumundan başladı ve sahibin yolunu izledi:
iki sunucu da panelin kendi güncelleme sayfasından güncellendi. Birinci
deneme (21 Kasım 2026, UTC 22:30'dan toparlamayla birlikte 22 Kasım
yaklaşık 01:40'a) sürümü ilk düzeltme turundan sonraki hâliyle denedi
(`c012e3eac`, yalnız laboratuvara ait v0.1.0-alpha.150 etiketiyle). Dört
görünüm ve ifade kusuru ile kalkmayan bir durum buldu; ikinci düzeltme turu
bunları düzeltti. İkinci deneme (22 Kasım 2026, UTC 03:38'den yaklaşık
05:24'e) son kodu denedi (`1d3d8801a`, yalnız laboratuvara ait
v0.1.0-alpha.151 etiketi) ve beşinin de düzeldiğini buldu. İki tarafı, aynı
bayinin ayrı hesaplardaki iki müşterisi oynadı: alice ve onun adına posta
göndermeye ve veritabanlarına ulaşmaya çalışan bob. Laboratuvarın yaptığı
yazılanlar dışında her şey sayfanın kendi düğmeleriyle yapıldı. İki
denemede de ağır hata yok. Aşağıdakiler son kodda görüldü; "(birinci
deneme)" yazanlar yalnız birinci denemede, ikinci düzeltme turundan önceki
kodda görüldü.

- **Güncellemeden önce yollar açıktı** (alpha.142'nin kodu, birinci
  deneme): bob'un posta kutusu girişiyle, "Kimden" satırında alice'in
  adresiyle gönderdiği posta onun alan adı için imzalandı
  (`d=kahvedukkani.dr.test`) ve imza doğrulandı; sitesinin sunucunun kendi geri döngü adresinde 25.
  porta girişsiz bıraktığı posta da onun alan adı için imzalandı; otomatik
  yanıtı onun adresiyle kabul edildi ve o adresten onun imzasıyla yanıt
  verdi. İkinci denemede eski kod o yanıtı yine kabul etti.
- **İki sunucu da panelin kendi güncelleme sayfasından kendiliğinden
  güncellendi**, her biri yaklaşık üç dakikada (birinci denemede yaklaşık
  iki dakikada). Şema 80 → 81, altı ad kuralı yerindeyken uygulandı; her
  sunucuda bir panel ve bir aracı çalıştı; yeni hatalı hizmet yok; web,
  posta, DNS (iki sunucudan aynı yanıtlar) ve veritabanları yanıt verdi.
- **Başka bir müşterinin alan adıyla imzalanan posta yok.** Güncellemeden
  sonra:
  - bob'un girişiyle, "Kimden" satırında alice'in adresiyle gönderdiği
    posta yalnız kendi alan adı için imzalandı (`d=pastane.dr.test`);
  - sitesinin 25. porta girişsiz bıraktığı posta, geri döngü adresinde de
    genel adreste de imzalanmadı;
  - onun adresinden yanıt verecek biçimde ayarlanan yanıtı ilk açılışta o
    adresi kaybetti (bir günlük satırı ve bir denetim kaydı satırı);
    penceresi bunu bir kez söyledi ve onun adresini yeniden kaydetmek,
    kimin olduğunu söylemeyen bir cümleyle reddedildi (Türkçe ve
    İngilizce);
  - iki müşterinin olağan postası kendi alan adları için imzalandı ve
    doğrulandı; onun yönlendirmesinden geçen sahte bir ileti eklenmiş bir
    imza almadı (birinci deneme).
- **Yanıtlar ad değişikliklerini izler.** bob'un kendi yönlendirmesinden
  yanıt veren yanıtı kabul edildi ve kendi alan adı için imzalı yanıt
  verdi. O yönlendirmeyi kaldırdığında ve alice yanıtının adlandırdığı ikinci
  bir alan adını sildiğinde yanıt aynı istek içinde yeniden yazıldı ve
  penceresi hangi adresin çıkarıldığını söyledi. Posta deposu hizmeti
  durmuşken yönlendirmeyi kaldırmak yanıtı kapattı; penceresi bunu ve
  yeniden kaydetmeyi söyledi.
- **Sahibin kendi imza dosyası.** Sahibin değiştirdiği imza dosyası
  birebir korundu; panelin metni yanında, **Şu anki sorunlar**da sahibin
  adımıyla bir satır. Sahip kendi dosyasına giriş ayarını
  (`SenderMacro {auth_authen}`) ekleyince satır sonraki panel açılışında
  kendiliğinden kalktı ve panelin yanındaki dosyası silindi; sahibin dosyası
  kaydettiği gibi kaldı, sonradan yeni bir alan adı imza anahtarını
  aldığında da. Laboratuvar, alpha.142'nin kodunun yeni bir alan adı
  anahtarını aldığında sahibin değiştirdiği imza dosyasını değiştirdiğini
  de gördü; bu sürüm sahibin dosyasını o yolda da olduğu gibi bırakır. Açık
  bir "posta kimler teslim edebilir" ayarı (`authorized_submit_users`) ve
  başka bir programın tuttuğu imza portu, her biri **Şu anki sorunlar**da
  açıkça söylendi; her satır düzeltilince kalktı. 25. port, sahibin posta
  sunucusunun listesine eklediği bir süzgece uydu (birinci deneme).
- **Eski PostgreSQL veritabanları.**
  - PostgreSQL sayfası izlenen her veritabanını izlemenin başladığı tarihle
    ve ne zaman kendiliğinden kapanacağıyla gösterdi (birinci deneme).
  - İzleme günü boyunca hiçbir şey kapatılmadı. O gün boyunca başka bir
    müşterinin veritabanı hesabı onlara hâlâ bağlanabildi.
  - Panelin kaydetmediği bir rolün bağlı görülmesi izlemeyi bitirdi ve
    sayfa onu adıyla söyledi (birinci deneme).
  - İzleme kaydı laboratuvarca bir gün geri alınınca (yazılı tek yazma)
    sonraki saatlik denetim veritabanını kapattı: diğer müşterinin hesabı
    reddedildi, veritabanının kendi kullanıcıları girdi ve sayfa kapatmayı
    (panel, zamanı, **Geri al**) **Yeniden oku** gerekmeden gösterdi. **Geri
    al** önceki hakları geri verdi, sayfa onlara "CelikPanel dışında
    değiştirilmiş" demedi ve sonraki denetim veritabanını yeniden
    kapatmadı.
  - Sahip veritabanının satırını tutarken yapılan kapatma yaklaşık yarım
    dakika sonra "hiçbir şey değiştirilmedi" ve veritabanı sunucusunun
    iletisiyle yanıtlandı; geride istemci kalmadı ve yeniden gönderilmedi.
  - Başarısız bir kapatma nedenini sayfanın dilinde, veritabanı sunucusunun
    iletisini tırnak içinde söyledi.
  - Sahibin kendi veritabanı ayarları (salt okunur kip, kendi rolü, kilit
    beklememe) okumayı, izlemeyi, kapatmayı ve geri almayı engellemedi.
  - Silinen bir veritabanının kaydı onunla birlikte gitti.
  - Güncellemeden sonra açılan bir veritabanı baştan başka hesaplara
    kapalıydı (birinci deneme).
- **Düzeltmeler.** alice'in sunucunun bölgesi altındaki kendi alan adının
  içinde bir alt alan adı açıldı ve iki sunucudan da yanıt verdi (birinci
  deneme). bob denediği her adda (sunucunun bölgesi, onun altında yeni bir
  ad, panelin ve posta sunucusunun adları, alice'in takma adının altında
  bir ad) kimseyi adlandırmayan aynı cümleyle reddedilmeye devam etti. Başka
  bir hesaba ad yerleştirmesi reddedilen yöneticiye en yakın sahip
  söylendi (birinci deneme). Uygulaması hata veren bir Node.js sitesi tek
  basışta silindi; hatalı hizmeti de onunla gitti.
- İki sunucuda da olağan sayfalar hatasız açıldı.

**Laboratuvarın sayfanın yolundan ayrıldığı yerler.** Laboratuvar,
laboratuvar sunucularında root olarak, posta sunucusunun ve imza hizmetinin
dosyalarını bir sahibin yapacağı gibi değiştirdi (imza dosyasına önce bir
yorum satırı, sonra giriş ayarı; "posta kimler teslim edebilir" ayarı; ek
bir 25. port süzgeci), bekleyen bir yeniden başlatma işaretini elle koydu
(birinci deneme), posta deposu hizmetini ve imza hizmetini durdurup
başlattı, imza portunda sahte bir dinleyici çalıştırdı, 25. porta posta
bırakmak için bob'un sitesine küçük bir posta betiği koydu, dışarıdan
deneme postası gönderdi ve sertifika aracını laboratuvarın kendi sertifika
yetkilisine kaydetti. Veritabanı yöneticisi olarak veritabanlarını
alpha.141'den önce açılanların durumuna getirdi (PostgreSQL'in varsayılan
hakları), sütun hakkı olan bir raporlama rolü ekledi ve kayıtlı bir
kullanıcının rolünü sildi (birinci denemede sonra yeniden oluşturdu); bir
veritabanının sahibi olarak kendi veritabanı ayarlarını yaptı ve bir
işlemi açık tuttu. Saatlik bir denetimi öne almak için paneli yeniden
başlattı. Her denemede bir kez, panel durmuşken, laboratuvar panelinin
veritabanına bir izleme kaydını bir gün geri alan tek bir satır yazdı.
Yöneticinin başka bir hesaba ad yerleştirmesi doğrudan panelin API'sine
gönderildi (birinci deneme).

**Denetlenmeyenler:**

- Web postası ve veritabanı web araçları: laboratuvar çiftinin web postası
  adresi yok ve araçlar kurulu değil (alpha.142'den taşındı, hâlâ açık).
- Bir yanıt alan adını adlandırırken aboneliği başka bir hesaba taşımak:
  sayfa bunu yalnız bir istek ve onay olarak sunuyor; testlerle kapsanıyor.
- Alan adının imza anahtarı olmayan bir posta kutusu girişi: her alan adı
  bir anahtar alıyor.
- Bir panel bildiriminin imzası: laboratuvarda bir bildirim göndermenin
  yolu bulunamadı; testlerle kapsanıyor.
- Sahibi kendi veritabanı ayarlarını yapmış bir veritabanının yedeklenmesi
  ve geri yüklenmesi: süre.
- Bir kapatmanın kaydından hemen sonra veritabanı istemcisini sonlandırmak:
  denenmedi; motor testleriyle kapsanıyor.
- Bir yanıt başka bir müşterinin adresini adlandırırken ilk açılışta
  aracının durmuş olması: panelin veritabanına ikinci bir yazma olmadan
  hazırlanamadı.
- Korunan dosya kipinin günlük satırı: tek seferlik düzeltme bu sunucularda
  zaten çalışmıştı.
- Sağlıklı bir Node.js sitesinin açılıp silinmesi: süre.
- Laboratuvar sunucusunun 17'si dışındaki PostgreSQL sürümleri ve
  şablonları SQL_ASCII olan bir sunucu.
- Bir veritabanını başka bir PostgreSQL sunucu sürümüne taşımak: bu sürümün
  parçası değil.

### Laboratuvarda bulunan, alpha.144'e planlananlar

Hiçbiri bir güvenlik ya da veri kaybı sorunu değil.

1. Bir müşterinin kendi alan adının içindeki takma ad oluşturuluyor, ama
   laboratuvar çiftinde DNS kaydı almadı: iki sunucudan da yanıt vermedi.
   alpha.143'ten önceki sürümlerin de böyle yapıp yapmadığı belirlenmedi.
2. Yönetici bir kapatma sürerken PostgreSQL sayfasından ayrılırsa, kapatma
   sunucuda bittiği hâlde panel yanıtı kaybolmuş diye kaydediyor: sayfa
   "gönderildi, henüz doğrulanmadı" diyor ve kapatma yeniden gönderilmiyor.
   Veritabanı hiçbir zaman **Geri al** düğmesi olmadan kapalı gösterilmez.
3. Başarısız bir kapatmanın penceresi "sunucuda çalışılıyor" adımını kırmızı
   bir işaretle tutuyor ve sayfanın dilindeki nedeni genel ("veritabanı
   sunucusu bunu yapmadı"); gerçek neden, veritabanı sunucusunun tırnak
   içindeki iletisi.
4. Bir ret (sertifika adımı başarısız olan bir takma ad) Türkçe sayfada
   İngilizce çıkıyor.
5. İngilizce **Problems now** satırı "1 PostgreSQL databases" diyor (tek
   veritabanı için çoğul).
6. Bu sürümden eski; birinci denemede alpha.142'nin kodunda görüldü ve son
   kodda yeniden denenmedi: bir posta kutusu oluşturulurken sayfadan
   ayrılmak, posta kutusunu sunucuda panelde satırı olmadan bırakabiliyor;
   onu yeniden oluşturmak o zaman bir iç hatayla yanıtlanıyor. Bugünkü
   çıkış yolu: yönetici posta kutusunu sunucuda kaldırır ya da onu başka
   bir adla oluşturur.

### Güncellemeden sonra ne beklenir

- Güncelleme sunucu başına yaklaşık üç dakika sürer ve bittiğini kendisi
  söyler.
- Yapı şema 81'dir: alpha.142'ye dönmek, güncellemeden önce alınan anlık
  görüntüyü ya da yedeği geri yüklemek demektir.
- Posta kutusu girişiyle gönderilen posta yalnız kendi alan adı için
  imzalanır; 25. porta girişsiz bırakılan posta (örneğin bir web sitesinin)
  artık imzalanmaz.
- Kendisinin olmayan bir adresi adlandıran otomatik yanıt, ilk açılışta o
  adresi kaybeder ve penceresi hangi adresin çıkarıldığını bir kez söyler;
  yeniden ayarlanamazsa kapatılır ve penceresi yeniden kaydetmeyi söyler.
- alpha.141'den önce açılan ve başka kimsenin kullandığı görülmeyen bir
  PostgreSQL veritabanı, izlemesi başladıktan en az 24 saat sonraki ilk
  saatlik denetimde, en az 24 saatlik denetim onu okumuşsa kendiliğinden
  başka hesaplara kapatılır; o zamana dek PostgreSQL sayfası ne zaman
  kapanacağını gösterir ve başka hesaplar ona hâlâ bağlanabilir. Diğerleri
  yöneticiye **Şu anki sorunlar**da ve PostgreSQL sayfasında, her biri onu
  kapatan tek düğmeyle listelenir.
- Sahibin elle düzenlediği imza dosyası korunur; **Şu anki sorunlar**daki
  bir satır, giriş ayarı dosyaya girince kendiliğinden kalkar.
- Alan adı bir sunucu adının ya da yöneticinin bir adının altında olan bir
  müşteri yeniden onun alt alan adlarını ekleyebilir.

## Kanıt

Şema 080 → 081: göç 081, 080'in altı tetikleyicisini kaldırıp aynı adlarla,
kendi alt alan adı istisnasıyla yeniden oluşturur; tablo, sütun ya da satır
yok. Önceden çakışma tutan bir veritabanı da geçer ve çalışmayı sürdürür
(`TestSchema80DatabaseWithServerZoneOverlapsUpgradesAndLetsOwnSubdomainsIn`);
eski bir sürüm 081'i taşıyan kaydı reddeder
(`TestOlderReleaseRefusesALedgerWithTheOwnSubdomainEntry`). Sürüm kurtarma
laboratuvarı şema 81'i sabitler. Yeni ve hepsi eklemeli: posta süzgeci
yanıtının `signing` alanı, "Şu anki sorunlar"da iki tür (`mail_signing`,
`postgresql_databases_open`), sürüme bağlı aracı çağrıları
`PostgreSQLPublicAccessScanV1`, `CloseV1` ve `UndoV1` (`unknown`, `missing`
ve `replication` yanıtlarıyla), panel ayar satırları
`postgresql_public_v1:<veritabanı kimliği>` ve `postgresql_public_list_v1`.

Kapanan her yolu deneyen, önceki kodda başarısız olup bu sürümün kodunda
geçen testler:

- Posta: `TestMailSigningKeyFollowsTheLoginNotTheFromHeader`; 25. port süzgeç
  listesinin, yazma sırasının, sahibin korunan ayarlarının, başarısız bir
  yazma ya da yeniden yüklemede geri almanın, kullanmadan önce denetimin ve
  seçeneksiz bir derlemenin bileşen testleri
  (`cmd/agent/mail_signing_bind_test.go`). Dağıtım yapılandırma matrisinde
  Debian 12, Debian 13, Ubuntu 22.04 ve Ubuntu 24.04'ün gerçek OpenDKIM'i,
  `a` girişli ve `b` "Kimden"li postayı `a` olarak, `a` girişli boş
  göndericiyi `a` olarak, girişsiz yerel postayı "Kimden" alan adıyla
  imzaladı ve kimliği doğrulanmamış dış istemciye imza vermedi; Arch'ta
  önceki yapılandırma kabul edilir. Bağımsız incelemenin durumları:
  `TestMailAutoresponderRefusesAnotherAccountsAddress`,
  `TestAutoresponderAgentRefusesAnotherAccountsAddress`,
  `TestMailAutoresponderStoredForeignAddressIsRemovedAtStart`,
  `TestMailSigningLocalSubmitDecidesTheState`,
  `TestMailSigningPort25FollowsTheOwnersLaterMainList`,
  `TestMailSigningPort25SettingGoesWithOpenDKIM`,
  `TestMailSigningOwedRestartIsMadeAtTheNextStart`,
  `TestMailSigningUnknownReplacesAnEarlierInPlace`,
  `TestMailSigningUnsupportedOwnerIsNeverAdvisedTheLine`,
  `TestMailSigningRestartWindowIsSaid` ve
  `TestMailSigningChainIsWrittenBeforeOpenDKIMRestarts`; matriste, sürümün
  gerçek Postfix araçlarıyla, sahibin sonradan eklediği 25. port süzgeci,
  birebir geri alma ve sahibin `authorized_submit_users` ayarı; her imza
  durumunun sözleri `web/tests/mail-signing-a143-fix.test.mjs` içinde.
  İkinci incelemenin posta durumları:
  `TestMailAutoresponderTransferRewritesTheReplyInTheSameRequest`,
  `TestMailAutoresponderForwarderRemovalRewritesTheReplyInTheSameRequest`,
  `TestMailAutoresponderRewriteThatCannotBeMadeIsNotSilent`,
  `TestMailAutoresponderReconcileDoesNotOverwriteASaveMadeMeanwhile`,
  `TestMailSigningVariableListStatesAndForeignReplyAreWorded`,
  `TestMailSigningVariableListKeepsCelikPanelsValueAndSaysSo`,
  `TestMailSigningPort25ValueIsRecognisedWithoutTheRecord`,
  `TestMailSigningOpenDKIMEndpointSpellings` ve
  `TestMailSigningConfigureDKIMSigningChecksAndPutsBack`; matris durumu
  `postfix.mail-signing-variable-list` (eklendi, henüz çalıştırılmadı).
  Ayrıca yeni ve eklemeli: "Şu anki sorunlar" türü
  `mail_autoresponder_foreign`, imza durumları `port25_variable` ve
  `port25_variable_own`, panel ayar satırları
  `mail_autoresponder_foreign_v1:<hesap kimliği>`.
- PostgreSQL, ikinci incelemenin durumları: özel bir PostgreSQL 17.12
  sunucusunda `TestS1bRound2ACloseThatWaitsIsEndedAndLeavesNothingHalfDone`
  (ikinci bir oturum veritabanının satırını tutar: ajanın süre sınırı
  kapatmayı "bilinmiyor" diye sonlandırır ve çalışma denetimi o bitene dek
  onu görür; kapatmanın kendi lock_timeout değeri, sahip ne ayarlamış olursa
  olsun onu sonlandırır; iki seferde de hiçbir şey değişmez),
  `TestS1bRound2TheOwnersDatabaseSettingsDoNotReachTheAdministratorSession`
  ve `TestS1bRound2TheClosesSessionGuardNamesTheRoleItCaught`; motor olmadan
  `TestPGPublicCloseAndUndoBoundThemselvesBeforeTheyCanWait`,
  `TestPGPublicAClientEndedAtItsDeadlineIsUnknown`,
  `TestPGPublicTheScanAsksWhichDatabasesExistBeforeEnteringAny`,
  `TestPGPublicAScanWhoseTimeRanOutKeepsWhatItRead`,
  `TestPGPublicAClientIsFoundByItsNameAmongTheProcesses` ve
  `TestAdministratorSessionPinsWhatADatabaseOwnerCouldSet`; panelin
  `TestPGPublicACloseThatMayStillRunStaysSendingAndIsNeverSentAgain`,
  `TestPGPublicAnUndoWhoseAnswerIsLostKeepsTheCloseUntilItIsConfirmed`,
  `TestPGPublicAWatchHoldingASentCloseFoundClosedLaterIsThatClosesRecord`,
  `TestPGPublicOneDatabasesCloseDoesNotHoldUpOthers`,
  `TestPGPublicARoundSkipsARecordChangedAfterItsScanBegan`,
  `TestPGPublicASessionTheClosesGuardCaughtIsRecordedAndEndsTheWatch`,
  `TestPGPublicADatabaseUnreadForMoreThanThreeRoundsIsListed` ve
  `TestPGPublicTheRecordOfARemovedDatabaseIsRemoved`; alpha.143
  laboratuvarının `TestMailSigningOwnersFileWithTheLoginSettingIsInPlace`,
  `TestPGPublicThePageShowsARoundsOwnCloseRightAway`,
  `TestPGPublicTheDefaultRightsWrittenOutAreTheDefault` testleri ve
  `TestPGPublicAnUnknownAnswerKeepsTheRecordAndReadsTheEngine` ile
  `TestS1bFixARecordedUserWithoutItsRoleIsSeenAndItsGrantIsNotNothingChanged`
  içindeki doğrulanmış hata durumları. Ayrıca yeni ve
  eklemeli: "Şu anki sorunlar" türü `postgresql_databases_unreadable`,
  sürüme bağlı aracı çağrısı `PostgreSQLPublicAccessRunningV1` ve kapatmanın
  `sessions` yanıtı.
- PostgreSQL, özel bir PostgreSQL 17.12 sunucusunda:
  `TestS1bClosingAnOldDatabaseKeepsItsUsersAndRefusesOthers` ve bağımsız
  incelemenin durumları
  `TestS1bFixACloseWhoseAnswerIsLostAfterCommitIsUnknown`,
  `TestS1bFixColumnRightsSettingsPoliciesAndMappingsCountAsInside` ve
  `TestS1bFixARoleNamedToBreakOutIsCarriedSafely` (bu üçü değişikliğin ilk
  sürümünde de başarısız olur),
  `TestS1bFixARecordedUserWithoutItsRoleIsSeenAndItsGrantIsNotNothingChanged`,
  `TestS1bFixAReplicationSubscriberIsSeen` ve
  `TestS1bFixAScanOf300DatabasesIsOneSession` (tek bağlantı, yaklaşık 5 sn);
  panelin kararları (bir günlük izleme, bilinmeyen yanıt, aynı anda iki
  kapatma, başarısız kapatma, yeniden açılan veritabanı, son okuma)
  `cmd/panel/postgresql_public_access_test.go` içinde; sayfa
  `web/tests/pg-public-access-s1b.test.mjs` içinde.
- Düzeltmeler: `TestOwnSubdomainUnderAServerNameRuleAndTriggersAgree`,
  `TestOwnSubdomainUnderAnAdministratorsNameRuleAndTriggersAgree`,
  `TestOwnSubdomainUnderTheServersZoneIsCreated`,
  `TestNameRuleAdministratorIsToldTheNearestHolder`,
  `TestAppUnitRemovalForgetsAFailedUnit`,
  `TestTenantIsolationEarlierFilesAreTightenedOnce`,
  `TestTenantIsolationModesAreTightenedOnceThenLeftToTheOwner`.

Testler, test makinesinde her paketin tam takımı, önce (alpha.142) ve
sonra:

| Paket | Önce | Sonra |
|---|---|---|
| `internal/transport` | 62 | 63 |
| `internal/db` | 182 | 184 |
| `internal/services` | 371 | 371 |
| `cmd/agent` | 5854 | 5920 |
| `cmd/panel` | 4726 | 4763 |

(Geçen testler ve alt testler. "Sonra" sütunu 22 Kasım 2026'da ikinci
düzeltme turunun son hâlinde (`14b7caf11`), özel PostgreSQL 17.12 ve
MariaDB 11.8 sunucuları bağlıyken ölçüldü; veritabanı testleri paket
takımlarının içinde çalıştı. alpha.143 adayı aynı gün aynı biçimde 63, 184,
371, 5906 ve 4749 verir. Bu test makinesinde `cmd/agent` paketinin 94 test
ve alt testi (19 test) ikisinde de aynı biçimde, makinenin kendisi yüzünden
başarısız: DNS motoru durumu PowerDNS'e yetki vermiyor ve geçici klasörleri
root grubuna ait değil. Son hâlin çalışmasında değişiklikten kaynaklanmayan
üç başarısızlık daha oldu: makinenin /run durumu değiştikten sonra adayda da
başarısız olan iki kurtarma paketi ve tam çalışmada süresi dolan, tek başına
geçen bir panel geri yükleme testi. Bir aracı sözleşme testi yeni
`PostgreSQLPublicAccessRunningV1` çağrısını sürüm kapısı listesinde eksik
buldu; çalışmadan sonra eklendi ve geçer. Şablonları SQL_ASCII olan bir
sunucu isteyen iki veritabanı testi atlanır.)

Yeni başarısızlık yok. Web: `npm test` 3090 → 3105 geçti (ikinci düzeltme turundan önce 3102), 0 başarısız; tür
denetimi temiz ve derleme her boyut bütçesini karşılıyor (ilk yükleme yolu
sıkıştırılmış 110,00 KiB'nin 109,36 KiB'i; alpha.142: 109,37).
