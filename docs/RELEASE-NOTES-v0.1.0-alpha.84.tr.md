# v0.1.0-alpha.84 (taslak, yayımlanmadı)

[English](RELEASE-NOTES-v0.1.0-alpha.84.md)

Bu, v0.1.0-alpha.83'ü izleyen sürümün taslağıdır. Sahip 13 Ekim 2026'da
numaranın v0.1.0-alpha.84 olduğuna karar verdi (commit 1636b73f0). Sürümün
hiçbir parçası yayımlanmadı; hiçbir kurulu sunucu onu çalıştırmıyor. Bu taslak
13 Ekim 2026'da yazıldı.

Sürüm, Panel'in "Sistem servisleri" kartı ve arkasındaki yanıtlar üzerinde üç
şey, kartın yanında iki şey daha yapar. Kart artık bileşenleri kendiliğinden
kontrol eder; saklı tarama beş dakikadan eskiyse "Bilinmiyor" deyip sizi başka
bir sayfada bir düğmeye basmaya bırakmaz ([D-032](DECISIONS.tr.md)). Sunucu
artık bir taramanın ne kadar eski olduğunu kendisi söyler; böylece tarayıcı
kendi saatini sunucununkiyle hiç karşılaştırmaz. Ve kendi sunucusundaki
CelikPanel Agent'ına ulaşamayan bir Panel, "iç hata" demek yerine bunu ve
sonraki adımı söyler.

Laboratuvarın yolda bulduğu ve düzeltilen bir kusur daha var: saati tarayıcının
saatinden yaklaşık bir dakikadan fazla geride olan bir sunucu, lisans denetimi
iki saati karşılaştırdığı için hiçbir Panel sayfasının açılmasına izin
vermiyordu. Sunucu artık kalan süreyi kendisi söyler. Kartın yanındaki ikinci
şey, Bileşenler sayfasının başarısız okuma şeridir: artık nedenini söyler.

Kartın yaptığı, bileşen testlerine, API taklidine karşı çalışan bir tarayıcıya
ve üç laboratuvar ölçümüne dayanır; üçü de gerçek bir Panel'e karşı gerçek,
başsız bir Chrome'dur: on dördüncü gerçek sistem kaydı (Debian 13 ve Ubuntu
24.04), ürün commit'lerinin yalnız ilkini ölçtü; on beşinci kayıt (set14b, yalnız
Debian 13, 13 Ekim 2026) ardından gelen iki düzeltmeyi (saat kuralı ve sunucunun
bildirdiği yaş) ve tarama ile kabul denetimindeki tipli Agent yanıtını ölçtü; ve
on altıncı kayıt (set14c, yalnız Debian 13, 13 Ekim 2026) son ürün commit'inde
lisans denetiminin düzeltmesini, güvenlik duvarı okumasında, sistem denetiminde
ve bir yeniden başlatma isteğinde tipli Agent yanıtını ve Bileşenler sayfasının
okuma şeridini ölçtü. Lisans denetimi kusuru on beşinci kayıtta laboratuvarda
bulundu ve on altıncıda yok ("Ne ölçüldü, nasıl ölçüldü" bölümüne bakın).
Hiç çalıştırılmayanları sınırlar bölümü söyler.

Bunlardan sonra on yedinci kayıt (set15, 13 Ekim 2026) yayımlama öncesi
ölçümdü: aday commit'in kendisi, Debian 13 ve Ubuntu 24.04'e temiz kuruldu,
Debian 13'te v0.1.0-alpha.83'ten güncellendi ve kendiliğinden geri dönen
kusurlu bir aday denendi. İki açık madde bıraktı (sınırlara bakın).

## Sunucu sahibi için ne değişiyor

**Güncellemeden sonra açık olan her CelikPanel sayfasını yeniden yükleyin.**
Güncellemeden önce yüklenmiş bir sayfada hâlâ eski kart vardır.

**Dashboard kendiliğinden bakar.** Bugüne kadar "Sistem servisleri" kartı, saklı
tarama beş dakikadan eskiyse "Bilinmiyor. Bileşen durumu eski. Yeniden taramak
için Bileşenleri açın." diyordu ve Bileşenler sayfasında "Yeniden tara"
düğmesine basmanızı istiyordu. Artık Panel, Bileşenler sayfasının çalıştırdığı
aynı önbellekli, koşullu taramayı çalıştırır (sunucuda aynı anda tek tarama,
önbellekli, bir değişiklik sunucunun kilidini tutarken reddedilen), maliyetini
küçük tutan kurallarla:

- Başarılı bir okumadan sonra, saklı tarama yoksa ya da 300 saniyeden eskiyse
  kontrol eder; yanıt ne olursa olsun beş dakikalık pencere başına en çok bir
  kez. Tek istisna: bir değişiklik sürdüğü için reddedilmeden sonra, hazırlık
  yoklaması sunucuyu hazır görünce bir takip kontrolü çalışır; bu, bir
  pencerenin içine bir kontrol daha ekleyebilir (`heldSawBusy` pencereyi
  sıfırlar).
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
istisna: bir değişiklik sürdüğü için reddedilmeden sonra hazırlık yoklaması
sunucuyu hazır görünce bir takip kontrolü çalışır ve bir pencerenin içine bir
kontrol daha ekleyebilir (`heldSawBusy` pencereyi sıfırlar); gizli sekme için
hiç.

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
"Sunucuda bir iç hata oluştu"nun yerini alır. Yanıt artık Panel'in ortak sunucu
hata yazıcısında tek yerde yazılır; bu yüzden o yazıcıya, Agent soketine başarısız
bağlantı kurmanın tek neden olduğu bir durumla ulaşan her işleyici için geçerlidir:
tarama ve kabul denetiminin yanında, Agent durmuşken hâlâ genel bir iç hata
yanıtlayan güvenlik duvarı okuması ve sistem denetimi için de. Birleşik bir hata
(örneğin bir hata ve onun telafisi) genel yanıtta kalır; çünkü isteğin önceki bir
parçası bir şeyi değiştirmiş olabilir ve "hiçbir şey değiştirilmedi" yanlış olur.
Kendi tipli yollarında kalanlar, değişmedi: DNSSEC durumu, güncelleme durumu ve
başlatması, alan adı silme ve posta parolası döndürme. Yalnız başarısız bağlantı
kurma sayılır. İstek gönderildikten sonra kopan bağlantı bilinmeyen sonuçtur ve
genel yanıtta kalır.

**Geride kalan sunucu saati artık Panel'in açılmasını engellemez.** Panel'in erişim
denetimi (`GET /api/v1/license/access`) erişimin sonunu sunucunun saatiyle bir an
olarak veriyordu ve sayfa, sonu tarayıcının kendi saatinden sonra olmayan olumlu
bir yanıtı reddediyordu. Saati tarayıcınınkinden yaklaşık bir dakikadan fazla
geride olan bir sunucu bu yüzden her yanıtı süresi dolmuş gösteriyordu: hiçbir
Panel sayfası açılmıyor, sayfa denetimi beş saniyede bir yeniden okuyordu
(laboratuvarda bulundu; "Ne ölçüldü, nasıl ölçüldü" bölümüne bakın). Şimdi sunucu
kalan süreyi kendisi söyler (`valid_for_seconds`) ve sayfa sonu, isteği gönderdiği
an artı bu süre olarak alır. İki saati hiç karşılaştırmaz: sunucu saatinin
tarayıcınınkinden geride ya da ileride olması bir şeyi değiştirmez ve son, sunucunun
izin verdiğinden geç çıkamaz. Mutlak zaman (`valid_until`) hâlâ gönderilir. Alan
yoksa (eski bir sunucu) eski kural sürer. Sessiz okuma, yeniden denetim ve 15 sn /
30 sn aşamaları değişmedi.

**Bileşenler sayfası, hiçbir işlem koşmamışken artık bir işlemin tamamlanmış
olabileceğini söylemez.** Agent durmuşken sayfanın otomatik denetimi başarısız
oluyor ve sayfa hiçbir işlem göndermemiş olduğu hâlde "Servis durumu doğrulanmalı"
ile "Bir sunucu işlemi tamamlanmış olabilir" gösteriyordu. Sayfa artık durumun
neden doğrulanmadığını saklar ve iki şeritten birini gösterir:

- Bu sayfadan bir bileşen işlemi gönderildi ve sonucu doğrulanamadı: şerit eskisidir.
  Başlık: "Servis durumu doğrulanmalı". İpucu: "Bir sunucu işlemi tamamlanmış olabilir
  ancak güncel durum doğrulanamadı. Başka bir bileşeni değiştirmeden önce Yeniden
  tara’yı çalıştırın." Sonradan başarısız olan bir okuma onun yerini asla almaz.
- İşlem gönderilmedi ve durum okunamadı (otomatik denetim, bir sayfa yüklemesi ya da
  Yeniden tara): şerit yenidir ve Yeniden tara düğmesi vardır. Başlık: "Bileşen
  durumu okunamadı". İpucu: "Bu sayfa hiçbir bileşen işlemi göndermedi;
  bileşenlerin güncel durumunu okuyamadı. {reason} Bir tarama durumu okuyana dek
  bileşen değişiklikleri kilitli kalır; Yeniden tara durumu yeniden okur."
  `{reason}`, başarısız olan yanıtın cümlesidir (Agent durmuşken yukarıdaki
  `AGENT_UNREACHABLE` cümlesi); yanıt gelmediyse "Servis taraması başarısız oldu."

Yeniden yüklemeden sonra yalnız nedenin türü hatırlanır, yanıt değil; bu yüzden
okuma şeridi o zaman "Servis taraması başarısız oldu." der. Bu değişiklikten önce
saklanmış bir kayıt eski sözü korur.

**Değişmeyenler.** Dashboard'daki posta yığını kartının hâlâ eski bayat metnin
kendi kopyası ve "Yeniden taramak için Bileşenleri aç" düğmesi var (açık;
sınırlara bakın). Bileşenler sayfası denetimlerini eskisi gibi yapar; yalnız başarısız
okuma şeridi değişir. Veritabanı şeması değişmedi, yeni rota yok.

### Laboratuvarın bulduğu kusur ve onu düzelten iki adım

On dördüncü kayıt kartın ilk sürümünü ölçtü. Konuğun saati tarayıcınınkinden bir
iki saniye ileride olduğunda (1,04 ile 2,15 sn ölçüldü), kart dört otomatik
kontrolün her birinden sonra (operatörün basışından 1,5 sn sonra) 20 ile 29
saniye "Son 5 dakikada kontrol edilmedi" gösterdi; çünkü sayfa kendi saatinden
sonraki bir tarama damgasını reddediyordu ve yalnız 30 saniyelik tiki o damgayı
geçiyordu (o kaydın D1 kusuru).

Birinci adım: sayfa, tarama damgası taşıyan her yanıtı aldığı anı kaydeder ve
taramanın zamanı olarak damga ile o anın erken olanını alır. İkinci adım
(yukarıda): sunucu yaşı bildirir. Bu, birinci adımın bıraktığı sınırı da kaldırır:
sunucu saati tarayıcınınkinden beş dakikadan fazla geride olduğunda taze tarama
hâlâ bayat okunuyordu.

## API değişiklikleri

Yeni rota yok. İki yeni alan ve bir yeni hata kodu.

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

### `valid_for_seconds`

`GET /api/v1/license/access` yanıtında, değişmeyen `valid_until`ın yanında.

```json
{
  "can_use_panel": true,
  "valid_until": 1791871840,
  "valid_for_seconds": 55.4,
  "state": "active",
  "observation": "known"
}
```

(Değerler örnektir.)

`valid_until`, erişimin sonudur: sunucunun saatiyle bir an (lisansın bitişi ile
çevrimdışı iznin erken olanı), eskisi gibi. `valid_for_seconds`, aynı sonun yanıt
yazılırken kalan süresidir, aynı saatle ölçülür: `valid_until` eksi sunucunun
saati, milisaniye hassasiyetiyle, sıfırın altına inmez (son yoksa ya da geçtiyse
0). Sayfa sonu, isteği gönderdiği an artı `valid_for_seconds` olarak alır;
`valid_until`ı kendi saatiyle karşılaştırmaz. O an sunucunun ölçümünden sonra
olamayacağı için son, sunucunun izin verdiğinden geç çıkamaz. Alanı bulamayan ya da
sonlu sayı olmayan bulan sayfa `valid_until`ı eskisi gibi kullanır. Kalan süresi 0
olan olumlu bir yanıt kabul edilmez. Alanı tanımayan bir istemci onu yok sayar.

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

- Panel'in ortak sunucu hata yazıcısı. Bir hatayı onun üzerinden bildiren her
  işleyici, Agent'ın Unix soketine başarısız bağlantı kurma (dinleyen yok ya da
  soket dosyası yok; hata sarılmış olabilir) tek neden olduğunda `503
  AGENT_UNREACHABLE` yanıtlar. Buna tarama işleyicisi (`POST
  /api/v1/managed-services/scan`), değiştirici işleyicilerin bir şeyi
  değiştirmeden önce çalıştırdığı ortak kabul denetimi (`beginServiceMutation`;
  68b79b372 commit'inde tarama işleyicisi dışında dokuz çağrı yeri; reddedilen
  kabul hiçbir şeyi değiştirmemiştir), güvenlik duvarı okuması (`GET
  /api/v1/firewall`) ve sistem denetimi dahildir. Sistem denetiminin öteki
  hataları artık düz metin yerine kodlu `INTERNAL` hatasını yanıtlar.
- Birleşik hata için değil. Örneğin bir hata ve onun telafisinden oluşan bir
  hata gerçekleşmiş bir değişiklik taşıyabilir; bu yüzden genel yolda kalır.
  Kabul edilen tek birleşim, platform kimliği işaretiyle nedenidir; çünkü o okuma
  koruduğu çağrıdan önce koşar.
- Kendi tipli yollarında kalanlar, değişmedi: DNSSEC durumu, güncelleme durumu ve
  başlatması, alan adı silme ve posta parolası döndürme.

Yanıt bir soket yolu taşımaz; neden Panel'in günlüğüne yazılır. İstek
gönderildikten sonra kopan bağlantı (sıfırlama, akış sonu) genel yolda kalır ve
bilinmeyen sonuçtur. Sıradan bir hata yine `500` yanıtlar.

## Fark edeceğiniz davranış değişiklikleri

- Dashboard'ın "Sistem servisleri" kartı artık sizi beklemez. Beş dakikadan
  eski saklı tarama, siz baktığınızda bir kez kontrol edilir; sekme görünür
  kaldıkça sonraki beş dakikalık pencerede yeniden.
- Kontrolden hemen sonra kart sonucu bir anda gösterir. Sunucunun saati
  tarayıcınınkinden ileride olduğunda artık 30 saniyeye kadar "Son 5 dakikada
  kontrol edilmedi"ye düşmez.
- Saati tarayıcınınkinden beş dakikadan fazla geride olan bir sunucu artık taze
  taramayı bayat okutmaz (bileşen testli; Debian 13'te bir kez ölçüldü, saat yedi
  dakika geriyken: tek koşullu tarama, sonra 90 sn'nin sonuna dek sonuç).
- Saati tarayıcınınkinden yaklaşık bir dakikadan fazla geride olan bir sunucu,
  hiçbir Panel sayfasının açılmasına izin vermiyordu. Şimdi sunucu kalan süreyi
  söyler ve tarayıcı iki saati hiç karşılaştırmaz (`/api/v1/license/access`
  yanıtında `valid_for_seconds`). Laboratuvarda bulundu; düzeltmeden sonra yalnız
  Debian 13'te ölçüldü: saat yedi dakika geriyken ve yedi dakika ileriyken
  Dashboard hemen açıldı.
- Agent durmuşken reddedilen bir tarama ya da değişiklik, güvenlik duvarı okuması
  ve sistem denetimi, iç hata yerine Agent'a ulaşılamadığını ve ne
  çalıştırılacağını İngilizce ve Türkçe söyler.
- Bileşenler sayfasında başarısız bir okuma, hiçbir sunucu işlemi gönderilmemişken
  bir işlemin tamamlanmış olabileceğini söylemek yerine durumun "okunamadığını" ve
  nedenini, Yeniden tara ile söyler. O söz, sonucu bilinmeyen bir işlem için kalır.
- Kartın metinleri İngilizce ve Türkçedir; `dashboard.systemServicesScanStale`
  kaldırıldı.

## Ne ölçüldü, nasıl ölçüldü

**Bileşen testleri.** `1a74eeda5`'te: `gofmt`, `go vet` ve arm64 derlemesi
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

`7024534bc`'de: `gofmt`, `go vet` ve arm64 derlemesi temiz;
`go test ./cmd/panel/... ./internal/...` 7028 geçti, 44 atlandı, 0 başarısız (3
yeni: her erişim durumunda kalan süre; ortak yazıcının reddedilen bağlantı, eksik
soket, sarılmış bağlantı ve üç yazıcının hepsinden geçen kimlik artı bağlantı
hatası için `503`, sıradan hata, sıfırlama, akış sonu, birleşik bağlantı ve
hata artı geri alma için `500` yanıtlaması; güvenlik duvarı ve sistem denetimi
okumalarının Agent'ı adıyla söylemesi); `npm test` 1339 / 1339 (3 yeni:
`valid_until`ı tarayıcının saatine göre yedi dakika geçmiş ama
`valid_for_seconds`ı 55 olan yanıt kabul edilir, 0 edilmez ve alan yokken eski
kural sürer; iki şerit); `npm run build` bütçe içinde. Ürün koduna dokunan
ondan sonraki commit, `68b79b372`, yalnız iki sınıf adını değiştirdi (anahtarlar,
metinler ve davranış değişmedi): `npm test` 1339 / 1339, `npm run build` bütçe
içinde. Gerçek bir sunucuda bir kez, yalnız Debian 13'te, `68b79b372` üzerinde
ölçüldü: aşağıdaki on altıncı kayda bakın.

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

**On beşinci gerçek sistem kaydı**
([set14b](../deploy/e2e/release-recovery/evidence/set14b-20261013/README.md),
13 Ekim 2026 UTC). `1a74eeda5`'in (saat kuralı, sunucunun bildirdiği yaş, tarama
ve kabul denetimindeki Agent yanıtı) test lisanslı bir derlemesi, tek bir geçici
Debian 13 konuğunda, ana makinede gerçek, başsız bir Chrome 154 ile: İngilizce
masaüstü penceresi (1440 × 900) ve bir Dashboard yüklemesi için Türkçe. Çalışma
düzeneği, on dördüncü kaydın sapmasını yeniden üretmek için konuğun saatini
kaydırdı; sapma yanıtların kendisinden okundu (her biri `scanned_at` ve
`scan_age_seconds` taşır; toplamları yanıtı yazarken sunucunun saatidir), yani
isteğin süresi kadar bir aralıkla sınırlıdır. Sonuçlar:

- Konuğun saati tarayıcınınkinden 1,48 ile 1,55 sn ileriydi. Bayat bir yükleme
  (saklı tarama 316,8 sn eski) tam bir koşullu tarama gönderdi
  (`max_age_seconds=300`, 200, 27 servis, 959 ms) ve kart "Bileşenler
  kontrol ediliyor…"dan doğrudan sonuca geçti: yanıttan 4 ms sonra, ve sayfa 45 sn
  sonra kapanana dek orada kaldı. Koşunun hiçbir anında "Son 5 dakikada kontrol
  edilmedi" karesi görünmedi (on dördüncü kayıt, Debian'da bu noktada 28,6 sn
  ölçmüştü): D1 kusuru gitti. Taze bir yükleme (saklı tarama 46 sn eski) hiç
  göndermedi.
- `scan_age_seconds`, tarama taşıyan, tarayıcının kaydettiği 19 yanıtın hepsinde
  bir sayıydı; sürücünün kendi taramasında `0`'dı; tarama taşımayan `503`
  gövdelerinde yoktur. Hiç `null` olmadı; çünkü bir tarama her zaman vardı.
- Bileşenler sayfası, 835,2 sn eski saklı taramayla, tam bir koşullu tarama
  gönderdi (200, 1557 ms) ve toast göstermedi.
- Agent durmuşken İngilizce Dashboard, Bileşenler sayfası ve Türkçe Dashboard
  birer tarama gönderdi ve `503 AGENT_UNREACHABLE` aldı (5, 158 ve 4 ms). Üç gövde
  aynıdır: `{"error": <cümle>, "code": "AGENT_UNREACHABLE"}`. İleti `error`
  içindedir (`message` alanı yoktur) ve soket yolu yoktur; neden Panel'in
  günlüğünde kaldı. Dashboard kartı cümleyle ve "Scan services" düğmesiyle
  "Unknown / Could not check:" gösterdi; Türkçede "Servisleri tara" düğmesiyle
  "Bilinmiyor / Kontrol edilemedi:"; sayfa kapanana dek tutuldu ve ikinci tarama
  gönderilmedi. Bileşenler sayfası cümleyi yanıttan 28 ms sonra bir hata
  toast'ında gösterdi.
- Agent durmuşken nginx için bir servis yeniden başlatma isteği (`POST
  /api/v1/service/action`, oturum açmış sahip olarak) aynı `503` ile reddedildi.
  Hiçbir şey değişmedi: nginx'in ana süreç numarası, başlangıç zaman damgası,
  yeniden başlama sayacı ve exec-başlangıç zamanı öncesinde ve sonrasında aynıydı.
- Agent yeniden başlatıldıktan sonra "Servisleri tara"ya bir basış bir koşulsuz
  tarama gönderdi (200, 1037 ms, `scan_age_seconds` 0,001) ve kart sonucu
  gösterdi. Türkçe sayfa, basıştan önceki 15 sn'de kendiliğinden hiçbir şey
  göndermemişti.
- Bütün koşuda altı tarama gönderildi: bayat yükleme, Bileşenler sayfası, üç
  reddedilen tarama ve basış.

Kaydın kabul denetimi (commit fb5ec06f8), README'sindeki üç rakamı ham dosyalara
göre düzeltti ve yanlış bir `phases.json` alanını kaydetti; buradaki rakamlar
README'nin şu anki hâlidir. Çalışma ürünün hiçbir şeyini değiştirmedi ve
`result.json`ı `native_evidence: false` taşır.

**On beşinci kayıtta laboratuvarda bulundu, on altıncıda yok: lisans denetimi
(D2).** Aynı kayıtta, konuğun saati geri kaydırıldığında (tarayıcınınkinden
418,5 sn geride ölçüldü), 90 sn içinde hiçbir Panel sayfası açılmadı. Sayfa
`/api/v1/license/access`i 19 kez, 5 sn'de bir okudu, hepsi 200, ve başka hiçbir şey:
yönetilen-servisler okuması yok, tarama yok, Dashboard yok. Neden yanıtlardan değil
koddan okundu: `valid_until`, sunucunun saatiyle son doğrulamadan en çok 60 sn
sonraki bir andır ve sayfa, `valid_until`ı tarayıcının saatinden sonra olmayan
olumlu bir yanıtı reddediyordu. Kayıt erişim yanıtlarının gövdelerini saklamadı ve
tutulan sayfanın görüntüsünü almadı. Düzeltme (`valid_for_seconds`, yukarıda)
kayıttan sonra yapıldı; on altıncı kayıt onu ölçtü. Aynı nedenle on beşinci
kaydın geride kalan saat için yapmak istediği ölçüm (tek koşullu tarama, sonuç
hemen, ikinci tarama yok) orada yapılamadı; on altıncı kayıt yaptı.

**Aynı kayıtta gözlenen, sonra kaynakta düzeltilen, sonra on altıncı kayıtta
ölçülen.** Agent
durmuşken Bileşenler sayfası hiçbir işlem koşmamışken "Bir sunucu işlemi
tamamlanmış olabilir" gösterdi (O2) ve güvenlik duvarı okuması `500` yanıtladı
(O3: Bileşenler sayfası bir iç hata metniyle "Firewall: Status unknown",
Dashboard "Part of this server's state could not be read" gösterdi). Yukarıdaki
okuma şeridi ve ortak hata yazıcısı bunları yanıtlar; bileşen testleri vardı ve
on altıncı kayıt ikisini de ölçtü.

**On altıncı gerçek sistem kaydı**
([set14c](../deploy/e2e/release-recovery/evidence/set14c-20261013/README.md),
13 Ekim 2026 UTC). Son ürün commit'i `68b79b372`'nin test lisanslı bir
derlemesi, tek bir geçici Debian 13 konuğunda, ana makinede gerçek, başsız bir
Chrome 154 ile: İngilizce masaüstü penceresi (1440 × 900) ve Bileşenler sayfası
için bir Türkçe telefon penceresi (390 × 844). Kurulu arşiv, ağacı
`68b79b372`'ninkinden yalnız bir dosyada, `deploy/release-sequence-policy`de
ayrılan geçici bir sabit derlemeydi. Çalışma düzeneği konuğun saatini kaydırdı.
Sapma yanıtların kendisinden okundu: bir yönetilen-servisler yanıtından
(`scanned_at` artı `scan_age_seconds`) ve daha sıkı olarak her lisans erişim
yanıtından (`valid_until` eksi `valid_for_seconds`, yanıtlarken sunucunun
saati). Sapmalar, konuk eksi tarayıcı:

- Doğal, ilk yüklemeden önce: -78 ile +64 ms.
- Yedi dakika geride: -420.002 ile -419.945 ms.
- Yedi dakika ileride: +419.996 ile +420.058 ms.
- Geri alınınca: +3 ile +52 ms, 30 sn sonra -4 ile +64 ms. Çalışma düzeneğinin
  net kaydırması sonda 0'dı.

Sonuçlar:

- **Lisans denetimi, üç yükleme.** Aynı Dashboard yüklemesi üç kez, her biri
  300 sn'den eski bir saklı tarama (316,8, 317,1 ve 930,9 sn) ile ve 90 sn
  izlenerek: saat olduğu gibi, 420 sn geride ve 420 sn ileride. Dashboard (ilk
  yönetilen-servisler okuması) yüklemeden 192, 130 ve 132 ms sonra açıldı; kart
  253, 165 ve 172 ms sonra belirdi. Sayfa her 90 sn'de `/api/v1/license/access`i
  4 kez okudu (0,11, 44,1, 60,1 ve 89,1 sn'de; 0,07, 44,0, 60,1 ve 88,9 sn'de;
  0,07, 42,6, 60,1 ve 87,6 sn'de), her yanıt 200 ve `can_use_panel: true`,
  `state: active`, `observation: known`. Her yükleme tam bir koşullu tarama
  gönderdi (`max_age_seconds=300`, 200; 1197, 956 ve 1047 ms) ve kart yok,
  kontrol ediliyor, "3 / 3 running / installed" sırasını izledi ve sonuna dek
  öyle kaldı. İkinci tarama gelmedi (birincinin 88,8, 89,0 ve 89,1 sn
  sonrasına dek görülmedi). Sunucu yedi dakika geriyken sayfa, saat doğruyken
  olduğu gibi davrandı: tutma yok, 5 sn'de bir okuma yok (on beşinci kayıt:
  90 sn'de 5 sn aralıkla 19 okuma ve Dashboard yok). Sunucu yedi dakika
  ileriyken de aynı.
- **Erişim yanıtları.** Üç yüklemenin 12 yanıtının hepsi tam olarak şu
  anahtarlara sahip: `can_use_panel`, `observation`, `state`,
  `valid_for_seconds` ve `valid_until`; `valid_for_seconds` bir JSON sayısı,
  42,5 ile 60,0 sn. (Bütün çalışmada 17 yanıt aynı beş anahtarı taşıdı; Agent
  aşamasının dördü 16,7 ile 59,99 sn arasındaydı.) Üç yüklemede de sayfa,
  isteği gönderdiği an artı `valid_for_seconds` olarak hesaplanan son tarihten
  15 sn önce yeniden okudu. Sunucu gerideyken mutlak `valid_until`, tarayıcının
  saatine göre zaten geçmişteydi: ilk yanıtın alınmasından 361,1 sn, sonraki
  üçünün alınmasından 360,0 ile 376,1 sn önce (420 sn değil, çünkü
  `valid_until` sunucunun saati artı yaklaşık 60 sn'dir); hesaba göre eski kural
  bunları reddederdi, son tarihi sayfanın kullandığından yaklaşık 419,9 sn
  önceydi. Sunucu ileriyken mutlak `valid_until`, sayfanın erişimi sunucunun
  söylediğinden 420,05 sn daha uzun kabul etmesine izin verirdi; sayfanın kendi
  son tarihi daha geç değildi. Tarayıcının iç son tarihi dışarıdan okunamaz;
  okumaların zamanlamasından çıkarılır.
- **Agent durmuşken okumalar.** Güvenlik duvarı okuması
  (`GET /api/v1/firewall`), sistem denetimi (`GET /api/v1/system/check`) ve
  nginx için bir yeniden başlatma isteği (`POST /api/v1/service/action`) her
  biri `503 AGENT_UNREACHABLE` yanıtladı. Üç gövde aynıdır, yalnız `code` ve
  `error` taşır ve soket yolu söylemez; neden Panel'in günlüğünde kaldı.
  Reddedilen yeniden başlatma hiçbir şeyi değiştirmedi: nginx'in ana süreç
  kimliği, yeniden başlatma sayacı ve başlangıç zaman damgaları öncesinde ve
  sonrasında aynıydı. (On beşinci kayıt güvenlik duvarı okumasında `500`
  ölçmüştü.)
- **Agent durmuşken Dashboard.** Tek koşullu tarama, 3 ms'de `503`; kart yok,
  kontrol ediliyor, sonra Agent cümlesi ve "Scan services" düğmesiyle "Unknown /
  Could not check:" oldu ve 40 sn'nin sonuna dek tutuldu; ikinci tarama yok.
  Sayfanın kendi güvenlik duvarı okuması aynı `503`ü yanıtladı. Öteki kartlar
  (CPU, bellek, disk, barındırma, son etkinlik) gösterildi. Dashboard bir
  güvenlik duvarı şeridi göstermedi ve İngilizce penceredeki "Needs attention"
  kutusu Agent'ı adıyla söylemedi (O6, aşağıda).
- **Agent durmuşken Bileşenler sayfası.** İngilizce masaüstü: tek koşullu
  tarama, 107 ms'de `503`; neden olarak Agent cümlesi, sayfanın hiçbir bileşen
  işlemi göndermediği cümlesi ve "Rescan" düğmesiyle "Component state could not
  be read" şeridi, yüklemeden 1007 ms sonra görüldü ve 5 sn sonra hâlâ
  oradaydı. Türkçe telefon: bir tarama, 88 ms'de `503`; "Yeniden tara" ile
  "Bileşen durumu okunamadı" şeridi, yüklemeden 1012 ms sonra görüldü. İki
  sayfa da "Service state needs verification" ya da "may have completed" sözünü
  (Türkçede "Servis durumu doğrulanmalı", "tamamlanmış olabilir") göstermedi.
  Her sayfa Agent cümlesini bir hata bildirimi olarak da gösterdi.
- **Agent yeniden başlatıldı ve basış.** Türkçe sayfa basıştan önceki 5 sn'de
  kendiliğinden hiçbir şey göndermedi. "Yeniden tara"ya bir basış tek koşulsuz
  tarama gönderdi (200, 1014 ms, `scan_age_seconds` 0, 27 servis); şerit
  basıştan 1092 ms sonra gitmişti ve 10 sn sonra hâlâ yoktu.
- Tarayıcı bütün çalışmada 7 tarama gönderdi: ölçülen her yükleme için bir,
  reddedilen üç ve basış.

Kayıt, o commit'lerin değiştirdiği kodda kusur bulmadı. Kabul denetimi dört
sağlama toplamı listesinde yazılı 273 dosyayı denetledi (hepsi var, gizli bilgi
ya da işletmeci kimliği yok) ve README'nin iki ifadesini ham dosyalara göre
düzeltti. Çalışma ürünün hiçbir şeyini değiştirmedi ve `result.json`ı
`native_evidence: false` taşır. Hiçbir P0 satırını kapatmaz. Gözlemleri O4, O5 ve
O6 ile çevrilmemiş posta yığını metni sınırlardadır.

**On yedinci gerçek sistem kaydı**
([set15](../deploy/e2e/release-recovery/evidence/set15-20261013/README.md),
13 Ekim 2026 UTC). Yayımlama öncesi ölçüm; aralarında hiçbir sabit commit
olmadan, bu sürümün hazırlandığı commit olan `1636b73f0`'ın kendisini çalıştıran
ilk kayıt. Aynı zamanda v0.1.0-alpha.83'ten bir güncelleme çalıştıran ilk kayıt.
Test lisanslı derlemeler, hiçbiri sürüm değil: aday (sürüm v0.1.0-alpha.84,
sıra 84) `1636b73f0`'dan derlendi; taban çizgisi, alpha.83 etiketinin kaynağından
(etiket commit'i, yamasız) yeniden yapılmış bir derlemedir, yayımlanmış arşiv
değil; kusurlu aday, `1636b73f0` artı daha önceki bir kaydın yalnız
göç aşamasını bozan kusurudur (`cmd/panel/main.go`'da). Geçici konuklar, ana
makinede gerçek, başsız bir Chrome; Debian 13'te İngilizce masaüstü penceresi
(1440 x 900), Ubuntu 24.04'te Türkçe masaüstü penceresi.

- **Temiz kurulumlar, Debian 13 ve Ubuntu 24.04.** Sürüm rotası
  `v0.1.0-alpha.84`, commit `1636b73f0979…`, `agent_matches: true`,
  `schema_version: 44` yanıtladı; kenar çubuğu "CelikPanel · v0.1.0-alpha.84 ·
  build 1636b73f0979" yazdı. Önyükleme sabitleri arşivin `libexec/get.sh`
  dosyasında (6. ve 7. satır) ve her iki konukta o dosyanın kurulu kopyasında 84
  ve `v0.1.0-alpha.84` okundu. Oturum açma, lisans ve `web` kurulum amacı geçti;
  kurulum DNS adımında bekler ve orada bırakıldı. Alan adı listesi boştu
  (`GET /api/v1/domains` `[]` yanıtladı) ve sayfa ilk durum satırı göstermedi.
  `GET /api/v1/license/access` `valid_for_seconds` taşıdı ve
  `can_use_panel: true`, `state: active`, `observation: known` idi;
  `GET /api/v1/managed-services` `scan_age_seconds` taşıdı.
- **Bayat taramayla Dashboard.** Yüklemede saklı tarama 317,1 sn (Debian) ve
  317,2 sn (Ubuntu) yaşındaydı. 90 sn içinde her Dashboard tam bir
  `POST /api/v1/managed-services/scan?max_age_seconds=300` gönderdi: 200,
  Debian'da 1126 ms, Ubuntu'da 1590 ms. Kart yoktan kontrol ediliyor
  durumuna (yüklemeden 1145 ms ve 1611 ms sonra), oradan "3 / 3 running /
  installed" (Türkçede "3 / 3 çalışan / kurulu") durumuna geçti. "Open
  Components to rescan" görülmedi. Hemen ardından yüklenen Bileşenler
  sayfası, taraması 90 sn yaşında olduğundan, tasarlandığı gibi tarama
  göndermedi. Ubuntu'da bayat taramayla (317 sn) Bileşenler sayfası tek koşullu
  tarama gönderdi (200, 2026 ms) ve uyarı ya da bildirim göstermedi. Debian'da o
  yükleme ölçülmedi.
- **v0.1.0-alpha.83'ten güncelleme, Debian 13, iyi aday, üç koşu (a, b, c).**
  Barındırılan bir statik siteli taban çizgisi, Panel'in kendi güncelleme yoluyla
  güncellendi. Her koşu `phase: succeeded`, `reason: update_verified`,
  `terminal_proof: update_verified` ile bitti; Agent ve Panel'in yapı kimliği
  `v0.1.0-alpha.84`, `1636b73f0…`, sürüm alt sınırı 84 sırasında, şema defteri
  `as-expected`. Şema öncesinde 44 (alpha.83 44 yanıtladı), sonrasında 44 idi.
  alpha.83 etiketi ile `1636b73f0` arasında hiçbir göç dosyası değişmedi
  (`cmd` ve `internal` altında 9 dosya, hiçbiri göç değil). Alan adları 1 satır
  gösterdi, `site_config: managed_unchanged`.
- **Güncellemeden sonra Dashboard (c koşusu).** Güncellemeden önceki 441 sn
  yaşında saklı taramayla kendiliğinden tek koşullu tarama gönderdi (200,
  1210 ms) ve yoktan kontrol ediliyor durumuna (1240 ms), oradan "6 / 6 running /
  installed" durumuna geçti. Bileşenler sayfası hemen ardından tarama
  göndermedi, tarama bayatlayınca tek koşullu tarama gönderdi (200, 1187 ms).
  Dashboard'ın Posta yığınları kartı, tarama yanıtlayana dek yaklaşık 1,2 sn
  "Status needs refresh" ve "Open Components to rescan" gösterdi. Bu, sınırlarda
  yazılı bilinen açık maddedir (kartın eski metnin kendi kopyası var); yeni
  değildir.
- **Kusurlu aday: otomatik geri dönüş.** "`payload_restored`da yeniden
  başlatma" hatasıyla konuk bir kez sıfırlandı. Güncelleme kendiliğinden
  kurtarıldı: `phase: recovered`, `reason: rollback_verified`,
  `previous_failure: update_failed`, iki otomatik deneme. Sonrasında Agent ve
  Panel alpha.83 yapısını bildirdi (`v0.1.0-alpha.83`, `0447558e…`) ve Panel
  yanıtladı: tek sayfa kökü ve bir API rotası 200, bir rota 401 ya da 403, var
  olmayan bir yol 404, sürüm rotası alpha.83'ün yapısını adlandırdı.
- **Açık bulgu: geri dönüşten sonra `managed_site_files`.** Sürücünün son
  adımı veritabanı karşılaştırmasında başarısız oldu: geri dönüşten sonra
  Panel'in veritabanı, güncellemeden önce alınan kopyadan tek bir tabloda,
  `managed_site_files`'ta farklıdır (iki seferde de bir satır, 21 sütun;
  satırlar değil yalnız özetler kaydedildi). Üç iyi koşu aynı tabloyu farklı
  diye listeler ve geçer, çünkü güncelleme yolu eşitlik istemez. Olası neden,
  kaynaktan ve **satırlardan doğrulanmadı**: Panel her başlangıçta her sitenin
  dosyalarını yeniden gözlemler ve satırın gözlem alanlarını, `observed_at`
  dahil, yeniden yazar (`internal/services/site_file_ledger.go` 263 ile 273.
  satırlar, `observed_at = excluded.observed_at` içeren bir upsert, ve 297 ile
  300, 327, 391 ve 426. satırlar; sütun
  `internal/db/migrations/044_managed_site_files.sql` 31. satırdadır). Geri
  dönüşten sonra alpha.83 başlar ve yeniden gözlemler; tablonun özeti bu yüzden
  değişir. Sürücünün denetimi tabloyu oynak saymadı, çünkü tablo set12 ve set13
  kayıtlarının alpha.82 taban çizgilerinde yoktu (defter 43). Adım başarısız
  olduğundan geri dönüş sonrası okumalar ve onu izleyen tarayıcı penceresi
  koşmadı; bu yüzden geri dönüşten sonra kenar çubuğu resmi ya da Dashboard
  okuması yok. Yukarıdaki ürün düzeyindeki olgular geçerlidir. Bulgu
  kaydedildi, hükme bağlanmadı.
- **Geri dönüşten sonra kurtarma temeli dosyası.** Otomatik geri dönüşten
  sonra sürüm alt sınırı 84 sırasını okur ve kurtarma temeli dosyası hâlâ kusurlu
  adayın commit'ini adlandırır; çalışan yapı alpha.83'tür. Kaydedildi, hükme
  bağlanmadı. Örüntü olarak yeni değildir:
  [upd2](../deploy/e2e/release-recovery/evidence/upd2-20261001/README.md) ve
  [upd7](../deploy/e2e/release-recovery/evidence/upd7-20261001/README.md)
  kayıtları (1 Ekim 2026) alt sınır ile kurtarma temelinin geri almadan sonra
  adayın değerlerinde kaldığını zaten gösterir (upd2 alt sınırın durumunu tasarım
  gereği der), set12 kaydı da 82'ye dönüşünden sonra alt sınırı 83'te gösterdi.
  set12 ve set13 README'leri kurtarma temeli dosyasından söz etmez. Yalnız alpha.83
  taban çizgili bir okuma olarak yenidir.
- **Ana makinenin uykusu.** Dizüstü ana makine, üçüncü iyi koşu sırasında
  yaklaşık 10:20Z ile 12:35Z arasında uyudu (modern bekleme; uyanık tut isteği
  engellemedi). Konuk donmuştu ve saati tarayıcınınkinin 8050 sn gerisinde
  bitti. O koşunun iş yükü hükmü başarısız oldu ("workload interruption outside
  the expected windows: web, smtp, cron, panel"); bu ortamın işidir. Uyku
  olmayan a ve b koşuları o hükmü geçti. c koşusunun güncellemesi uyanmadan
  sonra koştu.
- **Koşulmadı ya da ölçülmedi:** Arch; Debian temiz kurulumda bayat Bileşenler
  yüklemesi; kusurlu geri dönüşten sonraki okumalar (açık bulguya bakın).

Kabul denetimi, listelenen 1578 dosyanın hepsinin var olduğunu, gizli bilgi ya
da işletmeci kimliği bulunmadığını denetledi; bir ana makine yolu karartıldı ve
README'nin dört ifadesi ham dosyalara göre düzeltildi. Çalışma hiçbir ürün
kodunu değiştirmedi ve `result.json` dosyaları `native_evidence: false` taşır.
Hiçbir P0 satırını kapatmaz. Açık bulgu kaydedildi, bir ürün kusuru diye hükme
bağlanmadı.

## Bu sürümün sınırları

Bunlar bilinir. Gizli kusur değildir. İlk grup bu sürümle yeni; ikincisi
v0.1.0-alpha.83'ün sınırlarının, bu sürümde hiçbiri ele alınmadığı için her biri
"hâlâ açık" diye işaretlenmiş özetidir. Özetlenirken alpha.83'ün iki cümlesi
düşürüldü (kanıt kapsamındaki ana makine bekleme cümlesi ve "Bu sürüme
güncelleme sırasında açık sayfa hâlâ alpha.82'nindir"); ikisi de hâlâ
geçerlidir.

Bu sürümle yeni:

- **Açık bulgu: otomatik geri dönüşten sonra `managed_site_files`.** On yedinci
  kayıtta, kusurlu adaydan geri dönüşten sonra laboratuvarın veritabanı
  karşılaştırması, güncellemeden önce alınan kopyadan farklı olan tek bir tablo
  buldu: `managed_site_files`. Neden satırlardan doğrulanmadı (yalnız özetler
  tutuldu). Kaynaktan olası neden, Panel'in her başlangıçta her satırın gözlem
  alanlarını, `observed_at` dahil, yeniden yazması ve geri dönüşün yeniden
  başlatmasının onları değiştirmesidir (`internal/services/site_file_ledger.go`
  263 ile 273, 297 ile 300, 327, 391 ve 426. satırlar;
  `044_managed_site_files.sql` 31. satır). Laboratuvarın denetimi bir sonraki
  kayıtta alanı ya da tabloyu dışlamalıdır. Ürün kusuru diye hükme
  bağlanmadı; geri dönüşün kendisi oldu ve Panel alpha.83 olarak yanıtladı.
- **Geri dönüşten sonra kurtarma temeli dosyası kusurlu adayı adlandırır.**
  Otomatik geri dönüşten sonra dosya, çalışan yapı alpha.83 iken hâlâ kusurlu
  adayın commit'ini adlandırır ve sürüm alt sınırı 84 okunur. Kaydedildi,
  hükme bağlanmadı. Örüntü olarak upd2 ve upd7 kayıtlarından (1 Ekim 2026) ve
  alt sınır için set12 kaydından zaten bilinir; açık.
- **Ubuntu ve Arch değişikliklerden sonra ölçülmedi.** On dördüncü kayıt
  Debian 13 ve Ubuntu 24.04'te (yalnız ilk commit), on beşinci ve on altıncı
  yalnız Debian 13'te koştu. Debian laboratuvarlarının boşta duran Arch eşi
  ölçülmedi. On yedinci kayıt adayı Ubuntu 24.04'e temiz kurdu ve orada
  Dashboard ile Bileşenler sayfasını ölçtü; Ubuntu'da güncelleme koşmadı ve Arch
  koşulmadı.
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
- **Debian 13'te bir kez ölçüldü, başka hiçbir yerde:** lisans denetiminin
  düzeltmesi (D2), güvenlik duvarı okumasında, sistem denetiminde ve bir yeniden
  başlatma isteğinde tipli Agent yanıtı ve Bileşenler sayfasının okuma şeridi,
  hepsi on altıncı kayıtta. Ubuntu ve Arch bu değişikliklerden sonra koşulmadı.
  Sonucu bilinmeyen bir işlemden sonra "tamamlanmış olabilir" şeridi ölçülmedi:
  onu üretmek, yanıtı kaybolan bir bileşen işlemi ister ve çalışma düzeneğinin
  bunu yapmanın ucuz bir yolu yok. Yalnız bileşen testleri var.
- **O4: Türkçe Bileşenler sayfası güvenlik duvarı şeridinin sunucu cümlesini
  İngilizce gösteriyor.** On altıncı kayıtta Agent durmuşken görüldü: "Güvenlik
  duvarı: Durum bilinmiyor" ve ardından İngilizce Agent cümlesi.
  `FirewallBar.load`, yanıtın `error` dizesini ileti olarak alır
  (`web/src/components/ServiceList.tsx:3259-3271`, `:3286`) ve kodun çevirisi
  (`err.AGENT_UNREACHABLE`; tarama şeridi ve bildirim onu kullanır) yerine onu
  gösterir (`:3393`). Sonraki sürüm için açık. Buradaki satır numaraları
  `68b79b372`ninkilerdir.
- **O5: güvenlik duvarı şeridi Agent dönünce yeniden okunmuyor.** Basıştan 10 sn
  sonra da Türkçe sayfa, Agent çalışıyor ve tarama başarılı olduğu hâlde Agent
  cümlesiyle "Durum bilinmiyor" gösterdi: `FirewallBar` yalnız açılışta
  (`ServiceList.tsx:3289-3291`) ve kendi işleminden sonra (`:3318`) okur; Yeniden
  tara onu okumaz. Yeniden yükleme okurdu. Sonraki sürüm için açık.
- **O6: Dashboard'ın "İlgi istiyor" kutusu Agent'ı adıyla söylemiyor.**
  `GET /api/v1/firewall` `503 AGENT_UNREACHABLE` yanıtlarken kutu yalnız durumun
  bir kısmının okunamadığını söyler (`dashboard.attentionUnread`,
  `web/src/components/Dashboard.tsx:895`, `:1163-1165`); aynı sayfadaki Sistem
  servisleri kartı Agent'ı ve sahibin komutlarını söyler. Sonraki sürüm için
  açık.
- **Türkçe Bileşenler sayfasında çevrilmemiş İngilizce metin.** Posta yığını
  açıklamaları ("Postfix SMTP and Dovecot IMAP/POP3 with authenticated
  submission.", "Core mail plus Nginx, PHP-FPM and Roundcube webmail.") ve "Some
  components are already present. Installing this profile will reconcile the full
  plan without removing completed services." On altıncı kayıtta görüldü;
  incelenmedi. Sonraki sürüm için açık.
- **Konuğun NTP ayarları ölçülmedi.** On altıncı kayıtta `timedatectl show` hiç
  alan yazdırmadı; bu yüzden saat değişikliklerinin yalnız dönüş kodları
  kaydedilmiştir; saat hizmetinin, yeniden açıldıktan sonra saati ayarlayıp
  ayarlamadığı söylenemez.
- **Sunucu saati tarayıcınınkinden geriyken taramanın yaşı bir kez ölçüldü.**
  On beşinci kayıt bu ölçümü yapamadı (D2 sayfayı tuttu). On altıncı kayıt onu
  yalnız Debian 13'te, saat yedi dakika geriyken ve yedi dakika ileriyken birer
  kez yaptı (yukarıya bakın); Ubuntu ve Arch koşulmadı.
- **On beşinci kayıt yalnız Debian 13'te koştu.** Ubuntu ve Arch yeniden
  ölçülmedi; pencere kuralı, gizli sekme, tutulan (`409`) durum,
  `server_setup_busy` ve hiç kontrol edilmemiş durum istenmedi (pencere kuralı on
  dördüncü kayıttan beri değişmedi). Erişim yanıtlarının gövdeleri (on altıncı kayıt
  kaydetti) ve konuğun saat hizmeti ayarları kaydedilmedi.
- **Laboratuvar araçları hâlâ yalnız `valid_until` gönderir.** `mock.mjs` taklit
  sunucusu ile `cold-load-set9.mjs` ve `cold-load-set11.mjs` ilk yükleme araçları
  erişim denetimini yalnız `valid_until` ile yanıtlar; bu yüzden `valid_for_seconds`ı
  değil eski kuralı sınarlar.
- **`DomainSSLSettings`teki HSTS geri çekme hâlâ bir sunucu zaman damgasını
  tarayıcının saatiyle karşılaştırır.** Yukarıdaki kusurlarla aynı sınıf; burada
  değiştirilmedi. Açık.
- **"Hiçbir şey değiştirilmedi", aynı istekte daha önce bir değişiklik olmadığını
  varsayar.** Yazıcı başarısız bağlantı kurmayı yalnız tek neden olduğunda Agent
  yanıtı sayar ve birleşik-hata koruması toplu hataları (bir hata ve telafisi)
  kapsar. Bir şeyi değiştirip sonra, önceki sonuca birleşmemiş ayrı bir bağlantıda
  başarısız olan bir işleyici yine de bunu söylerdi. Açık; işleyici işleyici bir
  denetim iddia edilmez.
- **Aday, yayımlamadan önce kendisi bir kez ölçüldü.** On yedinci kayıt bir
  sabit değil, `1636b73f0` commit'inin kendisini çalıştırdı: Debian 13 ve
  Ubuntu 24.04'te temiz kurulumlar ve Debian 13'te v0.1.0-alpha.83 etiketinin
  yeniden derlemesinden güncelleme. Bu, sürüm hazırlığının (sürüm sırası
  numarası, önyükleme sabitleri, sürüm satırları) ölçülen hiçbir derlemede
  olmadığı önceki sınırı kapatır. Kapsamadığı: arşivler imzalı değil, test
  lisanslı yeniden derlemelerdi; taban çizgisi yayımlanmış arşiv değil,
  alpha.83 etiketinin yeniden derlemesiydi; güncelleme yalnız Debian 13'te
  koştu; Arch koşulmadı; kusurlu adayın geri dönüşten sonraki okumaları
  koşmadı (açık bulguya bakın).
- **Kanıt kapsamı.** Tek dizüstü ana makine; imzalı arşiv değil, test lisanslı
  bir derleme; trafiği yakalanmayan, dışarıya NAT'lı konuklar; her hücre bir kez
  koştu; İngilizce Debian'da masaüstü penceresinde, Türkçe Ubuntu'da telefon
  penceresinde görüldü, iki dil iki platformda değil. On beşinci kayıt yalnız
  Debian 13 ekledi: İngilizce masaüstü penceresi, bir Dashboard yüklemesi için
  Türkçe, konuğun saati çalışma düzeneğince kaydırıldı. On altıncı kayıt yine
  yalnız Debian 13 ekledi: İngilizce masaüstü penceresi, Bileşenler sayfası için
  bir Türkçe telefon penceresi, konuğun saati her yöne yedi dakika kaydırıldı,
  her hücre bir kez. On yedinci kayıt Debian 13'te (İngilizce masaüstü penceresi)
  ve Ubuntu 24.04'te (Türkçe masaüstü penceresi) birer temiz kurulum ve Debian
  13'te güncellemeyi (iyi aday üç kez, kusurlu aday bir kez) ekledi; saat
  kaydırılmadı.
- **Agent kapalıyken öteki kartlar.** On dördüncü kayıtta, Agent durmuşken
  Dashboard'ın CPU, bellek ve disk kartları kehribar uyarıyla "—" gösterdi;
  güvenlik duvarı okuması on dördüncü ve on beşinci kayıtlarda `500` yanıtladı.
  On altıncı kayıtta güvenlik duvarı okuması Agent'ı adıyla söyleyen `503
  AGENT_UNREACHABLE` yanıtladı ve CPU, bellek ve disk kartları değer gösterdi (%
  0, % 15, % 11); bu sürüm o kartları değiştirmez ve fark incelenmedi.
  Dashboard'ın "İlgi istiyor" kutusu Agent'ı adıyla söylemiyor (O6) ve Türkçe
  Bileşenler sayfasının güvenlik duvarı şeridi İngilizce bir cümle gösteriyor
  (O4).

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

1. Bir sonraki gerçek sistem kaydı: yapıldı (set14c, on altıncı, yalnız
   Debian 13, 13 Ekim 2026): lisans denetiminin düzeltmesi sunucu saati yedi
   dakika geride ve ileride, saat gerideyken taramanın yaşı, güvenlik duvarı ve
   sistem denetimi okumalarında Agent yanıtı ve Bileşenler sayfasının okuma
   şeridi, gerçek bir sunucuda. Burada artık açık değil. Orada ölçülmeyen ve hâlâ
   açık olanlar: Ubuntu, Arch, bir işlemden sonra "tamamlanmış olabilir" şeridi,
   konuğun NTP ayarları ve gözlemleri O4 ile O6 (sınırlara bakın).
2. Yayımlama öncesi ölçüm: yapıldı (set15, on yedinci kayıt, 13 Ekim 2026),
   1636b73f0 commit'inin kendisinde: Debian 13 ve Ubuntu 24.04'e temiz
   kurulumlar, Debian 13'te v0.1.0-alpha.83'ten güncelleme ve kusurlu bir
   adayın otomatik geri dönüşü. Sınırlardaki iki açık maddeyi bıraktı. Aşağıdaki
   sahip adımları değişmedi.
3. Sürüm hazırlığı: 1636b73f0 commit'inde yapıldı; `cmd/`, `internal/` ya da
   `web/` altında hiçbir dosyayı değiştirmez. İçinde 83'ten sonra 84 olan sürüm
   sırası politikası (önceki olarak alpha.83 sürüm commit'i),
   `download-portal/get.sh` sabitleri, README sürüm satırları ve üç sözleşme
   testi var; üç sözleşme testi derleme konuğunda geçer. Hâlâ açık: sahibin
   etiketten önce ayarladığı `CELIKPANEL_RELEASE_SEQUENCE=84` depo değişkeni.
   Gerçek arşivi etikette CI derler ve imzalar.
4. Numara: yapıldı, 13 Ekim 2026. Sahip numaranın v0.1.0-alpha.84 olduğuna karar
   verdi.
5. Çekme isteğinin son ucu kendi CI koşusunu alır.
6. Sahip birleştirir, sürümü etiketler, imzalı sürüm sözleşmesinin anlattığı
   gibi altı imzalı dosyayı denetler ve indirme portalını yayımlar.
7. Sürüm yayımlandıktan sonra, kurulu her sunucunun sahibi o sunucuyu panelin
   kendi güncelleme ekranından günceller. Bu, yukarıdaki her adımdan ayrı bir
   adımdır; onu başka kimse başlatmaz.

Bu sürümü yalnızca CelikPanel'in güncelleme arayüzünden kurun. Yayımlanması
kurulu sunucuları güncellemez; bir sunucunun güncellemesini yalnız sahibi,
panelin kendi güncelleme ekranından başlatır.
