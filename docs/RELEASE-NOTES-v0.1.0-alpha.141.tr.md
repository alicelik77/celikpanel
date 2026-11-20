# v0.1.0-alpha.141

[English](RELEASE-NOTES-v0.1.0-alpha.141.md)

**Bir güvenlik sürümü: lütfen yakında güncelleyin.** Bir müşterinin
veritabanı sunucusunun yönetici haklarına, onlarla da diğer bütün
müşterilerin verisine ve sunucunun kendisine ulaşabildiği yolları kapatır.
Ayrıca panelin oluşturduğu her PostgreSQL veritabanını UTF-8 yapar ve başka
hesaplara kapatır, Open WebUI'ı da katalogdan çıkarır.
**Kurulu panelleri yalnız sahibi, panelin kendi güncelleme ekranından
günceller.**

## alpha.140'tan güncelleme

1. Her zamanki gibi panelin kendi güncelleme ekranından güncelleyin. Bu
   sürüm panelin veritabanı düzenini değiştirmez (alpha.140'taki biçimde
   kalır); bu yüzden sonradan alpha.140'a dönmek için veritabanı geri
   yüklemesi gerekmez.
2. Güncellemeden sonra basılacak ya da ayarlanacak bir şey yok.
3. **alpha.140'a dönmek kapanan yolları yeniden açar.** Geri yüklemeler ve
   kopyalar yine veritabanı sunucusunun yönetici haklarıyla çalışır. Bu
   sürümün oluşturduğu veritabanları UTF-8 ve başka hesaplara kapalı kalır;
   alpha.140 onları değiştirmez.

## Güvenlik: kapanan yollar

- **Bir yedekten, kopyadan ya da içe aktarmadan gelen SQL artık veritabanının
  kendi sahibinin haklarıyla, kabuğa da sunucunun dosyalarına da ulaşamayan
  bir istemciyle çalışır.** Önceden veritabanı geri yükleme, veritabanı, site
  ve WordPress kopyaları ve cPanel içe aktarması dosyayı veritabanı
  sunucusunun yönetici hesabıyla çalıştırıyordu. Bu yüzden bir müşterinin
  kendi uzak hedefinden geri getirilen bir yedek (müşteri onu
  değiştirebilir) ya da yöneticiye verilen bir cPanel arşivi, yöneticinin
  haklarıyla komut çalıştırabiliyordu. Artık böyle bir dosya, veritabanının
  kendi kullanıcısının kendi veritabanında yapabildiğinden fazlasını
  yapamaz.
- **Yöneticinin bir müşterinin veritabanındaki PostgreSQL oturumları o
  müşterinin işlevlerini artık çalıştırmaz.** Panelin bir veritabanının
  içindeki kendi denetimleri ve değişiklikleri (boyut, denetleme, kullanıcı
  ekleme) ve yöneticinin PostgreSQL'deki bütün diğer oturumları adları
  artık yalnız PostgreSQL'in kendi kataloğunda arar, veritabanı sahibinin
  bir şey koyabileceği yerde asla.
- **Yeni bir PostgreSQL veritabanı artık başka hesapların kullanıcılarının
  bağlanmasını kabul etmez.** Şimdiye dek bir veritabanı PostgreSQL'in
  varsayılanıyla kalıyordu: sunucudaki her veritabanı kullanıcısı ona
  oturum açıp yapısını (verisini değil) okuyabiliyordu. Panelin oluşturduğu
  bir veritabanı artık yalnız sahibini ve kartından eklenen kullanıcıları
  içeri alır.

  **Bu sürümden önce açılan veritabanları bu bakımdan hâlâ açık.** Sonraki
  bir sürüm, her birinin neyi yitireceğini önce sahibe göstererek onları
  kapatır. O zamana dek sunucu sahibi birini elle, sunucuda root olarak
  kapatabilir (`ad`, veritabanının adı):

  ```sh
  sudo -u postgres psql -c 'REVOKE ALL ON DATABASE "ad" FROM PUBLIC;'
  ```

  Veritabanının kendi kullanıcısı ve kartından eklenen kullanıcılar
  erişimlerini korur (PostgreSQL 17'de denendi); ona başka bir veritabanı
  kullanıcısıyla oturum açan her şey (elle yapılmış bir araç ya da rol) bundan
  sonra reddedilir. Geri almak için:
  `sudo -u postgres psql -c 'GRANT CONNECT, TEMPORARY ON DATABASE "ad" TO PUBLIC;'`

## Sizin için ne değişiyor

### Geri yükleme, kopya ya da içe aktarma veritabanı kullanıcısının kayıtlı parolasını ister

Dosya, panelin o kullanıcı için sakladığı parolayla veritabanının kendi
kullanıcısı olarak yüklenir (içe aktarma zaten böyle çalışıyordu). Geri
yükleme bunu kullanamazsa **hiçbir şey yazılmadan önce** (güvenlik
kopyasından da önce) durur ve nedenini, kimin ne yapacağını söyler:

- **Veritabanı kullanıcısının burada kayıtlı parolası yok ya da veritabanı
  sunucusu onu kabul etmedi** (büyük olasılıkla panelin dışında
  değiştirildi): alan adının veritabanlarını yöneten kişi Veritabanları
  sayfasında kullanıcıya yeni bir parola verir, sonra yeniden geri yükler.
- **Bu sunucudaki MariaDB istemcisi bir dosyayı güvenle yükleyemeyecek kadar
  eski** (korumalı kipi yok; MariaDB 10.5.25, 10.6.18, 10.11.8 ve sonrası
  sahip): sunucu yöneticisi sunucunun MariaDB paketlerini günceller, sonra
  geri yüklemeye yeniden basılır.
- **Yedekte burada yalnız veritabanı sunucusunun yöneticisinin
  oluşturabileceği bir şey var:** MariaDB'de ikili günlük açık ve
  `log_bin_trust_function_creators` kapalıyken saklı işlevler ya da
  tetikleyiciler içeren bir yedek; PostgreSQL'de kendisi yönetici rolü olan
  bir veritabanı kullanıcısı. Kararı sunucu yöneticisi verir: veritabanı
  sunucusunda buna izin verir ya da o yedeği elle geri yükler.

Uzak bir geri yükleme için de aynı kurallar geçerli; sayfası da aynısını
söyler.

### Sahibin haklarıyla geri yüklemenin geri getiremedikleri

Panelin kendi yedekleme koduyla alınmış yedeklerle MariaDB 11.8 ve
PostgreSQL 17'de denendi:

- Olağan her şey geri gelir: tablolar ve satırlar, görünümler,
  tetikleyiciler, saklı yordamlar, diziler; PostgreSQL'de ayrıca şemalar,
  açıklamalar, büyük nesneler ve sahibin oluşturabildiği eklentiler.
- MariaDB'de veritabanının başka bir kullanıcısının yaptığı görünüm,
  tetikleyici ya da yordam, veritabanının kendi kullanıcısınınki olarak geri
  gelir.
- PostgreSQL'de yalnız yöneticinin oluşturabildiği bir eklenti, ancak
  veritabanında zaten varsa geri gelir: böyle bir eklenti isteyen yeni bir
  veritabanına kopya durur, yeni veritabanını boş bırakır ve eklentinin
  adını söyler.
- PostgreSQL'de bir veritabanına eklenen kullanıcılar, geri yüklemenin
  yeniden kurduğu nesnelerdeki haklarını yitirir (önceden de böyleydi; bkz.
  "Bilinen sınırlar").

### Panelsiz geri yükleme

Yedek paketinin biçimi değişmiyor. CelikPanel'siz geri yükleme adımları
(uzak yedekler rehberinde) artık her veritabanı dökümünü veritabanının kendi
kullanıcısı olarak yükler: MariaDB için `mariadb --sandbox`, PostgreSQL için
önce istemci komutlarına bakılır, sonra veritabanının kullanıcısıyla `psql`.

### PostgreSQL veritabanları UTF-8

- **Panelin oluşturduğu her PostgreSQL veritabanı UTF-8'dir**; sunucunun
  PostgreSQL şablonları ne olursa olsun, dilden bağımsız bir sıralama
  düzeniyle. C yerel ayarıyla kurulmuş bir sunucu (sahibin Ubuntu 24.04
  sunucusu) önceden SQL_ASCII veritabanları alıyordu; bunlar İngilizce
  dışındaki harfleri denetlenmemiş bayt olarak saklar.
- **Panelin kurduğu bir PostgreSQL sunucusu UTF-8'dir.** Var olan PostgreSQL
  sunucularına ve sahibin yenileri için kendi ayarlarına asla dokunulmaz.
- **Veritabanı kartı her veritabanının metin kodlamasını söyler**:
  oluşturulma tarihinden sonra "Metin: UTF-8". UTF-8 olmayan bir veritabanı
  kendi kartında tek bir uyarı ve çıkış yolunu gösterir: boşsa silip yeniden
  oluşturun; veri varsa **Kopyala**'ya basın: kopya bütün verisiyle ve yeni
  bir adla UTF-8 olur. Siteyi kopyaya çevirin, deneyin, sonra eskisini
  silin.
- **Kopyala, içe aktarma ve geri yükleme UTF-8 bir veritabanında her harfi
  denetler.** Önceden bir SQL_ASCII veritabanının dökümü UTF-8 bir
  veritabanına denetlenmemiş baytlar koyabiliyordu. Artık böyle bir metin
  kopyayı ya da içe aktarmayı veritabanı sunucusunun gerekçesiyle durdurur
  ve hiçbir şey değişmez. Bir SQL_ASCII veritabanının kendi yedeği ona yine
  olduğu gibi geri yüklenir.

### Open WebUI geri çekildi

Open WebUI artık sunulmuyor ve kurulmuyor (sahip, 20 Kasım 2026):
laboratuvarda kurulurken yaklaşık 15 GB boş disk ve yaklaşık 2 GB bellek
istedi, yarıda durdurulamadı ve Debian 13'te çalışamıyor. Bir alan adının
zaten tuttuğu Open WebUI çalışmayı sürdürür; kartı geri çekildiğini söyler,
durdurma, başlatma, günlük ve kaldırma sunar, güncelleme sunmaz. n8n ve
diğer uygulamalar değişmedi.

## Bilinen sınırlar

- **Bir veritabanını başka bir PostgreSQL sürümüne taşımak** (yönetici
  işlemi), taşıma sırasında içindeki müşteri işlevlerini hâlâ yöneticinin
  haklarıyla çalıştırır. Sonraki bir sürüm bunu düzeltene dek sahibine
  güvenmediğiniz bir veritabanını taşımayın.
- Bu sürümden önce açılan PostgreSQL veritabanları, sonraki bir sürüm
  kapatana dek başka hesapların oturum açmasına açık kalır (elle: yukarıya
  bakın).
- PostgreSQL'de bir veritabanına eklenen kullanıcılar, geri yüklemenin
  yeniden kurduğu nesnelerdeki haklarını yitirir; geri yüklemeden sonra
  onları karttan yeniden ekleyin.
- MariaDB olayları (bir veritabanının içindeki zamanlanmış işler) yedeklerde
  yok.
- **Kopyala** ile UTF-8'e taşınan bir veritabanı yeni bir ad alır; sitenin
  ayarları ona çevrilmelidir.
- Yeni bir veritabanı için sıralama dili seçmek ve bir veritabanını yerinde
  UTF-8'e dönüştürmek sonraki bir sürümde gelir.
- Uzak bir hedeften geri getirilen paket bu panelin yaptığı bir paket olarak
  işaretlenmiyor; her durumda yalnız kendi sitesinin haklarıyla iş görebilir.
- Bir cPanel içe aktarmasının görünümleri ve tetikleyicileri, önceden olduğu
  gibi buraya taşınmayan bir veritabanı hesabını adlandırır.

## Laboratuvarda denenenler

**AÇIK: laboratuvar denetimi henüz yapılmadı.** Kapsamı iki laboratuvar
sunucusu (Debian 13 ve Ubuntu 24.04) ve iki veritabanı sunucusu: müşterinin
kendi uzak hedefinde değiştirilmiş bir paketin geri yüklenmesi (reddedilir,
veritabanının dışında hiçbir şey değişmez); sahibinin kendi işlevlerini
koyduğu bir veritabanında yöneticinin veritabanı araçları ve kullanıcı
ekleme; olağan bir yedek ve geri yükleme, veritabanı kopyası, site kopyası,
WordPress kopyası ve cPanel içe aktarması ve bu sürümden önce alınmış bir
yedek; üç ret; C yerel ayarıyla kurulmuş bir sunucuda UTF-8 oluşturma ve
kartın çıkış yolu; yeni bir veritabanında başka bir hesabın kullanıcısının
reddedilmesi; Open WebUI'ın geri çekilmesi, var olan kurulumun çalışmayı
sürdürmesi ve kaldırılabilmesi.

## Kanıt

Şema değişikliği yok: alpha.140'taki gibi şema 079. Yeni olanların hepsi
eklemedir: geri yükleme isteğinin veritabanı hesapları, bir kopyanın yeni
veritabanı hesabı, cPanel içe aktarmasının sitesi; ret kodları
`BACKUP_RESTORE_DATABASE_ACCOUNT`, `BACKUP_RESTORE_CLIENT_UNSAFE`,
`BACKUP_RESTORE_NEEDS_ADMINISTRATOR` ve uzak `DATABASE_ACCOUNT`,
`DATABASE_CLIENT_UNSAFE`, `DATABASE_NEEDS_ADMINISTRATOR`; salt-okur Agent
çağrısı `PostgreSQLDatabaseEncodingsV1`; katalogdaki `withdrawn` işareti ve
ret nedeni.

Testler, her paketin tam takımı deneme makinesinde, önce (alpha.140) ve
sonra:

| Paket | Önce | Sonra |
|---|---|---|
| `internal/backupspec` | 4 | 4 |
| `internal/transport` | 42 | 42 |
| `internal/services` | 244 | 252 |
| `internal/serviceapps` | 18 | 18 |
| `cmd/agent` | 2656 | 2680 |
| `cmd/panel` | 2785 | 2789 |

Yeni hata yok. Gerçek veritabanı sunucularında (özel bir MariaDB 11.8.6 ve
PostgreSQL 17.12; bir kümesi UTF-8, biri C yerel ayarıyla kurulmuş),
önceki ve yeni kodda çalıştırılan testler:
`TestS1MariaDBRestoreOfAnEditedPackageStaysInsideTheDatabase`,
`TestS1PostgreSQLRestoreOfAnEditedPackageStaysInsideTheDatabase`,
`TestS1CPanelImportDumpStaysInsideTheNewDatabase` ve
`TestS1PostgreSQLAdministratorSessionsDoNotRunTheOwnersFunctions` önceki
kodda düşer (dosyanın komutu root olarak çalıştı, yönetici hesabı
kaydedildi, kullanıcıya her hak verildi) ve yeni kodda geçer;
`TestS1MariaDBPanelBackupRestoresWithTheOwnersRights`,
`TestS1PostgreSQLPanelBackupRestoresWithTheOwnersRights`,
`TestPGDatabaseIsUTF8OnAClusterWithSQLASCIITemplatesOnRealPostgreSQL`,
`TestANewDatabaseIsClosedToOtherCustomersOnRealPostgreSQL` ve
`TestSQLASCIIDatabaseMovesToUTF8ThroughExportAndImportOnRealPostgreSQL`
sürümün kodunda geçer. Ayrıca `TestEveryPostgreSQLAdministratorSessionHasTheFixedSearchPath`,
`service_apps_withdrawn_test.go`. Web: `npm test` 3067 → 3079 geçti, 0 hata; tür denetimi temiz ve derleme
her boyut bütçesini karşılıyor (ilk yükleme yolu 110,00 KiB sıkıştırılmışın
109,36 KiB'ı, alpha.140'taki gibi).
