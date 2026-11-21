# v0.1.0-alpha.142

[English](RELEASE-NOTES-v0.1.0-alpha.142.md)

> **Yayımlandı**, 21 Kasım 2026 (UTC): sürüm commit'i `7e51e951a`, GitHub ön sürümü 12:51:06Z, indirme portalı yaklaşık 12:54Z (dış denetim 27/27).

**Bir güvenlik sürümü: lütfen yakında güncelleyin.** Bir müşterinin ya da
bayinin başka bir hesaba ya da sunucunun kendisine ait adları, portları ve
dosyaları alabildiği yolları kapatır: başka bir müşterinin alan adının ya
da sunucunun kendi adlarının içinde ya da üstünde bir alan adı, başka bir
sitenin sistem hesabı, sunucunun kendi hizmetlerinden birinin portu ve
bir bağlantı üzerinden başka bir hesabın dosyaları.
**Kurulu panelleri yalnız sahibi, panelin kendi güncelleme ekranından
günceller.**

## alpha.141'den güncelleme

1. Her zamanki gibi panelin kendi güncelleme ekranından güncelleyin.
2. **Panelin veritabanı yapısı değişir** (şema 80): her yeni adı denetleyen
   altı kural eklenir; hiçbir tablo, sütun ya da satır değişmez. Zaten
   çakışan adlar tutan bir veritabanı da olağan biçimde güncellenir
   (aşağıda "Zaten çakışan adlar"). Sonradan alpha.141'e dönmek,
   yapıyı değiştiren her sürümde olduğu gibi, güncellemenin aldığı anlık
   görüntüyü geri yüklemek demektir.
3. Panel güncellemeden sonraki ilk açılışında her sitenin web sunucusu
   dosyasını yeni bağlantı kuralıyla yeniden yazar ve siteye özel kendi
   dosyalarının kiplerini bir kez düzeltir (aşağıda "Dosyalar"). Basmanız
   gereken bir şey yok.
4. **alpha.141'e dönmek kapatılan yolları yeniden açar.**

## Güvenlik: kapanan yollar

- **Başka bir hesabın ya da sunucunun adı.** Bir müşterinin ya da bayinin
  aboneliği, başka bir hesabın alan adına eşit, onun içinde ya da üstünde
  bir alan adı, alt alan adı ya da takma ad alabiliyordu; sunucunun kendi
  adlarını da (ana alan adının bölgesi; panelin, ad sunucularının, postanın
  ve web postasının adresleri). Böylece o adın DNS'ini, web sitesini,
  sertifikasını ve postasını ele geçirebiliyordu. Artık böyle bir ad, hiçbir
  şey oluşturulmadan reddedilir; panel reddeder, panelin veritabanı yeniden
  reddeder. Kendi alan adınızın alt alan adı her zamanki gibi alt alan
  adıdır. Yöneticinin kendi aboneliği böyle bir adı bilerek tutabilir;
  örneğin ana alan adının web sitesi.
- **İki alan adı için tek sistem hesabı.** İki farklı alan adı aynı sistem
  hesabı adına varabilir. Hesabı başka bir sitede ya da sistemde zaten olan
  bir alan adı artık hiçbir şey yapılmadan reddedilir; sunucudaki aracı da
  hesabın adıyla bir şey silmeden önce (zamanlanmış görevler, ek FTP
  girişleri, süreçler, hesabın kendisi) hesabın o siteye ait olduğunu
  kanıtlar. Önceden reddedilen bir oluşturma, başka bir müşterinin
  zamanlanmış görevlerini ve ek FTP girişlerini silebiliyordu.
- **Bir Node.js sitesinin portu.** Bir site sunucunun kendi hizmetlerinden
  birinin portuna (örneğin veritabanı web araçlarına) yönlendirilip onları
  internete açabiliyordu. Artık Node.js sitesinin portu 3001 ile 3999
  arasından gelir ve hiçbir zaman başka bir sitenin ya da sunucu
  hizmetlerinin portu olmaz; sunucudaki aracı, panel ne gönderirse göndersin
  böyle bir portu yeniden reddeder.
- **Web sunucusuna başka bir hesabın dosyasını sunduran bağlantı.** Web
  sunucusu bir sitenin klasöründeki bağlantıyı başka bir hesabın dosyasına,
  ya da okuyabildiği sunucu dosyalarına kadar izliyordu. Artık bağlantıyı
  yalnız bağlantının kendi sahibinin dosyasına izler.
- **Her hesabın okuyabildiği yapılandırma dosyaları.** Her sitenin web
  sunucusu ve PHP dosyaları sunucudaki her hesapça okunabiliyordu; artık
  yalnız onları kullanan hizmetler okur. Barındırma klasöründen geçilebilir
  ama içi listelenemez.
- **Sınırları kaldıran yönergeler.** Bir müşteriye verilebilen ek web
  sunucusu yönergeleri artık bağlantı kuralını kapatamaz, klasörleri
  listeleyemez, sitenin PHP sınırlarını yükseltemez ve web sunucusuna bir
  dosyayı yönetici olarak okutamaz.
- **Alan adı eklemenin erişimden önce okudukları.** Artık istekte adı geçen
  bir aboneliğe, onu kullanma izni denetlenmeden bakılmaz.

## Sizin için ne değişiyor

### Bir ad kullanılamadığında tek yanıt

Adı reddedilen müşteri ya da bayi, neden ne olursa olsun tek cümle okur:
bu ad ya da üstündeki veya altındaki bir ad bu sunucuda zaten kullanılıyor
ya da sunucuya ayrılmış; ad onunsa sunucunun yöneticisine sorar. Yanıt adı
kimin tuttuğunu ya da sunucunun ayırıp ayırmadığını hiçbir zaman söylemez;
söyleseydi, ret herkese burada hangi alan adlarının barındırıldığını
öğretirdi. Kendi adları onlara kendi adları olarak söylenir. Yönetici tam
nedeni ve öbür adı görür.

Zaten kullanılan bir ek FTP giriş adı için de aynı tek yanıt verilir; adları
alacak hesabın tutamayacağı bir abonelik devri reddedilir ve adlar yalnız
yöneticiye listelenir.

### Zaten çakışan adlar

Güncelleme var olan hiçbir şeyi değiştirmez. Bir müşteri başka bir hesabın
adının içinde ya da üstünde bir ad, ya da sunucunun adlarından birini zaten
tutuyorsa, ya da bir Node.js sitesi sunucu hizmetlerinden birinin portunu
zaten kullanıyorsa, yönetici bunu **Şu anki sorunlar** listesinde (Pano ve
İzleme) yapılacakla birlikte görür:

- başka bir hesabın adı: adı kaldırın ya da onu tutan aboneliği öbür adı
  tutan hesaba devredin;
- sunucunun adı: o hesabın bu adı tutması gerçekten gerekmiyorsa kaldırın;
  sunucunun adlarını yalnız bir yöneticinin kendi aboneliği tutar;
- bir sunucu hizmetinin portundaki Node.js sitesi: Node.js ayarlarını
  yeniden kaydedin; site boş bir port alır, programı o portu dinlemelidir.

Her satır, sorun çözülünce kendiliğinden kalkar. Panel bunları açılışta
kendi günlüğüne de yazar.

### Başka bir hesabın dosyalarına bağlantılar

Bir sitenin klasöründe sitenin kendi dosyasına giden bağlantı çalışmayı
sürdürür. Başka bir hesabın ya da sunucunun dosyasına giden bağlantı artık
sunulmaz; ziyaretçi "bulunamadı" ya da "yasak" görür. Sitenin **Günlükler**
ekranı böyle bir satırı tek cümleyle açıklar: bağlantı yerine dosyanın bir
kopyasını siteye koyun. Sahip bir site için bağlantı kuralını kendisi
yazdıysa CelikPanel onun satırını yerinde bırakır.

### Bir kez düzeltilen dosyalar

Güncellemeden sonraki ilk açılışta, CelikPanel'in herkesçe okunabilen ve
değişmemiş siteye özel kendi dosyaları yalnız hizmetlerince okunur olur
(0640); barındırma klasörü ve abonelikler klasörü geçilebilir ama
listelenemez olur (0711). Sahibin daha önce değiştirdiği bir kip korunur
(aracının günlüğü onu henüz adlandırmıyor: "Laboratuvarda denenenler"e
bakın); sahibin sonradan verdiği bir kip hiçbir zaman geri değiştirilmez.

### Node.js portları

Bir Node.js sitesi 3001 ile 3999 arasından boş bir port alır. Başka bir
sitenin ya da sunucu hizmetlerinden birinin kullandığı port, onu kim
kullanırsa kullansın tek cümleyle reddedilir; portu boş bırakırsanız panel
seçer. Bir sitenin var olan portu, sunucu hizmetlerinden birinin portu
olmadıkça korunur.

## Bilinen sınırlar

- Web sunucusunun grubu hâlâ her sitenin klasörünü okuyabilir; her siteye
  kendi grubu verilene kadar siteler arasındaki engel bağlantı kuralıdır.
- Her sitenin hesabı sistemin hesap listesinde görünür ve adı alan adından
  türetilir; numarayla adlandırılan site hesapları sonra gelir.
- Postanın adres eşlemeleri hâlâ her hesapça okunabilir.
- Posta hâlâ "Kimden" satırındaki, burada barındırılan her alan adı için
  imzalanır; yalnız gönderenin kendi alan adı için imzalamak sonraki
  sürümdür.
- PHP ve zamanlanmış görevler hâlâ siteye özel korumalı alanın ve onun
  sınırlarının dışında çalışır.
- Çalışan süreçler ve komut satırları her hesaba görünür.
- Bir Git deposundan kurulan site, sunucunun kendi üzerindeki bir adresten
  kopyalama yapabilir (sitenin kendi korumalı alanında çalışır; o alan bu
  adreslere zaten ulaşabilir).
- Bir uzak yedek hedefini denemek sunucunun ağı içindeki adreslere
  ulaşabilir.
- Ad ve port denetimleri panelin içinde ve panelin veritabanında çalışır;
  sunucuda elle verilen bir ad ya da port denetlenmez.
- Laboratuvar dört sınır daha buldu; her birinin alpha.143'e dek çıkış yolu
  aşağıda, "Laboratuvarda bulunan, alpha.143'e planlananlar" altında.

## Laboratuvarda denenenler

Laboratuvar denemesi bitti: 21 Kasım 2026'da tek deneme (UTC 09:07'den
yaklaşık 10:57'ye: denetimler 120 dakikalık sürenin 81 dakikasını aldı,
ardından toparlama), bir test sunucusu çiftinde: PowerDNS'li Debian 13 ana
DNS sunucusu ve BIND'li Ubuntu 24.04 yedek DNS sunucusu, alpha.141
durumundan başlatıldı. Sürüm, bu sürümün kodundan derlenen, yalnız
laboratuvara ait bir etiketle (v0.1.0-alpha.149) çalıştı. İki tarafı, aynı
bayinin ayrı hesaplardaki iki müşterisi oynadı: PHP siteleri, zamanlanmış
görevleri ve ek FTP girişleri olan alice ve onun şeylerine ulaşmaya çalışan
bob. Laboratuvarın yaptığı yazılanlar dışında her şey sayfanın kendi
düğmeleriyle yapıldı. Ağır hata yok: güncellemeden sonra başka bir hesabın
hiçbir şeyine ulaşılamadı.

- **Güncellemeden önce yollar açıktı** (alpha.141'in kodu): bob, alice'in
  alan adının altında bir ad ekledi ve ad iki sunucuda da DNS'te yanıt
  verdi; bob'un zamanlanmış görevinin sitesine koyduğu bağlantılar, alice'in
  sitesinin yanındaki bir dosyaya, sitesinin PHP yapılandırma dosyasına ve
  web sunucusunun okuyabildiği bir sunucu dosyasına, içerikleriyle sunuldu
  (PHP dosyası kaynak koduyla).
- **İki sunucu da panelin kendi güncelleme sayfasından**, kendiliğinden,
  yaklaşık 1,5 ve 3 dakikada güncellendi. Şema 79 → 80, bob'un çakışan adı
  dururken uygulandı; panel açıldı ve onu listeledi. Yeni hatalı hizmet yok.
  Web, posta, DNS ve veritabanları önceki gibi yanıt verdi: iki sunucudan
  aynı DNS yanıtları, sitelerin sayfaları, MariaDB ve PostgreSQL, posta
  portları; sertifika yenilemenin deneme çalıştırması iki sunucuda da
  başarılı oldu.
- **Bağlantı üzerinden hiçbir dosya yok.** Güncellemeden sonra bob'un
  alice'in dosyalarına ve sunucu dosyasına giden her bağlantısı bob'un kendi
  sayfasını, "bulunamadı"yı ya da "yasak"ı verdi, dosyayı hiçbir zaman
  vermedi. Parolayla korunan bir klasörün içindeki bağlantı ve alice'in site
  klasörüne giden bir bağlantıyla değiştirilen site klasörü de öyle; onun
  dosyasını gösteren özel bir hata sayfası kaydedilirken reddedildi. bob'un
  kendi dosyasına giden bağlantı sunulmayı sürdürdü. Sitenin **Günlükler**
  ekranı reddedilen satırları tek cümleyle açıklıyor (Türkçe ve İngilizce).
- **Hiçbir ad yok.** bob, alice'in alan adlarının ve sunucunun kendi
  adlarının (ana bölge, sunucuların ve panelin adları, ad sunucuları,
  posta, web postası) altında, onlara eşit ve üstünde adları, alan adı ve
  takma ad olarak denedi. Her biri Türkçe ve İngilizce, kimseyi
  adlandırmayan aynı cümleyle reddedildi; hiçbir şey oluşturulmadı ve
  hiçbir DNS yanıtı değişmedi. Güncellemeden önceki çakışma **Şu anki
  sorunlar**da yapılacakla birlikte listelendi; bob onu sayfadan sildi ve
  satır kendiliğinden kalktı.
- **Ortak bir sistem hesabı üzerinden hiçbir zamanlanmış görev ya da FTP
  girişi yok.** Sistem hesabı alice'in bir alan adında zaten olan bob'un
  alan adı, hiçbir şey yapılmadan aynı cümleyle reddedildi; alice'in
  hesapları, zamanlanmış görevleri ve FTP girişleri değişmedi, girişleri
  oturum açmayı sürdürdü. Tek bir hesaba varan iki uzun ad için de aynısı.
  Zaten kullanılan bir FTP giriş adı, kimin tuttuğunu söylemeyen tek bir
  cümle aldı. bob, site klasörünü alice'inkine giden bir bağlantıyla
  değiştirdiği kendi alan adını sildiğinde alice'in hiçbir şeyine
  dokunulmadı.
- **Hiçbir sunucu portu yok.** Sayfadan açılan yeni bir Node.js sitesi 3001
  portunu aldı. Veritabanı web araçlarının portu, posta süzgecinin denetim
  portu, veritabanı sunucusunun portu, bir programın portu, programlara ve
  hizmet uygulamalarına ayrılan aralıktan başka bir port ve başka bir
  sitenin portu her istendiğinde tek cümleyle reddedildi; hiçbir şey
  değişmedi.
- **Yapılandırma dosyaları**: iki sunucuda da siteye özel her web sunucusu
  ve PHP dosyası yalnız hizmetlerince okunur oldu (0640), barındırma
  klasörü geçilebilir ama listelenemez oldu (0711); laboratuvarın sahip
  olarak kipini değiştirdiği bir dosya kipini korudu. bob'un site hesabı
  barındırma klasörünü listeleyemedi, alice'in web sunucusu ve PHP
  dosyalarını okuyamadı.
- Güncellemeden sonra ilgisiz işler: yönetici bir alan adı oluşturdu, bir
  takma ad ekledi ve alan adını sildi; alan adı iki sunucuda da DNS'te yanıt
  verdi ve silindikten sonra yanıt vermeyi bıraktı. İki sunucuda da olağan
  sayfalar hatasız açıldı.

**Laboratuvarın sayfanın yolundan ayrıldığı yerler.** Port retleri ve
yöneticinin başka bir hesaba ad yerleştirmesi doğrudan panelin API'sine
gönderildi. Laboratuvar, laboratuvar sunucularında root olarak, alice'in
uydurma gizli dosyalarını ve bir sunucu dosyasını koydu, bir sitenin web
sunucusu dosyasının kipini bir sahibin yapacağı gibi değiştirdi, sertifika
aracını laboratuvarın kendi sertifika yetkilisine kaydetti ve iki kez
`systemctl reset-failed` çalıştırdı (aşağıdaki üçüncü sınırın çıkış yolu).

**Denetlenmeyenler:**

- Güncellemeden sonra bir sunucu portundaki var olan bir Node.js sitesi ve
  bir Node.js sitesinin genel klasöründeki bağlantı: alpha.141'in kendisi
  laboratuvarda böyle bir site açmayı zaten reddetti, bu yüzden bu durum
  hazırlanamadı.
- Bir müşterinin web sunucusu yönergesinin ekrandaki reddi: laboratuvarın
  müşterilerine yönergeler kutusu sunulmamıştı; bu yalnız testlerle
  kapsanıyor.
- Güncellemeden sonra web postası ve veritabanı web aracı: açılmadı.
- Bir müşterinin ikinci sunucuda denediği adlar: laboratuvarın müşterisinin
  orada hesabı yok (ikinci sunucunun adları birincide denendi).

### Laboratuvarda bulunan, alpha.143'e planlananlar

Hiçbiri bir güvenlik ya da veri kaybı sorunu değil.

1. Alan adı sunucunun kendi adlarından birinin altında olan bir müşteri
   (örneğin müşterilerine kendi ana alan adının altında adlar veren bir
   sunucuda) o alan adının alt alan adını ekleyemiyor; yanıt, kimseyi
   adlandırmayan ret cümlesi. **Şu anki sorunlar** böyle bir müşteri alan
   adını, hiçbir sorunu olmadığı ve yapılacak bir şey bulunmadığı hâlde,
   sunucunun adlarından biriyle çakışıyor diye listeliyor. alpha.143'e dek
   yönetici alt alan adını yöneticinin kendi hesabında ekleyebilir.
   Müşterileri kendi alan adlarını kullanan bir sunucu bundan etkilenmez.
2. Yöneticinin başka bir hesaba ad yerleştirmesi reddedildiğinde cümle,
   adı tutan hesabın daha yakın adı yerine isteğin çakıştığı sunucu adını
   söylüyor.
3. Uygulaması hatalı durumdaki bir Node.js sitesi silinmiyor: sayfa
   silmenin tamamlanmadığını söylüyor ve satır "Silinmeyi bekliyor" olarak
   kalıyor. Bugünkü çıkış yolu, sunucuda root olarak:
   `systemctl reset-failed celikapp-<id>.service` (`systemctl --failed`
   onu listeler), ardından yeniden Sil. alpha.142'den önceki sürümlerin de
   böyle davranıp davranmadığı belirlenmedi.
4. Aracının günlüğü, sahibin değiştirdiği bir dosya kipi korunduğu hâlde
   "0 kept" (0 korundu) diyor (kipin kendisi doğru biçimde korundu).

### Güncellemeden sonra ne beklenir

- Güncelleme birkaç dakika sürer ve bittiğini kendiliğinden söyler.
- Yapı şema 80'dir: alpha.141'e dönmek, güncellemeden önce alınan anlık
  görüntüyü ya da yedeği geri yüklemek demektir.
- Başka bir hesabın dosyalarına giden bağlantılar artık sunulmaz.
- Siteye özel web sunucusu ve PHP dosyalarını başka hesaplar artık okuyamaz.
- Zaten çakışan adlar ve portlar yöneticiye **Şu anki sorunlar**da
  listelenir; hiçbir şey değiştirilmez ya da kaldırılmaz.

## Kanıt

Şema 079 → 080: altı veritabanı kuralı (yeni alan adlarında, yeniden
adlandırılan ya da taşınan alan adlarında, yeni ya da değişen takma
adlarda ve bir aboneliğin yeni sahibinde); tablo, sütun ya da satır yok.
Daha önce oluşmuş çakışmaları tutan bir veritabanı da güncellenir ve
çalışmayı sürdürür (`TestSchema79DatabaseWithOverlapsUpgradesAndKeepsWorking`).
Yeni ve hepsi eklemeli: `HOSTNAME_NOT_AVAILABLE`, `APP_PORT_NOT_ALLOWED` ve
`TRANSFER_NAME_OVERLAP` ret kodları, aracının `site_identity_exists`
yanıtı, site parçasının `refused` sonucu, "Şu anki sorunlar"da üç tür, bir
aracı makbuz dosyası.

Kapatılan her yolu deneyen, alpha.141'in kodunda başarısız olup bu
sürümünkinde geçen testler: `TestNameRuleProofAnotherAccountsNameIsRefused`,
`TestNameRuleProofTheServersNamesAreRefused`,
`TestNameRuleProofTheInfrastructureZoneIsNotAdopted`,
`TestNameRuleProofASharedSystemAccountIsRefusedBeforeCreation`,
`TestNameRuleProofARefusedCreationIsNotCompensatedByAccountName`,
`TestNameRuleProofANodePortOfAServerServiceIsRefused`,
`TestNameRuleParentLookupWaitsForTheAccessCheck` (panel);
`TestDeleteSiteLeavesAnotherSitesAccountCrontabAndLoginsAlone`,
`TestNodeVhostRefusesAPortOfTheServersOwnServices`,
`TestTenantIsolationFTPLoginNameTakenSaysNothingOfWhoHasIt`,
`TestTenantIsolationHostingBaseIsNotListable` (aracı);
`TestTenantIsolationEveryBlockThatServesFilesRefusesForeignLinks`,
`TestTenantIsolationCustomerDirectivesCannotLiftTheRules`,
`TestTenantIsolationNewSiteFilesAreNotWorldReadable` (hizmetler). Bağlantı
kuralı Debian 13'ün kendi nginx 1.26.3'ünde denendi: bağlantı üzerinden
başka bir hesabın dosyası reddedildi, sitenin kendi dosyası sunuldu.

Testler, test makinesinde her paketin tam takımı, önce (alpha.141) ve
sonra:

| Paket | Önce | Sonra |
|---|---|---|
| `internal/transport` | 62 | 62 |
| `internal/panellisten` | 4 | 4 |
| `internal/sitesandbox` | 18 | 19 |
| `internal/db` | 180 | 182 |
| `internal/services` | 364 | 371 |
| `cmd/agent` | 5937 | 5948 |
| `cmd/panel` | 4713 | 4726 |

(Geçen testler ve alt testler.)

Yeni başarısızlık yok. Web: `npm test` 3079 → 3090 geçti, 0 başarısız; tür denetimi temiz ve derleme her boyut bütçesini karşılıyor (ilk yükleme yolu sıkıştırılmış 110,00 KiB'nin 109,37 KiB'i; alpha.141: 109,36).
