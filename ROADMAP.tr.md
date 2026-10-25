# CelikPanel Yol Haritası

*Son güncelleme: 10 Ekim 2026 · [English](ROADMAP.md)*

---

## Anayasa — Her Kararın Süzgeci

Her özellik, commit ve tasarım kararı bu gereksinimleri karşılamalıdır.
Bunlar mevcut uygulamanın karşıladığı iddiası değil, bağlayıcı yükümlülüklerdir.
[Dayanıklı çalışma sözleşmesi ve kaynak incelemesi](docs/RESILIENCE-CONTRACT.tr.md),
açık P0 işlerini, uygulama sırasını ve kapatılmaları için gereken kanıtı kaydeder (D-025).

### 1. Güvenlik ve sunucu sahibinin yetkisi
- Yönetim erişiminde kimlik doğrula; her eylemi ilgili kaynaklar için yetkilendir. En az yetki, yerel kimlik doğrulamalı IPC, parametreli SQL ve gizli değerler için `crypto/rand` kullan.
- Root yetkisi olmayan Panel ile ayrıcalıklı Agent ayrı kalır. Normal ayrıcalıklı otomasyon yetkili Agent API'sini kullanır; desteklenen kullanıcı kurtarmasının ayrı ve dar bir sözleşmesi vardır. Yapay zekâ planlayıcısı veya kurtarma ekranı sınırsız root çalıştırma yetkisi kazanmaz.
- Sunucu sahibi hizmetleri kendi araçlarıyla yönetebilir. Sahibinin değişikliklerini algıla ve açıkça uzlaştır; önbellekteki hedefe uydurmak için sessizce üzerlerine yazma (D-022).
- Kanıt belirsizse ilgili güvensiz değişikliği engelle. Doğrulanmamış yetki vermeden, kimlik doğrulamalı tanı ve desteklenen kurtarma erişimini koru.

### 2. Süreklilik ve kurtarılabilirlik
- Panel kesintisi, lisans kaybı veya panelin kaldırılması; barındırılan işleri ve hizmetlerin kendi yenileme, zamanlama ve açılış mekanizmalarını durdurmamalıdır. Bağımsızlık iddiasından önce kalan bağımlılıklar giderilip sınanmalıdır.
- Her değişiklik; etkilenen kaynakları, salt-okur ön kontrolü, kalıcı kontrol noktalarını, sınırlı tekrar/kurtarmayı ve sonuç kanıtını tanımlar. İzin veya sahiplik normalleştirmesi de değişikliktir.
- Aday sürüm, normal Agent, uygulama şema geçişi veya lisans doğrulayıcı çalışmadığında kurtarma kullanılabilir kalmalıdır. Karışık ya da doğrulanmamış uygulama durumu normal yönetimi engelleyebilir; bağımsız kurtarma yolunu ortadan kaldıramaz.
- Aday sürüm ve kurtarma uyumluluğu doğrulanana kadar son doğrulanmış kullanılabilir durum ile kurtarma malzemesi korunur. Temizlik bu kanıttan sonra gelir.
- Otomatik onarım, kabul edilen işlemin kuralları belli ve tekrarlandığında ek etki yaratmayan devamı veya telafisidir. Eksik kanıt uydurmaz, sahibinin sonraki değişikliklerini geri almaz, doğrulamayı atlamaz veya sonucu belirsiz ikinci bir değişiklik başlatmaz.

### 3. Gerçeği yansıtan durum ve ortak sözleşmeler
- Sahibin niyeti, yetki, yayımlanmış yapılandırma, gözlem, yürütme, doğrulama ve kurtarma ayrı tutulur. Bilinmeyen; yok, başarısız, süresi dolmuş veya tamamlanmış değildir.
- Her kalıcı çıktının sürümlü tek bir üretim/okuma/geri yükleme sözleşmesi ve açık desteklenen geçişleri vardır. Kanıt rolüne göre karşılaştırılır; geçerli yayımlama kaydın yalnız bir bölümünü ilerletiyorsa bütün kayıt eşitliği aranmaz.
- Tamamlanmış kurulum adımı geçmiş yürütmenin kanıtıdır; güncel sağlık kanıtı değildir. Mevcut neden, sorumlu kişi, sonraki eylem ve aynı işlemin nasıl süreceği açıklanır (D-024).
- Tarayıcı yetkili işlem durumunu gözlemler. Yenileme, yeniden bağlanma ve zaman aşımı ikinci işi yetkilendirmez veya tamamlanma anlamına gelmez.

### 4. Sadelik
- Her olağan işin kullanıcı için tek ve açık yolu olur. Tarayıcı ile desteklenen kullanıcı kurtarması aynı işlem sözleşmesini paylaşır; tek ekran, kurtarmanın tek arıza noktası olamaz.
- Özellik somut kullanıcı ihtiyacı için eklenir. Kabul edilen kapsam içinde güvenli varsayılanlar kullanılır; gelişmiş seçimler gerektiğinde gösterilir.
- Kullanılmayan hizmetlerin özel menüleri arayüzü doldurmaz. Kullanıcının işi tamamlamasına yardımcı olan kurulum keşfi, gerçek çakışmalar ve kurtarma eylemleri görünür kalır.
- Olağan işler panelden yapılabilmelidir. Sahibin kendi araçlarıyla yönetimi ve kurtarması desteklenir; elle kurtarma, kurtarmayı yasaklama gerekçesi değil otomasyon açığının kanıtıdır.

### 5. Kanıtla ölçülen hız
- API yanıtında 100 ms altı, hızlı arayüz tepkisi ve asgari kurulumda 60 saniye hedefleri korunur. Bir hedefin karşılandığı söylenmeden ölçülen platform ve kapsam kaydedilir.
- Çalışma ortamı küçük tutulur. Mevcut Panel/Agent yetki ayrımı, hizmetlerin kendi yaşam döngüsü ve bağımsız kurtarma mekanizması tek binary söyleminden önce gelir.
- Hız; doğrulamayı, kurtarılabilir kontrol noktalarını veya arıza testlerini atlamayı haklı çıkarmaz.

### 6. Esneklik ve bağımsızlık
- Türleri belirli API'ler, modüler hizmetler ve standart protokoller kullanılır; isteğe bağlı otomasyon, hizmetin çalışmasından ayrıdır.
- Yedekler ve dışa aktarımlar standart biçimdedir. Yönetim yazılımı sahibinin verisinin sahibi olamaz veya veriyi rehin tutamaz.
- Standart DNS çoğaltması karşı tarafta panel veya lisans gerektirmez. Ayrı yetkilendirilen uzaktan kayıt yönetimi isteğe bağlıdır.
- Kurulu panel güncellemelerini yalnız kullanıcı CelikPanel'in güncelleme ekranından başlatır. Yayınlama, tanı ve desteklenen geri alma asistana kurulum izni vermez.

### Dürüstlük ve sürüm kabul kuralı

Test, güvenlik incelemesi ve dokümantasyon gereklidir. Yaşam döngüsü desteği ayrıca
geçici ve gerçek sistem hizmetleri kullanan ortamlarda eksiksiz arıza geçiş kanıtı
ister: gerçek önceki sürüm durumu, başarısız güncelleme, fiilen otomatik geri yükleme,
kurtarmanın kesilmesi/yeniden başlatma, sahibin değişikliklerinin korunması,
yönetim kurtarması ve hizmet kontrolleri. Bileşen testleri, taklit servis yöneticisi
ve tek başına başarılı kurulum bu sözleşmeyi kanıtlamaz.

Her yaşam döngüsü değişikliği; etkilenen ilkeyi, şema/sürüm geçişini, kurtarma
davranışını ve kabul kanıtını adlandırır. Ölçülmemiş veya sonucu belirsiz işler
açık kalır. Dar kapsamlı bir olay düzeltmesi sınırları belirtilerek yayımlanabilir;
bu, temel P0 işlerini kapatmaz veya ilgisiz özellik genişlemesini haklı çıkarmaz.
Dayanıklı çalışma iddiasından önce D-025'in kabul matrisi uygulanıp geçmelidir.
Sistemin mümkün olan her arızayı kendiliğinden gidereceği vaat edilmez.


---

## Neredeyiz — 26 Eylül 2026

**Güncel öncelik: ürünü genişletmeden önce D-025 mimari dayanıklılık işini tamamlamak.**
Bu inceleme, yerel kaynağın `f253d318` commit'ine kadarki durumunu ve saklanan kabul
raporlarını kapsar. Frankfurt/Boston'un yeniden incelendiği, yeni sürüm yayımlandığı
veya değişikliklerin kurulu olduğu anlamına gelmez. P0.1–P0.5'in tamamı **kısmi**;
hiçbiri kapanmış değil. Anayasa gereksinimleri ve mevcut iş kimlikleri değişmedi.

### Kanıtlanan kapsam ve kalan işler

| Mevcut iş | Belirli kapsamda uygulanan veya doğrulanan | Kapatmak için gereken |
|---|---|---|
| P0.1 — Gerçek güncelleme/geri alma | Arch/Debian üzerinde gerçek eski sürüme dönüş; sonraki şema geçişi ve kurtarma deneylerinde gerçek Alpha64 schema38 verisi. [Kanıt](deploy/e2e/release-recovery/ISOLATED-DATABASE.md). | Desteklenen güncelleme/arıza/hizmet matrisi ve üretim imzalı aday kabulü tamamlanmalı; seçili aşamanın geçmesi bütün güncellemenin kabulü değildir. |
| P0.2 — Erişim ve doğru durum | Bağımsız, kimlik doğrulamalı kurtarma/durum girişi; seçili CLI, HTTP ve tarayıcı sonuçları eşleşiyor. Gerçek açılış beklemesinde tekrarlar aynı işlemi koruyor. [Kanıt](deploy/e2e/release-recovery/BOUND-WORKER.md). | Gerçek bekleme/hata/yeniden bağlanma durumları, bilinen hatanın korunması, beklerken tarayıcı erişimi ve üretim güven zinciriyle kullanıcının başlattığı güncelleme yolu. |
| P0.3 — Bağımsız kurtarma | Ayrı korunan kurtarma kodu/verisi, ayrı kopyada DB dönüşümü ve atomik yayın; geri alma sırasında seçili kesintiler ve yeniden açılış sonrasında kesinti anındaki satırlar korunarak otomatik kurtarma. [İki arızalı deney](deploy/e2e/release-recovery/NATIVE-EXCHANGE-RECOVERY.md). | Kalan kontrol noktaları, eksik yedek yakalama, dosya izin/sahiplik geçişleri, eski sürüm uyumu ve güvenli temizlik. Sonunda kurtulma, kesintisiz hizmet veya güç kaybı dayanıklılığı değildir. |
| P0.4 — Ortak DNS/TLS sözleşmeleri | DNS sahiplenme/yayın rolleri ayrıldı; ortak TLS ve DNS okuyucuları, bağımsız DNS gözlemi ve seçili Agent aracılı arıza kurtarması mevcut. [Sözleşme](docs/DNS-ENGINE-ARTIFACT.md). | Desteklenen **Agent'tan bağımsız DNS geri alma yürütmesi**, gerçek kesinti/sahip değişikliği kabulü, bütün üretici/geri yükleme geçişleri ve yetkili üst bölge bulunmayan ikincilde silme kanıtı. Henüz kullanıma açılmayan ters işlem kodu ve salt-okur gözlem bu açığı kapatmaz. |
| P0.5 — Hizmet bağımsızlığı | Sınırlı güvenlik duvarı/posta yenileme ve devreye alma kurtarması; raporda belirtilen yönetimsiz veya yönetim devre dışı açılışlarda tek PowerDNS ve BIND/BIND çiftinin hizmet vermesi. [Posta kanıtı](deploy/e2e/release-recovery/MAIL-ENROLLMENT-MANAGEMENT-ABSENT-BE.json); [DNS kanıtı](deploy/e2e/dns-kill-matrix/NATIVE-BIND-PAIR-TARGET-STAGED-20260926.md). | Farklı DNS motorlarının çiftleri, tam kurulum/devreye alma ve eski uygulama uyumu; desteklendiği söylenen her yönetimsiz/kaldırılmış durumda web/DB/posta/cron/yenileme/güvenlik duvarı kontrolleri. Yönetimi devre dışı bırakmak, tamamen kaldırmak değildir. |

Ayrıntılı [kabul kaydı](docs/RESILIENCE-CONTRACT.tr.md), başarısız denemeleri ve
kesin sınırları korur. Yeni kanıt mevcut P0 maddesini günceller;
yeni bir mimari plan başlatmaz.

### Sürüm durumu — 10 Ekim 2026

Bu alt bölüm sürümlerin nerede durduğunu kaydeder (10 Ekim 2026'da yazıldı;
v0.1.0-alpha.83'ün girdisi 11 Ekim 2026'da aday olarak eklendi, aynı gün
yayımlanması için değiştirildi ve 12 Ekim 2026'da kurulu sunucuların
okunması ve sahibin bildirimi için bir kez daha değiştirildi;
v0.1.0-alpha.84'ün girdisi 13 Ekim 2026'da aday olarak eklendi, aynı gün
yayımlanması için değiştirildi ve yine o gün kurulu sunucuların okunması ve
sahibin bildirimi için bir kez daha değiştirildi; v0.1.0-alpha.85'in
girdisi 15 Ekim 2026'da aday olarak eklendi ve aynı gün yayımlanması için
değiştirildi; v0.1.0-alpha.86'nın girdisi 15 Ekim 2026'da aday olarak
eklendi ve aynı gün set17 ve set17b gerçek sistem kayıtları ile
yayımlanması için değiştirildi; v0.1.0-alpha.87'nin girdisi 16 Ekim 2026'da
aday olarak eklendi ve aynı gün yayımlanması için değiştirildi;
v0.1.0-alpha.88'in girdisi 16 Ekim 2026'da aday olarak eklendi ve aynı gün
yayımlanması için değiştirildi; v0.1.0-alpha.89'un girdisi 16 Ekim 2026'da aday
olarak eklendi ve aynı gün yayımlanması ile sahibin iki güncellemeye ilişkin
bildirimi için değiştirildi; v0.1.0-alpha.90'ın girdisi 16 Ekim 2026'da aday
olarak eklendi ve aynı gün yayımlanması ile sahibin güncellemeye ilişkin
bildirimi için değiştirildi; v0.1.0-alpha.91'in girdisi 16 Ekim 2026'da aday
olarak eklendi ve aynı gün değiştirildi (etiketlendi, bilerek indirme portalında
yok); v0.1.0-alpha.92'nin girdisi 16 Ekim 2026'da aday olarak eklendi ve aynı
gün yayımlanması ile sahibin güncellemeye ilişkin bildirimi için değiştirildi); v0.1.0-alpha.93'ün girdisi 16 Ekim 2026'da aday olarak eklendi ve aynı gün
yayımlanması için değiştirildi; v0.1.0-alpha.94'ün girdisi 17 Ekim 2026'da aday olarak eklendi ve aynı gün
yayımlanması için değiştirildi; v0.1.0-alpha.95'in girdisi 17 Ekim 2026'da aday
olarak eklendi ve aynı gün yayımlanması ile sahibin alpha.94 ekranlarına ilişkin bildirimi için değiştirildi; v0.1.0-alpha.96'nın girdisi 17 Ekim 2026'da aday olarak eklendi ve aynı gün yayımlanması ile sahibin alpha.95 güncellemesine ilişkin bildirimi için değiştirildi; v0.1.0-alpha.97'nin girdisi 21 Ekim 2026'da aday olarak eklendi ve aynı gün yayımlanması ile sahibin gözlemleri için değiştirildi; v0.1.0-alpha.98'in girdisi 22 Ekim 2026'da aday olarak eklendi ve aynı gün yayımlanması ile yeni commit kimliği için, sonra sahibin güncellemeye ilişkin bildirimi için yeniden değiştirildi; v0.1.0-alpha.99'un girdisi 22 Ekim 2026'da aday olarak eklendi ve aynı gün yayımlanması için değiştirildi; v0.1.0-alpha.100'ün girdisi 22 Ekim 2026'da aday olarak eklendi ve aynı gün yayımlanması için değiştirildi; v0.1.0-alpha.101'in girdisi 23 Ekim 2026'da aday olarak eklendi ve 23 Ekim 2026'da yayımlanması için değiştirildi; v0.1.0-alpha.102'nin girdisi 23 Ekim 2026'da aday olarak eklendi ve 23 Ekim 2026'da yayımlanması için değiştirildi; v0.1.0-alpha.103'ün girdisi 23 Ekim 2026'da aday olarak eklendi ve 23 Ekim 2026'da yayımlanması için değiştirildi; v0.1.0-alpha.104'ün girdisi 23 Ekim 2026'da aday olarak eklendi ve 23 Ekim 2026'da yayımlanması için değiştirildi; v0.1.0-alpha.105'in girdisi 24 Ekim 2026'da aday olarak eklendi ve 24 Ekim 2026'da yayımlanması için değiştirildi; v0.1.0-alpha.106'nın girdisi 24 Ekim 2026'da aday olarak eklendi ve 24 Ekim 2026'da yayımlanması için değiştirildi; v0.1.0-alpha.107'nin girdisi 24 Ekim 2026'da aday olarak eklendi ve 24 Ekim 2026'da yayımlanması için değiştirildi; v0.1.0-alpha.108'in girdisi 25 Ekim 2026'da aday olarak eklendi; v0.1.0-alpha.109'un girdisi 26 Ekim 2026'da aday olarak eklendi). Hiçbir P0
durumunu
değiştirmez: P0.1–P0.5'in tamamı hâlâ **kısmi**; yukarıdaki ve aşağıdaki her açık
kabul işi açık kalır.

- **v0.1.0-alpha.81 yayımlandı.** `v0.1.0-alpha.81` etiketi `a0beb726`
  üzerindedir (4 Ekim 2026); bu, yayımlanmış `main` dalının ucudur. Sunucu
  sahibinin bildirdiği ve burada bir dosyada kayıtlı olmayanlar: yayımlama
  adımları (#204 numaralı çekme isteğinin birleştirilmesi, etiket, portal
  yayını), altı imzalı dosyanın doğrulanması ve sahibin kurulu iki paneli (biri
  Ubuntu 24.04, biri Debian 13) 8 Ekim 2026'da panelin kendi güncelleme
  ekranından güncellemesi. Depoda bu kurulu sunuculara ait bir kanıt dosyası
  yoktur; bu yüzden bu bir ölçüm değil, sahibin bildirimidir ve buradaki hiçbir
  iş için kabul kanıtı olarak kullanılmaz.
- **v0.1.0-alpha.82 10 Ekim 2026'da yayımlandı.** Sahibin 10 Ekim
  2026 (saat tarihi) üç kararı: numara v0.1.0-alpha.82'dir; son kod yayımlamadan
  önce yeni kurulur ve ölçülür (yapıldı, aşağıdaki altıncı koşu); Panel'in
  `Strict-Transport-Security` başlığı Panel'in kendi ana makine adıyla
  sınırlanır ve bu sürüme girer ([D-030](docs/DECISIONS.tr.md), `72b879eea`).
  Sahibin kendi denemesi, kurulu iki sunucunun sahip tarafından panelin
  güncelleme ekranından başlatılan güncellemesiydi; bunun için geçici sunucu
  kullanılmadı. Sürüm, `fix/setup-handover-guidance` dalıydı (#205 numaralı
  çekme isteği): aday girdisi yazılırken `a0beb726`'dan sonra 37 commit (uç
  `85aa05272`). `cmd/`, `internal/` ya da `web/` altında bir dosyayı
  değiştiren son commit `72b879eea`'dir; ondan sonraki commit'ler test düzeneği,
  kanıt, belge ve sürüm sırası hazırlığı ekler
  ([sürüm notu](docs/RELEASE-NOTES-v0.1.0-alpha.82.tr.md); aşağıdaki ayrıntılar
  onun "Yayımlama kaydı" bölümündedir).
  İçeriği: bilmediği olumsuz bir durumu söylemek yerine "denetleniyor" ya da
  "denetlenemedi" gösteren ekranlar; hangi durumdan kurulduğunu sürümüyle
  taşıyan ve durum eskimişse reddedilen ayar kayıtları (posta politikası, yedek
  zamanlaması, zamanlanmış görevler, yapılandırma dosyaları, catch-all);
  hizmetin sonradan gösterdiğine göre yanıtlanan yeniden yüklemeler ve hizmet
  işlemleri; sekiz rotada durum değiştiren her isteğin tek bir kimlik taşıması,
  böylece yinelenen isteğin yeniden çalıştırılmadan yanıtlanması
  ([D-029](docs/DECISIONS.tr.md)); cPanel içe aktarımı, sertifika istekleri ve
  Arch'ta PHP siteleri için düzeltmeler; `postconf`'tan okunan değerler için
  tek kural. Bu kural, yayımlanmış v0.1.0-alpha.81'de de bulunan bir posta
  sertifikası kusurunu düzeltir.
  - *Geçici konuklarda, 8 ve 9 Ekim 2026'da ölçülenler (Debian 13,
    Ubuntu 24.04, Arch; kayıt bir sayı vermedikçe hücre başına tek sıra):*
    gerçek cron, Postfix, PostgreSQL ve MariaDB üzerinde ayar kayıtları
    ([birinci koşu](deploy/e2e/release-recovery/evidence/set1-20261010/README.md));
    o koşunun yol açtığı düzeltmeler, hizmet işlemleri ve istek kimliği
    ([ikinci koşu](deploy/e2e/release-recovery/evidence/set2-20261011/README.md));
    ikinci tur düzeltmeler ve yayımlanmış v0.1.0-alpha.81'den sahibin başlattığı
    güncelleme; on hücrenin tamamı beklenen sonuca ulaştı
    ([üçüncü koşu](deploy/e2e/release-recovery/evidence/set3-20261012/README.md));
    son düzeltmeler, `557b554eb` hâliyle yeni kurulmuş konuklarda: Arch dahil
    üç platformda PHP sitesi oluşturuldu, çalıştı ve silindi; yayımlanmış
    v0.1.0-alpha.81'in oluşturduğu site Debian 13 ve Ubuntu 24.04'te güncelleme
    boyunca on isteği aynı biçimde yanıtladı; tek bir denetim geçmedi:
    Ubuntu 24.04'te Postfix durdurulduktan sonraki not
    ([dördüncü koşu](deploy/e2e/release-recovery/evidence/set4-20261009/README.md));
    o hatanın nedeni ve düzeltmesi, yeni kurulmuş Ubuntu 24.04 ve Debian 13
    konuklarında dörder durdurma
    ([ek ölçüm](deploy/e2e/release-recovery/evidence/set4b-20261009/README.md)).
    İlk üç dizinin adı, koşuların yapıldığı günlerden daha ileri tarihler
    taşır. Konuklar ağdan yalıtılmış değildir: dışarıya doğru NAT yapan QEMU
    kullanıcı ağında çalışırlar. Saptanan şudur: sürüm kaynağının her hücrede, Let's Encrypt adlarının bazı
    hücrelerde (hangilerinde ve hücrenin hangi anında olduğunu sürüm notları
    söyler) konuğun kendi loopback adresine sabitlenmesi, lisans adımının test
    derlemesince yanıtlanması ve ham kayıtlarda kurulu hiçbir sunucunun adının
    ya da adresinin bulunmaması. Konukların trafiği kaydedilmedi.
  - *Yalnız bir okuma olarak ölçülen:* `main.cf` uyarı verdirdiğinde gerçek
    `postconf`'un ne yazdırdığı ve eski kodun sakladığı metinle eski geri alma
    komutunun ne yaptığı
    ([okuma](deploy/e2e/release-recovery/evidence/set4c-20261009/README.md);
    Postfix 3.10.13, bir geliştirme konuğunda özel bir yapılandırma dizini).
    Buna dayanan düzeltmenin (`67b62cc0f`) yalnız bileşen testleri var: böyle
    bir dosyayla posta TLS değişikliği, geri alınması ve sertifika yayımı
    çalıştırılmadı.
  - *`67b62cc0f` hâlinde ölçülen, 9 Ekim 2026:* güncelleme matrisi bir kez
    daha, üçüncü koşunun aynı on hücresi, her biri bir kez
    ([beşinci koşu](deploy/e2e/release-recovery/evidence/set5-20261009/README.md)).
    O kod son koddan yalnız `72b879eea`'nin başlık kuralıyla ayrılır (bir
    satır kod, açıklaması ve testi).
    On hücrenin tamamı üçüncü koşunun `cfa329676` hâlinde ölçtüğü sonuca ulaştı;
    her adımın kararı, sonuç ve karşılaştırılan olgular aynıydı: Debian 13, Ubuntu 24.04
    ve Arch'ta doğrulanan güncelleme; üçünde de yayımlanmış v0.1.0-alpha.81'e
    otomatik dönüş; başarısız başlangıç denetiminden sonra dönüş (Debian 13);
    sahibin devam ettirmesi (Debian 13, Ubuntu 24.04); yeniden açılış boyunca
    yönetimin kapalı olması (Debian 13). v0.1.0-alpha.81'in oluşturduğu site
    güncelleme boyunca on isteği aynı biçimde yanıtladı (Debian 13,
    Ubuntu 24.04); güncellenmiş sunucuda platform başına bir Postfix durdurma
    notla yanıtlandı. Bu koşuda konukların diskleri bellekteydi; bu yüzden
    süreleri önceki koşularla karşılaştırılamaz. Ana makine koşunun büyük
    bölümünde "modern bekleme" kaydetti; örnekleyiciler duraklama göstermiyor.
  - *Son kodda (`72b879eea`) ölçülen, 9 Ekim 2026 21:46–23:21 UTC (yerel saatle
    10 Ekim 2026), altıncı gerçek sistem kaydı:* kodun Arch, Debian 13 ve
    Ubuntu 24.04'te yeni kurulumu, dördüncü koşunun ve ek ölçümünün
    denetimleriyle (PHP sitesi, üç listesiyle içe aktarım, mutlak adlı arşiv
    girdisi, reddedilen site, durmuş hizmetlerin yeniden yüklenmesi; Postfix
    durdurma notu Debian 13 ve Ubuntu 24.04'te, Arch'ta değil); Panel'in
    başlığının konukta çalışan Panel'den, her birinde on bir HTTPS yanıtı olan 14
    okumayla okunması: bu koddan okunan her yanıtta tam olarak
    `max-age=31536000`, her güncellemeden önce yayımlanmış v0.1.0-alpha.81'in her
    yanıtında `max-age=31536000; includeSubDomains`, Panel'in bağlantı noktasına
    düz HTTP için başlıksız `400`; ve yayımlanmış v0.1.0-alpha.81'den platform
    başına doğrulanmış bir güncelleme
    ([altıncı koşu](deploy/e2e/release-recovery/evidence/set6-20261009/README.md)).
    İlk Arch hücresinde bir düzenek kuralı düştü ve hücre yeniden koşulunca
    geçti; hiçbir ürün denetimi düşmedi. Ölçülmeyenler: v0.1.0-alpha.82 etiketli
    bir arşivin yeni kurulumu, tarayıcıda herhangi bir şey, yönetilen
    sertifikası olan bir Panel'de başlık, konukların trafiği.
  - *Yayımlama, 10 Ekim 2026 (aksi belirtilmedikçe UTC).* Sahip #205 numaralı
    çekme isteğini `2a0af8866` squash commit'i olarak `main`'e birleştirdi
    (07:15'te birleşti, depo barındırıcısından okundu), üzerinde
    `v0.1.0-alpha.82` açıklamalı etiketini oluşturdu (08:01) ve gönderdi;
    `CELIKPANEL_RELEASE_SEQUENCE` depo değişkenini sahip birleştirmeden önce
    82 yapmıştı (sahip bildirdi). Etiketin CI koşusu 22 işin 22'siyle
    başarıyla tamamlandı (08:01–08:21, depo barındırıcısından okundu) ve tam altı
    dosya yayımladı: genel arşiv ve sağlama toplamı, linux/amd64 arşivi ve
    sağlama toplamı, imzalı manifest v2 ve imzası. Platform arşivi SHA-256
    `a37671064f2ea0b08fd5e0e25c14218bb8b005067855bf9c13af2fc653ffe4bc`,
    65.894.979 bayt, sıra 82 (boyut ve özet depo barındırıcısından okundu); o
    barındırıcı sürümü 08:21:32'de yayımlanmış gösterir. Portalın
    `published_at` alanındaki 07:15:10 commit'in zamanıdır. Altı dosya etiketin
    özel bir klonunda doğrulandı (sürümü hazırlayan kişi bildirdi, burada bir
    dosyada kayıtlı değil): sağlama toplamları, genel arşivin platform
    arşiviyle bayt bayt aynılığı, `release.commit` ve `release.tree`'nin
    etikete eşitliği, önyükleme üyelerinin etiketinkilerle aynılığı, iki sürüm
    koruyucusunun geçmesi, manifest imzasının izlenen açık anahtarla
    doğrulanması, programlarda kabul-lisansı derleme etiketinin bulunmaması.
    İndirme portalı dört platform dosyasından imza öncesi kipte derlendi,
    yeniden üretilebilir biçimde paketlendi (paket SHA-256
    `aa19c55826e04bf7de470af44adb2bba864595e4648620bcf45793bf7fad215e`,
    132.163.894 bayt), alpha.81 portalını tutan bir deneme kökü üzerinde
    `promote-download-portal.py` ile yerelde prova edildi ve sahip tarafından
    yaklaşık 09:36'da `deploy/publish-download-portal.ps1` ile yayımlandı:
    işlem `status: committed`, 26 istekli tek bir genel doğrulama turu
    (`status: ok`), yedeğin tutulduğunu bildirdi (sahip bildirdi). Sonrasında
    genel sitenin bağımsız okunması (sürümü hazırlayan kişi, bir dosyada
    kayıtlı değil): `releases/latest.txt`, `latest.json` (sıra 82 ve commit),
    sunulan manifest, imza ve sağlama toplamı, `get.sh` ve açık anahtar
    etiketin dosyalarıyla uyuşuyor; alpha.81 arşivi hâlâ sunuluyor. **Kurulu
    paneller (sahip bildirdi):** sahip kurulu iki sunucuyu (biri Ubuntu 24.04,
    biri Debian 13) 10 Ekim 2026'da panelin kendi güncelleme ekranından
    güncelledi; ikisi de v0.1.0-alpha.82'yi ve `2a0af8866`'yı bildiriyor.
    Her Panel'in `Strict-Transport-Security` başlığı dışarıdan, önce ve sonra
    okundu: önce `max-age=31536000; includeSubDomains`, sonra
    `max-age=31536000`: D-030'un gerçek sunucularda gözlenmesi; tarayıcıda
    değil. Hiçbir asistan kurulu bir paneli güncellemedi ya da ona dokunmadı.
  - *Yayımlanmış kodda gerçek bir Chrome'da ölçülen, 10 Ekim 2026, yedinci
    gerçek sistem kaydı*
    ([yedinci koşu](deploy/e2e/release-recovery/evidence/set7-20261010/README.md);
    kabul denetiminden sonra commit edildi, kayıt denetimin daralttıklarını
    taşır): geçici Debian 13 konukları, Chrome 154 başsız, etiketlerin kaynağı
    kabul testi lisansıyla yeniden derlendi, yayımlanmış arşivler değil. (1)
    alpha.82 arayüzü, güncelleme karttan başlatıldı: **geçti** — Ayarlar sayfası,
    25 sn'lik kesintinin yaklaşık 9–11 sn'si boyunca bekletme katmanının
    altında bağlı ve etkisiz kaldı, kaldığı yerden devam etti. (2) alpha.81
    arayüzü: sayfanın yerini 89 sn'lik kesintinin yaklaşık 78 sn'si boyunca
    tam ekran "Lisans durumu denetlenemedi" sayfası aldı; sahibin anlattığı
    ekran (iki kesinti karşılaştırılamaz). (3) sekme 631 sn
    gizli, Panel çalışıyor: **geçti**, gizliyken sıfır istek, dönüşte tek
    sessiz okuma. (4) Panel yeniden başlarken gizli: **geçti**, yeniden
    başlatmanın 0,5 sn'den kısa sürmesi, yani yoklama aralıklarından kısa
    olması sınırıyla. (5) `/setup`, `/` ve `/settings?section=updates`
    yollarının soğuk tam sayfa yüklemesi, 18 yükleme: **üç yolda da
    başarısız**: henüz dönüştürülmemiş bir kapı olan `RecoveryAccess`'in tam
    sayfa "Panel erişimi denetleniyor" ekranı, hiçbir oturum okuması
    yanıtlanmadan boyandı; hızlı bağlantıda yaklaşık 80–100 ms, 2 Mbit/sn ve
    300 ms'ye kısılmış bağlantıda yaklaşık 1,7 sn. "Sessiz süre (1,5 sn)
    geçene ya da bir okuma erişimi doğrulamadan yanıtlayana kadar hiçbir şey
    çizilmez" kuralını bu kapı karşılamıyor. Ölçülmeyenler: Ubuntu, başlıklı
    tarayıcı, telefon görünümü, Türkçe, HTTP önbelleği açık yükleme, kurulu
    herhangi bir sunucu. Hiçbir P0 işi kapanmaz ya da ilerlemez.
  - *Hangi denetim hangi koda dayanıyor:* ayar kayıtları `c4cf7fd9d`;
    düzeltilmiş ayar kayıtları, hizmet işlemleri ve istek kimliği `faa5ef085`;
    ikinci düzeltmeler ve ilk güncelleme matrisi `cfa329676`; yeni kurulmuş
    sunucu denetimleri `557b554eb`; platform başına dört Postfix durdurma
    `1f182a483` ile aynı dosyalar; on hücreli güncelleme matrisi `67b62cc0f`;
    yeni kurulum, başlık ve platform başına bir güncelleme `72b879eea`. Son
    kodda yinelenmeyenler: ilk üç koşunun ayar kaydı, istek kimliği ve tam
    hizmet işlemi hücreleri ile on güncelleme hücresinin yedisi. Hiçbir gerçek
    sistemde çalıştırılmayan: düzeltilmiş posta sertifikası yolu (anlık görüntü,
    geri yükleme, geri okuma).
  - *Sonraki sürüme açık iş ve bitiş ölçütü:* Panel'in kendi başlangıcı,
    barındırılan sitelerin web sunucusu yapılandırmasını yeniden yazar; bu
    adayda da önceki sürümlerde de. Sahibin böyle bir dosyadaki el
    düzenlemesinin o başlangıçta neyle karşılaştığı ölçülmedi ve anayasanın 1.
    bölümündeki kurala (sahip değişikliklerini algıla, asla sessizce ezme)
    göre denetlenmedi. Bitiş: sahibin düzenlediği bir site yapılandırma
    dosyasının bir Panel başlangıcı ve bir güncelleme boyunca gerçek sistemde
    okunması; ürün düzenlemeyi ya algılayıp bildirir ya da ezme, açıkça
    yazılmış ve belgelenmiş bir sınırdır.
  - *Yalnız bileşen testleri olanlar:* `e2be8af30` (MariaDB olmayan bir
    `mysqld`, MariaDB dosyasının denetimi olarak kabul edilmez), durdurmadan
    sonra durumu oturmayan ya da okunamayan birim için iki yeni not ve posta
    sertifikası yenilemesinin sunulan sertifikayı denetlemesi.
  - *Açık ve sürüm notlarında adıyla yazılı:* arayüzün 31 kaynak dosyası
    sunucuyu hâlâ eski biçimde okuyor (erişim kapıları, kurulum sihirbazı,
    güncelleme ve işlem katmanları, Hizmetler sayfaları ve başkaları); sekiz
    rotanın dışındaki durum değiştiren rotalarda istek kimliği yok; bağımsız
    posta yenileme yardımcısı zaten kurulu olan sunucu kurulu yardımcısını
    korur; ilk altı gerçek sistem koşusunda gerçek bir Panel'e karşı hiçbir
    ekran çizilmedi (yedincisi yalnız güncelleme ve ilk sayfa yüklemesi için
    çizdi); yayımlanmış sürümden güncelleme imzalı arşivden değil, etiketin
    kaynağının deneme lisansıyla derlenmesinden ölçüldü; 58 karakterlik bir
    site adını olağan nginx reddeder; sertifika doğrulama dizini, reddedilen
    bir oluşturmadan ve bir silmeden sonra yerinde kalır.
  - *Sonraki sürüme açık işler (10 Ekim 2026, yayımlamadan sonra).* Hiçbiri
    v0.1.0-alpha.82'de düzeltilmedi ve hiçbiri bir P0 durumunu değiştirmez:
    1. Ubuntu sunucusunda sahibin gözlemi: v0.1.0-alpha.81 arayüzünden
       başlatılan güncelleme, Panel'in yeniden başlaması sırasında tam ekran
       "License status could not be checked" sayfasını gösterdi ve kendiliğinden
       toparlandı; aynı biçimde başlatılan Debian sunucusunda görünmedi (sahip
       bildirdi; laboratuvar alpha.81 ekranını yeniden üretti, yedinci kayıt).
    2. Debian sunucusunun güncellemeler sayfasında sahibin gözlemi: kart
       v0.1.0-alpha.82'yi kurulu gösterirken köşedeki bir bildirim, T+02:03'te
       hâlâ güncellemenin uygulandığını ve güncellemelerin kilitli olduğunu
       söylüyordu. İkisi birbirini tutmuyordu (sahip bildirdi).
    3. Sahibin gözlemi: `/setup` sayfasının soğuk tam yüklemesi bir an tam sayfa
       "Checking panel access" ekranı gösterir (sahip bildirdi). Yedinci kayıt,
       5. hücre, aynısını `/setup`, `/` ve güncellemeler sayfasında ölçtü:
       `RecoveryAccess` kapısı hiçbir oturum okuması yanıtlanmadan boyuyor.
       Bilinen-durum kuralına henüz dönüştürülmemiş kapılar listesindedir.
    4. Sahibin gözlemi: kurulum sihirbazı sayfası en üstte "This operation
       stopped ... A required check needs attention" derken son adım "Verify
       the prepared server - In progress" diyor, neden de kapalı "Checks and
       how to continue" ve "Technical details" bölümlerinin altında; D-024
       nedeni ve kimin işlem yapacağını listeden önce ister. alpha.81'de de
       aynıydı (sahip bildirdi).
    5. Sahibin üretilmiş bir site yapılandırma dosyasındaki düzenlemesinin bir
       Panel başlangıcında neyle karşılaştığının denetimi (yukarıdaki açık iş,
       değişmedi).
    6. alpha.82 sınırlarından ertelenen kalanlar: dönüştürülmemiş 31 arayüz
       kaynak dosyası; istek kimliği olmayan durum değiştiren rotalar; kurulu
       yenileme yardımcılarının taşınması; `postconf` uyarısının ardındaki posta
       sertifikası yolu, bir sunucuda hiç ölçülmedi; arşivin hâlâ taşıdığı
       satıcı yayın araçları ve iki tek seferlik kurtarma betiği (sahip kararı,
       11 Ekim 2026: v0.1.0-alpha.83'te kalırlar; çıkarılmaları sonraki bir
       sürümün paketleme değişikliğidir); bir DNSBL kaydının tek satıra çevirdiği Postfix kısıtlama
       listesi; ve bir veritabanı kullanıcısının parolası için denetimin
       bulunmaması.
  - Bu sürüm hiçbir P0 işini kapatmaz ve ilerletmez. Yayımlanması kurulu hiçbir
    paneli güncellemedi; iki kurulu paneli sahip, panelin kendi güncelleme
    ekranından güncelledi.
- **v0.1.0-alpha.83 yayımlandı, 11 Ekim 2026; sahip kurulu iki sunucuyu da 11
  Ekim 2026'da ona güncelledi (12 Ekim 2026'da dışarıdan gözlendi); sahip 12
  Ekim 2026'da iki ekran görüntüsü bildirdi, aşağıda kayıtlı, açık
  kısımlarıyla.** Sahibin 11 Ekim
  2026 (saat tarihi) iki kararı: numara v0.1.0-alpha.83'tür; ve sahibin
  koruduğu site yapılandırma dosyaları ([D-031](docs/DECISIONS.tr.md)) bu sürüme
  girer. Onlarla birlikte kaydedilen: v0.1.0-alpha.82'nin daraltılmış
  `Strict-Transport-Security` başlığı ([D-030](docs/DECISIONS.tr.md)) kurulu iki
  sunucuda güncellemeden önce ve sonra dışarıdan okundu (sahip bildirdi, yukarıdaki
  yayımlama girdisinde); bu sürüm onu değiştirmez. Sürüm `fix/alpha83-known-state-gates`
  dalıydı, `main`'e #206 numaralı çekme isteği: aday girdisi yazılırken `2a0af8866`'dan
  sonra 44 commit. `cmd/`, `internal/` ya da `web/src` altında test olmayan bir
  dosyayı değiştiren son commit `40f02ecb6`'dır; ondan sonraki commit'ler test,
  kanıt, belge ve sürüm hazırlığı ekler (sürüm sırası 83, kurucunun önyükleme
  sabitleri ve README sürüm satırları, `cmd/`, `internal/` ya da `web/` altında
  hiçbir dosyayı değiştirmeyen `ab67d9fc4` içinde); bu türden yeni commit'ler
  sayıyı artırır, bunu değiştirmez
  ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.83.tr.md); aşağıdaki
  ayrıntılar "Yayımlama kaydı"nda). İçeriği:
  sahibin değiştirdiği bir sitenin nginx dosyası, PHP-FPM havuzu ve uygulama
  birimi korunur ve adlandırılır, alan adının sayfasında üç seçenekle (veritabanı
  şeması 43'ten 44'e, 044 numaralı geçiş); korunan bir dosya için sertifika
  istenmeden önce bir denetimle, CelikPanel'in kendi dizininden yayımlanan
  sertifika doğrulaması; reddedilen bir değişiklik önceki dosyayı geri koyar ve
  hizmeti yalnız hizmetin kendi denetimi kabul ederse yeniden yükler; ilk sayfa
  yüklemesi söyleyecek bir şey olana kadar hiçbir şey çizmez, sayfa üzerindeki
  katman okuduğu nedeni söyler, güncelleme bildirimi ve kart birbirini tutar ve
  duran bir kurulum nedenini adımların üstünde söyler.
  - *Geçici konuklarda ölçülen, 10 ve 11 Ekim 2026 (UTC), yedinciden on üçüncüye
    gerçek sistem kayıtları* (test lisanslı derlemeler; v0.1.0-alpha.81 ve
    alpha.82'den güncellemeler yayımlanmış arşivlerden değil, etiketlerinin
    kaynağından yeniden derlenmiş sürümlerden; dışarıya NAT yapan QEMU kullanıcı
    ağı, trafik kaydedilmedi; loopback'e sabitlenmiş adlar; tek ana makine):
    [yedinci](deploy/e2e/release-recovery/evidence/set7-20261010/README.md)
    (alpha.81 ve alpha.82 arayüzleri gerçek bir Chrome'da, Debian 13),
    [sekizinci](deploy/e2e/release-recovery/evidence/set8-20261010/README.md)
    (alpha.82'nin sahip düzenlemesiyle yaptığı, üç platform),
    [dokuzuncu](deploy/e2e/release-recovery/evidence/set9-20261010/README.md)
    (gerçek bir Chrome'da ilk yükleme, erişim beklemesi ve bir güncelleme,
    Debian 13),
    [onuncu](deploy/e2e/release-recovery/evidence/set10-20261010/README.md)
    (D-031'in ilk bölümü, üç platform),
    [on birinci](deploy/e2e/release-recovery/evidence/set11-20261011/README.md)
    (düzeltmeleri: Debian 13 ve Ubuntu 24.04; include dizinleri, kilitli dosya
    ve bir doğrulama durumu için Arch),
    [on ikinci](deploy/e2e/release-recovery/evidence/set12-20261011/README.md)
    (havuz, birim ve alpha.82'den güncelleme, üç platform) ve
    [on üçüncü](deploy/e2e/release-recovery/evidence/set13-20261011/README.md)
    (ondan sonraki düzeltmeler, yalnız Debian 13 ve Ubuntu 24.04; Arch
    koşulmadı). Her kaydın kabul denetimindeki daraltmaları kendi metnini daraltır
    ve bu özete üstün gelir. Ölçülen kod, son koddan sürüm notunda adı geçen
    değişikliklerle ayrılır; on üçüncü kayıttan sonraki değişikliklerin (işin
    nasıl süreceğini ve "çalışmıyor" durumunu söyleyen cümleler, verify zaman
    aşımı, bulduğunu kaydeden sayfa okuması, başarılı bir seçimin `service`
    alanı) yalnız bileşen testleri var.
  - *Çekme isteğinde sürekli tümleştirme:* 11 Ekim 2026'da 10:20, 11:40, 11:44
    ve 11:49 UTC'deki uçların koşularının her biri 23 denetimin 23'ünü geçti
    (dosya yayımlama işi yalnız bir sürüm etiketinde çalışır); 11:44 UTC'deki uç
    ürün kodundaki son değişikliği taşır. Onlardan önceki altı koşu bir testin
    sahte Agent'ındaki veri yarışı yüzünden başarısız olmuştu ve test kodunda
    düzeltildi. 12:09 UTC'den sonra yapılan commit'lerin koşuları buraya
    girilmedi.
  - *Yayımlama, 11 Ekim 2026 (UTC).* Sahip #206 numaralı çekme isteğini
    `main`'e `0447558e956d4c02c7bf06b368def0924856f8fc` olarak squash ile
    birleştirdi (ağaç `7adde05327ecadb3c7d3451ec9b08fab9494a480`; çekme
    isteğinin son commit'i `304da2466`'nın ağacına eşit); o commit'teki CI
    (koşu 38145660891) çalışan 23 işin 23'ünü geçti, dosya yayımlayan iş yalnız
    bir etiket için çalıştığından atlandı. `main` dalının koruması yalnız
    "Release — reproducible archive and checksum" istiyordu (2026-10-11'de
    okundu); bu yüzden bölünen yarış dilimleri için ad değişikliği gerekmedi.
    Sahip etiketlemeden önce `CELIKPANEL_RELEASE_SEQUENCE` depo değişkenini 83
    yaptı, sonra o commit'te `v0.1.0-alpha.83` açıklamalı etiketini (etiket
    nesnesi `f5772a5b96fe8a341c63fd71a27a750bade4e382`) oluşturup gönderdi.
    Etiketin CI koşusu (38153704563) 24 işle başarıyla tamamlandı ve sürümü
    16:22:31'de ön sürüm olarak altı dosyayla yayımladı. Platform arşivi
    `celikpanel-v0.1.0-alpha.83-linux-amd64.tar.gz`, 66.132.418 bayt, SHA-256
    `6275bebed2b581bad58d4323217ed14defb54d06c23cef5af94218da573aefe5`, sıra 83.
    Dosyalar, etiketin bir klonundan yerel bir derleme konuğunda denetlendi
    (sürümü hazırlayan kişi bildirdi, burada bir dosyada kayıtlı değil): iki
    sağlama toplamı dosyası tutuyor; iki arşiv bayt bayt aynı; imzalı manifest
    sıra 83'ü, v0.1.0-alpha.83 sürümünü, yukarıdaki commit'i, bu özeti ve
    boyutu söyler, `published_at` 2026-10-11T14:03:46Z; Ed25519 imzası
    etiketteki `deploy/release-signing-ed25519.pem` ile doğrulanır (arşivdeki
    kopya ve arşivdeki `libexec/get.sh`'nin sabitlediği dosyayla aynı);
    arşivdeki `release.commit` ve `release.tree` etikete eşit; arşivdeki
    `install.sh`, `libexec/get.sh` ve açık anahtar etiketinkilerle bayt bayt
    aynı; iki sürüm koruyucusu 0 koduyla çıkıyor; önyükleme sabitleri sıra 83 ve
    v0.1.0-alpha.83. İndirme portalı paketi etiketten, altı dosyayla imza
    öncesi kipte, iki kez aynı sonuçla derlendi (132.634.305 bayt, SHA-256
    `6787d0b42ff70621fe1a2ee46c75d336ca24092108a2182e6c35a2da0f5c94d2`); sözleşme
    testi ile genel doğrulayıcı (17 test) ve terfi (14 test) testleri geçti ve
    gerçek terfi aracının alpha.82 sitesinin deneme kopyası üzerindeki yerel
    provası 26 istekle `committed` bitti. Sahip portalı
    `deploy/publish-download-portal.ps1` ile yayımladı (önceki v0.1.0-alpha.82,
    hedef v0.1.0-alpha.83): durum `committed`, yedek tutuldu, 26 istekli tek
    bir genel doğrulama turu (`status: ok`), başarı işaretçisi bir kez yazıldı
    (sahip bildirdi). Sonrasında genel sitenin dışarıdan salt-okunur okunması
    (sürümü hazırlayan kişi, bir dosyada kayıtlı değil): `releases/latest.txt`
    `v0.1.0-alpha.83`'tür, `latest.json` commit'i ve özeti söyler, sunulan
    manifest, imza ve sağlama toplamı doğrulanmış dosyalarla bayt bayt aynıdır,
    arşiv adresi `Content-Length: 66132418` ile 200 yanıtlar (yeniden
    indirilmedi), `get.sh` sıra 83'ü ve v0.1.0-alpha.83'ü sabitler ve alpha.82
    manifesti hâlâ sunuluyor. **Kurulu paneller: bu yayımlama kaydı 11 Ekim
    2026'da yazıldığında güncellenmemişti.** Bunu her sahip panelin kendi
    güncelleme ekranından başlatır. **12 Ekim 2026'da 18:36 UTC'de dışarıdan
    yapılan bir okuma** (salt-okunur, HTTPS üzerinden, oturum açmadan; sahibinden
    başka kimse sunuculara dokunmadı, sürümü hazırlayan kişi güncellemeyi
    başlatmadı, izlemedi ve doğrulamadı) kurulu iki sunucunun da v0.1.0-alpha.83'ün
    arayüz derlemesini sunduğunu gösterdi: başlangıç sayfaları doğrulanmış
    arşivdeki `web/dist/index.html` ile aynı derlenmiş arayüz dosyalarına
    başvuruyor; bu dosyalardan biri, her sunucudan okunduğunda, arşivdeki
    kopyayla bayt bayt aynı (aynı SHA-256); `Last-Modified` tarihleri 11 Ekim
    2026 19:27:52 UTC (ilk sunucu) ve 19:28:17 UTC (ikinci sunucu); ikisi de
    hâlâ `Strict-Transport-Security: max-age=31536000` gönderiyor. Bu,
    her sunucudaki Panel sürecinin bu sürümün arayüzünü sunduğunu, yani sahibin
    ikisini de panelin kendi güncelleme ekranından 11 Ekim 2026'da, yaklaşık
    19:27–19:28 UTC'de güncellediğini gösterir. Güncellemenin panel içindeki
    doğrulama sonucunu, Agent'ın sürümünü, ilk başlangıcın site yapılandırma
    dosyalarıyla ne yaptığını (kaçı devralındı, kaçı olduğu gibi bırakıldı) ya
    da sahibin ekranda gördüğü herhangi bir şeyi göstermez. Bu okuma kabul
    kanıtı değildir. **Sahibin 12 Ekim 2026'da bildirdikleri** (iki ekran
    görüntüsü, İngilizce arayüz, sunucu başına bir tane; sahibin gözlemi, kabul
    kanıtı değil, laboratuvarda hiçbir şey ölçülmedi): iki sunucuda da kenar
    çubuğu `CelikPanel · v0.1.0-alpha.83 · build 0447558e956d` yazıyor (sürüm
    commit'inin 12 karakterlik öneki) ve derleme kimliği sözcüğün ortasından
    satır kırıyor. İlk sunucu (barındırma amacı "Custom setup", alan adı yok)
    Pano'ya düşüyor; "System services" kartı "Unknown" ("Component status is
    outdated. Open Components to rescan.") ve "Needs attention" kutusu kurulu
    bileşenlerin yakın zamanda denetlenmediğini söylüyor; onun için ne hata ne
    lisans sayfası bildirildi. İkinci sunucu, kurulum planı güncellemeden önce
    yeniden açıldığı ve son denetimi hiç bitmediği için kurulum sayfasına, 5.
    adıma düşüyor: adımların üstünde sayfa "Setup stopped: its plan was
    reopened" diyor, bunun hata olmadığını, kendiliğinden hiçbir şeyin
    sürmeyeceğini, hangi denetimin beklediğini ve sonraki eylemi (planı gözden
    geçirip yeniden başlatmak) söylüyor; son adım "Verify the prepared server"
    "Stopped" diyor, diğer her adım "Completed"; hiçbiri "In progress"
    demiyor. Bu, alpha.82'deki kurulum sayfası gözleminin (durdu ve sürüyor bir
    arada) kurulu bir sunucuda çözülmüş görülmesidir. Sürümü hazırlayan kişinin
    12 Ekim 2026'da dışarıdan okuduğu: o sunucunun posta ana makine adı
    sunucunun adresine çözülüyor, ama adresin ters (PTR) kaydı başka bir ana
    makine adını söylüyor; bu yüzden bekleyen son denetim, sahip PTR kaydını
    barındırma sağlayıcısında, CelikPanel dışında değiştirene kadar
    geçmeyecek. Bildirilmeyenler: yeniden başlatma sırasında tam ekran lisans
    sayfasının görünüp görünmediği; güncelleme bildirimi ile kartın uyuşup
    uyuşmadığı; kurulum adresine doğrudan yüklemede anlık lisans sayfasının
    hâlâ görünüp görünmediği; güncellemenin panel içindeki doğrulama sonucu;
    Agent'ın sürümü; Alan Adları listesi (iki sunucunun da alan adı yok;
    "olduğu gibi bırakıldı" satırı onlarda görünemez ve ilk başlangıcın site
    dosyalarını devralması orada devralacak bir şey bulmadı); `postconf -n
    >/dev/null` çıktısı. Ayrıntılar sürüm notlarının "Yayımlama kaydı"
    bölümünün 12. ve 13. maddelerinde.
  - *Sonraki sürüme açık, sürümün sınırları sürüm notunun listelediği gibi:*
    kenar çubuğundaki derleme kimliğinin sözcüğün ortasından satır kırması
    (sahibin ekran görüntüleri, 12 Ekim 2026; başka bir numara gibi
    okunabilir); hiç ölçülmemiş bir PHP sürümü değişikliği; on üçüncü kayıtta olmayan Arch; bir
    havuz ya da birimde "Benimkini koru", "Yeniden oluştur" ve "CelikPanel'in
    metnini şimdi yaz" ile nginx ve bir uygulama için "çalışmıyor" durumu, hiçbir
    laboratuvarda koşulmadı; bir sitenin silinmesinde CelikPanel'in kendi include
    dizininin kaldırılmasına bakılmadı; hiçbir laboratuvarda sertifika
    alınmadı, çalışmayan bir denetimin başarı yanıtındaki alanlar hiç görülmedi;
    ilk başlangıcın yazımları kurulu bir sunucunun dosyalarında ölçülmedi; bir
    dönüşten sonra ikinci güncelleme yapılmadı; bilinen-durum kuralına
    dönüştürülmemiş 30 arayüz kaynak dosyası (88 yer); istek kimliği olan on iki
    rota, geri kalanı yok; güncellemenin değiştirmediği kurulu posta yenileme
    yardımcıları; bir sunucuda hiç ölçülmemiş, `postconf` uyarısının ardındaki
    posta sertifikası yolu; bir DNSBL kaydının tek satıra çevirdiği Postfix
    kısıtlama listesi; bir veritabanı kullanıcısının parolası için denetimin
    bulunmaması. "Sahibin üretilmiş bir site yapılandırma dosyasındaki elle
    düzenlemesinin bir Panel başlangıcında neyle karşılaştığı" açık işi bu
    sürümle yanıtlanır ve açık kalır: kurulu iki sunucu da bu sürümün arayüzünü
    sunuyor, ama alan adları yok; bu yüzden ilk başlangıcın onlarda devralacağı
    site dosyası yoktu ve soru kurulu bir sunucuda yanıtlanmış değil; sürüm
    siteleri olan kurulu bir sunucuda görülene kadar açık kalır.
  - Bu sürüm ve yayımlanması hiçbir P0 işini kapatmaz ve ilerletmez.
    Yayımlanması kurulu hiçbir paneli güncellemedi; güncellemeyi yalnız sahip,
    panelin kendi güncelleme ekranından başlatır.
- **v0.1.0-alpha.84 yayımlandı, 13 Ekim 2026; sahip kurulu iki sunucuyu da 13
  Ekim 2026'da ona güncelledi; dışarıdan ve sahibin ekran görüntülerinde
  görüldü, gözlemlerin hepsi bildirilmedi.** 13 Ekim 2026'da aday olarak
  yazıldı, aynı gün yayımlanması için değiştirildi ve yine o gün kurulu
  sunucular için bir kez daha değiştirildi. Sahip
  13 Ekim 2026'da numaranın v0.1.0-alpha.84 olduğuna karar verdi. Sahibin 12 Ekim 2026 kararı,
  v0.1.0-alpha.83 güncellemesinden sonra: Dashboard bileşenleri kaynak
  harcamadan kendiliğinden kontrol eder ([D-032](docs/DECISIONS.tr.md)). Aday
  `feat/alpha84-dashboard-self-check` dalıydı, `main`'in `0447558e9`'undan
  (v0.1.0-alpha.83 adayının birleştirilmesi), `main`'e #208 numaralı çekme
  isteği (yayımlandığı güne kadar taslak). Commit'leri:
  `a5830dd3e` (Dashboard'ın "Sistem servisleri" kartı
  kendiliğinden kontrol eder: en çok 300 sn'de bir, yalnız sekme görünürken,
  aralıklı zamanlayıcı yok, yeniden deneme döngüsü yok, dört ayrı durum, süren
  bir değişiklik için tutulma durumu), `f6eaa0b57` (on dördüncü gerçek sistem
  kaydı, yalnız kanıt), `e6de64a5b` (sayfa bir taramayı onu aldığı andan
  tarihler: o kaydın D1 kusuru), `1a74eeda5` (sunucu bir taramanın yaşını
  bildirir, `scan_age_seconds`; ulaşılamayan bir Agent tarama işleyicisinde ve
  on bir değiştirici işleyicinin kabul denetiminde `503 AGENT_UNREACHABLE`
  yanıtlar), `fb5ec06f8` (takip eden gerçek sistem kaydı set14b, 13 Ekim 2026,
  yalnız kanıt: ürün kodu yok), `7024534bc` (13 Ekim 2026: lisans denetimi o
  kaydın D2 kusurundan sonra kalan süreyi bildirir, `valid_for_seconds`; tipli
  Agent yanıtı ortak hata yazıcısına taşınır ve güvenlik duvarı okumasına ile
  sistem denetimine ulaşır, O3 gözlemi; Bileşenler sayfası, bir işlemin
  tamamlanmış olabileceğini söylemek yerine başarısız bir okumada durumu
  okuyamadığını söyler, O2 gözlemi), `68b79b372` (13 Ekim 2026: okuma şeridinin
  metni okunur bir genişlikte kalır; yalnız sınıflar), `4ed7b1278` (yalnız
  belgeler), `c76861cde` (13 Ekim 2026: on altıncı gerçek sistem kaydı,
  set14c, yalnız kanıt: ürün kodu yok), `6e264615f` (yalnız belgeler),
  `1636b73f0` (13 Ekim 2026: sürüm hazırlığı: 84 sürüm sırası politikası,
  önyükleme sabitleri, sürüm satırları ve üç sözleşme testi; `cmd/`, `internal/`
  ya da `web/` altında hiçbir dosya), `aa7dac2b2` (sürüm notu taslağı; yalnız
  belgeler) ve `e6ed563ad` (13 Ekim 2026: on yedinci gerçek sistem kaydı, set15,
  yalnız kanıt: ürün kodu yok). Şema değişikliği yok, yeni rota yok
  ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.84.tr.md)).
  - *Ölçülen: on dördüncü gerçek sistem kaydı*
    ([set14](deploy/e2e/release-recovery/evidence/set14-20261012/README.md),
    12 Ekim 2026 UTC; İngilizce masaüstü pencereli Debian 13 ve Türkçe telefon
    pencereli Ubuntu 24.04, gerçek, başsız bir Chrome; yalnız `a5830dd3e`'nin
    test lisanslı derlemesi). Bayat yüklemede tam bir koşullu tarama, tazede
    hiç, görünür tutulan sayfada 300 sn'den önce olmamak üzere bir tane daha,
    gizliyken hiç ve gösterilince bir tane, "bir değişiklik sürüyor" diye
    gösterilen ve tek takibi olan `409 service_operation_busy` ve Agent
    durmuşken "Kontrol edilemedi" ve düğmeyle gösterilen bir `500`; bir kontrol
    için Agent'ın CPU'su yaklaşık 0,2 ile 0,4 sn. D1 kusuru (konuğun saati
    ileriyken kart her kontrolden sonra 20 ile 29 sn "Son 5 dakikada kontrol
    edilmedi" dedi) orada bulundu ve `e6de64a5b` ile `1a74eeda5`'te düzeltildi.
    Sınırları: Arch koşulmadı; `server_setup_busy` ve hiç kontrol edilmemiş
    durum ölçülmedi; taramanın sunucu tarafı duvar süresi ölçülmedi; tutulma
    durumuna yalnız önce taramayan bir değişiklikle ulaşılır. Kaydın her kabul
    denetimi düzeltmesi kendi metnini daraltır ve bu özete üstün gelir.
  - *Ölçülen: on beşinci gerçek sistem kaydı, set14b*
    ([set14b](deploy/e2e/release-recovery/evidence/set14b-20261013/README.md),
    13 Ekim 2026 UTC; tek geçici Debian 13 konuğu, gerçek, başsız bir Chrome;
    `1a74eeda5`'in test lisanslı derlemesi). Konuğun saati 1,5 sn ileriyken
    bayat bir yükleme tek koşullu tarama gönderdi ve kart "Son 5 dakikada
    kontrol edilmedi" karesi olmadan doğrudan sonuca geçti (D1 gitti);
    `scan_age_seconds` tarama taşıyan her yanıtta bir sayıydı. Agent durmuşken
    Dashboard (İngilizce ve Türkçe), Bileşenler sayfası ve bir servis yeniden
    başlatma isteği, sahibin sonraki adımıyla ve soket yolu olmadan `503
    AGENT_UNREACHABLE` yanıtladı, nginx değişmedi; Agent başlatıldıktan sonra
    düğmeye bir basış bir tarama gönderdi. Bileşenler sayfası tek koşullu tarama
    gönderdi. **D2 kusuru, laboratuvarda bulundu; düzeltmesinden sonra on
    altıncı kayıtta ölçüldü:** konuğun saati 418 sn geriyken hiçbir Panel
    sayfası açılmadı (lisans denetimi 90 sn'de 19 kez okundu); bu yüzden
    taramanın yaşının "geride kalan saat" ölçümü yapılamadı. Ölçülmeyen: Ubuntu,
    Arch, pencere kuralı (on dördüncü kayıttan beri değişmedi).
  - *Ölçülen: on altıncı gerçek sistem kaydı, set14c*
      ([set14c](deploy/e2e/release-recovery/evidence/set14c-20261013/README.md),
      13 Ekim 2026 UTC; tek geçici Debian 13 konuğu, gerçek, başsız bir Chrome;
      `68b79b372`'nin test lisanslı derlemesi). Konuğun saati tarayıcınınkinden
      önce yedi dakika geride, sonra yedi dakika ileriyken Dashboard 130 ile 192
      ms'de açıldı, lisans erişimini 90 sn'de saat doğruyken olduğu gibi dört
      kez okudu, tam bir koşullu tarama gönderdi ve sonucu gösterdi; her erişim
      yanıtı `valid_for_seconds` taşıdı (42,5 ile 60,0 sn) ve sayfa ondan
      türettiği son tarihten 15 sn önce yeniden okudu (D2 gitti). Agent
      durmuşken güvenlik duvarı okuması, sistem denetimi ve bir yeniden başlatma
      isteği yalnız `code` ve `error` ile ve soket yolu olmadan `503
      AGENT_UNREACHABLE` yanıtladı, nginx değişmedi ve Dashboard kartı "Kontrol
      edilemedi" dedi (O3). Bileşenler sayfası Agent cümlesi ve Yeniden tara ile
      "Component state could not be read" / "Bileşen durumu okunamadı" gösterdi,
      "tamamlanmış olabilir" sözünü hiç; Agent başlatıldıktan sonra bir basış
      tek koşulsuz tarama gönderdi ve şerit gitti (O2). Değişen kodda kusur yok.
      Ölçülmeyen: Ubuntu, Arch, bir işlemden sonra "tamamlanmış olabilir"
      şeridi, konuğun NTP ayarları. Sonraki sürüm için gözlemler: O4, O5, O6 ve
      çevrilmemiş posta yığını metni (aşağıda).
  - *Ölçülen: on yedinci gerçek sistem kaydı, set15, yayımlama öncesi ölçüm*
    ([set15](deploy/e2e/release-recovery/evidence/set15-20261013/README.md),
    13 Ekim 2026 UTC; geçici konuklar, gerçek, başsız bir Chrome; `1636b73f0`'ın
    kendisinin ve alpha.83 etiketinin kaynağından yeniden yapılmış bir
    derlemenin test lisanslı derlemeleri). Debian 13 (İngilizce) ve Ubuntu 24.04
    (Türkçe) temiz kurulumları: sürüm v0.1.0-alpha.84, şema 44, arşivde ve kurulu
    kopyada önyükleme sabitleri 84, alan adı ve ilk durum satırı yok,
    `valid_for_seconds` ve `scan_age_seconds` var, bayat her Dashboard
    yüklemesinde tam bir koşullu tarama (1126 ms ve 1590 ms), tarama bayatlayınca
    Bileşenler sayfasının koşullu taraması (Ubuntu; Debian ölçülmedi).
    Debian 13'te alpha.83 yeniden derlemesinden adaya güncelleme, üç koşu, her
    biri `update_verified`, şema öncesinde ve sonrasında 44 (etiket ile aday
    arasında göç yok); güncellemeden sonra Dashboard kendiliğinden kontrol etti
    (bir tarama, "6 / 6"), Posta yığınları kartının kendi "Yeniden taramak için
    Bileşenleri aç" metni yaklaşık 1,2 sn görüldü (bilinen açık madde, yeni
    değil). Kusurlu aday kendiliğinden döndü (`recovered`, `rollback_verified`,
    iki deneme, `payload_restored`da yeniden başlatma) ve Panel alpha.83 olarak
    yanıtladı. **Açık, kaydedildi ve hükme bağlanmadı:** geri dönüşten sonra
    laboratuvarın veritabanı karşılaştırması `managed_site_files`'ı farklı buldu
    (yalnız özetler; kaynaktan olası neden Panel'in her başlangıçta
    `observed_at` ve öteki gözlem alanlarını yeniden yazması,
    `internal/services/site_file_ledger.go` 263 ile 273, 297 ile 300, 327, 391,
    426. satırlar; satırlardan doğrulanmadı); ve kurtarma temeli dosyası,
    çalışan yapı alpha.83 ve alt sınır 84 iken hâlâ kusurlu adayı adlandırır
    (örüntü upd2 ve upd7'den zaten bilinir). Ana makine c koşusu sırasında
    yaklaşık iki saat uyudu; o koşunun iş yükü hükmü ortam yüzünden başarısız
    oldu, a ve b koşuları geçti. Koşulmadı: Arch, Debian'da bayat Bileşenler
    yüklemesi, kusurlu geri dönüşten sonraki okumalar. Sürüm hazırlığının
    ölçülen hiçbir derlemede olmadığı önceki sınırı kapatır. Hiçbir P0 satırını
    kapatmaz.
  - *`7024534bc`'de bileşen testleri (ondan sonra `68b79b372` yalnız sınıfları
    değiştirdi; `npm test` 1339 / 1339):* `go test ./cmd/panel/...
    ./internal/...` 7028 geçti, 44 atlandı, 0 başarısız; `npm test` 1339 / 1339;
    `npm run build` bütçe içinde. D2'nin düzeltmesi (`valid_for_seconds`),
    güvenlik duvarı ve sistem denetimi okumalarındaki Agent yanıtı (O3) ve
    Bileşenler sayfasının okuma şeridi (O2) bileşen testliydi ve on altıncı
    kayıt onları ölçtü.
  - *Burada yanıtlanan, daha önce listelenmemiş olanlar:* sahibin kartın neden
    kendiliğinden kontrol etmediği sorusu (D-032, `a5830dd3e`); on dördüncü
    kaydın O1 gözlemi, Agent durmuşken genel iç hata (`1a74eeda5`); on beşinci
    kaydın D2 kusuru, tarayıcınınkinden geride bir sunucu saatinin hiçbir
    sayfanın açılmasına izin vermemesi, ve aynı kaydın O2 ile O3 gözlemleri
    (`7024534bc`, bileşen testli, sonra on altıncı kayıtta ölçüldü).
  - *Yayımlama, 13 Ekim 2026 (UTC).* v0.1.0-alpha.83'ün yayımlanma kaydı (#207
    numaralı çekme isteği, yalnız belgeler) önce sahip tarafından squash olarak
    birleştirildi; `main` `c1623ff71` oldu. Aday dal sonra `main`'i içine aldı
    (birleştirme commit'i `dc0ed2747`; tek çakışma yukarıdaki alpha.83
    girdisindeydi ve `main`'in metnine göre çözüldü), #208 numaralı çekme isteği
    taslak olmaktan çıktı ve o uçtaki CI 24 işin 24'ünü geçti. Sahip #208'i
    `main`'e `f0157e6c8f1614cff176821172f8a9067493e625` olarak squash ile
    birleştirdi (ağaç `df896a5b7288110b5418225a1439241f84a8ba49`; çekme
    isteğinin son commit'i `dc0ed2747`'nin ağacına eşit); o commit'teki CI
    (koşu 38418859503) çalışan 23 işin 23'ünü geçti, dosya yayımlayan iş yalnız
    bir etiket için çalıştığından atlandı. Sahip etiketlemeden önce
    `CELIKPANEL_RELEASE_SEQUENCE` depo değişkenini 84 yaptı, sonra o commit'te
    `v0.1.0-alpha.84` açıklamalı etiketini (etiket nesnesi
    `e735308d19fab5166a2f2f4a2860b857ad0e9362`) oluşturup gönderdi. **Süreç
    notu:** sahip etiketi, `main` commit'indeki koşu sürerken gönderdi (8 işi
    bitmişti, hiçbiri başarısız olmamıştı); koşu sonradan başarıyla tamamlandı.
    "`main`'deki koşu bitti, sonra etiket" sırası amaçlanan sıradır ve bu kez
    izlenmedi (düzeltme 2026-10-14: bu notun önceki hâli yayımlayan işin yalnız
    paketleme işine bağlı olduğunu, test işlerine bağlı olmadığını söylüyordu;
    oysa yayımlayan iş paketleme işine, o da aynı koşunun her test ve derleme
    işine bağlıdır, yani bir etiketin dosyaları ancak o koşu geçtikten sonra
    yayımlanır; iki koşu arasındaki sıraya dair not geçerlidir); aynı ağaç
    çekme isteğinde 24 işin 24'ünü geçmişti. Bu bir süreç notudur, sürümün kusuru değildir. Etiketin CI koşusu
    (38419289967) 24 işle başarıyla tamamlandı ve sürümü 15:30:58'de ön sürüm
    olarak altı dosyayla yayımladı. Platform arşivi
    `celikpanel-v0.1.0-alpha.84-linux-amd64.tar.gz`, 66.131.994 bayt, SHA-256
    `6dc899e5dabaf471978cde47bfde4a0a649b7c3f6941b1431284c84c9743c301`, sıra
    84. Dosyalar etiketteki bir klondan yerel bir derleme konuğunda denetlendi
    (sürümü hazırlayan kişi bildirdi, burada bir dosyada kayıtlı değil): iki
    sağlama toplamı dosyası tutuyor, iki arşiv bayt bayt aynı, imzalı manifest
    (`published_at=2026-10-13T15:04:13Z`) ve Ed25519 imzası etiketteki
    anahtarla doğrulanıyor (SHA-256
    `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`;
    arşivdeki kopya ve `libexec/get.sh`'deki sabitle aynı), arşivde kayıtlı
    commit ve ağaç etikete eşit, arşivdeki `install.sh`, `libexec/get.sh` ve
    açık anahtar etiketin dosyalarıyla bayt bayt aynı, içerik denetimi ve
    kabul-lisansı denetimi ikisi de 0 koduyla çıktı, önyükleme sabitleri sıra 84
    ve v0.1.0-alpha.84. İndirme portalı paketi etiketin çıkış yapılmış
    kopyasından imza öncesi kipte derlendi (132.639.245 bayt, SHA-256
    `dd1b93d26b4a7d3ebc1a7fd618f75be210a941c66a95b83e28965ec3a30f856f`, iki
    kez bayt bayt aynı, 54 üye, dosya düzeni alpha.83 paketininkiyle aynı);
    sözleşme testi geçti, etiketin kopyasında genel doğrulayıcı (17) ve terfi
    (14) testleri OK'ti ve gerçek terfi aracının alpha.83 sitesinin deneme
    kopyasındaki yerel provası 26 istekten sonra önceki sürüm ve yedek
    korunarak `committed` ile bitti. Sahip portalı
    `deploy/publish-download-portal.ps1` ile yayımladı (önceki
    v0.1.0-alpha.83, hedef v0.1.0-alpha.84); sahibin bildirdiği: durum
    `committed`, `httpdocs.before-v0.1.0-alpha.84.20261013T160559Z…` yedeği
    sunucuda tutuluyor, tek genel doğrulama turu `status: ok` (26 istek, bir
    arşiv indirmesi, 66.671.880 bayt) ve `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`
    işaretçisi bir kez yazıldı. Genel sitenin 16:16 UTC'de dışarıdan
    salt-okunur okunması (sürümü hazırlayan kişi): `latest.txt`
    `v0.1.0-alpha.84`'tür; `latest.json` sıra 84'ü, commit'i ve SHA-256'yı
    söyler; yayımlanan manifest, imza ve sağlama toplamı dosyası doğrulanmış
    GitHub dosyalarıyla bayt bayt aynıdır; arşiv adresi `Content-Length:
    66131994` ile 200 yanıtlar (yeniden indirilmedi); `get.sh` sıra 84'ü ve
    v0.1.0-alpha.84'ü sabitler; alpha.83 manifesti hâlâ sunuluyor. **Kurulu
    paneller: bu yayımlama kaydı 13 Ekim 2026'da yazıldığında güncellenmemişti.**
    Aynı okumada kurulu iki sunucu da hâlâ v0.1.0-alpha.83'ün arayüz derlemesini
    sunuyordu (başlangıç sayfaları alpha.84'ün değil, alpha.83 arşivinin
    derlenmiş arayüz dosyasına başvuruyor). Her sahip güncellemeyi panelin
    kendi güncelleme ekranından başlatır. **Sahip aynı gün, daha sonra, 13 Ekim
    2026'da güncelledi.** 16:59 UTC'de dışarıdan yapılan bir okuma (salt-okunur,
    HTTPS, oturum açılmadan; sunuculara sahipten başka kimse dokunmadı; sürümü
    hazırlayan kişi güncellemeyi onlarda başlatmadı, izlemedi ya da
    doğrulamadı) iki sunucunun da v0.1.0-alpha.84'ün arayüz derlemesini
    sunduğunu gösterdi: doğrulanmış arşivdeki `web/dist/index.html` ile aynı
    derlenmiş arayüz dosyası, her sunucuda bayt bayt aynı (aynı SHA-256),
    `Last-Modified` 16:57:37 UTC ve 16:57:50 UTC; ikisi de hâlâ
    `Strict-Transport-Security: max-age=31536000` gönderiyor. Bu, her Panel'in
    bu sürümün arayüzünü sunduğunu gösterir; güncellemenin panel içindeki
    doğrulama sonucunu, Agent'ın sürümünü ya da ilk başlangıcın ne yaptığını
    göstermez. **Sahibin 13 Ekim 2026'da bildirdikleri** (iki ekran görüntüsü,
    İngilizce arayüz, sunucu başına bir tane; sahibin gözlemi, kabul kanıtı
    değil, laboratuvarda hiçbir şey ölçülmedi; sahibin mesajı kurulum sayfası
    görüntüsünü ilk sunucununki olarak adlandırdı, ama görüntünün kendi kenar
    çubuğu ikinci sunucuyu adlandırıyor; bu yüzden gösterdiklerine göre
    kaydedildi): iki sunucuda da kenar çubuğu `CelikPanel · v0.1.0-alpha.84 ·
    build f0157e6c8f16` yazıyor ve yapı kimliği hâlâ sözcüğün ortasından satır
    kırıyor ("build f01", sonraki satırda "57e6c8f16"); bilinen açık madde,
    değişmedi. İlk sunucu (barındırma amacı "Custom setup", alan adı yok)
    Dashboard'a düşüyor; "System services" kartı "5 / 5 running / installed"
    diyor, "Unknown" ve "Open Components to rescan" yok ve sahibin hiçbir şeye
    basması gerekmedi (alpha.83 güncellemesinden sonra aynı kart "Unknown"
    demişti ve sahip elle yeniden taramıştı); "Needs attention" kutusu
    CelikPanel'in okuduğu hiçbir şeyin işlem gerektirmediğini söylüyor; "Mail
    stacks" kartı "Partially installed" diyor; bu sunucu için ne bir hata ne de
    bir lisans sayfası bildirildi. İkinci sunucu kurulum sayfasına, 5. adıma
    düşüyor, "Setup stopped: its plan was reopened"; alpha.83'ten beri
    değişmedi: son adım dışında her adım "Completed", son adım "Stopped",
    hiçbiri "In progress" değil. Bu beklenen durum: bekleyen son denetim, ters
    (PTR) kaydın posta ana makinesini söylemesini gerektiriyor ve 13 Ekim
    2026'da genel bir çözümleyiciyle yapılan okuma PTR'nin hâlâ sahibin alan
    adına ait başka bir ana makine adını söylediğini gösteriyor; yani sahibin
    barındırma sağlayıcısındaki işlemi hâlâ bekliyor; bu sürümün bir kusuru
    değildir. Bildirilmeyenler: güncellemenin panel içindeki doğrulama sonucu,
    Agent'ın sürümü, güncelleme sırasında tam ekran lisans sayfasının görünüp
    görünmediği, `postconf -n >/dev/null` çıktısı; iki sunucunun da alan adı
    yok, bu yüzden ilk başlangıcın site dosyalarını devralması devralacak bir
    şey bulmadı ve D-031 sorusu kurulu sunucularda açık kalır; alpha.83'ten beri
    değişmedi. Yayımlanma ürün kabulü değildir. [Sürüm
    notları](docs/RELEASE-NOTES-v0.1.0-alpha.84.tr.md)nın "Yayımlama kaydı"
    bölümü (kurulu sunucular için 14. ve 15. maddeler) her adımı kimin attığını
    ya da bildirdiğini söyler.
  - *Sonraki sürüme açık, bu sürümün sınırları sürüm notunun listelediği gibi:*
    Dashboard'daki posta yığını kartının hâlâ kendi bayat kopyası ve "Yeniden
    taramak için Bileşenleri aç" düğmesi var; kenar çubuğundaki yapı kimliği bir
    sözcüğün ortasından bölünüyor (sahibin alpha.83 güncellemesinden sonraki
    gözlemi, alpha.84 güncellemesinden sonra yeniden görüldü); Ubuntu (değişikliklerden sonra), Arch, `server_setup_busy` ve hiç
    kontrol edilmemiş durum ölçülmedi; bir işlemden sonra "tamamlanmış olabilir"
    şeridi ve konuğun NTP ayarları ölçülmedi; bu adaya güncelleme yalnız Debian
    13'te, alpha.83 etiketinin yeniden derlemesinden ölçüldü; otomatik geri
    dönüşten sonra `managed_site_files` açık bulgusu ve geri dönüşten sonra
    kusurlu adayı adlandıran kurtarma temeli dosyası (yukarıdaki on yedinci
    kayda bakın);
    `mock.mjs` laboratuvar aracı ile `cold-load-set9.mjs`/`cold-load-set11.mjs`
    erişim denetiminde hâlâ yalnız `valid_until` gönderir;
    `DomainSSLSettings`teki HSTS geri çekme hâlâ bir sunucu zaman damgasını
    tarayıcının saatiyle karşılaştırır; Agent yanıtındaki "hiçbir şey
    değiştirilmedi", aynı istekte daha önce bir değişiklik olmadığını varsayar
    (birleşik-hata koruması toplu hataları kapsar; bir şeyi değiştirip sonra
    ayrı, birleşmemiş bir bağlantıda başarısız olan bir işleyici yine de bunu
    söylerdi); on altıncı kaydın O4 gözlemi (Türkçe Bileşenler sayfası güvenlik
    duvarı şeridinin sunucu cümlesini İngilizce gösteriyor,
    `ServiceList.tsx:3259-3271`, `:3286`, `:3393`), O5 (güvenlik duvarı şeridi
    yalnız açılışta okunur, `ServiceList.tsx:3289-3291`, `:3318`), O6
    (Dashboard'ın "İlgi istiyor" kutusu Agent'ı adıyla söylemiyor,
    `Dashboard.tsx:895`, `:1163-1165`; satırların hepsi `68b79b372`'de) ve
    Türkçe Bileşenler sayfasında çevrilmemiş posta yığını metni; ve yukarıdaki
    alpha.83 adayının, bu sürümün ele almadığı her sınırı.
  - Bu sürüm ya da yayımlanması hiçbir P0 işini kapatmaz ve ilerletmez.
    P0.1–P0.5 **kısmi** kalır. Yayımlanması kurulu hiçbir paneli güncellemedi;
    güncellemeyi yalnız sahip, panelin kendi güncelleme ekranından başlatır.
- **v0.1.0-alpha.85 yayımlandı, 15 Ekim 2026; sahip iki kurulu sunucuyu da
  güncelledi (aynı günün bildirimi).** 15 Ekim 2026'da aday olarak yazıldı ve aynı gün yayımlanması
  için değiştirildi. Numarayı,
  sahibin 13 Ekim 2026 tarihli kalıcı talimatı uyarınca sürümü hazırlayan seçti:
  v0.1.0-alpha.85 (sürüm hazırlığı `5443fe055`). Sahibin 14 Ekim 2026
  kararı, kurulu ikinci sunucudaki kurulumun son adımından sonra: sahibi
  bekleten bir ekran beklediğini gösterir ya da yapılacak bir şey olmadığını
  söyler ve bir sınırdan sonra dürüst bir "henüz bilinmiyor" ile biter
  ([D-033](docs/DECISIONS.tr.md)). Aday `fix/alpha85-setup-finish-wait`
  dalıydı, `main`'in `e3c8f735a`'sından (v0.1.0-alpha.84'ün yayımlanma kaydı).
  Ürün commit'leri: `a94f83cb0` (ilk ters DNS yanıtı eskiyse Agent sonraki
  çözümleyiciye sorar), `febbe09e3` (kurulum, sahibin geçen yeniden
  kontrolüyle tamamlanır, `GET /api/v1/setup?check=1&manual=1`), `c714238d6`
  ve `e358ebe3c` ("Kurulum tamamlanıyor" etkin ve sınırlı gösterilir, "Ters
  DNS bekleniyor", tamamlanırken "Planı düzenle" yok), `2e273cc55` ile
  `1c5454c53` arası (bekleme durumu denetiminin on altı değişikliği: kurulumun
  yeniden bağlanması, Bileşenler güvenlik duvarı şeridi, alpha.84'ün O4 ve
  O5'i, Dashboard'ın ilgi bildirimi, O6, Posta paketleri kartı, türlü posta
  profili cümleleri, sunucu değişikliği beklemeleri, uzun isteklerde dönen
  simgeler, HSTS'nin kalan süresi `hsts_retire_remaining_seconds`, kenar
  çubuğundaki yapı kimliği, laboratuvar taklidinin `valid_for_seconds`'ı),
  `8e68b3ce2`, `3abbe3f76` ve `0ad26074d` (denetimin sahibin aynı gün karar
  verdiği 16., 25. ve 35. satırları) ve `8dfc3ff69`, `733b798db`, `795f31674`
  ve `89cf88397` (ekran görüntüsü turunun düzeltmeleri). Bunların yanında:
  `8b09e4e61` (yayımlayan iş hakkında alpha.84 kayıtlarında bir düzeltme),
  `a87ce0b97` (sürüm kurtarma yoklayıcısı `managed_site_files` satırlarını ve
  kararlı bir özeti kaydeder), `0b43ee81e` (on ikinci tarayıcı incelemesi
  turu), `a945a4dbf` ve `e3feee06d` (set16 düzeneği ve on sekizinci gerçek
  sistem kaydı, yalnız kanıt) ve belgeler. Şema değişikliği yok, yeni rota yok
  ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.85.tr.md)).
  - *Ölçülen: on sekizinci gerçek sistem kaydı, set16, yayımlama öncesi ölçüm*
    ([set16](deploy/e2e/release-recovery/evidence/set16-20261014/README.md),
    14 Ekim 2026 UTC; geçici QEMU/KVM konukları, gerçek, başsız bir Chrome;
    sabit commit `d5d673eb`'nin (`89cf88397` artı sıra politikası) ve
    yayımlanan arşivle bayt bayt aynı olmayan, alpha.84 etiketinin yerel
    yeniden derlemesinin test lisanslı derlemeleri). Debian 13'e temiz kurulum:
    sürüm v0.1.0-alpha.85, şema 44. Debian 13 ve Ubuntu 24.04'te alpha.84'ten
    güncelleme, her biri `update_verified`, taban 85, defter 44, Alan adları tek
    satır `managed_unchanged`. Kusurlu aday ikisinde de kendiliğinden geri
    döndü (`recovered`, `rollback_verified`, iki otomatik deneme) ve geri
    dönüşten sonra defter `equal-except-volatile` idi; `managed_site_files`'ta
    yalnız `observed_at` değişti, kararlı özet eşit: bu, alpha.84'ün açık
    bulgusunu bu sabit commit için yanıtlar. Tarayıcıda: Dashboard her
    laboratuvarda yükleme başına bir koşullu tarama gönderdi ve güncellenen adaylarda
    Posta paketleri kartı "Yeniden taramak için Bileşenleri aç" olmadan kontrol ediliyor →
    "Hazır" geçti; konuğun saati yaklaşık ±90 sn kaymışken sayfa her seferinde açıldı; Agent
    durmuşken (yalnız Debian) Türkçe güvenlik duvarı şeridi dikkat yüzeyinde
    Türkçe konuştu ve tek Yeniden tara'dan sonra "Güvenlik duvarı: Açık" dedi
    (O4, O5), "İlgi istiyor" Agent'ı adıyla söyledi (O6); kenar çubuğundaki
    yapı kimliği 1280×800 ve 390×844'te tek satırda durdu. Ulaşılamayan ya da
    ölçülmeyen: "Kurulum tamamlanıyor" (hiçbir laboratuvar konuğunun ters DNS
    adı yok), güncelleme kartının meşgul durumu, Arch, temiz bir Ubuntu
    kurulumu, Posta paketleri kartının bekletilme durumu. O1–O3 gözlemleri
    (sayfanın dil niteliği, İngilizce bir güvenlik duvarı motoru adı, ham
    denetim işlemi kimlikleri), önceden var olan ve yargılanmayan. Adayın
    hiçbir kusuru gözlenmedi. Hiçbir P0 satırını kapatmaz.
  - *Bileşen testleri:* `go test ./cmd/panel/...` 1542 geçti, 6 atlandı, 0
    başarısız (`1c5454c53`'te; ondan sonra hiçbir Go değişmedi);
    `go test ./cmd/agent/...` WSL konuğunda temiz bir bağlama ad alanında 1798
    geçti, 29 atlandı, 0 başarısız (`a94f83cb0` ile `e358ebe3c` arası); `npm test` 1391/1391 ve
    bütçe içinde `npm run build` (15 Ekim 2026'da `5443fe055`'te yeniden
    çalıştırıldı); sürüm hazırlığının üç sözleşme testi geçer. On ikinci
    tarayıcı incelemesi turu: taklide karşı 8 yapılandırmada 31 durum;
    kusurları set16'dan önce düzeltildi.
  - *Burada yanıtlanan, daha önce listelenmemiş:* sahibin kurulu ikinci
    sunucudan bildirimi (kurulumun tamamlanması, ters DNS beklemesi); D-033 ve
    denetimi.
  - *alpha.84 listesinden burada yanıtlanan:* O4, O5 ve O6; Posta paketleri
    kartının bayat kopyası; Türkçe Bileşenler sayfasındaki çevrilmemiş posta
    profili metni; kenar çubuğundaki yapı kimliğinin sözcük ortasından
    kırılması; HSTS süresinin iki saati karşılaştırması; laboratuvar
    araçlarının yalnız `valid_until` göndermesi; otomatik geri dönüşten sonraki
    `managed_site_files` bulgusu (set16'nın sabit commit'i için).
  - *Yayımlama, 15 Ekim 2026 (UTC).* Sahip #210 numaralı çekme isteğini
    `main`'e `36c33c4531ee6c1306d324db7a666403176f3bf7` olarak squash ile
    birleştirdi (ağacı `c451c26de3cb179251e8dfd4b033ec7a9e18005e`); çekme
    isteğindeki CI: 23 iş başarılı, 1 iş atlandı (yayımlayan iş, yalnız
    etiket). O `main` commit'indeki koşu (38685686782) önce başarısızlıkla
    sonuçlandı: 2 iş ("Platform portability - Linux arm64 compile only" ve
    "Panel race - Server setup") hiçbir adım çalışmadan başarısız oldu; ikisinde
    de GitHub'ın şu hatası vardı: "An action could not be found at the URI
    https://codeload.github.com/actions/setup-go/tar.gz/924ae3a1…"
    (`actions/setup-go@v6` indirilemedi). Sürümü hazırlayan kişi başarısız
    işleri yeniden çalıştırdı; koşu başarıyla sonuçlandı: 24 iş, 0 başarısız.
    Etiket ancak bundan sonra gönderildi; yani "`main`'deki koşu bitti, sonra
    etiket" sırası bu kez izlendi. Sahip `CELIKPANEL_RELEASE_SEQUENCE` depo
    değişkenini 84'ten 85'e çıkardı ve `v0.1.0-alpha.85` açıklamalı etiketini
    (etiket nesnesi `c491a103f2f6913f58e5ba418a3361188ef11dde`) oluşturup
    gönderdi. Etiketin CI koşusu (38688902643) 24 işle başarıyla tamamlandı ve
    sürümü 2026-10-15T06:10:06Z'de altı dosyayla ön sürüm olarak yayımladı.
    Platform arşivi, genel arşivle bayt bayt aynı, 66.146.417 bayt, SHA-256
    `cb6424b712052df2768f6c16dd6a66c8ea05a13393e71933c38ae639c3fb0792`, sıra
    85. Dosyaları sürümü hazırlayan kişi bir indirmeden denetledi (`gh release
    download`; hazırlayan kişi bildirdi, burada bir dosyada kayıtlı değil): iki
    `.sha256` dosyası doğrulanır; manifest (332 bayt: sıra 85, sürüm
    v0.1.0-alpha.85, commit `36c33c45…`, `published_at` 2026-10-15T05:14:06Z,
    linux/amd64) ve 64 baytlık Ed25519 imzası, izlenen
    `deploy/release-signing-ed25519.pem` ile (SHA-256
    `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`;
    alpha.84'ünkiyle aynı) `openssl pkeyutl -verify -rawin -pubin` kullanılarak
    doğrulanır; arşivde 234 üye var, içindeki `libexec/get.sh` 85 /
    v0.1.0-alpha.85'i sabitler, `release-sequence-policy` 85 der, önceki 84
    `f0157e6c8f1614cff176821172f8a9067493e625`, `release.commit` `main`
    commit'idir, arayüz paketi `web/dist/assets/app-Br53Jd-B.js`'tir (242.703
    bayt, SHA-256
    `33930d92e988c87ad677de54a3e2ef5c4b53361b64c777686eb4f0121806eda4`;
    alpha.84'ünki `app-CX7VTgkx.js` idi) ve içerik denetçisi ile kabul-lisansı
    denetçisi ikisi de 0 döndürdü. Sınır: etiketin dosyaları bir derleme
    konuğundaki klondan değil, Windows'ta `git archive` ile alındı. İndirme
    portalı paketi `deploy/build-download-portal.sh` ile imza öncesi kipte
    derlendi (özel anahtar kullanılmadı): 132.665.325 bayt, SHA-256
    `80358e4b84551778906833a9750bff2ab66a79ddfca0653668a7ae2565c8d69b`, 54 üye,
    iki kez bayt bayt aynı derlendi; `releases/latest.txt` v0.1.0-alpha.85;
    paketlenen `get.sh` (SHA-256
    `9b0cf04f0cd52578d837c37c41cea5adf19d81b0eb8d8b2fdbd2c1aeaf598d2f`)
    etiketinkine eşit. Canlı sitenin bir kopyasında terfi aracı provası
    yapılmadı. Portalı sahip değil, sürümü hazırlayan kişi yayımladı; sahibin 15
    Ekim 2026 tarihli talimatı uyarınca ("bunları neden sen yapmıyorsun…
    benden istiyorsun"; bu sürümden itibaren birleştirme, depo değişkeni, etiket
    ve portal yayımı hazırlayan kişiye aittir, kurulu panel güncellemelerini ise
    yalnız sahip yapar), `deploy/publish-download-portal.ps1` ile
    (`-PreviousVersion v0.1.0-alpha.84 -TargetVersion v0.1.0-alpha.85
    -TransferTimeoutSeconds 1800`). Göreli betik yoluyla yapılan ilk çağrı
    hiçbir aktarım olmadan parametre bağlama aşamasında durdu (`Join-Path :
    Cannot bind argument to parameter 'Path' because it is an empty string`,
    19. satır: `$PSScriptRoot` bir parametre varsayılanında boş). Betiğin mutlak
    yolu ve açık `-Promoter` ile `-PublicVerifier` ile yapılan ikinci çağrı
    başarılı oldu: terfi aracı `status: committed`, önceki v0.1.0-alpha.84,
    yedek `httpdocs.before-v0.1.0-alpha.85.20261015T062427Z…`, genel doğrulama
    `status: ok` (26 sınırından 26 istek, 66.686.303 baytlık bir arşiv GET'i),
    `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`. 06:24:40Z'de dışarıdan okuma:
    `releases/latest.txt` v0.1.0-alpha.85'tir; `get.sh`
    `Strict-Transport-Security: max-age=15768000; includeSubDomains` ile
    sunuluyor ve 85'i sabitliyor; `releases/latest.json` sıra 85'i, `36c33c45…`
    commit'ini ve `cb6424b7…` SHA-256'sını söyler. **Kurulu paneller: bu kayıt
    yazılırken güncellenmemişti; sahip sonra güncelledi.** Bunu yalnız sahip,
    panelin kendi güncelleme ekranından başlatır ve iki sunucu için de öyle yaptı
    (15 Ekim 2026 tarihli altı ekran görüntüsü ve bir mesaj, yaklaşık 07:00Z'de
    alındı; 09:58Z'de kaydedildi). İkisi de Current version v0.1.0-alpha.85,
    commit `36c33c45…` gösteriyor ve kenar çubuğundaki yapı kimliği tek satırda.
    Sahibin gözlemleri, dışarıdan bir okuma ve kabul kanıtı değil: Frankfurt'un
    güncellemesi sırasında sayfa, güncellemenin kendi beklenen yeniden
    başlatması içinde, uyarı renginde bir "could not be read just now, so it is
    not known whether it has finished" bildirimi ve aynı şeyi söyleyen bir toast
    gösterdi; sahip bunun, ortada sorun yokken sorun gibi okunduğunu söylüyor.
    **Sonraki sürüm için açık (D-033, D-024):** beklenen yeniden başlatma
    penceresinde T+ sayacıyla sakin ilerleme; uyarı yüzeyi ve "bilinmiyor"
    ifadesi yalnız pencere geçtikten sonra ya da doğrulanmış başarısızlıkta;
    bir bildirim şeridi artı bir toast yerine tek bir bildirim (bakılacak kod:
    `web/src/components/SystemUpdateOperation.tsx`,
    `web/src/components/PanelUpdateCard.tsx`). Boston'un Panosu güncellemeden
    sonra 5 / 5 hizmeti, türlenmiş kısmi posta profili metnini ve "Nothing
    CelikPanel read needs action" yazısını gösterdi; Needs attention güncellemeden
    önce (alpha.84) bir okumada başarısız olmuştu, neden bilinmiyor. Frankfurt'un
    Panosu güncellemeden hemen sonra "Checking…" durumlarını gösterdi; sonuç
    yakalanmadı. "Recent activity" hâlâ ham denetim kimliklerini gösteriyor
    (set16 O3). Bildirilmedi, bu yüzden kaydedilmedi: güncelleme sırasında tam
    ekran lisans sayfası, Agent'ın sürümü, Frankfurt'un güncellemeden sonraki
    kurulum son adımı ve `postconf -n >/dev/null` çıktısı. Laboratuvar ortamı notu, ürün gerçeği değil: laboratuvar
    bilgisayarı KB5127648 numaralı Windows güncellemesinden sonra KVM'yi kaybetti
    (AMD; microsoft/WSL sorunları #41917 ve #41918) ve sahip set16'yı
    çalıştırmak için o güncellemeyi kaldırdı. Yayımlanma, ürün kabulü değildir.
    [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.85.tr.md), "Yayımlama
    kaydı", her adımı kimin yaptığını ve kimin bildirdiğini söyler.
  - *Sonraki sürüm için açık, sürüm notlarının saydığı bu sürümün sınırlarıyla:*
    `deploy/publish-download-portal.ps1` göreli betik yoluyla çöker, çünkü
    parametre varsayılanları `$PSScriptRoot`'u kullanır (varsayılanlar betiğin
    gövdesinde hesaplanmalı); hiçbir kurulu sunucu ölçülmedi; "Kurulum tamamlanıyor"a ve güncelleme
    kartının meşgul durumuna gerçek sistemde ulaşılamadı (yalnız bağlanan
    testler ve taklide karşı tarayıcı), bileşen kilidinin yeniden bağlanmasına,
    yayılan alan adı kontrollerine ve HSTS'nin kalan süresine de; yanıt veren
    çözümleyici adlandırılmaz; sonraki çözümleyici değişikliği yalnız bir Agent
    güncellemesinden sonra etkili olur; set16 bir sabit commit çalıştırdı, bu
    yüzden sürüm hazırlığı `5443fe055` (85 sabitleri) ölçülmüş hiçbir derlemede
    yok; alpha.84 tabanı yayımlanan arşivle bayt bayt aynı olmayan yerel bir
    derlemeydi; kurtarma temel dosyası bir geri dönüşten sonra kusurlu adayı
    adlandırır (set12, set15, set16); set16'nın O1–O3'ü; şablonlu metin
    (`component_not_installable`, roller, posta profilleri dışında "rescan");
    Arch; ve yukarıdaki alpha.84 sürümünün bu sürümün ele almadığı her sınırı.
  - Bu sürüm ya da yayımlanması hiçbir P0 işini kapatmaz ve ilerletmez.
    P0.1–P0.5 **kısmi** kalır; set16 bir ölçümdür, kabul değildir.
    Yayımlanması kurulu hiçbir paneli güncellemedi; güncellemeyi yalnız sahip,
    panelin kendi güncelleme ekranından başlatır.

- **v0.1.0-alpha.86 15 Ekim 2026'da yayımlandı; hiçbir kurulu sunucu henüz
  güncellenmedi.** 15 Ekim 2026'da aday olarak yazıldı, aynı gün set17 ve set17b
  gerçek sistem kayıtları ile yayımlanması için güncellendi.
  v0.1.0-alpha.86 numarasını, sahibin 13 Ekim 2026 tarihli kalıcı talimatı
  uyarınca sürümü hazırlayan seçti (sürüm hazırlığı `313f3eea2`; önceki olarak
  alpha.85 sürüm commit'i `36c33c45…` ile 85'ten sonra sıra 86). İş, sahibin
  15 Ekim 2026'da ikinci kurulu sunucunun alpha.85'e gerçek güncellemesi
  sırasındaki gözlemiyle başladı: panel güncellemenin planının söylediği gibi
  yeniden başlarken güncelleme sayfası uyarı renginde "şu an okunamadı; bu
  yüzden bitip bitmediği bilinmiyor" bildirimi ve aynı şeyi T+02:59 ile
  söyleyen bir köşe bildirimi gösterdi; sahibin sözleri özetle: hiçbir sorun
  yokken bu bir sorun gibi görünüyor. D-033 ve D-024 uygulanır; yeni karar
  kaydedilmez (D-033 altındaki tarihli bir not bu adayın onu nasıl uyguladığını
  listeler). Sahibin 15 Ekim 2026 tarihli "devam. sorun yok." yanıtı, bu
  sürümün eldeki kalemlerle çıkacağına ve ikinci sunucu işinin sonra geleceğine
  karar verdi. Aday, `main`'in `36c33c453` commit'inden ayrılan
  `fix/alpha86-update-restart-notice` dalıydı. Ürün commit'leri: `8e27dd0eb` ve
  `15d80851e` (güncellemenin kendi yeniden başlaması, başlangıçtan 6 dakika ve
  ilk kopan okumadan en az 2 dakikalık bir sınır içinde sakin ilerleme olarak,
  iki bildirim yerine tek bildirimle), `f89551e9b` (yayımlama betiği yardımcı
  betik varsayılanlarını gövdede çözer), `0082c9d08` ve `1abe1717d` (okumalar
  yalnız sekme görünürken, sekme yeniden gösterilince hemen bir okuma),
  `d2c0add01` (sayfanın dil özniteliği, set16 O1), `151969774` ve `ef7690fc0`
  (27 bileşen adı ve açıklaması arayüz dilinde, O2), `8a5f299fa`, `8a3729cbe`
  ve `7956001d0` (179 denetim eylemi cümle olarak; listeyi kaynaktan türeten
  bir Go testiyle, O3), `4372fc86e` ve `8fe746582` (`system` aktörü "Sistem"
  diye söylenir ve `system` kullanıcı adı ayrıldı, `USERNAME_RESERVED`),
  `cd2275e47`, `d2b58e7e9`, `297f1f0b1`, `00138c530`, `a6bdf410b` ve
  `21f07e7a8` (hiçbir cümle yeniden tarama istemez; Dashboard, Bileşenler
  sayfası ve bir bileşenin kendi sayfası doğrulanamayan kaydı kendileri
  kontrol eder ve tutulan kontrol kendiliğinden sürer), `80d763949`,
  `0a81043cf` ve `370afa2c9` (`component_not_installable` içinde platform,
  posta profillerinde rol adları ve çoğul gereksinimler), `e2c630faa` (Agent,
  ters DNS yanıtının arkasındaki çözümleyiciyi adlandırır:
  `reverse_dns_resolver` ve `reverse_dns_resolvers_disagree`) ve `1f564fdb0`,
  `01861bfc9` ve `e2853c873` (Mağaza'nın "Durum henüz bilinmiyor"u ve on
  dördüncü tarayıcı turunun düzeltmeleri) ve gerçek sistem kayıtlarından sonra
  `e8aa9c5c0` (set17 D1: lisans kapısı son olumlu kararını güncellemenin
  yeniden başlaması boyunca korur), `b7b0f4766` (oturum kapısı
  `503 PANEL_STARTING` yanıtını güncellemenin yeniden başlamasına bırakır) ve
  `7265d3c99` (set17b F1: gezinti kilidi yeniden başlamayı en geç
  başlangıçtan 6 dakikaya dek kapsar; sürümü hazırlayanın 15 Ekim 2026 kararı,
  D-033 ve sahibin 13 Ekim 2026 tarihli kalıcı talimatı altında, sahibin kararı
  değil); üçü de yalnız web'de. Ayrıca: `a45c1bfcb` (14. tarayıcı turu; 13. tur
  `15d80851e` içinde), `59e793b2b`, `389ce2831` ve `155a984ea` (15. tarayıcı
  turu), `4a29f4754`, `0bf1ee601` ve `1f9495f7b` (düzenekleriyle set17 ve
  set17b gerçek sistem kayıtları), `d72e4528a` (kabuldeki düzeltmeleri),
  `498dc49e1` (`main`'den birleştirilen bir laboratuvar testi yarışı) ve
  belgeler. Şema değişikliği yok, yeni yol yok
  ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.86.tr.md)).
  - *Ölçülen:* adayın daha önceki commit'lerinde, test lisanslı yapılarla
    QEMU/KVM konuklarında iki gerçek sistem kaydı; kabul kanıtı değildir:
    `313f3eea2`'de
    [set17](deploy/e2e/release-recovery/evidence/set17-20261015/README.md)
    (Debian 13'te temiz kurulum; Debian 13 ve Ubuntu 24.04'te alpha.85'ten iyi
    ve kusurlu güncelleme, `update_verified` ve
    `recovered`/`rollback_verified`, defter `equal-except-volatile`; adayın
    kendi güncelleme sayfası iki kez) ve `ecd2a5f08`'de
    [set17b](deploy/e2e/release-recovery/evidence/set17b-20261015/README.md)
    (Debian 13; bir laboratuvar gecikmesiyle yapılmış yaklaşık 3 dakikalık
    yeniden başlama kesintisi; ürünün özelliği değil, laboratuvar
    değişikliği). Sonuç: adayın güncelleme sayfası beş aday koşusunun hepsinde
    (set17'de iki, set17b'de üç) "bilinmiyor" metninden ve uyarı yüzeyinden 0
    örnekle sakin yeniden başlamayı gösterdi; alpha.85'in sayfası üç denetim
    koşusunun hepsinde ikisini de gösterdi; set17 D1'i kaydetti (iki koşunun
    birinde lisans kapısının penceresi yeniden başlamanın üstünde), düzeltmesini
    taşıyan adayda set17b'nin iki koşusunda yeniden üretilmedi (mekanizma
    kanıtlanmadı); set17b F1'i kaydetti (kilit kesintinin ortasında T+120
    sn'de bitti), sonradan `7265d3c99`'da değiştirildi ve bu gerçek sistemde
    ölçülmedi. `PANEL_STARTING` sayfaya hiç ulaşmadı; 6 dakikalık gecikme
    metni üretilmedi. Doğruluk kaynağı README'lerdir; sürüm notları onları
    özetler.
  - *Bileşen testleri:* `go test -v ./cmd/panel/... ./internal/transport/...
    ./internal/auth/...` 1602 üst düzey test geçti, 6 atlandı, 0 başarısız
    (`8fe746582`'de ve aynı sayılarla yeniden `21f07e7a8`'de);
    `go test ./cmd/agent/...` 1799 geçti, 29 atlandı, 0 başarısız, WSL
    konuğunda temiz bağlama ad alanında (`1abe1717d`'de; Agent'ın derlediği
    hiçbir şey ondan sonra değişmedi); `21f07e7a8`'den sonra hiçbir Go dosyası
    değişmedi, bu yüzden Go yalnız web'deki düzeltmelerden sonra yeniden
    koşulmadı; `npm test` 1463/1463 (`00138c530`'da ve yeniden `313f3eea2`'de),
    ardından `7265d3c99`'da 1483/1483 (ve yeniden `155a984ea`'da) ve bütçe
    içinde `npm run build`; sürüm hazırlığının üç sözleşme testi geçer. 13.
    tarayıcı turu: taklide karşı 8 yapılandırmada yeniden başlama bildiriminin
    üç durumu; 14. tur: 8 yapılandırmada 23 durum, ardından düzeltilen altı
    kusur ve `npm run build`'in bulduğu yedinci bir düzeltme (paket bütçesi);
    15. tur: 8 yapılandırmada bir yeniden başlama boyunca lisans kapısı, oturum
    kapısının `PANEL_STARTING`'i ve kilit.
  - *Burada alpha.85 listesinden ve sahibin bildiriminden yanıtlanan:*
    beklenen yeniden başlama sırasında sorun gibi görünen bildirim (c);
    yayımlama betiğinin `$PSScriptRoot` kusuru; görünürlüğe bağlanmamış kurma
    penceresi ve kurulum okumaları; set16'nın O1–O3'ü ve iki kurulu sunucuda
    ham denetim kimlikleri (h); şablonlu metin (`component_not_installable`,
    roller, posta profilleri dışında "rescan"); yanıt veren çözümleyici
    (kaynakta; Agent güncellemesi ister). Hepsi kaynakta ve bileşen
    testleriyle; hiçbiri gerçek sistemde ölçülmedi.
  - *Yayımlama, 15 Ekim 2026 (UTC).* Hepsini sürümü hazırlayan kişi yaptı,
    sahibin 15 Ekim 2026 tarihli talimatı uyarınca (birleştirme, depo
    değişkeni, etiket ve portal yayımı hazırlayan kişiye aittir; kurulu panel
    güncellemelerini yalnız sahip yapar). #213 numaralı çekme isteği (CI: 24 iş,
    hepsi başarılı ya da atlandı) `main`'e
    `8d260f317e58bed7fcd84c6cbb01823c4fe3f166` olarak squash ile birleştirildi
    (ağacı `5aaee3d01a4c80632ee3e1abec2c11c986da90df`). `main` commit'indeki
    koşu (38795120470) 24 işle başarıyla sonuçlandı; etiket ancak koşu
    bittikten sonra gönderildi, yani "`main`'deki koşu bitti, sonra etiket"
    sırası izlendi. Hazırlayan kişi `CELIKPANEL_RELEASE_SEQUENCE` depo
    değişkenini 85'ten 86'ya çıkardı ve `v0.1.0-alpha.86` açıklamalı etiketini
    (etiket nesnesi `8399668a3fc600ed3951c1c3c22739b3f4ffa35d`) oluşturup
    gönderdi. Etiketin CI koşusu (38797990342) 24 işle başarıyla tamamlandı ve
    sürüm 2026-10-15T20:18:29Z'de ön sürüm olarak altı dosyayla yayımlandı.
    Platform arşivi, genel arşivle bayt bayt aynı, 66.159.282 bayt, SHA-256
    `5949a0fd36f8a9f9f0308749962ec92c641df0e8cdbce72ed6b6316578a16a7d`, sıra 86.
    Dosyaları hazırlayan kişi bir indirmeden denetledi (`gh release download`;
    denetimler `CelikPanel-S2-Debian` konuğunda; hazırlayan kişi bildirdi,
    burada bir dosyada kayıtlı değil): iki `.sha256` dosyası doğrulanır;
    manifest (332 bayt: biçim v2, sıra 86, sürüm v0.1.0-alpha.86, commit
    `8d260f31…`, `published_at` 2026-10-15T19:35:12Z, linux/amd64, arşivin
    SHA-256'sı ve boyutu eşleşir) ve 64 baytlık Ed25519 imzası, izlenen
    `deploy/release-signing-ed25519.pem` ile `openssl pkeyutl -verify -rawin
    -pubin` kullanılarak doğrulanır (SHA-256
    `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`,
    alpha.85'inkiyle aynı); arşivde 237 üye var, `libexec/get.sh` 86 /
    v0.1.0-alpha.86'yı sabitler (SHA-256
    `0a71748503e8e8b67766577a269379eb48f92da968f1daf00f5f6c47a286dbf7`),
    `release-sequence-policy` 86 der, önceki 85
    `36c33c4531ee6c1306d324db7a666403176f3bf7`, `release.commit` ve
    `release.tree` etiketinkilerle eşittir, ikili dosyalar go1.26.5 trimpath
    CGO_ENABLED=0'dır, bekçiler 0 döndürdü ve arayüz paketi
    `web/dist/assets/app-DAGE4avW.js`'tir (248.116 bayt, SHA-256
    `262f8434444ad325249c8949b34f9d4cd3ee7bc2839aa088986ad8a166dba33c`;
    alpha.85'inki `app-Br53Jd-B.js` idi). Sınırlar: etiketin dosyaları bir
    derleme konuğundaki klondan değil, Windows'ta `git archive` ile alındı;
    arşiv alpha.85'in paketiyle dosya dosya karşılaştırılmadı. İndirme portalı
    paketi `deploy/build-download-portal.sh` ile imza öncesi kipte derlendi
    (özel anahtar kullanılmadı): 132.691.628 bayt, SHA-256
    `a36496cdca0547609b12e2aaa0d103127d5ea3ed9cebd976a857dc865151e72b`, 54
    üye, iki kez bayt bayt aynı derlendi; `releases/latest.txt`
    v0.1.0-alpha.86; terfi aracının paket kuralları paketi kabul etti; etikette
    portal sözleşme testi 0 döndürdü, genel doğrulayıcı testleri 17'de 17, terfi
    testleri 14'te 14 geçti. Canlı sitenin bir kopyasında terfi aracı provası
    yapılmadı ve `deploy/test-download-portal-publisher.ps1` çalıştırılmadı.
    Portal `deploy/publish-download-portal.ps1` ile yayımlandı (mutlak yol, açık
    `-Promoter` ve `-PublicVerifier`, `-PreviousVersion v0.1.0-alpha.85
    -TargetVersion v0.1.0-alpha.86 -TransferTimeoutSeconds 1800`):
    `CELIKPANEL_PORTAL_UPLOAD_READY`, `CELIKPANEL_PORTAL_UPLOAD_PINNED`, terfi
    aracı `status: committed`, önceki v0.1.0-alpha.85, yedek
    `httpdocs.before-v0.1.0-alpha.86.20261015T202726Z…`, genel doğrulama
    `status: ok` (26 istekten 26'sı), `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`;
    mutlak yolla yapılan bu çağrı göreli yol kusurunun düzeltmesini denemedi.
    20:27:35Z'de dışarıdan okuma: `releases/latest.txt` v0.1.0-alpha.86'dır
    (yayımdan önce 20:24:59Z'de v0.1.0-alpha.85 okunuyordu); `get.sh`
    `Strict-Transport-Security: max-age=15768000; includeSubDomains` ile
    sunuluyor ve 86'yı sabitliyor; `releases/latest.json` sıra 86'yı,
    `8d260f31…` commit'ini ve `5949a0fd…` SHA-256'sını söyler. **Kurulu
    paneller: bu kayıt yazılırken güncellenmemişti.** Bunu yalnız sahip,
    panelin kendi güncelleme ekranından başlatır. Sahibin bildireceği ve o
    zamana kadar kaydedilmeyecek olanlar: iki sunucunun güncelleme sonucu ve
    güncelleme sayfasının yeniden başlama sırasında gösterdiği (sakin bildirim,
    kapı iletişim kutusunun olmaması, kilit), güncelleme sırasında tam ekran
    lisans sayfası, Agent'ın sürümü, Frankfurt'un güncellemeden sonraki kurulum
    son adımı ve `postconf -n >/dev/null` çıktısı. Yayımlanma ürün kabulü
    değildir. [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.86.tr.md),
    "Yayımlama kaydı", her adımı kimin yaptığını ve bildirdiğini söyler.
  - *Sonraki sürüm için açık, sürüm notlarının saydığı bu sürümün sınırlarıyla:* gerçek sistem
    kayıtları `313f3eea2`'yi ve `ecd2a5f08`'i ölçtü; bu yüzden `7265d3c99`'un
    F1 kilit uzatması (kilidin iki dakikadan sonra panel yanıt verene dek
    tutulması ve değişen satırı) gerçek sistemde ölçülmedi; oturum kapısının
    `PANEL_STARTING` yolu gerçek sistemde çalışmadı; 6 dakikalık gecikme metni
    üretilmedi; Ubuntu 24.04 ve Arch set17b'de, Arch ve temiz Ubuntu kurulumu
    hiçbirinde koşulmadı; 6 dakikalık sınır laboratuvar güncellemelerine ve bir
    sahip gözlemine dayanır; yeni güncelleme ekranı ancak bundan sonraki
    güncellemede görülür; çözümleyici cümlesi Agent güncellemesi ister;
    denetim günlüğü aktör türü saklamaz; hiçbir kurulu sunucu ölçülmedi; ve
    yukarıdaki alpha.85 sürümünün bu sürümün ele almadığı her sınırı ("Kurulum
    tamamlanıyor"a ve güncelleme kartının meşgul durumuna gerçek sistemde
    ulaşılamadı, geri dönüşten sonra kurtarma temel dosyası, Arch, sahibin
    alpha.85 güncellemesinden sonra bildirmedikleri).
  - Bu sürüm ve yayımlanması hiçbir P0 işini kapatmaz ve ilerletmez. P0.1–P0.5
    **kısmi** kalır. Yayımlanması kurulu hiçbir paneli güncellemedi;
    güncellemeyi yalnız sahip, panelin kendi güncelleme ekranından başlatır.

- **v0.1.0-alpha.87 yayımlandı, 16 Ekim 2026; hiçbir kurulu sunucunun
  güncellendiği henüz bildirilmedi.** 16 Ekim 2026'da aday olarak yazıldı ve aynı
  gün yayımlanması için değiştirildi.
  v0.1.0-alpha.87 numarasını sahibin 13 Ekim 2026 tarihli kalıcı talimatı
  uyarınca sürümü hazırlayan seçti (sürüm hazırlığı `471377d60`, 86'dan sonra
  sıra 87, önceki olarak alpha.86 sürüm commit'i `8d260f31…`). Sahibin, ikinci
  kurulu sunucunun (Boston, ikincil DNS) web sitelerini ve postayı birincinin
  (Frankfurt, birincil DNS) tam eşi olarak barındırması hedefinin birinci
  bölümüdür; sahibin 15 Ekim 2026 tarihli "devam. sorun yok." yanıtı bu işi
  alpha.86'nın sonrasına bıraktı. main `e3c8f735a` (13 Ekim 2026 tarihli main) üzerinde
  salt-okur bir inceleme sekiz boşluk buldu; sahibin kararını gerektirmeyen beşi düzeltildi
  ve ikinci bölüm için sahibin üç kararı açık kalır: birincilin kendi kurulumu
  eşin posta sunucusu adını taşısın mı, birincildeki altyapı bölgesi için bir
  düzenleyici ve ana makine kayıtları uzak DNS bağlantısıyla kendiliğinden mi
  gitsin. O zamana dek Boston'un posta sunucusu adının kaydını sahip
  Frankfurt'ta ya da DNS sağlayıcısında yapar. D-021 ve D-033 uygulanır; yeni
  karar kaydedilmez (ikisinin altında tarihli notlar). Sürümü hazırlayanın birinci
  bölüm için verdiği kararlar (sürüm notlarında listelidir; set18b O5'in "Özel
  kurulum" etiketi de bunlardandır) D-021, D-033 ve sahibin 13 Ekim 2026 tarihli
  kalıcı talimatı altında verildi; hiçbiri sahibin kararı değildir ve sahip
  onaylayabilir ya da değiştirebilir. Aday,
  `feat/alpha87-second-server` dalıydı (#215 numaralı çekme isteği),
  `main`'in alpha.86 sürüm commit'i `8d260f317`'den. Ürün commit'leri:
  `99e90ac74` (`POST /api/v1/setup/extend`, genişletme işareti ve tabanı,
  durum alanları `extension` ve `extension_base`), `eddc11610` (düzeltilen
  posta sunucusu adı yalnız işlemi kabul edilince kaydedilir), `60ea75c1c`
  (ikincil DNS sunucusunda posta kutusu birincili ve tam kaydı adlandırır,
  `mail_dns_guidance`), `f51d98e4b` (hazır ekranından ve Ayarlar'dan "Bu
  sunucunun kurulumunu genişletin", incelemenin üç bloğu, "Genişletmeden
  vazgeç"; her açık denetim için tek türlü cümle), `450af0d88` ve `32f0a3766`
  (hazırlanmış sunucuda kurulum üzerinden Bileşenler postası, posta sunucusu
  adının "Değiştir"i, ikincil kutusu), `c40f14f87` ve `b67987865` (16.
  tarayıcı turundan yedi düzeltme) ve `6b363ac46` (set18 D1: genişletme artık
  tamamlanmış çalışmayla açılmaz, ek alan `superseded`; O1: başlayan çalışma
  ilk adımını adlandırır; O3: basılan devam tek uçuşlu kalır). Ayrıca:
  `343d3eb9e` (işlem yönlendirmesi girdisi), `5d54aba1e` (16. tarayıcı turu),
  `ddea67344` ve `0be5c8e5a` (düzenekleriyle set18 ve set18b gerçek sistem
  kayıtları), `90b3fe2f2` (teslim alma düzeltmeleri), `main`'in birleştirilmesi
  (`bef12820e`) ve belgeler. Şema değişikliği yok (44; genişletme tabanı bir
  `panel_settings` satırıdır); `internal/` ya da `cmd/agent/` altında hiçbir
  şey değişmedi ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.87.tr.md)).
  - *Ölçülen:* iki gerçek sistem kaydı; test lisanslı derlemelerle Debian 13
    QEMU/KVM konukları ve bu kanıt klasöründe ilk kez konuğun içinde bir
    laboratuvar DNS yanıtlayıcısı ve bir laboratuvar ACME sunucusu (pebble);
    dışarı giden 443 paket sunucuları dışında reddedildi, 25 numaralı bağlantı
    noktasını konuğun kendi Postfix'i yanıtladı; kabul kanıtı değil:
    [set18](deploy/e2e/release-recovery/evidence/set18-20261015/README.md)
    `5d54aba1e`'de (genel DNS denetimi ve güvenilir panel sertifikasıyla "Your
    server is ready"e kurulum; Bileşenler posta yolu; posta kaydı beklemesi,
    posta sertifikası, 1.1.1.1'i adlandıran ters DNS cümlesi ve "Finishing
    setup" üzerinden Core Mail ekleyen genişletmenin hazır duruma varışı; D1
    kaydedildi, genişletme tamamlanmış çalışmayla açılıyordu) ve
    [set18b](deploy/e2e/release-recovery/evidence/set18b-20261016/README.md)
    `6b363ac46`'da (aynı akış, her adıma sayfada basılarak ekranda; D1, O1 ve
    O3'ün düzeldiği gözlendi; yeni kusur yok; O5, posta eklendikten sonra
    "Özel kurulum" amaç etiketi, kaydedildi ve olduğu gibi bırakıldı). Doğruluk
    kaynağı README'lerdir; sürüm notları onları özetler.
  - *Bileşen testleri:* `npm test` 1499'da 1499 (set18 düzeltmelerinden sonra
    ve yeniden `bef12820e`'de); `go test -count=1 -v ./cmd/panel/...` 1558
    üst düzey test geçti, 6 atlandı, 0 kaldı, 1675 alt test geçti,
    `bef12820e`'de (`6b363ac46` ile aynı Go kodu; sürümü hazırlayanın WSL konuğundaki ölçümü,
    bir dosyada kayıtlı değil), `go vet` ve gofmt temiz;
    Agent koşulmadı (değişmedi). 16. tarayıcı turu: taklide karşı 8
    yapılandırmada 36 durum, yedi kusur düzeltildi. Sürüm hazırlığının sıra
    politikası sözleşmesi geçiyor ("86 -> 87"); portal ve imzalı bildirim
    sözleşmeleri WSL konuğunda geçiyor (sürümü hazırlayanın bildirimi, bir dosyada
    kayıtlı değil).
  - *Yayımlama, 16 Ekim 2026 (UTC).* Hepsi sürümü hazırlayan kişi tarafından,
    sahibin 15 Ekim 2026 tarihli talimatı uyarınca (birleştirme, depo değişkeni,
    etiket ve portal yayımı hazırlayana aittir; kurulu panel güncellemelerini
    yalnız sahip yapar). #215 numaralı çekme isteği (CI: 24 iş, hepsi başarılı ya
    da atlandı) `main`'e `a8cddcb546302daa0f5eb65b18fe6b9566b69e66` olarak squash
    ile birleştirildi (ağaç `c63a9600572cba0dc6bd19ecb0388acb45b395c4`). `main`
    commit'indeki koşu (38834849023) 24 işle başarıyla sonuçlandı; etiket ancak
    koşu bittikten sonra gönderildi, yani "`main`'deki koşu bitti, sonra etiket"
    sırası izlendi. Hazırlayan kişi `CELIKPANEL_RELEASE_SEQUENCE` depo değişkenini
    86'dan 87'ye çıkardı ve `v0.1.0-alpha.87` açıklamalı etiketini (etiket
    nesnesi `f277e0b469acd3a33b9fb87c3327566ef1c2ba04`) oluşturup gönderdi.
    Etiketin CI koşusu (38836331003) 24 işle başarıyla tamamlandı ve sürüm
    2026-10-16T01:57:38Z'de ön sürüm olarak altı dosyayla yayımlandı. Platform
    arşivi, genel arşivle bayt bayt aynı, 66.182.207 bayt, SHA-256
    `d7b33548d431c05f99e2d98d3052692647a3c662c9cb57d60fb03bf5e5a91d60`, sıra 87.
    Dosyaları hazırlayan kişi bir indirmeden denetledi (`gh release download`;
    denetimler `CelikPanel-S2-Debian` konuğunda; hazırlayan kişi bildirdi, burada
    bir dosyada kayıtlı değil): iki `.sha256` dosyası doğrulanır; manifest (332
    bayt: biçim v2, sıra 87, sürüm v0.1.0-alpha.87, commit `a8cddcb5…`,
    `published_at` 2026-10-16T01:23:21Z, linux/amd64, arşivin SHA-256'sı ve
    boyutu eşleşir) ve 64 baytlık Ed25519 imzası, izlenen
    `deploy/release-signing-ed25519.pem` ile `openssl pkeyutl -verify -rawin
    -pubin` kullanılarak doğrulanır (SHA-256
    `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`; alpha.85
    ve alpha.86'nınkiyle aynı); arşivde 237 üye var, `libexec/get.sh` 87 /
    v0.1.0-alpha.87'yi sabitler (SHA-256
    `7c8823849cb2fb310b4a58d178364c9ed4d89c4d94908f0cf0856df3fb577b5d`),
    `release-sequence-policy` 87 der, önceki 86
    `8d260f317e58bed7fcd84c6cbb01823c4fe3f166`, `release.commit` ve
    `release.tree` etiketinkilerle eşittir, ikili dosyalar go1.26.5 trimpath
    CGO_ENABLED=0 ile derlenmiştir ve kabul etiketi yoktur, bekçiler 0 döndürdü,
    arayüz paketi `web/dist/assets/app-D8EjyWzW.js`'tir (248.119 bayt, SHA-256
    `b86955f5b8d4f1b3f0af15612c983b118037390e1cc01615cafa1ace4a184cb1`;
    alpha.86'nınki `app-DAGE4avW.js` idi). Sınırlar: etiketin dosyaları bir
    derleme konuğundaki klondan değil, Windows'ta `git archive` ile alındı; arşiv
    alpha.86'nın paketiyle dosya dosya karşılaştırılmadı. İndirme portalı paketi
    `deploy/build-download-portal.sh` ile imza öncesi kipte derlendi (özel
    anahtar kullanılmadı): 132.735.450 bayt, SHA-256
    `d7ec2b4940457d0b9bbe717d50506fd343ebed869b1f441348d670f35dbcf88c`, 54 üye,
    iki kez bayt bayt aynı derlendi; `releases/latest.txt` v0.1.0-alpha.87; terfi
    aracının paket kuralları kabul etti; etikette portal sözleşme testi 0
    döndürdü, genel doğrulayıcı testleri 17'de 17, terfi testleri 14'te 14
    geçti; dört portal betiği alpha.86'dan beri değişmedi. Canlı sitenin bir
    kopyasında terfi aracı provası yapılmadı ve
    `deploy/test-download-portal-publisher.ps1` çalıştırılmadı. Portal
    `deploy/publish-download-portal.ps1` ile yayımlandı (mutlak yol, açık
    `-Promoter` ve `-PublicVerifier`, `-PreviousVersion v0.1.0-alpha.86
    -TargetVersion v0.1.0-alpha.87 -TransferTimeoutSeconds 1800`):
    `CELIKPANEL_PORTAL_UPLOAD_READY`, `CELIKPANEL_PORTAL_UPLOAD_PINNED`, terfi
    aracı `status: committed`, önceki v0.1.0-alpha.86, yedek
    `httpdocs.before-v0.1.0-alpha.87.20261016T020434Z…`, genel doğrulama
    `status: ok` (26 istek), `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`. 02:04:42Z'de
    dışarıdan okuma: `releases/latest.txt` v0.1.0-alpha.87'dir (yayımdan önce
    02:02:54Z'de v0.1.0-alpha.86 okunuyordu); `get.sh`
    `Strict-Transport-Security: max-age=15768000; includeSubDomains` ile
    sunuluyor ve 87'yi sabitliyor; `releases/latest.json` sıra 87'yi,
    `a8cddcb5…` commit'ini ve `d7b33548…` SHA-256'sını söyler. **Kurulu paneller:
    bu kayıt yazılırken güncellendiği bildirilmemişti.** Bunu yalnız sahip,
    panelin kendi güncelleme ekranından başlatır. alpha.87, alpha.86'dan
    yaklaşık 5,5 saat sonra ve sahip herhangi bir sunucuyu alpha.86'ya
    güncellediğini bildirmeden önce geldi; bu yüzden alpha.86 gözlemleri de hâlâ
    açıktır; güncelleyicinin sıra tabanı alpha.85'ten ya da alpha.86'dan doğrudan
    alpha.87'ye güncellemeye izin verir (politikanın `previous_commit` değeri bir
    derleme sözleşmesi sabitlemesidir, kurulum önkoşulu değildir;
    `cmd/agent/system_update_contract.go` dosyasının okunmasından ve `cmd/`
    altında `previous_commit` denetiminin bulunmamasından çıkarılmıştır, ölçülmüş
    bir güncelleme değildir). Sahibin bildireceği ve o zamana kadar
    kaydedilmeyecek olanlar: iki sunucunun güncelleme sonucu (hangi sürümden
    olursa olsun), güncelleme sayfasının yeniden başlama sırasında gösterdiği
    (alpha.86'dan güncellemede sakin yeniden başlama bildirimi kurulu bir
    sunucuda ilk kez görülmelidir; alpha.85'ten güncellemede görünen sayfa
    alpha.85'inkidir), tam ekran lisans sayfası, Agent'ın sürümü, Frankfurt'un
    kurulum son adımı, `postconf -n >/dev/null` ve Boston'un hazır ekranında
    "Bu sunucunun kurulumunu genişletin" seçeneğinin görünüp görünmediği.
    Yayımlanma ürün kabulü değildir. [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.87.tr.md),
    "Yayımlama kaydı", her adımı kimin yaptığını ve bildirdiğini söyler.
  - *Sonraki sürüm için açık, bu sürümün sürüm notlarının saydığı sınırlarla:*
    ikinci bölüm (sahibin üç kararı); ikincil DNS kutularına gerçek sistemde
    ulaşılmadı; çözümleyicilerin ayrıştığı cümle ve yavaş "Kurulum
    tamamlanıyor" çeşidi üretilmedi; genel ACME ve DNS yolu değil,
    laboratuvar; yalnız Debian 13; ham dosyalarda gözlenen ve
    değerlendirilmeyenler: yalnız web kurulumunda her Panel başlangıcında
    "not a valid FQDN" günlük satırı, `cron DEATH` satırları, güvenilir posta
    sertifikası ardından gelirken bir "self-signed fallback" bildirimi (D-033
    adayı bir iş) ve kurulumun kendi planlı yeniden başlaması çevresinde lisans
    kapısının "latest access check failed" şeridi (alpha.86'nın güncelleme
    yeniden başlama payı bunu kapsamaz); O2 ve O4 değerlendirilmedi, O5
    sürümü hazırlayanın kararıyla olduğu gibi bırakıldı; hiçbir kurulu sunucu
    ölçülmedi; ve yukarıdaki alpha.86 sürümünün bu sürümün
    ele almadığı her sınırı.
  - Bu sürüm ya da yayımlanması hiçbir P0 işini kapatmaz ya da ilerletmez.
    P0.1–P0.5 **kısmi** kalır. Yayımlanması hiçbir kurulu paneli güncellemedi;
    bunu yalnız sahibi, panelin kendi güncelleme ekranından başlatır.

- **v0.1.0-alpha.88 16 Ekim 2026'da yayımlandı; hiçbir kurulu sunucu onu çalıştırmadı.**
  Aday olarak yazıldı, aynı gün yayımlanması için değiştirildi. Numarayı sahibin 13
  Ekim 2026 tarihli kalıcı talimatı uyarınca sürümü hazırlayan seçti (sürüm
  hazırlığı `7231afdbf`, 87'den sonra sıra 88, önceki olarak alpha.87 sürüm
  commit'i `a8cddcb5…`). Tek konu: sahip 16 Ekim 2026'da Frankfurt'u
  alpha.85'ten alpha.87'ye güncelleyip eski "bağlantı kesildi" uyarısını
  görünce her adım için bir ilerleme çubuğu, bir sonrakinin altta belirmesini,
  ürkütücü hiçbir şey olmamasını ve sonda güncel durumu istedi. Aday (`main`'in
  `89995199f` noktasından `feat/alpha88-update-steps` dalı): Agent
  güncelleyicinin indirme ve doğrulama ilerlemesini okur (`b1d2ee381`); Panel
  başlatma ve durum yanıtlarında beş adım bildirir ve yeniden başlamayı
  `panel_settings` içinde `panel_update_last_restart` anahtarına ölçer
  (`22cdc2c4a`); sayfa her adım için bir çubuk, tek sakin cümleli planlı bir
  adım olarak yeniden başlamayı ve sonda "sürümüne güncellendi"yi çizer
  (`8b96ca5ce`, `b63da83e0`); adımların yanındaki kilit cümlesi kısaldı
  (`07aaa50d5`). D-024, D-032 ve D-033 uygulanır; yeni karar yok. Şema
  değişikliği yok (44) ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.88.tr.md)).
  - *Ölçülen:* web `npm test` `07aaa50d5`'te 1523'te 1523; Go `b63da83e0`'da
    (panel ve transport 1602 test, 1695 alt test, 6 atlanan; Agent 1805 test,
    2838 alt test, 29 atlanan, temiz ad alanı); tarayıcı turu 17 (8
    yapılandırmada 9 durum, bir kusur düzeltildi). **Sahibin 16 Ekim 2026
    talimatıyla gerçek sistem kaydı yok.**
  - *Açık:* Agent'ın ilerleme okuması, taban zaman damgası ve yeniden başlama
    ölçümü gerçek bir güncellemede gözlenmedi; adımlar ilk kez alpha.88'den
    güncellemede görülür (alpha.88'e güncelleme alpha.87'nin sayfasında
    çalışır), bu yüzden alpha.89 aynı kodla gelir; aynı sürümün yeniden
    denenmesi doğrulama sınırını bildirmez; hiçbir kurulu sunucu ölçülmedi.
    Hiçbir P0 işi kapanmaz ya da ilerlemez; P0.1–P0.5 **kısmi** kalır;
    yayımlanma kabul olmaz.
  - *Yayımlama, 16 Ekim 2026 (UTC).* Sürümü hazırlayan kişi tarafından, sahibin
    15 Ekim 2026 talimatı uyarınca; kurulu panel güncellemelerini yalnız sahip
    yapar. #217 numaralı çekme isteği `main`'e
    `1cb3628ff1969f3fc2e9f0d848b7adf629a815b0` olarak squash ile birleştirildi
    (ağaç `abc2a0a3ee177f064d6b8af8304d66ae1feb1c63`); üzerindeki koşu
    (38865909812) 24 işle başarıyla sonuçlandı (hazırlayan kişinin zincir betiği
    önce erken durdu, çünkü `gh run watch` koşu bitmeden döndü; herhangi bir
    değişken ya da etiket adımından önce güvenle durdu ve devam ettirildi); depo
    değişkeni 87'den 88'e çıkarıldı; `v0.1.0-alpha.88` açıklamalı etiketi (nesne
    `851766854005a2a223edd9e097afd7853a02ff30`) `main` koşusundan sonra
    gönderildi; etiketin koşusu (38867798325) başarıyla sonuçlandı; sürüm
    2026-10-16T07:49:41Z'de ön sürüm olarak altı dosyayla yayımlandı. Bir
    indirmeden `CelikPanel-S2-Debian` konuğunda denetlendi (hazırlayan kişi
    bildirdi, burada bir dosyada kayıtlı değil): arşivler bayt bayt aynı,
    66.196.013 bayt, SHA-256
    `feaf77476a100425a2f048b47ece9da85a45585b5b8f3387692b497ad04a45d3`;
    manifest sıra 88, imza doğrulanır (anahtar `7eadeb0b…`); 237 üye;
    `libexec/get.sh` 88'i sabitler; politika 87'den sonra 88; `release.commit`
    ve `release.tree` etiketinkilerle eşit; go1.26.5 trimpath CGO_ENABLED=0;
    arayüz `app-DAi4coqL.js` (248.222 bayt). Portal paketi 132.766.352 bayt,
    SHA-256 `83bfe1327b4802f7c2d9b1ffdbb371227b1418a84c33a36cb8a83b502bfa63b5`,
    iki kez aynı derlendi, terfi aracı kabul etti; hazırlayan kişi yayımladı
    (committed, yedek `httpdocs.before-v0.1.0-alpha.88.20261016T080028Z…`,
    doğrulama ok, 26 istek); 08:00:37Z'de dışarıdan okundu (latest.txt, get.sh
    sabitlemesi ve latest.json hep 88). **Kurulu paneller:** sahip iki
    güncellemeyi de panelin kendi güncelleme ekranından başlattı ve ikisi de
    önceki sürümünden doğrudan alpha.89'a geçti; yani hiçbiri alpha.88'i
    çalıştırmadı (alpha.89 girdisine bakın). Sınırlar: etiket dosyaları
    Windows'ta `git archive` ile, canlı sitenin kopyasında terfi aracı provası
    yok, alpha.87'nin paketiyle dosya dosya karşılaştırma yok, yayımlayıcının
    kendi testi çalıştırılmadı. Yayımlanma kabul değildir. Kim neyi yaptı ve
    kim neyi bildirdi, [sürüm notlarının](docs/RELEASE-NOTES-v0.1.0-alpha.88.tr.md)
    "Yayımlama kaydı"nda yazılıdır.

- **v0.1.0-alpha.89 16 Ekim 2026'da yayımlandı; iki kurulu sunucuyu da sahibi,
  önceki sürümlerinden doğrudan buna güncelledi.** Aday olarak yazıldı, aynı gün
  değiştirildi. alpha.88 ile aynı ürün kodu
  (`git diff --stat 1cb3628ff HEAD -- cmd internal web` boştur); sahibin 16 Ekim 2026 talimatıyla hemen ardından çıkar, böylece güncelleme adımları ilk kez alpha.88'den güncellemede görülür; numarayı sürümü hazırlayan seçti. 88'den sonra sıra 89, şema 44, gerçek sistem kaydı yok; P0.1–P0.5 **kısmi** kalır ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.89.tr.md)).
  - *Yayımlama, 16 Ekim 2026 (UTC).* Sürümü hazırlayan kişi tarafından, aynı
    talimatla. #218 numaralı çekme isteği `main`'e
    `ac36fccf4e4238a8bc4624356a478ba8dab20b34` olarak squash ile birleştirildi
    (ağaç `29f08b6bef1ca6e19c4f02890f83cfeb3f678433`); `main` koşusu başarılı;
    değişken 88'den 89'a; `v0.1.0-alpha.89` etiketi; etiketin koşusu
    (38874797963) başarılı; sürüm 2026-10-16T08:57:23Z'de altı dosyayla
    yayımlandı. Bir indirmeden denetlendi (hazırlayan kişi, burada bir dosyada
    kayıtlı değil): arşivler bayt bayt aynı, 66.195.105 bayt, SHA-256
    `58c6911be7a1b640b0bafb595a8c2b54451d70d8238d51b1c726a76bb7f59526`;
    manifest sıra 89, imza doğrulanır; 237 üye; `get.sh` 89'u sabitler;
    politika 88'den sonra 89; `release.commit` ve `release.tree` etiketinkilerle
    eşit; arayüz paketi alpha.88'inkiyle bayt bayt aynı (doğrudan
    karşılaştırıldı). Portal paketi 132.763.175 bayt, SHA-256
    `bb36ebce7b52261988651d9df234ebe9c26b45ec605feacb5bda9f2da1649f1e`;
    hazırlayan kişi yayımladı (committed, yedek
    `httpdocs.before-v0.1.0-alpha.89.20261016T090510Z…`, doğrulama ok, 26
    istek); 09:05:19Z'de dışarıdan okundu (hepsi 89, HSTS var). **Kurulu
    paneller, sahibin bildirdiği (16 Ekim 2026, iki ekran görüntüsü, yaklaşık
    09:30Z):** sahip iki güncellemeyi de panelin kendi güncelleme ekranından
    başlattı. Frankfurt (alpha.87'den) ve Boston (çalıştırdığı sürümden;
    bildirilmedi, alpha.85 ya da sonrası) doğrudan alpha.89'a geçti; sıra tabanı
    buna izin verir. İkisinin Ayarlar, CelikPanel güncellemeleri sayfası geçerli
    sürüm v0.1.0-alpha.89 ve commit `ac36fccf4e42…` gösterir, kenar çubuğu
    build `ac36fccf4e42` der ve "v0.1.0-alpha.89 sürümüne güncellendi" kartı beş
    tamamlanmış adım sayar: yeniden yüklemeden sonra alpha.89 arayüzü sunucunun
    adım planını çizer. Yeniden başlama sırasındaki canlı adım görünümü
    **görülmedi** (her güncelleme önceki sürümün sayfasında çalıştı); ilk kez
    bir sonraki güncellemede, alpha.89'dan görülür. Frankfurt'un sayfası "Bu
    kanalda yeni güncelleme yok." metnini bir uyarı üçgeni simgesiyle gösterir:
    iyi bir durum uyarı simgesiyle çizilmiş; sonraki sürüm için D-033 maddesi,
    burada değiştirilmedi. Bildirilmeyen: lisans sayfası, Agent'ın sürümü,
    Frankfurt'un kurulum son adımı, Boston'un "Bu sunucunun kurulumunu
    genişletin" seçeneği, `postconf -n`. Yayımlanma kabul değildir; hiçbir P0 işi
    kapanmaz ya da ilerlemez; P0.1–P0.5 **kısmi** kalır. Ayrıntı
    [sürüm notlarının](docs/RELEASE-NOTES-v0.1.0-alpha.89.tr.md) "Yayımlama
    kaydı"nda.

- **v0.1.0-alpha.90 yayımlandı, 16 Ekim 2026; sahip iki kurulu sunucuyu da bu sürüme güncelledi, Frankfurt kendi kurtarması üzerinden.** Aday olarak yazıldı ve aynı gün değiştirildi. alpha.89'dan beri bir düzeltme (D-033): "Güncellemeyi kontrol et" bir şey bulmadığında Ayarlar, CelikPanel güncellemeleri sayfası, uyarı üçgenli "Bu kanal için yeni güncelleme yok." yerine normal yüzeyde onay işaretiyle "En son sürümü kullanıyorsunuz." der; aynı kartta bulunan bir güncelleme ve süren hazırlık okuması nötrdür, uyarı işareti yalnız okunamayan denetim, hazır olmayan sunucu, başlatılamayan güncelleme, daha önce başarısız olan deneme ve alpha uyarısında kalır. Sahibin 16 Ekim 2026 talimatıyla ("90'ı çıkar") çıkar; böylece adım adım güncelleme görünümü alpha.89'dan güncellemede canlı görülür; numarayı sürümü hazırlayan seçti. 89'dan sonra sıra 90, şema 44, geçiş yok, gerçek sistem kaydı yok; P0.1–P0.5 **kısmi** kalır ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.90.tr.md)).
  - *Yayımlama, 16 Ekim 2026 (UTC).* Sürümü hazırlayan kişi tarafından, sahibin
    15 Ekim 2026 talimatı uyarınca; kurulu panel güncellemeleri sahibindi. #220
    numaralı çekme isteği `main`'e `1743ac69edb0e5723dc60d992b53094dcc4b1431`
    olarak squash ile birleştirildi (ağaç
    `bbfa5305cc8c5b6bb4a34a2e640633582dc7534b`); `main` koşusu başarılı;
    değişken 89'dan 90'a; açıklamalı etiket `v0.1.0-alpha.90` (nesne
    `9c98483d71b4a747f522f87c476f22125868bca7`); etiket koşusu başarılı; sürüm
    2026-10-16T14:42:18Z'de altı dosyayla yayımlandı. Bir indirmeden denetlendi
    (hazırlayan kişi, burada bir dosyada kayıtlı değil): arşivler bayt bayt aynı,
    66.194.918 bayt, SHA-256
    `3719aba4f60b3eb3da0762b2d1c57302b628cdb6c958d72c2a5ac0430e2ff3e1`;
    manifest sıra 90 (`published_at` 2026-10-16T13:58:20Z), imza doğrulanır
    (anahtar `7eadeb0b…`); 237 üye; `get.sh` 90'ı sabitler; politika 90, önceki
    89; `release.commit` ve `release.tree` etiketinkilerle eşit; arayüz
    `app-HNXfBLHb.js` (248.218 bayt). Portal paketi 132.762.586 bayt, SHA-256
    `b2a31a43d2b1e497b7dc9da53f405d79fa70d7193cb86f518f5d367e74b3acef`, 54 üye,
    iki kez aynı derlendi, terfi aracı kabul etti, testler ok; hazırlayan kişi
    yayımladı (committed, yedek
    `httpdocs.before-v0.1.0-alpha.90.20261016T145144Z…`, doğrulama ok, 26
    istek); 14:51:50Z'de dışarıdan okundu (latest.txt, get.sh sabiti ve
    latest.json hep 90). **Kurulu paneller, sahibin bildirdiği (ekran
    görüntüleri, 16 Ekim 2026, alpha.89'dan güncelleme):** Boston temiz güncellendi
    (kart "v0.1.0-alpha.90 sürümüne güncellendi", build `1743ac69edb0`, beş adım,
    ardından "Güncellemeyi kontrol et"ten sonra yeşil "En son sürümü
    kullanıyorsunuz."). Frankfurt'un güncellemesi T+01:23'te kırmızı çarpı ve
    "reviewed updater failed: exit status 1: Certbot renewal scheduler state
    could not be restored" gösterdi, T+01:39'da aynı işlem için
    (`13193201ba7a3d832bb64cc6d972f8a5`) "Güncelleme doğrulandı"; tarayıcısı
    Ctrl+Shift+R'ye kadar alpha.89 arayüzünü korudu (kenar çubuğu alpha.89,
    "Geçerli sürüm" alpha.90), yeniden yüklemeden sonra güncelleme kurtarma kaydı
    üzerinden bittiği için "güncellendi" kartı yoktu. Boston'un kartı
    kopyalanınca yapı satırı kısa ve tam kimlik yapışık yapıştı. Frankfurt
    hatasının nedeni ve düzeltmesi: alpha.92 girdisi. Canlı adım görünümü görüldü
    diye bildirilmedi. Yayımlanma kabul değildir; hiçbir P0 işi kapanmaz ya da
    ilerlemez; P0.1–P0.5 **kısmi** kalır. Ayrıntı [sürüm
    notlarının](docs/RELEASE-NOTES-v0.1.0-alpha.90.tr.md) "Yayımlama
    kaydı"nda.
- **v0.1.0-alpha.91 16 Ekim 2026'da etiketlendi ve GitHub'da yayımlandı, bilerek indirme portalına konmadı; hiçbir kurulu sunucu onu çalıştırmadı.** Aday olarak yazıldı ve aynı gün değiştirildi. Sahibin 16 Ekim 2026 tarihli üç kararından ("önerin gibi olacak": ikinci sunucu tam bir web ve posta eşi olabilir; altyapı bölgesi birincide Alan adları'nın normal bir girdisidir, kurulumun oluşturduğu kayıtları kurulum korur ve sahibin kendi kayıtları korunur; ikinci sunucunun ana makine kayıtları var olan yetkili uzak DNS bağlantısı üzerinden birinciye otomatik gider, elle girmek de mümkün kalır) ve kullanıcıların kendi sunucularını yapılandırdığı sözünden (panel sunar; karar kullanıcınındır) sonra ikinci sunucu tam eş olarak, 2. bölüm. Boşluk 4: eşin posta sunucusu adı birincinin planında; boşluk 5: Alan adları'nda altyapı bölgesi, kayıt düzenleyicisi ve `managed_by`; boşluk 6: `POST /api/v1/dns/remote/receiver/host-record` ve ikincilin adım başına bir kez gönderdiği istek ile sonucunun kurulum sayfasında gösterilmesi. Numarayı sahibin 13 Ekim 2026 tarihli kalıcı talimatı uyarınca sürümü hazırlayan seçti; sekiz uygulama seçimi hazırlayanındır, onaylanacaktır. 90'dan sonra sıra 91, şema 44, geçiş yok, sahibin 16 Ekim 2026 talimatıyla gerçek sistem kaydı yok; kurulu hiçbir sunucuda ölçüm yok; P0.1–P0.5 **kısmi** kalır ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.91.tr.md)).
  - *Etiket, 16 Ekim 2026 (UTC).* Sürümü hazırlayan kişi tarafından, sahibin
    15 Ekim 2026 talimatı uyarınca. #221 numaralı çekme isteği `main`'e
    `356109b532b9d0d0df5dd31fc19e9932918a9648` olarak squash ile birleştirildi;
    `main` koşusu başarılı; değişken 90'dan 91'e; etiket `v0.1.0-alpha.91`;
    etiket koşusu başarılı; GitHub sürümü 2026-10-16T15:48:29Z'de altı dosyayla
    yayımlandı. **Hazırlayan kişinin kararıyla portala konmadı:** Frankfurt'un
    alpha.89'dan alpha.90'a güncellemesi alpha.91'de de bulunan bir güncelleyici
    kusurunu gösterdi; bu yüzden ikinci sunucu işi alpha.92 ile gelir. Onun için
    dosya denetimi ve portal paketi yapılmadı; canlı portal alpha.90'dan
    alpha.92'ye geçti ve sahibin güncellemeleri alpha.90'dan alpha.92'ye gitti.
    Yayımlanma kabul değildir; hiçbir P0 işi kapanmaz ya da ilerlemez;
    P0.1–P0.5 **kısmi** kalır. Ayrıntı [sürüm
    notlarının](docs/RELEASE-NOTES-v0.1.0-alpha.91.tr.md) "Yayımlama
    kaydı"nda.
- **v0.1.0-alpha.92 yayımlandı, 16 Ekim 2026; sahip iki kurulu sunucuyu da alpha.90'dan bu sürüme güncelledi, Frankfurt'ta hata ve kurtarma olmadan.** Aday olarak yazıldı ve aynı gün değiştirildi. Frankfurt'un 16 Ekim 2026 güncellemesi (alpha.89'dan alpha.90'a, işlem `13193201ba7a3d832bb64cc6d972f8a5`) son adımında durdu: `certbot.timer` yeniden açılınca Certbot'un telafi yenilemesi hemen başladı (`Persistent=true`) ve zamanlayıcıyı geri yükleme yenileme hizmetinin boşta olmasını istiyordu; sunucunun kendi kurtarması güncellemeyi 16 sn sonra tamamladı. Güncelleyici artık sahibin yenileme çalışmasını asla durdurmaz, başlatmaz, sıfırlamaz (D-022, D-025): yalnız zamanlayıcıları duraklatır, çalışan bir yenilemeyi salt-okur bekler (en çok 180 sn), başarısız son çalışmayı sahibin sonucu sayar ve hizmetleri yalnız yükleme ve etkinleştirmeden denetler; ileri yöndeki zamanlayıcı adımının kendi hata kodu var: `update_finishing_scheduler`; kurtarma işleme sahipken güncelleme sayfası ek bir `retrying` adımıyla sakin "Güncelleme tamamlanıyor" görünümünü korur ve güncellemeyi doğrulayan bir kurtarma sayfada ve güncelleme kartında "güncellendi" olarak görünür (D-024, D-033); yapı satırı göründüğü gibi kopyalanır. alpha.91 etiketlendi ama portala konmadı; bu yüzden alpha.92 onun ikinci sunucu işini de getirir. 91'den sonra sıra 92, şema 44, geçiş yok; bir yerel laboratuvar kaydı, set19 (Debian 13, QEMU, test lisanslı yapılar): güncellemenin kendi zamanlayıcı açılışı yenilemeyi başlattı ve güncelleme hatasız, kurtarmasız `update_verified` ile bitti; güncellemeden önce başarısız olmuş yenileme hizmeti sahibin durumu olarak kaldı ve güncelleme aynı biçimde bitti; sayfa sakin kaldı; P0.1–P0.5 **kısmi** kalır ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.92.tr.md)).
  - *Yayımlama, 16 Ekim 2026 (UTC).* Aynı talimatla sürümü hazırlayan kişi
    tarafından. #222 numaralı çekme isteği `main`'e
    `961f2cb2fcaeb48c08fc58828d15fe0b231e1e1a` olarak squash ile birleştirildi
    (ağaç `091d5fb23cb16cbd217c8c1191033185a7d50b81`); ilk CI koşusu "Verify
    offline release-recovery fixture contracts (no VM)" işinde başarısız oldu
    (koşum düzeneğinin yansıtılmış kod listelerinde
    `update_finishing_scheduler` yoktu), `5f662635f` ile düzeltildi, sonra 24
    işin 24'ü; `main` koşusu başarılı; değişken 91'den 92'ye; açıklamalı etiket
    `v0.1.0-alpha.92` (nesne `16a8a961aa9f06948902e4c972c4ce6603cc50fc`); etiket
    koşusu başarılı; sürüm 2026-10-16T18:51:22Z'de altı dosyayla yayımlandı. Bir
    indirmeden denetlendi (hazırlayan kişi, burada bir dosyada kayıtlı değil):
    arşivler bayt bayt aynı, 66.240.132 bayt, SHA-256
    `3e243da119a252988df80745d77f3e500b6856c60717fa4b2b8f94e1e54e4795`; manifest
    sıra 92 (`published_at` 2026-10-16T18:09:41Z), imza doğrulanır (aynı
    anahtar); 241 üye; `get.sh` 92'yi sabitler; politika 92, önceki 91;
    `release.commit` ve `release.tree` etiketinkilerle eşit; arşivin
    `deploy/panel-tls-snapshot.sh` dosyasında `_panel_tls_wait_service_idle` ve
    `_panel_tls_note_owner_failed_renewal`, `update.sh` dosyasında
    `update_finishing_scheduler` var; arayüz `app-DomwK8kh.js` (248.892 bayt).
    Portal paketi 132.855.924 bayt, SHA-256
    `809e4f501a020ee54a847c580406f8698d1eb0809aa287be27316f00dd72ffcd`, 54 üye,
    iki kez aynı derlendi, terfi aracı kabul etti, testler ok; hazırlayan kişi
    `-PreviousVersion v0.1.0-alpha.90` ile yayımladı (canlı portal alpha.91'i
    hiç sunmadı): committed, yedek
    `httpdocs.before-v0.1.0-alpha.92.20261016T185901Z…`, doğrulama ok, 26 istek;
    18:59:09Z'de dışarıdan okundu (hepsi 92). **Kurulu paneller, sahibin
    bildirdiği (iki ekran görüntüsü, yaklaşık 19:10Z, alpha.90'dan
    güncelleme):** ikisi de geçerli sürüm v0.1.0-alpha.92'yi, commit
    `961f2cb2fcae…`'yi, kenar çubuğunda build `961f2cb2fcae`'yi ve beş
    tamamlanmış adımlı "v0.1.0-alpha.92 sürümüne güncellendi" kartını gösterir,
    Frankfurt da: bu kez hata yok, kurtarma yok (her birinde bir güncelleme;
    Certbot düzeltmesinin kabulü değil). Güncelleme sırasında sayfanın gösterdiği
    (alpha.90'ın sayfasının çizdiği canlı adım görünümü) bildirilmedi.
    Bildirilmeyen: Frankfurt'un Alan adları'ndaki "Altyapı bölgesi" satırı,
    Boston'un eş sunucu kaydıyla posta genişletmesi, lisans sayfası, Agent'ın
    sürümü, `postconf -n`. Yayımlanma kabul değildir; hiçbir P0 işi kapanmaz ya
    da ilerlemez; P0.1–P0.5 **kısmi** kalır. Ayrıntı [sürüm
    notlarının](docs/RELEASE-NOTES-v0.1.0-alpha.92.tr.md) "Yayımlama
    kaydı"nda.
- **v0.1.0-alpha.93 yayımlandı, 16 Ekim 2026; bu yazılırken kurulu sunucular ona güncellenmemişti.** Aday olarak yazıldı ve aynı gün değiştirildi. Sahibin alpha.92 çalıştıran iki kurulu sunucudan aldığı ekran görüntülerinden (16 Ekim 2026) altı küçük düzeltme; her birine sahip aynı gün karar verdi (D-024, D-032, D-033): Temel Posta, onun bütün bileşenlerini içeren doğrulanmış daha büyük bir profil (Webmail, Spam Korumalı Posta) aynı taramada aynı uzlaştırmayı kanıtlıyorsa doğrulanmış sayılır; böylece Frankfurt'ta Temel Posta artık Onar istemez (yalnız bileşen durumu yine kanıt değildir; süren ya da başarısız kendi denemesi durumunu korur); "Web postayı onar" Yönet gibi çizilir ve yalnız sunucu bir webmail sorunu bildirdiğinde sarıdır; Pano "Posta bu sunucuda kurulu ve doğrulanmış." der; yalnız web sunucusu parçaları olan posta profili "Kısmen kurulu" değil "Kurulu değil"dir (Boston); Son etkinlik kurulumun hazırladığı posta profilini adıyla anar; sayılar doğru söylenir ("1 öğe"; İngilizcede "1 item"). Numarayı, sahibin 13 Ekim 2026 tarihli kalıcı talimatıyla sürümü hazırlayan seçti. 92'den sonra sıra 93, şema 44, geçiş yok, sahibin 16 Ekim 2026 talimatıyla gerçek sistem kaydı yok; kurulu bir sunucuda hiçbir şey ölçülmedi; P0.1–P0.5 **kısmi** kalır ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.93.tr.md)).
  - *Yayım, 16 Ekim 2026 (UTC).* Sürümü hazırlayan kişi tarafından, sahibin kalıcı
    talimatı uyarınca; kurulu panel güncellemeleri sahibindir. #224 numaralı
    çekme isteği `main`'e `0897ee9238bdfee295149adfe8b16968dd6fe0de` olarak
    birleştirildi. Sürüm zincirinin ilk koşusu yanlış commit'i etiketledi
    (çekme isteğinde çakışma olduğu için birleştirme reddedildi, betik yine de
    devam etti); CI'ın politika koruması onu reddetti ("tag does not match policy
    version"), hiçbir sürüm oluşmadı, etiket silindi ve zincir betiği artık
    çekme isteği birleştirilmemişse, `main` onun birleştirme commit'ine eşit
    değilse ya da `main`'in politika sürümü etiketle aynı değilse durur. İkinci
    koşu `v0.1.0-alpha.93` açıklamalı etiketini (nesne
    `c64b60021c3fc6b6bb506e99bdaf6b1354f531cf`, ağaç
    `7b18e5252045eef74264a946d41efcbbc927b545`) ve 2026-10-16T22:04:09Z'de altı
    dosyayla yayımlanan bir ön sürümü üretti. `CelikPanel-S2-Debian` WSL
    konuğunda bir indirmeden denetlendi (hazırlayan kişi, burada bir dosyada
    kayıtlı değil): arşivler bayt bayt aynı, 66.240.206 bayt, SHA-256
    `887fbe30a29c6fa364bfafc44ec676414c32e41758417738ff4cecc287f9fc39`; manifest
    sıra 93 (`published_at` 2026-10-16T21:21:23Z), imzası `openssl pkeyutl
    -verify -rawin -pubin` ile doğrulanır (anahtar `7eadeb0b…`); iki içerik
    koruması 0; Go go1.26.5, CGO kapalı, trimpath; `get.sh` 93'ü sabitler;
    politika 92'den sonra 93 (`961f2cb2…`); arayüz `app-BskYbaLr.js` (248.904
    bayt). Portal paketi 132.853.434 bayt, SHA-256
    `b27504acc26d7c291d7e0ec3ebfb051ac04996b27af65e78e99c3a0ea934f83c`, 54 girdi,
    iki kez aynı derlendi; sözleşme testi 0, genel doğrulayıcı 17 test, terfi 14
    test; hazırlayan kişi önceki v0.1.0-alpha.92 ile yayımladı: committed, yedek
    `httpdocs.before-v0.1.0-alpha.93.20261016T221039Z…`, doğrulama 26 istekten
    26'sı, `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`; yayımdan sonra dışarıdan
    okundu (`latest.txt` v0.1.0-alpha.93, `latest.json` sıra 93). **Kurulu
    paneller:** Boston ve Frankfurt bu yazılırken alpha.92'deydi; sahip henüz
    ikisini de güncellememişti, bu yüzden kurulu bir sunucudan hiçbir şey
    kaydedilmedi. Yayımlanma kabul değildir; hiçbir P0 işi kapanmaz ya da
    ilerlemez; P0.1–P0.5 **kısmi** kalır. Ayrıntı için [sürüm
    notları](docs/RELEASE-NOTES-v0.1.0-alpha.93.tr.md), "Yayım kaydı".
- **v0.1.0-alpha.94 yayımlandı, 17 Ekim 2026; bu yazılırken kurulu sunucular ona güncellenmemişti.** Aday olarak yazıldı ve aynı gün değiştirildi. Hazır paketler (sahibin 17 Ekim 2026 kararı, D-034): katalogdaki bileşenlerin üzerinde kısayollar, mağaza ürünü değil. Bileşenler'in en üstündeki üç posta paketi kartı yerini her kategorinin başındaki tek bir "Hazır paketler" satırına bırakır (posta mevcut posta profilleri üzerinden; eklenti olarak phpPgAdmin ve phpMyAdmin'li bir PostgreSQL paketi ve bir MariaDB paketi; Nginx + PHP-FPM); Panel bunları aynı taramadan ve posta profillerinden türetir; bir veritabanı ya da web paketi eksik bileşenlerini olağan bileşen kurulumuyla birer birer kurar, ilk hatada o bileşenin yönlendirmesiyle durur ve yalnız sahibin Devam et'i ile sürer. Panonun Posta paketleri kartı yerini tek bir sakin Hazır paketler kartına bırakır. 16 Ekim 2026 tasarım incelemesinin Bileşenler renk düzeltmeleri mevcut DESIGN.md kuralları olarak uygulanır (dinlenirken nötr satır simgeleri, kırmızı durmuş hizmetli dört durum noktası tonu, çerçeveli satır Kur düğmesi, tek satırlık sağlıklı güvenlik duvarı, tek haritadan çizgi simgeleri, sabit sütunlar, telefonda 44 piksellik satır eylemleri). Numarayı, sahibin 13 Ekim 2026 tarihli kalıcı talimatıyla sürümü hazırlayan seçti. 93'ten sonra sıra 94, şema 44, geçiş yok, yeni Agent iş türü yok, sahibin 16 Ekim 2026 talimatıyla gerçek sistem kaydı yok; kurulu bir sunucuda hiçbir şey ölçülmedi; P0.1–P0.5 **kısmi** kalır ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.94.tr.md)).
  - *Yayım, 17 Ekim 2026 (UTC).* Sürümü hazırlayan kişi tarafından, sahibin kalıcı
    talimatı uyarınca; kurulu panel güncellemeleri sahibindir. #226 numaralı
    çekme isteği `main`'e `d506a05e8c003dffee8de2e1268402865520bd6b` olarak
    birleştirildi. Sürüm sırası değişkeni etiket yapılmadan önce 93'ten 94'e
    geçirildi; `main` koşusu ve etiket koşusu başarılı oldu ve sürüm zinciri
    temiz çalıştı (olay yok). `v0.1.0-alpha.94` açıklamalı etiketi (nesne
    `d17aa39f13568a6254eeb5f1ab271eccff5eb5be`, ağaç
    `7c1fd69a4dd1c43953037a344f7850dc150922b6`) sürüm commit'ini gösterir; ön
    sürüm 2026-10-17T00:30:13Z'de altı dosyayla yayımlandı.
    `CelikPanel-S2-Debian` WSL konuğunda 17 Ekim 2026'da bir indirmeden
    denetlendi (hazırlayan kişi, burada bir dosyada kayıtlı değil): arşivler bayt
    bayt aynı, 66.259.323 bayt, SHA-256
    `bec3c015317fecc226d0a282ecf7e30547e305a621e7fa70943947c6ef0a6788`; manifest
    sıra 94 (`published_at` 2026-10-16T23:46:25Z), imzası `openssl pkeyutl
    -verify -rawin -pubin` ile doğrulanır (anahtar `7eadeb0b…`); iki içerik
    koruması 0; Go go1.26.5, CGO kapalı, trimpath; `get.sh` 94'ü sabitler;
    politika 93'ten sonra 94 (`0897ee92…`); arayüz `app-C9mMpV6A.js` (249.028
    bayt). Portal paketi 132.891.376 bayt, SHA-256
    `b493b72b09e49301fdce059a52ad2d0b92f892dd0e82c1ce1f1550c4b6dcf215`, 54 girdi,
    iki kez aynı derlendi; sözleşme testi 0, genel doğrulayıcı 17 test, terfi 14
    test; hazırlayan kişi yaklaşık 00:48Z'de önceki v0.1.0-alpha.93 ile
    yayımladı: committed, yedek
    `httpdocs.before-v0.1.0-alpha.94.20261017T004816Z…`, doğrulama 26 istekten
    26'sı, `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`; yayımdan sonra dışarıdan
    okundu (`latest.txt` v0.1.0-alpha.94, `latest.json` sıra 94). **Kurulu
    paneller:** Boston ve Frankfurt bu yazılırken alpha.92'deydi; sahip henüz
    ikisini de güncellememişti ve alpha.93'e güncellediğini de bildirmemişti; bu
    yüzden alpha.93 ya da alpha.94 için kurulu bir sunucudan hiçbir şey
    kaydedilmedi. İncelemede bulunan üç küçük iş sonraki bir sürüme açık
    bırakıldı (sürüm notları, "Yapılmayanlar"). Yayımlanma kabul değildir;
    hiçbir P0 işi kapanmaz ya da ilerlemez; P0.1–P0.5 **kısmi** kalır. Ayrıntı için
    [sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.94.tr.md), "Yayım kaydı".
- **v0.1.0-alpha.95 yayımlandı, 17 Ekim 2026; hiçbir kurulu sunucu ona güncellenmedi.** Aday olarak yazıldı ve aynı gün değiştirildi. `fix/alpha95-bundle-consistency` dalında, `main`'in alpha.94 sürüm işlemesinden (`d506a05e8c003dffee8de2e1268402865520bd6b`) yazıldı. Sahip, Boston ve Frankfurt'ta (alpha.94) Bileşenler ve Pano'ya baktı ve "ok. sorunları tekrar değerlendir ve düzelt" dedi (sahibin 17 Ekim 2026 kararı, D-034 değişikliği). Her Bileşenler kategorisi artık her sunucuda aynı hazır paketleri aynı sırayla, her çipte durumuyla gösterir; bu, alpha.94'ün "kurulu satır ve en çok bir öneri daha" kuralını (sürümü hazırlayanın hatası) geri çeker; o kural Boston'da MariaDB paketini, Frankfurt'ta Temel Posta'yı gizliyor ve Pano sayısını listeden farklı yapıyordu. Kart ve çipler için tek sayma kuralı (eklentiler dahil, engelli hariç, "içinde" kurulu sayılır). Kurulu çipler Onar (posta) ya da Doğrula (bileşenler) penceresi açar. alpha.94'ün üç inceleme maddesi kapandı (adım kapısının doğrulanmamış bayrağı, bir bileşen işlemi sürerken tutulan Doğrula, istek kabul edilene kadar "X kurulmadan önce sunucu denetleniyor…"). Satır eylemleri tek çerçeveli simge düğmesini ve dur işaretli Durdur simgesini paylaşır; çizgilerden oluşan masaüstü sürüm sütunu gitti, çünkü Panel sürümleri yalnız PHP-FPM ve Node.js için okur (yeni Agent RPC'si yok; **açık iş: her bileşen için kurulu paket sürümünü okumak**); Roundcube kalıntıları soluk bir not ve Agent'ın yollarını sıralayan bir "Kalıntıları temizle…" onayı alır; bu, satırda yalnız Panel'in kendi denetim kaydı yarım kalmış bir Roundcube kurulumu ya da kaldırması gösterdiğinde sunulur (Panel diski hâlâ göremez), Roundcube'un kendi sayfasında ise her zaman; kenar çubuğu sayıları sade sayılardır; Pano'nun Alan adları kutusu ve kenar çubuğundaki Alan adları rozeti, alan adları artı Alan adları sayfasının listelediği altyapı DNS bölgelerini tek bir yardımcıyla sayar, böylece kutu, rozet ve sayfa uyuşur. Adayın bağımsız incelemesi on sorun buldu, adayda düzeltildi (D-034 değişikliği): yalnız Panel kaydıyla kalıntılar, paketi betimleyen kurulu paket penceresi, çiplerin altında dürüst satırlar, "içinde" yalnız kendisi doğrulanmış ve başarısız ya da süren denemesi olmayan paket için, tek Alan adları sayısı, doğrulanmamış ve denemesi olan paket başına tek Pano satırı, nedenini gösteren sessiz düğme olarak engelli çipler, meşgul ana makine kilidi ya da paket yöneticisinde kullanılabilir Doğrula, EN ifadesi "2/3 · installing PHP-FPM" ve "n of m installed", ve "Kur" diyen posta çipi için kurulum biçimi. O ağaçtaki kanıt: web `npm test` 1599 geçti, `npm run build` başarılı; Go `./cmd/panel/` ve `./internal/...` 7124 geçti, 0 başarısız, 44 atlandı; tarayıcı 20. grubu inceleme düzeltmelerinden sonra yeniden çalıştırılmadı. 94'ten sonra sıra 95, şema 44, geçiş yok, Agent değişikliği yok, yeni Agent iş türü yok, güncelleyici değişmedi; bu yüzden gerçek sistem kaydı yok; kurulu bir sunucuda hiçbir şey ölçülmedi; P0.1–P0.5 **kısmi** kalır ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.95.tr.md)).
  - *Yayım, 17 Ekim 2026 (UTC).* Sürümü hazırlayan kişi tarafından, sahibin kalıcı
    talimatı uyarınca; kurulu panel güncellemeleri sahibindir. #228 numaralı
    çekme isteği `main`'e `5f3cb1a2fa336c529740a20e9a543b8ac086dc8d` olarak
    birleştirildi. Sürüm sırası değişkeni etiket yapılmadan önce 94'ten 95'e
    geçirildi; `main` koşusu ve etiket koşusu başarılı oldu ve sürüm zinciri
    temiz çalıştı (olay yok). Sürümden önce iki bağımsız inceleme yapıldı:
    birincisi on sorun buldu (hepsi düzeltildi), ikincisi sürümü yayıma hazır
    buldu; sürümü hazırlayan kişi ayrıca yeni Roundcube kaydı okumasını denetim
    kaydının en yeni 20.000 girdisiyle sınırladı. `v0.1.0-alpha.95` açıklamalı
    etiketi (nesne `44764f09aa74546bb5b5704fabde8bf1930fc803`, ağaç
    `0e9d3c486cae7250c4d4c4d525d50fd37c56920a`) sürüm commit'ini gösterir; ön
    sürüm 2026-10-17T11:01:22Z'de altı dosyayla yayımlandı.
    `CelikPanel-S2-Debian` WSL konuğunda 17 Ekim 2026'da bir indirmeden
    denetlendi (hazırlayan kişi, burada bir dosyada kayıtlı değil): arşivler bayt
    bayt aynı, 66.259.462 bayt, SHA-256
    `9d9116f3199f453c508a605403cf4e6c63c099145dbef0b98cc402abc0852ef5`; manifest
    sıra 95 (`published_at` 2026-10-17T10:22:04Z), imzası `openssl pkeyutl
    -verify -rawin -pubin` ile doğrulanır (anahtar `7eadeb0b…`); iki içerik
    koruması 0; Go go1.26.5, CGO kapalı, trimpath; `get.sh` 95'i sabitler;
    politika 94'ten sonra 95 (`d506a05e…`); arayüz `app-DU840jYm.js` (250.651
    bayt). Portal paketi 132.890.919 bayt, SHA-256
    `4f11dc01376626e6db9160f4716b65fa044b4084bd9b24991bced6b5a9985ca1`, 54 girdi,
    iki kez aynı derlendi; sözleşme testi 0, genel doğrulayıcı 17 test, terfi 14
    test; hazırlayan kişi yaklaşık 11:09Z'de önceki v0.1.0-alpha.94 ile
    yayımladı: committed, yedek
    `httpdocs.before-v0.1.0-alpha.95.20261017T110941Z…`, doğrulama 26 istekten
    26'sı, `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`; yayımdan sonra dışarıdan
    okundu (`latest.txt` v0.1.0-alpha.95, `latest.json` sıra 95). **Kurulu
    paneller:** sahip 17 Ekim 2026'da Boston ve Frankfurt'u alpha.94'e panelin
    kendi güncelleme ekranından güncelledi ve Pano ile Bileşenler ekran
    görüntülerini gönderdi (ikisi de yapı `d506a05e8c00`); alpha.95'in
    düzelttiği şey bu ekran görüntüleridir. Boston'un Bileşenler sayfasının bir
    ekran görüntüsü, başlıkları tekrarlanan, birleştirilmiş tam sayfa bir
    yakalamaydı; normal ekran görüntülerinde sayfa sağlamdı, bu yüzden kusur
    olarak kaydedilmedi. Kurulu bir sunucuda alpha.95'ten hiçbir şey kaydedilmedi.
    Küçük işler sonraki bir sürüme açık bırakıldı (sürüm notları,
    "Yapılmayanlar"). Yayımlanma kabul değildir; hiçbir P0 işi kapanmaz ya da
    ilerlemez; P0.1–P0.5 **kısmi** kalır. Ayrıntı için [sürüm
    notları](docs/RELEASE-NOTES-v0.1.0-alpha.95.tr.md), "Yayım kaydı".
- **v0.1.0-alpha.96 yayımlandı, 17 Ekim 2026; hiçbir kurulu sunucu ona güncellenmedi.** Aday olarak yazıldı ve aynı gün değiştirildi. 17 Ekim 2026'da `feat/alpha96-update-notice` dalında, alpha.95'in yayım kaydından sonraki `main`'den yazıldı (alpha.95 sürüm işlemesi `5f3cb1a2fa336c529740a20e9a543b8ac086dc8d`). Sahip alpha.95'e güncellerken kendi sözleriyle şunu istedi: "belirli periodlarda güncellemeleri kontrol etmeli ve güncelleme var diye uyarmalı. güncelleme konusunda yardımcı olmalı." (düzenli aralıklarla güncellemeleri denetlemeli ve güncelleme var diye uyarmalı; güncellemede yardımcı olmalı; D-035). Panel artık kendiliğinden bakar: başladıktan yaklaşık 2 dakika sonra ve ardından her 12 saatte bir salt-okur sürüm denetimi; 40 saniyeyle sınırlı; başarısızlıktan sonra 1 saat, 3 saat ve sonra yine 12 saatlik dönem; son bir saat içinde bir güncelleme başlatma kabul edildiyse, sunucu süren bir değişiklik bildiriyorsa, sunucu kurulumu sunucuyu değiştiriyorsa ya da Agent yanıt vermiyorsa atlanır (başarısızlık sayılmaz, yalnız bellekte tutulur); sonuç Panel'in ayarlar tablosunda saklanır (şema değişikliği yok), ama bir güncelleme kabul edilmiş ya da sürüyorken asla (bir sonraki boşta geçişe kadar bellekte tutulur) ve yalnız yöneticilere gösterilir: kenar çubuğunda küçük bir bağlantı, Pano'nun en üstünde "Sonra hatırlat" (o tarayıcıda 24 saat) olan tek sessiz bir satır ve Ayarlar gezinmesinde bir nokta; asla alarm rengiyle değil ve başarısız bir denetim için güncelleme kartının dışında asla. Sahip zamanlamayı Ayarlar → CelikPanel güncellemeleri içinde kapatabilir (varsayılan açık). Güncelleme kartı kayıtlı sonucu bilerek açılır; tek durum satırı, önceki bir deneme için en çok tek bir bildirim, sade bir "Güncellemeden önce" listesi ve tek dolu "… sürümüne güncelle" düğmesi vardır; sahibin Frankfurt ekran görüntüsünde görülen yinelenen satırlar ve ikinci sarı kutu gitti. Hiçbir şey kendiliğinden güncellenmez: güncellemeyi yalnız bir yönetici panelin kendi güncelleme ekranından başlatır. API: okuma `GET /api/v1/panel/update/discovery` (ayrı bir yol, çünkü `/update/status` zaten tek bir güncelleme isteğinin durumunu yanıtlıyor); anahtar için `PUT`. Sözü olmayan iki kurtarma aracı adımının (`mode`, `owner`) artık İngilizce ve Türkçe metni var, ve genel bir neden altında sunucunun ilk tanı satırını gösterir. Sahibin, Frankfurt'un alpha.95 güncellemesinin salt-okur denetiminde durduğu gözlemi, bilinenlerle kaydedildi (sunucu alpha.94'te kaldı; gösterilen neden genel olandı; neden **bilinmiyor** ve tahmin edilmedi; Ayrıntılar metni sahipten bekleniyor). alpha.95'in dört açık işi kapandı (sunucu herhangi bir nedenle hazır değilken Doğrula tutulur; Roundcube temizleme kaydı, Panel'in denetim girdisi ile posta profilinin son denemesinden yenisini alır; onay penceresi açıldıktan sonra `?cleanup=roundcube` adresten kaldırılır; telefonlarda pencere alt düğmeleri en az 44 px), ve tarayıcı 21. grubu, alpha.95'in incelemesinden sonra yeniden çalıştırılmayan 20. grubun yerini alır (sonuç sürüm notlarında). 95'ten sonra sıra 96, şema 44, geçiş yok, Agent değişikliği yok, yeni Agent iş türü yok, güncelleyicinin değiştirme yolu değişmedi, bu yüzden gerçek sistem kaydı yok (sahibin 16 Ekim 2026 talimatı). Kurulu bir sunucuda hiçbir şey ölçülmedi; zamanlanmış denetim gerçek bir Agent'a ya da gerçek sürüm sunucusuna karşı çalışmadı; **açık işler: her bileşen için kurulu paket sürümlerini okumak; Frankfurt'un alpha.95 salt-okur denetim durmasının nedeni**. P0.1–P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.96.tr.md)).
  - *Yayım, 17 Ekim 2026 (UTC).* Sürümü hazırlayan kişi tarafından, sahibin kalıcı
    talimatı uyarınca; kurulu panel güncellemeleri sahibindir. #230 numaralı
    çekme isteği `main`'e `08faa91e5a8c6622072b40f0f87c9c388035c9a3` olarak
    birleştirildi. Sürüm sırası değişkeni etiket yapılmadan önce 95'ten 96'ya
    geçirildi; `main` koşusu ve etiket koşusu başarılı oldu. Sürümden önce iki
    bağımsız inceleme yapıldı: birincisi 12 sorun buldu (aralarında, zamanlanmış
    denetimin bir güncellemenin salt-okur denetimi sırasında panel veritabanına
    yazması; düzeltildi), ikincisi sürümü yayıma hazır buldu ve önerdiği iki
    temizlik yapıldı. Sürüm zincirinde, hiçbiri bir sürümü etkilemeyen iki olay
    oldu: ilk CI koşusu iki sürüm-kurtarma deneme düzeneği testinde başarısız oldu
    (düzenek güncelleme kartının Türkçe metnini ve sonuç mantığını yansıtır; yeni
    "denetle" ifadesi ve değişen bir ifade orada yansıtılmamıştı; dalda
    düzeltildi); ardından bir yardımcının deneme düzeneği çalışması, ortak
    deponun git yapılandırmasında başıboş bir çalışma-ağacı ayarı bıraktı; bu,
    sürümü hazırlayanın makinesinde git'i kullanılmaz kıldı ve zinciri zaman
    aşımına uğrattı (o tek satır kaldırıldı ve zincir yeniden çalıştırıldı).
    `v0.1.0-alpha.96` açıklamalı etiketi (nesne
    `61eb922045079c0828e4872ad90d1b76d39007c2`, ağaç
    `aa73450d100c10e2a8fccdc45862e8da0deb5a9e`) sürüm commit'ini gösterir; ön
    sürüm 2026-10-17T18:03:34Z'de altı dosyayla yayımlandı.
    `CelikPanel-S2-Debian` WSL konuğunda 17 Ekim 2026'da bir indirmeden
    denetlendi (hazırlayan kişi, burada bir dosyada kayıtlı değil): arşivler bayt
    bayt aynı, 66.291.958 bayt, SHA-256
    `eeb14706f7dc57f1824ff2f22f3794d0e65adbb7d456000e978f6b3ebc5fac02`; manifest
    sıra 96 (`published_at` 2026-10-17T17:22:53Z), imzası `openssl pkeyutl
    -verify -rawin -pubin` ile doğrulanır (anahtar `7eadeb0b…`); iki içerik
    koruması 0; Go go1.26.5, CGO kapalı, trimpath; `get.sh` 96'yı sabitler;
    politika 95'ten sonra 96 (`5f3cb1a2…`); arayüz `app-BNx0yp-r.js` (256.533
    bayt). Portal paketi 132.959.242 bayt, SHA-256
    `890ef8f1c90b960b9935b5207ef4d5ec624d368b800b525e372fc1b80868b730`, 54 girdi,
    iki kez aynı derlendi; sözleşme testi 0, genel doğrulayıcı 17 test, terfi 14
    test; hazırlayan kişi yaklaşık 18:10Z'de önceki v0.1.0-alpha.95 ile
    yayımladı: committed, yedek
    `httpdocs.before-v0.1.0-alpha.96.20261017T181008Z…`, doğrulama 26 istekten
    26'sı, `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`; yayımdan sonra dışarıdan
    okundu (`latest.txt` v0.1.0-alpha.96, `latest.json` sıra 96). **Kurulu
    paneller:** sahip 17 Ekim 2026'da Boston ve Frankfurt'un alpha.95'e
    güncellemesini panelin kendi güncelleme ekranından başlattı. Frankfurt'un
    güncellemesi bir şeyi değiştirmeden önce salt-okur denetimde durdu (salt-okur
    bir denetim mevcut kurulumu doğrulayamadı); sunucu alpha.94'te kaldı ve
    sahipten Ayrıntılar metni istendi; bu yazılırken gelmemişti, neden
    **bilinmiyor**. Boston'un sonucu bildirilmemişti. Sahip düzenli güncelleme
    denetimi, bir güncelleme bildirimi ve güncellemede yardım istedi; alpha.96 bu
    işi yapar. Kurulu bir sunucuda alpha.96'dan hiçbir şey kaydedilmedi ve
    alpha.96, hâlâ alpha.95'te olan bir panele kendini duyuramaz (zamanlanmış
    denetim yalnız alpha.96'dan itibaren vardır). Yayımdan sonra açık:
    Frankfurt'un durmasının nedeni; Türkçe ekran katalogları neredeyse dolu ve
    daha fazla metin eklenmeden bölünmelidir; sunucu meşgulken bellekte tutulan
    elle denetim sonucu yeniden başlatmada kaybolur; kurulu bir sunucuda ya da
    gerçek sürüm sunucusuna karşı hiçbir şey ölçülmedi; tarayıcı 21. grubu son
    inceleme düzeltmelerinden sonra yeniden çalıştırılmadı; her bileşen için
    kurulu paket sürümlerinin okunması. Yayımlanma kabul değildir; hiçbir P0 işi
    kapanmaz ya da ilerlemez; P0.1–P0.5 **kısmi** kalır. Ayrıntı için [sürüm
    notları](docs/RELEASE-NOTES-v0.1.0-alpha.96.tr.md), "Yayım kaydı".
- **v0.1.0-alpha.97 yayımlandı, 21 Ekim 2026; sürümleri henüz bir kurulu sunucuda görülmedi.** Aday olarak yazıldı ve aynı gün değiştirildi. alpha.96'nın yayım kaydından sonra `main` üzerinden `feat/alpha97-component-versions` dalında yazıldı (alpha.96 sürüm kaydı `08faa91e5a8c6622072b40f0f87c9c388035c9a3`). Dört şey. (1) **Her bileşen için kurulu paket sürümleri** (D-036; D-034 değişikliğinin açık işi): yeni bir salt-okur Agent yeteneği, `Agent.InstalledPackageVersionsV1`, bileşen taraması başına bir kez sistem paket veritabanına sorar (Debian/Ubuntu'da `dpkg-query -W`, Arch'ta `pacman -Q`; sabit argv, 10 saniyelik zaman aşımı, çıktı sınırı, ağ yok, kilit alınmaz) ve bir paket işlemi sürerken asla çalışmaz (önce süreç ve kilit tablolarını okur ve meşgul yanıtı verir; Panel o zaman önceki sürümleri, korunmuş olarak işaretleyerek tutar). Panel onu bir okuma olarak kabul eder, mevcut tarama önbelleğiyle birlikte eklemeli bir alan olarak saklar (eski satırlar bilinmeyen olarak okunur; şema değişikliği yok) ve Bileşenler satırlarında yukarı akış kısa biçimini (yalnız en az bir sürüm biliniyorsa masaüstü sütunu; telefonda açıklamanın altında), her bileşen sayfasının başlığında kaynağıyla tam sürümü gösterir; paketleri farklı olanlar "birden çok sürüm" der; bilinmeyen bir şey söylemez. (2) **Ekran kataloğu bölündü**: her dilde dokuz parça (setup, sites, siteConfig, dnsCertificates, components, mailDatabases, security, dashboard, settings), her birinin en az %25 pay bırakılarak belirlenmiş kendi paket bütçesi var; anahtarlar, `t()` ve TypeScript eşlik tipleri değişmedi ve bir çalışma zamanı testi her parçanın her anahtarını iki dilde okur. (3) **Frankfurt'un alpha.95 güncellemesinin neden durduğu, doğrulandı:** sahibin salt-okur komutları saklanan `code=package_manager_busy state=unchanged` satırını döndürdü (Debian'ın otomatik paket güncellemeleri çalışıyordu, 2026-10-17T12:25:23Z); hiçbir şey değiştirilmedi. Sayfa, nedenin ona nasıl ulaştığındaki dört sebeple yalnız genel nedeni söyledi (OPERATION-GUIDANCE, "Frankfurt'taki alpha.95 duruşu neden gerekçesiz göründü"); aday durdurma penceresinde ve güncelleme kartının bildiriminde (hazır olma yeniden okuması paket yöneticisinin ne zaman boşaldığını söyler) meşgul bir paket yöneticisini sakin bir dille adlandırır, bir salt-okur denetim durmasıyla sınırlı, temizlenmiş bir tanı satırı tutar, `compatibility` adımına sözler verir ve bir adımın sözleri yüklenirken genel nedeni asla göstermez. (4) Tarayıcı 19. ve 20. grupları sürümleri yalnız yeni alanların bildireceği biçimde bildirir (örnek Debian 13 ve Ubuntu 24.04 metinleri) ve 22. grup sürümleri çizer. Bir Agent değişikliği (salt-okur yetenek, RPC politikası), ama güncelleyicinin değiştirme yolu değişmedi, bu yüzden gerçek sistem kaydı yok (sahibin 16 Ekim 2026 talimatı). Bağımsız bir incelemeden sonra aynı gün düzeltildi: dpkg'nin her kurulu durumu sayılır, sorgu çıktı tavanında öldürülür ve sabit bir ortamla çalışır, kilit tablosu okuması sınırlıdır, bayat bir pacman kilidi artık sonsuza dek meşgul demek değildir, bir günden uzun korunan okuma kısa bir satır ekler, sayfanın taraması sürümleri değişiklik kirası dışında okur ve bir durma nedeni sözlerini en çok 10 saniye bekler (yeni nedenler artık ekran metnidir). Sıra 96'dan sonra 97, şema 44, geçiş yok. Kurulu bir sunucuda hiçbir şey ölçülmedi. P0.1–P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.97.tr.md)).
  - *Yayım, 21 Ekim 2026 (UTC).* Sürümü hazırlayan kişi tarafından, sahibin
    kalıcı talimatı uyarınca; kurulu panel güncellemeleri sahibindir. #232
    numaralı çekme isteği `main`'e `0c1a8917ef3f8141702fa41ae085f64d62a713f0`
    olarak birleştirildi. Sürüm sırası değişkeni etiket yapılmadan önce 96'dan
    97'ye geçirildi; `main` koşusu ve etiket koşusu başarılı oldu ve sürüm
    zinciri sorunsuz çalıştı. Sürümden önce üç bağımsız inceleme turu yapıldı;
    bulguları düzeltildi (tutulan dpkg durumları, çıktı sınırının etiketi ve
    gerçek süreç testleri, sonsuza dek bekleyebilen bir neden satırı,
    temizleyicinin yol işlemesindeki boşluklar ve alpha.97'den alpha.96'ya
    Mağaza çözücüsü notu). `v0.1.0-alpha.97` açıklamalı etiketi (nesne
    `e5c64bcc3569513050ebfc89c01185a69cabbfb0`, ağaç
    `67db021959466f61a2defc232194d897c1eb9cb7`) sürüm commit'ini gösterir; ön
    sürüm 2026-10-21T20:24:20Z'de altı dosyayla yayımlandı.
    `CelikPanel-S2-Debian` WSL konuğunda 21 Ekim 2026'da bir indirmeden
    denetlendi (hazırlayan kişi, burada bir dosyada kayıtlı değil): arşivler
    bayt bayt aynı, 66.312.385 bayt, SHA-256
    `3784fa392b016a9fbd18ffc372f7017c8c2aab5d3643bf319c25e53514800aa3`; manifest
    sıra 97 (`published_at` 2026-10-21T19:40:33Z), imzası `openssl pkeyutl
    -verify -rawin -pubin` ile doğrulanır (anahtar `7eadeb0b…`); iki içerik
    koruması 0; Go go1.26.5, CGO kapalı, trimpath; 257 üye; `get.sh` 97'yi
    sabitler; politika 96'dan sonra 97 (`08faa91e…`); arayüz `app-Ckmq5jcD.js`
    (256.924 bayt); `bin/agent` içinde `InstalledPackageVersionsV1` var. Portal
    paketi 132.999.066 bayt, SHA-256
    `61ade15eee9ecd7e136d0eee599534a96c628d6bb38ae2365c0c74f56e339197`, 54
    girdi, iki kez aynı derlendi; sözleşme testi 0, genel doğrulayıcı 17 test,
    terfi 14 test; hazırlayan kişi yaklaşık 20:36Z'de önceki v0.1.0-alpha.96 ile
    yayımladı: committed, yedek
    `httpdocs.before-v0.1.0-alpha.97.20261021T203644Z…`, doğrulama 26 istekten
    26'sı, `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`; yayımdan sonra dışarıdan
    okundu (`latest.txt` v0.1.0-alpha.97, `latest.json` sıra 97). **Kurulu
    paneller ve sahibin gözlemleri:** sahip 21 Ekim 2026'da, Boston'un
    v0.1.0-alpha.96'da (yapı 08faa91e5a8c) olduğunu gösteren bir Ayarlar ekran
    görüntüsü gönderdi. Sahip, Frankfurt'un alpha.95 durması için 21 Ekim
    2026'da kendisine verilen iki salt-okur komutu çalıştırdı; saklanan satır
    `!! CELIKPANEL_UPDATE_FAILURE code=package_manager_busy state=unchanged
    reason=the host package manager is active detail=` idi
    (2026-10-17T12:25:23Z): Debian'ın otomatik paket güncellemeleri çalışıyordu,
    sunucuda hiçbir şey değişmedi ve alpha.97 ekranın bunu söylemesini sağlar.
    Frankfurt'un bundan sonraki güncellemesi bu yazılırken bildirilmemişti.
    Sahip ayrıca GitHub hesabının adını değiştirdi (celikbros → alicelik77);
    depo artık `alicelik77/celikpanel`, eski `celikbros/celikpanel` adı kalıcı
    olarak yönlendirilir. alpha.97'nin okuduğu sürümler henüz bir kurulu
    sunucuda görülmedi. Yayımdan sonra açık: Boston ve Frankfurt'taki sürümler
    (açık iş ancak sahip bunları gördüğünde kapanır); kritik açılış yolunda 1,8
    KiB bütçe kaldı; genel portal hâlâ `celikbros/...` GitHub adreslerine
    bağlanıyor (yönlendirilirler; portal bir sonraki değiştiğinde
    güncellenecek); Ayarlar için menü düzeni önerisi sahibin kararını bekliyor.
    Yayımlanma kabul değildir; hiçbir P0 işi kapanmaz ya da ilerlemez; P0.1–P0.5
    **kısmi** kalır. Ayrıntı için [sürüm
    notları](docs/RELEASE-NOTES-v0.1.0-alpha.97.tr.md), "Yayım kaydı".
- **v0.1.0-alpha.98 yayımlandı, 22 Ekim 2026; henüz hiçbir parçası kurulu bir sunucuda değil.** Aday olarak yazıldı ve aynı gün değiştirildi. alpha.97'nin yayım kaydından sonra `main` üzerinden `feat/alpha98-menu-layout` dalında yazıldı (alpha.97 sürüm kaydı `0c1a8917ef3f8141702fa41ae085f64d62a713f0`). **Yalnız gezinme** (D-037): her sayfa adının söylediği yerdedir. Sahip 21 Ekim 2026'da Ayarlar'daki her şeyin ayarlarla ilgili olup olmadığını sordu ("ayarlardaki bu alt menüdeki her şey ayarlar ile mi ilgili?"), her işlemin yerinin doğru olması ve herkesin aradığını kolayca bulması gerektiğini söyledi ve 22 Ekim 2026'da, bir taslaktan ve bir ikinci düşünce düzeltmesinden sonra taslağı onayladı ("menu düzeni taslağı ok."). (1) **Sunucu** üç sayfa kazanır: **DNS ve eş sunucular** (`/dns-infrastructure`; Ayarlar → DNS altyapısı idi), **Güvenlik** (`/security`: ayarlarıyla güvenlik duvarı kutusu, saklanan bileşen kaydından okunan ve bileşen sayfasına bağlantısı olan Fail2ban durumu ve Ayarlar → Sunucu güvenliği olan salt-okur dışa açık hizmet denetimi) ve **Bakım** (`/maintenance`; Ayarlar → Sistem veritabanları idi). (2) **Ayarlar dört şey tutar:** Panel adresi ve HTTPS ("Panel HTTPS" idi), Lisans, CelikPanel güncellemeleri ve Sunucu kurulumu; böyle bir ayar olmadığı için yöneticilerin tümü için bir politika bölümü uydurulmadı. (3) **Hesabım** (`/account`, sağ üstteki kişisel menüde ilk sırada) Ayarlar → Hesap güvenliği olan iki faktörlü doğrulama panelini ve aynı parola penceresini açan bir Parola kartını tutar; Ayarlar'daki tek bölümü kendi hesapları olan bayiler ve müşteriler artık boş bir Ayarlar görmez. (4) **Eski bağlantılar çalışır:** her `/settings?section=` adresi yeni sayfasına yönlendirilir (yerine koyma), bayi ya da müşteri için `/settings` `/account`'a gider ve panelin kendi bağlantıları, yönlendirme metinleri ve sunucunun yönlendirme eylemi adresleri (Go) yeni yerleri doğrudan adlandırır. (5) Sahibin onayladığı ikinci düşünceler: İçe aktar Sunucu altında, Sunucu kurulumu Ayarlar'da kalır; Mağaza ve VPN, bayiler ve müşteriler kullandığı için Barındırma altında kalır. (6) Genel portalın GitHub bağlantıları, hesap adı değiştirildikten sonra artık `github.com/alicelik77/...` adresini gösterir (portal bir sonraki yayımlandığında etkili olur). **API, Agent, güncelleyici, şema, rol, yoklama ya da bileşen davranışında hiçbir şey değişmedi**, bu yüzden gerçek sistem kaydı yok (sahibin 16 Ekim 2026 talimatı). Şema 44, geçiş yok. Aday ağaçtaki kanıt: web 1697 geçti (alpha.97'de 1675), yeni testler `web/tests/menu-layout-alpha98.test.mjs` içinde; Go 7156 geçti, 0 başarısız; sürüm kurtarma düzeneği taze bir `archlinux` klonunda 1109 test OK (1 atlandı), sürüm sırası sözleşmesi de geçti; tarayıcı denetimi 23 iki kez bakıldı (durmuş çizilen çalışan Fail2ban, hâlâ "Ayarlar" diyen güvenlik duvarı bağlantısı ve Türkçe denetim satırlarındaki "firewall" düzeltildi); yalnız ekranların gösterdiği kodlu retleri "errors" katalog parçasına taşıyan bir inceleme turundan sonra kritik açılış yolu ham 361 KiB'den 329,16 ve sıkıştırılmış 110 KiB'den 100,37 (öncesinde 360,75; tavan yükseltilmedi). Aynı inceleme turu güvenlik duvarı kutusuna kendi modülünü, her ekrana tek çalışma kuralını ("active (exited)" çalışmıyordur), metinlerde kalan eski yer adlarının düzeltilmesini ve henüz hiçbir ekranın değiştiremediği sertifika iletişim e-postası için dürüst bir reti getirdi (D-037). **Sahibin alpha.97'nin Boston'daki gözlemi (22 Ekim 2026):** sahibin ekran görüntüsü, Boston'un alpha.97'de olduğunu ve Bileşenler'de gerçek sürümlerin göründüğünü gösterir (PowerDNS 4.8.3, Nginx 1.24.0, PostgreSQL 16.15, certbot 2.9.0, Fail2ban 1.0.2, nftables 1.0.9, cron 3.0pl1, Node.js 24.21.0): D-036'nın kurulu bir sunucudaki ilk ölçümü. Frankfurt'un alpha.97 durumu bildirilmedi. Açık: yeni düzen hiçbir kurulu sunucuda görülmedi; Frankfurt'taki sürümler; **yöneticinin kendi e-postasını (sertifikanın iletişim adresi) değiştiren bir ekran yok; taslakta geçen açık oturumlar yapılmadı**. alpha.97'nin "genel portal hâlâ `celikbros/...` adreslerine bağlanıyor" ve "Ayarlar menü düzeni önerisi sahibin kararını bekliyor" açık işleri bu adayla kaynakta yanıtlandı. Aday kabul değildir; P0.1–P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.98.tr.md)).
  - *Yayım, 22 Ekim 2026 (UTC).* Sürümü hazırlayan kişi tarafından, sahibin
    kalıcı talimatı uyarınca; kurulu panel güncellemeleri sahibindir. Bu, tümüyle
    sahibin kendi GitHub kimliğiyle yapılan ilk sürümdür: sahip 21/22 Ekim
    2026'da `momerefe` hesabına yazma erişimi verdi ve GitHub CLI'yı `momerefe`
    olarak oturum açtırdı; böylece `main` üzerindeki sürüm commit'i, açıklamalı
    etiket ve itmeler Mehmet Ömer Efe Çelik <momerefe@gmail.com> adınadır. #234
    numaralı çekme isteği `main`'e `c249a9ba236212bfcdf6eddb590f55bccc5bac9c`
    olarak birleştirildi. Sürüm sırası değişkeni etiket yapılmadan önce 97'den
    98'e geçirildi; `main` koşusu ve etiket koşusu başarılı oldu ve sürüm zinciri
    sorunsuz çalıştı. Sürümden önce bir bağımsız inceleme yapıldı (12 bulgu ve bir
    bulgu daha, hepsi düzeltildi) ve yeniden yapılan bir denetim sürümü yayıma
    hazır buldu. `v0.1.0-alpha.98` açıklamalı etiketi (nesne
    `8696ddbc3880bf2fd2cf794f0c1cc03e27065c86`, ağaç
    `838d69121bf3213d7a71ec6fbf7437f7575413aa`, etiketleyen Mehmet Ömer Efe
    Çelik) sürüm commit'ini gösterir; ön sürüm 2026-10-22T01:01:25Z'de altı
    dosyayla yayımlandı. `CelikPanel-S2-Debian` WSL konuğunda 22 Ekim 2026'da bir
    indirmeden denetlendi (hazırlayan kişi, burada bir dosyada kayıtlı değil):
    arşivler bayt bayt aynı, 66.317.470 bayt, SHA-256
    `23820e87c8266e8321937ae9a2ea2a4b9864b1fd6fa30dace900836365948279`; manifest
    sıra 98 (`published_at` 2026-10-22T00:15:48Z), imzası `openssl pkeyutl
    -verify -rawin -pubin` ile doğrulanır (anahtar `7eadeb0b…`); iki içerik
    koruması 0; Go go1.26.5, CGO kapalı, trimpath; 267 üye; `get.sh` 98'i
    sabitler; politika 97'den sonra 98 (`0c1a8917…`); arayüz `app-qbukM16Q.js`
    (258.759 bayt). Portal paketi 133.007.110 bayt, SHA-256
    `1cb3374ee47f73f3b45a2b5a9ce12888bda0dd59d9a2ef2bb3a0ef64de416174`, 54
    girdi, iki kez aynı derlendi; sayfaları yalnız `github.com/alicelik77/...`
    adresine bağlanır; sözleşme testi 0, genel doğrulayıcı 17 test, terfi 14
    test; hazırlayan kişi yaklaşık 01:11Z'de önceki v0.1.0-alpha.97 ile yayımladı:
    committed, yedek `httpdocs.before-v0.1.0-alpha.98.20261022T011052Z…`,
    doğrulama 26 istekten 26'sı, `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`; yayımdan
    sonra dışarıdan okundu (`latest.txt` v0.1.0-alpha.98, `latest.json` sıra 98;
    canlı ana sayfa `github.com/alicelik77/` adresine bağlanır). **Kurulu paneller
    ve sahibin gözlemleri:** Boston'un alpha.97 sürümleri (yukarıdaki aday
    girdisinde zaten kayıtlı) yinelenmez; alpha.98'den hiçbir şey bir kurulu
    sunucuda değil ve Frankfurt'un alpha.95 durmasından sonraki güncellemesi
    bildirilmedi. Yayımdan sonra açık: yeni düzen hiçbir kurulu sunucuda
    görülmedi; yöneticinin kendi e-postasını değiştiren bir ekran yok (D-037);
    açık oturumlar yapılmadı. Yayımlanma kabul değildir; hiçbir P0 işi kapanmaz ya
    da ilerlemez; P0.1–P0.5 **kısmi** kalır. Ayrıntı için [sürüm
    notları](docs/RELEASE-NOTES-v0.1.0-alpha.98.tr.md), "Yayım kaydı".
  - *Sahibin güncellemeye ilişkin bildirimi, 22 Ekim 2026.* Sahip iki kurulu
    sunucuyu güncelledi ve ikisinin de alpha.98'de olduğunu bildirdi: Boston'un ve
    Frankfurt'un Panel sayfaları yeni menüyü gösteriyor. Frankfurt'un alpha.94'ten
    güncellemesi tamamlandı; daha önceki alpha.95 güncellemesi salt-okur denetimde
    meşgul bir paket yöneticisi yüzünden durmuştu (alpha.97 ile kaydedildi). Bu,
    "yeni düzen hiçbir kurulu sunucuda görülmedi" ve "Frankfurt'un alpha.95
    durmasından sonraki güncellemesi bildirilmedi" maddelerini yanıtlar. Sahibin
    Panel sayfalarında gördüğüdür, taşınan her sayfanın denetimi değildir;
    Frankfurt'un Bileşenler'deki paket sürümleri bildirilmedi, bu yüzden D-036'nın
    açık işi Frankfurt için açık kalır. Sürümü hazırlayan kişi iki sunucuda da hiçbir
    şey ölçmedi. Hiçbir P0 işi kapanmaz ya da ilerlemez.
- **v0.1.0-alpha.99 yayımlandı, 22 Ekim 2026; henüz hiçbir parçası kurulu bir sunucuda değil.** Aday olarak yazıldı ve aynı gün değiştirildi. alpha.98'in yayım kaydından sonra `main` üzerinden `feat/alpha99-own-email` dalında yazıldı (alpha.98 sürüm kaydı `c249a9ba236212bfcdf6eddb590f55bccc5bac9c`). **Giriş yapmış kişi kendi e-posta adresini değiştirebilir** (D-037 değişikliği); alpha.98'in bıraktığı açık iş; sahip yapılmasını 22 Ekim 2026'da onayladı ("ok"). (1) **Hesabım**, Parola ile İki adımlı giriş arasında üçüncü bir kart kazanır: "E-posta adresim" (`/account#email`); güncel adresi ve yeni adres ile mevcut parolayı soran küçük bir pencereyi açan bir Değiştir düğmesini gösterir. Hesabım sayfası olmayan ek kullanıcı (ekip üyesi), kişisel menüde "Parolayı değiştir"in yanında bir e-posta değiştirme öğesi alır. (2) **Sunucu tarafı:** `PUT /api/v1/auth/email`, parola değiştirmenin yanında ve aynı rollere açık (yönetici, bayi, müşteri, ek kullanıcı). Mevcut parola gerekir ve yanlış tahminler parola değiştirmenin sınırlayıcısına sayılır; D-029'un istek kimliği koruması onu korur (koruduğu ilk `PUT`; bir yineleme asla ikinci kez değiştirmez; saklanan özet yalnız yeni adresi tutar, anahtarlar tamdır ve başarı yanıtı adres taşımaz; süre sınırı 30 sn); tek doğrulayıcı, panel sertifikasının kendi iletişim denetimidir, bu yüzden kabul edilen bir adres sertifika isteğinin her zaman kabul ettiği bir adrestir; başka bir hesabın kullandığı adres reddedilir (büyük-küçük harf gözetmeden; var olan `UNIQUE` kısıtı son sözü söyler); yazım karşılaştır-ve-yaz biçimindedir, bu yüzden başka yerde yapılan bir değişiklik bildirilir, üstüne yazılmaz; oturumlar sürer; adressiz tek bir `account.email.changed` denetim kaydı. (3) **Panel sertifikası** adresi istendiğinde okur; eksik iletişim reti ve kurulum planının iletişim engeli artık "E-posta adresinizi Hesabım'da (sağ üst menü) ayarlayın" der ve Hesabım'ı açan bir düğme sunar. Bir e-posta değişikliği hiçbir sertifika için hiçbir şey istemez, yenilemez ya da yeniden denemez; incelenmiş bir kurulum planı değişiklikten sonra yeniden incelenmelidir ve çalışan bir kurulum planının adresini korur. (4) **Lisans kapısı değişmedi:** panel lisans nedeniyle kilitliyken e-posta değiştirme diğer rotalar gibi reddedilir (parola değiştirme erişilebilir kalır); kilitli bir panelin neyi kabul ettiğini değiştirmek lisans politikasıdır ve bu sürümün dışındadır. **Agent, güncelleyici, şema, yoklama, bileşen ya da katalog parçası bölünmesi değişikliği yok**; bu yüzden gerçek sistem kaydı yok (sahibin 16 Ekim 2026 talimatı) ve güncellemeyle ilgili hiçbir dosya değişmediği için sürüm kurtarma düzeneği gerekmez. Şema 44, geçiş yok; sürüm sırası 98'den 99'a geçer. Aday ağaçta kanıt: Go `CelikPanel-S2-Debian` konuğunda aday ağaçta `go test ./cmd/panel/ ./internal/...` (-timeout 45m, CGO kapalı): 64 paket geçti, 2895 üst düzey test geçti, 42 atlandı, 0 başarısız (4279 alt test geçti); gofmt ve go vet temiz; dört rolün işleyici testleri, hiçbir şeyi değiştirmeyen her ret, başka kökenden ve kökensiz `PUT` için CSRF denetimi, kilitlenme, tek kimliğin yinelenmesi, karşılaştır-ve-yaz, koruma ve rota kapıları, panel sertifikasının reti ve yeni adresi, ayrıca bir doğrulayıcı testi; web `npm test` 1709 testin 1709'u geçti (yeni `tests/own-email-mounted.test.mjs`: İngilizce ve Türkçe 11 test; Ayarlar'da sertifika retinin bandı için yeni bir bağlı test; ek kullanıcının menü öğesi), `npm run build` her bütçenin içinde geçti (kritik açılış yolu 361 KiB'nin 329,93'ü); tarayıcı denetimi 24 8 yapılandırmanın her birinde 13 durum (104 ekran görüntüsü), iki kez bakıldı: ilk bakış telefonda ret bandının düğmesini 28 px buldu, artık en az 44 px; ikinci bakış başka bir şey bulmadı; kişisel menüdeki dil seçici (telefonda 28 px) bu sürümden eskidir ve değişmedi; ekran görüntüleri ve tarayıcı sürücüsü sonra silindi; sürüm sırası politikası, indirme portalı ve imzalı sürüm manifesti sözleşmeleri geçti. **Sahibin alpha.98 bildirimi (22 Ekim 2026):** iki kurulu sunucu da alpha.98'de (Boston'un ve Frankfurt'un Panel sayfaları yeni menüyü gösteriyor) ve Frankfurt'un alpha.94'ten güncellemesi, meşgul bir paket yöneticisi yüzünden alpha.95'te durmasından sonra tamamlandı. Açık: e-posta değiştirme hiçbir kurulu sunucuda ölçülmedi; yeni adres ona gönderilen bir iletiyle doğrulanmaz; açık oturumlar yapılmadı; Frankfurt'un paket sürümleri bildirilmedi. alpha.98'in "yöneticinin kendi e-postasını değiştiren bir ekran yok" açık işi bu adayda kaynakta yanıtlanır. Aday kabul değildir; P0.1–P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.99.tr.md)).
  - *Yayım, 22 Ekim 2026 (UTC).* Sürümü hazırlayan kişi tarafından, sahibin
    kalıcı talimatı uyarınca; kurulu panel güncellemeleri sahibindir. #236
    numaralı çekme isteği `main`'e `7ce52a17b416f8ffcf64931e08a2c3234bfa2f51`
    olarak birleştirildi; yazarı Mehmet Ömer Efe Çelik <momerefe@gmail.com>.
    Sürüm sırası değişkeni etiket yapılmadan önce 98'den 99'a geçirildi; `main`
    koşusu ve etiket koşusu başarılı oldu ve sürüm zinciri sorunsuz çalıştı.
    Sürümden önce iki bağımsız güvenlik incelemesi turu yapıldı: ilki iletişim
    kapılarının fazla gevşek olduğunu, büyük-küçük harfi yok sayan anahtarlarla
    bir parolanın tekrar özetine ulaşabildiğini, eksik bir sıralama testini,
    saklanan yanıtta adresin bulunduğunu ve ayrı sınırlayıcı sayaçlarını buldu;
    ikincisi zorunlu bir düzeltme buldu: `REQUEST_OUTCOME_UNKNOWN` sonrasında
    pencere "sonuç bilinmiyor" durumunda döngüye giriyordu. Hepsi düzeltildi.
    `v0.1.0-alpha.99` açıklamalı etiketi (nesne
    `e9de364565e1e87c92f3e419369fda1f78bc1708`, ağaç
    `971776526f398812b66921482f8d779e57c132a9`, etiketleyen Mehmet Ömer Efe
    Çelik) sürüm commit'ini gösterir; ön sürüm 2026-10-22T13:07:25Z'de altı
    dosyayla yayımlandı. `CelikPanel-S2-Debian` WSL konuğunda 22 Ekim 2026'da bir
    indirmeden denetlendi (hazırlayan kişi, burada bir dosyada kayıtlı değil):
    arşivler bayt bayt aynı, 66.326.418 bayt, SHA-256
    `b749b7f469cf4d78c3b3ce7bc8cd0f9ef8f864a96f097f8b4258810d359f68e6`; manifest
    sıra 99 (`published_at` 2026-10-22T12:23:11Z), imzası `openssl pkeyutl
    -verify -rawin -pubin` ile doğrulanır (anahtar `7eadeb0b…`); iki içerik
    koruması 0; Go go1.26.5, CGO kapalı, trimpath; 267 üye; `get.sh` 99'u
    sabitler; politika 98'den sonra 99 (`c249a9ba…`); arayüz `app-CfcdYww_.js`
    (259.484 bayt); `bin/panel` içinde `account_email` var. Portal paketi
    133.025.091 bayt, SHA-256
    `95fcb5c61dc0c15321078bb7e9b36a6722e0fe629fe228541faf75f5566937a3`, 54
    girdi, iki kez aynı derlendi; sayfaları yalnız `github.com/alicelik77/...`
    adresine bağlanır; sözleşme testi 0, genel doğrulayıcı 17 test, terfi 14
    test; hazırlayan kişi yaklaşık 13:16Z'de önceki v0.1.0-alpha.98 ile
    yayımladı: committed, yedek
    `httpdocs.before-v0.1.0-alpha.99.20261022T131614Z…`, doğrulama 26 istekten
    26'sı, `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`; yayımdan sonra dışarıdan
    okundu (`latest.txt` v0.1.0-alpha.99, `latest.json` sıra 99). **Kurulu
    paneller ve sahibin bildirdikleri:** sahibin iki sunucunun da alpha.98'de
    olduğunu bildirmesi yukarıdaki aday girdisinde kayıtlı ve yinelenmez;
    alpha.99'dan henüz hiçbir şey bir kurulu sunucuda değil. Yayımdan sonra açık:
    e-posta değiştirme hiçbir kurulu sunucuda ölçülmedi; kayıtlı sınırlar duruyor
    (yalnız büyük-küçük harfle ayrılan adreslere aynı anda yapılan iki değişiklik
    birlikte geçebilir; Let's Encrypt herkese açık olmayan bir alan adındaki
    adresi reddedebilir; `--create-admin` verilen adresi doğrulamaz); kurulum
    planının iletişim engeli her plan için hâlâ geçerli, bu da sonraki bir karara
    bırakıldı. Yayımlanma kabul değildir; hiçbir P0 işi kapanmaz ya da ilerlemez;
    P0.1–P0.5 **kısmi** kalır. Ayrıntı için [sürüm
    notları](docs/RELEASE-NOTES-v0.1.0-alpha.99.tr.md), "Yayım kaydı".
- **v0.1.0-alpha.100 yayımlandı, 22 Ekim 2026; henüz hiçbir parçası kurulu bir sunucuda değil.** Aday olarak yazıldı ve aynı gün değiştirildi. alpha.99'un yayın kaydından sonra `main` üzerinden `feat/alpha100-removal-and-setup` dalında yazıldı (alpha.99 sürüm commit'i `7ce52a17b416f8ffcf64931e08a2c3234bfa2f51`). alpha.99'dan sonra sahip önerilen bir Bileşenler yeniden tasarımına baktı ve neden daha iyi olacağını sordu; mevcut sayfa seçilen görünümü zaten hatırlıyor ve kataloğu katlıyor, bu yüzden geniş yeniden tasarım **geri çekildi** ve sahip tam olarak üç şeyi onayladı (22 Ekim 2026, "1 evet, 2 b, 3 evet"; [D-037 değişikliği](docs/DECISIONS.tr.md)). (1) **Kaldırma penceresi neyin etkileneceğini adıyla ve neyin kaldığını söyler:** salt-okunur `GET /api/v1/service/removal-impact` (yalnız yöneticiler; katalog, saklanan bileşen kaydı ve Panel veritabanı; yeni Agent çağrısı ve değişiklik yok; sunucu profili önbellekteki Agent bilgilerinden, güvenlik duvarı motorunun durumu var olan okumadan gelebilir) Panel'in bileşen üzerinde barındırdıklarını (siteler, PHP ya da Node.js siteleri, veritabanları, e-posta adresleri ve yönlendirmeler, spam süzgeci için posta alan adları, Roundcube için posta kutuları; en çok beş ad, ardından "+N tane daha"), ona ihtiyaç duyan kurulu bileşenleri, eksik bıraktığı hazır paketleri ve güvenlik duvarı portlarını ya da "CelikPanel bu sunucuda X bileşenine bağlı başka hiçbir şey kaydetmiyor" cümlesini söyler; pencere bir kez, görünür ve 10 saniyeyle sınırlı bekleyerek okur, okuyamazsa "okunamadı — kaldırma yine izinli, önce kontrol edin" der, kaldırmayı asla tutmaz ve asla değişiklik başlatmaz; silinen ve kalan cümleleri koddaki kaldırma yolunu izler (paketler yapılandırma dosyalarıyla ve apt'ta artık hiçbir şeyin ihtiyaç duymadığı, otomatik kurulmuş her paketle — sistemin tamamında — ya da pacman'da başka hiçbir şeyin ihtiyaç duymadığı bağımlılıklarla silinir; apt'ta PHP-FPM ve PostgreSQL için kurulu her sürüm; spam süzgecinde Postfix yeniden bağlanır; veritabanı araçları yeniden ayarlanır; Roundcube'un ağacı, ayarları, adres defteri veritabanı ve `/webmail/` kaydı; CelikPanel kendisi başka bir şey silmez ve bir paketin kendi kaldırma betiğinin sildikleri iddia edilmez); kaldırma işleyicisi ve Agent artık yanıtın bildirdiği aynı küçük koşulları sorar. Var olan bekçiler, retler ve DNS motorlarının kendi akışı değişmedi. (2) **Bileşenler'in ilk ziyareti Kurulu görünümü açar** (D-010); kataloğu yalnız hatırlanan "katalog" seçimi açar; Roundcube kalıntı teklifi görünür kalır. (3) **Sunucu kurulumu Sunucu → Sunucu kurulumu'na taşınır** (`/server-setup`, Bileşenler'in hemen altında, yalnız yöneticiler): `/settings?section=setup` onunla değiştirilir, Pano'daki kullanım amacı kartı ve her ürün içi bağlantı oraya gösterir ve Ayarlar'da üç bölüm kalır. **İlk üç basamaklı ön sürüm numarası:** her sürüm ayrıştırıcısı, sıralayıcısı ve karşılaştırması (Agent güncelleyicisi ve tabanı, Panel keşfi, sürüm sırası ilkesi, kurtarma temeli, `get.sh`, CI etiket denetimi, portal oluşturucu ve yükseltici, web çözücüsü, düzenek) okundu; hiçbiri sürümleri metin olarak sıralamıyor ve testler artık sürümlerin karşılaştırıldığı her yerde alpha.100 > alpha.99 olduğunu kanıtlıyor. **Güncelleyicinin değişiklik yolunda ya da şemada değişiklik yok**; bu yüzden yerel kayıt yok (sahibin 16 Ekim 2026 talimatı); şema 44, geçiş yok; sürüm sırası 99'dan 100'e. Aday ağacındaki kanıt: Go `CelikPanel-S2-Debian` konuğunda `go test ./cmd/panel/ ./internal/...` (-timeout 45m, CGO kapalı): 64 paket tamam, 2913 üst düzey test geçti, 42 atlandı, 0 başarısız (4282 alt test geçti); temiz bir mount ad alanında `go test ./cmd/agent/...`: 1821 üst düzey test geçti, 29 atlandı, 0 başarısız (2838 alt test geçti); gofmt temiz ve `go vet ./cmd/... ./internal/...` temiz; web `npm test` 1740 testin 1740'ı geçti (`tests/removal-dialog-alpha100.test.mjs` 20 test, `tests/server-setup-page-alpha100.test.mjs` 10 test), `npm run build` her bütçenin içinde (kritik açılış yolu 361 KiB'nin 331,08 KiB'i); sürüm sırası ilkesi (taze klon, `archlinux` konuğu), indirme portalı ve imzalı sürüm manifesti sözleşmeleri geçti; hiçbir güncelleme metni değişmediği hâlde sürüm-kurtarma düzeneği taze klonda geçti (1109 test, 1 atlandı); tarayıcı grubu 25, 8 yapılandırmanın her birinde 13 durum (104 ekran görüntüsü), iki kez bakıldı: ilk bakış telefonda "okunamadı" bildiriminin Tekrar dene düğmesini 34 px buldu (artık en az 44 px), sayfasında iki kez "Sunucu kurulumu" yazıldığını ve gereksinim satırının ters okunduğunu buldu; hepsi düzeltildi; ikinci bakış başka bir şey bulmadı; Ayarlar'daki "Sertifika al" (telefonda 34 px) daha eskidir ve değişmedi; ekran görüntüleri ve tarayıcı sürücüsü silindi. Açık: kurulu bir sunucuda hiçbir şey ölçülmedi; bir dağıtım paketinin kendi kaldırma betiğinin sildikleri okunmaz; panel dışında yapılandırılan bir şey pencerenin listelerinde yoktur. Bağımsız bir inceleme (22 Ekim 2026, her yayından önce) kaldırma davranışını değişmemiş buldu ve pencereyi düzeltti: certbot ve güvenlik duvarı motoru artık "hiçbir şey bağlı değil" yerine yenilenmesi duracak sertifikaları, panel güvenlik duvarını ve sunucu kurulumunun gereksinimini adlandırır; önbellekler yalnız CelikPanel'in onları hangi sitelerin kullandığını kaydetmediğini söyler; apt'ın sistem genelindeki `--auto-remove`'u ve pacman'ın bağımlılık kaldırması aileye göre söylenir; veritabanı uyarısı kendi satırıdır; birimi olmayan araçlar için durdurulduğu söylenmez; katalog paketi olmayan bir ailede kaldırmanın reddedileceği söylenir; Roundcube'un kalan ve başarısız okuma cümleleri tamdır; Agent testleri kaydedilir. İkinci bir tur (aynı gün) kendi hatasını taşıyan bir güvenlik duvarı yanıtını "bilinmiyor" yaptı, güvenlik duvarı motorunu "CelikPanel panel güvenlik duvarını artık uygulayamaz, değiştiremez ve geri yükleyemez; yüklenmiş kurallar sunucu yeniden başlayana dek kalabilir" diye anlattı, WireGuard'ın VPN eşlerini ve vsftpd'nin FTP hesaplarını ekledi, certbot listesini otomatik yenilenen site sertifikaları, Panel'in yönettiği panel sertifikası ve sunucu kurulumunun kaydettiği posta sunucusu sertifikalarıyla daralttı, güvenlik duvarı okumasının bir güvenlik duvarı değişikliğini beklemesini önledi ve boş durumu "CelikPanel bu sunucuda X bileşenine bağlı başka hiçbir şey kaydetmiyor" diye daralttı. Aday kabul değildir; P0.1–P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.100.tr.md)).
  - *Yayım, 22 Ekim 2026 (UTC).* Sürümü hazırlayan kişi tarafından, sahibin
    kalıcı talimatı uyarınca; kurulu panel güncellemeleri sahibindir. #238
    numaralı çekme isteği `main`'e `3eeff24561799fb028c3c14f5da55a5de5bba866`
    olarak birleştirildi; yazarı Mehmet Ömer Efe Çelik <momerefe@gmail.com>.
    Sürüm sırası değişkeni etiket yapılmadan önce 99'dan 100'e geçirildi; `main`
    koşusu ve etiket koşusu başarılı oldu. Bir olay hiçbir sürümü etkilemedi:
    ilk CI koşusu Go araç zinciri sözleşmesinde durdu, çünkü kararlar kaydında
    düz bir test komut satırı yer alıyordu; cümle yeniden yazıldı ve zincir
    yeniden çalıştırıldı. Sürümden önce üç bağımsız inceleme turu yapıldı
    (bulgular: kaldırma penceresi certbot, nftables ve WireGuard için "hiçbir şey
    bağlı değil" diyordu; apt'ın otomatik kaldırmasının ifadesi; veritabanı
    uyarısının yeri; bilinmeyen güvenlik duvarı durumunun kapalı gösterilmesi;
    sertifikaların sayılış biçimi; FTP hesaplarının SFTP hesaplarıyla
    karıştırılması) ve hepsi düzeltildi; kaldırmanın kendi davranışının
    değişmediği doğrulandı. `v0.1.0-alpha.100` açıklamalı etiketi (nesne
    `7fc97f6efbc222d370b3518fa3fbde3fa854601f`, ağaç
    `ffd7b6b6cbf90e9da0975bf298712d6a7db6268b`, etiketleyen Mehmet Ömer Efe
    Çelik) sürüm commit'ini gösterir; ön sürüm 2026-10-22T19:03:06Z'de altı
    dosyayla yayımlandı. Bu, üç basamaklı ilk ön sürümdür: manifest, `get.sh`
    sabitleri, `latest.txt`, `latest.json`, `index.json` ve sürüm başına yollar
    v0.1.0-alpha.100 ve sıra 100'ü kesilmeden taşır; sürüm sıralaması denetlendi
    (her yerde sayısal). `CelikPanel-S2-Debian` WSL konuğunda 22 Ekim 2026'da bir
    indirmeden denetlendi (hazırlayan kişi, burada bir dosyada kayıtlı değil):
    arşivler bayt bayt aynı, 66.343.534 bayt, SHA-256
    `2f3855ac9d31b1484b52da681b388b77fbe7ee318560244e9a7d2fcc8e2667b8`; manifest
    sıra 100 (`published_at` 2026-10-22T18:17:33Z), imzası `openssl pkeyutl
    -verify -rawin -pubin` ile doğrulanır (anahtar `7eadeb0b…`); iki içerik
    koruması 0; Go go1.26.5, CGO kapalı, trimpath; 268 üye; `get.sh` 100'ü
    sabitler; politika 99'dan sonra 100 (`7ce52a17…`); arayüz `app-DBhnHj6L.js`
    (260.590 bayt); `bin/panel` içinde `removal-impact` var. Portal paketi
    133.059.310 bayt, SHA-256
    `f59809d953ddd1a61095acc10b8a053bb87e50904a9bafbaf23f61416510198f`, 54
    girdi, iki kez aynı derlendi; sayfaları yalnız `github.com/alicelik77/...`
    adresine bağlanır; sözleşme testi 0, genel doğrulayıcı 17 test, terfi 14
    test; hazırlayan kişi yaklaşık 19:16Z'de önceki v0.1.0-alpha.99 ile
    yayımladı: committed, yedek
    `httpdocs.before-v0.1.0-alpha.100.20261022T191552Z…`, doğrulama 26 istekten
    26'sı, `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`; yayımdan sonra dışarıdan
    okundu (`latest.txt` v0.1.0-alpha.100, `latest.json` sıra 100). **Kurulu
    paneller ve sahibin bildirdikleri:** bu yazılırken alpha.99 ya da alpha.100'den
    kurulu bir sunucuda hiçbir şey bildirilmemişti. Aynı gün sahip Eylül çekme
    isteklerinin kapatılmasını onayladı: 53'ünün tamamı (#151–#203) #204 numaralı
    çekme isteği (alpha.81) aracılığıyla zaten `main`'de bulundu ve bu kanıtla
    kapatıldı; açık çekme isteği kalmadı. Yayımdan sonra açık: alpha.99 ve
    alpha.100'den hiçbiri kurulu bir sunucuda ölçülmedi; bir dağıtım paketinin
    kendi kaldırma betiğinin sildikleri okunmaz; kurulum planının iletişim engeli
    her plan için hâlâ geçerli, bu da sonraki bir karara bırakıldı; kaldırma
    penceresindeki pacman `-Rns` ifadesi biraz geniş (zararsız). Yayımlanma kabul
    değildir; hiçbir P0 işi kapanmaz ya da ilerlemez; P0.1–P0.5 **kısmi** kalır.
    Ayrıntı için [sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.100.tr.md),
    "Yayım kaydı".
- **v0.1.0-alpha.101 yayımlandı, 22 Ekim 2026 (UTC); henüz hiçbir parçası kurulu bir sunucuda değil.** Aday olarak yazıldı ve yayımlanması için değiştirildi. alpha.100'ün yayın kaydından sonra `main` üzerinden `fix/alpha101-real-bundles` dalında yazıldı (alpha.100 sürüm commit'i `3eeff24561799fb028c3c14f5da55a5de5bba866`). 22 Ekim 2026'da sahip iki kurulu sunucuya da alpha.100'de baktı. Frankfurt'ta tek bileşenli veritabanı paketlerini gereksiz buldu ve şöyle karar verdi: "bundle diyip tek servisi kurmak nedir. saçmalık. postgresql ve phpPgAdmin bundle dır mariadb ve phpMyAdmin bundle dır. Diğer tüm bundle lar da böyle dir." (paket, bir hizmet ile ona eşlik eden araçlardır; [D-034 değişikliği](docs/DECISIONS.tr.md)). (1) **Hazır paketler, araçlarıyla birlikte hizmetlerdir.** "PostgreSQL + phpPgAdmin" (PostgreSQL, Nginx, PHP-FPM, phpPgAdmin) ve "MariaDB + phpMyAdmin" (MariaDB, Nginx, PHP-FPM, phpMyAdmin), tek bileşenli veritabanı paketlerinin ve onların phpPgAdmin ve phpMyAdmin eklentilerinin yerini alır; bağlantılar çalışmayı sürdürsün diye `db.postgresql` ve `db.mariadb` kimlikleri kalır, bir sayfa daha eski bir Panel'in tek bileşenli paketini asla araçlı bir paket diye adlandırmasın diye ad kodları değişir; posta paketleri ve Nginx + PHP-FPM değişmedi; kayıtta altı girdi vardır. Bir aracın üzerinde çalıştığı web sunucusu ve PHP-FPM, kurulu başka bir bileşenin doldurduğu rolü kullanan önkoşullardır (Apache altında Nginx "gerekmiyor"dur; Nginx + PHP-FPM Apache'nin yanında engelli kalır); sistemin sunmadığı bir parça, hiçbir zaman bir alt kümeyi değil, paketin tamamını engeller (Arch'ta "PostgreSQL + phpPgAdmin" "phpPgAdmin bu sistemde kullanılamıyor" der; MariaDB + phpMyAdmin kurulabilir kalır). Paketler kodda (`cmd/panel/bundles.go`) tanımlı kalır ve Panel'e derlenir; değiştirmek bir sürüm gerektirir; sahip isterse ileride bir veri kataloğu mümkündür. Go'daki bir kayıt sözleşmesi testi sahibin kuralını zorlar (en az iki bileşen ve bunlardan en az biri yalnız önkoşul değil; posta paketleri tam olarak kendi posta profilleridir; emekli kimlikler ve kodlar geri gelmez). (2) **Kurulu paketin penceresi**, "Zaten kurulu" yerine her parçanın kendi durumunu ve sürümünü satırların kendi verisinden söyler ("Çalışıyor · 1.26.3", "Kurulu · 5.2.1", "Durdu"); ipucunda tam paket sürümü vardır. Doğrula ve cümlesi geri çekildi (sayfanın Yeniden tara düğmesi bileşenleri yeniden okur); doğrulanmamış kurulu bir bileşen paketi çalışmayan parçayı gösterir ve onu satırından başlatmayı ya da Yeniden tara'yı kullanmayı söyler, kendi düğmesi yoktur; posta paketleri Onar'ı korur; Panel `verify` yerine `none` gönderir ve sayfa daha eski bir Panel'den gelen `verify`'ı yine kabul eder; sayfa yanıtın taşımadığı bir kimliği bütün listeyi reddetmek yerine dışarıda bırakır. (3) **Frankfurt'ta phpMyAdmin'in sürümü "5.2.2-really…" okunuyordu.** Panel'in kısa biçimi artık Debian'daki "really"den sonraki sürümü alır (`5.2.2-really5.2.1-1` 5.2.1 okunur); yanıt oluşturulurken türetildiği için saklanmış taramalar da hemen doğru okunur; bir satırdaki tek çalışma ortamı sürümü artık üç noktayla biter ve ipucunda tam metni vardır ([D-036 notu](docs/DECISIONS.tr.md)). Panel'in alpha.100 durumlarından türettiği biçimde: Boston "PostgreSQL + phpPgAdmin · Kur" gösterir (penceresi PostgreSQL ve Nginx'i korur, PHP-FPM'i, ardından phpPgAdmin'i var olan D-033/D-024 sırasıyla birer birer kurar) ve Pano kartı "Henüz kurulu yok · 6 kurulabilir" der; Frankfurt iki veritabanı paketini de kurulu ve doğrulanmış gösterir ve "6 kurulu ve doğrulandı" der. Boston gözlemlerine (kenar çubuğunda Sunucu kurulumu, Pano'daki kullanım amacı düğmesi, PostgreSQL kaldırma penceresi, E-posta adresim kartı) bakıldı; bu sürüm onları değiştirmez. **Agent, güncelleyici, sürüm kurtarma ya da şema değişikliği yok**; bu yüzden yerel kayıt yok (sahibin 16 Ekim 2026 talimatı); şema 44, geçiş yok; sürüm sırası 100'den 101'e. Aday ağacındaki kanıt: Go testleri (kayıt sözleşmesi testi; Boston, Frankfurt, Apache ve Arch biçimli paket testleri; birleşen paketleri adlandıran kaldırma etkisi testi; gerçek Debian ve Ubuntu sürüm metinleriyle kısa biçim testleri) ve web testleri (hazır paket bağlı ve sözleşme testleri, sürüm hücresi testleri, yeni adlarla kaldırma penceresi testleri). Sonuçlar: Panel'in ve iç paketlerin Go testleri `CelikPanel-S2-Debian` konuğunda son ağaçta geçti: 64 pakette 7.199 geçti, 44 atlandı, 0 başarısız (gofmt ve vet temiz; geri çekilen Doğrula bağlantısını hâlâ isteyen bir ön yüz sözleşme testi önce düzeltildi); web testleri geçti: 1.745/1.745; derleme hiçbir bütçe değişmeden geçti. Tarayıcı incelemesi (alpha.101 kaydıyla 19. ve 20. gruplar; 1440 px ve 390 px, İngilizce ve Türkçe, açık ve koyu): Boston ve Frankfurt biçimli şeritler, Boston'ın PostgreSQL + phpPgAdmin planı ve kurulumu (2/2 adımı phpPgAdmin), Frankfurt'un kurulu paket pencereleri ve MariaDB'nin durduğu bir pencere, Pano kartı; bir kusur bulundu ve düzeltildi (sürüm durum sözcüğünden sonraki noktaya yapışıyordu; artık mono yazıyla ayrı duruyor) ve ekranlara yeniden bakıldı; ham kod yok, Türkçe sayfada İngilizce yok, yatay taşma yok. Ekran görüntüleri silindi. Sürüm sırası ilkesi sözleşmesi Git Bash'te geçti ("100 -> 101; historical bootstrap sequence 100"); indirme portalı ve imzalı sürüm manifesti sözleşmeleri `CelikPanel-S2-Debian` konuğunda geçti (Windows'taki Git Bash'te bu ikisi Windows OpenSSL'inin satır sonlarında ve NTFS'in tutamadığı dosya kiplerinde durur; bu bir ürün bulgusu değildir). Açık: alpha.101'den hiçbir şey kurulu bir sunucuda ölçülmedi; paketlerin bir veri kataloğuna taşınıp taşınmayacağı sahibe bırakıldı. Aday kabul değildir; P0.1–P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.101.tr.md)).
  - *Yayım, 22 Ekim 2026 (UTC).* Sürümü hazırlayan kişi tarafından, sahibin
    kalıcı talimatı uyarınca; kurulu panel güncellemeleri sahibindir. #240
    numaralı çekme isteği `main`'e `f860053bea4772f008337238e5787c857fbb5efb`
    olarak birleştirildi; yazarı Mehmet Ömer Efe Çelik <momerefe@gmail.com>
    (commit 2026-10-23T01:34:29+03:00 taşır; bu, manifestin `published_at`
    değeri olan 22:34:29Z'dir). `v0.1.0-alpha.101` ön sürümü
    2026-10-22T23:13:34Z'de yayımlandı. Hazırlayan kişi yayımdan önce denetledi
    (burada bir dosyada kayıtlı değil): arşivler 66.346.084 bayt, SHA-256
    `6de8f594705e22250c56a6c45fe6a2b10c6c1e7db270eeb706878f3069c5f549`; sağlama
    toplamları, manifest, arşiv üyeleri, içerik korumaları ve imza (yayın
    anahtarıyla doğrulandı, SHA-256 `7eadeb0b…c68bb`) denetlendi; `get.sh` 101'i
    sabitler; politika 100'den sonra 101 der; arayüz paketi `app-EnAd5I-6.js`.
    Portal paketi iki kez derlendi ve bayt bayt aynı çıktı; terfi aracının kendi
    açma ve normalleştirme adımı onu kabul etti. Hazırlayan kişi portalı
    yaklaşık 23:22Z'de yayımladı: durum `committed`, genel doğrulama 26 istekten
    26'sı tamam; yayımdan sonra dışarıdan okundu: `releases/latest.txt`
    v0.1.0-alpha.101 der, `latest.json` sıra 101'i ve `f860053…` commit'ini
    gösterir. **Kurulu paneller ve sahibin bildirdikleri:** sahip hem Boston'a hem
    Frankfurt'a alpha.100'de bakmıştı (bu sürüm buna yanıt verir); alpha.101 henüz
    hiçbir kurulu sunucuda değil ve bu yazılırken ondan hiçbir şey
    bildirilmemişti. Yayımdan sonra açık: alpha.101'den hiçbir şey kurulu bir
    sunucuda ölçülmedi (sahibin, panelin kendi güncelleme ekranından güncelledikten
    sonraki gözlemi); kurulum planının iletişim e-postası gereksinimi her sunucu
    planı için geçerli, bu bir karara bırakıldı; paketlerin bir veri kataloğuna
    taşınıp taşınmayacağı sahibe bırakıldı. Yayımlanma kabul değildir; hiçbir P0
    işi kapanmaz ya da ilerlemez; P0.1–P0.5 **kısmi** kalır. Ayrıntı için [sürüm
    notları](docs/RELEASE-NOTES-v0.1.0-alpha.101.tr.md), "Yayım kaydı".
- **v0.1.0-alpha.102 yayımlandı, 23 Ekim 2026 (UTC); henüz hiçbir parçası kurulu bir sunucuda değil.** Aday olarak yazıldı ve yayımlanması için değiştirildi. alpha.101'in yayın kaydından sonraki `main` üzerinden açılan `fix/alpha102-install-email` dalında yazıldı (alpha.101 sürüm commit'i `f860053bea4772f008337238e5787c857fbb5efb`). alpha.99–alpha.101'in bıraktığı açık iş (kurulum planının iletişim e-postası engeli her plana uygulanıyor) karara bağlandı: sahip "iletişim epostası zorunlu olarak gerekiyor ise zorunlu olmalı. kurulumda eposta belirtiyoruz" dedi; engel her planda kalır (ilk kurulum her zaman panelin HTTPS sertifikasını ister ve sertifika yetkilisi yöneticinin adresini iletişim adresi olarak ister; sertifika adımı olmayan tek plan zaten geçerli sertifikası olan bir sunucunun planıdır, o durumda adres kayıtlıdır); ardından adresin ilk verildiği yerde denetlenmesini onayladı ("ok. yap") ([D-021 değişikliği](docs/DECISIONS.tr.md)). **Kurucu yöneticinin e-postasını sertifika iletişim kuralıyla denetler** (`mutationpayload.CanonicalContactEmail`; kurulum planının, sertifika isteğinin ve kendi e-posta değişikliğinin doğrulayıcısı): etkileşimli istemde reddedilen adres (`ali`, `ali@`, `@x.com`, `a b@x.com`, `ali@localhost`) örnekli iki dilli bir satır yazar ve soru yeniden sorulur, değer hiç yazdırılmaz; böyle bir adres içeren kimlik bilgisi dosyası hiçbir şey kurulmadan çalışan yalnız-doğrulama denetiminde reddedilir ve `install.sh` artık dosyaya dokunulmadığını, hiçbir şey kurulmadığını ve neyin düzeltileceğini söyler; yönetici oluşturma komutunun mevcut bir kullanıcı için yeniden çalıştırılması da aynı kurala bağlıdır ve ret hiçbir şeyi değiştirmez. Adres, kendi e-posta değişikliğinin sakladığı gibi doğrulayıcının kanonik biçiminde saklanır. Mevcut kurulumlar etkilenmez (göç yok, yeniden denetim yok; eski geçersiz bir adres için yol Sunucu kurulumu engeli ve "Hesabım’ı aç" olarak kalır). Başka hiçbir yol yönetici oluşturmaz (hesap API'si yönetici hedeflerini reddeder; ekip üyeleri sunucu kurulumunu açamayan müşteri rollü ek kullanıcılardır), bu yüzden ekip üyesi denetimi değişmez; `get.sh` yalnız sabitlerini değiştirir. **Agent, güncelleyici, sürüm kurtarma, arayüz ya da şema değişikliği yok**, bu yüzden gerçek sistem kaydı yok (sahibin 16 Ekim 2026 talimatı); şema 44, göç yok; sürüm sırası 101'den 102'ye geçer. Aday ağaçtaki kanıt: Son ağaçta `CelikPanel-S2-Debian` konuğunda geçti (Panel ve iç paketler, 45 dakika sınırı, CGO kapalı): 64 pakette 7.219 geçti, 44 atlandı, 0 başarısız (2.921 üst düzey test); gofmt ve go vet temiz. `archlinux` konuğunda taze bir klonda sürüm sırası politikası ("101 -> 102; historical bootstrap sequence 101"), indirme portalı, imzalı sürüm manifesti, önyükleme güncellemesi, ilk kurulum imzalı güven, yeni kurulum giriş sınırı ve kurulum yerelleştirme sözleşmeleri geçti; `install.sh` değiştiği için çalıştırılan çevrimdışı sürüm kurtarma düzeneği de geçti (1.109 test, 1 atlandı); Go araç zinciri sözleşmesi `CelikPanel-S2-Debian` konuğunda geçti (yalnız sabitlenmiş 1.26.5 kabul edildiğinden `archlinux` konuğunun Go 1.27'sini tasarım gereği reddeder). Hiçbir test verisinin değişmesi gerekmedi: kabuk ve düzenek verilerindeki her örnek yönetici adresi kuralı zaten geçiyor. Açık: kurulu bir sunucuda ya da gerçek bir ilk kurulumda hiçbir şey ölçülmedi; Beta 1'de sahip temiz ilk kurulum denemesi için iki sunucuyu da silip yeniden kuracak; bu yaklaşan bir kabul adımıdır, yayımlanmış bir iddia değil; sertifika yetkilisi genel olmayan bir alan adındaki adresi yine reddedebilir. Aday kabul değildir; P0.1–P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.102.tr.md)).
  - *Yayım, 23 Ekim 2026 (UTC).* Sürümü hazırlayan kişi tarafından, sahibin
    kalıcı talimatı uyarınca; kurulu panel güncellemeleri sahibindir. #242
    numaralı çekme isteği `main`'e `085cc400131072f3bcda81b560bfcf98dad47f03`
    olarak birleştirildi; yazarı Mehmet Ömer Efe Çelik <momerefe@gmail.com>
    (commit 2026-10-23T12:08:45+03:00 taşır; bu, manifestin `published_at`
    değeri olan 09:08:45Z'dir). `v0.1.0-alpha.102` ön sürümü
    2026-10-23T09:50:14Z'de yayımlandı. Hazırlayan kişi yayımdan önce denetledi
    (burada bir dosyada kayıtlı değil): arşivler 66.350.990 bayt, SHA-256
    `84d281de4cd983f7fb1c1ab4097ccecdd07397ec99870c053d98afbb07cc2753`; sağlama
    toplamları, manifest, arşiv üyeleri, içerik korumaları ve imza (yayın
    anahtarıyla doğrulandı, SHA-256 `7eadeb0b…c68bb`) denetlendi; `get.sh` 102'yi
    sabitler; `install.sh` etiketinkidir; politika 101'den sonra 102 der; Panel
    ikilisi kurucunun yeni e-posta iletisini taşır. Portal paketi iki kez derlendi
    ve bayt bayt aynı çıktı; terfi aracının kendi açma ve normalleştirme adımı onu
    kabul etti. Hazırlayan kişi portalı yaklaşık 10:02Z'de yayımladı: durum
    `committed`, genel doğrulama 26 istekten 26'sı tamam; yayımdan sonra dışarıdan
    okundu: `releases/latest.txt` v0.1.0-alpha.102 der, `latest.json` sıra 102'yi
    ve `085cc40…` commit'ini gösterir. **Kurulu paneller ve sahibin
    bildirdikleri:** sahip Boston'ı ve Frankfurt'u en son alpha.100'de gözlemledi;
    alpha.101 ve alpha.102 portalda, ve ikisinden birini çalıştırdığı bilinen bir
    kurulu sunucu henüz yok. alpha.99–alpha.101'in bıraktığı açık iş (kurulum
    planının iletişim e-postası) sahibin kararıyla kapandı: zorunlu kalır ve
    kurulumda denetlenir. Yayımdan sonra açık: alpha.102'den hiçbir şey kurulu bir
    sunucuda ya da gerçek bir ilk kurulumda ölçülmedi (sahibin, panelin kendi
    güncelleme ekranından güncelledikten sonraki gözlemi; sahibin Beta 1'de iki
    sunucuyu da temiz yeniden kurması yaklaşan kabul adımıdır, yayımlanmış bir
    iddia değil); paketlerin bir veri kataloğuna taşınıp taşınmayacağı sahibe
    bırakıldı; sertifika yetkilisi genel olmayan bir alan adındaki adresi yine
    reddedebilir. Yayımlanma kabul değildir; hiçbir P0 işi kapanmaz ya da
    ilerlemez; P0.1–P0.5 **kısmi** kalır. Ayrıntı için [sürüm
    notları](docs/RELEASE-NOTES-v0.1.0-alpha.102.tr.md), "Yayım kaydı".
- **v0.1.0-alpha.103 yayımlandı, 23 Ekim 2026 (UTC); henüz hiçbir parçası kurulu bir sunucuda değil.** Aday olarak yazıldı ve yayımlanması için değiştirildi. alpha.102'nin yayım kaydından sonraki `main`'den `fix/alpha103-fresh-update-check` dalında yazıldı (alpha.102 sürüm commit'i `085cc400131072f3bcda81b560bfcf98dad47f03`). Sahibin 23 Ekim 2026'daki iki gözlemi, iki sunucu da alpha.100'de ([D-035 değişikliği](docs/DECISIONS.tr.md)): Frankfurt'ta güncelleme ekranı, alpha.102 yayımdayken, "Check for updates"e basılana kadar 12 saatlik zamanlamanın saklanan sonucu olan alpha.101'i sundu ("güncellemede her zaman en son versiyonu göstermedi … sol alt kısımda 101 nolu güncellemeyi gösterdiğinde ve onu tıkladığımda güncelleme ekranında en son güncelleme görünmesi gerekmez miydi"); sahip ardından Frankfurt'u alpha.102'ye güncelledi. Boston'da alpha.102 güncellemesi meşgul paket yöneticisi yüzünden hiçbir şeyi değiştirmeden durdu ve bu bildirim açıkken güncelleme kartı yalnız kurulu sürümü, Teknik ayrıntılar'ı ve anahtarı gösterdi; kenar çubuğu ve Ayarlar öğesi "Güncelleme var · v0.1.0-alpha.102" diyordu. **Güncelleme ekranı açılınca artık yeniden denetler**: bilinen sonuç on dakikadan eskiyse ve otomatik denetim açıksa yalnız yöneticilere açık yeni bir okuma, `GET /api/v1/panel/update/discovery/refresh`, aynı keşfi zamanlamanın atlama ve yazma kurallarıyla, 50 saniyeyle sınırlı ve zamanlama ve düğmeyle aynı anda tek sürüm denetimi olarak çalıştırır; kart bekleyişi bilinen teklifi koruyarak ve Başlat'ı tutarak gösterir, ardından en yeni sürümü gösterir; kenar çubuğu, Ayarlar öğesi ve Pano satırı onu izler; başarısız, atlanan, yanıtsız ya da okunamayan denetim teklifi yaşı ve nedeniyle korur (HTTP hatası ya da okunamayan yanıt, zamanında gelmeyen yanıttan ayrı söylenir); tamamlanan açılış denetimi tarayıcı ayrılsa da saklanır ve zamanlama başka bir denetimin hemen ardından yeniden okumaz; otomatik denetim kapalıyken hiçbir yere sorulmaz ve teklifin yaşı gösterilir. **Durdurulan bir güncelleme kartı boş bırakmaz**: boş kartın nedeni, kartın tarayıcının güncelleme kilidi tutulurken durum satırını, teklifi, önceki denemeyi ve hazırlık okumasını gizlemesiydi; onaylanmayı bekleyen başarısız bir sonuç da bu kilidi tutar; sağlayıcı artık isteğin hiçbir şeyi değiştirmeden ya da doğrulanarak geri dönerek bittiğini söyler, kart da teklifi, nedeniyle denemeyi, paket yöneticisinin durumunu ("hâlâ meşgul" / "işini bitirdi") ve en üstte Başlat'ın bildirimin kapanmasını beklediğini gösterir; bildirim kapanınca, bilinen sonuç on dakikadan eskiyse kart yeniden denetler, böylece sunucunun değişmiş diye reddedeceği bir sürümü sunmaz (bağımsız inceleme). Kilit, başlatma işleyicisinin tam hedefi yeniden denetlemesi, Agent ve güncelleyici değişmedi. **Agent, güncelleyici, sürüm kurtarma ya da şema değişikliği yok**, bu yüzden gerçek sistem kaydı yok (sahibin 16 Ekim 2026 talimatı); şema 44, geçiş yok; sürüm sırası 102'den 103'e geçer. Aday ağaçtaki kanıt: açılış denetimi (eski sonuç yeniden denetlenir, on dakika içindeki taze sonuç denetlenmez, otomatik denetim kapalıyken Agent çağrısı yapılmaz, bir güncelleme kabul edilmişken ya da sunucu kilidi tutulurken denetim yapılmaz, meşgul paket yöneticisi onu tutmaz, başarısız denetim teklifi korur, yanıt vermeyen Agent atlama sayılır, beklerken biten istek hiçbir şey saklamaz, düğme, açılışlar ve zamanlama arasında aynı anda tek sürüm denetimi, yalnız yöneticiler, lisans kilidinde okunabilir); son Go ağacında `CelikPanel-S2-Debian` konuğunda geçti (Panel ve iç paketler, 45 dakika sınırı, CGO kapalı): 7.229 geçti, 44 atlandı, 0 başarısız (2.973 üst düzey test), başarısız paket yok; gofmt ve go vet temiz; web 1.757 test geçti ve derleme bütçeleri içinde; tarayıcı 26. grup (40 durum, EN/TR, açık/koyu, 1440/390; bakıldı, düzeltildi, yeniden bakıldı); `archlinux` konuğunda temiz bir kopyada çevrimdışı sürüm kurtarma düzeneği (1.109 test, 1 atlandı) ile sürüm sırası ("102 -> 103"), indirme portalı ve imzalı manifest sözleşmeleri geçti. Açık: kurulu bir sunucuda hiçbir şey ölçülmedi; açılış denetimi gerçek bir Agent'a ya da gerçek sürüm sunucusuna karşı çalışmadı; durdurulan güncellemenin bildirimi açıkken köşe bildirimi geniş ekranda kartın bir kısmını hâlâ örter. Aday kabul değildir; P0.1–P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.103.tr.md)).
  - *Yayım, 23 Ekim 2026 (UTC).* Sürümü hazırlayan kişi tarafından, sahibin
    kalıcı talimatı uyarınca; kurulu panel güncellemeleri sahibindir. #244
    numaralı çekme isteği `main`'e `f780d5f08d0f295fbf7d249a853d9c130cd28779`
    olarak birleştirildi; yazarı Mehmet Ömer Efe Çelik <momerefe@gmail.com>
    (commit 2026-10-23T16:23:04+03:00 taşır; bu, manifestin `published_at`
    değeri olan 13:23:04Z'dir). `v0.1.0-alpha.103` ön sürümü
    2026-10-23T14:08:46Z'de yayımlandı. Çekme isteğinden önce adayın bağımsız bir
    incelemesi bir zorunlu düzeltme (açık bir durdurma bildirimi, sayfa yenilemeleri
    boyunca açılış denetimini devre dışı bırakıyordu) ve daha küçük bulgular
    buldu; hepsi düzeltildi; sürüm notlarında kaydedilen eksikler ve Dayanıklılık sözleşmesi girdisindeki üç eksik kayıtlı kalır.
    Hazırlayan kişi yayımdan önce denetledi (burada bir dosyada kayıtlı değil):
    arşivler 66.351.430 bayt, SHA-256
    `8a065499b59764cbefbdbe1dc16cfe754b240d865f00191484f75ec2e4ae9a00`; sağlama
    toplamları, manifest, arşiv üyeleri, içerik korumaları ve imza (yayın
    anahtarıyla doğrulandı, SHA-256 `7eadeb0b…c68bb`) denetlendi; `get.sh` 103'ü
    sabitler; `install.sh` etiketinkidir; politika 102'den sonra 103 der; Panel
    ikilisi yeni açılış denetimi yolunu taşır; arayüz paketi `app-DVVjTxvw.js`.
    Portal paketi iki kez derlendi ve bayt bayt aynı çıktı; terfi aracının kendi
    açma ve normalleştirme adımı onu kabul etti. Hazırlayan kişi portalı yaklaşık
    14:17Z'de yayımladı: durum `committed`, genel doğrulama 26 istekten 26'sı
    tamam; yayımdan sonra dışarıdan okundu: `releases/latest.txt`
    v0.1.0-alpha.103 der, `latest.json` sıra 103'ü ve `f780d5f…` commit'ini
    gösterir. **Kurulu paneller ve sahibin bildirdikleri:** sahibin 23 Ekim
    2026 tarihli ekran görüntülerine göre Frankfurt'u sahip alpha.100'den
    alpha.102'ye başarıyla güncelledi (alpha.103'e yol açan eski alpha.101
    teklifinden sonra); Boston'ın alpha.102 güncellemesi paket yöneticisi meşgul
    olduğu için hiçbir şey değişmeden durdu ve Boston bilindiği kadarıyla hâlâ
    alpha.100'de. alpha.103'ü çalıştırdığı bilinen bir kurulu sunucu yok. Yayımdan
    sonra açık: sürüm notlarındaki bilinen eksikler (Dayanıklılık sözleşmesi girdisinde kaydedilen üç eksik); sahibin, panelin kendi
    güncelleme ekranından güncelledikten sonraki alpha.103 gözlemi (Boston'ın
    yeniden denemesi dahil); alpha.103'ten hiçbir şey kurulu bir sunucuda ya da
    gerçek sürüm sunucusuna karşı ölçülmedi; sahibin Beta 1'de iki sunucuyu da
    temiz yeniden kurması yaklaşan kabul adımıdır, yayımlanmış bir iddia değil;
    paketlerin bir veri kataloğuna taşınıp taşınmayacağı sahibe bırakıldı.
    Yayımlanma kabul değildir; hiçbir P0 işi kapanmaz ya da ilerlemez; P0.1–P0.5
    **kısmi** kalır. Ayrıntı için [sürüm
    notları](docs/RELEASE-NOTES-v0.1.0-alpha.103.tr.md), "Yayım kaydı".

- **v0.1.0-alpha.104 23 Ekim 2026'da (UTC) yayımlandı: sürüm commit'i `98c4750b4`, GitHub ön sürümü 22:03:19Z, yapıtlar ve imza doğrulanıp dış denetim geçtikten (26/26) sonra indirme portalı yaklaşık 22:10Z; hiçbir parçası henüz kurulu bir sunucuda değil.** alpha.103'ün yayım kaydından sonraki `main`'den `fix/alpha104-safe-removal` dalında yazıldı (alpha.103 sürüm commit'i `f780d5f08d0f295fbf7d249a853d9c130cd28779`). Sahibin 23 Ekim 2026'da Frankfurt'taki olayı (Debian 13, alpha.103; [D-037 değişikliği](docs/DECISIONS.tr.md)): sürüm listesinden PHP-FPM 8.4'ü kaldırmak (pencere yalnız site denetimi söz vermişti) `apt-get purge -y --auto-remove` üzerinden phpMyAdmin'i, dağıtımın PHP paketlerini ve yaklaşık 35 kütüphaneyi de sildi ve PHP 8.4'ten sunulan web postası ile phpPgAdmin'i durdurdu. Sahip her düzeltmenin bu sürümde çıkmasına karar verdi. **Bir kaldırma adını söylemediği hiçbir şeyi kaldırmaz:** her paket kaldırma (apt ve pacman; DNF erişilmez kalır), hiçbir unit durdurulmadan önce paket yöneticisinin kendi benzetimiyle planlanır; beklenen küme adı söylenen paketler, kaldırılan sürümün paketleri ve yalnız bu kaldırmanın kullanılmaz bıraktıklarıdır; başka her şey (başka bir bileşenin paketi, bileşenin kendi dağıtım paketi, başka bir ters bağımlılık, bir kurulum) kaldırmayı adıyla reddeder; gerçek kaldırma, aynı benzetim bir kez daha yapıldıktan sonra benzetimi yapılan tam listeyi tek paket kilidi altında çalıştırır. **Her PHP aracı sahibin seçtiği PHP sürümünde çalışır:** web postası, phpMyAdmin ve phpPgAdmin nginx satırlarının adını verdiği sürüme bağlıdır (sahibin değişikliği kazanır), her yeniden yapılandırmada orada tutulur (alfabetik "en yeni soket" seçimi kalktı), yalnız aracın Yönet penceresinden, eklentileri sağlanarak, nginx denetlenip yeniden yüklenerek ve aracın kendi soketinden bir kanıtla (CelikPanel'in sürüm ve eklenti denetimi, sonra aracın sayfası) ya da bayt bayt geri almayla taşınır. Kaldırma pencereleri benzetimin yanıtını, ek kaybı sonraki adımla, bir sürümdeki araçları ve siteleri adlandırır ve yalnız araçlar engelse "… sürümüne taşı, sonra kaldır" sunar; sürüm listesi her sürümü kimin kullandığını söyler; PHP'si eksik araç "Hazır değil" der; Debian 13'te phpMyAdmin 5.2.2 okunur. Agent kaldırma yolu değişti, bu yüzden gerçek sistem kaydı yapıldı: atılabilir bir Debian 13 sanal makinesi Frankfurt kaldırmasını phpmyadmin ve phppgadmin adıyla reddetti, temiz bir kaldırmada tam olarak benzetim listesini kaldırdı, phpPgAdmin'i kanıtla PHP 8.5'e taşıdı ve kanıtı başarısız olan bir taşımayı geri aldı ([kanıt](deploy/e2e/removal-safety/alpha104-debian13-20261023/README.md)). Şema 44, geçiş yok; sıra 103'ten 104'e. Aday ağaçtaki kanıt: Agent 4.702 geçti, 29 atlandı, 0 başarısız (temiz bağlama ad alanı); Panel ve iç paketler 7.238 geçti, 44 atlandı, 0 başarısız; web 1.772 geçti ve derleme bütçeleri içinde; tarayıcı 27. grup (88 ekran görüntüsü; bakıldı, düzeltildi, yeniden bakıldı); `archlinux` içinde temiz bir klonda sıra, portal ve bildirim sözleşmeleri ile çevrimdışı sürüm kurtarma düzeneği (1.109 test, 1 atlandı). Açık: kurulu bir sunucuda hiçbir şey ölçülmedi; pacman yalnız bileşen testleriyle; Roundcube'un taşınması sanal makinede koşturulmadı; taramanın "hazır değil" durumu soketin varlığına dayanır; kaldırma yolu istek kimliğiyle korunmaz. Aday kabul değildir; P0.1–P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.104.tr.md)).

- **v0.1.0-alpha.105 24 Ekim 2026'de (UTC) yayımlandı: sürüm commit'i `0a51195cd`, GitHub ön sürümü 09:38:34Z, yapıtlar ve imza doğrulanıp dış denetim geçtikten (26/26) sonra indirme portalı yaklaşık 09:45Z; hiçbir parçası henüz kurulu bir sunucuda değil.** alpha.104'ün yayım kaydından sonraki `main`'den `feat/alpha105-bundle-versions` dalında yazıldı (alpha.104 sürüm commit'i `98c4750b4a5d4ad67447468bcfdde3a3b848465a`). Sahip, paketlerin sürümleri nasıl seçtiğini ve uyumu zorlamaları gerekip gerekmediğini sordu ve ilkeyi hatırlattı: **öner, uyar, asla dayatma** ([D-034 değişikliği](docs/DECISIONS.tr.md), 24 Ekim 2026). Paket penceresi her parçanın sürümünü gösterir ("8.4 · dağıtımın sürümü · önerilen"); **Değiştir** o parçanın kendi kurulumunun sunduklarını listeler (üreticinin PHP deposu, PGDG; yeni depo yok); kurulu parçalar kendi sürümlerini korur; seçilen PHP paketin veritabanı aracını taşır ("PHP 8.3 üzerinde çalışır"). Çalışamayacak bir PHP hiçbir şey değişmeden reddedilir; sınanmamış ya da eski bir PHP uyarıdır. phpMyAdmin, phpPgAdmin ve Roundcube ilk kurulumda kanıtlanır; geçemezse doğrulanmış taşımayla eklentileri olan kurulu başka bir PHP'ye geçer, olmazsa nedeniyle "Hazır değil"; bir onarım kanıtlanır ama asla taşınmaz. Paket yöneticisinin kendi benzetimi bir kurulumun istenenin ötesinde ne getireceğini (başka bir PHP, Apache, başka bir bileşen, bir veritabanı sunucusu) söyler; paket ve bileşen kurulum pencereleri sahip onaylayana kadar Kur'u tutar, sunucu kurmadan hemen önce paket kilidi altında yeniden denetler ve onaylanmamış dikkat çeken bir paketi reddeder; okunamayan denetim "denetlenemedi" der. Agent kurulum yolu değişti, bu yüzden atılabilir bir Debian 13 sanal makinesinde gerçek sistem kaydı yapıldı: PHP 8.3 üzerinde tek başına phpMyAdmin onaysız reddedildi ve hiçbir şey kurulmadı, PHP 8.3 paketi başka PHP olmadan kuruldu ve kanıtlandı, bir ilk kurulum kanıtı 8.3'ten 8.4'e geçti ([kanıt](deploy/e2e/bundle-versions/alpha105-debian13-20261024/README.md)). Şema 44, geçiş yok; sıra 104'ten 105'e; güncelleyici değişmedi, bu yüzden sürüm kurtarma düzeneği koşturulmadı. CI: "Panel race — Server setup" parçası "(1/2)" ve "(2/2)" olarak bölündü. Aday ağacındaki kanıt: Agent 4723 geçti, 29 atlandı (temiz bir bağlama ad alanında); Panel ve iç paketler 7243 geçti, 44 atlandı; web 1796 geçti; üretim derlemesi bütçelerinin içinde. Açık: kurulu bir sunucuda hiçbir şey ölçülmedi; pacman önizlemesi yalnız bileşen testleriyle; posta profili, Sunucu kurulumu, ek PHP sürümü ve Onar kurulumları onayın kapsamında değil. Aday kabul değildir; P0.1–P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.105.tr.md)).
- **v0.1.0-alpha.106 24 Ekim 2026'de (UTC) yayımlandı: sürüm commit'i `a8f90616f`, GitHub ön sürümü 12:36:00Z, yapıtlar ve imza doğrulanıp dış denetim geçtikten (26/26) sonra indirme portalı yaklaşık 12:40Z; hiçbir parçası henüz kurulu bir sunucuda değil.** alpha.105'in yayım kaydından sonraki `main`'den `fix/alpha106-clear-removal` dalında yazıldı (alpha.105 sürüm commit'i `0a51195cd294be5f009f2a2b72e1dd372094e524`). Frankfurt'ta (alpha.105) sahip 8.6, 8.5 ve 8.4 kuruluyken PHP-FPM satırının kaldır düğmesine bastı ve hangi sürümün kaldırılacağını sordu; Panel haklı olarak reddetti, ama ret listenin ortasında "tam olarak şu 80 paketi kaldırır" cümlesinin yanındaydı ([D-037 değişikliği](docs/DECISIONS.tr.md), 24 Ekim 2026). Birden çok sürümü olan çalışma ortamı artık önce **hangi sürüm** olduğunu sorar (her birini kimin kullandığı; "Tüm sürümler" ayrı); tek sürümde başlık onu adlandırır. Her kaldırma penceresi yanıtla başlar ("PHP-FPM şu an kaldırılamaz"), nedenler her biri bir kez, bağlantılarla **Ne yapabilirsiniz** (önce adı geçen bileşeni kaldırmak, siteleri taşımak, tutmak) ve teknik kısım **Ayrıntılar** altında; reddedilen kaldırma Kapat ve görünür biçimde kullanılamaz "Kaldırılamaz" sunar. Bileşen sayfalarının metni kendi katalog parçasına taşındı (Bileşenler parçası boyut tavanını aşmıştı). Yalnız web: Agent ya da Panel değişikliği yok, sıra 106, şema 44. Web 1805 geçti; üretim derlemesi bütçelerinin içinde; hiçbir Go paketi değişmedi. Kurulu bir sunucuda ölçülmedi.
- **v0.1.0-alpha.107 24 Ekim 2026'de (UTC) yayımlandı: sürüm commit'i `2e68e6039`, GitHub ön sürümü 21:06:26Z, yapıtlar ve imza doğrulanıp dış denetim geçtikten (26/26) sonra indirme portalı yaklaşık 21:13Z; hiçbir parçası henüz kurulu bir sunucuda değil.** D-038 kaydından sonraki `main`'den `feat/alpha107-d038` dalında yazıldı (alpha.106 sürüm commit'i `a8f90616f6472300fe6bc29868fa962247da51ad`). Sahip, güvenlik yamalarının zorlanması gerekip gerekmediğini ve tek bir eski sürümün bütün sunucuyu riske sokup sokmadığını sordu ve her bileşen için tek bir ilkeyi onayladı ([D-038](docs/DECISIONS.tr.md)); `main`'in salt-okur denetimi on iki açık buldu ve sahip bunların tek sürümde kapatılmasını onayladı ([D-038 değişikliği](docs/DECISIONS.tr.md)). **Üretici depoları sahibi tarafından sınırlanabilir:** depo kartı planı gösterir (tam apt kural metni ve SHA-256'sı, üreticiyi izlemeye devam eden paketler, `apt upgrade` farkı) ve onaylanınca yalnız o metni yazar; hiçbir şey kendiliğinden sınırlamaz ve sahibin sildiği bir kural asla yeniden yazılmaz. Bir dağıtım paketinin yerine geçecek bir kurulum adıyla reddedilir, yalnız üreticinin sağladığı bir sürümün ortak paketi (`php-common`, `libpq5`) adlandırılır ve onaylanır ve yalın bir kurulum dağıtımın tam varsayılanını alır (`php8.4-fpm`, `postgresql-17`). **Güvenlik → Güvenlik güncellemeleri** işletim sisteminin otomatik güncellemelerini, bekleyen güncellemeleri, kapsamadıkları üretici kaynaklarını ve onların dışında kalanları okur; sınırlanmış kaynakları ekleyen, onaylanmış tek bir ek dosya yazar. **Veri korunur:** PostgreSQL ve MariaDB kaldırılır, purge edilmez. **Paket yöneticisinin dışında:** Roundcube imza, arşiv, atomik değiştirme, kanıt ve geri almayla güncellenir; Node.js yama bildirimleri site başına taşımalarla gelir, kanıtlanır ya da geri konur. Yeniden başlatmalar ve yeniden açılış (Bakım), onaylarıyla tarihli bir destek tablosu, sitenin kendi kullanıcısıyla çalışan yeni Node.js uygulamaları, denetim ayrıntıları (şema 45), depo kartı ve bir işletim sistemi yükseltmesinden sonra salt-okur denetimiyle desteklenen işletim sistemi listesi (Sunucu kurulumu). Agent depo, paket ve kurulum yolları değişti, bu yüzden atılabilir bir Debian 13 sanal makinesinde gerçek sistem kaydı ve sürüm incelemesinden sonra sahibin sınırlaması için ikinci, kısa bir deneme yapıldı; önceki üç deneme bu sürümde düzeltilen üç kusur buldu ([kanıt](deploy/e2e/d038/alpha107-debian13-20261024/README.md)). Şema 44'ten 45'e (geçiş 045, ekleyici); sıra 107. Aday ağacındaki kanıt: Agent 4771 geçti, 29 atlandı (temiz bağlama ad alanı); Panel ve iç paketler 7925 geçti, 45 atlandı, 0 başarısız; web 1844 geçti ve üretim derlemesi bütçelerinin içinde; tarayıcı grubu 30. Açık: kurulu bir sunucuda hiçbir şey ölçülmedi; pacman ve DNF bileşen testleriyle; D-038 değişikliğinin kalan olarak saydığı maddeler. Aday kabul değildir; P0.1–P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.107.tr.md)).
- **v0.1.0-alpha.108 25 Ekim 2026'de (UTC) yayımlandı: sürüm commit'i `f99ae9f3e`, GitHub ön sürümü 18:26:34Z, yapıtlar ve imza doğrulanıp dış denetim geçtikten (26/26) sonra indirme portalı yaklaşık 18:35Z; hiçbir parçası henüz kurulu bir sunucuda değil.** alpha.107'nin yayım kaydından sonraki `main`'den `feat/alpha108-safe-lanes` dalında yazıldı (alpha.107 sürüm commit'i `2e68e6039e5d21160c28944ad9fd74b83747ec4d`). Boston ve Frankfurt'u alpha.107'ye güncelledikten sonra sahip, güvenlik ve stabilitenin her zaman özgürlükten önce geldiğine ve PostgreSQL ana sürümlerinin yerinde yükseltilmek yerine yan yana çalışacağına karar verdi ([D-038 değişikliği](docs/DECISIONS.tr.md), 25 Ekim 2026). **İki yol:** varsayılan olarak işletim sisteminin paketleri, yalnız bir üreticinin getirdiği sürümler kilitli olarak ve kimin yamaladığıyla listelenir; Sury ya da PGDG yalnız, sınırlamayı, kaynağın otomatik güncellemelerdeki yerini ve kaynak satırını bu sırayla yazan, gösterilen ve onaylanan tek bir işlemle açılır (otomatik güncellemeler kapalıyken reddedilir); zaten açık bir depo eksik adımlarını tek bir **Tamamla…** ile gösterir, Güvenlik güncellemeleri üzerinden de; müşteriler hiçbir zaman depo açamaz; kapatmak hiçbir şeyi kaldırmaz ve ondan gelenleri artık güncellenmiyor diye işaretler; panelin PHP araçları dağıtımın PHP'sinde kalır. **Yan yana PostgreSQL:** portlarıyla kümeler, gösterilen portta başka bir ana sürüm, kanıtlı ve geri almalı port değişiklikleri, tablo başına satır sayısı kanıtlı veritabanı taşımaları ve ayrı geçiş ve bırakma adımları, bir küme veritabanı tuttuğu sürece reddedilen ana sürüm kaldırma (şema 46). **Sahibin bulguları:** her kurulum yolu önizlemeyi gösterir (sürüm listesi ve Onar göstermiyordu); Boston'un phpPgAdmin kanıtı Agent'ın umask'inde başarısız oldu (Yeniden denetle ile onarılır; "çalışmıyor" yalnız öyleyse); aynı sürümde bir PHP seçimi yalnız kanıtlar; Sury anahtarının bitişi en yeni kendi imzasınınkidir; yerleşim düzeltmeleri, daralan bir kenar çubuğu, doğrudan bir web postası Güncelle'si, sahip komutlarında `sudo`. **Kalan D-038 maddeleri:** müşterilere sunulan sürümler, denendi işaretleri, bir kaldırmadan önce sistemin kendi izleri. Agent depo, kurulum, PHP aracı ve PostgreSQL yolları değişti, bu yüzden saklanan temel imajdan atılabilir bir Debian 13 sanal makinesinde gerçek sistem kaydı yapıldı; ilk denemesi kümesiz kurulan bir PGDG ana sürümünü buldu, bu sürümde düzeltildi ([kanıt](deploy/e2e/d038/alpha108-debian13-20261025/README.md)). Şema 45'ten 46'ya (geçiş 046, ekleyici); sıra 108. Aday ağacındaki kanıt: Agent 4822 geçti, 29 atlandı (temiz bağlama ad alanı); Panel ve iç paketler 7945 geçti, 45 atlandı, 0 başarısız; web 1906 geçti ve üretim derlemesi bütçelerinin içinde; tarayıcı grubu 31. Açık: kurulu bir sunucuda hiçbir şey ölçülmedi; Ubuntu 24.04 sanal makine denemesi yok; pacman ve DNF'nin üretici yolu yok. Aday kabul değildir; P0.1–P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.108.tr.md)).
- **v0.1.0-alpha.109 bir adaydır (26 Ekim 2026); yayımlanmadı ve hiçbir parçası kurulu bir sunucuda değil.** alpha.108'in yayım kaydından sonraki `main`'den `fix/alpha109-webmail-update` dalında yazıldı (sürüm commit'i `f99ae9f3ec045ea45c7eada690d052c0767848f5`). Frankfurt'un Roundcube 1.6.20 güncellemesi geri alındı; eski bir sürümün yazdığı web postası yapılandırmasında CelikPanel'in PHP denetimi için konum yoktu (HTTP 404): denetim ve konumu artık bir güncellemeden ya da kanıttan önce, yalnız CelikPanel'in kendi biçiminde yerine konur. Güncelleme sunucuda kalıcı bir işlemdir (yeniden yüklenince yeniden bağlanır, asla iki kez gönderilmez, panel yeniden başlarsa salt okunarak uzlaştırılır) ve sonucu satırda kalır. Panelin kendi adresindeki web postası panelin sıkı betik kuralını korur; arayüzü orada tasarım gereği çizilmez, kendi adresini bekler (sonraki adım) ve Web postasını aç henüz sunulmaz. Kurulum onayları gruplu liste. Şema değişmez (46); sıra 109. Aday ağaçtaki kanıt: Agent 4828 geçti, 29 atlandı (temiz bağlama ad alanı; `nobody` olarak da); Panel 3448 geçti, 6 atlandı; iç paketler geçer; web 1921 geçti; atılabilir bir Debian 13 sanal makinesinde gerçek sistem denemesi Frankfurt'un 404'ünü yeniden üretti ve güncellemeyi kanıtladı ([kanıt](deploy/e2e/d038/alpha109-debian13-20261026/README.md)); tarayıcı grubu 32. Açık: kurulu bir sunucuda hiçbir şey ölçülmedi; Ubuntu sanal makine denemesi yok; yarış bölümleri CI'da. Aday kabul değildir; P0.1–P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.109.tr.md)).

### Son DNS sonuçları ve sınırları

- [Güncel kaynakla BIND/PowerDNS gerçek sunucu denemesi](deploy/e2e/dns-kill-matrix/NATIVE-BIND-PDNS-CURRENT-SOURCE-SMOKE-20260927.md), Debian üzerinde yönetilen BIND'den Arch üzerinde panelsiz PowerDNS'e ilk aktarımı doğruladı. SOA, NS ve seçili A yanıtları UDP/TCP üzerinden iki konuk yeniden açılmadan önce ve sonra eşleşti; gerçek Panel ve Agent birimleri kapalı kaldı. Paket yerel ve imzasızdı. Sonraki kayıt değişiklikleri, ters topoloji, kurulu sunucular ve güncelleme kurtarması bu denemede doğrulanmadı; P0.4/P0.5 açık.
- [Geçici PowerDNS birincil/BIND ikincil gerçek DNS denemesinde](deploy/e2e/dns-kill-matrix/evidence/pdns-master-bind-20260928/contract.json), Debian PowerDNS 4.9.17 üzerinde `MASTER` üye ve katalog, panelsiz Arch BIND'e aktarıldı; iki taraf UDP/TCP'de aynı SOA/A yanıtlarını verdi. Ayrı [yerel geçiş ölçümü](deploy/e2e/dns-kill-matrix/evidence/pdns-native-transform-20260927/contract.json), PowerDNS'in SOA/seri, `CATALOG-HASH`, WAL/SHM ve farklı katalog PTR adını ürettiğini gösterdi. Üreticiye göre AXFR ayrıştırıcısının odak testleri var; ürün geçişi, yeniden açılış ve bağımsız geri alma denenmedi. Eşlenik birincil kurulum kapısı kapalı.
- DNS çiftinde PowerDNS birincil yolu, Agent değişikliği başlamadan kurulumda ve motor ön izlemesinde engelleniyor. [Başarısız yerel denemede](deploy/e2e/dns-kill-matrix/NATIVE-PDNS-BIND-PEER-STAGE2-20260927.md) PowerDNS üretici kataloğu değişmiş bulundu; [PowerDNS belgesi](https://doc.powerdns.com/authoritative/catalog.html) otomatik seri artışını açıklıyor. V1 bu canlı hedefi veya güvenli geri almayı kanıtlayamıyor. V3 durum/sahiplik ve işlem günlüğü kodu ile başlangıç öncesi korumalı geri alma eklendi. Kullanıcının onayladığı tamamlanmış günlük arşivleme kaynak kodda uygulandı; arşivin kalıcı yazılması ve etkin kaydın kaldırılması için kesinti testleri var. Aynı işleme bağlı Agent aracılı başlangıç sonrası ileri kurtarmanın kontrol noktası ve sahip değişikliği odaklı paket testleri var; ürünün gerçek kesinti ve yeniden açılış kabulü açık. V3 kanıtı için açık bir Agent/kurtarma uyumluluk sözleşmesi bulunana kadar uygulama değişimi de reddediliyor; eski kanıt politikası işareti yeterli sayılmıyor. Yeni şemalar yayımlanmadı veya kurulu sunuculara uygulanmadı; güvenlik kapısı kapalı ve P0.4/P0.5 açık.
- [Yeni PowerDNS birincil V3 gerçek sistem kesinti denemesinde](deploy/e2e/dns-kill-matrix/evidence/pdns-v3-native-20260928/README.md) başlangıçta yapılandırılmamış Debian birincil ve panelsiz Arch BIND ikincil kullanıldı. `target-enable-intent` kesintisinden sonra ayrı sürecin aynı isteği ilk kurtarma girişiminde tamamladığı işlem defterinde `succeeded` olarak kaydedildi; V3 günlük arşivlendi ve yönetim devre dışıyken birincil yeniden açıldıktan sonra iki uç UDP/TCP üzerinden yetkili yanıt vermeyi sürdürdü. Önceki deneme, Debian PowerDNS SOA adlarını düzeltirken katalog NS içeriğini değiştirmediği için güvenle bilinmiyor durumunda kaldı; sonraki makine uzlaştırması geçmiş hata kaydını korudu. SSH ile başlatılan iki kesinti `255` bildirdi; üçüncü temiz denemede systemd bağımsız olarak `Result=signal`, `ExecMainStatus=9` (SIGKILL) kaydetti ve aynı istek yine ilk kurtarma girişiminde tamamlandı. Kabuk çıkışı `137` iddia edilmiyor. Var olan BIND birincilden PowerDNS geçişi, sahip düzenlemeleri, bağımsız geri alma, diğer kesinti noktaları ve çiftin tüm yaşam döngüsü açık; genel kurulum kapısı ve P0.4/P0.5 kapanmadı.
- [Yeni PowerDNS V3 başlangıç öncesi bağımsız geri alma](deploy/e2e/dns-kill-matrix/evidence/pdns-v3-prestart-20260928/README.md), geçici Debian sunucuda gerçek SIGKILL sonrasında ayrı ve doğrulanmış sahip CLI ile bir kez tamamlandı. Önceki iki temiz deneme, etkiden önce güvenli ret vererek kapalı/maskeli birimin cgroup alanı ve yalnız yerel DNS dinleyicisi hakkındaki aşırı katı denetimleri ortaya çıkardı; odak testleriyle düzeltildi. Başarılı deneme PowerDNS'i önceki kapalı durumuna döndürdü, aday veritabanı ile etkin günlüğü kaldırdı ve geri alma kararını yönetim kapalıyken yeniden açılış boyunca korudu. Eski kurulum isteğinin kaydı doğru biçimde başarısız kaldı. Gerçek sistem denemesinde yeniden açılış sonrası tam istek sorgusu geçici kilit yokluğunda çalışmadı; günlük yokken yalnız kalıcı sonuç kaydını okuyan yeni yol, odak testlerini ve önceki gerçek geri almanın kanonik kaydıyla ayrı bir geçici Debian yeniden açılış denemesini geçti. Bu, üretici/ters işlemin veya kurulu kurtarma paketinin yeniden denenmesi değildir. Sonraki kesintiler, motor göçü ve genel kullanım izni açık. P0.4/P0.5 kısmi kalır.
- İlerideki ayrı motor geçişi açığı: mevcut eşli yönetilen BIND birincilini PowerDNS birincile taşımak, yeni V3 veya tek sunuculu V4 yoluyla kanıtlanmış değildir. Bu geçiş için ayrı eşli kaynak/hedef kanıtı ve kurtarma denemeleri gerekir. Yeni PowerDNS birincil/BIND ikincil kurulumunun kabulünden ayrıdır.
- İki topolojinin doğrudan kabul yolu: geçici BIND birincil/PowerDNS ikincil çiftinde ilk yetki, tamamlanmış düzenleme, sahibi tarafından kaydedilmiş SSH ile tamamlanmış silme/yeniden ekleme ve yönetim kapalıyken yeniden açılış kanıtı var; sıradan sahip kaydı ve kesinti hücreleri açık. [Yeni PowerDNS birincil/BIND ikincil V3 denemesinde](deploy/e2e/dns-kill-matrix/evidence/pdns-v3-zone-20260928/README.md) gerçek target-enable-intent SIGKILL sonrasında aynı isteğe bağlı ayrı süreç kurtarması, üst bölgesi yetkili bir alt alan için tamamlanmış ekleme/düzenleme/silme/yeniden ekleme ve yönetim kapalıyken iki yerel DNS hizmetinin yeniden açılış sonrası yanıtı kanıtlandı. Bu denemede üst bölgesiz silme, ikincinin yerel yokluk kanıtı için yetkili kayıt olmadan doğru biçimde beklemede kaldı. Genel kurulum/RPC izni, sıradan sahip kaydı, sonraki kesinti hücreleri, sahip düzenlemeleri ve bağımsız ters işlem açık; P0.4/P0.5 ile eşli birincil kapısı tamamlanmadı.
- [Sahibin çalıştırdığı BIND inceleyici kaydı denemesi](deploy/e2e/dns-kill-matrix/evidence/owner-bind-enrollment-20260928/README.md), geçici Debian/Arch sunucularda yerel birincil/ikincil kaydı, sabit komutlu SSH kimlik doğrulaması, yeniden açılışta kalıcılık, açık yetki kaldırma ve yerel BIND hizmetinin devamını doğruladı. Son ikincil durumu yapılandırılmıştır; geçerli silme kanıtı değildir. Aktarılmış katalogla geçerli sorgu, ürünün sunucu anahtarı sabitlemesi, yarım kurulum temizliği/anahtar yenileme, Debian ikincil ve paketleme açık; P0.4/P0.5 kısmi kalır.
- Daha sonraki [sahip kayıtlı PowerDNS birincil/BIND ikincil üst bölgesiz silme denemesi](deploy/e2e/dns-kill-matrix/evidence/pdns-bind-owner-proof-20260928/README.md), sabitlenmiş ürün SSH aktarımını ve aktarılan katalog/yerel BIND bölge gözlemini kullandı. Gerçek V3 Agent silmesi kalıcı tamamlanmış işlem kaydına ulaştı; ikincil yeniden açıldıktan sonra yönetim kapalıyken katalog boş ve bölge yüklenmemişti. Yalnız bu eski deneme açığı kapandı. Ürün paketleme/izin, diğer kesintiler ve bağımsız ters işlem açık; P0.4/P0.5 kısmi.
- [Açık sahip kaydı devam yolu](deploy/e2e/dns-kill-matrix/evidence/owner-bind-resume-20260928/README.md), birebir hazırlanmış BIND inceleyici dosyalarını ve kısıtlı hesabı yeniden kullanıyor. Geçici Arch denemesinde hazırlanmış SSH kesinti durumları tamamlandı, bitmiş kayıt anahtarı değiştirmeden tekrarlandı, açık iptal sonrası yeniden yetki verildi ve sahip SSH düzenlemesi korunarak devam reddedildi. Yerel BIND çalıştı. Bu bir süreç sonlandırma/güç kaybı denemesi değildir; Debian, diğer kesintiler, temizlik/anahtar döndürme ve ürün paketleme açıktır.
- [Debian sahip yetkilendirmesini sürdürme ve arşiv bütünleştirmesi](deploy/e2e/dns-kill-matrix/evidence/owner-bind-debian-20260928/README.md), güncel sıkı hesap denetimleriyle gerçek Debian BIND/OpenSSH üzerinde geçti. Normal CLI, sahibin SSH değişikliğini korudu; anahtar kapalı, BIND aktif kaldı. Kaynak paketleme artık yetkilendirme başlatmadan isteğe bağlı sahip araçlarını taşıyor. Gerçek dist tarifi, gerçek araçlar ve ilgisiz bileşenlerin etkisiz test dosyalarıyla izin, checksum/bozulma ve umask tekrarlanabilirlik kontrollerini geçti. Bu sonuç Debian devam noktalarını ve kaynak arşiv bütünleştirmesini kapatır; tam yetkilendirme, genel kullanım kapısı, imzalı sürüm ve bağımsız geri alma açık kalır.
- Bağımsız yeni-PowerDNS V3 yürütücü denetimi, Linux üzerinde `syscall.Stat_t` döndüren `os.Lstat`/`File.Stat` çağrılarından yanlışlıkla `unix.Stat_t` bekliyordu. Gerçek root sahipli dosya testi, doğru dosyanın reddedildiğini yeniden üretti. Düzeltmeden sonra kurtarma paketinin bütün testleri geçti; değişmiş içerik, yanlış sahip/izin, özel izin bitleri ve sembolik/sabit bağlantılar reddediliyor. Yalnız geçici dosyalar kullanan root testi CI sistemine bağlandı. Şema ve kurtarma kararı değişmedi; bu dosya kanıtı düzeltmesidir, gerçek bağımsız kurtarmanın kabulü değildir.
- [Yönetilen BIND V3 silme işlemi](deploy/e2e/dns-kill-matrix/NATIVE-BIND-V3-DELETION-TERMINAL-20260926.md), yetkili üst bölge sunan, panelsiz ikincille doğrulanmış sonuca ulaştı. Yerel bölge kaldırılması yeniden açılışta korundu. Üst bölge kanıtı yokken silme beklemede kalıyor; yalnız `REFUSED` yokluk kanıtı sayılmıyor.
- [Çift sunucuda target-staged/after-write](deploy/e2e/dns-kill-matrix/NATIVE-BIND-PAIR-TARGET-STAGED-20260926.md), gerçek SIGKILL, aynı isteğin Agent aracılığıyla kurtarılması, ikincile aktarım ve yönetim devre dışıyken yeniden açılışı geçti. Yeniden açılış gözlemleri, özet değeri alınan arıza arşivinden ayrı belgelenmiştir.
- [PowerDNS sahip değişikliği reddi](deploy/e2e/dns-kill-matrix/NATIVE-PDNS-OWNER-EDIT-20260925.md), sonlandırma sonrası sahibin düzenlemesini, günlüğü ve çalışan DNS hizmetini korudu. Bu sınırlı ret kanıtıdır; her değişiklik anındaki yarış güvenliği veya bağımsız geri alma değildir.
- Tek sunucuda yönetilen BIND üzerinden başlangıçta kapalı ve devre dışı PowerDNS hedefine geçiş için V4 kanıtı ve korumalı sahip CLI geri alması kaynak kodda var. Tam aday dosya canlı veritabanı adına taşınmış fakat PowerDNS henüz başlamamış ve servisi kayıtlı devre dışı durumda kalmışsa, kaynak durduruldu aşamasında aynı dosya özel dizine geri alınabiliyor. Kaynak kod, servis etkin fakat henüz çalışmıyorsa bunu yalnız kalıcı V4 etkinleştirme niyetiyle kabul ediyor ve önce servisi kayıtlı devre dışı duruma getiriyor. Üretici henüz bağlı değil; gerçek sistem kesinti/yeniden açılış denemesi ve PowerDNS başladıktan sonraki geri alma kabulü yok. [Kesin kapsam](docs/DNS-ENGINE-ARTIFACT.md#standalone-bind-to-powerdns-v4-recovery-code-scope-2026-09-27). P0.4 açık.
- [DNS envanterinde](deploy/e2e/dns-kill-matrix/README.md) 510 ham birleşim var: 268 uygulanabilir/çalıştırılabilir, 242 gerekçeli kapsam dışı. **268, geçen test veya hazır deney ortamı sayısı değildir.** Yönetilen BIND ve eski ikincil kaynak hazırlayıcıları, çift sunucunun sonraki aşamaları ve çalıştırılmamış hücreler açık. Aynı hücrenin tekrarı aşama kapsamını artırmaz.
- 12 Eylül farklı motorlu çiftin silme iddiası düzeltildi: olumsuz DNS yanıtı yeterli kanıt değildi. Yeni BIND/BIND başarısı, geçmiş BIND/PowerDNS kabulünü kapatmıyor.

- 29 Eylül 2026: `e9d1019d` kaynağının salt-okur denetimi [DNS kurtarma kabul kütüğünü](docs/DNS-RECOVERY-ACCEPTANCE.tr.md) üretti. Kurtarma sözleşmesi olmayan tek erişilebilir yol bulundu: tek sunucuda veya çiftin ikincilinde çalışan BIND'i PowerDNS'e çevirmek eski V1 günlüğünü yazıyor, yalnız Agent'ın kendi ters işlemine dayanıyor ve gerçek sistemde hiç denenmemişti. [D-026](docs/DECISIONS.tr.md) bunu bu sürümde desteklenmiyor ilan etti; Panel, kurulum ve Agent artık değişiklik başlamadan reddediyor (`bind_source_pdns_switch_unsupported`), ekranda neden ve sonraki adım yazıyor. Var olan günlüğün açılış kurtarması bu kapıdan geçmez. İlk kurulumlarda Agent'ın aynı işlemle kurtarması kabul edildi. Kill-matrix düzeneği boş tek sunucu PowerDNS ve boş tek sunucu BIND target-verified hücrelerini hazırlayacak biçimde genişletildi; bu hazırlıktır, kanıt değildir; 268 çalıştırılabilir sayısı değişmedi. `cmd/panel` ve yeni `cmd/agent` testleri yerelde geçti; `cmd/agent` paketinin tamamı yerel WSL konuğunda hiçbir kullanıcı/grup düzeninde temiz koşmuyor ve `e9d1019d`'de de aynı biçimde düşüyor; tam doğrulama itilmemiş dal için CI işi olarak açık.
- 29 Eylül–1 Ekim 2026: kesinti matrisinin 4–12. grupları ve iki geçici CelikPanel sunucusunda yedi ürün akışı çift koşusu 2. maddeyi adı belli sınırlarla kapattı ([kütük](docs/DNS-RECOVERY-ACCEPTANCE.tr.md)). [7. çift](deploy/e2e/dns-pair-acceptance/evidence/pair7-20261001/README.md) BIND/BIND, BIND/PowerDNS ve PowerDNS/BIND düzenlerinde her adımı geçti: sahibin `dns-peer-enroll` kaydından sonra ikincilde kanıtlanan bölge silme, yeniden ekleme, panel ve Agent kapalıyken DNS yanıtlamaya devam ederken yeniden açılış ve yönetimin dönüşü dahil; [12. grup](deploy/e2e/dns-kill-matrix/evidence/batch12-zero-zone-complete-20261001/README.md) sıfır bölgeli boş PowerDNS birincil hücrelerini, kayıttan sonra sürdürülen ebeveynsiz silmeyle tamamladı. Yalnızca bu gerçek sistem koşularının bulduğu on bir ürün kusuru yol boyunca kaynakta kapatıldı; her biri kanıtıyla kütükte. Boş çift PowerDNS birincili kapısı ölçülen kapsam içinde ana hatta açık ([D-028](docs/DECISIONS.tr.md)). P0.4 ve P0.5 kısmi kalıyor: bölge eşitleme içinde SIGKILL yok, dizüstü ana makinede yalnızca test amaçlı lisansla hücre başına tek çalıştırma, damga kabulü ve kanıt süre aşımı gerçek sistemde görülmedi, kurulu sunucuya dokunulmadı.

### Sıradaki işler ve bağımlılık sırası

| Sıra | Mevcut plandaki iş | Bitiş kanıtı |
|---|---|---|
| 1 | P0.4: [DNS kurtarma kabul kütüğü](docs/DNS-RECOVERY-ACCEPTANCE.tr.md) (29 Eylül 2026), ürünün başlatabildiği her DNS motoru değişikliğini geçti / adı belli eksik / desteklenmiyor durumlarından birine bağlar. [D-026](docs/DECISIONS.tr.md) ile çalışan BIND→PowerDNS geçişi artık Panel, kurulum ve Agent'ta her topolojide reddediliyor (`bind_source_pdns_switch_unsupported`; bileşen testleri geçti); ilk kurulumlarda Agent'ın aynı işlemle kurtarması kabul edilen sözleşme. Debian'da korumalı sahip CLI kurtarması için dört sınırlı gerçek sistem kanıtı var: harici PowerDNS devralması, PowerDNS-BIND geçişi geri alması, çalışan-BIND devralması geri alması ve yeni PowerDNS V3 başlangıç öncesi ters işlemi. 29 Eylül 2026'da dört boş kurulum hücresi gerçek sistemde koştu ([kanıt](deploy/e2e/dns-kill-matrix/evidence/fresh-install-20260929/README.md)): boş PowerDNS hedef başladıktan sonra ve boş BIND hedef başladıktan sonra (Debian ve Arch) aynı istekle kurtarıldı; boş PowerDNS hedef başlamadan önce kesilince **düştü** — geri almanın durmuş hedef kanıtı hiç başlamamış, kurulumun maskelediği birimi reddediyordu. `1c336f6d` ile düzeltildi; `target-staged` ve `intent` hücrelerinin düzeltilmiş kaynakla [yeniden koşusu](deploy/e2e/dns-kill-matrix/evidence/fresh-install-rerun-20260929/README.md) açılışta geri aldı ve aynı isteğin yeniden denemesiyle tamamlandı. Sahip komutu yönlendirmesi (D-024) kaynakta kapandı: durum komutu ve Agent ret metinleri tam sahip komutunu ve istek kimliğini söylüyor ya da hiçbirinin uygulanmadığını açıkça belirtiyor. Bu iş açık bir kod eksiğini ortaya çıkardı: yeniden açılan Agent kirasını bıraktıktan sonra (`released-undecided`) `recover-dns-bind-switch`, `recover-dns-bind-adoption` ve `recover-dns-pdns-adoption` günlüğü reddediyor; yani Agent çalışırken yarıda kesilen PowerDNS→BIND geçişinin veya devralmanın kabul edilen sahip komutu yok. Önceki gerçek sistem kanıtları Agent kapalıyken alınmıştı. Kabul kuralı (yalnız Agent'ın bilerek bıraktığı iş; şema değişikliği yok, defter yeniden yazılmıyor; doğru metin okuma anında hesaplanıyor) ve denetleyicinin `--owner-inverse-after-restart` akışı kaynakta. Gerçek sistem sonucu: `7ad24282` üzerindeki ilk koşu güvenlik denetimleri geçerek düştü (sahip komutu hiç başlamamış, koruma maskeli BIND hedefini reddetti), `411398d9` ile düzeltildi ve `target-staged` ile `intent` hücrelerinin [yeniden koşusu](deploy/e2e/dns-kill-matrix/evidence/owner-inverse-after-restart-rerun-20260929/README.md) Agent yeniden başlamış ve çalışırken geçti — PowerDNS baştan sona aynı süreçle hizmet verdi, BIND hiç başlamadı. Ardından kaynak durdurulduktan sonraki kesintiler (`source-stopped`, `target-started`) akışın kritik çeşidinde [geçti](deploy/e2e/dns-kill-matrix/evidence/owner-inverse-critical-20260929/README.md): PowerDNS tek yetkili olarak yeniden hizmet verdi, bölge seri numarası değişmedi; ölçülen PowerDNS kesintisinin üst sınırı 21,6 sn ve 9,4 sn oldu, bu hücrelerde DNS yapısı gereği kesintisiz değildir. **1. madde 29 Eylül 2026 itibarıyla adı belli sınırlarla kapandı** (aynı günkü sahip kararı): tek sunucuda boş BIND, boş PowerDNS, PowerDNS→BIND ve harici PowerDNS devralması, hedef başlamadan önce ve sonra aynı işlemle kurtarma için gerçek sistem kanıtına sahip; kurtarma sözleşmesi olmayan yollar kodda reddediliyor. [Kütükte](docs/DNS-RECOVERY-ACCEPTANCE.tr.md) adlarıyla 2. maddeye taşınanlar: bütün çiftli topolojiler, Agent çalışırken BIND ve PowerDNS devralması (yalnız bileşen testi), durmuş BIND devralma ve BIND yeniden kurulumu (gerçek sistem denemesi yok), V2 before-write ve rolled-back hücreleri, yeniden açılış hücreleri, yeniden çalıştırma çıkış kodu ve diskte kalan artıklar. 1. maddenin kapanması P0.4'ü kapatmaz. Boş çift ikincil hücreleri, düzenekte olmayan panelsiz yerel birincil eşi gerektiriyor; 2. maddeye taşındı ve açık kalıyor. 1. madde/P0.4 tamamlanmamıştır. | [Kabul kütüğü](docs/DNS-RECOVERY-ACCEPTANCE.tr.md); [Çalışan-BIND devralması](deploy/e2e/dns-kill-matrix/NATIVE-BIND-ADOPTION-OWNER-CLI-20260927.md), [PowerDNS-BIND geçişi geri alması](deploy/e2e/dns-kill-matrix/NATIVE-BIND-PROTECTED-OWNER-CLI-20260927.md), [PowerDNS devralması](deploy/e2e/dns-kill-matrix/NATIVE-PDNS-PROTECTED-OWNER-CLI-20260926.md) ve [yeni PowerDNS V3 başlangıç öncesi ters işlem](deploy/e2e/dns-kill-matrix/evidence/pdns-v3-prestart-20260928/README.md). Deneme sınırları ilgili raporlarda yer alır. |
| 2 | P0.4/P0.5: eksik kaynak/ikincil deney ortamlarını ve üst bölge ya da uzak panel gerektirmeyen uygulanabilir silme doğrulamasını tamamla. 1. maddeden taşınanlar (29 Eylül 2026, [kütükte](docs/DNS-RECOVERY-ACCEPTANCE.tr.md) adlarıyla): boş çift ikincil hücreleri için panelsiz yerel birincil eş, Agent çalışırken devralma geri almaları, durmuş BIND devralma ve BIND yeniden kurulum hücreleri, V2 before-write ve rolled-back hücreleri, yeniden açılış hücreleri. **2. madde 1 Ekim 2026 itibarıyla adı belli sınırlarla kapandı** ([kütük bölümü](docs/DNS-RECOVERY-ACCEPTANCE.tr.md)). Kesinti matrisi 4–12. grupları koştu (panelsiz BIND ve PowerDNS birincil eşler; başlamadan önce ve sonra kesilen boş BIND ve PowerDNS ikincilleri, yönetim kapalıyken yeniden açılışlar; durmuş BIND devralma; V2 before-write ve rolled-back hücreleri; genel RPC üzerinden tek bölgeli ve sıfır bölgeli boş çift PowerDNS birincili, tutulan sahip düzenlemeleri, Agent'ın bıraktığı işin sahip komutuyla bitirilmesi, sahip kaydından sonra sürdürülen ebeveynsiz silme) ve ürün akışı çift sürücüsü iki geçici CelikPanel sunucusunda yedi kez koştu; [7. çift](deploy/e2e/dns-pair-acceptance/evidence/pair7-20261001/README.md) BIND/BIND, BIND/PowerDNS ve PowerDNS/BIND düzenlerinde her adımı geçti: kurulum, bölge ekleme, kayıt düzenleme, sahip kaydından sonra ürünün kanıtıyla bölge silme (`dns-peer-enroll`, BIND ve PowerDNS ikincilleri), yeniden ekleme, panel ve Agent kapalıyken DNS yanıtlamaya devam ederken yeniden açılış ve yönetimin dönüşü. Yalnızca gerçek sistem koşularının bulduğu on bir ürün kusuru yol boyunca kaynakta kapatıldı (sıfır bölgeli katalog denetimi, sihirbaz durumları ve lisans yenileme, müşteri arşivindeki kanıt, Arch'ta rndc anahtarı, PowerDNS bildirim bağlantı noktası, yönetilen ikincilde yerel katalog aktarımı ve denetleyici nedenleri, yalnızca DNS'li alan adında posta aşaması, yönetilen PowerDNS ikincilinde sahip denetleyicisi, BIND birincil planının kaynak durumu, sahip değişikliği sanılan daemon damgası, dalga sınırında atılan olumlu kanıt). Bu kanıtla boş çift PowerDNS birincili kapısı ölçülen kapsam içinde ana hatta açık ([D-028](docs/DECISIONS.tr.md)). Kütük satırları 3, 5, 6, 12 ve 17 sınırlarıyla GEÇTİ; 8 ve 14 EKSİK kalıyor; adı belli sınırlar (hücre başına tek çalıştırma, dizüstü ana makine, yalnızca test amaçlı lisans, bölge eşitleme içinde SIGKILL yok, damga kabulü ve kanıt süre aşımı gerçek sistemde görülmedi, güncelleme tetikleyicisi olmayan sürüm-1 BIND ikincili, müşteri arşivindeki düzenek) kütükte. 2. maddenin kapanması P0.4 ya da P0.5'i kapatmaz ve kurulu panel güncellemesine yetki vermez. | Desteklenen gerçek birincil/ikincil birleşimleri ekleme/düzenleme/silme, yüklenen yerel bölge durumu ve yeniden açılışı kanıtlar; belirsizlikte aynı işleme yönelik uygulanabilir kurtarma yolu sunulur. Deney altyapısı açığı kapsam dışı sayılamaz. |
| 3 | P0.1–P0.5: kalan uçtan uca güncelleme, erişim, şema, TLS/devreye alma ve hizmet matrisini kapat. **3. madde 1 Ekim 2026 itibarıyla adı belli sınırlarla kapandı** ([sözleşme bölümü](docs/RESILIENCE-CONTRACT.tr.md#yol-haritası-3-madde-durumu-2026-10-01-itibarıyla-adı-belli-sınırlarla-kapandı-p01p02p03p05)). Geçici Debian 13 ve Arch konuklarında sahibin başlattığı güncelleme koşuları upd2–upd6 (upd1 hiçbir güncellemeye ulaşmadı) şunları ölçtü: oturum açmış sahip olarak Panel'in güncelleme başlatma API'siyle kurulan iyi aday; tamamlanmadan önce başarısız olan adayın, ikinci bir arızadan sonra da (Debian'da VM sıfırlama, Arch'ta SIGKILL) otomatik olarak önceki sürüme dönmesi; tamamlandıktan sonra başarısız olan adayın üç kez yeniden denenmesi, nedeni korunarak duraklaması ve sahibin yazdırılmış yeniden denemesinden sonra bitmesi; cron'un hiç kesilmemesi ve sitenin yalnız enjekte edilen VM sıfırlaması çevresinde kesilmesi (en çok yaklaşık 22 sn); Panel ve Agent kapalıyken yeniden açılış boyunca site, veritabanı, cron ve güvenlik duvarının hizmet vermeye devam etmesi (SMTP Debian'da) ve yenileme zamanlayıcısının durumunu koruması. Bu koşuların bulduğu ürün kusurları kaynakta kapatıldı. 3. maddenin kapanması hiçbir P0 işini kapatmaz: deney imzası ve loopback kaynağı, az tekrar ve tarayıcı yok, Ubuntu yok, güç kaybı yok, tamamlandıktan sonra geri alma yok, Panel durmuşken Panel'in adresinde canlı durum yok ve gerçek sistemde tetiklenmeyen yollar sözleşme bölümünde adlarıyla kalır. | Kullanıcının arayüzden güncelleme başlatması, başarısız aday, otomatik geri alma, kurtarmadaki ikinci arıza, kimlik doğrulamalı yönlendirme ve korunan hizmetler tek işlemde doğrulanır. İddia edilen her platform/sürüm birleşiminin kanıtı saklanır. |
| 4 | Kesin adayın sürüm incelemesi ve kısa kullanıcı test yolu. **Aday incelemesi tamamlandı; adayın kesin kodu (`f6cdd5a0`) 2–3 Ekim 2026'da güncelleme matrisinin bir tam koşusundan geçti ve yayımlama sahibi bekliyor** ([sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.81.tr.md)). Müşteri arşivi artık düzeneği, test betiklerini ve kanıtı taşımıyor; imzalama adımı bunları taşıyan arşivi reddediyor. Güncelleme yolunun salt-okur incelemesi yedi kusur buldu, yedisi de kaynakta düzeltildi (bileşen testleri; kalan açıklar sözleşme bölümünde listelenir); en önemlisi, v0.1.0-alpha.80'in başlattığı işçinin durum kaydı yazmamasıydı, bu yüzden yeni yönlendirme ilk yükseltmede hiç görünmüyordu. Bu ilk yükseltme sonra, durum başına tek çalıştırmayla, test lisansıyla yeniden derlenen alpha.80 kaynağından (imzalı arşivden değil) Debian 13 ve Ubuntu 24.04'te ölçüldü (iyi aday, sahibin devam ettirmesi, alpha.80'e dönen kusurlu aday); Arch'ta alpha.80'den ölçülmedi. Ubuntu, boştaki bir paket yardımcısının kurulumu engellediğini ve güncelleme başlatmayı reddettirebildiğini ortaya çıkardı; ilk düzeltme gerçek sistemde tutmadı, düzeltilmişi ölçüldü. Sahip için açık olanlar: sürüm numarası; yalnız root ile çalışan paketleme testleri (18), geçici bir makinede veya CI'da; üretim imzalaması ve uyum denetimi; satıcıya ait yayımlama araçlarının arşivde kalıp kalmayacağı; sahip testi; kurulu panellerin her güncellemesi. İnceleme düzeltmelerinden sonra Panel'in posta başlangıç adımlarını ertelenmiş yeniden denemesi eklendi ve ölçüldü, ardından tüm matris son kodda bir kez daha koşuldu (upd13: 20 hücre, aday kusuru yok). Adı belli sınırlar sürüm notlarında; hiçbir P0 işi kapanmadı. **9 Ekim 2026 durumu:** sürüm `v0.1.0-alpha.81` olarak yayımlandı (etiket `a0beb726` üzerinde, 4 Ekim 2026). Sahibin bildirdiği ve burada bir dosyada kayıtlı olmayanlar: yayımlama adımları (#204 numaralı çekme isteğinin birleştirilmesi, etiket, portal yayını), altı imzalı dosyanın doğrulanması ve kurulu iki panelin (biri Ubuntu 24.04, biri Debian 13) 8 Ekim 2026'da panelin kendi güncelleme ekranından güncellenmesi. | Gerekli kabul işleri kapanır veya bilinçli olarak dar kapsamlı sürümün açık sınırları belirtilir. İmzalı dosyalar ve kurtarma uyumu doğrulanır; kurulu panel güncellemesini yalnız kullanıcı başlatır. |
| 5 | `v0.1.0-alpha.82` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümüne bakın). **10 Ekim 2026'da yayımlandı (etiket `2a0af8866` üzerinde; 22 CI işinin 22'si; altı dosya; portalı sahip yayımladı; kurulu iki sunucuyu sahip panelin güncelleme ekranından güncelledi); numara v0.1.0-alpha.82 (sahibin 10 Ekim 2026 kararı).** Yedinci bir gerçek sistem kaydı, yayımlanmış koda karşı gerçek bir Chrome, güncelleme penceresi ve gizli sekme hücrelerini geçti, soğuk sayfa yüklemesinde (`RecoveryAccess` kapısı) kaldı; kabul denetimindeki daraltmalarıyla saklanıyor. 8 ve 9 Ekim 2026'daki dört gerçek sistem koşusu, bir ek ölçüm ve `postconf`'un bir okuması; ayar kayıtlarını, hizmet işlemlerini, istek kimliğini, yayımlanmış v0.1.0-alpha.81'den güncellemeyi (`cfa329676` hâliyle) ve yeni kurulmuş konuklarda son düzeltmeleri (`557b554eb` hâliyle, Arch'ta PHP siteleri dahil) ölçtü; geçmeyen tek denetim, Ubuntu 24.04'te Postfix durdurulduktan sonraki not, düzeltildi ve yeniden ölçüldü. `postconf` düzeltmesi (`67b62cc0f`) gerçek programın bir okumasına ve bileşen testlerine dayanır. Güncelleme matrisi 9 Ekim 2026'da `67b62cc0f` hâlinde, beşinci bir koşuda yinelendi: on hücrenin tamamı, her biri bir kez ve konuk diskleri bellekteyken, üçüncü koşunun ölçtüğü sonuca ulaştı. Ardından son kod (`72b879eea`; Panel'in daraltılmış `Strict-Transport-Security` başlığını ekler, [D-030](docs/DECISIONS.tr.md)) altıncı bir koşuda üç platformda yeni kuruldu, başlığı konuklarda okundu ve platform başına bir güncelleme doğrulandı. Son kodda yinelenmeyenler: ayar kaydı, istek kimliği ve tam hizmet işlemi hücreleri ile yedi güncelleme hücresi. Gerçek bir sistemde çalıştırılmayan: düzeltilmiş posta sertifikası yolu. Sahibin 10 Ekim 2026'da karara bağladıkları: numara; yayımlamadan önce yeni kurulum ölçümü; başlık değişikliği. Sahibin kendi denemesi, kurulu iki sunucunun sahip tarafından panelin güncelleme ekranından başlatılan güncellemesidir; geçici sunucu kullanılmaz. Sahibin 10 Ekim 2026'da yaptıkları: sürüm sırası değişkeni, birleştirme, etiket ve portal yayını; üretim imzalaması etiket üzerinde CI içinde çalıştı ve dosyalar doğrulandı; kurulu iki panelin güncellemesi (sahip başlattı). Sonraki sürüme taşınan: sahibin üretilmiş bir site yapılandırma dosyasındaki düzenlemesinin bir Panel başlangıcında neyle karşılaştığının denetimi ve "Sürüm durumu" altındaki açık liste ("Sonraki sürüme açık işler (10 Ekim 2026, yayımlamadan sonra)"); bu liste kurulu güncellemelerden dört sahip gözlemini ve ilk yükleme ara ekranını içerir. Hiçbir P0 işi kapanmadı ve ilerlemedi. | Beşinci ve altıncı koşular saklanır ([güncelleme matrisi](deploy/e2e/release-recovery/evidence/set5-20261009/README.md), [yeni kurulum ve başlık](deploy/e2e/release-recovery/evidence/set6-20261009/README.md)) ve [sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.82.tr.md) hangi denetimin hangi koda dayandığını söyler. Sürüm, açık kalan sınırlarını belirtir. İmzalı dosyalar ve kurtarma uyumu doğrulanır; kurulu panel güncellemesini yalnız kullanıcı başlatır. |
| 6 | `v0.1.0-alpha.83` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisine bakın). **11 Ekim 2026'da yayımlandı (etiket `0447558e9` üzerinde; etiketin CI'ı 24 iş, başarılı; altı dosya; portalı sahip yayımladı); sahip kurulu iki sunucuyu da 11 Ekim 2026'da ona güncelledi, 12 Ekim 2026'da dışarıdan arayüzünü sunarken gözlendi ve sahibin 12 Ekim 2026 tarihli iki ekran görüntüsünde görüldü (kayıtlı, açık kısımlarıyla); numara v0.1.0-alpha.83 (sahibin 11 Ekim 2026 kararı); D-031 bu sürüme girer.** Yedi gerçek sistem kaydı (yedinciden on üçüncüye, 10 ve 11 Ekim 2026, geçici konuklar, test lisanslı derlemeler, etiketlerin yeniden derlenmiş kaynağından güncellemeler) alpha.81 ve alpha.82 arayüzlerini gerçek bir Chrome'da, adayın ilk yüklemesini ve erişim beklemesini, v0.1.0-alpha.82'nin sahip düzenlemesiyle yaptığını, D-031'in ilk bölümünü, düzeltmelerini, ikinci adımını (havuz, uygulama birimi) alpha.82'den güncellemeyle ve ondan sonraki düzeltmeleri, kaydın aksini söylediği yerler dışında Debian 13, Ubuntu 24.04 ve Arch'ta ölçtü; on üçüncü kayıt yalnız Debian 13 ve Ubuntu 24.04'te koştu. On üçüncü kayıttan sonra yapılan değişikliklerin yalnız bileşen testleri var. Sahibin 11 Ekim 2026'da yaptıkları: sürüm sırası değişkeni 83 yapıldı, satıcının yayın araçları ile iki kurtarma betiğinin arşivde kalacağına karar verildi, birleştirme, etiket, portal yayımı; altı imzalı dosyayı etikette CI üretti ve denetlendi (`main` korumasının okunması zorunlu denetimlerde ad değişikliği gerekmediğini gösterdi). Kurulu paneller: yayımlama kaydı 11 Ekim 2026'da yazıldığında güncellenmemişti; 12 Ekim 2026'da dışarıdan yapılan bir okuma ikisinin de bu sürümün arayüzünü sunduğunu gösterdi, yani sahip ikisini de panelin kendi güncelleme ekranından 11 Ekim 2026'da güncelledi. Sahibin 12 Ekim 2026 tarihli iki ekran görüntüsü, ikisinin de kenar çubuğunda v0.1.0-alpha.83'ü (derleme kimliği sözcüğün ortasından satır kırıyor), ilk sunucuda bir Pano'yu, ikincisinde ise "Setup stopped: its plan was reopened" diyen, son adımı "Stopped" olan ve hiçbir adımı "In progress" olmayan, son denetimi yalnız sahibin değiştirebileceği bir ters DNS (PTR) kaydını bekleyen kurulum sayfasını gösteriyor. Bildirilmeyenler: yeniden başlatma sırasında tam ekran lisans sayfası, güncelleme bildirimi ile kart, kurulum adresine doğrudan yüklemede anlık lisans sayfası, güncellemenin panel içindeki doğrulama sonucu, Agent'ın sürümü, `postconf -n >/dev/null` çıktısı; iki sunucunun da alan adı yok, bu yüzden ilk başlangıcın site dosyası devralması orada devralacak bir şey bulmadı ve Alan Adları satırı görülemez. Hiçbir P0 işi kapanmaz ya da ilerlemez; bu sahibin gözlemidir, kabul kanıtı değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.83.tr.md) hangi denetimin hangi koda dayandığını söyler ve sınırları sayar; kayıtlar `deploy/e2e/release-recovery/evidence/` altındadır. İmzalı dosyalar ve kurtarma uyumu doğrulanır; kurulu panel güncellemesini yalnız kullanıcı başlatır. |
| 7 | `v0.1.0-alpha.84` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisine bakın). **13 Ekim 2026'da yayımlandı (etiket `f0157e6c8` üzerinde; etiketin CI'ı 24 iş, başarılı; altı dosya; portalı sahip yayımladı); kurulu iki sunucuyu da sahip 13 Ekim 2026'da ona güncelledi (16:16 UTC'de dışarıdan yapılan okumada hâlâ v0.1.0-alpha.83'ün arayüz derlemesini sunuyorlardı; 16:59 UTC'de alpha.84'ünkini sunuyorlardı) ve sahibin 13 Ekim 2026 tarihli iki ekran görüntüsünde görüldü (kaydedildi, açık kısımlarla); numara v0.1.0-alpha.84 (sahibin 13 Ekim 2026 kararı); D-032 bu sürüme girer.** Dashboard'ın "Sistem servisleri" kartı bileşenleri kendiliğinden kontrol eder, sunucu bir taramanın yaşını ve lisansın kalan süresini söyler, ulaşılamayan Agent adıyla söylenir. Dört gerçek sistem kaydı (on dördüncüden on yediye, 12 ve 13 Ekim 2026, geçici konuklar, test lisanslı derlemeler, gerçek başsız Chrome) bunu ölçtü: on yedinci, sürüm commit'inin kendisinde, Debian 13 ve Ubuntu 24.04'e temiz kurulumları ve yalnız Debian 13'te alpha.83'ten güncellemeyi kapsadı ve iki açık madde bıraktı. Sahibin 13 Ekim 2026'da yaptıkları: #207 sonra #208'in birleştirilmesi, sürüm sırası değişkeninin 84 yapılması, etiket, portal yayımı; altı imzalı dosyayı etikette CI üretti ve denetlendi. Süreç notu: etiket, `main` commit'indeki CI koşusu sürerken gönderildi (koşu sonradan geçti; aynı ağaç çekme isteğinde 24 işin 24'ünü geçmişti); (düzeltme 2026-10-14: yayımlayan iş paketleme işine, o da aynı koşunun her test ve derleme işine bağlıdır; yani bir etiketin dosyaları ancak o koşu geçtikten sonra yayımlanır; `main` koşusu ile etiket arasındaki sıraya dair not geçerlidir). Kurulu paneller: yayımlama kaydı yazıldığında güncellenmemişti; 13 Ekim 2026'da dışarıdan yapılan bir okuma ikisinin de bu sürümün arayüzünü sunduğunu gösterdi; yani sahip ikisini de panelin kendi güncelleme ekranından güncelledi. Sahibin iki ekran görüntüsü ikisinin kenar çubuğunda v0.1.0-alpha.84'ü (yapı kimliği hâlâ sözcüğün ortasından satır kırıyor), ilk sunucuda "System services" kartı hiçbir şeye basılmadan "5 / 5 running / installed" diyen bir Dashboard'u, ikincisinde alpha.83'ten beri değişmeyen kurulum sayfasını gösteriyor; son denetimi yalnız sahibinin değiştirebileceği bir ters DNS (PTR) kaydını bekliyor. Bildirilmeyenler: güncellemenin panel içindeki doğrulama sonucu, Agent'ın sürümü, güncelleme sırasında tam ekran lisans sayfası, `postconf -n >/dev/null` çıktısı; iki sunucunun da alan adı yok, bu yüzden D-031 sorusu kurulu sunucularda açık kalır. Hiçbir P0 işi kapanmaz ya da ilerlemez; yayımlanma ürün kabulü değildir ve bu sahibin gözlemi ile dışarıdan bir okumadır, kabul kanıtı değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.84.tr.md) hangi denetimin hangi koda dayandığını söyler ve sınırları sayar; kayıtlar `deploy/e2e/release-recovery/evidence/` altındadır. İmzalı dosyalar ve kurtarma uyumu doğrulanır; kurulu panel güncellemesini yalnız kullanıcı başlatır. |
| 8 | `v0.1.0-alpha.85` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisine bakın). **15 Ekim 2026'da yayımlandı (etiket `36c33c45` üzerinde; etiketin CI'ı 24 iş, başarılı; altı dosya; portalı sürümü hazırlayan kişi yayımladı); yayımlama kaydı yazılırken kurulu paneller güncellenmemişti, sonra sahip onları panelin kendi güncelleme ekranından güncelledi (15 Ekim 2026 bildirimi, 09:58Z'de kaydedildi); v0.1.0-alpha.85 numarasını sahibin 13 Ekim 2026 tarihli kalıcı talimatı uyarınca sürümü hazırlayan seçti (sürüm hazırlığı `5443fe055`, 84'ten sonra sıra 85); D-033 bu sürüme girer.** Sahibin kurulu ikinci sunucudaki son kurulum adımından sonra 14 Ekim 2026'da koyduğu kural: sahibi bekleten bir ekran beklediğini gösterir ya da yapılacak bir şey olmadığını söyler ve bir sınırdan sonra dürüst bir "henüz bilinmiyor" ile biter. Kurulum sahibin geçen kontrolüyle tamamlanır ve ters DNS beklemesini adıyla söyler; 37 bekleme durumunun denetimi 19'unu değiştirdi (önce 16, sonra sahibin aynı gün karar verdiği 3); alpha.84'ün O4–O6'sı, Posta paketleri kartı, türlü posta profili cümleleri, sunucunun saatiyle HSTS'nin kalan süresi ve kenar çubuğundaki yapı kimliği düzeltildi; Agent eski bir PTR yanıtında sonraki çözümleyiciye sorar. Bir gerçek sistem kaydı (on sekizinci, set16, 14 Ekim 2026, geçici konuklar, bir sabit commit'in test lisanslı derlemeleri, gerçek başsız Chrome) Debian 13'e temiz kurulumu ve Debian 13 ile Ubuntu 24.04'te alpha.84'ten sağlam ve kusurlu güncellemeyi ölçtü (`update_verified`; `recovered` / `rollback_verified`; geri dönüşten sonra defter `equal-except-volatile`, yalnız `observed_at`). Gerçek sistemde ulaşılamayan: "Kurulum tamamlanıyor" ve güncelleme kartının meşgul durumu. 15 Ekim 2026'da yapılanlar: #210 numaralı çekme isteğini sahip squash ile birleştirdi; `main` commit'indeki koşu, `actions/setup-go@v6` indirilemediği için iki işte hiçbir adım çalışmadan bir kez başarısız oldu, sürümü hazırlayan kişi onu yeniden çalıştırdı ve başarılı oldu, etiket ancak ondan sonra gönderildi (amaçlanan sıra izlendi); sahip sürüm sırası değişkenini 85 yaptı ve etiketi gönderdi; CI altı dosyayı imzaladı; dosyaları ve portal paketini hazırlayan kişi bir indirmeden denetledi; portalı hazırlayan kişi yayımladı (ilk çağrı, `$PSScriptRoot` bir parametre varsayılanında boş olduğundan hiçbir aktarım olmadan parametre bağlama aşamasında durdu: sonraki sürüm için açık). Sahibin güncellemelerden sonraki bildirimi (ekran görüntüleri, dışarıdan bir okuma, kabul kanıtı değil): iki sunucu da v0.1.0-alpha.85 gösteriyor ve kenar çubuğundaki yapı kimliği tek satırda; Frankfurt'un güncelleme sayfası, güncellemenin kendi beklenen yeniden başlatması sırasında uyarı renginde bir "could not be read just now / not known whether it has finished" bildirimi ve bir toast gösterdi, sahip bunun ortada sorun yokken sorun gibi okunduğunu söylüyor (sonraki sürüm için D-033 ve D-024 altında açık: beklenen pencerede T+ sayacıyla sakin ilerleme, uyarı ve "bilinmiyor" ifadesi yalnız pencereden sonra ya da doğrulanmış başarısızlıkta, tek bildirim; `web/src/components/SystemUpdateOperation.tsx`, `PanelUpdateCard.tsx`); Boston'un Panosu 5 / 5 hizmeti ve "Nothing CelikPanel read needs action" yazısını gösterdi (önceki başarısız okumanın bilinen nedeni yok); Frankfurt'un Panosu "Checking…" durumlarını gösterdi, sonuç yakalanmadı; "Recent activity"de ham denetim kimlikleri duruyor. Bildirilmeyenler: tam ekran lisans sayfası, Agent'ın sürümü, Frankfurt'un güncellemeden sonraki kurulum son adımı, `postconf -n >/dev/null`. Sürümü hazırlayan kişi hiçbir kurulu sunucuyu ölçmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; yayımlanma ürün kabulü değildir ve set16 bir ölçümdür, kabul kanıtı değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.85.tr.md) hangi denetimin hangi koda dayandığını söyler ve sınırları sayar; kayıtlar `deploy/e2e/release-recovery/evidence/` altındadır. İmzalı dosyalar ve kurtarma uyumu doğrulanır; kurulu panel güncellemesini yalnız kullanıcı başlatır. |
| 9 | `v0.1.0-alpha.86` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisine bakın). **15 Ekim 2026'da yayımlandı (etiket `8d260f31` üzerinde; etiketin CI'ı 24 iş, başarılı; altı dosya; portalı sürümü hazırlayan kişi yayımladı); yayımlama kaydı yazılırken kurulu paneller güncellenmemişti; v0.1.0-alpha.86 numarasını sahibin 13 Ekim 2026 tarihli kalıcı talimatı uyarınca sürümü hazırlayan seçti (sürüm hazırlığı `313f3eea2`, 85'ten sonra sıra 86); D-033 ve D-024 uygulanır, yeni karar yok.** Sahibin 15 Ekim 2026'da alpha.85'e gerçek güncelleme sırasındaki gözlemi: güncellemenin kendi yeniden başlaması, hiçbir sorun yokken durumun bilinmediğini söyleyen bir uyarı ve köşe bildirimiyle gösterildi. Aday o yeniden başlamayı sakin ilerleme olarak gösterir, uyarıyı ve "bilinmiyor" metnini bir sınırdan (başlangıçtan 6 dakika, ilk kopan okumadan en az 2 dakika) ya da doğrulanmış bir hatadan sonraya bırakır ve bunu tek bildirimle söyler. Ayrıca: yayımlama betiğinin göreli yol kusuru; okumalar yalnız sekme görünürken; set16 O1–O3 (sayfa dili, Türkçe 27 bileşen adı ve açıklaması, cümle olarak 179 denetim eylemi); `system` kullanıcı adı ayrıldı; hiçbir cümle yeniden tarama istemez ve bir bileşenin kendi sayfası doğrulanamayan kaydı kendisi kontrol eder; posta profili cümlelerinde platform ve rol adları; ters DNS cümlesinin arkasındaki çözümleyici (Agent güncellemesi ister); Mağaza'nın "Durum henüz bilinmiyor"u; gerçek sistem kayıtlarından sonra lisans kapısı ve oturum kapısı güncellemenin yeniden başlaması boyunca sessiz kalır (set17 D1) ve gezinti kilidi yeniden başlamayı en geç başlangıçtan 6 dakikaya dek kapsar (set17b F1, sürümü hazırlayanın D-033 altındaki kararı, sahibin kararı değil); üçü de yalnız web'de. Bileşen testleri (`7265d3c99`'da web 1483/1483) ve taklide karşı 13.–15. tarayıcı turları; gerçek sistem kayıtları [set17](deploy/e2e/release-recovery/evidence/set17-20261015/README.md) (`313f3eea2`'de) ve [set17b](deploy/e2e/release-recovery/evidence/set17b-20261015/README.md) (`ecd2a5f08`'de; bir laboratuvar gecikmesiyle yapılmış yaklaşık 3 dakikalık yeniden başlama kesintisi): adayın sayfası beş aday koşusunun hepsinde "bilinmiyor" metni olmadan sakin kaldı, alpha.85'in sayfası onu gösterdi; set17 D1 set17b'nin iki koşusunda yeniden üretilmedi (mekanizma kanıtlanmadı); set17b'den sonraki F1 değişikliği ve `PANEL_STARTING` yolu gerçek sistemde ölçülmedi. 15 Ekim 2026'da yapıldı, hepsini sürümü hazırlayan kişi sahibin talimatıyla yaptı: #213 squash olarak birleştirildi; `main` commit'indeki koşu başarılı oldu (24 iş) ve etiket ancak ondan sonra gönderildi (öngörülen sıra izlendi); sürüm sırası değişkeni 85'ten 86'ya çıkarıldı; CI altı dosyayı imzaladı; dosyaları ve portal paketini hazırlayan kişi bir indirmeden denetledi; portalı yayımladı (çağrı mutlak yolla yapıldı, bu yüzden göreli yol düzeltmesi onunla denenmedi) ve genel siteyi dışarıdan okudu. Sınırlar: etiket dosyaları Windows'ta `git archive` ile, canlı sitenin kopyasında terfi aracı provası yok, alpha.85'in paketiyle dosya dosya karşılaştırma yok, yayımlayıcının kendi testi çalıştırılmadı. Sahibin güncellemeden sonra bildireceği: iki sunucunun güncelleme sonucu ve güncelleme sayfasının yeniden başlama sırasında gösterdiği (sakin bildirim, kapı iletişim kutusunun olmaması, kilit), tam ekran lisans sayfası, Agent'ın sürümü, Frankfurt'un kurulum son adımı, `postconf -n >/dev/null`. Hiçbir kurulu sunucu ölçülmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; yayımlanma kabul olmaz. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.86.tr.md) hangi denetimin hangi koda dayandığını söyler ve sınırları sayar; kayıtlar `deploy/e2e/release-recovery/evidence/` altındadır. İmzalı dosyalar ve kurtarma uyumu doğrulanır; kurulu panel güncellemesini yalnız kullanıcı başlatır. |
| 10 | `v0.1.0-alpha.87` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisine bakın). **16 Ekim 2026'da yayımlandı (etiket `a8cddcb5`'te; etiketteki CI 24 iş, başarılı; altı dosya; portalı sürümü hazırlayan kişi yayımladı); yayımlama kaydı yazılırken kurulu panellerin güncellendiği bildirilmemişti; v0.1.0-alpha.87 numarasını sahibin 13 Ekim 2026 tarihli kalıcı talimatı uyarınca sürümü hazırlayan seçti (sürüm hazırlığı `471377d60`, 86'dan sonra sıra 87); D-021 ve D-033 uygulanır, yeni karar yok.** Tam eş olarak ikinci kurulu sunucunun birinci bölümü: tamamlanmış bir kurulum genişletilebilir (`POST /api/v1/setup/extend`, "Bu sunucunun kurulumunu genişletin", neyin değiştiğini, neyin kurulacağını ve neyin kaldığını gösteren bir inceleme, "Genişletmeden vazgeç" hiçbir şey kaldırmaz); hazırlanmış sunucuda Bileşenler postası kurulumun adımlarından ve denetimlerinden geçer; kayıtlı posta sunucusu adı düzeltilebilir ve reddedilen bir değişiklik onu olduğu gibi bırakır; ikincil DNS sunucusu "DNS'i yapılandır" yerine birincili ve tam kaydı adlandırır (`mail_dns_guidance`); her açık denetim için tek türlü cümle; set18'den sonra genişletme artık tamamlanmış çalışmayla açılmaz (D1, `superseded`), başlayan çalışma ilk adımını adlandırır (O1) ve basılan devam tek uçuşlu kalır (O3). Bileşen testleri (web 1499/1499; `bef12820e`'de `./cmd/panel` 1558 geçti, 6 atlandı, 1675 alt test; Agent değişmedi) ve 16. tarayıcı turu (8 yapılandırmada 36 durum, yedi kusur düzeltildi); Debian 13'te kendi DNS'i ve ACME'si olan bir laboratuvarda gerçek sistem kayıtları [set18](deploy/e2e/release-recovery/evidence/set18-20261015/README.md) (`5d54aba1e`'de) ve [set18b](deploy/e2e/release-recovery/evidence/set18b-20261016/README.md) (`6b363ac46`'da): kurulum ve Core Mail genişletmesi hazır duruma, D1 set18'de kaydedildi ve set18b'de O1 ve O3 ile düzeldiği gözlendi, yeni kusur yok. Açık: ikinci bölümün sahibe ait üç kararı (o zamana dek Boston'un posta kaydı Frankfurt'ta ya da DNS sağlayıcısında elle yapılır); ikincil DNS kutularına gerçek sistemde ulaşılmadı; genel ACME/DNS yolu, Ubuntu ve Arch koşulmadı; teslim almadaki ham dosya gözlemleri (FQDN günlük satırı, `cron DEATH`, kendinden imzalı yedek bildirimi, kurulumun yeniden başlaması çevresindeki erişim şeridi) değerlendirilmedi. 16 Ekim 2026'da yapılanlar, hepsi sahibin talimatıyla sürümü hazırlayan kişi tarafından: #215 numaralı çekme isteği squash ile birleştirildi; `main` commit'indeki koşu başarılı oldu (24 iş) ve etiket ancak ondan sonra gönderildi (öngörülen sıra izlendi); sürüm sırası değişkeni 86'dan 87'ye çıkarıldı; CI altı dosyayı imzaladı; dosyaları ve portal paketini hazırlayan kişi bir indirmeden denetledi; portalı hazırlayan kişi yayımladı ve genel siteyi dışarıdan okudu. Sınırlar: etiket dosyaları Windows'ta `git archive` ile, canlı sitenin bir kopyasında terfi aracı provası yok, alpha.86'nın paketiyle dosya dosya karşılaştırma yok, yayımlayıcının kendi testi çalıştırılmadı. alpha.86'dan yaklaşık 5,5 saat sonra ve sahip herhangi bir sunucuyu alpha.86'ya güncellediğini bildirmeden önce yayımlandı; güncelleyicinin sıra tabanı alpha.85'ten ya da alpha.86'dan doğrudan alpha.87'ye güncellemeye izin verir (koddan okundu, ölçülmedi). Sahibin güncellemeden sonra bildireceği: iki sunucunun güncelleme sonucu (hangi sürümden olursa olsun), güncelleme sayfasının yeniden başlama sırasında gösterdiği, tam ekran lisans sayfası, Agent'ın sürümü, Frankfurt'un kurulum son adımı, `postconf -n >/dev/null` ve Boston'un hazır ekranında "Bu sunucunun kurulumunu genişletin" seçeneğinin görünüp görünmediği. Hiçbir kurulu sunucu ölçülmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; yayımlanma kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.87.tr.md) hangi denetimin hangi koda dayandığını söyler ve sınırları sayar; kayıtlar `deploy/e2e/release-recovery/evidence/` altındadır. İmzalı dosyalar ve kurtarma uyumu doğrulanır; kurulu panel güncellemesini yalnız kullanıcı başlatır. |
| 11 | `v0.1.0-alpha.88` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisine bakın). **16 Ekim 2026'da yayımlandı (etiket `1cb3628f`'te; etiketin koşusu başarılı; altı dosya; portalı sürümü hazırlayan kişi yayımladı); hiçbir kurulu sunucu onu çalıştırmadı, çünkü ikisi de doğrudan alpha.89'a geçti; v0.1.0-alpha.88 numarasını sahibin 13 Ekim 2026 tarihli kalıcı talimatı uyarınca sürümü hazırlayan seçti (sürüm hazırlığı `7231afdbf`, 87'den sonra sıra 88); D-024, D-032 ve D-033 uygulanır, yeni karar yok.** Sahibin Frankfurt'u alpha.85'ten alpha.87'ye güncelledikten sonraki 16 Ekim 2026 tasarımı: güncellemenin her adımı için bir çubuk (indirme, doğrulama, hazırlık, tek adım olarak kurma ve yeniden başlatma, onay), tek sakin cümleli planlı bir adım olarak yeniden başlama, sonda "{version} sürümüne güncellendi", gecikme ya da hata metni yalnız 6 dakikalık pencereden sonra ya da doğrulanmış hatada, tek bildirim, adımların yanında kısa bir kilit cümlesi. Agent ilerleme okuması, Panel planı ve yeniden başlama ölçümü (`panel_settings` `panel_update_last_restart`), sayfa adımları. Bileşen testleri (web 1523'te 1523; Go panel ve transport 1602/1695/6, Agent 1805/2838/29) ve tarayıcı turu 17 (9 durum × 8, bir kusur düzeltildi). Sahibin talimatıyla gerçek sistem kaydı yok; adımların ilk kez alpha.88'den güncellemede görülmesi hedeflenmişti, bu yüzden alpha.89 aynı kodla geldi. Hiçbir kurulu sunucu ölçülmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; yayımlanma kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.88.tr.md) hangi denetimin hangi koda dayandığını söyler ve sınırları sayar. İmzalı dosyalar ve kurtarma uyumu doğrulanır; kurulu panel güncellemesini yalnız kullanıcı başlatır. |
| 12 | `v0.1.0-alpha.89` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisine bakın). **16 Ekim 2026'da yayımlandı (etiket `ac36fccf`'te; etiketin koşusu başarılı; altı dosya; portalı sürümü hazırlayan kişi yayımladı); numarayı sahibin 13 Ekim 2026 tarihli kalıcı talimatı uyarınca sürümü hazırlayan seçti; sahibin 16 Ekim 2026 talimatıyla alpha.88'in hemen ardından çıktı.** alpha.88 ile aynı ürün kodu (`git diff --stat 1cb3628ff HEAD -- cmd internal web` boştur). Sahip iki kurulu sunucuyu da panelin kendi güncelleme ekranından, önceki sürümlerinden doğrudan buna güncelledi (Frankfurt alpha.87'den); ikisi de sürümü, commit'i ve beş tamamlanmış adımlı bir kartı gösterir (iki ekran görüntüsü, yaklaşık 09:30Z). Yeniden başlama sırasındaki canlı adım görünümü görülmedi (güncelleme önceki sürümün sayfasında çalıştı): ilk kez bir sonraki güncellemede, alpha.89'dan görülür. İyi bir durumun ("yeni güncelleme yok") uyarı simgesiyle çizilmesi sonraki sürüm için D-033 maddesidir. Sahibin talimatıyla gerçek sistem kaydı yok. Lisans sayfası, Agent sürümü, Frankfurt'un kurulum son adımı, Boston'un genişletme seçeneği ve `postconf -n` bildirilmedi. Hiçbir P0 işi kapanmaz ya da ilerlemez; yayımlanma kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.89.tr.md) geri kalan her şey için alpha.88 notlarına bakar. İmzalı dosyalar ve kurtarma uyumu doğrulanır; kurulu panel güncellemesini yalnız kullanıcı başlatır. |
| 13 | `v0.1.0-alpha.90` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisine bakın). **16 Ekim 2026'da yayımlandı (etiket `1743ac69` üzerinde; etiket koşusu başarılı; altı dosya; portalı sürümü hazırlayan kişi yayımladı); numarayı sahibin 13 Ekim 2026 tarihli kalıcı talimatı uyarınca sürümü hazırlayan seçti; sahibin 16 Ekim 2026 talimatıyla ("90'ı çıkar") çıkar.** Bir D-033 düzeltmesi: yeni güncelleme bulamayan denetim uyarı üçgeni yerine onay işaretiyle "En son sürümü kullanıyorsunuz." der; kartın diğer iyi ya da nötr durumları artık uyarı gibi çizilmez. Sahip iki kurulu sunucuyu da panelin kendi güncelleme ekranından bu sürüme güncelledi: Boston temiz (kart "güncellendi", sonra yeşil "En son sürümü kullanıyorsunuz."); Frankfurt son adımda yenileme zamanlayıcısının telafi çalışması yüzünden başarısız göründü ve 16 sn sonra kendi kurtarmasıyla bitti (neden ve düzeltme alpha.92'de); tarayıcı sert yeniden yüklemeye kadar alpha.89 arayüzünü korudu ve sonra "güncellendi" kartı yoktu (ekran görüntüleri). Canlı adım görünümü görüldü diye bildirilmedi. Sahibin talimatıyla gerçek sistem kaydı yok. Sürümü hazırlayan kişi hiçbir kurulu sunucuyu ölçmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; yayımlanma kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.90.tr.md). İmzalı dosyalar ve kurtarma uyumu doğrulanır; kurulu panel güncellemesini yalnız kullanıcı başlatır. |
| 14 | `v0.1.0-alpha.91` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisine bakın). **16 Ekim 2026'da etiketlendi (etiket `356109b5` üzerinde; etiket koşusu başarılı; GitHub'da altı dosya), bilerek indirme portalına konmadı, bu yüzden hiçbir kurulu sunucu onu çalıştırmadı; numarayı sahibin 13 Ekim 2026 tarihli kalıcı talimatı uyarınca sürümü hazırlayan seçti.** İkinci sunucu tam eş olarak, 2. bölüm (boşluk 4, 5 ve 6): altyapı bölgesi kayıt düzenleyicisiyle Alan adları girdisi olarak, eşin posta sunucusu adı birincinin planında ve ana makine kaydının yetkili uzak DNS bağlantısı üzerinden birinciden istenmesi; bağlantı yoksa ya da ret varsa elle girilecek kesin kayıt gösterilir. Otomatik kayıt için iki kurulu panel arasındaki kanal gerekir. Sunuculara alpha.92 ile ulaşır, çünkü Frankfurt'un alpha.89'dan alpha.90'a güncellemesi bu sürümde de bulunan bir güncelleyici kusurunu gösterdi. Sahibin 16 Ekim 2026 talimatıyla gerçek sistem kaydı yok. Hiçbir kurulu sunucu ölçülmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; yayımlanma kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.91.tr.md). İmzalı dosyalar ve kurtarma uyumu doğrulanır; kurulu panel güncellemesini yalnız kullanıcı başlatır. |
| 15 | `v0.1.0-alpha.92` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisine bakın). **16 Ekim 2026'da yayımlandı (etiket `961f2cb2` üzerinde; etiket koşusu başarılı; altı dosya; portalı sürümü hazırlayan kişi önceki sürüm alpha.90 ile yayımladı, çünkü canlı portal alpha.91'i hiç sunmadı); numara, alpha.91'in portala konmaması yönündeki 16 Ekim 2026 kararından gelir.** Güncelleyicinin Certbot yenileme düzeltmesi (Frankfurt'un 16 Ekim 2026 güncellemesi), tipli `update_finishing_scheduler` kodu, sakin "Güncelleme tamamlanıyor" görünümü ve kurtarmayla gelen başarının "güncellendi" olarak gösterilmesi, temiz kopyalanan yapı satırı; alpha.91'in ikinci sunucu işini de getirir. Bir yerel laboratuvar kaydı, set19. Sahip iki kurulu sunucuyu da alpha.90'dan bu sürüme panelin kendi güncelleme ekranından güncelledi; ikisi de sürümü, commit'i ve beş tamamlanmış adımlı bir kartı gösterir, Frankfurt da hata ve kurtarma olmadan (iki ekran görüntüsü, yaklaşık 19:10Z). Güncelleme sırasındaki canlı adım görünümü, Frankfurt'un Alan adları'ndaki altyapı bölgesi satırı, Boston'un posta genişletmesi, lisans sayfası, Agent'ın sürümü ve `postconf -n` bildirilmedi. Sürümü hazırlayan kişi hiçbir kurulu sunucuyu ölçmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; yayımlanma kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.92.tr.md). İmzalı dosyalar ve kurtarma uyumu doğrulanır; kurulu panel güncellemesini yalnız kullanıcı başlatır. |
| 16 | `v0.1.0-alpha.93` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisine bakın). **16 Ekim 2026'da yayımlandı (etiket `0897ee92` üzerinde; altı dosya; portalı sürümü hazırlayan kişi yayımladı); numarayı, sahibin 13 Ekim 2026 tarihli kalıcı talimatıyla sürümü hazırlayan seçti.** alpha.92'deki kurulu sunuculardan altı küçük düzeltme: doğrulanmış daha büyük bir profille doğrulanan Temel Posta, sakin bir "Web postayı onar" düğmesi, doğrulanmış posta için düz sözler, hiç posta bileşeni olmayan posta paketi için "Kurulu değil", Son etkinlikte profil adları, doğru söylenen sayılar. Sahibin 16 Ekim 2026 talimatıyla gerçek sistem kaydı yok. Sürüm zincirinin ilk koşusu yanlış commit'i etiketledi ve henüz hiçbir sürüm yokken CI tarafından reddedildi; ikinci koşu bu sürümü üretti (ayrıntı sürüm notlarında). Boston ve Frankfurt bu yazılırken alpha.92'deydi, henüz güncellenmemişti; kurulu bir sunucudan hiçbir şey kaydedilmedi. Sürümü hazırlayan kişi hiçbir kurulu sunucuyu ölçmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; yayımlanma kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.93.tr.md). İmzalı dosyalar ve kurtarma uyumu doğrulanır; kurulu panel güncellemesini yalnız kullanıcı başlatır. |
| 17 | `v0.1.0-alpha.94` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisine bakın). **17 Ekim 2026'da yayımlandı (etiket `d506a05e` üzerinde; altı dosya; portalı sürümü hazırlayan kişi yayımladı); numarayı, sahibin 13 Ekim 2026 tarihli kalıcı talimatıyla sürümü hazırlayan seçti.** Her Bileşenler kategorisinin başında Hazır paketler ve Panoda tek bir Hazır paketler kartı (D-034), Bileşenler renk düzeltmeleriyle. Sahibin 16 Ekim 2026 talimatıyla gerçek sistem kaydı yok. Sürüm zinciri temiz çalıştı. Boston ve Frankfurt bu yazılırken alpha.92'deydi, henüz güncellenmemişti; kurulu bir sunucudan hiçbir şey kaydedilmedi. İncelemede bulunan üç küçük iş açık bırakıldı (sürüm notları, "Yapılmayanlar"). Sürümü hazırlayan kişi hiçbir kurulu sunucuyu ölçmedi; paket sırası gerçek bir Agent'a karşı çalışmadı; hiçbir P0 işi kapanmaz ya da ilerlemez; yayımlanma kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.94.tr.md). İmzalı dosyalar ve kurtarma uyumu doğrulanır; kurulu panel güncellemesini yalnız kullanıcı başlatır. |
| 18 | `v0.1.0-alpha.95` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisine bakın). **17 Ekim 2026'da yayımlandı (etiket `5f3cb1a2` üzerinde; altı dosya; portalı sürümü hazırlayan kişi yayımladı); numarayı, sahibin 13 Ekim 2026 tarihli kalıcı talimatıyla sürümü hazırlayan seçti.** Her sunucuda aynı hazır paket listesi, her çipte durumuyla (sahibin 17 Ekim 2026 kararı, D-034 değişikliği), Pano kartı ve çipler için tek sayma kuralı, alpha.94'ün açık üç inceleme maddesi kapandı, satır eylemleri için tek çerçeveli simge düğmesi ve dur işaretli Durdur simgesi, çizgilerden oluşan sürüm sütunu yok, Roundcube kalıntılarını temizleme (satırda yalnız Panel'in yarım kalmış kurulum ya da kaldırma kaydıyla sunulur), kutu, kenar çubuğu rozeti ve sayfa için tek Alan adları sayısı, sade kenar çubuğu sayıları ve bağımsız incelemenin on düzeltmesi (web 1599 ve Go 7124 test geçti; tarayıcı 20. grubu inceleme düzeltmelerinden sonra yeniden çalıştırılmadı). Şema, Agent ya da güncelleyici değişikliği yok, bu yüzden gerçek sistem kaydı yok. Açık: her bileşen için kurulu paket sürümleri okunmuyor ve beş küçük iş sonraki bir sürüme bırakıldı (sürüm notları, "Yapılmayanlar"). Sürüm zinciri temiz çalıştı. Sahip Boston ve Frankfurt'u alpha.94'e güncelledi ve bu sürümün düzelttiği ekran görüntülerini gönderdi; kurulu bir sunucuda alpha.95'ten hiçbir şey kaydedilmedi. Sürümü hazırlayan kişi hiçbir kurulu sunucuyu ölçmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; yayımlanma kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.95.tr.md). İmzalı dosyalar ve kurtarma uyumu doğrulanır; kurulu panel güncellemesini yalnız kullanıcı başlatır. |
| 19 | `v0.1.0-alpha.96` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisine bakın). **17 Ekim 2026'da yayımlandı (etiket `08faa91e` üzerinde; altı dosya; portalı sürümü hazırlayan kişi yayımladı); numarayı, sahibin 13 Ekim 2026 tarihli kalıcı talimatıyla sürümü hazırlayan seçti.** Panel daha yeni imzalı bir sürümü kendiliğinden arar (başladıktan yaklaşık 2 dakika sonra, sonra her 12 saatte bir; salt-okur, sınırlı, geri çekilmeli, Ayarlar'dan kapatılabilir; D-035) ve yöneticilere sakin bir dille söyler: kenar çubuğunda küçük bir bağlantı, "Sonra hatırlat" olan tek sessiz Pano satırı, Ayarlar gezinmesinde bir nokta; asla alarm rengi yok ve başarısız denetim için güncelleme kartının dışında hiçbir şey yok. Güncelleme kartı güncellemede yardımcı olmak için yeniden kuruldu: tek durum satırı, en çok tek bildirim, sade bir "Güncellemeden önce" listesi, tek dolu düğme. Hiçbir şey kendiliğinden güncellenmez. Bilinmeyen iki salt-okur denetim adımı için metinler, genel bir nedenin altında sunucunun ilk tanı satırı ve alpha.95'in dört açık işi kapandı. Sıra 96, şema 44, geçiş yok; Agent ya da güncelleyici değişikliği yok, bu yüzden gerçek sistem kaydı yok. Açık: kurulu bir sunucuda hiçbir şey ölçülmedi ve zamanlanmış denetim gerçek bir Agent'a ya da gerçek sürüm sunucusuna karşı çalışmadı; Frankfurt'un alpha.95 salt-okur denetim durmasının nedeni bilinmiyor (Ayrıntılar metni sahipten bekleniyor); her bileşen için kurulu paket sürümleri okunmuyor. Aynı gün bağımsız bir incelemeden sonra düzeltildi: keşif, bir güncelleme kabul edilmiş ya da sürüyorken asla yazmaz ve atlanma yalnız bellektedir. Son ağaç: Go 7147 geçti, 0 başarısız, 43 atlandı; web 1623 geçti, 0 başarısız; derleme bütçesi içinde. Sürüm zincirinde hiçbir sürümü etkilemeyen iki olay oldu (güncelleme kartının metnini yansıtan bir deneme düzeneği güncellenmemişti; bir yardımcının çalışmasından kalan başıboş bir git ayarı sürümü hazırlayanın makinesinde git'i bir süre durdurdu); ikisi de düzeltildi ve zincir yeniden çalıştırıldı. Sahip Boston ve Frankfurt'un alpha.95'e güncellemesini başlattı; Frankfurt'unki salt-okur denetimde durdu ve alpha.94'te kaldı (neden bilinmiyor, Ayrıntılar bekleniyor), Boston'un sonucu bildirilmemişti. Kurulu bir sunucuda alpha.96'dan hiçbir şey kaydedilmedi ve alpha.96, hâlâ alpha.95'te olan bir panele kendini duyuramaz. Ayrıca açık: Türkçe ekran katalogları neredeyse dolu; sunucu meşgulken bellekte tutulan elle denetim sonucu yeniden başlatmada kaybolur; tarayıcı 21. grubu son inceleme düzeltmelerinden sonra yeniden çalıştırılmadı. Sürümü hazırlayan kişi hiçbir kurulu sunucuyu ölçmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; P0.1–P0.5 kısmi kalır; yayımlanma kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.96.tr.md). Herhangi bir yayımdan önce imzalı dosyalar ve kurtarma uyumu doğrulanır; kurulu panel güncellemesini yalnız kullanıcı başlatır. |
| 20 | `v0.1.0-alpha.97` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisine bakın). **21 Ekim 2026'da yayımlandı (etiket `0c1a8917` üzerinde; altı dosya; portalı sürümü hazırlayan kişi yayımladı); numarayı, sahibin 13 Ekim 2026 tarihli kalıcı talimatıyla sürümü hazırlayan seçti.** Paket veritabanının salt-okur bir Agent sorgusundan her bileşen için kurulu paket sürümleri (D-036): Bileşenler satırlarında kısa biçim, her bileşen sayfasının başlığında kaynağıyla tam sürüm, paketler farklıysa "birden çok sürüm", bilinmiyorsa hiçbir şey, paket yöneticisi meşgulken önceki okuma korunur ve işaretlenir. Ekran kataloğu kendi bütçeleri olan (en az %25 pay) dokuz parçaya bölündü. Frankfurt'un alpha.95 durmasının meşgul bir paket yöneticisi (Debian'ın otomatik paket güncellemeleri) olduğu doğrulandı; durdurma penceresi ve güncelleme kartı artık bunu söylüyor, bir salt-okur denetim durması sınırlı, temizlenmiş bir tanı satırı tutuyor, `compatibility` adımının sözleri var ve bir adımın nedeni genel neden yerine kendi sözlerini bekliyor. Tarayıcı taklitleri sürümleri yalnız yeni alanların bildireceği biçimde bildiriyor; 22. grup eklendi. Bir Agent değişikliği (salt-okur yetenek ve RPC politikası); güncelleyicinin değiştirme yolu değişmedi, bu yüzden gerçek sistem kaydı yok. Sıra 97, şema 44, geçiş yok. Sürümden önce üç bağımsız inceleme turu yapıldı ve sürüm zinciri sorunsuz çalıştı. Sahibin 21 Ekim 2026 tarihli ekran görüntüsü Boston'un alpha.96'da olduğunu gösteriyor; Frankfurt'un alpha.95 durmasının saklanan satırı, paket yöneticisinin meşgul olduğunu doğruladı; sahip ayrıca GitHub hesabının adını değiştirdi (depo artık `alicelik77/celikpanel`). Açık: sürümler hiçbir kurulu sunucuda ölçülmedi (açık iş ancak sahip bunları Boston ve Frankfurt'ta gördüğünde kapanır); kritik açılış yolunda 1,8 KiB bütçe kaldı; genel portal hâlâ `celikbros/...` GitHub adreslerine bağlanıyor (yönlendirilirler); Ayarlar menü düzeni önerisi sahibin kararını bekliyor; Frankfurt'un alpha.95 durmasından sonraki güncellemesi bildirilmedi. Sürümü hazırlayan kişi hiçbir kurulu sunucuyu ölçmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; P0.1–P0.5 kısmi kalır; yayımlanma kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.97.tr.md). İmzalı dosyalar ve kurtarma uyumu doğrulanır; kurulu panel güncellemesini yalnız kullanıcı başlatır. |
| 21 | `v0.1.0-alpha.98` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisine bakın). **22 Ekim 2026'da yayımlandı (etiket `c249a9ba` üzerinde; altı dosya; portalı sürümü hazırlayan kişi yayımladı); numarayı, sahibin 13 Ekim 2026 tarihli kalıcı talimatıyla sürümü hazırlayan seçti.** Yalnız gezinme (D-037): Sunucu, DNS ve eş sunucular, Güvenlik (güvenlik duvarı, Fail2ban durumu, dışa açık hizmet denetimi) ve Bakım (sistem veritabanları) sayfalarını kazanır; Ayarlar Panel adresi ve HTTPS, Lisans, CelikPanel güncellemeleri ve Sunucu kurulumunu tutar; kişinin kendi iki faktörlü doğrulaması ve parolası kişisel menüdeki Hesabım'dadır. Eski `/settings?section=` bağlantıları yönlendirilir; bayiler ve müşteriler artık boş bir Ayarlar görmez; ürün içi bağlantılar, yönlendirme metinleri ve sunucunun yönlendirme eylemi adresleri yeni yerleri adlandırır; genel portalın GitHub bağlantıları artık `alicelik77`'yi gösterir. API, Agent, güncelleyici, şema, rol ya da yoklama değişikliği yok, bu yüzden gerçek sistem kaydı yok. Web 1697 geçti; Go 7156 geçti, 0 başarısız; yalnız ekranların gösterdiği retler "errors" katalog parçasına taşındıktan sonra kritik açılış yolu 361 KiB'den 329,16 (öncesinde 360,75). Sahibin 22 Ekim 2026 tarihli ekran görüntüsü Boston'un alpha.97'de olduğunu ve Bileşenler'de gerçek sürümlerin göründüğünü gösteriyor, D-036'nın kurulu bir sunucudaki ilk ölçümü; Frankfurt bildirilmedi. Önce bir bağımsız inceleme (12 bulgu ve bir bulgu daha, hepsi düzeltildi) ve yeniden bir denetim yapıldı; sürüm zinciri sorunsuz çalıştı. Bu, tümüyle sahibin kendi GitHub kimliğiyle yapılan ilk sürümdür (sürüm commit'i, etiket ve itmeler Mehmet Ömer Efe Çelik adınadır). Yayımda açık: yeni düzen hiçbir kurulu sunucuda görülmedi; yöneticinin kendi e-postasını değiştiren bir ekran yok; açık oturumlar yapılmadı; Frankfurt'un alpha.95 durmasından sonraki güncellemesi bildirilmedi. **Sahibin bildirimi, 22 Ekim 2026:** iki kurulu sunucu da alpha.98'de (Boston'un ve Frankfurt'un Panel sayfaları yeni menüyü gösteriyor); Frankfurt'un alpha.94'ten güncellemesi, meşgul bir paket yöneticisi yüzünden alpha.95'te durmasından sonra tamamlandı. Bu, ilk ve son açık maddeyi yanıtlar; e-posta maddesi alpha.99 adayı için kaynakta yapıldı. Sürümü hazırlayan kişi hiçbir kurulu sunucuyu ölçmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; P0.1–P0.5 kısmi kalır; yayımlanma kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.98.tr.md). İmzalı dosyalar ve kurtarma uyumu herhangi bir yayımdan önce doğrulanır; kurulu panel güncellemesini yalnız kullanıcı başlatır. |
| 22 | `v0.1.0-alpha.99` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisine bakın). **22 Ekim 2026'da yayımlandı (etiket `7ce52a17` üzerinde; altı dosya; portalı sürümü hazırlayan kişi yayımladı); numarayı, sahibin 13 Ekim 2026 tarihli kalıcı talimatıyla sürümü hazırlayan seçti.** Giriş yapmış kişi kendi e-posta adresini değiştirir (D-037 değişikliği): Hesabım'da bir "E-posta adresim" kartı (`/account#email`) ve ek kullanıcı için kişisel menüde bir e-posta değiştirme öğesi; kendi parolasını değiştirebilen her rol için `PUT /api/v1/auth/email`, mevcut parola (parola değiştirmeyle paylaşılan tek yeniden kimlik doğrulama sınırı), D-029 istek kimliği koruması (koruduğu ilk `PUT`; saklanan özet yalnız yeni adresi tutar), 22 Ekim 2026 güvenlik incelemesinden sonra aynı doğrulayıcıdaki iki iletişim kapısı (panel sertifikası ve kurulum planı), panel sertifikasının kendi adres doğrulayıcısı, bir adres bir hesap, karşılaştır-ve-yaz yazımı, süren oturumlar ve adressiz bir denetim kaydıyla. Panel sertifikasının eksik iletişim reti ve kurulum planının iletişim engeli kişiyi Hesabım'a gönderir; bir e-posta değişikliği hiçbir sertifika istemez; lisans kapısı değişmedi. Agent, güncelleyici, şema, yoklama, bileşen ya da katalog parçası bölünmesi değişikliği yok; bu yüzden gerçek sistem kaydı yok ve sürüm kurtarma düzeneği çalıştırılmadı. Go `CelikPanel-S2-Debian` konuğunda aday ağaçta `go test ./cmd/panel/ ./internal/...` (-timeout 45m, CGO kapalı): 64 paket geçti, 2895 üst düzey test geçti, 42 atlandı, 0 başarısız (4279 alt test geçti); gofmt ve go vet temiz; web `npm test` 1709 testin 1709'u geçti (yeni `tests/own-email-mounted.test.mjs`: İngilizce ve Türkçe 11 test; Ayarlar'da sertifika retinin bandı için yeni bir bağlı test; ek kullanıcının menü öğesi), `npm run build` her bütçenin içinde geçti (kritik açılış yolu 361 KiB'nin 329,93'ü); tarayıcı denetimi 24 8 yapılandırmanın her birinde 13 durum (104 ekran görüntüsü), iki kez bakıldı: ilk bakış telefonda ret bandının düğmesini 28 px buldu, artık en az 44 px; ikinci bakış başka bir şey bulmadı; kişisel menüdeki dil seçici (telefonda 28 px) bu sürümden eskidir ve değişmedi; ekran görüntüleri ve tarayıcı sürücüsü sonra silindi. Sahibin 22 Ekim 2026 bildirimi: iki kurulu sunucu da alpha.98'de ve Frankfurt'un alpha.94'ten güncellemesi tamamlandı. Sürümden önce iki bağımsız güvenlik incelemesi turu yapıldı (bütün bulgular düzeltildi) ve sürüm zinciri sorunsuz çalıştı. Açık: e-posta değiştirme hiçbir kurulu sunucuda ölçülmedi; yeni adres bir iletiyle doğrulanmaz; açık oturumlar yapılmadı; kayıtlı sınırlar duruyor (yalnız harf büyüklüğüyle ayrılan çift adres yarışı, Let's Encrypt'in herkese açık olmayan alan adlarını reddetmesi, `--create-admin`'in doğrulama yapmaması); kurulum planının iletişim engeli her plan için hâlâ geçerli. Sürümü hazırlayan kişi hiçbir kurulu sunucuyu ölçmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; P0.1–P0.5 kısmi kalır; yayımlanma kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.99.tr.md). İmzalı dosyalar ve kurtarma uyumu herhangi bir yayımdan önce doğrulanır; kurulu panel güncellemesini yalnız kullanıcı başlatır. |
| 23 | `v0.1.0-alpha.100` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisine bakın). **22 Ekim 2026'da yayımlandı (etiket `3eeff245` üzerinde; altı dosya; portalı sürümü hazırlayan kişi yayımladı); numarayı, sahibin 13 Ekim 2026 tarihli kalıcı talimatıyla sürümü hazırlayan seçti.** Kaldırma penceresi, salt-okunur bir Panel yanıtından ve kaldırma yolunun kendi koşullarından bir kaldırmanın bu sunucuda neyi etkileyeceğini ve neyi silip neyi bıraktığını adlandırır; Bileşenler'in ilk ziyareti Kurulu görünümü açar; Sunucu kurulumunun Sunucu altında, Bileşenler'in hemen altında kendi sayfası vardır ve Ayarlar'da üç bölüm kalır (D-037 değişikliği; geniş Bileşenler yeniden tasarımı önerildi ve geri çekildi). İlk üç basamaklı ön sürüm numarası: sürümlerin metin olarak sıralandığı bir yer bulunmadı; testler sürümlerin karşılaştırıldığı her yerde alpha.100 > alpha.99 olduğunu kanıtlar. Güncelleyicinin değişiklik yolunda ya da şemada değişiklik yok, bu yüzden yerel kayıt yok; sıra 100, şema 44. Go `CelikPanel-S2-Debian` konuğunda `go test ./cmd/panel/ ./internal/...` (-timeout 45m, CGO kapalı): 64 paket tamam, 2913 üst düzey test geçti, 42 atlandı, 0 başarısız (4282 alt test geçti); temiz bir mount ad alanında `go test ./cmd/agent/...`: 1821 üst düzey test geçti, 29 atlandı, 0 başarısız (2838 alt test geçti); gofmt temiz ve `go vet ./cmd/... ./internal/...` temiz; web 1740 testin 1740'ı geçti; derleme bütçelerin içinde (açılış yolu 361 KiB'nin 331,08 KiB'i); tarayıcı grubu 25'e iki kez bakıldı. Önce üç bağımsız inceleme turu yapıldı (bulguların hepsi düzeltildi; kaldırmanın davranışının değişmediği doğrulandı) ve hiçbir sürümü etkilemeyen bir olaydan sonra sürüm zinciri yeniden çalıştı (kararlar kaydındaki düz bir test komut satırı Go araç zinciri sözleşmesini durdurdu; cümle yeniden yazıldı). Açık: alpha.99 ve alpha.100'den hiçbiri kurulu bir sunucuda ölçülmedi; bir dağıtım paketinin kaldırma betiğinin sildikleri iddia edilmez; kurulum planının iletişim engeli her plan için hâlâ geçerli. Sürümü hazırlayan kişi kurulu sunucu ölçmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; P0.1–P0.5 kısmi kalır; yayımlanma kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.100.tr.md). Her yayından önce imzalı ürünleri ve kurtarma uyumluluğunu doğrulayın; kurulu panel güncellemelerini yalnız kullanıcı başlatır. |
| 24 | `v0.1.0-alpha.101` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisine bakın). **22 Ekim 2026'da yayımlandı (sürüm commit'i `f860053b`; ön sürüm 23:13:34Z'de; portalı sürümü hazırlayan kişi yaklaşık 23:22Z'de yayımladı).** Hazır paket, bir hizmet ile ona eşlik eden araçlardır (D-034 değişikliği; sahibin Frankfurt'un alpha.100 ekranlarından sonraki 22 Ekim 2026 kararı): "PostgreSQL + phpPgAdmin" ve "MariaDB + phpMyAdmin" tek bileşenli veritabanı paketlerinin ve eklentilerinin yerini alır (kimlikler korunur, ad kodları değişir); bir aracın üzerinde çalıştığı web sunucusu ve PHP-FPM başka bir bileşenin doldurduğu rolü kullanır; sistemin sunmadığı bir parça paketin tamamını engeller (Arch'ta phpPgAdmin); paketler Panel'e derlenmiş kalır; Go'daki bir kayıt sözleşmesi testi kuralı zorlar. Kurulu paketin penceresi her parçanın durumunu ve sürümünü söyler; Doğrula geri çekildi (Yeniden tara yeniden okur; posta Onar'ı korur). Sürümün kısa biçimi Debian'daki bir "really" sürümünü doğru okur (Frankfurt'ta phpMyAdmin "5.2.2-really…"; D-036 notu) ve tek bir çalışma ortamı sürümü artık sessizce kırpılmaz. Agent, güncelleyici, sürüm kurtarma ya da şema değişikliği yok, bu yüzden yerel kayıt yok; sıra 101, şema 44. Go 7.199 geçti, 44 atlandı, 0 başarısız; web 1.745/1.745; derleme bütçeler içinde; şeritler ve pencereler 1440 ve 390 px'te tarayıcıda incelendi, bir kusur düzeltildi ve yeniden bakıldı. Sürüm sırası ilkesi, indirme portalı ve imzalı sürüm manifesti sözleşmeleri geçti. Yayımdan önce hazırlayan kişi dosyaları denetledi (sağlama toplamları, manifest, imza, arşiv üyeleri, `get.sh` sabiti 101, politika 100'den sonra 101, içerik korumaları, arayüz paketi `app-EnAd5I-6.js`); portal paketi iki kez bayt bayt aynı derlendi ve genel doğrulama 26/26 ile yayımlandı. Sahip iki kurulu sunucuyu da alpha.100'de görmüştü; alpha.101 ikisinde de yok. Açık: alpha.101'den hiçbir şey kurulu bir sunucuda ölçülmedi (sahibin güncelledikten sonraki gözlemi); kurulum planının iletişim e-postası gereksinimi her sunucu planı için geçerli (karar bekliyor); paketler için bir veri kataloğu sahibe bırakıldı. Sürümü hazırlayan kişi kurulu sunucu ölçmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; P0.1–P0.5 kısmi kalır; yayımlanma kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.101.tr.md). Her yayından önce imzalı ürünleri ve kurtarma uyumluluğunu doğrulayın; kurulu panel güncellemelerini yalnız kullanıcı başlatır. |
| 25 | `v0.1.0-alpha.102` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisine bakın). **23 Ekim 2026'da yayımlandı (sürüm commit'i `085cc400`; ön sürüm 09:50:14Z'de; portalı sürümü hazırlayan kişi yaklaşık 10:02Z'de yayımladı).** Sahip kurulum planının iletişim e-postasının her planda zorunlu kalmasına karar verdi (D-021 değişikliği, 23 Ekim 2026: ilk kurulum her zaman panelin HTTPS sertifikasını ister) ve adresin kurulumda denetlenmesini onayladı: kurucunun istemi, kimlik bilgisi dosyası ve (hiçbir şey kurulmadan çalışan) yalnız-doğrulama denetimi sertifika iletişim doğrulayıcısını kullanır; reddedilen istem yanıtı iki dilli bir satırla yeniden sorulur, reddedilen dosya neyin düzeltileceğini söyleyerek kurulumu durdurur; adres kanonik biçimde saklanır; ret durumunda mevcut yönetici değişmez; mevcut kurulumlar yeniden denetlenmez. Başka hiçbir yol yönetici oluşturmadığı için ekip üyesi denetimi değişmez. Agent, güncelleyici, arayüz ya da şema değişikliği yok, bu yüzden gerçek sistem kaydı yok; sıra 102, şema 44. Go 7.219 geçti, 44 atlandı, 0 başarısız; kabuk sözleşmeleri ve çevrimdışı sürüm kurtarma düzeneği (1.109 test, 1 atlandı) geçti. Yayımdan önce hazırlayan kişi dosyaları denetledi (sağlama toplamları, manifest, imza, arşiv üyeleri, `get.sh` sabiti 102, `install.sh` etiketinkidir, politika 101'den sonra 102, içerik korumaları, Panel ikilisinin kurucunun yeni e-posta iletisini taşıması); portal paketi iki kez bayt bayt aynı derlendi ve genel doğrulama 26 istekten 26'sıyla yayımlandı. Sahip Boston'ı ve Frankfurt'u en son alpha.100'de gözlemledi; alpha.101 ve alpha.102 ikisinde de değil. İletişim e-postası açık işi sahibin kararıyla kapandı (zorunlu, kurulumda denetlenir). Açık: kurulu bir sunucuda ya da gerçek bir ilk kurulumda hiçbir şey ölçülmedi (sahibin güncelledikten sonraki gözlemi); sahibin iki sunucuyu Beta 1'de temiz yeniden kurması yaklaşan kabul adımıdır, yayımlanmış bir iddia değil; paketler için bir veri kataloğu sahibe bırakıldı. Sürümü hazırlayan kişi hiçbir kurulu sunucuyu ölçmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; P0.1–P0.5 kısmi kalır; yayımlanma kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.102.tr.md). Her yayından önce imzalı ürünleri ve kurtarma uyumluluğunu doğrulayın; kurulu panel güncellemelerini yalnız kullanıcı başlatır. |
| 26 | `v0.1.0-alpha.103` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisine bakın). **23 Ekim 2026'da yayımlandı (sürüm commit'i `f780d5f0`; ön sürüm 14:08:46Z'de; portalı sürümü hazırlayan kişi yaklaşık 14:17Z'de yayımladı).** Güncelleme ekranı açılınca, bilinen sonuç on dakikadan eskiyse ve otomatik denetim açıksa yeniden denetler (D-035 değişikliği, 23 Ekim 2026: Frankfurt'un ekranı alpha.102 yayımdayken alpha.101'i sundu): zamanlamanın atlama ve yazma kurallarıyla, yalnız yöneticilere açık yeni bir okuma, aynı anda tek sürüm denetimi, bilinen teklifi koruyan ve Başlat'ı tutan görünür bekleme; kapalıyken hiçbir yere sorulmaz. Durdurulan güncellemenin açık bildirimi kartı artık boşaltmaz (Boston): teklif, nedeniyle önceki deneme ve paket yöneticisinin durumu gösterilir, Başlat bildirimi bekler; güncelleme kilidi değişmedi. Agent, güncelleyici ya da şema değişikliği yok, bu yüzden gerçek sistem kaydı yok; sıra 103, şema 44. Go 7.229 geçti, 44 atlandı, 0 başarısız; web 1.757 geçti; tarayıcı 26. grup; çevrimdışı sürüm kurtarma düzeneği (1.109 test, 1 atlandı) ve kabuk sözleşmeleri geçti. Çekme isteğinden önce bağımsız bir inceleme bir zorunlu düzeltme (açık bir durdurma bildirimi, sayfa yenilemeleri boyunca açılış denetimini devre dışı bırakıyordu) ve daha küçük bulgular buldu; hepsi düzeltildi. Yayımdan önce hazırlayan kişi dosyaları denetledi (sağlama toplamları, manifest, imza, arşiv üyeleri, `get.sh` sabiti 103, `install.sh` etiketinkidir, politika 102'den sonra 103, içerik korumaları, Panel ikilisi yeni açılış denetimi yolunu taşır); portal paketi iki kez bayt bayt aynı derlendi ve genel doğrulama 26 istekten 26'sı ile yayımlandı. Frankfurt'u sahip alpha.100'den alpha.102'ye başarıyla güncelledi; Boston'ın alpha.102 güncellemesi meşgul bir paket yöneticisi yüzünden hiçbir şey değişmeden durdu ve Boston bilindiği kadarıyla hâlâ alpha.100'de; alpha.103'ü çalıştırdığı bilinen bir kurulu sunucu yok. Açık: kayıtlı bilinen eksikler; kurulu bir sunucuda ya da gerçek sürüm sunucusuna karşı hiçbir şey ölçülmedi (sahibin güncelledikten sonraki gözlemi, Boston'ın yeniden denemesi dahil); sahibin Beta 1'de iki sunucuyu da temiz yeniden kurması yaklaşan kabul adımıdır, yayımlanmış bir iddia değil; paketler için bir veri kataloğu sahibe bırakıldı. Kurulu bir sunucuyu sürümü hazırlayan kişi ölçmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; P0.1–P0.5 kısmi kalır; yayımlanma kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.103.tr.md). Herhangi bir yayımdan önce imzalı yapıtları ve kurtarma uyumluluğunu doğrulayın; kurulu panel güncellemelerini yalnız kullanıcı başlatır. |
| 27 | `v0.1.0-alpha.104` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisi). **Yayımlandı, 23 Ekim 2026 (sürüm commit'i `98c4750b4`; ön sürüm 22:03:19Z; portal yaklaşık 22:10Z).** 23 Ekim 2026 Frankfurt olayından sonra (PHP-FPM 8.4'ü kaldırmak phpMyAdmin'i sildi, web postasını ve phpPgAdmin'i durdurdu; D-037 değişikliği): her paket kaldırma paket yöneticisinin kendi benzetimiyle planlanır ve adını söylemediği bir şeyi kaldıracaksa, hiçbir unit durmadan reddedilir; gerçek kaldırma benzetimi yapılan tam listeyi tek kilit altında çalıştırır. Web postası, phpMyAdmin ve phpPgAdmin nginx satırlarının adını verdiği PHP sürümünde kalır ve yalnız aracın Yönet penceresinden, eklentileri sağlanarak ve kendi soketlerinden kanıtla ya da bayt bayt geri almayla taşınır; kaldırma pencereleri ek kaybı, araçları ve siteleri adlandırır ve "taşı, sonra kaldır" sunar; sürüm listesi her sürümü kimin kullandığını söyler; PHP'si eksik araç "Hazır değil" der; phpMyAdmin'in Debian 13 sürümü 5.2.2 okunur. Agent kaldırma yolu değişti: atılabilir bir Debian 13 sanal makinesinde gerçek sistem kaydı. Sıra 104, şema 44. Agent 4.702 geçti; Go 7.238 geçti, 44 atlandı, 0 başarısız; web 1.772 geçti; tarayıcı 27. grup; kabuk sözleşmeleri ve çevrimdışı sürüm kurtarma düzeneği (1.109 test, 1 atlandı) geçti. Açık: kurulu bir sunucuda hiçbir şey ölçülmedi; pacman bileşen testleriyle; Roundcube'un taşınması sanal makinede koşturulmadı. Sürüm hazırlayıcı hiçbir kurulu sunucuyu ölçmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; P0.1–P0.5 kısmi kalır; aday kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.104.tr.md). Herhangi bir yayından önce imzalı yapıtları ve kurtarma uyumluluğunu doğrulayın; kurulu panel güncellemelerini yalnız kullanıcı başlatır. |
| 28 | `v0.1.0-alpha.105` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisi). **Yayımlandı, 24 Ekim 2026 (sürüm commit'i `0a51195cd`; ön sürüm 09:38:34Z; portal yaklaşık 09:45Z).** Paketler her parçanın sürümünü gösterir ve sahibe değiştirtir; önerilen işaretlenir, asla zorlanmaz (D-034 değişikliği: öner, uyar, asla dayatma); çalışamayacak bir PHP hiçbir şey değişmeden reddedilir, sınanmamış olan uyarılır; PHP araçları ilk kurulumda doğrulanmış bir yedekle kanıtlanır, olmazsa "Hazır değil"; bir kurulumun kendi benzetimi başka bir PHP'yi, Apache'yi, başka bir bileşeni ya da bir veritabanı sunucusunu adlandırır ve onaylanana kadar Kur'u tutar, kurmadan önce paket kilidi altında yeniden denetlenir. Agent kurulum yolu değişti: atılabilir bir Debian 13 sanal makinesinde gerçek sistem kaydı. Sıra 105, şema 44. Agent 4723 geçti, 29 atlandı (temiz bir bağlama ad alanında); Go 7243 geçti, 44 atlandı; web 1796 geçti; üretim derlemesi bütçelerinin içinde. Açık: kurulu bir sunucuda hiçbir şey ölçülmedi; pacman bileşen testleriyle; dört kurulum yolu onayın dışında. Sürüm hazırlayıcı hiçbir kurulu sunucuyu ölçmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; P0.1–P0.5 kısmi kalır; aday kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.105.tr.md). Herhangi bir yayından önce imzalı yapıtları ve kurtarma uyumluluğunu doğrulayın; kurulu panel güncellemelerini yalnız kullanıcı başlatır. |
| 29 | `v0.1.0-alpha.106` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisi). **Yayımlandı, 24 Ekim 2026 (sürüm commit'i `a8f90616f`; ön sürüm 12:36:00Z; portal yaklaşık 12:40Z).** Kaldırma hangi sürüm olduğunu söyler: birden çok sürümü olan çalışma ortamı önce sorar; her birini kimin kullandığı ve ayrı "Tüm sürümler"; her başlık neyin gideceğini söyler. Her kaldırma penceresi yanıtla başlar, nedenler her biri bir kez, bağlantılarla yapılabilecekler, teknik kısım Ayrıntılar altında; reddedilen kaldırma Kapat ve "Kaldırılamaz" sunar (D-037 değişikliği). Yalnız web; sıra 106, şema 44. Web 1805 geçti; üretim derlemesi bütçelerinin içinde; hiçbir Go paketi değişmedi. Açık: kurulu bir sunucuda hiçbir şey ölçülmedi. Sürüm hazırlayıcı hiçbir kurulu sunucuyu ölçmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; P0.1–P0.5 kısmi kalır; aday kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.106.tr.md). Herhangi bir yayından önce imzalı yapıtları ve kurtarma uyumluluğunu doğrulayın; kurulu panel güncellemelerini yalnız kullanıcı başlatır. |
| 30 | `v0.1.0-alpha.107` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisi). **Yayımlandı, 24 Ekim 2026 (sürüm commit'i `2e68e6039`; ön sürüm 21:06:26Z; portal yaklaşık 21:13Z).** D-038'in on iki açığı tek sürümde kapatıldı: üretici depoları yalnız sahibi tarafından depo kartından, önce gösterilerek sınırlanır (bir dağıtım paketinin yerine geçmek adıyla reddedilir, kurulumlar tam varsayılanı alır), işletim sisteminin güvenlik güncellemeleri üretici kaynakları için onaylanmış bir ek dosyayla gösterilir, veritabanı motorları purge edilmeden kaldırılır, Roundcube güncellemeleri ve Node.js yama bildirimleri kanıt ve geri almayla, yeniden başlatmalar ve yeniden açılış, tarihli bir destek tablosu, sitenin kullanıcısıyla çalışan yeni Node.js uygulamaları, denetim ayrıntıları, depo kartı ve desteklenen işletim sistemi listesi. Agent yolları değişti: atılabilir bir Debian 13 sanal makinesinde gerçek sistem kaydı. Sıra 107, şema 45. Agent 4771 geçti, 29 atlandı; Go 7925 geçti, 45 atlandı, 0 başarısız; web 1844 geçti; üretim derlemesi bütçelerinin içinde; tarayıcı grubu 30. Açık: kurulu bir sunucuda hiçbir şey ölçülmedi; pacman ve DNF bileşen testleriyle. Sürüm hazırlayıcı hiçbir kurulu sunucuyu ölçmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; P0.1–P0.5 kısmi kalır; aday kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.107.tr.md). Herhangi bir yayından önce imzalı yapıtları ve kurtarma uyumluluğunu doğrulayın; kurulu panel güncellemelerini yalnız kullanıcı başlatır. |
| 31 | `v0.1.0-alpha.108` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisi). **Yayımlandı, 25 Ekim 2026 (sürüm commit'i `f99ae9f3e`; ön sürüm 18:26:34Z; portal yaklaşık 18:35Z).** İki yol (varsayılan olarak işletim sisteminin paketleri; Sury ve PGDG yalnız sınırlama ve otomatik güncellemelerle onaylanmış tek bir işlem olarak, açık sunucularda eksik adımlar tamamlanır); yan yana PostgreSQL ana sürümleri (kendi portları, kanıtlı port değişiklikleri ve veritabanı taşımaları, bir küme veritabanı tuttuğu sürece reddedilen kaldırma); sahibin alpha.107 bulguları (her kurulum önizlenir, phpPgAdmin'in kanıtı, kaybolan bağlama yanıtı, Sury anahtarının bitişi, yerleşim, kenar çubuğu, web postası Güncelle'si, `sudo`); müşterilere sunulan sürümler, denendi işaretleri ve kaldırma izleri. Agent yolları değişti: atılabilir bir Debian 13 sanal makinesinde gerçek sistem kaydı. Sıra 108, şema 46. Agent 4822 geçti, 29 atlandı; Go 7945 geçti, 45 atlandı, 0 başarısız; web 1906 geçti ve üretim derlemesi bütçelerinin içinde; tarayıcı grubu 31. Açık: kurulu bir sunucuda hiçbir şey ölçülmedi; Ubuntu 24.04 sanal makine denemesi yok. Sürüm hazırlayıcı hiçbir kurulu sunucuyu ölçmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; P0.1–P0.5 kısmi kalır; aday kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.108.tr.md). Herhangi bir yayından önce imzalı yapıtları ve kurtarma uyumluluğunu doğrulayın; kurulu panel güncellemelerini yalnız kullanıcı başlatır. |
| 32 | `v0.1.0-alpha.109` sürüm incelemesi (yukarıdaki "Sürüm durumu — 10 Ekim 2026" bölümünde bu sürümün girdisi). **Aday (26 Ekim 2026), yayımlanmadı.** Frankfurt'un eski yapılandırmadan web postası güncellemesi (önce PHP denetim konumu), sonucu satırda kalan kalıcı bir işlem olarak güncelleme, gruplu kurulum onayları; web postasının arayüzü kendi adresine dek panelin kökeninde bekletilir. Agent web postası güncelleme ve kanıt yolları değişti: atılabilir bir Debian 13 sanal makinesinde gerçek sistem kaydı. Sıra 109, şema 46. Agent 4828 geçti, 29 atlandı; Panel 3448 geçti, 6 atlandı; web 1921 geçti; tarayıcı grubu 32. Açık: kurulu bir sunucuda hiçbir şey ölçülmedi; Ubuntu sanal makine denemesi yok. Sürümü hazırlayan kurulu bir sunucu ölçmedi; hiçbir P0 işi kapanmaz ya da ilerlemez; P0.1–P0.5 kısmi kalır; aday kabul değildir. | [Sürüm notları](docs/RELEASE-NOTES-v0.1.0-alpha.109.tr.md). Her yayından önce imzalı yapıtları ve kurtarma uyumluluğunu doğrulayın; kurulu panel güncellemelerini yalnız kullanıcı başlatır. |

**Açık iş (alpha.95, 17 Ekim 2026): her bileşen için kurulu paket sürümünü okumak.** Bileşen taraması bir sürümü yalnız PHP-FPM ve Node.js için doldurur (çalışma ortamı örneklerinden). Agent'ın birim başına ikili yoklaması bir paket sürümü değildir, birkaç birim için güvenilmezdir ve birimi olmayan araçları kapsamaz; bu yüzden Bileşenler satırları sürümü yalnız bilindiği yerde söyler. Gerçek yanıt yeni bir Agent RPC'si ya da genişletilmiş `dpkg`/`pacman` sorguları ister; henüz hiçbir sürümün parçası değildir ([D-034 değişikliği](docs/DECISIONS.tr.md)). **Güncelleme 2026-10-21: v0.1.0-alpha.97 adayı için kaynakta uygulandı** ([D-036](docs/DECISIONS.tr.md)): bileşen taraması başına paket veritabanının salt-okur bir Agent sorgusu, Bileşenler satırlarında ve her bileşen sayfasında gösterilir; bilinmeyen, bilinmeyen olarak kalır. **Hâlâ açık:** hiçbir kurulu sunucuda ölçülmedi; açık iş ancak sahip Boston ve Frankfurt'ta gerçek sürümleri gördüğünde kapanır. **Güncelleme 2026-10-22: Boston ölçüldü.** Sahibin ekran görüntüsü, Boston'un alpha.97'de olduğunu ve Bileşenler'de gerçek sürümlerin göründüğünü gösterir (PowerDNS 4.8.3, Nginx 1.24.0, PostgreSQL 16.15, certbot 2.9.0, Fail2ban 1.0.2, nftables 1.0.9, cron 3.0pl1, Node.js 24.21.0): D-036'nın kurulu bir sunucudaki ilk ölçümü. **Bundan sonra hâlâ açık:** Frankfurt'un alpha.97 durumu bildirilmedi, bu yüzden açık iş kapanmaz; tek bir sunucunun tek bir ekran görüntüsü de her bileşenin, meşgul bir paket yöneticisinin ya da korunan bir okumanın denetimi değildir. **Güncelleme 22 Ekim 2026 (alpha.98):** sahip Frankfurt'un alpha.98'de olduğunu bildirdi, ama Bileşenler'deki sürümlerini bildirmedi; bu yüzden açık iş açık kalır. **Güncelleme 22 Ekim 2026 (alpha.100; alpha.101 adayıyla kaydedildi):** sahibin Frankfurt ekranı Bileşenler'de paket sürümlerini gösterdi; phpMyAdmin'in kısa biçimi "5.2.2-really…" okunuyordu, çünkü Debian'daki bir "really" sürümü gerçekte dağıtılan sürüme kısaltılmıyordu. Bu, v0.1.0-alpha.101 adayı için kaynakta düzeltildi ([D-036 notu](docs/DECISIONS.tr.md)). Açık iş açık kalır: düzeltilmiş kısa biçim henüz hiçbir kurulu sunucuda görülmedi.

**Aynı adayda kapandı (alpha.98, 22 Ekim 2026): kritik açılış yolunda 0,25 KiB bütçe kalmıştı** (ham 361 KiB'den 360,75). İnceleme turu tavanı yükseltmek yerine yalnız ekranların gösterdiği kodlu retleri "errors" ekran parçasına taşıdı: ham 361 KiB'den 329,16, sıkıştırılmış 110 KiB'den 100,37. **Açık iş (alpha.98):** panel sertifikasının iletişim adresi olarak istediği yönetici e-postasını değiştiren bir ekran yok; kendi e-postasını Hesabım'dan değiştirmenin bir yolu yapılacak. **v0.1.0-alpha.99 adayı için kaynakta kapandı (22 Ekim 2026)** ([D-037 değişikliği](docs/DECISIONS.tr.md)): sahip yapılmasını onayladı ("ok"); giriş yapmış kişi mevcut parolasını verdikten sonra kendi adresini Hesabım'da (ek kullanıcı kişisel menüden) değiştirir ve panel sertifikasının reti oraya yönlendirir. alpha.98'de açıktı. **Hâlâ açık:** hiçbir kurulu sunucuda ölçülmedi.

Yeni test eklemeden önce hangi açık kabulün kapanacağı belirtilir.
Geçen deney, ancak ilgili değişiklik veya adı konmuş belirsizlik nedeniyle tekrarlanır.
Kanıt saklanıp kontrol edildikten sonra geçici konuklar durdurulur ve doğrulanmış
geçici diskleri temizlenir; kurtarma malzemesi ve sahibin verisi korunur.

**Bitiş tarihi:** mevcut kanıtla belirlenmiş değil. Kalan uygulama ve deney altyapısı
açıkları varken güvenilir yüzde veya kesin tarih verilemiyor. Bağımsız DNS yolu ve
eksik deney kapsamı doğrulandığında süre yeniden değerlendirilir; bitiş, küçük
commit sayısına değil mevcut kabul koşullarının karşılanmasına bağlıdır.

### Yapay zekâ asistanı aşaması — işlem/kurtarma temelinden sonra

Kullanıcının istediği yapay zekâ asistanı planda: gözlenen durumu açıklayacak,
kullanıcının yetkilendirdiği panel işlerini aynı türlenmiş ve kapsamı sınırlı
işlem API'leriyle yapmasına yardımcı olacak. Planı gösterecek, izinleri koruyacak,
işlem kimliğini kaybetmeyecek ve doğrulanmış sonucu bildirecek. Kabul deneyleri;
izin reddini, kullanılamayan hizmeti, kesilen isteği ve ikinci değişiklik üretmeyen
tekrarı kapsamalı. Sınırsız root, eksik kanıt uydurma, lisans atlama veya kurulu
panel güncellemesi başlatma yetkisi almayacak. Yapay zekâ entegrasyonu kuralları
belli kurtarmanın yerine geçmez; bu incelemede uygulanmış sayılmıyor.

---

## Sürüm Merdiveni

Aşağıdaki sürüm merdiveni önceki aşamaların tarihsel kaydıdır. Geçmiş başarılı
kurulum sonuçları ve mimari değerlendirmeler, 14 Eylül dayanıklılık sözleşmesinin
karşılandığını göstermez; bu kabul çalışması açık kalmaktadır.

Varış noktası: **v1.0 — bir yabancının temiz VPS'e dakikalar içinde kurabildiği,
üzerinde gerçek hosting işi yürütebildiği ve güvenebildiği panel.** Aşağıdaki her şey o yolun taşı.

17 Temmuz güncellemesiyle merdivene üç yeni gereksinim işlendi — muğlak "ileride" değil, basamak basamak:
1. **Panel-içi yardım/ipuçları:** temel taşları v0.2.5'te (mevcut borç kalemlerinin doğal uzantısı olarak),
   kullanıcıya dönük içerik v0.3'te, yardım merkezi + sihirbaz + palet v0.6–0.9'da.
2. **Bayi + müşteri birinci sınıf deneyim:** kapı açan dilim v0.2.5/B1'de, gövde v0.3'te
   (tahsilat yetkisi dahil), `additional_user` gerçek özelliği v0.35'te.
3. **Ücretsiz katmanlı abonelik/plan sistemi:** veri modeli + dönem/iptal/askı makinesi + ödeme defteri +
   bayi havuzu v0.3'te, "her vaat uygulanır ya da silinir" dürüstlüğü v0.35'te, ödeme sağlayıcı kararı
   (iade/chargeback dahil) v0.5'te, self-signup + kötüye kullanım freni v0.6–0.9'da.

### ✅ v0.0 — Devralma *(3 Temmuz 2026)*
~23 bin satır devralındı: mimari sağlam (Panel + root Agent, SQLite) ama güvensiz
(açık TCP agent, SQL injection, kimlik doğrulama yok) ve arayüz sahte veriyle dolu.
Karar verildi: devam, sıfırdan yazma yok.

### ✅ v0.1 — Güvenli Çekirdek + Kanıtlı Golden Path *(3–10 Temmuz 2026 — tarihsel v0.1.0 aşaması)*
Sekiz gün, dört cephe, hepsi push'lu:
- **Güvenlik (Faz 0):** agent Unix socket + token arkasında · oturum kimliği (argon2id) + 2FA/TOTP ·
  SQL injection temizliği · CSRF/başlıklar/hız sınırı · gosec yüksekleri kapandı · sızmış parola etkisiz.
- **Barındırma çekirdeği (Faz 1–3):** domain tipleri (php/statik/node/proxy/yönlendirme) + alt alanlar ·
  gerçek oto-yenilemeli SSL · otoriter DNS (PowerDNS/SQLite eşitleme, DNSSEC, DANE) ·
  tam posta yığını (TLS+SNI, kimlikli gönderim 587/465, DKIM imzalama, sunucu politikası,
  teslim edilebilirlik sağlık ekranı) · veritabanları v2 · tek tık WordPress · cPanel içe aktarıcı v1 ·
  hesaplar (yönetici/bayi/müşteri, planlar, kotalar, yerine-geçme) · haklar + WireGuard VPN ·
  saklama süreli zamanlanmış yedek · denetim günlüğü · güvenlik duvarı (varsayılan-reddet) ·
  servis kaldırma · otomatik güvenlik yaması · yönetilen vendor depoları (PGDG sürüm seçimi).
- **İşletim (Faz 2):** `install.sh` (tek komut → giriş ekranı) · anlık görüntülü `update.sh` · `rollback.sh` ·
  systemd unit'leri · **golden path Ubuntu'da uçtan uca kanıtlı**: temiz kurulum → domain → HTTPS →
  dünyaya cevap veren kendi DNS'i → Gmail INBOX'a DKIM imzalı posta.
- **Ürün ve tasarım:** Plesk yoğunluğunda arayüz, açık/koyu, TR+EN · tasarım sistemi claude.ai/design'da
  (tasarım döngüsü: tarif et → ajan gerçek bileşenlerle çizer → süz → yayınla) · self-hosted marka
  fontları · tüm sayfalarda yeni ölçek · kurulum yolculuklu canlı pano.
- **Alfa çalışma modeli (D-008):** operatör paneli gerçek müşteri gibi sürer; çarptığı her duvar ürün
  düzeltmesi olur. İki günde ~20 gerçek hata bu yolla bulunup yayınlandı.

**Çıkış ölçütü karşılandı:** golden path uçtan uca kanıtlı (Ubuntu) · panel kendi güncellemesini taşıyor · alfa modeli işliyor.

### 🔶 v0.2 — Alfa Tamam: Debian Yeniden-Kanıtı *(tarihsel aşama; güncel öncelik yukarıda)*
Aynı golden path, üretim VPS'inde (Debian 13) **tamamen panel tıklamalarıyla** yeniden kanıtlanacak:
- ✅ Yalnız-panel kurulum (sıfır ek paket) · ✅ PowerDNS panelden kuruldu ·
  ✅ yönetim sayfası dürüst (config görünürlüğü, çalışan onarım)
- ⏳ Sıradaki tıklamalar: otomatik onarım → ilk domain → panel Let's Encrypt sertifikası →
  kayıt operatörüne DS kaydı → web sunucusu + canlı site → posta yığını → **Debian'dan Gmail INBOX**
- Çıktıkça kalan alfa pürüzleri + `autodiscover` (posta istemci otomatik ayarı)
- Dış engel: kayıt operatöründeki alan adı askısı (operatörün işi)

**Çıkış ölçütü:** bir ziyaretçi `https://celikpanel.cloud`'u açabiliyor ve oradan atılan posta Gmail
INBOX'ına düşüyor — yapılandıran her tıklama panelde, hiçbiri kabukta değil.

### 🩺 v0.2.5 — Borç Ödeme (Otopsi reçetesi) + Temel Taşlar
11 Tem 2026 adli denetimi ([AUTOPSY](docs/AUTOPSY.tr.md)) canlı kırıklar ve yapısal borç çıkardı;
karar: **refaktör, rewrite değil.** B0 (kanamayı durdur: ölü TypeID sabitleri, admin-olmayana kırık
Databases sayfası, ölü kod) aynı gün kapandı. Kalanı sırayla: **B1** tek API (v2→v1, kiracı kapsamı
auth'tan, OpenAPI + üretilen istemci) · **B2** route+authz tablosu · **B3** servis bilgisinin tek
sahibi katalog · **B4** UI disiplini (tek Button/fmtBytes/modal) · **B5** golden-path smoke CI.

Üç yeni gereksinimin temel taşları **ayrı iş olarak değil, B1–B5'in doğal uzantısı olarak** bu basamağa döşenir
(sonradan eklenirse hepsi ikinci kez kırılır):
- **B1'e ek — hata sözleşmesi:** tüm hata gövdeleri `{code, message, hint?, action?}` standardına geçer.
  `code` makine-okur sabittir (örn. `DNS_SERVER_REQUIRED`), `hint` i18n anahtarıyla ön yüzde çözülür,
  `action` panel-içi link olabilir ("PowerDNS kur" → /services). Tüm bilinçli retler kodlu: D-009 409'u,
  kota 409'ları, çakışma grupları, entitlement 402'si. Ön yüzde tek `ErrorBanner` bileşeni; hata gövdesi
  OpenAPI şemasında tanımlı — üretilen istemci bir kez doğru doğar.
- **B1'e ek — Databases self-servisinin önü:** sunucu-kaydı admin'de kalır; DB/kullanıcı CRUD uçları
  kiracı kapsamıyla müşteri+bayiye açılır; `nav.ts` geçici admin kilidi kalkar; phpMyAdmin vekili
  sahiplik doğrular. (v0.3'ün gerçek ön koşulu — sert kısıtın nedeni budur.)
- **B2'ye ek — fail-closed rol:** kullanıcı kaydı okunamayan istek boş rolle asla ilerlemez (bugün
  `middleware.go`'da Role='' ile devam ediyor). Route+authz tablosunun üstüne **rol×uç matris testi**:
  `--demo` tohum hesaplarıyla her (uç × admin/bayi/müşteri/anonim) hücresi beklenen 200/403/404'e
  karşı doğrulanır; tabloya kayıtsız uç testte fail eder.
- **B3'e ek — Setup Journey dürüstlüğü:** journey adımları paket varlığından değil, katalogdan gelen
  gerçek servis durumundan (kurulu + etkin + koşuyor) okunur. Saha kanıtı zaten var: Hostinger Arch
  imajındaki uyuyan bind "DNS kuruldu: Done" saydı (16 Tem). Bu senaryo B5 smoke'una regresyon olarak girer.
- **B3'e ek — katalogda tür ayrımı + "kurulu-önce" varsayılan (20 Tem, D-010):** `ManagedService`'e
  `Kind` (service/runtime/tool) ve `Role` alanları girer; php-fpm ve yeni **node** kalemi `Kind=runtime`,
  phpMyAdmin/phpPgAdmin `Kind=tool` olur. Satır çizimi `Kind`'e dallanır ve `Daemonless = len(SystemNames)==0`
  sezgisi silinir (bugün üç ayrı şeyi işaretliyor). **Sürümler satır değil, satırın içinde** (sürüm
  çekmecesi) — liste patlaması kaynağında kesilir. Servisler sayfası "kurulu-önce": "kurulu olmayanı gizle"
  varsayılan AÇIK, kategoriler katlı, arama her zaman tüm katalogda ve ikisini de geçersiz kılar. Ayrı
  `/runtimes` ya da `/apps` sayfası açılmaz.
- **B3'e ek — sürüm birinci sınıf + Node katalogda ilan edilir:** tek agent sözleşmesi
  `Agent.ListServiceInstances(id)` her örnek için Version/Unit/Path/Managed/SizeBytes döner
  (`DetectInstalledPHPVersions` ve `ListNodeVersions` bunun ilk iki uygulaması); `extractVersion` switch'i
  ve `"default"` sentinel'i gider. **Node.js yeteneği zaten kodda var** (`runtime_rpc.go`, `app_rpc.go`)
  ama katalogda ilan edilmiyor — yeni kod değil, görünürlük işi. node kalemi web sunucusunu `Requires` ile
  şart koşar (bugün hiçbir yerde ifade edilmeyen reverse-proxy gereksinimi deklaratif olur).
- **B3'e ek — PHP çoklu-sürüm gerçek olur (Sury):** D-002 "✅ yapıldı" diyordu ama yalnız yarısı yapılmış —
  tespit ve site başına seçim çalışıyor, çoklu sürüm KURULUMU yok (`php-fpm`'de `Repo` tanımlı değil,
  kodda `sury` geçen tek satır yok). PGDG için var olan `ManagedRepo` mekanizması php-fpm'e uygulanır:
  Debian/Ubuntu'da yan yana `php8.x-fpm` panelden kurulur; Arch'ta dürüstçe "tek dağıtım sürümü" denir.
  "Seçici var, seçenek yok" hali biter.
- **B3'e ek — runtime kurulumunun tek adresi + sahiplik defteri:** `AdminNodeInstall` (HostingTypePanel)
  silinir; kur/kaldır yalnız Servisler'deki sürüm çekmecesinde (sürüm uçları parametreli — Hestia 5050'ye
  düşülmez). Serbest semver kutusu kalkar, agent LTS listesini çeker (3-5 adlandırılmış seçenek).
  "Sistem yorumlayıcısı" kaçağı kaldırılır — panel yalnız kendi kurduğuyla çalışır. `site_runtimes`
  defteri + `RuntimeUsage`/`Dependents`: kullanımdaki sürüm/servis kaldırılamaz, B1 sözleşmesiyle kodlu
  ret (`RUNTIME_IN_USE`, `SERVICE_HAS_DEPENDENTS`) + engelleyen site listesi döner (bugün 40 site
  kullanırken php-fpm tek tıkla kaldırılabiliyor).
- **B3'e ek — proje tipi tek kaynak + Node oluşturmada seçilebilir:** `CreationProjectTypes` (3 tip) ile
  `validProjectTypes` (5 tip) tek `ProjectTypes` tablosundan türetilir. Add Domain'e "Node.js Uygulaması"
  kartı gelir (alan adı + sürüm + başlangıç komutu; port otomatik). Ön-denetimler tablodan okunur; node
  için de web sunucusu şartı **kaydetmeden önce** kodlu retle döner — bugünkü "PHP'de düğmeyi kapat,
  Node'da kaydet ve agent'ta patlat" asimetrisi kod düzeyinde imkânsızlaşır. Çalışan uygulamalar Node
  runtime satırının altında sayılır ("3 uygulama · 1 hatalı") ve domain'e link verir.
- **B4'e ek — yardım katmanının atomu:** `ui.tsx`'e tek Tooltip/InfoTip bileşeni (HelpCircle + i18n'li
  açıklama, klavye erişilebilir); mevcut 6 Info callout ve ≥10 kritik "ne bu?" alanı (DNSSEC DS, DKIM,
  catch-all, SNI…) taşınır. Yeni ipucunun tek bariz yolu bu bileşendir.
- **B4'e ek — i18n disiplini:** JSX'te çıplak string yakalayan lint (App.tsx'teki "Coming soon..." gider;
  vsftpd yer tutucusu ya dürüst i18n'li EmptyState olur ya nav'dan düşer) · en.ts/tr.ts anahtar eşitliği
  kontrolü (`tools/check-i18n`) CI'da — eksik anahtar sessizce İngilizce'ye düşemez.
- **B5'e ek — framework adı CI kapısı (D-011):** Go kaynaklarında
  `laravel|symfony|django|nextjs|ghost` grep'i sıfır eşleşme vermeli (i18n dizeleri hariç). Kural
  Markdown'da kalırsa iki yıl dayanmaz; enum sabiti/DB kolonu/API değeri/systemd unit adı bir kez
  framework adı taşıdığında katalog örtük olarak doğar ve DB'de kalıcılaştığı için geri alınamaz.
- **Sürüm tekliği + CHANGELOG:** annotated git tag (ilk aday v0.2.0) · sürüm `-ldflags` ile iki binary'ye
  gömülür, `/api/v1/panel/version`'dan servis edilir, Layout.tsx'teki sabit "v0.1.0" silinir · Keep-a-Changelog
  biçiminde CHANGELOG.md + CHANGELOG.tr.md başlar; `update.sh` çıkışında "değişiklikler: CHANGELOG.md" basılır.

**Çıkış ölçütü:** AUTOPSY B1–B5 kapalı · rol×uç matrisi CI'da koşuyor ve hiçbir uç anonim/boş-rol için
200 dönmez · tüm 4xx ön-denetimleri kodlu, ErrorBanner çeviriyor · `panel --version`, UI alt bilgisi ve
git tag aynı diziyi söylüyor · web'de i18n dışı kullanıcıya görünür İngilizce string sayısı 0 (lint CI'da) ·
müşteri rolüyle DB oluştur → phpMyAdmin'e gir → başkasının DB'sine erişim 404 (üçü de B5 smoke'unda) ·
**temiz sunucuda Servisler sayfası en çok 4 satır** (kurulu olmayan hiçbir kalem çizilmez) · Node sürümü
kurmanın panelde tek yolu Servisler'dir (`AdminNodeInstall` kod tabanında yok) · Add Domain'den tek formda
çalışan Node sitesi kuruluyor ("önce statik kur sonra tipi çevir" adımı yok) · Debian'da panelden ikinci bir
PHP sürümü kurulup bir siteye atanıyor (Sury), Arch'ta aynı ekran dürüstçe "tek dağıtım sürümü" diyor ·
kullanımdaki PHP sürümünü/servisi kaldırma denemesi kodlu retle dönüyor ve engelleyen site listesini
gösteriyor · proje tipi listesi tek dosyada; her ölçüt iki test sunucusunda doğrulanmış.
**Sert kısıt: v0.3, B1 bitmeden başlayamaz.**

### 🚨 v0.2.6 — Emanet Edilebilirlik Zemini *(yeni basamak, 25 Tem 2026)*
Yol haritası hazırlanırken yapılan kod denetimi **üç canlı veri-kaybı/yalan kusuru** buldu. Bunlar
sıradaki her özelliğin önüne geçer: bir panel, müşterinin sitesini emanet edilebilir olmadan satılamaz.
Üçü de kodda doğrulandı, tahmin değil.

**Zeminin üç deliği:**
1. **"Tam yedek" veritabanını içermiyor.** `createFullBackup`, kendi yorumuyla birlikte dosya
   yedeğine düşüyor (*"For now, just backup files"*). Riskli bir değişiklik öncesi "Tam yedek" alan
   operatör, geri yüklediğinde veritabanını geri alamaz — güvenlik ağı kılığında sessiz veri kaybı.
   Aynı sebeple `full_` arşivinin geri yüklenmesi de yalnız dosyaları döndürür.
2. **Geri yükleme yanlış veritabanının üzerine yazabiliyor.** Hedef ad, dosya adından alt çizgiyle
   bölünüp ilk parça alınarak çıkarılıyor: `wp_site1` veritabanının yedeği `wp` adlı veritabanına
   dönüyor. Alt çizgi veritabanı adlarında yaygın olduğu için bu, **başka bir müşterinin verisinin
   üzerine yazmak** demektir.
3. **Kurulum, servisin başlayıp başlamadığına bakmıyor.** `systemctl enable --now` sonucu yutuluyor
   ve ardından koşulsuz "kuruldu" deniyor. Anayasanın birinci kuralı ("kurulunca çalışır") tam da
   kurulum anında ölçülmüyor.

**Zeminin dört yapısal eksiği:**
4. **Şablon düzeltmeleri eski sitelere ulaşmıyor** (config drift, AUTOPSY C bölümü). Kapatılmış iki
   güvenlik bulgusu — `.env`'in düz metin sunulması ve PHP kaynağının indirilmesi — düzeltmeden ÖNCE
   oluşturulmuş her sitede hâlâ açık. "Tüm vhost'ları yeniden üret" yolu yok.
5. **Dört site ayarı sunucuya hiç gitmiyor.** Belge kökü, www/https yönlendirmesi, HSTS/zorunlu-HTTPS
   ve alan takma adları veritabanına yazılıyor, nginx'e ulaşmıyor. Operatör ayarı değiştiriyor, ekran
   onaylıyor, sunucu eskisini sunmaya devam ediyor.
6. **SSL kaldırmak vhost'u yenilemiyor** — bir sonraki yeniden yüklemede patlamaya hazır yapılandırma
   bırakıyor.
7. **Apache koltuğu alıyor ama Apache yazıcısı yok.** Tek vhost üreteci nginx'e ait
   (`internal/services/nginx_generator.go`, şablon dizini yalnız `nginx/`). Apache kurmak 80 portunu
   karartır. Yazıcı yazılana dek satır dürüstçe reddedilmeli.

**Çıkış ölçütü:** "Tam yedek" arşivi açıldığında içinde veritabanı dökümü VAR ve her arşiv kendi
içindekiler dosyasını taşıyor · geri yükleme hedefi dosya adından tahmin edilmiyor, içindekiler
dosyasından okunuyor (alt çizgili ad iki sunucuda da doğru veritabanına dönüyor) · geri yükleme
öncesi otomatik anlık görüntü alınıyor · kurulum, başlatma başarısızsa "kuruldu" demiyor ve günlük
kuyruğunu gerekçe olarak gösteriyor · panelden değiştirilen dört site ayarı sunucuda ölçülebiliyor ·
SSL kaldırıldıktan sonra `nginx -t` geçiyor · tek vhost yazıcı kaldı ve "tüm siteleri yeniden üret"
düğmesi eski siteleri de düzeltiyor (iki eski güvenlik bulgusu eski sitelerde de kapanıyor) ·
Apache satırı yazıcı yokken kodlu retle kapalı · CI gerçek bir panel açıp bu ölçütlerin duman
betiklerini koşuyor. **Sert kısıt: v0.3 bu basamak bitmeden başlayamaz.**

### v0.3 — Çok Kiracılı Gerçeklik
Birden fazla kiracıya utanmadan satabilmek. Dört ayak: müşteri ve bayi kendi başına yaşayabiliyor;
plan/abonelik makinesi "ücretsiz katman + ücretli plan"ı **girişten çıkışa** ifade edebiliyor
(satın alma kadar iptal de birinci sınıf); cPanel'den gelenin ilk hafta aradığı asgariler yerinde;
ilk gerçek kiracıdan önce üretim güveni tamam.

**Müşteri ve bayi birinci sınıf:**
- Müşteri kendi aboneliğini görür: `GET /api/v1/my/subscription` (B1 kiracı kapsamı üstüne) — plan adı,
  kotalar, canlı kullanım (domain/DB/mail sayısı + ölçülen disk). Dashboard'a "Planım" kartı: doluluk
  çubukları, %80 üstü uyarı rengi, "Yükselt" düğmesi. 409 kota hatası ekrandaki sayılarla tutarlıdır.
- Parola kurtarma: tek kullanımlık, 15 dk ömürlü, argon2id-hash'li token; posta panelin kendi MTA'sından.
  E-posta doğrulama gelir — doğrulanmamış adrese sıfırlama gönderilmez.
- Davet akışı: kullanıcı oluşturmada parola opsiyonel; verilmezse hesap "pending" açılır, ilk-parola
  bağlantısı postayla gider. Bayi müşterisinin parolasını hiçbir kanaldan görmez/iletmez.
- Parola değişimi ve sıfırlama, hedefin diğer tüm oturumlarını düşürür ("sızmış parola etkisiz" sözünün
  tamamlanması — bugün açık oturum parola değişiminden sağ çıkıyor).
- Impersonation hesap verir: `impersonate.start/stop` audit_logs'a yazılır; bürünme altındaki eylemlerde
  `acting_as` alanı gerçek operatörü işaretler; panelin üstünde kalıcı "X olarak görüntülüyorsunuz — çık" şeridi.
- **Bayi tahsilat yetkisi:** bayi, kendi ağacındaki abonelikler için askıya alma/devam ettirme,
  "ödendi işaretle" ve plan değiştirme çağırabilir (B1 kiracı kapsamı süzer); hepsi `acting_as`'lı
  audit'e düşer. Müşterisinin abonelik + ödeme durumunu bayi de görür. Ödemeyen müşterisini kesemeyen
  bayi tahsilatı operatöre taşır — "bayi birinci sınıf" sözü tahsilatsız yarımdır.
- Rol-farkındalıklı onboarding (mevcut journey kart deseni, kütüphane yok): müşteri için "ilk domain →
  SSL → ilk posta kutusu → istemcini bağla"; bayi için "plan → ilk müşteri → abonelik". Canlı tamamlanma
  izler, bitince kaybolur.
- Sayfa başı açıklamalar: 12 rota + 8 domain sekmesi için birer-iki cümle TR+EN (`pages.<id>.desc`).
  Kısıt üreten 5 yere "Neden?" açıklaması (D-002, D-003, D-009, çakışma grubu, pkg desteği) — metinler
  DECISIONS kayıtlarıyla tutarlı; kısıt açıklamasız duvar olarak çarpmaz.

**Plan ve abonelik makinesi (ücretsiz katmanın temeli):**
- Planlara fiyat: `service_plans`'a `price_cents, currency, billing_period, is_free, is_public, sort_order,
  kdv_included`. Admin "Free — 1 domain, 0₺" ve "Pro — 10 domain, X₺/ay" planlarını panelden tanımlar;
  ürün fiyatları kod sabitinden DB'ye taşınır. Müşteri kendi planının adını ve fiyatını görür.
- **Sunum defteri (D-017 / D-014'ün eksik halkası):** `plan_offerings` (plan ↔ teklif-kimliği) tek
  kanonik uzayda — `component:<id>[:<sürüm>]`, `integration:acme:<id>`, `product:<id>`. Her seçici uç
  (php sürümleri, node sürümleri, SSL sağlayıcıları) çağıranın etkin kümesine süzülür: mevcut(admin) ∩
  plan(bayi); eylem uçları `NOT_OFFERED` kodlu retle yeniden doğrular. Kısıt bildirmeyen plan her şeyi
  sunar (geriye dönük kırılma yok); admin kayıt-defteri kalemlerini (örn. bir CA'yı) sunucu genelinde
  kapatabilir. Çıkış ölçütü: bayi planından PHP 7.4'ü ve ZeroSSL'i çıkarır → müşterisinin seçicilerinde
  görünmez, API'den denerse kodlu ret + audit.
- **Dönem modeli** ("yükselttim, ne ödüyorum?" sorusunun cevabı): abonelikte `current_period_start/end`;
  kural en basit dürüst olandır ve DECISIONS'a yazılır — yükseltme anında yeni dönem başlar (kıst yok,
  bu açıkça ilan edilir); düşürme ve iptal dönem sonunda uygulanır; `expires_at` dönem ucundan türetilir.
  v0.5 webhook'u bu alanları uzatır — tanımsızlığın üstüne sağlayıcı entegrasyonu kurulmaz.
- Abonelik askısı gerçek etki üretir (bugün `status`/`expires_at` ölü alan): suspended/expired abonelikte
  yeni kaynak 403, vhost'lar geri-alınabilir "hesap askıda" sayfasına döner, posta teslimi durur (kutular
  silinmez). `expires_at` geçmiş abonelikler günlük döngüde expired'a çekilir + grace period alanı.
  Manuel "ödendi işaretle" düğmesi aynı makineyi sürer — ödeme sağlayıcı kararı v0.5'te, makine bugünden çalışır.
- **İptal birinci sınıftır** ("veri rehin tutulmaz" sözünün abonelik hali): müşteri-tetiklemeli
  "dönem sonunda iptal" (`cancel_at_period_end`); dönem sonunda otomatik Free'ye düşüş. Kullanım Free
  kotasını aşıyorsa makine kilitlenmez: **zorunlu-düşürme modu** — kaynak silinmez, aşan kısım dondurulur
  (yeni kaynak 403 + aşan vhost'lar askı sayfası) + taşan kaynak listesi + X günlük tasfiye süresi +
  bilgilendirme postası. `subscription.cancel` audit'e düşer. (Aynı mod v0.6'daki trial bitişini de sürer —
  insansız düşürme 409'a çarpıp sonsuza dek Pro'da kalamaz.)
- Plan değiştirme (yükseltme monetizasyonun ana akışıdır): `PUT /api/v1/subscriptions/{id}/plan` — kotalar
  plandan yeniden kopyalanır; **insanlı** düşürmede mevcut kullanım > yeni kota ise 409 + taşıran kaynak
  listesi; **insansız/zorunlu** modda yukarıdaki dondurma kuralı. Audit'e `subscription.plan_change`.
  Müşterinin "Yükselt" talebi ilk aşamada operatöre bildirim üretir.
- **Ödeme defteri** (manuel modda bile ödemenin izi kalır): `payments` tablosu
  (subscription_id, amount_cents, currency, period, method=manual|provider, marked_by, created_at).
  "Ödendi işaretle" bu tabloya yazar; v0.5 webhook'u aynı tabloyu besler. Müşteri "Planım" kartının
  altında ödeme geçmişini görür + yazdırılabilir basit makbuz. "Ne ödedim, ne aldım" panelden cevaplanır.
- **Süre bildirimleri** (en ucuz tahsilat aracı): `expires_at`−7/−3/−1 gün ve grace başlangıcında
  müşteriye (bayili senaryoda bayiye de) panelin kendi MTA'sından posta; askı anında "neden + nasıl açılır"
  postası. Müşteri askıyı ziyaretçisinden değil postasından öğrenir.
- **Bayi havuzu bir plan türüdür** (ticari hayatı olan kota): `reseller_pools` (max_customers, toplam
  disk/domain/DB) bayi planına bağlanır — havuz boyutları + fiyatı bayi planında yazar; "Yükselt" akışının
  bayi sürümü havuzu büyütür. Abonelik açılırken bayinin ağacındaki toplam taahhüt havuzla karşılaştırılır,
  aşımda 409 + kalan-havuz mesajı; Users'da doluluk çubuğu. **Zincir kuralı** (DECISIONS'a): bayi askıya
  düşerse ağacında yeni kaynak 403, ama mevcut müşteri siteleri/postası grace sonuna dek yaşar — masum
  son-müşteri bayisinin borcu yüzünden anında karartılmaz.
- Bayiye ait plan: ölü `service_plans.owner_id` canlanır — bayi kendi planını kurar (kotalar havuzunu
  aşamaz), listede global + kendi planlarını görür, yalnız kendi müşterisine atar; "apply to subscribers"
  kopyalaması owner kapsamına saygılıdır.
- **Lisanslı ürünler ve satış zinciri (D-012, 20 Tem — üçüncü-taraf baştan kapsamda):** hak, disk gibi bir
  **havuz** olur; `reseller_pools` deseni ürünlere genişler (admin kontenjanı → bayi → müşteri; admin
  doğrudan müşteriye de satabilir). Ürün tanımına `license_model {server|seat}` + `seat_unit
  {mailbox|site|subscription|server}` girer — *seat* modelinde fazla tahsis gerçek para ve lisans ihlali
  olduğundan havuz sertçe uygulanır (kodlu ret, uyarı değil). Lisans anahtarı A4'ün `enc:v1` mekanizmasıyla
  mühürlenir. Fiyat tek sayı olmaktan çıkar (satıcı→admin, admin→bayi, bayi→müşteri; bayiye-ait-plan
  deseniyle aynı). Görünürlük hakkı izler: bayi almadıysa müşterisi ürünü hiç görmez (bayi isterse
  "satın al" tanıtımını açar). Kurulu olmayan ürüne hak satılamaz → kodlu ret. Geri alma, abonelik
  askısıyla AYNI kural (yeni tahsis 403, mevcut kullanım grace sonuna dek yaşar, veri silinmez).
  **Dürüstlük sınırı UI'da ve dokümanda yazar:** panel yalnız kendi tahsis kaydını uygular; satıcı farklı
  sayabilir (mutabakat ekranı gösterilir ama fatura satıcının gerçeğidir) ve **alt-lisanslama hakkı
  operatörle satıcı arasındadır** — panel bunu doğrulamaz, doğruladığını iddia etmez.
- Faturalama defteri: `plan.create/update/delete`, `subscription.plan_change`, `subscription.cancel`,
  `subscription.suspend/resume`, `quota.exceeded` audit olayları — "bu kota ne zaman, kim tarafından
  değişti" sorusu ihtilaf sorusudur, kayıtsız kalınmaz.
- **Mağaza seçki ilkesi (operatör, 24 Tem): farklılaşma, herkesin sunduğunu sunmakta değil,
  görmezden geldiği MÜKEMMEL açık kaynak alternatiflerini birinci sınıf sunmaktadır.** "Birçok panel
  hep aynı şeyleri sunuyor; her alanda güzel alternatifler var, bunları sunmamız gerekir." Mağaza
  kataloğu kurulurken her kategoride yerleşiğin yanına modern FOSS alternatifi bilinçle aranır.
  İlk adaylar (operatör kararı 24 Tem: grommunio elendi — kendi vendor deposuna + ağır dağıtıma
  özgü yığına bağlı olması "her Linux'ta tek yol" ilkesine aykırı): Stalwart (tek-binary modern posta
  sunucusu — taşınabilir) · restic/borg (yedekleme) · Uptime Kuma (izleme). Her aday, girmeden önce canlı
  paket/depo doğrulamasından geçer (Buypass dersi: dış uç da bayatlıyor) VE dağıtımdan bağımsız
  kurulabilmelidir (D-004: dağıtıma özgü çözüm kabul edilmez; webmail için Roundcube resmi
  tarball'ı — Node deseni — çekirdek yol oldu).

**Barındırma asgarileri (cPanel'den gelenin ilk haftası):**
- FTP (vsftpd) uçtan uca — ölçütlü: domain başına hesap, site kullanıcısının docroot'una chroot,
  **FTPS zorunlu** (düz FTP reddedilir — güvenlik varsayılandır). Kanıt: FileZilla ile bağlan → yükle →
  site canlıda değişir; chroot dışına çıkma denemesi başarısız.
- Webmail (Roundcube) — ölçütlü: katalogdan tek tık kurulum; `webmail.<domain>` vhost + Let's Encrypt +
  Dovecot bağlantısı otomatik. Kanıt: panelde açılan posta hesabı, kabuğa hiç dokunmadan webmail'den
  Gmail'e posta atıp cevabını okuyor. (Roundcube "panel için" değil "panelin kurduğu servis"tir — dış
  bağımlılık yasağına dokunmaz.)
- Dosya yöneticisi cilası — üç ölçülebilir kalem: (1) zip/tar.gz yükle-ve-aç + seçileni sıkıştır-indir,
  (2) metin dosyası yerinde düzenleme (sahiplik/izin korunur), (3) izin görüntüle/değiştir (777'ye uyarı).
  Kanıt: bir WordPress tema zip'i yalnız dosya yöneticisiyle kurulup sitede görünüyor.
- Gürültülü komşu freni: site/abonelik başına systemd slice (CPUWeight + MemoryMax, plan alanı olarak;
  alan ancak uygulamasıyla birlikte gelir — ölü alan doğmaz). PHP-FPM havuzları ve `celikapp-*` unit'leri
  ilgili slice'a bağlanır; CloudLinux lisansı gerekmez. Kanıt: bir kiracıda sonsuz döngü PHP koşarken
  komşu site <1 sn açılıyor.
- OS seviyesinde disk uygulaması (ROLES ertelenenleri) · cPanel içe aktarıcının **gerçek** müşteri
  arşiviyle kanıtı (DB kullanıcıları dahil) · WordPress Toolkit derinliği (güncelleme, sertleştirme,
  klon/staging).
- **Framework barındırma ilkelleri (D-011, 20 Tem):** Laravel/Symfony/Django'yu katalogsuz barındırmanın
  önündeki dört gerçek engel — hiçbiri framework'e özel değil, hepsi eksik *jenerik* yetenek:
  (1) **docroot alt dizin seçimi** — bugün `public_html`'e sabit; açılır kutu framework adı değil YOL
  değeri listeler (`(kök) | public | public_html`). (2) **Site kullanıcısı kimliğiyle komut ucu** —
  `composer install`, `artisan migrate`, `npm ci` çalıştırabilmek (bugün kod tabanında `composer` sıfır
  kez geçiyor; kullanıcı SSH'a itiliyor). Çıktı akışlı, zaman aşımlı, audit'li. (3) **Uzun süreli süreç
  (queue worker)** — bugün üç bağımsız engel var: `RunAsUser: "www-data"` sabiti, `req.Port <= 0` reddi,
  `project_type == "node"` kilidi; `celikapp-*` unit soyutlaması portsuz/site-kullanıcılı işçiyi de
  taşımalı. (4) **Site cron'u** — scheduler ayrı bir kavram değil, sıradan bir crontab satırı.
  Bunların üstüne **preset**: framework varsayılanlarını forma ÖN-DOLDURAN düğme (tip değil, kurucu
  değil); D-011'in yapısal saflık şartına tabidir — tek yeni alan ya da tek `if framework ==` dalı
  gerektiren preset reddedilir, preset sayısı 3'ü aşarsa strateji tartışması açılır.
- **DNS sağlayıcı soyutlaması (D-009 yeniden tartımı + 18 Tem operatör kararı):** DNS bir SEÇİM olmalı,
  dayatma değil. Panel bugün tam kayıt setini hesaplayıp tek yere (kendi PowerDNS'i) yazıyor; o "yazıcı"
  takılabilir kılınır — üç arka uç, operatör seçer (domain başına da olabilir):
  (1) **Kendi PowerDNS'i** (varsayılan, sıfır-bağımlılık: her şeyi tek kutuda isteyen için — panel
  otoriter, ns1/ns2 bu sunucu);
  (2) **Cloudflare-sınıfı yönetilen DNS** (operatör API token'ı verir — DNS-edit kapsamlı; panel AYNI kayıt
  setini PowerDNS SQL yerine sağlayıcının API'sine yazar; zone yoksa oluşturur). **Güvenlik için önerilen
  yol** ve operatörün 18 Tem gözlemi: :53 kutuda açık kalmaz, DDoS'u Cloudflare yutar, tek-nokta arıza
  kalkar. Plesk'in "Cloudflare DNS Integration" eklentisinin dürüst çekirdek karşılığı;
  (3) **Dış/elle** (panel hiçbir şey yazmaz; "şu kayıtları girin" listesi + mail-auth'taki canlı doğrulama).
  Downstream'in tamamı (mail-auth kayıtları, HTTP-01 sertifikası, panel hostname'i) değişmeden çalışır —
  çünkü değişen yalnız yazıcı, hesaplanan kayıt seti aynı. **Otomatikleştirilemeyen TEK adım dürüstçe
  söylenir:** registrar'daki nameserver delegasyonu (celikhost.com'un NS'ini Cloudflare'e ya da bu sunucuya
  yöneltmek) hiçbir sağlayıcı API'sinde yoktur; panel bunu gösterir + doğrular, o tek tıkı insan registrar'da
  atar. Karar (hangi arka uçlar, hangisi varsayılan, öneri metni) DECISIONS'a; abstraction seam v0.3'te +
  (1) ve (3) yolları; (2) Cloudflare arka ucu v0.4'te (bkz. yönetilen DNS backend'i).
- **Panel kimliği — rehberli hostname + sertifika (18 Tem saha boşluğu):** bugün panelin kendi adının
  (örn. `boston.celikhost.com`) çözülür olması TASARLANMADI — operatör test sunucusunda bunu operatör-dışı
  el (başka sunucunun panelinden kayıt) çözdü; bu, D-008'in yasakladığı gizli elle adımdır. Panel, kendi
  hostname'ini domain kurmakla aynı dürüst üç yolla ele almalı: (a) **bu panel adının ana zone'unu kendisi
  sunuyorsa** → tek tık A kaydını kendi zone'una yaz (tek-sunucu, zone şablonunun kendi-FQDN tohumlamasının
  genellemesi); (b) **DNS dışarıda/başka sunucuda** → "şu A kaydını girin: `<host>` → `<IP>`" göster ve
  mail-auth'taki canlı DNS doğrulamasıyla çözülene dek bekle, sonra sertifikayı sun; (c) **zaten çözülüyor**
  → doğrudan sertifikaya geç. Sertifika akışı (v0.2) bu ön-adımın üstüne oturur — "install.sh → giriş →
  gerçek sertifika" zincirinde artık elle DNS boşluğu kalmaz. Çok-sunucu oto-kaydı (kardeş sunucunun adını
  zone-otoritesi sunucuya kaydettirme) bilinçli olarak çok-sunucu özelliğine (1.0-sonrası) ait — orada
  panel-arası güven modeli gerekir; o güne dek (b) yolu N sunucuyu dürüstçe karşılar.

**Üretim güveni (ilk kiracıdan önce şart):**
- Sır şifrelemesi öne çekildi: A4'ün kanıtlı `enc:v1` mekanizması TOTP secret'larına ve panelin sakladığı
  özel anahtarlara (DKIM, WireGuard) genişler; eski satırlar açılışta idempotent mühürlenir. (v0.5'te
  yalnız dış denetim doğrulaması kalır — ilk kiracının 2FA sırrı aylarca düz metin beklemez.)
- Kiracı-başına rate limit: pahalı uçlar (sertifika alma, yedek tetikleme, import, DNS toplu yazma)
  abonelik anahtarlı limitle korunur; Let's Encrypt başarısız deneme sayacı + "LE limitine yaklaşıldı"
  dürüst uyarısı — tek kiracının döngüsü herkesin sertifikasını engelleyemez.
- Migrasyon disiplini (expand/contract): yıkıcı şema değişikliği iki sürüme bölünür (N'de ekle+çift yaz,
  N+1'de kaldır) — rollback veri kaybetmez. CI'ya iki test: temiz DB'ye tam zincir + dolu v(N−1) fixture
  üstüne güncel zincir; `rollback.sh` anlık görüntüden sonra yazılmış satır sayısını raporlayıp açık onay
  ister; "geri alma, snapshot sonrası değişiklikleri kaybeder" cümlesi belgelidir.
- CI güvenlik kapıları: `gosec` ve `govulncheck` her PR'da; ağ kullanan bağımlılık denetimleri paket adlarını ve
  sürümlerini yapılandırılmış kayıt servisine gönderdiği için açık işletmen onayı gerektirir; istisnalar
  `#nosec` + gerekçe. (v0.5 dış denetimi bu kapıların aylık yeşil geçmişiyle karşılanır.)
- Yazılı terfi ritüeli (OPERATIONS.md): (1) CI yeşil → (2) iki test sunucusunda (boston/Debian,
  frankfurt/Arch) `update.sh` + golden-path smoke → (3) üretim. Kanal netleşir: main=edge (test
  sunucuları), tag=stable (üretim yalnız tag'li commit çalıştırır).
- `release.yml`: `v*` tag push'unda CI ortamında `make dist` → SHA256SUMS → GitHub Release'e
  tarball+checksum+CHANGELOG bölümü otomatik.

**Çıkış ölçütü:** bir bayi + iki müşteri **bir hafta self-servis** işliyor — parola sıfırlama, davet,
kota görüntüleme, DB, FTP, webmail dahil; operatör dokunuşu sıfır · admin panelden Free + Pro planı
tanımlıyor, bir abonelik tek çağrıyla Pro'ya taşınıyor, audit kaydı düşüyor · iptal eden Pro müşterisi
dönem sonunda otomatik Free'ye düşüyor; kotayı aşan kaynakları dondurulmuş ve listelenmiş, verisi silinmemiş ·
süresi dolmak üzere olan abonelik −7/−3/−1 postalarını alıyor; askıya düşen "neden + nasıl açılır" postası
alıyor · her "ödendi işaretle" payments defterine düşüyor ve müşteri makbuzunu panelden görüyor · askıya
alınan abonelik 60 sn içinde askı sayfası dönüyor, devam ettirilince veri kaybı sıfır · 10 GB havuzlu bayi
6+6 GB iki aboneliği açamıyor (ikincisi 409); bayi ödemeyen müşterisini kendi başına askıya alıp
"ödendi işaretle" ile geri açabiliyor · gerçek bir cPanel hesabı tek tıkla taşınıyor · dış DNS kararı
DECISIONS'a işlenmiş · v0.3.0 tag'i insan eli değmeden indirilebilir release üretmiş.

### v0.35 — Plan Dürüstlüğü: Ölü Alan Kalmasın
Kısa basamak. Şemada/katalogda olup uygulanmayan her vaat ya uygulanır ya silinir — Dürüstlük kuralının
plan hali. Satılan planın her satırı gerçek olmadan ücretli katman "bitti" sayılmaz:
- `bandwidth_quota_mb`: ya uygula ya kaldır. Uygulama: usage ölçümünden abonelik-düzeyi aylık sayaç
  (dönem başı sıfırlanır); nginx log yalnız web trafiğini sayar — mail/FTP hariç olduğu plan metninde
  dürüstçe yazılır.
- Aşım politikası — "limit ne zaman ısırır" sorusunun tek cevabı: plan başına
  `enforcement ∈ {block_new, notify, suspend_writes}`; %80 ve %100 eşiklerinde müşteriye + operatöre
  posta; Dashboard "Needs attention"a kota satırı.
- Posta kutusu kotası: `mailbox_quota_mb` + Dovecot quota plugin'i (mevcut dosya-tabanlı desenle,
  idempotent tam-durum-itme). `business_email` bu limiti yükseltir — ürünün ilk gerçek kapısı.
- Ürün kapıları: Addons'ta listelenen her ürün ya en az bir `requireEntitlement` kapısına bağlı ya satın
  alınamaz. `extra_ip` tesisatı v0.5'e dek "yakında" işaretli; `firewall` müşteri-görünür bir özellik
  doğana dek katalogdan çıkar. Satın alınmadan da çalışan "satılık" ürün kalmaz. **`app_installer`
  gerçekliğe indirilir (D-011):** bugünkü "WordPress *ve diğer uygulamalar*" ifadesi tek girişli bir
  listeyi çoğul plan özelliği olarak satıyor — satış tarafına doldurulacak boş kova gösteriyor ve liste
  plan özelliğine bağlandığı an giriş silmek sözleşme ihlaline dönüyor. Ürün adı "WordPress tek-tık
  kurulumu" olur; çoğul ifade kaldırılır.
- `additional_user` gerçek özellik olur: müşteri hesabına bağlı (`parent_id`), `user_permissions` ile
  kaynak-bazlı izin (domain listesi + dosya/mail alt izinleri), kendi girişi. (CHECK genişletme ve
  frontend'deki ölü rol dallarının dürüstlüğü v0.2.5/B2'de yapılır.)
- Kullanıcı-detay görünümü: bir müşterinin abonelikleri + kota doluluğu, domainleri, entitlement'ları,
  son girişi (`users.last_login_at`) ve son 10 audit satırı tek ekranda — "bu müşteri neden 409 alıyor"
  sorusu 4 ekran gezdirmez. Admin tümünü, bayi kendi ağacını görür (v0.3 tahsilat yetkisinin ekranı).
- Cron güvenilirliği: her iş için son çalışma zamanı + çıkış kodu + son çıktı kuyruğu; başarısız işte
  domain sahibine posta (mevcut posta yığını, yeni bağımlılık yok).

**Çıkış ölçütü:** şema ile uygulama arasında tek ölü kota/durum alanı yok (alan alan denetim listesiyle
kanıtlı) · %80 doluluğa ulaşan test aboneliği 5 dk içinde posta alıyor, %100'de politika uygulanıyor ·
100 MB kotalı kutuya 101. MB "Quota exceeded" ile reddediliyor · Addons'taki her ürün kapılı ya da satın
alınamaz · ek kullanıcı yalnız izinli domain'in sekmelerini görüyor · bilerek exit 1 dönen cron listede
kırmızı ve sahibinin INBOX'ında.

### v0.4 — İşletim Güveni
Operatörün gece 3'te ihtiyaç duyduğu şeyler:
- İzleme + uyarı (servis düştü, disk doldu, sertifika hatası → posta/webhook) · panelde log görüntüleyici
- **Metrik tarihi (Plesk kıyasından, 17 Tem):** bugünkü kartlar anlık değer gösterir, hikâye göstermez.
  Agent'ta hafif örnekleyici — CPU/RAM/disk/trafik N saniyede bir SQLite halkalı tabloya, eski veri
  otomatik seyreltilir (dış bağımlılık YOK: Prometheus/Grafana değil, anayasa korunur). Pano kartlarına
  sparkline (kart sade kalır); karta tıklayınca 24 saat / 7 gün detay grafiği. Uyarı eşikleri aynı
  veriyi okur — grafik ve alarm tek altyapının iki yüzü. Çok-lokasyonlu dış uptime izleme bilinçli
  hedef-dışı: dürüst cevap heartbeat + "UptimeRobot/360 kullanın".
- **Uyarı kanalının kendi sağlığı:** uyarılar posta VE webhook'tan bağımsız iki kanaldan gider (posta
  kuyruğa giremezse webhook'a düşer) + dışa dönük heartbeat — panel N dakikada bir operatörün seçtiği
  dış uca ping atar, kesilince alarm DIŞARIDAN çalar. En olası arıza, uyarıyı taşıyacak kanalın ölmesidir.
- Uzak yedek hedefleri (S3/FTP) + ürün özelliği olarak geri yükleme tatbikatı — **iki sertleştirmeyle:**
  (1) **Panel state felaket yedeği:** SQLite (online backup API ile tutarlı kopya) + `secret.key` +
  DKIM/WireGuard anahtarları + panel sertifikaları tek arşivde, domain yedekleriyle aynı saklama süresiyle
  uzak hedefe. `secret.key` kaybı = tüm mühürlü sırlar geri dönüşsüz; "domainler yedekte ama panelin
  beyni yok" durumu kabul edilmez. (2) **İstemci tarafında şifreleme:** uzak hedefe çıkan her arşiv
  panelde şifrelenir; yedek anahtarı `secret.key`'den ayrı tutulur ve kurulumda bir kez gösterilir —
  "anahtarsız yedek okunamaz" dürüstlüğü UI'da yazar.
- **Müşteri self-servis geri yükleme:** yedek listesinden tam site / tek dizin-dosya / DB dump dönüşü —
  onay modalı + audit kaydıyla. "Her kullanıcının sorununu sen mi çözeceksin?" sorusunun en pahalı
  cevabı restore'dur; müşteri gece 3'te kendi bozduğunu kendi döner.
- **İkincil DNS gerçeği:** bugün ns1/ns2 aynı makineyi gösteriyor (tek nokta arızası);
  ikinci ucuz VPS'e secondary PowerDNS (AXFR) ya da dürüst belgeleme (11 Tem eklendi)
- **Yönetilen DNS backend'i (Cloudflare-sınıfı) — v0.3 soyutlamasının somut sağlayıcısı:** Settings'te
  "DNS sağlayıcısı" seçimi; Cloudflare arka ucu = scoped API token + zone oto-oluşturma + `syncZoneToDNS`
  seam'ine ikinci yazıcı (aynı kayıt seti, farklı hedef). İkincil-DNS tek-nokta sorununun en temiz
  cevabı da budur: DNS'i tümüyle Cloudflare'e vermek, ikinci VPS'e AXFR kurmaktan basit ve daha güvenli.
  Panel :53'ü hiç açmaz; "güvenlik varsayılandır" ilkesiyle bu yolu ÖNERİR ama dayatmaz (kendi PowerDNS'i
  isteyen için sıfır-bağımlılık varsayılan kalır). Registrar NS delegasyonu dürüstçe elle adım olarak
  gösterilir + doğrulanır. Bu, panelin kendisine dış bağımlılık DEĞİLDİR — operatörün seçtiği opsiyonel
  arka uç (MariaDB↔PostgreSQL seçimi gibi); token operatörün, panel internetsiz de tam çalışır.
- Arayüzden tek tık panel güncellemesi (update.sh'ın ön yüzü) · WebSocket canlı bildirimler
- **Güncelleme zinciri sertleşir:** release-binary kanalı birincil olur — `update.sh` sürüm tarball'ını
  indirir, **imza doğrular** (minisign/cosign; açık anahtar install.sh'a gömülü, rotasyon planı yazılı),
  SHA256 doğrular, derleme adımını atlar. Üretim sunucusunda Go/Node toolchain'i gerekmez (saldırı
  yüzeyi + küçük VPS'te OOM + bit düzeyinde fark, üçü birden kapanır); `--from-source` geliştirici/test
  sunucularına kalır. Doğrulama başarısızsa mevcut sürümde kalınır + "Needs attention" + audit kaydı.
- **Güncelleme sonrası self-check:** bitişte otomatik smoke — panel HTTP 200 + login render + agent
  socket ping + `PRAGMA quick_check`; kalan varsa çıktı "rollback.sh önerilir" der, tek-tık akışında
  geri alma düğmesi sunulur.
- **Arıza matrisi — "panel öldü, hosting yaşıyor" kanıtı:** panel process kill → web/DNS/posta serviste
  (testle ölçülür), panel `Restart=on-failure` ile kendine gelir; "kırılma camı" runbook'u (panel
  açılmıyorsa SSH'tan teşhis adımları) D-008'e acil durum istisnası olarak yazılır.
- **CelikPanel→CelikPanel taşıma:** hesap-düzeyi export arşivi (domainler + docroot + DB dump + maildir +
  DNS zone + DKIM anahtarı + abonelik/kota metadata'sı tek imzalı tar'da); karşı sunucuda cPanel
  içe aktarıcının inspect→onay→apply akışı bu formatı da tanır. "Veri rehin tutulmaz" sözünün tam hali;
  sunucu değişimi hosting işinin rutinidir.
- **Ziyaretçi istatistiği (minimal, bilinçli sınırlı):** trafik ölçümü zaten nginx access log'unu okuyor;
  aynı geçişten günlük hit / tekil IP / ilk 10 sayfa / ilk 10 referrer çıkarılıp domain Overview'a kart
  olur. Tam analitik (oturum, coğrafya, gerçek zamanlı) hedef-dışıdır — dürüst cevap: "siteye
  Plausible/GA koyun".
- **Audit bütünlüğü:** yazma hatası sayaçlanıp "Needs attention"a düşer (eylemi bloklamadan ama sessiz
  de kalmadan); audit için yapılandırılabilir saklama süresi + budama.
- **Aktif oturumlar:** Settings'te oturum listesi (oluşturma, son kullanım, IP) + tekil/toplu sonlandırma;
  admin Users'tan hedefin oturumlarını düşürebilir.
- **Temiz başlangıç:** panel, kendisinin kurmadığı katalog servislerini "yabancı" diye işaretler
  (audit log'da `service.install` kaydı olmayan her katalog servisi) ve operatör onayıyla kaldırmayı
  önerir — sahiplen ya da tahliye et, Import felsefesinin aynası. Saha kanıtı (16 Tem): Hostinger
  Arch imajı uyuyan bir bind ile geldi; kurulum yolculuğu "DNS kuruldu: Done" saydı. B3 üstüne oturur.
- **Pano SSL/TLS özeti** (Plesk kıyasından, 17 Tem): yakında dolacak / geçerli / sertifikasız site
  sayıları tek kartta — "90 günde sessizce ölen sertifika" sınıfı, panelin yüzünde görünür olur
- **Pano Mail Queue kartı** (Plesk kıyasından, 17 Tem): Total/Deferred/Held + tek tık kuyruk
  temizleme — operatörün gece 3'te ilk baktığı yer
- **Kendi kendine teşhis:** operatörün 17 Tem'de sorduğu soru tasarım ölçütü — "her kullanıcının
  sorununu sen mi çözeceksin?" Panel, bugün elle teşhis edilen sınıfları kendisi denetlemeli:
  DNS delegasyonu bu sunucuya bakıyor mu, **panelin kendi hostname'i BU sunucuya çözülüyor mu** (18 Tem:
  boston.celikhost.com kaydı frankfurt'un zone'unda yaşıyordu, boston'ın haberi yoktu — bu bağ görünmezdi),
  sertifika yenileme zamanlayıcısı gerçekten koşuyor mu, servis config'i motoru gerçekten başlatabiliyor mu.
  Bulgu = "Needs attention" satırı + tek tık onarım (Plesk'in Repair Kit'inin dürüst karşılığı)

- **Katalog genişlemesi — Python, Arch'ta veritabanları, göç kolu (25 Tem 2026):**
  - **Python çalışma ortamı.** Bugün panelde Python **yok** — ne katalogda bir kalem, ne kodda bir
    satır; `hosting_handlers.go`'daki yorum "go/python sonradan eklenebilsin diye" diyor ama kalem hiç
    açılmadı. Node'un kullandığı üç ilkel zaten var ve büyük ölçüde genel: site başına systemd unit
    (`ApplyAppUnit`/`ControlAppUnit`/`AppUnitStatus`/`AppUnitLogs`), önündeki ters vekil ve sürüm
    çekmecesi. Python'un eklediği üç şey: **site başına sanal ortam (venv)**, **uygulama sunucusu
    seçimi** (gunicorn/uvicorn — WSGI ve ASGI ayrımı ürün kararıdır, gizlenemez) ve Add Domain'de
    "Python Uygulaması" kartı. **Sistem python3'ü kullanmak yok:** Node'da bilerek kaldırılan
    "sistem yorumlayıcısı" kaçağı burada açılmaz — panel yalnız kendi kurduğuyla çalışır.
    Django/Flask/FastAPI barındırma, cPanel'in "Setup Python App"ine karşılık gelen ve ücretsiz
    panellerin çoğunun kötü yaptığı yerdir; yeni kavram gerektirmeyen en büyük pazar genişlemesi.
  - **Arch'ta veritabanları.** MariaDB ve PostgreSQL Arch'ta hiç sunulmuyor; sebep katalogda
    "kurulum sonrası ilk hazırlık" alanının olmaması (`initdb` / `mariadb-install-db` adımı ifade
    edilemiyor). **Tek katalog alanı** iki bileşeni açar, dağıtım matrisindeki iki boşluğu kapatır ve
    Arch'ta tek tık WordPress'i mümkün kılar. "OS bağımsızlığı" iddiasının ölçülebilir sınavı budur.
  - **DNS bölge içe/dışa aktarma (+ CAA).** Kimse içeri giden yol olmadan panel değiştirmez; zone
    aktarımı en ucuz göç koludur ve panelin kendi yetkili DNS'ine sahip olması bunu yapısal olarak
    kolaylaştırır.

  **Çıkış ölçütü:** iki dağıtımda da panelden Python sürümü kurulup Add Domain'den tek formda çalışan
  bir Django/FastAPI sitesi ayağa kalkıyor (venv site başına, unit çalışıyor, ters vekil doğru) ·
  Arch'ta panelden MariaDB VE PostgreSQL kurulup ilk veritabanı oluşturuluyor · dağıtım matrisinde
  bu üç hücre artık boş değil · bir cPanel zone dosyası panele aktarılıp alan adı sorgulara doğru
  cevap veriyor.

**Çıkış ölçütü:** öldürülen servis bir dakikada uyarı üretiyor; fişi çekilen sunucu 5 dakikada **dış**
alarm üretiyor · geri yükleme tatbikatının tanımı: temiz VPS'te `install.sh` + panel-state restore +
domain yedekleri ile tam sunucu ayağa kalkıyor ve DKIM imzalı posta **aynı anahtarlarla** INBOX'a düşüyor ·
uzak depodaki hiçbir nesne şifresiz değil · üretim VPS'inde Go/Node kurulu olmadan sürüm güncellemesi
başarılı; bozuk imzalı tarball reddediliyor ve audit'e düşüyor · müşteri sildiği `wp-config.php`'yi
panelden tek başına geri getiriyor · arıza matrisindeki her hücre en az bir kez tatbik edilmiş.

### v0.5 — Güvenlik Derinliği
- WAF kararı (ModSecurity ya da dürüst alternatif) · fail2ban derin entegrasyonu · ClamAV zamanlanmış site taramaları
- Sır şifrelemesinin dış denetimle doğrulanması (uygulama v0.3'te yapıldı: TOTP + DKIM + WG anahtarları `enc:v1`)
- Dış güvenlik denetimi · dedicated IP tesisatı (satılabilir `extra_ip` — v0.35'ten beri "yakında" duran kapı açılır)
- **API token yönetimi:** kullanıcı başına adlandırılmış token (crypto/rand, DB'de yalnız hash, bir kez
  gösterim, kapsam: salt-okunur/tam, iptal); token'lı istekler kiracı-kapsam süzgecinden geçer ve audit'e
  düşer. "Her şey önce API" sözü, curl ile kullanılamayan bir API ile tutulamaz.
- **SFTP/SSH anahtar yönetimi:** site kullanıcısı için müşteri public key ekler/siler (defter panelde,
  agent `authorized_keys`'i tam-durum-itme ile yazar); parola girişi ve shell **varsayılan kapalı**
  (internal-sftp + chroot docroot); `access.ssh_key.add` audit'te. Düz FTP'nin ötesini isteyen ajans ve
  CI/rsync/git-hook akışlarının yolu budur; panel-içi terminal bilinçli hedef-dışı.
- **2FA derinliği:** etkinleştirmede 8 tek-kullanımlık kurtarma kodu (argon2id hash'li, bir kez gösterilir);
  login'de "kurtarma kodu kullan" dalı; "admin/bayi için 2FA zorunlu" ayarı — zorunluysa 2FA'sız kullanıcı
  ilk girişte kurulum ekranına kilitlenir.
- **Ödeme entegrasyonu karar kaydı (D-0xx):** sağlayıcı sınıfı = hosted-checkout / Merchant-of-Record
  (Stripe Checkout / Paddle / iyzico sınıfı — kart verisi panele asla girmez, PCI kapsamı sıfır); panel
  yalnız **webhook tüketicisidir**: abonelik↔sağlayıcı-müşteri eşleme tablosu + imza doğrulamalı,
  idempotent tek webhook ucu (işlenen event_id defteri) + `payment.event` audit'i. Olay sözleşmesi üç
  sınıftır: "ödendi" → dönem uzar (payments defterine satır), "ödenmedi" → grace → askı (v0.3 makinesi),
  **"iade/chargeback"** → iade: ilgili dönemin uzatması geri alınır (`expires_at` kısalır) +
  `payment.refund` audit'i + operatör bildirimi; chargeback: otomatik askı + "Needs attention" satırı
  (chargeback aynı zamanda kötüye-kullanım sinyalidir). Fatura PDF'i sağlayıcıdan link. Manuel "ödendi
  işaretle" modu sağlayıcısız operatör için kalır; **ilk sürümde sağlayıcı entegrasyonu yalnız operatör
  düzlemindedir, bayi tahsilatı manuel akışla sürer (bilinçli ve yazılı).** Anayasa kısıtı: tek binary +
  SQLite — ödeme çekirdeğe gömülmez.

**Çıkış ölçütü:** dış denetim yüksek önem bulgu vermiyor · bir müşteri panelden dedicated IP satın alıp
kullanıyor · dokümandaki tek curl örneği token ile domain açıyor; iptal edilen token anında 401 ·
DB dump'ında ve dataDir'de grep ile tek düz sır bulunamıyor · sahte webhook event'i test sunucusunda
`expires_at` uzatıyor; aynı event ikinci kez etkisiz; refund event'i uzatmayı geri alıyor · kurtarma
koduyla giriş bir kez başarılı, ikincide 403; zorunluluk açıkken 2FA'sız bayi giremiyor · anahtarlı
müşteri sftp ile yalnız kendi docroot'unu görüyor, shell denemesi reddediliyor.

### v0.6 – v0.9 — Beta Programı
- OpenAPI dokümantasyonu (API-first sözünün kanıtı) · yönetici ve kullanıcı kılavuzları TR+EN
- **Yardım merkezi + derin bağlantı:** docs/ altındaki markdown'dan üretilen statik TR+EN site (üretici
  basit, panele dış bağımlılık girmez); her panel sayfasının başlığında sabit slug'la doküman sayfasına
  giden yardım ikonu (`pages.<id>` → `/help/<lang>/<slug>`); kırık slug'ları yakalayan test B5 CI'ında.
  Doküman panele bağlanmazsa ölü doğar.
- **Tarayıcıdan ilk-açılış:** DB'de hiç admin yoksa panel tek seferlik ilk-açılış moduna girer —
  `install.sh`'ın ürettiği tek-kullanımlık token'la tarayıcıdan admin oluşturulur, admin oluşunca mod
  kalıcı kapanır. "install.sh → giriş ekranı" vaadinin ortasındaki "terminale dön, CLI çalıştır" adımı
  kalkar; korumasız ilk-kurulum ucu olmasın diye aceleye değil bu sürüme kondu.
- **Self-signup + kötüye kullanım freni** (ücretsiz katmanın ölçeklenme yolu): e-posta doğrulamalı kayıt
  formu — **varsayılan KAPALI** (güvenlik varsayılandır), açan operatör free plana bağlar; disposable-domain
  listesi + IP başına kayıt hız sınırı. Fren: free planda saatlik giden posta üst sınırı (mail_policy
  deseni üstüne), yeni hesapta ilk 24 saat gönderim bekletme seçeneği, kaynak oluşturma hız sınırı.
  Deneme: plan başına `trial_days` → `expires_at` otomatik dolar, dolunca **zorunlu-düşürme moduyla**
  free'ye iner (v0.3 kuralı — insansız geçiş 409'a çarpamaz). Tek kötü müşteri, teslim edilebilirlik
  ekranıyla kazanılan IP itibarını yakabilir — signup freni olmadan açılmaz.
- **Komut paleti:** Ctrl+K — nav + domain listesi + servis sayfaları tek fuzzy arama; ilk sürümde yalnız
  gezinme (eylem çalıştırma yok — güvenlik yüzeyi büyümez), yanına "?" kısayol listesi; dış bağımlılıksız,
  rol süzgecine uyar. Bugünkü 12 rotada lüks olurdu; beta operatörünün günlük verimi için doğru an burası.
- **SUPPORT.md (TR+EN):** 1.0 öncesi yalnız en son minor destek görür, güvenlik düzeltmeleri son minor'a
  patch gelir; yükseltme yolu "her sürümden en yenisine, migration zinciri fixture testleriyle kanıtlı";
  v1.0'da N−1'e genişler. Beta davetiyle birlikte yayında — cevap o anda uydurulmaz.
- **KVKK/GDPR asgarileri:** veri ihracı = taşıma arşivinin müşteri-tetiklemeli hali; hesap silme DB
  kaskadına ek maildir + docroot + hesabın yedek arşivlerini kapsar ve "neyin silindiği" raporu üretir;
  log/audit saklama süreleri yapılandırılabilir ve belgeli.
- Gerçek dış beta kullanıcıları; onların duvarları düzeltme olur (alfa modeli, ölçeklenmiş)
- Performans hedefleri ölçülüp uygulanıyor (<100 ms API)
- **Lisans/iş modeli kararı** (öneri: open core) → repo görünürlüğü buna göre. Kararla birlikte iki
  taahhüt DECISIONS'a yazılır: (1) **iki düzlem ayrımı** — operatörün kendi müşterisinden para alması
  (panelin özelliği) ile CelikPanel'in operatörden para alması (lisans) ayrı düzlemlerdir; birincinin
  hiçbir tablosu/ucu ikinciye telemetri/lisans denetimi taşımaz, panel internetsiz ortamda tam çalışır.
  (2) **Fiyat konumu:** fiyat sunucu başınadır, barındırılan hesap/domain sayısına göre **asla**
  ölçeklenmez — cPanel'in 2019 sonrası hesap-başına modeli panel tarihinin en büyük göçünü tetikledi;
  bu vaat geç ilan edilirse beta "bu da büyüyünce cPanel'leşir" şüphesiyle gelir.
- **"Neden CelikPanel" belgesi** (docs/WHY.tr.md + WHY.md): üç sütun (CelikPanel/cPanel/Plesk) —
  kurulum ayak izi, varsayılan güvenlik, tek binary vs servis ormanı, TR-birinci-sınıf, fiyat ilkesi;
  artı bilinçli eksiklerin gerekçeli itirafı (her ret bir DECISIONS kaydına link). Karşılaştırmayı
  rakip pazarlaması yapmadan biz yaparız — dürüstçe.
- **Hafif marka özelleştirme:** global "panel adı + logo + vurgu rengi" ayarı (tek tablo satırı;
  login + sidebar + posta şablonları tek kaynaktan okur). Bayi-başına marka 1.0-sonrasıdır.

**Çıkış ölçütü:** ≥3 dış operatör ≥1 ay gerçek site işletiyor; dokümantasyon sorularını bizden önce
cevaplıyor · temiz VPS'te SSH'a dönmeden tarayıcıdan admin oluşturulup giriş yapılıyor; token'sız/ikinci
deneme 403 · doğrulamasız e-postayla hesap açılamıyor; free hesap saatte N+1'inci postada sınırlanıyor ·
her panel sayfasından çalışan doküman bağlantısı var, kırık slug testi CI'da · beta duyurusu fiyat
ilkesi ve WHY sayfasıyla çıkmış · SUPPORT.md yayında ve v0.3.0→güncel atlama testi CI'da yeşil.

### 🎯 v1.0 — Genel Çıkış
- Temiz VPS → dakikalar içinde çalışan panel, dokümante, kendi kendini güncelleyen
- "Domain → canlı site" arka arkaya 100 kez hatasız (Faz 1 sözü, artık CI'da)
- cPanel'den taşınma defalarca kanıtlı · fiyatlandırma/lisans yayında

**Çıkış ölçütü:** dokümandan başka hiçbir yardım almayan bir yabancı, temiz VPS'te kurulumdan canlı
HTTPS'li siteye ve INBOX'a düşen postaya kendi başına ulaşıyor · "domain → canlı site" 100/100 CI'da ·
en az bir gerçek cPanel göçü ve bir CelikPanel→CelikPanel taşıma üretimde kanıtlı · fiyat/lisans sayfası
ve SUPPORT politikası yayında.

### 1.0 Sonrası — ufuk *(talep sürer, hayal sürmez)*
Her biri ancak gerçek talep varsa, bilinçli hedef-dışılara uygun biçimde:
- Çoklu sunucu (panel-arası güven modeli + **kardeş sunucu DNS oto-kaydı**: yeni bir CelikPanel sunucusu,
  marka domain'inin zone-otoritesi olan diğer CelikPanel'e kendi hostname A kaydını, operatörün verdiği
  API token'ıyla kaydettirir — bugün elle yapılan boston.celikhost.com adımının ürünleşmiş hali; v0.3'ün
  (b) dış-DNS yolu bu gelene dek N sunucuyu karşılar) · BSD agent arka ucu · faturalama entegrasyonları (WHMCS vb.)
- **Plesk/DirectAdmin içe aktarıcıları** — ancak gerçek talep (≥5 somut göç isteği) doğunca; cPanel
  içe aktarıcının inspect→onay→apply deseni yeniden kullanılır. O güne kadar dürüst cevap dokümante:
  "Plesk'ten geliyorsanız şimdilik elle taşıma kılavuzu."
- **Bayi-başına white-label** (özel giriş domain'i, bayi logosu) — hafif global marka v0.6–0.9'da;
  bayi-başına ancak bayi satışları bunu gerektirirse.
- **Hosted hizmet** (CelikPanel'in kendisinin VPS+panel kiralaması) — pazar yeri gibi peşin reddedilmez
  ama şimdi tasarlanmaz; açılırsa self-hosted ürünün mimarisine tek satır telemetri/phone-home taşımaz.
- Pazar yeri **asla** (AltaVista hatası).

---

## Bilinçli Hedef-Dışılar

Sadelik hayır diyebilmektir. Bunlar **bilerek** yok — ve retlerin çoğu ürünün ta kendisidir:

- ❌ **Docker/konteyner katmanı** — hedef pazar klasik hosting; doğrusu native. Rekabet cümlesi de budur:
  Plesk kurulumu yüzlerce paket ve düzinelerce servis getirir; CelikPanel iki binary getirir. Bu fark
  bir eksik değil, üründür.
- ❌ **Akla gelen her servise yönetim ekranı** — kurulu olmayan görünmezdir.
- ❌ **Tema/görünüm pazarları, portal vitrinleri, eklenti pazarı** — AltaVista hatası; ayrıca üçüncü-taraf
  kod çalıştırmama garantisi, "root Agent'a yalnız Panel erişir" ilkesinin doğal sonucudur. Eklenti
  pazarı bu garantiyi satar.
- ❌ **Genel uygulama kataloğu yarışı (Softaculous'un 400+ girişi)** — sığ-geniş katalog bakım
  karadeliğidir ve talep ezici biçimde WordPress'tedir. Katalog ancak (a) gerçek talep kanıtı ve
  (b) WordPress kalitesinde uçtan uca kurulum (resmi tarball, doğrulama, tam yapılandırma) **birlikte**
  sağlanınca tek tük büyür. Konum: WordPress'te herkesten derin, katalogda bilerek dar.
- ❌ **Tam ziyaretçi analitiği** (oturum, coğrafya, gerçek zamanlı) — v0.4'teki minimal log kartıyla
  yetinilir; ötesini isteyene dürüst cevap: siteye Plausible/GA koyun. AWStats klonu sadelik süzgecini geçmez.
- ❌ **Panel-içi terminal** — saldırı yüzeyi ve sadelik; SFTP/SSH anahtar yönetimi (v0.5) meşru ihtiyacı karşılar.
- ❌ **Çoklu sunucu / cluster (şimdilik)** — tek sunucu kusursuz olmadan dağıtık sistem hayali yok.
- ❌ **Panelin kendisi için dış bağımlılık** (Redis, harici DB, mesaj kuyruğu) — tek binary + SQLite kalır.
  Ödeme bile çekirdeğe gömülmez; panel yalnız webhook tüketicisidir (v0.5 kararı).
- ❌ **Telemetri / phone-home** — panel dış ağa kendi iradesiyle istek atmaz (operatörün kurduğu
  heartbeat ve gelen webhook hariç). Lisans modeli ne olursa olsun bu değişmez.
- ❌ **BSD desteği (şimdilik)** — **ama seçenek bilinçle korunuyor ve asla fork olarak değil.** Panel↔agent
  RPC sözleşmesi tasarım gereği OS-nötr: panel (HTTP/SQLite/UI/iş mantığı) zaten FreeBSD'ye çapraz
  derlenen taşınabilir Go; yalnız agent'ın "elleri" (systemd/apt/nftables) Linux'a özgü. Gerçek talep
  doğarsa (örn. hosting'çileri BSD'ye iten bir Linux güven krizi) hamle, aynı RPC yüzeyinin arkasına
  BSD agent arka ucu koymaktır — haftalarla ölçülen iş, tek ürün. İki CelikPanel asla olmayacak.
  Bunu ucuz tutan disiplin: yeni agent özellikleri "ne"yi (RPC yüzeyi) "nasıl"dan (exec çağrıları)
  ayrı tutar — kod zaten böyle yazılıyor. *(Karar: 8 Temmuz 2026.)*

---

## Tarihsel Durum — 29 Ağustos 2026

**Sürüm:** artık tek kaynaklı — sürüm ve commit bağlama anında HER İKİ binary'ye gömülüyor,
`/api/v1/panel/version` sunuyor, panelin alt bilgisi oradan okuyor ve panel ile agent farklı yapıdaysa
uyarı gösteriyor. Elle yazılmış "v0.1.0" metni silindi. İki test sunucusu aynı commit'te doğrulandı.
**Güncel canlı kimlik:** Önceki iki sunuculu eşleşen-commit gözlemi tarihsel
kanıttır; iki sunucunun güncel durumunu kanıtlamaz. Canlı değerleri
[tarihli canlı durum kaydında](docs/LIVE-STATE-2026-08-29.tr.md) yeniden doğrulayın.
**Merdivendeki yer:** v0.2 sürüyor; v0.2.5'in birçok kalemi yolda kapandı; **yeni v0.2.6 basamağı
v0.3'ün önüne girdi** (yukarıdaki üç veri-kaybı kusuru sebebiyle).

**Temmuz ikinci yarısında kapanan canlı kırıklar (hepsi iki sunucuda kanıtlı):** spam filtresi artık
gerçekten süzüyor (milter zinciri tek sahipli; GTUBE testi iki dağıtımda da reddedildi) · Arch'ta
Postfix gelen HER postayı reddediyordu (harita tipi artık keşfediliyor: lmdb/hash/btree/texthash) ·
WireGuard kurulunca gerçekten çalışıyor · agent artık istekten gelen yola/URL'e güvenmiyor
(yol beyaz listesi + sembolik bağ reddi; depo anahtarı kendi katalogdan) · yapılandırma editörü
yaz→doğrula→**geri al** yapıyor (nginx/postfix/dovecot/apache doğrulayıcıları) · koltuk kuralı artık
agent'ta uygulanıyor (arayüz engelliyordu, API kabul ediyordu) · takma ad unit'i artık kurulu
saymıyor · güncelleme betiği artık bayat yapı kurmuyor.

**Yeni yetenekler:** her yönetim sayfasında **Yardım** düğmesi (21 bileşen için özel içerik + türe
göre 3 genel yedek, tamamı TR+EN; yardımsız sayfa yapısal olarak imkânsız) · boş yönetim sayfası
yasağı (D-019) · dağıtım destek matrisi **katalogdan üretiliyor** ve bayatlarsa test kırmızı yanıyor ·
"bu dağıtımda sunulmuyor" rozeti · izleme sayfası · Valkey kataloğa eklendi (yeni bileşenin gerçek
maliyeti: 2 kaynak dosya, sıfır Go kodu).

**Borç durumu:** üretilen OpenAPI istemcisi, tek bildirimsel route/authz tablosu, arayüz tekleştirmesi
ve ölçülmüş uçtan uca gecikme açık. D-017 kanonik teklif kimliklerini tanımlıyor; kalıcı Mağaza satırları
ise hâlâ düz eski kimlikleri kullanıyor ve geçiş açık bir veri/API uyumluluk migration'ı gerektiriyor.
CI artık yalnız derlemiyor: Go biçim/derleme/vet/test/race, shell ve depo sözleşmeleri, kilitli web
derlemesi ve bağımlılık kapısı ile yeniden üretilebilir sürüm ürünlerini denetliyor.

**Depo anlık görüntüsü:** Ölçüm gerektiğinde tam commit üzerinde
`bash tools/repo-metrics.sh` çalıştırılır. Çıktı, üretilen kaynak-ağaç
kanıtıdır; satır, dosya, rota, migration veya komut-yeri sayıları bu yol
haritasına elle kopyalanıp tutulmaz. Komut çıktısını tam commit'iyle birlikte
inceleme veya release kanıtına kaydedin. Bu ölçüm iki sunucunun dağıtılmış
build'ini kanıtlamaz.


---

## Bu Belge Nasıl Güncellenir

- **Her önemli karar commit ile buraya işlenir.** Gerekçe [DECISIONS](docs/DECISIONS.tr.md)'a, borç
  [AUTOPSY](docs/AUTOPSY.tr.md)'ye; bu dosyaya yalnız merdivendeki yeri ve ölçütü yazılır.
- Yeni istek buraya **"ileride" diye giremez**: ya bir sürüm basamağına ölçülebilir çıkış ölçütüyle
  yazılır ya da Bilinçli Hedef-Dışılar'a gerekçesiyle eklenir. Üçüncü yol yok.
- Çıkış ölçütü karşılanmadan sonraki basamağa geçilmez; bir ölçüt değişecekse değişiklik gerekçesiyle
  birlikte commit'lenir (sessiz sulandırma yok).
- Her işlemede "Son güncelleme" tarihi ve "Neredeyiz" bölümü tazelenir; İngilizce eş (ROADMAP.md)
  aynı commit'te güncellenir.
