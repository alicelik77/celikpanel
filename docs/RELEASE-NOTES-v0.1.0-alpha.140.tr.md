# v0.1.0-alpha.140

[English](RELEASE-NOTES-v0.1.0-alpha.140.md)

alpha.138 ve alpha.139 laboratuvarlarından taşınan dört işin küçük bir
düzeltme sürümü. En önemlisi bir veri kaybı riskini kapatıyor: bir
veritabanında **Sil**'e yeniden basmak, arada aynı adla açılmış başka bir
veritabanını silebiliyordu. Diğer üçü, üç ekranın gerçekte ne olduğunu
söylemesini sağlıyor: yedek DNS sunucusunu bekleyen alan adı silme, web
postasının kaydı için **Ekle…**'nin meşgul sonu ve henüz kurulamayan yapay
zekâ ve otomasyon uygulamaları.
**Kurulu panelleri yalnız sahip, panelin kendi güncelleme ekranından
günceller.**

## alpha.139'dan güncelleme

1. Her zamanki gibi panelin kendi güncelleme ekranından güncelleyin. Bu
   sürüm panelin veritabanı düzenini değiştirmez (alpha.139'daki gibi
   kalır); bu yüzden sonradan alpha.139'a dönmek veritabanı geri yüklemesi
   gerektirmez.
2. Güncellemeden sonra basılacak ya da ayarlanacak bir şey yok.
3. **Yeni kayıtların geri dönüşte anlamı.** Veritabanı silme artık
   sürerken panelin ayarlarında küçük bir not tutar (silinen her veritabanı
   için bir tane; adı `database_delete_v1:` ve veritabanının numarası).
   Silme bitince not kaldırılır. Böyle bir not dururken alpha.139'a
   dönerseniz alpha.139 onu görmezden gelir: orada silme bu sürümden önceki
   gibi çalışır. Notun bir zararı yoktur (veritabanı numaraları iki kez
   kullanılmaz); bu sürüm, geride kalmış bir notu bir dahaki veritabanı
   silmede kaldırır.

## Sizin için ne değişiyor

### Veritabanı bir kez silinir, yalnız istediğiniz veritabanı

Bir veritabanını silmek iki adımdır: önce veritabanı sunucusu veritabanını
siler, sonra CelikPanel onun kayıtlarını (veritabanı, kullanıcıları,
hakları) kaldırır. Bazen ikinci adım birincinin ardından gelmez:
veritabanı sunucusunun yanıtı kaybolur (zaman aşımı, yeniden başlatma,
kapanan sayfa), veritabanının kullanıcılarından ya da haklarından biri
henüz kaldırılamaz ya da kayıtlar yazılamaz. Veritabanı o zaman listede
kalır ve ekran **Sil**'e yeniden basmanızı söyler.

Şimdiye dek her basış "bu adlı veritabanını sil" diyordu. İki basış
arasında sunucuda aynı adla bir veritabanı açıldıysa, ikinci basış o yeni
veritabanını içindeki her şeyle birlikte siliyordu. Bu, kimse kötü bir şey
istemeden de olabilir: müşterinin kendi veritabanı kullanıcısı silme
bitene dek o ad üzerindeki haklarını korur, onu kullanan bir uygulama ya da
kurulum veritabanını yeniden açabilir; sahip onu elle açabilir ya da bir
yedekten geri yükleyebilir. Alan adının veritabanlarını da silmesi istenen
bir alan adı silmede yeniden basıldığında da aynısı geçerliydi.

Şimdi:

- **Silme, ilk silmeden önce hangi veritabanı olduğunu not eder.**
  Veritabanının veritabanı sunucusundaki kimliğini okur (PostgreSQL'de iç
  numarası; bu sunucunun MariaDB'sinde veritabanının durduğu klasörün
  kimliği) ve hiçbir şey silinmeden önce saklar.
- **Veritabanı sunucusu onu yalnız hâlâ aynı veritabanıyken siler.**
  Yanıtı kaybolan bir silmeden sonraki basış ya veritabanını gitmiş bulur
  ve kayıtları bitirir, ya aynı veritabanını bulur ve siler, ya da başka
  bir veritabanı bulur ve ona dokunmaz.
- **Veritabanı gittikten sonra hiçbir basış bir şeyi yeniden silmez.**
  Yalnız kullanıcıları ve kayıtları bitirir.
- **Arada aynı adla açılmış bir veritabanı olduğu gibi bırakılır**, onu
  kullanıyor olabilecek kullanıcılarıyla birlikte. Ekran bunu söyler:
  CelikPanel onu silmez; ona ne olacağına sunucu yöneticisi veritabanı
  sunucusunda karar verir, ardından **Sil** silmeyi tamamlar.
- **Kimlik okunamadığında** (klasörün ne zaman açıldığını tutmayan bir dosya
  sistemi ya da başka bir makinede kendi hesabıyla bağlanılan bir
  veritabanı sunucusu), yanıtı kaybolan bir silmeden sonraki basış ayırt
  edemediği hiçbir şeyi silmez. Ekran, şimdi oradaki veritabanının aynısı
  olup olmadığının bilinemediğini söyler; yöneticiye onu veritabanı
  sunucusunda kontrol etmesi (istenmiyorsa orada silip sonra burada
  **Sil**'e basması), müşteriye yöneticiye sorması söylenir. O zamana dek
  **Sil** hiçbir şeyi değiştirmez.

### Yedek DNS sunucusunu bekleyen alan adı silme bunu söyler

Bir çiftin ana DNS sunucusunda bir alan adı silme, önce yedek DNS
sunucusunun alan adı listesini onaylamasını bekleyebilir. Böyle bir silmenin
hemen ardından durum "Bu silme işlemi bitmeden durdu … alan adını yeniden
silin" diyordu; oysa kısa süre sonra kendiliğinden bitiyordu (alpha.138
laboratuvarında 51 saniye). Artık silmenin yedek DNS sunucusunu beklediğini,
kimsenin bir şey yapması gerekmediğini ve kendiliğinden biteceğini söyler;
alan adı listesi de kendiliğinden boşalır. Onu kendiliğinden bitirecek bir
şey yoksa yine "yeniden silin" der.

### Web postasının kaydı için "Ekle…": meşgul yanıtı bir yanıt olarak gösterilir

Ana DNS sunucusu "meşgul" yanıtını verdiğinde ilerleme penceresinin ilk
adımı bekleme saatini koruyordu. Artık adım tamamlanmış görünür ve "…
sunucusu yanıt verdi" der; yanıtın ne anlama geldiği aşağıda bir kez
söylenir.

### Yapay zekâ ve otomasyon uygulamaları neyin eksik olduğunu ve onu kimin kuracağını söyler

- Henüz kurulamayan bir uygulama, sizin dilinizde, neye ihtiyaç duyduğunu,
  bu sunucuda ne olduğunu ve sonraki adımı söyler. Örneğin: "Node.js 24
  gerekir. Bu sunucuda henüz Node.js kurulu değil. Sizin için sonraki adım:
  Bileşenler → Node.js bölümünden Node.js 24 sürümünü kurun." Sunucunun
  kendi İngilizce sözleri artık Türkçe bir cümlenin içinde görünmez.
- Yöneticinin düğmesi işin yapıldığı yeri açar: sürüm listesinde
  Bileşenler → Node.js (başka bir sürüm kuruluyken de) ya da Ubuntu 24.04'te
  Python'un venv modülünü eklemek için Bileşenler → Python; o da siz
  onaylayınca işletim sisteminin python3-venv ve python3.12-venv paketlerini
  kurar. Onay penceresi bu paketlerin adını henüz söylemiyor (bkz.
  "Laboratuvarda bulunan, sonraki sürüme bırakılanlar").
- Bu sunucunun işletim sisteminin çalıştıramadığı bir uygulama (Debian 13'te
  Open WebUI) yöneticiye nedeniyle gösterilir, müşterilere ve bayilere
  gösterilmez ve onlara sunulamaz. Bir alan adının zaten tuttuğu uygulama,
  yönetilebilsin ve kaldırılabilsin diye kartında kalır.
- Ekranın hiç göndermediği, Node.js için genel paket kurulumu hiçbir şey
  sıraya girmeden, sürüm listesine giden yolla birlikte reddedilir.

## alpha.138 notlarına bir düzeltme

alpha.138 notları, yapay zekâ ve otomasyon uygulamalarının Debian 13 ve
Ubuntu 24.04'te kurulamadığını söylüyordu. Bu doğru değildi: o laboratuvar,
ekranın hiç göndermediği bir kurulum isteği gönderdi. Doğrusu şu:

- **n8n**, yönetici Bileşenler → Node.js bölümünden Node.js 24'ü seçtikten
  sonra kurulur.
- **Ubuntu 24.04'te Open WebUI** önce Bileşenler → Python'u (Python'un venv
  modülü) gerektirir; bu sürümün laboratuvarı onu bu yolla, ilk kez kurdu.
- **Debian 13'te Open WebUI** dağıtımın Python'uyla çalışamaz (3.13; Open
  WebUI 3.11 ve 3.12'yi destekler); bu yüzden orada sunulmaz.

Yayımlanmış alpha.138 notları olduğu gibi bırakıldı; düzeltme bu
paragraftır.

## Bilinen sınırlar

- **Silme aynı adlı başka bir veritabanıyla** ya da ayırt edemediği bir
  veritabanıyla karşılaşınca, panelde o veritabanını tutup girişi kapatan
  bir düğme yok: yönetici onu veritabanı sunucusunda kaldırır ya da yeniden
  adlandırır, ardından **Sil** silmeyi tamamlar.
- **Bitmeyen bir oluşturmayı geri almak** (alan adı veritabanı, site
  kopyası, uygulama kataloğu, WordPress, veritabanı araçlarının kopyası),
  aynı isteğin az önce yaptığını hâlâ adıyla siler.
- **Başka bir makinedeki MariaDB'de** kimlik yoktur; orada yanıtı kaybolan
  bir silmeden sonra güvenli tek yanıt "ayırt edilemiyor"dur.
- **Open WebUI çok yer ister ve sayfa bunu denetlemez.** Laboratuvarda
  kurulumu 6 dakika sürdü; kök dosya sisteminin kullanımı kurulumun en yoğun
  anında yaklaşık 15 GB arttı (23 GB'lık disk %96'ya çıktı), yaklaşık 9 GB'ı
  kaldı (klasörü 8,3 GB); çalışırken yaklaşık 2 GB bellek kullandı. Sayfa
  önceden boş diski ya da belleği denetlemiyor ve süren bir kurulum sayfadan
  durdurulamıyor. Yönetici onu sunmadan ya da kurmadan önce boş diske (en az
  15 GB) ve belleğe bakmalı. n8n yaklaşık 3 GB yer kapladı ve dokuz dakika
  sürdü.
- **Bir Node.js sürümü** resmî sağlama dosyasına göre denetlenir, ama o
  dosyanın imzası henüz denetlenmiyor.
- **Agent yanıt veremezken** işletim sistemi yüzünden hiçbir şey
  gizlenmez; kurulum yine Agent'a sorar ve o zaman reddeder.

## Laboratuvarda denetlenenler

Laboratuvar denemesi bitti: 20 Kasım 2026'da tek deneme (UTC 09:00'dan
yaklaşık 10:25'e, 100 dakikalık sürenin 85 dakikası), bir test sunucusu
çiftinde: PowerDNS'li Debian 13 ana DNS sunucusu ve BIND'li Ubuntu 24.04
yedek DNS sunucusu, alpha.139 durumundan başlatıldı. Sürüm, bu sürümün
kodundan derlenen, yalnız laboratuvara ait bir etiketle (v0.1.0-alpha.147)
çalıştı. Aksi yazılmadıkça her şey sayfanın kendi düğmeleriyle yapıldı. Ağır
hata yok.

- **İki sunucu da panelin kendi güncelleme sayfasından**, kendiliğinden,
  yaklaşık iki ve üç dakikada güncellendi. İkisi de basıştan hemen sonra
  başlayan, işletim sisteminin günlük paket işini bekledi; sayfa
  güncellemenin onu bekleyeceğini önceden söylüyor. Hiçbir hizmet başarısız
  kalmadı; iki sunucu da aynı DNS sorularına öncesinde ve sonrasında aynı
  yanıtı verdi.
- **Veritabanları sayfasından veritabanı silme**, MariaDB'de ve
  PostgreSQL'de: ilerleme penceresi, ardından "Tamamlandı"; satır listeden
  kendiliğinden çıktı ve veritabanı sunucudan gitti.
- **Bir veritabanı asla iki kez silinmez.** MariaDB'de (Debian 13 ve Ubuntu
  24.04) ve PostgreSQL'de (Debian 13) bir silmenin yanıtı bilerek kesildi
  (laboratuvar basış anında Agent'ı durdurdu). Her seferinde veritabanı
  silinmişti, kullanıcıları ya da hakları henüz bitmemişti ve sayfa **Sil**'e
  yeniden basmayı söyledi. Ardından aynı adla, yeni satırlarla bir veritabanı
  yeniden açıldı: Ubuntu 24.04'te müşterinin kendi veritabanı kullanıcısıyla,
  yani bu sürümün tam olarak ele aldığı durum; Debian 13'te elle. Her
  seferinde bir kez Türkçe, bir kez İngilizce sayfada yeniden basılan **Sil**
  yeni veritabanını ve satırlarını yerinde bıraktı; sayfa CelikPanel'in onu
  silmediğini ve kararı yöneticinin verdiğini söyledi. Laboratuvar o
  veritabanını elle silince **Sil** silmeyi bitirdi. Kimlik her seferinde
  okundu: MariaDB yeni klasöre aynı düğüm numarasını verdi, onları düğüm
  kuşağı ve klasörün açıldığı an ayırdı; PostgreSQL'de iç numarası.
- **Veritabanlarıyla birlikte alan adı silme** (bir MariaDB, bir
  PostgreSQL): adlarıyla "2 veritabanı silindi", 14 saniyede; alan adı
  listeden kendiliğinden çıktı; sunucularda ona ait veritabanı ya da
  veritabanı kullanıcısı kalmadı.
- **Debian 13'te yapay zekâ ve otomasyon uygulamaları**, yönetici olarak:
  n8n'in satırı Türkçe olarak Node.js 24 gerektiğini, kurulu olmadığını ve ne
  yapılacağını söyledi; **Node.js 24 sürümünü kur** düğmesi Bileşenler →
  Node.js'i sürüm listesinde açtı. Open WebUI nedenini (Python 3.13)
  gösterdi, sunulamadı ve müşteriler onu görmedi. Doğrudan gönderilen genel
  Node.js kurulum isteği reddedildi, hiçbir şey sıraya girmedi.
- **Debian 13'te sayfadan n8n kuruldu**: Bileşenler'den Node.js 24
  (saniyeler), n8n bir müşterinin alan adına sunuldu ve kuruldu (9 dakika
  24 saniye, yaklaşık 3 GB); HTTPS üzerinden yanıt verdi, kartın kendi
  yeniden başlatmasından sonra da.
- **Ubuntu 24.04'te sayfadan Open WebUI kuruldu**, her yerde ilk kez:
  Bileşenler → Python onarımı python3-venv ve python3.12-venv paketlerini
  ekledi, ardından Open WebUI sunuldu ve kuruldu (6 dakika 18 saniye); HTTPS
  üzerinden yanıt verdi ve dururken yaklaşık 2 GB bellek kullandı. Disk
  kullanımı için "Bilinen sınırlar"a bakın.
- **"Ekle…"nin meşgul sonu** telefon genişliğinde, Türkçe ve İngilizce: ilk
  adım "… sunucusu yanıt verdi" ile tamamlanmış, meşgul cümlesi bir kez,
  ardından **Yeniden sor…** kaydı ekledi.
- İki sunucuda da olağan sayfalar hatasız açıldı.

**Denetlenmeyenler:** alan adı silmenin "yedek DNS sunucusunu bekliyor"
cümlesi. Çiftteki iki alan adı silme de 12-14 saniyede bitti, çünkü yedek
sunucu onayı silmenin kendisi sırasında verdi; bekleme durumu oluşmadı,
"durdu … alan adını yeniden silin" de görünmedi. Müşterinin "Node.js'i
yönetici kurar" satırı (o sırada n8n müşterilere sunulmuyordu). Hem program
hem yapay zekâ uygulaması tutan bir alan adı (süre yüzünden yapılmadı).

### Laboratuvarda bulunan, sonraki sürüme bırakılanlar

Hiçbiri bir güvenlik ya da veri kaybı sorunu değil.

- Bir yapay zekâ uygulamasının kurulumu sayfadan durdurulamıyor ve önceden
  boş diski hiçbir şey denetlemiyor: Open WebUI'ın kurulumu laboratuvarın
  23 GB'lık diskini %96'ya çıkardı.
- Debian 13'te yöneticinin bir alan adının Programlar bölümündeki görünümü,
  Open WebUI'ın bu sunucuda sunulmadığını ve Katalog yönetiminde sunulmasını
  söylüyor; oysa orada sunulamıyor.
- Python onarımının onay penceresi onay gerektiren başka bir şey olmadığını
  söylüyor ve ardından kurduğu python3-venv ile python3.12-venv paketlerini
  adlandırmıyor.
- Yanıtı kaybolan bir silmeden sonra aynı adlı veritabanıyla ilgili cümle,
  kullanıcılarına "dokunulmadığını" söylüyor; oysa MariaDB'de kullanıcı ilk
  basışla zaten kaldırılmıştı ve panel onu hâlâ listeliyordu.
- Bir alan adı silme sırasında veritabanı adımı, iki veritabanı silinirken
  yaklaşık 9 saniye "İşlenecek veritabanı yoktu" dedi (sonuç doğruydu).
- "Silmeden sonra bu adla bir veritabanı açılmış" durumu, yöneticinin
  vereceği bir karar olduğu halde kırmızı hata rengiyle gösteriliyor.
- Hizmet işlemi durumu, en son işlem olarak daha eski, başarısız bir genel
  Node.js kurulumunu döndürüyor.
- Sertifika onayı, sunucu hangi sertifika kuruluşuna ayarlanmış olursa olsun
  "Let's Encrypt" diyor.

## Kanıt

Şema değişikliği yok: alpha.139'daki gibi şema 079. Yeni olanlar, hepsi
ekleme: panel ayarlarındaki `database_delete_v1:<numara>` girdileri (silinen
her veritabanı için bir tane, onunla birlikte kaldırılır); iki Agent
çağrısı, `DatabaseIdentityV1` (yalnız okur) ve `DropDatabaseOnceV1` (yalnız
kimliği belirlenen veritabanını siler; eski bir Agent'ta ikisi de yoktur,
bu yüzden karışık bir panel ve Agent hiçbir şey silmez); ret kodları
`DATABASE_DELETE_UNVERIFIED` ve `COMPONENT_INSTALLED_BY_VERSION`; yapay zekâ
uygulamalarının olguları `needs`, `found` ve `found_known`.

Testler, her paketin tam takımı test makinesinde, önce (alpha.139) ve sonra:

| Paket | Önce | Sonra |
|---|---|---|
| `internal/core` | 42 | 43 |
| `internal/serviceapps` | 17 | 18 |
| `internal/transport` | 42 | 42 |
| `internal/services` | 244 | 244 |
| `cmd/agent` | 2659 | 2665 |
| `cmd/panel` | 2773 | 2785 |

Önce de sonra da başarısız test yok. Bu sürümden önce başarısız olan yeni
testler: `TestDatabaseDeleteAgainNeverDropsASameNamedDatabaseMadeMeanwhile`
(MariaDB ve PostgreSQL'de kaybolan yanıt, kaldırılamayan kullanıcılar),
`TestDatabaseDeleteWithoutAnIdentityNeverDropsWhatItCannotTellApart`,
`TestDomainDeletionRetryNeverDropsASameNamedDatabaseMadeMeanwhile`,
`TestDatabaseDeleteAgainOnAnotherMachineNeverDropsASameNamedDatabase` ve
`TestDeletionHeldForTheBackupsZoneListSaysItFinishesByItself`. Ayrıca:
`TestDatabaseDeleteAgainAfterADropThatNeverRanDeletesTheSameDatabase`,
`TestDatabaseDeleteRefusedLeavesNoMarker`, Agent'ın `TestDropDatabaseOnce…`
testleri ve `TestDatabaseDirectoryIdentityChangesWhenTheDirectoryIsMadeAgain`
(gerçek bir ext4 dosya sisteminde), `component_installed_by_version_test.go`,
`service_apps_os_test.go`. Web: `npm test` 3054 → 3067 geçti,
0 başarısız (`databases-page-alpha133`, `webmail-dns-add-alpha135`,
`ai-apps-truthful-alpha140`); tür denetimi temiz, derleme her boyut
sınırını karşılıyor (ilk yükleme yolu 109,36 KiB / 110,00 KiB sıkıştırılmış).
