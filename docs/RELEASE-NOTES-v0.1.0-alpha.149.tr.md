# v0.1.0-alpha.149

[English](RELEASE-NOTES-v0.1.0-alpha.149.md)

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
- **Bir program ancak çalıştıktan sonra yeniden çalışıyor sayılır:**
  başlama süresi boyunca (program başkasını belirtmedikçe 90 saniye, en az
  20) kesintisiz, çalıştığı yerde ve portları varsa birinden dinleyerek.
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
- alpha.148'in açık listesi, bu sürümün kapatmadığı yerlerde açık kalır
  ([alpha.148 sürüm notları](RELEASE-NOTES-v0.1.0-alpha.148.tr.md)).

## Laboratuvarda denenenler

AÇIK.

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
  sürdürür.
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
