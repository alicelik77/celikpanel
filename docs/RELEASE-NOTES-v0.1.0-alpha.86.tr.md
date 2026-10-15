# v0.1.0-alpha.86 (taslak, yayımlanmadı)

[English](RELEASE-NOTES-v0.1.0-alpha.86.md)

Bu, v0.1.0-alpha.85'i izleyen sürümün taslağıdır. v0.1.0-alpha.86 numarasını,
sahibin 13 Ekim 2026 tarihli kalıcı talimatı uyarınca ("hem sürüm numaraları
hem de commit işlemleri için bana sormana gerek yok": sürüm numaraları ve
commit'ler için sahibe sorulmaz) sürümü hazırlayan seçti; sahibin bu sürüm için
adını koyduğu bir numara değildir. Sahibin bu sürümdeki kendi kararları
şunlardır: kural (D-033, 14 Ekim 2026), aşağıda anlatılan 15 Ekim 2026 gözlemi
ve "devam. sorun yok." yanıtı (v0.1.0-alpha.86'yı eldeki kalemlerle yayımla;
ikinci sunucu işi sonra). Aşağıdaki F1 kilit kararı sürümü hazırlayanındır.
Sürüm hazırlığı `313f3eea2`
commit'idir (85'ten sonra sıra 86; önceki olarak alpha.85 sürüm commit'i
`36c33c4531ee6c1306d324db7a666403176f3bf7`). Hiçbir parçası yayımlanmadı ve bu
taslak onu çalıştıran hiçbir kurulu sunucu kaydetmez. Bu taslak 15 Ekim
2026'da (UTC) yazıldı ve aynı gün (UTC), set17 ve set17b gerçek sistem
kayıtlarından ve onlardan doğan üç web düzeltmesinden sonra tamamlandı; adayın
son ürün commit'i `7265d3c99`'dur.

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

Yaptıkları; bileşen testlerine, API taklidine karşı üç tarayıcı incelemesine
(on üçüncü tur: yeniden başlama bildirimi; on dördüncü: küçük düzeltmeler; on
beşinci: yeniden başlama sırasında lisans kapısı, oturum kapısı ve gezinti
kilidi) ve bu adayın daha önceki commit'lerinde yapılmış iki gerçek sistem
kaydına dayanır: sürüm hazırlığı `313f3eea2`'de set17 ve `ecd2a5f08`'de
set17b ("Gerçek sistem kayıtları set17 ve set17b" bölümü). set17 bir kusur
buldu (D1: lisans kapısı yeniden başlamanın üstüne kendi penceresini çizdi);
set17b, sürümü hazırlayanın ardından değiştirmeye karar verdiği bir davranışı
kaydetti; bu karar D-033 ve sahibin 13 Ekim 2026 tarihli kalıcı talimatı
altındadır, sahibin kararı değildir (F1: gezinti kilidi uzun bir yeniden
başlamanın ortasında bitti). İkisi de bu
adayda düzeltildi; F1 düzeltmesi (`7265d3c99`) son gerçek sistem kaydından
sonra geldi. Hiç çalıştırılmayanları sınırlar bölümü söyler.

## Ne göreceksiniz

Önce sade sözlerle, sunucu sahibi için. Güncellemeden sonra açık olan her
CelikPanel sayfasını yeniden yükleyin; güncellemeden önce yüklenmiş bir sayfada
eski ekranlar durur. CelikPanel'i bir dahaki sefer kendi güncelleme ekranından
güncellediğinizde, panelin o güncellemenin parçası olarak yeniden başladığı an
"Panel, güncellemenin bir parçası olarak yeniden başlıyor. Bu sayfa
kendiliğinden yeniden bağlanır; hiçbir şeye basmanız gerekmiyor." der; yanında
dönen bir simge ve süre sayacı vardır ve iki bildirim yerine bir bildirim
görünür. O yeniden başlama sırasında sayfayı hiçbir lisans ya da oturum
penceresi örtmez; olağan iki dakikalık gezinti kilidi bittiğinde panel hâlâ
yeniden başlıyorsa kilit, panel yeniden yanıt verene dek, en geç
güncellemenin başlangıcından 6 dakika sonrasına kadar sürer. Panel, güncellemenin başlangıcından yaklaşık 6 dakika sonra (ve yanıt
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
yapıyordu (`web/src/components/SystemUpdateOperation.tsx` içinde kayıt olmadan
çağrılan `systemUpdateOperationPhase(outcome.operation)`; işlev
`web/src/lib/systemUpdateTracking.ts` içindedir); güncellemenin planladığı
yeniden
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
taklidine karşı gerçek Chrome'da, set17 ve set17b'de QEMU konuklarındaki
gerçek Panellere karşı çalıştı (aşağıda).

Gerçek sistem kayıtları ardından sayfanın iki başka parçasının sakin yeniden
başlamanın yerine hâlâ geçebildiğini ve gezinti kilidinin uzun bir yeniden
başlamanın ortasında bitebildiğini gösterdi. Aşağıdaki üç düzeltme yalnız
web'dedir; hiçbiri Go dosyası, şema ya da sürüm geçişi değiştirmez; her biri
yalnız okur ve yalnız tarayıcının saatini kullanır (D-032).

**Lisans kapısı yeniden başlama sırasında sessiz kalır (set17 D1,
`e8aa9c5c0`).** set17'nin iki aday koşusundan birinde lisans erişim kapısı
sakin yeniden başlamanın üstüne T+55,9 sn'de kendi penceresini çizdi ("The
Panel is not answering; an update may be restarting it" / "Panel yanıt
vermiyor; bir güncelleme onu yeniden başlatıyor olabilir") ve güncellemenin
gezinti kilidi geri gelmedi. Kaynakta bulunan neden: kapının son olumlu kararı
kesinti sırasında süresini doldurdu; kapı ardından tutma ekranını çizdi ve
gözlem kurtarmasını başlattı; `App.tsx` bunu
`publishSystemUpdateAuthentication(false)` yapar; güncelleme sağlayıcısı
izlemeyi duraklattı, kilit artık istenmedi ve gezinti kirası artık istenmeyen
etkin bir kilidi kalıcı olarak bırakır (`useSystemUpdateNavigationLease.ts`,
tasarım gereği; değişmedi). Sağlayıcı artık, sunucu güncellemenin etkin
olduğunu söyledikten sonra izlenen güncellemeyi ilk kopan okumasıyla birlikte
yayımlar (`web/src/lib/systemUpdateRestartSignal.ts`). O güncelleme Panel'i
yeniden başlatırken yanıt alamayan bir erişim okuması, kapının son olumlu
kararını güncellemenin kendi yeniden başlama penceresi bitene dek korur
(başlangıçtan 6 dakika, ilk kopan okumadan en az 2 dakika; kararın kendi
bitişinden en fazla 6 dakika sonraya kadar). Kapı o sırada hiçbir şey çizmez,
sekme görünürken erişimi 5 saniyede bir okur ve izleme Panel'e yeniden
ulaştığında hemen bir kez daha okur; Panel'den gelen her yanıt eskisi gibi
uygulanır ve pencereden sonra var olan pencere görünür (`LicenseOnboarding.tsx`,
`SystemUpdateOperation.tsx`). Yeni metin yok. Testler:
`web/tests/license-onboarding-runtime.test.mjs` içinde denetimli saatli beş
bağlanmış durum (dördü önceki kapıya karşı başarısız olur; güncellemesiz
denetim ikisinde de geçer) ve `update-restart-notice-mounted.test.mjs` içinde
üç. set17 kaydının kendisi, klasörünün mekanizmayı kanıtlamadığını söyler
(başarısız erişim okumaları ve kapının göründüğü an tek bir kurala uymaz);
yukarıdaki neden kaynaktan okunmuştur. Bu değişikliği taşıyan bir adayda
set17b, iki koşuda pencereyi göstermedi.

**Yeni Panel başlarken oturum tutması yok (`b7b0f4766`).** Aynı sınıftan bir
kusur; D1 düzeltilirken kaynakta bulundu, iki kayıt da bir oturum tutması
göstermez (set17b dört koşuda onu aradı; set17'nin diğer pencere örneklemesi
kısmiydi: Debian denetim koşusunda örneklenmedi ve aracı her görünür pencereyi
ancak adayın kendi sayfasının a koşusundan sonra örnekledi). Yeni Panel
başlarken yönetimi, güncellemenin kendi durum
okuması dahil `503 PANEL_STARTING` yanıtı verir ve `App.tsx` (AuthGate)
içindeki fetch yakalaması böyle ilk yanıtta oturumu kullanılamaz işaretliyordu:
sayfalar "Panel başlatılıyor" ile tutuldu, izleme duraklatıldı ve kilit kalıcı
olarak bırakıldı. İzlenen güncelleme Panel'i yeniden başlatırken böyle bir
yanıt artık oturumu işaretlemez (`web/src/lib/useUpdateRestartStarting.ts`;
sınır `systemUpdateSessionGraceEnd`: aynı pencere, izlemenin ilk kopan
okumasından ya da sayfanın hiç ileri alınmayan ilk `PANEL_STARTING`
yanıtından sayılır). İçinde hiçbir şey çizilmez, izleme sürer ve Panel'in
hazırlığı (`/api/v1/panel/availability`) sekme görünürken 5 saniyede bir
okunur; hazır yanıtı beklemeyi bitirir, 401 oturumu yine bitirir ve pencere
hazır yanıtı olmadan biterse oturum eskisi gibi işaretlenir. Yeni metin yok.
Testler: `web/tests/access-hold-runtime.test.mjs` içinde denetimli saatli üç
bağlanmış durum (ilk ikisi önceki `App.tsx`'e karşı başarısız olur;
güncellemesiz denetim ikisinde de geçer) ve
`update-restart-notice-mounted.test.mjs` içinde bir. Hâlâ açık
([OPERATION-GUIDANCE](OPERATION-GUIDANCE.tr.md), 16. ve 17. kalemler):
`AUTH_STATUS_UNAVAILABLE` yanıtı oturumu yine hemen işaretler; kilit bittikten
sonra ve pencere bitmeden sahibin açtığı sayfalar, hâlâ başlayan bir Panel'in
kendi retlerini gösterebilir (artık yalnız iki dakikalık kiradan sonra başlayan
bir yeniden başlamada ya da yeniden bağlantı ile güncellemenin sonucu
arasında).

**Gezinti kilidi yeniden başlamayı kapsar (set17b F1, `7265d3c99`).**
Yeniden başlama kesintisini laboratuvarın yaklaşık 3 dakikaya uzattığı set17b'de
güncellemenin gezinti kilidi (tam sayfa penceresi) Panel hâlâ kapalıyken iki
dakikalık kirasıyla T+120,3 sn ve T+120,5 sn'de bitti; kart sakin kaldı ve
"Güncellemeyi kontrol et" devre dışı kaldı. Sürümü hazırlayan 15 Ekim 2026'da,
güncellemenin yeniden başlama sinyali etkinken kilidin yeniden başlamayı
kapsamasına karar verdi (OPERATION-GUIDANCE 17. kalem): D-033'ün bir
uygulamasıdır, sahibin 13 Ekim 2026 tarihli kalıcı talimatı altındadır; sahibin
kararı değildir (set17b'nin README'si soruyu sürümü hazırlayana bırakır). Artık
izleme, izlenen
güncellemenin okumasını iki dakikalık kira bitmeden kaybettiyse, hâlâ tutulan
kilit izleme Panel'e yeniden ulaşıp güncellemenin kaydını okuyana ya da yeniden
başlama penceresi bitene dek sürer; hangisi önce gelirse
(`web/src/lib/systemUpdateWatchdog.ts` içinde `systemUpdateNavigationDeadline`,
`systemUpdateRestartSignal.ts` içinde `systemUpdateNavigationRestart`,
`useSystemUpdateNavigationLease.ts`). Başlangıçtan 6 dakikayı hiç aşmaz; iki
dakikalık kiranın bittiği anda ya da sonra kopan okuma hiçbir şeyi uzatmaz;
bırakılan kilit hiç yeniden açılmaz; son an tekdüze bir dayanakla ölçülür,
duvar saatinin geri alınması onu uzatamaz. Yeniden başlama yoksa iki dakikalık
kira değişmez. Tek metin değişti, kilit satırı (`panelUpdate.interactionLocked`):
"Bu güncelleme denetlenirken gezinme geçici olarak kilitlidir. İki dakika sonra
kendiliğinden açılır; panel o sırada hâlâ yeniden başlıyorsa yeniden yanıt
verdiğinde, en geç başlangıçtan altı dakika sonra açılır. İşlem arka planda
izlenmeye devam eder." Testler:
`web/tests/system-update-navigation-lease-mounted.test.mjs` içinde denetimli
saatli yedi bağlanmış test (üçü önceki kiraya karşı başarısız olur; değişmeyen
kira, denetim ve saat testi ikisinde de geçer),
`update-restart-notice-mounted.test.mjs` içinde bir bağlantı testi ve
`panel-update-ui-contract.test.mjs` içinde satırın sınırları. `7265d3c99`'da
`npm test` 1483/1483. Bu değişiklik son gerçek sistem kaydından sonra geldi ve
gerçek sistemde ölçülmedi (sınırlara bakın).

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
biçiminde reddeder (`ErrReservedUsername`). Bir kişiden kullanıcı adı alan her
yol ondan geçer: hesap oluşturma, ekip üyesi oluşturma ve yeniden adlandırma,
yönetici CLI'si ve istemi (demo modu tohumu, hiçbiri `system` adını taşımayan üç
sabit hesabı onsuz oluşturur). İki HTTP yolu `400 USERNAME_RESERVED` döner
(kodlu; İngilizce ve Türkçe
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
yalnız posta sağlık yanıtını değiştirir. set17'den sonraki üç düzeltme
(`e8aa9c5c0`, `b7b0f4766`, `7265d3c99`) yalnız web'dedir.

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
- O yeniden başlama sırasında güncelleme sayfasını ne lisans penceresi ne
  "Panel başlatılıyor" örter; yeniden başlama iki dakikada hâlâ sürüyorsa
  gezinti kilidi panel yeniden yanıt verene dek (en geç başlangıçtan 6 dakika)
  sürer; kilit satırı bunu söyler.
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

**Bileşen testleri.** Bu adayın ürün kodu `7265d3c99`'unkidir: `21f07e7a8`
kodu artı yalnız web'deki üç düzeltme, `e8aa9c5c0`, `b7b0f4766` ve
`7265d3c99`. Sürüm hazırlığı `313f3eea2` `cmd/`, `internal/` ya da `web/`
altında hiçbir dosyayı değiştirmez; `7265d3c99`'dan sonraki commit'ler
`web/src`, `cmd/` ya da `internal/` altında hiçbir dosyayı değiştirmez
(yalnız `web/tools`, kanıt README'leri ve belgeler).

- Web: `npm test` 1483/1483; [OPERATION-GUIDANCE](OPERATION-GUIDANCE.tr.md)
  içinde `7265d3c99` için kaydedildi (17. kalemden sonraki sayılar; `npx tsc
  --noEmit` temiz ve `npm run build` bütçe içinde) ve 15 Ekim 2026'da
  `155a984ea`'da ve `70870dfad`'de aynı sayıyla yeniden koşuldu. Üç düzeltmeden
  önce: 1463/1463;
  `00138c530` için kaydedildi ve sürüm hazırlığı commit'i `313f3eea2`'de
  yeniden koşuldu (aralarında `web/src` ya da `web/tests` altında hiçbir dosya
  değişmedi); `npm run build` o zaman tür denetimi ve paket bütçesiyle geçti
  (en büyük zaman uyumsuz parça, Türkçe ekran kataloğu, 152,44 KiB;
  `70870dfad`'de aynı, orada `npm run build` yeniden geçti). Web sayısı
  alpha.85'te 1391,
  yeniden başlama bildiriminden sonra 1404, görünürlük sınırlamasından sonra
  1409, `1abe1717d`'de 1435, `297f1f0b1`'de 1457, `00138c530`'da 1463 ve
  `7265d3c99`'da 1483 idi.
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
  `cmd/panel` içe aktarır; Agent yeniden koşulmadı. `21f07e7a8`'den sonra
  `cmd/` ya da `internal/` altında hiçbir dosya (ne de `go.mod` ya da
  `go.sum`) değişmedi (`git diff --stat 21f07e7a8 155a984ea -- cmd internal
  go.mod go.sum` boştur); bu yüzden Go, yalnız web'deki düzeltmelerden sonra
  yeniden koşulmadı; yukarıdaki Go sayıları kaydedilmiş olanlardır.
- Sürüm hazırlığının sözleşme testleri geçer: sıra politikası sözleşmesi,
  okuduğu `v0.1.0-alpha.85` etiketine sahip Windows Git Bash kopyasında ("85 ->
  86; historical bootstrap sequence 85"); ve, sürümü hazırlayanın bildirdiği
  gibi, indirme portalı ve imzalı manifest sözleşmeleri WSL konuğunda (hiçbir
  commit'te kayıtlı değil ve bu belgelerin doğrulanmasında yeniden koşulmadı:
  Windows kopyasında anahtar dosyası denetimleri nedeniyle reddederler).

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

**On beşinci tarayıcı inceleme turu** (`d1restart`, `d1starting`;
`web/tools/browser-inspect/scenarios-batch15.mjs`, `59e793b2b`, `389ce2831`,
`155a984ea`), derlenmiş aday üzerinde aynı 8 yapılandırmada. `d1restart`: bu
tarayıcının başlattığı, kaydı 40 sn eski bir güncelleme; erişim 45 sn geçerli;
Panel iki dakikalık kirayı aşan 90 sn kapalı, sonra yeni sürümle geri. Kesintinin
60. saniyesinde (T+01:43) ve 88. saniyesinde (T+02:11) güncellemenin kilit
satırıyla sakin yeniden başlama penceresi ve etkisiz kılınmış uygulama; lisans
kapısı penceresi yok, tutulan sayfa yok, uyarı işareti yok, bağlantı şeridi
yok, yatay taşma yok; yeniden bağlantıdan sonra (T+02:16) pencere yok, etkisiz
uygulama yok, kart "doğrulanıyor"; ardından yeni sürüm, sekizinde de.
`d1starting`: Panel 25 sn kapalı, sonra 20 sn `503 PANEL_STARTING` (hazırlık
"starting"), sonra geri. Başlamanın 10. ve 18. saniyesinde yalnız
güncellemenin kilitli sakin yeniden başlama penceresi (iki aralıkta 4 ve 2 istek
`PANEL_STARTING` ile reddedildi); oturum tutması yok, "Panel başlatılıyor" yok,
kapı penceresi yok; ardından kilitle "Güncelleme kuruldu, doğrulanıyor",
sonra yeni sürüm. Sonuçlar [OPERATION-GUIDANCE](OPERATION-GUIDANCE.tr.md)
16. ve 17. kalemlerde ve commit iletilerinde kayıtlıdır; resimler depoda
saklanmaz.

### Gerçek sistem kayıtları set17 ve set17b

Bu bölümdeki her rakamın doğruluk kaynağı kaydın kendi README'sidir:
`deploy/e2e/release-recovery/evidence/set17-20261015/README.md` (on dokuzuncu
gerçek sistem kaydı; `4a29f4754`, mühür `0bf1ee601`'de tamamlandı) ve
`deploy/e2e/release-recovery/evidence/set17b-20261015/README.md` (yirminci;
`1f9495f7b`); ikisinin de ifadesi kabulde `d72e4528a` ile düzeltildi (hiçbir
ham dosya değişmedi). Bu özet ile bir README ayrışırsa README geçerlidir.
İkisi de ölçümdür, kabul kanıtı değildir: dışa NAT'lı QEMU/KVM konukları,
`release: false` ile test lisanslı (`acceptance_license`) yapılar, her
`result.json`'da `native_evidence: false`. Hiçbir kurulu sunucuya, lisans
hizmetine ya da sertifika yetkilisine gidilmedi; sürüm kaynağının adı ve
sertifika yetkilisi adları her laboratuvarın ilk sürücü adımında konuğun geri
döngüsüne sabitlendi ve sonunda yeniden okundu (set17'nin 7 ve set17b'nin 4
laboratuvarının hepsinde geçti). Hiçbir P0 satırını kapatmazlar.

**set17: sürüm hazırlığı `313f3eea2`'deki aday.** Koşu dizini 13:50:54Z'de
oluşturuldu, son laboratuvar 15:57:57Z'de bitti (15 Ekim 2026, UTC). Go
1.26.5, yapılar `git clone`'dan. Adayı hiçbir fikstür commit'i etiketlemez:
aday, kendi politikası (85'ten sonra 86) ve önyükleme sabitlemeleri 86 ile
kaynak commit'in kendisidir; `git diff --stat 21f07e7a8 313f3eea2 -- cmd
internal web` boştur.

| Yapı | Sürüm / sıra | Commit | Arşiv SHA-256 |
| --- | --- | --- | --- |
| taban | v0.1.0-alpha.85 / 85 | `36c33c45` (etiket commit'i, yerel test lisanslı yapı) | `31b03ddd762e20d0a841e728b75df321ba0fe96b15e9d1a568843d49d3245c16` |
| aday | v0.1.0-alpha.86 / 86 | `313f3eea` | `6c64289048f9e3fd98655bf761665c3b1f500ec654007da776d422654f747d39` |
| kusurlu aday | v0.1.0-alpha.86 / 86 | `6544ee8b1186ad42b38bd2b3b0cac1c8f07f50b1` (313f3eea + set5'in yalnız-taşıma kusuru) | `0397f4e85a3d60b2f82391166a95909cf04e6884ed7e5a2ecb33eb105011dfbb` |
| ikinci güncellemenin hedefi | v0.1.0-alpha.87 / 87 | `c48678d34583868ab583db4cfb6c97c36e423db6` (yalnız politika) | `887a16c0d3c7c1ce30371dbdc91661aadf6a9bf95663233ef8c35464d026ccfc` |
| onun kusurlu ikizi (hiç kurulmadı) | v0.1.0-alpha.87 / 87 | `8c6ac88b290b6662c8618878661b02857ff80348` | `8227bac036ac5ccdeb7fc29a4f00e1d56fad2d0b98bd2a93023458ffc56d2f86` |

Yerel alpha.85 yapısı yayımlanmış arşivle bayt bayt aynı değildir
(`cb6424b712052df2768f6c16dd6a66c8ea05a13393e71933c38ae639c3fb0792`;
laboratuvarlar dışında yayımlanmış sürüm manifestinden bir kez okundu).

| Hücre | Debian 13 | Ubuntu 24.04 |
| --- | --- | --- |
| adayın temiz kurulumu, oturum açma, lisans, kurulum `web` | GEÇTİ (14:30:23Z–14:40:33Z) | planlanmadı |
| alpha.85 → aday güncellemesi, iyi | `update_verified` (13:59:03Z–14:14:40Z) | `update_verified` (15:12:18Z–15:34:38Z) |
| güncelleme, kusurlu → kendiliğinden geri dönüş | `recovered` / `rollback_verified` (14:56:24Z–15:12:04Z) | aynı (15:34:51Z–15:57:57Z) |
| adayın kendi güncelleme sayfası (aday → v0.1.0-alpha.87 etiketi) | iki kez, `update_verified` (a koşusu 14:15:12Z–14:30:06Z, b koşusu 14:40:47Z–14:56:04Z) | planlanmadı |

Yedi laboratuvarın hepsi `complete-for-review`'a kadar koştu; altı güncelleme
laboratuvarının `verdicts` adımı geçti. Temiz kurulum `v0.1.0-alpha.86`,
`agent_matches true`, `schema_version 44`, defter `as-expected` ve kurulu
sabitlemeler 86 bildirdi. Kusurlu güncellemeye bir konuk sıfırlaması verildi
(Debian 15:07:28Z, SSH 8,91 sn sonra geri; Ubuntu 15:53:19Z, 11,62 sn); iki
kendiliğinden deneme yapıldı, sahip denemesi yok; sonuç `phase: recovered,
reason: rollback_verified, previous_failure: update_failed`, çalışan alpha.85
(`36c33c45…`); oturum açma, site işareti ve güvenlik duvarı aynı. Her geri
dönüşten sonra defter: `equal-except-volatile`, `unexpected []`, önce ve sonra
tek `managed_site_files` satırı, yalnız `observed_at` değişti; `stable_sha256`
önce ve sonra aynı (Debian `7efbb2c8…`, Ubuntu `45c5e796…`); 67 tablo
karşılaştırıldı, beş değişken tablo dışarıda bırakıldı. İyi güncellemeler de
yalnız `observed_at` ile `equal-except-volatile` idi.

*Gerçek bir güncelleme sırasında güncelleme sayfası* (500 ms'de bir örneklendi;
T+, sahibin gerçek başsız Chrome'daki basışından; sayfanın kendi başlatma
isteği, sürücünün salt okur gözlemcisi kurulana dek 880 ile 1202 ms arasında
değiştirilmeden tutuldu; sayfanın 15 sn başlatma zaman aşımının altında):

| | alpha.85, Debian (denetim) | alpha.85, Ubuntu (denetim) | aday a koşusu | aday b koşusu |
| --- | --- | --- | --- | --- |
| ilk kopan durum okuması | T+41,0 sn | T+25,5 sn | T+25,6 sn | T+25,5 sn |
| sayfa kesintisi (ilk kopan → ilk yanıtlanan okuma) | 30,1 sn | 26,0 sn | 35,2 sn | 30,3 sn |
| kesinti sırasında | "could not be read just now … not known", uyarı yüzeyi (59 örnek) | aynı (51 örnek) | "The panel is restarting as part of the update…", birincil renkte dönen simge; 0 "not known", 0 uyarı | aynı; 0, 0 |
| kesinti sırasında köşe bildirimi | yok | yok | yok | yok |
| sayfanın üstünde başka pencere | görülmedi (o koşuda örneklenmedi) | lisans kapısı, T+36,5–51,3 sn | **lisans kapısı, T+55,9–61,0 sn (D1)** | yok |
| güncelleme penceresi (gezinti kilidi) | T+77,9 sn'ye dek | T+36,0 sn'ye dek (lisans kapısı) | T+55,4 sn'ye dek (lisans kapısı) | T+70,6 sn'ye dek |
| yeniden bağlantıdan sonra | köşe bildirimi **ve** kart "being verified" (T+78,4–83,0 sn) | ikisi (T+51,8–60,5 sn) | kart "verifying", ardından yalnız bilinen sonuç için köşe bildirimi (T+70,2–71,2 sn) | "Update installed, being verified" penceresi, ardından "The update completed…" |
| sayfada yeni sürüm | T+85,6 sn | T+63,0 sn | T+71,7 sn | T+71,2 sn |

"Check for updates" dördünde de yeni sürüm görünene dek devre dışı kaldı.
Sınıra yaklaşılmadı: en uzun sayfa kesintisi ilk kopan okumadan 35,2 sn (sınır
120 sn), en geç yeniden bağlantı basıştan T+71,0 sn (sınır 360 sn); sürücünün
ana makine döngüsü her iyi güncellemede Panel'i 25–35 sn kapalı gördü.

*Aday üzerinde tarayıcı denetimleri* (TR ve EN; temiz hücrede ve dört aday
güncellemesinin hepsinden sonra): Bileşenler sayfasında 27 adın 27'si ve 27
açıklamanın 27'si katalogla aynı; `document.documentElement.lang` Türkçe
sayfalarda `tr`, İngilizcede `en`; Son etkinlik cümle olarak, ham kimlik yok
(her birinde 7 satır); kenar çubuğu yapı kimliği 1280×800 ve 390×844'te tek
satır; Mağaza süzgeci "Durum henüz bilinmiyor"u "Bu sunucuda desteklenmiyor"dan
ayrı listeler (o durumda ürün yoktu); ve yalnız temiz Debian hücresinde bir
laboratuvar hazırlığıyla üretilen `SERVICE_STATE_UNVERIFIED` ile bir
bileşenin kendi sayfası: 0,5 sn'den itibaren sakin kontrol satırı, 200 yanıtlı
tek `POST /api/v1/managed-services/scan?max_age_seconds=300`, kaydın yeniden
okunması, sayfa 1,3 / 1,6 sn'de doldu, uyarı ya da köşe bildirimi yok. İki geri
dönüşten sonra alpha.85, alpha.85 gibi davrandı (`lang` `en`, 7 satırın 6'sında
ham kimlik, İngilizce sunucu adları). Kurulum son adımının çözümleyici cümlesine
ulaşılamadı (hiçbir laboratuvar konuğunun genel ters DNS adı yok; taklit
edilmedi).

*Bulgular.* **D1** (yeniden başlama sırasında lisans kapısı; yukarıda): bu
laboratuvarda iki aday koşusunun birinde ve iki denetim koşusunun birinde
doğrulanmış bir gözlem; güncellemenin ölçülmüş bir başarısızlığı değil; klasör
kapının ne zaman göründüğünü kanıtlamaz. **O1** (düzenek): Chrome'un ağ olayları
tutulan başlatma isteğinin yanıtını bildirmez (istek sayfanın service
worker'ından çıkar); her başlatma bir durum okumasıyla uzlaştırıldı. **O2**
(alpha.85): kilit kapandıktan sonra köşe bildirimi ve kart "being verified"
ifadesini yineler; adayda görülmedi. Kayıtta açık: her geri dönüşten sonra
temel dosyası, alpha.85 çalışırken hâlâ kusurlu adayı adlandırır
(`release-commit=6544ee8b…`, sıra 86) ve taban 86 okur; set12, set15 ve
set16'daki gibi.

**set17b: `ecd2a5f08`'deki aday, yaklaşık 3 dakikalık bir yeniden başlama
kesintisi boyunca.** Koşu dizini 17:05:07Z'de oluşturuldu, son laboratuvar
18:16:23Z'de bitti (15 Ekim 2026, UTC). Aday `313f3eea2`'ye göre hepsi
`web/src` altında beş ürün dosyasını değiştirir (D1'in lisans kapısı payı ve
oturum kapısının `PANEL_STARTING` payı); yine kendi politikasıyla kaynak
commit'in kendisidir. Yalnız Debian 13, dört laboratuvar, birer birer. Yapılar
(Go 1.26.5): aday `ecd2a5f0` →
`4569b5eb0dba432799be5f63abeec4afca48669ead46151dd10d8ae80ccd72d0`; alpha.85
`31b03ddd…`, set17'nin yerel yapısıyla bayt bayt aynı; v0.1.0-alpha.87 etiketi
`20a9baa5d0ccb269528a02ec8c14229bc2ee2493` →
`34c753c6158ecedfea501d3f9c5494557f12609b61e989bbc00cecdba82cb93e`; kusurlu
arşivler (`8acfb74a842488c46a8083cca05e2bb10fc1fb97` → `ab140cf3…`,
`0aa2bb3c1394aeee5cc7e153a8b96a0e4d0ee51b` → `1df2f933…`) yalnız yapı
belgeleri istediği için derlendi ve hiç kurulmadı.

*Uzun kesinti bir laboratuvar değişikliğidir, ürünün bir özelliği değildir.*
`ExecStartPre=/bin/sleep 150` içeren bir ek yapılandırma (drop-in),
güncelleyicinin kendi işlem korumasınca reddedilirdi (systemd'nin yalnız kendi
koruma ek dosyasını yüklemesini ister); bu yüzden hiçbir ek dosya ve hiçbir
kalıcı birim dosyası yazılmadı. Geçici bir yardımcı birim
(`guest_set17b_restart_delay.py`), güncelleyici `active` işaretini
`completion.pending` olarak yeniden adlandırınca (aday kurulu, Agent ve Panel
durdurulmuş) güncellemenin kendi işçi birimini `systemctl freeze` ile
dondurdu, 150,2 sn, 150,7 sn ve 150,3 sn (a, b, denetim) tuttu ve çözdü;
güncelleyici ardından Agent'ı (çözmeden 0,6–1,1 sn sonra) ve Panel'i
(1,6–1,7 sn) kendisi başlattı. İlk deneme, a0 koşusu, hiçbir şey tutmadı (H1)
ve sıradan kısa kesintili bir koşu olarak saklandı.

| | a0 koşusu (tutulmadı) | a koşusu | b koşusu | denetim (alpha.85'in sayfası) |
| --- | --- | --- | --- | --- |
| basış | 17:23:22.3Z | 17:39:19.5Z | 17:56:27.3Z | 18:11:23.3Z |
| ilk kopan → ilk yanıtlanan okuma | T+25,5 → 62,0 sn | T+25,5 → 202,8 sn | T+27,0 → 193,6 sn | T+25,5 → 194,6 sn |
| sayfa kesintisi | 36,5 sn | **177,2 sn** | **166,5 sn** | **169,1 sn** |
| kesinti sırasında | "restarting", 72 örnek | "restarting", 347 örnek | "restarting", 328 örnek | "could not be read just now … not known", T+25,6 sn'den 333 örnek |
| "not known" / uyarı yüzeyi | 0 / 0 | **0 / 0** | **0 / 0** | 333 / 333 |
| lisans kapısı penceresi | yok | **yok** | **yok** | T+59,8–194,3 sn (266 örnek), sayfalar tutuldu |
| kesinti sırasında başarısız lisans erişim okumaları | 7 | 39 | 35 | 31 |
| sayfaya `PANEL_STARTING` yanıtları | 0 | 0 | 0 | 0 |
| kilit satırlı güncelleme penceresi | yeniden yüklemeye dek (T+76,6 sn) | **T+0,5–119,8 sn (F1)** | **T+0,5–120,0 sn (F1)** | T+59,3 sn'ye dek (kapı) |
| pencereden sonra, kesinti sırasında | – | yalnız kart: "restarting", dönen simge, işleyen T+, köşe bildirimi yok | aynı | kapının penceresi; kart "unknown", uyarı |
| "Check for updates" devre dışı | T+76,6 sn'ye dek | T+217,3 sn'ye dek | T+208,2 sn'ye dek | T+210,5 sn'ye dek |
| yeniden bağlantıdan sonra | "being verified", ardından "The update completed… Reloading" | kart "verifying", ardından köşe "completed… Reloading" | aynı | köşe **ve** kart "being verified" (T+194,8–209,0 sn) |
| sayfada yeni sürüm | T+77,1 sn | T+217,8 sn | T+209,2 sn | T+211,0 sn |
| sürücünün son durumu | `update_verified` 17:24:33Z | 17:42:50Z | 17:59:51Z | 18:14:48Z |

Her laboratuvarın `verdicts` adımı geçti (web, SMTP ve cron hiç kesilmedi;
Panel `down-only-during-transaction`); her güncelleme `update_verified` ile,
`post-update-facts` geçerek ve şema defteri 44 `as-expected` ile bitti; her
veritabanı karşılaştırması `equal-except-volatile`, `managed_site_files`
yalnız `observed_at`'ta. a0 ve a koşuları yalnız bir düzenek adımının birim
okuması erken durduğu için (H2) `inconclusive` biter; b ve denetim koşuları
`complete-for-review` biter. Sınırlara göre: pencere T+360 sn'de bitiyordu;
yeniden bağlantı ondan 157 sn ve 166 sn önce geldi, bu yüzden "not known"a
doğru biçimde ulaşılmadı; 6 dakikalık gecikme metni üretilmedi (kesinti
tasarım gereği onun altında tutuldu). Aday koşularında son başarılı erişim
okumasından son başarısıza 205 sn ve 195 sn geçti, kapının penceresi olmadan;
denetim onu son başarılı erişim okumasından 60,3 sn sonra çizdi.

*Bulgular.* **D1** adayda yeniden üretilmedi (2 koşunun 0'ında) ve alpha.85'te
yeniden görüldü (1'in 1'inde). Kayıttaki çekinceler: set17b'nin adayı
set17'ninki değildir (D1'den sonra yazılan kapı payını taşır); D1 set17'de
zamanlamaya bağlıydı (2 aday koşusunun 1'inde), bu yüzden onsuz iki koşu
olamayacağını göstermez; mekanizma kanıtlanmadı. **F1**: kilit, ölçülen commit
`ecd2a5f08`'in belgelenmiş iki dakikalık kirasıyla T+120,3 sn ve T+120,5 sn'de
bitti; Panel 82 sn ve 73 sn daha kapalıydı; ardından gelen sakindi. Sonradan
`7265d3c99`'da değiştirildi (yukarıda); bu değişiklik bileşen testli ve 15.
turda incelendi, gerçek sistemde ölçülmedi. **O1**: `PANEL_STARTING` dört
koşuda sayfaya hiç ulaşmadı (systemd Panel'i çözmeden 1,6–1,7 sn sonra etkin
bildirdi ve sayfanın sonraki okumaları zaten `200` yanıtlandı); bu yüzden
oturum kapısının payı gerçek sistemde çalışmadı. **O2** (incelenmedi): Panel
birimi, güncelleyici onu durdurmuşken `failed` okundu ve güncelleyicinin kendi
başlatmasıyla `active`'e döndü. **O3** (alpha.85): yeniden bağlantıdan sonra
iki bildirim, set17'nin O2'si gibi. **H1–H3** (düzenek): güncelleyicinin
denetim noktası satırı sahibin güncellemesinin günlüğünde yoktur (işaret
tetikleyicisiyle düzeltildi); `set -eu` altında birim okuması (`set +e` ile
düzeltildi); tutulan başlatmanın yanıtı Chrome'ca bildirilmez (set17 O1 gibi).
Yalnız güncelleme sayfası gözlendi; set17'nin tarayıcı denetimleri
yinelenmedi. Konsol: kesinti sırasında yalnız `ERR_CONNECTION_CLOSED` ve oturum
açmadan önce iki `401`.

**Bu daldaki düzenek ve laboratuvar aracı değişiklikleri.**

- `15d80851e`, `a45c1bfcb`: on üçüncü ve on dördüncü tarayıcı inceleme turları;
  `59e793b2b`, `389ce2831`, `155a984ea`: on beşincisi.
- `4a29f4754`: set17'nin düzeneği (`set17_fixture.py`, `set17_trial.py`,
  `build-set17-artifacts.sh`, `run-set17.sh`,
  `web/tools/browser-inspect/cold-load-set17.mjs`); `1f9495f7b`: set17b'ninki
  (`set17b_trial.py`, `run-set17b.sh`, laboratuvar hazırlığı
  `guest_set17b_restart_delay.py`, `cold-load-set17b.mjs`). Yalnız
  laboratuvar kodu; her koşu sırasında onlarda yapılan değişiklikler
  README'lerde kayıtlıdır.
- `f89551e9b`: yayımlayıcının kendi testi (yukarıda).
- `498dc49e1` (`main`'den birleştirilen #212 çekme isteği): WAL izleme
  testindeki izleyici ölümü testi, iki `/proc` okuması arasında kaybolan bir
  görevi CI'ı bu yarışta düşürmek yerine çalışmıyor sayar
  (`trace_fixture.is_running`). Yalnız laboratuvar kodu.

## Bu sürümün sınırları

Bunlar bilinir. Gizli kusur değildir.

Bu sürümle yeni:

- **Gerçek sistem kayıtları daha önceki commit'leri ölçtü.** set17 sürüm
  hazırlığı `313f3eea2`'yi, set17b `ecd2a5f08`'i ölçtü. Son aday onlardan
  sonra yalnız web'de değişiklikler taşır: `7265d3c99` (ürün: yeniden başlama
  boyunca gezinti kilidi), `155a984ea` (on beşinci tarayıcı turunun senaryosu
  ve işlem yönlendirmesi; ürün dosyası yok) ve `d72e4528a` (iki README'nin
  ifadesi). Bu yüzden gerçek sistemde ölçülmeyen: F1 kilit uzatması, yani
  kilidin iki dakikadan sonra izleme güncellemenin kaydını yeniden okuyana dek
  (en geç başlangıçtan 6 dakika) tutulması, ve değişen kilit satırı. Bileşen
  testlerine ve taklide karşı on beşinci tarayıcı turuna dayanır. Adayın ürün
  kodunun geri kalanı set17b'nin ölçtüğü koddur.
- **Oturum kapısının `PANEL_STARTING` yolu gerçek sistemde çalışmadı.**
  `PANEL_STARTING` set17b'nin dört koşusunda sayfaya 0 kez ulaştı; pay bileşen
  testlerine ve 15. turun `d1starting` senaryosuna dayanır.
- **Lisans kapısının sessiz yolu iki gerçek sistem koşusuna dayanır.** set17b
  iki aday koşusunda D1'i göstermedi; D1 zamanlamaya bağlıydı (set17'de 2'nin
  1'i) ve mekanizmasını iki kayıt da kanıtlamaz.
- **6 dakikalık "gecikme" metni gerçek sistemde üretilmedi.** Her gerçek sistem
  aday koşusu yeniden başlama penceresi bitmeden yeniden bağlandı (set17: sayfa
  kesintisi en çok 35,2 sn; set17b: en çok 177,2 sn, tasarım gereği altında
  tutuldu; en geç yeniden bağlantı basıştan T+202,8 sn'de, pencere T+360 sn'de
  biter). set17b'nin 3 dakikalık
  kesintilerini laboratuvar yaptı (dondurulmuş güncelleme işçisi): ürünün bir
  özelliği değil, bir laboratuvar değişikliğidir ve gerçek yeniden başlamaların
  ne kadar sürdüğünün kanıtı değildir.
- **6 dakikalık sınır laboratuvar güncellemelerine ve bir sahip gözlemine
  dayanır.** set16'nın iki iyi güncellemesi (51 sn ve 48 sn), set17'nin
  sayfada gözlediği dört güncelleme (26,0 ile 35,2 sn arası sayfa kesintisi,
  en geç T+71,0 sn'de yeniden bağlantı) ve sahibin gerçek güncellemesi
  (T+02:59'da yanıt yok). Yeniden başlaması başlangıcından itibaren 6
  dakikadan ve ilk kopan okumadan itibaren 2 dakikadan uzun süren gerçek bir
  güncelleme, hâlâ olağanken bilinmiyor metnini gösterir. Yeniden başlama
  sağlayıcısı bir bileşen testinde bağlanmadı; taklit tarayıcıda ve set17 ile
  set17b'nin QEMU konuklarında çalıştı.
- **Platformlar.** set17 Debian 13'ü (temiz kurulum, güncelleme, geri dönüş,
  adayın kendi sayfası) ve Ubuntu 24.04'ü (yalnız güncelleme ve geri dönüş)
  koştu; set17b yalnız Debian 13'ü. Ubuntu 24.04 ve Arch set17b'de koşulmadı;
  Arch ve adayın temiz Ubuntu kurulumu ikisinde de koşulmadı.
- **Çözümleyici cümlesine gerçek sistemde ulaşılamadı.** Sunucunun genel ters
  DNS adını ister, hiçbir laboratuvar konuğunda yoktur; taklit edilmedi.
- **İncelenmedi:** güncelleyici durdurmuşken Panel biriminin neden `failed`
  okunduğu (set17b O2); tutulan üç koşuda (a, b, denetim) güncelleyicinin kendi
  başlatmasıyla `active`'e döndü.
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
  sunucuya ulaşmadı. Gerçek sistem kayıtları test lisanslı yapılarla QEMU/KVM
  konuklarıdır; ölçümdür, kabul değildir.

Bu sürümün yanıtladıkları, kaynakta ve bileşen testleriyle: beklenen yeniden
başlama sırasında güncelleme ekranının sorun gibi görünen bildirimi (sahibin 15
Ekim 2026 tarihli gözlemi; kurulu sunuculara ilişkin işlem rehberi girdisinin
(b) ve (c) maddeleri; ayrıca set17 ve set17b'de daha
önceki commit'ler `313f3eea2` ve `ecd2a5f08` üzerinde, denetim olarak
alpha.85'in sayfasıyla gerçek sistemde gözlendi); set17'nin D1'i (lisans
kapısı; set17b'nin iki koşusunda yeniden üretilmedi); oturum kapısının
`PANEL_STARTING` tutması (gerçek sistemde çalışmadı); set17b'nin F1'i (gerçek
sistemde ölçülmedi); yayımlama betiğinin `$PSScriptRoot` kusuru; görünürlüğe bağlanmamış okumalar;
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
  set16 ve yeniden set17). Açık.
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

1. Yayımlama öncesi ölçüm: yapıldı. `313f3eea2`'de set17 ve `ecd2a5f08`'de
   set17b; README'lerinden yukarıdaki "Gerçek sistem kayıtları set17 ve
   set17b" bölümünde özetlendi. Son adayın onlardan sonraki web değişikliğinin
   (`7265d3c99`) gerçek sistem kaydı yoktur; sınırlar bunu söyler.
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
