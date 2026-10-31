# Üyelik ve yıllık sunucu lisansı

*Güncel politika uyumu: 30 Ekim 2026 (yedi günlük çevrimdışı süre, D-039) · [English](MEMBERSHIP_LICENSING.md) · D-025*

Bu belge yıllık lisans süresini, erişim doğrulama aralığını (D-039'dan beri
çevrimdışı yedi gün; sahip 2026-10-29'da onayladı) ve geçmiş davranışı ayırır.
Mevcut kodu ve gereken kurtarma çalışmalarını kaydeder; kurulu bir panelin
güncellenmesine izin vermez.

Onaylanan ürün modeli: tek CelikPanel ürünü, mevcut dört yetki rolü ve panel
yöneticilerinden bağımsız bir celikpanel.net üye hesabı. Başlangıçta doğrulanmış
bir üye ücretsiz sunucu lisansları oluşturabilir. Her lisans, doğrulanmış tek bir
genel sunucu IP adresine bağlanır ve oluşturulduğu tarihten itibaren bir takvim
yılı geçerlidir. Önceden etkinleştirilmiş lisansların verilmiş bitiş tarihleri
korunur; eski, etkinleştirilmemiş lisanslara mevcut veriyi koruyan geçiş sırasında
oluşturulma tarihinden bir takvim yılı sonrası atanır. Etkinleştirmeyi yeniden
denemek özgün süreyi korur. Sunucu taşıma işlemi süreyi korur ve üyenin parolasını
gerektirir. Henüz ödeme veya otomatik faturalama yoktur.

Merkezi hizmet, mevcut portal sunucusunda PHP 8.3/PDO SQLite kullanır.
Veritabanı, oturumlar ve Ed25519 lisans imzalama sırrı hem httpdocs dışında hem de
atomik web sitesi sürümlerinden ayrı tutulur. Her portal sarmalayıcısı, değişmez
bir özel uygulama sürümünü kaynak manifestosunun özetiyle sabitler; böylece web
sitesini geri almak önceki uygulama sürümünü de seçer. Desteklenen portal yayım
aracından önce deploy/stage-membership.py ile hazırlık yapılır; özel durum
verileri portal arşivine hiçbir zaman eklenmez. Sürüm imzalama ve lisans imzalama
anahtarları ayrıdır. İstemciler imzalı yetki belgeleri alır; veritabanına erişmez.
E-posta, yapılandırılmış ve kimlik doğrulaması kullanan SMTP taşımasıyla gönderilir.
Doğrulama ve parola sıfırlama belirteçleri kısa ömürlü ve tek kullanımlıktır;
özetleri saklanır ve erişim URL'lerine hiçbir zaman yazılmaz.

Üye arayüzü: kayıt, e-posta doğrulama, oturum açma, parola sıfırlama, lisans
listesi, lisans oluşturma, anahtar kopyalama, yenileme ve sunucu bağını kaldırma.
Portalın mevcut lacivert/beyaz tipografisi korunur; Türkçe ve İngilizce sunulur.
Lisans anahtarları, sahibinin parolası doğrulandıktan sonra yeniden gösterilebilir.
Saklanan özetler etkinleştirmeyi doğrular; kimlik doğrulamalı secretbox şifrelemesi
anahtarın yeniden gösterilmesini sağlar. Saklama anahtarı, özel lisans imzalama
sırrından ayrı bir kullanım alanı için türetilir. İmzalama sırrı değiştirilirken
eski sır kullanımdan kaldırılmadan önce mevcut anahtarlar yeniden şifrelenmelidir.
Yalnız özeti saklanmış eski anahtarlar yeniden oluşturulamaz: geçerli bir
etkinleştirme sırasında verilen anahtar saklanır veya sahibi mevcut anahtarını
değiştirmeden açıkça kaydedebilir. Panel kimlik bilgileri müşterinin sunucusunda
kalır ve oluşturulmaları hâlâ terminalden yapılır.

Lisans doğrulaması, panel kimlik doğrulamasından ayrıdır. Kurucu, lisansı ekrana
yazdırmadan ve komut satırı argümanlarına koymadan ister. Kuruluma devam edilmeden
önce etkinleştirme kaydedilir; kesintiye uğrayan kurulum aynı kimliği ve yetki
belgesini yeniden kullanır. Panel yöneticileri Ayarlar'dan etkinleştirme ve
yenileme yapabilir. Güncel HTTP erişim kontrolü, aşağıda tam olarak listelenen
kurtarma istisnalarıyla birlikte dört rolün tamamında normal panel yönetimine
uygulanır. Yalnız yeni kaynak oluşturmaya ilişkin eski açıklamalar ve geniş bakım
istisnaları geçmiş davranışı anlatır; güncel API sözleşmesi değildir. Merkezi
hizmet kesildiğinde mevcut imzalı kayıt korunur; kesinti bu kaydın fiilî erişim
son tarihini uzatamaz (son başarılı kontrolden en çok yedi gün sonra, hiçbir zaman
yıllık bitişten sonra değil).

Sunucu taşıma, anahtar değiştirme ve lisans kaldırma eski bağı merkezi olarak
geçersiz kılar; panel bunu bir sonraki kontrolünde, kullanımdayken yaklaşık saatte
bir öğrenir. Önbellekteki imzalı kayıtlar yalnız aşağıda açıklanan fiilî erişim son
tarihine kadar kullanılabilir. Yıllık süre hâlâ geçerliyken bu son tarih aşılmışsa
doğrulama hizmetine erişilememesi `offline_ended` (ya da aşağıdaki `clock_behind`)
nedeniyle `verification_unavailable` durumunu oluşturur. Erişim uç noktası bu
nedenle birlikte `can_use_panel=false` yanıtı verir; tarayıcı kurtarma ekranında
nedeni, kimin ne yapacağını gösterir ve etkinleştirme sayfasına yönlenmez. Bu HTTP
erişim kontrolü mevcut iş yüklerini durdurmaz.

Yayımdan önce: etkinleştirme, taşıma ve yenileme işlemlerinin atomikliğini;
kimlik doğrulama, CSRF ve belirteçlerin kötüye kullanımını; imzalı yetki belgesini
ve sunucu bağını; kaynak oluşturma uç noktalarının kapsamını; kesintiye uğramış
kurulumu ve rollerin ayrılığını doğrulayan testler gerekir. Ayrıca masaüstü ve
mobilde Türkçe/İngilizce için sınırları belirli bir tarayıcı incelemesi ve bir
düzeltme turu yapılır. Üretimde e-posta teslimi ve PHP web işleyicisinin
çalışabilirliği açıkça doğrulanmalıdır; CLI PHP'nin bulunması web işleyicisinin
çalıştığını tek başına kanıtlamaz.

## Güncel uygulanan erişim politikası

Kaynaklar: [lisans yöneticisi](../internal/licensing/license.go),
[panel erişim kontrolü ve istisnaları](../cmd/panel/license.go) ve
[tarayıcı erişim kontrolü](../web/src/components/LicenseOnboarding.tsx).
Yıllık lisans süresi ile bu erişim doğrulama aralığı ayrıdır:

| Karar | Güncel uygulama |
|---|---|
| Fiilî yetkilendirme son tarihi | Yıllık bitiş tarihi, imzalı `offline_until` ve `issued_at + 7 gün` değerlerinin en erken olanı. Hizmet `offline_until = min(şimdi + 7 gün, bitiş)` imzalar. D-039 öncesi hizmetin kaydı (`offline_until = issued_at + 60 sn`) tam o bir dakikayı korur. |
| Olağan yenilemenin yapılabileceği zaman | İmzalı `refresh_after` (hizmet: `şimdi + 1 saat`) ile `issued_at + 1 saat` değerlerinin erken olanından, sunucuya özgü belirlenimci sapmayla beşte bire kadar önce (48–60 dakika). Yalnız kimliği doğrulanmış kullanım tetikler; boştaki kurulumu sorgulayan sunucu zamanlayıcısı yoktur. Eşzamanlı istekler tek yenilemeyi paylaşır. |
| Zamanı gelen yenilemede isteğin beklemesi | Yönetimi artık açmayan kayıt ortak yenilemeyi bekler. Hâlâ geçerli kayıt en çok 3 saniye bekler, sonra istek o kayıtla devam eder ve yenileme arka planda biter; erişilebilen hizmetin reddi bu yüzden onu saptayan isteğe uygulanır. |
| Başarısız yenilemeyi yeniden deneme | Kayıt geçerliyken 1, 2, 4, 8, 16, sonra 30 dakika (her biri sunucuya göre beşte bire kadar kısalır) ve hiçbir zaman kaydın geçersizleşeceği andan sonra değil. Geçersizleştikten sonra kullanımda 30 saniyede bir. Yöneticinin "Tekrar kontrol et" isteği beklemeyi atlar; imza veya bağ doğrulamasını atlamaz. |
| Açık ret | Yenilemede `invalid_license`, `invalid_activation`, `license_in_use` veya `license_expired` hemen kalıcı kaydedilir (`rejected`) ve çevrimdışı süre içinde de dışında da yönetimi hemen kapatır. |
| Saat güvenliği | `issued_at` hizmetin saatidir. Sunucu saati `issued_at`'tan 5 dakikadan fazla önceyse, kayıtla saklanan en yüksek saat okumasının (`seen_at`, yerel, imzasız) 5 dakikadan fazla altındaysa ya da panel çalışırken geri alındıysa (duvar saati ile monoton zaman karşılaştırılır) bir kontrol başarılı olana kadar `clock_behind` nedeniyle `verification_unavailable` olur; erişilebilen hizmete hemen sorulur. Bu saatten 5 dakikadan fazla sonra tarihli yeni yanıt `clock_behind` kontrol sonucuyla reddedilir. |
| HTTP yenileme süresi sınırı | Lisans istemcisinin zaman aşımı 15 saniyedir; ortak arka plan yenilemesi 20 saniyeyle sınırlıdır. |
| Eksik, süresi dolmuş, reddedilmiş veya doğrulanamayan yetki belgesi | Normal yönetim reddedilir. Erişim uç noktası durum, gözlem ve `verification_unavailable` için neden ile `can_use_panel=false` yanıtı verir. Lisans sayfası (`/api/v1/panel/license`, yöneticiler) ayrıca `check` (son başarısız deneme: `unreachable`, `rate_limited`, `invalid_answer`, `clock_behind`), `offline_for_seconds` ve `verified_seconds_ago` bildirir. |
| Tarayıcı davranışı | Başarısız bir erişim isteğinde yalnız aynı kullanıcı için hâlâ geçerli olan karar, son tarihi uzatılmadan korunur. Bilinmeyen erişim durumunda bağlantıyı kurtarma görünümü gösterilir; sunucunun reddi veya `license_required` yanıtı yönetimi kilitler. |

Kimlik doğrulaması gerektiren güncel istisnalar bilinçli olarak dardır. Rol,
sahiplik ve yöntem kontrollerinin tamamı uygulanmaya devam eder:

- Oturum kimliği ve erişim durumu okumaları; çıkış yapma, parola değiştirme ve
  başka kullanıcı adına açılan oturumu sonlandırma kullanılabilir.
- Lisans okumaları, etkinleştirme ve yenileme yöneticiler için kullanılabilir.
- Yöneticiler panel sürümünü, güncelleme denetimini ve durumunu, ayrıca sunucunun
  değişiklik işlemine hazır olup olmadığını okuyabilir; imzalı güncellemenin
  başlatma ve vazgeçme uç noktalarını kullanabilir. Sürüme güven, işlem kimliği
  ve değişiklik işlemine kabul kontrolleri uygulanmaya devam eder. Kurulu paneldeki
  her güncellemeyi kullanıcı panel arayüzünden başlatmalıdır; API'nin bu yeteneği
  sunması, asistanın güncellemeyi kurmasına izin vermez.

Bu istisnalar, yetki belgesi yönetime izin veremediğinde yedekleme, silme, hizmet
yapılandırması veya diğer bakım işlemlerine **genel panel erişimi sağlamaz**.
Mevcut yerel iş yükleri ve bağımsız zamanlamaları bu HTTP lisans kontrolüyle
durdurulmaz. Yönetim programları kaldırıldıktan sonra tam çalışabilirlik ise
ayrı ve henüz tamamlanmamış bir
[sunucu sahibinin bağımsızlığı gereksinimidir](OWNER-INDEPENDENCE.tr.md).

Fiilî son tarihten sonra doğrulama hizmetine erişilememesi, yöneticide
`verification_unavailable` durumunu oluşturur; bu, yıllık lisans süresinin
dolduğunun kanıtı değildir. Bu kurulumun yenileme isteğinin açıkça reddedilmesi,
yerel imzalı kaydını geçersiz kılabilir. Yeni anahtarın yanlış yazılması, mevcut
geçerli bağı iptal etmemelidir. Yedi günlük süre çevrimdışı payın tamamıdır;
hiçbir şey onu uzatmaz.

Saatin geri alınması, açıkça: süre hizmetin `issued_at` değerine bağlıdır ve
imzalı `offline_until` ile sınırlıdır; saati ileri almak işe yaramaz, `issued_at`
öncesindeki saat reddedilir. Süre içinde saatin geri alınması panel çalışırken
saptanır; yeniden başlatmada ise `seen_at` sıklığına kadar saptanır (her başarılı
kontrolde ve her başarısız denemede yazılır; kesintide kullanımdayken en az 30
dakikada bir). Sunucunun sahibi kendi sunucusunda root'tur: eski lisans dosyasını
geri koyup saati de geri almak ya da programı değiştirmek yerel bir kontrolün
durdurabileceği şey değildir; bu D-039 öncesindeki sınırın aynısıdır, yalnız süre
bir dakika yerine yedi gündür.

## Gereken kurtarma davranışı — henüz tamamen uygulanmadı

[D-025 ve dayanıklılık sözleşmesi](RESILIENCE-CONTRACT.tr.md), türleri açıkça
ayrılmış erişim nedenleri ile kapsamları ayrı belirlenmiş, kimlik doğrulamalı
durum okuma ve kurtarma yetenekleri gerektirir. Başarısız bir gözlem farklı bir
teşhise dönüştürülmemelidir: doğrulama hizmetinin erişilemez olması, sahibin yeni
bir lisans anahtarına ihtiyacı olduğu anlamına gelmemeli; başarısız oturum sorgusu,
kimlik doğrulanmadığını kesinleştiren bir yanıt olarak ele alınmamalıdır.

Güncel mantıksal erişim yanıtı ve ortak `license_required` reddi, bu ayrımı ön yüz
ile arka uç arasında korumaz. Normal Agent'tan ve aday sürümün başlamasından
bağımsız, kimlik doğrulamalı bir kurtarma/durum arayüzü de tamamlanmayı bekler.
Uygulaması; kimliği, sunucu ve işlem kapsamını, hassas veri sınırlarını, güncel
lisans son tarihlerini ve paneli yalnız kullanıcının güncellemesi kuralını
korumalıdır. Bu gereksinim, yetki belgelerinin süresini uzatmaya, lisanslamayı
atlamaya veya sınırsız ayrıcalıklı işlemler sunmaya izin vermez.

## Geçmiş etkinlik politikası (Alpha 59) — güncel politikayla değiştirildi

Aşağıdaki zamanlama ve işlem kabul açıklaması Alpha59 davranışını kaydeder.
Eski kurulumları ve imzalı kayıtları açıklamak için korunur; mevcut kod için
yukarıdaki güncel politika esas alınır. Günlük yenileme, hata sonrası 15 dakika
bekleme, yedi günlük önbellek erişimi ve geniş bakım erişimi artık güncel istemci
sözleşmesi değildir.

Alpha59'da başlangıçta veya saatlik lisans sorgusu yoktu. Yönetici, bayi, müşteri
ve ek kullanıcı oturumlarından gelen yetkili API istekleri ortak bir etkinlik
izleme yolunu kullanıyordu. İmzalı günlük yenileme son tarihinden sonraki ilk
istek, eşzamanlı istekleri tek yenilemede birleştiren ve süresi sınırlı bir
yenileme başlatıyordu. Hata sonrası bekleme 15 dakikaydı; boşta yeniden deneme
zamanlayıcısı yoktu. Kaynak oluşturma işlemleri yerel imzalı kaydı kullanıyor;
oturum açma ve bakım erişimi açık kalıyordu. Bu paragraf geçmiş davranışın
kaydıdır; bugün bu istisnaların bulunduğu vaadi değildir.

## IP bağı ve korunan imzalı kayıtların uyumluluğu

Merkezi API kaynak IP'yi yalnız web sunucusunun REMOTE_ADDR değerinden alır;
istemci JSON'undan veya yönlendirme başlıklarından almaz. Önündeki vekil sunucu,
gerçek bağlantı IP'sini güvenilir biçimde sağlamalıdır. IPv4 eşlemeli IPv6
adresleri normalleştirilir. Özel ve ayrılmış kaynak adresler reddedilir.
Geliştirme için açıkça tanımlanmış yerel döngü e-posta test düzeneği, yerel
bağlantı adresini sabit bir genel test IP'sine eşler; bu özellik üretimin HTTPS
adresinde etkin değildir.

Sabit genel IP, fiziksel donanımı değil lisansın sunucu atamasını tanımlar.
Paylaşımlı NAT sunucuları ayırt etmeye uygun değildir; özel, sabit bir çıkış IP'si
kullanılmalıdır. Çift yığınlı bağlantıda çıkış adresinin değişmesi de diğer IP
değişiklikleri gibi bağın değiştirilmesini gerektirir. Kurulu istemcilerle
uyumluluk için imzalı kayıtlar v1 biçiminde ve yerelde makineye bağlı kalır.
Aynı bağlı IP'de anahtarla yeniden kurulum yapmak, anahtarı ve bitiş tarihini
koruyarak makine bağını değiştirir ve önceki yenileme kimlik bilgisini geçersiz
kılar. İmzalı v1 biçimi, yıllık bitiş tarihiyle sınırlı olmak üzere oluşturulmadan
sonra yedi güne kadar çevrimdışı son tarih taşıyabilir; D-039'dan beri hizmet tam
bunu imzalar ve güncel paneller uygular. v0.1.0-alpha.64'ten (#134) D-039'u taşıyan
sürüme kadarki paneller yedi günlük kayda yine kendi bir dakikalık sınırlarını
uygular; alpha.64 öncesi paneller imzalı süreyi zaten uyguluyordu. Eski kayıt biçimleri, güncel
istemciye daha uzun bir yetkilendirme aralığı sağlamaz.
Eski makineye bağlı lisanslar, özgün makine ve anahtar/yenileme kimlik bilgisi
kanıtlandıktan sonra gözlemlenen IP'ye bağlanır; önceden yeniden kurulmuş eski bir
sunucuda sahibi önce eski bağı kaldırmalıdır.

Sunucuyu/IP'yi değiştirmek üye parolasını gerektirir; bağı temizler ve kimlik
bilgisinin nesil numarasını artırır, ancak anahtarı ve süreyi korur. Anahtar
değiştirme ayrı bir işlemdir; IP'yi ve süreyi korur. Bu işlemlerin hiçbiri mevcut
iş yüklerini durdurmaz.
