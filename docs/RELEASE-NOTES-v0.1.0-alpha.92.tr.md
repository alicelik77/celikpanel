# v0.1.0-alpha.92 (16 Ekim 2026'da yayımlandı)

[English](RELEASE-NOTES-v0.1.0-alpha.92.md)

Bu sürüm v0.1.0-alpha.91'in ardından gelir. v0.1.0-alpha.91 etiketlendi
(`356109b532b9d0d0df5dd31fc19e9932918a9648`) ama indirme portalına konmadı; bu
yüzden alpha.92 alpha.91'in işini de getirir: ikinci sunucunun tam eş olması,
2. bölüm (Alan adları'nda altyapı bölgesi ve kayıt düzenleyicisi, birincil
sunucunun planında eşin posta sunucusu adı, yetkili DNS bağlantısı üzerinden
birincilden istenen sunucu kaydı; bkz.
[alpha.91 notları](RELEASE-NOTES-v0.1.0-alpha.91.tr.md)). 2026-10-16'da (UTC)
`fix/alpha92-certbot-scheduler-restore` dalında, `main`'in alpha.91 sürüm
commit'inden yazıldı.

## Ne oldu

Sahip 2026-10-16'da Frankfurt'u (Debian 13) alpha.89'dan alpha.90'a güncelledi.
Güncelleme son adımında durdu: "Certbot renewal scheduler state could not be
restored". Sayfa kırmızı bir çarpı ve ham satırı gösterdi; 16 saniye sonra
sunucunun kendi kurtarması güncellemeyi tamamladı ve sayfa eski bildirim
biçiminde, kapatma düğmesiyle "Güncelleme doğrulandı" dedi. alpha.90 kurulu ve
çalışıyor. Boston (Ubuntu) aynı güncellemeden sorunsuz geçti.

Nedeni: Debian'ın ve Ubuntu'nun `certbot.timer` birimi, yeniden açılır açılmaz
kaçırdığı çalışmayı telafi eder. Güncelleme zamanlayıcıyı yeniden açınca
Certbot yenilemesi hemen başladı; güncelleyici ise onun boşta olmasını istiyordu.
Saate göre aşağı yukarı dört beş güncellemeden biri buna denk gelir.

## Ne değişti

- **Güncelleyici sahibin yenileme çalışmasına dokunmaz.** Çalışırken yalnız
  zamanlayıcıları durdurur, zaten çalışan bir yenilemenin bitmesini bekler
  (salt-okur, en çok 180 sn) ve `certbot.service`'i asla durdurmaz, başlatmaz,
  sıfırlamaz. Zamanlayıcı yeniden açılınca başlayan ya da güncellemeden önce
  başarısız olmuş bir yenileme sahibin yenilemesidir, güncelleme hatası
  değildir. Güncelleme artık `certbot.service` başarısız durumdayken de
  başlayabilir.
- **Sunucu işi kendisi bitirirken güncelleme sayfası sakin kalır.** Son adımın
  yinelenmesi gerekirse sayfa **"Güncelleme tamamlanıyor"** der: yeni sürüm
  kurulu ve çalışıyor, son adım (otomatik sertifika yenilemeyi yeniden açma)
  sunucu tarafından yineleniyor, yapmanız gereken bir şey yok. Adımlarda
  sonuncusu "Kendiliğinden yineleniyor (deneme 1/3)" olarak görünür. Kırmızı
  işaret ve kapatma düğmesi yok; sunucunun kendi satırı "Ayrıntılar"ın içinde.
  Sunucu doğruladığında sayfa ve güncelleme kartı **"{version} sürümüne
  güncellendi"** der; yapı satırı, tamamlanmış beş adım ve "Bir adım
  kendiliğinden yinelendi; yapmanız gereken bir şey yok." satırı ile bir yeniden
  yükleme düğmesi görünür. Kırmızı, sizin işlem yapmanız gereken durumlar için
  kalır.
- **Yapı satırı göründüğü gibi kopyalanır**: kısa ve tam kimlik bitişik değil,
  "build 1743ac69edb0" (sahibin 2026-10-16 bildirimi).

## Kanıt

Yerel zamanlayıcıyı taklit eden sözleşme testleri (zamanlayıcı açılınca
yenilemesinin başlaması; güncellemeden önce başarısız ya da çalışan yenileme) ve
önceki kodda Frankfurt'un satırını yeniden üreten negatif kontroller; yeni hata
kodu, planın `retrying` adımı ve kurtarma metni için Go testleri; güncelleme
sayfası testlerde bağlanarak (yineleniyor, sonra doğrulandı, sonra duraklamış
kontrol). Bir yerel laboratuvar kaydı, set19 (Debian 13, QEMU, test lisanslı
yapılar, dışa giden 443 reddedildi): güncellemenin kendi zamanlayıcı açılışı yenilemeyi başlattı ve güncelleme hatasız `update_verified` bitti; yenileme güncellemeden önce başarısızken güncelleme aynı biçimde bitti ve başarısızlık sahibin durumu olarak kaldı; sayfa sakin kaldı (adımlar, ardından "güncellendi") ve yeni yapı satırı `build 16cbfc877bd4` olarak kopyalandı ([set19](../deploy/e2e/release-recovery/evidence/set19-20261016/README.md)).

## Ne göreceksiniz

- **alpha.90 ya da alpha.91'den alpha.92'ye güncellerken:** güncellemeyi eski
  sayfa çizer (sakin "Güncelleme tamamlanıyor" görünümü alpha.92'nin
  sayfasındadır). Düzeltilmiş güncelleyiciyle bu güncellemenin yenileme
  adımında durmaması beklenir.
- **alpha.92'den sonraki güncellemelerde:** yenileme adımının yinelenmesi
  gerekirse adımlarla birlikte "Güncelleme tamamlanıyor", ardından
  "güncellendi".

## Sınırlar

- Sakin görünüm ilk kez bir sunucu alpha.92'**den** güncellenirken görülür.
- Frankfurt güncellemesinin kaydını zaten kapatmış bir tarayıcı onun için
  "v0.1.0-alpha.90 sürümüne güncellendi" kartı tutmaz.
- set19, test lisanslı yapılarla QEMU konuklarında alınmış bir laboratuvar
  ölçümüdür, kabul kanıtı değildir. P0.1-P0.5 **kısmi** kalır; hiçbir P0 işi
  kapanmaz ya da ilerlemez; yayım kabul değildir.
- Şema 44, geçiş yok. Güncelleme anlık görüntü biçimi değişmedi.

## Yayım kaydı

Aşağıdaki her şey 16 Ekim 2026 tarihlidir (saat dilimi belirtilmedikçe UTC).
Birleştirmeyi, depo değişkenini, etiketi, denetimleri, portal yayımını ve genel
sitenin okunmasını sürümü hazırlayan kişi yaptı, sahibin 15 Ekim 2026 tarihli
talimatı uyarınca; kurulu panel güncellemelerini sahip, panelin kendi güncelleme
ekranından başlattı. Yayım için sahibin talimatıyla gerçek sistem kaydı yok
(alpha.92'nin kendi kodunu yukarıdaki set19 ölçtü). Bu girdi, aşağıdaki
sonuçların hiçbirini depoda bir dosya olarak saklamaz.

**Ne oldu.**

1. #222 numaralı çekme isteği hazırlayan kişi tarafından `main`'e squash olarak
   birleştirildi: commit `961f2cb2fcaeb48c08fc58828d15fe0b231e1e1a`, ağaç
   `091d5fb23cb16cbd217c8c1191033185a7d50b81`. Çekme isteğindeki ilk CI koşusu
   "Verify offline release-recovery fixture contracts (no VM)" işinde başarısız
   oldu: koşum düzeneğinin yansıtılmış kod listelerinde yeni
   `update_finishing_scheduler` kodu yoktu. `5f662635f` ile düzeltildi ve CI
   sonra geçti, 24 işin 24'ü. `main` üzerindeki koşu başarıyla sonuçlandı.
2. Hazırlayan kişi `CELIKPANEL_RELEASE_SEQUENCE` değişkenini 91'den 92'ye
   çıkardı ve `v0.1.0-alpha.92` açıklamalı etiketini gönderdi (nesne
   `16a8a961aa9f06948902e4c972c4ce6603cc50fc`). Etiketin koşusu başarıyla
   sonuçlandı; sürüm 2026-10-16T18:51:22Z'de altı dosyayla yayımlandı.
3. Bir indirmeden denetlendi (hazırlayan kişi; burada bir dosyada kayıtlı
   değil): arşivler bayt bayt aynı, 66.240.132 bayt, SHA-256
   `3e243da119a252988df80745d77f3e500b6856c60717fa4b2b8f94e1e54e4795`; manifest
   sıra 92, `published_at` 2026-10-16T18:09:41Z der (SHA-256 `d5824856…`), imzası
   doğrulanır (aynı anahtar); 241 üye; `libexec/get.sh` 92'yi sabitler (SHA-256
   `6c5a7b8e8145b14cd75506ab801775fad7c893c85210937243d32c9f2aeb8575`); politika
   92 der, önceki 91 (`356109b53…`); `release.commit` ve `release.tree`
   etiketinkilerle eşittir. Arşivin `deploy/panel-tls-snapshot.sh` dosyasında
   `_panel_tls_wait_service_idle` ve `_panel_tls_note_owner_failed_renewal`,
   `update.sh` dosyasında `update_finishing_scheduler` vardır. Arayüz paketi
   `app-DomwK8kh.js`, 248.892 bayt, SHA-256
   `92709cadaed9e3dfcf48b55956e0282c61eebf785a0b117c8f229a9fbacfd491`.
4. Portal paketi: 132.855.924 bayt, SHA-256
   `809e4f501a020ee54a847c580406f8698d1eb0809aa287be27316f00dd72ffcd`, 54 üye,
   iki kez aynı derlendi; terfi aracı kabul etti; testler geçti. Hazırlayan kişi
   `-PreviousVersion v0.1.0-alpha.90` ile yayımladı (canlı portal alpha.91'i hiç
   sunmadı): committed, yedek
   `httpdocs.before-v0.1.0-alpha.92.20261016T185901Z…`, doğrulama ok (26 istek),
   `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`.
5. 18:59:09Z'de dışarıdan okuma: `releases/latest.txt` v0.1.0-alpha.92, `get.sh`
   92'yi sabitliyor, `releases/latest.json` sıra 92, commit `961f2cb2…`, SHA-256
   `3e243da1…`.

**Sahibin bildirdikleri (16 Ekim 2026, iki ekran görüntüsü, yaklaşık 19:10Z;
alpha.90'dan güncelleme).** Sahip iki güncellemeyi de panelin kendi güncelleme
ekranından başlattı. İki sunucuda da Ayarlar, CelikPanel güncellemeleri
"Geçerli sürüm v0.1.0-alpha.92" ve "Geçerli commit
961f2cb2fcaeb48c08fc58828d15fe0b231e1e1a" der; kenar çubuğu "CelikPanel ·
v0.1.0-alpha.92 · build 961f2cb2fcae" der; ve "v0.1.0-alpha.92 sürümüne
güncellendi — build 961f2cb2fcae" kartı beş tamamlanmış adım sayar. Bu,
**Frankfurt için de** geçerlidir: bu kez hata yok, kurtarma yok. Bu, her
sunucuda bir güncellemedir ve Certbot düzeltmesinin kabulü değildir:
Frankfurt'un güncellemesi sırasında bir telafi yenilemesinin başlayıp
başlamadığı bildirilmedi. Güncelleme sırasında sayfanın gösterdiği (alpha.90'ın
sayfasının çizdiği canlı adım görünümü) bir ekran görüntüsüyle bildirilmedi.

**Yapılmayanlar ve yayımlamanın değiştirmedikleri.**

- Kurulu sunucuları sürümü hazırlayan kişi güncellemedi.
- Bildirilmeyen ve kaydedilmeyenler: Frankfurt'un Alan adları'ndaki "Altyapı
  bölgesi" satırı, Boston'un eş sunucu kaydıyla posta genişletmesi, lisans
  sayfası, Agent'ın sürümü, `postconf -n >/dev/null`. Her biri "kaydedilmedi"dir;
  "çalıştı" da değildir, "başarısız" da.
- Sakin "Güncelleme tamamlanıyor" görünümü ilk kez bir sunucu alpha.92'**den**
  güncellenirken görülür.
- Önceki yayımlama kayıtlarının sınırları ([alpha.88
  notları](RELEASE-NOTES-v0.1.0-alpha.88.tr.md)) kalkmış olarak bildirilmedi.
- Yayımlanma kabul değildir. P0.1–P0.5 kısmi kalır; hiçbir P0 işi kapanmaz ya da
  ilerlemez.

Bu sürümü yalnız CelikPanel'in güncelleme arayüzünden kurun. Yayımlanması
kurulu sunucuları güncellemez; bir sunucunun güncellemesini yalnız sahibi,
panelin kendi güncelleme ekranından başlatır.
