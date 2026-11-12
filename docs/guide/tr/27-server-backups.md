---
id: server-backups
title: Sunucuda ve uzak hedefte yedekler
roles: [admin]
topics: []
keywords: [yedek, geri yükleme, uzak, s3, sftp, parola cümlesi, gpg, zamanlama, saklama, hedef]
---

Yedekler site başına alınır. Sunucunun kendi diskindeki kopya, sunucu kaybolursa onunla birlikte gider; bu yüzden kopyaları başka bir yere de gönderin.

## Sunucuda {#local}

Her alan adının sayfasında bir **Yedekler** bölümü vardır: dosyalar, veritabanları ya da ikisi, elle ya da zamanlanmış. Geri yükleme önce bir güvenlik yedeği alır. Panelde sunucunun tamamının tek bir yedeği yoktur; her siteyi uzak hedefe giden bir tüm siteler zamanlamasıyla koruyun, sistemin kendisi için sağlayıcınızın anlık görüntülerini kullanın.

## Uzak hedefler {#destination}

**Ayarlar** sayfasının **Uzak yedekler** sekmesinde **Hedef ekle** düğmesine basın: HTTPS üzerinden **S3 uyumlu depolama** ya da parmak izini onayladığınız bir **SFTP sunucusu**. Bir parola cümlesi seçin ya da **Güçlü bir parola cümlesi üret** düğmesini kullanın. **Dene ve kaydet**, kaydetmeden önce bir deneme dosyası yazar, okur ve siler. **Hesaplara sun**, hedefi bayilerin ve müşterilerin kullanımına açar.

:::warning
Parola cümlesini bir yere yazın ve bu sunucudan uzakta saklayın. O olmadan yedekleri kimse açamaz; sonradan değiştirilemez ve bir daha gösterilmez.
:::

## Zamanlamalar ve geri yükleme {#schedules}

**Tüm siteler için zamanlamalar** her siteyi günlük ya da haftalık gönderir ve belirlediğiniz sayıda kopya tutar. Bir sitenin **Yedekler** bölümünde **Zamanlama ekle** ve **Ayrıca gönder…** yalnız o site için çalışır; **Hedefteki yedekler** o sitenin kopyalarını **Geri yükle** düğmesiyle listeler. Geri yükleme bir güvenlik yedeğinden sonra aynı siteye yapılır.

Bir zamanlama yalnız kendi eski kopyalarını siler. **CelikPanel’den kaldır**, hedefteki hiçbir şeyi silmez. Başarısız bir gönderim en çok üç kez yeniden denenir; sonucu bilinmeyen bir gönderim yeniden okunur, asla iki kez gönderilmez.

## CelikPanel olmadan {#without-panel}

Uzak kopya standart bir OpenPGP dosyasıdır. Herhangi bir bilgisayarda `gpg`, parola cümleniz ve `tar` ile açılır:

```
gpg --output site.cpbak --decrypt ornek.cpbak.gpg
mkdir restore && tar -xzf site.cpbak -C restore
```

İçinden sitenin dosyaları ve her veritabanı için bir döküm çıkar.
