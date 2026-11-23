# v0.1.0-alpha.145

[English](RELEASE-NOTES-v0.1.0-alpha.145.md)

**Küçük bir sürüm.** Web postasının yerel kapısındaki bir güvenlik açığını
kapatır, web postasının posta gönderebilmesini sağlar (daha önce hiç
gönderemiyordu) ve alpha.144 laboratuvarının istediği iki küçük görünüm
düzeltmesini getirir. Başka yeni bir şey yoktur; panelin veritabanı düzeni
değişmez.
**Kurulu panelleri yalnız sahip, panelin kendi güncelleme ekranından
günceller.**

## alpha.144'ten güncelleme

1. Panelin kendi güncelleme ekranından, önceki gibi güncelleyin.
2. **Panelin veritabanı düzeni değişmez** (şema 81 kalır). alpha.144'e geri
   dönmek olağan geri almadır; hiçbir anlık görüntünün geri yüklenmesi
   gerekmez.
3. Güncellemeden sonraki ilk açılışında Agent kendiliğinden, bir kez iki şey
   yapar: web postasının yerel kapısının web sunucusu dosyasını yeni
   biçimiyle yazar ve web sunucusunu yeniden yükler ("Güvenlik: kapatılan"
   bölümüne bakın); web postasının kendi yapılandırmasını gönderebilecek
   biçimde yeniden yazar ("Web postası gönderir" bölümüne bakın). Basılacak
   bir şey yoktur. Yapamadığı şey **Şu anki sorunlar**'da adımıyla birlikte
   tek satırdır.

## Güvenlik: kapatılan

- **Web postasının yerel kapısı sunucudaki her hesaba yanıt veriyordu.**
  Panel web postasını yerel bir kapıdan denetler: sunucunun kendisindeki bir
  soket (`/run/celikpanel-webmail.sock`). Web sunucusu bu soketi sunucudaki
  her hesaba açar; bu yüzden oradaki her hesap — bir müşterinin site kodu ya
  da zamanlanmış görevi — onu oturum açmadan kullanabiliyordu. Onun üzerinden
  sunucudaki herkes web postasının herkese açık klasöründeki her dosyayı
  olduğu gibi okuyabiliyordu; Roundcube'un eklenti ve tema klasörleri de
  dahil: eklenti ayarlarının artık kopyaları (örneğin bir eklentinin
  `config.inc.php.bak` dosyası), nokta dosyaları, `composer.json`, `.dist`,
  `.inc` ve SQL dosyaları; eklenti betiklerini de doğrudan
  çalıştırabiliyordu. Ağdan **erişilemiyordu**; web postasının kendi
  yapılandırması, veritabanı ve posta kutularının parolaları bu yoldan hiçbir
  zaman erişilebilir değildi. Bu durum alpha.13'ten beri vardı (alpha.1'den
  beri aynı ağaç sunucunun iç adresi 127.0.0.1 üzerinde açıktı).
- **Kapı artık yalnız CelikPanel'in kendi denetimlerinin sorduğunu yanıtlar:**
  web postasının oturum açma sayfası (`/webmail/`, Roundcube'un giriş
  noktası, orada çalışan tek PHP), stil dosyaları ve CelikPanel'in PHP
  denetimi. Gerisi "bulunamadı" yanıtı alır. Soketin kendisi taşınmaz, kipi
  değişmez.
- **Kurulu sunucular kendiliğinden düzeltilir:** güncellemeden sonraki ilk
  açılışta yeni dosya yazılır, `nginx -t` ile denetlenir ve web sunucusu
  yeniden yüklenir. Denetim ya da yeniden yükleme başarısız olursa önceki
  dosya tam olarak geri konur, Agent açılmayı sürdürür ve **Şu anki
  sorunlar** nedenini ve adımı söyler (denetimin gösterdiğini ya da Agent
  günlüğündeki nedeni düzeltin, sonra Agent'ı yeniden başlatın: CelikPanel
  yeni dosyayı o açılışta yazar).
- **Elle değiştirdiğiniz yerel kapı dosyası korunur;** CelikPanel'in metni
  yanına `celikpanel-webmail.conf.celikpanel-pending` olarak konur (web
  sunucusu onu okumaz) ve **Şu anki sorunlar** tek satırda yerel kapının hâlâ
  Roundcube dosyalarını verdiğini, sunucu sahibinin yapacağını ve adımı
  söyler. Dosyada CelikPanel'in kuralları olunca satır kendiliğinden kalkar.

## Sizin için ne değişiyor

### Web postası gönderir

- **Web postasından gönderim önceki hiçbir sürümde çalışmadı** (posta okuma,
  ekler ve ayarlar çalışıyordu). Web postası bu sunucunun kendi gönderim
  hizmetiyle şifresiz konuşuyordu; o hizmet ise girişi yalnız şifreli
  bağlantıda kabul eder; bu yüzden her ileti "kimlik doğrulanamadı" ile
  bitiyordu.
- **Web postası artık şifrelemeyi açar** ve bir posta programı gibi posta
  kutusunun kendi adresi ve parolasıyla giriş yapar. Bağlantı sunucunun
  içinde kalır (`127.0.0.1`); bu yüzden yalnız bu iç adreste sertifikanın adı
  ve zinciri denetlenmez: gönderim hizmeti posta adının sertifikasını ya da o
  yokken kendi imzalı bir sertifikayı gösterebilir ve gönderim iki durumda da
  çalışmalıdır. Başka hiçbir adres için hiçbir şey gevşetilmez.
- Web postasından giden posta, o kutunun diğer postaları gibi göndericinin
  kendi alan adı adına imzalanır (alpha.143'ün kuralı, değişmedi). Posta
  programları için 465 ve 587 kapıları değişmedi.
- **Kurulu sunucular kendiliğinden düzeltilir:** güncellemeden sonraki ilk
  açılışta web postasının yapılandırması bir kez, kipi, sahibi ve gizli
  anahtarı korunarak yeniden yazılır.
- **Elle değiştirdiğiniz web postası yapılandırması korunur;** CelikPanel'in
  metni yanına `config.inc.php.celikpanel-pending` olarak konur (yalnız root
  okuyabilir; Roundcube onu okumaz) ve **Şu anki sorunlar'da tek satır** web
  postasının gönderemediğini, nedenini ve adımı söyler: o metni (ya da iki
  gönderim satırını) dosyanıza kopyalayın; yanına metin konamadıysa satır
  yazılacak iki satırı verir. CelikPanel kendi önceki metnini yeniden
  yazamadıysa (salt okunur dosya sistemi, değiştirilemez işareti, dolu disk)
  satır bu nedeni giderip Agent'ı yeniden başlatmayı söyler — dosyayı elle
  düzenlemeyi değil. Dosya şifreli gönderince satır kendiliğinden kalkar.
- Web postası gönderemediği sürece Webmail kartı "kurulu ve doğrulanmış"
  demez.

### Küçük düzeltmeler

- Bir takma adın "yayımlanmayı bekliyor" durumu, bölgesinin bekletilen
  değişikliği yayımlanınca ya da bölgenin yedek sunucuyu bekleyen kendi yayını
  tamamlanınca biter — bir saat sonra değil. Beklerken bölgenin uyarısı artık
  değişikliği yeniden yapmayı istemez; takma ad kayıtlarının kendiliğinden
  yayımlanacağını söyler.
- PostgreSQL kapatma ya da geri alma penceresi sonucu söyleyen bir başlıkla
  biter (kapatıldı, kapatılamadı, bilinmiyor); sürmekte olan "… kapatılıyor"
  başlığıyla bitmez.

## Bilinen sınırlar

- **Sunucudaki her hesabın yerel kapıdan hâlâ erişebildikleri:** web
  postasının oturum açma sayfası — herkese açık adresinin gösterdiği aynı
  sayfa; yani Roundcube'un kendisi, bir girişle; istemci adresine göre
  çalışan bir deneme sınırı bunu görmez — ve PHP sürümü (CelikPanel'in PHP
  denetimi söyler). root'un Roundcube'un `skins` klasörünün içinde yaptığı bir
  bağlantı üzerinden erişilen stil dosyası verilir.
- **Sahibin o sokete istek ileten kendi web sunucusu bloğu** (örneğin web
  postasını bir sitenin yolunda tutmak için) artık yalnız oturum açma
  sayfasını ve stil dosyalarını alır; bunun yerine web postasının kendi
  adresini kullanın.
- **"Gönderemiyor" denetimi yalnız açık durumları okur.** Sahibin kendi
  sertifika seçenekleriyle elle yazdığı bir gönderim ayarı sahibin seçimi
  sayılır ve denetlenmez; `/* */` yorumunun içindeki bir `smtp_host` etkin
  okunur. Şifrelemeyi adla açıp sertifika ayarı olmayan bir yapılandırma ise
  adlandırılır.
- **Web postası gönderemezken Webmail kartı Onar düğmesini yine sunar;**
  Onar yapılandırmayı düzeltmez. Adım Şu anki sorunlar'dadır.
- **alpha.144'e geri dönülürse:** web postası göndermeyi sürdürür ve yerel
  kapı kapalı kalır; ama alpha.144 yeni metinleri tanımaz: web postasının
  yapılandırmasını ve yerel kapının dosyasını sahibin değiştirdiği dosyalar
  sayar; bu yüzden sunucu yeniden güncellenene dek web postasının adres
  satırları, web postasının onarımı ve web postasını başka bir PHP sürümüne
  taşımak elle değiştirilmiş diye reddedilir. (Bu iletinin dediği gibi yerel
  kapının dosyasını silerseniz alpha.144 önceki, açık biçimini yeniden
  yazar.)
- alpha.144'ün açık listesi açık kalır
  ([alpha.144 sürüm notları](RELEASE-NOTES-v0.1.0-alpha.144.tr.md)); aralarında
  tarayıcının isteğiyle çalışan diğer değişiklikler ve web sunucusunun ortak
  hesabında kalan programların trafik günlüklerini okuyabilmesi var.

## Laboratuvarda denetlenenler

**AÇIK.** Bu sürümün laboratuvar denemesi henüz yapılmadı. Liste sürüm çekme
isteğindedir (#357): yerel kapı güncellemeden önce ve sonra bir site hesabı
olarak; Debian 13 ve Ubuntu 24.04'te iki sertifika durumunda web postasından
gönderim, yolun posta günlüğünde kanıtıyla; iki dosya için başarısız yeniden
yazma ve sahibin değiştirdiği dosya; takma ad beklemesi iki yolda;
PostgreSQL penceresinin başlıkları.

## Güncellemeden sonra ne beklenir

- Şema adımı yoktur: düzen şema 81 kalır; alpha.144'e geri dönmek olağan
  geri almadır.
- Web sunucusu ilk açılışta, düzeltilen her dosya için bir kez (yerel
  kapının dosyası; bir web postası adres dosyası yalnız hâlâ önceki
  biçimindeyse), her biri yapılandırma denetimi geçtikten sonra yeniden
  yüklenir.
- Web postasının yapılandırması ilk açılışta bir kez, kipi, sahibi ve gizli
  anahtarı korunarak yeniden yazılır; web postası o andan sonra gönderebilir.
  Elle yapılacak bir şey yoktur.
- **Şu anki sorunlar** bir web postası satırı gösterirse adım o satırdadır.

## Kanıt

Şema değişikliği yoktur: `internal/db/migrations/` altında hiçbir dosya
değişmez; düzen 81 kalır. Yeni olanların hepsi eklemedir: salt okur Agent
çağrısı `WebmailAddressFormsV1`'in `sending`, `sending_file` ve
`sending_reason` alanları (ve yerel kapı için adressiz bir kayıt); bölge
yanıtının `aliases_waiting` alanı; bileşen kaydının `webmail_cannot_send`
alanı; Şu anki sorunlar türü `webmail_cannot_send` ve posta profili kodu
`webmail_cannot_send`. Yeni Agent çağrısı, yol ya da denetim kaydı eylemi
yoktur.

Maddeye göre testler:

- Yerel kapı: `TestA145LocalServerAnswersOnlyCelikPanelsOwnChecks` (işlenmiş
  dosyayı nginx'in konum seçtiği gibi okur: oturum açma sayfası, giriş
  noktası, stil dosyaları ve PHP denetimi yanıt verir; bir eklenti kopyası,
  bir eklentinin PHP'si, nokta dosyaları, `composer.json`,
  `index.php/extra` ve bir `..` yolu bulunamaz; önceki biçim bunları veriyor
  ve çalıştırıyordu), `TestA145ProofAndReadinessStillAnswerOnTheLocalServer`,
  `TestA145EarlierLocalServerIsRewrittenAtStartWithOneReload`,
  `TestA145OwnersLocalServerFileIsKeptWithTheTextBesideIt`,
  `TestA145LocalServerRewriteFailuresThatAreNotAnotherFileAreUnnamed`,
  `TestA145LocalServerPendingTextGoesWithTheFile`,
  `TestA145LocalServerInItsEarlierFormIsAProblemUntilItIsNot`,
  `TestA145LocalServerAlertMailFollowsTheScreensBranches`; dağıtım
  yapılandırma matrisi yeni dosyayı işler (`TestRenderDistroConfigMatrix`, CI'da
  nginx ile denetlenir). Bağımsız bir inceleme yeni dosyayı gerçek bir nginx
  üzerinde de çalıştırdı (Debian 13): yalnız listelenen istekler, hangi
  kodlamayla gelirse gelsin, yanıt alır.
- Web postası gönderir: `TestA145WebmailSendsWithEncryptionAndRelaxesTheCheckOnLoopbackOnly`,
  `TestA145SendingSettingIsReadFromTheLastAssignment`,
  `TestA145EarlierConfigurationIsCorrectedAtTheFirstStart`,
  `TestA145AddressLinesOnTheEarlierTextWriteTheCurrentOne`,
  `TestA145OwnerEditedConfigurationIsKeptWithTheTextBesideIt`,
  `TestA145OwnerSendingSettingIsNotNamed`,
  `TestA145FailedRewriteOfCelikPanelsTextIsItsOwnState`,
  `TestA145RewrittenConfigurationIsPublishedWithItsFinalOwner`,
  `TestA145WebmailThatCannotSendIsAProblemUntilItCan`,
  `TestA145FailedRewriteIsNamedWithTheRestartNotAHandEdit`,
  `TestA145TheOwnerIsGivenCelikPanelsTwoSendingLines`,
  `TestA145WebmailProfileIsNotCompleteWhileItCannotSend`; var olan gönderim,
  imzalama ve web postası adres testleri değişmeden geçer.
- Takma ad beklemesi: `TestA145AliasWaitEndsWhenTheHeldPublicationCompletes`,
  `TestA145AliasWaitIsSettledWhenThePendingPublicationCompletes`,
  `TestA145AliasWaitSettledByTheFollowUpPublicationWithoutASecondOne`.
- Metinler, PostgreSQL penceresinin başlıkları ve bölge uyarısı:
  `web/tests/a145-webmail-send-lab.test.mjs`.

Değişen paketlerin Go testleri ve web testleri temel üzerinde (alpha.144'ün
kodu) ve bu sürümün kodu üzerinde, adıyla aynı başarısız kümeyle çalıştı
(test makinesinde root'a ait geçici klasör gerektiren iki test) ve sürüm
çekme isteğinin her denetimi CI'da geçti. Çekme isteğinin iki bağımsız, salt
okur incelemesi sürümü durduran bir şey bulmadı; bulguları bu sürümde
düzeltildi ya da yukarıda listelendi.
