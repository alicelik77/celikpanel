# v0.1.0-alpha.99 (aday, 22 Ekim 2026)

[English](RELEASE-NOTES-v0.1.0-alpha.99.md)

Bu sürüm v0.1.0-alpha.98'in ardından gelir. Bir adaydır: bu yazılırken
etiketlenmemişti ya da yayımlanmamıştı ve hiçbir parçası bir kurulu sunucuda
değildi. 2026-10-22'de, `feat/alpha99-own-email` dalında, alpha.98'in yayım
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
  sonraki Kaydet'i aynı kimliği taşır.
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
  kimliği taşır**, kesin bir yanıt gelene dek; başka bir adres yeni bir eylemdir.
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

- Go: `CelikPanel-S2-Debian` konuğunda aday ağaçta `go test ./cmd/panel/ ./internal/...` (-timeout 45m, CGO kapalı): 64 paket geçti, 2894 üst düzey test geçti, 42 atlandı, 0 başarısız (4279 alt test geçti); gofmt ve go vet temiz. Yeni testler `cmd/panel/account_email_handler_test.go` ve
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
- Web: `npm test` 1708 testin 1708'i geçti (yeni `tests/own-email-mounted.test.mjs`: İngilizce ve Türkçe 10 test; Ayarlar'da sertifika retinin bandı için yeni bir bağlı test; ek kullanıcının menü öğesi), `npm run build` her bütçenin içinde geçti (kritik açılış yolu 361 KiB'nin 329,93'ü).
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
