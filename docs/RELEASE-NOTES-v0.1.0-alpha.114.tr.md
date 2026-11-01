# v0.1.0-alpha.114

[English](RELEASE-NOTES-v0.1.0-alpha.114.md)

Bu sürüm v0.1.0-alpha.113'ün ardından gelir. İlk gerçek sunucuların gösterdiği
sorunları düzeltir; kullanıcıları ve bayileri, Plesk kullananların beklediği
birkaç küçük özelliği, sahibin elindeki işletim sistemi güncellemelerini ve
yeni kurulumlar için yeni bir varsayılan panel portunu ekler. **Bu bir aday
sürümdür, 2026-11-01'de yazıldı; yayımlanmadı ve kurulu hiçbir sunucuda
değildir. Kurulu panelleri yalnız sahibi, panelin kendi güncelleme ekranından
günceller.** Dal `release/alpha114-prep`, `1bd1793fe` üzerindeki entegrasyon
adayı `release/alpha114-integration`'dan; önceki sürüm kaydı
`d3f3a323da46ce8f8dca5ad4385666c5428c353e`.

## Kimin için ve neden

Sunucu kuran herkes için ve sahibin planladığı yeniden kurulum için: Frankfurt
(Debian 13, ilk sunucu, ana DNS olarak PowerDNS, posta) ve Boston (Ubuntu
24.04, kodla katılır) silinip bu sürümle yeniden kurulur. alpha.113'teki kurulu
paneller kendi ekranlarından güncellenebilir (şema 46'dan 51'e; güncelleme eski
panel portunu korur, aşağıya bakın). Daha yeni DNS durumunu taşıyan alpha.111
veya alpha.112 sunucularını güncelleme reddeder; onları yeniden kurun.

## Neler değişti

**Kurulum ve ilk ekran**
- İlk soru "Bu sunucu ne olacak?": ilk sunucu ya da var olan bir sunucuya
  katılan sunucu. Katılma yolu yalnız adresi ve kodu sorar.
- Ana alan adı varsayılan olarak bir web sitesi olur. İlk sunucuda sihirbaz
  "Ana alan adının web sitesi bu sunucuda mı olacak?" diye sorar; varsayılan
  Evet'tir. Evet: kurulum, Alan adı ekle ile aynı yoldan siteyle (çıplak ad ve
  `www`) ve Let's Encrypt sertifikasıyla biter. Sertifika alınamazsa site
  kalır ve adım nedenini söyler. Hayır: ad yalnız DNS olarak kalır, sihirbaz
  bunu düz bir dille söyler ve Alan adları listesinde "Web sitesi olarak aç"
  düğmesi bulunur. Katılan sunucuya bu sorulmaz: ana alan adı ilk sunucudadır.
- Son özet, başlamayı durduranı önce, düz dille ve düzelten düğmeyle gösterir.
  Hazır sayfası, sahibe kalan işleri (web sitesi, kayıt firması, öneriler)
  eylemleriyle sıralar.
- Kodla katılan sunucu kendi müşterileri için posta çalıştırabilir. Ana alan
  adının posta kayıtlarını hiçbir zaman almaz; yalnız kendi posta adını
  yayımlar. İlk sunucunun, ana alan adının ya da bir ad sunucusunun zaten
  kullandığı posta adı, nedeni ve düzeltme yoluyla reddedilir.
- Bileşenler'den bir posta profili kurmak sahibi artık kurulum sihirbazına
  göndermez. Bir pencere yalnız posta adını sorar, adımları sıralar, çalıştırır
  ve sonucu yerinde gösterir. Katılan sunucuda ana alan adının kayıtlarına
  dokunulmaz.
- Çalışan DNS yazılımı tek sabit satır olarak gösterilir; kurulum onu
  değiştirebiliyormuş gibi davranmaz.

**Kullanıcılar ve bayiler**
- Plan ve kotalarıyla müşteri ve bayi hesapları. Alan adları, veritabanları,
  posta kutuları ve disk için 0 sınırsız demektir.
- Yönetici ya da bayi, hesap açıldıktan sonra plan atayabilir veya
  değiştirebilir. Mevcut kullanımın altına inen değişiklik, aşılan şeylerin
  listesiyle reddedilir; kişi onaylayana kadar. Hiçbir şey silinmez.
- Bayilerin kendi planları ve yöneticinin belirlediği bir havuz vardır
  (müşteriler, alan adları, veritabanları, posta kutuları, disk). Havuzu aşmak
  nedeniyle reddedilir.
- "Planım", hesabın kendi sınırlarını ve kullanımını Panel'de ve Hesabım'da
  gösterir. Kotaya takılan reddedilmiş bir eylem buraya yönlendirir.
- "Olarak giriş yap" hesap verebilirdir: Denetim günlüğü kimin kim adına
  işlem yaptığını kaydeder, bir şerit işlemi yapanı adlandırır ve oturum
  işlemi yapanın yetkisiyle biter.
- E-postayla parolamı unuttum: her ad için tek yanıt, hız sınırları, tek
  kullanımlık ve 30 dakika geçerli bağlantı, yalnız onaylı adrese gönderilir.
  Yeni parolayı kaydetmek hesabın her yerdeki oturumunu kapatır.
- Davetler: hesap parolasız açılabilir; kişi parolasını 7 gün geçerli bir
  bağlantıyla seçer. Sunucu posta gönderemiyorsa bağlantıyı yalnız davet eden
  nedeniyle birlikte görür. Bekleyen davetler yeniden gönderilebilir,
  değiştirilebilir veya iptal edilebilir. Kullanıcılar listesi "Davet edildi,
  henüz giriş yapmadı" der.
- E-posta adresleri Hesabım'da onaylanır; adresin her değişikliği onayı siler.
- Bant genişliği ölçülmediği için plan sınırı değildir ve plan formlarında
  gösterilmez.

**Plesk ile küçük farklar**
- Site başına parola korumalı klasörler (sitenin kendi dosyasında nginx temel
  kimlik doğrulaması; parola dosyası web kökünün dışındadır).
- Site başına yönlendirmeler: tam ya da klasör eşleşmesi, 301, 302, 307 veya
  308, sorguyu koruma seçeneğiyle. Döngü, yineleme ve çakışma kontrol edilir.
- Posta kutusu başına "ofis dışında" otomatik yanıtı (Dovecot Sieve).
  Yanıtı Dovecot gönderir; bu yüzden panel durdurulsa ya da kaldırılsa da
  çalışır. Sahibin düzenlediği bir Sieve betiği, drop-in dosyası ya da
  transport haritası korunur ve değişiklik nedeniyle reddedilir.
- İki adımlı girişin kurtarma kodları: bir kez gösterilen on tek kullanımlık
  kod; yeni kodlar parola ve kodla alınır; her kod bir kez harcanır.
- DNS genel görünümü: bu sunucunun listesinde olmayan posta kaydı bekleme
  sayılmaz. İsteğe bağlı IPv6 ad sunucusu kayıtları hiçbir zaman bekleme
  değildir; kart bunların isteğe bağlı olduğunu ve kayıt firmasında neyin
  eklenebileceğini söyler. Katılan sunucu diğerine "İlk sunucu" der, ilk
  sunucu ona "İkinci sunucu" der. Yedeğini bekleyen ana DNS sunucusu "DNS
  motoru yok" diye gösterilmez.

**İşletim sistemi güncellemeleri**
- Debian ve Ubuntu: sahibi otomatik ya da elle güncellemeyi seçer. Panel
  apt'nin kendi dosyasını, `/etc/apt/apt.conf.d/20auto-upgrades`'i kullanır.
  Sahibi o dosyayı düzenlediyse panel bunu söyler ve dosyayı yalnız sahibi
  okuyup onayladıktan sonra değiştirir.
- Arch'ta anahtar yoktur. CelikPanel kendiliğinden `pacman -Syu` çalıştırmaz.
  Sayfa adım adım yönlendirme verir ve PHP ile PostgreSQL ana sürümleri için
  uyarır.
- Panel, bekleyen güncelleme sayısını gösterir (önbellekten, salt okunur; hiçbir
  değişiklik başlatmaz).
- İşletim sistemi kartı yalnız çalışan sistemi açıklar.
- Yeni bir herkese açık sayfa desteklenen sistemleri (Debian 12 ve 13, Ubuntu
  22.04 ve 24.04, Arch), her birinde nelerin sunulduğunu, nelerin sunulmadığını
  ve nedenini listeler. İngilizce ve Türkçedir; indirme portalından bağlanır.

**Panel portu 1923 (D-041)**
- Yeni kurulum 1923'te dinler, çünkü 2083 cPanel'in portudur. Bu sürümden önce
  kurulan sunucu portunu korur (sahibin sunucularında 2083): güncelleme onu
  hiçbir zaman yeniden yazmaz.
- Ayarlar, geçerli panel adresini ve "Değiştir" düğmesini gösterir. Panel yeni
  portu bağlar ve güvenlik duvarında açar, sahibi yeni adresten onaylar ve port
  ancak ondan sonra yazılır. Zaman aşımı (10 dakika), iptal ya da yeniden
  başlatma eski portu korur. Sahibin düzenlediği dosya bildirilir, üzerine
  yazılmaz.
- Bir çiftin parçası olan ya da kendisine sunucu katılmış olan sunucu
  değişikliği reddeder (`PANEL_PORT_PAIRED`), çünkü diğer sunucu onun adresini
  saklar. Bu alpha.115'i bekler (Bilinen sınırlara bakın).

**Laboratuvar koşularında bulunan düzeltmeler**
- Katılan sunucunun kurulumu, ilk sunucu yalnız meşgulken "kod süresi doldu ya
  da kullanıldı" diye duruyordu. İlk sunucu artık gerçek reddi meşgul andan
  ayırır; katılan sunucu bekler, beklediği sunucuyu adlandırır ve devam eder.
- Otomatik DKIM aynı bölgeyi yazarken eklenen bir DNS kaydı, kayıt kaydedilmiş
  olduğu halde "iç hata" yanıtı veriyordu. Artık kaydedildi yanıtı verir, DNS
  sunucularının değişikliği bir dakika içinde alacağını söyler ve kendiliğinden
  yayımlar.
- Debian 13: ofis dışı yanıtını açmak, o posta kutusuna giden her iletiyi geri
  çeviriyordu. Dovecot'un posta teslimi artık posta kutusunu tam adresiyle
  arar, panel posta yönlendirmeden önce Dovecot'a sorar ve Dovecot'un
  reddettiği posta kutusu Postfix'in kendi teslimini korur. Hiçbir şey geri
  dönmez.
- Adresin sorgusu olduğunda klasör yönlendirmesi yolun kalanını kaybediyordu
  (`/blog/post?q=1` çıplak hedefe gidiyordu).
- Katılan sunucudaki müşteri alan adı MX, SPF, DMARC ve DKIM kayıtlarını
  almıyordu. Artık ana DNS sunucusuna eklenir (yalnız ekleme; sahibin kayıtları
  korunur) ve bölgeyle birlikte kaldırılır.
- alpha.113 ile kurulan bir PowerDNS ilk sunucusunun güncellemesi "uygulama
  uyumluluğu" diye reddediliyordu. Kontrol artık alpha.113'ün daha yeni DNS
  durumunu yazdığını bilir. Güncelleme geçer ve alpha.113'e geri dönüş o
  durumu korur.
- Sonradan eklenen posta (Bileşenler'den) son kontrolünde sonsuza dek
  bekleyebiliyordu. Artık canlı bağlantıyı kanıtlar, açık her kontrolü
  adlandırır, bir kontrolün gecikmesini söyler ve "Yeniden kontrol et" sunar.
- Arch: ofis dışı yanıtı yeniden yüklemede başarısız oluyordu ("Unknown
  section name: sieve_script"), oysa `systemctl reload` başarılı diyordu. Posta
  kurulumu artık Sieve paketini içerir, her Dovecot yeniden yüklemesi Dovecot'un
  kendi günlüğünde kontrol edilir ve eski bir kurulum, tek çare buysa bir kez ve
  uyarıyla yeniden başlatılır.
- Dovecot durmuşken ofis dışı yanıtını kaydetmek yanlış neden veriyordu. Artık
  Dovecot'un durduğunu söyler ve hiçbir şey yönlendirilmez.
- Katılan sunucuda alan adı silmek, ana DNS sunucusunun yedeğiyle onaylaması
  zaman alınca "DNS temizliği, bilinmiyor"da kalıyordu. Katılan sunucu artık
  kendiliğinden yeniden sorar (15 sn, 5 dakikaya kadar iki katına çıkarak) ve
  bitirir. Durum hiçbir zaman "bilinmiyor" demez.
- Bir güncelleme geri alınıp doğrulandıktan sonra güncelleme kaydı "başarısız,
  kurtarma gerekli" diyor ve neden boş kalıyordu. Artık önceki sürümün
  çalıştığını söyler, yükleyicinin nedenini verir ve yükleyicinin son satırını
  ayrıntı olarak saklar (yollar, adresler ve gizli bilgiler çıkarılır).
- Debian 13 ve Ubuntu 24.04: yeni Dovecot yeniden yükleme kontrolüyle posta
  kurulumu hiç bitmiyor, ardından Agent her değişikliği bekletiyordu. Kontrol
  artık yalnız Dovecot'un kendi günlüğü bir reddi kaydettiğinde başarısız olur
  ve hem 2.3 hem 2.4 günlük sırasını okur.
- Bir geri almadan birkaç saniye sonra başlatılan güncelleme paneli durmuş
  bırakabiliyordu. Reddedilen güncelleme artık paneli ve Agent'ı çalışır
  bırakır; Agent kısa bir işlemi bitirirken güncelleme bir dakikaya kadar
  bekler.
- Debian 13: DKIM kurulurken ilk sunucunun kurulumu web posta adımında
  "güvenlik duvarı eşitlemesi başarısız" ile durabiliyordu. Güvenlik duvarı
  adımı artık sunucu meşgulken bekler (10 dakikaya kadar) ve bunu söyler.
- Dovecot posta kutusunu kabul etmediği için ofis dışı yanıtı reddedildiğinde
  sonraki ileti yine de geri dönebiliyordu. Postfix artık ofis dışı
  yönlendirmelerindeki her değişiklikten sonra yeniden yüklenir.
- Katılan sunucu, silinen bir alan adının ana DNS sunucusunda kaldırılmasını
  beklerken ekran, ana sunucu yalnız yavaşken ulaşılamıyor diyebiliyor ya da
  hiç neden vermiyordu. Artık ana sunucu isteği aldıysa "onaylanıyor", yalnız
  ulaşılamadığında "ulaşılamıyor" der; biten silme listeden kendiliğinden çıkar.
- Bir Türkçe metin dosyasındaki paket boyutu sınırı web derlemesini
  başarısız kılacaktı; dosya bölündü. Kullanıma etkisi yok.
- Ekranlar (ilk incelemenin 24 bulgusu ve ikincinin 4'ü): sihirbaz, Panel,
  Alan adları, Planlar, Ayarlar, Kullanıcılar, Bileşenler ve Denetim günlüğünde
  metin ve düğmeler. Örnekler: Panel "`<alan adı>` için henüz web sitesi yok"
  der ve Alan adı ekle'yi açan bir düğme sunar (katılan sunucuda değil);
  yöneticinin kendi çalışma alanında plan sınırı ve gizli bir sayı yoktur;
  Alan adı ekle, bu sunucu adın DNS'ini sunuyorsa sertifika kutusu işaretli
  başlar; Denetim günlüğü eylem kodları yerine sözcükler gösterir; tek
  sunuculuk kurulum artık "katılacak sunucu"dan söz etmez.
- Her değişiklik onu gösteren her ekranı yeniden okutur; bu yüzden bir eylemden
  sonra liste kendiliğinden güncellenir ve kimsenin sayfayı yenilemesi gerekmez.
  Buna yalnız DNS olan bir ad üzerinde site oluşturulduktan sonra Alan adları
  listesi de dahildir (eskimiş "yalnız DNS" satırı kalmaz).
- Reddedilen değişiklik, olduğu sayfada nedeni ve düzeltmesiyle kalır; hata
  iletisi kapatılana kadar durur. Sunucunun ürettiği veritabanı parolası
  kapatılana kadar sayfada kalır; Kopyala düğmesi ve bir daha gösterilmeyeceği
  notu vardır. Kapalı bir ana düğme nedenini söyler.
- "Hizmetler" her yerde "Bileşenler" olarak adlandırılır.

## Bilinen sınırlar

- Panel portu bir çiftin parçası olan sunucuda değiştirilemez; sahibin
  Frankfurt ve Boston sunucuları, alpha.115 yeni uç noktayı diğer sunucuya
  duyurana kadar 2083'te kalır.
- Joker karakterli sertifika bu sürümün parçası değildir (alpha.115).
- SMTP aktarıcısı yoktur. Panel postası (parolamı unuttum, davetler, onaylar)
  bu sunucunun kendi posta sisteminden `127.0.0.1:25` ile gider; yoksa
  ekranlar bunu ve kimin yardım edebileceğini söyler.
- Katılan sunucuda ana alan adının DKIM anahtarı ilk sunucudadır. Katılan
  sunucudan gönderilen davet postasının From alan adıyla DKIM hizalı kalıp
  kalmadığı kanıtlanmadı.
- `install.sh` yeni kurulumlar için hâlâ eski `20celikpanel-auto-upgrades`
  dosyasını yazar. Yeni seçim `20auto-upgrades` kullanır.
- Bir davet iptal edildikten sonra aynı kişi baştan yeniden davet edilmelidir.
- Güncelleme kartı, aynı sürümün aynı yerde duracağı durumlarda bile "önceki
  deneme" metnini gösterir ve yeniden denemenin güvenli olduğunu söyler.
- Daha yeni DNS durumunu taşıyan alpha.111 veya alpha.112 sunucuları güncelleme
  için reddedilir. Onları yeniden kurun.
- Tek bir alan adı için postayı kapatmak o alan adının posta kayıtlarını
  kaldırmaz.
- Sınırsız bir sınır planda "∞" olarak gösterilir.
- Katılan sunucunun Paneli bütün bölge kartlarını gizler; bu yüzden katılan
  sunucudaki yalnız DNS müşteri bölgesi orada kart almaz (Alan adları satırı
  eylemi yine sunar).
- Ana sunucuda, hiç gözden geçirilmemiş bir kodla bekleyen katılan sunucu
  silmesi hâlâ nedensiz `dns_cleanup` olarak görünür.
- Kurulu hiçbir sunucuda ölçüm yapılmadı. Hiçbir P0 işi kapanmaz veya
  ilerlemez; aday kabul değildir.

## Kanıt

`1bd1793fe` üzerindeki testler: Go 69 paket tamam, 7901 test geçti; Agent 5084
geçti, 0 başarısız; dağıtım sözleşmeleri geçti; web 2216 geçti, tür kontrolü ve
paket bütçeli derleme geçti. Sıra 114, şema 51 (047'den 051'e geçişler: panel
portu değişikliği, hesaplar-planlar-bayiler, hesap kurtarma, korumalı
klasörler-yönlendirmeler-kurtarma kodları, posta otomatik yanıtları; hepsi
ekleyicidir).

Laboratuvar kabul koşuları (özetler `lab/alpha114-acceptance` dalında,
`deploy/e2e/dns-pair-acceptance/evidence/` altındadır):

- Koşu 1, `final-alpha114-run1-20261031/SUMMARY.md`, aday `8303c0a87`.
  Yayımlanamaz: 8 kusur (E1'den E8'e) ve 24 ekran bulgusu. E7, PowerDNS ilk
  sunucularının güncellemesini engelliyordu. Hepsi düzeltildi.
- Koşu 2, `final-alpha114-run2-20261101/SUMMARY.md`, aday `7d1d4ac7a`.
  Yayımlanamaz: koşu 1'in hepsi geçti, ancak bir derleme engeli ve iki yeni
  Yüksek kusur (E-new-1, E-new-2, E-new-4) buldu ve hızlı durma kuralıyla erken
  durdu. E-new-1'den E-new-5'e ve dört yeni ekran bulgusu sonradan düzeltildi.
  Koşu 2'nin geçenlerinden: bir PowerDNS ilk sunucusunda ve çiftin iki
  sunucusunda 113'ten 114'e güncelleme (şema 46'dan 51'e, port 2083 korundu,
  geri alma doğrulandı, ikinci güncelleme başarılı).
- Koşu 3'ten 6'ya kısa, hedefli kontrollerdi (`final-alpha114-run3..run6-20261101`);
  her biri ilk ciddi hatada durdu, ardından düzeltildi:
  - Koşu 3 (`aa84d2307`): Debian 13 ve Ubuntu 24.04'te posta kurulumu hiç
    bitmiyordu (Dovecot yeniden yükleme kontrolü); Arch'ta geri almadan hemen
    sonra reddedilen güncelleme paneli durmuş bırakıyordu.
  - Koşu 4 (`449701242`): koşu 3'ün iki kusuru geçti. Arch'ta ofis dışı yanıtı
    güncellenmiş ve yeni kurulmuş sunucuda geçti. Debian 13 kurulumu meşgul
    güvenlik duvarı adımında durdu; geri alma kaydının ayrıntısı boştu.
  - Koşu 5 (`d70c1d7ce`): iki sunucu da kurulumu bitirdi; Dovecot durmuşken
    kontrolü geçti. Reddedilen bir ofis dışı kaydından sonra bir ileti geri döndü.
  - Koşu 6 (`69df35d0a`): ofis dışı yanıtı Debian 13 ve Ubuntu 24.04'te, bir
    retten hemen sonra da geçti. Katılan sunucuda alan adı silme her durumda
    kendiliğinden bitti ve DNS'i kaldırdı: normalde, ilk sunucunun paneli
    durmuşken ve katılan sunucu yeniden başlarken. Beklerken gösterilen neden
    yanlış ya da eksikti; bu metin sonradan düzeltildi ve kaynakta sınandı,
    laboratuvarda değil.

Son düzeltmelerden sonra laboratuvarda, sahibin kısa son deneme seçimiyle
denenmeyenler: son turun ekranları (katılan sunucu "Web sitesi barındır"
sunmaz, DNS yazılımı adları, Denetim günlüğü sözcükleri), unutulan yönetici
komutunun terminalde çalıştırılması ve gerçek bir başarısız güncellemenin geri
alma ayrıntısı. Bunlar kaynakta sınanmıştır. Sıra 114, şema 51. Hiçbir P0 işi
kapanmaz veya ilerlemez; aday kabul değildir.
