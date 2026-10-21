# v0.1.0-alpha.98 (aday, 22 Ekim 2026)

[English](RELEASE-NOTES-v0.1.0-alpha.98.md)

Bu sürüm v0.1.0-alpha.97'nin ardından gelir. Bir adaydır: bu yazılırken
etiketlenmemişti ya da yayımlanmamıştı ve hiçbir parçası bir kurulu sunucuda
değildi. 2026-10-22'de, `feat/alpha98-menu-layout` dalında, alpha.97'nin yayım
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
  1109 test, OK (1 atlandı). Aynı klonda `deploy/test-release-sequence-policy.sh`
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
  px'ten küçük düğmeler.

Bileşen ve bağlanmış testler gerçek sistem davranışını kanıtlamaz.

## Nasıl güncellenir

Bu sürümü yalnız CelikPanel'in güncelleme arayüzü (Ayarlar → CelikPanel
güncellemeleri) üzerinden kurun. Yayımlamak kurulu sunucuları güncellemez; bir
sunucunun güncellemesini yalnız sahibi, panelin kendi güncelleme ekranından başlatır.
