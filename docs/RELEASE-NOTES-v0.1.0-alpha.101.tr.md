# v0.1.0-alpha.101 (22 Ekim 2026'da yayımlandı)

[English](RELEASE-NOTES-v0.1.0-alpha.101.md)

Bu sürüm v0.1.0-alpha.100'ün ardından gelir. 22 Ekim 2026'da yayımlandı (sonda
"Yayım kaydı"na bakın); henüz hiçbir parçası bir kurulu sunucuda değil.
23 Ekim 2026'da (kod 22 Ekim 2026'da), aday olarak, alpha.100'ün yayın kaydından
sonra `main` üzerinden `fix/alpha101-real-bundles` dalında yazıldı.

## Kim güncellemeli ve neden

Yayımlandığında alpha.100 ya da daha eski bir sürümü çalıştıran herkes. 22 Ekim
2026'da sahip iki kurulu sunucuya da alpha.100'de baktı. Frankfurt'ta Bileşenler
sayfası, her biri tek bileşen olan bir "PostgreSQL" paketi ve bir "MariaDB" paketi
sunuyordu; bunlar hemen aşağıda kendi satırı olan bileşenlerin aynısıydı.
phpPgAdmin ve phpMyAdmin ise ayrı eklentilerdi. Sahibin kararı, kendi sözleriyle:
"bundle diyip tek servisi kurmak nedir. saçmalık. postgresql ve phpPgAdmin bundle
dır mariadb ve phpMyAdmin bundle dır. Diğer tüm bundle lar da böyle dir." Hazır
paket artık bir hizmet ile ona eşlik eden araçlardır (D-034 değişikliği). Sahibin
Frankfurt'ta gördüğü iki küçük şey de düzeltildi: kurulu paketin penceresi her
parçanın yanında "Zaten kurulu" diyordu ve phpMyAdmin'in sürümü "5.2.2-really…"
okunuyordu.

Sahip Boston'da kenar çubuğundaki Sunucu kurulumuna, Pano'daki kullanım amacı
düğmesine, PostgreSQL kaldırma penceresine ve E-posta adresim kartına da baktı; bu
sürüm onları değiştirmez.

Bir sunucuyu yalnız sahibi, panelin kendi güncelleme ekranından günceller.

## Ekranda değişenler

**Hazır paketler (Bileşenler'de her kategorinin başında; Pano kartı).** Liste her
sunucuda aynıdır ve şu sıradadır:

| Kategori | Paket | Kurduğu, sırasıyla |
| --- | --- | --- |
| Posta | Temel Posta | Postfix, Dovecot |
| Posta | Webmail | Postfix, Dovecot, Nginx, PHP-FPM, Roundcube |
| Posta | Spam Korumalı Posta | Postfix, Dovecot, Rspamd |
| Veritabanı | **PostgreSQL + phpPgAdmin** | PostgreSQL, Nginx, PHP-FPM, phpPgAdmin |
| Veritabanı | **MariaDB + phpMyAdmin** | MariaDB, Nginx, PHP-FPM, phpMyAdmin |
| Web | Nginx + PHP-FPM | Nginx, PHP-FPM |

Tek bileşenli "PostgreSQL" ve "MariaDB" paketleri ve ayrı "phpPgAdmin" ve
"phpMyAdmin" eklentileri kaldırıldı; kurdukları şeyler iki veritabanı paketinin
içindedir. Posta paketleri ve Nginx + PHP-FPM değişmedi. Sekiz yerine altı paket
çizilir ve sayılır.

- **Bir veritabanı paketini kurmak**, her bileşen paketinde olduğu gibi işler:
  pencere korunanları, gerekmeyenleri ve kurulacakları sıralar, sonra bileşenleri
  birer birer kurar ve ilk hatada nedeni ve Devam et ile durur. Kurulu hiçbir şey
  kaldırılmaz.
- **Apache'nin yanında** veritabanı paketleri kurulabilir kalır: web sunucusu
  rolünü zaten Apache doldurur; bu yüzden Nginx çakışma olarak değil, "gerekmiyor"
  olarak sıralanır ve diğer parçaları kurulu olan bir veritabanı paketi Nginx
  olmadan kurulu ve doğrulanmıştır. "Nginx + PHP-FPM" Nginx'in kendisini adlandırır
  ve önceki gibi Apache'nin yanında engelli kalır.
- **Arch Linux'ta** phpPgAdmin'in paketi yoktur; bu yüzden "PostgreSQL +
  phpPgAdmin" bütünüyle engellidir ve çipi "phpPgAdmin bu sistemde kullanılamıyor"
  der. Bir paketin yalnız bir kısmı asla kurulmaz. "MariaDB + phpMyAdmin" orada
  kurulabilir kalır.

**Kurulu paketin penceresi.** Kurulu bir pakete basınca açılan pencere, her
parçanın yanında "Zaten kurulu" demek yerine her parçayı kendi satırının durumu ve
sürümüyle sıralar — "Çalışıyor · 1.26.3", "Kurulu · 5.2.1", "Durdu" — ve ipucunda
tam paket sürümü vardır. Doğrula düğmesi ve "Doğrula, bileşenleri yeniden okur…"
cümlesi kaldırıldı: bunu sayfanın üstündeki Yeniden tara yapar. Kapat kalır. Posta
paketleri, posta kurulumunu gerçekten yeniden çalıştıran Onar'ı korur (eksik olanı
tamamlar, hiçbir şeyi kaldırmaz). Kurulu bir bileşen paketi bir parçası çalışmadığı
için doğrulanmamışsa o parça satırının kırmızısıyla gösterilir ve pencere onu
aşağıdaki satırından başlatmayı ya da sayfanın üstündeki Yeniden tara'yı
kullanmayı ve pencerenin sunucuda hiçbir şeyi değiştirmediğini söyler.

**Bileşenler satırlarındaki sürümler.** Debian'daki bir "really" sürümü, gerçekte
dağıtılan sürüm olarak gösterilir: `5.2.2-really5.2.1-1` gibi bir sürüm,
phpMyAdmin'in Frankfurt'ta okunduğu gibi "5.2.2-really…" değil, 5.2.1 okunur. Tam
sürüm yine ipucunda ve bileşenin kendi sayfasındadır. Tek bir çalışma ortamı sürümü
olan satır artık üç noktayla biter ve tam metin ipucundadır; böylece hiçbir sürüm
iz bırakmadan kırpılmaz.

**Sahibin sunucularının göstereceği, panelin hesapladığı biçimde** (alpha.100
durumlarından; alpha.101'de henüz görülmedi):

- **Boston** (PostgreSQL ve Nginx kurulu, PHP-FPM yok, phpPgAdmin yok):
  "PostgreSQL + phpPgAdmin · Kur". Penceresi PostgreSQL ve Nginx'i korur, PHP-FPM'i,
  ardından phpPgAdmin'i birer birer kurar. Pano'daki "Hazır paketler" kartı:
  "Henüz kurulu yok · 6 kurulabilir".
- **Frankfurt** (her şey kurulu): onay işaretiyle "PostgreSQL + phpPgAdmin" ve
  "MariaDB + phpMyAdmin"; kart: "6 kurulu ve doğrulandı".

## Altta değişenler

- **Paket kaydı** (`cmd/panel/bundles.go` içindeki `bundleDefinitions`) Panel'e
  derlenir. Paketler bir dosyadan ya da veritabanından okunmaz ve panel çalışırken
  değiştirilemez; birini değiştirmek bir sürüm gerektirir. Sahip isterse ileride
  paketler için bir veri kataloğu mümkündür.
- **Kimlikler korunur, adlar değişir.** `db.postgresql` ve `db.mariadb`
  kimliklerini korur; bağlantılar yine aynı pakete varır. Ad kodları artık
  `db_postgresql_phppgadmin` ve `db_mariadb_phpmyadmin`'dir; böylece bir sayfa daha
  eski bir panelin tek bileşenli paketine asla araçlı paket demez.
  `db.postgresql.admin` ve `db.mariadb.admin` artık yoktur.
- **Önkoşullar.** Bir veritabanı aracının üzerinde çalıştığı web sunucusu ve
  PHP-FPM önkoşul olarak sıralanır: pakette doldurdukları rol için bulunurlar ve o
  rolü dolduran kurulu başka bir bileşen (Apache) onların yerine kullanılır.
- **Bir kayıt sözleşmesi testi** (Go) sahibin kuralını zorlar: her bileşen
  paketinde en az iki bileşen ve bunlardan yalnız önkoşul olmayan en az bir tane
  vardır; posta paketleri tam olarak kendi posta profilleridir; emekli kimlikler ve
  ad kodları geri gelmez.
- **Doğrula eylemi yok.** Panel, `verify` gönderdiği yerde `none` gönderir; sayfa
  daha eski bir panelden gelen `verify`'ı yine kabul eder ve onun için düğme
  çizmez. Sayfa artık yanıtın taşımadığı bir paket kimliğini, bütün listeyi
  reddetmek yerine dışarıda bırakır.
- **Sürümün kısa biçimi**, epoch ve paketleme revizyonu önceki gibi çıkarıldıktan
  sonra, "really" bir sürüm ayracından (`+`, `-`, `~` ya da `.`) sonra geliyor ve
  ardından bir rakam geliyorsa "really"den sonraki sürümü alır. Panel yanıtını
  oluştururken hesaplanır; bu yüzden güncellemeden önce okunmuş sürümler de hemen
  doğru gösterilir.
- **Kaldırma penceresi**, bir kaldırma yeni paketlerden birini eksik bırakırsa onu
  adlandırır (Nginx'i kaldırmak artık iki veritabanı paketini de eksik bırakır).
- **Sürüm sırası 100'den sonra 101**, önyükleme sabitleri ve README sabitleri.

## Değişmeyenler

Bir paketin nasıl kurulduğu (birer birer olağan bileşen kurulumu, bir hatadan sonra
Devam et, hiçbir şey kaldırılmaz), posta paketleri ve Onar'ları, Nginx + PHP-FPM,
Bileşenler sayfasının geri kalanı, her ret, roller, Agent, güncelleyici, sürüm
kurtarma, şema (44, geçiş yok) ve yoklama. Sahibin 16 Ekim 2026 talimatıyla
(güncelleyicinin değişiklik yolu değişmedikçe yerel kayıt yok) yerel kayıt
yapılmaz.

## Bilinen açıklar

- Kurulu bir sunucuda hiçbir şey ölçülmedi.
- Paketler sürümle sabittir; kendi paketinizi tanımlamanın bir yolu yoktur.
  Bir veri kataloğuna taşınıp taşınmayacakları sahibe bırakıldı.

## Kanıt

Aday ağacında:

- Go: kayıt sözleşmesi testi; Boston, Frankfurt, Apache'li bir sunucu ve bir Arch
  sunucusu biçimli paket testleri; yeni paketlerle kaldırma etkisi testi; gerçek
  Debian ve Ubuntu sürüm metinleriyle kısa biçim testleri (`4:5.2.2+dfsg-1` →
  5.2.2, `1:5.2.1+dfsg-3` → 5.2.1, `5.2.2-really5.2.1-1` → 5.2.1,
  `2:4.1.0+really4.0.9-2` → 4.0.9, `3:7.2` → 7.2 ve `~` içeren sürümler).
  Son ağaçta `CelikPanel-S2-Debian` konuğunda geçti: 64 pakette 7.199 geçti,
  44 atlandı, 0 başarısız; gofmt ve vet temiz.
- Web: hazır paket bağlı ve sözleşme testleri (Boston, Frankfurt ve Arch yükleri,
  pencerede her parçanın durumu, Doğrula yok, doğrulanmadı cümlesi, daha eski bir
  panelin `verify`'ı, dışarıda bırakılan bilinmeyen kimlikler), sürüm hücresi
  testleri ve yeni adlarla kaldırma penceresi testleri. 1.745/1.745
  geçti; derleme hiçbir bütçe değişmeden geçti.
- Tarayıcı: alpha.101 kaydıyla 19. ve 20. gruplar (1440 px ve 390 px, İngilizce ve Türkçe, açık ve koyu): Boston ve Frankfurt biçimli şeritler, Boston'ın PostgreSQL + phpPgAdmin planı ve kurulumu (2/2 adımı phpPgAdmin), Frankfurt'un kurulu paket pencereleri ve MariaDB'nin durduğu bir pencere, Pano kartı; bir kusur bulundu ve düzeltildi (sürüm durum sözcüğünden sonraki noktaya yapışıyordu; artık mono yazıyla ayrı duruyor) ve ekranlara yeniden bakıldı; ham kod yok, Türkçe sayfada İngilizce yok, yatay taşma yok. Ekran görüntüleri silindi.
- Kabuk: sürüm sırası ilkesi sözleşmesi Git Bash'te geçti ("100 -> 101;
  historical bootstrap sequence 100"); indirme portalı sözleşmesi ve imzalı sürüm
  manifesti sözleşmesi `CelikPanel-S2-Debian` konuğunda geçti.

## Nasıl güncellenir

Yayımlandığında: sunucunun sahibi tarafından, panelin kendi güncelleme ekranından
(Ayarlar → CelikPanel güncellemeleri). Öncesinde ya da sonrasında yapılacak bir şey
yok.

## Yayım kaydı

Aşağıdaki her şey 22 Ekim 2026 tarihlidir (saat dilimi belirtilmedikçe UTC). Sürüm
commit'i 2026-10-23T01:34:29+03:00 taşır; bu, 22:34:29Z ile aynı andır, bu yüzden
farklı yerlerde iki tarihle görünür. Birleştirmeyi, denetimleri, portal yayımını ve
genel sitenin okunmasını sürümü hazırlayan kişi yaptı, önceki sürümlerde olduğu
gibi sahibin kalıcı talimatı uyarınca; kurulu panel güncellemelerini yalnız sahip,
panelin kendi güncelleme ekranından başlatır. Yayım için sahibin talimatıyla gerçek
sistem kaydı yok. Bu girdi, aşağıdaki sonuçların hiçbirini depoda bir dosya olarak
saklamaz.

**Ne oldu.**

1. "Release v0.1.0-alpha.101: a bundle is a service with its companion tools"
   başlıklı #240 numaralı çekme isteği `main`'e
   `f860053bea4772f008337238e5787c857fbb5efb` commit'i olarak birleştirildi;
   yazarı Mehmet Ömer Efe Çelik. Manifestin `published_at` değeri
   2026-10-22T22:34:29Z.
2. Sürüm bir ön sürümdür (pre-release), `v0.1.0-alpha.101`; GitHub'da
   2026-10-22T23:13:34Z'de yayımlandı.
3. Hazırlayan kişi yayımdan önce denetledi (burada bir dosyada kayıtlı değil):
   arşivler 66.346.084 bayt, SHA-256
   `6de8f594705e22250c56a6c45fe6a2b10c6c1e7db270eeb706878f3069c5f549`; sağlama
   toplamları, manifest ve arşiv üyeleri denetlendi; manifestin imzası yayın
   anahtarıyla (SHA-256 `7eadeb0b…c68bb`) doğrulanır; içerik korumaları
   geçti; `libexec/get.sh` 101'i sabitler; içindeki politika 100'den sonra 101
   der; arayüz paketi `web/dist/assets/app-EnAd5I-6.js`.
4. Portal paketi iki kez derlendi ve bayt bayt aynı çıktı; terfi aracının kendi
   açma ve normalleştirme adımı onu kabul etti.
5. Yaklaşık 23:22Z'de hazırlayan kişi portalı yayımladı: durum `committed`, genel
   doğrulama 26 istekten 26'sı tamam.
6. Yayımdan sonra dışarıdan okuma: `https://celikpanel.net/releases/latest.txt`
   v0.1.0-alpha.101 der; `latest.json` sıra 101'i ve `f860053…` commit'ini
   gösterir.

**Sahibin bildirdikleri.** Sahip 22 Ekim 2026'da iki kurulu sunucuya, Boston'a ve
Frankfurt'a, alpha.100'de bakmıştı; gördüğü ekranlar bu sürümün yanıt verdiği
şeylerdir (yukarıdaki "Kim güncellemeli ve neden"e bakın). Bu yazılırken
alpha.101'den kurulu bir sunucuda hiçbir şey bildirilmemişti.

**Yapılmayanlar ve yayımlamanın değiştirmedikleri.**

- Kurulu sunucuları sürümü hazırlayan kişi alpha.101'e güncellemedi. Onları yalnız
  sahip, panelin kendi güncelleme ekranından günceller. alpha.101 henüz hiçbir
  kurulu sunucuda değil.
- alpha.101'den hiçbir şey kurulu bir sunucuda ölçülmedi: ne yeniden adlandırılan
  veritabanı paketleri, ne kurulu paketin penceresi, ne Frankfurt'un phpMyAdmin
  satırındaki düzeltilmiş sürüm, ne de Boston'ın "PostgreSQL + phpPgAdmin" planı.
  Sahibin güncelledikten sonra alpha.101'e ilişkin gözlemi henüz gelmedi.
- Kurulum planının iletişim e-postası gereksinimi her sunucu planı için geçerli.
  Öyle olması gerekip gerekmediği bir karara bırakıldı.
- Paketlerin bir veri kataloğuna taşınıp taşınmayacağı sahibe bırakıldı
  (yukarıdaki "Bilinen açıklar"a bakın).
- Bildirilmeyen ve kaydedilmeyenler: Boston ya da Frankfurt'tan alpha.101'e dair
  her şey ve önceki notların bildirilmedi diye saydığı her şey. Her biri
  "kaydedilmedi"dir; "çalıştı" da değildir, "başarısız" da. alpha.100 kaydının açık
  bıraktıkları açık kalır.
- Önceki yayımlama kayıtlarının sınırları ([alpha.88
  notları](RELEASE-NOTES-v0.1.0-alpha.88.tr.md)) kalkmış olarak bildirilmedi.
- Bu kayıtta kod, şema (44) ya da kurtarma davranışı değişmedi.
- Yayımlanma kabul değildir. P0.1–P0.5 kısmi kalır; hiçbir P0 işi kapanmaz ya da
  ilerlemez.
