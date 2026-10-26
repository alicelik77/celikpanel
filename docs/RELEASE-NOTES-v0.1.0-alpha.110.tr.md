# v0.1.0-alpha.110

[English](RELEASE-NOTES-v0.1.0-alpha.110.md)

> **Yayımlandı.** Bu sürüm 26 Ekim 2026'de (UTC) yayımlandı: sürüm commit'i `3abeb40d1`, GitHub ön sürümü 04:52:06Z, yapıtlar ve imza doğrulanıp dış denetim geçtikten (26/26) sonra indirme portalı yaklaşık 04:58Z. Hiçbir parçası henüz kurulu bir sunucuda değil; kurulu panelleri yalnız sahip günceller. Aşağıdaki aday metni yazıldığı gibi korunur.

Bu sürüm v0.1.0-alpha.109'un ardından gelir. **26 Ekim 2026'da yazılmış bir
adaydır; yayımlanmadı ve hiçbir parçası kurulu bir sunucuda değil. Kurulu panelleri
yalnız sahibi, panelin kendi güncelleme ekranından günceller.** alpha.109'un sürüm
commit'i `c7a05c7dc04cd133528e9aa4b67f21c0132369cb` üzerindeki `main`'den
`feat/alpha110-webmail-origin` dalında yazıldı.

## Kim güncellemeli, neden

Web postası olan her sunucu. alpha.109'da web postası panelin sıkı güvenlik kuralının
arkasında, panelin kendi adresinde kaldı; Roundcube'un arayüzü orada başlayamadı.
Panelin adresinde çalışan bir betik, panelin API'sini oturum açmış yönetici olarak
kullanabilir ve Roundcube'da böyle siteler arası betik açıkları olmuştur
(CVE-2023-5631, CVE-2024-37383, CVE-2024-42009). Web postası artık **kendi
adresinde** açılır, örneğin `https://mail.celikhost.com/`; onu sunucunun kendi web
sunucusu sunar. Sahibin her sunucuda yapacakları
[işlem yönlendirmesinde](OPERATION-GUIDANCE.tr.md#web-postası-kendi-adresinde-26-ekim-2026).

## Web postası kendi adresinde

- **Adı siz seçersiniz.** Bileşenler → Web postası (Roundcube) → Yönet'teki yeni
  **Web postası adresi** kartı herhangi bir adı alır; Sunucu kurulumundaki posta
  sunucusu adını önerir. Sunucu önekleri (`frankfurt.` gibi) gerekmez. Panelin kendi
  adı reddedilir: tarayıcı panelin oturum çerezini aynı adın her portuna gönderir.
- **Önce denetim, hiçbir şey değişmez.** **Denetle**, genel DNS çözümleyicilerine
  adın nereyi gösterdiğini sorar ve bu sunucunun adresleriyle karşılaştırır. Kayıt
  henüz yoksa kart eklenecek kaydın kendisini gösterir; bölge bu panelde ise,
  onayladıktan sonra kaydı sizin için ekleyebilir. Ad başka bir sunucuyu
  gösteriyorsa reddedilir; DNS'in yanıtı ve olması gereken söylenir (o sunucu diğer
  DNS sunucusuysa adıyla). DNS'e sorulamadıysa "bilinmiyor"; yeniden denetleyin.
  Güvenlik duvarında 80 ya da 443 kapalıysa kart hangisi olduğunu ve nerede
  açılacağını söyler; CelikPanel portları kendiliğinden asla açmaz.
- **Tek onaylı işlem kurar.** Sunucu ad için Let's Encrypt'ten sertifika alır (posta
  sertifikasıyla aynı yoldan: web sunucusu üzerinden 80 portunda; işletim sisteminin
  kendi certbot zamanlayıcısı yeniler ve her yenilemeden sonra certbot web
  sunucusunu yeniden yükler), ad için tek bir web sunucusu dosyası yazar, denetler,
  yeniden yükler ve web postasının orada HTTPS ile yanıt verdiğini kanıtlar
  (sertifikası, PHP'si, giriş sayfası ve stil dosyası). Bir şey başarısız olursa web
  sunucusu dosyası eski haline döner; sertifika kalır. İşlem sunucuda koşar; ekran
  onu izler ve asla ikinci kez başlatmaz.
- **Web postası artık panele bağlı değil.** Adresi nginx ve PHP-FPM, Roundcube'un
  kendi kökeni için güvenlik kuralıyla sunar; panel dururken de yanıt verir.
- **Panelin `/webmail/` yolu** artık adrese yönlendirir. Adres yoksa nerede
  ayarlanacağını söyleyen kısa bir sayfadır. Roundcube sayfaları panelin adresinde
  artık hiç sunulmaz.
- **Bağlantılar geri döndü.** Roundcube satırındaki, Yönet sayfasındaki ve alan
  adının posta ekranındaki **Web postasını aç** `https://<adres>/`'i gösterir. Posta
  kutusu kullanıcıları orada e-posta adresi ve parolasıyla girer; panel hesabı
  gerekmez.
- **Adresi değiştirmek ya da kaldırmak.** Ad değişince önce yenisi kurulur, eskisi
  yalnız yenisi çalışınca kaldırılır. Kaldırma onaylanır ve ne olacağını söyler;
  "Sertifikasını da sil" işaretlenmedikçe sertifika kalır. Elle değiştirdiğiniz bir
  web sunucusu dosyası asla üzerine yazılmaz ya da silinmez; kart bunu söyler.

Henüz yapılmadı (sonraki adımlar olarak kayıtlı): birden çok sunucudaki posta
kutuları için tek adres ve müşterilerin kendi `webmail.<alanadı>` adları.

## Güncelleme ve kurtarma

Şema değişikliği yok (şema 46): adres `panel_settings` içinde tek satırdır; işlemler
mevcut hizmet işlemleri tablosunu kullanır (`webmail_address` ve
`webmail_address_remove` türleri). alpha.109'a dönüş olağan kurtarmadır; alpha.109
Paneli etkin bir adresin web sunucusu dosyasını ve sertifikasını olduğu gibi bırakır
(web postası adresinde yanıt vermeyi sürdürür) ve `/webmail/` yolu geri tutulan
vekile döner. Bir adresi elle kaldırmak için: `sudo rm
/etc/nginx/conf.d/celikpanel-webmail-origin-<ad>.conf`, `sudo nginx -t`, `sudo
systemctl reload nginx`.

## Kanıt

- Ajan 4840 geçti, 29 atlandı (temiz bağlama ad alanında root olarak; ayrıca yetkisiz `nobody` kullanıcısı olarak); Panel 3450 geçti, 6 atlandı; her iç paket geçer; web 1947 geçti ve üretim derlemesi bütçeleri içinde; yarış parçaları yerelde koşulmadı (o konukta C derleyicisi yok; CI koşar). Sürüm kurtarma düzeneği değişmedi (şema 46).
- Saklanan temel imajdan atılabilir bir Debian 13 sanal makinesinde, Ajan biriminin
  umask'ı altında, Let's Encrypt yerine atılabilir bir test CA'sıyla tek gerçek
  sistem koşusu: Roundcube dağıtımın PHP-FPM'iyle kuruldu; certbot denetimi nginx
  üzerinden 80 portunda yanıtlandı; adres kuruldu ve kanıtlandı; `http://`
  yönlendirdi; giriş sayfası kendi kuralıyla, güvenli ve katı çerezlerle; stil dosyası
  `text/css`; CelikPanel'in PHP denetimi başka bir adresten reddedildi; panelin
  `/webmail/` yolu oraya yönlendirdi ve yönlendirme sertifika doğrulanarak
  Roundcube'a ulaştı; kaldırma adda web postasını durdurdu ve sertifikayı korudu
  ([kanıt](../deploy/e2e/d038/alpha110-debian13-20261026/README.md), arşiv SHA-256 `cf485342…888497b6`).
- Sahte veride bir tarayıcı bakışı (EN ve TR, 1440 ve 390, açık tema; grup 33): adressiz, kaydı henüz olmayan, hazır ve kurma onayıyla, etkin ve kaldırma onayıyla Web postası adresi kartı; yana kaydırma yok, ham anahtar yok.

Kurulu bir sunucuda ölçülmedi. Aday kabul değildir; P0.1–P0.5 **kısmi** kalır; hiçbir
P0 işi kapanmaz ya da ilerlemez.
