# v0.1.0-alpha.133

[English](RELEASE-NOTES-v0.1.0-alpha.133.md)

Küçük tasarım sürümlerinin ikincisi. Sol menüdeki **Veritabanları** sayfası
artık herkesin veritabanlarını tek görünürlük kuralıyla listeliyor; sunucunun
kendi işleri bu sayfadan çıkıp Güvenlik sayfasına ve MariaDB ile PostgreSQL
sayfalarına taşındı. Alanlar tuttukları değer kadar geniş, alan adı silinirken
neyin gidip neyin kaldığı söyleniyor ve yanlış bir kara liste uyarısı
giderildi.
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
veritabanı olduğunu yazar (`database.delete`). Yönetim hizmetinin kara liste
yanıtına geriye uyumlu Outcome, Reason ve Sublists alanları eklendi; eski
panel Listed alanını okur ve güncel hizmet bunu yalnız belgelenmiş bir
listeleme için koyar; eski hizmetin yanıtı "denetlenemedi" olarak gösterilir.
Sürüm sırası 132 → 133.
