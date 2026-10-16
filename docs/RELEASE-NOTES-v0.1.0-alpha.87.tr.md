# v0.1.0-alpha.87 (16 Ekim 2026'da yayımlandı)

[English](RELEASE-NOTES-v0.1.0-alpha.87.md)

Bu, v0.1.0-alpha.86'yı izleyen sürümdür. 16 Ekim 2026'da yayımlandı (sondaki
"Yayımlama kaydı"na bakın). v0.1.0-alpha.87 numarasını,
sahibin 13 Ekim 2026 tarihli kalıcı talimatı uyarınca ("hem sürüm numaraları
hem de commit işlemleri için bana sormana gerek yok": sürüm numaraları ve
commit'ler için sahibe sorulmaz) sürümü hazırlayan seçti; sahibin bu sürüm için
adını koyduğu bir numara değildir. Sahibin bu sürümdeki kendi payı, üzerinde
çalışılan hedeftir (ikinci kurulu sunucunun birincinin tam eşi olması) ve bu
işi alpha.86'nın sonrasına bırakan 15 Ekim 2026 tarihli "devam. sorun yok."
yanıtıdır. Aşağıda "Sürümü hazırlayanın verdiği kararlar" altında sayılan her
seçim sürümü hazırlayanındır; D-021, D-033 ve bu kalıcı talimat altında verildi,
sahibin onayına ya da değiştirmesine açıktır ve hiçbiri sahibin kararı değildir. Sürüm hazırlığı
`471377d60` commit'idir (86'dan sonra sıra 87; önceki olarak alpha.86 sürüm
commit'i `8d260f317e58bed7fcd84c6cbb01823c4fe3f166`). Yayımlama kaydı 16 Ekim
2026'da yazıldığında hiçbir kurulu sunucunun onu çalıştırdığı bildirilmemişti;
her birinin güncellemesini sahibi panelin kendi güncelleme ekranından başlatır.
Bu notlar 16 Ekim 2026'da (UTC), set18 ve set18b gerçek sistem kayıtlarından
sonra taslak olarak yazıldı ve aynı gün yayımlanma için güncellendi; adayın son
ürün commit'i `6b363ac46`'dır.

Sahip, ikinci kurulu sunucunun (Boston, çiftin ikincil DNS sunucusu) web
sitelerini ve postayı birincinin (Frankfurt, birincil DNS sunucusu) tam eşi
olarak barındırmasını istiyor. Kaynağın main `e3c8f735a` (13 Ekim 2026 tarihli main; alpha.85 ve
alpha.86'dan önce) üzerinde salt-okur incelenmesi (inceleme depoda bir dosya
olarak kayıtlı değildir), panelin sahibi oraya götüremediği sekiz yer buldu. Bunlardan beşi
sahibin kararını gerektirmiyor ve bu sürüm onları düzeltiyor: bu **birinci
bölümdür**. Kalan üçü sahibin kararını gerektiriyor ve bu sürümde yok; onlar
**ikinci bölümdür** ve düz sözlerle şunlardır:

1. Frankfurt'un kendi kurulumu Boston'un posta sunucusu adını da taşısın mı;
   böylece o DNS kaydını Frankfurt kendisi yayımlasın mı?
2. Panelde birincildeki sunucu altyapısının DNS kayıtları için bir düzenleyici
   olsun mu (bugün kurulum en küçük bir kümeyi bir kez yazıyor)?
3. Boston'un kendi kayıtları uzak DNS bağlantısıyla Frankfurt'a kendiliğinden mi
   gitsin, yoksa sahibin elle attığı bir adım olarak mı kalsın?

Bunlara karar verilene kadar Boston'daki bir posta sunucusu adının DNS kaydını
sahip elle, Frankfurt'ta ya da DNS sağlayıcısında yapar; panel onu
yönlendirmesiyle bekler (aşağıda).

Bu sürümün yaptıkları bileşen testlerine, API'nin taklidine karşı on altıncı
tarayıcı inceleme turuna (8 yapılandırmada 36 durum; bulunan ve düzeltilen yedi
kusur) ve kendi DNS'i ile kendi sertifika yetkilisi olan bir laboratuvarda
yapılan iki gerçek sistem kaydına dayanır: `5d54aba1e`'de set18 ve
`6b363ac46`'da set18b ("Gerçek sistem kayıtları set18 ve set18b" bölümü). set18
bir kusur (D1: genişletme tamamlanmış kurulumun çalışmasıyla açılıyordu; sahip
onu inceleyemiyor, başlatamıyor ya da ondan vazgeçemiyordu) ve ayrıca düzeltilen
iki gözlem (O1, O3) buldu; bu düzeltmeleri taşıyan aday üzerinde set18b üçünün
de düzeldiğini gözledi ve yeni kusur bulmadı. Hiç çalıştırılmayanları sınırlar
söyler.

## Ne göreceksiniz

Önce düz sözlerle, sunucu sahibi için; bir sunucu bu sürüme güncellendiğinde
ekranların neyi göstereceği (şimdilik hiçbir yerde kurulu değil).
Güncellemeden sonra açık olan her
CelikPanel sayfasını yeniden yükleyin; güncellemeden önce yüklenmiş bir sayfada
hâlâ eski ekranlar vardır. Bir sunucunun kurulumu bittiğinde "Sunucunuz hazır"
ekranı ve Ayarlar → Sunucu kurulumu artık **"Bu sunucunun kurulumunu
genişletin"** seçeneğini sunar. Seçtiğinizi, örneğin postayı, kurulumun
kullandığı adımların aynısıyla ekler ve önce neyi yapmadığını söyler: kurulu
hiçbir şeyi kaldırmaz, durdurmaz ya da değiştirmez, siteleriniz çalışmaya devam
eder ve incelemeyi onaylamadan hiçbir şey başlamaz. İnceleme neyin değiştiğini,
neyin kurulacağını ve neyin olduğu gibi kaldığını listeler. **"Genişletmeden
vazgeç"** sunucuyu kurulumunun tamamlandığı hâline döndürür ve hiçbir şey
kaldırmaz. Bileşenler sayfasında, kurulumu bitmiş bir sunucuya posta yığını
eklemek artık aynı genişletmeyi açar; böylece posta, kurulumun yönlendirmesiyle
genel DNS denetiminden, güvenilir bir sertifikadan ve yenilenmesinden, ters DNS
denetiminden ve dışarı giden 25 numaralı bağlantı noktası denetiminden geçer.
Kurulumu bitmemiş bir sunucuda posta yine doğrudan kurulur ve pencere artık
bunun geçici, kendinden imzalı bir sertifikayla bittiğini söyler. Kayıtlı bir
posta sunucusu adı "Değiştir" ile değiştirilebilir; alanın altındaki not, siz
onaylamadan önce değişikliğin ne anlama geldiğini söyler.

**Boston bu sürüme güncellendiğinde panelden posta ile genişletilebilir.** (Bu,
bileşen testlerine ve taklide dayanır: hiçbir gerçek sistem kaydı bir ikincil
DNS sunucusuna ulaşmadı ve Boston'da hiçbir şey koşmadı.) İkinci bölüme kadar bir
şey hâlâ elle yapılmalıdır: Boston'un posta sunucusu adının DNS kaydı (örneğin
`mail.example.com`, A türü, Boston'un genel adresi) Frankfurt'ta ya da DNS
sağlayıcınızda oluşturulmalıdır; çünkü Boston ikincil DNS sunucusudur ve
Frankfurt'un bölgelerinin yalnız kopyalarını tutar. Boston gibi bir çiftin
ikincil DNS sunucusu olan bir sunucuda Bileşenler sayfası artık tam olarak bunu
söyler, birincili adlandırır ve tam kaydı gösterir; sizi ikincil bir sunucunun
kayıt oluşturamadığı bir DNS ayarları sayfasına göndermez (Boston'un kendisinde
henüz görülmedi). Genişletmenin genel DNS adımı o kaydı bekler, neyi beklediğini
söyler ve kayıt görünür olunca kendiliğinden devam eder. Boston'un adresinin ters
DNS (PTR) kaydı, önceden olduğu gibi sunucu sağlayıcınızda ayarlanır. Buradaki
hiçbir şey bir sunucunun hizmetlerini kendiliğinden değiştirmez. Bu sürümün
yayımlanması hiçbir kurulu sunucuyu güncellemez; bunu siz, panelin kendi
güncelleme ekranından başlatırsınız.

## Sunucu sahibi için ne değişiyor

Aşağıdaki başlıklar, incelemenin boşluk numaralarını #215 numaralı çekme
isteğinin kullandığı biçimde korur: bu sürüm 1, 2, 3 ve 7 numaralı boşlukları
ve 8 numaralı boşluğun kalanını düzeltir; kalan üç boşluk yukarıdaki sahip
kararlarıdır. (İşlem yönlendirmesi aynı beş değişikliği 1'den 5'e numaralar.)
Her değişiklik D-021'i (kurulu olanı koruyan, amaca göre kurulum), D-024'ü
(mevcut neden, kimin işlem yapacağı, sonraki eylem ve işin nasıl süreceği),
D-032'yi (panel okuyabildiği bir durumu okur) ve D-033'ü (bir ekran ne yaptığını
ve neyi beklediğini söyler) izler. D-022 değişmedi: her adım, kurulumun zaten
kullandığı yerel hizmet yapılandırmasını kullanır.

### 1. Tamamlanmış bir kurulumu genişletmek

**Önce.** Kurulum hazır olunca kurulum sayfası ve Ayarlar → Sunucu kurulumu
yalnız "Sunucunuz hazır" gösteriyordu; posta eklemek, kurulumun denetimlerini
çalıştırmayan Bileşenler sayfası demekti (2. madde).

**Şimdi.** Hazır ekranı ve Ayarlar → Sunucu kurulumu, neyi yapıp neyi yapmadığını
söyleyen cümleyle "Bu sunucunun kurulumunu genişletin" seçeneğini sunar
(yalnız ekleneni kurar ve yapılandırır; kurulu hiçbir şeyi kaldırmaz, durdurmaz
ya da değiştirmez; siteler çalışmaya devam eder; incelemeden önce hiçbir şey
başlamaz). "Kurulumu genişlet" sihirbazı tamamlanmış değerlerle açar: amaç
adımı, kurulu bileşenlerin "Kurulu" / "Mevcut kurulum korunur" diye işaretlendiği
bileşenler adımı, erişim adımı ve üç bloklu bir inceleme: "Neler değişiyor" (her
ayar önce ve sonra), "Kurulacaklar" ve katlanmış olarak "Zaten kurulu, olduğu
gibi kalanlar". Çalıştırma, tek uçuşu ve "kurulum sürüyor" retleriyle var olan
kurulum çalıştırıcısıdır. "Genişletmeden vazgeç" kurulumu tamamlanmış taslağa
döndürür. Bir genişletme sahibi asla kuruluma yönlendirmez; Dashboard sunucunun
genişletildiğini ve çalışmaya devam ettiğini söyler; taslak için "Genişletmeye
devam et", çalışan genişletme için "İlerlemesini göster" bağlantısı vardır.

Kod. `cmd/panel/server_setup_extend.go`: `begin` ve `cancel` eylemleriyle
`POST /api/v1/setup/extend`; türlü retler `server_setup_busy`,
`server_setup_extend_not_ready`, `server_setup_extend_not_active` ve
`setup_conflict`; `begin` ve `cancel` her biri durum satırını ve bir ayarı
(`panel_settings` içinde `server_setup_extension_base`) tek işlemde değiştirir;
iki yeni denetim eylemi, `server.setup.extend` ve `server.setup.extend.cancel`,
"sunucu kurulumunu genişletmeye başladı" ve "sunucu kurulumunu genişletmekten
vazgeçti" der. `cmd/panel/server_setup.go`: kurulum durumu `extension` ve
`extension_base` alanlarını yanıtlar (ikisi de ek alan); bir genişletme
`completed_at`'i korur ve `required` değildir; hazır bir kurulumun üzerine
kaydedilen taslak tamamlanmış taslağı taban olarak tutar, tamamlanınca taban
silinir. `cmd/panel/server_setup_revise.go`: bir genişletme çalışmasının planını
yeniden açmak onu genişletme olarak tutar (`99e90ac74`). Web:
`web/src/components/ServerSetup.tsx`, `ServerSetupChoice.tsx`,
`ServerSetupGate.tsx`, `web/src/lib/serverSetup.ts` (`f51d98e4b`; 16. turun
düzeltmeleri `b67987865`'te).

Testler. `cmd/panel/server_setup_extend_test.go`: `99e90ac74`'te altı test
(`TestServerSetupExtendKeepsTheCompletedSetupAndItsBase`,
`TestServerSetupExtendRefusals`,
`TestServerSetupExtendAddsAMailProfileWithItsHostName`,
`TestServerSetupDraftWithMailProfile`,
`TestServerSetupExtensionMarkerFollowsTheCompletionRecord`,
`TestServerSetupReviseKeepsAnExtension`), her biri bir denetim ya da bir retle;
genişletme işareti ve düzenleme kuralı testlerin kaldığını görmek için birer kez
bozuldu. `web/tests/server-setup-runtime.test.mjs`: denetimli takılı ve çözücü
testleri (sihirbazda genişletme kapatılıp yönlendirme kuralı kaldırılınca dördü
kalıyor).

### 2. Bileşenler sayfasından posta kurulumun adımlarını alır

**Önce.** `POST /api/v1/service/profile/install` posta profilini kuruyor ve
kendinden imzalı yedek sertifikayla bitiyordu: posta sunucusu adının genel DNS
denetimi yok, güvenilir posta sertifikası yok, yenileme kaydı yok, ters DNS ya
da 25 numaralı bağlantı noktası denetimi yok.

**Şimdi.** Kurulumu bitmiş (ya da çalışan bir çalıştırma olmadan genişletilen)
bir sunucuda Bileşenler penceresi profili bir kurulum genişletmesine ekler ve
kurulumu, posta sunucusu adı doldurulmuş ve "Genişletmeden vazgeç" ile erişim
adımında açar. Pencere bunu önce söyler ("Bu sunucuda posta, sunucu kurulumu
üzerinden eklenir; böylece kurulumdaki adımların ve denetimlerin aynısından
geçer: …") ve düğmesi "Sunucu kurulumunda incele" der. İnceleme ve çalıştırma
kurulumun kendisidir: posta sunucusu adı için genel DNS beklemesi, güvenilir
sertifika ve yenilenmesi ve son denetimde çözümleyicinin adıyla ters DNS
beklemesi ve dürüst bir "okunamadı". Pencerede hiçbir şey başlamaz. İstek
yanıtlanırken pencere kilitlidir (İptal, ad alanı ve "{hostname} kalsın"
kapalıdır); bir ret, örneğin süren bir kurulum çalışması, pencereyi nedeniyle
açık tutar; kaybolan bir yanıt kurulum yeniden okunarak çözülür. Kayıtlı posta
sunucusu adı yoksa pencere onu sorar. Kurulumu bitmemiş bir sunucuda pencere
eskisi gibi doğrudan kurar ve artık bunun geçici, kendinden imzalı bir
sertifikayla bittiğini ve tam denetimlerin kurulumun işi olduğunu söyler.

Kod. `cmd/panel/server_setup_extend.go` içinde `serverSetupDraftWithMailProfile`
(`begin` üzerinde `mail_profile` ve `mail_hostname`);
`web/src/components/ServiceList.tsx` içinde `addProfileThroughSetup` ve
`MailProfileInstallDialog` (`450af0d88`; kilit ve görünür ret `c40f14f87`'de).
Testler: `TestServerSetupExtendAddsAMailProfileWithItsHostName`,
`TestServerSetupDraftWithMailProfile`;
`web/tests/remote-state-mounted-batch3.test.mjs` (kurulum yolu ve denetimi,
yanıt beklenirken kilitli ve sonra açılan, reddedilmiş bir genişletme).

### 3. Kayıtlı bir posta sunucusu adı düzeltilebilir

**Önce.** Bir posta sunucusu adı kaydedilince pencere onu yalnız söylüyordu.

**Şimdi.** "Değiştir" alanı kayıtlı adla açar. Hiçbir şey onaylanmadan önce
alanın altındaki not değişikliğin ne yaptığını söyler: posta sertifikası yeni ad
için alınır, Postfix kendini yeni ad olarak tanıtır ve sunucunun adresinin ters
DNS (PTR) kaydı sağlayıcıda ayarlanarak yeni adı göstermelidir. Ad, kurulumdaki
gibi doğrulanır. Hazırlanmış bir sunucuda düzeltme kurulum genişletmesinden geçer
(inceleme eski ve yeni adı gösterir). Doğrudan yolda ad artık yalnız işlem kabul
edilince kaydedilir; böylece bir kurulum çalışması ya da başka bir işlem sunucuyu
tuttuğu için gelen ret kayıtlı adı değiştirmez; önceden ad önce kaydediliyordu.

Kod. `cmd/panel/mail_profiles.go` içinde `handleMailProfileInstall`
(`eddc11610`); `MailProfileInstallDialog` (`450af0d88`; not yeni adı iki kez
söylüyordu, 16. turdan önceki bir tarayıcı incelemesinde bulundu ve `32f0a3766`'da
düzeltildi). Testler: `TestMailHostnameCorrectionIsSavedOnlyWhenAdmitted`
(önceki sırada kalıyor) ve `remote-state-mounted-batch3.test.mjs` içinde bir
takılı test.

### 7. İkincil DNS sunucusunda "DNS’i yapılandır" çıkmazı yok

**Önce.** DNS motoru bir çiftin ikincili olan (yerel kip) bir sunucuda posta
kutusu sahibi, ikincil bir sunucunun kayıt oluşturamadığı "DNS’i yapılandır"
sayfasına gönderiyordu.

**Şimdi.** Kutu, bu sunucunun birincilin bölgelerinin kopyalarını tuttuğunu ve
kayıt oluşturamadığını söyler, birincil DNS sunucusunu ve adresini kayıtlı DNS
ayarlarından adlandırır, kaydın orada ya da DNS sağlayıcısında yapıldığını
söyler, tam kaydı (posta sunucusu adı, A, bu sunucunun incelenmiş adresi)
gösterir ve bu sunucunun barındırma kayıtlarının nasıl yönetileceğini seçmek
için "Bu sunucunun kurulumunu genişletin" (kurulum bitmeden önce "Sunucu
kurulumunu açın") sunar; DNS ayarları düğmesi yoktur. Profil kartları da kapalı
bir düğmenin yanında "Kuruluma hazır" yerine "Posta DNS kaydı bekleniyor" der
(`c40f14f87`).

Kod. `cmd/panel/mail_dns_identity.go` içinde `mailProfileDNSGuidanceFor` ve
Bileşenler yanıtının ek alanı `mail_dns_guidance`
(`cmd/panel/managed_service_handlers.go`, `60ea75c1c`); `ServiceList.tsx`
içinde `MailProfileCards`. Kaydın değeri kurulumun incelenmiş adresi, yoksa
sunucunun algılanan adresidir. Testler: `cmd/panel/mail_hostname_edit_test.go`
içinde `TestMailProfileDNSGuidanceOnASecondaryNamesThePrimaryAndTheRecord` ve
`TestMailProfileDNSGuidanceIsAbsentOnAStandaloneServer` (denetimleriyle ikincil:
hazır bir kimlik, harici kip, tek başına bir sunucu); bekleme durumu için
genişletilmiş, denetimli bir takılı test.

### 8. "Gereksinimleri tekrar kontrol et" türlü nedeni söyler

**Önce.** Sahibin denetiminden sonra düğmenin altındaki kutu her açık denetim
için genel cümleyi söylüyor, üstteki yönlendirme ise türlü olanı (ters DNS
beklemesi için adres, bulunan ad, çözümleyici) söylüyordu.

**Şimdi.** İkisi aynı cümleyi kullanır (`web/src/lib/serverSetupGuidance.ts`
içinde `setupCheckLine`, `f51d98e4b`); okunamayan bir denetim okunamadı diye
söylenir. Yanıt geldiğinde görünür alana kaydırılır (`b67987865`).
`web/tests/server-setup-runtime.test.mjs` içinde testler: bir yeni ve bir
güncellenmiş (ikisi de önceki kutuyla kalıyor) ve takılı bir yeniden denetim
testi (okunurken kaydırma yok, sonra bir kaydırma).

### set18 düzeltmeleri: D1, O1 ve O3

set18 gerçek sistem kaydı (aşağıda) gerçek bir başsız Chrome'da temiz bir
kurulumu ve Core Mail ekleyen bir genişletmeyi hazır duruma kadar izledi.
Bulgularından üçü `6b363ac46`'da düzeltildi; ardından set18b üçünün de
düzeldiğini gözledi.

- **D1 (laboratuvarda doğrulanmış hata, iki yol, iki tarayıcı): genişletme
  tamamlanmış çalışmayla açılıyordu.** İstek kimliği olmadan
  `GET /api/v1/setup/operation` son çalışmayı yanıtlar; taslaktaki bir
  genişletme sırasında bu, tamamlanmış kurulumun kendi başarılı çalışmasıdır.
  Sayfa kurulumu yeniden okuyup `status: draft` görünce "Sonuç doğrulanıyor" ve
  "Tamamlanma doğrulanamadı… sunucu hazır olarak işaretlenmedi" gösteriyor, 3
  saniyede bir okuyordu; erişim adımı, inceleme, Başlat ve "Genişletmeden
  vazgeç" ulaşılamazdı: hazır bir sunucuda yanlış bir alarm. İşlem yanıtı artık
  tam olarak çalışma başarılı ve kurulum taslaktaki bir genişletme olduğunda ek
  alan `superseded: true` taşır (`cmd/panel/server_setup_operations.go` içinde
  `serverSetupRunSuperseded`, alan `server_setup_execution_context.go` içinde);
  eski bir sayfa bunu yok sayar ve çalışmanın kendisi yine yanıtlanır. Sayfa
  yerini almış çalışmayı güncel işlem saymaz (`ServerSetup.tsx` içinde
  `readExecution`) ve genişletmenin sihirbazını gösterir; o çalışmadan kalan bir
  başlatma işareti silinir. Çözücü alanı yalnız başarılı çalışmada tutar
  (`web/src/lib/serverSetupOperation.ts`); böylece süren bir çalışmayı ya da bir
  hatayı asla gizleyemez. Çalışması başarılı olup durumu hazır olmayan ilk
  kurulum yine "Tamamlanma doğrulanamadı" der. Testler:
  `TestServerSetupOperationMarksTheCompletedRunSupersededByAnExtension` (hazır
  olmayan ilk kurulum, hazır, iki okuma yolundan taslaktaki genişletme,
  bırakılmış, yeniden, çalışıyor; alan kapatılınca kalıyor) ve
  `server-setup-runtime.test.mjs` içinde takılı testler (alanla iki yolda ve eski
  işaretle sihirbaz ve "Genişletmeden vazgeç", denetim olarak alansız D1 ekranı;
  sayfa alanı yok sayınca iki takılı D1 testi de kalıyor).
- **O1: "İncelenen kurulumu başlat"tan sonra yaklaşık üç saniye yönlendirme
  genel "Bu adım için daha belirli bir gereksinim henüz doğrulanmadı…"
  cümlesini söylüyordu.** Yönlendirme artık çalışmanın kendi ilk adımını
  adlandırır ve "Kurulum burada gösterilen adımla başladı. Panel bu adımı
  birazdan başlatır ve bu sayfa kendiliğinden güncellenir; bir şey yapmanız
  gerekmiyor." der (`setup.guide.queued`, `setupExecutionGuidance`). Takılı test
  (cümle olmadan kalıyor).
- **O3: posta yenileme kaydı çalışırken "Kayıtlı işleme devam et" yaklaşık 4
  saniye etkin kaldı; araç dört örnekte bastı ve Panel'e üç POST ulaştı** (tek istek
  kimliği, ikinci kayıt yok). Basış artık bir okuma işlemin ilerlediğini gösterene kadar hatırlanır:
  düğme dönen işaretiyle kapalı kalır ve "Aynı işlem denetleniyor" der
  (`setup.mailEnrollment.sent`); hiçbir okumanın doğrulamadığı bir basış 15
  saniye sonra yeniden sunulur. Sunucu aynı isteğin tekrarını zaten ikinci işlem
  başlatmadan yanıtlıyordu. Takılı test (basış hatırlanmayınca kalıyor).

### On altıncı tarayıcı turunun düzeltmeleri

On altıncı tur (aşağıda) yukarıdaki değişiklikleri taklide karşı fotoğrafladı.
Gerçek sistem kayıtlarından önce düzeltilen yedi kusur buldu:

- "Sunucu kurulumunda incele" yanıtını beklerken İptal, ad alanı ve "{hostname}
  kalsın" kullanılabilir kalıyordu; kurulum sorulurken pencere kapatılabilir ya
  da ad değiştirilebilirdi; pencere artık yanıta kadar kilitli (`c40f14f87`);
- telefonda ret, pencerenin görünür gövdesinin altında çiziliyordu; görünür alana
  getirilir (`c40f14f87`);
- posta DNS kaydı hazır olmayan bir sunucuda kartlar kapalı bir "Kullanılamıyor"
  düğmesinin yanında "Kuruluma hazır" diyordu; artık "Posta DNS kaydı
  bekleniyor" der ve kapalı düğme kapalı görünür (`c40f14f87`);
- "Gereksinimleri tekrar kontrol et" yanıtı büyük ölçüde ekranın altında
  açılıyordu; görünür alana kaydırılır (`b67987865`);
- genişletme çalışırken Dashboard bildirimi ve Ayarlar "Genişletmeye devam et"in
  yanında "burada bir şey yapmanız gerekmiyor" diyordu; bağlantı "İlerlemesini
  göster" der (`b67987865`);
- inceleme "yok" sözcüğünü bir sunucu adının yanında eş aralıklı yazı yüzüyle
  çiziyordu; sözcük normal yüzde kalır (`b67987865`);
- "E-posta" ve "PHP-FPM" 390 pikselde tirelerinden bölünüyordu; bütün kalırlar
  (`b67987865`).

**Değişmeyenler.** Veritabanı şeması değişmedi (kayıt defteri 44'te kalır;
alpha.86 etiketi ile bu aday arasında taşıma yok: `git diff --stat
8d260f317..HEAD -- internal/ cmd/` hiçbir taşıma dosyası adlandırmaz).
Genişletme tabanı `panel_settings` içinde bir satırdır. `internal/` ya da
`cmd/agent/` altında hiçbir şey değişmedi: CelikPanel Agent alpha.86'nınkiyle
aynıdır. Güncelleme yolu, kurtarma çalıştırıcısı, sürüm tabanı ve yerel
hizmetler değişmedi.

## API değişiklikleri

Bir yeni yol, ek alanlar ve iki denetim eylemi.

### `POST /api/v1/setup/extend`

Gövde `{"revision": n, "action": "begin" | "cancel"}`; `begin` üzerinde
isteğe bağlı `mail_profile` ve `mail_hostname`. Çalışan bir çalıştırma
olmadan hazır bir kurulumda `begin` onu taslakta bir genişletmeye çevirir
(`completed_at` korunur) ve tamamlanmış taslağı taban olarak saklar; zaten
taslakta (ya da başarısız) olan bir genişletmeye ekleme yapar. `cancel` o
taslağı ve hazır durumu geri getirir. İncelenen başlatmadan önce sunucuda
hiçbir şey değişmez. Retler, her biri kendi koduyla `409`: `server_setup_busy`
(bir kurulum çalışması sürüyor), `server_setup_extend_not_ready` (kurulum
bitmedi), `server_setup_extend_not_active` (genişletme yokken vazgeçme) ve
`setup_conflict` (eski bir revizyon); bozuk bir istek `400`'dür. Denetim
eylemleri `server.setup.extend` ve `server.setup.extend.cancel`.

### `extension`, `extension_base` (kurulum durumu)

Kurulum durumu yanıtında: tamamlanmış bir kurulum genişletilirken
`extension: true` ve tamamlanmış taslak olan `extension_base`. Aksi hâlde
yoktur; bunları tanımayan bir istemci yok sayar.

### `superseded` (kurulum işlemi)

`GET /api/v1/setup/operation` üzerinde: yanıtlanan çalışma başarılıysa ve
kurulum taslakta bir genişletmeyse `superseded: true`. Aksi hâlde yoktur.

### `mail_dns_guidance` (Bileşenler yanıtı)

Yönetilen hizmetler yanıtında, posta DNS kaydı bu sunucuda yapılamadığında,
çünkü sunucu bir DNS çiftinin ikincilidir: `reason` (`dns_secondary`),
`record_type`, `primary_nameserver`, `primary_ip`, `record_name`,
`record_value`. Aksi hâlde yoktur.

## Fark edeceğiniz davranış değişiklikleri

- Hazır ekranı ve Ayarlar → Sunucu kurulumu "Bu sunucunun kurulumunu
  genişletin" seçeneğini sunar; sihirbazı kurulu bileşenleri korur ve
  incelemesi neyin değiştiğini, neyin kurulacağını ve neyin kaldığını söyler;
  "Genişletmeden vazgeç" hiçbir şey kaldırmaz.
- Kurulumu bitmiş bir sunucuda Bileşenler sayfasından bir posta yığını kurulumun
  genişletmesinden ve denetimlerinden geçer; kurulumu bitmemiş bir sunucuda
  doğrudan kurulum kendinden imzalı bittiğini söyler.
- Kayıtlı bir posta sunucusu adı değiştirilebilir; reddedilen bir değişiklik onu
  olduğu gibi bırakır.
- İkincil bir DNS sunucusunda posta kutusu "DNS’i yapılandır" yerine birincili ve
  tam kaydı adlandırır, kartlar "Posta DNS kaydı bekleniyor" der.
- "Gereksinimleri tekrar kontrol et" yanıtı türlü nedeni görünür alanda söyler.
- Başlayan bir kurulum ilk adımını hemen adlandırır; basılan "Kayıtlı işleme
  devam et" "Aynı işlem denetleniyor" olarak kapalı kalır.
- Web barındırma için kurulmuş bir sunucuya "Seçimi özelleştir" ile posta
  eklemek amaç etiketini "Özel kurulum" yapar (set18b O5; sınırlara bakın).

## Ne ölçüldü ve nasıl

**Bileşen testleri.** Bu adayın ürün kodu `6b363ac46`'nınkidir. Ondan sonraki
commit'ler `web/src`, `cmd/` ya da `internal/` altında hiçbir dosyayı
değiştirmez: `0be5c8e5a` (set18b'nin kaydı ve düzeneği), `90b3fe2f2` (set18 ve
set18b README'lerinin teslim alma düzeltmeleri), `main`'in birleştirilmesi
(`bef12820e`, alpha.86 yayımlama kaydı, yalnız belgeler), sürüm hazırlığı
`471377d60` ve bu belgeler.

- Web: `npm test` 1499'da 1499, set18 düzeltmelerinden (`6b363ac46`) sonra
  [İŞLEM YÖNLENDİRMESİ](OPERATION-GUIDANCE.tr.md) içinde kaydedildi (`npx tsc
  --noEmit` temiz ve `npm run build` paket bütçesi içinde) ve 16 Ekim 2026'da
  `bef12820e`'de aynı sayıyla yeniden koşuldu (21,7 sn), bu belgelerin gerçeklik denetimi de onu
  `b6c43a4b5`'te yeniden koştu (1499'da 1499; önce `tsc` koşan `npm run build`
  paket bütçesi içinde). Web sayısı alpha.86'da
  1483, birinci bölüm ilk yazıldığında 1494 (16 Ekim 2026 tarihli yönlendirme
  girdisi) ve set18 düzeltmelerinden sonra 1499 idi.
- Go, WSL konuğu `CelikPanel-S2-Debian`'da (Go 1.26.5): `go test ./cmd/panel`
  1557 üst düzey test geçti, 6 atlandı, 1675 alt test geçti; `60ea75c1c`'de
  kaydedildi. set18 düzeltmelerinden sonra `go test -run Setup ./cmd/panel` 181
  üst düzey test geçti (yönlendirmede kayıtlı), `go vet` ve gofmt temiz.
  `6b363ac46`'dan sonraki tam bir koşu bu notlardan önce kaydedilmemişti; bu
  yüzden onlar için 16 Ekim 2026'da `bef12820e`'de (`6b363ac46` ile aynı Go
  kodu) yapıldı: `go test -count=1 -v ./cmd/panel/...` 1558 üst düzey testi
  geçirdi, 6'sını atladı, 0 kaldı ve 1675 alt testi geçirdi (paket için 272,7
  sn, derlemeyle 288 sn); `go vet ./cmd/panel/...` ve gofmt temiz. Bu koşu sürümü
  hazırlayanın WSL konuğundaki ölçümüdür; depodaki hiçbir dosya onu kaydetmez ve
  bu belgelerin gerçeklik denetimi, WSL konuğu olmadan, onu yinelemedi. Denetim
  `git diff --stat 6b363ac46 HEAD -- cmd internal go.mod go.sum` çıktısının boş
  olduğunu doğruladı. `./cmd/panel/...` tek paket `./cmd/panel`'dir; `60ea75c1c`'dekinden bir fazla
  üst düzey test, `6b363ac46`'nın D1 testidir.
- Agent (`cmd/agent/`) ve `internal/` bu dalda değişmedi ve yeniden
  koşulmadı; sayıları alpha.86'nınkilerdir.
- Sürüm hazırlığının sözleşme testi geçiyor: okuduğu `v0.1.0-alpha.86`
  etiketine sahip Windows Git Bash kopyasında sıra politikası sözleşmesi ("86
  -> 87; historical bootstrap sequence 86"; bu belgelerin gerçeklik denetimi de
  yeniden koştu, çıktı aynı). İndirme portalı ve imzalı bildirim
  sözleşmeleri, anahtar dosyası denetimlerinin kaldığı Windows kopyasında
  reddeder (alpha.86'da olduğu gibi); WSL konuğunda `471377d60`'ın bir
  `git archive`'i üzerinde ikisi de geçer ("download portal contract passed",
  "signed release manifest contract passed", v0.1.0-alpha.87 için), sürümü
  hazırlayanın bildirdiği gibi (bir dosyada kayıtlı değildir ve WSL konuğu
  olmayan gerçeklik denetimi yeniden koşmadı).

**On altıncı tarayıcı inceleme turu** (`x16*`,
`web/tools/browser-inspect/scenarios-batch16.mjs` ve `mock-batch16.mjs`,
`5d54aba1e`). Döngü adresindeki taklide karşı Windows'ta gerçek bir Chrome: hazır
ekranı ve genişletme sihirbazı (amaç, korunan bileşenler, yeni posta sunucusu
adıyla erişim, incelemenin üç bloğu, "Genişletmeden vazgeç", reddedilen bir
başlatma), taslakta ve çalışan bir genişletme için Dashboard bildirimi ve
Ayarlar, hazırlanmış ve hazırlanmamış bir sunucuda Bileşenler posta penceresi
(kurulum yolu, doğrudan yol, "Değiştir", tutulup reddedilen bir istek),
birincil adıyla ve adsız ikincil DNS kutusu ve türlü ters DNS cümlesi ile
okunamayan bir denetimle "Gereksinimleri tekrar kontrol et" yanıtı: 8
yapılandırmada (1280×800 ve 390×844, İngilizce ve Türkçe, açık ve koyu) 36 durum
(senaryo kaynağından sayıldı). Yedi kusuru yukarıda listelendi. Turun resimleri
depoda saklanmaz ve koşu sonuçları bir dosyaya kaydedilmedi.

### Gerçek sistem kayıtları set18 ve set18b

Bu bölümdeki her rakamın doğruluk kaynağı kaydın kendi README'sidir:
`deploy/e2e/release-recovery/evidence/set18-20261015/README.md` (yirmi birinci
gerçek sistem kaydı; `ddea67344`) ve
`deploy/e2e/release-recovery/evidence/set18b-20261016/README.md` (yirmi
ikincisi; `0be5c8e5a`). Bu özet ile bir README ayrıştığında README geçerlidir.
İkisi de ölçümdür, kabul kanıtı değildir: QEMU/KVM konukları, test lisanslı
(`acceptance_license`) derlemeler, her `result.json`'da `native_evidence:
false`, yalnız Debian 13. Hiçbir kurulu sunucuya, lisans hizmetine ya da
sertifika yetkilisine gidilmedi ve celikpanel.net ile bağlantı kurulmadı; sürüm
kaynağının adı ve sertifika yetkilisi adları ilk sürücü adımı olarak konuğun
döngü adresine sabitlendi ve sonunda geri okundu (iki kayıtta da geçti). Hiçbir
P0 satırını kapatmazlar. Aşağıdaki rakamlar README'lerin `90b3fe2f2`'deki
teslim alma düzeltmelerinden sonraki hâlini izler (hiçbir ham dosya değişmedi).

**Laboratuvar (ürün değişikliği değil, laboratuvar değişiklikleri;
`guest_set18_lab_dns.py`).** Bu kanıt klasöründe ilk kez bir laboratuvar,
interneti konuğun içinde taklit ederek kurulumun genel adımlarının gerçek
sistemde çalışmasına izin verdi:

- *Laboratuvar DNS'i.* 127.0.0.1 üzerinde küçük bir yanıtlayıcı `set18.test`
  bölgesini ve 192.0.2.0/24'ün ters bölgesini yanıtladı; nftables DNAT, ürünün
  aradığı genel çözümleyicileri (1.1.1.1, 8.8.8.8, 9.9.9.9, 208.67.222.222;
  `cmd/` ve `internal/` aranarak bulundu) ona gönderdi. Sunucunun adresi
  `192.0.2.10` idi (TEST-NET-1, ürüne göre "genel"). Sonraki iki kayıt
  değişikliği sahibin DNS sağlayıcısındaki (posta A kaydı) ve sunucu
  sağlayıcısındaki (PTR) eylemlerinin yerine geçti.
- *Laboratuvar ACME'si.* Konuğun içinde pebble 2.6.0; certbot'un kendi kullanıcı
  yapılandırmasıyla (`/root/.config/letsencrypt/cli.ini`) seçildi, laboratuvar
  kökü sistem güveninde; ürün certbot'u `--server` olmadan çalıştırdığı için bu
  hiçbir ürün değişikliği gerektirmedi. İki sertifika da (panel ve posta)
  pebble'dan geldi.
- *443 numaralı bağlantı noktası kapalı.* Dışarı giden TCP ve UDP 443, döngü
  adresi, laboratuvar ağları ve paket sunucuları dışında reddedildi; her kayıtta
  reddedilen tek 443 bağlantısı laboratuvarın kendi öz denetimidir (ret sayacı
  `packets 1`). **Hiçbir ACME isteği konuktan çıkmadı.**
- *25 numaralı bağlantı noktası.* Ürünün dışarı giden 25 denetimini DNAT ile
  konuğun kendi Postfix'i yanıtladı; bu yüzden "Giden posta (25 numaralı
  bağlantı noktası)" geçmesi laboratuvarın bir özelliğidir, herhangi bir ağın
  ölçümü değildir; hiçbir SMTP konuktan çıkmadı.

**set18: `5d54aba1e`'deki aday, 15 Ekim 2026 (UTC).** Koşu klasörü 22:38:28Z'de
oluşturuldu, son laboratuvar 23:27:49Z'de bitti. Kurulan arşiv `5d54aba1e`
üzerinde yalnız politikayı değiştiren bir düzenektir (`c84ebee3…`,
v0.1.0-alpha.87 / 87 etiketli, önceki alpha.86 etiket commit'i), arşiv SHA-256
`708e2030d5c46b9074c94a1ced72cc122d96beb5ba77419e8f9df64e6e87b0a6`. s18-a
laboratuvarı hiçbir kurulumdan önce durdu (H1: bulut imajında `nft` yok); s18-b
koştu; genel sonucu yalnız sertifika devrinden sonra sürücünün API okumaları
kaldığı için `inconclusive` (H2, bir düzenek kusuru).

- *Hazır duruma kurulum, bu kanıt klasöründe ilk kez* (oradaki daha önceki
  hiçbir README "Your server is ready" kaydetmez). Amaç Web barındırma, panel
  adresi `panel.set18.test`, harici DNS (sayfa İngilizceydi). Her adım gerçek
  sistemde koştu: "Verify public DNS before certificate issuance" (3 saniye
  arayla alınan iki örnek arasında başarılı oldu, kendi durumu olarak hiç
  örneklenmedi) ve laboratuvar CA'sından güvenilir bir sertifika ve planlı
  Panel yeniden başlamasıyla ("The Panel restarts at this step") "Secure panel
  access" dahil; ardından 115 saniyede "Your server is ready". Son denetimde
  hiçbir bekleme durumu örneklenmediği için "Finishing setup" burada görünmedi.
- *Bileşenler posta yolu.* Pencere kurulum yolu notunu ve "Review in server
  setup" düğmesini gösterdi; basış, taslakta postfix ve dovecot ile posta
  sunucusu adı olan `extension: true`, revizyon 4 ile yanıtlandı. Erişim
  adımına inmedi (D1).
- *Hazır duruma genişletme.* D1 yüzünden sihirbazın kendi istekleri sayfanın
  oturumuyla kayıtlı vekiller olarak yapıldı (başlat, planla, çalıştır).
  Çalıştırma sonra ekranda izlendi (sayfa İngilizceydi; cümleler olduğu gibi
  aktarılır): "Verify public DNS before certificate issuance: mail.set18.test"
  adımında "Setup is waiting for a requirement" ve "Certificate issuance
  requires this public A record: mail.set18.test → 192.0.2.10. … Create or
  correct this record at the DNS provider or primary that manages it. … This
  prerequisite is checked automatically. After it passes, the same setup
  continues.", A kaydı eklenene kadar 9 dk 11 sn; posta sertifikası; posta
  yenileme kaydı (O3); son denetimde "The public resolver 1.1.1.1 reports
  host-192-0-2-10.provider.set18.test as the reverse DNS (PTR) name of this
  server's address 192.0.2.10; mail servers expect mail.set18.test. You set
  this at your server provider, not in CelikPanel: …"; PTR değişikliği ve
  "Check requirements again"ten sonra **"Finishing setup"** (o basıştan 1,3 sn
  sonra: "Every check passed. The panel is completing setup now, and you do
  not need to do anything. …"; bu kanıt klasöründe ilk kez), ardından "Your
  server is ready". Aracın örneklerinde 689 sn izlendi (başlatma isteğinin
  yanıtından hazır duruma 705 sn). Sonrasında Postfix `myhostname =
  mail.set18.test`, 587'de laboratuvar CA sertifikası (993: aynı yayıncı ve
  geçerlilik, SAN satırı kaydedilmedi), sistem ana makine adı korundu.
- *Bulgular.* **D1** (kusur, laboratuvarda iki yolda ve iki tarayıcıda
  doğrulandı; yukarıda). **O1** (Başlat'tan sonra yaklaşık üç saniye genel
  cümle) ve **O3** (devam düğmesi yaklaşık 4 saniye etkin; üç POST, tek istek
  kimliği, ikinci kayıt yok). **O2**: devirden sonra Panel, SNI göndermeyen bir
  istemciye özgün kendinden imzalı sertifikasını sunar; değerlendirilmedi.
  **O4**: devir bildirimi, çalıştırmanın sunucu adresi olmadan önce 2083 yerine
  laboratuvar yönlendirmesinin bağlantı noktasını sunar; bir laboratuvar izi,
  değerlendirilmedi. H1–H4 düzenek notlarıdır.

**set18b: `6b363ac46`'daki aday, 16 Ekim 2026 (UTC).** Derleme
23:59:40Z–00:03:04Z, laboratuvar 00:03:56Z–00:17:33Z, set18'in laboratuvarı
değiştirilmeden; kurulan arşiv aynı türden, yalnız politikayı değiştiren bir
düzenek (`a97050d4…`), SHA-256
`6d645d8def2316853d7b1c191b4771c48d51fc65c925d3b0aae3e3312ad0464b`. Tek
laboratuvar, genel sonuç `complete-for-review`; devirden sonra sürücünün
okumaları 200 ile yanıtlandı (H2 düzeldi).

- *Hazır duruma kurulum* 119 saniyede (son denetimde bekleme durumu
  örneklenmedi, bu yüzden burada "Finishing setup" yok); Başlat'tan yaklaşık
  0,5 saniye sonra yönlendirme
  "Configure DNS: Use an external DNS provider"ı "Setup has started with the
  step shown here. …" ile adlandırdı (O1 düzeldi). 390×844'te Türkçe hazır
  ekranı ("Sunucunuz hazır", "Bu sunucunun kurulumunu genişletin", "Kurulumu
  genişlet"), yatay taşma yok.
- *Bileşenler yolu* genişletmenin erişim adımına indi: "Extend this server's
  setup", posta sunucusu adı `mail.set18.test` olarak önceden dolu, "Stopping
  returns this server to its setup as it was completed. Nothing installed is
  removed." ile "Stop extending"; 6 saniyedeki on iki örnekte tamamlanmış
  çalışmanın hiçbir izi yok. "Stop extending" Postfix ve Dovecot kurulmamışken
  "Your server is ready" ekranına döndü (kayıtlı Bileşenler yanıtından: aracın
  kendi özeti yanıtta olmayan bir alanı okudu, H6).
- *Hazır ekranından genişletme*, her adıma sayfada basıldı (vekil yok): Web
  barındırma işaretli amaç adımı; Nginx, PHP-FPM ve MariaDB'nin "Installed" /
  "Keep existing installation" olduğu ve Postfix + Dovecot'u sahibin
  işaretlediği bileşenler adımı; erişim adımı ve inceleme: "What changes:
  Server purpose: Web hosting → Custom setup; Mail server name: none →
  mail.set18.test", "To be installed: Prepare mail profile: Core Mail",
  "Already installed, kept as they are (6)"; 390×844'te Türkçesi aynı ("Neler
  değişiyor: Sunucunun kullanım amacı Web barındırma → Özel kurulum; Posta
  sunucusu adı yok → mail.set18.test", "Kurulacaklar: E-posta profilini hazırla:
  Temel Posta", "Zaten kurulu, olduğu gibi kalanlar (6)"). Çalıştırma: ilk adım
  hemen adlandırıldı (O1); posta A kaydı beklemesi (aracın kendi tetikleyicisi
  kaydı ekleyene kadar 57 sn; H3 düzeldi); posta sertifikası; "Continue recorded
  operation" bir kez basıldı, sonra basıştan 0,3 sn ve 2 sn sonra "Checking the
  same operation" olarak kapalı; bütün çalıştırmada tek POST (O3 düzeldi);
  1.1.1.1'i adlandıran ters DNS cümlesi (58 sn); "Finishing setup"; "Custom
  setup" etiketiyle "Your server is ready". Çalıştırma 205 sn. Sonrasında
  Postfix ve Dovecot kurulu ve çalışıyor (kayıtlı Bileşenler yanıtı, H6),
  `myhostname = mail.set18.test`, 2083'te (SNI
  ile), 587 ve 993'te laboratuvar CA sertifikaları, certbot.timer etkin, sistem
  ana makine adı korunmuş, kayıt defteri sürüm 44.
- *Hükümler.* D1, O1 ve O3 **düzeldi (gözlendi)**. O2 ve O4 değişmedi,
  değerlendirilmedi. **Yeni kusur yok.** **O5** (gözlem, değerlendirilmedi):
  Core Mail'i "Seçimi özelleştir" ile eklemek amaç etiketini "Özel kurulum"
  yapar; Başlat'tan önce incelemede ve sonra hazır ekranında. Doğrudur (seçim
  artık Web barındırma hazır seçimi değil) ve Başlat'tan önce söylenir; sürümü
  hazırlayan etiketi olduğu gibi bırakır ve burada kaydeder. Etiketi olduğu gibi
  bırakma kararı, D-021 ve D-033 ile sahibin 13 Ekim 2026 tarihli kalıcı
  talimatına dayanan sürüm hazırlayanın kararıdır; sahibin kararı değildir.
  Posta eklenmiş bir web sunucusunun bunun yerine "Web barındırma" mı kalacağı
  yoksa "Web ve posta barındırma" mı okuyacağı sahibin değiştirmesine açıktır;
  bu konuda sahibin verdiği bir karar yoktur.

**Bu daldaki düzenek ve laboratuvar aracı değişiklikleri.**

- `5d54aba1e`: on altıncı tarayıcı turu (`scenarios-batch16.mjs`,
  `mock-batch16.mjs`).
- `ddea67344`: set18'in düzeneği (`set18_fixture.py`, `build-set18-artifacts.sh`,
  `set18_trial.py`, `run-set18.sh`, laboratuvar hazırlığı
  `guest_set18_lab_dns.py`, `web/tools/browser-inspect/cold-load-set18.mjs`);
  `0be5c8e5a`: set18b'ninki (`set18b_fixture.py`, `build-set18b-artifacts.sh`,
  H2 düzeltmesiyle `set18b_trial.py`, `run-set18b.sh`, H3 düzeltmesiyle
  `cold-load-set18b.mjs`). Yalnız laboratuvar kodu.

## Sürümü hazırlayanın verdiği kararlar

Sürümü hazırlayan tarafından D-021 ve D-033 altında ve sahibin 13 Ekim 2026
tarihli kalıcı talimatıyla verildi ve sahibin onayı için işlem
yönlendirmesinde kaydedildi; hiçbiri sahibin kararı değildir.

- Genişletme, var olan PUT üzerinde bir bayrak değil, yeni bir uç noktadır:
  `POST /api/v1/setup/extend`.
- Bir genişletme `completed_at`'i korur ve `required` değildir: sahibi asla
  kuruluma geri göndermez.
- "Genişletmeden vazgeç" vardır ve tamamlanmış taslağı geri getirir; kısmen
  çalışmış bir genişletmenin kurduğu her şey kurulu kalır ve Bileşenler'den
  yönetilir.
- Genişletme amaç adımında açılır; Bileşenler'den başlatılan erişim adımında
  açılır.
- Hazırlanmış bir sunucuda Bileşenler penceresi postayı artık doğrudan kurmaz;
  kuruluma yönlendirir. Kurulumu bitmemiş bir sunucu, kendinden imzalı cümlesiyle
  doğrudan kurulumu korur.
- Bileşensiz bir taslağa profil eklemek onu eşleşen bileşen seçimine çevirir;
  yalnız DNS amacı "Özel kurulum" olur; Web barındırmaya "Seçimi özelleştir" ile
  posta eklemek "Özel kurulum" okur (O5, olduğu gibi bırakıldı).
- İkincil bir sunucuda kaydın değeri kurulumun incelenmiş adresi, yoksa
  sunucunun algılanan adresidir.
- Web barındırmanın, "Seçimi özelleştir" ile posta eklendikten sonra okuduğu
  "Özel kurulum" etiketi (set18b O5) olduğu gibi bırakılır.
- Dalın sürüm belgelerinden önceki commit'leri onları yazan modelin atfını
  taşır.

[KARARLAR](DECISIONS.tr.md) belgesine yeni bir girdi eklenmez: tamamlanmış bir
kurulumu genişletmek D-021'i uygular (kurulum kaldığı yerden sürer ve kurulu
olanı korur; bir seçimi temizlemek kurulu bir hizmeti asla kaldırmaz) ve
ekranlar D-033'ü uygular. D-021 ve D-033 altındaki tarihli notlar bunun nasıl
olduğunu kaydeder.

## Bu sürümün sınırları

Bunlar bilinir. Gizli kusur değildir.

- **İkinci bölüm bu sürümde yok.** Yukarıdaki üç sahip kararı açık. Boston'da
  (ikincil DNS sunucusu) bir posta sunucusu adının DNS kaydını sahip elle,
  Frankfurt'ta ya da DNS sağlayıcısında yapar; genişletmenin genel DNS adımı onu
  yönlendirmesiyle bekler ve kendiliğinden devam eder. Panel o kaydı hiçbir yerde
  oluşturmaz.
- **İkincil DNS kutularına gerçek sistemde ulaşılmadı.** Ne bir DNS çiftindeki
  ikincil için Bileşenler kutusu (`mail_dns_guidance`; birincili olan
  tamamlanmış bir ikincil kurulum gerektirir, denenmedi) ne de kurulum erişim
  adımının ikincil kutusu (set18'de D1 ulaşılamaz kıldı; set18b'de aracın "Bu
  sunucu"ya tıklaması seçimi değiştirmedi, araştırılmadı) gerçek bir sunucuda
  çizilmiş görüldü. Bileşen testlerine ve taklide karşı on altıncı tura dayanır.
- **Gerçek sistemde üretilmeyen çeşitler.** Çözümleyiciler ayrıştığında ters DNS
  cümlesi (iki laboratuvar yanıtlayıcısı da aynı yanıtı verdi), yavaş "Kurulum
  tamamlanıyor" çeşidi ve "Kurulum tamamlanıyor"dan sonra kalan bir son denetim.
  set18b'nin kurulumunda Panel'in kendi yeniden başlaması ayrı bir sayfa durumu
  olarak yakalanmadı.
- **İnternet değil, laboratuvar.** Her genel adım laboratuvarın DNS
  yanıtlayıcısına ve laboratuvarın pebble'ına karşı koştu; genel ACME ve DNS yolu
  (Let's Encrypt, gerçek çözümleyiciler, gerçek bir sağlayıcının PTR'si)
  ölçülmedi. "Giden posta (25 numaralı bağlantı noktası)" konuğun kendi Postfix'i
  yanıtladığı için geçti.
- **Platformlar.** Yalnız Debian 13. Ubuntu 24.04 ve Arch koşulmadı.
- **Doğrudan Bileşenler kurulumu**, kurulumu bitmemiş bir sunucuda hâlâ kendinden
  imzalı biter; pencere bunu söyler.
- **Teslim alma sırasında ham dosyalarda gözlendi, değerlendirilmedi** (`90b3fe2f2`;
  hiçbiri sayfa metni örneklerinde yok ve hiçbiri bu sürüm için
  araştırılmadı):
  1. set18'de Panel'in günlüğü
     (`deploy/e2e/release-recovery/evidence/set18-20261015/debian13/fresh-setup-b/driver/steps/11-collect/journal-product.txt`)
     kurulum sırasındaki 22:57:19Z yeniden başlaması dahil her Panel
     başlangıcında, posta sunucusu adı kaydedilmemiş yalnız web kurulumunda,
     `certificate startup reconcile: certificate dependents: publish full mail
     SNI snapshot: mail server hostname is not a valid FQDN: invalid hostname
     "dns-debian13"` yazar (set18b'nin günlüğünde aynı satır üç Panel
     başlangıcının her birinde vardır).
  2. Aynı günlük `(CRON) DEATH (can't lock
     /var/run/crond.pid, otherpid may be 15418 …)` satırını 22:56:58Z'den
     (sonra 22:56:59Z, 22:57:19Z ve devamı) 23:27:27Z'ye kadar 128 kez yazar;
     set18b'nin günlüğünde 23 böyle satır vardır.
  3. set18'in genişletmesi posta A kaydını beklerken ekranda "core-mail
     installed with a self-signed fallback certificate. Mail clients will show
     a trust warning until a matching trusted certificate is configured."
     uyarı bildirimi vardı
     (`set18-20261015/debian13/fresh-setup-b/browser/resume/c2-state-02-viewport.png`);
     oysa güvenilir laboratuvar CA posta sertifikası aynı çalıştırmada geldi.
     O an için doğru olandan fazlasını söyleyen bir ekran: sonraki sürüm için
     D-033 adayı bir iş.
  4. Lisans kapısının "The latest access check failed. Retrying automatically;
     the current page is preserved while the last verified access remains
     valid." şeridi, "Check again" ve "Reload page" ile, kurulumun kendi
     planlı Panel yeniden başlaması çevresinde çizildi: set18'de devirde
     (`set18-20261015/debian13/fresh-setup-b/browser/setup/c1-state-10-viewport.png`,
     22:57:18Z) ve set18b'de hemen ardından son denetimde
     (`set18b-20261016/debian13/fresh-setup-a/browser/setup/c1-state-09-viewport.png`,
     00:08:29Z). alpha.86'nın yeniden başlama payı yalnız güncellemenin kendi
     yeniden başlamasını kapsar, kurulumun planlı yeniden başlamasını kapsamaz:
     sonraki sürüm için bir iş.
  Ayrıca: set18 ve set18b'nin tarayıcı araçları Bileşenler'i yanıtta olmayan bir
  alanla (`is_installed` yerine `installed`) özetledi; bu yüzden README'lerdeki
  ve buradaki kurulu / kurulmamış değerleri kayıtlı API yanıtlarından gelir
  (set18b H6).
- **O2, O4 ve O5 değerlendirilmedi.** O2: devirden sonra SNI'sız bir istemci
  Panel'in özgün kendinden imzalı sertifikasını alır. O4: devir adresinde bir
  laboratuvar izi. O5: posta eklendikten sonra "Özel kurulum" etiketi; sürümü hazırlayanın
  kararıyla olduğu gibi bırakıldı (sözcük sahibin değiştirmesine açıktır).
- **Agent değişmedi.** `cmd/agent/` ya da `internal/` altında hiçbir şey
  değişmedi; Agent'ın alpha.86 sınırları (çözümleyici cümlesi bir Agent
  güncellemesi gerektirir) olduğu gibi durur.
- **Hiçbir kurulu sunucu ölçülmedi.** Bu adaydaki hiçbir şey kurulu bir sunucuya
  ulaşmadı. Gerçek sistem kayıtları test lisanslı derlemeler ve bir laboratuvar
  DNS'i ile ACME'si olan QEMU/KVM konuklarıdır; ölçümdür, kabul değildir.

v0.1.0-alpha.86'nın bu sürümün ele almadığı sınırları (her biri hâlâ açık),
özetle; tam liste [alpha.86 notlarında](RELEASE-NOTES-v0.1.0-alpha.86.tr.md):

- `7265d3c99`'un F1 kilit uzatması ve oturum kapısının `PANEL_STARTING` yolu
  gerçek sistemde ölçülmedi; 6 dakikalık gecikme metni üretilmedi; 6 dakikalık
  sınır laboratuvar güncellemelerine ve bir sahip gözlemine dayanır.
- alpha.86'nın yeni güncelleme ekranı kurulu bir sunucuda yalnız alpha.86'dan
  sonraki güncellemeden, yani bu sürüme güncellemeden görülür. Sahibin o
  güncelleme sırasında gördüklerini sahip bildirecektir.
- alpha.86 yayımından sonra sahibin bildirmedikleri: her sunucunun güncelleme
  sonucu ve yeniden başlama sırasında güncelleme sayfasının gösterdikleri, tam
  ekran lisans sayfası, Agent'ın sürümü, güncellemeden sonra Frankfurt'un kurulum
  son adımı, `postconf -n >/dev/null` çıktısı.
- Bir geri dönüşten sonra kurtarma temel dosyası hâlâ kusurlu adayı adlandırır
  (set12, set15, set16, set17); denetim günlüğü aktör türü saklamaz; Arch; temiz
  bir Ubuntu kurulumu; ve alpha.86 notlarının hâlâ açık saydığı önceki sürümlerin
  sınırları.
- **Açık kabul işleri.** [Dayanıklılık sözleşmesinin](RESILIENCE-CONTRACT.tr.md)
  beş temel işinin hepsi kısmi kalır; bu sürüm hiçbirini kapatmaz ve
  ilerletmez. P0.1–P0.5 **kısmi** kalır. Bu sürümün yayımlanması onun kabulü
  olmaz. Kararlar: [KARARLAR](DECISIONS.tr.md) içinde D-021, D-022, D-024,
  D-025, D-032 ve D-033.

## Yayımlama kaydı

Aşağıdaki her şey 16 Ekim 2026 tarihlidir (saat tarihi; saat dilimi
belirtilmedikçe UTC). 1'den 3'e kadar olan maddeler hazırlıktır; yayımlamadan
önce yazıldılar ve geçmiş olarak korunur. Birleştirmeyi (4. madde), depo
değişkenini ve etiketi (6. madde), dosyaların, portal paketinin ve genel sitenin
denetimlerini ve portalın yayımlanmasını (9. madde) sürümü hazırlayan kişi yaptı;
sahibin alpha.85 notlarında kayıtlı 15 Ekim 2026 tarihli talimatı uyarınca
("bunları neden sen yapmıyorsun… benden istiyorsun"): birleştirme, depo
değişkeni, etiket ve portal yayımı sürümü hazırlayan kişiye aittir; kurulu panel
güncellemelerini yalnız sahip yapar. Bu girdi, aşağıdaki sonuçların hiçbirini
depoda bir dosya olarak saklamaz.

**Ne oldu.**

1. Yayımlama öncesi ölçüm: yapıldı. `5d54aba1e`'de set18 ve `6b363ac46`'da
   set18b; README'lerinden yukarıdaki "Gerçek sistem kayıtları set18 ve
   set18b" bölümünde özetlendi. Adayın ürün kodu, set18b'nin ölçtüğü
   `6b363ac46`'nınkidir.
2. Sürüm hazırlığı: `471377d60` commit'inde yapıldı; `cmd/`, `internal/` ya da
   `web/` altında hiçbir dosyayı değiştirmez: önceki olarak alpha.86 sürüm
   commit'i `8d260f317e58bed7fcd84c6cbb01823c4fe3f166` ile 86'dan sonra 87
   sürüm sırası politikası, `download-portal/get.sh` sabitlemeleri (sıra 87 ve
   sürüm v0.1.0-alpha.87; açık anahtar sabitlemesi değişmedi), README sürüm
   satırları ve üç sözleşme testi. Depo değişkeni 6. maddededir. Gerçek arşivi
   etikette CI derledi ve imzaladı.
3. Numara: v0.1.0-alpha.87; sürümü hazırlayan seçti, sahibin 13 Ekim 2026
   tarihli kalıcı talimatı uyarınca. Sahibin adını koyduğu bir numara değildir.
4. #215 numaralı çekme isteği (sürüm adayı; CI: 24 iş, hepsi başarılı ya da
   atlandı) 16 Ekim 2026'da sürümü hazırlayan kişi tarafından `main`'e squash
   olarak birleştirildi. `main`, `a8cddcb546302daa0f5eb65b18fe6b9566b69e66`
   commit'idir, ağacı `c63a9600572cba0dc6bd19ecb0388acb45b395c4`.
5. O `main` commit'indeki sürekli tümleştirme koşusu (koşu 38834849023) 24 işle
   başarıyla sonuçlandı. Etiket ancak koşu bittikten sonra gönderildi; yani
   "`main`'deki koşu bitti, sonra etiket" sırası izlendi.
6. Sürümü hazırlayan kişi `CELIKPANEL_RELEASE_SEQUENCE` depo değişkenini 86'dan
   87'ye çıkardı, sonra yukarıdaki `main` commit'inde `v0.1.0-alpha.87`
   açıklamalı etiketini (etiket nesnesi
   `f277e0b469acd3a33b9fb87c3327566ef1c2ba04`) oluşturdu ve gönderdi. Etiketin CI
   koşusu (38836331003) 24 işle başarıyla tamamlandı ve sürüm
   2026-10-16T01:57:38Z'de ön sürüm olarak altı dosyayla yayımlandı: iki arşiv,
   iki sağlama toplamı dosyası, imzalı manifest ve imzası.
7. Dosyaları sürümü hazırlayan kişi bir indirmeden denetledi (`gh release
   download`; denetimler `CelikPanel-S2-Debian` konuğunda; hazırlayan kişi
   bildirdi, burada bir dosyada kayıtlı değil). Platform arşivi ile genel arşiv
   bayt bayt aynıdır: 66.182.207 bayt, SHA-256
   `d7b33548d431c05f99e2d98d3052692647a3c662c9cb57d60fb03bf5e5a91d60`. İki
   `.sha256` dosyası da doğrulanır. İmzalı manifest 332 bayttır; biçim v2, sıra
   87, sürüm v0.1.0-alpha.87, yukarıdaki `main` commit'i (`a8cddcb5…`),
   `published_at` 2026-10-16T01:23:21Z, linux/amd64 der; arşivin SHA-256'sı ve
   boyutu eşleşir. 64 baytlık Ed25519 imzası, izlenen
   `deploy/release-signing-ed25519.pem` ile `openssl pkeyutl -verify -rawin
   -pubin` kullanılarak doğrulanır (anahtarın SHA-256'sı
   `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`;
   alpha.85 ve alpha.86'nınkiyle aynı). Arşivde 237 üye var. İçindeki
   `libexec/get.sh` sıra 87'yi ve v0.1.0-alpha.87'yi sabitler (SHA-256
   `7c8823849cb2fb310b4a58d178364c9ed4d89c4d94908f0cf0856df3fb577b5d`);
   `release-sequence-policy` 87 der, önceki 86,
   `8d260f317e58bed7fcd84c6cbb01823c4fe3f166`; `release.commit` ve
   `release.tree` etiketinkilerle eşittir. İkili dosyalar go1.26.5, trimpath,
   CGO_ENABLED=0 ile derlenmiştir, kabul etiketi yoktur. İçerik bekçisi ve
   kabul-lisansı bekçisi ikisi de 0 döndürdü. Arayüz paketi
   `web/dist/assets/app-D8EjyWzW.js`'tir (248.119 bayt, SHA-256
   `b86955f5b8d4f1b3f0af15612c983b118037390e1cc01615cafa1ace4a184cb1`;
   alpha.86'nınki `app-DAGE4avW.js` idi). Sınırlar: etiketin dosyaları bir
   derleme konuğundaki klondan değil, Windows'ta `git archive` ile alındı ve
   arşiv alpha.86'nın paketiyle dosya dosya karşılaştırılmadı.
8. İndirme portalı paketi `deploy/build-download-portal.sh` ile imza öncesi
   kipte derlendi (özel anahtar kullanılmadı): 132.735.450 bayt, SHA-256
   `d7ec2b4940457d0b9bbe717d50506fd343ebed869b1f441348d670f35dbcf88c`, 54 üye,
   iki kez bayt bayt aynı derlendi. `releases/latest.txt` v0.1.0-alpha.87 der.
   Terfi aracının paket kuralları paketi kabul etti. Etikette portal sözleşme
   testi 0 döndürdü, genel doğrulayıcı testleri 17'de 17, terfi testleri 14'te
   14 geçti. Dört portal betiği alpha.86'dan beri değişmedi. Sınırlar: canlı
   sitenin bir kopyasında terfi aracı provası yapılmadı ve
   `deploy/test-download-portal-publisher.ps1` çalıştırılmadı.
9. Portalı sürümü hazırlayan kişi `deploy/publish-download-portal.ps1` ile,
   betiğin mutlak yoluyla ve açık `-Promoter` ile `-PublicVerifier` ile,
   `-PreviousVersion v0.1.0-alpha.86 -TargetVersion v0.1.0-alpha.87
   -TransferTimeoutSeconds 1800` ile yayımladı. Çıktı:
   `CELIKPANEL_PORTAL_UPLOAD_READY`, `CELIKPANEL_PORTAL_UPLOAD_PINNED`, terfi
   aracının sonucu `status: committed`, previous_version v0.1.0-alpha.86, yedek
   `httpdocs.before-v0.1.0-alpha.87.20261016T020434Z…`, genel doğrulama
   `status: ok` (26 istek), sonra `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`.
10. 16 Ekim 2026 02:04:42Z'de dışarıdan okuma (salt-okunur; sürümü hazırlayan
    kişi): `releases/latest.txt` v0.1.0-alpha.87'dir (yayımdan önce 02:02:54Z'de
    v0.1.0-alpha.86 okunuyordu); `get.sh`
    `Strict-Transport-Security: max-age=15768000; includeSubDomains` ile
    sunuluyor ve sıra 87'yi sabitliyor; `releases/latest.json` sıra 87'yi,
    `a8cddcb5…` commit'ini ve `d7b33548…` SHA-256'sını söyler. Bildirilmeyen:
    yayımlanan diğer dosyaların GitHub dosyalarıyla bayt karşılaştırması.
11. Kurulu sunucular, bu kayıt yazılırken: güncellendiği bildirilmedi. Bunu yalnız
    sahip, panelin kendi güncelleme ekranından başlatır. Sonuç ayrıca
    kaydedilecek.

**Yapılmayanlar ve yayımlamanın değiştirmedikleri.**

- Kurulu sunucuları sürümü hazırlayan kişi güncellemedi ve bu kayıt yazılırken
  ikisinin de güncellendiği bildirilmemişti. Her birinin güncellemesini sahibi
  panelin kendi güncelleme ekranından, yukarıdaki her adımdan ayrı bir adım
  olarak başlatır; başka kimse başlatmaz. Sahibin bildirmedikleri, ve bildirene
  kadar kaydedilmeyecek olanlar: iki sunucunun güncelleme sonucu (hangi sürümden
  olursa olsun); güncelleme sayfasının yeniden başlama sırasında gösterdiği;
  güncelleme sırasında tam ekran lisans sayfasının görünüp görünmediği; Agent'ın
  sürümü; Frankfurt'un güncellemeden sonraki kurulum son adımı; `postconf -n
  >/dev/null` çıktısı; ve Boston'un hazır ekranında "Bu sunucunun kurulumunu
  genişletin" seçeneğinin görünüp görünmediği. Bu kayıttaki hiçbir şey, kurulu bir
  sunucunun bu sürümle ya da alpha.86 ile ne yaptığını anlatmaz.
- alpha.87, alpha.86'dan yaklaşık 5,5 saat sonra ve sahip herhangi bir kurulu
  sunucuyu alpha.86'ya güncellediğini bildirmeden önce yayımlandı; bu yüzden
  sahibin alpha.86 gözlemleri (alpha.86 notlarında sayılan) de hâlâ açıktır.
  Güncelleyicinin sıra tabanı alpha.85'ten ya da alpha.86'dan doğrudan
  alpha.87'ye güncellemeye izin verir; sürüm sırası politikasının
  `previous_commit` değeri bir derleme sözleşmesi sabitlemesidir, kurulum önkoşulu
  değildir. Bu, `cmd/agent/system_update_contract.go` dosyasının okunmasından ve
  `cmd/` altında `previous_commit` denetiminin bulunmamasından çıkarılmıştır; kodun
  okunmasıdır, ölçülmüş bir güncelleme değildir. alpha.86'dan güncellemede
  alpha.86'nın sakin yeniden başlama bildirimi kurulu bir sunucuda ilk kez
  görülmelidir; alpha.85'ten güncellemede görünen güncelleme sayfası alpha.85'in
  kendi sayfasıdır.
- Yukarıdaki denetimlerin sınırları: dosyalar bir derleme konuğundaki klondan
  değil, Windows'ta `git archive` ile alındı ve arşiv alpha.86'nın paketiyle
  dosya dosya karşılaştırılmadı (7. madde); canlı sitenin bir kopyasında terfi
  aracı provası yapılmadı ve yayımlayıcının kendi testi çalıştırılmadı (8.
  madde).
- Yayımlanma ürün kabulü değildir. "Bu sürümün sınırları" altında sayılanlar
  ondan etkilenmez; bunların arasında ikinci bölümün sürümde olmaması ve hiçbir
  kurulu sunucunun ölçülmemiş olması var.
  [Dayanıklılık sözleşmesinin](RESILIENCE-CONTRACT.tr.md) beş temel işi kısmi
  kalır; P0.1–P0.5 kısmi kalır ve hiçbir P0 işi sürümle ya da yayımlanmasıyla
  kapanmaz ya da ilerlemez.

Bu sürümü yalnız CelikPanel'in güncelleme arayüzünden kurun. Yayımlanması kurulu
sunucuları güncellemez; bir sunucunun güncellemesini yalnız sahibi, panelin
kendi güncelleme ekranından başlatır.
