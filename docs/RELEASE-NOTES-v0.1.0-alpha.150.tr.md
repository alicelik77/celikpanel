# v0.1.0-alpha.150

[English](RELEASE-NOTES-v0.1.0-alpha.150.md)

**Düzeltmeler içeren bir güvenlik sürümü.** Bir sitenin zamanlanmış
görevleri artık sitenin kendi sınırlarının içinde çalışır; bir sitenin
kendi kodunun bu sınırların dışında iş çalıştırabildiği son yollar
kapatıldı. Güncellemeden sonra her site, zamanlanmış görevleri ve çalışan
programlarıyla birlikte, kendiliğinden sınırlarının içine alınır; basılacak
bir şey yoktur. Bu sürüme güncelleme sırasında sunucunun yeniden
başlatılması artık güncellemeyi yarım bırakmaz. Ardından dört küçük
düzeltme gelir. Panelin veritabanı düzeni değişmez.
**Kurulu panelleri yalnız sahibi, panelin kendi güncelleme ekranından
günceller.**

## alpha.149'dan güncelleme

1. Panelin kendi güncelleme ekranından, eskisi gibi güncelleyin.
2. **Panelin veritabanı düzeni değişmez** (şema 81 olarak kalır). Yeni panel
   açılmadan önce başarısız olan bir güncelleme kendiliğinden alpha.149'a
   döner. Tamamlanmış bir güncelleme geri alınmaz; aşağıdaki "alpha.149'a
   dönmek" bölümüne bakın.
3. **Güncellemenin kendisi bir müşterinin hiçbir şeyini yeniden
   başlatmaz.** Güncellenen panel açıldıktan yaklaşık beş dakika sonra panel
   siteleri tek tek sınırlarının içine almaya başlar: bir sitenin çalışan her
   programı bir kez yeniden başlatılır (bkz. "Güncellemeden sonra ne
   beklenir").
4. **Bu güncelleme sürerken sunucunun yeniden başlatılması artık onu yarım
   bırakmaz:** sunucu yeniden açıldıktan sonra güncelleme kendiliğinden
   tamamlanır.

## Güvenlik: bir sitenin zamanlanmış görevleri sitenin sınırlarının içinde çalışır

- **Şimdiye kadar bir sitenin zamanlanmış görevleri sitenin sınırlarının
  dışında çalışıyordu.** Cron'dan, sitenin bellek, süreç ve işlemci
  sınırlarının ve doğrudan posta göndermeye konan engelin dışında
  çalışıyorlardı; bir sitenin kendi kodu da orada kendi işlerini
  çalıştırabiliyordu: kendi crontab'ını kurarak, hesabı için "lingering"i
  açarak ya da bir SFTP girişinin başlattığı kendi hizmet yöneticisi
  üzerinden.
- **Artık her zamanlanmış görev standart bir systemd zamanlayıcısıdır**;
  sitesinin sınırlarının içinde, sitenin hesabıyla ve sitenin programlarıyla
  aynı posta engelinin altında çalışır. Bir görev zamanlamasını korur. Bir
  görevin arka planda çalışır bıraktığı şey sitenin sınırlarının içinde
  kalır.
- **Müşteri ne görür.** Görevler Zamanlanmış görevler ekranında eskisi gibi
  eklenir ve değiştirilir. Ekran artık her görev için son çalışmasını (ne
  zaman, ne kadar sürdü, çıkış durumu ve yazdığı son satırlar) ve bir
  sonraki çalışmasını gösterir; vakti yeniden geldiğinde hâlâ çalışan bir
  göreve ne olduğunu tek cümleyle söyler: ikinci kez başlatılmaz, bitince
  hemen bir kez daha çalışır. Müşteri yalnız kendi sitesinin görevlerini
  görür.
- **Ne reddedilir ve neden.** Bir sitenin hesabı artık kendi crontab'ını
  kuramaz (`crontab -e` hesabın bu programı kullanamayacağını söyler),
  kendisi için "lingering"i açamaz (bir yönetici parolası istenir) ve bir
  SFTP girişinde kendi hizmet yöneticisini başlatamaz (giriş yine çalışır).
  Bunlar bir sitenin kendi kodunun sınırlarının dışında iş çalıştırabildiği
  yollardı. Bir görevin içinde ek yetki isteyen programlar bu yetkiyi almaz
  (`sudo`, `crontab`, `at` ve sunucunun `sendmail` komutu, dolayısıyla komut
  satırından çağrılan PHP `mail()` de): bir görev postayı bir posta
  kutusunun adı ve parolasıyla SMTP üzerinden gönderir. `CRON_TZ=`,
  `RANDOM_DELAY=`, `START_HOURS_RANGE=` ya da Go çalışma ortamının bir
  ayarını (`GOGC` ve benzerleri) koyan bir görev satırı kaydedilirken
  reddedilir, çünkü bir zamanlayıcı onun anlamını koruyamaz; saat görevin
  kendi zamanlamasına yazılır.
- **Her site kendiliğinden sınırlarının içine alınır.** Güncellemeden
  sonra panel, hâlâ sınırlarının dışında olan her siteyi önce zamanlanmış
  görevleri, sonra çalışan programlarıyla, sitelerin oluşturulma sırasıyla,
  tek tek ve yaklaşık 20 saniye arayla taşır. Güncellenen panel açıldıktan
  yaklaşık beş dakika sonra başlar; güncelleme bitmeden asla, bir panel
  güncellemesi kurulurken de asla. Basılacak bir şey yoktur. Sürerken
  yönetici Ana sayfada, **Şu anki sorunlar**'da ve sitelerin kartlarında tek
  bir satır okur: "Siteler sınırlarının içine alınıyor (3 / 12) — bir şey
  yapmanız gerekmiyor"; yalnız beklerken (panelin güncellemesini ya da
  sunucudaki başka bir değişikliği) satır bunu söyler. Bitince satır
  kaybolur ve her sitenin kartı sitenin sınırlarının içinde olduğunu
  söyler. Müşteriler ve bayiler bunların hiçbirini görmez. Bu sürümde
  oluşturulan ya da geri yüklenen bir site ilk açılışından itibaren
  sınırlarının içindedir ve görevleri baştan zamanlayıcıdır.
- **"Sınırların içine taşı" ve "Hepsini taşı" düğmeleri kaldırıldı.**
- **Bir site taşınamadığında** o site olduğu gibi çalışmaya devam eder ve
  **Şu anki sorunlar**'da ve kartında tek bir satır alır: neden, kimin
  yapacağı ve **Tekrar dene**. Panel o siteyi kendiliğinden yeniden
  denemez, sonraki güncellemelerde de denemez; böylece taşınamayan bir site
  tekrar tekrar yeniden başlatılmaz. Sınırlarının içindeyken yeniden
  dışında çalışan bir site (örneğin sınır dosyası sunucuda silindiği için)
  aynı satırı alır; hiçbir site satırsız olarak sınırlarının dışında kalmaz.
- **Siteler taşınırken panelin kendi güncellemesi yapılabilir.** Bir site
  taşınırken **Güncelle**'ye basılırsa yanıt, o sitenin taşınmasının
  yaklaşık 12 dakika içinde biteceğini ve panelin o arada başka site
  başlatmayacağını söyler; sonraki basış güncellemeyi başlatır, taşımalar
  güncellemeden sonra kendiliğinden sürer.
- **Sunucu sahibinin kendi yapılandırması korunur (algılanır, üzerine
  yazılmaz).** Sunucu sahibinin birimini değiştirdiği ya da bir ek dosyayla
  (drop-in) yerleştirdiği bir program taşınmaz; sitesinin kartı onu ve kimin
  yapacağını söyler. Sahibin kendi `/etc/cron.allow` dosyası bayt bayt
  korunur: adını taşıdığı bir hesap cron'da kalır, sitesinin kartı bunu ve
  kimin yapacağını söyler. Zamanlayıcıların aynen yapamadığı bir crontab
  satırı (adresli `MAILTO=`, yukarıdaki ayarlar, tanımadıkları bir zamanlama
  biçimi) ya da CelikPanel'de yapılmamış ve yöneticinin benimsemediği bir
  satır, sitenin bütün crontab'ını cron'da tutar: programları sınırlarının
  içine girer; kartı satırı, kimin yapacağını ve satır değiştiğinde
  kullanılacak **Tekrar dene**'yi gösterir. Sahibin CelikPanel'in
  `cron.allow` dosyasını ya da "lingering" kuralını silmesi geçerli kalır
  (**Şu anki sorunlar**, **Yeniden kapat** ile); sahibin elle düzenlediği
  bir görev dosyası korunur ve uygulanır, sahibin kapattığı bir
  zamanlayıcı kapalı kalır.
- **Bu yolları şimdiye kadar kullanmış bir müşteri için.** Bir müşterinin
  panelin dışında `crontab -e` ile eklediği satırlar, yönetici onları
  Zamanlanmış görevler ekranında benimseyene kadar siteyi cron'da tutar;
  sonra **Tekrar dene** onları taşır ve orada görev olarak görünürler. Zaten
  "lingering" açık olan bir site hesabı, çalışan hizmet yöneticisini sunucu
  sahibi onu bitirene (**Şu anki sorunlar** hesabı
  `loginctl disable-linger <hesap>` ile birlikte gösterir) ya da sunucu
  yeniden başlayana kadar korur; yeniden başlatmadan sonra o yönetici artık
  başlamaz. Değişiklik yapıldığı anda açık olan bir SFTP oturumu hizmet
  yöneticisini oturum bitene kadar korur.
- **Her şey panel olmadan da çalışmaya devam eder.** Zamanlayıcılar,
  hizmetleri, görev dosyaları, `/etc/cron.allow` ve "lingering" kuralı
  sistemin standart dosyalarıdır: görevler panel durdurulmuşken,
  kaldırılmışken ya da lisanssızken de çalışır. Panelle, yönetici bir görevi
  Zamanlanmış görevler ekranında değiştirir ya da sunucu sahibi
  `/etc/celikpanel-cron/<site no>.crontab` dosyasını düzenler ve
  zamanlayıcılar bir sonraki kayıtta ya da panelin bir sonraki açılışında
  onu izler. Panel olmadan sahip, saati
  `/etc/systemd/system/celikcron-<site no>-<n>.timer` dosyasında, komutu
  `/etc/celikpanel-cron/<site no>.d/<n>.sh` dosyasında değiştirir, sonra
  `sudo systemctl daemon-reload` çalıştırır.
- **Yönetici bir sitenin görevlerini cron'a geri koyabilir:** sitenin
  kartındaki "Görevleri cron’a geri koy" ile; böyle bir site, yönetici onu
  o karttan yeniden taşıyana kadar olduğu gibi bırakılır. Zamanlayıcısını
  sunucu sahibinin kapattığı bir görev duraklatılmış olarak geri gelir.

## Güncellemeler: güncelleme sırasında sunucunun yeniden başlatılması

- **Şimdiye kadar**, CelikPanel'in güvenlik duvarı kuralları ilk kez
  kaydedildiğinden beri yeniden başlatılmamış bir sunucuda, bir
  güncellemenin son aşamasında sunucunun yeniden başlatılması güncellemeyi
  yarım bırakabiliyordu: yeniden başlatma bu kuralları açılışta, olması
  gerektiği gibi uyguluyordu; güncelleme de durmuş olarak kaydettiği bir
  hizmet çalıştığı için devam etmeyi reddediyordu. Panel ve ajanı durmuş
  kalıyordu; ne otomatik kurtarma ne de kurtarma komutu güncellemeyi
  bitirebiliyordu; barındırılan siteler, posta ve DNS çalışmaya devam
  ediyordu.
- **Artık güncelleme böyle bir yeniden başlatmadan sonra kendiliğinden
  tamamlanır.** Güncelleme, sunucunun hangi açılışında yapıldığını kaydeder;
  yeniden başlatmadan sonra yalnız sunucunun kendi açılışının uyguladığı
  güvenlik duvarı kurallarını kabul eder (bunların elle ya da başka bir şeyle
  başlatılması yine reddedilir). Otomatik kurtarma, sunucu hâlâ açılırken
  bekler ve bu arada denemelerinden hiçbirini harcamaz. Yeniden başlatmadan
  önce kaydedilen bir süreç numarası, ancak tesadüfen eşleşebileceği
  yeniden başlatmadan sonra artık karşılaştırılmaz.
- **Bu, bu sürüme ve sonrakilere yapılan güncellemeler için geçerlidir.**
  Daha önceki bir sürüme yapılan ve bu şekilde bir yeniden başlatmanın yarım
  bıraktığı bir güncellemeyi bu sürüm bitirmez.

OPEN: önceki bir sürüme yapılan böyle bir güncellemeyi bitirmek için sunucu
sahibinin adımları buraya yalnız laboratuvarda denendikten sonra eklenir.

## Düzeltmeler

- **Başlamadan reddedilen bir güncellemeden sonra güncelleme sayfası**
  artık kilitli kalmaz: iki dakika geçtiğinde ve hiçbir kurtarma devralmamışsa
  bildirim kırmızıya döner ve kapatılabilir; kapatmak hiçbir şey başlatmaz.
  Sayfayı yenilemek ya da ikinci bir sekme iki dakikayı yeniden başlatmaz.
  İşi başlamış bir güncelleme, kurtarma hâlâ başlayabilecekken bildirimini
  eskisi gibi açık tutar.
- **Bir reddin başlığı kimin yapacağını söyler.** Bir veritabanı
  kullanıcısını kaldırmak ya da parolasını değiştirmek, kararı sunucunun
  yöneticisine ait bir nedenle reddedildiğinde, yöneticinin penceresi
  "Sunucunun yöneticisinin bir şey yapması bekleniyor" der. Müşteriler ve
  bayiler genel başlığı görmeye devam eder.
- **Kullanıcısı veritabanı sunucusunda kalan bir veritabanını silmek**
  artık veritabanının silindiğini, hangi kullanıcıların neden kaldığını ve
  sunucunun yöneticisinin yapacağını söyler; yeniden basmanın işe
  yaramayacağı yerde artık Sil'e yeniden basmayı istemez. Kullanıcıların
  yetkileri okunamadığında Sil'e yeniden basılabileceğini söyler. Bazı
  kullanıcılar hakkındaki yanıt bilinmediğinde eski yanıt kalır.
- **Taşımanın yanıtı bir programın durumunu tek ve sade bir sözcükle
  söyler**, ajanın günlüğünde ve denetim kaydında.

## Güncellemeden sonra ne beklenir

- Şema adımı yok: düzen şema 81 olarak kalır. Tamamlanmış bir güncelleme
  geri alınmaz (bkz. "alpha.149'a dönmek").
- **Güncellemenin kendisi bir müşterinin hiçbir şeyini yeniden başlatmaz
  ya da yeniden yüklemez:** ne bir programı, ne bir zamanlanmış görevi, ne
  bir PHP havuzunu, ne web sunucusunu, ne bir veritabanı oturumunu. Sürerken
  panel kullanılamaz; barındırılan siteler, posta, DNS ve veritabanları
  çalışmaya devam eder.
- **Güncellenen panel açıldıktan yaklaşık üç dakika sonra**, panelin her
  açılışından sonra çalışan bir adım kendiliğinden şunları yazar:
  - polkit kuruluysa "lingering" kuralını
    (`/etc/polkit-1/rules.d/49-celikpanel-linger.rules`): bundan sonra bir
    sitenin hesabı kendisi için "lingering"i açamaz;
  - her site hesabı için kendi hizmet yöneticisinin maskesini
    (`/etc/systemd/system/user@<uid>.service`, `/dev/null`'a bir bağlantı),
    ardından systemd'nin yapılandırmasının yeniden okunmasını: bir SFTP
    girişi yine çalışır ama hizmet yöneticisi başlatmaz; o anda açık olan
    bir oturum yöneticisini oturum bitene kadar korur;
  - sitelerin posta engeli yerindeyse engeli yeniden: bu sunucunun kendi
    gönderim hizmeti CelikPanel'in değiştirilmemiş hizmetiyse sitelerin
    programları ve görevleri artık onun üzerinden gönderebilir (bu
    sunucunun 465 ve 587 numaralı kapıları, bir posta kutusunun adı ve
    parolasıyla); değişiklik hemen yüklenir ve hiçbir şeyi yeniden
    başlatmaz.
  Bu adım bir müşterinin hiçbir şeyini yeniden başlatmaz.
- **Güncellenen panel açıldıktan yaklaşık beş dakika sonra** panel siteleri
  tek tek, yaklaşık 20 saniye arayla sınırlarının içine alır. Her site için:
  - **Önce zamanlanmış görevleri.** Site hesabının crontab'ı zamanlayıcılara
    çevrilir ve boşaltılır, hesap cron'dan çıkar: bundan sonra `crontab -e`
    ve `crontab -l` onun için reddedilir (crontab'ı olmayan bir site için
    de). Görevler saatlerini korur; bir görevin vakti tam taşıma dakikasına
    denk gelirse bir çalışması kaçabilir, hiçbiri iki kez çalışmaz. Bundan
    sonra bir görevin çıktısı çalışmasıyla birlikte sistemin günlüğünde
    tutulur (daha önce de postayla gönderilmiyordu) ve bir görevin içinde
    "Güvenlik" bölümünde sayılan farklar geçerlidir (ek yetki yok, posta bir
    posta kutusu girişiyle SMTP üzerinden).
  - **Sonra çalışan programları** (yüklenen programlar, Python
    uygulamaları, Node.js uygulaması, n8n gibi hizmet uygulamaları): **her
    biri bir kez** sitenin sınırlarının içinde yeniden başlatılır ve başlama
    süresi içinde yeniden çalışıp kapılarında yanıt vermelidir: bu programlar
    için 90 saniye (asla 20'nin altında ya da 300'ün üstünde değil). Bir
    program yeniden başlaması ne kadar sürerse o kadar kapalı kalır. Bu
    sürede ayağa kalkmayan bir program çalıştığı yere geri konur ve orada
    yeniden başlatılır (yani iki kez yeniden başlatılmış olur) ve sitesi
    **Tekrar dene** ile birlikte satırı alır. Durmuş bir program
    başlatılmaz; bir sonraki başlatılışında sınırlarının içinde başlar.
    Zaten sınırlarının içinde olan bir program (alpha.148 ya da alpha.149'da
    taşınmış) yeniden başlatılmaz.
  - Sitenin PHP havuzu, web sunucusu, veritabanları, posta ve DNS yeniden
    başlatılmaz ya da yeniden yüklenmez.
  Sınırlarının dışında N sitesi olan bir sunucu yaklaşık N × (20 saniye +
  o sitenin kendi taşınması) sonra biter: çalışan programı olmayan bir site
  birkaç saniye, programları olan bir site en yavaş programının yeniden
  başlayıp yanıt vermesi kadar sürer.
- **Crontab'ı cron'da kalan bir site** (zamanlayıcıların aynen yapamadığı
  bir satır, CelikPanel'de yapılmamış ve yöneticinin benimsemediği bir satır
  ya da sunucu sahibinin kendi `/etc/cron.allow` dosyasının adını taşıdığı
  bir hesap): programları yukarıdaki gibi taşınır ve bir kez yeniden
  başlatılır; crontab'ı ve `crontab -e` olduğu gibi kalır.

## alpha.149'a dönmek

- **Başarısız olan bir güncelleme, bu sürümün hiçbir şeyi çalışmadan önce
  kendiliğinden alpha.149'a döner.** Yeni panelin açılış denetimi yeni
  panel açılmadan önce çalışır; başarısız olursa ya da güncelleme o
  açılıştan önce herhangi bir noktada durursa güncelleme alpha.149'u ve
  veritabanını kendiliğinden geri koyar. O zaman hiçbir site taşınmamıştır:
  taşımalar ancak güncelleme bittikten sonra başlar ve panelin ajanı,
  güncelleme bitmemişken, sunucunun yeniden başlatılması da dahil, her
  taşımayı reddeder. Yalnız güncellemenin kendisi bu sürümdendir: yapıldığı
  sunucu açılışını, kaydettiği durumların yanına yazar; alpha.149 bu kaydı
  dikkate almaz.
- **Tamamlanmış bir güncelleme geri alınmaz.** Yeni panel açıldığı andan
  itibaren güncelleme sonuna kadar götürülür. Panel geri dönüş yolu sunmaz;
  sürümün `rollback.sh` betiği tamamlanmış bir güncellemeyi hiçbir şeyi
  durdurmadan önce şu satırla reddeder:
  `!! a new rollback cannot reuse an earlier transaction's recovery material; preserve the snapshot and use a supported recovery plan`;
  kurtarma komutu da yalnız yarım kalmış bir güncellemeyi bitirir.
- **alpha.149'u elle geri getirmeyin.** Bu sürümü çalıştırmış bir sunucuya
  yine de geri getirilirse:
  - taşınan programlar sınırlarının içinde kalır (alpha.149 sitenin
    sınırlarını bilir ve onları orada başlatır);
  - **taşınan her sitenin zamanlanmış görevleri, alpha.149'un Zamanlanmış
    görevler ekranının gösteremediği zamanlayıcılar olarak kalır** ve
    taşınan her sitenin hesabı için `crontab -e` kapalı kalır: alpha.149'un
    ekranı, `/etc/cron.allow` hesabı listelemediği için crontab'ın
    okunamadığını söyler ve oraya görev ekleyemez. Görevler sınırların
    içinde çalışmaya devam eder;
  - alpha.149 ilk program değişikliğinde kendi posta engelini geri yazar;
    görevler ve programlar artık bu sunucunun gönderim hizmeti üzerinden
    gönderemez;
  - "lingering" kuralı ve site hesaplarının hizmet yöneticilerinin
    maskeleri kalır;
  - bitmemiş bir görev taşıması alpha.149 tarafından unutulur (crontab
    kaybolmaz; crontab boşaltıldıktan sonra durmuş bir taşıma o sitenin
    görevlerini çalışmaz hâlde bırakır);
  - "Görevleri cron’a geri koy"un duraklatılmış olarak geri getirdiği bir
    görevin üstündeki not, alpha.149'da İngilizce olarak o görevin açıklaması
    gibi görünür;
  - panelin taşımalara ait kendi kayıtları, alpha.149'un okumadığı
    ayarlardır.
  Bir sitenin görevlerini elle cron'a geri koymak için (root olarak;
  `<id>` sitenin numarası, `<account>` hesabıdır; hesabın bir crontab'ı
  varken hiçbir şey yapmaz):
  `sudo test ! -e /var/spool/cron/crontabs/<account> && sudo test ! -e /var/spool/cron/<account> && sudo systemctl disable --now $(systemctl list-unit-files --no-legend 'celikcron-<id>-*.timer' | awk '{print $1}') && echo <account> | sudo tee -a /etc/cron.allow >/dev/null && sudo crontab -u <account> /etc/celikpanel-cron/<id>.crontab`
  (`/etc/cron.allow` yoksa hesap bunun yerine `/etc/cron.deny` dosyasından
  çıkarılır). Komut gerektirmeyen yol, bu sürüm hâlâ çalışırken her sitenin
  kartındaki "Görevleri cron’a geri koy"dur.

## Bilinen sınırlar

- **Tamamlanmış bir güncelleme geri alınmaz** (bkz. "alpha.149'a
  dönmek").
- **Yumuşak bir yeniden başlatma, yeniden başlatma sayılmaz.** Bir
  `systemctl soft-reboot` ya da ana makinesinin çekirdeğini paylaşan bir
  kapsayıcının yeniden başlatılması, çalışan çekirdeğin açılış kimliğini
  korur; son aşamasındaki bir güncelleme o zaman eskisi gibi yarım kalır.
  Güncelleme sayfası güncellemenin bittiğini söyleyene kadar sunucuyu bu
  yolla yeniden başlatmayın.
- **Bir sitenin PHP'si sınırlarının dışında çalışır.** Sitenin PHP-FPM
  havuzu ve bir PHP betiğinin başlattığı şeyler sitenin sınırlarının içinde
  değildir.
- **Sınırlarının içinde ayağa kalkmayan bir program** geri konur ve
  çalıştığı yerde yeniden başlatılır; sitesi **Tekrar dene** ile birlikte
  sınırlarının dışında kalır; kendiliğinden yeniden denenmez.
- **Meşgul bir sunucunun arkasında saatlerce bekleyen bir site** (başka bir
  değişiklik, panelin güncellemesi ya da başka bir taşıma, altı saat
  boyunca) sonsuza dek denenmek yerine **Tekrar dene** ile gösterilir.
- **Taşımalar, yöneticiyi gerektiren bir taşımayı beklerken** (bitmemiş ve
  **Şu anki sorunlar**'da kendi satırıyla gösterilen bir taşıma) ondan
  sonraki siteler de bekler.
- **Bir sitenin taşınması sırasında güncellemeye basıldığında** panel en
  çok 30 dakika boyunca ya da güncelleme başlayana kadar başka site
  başlatmaz.
- **Bir görevin çalışır bıraktığı bir süreç**, yönetici onu bitirene kadar
  (`systemctl kill celikcron-<id>-<n>.service`), görev kaldırıldıktan sonra
  da, sitenin sınırlarının içinde çalışmaya devam eder; cron altında da
  çalışmaya devam ediyordu, sınırların dışında.
- **Aynı saniyede vakti gelen çok sayıda görev birlikte başlar**, cron
  altında olduğu gibi.
- **Bir görevin içinde sunucu sahibinin oturum başına sınırları**
  (`/etc/security/limits.conf`) uygulanmaz; sitenin sınırları uygulanır.
- **Çok büyük bir crontab taşınırken** sunucudaki zamanlanmış görev
  kayıtları onu bekler.
- **Sunucu sahibinin yalnız durdurduğu** (kapatmadığı) bir zamanlayıcı,
  görevler cron'a geri konduğunda çalışan bir cron satırı olarak geri gelir.
- **Posta istisnası panelin her açılışında posta hizmetlerine göre
  belirlenir;** sahibin sonradan 465 ya da 587 numaralı kapıya koyduğu bir
  program panelin bir sonraki açılışına kadar fark edilmez; panel olmadan
  istisna en son belirlendiği gibi kalır.
- **Sunucu sahibinin sonradan oluşturduğu bir hesap**, panelin bir sonraki
  açılışından itibaren `crontab` kullanabilir; o zamana kadar ya da panel
  olmadan sahip adını `/etc/cron.allow` dosyasına ekler ya da dosyayı siler.
- **Ubuntu 22.04'te** (polkit 0.105) "lingering" kuralı okunmaz: panel orada
  "lingering"i kapatılmamış olarak bildirir.
- **Bir dizin hizmetinden gelen hesaplar** (LDAP, SSSD): `/etc/cron.allow`
  dosyasına hiçbir şey yazılmaz ve panel bunu söyler.
- **Çok büyük bir crontab** (yavaş bir sunucuda yüzlerce görev), panelin
  yanıtı beklediğinden uzun sürebilir; sitenin kartı o zaman sonucun henüz
  bilinmediğini ve yeniden basmadan önce yeniden okunmasını söyler.
- **Elle düzenlenen bir görev dosyası** zamanlayıcılara ekrandaki bir
  sonraki kayıtta ya da panelin bir sonraki açılışında ulaşır.
- alpha.149'un açık listesi, bu sürümün kapatmadığı yerde açık kalır
  ([alpha.149 sürüm notları](RELEASE-NOTES-v0.1.0-alpha.149.tr.md)).

## Laboratuvarda denenenler

OPEN: laboratuvar çalışmasından sonra doldurulur.

## Kanıt

Şema değişikliği yok: `internal/db/migrations/` altında hiçbir dosya
değişmez; düzen 81 olarak kalır. Yeni ve ekleyici: panel ayarları
`site_auto_move_v1`, `site_tasks_put_back_v1` ve `site_move_tasks_v1`; Şu
anki sorunlar türleri 48 (`site_auto_move_failed`), 49–58 (`cron.allow`,
geri gelen crontab'lar, "lingering", bekletilen görevler, site hesaplarının
hizmet yöneticileri) ve 59 (`site_auto_move_unread`); güncelleme reddi
`PANEL_UPDATE_SITE_AUTO_MOVE_RUNNING`; ajan çağrıları `MoveSiteTasksV1`,
`UndoSiteTasksMoveV1`, `SiteTasksMovedV1` ve `RebuildSiteTaskSourceV1`;
taşıma kaydının görevler bölümlü 3. sürümü; güncellemenin anlık
görüntüsündeki `service-states.boot-id` dosyası (sağlama listesinde).
Kaldırılanlar: `/api/v1/site-limits/outside` yolu ve Şu anki sorunlar'ın
sınırlarının dışındaki siteler satırı (alpha.149'un açtığı bir uyarı
kapanabilsin diye 42 numaralı tür bilinir kalır).

Bölüme göre testler: İngilizce notlardaki "Evidence" bölümünde aynı adlarla
listelenmiştir ([English](RELEASE-NOTES-v0.1.0-alpha.150.md#evidence)).
