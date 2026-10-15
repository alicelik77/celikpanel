# v0.1.0-alpha.86 (taslak, yayımlanmadı)

[English](RELEASE-NOTES-v0.1.0-alpha.86.md)

Bu, v0.1.0-alpha.85'i izleyen sürümün taslağıdır. v0.1.0-alpha.86 numarasını,
sahibin 13 Ekim 2026 tarihli kalıcı talimatı uyarınca ("hem sürüm numaraları
hem de commit işlemleri için bana sormana gerek yok": sürüm numaraları ve
commit'ler için sahibe sorulmaz) sürümü hazırlayan seçti; sahibin bu sürüm
için adını koyduğu bir numara değildir. Sürüm hazırlığı `313f3eea2`
commit'idir (85'ten sonra sıra 86; önceki olarak alpha.85 sürüm commit'i
`36c33c4531ee6c1306d324db7a666403176f3bf7`). Hiçbir parçası yayımlanmadı ve bu
taslak onu çalıştıran hiçbir kurulu sunucu kaydetmez. Bu taslak 15 Ekim
2026'da (UTC) yazıldı.

İş, sahibin 15 Ekim 2026'da ikinci kurulu sunucuyu (Frankfurt) panelin kendi
güncelleme ekranından v0.1.0-alpha.84'ten v0.1.0-alpha.85'e güncellerken
gördüğüyle başladı. Panel, güncellemenin kendi planının söylediği gibi yeniden
başlarken güncelleme sayfası uyarı renginde bir bildirim gösterdi:
"v0.1.0-alpha.85 güncellemesinin durumu şu an okunamadı; bu yüzden bitip
bitmediği bilinmiyor…"; aynı anda bir köşe bildirimi aynı şeyi "T+02:59" ile
söyledi. Güncelleme sonra olağan biçimde tamamlandı. Sahibin sözleri özetle:
hiçbir sorun yokken bu bir sorun gibi görünüyor; işlem söylediğini yaparken
daha az ürkütücü bir ileti doğrudur, sorun metni yalnız gerçek bir soruna
aittir. Bu sürüm güncellemenin kendi yeniden başlamasını sakin ilerleme olarak
gösterir, "bilinmiyor" metnini bir sınırın sonrasına bırakır ve bunu iki
bildirim yerine bir bildirimle söyler ([D-033](DECISIONS.tr.md) ve
[D-024](DECISIONS.tr.md)).

Bunun yanında alpha.85 notlarının ve sahibin bildiriminin açık kalemlerini
düzeltir: sayfanın dil özniteliği, bileşen adları ve açıklamaları Türkçe,
"Son etkinlik" ham denetim kimlikleri yerine cümle olarak, hâlâ yeniden tarama
isteyen sunucu cümleleri, bir posta profili retinde platform, posta profili
gereksinimlerinde rol adları, kurulumun ters DNS cümlesinin arkasındaki
çözümleyici, sekme gizliyken çalışan okumalar ve yayımlama betiğinin göreli yol
kusuru. Denetim günlüğü artık o aktörün adını "Sistem" diye söylediği için
`system` kullanıcı adını ayırır.

Yaptıkları, bileşen testlerine ve API taklidine karşı iki tarayıcı incelemesine
dayanır (on üçüncü tur: yeniden başlama bildirimi; on dördüncü tur: küçük
düzeltmeler). Bu taslak yazıldığında adayın hiçbir gerçek sistem kaydı
yapılmamıştı; "Gerçek sistem kaydı set17" bölümü yayımlamadan önce
doldurulacak. Hiç çalıştırılmayanları sınırlar bölümü söyler.

## Ne göreceksiniz

Önce sade sözlerle, sunucu sahibi için. Güncellemeden sonra açık olan her
CelikPanel sayfasını yeniden yükleyin; güncellemeden önce yüklenmiş bir sayfada
eski ekranlar durur. CelikPanel'i bir dahaki sefer kendi güncelleme ekranından
güncellediğinizde, panelin o güncellemenin parçası olarak yeniden başladığı an
"Panel, güncellemenin bir parçası olarak yeniden başlıyor. Bu sayfa
kendiliğinden yeniden bağlanır; hiçbir şeye basmanız gerekmiyor." der; yanında
dönen bir simge ve süre sayacı vardır ve iki bildirim yerine bir bildirim
görünür. Panel, güncellemenin başlangıcından yaklaşık 6 dakika sonra (ve yanıt
vermeyi bıraktıktan en az 2 dakika sonra) hâlâ yanıt vermiyorsa, bildirim ancak
o zaman durumun henüz bilinmediğini, bunun başarısızlık anlamına gelmediğini ve
sunucuda neyi denetleyeceğinizi söyler. Bu değişiklik yeni sürümün
ekranlarındadır; bu yüzden onu bundan sonraki güncellemede görürsünüz: bu sürüme
güncelleme, hâlâ çalıştırdığınız sürümün ekranlarıyla izlenir. Türkçe
sayfalarda bileşen adları ve açıklamaları artık Türkçedir; Dashboard'daki
"Son etkinlik", `server.setup.child:nginx` gibi kimlikler yerine ne olduğunu
bir cümleyle söyler (oturum açmış kullanıcısı olmayan eylemler için
"Sistem …"). Hiçbir ekran artık Yeniden tara'ya basmanızı söylemez: panelin
bileşen kaydı doğrulanamadığında Dashboard, Bileşenler sayfası ya da bileşenin
kendi sayfası kendiliğinden kontrol eder ve bunu söyler. Kurulum bir ters DNS
adını bekliyorsa, CelikPanel Agent'ı da güncellendikten sonra eski yanıtı veren
genel çözümleyiciyi adlandırır. Buradaki hiçbir şey sunucunuzun hizmetlerini
değiştirmez. Bu sürümün yayımlanması hiçbir kurulu sunucuyu güncellemez;
güncellemeyi panelin kendi güncelleme ekranından siz başlatırsınız.

## Sunucu sahibi için ne değişiyor

### Güncellemenin kendi yeniden başlaması

**Beklenen yeniden başlama içinde sakin ilerleme.** Bugüne kadar bağlantıyı
yitiren her durum okuması, izlenen güncellemenin aşamasını hemen bilinmiyor
yapıyordu (`web/src/components/SystemUpdateOperation.tsx` içinde
`systemUpdateOperationPhase(undefined)`); güncellemenin planladığı yeniden
başlama hesaba katılmıyordu. Panel'in durum yanıtı yalnız kuyrukta, çalışıyor
(yeni Panel yanıt verince "verifying"), başarılı ve başarısız bilir; Agent'ın
güncelleyicisi Panel'i ve Agent'ı kayıt çalışıyorken yeniden başlatır. Artık,
sunucu bu isteğin etkin olduğunu son söyledikten sonra (başlatma kabul edildi
ya da bir okuma kuyrukta veya çalışıyor dedi) bağlantıyı yitiren ya da 5xx yanıt
alan okuma "yeniden başlıyor" aşamasıdır: dönen simge, "Panel, güncellemenin
bir parçası olarak yeniden başlıyor. Bu sayfa kendiliğinden yeniden bağlanır;
hiçbir şeye basmanız gerekmiyor." (`panelUpdate.tracking.restarting`; kartta
sürümle birlikte `panelUpdate.card.restarting`), T+ sayacı ve değişmeyen
güncelleme kilidi. Bu sürerken sayfa en geç 3 saniyede bir okur; böylece hemen
yeniden bağlanır (`8e27dd0eb`).

**Sınır.** Uyarı işareti ve "bilinmiyor" metni yalnız pencere aşılınca
görünür: güncellemenin başlangıcından 6 dakika ve ilk kopan okumadan en az 2
dakika (`web/src/lib/systemUpdateTracking.ts` içinde
`systemUpdateRestartDeadline`: başlangıçtan `SYSTEM_UPDATE_EXPECTED_RESTART_MS`
ile ilk kopan okumadan `SYSTEM_UPDATE_RESTART_AFTER_LOSS_MS`'nin geç olanı;
ikisi de tarayıcının saatiyle, sunucunun saatiyle hiçbir şey karşılaştırılmaz).
Bilinmiyor satırının altında bildirim, panelin bir güncellemenin onu yeniden
başlatması için normalde gereken süreden daha uzun süredir yanıt vermediğini,
bunun güncellemenin başarısız olduğu anlamına gelmediğini, sunucunun
güncellemenin kaydını tuttuğunu ve bildirimin onu okumayı sürdürdüğünü ve
sahibin neyi denetleyeceğini (`sudo systemctl status celikpanel-panel`) söyler
(`panelUpdate.tracking.restartOverdue`). Doğrulanmış bir hata sonlanmış bir
sonuçtur ve kendi metnini korur. "İkinci bir güncelleme başlatmayın" kalır.

*6 dakika nereden geliyor.* On sekizinci gerçek sistem kaydı (set16,
`deploy/e2e/release-recovery/evidence/set16-20261014/*/upd84-good-a/driver/steps/12-owner-start`
ve `13-track`): alpha.84'ten alpha.85 adayına iyi bir güncelleme Debian 13'te
51 sn (başlangıç 22:55:07Z, `update_verified` 22:55:58Z), Ubuntu 24.04'te 48 sn
sürdü; Panel API'si en çok 32 sn ve 25 sn yanıtsız kaldı; laboratuvar arşivi
yerel bir kaynaktan verdi. Arşivi internetten indiren sahibin gerçek
güncellemesi T+02:59'da hâlâ yanıt vermiyordu. Pencere bunların en uzununun
yaklaşık iki katıdır (2 × 3 dk); ilk kopan okumadan itibaren 2 dakika, yavaş bir
güncellemede geç başlayan bir yeniden başlama için ölçülen en uzun kesintinin
yaklaşık dört katıdır. İki laboratuvar güncellemesine ve bir sahip gözlemine
dayanır (sınırlara bakın).

**Tek bildirim.** İki dakikalık gezinti kilidinden
(`SYSTEM_UPDATE_BLOCKING_DEADLINE_MS`) sonra köşe bildirimi her sayfada
çiziliyordu; bu yüzden güncelleme sayfasında kart ve köşe bildirimi aynı şeyi
söyledi. Artık güncelleme kartı ekrandayken T+ sayacını ve işlem kimliğini o
taşır ve izlenen güncelleme için köşe bildirimi çizilmez; başka sayfalarda tek
bildirim köşe bildirimidir. Bilinen bir sonuç köşe bildirimini korur. Kartın
bilinmiyor satırı köşe bildirimini göstermek yerine artık "Bu sayfa onu
kendiliğinden yeniden okur" der; T+ sayacı ve kimlik iki ayrı birim olarak
satır kırar, böylece telefonda kimlik satır ortasında bölünmez (`15d80851e`).

Testler: `web/tests/update-restart-notice-mounted.test.mjs`, 13 test (okuma
sınıflandırması ve pencere, İngilizce ve Türkçe metin, yeniden başlamada ve
pencere sonrasında bağlanan kart, tek bildirim kuralı ve sağlayıcının
bağlantıları için kaynak denetimi); her biri önceki kart, sağlayıcı ya da
kitaplığa karşı başarısız olur; `8e27dd0eb`'de `npm test` 1404/1404.
Sağlayıcının kendisi bir bileşen testinde bağlanmadı (IndexedDB, Web Locks ve
kimlik doğrulama sinyali gerektirir); on üçüncü tarayıcı turunda laboratuvar
taklidine karşı gerçek Chrome'da çalıştı (aşağıda).

### Yayımlama betiği

**`deploy/publish-download-portal.ps1` her çalışma dizininden çalışır.** 15
Ekim 2026'da alpha.85'in portal yayımı önce parametre bağlamada durdu
("Join-Path : Cannot bind argument to parameter 'Path' because it is an empty
string"); çünkü betik `-File` ve göreli yolla başlatıldığında `$PSScriptRoot`
parametre varsayılanında boştur. `-Promoter` ve `-PublicVerifier` artık boş
varsayılanlıdır ve betiğin gövdesinde `$PSScriptRoot`, yoksa `$PSCommandPath`,
yoksa `$MyInvocation.MyCommand.Path` ile çözülür
(`Resolve-PublisherScriptDirectory`); hiçbiri yoksa betik reddeder ve açık
parametreleri ister. Açık değerler verildiği gibi kullanılır (`f89551e9b`).
Test: `deploy/test-download-portal-publisher.ps1` (CI işi `publisher-windows`)
hiçbir parametre varsayılanının betiğin konumuna bağlı olmadığını denetler,
çözücünün sırasını ve reddini birim düzeyinde sınar ve yayımlayıcıyı geçici
dizinden `-File` ile, bilerek geçersiz bir sürümle çalıştırır; yayımlayıcı ilk
doğrulamada (herhangi bir dosya, ağ ya da uzak erişimden önce) durmalı, asla
bağlamada durmamalıdır. Depo kökünden ve `%TEMP%`'ten geçer, önceki
yayımlayıcıya karşı başarısız olur. Yayımlayıcı hiçbir hedefe karşı
çalıştırılmadı.

### Okumalar yalnız sayfa görünürken

Alpha.85 notları bir sınır olarak, bileşen kurma penceresinin 5 saniyelik
hazırlık yeniden okumasının (`ServiceShell.tsx`) ve kurulum sayfasının işlem
okumalarının (tamamlanırken 1,5 sn, diğer durumlarda 3 sn; `ServerSetup.tsx`)
sekmenin görünürlüğünden bağımsız çalıştığını listelemişti.
`web/src/lib/visibleReads.ts` (`startVisibleReads`) artık ikisini de yürütür:
sekme gizliyken aralık temizlenir ve hiçbir şey okunmaz; sekme yeniden
gösterilince hemen bir okuma çalışır ve aralık sürer; süren okumayı bulan tik
atlanır; belge yoksa sayfa görünür sayılır. İki çağıran da yalnız GET gönderir.
Pencere açılış okumasını, kurulum sayfası bağlanma okumasını korur; işlem
sunucuda çalışır ve kaydı orada kalır, bu yüzden gizliyken okunması gereken bir
şey yoktur (`0082c9d08`). Testler: `web/tests/visible-reads.test.mjs` (üç birim
testi) ve `setup-finishing-mounted.test.mjs` ile
`remote-state-mounted.test.mjs` içinde bağlanmış testler (gizli: aralık yok;
gösterildi: bir okuma ve aralık; yeniden gizli: durdu); ikisi de önceki
bileşenlere karşı başarısız olur; o commit'te `npm test` 1409/1409.

Gizliyken zaten duran iki yeniden okuma, güncelleme kartının sessiz hazırlık
okuması ve yayılan alan adının yeniden kontrolü, sekme yeniden gösterilince
olağan gecikmelerini (5 sn, 30 sn) bekliyordu. Artık `visibleReads.ts` gibi
hemen bir kez okurlar, sonra gecikmeyi o okumadan itibaren sürdürürler; süren
okuma o okumadır, alan adı kontrolü 60 kontrollük sınırına yine sayılır ve
ikisi de yalnız GET kalır (`1abe1717d`; `domain-connection-spread-mounted` ve
`panel-update-readiness-poll-mounted` testleri; ikisi de önceki bileşenlere
karşı başarısız olur). Bu, D-033'ün 6. kuralıdır.

### Türkçe sayfalar Türkçe konuşur

**Sayfanın dil özniteliği (set16 gözlemi O1).** Türkçe sayfalar
`<html lang="en">` taşıyordu; çünkü özniteliği yalnız `web/index.html`
ayarlıyordu. i18n sağlayıcısı artık `document.documentElement.lang` değerini
sayfanın çizildiği dile ayarlar: açılış ekranında hedef dil, dil değişince ancak
yeni katalog geldiğinde (`d2c0add01`, `web/src/i18n/index.tsx`). Test: gerçek
kataloglar üzerinde `web/tests/document-language.test.mjs`; dört durumundan
üçü önceki sağlayıcıya karşı başarısız olur.

**Bileşen adları ve açıklamaları (set16 gözlemi O2).** `internal/core`
kataloğunun her kimliğinin (27 kimlik) İngilizce ve Türkçe görünen adı
(`services.component.name.<id>`) ve tek satırlık açıklaması
(`services.component.description.<id>`) vardır. Ürün adları olduğu gibi kalır;
betimleyici adlar çevrilir ("Firewall engine" "Güvenlik duvarı motoru",
"Intrusion Prevention" "Saldırı önleme" olur); kimlikler değişmez, sayfanın
bilmediği bir kimlik sunucunun adını ve metnini korur
(`web/src/lib/componentName.ts`, `componentName` ve `componentDescription`).
Bileşenler listesinde, iletişim kutularında, işlem etiketlerinde ve aramada,
bileşen sayfasında, kurulumun bileşen adımında ve gözden geçirmesinde ve
Dashboard'ın duran hizmet satırında kullanılırlar; cron ve güvenlik duvarı
motoru yönlendirmesi Bileşenler satırını sayfanın gösterdiği adla anar
(`151969774`, `ef7690fc0`). Test: `web/tests/component-names.test.mjs`
kimlikleri Go kataloğundan okur; iki adı ve iki açıklaması olmayan bileşen
başarısız olur.

### Son etkinlik cümle olarak

**Her denetim eylemi bir cümledir (set16 gözlemi O3; alpha.85 güncellemesinden
sonra iki kurulu sunucuda da görüldü).** Dashboard, oturum açma ve posta
profilleri dışındaki her eylem için ham denetim kimliklerini gösteriyordu.
`cmd/panel/audit_actions.go` artık Panel'in yazdığı 179 eylem adını listeler.
Bir Go testi, `TestAuditActionNamesListEveryActionThePanelWrites`, adları her
`INSERT INTO audit_logs` kaynağından go/ast ile türetir; eylem argümanını
parametreler (çağrı yeri başına), kapanışlar, yerel değişkenler, birleştirme,
`fmt.Sprintf`, dize yardımcıları ve değer olarak geçirilen hizmet işlemi
yazıcısı boyunca izler; yazılabilen bir ad eksikse ya da listedeki bir ad
yazılamıyorsa başarısız olur; ulaşılamayan bir biçim gerekçesiyle muaf tutulur
(`TestAuditActionAnalysisFollowsParametersClosuresAndLocals` çözümlemenin
kendisini sınar). `web/src/lib/auditAction.ts` her adı İngilizce ve Türkçe,
kullanıcının adından sonra, hedefi `{target}` olarak söyler (bileşen
kimlikleri görünen adlarıyla, posta profilleri adlarıyla). Sayfanın sözcüğü
olmayan bir eylem kimlik olarak değil sözcük olarak söylenir ve hedefi
değişmeden ardından gelir ("… example.com", `7956001d0`) (`8a5f299fa`). Test:
`web/tests/audit-actions.test.mjs` Go listesini okur ve her ad için iki dilde
cümle ister; beş testinden dördü önceki Dashboard ve kataloglara karşı
başarısız oldu. 179 cümle sunucu katalog bölümünde durur
(`web/src/i18n/screens/server/{en,tr}.ts`); çünkü genel ekran kataloğunda
Türkçe parçayı parça başı tavanın 10 KiB üstüne çıkardılar ve `npm run build`
paket bütçesi denetiminde başarısız oldu; tavan yükseltilmedi (`8a3729cbe`).

**Sistem aktörü ve ayrılmış kullanıcı adı `system`.** Panel, oturum açmış
kullanıcısı olmayan bir eylem için aktör olarak `system` yazar. Dashboard ve
denetim günlüğü artık onu "Sistem" / "System" diye söyler; başka her ad olduğu
gibi gösterilir (`4372fc86e`). Bu adı hiçbir kişinin alamaması için
`auth.ValidateUsername` (`internal/auth/validate.go`) `system` adını her harf
biçiminde reddeder (`ErrReservedUsername`). Her oluşturma yolu ondan geçer:
hesap oluşturma, ekip üyesi oluşturma ve yeniden adlandırma, yönetici CLI'si ve
istemi. İki HTTP yolu `400 USERNAME_RESERVED` döner (kodlu; İngilizce ve Türkçe
cümle `err.USERNAME_RESERVED`); CLI her zamanki geçersiz ad iletisini yazar.
Hiçbir geçiş ya da tohum `system` adlı kullanıcı oluşturmaz; var olan satıra
dokunulmaz ve bu adı zaten taşıyan bir ekip üyesi, ad değişmediği sürece
düzenlenebilir (`8fe746582`). Testler: `TestValidateUsernameReservesSystemActor`,
`TestReservedUsernameIsRefusedWithItsOwnCode`,
`TestReservedUsernameDoesNotBreakAnExistingMember`. Yapılmadı: denetim günlüğü
aktör türü saklamaz; bu yüzden önceden var olan `system` kullanıcı adlı bir
satır yine "Sistem" çizilirdi (bu Panelden doğamaz).

### Hiçbir cümle yeniden tarama istemez; panel kendiliğinden kontrol eder

Bunlar D-032'yi (panel kendiliğinden bakar ve okuyabileceği bir durumu öğrenmek
için sahipten asla düğmeye basmasını istemez) izler ve alpha.85'in "Şablonlu
metin" sınırını yeniden tarama ifadesi için kapatır.

- **Doğrulanamayan kaydın reti** (`503 SERVICE_STATE_UNVERIFIED`, zaten kodlu)
  artık kayıtlı bileşen kaydının doğrulanamadığını, bu yüzden gösterilmediğini
  ve hiçbir şeyin değiştirilmediğini, CelikPanel'in Dashboard, Bileşenler
  sayfası ya da bir bileşenin sayfası açıkken bileşenleri kendiliğinden yeniden
  kontrol ettiğini söyler (`cmd/panel/managed_service_handlers.go` içinde
  `serviceStateUnverifiedMessage`, `cd2275e47`, `a6bdf410b`; web bunu koddan
  söyler, `err.SERVICE_STATE_UNVERIFIED`). Test:
  `TestUnverifiedComponentRecordRefusalSaysThePanelChecksByItself`.
- **Bu artık Dashboard'da ve Bileşenler sayfasında doğrudur.** Bu koda kendi
  koşullu kontrolleriyle (`POST /api/v1/managed-services/scan?max_age_seconds=…`,
  bir okuma), bir kez, bildirim olmadan, eskimiş bir kayıtta yaptıkları gibi
  yanıt verirler. Başarısız kontrol mevcut "kontrol edilemedi" durumunu korur
  ve kendiliğinden yeniden sorulmaz (`cd2275e47`;
  `remote-state-mounted-batch3.test.mjs` içinde bağlanmış testler, üçü önceki
  sayfalara karşı başarısız olur).
- **Ve bir bileşenin kendi sayfasında** (`/services/<id>`). Orada `503
  SERVICE_STATE_UNVERIFIED` yanıtı, Dashboard ya da Bileşenler sayfası
  kontrolünü çalıştırana dek Tekrar dene'nin başarısız kaldığı genel "Bu
  bileşenin kaydı okunamadı" idi. Sayfa artık aynı koşullu kontrolü çalıştırır
  (tek `POST /api/v1/managed-services/scan?max_age_seconds=300`; yeniden
  yüklenen sayfanın arama adımı, bölümler ve başlık onu paylaşır; bildirim
  yok), ardından kaydı yeniden okur. Sürerken sakin bir satır CelikPanel'in
  bileşenleri şimdi kontrol ettiğini ve yapılacak bir şey olmadığını söyler;
  Tekrar dene yoktur (`component.checkingStored`). Başarısız kontrol mevcut
  "kontrol edilemedi" durumunu korur ve kendiliğinden yeniden sorulmaz;
  başarılı bir kontrolden sonra geri dönen ret ikinci kez kontrol edilmez
  (`297f1f0b1`). Süren bir değişiklik yüzünden tutulan kontrol (`409
  server_setup_busy` ya da `service_operation_busy`) bunu söyler ve Dashboard
  gibi kendiliğinden sürer: tutulan durum ekrandayken sayfa
  `GET /api/v1/host-mutation-readiness` okur (yalnız okuma; 5 saniyede bir,
  yalnız sekme görünürken, tek tek; sayfada kaç okuyan olursa olsun tek
  yoklama) ve bu okuma meşgulden hazıra geçince kaydı bir kez daha ister (tek
  izleme kontrolü, döngü yok). Yoklamanın hiç meşgul görmediği bir değişiklik
  ya da sürekli meşgul kalan bir sunucu tutulan durumu yerinde bırakır; Tekrar
  dene durur ve hemen sorar. `component.checkHeld` şunu der: "Değişiklik bitince
  sayfa kendiliğinden yeniden denetler; Tekrar dene hemen sorar."
  (`00138c530`; `web/src/lib/managedServices.ts` içinde `useStoredRecordCheck`
  ve `runStoredRecordCheck`, `ServiceRecordLookup`, `ComponentPanels`,
  `ServiceShell`). Testler: `web/tests/remote-state-mounted.test.mjs` içinde on
  altı bağlanmış durum (iki giriş noktası, her birinde sekiz: kontrol, kontrol
  ediliyor, başarısız, tutulan, tutulanın kendiliğinden sürmesi, hiç ya da hep
  meşgulken izlenmemesi, yalnız sekme görünürken okuma, döngü yok) ve bir
  olumsuz kontrol (başka her ret değişmez); izleme ya da görünürlük kuralı
  kaldırılınca süren durumlar başarısız olur.
- **`err.SERVICE_STATE_REFRESH_FAILED`** artık düğme talimatı taşımaz. Sayfanın
  ne sonucu ne durumu varsaydığını ve CelikPanel'in Dashboard, Bileşenler
  sayfası ya da bir bileşenin sayfası açıkken bileşenleri kendiliğinden
  yeniden kontrol ettiğini söyler (`d2b58e7e9`, `00138c530`).
- **`services.stateUnverifiedHint`**, sonucu bilinmeyen bir işlemden sonraki
  şerit, artık "Yeniden tara'yı çalıştırın" demez. İşlemin tamamlanmış
  olabileceğini, bileşen değişikliklerinin şimdilik kilitli kaldığını,
  CelikPanel'in bu sayfa her açıldığında son kontrol birkaç dakikadan eskiyse
  bileşenleri kendiliğinden yeniden kontrol ettiğini ve kilidin bu kontrol yeni
  durumu gösterince kalktığını söyler. Yükleme akışı tam bunu yapar
  (`AUTO_SCAN_MAX_AGE_SECONDS` ile koşullu tarama); zamanlayıcı yoktur, cümle de
  zamanlayıcı vaat etmez (`8fe746582`; test sözcükleri ve dayandıkları sabiti
  sabitler).
- **Mağaza'nın üç `component_state_*` cümlesi** (`blocker_reason` ile kodlu,
  sunucu `cmd/panel/store_backend.go` içinde yerelleştirir) CelikPanel'in bir
  yönetici Dashboard'ı, Bileşenler sayfasını ya da bir bileşenin sayfasını açık
  tuttuğu sürece bileşenleri kendiliğinden kontrol ettiğini ve o kontrolden
  sonra Mağaza'nın yeniden açılmasını söyler (`cd2275e47`, `21f07e7a8`). Test:
  `TestStoreComponentRecordStatesSayWhatThePanelDoes` (iki dil, yeniden tarama
  ifadesi yok).
- **Bilinmeyen durum için Mağaza'nın rozeti ve süzgeci.** Panel'in bileşenlere
  ait kullanılabilir kaydı olmadığında (henüz yok, 15 dakikadan eski,
  okunamıyor) bir ürünü `blocker_reason component_state_*` ile "desteklenmiyor"
  işaretler. O cümlenin yanındaki rozet "Bu sunucuda desteklenmiyor" diyordu;
  bu, kimsenin bilmediği bir durum hakkında bir iddiadır (D-024). Bu ürünler
  artık bir saat simgesiyle "Durum henüz bilinmiyor" / "State not known yet"
  taşır; gerçekten desteklenmeyen ürün rozetini korur (`1f564fdb0`). Durum
  süzgeci onları "Bu sunucuda desteklenmiyor" altında değil, kendi girdileri
  "Durum henüz bilinmiyor" altında listeler; varsayılan görünüm onları yine
  gösterir (`297f1f0b1`). Test:
  `web/tests/store-state-not-known-mounted.test.mjs`, iki dil, önceki rozet ve
  gruplamaya karşı başarısız olur.

### Posta profili cümleleri

**`component_not_installable` içinde platform.** Alpha.85'in kodlu cümlesi,
yalnız eski İngilizce metnin taşıdığı platform ayrıntısını düşürüyordu. Panel
artık paket ekosistemi doğrulanmışsa `blocked_vars.platform` ekler
("Debian/Ubuntu (apt)", "Arch Linux (pacman)", "Fedora/RHEL (dnf)"; ürün
adları, iki dilde aynı) ve platform doğrulanmamışsa eklemez
(`cmd/panel/mail_profiles.go`, `80d763949`). Sayfa o zaman "{component} bu
sunucunun platformunda otomatik olarak kurulamıyor: {platform}." der; aksi
durumda (doğrulanmamış platform ya da eski Panel) kısa cümleyi korur. Testler:
`TestMailProfileViewTypesItsSentences` içinde (pacman Dovecot'u ve platformu
adlandırır, dnf platformu adlandırır, doğrulanmamış platform hiçbirini
adlandırmaz) ve `web/tests/mail-profile-text.test.mjs`; her biri önceki kaynağa
karşı başarısız olur.

**Gereksinimde rol adları.** `component_requires` içinde `required`, bileşen
adlarının yanında bir rol belirteci taşıyabilir (`web-server`, `smtp-server`,
`imap-server`, `spam-filter`, `dns-server`, `kv-store`: katalogdaki
`ConflictGroup` değerleri). Sayfa her rolü görünen adıyla söyler
(`services.mailProfiles.role.<token>`, İngilizce ve Türkçe); tanımadığı rol
belirteç olarak değil sözcük olarak söylenir (`0a81043cf`; test rolleri Go
kataloğundan okur). Birden çok gereksinim artık "ve" / "and" ile biten bir
liste olarak, kendi çoğul cümlesiyle söylenir (`component_requires.many`);
İngilizce rol adları tanımlıklarını taşır ("requires a web server"); Türkçe
cümle biçimini korur. On dördüncü turdaki tekil cümle İngilizcede "Rspamd
requires Redis, key-value store, mail relay host, which is not part of this
profile." diyordu (`370afa2c9`).

### Kurulumun ters DNS cümlesinin arkasındaki çözümleyici

Kurulumun son kontrolü hangi ters DNS adını gördüğünü söylüyordu ama onu hangi
genel çözümleyicinin verdiğini söylemiyordu; bu yüzden sağlayıcıda yapılan bir
değişiklik yayılırken sahip eski bir çözümleyiciyi değişmemiş bir kayıttan
ayıramıyordu (alpha.85 sınırı "Yanıt veren çözümleyici adlandırılmaz" ve
sahibin 15 Ekim 2026 tarihli bildiriminin (f) maddesi). Agent'ın posta sağlık
yanıtı artık iki ek alan taşır: `reverse_dns_resolver` (bildirilen PTR'nin
yanıtı olduğu çözümleyicinin IP'si) ve `reverse_dns_resolvers_disagree` (iki
çözümleyici farklı yanıt verdi); yalnız arama yanıt verdiyse ayarlanırlar
(`cmd/agent/mail_health_dns.go`, `cmd/agent/mail_health_rpc.go`,
`internal/transport/mail_observability_contracts.go`). `mailDNSIdentityAt`
sonucunu korur ve var olan her alan değişmez. Panel'in
`setupMailIdentityReason` işlevi (`cmd/panel/server_setup_readiness.go`)
`reverse_dns_mismatch` değişkenlerine `resolver` (yalnız geçerli bir IP) ve
`resolvers=disagree` ekler. Kurulumun son kontrolü çözümleyiciyi adlandırır ve
çözümleyiciler uyuşmadığında bir değişiklik yayılırken böyle olduğunu söyler
(dört yeni İngilizce ve Türkçe cümle,
`setup.check.mailIdentity.reverseDNSFrom`, `reverseDNSMissingFrom`,
`reverseDNSDisagree`, `reverseDNSMissingDisagree`;
`web/src/lib/serverSetupGuidance.ts`); adım yine "Ters DNS bekleniyor" der. Eski
bir Agent iki alanı da göndermez ve sayfa önceki cümleyi korur (`e2c630faa`).
Testler: `TestMailHealthNamesTheResolverAndSaysWhenResolversDisagree`
(çözümleyici adlandırılır, uyuşma ve uyuşmazlık, doğrulanmış kimlik, başarısız
arama hiçbirini adlandırmaz), Panel'in değişkenleri
`TestServerSetupMailIdentityCheckNamesTheTypedReason` içinde (adres olmayan
çözümleyici düşürülür) ve `setup-final-check-mounted.test.mjs` içinde
bağlanmış web testleri (iki dil; sekizinden altısı önceki sayfaya karşı
başarısız olur). Bu ancak Agent güncellendikten sonra etkili olur (sınırlara
bakın).

### On dördüncü tarayıcı turunun düzeltmeleri

On dördüncü tur (aşağıda) yukarıdaki düzeltmelerin fotoğrafını çekti. Bu
taslaktan önce düzeltilen altı kusur buldu:

- bilinmeyen bir denetim eylemi hedefinin noktalarını yitiriyordu ("backup
  offsite verify partial example com"); hedef artık değişmeden gelir
  (`7956001d0`);
- `SERVICE_STATE_REFRESH_FAILED` sonrasında Bileşenler şeridi değişikliklerin
  kilitli olduğunu söylüyordu ve satırın fieldset'i her düğmeyi gerçekten
  devre dışı bırakıyordu; ama Yönet, DNS motorunu yönet, Yerleşik SFTP'yi
  kullan, sürümler düğmesi ve Kaldır etkin görünümlerini koruyordu; artık
  soluklaşır ve izin verilmeyen imleci gösterirler (`01861bfc9`);
- çevrilmiş bir açıklama 390 px'te üç noktayla kesiliyordu ve bütününü okumak
  için başlığı yoktu; artık satır kırar (`01861bfc9`);
- Dashboard'ın duran hizmet bağlantısı, menünün Bileşenler dediği sayfa için
  "Servislere git" / "Go to services" diyordu; artık "Bileşenleri aç" / "Open
  Components" der (`e2853c873`);
- birden çok posta profili gereksinimi tek bir uzun cümle gibi okunuyordu;
  artık çoğul bir listedir (`370afa2c9`);
- bileşen durumu bilinmeyen bir Mağaza ürünü "Bu sunucuda desteklenmiyor"
  taşıyordu (`1f564fdb0`).

Yedinci bir düzeltme fotoğraflardan değil `npm run build`'den geldi: paket
bütçesi (`8a3729cbe`, yukarıda).

**Neler değişmiyor.** Veritabanı şeması değişmedi (defter 44'te kalır;
alpha.85 etiketi ile bu aday arasında geçiş yok) ve yeni yol yok. Güncelleme
yolu, kurtarma yürütücüsü, sürüm tabanı ve yerel hizmetler değişmedi. Agent
yalnız posta sağlık yanıtını değiştirir.

## API değişiklikleri

Yeni yol yok. Ek alanlar ve bir yeni ret kodu.

### `reverse_dns_resolver`, `reverse_dns_resolvers_disagree` (Agent posta sağlığı)

Agent'ın Panel'e verdiği posta sağlık yanıtında, değişmeyen alanların yanında.
`reverse_dns_resolver`, bildirilen PTR'nin yanıtı olduğu genel çözümleyicinin
IP adresidir; `reverse_dns_resolvers_disagree`, iki çözümleyici farklı yanıt
verdiğinde `true` olur. Arama yanıt vermediyse ikisi de yazılmaz; eski bir
Agent onları hiç göndermez. Kurulum anlık görüntüsünde bir
`reverse_dns_mismatch` kontrolünün `vars` alanına `resolver` (yalnız geçerli
bir IP adresi) ve `resolvers: "disagree"` eklenir. Onları tanımayan istemci
yok sayar.

### `blocked_vars.platform`

`blocked_code` değeri `component_not_installable` olan bir posta profilinde,
sunucunun paket ekosistemi doğrulanmışsa: `"Debian/Ubuntu (apt)"`, `"Arch
Linux (pacman)"` ya da `"Fedora/RHEL (dnf)"`. Aksi durumda yoktur.

### `400 USERNAME_RESERVED`

Hesap oluşturma, ekip üyesi oluşturma ve ekip üyesini yeniden adlandırma,
`system` kullanıcı adını her harf biçiminde `400` ve `USERNAME_RESERVED`
koduyla reddeder. Var olan bir üyenin adını değiştirmeyen yeniden adlandırma
reddedilmez.

### Yeniden yazılan sunucu cümleleri

`503 SERVICE_STATE_UNVERIFIED`'ın İngilizce metni ve Mağaza'nın üç
`component_state_*` cümlesi (İngilizce ve Türkçe) yeniden yazıldı; kodları
değişmedi.

## Fark edeceğiniz davranış değişiklikleri

- Bir güncelleme içinde panelin kendi yeniden başlaması sırasında güncelleme
  sayfası dönen bir simge, "Panel, güncellemenin bir parçası olarak yeniden
  başlıyor…" ve T+ sayacını gösterir; güncelleme kartının yanında köşe
  bildirimi çizilmez. Uyarı işareti ve "bilinmiyor" ancak başlangıçtan 6
  dakika ve ilk kopan okumadan 2 dakika sonra gelir. Bunu bu sürümden sonraki
  güncellemede görürsünüz.
- Sayfanın dil özniteliği sayfanın çizildiği dili izler.
- Türkçe sayfalarda bileşen adları ve açıklamaları Türkçedir.
- "Son etkinlik" her eylemi bir cümleyle söyler; oturum açmış kullanıcısı
  olmayan eylemler için "Sistem".
- Yeni bir hesap ya da ekip üyesi `system` adını alamaz.
- Hiçbir cümle Yeniden tara'ya basmanızı istemez. Doğrulanamayan bir bileşen
  kaydını Dashboard, Bileşenler sayfası ve bileşenin kendi sayfası
  kendiliğinden kontrol eder; bir değişiklik yüzünden tutulan kontrol
  kendiliğinden sürer.
- Mağaza, bileşen durumu bilinmeyen ürünler için kendi süzgeç girdisiyle
  "Durum henüz bilinmiyor" gösterir.
- Kurma penceresinin ve kurulum sayfasının okumaları sekme gizliyken durur ve
  gösterilince hemen okur; güncelleme kartı ve yayılan alan adı, sekme yeniden
  gösterilince hemen okur.
- Posta profili retleri platformu ve rolleri adlandırır.
- Kurulumun ters DNS cümlesi, Agent güncellemesinden sonra çözümleyiciyi
  adlandırır.

## Ne ölçüldü ve nasıl

**Bileşen testleri.** Bu adayın ürün kodu `21f07e7a8`'inkidir; sürüm hazırlığı
`313f3eea2` `cmd/`, `internal/` ya da `web/` altında hiçbir dosyayı
değiştirmez.

- Web: `npm test` 1463/1463; [OPERATION-GUIDANCE](OPERATION-GUIDANCE.tr.md)
  içinde (15 Ekim 2026 kaydı) `00138c530` için kaydedildi ve 15 Ekim 2026'da
  sürüm hazırlığı commit'i `313f3eea2`'de yeniden koşuldu (aynı sayı;
  `00138c530`'dan sonra `web/src` ya da `web/tests` altında hiçbir dosya
  değişmedi); `npm run build` tür denetimi ve paket bütçesiyle geçer (en büyük
  zaman uyumsuz parça, Türkçe ekran kataloğu, 152,44 KiB). Web sayısı
  alpha.85'te 1391, yeniden başlama bildiriminden sonra 1404, görünürlük
  sınırlamasından sonra 1409, `1abe1717d`'de 1435, `297f1f0b1`'de 1457 ve
  `00138c530`'da 1463 idi.
- Go, WSL konuğu `CelikPanel-S2-Debian` içinde:
  `go test -v ./cmd/panel/... ./internal/transport/... ./internal/auth/...`
  1602 üst düzey test ve 1655 birinci düzey alt test geçti, 6 atlandı, 0
  başarısız; `8fe746582` için kaydedildi ve 15 Ekim 2026'da `21f07e7a8`'de
  aynı sayılarla yeniden koşuldu (kaydedilen koşudan sonra `a6bdf410b` ve
  `21f07e7a8`, `cmd/panel/` altında reti ve Mağaza cümlelerini ve bir testi
  yeniden yazdı); `go test ./cmd/agent/...` 1799 test ve 2829 alt test geçti,
  29 atlandı, 0 başarısız; `1abe1717d` için, 14 Ekim 2026 denetim kaydının
  temiz bağlama ad alanında kaydedildi (konuk kendisi çalışan bir CelikPanel
  sunucusudur). `1abe1717d`'den sonra `cmd/agent/` altında hiçbir şey
  değişmedi; `internal/` altında yalnız `internal/auth` değişti ve onu yalnız
  `cmd/panel` içe aktarır; Agent yeniden koşulmadı.
- Sürüm hazırlığının sözleşme testleri geçer: sıra politikası sözleşmesi,
  okuduğu `v0.1.0-alpha.85` etiketine sahip Windows Git Bash kopyasında ("85 ->
  86; historical bootstrap sequence 85"); indirme portalı ve imzalı manifest
  sözleşmeleri WSL konuğunda.

**On üçüncü tarayıcı inceleme turu** (`u86restart`,
`web/tools/browser-inspect/scenarios-batch13.mjs`, `15d80851e`). Windows'taki
gerçek bir Chrome, geri döngü taklidine karşı, bir "çalışıyor" okumasından
sonra durum okuması düşürülerek gerçek güncelleme sağlayıcısını çalıştırdı: üç
durum (güncelleme sayfasında sakin yeniden başlama satırı; sayfanın `Date.now`
değeri test tarafında 7 dakika ileri alınarak pencere sonrasında aynı sayfa;
yeniden başlama sırasında Alan adları sayfasında köşe bildirimi), 8
yapılandırmada (1280×800 ve 390×844, İngilizce ve Türkçe, açık ve koyu), 24
kayıt. Yeniden başlamada: dönen simge, uyarı işareti yok, güncelleme sayfasında
köşe bildirimi yok, Alan adları sayfasında köşe bildirimi sakin; pencere
sonrasında: uyarı işareti ve bilinmiyor metni; hiçbir sayfa yana kaymadı.

**On dördüncü tarayıcı inceleme turu** (`c86*`,
`web/tools/browser-inspect/scenarios-batch14.mjs`, `a45c1bfcb`). Altı senaryo,
23 durum (27 bileşenin bütün kataloğu, üstte ve bütünüyle; çevrilmiş bir
açıklamayla arama; kurma ve kaldırma pencereleri; `SERVICE_STATE_UNVERIFIED`
ile reddedilen bir bileşen sayfası; kilitli satırlarıyla
`SERVICE_STATE_REFRESH_FAILED`; yüklemede kontrol edilen kayıtlı kayıt; posta
profili kartları; Son etkinlik ve duran hizmet satırları; beş ters DNS cümlesi;
Mağaza'nın üç `component_state_*` cümlesi, üstte ve bütünüyle; dil değişimi ve
geri dönüşten sonra belge dili), aynı 8 yapılandırmada. Her kayıt belge dilini,
Türkçe sayfadaki İngilizce katalog metnini, ham kimlikleri, kodları ve
belirteçleri, görünür bildirimleri, düğmelerin devre dışı durumunu ve yana
kaymayı ekler. Altı kusuru yukarıda listelendi. İki turun da resimleri depoda
saklanmaz ve turların koşu sonuçları bir dosyada kaydedilmedi; 23 durum
senaryo kaynağından sayıldı.

### Gerçek sistem kaydı set17

*Yayımlamadan önce set17 README'sinden doldurulacak.*

**Bu daldaki düzenek ve laboratuvar aracı değişiklikleri.**

- `15d80851e`, `a45c1bfcb`: on üçüncü ve on dördüncü tarayıcı inceleme turları.
- `f89551e9b`: yayımlayıcının kendi testi (yukarıda).
- `498dc49e1` (`main`'den birleştirilen #212 çekme isteği): WAL izleme
  testindeki izleyici ölümü testi, iki `/proc` okuması arasında kaybolan bir
  görevi CI'ı bu yarışta düşürmek yerine çalışmıyor sayar
  (`trace_fixture.is_running`). Yalnız laboratuvar kodu.

## Bu sürümün sınırları

Bunlar bilinir. Gizli kusur değildir.

Bu sürümle yeni:

- **Henüz gerçek sistem koşusu yok.** Bu taslak yazıldığında bu adayın gerçek
  sistem kaydı yoktu; yukarıdaki her şey bileşen testlerine ve taklit
  tarayıcıya dayanır. "Gerçek sistem kaydı set17" bölümü yayımlamadan önce
  doldurulur.
- **6 dakikalık sınır iki laboratuvar güncellemesine ve bir sahip gözlemine
  dayanır.** set16'nın iki iyi güncellemesi (51 sn ve 48 sn) ve sahibin
  gerçek güncellemesi (T+02:59'da yanıt yok). Yeniden başlaması
  başlangıcından itibaren 6 dakikadan ve ilk kopan okumadan itibaren 2
  dakikadan uzun süren gerçek bir güncelleme, hâlâ olağanken bilinmiyor
  metnini gösterir. Yeniden başlama sağlayıcısı bir bileşen testinde
  bağlanmadı; yalnız taklit tarayıcıda çalıştı.
- **Yeni güncelleme ekranı ancak sonraki güncellemede görülür.** Bu sürüme
  güncelleme, sunucunun çalıştırdığı sürümün ekranlarıyla izlenir (iki kurulu
  sunucuda da alpha.85); bu yüzden sahip o güncelleme sırasında alpha.85
  davranışını görür.
- **Çözümleyici cümlesi Agent güncellemesi ister.** Agent alanları bir
  sunucuya ancak Agent güncellemesiyle ulaşır; o zamana dek sayfa önceki
  cümleyi korur. Çözümleyici karşılaştırması gerçek çözümleyicilere karşı
  üretilmedi.
- **Denetim günlüğünde aktör türü yok.** Önceden var olan `system` adlı bir
  kullanıcı satırı (bu Panel oluşturamaz) yine "Sistem" çizilirdi.
- **Mağaza cümleleri sunucu metnidir.** Mağaza'nın `component_state_*`
  cümleleri sunucuda yerelleştirilir ve kontrolü yalnız bir yönetici üç
  sayfadan birini açık tuttuğu sürece vaat eder; kimse bakmazken hiçbir şey
  kontrol etmez (tasarım gereği, D-032).
- **Hiçbir kurulu sunucu ölçülmedi.** Bu adaydaki hiçbir şey bir kurulu
  sunucuya ulaşmadı.

Bu sürümün yanıtladıkları, kaynakta ve bileşen testleriyle (gerçek sistem
koşusu yok): beklenen yeniden başlama sırasında güncelleme ekranının sorun gibi
görünen bildirimi (sahibin 15 Ekim 2026 tarihli bildiriminin (c) maddesi);
yayımlama betiğinin `$PSScriptRoot` kusuru; görünürlüğe bağlanmamış okumalar;
set16'nın O1–O3 gözlemleri ve iki kurulu sunucuda "Son etkinlik" ham kimlikleri
(h); "Şablonlu metin" sınırı (`component_not_installable` platformu, belirteç
olarak gösterilen roller, posta profilleri dışında "yeniden tara"); "Yanıt
veren çözümleyici adlandırılmaz" (kaynakta; Agent güncellemesi ister).

v0.1.0-alpha.85'in bu sürümün ele almadığı sınırları (her biri hâlâ açık),
özetle; tam liste [alpha.85 notlarında](RELEASE-NOTES-v0.1.0-alpha.85.tr.md):

- "Kurulum tamamlanıyor" ve güncelleme kartının meşgul durumuna gerçek
  sistemde ulaşılmadı; bileşen kilidinin yeniden bağlanmasına, yayılan alan adı
  kontrollerine, kurma penceresinin yeniden okumasına, duraklamalara ve HSTS
  kalan süresine de; yürütücünün kendi yeniden kontrolü aynı karşılanmamış
  sonucu bulduğunda tamamlama 60 saniyelik bildirimde biter.
- alpha.85'in sonraki çözümleyici değişikliği ancak Agent güncellemesinden
  sonra etkili olur; sahip Agent sürümünü bildirmedi.
- Geri dönüşten sonra kurtarma temel dosyası, çalışan yapı önceki sürüm iken
  hâlâ kusurlu adayı adlandırır ve taban adayın sırasını okur (set12, set15,
  set16). Açık.
- alpha.85 güncellemesinden sonra sahibin bildirmedikleri: güncelleme
  sırasında tam ekran lisans sayfası, Agent'ın sürümü, Frankfurt'un güncelleme
  sonrası kurulum son adımı, `postconf -n >/dev/null` çıktısı ve Frankfurt'un
  güncelleme sonrası bileşen kontrolünün sonucu.
- Arch; temiz bir Ubuntu kurulumu; Posta paketleri kartının "bir değişiklikle
  tutuluyor" durumu; değişikliklerinden sonra 9 ve 11. setlerin soğuk yükleme
  araçları; ve alpha.85 notlarının hâlâ açık saydığı v0.1.0-alpha.84 sınırları.
- **Açık kabul işleri.** [Dayanıklılık sözleşmesinin](RESILIENCE-CONTRACT.tr.md)
  beş temel işinin hepsi kısmi kalır; bu sürüm hiçbirini kapatmaz ve
  ilerletmez. P0.1–P0.5 **kısmi** kalır. Bu sürümü yayımlamak onun kabulü
  olmaz. Kararlar: [DECISIONS](DECISIONS.tr.md) içinde D-022, D-024, D-025,
  D-032 ve D-033.

## Yayımlama kaydı

Yayımlanmadı. Yayımlanana dek burada hiçbir şey kaydedilmez. Yayımlamadan önce
açık olanlar:

1. Yayımlama öncesi ölçüm: set17; bu taslak yazıldığında yapılmamıştı.
   Rakamları README'sinden yukarıdaki "Gerçek sistem kaydı set17" bölümüne
   girer.
2. Sürüm hazırlığı: `313f3eea2` commit'inde yapıldı (numarayı sahibin 13 Ekim
   2026 tarihli kalıcı talimatı uyarınca sürümü hazırlayan seçti); `cmd/`,
   `internal/` ya da `web/` altında hiçbir dosyayı değiştirmez: önceki olarak
   alpha.85 sürüm commit'i `36c33c4531ee6c1306d324db7a666403176f3bf7` ile
   85'ten sonra 86 sürüm sırası politikası, `download-portal/get.sh`
   sabitlemeleri (sıra 86 ve sürüm v0.1.0-alpha.86; açık anahtar sabitlemesi
   değişmedi), README sürüm satırları ve geçen üç sözleşme testi. Hâlâ açık:
   etiketten önce ayarlanan depo değişkeni `CELIKPANEL_RELEASE_SEQUENCE=86`.
   Gerçek arşivi CI etikette derler ve imzalar.
3. Çekme isteğinin son başı kendi CI koşusunu alır.
4. Birleştirme, ana commit üzerindeki koşu, etiket, altı imzalı varlığın imzalı
   sürüm sözleşmesinin anlattığı biçimde denetimi ve indirme portalının
   yayımı. Sahibin 15 Ekim 2026 tarihli talimatıyla bunlar sürümü hazırlayanın
   işidir.
5. Sürüm yayımlandıktan sonra her kurulu sunucunun sahibi o sunucuyu panelin
   kendi güncelleme ekranından günceller. Bu, yukarıdaki her adımdan ayrı bir
   adımdır; başka kimse başlatmaz.

Bu sürümü yalnız CelikPanel'in güncelleme arayüzünden kurun. Yayımlanması kurulu
sunucuları güncellemez; bir sunucunun güncellemesini yalnız sahibi, panelin
kendi güncelleme ekranından başlatır.
