---
id: dns
title: Alan adınızın DNS'i
roles: [admin, reseller, customer]
topics: [domain.dns#records]
keywords: [dns, bölge, zone, kayıt, a kaydı, mx, txt, cname, ad sunucusu, kayıt firması, dnssec, ds]
---

DNS, web sitenizin ve e-postanızın nerede olduğunu internete söyler. Alan adının **DNS** sekmesi bölgeyi, yani ad için cevap veren kayıtların listesini gösterir.

## DNS'iniz nereden sunuluyor {#where}

**DNS** sekmesi üç durumdan hangisinin geçerli olduğunu söyler:

- **Bu sunucudan sunuluyor.** Kayıtlar listelenir ve değiştirebilirsiniz. Değişiklikler sunucunun DNS hizmetine, varsa ikinci sunucusuna da yayımlanır.
- **DNS sağlayıcınızda yönetiliyor.** Alan adı başka bir firmanın ad sunucularını kullanır. Sekme oraya eklenecek değerleri listeler. Bu liste gerekenleri gösterir, yayında olduklarını kanıtlamaz; genel kayıtları **Genel Bakış** sekmesindeki **Bu alan adının bağlantısı** kartı kontrol eder.
- **Yetkilendirilmiş bir CelikPanel DNS bağlantısıyla yönetiliyor.** Bölgeyi başka bir CelikPanel sunucusu tutar ve değişikliklerinizi o yayımlar.

## Kayıt eklemek ve silmek {#records}

1. **Kayıt ekle** düğmesine basın.
2. **Tür** seçin: `A`, `AAAA`, `CNAME`, `MX`, `TXT`, `NS` ya da `SRV`.
3. **Ad** (alan adının kendisi için `@`, www.example.com için `www`), **Değer** ve **TTL** yazın. `MX` ve `SRV` için **Öncelik** de girilir.
4. **Kaydı kaydet** düğmesine basın.

Bir kaydı değiştirmek için çöp kutusu simgesiyle silip yeniden ekleyin. **Kayıtları yenile** bölgeyi yeniden okur.

Bazı kayıtlar burada değiştirilemez. **Sunucu → DNS ve eş sunucular** sayfasından yönetildiği yazan kayıt, sunucunun kendi kurulumuna aittir. Planınızda DNS düzenleme yoksa kayıtlar görünür ama değiştirilemez; sağlayıcınız bunu ekleyebilir.

:::warning
Yanlış bir `A`, `MX` ya da `NS` kaydı web sitenizi veya e-postanızı TTL süresi boyunca erişilemez yapabilir. Bir kaydı silmeden önce eski değerini not alın.
:::

## Alan adını bu sunucuya yöneltmek {#pointing}

Alan adı, onu aldığınız firmada (kayıt firması) ayar yapılmadan bu sunucuya ulaşmaz. **Genel Bakış** sekmesindeki **Bu alan adının bağlantısı** kartı tam değerleri verir: A yolu için ad sunucularını, B yolu için adres kayıtlarını. Bkz. [Genel bakış](guide:domains-websites#overview).

## DNS sunucularının sahibi ne yapar {#dns-owner}

Sunucunun yöneticisi DNS yazılımını (BIND ya da PowerDNS) çalıştırır, ad sunucusu adlarını seçer ve ikinci sunucuyu **Sunucu → DNS ve eş sunucular** sayfasından bağlar. İkinci sunucu bölgenizi standart bölge aktarımıyla kopyalar; sizin CelikPanel'de bir şey yapmanız gerekmez.

| Sekmede yazan | Kim yapar | Ne yapılır |
|---|---|---|
| DNS bölgesi aktif değil | Siz ya da yönetici | **DNS bölgesini etkinleştir** düğmesine basın. |
| Kayıtlar yayımlanamadı | Yönetici | Yanında yazan DNS sunucusu sorununu giderir, sonra **DNS’i yeniden yayımla** düğmesine basar. |
| Paylaşılan ad sunucusu kurulumu henüz hazır değil | Yönetici | Ad sunucularını tamamlar; o zamana dek B yolunu kullanın. |

Sayfayı yenilemeniz gerekmez: kayıtlar yayımlanınca sekmede görünür.

## Bölgeyi imzalamak (DNSSEC) {#dnssec}

**Bölgeyi imzala** DNSSEC imzalarını ekler. Ardından sekme kayıt firmanıza girilecek DS değerlerini gösterir. Bunları en son, iki ad sunucusu da imzalı bölgeyi sunduktan sonra girin: erken ya da yanlış girilen DS kaydı, DNSSEC doğrulayan ziyaretçiler için alan adını erişilemez yapar. Seçenek görünmüyorsa planınızda yoktur.
