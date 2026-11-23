# v0.1.0-alpha.147

[English](RELEASE-NOTES-v0.1.0-alpha.147.md)

**Bir düzeltme de içeren bir güvenlik sürümü.** Veritabanı araçları ve web
postası artık web sunucusunun ortak hesabıyla çalışmıyor, site hesapları
sistemin `at` komutuyla iş zamanlayamıyor ve dağıtımın ortak PHP havuzu
hiçbir şey kullanmıyorsa kapatılıyor. Uzun süren bir değişiklik panelde artık
sonsuza dek "sürüyor" görünmüyor. Panelin veritabanı düzeni değişmez.
**Kurulu panelleri yalnız sahibi, panelin kendi güncelleme ekranından
günceller.**

## alpha.146'dan güncelleme

1. Panelin kendi güncelleme ekranından, eskisi gibi güncelleyin.
2. **Panelin veritabanı düzeni değişmez** (şema 81 olarak kalır). Ürünün
   geri dönüşüyle alpha.146'ya dönmek yalnız güncellemeden sonraki ilk sunucu
   değişikliğine kadar kabul edilir; aşağıdaki "alpha.146'ya dönmek"
   bölümüne bakın.
3. Panelin ilk açılışından yaklaşık üç dakika sonra, sunucu sakinken, panel
   kendi işinde tek bir adımı kendiliğinden çalıştırır: veritabanı araçlarını
   ve web postasını kendi hesaplarına taşır, dağıtımın PHP havuzunu hiçbir şey
   kullanmıyorsa kapatır, `at` komutunu site hesaplarına kapatır ve site
   crontab'larını kayda alır (bkz. "Güvenlik: müşteriler birbirinden daha iyi
   ayrılıyor"). Basılacak bir şey yoktur. Yapamadığı her şey **Şu anki
   sorunlar**'da, kimin ne yapacağıyla birlikte bir satırdır.

## Güvenlik: müşteriler birbirinden daha iyi ayrılıyor

- **Veritabanı araçları ve web postası web sunucusunun ortak hesabıyla
  çalışıyordu.** phpMyAdmin, phpPgAdmin ve web postası (Roundcube),
  dağıtımın ortak PHP havuzunda, grubu her sitenin klasörünü okuyabilen web
  sunucusu hesabıyla çalışıyordu. Birindeki bir hata her siteyi okuyabilirdi.
- **Artık her birinin kendi sistem hesabı var**: `celikpanel-webmail`,
  `celikpanel-phpmyadmin` ve `celikpanel-phppgadmin` (girişi ve ev klasörü
  yok, başka hiçbir grupta değil). Her birinin, aracın çalıştığı PHP
  sürümünde kendi PHP havuzu var; oturumları, yüklemeleri ve geçici dosyaları
  `/var/lib/celikpanel-php-tools/<araç>/` içinde. Bir aracın havuzuna yalnız
  web sunucusu ulaşır. Yeni kurulumlar bunu baştan alır; var olanlar yukarıdaki
  adımla taşınır. Taşıma, bir aracın Yönet penceresinde PHP sürümünü seçmekle
  aynı doğrulanmış adımdır: araç sonrasında kendisi olarak yanıt vermelidir;
  vermezse her şey olduğu gibi geri gelir ve **Şu anki sorunlar** bunu söyler.
  Aracın Yönet penceresinde PHP sürümünü seçmek taşımayı yeniden dener.
- **Web postasının klasörü her site hesabına kapalı.**
  `/var/lib/celikpanel-webmail` kökü, kurulumun bıraktığı gibi başka hesaplara
  kapalı kalır; içinden yalnız web postasının kendi hesabı, tek bir erişim
  listesi girdisiyle geçer (`user:celikpanel-webmail:--x`; sahip onu
  `getfacl` ile okur, `setfacl -x` ile kaldırır). Web postasının veritabanı,
  geçici dosyaları ve günlükleri onun hesabının olur; yapılandırmasını yalnız
  onun hesabı okur. Web postasının sunucudaki yerel kapısı (alpha.145'te
  CelikPanel'in kendi denetimleri dışındaki her şeye kapatıldı) PHP'sini de
  artık web postasının kendi havuzuna verir. **Bunun için web postası erişim
  listelerini destekleyen bir dosya sistemi ister:** dosya sistemi bunları
  kabul etmiyorsa web postası web sunucusu hesabında kalır, hiçbir şey
  değiştirilmez ve **Şu anki sorunlar** nedenini ve sahibin ne yapabileceğini
  (dosya sisteminin `acl` seçeneği) söyler.
- **Site hesapları artık `at` ile iş zamanlayamaz.** `at` paketi kuruluysa ve
  `/etc/at.allow` yoksa CelikPanel, bugün `at` kullanabilen her hesabı, site
  hesapları (ve bir site hesabının kimliğini paylaşan her hesap, örneğin ek
  bir FTP girişi) dışında listeleyen bir dosya yazar. Site hesaplarının
  bekleyen `at` işleri yöneticiye listelenir (numara, hesap ve zaman), asla
  silinmez. Sahibin yazdığı bir `/etc/at.allow` olduğu gibi korunur; andığı
  site hesapları yöneticiye gösterilir.
- **Bir sitenin panel dışında eklediği crontab satırları görünür.** Bir
  sitenin Zamanlanmış görevler listesinde CelikPanel'de yapılmamış bir satır
  "CelikPanel’de yapılmadı" diye işaretlenir: silinebilir; yönetici onu
  tutabilir. Agent, tutulana kadar böyle bir satırı değiştirmeyi ya da
  etkinleştirmeyi reddeder. **Şu anki sorunlar**'daki tek bir satır siteleri
  ve satır sayısını söyler.
- **Dağıtımın ortak PHP havuzu, hiçbir şey kullanmıyorsa kapatılır.** Her PHP
  sürümünün `www.conf` havuzu web sunucusu hesabıyla çalışır. Yalnız paketin
  dokunulmamış dosyasıysa, yanında başka bir havuz çalışıyorsa ve bir web
  sunucusunun yüklediği hiçbir şey PHP'yi ona vermiyorsa kapatılır (adı
  `www.conf.off-by-celikpanel` yapılır, `php-fpm -t` ile denetlenir ve
  yeniden yüklenir; biri reddederse geri konur). Şu durumlarda açık tutulur
  ve **Şu anki sorunlar** onu neyin tuttuğunu söyler: nginx yapılandırması
  soketini anıyorsa (başka bir yazılışla ya da bağlantıyla, izlenemeyen bir
  soket yoluyla, ya da CelikPanel'in kendisininki dışında değişkenle kurulan
  bir `*_pass` adresiyle de), Apache kuruluysa ve onu anan bir dosyayı
  yüklüyorsa, etkin ya da çalışan bir Apache `.htaccess` dosyalarının bir
  şeyi değiştirmesine izin veriyorsa, ya da başka bir web sunucusu (httpd,
  lighttpd, Caddy, OpenLiteSpeed) etkin ya da çalışıyorsa. Bir sürüm asla
  havuzsuz bırakılmaz: son diğer havuzu gitmeden önce havuz geri konur.
- **Sahibin kendi kararları geçerlidir.** Sahibin değiştirdiği bir aracın web
  sunucusu dosyası ya da havuzu olduğu gibi korunur. Sahip kapatılmış bir
  havuzu geri koyarsa ya da `/etc/at.allow` dosyasını silerse CelikPanel onu
  öyle bırakır ve **Şu anki sorunlar**'da tek bir düğmeyle ("Yeniden kapat")
  bir satır gösterir; düğme, panelin kendi onayından sonra yalnız o tek şeye
  dokunur. Başarılı olmadığında sonucu, panel yeniden başlasa da o satırın
  altında görünür kalır.
- **Bir site hesabının hâlâ ulaşabildiği.** Dosya sisteminde web postası
  klasörünün altında hiçbir şey. Araçların havuzlarına yalnız web sunucusu
  üzerinden. Müşteriler `*_pass` yönergesi yazamaz; ortak havuzun soketini
  zararsız bir yönergede anan müşteri o havuzu açık tutar ve bundan bir şey
  kazanmaz.

## Düzeltme: uzun süren bir değişiklik artık "sürüyor" diye kalmıyor

- **Şimdiye kadar** (alpha.145 ve alpha.146'nın bilinen sınırı), uzun süren
  bir değişiklik — bir web postası güncellemesi, bir bileşen, posta profili
  ya da Node.js kurulumu, birlikte kurulum ya da kaldırma, bir web postası
  adresi — panel agent ile bağlantısını bir an kaybettiğinde, sunucuda
  bittikten sonra da panelde "sürüyor" görünebiliyordu. Panel yeniden
  başlatılana kadar diğer sunucu değişiklikleri reddediliyordu.
- **Artık panel sunucunun ne yaptığını okur ve sonucu kendisi kaydeder.** Bir
  web postası güncellemesi ya da web postası adresi yalnız okunur; hiçbir şey
  yeniden çalıştırılmaz. Yarıda kalan bir kurulum ya da kaldırma bir kez
  daha baştan çalıştırılır; bu yine o kadar sürebilir. Bu sırada sayfa ne
  olduğunu ("sonuç sunucudan okunuyor", "sonuç kaydediliyor") ve kimsenin bir
  şey yapması gerekmediğini söyler.
- **Agent'ın tek bir geç yanıtı artık süren bir değişikliği durdurmaz**:
  değişiklik yalnız agent panelin yenilemelerini bütün süre boyunca
  yanıtsız bırakırsa durdurulur ve o zaman yukarıdaki gibi sonuçlandırılır.
- **Sonuç yine de kaydedilemezse** sayfa, başka bir değişikliğin her reddi ve
  **Şu anki sorunlar** adımları verir: agent'ı denetleyin
  (`sudo systemctl status celikpanel-agent`), sonra
  `sudo systemctl restart celikpanel-panel`. Bir kurulum yeniden
  çalıştırılırken sunucunun paket yöneticisi meşgul kalırsa değişiklik
  "başlatılamadı" yerine kendi sözleriyle ("sunucu meşgul kaldı") biter.

## Arka planda: site başına sınırlara hazırlık

Bu sürüm, site başına sınırları getiren sürümden geri dönülebilecek sürüm
olabilir: kendi sitesinin sınırlarına taşınmış programlar posta engeliyle
başlamayı sürdürür, planlar ve hesaplar burada düzenlendiğinde site
sınırlarını korur ve bu sürüm bir programı yeniden yazdığında program
sitesinin sınırlarında kalır. Site başına sınırları çalıştırmış sunucular
için bu sürümden daha geriye dönmek desteklenmez: bir sitenin sınırlarına
taşınmış programlar başlamaz. Bugünkü bir sunucuda müşterinin gördüğü hiçbir
şey değişmez: her program başlarken ve dururken, programı posta
kapılarından uzak tutan küçük yardımcı önce programın nerede çalıştığını
sistemin kendisinden okur, sonra eskisi gibi systemd'ye sorar.

## alpha.146'ya dönmek

- **Geri dönüş yalnız güncellemeden sonraki ilk sunucu değişikliğine kadar
  açıktır.** Ürünün geri dönüşü (`rollback.sh`) önceki sürümü yalnız agent'ın
  sunucu değişiklikleri kaydı güncellemenin sakladığıyla aynıyken geri koyar.
  Güncellemeden sonra herhangi bir değişiklik kaydedildiğinde reddeder ve şunu
  yazar:
  `!! current agent ledger differs from the verified snapshot; rollback would forget host mutations`.
  Araçları taşıyan adım böyle bir değişikliktir ve panelin açılışından
  yaklaşık üç dakika sonra çalışır; bu yüzden geri dönüş pratikte yalnız o ilk
  dakikalarda kabul edilir. Başarısız olan bir güncelleme yeni panel açılmadan
  önce kendiliğinden geri alınır; o sırada bu sürümün değişikliklerinin hiçbiri
  yoktur.
- **Sonrasında her bölüm için geri dönüş bu sürümün kendi geri almasıdır:**
  kapatılmış bir ortak havuz adı geri verildiğinde yeniden açılır (CelikPanel
  onu sonra açık bırakır):

  ```bash
  sudo mv /etc/php/<sürüm>/fpm/pool.d/www.conf.off-by-celikpanel /etc/php/<sürüm>/fpm/pool.d/www.conf
  sudo systemctl reload php<sürüm>-fpm
  ```

  `at`, `/etc/at.allow` silinerek yeniden açılır; araçlar
  `docs/TENANT-ISOLATION.tr.md` §7'deki sıralı adımlarla web sunucusu hesabına
  geri alınır (önce ortak havuz, dosyalar web sunucusunun grubuna, web sunucusu
  dosyaları ortak sokete, kendi havuzlar kaldırılır, hesaplar en son). Panelin
  kendisinde araçları geri almak için bir düğme yoktur.
- **Yalnız eksiksiz olsun diye**, bu sürümün çalıştığı bir sunucuda
  alpha.146'nın neyi korumayacağı: bir aracın kendi havuzunu tanımaz; bu yüzden
  aracın çalıştığı PHP sürümünün kaldırılmasını reddetmez ve araçların Yönet
  pencerelerinde PHP sürümü görünmez; web postasının kendi güncellemesi
  başarısız olur ve eski klasörü geri koyar; ortak havuzu kapalı bir PHP
  sürümünün son sitesini silmek ya da taşımak reddedilir (alpha.146 havuzu geri
  koymaz); bu sürümün **Şu anki sorunlar** satırları görünmez.

## Bilinen sınırlar

- **Arch Linux'ta** araçlar web sunucusu hesabında kalır ve PHP havuzu
  kapatılmaz: bu sürüm onları Debian ve Ubuntu'da taşır. `at` ve site
  crontab'ları her sistemde ele alınır.
- **Bir site hesabı kendi crontab'ına hâlâ satır ekleyebilir.** Satırlar
  görünür, silinebilir ve yönetici tarafından tutulabilir; engellenmez.
- **Ortak PHP havuzu kapatılırken bakılmayanlar:** soketine doğrudan bağlanan
  programlar (örneğin işlem kodu önbelleğini sıfırlayan zamanlanmış bir iş ya
  da havuzun durum sayfasını okuyan bir izleme aracı), systemd dışında
  başlatılan bir web sunucusu ve `nginx -T`'nin yazdırmadığı Lua kodu.
  Bunlardan biri havuzu kullanıyorsa onu yukarıdaki gibi geri koyun;
  CelikPanel sonra onu açık bırakır.
- **Veritabanı araçlarının yerel sunucusu** (`127.0.0.1:8306`, panelin
  `/dbtool/` yolunun arkasında) sunucudaki her hesaba ve oturum açmış her panel
  kullanıcısına yanıt verir. Paketlerin kendi dosyalarını (kod, belgeler,
  örnek SQL; gizli bilgi yok) verir ve PHP'lerini araçların kendi hesaplarıyla
  çalıştırır; veritabanlarını araçların kendi girişleri korur.
- **Bir taşıma geri alınırken** o anda zaten gitmiş bir dosya, önemli olsa da,
  yalnız agent'ın günlüğünde bir satırdır.
- **Sahip web postasının klasörünü başka hesaplara yeniden açarsa**
  CelikPanel bunu geri almaz ve söylemez; kaldırılmış bir erişim listesi
  girdisi **Şu anki sorunlar**'da söylenir.
- **Bir sitenin hesap kimliğini paylaşan bir girişin adına yazılmış bir
  crontab** ve `systemd --user` zamanlayıcıları ele alınmaz.
- **Panelin kaydedemediği bir değişiklik**, "kaydedilemedi" diye anıldıktan
  sonra ancak panelin bir sonraki açılışında kendiliğinden sonuçlanır; o
  zamana kadar güvenlik duvarı, DNS ve posta TLS değişiklikleri yarım kalmış
  bir bileşen kurulumunun yanında kabul edilir. Bir kurulum, adımlarının
  yinelenebilir olduğu varsayımıyla baştan yeniden çalıştırılır.
- alpha.145'in açık listesi açık kalır
  ([alpha.145 sürüm notları](RELEASE-NOTES-v0.1.0-alpha.145.tr.md)).

## Laboratuvarda denenenler

**Açık.** Test sunucusu çiftindeki (Debian 13 ve Ubuntu 24.04) laboratuvar
turu henüz yapılmadı. Adımları bu sıradadır: site başına sınırlar
hazırlığının dokuz adımı (ilki: güvenlik duvarı var olmayan bir yeri
reddetmeli; kabul ederse bu sürüm yayımlanmaz), sonra "sürüyor" düzeltmesinin
on adımı, sonra güvenlik bölümünün adımları (#348 numaralı istek): güncelleme ve
taşıma boyunca her saniye izlenen web postası ve veritabanı araçları, takılan ya da
yeniden başlatılan bir panelin kestiği bir taşıma, başarısız bir güncellemenin
kendiliğinden geri alınması, taşımadan önce ve sonra geri dönüş, ve iki sunucuda
taşımalar, havuz, `at` ve crontab satırları.

## Güncellemeden sonra ne beklenir

- Şema adımı yok: düzen şema 81 olarak kalır. Ürünün alpha.146'ya geri
  dönüşü yalnız güncellemeden sonraki ilk dakikalarda, araçları taşıyan adım
  çalışmadan önce kabul edilir (bkz. "alpha.146'ya dönmek").
- İlk açılıştan yaklaşık üç dakika sonra: her aracın kendi havuzu başlar ve
  taşınan her araç için web sunucusu bir kez yeniden yüklenir; ortak havuzu
  kapatılan her sürüm için PHP-FPM bir kez yeniden yüklenir; `at` kurulu olan
  ve sahibin dosyası bulunmayan yerde `/etc/at.allow` belirir.
- phpMyAdmin, phpPgAdmin ve web postası eskisi gibi oturum açar ve çalışır.
  Biri yanıt vermezse **Şu anki sorunlar** bunu adımıyla birlikte söyler.
- **Şu anki sorunlar** sahibi gerektiren satırlar gösterebilir: bir şey
  kullandığı için açık tutulan bir ortak havuz (neyin kullandığını söyler),
  site hesaplarının bekleyen `at` işleri, panelde yapılmamış crontab
  satırları. Her biri kimin ne yapacağını söyler.
- Agent ile bağlantısını kaybeden uzun bir değişiklik kendiliğinden biter;
  sayfa sonuçlanırken kimsenin bir şey yapması gerekmediğini söyler.

## Kanıt

Şema değişikliği yok: `internal/db/migrations/` altındaki hiçbir dosya
değişmez; düzen 81 olarak kalır. Yeni ve eklemeli: agent çağrıları
`CloseSideDoorsV1`, `ReadSideDoorsV1` ve `KeepCronLineV1`; panel yolu
`/api/v1/metrics/side-doors/reclose` (yalnız yönetici); taşımaların, havuzun,
`at`'in ve tutulan crontab satırlarının denetim kayıtları; 22'den 38'e Şu anki
sorunlar türleri; düzeltmenin sonuçlandırma aşamaları ve iki bitiş kodu.

Bölümlere göre testler:

- Kendi hesaplarındaki araçlar: `TestASiteAccountReachesNothingUnderWebmailsTree`
  (gerçek dosyalar ve sahipler), `TestWebmailStaysOnTheWebAccountWhereTheFileSystemRefusesAccessControlLists`,
  `TestTheUndoKeepsAPathChangedSinceAndNamesIt`,
  `TestTheUndoRemovesAFileTheMoveCreatedAndSaysNothingOfOneAlreadyGone`,
  `TestAnAccessControlListOfAnySizeIsRead`, `TestInterruptedMoveTable`,
  `TestWebmailLocalServerOnItsOwnPoolKeepsAlpha145Rules`,
  `TestWebmailConfigRewriteKeepsTheOwnAccountsGroup`,
  `TestEveryToolMoveTakesTheSideDoorsLockAfterItsStep`.
- Ortak PHP havuzu: `TestTheStockPoolInUseCheckFailsOnlyInTheSafeDirection`,
  `TestOnlyApacheFilesApacheLoadsKeepTheStockPoolOn`,
  `TestAnApacheIncludeThatCannotBeFollowedKeepsThePoolOn`,
  `TestOtherWebServersKeepTheStockPoolOn`,
  `TestUnixPathsAreComparedByWhatTheyResolveTo`,
  `TestThePanelsOwnUpstreamLineDoesNotKeepTheStockPoolOn`,
  `TestThePutBackKeepsTheTurningMarkUntilThePoolIsBack`,
  `TestTheLastSiteDeletedWhileAToolMovesOffLeavesThatVersionAPool`,
  `TestForgettingAVersionNeverRewritesAnUnreadableRecord`.
- `at` ve crontab'lar: `TestACutAtAllowWriteIsCelikPanelsUnfinishedStep`,
  `TestTheAgentRefusesToEditOrEnableALineNotMadeInCelikPanel`,
  `TestADeleteWritesTheRecordWithoutTheLineBeforeTheCrontab`.
- Yöneticinin gördükleri: `TestClosingAgainRunsApartFromTheRequestOnceAndOnlyForItsDoor`,
  `TestTheCloseAgainResultSurvivesAPanelRestart`,
  `TestAReadNeverReplacesANewerAnswer`, `TestOneSlowAnswerIsNotDown`,
  `TestASideDoorsStepThatLosesTheAgentEndsAndIsNotLeftRunning`; web
  `side-doors-problems.test.mjs`, `side-doors-undo.test.mjs`.
- "Sürüyor" düzeltmesi: `TestD1…` testleri (web postası güncellemesi ve bir
  bileşen kurulumu kendiliğinden sonuçlanır, bir kayıt en çok bir kez yeniden
  çalıştırılır, yanıtsız tek bir yenileme bir adımı durdurmaz, tek başına bir
  değişiklik aynı kurala uyar); web `d1-stuck-operation.test.mjs`.
- Site başına sınırlar hazırlığı: `TestEgressPlaceOrder`,
  `TestAddFailureIsAPlaceProblemOnlyWhenItIs`, `TestProductionPlaceReads`,
  `TestGuardReaddsUnitsInTheirSitesSlice`, `TestProofAcceptsTheSitesSlice`,
  `TestUnitKeepsOnlyItsOwnSitesSliceWithItsFile`,
  `TestPlanAndAccountSavesKeepCeilingsThisReleaseDoesNotKnow`.

Dokunulan paketlerin Go test takımları ve web test takımı, temel üzerinde ve
bu sürümün kodu üzerinde adıyla aynı başarısız kümeyle çalıştı (test
makinesinin kopyasının dışarıda bıraktığı kanıt dosyalarını okuyan üç test;
CI'da geçerler) ve sürüm isteğinin her denetimi CI'da geçti. Güvenlik bölümü
dört, diğer iki bölüm ikişer bağımsız incelemeden geçti.
