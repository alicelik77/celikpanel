# v0.1.0-alpha.138

[English](RELEASE-NOTES-v0.1.0-alpha.138.md)

İki işlik küçük bir sürüm. Bir alan adının programları ve yapay zekâ
uygulamaları alan adı sayfasında kendi yerlerine kavuşuyor; bir program ile
bir yapay zekâ uygulaması artık birbirinin alan adını almıyor. Her posta
kutusu, bir posta programının gerek duyduğu ayarları gösteriyor; bu ayarların
verdiği posta sunucusu adı artık bu sunucunun gerçekten sunduğu ad.
**Kurulu panelleri yalnız sahip, panelin kendi güncelleme ekranından
günceller.**

## alpha.137'den güncelleme

1. Her zamanki gibi panelin kendi güncelleme ekranından güncelleyin. Bu
   sürüm panelin veritabanı düzenini değiştirmez (alpha.137'deki biçimde
   kalır); bu yüzden sonradan alpha.137'ye dönmek veritabanı geri yüklemesi
   gerektirmez.
2. Güncellemeden sonra basılacak ya da ayarlanacak bir şey yok. Alan adı
   sayfaları yeni Programlar bölümünü kendiliğinden gösterir; güncellemeden
   önce açık kalmış bir sayfa, yeniden açılınca gösterir.

## Sizin için ne değişiyor

### Programlar, alan adı sayfasında kendi bölümünde

Sahip, çalışan programını **Site** altında buldu ve alan adı sayfasının
satırında kendi yeri olmasını istedi (19 Kasım 2026). Satır artık şöyle:

**Genel Bakış · Site · Programlar · Dosyalar · E-posta · Veritabanları ·
Alan adı ve DNS · Güvenlik · Yedekler · Gelişmiş**

- **Programlar**, Site'taki iki kartı değişmeden tutar: "Programlar ve Python
  uygulamaları" (Yeniden yayına al, Durdur, Yeniden başlat, Günlük, Ortam
  değişkenleri, Ayarlar ve Programı kaldır ile program; programın nasıl
  yükleneceği; bir programın neler yapabildiği ve neyin engellendiği) ve
  "Yapay zekâ ve otomasyon". Program sitesinde önce programın kartı, yapay
  zekâ uygulaması sitesinde önce uygulamanın kartı gelir.
- **Site**, sitenin ne olduğunu ve ziyaretçilerin ona nasıl ulaştığını
  tutar: Site türü, Uygulamalar (PHP siteleri), PHP ayarları ve
  Yönlendirmeler.
- **Genel Bakış**'ta bir Programlar kutucuğu var; program ya da uygulama
  sitesinde orada ne çalıştığını söyler.
- **Eski bağlantılar çalışmayı sürdürür.** Eski yere giden bir bağlantı ya da
  yer imi (`?tab=programs`, `?tab=ai` ve kartların Site altındaki adresi)
  kartları Programlar'da açar.
- **Programlar yalnız içinde yapılabilecek bir şey olduğunda görünür.** Plan
  program, Python uygulaması ya da yapay zekâ uygulaması içeriyorsa ve
  sitenin türü bunlardan birini alabiliyorsa, alan adında zaten biri varsa
  her zaman oradadır. Node.js sitesi ikisini de almaz; bu yüzden Programlar
  bölümü yoktur. Ekip üyesi onu eskisi gibi görmez. Bu kartlar için "Site
  bölümü" ya da "Programlar sekmesi" diyen cümleler artık "Programlar
  bölümü" der.
- **Kullanım kılavuzu** yeni bölümü sayar ve onun için kısa bir kısım içerir.

### Bir program asla elinden alınmaz

Bir alan adı aynı anda bunlardan yalnız birine hizmet verir: bir program (ya
da Python uygulaması) veya bir yapay zekâ uygulaması. Bu sürümden önce sunucu,
programı zaten oluşturulmuş bir alan adına yapay zekâ uygulaması kurulmasına
ve bir yapay zekâ uygulamasının hizmet verdiği alan adında bir programın
yeniden yayına alınmasına izin veriyordu. İkisi de alan adını ötekinden
alıyordu; ardından alan adı silinemiyordu, sayfa da geride kalanı kaldıran
kartı artık göstermiyordu. Sunucu artık üçünü de, hiçbir şeyi değiştirmeden
önce reddeder:

- **Bir programın kullandığı alan adına yapay zekâ uygulaması kurmak**
  (oluşturulmuş ya da yayında): "Hiçbir şey değişmedi: bu alan adını bir
  program ya da Python uygulaması kullanıyor ve uygulama onun yerini alırdı.
  Önce programı Programlar bölümünden kaldırın ya da uygulamayı başka bir
  alan adına veya alt alan adına kurun."
- **Bir programın kullandığı alan adında yapay zekâ uygulamasını güncellemek**
  (yalnız ikisini birden tutan bir alan adında): aynı cümle.
- **Bir yapay zekâ uygulamasının kullandığı alan adında programı yayına
  almak:** "Hiçbir şey değişmedi: bu alan adını bir yapay zekâ veya otomasyon
  uygulaması kullanıyor. Önce onu Programlar bölümünde, Yapay zekâ ve
  otomasyon kısmından kaldırın."

Sayfa bu basışları zaten sunmaz.

**Bir alan adında duran her şey, kaldırılabileceği yerde gösterilir.** Alan
adı listesi artık plan ya da sitenin türü ne derse desin bir alan adının
program mı yapay zekâ uygulaması mı tuttuğunu söyler; Programlar onun kartını
çizer:

- **İkisini birden tutan alan adı** (önceki bir sürümden güncellenmiş
  sunucularda olabilir): Programlar iki kartı da gösterir. Alan adının hizmet
  vermediği tarafın kartı onu yönetmeye ve kaldırmaya yarayanları gösterir
  (Durdur, Yeniden başlat, Günlük, Ortam değişkenleri, Ayarlar, Kaldır), onu
  alan adına geri koyacak hiçbir şeyi göstermez (Yeniden yayına al, Aç,
  güncelleme ya da kurulumu yeniden deneme yok). İstemediğinizi kaldırın;
  alan adı ondan sonra silinebilir.
- **Oluşturulmuş ama hiç yayına alınmamış, planın program hakları sonradan
  kapanmış bir program:** Programlar ve programın kartı oradadır; planın hâlâ
  izin verdikleri ve **Programı kaldır** ile.
- Alan adı bunlardan biri yüzünden silinemediğinde ret sizi Programlar
  bölümüne gönderir; kaldırma düğmesiyle kart orada gösterilir.
- Program tutan (hiç yayına alınmamış olsa da) bir alan adı yapay zekâ
  uygulamasının **Kur** düğmesini, yapay zekâ uygulaması tutan bir alan adı
  da yeni bir programı sunmaz: sunucu ikisini de reddederdi.

### Her posta kutusunun bağlantı bilgileri

- Her posta kutusu satırının **Diğer** menüsü **Bağlantı bilgileri…** ile
  başlar. Açılan pencere bir posta programının (Outlook, Thunderbird, Apple
  Mail, telefon) gerek duyduklarını gösterir: kullanıcı adı, gelen (IMAP)
  sunucu, POP3 seçeneği ve giden (SMTP) sunucu, bağlantı noktası ve
  güvenlikle birlikte, ve posta kutusunun web postası bağlantısı. Her değer
  kopyalanabilir; **Tümünü kopyala** her şeyi bir mesaja yapıştırmak için
  sayfanın dilinde düz metin olarak kopyalar.
- **Parola asla gösterilmez ve kopyalanmaz.** Pencere, parolanın bu posta
  kutusu için belirlenen parola olduğunu ve yenisinin nerede belirlendiğini
  söyler.
- **Giden posta, aynı kullanıcı adı ve parolayla oturum açmayı ister**;
  sunucu böyle istiyorsa pencere bunu söyler.
- **Ayarlar çalışmayacaksa pencere bunu söyler.** Bu sunucuda posta kurulu
  değilse burada posta programlarına yanıt veren bir şey olmadığını, kimin ne
  yapacağını söyler ve **Tümünü kopyala** kapalıdır. Gösterilen sunucu adı
  doğrulanmamışsa ayarların posta kutusuna ulaşmayabileceğini söyler;
  kopyalanan metin de sunucu adının henüz doğrulanmadığını belirten bir notla
  başlar.
- Bir posta kutusu oluşturulduktan sonra bildirimi **Bağlantı bilgilerini
  göster** önerir; bir sonraki adım öneren bildirim kapatılana dek kalır.
- Posta kutularını yalnız görebilen bir ekip üyesi pencereyi **Web postası**
  düğmesinin yanından açar.

### Posta programlarına bu sunucunun gerçekten sunduğu posta adı verilir

Bir alt alan adında, web sitesi olmayan bir alan adında ya da posta adı başka
yerde zaten kullanılan bir alan adında, posta ayarları (alan adının
postasındaki **Ayarlar** sekmesi ve artık her posta kutusunun penceresi)
`mail.<alan adı>` adını veriyordu. Bu sunucu o adı ne seçmiş ne de eşleşen bir
sertifikayla sunuyordu; bu adla kurulan bir posta programı sertifika uyarısı
verebiliyor ya da bağlanamıyordu. Ayarlar artık bu sunucunun gerçekten
sunduğu posta adını, çoğunlukla sunucunun kendi posta adını verir.

**Böyle bir alan adı için birine sunucu olarak `mail.<alan adı>` verdiyseniz,
panelin şimdi gösterdiği adı kullanın.** Kendi `mail.<alan adı>` adını bu
sunucunun sertifikayla sunduğu alan adları o adı korur.

### Dört stilin yeni adları

Panelin stilleri (açık ve koyu anahtarının yanındaki palet düğmesi) artık
**Celik**, **Gök**, **Orman** ve **Kehribar**; hiçbiri başka bir firmanın
ürününün adını taşımaz. Daha önce seçilen stil korunur ve yeni adıyla
gösterilir.

## Bilinen sınırlar

- **Node.js Site altında kalır.** Bir Node.js uygulamasının başlatma,
  durdurma, yeniden başlatma ve günlüğü, ait oldukları tür, sürüm ve başlatma
  komutunun yanında Site → Site türü altında kalır.
- **Bir İngilizce cümle hâlâ "Programs tab" diyor.** Site türü değişikliğinin
  ya da alan adı silmenin ardındaki, sunucunun kendi İngilizce reddi hâlâ
  "the Programs tab" der; sayfa kendi sözlerini gösterir ve o sözler
  Programlar bölümünü anar.
- **Postası olmayan sunucuda alt alan adı için "ulaşmayabilir".** Postanın
  kurulu olmadığı bir sunucuda bir alt alan adının posta ayarları, postanın
  burada olmadığını değil, ayarların posta kutusuna ulaşmayabileceğini
  söyler.
- **Kendiliğinden kurulum ipucu yok.** Posta kutusu penceresi, bazı posta
  programlarının bu ayarları kendiliğinden bulabildiğini söylemez.

## Laboratuvarda denendi

Laboratuvar denetimi bitti: bir Debian 13 ve bir Ubuntu 24.04 deneme
sunucusunda bir deneme.

- İki sunucu da panelin kendi güncelleme sayfasından, kendiliğinden
  güncellendi; başarısız işaretli hiçbir şey kalmadı.
- **Çalışan bir program elinden alınmadı.** Programı yayında ve çalışır
  durumdaki bir alan adında yapay zekâ uygulaması kurmak için gönderilen
  doğrudan istek "Nothing was changed: a program uses this domain. Remove it
  in the Programs section first, or use another domain or subdomain for
  n8n." ile reddedildi (sayfa bunu Türkçe sözleriyle gösterir). Hiçbir şey
  oluşturulmadı; program deneme sırasında ve sonrasında 45 denetimin 45'ine
  yanıt verdi.
- **Alan adının tuttuğu gösterilir.** Planı artık program içermeyen bir alan
  adında oluşturulmuş ama hiç yayına alınmamış bir program, **Programı
  kaldır** ile Programlar'da hâlâ göründü; kaldırıldı ve alan adı silindi.
- On girişli bölüm satırı masaüstü genişliğinde tek satıra sığar, telefonda
  sayfa yana kaymadan kendi içinde kayar. Eski adresler
  (`?section=site&part=programs`, `?tab=ai`) Programlar'a varır.
- Posta kutusu penceresi Türkçe ve İngilizce, masaüstü ve telefon
  genişliğinde: ayarlar, giden posta oturum açma satırı ve metninde parola
  olmayan **Tümünü kopyala**. Yeni posta kutusunun bildirimi **Bağlantı
  bilgilerini göster** önerdi ve pencereyi açtı.
- Sunucunun posta adı bir alt alan adı ve sıradan bir alan adı için
  gösterildi; 993, 995 ve 587 numaralı bağlantı noktalarında sunucunun
  sertifikasıyla eşleşti ve bu adla IMAP üzerinden oturum açmak çalıştı.
- Bir birincil ve yedek DNS sunucusu çiftinde bir alan adı oluşturulup
  silindi.

**Denenmedi:** hem program hem yapay zekâ uygulaması tutan bir alan adı ve
önceki sürümde bir programın üzerine yapay zekâ uygulaması kurmanın ne
yaptığı: laboratuvarın Debian 13 ve Ubuntu 24.04 sunucularına hiçbir yapay
zekâ ya da otomasyon uygulaması kurulamıyor (orada Node.js 24 ve Python 3.11
ya da 3.12 yok); bu kısım bileşen testlerine dayanır. Bölümü olmayan bir
Node.js sitesi. Doğrulanmamış bir posta adı için uyarı.

**Laboratuvar denemesinden sonra düzeltildi (yalnız ekran):** oluşturulmuş
ama hiç yayına alınmamış programı olan bir PHP alan adında Programlar, sunucunun
orada reddettiği **Kur** düğmeleriyle yapay zekâ kartını da gösteriyordu.
Artık göstermiyor; yapay zekâ uygulaması tutan bir alan adı da artık yeni bir
program sunmuyor.

### Laboratuvarda bulunan, sonraki sürüme bırakılanlar

- n8n için "henüz kurulamaz" diyen satır iki dili karıştırıyor ve sunucunun
  yapamadığı bir Node.js kurulumunu gösteriyor.
- Yedek DNS sunucusunu bekleyen bir alan adı silmenin hemen ardından durum,
  silmenin durduğunu ve yeniden silinmesini söylüyor; oysa kendiliğinden
  bitiyor (laboratuvarda yaklaşık 51 saniye).
- Debian 13 ve Ubuntu 24.04'e hiçbir yapay zekâ ya da otomasyon uygulaması
  kurulamıyor (gerektirdikleri çalışma ortamı sürümleri orada yok).

## Kanıt

Şema değişikliği yok: şema 079, alpha.137'deki gibi. Kalıcı kayıt
değişikliği yok; alan adı listesi iki ek alan taşır (`holds_program`,
`holds_app`): yanlışken ve ekip üyesi için yazılmaz, bütün liste için birer
sorguyla okunur. Değişiklikler alan adı sayfasında (`web/src/components/domain`,
Programlar bölümü ve kuralı), iki kartta (`DomainRuntimeApps`,
`DomainServiceApps`), sunucunun yapay zekâ uygulaması ve program kurma,
güncelleme, yayına alma işleyicilerinde ve alan adı listesinde
(`cmd/panel`), posta kutusu penceresinde (`MailboxConnectionDialog`,
`DomainMailManager`) ve posta kurulum yanıtındadır
(`cmd/panel/mail_autoconfig.go`, `domain_mail_extra_handlers.go`). Bileşen
testleri: site türü, plan, rol ve alan adının tuttuklarına göre bölüm;
ekranın ret tablosunun sunucunun kaynağındaki koşullarla karşılaştırılması;
üç reddin sunucunun kendi işleyicilerinde denenmesi (hiçbir şey kaydedilmez,
Agent'a sorulmaz, öteki site türleri değişmez); alan adı listesinin alanları
role göre; kartların her eski adresi; posta kutusu penceresi her posta
durumunda, iki dilde; her tür alan adı için posta kurulum yanıtı; dört stil
ve eski adıyla saklanmış bir stil. `cmd/panel` testleri alpha.137 ile ad ad
karşılaştırıldı: yeni hata yok. Bir laboratuvar denemesi (yukarıda).
**Hiçbir P0 maddesi kapanmadı.**
