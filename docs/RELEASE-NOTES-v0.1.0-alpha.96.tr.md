# v0.1.0-alpha.96 (17 Ekim 2026'da yayımlandı)

[English](RELEASE-NOTES-v0.1.0-alpha.96.md)

Bu sürüm v0.1.0-alpha.95'in ardından gelir. 17 Ekim 2026'da yayımlandı (sonda
"Yayım kaydı"na bakın); hiçbir kurulu sunucu ona güncellenmedi. Numarayı, sahibin
13 Ekim 2026 tarihli kalıcı talimatıyla sürümü hazırlayan seçti. 2026-10-17'de,
`feat/alpha96-update-notice` dalında, alpha.95'in yayım kaydından sonraki `main`
üzerinden yazıldı (alpha.95 sürüm kaydı
`5f3cb1a2fa336c529740a20e9a543b8ac086dc8d`).

## Kim güncellemeli ve neden

Bu sürüm yayımlandığında alpha.95'i ya da daha eski bir sürümü çalıştıran herkes.
Sahip alpha.95'e güncellerken kendi sözleriyle şunu istedi: "belirli periodlarda
güncellemeleri kontrol etmeli ve güncelleme var diye uyarmalı. güncelleme konusunda
yardımcı olmalı." (D-035). Bu sürüm bunu yapar. Bir panel güncellemeleri ancak bu
sürümü çalıştırdığında arayabilir; bu yüzden alpha.96, hâlâ alpha.95 çalıştıran bir
panele kendini duyuramaz, ona ilk güncelleme eskisi gibi "Güncellemeleri denetle"
düğmesiyle bulunur. Bir sunucuyu yalnız sahibi, panelin kendi güncelleme ekranından
günceller.

## Ekranda ne değişir

**Panel kendiliğinden bakar.** Panel başladıktan yaklaşık 2 dakika sonra, sonra
her 12 saatte bir, panel celikpanel.net'e daha yeni imzalı bir sürüm olup
olmadığını sorar. Bu, "Güncellemeleri denetle" düğmesiyle aynı salt-okur
denetimdir; tarayıcı değil panel yapar. Hiçbir şey kurmaz, sunucuda hiçbir şeyi
değiştirmez. Bir denetim 40 saniye sonra kesilir. Başarısız bir denetimden sonra
bir sonraki deneme 1 saat, sonra 3 saat sonradır, sonra yine 12 saatlik döneme
dönülür; böylece geçici bir kesinti bir istek yağmuruna dönüşmez. Son bir saat
içinde bir güncelleme başlatıldıysa, sunucu başka bir değişikliğin sürdüğünü ya da
bu durumu doğrulayamadığını bildiriyorsa, sunucu kurulumu sunucuyu
değiştiriyorsa (bir DNS kaydı beklerken değil) ya da Agent yanıt vermiyorsa
denetim atlanır ve 1 saat sonra yeniden denenir; atlanma başarısızlık sayılmaz,
yalnız bellekte tutulur ve bilinen sonucun yerini asla almaz. Meşgul bir paket
yöneticisi onu atlatmaz. **Panel, bir güncelleme kabul edilmiş ya da sürüyorken
sonucu asla yazmaz:** yazmadan hemen önce hiçbir güncellemenin kabul edilmediğini
ve sunucunun süren bir değişiklik bildirmediğini yeniden okur; aksi hâlde sonucu
bir sonraki boşta geçişe kadar bellekte tutar, çünkü güncelleyicinin salt-okur
denetimi sırasında panel veritabanına yazmak o denetimi durdurur. Agent'a hiçbir
kilit tutulmadan sorulur; panelin kendi değişiklik kilidi yalnız kısa bir süreç
içi yeniden denetim ve yazmanın kendisi için tutulur.

**Sahip kapatabilir.** Ayarlar → CelikPanel güncellemeleri içinde "Güncellemeleri
otomatik denetle" vardır, varsayılan olarak açıktır: "Panel 12 saatte bir
celikpanel.net'e daha yeni imzalı bir sürüm olup olmadığını sorar. Hiçbir şey
kendiliğinden kurulmaz." Kapalı olması, zamanlamanın hiçbir ağ okuması
yapmaması demektir; "Güncellemeleri denetle" düğmesi yine çalışır.

**Yöneticilere yalnız bir sürüm varken, sakin bir dille haber verilir.**

- kenar çubuğunda sürüm satırının altında küçük bir bağlantı, "Güncelleme var ·
  v0.1.0-alpha.96"; Ayarlar → CelikPanel güncellemeleri'ni açar;
- Pano'nun en üstünde tek sessiz bir satır, "Yeni bir CelikPanel sürümü var:
  v0.1.0-alpha.96", "İncele ve güncelle" ve "Sonra hatırlat" ile (satır o tarayıcıda
  o sürüm için 24 saat gizli kalır; kenar çubuğu bağlantısı kalır). Satır "İlgi
  istiyor"un altında değildir, çünkü güncelleme bir sorun değildir;
- Ayarlar'da "CelikPanel güncellemeleri"nin yanında küçük bir nokta ve "Güncelleme
  var · v0.1.0-alpha.96".

Bulunan bir güncelleme için alarm rengi kullanılmaz. Başarısız, bilinmeyen ya da
atlanan bir otomatik denetim güncelleme kartının dışında hiçbir şey göstermez ve
daha önce bulunan bir sürüm, başarısız bir denetimden sonra da sunulmaya devam
eder. Yönetici olmayanlar bunların hiçbirini görmez ve okumaz.

**Güncelleme kartı güncellemede yardımcı olur.** Son sonucu bilerek açılır; bir
düğmeye basılmasını ve sürüm sunucusuna çağrı yapılmasını beklemez; tek bir durum
satırı vardır: "En son sürümü kullanıyorsunuz (2 saat önce otomatik olarak
denetlendi)." ya da "v0.1.0-alpha.96 sürümü var (2 saat önce bulundu)." Başarısız
bir otomatik denetimden sonra neyin bilindiğini ve neyin bilinmediğini söyler,
örneğin "Son otomatik denetim: 2 saat önce — sürüm sunucusundan (celikpanel.net)
doğrulanmış bir yanıt alınamadı." ve "Sonraki denetim 15:30 saatinde kendiliğinden
yapılır; isterseniz şimdi de denetleyebilirsiniz." Zamanında bitmeyen bir denetim
sonucunun bilinmediğini söyler; bu bir başarısızlık değildir. Atlanan bir denetim
nedenini söyler ("sunucuda başka bir değişiklik sürüyordu"). Otomatik denetim
kapalıyken: "Otomatik denetim kapalı. Şimdi bakmak için "Güncellemeleri denetle"ye
basın."

Güncelleme varken kart, o sürüme yapılan önceki bir deneme için en çok tek bir
bildirim gösterir (hiçbir şeyi değiştirmeden durduysa nötr; geri alınan ya da
başarısız olan bir deneme dikkat işaretini ve kendi sözlerini korur), sonra
"Güncellemeden önce": panel ve Agent'ı bir kez yaklaşık N saniyeliğine yeniden
başlar (son ölçülen yeniden başlatmadan alınır, en az 90); barındırılan web
siteleri, posta, DNS ve veritabanları çalışmaya devam eder; bu bir alfa
sürümüdür, bu yüzden önemli verilerin güncel bir yedeği önerilir (sade metin, sarı
kutu değil); hiçbir şey kendiliğinden başlamaz; her adımı sayfada izleyebilirsiniz.
Ardından hazır olma sonucu ("Sunucu güncellemeye hazır." onay işaretiyle; meşgul
bir sunucu dikkat kutusunu korur) ve tek dolu düğme, "v0.1.0-alpha.96 sürümüne
güncelle". "Güncellemeleri denetle" çerçevelidir. Sürüm, commit, platform, sıra,
paket boyutu ve SHA-256 daralmış bir "Teknik ayrıntılar" içindedir. Sahibin 17 Ekim
2026'da Frankfurt'tan gönderdiği ekranda iki sarı kutu, yeşil bir "İmzalı bir
güncelleme bulundu." ve Başlat düğmesinin altında aynısının ikinci bir satırı vardı;
hepsi gitti.

**Hiçbir şey kendiliğinden güncellenmez.** Başlatma değişmedi: panel keşfi
yeniden çalıştırır ve herhangi bir başlatmadan önce tam olarak aynı hedefi ister.
Güncellemeyi yalnız bir yönetici, panelin kendi güncelleme ekranından başlatır.

**Durma penceresi, salt-okur denetimde duran bir güncelleme hakkında daha çok şey
söyler.** Kurtarma aracının salt-okur denetiminin iki adımının, `mode` ve `owner`,
sözü yoktu ve "Neden: salt-okur bir denetim mevcut kurulumu doğrulayamadı."
cümlesine düşüyordu. Artık ne anlama geldiklerini söylüyorlar ("kurtarma aracından
bilmediği türde bir denetim istendi" ve "salt-okur denetim sunucunun yönetici (root)
yetkileriyle çalışmıyordu"). Sayfanın sözünün olmadığı başka her adım için,
sunucunun kendi ilk tanı satırı yalnız daralmış Ayrıntılar'ın içinde değil, genel
nedenin altında da gösterilir ("Sunucunun İngilizce günlük satırı: …"). Frankfurt'un
alpha.95 güncellemesi gibi (aşağıda) bir duruş bir dahaki sefere daha çok şey
söyleyecek demektir; bu metinler, o duruşun nedeni olduklarını iddia etmez.

**alpha.95'in dört açık işi kapandı.**

1. Bir bileşen paketinin Doğrula'sı artık yalnız bir bileşen işlemi sürerken değil,
   sunucu herhangi bir nedenle hazır değilken de (meşgul bir ana makine kilidi,
   çalışan bir paket yöneticisi, doğrulanamayan bir durum) sunucunun kendi hazır
   olma nedeniyle tutulur; sayfanın hazır olma yeniden okuması sunucunun hazır
   olduğunu söylediğinde kendiliğinden geri gelir. Bu, alpha.95'in Doğrula'nın
   meşgul bir ana makine kilidinde ya da paket yöneticisinde kullanılabilir
   kaldığına dair notunu tersine çevirir.
2. Roundcube temizleme önerisi, Panel'in en yeni Roundcube kurulum ya da kaldırma
   girdisi ile posta profilinin son denemesinden yenisini izler (eşit saniyede
   Roundcube girdisi karar verir); böylece sonraki temiz bir kaldırma, başarısız bir
   posta denemesinin açtığı öneriyi bitirir.
3. `/services?cleanup=roundcube`, onayı açtıktan sonra `?cleanup`'ı adresten
   kaldırır; böylece yeniden yükleme onayı yeniden açmaz.
4. Bir pencerenin altındaki düğmeler telefonlarda, 639 px ve daha dar ekranlarda
   en az 44 px yüksekliktedir (ve ortalanmıştır).

alpha.95'in incelemesinden sonra yeniden çalıştırılmayan tarayıcı ekran denetiminin
yerini bu adayda 21. grup alır ("Kanıt"a bakın).

**21. grubun ilk bakışında düzeltilenler (17 Ekim 2026).** Bir güncelleme
varken telefonda Ayarlar sayfası yaklaşık 730 px genişliğindeydi (CelikPanel
güncellemeleri ögesinin ekran okuyucu notu ögeye göre değil sayfaya göre
yerleşiyordu); Pano satırı sürümü "v0.1.0-" / "alpha.96" diye bölüyordu; koyu
temada seçili Ayarlar ögesinin simgesi soluk bir kare üzerinde beyazdı; telefonda
menü düğmesi 44 px'in altındaydı. Not artık kendi ögesinin içinde yerleşir; sürüm
satırda, kenar çubuğu bağlantısında ve Ayarlar notunda bölünmez; seçili simge ana
renk çiftini kullanır; menü düğmesi 44 px'tir. Türkçede elle denetimin her yerde
tek fiili var: "Güncellemeleri denetle" (düğme, "Güncellemeleri otomatik denetle",
"denetlendi" ve ilgili iletiler); düğme önceden "Güncellemeyi kontrol et" diyordu.

**Adayın bağımsız incelemesinin düzelttikleri (17 Ekim 2026).** Adayın ilk biçimi
atlanan bir denetimi panelin ayarlarına yazıyordu ve bir güncelleme kabul
edilirken bir sonuç yazabiliyordu; bu, güncelleyicinin salt-okur denetimini
durdurabilirdi; bir DNS kaydı bekleyen kurulumu çalışan kurulum sayıyordu;
zamanlayıcıyı yönetim açıldıktan sonra atıyordu. Artık: atlanma yalnız bellektedir
ve bilinen sonucun yanında söylenir ("Son otomatik denetim … atlandı"); sonuç yalnız
hiçbir güncelleme kabul edilmemiş ya da sürmüyorken yazılır; denetimi yalnız
sunucuyu değiştiren bir kurulum tutar; zamanlayıcı hazırlanır ve eski kayıt
yönetim açılmadan önce temizlenir; yeniden açılan denetim son tamamlanan
denetimden 12 saat sonra sürer; aynı sürümün başka commit'li yeniden derlemesi
Agent'ın imzalı yanıtına göre saklanır; durma penceresi, gösterilen satır
kesildiğinde (üç noktayla) ya da farklı olduğunda sunucunun tam iletisini
Ayrıntılar'da tutar; ertelenmiş Pano satırı bir kare bile çizilmez; anahtar,
ayarı okunmadan önce "açık" yerine "henüz bilinmiyor" der; kart yaşları görünürken
dakikada bir yeniler; Türkçe meşgul ve yeniden başlama iletileri "denetle" ve "bu
yaklaşık N saniye sürer" der.

## Ne değişmez

- Agent değişikliği yok (`cmd/agent`'a dokunulmadı), yeni Agent RPC'si ve yeni Agent
  iş türü yok. Güncelleyicinin değiştirme yolu değişmedi: `update.sh`, `get.sh`'ın
  davranışı, Agent'ın sistem güncelleme işçisi, geri alma ve kurtarma. Sürüm yalnız
  güncelleme keşfine (salt-okur) ve güncelleme ekranına dokunur. Bu yüzden sahibin
  16 Ekim 2026 talimatıyla gerçek sistem kaydı yapılmaz.
- Şema 44, geçiş yok. Saklanan keşif sonucu, Panel'in mevcut ayarlar tablosunda bir
  ayardır (son yeniden başlatma kaydının mekanizması).
- Mevcut `/api/v1/panel/update/check`, `/update/start` ve `/update/status` biçimlerini
  korur (denetim artık sonucunu da kaydeder). Yeni okuma
  `GET /api/v1/panel/update/discovery`'dir; ayrı bir yoldur, çünkü `/update/status`
  zaten vardır ve tek bir güncelleme isteğinin durumunu istek kimliğiyle yanıtlar;
  yöneticiler için okuma Agent çağrısı ve ağ okuması yapmaz. Aynı yola `PUT`
  anahtarı kaydeder.
- Barındırılan web siteleri, posta, DNS, veritabanları, zamanlanmış işler ve
  sertifika yenileme denetime bağlı değildir. Denetim sürüm sunucusuna ulaşamazsa
  sunucuda hiçbir şey değişmez.
- Kurulu panelleri yalnız sahipleri, panelin kendi güncelleme ekranından günceller.

## Bilinen açıklar

- **Kurulu bir sunucuda hiçbir şey ölçülmedi.** Zamanlanmış denetim gerçek bir
  Agent'a ya da gerçek sürüm sunucusuna karşı çalışmadı: Go testlerinde Agent
  taklittir ve tarayıcı turu loopback üzerinde bir taklit kullanır. Testlerden
  celikpanel.net'e, lisans hizmetlerine ya da bir sertifika otoritesine hiç
  bağlanılmadı.
- **Frankfurt'un alpha.95 güncellemesinin durma nedeni hâlâ bilinmiyor.**
  Güncelleme 17 Ekim 2026'da salt-okur denetiminde durdu ve sunucu alpha.94'te
  kaldı; yani kurulu hiçbir şey değişmedi. Durma penceresi yalnız genel nedeni
  gösterdi ve Ayrıntılar metni sahipten bekleniyor. Burada hiçbir neden tahmin
  edilmez ve bu sürümde eklenen metinlerin o neden olduğu iddia edilmez. Boston'un
  alpha.95 güncellemesine dair hiçbir şey kayıtlı değildir.
- **Her bileşen için kurulu paket sürümlerinin okunması açık kalır.** Panel bir
  sürümü yalnız PHP-FPM ve Node.js için okur; gerçek bir yanıt yeni bir Agent okuması
  ister, bu sürüm onu eklemez.
- Diskte kalan Roundcube dosyalarını Panel hâlâ görmez, kendi kaydına dayanır
  (alpha.95'ten değişmedi).
- **Türkçe ekran katalogları boyut tavanlarına yakın.** Sunucu ekranlarının Türkçe
  bölümünü 160,25 KiB sınırının altında tutmak için (sınır yükseltilmedi)
  güncelleme kartının nedenleri, "Güncellemeden önce" listesi ve anahtar metinleri
  genel ekran kataloğuna taşındı; kartın artık kullanmadığı üç metin kaldırıldı.
  Bundan sonra Türkçe sunucu bölümünde yaklaşık 0,3 KiB, genel Türkçe bölümde
  yaklaşık 1,1 KiB yer kalıyor; oraya eklenecek bir sonraki metin için katalog
  yeniden bölünmelidir.
- P0.1-P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez; aday kabul
  değildir.

## Kanıt

Aşağıdakilerin hepsi 2026-10-17'de bir geliştirme ortamında aday ağacında
çalıştırıldı; hiçbiri kurulu bir sunucuda çalıştırılmadı.

- Go: `./cmd/panel/` ve `./internal/...` inceleme düzeltmelerinden sonraki son ağaçta 7147 geçti, 0 başarısız, 43 atlandı (`-timeout 45m` ile; yalnız panel paketi WSL konuğunda yaklaşık 16 dakika sürer); `go vet ./cmd/panel/` ve gofmt temiz; keşif testleri `-race` altında da geçti (C derleyicisi olan `archlinux` WSL konuğunda Go 1.27; Debian konuğunda derleyici yok)
- Web: `npm test` son ağaçta 1623 geçti, 0 başarısız; `npm run build` paket bütçesi değiştirilmeden başarılı (Türkçe sunucu ekran bölümü 160,25 KiB'nin 159,96 KiB'ı, Türkçe genel ekran bölümü 160,25 KiB'nin 159,25 KiB'ı)
- Tarayıcı grubu 21 (taklit sunucu; 20. grubun yerini alır): altı senaryo (en son sürüm, güncelleme var, daha önce duran bir deneme, başarısız otomatik denetim, kapalı, "Sonra hatırlat" öncesi ve sonrası Pano satırı) İngilizce ve Türkçe, açık ve koyu temada, 1440 ve 390 px'te: 8 yapılandırma, 100 ekran görüntüsü. İlk bakış telefonda Ayarlar sayfasını yaklaşık 730 px genişliğinde, Pano satırında sürümü tireden bölünmüş, koyu temada seçili Ayarlar simgesini neredeyse görünmez, telefonda menü düğmesini 44 px'in altında ve elle denetim için iki ayrı Türkçe fiil buldu; hepsi düzeltildi (yukarıya bakın). Son derlemedeki ikinci bakış: yatay taşma yok, telefonda 44 px'ten küçük hedef yok, bu ekranlarda sarı ya da kırmızı yok, çevrilmemiş anahtar ve Türkçe sayfada İngilizce yok, sürüm bütün, tek dolu eylem düğmesi, başarısız denetimden sonra kart dışında güncelleme bildirimi yok. 21. grup inceleme düzeltmelerinden sonra yeniden çalıştırılmadı; düzeltmeleri bileşen ve bağlı testler kapsar
- Sürüm sözleşmeleri (sıra ilkesi 96, 95'ten sonra; indirme portalı; imzalı
  bildirim): sıra ilkesi (Git Bash) "95 -> 96" geçti; indirme portalı ve imzalı bildirim (WSL konuğu) geçti

Keşif döngüsünün Go testleri taklit bir Agent kullanır ve şunları kapsar: sapma
sınırları, 1, 3 ve 12 saatlik geri çekilme, atlanma nedenleri (atlanma ağ okuması
yapmaz ve yanıt vermeyen Agent bir başarısızlık değil bir atlanmadır), anahtar
kapalıyken denetim yapılmaması, kapanış bir denetimi yarıda keserse hiçbir kayıt
yazılmaması, otomatik bir denetimin daha önce bulunan sürümü koruması, elle
denetimin aynı kaydı yazması, yapı hedefe ulaşınca kaydın temizlenmesi ve Agent
çağrısı yapmayan yalnız yöneticilere açık okuma. Web sözleşme ve bağlanmış testleri
güncelleme kartını, bildirimleri ve İngilizce ile Türkçe metinleri kapsar. Bileşen,
bağlanmış ve taklit tarayıcı testleri gerçek sistem davranışını kanıtlamaz.

## Yayım kaydı

Aşağıdaki her şey 17 Ekim 2026 tarihlidir (saat dilimi belirtilmedikçe UTC).
Birleştirmeyi, etiketi, denetimleri, portal yayımını ve genel sitenin okunmasını
sürümü hazırlayan kişi yaptı, önceki sürümlerde olduğu gibi sahibin kalıcı
talimatı uyarınca; kurulu panel güncellemelerini yalnız sahip, panelin kendi
güncelleme ekranından başlatır. Yayım için sahibin talimatıyla gerçek sistem
kaydı yok. Bu girdi, aşağıdaki sonuçların hiçbirini depoda bir dosya olarak
saklamaz.

**Ne oldu.**

1. Sürümden önce iki bağımsız inceleme yapıldı. Birincisi 12 sorun buldu;
   aralarında, zamanlanmış denetimin bir güncellemenin salt-okur denetimi
   sırasında panel veritabanına yazması vardı (düzeltildi, yukarıda anlatıldı);
   ikincisi sürümü yayıma hazır buldu ve önerdiği iki temizlik yapıldı.
2. "Release v0.1.0-alpha.96: the panel looks for updates by itself, says so
   calmly, and helps with the update" başlıklı #230 numaralı çekme isteği
   `main`'e `08faa91e5a8c6622072b40f0f87c9c388035c9a3` commit'i olarak
   birleştirildi. Sürüm sırası değişkeni etiket yapılmadan önce 95'ten 96'ya
   geçirildi. `main` koşusu ve etiket koşusu başarılı oldu.
3. Sürüm zincirinde iki olay oldu; hiçbiri bir sürümü etkilemedi. Birincisi: ilk
   CI koşusu iki sürüm-kurtarma deneme düzeneği testinde başarısız oldu; düzenek
   güncelleme kartının Türkçe metnini ve sonuç mantığını yansıtır, yeni "denetle"
   ifadesi ve değişen bir ifade orada yansıtılmamıştı. Bu dalda düzeltildi.
   İkincisi: bir yardımcının deneme düzeneği çalışması, ortak deponun git
   yapılandırmasında başıboş bir çalışma-ağacı ayarı bıraktı; bu, sürümü
   hazırlayanın makinesinde git'i kullanılmaz kıldı ve zincir zaman aşımına
   uğradı. O tek satır kaldırıldı ve zincir yeniden çalıştırıldı.
4. `v0.1.0-alpha.96` açıklamalı etiketi (nesne
   `61eb922045079c0828e4872ad90d1b76d39007c2`, ağaç
   `aa73450d100c10e2a8fccdc45862e8da0deb5a9e`) sürüm commit'ini gösterir. Sürüm
   bir ön sürümdür (pre-release); 2026-10-17T18:03:34Z'de altı dosyayla
   yayımlandı.
5. 17 Ekim 2026'da `CelikPanel-S2-Debian` WSL konuğunda bir indirmeden
   denetlendi (hazırlayan kişi; burada bir dosyada kayıtlı değil): arşivler bayt
   bayt aynı, her biri 66.291.958 bayt, SHA-256
   `eeb14706f7dc57f1824ff2f22f3794d0e65adbb7d456000e978f6b3ebc5fac02`; manifest
   sıra 96, `published_at` 2026-10-17T17:22:53Z der (SHA-256
   `fa03ccbcefd6255a2e6fe02d1c3517007e69127ec62cc084154d72c2d671b406`); imzası
   (SHA-256 `c45707199bc8bb7515abfe4218d2fb4217d3622f80b466abc9343eb8c979553a`),
   imza anahtarına (SHA-256
   `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`, öncekiyle
   aynı anahtar) karşı `openssl pkeyutl -verify -rawin -pubin` ile doğrulanır;
   iki içerik koruması da 0 döndürdü; derleme Go go1.26.5, CGO kapalı,
   `-trimpath` ile yapıldı. `libexec/get.sh` 96'yı sabitler (SHA-256
   `3e8d1aef71aaf114126fb0024ccb53e3d4d886eb06e6df231a9dfe3b6238c159`); içindeki
   politika şimdiki 96, önceki 95, önceki commit
   `5f3cb1a2fa336c529740a20e9a543b8ac086dc8d` der. Arşivde 243 üye var. Arayüz
   paketi `web/dist/assets/app-BNx0yp-r.js`, 256.533 bayt, SHA-256
   `3a551a4098d223914bbe8b586be835670905b72ba64066461ddb00868a960d5d`.
6. Portal paketi: `portal.tar.gz`, 132.959.242 bayt, SHA-256
   `890ef8f1c90b960b9935b5207ef4d5ec624d368b800b525e372fc1b80868b730`, 54 girdi;
   etiketin `deploy/build-download-portal.sh` betiğiyle (önceden imzalı kip) iki
   kez bayt bayt aynı derlendi. Sözleşme testi 0 döndürdü; genel doğrulayıcının
   17 testi ve terfi aracının 14 testi geçti.
7. Yaklaşık 18:10Z'de hazırlayan kişi onu kendi bilgisayarından
   `deploy/publish-download-portal.ps1` ile yayımladı: önceki sürüm
   v0.1.0-alpha.95, hedef v0.1.0-alpha.96; durum `committed`, genel doğrulama 26
   istekten 26'sı, `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`.
8. Önceki sitenin yedeği
   `/var/www/vhosts/celikpanel.net/portal-backups/httpdocs.before-v0.1.0-alpha.96.20261017T181008Z.854640.d00eee2822700c9f`;
   yüklenen paket
   `/var/www/vhosts/celikpanel.net/.upload-portal-20261017T180924Z-v0.1.0-alpha.96-e8a05cb81963c14f`
   altında saklandı.
9. Yayımdan sonra dışarıdan okuma: `https://celikpanel.net/releases/latest.txt`
   v0.1.0-alpha.96 der; `latest.json` sıra 96'yı, yukarıdaki commit ve SHA-256'yı
   gösterir.

**Sahibin bildirdikleri.** 17 Ekim 2026'da sahip Boston ve Frankfurt'un alpha.95'e
güncellemesini panelin kendi güncelleme ekranından başlattı. Frankfurt'un
güncellemesi hiçbir şeyi değiştirmeden salt-okur denetimde durdu (salt-okur bir
denetim mevcut kurulumu doğrulayamadı); sunucu alpha.94'te kaldı ve sahipten
Ayrıntılar metni istendi; bu yazılırken gelmemişti. Neden bilinmiyor ve burada
tahmin edilmez. Boston'un sonucu bildirilmemişti. Sahip ayrıca düzenli güncelleme
denetimi, bir güncelleme bildirimi ve güncellemede yardım istedi; bu sürüm o
işi yapar (yukarıdaki "Kim güncellemeli ve neden" ve "Ekranda ne değişir"
bölümlerine bakın; burada yinelenmez). Kurulu bir sunucuda alpha.96'dan henüz
hiçbir şey bildirilmedi ve çıkarılmamalıdır.

**Yapılmayanlar ve yayımlamanın değiştirmedikleri.**

- Kurulu sunucuları sürümü hazırlayan kişi alpha.96'ya güncellemedi. Bildirildiği
  kadarıyla Frankfurt alpha.94'tedir; Boston'un sonucu bildirilmedi. Onları yalnız
  sahip, panelin kendi güncelleme ekranından günceller.
- alpha.96, hâlâ alpha.95'te olan bir panele kendini duyuramaz: zamanlanmış
  denetim yalnız alpha.96'dan itibaren vardır. Bir panel alpha.96'ya geçene kadar
  sahibi güncellemeyi güncelleme ekranında kendisi aramalıdır.
- Frankfurt'un alpha.95 güncellemesinin durmasının nedeni hâlâ bilinmiyor
  (Ayrıntılar metni sahipten bekleniyor).
- Bu sürümün açık bıraktığı işler:
  1. Türkçe ekran katalogları neredeyse dolu; daha fazla metin eklenmeden
     bölünmelidir.
  2. Sunucu meşgulken bellekte tutulan elle denetim sonucu, Panel yeniden
     başlayınca kaybolur.
  3. Kurulu bir sunucuda ya da gerçek sürüm sunucusuna karşı hiçbir şey
     ölçülmedi.
  4. Tarayıcı 21. grubu son inceleme düzeltmelerinden sonra yeniden
     çalıştırılmadı.
- Her bileşen için kurulu paket sürümlerinin okunması açıktır ("Bilinen
  açıklar"a bakın).
- Bildirilmeyen ve kaydedilmeyenler: Boston ya da Frankfurt'tan alpha.96'ya dair
  her şey, Boston'un alpha.95 sonucu ve önceki notların bildirilmedi diye saydığı
  her şey. Her biri "kaydedilmedi"dir; "çalıştı" da değildir, "başarısız" da.
- Önceki yayımlama kayıtlarının sınırları ([alpha.88
  notları](RELEASE-NOTES-v0.1.0-alpha.88.tr.md)) kalkmış olarak bildirilmedi.
- Yayımlanma kabul değildir. P0.1–P0.5 kısmi kalır; hiçbir P0 işi kapanmaz ya da
  ilerlemez.

Bu sürümü yalnız CelikPanel'in güncelleme arayüzü üzerinden kurun. Yayımlamak kurulu
sunucuları güncellemez; bir sunucunun güncellemesini yalnız sahibi, panelin kendi
güncelleme ekranından başlatır.
