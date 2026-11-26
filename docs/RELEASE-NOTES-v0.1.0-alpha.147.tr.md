# v0.1.0-alpha.147

[English](RELEASE-NOTES-v0.1.0-alpha.147.md)

> **Yayımlandı**, 24 Kasım 2026 (UTC): sürüm commit'i `eef8fe894`, GitHub ön sürümü 04:31:04Z, indirme portalı yaklaşık 04:35Z (dış denetim 27/27).

**Bir düzeltme de içeren bir güvenlik sürümü.** Veritabanı araçları ve web
postası artık web sunucusunun ortak hesabıyla çalışmıyor, site hesapları
sistemin `at` komutuyla iş zamanlayamıyor ve dağıtımın ortak PHP havuzu
hiçbir şey kullanmıyorsa kapatılıyor. Uzun süren bir değişiklik panelde artık
sonsuza dek "sürüyor" görünmüyor. Panelin veritabanı düzeni değişmez.
**Kurulu panelleri yalnız sahibi, panelin kendi güncelleme ekranından
günceller.**

## alpha.146'dan güncelleme

1. Panelin kendi güncelleme ekranından, eskisi gibi güncelleyin.
2. **Panelin veritabanı düzeni değişmez** (şema 81 olarak kalır). Yeni panel
   açılmadan önce başarısız olan bir güncelleme kendiliğinden alpha.146'ya
   döner. Tamamlanmış bir güncelleme geri alınmaz; bunun yerine bu sürümün
   her değişikliği alpha.147'de geri alınır; aşağıdaki "alpha.146'ya dönmek"
   bölümüne bakın.
3. Panelin ilk açılışından yaklaşık üç dakika sonra, başka bir sunucu
   değişikliği sürmüyorsa, panel kendi işinde tek bir adımı kendiliğinden
   çalıştırır: veritabanı araçlarını ve web postasını kendi hesaplarına
   taşır, dağıtımın PHP havuzunu hiçbir şey kullanmıyorsa kapatır, `at` komutunu site hesaplarına kapatır ve site
   crontab'larını kayda alır (bkz. "Güvenlik: müşteriler birbirinden daha iyi
   ayrılıyor"). Basılacak bir şey yoktur. Yapamadığı her şey **Şu anki
   sorunlar**'da, kimin ne yapacağıyla birlikte bir satırdır. Bu adım PHP'yi
   yeniden yüklerken PHP siteleri ve web postası birkaç saniye hata
   verebilir; bu anı seçmek için güncellemeyi sakin bir saatte yapın.

## Güvenlik: müşteriler birbirinden daha iyi ayrılıyor

- **Veritabanı araçları ve web postası web sunucusunun ortak hesabıyla
  çalışıyordu.** phpMyAdmin, phpPgAdmin ve web postası (Roundcube),
  dağıtımın ortak PHP havuzunda, grubu her sitenin klasörünü okuyabilen web
  sunucusu hesabıyla çalışıyordu. Birindeki bir hata her siteyi okuyabilirdi.
- **Artık her birinin kendi sistem hesabı var**: `celikpanel-webmail`,
  `celikpanel-phpmyadmin` ve `celikpanel-phppgadmin` (girişi ve ev klasörü
  yok, başka hiçbir grupta değil). Her birinin, aracın çalıştığı PHP
  sürümünde kendi PHP havuzu var; oturumları, yüklemeleri ve geçici dosyaları
  `/var/lib/celikpanel-php-tools/<araç>/` içinde. Bir aracın havuzuna yalnız
  web sunucusu ulaşır. Yeni kurulumlar bunu baştan alır; var olanlar yukarıdaki
  adımla taşınır. Taşıma, bir aracın Yönet penceresinde PHP sürümünü seçmekle
  aynı doğrulanmış adımdır: araç sonrasında kendisi olarak yanıt vermelidir;
  vermezse her şey olduğu gibi geri gelir ve **Şu anki sorunlar** bunu söyler.
  Aracın Yönet penceresinde PHP sürümünü seçmek taşımayı yeniden dener.
- **Web postasının klasörü her site hesabına kapalı.**
  `/var/lib/celikpanel-webmail` kökü, kurulumun bıraktığı gibi başka hesaplara
  kapalı kalır; içinden yalnız web postasının kendi hesabı, tek bir erişim
  listesi girdisiyle geçer (`user:celikpanel-webmail:--x`; sahip onu
  `getfacl` ile okur, `setfacl -x` ile kaldırır). Web postasının veritabanı,
  geçici dosyaları ve günlükleri onun hesabının olur; yapılandırmasını yalnız
  onun hesabı okur. Web postasının sunucudaki yerel kapısı (alpha.145'te
  CelikPanel'in kendi denetimleri dışındaki her şeye kapatıldı) PHP'sini de
  artık web postasının kendi havuzuna verir. **Bunun için web postası erişim
  listelerini destekleyen bir dosya sistemi ister:** dosya sistemi bunları
  kabul etmiyorsa web postası web sunucusu hesabında kalır, hiçbir şey
  değiştirilmez ve **Şu anki sorunlar** nedenini ve sahibin ne yapabileceğini
  (dosya sisteminin `acl` seçeneği) söyler.
- **Site hesapları artık `at` ile iş zamanlayamaz.** `at` paketi kuruluysa ve
  `/etc/at.allow` yoksa CelikPanel, bugün `at` kullanabilen her hesabı, site
  hesapları (ve bir site hesabının kimliğini paylaşan her hesap, örneğin ek
  bir FTP girişi) dışında listeleyen bir dosya yazar. Site hesaplarının
  bekleyen `at` işleri yöneticiye listelenir (numara, hesap ve zaman), asla
  silinmez. Sahibin yazdığı bir `/etc/at.allow` olduğu gibi korunur; andığı
  site hesapları yöneticiye gösterilir.
- **Bir sitenin panel dışında eklediği crontab satırları görünür.** Bir
  sitenin Zamanlanmış görevler listesinde CelikPanel'de yapılmamış bir satır
  "CelikPanel’de yapılmadı" diye işaretlenir: silinebilir; yönetici onu
  tutabilir. Agent, tutulana kadar böyle bir satırı değiştirmeyi ya da
  etkinleştirmeyi reddeder. **Şu anki sorunlar**'daki tek bir satır siteleri
  ve satır sayısını söyler.
- **Dağıtımın ortak PHP havuzu, hiçbir şey kullanmıyorsa kapatılır.** Her PHP
  sürümünün `www.conf` havuzu web sunucusu hesabıyla çalışır. Yalnız paketin
  dokunulmamış dosyasıysa, yanında başka bir havuz çalışıyorsa ve bir web
  sunucusunun yüklediği hiçbir şey PHP'yi ona vermiyorsa kapatılır (adı
  `www.conf.off-by-celikpanel` yapılır, `php-fpm -t` ile denetlenir ve
  yeniden yüklenir; biri reddederse geri konur). Şu durumlarda açık tutulur
  ve **Şu anki sorunlar** onu neyin tuttuğunu söyler: nginx yapılandırması
  soketini anıyorsa (başka bir yazılışla ya da bağlantıyla, izlenemeyen bir
  soket yoluyla, ya da CelikPanel'in kendisininki dışında değişkenle kurulan
  bir `*_pass` adresiyle de), Apache kuruluysa ve onu anan bir dosyayı
  yüklüyorsa, etkin ya da çalışan bir Apache `.htaccess` dosyalarının bir
  şeyi değiştirmesine izin veriyorsa, ya da başka bir web sunucusu (httpd,
  lighttpd, Caddy, OpenLiteSpeed) etkin ya da çalışıyorsa. Bir sürüm asla
  havuzsuz bırakılmaz: son diğer havuzu gitmeden önce havuz geri konur.
- **Sahibin kendi kararları geçerlidir.** Sahibin değiştirdiği bir aracın web
  sunucusu dosyası ya da havuzu olduğu gibi korunur. Sahip kapatılmış bir
  havuzu geri koyarsa ya da `/etc/at.allow` dosyasını silerse CelikPanel onu
  öyle bırakır ve **Şu anki sorunlar**'da tek bir düğmeyle ("Yeniden kapat")
  bir satır gösterir; düğme, panelin kendi onayından sonra yalnız o tek şeye
  dokunur. Başarılı olmadığında sonucu, panel yeniden başlasa da o satırın
  altında görünür kalır.
- **Bir site hesabının hâlâ ulaşabildiği.** Dosya sisteminde web postası
  klasörünün altında hiçbir şey. Araçların havuzlarına yalnız web sunucusu
  üzerinden. Müşteriler `*_pass` yönergesi yazamaz; ortak havuzun soketini
  zararsız bir yönergede anan müşteri o havuzu açık tutar ve bundan bir şey
  kazanmaz.

## Düzeltme: uzun süren bir değişiklik artık "sürüyor" diye kalmıyor

- **Şimdiye kadar** (alpha.145 ve alpha.146'nın bilinen sınırı), uzun süren
  bir değişiklik — bir web postası güncellemesi, bir bileşen, posta profili
  ya da Node.js kurulumu, birlikte kurulum ya da kaldırma, bir web postası
  adresi — panel agent ile bağlantısını bir an kaybettiğinde, sunucuda
  bittikten sonra da panelde "sürüyor" görünebiliyordu. Panel yeniden
  başlatılana kadar diğer sunucu değişiklikleri reddediliyordu.
- **Artık panel sunucunun ne yaptığını okur ve sonucu kendisi kaydeder.** Bir
  web postası güncellemesi ya da web postası adresi yalnız okunur; hiçbir şey
  yeniden çalıştırılmaz. Yarıda kalan bir kurulum ya da kaldırma bir kez
  daha baştan çalıştırılır; bu yine o kadar sürebilir. Bu sırada sayfa ne
  olduğunu ("sonuç sunucudan okunuyor", "sonuç kaydediliyor") ve kimsenin bir
  şey yapması gerekmediğini söyler.
- **Agent'ın tek bir geç yanıtı artık süren bir değişikliği durdurmaz**:
  değişiklik yalnız agent panelin yenilemelerini bütün süre boyunca
  yanıtsız bırakırsa durdurulur ve o zaman yukarıdaki gibi sonuçlandırılır.
- **Sonuç yine de kaydedilemezse** sayfa, başka bir değişikliğin her reddi ve
  **Şu anki sorunlar** adımları verir: agent'ı denetleyin
  (`sudo systemctl status celikpanel-agent`), sonra
  `sudo systemctl restart celikpanel-panel`. Bir kurulum yeniden
  çalıştırılırken sunucunun paket yöneticisi meşgul kalırsa değişiklik
  "başlatılamadı" yerine kendi sözleriyle ("sunucu meşgul kaldı") biter.

## Arka planda: site başına sınırlara hazırlık

Bu sürüm, site başına sınırları getiren sürümden geri dönülebilecek sürüm
olabilir: kendi sitesinin sınırlarına taşınmış programlar posta engeliyle
başlamayı sürdürür, planlar ve hesaplar burada düzenlendiğinde site
sınırlarını korur ve bu sürüm bir programı yeniden yazdığında program
sitesinin sınırlarında kalır. Site başına sınırları çalıştırmış sunucular
için bu sürümden daha geriye dönmek desteklenmez: bir sitenin sınırlarına
taşınmış programlar başlamaz. Bugünkü bir sunucuda müşterinin gördüğü hiçbir
şey değişmez: her program başlarken ve dururken, programı posta
kapılarından uzak tutan küçük yardımcı önce programın nerede çalıştığını
sistemin kendisinden okur, sonra eskisi gibi systemd'ye sorar.

## alpha.146'ya dönmek

- **Başarısız olan bir güncelleme kendiliğinden alpha.146'ya döner.** Yeni
  panelin açılış denetimi yeni panel başlatılmadan önce çalışır; bu denetim
  başarısız olursa ya da güncelleme o açılıştan önce herhangi bir yerde
  durursa, güncelleme alpha.146'yı ve veritabanını kendisi geri koyar. O
  sırada bu sürümün değişikliklerinin hiçbiri yoktur: araçları taşıyan adım
  yeni panel açıldıktan ancak yaklaşık üç dakika sonra çalışır.
- **Tamamlanmış bir güncelleme geri alınmaz.** Yeni panel başlatıldığı andan
  itibaren güncelleme, bu açılış başarısız olsa da sonuna kadar götürülür.
  Panel önceki sürüme dönüş sunmaz; sürümün `rollback.sh` dosyası da
  tamamlanmış bir güncellemeyi hiçbir şeyi durdurmadan reddeder ve şunu
  yazar:
  `!! a new rollback cannot reuse an earlier transaction's recovery material; preserve the snapshot and use a supported recovery plan`.
  Güncelleme kendi yedeğini saklar. `sudo /usr/libexec/celikpanel/recovery recover`
  yarıda kalmış bir güncellemeyi ya da geri dönüşü tamamlar; tamamlanmış bir
  güncellemeden sonra yapacağı bir şey yoktur.
- **Bu sürümün her değişikliği alpha.147'de geri alınır:**
  - Dağıtımın ortak PHP havuzu: adını geri verin ve PHP-FPM'i yeniden
    yükleyin. CelikPanel onu açık bırakır ve **Şu anki sorunlar**'da
    "Dağıtımın PHP … havuzu sunucuda geri açıldı" satırını "Yeniden kapat"
    düğmesiyle gösterir.

    ```bash
    sudo mv /etc/php/<sürüm>/fpm/pool.d/www.conf.off-by-celikpanel /etc/php/<sürüm>/fpm/pool.d/www.conf
    sudo systemctl reload php<sürüm>-fpm
    ```

  - Site hesapları için `at`: `/etc/at.allow` dosyasını silin. CelikPanel onu
    kendiliğinden yeniden yazmaz ve "Web sitesi hesapları yeniden at
    kullanabiliyor" satırını "Yeniden kapat" düğmesiyle gösterir.
  - Veritabanı araçları ve web postası kendi hesaplarında kalır: panelde
    onları web sunucusu hesabına geri koyan bir işlem yoktur (bir aracın
    Yönet penceresinde PHP sürümünü seçmek kendi hesabını korur).

## Bilinen sınırlar

- **Tamamlanmış bir güncelleme geri alınmaz** (bkz. "alpha.146'ya dönmek");
  bu sürümün değişiklikleri alpha.147'de geri alınır, veritabanı araçları ve
  web postası kendi hesaplarında kalır.
- **Adım çalışırken birkaç saniyelik hatalar.** Güncellemeden yaklaşık üç
  dakika sonra panel araçları taşır ve PHP'yi yeniden yükler; hepsi birkaç
  saniye içinde olur. O anda PHP siteleri ve web postası birkaç saniye hata
  verebilir (laboratuvarda: en çok dört saniye, birkaç istek). Bu anı
  seçmek isteyen yönetici güncellemeyi sakin bir saatte yapar.
- **Panel tam o adım biterken takılırsa** adım sunucuda tamamlanmıştır, ama
  **Şu anki sorunlar**'daki satırları ancak panelin bir sonraki açılışından
  sonra görünür.
- **Arch Linux'ta** araçlar web sunucusu hesabında kalır ve PHP havuzu
  kapatılmaz: bu sürüm onları Debian ve Ubuntu'da taşır. `at` ve site
  crontab'ları her sistemde ele alınır.
- **Bir site hesabı kendi crontab'ına hâlâ satır ekleyebilir.** Satırlar
  görünür, silinebilir ve yönetici tarafından tutulabilir; engellenmez.
- **Ortak PHP havuzu kapatılırken bakılmayanlar:** soketine doğrudan bağlanan
  programlar (örneğin işlem kodu önbelleğini sıfırlayan zamanlanmış bir iş ya
  da havuzun durum sayfasını okuyan bir izleme aracı), systemd dışında
  başlatılan bir web sunucusu ve `nginx -T`'nin yazdırmadığı Lua kodu.
  Bunlardan biri havuzu kullanıyorsa onu yukarıdaki gibi geri koyun;
  CelikPanel sonra onu açık bırakır.
- **Veritabanı araçlarının yerel sunucusu** (`127.0.0.1:8306`, panelin
  `/dbtool/` yolunun arkasında) sunucudaki her hesaba ve oturum açmış her panel
  kullanıcısına yanıt verir. Paketlerin kendi dosyalarını (kod, belgeler,
  örnek SQL; gizli bilgi yok) verir ve PHP'lerini araçların kendi hesaplarıyla
  çalıştırır; veritabanlarını araçların kendi girişleri korur.
- **Bir taşıma geri alınırken** o anda zaten gitmiş bir dosya, önemli olsa da,
  yalnız agent'ın günlüğünde bir satırdır.
- **Sahip web postasının klasörünü başka hesaplara yeniden açarsa**
  CelikPanel bunu geri almaz ve söylemez; kaldırılmış bir erişim listesi
  girdisi **Şu anki sorunlar**'da söylenir.
- **Bir sitenin hesap kimliğini paylaşan bir girişin adına yazılmış bir
  crontab** ve `systemd --user` zamanlayıcıları ele alınmaz.
- **Panelin kaydedemediği bir değişiklik**, "kaydedilemedi" diye anıldıktan
  sonra ancak panelin bir sonraki açılışında kendiliğinden sonuçlanır; o
  zamana kadar güvenlik duvarı, DNS ve posta TLS değişiklikleri yarım kalmış
  bir bileşen kurulumunun yanında kabul edilir. Bir kurulum, adımlarının
  yinelenebilir olduğu varsayımıyla baştan yeniden çalıştırılır.
- alpha.145'in açık listesi açık kalır
  ([alpha.145 sürüm notları](RELEASE-NOTES-v0.1.0-alpha.145.tr.md)).

## Laboratuvarda denenenler

Laboratuvar denetimi bitti: test sunucusu çiftinde tek bir tur; posta, kendi
adresinde web postası, phpMyAdmin ve phpPgAdmin bulunan bir Debian 13 ana
sunucu ve phpMyAdmin ile web postası bulunan bir Ubuntu 24.04 sunucu
(24 Kasım 2026, UTC 00:25'ten temizlikle birlikte yaklaşık 02:17'ye kadar).
İki sunucu da alpha.146'nın koduyla başladı ve panelin kendi güncelleme
sayfasından bu sürümün koduna güncellendi (`1c605428d`, yalnız laboratuvar
için verilen v0.1.0-alpha.155 etiketiyle). Her sunucuda bir müşterinin site
hesabı karşı tarafı oynadı. Laboratuvarın yaptığı yazanlar dışında her şey
panelin kendi sayfalarından yapıldı. Ağır bir hata yok; laboratuvarın
karşılaştığı şeyler "Bilinen sınırlar"da.

- **Güncellemeden önce** (alpha.146'nın kodu), iki sunucuda da: phpMyAdmin,
  phpPgAdmin ve web postası, web postasının yapılandırmasını ve
  veritabanını, phpMyAdmin'in gizli dosyalarını ve her sitenin dosyalarını
  okuyan web sunucusu hesabıyla çalışıyordu. Bir site hesabı web postasının
  klasörünü ya da phpMyAdmin'in gizli dosyalarını okuyamıyordu ve bir `at`
  işi zamanlayabiliyordu.
- **İki sunucu da panelin kendi güncelleme sayfasından kendiliğinden
  güncellendi**, her biri yaklaşık bir buçuk dakikada. Şema adımı yok (81
  kalır); her sunucuda bir panel ve bir agent çalıştı; başarısız hizmet yok.
  Her saniye yapılan bir denetim, iki sunucuda da güncelleme boyunca web
  postasının, veritabanı araçlarının ve bir sitenin yanıt verdiğini gördü.
- **Açılıştan yaklaşık üç dakika sonraki adım** iki sunucuda da
  kendiliğinden çalıştı: araçlar kendi hesaplarına ve PHP havuzlarına
  taşındı, web sunucusunun araçlar ve web postasının kendi adresi için
  dosyaları PHP'yi bu havuzlara verir oldu, dağıtımın havuzu kapatıldı
  (paketin dosyası yeni adıyla değişmeden duruyor) ve `/etc/at.allow`, daha
  önce `at` kullanabilen hesaplarla, hiçbir site hesabı, ek FTP girişi ya da
  araç hesabı olmadan yazıldı. Adım çalışırken o PHP sürümündeki sitelerin
  PHP istekleri en çok yaklaşık dört saniye, ana sunucuda web postası
  yaklaşık iki saniye hatayla yanıtlandı (bkz. "Bilinen sınırlar").
- **Site hesabı olarak güvenlik değişikliği**, iki sunucuda da: PHP'si ve
  zamanlanmış görevleri araçların dosyalarından yine hiçbir şey okuyamadı ve
  `at` reddedildi, sitenin hesabını paylaşan ek bir FTP girişi için de;
  sahibin kendi hesabı `at` kullanmayı sürdürdü. Araçların kendi hesapları
  bir sitenin dosyalarını okuyamadı; web sunucusu hesabı da artık web
  postasının yapılandırmasını ya da veritabanını ve phpMyAdmin'in gizli
  dosyalarını okumuyor. Web postasının yerel kapısından site hesabı giriş
  sayfasını aldı; bir eklentinin yedek kopyası ve yapılandırma dosyaları
  için "bulunamadı" yanıtı aldı.
- **Araçlar taşımadan sonra çalışıyor:** ana sunucuda bir posta kutusu web
  postasının kendi adresinde oturum açıp ekli bir ileti gönderdi (ileti kendi
  alan adı için imzalı ulaştı ve Gönderilenler'de); bir veritabanı kullanıcısı
  phpMyAdmin'de oturum açıp bir sorgu çalıştırdı. Ana sunucu yeniden
  başlatıldıktan sonra phpMyAdmin, phpPgAdmin ve web postası yanıt verdi ve
  taşımalar korundu.
- **Şu anki sorunlar**, Türkçe ve İngilizce: site hesaplarının bekleyen `at`
  işleri numara, hesap ve zamanla (komut olmadan) ve CelikPanel'de yapılmamış
  crontab satırları site ve sayılarıyla; sitenin Zamanlanmış görevler
  bölümünde laboratuvarın eklediği `@reboot` satırı diğer satırların altında,
  Kalsın ve silme ile listelendi.
- **Sahibin geri alması ve yeniden kapatma** (Ubuntu sunucu): laboratuvar
  sahip olarak `/etc/at.allow` dosyasını silip dağıtımın havuzunun adını geri
  verdikten sonra CelikPanel, panel yeniden başlatıldığında ikisini de öyle
  bıraktı ve "Web sitesi hesapları yeniden at kullanabiliyor" satırını
  "Yeniden kapat" ile, "Dağıtımın PHP 8.3 havuzu sunucuda geri açıldı"
  satırını "Yeniden kapat" ile gösterdi. Sahibin bir sitesi o havuzu hâlâ
  kullanırken havuzun "Yeniden kapat" düğmesi onu açık tuttu ve sitenin
  dosyasını andı; `at` için "Yeniden kapat" `/etc/at.allow` dosyasını aynı
  adlarla yeniden yazdı. Sahibin sitesi kaldırıldıktan sonra havuz, "Yeniden
  kapat"a basılana kadar açık kaldı; basılınca yalnız o havuz tek bir
  yeniden yüklemeyle kapatıldı; web postası, phpMyAdmin ve site yanıt
  vermeyi sürdürdü.
- **"Sürüyor" düzeltmesi:** agent 30 saniye duraklatılırken bir web postası
  güncellemesi bitti ve yeniden başlatma olmadan kendiliğinden kaydedildi;
  on saniyelik bir duraklama bir kurulumu durdurmadı; kurulumun ortasında
  agent 30 saniye duraklatılınca uzun bir kurulum kesilmedi, bir kez daha
  çalıştırıldı ve kendiliğinden kaydedildi, hiçbir zaman iki paket işlemi
  aynı anda olmadı; bir kurulum sırasında agent üç dakika durdurulunca bir
  DNS kaydı ve bir güvenlik duvarı değişikliği nedeniyle birlikte hemen
  yanıtlandı ve hiçbir şey asılı kalmadı, sayfa önce agent denetimini, sonra
  panelin yeniden başlatılmasını söyledi, başka bir kurulum başlatılamadı ve
  panel yeniden başlatıldıktan sonra kurulum bir kez daha çalışıp başarılı
  oldu; kurulumun ortasında durdurulan panel, paket yöneticisi üç dakika
  tutulurken açılınca değişiklik "sunucu meşgul kaldı" sözleriyle bitti.
- **Site başına sınırlar hazırlığı:** güvenlik duvarı var olmayan bir yeri
  iki sunucuda da reddetti, ana sunucuda güncellemeden sonra yine reddetti;
  Debian 13'te yardımcı programın kendi yerini görüyor; site başına sınırları
  olmayan bir sunucuda bir program yeniden başlatmada, öldürülmede, yeniden
  yayında ve sunucunun yeniden başlatılmasında posta engelinde kaldı, içinden
  25, 465 ve 587 numaralı kapılar reddedildi; laboratuvarın elle sitesinin
  sınırlarına koyduğu bir program engeli korudu, yeniden yayından sonra da;
  o dosya kaldırılınca eski yerine döndü.
- **Geri dönüş:** sürümün `rollback.sh` dosyası güncellemeden hemen sonra,
  Ubuntu sunucuda adımdan önce, ana sunucuda adımdan sonra, "alpha.146'ya
  dönmek" bölümündeki satırla reddedildi; hiçbir şey durdurulmadı ya da
  değiştirilmedi.
- İki sunucuda da olağan sayfalar betik hatası olmadan açıldı.

**Laboratuvarın sayfanın yolundan ayrıldığı yerler.** Laboratuvar sunucularında
root olarak laboratuvar `at` kurdu, bir sahip hesabı açtı, bir site hesabı
olarak bir `at` işi zamanladı ve crontab satırları ekledi, web postasının
eklenti klasörüne kopyalar ve bir bağlantı koydu, agent'ı ve paneli
duraklattı ve durdurdu, paket yöneticisinin kilidini tuttu, `/etc/at.allow`
dosyasını sildi, dağıtımın havuzunun adını geri verdi, onu kullanan bir sahip
sitesi yazdı, paneli yeniden başlattı, ana sunucuyu yeniden başlattı, bir
programı elle sitesinin sınırlarına koydu, `rollback.sh` çalıştırdı ve sonda
temizlik yaptı. Tur boyunca iki sunucuda da laboratuvarın kendi sertifika
yetkilisine güvenildi.

**Denenmeyenler:**

- Yeni panelin açılış denetiminin başarısız olmasıyla kendiliğinden dönüş.
- Panelin yeniden başlatılmasıyla kesilen bir taşıma (yalnız duraklatma
  denendi) ve bir kurulumun ikinci çalışması sırasında yeniden başlatma.
- Agent duraklatılırken sunucu kurulum adımları.
- İkinci bir PHP sürümü, erişim listesi olmayan bir dosya sistemi, XFS ya da
  Btrfs, Apache, lighttpd ya da ters vekil bir site gerektirenler: kendi
  hesabını alamayan bir araç, bir araç taşınırken silinen bir site, bir
  taşımada öldürülen agent, sahibin web postasının girdisini kaldırması, bir
  taşımada elektrik kesilmesi, başka yazılışların "kullanılıyor" durumları,
  yavaş bir araçla saatlik okuma.
- Site başına sınırlar hazırlığında: yük altında systemd, sitesinin
  sınırlarındaki bir programla önceki yardımcı, sınır satırları, sitesinin
  sınırlarındaki bir programla güvenlik duvarının boşaltılması ve yeniden
  başlatma, ve düz bir program dışındaki program türleri.
- Arch Linux'ta hiçbir şey denenmedi.

## Güncellemeden sonra ne beklenir

- Şema adımı yok: düzen şema 81 olarak kalır. Tamamlanmış bir güncelleme
  geri alınmaz; bu sürümün her değişikliği alpha.147'de geri alınır (bkz.
  "alpha.146'ya dönmek").
- İlk açılıştan yaklaşık üç dakika sonra panel araçları kendi hesaplarına
  taşır ve ortak havuzu hiçbir şey kullanmıyorsa kapatır: PHP-FPM ve web
  sunucusu yeniden yüklenir, hepsi birkaç saniye içinde. **O anda PHP
  siteleri ve web postası birkaç saniye hata verebilir** (laboratuvarda: en
  çok dört saniye, birkaç istek); bu anı seçmek için güncellemeyi sakin bir
  saatte yapın. `at` kurulu olan ve sahibin dosyası bulunmayan yerde
  `/etc/at.allow` belirir.
- phpMyAdmin, phpPgAdmin ve web postası eskisi gibi oturum açar ve çalışır.
  Biri yanıt vermezse **Şu anki sorunlar** bunu adımıyla birlikte söyler.
- **Şu anki sorunlar** sahibi gerektiren satırlar gösterebilir: bir şey
  kullandığı için açık tutulan bir ortak havuz (neyin kullandığını söyler),
  site hesaplarının bekleyen `at` işleri, panelde yapılmamış crontab
  satırları. Her biri kimin ne yapacağını söyler.
- Agent ile bağlantısını kaybeden uzun bir değişiklik kendiliğinden biter;
  sayfa sonuçlanırken kimsenin bir şey yapması gerekmediğini söyler.

## Kanıt

Şema değişikliği yok: `internal/db/migrations/` altındaki hiçbir dosya
değişmez; düzen 81 olarak kalır. Yeni ve eklemeli: agent çağrıları
`CloseSideDoorsV1`, `ReadSideDoorsV1` ve `KeepCronLineV1`; panel yolu
`/api/v1/metrics/side-doors/reclose` (yalnız yönetici); taşımaların, havuzun,
`at`'in ve tutulan crontab satırlarının denetim kayıtları; 22'den 38'e Şu anki
sorunlar türleri; düzeltmenin sonuçlandırma aşamaları ve iki bitiş kodu.

Bölümlere göre testler:

- Kendi hesaplarındaki araçlar: `TestASiteAccountReachesNothingUnderWebmailsTree`
  (gerçek dosyalar ve sahipler), `TestWebmailStaysOnTheWebAccountWhereTheFileSystemRefusesAccessControlLists`,
  `TestTheUndoKeepsAPathChangedSinceAndNamesIt`,
  `TestTheUndoRemovesAFileTheMoveCreatedAndSaysNothingOfOneAlreadyGone`,
  `TestAnAccessControlListOfAnySizeIsRead`, `TestInterruptedMoveTable`,
  `TestWebmailLocalServerOnItsOwnPoolKeepsAlpha145Rules`,
  `TestWebmailConfigRewriteKeepsTheOwnAccountsGroup`,
  `TestEveryToolMoveTakesTheSideDoorsLockAfterItsStep`.
- Ortak PHP havuzu: `TestTheStockPoolInUseCheckFailsOnlyInTheSafeDirection`,
  `TestOnlyApacheFilesApacheLoadsKeepTheStockPoolOn`,
  `TestAnApacheIncludeThatCannotBeFollowedKeepsThePoolOn`,
  `TestOtherWebServersKeepTheStockPoolOn`,
  `TestUnixPathsAreComparedByWhatTheyResolveTo`,
  `TestThePanelsOwnUpstreamLineDoesNotKeepTheStockPoolOn`,
  `TestThePutBackKeepsTheTurningMarkUntilThePoolIsBack`,
  `TestTheLastSiteDeletedWhileAToolMovesOffLeavesThatVersionAPool`,
  `TestForgettingAVersionNeverRewritesAnUnreadableRecord`.
- `at` ve crontab'lar: `TestACutAtAllowWriteIsCelikPanelsUnfinishedStep`,
  `TestTheAgentRefusesToEditOrEnableALineNotMadeInCelikPanel`,
  `TestADeleteWritesTheRecordWithoutTheLineBeforeTheCrontab`.
- Yöneticinin gördükleri: `TestClosingAgainRunsApartFromTheRequestOnceAndOnlyForItsDoor`,
  `TestTheCloseAgainResultSurvivesAPanelRestart`,
  `TestAReadNeverReplacesANewerAnswer`, `TestOneSlowAnswerIsNotDown`,
  `TestASideDoorsStepThatLosesTheAgentEndsAndIsNotLeftRunning`; web
  `side-doors-problems.test.mjs`, `side-doors-undo.test.mjs`.
- "Sürüyor" düzeltmesi: `TestD1…` testleri (web postası güncellemesi ve bir
  bileşen kurulumu kendiliğinden sonuçlanır, bir kayıt en çok bir kez yeniden
  çalıştırılır, yanıtsız tek bir yenileme bir adımı durdurmaz, tek başına bir
  değişiklik aynı kurala uyar); web `d1-stuck-operation.test.mjs`.
- Site başına sınırlar hazırlığı: `TestEgressPlaceOrder`,
  `TestAddFailureIsAPlaceProblemOnlyWhenItIs`, `TestProductionPlaceReads`,
  `TestGuardReaddsUnitsInTheirSitesSlice`, `TestProofAcceptsTheSitesSlice`,
  `TestUnitKeepsOnlyItsOwnSitesSliceWithItsFile`,
  `TestPlanAndAccountSavesKeepCeilingsThisReleaseDoesNotKnow`.

Dokunulan paketlerin Go test takımları ve web test takımı, temel üzerinde ve
bu sürümün kodu üzerinde adıyla aynı başarısız kümeyle çalıştı (test
makinesinin kopyasının dışarıda bıraktığı kanıt dosyalarını okuyan üç test;
CI'da geçerler) ve sürüm isteğinin her denetimi CI'da geçti. Güvenlik bölümü
dört, diğer iki bölüm ikişer bağımsız incelemeden geçti.
