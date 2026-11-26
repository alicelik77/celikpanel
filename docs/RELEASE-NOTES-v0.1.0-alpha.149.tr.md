# v0.1.0-alpha.149

[English](RELEASE-NOTES-v0.1.0-alpha.149.md)

> **Yayımlandı**, 26 Kasım 2026 (UTC): sürüm commit'i `c04064161`, GitHub ön sürümü 14:21:27Z, indirme portalı yaklaşık 14:26Z (dış denetim 27/27).

**Bir güvenlik ve düzeltme sürümü.** Panelin ajanı, yani sunucudaki
yardımcısı, kendisine hangi ad verilirse verilsin artık veritabanı
sunucusunun bir yönetici hesabını değiştirmez, veritabanı sunucusunun kendi
sistem veritabanlarından birini silmez ya da boşaltmaz. Planın veritabanı
sınırları artık sitelerin PostgreSQL kullanıcılarına ve yeni oluşturulan her
veritabanı kullanıcısına ulaşıyor. Bir sitenin programlarını sınırlarının
içine taşıma, geri konan bir program çalışmıyorsa bunu doğru söyler; panelden
başlatılan durmuş bir program sitesinin sınırlarının içinde başlar; bir site
taşınırken güncelleme başlamaz. Güncelleme artık panelin ajanının açılması
için yaklaşık bir dakika bekler. Panelin veritabanı düzeni değişmez.
**Kurulu panelleri yalnız sahibi, panelin kendi güncelleme ekranından
günceller.**

## alpha.148'den güncelleme

1. Panelin kendi güncelleme ekranından, eskisi gibi güncelleyin.
2. **Panelin veritabanı düzeni değişmez** (şema 81 olarak kalır). Yeni panel
   açılmadan önce başarısız olan bir güncelleme kendiliğinden alpha.148'e
   döner. Tamamlanmış bir güncelleme geri alınmaz; aşağıdaki "alpha.148'e
   dönmek" bölümüne bakın.
3. **Güncelleme bir müşterinin hiçbir şeyini yeniden başlatmaz ve hiçbir
   veritabanı kullanıcısını değiştirmez.** Planın veritabanı sınırları, henüz
   almamış veritabanı kullanıcılarına planın veritabanı sınırlarının bir
   sonraki değişikliğinde ulaşır; yeni bir veritabanı kullanıcısı onları
   oluşturulurken alır (bkz. "Veritabanı sınırları").
4. **Bir site sınırlarının içine taşınırken güncellemeyi başlatmayın:** panel
   artık bunu reddeder ve taşımanın penceresi bittikten sonra güncellemeye
   yeniden basmayı söyler.

## Güvenlik: panelin ajanı artık veritabanı sunucusunun yönetici hesaplarını değiştirmez

- **Şimdiye kadar panelin ajanı kendisine verilen hesap ve veritabanı
  üzerinde işlem yapıyordu.** Ajan sunucuda root olarak, veritabanı
  sunucusunda da onun kendi yöneticisi olarak çalışır. Panel ondan bir
  veritabanı kullanıcısını değiştirmesini istediğinde (yeni bir parola, bir
  askıya alma, bir silme, eklenmiş bir kullanıcının kaldırılması, sınırları)
  adın biçimini ve kısa bir ayrılmış adlar listesini denetliyor, hesabın
  neler yapabildiğine bakmıyordu: veritabanı sunucusunda yönetici yetkisi
  olan bir hesap değiştirilebiliyor, kilitlenebiliyor ya da silinebiliyordu;
  böyle bir hesabın sahibi olduğu bir veritabanına eklenen bir veritabanı
  kullanıcısı o sahibin nesneleri üzerinde yetki alıyordu. Ayrıca adı verilen
  her veritabanını, veritabanı sunucusunun kendi sistem veritabanları da
  dahil (MariaDB `mysql` ve `sys`, PostgreSQL `postgres`), siliyor ya da
  boşaltıyordu; bu da bütün hesapları siler ya da veritabanı sunucusunun
  panel için çalışmasını durdurur.
- **Artık ajan bir hesabı değiştirmeden önce veritabanı sunucusunun
  kendisine sorar,** değişiklikle aynı adımda bir kez daha: yönetici
  yetkisi olan, yönetici yetkisi veren bir rolü olan ya da veritabanı
  sunucusunun kendi verisi üzerinde yetkisi olan bir hesap reddedilir ve
  hiçbir şey değiştirilmez. Bir veritabanının sahibi de aynı kuraldan geçer;
  tek istisna, yönetici yetkisi taşımadığı sürece CelikPanel'in kendi
  veritabanı hesabıdır. Durumu okunamayan bir hesap da reddedilir. Hiçbir ajan
  çağrısı bir sistem veritabanını (MariaDB `mysql`, `information_schema`,
  `performance_schema`, `sys`; PostgreSQL `postgres`, `template0`,
  `template1`) silmez, boşaltmaz, içine yükleme yapmaz, bir kullanıcıya
  vermez, açmaz ya da kapatmaz. Bir MariaDB veritabanı, adı herhangi bir
  ana makinede ya da bir rol olarak zaten bulunan bir kullanıcıyla
  oluşturulmaz.
- **Olağan işlerde değişen: hiçbir şey.** Panelin bir site için yaptığı her
  veritabanı kullanıcısı sıradan bir hesaptır ve geçer; bir sitenin
  veritabanlarını oluşturmak, kopyalamak, geri yüklemek ve silmek eskisi gibi
  çalışır. Bir ret sayfanın dilinde neyin reddedildiğini, hiçbir şeyin
  değiştirilmediğini ve kararı sunucunun yöneticisinin verdiğini söyler;
  müşteriden hiçbir zaman bir şey yapmasını istemez.
- **Bunun neyi koruduğu, düz sözle.** PostgreSQL 16 ve sonrasında gerçek bir
  sınırdır: başkasının ele geçirdiği bir panel bile ajana bir hesabı güçlü
  hâle getirtemez ve böyle bir hesabı kullandıramaz. MariaDB'de ve PostgreSQL
  15 ve öncesinde CelikPanel'in kendi veritabanı hesabı zaten bu ağırlıkta
  yetkiler taşır; orada kural yanlışlıklara karşı korur. Bir veritabanının
  hangi siteye ait olduğunu panel denetler.
- **MariaDB'nin yerinde çalışan yerel bir MySQL sunucusu yönetilmez.**
  CelikPanel veritabanı hesaplarını yalnız MariaDB'de değiştirir: böyle bir
  sunucuda her hesap değişikliği bunu söyleyen bir cümleyle reddedilir;
  değişikliği sunucunun yöneticisi sunucunun kendisinde yapar. Kullanıcısını
  yöneticinin elle kaldırdığı bir veritabanı bundan sonra panelden
  silinebilir.

**Eklenmiş bir veritabanı kullanıcısını kaldırmak, onun yaptıklarını artık
panelin kendi veritabanı hesabına vermez.** Şimdiye kadar CelikPanel'in kendi
veritabanı hesabına ait bir PostgreSQL veritabanına eklenmiş bir kullanıcıyı
kaldırmak, o kullanıcının yaptığı tabloları, işlevleri ve öteki nesneleri bu
hesaba veriyordu; böylece bir müşterinin yazdığı kod sonradan o hesabın
yetkileriyle çalışabiliyordu. Artık böyle bir kullanıcı, o veritabanında
tabloları ya da başka nesneleri olduğu sürece kaldırılmaz: hiçbir şey
değiştirilmez; sunucunun yöneticisi önce bunları sunucuda veritabanının
kendi kullanıcısına verir (komutu kullanım kılavuzunun Veritabanları bölümü
verir), sonra kullanıcıyı panelden yeniden kaldırır. Hiçbir şeyi olmayan bir
kullanıcı eskisi gibi kaldırılır; bir sitenin kendi kullanıcısına ait bir
veritabanında kaldırılan kullanıcının yaptıkları eskisi gibi o kullanıcıya
geçer.

## Veritabanı sınırları

- **Planın veritabanı sınırları artık PostgreSQL'e ulaşıyor.** En uzun
  sorgu, en uzun boşta açık işlem, bir oturumun geçici dosyaları ve bağlantı
  sayısı artık sitelerin PostgreSQL kullanıcılarına ulaşıyor; şimdiye kadar
  PostgreSQL 16 ve sonrasında bunların hiçbiri panelin siteler için yaptığı
  kullanıcılara ulaşmıyordu, geçici dosya sınırı ise hiçbir sürümde
  ulaşmıyordu. Panelin ajanı onları veritabanı sunucusunun kendi rol
  ayarları olarak, yalnız panelin kaydettiği sıradan site kullanıcılarına
  yazar.
- **Her yeni veritabanı kullanıcısı onları alır,** hangi yolla yapılırsa
  yapılsın: Veritabanları, bir veritabanına eklenen kullanıcı, uygulamalar
  ve tek tıkla kurulan uygulamalar, bir WordPress ya da site kopyası,
  veritabanı araçlarının kopyası, Veritabanı Sunucuları ekranı ve başka bir
  PostgreSQL kümesine taşıma. Bir MariaDB kullanıcısının uzaktan erişim
  kopyası sorgu süresini taşır.
- **Sunucu sahibinin kendi değerleri korunur.** Sunucu sahibinin bir
  veritabanı kullanıcısına elle verdiği bir değer, PostgreSQL bağlantı
  sınırı da dahil, üzerine yazılmaz ve sitenin kartı onu adıyla söyler.
  Sitenin kartı, kendi sorgu ya da boşta bekleme süresi kullanıcının
  kendisinde (sunucu sahibi ya da kullanıcının kendi oturumu tarafından)
  değiştirilmiş kullanıcıları ayrıca söyler; bunlar kullanıcının
  değiştirebileceği varsayılanlardır ve CelikPanel onları yeniden dayatmaz.
- **Bir sınır bir kullanıcıya ulaşmadığında** **Şu anki sorunlar**
  kullanıcıları adıyla söyler ve nedenini her birinin kendi nedeniyle
  belirtir: panelin ajanı yanıt vermedi; veritabanı sunucusu çalışmıyor
  (sunucu sahibi onu başlatır); çalışıyor ama bağlantıyı reddetti; kullanıcı
  sıradan bir site kullanıcısı değil; veritabanı sunucusu sınırı reddetti; ya
  da henüz bilinmiyor. **Yeniden uygula**, satırın adını verdiği
  kullanıcılara sınırları arka planda bir kez yeniden verir; yeniden basmak
  aynı çalışmayı izler. Panel bu kullanıcıları her saat kendiliğinden de
  yeniden dener. Bir veritabanı sınırını değiştiren plan kaydı adımı
  gösterir ve sınırların ulaşmadığı kullanıcıları adıyla söyler.

## Site başına sınırlar

- **Taşıma doğruyu söyler.** Sınırların içine taşıma tamamlanmadığında,
  çalıştığı yerde yeniden başlamayan bir program artık geri kondu diye
  bildirilmez: taşıma kaydını tutar; sitenin kartı ve **Şu anki sorunlar**
  programın çalışmadığını, yöneticinin günlüğünü okuyacağını, sonra **Tekrar
  dene**'ye basacağını ya da onu kendi sayfasından başlatacağını ve başka
  hiçbir şeyin değiştirilmediğini söyler. systemd'nin tekrar tekrar
  başlattığı bir program çalışmıyor olarak gösterilir. Taşımanın penceresi,
  sınırların her siteye ulaşmadığını söyleyen satır ve planın önizlemesi
  nedenlerini sayfanın dilinde verir.
- **Bir program ne zaman yeniden çalışıyor sayılır.** Taşımanın kendi
  penceresinde ve **Tekrar dene**'de bir program, olması gereken yerde
  çalıştığı ve portları varsa yeniden dinlediği anda çalışıyor sayılır.
  Sonradan, kartın ya da **Şu anki sorunlar**'ın bir okumasıyla kapanan bir
  kayıt ise ancak program başlama süresi boyunca (program başkasını
  belirtmedikçe 90 saniye, en az 20) kesintisiz, çalıştığı yerde çalıştıktan
  ve portları varsa birinden dinledikten sonra kapanır.
- **Tutulan bir kayıt nasıl kapanır.** **Tekrar dene** hâlâ çalışmayanı
  geri koyar ve denetler; çalıştığı yerde çalışan ama hiçbir porttan
  dinlemeyen bir program **Tekrar dene** ile kapatılır ve adı söylenir.
  Programı kendi sayfasından başlatmak da, program başlama süresi boyunca
  çalışınca kaydı kapatır; kendiliğinden yeniden başlayan bir program da
  öyle: kartın ya da **Şu anki sorunlar**'ın bir sonraki okuması (yaklaşık
  beş dakika içinde) kaydı kapatır. Bir kayıt tutulurken hiçbir site
  taşınmaz.
- **Panelin ajanı açılırken bir sitenin programını hiçbir zaman beklemez:**
  bitmemiş bulduğu bir taşımayı kaydından birkaç kısa adımla çözer;
  programın yeniden çalışıp çalışmadığı sonradan okunur. Ajan, açılışının
  her adımının ne kadar sürdüğünü günlüğüne yazar.
- **Panelden başlatılan durmuş bir program sitesinin sınırlarının içinde
  başlar:** yüklenen bir program, bir Node.js sitesi ve bir hizmet uygulaması
  için, bu sürümün korumalı alan yardımcısı yerine konduktan sonra; çalışan
  bir program hâlâ yalnız yöneticinin taşımasıyla taşınır.
- **Kart, sitenin programlarının Ubuntu 24.04'te işlem sınırına ulaştığını
  söyler,** aynı sınır altındaki iki okuma arasında görülmüşse; oradaki
  çekirdek kaç kez olduğunu saymaz.
- **Sınırların her siteye ulaşmadığını söyleyen satır, hangi sitelerin hâlâ
  önceki sınırlara sahip olduğunu ve hangileri için bilinmediğini söyler,**
  her birini kendi nedeniyle. Karttaki önceki bir olay, olduğu sırada geçerli
  olan sınırla, bu bilinmiyorsa sınırsız gösterilir. Bir bayinin müşterisinin
  varsayılan sınırı sunucunun varsayılanı olarak adlandırılır.
- **Bir site taşınırken güncelleme yok, güncelleme çalışırken taşıma yok.**
  Panel bir taşıma sırasında güncellemeyi reddeder ve taşımanın penceresi
  bitince yeniden basmayı söyler; bir güncelleme çalışırken taşıma ve
  **Tekrar dene** nedeniyle birlikte reddedilir. Sonlandığını söylemeden
  biten bir güncellemenin kaydı, yapılışından 31 dakika sonra taşımaları
  kendiliğinden reddetmeyi bırakır.
- **Giriş sayfasının iki kez gönderilen bir giriş hakkındaki sözleri** artık
  olanı söyler: panel ikisini de kullanmaz; başka bir web sitesinin bütün alan
  adı için koyduğu bir çerez tarayıcıda kalır ve giriş yaptıktan sonra yok
  sayılır; bu yüzden uyarı çıkış yaptıktan sonra yeniden görünebilir.

## Güncellemeler

- **Güncelleme artık panelin ajanına açılması için yaklaşık bir dakika
  verir** (önce: yaklaşık on iki saniye). Sağlıklı ama yavaş açılan bir ajan
  artık güncellemeyi durdurup paneli kendiliğinden kurtarmaya bırakmaz. Bir
  dakika sonra hâlâ açılmamış bir ajan eskisi gibi güvenle durdurulur ve
  kendiliğinden kurtarma devralır. Önceki sürüme kendiliğinden dönüş de o
  sürümün ajanını aynı biçimde bekler.

## alpha.148'e dönmek

- **Başarısız olan bir güncelleme kendiliğinden alpha.148'e döner.** Yeni
  panelin açılış denetimi yeni panel başlatılmadan önce çalışır; bu denetim
  başarısız olursa ya da güncelleme o açılıştan önce herhangi bir yerde
  durursa, güncelleme alpha.148'i ve veritabanını kendisi geri koyar. O
  sırada bu sürümün değişikliklerinin hiçbiri yoktur: veritabanı adımı
  yoktur, hiçbir birimi yeniden yazmaz; veritabanı retlerini, veritabanı
  sınırlarını ve taşımanın düzeltmelerini taşıyan bu sürümün ajanı henüz
  açılmamıştır. Bu sürüme ait olan yalnız güncellemenin kendisidir: o ve
  kendiliğinden dönüşü, panelin ajanı için yaklaşık bir dakikaya kadar
  bekler.
- **Tamamlanmış bir güncelleme geri alınmaz.** Yeni panel başlatıldığı andan
  itibaren güncelleme, bu açılış başarısız olsa da ya da ajan bir dakikası
  içinde açılmasa da sonuna kadar götürülür: o zaman panel ve ajanı durmuş
  kalır, barındırılan siteler çalışmayı sürdürür ve kendiliğinden kurtarma
  aynı güncellemeyi en çok üç kez yeniden dener; ondan sonra sahibi onu,
  iletisinin adını verdiği kurtarma komutuyla tamamlar
  (`sudo /usr/libexec/celikpanel/recovery recover …`). Panel önceki sürüme
  dönüş sunmaz; sürümün `rollback.sh` dosyası da tamamlanmış bir
  güncellemeyi hiçbir şeyi durdurmadan reddeder ve şunu yazar:
  `!! a new rollback cannot reuse an earlier transaction's recovery material; preserve the snapshot and use a supported recovery plan`.
- **Bu sürümün yaptıkları alpha.149'da geri alınır:**
  - Bir veritabanı kullanıcısının sınırları: plandaki anahtarı boşaltmak
    panelin kendi yazdığı değerleri kaldırmasını sağlar. Sınırlar veritabanı
    sunucusunun kendi ayarlarıdır ve panel olmadan da geçerli kalır; birini
    elle kaldırmak için sunucu sahibi örneğin şunu çalıştırır:
    `sudo -u postgres psql -c 'ALTER ROLE "user" RESET temp_file_limit'`.
  - Retler hiçbir durum saklamaz: geri alınacak bir şey yoktur.

## Bilinen sınırlar

- **Tamamlanmış bir güncelleme geri alınmaz** (bkz. "alpha.148'e dönmek").
- **PostgreSQL'deki hesap kuralı yönetici niteliklerine ve rollere bakar.**
  Sunucu sahibinin tek tek ayarlar, tek tek yönetici işlevleri ya da bir
  veritabanı sunucusu yöneticisinin çalıştırdığı nesneler üzerinde elle
  verdiği yetkilere bakmaz.
- **Önceki sürümlerin, eklenmiş bir kullanıcı kaldırılırken CelikPanel'in
  kendi veritabanı hesabına verdiği nesneler** olduğu gibi kalır.
  Veritabanı kullanıcılarının belgesi (`docs/DATABASE-USERS.md`) yöneticiye
  bunları bir sitenin veritabanında listeleyen sorguyu ve her birini
  veritabanının kendi kullanıcısına nasıl vereceğini söyler.
- **Bir PostgreSQL veritabanına eklenen kullanıcı, veritabanının kendi
  şemalarındaki her tablo, dizi ve işlev üzerinde yetki alır;** sunucunun
  yöneticisinin oraya kurduğu şeyler de dahil (örneğin bir sitenin
  veritabanındaki bir eklenti); yönetici eklentilerini başka bir yere kurun.
- **MariaDB'nin yerindeki yerel bir MySQL sunucusu yönetilmez** (bkz.
  "Güvenlik").
- **Bir veritabanını başka bir PostgreSQL kümesine taşımak,** gereken
  rolleri kaynağın tanımladığı gibi yeniden oluşturur; orada yalnız
  veritabanı sunucusu yöneticileri reddedilir.
- **Önceki bir sürümün yazdığı bir veritabanı bağlantı sınırı,** o sınırın
  bir sonraki değişikliğinde (bir plan kaydı, başka bir plana geçiş, bir
  bayinin ya da sunucunun sınırı) panelin sayılır: değeri önceki plan
  değerine eşit olan kullanıcı yenisini alır; başka her değer sunucu
  sahibininki olarak korunur. **Yeniden uygula** ve saatlik tur bunu yapmaz;
  sunucu sahibinin elle verdiği ve önceki plan değerine eşit olan bir değer
  de panelin sayılır.
- **Sunucu sahibinin bağlantı sınırı CelikPanel'in kendi PostgreSQL'inde
  tanınır;** yöneticinin kendi hesabıyla kaydettiği bir PostgreSQL
  sunucusunda planın değeri eskisi gibi yazılır.
- **Sahibinin başka bir rolün üyesi yaptığı ya da yönetici yetkisi verdiği
  bir sitenin veritabanı kullanıcısı,** o rol ya da yetki kaldırılana veya
  kullanıcı CelikPanel'den kaldırılana kadar **Şu anki sorunlar**'da adıyla
  kalır.
- **Çok büyük bir kayıt:** sınırların on dakika içinde ulaşmadığı
  kullanıcılar **Yeniden uygula** ile birlikte "henüz bilinmiyor" olarak
  görünür.
- **Şu anki sorunlar'daki veritabanı sınırları satırı, sözleri yüklenince
  görünür;** yüklenemezlerse satır gizli kalır.
- **Askıya alma, parola değişiklikleri ve eklenen kullanıcılar** panelin
  ajanında birbirini bekler ve kendilerine ait bir süre sınırları yoktur;
  sunucu sahibinin kilitli tuttuğu bir veritabanı kullanıcısı onları
  bekletebilir. Sitenin kartı, veritabanı sınırları verilirken
  bekleyebilir.
- **Panelin kendi kayıtları sonuna kadar okunamazsa,** veritabanı sınırları
  panelin ajanı yeniden başlayana kadar reddedilir.
- **systemd'si 254'ten eski sistemlerde (Debian 12, Ubuntu 22.04) panelin
  ajanı, açılışında tekrar tekrar başarısız olursa, güncellemenin bir
  dakikası içinde daha sık yeniden başlatılır** ve güncellemenin güvenli
  duruşu öncekinden yaklaşık elli saniye sonra gelir; barındırılan siteler
  çalışmayı sürdürür.
- **Nadir bir zamanlamada bir durum okuması, sitenin programlarından biri
  kaydedilirken sitenin sınır dosyasını kaldırabilir;** sitenin kartı o zaman
  sınır dosyasının eksik olduğunu söyler ve sitenin sınırlarının bir sonraki
  değişikliği onu yeniden yazar.
- **Bir durum okuması, o sırada yeniden başlamakta olan bir program için
  "çalışmıyor" diyebilir;** program başlama süresi boyunca çalışınca sonraki
  bir okuma kaydı kapatır.
- **Başlayan, yanıt veren ve bir an sonra duran bir program** taşımanın
  penceresinde taşındı, **Tekrar dene**'de geri kondu olarak bildirilir;
  sitenin kartı sonra programın çalışmadığını gösterir.
- **Güncelleme sürerken sunucuyu yeniden başlatmak.** CelikPanel'in güvenlik
  duvarı kuralları ilk kez kaydedildiğinden beri yeniden başlatılmamış bir
  sunucuda, güncelleme sırasında sunucunun yeniden başlatılması güncellemeyi
  yarım bırakır: panel ve ajanı durmuş kalır, onu ne kendiliğinden kurtarma
  ne de kurtarma komutu tamamlar; barındırılan siteler, posta ve DNS
  çalışmayı sürdürür. Güncellemenin sayfası güncellemenin bittiğini
  söyleyene kadar sunucuyu yeniden başlatmayın.
- **Başlamadan reddedilen bir güncelleme boyunca açık kalmış bir güncelleme
  sayfası** o tarayıcıda kilitli kalabilir; başka bir tarayıcıdan giriş
  yapınca güncelleme yeniden sunulur.
- alpha.148'in açık listesi, bu sürümün kapatmadığı yerlerde açık kalır
  ([alpha.148 sürüm notları](RELEASE-NOTES-v0.1.0-alpha.148.tr.md)).

## Laboratuvarda denenenler

Laboratuvar denemesi bitti: deneme sunucusu çiftinde, bir Debian 13
sunucusu (PostgreSQL 17, MariaDB 11.8) ile bir Ubuntu 24.04 sunucusunda
(MariaDB 10.11 ve bu deneme için panelin Bileşenler sayfasından kurulan
PostgreSQL 16) tek bir çalışma: 26 Kasım 2026, UTC 07:45'ten temizlikle
birlikte yaklaşık 11:20'ye kadar. İki sunucu da alpha.148'in koduyla başladı
ve panelin kendi güncelleme sayfasından bu sürümün koduna (`578b98e6f`,
yalnız laboratuvarda kullanılan v0.1.0-alpha.158 ile v0.1.0-alpha.160
arasındaki etiketlerle) güncellendi. Güncellemeler, ret pencereleri ve
olağan sayfalar sayfaların kendisinde yapıldı; veritabanı ve sınır işlemleri
yönetici olarak, panelin sayfalarının gönderdiği biçimde gönderildi.
Laboratuvarın sunucularda elle yaptıkları aşağıda söylenir. Laboratuvarın
karşılaştıkları "Bilinen sınırlar"dadır.

- **İki sunucu da, panelin ajanı açılışında 20 saniye bekletilirken, panelin
  kendi güncelleme sayfasından kendiliğinden güncellendi** (bekleten
  laboratuvardı): güncelleme ajanı bekledi, ajan açılışından 20,2 ve 20,6
  saniye sonra yanıt verdi; sayfa yalnız güncellemenin adımlarını, sonra
  kendiliğinden "güncellendi" yazısını gösterdi; her sunucuda yaklaşık 2
  dakika 12 saniye, yönetici baştan sona oturumu açık olarak. Kurtarma
  denemesi olmadı. Şema adımı yok (81 olarak kalır); her sunucuda bir panel
  ve bir ajan; yeni başarısız hizmet yok. Bir müşterinin hiçbir şeyi yeniden
  başlatılmadı ya da yeniden yüklenmedi: her program işlemini korudu, web
  sunucusu ve PHP kendilerininkini korudu, her sunucuda güncelleme boyunca
  açık tutulan bir PostgreSQL ve bir MariaDB oturumu ayarlarıyla bağlı kaldı,
  hiçbir veritabanı kullanıcısı değiştirilmedi ve saniyede bir yapılan bir
  denetim sitelerin, posta portunun ve DNS'in baştan sona yanıt verdiğini
  gördü. Debian 13 sunucusunun, aynı kodun etiketleri arasındaki iki
  güncellemesi daha, bekletme olmadan aynı biçimde tamamlandı (ajan
  açılışından sonra saniyenin onda biri içinde yanıt verdi).
- **Başarısız olan bir güncelleme iki sunucuda da kendiliğinden alpha.148'e
  döndü.** Laboratuvar, yeni paneli açılış denetimini geçemeyen bir sürüm
  sundu ve alpha.148'in ajanını 20 saniye bekletti: dönüş onu bekledi (20,7
  ve 20,1 saniye) ve tamamlandı; sayfa kendiliğinden, güncellemenin
  tamamlanmadığını ve önceki sürümün geri konduğunu söyledi, yöneticinin
  oturumu açıktı; her program işlemini korudu ve siteler baştan sona yanıt
  verdi. Debian 13'te kendiliğinden ilk deneme, sistemin kendi paket
  güncellemesi tam o sırada başladığı için durdu; yarım dakika sonraki
  ikincisi dönüşü tamamladı.
- **Dakikası içinde açılmayan bir ajan** (Ubuntu 24.04, her açılışında 70
  saniye bekletildi): güncelleme ajanı 63 saniye sonra güvenle durdurdu;
  sayfa panelin yanıt vermediğini ve bir güncellemenin onu yeniden başlatıyor
  olabileceğini söyledi, güncellemenin başarısız olduğunu hiç söylemedi;
  kendiliğinden kurtarma her biri yaklaşık bir dakika süren üç deneme yaptı,
  sonra durdu ve sahibin komutunu adıyla verdi. Siteler, posta ve DNS bu
  süre boyunca her saniye yanıt verdi. Bekletme kaldırılınca adı verilen
  komut (`sudo /usr/libexec/celikpanel/recovery recover --retry --snapshot …`)
  güncellemeyi tamamladı.
- **Olağan veritabanı işlemleri, MariaDB'de ve PostgreSQL'de, iki sunucuda
  da:** kullanıcısıyla veritabanı oluşturmak, kullanıcı eklemek ve
  kaldırmak, parola değiştirmek, aboneliği askıya almak ve sürdürmek,
  veritabanını kullanıcısıyla silmek. Hepsi başarılı oldu; hiçbiri
  reddedilmedi.
- **MariaDB'de ret (11.8 ve 10.11).** Laboratuvar, veritabanı sunucusunun
  root'u olarak bir sitenin veritabanı kullanıcısına sırayla sunucu genelinde
  bir hak, `mysql` veritabanında bir hak, bir rol ve herhangi bir adresten
  bağlanan, sunucu genelinde hak tutan aynı adlı bir hesap verdi. Her
  seferinde parola değişikliği, kullanıcının adı verilerek ve hiçbir şeyin
  değiştirilmediği söylenerek reddedildi; kayıtlı parola değişmedi; askıya
  alma o hesabı kilitlemeden bıraktı ve bunu söyledi, aboneliğin öteki
  kullanıcıları ise her zamanki gibi kilitlendi ve açıldı; veritabanını
  silmek veritabanını kaldırdı, hesabı bıraktı ve bunu söyledi. Laboratuvar
  hakkı geri alınca her işlem tamamlandı. Kullanıcısının adı başka bir
  adresten bağlanan bir hesap olarak zaten bulunan yeni bir veritabanı,
  alınmış ad olarak reddedildi.
- **PostgreSQL'de ret (16 ve 17).** Bir sitenin veritabanı kullanıcısına
  sırayla veritabanı oluşturma hakkı, `pg_read_server_files` üyeliği, rol
  oluşturabilen bir role devralmasız üyelik ve `pg_read_server_files` ile
  biten bir üyelik zinciri verildi. Her seferinde parola değişikliği, askıya
  alma ve sınırlar o kullanıcı için adı verilerek reddedildi; veritabanını
  silmek veritabanını kaldırdı ve kullanıcıyı bıraktı; kullanıcının parolası,
  giriş hakkı ve ayarları değişmedi. Laboratuvar hakkı geri alınca her biri
  tamamlandı; böyle hakları olmayan bir kullanıcı hiç reddedilmedi.
- **Sahibi veritabanı sunucusu yöneticisi olan bir veritabanı**
  (laboratuvar, sahipleri koruyan bir geri yüklemenin yaptığı gibi, bir
  sitenin veritabanının sahibini `postgres` yaptı): kullanıcı eklemek
  `postgres` adı verilerek reddedildi; hiçbir kullanıcı oluşturulmadı ve
  hiçbir varsayılan hak yazılmadı. Laboratuvar CelikPanel'in kendi veritabanı
  hesabına `pg_read_server_files` verdiğinde de aynısı oldu: o hesabın adı
  verilerek reddedildi; laboratuvar bunu geri alınca kullanıcı eklemek ve
  kaldırmak çalıştı.
- **Nesneleri olan eklenmiş bir kullanıcıyı kaldırmak** (PostgreSQL 16 ve
  17; Veritabanı Sunucuları sayfasından oluşturulan, sahibi CelikPanel'in
  kendi veritabanı hesabı olan bir veritabanında): eklenen kullanıcı,
  sahibinin haklarıyla çalışan bir işlev ve kendi işlevini çağıran bir
  dizini olan bir tablo yaptı. Kaldırma, kaldırmanın penceresinde Türkçe ve
  İngilizce reddedildi: kullanıcının nesneleri var, hiçbir şey
  değiştirilmedi ve sunucunun yöneticisi onları kılavuzdaki komutla devreder.
  Sonrasında her nesne hâlâ eklenen kullanıcınındı, CelikPanel'in kendi
  hesabının veritabanında hiçbir nesnesi yoktu, işlev ve dizinin işlevi
  eklenen kullanıcı olarak çalıştı. Yönetici kılavuzdaki komutu çalıştırınca
  kaldırma tamamlandı ve nesneler veritabanının kendi kullanıcısının oldu.
  Hiçbir şey yapmamış eklenmiş bir kullanıcı hemen kaldırıldı. Veritabanı
  sunucusunun bütün deneme boyunca tuttuğu komut kaydında tek bir
  `REASSIGN OWNED` vardı: yöneticinin kendi komutu. Sahibi sitenin kendi
  kullanıcısı olan bir veritabanında, tablo yapmış eklenmiş bir kullanıcıyı
  kaldırmak tabloyu eskisi gibi, satırlarıyla o sahibe verdi.
- **CelikPanel'in kendi PostgreSQL hesabı,** bütün bunlardan sonra iki
  sunucuda da: süper kullanıcı değil, çoğaltma hakkı yok, hiçbir `pg_*`
  rolüne üye değil, tek tek sunucu ayarlarında hakkı yok.
- **Veritabanı sınırları PostgreSQL kullanıcılarına ulaşıyor,** iki sunucuda
  da: en uzun sorgu 5 saniye, boşta açık işlem 60 saniye, geçici dosyalar
  64 MB ve 7 bağlantı ile kaydedilen bir plan dördünü de sitenin PostgreSQL
  kullanıcılarına yazdı; böyle bir kullanıcı olarak açılan bir oturum
  `temp_file_limit` değerini 64MB olarak okudu. Ubuntu 24.04'te, alpha.148
  altında kaydedilmiş (yalnız MariaDB kullanıcısına ulaşmış olan) ve
  güncellemeden sonra 10'dan 20'ye değiştirilen bir plan bağlantı sınırı
  PostgreSQL kullanıcısına (20; önce: yok) ve MariaDB kullanıcısına ulaştı.
  Sonradan oluşturulan bir kullanıcı planın sınırlarını baştan aldı.
  Laboratuvarın başka bir role üye yaptığı bir kullanıcı dışarıda bırakıldı
  ve adı söylendi; öteki kullanıcılar değerlerini aldı.
- **Sunucu sahibinin kendi değerleri korunur,** iki sunucuda da: bir
  PostgreSQL kullanıcısına ve bir MariaDB kullanıcısına elle verilen 30
  saniyelik sorgu süresi ile elle verilen 25 bağlantılık sınır, bu sınırları
  öteki kullanıcılar için değiştiren kayıtlardan sonra da kaldı; sitenin
  kartı her birini sahibin değeri olarak adlandırdı.
- **Sınırlar verilemediğinde** (Ubuntu 24.04): PostgreSQL laboratuvarca
  durdurulmuşken kayıt, sınırların hangi kullanıcılara ulaşmadığını söyledi;
  **Şu anki sorunlar** veritabanı sunucusunun çalışmadığını ve işin sunucu
  sahibinde olduğunu söyledi; panelin ajanı durdurulmuşken yöneticiyi
  gösterdi. Her birinden sonra **Yeniden uygula** satırı kaldırdı.
  Laboratuvarın açık bir işlemiyle kilitli tutulan bir veritabanı
  kullanıcısında sınır çağrısı 5 saniye sonra vazgeçti ve kayıt bunu
  söyledi; o sırada başka bir aboneliğin askıya alınması 4 saniyede
  tamamlandı ve kullanıcının ayarları değişmedi.
- **Panelin veritabanı dosyası,** ajan onu okurken de sahibi ve erişim
  hakları değişmeden panelin kendisinin kaldı; panel yeniden başlatıldıktan
  sonra kayıtlar çalıştı.
- **Panelden başlatılan durmuş bir program** (yüklenmiş programlar, iki
  sunucu) sitesinin sınırları içinde, posta engeliyle başladı. Sunucu
  sahibinin kendi ek ayar dosyasıyla ya da elle düzenlenmiş bir birim
  dosyasıyla, sahibin koyduğu yerde başladı; dosyalar olduğu gibi kaldı ve
  kart nedenini söyledi.
- **Tamamlanmayan bir taşıma ve ajanın açılışı** (Debian 13).
  Laboratuvarın başlayamaz hâle getirdiği bir programla taşıma, programın
  başlama süresinden sonra, tamamlanmadığını ve programın çalışmadığını
  yanıtladı; kayıt tutuldu ve kart bunu söyledi. O kayıtla ajanın üç yeniden
  başlatılması: her seferinde 2,6 ile 2,9 saniye sonra yanıt verdi, kayda
  bakması saniyenin binde 14 ile 25'i kadar sürdü ve hiçbir şey başlatmadı.
  Kayıt tutulurken yapılan bir güncelleme kendiliğinden tamamlandı ve kayıt
  sonrasında da tutuluyordu. Laboratuvar programın başlamasına izin verince
  **Tekrar dene** kaydı kapattı. Dinlemeye 30 saniyede başlayan bir programla
  ajan, taşımanın 12. saniyesinde yeniden başlatıldı: bir yeniden yükleme ve
  programın bir yeniden başlatılması, ajan açılışından 1,5 saniye sonra yanıt
  verdi; kart kaydı, program 90 saniye çalışana kadar henüz bilinmiyor olarak
  tuttu, sonra kendiliğinden kapattı.
- **Güncelleme sürerken taşıma yok** (Debian 13): bir güncelleme sürerken
  taşıma ve **Tekrar dene** nedeniyle birlikte reddedildi; kartlar baştan
  sona okunabildi; güncellemeden sonra taşıma yanıt verdi.
- **Bir olayın gerçekleştiği sınır** (Debian 13): bir planın site belleği
  128 MB'a indirildi, çekirdek bunu aşan bir programı durdurdu ve kimse kartı
  okumadan varsayılan geri kondu; kart olayı 128 MB ile gösterdi, yönetici
  için de müşterinin hesabı için de.
- Güncellemelerden sonra iki sunucuda da olağan sayfalar açıldı ve
  veritabanı araçları panelden açıldı.

**Laboratuvarın sayfanın yolundan ayrıldığı yerler.** Laboratuvar, deneme
sunucularında root olarak veritabanı hakları ve rolleri verip geri aldı, bir
veritabanının sahibini değiştirdi, veritabanı değerlerini elle verdi,
veritabanı sunucusunun komut kaydını açıp kapattı, bir veritabanı
kullanıcısını kilitli tuttu, panelin ajanını açılışında bekletti, ajanı,
paneli ve PostgreSQL'i yeniden başlattı ve durdurdu, deneme programını
sitelerin klasörlerine kopyaladı ve onu başlayamaz ya da yavaş başlar hâle
getirdi, sunucu sahibinin ek ayar dosyasını yazdı ve bir birim dosyasını
düzenledi, bir güncelleme sırasında Ubuntu 24.04 sunucusunu yeniden
başlattı, kurtarma komutunu çalıştırdı, Ubuntu 24.04 sunucusunu üç kez
kayıtlı alpha.148 durumuna döndürdü ve sonunda temizlik yaptı.

**Gerçek sunucularda denenmeyenler:**

- İki MariaDB sunucusu arasında veritabanı kopyalamak (deneme sunucularında
  veritabanlarına uzaktan erişim kapalıydı).
- alpha.148'in, Veritabanı Sunucuları sayfasından oluşturulmuş bir kullanıcı
  için yazdığı bağlantı sınırı, o sınırın bir sonraki değişikliğinde.
- Durmuş bir Node.js sitesini ya da hizmet uygulamasını panelden başlatmak.
- Ubuntu 24.04'te tamamlanmayan bir taşıma ve tutulan bir kayıtla ajanın
  açılışı.
- Bir taşıma sürerken güncellemeye basmak, bir taşıma sırasında sunucunun
  yeniden başlatılması ve durup duran bir programın taşınması sırasında
  ajanın yeniden başlatılması.
- Bir kayıt tutulurken programın kendi sayfasından başlatılması; systemd'nin
  durmadan yeniden başlattığı bir program için yazılanlar; tamamlanmayan bir
  taşımadan sonra başka bir porttan, yalnız IPv6'dan dinleyen ya da hiçbir
  porttan dinlemeyen bir program.
- Sınırların her siteye ulaşmadığını söyleyen satır, sınırları bilinmeyen
  bir siteyle.
- PostgreSQL'in kendi erişim kurallarının reddettiği veritabanı sınırları;
  büyük bir kaydın ortasında panelin yeniden başlatılması; saatlik tur.
- Ubuntu 24.04'te işlem sınırı olayı, giriş sayfasının iki kez gönderilen
  giriş için yeni yazısı ve programın gereksindiğinden küçük sınırlarla bir
  başlangıç.
- alpha.148'in bu sürümün panel veritabanıyla başlatılması.
- Güncellemeden sonra web postası.
- Arch Linux'ta hiçbir şey denenmedi.

## Güncellemeden sonra ne beklenir

- Şema adımı yok: düzen şema 81 olarak kalır. Tamamlanmış bir güncelleme geri
  alınmaz (bkz. "alpha.148'e dönmek").
- **Bu sürüm, güncellemede ya da panelin ilk açılışında bir müşterinin
  hiçbir şeyini yeniden başlatmaz ve yeniden yüklemez:** hiçbir programı,
  PHP havuzunu, web sunucusunu, veritabanı oturumunu. alpha.148'den kalan bir
  taşıma kaydı ajanın açılışında yalnız okunur; onun için hiçbir program
  başlatılmaz ya da yeniden başlatılmaz. (Tek istisna: alpha.148'in, geri
  koyması önceki dosyayı yazamadığı için yeni sınır dosyası hâlâ yerinde
  bıraktığı bir taşıma; orada önceki dosya geri konur ve o taşımadan önce
  çalışan programlar, alpha.148'in de yapacağı gibi bir kez yeniden
  başlatılır.)
- **Güncellemede hiçbir veritabanı kullanıcısı değiştirilmez.** Planın
  veritabanı sınırları bir sonraki kez yazıldığında (planın veritabanı
  sınırlarının bir değişikliği, yeni bir kullanıcı, **Yeniden uygula**)
  veritabanı sunucusu onları sonradan başlayan oturumlara uygular: açık
  oturumlar kendi ayarlarını korur; yeni bir bağlantı sınırı, açık
  bağlantıları kesmeden sınırın üzerindeki yeni bağlantıları reddeder.
- **Güncellemenin kendisi:** o çalışırken panel kullanılamaz; panelin ajanı
  yavaş açılırsa öncekinden yaklaşık bir dakikaya kadar daha uzun.
  Barındırılan siteler, posta, DNS ve veritabanları baştan sona çalışmayı
  sürdürür. Güncelleme sürerken sunucuyu yeniden başlatmayın (bkz. "Bilinen
  sınırlar").
- Tutulan bir taşıma kaydını kapatan bir durum okuması sitenin önceki sınır
  dosyasını geri koyar ve systemd'nin yapılandırmasını yeniden yükler; bunun
  için hiçbir program yeniden başlatılmaz.

## Kanıt

Şema değişikliği yok: `internal/db/migrations/` altında hiçbir dosya
değişmez; düzen 81 olarak kalır. Yeni ve eklemeli olanlar: ajan çağrısı
`SetPostgreSQLRoleLimitsV1`; ret kodları `absent`, `privileged`, `member`,
`owns_objects`, `unreadable`, `system_database` ve `unsupported_engine`
(`DATABASE_ACCOUNT_REFUSED`, 409); `POST /api/v1/metrics/database-limits/apply`
yolu (yönetici) ve denetim eylemi `database.limits.apply_again`; Şu anki
sorunlar türü 47 (`database_limits_unapplied`); panel ayarları
`db_limits_unapplied_v1` ve `site_move_stopped_v1`; güncelleme reddi
`PANEL_UPDATE_SITE_MOVE_RUNNING`.

Bölüme göre testler:

- Hesap kuralı: `TestTheAccountRuleCoversEveryPrivilege`,
  `TestAnUnreadableAccountStateRefuses`,
  `TestPasswordRotationRefusesAPrivilegedAccount`,
  `TestSuspensionRefusesAPrivilegedAccount`,
  `TestDeletingADatabaseNeverDropsAPrivilegedUser`,
  `TestRemovingAnAddedUserNeverTouchesAPrivilegedAccount`,
  `TestAddingAUserRefusesADatabaseOwnedByAPrivilegedRole`,
  `TestRemovingAUserNeverHandsItsObjectsToAPrivilegedOwner`,
  `TestRemovingAUserNeverHandsAnythingToCelikPanelsOwnAccount`,
  `TestARefusalIsReadOnlyFromTheClientsOwnErrorLine`,
  `TestTheOwnerRuleOfCelikPanelsOwnAccount`,
  `TestNoAgentCallDropsOrEmptiesASystemDatabase`,
  `TestAMariaDBCreateNeverMakesAnAccountBesideOneOfTheSameName`,
  `TestMySQLInMariaDBsPlaceIsNamedNotUnreadable`; gerçek motorlarda
  `TestTheAccountGuardOnRealEngines`; web `db-account-refusal-a149`.
- Veritabanı sınırları: `TestEveryPathThatCreatesADatabaseUserAppliesItsLimits`,
  `TestANewDatabaseUserGetsEveryLimitThePlanSets`,
  `TestPGRoleLimitsOutcomes`, `TestTheOwnersPostgreSQLConnectionLimitIsKept`,
  `TestALimitEarlierReleasesWroteFollowsTheNextChange`,
  `TestApplyAgainNeverStartsTwice`, `TestTheHourlyRoundIsOneBoundedRun`,
  `TestARestartLeavesARunsUsersNotKnownYet`; gerçek bir motorda
  `TestPGRoleLimitsOnARealEngine`; web `db-limits-a149`,
  `site-limits-db-origin-a149`.
- Site başına sınırlar: `TestTheAgentsStartIsBoundedByItsReadsAndOneReload`,
  `TestTheStartsSystemdCallsShareOneBound`,
  `TestAReadClosesOnlyAProgramThatReallyRuns`,
  `TestStartingItOnItsPageSettlesTheRecord`, `TestOnePressHasOneDeadline`,
  `TestTryAgainSettlesAProgramThatRunsWithoutAPort`,
  `TestAStaleUpdateRecordNeverRefusesMovesForGood`,
  `TestStatusReadsNeverRefuseTheUpdateOrAMove`,
  `TestTheUpdateIsNeverHeldBehindAMovesRecordLock`,
  `TestAStoppedProgramStartedFromThePanelStartsInsideItsLimits`,
  `TestAReachedProcessLimitSeenByThePeak`,
  `TestTheUnappliedLineWordsEachSiteByItsReason`; web `site-limits-a149`,
  `site-limits-a149-fix2`.
- Güncellemeler: `deploy/test-bootstrap-update-contract.sh`,
  `deploy/test-finalize-pending-update-contract.sh`,
  `deploy/test-finalize-pending-rollback-contract.sh`,
  `deploy/test-abort-pre-mutation-active-update-contract.sh` (her biri kendi
  beklemesini sabitler).
