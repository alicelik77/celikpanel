# v0.1.0-alpha.100 (22 Ekim 2026'da yayımlandı)

[English](RELEASE-NOTES-v0.1.0-alpha.100.md)

Bu sürüm v0.1.0-alpha.99'un ardından gelir. 22 Ekim 2026'da yayımlandı (sonda
"Yayım kaydı"na bakın); henüz hiçbir parçası bir kurulu sunucuda değil.
22 Ekim 2026'da, aday olarak, alpha.99'un yayın kaydından sonra `main` üzerinden
`feat/alpha100-removal-and-setup` dalında yazıldı. Ön sürüm numarası üç basamaklı
olan ilk sürümdür.

## Kim güncellemeli ve neden

Yayımlandığında alpha.99 ya da daha eski bir sürümü çalıştıran herkes. alpha.99'dan
sonra sahip, Bileşenler sayfası için önerilen bir yeniden tasarıma baktı ve neden
daha iyi olacağını sordu. Büyük kısmı daha iyi olmayacaktı (sayfa sahibin seçtiği
görünümü zaten hatırlıyor ve kataloğu katlıyor); bu yüzden geniş yeniden tasarım
geri çekildi. Sahip o incelemeden üç şeyi, yalnız onları onayladı (22 Ekim 2026,
"1 evet, 2 b, 3 evet"; D-037 değişikliği):

1. kaldırma penceresi, bir bileşeni kaldırmanın bu sunucuda neyi etkileyeceğini,
   neyin silindiğini ve neyin kaldığını adıyla söyler;
2. Bileşenler'in ilk ziyareti, D-010'un hep söylediği gibi Kurulu görünümü açar;
3. Sunucu kurulumu Ayarlar'dan çıkıp Sunucu altında, Bileşenler'in hemen altında
   kendi sayfası olur.

Bir sunucuyu yalnız sahibi, panelin kendi güncelleme ekranından günceller.

## Ekranda değişenler

**Kaldırma penceresi.** Bileşenler → bir satırdaki çöp kutusu düğmesi → "X
kaldırılsın mı?". Pencere açılınca kaldırmanın neyi etkileyeceğini bir kez okur;
okurken dönen bir simgeyle "X kaldırılınca bu sunucuda neyin etkileneceği
okunuyor…" der ve Kaldır kullanılabilir kalır. Ardından üç bölüm gösterir:

- **Bu sunucuda etkilenecekler** (önce): panelin bileşen üzerinde
  barındırdıkları, adıyla, en çok beşi ve ardından "+N tane daha" — web sunucusu
  için siteler, bir çalışma ortamı için PHP ya da Node.js siteleri, bir veritabanı
  motoru için veritabanları, Postfix ve Dovecot için e-posta adresleri ve
  yönlendirmeler, Rspamd ya da SpamAssassin için postası artık spam denetiminden
  geçmeyecek posta alan adları, Roundcube için web postasını kaybedecek posta
  kutuları WireGuard için artık bağlanamayacak VPN eşleri, vsftpd için FTP hesapları ve certbot için panelin onunla yenilediği sertifikalar (kendi sertifikanızı sunmuyorsanız panelinki, sunucu kurulumunun kaydettiği posta sunucusu sertifikaları, otomatik yenilenen site sertifikaları), "Yenilenmeleri durur." cümlesiyle. Bunlar panelin
  kaldırmayı reddetmesine yol açıyorsa kalın bir satır "Bunlar varken CelikPanel X
  bileşenini kaldırmaz: önce taşıyın ya da silin." der. Güvenlik duvarı motoru
  için: panel güvenlik duvarının durumu (açık ve N açık port, kapalı ya da bilinmiyor) ve "CelikPanel onu artık uygulayamaz, değiştiremez ve geri yükleyemez; yüklenmiş kurallar sunucu yeniden başlayana dek kalabilir."  Redis, Valkey
  ve Memcached için: "CelikPanel, X bileşenini hangi sitelerin kullandığını
  kaydetmez." certbot ve güvenlik duvarı motoru için: "Sunucu kurulumu, hazırladığı
  her sunucuda X bileşenini gerektirir…". Ardından ona ihtiyaç duyan kurulu
  bileşenler ("X bileşenine ihtiyaç duyan ve çalışmayı durduracak bileşenler: …"),
  eksik bıraktığı hazır paketler (""Nginx + PHP-FPM" hazır paketi eksik kalır.")
  ve güvenlik duvarından istediği portlar. Yalnız bunların hiçbiri yoksa: "CelikPanel bu sunucuda X bileşenine bağlı başka hiçbir şey kaydetmiyor." 
- **Silinenler:** panelin kaldırmasının bu bileşen için gerçekten yaptığı
  (aşağıda), paketler ve apt ile pacman'da paket yöneticisi komutu. Bir veritabanı
  motorunda ardından bir uyarı satırı gelir: veritabanı dosyaları paketin soru
  sormadan çalışan kendi kaldırma betiğince silinebilir; önce yedek alın.
- **Yerinde kalanlar:** CelikPanel kendisi başka hiçbir şey silmez; bir paketin
  kendi kaldırma betiği o paketin oluşturduğu verileri silebilir ve bu, dağıtım
  paketinin kararıdır.

Okuma başarısız olursa ya da 10 saniyeyi aşarsa: "X kaldırılınca bu sunucuda neyin
etkileneceği okunamadı. Hiçbir şey değiştirilmedi. Yine de kaldırabilirsiniz, ama
önce kontrol edin: Alan Adları ve Veritabanları sayfaları bu sunucuda neyin
çalıştığını gösterir." ve Tekrar dene (yalnız okur); eski genel uyarı altında
kalır. Panelin asla kaldırmadığı yerel cron, pencere açılır açılmaz reddini
gösterir. Kaldır düğmesi yine çerçeveli kırmızı düğmedir; pencerede başka hiçbir
şey kırmızı değildir.

Pencerenin silinen ve kalan olarak söyledikleri, bu sürümün kodundan:

| Bileşen | Silinenler | Yerinde kalanlar |
| --- | --- | --- |
| Paketli bileşen | Katalog hizmet adlandırıyorsa hizmetleri durdurulur ve açılışta başlaması kapatılır (certbot, güvenlik duvarı motoru, phpMyAdmin ve phpPgAdmin'in yoktur). apt'ta `apt-get purge -y --auto-remove` paketleri, yapılandırma dosyalarını ve artık hiçbir şeyin ihtiyaç duymadığı, otomatik kurulmuş her paketi kaldırır — yalnız onun için kurulanları değil, sistemin tamamında. pacman'da `pacman -Rns --noconfirm` paketleri, yapılandırma dosyalarını ve başka hiçbir şeyin ihtiyaç duymadığı bağımlılıklarını kaldırır. Güvenlik duvarı açıksa kurulu kalanlara göre yeniden ayarlanır. | Site dosyalarına, posta kutularına ve veritabanı dosyalarına CelikPanel dokunmaz. Bir paketin soru sormadan çalışan kendi kaldırma betiği oluşturduğu verileri silebilir; hangileri olduğu iddia edilmez. |
| apt'ta PHP-FPM, PostgreSQL | Yukarıdaki gibi; ayrıca kurulu her sürüm (`php8.3-fpm`, `postgresql-17`, …) ve her birinin yanında kurulan paketler. | Yukarıdaki gibi; veritabanı motoru önce yedek alınmasını söyleyen uyarıyı ekler. |
| Rspamd, SpamAssassin | Yukarıdaki gibi; Postfix postayı artık ondan geçirmez. | Posta kalır; artık spam denetiminden geçmez. |
| phpMyAdmin, phpPgAdmin | Yukarıdaki gibi; panel onun sayfalarını artık sunmaz. | Yukarıdaki gibi. |
| certbot | Yukarıdaki gibi. Panelin onunla yenilediği sertifikaların yenilenmesi durur. | Yukarıdaki gibi. |
| WireGuard | Yukarıdaki gibi. Panelin etkin tuttuğu VPN eşleri artık bağlanamaz. | Yukarıdaki gibi. |
| Güvenlik duvarı motoru (nftables) | Yukarıdaki gibi. CelikPanel panel güvenlik duvarını artık uygulayamaz, değiştiremez ve geri yükleyemez. | Yüklenmiş kurallar sunucu yeniden başlayana dek kalabilir. |
| Bu paket yöneticisinde katalog paketi yok (pacman'da Redis, SpamAssassin, phpPgAdmin) | Hiçbir şey: Kaldır reddedilir ("bu sistemde henüz otomatik olarak kaldırılamaz"). | Her şey. |
| Roundcube | Uygulama dosyaları, kendi ayarları ve adres defteri veritabanı ile `/webmail/` kaydı. | Posta kutuları ve Maildir verileri, diğer siteler ile Nginx ve PHP-FPM paketleri. |

**Bileşenler'in ilk ziyareti** "Kurulu" görünümü açar. "Katalog" yalnız daha önce
onu seçtiyseniz açılır (seçim önceki gibi tarayıcıda hatırlanır). Kalıntı
temizliğini öneren Roundcube satırı Kurulu görünümde görünür kalır.

**Sunucu kurulumu**, Bileşenler'in hemen altında bir Sunucu öğesidir
(`/server-setup`, yalnız yöneticiler): sunucunun kullanım amacı, kurulumun
durumu, "Bu sunucunun kurulumunu genişletin" ve kurulum sihirbazı. Pano'daki
"Sunucunun kullanım amacı" kartında "Sunucu kurulumunu aç" düğmesi vardır. Ayarlar
→ Sunucu kurulumu'na giden eski bir bağlantı ya da yer imi oraya ulaşır. Ayarlar'da
artık üç bölüm var: Panel adresi ve HTTPS, Lisans, CelikPanel güncellemeleri.

## Altta değişenler

- **Pencere için salt-okunur bir yanıt:** `GET /api/v1/service/removal-impact?service_id=…`
  (`/api/v1/service/` önekiyle yalnız yöneticiler; `Cache-Control: no-store`).
  Kataloğu, saklanan bileşen kaydını, panel veritabanını ve panel sertifikası
  dosyasını okur; yeni Agent çağrısı ve değişiklik yoktur (sunucu profili
  önbellekteki Agent bilgilerinden gelebilir; güvenlik duvarı motoru için duvarın
  durumu var olan okumayla, 5 saniyeyle sınırlı okunur). Henüz saklı kayıt yoksa:
  `SERVICE_STATE_UNVERIFIED`. DNS motorları `DNS_ENGINE_WORKFLOW_REQUIRED`, yerel
  cron `NATIVE_CRON_REMOVAL_REFUSED` ile yanıtlanır.
- **Tek koşul kümesi.** Kaldırma işleyicisi ve Agent'ın kaldırması artık yanıtın
  bildirdiği aynı küçük soruları sorar (Postfix'in süzgeç zinciri yeniden kurulur
  mu, veritabanı araçları yeniden ayarlanır mı ve
  `core.UninstallPurgesAllVersions`); böylece pencere kaldırmanın yapmadığı bir şeyi
  vaat edemez. Kaldırmanın yaptığı hiçbir şey değişmedi.
- **Bekçi aynıdır.** Bir kaldırmayı engelleyen barındırılanlar var olan bekçinin
  kendi yanıtıdır (`SERVICE_HAS_DEPENDENTS`); spam süzgeci için posta alan adları ve
  Roundcube için posta kutuları yalnız pencere için sayılır ve asla engellemez.
- **Sunucu kurulumunun yönlendirmesi.** Panel'in, sunucu kurulumunun yönettiği bir
  adres kaydı için verdiği ret artık `/server-setup` eylemini taşır.
- **99'dan sonra 100 sürüm sırası**, önyükleme sabitleri ve README sabitleri.

## Üç basamaklı sürüm denetimi

alpha.100 üç basamaklı ilk ön sürüm numarasıdır; düz metin olarak "alpha.100",
"alpha.99"dan önce sıralanır. Bir CelikPanel sürümünü ayrıştıran, sıralayan ya da
karşılaştıran her yer okundu:

| Yer | Bulgu | Eklenen test |
| --- | --- | --- |
| Agent güncelleyicisi (`cmd/agent/system_update_contract.go`, SemVer önceliği; sıra tabanı) | sayısal | iki yönde alpha.100 > alpha.99, 1000 > 999, baştaki sıfır reddedilir, manifest ve taban gidiş-dönüşü, taban 99'un üstünde 100'ü kabul eder ve 100'ün altında 99'u reddeder; alpha.99 yapısına alpha.100 önerilir ve başlatabilir, alpha.100 yapısına asla alpha.99 |
| Panel güncelleme keşfi (`comparePanelUpdateVersions`, `validPanelUpdateVersion`) | sayısal | iki yön, eşitlik, 1000 > 999, doğrulama |
| Sürüm sırası ilkesi testi (`sequence_gt`, sabit genişlik) | sayısal | iki yönde 100 > 99; v0.1.0-alpha.100 kanonik, alpha.0100 değil |
| Sürüm-kurtarma temeli (`release_recovery_sequence_gt`: önce uzunluk, sonra rakamlar) | sayısal | 100 > 99, 99 > 100 değil, 100 > 100 değil |
| `get.sh` (sıra tabanı ve sabitler) | sayısal `-ge`; sürümler yalnız eşitlikle karşılaştırılır | sözleşme testleri 100'ü sabitler |
| CI etiket denetimi, portal oluşturucu ve yükseltici, imzalı manifest yazıcı | yalnız doğrulama ve eşitlik | sabitlenmiş sözleşmeler 100 ile geçer |
| Web güncelleme hedefi çözücüsü | yalnız doğrulama; sayfa sürümleri hiç sıralamaz | alpha.100 ve sıra 100'ü okur, 0100 sırasını reddeder |
| Sürüm-kurtarma düzeneği (Python) | sabit laboratuvar sürümleri, güncel sürüm sıralanmaz | gerekmedi |

Sürümleri metin olarak sıralayan bir yer bulunmadı.

## Değişmeyenler

Bileşenler sayfasının geri kalanı (satırlar, düğmeler — yeniden başlat, durdur,
Yönet ve kaldır görünür kalır, "⋯" menüsü yok —, hazır paket şeritleri, rozetler,
katlama), kaldırmanın kendisi, her ret, DNS motorlarının kendi akışı, roller,
Agent'ın davranışı, güncelleyici, şema (44, geçiş yok) ve yoklama. Sahibin 16 Ekim
2026 talimatıyla (güncelleyicinin değişiklik yolu değişmedikçe yerel kayıt yok)
yerel kayıt yapılmaz.

## İnceleme düzeltmeleri

Adayın bağımsız incelemesi kaldırma davranışını değişmemiş buldu ve her yayından
önce şunları istedi; kaynaktadırlar: certbot ve güvenlik duvarı motoru için onlara
hiçbir şeyin bağlı olmadığı söyleniyordu (artık yenilenmesi duracak sertifikaları,
panel güvenlik duvarını ve sunucu kurulumunun gereksinimini adlandırırlar);
önbellekler artık yalnız CelikPanel'in onları hangi sitelerin kullandığını
kaydetmediğini söyler; apt'ın sistem genelindeki `--auto-remove`'u ve pacman'ın
bağımlılık kaldırması aileye göre söylenir; veritabanı uyarısı silinenlerin
altında kendi satırıdır; birimi olmayan bir araç için durdurulduğu söylenmez;
katalog paketi olmayan bir paket ailesinde kaldırmanın reddedileceği, purge
olmadan söylenir; Roundcube'un kalanlar cümlesi Nginx ve PHP-FPM paketlerini
adlandırır ve başarısız okuması kaldırmanın sildiğini gösterir; Agent testleri
koşturulur ve kaydedilir. İkinci, daha kısa bir tarayıcı bakışı (Nginx,
PostgreSQL, Postfix, certbot ve güvenlik duvarı motoru için kaldırma penceresi,
açık tutulan ve reddedilen okuma; 4 yapılandırma, 28 ekran görüntüsü) başka bir
şey bulmadı; ekran görüntüleri ve sürücü silindi.

İkinci bir inceleme turu daha fazlasını istedi; bunlar da kaynaktadır: kendi
hatasını taşıyan bir güvenlik duvarı yanıtı ya da okunamayan bir motor
"bilinmiyor"dur; güvenlik duvarı motorunun cümlesi CelikPanel'in panel güvenlik
duvarını artık uygulayamayacağını, değiştiremeyeceğini ve geri yükleyemeyeceğini,
yüklenmiş kuralların sunucu yeniden başlayana dek kalabileceğini söyler (panel
onları `nft -f` ile yükler); WireGuard artık bağlanamayacak VPN eşlerini, vsftpd
panelin kaydettiği FTP hesaplarını adlandırır; certbot yalnız yenileme işinin
yenilediği site sertifikalarını (Let's Encrypt, otomatik yenileme açık, etkin)
sayar, sahibin sunduğu panel sertifikasını dışarıda bırakır ve bir posta
sunucusu sertifikasını yalnız sunucu kurulumu adımını başarılı olarak
kaydettiyse adlandırır; güvenlik duvarı okuması bir güvenlik duvarı
değişikliğini asla beklemez; boş durum artık "CelikPanel bu sunucuda X
bileşenine bağlı başka hiçbir şey kaydetmiyor" der.

## Bilinen açıklar

- Kurulu bir sunucuda hiçbir şey ölçülmedi.
- Bir dağıtım paketinin kendi kaldırma betiğinin sildikleri (örneğin purge
  sırasında bir veritabanının veri dizini) okunmaz ve iddia edilmez; pencere kimin
  karar verdiğini söyler.
- Pencerenin listeleri panelin kaydettiklerinden gelir. Panel dışında
  yapılandırılmış ve bir bileşeni kullanan bir şeyi panel bilmez.
- Ayarlar'daki "Sertifika al" düğmesi telefonda 34 px yüksekliğindedir (bu sürümden
  eski, değişmedi).
- Web güncelleme çözücüsü sayısal bir ön sürüm tanımlayıcısında baştaki sıfırı
  kabul eder; Panel ve Agent sayfaya bir şey ulaşmadan önce onu reddeder.

## Kanıt

Aday ağacında:

- Go: `CelikPanel-S2-Debian` konuğunda `go test ./cmd/panel/ ./internal/...` (-timeout 45m, CGO kapalı): 64 paket tamam, 2913 üst düzey test geçti, 42 atlandı, 0 başarısız (4282 alt test geçti); temiz bir mount ad alanında `go test ./cmd/agent/...` (`/var/lib/celikpanel-agent-private` ve `/run/celikpanel` üzerine boş bağlamalar, sunucunun `celikpanel` grubu olmadan bir `/etc/group` kopyası; sunucuya dokunulmadı): 1821 üst düzey test geçti, 29 atlandı, 0 başarısız (2838 alt test geçti); gofmt temiz ve `go vet ./cmd/... ./internal/...` temiz.
- Web: `npm test` 1740 testin 1740'ı geçti (yeni: `tests/removal-dialog-alpha100.test.mjs`, 20 test; `tests/server-setup-page-alpha100.test.mjs`, 10 test;
  `tests/panel-update-discovery-mounted.test.mjs` içinde üç basamaklı sürüm durumu);
  `npm run build` her bütçenin içinde (kritik açılış yolu 361 KiB'nin
  331,08 KiB'i; sınırlar yükseltilmedi).
- Kabuk: sürüm sırası ilkesi sözleşmesi (`archlinux` konuğunda taze klonda: "99 ->
  100; historical bootstrap sequence 99"), indirme portalı sözleşmesi ve imzalı sürüm
  manifesti sözleşmesi geçti.
- Tarayıcı grubu 25: 8 yapılandırmanın her birinde 13 durum (Türkçe ve İngilizce,
  açık ve koyu, 1440 ve 390 genişlik; 104 ekran görüntüsü), iki kez bakıldı. İlk bakış
  telefonda "okunamadı" bildiriminin Tekrar dene düğmesini 34 px buldu (paylaşılan
  bildirim artık en az 44 px), sayfasında iki kez yazılan "Sunucu kurulumu"nu ve ters
  okunan gereksinim satırını buldu; hepsi düzeltildi. İkinci bakış başka bir şey
  bulmadı. Ekran görüntüleri ve tarayıcı sürücüsü sonra silindi.
- Sürüm-kurtarma düzeneği gerekmiyordu (güncellemeyle ilgili hiçbir dosya ya da
  güncelleme metni değişmedi); yine de taze klonda koşturuldu ve geçti (1109 test,
  1 atlandı).

## Nasıl güncellenir

Yayımlandığında: sunucunun sahibi tarafından, panelin kendi güncelleme ekranından
(Ayarlar → CelikPanel güncellemeleri). Öncesinde ya da sonrasında yapılacak bir şey
yok.

## Yayım kaydı

Aşağıdaki her şey 22 Ekim 2026 tarihlidir (saat dilimi belirtilmedikçe UTC).
Birleştirmeyi, etiketi, denetimleri, portal yayımını ve genel sitenin okunmasını
sürümü hazırlayan kişi yaptı, önceki sürümlerde olduğu gibi sahibin kalıcı
talimatı uyarınca; kurulu panel güncellemelerini yalnız sahip, panelin kendi
güncelleme ekranından başlatır. Yayım için sahibin talimatıyla gerçek sistem
kaydı yok. Bu girdi, aşağıdaki sonuçların hiçbirini depoda bir dosya olarak
saklamaz.

**Ne oldu.**

1. Sürümden önce üç bağımsız inceleme turu yapıldı. Bulgular: kaldırma penceresi
   certbot, nftables ve WireGuard için "hiçbir şey bağlı değil" diyordu; apt'ın
   otomatik kaldırmasının ifadesi; veritabanı uyarısının yeri; bilinmeyen
   güvenlik duvarı durumunun kapalı gösterilmesi; sertifikaların sayılış biçimi;
   FTP hesaplarının SFTP hesaplarıyla karıştırılması. Hepsi düzeltildi
   (yukarıdaki "İnceleme düzeltmeleri"ne bakın) ve kaldırmanın kendi
   davranışının değişmediği doğrulandı.
2. "Release v0.1.0-alpha.100: an honest removal dialog, the installed view first,
   Server setup under Server" başlıklı #238 numaralı çekme isteği `main`'e
   `3eeff24561799fb028c3c14f5da55a5de5bba866` commit'i olarak
   birleştirildi; yazarı Mehmet Ömer Efe Çelik. Sürüm sırası değişkeni etiket
   yapılmadan önce 99'dan 100'e geçirildi. `main` koşusu ve etiket koşusu başarılı
   oldu. **Hiçbir sürümü etkilemeyen bir olay:** ilk CI koşusu Go araç zinciri
   sözleşmesinde durdu, çünkü kararlar kaydında düz bir test komut satırı vardı ve
   o sözleşme bunu yasaklar; cümle yeniden yazıldı ve zincir yeniden çalıştırıldı.
3. `v0.1.0-alpha.100` açıklamalı etiketi (nesne
   `7fc97f6efbc222d370b3518fa3fbde3fa854601f`, ağaç
   `ffd7b6b6cbf90e9da0975bf298712d6a7db6268b`, etiketleyen Mehmet Ömer Efe Çelik)
   sürüm commit'ini gösterir. Sürüm bir ön sürümdür (pre-release);
   2026-10-22T19:03:06Z'de altı dosyayla yayımlandı.
4. 22 Ekim 2026'da `CelikPanel-S2-Debian` WSL konuğunda bir indirmeden
   denetlendi (hazırlayan kişi; burada bir dosyada kayıtlı değil): arşivler bayt
   bayt aynı, her biri 66.343.534 bayt, SHA-256
   `2f3855ac9d31b1484b52da681b388b77fbe7ee318560244e9a7d2fcc8e2667b8`; manifest
   sıra 100, `published_at` 2026-10-22T18:17:33Z der (335 bayt, SHA-256
   `761186023e57734efd8f7006a39a66d4f46a1ed0b365e19bf2f93cf49ffbe851`); imzası
   (SHA-256 `4a50a3df77ca4fb4f4be50fff9ab2285506f5414a240215a8ef56b10f43c7058`),
   imza anahtarına (SHA-256
   `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`, öncekiyle
   aynı anahtar) karşı `openssl pkeyutl -verify -rawin -pubin` ile doğrulanır;
   iki içerik koruması da 0 döndürdü; derleme Go go1.26.5, CGO kapalı,
   `-trimpath` ile yapıldı. `libexec/get.sh` 100'ü sabitler (SHA-256
   `0d02d0e80ce4f0fefb22abefaf6472b951711d4d9c51dd347caa4c66b38d4771`); içindeki
   politika şimdiki 100, önceki 99, önceki commit
   `7ce52a17b416f8ffcf64931e08a2c3234bfa2f51` der. Arşivde 268 üye var (244
   dosya, 24 dizin). Arayüz paketi `web/dist/assets/app-DBhnHj6L.js`, 260.590
   bayt, SHA-256
   `eaac271391e68ea15f2874e941d6e9fbdb0a2149af248047b8febf5c0ac717e2`.
   `bin/panel` çalıştırılabilir dosyasında `removal-impact` var.
5. **Üç basamaklı ilk ön sürüm.** Manifest, `get.sh` sabitleri, `latest.txt`,
   `latest.json`, `index.json` ve sürüm başına yollar v0.1.0-alpha.100 ve sıra
   100'ü kesilmeden taşır; sürüm sıralaması denetlendi: her yerde sayısaldır
   (yukarıdaki "Üç basamaklı sürüm denetimi"ne bakın).
6. Portal paketi: `portal.tar.gz`, 133.059.310 bayt, SHA-256
   `f59809d953ddd1a61095acc10b8a053bb87e50904a9bafbaf23f61416510198f`, 54 girdi;
   iki kez bayt bayt aynı derlendi (önceden imzalı kip); terfi aracının kendi
   açma ve normalleştirme adımı onu kabul etti. Sayfaları yalnız
   `github.com/alicelik77/...` adresine bağlanır. Sözleşme testi 0 döndürdü;
   genel doğrulayıcının 17 testi ve terfi aracının 14 testi geçti.
7. Yaklaşık 19:16Z'de hazırlayan kişi onu yayımladı: önceki sürüm
   v0.1.0-alpha.99, hedef v0.1.0-alpha.100; durum `committed`, genel doğrulama 26
   istekten 26'sı, `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`.
8. Önceki sitenin yedeği
   `/var/www/vhosts/celikpanel.net/portal-backups/httpdocs.before-v0.1.0-alpha.100.20261022T191552Z.433631.93e26e8f07fc857f`;
   yüklenen paket
   `/var/www/vhosts/celikpanel.net/.upload-portal-20261022T191506Z-v0.1.0-alpha.100-3189fdffecf0d9b0`
   altında saklandı.
9. Yayımdan sonra dışarıdan okuma: `https://celikpanel.net/releases/latest.txt`
   v0.1.0-alpha.100 der; `latest.json` sürüm v0.1.0-alpha.100 ve sıra 100'ü,
   yukarıdaki commit ve SHA-256'yı gösterir.

**Sahibin bildirdikleri.** Bu yazılırken alpha.99 ya da alpha.100'den kurulu bir
sunucuda hiçbir şey bildirilmemişti. Sahip 22 Ekim 2026'da ayrıca Eylül çekme
isteklerinin kapatılmasını onayladı. 53'ünün tamamı (#151–#203) #204 numaralı
çekme isteği (alpha.81) aracılığıyla zaten `main`'de bulundu ve bu kanıtla
kapatıldı. Açık çekme isteği kalmadı.

**Yapılmayanlar ve yayımlamanın değiştirmedikleri.**

- Kurulu sunucuları sürümü hazırlayan kişi alpha.100'e (ya da alpha.99'a)
  güncellemedi. Onları yalnız sahip, panelin kendi güncelleme ekranından günceller.
- alpha.99 ya da alpha.100'den hiçbir şey kurulu bir sunucuda ölçülmedi: ne yeni
  kaldırma penceresi, ne Kurulu görünümün önce açılması, ne Sunucu kurulumunun
  yeni yeri, ne de alpha.99'un e-posta değiştirmesi.
- Bir dağıtım paketinin kendi kaldırma betiğinin sildikleri okunmaz ve panel
  dışında yapılandırılan bir şey pencerenin listelerinde yoktur (yukarıdaki
  "Bilinen açıklar"a bakın).
- Kaldırma penceresindeki pacman `-Rns` ifadesi biraz geniş. Zararsızdır ve olduğu
  gibi bırakıldı.
- Kurulum planının iletişim engeli her plan için hâlâ geçerli. Öyle olması gerekip
  gerekmediği sonraki bir karara bırakıldı.
- Bildirilmeyen ve kaydedilmeyenler: Boston ya da Frankfurt'tan alpha.99 veya
  alpha.100'e dair her şey, Frankfurt'un paket sürümleri ve önceki notların
  bildirilmedi diye saydığı her şey. Her biri "kaydedilmedi"dir; "çalıştı" da
  değildir, "başarısız" da.
- Önceki yayımlama kayıtlarının sınırları ([alpha.88
  notları](RELEASE-NOTES-v0.1.0-alpha.88.tr.md)) kalkmış olarak bildirilmedi.
- Yayımlanma kabul değildir. P0.1–P0.5 kısmi kalır; hiçbir P0 işi kapanmaz ya da
  ilerlemez.
