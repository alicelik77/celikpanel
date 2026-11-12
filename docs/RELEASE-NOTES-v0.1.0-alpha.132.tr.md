# v0.1.0-alpha.132

[English](RELEASE-NOTES-v0.1.0-alpha.132.md)

Sahibin 12 Kasım 2026'da onayladığı küçük tasarım sürümlerinin ilki. Paneli
kullanan herkes için üç şey değişiyor: bir ayar yalnız Kaydet'e basınca
kaydedilir, bir alan adının sayfası on bir sekme yerine dokuz bölümdür ve
alan adı listesi düğmelerini, türleri ve durumları sözcükle söyler.
**Kurulu panelleri yalnız sahip, panelin kendi güncelleme ekranından
günceller.**

## Sizin için ne değişti

### Tek kaydetme biçimi

- **Bir ayara tıklamak sunucuda artık hiçbir şeyi değiştirmez.** Değişen
  ayar işaretlenir ("Değişti: kapalı → açık"), altta bir çubuk "1 değişiklik
  kaydedilmedi. Kaydet’e basana dek hiçbir şey değişmez." der; yanında
  **Vazgeç** ve **Kaydet** vardır. Kaydet yalnız bir şey değişince görünür;
  çalışamıyorsa nedeni yanında yazar.
- **Kaydedilmemiş değişiklikle çıkarken önce sorulur,** panelin kendi
  penceresiyle: bir bağlantı, Geri ya da İleri, alan adı sayfasının başka
  bir bölümü, Kullanıcılar'ın başka bir sekmesi. "Sayfada kal"
  değişikliklerinizi korur; "Kaydetmeden çık" onlardan vazgeçer. Tarayıcı
  sekmesini kapatırken tarayıcının kendi sorusu yine gelir.
- **Nerelerde:** alan adının sertifika ayarları (otomatik yenileme, HTTPS
  zorlama, HSTS ve süresi, posta sertifikası), WordPress otomatik küçük
  güncellemeleri, haftalık zararlı yazılım taraması, PHP havuzu, web sunucusu
  ayarları ve yönergeleri, Node.js sitesinin değişkenleri, bir programın
  ayarları ve ortamı, site türü, www yönlendirmesi, otomatik yedekler, posta
  kutusu gönderim sınırları, otomatik yanıt, sitenin diğer girişleri,
  zamanlanmış görevler, sitenin PHP ayarları ve dosya düzenleyici; yönetici
  için Sunulanlar (izinler, yazılımlar, uygulamalar), sunulan PHP ve Node.js
  sürümleri, PHP eklentileri, otomatik güncelleme denetimi, sunucunun posta
  kuralları, giden posta sınırları, alt adresleme, planların posta
  sınırları, bildirimler, servis ve bayi planı formları, sunucu sınırları ve
  parola düzeyi.
- **Kaydetmenin gerektirdiği her soru, hiçbir şey gönderilmeden önce
  sorulur.** Sunulanlar'da Kaydet bir uygulamayı sunmayı sorduğunda
  "hayır" demek hiçbir şey göndermez ve her değişikliği korur. Kayıt
  sürerken çubuk bunu söyler; kayıt bitince kapanır.
- **Geri dönmek için tek sözcük:** her pencere ve form "Vazgeç" der (bazı
  yerlerde "İptal" yazıyordu).
- **Birkaç değişiklik birden kaydedilince her birine ne olduğu söylenir:**
  neyin kaydedildiği, neyin neden reddedildiği ve neyin bilinmediği. Yanıtı
  kaybolan bir kayıt ikinci kez gönderilmez; panel önce sunucuyu yeniden
  okur, sonra kaydedilip kaydedilmediğini söyler.
- **Siz düzenlerken biri değiştirdiyse** satır sunucudaki değeri gösterir ve
  onu almayı önerir.
- **Dosya düzenleyici yazmadan önce dosyayı yeniden okur.** Dosya siz
  açtıktan sonra sunucuda değiştiyse hiçbir şey yazılmaz ve siz seçersiniz:
  sunucudakini açmak ya da sizinkini üzerine yazmak.
- Bir programı ya da yapay zekâ ve otomasyon uygulamasını kaldırmak tek bir
  pencerede sorulur: neyin gideceği, neyin kalacağı ve alan adının yazılması.

### Dokuz bölümlü alan adı sayfası

- **Genel Bakış · Site · Dosyalar · E-posta · Veritabanları · Alan adı ve
  DNS · Güvenlik · Yedekler · Gelişmiş.** Var olan her ekran duruyor, içi
  değişmeden, artık onu tutan bölümde. Birden çok parçası olan bölüm
  parçalarını "Bu sayfada:" altında sıralar.
- **Site ve Gelişmiş artık kısa:** her parça adı, ne olduğunu söyleyen tek
  bir cümle ve Aç / Kapat ile bir karttır. Geçerli olan parçalar açık
  başlar (site türü, PHP sitesinde hazır uygulamalar ve PHP, yönlendirmeler,
  zamanlanmış görevler); ötekiler kapalı başlar ve açılana dek hiçbir şey
  yüklemez. Bir parçaya giden bağlantı onu açar. Kaydedilmemiş değişiklik
  taşıyan parçayı kapatmak önce sorar.
- **Sayfadaki yer adreste durur;** yenileme, Geri ve paylaşılan bağlantı onu
  korur. Eski her bağlantı aynı ekranı açar: sunucunun kurduğu bağlantılar,
  daha önce saklanan bildirimler ve sağlık denetimlerinin düzeltmeleri.
- **Planınızda olmayan bölüm yerinde kalır,** soluk ve kilitli. Bölümün ne
  işe yaradığını, kimin ekleyebileceğini ve ne istemeniz gerektiğini söyler.
  Ekip üyeleri yine yalnız kendilerine verileni görür; sunucuda motoru
  olmayan bölüm gösterilmez.
- Başlık sitenin türünü ve alan adının durumunu sözcükle söyler ("PHP
  sitesi", "Aktif", "Askıya alındı").
- Eski sekmelerin adını anan metinler (Barındırma, Erişim, sertifika
  sekmesi …) artık bölümleri anıyor; iki dilde ve kılavuzda.

### Satır düğmeleri sözcükle

- **Alan adı listesi:** her satırda sözcükle "Yönet" ve "Siteyi aç" ile bir
  "Diğer" menüsü var; menünün son öğesi, bir çizgiden sonra kırmızıyla
  "Sil…". Menü klavyeyle ve ekran okuyucuyla çalışır. Telefonda her satır
  aynı düğmeleri taşıyan bir karttır.
- **Türler ve durumlar sözcükle:** alan adı listesinde, alan adının
  başlığında ve yöneticinin ana sayfasında. Panelin tanımadığı bir kod nötr
  bir sözcükle yazılır (yönetici kodu da görür).
- **Tek yol gösterme kutusu** (Neden · Kim yapar · Sonraki adım · Sonra);
  DNS sayfasındaki, korunan kayıtlardaki, kurulum sihirbazındaki, zararlı
  yazılım taramasındaki, WordPress araçlarındaki ve DNS yayımlama
  uyarısındaki kopyaların yerini alır.
- **Bir ret önce düz cümlesini gösterir;** sunucunun kendi sözleri kapalı bir
  "Ayrıntı" altındadır. Müşteri ve bayi onları orada yalnız bir yol, hizmet
  ya da komut içermiyorsa görür.

### Küçük düzeltmeler

- "Planınızda dışarıdan bağlantı yok" cümlesi sağlayıcınızı ya da yöneticiyi
  doğru Türkçeyle anıyor (önce "yönetici kişisinden" yazıyordu).
- Sunulanlar, sunulan sürümler ve sunulan uygulamalar: değişen satırdaki
  durum sözcüğü sunucuda şu an olanı söyler; "Değişti" işareti Kaydet'in ne
  yapacağını söyler.
- Alan adı sayfasının başlığı boyutları, liste gibi, dilinizin sayı
  biçimiyle yazar (Türkçede "3,2 KB").
- Müşteriler ve bayiler, yöneticinin çalışan bileşen işlemini artık her
  sayfada sunucuya sormuyor (sunucu her seferinde "izin yok" diyordu).

## Bilinen sınırlar

- **Dokuz bölümün içi bugünkü ekranlardır, yeniden gruplanmış hâlde.**
  Sonraki sürümlerde bölüm bölüm yeniden tasarlanır: önce Genel Bakış, alan
  adı listesi ve müşterinin ana sayfası, sonra E-posta, Veritabanları, Alan
  adı ve DNS ile Güvenlik, Site ile Dosyalar, Yedekler ile Gelişmiş.
- **Dosya düzenleyici:** denetim okuması ve yazma iki ayrı adımdır; ikisinin
  arasındaki anda sunucuda yapılan bir değişiklik görülmez. Bu aralığı
  kapatmak sunucu tarafında bir sürüm denetimi gerektirir.
- **PHP eklentileri birer birer uygulanır,** adım adım gösterilir. Ortada
  bir durma olursa öncekiler uygulanmış kalır ve sayfa onları adıyla söyler.
- **Yüklenen bir programın tür sözcüğü** alan adı listesinde programı Python
  uygulamasından henüz ayıramıyor; "Program ya da Python uygulaması" yazar.
- **"Diğer" menüsü şimdilik yalnız alan adı listesinde;** öteki listeler
  bölümlerinin yeniden tasarımıyla gelir.
- Aynı bölümün başka bir parçasına geçmek hiçbir şey sormaz, çünkü bütün
  parçalar aynı sayfada durur; başka bir bölüme geçmek sorar.
- **Alan adı ve DNS › Adlar hâlâ belge kök dizinini ve web sunucusu
  satırlarını gösteriyor;** bunlar Site'ye aittir ve o bölüm yeniden
  kurulurken taşınır.
- **Postfix sayfasındaki "başka bir sunucu üzerinden gönder" formu henüz
  yeni kaydetmeye dönüştürülmedi:** kendi Kaydet düğmesini koruyor (parola
  ve giriş denemesi gerektiriyor); E-posta sürümünde dönüştürülür.
- **Plan düzenleyici:** izin verilen en yüksek değerin üstünde bir değer
  girilince Kaydet açık kalır; sunucu onu reddeder.
- Gerçek sistemde denendi (Ubuntu 24.04, tek sunucu, laboratuvar): aday
  `ce0dcd467`, alpha.131'den panelin kendi güncelleme ekranıyla üç kez
  kuruldu; son deneme 68 saniye sürdü ve sayfa kendiliğinden geri geldi.
  Siteler, posta kutuları, veritabanları, DNS kayıtları, sertifikalar,
  zamanlanmış görevler, FTP girişleri ve bilerek değiştirilen ayarlar önce
  ve sonra aynıydı. Dokuz bölüm, eski bağlantılar, satır menüsü, kaydetme
  çubukları, çıkış sorusu, planda olmayan bölüm, dosya düzenleyicinin
  çakışma uyarısı, Sunulanlar ve PHP eklentileri tarayıcıda yönetici, bayi
  ve müşteri olarak elle denendi. Gerçek sistemde denenmeyenler: Debian ve
  Arch, iki sunuculu çift, herkese açık bir sertifika yetkilisi.
- Önceki sınırlar sürüyor (bkz. alpha.131).

## Kanıt

Geçiş yok (şema 77 kalıyor); Go, Ajan ya da saklanan biçim değişikliği yok.
Eski alan adı sayfası bağlantıları (`?tab=…`; 072 geçişinden beri
`account_notices.fix_path` içinde saklanan düzeltme bağlantıları ve sağlık
denetimlerinin `{tab, sub}` değerleri dahil) kalıcı bir eşleme tablosuyla
okunur ve yerinde yeniden yazılır; saklanan hiçbir şey değiştirilmez.
Kaydedilmemiş değişiklikler yalnız açık sayfada durur; 131'e geri dönülürse
kaydedilmiş her şey eskisi gibi okunur. Sürüm sırası 131 → 132. Web: bileşen
testleri; kaydetme çubuğunun, çıkış sorusunun (yönlendirici bağlantıları,
Geri/İleri, bölüm değişimi), dönüştürülen her ekranın ve dokuz bölümün
bağlı testleri; paket bütçeleri yükseltilmedi (kritik açılış yolu 110
KiB'nin 109,91 KiB'ı, gzip).
