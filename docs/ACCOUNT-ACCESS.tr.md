# Hesap erişimi: parolamı unuttum, davetler, e-posta onayı

*v0.1.0-alpha.114. English: [ACCOUNT-ACCESS.md](ACCOUNT-ACCESS.md).*

Müşteriler ve bayiler kendi girişleriyle kendileri ilgilenir. Kimse başkasının
parolasını seçmez ya da görmez.

## Panel hesap postasını nasıl gönderir

Panelin kendi posta sistemi yoktur ve dış bağımlılık eklenmez. Her ileti
**bu sunucunun kendi posta sistemine** `127.0.0.1:25` üzerinden SMTP ile verilir
(Postfix sunucunun kendisinden gelen postayı kabul eder). Bu özellik posta
sisteminde hiçbir şeyi yapılandırmaz ya da değiştirmez.

Hesap postası yalnız üç koşul birlikte sağlanınca çalışır; sağlanmazsa ekran
hangisinin eksik olduğunu ve kimin yardım edeceğini söyler (`reason`):

| reason | anlamı | kim yapar |
| --- | --- | --- |
| `no_mail_server` | 127.0.0.1:25 üzerinde SMTP yanıt vermiyor | yönetici posta yığınını kurar ya da geçici parola belirler |
| `no_panel_address` | panel sertifikasında alan adı yok; bağlantılar panele gösterilemez | yönetici Sunucu kurulumunda panel adresini tamamlar |
| `no_sender` | gönderen alan adı yok (kurulumun ana alan adı, kayıtlı posta adı ya da panel adı) | yönetici sunucu kurulumunu tamamlar |

Bağlantılar panel sertifikasındaki adı ve **yapılandırılmış** panel portunu
(`CELIKPANEL_LISTEN`) kullanır; isteğin `Host` başlığını asla kullanmaz.
İletiler `no-reply@<ana alan adı>` adresinden gelir. Panel yalnız "posta
sistemine verildi" bilgisini bilir; iletinin gelen kutusuna ulaşıp ulaşmadığını
bilmez.

Yapılmadı: isteğe bağlı dış SMTP aktarıcısı (sunucu, port, kullanıcı, parola).
Yerel posta sistemi olmayan sunucuda yukarıdaki yönlendirme kullanılır.

## Parolamı unuttum

1. Giriş kartında **Parolamı unuttum** vardır. Önce
   `GET /api/v1/auth/recovery/mail-status` okunur; posta gönderilemiyorsa kimse
   bir şey yazmadan söylenir.
2. `POST /api/v1/auth/recovery/request {identifier, lang}` kullanıcı adı ya da
   adres alır ve **her zaman `202 {"accepted":true}` yanıtını verir**: bilinmeyen
   ad, askıdaki hesap, onaysız adres, açık daveti olan hesap ve gerçek eşleşme
   için aynı. İleti yanıttan sonra verilir; süre de bir şey söylemez.
3. Sıfırlama yalnız **onaylı** adrese gider — yönetici dahil her rol için.
   Onaylı adresi olmayan yönetici sunucuda panelin yönetici oluşturma
   komutuyla kurtarır (hesap kalır, yalnız parolası değişir).
4. Bağlantı (`/account-link/reset#t=<jeton>`) **bir kez, 30 dakika** çalışır.
   Sayfa bağlantıyı denetler, yeni parolayı iki kez sorar ve kaydeder.
5. Kayıt parolayı yeni bir kimlik dönemiyle yazar, **o hesabın bütün
   oturumlarını** ve bekleyen iki adımlı girişlerini siler, öteki açık
   bağlantılarını geçersiz kılar ve kişiyi kendiliğinden oturum açtırmaz. İki
   adımlı doğrulama açık kalır.

Sınırlar: istemci adresi başına 15 dakikada 5 istek (sonra `429`); hesap başına
saatte 3 sıfırlama iletisi (fazlasında yanıt aynıdır, hiçbir şey gönderilmez);
istemci adresi başına 15 dakikada 30 bağlantı okuma ve kullanma.

## Davetler

- `POST /api/v1/users` ile `"invite": true` (yönetici, bayi) ya da
  `POST /api/v1/team-members` ile `"invite": true` (müşteri, ek kullanıcı için)
  hesabı **parolasız** (kimsenin görmediği rastgele bir parolayla) ve **7 gün**
  geçerli bir davet bağlantısıyla açar. `invite` ile birlikte parola verilirse
  reddedilir.
- Sunucu posta gönderebiliyorsa bağlantı e-postayla gider ve yanıt
  `delivery: "sent"` der. Gönderemiyorsa — ya da posta sistemi iletiyi kabul
  etmezse — yanıt yalnız davet edene, bir kez, `link_path` ve `reason` verir;
  sayfa tam bağlantıyı davet edenin kullandığı adresle kurar.
- `GET /api/v1/invitations` çağıranın yönettiği açık davetleri listeler
  (yönetici: yönetici olmayan her hesap; bayi ve müşteri: `parent_id`'si
  kendisi olan hesaplar). `POST /api/v1/invitations/{id}/resend
  {delivery: "email"|"link"}` yeni bağlantı yapar, eskisini geçersiz kılar;
  `DELETE /api/v1/invitations/{id}` iptal eder (hesap kalır). Başka bir yolla
  parola almış hesap yeniden davet edilmez.
- Kabul (`/account-link/invitation`) parolayı belirler; e-postayla giden davet
  adresi de onaylar.

## E-posta onayı

047 göçü `users.email_verified_at` kolonunu ekler; var olan her adres onaysız
başlar ve adres hangi yoldan değişirse değişsin bir tetikleyici onayı düşürür.
Hesabım sayfası durumu ve **Onay bağlantısı gönder** düğmesini gösterir
(`/api/v1/auth/email/verification`, 24 saat geçerli, saatte 3). Kişi kendi
adresini değiştirince yeni adrese onay bağlantısı, eski adrese yeni adresi
içermeyen bir bildirim gider.

## Saklama ve denetim

`account_links` her 256 bitlik jetonun yalnız SHA-256 özetini (belirlenimci
özet satırın bulunmasını sağlar; jetonun rastgeleliği yavaş özeti gereksiz
kılar), kullanıcıyı, amacı, adresi, kimlik dönemini ve zamanları tutar. Hesabın
parolası, askısı ya da iki adımlı durumu değişince veya adresi farklılaşınca
bağlantı geçersizdir. Kullanılmış, geçersiz kılınmış ve süresi dolmuş satırlar
30 gün sonra silinir. Jeton bağlantının parça kısmında taşınır; sunucu
günlüğüne ya da `Referer` başlığına gitmez, adresi yalnızca açan bir posta
tarayıcısı hiçbir şeyi değiştirmez.

Denetim eylemleri: `account.password_reset.requested|mail_failed|completed`,
`account.invitation.sent|mail_failed|revoked|accepted`,
`account.email.verification_sent|verified`. İze adres yazılmaz.

## Sahibin bağımsızlığı

Buradaki hiçbir şey posta sistemi adına çalışmaz ya da onu değiştirmez; panel
durduğunda posta, siteler ve DNS eskisi gibi çalışır. Sunucu sahibinin kendi
yönetici hesabının kurtarılması hiçbir zaman postaya bağlı değildir.
