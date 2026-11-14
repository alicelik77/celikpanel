# v0.1.0-alpha.135

[English](RELEASE-NOTES-v0.1.0-alpha.135.md)

İki işlik küçük bir sürüm. Bir alan adını silerken veritabanlarına ne
olacağı sorulur ve siteden geride hiçbir şey kalmaz; silmeden kalan posta
ve yedekler tek bir listede 30 gün saklanır, sonra kendiliğinden silinir.
Her posta kutusu kendi web postasını açar, gönderim sınırları posta
kutusunun satırında tek satırdır.
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
4. **Önceki silmelerin posta ve yedek dışında bıraktıkları yerinde kalır.**
   Önceki bir sürümle alan adı silmiş bir sunucuda bu sürüm şunları ne
   listeler ne de siler: silinen sitenin web sunucusu ve trafik günlükleri,
   zamanlanmış görevler dosyası (artık var olmayan bir hesabın crontab'ı),
   sertifika denetim klasörü ve MariaDB ile PostgreSQL'de duran ama panelin
   hiçbir listesinde görünmeyen veritabanları ve veritabanı kullanıcıları.
   Bunların hiçbiri hiçbir sayfada görünmez. Durdukları yerde zarar
   vermezler; sunucu yöneticisi onları elle kaldırabilir. Bundan sonra
   silinen bir alan adı bunların hiçbirini geride bırakmaz; veritabanları
   da siz silmeyi seçmedikçe listede kalır.

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
- Önceki sürümlerin alan adı silmelerinin posta ve yedek dışında
  bıraktıkları (günlükler, bir crontab, sertifika denetim klasörü,
  veritabanları ve kullanıcıları) ne listelenir ne silinir (bkz.
  "alpha.134'ten güncelleme", 4. adım).

### Posta kutuları

- Tarayıcıda başka bir posta kutusunun açık web postası oturumu varsa
  **Web posta** o oturumu açar; öteki kutuya geçmek için önce web
  postasından çıkın.

### alpha.136'ya taşınanlar

Bir veritabanının zaten silindiğini hatırlamak (ikinci basış arada aynı
adla açılan veritabanını silmesin diye); CelikPanel'in motor hesabının
geride kalmış kopyalarının kendiliğinden onarılması; veritabanı
kullanıcısının adını neyin boşa çıkaracağını söyleyen ret; PostgreSQL'de
veritabanının kendi kullanıcısına ait tablolardaki haklar; hesap kalırken
"kullanıcı kaldırıldı" sonucu; Veritabanları listesinde boyutlar. Ayrıca web
postası adres denetiminde **Kopyala**'nın yanındaki, web postası kaydını
eklemesini eşli ana DNS sunucusundan isteyen **Ekle…** düğmesi:
laboratuvar, gerçek bir sunucu çiftinde kaydı eklemediğini gördü; bu yüzden
bu sürümde yok (denetim, alpha.134'teki gibi **Kopyala** sunar).

### Laboratuvarda bulunan, sonraki sürüme kalan kusurlar

- Başka bir sekmede açık **Alan Adları** sayfası başka yerde yapılan bir
  silmeyi izlemez; silinen alan adı sayfa yeniden açılana dek orada durur
  (**Veritabanları** sayfası izler).
- Yerine bağlantı konmuş tutulan bir klasörü **Şimdi sil…** doğru biçimde
  reddeder, ama sonraki tur onu listeden düşürür; bağlantı diskte kalır.
- Yarıda kalan bir silme (silinemeyen bir dosya) o klasörün geri kalanını
  silmiş olur; satır başarısız olduğunu ve nedenini söyler.
- **30 gün saklamaya al…** sonrasında biten pencerede "Tamamlandı" altında
  artık bir "Sunucuda çalışılıyor…" satırı kalır.
- Ana sayfanın etkinlik akışı tutulan bir öğeyi iç anahtarıyla (örneğin
  `backups.s5-d39`) anar, "<alan adı> yedekleri" diye değil.
- Alan adı silmesinin sonucundaki haklar paragrafı uzun; tek satır olmalı,
  kalanı kapalı **Ayrıntılar** altında. Silmelerden kalanlar listesindeki
  klasör yolları açık gösterilir.

### Diğer

- Önceki sınırlar (bkz. alpha.134) bu sürümün düzelttikleri dışında
  geçerlidir: alan adı silme artık veritabanlarını panelin listelerinden
  düşürmez; önceki bir adresle yeniden açılan posta kutusu artık eski
  postayı görmez.

## Laboratuvarda denenenler

İki sanal sunucuda bir laboratuvar denemesi: MariaDB 11.8.6 ve PostgreSQL
ile Debian 13, MariaDB 10.11.14 ile Ubuntu 24.04. İkisi de alpha.134'ten
panelin kendi güncelleme sayfasıyla güncellendi; her güncelleme
kendiliğinden bitti, bir panel ve bir Ajan çalışıyordu. Hiçbir denemede
ağır bir hata çıkmadı.

**Denendi ve geçti:**
- Bir alpha.134 silmesinin bıraktıkları (silinen bir alan adının postası
  ve yedeği) güncellemede, 30 günü o andan başlayarak listeye alındı ve
  diskte hiçbir şey değişmedi: ne güncellemede, ne yeniden başlatmadan
  sonra, ne de sonraki iki turda.
- Tarihi geçen bir öğe kendiliğinden silindi; silinemeyen bir öğe
  nedeniyle başarısız olarak kaldı ve neden giderilince sonraki denemede
  silindi. **Şimdi sil…** tam o öğeyi sildi; **Hepsini şimdi sil…**
  listedeki on öğeyi sildi, başka hiçbir şeyi değil (canlı yedekler ve posta
  yerinde). Tutulan bir yedek bayt bayt aynı indirildi.
- Tutulan bir klasörün içindeki bağlantılar bağlantı olarak silindi;
  gösterdikleri dışarıdaki şeyler kaldı. Yerine bağlantı konmuş tutulan
  bir klasör reddedildi ve olduğu gibi bırakıldı.
- "Veritabanlarını da sil" işaretliyken alan adını silmek, yönetici ve
  müşteri olarak, MariaDB ve PostgreSQL'de: veritabanları ve kullanıcıları
  iki motordan da gitti, başka bir hesaptaki haklar kaldırıldı (yalnız
  yöneticiye adıyla söylendi), crontab, günlük ya da sertifika denetim
  klasörü kalmadı, posta ve yedekler listede. İşaretli değilken
  veritabanları kaldı, listede ve çalışır durumda.
- Silinip aynı adresle yeniden açılan posta kutusu boş başladı; eski
  postasını alpha.134'ün yerinde bıraktığı bir adreste de (o posta önce
  listeye alındı). **30 gün saklamaya al…** kalan bir klasörü listeye
  taşıdı.
- Posta kutusu satırının **Parola…** ve **Diğer** menüleri, sorunun
  İngilizcesi ve iki sunucuda ana sayfalara kısa bir bakış.

**Denenmedi:** posta kutusu satırından web postasını açmak (laboratuvarın
web postasının kendi adresi yoktu), sınırlar uygulanırken gönderim sınırı
satırı, alpha.134'e dönüş ve süreler (laboratuvar makinesini başka bir iş
yüklüyordu). **Ekle…** düğmesi başarısız oldu (bkz. "alpha.136'ya
taşınanlar").

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
indirme, yerinde kalan klasörü saklamaya alma). Yeni denetim kaydı
eylemleri `deletion_hold.*`. Alan adı
silmesiyle silinen veritabanı, alpha.134'ün hak kaldırmasıyla tek
veritabanı silme yolundan (`deleteDatabaseRecord`) geçer; alan adı
silmesinin sonucu da yalnız yöneticiye gösterilen aynı hak bilgilerini
taşır (`databaseDeletionAnswerFor`).
Bileşen testleri: `cmd/panel/deletion_holding_test.go`,
`cmd/panel/deletion_holding_left_test.go`,
`cmd/panel/domain_delete_cleanup_test.go`,
`cmd/panel/mail_outgoing_test.go`, `cmd/agent/deletion_holding_linux_test.go`,
`cmd/agent/deletion_holding_left_linux_test.go`,
`cmd/agent/site_delete_cleanup_test.go`; bağlı web testleri
`web/tests/domain-delete-question-mounted.test.mjs`,
`web/tests/remote-state-mounted.test.mjs`,
`web/tests/mail-rows-alpha135.test.mjs`,
`web/tests/refusal-words-alpha135.test.mjs`
(RESILIENCE-CONTRACT, "Branch a134/deldomain" ve "Branch a135/mailrow").
Sürümün `90293c5c2` başında bir laboratuvar denemesi (bkz.
"Laboratuvarda denenenler"); ondan sonra yalnız **Ekle…** işi çıkarıldı
(birleştirmesi geri alındı). **Hiçbir P0 işi kapanmadı.**
Sürüm sırası 134 → 135.
