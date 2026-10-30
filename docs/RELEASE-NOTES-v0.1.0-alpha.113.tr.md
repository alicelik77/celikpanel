# v0.1.0-alpha.113

[English](RELEASE-NOTES-v0.1.0-alpha.113.md)

Bu sürüm v0.1.0-alpha.112'nin ardından gelir. CelikPanel başkalarına
açılmadan önce sahibin sunucularının yeniden kurulacağı sürümdür ve Arch
Linux'u sunucu seçeneği olarak ekler. **Bu bir aday sürümdür, 2026-10-30'da
yazıldı; yayımlanmadı ve kurulu hiçbir sunucuda değildir. Kurulu panelleri
yalnız sahibi, panelin kendi güncelleme ekranından günceller.** Dal
`release/alpha113`, `main` üzerindeki `b5fa3fae0`'dan; önceki sürüm kaydı
`95c23ef414cc7420e5df6a119b7325cacfdef5d8`.

## Kimin için ve neden

Sunucu kuran herkes için, özellikle sahibin planladığı yeniden kurulum için:
Frankfurt (ilk sunucu, ana DNS, posta), Boston (kodla katılır, yedek DNS) ve
bir Arch Linux sunucusu. İlk iki gerçek kurulumda ve 2026-10-30 deneme
kurulumlarında bulunanları düzeltir; bu akışlarda bilinen kusur kalmaz. Kurulu
paneller kendi ekranlarından güncellenebilir; veri dönüşümü gerekmez (şema 46,
değişmedi).

## Neler değişti

**Kurulum ve ilk kurulum**
- Kısa çekirdek adının okunduğu her yerde sunucunun tam adı (`hostname -f`)
  kullanılır: sunucu bilgisi, lisans hesabındaki sunucu adı, ad sunucusu
  önerileri, bölge şablonu ve panelin ilk sertifikası.
- Kurulum sihirbazı: silinip yeniden kurulan sunucudan kalan başlangıç işareti
  atılır; katılma alanlarında Enter katılır; sahibin kendi seçimlerinde
  "Önerilen" rozeti yok; alan adı zaten bu ad sunucularını kullanıyorsa kayıt
  firması kartı yapılacak bir şey olmadığını söyler.
- Sunucu o an meşgul olduğu için reddedilen adım bekler ve kendiliğinden
  sürer; salt-okur kilit denetimi gerçek bir değişikliği meşgul göstermez,
  meşgul yanıtı kilidi tutan işlemi adlandırır.
- Yarıda kalan DNS kurulumunun geçiş kaydı tutulduysa kurulum sayfası artık
  "geri alındı" demez; kimin ne yapacağını ve kurulumun nasıl süreceğini
  söyler. Yedeği hâlâ bitiren ana sunucu "çalışıyor"da gerekçesiz kalmaz,
  bekler ve yeniden bakar.

**Siteler ve nginx**
- Yeni sitenin karşılama sayfası daha açık; panelin kendi bölüm adlarını
  verir, Uygulamalar'ı yalnız PHP sitelerinde anar ve sunucunun yazılımı
  hakkında bir şey söylemez.
- Alan adı eklerken "SSL'i etkinleştir" artık sertifikayı ister ve sonucu
  gösterir (verildi, gerekçesiyle başarısız, bekliyor). Sertifika etkin
  olunca http adresi https'e yönlenir (SSL/TLS sayfasından kapatılabilir).
  Geçerlilik başlangıcı sunucu saatinin birkaç saniye ilerisinde olan
  sertifika beklenir; daha büyük saat farkı çıplak hata yerine açıklanır
  (sunucu saatini denetleyin).
- Statik olarak eklenen site PHP'ye çevrilebilir: geçiş artık sitenin kendi
  PHP-FPM havuzunu oluşturur (önceden reddediliyordu).
- Ziyaretçilere yazılım sürümleri söylenmez: nginx sürümünü adlandırmaz
  (`server_tokens off;`; Ubuntu ve Arch adlandırıyordu), PHP sayfaları
  `X-Powered-By` göndermez (Arch gönderiyordu).
- Web posta adresi http'den https'e yönlenir.
- nginx: HTTP/2 biçimi kurulu nginx'e uyar (Ubuntu 24.04'ün 1.24'ü
  `http2 on;` satırını reddediyordu), Mozilla ara TLS ayarları, sitesi
  olmayan adlar için varsayılan HTTPS sunucusu (nginx 1.18'de de) ve
  çözülemeyen bir vekil hedefi nginx'i asla durdurmaz. Nginx sayfası
  varsayılan sunucunun durumunu gösterir.

**Sertifikalar ve posta (D-040)**
- Site sertifikalarını işletim sisteminin certbot zamanlayıcısı yeniler;
  panel olsa da olmasa da. Postanın SNI sertifikaları yenilemeyi izler.
  Önceki sürümlerin verdiği sertifikalar bir sonraki yenilemede bu düzene
  geçer.
- Posta TLS'i sunucunun tam adını kullanır; varsayılan posta sertifikalarını
  panelin kendisi üretir (`openssl` gerekmez); Arch'ta Postfix, gönderim
  açılmadan önce sunucu sertifikası alır.
- Debian 13 (Dovecot 2.4): gelen posta IMAP'ten ve web postadan okunur.
  Debian'ın kendi `10-mail.conf` dosyası gelen kutusunu `/var/mail`'e
  yönlendiriyordu; CelikPanel'in ek dosyası artık posta kutusunun maildir
  yolunu açıkça verir, mevcut sunucuda panelin bir sonraki açılışında
  onarılır, sahibin elle değiştirdiği dosya üzerine yazılmaz, bildirilir.
  Dağıtım matrisi artık her sürümde bir ileti teslim edip geri okur.
- Agent, Debian'da her açılışta `postmap: fatal: unsupported map type: lmdb`
  satırı yazmaz.
- Kullanıcıların gönderdiği posta (587 ve 465 kapıları) her dağıtımda DKIM
  ile imzalanır ve spam denetiminden geçer. Arch'ta bu hizmetler,
  `localhost` adını çözecek dosyalar olmadan kapalı alanda (chroot)
  çalıştığı için süzgeçler atlanıyordu. Postfix süzgeçlere artık
  `127.0.0.1` üzerinden ulaşır, gönderim hizmeti dağıtımın kendi chroot
  seçimine uyar; mevcut sunucu panelin bir sonraki açılışında onarılır
  (sahibin değiştirdiği satırlar korunur ve adlandırılır).
- DKIM: "DKIM anahtarı üret" yeniden basılınca OpenDKIM'in mevcut anahtarı
  okuma izni artık kalkmaz; imzalama adımı başarısız olursa "anahtar
  oluşturuldu" yerine bu bildirilir.
- Sunucuda postası olan her alan adı DKIM anahtarını ve imzalamayı kendiliğinden
  alır. Bu üç durumda olur: posta kuruluyken alan adı eklendiğinde, alan adına
  ilk posta kutusu açıldığında ve posta profili Hizmetler'den kurulduğunda (bu
  kurulum artık OpenDKIM'i de kurar). Önceden sahip "DKIM anahtarı üret"a
  basana kadar posta imzasız gidiyordu.
  - Bölge bu sunucuda tutuluyorsa DKIM kaydı orada ve yedek DNS sunucusunda
    yayımlanır.
  - DNS başka yerdeyse kart, sağlayıcıya eklenecek kaydı gösterir.
  - Var olan anahtar ya da bölgede zaten bulunan farklı bir DKIM kaydı
    korunur ve kart bunu söyler.
  - Alan adı eklemek bu yüzden hiç beklemez. Kart ne olduğunu, kimin işlem
    yapacağını ve nasıl süreceğini söyler; "Yeniden dene" düğmesi vardır.
  - Alan adı silinince CelikPanel'in oluşturduğu anahtar ve kaydı da kalkar;
    sahibin oluşturduğu anahtar korunur ve adı bildirilir.
- Dosya yöneticisi; olmayan bir yol, zaten var olan bir ad ya da indirilemeyen
  bir dosya için çıplak hata yerine ne yapılacağını söyler.

**Lisans (D-039)**
- Bir başarılı denetim, yönetimi çevrim dışı yedi güne kadar açık tutar;
  kullanımdaki paneller yaklaşık saatte bir denetler; ileri alınan saat süreyi
  uzatamaz.

**Arch Linux**
- Sunucu rolleri Arch'ta yalnız resmi paketlerle çalışır: web, PHP, MariaDB,
  posta (Postfix, Dovecot, DKIM), DNS, Fail2ban, phpMyAdmin.
- Arch'ın `extra` deposundaki PowerDNS 5.1; tek başına, ana ya da yedek DNS
  sunucusu olarak (DNS yazılımı olmayan sunucuda ilk kurulum; D-028 eki).
- Güvenlik güncellemeleri sayılır ve gösterilir; CelikPanel Arch'ta kendi
  kendine `pacman -Syu` çalıştırmaz, çünkü tam yükseltme çalışan sitelerin
  altındaki PHP'yi ya da PostgreSQL'i değiştirebilir.
- Arch paketi olmayan bileşenler bunu ve yerine ne yapılabileceğini söyler.

**DNS**
- Kodla katılarak kurulan çift sunucuda bir bölgeyi silmek artık terminal
  işi gerektirmez. Önceden silme, iki yönetici `dns-peer-enroll` aracıyla
  salt-okur silme denetimini kurana kadar bekliyor, sonraki bütün DNS
  değişiklikleri de onun arkasında kalıyordu. Eşleştirme bu denetimi özel
  kurulum bağlantısı üzerinden kendisi kurar; araçla aynı yerel dosyalar
  oluşur (panel olmadan geri alınabilir). Sahibin mevcut kaydı ya da elle
  değiştirdiği `sshd_config` bildirilir, üzerine yazılmaz. PowerDNS
  denetçisi PowerDNS'in kendi katalog satırını artık doğru okur (bu, elle
  kullanılan aracı da etkiliyordu).
- Yedek sunucuya kısa süre ulaşılamazken (yeniden başlatma, ağ kesintisi)
  yapılan DNS değişikliği kaydedilir ve ana sunucu onu yayımlar; DNS kartı
  değişikliğin yedeği beklediğini ve bir sonraki denetimi gösterir. Panel aynı
  değişikliği kendisi yeniden denetler (15 sn, iki katına çıkarak 5 dakikaya
  kadar) ve yedek dönünce tamamlar; arkasında bekleyen değişiklikler de
  kendiliğinden yayımlanır. Önceden bekler, aynı bölge yeniden değiştirilene
  ya da panel yeniden başlayana kadar sonraki her değişikliği tutardı.
- Yarıda kalan PowerDNS kurulumunun geri alınmasında iki btrfs düzeltmesi:
  bir klasörün bağlantı sayısı ve açılıştan açılışa değişen aygıt numarası
  artık kimlik kanıtı sayılmaz.
- Çift sunucuda günlük işler: yedek sunucu yetişirken yapılan değişiklik kısa
  bir süre bekler, sonra "DNS motoru yok" yerine bunu söyler; bekletilen
  yayın nedenini adlandırır.

**Hizmetler**
- Hizmet sürümleri paket veritabanından okunur. Sürümünü sormak için
  programı başlatmak kaldırıldı (cron'un "can't lock crond.pid" yazmasına yol
  açıyordu).

## Bilinen sınırlar

- Arch: PowerDNS yalnız ilk kurulum olarak (BIND'den geçiş yok); otomatik
  yükseltme yok; Redis (Arch yerine Valkey verir), phpPgAdmin ve
  SpamAssassin sunulmaz, çünkü Arch'ın resmi depolarında çalışan paketleri
  yok; panel yöneticinin yerine ne yapabileceğini söyler.
- Önceki bir sürümün ZeroSSL ya da Google ile verdiği sertifikalar kendiliğinden
  taşınmaz; sertifika kartı bir kez yeniden almayı ister. Snap certbot
  kapsanmaz.
- Roundcube giriş sayfasında kendi sürümünü gösterir; Roundcube'un bunu
  gizleyen bir ayarı yok.
- Otomatik silme denetimi, çift bu sürümle kurulduğunda kurulur. Daha önce
  kurulup güncellenen çift, aracın elle kullanıldığı yolu korur (güncelleme
  araç klasörünü eklemez).
- Yedeğe ulaşılamazken silinen bir alan adının DNS kısmını panel, yedek
  dönünce kendisi bitirir; alan adının silme durumu kalanı kaldırmak için bir
  kez "Bu silme işlemini yeniden dene" ister.
- Önceki bir sürümden güncellenen sunucuda mevcut alan adları DKIM anahtarını
  kendiliğinden almaz; kartları "DKIM anahtarı üret"u sunar. Bu sürümden
  önce oluşturulmuş anahtarlar, alan adı silinince korunur ve adları bildirilir.
  cPanel içe aktarmasıyla gelen alan adları ve posta kutuları otomatik DKIM'i
  başlatmaz.
- Değişen `/etc/resolv.conf`, nginx vekil hedefine ancak o sitenin yapılandırması
  yeniden yazıldığında ulaşır.

## Kanıt

Beş sürümde (Debian 12 ve 13, Ubuntu 22.04 ve 24.04, Arch) dağıtım yapılandırma
matrisi, 0 hata; panel kaldırılmışken sertifika yenileyen gerçek Ubuntu 24.04
ve Arch açılışları; Arch eşitliği ve Arch PowerDNS deneme kurulumları; bkz.
`deploy/e2e/dns-pair-acceptance/evidence/*-20261030/SUMMARY.md`. Sıra 113,
şema 46. Hiçbir P0 işi kapanmaz ya da ilerlemez; aday sürüm kabul değildir.
