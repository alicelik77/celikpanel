# v0.1.0-alpha.112

[English](RELEASE-NOTES-v0.1.0-alpha.112.md)

> **Yayımlandı.** Bu sürüm 29 Ekim 2026'da (UTC) yayımlandı: sürüm commit'i `95c23ef41`, GitHub ön sürümü 17:22:09Z, indirme portalı dosyalar ve imza doğrulanıp genel kontrol geçtikten sonra (26/26) yaklaşık 17:28Z. Henüz hiçbir kurulu sunucuda yok; kurulu panelleri yalnız sahip günceller. Aşağıdaki aday metni yazıldığı gibi korunur.

Bu sürüm v0.1.0-alpha.111'in ardından gelir ve yalnız ilk kurulumu düzeltir.
**2026-10-29'da yazılmış bir adaydır; yayımlanmadı ve hiçbir kurulu sunucuda
yok. Kurulu panelleri yalnız sahip, panelin kendi güncelleme ekranından
günceller.** `main` üzerindeki `cb225a922`'den `fix/fresh-install-kit-modes`
dalı; önceki sürüm commit'i `34786df487ee5d4bc3c946ed947952086ead26c4`.

## Kimin için, neden

Yeni sunucu kuran herkes için. alpha.111'de herkese açık kurulum komutu
(`curl … https://celikpanel.net/get.sh && sh celikpanel-install.sh`)
"systemd servisleri" adımında şu hatayla durdu:

```
recovery runtime unavailable: runtime_unsafe_metadata
ERROR / HATA: fresh install independent recovery runtime could not be enrolled
```

Kurulu paneller etkilenmez: panelin kendi ekranından yapılan güncelleme dosya
izinlerini zaten geri yükler, onlar için bir şey değişmez. Komutun yarıda
kaldığı sunucu sıfırlanır ve bu sürümle yeniden kurulur.

## Ne değişti

- `get.sh` dosyaları `umask 077` ile indirip açar; sürüm dosyaları 0700/0600
  izinle gelir. alpha.81'den beri ilk kurulum bağımsız kurtarma, güvenlik
  duvarı ve posta sertifikası yenileme kitlerini kaydeder; bunlar yayımlanan
  0755/0644 izinleri ister, başkasını reddeder. İlk kurulum dalı artık bütün
  sağlama toplamı ve imza kontrollerinden sonra, yalnız root'a ait ve bağlantı
  içermeyen hazırlık ağacında arşivin izinlerini geri yükler
  (`chmod -R go+rX`); sürüm kökü 0700 kalır. Güncelleme dalı aynısını
  `bootstrap-prebuilt-update.sh` içinde zaten yapıyordu. Açma işleminin
  kendisi değişmedi, çünkü güncelleme dalı ürettiği 0700 sürüm kökünü ister.
- `deploy/test-fresh-install-entry-boundary-contract.sh` gerçek `get.sh`'yi
  imzalı bir test portalına karşı çalıştırır; artık kurucunun aldığı izinleri
  de denetler (sürüm kökü 0700, dizinler 0755, çalıştırılabilirler 0755, diğer
  dosyalar 0644).

## Önceki testler neden kaçırdı

alpha.111'in dört laboratuvar koşusu `install.sh`'yi `get.sh` üzerinden değil,
açılmış sürümden doğrudan başlattı. Eylülde kurulan sunucular alpha.81'den önce
kurulup sonra panelden güncellenmişti. Yukarıdaki test artık sahibin yazdığı
komutu kapsar.

## Kanıt

- Gerçek alpha.111 arşivi `get.sh`'nin yaptığı gibi açıldığında kurtarma kiti
  `runtime_unsafe_metadata` ile reddedildi (sahibin gördüğü hata); değişiklikten
  sonra kurucunun kullandığı aynı doğrulayıcı kiti kabul etti.
- CI giriş sınırı sözleşmesini temiz bir ortamda çalıştırır.

Sıra 112, şema 46, başka kod değişikliği yok. Açık: herkese açık komutla tam
laboratuvar kurulumu yapılmadı; ölçüm sahibin sunucularındaki ilk kurulumdur.
Hiçbir P0 işi kapanmadı veya ilerlemedi.
