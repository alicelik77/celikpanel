# v0.1.0-alpha.139

[English](RELEASE-NOTES-v0.1.0-alpha.139.md)

İki işlik küçük bir sürüm; ikisi de alpha.135 laboratuvarından. Bir çiftin
yedek DNS sunucusu ana DNS sunucusundan kendi ana makine kayıtlarından birini
eklemesini istediğinde, ana sunucu artık ne yaptığını zamanında ve doğru
söylüyor. Bunun üzerine yedek DNS sunucusundaki web postası adres
denetiminde **Kopyala**'nın yanına **Ekle…** geliyor; ana DNS sunucusundan web
postasının kaydını eklemesini istiyor.
**Kurulu panelleri yalnız sahip, panelin kendi güncelleme ekranından
günceller.**

## alpha.138'den güncelleme

1. Her zamanki gibi panelin kendi güncelleme ekranından güncelleyin. Bu sürüm
   panelin veritabanı düzenini değiştirmez (alpha.138 biçiminde kalır); bu
   yüzden daha sonra alpha.138'e dönmek veritabanı geri yüklemesi gerektirmez.
2. Güncellemeden sonra basılacak ya da ayarlanacak bir şey yok. Bir çiftin
   iki sunucusunu da elinize geldiğinde güncelleyin; aşağıdaki her yanıt
   hâlâ alpha.138 çalıştıran bir sunucuyla da çalışır ("Eski sürümdeki bir
   sunucuyla" bölümüne bakın).

## Sizin için ne değişiyor

### Ana DNS sunucusu ana makine kaydı isteğine doğru yanıt veriyor

Bir çiftin yedek DNS sunucusu, yetkili DNS bağlantıları üzerinden ana DNS
sunucusundan kendi adlarından birini (örneğin posta adını) ana alan adının
bölgesine eklemesini isteyebilir. Yedek sunucudaki sunucu kurulumu bunu yapar;
aşağıdaki **Ekle…** de aynı isteği kullanır. Bu, iki sunucu arasındaki olağan
bölge aktarımının yerini asla almaz; yalnız bir kayıt ekler.

alpha.135 laboratuvarında bu iki kez yanlış gitti: ana sunucu böyle bir kaydı
tutmadığı hâlde adın zaten kullanıldığını söyledi, bir kez de bir alan adını
silmekle meşgulken hiç yanıt vermedi. Şimdi:

- **Ana alan adının altındaki bir müşteri alan adı artık engel değil.** Ana
  alan adı `example.com`'un yanında `shop.example.com` gibi bir müşteri alan
  adı her adı reddettirmeye yetiyordu. Artık yalnız istenen adı gerçekten
  tutan bir bölge onu reddettirir ve yanıt bunu söyler.
- **Her ret ne olduğunu söylüyor.** "Bu adı başka bir kayıt kullanıyor" artık
  tam olarak bunu anlatır (başka bir kayıt, araya giren bir takma ad, kapalı
  bir kayıt ya da ana sunucuda elle değiştirilmiş veya silinmiş bu kayıt).
  Bölgenin kendisi kaydı alamadığında (örneğin DNSSEC ile imzalıysa ya da ana
  sunucu bölgenin kendisine ait olduğunu doğrulayamıyorsa) yanıt, bölgenin ana
  sunucunun yöneticisi tarafından gözden geçirilmesi gerektiğini söyler.
- **Meşgul ana sunucu bunu hemen söylüyor.** Ana sunucu başka bir DNS
  değişikliğinin bitmesini en çok üç saniye bekler, sonra "meşgul, hiçbir şey
  eklenmedi" der. Yeniden sormak güvenlidir: kayıt asla iki kez eklenmez.
  Yedek sunucudaki sunucu kurulumu iki dakika sonra kendiliğinden yeniden
  sorar.
- **Her yanıt ana sunucuda yazılı kalıyor.** Hiçbir şey eklemeyen her istek,
  nedeniyle birlikte panelin günlüğünde bir satır ve etkinlik kaydında bir
  satır bırakır; isteyen sunucu beklemeyi bırakmış olsa da.

### Yedek DNS sunucusunda web postasının adresi için "Ekle…"

Bir çiftin yedek DNS sunucusunda Bileşenler → Roundcube → Web postası adresi
→ **Denetle**, web postasının adının gerektirdiği kaydı gösterir. Ad ana alan
adının içindeyse ve ana sunucuya DNS bağlantısı hazırsa **Kopyala**'nın
yanında **Ekle…** görünür. Başka her yerde **Kopyala** eskisi gibi kalır.

- **Ekle…** önce istenecek kaydı tam olarak gösterir. Onayladıktan sonra
  sayfa kilitlenir ve adımları gösterir: ana sunucuya soruluyor, kayıt orada
  yazılıyor, genel DNS kaydı gösteriyor.
- **Eklendi** (ya da zaten vardı): sayfa sonra genel DNS'i kendiliğinden 20
  saniyede bir, en çok beş dakika okur.
- **Ana sunucu meşgul:** hiçbir şey eklenmedi. **Yeniden sor…** aynı soruyu
  yeniden açar; yeniden sormak kaydı asla iki kez eklemez.
- **10 saniye içinde yanıt yok:** ana sunucu kaydı hâlâ ekliyor olabilir.
  Sayfa 25 saniye bekler ve kendiliğinden bir kez daha sorar; kayıt o zaman
  oradaysa eklendi olarak biter. Hâlâ yanıt yoksa sonucun bilinmediğini söyler
  ve **Yeniden sor…** sunar.
- **Reddedildi:** cümle nedenini ve kimin işlem yapacağını söyler: ana
  sunucuda başka bir kayıt ya da elle değişiklik (adın orada bakın ya da
  Kopyala), oradaki bölgenin yöneticisi tarafından gözden geçirilmesi
  gerekiyor ya da ad, ana sunucunun bu çift için tuttuğu bir bölgede değil
  (kaydı adın yönetildiği yere Kopyala ile ekleyin).
- Bu sunucunun genel bir IPv6 adresi varsa bir IPv6 (AAAA) kaydı da istenir;
  isteğe bağlıdır, web postası yalnız IPv4 kaydıyla da çalışır.

### Sunucu kurulumu bir bölge çakışmasını olduğu gibi söylüyor

Ana sunucuda barındırılan başka bir DNS bölgesi ana alan adının bölgesinin
içinde ya da çevresinde olduğunda (örneğin ana alan adının altında bir müşteri
alan adı), sunucu kurulumu adlarını o bölgeye yine eklemez; bu kural
değişmedi. Değişen ekranın söyledikleri:

- **Planın özeti** başka bir bölgenin ana alan adının bölgesinin içinde ya da
  çevresinde olduğunu, bunun bir kayıtla ya da seçtiğiniz bir adla çakışma
  olmadığını, yöneticinin o bölgeyi Alan adları'nda gözden geçirip sonra planı
  yeniden okuduğunu söyler. Önceden var olan bir kaydın kurulumun eklediği
  adlarla çakıştığını söylüyordu.
- **İkinci sunucu katılırken** kurulum sayfası aynısını ve bunun katılan
  sunucunun adlarıyla ilgili olmadığını söyler; bölge artık çakışmadığında
  kurulum kendiliğinden sürer. Önceden o sunucunun adlarını değiştirmenizi
  söylüyordu.

### Başka bir firmanın ürün adı yok

Ayarlar'daki Panel portu kartı artık 2083 portu için "cPanel'in de kullandığı
port" demiyor; bu portu başka barındırma panellerinin de kullandığını
söylüyor. cPanel yedeklerinin içe aktarılması adını korur.

## Eski sürümdeki bir sunucuyla

- **alpha.138 ya da daha eski bir yedeğin alpha.139 bir ana sunucuya
  sorması:** meşgul yanıtı orada "henüz yanıtlanmadı"dır ve kurulum iki dakika
  sonra yeniden sorar. Gözden geçirilmesi gereken bir bölge, genel cümlesiyle
  ("bu kaydı oluşturmadı; elle ekleyin") bir ret olarak gösterilir.
- **alpha.139 bir yedeğin alpha.138 ya da daha eski bir ana sunucuya
  sorması:** o sunucu yukarıdaki bölge geneli neden için de "bu adı başka bir
  kayıt kullanıyor" der (**Ekle…** cümlesi ikisi için de doğru olacak biçimde
  yazıldı) ve meşgulken yine yanıt vermeyebilir (sayfa o zaman kendiliğinden
  bir kez daha sorar, sonra sonucun bilinmediğini söyler).

## Bilinen sınırlar

- **Kurulum kuralının kendisi değişmedi.** Altında bir müşteri bölgesi olan bir
  ana alan adı kurulumunu hâlâ genişletemez ya da ikinci bir sunucu bağlayamaz;
  yalnız sözcükler yeni. Daha dar bir kural, kendi laboratuvarıyla ayrıca
  planlanıyor.
- **Kart Roundcube gerektirir.** Web postası adres kartı (ve **Ekle…**)
  Roundcube'un sayfasındadır; bu yüzden yalnız Roundcube kurulu olan yerde
  görünür.
- **İlk yanıt "bilinmiyor" olabilir.** Ana sunucunun kaydı yayımlaması 10
  saniyeden uzun sürerse ilk yanıt "bilinmiyor" olur; 25 saniye sonraki
  kendiliğinden yeniden soru o zaman "zaten var" okur.
- **DNS bağlantısının kendi oturum açma retleri** (bilinmeyen ya da iptal
  edilmiş, istek okunmadan reddedilen bir bağlantı) ana sunucuda hâlâ günlük
  satırı bırakmaz.

## Laboratuvarda denetlenenler

Birincil ve yedek DNS sunucusu çiftindeki laboratuvar denetimi **açık**; çalışınca
buraya yazılır.

## Kanıt

Şema değişikliği yok: şema 079, alpha.138'deki gibi. Kalıcı kayıt değişmez;
ana sunucunun yanıtları eklemelidir (yeni bir ret kodu,
`REMOTE_DNS_HOST_RECORD_ZONE_REVIEW`, ve kodsuz bir "meşgul" yanıtı), yedek
sunucunun sayfaya kendi yanıtında bir sonuç daha var (`busy`). Değişiklikler
ana makine kaydı alıcısında ve uzak DNS yayım alıcısında
(`cmd/panel/remote_dns_host_record.go`, `setup_dns_remote_receiver.go`,
`server_setup_infrastructure_dns.go`), yedek sunucunun isteğinde
(`webmail_address_dns_record.go`), kurulum incelemesi ve katılma
sözcüklerinde (`server_setup_operations.go`, `server_setup_peer_join.go`) ve
sayfadadır (artık yalnız Roundcube sayfasında yüklenen
`WebmailAddressCard.tsx`, `lib/webmailAddressRecords.ts`, `dnsAdd`,
`setupDNS`, `setupLink`, `problems` ve `panelPort` metinleri). Go testleri
isteği gerçek TLS taşıması üzerinden üretim alıcısına karşı çalıştırır: ana
bölgenin yanında bir müşteri bölgesi eklendi ile biter; onun altındaki bir ad
reddedilir, hiçbir şey yazılmaz; sınır içinde meşgul, hiçbir şey yazılmaz;
yanıt yok, sonra zaten var, tek kayıtla; eski bir ana sunucunun "kullanılıyor"
yanıtı; her ret bir günlük satırı ve etkinlik kaydı bırakır; kurulum
incelemesi ve katılma çakışmayı kendi nedeniyle söyler. Bağlı web testleri
**Ekle…**'nin her sonunu (meşgul, tek yeniden soru, bilinmiyor, her ret) ve iki
kurulum cümlesini kapsar. `./cmd/panel` bir Debian konuğunda alpha.138 ile ad
ad karşılaştırıldı. **Hiçbir P0 maddesi kapanmadı.**
