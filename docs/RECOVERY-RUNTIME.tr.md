# Bağımsız kurtarma çalışma ortamı

*14 Eylül 2026 · [English](RECOVERY-RUNTIME.md) · D-025 · P0.2/P0.3*

Sunucu sahibi, panel açılışı, Agent RPC bağlantısı, uygulama şema geçişi veya lisans
kontrolü çalışmadan tam güncelleme işlemini sorgulayabilir ve mevcut kurtarmayı
sürdürebilir. Bu dar girişte kimlik doğrulama işletim sisteminin root/yetkili sudo
oturumudur. Ayrı HTTP dinleyicisi veya ikinci bir oturum yetkisi oluşturulmaz.

## Sınır

`/usr/libexec/celikpanel/recovery status --request-id <id> --json`, panelin de
kullandığı sınırlı gözlem sözleşmesini okur. Sıfır çıkış kodu bilinen gözlem demektir;
güncellemenin başarılı olduğunu söylemez. Bilinmeyen sonuç bilinmeyen kalır.
`recover` yalnız kalıcı olarak başlamış işlemi yerel sürüm kilidi altında inceler.
Yeni kurulu-panel güncellemesi başlatamaz veya keyfi hedef alamaz. Her kurulu-panel
güncellemesini kullanıcı panelin kendi arayüzünden başlatmaya devam eder.

İlk kabul edilmiş kurulum/güncelleme, koordinatörleri durdurmadan doğrulanmış
kurtarma paketini kaydeder. Kod
`/usr/libexec/celikpanel/recovery-runtimes/v1/<manifest-sha256>` altında;
root erişimli seçim `/var/lib/celikpanel-release-state/recovery-runtime.v1`
dosyasındadır. Tam paket ve başlatıcı diske kalıcı yazılmadan seçim yayımlanmaz.
İlk kayıt mevcut uyumlu seçimi korur. Ayrı kayda bağlı
[kurtarma ortamı geçişi](RECOVERY-RUNTIME-PROMOTION.tr.md), kabul edilmiş
güncellemenin ön kontrolünde yeni kiti hazırlar; önceki kiti korur ve doğrular.
Seçilmiş kodun kayıp/bozuk olması yeni adayla üzerine yazma veya aday yaşam
döngüsü koduna geri dönme yetkisi değildir. Yarım kalmış paketler seçilmeden
kanıt olarak saklanır.

Kanonik manifest protokol1, snapshot6, on iki dosya, izinler, SHA-256 ve tam
envanteri sabitler. Okuyucu root sahipliğini, yol zincirini, bağlantıları, boyut
sınırlarını ve dosya kimliklerini denetler; açık dosyaları tutup çalıştırmadan
önce kanıtı tekrarlar. Bu, son kontrolden sonra keyfi eşzamanlı root müdahalesine
karşı sınırsız bir garanti değildir.

## Mevcut işlem ve snapshot geçişleri

Çalıştırıcı kayıtlı paketin scriptlerini ve çevrimdışı denetleyicilerini kullanır.
Başarısız hedef sürüm ayrı veri/köken kaynağıdır. Panel/Agent denetleyicileri normal
main/RPC/HTTP başlangıcı olmadan üretimdeki aynı snapshot ve işlem kaydı kodundan
derlenir; ikinci bir ayrıştırıcı yazılmaz. Şema17 köprüsü de paketin içindedir.

| Kalıcı durum | Bağımsız davranış |
|---|---|
| İşlem yok | Değişiklik başlatılmaz. |
| Kilit canlı güncelleyicide | Yarışılmaz; yerel kurtarma zamanlayıcısı tekrar dener. |
| Quiesce yakalama | Tam active-öncesi işlem mevcut kanıtıyla iptal edilir. |
| Active, eksik snapshot | Doğrulanmış snapshot tamamlanır; aday kurulmadan paketin geri alma yoluna geçilir. |
| Active, tam snapshot | Tam olarak o snapshot geri yüklenir. |
| Tamamlama/zamanlayıcı temizliği | Mevcut çalışma durumu doğrulanır ve kayıtlı temizlik tamamlanır; aday şema geçişi çalıştırılmaz. |
| Bilinmeyen/uyuşmayan kanıt | İşlem korunur, ilgili eylem reddedilir, bağımsız durum sorgusu kullanılabilir kalır. |

Yalnız dış snapshot biçimi6 kabul edilir. İçindeki normal, işlem kaydı öncesi
şema20 ve şema17 köprü geçişlerinin açık kuralları korunur. Biçim4/5 sessizce
başka anlamda yorumlanmaz. Kilit tanıtıcısı, durmuş koordinatörler, tam kalıcı
işlem kimliği, snapshot bütünlüğü ve sahip değişiklikleri kontrol edilmeye devam eder.

## Kanıt ve kapsam

- Manifest, yol/sahiplik/FIFO/bağlantı/içerik/değiştirme testleri ve gerçek root
  miras FD kayıt testleri bağımsız giriş sınırını denetler.
- Çevrimdışı denetleyiciler gerçek SQLite şemaları/WAL ve işlem kayıtlarıyla
  sınanır; başarılı alt süreç çıkışı gerçek geri yüklenmiş sistem kanıtı sayılmaz.
- Geçici VM toplayıcısı bütün tabloları tek tutarlı SQLite okuma işleminde
  karşılaştırır. Oturumlar ve işlemler dışlanmaz. SQLite WAL koordinasyon
  metadata farkları raporlanır; DB/WAL içerikleri yeniden yazılmaz.
- Kurtarma sürecinin öldürülmesi ve yeniden başlatılması ayrıca gerçek VM
  matrisiyle kanıtlanır; bileşen testlerinden çıkarılmaz.

Bütün desteklenen aşamalar sınanmadan P0.3 kapanmaz. Aşağıda tanımlanan bin/web
kaynaklarının kabul edilen program yayını atomiktir. Diğer geri yükleme adımları
sürüm scriptleriyle ortak sözleşmeleri kullanır; bu dilim TLS normalizasyonunu
salt-okur veya adayın veri bütünlüğünden tamamen bağımsız yapmaz. [Seçili kit geçişi](RECOVERY-RUNTIME-PROMOTION.tr.md), uyumluluk
kontrollerini, önceki kitin korunmasını ve ayrı gerçek sistem kabulünü kaydeder.
Yeni kurtarma protokolü yine açık bir uyumluluk geçişi gerektirir. P0.4 ortak veri sözleşmeleri ve P0.5 bağımsız yenileme/açılış kanıtı
ayrı işlerdir.

## Servisler durdurulmadan önce uyumluluk

Kayıttan sonra aday CLI, seçili kitin çevrimdışı okuyucularıyla mevcut normal,
pre-ledger veya schema17 durumunu release kilidi altında denetler. Bu kontrol
quiesce niyeti yazılmadan ve koordinatörler durdurulmadan önce yapılır. Okuyucu
uyumsuzsa panel hâlâ çalışırken güncelleme reddedilir; bu sonuç gelecekteki geri
almanın başarılı olduğunu kanıtlamaz.

Normal geri alma, zaten kabul edilmiş Agent işlem kaydının inode'unu yerinde
korur; kilit, tam içerik, sahiplik ve izin kanıtını tekrarlar. Kaydı silip yeniden
yazarak kesinti aralığı oluşturmaz. Eksik veya sahibi tarafından değiştirilmiş
kayıt reddedilmeye devam eder.

Kayıt işlemi başlatıcıyı yayımladıktan, seçiciyi yazmadan önce kesilirse aynı kit
ile tekrar desteklenir ve gerçek SIGKILL ile sınanmıştır. Farklı bir kitin
başlatıcısına otomatik geçilmez: ilk kit ve başlatıcı korunur; seçici uydurulmaz.

## Atomik program yayını

Seçili kurtarma programı hem güncellemede hem geri almada
`/opt/celikpanel/bin` ve `/opt/celikpanel/web` dizinlerini yayımlar. Kapalı iç
komutlar yalnız kaynak, snapshot ve korunan aday manifest kimliğini kabul eder;
işlem token'ını parametreden almaz, mevcut yerel işlemden okur. Çalışan programın
hash'i kaynak değişikliğinden önce seçili kitle aynı olmalıdır.

Tam özel dizin doğrulanıp fsync edildikten sonra root erişimli değişmez kaynak
niyeti yazılır. Aynı dosya sistemindeki atomik dizin takası bütün dizini yayımlar
ve çıkarılan dizini korur. Tekrar giriş, önceki/sonraki inode, içerik ve metadata
çiftini doğrular. Bilinmeyen ekler, bağlantılar, sahip değişiklikleri ve kanıtsız
eksik dizinler korunup reddedilir. Yalnız panel/Agent değişirken kabul edilmiş ek
bin dosyaları korunur. İlk kurulum kendi kabul yolunu kullanır; güncelleme snapshot'ı
uydurmaz.

`celikpanel/recovery-resource-intent/v1` şeması yerel token özeti, snapshot v6
manifesti ve aday manifestine bağlıdır. Kesilen staging ve çıkarılmış dizinler
`.recovery-publications` altında kanıt olarak kalır; bu dilimde otomatik temizleme
yoktur. Eski, kayıtsız kurucuların kısmen yazdığı dosyalar geriye dönük olarak
sahiplenilmez.


Yayın, sınırlandırılmış `user.*` genişletilmiş özelliklerini korur ve tekrar girişte
doğrular. Protokol 1 ACL, dosya yetenekleri ve SELinux etiketlerini kabul etmez;
salt-okur kaynak taraması bunları koordinatörler durmadan önce bildirir. SELinux
hedef politika geçişinin kalıcı niyette tanımlanmasını gerektirir; yedek yolunun
etiketini kopyalayıp sonra `restorecon` çalıştırmak geçerli yayın kanıtı değildir.
Güncellemeyi geçirebilmek için sahip metadata'sı sessizce kaldırılmaz.

Yeniden başlatmadan sonra kurtarma, eksik `/run/celikpanel` dizinini yalnız tam
kabul edilmiş geri alma ve iki koordinatörün durduğu kanıtıyla oluşturabilir.
Mevcut dizinin özelliklerini düzeltmeye kalkmaz. Tamamlama bekleyen veritabanı
kontrolü mevcut WAL'ı özel kopyada okur; canlı DB/WAL'ı değiştirmez veya yeniden
geri yüklemez. WAL ve atomik yayın kesintileri gerçek alt süreç öldürülerek sınanır.

Gerçek deneyler ve uygulanamayan hata girişimleri
[bağımsız kurtarma kabul kaydında](../deploy/e2e/release-recovery/INDEPENDENT-RUNTIME.tr.md)
ayrı sonuçlarla tutulur.

Tamamlanmış yedekten geri alma ayrıca [bağımsız kurtarma verisini](RECOVERY-MATERIAL.tr.md) ve ona bağlı v2 yayınlama kaydını kullanır. Eski v1 işlemleri mevcut okuyucuyu korur; kalan kabul matrisi tamamlanmış sayılmaz.

## İşletim sistemi geçişinde kurtarmayı erteleme

Açılışta etkin olan kurtarma oneshot servisi, `multi-user.target` hedefine ulaşmanın
bir parçasıdır. İçinde systemd açılışının bitmesini beklemek, açılışın kendisini
engelleyebilir. Runner artık doğrulanmış güncelleme/geri alma alt sürecini başlatmadan
önce tek ve süre sınırı olan salt-okur hazırlık sorgusu yapar. `running` (çıkış 0)
ve `degraded` (çıkış 0 veya 1) başlatmaya izin verir. Çıkış 1 ile `initializing`,
`starting` veya `stopping` ise başlatmayı erteler: işlem kilidi bırakılır; işlem
işaretçileri, snapshot ve istek bağlantısı değiştirilmeden çağrı sonlanır. Mevcut
yerel zamanlayıcı çağrı bittikten sonra aynı işlemi yeniden ele alır. Sorgunun
zaman aşımı beş saniye, zorla sonlandırma ek sınırı bir saniyedir. Bilinmeyen çıktı,
beklenmeyen çıkış kodu veya başarısız/zaman aşımına uğramış sorgu kurtarmayı başlatma
yetkisi vermez; günlük incelenecek hazırlık kontrolünü açıklar. Alt sürecin değişiklik
anındaki platform ve kilit kontrolleri zorunlu kalır. Salt-okur son durum doğrulaması,
kurtarma başlatmadan veya gözlem yazmadan önce bekleyen işlem işaretçilerini reddeder;
ertelemeyi tamamlanma kanıtına çeviremez.

Ertelenen çağrı, tamamlanmış kurtarma değildir. Günlük beklenen önkoşulu ve otomatik
sonraki adımı açıklar. Tam isteğe bağlı gözlem `recovering`, `terminal_proof=none`
olarak kalır; önceki hata korunur. İlk hazır olma düzeltmesi kalıcı şemayı, kit
protokolünü veya veritabanı sürümünü değiştirmedi. İlk runner değişikliği tarayıcıda yalnız genel kurtarma durumunu gösteriyordu.
Sonraki [isteğe bağlı bekleme gözlemi](RECOVERY-ACCESS.tr.md), ana v1 kaydını
değiştirmeden uyumlu arayüz/CLI açıklaması ekler.

Bu çalışma D-025 ilkeleri 2–5 ve P0.2/P0.3 kapsamındadır. Gerçek runner sözleşme
testleri; ilk açılışta hata uydurulmamasını, bilinen hatadan sonra üç bekleme durumunu,
istek/işaretçi kanıtlarının korunmasını, kilidin bırakılmasını, aynı işlemin sonradan
tamamlanmasını, degraded durumunu, bozuk/bilinmeyen yanıtları ve zaman aşımını sınar.
Geri alma giriş testleri gerçek devralınmış kilit kontrollerini korur. Bu testlerde
systemctl ve geri yükleme alt süreci modellenir; **değişen runner için gerçek yeniden
başlatma kabul kanıtı değildir**. Debian X ve Arch Z eski runner ile çalışmıştır;
açılış hataları tarihsel kanıt olarak korunur. Aşağıdaki ayrı AB sonucu bir Arch
sınırını kanıtlar; geniş kontrol noktası/hizmet matrisi açık kalır.

[Değişen kaynakla AA denemesi](../deploy/e2e/release-recovery/NATIVE-EXCHANGE-RECOVERY.tr.md), izleyici uyumluluk kontrolünde beklediği için iki hata sınırına da ulaşamadı. Sonuçsuz ölçüm, tamamlanmamış detach ve durdurulan misafirler kayıtlı kalır.

Yeni [AB Arch kabulü](../deploy/e2e/release-recovery/NATIVE-EXCHANGE-RECOVERY.tr.md),
değişen adayı ve ayrı kaynak kimliğiyle sabitlenmiş düzeltilen izleyiciyi kullanır.
Exchange kesintisi ve yerel geri almanın `payload_restored` aşamasındaki reset
sonrası iki açılış çağrısı hata vermeden erteler; yerel zamanlayıcı aynı geri almayı
tamamlar. 55 tablodaki 102 eski satır, DNS ve HTTPS dar kapsamlı denetimleri geçer.
Ayrı [AD Debian kabulü](../deploy/e2e/release-recovery/NATIVE-EXCHANGE-RECOVERY.tr.md),
aynı iki kesinti sınırını geçer: bir açılış ertelemesi, açılış sonrası sıfır
kurtarma hatası ve aynı geri almanın otomatik tamamlanması. 55 tablodaki 100 eski
satır, DNS ve HTTPS denetimleri geçer; sonradan eklenen bir metrik satırı ayrı
kaydedilir. AC'nin önceki hazırlık hatası korunur. Sonraki
[AJ kabulü](../deploy/e2e/release-recovery/BOUND-WORKER.tr.md), iki gerçek kesintiden
sonra gerçek worker→snapshot→CLI/kimlik doğrulamalı HTTP/tarayıcı nihai okuyucu
bağını sınırlı kapsamda kanıtlar. AJ'de yerel `waiting_for` ipucu gözlenmedi.
Ayrı [AK deneyi](../deploy/e2e/release-recovery/BOOT-WAIT.tr.md), gerçek `starting`
yayını, root CLI yönlendirmesi, bırakılmış kilit ve aynı isteğin otomatik yeniden
denenerek doğrulanmış geri almaya ulaşmasını kanıtlar. Diğer beklemeler, beklerken
HTTP/tarayıcı erişimi ve geniş P0 matrisi açıktır.
AB/AD ve AJ, kayıtlı sınırlarında sonunda kurtarmayı kanıtlar; kesintisiz erişim
veya güç kaybı dayanıklılığı değildir.


## Otomatik deneme sınırı (21 Eylül 2026)

[Kalıcı deneme sınırı](RECOVERY-DISPATCH-BUDGET.tr.md), aynı snapshot için
üç otomatik alt işlem ve sonrasında açık sahip tekrar? uygular.
Bekleme ve salt-okur sorgular hak tüketmez. Bu yeni kaynağın shell/Go
sözleşmeleri geçmiştir; sınırlı gerçek sistem sonucu aşağıda kaydedilmiştir.

[Debian AL deneme sınırı kabulü](../deploy/e2e/release-recovery/DISPATCH-BUDGET.tr.md),
yeni kitin yeniden başlatmaya rağmen üç kesilmiş girişimi koruduğunu, otomatik
kurtarmayı durdurduğunu ve tek açık sahip tekrarıyla aynı geri almayı tamamladığını
kanıtlar. Yayın sınırındaki güç kaybı, tarayıcı yönlendirmesi ve kalan P0.2/P0.3
matrisi açıktır.

## Boşta denetim çalışmaları ve saatlik kit kontrolü (alpha.126)

Yerel zamanlayıcı `/usr/libexec/celikpanel/release-recovery` dosyasını yaklaşık
31 saniyede bir başlatır. alpha.125'e kadar her çalışma, bekleyen bir işlem
olmasa bile seçili kitin tamamını doğruluyordu (yaklaşık 45 MB üzerinde birkaç
SHA-256 geçişi, iki Go ve iki bash başlatması): denemede çalışma başına 0,35–1,3 sn
işlemci, günde yaklaşık 18–58 işlemci-dakikası ve İzleme ekranındaki işlemci
örneğinin sahte tepe olarak yakalayabildiği kısa yükler.

**Hızlı yol.** Aşağıdakilerin hepsi doğruysa betik ilk satırlarında, yalnız bash
yerleşik komutlarıyla çıkar: argümansız başlatılmıştır ve kit girişi değildir;
`/var/lib/celikpanel-release-transaction` yoktur ya da içinde yalnız normal bir
`transaction.lock` vardır; `recovery-promotions/v1/current` yoktur ya da içinde
yalnız tamamlanmış `intent.json` ve `committed.json` vardır; ve
`/run/celikpanel-release-recovery/idle-verified`, tam yolun bu açılışta, şu an
seçili kit için, bir saatten kısa süre önce "bekleyen işlem yok" sonucunu
doğruladığını gösterir. Herhangi bir işaretçi, başka ya da okunamayan bir girdi,
sembolik bağlantı, yarım kalmış kit geçişi, değişmiş seçim ya da eksik, eski veya
başka açılışa ait damga değişmeyen tam yola düşer. Sahibin yeniden denemesi ve
son durum kanıtı hızlı yolu hiç kullanmaz. Hızlı yol hiçbir şey yazmaz, kilit
almaz ve hiçbir işlem başlatmaz.

**Bütünlük kontrolü.** Damgayı yalnız tam yol, doğrulanmış "bekleyen işlem yok"
çıkışlarında ve sürüm kilidi altındayken yazar. `/run` her açılışta boşalır; bu
yüzden her açılışın ilk çalışmaları ve saatte en az bir çalışma kiti tamamen
doğrular. Başarısız doğrulama damga yazmaz; sonraki her çalışma yine tam olur ve
yine başarısız olur, birim de başarısız kalır. Günlükteki ilk satır aynı kalır;
ardından İngilizce ve Türkçe olarak hiçbir kurtarma adımının başlamadığı, sunucu
sahibinin dosyaları değiştirmeden `sudo /usr/libexec/celikpanel/recovery
runtime-status --lang tr` çalıştırması gerektiği ve kit doğrulandığında hatanın
kendiliğinden kalkacağı yazılır. İki tam kontrol arasında bozulan bir kit bir
sonrakinde (en geç bir saat sonra ya da sonraki açılışta) bulunur; bekleyen bir
işlem bunu hiç beklemez, çünkü işaretçi her zaman tam yolu çalıştırır.

**Daha az tekrar özet.** Kurtarma gerçekten çalıştığında kit girişi, az önce
bitmiş tam kanıtı tekrarlamaz; başlatıcı da iki tam yeniden doğrulama arasında
seçili çalıştırılabilir dosyayı bir kez daha özetlemez. Diğer kontroller değişmedi.

**Geçiş.** Yalnız betiğin baytları ve kitin Go programı değişir. Betik
`update.sh`'nin mevcut tek yönlü temel yayın adımıyla kurulur; birim dosyaları,
temel bildirim biçimi, kit protokolü ve veritabanı değişmez. Geri alma bu betiği
korur. Eski bir sürümün seçili kiti damga yazmaz; onun çalışmaları yalnızca tam
kalır.

**Kanıt.** Sözleşme testi 15 işaretçi birleşiminin hepsini, beklenmeyen adları,
gizli dosyaları, dizinleri, kilit, kök ve kit geçişi türevlerini, eski, ileri
tarihli, başka açılışa ait ve bozuk damgaları, değişmiş seçimi, sahibin yeniden
denemesini, son durum kanıtını ve kit girişini çalıştırır; hepsi tam yola ulaşır.
Debian 13 deneme makinesinde (2 vCPU) boşta çalışma 349–397 ms'den 11–15 ms'ye
(SHA uzantısı kapalıyken 1161–1291 ms'den 11–12 ms'ye) indi; tam çalışma 304–317 ms
(847–866 ms) oldu. Bekleyen `active` işaretçisi ve taze damga ile yapılan bir
gerçek yeniden başlatmada açılışta (7,2 sn, 408 ms) ve her zamanlayıcı tekrarında
tam yol çalıştı; işaretçi kaldırılınca sonraki çalışma damgayı yazdı. Bozulmuş bir
kit baytı, düzeltilene kadar her tam çalışmayı başarısız kıldı. Bu deneme anlık
görüntüsü olmayan bir işaretçi kullandı; açılışta kurtarmanın başladığını kanıtlar,
tamamlanmış bir geri almayı değil. Geniş P0.2/P0.3 matrisi ve olaya bağlı yol
birimi açıktır.
