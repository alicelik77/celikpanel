# v0.1.0-alpha.100 (aday, yayımlanmadı)

[English](RELEASE-NOTES-v0.1.0-alpha.100.md)

Bu, v0.1.0-alpha.99'dan sonra gelen sürüm adayıdır. Yayımlanmadı; hiçbir parçası
kurulu bir sunucuda değil. 22 Ekim 2026'da, alpha.99'un yayın kaydından sonra
`main` üzerinden `feat/alpha100-removal-and-setup` dalında yazıldı. Ön sürüm
numarası üç basamaklı olan ilk sürümdür.

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
  kutuları. Bunlar panelin kaldırmayı reddetmesine yol açıyorsa kalın bir satır
  "Bunlar varken CelikPanel X bileşenini kaldırmaz: önce taşıyın ya da silin."
  der. Ardından ona ihtiyaç duyan kurulu bileşenler ("X bileşenine ihtiyaç duyan
  ve çalışmayı durduracak bileşenler: …"), eksik bıraktığı hazır paketler
  (""Nginx + PHP-FPM" hazır paketi eksik kalır.") ve güvenlik duvarından istediği
  portlar. Bunların hiçbiri yoksa: "Bu sunucuda başka hiçbir şey X bileşenine
  bağlı değil."
- **Silinenler:** panelin kaldırmasının bu bileşen için gerçekten yaptığı
  (aşağıda), paketler ve apt ile pacman'da paket yöneticisi komutu.
- **Yerinde kalanlar:** CelikPanel kendisi başka hiçbir şey silmez; bir paketin
  kendi kaldırma betiği o paketin oluşturduğu verileri silebilir ve bu, dağıtım
  paketinin kararıdır. Bir veritabanı motorunda: önce veritabanı dosyalarının
  yedeğini alın.

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
| Paketli bileşen | Hizmetleri durdurulur ve açılışta başlaması kapatılır; paketleri `apt-get purge -y --auto-remove` ya da `pacman -Rns --noconfirm` ile kaldırılır: paketler, yapılandırma dosyaları ve yalnız onlar için kurulmuş paketler. Güvenlik duvarı açıksa kurulu kalanlara göre yeniden ayarlanır. | Site dosyalarına, posta kutularına ve veritabanı dosyalarına CelikPanel dokunmaz. Bir paketin soru sormadan çalışan kendi kaldırma betiği oluşturduğu verileri silebilir; hangileri olduğu iddia edilmez. |
| apt'ta PHP-FPM, PostgreSQL | Yukarıdaki gibi; ayrıca kurulu her sürüm (`php8.3-fpm`, `postgresql-17`, …) ve her birinin yanında kurulan paketler. | Yukarıdaki gibi; veritabanı motoru "önce yedek alın" ekler. |
| Rspamd, SpamAssassin | Yukarıdaki gibi; Postfix postayı artık ondan geçirmez. | Posta kalır; artık spam denetiminden geçmez. |
| phpMyAdmin, phpPgAdmin | Yukarıdaki gibi; panel onun sayfalarını artık sunmaz. | Yukarıdaki gibi. |
| Roundcube | Uygulama dosyaları, kendi ayarları ve adres defteri veritabanı ile `/webmail/` kaydı. | Posta kutuları ve Maildir verileri, diğer siteler, Nginx, PHP-FPM ve paketler. |

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
  Kataloğu, saklanan bileşen kaydını ve panel veritabanını okur; Agent'a sormaz,
  tarama yapmaz, yazmaz. Henüz saklı kayıt yoksa: `SERVICE_STATE_UNVERIFIED`. DNS
  motorları `DNS_ENGINE_WORKFLOW_REQUIRED`, yerel cron
  `NATIVE_CRON_REMOVAL_REFUSED` ile yanıtlanır.
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

- Go: `CelikPanel-S2-Debian` konuğunda `go test ./cmd/panel/ ./internal/...` (-timeout 45m, CGO kapalı): 64 paket tamam, 2904 üst düzey test geçti, 42 atlandı, 0 başarısız (4282 alt test geçti); gofmt temiz ve `go vet ./cmd/... ./internal/...` temiz.
- Web: `npm test` 1734 testin 1734'ü geçti (yeni: `tests/removal-dialog-alpha100.test.mjs`,
  14 test; `tests/server-setup-page-alpha100.test.mjs`, 10 test;
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
