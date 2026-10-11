# v0.1.0-alpha.83 (taslak, yayımlanmadı)

[English](RELEASE-NOTES-v0.1.0-alpha.83.md)

Bu, v0.1.0-alpha.82'yi izleyen sürümün taslağıdır. Çalışma numarası olan
v0.1.0-alpha.83 sahibin seçimidir ve kesin değildir. Sürümün hiçbir parçası
yayımlanmadı; hiçbir kurulu sunucu onu çalıştırmıyor. Bu taslak 11 Ekim 2026'da
yazıldı.

Sürüm iki şey yapar. Birincisi, CelikPanel'in elle değiştirdiğiniz bir site
yapılandırma dosyasının üzerine yazmasını durdurur: dosya korunur, site bunu
söyler ve dosyaya ne olacağını siz seçersiniz. İkincisi, sahibin 10 Ekim 2026'da
v0.1.0-alpha.82'ye güncellemeden sonra kurulu iki sunucuda gördüğü dört şeye
yanıt verir (aşağıda "Son güncellemeden sonraki dört gözlem" altında
sıralıdır).

Sunucunun site dosyalarıyla ne yaptığı, bileşen testlerine ve bu çalışmanın ilk
bölümünün Debian 13, Ubuntu 24.04 ve Arch'ta yapılan tek bir laboratuvar
ölçümüne dayanır. O ölçümden sonra eklenen bölümlerin, bu yazılırken yalnız
bileşen testleri ve API'nin bir taklidine karşı çalışan bir tarayıcı denetimi
vardı. Bir ekranın ne gösterdiği bileşen testlerine ve API taklidine karşı
çalışan bir tarayıcıya dayanır; ilk sayfa yüklemesi, erişim beklemesi ve
güncelleme bildirimi için laboratuvarda gerçek bir Panel'e karşı gerçek bir
Chrome'a da. İki laboratuvar kaydı daha bu yazılırken bitmemişti; yerleri "Ne
ölçüldü" bölümünde işaretlidir. Hiç çalıştırılmayanları sınırlar bölümü söyler.

## Sunucu sahibi için ne değişiyor

**Güncellemeden sonra açık olan her CelikPanel sayfasını yeniden yükleyin.**
Güncellemeden önce yüklenmiş bir sayfada yeni Yapılandırma dosyası sayfası
yoktur; sayfa güncellemenin yeni aşamasını tanımaz ve eski beklemeleri çizer. Bu
sürümün yeni rotaları, istek kimliği taşımayan isteği `428` ile reddeder;
reddedilen istek hiçbir şeyi değiştirmez.

**Güncelleme sırasında hâlâ önceki sürümün sayfasını görürsünüz.** Güncelleme
sürerken açık olan sayfa, v0.1.0-alpha.82'nin arayüzüdür. Panel yeniden
başlarken o arayüzün sayfa üzerindeki katmanı lisanstan söz edebilir
(laboratuvarda o arayüzle ölçüldü); köşedeki bildirimi de kart yeni sürümü
gösterirken güncellemenin hâlâ uygulandığını söyleyebilir. İkisini de sahip son
güncellemeden sonra bildirmişti. Bu sürüm ikisini de düzeltir; düzeltmeler, bu
sürümün bir sayfasından başlatılan güncellemede görünür. Başarılı bir
güncellemeden sonra sayfa kendini yeni arayüzle yeniden yükler.

**Değiştirdiğiniz site yapılandırma dosyası korunur.** Şimdiye kadar her Panel
başlangıcı, yeniden başlatma ve kaydedilen her ayar, barındırılan her sitenin
nginx dosyasını, orada ne olduğuna bakmadan CelikPanel'in kayıtlarından yeniden
yazıyordu: eklediğiniz bir `location`, değiştirdiğiniz bir yönerge, kendi
include satırınız ya da baştan değiştirdiğiniz bir dosya gidiyor, sildiğiniz bir
dosya geri geliyordu. Bu, v0.1.0-alpha.82'de Debian 13, Ubuntu 24.04 ve Arch'ta
ölçüldü. Artık CelikPanel'in bir site için yazdığı her dosya, dosyanın geri
kalanının özetini taşıyan bir satırla başlar:

```
# celikpanel-render v2 sha256=<64 onaltılık karakter>
```

CelikPanel yazmadan önce dosyayı bu satırla karşılaştırır. Yalnız kendi
değişmemiş metnini, o da yeni metin farklıysa, değiştirir; aynı içerikli dosya
yazılmaz ve onun için nginx yeniden yüklenmez. Değiştirilmiş, başka bir dosyayla
değiştirilmiş, silinmiş, kilitlenmiş ya da okunamayan bir dosya olduğu gibi
bırakılır; onu değiştirmek isteyen işlem de başarı bildirmek yerine bunu
söyler. CelikPanel'in kendi metni dosyanın yanında, nginx'in okumadığı
`<dosya>.celikpanel-pending` olarak bekletilir.

**Alan adının sayfası ne bulduğunu söyler, seçimi siz yaparsınız.** Alan adı →
Barındırma → Yapılandırma dosyası (yalnız yöneticiler). Alan Adları listesi o
alan adını işaretler ("Yapılandırma dosyası: seçiminiz gerekiyor", "Yapılandırma
dosyası eksik", "Yapılandırma dosyası okunamıyor", "Yapılandırma dosyası
değiştirilemiyor"); alan adının sekmelerinin üstündeki bir satır da sayfaya
yönlendirir. Sayfa, kimlik bilgisine benzeyen değerler gizlenmiş olarak,
dosyanızla CelikPanel'in metni arasındaki farkı gösterir ve üç seçenek sunar:

- *Benimkini koru.* Dosyanız ilk satırıyla birlikte bayt bayt olduğu gibi kalır.
  CelikPanel seçiminizi kaydeder ve yalnız dosya değişirse yeniden sorar. Sitede
  CelikPanel'den yaptığınız değişiklikler (ayarlar, sertifikalar, barındırma
  türü, PHP sürümü) o dosyaya uygulanmaz.
- *CelikPanel'in metnini al.* Dosyanız önce yanına tarihli bir kopya olarak
  bırakılır (`<dosya>.celikpanel-backup-<tarih ve saat>`), sonra yerine
  CelikPanel'in metni yazılır ve nginx yeniden yüklenir. nginx metni reddederse
  dosyanız geri konur; tarihli kopya yerinde kalır.
- *Elle birleştir.* Panel hiçbir şey yapmaz. CelikPanel'in metni
  `.celikpanel-pending` dosyasındadır; dosyanızı sunucuda kendiniz düzenler,
  nginx'i kendiniz kontrol edip yeniden yükler ve sayfayı yeniden yüklersiniz.
  Bekleyen dosyayı kendi dosyanızın üzerine kopyalamak, dosyayı CelikPanel'e
  geri bırakır.

Sildiğiniz bir dosya ne bir başlangıçta ne de kaydedilen bir ayarla yeniden
oluşturulur; sayfa "Yeniden oluştur" seçeneğini sunar. Değişikliğe karşı
kilitlenmiş bir dosya (örneğin `chattr +i` ile) böyle adlandırılır; sistem
hatası, Panel'in bulduğu koruma ve CelikPanel'in yazmak istediği metin de
gösterilir. CelikPanel'in onu güncelleyebilmesi için korumayı kaldırın (örneğin
`sudo chattr -i <yol>`) ve "Yeniden kontrol et"yi seçin; sayfa o zaman
"CelikPanel'in metnini şimdi yaz" seçeneğini sunar, bir sonraki başlangıç ya da
kaydedilen değişiklik de onu yazar. Kilitli kalsın istiyorsanız bir şey
yapmayın: CelikPanel onu değiştirmez.

**Bu sürümün ilk başlangıcından sonra ne görürsünüz.** v0.1.0-alpha.81 ya da
alpha.82'nin yazdığı dosyalarda böyle bir ilk satır yoktur. İlk başlangıçta
CelikPanel bunların her birini, o iki sürümün aynı site için yazdığı metinlerle
(site oluşturulurken yazılan metin ve sonraki bir başlangıçta ya da kayıtta
yazılan metin) bayt bayt karşılaştırır. Bunlardan birine eşit olan dosya
CelikPanel'in kendi eski metnidir, yani içinde sizden bir şey olamaz: bu sürümün
metni olarak, ilk satırıyla yeniden yazılır. Başka her dosya olduğu gibi
bırakılır, "CelikPanel'inki olarak tanınmadı" diye gösterilir ve hiç yazılmaz.
Yönetici olarak Alan Adları listesinde şunu görürsünüz:

> Site yapılandırma dosyaları: N tanesi CelikPanel’inki olarak tanınıp
> devralındı; M tanesi bilinen her CelikPanel metninden farklı olduğu için olduğu
> gibi bırakıldı. Dosyasını görmek için bir alan adını açın. Olduğu gibi
> bırakılan bir dosyayı yeniden CelikPanel’in yönetmesini istemiyorsanız bir şey
> yapmanız gerekmez.

Olduğu gibi bırakılan dosya, sizin düzenlediğiniz bir dosya olmak zorunda
değildir. Önceki sürüm dosyasını son yazdıktan sonra sertifika yolu, takma
adları, PHP sürümü ya da adları değişen bir sitenin dosyası da bilinen her
metinden farklı olabilir; bu, güvenli yönde bir yanılgıdır ve "CelikPanel'in
metnini al" onu çözer. Olduğu gibi bırakılan her dosya için "Benimkini koru"yu
seçtiğinizde ya da CelikPanel'in metnini aldığınızda satır kaybolur.

**Dosyası korunan sitenin sertifikaları.** CelikPanel sertifika doğrulamasını
artık kendi metninin içerdiği, kendisine ait bir dizinden yayımlar:
`/etc/nginx/celikpanel-managed.d/<alan-adı>/`. Doğrulamayı artık sitenin
dosyasına yazmaz. Korunan bir dosya için sertifika istemeden ya da yenilemeden
önce CelikPanel, bu sunucudaki nginx'in doğrulamasını sertifikanın her adı
altında sunup sunmadığını denetler (sunucunun kendisinden kendi 80 numaralı
bağlantı noktasına yapılan bir istekle). Sunuyorsa sertifika, dosyanıza
dokunulmadan alınır; ama siz CelikPanel'in metnini alana ya da iki sertifika
satırını kendiniz değiştirene kadar site, dosyanızın adını verdiği sertifikayı
sunmayı sürdürür; SSL sekmesi ve pano bunu, kullanılan sertifikanın kalan
günleriyle birlikte söyler. Sunmuyorsa hiçbir şey istenmez: SSL sekmesinden
yapılan istek eklenecek satırla birlikte reddedilir, yenileme de başarısız olmak
yerine sizi bekler. nginx bu denetime yanıt vermezse sonuç "bilinmiyor"dur,
asla "hazır değil" değildir. v0.1.0-alpha.81 ya da alpha.82'den kalan korunmuş
bir dosyada yeni dizini içeren satır yoktur; doğrulamanın yine de çalışıp
çalışmadığına bu denetim karar verir. Bunların hiçbiri, bu yazılırken gerçek bir
nginx'e ya da bir sertifika otoritesine karşı çalıştırılmamıştı (sınırlara
bakın). Panoyu izleyin: seçiminizi bekleyen sertifika en başta listelenir.

**PHP-FPM havuzu ve uygulama birimi de aynı kurala uyar.** Bir PHP sitesinin
havuz dosyası ve bir Node sitesinin uygulama birimi aynı türden bir ilk satır
alır (havuzda `; celikpanel-render v2 sha256=…`, birimde `# celikpanel-render v2
sha256=…`). Düzenlediğiniz bir dosyada havuz kaydı, PHP sürümü değişikliği ya da
barındırma değişikliği reddedilir; dosyanız hizmette kalır. v0.1.0-alpha.82,
havuz Panel'den kaydedildiğinde kendi yazmadığı havuz satırlarını siliyordu
(ölçüldü); artık sizin satırlarınızı taşıyan havuz korunur ve gösterilir.
PHP-FPM'de bir havuzun kendi ayarları için ayrı bir yer yoktur; bu yüzden
satırlarınızın yeri havuz dosyasının kendisidir. Bir uygulamanın kendi
ayarlarının yeri, systemd'nin ek ayar dizinidir:
`/etc/systemd/system/celikapp-<N>.service.d/*.conf`; ardından
`sudo systemctl daemon-reload` çalıştırılır. CelikPanel o dizine hiç yazmaz ve
onu hiç silmez. alpha.81 ya da alpha.82'nin yazdığıyla bayt bayt aynı olan havuz
ya da birim tanınır ve bir sonraki yazımda bu sürümün metni olarak yazılır. Bu
dosyalara Panel başlarken bakılmaz; bir havuz ya da birim, Alan Adları
listesinde ancak alan adının Yapılandırma dosyası sayfası açıldıktan ya da o
dosya kaydedildikten, sürümü değiştirildikten veya birimi değiştirildikten sonra
görünür. Bu bölümün yalnız bileşen testleri var.

**Kendi nginx satırlarınızın, CelikPanel'in hiç dokunmadığı bir yeri var.** Her
sitenin dosyası `/etc/nginx/celikpanel-sites.d/<alan-adı>/*.conf` yolunu
içerir. O site için kendi yönergelerinizi oradaki bir `.conf` dosyasına yazın.
CelikPanel o dizinde hiçbir şeyi yazmaz, değiştirmez ya da silmez; site
silinirken de. Bu include satırı sitenin `location` bloklarından sonra gelir;
bu yüzden orada bunlardan birini yineleyen bir dosya (örneğin `location /`),
`nginx -t` komutunun bütün yapılandırmayı reddetmesine yol açar (nginx'in kendi
kuralı; CelikPanel ile ölçülmedi). Öteki dizin,
`/etc/nginx/celikpanel-managed.d/<alan-adı>/`, CelikPanel'indir: bir sitenin
dosyasını düzenlerseniz onu içeren satırı silmeyin.

**Başlangıçlar daha sessiz.** Bütün site dosyalarını değişmemiş bulan bir Panel
başlangıcı hiçbir şey yazmaz ve nginx'i yeniden yüklemez. Yeni bir sitenin
dosyası artık sonraki bir başlangıcın yazdığı metnin aynısıdır (`www.` adı
oluşturulduğu andan itibaren oradadır). Her yazım, siteyi ve yapılanı adıyla
veren bir satır günlüğe yazar; başlangıç satırı da yazılan, değişmeyen,
korunan, eksik, okunamayan ve başarısız dosyaları ayrı ayrı sayar. Bunları
`sudo journalctl -u celikpanel-panel | grep "site configuration"` ile
okuyabilirsiniz.

**Kilitli bir dosya artık öteki siteleri durdurmuyor.** v0.1.0-alpha.82'de
`chattr +i` ile kilitlenmiş bir site dosyası bütün başlangıcı başarısız
kılıyordu; sitenin etkinleştirme bağlantısı ardından yok oluyor, bir sonraki
nginx yeniden yüklemesi de o siteyi hizmetten düşürüyordu ve bunların hiçbiri
gösterilmiyordu (ölçüldü). Artık yalnız o dosya reddedilir; öteki siteler tek
bir denetim ve tek bir yeniden yüklemeyle yazılır, bağlantı da yerinde kalır
(bu çalışmanın ilk bölümünde ölçüldü).

**İlk sayfa yüklemesi, söyleyecek bir şey olana kadar hiçbir şey çizmiyor.** Bir
CelikPanel sayfasının tam yüklemesi, henüz hiçbir şey yanıt vermeden kısa bir an
"Reload CelikPanel" düğmeli tam sayfa bir "Checking panel access" ekranı
gösteriyordu. Artık sayfa ilk 1,5 saniye boş kalır. Erişim bundan sonra hâlâ
denetleniyorsa sayfa, Panel'in henüz yanıt vermediğini ve bir şey yapmanız
gerekmediğini söyler; 15 saniyeden sonra "Şimdi kontrol et"i, 30 saniyeden
sonra "CelikPanel'i yeniden yükle"yi de sunar. Erişimi doğrulamayan bir yanıt
(oturum yok, bir hata durumu, Panel başlıyor) ya da başarısız olan bir bağlantı
nedeniyle birlikte hemen gösterilir. Kendi süre sınırına ulaşan bir denetim
hâlâ bilinmiyor olarak gösterilir, asla başarısızlık olarak değil.

**Sayfa üzerindeki katman, okuduğu nedeni söylüyor.** Panel erişimi kısa bir
süre doğrulayamadığında sayfa, v0.1.0-alpha.82'deki gibi bir katmanın altında
kalır. Katman artık hangisini okuduğunu söyler: Panel yanıt vermedi; Panel
başlıyor; ya da lisans sonucu okunamadı. Aynı tarayıcıdan son 30 dakika içinde
bir güncelleme başlatıldıysa ve bitişi orada henüz görülmediyse, Panel'in yanıt
vermediğini ve bir güncellemenin onu yeniden başlatıyor olabileceğini, lisans
hakkında hiçbir karar verilmediğini söyler.

**Güncelleme bildirimi ve kart birbirini tutuyor.** Yeni sürüm çalışıyor ama
güncelleyici onu hâlâ denetliyorken bildirim de kart da "Güncelleme kuruldu,
doğrulanıyor" der ve bir şey yapmanız gerekmediğini söyler. Okunamayan durum
"bilinmiyor" diye söylenir ve "başka bir güncelleme başlatmayın" uyarısı eklenir.
Güncelleme penceresindeki saat artık "UTC" diye etiketlenmez; yerel saatiniz,
saat dilimiyle birlikte gösterilir.

**Duran kurulum, nedenini adımların üstünde söylüyor.** Planı düzenleme için
yeniden açıldığı için duran bir kurulum çalışması, son adımı "Sürüyor" olan bir
başarısızlık gibi gösterilmez. Neden, kimin işlem yapacağı ve sonraki adım,
adım listesinin üstünde ve açık olarak durur. Posta kimliği denetimi,
karşılanmayan koşulu adıyla söyler: sunucunun adresi genel değil, posta hizmeti
başka bir ad kullanıyor, adresin ters DNS (PTR) adı başka bir şey gösteriyor, o
ad adrese geri götürmüyor ya da posta ana makine adı tam bir ana makine adı
değil. Yanıt alamayan bir ters DNS sorgusu artık "ters DNS yok" değil, "kontrol
edilemedi" olarak gösterilir. Kurulumun sırası ve gereksinimleri değişmedi.

### Son güncellemeden sonraki dört gözlem

Bunları sahip, kurulu iki sunucuyu (biri Ubuntu 24.04, biri Debian 13) 10 Ekim
2026'da panelin kendi güncelleme ekranından v0.1.0-alpha.82'ye güncelledikten
sonra kendi ekranlarından bildirdi. O sunuculara hiçbir asistan bakmadı.

1. *Panel yeniden başlarken tam ekran "License status could not be checked"
   sayfası (Ubuntu sunucusu).* O güncelleme, erişim bilinmediğinde sayfanın
   yerine başka bir sayfa koyan v0.1.0-alpha.81 arayüzünden başlatılmıştı;
   laboratuvardaki bir ölçüm bunu o arayüzle yeniden üretti ve alpha.82
   arayüzünün sayfayı bir katmanın altında tuttuğunu gördü. alpha.81'in
   arayüzünü hiçbir şey değiştiremez. Bu sürüm katmanın söylediğini değiştirir
   (yukarıda). Değişikliğin önceki ifadesi laboratuvarda gerçek bir Panel'de
   görüldü; son ifadesinin yalnız bileşen testleri ve taklit API'ye karşı
   tarayıcı denetimi var.
2. *Kart yeni sürümü gösterirken köşedeki bildirim güncellemenin hâlâ
   uygulandığını söylüyordu (Debian sunucusu, başlangıçtan yaklaşık iki dakika
   sonra).* Değişti: o aralıkta ikisi de artık "kuruldu, doğrulanıyor" der.
   Laboratuvarda gerçek bir Panel'de (Debian 13), bu çalışmanın aynı kodunun iki
   derlemesi arasındaki bir güncellemede görüldü; değerlendirilmiş bir denetim
   olarak değil, gözlem olarak. Bu sürümün bir sayfasından başlatılan
   güncellemede geçerlidir (yukarıya bakın).
3. *`/setup` sayfasının soğuk yüklemesi, kısa bir an tam sayfa "Checking panel
   access" gösteriyordu.* Değişti: ilk 1,5 saniye boyunca hiçbir şey çizilmez.
   Laboratuvarda gerçek bir Panel'de (Debian 13) ölçüldü: 18 yüklemenin 18'i
   geçti. Ardından gelen 15 ve 30 saniyelik adımlar o ölçümden sonra
   değiştirildi; bileşen testleri ve taklit API'ye karşı tarayıcı denetimi var,
   onları bir sonraki laboratuvar kaydı ölçer.
4. *Kurulum sayfası "This operation stopped" derken son adımı "In progress"
   gösteriyordu, neden de kapalı bölümlerin altındaydı (Debian sunucusu).*
   Değişti: planı yeniden açıldığı için duran çalışma bunu söyler, neden de
   adımların üstünde durur. Yalnız bileşen testleri ve taklit API'ye karşı
   tarayıcı denetimi; sahibin kendi kurulum kaydı okunmadı.

### Önceki bir sürüme dönüş

Bu sürüm Panel'in veritabanını 43. şemadan 44.'ye taşır: 044 numaralı geçiş tek
bir tablo, `managed_site_files`, ekler ve var olan hiçbir tabloyu değiştirmez.
Geri adım yoktur. v0.1.0-alpha.82, 44'teki bir veritabanını reddeder; bu yüzden
otomatik olsun olmasın bir dönüş, her geçişte olduğu gibi, güncellemeden önce
alınmış veritabanı anlık görüntüsünü geri yükler.

Dönüşte bu koruma kaybolur. Önceki sürüm yeni ilk satırı tanımaz: başlangıcında
her sitenin nginx dosyasını kendi kayıtlarından yeniden yazar; böylece yaptığınız
değişiklik, ilk satır ve kendi dizininizi içeren satır dosyadan gider.
`celikpanel-sites.d` içindeki dosyalarınız, tarihli kopyalar ve bekleyen
dosyalar diskte kalır; ama bu sürüm yeniden çalışana kadar nginx dizininizi
okumayı bırakır. Laboratuvarda v0.1.0-alpha.81, otomatik bir dönüşten sonraki
başlangıcında bir sahip düzenlemesi de dahil her site dosyasını yeniden yazdı;
o dosyalarda henüz ilk satır yoktu. Önceki bir sürümün ilk satırı zaten taşıyan
bir dosyaya ne yaptığı onun kaynağından okundu; bunun laboratuvar ölçümü "Ne
ölçüldü" bölümünde anılır. Bu sürüm yeniden çalıştığında önceki sürümün metnini
tanır ve devralır; önceki sürüm çalışırken ve onun bir sonraki başlangıcından
önce yaptığınız düzenleme korunur.

### Yapmanız gerekebilecekler

- Güncellemeden sonra açık CelikPanel sayfalarını yeniden yükleyin.
- API'yi çağıran betikleriniz varsa güncellemeden önce "API değişiklikleri"
  bölümünü okuyun: var olan birkaç rota, dosyası korunan bir site için artık
  `409` yanıtlayabilir; sertifika isteği de `waiting_for_owner` ile `200`
  yanıtlayabilir.
- İlk başlangıçtan sonra Alan Adları listesini yönetici olarak açın. Olduğu gibi
  bırakılan dosyalarla ilgili satırı görüyorsanız işaretli her alan adının
  Yapılandırma dosyası sayfasını açıp seçiminizi yapın. Bu zorunlu değildir:
  olduğu gibi bırakılan dosya sitesine hizmet vermeyi sürdürür; ama
  CelikPanel'den yaptığınız değişiklikler ona uygulanmaz ve o sitenin bir
  sertifikası seçiminizi bekleyebilir.
- Bir siteye ait kendi nginx satırlarınızı
  `/etc/nginx/celikpanel-sites.d/<alan-adı>/` içine koyun. Düzenlediğiniz bir
  site dosyasında `/etc/nginx/celikpanel-managed.d/<alan-adı>/*.conf` yolunu
  içeren satırı silmeyin.
- Bir uygulamanın kendi ayarlarını
  `/etc/systemd/system/celikapp-<N>.service.d/` içine koyun, ardından
  `sudo systemctl daemon-reload` çalıştırın.
- v0.1.0-alpha.82'nin Postfix önerisi geçerliliğini korur: posta kurulu bir
  sunucuda `postconf -n >/dev/null` çalıştırın; bir uyarı yazdırıyorsa `main.cf`
  içinde adını verdiği satırı düzeltin.
- Güncellemeden önce bilin: önceki bir sürüme dönüş, otomatik olsun olmasın, bu
  korumayı kaldırır (yukarıda).

## API değişiklikleri

Panel ve Agent tek bir sürümle birlikte kurulur. Havuz ve birim için yeni
çağrılar Panel'in kendi derlemesine bağlıdır: başka bir derlemeden bir Agent
"panel/agent build mismatch" yanıtlar ve hiçbir şey yazmaz; eski havuz
çağrılarına geri dönülmez. nginx dosyaları için derleme eşleşmesi, başka bir
sürümün Agent'ının Panel'in yazımlarını yanıtlamasını engeller; başlangıç da
toplu işini reddeder. Dosya başına sonuç vermeden yanıtlayan bir Agent
"bilinmiyor" (`unknown`, `agent_does_not_report`) olarak kaydedilir ve
gösterilir, asla değişmemiş olarak değil.

Hatalar `{"error", "code", "reason"?, "detail"?, "vars"?, "details"?}`
biçimini korur ve ek olarak `restore` alanını kazanır. Ekranlar `code` üzerinden
çevirir.

### Site yapılandırma dosyaları (yeni rotalar)

Hepsi yönetici içindir (başka durumda `403 {"error":"administrator access is
required"}`); barındırılan sitesi olmayan alan adı `404` yanıtlar.

| Rota | Gövde |
| --- | --- |
| `GET /api/v1/domains/{id}/site-config` | yok |
| `POST /api/v1/domains/{id}/site-config/keep` | `{"file_sha256"}` |
| `POST /api/v1/domains/{id}/site-config/take` | `{"file_sha256", "render_sha256"}` |
| `POST /api/v1/domains/{id}/site-config/recreate` | `{}` ya da yok |
| `POST /api/v1/domains/{id}/site-config/apply` | `{"file_sha256", "render_sha256"}` |

Her POST gövdesi `"kind"` taşıyabilir: `nginx_vhost` (varsayılan), `php_pool`
ya da `app_unit`; başka bir değer `400` ("kind is nginx_vhost, php_pool or
app_unit") olur; sitede olmayan bir tür `409 SITE_CONFIG_NOT_APPLICABLE` olur.
Özetler, GET'in döndürdükleridir. Dört POST rotası v0.1.0-alpha.82'nin
`X-CelikPanel-Request-Id` başlığını aynı yanıtlarla ister (başlıksız `428`, bozuk
başlıkla `400`, yinelenen istek saklanan yanıtla ve
`X-CelikPanel-Request-Replayed: 1` ile, `409 REQUEST_ID_REUSED`,
`409 REQUEST_IN_PROGRESS`, `409 REQUEST_OUTCOME_UNKNOWN`). Bunlarla birlikte
başlığı isteyen rota sayısı on ikidir.

**GET yanıtı.** Üst düzey alanlar, tek dosya okuyan bir istemcinin beklediği
gibi nginx dosyasını anlatır: `domain_id`, `domain`, `kind`, `path`, `state`,
`reason`, `detail`, `adopted_from`, `include_dir`, `enabled` (`link`, `absent`
ya da `other`), `file_sha256`, `render_sha256`, `pending_path`, `diff`,
`diff_truncated`, `actions`, `decision` (`kind`, `decided_at`, `current`) ve
`ledger` (`written_release`, `written_at`, `observed_at`, `backup_path`).
Yanlarına eklenenler: `files[]` (önce nginx dosyası, sonra bir PHP sitesinin
havuzu, sonra bir Node sitesinin birimi; her biri kendi `kind` ve `actions`
alanlarıyla aynı biçimde), `overall_state` ve `overall_kind`.

- `state`: `absent`, `managed_unchanged`, `owner_edited`, `foreign`,
  `unreadable`, `unwritable`, `unknown_origin` ya da `unknown` (`reason`
  `agent_does_not_report`; ya da `files[]` içinde Agent'ın şu an yanıt vermediği
  bir dosya için `not_read`).
- `unreadable` için `reason`: `symlink`, `not_regular`, `permission`,
  `too_large`, `read_failed`; `unwritable` için `write_refused`. Her yanıtta ek
  olarak, Agent'ın yalnız okuyarak ölçtüğü `system_error` (örneğin `EPERM`,
  `EROFS`) ve `protection` (`immutable`, `append_only`, `directory_immutable`,
  `read_only_filesystem`, `none`; ölçülmediğinde yok). Sahibin değiştirdiği bir
  dosya `unwritable` olduğunda `file_state`, o dosyanın kendi durumunu verir.
- `actions` (her zaman bir dizi): `owner_edited`, `foreign`, `unknown_origin`
  için `["keep","take","merge"]`; `absent` için `["recreate"]`; `unreadable` ve
  `unwritable` için `["recheck"]`; `file_state` taşıyan `unwritable` için
  `["keep","take","merge","recheck"]`; CelikPanel'in metni bekletilirken
  `reason` değeri `protection_removed` olan `managed_unchanged` için
  `["apply"]`; diğer durumlarda `[]`. `merge` ve `recheck` için rota yoktur:
  birleştirme sunucuda yapılır, yeniden denetleme GET'in yinelenmesidir.
- `diff`: diskteki dosyanın CelikPanel'in metnine göre birleşik farkı (ilk satır
  dışarıda), taraf başına en çok 4000 satır ve 64 KiB. authorization, password,
  passwd, secret, token, api key, private key ya da cookie geçen bir satır
  yönergesini korur, değeri gizlenmiş gösterilir; böyle bir sözcük geçen yorum
  satırının tamamı değiştirilir. Bu sözcükler olmadan yazılmış bir kimlik
  bilgisi gizlenmez.
- Sertifika alanları (nginx dosyası): `managed_dir`, `managed_include` (bir
  dosyanın gereksindiği satırın tam hâli), `challenge_file` (`absent`,
  `unchanged`, `differs`, `kept`; yazan bir işlemin yanıtında `written` ve
  `failed`), `validation` (`ready`, `include_missing`, `names_missing`,
  `challenge_kept`, `challenge_failed`, `unknown`; `validation_name` ve
  `validation_status` ile), `resolved_reason`, `pending_reason` (`certificate`,
  `certificate_validation`) ve `certificate` (`cert_path`, `key_path`,
  `expires_at`, `served_expires_at`, `served_days_left`, `referenced`); her biri
  yalnız bilindiğinde.
- `overall_state`: sitenin dosyalarının gösterilen en yüksek durumu; sıra
  okunamıyor, değiştirilemiyor, eksik, korunan (düzenlenmiş, değiştirilmiş,
  tanınmamış), bilinmiyor, CelikPanel'in değişmemiş metni; eşitlikte ilk dosya.
- Dosyanın kayıtlı nedeni `certificate_validation` iken GET yalnız okumaz:
  doğrulama denetimini yineler (doğrulama dizinine bir deneme dosyası yazar ve
  yeniden siler) ve CelikPanel'in kendi doğrulama dosyası yoksa ya da güncel
  değilse onu bir `nginx -t` ve bir yeniden yüklemeyle yazar; her adın sunulduğunu
  bulan denetim nedeni kapatır ve `resolved_reason: "certificate_validation"`
  yanıtlar.

**POST yanıtları.** `keep`: karardan sonraki GET nesnesiyle `200`; dosyaya
hiçbir şey yazılmaz. `take`: `outcome` `taken` (ya da `unchanged`) ve
`backup_path` ile `200`. `recreate`: `outcome` `recreated` (ya da `unchanged`)
ile `200`. `apply`: `outcome` `written` (ya da `unchanged`) ile, tarihli kopya
olmadan `200`. `take` ve `recreate` ayrıca `managed_dir`, `managed_include` ve
`challenge_file` taşır.

| Ret | Anlamı |
| --- | --- |
| `400` | bir özet eksik ya da 64 küçük harfli onaltılık karakter değil; bilinmeyen `kind` |
| `409 SITE_CONFIG_CHANGED` | dosya ya da CelikPanel'in metni GET'ten sonra değişti; hiçbir şey yapılmadı |
| `409 SITE_CONFIG_NOT_APPLICABLE` | seçenek dosyanın şimdiki hâli için sunulmuyor ya da sitede böyle bir dosya yok |
| `409 SITE_CONFIG_MISSING` | `take`: dosya yok |
| `409 SITE_CONFIG_UNWRITABLE` | `reason`: `symlink`, `not_regular`, `permission`, `too_large`, `read_failed` ya da `write_refused`; Agent şu an bir koruma ölçüyorsa da, hiçbir şey yazılmadan (tarihli kopya yapılmaz) |
| `502 SITE_CONFIG_NGINX_REFUSED` | `reason`: `nginx_refused` ya da `reload_failed`; `restore`: `confirmed` ya da `unconfirmed`; nginx'in satırı `details` içinde |
| `502 SITE_CONFIG_SERVICE_REFUSED` | havuz ya da birim: `detail` `config_refused` (hizmetin kendi denetimi) ya da `reload_failed`; `restore` yukarıdaki gibi |
| `502 SITE_CONFIG_NOT_READ` | Agent yanıt vermedi, hatayla yanıtladı ya da girdi hazırlanamadı |

`restore` yalnız Agent önceki dosyanın geri konduğunu ve ardından yapılan
yeniden yüklemenin tuttuğunu bildirdiğinde `confirmed` olur; başka durumda
`unconfirmed` olur ve cümle, dosyayı sunucuda sunucu yöneticisinin denetleyeceğini
söyler. Agent'ın kendi satırı yalnız yöneticilere gönderilir.

### Korunan bir dosyayı artık değiştirmeyen mevcut rotalar

Bir site dosyası yazan her işlem önce onu sınıflandırır. Bunlar arasında: genel
ayarların kaydı (`POST /api/v1/domains/{id}/general`), takma ad rotaları
(`POST /api/v1/domains/{id}/aliases`, `DELETE /api/v1/domains/{id}/aliases/{alias}`),
sertifika isteği, barındırma değişikliği (`PUT /api/v1/domains/{id}/hosting`),
PHP sürümü değişikliği (`POST /api/v1/domains/{id}/php`), havuz kaydı
(`POST /api/v1/domains/{id}/php/pool`), site oluşturma
(`POST /api/v1/domains/create`) ve cPanel içe aktarımı. CelikPanel'in koruduğu
bir dosyada şunları yanıtlarlar:

- `409 SITE_CONFIG_OWNER_EDITED`: nginx dosyası için `reason` durumdur
  (`owner_edited`, `foreign`, `unknown_origin`); havuz ya da birim için
  `reason` türdür (`php_pool`, `app_unit`), `detail` ise durum. Hiçbir şey
  değişmedi; reddedilen bir PHP sürümü değişikliği hiçbir şeyi değiştirmedi
  (havuz, nginx dosyası ve sitenin PHP sürümü yerinde kalır), reddedilen bir
  barındırma değişikliği de önceki ayarları korur.
- `409 SITE_CONFIG_MISSING` (havuz ya da birim için `reason` türdür).
- `409 SITE_CONFIG_UNWRITABLE`: nginx dosyası için `reason` Agent'ın
  nedenidir; havuz ya da birim için `reason` tür, `detail` neden.
- `409 SITE_CONFIG_EXISTS`: yeni sitenin yolunda CelikPanel'in değişmemiş metni
  olmayan bir dosya varsa site oluşturma ve içe aktarım (dosya korunur, yalnız
  kayıtlar silinir); hedef sürümde site için zaten bir havuz dosyası olan PHP
  sürümü değişikliği (`reason` `php_pool`).
- Yukarıdaki tablodaki gibi, `restore` ile `502 SITE_CONFIG_SERVICE_REFUSED` ve
  `502 SITE_CONFIG_NGINX_REFUSED`.

### Sertifikalar

- Dosyası korunan bir sitede `POST /api/v1/domains/{id}/ssl/letsencrypt`:
  sertifika alınıp saklandı ama korunan dosya onu henüz sunmuyorsa
  `200 {"status":"waiting_for_owner","expires_at":…,"pending_reason":"certificate"}`
  (olağan başarı `{"status":"success","expires_at":…}` olarak kalır); hiçbir
  şey istenmeden önce, `code` `SITE_CONFIG_OWNER_EDITED`, `reason`
  `certificate_validation`, `detail` `include_missing`, `names_missing`,
  `challenge_kept` ya da `challenge_failed` ve bilindiklerinde `vars.include`
  (eklenecek satır), `vars.name` ve `vars.status` (sunulmayan ilk ad ve nginx'in
  onun için verdiği HTTP durumu) ile `409`; nginx denetime yanıt vermediyse
  `detail` `unknown` ile `503 CERTIFICATE_VALIDATION_UNKNOWN`. Eksik ya da
  okunamayan bir dosya, v0.1.0-alpha.82'nin türsüz `409` verdiği yerde
  `409 SITE_CONFIG_MISSING` ya da `SITE_CONFIG_UNWRITABLE` yanıtlar. Doğrulamanın
  yayımlanıp yayımlanamayacağını söylemeyen bir Agent `include_missing` olarak
  yanıtlanır.
- Takma ad rotaları aynı retleri verir; hazır bir korunan dosyada sertifikanın
  yeniden alınması `"success"` yerine
  `"status":"waiting_for_owner","pending_reason":"certificate"` yanıtlar.
- `GET /api/v1/domains/{id}/ssl`: her sertifika `waiting_for_owner` (mantıksal)
  taşır; `renewal_status` `waiting_for_owner` değerini kazanır; ek olarak
  `waiting_for_owner_reason`: `certificate` ya da `certificate_validation`.
- `GET /api/v1/dashboard`: `expiring_certs` içindeki bir girdi
  `waiting_for_owner: true`, `domain_id` ve `served_days_left` (kullanılan
  sertifikanın kalan günleri; süresi dolduğunda eksi) taşıyabilir. Böyle
  girdiler, kalan günleri ne olursa olsun önce gelir ve `served_days_left`
  değerine göre sıralanır.

### Alan adları listesi

`GET /api/v1/domains`, yalnız yönetici için: her alan adı `state` ile, ve
geçerli olduklarında `kind` (nginx dosyası için yok), `adopted_from`,
`kept_by_choice` ve `pending_reason` ile `site_config` taşıyabilir. Bu, taze bir
okuma değil, Panel'in kaydettiği son gözlemdir; kayıtlı satırı olmayan dosya
burada yer almaz. `missing` burada görünür (yukarıdaki GET aynı dosya için
`absent` yanıtlar); kayıtlı, reddedilmiş bir değiştirme de sayfadaki gibi
`unwritable` olarak yanıtlanır.

### Sunucu kurulumu

`GET /api/v1/setup` ve `GET /api/v1/setup/operation`: `action_required`
durumundaki `mail_identity` denetimi ek olarak `reason` ve `vars` taşır:
`server_address_not_public` (`ip`), `mail_name_differs` (`hostname`, `ip`,
`current`), `reverse_dns_mismatch` (`hostname`, `ip`, `ptr`),
`forward_dns_mismatch` (`hostname`, `ip`) ya da `mail_name_not_canonical`
(`name`); her değer yalnız bilindiğinde. Yanıt alamayan bir ters DNS sorgusu
denetimi `state: "unknown"`, `code: "mail_identity_unavailable"`,
`reason: "reverse_dns_unknown"` yapar; `vars.ip` ve `vars.error` (`timeout`,
`no_resolver`, `refused`, `other`) ile. Bu sürümden önce yazılmış bir kayıt
genel biçimini korur.

### Güncelleme durumu

`GET /api/v1/panel/update/status` ve `POST /api/v1/panel/update/abandon`, kayıt
çalışıyorken ve yanıt veren Panel hedefin sürümüne ve commit'ine sahipken ek
olarak `phase: "verifying"` taşır (arşiv özeti ve sürüm sırası karşılaştırılmaz).
`status` değerini hiçbir zaman değiştirmez.

Bu yanıtlardan hangilerinin gerçek bir sistemde üretildiği sonraki iki bölümde
ve sınırlarda yazılıdır.

## Fark edeceğiniz davranış değişiklikleri

- Bütün site dosyalarını değişmemiş bulan bir Panel başlangıcı hiçbir şey
  yazmaz ve nginx'i yeniden yüklemez.
- Yeni bir sitenin nginx dosyası, sonraki bir başlangıcın yazdığı metindir.
- Dosyası korunan bir sitede kaydedilen ayar, sertifika, barındırma değişikliği
  ya da PHP değişikliği reddedilir ve hiçbir şeyi değiştirmez.
- Silinen bir site dosyası yeniden oluşturulmaz; sayfa "Yeniden oluştur"
  seçeneğini sunar.
- Kilitli bir site dosyası tek başına reddedilir; öteki siteler yazılır.
- Yolunda CelikPanel'in olmayan bir dosya duran site oluşturulamaz; o dosya
  korunur.
- Bir site silinirken düzenlenmiş nginx dosyasının tarihli bir kopyası bırakılır
  ve sizin include dizininiz yerinde kalır; CelikPanel'in kendi dizini yalnız
  içinde CelikPanel'in değişmemiş doğrulama dosyasından başka bir şey yoksa
  silinir.
- Yeni ya da yeniden oluşturulan nginx dosyası `root:celikpanel 0644` olur;
  korunan ya da metni alınan dosya sizin izinlerinizi, sahibinizi ve grubunuzu
  korur.
- İki include dizini `0755` olarak oluşturulur; zaten var olan bir dizin kendi
  izinlerini korur.
- Korunan bir dosya için sertifika, ancak sunucunun kendi denetimi doğrulamanın
  sunulduğunu bulursa alınır; siz bir şey yapana kadar site, dosyasının adını
  verdiği sertifikayı sunar.
- Düzenlediğiniz bir havuzda havuz kaydı ya da PHP sürümü değişikliği
  reddedilir; havuz satırlarınız artık silinmez.
- Kurulum sayfası bir çalışmanın neden durduğunu adımların üstünde söyler; posta
  kimliği denetimi koşulunu adıyla verir.
- Güncelleme bildirimi ve kart aynı aralıkta "kuruldu, doğrulanıyor" der;
  güncelleme penceresi yerel saati saat dilimiyle gösterir.
- Soğuk sayfa yüklemesi 1,5 saniye boyunca hiçbir şey çizmez; "Şimdi kontrol
  et" 15 saniyede, "CelikPanel'i yeniden yükle" 30 saniyede gelir.
- Sayfa üzerindeki katman okuduğu nedeni söyler.
- SSL sekmesi ve pano, eksi gün sayısı yerine "süresi N gün önce doldu" der.

Gerçek hizmetlerde ölçülenler (sonraki bölüm): değişmeyen başlangıç, oluşturma
metni, reddedilen kayıt, silinen dosya, kilitli dosya (sayfası dışında),
silme, yazılan dosyaların sahibi ve izinleri ve ilk yüklemedeki sessiz süre.
Include dizinlerinin izinleri, sertifika yolu, havuz, birim, kurulum sayfası,
katmanın ve beklemenin son ifadesi ve SSL sekmesinin ifadesi için bileşen
testleri ve taklit API'ye karşı tarayıcı denetimi var; güncelleme bildiriminin
yeni sözleri laboratuvarda gözlem olarak görüldü (sonraki bölüm).

## Ne ölçüldü, nasıl ölçüldü

Tek bir dizüstü ana makinedeki geçici sanal makinelerde 10 Ekim 2026'da (UTC;
onuncusunun kabul denetimi 11 Ekim 2026'da yapıldı) alınmış dört kayıt; bir de
bu yazılırken bitmemiş iki kayıt. Ürün imzalı arşiv olarak değil, kabul testi
lisansıyla derlenmiş hâliyle çalıştı. Dizinler
`deploy/e2e/release-recovery/evidence/` altındadır. Her sonuç dosyası kendisini
henüz kabul edilmemiş kanıt olarak işaretler; değerlendirmesi sahibindir.

**Laboratuvarın saptadığı ve saptamadığı.** Sanal makineler ağdan kopuk
değildir: dışarıya doğru NAT yapan QEMU kullanıcı ağıyla çalışırlar ve
dağıtımların paketleri koşu sırasında kendi depolarından kurulur. Bir makinenin
neye erişebildiğini ya da erişemediğini burada hiçbir şey göstermez. Sürüm ve
lisans sunucusunun adı ile dört sertifika otoritesi adı, hiçbir ürün yolu
oluşmadan önce makinenin hosts dosyasında kendi loopback adresine sabitlendi ve
sonda geri okundu (sekizinci ve dokuzuncu kayıtların bütün laboratuvarlarında;
onuncunun 12 laboratuvarından 11'inde, durdurulan bir koşunun geri okuması yok).
Lisans adımını test derlemesinin kendisi yanıtlar; bu, derlemenin kendisi
hakkında söylediğidir, trafiğin gözlemi değildir. Hiçbir sürücü kurulu bir
sunucuya yöneltilmedi. Makinelerin trafiği yakalanmadı.

**v0.1.0-alpha.82'nin sahibin düzenlediği site dosyasıyla yaptığı (sekizinci
kayıt, `set8-20261010`; Debian 13, Ubuntu 24.04, Arch).** Yayımlanmış etiketin
kaynağı, test lisansıyla ve önceki sürümün etiketiyle derlendi; 48 koşu. nginx
dosyasındaki 40 koşunun hepsinde sahibin eklediği `location`, değiştirdiği
`index`, yerine koyduğu dosya ya da kendi include satırı, bir sonraki
başlangıçta, yeniden başlatmada ya da Genel ayarların kaydında Panel'in kendi
metniyle değiştirildi (36), ya da silinen dosya ve bağlantısı yeniden
oluşturulup yine hizmete girdi (4); okunan Panel API yanıtlarının hiçbiri bunu
söylemedi; bir kayıttan sonra hiçbir Panel ya da Agent günlük satırı siteyi
anmadı. Bir havuz düzenlemesi başlangıçtan sağ çıktı (4); ardından Panel
üzerinden yapılan havuz kaydı sahibin yönergesini ve yorumunu sildi (4). Bir site
dosyasında `chattr +i` ile (4) dosya korundu, başlangıç toplu işi başarısız
oldu, sitenin etkinleştirme bağlantısı ardından yoktu; laboratuvarın bir günlük
döndürmesinin yapacağı gibi çalıştırdığı bir sonraki nginx yeniden yüklemesi de
Panel siteyi hâlâ etkin gösterirken onu hizmetten düşürdü. Ayrıca ölçülenler:
oluşturmada yazılan metin bir sonraki başlangıcın metninden farklıydı (`www.`
ekleniyordu); her başlangıç değişmemiş dosyaları yeniden yazıyordu; Panel,
sahibin `root:root` dosyasının üzerine `root:celikpanel` yazıyordu (sahip ve grup
yalnız bir Debian koşusunda değiştirildi).

**Site dosyası korumasının ilk bölümü (onuncu kayıt, `set10-20261010`; nginx
1.26.3 ile Debian 13, nginx 1.24.0 ile Ubuntu 24.04, nginx 1.30.5 ile Arch).**
Bu çalışmanın ilk bölümünün kodu: ilk satırıyla nginx dosyası, dosya başına
kayıt, üç seçenek ve sizin include dizininiz. CelikPanel'in kendi include
dizini ve doğrulama denetimi, "değiştirilemiyor" yanıtı, "CelikPanel'in metnini
şimdi yaz", dosyalar arasındaki durum sırası, geri koyma bilgisi, havuz ve birim
henüz içinde değildi. Yeni kuruldu; ayrıca yayımlanmış v0.1.0-alpha.81'den
(etiketinin kaynağı, test lisansı) güncellemeyle ulaşıldı. Platform başına 31
olmak üzere 93 hücre: 84'ü geçti, 6'sı geçmedi (her platformda aynı iki
bulgu), 3'ü ölçülmedi.

- Üç platformda da geçenler: sahibin her düzenlemesi (eklenen location,
  değiştirilen yönerge, yerine konan dosya, kendi include satırı) başlangıçta,
  yeniden başlatmada ve Genel kayıtta bayt bayt ve aynı inode ile korundu;
  CelikPanel'in metni yanında bekletildi; günlük siteyi adıyla andı; sayfa
  durumu farkla ve üç seçenekle yanıtladı; Alan Adları satırı durumu taşıdı;
  kayıt `409 SITE_CONFIG_OWNER_EDITED` yanıtladı; nginx yeniden yüklenmedi;
  sahibin içeriği sunulmayı sürdürdü. *Koru*: istek kimliği olmadan `428`,
  sonra `200`; dosyaya ve ilk satırına dokunulmadı, yinelenen istek yeniden
  çalışmadan yanıtlandı, sonraki başlangıçta korundu. Sahibin `root:root 0640`
  dosyası izinlerini ve sahibini korudu. *Al*: sahibin baytları, izinleri ve
  sahibiyle tarihli kopya; sahibin izinleri ve sahibiyle CelikPanel'in metni;
  yeniden yükleme; site sunuldu; yinelenen istek yeniden çalışmadan
  yanıtlandı. Eskimiş özetle alma: `409 SITE_CONFIG_CHANGED`, hiçbir şey
  yapılmadı. nginx'in reddettiği alma: `502 SITE_CONFIG_NGINX_REFUSED`, sahibin
  dosyası geri kondu. Silinen dosya ve bağlantısı: yeniden oluşturulmadı, kayıt
  `409 SITE_CONFIG_MISSING` yanıtladı, "Yeniden oluştur" siteyi yine sundu.
  Sembolik bağlantı: reddedildi, izlenmedi. Silme: düzenlenmiş dosya tarihli
  kopya olarak bırakıldı, değişmemiş olan kaldırıldı, include dizinleri yerinde
  kaldı. Bütün dosyaların değişmediği başlangıç: yazma yok, yeniden yükleme yok.
  Oluşturma ve başlangıç aynı metni yazdı. Kilitli dosya: yalnız o reddedildi,
  bağlantısı yerinde kaldı, öteki siteler yazıldı, tek yeniden yükleme; kilit
  kaldırılınca sonraki başlangıç onu yazdı.
- Include dizinindeki bir sahip `.conf` dosyası sunuldu ve bir başlangıçtan ve
  bir kayıttan sağ çıktı; o hücredeki PHP sürümü değişikliği ölçülmedi (her
  makinede tek PHP sürümü var).
- v0.1.0-alpha.81'den güncelleme: geçiş çalıştı; dokunulmamış dosyalar
  devralındı ve ilk satırıyla bu sürümün metni olarak yeniden yazıldı; sahibin
  düzenlediği dosya bayt bayt ve ilk satırsız olarak olduğu gibi bırakıldı:
  Debian 13 ve Ubuntu 24.04'te "4 adopted, 1 left alone", Arch'ta "3 adopted,
  1 left alone". O günkü üç rota (keep, take, recreate) istek kimliği olmadan
  `428` yanıtladı. Geçişten önceki hâline geri konan veritabanı yeniden
  geçirildi ve başlangıç hiçbir şey yazmadı.
- Otomatik dönüş: geçişi başarısız olan bir aday v0.1.0-alpha.81'e döndü;
  veritabanı yeniden 42'deydi ve alpha.81'in başlangıcı, sahibin düzenlemesi
  dahil her site dosyasını yeniden yazdı. Bu hücrelerde aday hiç başlamadı; bu
  yüzden dönüş anında hiçbir dosya ilk satırı taşımıyordu.
- Her platformda geçmeyenler: sahibin include dizini belgelenen 0755 yerine
  `root:celikpanel 0750` olarak oluşturuldu; kilitli bir dosya için de günlük,
  kayıt ve Alan Adları satırı kilidi anarken sayfanın kendi okuması nedensiz ve
  seçeneksiz "CelikPanel'in metni, değişmemiş" yanıtladı. İkisi de sonradan
  kaynakta düzeltildi (bileşen testleri).
- Ölçülmeyenler: PHP sürümü değişikliği, sertifikası olan bir site, dönüşten
  sonra ikinci bir güncelleme, tarayıcıda ekranlar.

**Gerçek bir Chrome'da ilk sayfa yüklemesi, erişim beklemesi ve güncelleme
(dokuzuncu kayıt, `set9-20261010`; Debian 13).** Dizüstü ana makinedeki başsız
bir Chrome, her konuğun gerçek Panel'ine karşı; bu çalışmanın, ilk kapı
düzeltmelerinden sonraki, 15 ve 30 saniyelik adımlardan ve katmanın son
ifadesinden önceki arayüzü.

- `/setup`, `/` ve `/settings?section=updates` yollarının soğuk tam yüklemesi,
  düz ve 2 Mbit/sn ile 300 ms'ye kısılmış bağlantıda: 18 yüklemenin 18'inde
  geçti. Bir okuma yanıtlanmadan hiçbir cümle ve düğme görünmedi; ilk okuma ilk
  bağlanmadan 71–97 ms (düz) ve 0,92–0,98 sn (kısılmış) sonra, yani 1,5 saniye
  içinde yanıtlandı.
- Her oturum okuması 2,5 sn bekletildi: açıklamalı bekleme, sessiz yüzey
  başladıktan 1,57–1,62 sn sonra, İngilizce ve Türkçe göründü. Geçti.
- Her oturum okuması 35 sn bekletildi: geçmedi. Daha eski bir 15 saniyelik
  sınır 15,18 sn'de "Your session could not be checked" ile "Reload CelikPanel"
  gösterdi, yeniden okumada da eski cümle geri geldi. Yukarıda anlatılan 15 ve
  30 saniyelik adımlar bundan sonra yapıldı; bileşen testleri ve taklit API'ye
  karşı tarayıcı denetimi var.
- Ayarlar'dan başlatılan güncelleme: üç konuğun birinde katman, o günkü
  ifadesiyle ("The Panel is not answering during an update … This is not a
  license problem.") göründü; Ayarlar sayfası 29,3 sn'lik kesintinin en az
  14,1 ve en çok 18,1 sn'si boyunca onun altında bağlı ve etkisiz kaldı, sonra
  kaldığı yerden sürdü. Öteki iki konukta katman çizilmedi (sayfanın erişim
  kararının yeniden başlatma sırasında süresinin dolmadığı kabul edilir; bitiş
  zamanı orada kaydedilmedi). İfade sonradan değiştirildi; son ifadenin bileşen
  testleri ve taklit API'ye karşı tarayıcı denetimi var.
- Gözlem, değerlendirilmiş bir denetim değil: iki konukta, güncelleyici işini
  bitirirken köşedeki bildirim ve kart "Update installed, being verified" dedi;
  güncelleme penceresi yerel saati saat dilimiyle gösterdi. Aynı aralıkta
  kartın "Current version" satırı ve kenar çubuğu, ürünün kendi yeniden
  yüklemesine kadar önceki sürümü göstermeyi sürdürdü.
- Oturum yok, kapanan bağlantı gibi başarısız olan oturum okuması ve başlayan
  Panel hemen gösterildi (geçti). Panel durdurulduğunda Chrome kendi hata
  sayfasını gösterdi: kurtarma hizmet çalışanı, oturum açılmış tarayıcı
  bağlamlarında kayıtlı değildi; oysa yeni bir bağlamın oturumsuz yüklemelerini
  o denetliyordu. Nedeni saptanmadı; ürünün çevrimdışı sayfasına ulaşılmadı
  (ölçülmedi).

**İlk yükleme ve güncelleme için başlangıç ölçümü (yedinci kayıt,
`set7-20261010`; Debian 13).** v0.1.0-alpha.82 notlarında anlatılır.
Yayımlanmış kodda soğuk yükleme 18 yüklemenin 18'inde tam sayfa "Checking panel
access" çizdi; bir güncelleme sırasında alpha.81 arayüzü sayfanın yerine yaklaşık
78 sn lisans sayfasını koydu (sahibin gördüğü ekran); alpha.82 arayüzü ise 25 sn'lik
kesintinin yaklaşık 9–11 sn'si boyunca sayfayı bir katmanın altında bağlı tuttu,
katmanın ilk cümlesi lisanstan söz ediyordu.

**On birinci kayıt (`set11-20261011`).** Bu yazılırken sürüyordu. Onuncu
kayıttan sonra yapılan düzeltmeleri Debian 13, Ubuntu 24.04 ve Arch'ın yeni
kurulmuş konuklarında ölçecek biçimde kuruldu: iki include dizininin izinleri;
korunan bir dosyada, doğrulama denetimi gerçek bir nginx'e karşı olmak üzere
sertifika isteği (Arch'ta yalnız CelikPanel'in include satırı olmayan dosya),
tek bir adrese bağlı 80 numaralı bağlantı noktası bloğu dahil; kilitli dosyanın
yanıtı, "Yeniden kontrol et", bir Genel kayıt ve reddedilen bir alma; gerçek bir
Chrome'da erişim beklemesi ve kurtarma hizmet çalışanı; ve Debian 13'te,
başlangıç denetiminden geçemeyen bir adaydan otomatik dönüşten sonra
v0.1.0-alpha.81'in ilk satırı taşıyan site dosyalarıyla ne yaptığı. Sonucu
buraya girene kadar bunların her biri gerçek bir sistemde ölçülmemiş sayılır.

[NATIVE RE-CHECK PENDING — set11]

**On ikinci kayıt.** Bu yazılırken koşmamıştı. Bu çalışmanın ikinci bölümünü,
PHP-FPM havuzunu ve uygulama birimini ölçecek. Sonucu buraya girene kadar o
bölüm gerçek bir sistemde ölçülmemiş sayılır.

[NATIVE RE-CHECK PENDING — set12]

**Hangi denetim hangi koda dayanıyor.** Kodun sonraki her hâli önceki
düzeltmeleri içerir.

| Denetlenen | Üzerinde koştuğu kod |
| --- | --- |
| nginx dosyasının korunması, üç seçenek, v0.1.0-alpha.81'den güncellemeden sonra devralma, dönüş, geri konan veritabanı, kilitli dosyanın öteki sitelere etkisi | bu çalışmanın ilk bölümü (onuncu kayıt); son kodda yinelenmedi |
| CelikPanel'in kendi include dizini, doğrulama denetimi, korunan dosyada sertifikalar, kilitli dosyanın yanıtı ve "Yeniden kontrol et", "CelikPanel'in metnini şimdi yaz", dosyalar arasındaki durum sırası, geri koyma bilgisi, 0755 izni | bileşen testleri ve taklit API'ye karşı tarayıcı; gerçek bir sistemde yalnız on birinci kayıtla |
| PHP-FPM havuzu ve uygulama birimi | bileşen testleri ve taklit API'ye karşı tarayıcı; gerçek bir sistemde yalnız on ikinci kayıtla |
| Soğuk ilk yükleme ve 1,5 saniyedeki açıklamalı bekleme | ilk kapı düzeltmelerinden sonraki arayüz (dokuzuncu kayıt) |
| 15 ve 30 saniyelik adımlar, önceki bir yanıttan sonra bilinmiyor | bileşen testleri ve taklit API'ye karşı tarayıcı; gerçek bir sistemde yalnız on birinci kayıtla |
| Katmanın son ifadesi, kurulum sayfası, posta kimliği nedeni | bileşen testleri ve taklit API'ye karşı tarayıcı |
| "Kuruldu, doğrulanıyor" bildirimi | dokuzuncu kayıtta gözlem olarak görüldü; bileşen testleri |
| v0.1.0-alpha.82'nin sahip düzenlemesiyle yaptığı | yayımlanmış etiketin kaynağı (sekizinci kayıt) |

**Ekranlar.** Arayüz değişikliklerinin bileşen testleri var; ayrıca bir
Chrome'da, API'nin loopback üzerindeki bir taklidine karşı incelendiler (masaüstü
ve telefon, İngilizce ve Türkçe; Yapılandırma dosyası sayfası için açık ve koyu
tema). O tarayıcının arkasında gerçek bir Panel yoktu. Dokuzuncu kayıt ilk
yükleme, erişim beklemesi ve bir güncelleme için gerçek bir Chrome'u gerçek bir
Panel'in önüne koydu (masaüstü boyutu, başsız; Türkçe yalnız erişim beklemesi
için).

**Testler.** Kodun son hâlinin değişiklik kaydına göre: 1313 arayüz testi geçti;
Go paketleri, Agent paketi dışında geçti; Agent paketinde bu değişikliklerden
öncekiyle aynı 19 test düştü ve değişiklik kaydı bunların nedeni olarak
geliştirme makinesinin ortamını gösterir. Bu sayılar saklanan bir test
günlüğünden değil, değişiklik kaydından gelir.

## Bu sürümün sınırları

Bunlar bilinen sınırlardır. Gizli kusur değildir.

- **Son kod gerçek bir sistemde ölçülmedi.** Onuncu kayıt bu çalışmanın ilk
  bölümünü ölçtü. Ondan sonra eklenen her şeyin (yukarıdaki tabloya bakın)
  bileşen testleri ve taklit API'ye karşı tarayıcı denetimi var; gerçek bir
  sistemde yalnız on birinci ve on ikinci kayıtlarla ölçülür. Sürüm hazırlığı
  yapılmadı ve bu sürümün adını taşıyan hiçbir arşiv derlenmedi.
- **v0.1.0-alpha.82'den güncelleme ölçülmedi.** Her güncelleme hücresi
  v0.1.0-alpha.81'in kaynağından başladı. v0.1.0-alpha.82'nin yazdığı dosyaların
  tanınması, bu sürüme gömülü ve özetleriyle sabitlenmiş dondurulmuş metinlerine
  ve bileşen testlerine dayanır.
- **Sertifikası olan hiçbir site ölçülmedi ve hiçbir sertifika alınmadı.**
  Doğrulama denetimi, bu yazılırken hiç gerçek bir nginx'e sormamıştı.
- **Doğrulama denetimi.** Yalnız `127.0.0.1` üzerindeki nginx'e sorar; bu yüzden
  80 numaralı bağlantı noktası bloğu tek bir adrese bağlı bir siteye orada
  ulaşılmaz: loopback'te 80'de başka bir şey dinlemiyorsa yanıt "bilinmiyor"
  olur, genel adreste dinleyen başka bir blok da yanıtı onun yerine verip
  "sunulmuyor" gibi okunabilir (nginx'in kuralından okundu, ölçülmedi). Sayfanın
  okumasında, CelikPanel doğrulama dosyasını yayımlarken başarısız olan bir
  `nginx -t` ya da yeniden yükleme, neden başka bir dosya olabilecekken
  `challenge_failed` olarak bildirilir. Eksik ya da okunamayan bir dosya neden
  kaydetmez; bu yüzden onun durdurduğu bir yenileme, bir sonraki tamamlanan
  yenilemeye kadar bekler. Hazır bulunan bir denetim, nginx'in yolu sunucunun
  kendisinden sunduğunu gösterir; DNS'in ya da bir güvenlik duvarının sertifika
  otoritesini içeri aldığını göstermez. İlk istek hiç yeniden denenmez; yenileme
  yalnız zamanlamayla yeniden denenir (her 12 saatte bir ve başlangıçta,
  süresinin dolmasına 30 günden az kalmış sertifikalar için). Sayfalar gerçek bir
  barındırma müşterisinin oturumunda çizilmedi.
- **Önceki bir sürüme dönüşte koruma kaybolur** ("Önceki bir sürüme dönüş"
  bölümüne bakın). İlk satırı olmayan dosyalar için ölçüldü; olanlar için önceki
  sürümün kaynağından okundu.
- **Havuz ve birim.** Panel başlarken bunlara bakılmaz; başlangıç satırı yalnız
  nginx dosyalarını sayar. Site oluşturma onlar için kayıt satırı açmaz; bu
  yüzden kimsenin açmadığı ya da değiştirmediği bir havuz ya da birim Alan
  Adları listesinde yer almaz. Çağrılar Panel'in derlemesine bağlıdır; başka
  derlemeden bir Agent için geri dönüş yolu yoktur. Genel PHP sayfasının havuz
  adına göre yaptığı havuz kaydı türlü reddi yalnız Agent'ın hata metninden
  alır.
- **nginx bütün siteleri birlikte değerlendirir.** Sahibin başka bir yerdeki
  bozuk dosyası, eskisi gibi, bir site dosyasının her yazımını başarısız kılar
  (yazılan dosyalar geri konur). Sizin include dizininiz sitenin `location`
  bloklarından sonra okunur.
- **Kapsam dışı:** Panel'in kendi sertifikası için nginx sitesi ile web postası
  ve veritabanı araçlarının nginx dosyaları.
- **Henüz dönüştürülmeyen ekranlar.** Arayüzün 30 kaynak dosyası
  (v0.1.0-alpha.82'de 31) sunucuyu en az bir yerde hâlâ eski biçimde okuyor:
  uygulama kabuğu ve oturum okuması, oturum açma sayfası, kurulum sihirbazı,
  güncelleme kartı ile güncelleme ve bileşen işlemi katmanları, Hizmetler
  listesi ve hizmet sayfaları, kenar çubuğu, panonun bazı bölümleri, eklentiler,
  denetim günlüğü, VPN, ekip üyeleri, lisans sayfası ve birkaç küçük bölüm.
  Bunlarda başarısız bir okuma hâlâ boş ya da olumsuz bir durum gibi
  görünebilir.
- **Erişim beklemesi.** Her sayfa yüklemesi oturumu hâlâ iki kez okur. HTTP
  önbelleği açık bir yükleme ölçülmedi. Kurtarma hizmet çalışanının
  laboratuvardaki oturum açılmış tarayıcı bağlamlarında neden kayıtlı olmadığı
  saptanmadı; bu yüzden Panel durmuşken çevrimdışı sayfa ölçülmedi.
- **Güncelleme bildirimi.** Kartın "Current version" satırı, sayfa kendini
  yeniden yükleyene kadar önceki sürümde kalır (laboratuvarda gözlendi).
  Telefonda köşedeki bildirim, taklit API'ye karşı tarayıcıda kartın durum
  satırını örttü. Doğrulama aralığı kurulu bir sunucuda görülmedi. Bu sürüme
  güncelleme sırasında açık sayfa hâlâ v0.1.0-alpha.82'ninkidir.
- **Kurulum.** Posta kimliği nedeni hiç gerçek bir Agent'tan okunmadı. Eşleşen
  bir ters DNS adından sonra başarısız olan ileri sorgu aynı "bilinmiyor" olarak
  bildirilir; ikisi ayırt edilmez. Önceki bir sürümün Agent'ı boş bir ters DNS
  adını hâlâ uyuşmazlık olarak bildirir.
- **v0.1.0-alpha.82'den geçerliliğini koruyan sınırlar.**
  - İstek kimliği olmayan rotalar: artık on iki rota onu ister
    (v0.1.0-alpha.82'nin sekizi ve yukarıdaki dört POST rotası). Hizmet ve
    uygulama yeniden başlatma, planlar, kayıt kodları, yaklaşık 38 başka rota,
    DNS kayıtları, barındırma türü, PHP ayarları, takma adlar, uygulama
    kurulumu, posta kimlik doğrulaması, hesaplar ve dosyalar korunmuyor.
  - Sunucuda zaten kurulu bir posta yenileme yardımcısı panel güncellemesiyle
    değiştirilmez.
  - `postconf` uyarısının ardındaki posta sertifikası yolu bir sunucuda
    ölçülmedi.
  - Paketleme v0.1.0-alpha.82'den beri değişmedi; o sürümün arşivi satıcının
    yayın araçlarını ve eski bir olaydan kalan iki tek seferlik kurtarma
    betiğini taşır. Bunların arşivden çıkıp çıkmayacağı, hâlâ açık bir sahip
    kararıdır.
  - Her satıra bir kısıtlama yazılmış Postfix kısıtlama listesi, bir DNSBL
    kaydından sonra tek satır olur.
  - Panel'de bir veritabanı kullanıcısının parolasını belirleyen bir denetim
    yoktur.
  - [v0.1.0-alpha.82](RELEASE-NOTES-v0.1.0-alpha.82.tr.md) sınırlarından bu
    sürümün ele almadıkları geçerliliğini korur. O sürümün "Üretilmiş bir site
    yapılandırma dosyasında kendi düzenlemeleriniz" sınırını bu sürüm ele alır:
    nginx dosyası için kısmen ölçülerek, havuz ve birim için kaynakta.
- **Kanıt kapsamı.** Tek bir dizüstü ana makine; imzalı arşiv değil, kabul testi
  lisanslı derlemeler; dışarıya NAT yapan ve trafiği yakalanmayan makineler;
  loopback'e sabitlenmiş adlar; bir kayıt yinelenen bir koşuyu adıyla
  vermedikçe her hücre bir kez koştu. Ana makine sekizinci, dokuzuncu ve
  onuncu kayıtlar sırasında "modern bekleme" kaydetti; disk izleyicileri 31
  saniyeden uzun boşluk göstermiyor. Geçmeyen denetimler her dizinin
  `checks-not-passed.txt` dosyasında sayılır.
- **Açık kabul işleri.** [Dayanıklılık sözleşmesinin](RESILIENCE-CONTRACT.tr.md)
  beş temel işinin tamamı kısmi kalır; bu sürüm hiçbirini kapatmaz. Kararlar:
  [DECISIONS](DECISIONS.tr.md) içinde D-021 (değişmedi), D-022, D-024, D-025,
  D-029, D-031.

## Yayımlama kaydı

Yayımlanmadı. Yayımlamadan önce açık olanlar:

1. Numara: v0.1.0-alpha.83 çalışma numarasıdır (sahibin seçimi; kesin değil).
2. On birinci ve on ikinci kayıtların sonuçları yukarıya girilir, ya da
   kapsadıkları adı belli sınırlar olarak kalır.
3. Sürüm hazırlığı (sürüm sırası, önyükleme sabitleri, sürüm satırları) ayrı
   bir commit'te yapılır; arşivi etikette CI derler ve imzalar.
4. Satıcının yayın araçları ile iki tek seferlik kurtarma betiğinin arşivde
   kalıp kalmayacağı (sahip kararı).

Bu sürümü yalnız CelikPanel'in güncelleme arayüzünden kurun. Yayımlamak kurulu
sunucuları güncellemez; bir sunucunun güncellemesini yalnız o sunucunun sahibi,
panelin kendi güncelleme ekranından başlatır.
