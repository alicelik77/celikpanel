# v0.1.0-alpha.143

[English](RELEASE-NOTES-v0.1.0-alpha.143.md)

**Bir güvenlik sürümü: lütfen yakında güncelleyin.** Bir müşterinin başka
bir müşteri gibi davranabildiği üç yolu kapatır: başka bir müşterinin alan
adıyla imzalanan giden posta, bir müşterinin kodunu yöneticinin haklarıyla
çalıştıran PostgreSQL veritabanı taşıma ve alpha.141'den önce açılan,
sunucudaki her hesabın hâlâ girebildiği PostgreSQL veritabanları. Ayrıca
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
   PostgreSQL veritabanlarını kapatmaya başlar; sonra her saat yeniden
   bakar (aşağıda "alpha.141'den önce açılan PostgreSQL veritabanları").
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
- **Bir müşterinin kodunu yönetici olarak çalıştıran veritabanı taşıma.**
  Bir PostgreSQL veritabanını bu makinedeki başka bir veritabanı sunucusuna
  taşımak, satırlarını yöneticinin haklarıyla yüklüyordu; böylece
  veritabanının sahibinin bir tabloya koyduğu kod bu haklarla çalışıyordu.
  Artık her tablonun satırları yalnız o tablonun sahibinin haklarıyla
  yüklenir.
- **Her hesaba açık eski PostgreSQL veritabanları.** alpha.141'den önce
  açılan veritabanları PostgreSQL'in varsayılanını koruyordu: sunucudaki
  her veritabanı kullanıcısı her veritabanına bağlanabilir ve içinde geçici
  tablo açabilir. Bir müşterinin veritabanı kullanıcısı başka bir müşterinin
  veritabanına girip yapısını görebiliyordu. Artık bu veritabanları başka
  hesaplara kapatılır: güvenli olduğu yerde panelin kendisi, yöneticinin
  karar vermesi gereken yerde tek düğmeyle.

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

- Panel böyle bir veritabanını kendi başına yalnız şunların hepsi
  geçerliyse kapatır: hakları hâlâ PostgreSQL'in dokunulmamış varsayılanı,
  içinde panelin onun için kaydetmediği bir hesaba ait hiçbir şey yok, böyle
  bir hesap ona bağlı değil ve veritabanı sunucusunda izleme hesabı yok.
  Sahibi ve panelin onun için kaydettiği kullanıcılar girmeye devam eder;
  diğer her hesap artık ona bağlanamaz ve içinde geçici tablo açamaz.
- Diğer her böyle veritabanı olduğu gibi bırakılır ve **Şu anki sorunlar**da
  ve **PostgreSQL** sayfasında, neyin bulunduğu, kapatmanın neyi
  değiştireceği ve onaydan sonra kapatan tek düğmeyle listelenir.
- Her kapatma ne zaman ve kim tarafından yapıldığını, **Geri al** düğmesini
  (ve panel olmadan geri almak için düz komutları) gösterir; geri alma,
  kapatmanın değiştirdiğini tam olarak geri verir. Yöneticinin yeniden
  açtığı bir veritabanı bir daha kendiliğinden kapatılmaz.
- MariaDB'de böyle bir varsayılan yok: orada hiçbir şey değişmez. Başka
  makinelerde kayıtlı PostgreSQL sunucularına dokunulmaz.

### Bir PostgreSQL veritabanını başka bir veritabanı sunucusuna taşımak

Yapı eskisi gibi yüklenir; her tablonun satırları, sonradan kaldırılan geçici
bir hesap üzerinden, yalnız o tablonun sahibinin haklarıyla yüklenir.
Taşımanın koruduğu şeyler değişmez (sahipler, haklar, açıklamalar,
eklentiler, ayarlar, büyük nesneler) ve kaynak, eskisi gibi, yalnız kopya
doğrulandıktan sonra kaldırılır. Bu yolla yapılamayan bir taşıma, hiçbir şey
yazılmadan, nedeni ve yapılacakla birlikte reddedilir ve kaynak olduğu gibi
kalır: bir yönetici hesabının sahibi olduğu bir tablo, Ağustos 2025
güncellemelerinden eski veritabanı istemci programları, 13.8 ya da 14.5'ten
eski bir hedef veritabanı sunucusu ya da erişim kuralları geçici hesabın
girişini reddeden bir hedef.

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
- Panelin içinde kendi başına karar vermediği bir şey bulduğu bir PostgreSQL
  veritabanı, yönetici onu PostgreSQL sayfasında kapatana kadar başka
  hesaplara açık kalır. Panelin kaydetmediği veritabanlarına (örneğin elle
  açılanlar) dokunulmaz.
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
- İlk saatler içinde, alpha.141'den önce açılan PostgreSQL veritabanları
  güvenli olduğu yerde başka hesaplara kapatılır; diğerleri yöneticiye
  **Şu anki sorunlar**da ve PostgreSQL sayfasında listelenir.
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
`PostgreSQLPublicAccessScanV1`, `CloseV1` ve `UndoV1`, panel ayar satırları
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
- PostgreSQL: özel PostgreSQL 17 sunucularında
  `TestS1bMovingADatabaseNeverRunsItsCodeWithAdministratorRights` ve
  `TestS1bClosingAnOldDatabaseKeepsItsUsersAndRefusesOthers`; panelin
  kararları ve sayfa `cmd/panel/postgresql_public_access_test.go` ve
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
