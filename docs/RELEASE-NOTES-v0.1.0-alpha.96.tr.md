# v0.1.0-alpha.96 (aday, yayımlanmadı)

[English](RELEASE-NOTES-v0.1.0-alpha.96.md)

Bu sürüm v0.1.0-alpha.95'in ardından gelir. Bir adaydır: etiketlenmedi ve
yayımlanmadı, hiçbir kurulu sunucu ona güncellenmedi. 2026-10-17'de,
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
