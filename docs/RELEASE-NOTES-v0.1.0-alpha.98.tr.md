# v0.1.0-alpha.98 (22 Ekim 2026'da yayımlandı)

[English](RELEASE-NOTES-v0.1.0-alpha.98.md)

Bu sürüm v0.1.0-alpha.97'nin ardından gelir. 22 Ekim 2026'da yayımlandı (sonda
"Yayım kaydı"na bakın); henüz hiçbir parçası bir kurulu sunucuda değil.
2026-10-22'de, aday olarak, `feat/alpha98-menu-layout` dalında, alpha.97'nin yayım
kaydından sonraki `main` üzerinden yazıldı.

## Kim güncellemeli ve neden

Bu sürüm yayımlandığında alpha.97'yi ya da daha eski bir sürümü çalıştıran herkes.
Sahip 2026-10-21'de Ayarlar alt menüsündeki her şeyin gerçekten ayarlarla ilgili olup
olmadığını sordu ve her işlemin yerinin doğru olması, hiçbir şeyin saçma sapan
alakasız bir yerde durmaması ve herkesin aradığını kolayca bulması gerektiğini
söyledi. Bu sürüm her sayfayı adının söylediği yere taşır (D-037). Başka hiçbir şey
değişmez: aynı sayfalar, aynı okumalar ve değişiklikler, aynı roller. Bir sunucuyu
yalnız sahibi, panelin kendi güncelleme ekranından günceller.

## Ekranda ne değişir

**Taşınan her şey şimdi nerede.**

- Sunucu güvenliği artık **Sunucu → Güvenlik**'te. "Bu sunucu güvende mi?" sorusunu tek
  yerde yanıtlar: ayarlarıyla güvenlik duvarı (aç/kapat, yeniden başlatma için kaydet,
  açık portlar, SSH satırı), bileşen sayfasına bağlantısıyla Fail2ban'ın durumu ve
  sunucunun dışa açtıklarının salt-okur denetimi. Güvenlik duvarı kutusu eskiden
  Bileşenler'deki tek satırlık güvenlik duvarı çubuğundan açılıyordu, denetim ise
  Ayarlar → Sunucu güvenliği idi.
- DNS altyapısı artık **Sunucu → DNS ve eş sunucular**: ortak ad sunucuları, bu
  sunucunun rolü, eş sunucusu ve DNS çoğaltması, eskisi gibi.
- Sistem veritabanları artık **Sunucu → Bakım**: CelikPanel'in ve kurulu servislerin
  tuttuğu SQLite dosyalarını inceleyin ve güvenle bakımını yapın, eskisi gibi.
- Hesap güvenliği artık sağ üstteki menüde (kullanıcı adı), ilk sırada **Hesabım**.
  İki faktörlü doğrulama panelini ve menünün eskiden açtığı parola penceresini açan
  bir Parola kartını tutar.
- Ayarlar'daki "Panel HTTPS" bölümü artık **Panel adresi ve HTTPS**.

**Ayarlar dört şey tutar:** Panel adresi ve HTTPS, Lisans, CelikPanel güncellemeleri
ve Sunucu kurulumu. Bunlar bir kez ya da seyrek yapılan ve panelin nasıl
davranacağını belirleyen seçimlerdir. Yönetici güvenlik kuralları bölümü eklenmedi,
çünkü böyle bir ayar yok.

**Sunucu artık şunları tutar:** Bileşenler · DNS ve eş sunucular · İzleme · Güvenlik ·
Bakım · İçe aktar · Denetim günlüğü. **Barındırma değişmedi:** Alan Adları,
Veritabanları, Kullanıcılar, Mağaza, VPN.

**İkinci düşüncelerin vardığı yer.** İlk taslak İçe aktar'ı ve Sunucu kurulumu'nu
taşıyordu; ikisi de yerinde kalır. İçe aktar Sunucu altında kalır (seyrek kullanılan
bir araç, sunucu araçları arasında aranır) ve Sunucu kurulumu Ayarlar'da kalır (bir
kez, sonra genişletmek için seyrek çalıştırılır; sunucunun amacı bir yapılandırma
seçimidir). Mağaza ve VPN, bayiler ve müşteriler kullandığı için Barındırma altında
kalır.

**Bayiler ve müşteriler artık boş bir Ayarlar görmez.** Ayarlar'larında tek bölüm,
kendi hesapları vardı. Artık menüde **Hesabım**'ları var ve menüde Ayarlar yok. Ek
kullanıcı (ekip üyesi) hiç Ayarlar'a ya da hesap bölümüne sahip olmadı; menüde
"Parolayı değiştir"i korur.

**Eski bağlantılar ve yer imleri çalışmaya devam eder.** `/settings?section=account`
`/account`'a, `?section=security` `/security`'ye, `?section=dns`
`/dns-infrastructure`'a ve `?section=system-databases` `/maintenance`'a gider; Geri
düğmesinin geçmişinde fazladan bir adım olmaz. Bayi ya da müşteri için `/settings`
Hesabım'a gider. Panel'in kendi bağlantıları (başlangıç rehberi, Alan Adları'nın "DNS
gerekli"si, Alan adı ekle penceresi, Bileşenler, sunucu kurulumunun sonraki adımı
"DNS ve eş sunucuları aç", kurulum kurtarma bağlantıları) ve yönlendirme metinleri
artık "Ayarlar" değil, yeni yeri adlandırır: "Sunucu → DNS ve eş sunucular" ya da
"Sunucu → Güvenlik".

**Önemli yerde sakin.** Sağlıklı bir güvenlik duvarı Bileşenler'de hâlâ tek sakin bir
satırdır; artık "Güvenlik'te yönet" diyen bağlantısı Güvenlik sayfasındaki güvenlik duvarını açar. Kapalı,
okunamayan, motoru olmayan ya da dikkat gerektiren bir güvenlik duvarı, önemli olduğu
anda orada olacağınız için tam kutusunu Bileşenler'de de göstermeye devam eder.
Güvenlik sayfası Fail2ban'ı şunlardan biri olarak gösterir: çalışıyor; kurulu ama
çalışmıyor (bileşen sayfasından başlatın); kurulu, çalışma durumu henüz okunmadı;
kurulu değil (Bileşenler'den kurun); henüz denetlenmedi. Okuma başarısızsa bunu söyler
ve "Tekrar dene" sunar. Sunucu meşgulken sayfa Bileşenler'deki aynı beklemeyi
gösterir ("Sunucu değişiklikleri geçici olarak kullanılamıyor.") ve onu kendiliğinden
kaldırır; yeniden yüklenecek bir şey yoktur.

## Altta ne değişir

- **Yalnız arayüz.** Menü, üç yeni sayfa (`/dns-infrastructure`, `/security`,
  `/maintenance`), Hesabım (`/account`), dört bölümlü bir Ayarlar, yönlendirmeler ve
  eski yerleri adlandıran bağlantılar ile metinler. Simgeler açılış paketindeki lucide
  kümesinden gelir. Kişisel menü bir açılır bölümdür: `aria-expanded`, Escape onu
  kapatır ve odağı düğmesine geri verir.
- **Parola penceresi** her sayfa yüklenişinde değil, açıldığında getirilir; sözleri
  kabuk kataloğunda kalır, bu yüzden asla ham anahtar göstermez.
- **Yalnız ekranların gösterdiği kodlu retler** kabuk kataloğundan "errors" adlı bir
  ekran parçasına taşındı; bu parça diğerleriyle birlikte, hiçbir ekran çizilmeden
  önce yüklenir. Kabuğun ondan önce gösterebildikleri kabukta kalır; sözleri henüz
  gelmemiş bir ret sunucunun cümlesini ya da genel hatayı gösterir, asla anahtarını.
- **Güvenlik duvarı kutusunun kendi modülü var** ve **bir bileşenin çalışıp
  çalışmadığını her yerde tek kural söyler**: "running" ya da "(running)" diyen
  durum; "active (exited)" çalışmıyordur.
- **Sunucudaki yönlendirme eylemi adresleri.** `/settings?section=dns` gösteren her
  yönlendirme eylemi (DNS motoru iş akışı, DNSSEC, yalnız-DNS çocukları, içe aktarma,
  uzak DNS yetkisi, DNS kurulumu) artık `/dns-infrastructure`'ı gösterir; Panel
  sertifikasının eksik iletişim e-postası artık eylem taşımaz (yöneticinin kendi
  e-postasını değiştiren bir ekran henüz yok; Bilinen eksikler'e bakın). Ayarlar'ı ya da
  DNS altyapısı ayarlarını adlandıran iki İngilizce yedek ileti artık Sunucu → DNS ve
  eş sunucular der. Bunlar yanıtlardaki metinlerdir; eski bir Panel'in bağlantıları da
  çalışır, çünkü tarayıcı onları yönlendirir.
- **Kurulum geçidi**, kurulum gerekirken `/account`, `/security`, `/dns-infrastructure`
  ve `/maintenance`'ı `/settings` gibi erişilebilir tutar; çevrimdışı kurtarma
  kabuğunun rota listesi de onları içerir.
- **Genel portalın bağlantıları** (`download-portal/index.html`, `technical.html`,
  `security.txt`, `SECURITY.md` ve portal sözleşme testi), sahibin GitHub hesabının
  adını 2026-10-21'de `celikbros` adından `alicelik77` adına değiştirmesinden sonra artık
  `github.com/alicelik77/...` adresini gösterir (eski adresler kalıcı olarak
  yönlendirilir; geri bildirim deposu `alicelik77/celikpanel-feedback`). Portal bir
  sonraki yayımlandığında etkili olurlar. `deploy/e2e/**/evidence/**` altındaki kayıtlı
  kanıt dosyaları tarihseldir ve dokunulmadı.

## Neyin değişmediği

- API, Agent, güncelleyici, şema, rol, yoklama ya da bileşen davranışında değişiklik
  yok. Aynı bileşenler, aynı okumalar, aynı testlerin anlamı. Her sayfanın ve API'nin
  rolleri değişmedi; yalnız yer taşındı.
- Güncelleyicinin değiştirme yoluna (`update.sh`, `get.sh`, sistem güncelleme işçisi,
  geri alma ve kurtarma) dokunulmaz, bu yüzden sahibin 16 Ekim 2026 talimatı uyarınca
  gerçek sistem kaydı yapılmaz.
- Şema 44, geçiş yok. Kalıcı hiçbir şey değişmez.
- Barındırılan web siteleri, posta, DNS, veritabanları, zamanlanmış işler ve sertifika
  yenilemesi bir sayfanın nerede olduğuna bağlı değildir.
- Kurulu paneller yalnız sahipleri tarafından, panelin kendi güncelleme ekranından
  güncellenir.

## Sahibin bildirdiği

Sahip 2026-10-22'de Boston'un alpha.97'deki bir ekran görüntüsünü gönderdi.
Bileşenler gerçek sürümleri gösteriyor: PowerDNS 4.8.3, Nginx 1.24.0, PostgreSQL
16.15, certbot 2.9.0, Fail2ban 1.0.2, nftables 1.0.9, cron 3.0pl1 ve Node.js 24.21.0.
Bu, D-036'nın kurulu bir sunucudaki ilk ölçümüdür. Frankfurt'un alpha.97 durumu
bildirilmedi; bu yüzden "her bileşen için kurulu paket sürümlerini oku" açık işi,
sahip onları orada da görene kadar açık kalır; korunan bir okuma ve meşgul bir paket
yöneticisi gerçek bir sunucuda görülmedi.

## Bilinen eksikler

- **Kurulu bir sunucuda hiçbir şey ölçülmedi.** Yeni düzen yalnız bileşen ve bağlanmış
  testlerle denendi; sahip onu henüz Boston'da ya da Frankfurt'ta görmedi.
- **Yöneticinin kendi e-postasını değiştiren bir ekran yok.** Panel sertifikası onu
  iletişim adresi olarak ister; yönetici sunucuda oluşturulurken verilen adrestir.
  Ret bunu söyler; kendi e-postasını Hesabım'dan değiştirmenin bir yolu açık iştir.
- Taslağın Hesabım tanımındaki **"açık oturumlar"** yapılmadı; üründe oturum listesi
  yok.
- **alpha.98'den alpha.97'ye geri dönüş.** Yeni adresler (`/security`, `/maintenance`,
  `/dns-infrastructure`, `/account`) alpha.97'ye bilinmez; bunlardan birine giden bir
  yer imi alpha.97'nin Panel sayfasına düşer. Hiçbir şey kaybolmaz ve hiçbir şeyin
  onarılması gerekmez; alpha.97'nin kendi menüsü ve eski adreslerindeki Ayarlar
  bölümleri vardır.
- **Frankfurt'un durumu** (alpha.97 güncellemesi ve alpha.95 durmasından sonraki
  güncellemesi) bildirilmedi.
- P0.1-P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez; aday kabul
  değildir.

## Kanıt

Aşağıdakilerin hepsi 2026-10-22'de aday ağaçta bir geliştirme ortamında çalıştırıldı;
hiçbiri bir kurulu sunucuda çalıştırılmadı.

- Go: `CelikPanel-S2-Debian` WSL konuğunda `go test ./cmd/panel/ ./internal/...`
  (-timeout 45m): 7156 geçti, 0 başarısız, 44 atlandı.
- Web: `npm test` 1697 geçti (alpha.97'de 1675). Yeni testler
  `web/tests/menu-layout-alpha98.test.mjs` içinde: rol başına menü, yönlendirmeler,
  bir rotaya ve var olan bir Ayarlar bölümüne çözülen her web bağlantısı ve her Go
  yönlendirme eylemi, bağlanan kişisel menü, Fail2ban durumları, İngilizce ve
  Türkçe yeni etiketler ve başlığıyla birlikte aldığı içerikle her yeni sayfa.
  `npm run build` bütçelerinin içinde: kritik açılış yolu ham 361 KiB'den 329,16 ve
  sıkıştırılmış 110 KiB'den 100,37 (alpha.97'de ham 359,20; bu adayın ilk
  taslağında, yalnız ekranların gösterdiği retler kendi parçalarına taşınmadan önce
  360,75; "errors" parçası Türkçe için 44 KiB'den 32,40).
- Sürüm kurtarma düzeneği (güncelleme kartının metinlerini ve sonuç mantığını
  yansıtır, bu sürümün değiştirdiği katalog parçalarını okur): `archlinux` WSL
  konuğunda taze bir klonda
  `python3 -m unittest discover -s deploy/e2e/release-recovery -p 'test_*.py'`:
  1109 test, OK (1 atlandı); inceleme turu retleri kendi parçalarına taşıdıktan sonra
  son ağaçta yeniden çalıştırıldı. Aynı klonda `deploy/test-release-sequence-policy.sh`
  geçti (97 -> 98).
- Dağıtım sözleşmeleri: `deploy/test-download-portal-contract.sh` (portalın yeni
  GitHub bağlantılarıyla) ve `deploy/test-signed-release-manifest-contract.sh`
  `CelikPanel-S2-Debian` konuğunda geçti.
- Tarayıcı denetimi 23 (sahte sunucu, yalnız yerel adres): yönetici, bayi ve müşteri
  için menü (ve telefondaki çekmece), her yeni sayfa, dört bölümlü Ayarlar, kişisel
  menü, her eski `/settings?section=` adresi ve müşteri olarak `/settings`,
  Bileşenler'deki güvenlik duvarı satırından `/security#firewall`'a gidiş;
  İngilizce ve Türkçe, açık ve koyu, 1440 ve 390 px (masaüstünde 30, telefonda 27
  durum). Her yönlendirme yeni bir geçmiş girdisi olmadan sayfasına vardı; ham
  anahtar ve yana kaydırma yok. İlk bakış, çalışan bir Fail2ban'ın "kurulu ama
  çalışmıyor" diye çizildiğini (kayıtlı kayıt systemd'nin "active (running)" sözünü
  taşır), güvenlik duvarı satırındaki bağlantının hâlâ "Ayarlar" dediğini ve Türkçe
  denetim satırlarının "Güvenlik duvarı" yanında "firewall" dediğini buldu; üçü de
  düzeltildi. İkinci bakış (Güvenlik, Bileşenler, yönlendirmeler ve Hesabım; iki dilde, iki
  genişlikte, iki temadan örneklerle): Fail2ban "Çalışıyor." der, durmuş olan "Kurulu
  ama çalışmıyor", kurulu olmayan sakin çizilir; bağlantı "Güvenlik'te yönet" der;
  Türkçe sayfada "firewall" yok; yeni bir şey yok. Olduğu gibi bırakılanlar (taşındı,
  yeni değil): Sistem veritabanları kartındaki "panel ayarlarını aç" bağlantısı,
  telefonda Ayarlar'ın yana kayan sekme şeridi ve taşınan kartlarda telefonda 44
  px'ten küçük düğmeler. İnceleme turundan sonra tüm denetimin bir kez daha
  çalıştırılması (masaüstünde 32, telefonda 29 durum; İngilizce ve Türkçe, iki temadan
  örnekler) temizdi: Fail2ban "Çalışıyor.", odak çıkınca kapanan menü, ham anahtar
  yok (`err.*` dahil), yana kaydırma yok.

Bileşen ve bağlanmış testler gerçek sistem davranışını kanıtlamaz.

## Nasıl güncellenir

Bu sürümü yalnız CelikPanel'in güncelleme arayüzü (Ayarlar → CelikPanel
güncellemeleri) üzerinden kurun. Yayımlamak kurulu sunucuları güncellemez; bir
sunucunun güncellemesini yalnız sahibi, panelin kendi güncelleme ekranından başlatır.

## Yayım kaydı

Aşağıdaki her şey 22 Ekim 2026 tarihlidir (saat dilimi belirtilmedikçe UTC).
Birleştirmeyi, etiketi, denetimleri, portal yayımını ve genel sitenin okunmasını
sürümü hazırlayan kişi yaptı, önceki sürümlerde olduğu gibi sahibin kalıcı
talimatı uyarınca; kurulu panel güncellemelerini yalnız sahip, panelin kendi
güncelleme ekranından başlatır. Yayım için sahibin talimatıyla gerçek sistem
kaydı yok. Bu girdi, aşağıdaki sonuçların hiçbirini depoda bir dosya olarak
saklamaz.

**Ne oldu.**

1. Sürümden önce bir bağımsız inceleme yapıldı: 12 bulgu ve bir bulgu daha,
   hepsi düzeltildi (düzeltmeler yukarıda anlatıldı). Yeniden yapılan bir
   denetim sürümü yayıma hazır buldu.
2. "Release v0.1.0-alpha.98: every page where its name says it is" başlıklı #234
   numaralı çekme isteği `main`'e `c249a9ba236212bfcdf6eddb590f55bccc5bac9c`
   commit'i olarak birleştirildi; yazarı Mehmet Ömer Efe Çelik. Sürüm sırası
   değişkeni etiket yapılmadan önce 97'den 98'e geçirildi. `main` koşusu ve
   etiket koşusu başarılı oldu; sürüm zinciri hiçbir olay olmadan çalıştı.
3. `v0.1.0-alpha.98` açıklamalı etiketi (nesne
   `8696ddbc3880bf2fd2cf794f0c1cc03e27065c86`, ağaç
   `838d69121bf3213d7a71ec6fbf7437f7575413aa`, etiketleyen Mehmet Ömer Efe Çelik)
   sürüm commit'ini gösterir. Sürüm bir ön sürümdür (pre-release);
   2026-10-22T01:01:25Z'de altı dosyayla yayımlandı.
4. 22 Ekim 2026'da `CelikPanel-S2-Debian` WSL konuğunda bir indirmeden
   denetlendi (hazırlayan kişi; burada bir dosyada kayıtlı değil): arşivler bayt
   bayt aynı, her biri 66.317.470 bayt, SHA-256
   `23820e87c8266e8321937ae9a2ea2a4b9864b1fd6fa30dace900836365948279`; manifest
   sıra 98, `published_at` 2026-10-22T00:15:48Z der (SHA-256
   `2e962d44560e36a6d26880a9d6b94090df20fbe7885137abbf7b78cb9c18b2db`); imzası
   (SHA-256 `3a27af0548c269b65e9c4d3e53b3328170bd7a843be034b1aa53a61c5c956498`),
   imza anahtarına (SHA-256
   `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`, öncekiyle
   aynı anahtar) karşı `openssl pkeyutl -verify -rawin -pubin` ile doğrulanır;
   iki içerik koruması da 0 döndürdü; derleme Go go1.26.5, CGO kapalı,
   `-trimpath` ile yapıldı. `libexec/get.sh` 98'i sabitler (SHA-256
   `f2acf8794ff2bc202d193780f03a9229244af462ab5e22f2c682ec928719faeb`); içindeki
   politika şimdiki 98, önceki 97, önceki commit
   `0c1a8917ef3f8141702fa41ae085f64d62a713f0` der. Arşivde 267 üye var (243
   dosya, 24 dizin). Arayüz paketi `web/dist/assets/app-qbukM16Q.js`, 258.759
   bayt, SHA-256
   `736a15f2bc0e6ddca1a4189079feafc8cc6afed88eb28c6ebdd325214a6f90aa`.
5. Portal paketi: `portal.tar.gz`, 133.007.110 bayt, SHA-256
   `1cb3374ee47f73f3b45a2b5a9ce12888bda0dd59d9a2ef2bb3a0ef64de416174`, 54 girdi;
   etiketin `deploy/build-download-portal.sh` betiğiyle (önceden imzalı kip) iki
   kez bayt bayt aynı derlendi. Sayfaları yalnız `github.com/alicelik77/...`
   adresine bağlanır (ana sayfa 7, teknik sayfa 16, `security.txt` 2; hiçbiri
   `celikbros`'a değil). Sözleşme testi 0 döndürdü; genel doğrulayıcının 17 testi
   ve terfi aracının 14 testi geçti.
6. Yaklaşık 01:11Z'de hazırlayan kişi onu kendi bilgisayarından
   `deploy/publish-download-portal.ps1` ile yayımladı: önceki sürüm
   v0.1.0-alpha.97, hedef v0.1.0-alpha.98; durum `committed`, genel doğrulama 26
   istekten 26'sı, `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`.
7. Önceki sitenin yedeği
   `/var/www/vhosts/celikpanel.net/portal-backups/httpdocs.before-v0.1.0-alpha.98.20261022T011052Z.112529.a585ec0704b24049`;
   yüklenen paket
   `/var/www/vhosts/celikpanel.net/.upload-portal-20261022T011011Z-v0.1.0-alpha.98-bc69f36337b59bba`
   altında saklandı.
8. Yayımdan sonra dışarıdan okuma: `https://celikpanel.net/releases/latest.txt`
   v0.1.0-alpha.98 der; `latest.json` sıra 98'i, yukarıdaki commit ve SHA-256'yı
   gösterir; canlı ana sayfa `github.com/alicelik77/` adresine bağlanır (7 kez).

**Sahibin bildirdikleri.** Boston'un alpha.97'de gerçek paket sürümlerini
göstermesi yukarıda ("Sahibin bildirdiği") kayıtlı; burada yinelenmez. Sahip 21 ve
22 Ekim 2026'da `momerefe` hesabına depoya yazma erişimi verdi ve GitHub CLI'yı
`momerefe` olarak oturum açtırdı. alpha.98'den başlayarak `main` üzerindeki sürüm
commit'i, açıklamalı etiket ve itmeler Mehmet Ömer Efe Çelik
<momerefe@gmail.com> adına yapılır; alpha.98, tümüyle bu kimlikle yapılan ilk
sürümdür. alpha.98'den henüz hiçbir şey bir kurulu sunucuda değil. Frankfurt'un
alpha.95 durmasından sonraki güncellemesi bildirilmedi.

**Yapılmayanlar ve yayımlamanın değiştirmedikleri.**

- Kurulu sunucuları sürümü hazırlayan kişi alpha.98'e güncellemedi. Onları yalnız
  sahip, panelin kendi güncelleme ekranından günceller.
- Yeni düzen bir kurulu sunucuda görülmedi.
- Hiçbir ekran bir yöneticinin kendi e-postasını değiştirmiyor; panel sertifikası
  onu iletişim adresi olarak ister (D-037'de kayıtlı).
- "Açık oturumlar" yapılmadı.
- Bildirilmeyen ve kaydedilmeyenler: Frankfurt'un alpha.95 durmasından sonraki
  güncellemesi (ve alpha.97 durumu), Boston ya da Frankfurt'tan alpha.98'e dair
  her şey ve önceki notların bildirilmedi diye saydığı her şey. Her biri
  "kaydedilmedi"dir; "çalıştı" da değildir, "başarısız" da.
- Önceki yayımlama kayıtlarının sınırları ([alpha.88
  notları](RELEASE-NOTES-v0.1.0-alpha.88.tr.md)) kalkmış olarak bildirilmedi.
- Yayımlanma kabul değildir. P0.1–P0.5 kısmi kalır; hiçbir P0 işi kapanmaz ya da
  ilerlemez.
