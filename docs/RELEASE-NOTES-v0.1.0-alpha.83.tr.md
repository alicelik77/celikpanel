# v0.1.0-alpha.83 (taslak, yayımlanmadı)

[English](RELEASE-NOTES-v0.1.0-alpha.83.md)

Bu, v0.1.0-alpha.82'yi izleyen sürümün taslağıdır. Sahip numarasının
v0.1.0-alpha.83 olmasına karar verdi. Sürümün hiçbir parçası yayımlanmadı;
hiçbir kurulu sunucu onu çalıştırmıyor. Bu taslak 11 Ekim 2026'da yazıldı.

Sürüm iki şey yapar. Birincisi, CelikPanel'in elle değiştirdiğiniz bir site
yapılandırma dosyasının üzerine yazmasını durdurur: dosya korunur, site bunu
söyler ve dosyaya ne olacağını siz seçersiniz. İkincisi, sahibin 10 Ekim 2026'da
v0.1.0-alpha.82'ye güncellemeden sonra kurulu iki sunucuda gördüğü dört şeye
yanıt verir (aşağıda "Son güncellemeden sonraki dört gözlem" altında
sıralıdır).

Sunucunun site dosyalarıyla ne yaptığı, bileşen testlerine ve dört laboratuvar
ölçümüne dayanır: biri bu çalışmanın ilk bölümünün, Debian 13, Ubuntu 24.04 ve
Arch'ta; biri onu izleyen düzeltmelerin (gerçek bir nginx'e karşı sertifika
doğrulama denetimi, kilitli dosyanın sayfası, include dizinleri), Debian 13 ve
Ubuntu 24.04'te, Arch yalnız include dizinleri, kilitli dosya ve bir doğrulama
durumu için; biri de havuzun, uygulama biriminin ve v0.1.0-alpha.82'den
güncellemenin, üçünde de; ayrıca üçüncüsünün yol açtığı düzeltmelerin yalnız
Debian 13 ve Ubuntu 24.04'te yapılan kısa bir turu var. Ölçülen güncellemeler, yayımlanmış
arşivlerden değil, etiketlerinin kaynağından test lisanslı derleme olarak
yeniden derlenmiş v0.1.0-alpha.81 ve alpha.82'den başladı. Kısa turdan sonra
yapılan son değişikliklerin yalnız bileşen testleri var. Bir ekranın ne
gösterdiği bileşen testlerine ve API taklidine karşı çalışan bir tarayıcıya
dayanır; ilk sayfa yüklemesi, erişim beklemesi, güncelleme bildirimi ve
Yapılandırma dosyası sayfası için laboratuvarda gerçek bir Panel'e karşı gerçek
bir Chrome'a da. Hiç çalıştırılmayanları sınırlar bölümü söyler.

## Sunucu sahibi için ne değişiyor

**Güncellemeden sonra açık olan her CelikPanel sayfasını yeniden yükleyin.**
Güncellemeden önce yüklenmiş bir sayfada yeni Yapılandırma dosyası sayfası
yoktur; sayfa güncellemenin yeni aşamasını tanımaz ve eski beklemeleri çizer. Bu
sürümün yeni rotaları, istek kimliği taşımayan isteği `428` ile reddeder;
reddedilen istek hiçbir şeyi değiştirmez.

**Güncelleme sırasında hâlâ önceki sürümün sayfasını görürsünüz.** Güncelleme
sürerken açık olan sayfa, v0.1.0-alpha.82'nin arayüzüdür. Panel yeniden
başlarken o arayüzün sayfa üzerindeki katmanı lisanstan söz edebilir
(laboratuvarda o arayüzle görüldü); köşedeki bildirimi de kart yeni sürümü
gösterirken güncellemenin hâlâ uygulandığını söyleyebilir (sahip son
güncellemeden sonra bildirdi). Bu sürüm ikisini de düzeltir; düzeltmeler, bu
sürümün bir sayfasından başlatılan güncellemede görünür. Başarılı bir
güncellemeden sonra sayfa kendini yeni arayüzle yeniden yükler.

**Değiştirdiğiniz site yapılandırma dosyası korunur.** Şimdiye kadar her Panel
başlangıcı ve yeniden başlatma, barındırılan her sitenin nginx dosyasını,
kaydedilen bir ayar da o sitenin dosyasını, orada ne olduğuna bakmadan
CelikPanel'in kayıtlarından yeniden yazıyordu: eklediğiniz bir `location`,
değiştirdiğiniz bir yönerge, kendi include satırınız ya da baştan değiştirdiğiniz
bir dosya gidiyor, sildiğiniz bir dosya geri geliyordu. Bu, v0.1.0-alpha.82'de
(etiketinin kaynağı test lisanslı derleme olarak derlendi) Debian 13, Ubuntu
24.04 ve Arch'ta ölçüldü. Artık CelikPanel'in bir site için yazdığı her dosya, dosyanın geri
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
`sudo chattr -i <yol>`) ve "Yeniden kontrol et"i seçin; sayfa o zaman
"CelikPanel'in metnini şimdi yaz" seçeneğini sunar, bir sonraki başlangıç ya da
kaydedilen değişiklik de onu yazar. Kilitli kalsın istiyorsanız bir şey
yapmayın: CelikPanel onu değiştirmez. Bir korumanın reddettiği "CelikPanel'in
metnini al" işlemi geride tarihli kopya bırakmaz.

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
altında sunup sunmadığını denetler (sunucunun kendisinden 80 numaralı bağlantı
noktasına, dosyanızın 80 numaralı bağlantı noktası bloklarının o ad için
dinlediği her adreste, genel adres için de loopback'te yapılan bir istekle). Sunuyorsa sertifika, dosyanıza
dokunulmadan alınır; ama siz CelikPanel'in metnini alana ya da iki sertifika
satırını kendiniz değiştirene kadar site, dosyanızın adını verdiği sertifikayı
sunmayı sürdürür; SSL sekmesi ve pano bunu, kullanılan sertifikanın kalan
günleriyle birlikte söyler. Sunmuyorsa hiçbir şey istenmez: SSL sekmesinden
yapılan istek eklenecek satırla birlikte reddedilir, yenileme de başarısız olmak
yerine sizi bekler. nginx bu denetime yanıt vermezse sonuç "bilinmiyor"dur,
asla "hazır değil" değildir. Dosyanız siteyi bu sunucunun kendi içinden
ulaşamadığı bir adreste dinletiyorsa denetim orada çalışamaz: istek yine de
gönderilir, yanıt denetimin çalışamadığını söyler ve adresi verir, kararı
sertifika otoritesinin kendi denemesi verir; hiçbir şey kendiliğinden yeniden
denemez. v0.1.0-alpha.81 ya da alpha.82'den kalan korunmuş bir dosyada yeni
dizini içeren satır yoktur; doğrulamanın yine de çalışıp çalışmadığına bu
denetim karar verir. Denetim laboratuvarda gerçek bir nginx'e karşı ölçüldü
("Ne ölçüldü" bölümüne bakın); orada hiçbir sertifika alınmadı, adreslerin
seçimi de o ölçümden sonra geldi. Panoyu izleyin: seçiminizi bekleyen sertifika en başta listelenir.

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
görünür; bu yüzden Alan Adları listesindeki olduğu gibi bırakılan dosya sayısı,
alan adlarınızı açtıkça artabilir. Bu sürümün yazdığı bir birim, uygulamanın
komutunu oturum açma kabuğuyla değil, `/bin/sh -c` ve Panel'in Node dizinini
adıyla veren açık bir `PATH` ile başlatır; bu yüzden `/etc/profile` ona artık
uygulanmaz, uygulamanızın ortamdan gereksindiği şeylerin yeri ek ayar
dosyasıdır. Bu bölüm laboratuvarda üç platformda ölçüldü; iki bulgu sonradan
düzeltildi ve Debian 13 ile Ubuntu 24.04'te yeniden ölçüldü (sonraki paragrafa
ve "Ne ölçüldü" bölümüne bakın).

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
bir denetim ve tek bir yeniden yüklemeyle yazılır, bağlantı da yerinde kalır.
Sayfa dosyayı, sistem hatası ve korumayla birlikte CelikPanel'in
değiştiremediği bir dosya olarak adlandırır; "Yeniden kontrol et" yalnız okur
(ikisi de laboratuvarda ölçüldü).

**Reddedilen bir değişiklik bir hizmeti durdurmuyor.** nginx, PHP-FPM ya da
systemd CelikPanel'in metnini reddettiğinde CelikPanel önceki dosyanızı geri
koyar. Hizmeti o dosyayla yeniden yüklemeden önce artık hizmetin kendi
denetimini (`nginx -t`, `php-fpm -t`, bir uygulama birimi için de
`systemd-analyze verify`) çalıştırır. Denetim yapılandırmayı başka bir dosya
yüzünden reddederse hizmet yeniden yüklenmez ve daha önce yüklediği
yapılandırmayla çalışmayı sürdürür; yanıt bunu söyler ve yöneticiye denetimin
yazdığı satırı verir. O dosyayı düzeltin, hizmeti kendiniz yeniden yükleyin,
sonra CelikPanel'in metnini hâlâ istiyorsanız yeniden seçin. Önceden çalışmayan
bir hizmet ne yeniden yüklenir ne başlatılır; yanıt onu başlatmanızı söyler
(laboratuvarda yalnız PHP-FPM ile denendi; nginx ve bir uygulama için bileşen
testleri var). Laboratuvar önce eski davranışı üç platformda (reddedilmiş bir
yapılandırmayla yeniden yüklenen PHP-FPM, yanıt onun çalışmayı sürdürdüğünü
söylerken `failed` durumunda kaldı), sonra düzeltmeyi Debian 13 ve Ubuntu
24.04'te ölçtü: PHP-FPM ve nginx çalışan süreçlerini korudu, uygulama da
reddedilen ya da çalıştırılamayan bir birim denetiminin ardından yeniden
başlatılmadı (orada laboratuvarın geri koymayı zorlaması gerekti).
CelikPanel'in okuyamadığı bir havuz ya da birim,
sonraki bir okuma yanıt verene kadar, Alan Adları listesinde "Bir yapılandırma
dosyası okunamadı; durumu bilinmiyor" olarak gösterilir (bu da ölçüldü).

**İlk sayfa yüklemesi, söyleyecek bir şey olana kadar hiçbir şey çizmiyor.** Bir
CelikPanel sayfasının tam yüklemesi, henüz hiçbir şey yanıt vermeden kısa bir an
"Reload CelikPanel" düğmeli tam sayfa bir "Checking panel access" ekranı
gösteriyordu. Artık sayfa ilk 1,5 saniye boş kalır. Erişim bundan sonra hâlâ
denetleniyorsa sayfa, Panel'in henüz yanıt vermediğini ve bir şey yapmanız
gerekmediğini söyler; 15 saniyeden sonra "Şimdi kontrol et"i, 30 saniyeden
sonra "CelikPanel'i yeniden yükle"yi de sunar. Erişimi doğrulamayan bir yanıt
(oturum yok, bir hata durumu, Panel başlıyor) ya da başarısız olan bir bağlantı,
nedeniyle birlikte hemen gösterilir. Kendi süre sınırına ulaşan bir denetim
hâlâ bilinmiyor olarak gösterilir, asla başarısızlık olarak değil.

**Sayfa üzerindeki katman, okuduğu nedeni söylüyor.** Panel erişimi kısa bir
süre doğrulayamadığında sayfa, v0.1.0-alpha.82'deki gibi bir katmanın altında
kalır. Katman artık hangisini okuduğunu söyler: Panel yanıt vermedi; Panel
başlıyor; lisans sonucu okunamadı; ya da oturumunuz doğrulanamadı. Panel yanıt
vermediyse ve aynı tarayıcıdan son 30 dakika içinde bir güncelleme
başlatılmışsa ve bitişi orada henüz görülmediyse, Panel'in yanıt vermediğini ve
bir güncellemenin onu yeniden başlatıyor olabileceğini, lisans hakkında hiçbir
karar verilmediğini söyler.

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
   laboratuvardaki bir ölçüm (sürümler etiketlerinin kaynağından yeniden
   derlenmişti) bunu o arayüzle yeniden üretti ve alpha.82
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
   geçti. Ardından gelen 15 ve 30 saniyelik adımlar o ölçümden sonra yapıldı ve
   daha sonraki bir laboratuvar kaydında Debian 13 ile Ubuntu 24.04'te ölçüldü:
   geçti.
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
okumayı bırakır. Laboratuvarda otomatik bir dönüşten sonra önceki sürümün
başlangıcı, bir sahip düzenlemesi de dahil her site dosyasını yeniden yazdı:
v0.1.0-alpha.81 bir kayıtta üç platformda, bir başkasında Debian 13'te,
v0.1.0-alpha.82 de üçüncü bir kayıtta üç platformda (her biri etiketinin
kaynağından test lisanslı derleme olarak yeniden derlenmişti). alpha.82'nin yazdığı havuzlar ve birim bayt bayt olduğu gibi
kaldı. Her durumda başarısız aday kendi başlangıcından önce durdu; bu yüzden
hiçbir dosya henüz ilk satırı taşımıyordu. Otomatik dönüş, başlangıcında
ya da başlangıç denetiminde başarısız olan bir adayı izler; böyle bir adayın
başarısız olmadan önce ilk satırı yazmış olabileceği ölçülmedi. Önceki bir
sürümün ilk satırı zaten taşıyan bir dosyaya ne yaptığı ölçülmedi; o sürümün
kaynağından okundu. Bu sürüm yeniden çalıştığında önceki sürümün metnini
tanır ve devralır; önceki sürüm çalışırken ve onun bir sonraki başlangıcından
önce yaptığınız düzenleme korunur. Dönüşten sonraki bu ikinci güncelleme
laboratuvarda yapılmadı; bu cümle tasarım ve bileşen testlerine dayanır.

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

Hatalar `{"error", "code", "action"?, "reason"?, "detail"?, "vars"?,
"details"?}` biçimini korur ve ek olarak `restore`, `service`, `service_reason`
ve `service_active` alanlarını kazanır (aşağıda). Ekranlar `code` üzerinden
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
  birleştirme sunucuda yapılır, yeniden kontrol GET'in yinelenmesidir.
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
  `validation_status` ile; sorulan adresler ya da bu sunucunun olmayan adres
  için `validation_address` ve `validation_unknown_reason`), `resolved_reason`, `pending_reason` (`certificate`,
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
- GET ayrıca bulduğunu Panel'in kendi veritabanına kaydeder; böylece Alan
  Adları satırı dosyayı izler: artık var olmayan reddedilmiş bir değiştirme,
  elle geri konan bir dosya, Agent'ın yanıt vermediği bir havuz ya da birim
  (bunun dışında hiçbir site dosyasını yazmaz).

**POST yanıtları.** `keep`: karardan sonraki GET nesnesiyle (`files[]` olmadan)
`200`; dosyaya hiçbir şey yazılmaz. `take`: `outcome` `taken` (ya da
`unchanged`) ve `backup_path` ile `200`. `recreate`: `outcome` `recreated` (ya da
`unchanged`) ile `200`. `apply`: `outcome` `written` (ya da `unchanged`) ile,
tarihli kopya olmadan `200`. `take`, `recreate` ve `apply` ayrıca, Agent
bildirdiğinde, `managed_dir`, `managed_include` ve `challenge_file` taşır.

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

Bir korumanın reddettiği `take` tarihli kopya bırakmaz: Agent korumayı
kopyadan önce ölçer, değiştirme yine de reddedilirse kopyayı siler. Havuz ya da
birim için CelikPanel'in metni yine de dosyanın yanına bekleyen dosya olarak
konur.

`restore`, Agent önceki baytların geri konduğunu (geri koymadan sonra yeniden
okunarak) bildirdiğinde `confirmed` olur; başka durumda `unconfirmed` olur ve
cümle, dosyayı sunucuda sunucu yöneticisinin denetleyeceğini söyler. İki 502
yanıtı ek olarak, geri koymadan sonra hizmete ne olduğunu da taşır: `service`
(`reloaded`; hizmetin kendi denetimi geri konan yapılandırmayı reddettiyse
`service_reason` `config_refused_elsewhere` ile ya da `check_unavailable` ile
`not_reloaded`; hizmet önceden çalışmıyorduysa ve ne yeniden yüklendi ne
başlatıldıysa `not_running`; `unconfirmed` kalan `reload_failed`) ve
`service_active` (işlemden sonra `systemctl is-active` ile okunan `"true"`,
`"false"` ya da `"unknown"` dizgeleri). Yönetici için denetimin satırı `vars.line` ve
`details` içindedir. Süresi dolan, başlatılamayan ya da çıktısız biten bir
`systemd-analyze verify` ret değil, `check_unavailable` sayılır. Agent'ın kendi
satırı yalnız yöneticilere gönderilir. Bu sürümün son değişikliğinden beri
(yalnız bileşen testleri), hizmeti yeniden yükleyen bir `take`, `apply` ya da
`recreate` işleminin `200` yanıtı da `service: "reloaded"` ve `service_active`
taşır.

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
  onun için verdiği HTTP durumu) ve `vars.address` (yanıt veren adres) ile
  `409`; denetim yerel bir nedenle çalışamadıysa (sorulan bir adreste nginx 80
  numaralı bağlantı noktasında yanıt vermedi, doğrulama dizini yok, deneme
  dosyası oluşturulamadı ya da bu sunucunun adresleri listelenemedi) `detail`
  `unknown` (ve `vars.address`) ile `503 CERTIFICATE_VALIDATION_UNKNOWN`. Bir
  ad bu sunucunun olmayan bir adreste dinliyorsa istek yine de gönderilir ve
  başarı yanıtı (sertifika isteği, takma ad ekleme ve takma ad silmede yeniden
  alma) `validation_probe: "not_run"`,
  `validation_unknown_reason: "address_not_local"`, `validation_address` ve
  `validation_note` taşır; bunun Panel'in günlük satırı dışında hiçbir kaydı
  saklanmaz. Eksik ya da
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
`kept_by_choice`, `pending_reason`, `reason` ve `read_error` ile `site_config`
taşıyabilir. Agent'ın yanıt vermediği bir havuz ya da birim, bir sonraki
yanıtlanan okumaya kadar `read_error` (`timeout`, `agent_unreachable`,
`agent_error`, `render_input_unavailable`; sayfadaki dosya da aynı alanı
taşır) ile `unknown` olarak kaydedilir; bilinen bir sorunu olan satır onu
korur. Bu, taze bir
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
- Bu sunucunun kendi içinden ulaşamadığı bir adreste dinleyen korunmuş bir
  dosya için sertifika isteği reddedilmez, bir uyarıyla gönderilir.
- Düzenlediğiniz bir havuzda havuz kaydı ya da PHP sürümü değişikliği
  reddedilir; havuz satırlarınız artık silinmez.
- Reddedilen bir değişiklikten sonra, kendi denetimi geri konan yapılandırmayı
  reddeden hizmet yeniden yüklenmez; çalışmayan bir hizmet başlatılmaz.
- Bir uygulama birimi komutunu oturum açma kabuğu olmadan, açık bir `PATH` ile
  başlatır.
- Yapılandırma dosyası sayfasının okuması bulduğunu kaydeder; böylece Alan Adları
  satırı, elle geri koyduğunuz bir dosyayı izler.
- Kurulum sayfası bir çalışmanın neden durduğunu adımların üstünde söyler; posta
  kimliği denetimi koşulunu adıyla verir.
- Güncelleme bildirimi ve kart aynı aralıkta "kuruldu, doğrulanıyor" der;
  güncelleme penceresi yerel saati saat dilimiyle gösterir.
- Soğuk sayfa yüklemesi 1,5 saniye boyunca hiçbir şey çizmez; "Şimdi kontrol
  et" 15 saniyede, "CelikPanel'i yeniden yükle" 30 saniyede gelir.
- Sayfa üzerindeki katman okuduğu nedeni söyler.
- SSL sekmesi ve, seçiminizi bekleyen bir sertifika için pano, eksi gün sayısı
  yerine "süresi N gün önce doldu" der.

Gerçek hizmetlerde ölçülenler (sonraki bölüm): değişmeyen başlangıç, oluşturma
metni, reddedilen kayıt, silinen dosya, kilitli dosya ve sayfası, bir sitenin
silinmesi (düzenlenmiş nginx dosyası ya da havuz tarihli kopya olarak kalır,
sizin include dizininiz yerinde kalır; CelikPanel'in kendi dizininin kaldırılmasına
bakılmadı), yazılan dosyaların sahibi ve izinleri, include dizinlerinin izinleri, korunan
dosyada doğrulama denetimi ve sorduğu adresler (sertifika alınmadan),
"CelikPanel'in metnini şimdi yaz", kopya bırakmayan reddedilmiş alma, havuz ve
birim, geri koymadan sonra yeniden yüklemeden önceki denetim, Alan Adları
listesinde okunamayan dosya, birimin `PATH`'i, ilk yüklemedeki sessiz süre ve
beklemenin 15 ve 30 saniyelik adımları. Güncelleme bildiriminin yeni sözleri
laboratuvarda gerçek bir Panel'de gözlem olarak görüldü. Bulduğunu kaydeden
sayfa okumasının ve başarılı bir seçimin `service` alanının yalnız bileşen
testleri var; kurulum sayfasının, katmanın son ifadesinin ve SSL sekmesinin
ifadesinin bileşen testleri ve taklit API'ye karşı tarayıcı denetimi var.

## Ne ölçüldü, nasıl ölçüldü

Tek bir dizüstü ana makinedeki geçici sanal makinelerde 10 ve 11 Ekim 2026'da
(UTC; on birincisi 10 Ekim'i 11 Ekim'e bağlayan gece koştu) alınmış yedi kayıt.
Ürün imzalı arşiv olarak değil, kabul testi lisansıyla derlenmiş hâliyle çalıştı.
Her güncelleme, yayımlanmış arşivden değil, etiketinin kaynağından test lisanslı
derleme olarak yeniden derlenmiş önceki bir sürümden ölçüldü; bu derlemelerin
özetleri yayımlanmış olanlarınkinden farklıdır. Dizinler
`deploy/e2e/release-recovery/evidence/` altındadır. Her sonuç dosyası kendisini
henüz kabul edilmemiş kanıt olarak işaretler; değerlendirmesi sahibindir.

**Laboratuvarın saptadığı ve saptamadığı.** Sanal makineler ağdan kopuk
değildir: dışarıya doğru NAT yapan QEMU kullanıcı ağıyla çalışırlar ve
dağıtımların paketleri koşu sırasında kendi depolarından kurulur. Bir makinenin
neye erişebildiğini ya da erişemediğini burada hiçbir şey göstermez. Sürüm ve
lisans sunucusunun adı ile dört sertifika otoritesi adı, hiçbir ürün yolu
oluşmadan önce makinenin hosts dosyasında kendi loopback adresine sabitlendi ve
sonda geri okundu (yedinci, sekizinci, dokuzuncu, on ikinci ve on üçüncü
kayıtların bütün laboratuvarlarında; onuncunun 12 laboratuvarından 11'inde ve
on birincinin biten her laboratuvarında; durdurulan bir koşunun geri okuması
yok). On birinci ve on ikinci kayıtlarda Panel'in yaptığı sertifika istekleri
sabitlenmiş ada, yani konuğun kendisine gitti ve certbot orada bulduğu
sertifikayı reddetti; bu yüzden hiçbir sertifika otoritesi işe katılmadı.
Lisans adımını test derlemesinin kendisi yanıtlar; bu, derlemenin kendisi
hakkında söylediğidir, trafiğin gözlemi değildir. Hiçbir sürücü kurulu bir
sunucuya yöneltilmedi. Makinelerin trafiği yakalanmadı.

**v0.1.0-alpha.82'nin sahibin düzenlediği site dosyasıyla yaptığı (sekizinci
kayıt, `set8-20261010`; Debian 13, Ubuntu 24.04, Arch).** Yayımlanmış etiketin
kaynağı, yayımlanmış arşiv değil, test lisansıyla ve bir alpha.81 etiketiyle
yeniden derlendi; 48 koşu (nginx dosyasında 40, havuzda 4, kilitli dosyayla 4).
nginx
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
henüz içinde değildi. Yeni kuruldu; ayrıca etiketinin kaynağından test
lisanslı derleme olarak yeniden derlenmiş (yayımlanmış arşiv değil)
v0.1.0-alpha.81'den güncellemeyle ulaşıldı. Platform başına 31
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
  kaynakta düzeltildi ve on birinci kayıtta yeniden ölçüldü (aşağıda).
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
  30 saniyelik adımlar bundan sonra yapıldı ve on birinci kayıtta ölçüldü.
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
  o denetliyordu. Ürünün çevrimdışı sayfasına burada ulaşılmadı; on birinci
  kayıt nedeni laboratuvar tarayıcısının profiline daralttı (aşağıda).

**İlk yükleme ve güncelleme için başlangıç ölçümü (yedinci kayıt,
`set7-20261010`; Debian 13).** v0.1.0-alpha.82 notlarında da anlatılır
(yayımlanmasından sonra güncellendi). alpha.82 etiketinin test lisanslı
derleme olarak yeniden derlenmiş kaynağında soğuk yükleme 18 yüklemenin 18'inde
tam sayfa "Checking panel access" çizdi; bir güncelleme sırasında alpha.81
arayüzü (etiketin yeniden derlenmiş kaynağı) 89 sn'lik kesintinin yaklaşık
78 sn'si boyunca sayfanın yerine lisans sayfasını koydu (sahibin anlattığı
ekran); alpha.82 arayüzü ise 25 sn'lik kesintinin yaklaşık 9–11 sn'si boyunca
sayfayı bir katmanın altında bağlı tuttu, katmanın ilk cümlesi lisanstan söz
ediyordu. İki kesinti farklıdır (birincisi alpha.81'den alpha.82'ye, ikincisi
alpha.81 etiketli bir alpha.82 derlemesinden alpha.82'ye güncellemeydi); bu
yüzden sayılar güncellemeleri değil, iki arayüzü karşılaştırır.

**Onuncu kayıttan sonraki düzeltmeler (on birinci kayıt, `set11-20261011`;
Debian 13 ve Ubuntu 24.04; include dizinleri, kilitli dosya ve bir doğrulama
durumu için Arch).** Kilitli dosya ve dizin düzeltmelerinden sonraki kod:
CelikPanel'in kendi include dizini, doğrulama denetimi (o zaman yalnız
`127.0.0.1`'e soruyordu), kilitli dosyanın yanıtı ve 0755 izni. "CelikPanel'in
metnini şimdi yaz", dosyalar arasındaki durum sırası, havuz ve birim, geri koyma
bilgisi, denetimin sorduğu adresler ve kopya bırakmayan reddedilmiş alma henüz
içinde değildi. Yeni kuruldu; Debian 13'te ayrıca etiketinin kaynağından test lisanslı derleme olarak
yeniden derlenmiş v0.1.0-alpha.81'den, başlangıç denetiminden geçemeyen bir
adaya güncelleme yapıldı. Tarayıcı
hücrelerinde dizüstü ana makinedeki başsız bir Chrome 154 kullanıldı.

- Include dizinleri: ikisi de üç platformda `root:celikpanel 0755` olarak
  oluşturuldu. Geçti.
- Gerçek bir nginx'e karşı doğrulama denetimi (Debian 13, Ubuntu 24.04).
  Doğrulama dizininde son izni `0644` olan ve 0,2–4,8 ms boyunca var olan tek bir
  deneme dosyası belirdi; nginx'e her ad için bir kez `127.0.0.1:80` üzerinden
  soruldu ve her ad için deneme dosyasının içeriğiyle `200` yanıtladı; istek
  ardından gönderildi (sabitlenmiş ada: certbot onu reddetti,
  `502 CERTIFICATE_ISSUE_FAILED` / `authority_unreachable`) ve sahibin dosyasına
  dokunulmadı (CelikPanel kendi bekleyen metnini yanına yazdı). nginx
  durdurulmuşken: `503 CERTIFICATE_VALIDATION_UNKNOWN`.
  CelikPanel'in include satırı olmayan korunmuş bir dosya (Arch'ta da):
  eklenecek satır, ad ve nginx'in durumu ile `409 SITE_CONFIG_OWNER_EDITED` /
  `certificate_validation` / `include_missing`. Sahibin `mail.` adı için kendi
  bloğu varken: hazır; o blok kaldırılınca `mail.` adı için `names_missing`,
  durum `404`. Sahip satırı geri koyup nginx'i yeniden yükledi: sayfanın bir
  sonraki okuması dosyayı hazır buldu ve hiçbir şey yazmadan ya da yeniden
  yüklemeden nedeni kapattı (CelikPanel'in doğrulama dosyası zaten yerindeydi;
  GET'in onardığı, dosyanın yok ya da güncel olmadığı durum denenmedi). Geçti;
  o hücrenin pano bölümü ölçülmedi (bu laboratuvarda hiçbir sertifika
  olamazdı).
- Geçmedi (Debian 13, Ubuntu 24.04): sahibin 80 numaralı bağlantı noktası
  bloğunu konuğun tek bir adresine bağladığı, öteki blokların genel adreste
  dinlediği bir site. Denetim `127.0.0.1`'e sordu, varsayılan sunucu `404`
  yanıtladı ve dosya CelikPanel'in satırını taşıdığı hâlde istek
  `include_missing` olarak reddedildi. Sonradan değişti: denetim, sitenin
  dosyasının dinlediği adreslere sorar (on ikinci kayıtta ölçüldü, aşağıda).
- Kilitli dosya (üç platformda): başlangıçta yalnız o dosya reddedildi, öteki
  siteler yazıldı; sayfa `EPERM` ve değiştirilemezlik özniteliğiyle
  "değiştirilemiyor" yanıtladı, Alan Adları satırı aynısını söyledi, "Yeniden
  kontrol et" yalnız okudu (yazma yok, yeniden yükleme yok). Kilit kaldırılınca
  "Yeniden kontrol et" dosyayı hâlâ değiştirilmemiş ve koruması bulunmamış
  olarak gösterdi (o kodun davranışı), ardından bir Genel kayıt onu yazdı.
  Geçti. İşaret bir tarayıcıda görülmedi: bu laboratuvarda Alan Adları
  listesinin önünde kurulum sayfası durur.
- Geçmedi (üç platformda): sayfanın okumasıyla alma arasında kilitlenen,
  sahibin düzenlediği bir dosyada alma `409 SITE_CONFIG_UNWRITABLE` ile
  reddedildi ve sahibin dosyasına dokunulmadı; ama yanında tarihli bir kopya
  kaldı, sayfa "CelikPanel dışında düzenlendi" derken Alan Adları satırı
  "değiştirilemiyor" dedi. Sonradan değişti: sayfa ve satır için tek bir durum
  sırası, ve geride kopya kalmaz (on ikinci kayıtta ölçüldü, aşağıda).
- Gerçek bir Chrome'da erişim beklemesi (Debian 13, Ubuntu 24.04; İngilizce).
  Her oturum okuması 35 sn bekletildi: sessiz süreden sonra açıklamalı bekleme,
  15,0 sn'de "Check now", 30,0 sn'de yarım dakika cümlesi ve "Reload
  CelikPanel"; arada başka hiçbir ekran yok. Geçti. Başarısız bir okumanın
  ardından bekletilen bir yeniden okuma: başarısızlık hemen gösterildi;
  kendiliğinden yapılan yeniden okuma bekletilirken 15 sn boyunca sayfanın
  yeniden okuma cümlesi ("Confirming your session and panel readiness") ile
  yeniden yükleme düğmesi; ardından o okumadan sayılan bekleme, "Last known,
  before this wait:" ve önceki başarısızlıkla, yarım dakika adımı da o okuma
  başladıktan 30 sn sonra; "Your session could not be checked" geri gelmedi.
  Geçti. Başlayan bir Panel'in ardından bekletilen yeniden okuma: Debian 13'te
  geçti; Ubuntu'da iki yükleme de yeniden başlatmanın hemen ardından Chrome'un
  kendi hata sayfasına düştü (ölçülmedi).
- Kurtarma hizmet çalışanı. Debian koşusunda oturum açılmış tarayıcı bağlamı,
  yaklaşık 177 karakter derinlikteki diskteki Chrome profilini kullandı (klasör
  düzeninden hesaplandı; yolun kendisi kaydedilmedi): hizmet çalışanı, önbellek deposunu açarken aldığı bir hatayla kurulamadı; aynı
  tarayıcının bellekteki yeni bir bağlamı ise onu kaydetti ve onun denetiminde
  kaldı. Ubuntu koşusunda profil kısa bir yoldayken oturum açılmış bağlam onu
  kaydetti. Panel durdurulduğunda, hizmet çalışanı olan bir bağlam ürünün
  çevrimdışı sayfasını ("The panel connection is unavailable") gösterdi; Debian'ın
  oturum açılmış bağlamı ise Chrome'un kendi hata sayfasını gösterdi. Yani
  dokuzuncu kayıttaki eksik hizmet çalışanı üründe değil, laboratuvar
  tarayıcısının profilinde olduğuna işaret eder. Mekanizmanın Windows yol
  uzunluğu sınırı olduğu çıkarımdır, Chrome'un bir satırından okunmadı; Ubuntu koşusu başka bir konuk ve
  başka bir tarayıcı süreciydi, denetimli bir karşılaştırma değildir; tarayıcının
  sabitleme olmadan güvendiği bir sertifika ölçülmedi. Laboratuvarın tarayıcı
  araçları profili artık kısa bir yolda tutar.
- Otomatik dönüş (Debian 13): aday başlangıç denetiminden geçemedi ve sunucu
  kendiliğinden v0.1.0-alpha.81'e döndü; veritabanı yeniden 42'deydi ve
  alpha.81'in başlangıcı, sahibin düzenlemesi dahil her site dosyasını yeniden
  yazdı. Aday kendi başlangıcından önce başarısız oldu; bu yüzden hiçbir dosya
  hiç ilk satırı taşımadı; önceki bir sürümün böyle bir dosyayla ne yaptığı
  ölçülmedi.

**İkinci bölüm, denetimin adresleri ve v0.1.0-alpha.82'den güncelleme (on
ikinci kayıt, `set12-20261011`; Debian 13, Ubuntu 24.04, Arch; Debian'da başsız
bir Chrome 154).** Havuzu ve birimi, "CelikPanel'in metnini şimdi yaz"ı,
dosyalar arasındaki durum sırasını, geri koyma bilgisini, denetimin sorduğu
adresleri ve kopya bırakmayan reddedilmiş almayı içeren kod. Son koddan, bu
kayıttan sonra sayılan düzeltmelerle, on üçüncü kayıttan sonra sayılan
değişikliklerle ve bazı ekran metinleriyle ayrılır. Yeni kuruldu;
ayrıca v0.1.0-alpha.82'den (üç platform) ve v0.1.0-alpha.81'den (Debian 13)
güncellemeyle ulaşıldı; her biri yayımlanmış arşivden değil, etiketinin
kaynağından test lisanslı derleme olarak yeniden derlenmişti. Sabitlenen adlar
dışında hiçbir şey engellenmedi: kurulumun paketleri dağıtımların depolarından,
Panel'in Node.js kurulumu da dışarıdan, dışarıya NAT üzerinden geldi; bu trafik
kaydedilmedi.

- Üç platformda da geçenler:
  - Havuz: sahibin yorumu ve yönergesi havuz kaydında korundu
    (`409 SITE_CONFIG_OWNER_EDITED` / `php_pool`; kaydedilen değeri taşıyan
    CelikPanel metni dosyanın yanında bekletildi); sayfa nginx dosyasını ve
    havuzu farkıyla listeledi; "CelikPanel'in metnini al" sahibin havuzunun
    tarihli kopyasını bıraktı, izin ve sahibi koruyarak CelikPanel'in metnini
    yazdı ve PHP-FPM'i yeniden yükledi. Bir site silinirken düzenlenmiş havuzun
    tarihli kopyası bırakıldı.
  - Birim (Panel üzerinden kurulan Node.js 22.20.0): sahibin ek ayar dosyası bir
    barındırma değişikliğinden ve bir almadan bayt bayt sağ çıktı; birimdeki
    bir sahip satırı, barındırma değişikliğini `409` / `app_unit` ile
    reddettirdi; alma tarihli kopya bıraktı, systemd'yi yeniden yükledi ve
    uygulamayı yeniden başlattı.
  - nginx'in reddettiği alma: `502 SITE_CONFIG_NGINX_REFUSED`, `restore:
    confirmed`, sahibin dosyası geri kondu ve sunuldu. Sahibin PHP-FPM'i
    durdurduğu bir alma: kendi cümlesiyle `restore: unconfirmed`, dosya geri
    kondu.
  - "CelikPanel'in metnini şimdi yaz": başlangıçta kilitli olup sonra kilidi
    kaldırılan dosyaya sunuldu; yazım tarihli kopya bırakmadı ve nginx'i
    yeniden yükledi; sayfa ve Alan Adları satırı aynı şeyi söyledi. Sayfanın
    okumasından sonra konan bir kilidin reddettiği alma:
    `409 SITE_CONFIG_UNWRITABLE`, tarihli kopya yok, sayfa iki olguyu da
    gösterdi, satır aynı şeyi söyledi.
  - Denetimin adresleri: konuğun kendi adresindeki 80 numaralı bağlantı noktası
    bloğuna o adreste, yalnız `[::]:80` üzerindeki bloğa da `::1`'de soruldu;
    ikisi de hazırdı ve istek gönderildi (sabitlenmiş adda reddedildi,
    `502 authority_unreachable`; Arch'ta paket okuyucu yalnız durum satırını
    gördü, bu yüzden o iki hücre nginx'in erişim günlüğünden değerlendirildi).
    Konuğun sahip olmadığı bir adres: deneme
    yok, istek gönderildi, adresle birlikte günlük satırı
    `validation_probe=not_run validation_unknown_reason=address_not_local`,
    sayfa da `unknown` / `address_not_local` okudu. nginx durmuşken: `503`.
  - v0.1.0-alpha.82'den güncelleme: veritabanı şeması 43'ten 44'e; Debian 13 ve
    Ubuntu 24.04'te "5 adopted, 1 left alone", Arch'ta "4 adopted, 1 left
    alone"; sahibin düzenlediği nginx dosyası olduğu gibi bırakıldı ve hiç
    yazılmadı; alpha.82'nin havuzu ve birimi devralındı, sahibin düzenlediği
    havuz olduğu gibi bırakıldı; dört yeni rota istek kimliği olmadan `428`
    yanıtladı ve hiçbir şey değişmedi. Sayfalar okunduktan sonra Alan Adları
    satırındaki sayılar değişti (5 ve 1, 4 ve 2 oldu; Arch'ta 4 ve 1, 3 ve 2),
    çünkü havuzun satırı o zaman açıldı.
  - v0.1.0-alpha.82'den kusurlu bir aday kendiliğinden geri döndü; veritabanı
    43'teydi ve Panel yanıt veriyordu; havuzlar ve birim bayt bayt alpha.82'ninkiydi,
    alpha.82'nin başlangıcı ise sahibin düzenlemesi dahil her nginx dosyasını
    yeniden yazdı. Aday kendi başlangıcından önce başarısız oldu; bu yüzden
    hiçbir dosya ilk satırı taşımıyordu.
  - v0.1.0-alpha.81'den güncelleme (Debian 13): dosyalar önceki gibi devralındı;
    geçişten önceki hâline geri konan veritabanı yeniden geçirildi.
- Üç platformda da geçmeyenler:
  - Agent'ın okuyamadığı bir havuz (kilidi 75 sn tutuldu): sayfa "okunmadı"
    dedi ve onu değişmemiş nginx dosyasının üstünde sıraladı, ama Alan Adları
    satırı "CelikPanel'in metni, değişmemiş" olarak kaldı, çünkü başarısız bir
    okuma satır yazmıyordu.
  - Havuz dizinindeki başka bir sahip dosyası yüzünden `php-fpm -t`'nin
    reddettiği alma: sahibin havuzu geri kondu, ardından PHP-FPM, denetimin
    hâlâ reddettiği yapılandırmayla yeniden yüklendi (günlükteki zamanlamadan
    çıkarıldı) ve `failed` durumunda kaldı; yanıt ise `restore: confirmed`
    diyor ve PHP-FPM'in çalışmayı sürdürdüğünü söylüyordu.
- Ölçülmeyenler: PHP sürümü değişikliği (her konukta tek PHP sürümü), kendisi
  başarısız olan bir geri koyma, çalışmayan bir denetimin başarı yanıtındaki
  alanlar (hiçbir istek başarılı olamazdı).
- Gözlem (Debian 13): birim `PATH` içinde Node dizinini adıyla verdiği hâlde,
  çıplak bir `node` başlatma komutu birimin oturum açma kabuğu altında 127 ile
  çıktı; olası neden, oturum açma profilinin `PATH`'i sıfırlamasıdır (çıkarım).
- Gerçek tarayıcı (Debian 13; İngilizce ve Türkçe, masaüstü ve telefon): Alan
  Adları işareti, nginx dosyası ve havuzla Yapılandırma dosyası sayfası,
  havuzun farkı ve "CelikPanel'in metnini şimdi yaz" sayfası; sonuç cümlesi
  sayfa metninden okundu. Kısa bir tarayıcı profil yolunda kurtarma hizmet
  çalışanı kaydedildi. İki geri koyma bildirimi sayfa metnindedir, ama onlar
  için alınan resimlerin hiçbirinde görünmez. Telefon genişliğinde Alan Adları
  listesi durum sütununu kesti, işaret alan adının altında durdu.

**On ikinci kayıttan sonraki düzeltmeler.** Sonradan yapıldı ve on üçüncü
kayıtta (sonraki paragraf) Debian 13 ile Ubuntu 24.04'te ölçüldü:

- Agent'ın yanıt vermediği bir havuz ya da birim, hata sınıfıyla birlikte
  okunmadı olarak kaydedilir; sonraki bir okuma yanıt verene kadar Alan Adları
  satırı "Bir yapılandırma dosyası okunamadı; durumu bilinmiyor" der.
- Geri koymadan sonra, önceden çalışmayan bir hizmet olduğu gibi bırakılır;
  başka durumda önce kendi denetimi geri konan yapılandırmada çalışır ve yalnız
  denetimin kabul ettiği yapılandırma yeniden yüklenir (bir birimin uygulaması
  yalnız `systemd-analyze verify` kabul ederse yeniden başlatılır). Yanıt
  hizmete ne olduğunu söyler. Kayıttaki, sahibin PHP-FPM'i durdurduğu durum
  artık `not_running` ile `confirmed` yanıtlanır.
- Birimin başlatma komutu `/bin/sh -c` ve Panel'in Node dizinini içeren açık
  bir `PATH` kullanır; önceki sürümlerin birim metinleri tanınmak için olduğu
  gibi kalır.
- Sayfanın bir seçime verdiği yanıt kendiliğinden görünür alana kayar.

**On ikinci kayıttan sonraki düzeltmeler (on üçüncü kayıt, `set13-20261011`;
Debian 13 ve Ubuntu 24.04; Arch koşulmadı; Debian'da başsız bir Chrome 154).**
Bu düzeltmeleri içeren kod; ondan yalnız sürüm sırası dosyasında ayrılan, test
lisanslı bir fikstür olarak derlendi; bu kayıttan sonra sayılan değişiklikleri
içermez. Yeni kuruldu; bu turda güncelleme ve sertifika isteği yok. Konukların
önceki gibi dışarıya NAT'ı vardı (kurulumun paketleri, Panel'in Node.js
kurulumu); bu trafik kaydedilmedi. Hiçbir ürün denetimi düşmedi; ilk koşuların
üç hücresi ölçülemedi (ikisinde düzeneğin sırası yüzünden önkoşul sağlanmadı,
birinde düzenek hatası) ve yeni laboratuvarlarda yeniden ölçüldü.

- Havuz dizinindeki sahip dosyası yüzünden `php-fpm -t`'nin reddettiği bir
  havuz alması: `502 SITE_CONFIG_SERVICE_REFUSED`, `restore: confirmed`,
  `service: not_reloaded`, `service_reason: config_refused_elsewhere`,
  `service_active: true`; denetimin satırı `vars.line` içinde ve cümlede.
  12–13 sn sonra PHP-FPM aynı ana süreçle ve yeniden başlamadan etkindi, almadan
  sonra onu yeniden yükleyen bir şey olmadı; sahibin havuzu bayt bayt aynıydı. O
  dosya kaldırılınca sonraki alma `200` yanıtladı ve PHP-FPM yeniden yüklendi
  (günlükten okundu; o `200` yanıtı `service` alanı taşımıyordu).
- Sahibin durdurduğu PHP-FPM: `restore: confirmed`, `service: not_running`,
  `service_active: false` ve sahibe PHP-FPM'i başlatmasını söyleyen cümle;
  hiçbir şey onu başlatmadı.
- nginx'in reddettiği alma: çakışma yalnız sitenin include dizinindeyken geri
  konan yapılandırma `nginx -t`'den geçti ve nginx yeniden yüklendi (`service:
  reloaded`); sahibin `conf.d` içindeki bozuk dosyasıyla nginx'in satırıyla
  birlikte `service: not_reloaded` ve nginx ana sürecini korudu. Sahibin dosyası
  ikisinde de bayt bayt aynıydı ve sunuldu.
- Agent'ın okuyamadığı bir havuz (kilidi 75 sn tutuldu): sayfa onu
  `read_error: timeout` ile `unknown` / `not_read` yanıtladı; Alan Adları satırı
  da artık aynısını söyledi, kilit kalktıktan sonra ve bir sonraki okumadan önce
  de; kayıt satırında hiçbir özet değişmedi. Yanıtlanan bir sonraki okuma satırı
  yeniden "CelikPanel'in metni, değişmemiş" yaptı.
- Node uygulama birimi: çıplak bir `node` başlatma komutu `/bin/sh -c` altında,
  açık `PATH` ile çalıştı ve yanıt verdi. Bir geri koymadan sonra uygulama,
  `systemd-analyze verify` birimi kabul edince yeniden başlatıldı, reddedince
  başlatılmadı (verify satırıyla `service: not_reloaded`), `systemd-analyze`
  çalıştırılamayınca da başlatılmadı (`check_unavailable`). (Laboratuvar geri
  koymayı, bir `daemon-reload`'u bir kez başarısız kılarak oluşturdu.)
- Gerçek tarayıcıda (yalnız Debian 13): reddedilen almanın yanıtı, masaüstünde
  İngilizce ve telefonda Türkçe, görünür alanın tepesinde bir durum kutusu
  olarak göründü; Alan Adları işareti "A configuration file could not be read; state not
  known" / "Bir yapılandırma dosyası okunamadı; durumu bilinmiyor" masaüstünde ve
  telefonda gösterildi ve sayfa okunduktan sonra kayboldu.
- Gözlem, hücre değil: bir nginx dosyasının reddedilen almasından sonra sahip
  CelikPanel'in metnini elle geri koydu; sayfa artık "CelikPanel'in metni,
  değişmemiş" derken Alan Adları satırı, bir sonraki yazıma kadar
  "Yapılandırma dosyası: seçiminiz gerekiyor" demeyi sürdürdü. Sonradan değişti:
  sayfa okuması bulduğunu kaydeder (yalnız bileşen testleri).

**On üçüncü kayıttan sonra.** Sonradan yapıldı; yalnız bileşen testleri;
gerçek bir sistemde ölçülmedi: reddedilen bir geri koymadan sonraki cümleler
işin nasıl süreceğini söyler, hizmet çalışmıyorsa sahibe onu başlatmasını
söyler; süresi dolan bir `systemd-analyze verify` `check_unavailable` olur;
sayfa okuması bulduğunu satıra kaydeder, hâlâ önceki bir reddi taşıyan bir havuz
ya da birim satırı için de (yukarıda); hizmeti yeniden yükleyen başarılı bir
seçim bunu yanıtında söyler.

**Hangi denetim hangi koda dayanıyor.** Kodun sonraki her hâli önceki
düzeltmeleri içerir.

| Denetlenen | Üzerinde koştuğu kod |
| --- | --- |
| nginx dosyasının korunması, üç seçenek, v0.1.0-alpha.81'den güncellemeden sonra devralma, başarısız geçişten sonra dönüş, geri konan veritabanı, kilitli dosyanın öteki sitelere etkisi | bu çalışmanın ilk bölümü (onuncu kayıt); alpha.81'den devralma ve geri konan veritabanı on ikinci kayıtta Debian 13'te yinelendi; hiçbiri son kodda yinelenmedi |
| CelikPanel'in kendi include dizini, 0755 izni, `127.0.0.1` üzerinde doğrulama denetimi, kilitli dosyanın sayfası ve "Yeniden kontrol et", erişim beklemesinin 15 ve 30 saniyelik adımları, hizmet çalışanı, başarısız başlangıç denetiminden sonra dönüş | kilitli dosya ve dizin düzeltmelerinden sonraki kod (on birinci kayıt) |
| Havuz ve birim, çok dosyalı sayfa, denetimin sorduğu adresler, "CelikPanel'in metnini şimdi yaz", kopya bırakmayan reddedilmiş alma, geri koyma bilgisi, v0.1.0-alpha.82'den güncelleme ve otomatik dönüşü | son ekran metinlerinden ve on ikinci kayıttan sonraki düzeltmelerden önceki kod (on ikinci kayıt) |
| Geri koymadan sonra yeniden yüklemeden önceki denetim (havuz, nginx dosyası, birim), durmuş PHP-FPM, Alan Adları listesinde okunamayan dosya, birimin `PATH`'i, görünür alandaki yanıt | on ikinci kaydın düzeltmelerinden sonraki kod (on üçüncü kayıt; yalnız Debian 13 ve Ubuntu 24.04, tarayıcı bölümleri yalnız Debian 13) |
| İşin nasıl süreceğini ve "çalışmıyor" durumunu söyleyen cümleler, verify zaman aşımı, bulduğunu kaydeden sayfa okuması, başarılı bir seçimin `service` alanı | yalnız bileşen testleri; son kod on üçüncü kaydın kodundan bunlarla ayrılır |
| Soğuk ilk yükleme ve 1,5 saniyedeki açıklamalı bekleme | ilk kapı düzeltmelerinden sonraki arayüz (dokuzuncu kayıt) |
| Katmanın son ifadesi, kurulum sayfası, posta kimliği nedeni | bileşen testleri ve taklit API'ye karşı tarayıcı |
| "Kuruldu, doğrulanıyor" bildirimi | dokuzuncu kayıtta gerçek bir Panel'de gözlendi; bileşen testleri |
| v0.1.0-alpha.82'nin sahip düzenlemesiyle yaptığı | yayımlanmış etiketin test lisanslı derleme olarak yeniden derlenmiş kaynağı (sekizinci kayıt) |
| alpha.81 ve alpha.82 arayüzlerinin bir güncelleme sırasında ve soğuk yüklemede yaptıkları | iki etiketin test lisanslı derleme olarak yeniden derlenmiş kaynağı (yedinci kayıt) |
| Bu yazılırkenki kodun tamamı | bileşen testleri; aşağıda anılan o anki uçta CI |

**Ekranlar.** Arayüz değişikliklerinin bileşen testleri var; ayrıca bir
Chrome'da, API'nin loopback üzerindeki bir taklidine karşı incelendiler (masaüstü
ve telefon, İngilizce ve Türkçe; Yapılandırma dosyası sayfası için açık ve koyu
tema). O tarayıcının arkasında gerçek bir Panel yoktu. Dokuzuncu kayıt ilk
yükleme, erişim beklemesi ve bir güncelleme için gerçek bir Chrome'u gerçek bir
Panel'in önüne koydu (masaüstü boyutu, başsız; Türkçe yalnız erişim beklemesi
için), on birinci kayıt erişim beklemesi ve hizmet çalışanı için (İngilizce),
on ikinci kayıt Alan Adları listesi ve Yapılandırma dosyası sayfası için
(İngilizce ve Türkçe, masaüstü ve telefon), on üçüncü kayıt da reddedilen almanın
yanıtı ile okunamayan bir dosyanın Alan Adları işareti için (İngilizce ve
Türkçe, masaüstü ve telefon; yalnız Debian 13).

**Testler.** Kodun son hâllerinin değişiklik kayıtlarına göre: 1315 arayüz
testi geçti; Panel ve ortak Go paketleri geçti; Agent paketinde bu
değişikliklerden öncekiyle aynı 19 test düştü ve değişiklik kaydı bunların
nedeni olarak geliştirme makinesinin ortamını gösterir. Bu sayılar saklanan bir
test günlüğünden değil, değişiklik kayıtlarından gelir; arayüz testleri bu
notlar denetlenirken ucun üzerinde yeniden koşuldu (1315 geçti, hiçbiri
düşmedi), Go paketleri koşulmadı. Çekme isteğinin 11 Ekim 2026'da 10:20, 11:40,
11:44 ve 11:49 UTC'deki uçlarındaki sürekli tümleştirme koşularının her biri 23
denetimin 23'ünü geçti (24 iş: yalnız bir sürüm etiketinde çalışan dosya
yayımlama işi atlandı); sırasıyla 10:41, 11:57, 11:59 ve 12:09 UTC'de bitti.
11:44 UTC'deki uç ürün kodundaki son değişikliği (bulduğunu kaydeden sayfa
okuması ve başarılı bir seçimin `service` alanı) taşır; 10:20 UTC'deki uç ondan
öncedir. 09:22 ile 09:48 UTC arasındaki altı koşu, hepsi "Panel race — A and B,
exact and non-uppercase" işinde, bir testin sahte Agent'ındaki bir veri yarışı
yüzünden başarısız olmuştu; sonraki koşuyu geçiren değişiklik test kodundadır.
12:09 UTC'den sonra yapılan commit'ler kendi koşularını alır; bunlar buraya
girilmedi. Geçen bir koşu sahibin kendi denemesinin yerini tutmaz.

## Bu sürümün sınırları

Bunlar bilinen sınırlardır. Gizli kusur değildir.

- **Son kod gerçek bir sistemde bütünüyle ölçülmedi.** Onuncu ile on üçüncü
  arasındaki kayıtlar kodu dört aşamada ölçtü. On üçüncü kayıttan sonraki
  değişikliklerin (işin nasıl süreceğini ve "çalışmıyor" durumunu söyleyen
  cümleler, verify zaman aşımı, bulduğunu kaydeden sayfa okuması, başarılı bir
  seçimin `service` alanı) yalnız bileşen testleri var. Sürüm hazırlığı (sürüm
  sırası 83, önyükleme sabitleri, sürüm satırları) ölçülen hiçbir derlemede
  yoktur (on ikinci kaydın güncelleme adayları laboratuvarda yapılmış bir sürüm
  sırası 83 ilkesi ve alpha.83 etiketi taşıyordu, test lisanslı derleme olarak);
  çekme isteğindeki sürekli tümleştirmenin yeniden üretilebilir arşiv denetimi
  dışında, bu yazılırken hazırlanmış daldan bu sürümün hiçbir arşivi
  derlenmemişti.
- **Arch** on üçüncü kayıtta yoktu: yeniden yüklemeden önceki denetim,
  okunamayan satır ve birimin `PATH`'i yalnız Debian 13 ve Ubuntu 24.04'te
  ölçüldü.
- **Ekran metinleri yüklenmemişken çıkan metin.** Tarayıcı ekran kataloğunu
  yüklememişse, sunucunun durmuş bir hizmet için kendi yedek cümlesi hâlâ
  hizmetin çalışmayı sürdürdüğünü söyler.
- **Güncellemeler yayımlanmış arşivlerden değil, yeniden derlemelerden
  ölçüldü.** v0.1.0-alpha.81 ve alpha.82, etiketlerinin kaynağından test
  lisanslı derleme olarak yeniden derlendi; özetleri yayımlanmış arşivlerinkinden
  farklıdır. alpha.82'den güncelleme üç platformda, alpha.81'den güncelleme on
  ikinci kayıtta Debian 13'te, onuncuda üç platformda, başarısız olan bir
  alpha.81 güncellemesi de on birincide Debian 13'te koştu. Kurulu bir
  sunucunun kendi arşivi ve verisi kullanılmadı. Makinelerin dışarıya NAT'ı vardı
  (paketler ve Node.js dışarıdan geldi; bu trafik kaydedilmedi).
- **Sertifikası olan hiçbir site ölçülmedi ve hiçbir sertifika alınmadı.**
  Doğrulama denetimi gerçek bir nginx'e `127.0.0.1` üzerinden (on birinci kayıt)
  ve bir sitenin kendi adresi ile `::1` üzerinden (on ikinci kayıt) soruldu;
  hiçbir sertifika otoritesi işe katılmadı: istekler makinenin kendi
  sabitlenmiş adına gitti ve certbot onları reddetti. Çalışmayan bir denetimin
  başarı yanıtındaki alanlar (`validation_probe: "not_run"` ve yanındakiler)
  gerçek bir sistemde hiç görülmedi, çünkü hiçbir istek başarılı olamazdı.
- **Doğrulama denetimi.** Yalnız korunan dosyanın kendi `listen` satırlarını
  okur (bir `include` izlenmez). Bu sunucunun olmayan bir adreste dinleyen bir
  ad buradan denetlenemez: istek yine de gönderilir ve sınama, sertifika
  otoritesinin kendi denemesidir. Reddedilen bir doğrulamayı türlü bir nedenle
  yalnız SSL sekmesindeki istek yanıtlar (`502 CERTIFICATE_ISSUE_FAILED`,
  `reason` `validation`, Agent certbot'un çıktısını böyle okuduğunda); takma
  adda yeniden alma, otoritenin başarısızlığında düz bir `409` yanıtlar,
  yenileme de başarısızlığını türlü bir neden olmadan kaydeder. Denetimin
  çalışmadığı yalnız Panel'in günlüğüne yazılır. Sayfanın
  okumasında, CelikPanel doğrulama dosyasını yayımlarken başarısız olan bir
  `nginx -t` ya da yeniden yükleme, neden başka bir dosya olabilecekken
  `challenge_failed` olarak bildirilir. Eksik ya da okunamayan bir dosya neden
  kaydetmez; bu yüzden onun durdurduğu bir yenileme, bir sonraki tamamlanan
  yenilemeye kadar bekler. Hazır bulunan bir denetim, nginx'in yolu sorulan
  adreslerde sunucunun kendisinden sunduğunu gösterir; DNS'in ya da bir güvenlik
  duvarının sertifika otoritesini içeri aldığını göstermez. Bekleyen bir
  sertifikanın pano girdisinin, neden kapanınca kaybolduğu ölçülmedi. İlk istek hiç yeniden denenmez; yenileme
  yalnız zamanlamayla yeniden denenir (her 12 saatte bir ve başlangıçta,
  süresinin dolmasına 30 günden az kalmış sertifikalar için). Sayfalar gerçek bir
  barındırma müşterisinin oturumunda çizilmedi.
- **Önceki bir sürüme dönüşte koruma kaybolur** ("Önceki bir sürüme dönüş"
  bölümüne bakın). İlk satırı olmayan dosyalar için ölçüldü (alpha.81 iki
  kayıtta, alpha.82 üçüncüde); olanlar için önceki sürümün kaynağından okundu.
- **İlk başlangıç, barındırılan siteleri yazar.** Bu sürüm ilk başlangıcında,
  bir önceki sürümün kendi metni olarak tanıdığı her barındırılan sitenin nginx
  dosyasını yazar (devralma; bir nginx denetimi ve yeniden yüklemesiyle); yalnız
  bilinen her metinden farklı bir dosya olduğu gibi bırakılır. Bu yazım
  laboratuvarda yeniden derlenmiş baz sürümlerde ölçüldü, kurulu bir sunucunun
  dosyalarında hiç ölçülmedi.
- **Bir sitenin silinmesi.** CelikPanel'in kendi include dizininin
  (`celikpanel-managed.d`) silmede kaldırılmasının yalnız bileşen testleri var:
  silme ölçümleri tarihli kopyaları ve sizin dizininizi korudu, o dizine
  bakmadı.
- **Havuz ve birim.** PHP sürümü değişikliği hiç ölçülmedi (her laboratuvar
  makinesinde tek PHP sürümü var); onun kendi geri koymasının kaldırma ve
  temizleme adımları hâlâ denetimsiz yeniden yükler. Laboratuvarda bir havuz ya
  da birim üzerinde yalnız "CelikPanel'in metnini al" çalıştırıldı (bir havuz
  kaydının ve bir barındırma değişikliğinin retleriyle); o iki dosya için
  "Benimkini koru", "Yeniden oluştur" ve "CelikPanel'in metnini şimdi yaz"ın
  yalnız bileşen testleri var. Panel başlarken bunlara
  bakılmaz; başlangıç satırı yalnız nginx dosyalarını sayar. Site oluşturma onlar için kayıt satırı açmaz; bu
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
- **Henüz dönüştürülmeyen ekranlar.** Arayüzün 30 kaynak dosyası (88 yer;
  v0.1.0-alpha.82'de 31 dosya) sunucuyu en az bir yerde hâlâ eski biçimde okuyor:
  uygulama kabuğu ve oturum okuması, oturum açma sayfası, kurulum sihirbazı,
  güncelleme kartı ile güncelleme ve bileşen işlemi katmanları, Hizmetler
  listesi ve hizmet sayfaları, kenar çubuğu, panonun bazı bölümleri, eklentiler,
  denetim günlüğü, VPN, ekip üyeleri, lisans sayfası ve birkaç küçük bölüm.
  Bunlarda başarısız bir okuma hâlâ boş ya da olumsuz bir durum gibi
  görünebilir.
- **Erişim beklemesi.** Her sayfa yüklemesi oturumu hâlâ iki kez okur. HTTP
  önbelleği açık bir yükleme ölçülmedi. Panel durmuşken çevrimdışı sayfa
  yalnız kurtarma hizmet çalışanı olan laboratuvar tarayıcı bağlamlarında
  görüldü; tarayıcının sabitleme olmadan güvendiği bir sertifikayla hizmet
  çalışanı ölçülmedi. Kilitli dosyanın işareti ve Yapılandırma dosyası sayfası
  gerçek bir tarayıcıda görülmedi.
- **Güncelleme bildirimi.** Kartın "Current version" satırı, sayfa kendini
  yeniden yükleyene kadar önceki sürümde kalır (laboratuvarda gözlendi).
  Telefonda köşedeki bildirim, taklit API'ye karşı tarayıcıda kartın durum
  satırını örttü. Doğrulama aralığı laboratuvarda gerçek bir Panel'de gözlendi,
  kurulu bir sunucuda görülmedi. Bu sürüme
  güncelleme sırasında açık sayfa hâlâ v0.1.0-alpha.82'ninkidir.
- **Kurulum.** Posta kimliği nedeni hiç gerçek bir Agent'tan okunmadı. Eşleşen
  bir ters DNS adından sonra başarısız olan ileri sorgu aynı "bilinmiyor"
  ("kontrol edilemedi") olarak bildirilir; iki sorgu ayırt edilmez. Önceki bir sürümün Agent'ı boş bir ters DNS
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
    yapılandırma dosyasında kendi düzenlemeleriniz" sınırını bu sürüm nginx
    dosyası, havuz ve birim için, yukarıdaki ölçümlerin ve bu listedeki
    sınırların elverdiği ölçüde ele alır.
- **Kanıt kapsamı.** Tek bir dizüstü ana makine; imzalı arşiv değil, kabul testi
  lisanslı derlemeler; dışarıya NAT yapan ve trafiği yakalanmayan makineler;
  loopback'e sabitlenmiş adlar; bir kayıt yinelenen bir koşuyu adıyla
  vermedikçe her hücre bir kez koştu. Ana makine yedinciden on üçüncüye kadarki
  kayıtların bazı bölümlerinde "modern bekleme" kaydetti; disk izleyicileri 31
  saniyeden uzun boşluk göstermiyor. Geçmeyen denetimler her dizinin
  `checks-not-passed.txt` dosyasında sayılır.
- **Açık kabul işleri.** [Dayanıklılık sözleşmesinin](RESILIENCE-CONTRACT.tr.md)
  beş temel işinin tamamı kısmi kalır; bu sürüm hiçbirini kapatmaz. Kararlar:
  [DECISIONS](DECISIONS.tr.md) içinde D-021 (değişmedi), D-022, D-024, D-025,
  D-029, D-031.

## Yayımlama kaydı

Yayımlanmadı. Yayımlamadan önce açık olanlar:

1. Numara: v0.1.0-alpha.83, sahibin 11 Ekim 2026 kararı. Yapıldı.
2. Sürüm hazırlığı, `cmd/`, `internal/` ya da `web/` altında hiçbir dosyayı
   değiştirmeyen ayrı bir commit'te yapıldı: 82'den sonra sürüm sırası 83,
   kurucunun önyükleme sabitleri, README'nin sürüm satırları ve bunları
   sabitleyen sözleşme testleri. (Sahibin kararı 11 Ekim 2026 tarihlidir; o
   commit'in iletisi yanlışlıkla 12 Ekim 2026 der.) Yayımlamada sahip
   `CELIKPANEL_RELEASE_SEQUENCE` depo değişkenini etiketten önce 83 yapar.
   Gerçek arşivi etikette CI derler ve imzalar.
3. Yapıldı: on birinci, on ikinci ve on üçüncü kayıtların sonuçları girildi.
   Çekme isteğinin son ucu kendi CI koşusunu alır; 11 Ekim 2026'da 12:09
   UTC'ye kadarki koşular "Testler" altında anılır.
4. Birleştirmeden önce sahip, `main` dalının korumasındaki zorunlu denetimlerin
   adlarını değiştirir: sürekli tümleştirme artık bir test grubunu üçe böler.
   "Panel race — S, except the service tests" adı "Panel race — S, except the
   service, setup, site and startup tests" olur; "Panel race — Startup
   readiness" ve "Panel race — Site configuration" da zorunlu denetim olarak
   eklenir.
5. Satıcının yayın araçları ile iki tek seferlik kurtarma betiğinin arşivde
   kalıp kalmayacağı: sahibin kararı, bekliyor.
6. Sahip birleştirir, sürümü etiketler, altı imzalı dosyayı imzalı sürüm
   sözleşmesinde anlatıldığı gibi doğrular ve indirme portalını yayımlar.
7. Sürüm yayımlandıktan sonra, kurulu her sunucunun sahibi o sunucuyu panelin
   kendi güncelleme ekranından günceller. Bu, yukarıdaki her adımdan ayrı bir
   adımdır; onu başka kimse başlatmaz.

Bu sürümü yalnız CelikPanel'in güncelleme arayüzünden kurun. Yayımlamak kurulu
sunucuları güncellemez; bir sunucunun güncellemesini yalnız o sunucunun sahibi,
panelin kendi güncelleme ekranından başlatır.
