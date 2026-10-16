# v0.1.0-alpha.91 (aday, yayımlanmadı)

[English](RELEASE-NOTES-v0.1.0-alpha.91.md)

Bu, v0.1.0-alpha.90'ı izleyen sürümdür. v0.1.0-alpha.91 numarasını sahibin
13 Ekim 2026 tarihli kalıcı talimatı uyarınca sürümü hazırlayan seçti.
16 Ekim 2026'da (UTC) `feat/alpha91-second-server-part2` dalında, `main`'in
`1743ac69edb0e5723dc60d992b53094dcc4b1431` noktasından (alpha.90 sürüm
commit'i; dala birleştirildi) yazıldı. Dalın ürün commit'leri `7cd9232db`,
`5a749b5c6` (kod, testler, 18. toplu deneme) ve `c99c48b70` (işlem
yönlendirmesi kaydı).

## Sahibin kararları ve sözü

Sahip 16 Ekim 2026'da, ikinci sunucu işinin 1. bölümünün (alpha.87) açık
bıraktığı üç soruyu **"önerin gibi olacak"** diyerek karara bağladı:

1. İkinci sunucu, birincinin tam bir **web ve posta eşi** olabilir.
2. **Altyapı bölgesi** (örneğin panellerin kendi bölgesi `celikhost.com`)
   birincide **Alan adları'nın normal bir girdisi** olarak, mevcut DNS kayıt
   düzenleyicisiyle gösterilir. Kurulumun oluşturduğu kayıtları kurulum
   yönetmeye devam eder; sahibin kendi kayıtları korunur.
3. İkinci sunucunun **ana makine kayıtları birinciye otomatik gider**: var olan
   yetkili uzak DNS bağlantısı üzerinden; elle girmek de mümkün kalır.

Sahip ayrıca kullanıcıların kendi sunucularını kendilerinin yapılandırdığını
söyledi: **panel sunar; karar kullanıcınındır.** Bu yüzden hiçbir şey dayatılmaz.
Reddedilen ya da yapılamayan bir istek, elle girilecek kesin kaydı bırakır ve
sahibin kendi kayıtlarının üzerine asla yazılmaz.

## Ne değişti

alpha.87 incelemesinde bulunan boşluklardan üçü (4, 5 ve 6) kaynakta
düzeltildi.

**Boşluk 4: eşin posta sunucusu adı birincinin planında.** Birincinin altyapı
DNS seçimine `peer_mail_hostname` eklendi; gözden geçirilmiş eş adresine bir A
kaydı olarak hazırlanır ve plan incelemesinde görünür. Türü belli plan
engelleri: `server_setup_infrastructure_dns_peer_mail_own_name` ve
`server_setup_infrastructure_dns_peer_mail_outside_zone`. Eski planda bu alan
yoktur ve kimliğini korur. Kod: `cmd/panel/server_setup.go`,
`server_setup_infrastructure_dns.go`, `server_setup_operations.go`;
`web/src/components/ServerSetup.tsx`. Testler: `server_setup_peer_mail_test.go`
(üç); `web/tests/server-setup-runtime.test.mjs`.

**Boşluk 5: Alan adları'nda altyapı bölgesi.** Yalnız yönetici:
`GET /api/v1/dns/infrastructure-zones`, ayrıca mevcut kayıt işleyicileri
üzerinde `GET …/{zone}/zone` ve `GET|POST|PUT|DELETE …/{zone}/records`. Her kayıt
kimin yönettiğini söyler (`managed_by`: `setup` ya da `peer`). Kurulum
kaydını değiştirmek ya da silmek `INFRASTRUCTURE_DNS_RECORD_MANAGED_BY_SETUP`
ile reddedilir (eylem Ayarlar, Sunucu kurulumu'ndadır); sahibin kayıtları eskisi
gibi düzenlenir ve sahibin değiştirdiği kurulum adresi artık kurulumun değildir.
Bölgenin kendi adı için bir web sitesi eklemek var olan bölgeyi dokunmadan
devralır; o web sitesini kaldırmak bölgeyi korur. Kod:
`cmd/panel/infrastructure_dns_zone.go`, `domain_dns_handlers.go`,
`domain_delete_saga.go`; `web/src/components/Domains.tsx`,
`InfrastructureZone.tsx`. Testler: `infrastructure_dns_zone_test.go` (beş);
`web/tests/remote-state-mounted.test.mjs` (beş, her biri bir denetimle).

**Boşluk 6: uzak DNS bağlantısı üzerinden ana makine kayıtları.** Birincil,
`POST /api/v1/dns/remote/receiver/host-record` isteğini kabul eder (uzak DNS
alıcısında tek yeni makine ucu; bağlantının var olan makine kimlik bilgisini
kullanır): altyapı bölgesinin kesin içinde tek bir ad, DNS çiftinin doğrulanmış
eş adresiyle A türünde. Asla üzerine yazmaz; reddedilmeler türlüdür:
`REMOTE_DNS_HOST_RECORD_CONFLICT`, `…_INVALID`, `…_OUTSIDE_ZONE`, `…_ADDRESS`,
`…_PENDING` (kaydedildi, yayımı henüz doğrulanmadı; aynı istek yeniden
gönderilebilir). Her DNS yayımıyla aynı kilitler altında çalışır ve
denetim kaydına yazılır (`dns.remote.host_record.*`). İkincil, isteği kurulumun
herkese açık DNS adımından adım kimliği başına bir kez gönderir; bilinmeyen
sonuç iki dakika sonra yeniden, en çok beş kez sorulur; sayfayı okumak hiçbir
şey göndermez. Bölge kopyalaması yerel aktarım olarak kalır ve bu kanala hiç
bağlı değildir. Kod: `cmd/panel/remote_dns_host_record.go`,
`server_setup_access_dns.go`, `setup_dns_remote.go`, `remote_dns_auth.go`,
`mail_dns_identity.go`. Testler: `remote_dns_host_record_test.go` (on; biri
bölgenin hiç bağlantı olmadan çalışması); `web/tests/server-setup-guidance.test.mjs`
(iki), `remote-state-mounted-batch3.test.mjs` (bir).

## Sahip ne görecek

**Frankfurt'ta (birincil).** Alan adları, altyapı bölgesini "Altyapı bölgesi ·
yalnız DNS" işaretli bir satır olarak ve tek eylemle, "DNS kayıtlarını yönet",
listeler. Bölgenin sayfası önce bölgenin ne olduğunu, kurulumun hangi kayıtları
koruduğunu ve nerede değiştirildiklerini, kalanların sahibin olduğunu ve
hangilerinin eşin olduğunu söyler; sonra olağan kayıt düzenleyicisini gösterir.
Kurulumun oluşturduğu kayıtlar "Sunucu kurulumu yönetiyor" der; diğer sunucunun
istediği kayıtlar eşinki olarak görünür. Okunamayan bir bölge listesi, Yeniden
dene'li tek satırdır ve alan adlarını asla gizlemez.

**Boston'da (ikincil), posta eklerken.** İki panelin uzak DNS bağlantısı varsa
kurulum, posta sunucusunun kaydını Frankfurt'tan kendisi ister ve ne olduğunu
söyler: isteniyor; uygulandı ya da zaten var ("sizden başka bir şey gerekmiyor");
nedeniyle reddedildi; birincil böyle istekleri kabul etmiyor; henüz yanıt yok.
Bağlantı yoksa sayfa elle girilecek kesin kaydı gösterir (birincilde Alan
adları'nda, altyapı bölgesinde ya da DNS sağlayıcısında). Nasıl anlaşılır:
Boston'daki Bileşenler sayfasının posta kutusu ya "Bu sunucunun birincile yetkili
bir DNS bağlantısı var…" ya da "Bu sunucunun birincile yetkili bir DNS bağlantısı
yok; bu kaydı hiçbir şey kendiliğinden oluşturmaz. Kaydı elle ekleyin…" der.

## Sürümü hazırlayanın verdiği kararlar

D-021 ve D-033 ile sahibin 13 Ekim 2026 tarihli kalıcı talimatı altında sürümü
hazırlayan verdi; sahibin onaylaması için işlem yönlendirmesinde kayıtlıdır;
hiçbiri sahibin kararı değildir.

- Şema değişikliği yok: altyapı bölgesi bir kiracı `domains` satırı değildir;
  kurulumun sahiplik kaydından listelenir ve kendi yönetici yolundan düzenlenir.
- Altyapı bölgesinin kendi adından kaldırılan bir web sitesi bölgeyi korur.
- "Kurulum yönetiyor", kurulumun tam değeriyle hazırladığı A/AAAA kayıtları
  demektir; SOA ve tepe NS "DNS ayarları yönetiyor" olarak kalır.
- Eşin kayıtlarını sahip değiştirebilir ya da silebilir; kanal bundan sonra
  üzerine yazmayı ya da yeniden oluşturmayı reddeder.
- Birincil, A kaydını yalnız DNS çiftinin eş adresiyle, AAAA kaydını yalnız
  isteğin kendi kaynak adresiyle kabul eder; ikincil yalnız A kaydını gönderir.
- İstek ayrı bir adım ya da düğmeyle değil, kurulumun erişim DNS adımından
  gönderilir; böylece var olan planların adımları değişmez.
- Bilinmeyen sonuç iki dakika sonra, en çok beş kez yeniden sorulur; reddedilen
  istek aynı adım için yeniden gönderilmez.
- Birincil kaydı uyguladıktan sonra kurulum sayfası genel "kayıt yönergelerini
  izleyin" satırını artık çizmez.

## Ne ölçüldü

- `web/` içinde `npm test`: 1534 / 1534 (alpha.90 birleştirmesinden önce;
  birleştirmeden sonra yeniden çalıştırılmadı). `npx tsc --noEmit` temiz;
  `npm run build` bütçesi içinde.
- WSL konuğunda `5a749b5c6` üzerinde `go test ./cmd/panel`: 1581 üst düzey test
  geçti, 6 atlandı, 1687 alt test geçti; gofmt ve vet temiz. On sekiz yeni Go
  testi ve on bir yeni web testi; her yeni kural bir kez kapatıldı ve testi
  başarısız oldu. Birleştirmeden sonra yeniden çalıştırılmadı.
- Sahte tarayıcı toplu denemesi 18 (masaüstü ve telefon, İngilizce ve Türkçe,
  açık ve koyu): Türkçede İngilizce yok, ham anahtar yok, yatay taşma yok; iki
  arayüz kusuru bulundu ve düzeltildi. Bağlantılı ve bağlantısız Bileşenler
  kutusunu sahte ortam çizmedi; bağlı bir testte dayanır.
- Sürüm sırası ilkesi sözleşme testi geçer ("90 -> 91").

## Sınırlar

- Sahibin 16 Ekim 2026 talimatıyla **gerçek sistem kaydı yok**. Kanıt; bileşen,
  bağlı ve Go testleri ile bir sahte tarayıcı toplu denemesidir.
- Kurulu hiçbir sunucuda ölçüm yapılmadı; kurulu hiçbir panel güncellenmedi ya
  da ona ulaşılmadı.
- Otomatik kayıt için iki kurulu panel arasındaki kanalın (yetkili uzak DNS
  bağlantısı) bulunması gerekir; yoksa kayıt, yukarıda gösterildiği gibi elle
  girilir. Eski bir birincil yeni isteği düz bir ret ile yanıtlar; ikincil o
  zaman elle girilecek kaydı gösterir.
- Şema 44 değişmez ve geçiş çalışmaz; kurtarma davranışı değişmez. Yeni
  `panel_settings` anahtarları ve JSON alanları eklemelidir.
- P0.1-P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez; yayımlanma
  kabul olmaz.

## Yayımlama kaydı

Yayımlanmadı.

Bu sürümü yalnız CelikPanel'in güncelleme arayüzünden kurun. Yayımlanması
kurulu sunucuları güncellemez; bir sunucunun güncellemesini yalnız sahibi,
panelin kendi güncelleme ekranından başlatır.
