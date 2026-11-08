# v0.1.0-alpha.129

[English](RELEASE-NOTES-v0.1.0-alpha.129.md)

> **Yayımlandı**, 7 Kasım 2026 (UTC): sürüm commit'i `667dba6ba`, GitHub ön sürümü 23:17:29Z, indirme portalı yaklaşık 23:22Z (dış denetim 27/27).

Veritabanları eksik kalan günlük işlerine kavuşuyor: veritabanı kullanıcısı
için yeni parola, yalnız okuyan ya da okuyup yazan ek kullanıcılar,
masaüstü araçları için bağlantı rehberi ve daha açık kartlar. Plan izin
verdiğinde sitenin müşterisi kendi bilgisayarından veritabanına bağlanabilir;
yalnız eklediği adreslerden ve her zaman şifreli. Bu sürüm, alpha.128
Debian 12, Ubuntu 22.04 ve Arch Linux'ta denendiğinde laboratuvarın
bulduklarını da düzeltiyor; en önemlisi Ubuntu 22.04 sunucuları yeniden
güncellenebiliyor. **Kurulu panelleri yalnız sahip, panelin kendi güncelleme
ekranından günceller.**

## Veritabanları

- **Veritabanı kullanıcısına yeni parola**, alan adının Veritabanları
  sekmesinde ve Veritabanları sayfasında: ortak parola alanı (üret,
  göster/gizle, kopyala, güç göstergesi), veritabanı kullanıcısı kuralı (en
  az 12 karakter, büyük ve küçük harf, özel karakter) ve iş sürerken
  sunucunun adımları. MariaDB parolayı hesabın her ana makinesinde
  değiştirir; PostgreSQL rolün kendi kümesinde saklar (SCRAM). Son adım
  siteye ne olacağını söyler: CelikPanel'in kurduğu bir WordPress ya da
  katalog uygulaması yeni parolayı yapılandırmasında alır; başka bir
  uygulama için ekran parolanın oraya elle girilmesi gerektiğini söyler.
- **Bir veritabanına ek kullanıcı:** "yalnız okuma" ya da "okuma ve yazma".
  Yalnız okuyan kullanıcı veritabanının her şemasını, sahibin ya da okuyup
  yazan bir kullanıcının sonradan oluşturduğu tablolar dahil okur, veriyi
  değiştiremez. Böyle bir kullanıcıyı kaldırmak veritabanına hiç dokunmaz.
- **Bağlantı rehberi**, "Bağlantı bilgileri" altında: site için sunucu,
  port, veritabanı ve kullanıcı; pgAdmin, DBeaver ve HeidiSQL için
  yazılacak alanlar. Yönetici SSH tüneli yolunu da görür.
- **Kartlar:** her veritabanı motorunu kendi işareti ve rengiyle, gerçek
  motor adıyla (MariaDB veritabanına artık "MySQL" yazılmıyor) gösterir;
  boyut, tablo ve açık bağlantı üç kutuda; "İçe aktar" düğmesi dosya
  seçilene kadar normal görünen kapalı bir düğme olarak kalır.
- **Dışa aktarma dosya adlarında motor yer alır**
  (`<ad>-<motor>-<zaman>.sql.gz`); aynı adlı iki farklı motor veritabanının
  indirmesi birbirinin üstüne yazılmaz.
- **Güvenlik:** veritabanı ya da kullanıcı oluşturmak, motorda zaten var
  olan birini (CelikPanel'in yaptığı ya da yapmadığı) artık devralmıyor;
  yanıt neyin dolu olduğunu adıyla söylüyor (`DATABASE_NAME_TAKEN`). Bu
  sürümden önce hesap düzeyindeki oluşturma var olan bir veritabanını
  devralıp yeni kullanıcıya üzerinde yetki verebiliyordu.

## Veritabanına dışarıdan bağlanma

Sahibin 7 Kasım 2026 kararı: varsayılan kapalı; müşteri yalnız eklediği
adreslerden bağlanır; bağlantılar her zaman şifreli; yanlış parolalar
yavaşlatılır; bir plan izniyle verilir.

- **Plan izni (`database_remote`)**, yönetici sunana kadar kapalı. Geri
  alınınca açık olan her şey kapanır.
- **Bir kural bir veritabanı, bir kullanıcı ve bir adrestir.** Kullanıcı
  başına en çok 10 adres. Bütün internet, geniş aralıklar (IPv4'te /24'ten,
  IPv6'da /64'ten geniş), yerel, bağlantı-yerel ve çoklu yayın adresleri
  reddedilir.
- **PostgreSQL:** `pg_hba.conf` içinde CelikPanel'in tuttuğu bir blokta
  SCRAM'lı `hostssl` satırları; dosyanın başka bir yerinde sahibin kendi ağ
  satırı varsa CelikPanel o kümede hiçbir şey açmaz ve satırı söyler.
- **MariaDB:** her adres için ayrı bir hesap; `REQUIRE SSL`, yerel hesapla
  aynı parola, yetkiler ve bağlantı sınırı; site askıya alınınca aynı kilit.
- **Güvenlik duvarı:** veritabanı portu yalnız listelenen adreslere ve
  yalnız CelikPanel'in süzgeci yüklü olduğu kanıtlandığı sürece açık; süzgeç
  kaybolursa bir bekçi onu yeniden yükler.
- **Nedeniyle bekleme:** bir kural, TLS açılana, motor yeniden başlatılıp
  ağda dinleyene (yönetici Bileşenler'den yeniden başlatır) ve yanlış
  parolalar yavaşlatılana kadar (MariaDB: CelikPanel'in Fail2ban kuralı;
  PostgreSQL: `auth_delay`) bekler. Ekran neyin eksik olduğunu ve kimin
  yapacağını söyler.
- pgAdmin, DBeaver ve HeidiSQL'e yazılacak **istemci ayarları**, "SSL mode:
  require" dahil.
- Uzak API anahtarları bu kuralları değiştiremez. Bağımsız bir güvenlik
  incelemesi özelliği yayından önce üç kez denetledi; her bulgu düzeltildi
  (docs/DATABASE-REMOTE-ACCESS.md).

## Posta

- **Yeniden açılan posta kutusu aynı saat içinde yeniden durdurulabiliyor.**
  alpha.128'de parola değişikliğiyle açılan bir kutu bir sonraki saate kadar
  yeniden durdurulamıyordu. Artık o saat içinde yalnız değişiklikten sonraki
  iletiler sayılıyor; ikinci bir patlama kutuyu yeniden durduruyor ve hemen
  bildiriyor.
- **Debian 12 ve Ubuntu 22.04:** değiştirilen gönderim sınırı postfwd elle
  yeniden başlatılana kadar uygulanmıyordu, yeniden açılan kutu da yaklaşık
  dört dakika reddediliyordu. postfwd 1.x artık değişen kuralları almak için
  yeniden başlatılıyor, saatlik sayımlarını koruyor; bu sırada posta akmayı
  sürdürüyor.

## Alan adı silme

- **DNS'i bekleyen bir silme kendiliğinden tamamlanır**, uzak API
  anahtarıyla başlatıldıysa da: yanıt bunu söyler ve anahtarların
  okuyabildiği bir durum adresi verir
  (`/api/v1/domains/{id}/deletion-status`). İkinci basış yeni bir iş
  başlatmaz.
- **Sürekli başarısız olan bir bitirme adımı** 5 dakika, 15 dakika ve 1
  saat sonra, ardından 6 saatte bir yeniden denenir; üçüncü başarısızlıktan
  sonra durum adımı ve hatayı "Bu silmeyi yeniden dene" ile söyler; her
  başarısızlık dizisi için tek bildirim.

## Debian 12, Ubuntu 22.04 ve Arch laboratuvarından düzeltmeler

- **Ubuntu 22.04 sunucuları yeniden güncellenebiliyor.** alpha.123'ten beri
  Ubuntu 22.04'teki her güncelleme "panel dondurulamadı" diye duruyordu
  (systemd 249 güncellemenin kullandığı bir seçeneği tanımıyor); sunucu kendi
  sürümünde sağlıklı kalıyordu. Güncelleme her zaman yeni sürümün kendi
  betikleriyle çalıştığı için alpha.123–128'deki bir Ubuntu 22.04 sunucusu
  129'a olağan biçimde güncellenir. Ubuntu 22.04'te otomatik geri alma da
  aynı biçimde düzeldi.
- **DNS, bir denetim bitmedi diye asla durdurulmaz.** Yoğun bir Debian 12
  sunucusunda yavaş bir paket denetimi panelin BIND'i durdurmasına yol
  açtı. Süresi biten bir denetim artık "bilinmiyor" sayılıyor: yeniden
  soruluyor (5, 15 ve 30 saniye sonra) ve asla hata gibi işlenmiyor. Kurulum
  ve DNS sayfası durmuş bir DNS motorunu, onu başlatan tek komutla
  gösteriyor.
- **Arch Linux:** MariaDB komutları `mariadb` istemcisiyle çalışıyor (`mysql`
  adı artık bir uyarı yazıyor; bu uyarı veritabanı boyut görünümünü ve
  reddedilen site kopyasının temizliğini bozuyordu).
- **Site kopyası:** kopyanın veritabanı kaldırılamadığında panel kayıtlarını
  tutuyor ve onu gösteriyor; hâlâ var olan bir veritabanını unutmuyor.

## Laboratuvar sonuçları

Laboratuvar bu sürümü 7 Kasım 2026'da yalnız kendi sanal makinelerinde
denedi. Her kurulum sahibin `get.sh` komutuyla yapıldı, her güncelleme
panelin güncelleme ekranından bir kez başlatıldı.

| Denenen | Sonuç |
|---|---|
| Sahibinki gibi bir DNS çifti (PowerDNS ve postalı Debian 13, BIND'li ve katılmış Ubuntu 24.04), 128 → 129 | Geçti: 67 sn ve 79 sn, hiçbir hizmet çökmedi, hiçbir yerel yapılandırma dosyası değişmedi |
| Ubuntu 22.04: 127 → 129, sıfırdan 129 kurulumu ve bilerek bozulan bir güncelleme | Geçti: güncelleme 84 sn sürdü; bozulan güncellemeden sonra otomatik geri alma 23 sn'de bitti, sonraki güncelleme başarılı oldu |
| MariaDB ve PostgreSQL'de veritabanı kullanıcıları | Geçti: eski parola reddediliyor, yalnız okuyan kullanıcı sonradan açılan tabloları okuyor ama yazamıyor, kullanıcıyı kaldırmak veritabanını koruyor, panel dışında açılmış adlar reddediliyor |
| WordPress veritabanına yeni parola | Geçti: WordPress yeni parolayı aldı; site yaklaşık bir saniye hata verdi |
| Debian 13'te (MariaDB, PostgreSQL) ve Arch'ta (PostgreSQL) dışarıdan bağlanma | Aşağıdaki düzeltmelerden sonra geçti: şifreli bağlantı çalışıyor, şifresiz bağlantı ve listede olmayan adres reddediliyor, askıya alma ve plan izninin geri alınması erişimi 13 sn (MariaDB) ve 34 sn (PostgreSQL) içinde kapatıyor, beş yanlış paroladan sonra adres engellendi |
| Posta: yeniden açılan kutunun aynı saat içinde yeniden durması | Geçti |
| DNS'i bekleyen alan adının anahtarla silinmesi | Geçti: silme, yedek DNS sunucusu yanıt verdikten 31 sn sonra kendiliğinden bitti; ikinci basış yeni iş başlatmadı |

Laboratuvar veritabanına dışarıdan bağlanmada şu hataları buldu; hepsi bu
sürümde düzeltildi:

- PostgreSQL sertifikasını okuyamadığı için yeniden başlatmadan sonra
  açılmıyordu; sertifika klasörü artık okunabilir ve her uygulamada
  onarılıyor.
- MariaDB'nin yanlış parola kuralı MariaDB'nin gerçekten yazdığı hiçbir
  satırla eşleşmiyordu, kimse engellenmiyordu; artık eşleşiyor.
- Güncelleme önceki kural dosyasını sahip yazmış gibi koruyordu; artık
  yenisiyle değiştiriliyor.
- Sunucunun eklendiğini doğrulamadığı bir adres beş dakikaya kadar
  bekliyordu; artık hemen yeniden uygulanıyor.

Laboratuvar ayrıca, silme yedek DNS sunucusunu bekleyip kendiliğinden
biterken silme durumunun "durdu, yeniden silin" dediğini buldu. Durum artık
beklediğini ve kendiliğinden biteceğini söylüyor (otomatik test kapsıyor;
laboratuvarda yeniden denenmedi).

Denenmeyenler: dışarıdan bağlanma sunucusu olarak Ubuntu 24.04, IPv6
kuralları. Kanıt: laboratuvar dalında
`deploy/e2e/dns-pair-acceptance/evidence/a129/`.

## Bilinen sınırlar

- **Veritabanına dışarıdan bağlanma:**
  - motor ağda ancak yönetici onu yeniden başlattıktan sonra dinliyor;
  - plan izninin geri alınması erişimi bir sonraki uygulamada, en geç
    yaklaşık 35 saniyede kapatıyor;
  - bir yöneticinin uzak bir MariaDB hesabına elle eklediği tablo, sütun ya
    da rol yetkilerini CelikPanel kaldırmıyor;
  - MariaDB yalnız tek IPv6 adreslerini kabul ediyor, IPv6 aralıklarını
    değil;
  - PostgreSQL'de sahibin kendi `shared_preload_libraries` değeri asla
    yeniden yazılmıyor; yönetici `auth_delay` için girilecek değeri tam
    olarak görüyor;
  - MariaDB, adresin adını bulabildiğinde bağlanan istemcinin adresi yerine
    adını yazıyor (MariaDB'de ad sorgusu varsayılan olarak açık); Fail2ban
    kuralı o istemciyi engellemiyor, o istemcinin yanlış parolaları
    MariaDB'de yavaşlatılmıyor. PostgreSQL etkilenmiyor. Düzeltmesi alpha.130 için
    planlandı;
  - sunucunun açılışı hiç bitmezse (bir hizmet takılır, systemd "açılıyor"
    durumunda kalır) CelikPanel hiçbir değişikliği uygulamıyor: geri alınan
    plan izni açılış bitene kadar erişimi açık bırakıyor, ekran ise izni
    geri alınmış gösteriyor;
  - elle yazılan `nft flush ruleset` CelikPanel'in güvenlik duvarını
    siliyor. Veritabanı süzgeci yaklaşık 15 saniyede geri geliyor (bu sırada
    motorlar listede olmayan adresleri girişte yine reddediyor); güvenlik
    duvarının geri kalanı sunucu bir sonraki yeniden başlatıldığında geri
    geliyor.
- **Veritabanı kullanıcıları:** yalnız okuyan bir PostgreSQL kullanıcısının
  neyi görüp neyi görmediği docs/DATABASE-USERS.md'de listeli. Bir
  veritabanını başka bir PostgreSQL kümesine taşımak, hedefte aynı adlı bir
  rol varsa onu kullanıyor.
- **Yeni kurulumda DNS:** ilk DNS yapılandırması sırasında bir denetim
  yeniden denemelerden sonra da bitemezse kurulum "DNS motoru yok" durumuna
  döner (henüz hiçbir şey hizmet vermiyordu), nedenini gösterir ve
  kendiliğinden yeniden denetler.
- **Uygulamalar:** bir Node.js ya da hizmet uygulamasının korumalı alanı
  okunamadığında uygulama korumasız çalıştırılmaz, durdurulur.
- Önceki sınırlar sürüyor (alpha.128'e bakın).

## Kanıt

Geçişler (ikisi de eklemeli):

- **073 `mail_outgoing_resume_baseline`:** `mailbox_outgoing` üzerinde iki
  sütun (`resume_base_recipients`, `resume_base_refusals`, varsayılan 0).
- **074 `database_remote_rules`:** `database_remote_rules` tablosu.

Her geçişte olduğu gibi alpha.128 bu geçişleri içeren bir veritabanını
reddeder; geri dönmek, güncellemeden önce alınan anlık görüntüyü geri
yüklemek demektir. Sürüm sırası 128 → 129.

alpha.128'in Debian 12, Ubuntu 22.04 ve Arch denemelerinin laboratuvar
kanıtı: laboratuvar dalında
`deploy/e2e/dns-pair-acceptance/evidence/a128-other-systems/`.
