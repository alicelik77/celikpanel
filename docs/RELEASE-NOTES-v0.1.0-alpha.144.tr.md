# v0.1.0-alpha.144

[English](RELEASE-NOTES-v0.1.0-alpha.144.md)

**Küçük bir sürüm.** Web postasının kendi adresindeki bir güvenlik açığını
kapatır, alpha.143 laboratuvarının istediği altı düzeltmeyi ve trafik
rakamlarının bir düzeltmesini getirir ve küçük bir şey ekler: aynı hesabın
bir alan adının içindeki takma ad artık DNS'te yanıt verir. Başka yenilik
yoktur ve panelin veritabanı yapısı değişmez.
**Kurulu panelleri yalnız sahibi, panelin kendi güncelleme ekranından
günceller.**

## alpha.143'ten güncelleme

1. Her zamanki gibi panelin kendi güncelleme ekranından güncelleyin.
2. **Panelin veritabanı yapısı değişmez** (şema 81'de kalır). alpha.143'e
   dönmek olağan geri dönüştür; bir anlık görüntüyü geri yüklemek gerekmez.
3. Aracı güncellemeden sonraki ilk açılışında kendiliğinden, bir kez, iki şey
   yapar: web postası adresi olan bir sunucuda o adresin web sunucusu
   dosyasını yeni biçimde yazar ve web sunucusunu yeniden yükler (aşağıda
   "Güvenlik: kapanan yollar"), ve CelikPanel'in günlük klasörünü web
   sunucusunun çalışan süreçleri için geçilebilir yapar (aşağıda "Gece günlük
   döndürmesinden sonra trafik rakamları"). Basmanız gereken bir şey yok.
   Yapamadığı şey **Şu anki sorunlar**da adımıyla birlikte tek satırdır.

## Güvenlik: kapanan yollar

- **Web postasının kendi adresi sayfalarına ait olmayan dosyaları veriyordu.**
  alpha.110'dan beri web postasının kendi adresi (Roundcube'un ayrı web
  adresi), bir Roundcube eklentisinin klasöründeki PHP olmayan her dosyayı,
  adresi açabilen herkese olduğu gibi veriyordu: örneğin bir eklentinin
  yapılandırmasının bir kopyası (`config.inc.php.bak`, doldurulmuş bir
  `config.inc.php.dist`) ya da bir eklentinin `.sql`, `.log`, `.ini`, `.yml`
  veya `composer.json` dosyası. Ayrıca herhangi bir eklentinin PHP dosyasını
  doğrudan çalıştırıyordu. Roundcube'un kendi program klasörlerine
  ulaşılamıyordu. Artık adres yalnız web postasının kendi giriş noktasını
  (`index.php`) çalıştırır ve yalnız web postası sayfalarının yüklediği durağan
  dosyaları verir (betikler, stil dosyaları, resimler, yazı tipleri, yeni posta
  sesleri, tarayıcı denetiminin PDF ve TIFF dosyaları ve temaların sayfaları);
  geri kalan her şey "bulunamadı" yanıtı verir ve yedek, döküm, günlük ya da
  ayar dosyasına benzeyen bir ad her yerde reddedilir.
- **Var olan sunucular kendiliğinden düzeltilir:** aracının güncellemeden
  sonraki ilk açılışında yeni dosya yazılır, `nginx -t` ile denetlenir ve web
  sunucusu yeniden yüklenir. Denetim ya da yeniden yükleme başarısız olursa
  önceki dosya aynen geri konur; web postası ve bütün siteler hizmet vermeyi
  sürdürür.
- **Elle değiştirdiğiniz bir web postası adresi dosyası korunur;**
  CelikPanel'in metni yanına `<dosya>.celikpanel-pending` olarak konur (web
  sunucusu onu okumaz). **Şu anki sorunlar** bunun üzerine tek satırda web
  postasının adresinin hâlâ eklenti dosyalarını verdiğini, sunucu sahibinin
  işlem yapacağını ve adımı söyler: o metni adlandırır ya da CelikPanel onu
  dosyanızın yanına yazamadıysa dosyanızı kaldırıp Roundcube sayfasında adres
  için **Yeniden kur**'a basmanızı söyler. Web sunucusunun denetimi başka bir
  dosyada başarısız olduğu için yeni metin geri alındığında da aynısını
  yapar ve o dosyayı adlandırır. Aracı her açılışta yeniden dener; dosya yeni
  biçime girince satır kendiliğinden kalkar.
- Panelin kendi `/webmail/` adresi tarayıcıyı yalnız o adrese yönlendirir
  (alpha.110'dan beri) ve değişmez. Yalnız yerel bir soket üzerinden dinleyen
  web postası sunucusuna ağdan ulaşılamaz; önceki kurallarını korur.

## Sizin için ne değişir

### alpha.143 laboratuvarının düzeltmeleri

- **Siz sayfadan ayrılırken oluşturulan posta kutusu** eksiksiz oluşturulur:
  sayfadan ayrılsanız ya da bağlantı kesilse bile panel oluşturmayı ve
  kaydını bitirir; aynı basış iki kez ulaşırsa kutu bir kez oluşturulur. Bir
  otomatik yanıtı kaydetmek için de aynısı geçerlidir.
- **Sunucuda kaydı olmadan bulunan posta kutusu.** Sunucunun panelde kaydı
  olmadan zaten tuttuğu bir adla posta kutusu oluşturmak artık iç hatayla
  yanıtlanmaz ve hiçbir şey değişmez. Bir müşteri ya da bayi, posta kutusunun
  sunucuda var olduğunu okur ve yöneticiye başvurması ya da başka bir ad
  seçmesi istenir. Yöneticiye **Bu alan adına al** sunulur: yalnız içinde
  posta olmayan ve CelikPanel'in yazdığı biçimde olan bir kutu için çalışır ve
  her zaman alma penceresine yazılan yeni parolayı koyar; böylece önceki
  hiçbir parola çalışmaz. İçinde posta olan ya da klasörü denetlenemeyen bir
  kutu alınmaz: ne olacağına sunucunun sahibi sunucuda karar verir. Alınan
  kutu bundan sonra sıradan bir kutudur: sunucuda bulunan boyut sınırını
  korur, alan adının imza anahtarı ve ilk gün gönderim sınırı ona da
  uygulanır.
- **Sayfası bırakılan bir PostgreSQL kapatması** artık "gönderildi, henüz
  doğrulanmadı" diye kaydedilmez: kapatma ve kaydı biter, sayfaya
  dönüldüğünde kapatma gerçek sonucunu gösterir. Geri al ve Yeniden oku için
  de aynısı geçerlidir.
- **Başarısız bir kapatmanın penceresi** "Sunucuda çalışılıyor…" adımında
  kırmızı bir işaretle değil, "Yapılmadı — hiçbir şey değiştirilmedi" adımında
  (sonuç bilinmediği sürece "Sonuç henüz doğrulanmadı — yeniden gönderilmez")
  biter. Nedeni önce sayfanın dilinde nedenin türünü söyler (veritabanı
  kullanımda, süre doldu, bağlantı kurulamadı, bir hak eksik, bir rol ya da
  veritabanı yok, veritabanı sunucusu reddetti), ardından veritabanı
  sunucusunun kendi iletisini tırnak içinde verir.
- **Retler sayfanın dilinde.** Türkçe sayfada İngilizce çıkan takma ad ve
  sertifika retleri sayfanın dilinde söylenir: takma ad için yeniden
  verilemeyen ya da onu kapsamayan bir sertifika, geçersiz bir takma ad,
  kullanımda olan bir ad, bulunamayan bir takma ad, HSTS'nin engellediği bir
  kaldırma, kaldırılacak etkin sertifikanın olmaması. Yanıtların kendisi
  değişmez.
- **"1 PostgreSQL veritabanı":** tek veritabanı **Şu anki sorunlar**da ve
  uyarı postasında tekil söylenir (Türkçe satırda zamirler de buna uyar).
- **Gece günlük döndürmesinden sonra trafik rakamları.** Trafik rakamları,
  her gecenin günlük döndürmesinden sonra web sunucusu bir dahaki sefer
  yeniden yüklenene dek eksik sayılıyordu: web sunucusunun çalışan süreçleri
  CelikPanel'in trafik günlüklerini yeniden açamıyordu (her gece günlük başına
  bir "Permission denied" hata satırı) ve döndürülen dosyaya yazmayı
  sürdürüyordu; o dosyayı bir sonraki gecenin döndürmesi siliyordu.
  Güncellemeden sonraki ilk açılışta CelikPanel'in günlük klasörü
  `/var/log/celikpanel` geçilebilir olur (diğer hesaplar içinden geçebilir,
  listeleyemez) ve `traffic` klasörüne yalnız web sunucusunun çalışan
  süreçleri geçebilir (0710; sahibi root, grubu web sunucusunun grubu):
  sitelerin programları dahil başka hiçbir hesap başka bir sitenin trafik
  günlüğünü açamaz. Diğer hesapların okuyabildiği bir trafik günlüğü (ilkini
  web sunucusu herkese okunur yapıyordu) onlara kapatılır; bir sitenin ilk
  günlüğü artık web sunucusu açmadan önce kapalı olarak oluşturulur. Kendi
  koyduğunuz bir klasör kipi ya da yaptığınız bir bağlantı korunur ve
  aracının günlüğünde adlandırılır; sertifika aracının günlük klasörü kapalı
  kalır. Yeniden yükleme gerekmez: güncellemeden sonraki ilk geceden
  itibaren böyle bir hata satırı yazılmaz ve trafik, bir yeniden yükleme
  beklemeden yeniden eksiksiz sayılır. Yeni kurulumlar klasörleri ve
  günlükleri baştan böyle oluşturur.

### Yeni, küçük: takma ad DNS'te yanıt verir

- Aynı hesabın, DNS'ini bu sunucunun tuttuğu bir alan adının içindeki takma
  ad artık o alan adının bölgesinde sitenin adres kayıtlarını alır; kayıtlar
  bölgeyle birlikte yayımlanır (yedek DNS sunucusu onları bölgenin olağan
  aktarımıyla alır). Önceki hiçbir sürüm bunu yapmıyordu: takma ad yalnız
  sitenin web sunucusu dosyasında ve sertifikasında bir addı. Takma ad için
  sertifika yeniden verilirken kayıt, sertifika yetkilisi adı denetlemeden
  önce yayımlanır ve takma ad eklenmezse geri çekilir.
- Kayıt takma adla birlikte gider; takma adın alan adı silindiğinde de. Yalnız
  CelikPanel'in eklediği kaldırılır.
- Yayın bitemezse (yedek DNS sunucusu bekliyorsa, DNS yayımlayıcısı hazır
  değilse) takma ad satırı "DNS kaydı yayımlanmayı bekliyor — her saat yeniden
  denenir, bir şey yapmanız gerekmez" der ve panelin saatlik turu onu yayımlar.
  Bir alan adının silinmesini asla durdurmaz.
- O adda sizin oluşturduğunuz bir adres kaydı asla değiştirilmez ya da
  kaldırılmaz ve yanına bir şey eklenmez: takma ad "korundu" yanıtını verir ve
  sayfa, takma ad bu siteye ulaşacaksa adı DNS kayıtlarından bu sunucuya
  yönlendirmenizi söyler.
- Yapmadığı şey: kendi başına bir alan adı olan takma ad (burada tutulan aynı
  hesabın bir alan adının içinde olmayan) hâlâ bölge almaz; bu sürümden önce
  oluşturulan bir takma ad kaydını yalnız kaldırılıp yeniden eklendiğinde
  alır.

## Bilinen sınırlar

- Başarısız bir PostgreSQL kapatmasının neden türü, veritabanı sunucusunun
  İngilizce iletisinden tanınır. PostgreSQL iletileri başka bir dilde olan
  bir sunucuda her başarısızlık, tırnak içindeki iletisiyle birlikte,
  "veritabanı sunucusu reddetti" diye okunur.
- Başka değişiklikler hâlâ tarayıcının isteğiyle çalışır: sunucu çalışırken
  sayfadan ayrılırsanız böyle bir değişiklik sunucuda panelin kaydı olmadan
  bitebilir ya da yalnız yanıtı kaybolur. Bunlar arasında zamanlanmış
  görevler, posta yönlendiricileri ve her şeyi yakalayan adres, bir posta
  kutusunun parolası, boyut sınırı ve silinmesi, DKIM, bir sitenin SSL ve PHP
  ayarları, dosya erişimi, DNS kurulumu ve DNSSEC, VPN, bir yedeği silmek, bir
  katalog uygulamasını kurmak ya da kaldırmak ve veritabanı içe aktarma
  vardır; tam liste sonraki bir sürüm için bu sürümün çekme isteğindedir
  (#355).
- Kendi başına bir alan adı olan takma ad bölge almaz (olması gerekip
  gerekmediğine sahip karar verir) ve bu sürümden önce oluşturulan takma
  adlar kaydını yalnız yeniden eklendiğinde alır (yukarıda).
- Güncellemeden sonraki ilk gece döndürmesine dek web sunucusunun çalışan
  süreçleri önceden yazdıkları yere yazar; sayım o geceden itibaren
  eksiksizdir.
- Yalnız yerel bir soket üzerinden dinleyen web postası sunucusu önceki
  kurallarını korur; ona ağdan ulaşılamaz.
- alpha.143'ün notlarının hâlâ açık diye listelediği her şey açık kalır
  ([alpha.143 sürüm notları](RELEASE-NOTES-v0.1.0-alpha.143.tr.md)).

## Laboratuvarda denetlenenler

**Açık.** Bu sürüm henüz laboratuvarda denenmedi. Planlanan deneme, test
sunucusu çiftinde (PowerDNS'li bir Debian 13 ana DNS sunucusu ve BIND'li bir
Ubuntu 24.04 yedek DNS sunucusu), alpha.143 durumundan panelin kendi
güncelleme sayfasıyla güncellenerek:

1. Web postası: güncellemeden önce web postası adresi dosyasının bir kopyası;
   sonra aracının günlüğü dosyanın yeniden yazıldığını söyler; oturum açma,
   bir kutuyu listeleme, ekli bir iletiyi açma (bir PDF açılır, bir metin
   dosyası indirilir), düzenleyiciyle yazma, konsolda hata yok; yerleştirilmiş
   `plugins/archive/config.inc.php.bak` ve `plugins/archive/archive.php` 404,
   `program/js/app.min.js`, `program/resources/blank.tiff` ve
   `skins/elastic/watermark.html` 200 yanıtı verir, `index.php/x.txt`
   Roundcube'un sayfasını verir. Elle değiştirilen bir adres dosyası yanında
   `.celikpanel-pending` ile korunur ve **Şu anki sorunlar**da adlandırılır;
   başka bir web sunucusu dosyası bozukken web postası ve bütün siteler yanıt
   vermeyi sürdürür, adres dosyası değişmez ve **Şu anki sorunlar** bozuk
   dosyayı adlandırır.
2. Trafik günlükleri: güncellemeden sonraki klasör ve günlük kipleri,
   "Permission denied" satırı olmadan zorla bir döndürme, yeni günlüğe düşen
   istekler, aracının günlüğündeki bloğun yalnız bir kez yazılması ve bir
   sitenin hesabıyla başka bir sitenin trafik günlüğünü okumanın reddedilmesi.
3. DNS'te takma ad: iki sunucudan yanıt, kaldırmadan sonra yok; sertifikayla;
   reddedilen bir sertifika kayıt bırakmaz; sahibin kaydının yanında hiçbir
   şey yazılmaz ya da eklenmez; yedek sunucuya ulaşılamazken silinen bir alan
   adı silinmeyi tamamlar ve saatlik tur takma adın bölgesini yayımlar.
4. Sayfası bırakılan bir PostgreSQL kapatması ve başarısız bir kapatmanın
   penceresi ve nedeni iki dilde; PostgreSQL iletileri İngilizce olmayan bir
   sunucu.
5. Türkçe sayfadaki takma ad retleri (geçersiz ve yinelenen takma ad,
   sertifika adımı, HSTS) ve tekil satır.
6. Sayfası bırakılarak kaydedilen bir posta kutusu ve otomatik yanıt;
   sunucuda bulunan bir posta kutusu: müşterinin cümlesi, yöneticinin yeni
   parolayla alması (önceki parola artık oturum açmaz), içinde posta olan bir
   kutunun reddi, başka bir hesabın ya da başka bir alan adının adresinin
   reddi.

## Güncellemeden sonra ne beklenir

- Şema adımı yok: yapı şema 81'de kalır ve alpha.143'e dönmek olağan geri
  dönüştür.
- Web postası adresi olan bir sunucuda web sunucusu ilk açılışta,
  yapılandırma denetimi geçtikten sonra bir kez yeniden yüklenir; bundan
  sonra web postasının adresi yalnız sayfalarının yüklediğini verir.
- Günlük klasörlerinin kipleri ilk açılışta bir kez değişir:
  `/var/log/celikpanel` 0700'den 0711'e ya da 0750'den 0751'e; `traffic`
  klasörü 0755'ten 0710'a, sahibi root ve grubu web sunucusunun grubu
  (Debian ve Ubuntu'da `www-data`, Arch Linux'ta `http`); diğer hesapların
  okuyabildiği trafik günlükleri 0644'ten 0640'a. Kendi koyduğunuz bir kip ya
  da yaptığınız bir bağlantı korunur. Güncellemeden sonraki ilk geceden
  itibaren trafik rakamları eksiksizdir.
- Sunucunun panel kaydı olmadan tuttuğu bir posta kutusu açıkça söylenir;
  onu yalnız yönetici, yalnız içinde posta yokken ve yeni bir parolayla
  alabilir.
- alpha.143'e döndükten sonra: alpha.143 yeni web postası adresi dosyasını
  tanımaz ve onu elle değiştirmişsiniz gibi korur; böylece web postası hizmet
  vermeyi sürdürür ve hiçbir şey yeniden açılmaz; web postası adresi
  değişiklikleri ise yeniden güncelleyene dek elle düzenlenmiş diye reddedilir.
  alpha.144'ün eklediği bir takma ad kaydı, takma ad alpha.143 altında
  kaldırıldığında kalır; onu DNS kayıtlarından kaldırın.

## Kanıt

Şema değişikliği yok: `internal/db/migrations/` altında hiçbir dosya
değişmez; yapı 81'de kalır. Yeni, hepsi ekleme: derlemeye bağlı, salt-okur
aracı çağrısı `MailboxOnServerV1`; salt-okur aracı çağrısı
`WebmailAddressFormsV1`; Şu anki sorunlar türleri `webmail_address_kept` ve
`webmail_address_refused`; ret kodları `MAIL_ACCOUNT_ON_SERVER_ONLY`,
`MAIL_ACCOUNT_ADOPT_REFUSED`, `MAIL_ACCOUNT_ADOPT_RECORD_NOT_WRITTEN`,
`ALIAS_INVALID`, `ALIAS_CONFLICT`,
`ALIAS_NOT_FOUND`, `ALIAS_CERTIFICATE_REISSUE_FAILED`,
`ALIAS_CERTIFICATE_NOT_COVERING`, `HSTS_REMOVAL_BLOCKED` ve
`SSL_NO_ACTIVE_CERTIFICATE` (durum kodları değişmedi); panel ayar satırları
`alias_dns_v1:<takma ad>`; denetim kaydı eylemi `mail.account.adopt`; aracının
bir kezlik makbuzu `log-root-modes-v2.done`. Dört yol, istek kimliği
korumasının işlem adımlarına katılır (PostgreSQL kapatma, geri alma ve
Yeniden oku; posta kutusu oluşturma; otomatik yanıt kaydı; alma); saklanan
özetleri asla parola tutmaz.

Maddeye göre testler:

- Web postasının adresi: `TestWebmailAddressServesOnlyRoundcubesStaticFilesAndEntryPoint`
  (oluşturulan dosyayı nginx'in konum seçtiği gibi okur: 21 ad reddedilir;
  bir betik, bir stil dosyası, bir resim, bir yazı tipi, bir temanın boş
  sayfası, bir ses ve `index.php` verilir), `TestWebmailAddressEarlierFormServedPluginFiles`
  (önceki biçim `config.inc.php.bak` dosyasını veriyor ve bir eklentinin
  PHP'sini çalıştırıyordu),
  `TestWebmailAddressEarlierFormIsRewrittenAtStartAndAnOwnersFileKept`,
  `TestWebmailAddressFormsNameWhatStillServesPluginFiles`,
  `TestA144WebmailAddressInItsEarlierFormIsAProblemUntilItIsNot`; dağıtım
  yapılandırma matrisi yeni dosyayı oluşturur (`TestRenderDistroConfigMatrix`,
  CI'da nginx ile denetlenir).
- DNS'te takma ad: `TestA144AliasInsideItsAccountsZoneAnswersAndGoesAgain`,
  `TestA144AliasNeverTakesTheOwnersRecords`,
  `TestA144AliasRecordsSurviveAFailedPublicationAndArePublishedLater`,
  `TestA144AliasAddsNothingBesideAnOwnersAddressRecord`,
  `TestA144AliasPublicationThatWaitsIsRetriedAndNeverStopsADeletion`,
  `TestA144DeletingADomainWithExternalDNSRemovesItsAliasRecordsHere`.
- Sayfası bırakılan kapatma: `TestA144PGPublicCloseIsNotLostWhenThePageIsLeft`
  (yol korunmadığında laboratuvarın kaydını yeniden üretir).
- Başarısız kapatma: `TestA144PGFailureKindNamesTheCause`,
  `TestA144FailedCloseAnswersTheKindOfCause`.
- Sayfanın dilinde retler: `TestA144AliasRefusalsAreCoded`,
  `TestA144AliasHandlersRefuseAnInvalidNameCoded`.
- Tekil satır: `TestA144OnePostgreSQLDatabaseIsSaidInTheSingular` ve web
  takımının sorun anahtarı testleri.
- Sayfası bırakılan posta kutusu ve sunucuda bulunan posta kutusu:
  `TestA144MailboxCreatedWhileThePageIsLeftHasItsRow`,
  `TestA144AutoresponderSavedWhileThePageIsLeftHasItsCopy` (ikisi de koruma
  kaldırılınca başarısız olur), `TestA144MailboxOnTheServerWithoutARowIsSaidPlainly`,
  `TestA144AdministratorAdoptsAMailboxFoundOnTheServer` (klasörde posta
  varken ve klasör sayılamazken reddedilir; parola, parola adımıyla bir kez
  değiştirilir), `TestMailboxOnServerReadsOnlyCelikPanelsOwnForm`,
  `TestMailboxMessageCountReadsCurAndNewOnly`.
- Trafik günlükleri: `TestLogRootIsMadePassableWhateverTheUmask`,
  `TestASitesFirstTrafficLogIsMadeClosed`,
  `TestALogFolderThatIsTheOwnersLinkIsUsedAsBefore`,
  `TestLogRootBecomesPassableOnceAndKeepsTheOwnersModes`,
  `TestLogRootKeepsTheOwnersLinks`, `TestCertbotStorageGoesOnThroughTheOwnersLogLink`,
  `TestTrafficLogsReachOnlyTheWebServerWorkers` (web sunucusunun hesabı
  günlüğüne döndürmeden önce ve sonra ulaşır; ne root ne de web sunucusunun
  grubunda olan bir hesap hiçbir trafik günlüğünü adıyla açamaz ve hiçbir
  şeyi listeleyemez); düzeltmeden önceki kodda başarısız olurlar.

Go modülünün ve web'in tam test takımları düzeltme şeridinde temel
(alpha.143'ün kodu) ve düzeltmeler üzerinde çalıştı; sürüm çekme isteğinin
her denetimi CI'da geçti. Düzeltmelerin bağımsız, salt-okur bir incelemesi
sürümü engelleyen bir şey bulmadı; bulguları bu sürümde düzeltildi, yukarıda
bilinen sınır olarak bırakılan biri dışında (neden türünün veritabanı
sunucusunun İngilizce iletisinden okunması).
