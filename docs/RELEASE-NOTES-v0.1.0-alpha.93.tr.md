# v0.1.0-alpha.93 (aday, yayımlanmadı)

[English](RELEASE-NOTES-v0.1.0-alpha.93.md)

Bu sürüm v0.1.0-alpha.92'nin ardından gelir. Numarayı, sahibin 2026-10-13
tarihli kalıcı talimatıyla sürümü hazırlayan seçti. 2026-10-16'da (UTC),
`fix/alpha93-small-truths` dalında, `main` üzerindeki alpha.92 sürüm kaydından
(`961f2cb2fcaeb48c08fc58828d15fe0b231e1e1a`) yazıldı.

## Ne değişti

Sahibin alpha.92 çalıştıran iki kurulu sunucudan aldığı ekran görüntülerinden
(2026-10-16) altı küçük düzeltme; her birine sahip aynı gün karar verdi:

1. **Posta doğrulanmışken Temel Posta artık Onar istemez.** Bir profil, onun
   bütün bileşenlerini içeren doğrulanmış daha büyük bir profil (Webmail ya da
   Spam Korumalı Posta) aynı taramada aynı uzlaştırmayı kanıtlıyorsa doğrulanmış
   sayılır. Yalnız bileşen durumu yine kanıt değildir; süren ya da başarısız bir
   Temel Posta denemesi kendi durumunu korur.
2. **"Web postayı onar" olağan bir düğmedir**, Yönet gibi çizilir. Yalnız sunucu
   bir webmail sorunu bildirdiğinde sarıdır.
3. **Pano doğrulanmış postayı düz sözle söyler:** "Posta bu sunucuda kurulu ve
   doğrulanmış."
4. **Posta bileşeni yoksa "Kısmen kurulu" da yok.** Yalnız Nginx (ya da PHP-FPM)
   varken Posta paketleri kartı nötr tonda "Kurulu değil" der; Bileşenler'deki
   Webmail kartı "Bazı bileşenler zaten kurulu…" notunu soluk tonda tutar. Tek
   bir posta bileşeni (Postfix, Dovecot, Rspamd, Roundcube) profili yine kısmen
   kurulu yapar.
5. **Son etkinlik posta profillerini adıyla anar:** "kurulum sırasında
   protected-mail bileşenini hazırladı" yerine "kurulum sırasında Spam Korumalı
   Posta profilini hazırladı".
6. **Sayılar doğru söylenir:** "1 öğe", "3 öğe" (İngilizcede "1 item",
   "3 items").

## alpha.93'e güncelledikten sonra ne göreceksiniz

- Frankfurt: Temel Posta kartı sakindir, sarı Onar cümlesi yoktur; Panodaki
  Posta paketleri kartı "Posta bu sunucuda kurulu ve doğrulanmış." der.
- Boston: Posta paketleri kartı nötr tonda "Kurulu değil" der.
- İkisinde de: Roundcube'un yanındaki "Web postayı onar" sade bir düğmedir; Son
  etkinlik profil adlarını gösterir; Alan adları "1 öğe" der.

## Sınırlar

- Sahibin 2026-10-16 talimatıyla (hızlı sürümler) gerçek sistem kaydı yok.
  Yalnız Go, bileşen ve bağlanmış bileşen testleri; kurulu bir sunucuda hiçbir
  şey ölçülmedi.
- Şema 44, geçiş yok; kurtarma davranışı değişmedi. API değişikliği yalnız bir
  kural değişikliğidir: bir posta profili artık kendi son denemesi `none`
  iken `verified` olabilir.
- P0.1-P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez; yayım kabul
  değildir.

## Yayım kaydı

Yayımlanmadı.

Bu sürümü yalnız CelikPanel'in güncelleme arayüzünden kurun. Yayımlamak kurulu
sunucuları güncellemez; bir sunucunun güncellemesini yalnız o sunucunun sahibi,
panelin kendi güncelleme ekranından başlatır.
