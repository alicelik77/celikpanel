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
    dosyayı ya da ayarı ve sahibin somut adımını söyler.
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
  karar yöneticinindir ve hesap adıyla söylenir. **PostgreSQL** sayfası her
  veritabanının ne zamandan beri izlendiğini ve başka kimse bağlanmazsa ne
  zaman kendiliğinden kapanacağını gösterir; yönetici onu istediği an
  kapatabilir.
- Diğer her böyle veritabanı olduğu gibi bırakılır ve **Şu anki sorunlar**da
  ve **PostgreSQL** sayfasında, neyin bulunduğu, her hesabın orada şu an ne
  yapabildiği (bağlanmak ya da yalnız geçici tablo açmak), kapatmanın neyi
  değiştireceği ve son okumada bağlı olan hesapları adlandıran bir onaydan
  sonra kapatan tek düğmeyle listelenir.
- Yanıtı kaybolan bir kapatma hiçbir zaman "hiçbir şey değişmedi" sayılmaz:
  panel kaydını korur ve başka bir şey yapmadan önce veritabanını yeniden
  okur. Başarısız bir kapatma nedeni, zamanı ve atılacak adımla gösterilir;
  her saat yeniden denenmez, yalnız veritabanında bir şey değişince ya da
  yönetici Kapat'a basınca denenir. Aynı veritabanının iki kapatması asla
  aynı anda çalışmaz.
- Her kapatma ne zaman ve kim tarafından yapıldığını, **Geri al** düğmesini
  (ve panel olmadan geri almak için düz komutları) gösterir; geri alma,
  kapatmanın değiştirdiğini tam olarak geri verir. Yöneticinin yeniden
  açtığı bir veritabanı sayfada yöneticinin tercihi olarak kalır, **Şu anki
  sorunlar**dan ve uyarı postasından çıkar ve bir daha kendiliğinden
  kapatılmaz.
- Sayfa son okumayı zamanıyla gösterir; **Yeniden oku** bir veritabanı
  sunucusunun bütün veritabanlarını tek bağlantıda okur.
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

## Laboratuvarda denenenler

**AÇIK.** Henüz laboratuvar çalışması yok.

### Güncellemeden sonra ne beklenir

- Güncelleme birkaç dakika sürer ve bittiğini kendisi söyler.
- Yapı şema 81'dir: alpha.142'ye dönmek, güncellemeden önce alınan anlık
  görüntüyü ya da yedeği geri yüklemek demektir.
- Posta kutusu girişiyle gönderilen posta yalnız kendi alan adı için
  imzalanır; 25. porta girişsiz bırakılan posta (örneğin bir web sitesinin)
  artık imzalanmaz.
- Kendisinin olmayan bir adresi adlandıran otomatik yanıt, ilk açılışta o
  adresi kaybeder ve penceresi hangi adresin çıkarıldığını bir kez söyler;
  yeniden ayarlanamadıysa kapatılır ve penceresi yeniden kaydetmeyi söyler.
- alpha.141'den önce açılan ve başka kimsenin kullandığı görülmeyen bir
  PostgreSQL veritabanı, izlemesi başladıktan en az 24 saat sonraki ilk
  saatlik denetimde, en az 24 saatlik denetim onu okumuşsa kendiliğinden
  başka hesaplara kapatılır; o zamana dek PostgreSQL sayfası ne zaman
  kapanacağını gösterir. Diğerleri yöneticiye **Şu anki sorunlar**da ve
  PostgreSQL sayfasında, her biri onu kapatan tek düğmeyle listelenir.
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
| `cmd/agent` | 5854 | 5884 |
| `cmd/panel` | 4726 | 4749 |

(Geçen testler ve alt testler; Go modülünün tamamı 15838 → 15894, birleşmiş
sürümde ölçüldü. Bu test makinesinde `cmd/agent` paketinin 19 testi önce ve
sonra aynı biçimde, makinenin kendisi yüzünden başarısız: DNS motoru durumu
PowerDNS'e yetki vermiyor ve geçici klasörleri root grubuna ait değil. Özel
PostgreSQL 17.12 ve MariaDB 11.8 sunucularında veritabanı testleri: 22'si
geçer, şablonları SQL_ASCII olan bir sunucu isteyen ikisi orada atlanır.)

Yeni başarısızlık yok. Web: `npm test` 3090 → 3102 geçti, 0 başarısız; tür
denetimi temiz ve derleme her boyut bütçesini karşılıyor (ilk yükleme yolu
sıkıştırılmış 110,00 KiB'nin 109,38 KiB'i; alpha.142: 109,37).
