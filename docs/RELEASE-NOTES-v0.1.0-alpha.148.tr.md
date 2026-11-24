# v0.1.0-alpha.148

[English](RELEASE-NOTES-v0.1.0-alpha.148.md)

**Bir güvenlik sürümü.** Her müşteri sitesinin programları artık kendi
sınırlarının içinde çalışıyor: hepsi birlikte yalnız sitesine verilen belleği,
işlem sayısını ve işlemci süresini kullanabilir; böylece bir site artık
sunucunun tamamını öteki müşterilerden alamaz. Veritabanı kullanıcılarına
süre sınırları ve PostgreSQL'de geçici dosyalar için kesin bir sınır
konabilir. Panelin veritabanı düzeni değişmez.
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

## Güvenlik: bir site artık sunucunun tamamını alamaz

- **Şimdiye kadar bir sitenin programları yalnız tek tek sınırlanıyordu.**
  Bir müşterinin yüklediği programlar, Python uygulamaları, Node.js
  uygulaması ve hizmet uygulamaları (n8n gibi) ile bunların kurulum, derleme
  ve dağıtım adımlarının her birinin kendi sınırı vardı, ama hiçbir şey
  onları birlikte sınırlamıyordu. Birkaç programı olan ya da durmadan yeni
  program başlatan bir site sunucunun bütün belleğini, işlem sayısını ya da
  işlemci süresini kullanabiliyordu; öteki müşterilerin siteleri ve sunucunun
  kendisi beklemek zorunda kalıyor ya da bellek yetmediği için
  durduruluyordu.
- **Artık CelikPanel'in bir site için başlattığı her program o sitenin kendi
  sınırlarında çalışır**: yerel bir systemd dilimi,
  `celiksite_<site kimliği>.slice`; sitenin bütün programları için birlikte
  bir bellek sınırı (takas yok), bir işlem sınırı ve bir işlemci sınırı.
  Programlar bellek sınırına ulaşınca sistem içlerinden birini durdurur;
  işlem sınırında yeni işlem başlamaz, çalışanlar sürer; işlemci sınırında
  yavaşlatılırlar. Öteki siteler ve sunucu kendilerinin olanı korur. Bir
  programın kendi sınırları olduğu gibi kalır; sitenin sınırları dış sınırdır.
- **Bir program nereye gider.** Çalışmıyorken yazılan bir program (yeni bir
  program, durmuş bir program ve her kurulum, derleme ya da dağıtım adımı)
  sitesinin sınırlarına girer. Çalışan bir program sıradan bir kaydetmede ya
  da yeniden başlatmada çalıştığı yerde kalır; böylece kimseden habersiz
  hiçbir şey yeniden başlatılmaz. Bir sitenin çalışan programlarını yöneticinin
  **Sınırların içine taşı** işlemi içeri alır.
- **Sınırlar ve onları kim koyar.** Üç plan anahtarı: bir sitenin
  programlarının birlikte belleği, işlem sayısı ve işlemcisi. Hiçbir ayar
  yoksa bir site 512 işlem, bir işlemci çekirdeğinin %200'ü ve bellek olarak
  3072 MB ile sunucu belleğinin %75'inden küçük olanını alır; böylece hiçbir
  site varsayılan olarak sunucunun tamamını almaz. Bu bellek varsayılanı
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
  bir PostgreSQL oturumunun geçici dosyaları (kesin bir sınır). Geri
  yüklemeler, kopyalar ve içe aktarmalar kullanıcının sınırını korur: daha
  fazla geçici alan isteyen bir yükleme bütünüyle geri alınır ve sınırı ve
  onu kimin yükseltebileceğini söyler. Sunucu sahibinin bir veritabanı
  kullanıcısına elle koyduğu bir değer korunur ve yöneticiye sahibinki
  olarak gösterilir; panel yalnız kendi yazdığını değiştirir.
- **İnsanların gördüğü.** Sitenin Genel Bakış sayfası sitenin sınırlarını,
  uygulanıp uygulanmadıklarını, programların şu an ne kullandığını ve sade
  sözlerle bir sınıra ne zaman ulaştıklarını gösterir; müşteri, sağlayıcısının
  sınırı yükseltebileceğini okur. Yönetici ayrıca her programın nerede
  çalıştığını ve taşımayı görür. **Şu anki sorunlar** yöneticiye taşınacak
  siteleri, sınırlarına tekrar tekrar ulaşan bir siteyi, henüz her siteye
  ulaşmamış bir sınır değişikliğini (panel o siteleri her saat yeniden yazar),
  bitmemiş bir taşımayı ve sınırları tutamayan bir sunucuyu söyler.
- **Koşullar.** Sınırlar için sistemin birleşik denetim gruplarının (cgroup
  v2) bellek, işlem ve işlemci denetleyicileriyle birlikte olması gerekir;
  Debian 13 ve Ubuntu 24.04'te vardır. Bunlar olmayan bir sunucuda hiçbir şey
  sınırlanmaz ve **Şu anki sorunlar** neyin eksik olduğunu söyler. Sınırlar
  yerel systemd dosyalarıdır: panel durdurulsa, kaldırılsa ya da lisanssız
  kalsa da, yeniden başlatmalardan sonra da çalışmayı sürdürür. Sunucu
  sahibinin değiştirdiği bir dilim dosyası olduğu gibi korunur ve sahibin
  kendi eklemelerine
  (`/etc/systemd/system/celiksite_<site kimliği>.slice.d/*.conf`) hiç
  dokunulmaz; sitenin kartı onları adlarıyla gösterir.

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
    `sudo systemctl daemon-reload` çalıştırır; Debian 13'te yeni değer
    çalışan programlara içlerinden biri başladığında ulaşır.
  - Sitesinin sınırlarının içindeki bir program: panelde onu dışarı çıkaran
    bir işlem yoktur. Sunucu sahibi bunu `docs/SITE-SANDBOX.tr.md` (bölüm
    8.7) anlattığı gibi elle yapar: programın biriminde `Slice=system.slice`,
    sonra `sudo systemctl daemon-reload` ve programın yeniden başlatılması.
  - Bir veritabanı kullanıcısının sınırları: anahtarı boşaltmak panelin kendi
    yazdığı değeri kaldırmasını sağlar; sahibin elle koyduğu bir değer
    sahibinin olarak kalır.

## Bilinen sınırlar

- **Tamamlanmış bir güncelleme geri alınmaz** (bkz. "alpha.147'ye dönmek");
  bu sürümün değişiklikleri alpha.148'de geri alınır.
- **Güncellemeden önce çalışan programlar, yönetici siteyi taşıyana kadar
  sitesinin sınırlarının dışında kalır;** böyle bir program taşınana kadar
  sunucunun her yeniden başlatılmasından sonra yine dışarıda başlar.
- **Henüz bir sitenin sınırlarının içinde olmayanlar:** PHP'si (her sitenin
  PHP-FPM havuzu; eskisi gibi kendi PHP sınırlarıyla), zamanlanmış görevleri
  ve bir site hesabının açık tuttuğu kullanıcı yöneticileri.
- **Taşıma sitenin çalışan her programını bir kez yeniden başlatır** ve onun
  başlama süresi içinde (program başkasını koymadıkça 90 saniye, en az 20, en
  çok 300) sınırların içinde çalışıp yeniden yanıt vermesini bekler; vermeyen
  program önceki dosyasıyla çalıştığı yere geri konur. Bir seferde tek bir
  site taşınır, bir taşımada en çok 64 program; bir site taşınırken o sitenin
  değişiklikleri bekler ya da "değiştiriliyor" yanıtını alır. Dosyasını
  sunucu sahibinin değiştirdiği programlar, önceki bir korumadaki programlar
  ve sahibin bir dosyasının başka yere koyduğu programlar taşınmaz; kart
  onları ve kimin ne yapacağını söyler.
- **Debian 13'te değişen bir dilim dosyası çalışan programlara yalnız panelin
  kendi adımıyla ya da bir programın başlamasıyla ulaşır.** Sahip dosyayı
  elle değiştirdikten sonra `sudo systemctl daemon-reload` gerekir ve
  çekirdeğin onu tutması yaklaşık iki dakikayı bulabilir; o zamana kadar kart
  "yazıldı, henüz yürürlükte değil" der.
- **Sunucunun kendi sınırlar ekranı**, düşürülen bir site sınırını
  önizlemez: sunucunun bir site anahtarındaki tavanını ya da varsayılanını
  düşürmek, durduracağı sitelerin listesi gösterilmeden uygulanır.
- **En uzun veritabanı komutu bir varsayılandır:** kullanıcının kendi oturumu
  onu değiştirebilir. Geçici dosya sınırı kesindir. MariaDB'de geçici dosya
  ya da boşta işlem anahtarı yoktur.
- **Disk sınırları sayılır ama dosya sistemince uygulanmaz**; barındırma
  dosya sisteminde proje kotası yoksa panel sayar ve davranır (yüklemeler
  durur), dosya sistemi bir yazmayı kendisi reddetmez. **Şu anki sorunlar**
  bunu bir kez söyler.
- alpha.147'nin açık listesi açık kalır
  ([alpha.147 sürüm notları](RELEASE-NOTES-v0.1.0-alpha.147.tr.md)).

## Laboratuvarda denenenler

**Açık.** Deneme sunucusu çiftindeki (Debian 13 ve Ubuntu 24.04)
laboratuvar çalışması henüz yapılmadı.

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
  bir programı hemen durdurur; düşürülen işlem sınırında yalnız yeni işlemler
  reddedilir.
- **Sınırların içine taşı, o sitenin çalışan her programını bir kez, birbiri
  ardına yeniden başlatır:** her program kendi yeniden başlatması boyunca,
  genellikle birkaç saniye, en çok da başlama süresi kadar (programın
  koyduğu başka bir süre yoksa 90 saniye, en çok 300) çalışmaz; sonra
  çalıştığı yere geri konur. Sitenin PHP'sinden, havuzundan ya da web
  sunucusundan hiçbir şey yeniden başlatılmaz ya da yeniden yüklenmez. Anı
  seçmek için bir siteyi onun için sakin bir saatte taşıyın.
- Yeni bir program, çalışmıyorken kaydedilen bir program ve her kurulum,
  derleme ya da dağıtım adımı artık sitenin sınırlarının içinde çalışır:
  sitenin sahip olduğundan fazla bellek isteyen bir adımı (varsayılan olarak
  3072 MB ya da sunucu belleğinin %75'i) sistem durdurur ve kart hangi sınıra
  takıldığını söyler.
- **Şu anki sorunlar**, her site taşınana kadar yöneticiye "Programları
  sınırlarının dışında çalışan siteler" satırını gösterir.

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
türleri 39 ile 46 arası.

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
  `TestAKeptMoveIsOneLineAndCanBeTriedAgain`.
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
  `site-limits-round3.test.mjs`.

Güvenlik bölümü üç bağımsız incelemeden ve üç düzeltme turundan geçti;
sürüm çekme isteğinin her denetimi CI'da geçti.
