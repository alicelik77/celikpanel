# v0.1.0-alpha.135

[English](RELEASE-NOTES-v0.1.0-alpha.135.md)

Üç işlik küçük bir sürüm. Bir alan adını silerken veritabanlarına ne
olacağı sorulur ve siteden geride hiçbir şey kalmaz; silmeden kalan posta
ve yedekler tek bir listede 30 gün saklanır, sonra kendiliğinden silinir.
Her posta kutusu kendi web postasını açar, gönderim sınırları posta
kutusunun satırında tek satırdır. Yedek DNS sunucusunda web postası adresi
denetimi, eksik kaydı ana DNS sunucusundan eklemesini isteyebilir.
**Kurulu panelleri yalnız sahip, panelin kendi güncelleme ekranından
günceller.**

## alpha.134'ten güncelleme

1. Her zamanki gibi panelin kendi güncelleme ekranından güncelleyin. Bu
   sürüm panelin veritabanı yapısını değiştirmez (alpha.134'teki gibi
   kalır); bu yüzden sonra alpha.134'e dönmek veritabanı geri yüklemesi
   gerektirmez.
2. **Güncellemeden sonra bir kez Sunucu → Bakım → Silmelerden kalanlar
   sayfasına bakın.** Panel açılır açılmaz liste, önceki sürümlerin bu
   sunucuda bıraktıklarını alır (yalnız bir kayıt yazılır; diskte hiçbir
   şey değişmez): önceden silinen alan adlarının kenara alınmış ama hiç
   silinmemiş postası ve artık var olmayan alan adlarının yedekleri.
   **Bunların 30 günü güncellemeyle başlar; sonra kendiliğinden silinirler.**
   Bir şeyi saklamak isterseniz:
   - yedek için: satırında **İndir**'e, sonra istediğiniz her dosyaya basın;
   - posta için: satırın **Ayrıntılar** bölümünde yazan klasörü tarihten
     önce başka bir yere kopyalayın (sunucu yöneticisi olarak; henüz "geri
     ver" düğmesi yok, bkz. Bilinen sınırlar).
   **Şimdi sil…** ya da **Hepsini şimdi sil…** düğmesine basmadıkça listedeki
   hiçbir şey tarihinden önce silinmez. Uzak yedekleme hedefindeki kopyalara
   hiç dokunulmaz.
3. Aynı sayfa **yerinde kalan postayı** da listeler: bu sürümden önce
   silinen posta kutularının, alan adının canlı posta klasöründe duran ve
   aynı adresle açılacak yeni bir kutunun göreceği postası. Hiçbir şey
   kendiliğinden taşınmaz. Kenara almak istediğiniz her biri için **30 gün
   saklamaya al…** düğmesine basın; böyle bir adreste açılan yeni posta
   kutusu da önce eski postayı kendiliğinden listeye alır.
4. Önceki sürümlerin alan adı silmeleri, veritabanlarını veritabanı
   sunucusunda ama panelin listelerinin dışında bıraktı. Bu sürüm onları
   geri getirmez (bkz. Bilinen sınırlar); bundan sonra silinen bir alan adı,
   siz silmeyi seçmedikçe veritabanlarını listede tutar.

## Sizin için ne değişiyor

### Alan adı silme

- **Soru önce veritabanlarını sorar.** Alan adının veritabanları varsa
  "Veritabanlarını da sil" sorunun ilk satırıdır; altındaki tek cümle,
  işaretliyken ve değilken ne olacağını söyler. İşaretli değilken
  (varsayılan) veritabanları kullanıcıları ve parolalarıyla, hiçbir alan
  adına bağlı olmadan Veritabanları sayfasında kalır. İşaretliyken her
  veritabanı Veritabanları sayfasının sildiği gibi silinir (başka bir
  veritabanının kullandığı kullanıcı kalır, yalnız bu veritabanındaki
  hakları gider). Yöneticiye sonuç, Veritabanları sayfası gibi, veritabanı
  üzerindeki haklarının kimden kaldırıldığını da söyler.
- **Siteden geride hiçbir şey kalmaz.** Sitenin zamanlanmış görevleri, web
  sunucusu ve trafik günlükleri (eski kopyalarıyla) ve sertifika denetim
  klasörü siteyle birlikte gider; sunucunun gönderim sınırı listeleri alan
  adı olmadan yeniden yazılır.
- **Posta ve yedekler 30 gün saklanır.** Alan adının saklanan postası ve bu
  sunucudaki yedekleri Bakım → Silmelerden kalanlar listesine gider; soru
  bunu söyler. Biten pencere "<alan adı> silindi" der ve kalanları sayar:
  veritabanları ve nerede oldukları, posta ve yedekler boyutları ve
  silinecekleri tarihle. Müşteriye, sağlayıcının bunları o tarihe kadar
  geri verebileceği söylenir; klasörleri yalnız yönetici görür.
- Bir adımda duran silme hangi adımda durduğunu, kimin ne yapacağını
  söyler; Sil'e yeniden basmak, başladığı veritabanı seçimiyle kaldığı
  yerden sürdürür.

### Silmelerden kalanlar (Sunucu → Bakım)

- Tek liste: silinen alan adlarının ve posta kutularının postası ve
  silinen alan adlarının bu sunucudaki yedekleri; her biri boyutu ve
  kendiliğinden silineceği tarihle, toplamı en üstte.
- Satırdaki **Şimdi sil…** ve **Hepsini şimdi sil…** panelin kendi
  penceresinde sorar ve her adımı gösterir; liste kendini yeniden okur.
- Yedeğin dosyaları için **İndir**.
- Silinemeyen bir öğe nedeniyle (**Ayrıntılar** altında) listede kalır ve
  kendiliğinden yeniden denenir (bir saat sonra, sonra daha seyrek); yanıtı
  kaybolan bir silme "bilinmiyor" diye söylenir, asla yapıldı diye değil.
- Yöneticinin ana sayfası ne kadarının saklandığını tek satırda söyler ve
  listeyi açar. Disk kullanımı %85'i geçince disk uyarısı bu listeyi şimdi
  silinebilecek bir şey olarak anar.
- Tek bir posta kutusunu silmek de artık postasını bu listeye taşır; aynı
  adresle yeniden açılan kutu boş başlar. Soru ve sonuç bunu söyler.

### Posta kutuları

- **Her posta kutusu satırında "Web posta" var.** Web postasını yeni
  sekmede, giriş sayfasında o kutunun adresi yazılı olarak açar.
  Bağlantıda başka bir şey yoktur: parola da oturum da yok.
- Satırın eylemleri sözcüktür: satırın yanında **Web posta** ve
  **Parola…**, kalanlar **Diğer** altında, silenler en sonda. Telefonda her
  posta kutusu, eylemleri genişlik boyunca duran bir karttır.
- Genel web postası kartı, web postası çalışırken kalktı. Yalnız sorun ve
  çözümüyle görünür (kurulu değil, adresi yok, yanıt vermiyor): yönetici
  düzelten düğmeyi alır, müşteriye sağlayıcısına sorması söylenir.
- **Gönderim sınırları artık bir kart değil.** Sınırlar sunucuda
  uygulanırken her satırda kısa bir satır vardır: sınırı (ya da yeni
  kutunun sınırı ve ne zamana kadar) ve bu saat gönderdiği. **Diğer**
  altındaki **Gönderim sınırı…** o kutunun sınırını Kaydet ve Vazgeç ile
  kendi penceresinde açar. Sınırlar uygulanmıyorken müşteri bunlarla ilgili
  hiçbir şey görmez; yönetici posta sunucusu ayarlarına götüren düğmeyle
  tek satır görür. Alan adının ve barındırma hesabının sınırları yalnız
  önemli olduğunda anılır (bu saatin %80'i ya da son 7 günde geri
  çevrilenler).
- Gönderimi durdurulan posta kutusunun uyarısı ve çıkış yolu (yeni parola)
  listenin bütün genişliğinde bir satırda durur.

### Web postası adres kaydını yedek DNS sunucusundan eklemek

Bu sunucu bir eşin yedek DNS sunucusuysa ve web postasının adresi
(örneğin `webmail.secondary.example.com`) ana DNS sunucusunun bölgesindeyse,
adres denetimi artık **Kopyala**'nın yanında **Ekle…** sunar. Onaylanan tek
bir istek, iki sunucunun zaten sahip olduğu bağlantıyla ana DNS
sunucusundan kaydı eklemesini ister. Panelin kendi penceresi hangi kaydın,
hangi bölgeye, hangi sunucuda ekleneceğini ve başka hiçbir şeyin
değişmediğini söyler; adımlar ardından gelir (ana sunucuya sormak, kaydın
orada yazılması, genel DNS'in onu göstermesi). Eklemeden sonra sayfa
genel DNS'i en çok 5 dakika boyunca 20 saniyede bir okur, sonra kurulum
düğmesini gösterir ya da sürenin dolduğunu söyleyip **Denetle** sunar.
Her son olduğu gibi söylenir: eklendi ya da zaten vardı; adı konmuş bir
kuralla reddedildi, kimin ne yapacağıyla birlikte, **Kopyala** yerinde
kalır; ya da bilinmiyor, sonraki adım **Denetle**, hiçbir şey yeniden
gönderilmez. Ana sunucunun kabul etmediği IPv6 (AAAA) kaydı söylenir ve
kendi **Kopyala**'sıyla isteğe bağlı bırakılır. Bu panelin kendi
barındırdığı bir bölge, kendi kayıt düzenleyicisi üzerinden aynı onayı ve
adımları alır. Başka yerde yönetilen bir ad için denetim, genel DNS
gösteriyorsa en yakın bölgesinin ad sunucularını adıyla söyler.

## Bilinen sınırlar

### Silmelerden kalanlar

- Saklanan posta ve yedekler için henüz **Geri yükle** yok. Yedek
  indirilebilir; postayı sunucu yöneticisi **Ayrıntılar** altında yazan
  klasörden geri kopyalar.
- Alan adının adını taşıyan bir klasörde (barındırma hesabının klasöründe
  değil) duran eski yedekler listelenmez ve liste onları silmez.
- Saklanan boş bir yedek klasörü 0 B görünür.
- Liste yerinde kalan posta klasörlerinden bir seferde en çok 500 tanesini
  gösterir, daha fazlası varsa bunu söyler; bunlar saklamaya alındıkça
  kalanlar görünür.
- Önceki sürümlerin alan adı silmelerinin veritabanı sunucusunda (panelin
  listeleri dışında) bıraktığı veritabanları panele geri getirilmez.

### Posta kutuları

- Tarayıcıda başka bir posta kutusunun açık web postası oturumu varsa
  **Web posta** o oturumu açar; öteki kutuya geçmek için önce web
  postasından çıkın.

### Ana DNS sunucusu üzerinden kayıt eklemek

- Bu yolla yalnız adres kayıtları (A ve AAAA) eklenebilir; TXT kayıtları
  (örneğin posta doğrulama kayıtları) iki sunucu arasındaki bağlantıyla
  eklenemez ve **Kopyala** olarak kalır.

### alpha.136'ya taşınanlar

Bir veritabanının zaten silindiğini hatırlamak (ikinci basış arada aynı
adla açılan veritabanını silmesin diye); CelikPanel'in motor hesabının
geride kalmış kopyalarının kendiliğinden onarılması; veritabanı
kullanıcısının adını neyin boşa çıkaracağını söyleyen ret; PostgreSQL'de
veritabanının kendi kullanıcısına ait tablolardaki haklar; hesap kalırken
"kullanıcı kaldırıldı" sonucu; Veritabanları listesinde boyutlar.

### Diğer

- Önceki sınırlar (bkz. alpha.134) bu sürümün düzelttikleri dışında
  geçerlidir: alan adı silme artık veritabanlarını panelin listelerinden
  düşürmez; önceki bir adresle yeniden açılan posta kutusu artık eski
  postayı görmez.

## Laboratuvarda denenenler

**Yer tutucu: bu sürümün laboratuvar denemesi henüz yapılmadı.** Bu bölüm
denemeden sonra; neyin denendiği, neyin geçtiği ve hiçbir denemenin neye
bakmadığıyla yazılır.

## Kanıt

Şema değişikliği yok: alpha.134'teki gibi şema 079. Saklama listesi
`panel_settings` içinde öğe başına bir sürümlü kayıttır
(`deletion_hold_v1:<id>`); alan adı silmesinin veritabanı seçimi silme
işaretiyle birlikte saklanır (sürümlü). Yeni Ajan çağrıları: yalnız okuyan
iki çağrı (saklananlar, yerinde kalan posta klasörleri) ve değiştiren iki
çağrı; değiştirenleri başka yapıdaki bir Ajan reddeder (saklanan bir öğeyi
silmek: yol türünden ve kimliğinden çıkarılır, tam bir klasör, bağlantı
izlenmez, zaten gitmiş öğe silinmiş sayılır; yerinde kalan bir posta
klasörünü saklamaya almak: adresten çıkarılır, adreste posta kutusu varken
reddedilir). Posta kutusu ve alan adı silmeleri postayı yerinde bırakmak
yerine saklama klasörüne taşır. `/api/v1/maintenance/deletion-holding`
altında yeni yönetici uçları (liste, şimdi sil, yedeğin dosyaları ve
indirme, yerinde kalan klasörü saklamaya alma) ve
`POST /api/v1/service/roundcube/address/dns-record` (yalnız yönetici, bir
istek kimliğiyle: aynı kimlik saklanandan yanıtlanır, asla yeniden
gönderilmez). İsteği alan ana DNS sunucusu değişmedi; alpha.133 ya da alpha.134
üzerindeki bir ana sunucu da aynı yanıtı verir. Yeni denetim kaydı
eylemleri `deletion_hold.*` ve `webmail.address.dns_record.*`. Alan adı
silmesiyle silinen veritabanı, alpha.134'ün hak kaldırmasıyla tek
veritabanı silme yolundan (`deleteDatabaseRecord`) geçer; alan adı
silmesinin sonucu da yalnız yöneticiye gösterilen aynı hak bilgilerini
taşır (`databaseDeletionAnswerFor`).
Bileşen testleri: `cmd/panel/deletion_holding_test.go`,
`cmd/panel/deletion_holding_left_test.go`,
`cmd/panel/domain_delete_cleanup_test.go`,
`cmd/panel/webmail_address_dns_record_test.go`,
`cmd/panel/mail_outgoing_test.go`, `cmd/agent/deletion_holding_linux_test.go`,
`cmd/agent/deletion_holding_left_linux_test.go`,
`cmd/agent/site_delete_cleanup_test.go`; bağlı web testleri
`web/tests/domain-delete-question-mounted.test.mjs`,
`web/tests/remote-state-mounted.test.mjs`,
`web/tests/mail-rows-alpha135.test.mjs`,
`web/tests/webmail-dns-add-alpha135.test.mjs`,
`web/tests/refusal-words-alpha135.test.mjs`
(RESILIENCE-CONTRACT, "Branch a134/deldomain", "Branch a135/mailrow" ve
"Branch a135/dnsadd"). Laboratuvar denemesine kadar yalnız bileşen
testleri. **Hiçbir P0 işi kapanmadı.**
Sürüm sırası 134 → 135.
