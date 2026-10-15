# v0.1.0-alpha.85 (taslak, yayımlanmadı)

[English](RELEASE-NOTES-v0.1.0-alpha.85.md)

Bu, v0.1.0-alpha.84'ü izleyen sürümün taslağıdır. Numara v0.1.0-alpha.85'tir;
sürüm hazırlığı 5443fe055 commit'idir (84'ten sonra sıra 85; önceki olarak
alpha.84 sürüm commit'i `f0157e6c8`). Sürümün hiçbir parçası yayımlanmadı;
hiçbir kurulu sunucu onu çalıştırmıyor. Bu taslak 15 Ekim 2026'da (UTC)
yazıldı.

Sürüm, sahibin 14 Ekim 2026'da istediği bir şeyi her yerde yapar: sizi
bekleten bir ekran artık beklediğini gösterir ya da sizin bir şey yapmanız
gerekmediğini açıkça söyler ([D-033](DECISIONS.tr.md)). İş, kurulu ikinci
sunucuda kurulumun son adımında başladı. Orada son kontrol, sahibin
sağlayıcıda çoktan değiştirdiği ve genel DNS'in hâlâ yaymakta olduğu bir ters
DNS adını bekliyordu. Adım "Waiting for you" diyordu; bütün kontroller
geçtikten sonra da bir sonraki 20 saniyelik geçişe kadar etkin düğmelerin
yanında hareketsiz bir cümle gösterdi. O adım artık sahibin geçen kontrolüyle
tamamlanır, hareketle "Kurulum tamamlanıyor" gösterir ve ters DNS beklemesini
kendi adıyla söyler. Ardından arayüzün geri kalanı, bir sayfanın beklediği her
durum için okundu: 37 durum; 18'i kurala zaten uyuyordu, 16'sı hemen
değiştirildi, açık kalan 3'ü için sahip aynı gün karar verdi ve onlar da
değiştirildi.

Bunun yanında: alpha.84 notlarının O4–O6 gözlemleri düzeltildi (Bileşenler
güvenlik duvarı şeridi ve Dashboard'ın "İlgi istiyor" kutusu), Dashboard'ın
Posta paketleri kartı sayfayla birlikte kontrol eder, posta profili cümleleri
türlü gönderilir ve sayfanın dilinde söylenir, HSTS kaldırma süresi artık
tarayıcının saatini sunucununkiyle karşılaştırmaz, kenar çubuğundaki yapı
kimliği tek parça kalır ve Agent, ilk ters DNS yanıtı eskiyse sonraki
çözümleyiciye sorar.

Yaptıkları; bileşen testlerine, API taklidine karşı bir tarayıcı incelemesine
(on ikinci tur: 8 yapılandırmada 31 durum) ve bir laboratuvar ölçümüne dayanır:
on sekizinci gerçek sistem kaydı (set16, 14 Ekim 2026 UTC); Debian 13'e temiz
kurulum ve sahibin alpha.84'ten güncellemesi, Debian 13 ve Ubuntu 24.04'te,
sağlam ve kusurlu adayla, gerçek, başsız bir Chrome'da. Hiç çalıştırılmayanları
sınırlar bölümü söyler. Bekleme durumlarından ikisine, kurulumun
tamamlanmasına (sahibin karşılaştığı durum) ve güncelleme kartının meşgul
durumuna, laboratuvarda ulaşılamadı.

## Ne göreceksiniz

Önce sade sözlerle, sunucu sahibi için. Güncellemeden sonra açık olan her
CelikPanel sayfasını yeniden yükleyin; güncellemeden önce yüklenmiş bir sayfada
eski ekranlar durur. Bir sayfa sunucuyu, bir sağlayıcıyı ya da arka plandaki bir
işi beklediğinde artık ya hareket eden bir şey görürsünüz (cümlenin yanında
dönen bir simge; araya girebilecek düğmeler kapalıdır) ya da yapılacak bir şey
olmadığını ve sayfanın kendiliğinden güncelleneceğini söyleyen bir cümle. Her
beklemenin bir sonu vardır: yaklaşık bir dakika içinde hiçbir şey doğrulanmazsa
sayfa sonucun henüz bilinmediğini, bunun bir hata olmadığını ve neyi
denetleyeceğinizi söyler. Kurulum, sunucu sağlayıcınızda çoktan değiştirdiğiniz
bir ters DNS adını bekliyorsa adım artık "Ters DNS bekleniyor" der; değişikliğin
genel DNS'e ulaşmasının birkaç dakikadan birkaç saate kadar sürebileceğini ve
CelikPanel'de başka bir şey yapmanız gerekmediğini söyler. "Gereksinimleri
tekrar kontrol et" düğmesine bastığınızda bütün kontroller geçerse adım
"Kurulum tamamlanıyor" der ve bir sonraki 20 saniyelik geçişi beklemeden
saniyeler içinde tamamlanır. CelikPanel Agent'ı durmuşken Türkçe Bileşenler
sayfası ve Dashboard'ın "İlgi istiyor" kutusu bunu artık sayfanın dilinde,
çalıştırılacak komutlarla söyler. Buradaki hiçbir şey sunucunuzun hizmetlerini
değiştirmez. Bu sürümün yayımlanması hiçbir kurulu sunucuyu güncellemez;
güncellemeyi panelin kendi güncelleme ekranından siz başlatırsınız.

## Sunucu sahibi için ne değişiyor

**Kurulumun son adımı, sizin geçen kontrolünüzle tamamlanır.** Bugüne kadar
"Gereksinimleri tekrar kontrol et" bütün kontrolleri geçirdikten sonra, bekleyen
kurulum çalışması bunu ancak bir sonraki 20 saniyelik geçişinde fark ediyordu.
Artık sahibin açık kontrolü (`GET /api/v1/setup?check=1&manual=1`) bekleyen
çalışmayı uyandırır; yalnız bütün kontroller geçtiyse ve son çalışma son
doğrulamasında bekliyorsa (`cmd/panel/server_setup_operations.go` içindeki
`wakeServerSetupVerification`, `cmd/panel/server_setup.go` içindeki kurulum
işleyicisinden çağrılır; panel başına tek yerlik tamponlu bir uyandırma
sinyali). İşleyici kurulumu tamamlamaz ve değişiklik başlatmaz; çalışan kendi
tek geçişinde yeniden kontrol eder ve tamamlar. Sayfa sorguları `manual=1`
göndermez; gözetimsiz 20 saniyelik aralık değişmedi. Testler:
`TestServerSetupManualCheckWakesWaitingVerification` ve
`TestServerSetupManualCheckWakesOnlyPassingVerification` (parametre adı
değiştirilmiş olumsuz kontrol 5,6 sn'de başarısız oldu).

**Tamamlanma etkin ve sınırlı gösterilir.** Geçen bir kontrolden sonra başlık
ve son adım "Kurulum tamamlanıyor" der; adımda ve şu cümlenin yanında dönen bir
simge vardır: "Bütün kontroller geçti. Panel kurulumu şimdi tamamlıyor; sizin
bir şey yapmanız gerekmiyor. İş bitince bu sayfa kendiliğinden güncellenir."
Sayfa işlemi 3 sn yerine 1,5 sn'de bir okur (`web/src/components/ServerSetup.tsx`,
60 000 ms'lik `setupFinishingLimit`). Bu durum, çalışma son kontrolden çıkınca
ya da çalışma yeni ve farklı bir karşılanmamış sonuç kaydedince biter; o sonuç
gösterilir. 60 sn içinde doğrulama gelmezse sayfa sonucun henüz bilinmediğini
(bunun bir hata olmadığını), sayfanın yeniden yüklenmesini ve durum sürerse
`sudo systemctl restart celikpanel-panel` komutunun aynı kurulumu, biten
adımları tekrarlamadan son kontrolünden sürdüreceğini söyler; bu bildirim
dururken adım "Henüz doğrulanmadı" der (ekran görüntüsü turuna kadar
"Gereksinimler bekleniyor" diyordu; aşağıya bakın). O adımda çalışmayı durduran
tek denetim olan "Planı düzenle", tamamlanırken sunulmaz. Pasif "These checks
passed. Waiting for the server to confirm setup completion." metni kaldırıldı.
Testler: `web/tests/setup-finishing-mounted.test.mjs`, İngilizce ve Türkçe:
dönen simgeler, "Gereksinimleri tekrar kontrol et" ve "Planı düzenle" yok,
1,5 sn'lik okumalar, tıklamasız ve yalnız GET istekleriyle bitiş, 60 sn'lik
geri dönüş, yeni bir karşılanmamış sonucun tamamlanmayı bitirmesi ve
"Henüz doğrulanmadı" etiketi.

**Ters DNS beklemesi kendi adıyla söylenir.** Yalnız ters DNS uyuşmazlığını
bekleyen son kontrol "Sizi bekliyor" yerine "Ters DNS bekleniyor" der; yanında
açık bir plan düzeltmesi varsa yine "Sizi bekliyor" der. Yönlendirmeye, sunucu
sağlayıcısında yapılan bir değişikliğin genel DNS'e ulaşmasının birkaç dakikadan
birkaç saate kadar sürebileceği (çünkü çözümleyiciler eski adı, kayıtlı
kopyalarının süresi dolana kadar kullanır) ve değişiklik yapıldıktan sonra
CelikPanel'de başka bir şey gerekmediği eklendi
(`web/src/lib/serverSetupGuidance.ts`; testler
`web/tests/setup-final-check-mounted.test.mjs` içinde).

**Agent, ilk ters DNS yanıtı eskiyse sonraki çözümleyiciye sorar.** Agent'ın
posta kimliği sorgusunda (`cmd/agent/mail_health_dns.go` içindeki
`mailDNSIdentityAt`), beklenen ad olmayan bir PTR yanıtı ya da PTR'nin
olmaması artık sonraki çözümleyiciye de sorulur. Beklenen adı eşleşen bir ileri
A/AAAA kaydıyla veren herhangi bir çözümleyici kimliği kanıtlar. Aksi halde ilk
gözlem eskisi gibi bildirilir; ana makineyi adlandıran ama ileri kaydı henüz
eşleşmeyen bir PTR öne alınır. Eşleşen bir ilk yanıttan sonra başka sorgu
yapılmaz. Test: `TestMailDNSIdentityAsksNextResolverWhenFirstAnswerIsStale`
(altı durum). Bu değişiklik yalnız Agent güncellendikten sonra etkili olur
(sınırlara bakın).

**Her bekleme durumu okundu ve 19'u değiştirildi.** Web arayüzü, bir sayfanın
sunucuyu, bir arka plan işlemini, bir yoklamayı, bir sağlayıcıyı, lisansı ya
da dış bir denetimi beklediği her durum için kaynaktan okundu: 37 durum.
Kapsam: kurulum (son kontrol dışındaki aşamalar), Bileşenler ve bileşen
sayfaları, Dashboard, lisans ve erişim katmanları, panel güncelleme akışı, alan
adı sertifikaları ve HSTS, DNS, içe aktarma, PHP, yedekler ve kenar çubuğundaki
yapı damgası. 18'i kurala zaten uyuyordu; 16'sı uymuyordu ve değiştirildi; 3'ü
uymuyordu, sahip onlar için 14 Ekim 2026'da karar verdi ve ardından onlar da
değiştirildi. Her satır koddan, dosya ve satır numarasıyla okundu; hiçbiri
kurulu bir sunucuda ölçülmedi. Denetim
[OPERATION-GUIDANCE](OPERATION-GUIDANCE.tr.md) içinde, "Hareketsiz görünen
bekleme ekranları" kaydındadır (2026-10-14). Ekran ekran değişiklikler:

- *Kurulum: kesilen okumadan sonra yeniden bağlanma ve planlı sertifika yeniden
  başlatması* (`ServerSetup.tsx`, `2e273cc55`). Cümlenin yanında dönen bir
  simge; cümle artık sayfanın birkaç saniyede bir sunucuya kendiliğinden
  yeniden sorduğunu ve yapılacak bir şey olmadığını söyler (önceden sahibe
  "Bu işleme yeniden bağlan" düğmesine basmasını söylüyordu). 60 sn sonra simge
  durur; sayfa sonucun bilinmediğini (hata olmadığını), sormaya devam ettiğini
  ve paneli sunucuda nasıl denetleyeceğini söyler
  (`sudo systemctl status celikpanel-panel`, sonra
  `sudo systemctl start celikpanel-panel`); "Bu sayfayı yeniden yükle" sunulur.
- *Kurulum: "Gereksinimleri tekrar kontrol et" ve "Bu bağlantıyla kuruluma devam
  et".* İstek sürerken dönen simge.
- *Bağlantı kesildikten sonra bileşen işlem kilidi* (16. satır,
  `OperationOverlay.tsx`, `8e68b3ce2`). "Yeniden bağlanılıyor…" yanında dönen
  bir simge ve sayfanın birkaç saniyede bir kendiliğinden yeniden sorduğunu
  söyleyen cümle. Kullanılabilir bir yanıt gelmeden 60 sn geçerse simge durur;
  kilit sonucun henüz bilinmediğini, bunun başarısız olduğu anlamına
  gelmediğini, sunucunun işlemin kaydını tuttuğunu ve paneli sunucuda nasıl
  denetleyeceğinizi söyler; "Sayfayı yeniden yükle" kalır. Okumalar 3 sn'de bir
  sürer; kaydedilmiş bir hata kendi sözlerini korur.
- *Sunucu meşgulken panel güncelleme kartı* (25. satır, `PanelUpdateCard.tsx`,
  `3abbe3f76`). Hazır olma durumunu 5 sn'de bir yeniden okur: yalnız GET, aynı
  anda tek okuma, yalnız sekme görünürken ve kendi denetleniyor görünümünü
  çizmeden; sekme gizlenince ya da kart meşgul durumundan çıkınca durur. Bu
  okumaların hiçbiri bir güncelleme başlatmaz ya da kabul etmez. "Yeniden dene"
  kalır.
- *Bir alan adının yayılan DNS değişikliği* (35. satır, `DomainConnection.tsx`,
  `0ad26074d`). Sekme görünürken bağlantı kartı, "Yeniden kontrol et"in
  gönderdiği GET ile 30 sn'de bir, en çok 60 kez (görünür bir sayfada 30
  dakika) yeniden kontrol eder. Sağlayıcıdaki bir değişikliğin yayılmasının
  birkaç dakikadan birkaç saate kadar sürebileceğini, sayfanın kendiliğinden
  kontrol ettiğini ve yapılacak bir şey olmadığını, son kontrolün ne zaman
  yapıldığını söyler (yanıtın `checked_at`'i, yoksa sayfanın yanıtı aldığı an;
  tarayıcının saatiyle asla karşılaştırılmaz). Sonra durur; değişikliğin son
  kontrolde hâlâ yayılmakta olduğunu, bunun bir hata olmadığını ve "Yeniden
  kontrol et"in kullanılabileceğini ya da daha sonra dönülebileceğini söyler.
- *Sunucu değişikliği beklemeleri* (Bileşenler şeridi, Dashboard güvenlik
  duvarı penceresi; `5403eed88`). Şu cümle eklendi: "Bu sayfa birkaç saniyede
  bir yeniden denetler ve bu beklemeyi kendiliğinden kaldırır; sayfayı yeniden
  yüklemeniz gerekmez." Doğrulanamayan durum artık yeniden yükleme istemez. Bir
  bileşen sayfasının kurulum penceresi hazır olma durumunu artık bir kez değil,
  açıkken 5 sn'de bir okur (yalnız GET).
- *Uzun bir isteği devre dışı ve hareketsiz bir etiketin arkasında yürüten
  düğmeler* (`f643f838d`, `875e9c2bd`): alan adı sertifikası alma,
  etkinleştirmeyi yeniden deneme ve yükleme, panel sertifikası, bölge yayımlama,
  PHP sürümü, içe aktarma incelemesi ve çalıştırma, depo etkinleştirme, sürüm
  kaldırma. Her biri istek süresince kendi dönen simgesini gösterir.

**Bileşenler güvenlik duvarı şeridi (alpha.84 notlarının O4 ve O5'i).** Kodlu
bir yanıt sayfanın dilinde söylenir: Türkçe sayfada şerit, sunucunun İngilizce
cümlesini değil `err.AGENT_UNREACHABLE` çevirisini gösterir. Şerit, sayfanın
aldığı her taramada güvenlik duvarını yeniden okur; böylece Agent döndükten ve
Yeniden tara başarılı olduktan sonra "Durum bilinmiyor"u korumaz
(`web/src/components/ServiceList.tsx` içindeki `FirewallBar`, `723d4894f`).
Ekran görüntüsü turundan sonra reddedilen okuma, hata yüzeyinde değil, mürekkep
renkli metinle dikkat yüzeyinde durur; dar bir kartta kullanılamayan denetim
metnin altına geçer (`795f31674`). Testler:
`web/tests/remote-state-mounted-batch3.test.mjs`.

**Dashboard'ın "İlgi istiyor" kutusu Agent'ı adıyla söyler (O6).** Okumalarından
biri `AGENT_UNREACHABLE` ile reddedildiyse bildirim, bunun bir şeyin yanlış
olduğu anlamına gelmediğini söylemek yerine Agent'ı ve sahibin komutlarını
(`err.AGENT_UNREACHABLE`) adlandırır (`web/src/components/Dashboard.tsx`,
`6c2b9e3f3`).

**Dashboard'ın Posta paketleri kartı sayfanın kendi kontrolünü izler.** Sistem
servisleri kartının çalıştırdığı aynı kontrolden; kontrol ediliyor (dönen
simgeyle), bir değişiklik bekletiyor, kontrol edilemedi ya da yakın zamanda
kontrol edilmedi durumlarını gösterir. "Yeniden taramak için Bileşenleri aç" ve
"…posta durumuna güvenmeden önce yeniden tarayın" kaldırıldı. Bu, alpha.84
notlarının açık bir maddesiydi (`Dashboard.tsx`, `11762f96f`; testler
`web/tests/dashboard-mail-stack-card-contract.test.mjs` içinde).

**Posta profili cümleleri türlüdür.** Adlar ve açıklamalar profil kimliğine göre
katalogdan gelir. Panel engel ve uyarı cümlelerini artık türlü gönderir
(`blocked_code` ile `blocked_vars` ve `warning_code`; eski metnin yanında, o
metin eski sayfalar için kalır) ve sayfa kodu kendi dilinde söyler
(`cmd/panel/mail_profiles.go` içindeki `mailProfilesView`, `1c5454c53`;
`web/src/lib/mailProfileText.ts`). Üç sunucu cümlesi artık "run a fresh scan"
ya da "run another scan" demez; durumu bir sonraki bileşen kontrolünün
doğrulayacağını söyler. Son denemenin hatası, "Son deneme şunu bildirdi:"
girişinin ardından sunucunun kendi sözleriyle kalır. Bu, alpha.84 notlarının
saydığı Türkçe Bileşenler sayfasındaki çevrilmemiş posta profili metnini de
karşılar. Testler: `TestMailProfileTextCodesAreListed` (kod türünün her sabiti
listededir), `TestMailProfileViewTypesItsSentences` ve
`mailProfileTextCodes` listesini panelin kaynağından okuyup her kod için tarama
sözü geçmeyen birer İngilizce ve Türkçe cümle isteyen
`web/tests/mail-profile-text.test.mjs`.

**HSTS kaldırma süresi artık iki saati karşılaştırmaz.** Alan adı SSL yanıtına
`settings.hsts_retire_remaining_seconds` eklendi: yanıt yazılırken sunucunun
saatiyle kalan süre (`cmd/panel/domain_ssl_handlers.go` içindeki
`stampHSTSRetireRemaining`, `f643f838d`). Sayfa (`DomainSSLSettings.tsx`) sonu,
yanıtı aldığı andan tarihler ve kendi saatini, korunan `hsts_retire_after` ile
artık karşılaştırmaz. Alanı göndermeyen eski bir Panel'de eski kural sürer. Bu,
`DomainSSLSettings` içindeki HSTS süresinin bir sunucu zaman damgasını
tarayıcının saatiyle karşılaştırdığını söyleyen alpha.84 sınırını kapatır.
Testler: `TestStampHSTSRetireRemaining`,
`TestDomainSSLAnswerCarriesHSTSRetireRemaining` ve sunucunun saati geride,
ileride ve eski Panel'le `web/tests/remote-state-mounted.test.mjs` içindeki
bağlanan testler.

**Kenar çubuğundaki yapı kimliği tek parça kalır.** "build" ve 12 karakterlik
kimlik tek birimdir; satır artık kimliğin içinde kırılmaz; tam kimlik başlıkta
ve ekran okuyucu metnindedir (`web/src/components/Layout.tsx`, `942623e1d`;
eski sınıfla başarısız olan `web/tests/layout-server-identity-contract.test.mjs`).
Sahip bu kırılmayı alpha.83 ve alpha.84 güncellemelerinden sonra gözlemlemişti.

**Ekran görüntüsü turundan metin düzeltmeleri** (`89cf88397`). Türkçe HSTS
terimi "emeklilik", üç SSL anahtarında "kaldırma süresi" oldu; Dashboard'ın
antivirüs satırı ve iki servis ve güvenlik duvarı ipucu sahibe "siz" diye
seslenir; telefon menü düğmesinin erişilebilir etiketi katalogdaki `nav.menu`'yü
kullanır; HSTS'nin kalan süresi yuvarlanmış göreli bir zaman yerine bir süredir
("1 sa 30 dk", "2 gün 3 sa" gibi) (`web/src/lib/hstsRemaining.ts`,
`web/tests/hsts-remaining.test.mjs`).

**Ne değişmiyor.** Veritabanı şeması değişmedi (defter 44 kalır; alpha.84
etiketi ile bu aday arasında taşıma yok) ve yeni rota yok. alpha.84'ün Sistem
servisleri kontrolü, lisans denetimi ve Agent yanıtı değişmedi. Posta profilleri
dışında hâlâ "rescan" diyen sunucu cümleleri değişmedi (sınırlara bakın).

## API değişiklikleri

Yeni rota yok. Bir yeni sorgu parametresi, bir yeni yanıt alanı ve üç yeni
profil alanı; hepsi eklemeli.

### `GET /api/v1/setup?check=1&manual=1`

`check=1` yanındaki `manual=1`, sahibin açık "Gereksinimleri tekrar kontrol
et" isteğini işaretler. Yanıt, önceki aynı kurulum anlık görüntüsüdür. Bütün
kontroller geçtiyse ve son çalışma son doğrulamasında bekliyorsa Panel bekleyen
çalışmayı da uyandırır; çalışma sonra kendi geçişinde yeniden kontrol eder ve
tamamlar. İsteğin kendisi hiçbir şeyi tamamlamaz ve hiçbir şey başlatmaz.
`manual=1` olmadan hiçbir şey uyandırılmaz. Bunu göndermeyen bir istemci hiçbir
değişiklik görmez.

### `hsts_retire_remaining_seconds`

Alan adı SSL yanıtının `settings` nesnesinde, değişmeyen `hsts_retire_after`'ın
yanında:

```json
{
  "settings": {
    "hsts_retire_after": "2026-10-15T01:30:00Z",
    "hsts_retire_remaining_seconds": 5399.512
  }
}
```

(Değerler örnektir; diğer alanlar değişmedi.)

Yanıt yazılırken sunucunun saatiyle kalan süre; milisaniye hassasiyetinde, asla
sıfırın altında değil (süre geçtiyse 0). Kaldırma süresi yoksa alan yoktur. Bir
sayfa sonu, yanıtı aldığı an artı bu değer olarak alır; `hsts_retire_after`'ı
kendi saatiyle karşılaştırmaz. Alanı bulamayan bir sayfa eskisi gibi
`hsts_retire_after`'ı kullanır.

### `blocked_code`, `blocked_vars`, `warning_code`

Yönetilen servis yanıtlarının `profiles` listesindeki her posta profilinde,
değişmeyen `blocked_reason` ve `warning`'in yanında. Kodlar:
`state_unverified`, `state_incomplete`, `catalogue_incomplete`,
`hostname_unknown`, `dns_identity_not_ready`, `component_not_installable`,
`component_conflict` ve `component_requires` (engel) ile `repair_to_verify`,
`webmail_unverified`, `roundcube_not_ready` ve `partly_present` (uyarı);
`blocked_vars` bir cümlenin andığı değerleri taşır. Bir sayfa kodu kendi
dilinde söyler; İngilizce metni yalnız tanımadığı bir kod için korur. Alanları
tanımayan bir istemci onları yok sayar.

## Fark edeceğiniz davranış değişiklikleri

- Kurulumun son adımı, geçen bir "Gereksinimleri tekrar kontrol et"ten sonra
  saniyeler içinde tamamlanır; bu arada dönen simgeyle "Kurulum tamamlanıyor"
  gösterir ve tamamlanırken "Planı düzenle" sunmaz.
- Yalnız ters DNS'i bekleyen son kontrol "Sizi bekliyor" değil "Ters DNS
  bekleniyor" der; değişikliğin yayılmasının saatler sürebileceğini ve
  CelikPanel'de başka bir şey gerekmediğini söyler.
- Denetimin bulduğu her bekleme ya hareket eder ya da yapılacak bir şey
  olmadığını söyler ve sınırından sonra dürüst bir "henüz bilinmiyor" ile biter:
  kurulum ve bileşen kilidi 60 sn sonra, yayılan bir alan adı 60 kontrolden
  sonra.
- Güncelleme kartı, bir bileşenin kurulum penceresi, beklemeler ve yayılan bir
  alan adı görünürken sunucuyu kendiliğinden yeniden okur (yalnız okuma).
- Agent durmuşken Türkçe Bileşenler sayfasında güvenlik duvarı şeridi Türkçe
  konuşur ve Yeniden tara'dan sonra yeniden okunur; Dashboard'ın "İlgi istiyor"
  kutusu Agent'ı adıyla söyler.
- Posta paketleri kartı artık sizden yeniden taramak için Bileşenleri açmanızı
  istemez.
- HSTS'nin kalan süresi bir süredir ve tarayıcının saati ne derse desin
  doğrudur.
- Kenar çubuğundaki yapı kimliği artık ortasından kırılmaz.

## Ne ölçüldü, nasıl ölçüldü

**Bileşen testleri.** Bu sürümün ürün kodu `89cf88397`'ninkidir. Ondan sonraki
commit'ler kanıtı, belgeleri, bir laboratuvar aracını (`cold-load-set16.mjs`)
ve sürüm hazırlığını değiştirir; hiçbiri `cmd/panel/`, `cmd/agent/`,
`internal/` ya da `web/src/` altına dokunmaz.

- Go, WSL konuğu `CelikPanel-S2-Debian` içinde: `go test ./cmd/panel/...` 1542
  test ve 1665 alt test geçti, 6 atlandı, 0 başarısız; `1c5454c53`'te
  kaydedildi (ondan sonra hiçbir Go dosyası değişmedi). `go test
  ./cmd/agent/...` 1798 test ve 2822 alt test geçti, 29 atlandı, 0 başarısız;
  `a94f83cb0` ile `e358ebe3c` arası commit'lerle kaydedildi (o çalıştırmadan sonra
  `cmd/agent/` ya da `internal/` altında hiçbir dosya değişmedi). Agent sayısı,
  testlere temiz bir sunucu gösteren özel bir bağlama ad alanında ölçüldü
  (`/var/lib/celikpanel-agent-private` ve `/run/celikpanel` üzerine boş
  dizinler, `celikpanel` satırı olmayan bir `/etc/group`): o konuk kendisi de
  çalışan bir CelikPanel sunucusudur ve paket orada bu dalda da `e3c8f735a`
  üzerinde de aynı 19 testte başarısız olur. Konuk değiştirilmedi. İki sayı da
  [OPERATION-GUIDANCE](OPERATION-GUIDANCE.tr.md)'ın 2026-10-14 kayıtlarındaki
  sayılardır. `./internal/...` bu dalda çalıştırılmadı; `internal/` altında
  hiçbir dosya değişmedi.
- Web: `npm test` 1391/1391; 15 Ekim 2026'da sürüm hazırlığı commit'inde yeniden
  çalıştırıldı. `npm run build` tür denetimi ve paket bütçesiyle geçer
  (Dashboard parçası 160,25 KiB'ın 38,28 KiB'ı). Web sayısı kurulum
  değişikliğinden sonra 1348, denetimin on altı değişikliğinden sonra 1375, 16,
  25 ve 35. satırlardan sonra 1386 ve ekran görüntüsü turunun düzeltmelerinden
  sonra 1391'di. Değişen bir durumun her bağlanan testi, değişiklikten önceki
  ekrana karşı başarısız oldu; istisnaları rehber kaydı adlandırır (depo ve
  sürüm kaldırma düğmelerinin yalnız kaynak denetimi var; iki metin denetimi
  katalogları okur).
- Sürüm hazırlığının üç sözleşme testi geçer: indirme portalı ve imzalı manifest
  sözleşmeleri derleme konuğunda; sıra politikası sözleşmesi Windows çalışma
  kopyasında, çünkü `v0.1.0-alpha.84` etiketini konukta bulunmayan git ile okur
  ("84 -> 85; historical bootstrap sequence 84").

**Ekran görüntüsü turu (on ikinci tarayıcı incelemesi turu).** Windows'ta
gerçek bir Chrome, yerel API taklidine karşı (`0b43ee81e`,
`web/tools/browser-inspect/scenarios-batch12.mjs` ve `mock-batch12.mjs`),
2026-10-14 tarihli iki rehber kaydının değiştirdiği her durumu sürdü: 8
yapılandırmada 31 durum (1280 ve 390 genişlik, açık ve koyu tema, İngilizce ve
Türkçe); o commit'te kaydedildiği gibi senaryo hatası yok, yana kayan sayfa yok.
60 sn'lik sınırlara ve 30 sn'lik aralığa ürün değişikliğiyle değil, test
tarafındaki bir saat denetimiyle ulaşıldı. Gerçek sistem kaydından önce
düzeltilen kusurlar buldu:

- 60 sn'lik tamamlanma sınırından sonra adım "Gereksinimler bekleniyor" diyordu;
  sonuç bilinmezken bu, karşılanmamış bir gereksinim gibi okunur; artık
  "Henüz doğrulanmadı" der (`8dfc3ff69`);
- 390 px'lik bir telefonda bileşen kilidinin dönen simgesi cümlesinden ayrı
  duruyordu; artık satır içindedir (`733b798db`);
- reddedilen güvenlik duvarı okuması hata renginde, hata yüzeyinde duruyordu ve
  devre dışı denetim telefonda cümlesini sıkıştırıyordu (`795f31674`);
- Agent'ın komutları kısa çizgiden kırılıyordu ("celikpanel-" / "agent") ve
  yayılan bir alan adının çözümleyici adları ana makine adının içinden
  kırılıyordu; `KeepWords` (`web/src/components/ui.tsx`) bu tür sözcükleri
  bütün tutar (`8dfc3ff69`, `733b798db`, `795f31674`);
- yukarıdaki dört metin kusuru (`89cf88397`).

O turun görüntüleri depoda saklanmaz.

**On sekizinci gerçek sistem kaydı**
([set16](../deploy/e2e/release-recovery/evidence/set16-20261014/README.md),
14 Ekim 2026 22:16:46Z ile 15 Ekim 2026 00:01:17Z UTC arası). Aday commit
`89cf88397`'nin yayımlama öncesi ölçümü; çalışma ağacından değil, işlenmiş
ucun bir klonundan derlendi. Test lisanslı derlemeler, hiçbiri sürüm değil;
dışarıya NAT'lı QEMU/KVM konukları; ana makinede gerçek, başsız bir Chrome. Her
`result.json` `native_evidence: false` taşır; kayıt hiçbir P0 satırını
kapatmaz. Düzenek hiçbir kurulu sunucuya, lisans hizmetine, sertifika
otoritesine ve celikpanel.net'e yönelmedi.

- **Ne kuruldu.** Aday arşivi bir sabit commit'ten, `d5d673eb`'den derlendi; bu
  `89cf88397` artı yalnız `deploy/release-sequence-policy`'dir (84'ten sonra
  sıra 85), SHA-256
  `3913d51374ba8db47fd4acce0af21d0c4150690664809d8932e355dbfdff40ad`. Taban,
  alpha.84 etiket commit'i `f0157e6c`'nin yerel, test lisanslı derlemesidir,
  `9f86eb4352ce685e064c51c68aab9b592f99612ff3dcca7f71983c9305708dd4`; yayımlanan
  arşivle (`6dc899e5dabaf471978cde47bfde4a0a649b7c3f6941b1431284c84c9743c301`)
  bayt bayt aynı değildir. Kusurlu aday `524f86dd`'dir (`d5d673eb` artı önceki
  bir kaydın `cmd/panel/main.go` içindeki yalnız-taşıma kusuru),
  `77f39013cb3ef7fff68e3495794dd0034bcb4a396f7e8c2bd649db710119be3a`. Sabit
  commit yüzünden her aday sayfası `build d5d673ebd95e` okur ve her aday
  arşivinin içindeki önyükleme sabitleri hâlâ 84 ve `v0.1.0-alpha.84` okur
  (sabit commit yalnız politikayı değiştirir); her konuktaki kurulu kopya da
  aynısını okudu. alpha.84 etiketi ile aday arasında taşıma yok; defter 44
  kalır.
- **Temiz kurulum, Debian 13 (İngilizce masaüstü, 1440×900), 22:24:54Z ile
  22:44:41Z arası.** Kurulum, sahibin oturum açması ve lisans geçti; kurulum
  amacı `web` (DNS dışarıda) genel DNS adımında, `access_dns`'te bekledi ve sahip
  elle kurmayı seçti. Sürüm rotası `v0.1.0-alpha.85`, commit `d5d673eb…`,
  `agent_matches true`, `schema_version 44` yanıtladı; defter `as-expected`;
  alan adları `[]`; `valid_for_seconds` ve `scan_age_seconds` var.
- **alpha.84'ten güncelleme, sağlam aday.** Debian 13 (22:45:01Z ile 23:00:07Z
  arası; kurma 22:55:05Z, son 22:55:58Z) ve Ubuntu 24.04 (23:16:39Z ile
  23:38:07Z arası; kurma 23:33:07Z, son 23:33:58Z); her biri `web_mail` olarak
  kurulmuş, tek statik siteli bir tabandan, Panel'in kendi güncelleme yoluyla.
  İkisi de `succeeded / update_verified` ile bitti; Agent ve Panel
  `v0.1.0-alpha.85 / d5d673eb…`; taban sırası 85; defter 44; öncesi ve sonrası
  HSTS okumaları geçti; Alan adları tek satır, `managed_unchanged`; veritabanı
  karşılaştırması `equal-except-volatile`, `managed_site_files` yalnız
  `observed_at`'te uçucu.
- **Kusurlu aday: kendiliğinden geri dönüş.** Güncelleme sırasında konuk bir kez
  sıfırlandı (Debian 23:11:06Z, SSH 8,26 sn sonra döndü; Ubuntu 23:56:04Z,
  8,77 sn). İkisi de kendiliğinden kurtuldu: `phase: recovered`,
  `reason: rollback_verified`, `previous_failure: update_failed` (Debian
  23:12:07Z, Ubuntu 23:57:09Z), her birinde iki otomatik deneme, sahip denemesi
  yok. Agent ve Panel ardından `v0.1.0-alpha.84 / f0157e6c…` bildirdi; oturum
  açma, site işareti, SMTP ve güvenlik duvarı eşitti. Son adım bu kez geçti
  (set15'inki `managed_site_files` yüzünden başarısız olmuştu); böylece geri
  dönüşten sonraki okumalar ve tarayıcı penceresi de çalıştı.
- **Geri dönüşten sonra defter.** İki dağıtımda da: sonuç
  `equal-except-volatile`, beklenmeyen bir şey yok, önce ve sonra tek satır,
  yalnız `observed_at` değişti (Debian 1792019311 → 1792019522; Ubuntu
  1792022021 → 1792022224), kararlı özet eşit (Debian `a39f9b86…`, Ubuntu
  `985eed03…`) ve tam özet farklı. 67 tablo karşılaştırıldı, beşi uçucu olarak
  dışarıda bırakıldı. Bu, set15'in açık bulgusunu bu sabit commit için
  yanıtlar: geri dönüşten sonra yalnız `observed_at` değişti. Bu okumayı yapan
  yoklayıcı, düzenek değişikliği `a87ce0b97`'dir (aşağıda).
- **Dashboard tıklamasız, kendiliğinden kontrol eder.** Her laboratuvarda 90 sn
  içinde yükleme başına bir koşullu tarama: Debian temiz (saklı tarama 317,3 sn
  eski, yanıt 864 ms'de), güncellemeden sonra Debian (413,5 sn, 1025 ms),
  güncellemeden sonra Ubuntu (557,7 sn, 2622 ms), geri dönüşten sonra Debian
  (462,7 sn, 1908 ms), geri dönüşten sonra Ubuntu (641,4 sn, 1492 ms); Sistem
  servisleri kartı kontrol ediliyor durumuna, sonra sonuca geçti. Adayda Posta
  paketleri kartı bir dönen simgeyle "Kontrol ediliyor…" durumuna, sonra
  "Hazır"a geçti; "Yeniden taramak için Bileşenleri aç" hiç görülmedi. Geri
  dönüşlerden sonra tarama yanıtlanana kadar alpha.84'ün kendi eski metni
  göründü; bu bir geri dönüşten beklenendir. Temiz kurulumda Posta paketleri
  kartı ancak profiller posta etkinliği gösterince çizilir; bu yüzden orada
  kontrol ediliyor durumu görülemedi. Dashboard'dan hemen sonra Bileşenler
  sayfası hiç tarama göndermedi; tarama bayatken (317,1 sn) uyarısız ve
  bildirimsiz bir koşullu tarama gönderdi, 200, 873 ms.
- **Konuğun saati kaydırılmışken lisans denetimi** (temiz hücre). Konuk
  tarayıcıdan +91,065 ile +91,070 sn ve −88,937 ile −88,932 sn farklıyken sayfa
  her seferinde açıldı ve erişim denetimini üç kez okudu; her yanıt
  `can_use_panel: true`. Hesapla, eski kural (`valid_until`'ı tarayıcının
  saatiyle karşılaştıran) 3'ün 0'ını ve 3'ün 3'ünü reddederdi. Sonunda saat
  yaklaşık +1,05 sn'ye geri getirildi ve NTP yeniden açıldı.
- **Agent durmuşken Türkçe Bileşenler sayfası.** `GET /api/v1/firewall`
  `503 AGENT_UNREACHABLE` yanıtladı. Şerit, Türkçe Agent cümlesiyle "Güvenlik
  duvarı: Durum bilinmiyor" dedi; İngilizce cümle yoktu (O4 düzeltildi). Yüzeyi
  dikkat yüzeyiydi (kehribar kenar ve arka plan, mürekkep renkli metin); hata
  renginde öğe yoktu; `celikpanel-agent` üç kez geçti, her biri tek satırda,
  1440×900'de ve 390×844'te. Sahip Agent'ı başlattıktan sonra "Yeniden tara"ya
  bir basış bir tarama (200, 850 ms) ve bir güvenlik duvarı okuması (200, 16 ms)
  gönderdi; şerit uyarısız "Güvenlik duvarı: Açık" dedi (O5 düzeltildi).
- **Agent durmuşken Dashboard.** Türkçede "İlgi istiyor" ve İngilizcede "Needs
  attention" Agent'ı cümlesi ve üç komutuyla, dikkat yüzeyinde adlandırdı;
  `celikpanel-agent` kırılmadı (O6 düzeltildi). İngilizce yüklemede saklı
  tarama sayfa açıkken 300 sn'yi geçti; sayfa bir koşullu tarama gönderdi,
  3 ms'de `503`; Sistem servisleri kontrol ediliyordan Agent cümlesiyle
  "Could not check"e, Posta paketleri "Checking…"ten "Unknown: The components
  could not be checked just now, so the mail status is not known. The System
  services card above says why." durumuna geçti. Posta kartının "bir değişiklik
  bekletiyor" durumu üretilmedi.
- **Kenar çubuğundaki yapı kimliği.** Adayda `build d5d673ebd95e` tek satırda
  durdu, kimlik tek dikdörtgen, taşma yok; 1280×800'de ve menü açıkken
  390×844'te. Geri dönüşlerden sonra alpha.84'ün kendi damgası 1280×800'de
  kırıldı, alpha.84'te olduğu gibi; 390×844'te geri dönüşlerden sonra damga boş
  kutularla bulundu, bu yüzden orada bir şey iddia edilmez.
- **Ulaşılamayan ya da ölçülmeyen.** "Kurulum tamamlanıyor": temiz kurulum genel
  DNS adımında bekler ve son adım hiçbir laboratuvar konuğunda olmayan bir ters
  DNS adı ister; kayıt bunu taklit etmez. Güncelleme kartının meşgul durumu:
  tarayıcı aracının Ayarlar aşaması yok ve kart açıkken sunucuyu meşgul tutan
  bir adım yok. Arch; adayın temiz bir Ubuntu kurulumu (planlanmadı).
- **Önceden var olan, yargılanmayan gözlemler** (arkalarındaki dosyalar
  alpha.84'ten beri değişmedi). O1: Türkçe sayfalar `<html lang="en">` taşır
  (`web/index.html:2`; uygulama dili yalnız çevrimdışı sayfada ayarlar). O2:
  "Firewall engine" ve açıklaması Türkçe Bileşenler sayfasında İngilizce kalır;
  sunucu katalogundan gelir (`internal/core/managed_services.go:915-916`). O3:
  Dashboard'ın "Son etkinlik" listesi `server.setup.guidance` gibi ham denetim
  işlemi kimlikleri gösterir, çünkü `auditActionText`
  (`web/src/components/Dashboard.tsx:120`) yalnız oturum açma ve posta profili
  olaylarını eşler.
- **Kaydın açık maddeleri.** Her geri dönüşten sonra kurtarma temel dosyası
  hâlâ kusurlu adayı adlandırır (`524f86dd…`, sıra 85); çalışan derleme
  alpha.84'tür ve taban 85 okur: set12 ve set15'teki örüntünün aynısı,
  yargılanmadı. Temiz hücrenin `result.json` dosyası, "yayımlanmış
  v0.1.0-alpha.85 tabanında" `web_mail` üzerine düzeneğin sabit bir cümlesini
  taşır; bu, bu çalıştırmanın ölçümü değildir (`web` amacı kuruldu). set4/set5'in
  PHP sitesi ve M10 posta bölümleri kapatıldı.

1. ve 2. hücreler birlikte 51 dk 18 sn sürdü. Kaydın README'si kabulde ham
dosyalarına karşı düzeltildi (`2957b5196`, `f24d33a51`); buradaki rakamlar
README'nin bugünkü hâlindekilerdir ve README bu özete üstün gelir. Bu beş
laboratuvarda adayın hiçbir kusuru gözlenmedi. Çalıştırma hiçbir ürün dosyasını
değiştirmedi.

**Bu daldaki düzenek ve laboratuvar aracı değişiklikleri.**

- `a87ce0b97`: sürüm kurtarma yoklayıcısı (`guest_probe.py`,
  `owner_update_trial.py`) `managed_site_files` satırlarını, tam özeti ve
  `observed_at` olmadan kararlı bir özeti kaydeder; yalnız `observed_at`'te
  farklı bir tabloyu beklenmeyen değil uçucu olarak listeler; çevrimdışı
  testleriyle (`test_guest_probe.py`, `test_owner_update_trial.py`). İlk konuk
  çalıştırması set16'dır.
- `c2144b785`: laboratuvarın API taklidi lisans erişim denetimini Panel gibi
  `valid_for_seconds` ile yanıtlar ve bir `clockSkew` denetimi vardır; 9. ve 11.
  setlerin soğuk yükleme araçları zamanlı tıklamayı sayfanın son anına göre
  planlar. Bu, laboratuvar araçlarının yalnız `valid_until` gönderdiğini
  söyleyen alpha.84 sınırını kapatır. Taklit yerel bir bağlantı noktasında elle
  ölçüldü (etkin lisans için 59,2 sn kaldı; `clockSkew` −420 ile damga 420 sn
  kaydı ve kalan süre 59,1 sn kaldı; eksik lisans 0 yanıtladı); soğuk yükleme
  araçları çalıştırılmadı.
- `0b43ee81e`: on ikinci tarayıcı incelemesi turu (yukarıda) ve `run.mjs`
  içinde 1280×800'lük bir görünüm.
- `a945a4dbf`, `e3feee06d`: set16 düzeneği (`build-set16-artifacts.sh`,
  `run-set16.sh`, `set16_fixture.py`, 11 çevrimdışı testli `set16_trial.py`,
  `cold-load-set16.mjs`) ve kaydın kendisi.

**alpha.84 kayıtlarında bir düzeltme** (`8b09e4e61`). alpha.84 sürüm notları,
ROADMAP ve dayanıklılık sözleşmesi, sürüm dosyalarını yayımlayan işin test
işlerine değil yalnız paketleme işine bağlı olduğunu söylüyordu. Bu yanlıştı:
`publish-release` `package`'a, o da aynı koşunun her test ve derleme işine
bağlıdır; yani bir etiketin dosyaları ancak o koşu geçtikten sonra yayımlanır.
İngilizce ve Türkçe tarihli bir düzeltme eklendi; `main` koşusu ile etiket
arasındaki sıraya dair süreç notu geçerlidir.

## Bu sürümün sınırları

Bunlar bilinir. Gizli kusur değildir. İlk grup bu sürümle yenidir; ikincisi
alpha.84 listesidir, kısaltılmış.

Bu sürümle yeni:

- **Hiçbir kurulu sunucu ölçülmedi.** Bu sürümün hiçbir parçası kurulu bir
  sunucuda görülmedi; denetimin satırları koddan okundu ve gerçek sistem kaydı
  geçici konuklarda çalıştı.
- **"Kurulum tamamlanıyor" gerçek sistemde görülmedi.** Sahibin kurulu ikinci
  sunucuda karşılaştığı durum, hiçbir laboratuvar konuğunda olmayan bir ters
  DNS adı ister. Yalnız bağlanan testleri ve taklide karşı tarayıcısı var.
  Çalışanın geçen bir kontrolden sonraki kendi yeniden kontrolü öncekiyle
  tıpatıp aynı karşılanmamış sonucu bulursa sayfa bunu henüz yeniden kontrol
  yapılmamış olmaktan ayıramaz; o zaman hemen değil, 60 sn'lik "henüz
  bilinmiyor" bildirimiyle biter.
- **Güncelleme kartının meşgul durumu gerçek sistemde ölçülmedi.** Yalnız
  bağlanan testleri ve taklide karşı tarayıcısı var. Aynısı bileşen kilidinin
  yeniden bağlanması, yayılan alan adı kontrolleri, kurulum penceresinin
  yeniden okuması, beklemeler, sertifika, bölge, PHP ve içe aktarma simgeleri ve
  HSTS'nin kalan süresi için de geçerlidir: hiçbiri gerçek sistem kaydında
  üretilmedi.
- **Yanıt veren çözümleyici adlandırılmaz.** Agent ters DNS yanıtını hangi
  çözümleyicinin verdiğini bildirmez; bu yüzden sayfa bunu söyleyemez.
- **Agent değişikliği bir Agent güncellemesi ister.** Sonraki çözümleyiciye
  sormak yalnız Agent güncellendikten sonra etkili olur. O zamana kadar eski
  yanıtlı ilk çözümleyici yine uyuşmazlık gibi okunabilir; ama metin artık bu
  beklemeyi açıklar.
- **Aday, bir sabit commit olarak ölçüldü.** set16 `d5d673eb`'yi çalıştırdı
  (`89cf88397` artı sıra politikası); bu yüzden sayfaları `build d5d673ebd95e`
  okudu ve arşivleri 84 önyükleme sabitlerini taşıdı. Sürüm hazırlığı commit'i
  5443fe055 (85 sabitleri, sürüm satırları, sözleşme testleri) ve ondan
  sonraki belgeler ölçülmüş hiçbir derlemede yoktur; ürün kodları
  `89cf88397`'ninkidir.
- **alpha.84 tabanı yerel bir derlemeydi, yayımlanan arşivle bayt bayt aynı
  değildi.** Güncelleme `6dc899e5…`'ten değil, alpha.84 etiketinin test lisanslı
  yeniden derlemesinden (`9f86eb43…`) ölçüldü; o commit'in olağan arşivi
  derlenmedi, bu yüzden karşılaştırılmadı.
- **set16 bir ölçümdür, kabul değildir.** QEMU/KVM konukları, test lisanslı
  derlemeler, tek dizüstü ana makinede hücre başına tek çalıştırma, dışarıya
  NAT trafiği yakalanmadı; her `result.json` `native_evidence: false` taşır.
- **Geri dönüşten sonra kurtarma temel dosyası** çalışan derleme önceki sürüm
  olduğu hâlde kusurlu adayı adlandırmayı sürdürür ve taban adayın sırasını
  okur (set12, set15 ve set16). Kaydedildi, yargılanmadı; açık.
- **set16'nın O1–O3 gözlemleri** (sayfanın dil niteliği, Türkçe Bileşenler
  sayfasındaki İngilizce güvenlik duvarı motoru adı, Dashboard'daki ham denetim
  işlemi kimlikleri). Açık.
- **Şablonlu metin.** `component_not_installable` yalnız eski metinde bulunan
  platform ayrıntısını taşımaz; bir hizmet yerine bir rol adlandıran gereksinim
  rolün simgesiyle gösterilir. Posta profilleri dışındaki sunucu cümleleri hâlâ
  "rescan" der (`managed_service_handlers.go` içindeki önbellekli durum reddi,
  `store_backend.go` içindeki sunum durumları).
- **Çalıştırılmayanlar:** Arch; adayın temiz bir Ubuntu kurulumu; Posta
  paketleri kartının "bir değişiklik bekletiyor" durumu; değişiklikten sonra 9.
  ve 11. setlerin soğuk yükleme araçları.

Bu sürümün alpha.84 listesinden kapattıkları: O4, O5 ve O6; Posta paketleri
kartının bayat kopyası; Türkçe Bileşenler sayfasındaki çevrilmemiş posta profili
metni; kenar çubuğundaki yapı kimliğinin sözcük ortasından kırılması; HSTS
süresinin iki saati karşılaştırması; laboratuvar araçlarının yalnız
`valid_until` göndermesi; ve otomatik geri dönüşten sonraki `managed_site_files`
bulgusu, set16'nın sabit commit'i için yanıtlandı (yalnız `observed_at`
değişti).

v0.1.0-alpha.84'ün bu sürümün ele almadığı sınırları (her biri hâlâ açık),
kısaltılmış; tam liste [alpha.84 notlarındadır](RELEASE-NOTES-v0.1.0-alpha.84.tr.md):

- `server_setup_busy`, hiç kontrol edilmemiş durum, taramanın sunucu tarafı
  duvar süresi ve bir işlemden sonraki "tamamlanmış olabilir" şeridi ölçülmedi;
  tutulma durumuna yalnız önce taramayan bir değişiklikle ulaşılır.
- Agent yanıtındaki "hiçbir şey değiştirilmedi", aynı istekte daha önce bir
  değişiklik olmadığını varsayar; işleyici işleyici bir denetim iddia edilmez.
- On altıncı kayıtta konuğun NTP ayarları; on altıncı kaydın saat durumları
  yalnız Debian 13'te ölçüldü.
- Sunucuyu hâlâ eski biçimde okuyan ekranlar (sayı bu taslak için yeniden
  hesaplanmadı); erişim beklemesi; güncelleme bildirimi; kurulumun posta kimliği
  nedeni gerçek bir Agent'tan hiç okunmadı; ve alpha.84 notlarının hâlâ açık
  saydığı v0.1.0-alpha.83 ve v0.1.0-alpha.82 sınırları.
- **Açık kabul işleri.** [Dayanıklılık sözleşmesinin](RESILIENCE-CONTRACT.tr.md)
  beş temel işinin hepsi kısmi kalır; bu sürüm hiçbirini kapatmaz ve
  ilerletmez. P0.1–P0.5 **kısmi** kalır. Kararlar: [DECISIONS](DECISIONS.tr.md)
  içinde D-022, D-024, D-025, D-032 ve D-033.

## Yayımlama kaydı

Yayımlanmadı. Sahip yayımlayana kadar burada hiçbir şey kaydedilmez.
Yayımlamadan önce açık olanlar:

1. Yayımlama öncesi ölçüm: yapıldı (set16, on sekizinci kayıt, 14 Ekim 2026
   UTC), adayın ürün kodunda, bir sabit commit üzerinden: Debian 13'e temiz
   kurulum ve Debian 13 ile Ubuntu 24.04'te alpha.84'ten güncelleme, sağlam ve
   kusurlu. Sınırları yukarıdadır.
2. Sürüm hazırlığı: 5443fe055 commit'inde yapıldı; `cmd/`, `internal/` ya da
   `web/` altında hiçbir dosyayı değiştirmez: önceki olarak alpha.84 sürüm
   commit'i `f0157e6c8` ile 84'ten sonra 85 sürüm sırası politikası,
   `download-portal/get.sh` sabitleri (sıra 85 ve sürüm v0.1.0-alpha.85; açık
   anahtar sabiti değişmedi), README sürüm satırları ve geçen üç sözleşme
   testi. Hâlâ açık: sahibin etiketten önce ayarladığı depo değişkeni
   `CELIKPANEL_RELEASE_SEQUENCE=85`. Gerçek arşivi etikette CI derler ve imzalar.
3. Çekme isteğinin son ucu kendi CI koşusunu alır.
4. Sahip birleştirir, sürümü etiketler, altı imzalı dosyayı imzalı sürüm
   sözleşmesinin anlattığı gibi denetler ve indirme portalını yayımlar.
5. Sürüm yayımlandıktan sonra her kurulu sunucunun sahibi o sunucuyu panelin
   kendi güncelleme ekranından günceller. Bu, yukarıdaki her adımdan ayrı bir
   adımdır; onu başka kimse başlatmaz.

Bu sürümü yalnız CelikPanel'in güncelleme arayüzüyle kurun. Yayımlanması kurulu
sunucuları güncellemez; bir sunucunun güncellemesini yalnız sahibi, panelin
kendi güncelleme ekranından başlatır.
