# v0.1.0-alpha.127

[English](RELEASE-NOTES-v0.1.0-alpha.127.md)

Onaylanan her değişiklik artık çalışırken sunucunun gerçek adımlarını
gösteriyor. Alan adı sayfası yavaş DNS ve posta denetimlerini beklemiyor: son
doğrulanmış yanıtı hemen gösterip arkada yeniden denetliyor. PostgreSQL
veritabanı değişiklikleri yalnız çalışan bir kümeye gidiyor; kaldırılmış bir
sürümün saklanan kümesi yeni veritabanlarını artık bozmuyor. Her parola alanı
tek bir kuralı, parola üreticiyi ve güç göstergesini paylaşıyor; zayıf parola
her yerde aynı biçimde reddediliyor. Ekranlar sunucunun yöneticisine
düzeltmenin kendisini veriyor, diğer herkese kimin işlem yapacağını söylüyor.
İzleme uyarılar (panelde ve e-postayla), hizmet sağlığı, web sitesi
denetimleri ve ani yükte en yoğun süreçleri kazanıyor. Node.js siteleri site
korumasına katılıyor. **Kurulu panelleri yalnız sahibi, panelin kendi
güncelleme ekranından günceller.**

## Onaylanan her değişiklik gerçek adımlarını gösteriyor

alpha.126 gerçek adımları yalnız alan adı silmede gösteriyordu. Artık ilerleme
penceresi diğer onaylanan değişikliklerde de sunucunun kendi adımlarını
izliyor.

- **Sertifikalar:** Let's Encrypt isteği (denetimler, DNS denetimi,
  doğrulamanın hazırlanması, sertifika otoritesi, doğrulama, kurulum, yeniden
  yükleme, posta), sertifika yükleme ve kaldırma.
- **Yedekler:** geri yükleme (denetim, inceleme, geri yükleme, bitiş) ve
  artık kilitli pencerede çalışan yedek alma.
- **PHP sürümü, php.ini ve barındırma türü** değişiklikleri; **uygulama
  kurulumu** ve **WordPress araçları** kendi kayıtlı durumlarını izler.
- **Posta şifrelemesi, DNS kayıtları** (ekleme, değiştirme, silme),
  **PowerDNS onarımı**, **hizmet başlatma/durdurma/yeniden başlatma**,
  **zararlı yazılım** karantinası, geri getirme ve silme ve **veritabanı,
  veritabanı kullanıcısı ve hesap silme**.
- **Başarısız adım işaretlenir** ve olduğu gibi söylenir: reddeden bir
  denetim karşılanacak bir önkoşuldur (kimin işlem yapacağı ve sonraki
  adımla), kaybolan yanıt "bilinmiyor"dur, hiçbir zaman "başarısız" değil.
- **Sayfa yenilense de pencere kalır.** Sertifika isteği, yedek, PHP sürümü
  değişikliği, uygulama kurulumu ya da WordPress aracı sırasında sayfa
  yenilenirse pencere sunucunun o anki adımında yeniden açılır ve nasıl
  bittiğini söyler. Adımı sormak yalnız okur: hiçbir değişikliği başlatmaz ya
  da tekrarlamaz.
- İlk girişteki tanıtım turu artık bir soru ya da ilerleme penceresi açıkken
  bekliyor.

## Alan adı sayfası hemen yanıt veriyor

Boston'da Veritabanları sekmesi yaklaşık 10 saniye, Genel bakış 5–10 saniye
bekliyordu; çünkü her biri DNS çifti denetiminin tamamını (bağlı sunucuda bir
de ana DNS sunucusundaki denetimi) başlatıyordu.

- **Ekranlar son doğrulanmış yanıtı hemen gösteriyor.** Bir DNS denetiminin
  sonucu 30 saniye kullanılır; sonrasında sayfa onu zamanıyla ve "yeniden
  denetleniyor" notuyla gösterirken arkada tek bir denetim çalışır. Sayfa
  kendiliğinden yeniden okur (yalnız sekme görünürken, en çok altı kez); yeni
  yanıt sayfa yenilenmeden görünür. Sayfayı yenilemek ikinci bir denetim
  başlatmaz; başarısız bir denetim hiçbir zaman son yanıt olarak gösterilmez.
- **Değişiklikler canlı denetimi kullanmayı sürdürür.** Alan adı oluşturma ya
  da içe aktarma, DNS yayını ve kurulum her zaman sunuculara doğrudan sorar;
  her DNS değişikliği ekranların yeniden denetlemesini sağlar.
- **Daha hızlı denetimler.** İki sunucuya giden DNS soruları, bağlantı
  kartının sorguları, posta sağlığı ve posta kimlik doğrulama sorguları artık
  art arda değil aynı anda çalışıyor.
- **Kullanım ve programlar.** Bir sayfayı açmak disk kullanımını ölçmez,
  erişim kayıtlarını taramaz; son ölçümü zamanıyla gösterir ve 15 dakikadan
  eskiyse arkada tek bir yeni ölçüm başlatır. Hiç ölçülmemiş disk böyle
  söylenir, hiçbir zaman 0 diye değil. Programların koruma düzeyi son
  doğrulanmış yanıttır; her değişiklik yine Ajana sorar.

## PostgreSQL: değişiklikler çalışan kümeye gidiyor

Boston'da bir PostgreSQL veritabanı oluşturmak iki kez "iç hata" yanıtı
verdi: PostgreSQL 16 verisi saklanarak kaldırılmıştı, kümesi hâlâ 5432
portunu tutuyordu ve PostgreSQL 18 5433'te çalışıyordu. Panel her ifadeyi
programı olmayan kümeye gönderiyordu.

- **Veritabanı değişiklikleri yalnız çalışan bir kümeye gider.** Kaldırılmış
  ya da durdurulmuş bir kümeyi gösteren kayıt, olgularıyla birlikte
  reddedilir. Boş bir kayıt çalışan kümeye taşınır; içinde bir şey olan kayıt
  hiçbir zaman taşınmaz, saklanan kümelere hiç dokunulmaz.
- **Hiçbir PostgreSQL çalışmıyorsa** yeni veritabanı nedeni, kimin işlem
  yapacağı ve sonraki adımla reddedilir ("iç hata" yok).
- **Kaldırılmış bir sürümün saklanan kümesi** sayfada dürüstçe anlatılır:
  veri ve ayarlar saklandı, port hâlâ ayrılmış, sahip onu nasıl kaldırır.
- **Yedekler kümelerini adlandırır.** PostgreSQL yedeği ana sürümünü ve
  portunu kaydeder. Geri yükleme o veritabanının kümesine, yalnız küme
  çalışıyorsa ve aynı ana sürümse yapılır; başka bir ana sürüm, hiçbir şey
  yazılmadan ve yedek saklanarak reddedilir. alpha.127'den önce alınmış
  yedekler eskisi gibi geri yüklenir.
- **Tabloları duran bir PostgreSQL veritabanını geri yüklemek çalışır.**
  Önceden böyle her geri yükleme "relation already exists" hatasında
  duruyordu (güvenlik kopyasına otomatik dönüş de). Geri yükleme artık önce
  veritabanı sahibinin o veritabanındaki nesnelerini düşürür, sonra onları
  yedekten yine aynı sahibe ait olarak kurar. Başka rollerin nesnelerine
  dokunulmaz; sahibi süper kullanıcı olan bir veritabanı eskisi gibi geri
  yüklenir.
- **phpPgAdmin çalışan kümeleri izler.** Kurulduktan, başka bir PHP sürümüne
  taşındıktan ya da bir PostgreSQL sürümü kurulup kaldırıldıktan sonra sunucu
  listesi çalışan her kümeyi adlandırır ("PostgreSQL 18 (port 5433)"). Sizin
  düzenlediğiniz liste hiçbir zaman yeniden yazılmaz: phpPgAdmin kartı neyi
  gösterdiğini, neyin çalıştığını ve neyi ayarlamanız gerektiğini söyler.
- **Reddedilen değişiklik bir kez söylenir:** formun üstündeki bantta,
  düzeltmesiyle birlikte, görünür kılınarak ve duyurularak; aynı cümleyi bir
  bildirim tekrar etmez.

## Her yerde tek parola kuralı

- **Her parola alanı** (posta kutuları, veritabanı kullanıcıları, panel
  hesapları, davet ve sıfırlama bağlantıları, ekip üyeleri, FTP/SFTP/SSH
  erişimi, korumalı klasörler, hizmet uygulaması sahipleri) kuralı, güç
  göstergesini ve hâlâ eksik olanı gösterir; **Üret** ve **Kopyala** sunar ve
  kural sağlanana dek düğmeyi kapalı tutar. Var olan gizli bilgilerin
  alanlarında yalnız göster/gizle vardır.
- **Kural:** en az 6 karakter (ya da alanın daha sıkı alt sınırı), bir büyük
  ve bir küçük harf ve bir özel karakter. Sunucu aynı kuralı denetler ve
  zayıf parolayı `PASSWORD_WEAK` olarak, bozduğu ilk kuralı adlandırarak
  reddeder. Sunucunun ürettiği parolalar kurala her zaman uyar.
- **İlk yönetici.** `get.sh`, kimlik bilgisi dosyasındaki parolayı hiçbir şey
  indirmeden ya da değiştirmeden önce kurala göre denetler ve sağlanmayan
  kuralı adlandırır; terminal istemi önce kuralı söyler, zayıf parolada
  yeniden sorar.

## Yönetici düzeltmeyi alıyor; diğerlerine kimin işlem yapacağı söyleniyor

Boston'da Erişim sekmesi eksik unzip/zip ve FTP için "sağlayıcınıza sorun"
diyordu; Posta sekmesi yalnız web postasının kullanılamadığını söylüyordu —
bir müşteri hesabına. Ekranlar artık her rol için kimin işlem yapacağını
söylüyor.

- **Yönetici işlemin kendisini alır:** Erişim sekmesinde "Araçları kur"
  (panelin kendi penceresinde, kilitli ilerleme adımlarıyla), web postası
  kartında "Web postasını kur", "Web postası ayarlarını aç" ya da "Roundcube'u
  aç"; cron, veritabanı web araçları, Python ve Node.js çalışma ortamları ve
  ClamAV imzalarının yanında kurulum bağlantıları; düzeltme oradaysa PHP-FPM
  sayfasına, Katalog yönetimine, Kullanıcılara ya da posta kurulumunun
  Onar'ına bağlantılar.
- **Müşterilere sağlayıcılarının (sunucunun yöneticisi) işlem yapacağı
  söylenir** ve açamayacakları bir sayfaya düğme almazlar. Müşteri olarak
  oturum açmış bir yöneticiye yönetici hesabını kullanması söylenir.
- **Planlar ve bayi havuzları.** Bir hesabı yöneten yönetici ya da bayi onun
  adını ve "<hesap> haklarını aç" düğmesini görür; bayi havuzu bildirimi
  bayinin haklarını yerinde açar.
- **CelikPanel'in kurulum yolu olmayan yerlerde** (otomatik güncellemeler,
  unattended-upgrades, logrotate) yönetici komutun tamamını Kopyala düğmesiyle
  alır.
- Kurulumdan sonraki PHP aracı notu aracın kendi sayfasına bağlanır.

## İzleme: uyarılar, hizmetler, web siteleri

- **Uyarılar.** "Şu anki sorunlar" altında, belirlediğiniz gecikmeden
  (5–60 dakika, varsayılan 10) uzun süren bir sorun, ne zamandan beri
  sürdüğüyle tek bir uyarı olur ve sorun kalktıktan 10 dakika sonra çözülmüş
  sayılır. Denetlenemeyen bir kaynak uyarı ne açar ne kapatır. "Tümünü
  görüldü say" Ana sayfadaki satırı kaldırır.
- **Doğrulanmış yöneticilere e-posta.** Uyarılar yalnız hesap adresi
  doğrulanmış (Hesabım) etkin yöneticilere, sunucunun kendi postasıyla
  gönderilir: her seferinde tek bir özet, saatte en çok dört, aynı uyarı altı
  saat içinde yeniden gönderilmez. Her uyarı e-postasına ne olduğunu söyler.
  Panel ya da sunucu durmuşken uyarı yoktur; sayfa dışarıdan bir erişilebilirlik
  denetleyicisi önerir.
- **Hizmetler ve web siteleri.** Her hizmet ne zamandan beri çalıştığını, son
  bir saatteki yeniden başlatmaları, işlemci ve belleği gösterir; bir saatte
  üç yeniden başlatma bir sorundur. Sunucu kendi web sitelerini 5 dakikada bir
  denetler (yanıt, ne zamandan beri ve neden kapalı, sertifikanın kalan
  günleri).
- **Daha çok grafik.** Disk okuma/yazma ve ağ giriş/çıkış; işlemci ya da
  bellek ani yükseldiğinde en yoğun beş süreç yakalanır (yalnız kısa adı ve
  sahibi, hiçbir zaman komut satırı değil) ve grafiğin altında gösterilir.

## Site korumasında Node.js siteleri

- **Node.js siteleri diğer sitelerle aynı korumada çalışır:** kendi belge
  kökü, tek bir Node.js sürümü, sitenin tek portu ve giden postanın
  engellenmesi; sunucunun sisteminin desteklediği düzeyde.
- **Paketleri kur ve derle.** Barındırma sekmesi korumanın içinde `npm ci`
  (kilit dosyası yoksa `npm install`) ve `npm run build` çalıştırır; onay ve
  ilerleme penceresiyle.
- **Var olan Node.js siteleri sessizce yeniden yazılmaz.** Bileşenler →
  Node.js sunucunun düzeyini, eksiklerini ve taşınacak siteleri gösterir;
  yönetici onları panelin kendi penceresinde, her seferinde bir site olmak
  üzere taşır. Taşınan ve çalışan her uygulama denetlenir; yanıt vermeyi
  bırakan önceki düzenine geri alınır ve yeniden başlatılır. Hizmet dosyasını
  sizin düzenlediğiniz siteler korunur ve listelenir.

## Küçük düzeltmeler

- Posta sunucusu olmayan bir sunucu artık her güncellemeden sonra posta
  sertifikası "hatası" yazmıyor; yayımlanacak bir şey olmadığını bir kez not
  ediyor.
- alpha.125'te takılan bir silmeden kalan boş site klasörleri Ajan
  başlarken kaldırılıyor (yalnız hiçbir sitenin kullanamayacağı boş
  klasörler).

## Laboratuvar sonuçları

_Laboratuvar turundan sonra doldurulacak._

## Bilinen sınırlar

- **İlerleme adımları:** ekip üyesi kaldırma hâlâ tek bir genel adım
  gösteriyor. Bu sürümün ikinci turunda adım kazanan değişiklikler (sertifika
  yükleme ve kaldırma, posta şifrelemesi, DNS kayıtları, PowerDNS onarımı,
  php.ini, barındırma türü, hizmetler, zararlı yazılım, silmeler) sayfa
  yenilenince pencerelerini yeniden açmıyor. Alan adı silme, ortak kayıt
  yerine alpha.126'daki kendi adım kaydını kullanmayı sürdürüyor. Panel
  yeniden başlarsa o sırada çalışan değişikliğin adımları kaybolur ve sonucu
  "bilinmiyor" olur.
- **Alan adı sayfası:** yavaş bir denetimin yanıtı, sayfa yeniden
  denetlemeden önce en çok 30 saniye kadar eski olabilir; gösterilen zaman ne
  kadar eski olduğunu söyler.
- **PostgreSQL:** ne çalışan varsayılan küme ne de kendi kayıtlı kümesi olan
  bir kümedeki veritabanı kendiliğinden taşınmaz; phpPgAdmin sunucu listesini
  kendiniz düzenlediyseniz yazdığınız gibi kalır.
- **Yöneticinin sonraki adımı:** bazı bildirimler hâlâ her rol için sağlayıcı
  ifadesini kullanıyor; docs/OPERATION-GUIDANCE.tr.md içinde listelenmiştir.
- **İzleme:** web siteleri yalnız sunucunun kendisinden denetlenir (eşli DNS
  sunucusundan denetim yok); `/proc` başka kullanıcıların süreçlerini
  gizliyorsa (hidepid) süreç yakalama kapalıdır.
- **Node.js koruması:** şimdilik yalnız WSL Debian 13'te denendi.
  Laboratuvar için açık kalanlar: desteklenen beş sistem, çıkış tablosu
  yüklüyken joker bir Node.js dinleyicisine dışarıdan ulaşılamadığı (kaçış
  takımı 10. madde) ve her sistemde yöneticinin işlemiyle taşınan gerçek bir
  Node.js uygulaması (Express, Next.js).
- Güncellemenin son veritabanı denetimi hâlâ çalışan veritabanının bir
  kopyasını alıyor. Bunun yerine çalışan panele sormak yapılmadı: denetim
  güncelleyicinin kendi bağımsız kanıtı olarak kalmalı
  (docs/UPDATE-LATE-DATABASE-PROOF.md daha güvenli bir yön adlandırıyor).
- alpha.125'e ya da daha eskisine geri dönmek, yedek DNS sunucusundaki yeni
  DNS denetim aracını yerinde bırakır (aynı istek biçimini konuşur).
- Önceki sınırlar sürüyor: yedek sunucusu olan PowerDNS ana sunucusunda DNSSEC
  imzalama yok; site cron çıktısı postayla gönderilmiyor ve saklanmıyor;
  kurulum makinenin kendi adı için AAAA (IPv6) kaydı eklemiyor; Ubuntu 22.04
  üzerinde güncelleme son laboratuvarlarda denenmedi.

## Kanıt

Geçiş 065 (`monitoring_alerts`): `metrics_samples` tablosunda dört yeni
isteğe bağlı sütun (ağ ve disk hızları) ve üç yeni tablo
(`metrics_process_snapshots`, `monitoring_alerts`, `site_uptime`); var olan
hiçbir şey değişmiyor. Her geçişte olduğu gibi alpha.126 bu geçişi içeren bir
veritabanını reddeder; geri dönüş, güncelleme öncesi anlık görüntüden geri
yüklemedir. Sürüm sırası 126 → 127. Go testleri (bütün modül) ve web
testleri geçiyor, web paketleri sınırları içinde.
