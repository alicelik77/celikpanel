# v0.1.0-alpha.99 (22 Ekim 2026'da yayımlandı)

[English](RELEASE-NOTES-v0.1.0-alpha.99.md)

Bu sürüm v0.1.0-alpha.98'in ardından gelir. 22 Ekim 2026'da yayımlandı (sonda
"Yayım kaydı"na bakın); henüz hiçbir parçası bir kurulu sunucuda değil.
2026-10-22'de, aday olarak, `feat/alpha99-own-email` dalında, alpha.98'in yayım
kaydından sonraki `main` üzerinden yazıldı.

## Kim güncellemeli ve neden

Bu sürüm yayımlandığında alpha.98'i ya da daha eski bir sürümü çalıştıran herkes.
Şimdiye kadar kimse panelde kendi e-posta adresini değiştiremiyordu: bir yöneticinin
adresi, yönetici sunucuda oluşturulurken verilen adresti. Panel sertifikası bu adresi
sertifika yetkilisine iletişim adresi olarak verir; bu yüzden kullanılabilir bir
adresi olmayan yönetici takılıp kalıyordu: alpha.98'den beri panel bunun için henüz
bir ekran olmadığını dürüstçe söylüyordu. Bu sürüm o ekranı ekler (D-037
değişikliği); sahip yapılmasını 22 Ekim 2026'da onayladı. Bir sunucuyu yalnız sahibi,
panelin kendi güncelleme ekranından günceller.

## Ekranda ne değişir

**Hesabım'da bir "E-posta adresim" kartı var.** Sağ üstteki menüyü (kullanıcı
adınız) açın → Hesabım. Parola ile İki adımlı giriş arasında üçüncü bir kart vardır:
"E-posta adresim" ("My email"). Güncel adresinizi gösterir. Adresi okurken bunu
söyler; okuma başarısızsa adresin okunamadığını söyler ve "Tekrar dene" sunar.
`/account#email` adresi sayfayı bu kartta açar.

**Değiştirmek.** "E-postayı değiştir" düğmesi küçük bir pencere açar: yeni adresiniz ve mevcut
parolanız. Enter kaydeder, Escape kapatır. Kaydederken düğme döner bir gösterge
gösterir, alanlar ve düğmeler kilitlenir ve bir satır değişikliğin kaydedildiğini ve
başka bir şey gerekmediğini söyler. Bittiğinde "E-posta adresiniz değiştirildi."
yazısını görürsünüz ve kart yeni adresi gösterir. Oturumunuz sürer: giriş adresinizle
değil kullanıcı adınızla yapılır.

**Uzun sürerse.** Pencere en çok 20 saniye bekler. Sonra şunu söyler: "Yeni
adresinizin kaydedilip kaydedilmediği henüz bilinmiyor. Bu bir hata değil. Bu
sayfadaki adres yeniden okundu; hâlâ eskisiyse yeniden deneyin." Bağlantı kopup yanıt
yeniden istense bile tek bir tıklama adresinizi asla iki kez değiştirmez.

**Bir şey doğru değilse hiçbir şey değiştirilmez ve pencere ne yapılacağını söyler:**

- "Bu, panelin kullanabileceği bir e-posta adresi değil; hiçbir şey değiştirilmedi.
  name@example.com gibi tek bir adres girin; İngilizce dışı harfler içeren bir alan
  adı ASCII biçimiyle (xn--…) yazılır."
- "Bu zaten e-posta adresiniz; hiçbir şey değiştirilmedi."
- "Bu e-posta adresini başka bir hesap kullanıyor; hiçbir şey değiştirilmedi. Başka
  bir adres girin."
- "Mevcut parolanız doğru değil; hiçbir şey değiştirilmedi. Giriş yaptığınız parolayı
  girin."
- "Çok fazla deneme yapıldı; hiçbir şey değiştirilmedi. Birkaç dakika bekleyip
  yeniden deneyin."
- "Bu kaydedilirken hesabınız başka bir yerde değiştirildi; hiçbir şey
  değiştirilmedi. Sayfayı yenileyip yeniden deneyin."
- "İstek okunamadı; hiçbir şey değiştirilmedi. Sayfayı yenileyip yeniden deneyin."

**Ekip üyeleri de.** Ek kullanıcının (ekip üyesi) Hesabım sayfası yoktur; kişisel
menüde "Parolayı değiştir"in yanında "E-postayı değiştir" öğesini bulur. Aynı
pencereyi açar. Yöneticiler, bayiler ve müşteriler kartı kullanır.

**Panel sertifikası ve sunucu kurulumu oraya yönlendirir.** Yöneticinin
kullanılabilir bir adresi yoksa panel sertifikası ve sunucu kurulum planı şunu
söyler: "Yönetici hesabının geçerli bir e-posta adresi yok; sertifika yetkilisi bunu
iletişim adresi olarak ister. Hiçbir şey istenmedi. E-posta adresinizi Hesabım'da
(sağ üst menü) ayarlayın, sonra yeniden deneyin." ve yanında kartı açan bir "Hesabım’ı aç"
düğmesi vardır. Adresinizi değiştirmek kendiliğinden hiçbir sertifika istemez
ya da yenilemez: hazır olduğunuzda sertifikayı yeniden isteyin. Değişiklikten önce
incelediğiniz bir sunucu kurulum planı yeniden incelenmelidir; yeni inceleme yeni
adresi kullanır. Zaten çalışan bir kurulum başladığı adresi korur.

**Telefonda ret düğmeleri.** Çözüldüğü yeri adlandıran bir retin yanındaki düğme
("Hesabım’ı aç" gibi) artık telefonda panelin her yerinde en az 44 px yüksekliğindedir.
Tarayıcı denetimi 24 onu 28 px buldu.

## Altta ne değişir

- **Tek yeni rota,** `POST /api/v1/auth/password`'ün yanında `PUT
  /api/v1/auth/email`. Gövde `{"email", "current_password"}`, anahtarlar tam olarak
  böyle yazılır; başka her anahtar (farklı yazılmış biri, `user_id`, `id`) ya da
  birden çok JSON nesnesi reddedilir. Kendi parolasını değiştirebilen her role açık:
  yönetici, bayi, müşteri ve ek kullanıcı.
- **Mevcut parola gerekir.** Her deneme, parola değiştirmeyle paylaşılan tek bir
  yeniden kimlik doğrulama sınırına (`reauth`, hesap ve istemci adresi başına, 5
  dakikada 10) sayılır: `403
  account_email_wrong_password`, sonra `429 account_email_too_many_attempts`.
- **Her eylemin tek kimliği (D-029).** İstek kimliği koruması artık bu rotayı korur;
  koruduğu ilk `PUT`'tur. Aynı kimlik ilk gelişten (`X-CelikPanel-Request-Replayed:
  1`) yanıtlanır ve adresi asla iki kez değiştirmez; başka bir adresle aynı kimlik
  `409 REQUEST_ID_REUSED`; kimliksiz bir istek `428 REQUEST_ID_REQUIRED`'dır. Süre
  sınırı 30 saniye. Korumanın sakladığı özet yalnız yeni adresin yeniden kodlanmış
  hâlidir ve başarı yanıtı adres taşımaz; bu yüzden saklanan satırlar ne parolayı ne
  de adresi tutar. Sonucu bilinmeyen bir gönderiden sonra pencerenin aynı adres için
  sonraki Kaydet'i aynı kimliği taşır (Panel'in o kimlik için kesin olan `REQUEST_OUTCOME_UNKNOWN` yanıtından sonra değil: sonraki Kaydet o zaman yeni bir eylemdir).
- **Tek doğrulayıcı.** `mutationpayload.CanonicalContactEmail`, panel sertifikasının
  var olan iletişim denetimidir, dışa açılıp yeniden kullanıldı: boşluklar atılır,
  yalnız yazdırılabilir ASCII, tam bir `@`, en çok 64 baytlık yerel kısım ve en çok
  254 baytlık bütün, küçük harfle saklanan tam nitelikli bir alan adı, görünen ad yok.
  Burada kabul edilen bir adres sertifika isteğinin de her zaman kabul ettiği bir
  adrestir.
- **Bir adres, bir hesap.** Başka bir hesabın kullandığı adres, büyük-küçük harf
  gözetmeden karşılaştırılarak `409 account_email_taken`'dır; `users.email`
  üzerindeki `UNIQUE` kısıtı (`001_full_schema.sql`'den beri şemada) son sözü söyler.
  Adresin zaten sizin mi (`409 account_email_unchanged`) yoksa başkasının mı olduğu
  ancak parola kanıtlandıktan sonra söylenir.
- **Başka yerde yapılan bir değişikliğin üstüne yazılmaz.** Yazım, doğrulanan adres ve
  parola özeti üzerinde karşılaştır-ve-yaz biçimindedir: `409
  account_email_changed_elsewhere`.
- **Oturumlar sürer; denetim izi adresi değil hesabı adlandırır.** Kullanıcı üzerinde
  tek bir `account.email.changed` kaydı; Panel sayfasındaki "Son etkinlik" "kendi
  e-posta adresini değiştirdi" der.
- **Panel sertifikası**, giriş yapmış yöneticinin adresini istendiğinde veritabanından
  okur (önbellek yok). `panel_certificate_contact_email_invalid` reti yeniden
  `/account#email` eylemini taşır. Sunucu kurulum planı incelendiği iletişim adresini
  planın kimliğinde tutar.
- **Lisans kapısı değişmedi.** E-posta değiştirme, lisansı kilitli bir panelin hâlâ
  kabul ettiği rotalara eklenmedi (parola değiştirme onlardan biridir): panel lisans
  nedeniyle kilitliyken diğer rotalar gibi reddedilir. Kilitli bir panelin neyi kabul
  ettiği lisans politikasıdır, bu sürümün dışındadır.

## Neyin değişmediği

- Agent, güncelleyici, şema, yoklama, bileşen ya da katalog parçası bölünmesi
  değişikliği yok. Yeni dizin yok; şema 44, geçiş yok.
- Güncelleyicinin değiştirme yoluna (`update.sh`, `get.sh`, sistem güncelleme işçisi,
  geri alma ve kurtarma) dokunulmaz; bu yüzden sahibin 16 Ekim 2026 talimatı uyarınca
  gerçek sistem kaydı yapılmaz ve sürüm kurtarma düzeneği gerekmez: güncellemeyle
  ilgili hiçbir dosya değişmedi.
- Eski ya da yeni adrese posta gönderilmez; bir e-posta değişikliği hiçbir sertifikayı
  istemez, yenilemez ya da yeniden denemez.
- Barındırılan web siteleri, posta, DNS, veritabanları, zamanlanmış işler ve sertifika
  yenilemesi bir panel kullanıcısının adresine bağlı değildir.
- Kurulu paneller yalnız sahipleri tarafından, panelin kendi güncelleme ekranından
  güncellenir.

## Sahibin bildirdiği

Sahip 22 Ekim 2026'da iki kurulu sunucunun da alpha.98'de olduğunu bildirdi:
Boston'un ve Frankfurt'un Panel sayfaları yeni menüyü gösteriyor. Frankfurt'un
alpha.94'ten güncellemesi tamamlandı; daha önceki alpha.95 güncellemesi salt-okur
denetimde meşgul bir paket yöneticisi yüzünden durmuştu (Debian'ın kendiliğinden
paket güncellemeleri, alpha.97 ile kaydedildi). Bu, alpha.98 yayımının iki açık
maddesini yanıtlar. Frankfurt'un Bileşenler'deki paket sürümleri bildirilmedi; bu
yüzden "her bileşen için kurulu paket sürümlerini oku" açık işi Frankfurt için açık
kalır.

## Güvenlik incelemesi düzeltmeleri

Adayın bağımsız bir güvenlik incelemesi bunları 22 Ekim 2026'da, hiçbir yayımdan
önce istedi; kaynaktadırlar.

- **İki iletişim kapısı da tek doğrulayıcıyı kullanır.** Panel sertifikası isteği
  ve kurulum planının iletişim engeli yöneticinin adresini Go'nun
  `mail.ParseAddress`'iyle denetliyordu; bu, sertifika isteğinin kendi denetiminin
  reddettiği `admin@localhost`, `root@myhost` ya da ASCII dışı bir adresi kabul
  eder. Böyle bir adres (`--create-admin` verilen adresi denetlemediği için
  mümkündür) kapıdan geçiyor ve sonra Hesabım yönlendirmesi olmadan başarısız
  oluyordu. İki kapı da artık `CanonicalContactEmail` kullanır: sertifikanın
  reddedeceği her adres `/account#email` eylemli kodlu reddi ve kurulum engelini alır.
- **Saklanan özet yalnız yeni adresi tutar.** Go'nun JSON çözümü anahtarları
  büyük-küçük harf gözetmeden eşler (ve `ſ`'yi `s`'ye katlar); bu yüzden
  `"Current_Password"` taşıyan bir gövde parola alanına ulaşıyor, özet ise yalnız tam
  `current_password` anahtarını çıkarıyordu. Gövdenin anahtarları artık tam olarak
  `email` ve `current_password` olmalıdır; başka her anahtar (`Current_Password`,
  `user_id`, `id`, …) `400 account_email_request_invalid`'dır. Özet yalnız yeni
  adresin yeniden kodlanmış hâlidir; okunamayan bir gövde sabit bir özet alır.
- **Başarı yanıtı adres taşımaz** (koruma yanıtları 24 saat saklar); sayfa adresi
  yeniden okur.
- **Tek yeniden kimlik doğrulama sınırı.** E-posta değiştirme ve parola değiştirme,
  kullanıcı ve istemci adresi başına tek bir sınırlayıcı anahtarını (`reauth`)
  paylaşır; çalınmış bir oturum ikisi için tek bir tahmin bütçesi alır (5 dakikada
  10), rota başına bir değil.
- **Sonucu bilinmeyen bir gönderiden sonra aynı adresin sonraki Kaydet'i aynı
  kimliği taşır**, kesin bir yanıt gelene dek (Panel'in o kimlik için kesin olan `REQUEST_OUTCOME_UNKNOWN` yanıtından sonra değil: sonraki Kaydet o zaman yeni bir eylemdir); başka bir adres
  yeni bir eylemdir.
- **Kullanılamayan adres reddi**, İngilizce dışı harfler içeren bir alan adının ASCII
  biçimiyle (`xn--…`) yazıldığını söyler.
- Eklenen testler: `admin@localhost`, `root@myhost` ve ASCII dışı bir adresle iki
  kapı; `Current_Password`, `current_paſsword`, `user_id` ve `id` taşıyan gövdeler;
  "kullanılıyor" ve "zaten sizin" yanıtlarından önce söylenen yanlış parola; bir
  müşterinin yerine geçen yöneticinin müşterinin parolası olmadan reddi; başka
  parolayla gelen aynı kimliğin saklanan yanıtla yanıtlanması; iki rotanın paylaştığı
  sınır; bilinmeyen sonuçtan sonra aynı kimlik (web).

Kaydedilen bilinen sınırlar: yalnız büyük-küçük harfle ayrılan iki adrese aynı anda
yapılan iki değişiklik, harf gözetmeyen denetimden birlikte geçebilir, çünkü şemanın
`UNIQUE` kısıtı tam karşılaştırır; bu sürümde harf gözetmeyen bir dizin eklenmez.
Sertifika yetkilisi herkese açık olmayan bir alan adındaki (`.test`, `.local`,
`.internal`, `example.com`) adresi yine reddedebilir: panel biçimi denetler, alan
adına yetkili karar verir. `--create-admin` verilen adresi hâlâ denetlemez; panelin
kapıları artık onu yakalar.

## Bilinen eksikler

- **Alan adına sertifika yetkilisi karar verir.** Panel adresin biçimini denetler;
  Let's Encrypt herkese açık olmayan bir alan adındaki (`.test`, `.local`,
  `.internal`, `example.com`) adresi yine reddedebilir. Herkese açık internette
  posta alan bir alan adında bir adres kullanın.
- **Kurulu bir sunucuda hiçbir şey ölçülmedi.** E-posta değiştirme yalnız bileşen,
  bağlanmış ve taklit tarayıcı testleriyle denendi.
- **Yeni adres bir iletiyle doğrulanmaz.** Panel yeni adrese posta göndermez; kişi kim
  olduğunu parolasıyla kanıtlar.
- **Panel lisans nedeniyle kilitliyken** e-posta değiştirme diğer rotalar gibi
  reddedilir (parola değiştirme erişilebilir kalır).
- **"Açık oturumlar"** hâlâ yapılmadı; üründe oturum listesi yok.
- **alpha.99'dan alpha.98'e geri dönüş.** alpha.99'da değiştirilen bir adres kalır;
  alpha.98'in onu yeniden değiştirecek bir ekranı yoktur ve sertifika reti bunu söyler.
  Hiçbir şey kaybolmaz ve hiçbir şeyin onarılması gerekmez.
- **Frankfurt'un paket sürümleri** bildirilmedi.
- P0.1-P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez; aday kabul
  değildir.

## Kanıt

Aşağıdakilerin hepsi 2026-10-22'de aday ağaçta bir geliştirme ortamında çalıştırıldı;
hiçbiri bir kurulu sunucuda çalıştırılmadı.

- Go: `CelikPanel-S2-Debian` konuğunda aday ağaçta `go test ./cmd/panel/ ./internal/...` (-timeout 45m, CGO kapalı): 64 paket geçti, 2895 üst düzey test geçti, 42 atlandı, 0 başarısız (4279 alt test geçti); gofmt ve go vet temiz. Yeni testler `cmd/panel/account_email_handler_test.go` ve
  `internal/mutationpayload/panel_certificate_issue_test.go` içinde: dört rol için
  değişiklik (oturumlar sürer, tek denetim satırı, içinde adres yok); hiçbir şeyi
  değiştirmeyen her ret (geçersiz biçimler, aynı adres, başka harf büyüklüğüyle dahil
  alınmış bir adres, yanlış ya da eksik parola, bilinmeyen alan, iki JSON nesnesi,
  kimliksiz ya da bozuk kimlikli istek, yanlış yöntem); CSRF denetimiyle engellenen
  başka kökenden ve kökensiz `PUT`; kilitlenmeye sayılan yanlış parolalar (testin
  üçüncü denemesi doğru parolayla bile reddedilir; başka bir hesap etkilenmez); tek
  kimliğin yinelenmesi (aynı yanıt, tek denetim satırı; başka adresle yeniden
  kullanılan kimlik reddedilir; saklanan özette parola yok); eski bir okumaya karşı
  karşılaştır-ve-yaz; korumasız çalışmayı reddeden işleyici; rota kapıları; ve
  `/account#email` ile reddedilen, değişiklikten sonra yeni adresi taşıyan ve
  değişikliğin hiçbir sertifika istemediği panel sertifikası isteği; ayrıca
  doğrulayıcı.
- Web: `npm test` 1709 testin 1709'u geçti (yeni `tests/own-email-mounted.test.mjs`: İngilizce ve Türkçe 11 test; Ayarlar'da sertifika retinin bandı için yeni bir bağlı test; ek kullanıcının menü öğesi), `npm run build` her bütçenin içinde geçti (kritik açılış yolu 361 KiB'nin 329,93'ü).
- Sürüm kurtarma düzeneği: güncellemeyle ilgili hiçbir dosya değişmediği için
  çalıştırılmadı (bu sürümün dokunmadığı güncelleme kartının metinlerini ve sonuç
  mantığını yansıtır).
- Dağıtım sözleşmeleri: `deploy/test-release-sequence-policy.sh` Git Bash'te aday
  çalışma ağacına karşı geçti (98 -> 99); `deploy/test-download-portal-contract.sh`
  ve `deploy/test-signed-release-manifest-contract.sh` `CelikPanel-S2-Debian`
  konuğunda adayın izlenen dosyalarının bir kopyasında geçti.
- Tarayıcı denetimi 24 (sahte yanıtlar, yalnız yerel adres), e-posta kartının
  durumları; İngilizce ve Türkçe, açık ve koyu, 1440 ve 390 px: 8 yapılandırmanın her birinde 13 durum (104 ekran görüntüsü), iki kez bakıldı: ilk bakış telefonda ret bandının düğmesini 28 px buldu, artık en az 44 px; ikinci bakış başka bir şey bulmadı; kişisel menüdeki dil seçici (telefonda 28 px) bu sürümden eskidir ve değişmedi; ekran görüntüleri ve tarayıcı sürücüsü sonra silindi.

Bileşen ve bağlanmış testler gerçek sistem davranışını kanıtlamaz.

## Nasıl güncellenir

Bu sürümü yalnız CelikPanel'in güncelleme arayüzü (Ayarlar → CelikPanel
güncellemeleri) üzerinden kurun. Yayımlamak kurulu sunucuları güncellemez; bir
sunucunun güncellemesini yalnız sahibi, panelin kendi güncelleme ekranından başlatır.

## Yayım kaydı

Aşağıdaki her şey 22 Ekim 2026 tarihlidir (saat dilimi belirtilmedikçe UTC).
Birleştirmeyi, etiketi, denetimleri, portal yayımını ve genel sitenin okunmasını
sürümü hazırlayan kişi yaptı, önceki sürümlerde olduğu gibi sahibin kalıcı
talimatı uyarınca; kurulu panel güncellemelerini yalnız sahip, panelin kendi
güncelleme ekranından başlatır. Yayım için sahibin talimatıyla gerçek sistem
kaydı yok. Bu girdi, aşağıdaki sonuçların hiçbirini depoda bir dosya olarak
saklamaz.

**Ne oldu.**

1. Sürümden önce iki bağımsız güvenlik incelemesi turu yapıldı. İlki iletişim
   kapılarının fazla gevşek olduğunu, büyük-küçük harfi yok sayan anahtarlarla
   bir parolanın tekrar özetine ulaşabildiğini, eksik bir sıralama testini,
   saklanan yanıtta adresin bulunduğunu ve ayrı sınırlayıcı sayaçlarını buldu.
   İkincisi zorunlu bir düzeltme buldu: `REQUEST_OUTCOME_UNKNOWN` sonrasında
   pencere "sonuç bilinmiyor" durumunda döngüye giriyordu. Hepsi düzeltildi
   (yukarıdaki "Güvenlik incelemesi düzeltmeleri"ne bakın).
2. "Release v0.1.0-alpha.99: change your own email on My account" başlıklı #236
   numaralı çekme isteği `main`'e `7ce52a17b416f8ffcf64931e08a2c3234bfa2f51`
   commit'i olarak birleştirildi; yazarı Mehmet Ömer Efe Çelik. Sürüm sırası
   değişkeni etiket yapılmadan önce 98'den 99'a geçirildi. `main` koşusu ve
   etiket koşusu başarılı oldu; sürüm zinciri hiçbir olay olmadan çalıştı.
3. `v0.1.0-alpha.99` açıklamalı etiketi (nesne
   `e9de364565e1e87c92f3e419369fda1f78bc1708`, ağaç
   `971776526f398812b66921482f8d779e57c132a9`, etiketleyen Mehmet Ömer Efe Çelik)
   sürüm commit'ini gösterir. Sürüm bir ön sürümdür (pre-release);
   2026-10-22T13:07:25Z'de altı dosyayla yayımlandı.
4. 22 Ekim 2026'da `CelikPanel-S2-Debian` WSL konuğunda bir indirmeden
   denetlendi (hazırlayan kişi; burada bir dosyada kayıtlı değil): arşivler bayt
   bayt aynı, her biri 66.326.418 bayt, SHA-256
   `b749b7f469cf4d78c3b3ce7bc8cd0f9ef8f864a96f097f8b4258810d359f68e6`; manifest
   sıra 99, `published_at` 2026-10-22T12:23:11Z der (SHA-256
   `aac97d747e4f962454c29e78f1629c22d701181e6fe3d121cc9b2bb53ff08c4e`); imzası
   (SHA-256 `6493e9be97965df22d5476adfdc054a29f111bef6761c3eb0e01d0a15ba62afa`),
   imza anahtarına (SHA-256
   `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`, öncekiyle
   aynı anahtar) karşı `openssl pkeyutl -verify -rawin -pubin` ile doğrulanır;
   iki içerik koruması da 0 döndürdü; derleme Go go1.26.5, CGO kapalı,
   `-trimpath` ile yapıldı. `libexec/get.sh` 99'u sabitler (SHA-256
   `dc11adc2096222242fa10e755926356fd076a3364967283fe3a835362c390768`); içindeki
   politika şimdiki 99, önceki 98, önceki commit
   `c249a9ba236212bfcdf6eddb590f55bccc5bac9c` der. Arşivde 267 üye var (243
   dosya, 24 dizin). Arayüz paketi `web/dist/assets/app-CfcdYww_.js`, 259.484
   bayt, SHA-256
   `d0ea016979b893250bb710062bc8926eae03ac1ab336a6be62d99947657dc6a3`.
   `bin/panel` çalıştırılabilir dosyasında `account_email` var.
5. Portal paketi: `portal.tar.gz`, 133.025.091 bayt, SHA-256
   `95fcb5c61dc0c15321078bb7e9b36a6722e0fe629fe228541faf75f5566937a3`, 54 girdi;
   iki kez bayt bayt aynı derlendi (önceden imzalı kip). Sayfaları yalnız
   `github.com/alicelik77/...` adresine bağlanır. Sözleşme testi 0 döndürdü;
   genel doğrulayıcının 17 testi ve terfi aracının 14 testi geçti.
6. Yaklaşık 13:16Z'de hazırlayan kişi onu yayımladı: önceki sürüm
   v0.1.0-alpha.98, hedef v0.1.0-alpha.99; durum `committed`, genel doğrulama 26
   istekten 26'sı, `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`.
7. Önceki sitenin yedeği
   `/var/www/vhosts/celikpanel.net/portal-backups/httpdocs.before-v0.1.0-alpha.99.20261022T131614Z.342386.fc06eb2d06051adc`;
   yüklenen paket
   `/var/www/vhosts/celikpanel.net/.upload-portal-20261022T131537Z-v0.1.0-alpha.99-3546cbb9f47a93b5`
   altında saklandı.
8. Yayımdan sonra dışarıdan okuma: `https://celikpanel.net/releases/latest.txt`
   v0.1.0-alpha.99 der; `latest.json` sıra 99'u, yukarıdaki commit ve SHA-256'yı
   gösterir.

**Sahibin bildirdikleri.** İki kurulu sunucunun da alpha.98'de olduğu yukarıda
("Sahibin bildirdiği") kayıtlı; burada yinelenmez. alpha.99'dan henüz hiçbir şey
bir kurulu sunucuda değil.

**Yapılmayanlar ve yayımlamanın değiştirmedikleri.**

- Kurulu sunucuları sürümü hazırlayan kişi alpha.99'a güncellemedi. Onları yalnız
  sahip, panelin kendi güncelleme ekranından günceller.
- E-posta değiştirme bir kurulu sunucuda görülmedi.
- Yukarıda kayıtlı sınırlar duruyor: yalnız büyük-küçük harfle ayrılan adreslere
  aynı anda yapılan iki değişiklik birlikte geçebilir; Let's Encrypt herkese açık
  olmayan bir alan adındaki adresi reddedebilir; `--create-admin` verilen adresi
  doğrulamaz.
- Kurulum planının iletişim engeli her plan için hâlâ geçerli. Öyle olması gerekip
  gerekmediği sonraki bir karara bırakıldı.
- Yeni adres bir iletiyle doğrulanmaz ve "açık oturumlar" yapılmadı.
- Bildirilmeyen ve kaydedilmeyenler: Boston ya da Frankfurt'tan alpha.99'a dair
  her şey, Frankfurt'un paket sürümleri ve önceki notların bildirilmedi diye
  saydığı her şey. Her biri "kaydedilmedi"dir; "çalıştı" da değildir,
  "başarısız" da.
- Önceki yayımlama kayıtlarının sınırları ([alpha.88
  notları](RELEASE-NOTES-v0.1.0-alpha.88.tr.md)) kalkmış olarak bildirilmedi.
- Yayımlanma kabul değildir. P0.1–P0.5 kısmi kalır; hiçbir P0 işi kapanmaz ya da
  ilerlemez.
