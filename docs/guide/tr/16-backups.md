---
id: backups
title: Yedekler, uzak kopyalar ve zamanlanmış görevler
roles: [admin, reseller, customer]
topics: [domain.backups#backups, domain.advanced#cron-logs]
keywords: [yedek, geri yükleme, uzak yedek, s3, sftp, backblaze, wasabi, gpg, parola cümlesi, cron, zamanlanmış görev, günlük, log]
---

Alan adının yedekleri **Yedekler** bölümündedir. **Zamanlanmış görevler** ve **Günlükler** **Gelişmiş** bölümünde, zararlı yazılım taraması **Güvenlik** bölümündedir (bkz. [Zararlı yazılım taraması](guide:apps-wordpress#malware)).

## Yedek almak {#backups}

**Yeni yedek oluştur** altında şunlardan birini seçin:

- **Dosya yedeği**: yalnız sitenin dosyaları;
- **Veritabanı yedeği**: alan adına bağlı bir veritabanı;
- **Tam yedek**: dosyalar ve bağlı bütün veritabanları.

Sonra **Yedek oluştur** düğmesine basın. **Mevcut yedekler** listesinde her yedeğin yanında **Geri yükle**, **İndir** ve **Sil** bulunur. Yedekler bu sunucunun korumalı bir alanında durur ve disk sınırınızdan düşmez.

**Otomatik yedekler** yedeği kendiliğinden alır: **Sıklık** (**Günlük** ya da **Haftalık**), **Ne yedeklensin** ve **Tutulacak kopya** seçip **Aç** düğmesine basın. Bu sayıyı aşan eski kopyalar silinir.

## Geri yüklemek {#restore}

Bir yedeğin yanındaki **Geri yükle** düğmesine basıp onaylayın. Mevcut dosyaların ve verilerin üzerine yazılır. Bundan önce CelikPanel şu anki durumun bir güvenlik yedeğini alır ve listede gösterir; sonuç istediğiniz gibi değilse onu geri yükleyin. Disk sınırınıza sığmayan geri yükleme, hiçbir şey değiştirilmeden reddedilir.

## Uzak kopyalar {#remote}

Bu sunucudaki yedek, sunucu kaybolursa onunla gider. Uzak hedef, başka yerde sizin denetlediğiniz bir depolamadır: S3 uyumlu depolama (AWS, Backblaze B2, Wasabi, MinIO, Cloudflare R2) ya da bir SFTP sunucusu.

**Hedef eklemek.** Müşteriler ve bayiler bunu, planları izin veriyorsa **Hesabım** sayfasında **Uzak yedek hedefleri** bölümünden yapar; yönetici ise **Ayarlar** sayfasından yapar ve hedefi bütün hesaplara sunabilir.

1. **Hedef ekle** düğmesine basın, bir **Ad** verin ve **Tür** seçin.
2. Depolamanın adresini ve giriş bilgilerini yazın.
3. En az 12 karakterlik bir **Parola cümlesi** belirleyin ya da **Güçlü bir parola cümlesi üret** düğmesine basın. Her kopya sunucudan çıkmadan önce bununla şifrelenir.
4. Parola cümlesini yazdığınızı işaretleyip kaydedin. Panel önce küçük bir dosya yazıp okuyarak ve silerek bağlantıyı dener. SFTP'de sunucunun anahtar parmak izini yalnız o sunucudakiyle aynıysa onaylayın.

:::warning
Parola cümlesini bu sunucudan uzakta, güvenli bir yerde saklayın. O olmadan bu yedekleri kimse açamaz; sunucu kaybolursa CelikPanel onu geri veremez. Sonradan değiştirilemez; yenisi için yeni bir hedef ekleyin.
:::

**Kullanmak.** **Yedekler** bölümündeki **Uzak kopyalar** kısmı bu sitenin kullanabileceği hedefleri gösterir:

- Bir yedeğin yanındaki **Ayrıca gönder…** o yedeği gönderir. Yerel yedek burada kalır.
- **Zamanlama ekle** günlük ya da haftalık yeni bir yedek alıp gönderir ve belirlediğiniz sayıda günlük ve haftalık kopya tutar. Yalnız kendi eski kopyalarını siler.
- **Hedefteki yedekleri göster** orada saklananları listeler; **Geri yükle** birini bu siteye geri getirir.

Bir hedefi kaldırmak ona göndermeyi durdurur; orada saklananlar silinmez.

## Uzak kopyayı panelsiz açmak {#gpg}

Uzak kopya standart bir şifreli dosyadır (`.cpbak.gpg`), yanında düz metin bir `.json` açıklaması bulunur. `gpg` ve `tar` olan herhangi bir bilgisayarda:

```
gpg --output site.cpbak --decrypt yedek.cpbak.gpg
mkdir geri && tar -xzf site.cpbak -C geri
```

`gpg` parola cümlesini sorar. Sitenin dosyaları `geri/payload/files.tar.gz` içinde, her veritabanının dökümü `geri/payload/databases/` klasöründe olur.

## Zamanlanmış görevler ve loglar {#cron-logs}

**Zamanlanmış görevler** bir komutu belirlenen zamanlarda sitenin kendi hesabıyla çalıştırır. **Görev ekle** düğmesine basın, **Zamanlama** alanına bilinen beş alanı (dakika, saat, ayın günü, ay, haftanın günü), **Komut** alanına komutu yazıp kaydedin.

**Loglar** sitenin **Erişim günlüğü**, **Hata günlüğü** ve **PHP hata günlüğü** kayıtlarını süzme ve indirme seçenekleriyle gösterir.
