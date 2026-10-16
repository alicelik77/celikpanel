# v0.1.0-alpha.92 (aday, yayımlanmadı)

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

Yayımlanmadı.

Bu sürümü yalnız CelikPanel'in güncelleme arayüzünden kurun. Yayımlamak kurulu
sunucuları güncellemez; bir sunucunun güncellemesini yalnız sahibi, panelin
kendi güncelleme ekranından başlatır.
