# v0.1.0-alpha.133

[English](RELEASE-NOTES-v0.1.0-alpha.133.md)

Küçük tasarım sürümlerinin ikincisi. Sol menüdeki **Veritabanları** sayfası
artık herkesin veritabanlarını tek görünürlük kuralıyla listeliyor; sunucunun
kendi işleri bu sayfadan çıkıp Güvenlik sayfasına ve MariaDB ile PostgreSQL
sayfalarına taşındı. Alanlar tuttukları değer kadar geniş, alan adı silinirken
neyin gidip neyin kaldığı söyleniyor, iki sunuculu DNS çiftinde "bekliyor"
durumunda kalan alan adı silme yeniden bitiyor ve yanlış bir kara liste
uyarısı giderildi.
**Kurulu panelleri yalnız sahibi, panelin kendi güncelleme ekranından
günceller.**

## Sizin için ne değişiyor

### Veritabanları: kim neyi görebiliyorsa tek liste

- **Yönetici sunucudaki bütün veritabanlarını görür,** bayi kendi
  müşterilerininkini, müşteri kendisininkini. Önceden yönetici, müşterilerin
  veritabanları olduğu hâlde boş bir liste ("Henüz veritabanı yok") görüyordu.
- Yönetici ve bayi **Veritabanı, Sahibi, Alan adı ve Tür** sütunlarıyla tek
  liste okur. Sahibin altında "siz" ya da müşterinin hangi bayiye bağlı
  olduğu yazar. Tek arama ad, sahip ve alan adında arar; sütun başlığına
  tıklamak o sütuna göre sıralar. Telefonda her satır bir karttır. Müşteri
  veritabanlarını alan adına göre gruplanmış görür.
- **Her satır işlemlerini sözle söyler:** "Aç" alan adının Veritabanları
  bölümüne götürür; "Diğer" menüsünde **Yeni parola…** (birden çok kullanıcı
  varsa hangisi diye sorar) ve **Sil…** (neyin gidip neyin kaldığını listeler
  ve veritabanının adının yazılmasını ister) vardır. Bu listeden silmek
  **veritabanının kullanıcılarını silmez**; soru bunu ve veritabanı alan
  adının Veritabanları bölümünden silinirse onunla açılan kullanıcının da
  silindiğini söyler.
- **"Veritabanı ekle"** "Hangi alan adı için?" diye sorar ve o alan adının
  Veritabanları bölümünü formu açık olarak getirir.
- Parolalar ve veritabanının içeriği bu sayfada hiç gösterilmez. Liste her
  değişiklikten sonra ve yarım dakikada bir kendiliğinden yeniden okunur.
- Sunucuya hangi veritabanı hizmetlerinin çalıştığı sorulamadıysa yöneticiye
  tam olarak bu söylenir; asla "veritabanı hizmeti yok" denmez.

### Sunucu işleri kendi yerlerine taşındı

- **"Müşteriler veritabanlarına dışarıdan bağlanabilsin"** artık
  **Güvenlik** sayfasında, **Dışarıya açık olanlar** kartında. Yalnız Kaydet
  ile kaydedilir; kaydetmeden önceki soru açmanın ya da kapatmanın ne demek
  olduğunu söyler. Altında kurulu her veritabanı hizmeti "Hazır · bağlantı
  noktası · yalnız şifreli bağlantı" ya da "Hazır değil" der; neden, kimin
  yapacağı, sonraki adım ve tek düğme düz Türkçeyle yazar.
- **CelikPanel'in MariaDB'ye ya da PostgreSQL'e bağlandığı hesap**, Bileşenler
  altında o hizmetin sayfasında; "Diğer" menüsünde Parolayı göster, Yeni
  parola… ve Hesabı kaldır… vardır.
- Yöneticinin Veritabanları listesinin sonunda Güvenlik'teki dışarıdan
  bağlantı ayarına giden sade bir bağlantı var. Alan adının dışarıdan
  bağlantı kartı, anahtarı kendisi açmak yerine yöneticiyi oraya götürür.
- MariaDB sayfası uzaktan bağlantı için artık `bind-address = 0.0.0.0`
  önermiyor: bu, sunucuyu her ağa açar.

### Alanlar tuttukları değer kadar

Paneldeki her metin, sayı ve tarih alanı ile her açılır liste, tuttuğu değere
göre seçilmiş altı boyuttan birini taşır: bağlantı noktası ya da sayı kısa,
IP adresi ya da ad orta, sunucu adı, e-posta adresi ya da parola daha geniş,
web adresi, yol ya da komut en geniş. Telefonda hiçbir alan ekrandan taşmaz.
Bir test bunu istisnasız korur.

### Alan adı silme, DNS çiftinde artık "bekliyor"da kalmıyor

- İki sunuculu bir DNS çiftinin ana sunucusunda bir alan adını silmek
  sonsuza dek "siliniyor" durumunda kalabiliyordu. Yedek DNS sunucusunun da
  bölgeyi kaldırdığını kanıtlamak için sunucu kendi tam DNS denetimini tek
  denemede on iki kez, dar süre sınırları içinde çalıştırıyordu; yoğun bir
  sunucuda deneme hep süresini dolduruyordu, önceki bir silmeden sonra da
  önce o silmenin tek kullanımlık denetim kaydını temizlemesi gerekiyordu.
  Tam denetim artık yalnız kanıtın ona dayandığı yerlerde, dört kez
  çalışıyor; aradaki yazmalar yalnız onlara yetki verenleri denetliyor (iki
  kilit, tam deneme, eşleşme). Önceki silmenin temizliği bir kez yapılıyor.
  Bir silmenin 20–27 saniye yerine yaklaşık 9–12 saniye sürmesi bekleniyor
  (LAB DENEMESİNDEN SONRA DOĞRULANACAK). Bu değişiklikler bileşen
  testleriyle kanıtlandı; gerçek iki sunuculu denetim, henüz yapılacak olan
  lab tekrarıdır.
- Yedek sunucunun doğrulaması geciktiğinde pencere doğruyu söyler: alan adı
  bu sunucuda DNS'ten zaten kaldırıldı, açık olan yalnız yedek sunucunun
  doğrulaması, **kimsenin bir şey yapması gerekmiyor** ve CelikPanel
  kendiliğinden yeniden denetler (en çok 5 dakikada bir). Başarısız değil
  bekliyor olarak görünür; biten adımlar bitti olarak okunur ("Posta
  kaldırıldı", …). Sürenin hangi tarafta dolduğunu da söyler: bu sunucunun
  kendi denetiminde mi, yedek sunucunun yanıtında mı.
- İleti duruma göre değişir: kendiliğinden sürüyor; takıldı ve yöneticilere
  bildirildi; ya da kendiliğinden denetimler bitti. Aynı neden üst üste altı
  kez yinelenirse (yaklaşık çeyrek saat) yöneticilere neden, kimin yapacağı
  ve sonraki adımla **tek** bir bildirim gider; yeniden denetimler sürer ve
  alan adının satırındaki Diğer → Sil… yeniden gerçek bir yeniden denemedir.
  Panelin beklemeyi bıraktığı bir denetim yineleme sayılmaz; bu yüzden
  yanlış "takıldı" bildirilmez.
- **Yarım kalan denetim kaydı kendiliğinden tamamlanır.** Sunucu, yedek
  sunucuyu doğrulamak için tek kullanımlık bir denetim kaydı tutar.
  alpha.132, önceki silmenin kaydını kaldırırken yarıda kalabiliyordu (lab'da
  eski kaydın yanında boş bir kayıt dosyası kaldı) ve sonraki silmenin her
  denetimi bunu reddediyordu; ekran kimsenin bir şey yapması gerekmediğini
  söylerken o silme hiç bitmiyordu. Sonraki denetim artık böyle yarım kalmış
  bir değişikliği kendisi tamamlar ya da geri alır ve aynı denemede sürer;
  kaydı kaldırmak artık yarıda kalamayan tek bir adım. CelikPanel'in böyle bir
  kesintiye bağlayamadığı durum olduğu gibi bırakılır ve kendi durumu olarak
  söylenir: sunucu yöneticisi tek sonraki adımla adlandırılır (iki komutun
  çıktısını desteğe gönderin; dosyaları silmeyin) ve yöneticilere tek bir
  bildirim gider.
- **Bekleme iletisi kısa ve kimin yapacağını söylüyor.** Alan Adları sayfası
  bekleyen silmeyi tek bir kutu olarak gösterir: ne bekliyor, tek cümleyle
  neden, kim yapar ("Sizin bir şey yapmanız gerekmiyor" yalnız CelikPanel'in
  kendi denetimleri bunu bitirebiliyorken), sonraki denetimin saati ve
  sayfanın kendi "Silme durumunu kontrol et" düğmesi; yöneticinin komutu ve
  teknik açıklama kapalı "Ayrıntılar" altındadır. Silme penceresi ve bildirim
  aynı cümleleri kullanır ve aynı durumu izler (CelikPanel kendiliğinden mi
  denetliyor, yöneticilere mi bildirildi, bir kişi mi yapmalı); yalnız okuyanın
  sahip olduğu düğmeleri adlandırır ve pencere komutlar için kutunun
  Ayrıntılar'ına yönlendirir.

### Diğer düzeltmeler

- **Alan adı silinirken** neyin gittiği (web sitesi ve dosyaları, ek FTP
  girişleri, posta kutuları ve yönlendirmeler, CelikPanel'in DNS kayıtları,
  sertifikalar, yedekleme zamanlaması) ve neyin kaldığı (veritabanları
  veritabanı sunucusunda kalır ama panelden çıkar; alınmış yedekler kalır ama
  panelden çıkar; saklanan posta kenara taşınır; başka DNS sağlayıcısındaki
  kayıtlara dokunulmaz) listelenir ve alan adının adının yazılması istenir.
- **Plan sınırları:** aralığı dışındaki değer Kaydet'in yanında adıyla
  söylenir; düzeltilene dek Kaydet kapalı kalır (önceden açık kalıyor, sunucu
  reddediyordu). Bildirim düzeyleri (%50–95) yazılanı korur. Bir anahtarın
  kendi üst sınırını aşan değer o sınırı söyler, asla "en çok sınır yok"
  demez.
- **"Postayı başka sunucu üzerinden gönder" (Postfix):** kendi Kaydet düğmesi
  olan son form da artık her ekran gibi kaydeder: değişiklik işaretlenir,
  kaydetme çubuğu ve ayrılma sorusu vardır. "Bağlantıyı dene" yazılan
  değerleri dener, hiçbir şey kaydetmez ve göndermez, nerede durduğunu
  söyler: ulaşılamadı, posta sunucusu gibi yanıt vermedi, şifreli bağlantı
  kurulamadı ya da kullanıcı adı veya parola kabul edilmedi.
- **Kara liste denetimi:** bir liste soruyu reddettiğinde panel artık
  "listede" demiyor. Spamhaus, herkese açık bir DNS sunucusu üzerinden gelen
  soruları yanıtlamıyor ve bu ret "listede" diye okunuyordu. Her listenin
  yanıtı artık belgelediği kodlara göre okunur: listede, listede değil ya da
  nedeniyle birlikte **denetlenemedi**. Sorulamayan liste, posta kartını ve
  alan adını hiç boyamayan gri bir satırdır. Politika kaydı (Spamhaus PBL)
  politika kaydı olarak söylenir. 2024'te kapanan dnsbl.sorbs.net artık
  sorulmuyor.
- **Program ya da hizmet uygulaması kolay bulunur:** alan adının Site
  bölümünde sitenin kendisi olan parça ("Programlar" ya da "Yapay zekâ ve
  otomasyon") en başta ve açık gelir; Genel Bakış'taki Site kutucuğu orada ne
  çalıştığını söyler ve doğrudan oraya götürür.
- Türkçede parola için her yerde "Parola", yeniden deneme için "Tekrar dene"
  denir; telefonda Alan adları sayfasının başlığı plan kartlarından önce
  gelir; bildirimin kapatma düğmesi sizin dilinizde adlandırılır.
- **Güncelleme penceresi güncellemeyi sonuna dek izler;** güncelleme
  sırasında sayfa bir an bekletilse bile (bu tarayıcı daha önce başarılı bir
  güncelleme yaptıysa ikinci adımda kalıyordu). Hiçbir şey değişmeden
  reddedilen bir güncelleme isteği geride başarısız bir hizmet birimi
  bırakmaz.
- **Silinmeyi bekleyen bir alan adı yalnız silme durumunu gösterir:**
  "yapılandırma dosyası yok" rozeti çıkmaz, disk kullanımı ölçülmez ve ana
  sayfanın sertifika listesinde yer almaz.

### alpha.132'den güncellerken

Bu güncelleme sırasında tarayıcı hâlâ alpha.132'nin kendi sayfasını
çalıştırır; bu yüzden sunucu bitirdiği hâlde güncelleme penceresi ikinci
adımında kalabilir. Sonucu görmek için sayfayı yenileyin. alpha.133'ten
itibaren pencere kendiliğinden devam eder.

## Bilinen sınırlar

- **Site yedekleri bir programın dosyalarını ve verisini içermez.** Bir
  sitenin yedeği web klasörünü (tam yedekte veritabanlarını da) tutar;
  yüklenen bir programın ya da Python uygulamasının çalıştığı klasörü ve
  orada tuttuğu veriyi tutmaz. Bu, sonraki sürümde düzeltilecek. O zamana
  dek: alan adının zamanlanmış **tam** yedeği veritabanlarını kapsar;
  programın klasörünün kendi kopyanızı saklayın.
- **Alan adını silmek, panelin o alan adının veritabanlarına ait kayıtlarını
  kaldırır ama veritabanlarının kendisini sunucuda bırakır.** Silme sorusu
  bunu söyler; onların da gitmesini istiyorsanız önce veritabanlarını silin.
  Bu soruda bir seçim sonraki sürümde gelecek.
- **Veritabanları listesinden bir veritabanını silmek kullanıcılarını
  silmez;** kullanıcılar veritabanı sunucusunda kalır. Panel bir kullanıcıyı
  yalnız erişebildiği bir veritabanının altında gösterir; onunla açılan
  kullanıcının da gitmesi gerekiyorsa veritabanını alan adının Veritabanları
  bölümünden silin.
- **Kapladığı yer sütunu** Veritabanları listesinde yok; bütün liste için
  boyutlar tek seferde okunabildiğinde gelecek.
- **Sertifika ekranı posta adının kapsanıp kapsanmadığını henüz
  söylemiyor.**
- alpha.132'de bu şekilde takılan bir silme, önceki silmenin denetim kaydı
  ister sağlam ister yarım kaldırılmış olsun, güncellemeden birkaç dakika
  sonra kendiliğinden biter; yeter ki o önceki silmenin kaydı hâlâ tutuluyor
  olsun (son 128 işlem tutulur). Kayıt düşmüşse silme artık sunucu
  yöneticisinin yapacağını söyler (iki komutun çıktısını desteğe gönderin);
  bunun için henüz bir düğme yok.
- **CelikPanel'in kanıtlayamadığı ("çözülemedi") bir denetim kaydı için
  panelde henüz bir işlem yok.** Ekran sunucu yöneticisini ve çıktısı
  desteğe gönderilecek iki salt-okur komutu söyler; hiçbir şey silinmez ya da
  kendiliğinden yeniden denetlenmez. Sahip için, adın yazılmasıyla onaylanan
  bir işlem planlandı. İşi son 128 tutulan işlemin dışına düşmüş önceki bir
  silme bu duruma girer.
- Güncelleyicinin kendisi çalışmadan önce başarısız olan bir güncelleme
  işçisi, başarısız işaretini sonraki başarılı bir güncellemeye dek korur.
- Alan adı sayfasının Gelişmiş bölümü, silinmeyi bekleyen bir alan adı için
  hâlâ "Yeniden oluştur" önerebilir.
- DNS çiftinde bir silme, iki deneme arasında sunucuda CelikPanel dışında
  başka bir bölge eklenir ya da kaldırılırsa da kendiliğinden bir çıkış yolu
  olmadan bekler (bu sürümden önce de olabiliyordu). Denetim kaydının
  yazılması ya da kaldırılması sırasında durdurulan bir süreç artık böyle bir
  durum değil; bir yazmadan hemen sonra, veri diske ulaşmadan elektrik
  kesilirse sonraki denetimin bağlayamadığı bir dosya yine kalabilir ve o
  zaman sunucu yöneticisi adlandırılır.
- Bir silmenin diğer bekleme nedenleri (örneğin iki sunucu arasında
  kurulmamış inceleme erişimi) önceki, daha uzun metinlerini göstermeyi
  sürdürür; bunların bazıları "Bu silme işlemini tekrar dene" der; o düğme,
  alan adının satırındaki Diğer → Sil… işlemidir.
- CelikPanel dışında yapılan bir DNS değişikliğinin geri tuttuğu bir yeniden
  denetim, yönetim hizmetinin süre kirasını yine aşabilir; o zaman sonucu
  sayılmaz ve bir sonraki yeniden denetim karar verir.
- CelikPanel dışında yapılan bir DNS değişikliği kanıtın son bir-üç
  saniyesine denk gelirse sonradan bulunur: silme bekler ve yeniden denenir,
  asla bitti diye bildirilmez.
- Ekip üyesinin Veritabanları listesi boştur; veritabanları, eskisi gibi
  kendisine verilen alan adlarının sayfalarındadır.
- LAB DENEMESİNDEN SONRA DOĞRULANACAK: gerçek sistem denetimi (alpha.132'den
  panelin kendi güncelleme ekranıyla güncelleme; elle neyin, hangi rollerle
  denendiği; neyin denetlenmediği).
- Önceki sınırlar sürüyor (bkz. alpha.132).

## Kanıt

Geçiş yok (şema 77 kalıyor). Yeni uç nokta `GET /api/v1/databases/visible`
(yalnız adlar, sahipler, alan adları, motorlar ve kullanıcı adları; asla
parola, bağlantı sırrı ya da içerik). Veritabanı silmek ve veritabanı
kullanıcısına parola vermek artık çağıranın ilk aboneliğine göre değil,
veritabanı sahibinin görünürlüğüne göre yetkilendirilir; olmayan ve başkasına
ait kayıt aynı yanıtı alır; iki denetim satırı da kimin yaptığını ve kimin
veritabanı olduğunu yazar (`database.delete`). Takılan silme düzeltmesi
hiçbir saklanan biçimi değiştirmez (günlük eki ve iş kaydı sürüm 1 kalır; yeni
neden kodları `dns_peer_proof_timeout:local_check` / `:peer_exchange` ve
`dns_peer_journal_unresolved` mevcut hata kodu alanını kullanır, eski
okuyucular bunları bilinmeyen sayar). Denetim kaydının içeriği değil dosya
adımları değişti: kaldırma, özel bir ada tek bir yeniden adlandırmadır ve
Agent, önceki bir sürümün yarım bıraktığını tamamlar ya da geri alır
(RESILIENCE-CONTRACT, "Branch a133/dnsproof4"). Silme durumu isteğe bağlı
`follow_up` ve `next_check_at` alanlarını kazanır. Yönetim hizmetinin kara liste
yanıtına geriye uyumlu Outcome, Reason ve Sublists alanları eklendi; eski
panel Listed alanını okur ve güncel hizmet bunu yalnız belgelenmiş bir
listeleme için koyar; eski hizmetin yanıtı "denetlenemedi" olarak gösterilir.
Sürüm sırası 132 → 133.
