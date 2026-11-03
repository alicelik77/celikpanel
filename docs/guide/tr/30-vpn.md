---
id: vpn
title: VPN: sunucuya güvenli erişim
roles: [admin, reseller, customer]
topics: [vpn]
keywords: [vpn, wireguard, cihaz, dizüstü, telefon, tünel, qr]
---

**VPN**, bir dizüstü bilgisayara ya da telefona sunucunun yerleşik WireGuard VPN'i üzerinden güvenli bir bağlantı verir.

## Açmak (yönetici) {#server}

1. WireGuard'ı **WireGuard’ı kur** ile ya da **Bileşenler** sayfasından kurun.
2. **VPN sunucusunu başlat** düğmesine basın.
3. Sunucu UDP 51820 portunu kabul etmelidir. Barındırma sağlayıcınızın kendi güvenlik duvarı varsa bu portu orada da açın.

**Sunucu politikası** sekmesi sabit ayarları gösterir; bunlar düzenlenemez.

## Cihaz eklemek {#devices}

Her cihaz, planında VPN bulunan bir aboneliğe bağlıdır.

1. **Cihazlar** sekmesinde **Cihaz ekle** düğmesine basın, aboneliği seçin ve `is-dizustu` gibi bir ad yazın.
2. **Config oluştur** düğmesine basın.
3. QR kodunu WireGuard uygulamasıyla okutun ya da **.conf indir** ile dosyayı indirip uygulamaya aktarın.
4. **Kaydettim — kapat** düğmesine basın.

:::warning
Yapılandırma yalnız bir kez gösterilir ve özel anahtarı saklanmaz. Kaybolursa cihazı kaldırıp yeniden ekleyin.
:::

Bir cihazı kaldırmak erişimini hemen keser.

## Bir şey uyuşmuyorsa {#sync}

Cihaz listesi istenen durumu ve uygulanan durumu ayrı gösterir; böylece başarısız bir değişiklik görünür kalır. Yöneticinin **Güvenle yeniden eşitle** düğmesi istenen durumu WireGuard'a yeniden uygular. VPN planınızda yoksa sayfa bunu söyler; sağlayıcınız ekleyebilir.
