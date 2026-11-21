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
   korunur ve **Şu anki sorunlar**da adlandırılır. Basmanız gereken bir şey
   yok.
4. Açılıştan birkaç dakika sonra, güncelleme doğrulanmış ve sunucu sakinken
   panel, alpha.141'den önce açılan ve kendi başına kapatabileceği
   PostgreSQL veritabanlarını izlemeye başlar; her birini, başka kimsenin
   kullandığının görülmediği tam bir günden sonra kapatır ve her saat bakar
   (aşağıda "alpha.141'den önce açılan PostgreSQL veritabanları").
5. **alpha.142'ye dönmek kapatılan yolları yeniden açar.**

## Güvenlik: kapanan yollar

- **Başka bir müşterinin alan adıyla imzalanan posta.** Posta sunucusu,
  "Kimden" satırında burada barındırılan bir alan adı geçen her iletiyi
  imzalıyordu; hem girişini kullanarak gönderen bir posta kutusu için hem
  de sunucuya girişsiz posta bırakan bir web sitesi için. Böylece başka bir
  müşterinin adresini taşıyan posta, o müşterinin geçerli imzasıyla bu
  sunucudan çıkabiliyordu. Artık posta kutusu girişiyle gönderilen posta
  yalnız o posta kutusunun kendi alan adı için imzalanır; 25. porta gelen
  posta hiç imzalanmaz.
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
- **Sunucunun kendisinde oluşan posta** (panelin bildirimleri, zamanlanmış
  görevlerin postası, otomatik yanıtlar) eskisi gibi "Kimden" alan adı için
  imzalanır.
- Web postası, posta programları, otomatik yanıtlar ve yönlendirmeler
  bunun dışında eskisi gibi çalışır; gelen posta değişmez.
- **Sahip için:** değişiklik, panel durdurulsa ya da kaldırılsa da çalışan
  olağan posta sunucusu ayarlarıdır. CelikPanel yeni bir imza ayarını
  kullanmadan önce posta imzalayıcısı onu denetler; posta sunucusu onunla
  çalışmazsa önceki ayar geri konur. Sahibin elle değiştirdiği bir imza
  ayar dosyası ya da süzgeç ayarı olduğu gibi kalır: CelikPanel'in kendi
  metni yanına `opendkim.conf.celikpanel-pending` olarak konur ve **Şu anki
  sorunlar** postanın hâlâ başka bir müşterinin alan adıyla
  imzalanabileceğini söyler; panelin günlüğü ayarı ve sahibin adımını
  adlandırır.

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

- Arch Linux'un OpenDKIM paketi imzayı girişe bağlayamaz: orada girişle
  gönderilen posta, "Kimden" satırında geçen barındırılan her alan adı için
  hâlâ imzalanır. Orada da 25. port imzaya kapalıdır ve **Şu anki
  sorunlar** bunu söyler.
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
  imzalanır; postayı 25. porta girişsiz bırakan bir web sitesi artık
  imzalanmaz.
- Güncellemeden bir gün sonra, alpha.141'den önce açılan PostgreSQL
  veritabanları başka kimsenin kullandığının görülmediği yerde başka
  hesaplara kapatılır; o zamana dek PostgreSQL sayfası her birinin ne zaman
  kapanacağını gösterir. Diğerleri yöneticiye **Şu anki sorunlar**da ve
  PostgreSQL sayfasında listelenir.
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
  önceki yapılandırma kabul edilir.
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
| `internal/transport` | 62 | 62 |
| `internal/db` | 182 | 184 |
| `internal/services` | 371 | 371 |
| `cmd/agent` | 5854 | 5878 |
| `cmd/panel` | 4726 | 4737 |

(Geçen testler ve alt testler; Go modülünün tamamı 15838 → 15875. Bu test
makinesinde `cmd/agent` paketinin 19 testi önce ve sonra aynı biçimde,
makinenin kendisi yüzünden başarısız: DNS motoru durumu PowerDNS'e yetki
vermiyor ve geçici klasörleri root grubuna ait değil.)

Yeni başarısızlık yok. Web: `npm test` 3090 → 3098 geçti, 0 başarısız; tür
denetimi temiz ve derleme her boyut bütçesini karşılıyor (ilk yükleme yolu
sıkıştırılmış 110,00 KiB'nin 109,38 KiB'i; alpha.142: 109,37).
