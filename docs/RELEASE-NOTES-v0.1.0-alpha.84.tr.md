# v0.1.0-alpha.84 (taslak, yayımlanmadı)

[English](RELEASE-NOTES-v0.1.0-alpha.84.md)

Bu, v0.1.0-alpha.83'ü izleyen sürümün taslağıdır. v0.1.0-alpha.84 numarası
dalın çalışma adıdır; numarayı yayımlama sırasında sahip belirler. Sürümün hiçbir
parçası yayımlanmadı; hiçbir kurulu sunucu onu çalıştırmıyor. Bu taslak 13 Ekim
2026'da yazıldı.

Sürüm üç şey yapar; üçü de Panel'in "Sistem servisleri" kartı ve arkasındaki
yanıtlar üzerindedir. Kart artık bileşenleri kendiliğinden kontrol eder; saklı
tarama beş dakikadan eskiyse "Bilinmiyor" deyip sizi başka bir sayfada bir
düğmeye basmaya bırakmaz ([D-032](DECISIONS.tr.md)). Sunucu artık bir taramanın
ne kadar eski olduğunu kendisi söyler; böylece tarayıcı kendi saatini sunucununkiyle
hiç karşılaştırmaz. Ve kendi sunucusundaki CelikPanel Agent'ına ulaşamayan bir
Panel, "iç hata" demek yerine bunu ve sonraki adımı söyler.

Kartın yaptığı, bileşen testlerine, API taklidine karşı çalışan bir tarayıcıya
ve tek bir laboratuvar ölçümüne dayanır (on dördüncü gerçek sistem kaydı:
Debian 13 ve Ubuntu 24.04, gerçek bir Panel'e karşı gerçek, başsız bir Chrome);
bu ölçüm üç ürün commit'inin yalnız ilkini ölçtü. Ölçümden sonra yapılan iki
düzeltmenin (saat kuralı ve sunucunun bildirdiği yaş) ve tipli Agent yanıtının
yalnız bileşen testleri var. Onları tek platformda ölçen kısa bir laboratuvar
turu sürüyor; buraya girilmedi ("Ne ölçüldü, nasıl ölçüldü" bölümüne bakın).
Hiç çalıştırılmayanları sınırlar bölümü söyler.

## Sunucu sahibi için ne değişiyor

**Güncellemeden sonra açık olan her CelikPanel sayfasını yeniden yükleyin.**
Güncellemeden önce yüklenmiş bir sayfada hâlâ eski kart vardır.

**Panel kendiliğinden bakar.** Bugüne kadar "Sistem servisleri" kartı, saklı
tarama beş dakikadan eskiyse "Bilinmiyor. Bileşen durumu eski. Yeniden taramak
için Bileşenleri açın." diyordu ve Bileşenler sayfasında "Yeniden tara"
düğmesine basmanızı istiyordu. Artık Panel, Bileşenler sayfasının çalıştırdığı
aynı önbellekli, koşullu taramayı çalıştırır (sunucuda aynı anda tek tarama,
önbellekli, bir değişiklik sunucunun kilidini tutarken reddedilen), maliyetini
küçük tutan kurallarla:

- Başarılı bir okumadan sonra, saklı tarama yoksa ya da 300 saniyeden eskiyse
  kontrol eder; yanıt ne olursa olsun beş dakikalık pencere başına en çok bir
  kez.
- Yalnız sekme görünürken kontrol eder. Gizli bir sekme hiçbir şey çalıştırmaz;
  yeniden gösterilen sekme, kontrol zamanı gelmişse bir kez kontrol eder.
- Biri sürerken ikincisini başlatmaz; aralıkla, ne olursa olsun tarayan bir
  zamanlayıcı yoktur. Reddedilmeden sonra yeniden deneme döngüsü yoktur.
- Sunucuda bir değişiklik sürüyorsa (`server_setup_busy` ya da
  `service_operation_busy` kodlu bir ret), kart bunu söyler ve başka bir şey
  göndermez. Sayfanın mevcut hazırlık yoklaması sunucunun meşgulden hazıra
  geçtiğini görünce bir takip kontrolü çalışır (yalnız paket işlemleri için;
  kurulum retinden sonra bir sonraki kontrol pencereyi bekler).
- Sizin "Servisleri tara" düğmesine basmanız koşulsuz kalır: her zaman tarar.

Kart dört ayrı durum gösterir: "Bileşenler kontrol ediliyor…", sonuç, yerinde
"Servisleri tara" düğmesiyle "Kontrol edilemedi: <neden>" ve "Bir değişiklik
sürüyor; durum, değişiklik bitince gösterilir." "İlgi istiyor" cümlesi
bunları izler. "Son 5 dakikada kontrol edilmedi. Bu sayfa açık kaldıkça yeniden
kendiliğinden kontrol eder." artık sizi bekleyen bir durum değil, yaşlanmış bir
sonuçla sonraki kontrol arasındaki kısa bir durumdur.

Maliyeti: bir kontrol bir koşullu taramadır. İki laboratuvar konuğunda ölçüldüğü
üzere bir kontrol Agent'a yaklaşık 0,2 ile 0,4 saniye CPU süresine mal oldu (47
saniyelik iki pencereden bir tahmin; "Ne ölçüldü, nasıl ölçüldü" bölümüne
bakın). Açık ve görünür bir Dashboard için beş dakikada en çok bir kontrol;
gizli sekme için hiç.

**Taramanın yaşı sunucunun beyanıdır.** Üç yönetilen-servisler yanıtı
`scan_age_seconds` taşır: yanıt yazılırken taramanın sunucu saatiyle yaşı.
Dashboard ve Bileşenler sayfası taramayı, yanıtı aldıkları an eksi bu yaş olarak
tarihler; böylece hiçbiri sunucunun saatini tarayıcınınkiyle karşılaştırmaz. Bu,
laboratuvarın ilk sürümde bulduğu bir kusuru kapatır (aşağıda). Alan yoksa (eski
bir sunucu) sayfa önceki adımın kuralına döner.

**Ulaşılamayan Agent adıyla söylenir.** Panel, Agent'ın yerel soketini
açamadığında (dinleyen yok ya da soket dosyası yok) `AGENT_UNREACHABLE` koduyla
`503` yanıtlar ve şunu söyler: nedeni, hiçbir şeyin değiştirilmediğini, sunucu
sahibinin Agent'ı sunucuda `sudo systemctl status celikpanel-agent` ile
denetlediğini (günlüğü: `sudo journalctl -u celikpanel-agent`) ve durmuşsa `sudo
systemctl start celikpanel-agent` ile başlattığını, Agent çalışınca yeniden
denemenizi. Bu, kartta ve reddedilen bir kabulün yanıtını gösteren her ekranda
"Sunucuda bir iç hata oluştu"nun yerini alır. Yalnız başarısız bağlantı kurma
sayılır. İstek gönderildikten sonra kopan bağlantı bilinmeyen sonuçtur ve genel
yanıtta kalır.

**Değişmeyenler.** Dashboard'daki posta yığını kartının hâlâ eski bayat metnin
kendi kopyası ve "Yeniden taramak için Bileşenleri aç" düğmesi var (açık;
sınırlara bakın). Bileşenler sayfası eskisi gibi davranır. Veritabanı şeması değişmedi, yeni rota
yok.

### Laboratuvarın bulduğu kusur ve onu düzelten iki adım

On dördüncü kayıt kartın ilk sürümünü ölçtü. Konuğun saati tarayıcınınkinden bir
iki saniye ileride olduğunda (1,05 ile 2,16 sn ölçüldü), kart her kontrolden
sonra 20 ile 29 saniye "Son 5 dakikada kontrol edilmedi" gösterdi; çünkü sayfa
kendi saatinden sonraki bir tarama damgasını reddediyordu ve yalnız 30 saniyelik
tiki o damgayı geçiyordu (o kaydın D1 kusuru).

Birinci adım: sayfa, tarama damgası taşıyan her yanıtı aldığı anı kaydeder ve
taramanın zamanı olarak damga ile o anın erken olanını alır. İkinci adım
(yukarıda): sunucu yaşı bildirir. Bu, birinci adımın bıraktığı sınırı da kaldırır:
sunucu saati tarayıcınınkinden beş dakikadan fazla geride olduğunda taze tarama
hâlâ bayat okunuyordu.

## API değişiklikleri

Yeni rota yok. Bir yeni alan ve bir yeni hata kodu.

### `scan_age_seconds`

Saklı taramayı taşıyan üç yanıtta: `GET /api/v1/managed-services`, `POST
/api/v1/managed-services/scan?max_age_seconds=N` rotasının önbellekten yanıtı ve
o rotanın yeni taraması.

```json
{
  "scanned_at": "2026-10-12T21:41:48Z",
  "scan_age_seconds": 46.7,
  "services": [ ... ],
  "profiles": [ ... ],
  "dns_identity_ready": true,
  "mail_hostname": { ... }
}
```

(Değerler örnektir; `...` değişmeyen alanların yerini tutar.)

`scan_age_seconds`, yanıt yazılırken taramanın sunucu saatiyle yaşıdır,
milisaniye hassasiyetiyle. Hiç tarama koşmamışsa `null`dır (`scanned_at` da
`null`). Saklı damga tam saniye tutar; bu yüzden yaş, taramadan bir saniyeye
kadar daha eski okunabilir. Eksi değer, damganın sunucunun saatinden sonra
olduğu anlamına gelir (saat geri gitti); koşullu tarama gibi sayfa da onu taze
saymaz. Alanı tanımayan bir istemci onu yok sayar. Alanı bozuk bulan bir sayfa
ona güvenmez.

### `503 AGENT_UNREACHABLE`

```json
{
  "error": "The CelikPanel Agent on this server is not reachable, so this request was not carried out and nothing was changed. The server owner checks the Agent on the server with sudo systemctl status celikpanel-agent (its log: sudo journalctl -u celikpanel-agent) and, if it is stopped, starts it with sudo systemctl start celikpanel-agent. Once the Agent is running, try again.",
  "code": "AGENT_UNREACHABLE"
}
```

Gövdedeki `error` cümlesi İngilizcedir. Ekranlar `code`a göre kendi çevirilerini
gösterir; Türkçesi: "Bu sunucudaki CelikPanel Agent’ına ulaşılamıyor; bu yüzden
istek yapılmadı ve hiçbir şey değiştirilmedi. Sunucunun sahibi Agent’ı sunucuda
sudo systemctl status celikpanel-agent komutuyla denetler (günlüğü: sudo
journalctl -u celikpanel-agent); durmuşsa sudo systemctl start
celikpanel-agent ile başlatır. Agent çalışınca yeniden deneyin." Uygulandığı
yerler:

- Tarama işleyicisi (`POST /api/v1/managed-services/scan`), Panel Agent'ın
  soketini açamadığında.
- On bir değiştirici işleyicinin bir şeyi değiştirmeden önce çalıştırdığı ortak
  kabul denetimi. Reddedilen bir kabul artık bunu söyler; kabul her değişiklikten
  önce çalışır, yani hiçbir şey değişmemiştir.

Yanıt bir soket yolu taşımaz; neden Panel'in günlüğüne yazılır. Yalnız Agent'ın
Unix soketine başarısız bağlantı kurma sayılır (dinleyen yok ya da soket
dosyası yok). İstek gönderildikten sonra kopan bağlantı genel yolda kalır ve
bilinmeyen sonuçtur. Sıradan bir tarama hatası yine `500` yanıtlar.

## Fark edeceğiniz davranış değişiklikleri

- Dashboard'ın "Sistem servisleri" kartı artık sizi beklemez. Beş dakikadan
  eski saklı tarama, siz baktığınızda bir kez kontrol edilir; sekme görünür
  kaldıkça sonraki beş dakikalık pencerede yeniden.
- Kontrolden hemen sonra kart sonucu bir anda gösterir. Sunucunun saati
  tarayıcınınkinden ileride olduğunda artık 30 saniyeye kadar "Son 5 dakikada
  kontrol edilmedi"ye düşmez.
- Saati tarayıcınınkinden beş dakikadan fazla geride olan bir sunucu artık taze
  taramayı bayat okutmaz.
- Agent durmuşken reddedilen bir tarama ya da değişiklik, iç hata yerine
  Agent'a ulaşılamadığını ve ne çalıştırılacağını İngilizce ve Türkçe söyler.
- Kartın metinleri İngilizce ve Türkçedir; `dashboard.systemServicesScanStale`
  kaldırıldı.

## Ne ölçüldü, nasıl ölçüldü

**Bileşen testleri.** Son ürün commit'inde: `gofmt`, `go vet` ve arm64 derlemesi
temiz; `go test ./cmd/panel/... ./internal/...` 7025 geçti, 44 atlandı, 0
başarısız (7 yeni: her yanıttaki alan, yalnız başarısız bağlantı kurmayı kabul
eden yardımcı, gövdede soket metni olmadan kabulde `503`, kabulden sonra Agent
düşünce `503`, sıradan tarama hatasının hâlâ `500` olması); `npm test` 1336 /
1336 (o commit'te 8 yeni; aralarında: damgası yedi dakika geride olan 10 sn'lik
yaş sonucu hemen gösterir ve hiçbir şey göndermez; 400 sn'lik yaş tam bir
koşullu tarama gönderir; bozuk yaşa güvenilmez; `503` nedenle "kontrol
edilemedi" gösterir ve tekrarlanmaz); `npm run build` bütçe içinde (Dashboard
parçası 160,25 KiB'tan 37,67 KiB). Web test sayısı ilk commit'ten sonra 1324,
saat düzeltmesinden sonra 1328'di.

**API taklidine karşı tarayıcı.** API'nin loopback taklidine karşı başsız bir
Chrome, dört durumu İngilizce masaüstünde ve Türkçe telefonda çizdi; her yükleme
başına tam bir `POST …/scan?max_age_seconds=300` (ilk commit).

**On dördüncü gerçek sistem kaydı**
([set14](../deploy/e2e/release-recovery/evidence/set14-20261012/README.md),
12 Ekim 2026 UTC). İlk ürün commit'inin test lisanslı bir derlemesi, iki geçici
konukta, ana makinede gerçek, başsız bir Chrome 154 ile: İngilizce masaüstü
pencereli (1440 × 900) Debian 13, Türkçe telefon pencereli (390 × 844) Ubuntu
24.04. Konukların saatleri ana makinenin saatinden 1,04 ile 1,62 sn (Debian) ve
1,23 ile 2,15 sn (Ubuntu) ileriydi. Her istek durumu ve zamanıyla, kartın metni
de her 100 ms'de sayfanın görünürlüğüyle kaydedildi. Sonuçlar:

- Bayat bir yükleme (saklı tarama 317,4 sn ve 319,2 sn eski) tam bir koşullu
  tarama gönderdi (`max_age_seconds=300`, 200, 27 servis; laboratuvarın SSH
  yönlendirmesi üzerinden Debian'da 1,34 sn, Ubuntu'da 4,45 sn). Taze bir
  yükleme (saklı tarama yaklaşık 45 sn eski) hiç göndermedi.
- Bir taramanın Agent CPU'su, her biri yaklaşık 47 sn'lik bayat ve taze bir
  yüklemenin farkı olarak: yaklaşık 189 ms (Debian) ve yaklaşık 417 ms
  (Ubuntu), alt süreçleri dahil. Bir tahmindir, tek bir taramanın doğrudan
  ölçümü değil.
- Görünür tutulan bir sayfa, 282 sn ve 284 sn boyunca tam bir koşullu tarama
  daha gönderdi; saklı tarama 300 sn'yi geçtikten sonra sayfanın ilk 30
  saniyelik tikinde (Debian'da 317,4 sn; Ubuntu'da yaklaşık 314 sn): 300
  sn'den önce değil.
- Gizli bir sekme (tarayıcının kendi gizli durumu, 303,9 sn ve 301,8 sn)
  gizliyken hiç tarama göndermedi; sayfa görünür olduktan 13 ms ve 34 ms sonra
  bir tarama gönderdi, saklı tarama o sırada yaklaşık 61 sn ve 64 sn gecikmişti.
- Yavaş bir nginx yeniden yüklemesi sunucunun kilidini tutarken (sahibin
  Panel'den yaptığı yeniden yükleme, bir laboratuvar ayarıyla yaklaşık 90 sn'ye
  uzatıldı) yapılan yükleme 12 ms ve 8 ms'de `409 service_operation_busy`
  aldı, 89,9 sn "bir değişiklik sürüyor" gösterdi ve hazırlık yoklaması
  değişikliği izleyene kadar başka bir şey göndermedi; sonra önbellekten bir
  takip (200, 4 ms ve 45 ms).
- Agent durmuşken tek yükleme bir tarama gönderdi ve `500 INTERNAL` aldı
  (`503 AGENT_UNREACHABLE`'ın artık yerini aldığı yanıt); kart 57,1 sn ve 56,0
  sn düğmeyle "Kontrol edilemedi" gösterdi; Agent yeniden başlatıldıktan sonra
  bir basış bir koşulsuz tarama gönderdi (200, 1,34 sn ve 2,34 sn) ve kart
  sonucu gösterdi.
- Bileşenler sayfası yükleme başına yine tek koşullu tarama gönderdi.
- Her kart durumu İngilizce (Debian) ve Türkçe (Ubuntu) görüntülendi.
- Yukarıdaki kusur: konuk başına dört kontrolün her birinden sonra kart "Son 5
  dakikada kontrol edilmedi"yi 28,6, 28,3, 25,9 ve 1,5 sn (Debian) ile 25,5,
  26,9, 20,0 ve 1,5 sn (Ubuntu) gösterdi.

Kayıt kendi sınırlarını söyler: hazırlık yanıtlarının gövdeleri kaydedilmedi
(yalnız durum kodları), bu yüzden "meşgul, sonra hazır" koddan ve takibin
zamanlamasından okunur; iki Türkçe telefon görüntüsü kartın üstünden kesik.
Çalışma ürünün hiçbir şeyini değiştirmedi ve her `result.json` `native_evidence:
false` taşır.

**Bekleyen, ölçülmedi: on beşinci kayıt (set14b).** Tek platform, iki
düzeltmeden sonraki kodda D1 kusurunu ve O1 gözlemini yeniden ölçen kısa bir
tur: konuğun saati tarayıcınınkinden ileriyken kartın bir kontrolden sonra
hâlâ "Son 5 dakikada kontrol edilmedi"ye düşüp düşmediği ve Agent durmuşken
kartın ve yanıtın ne söylediği (`503 AGENT_UNREACHABLE`). **Sürüyor ve bu
yazılırken ölçülmedi. Sayıları, kabul denetiminden sonra buraya girilir. O güne
kadar bu notlardaki iki düzeltme ve tipli Agent yanıtı hakkındaki her ifade
yalnız bileşen testlerine dayanır.** [sayılar girilecek]

## Bu sürümün sınırları

Bunlar bilinir. Gizli kusur değildir. İlk grup bu sürümle yeni; ikincisi
v0.1.0-alpha.83'ün sınırlarının, bu sürümde hiçbiri ele alınmadığı için her
biri "hâlâ açık" diye işaretlenmiş kopyasıdır.

Bu sürümle yeni:

- **Arch ölçülmedi.** On dördüncü kayıt Debian 13 ve Ubuntu 24.04'te koştu.
  Debian laboratuvarının boşta duran Arch eşi ölçülmedi.
- **`server_setup_busy` ölçülmedi.** Kurulum tarayıcı penceresi açılmadan önce
  koştu; bu yüzden onun sırasında hiçbir Dashboard yüklemesi olmadı. Yalnız
  `service_operation_busy` görüldü.
- **Hiç kontrol edilmemiş durum ölçülmedi.** Kurulum zaten taramıştı; bu yüzden
  saklı tarama laboratuvarda hiç yok değildi.
- **Taramanın sunucu tarafındaki duvar süresi ölçülmedi.** Yalnız tarayıcının
  laboratuvar yönlendirmesi üzerinden istek süresi; önbellek yanıtları 4 ile 45
  ms, reddedilen yanıtlar aynı yönlendirmeden 8 ile 14 ms sürdü.
- **Tutulma durumuna yalnız bazı durumlarda ulaşılır.** Bir servis kurulumu ön
  denetiminde tarar; bu yüzden kurulum kilidi tutarken saklı tarama tazedir ve
  Dashboard bir şey sormaz. Tutulma durumu önce taramayan bir değişiklikte (bir
  servis yeniden yüklemesi kullanıldı) ya da pencereyi aşan bir değişiklikte
  görünür.
- **Dashboard'daki posta yığını kartının hâlâ kendi bayat kopyası** ve
  "Yeniden taramak için Bileşenleri aç" düğmesi var. Kendiliğinden kontrol
  etmez.
- **Kenar çubuğundaki yapı kimliği bir sözcüğün ortasından bölünüyor.** Sahip
  tarafından v0.1.0-alpha.83 güncellemesinden sonra gözlendi; düzeltilmedi.
- **İki düzeltme ve tipli Agent yanıtı gerçek bir sistemde ölçülmedi**, bu
  notların şu anki durumunda (bekleyen on beşinci kayda bakın). Birkaç dakika
  geride bir sunucu saati de `503` yanıtı da gerçek bir Panel'e karşı bir
  tarayıcıda görülmedi.
- **Bu adaya güncelleme ölçülmedi.** On dördüncü kaydın konukları,
  v0.1.0-alpha.83'ten bir güncelleme değil, geçici bir sabit derleme çalıştırdı.
  Sürüm hazırlığı (sürüm sırası numarası, önyükleme sabitleri, sürüm satırları)
  yapılmadı ve ölçülen hiçbir derlemede yok.
- **Kanıt kapsamı.** Tek dizüstü ana makine; imzalı arşiv değil, test lisanslı
  bir derleme; trafiği yakalanmayan, dışarıya NAT'lı konuklar; her hücre bir kez
  koştu; İngilizce Debian'da masaüstü penceresinde, Türkçe Ubuntu'da telefon
  penceresinde görüldü, iki dil iki platformda değil.
- **Agent kapalıyken öteki kartlar.** Agent durmuşken Dashboard'ın CPU, bellek ve
  disk kartları kehribar uyarıyla "—" gösterdi ve güvenlik duvarı okuması `500`
  yanıtladı. Bu, on dördüncü kaydın kabul denetiminde görüldü, daha fazla
  ölçülmedi ve burada değiştirilmedi.

Bu sürümün ele almadığı v0.1.0-alpha.83 sınırları (her biri hâlâ açık):

- **Son kod gerçek bir sistemde bütünüyle ölçülmedi.** (Hâlâ açık. Onuncu ile
  on üçüncü kayıtlar alpha.83'ü dört aşamada ölçtü; alpha.83'ün son
  değişikliklerinin yalnız bileşen testleri vardı.)
- **Arch** on üçüncü kayıtta yoktu: yeniden yüklemeden önceki denetim, okunamayan
  satır ve birimin `PATH`'i yalnız Debian 13 ve Ubuntu 24.04'te ölçüldü. (Hâlâ
  açık.)
- **Ekran metinleri yüklenmemişken çıkan metin.** Tarayıcı ekran kataloğunu
  yüklememişse, sunucunun durmuş bir hizmet için kendi yedek cümlesi hâlâ
  hizmetin çalışmayı sürdürdüğünü söyler. (Hâlâ açık.)
- **Güncellemeler yayımlanmış arşivlerden değil, yeniden derlemelerden
  ölçüldü.** v0.1.0-alpha.81 ve alpha.82, etiketlerinin kaynağından test lisanslı
  derleme olarak yeniden derlendi; özetleri yayımlanmış arşivlerinkinden
  farklıdır. Kurulu bir sunucunun kendi arşivi ve verisi kullanılmadı. (Hâlâ
  açık.)
- **Sertifikası olan hiçbir site ölçülmedi ve hiçbir sertifika alınmadı.**
  Çalışmayan bir doğrulama denetiminin başarı yanıtındaki alanlar gerçek bir
  sistemde hiç görülmedi. (Hâlâ açık.)
- **Doğrulama denetimi** yalnız korunan dosyanın `listen` satırlarını okur (bir
  `include` izlenmez); bu sunucunun olmayan bir adreste dinleyen bir ad buradan
  denetlenemez; reddedilen bir doğrulamayı türlü bir nedenle yalnız SSL
  sekmesindeki istek yanıtlar; sayfanın okumasında başarısız bir `nginx -t` ya da
  yeniden yükleme, neden başka bir dosya olsa da `challenge_failed` diye
  bildirilir; eksik ya da okunamayan bir dosya neden kaydetmez; hazır bir
  denetim DNS'in ya da güvenlik duvarının sertifika otoritesini içeri aldığını
  göstermez; bekleyen bir sertifikanın panel girdisinin neden bitince
  kalktığı ölçülmedi; ilk alma hiç yeniden denenmez, yenileme yalnız
  zamanlamayla; sayfalar gerçek bir barındırma müşterisinin oturumunda
  çizilmedi. (Hâlâ açık.)
- **Eski bir sürüme dönüş korumayı kaybeder** (site dosyası başlığının). Birinci
  satırı olmayan dosyalar için ölçüldü; olanlar için eski sürümün kaynağından
  okundu. (Hâlâ açık.)
- **İlk başlangıç barındırılan siteleri işler.** alpha.83 ilk başlangıcında,
  önceki bir sürümün kendi metni olarak tanıdığı her barındırılan sitenin nginx
  dosyasını yazar; yeniden derlenmiş taban çizgilerinde ölçüldü, kurulu bir
  sunucunun dosyalarında hiç. (Hâlâ açık.)
- **Bir sitenin silinmesi.** CelikPanel'in kendi include dizininin kaldırılmasının
  yalnız bileşen testleri var. (Hâlâ açık.)
- **Havuz ve birim.** Bir PHP sürümü değişikliği hiç ölçülmedi; bir havuz ya da
  birimde "Benimkini koru", "Yeniden oluştur" ve "CelikPanel'in metnini şimdi
  yaz"ın yalnız bileşen testleri var; havuz ve birimlere Panel başlarken
  bakılmaz; kimsenin açmadığı bir havuz ya da birim Alan adları listesinde
  değildir; çağrılar Panel'in derlemesine bağlıdır, başka bir derlemenin
  Agent'ı için yedek yoktur. (Hâlâ açık.)
- **nginx bütün siteleri birlikte yargılar.** Başka yerdeki bozuk bir sahip
  dosyası, bir site dosyasının her yazımını hâlâ başarısız kılar (yazılan
  dosyalar geri konur). (Hâlâ açık.)
- **Kapsanmayanlar:** Panel'in kendi sertifikası için nginx sitesi, ve web
  postası ile veritabanı araçlarının nginx dosyaları. (Hâlâ açık.)
- **Henüz dönüştürülmemiş ekranlar.** Arayüzün 30 kaynak dosyası (88 yer),
  aralarında uygulama kabuğu, oturum açma sayfası, kurulum sihirbazı, güncelleme
  kartı ve katmanları, Servisler listesi ve sayfaları, kenar çubuğu, dashboard'un
  bazı bölümleri, eklentiler, denetim günlüğü, VPN, ekip üyeleri ve lisans
  sayfası, en az bir yerde sunucuyu hâlâ eski biçimde okur. Bunlarda başarısız
  bir okuma hâlâ boş ya da olumsuz bir durum gibi görünebilir. (Hâlâ açık; sayı
  bu taslak için yeniden hesaplanmadı.)
- **Erişim beklemesi.** Her sayfa yüklemesi oturumu hâlâ iki kez okur; HTTP
  önbelleği açık bir yükleme ölçülmedi; çevrimdışı sayfa yalnız kurtarma
  çalışanı olan laboratuvar bağlamlarında görüldü; kilitli dosyanın rozeti ve
  Yapılandırma dosyası sayfası gerçek bir tarayıcıda görülmedi. (Hâlâ açık.)
- **Güncelleme bildirimi.** Kartın "Geçerli sürüm" satırı sayfa kendini yeniden
  yükleyene kadar önceki sürümde kalır; telefonda köşe bildirimi API taklidi
  tarayıcısında kartın durum satırını örttü; doğrulama penceresi kurulu bir
  sunucuda değil laboratuvarda gözlendi. (Hâlâ açık.)
- **Kurulum.** Posta kimliği nedeni gerçek bir Agent'tan hiç okunmadı; eşleşen
  bir ters DNS adından sonra başarısız bir ileri arama aynı bilinmeyen olarak
  bildirilir; önceki bir sürümün Agent'ı boş bir ters DNS adını hâlâ uyuşmazlık
  diye bildirir. (Hâlâ açık.)
- **Hâlâ geçerli olan v0.1.0-alpha.82 sınırları.** On iki rota istek kimliği
  ister; gerisi (servis ve uygulama yeniden başlatma, planlar, kayıt kodları,
  yaklaşık 38 başka rota, DNS kayıtları, barındırma türü, PHP ayarları, takma
  adlar, uygulama kurulumu, posta kimlik doğrulaması, hesaplar ve dosyalar)
  istemez; kurulu bir posta yenileme yardımcısı panel güncellemesiyle
  değiştirilmez; `postconf` uyarısının ardındaki posta sertifikası yolu bir
  sunucuda ölçülmedi; arşiv satıcının yayımlama araçlarını ve iki tek seferlik
  kurtarma betiğini taşır, kaldırılmaları hâlâ açık bir sahip kararıdır; satır
  başına bir kısıtlama yazılmış bir Postfix kısıtlama listesi bir DNSBL
  kaydından sonra tek satır olur; Panel'de bir veritabanı kullanıcısının
  parolasını belirleyen denetim yoktur; ve alpha.83'ün ele almadığı
  [v0.1.0-alpha.82](RELEASE-NOTES-v0.1.0-alpha.82.tr.md) sınırları. (Hâlâ açık.)
- **Açık kabul işleri.** [Dayanıklılık sözleşmesi](RESILIENCE-CONTRACT.tr.md)nin
  beş temel işinin hepsi kısmi kalır; bu sürüm hiçbirini kapatmaz. Bu aday hiçbir
  P0 işini kapatmaz ve ilerletmez. Kararlar: D-021 (değişmedi), D-022, D-024,
  D-025, D-029, D-031, D-032, [DECISIONS](DECISIONS.tr.md) içinde.

## Yayımlama kaydı

Yayımlanmadı. Yayımlamadan önce açık olanlar:

1. On beşinci kayıt (set14b): tek platform, son ürün kodunda D1 kusurunun ve O1
   gözleminin yeniden ölçümü. Sürüyor; sayıları, kabul denetimi bitince "Ne
   ölçüldü, nasıl ölçüldü" bölümüne girilir.
2. Ayrı bir commit'te yapılan ve `cmd/`, `internal/` ya da `web/` altında
   hiçbir dosyayı değiştirmeyen sürüm hazırlığı: öncekinden sonra gelen sürüm
   sırası numarası, kurucunun önyükleme sabitleri, README sürüm satırları ve
   onları sabitleyen sözleşme testleri. Yapılmadı. Yayımlamada sahip, etiketten
   önce `CELIKPANEL_RELEASE_SEQUENCE` depo değişkenini ayarlar. Gerçek arşivi
   etikette CI derler ve imzalar.
3. Numara: sahip belirler. v0.1.0-alpha.84 dalın çalışma adıdır.
4. Çekme isteğinin son ucu kendi CI koşusunu alır.
5. Sahip birleştirir, sürümü etiketler, imzalı sürüm sözleşmesinin anlattığı
   gibi altı imzalı dosyayı denetler ve indirme portalını yayımlar.
6. Sürüm yayımlandıktan sonra, kurulu her sunucunun sahibi o sunucuyu panelin
   kendi güncelleme ekranından günceller. Bu, yukarıdaki her adımdan ayrı bir
   adımdır; onu başka kimse başlatmaz.

Bu sürümü yalnızca CelikPanel'in güncelleme arayüzünden kurun. Yayımlanması
kurulu sunucuları güncellemez; bir sunucunun güncellemesini yalnız sahibi,
panelin kendi güncelleme ekranından başlatır.
