---
id: panel-settings
title: Panel adresi, lisans, güncellemeler ve bakım
roles: [admin]
topics: [settings, maintenance]
keywords: [ayarlar, port, https, lisans, güncelleme, sürüm yükseltme, geri alma, yeniden başlatma, sqlite]
---

**Ayarlar** sayfasında **Panel adresi ve HTTPS**, **Lisans**, **CelikPanel güncellemeleri** ve **Uzak yedekler** (bkz. [Yedekler](guide:backups)) sekmeleri bulunur. **Bakım** sayfası yeniden başlatmaları ve sistemin SQLite veritabanlarını tutar.

## Adres ve port {#address}

**Panel sertifikası**, **Adresi değiştir**, **Sertifika al** ve **Tekrar kontrol et** düğmelerini sunar. **Panel portu** bölümünde port **Portu değiştir** ile değişir: panel bir süre iki porttan da yanıt verir, siz yeni adresten onaylarsınız; onay gelmezse 10 dakika sonra kendiliğinden eski porta döner.

## Lisans {#license}

**Lisans anahtarı** alanını doldurup **Lisansı etkinleştir** düğmesine basın. Geçerli lisans yoksa panel, sayfaları yerine bir lisans ekranı gösterir; siteler, posta, veritabanları ve zamanlanmış görevler çalışmayı sürdürür. Sunucu celikpanel.net'e ulaşamazsa yönetim 7 gün açık kalır.

## Paneli güncellemek {#update}

Paneli yalnız siz, sunucunun sahibi, **CelikPanel güncellemeleri** sekmesinden ve ana yönetici hesabıyla güncellersiniz.

1. **Güncellemeleri denetle** düğmesine basın. Otomatik denetim yalnız bakar; hiçbir şey kendiliğinden kurulmaz.
2. **Güncellemeden önce** listesini okuyun: panel bir kez yeniden başlar; web siteleri, posta, DNS ve veritabanları çalışmayı sürdürür.
3. Güncellemeyi başlatın. Sayfa her adımı (indirme, imza, salt okunur denetim, kurulum, doğrulama) gösterir ve kendiliğinden yeniden bağlanır.

## Güncelleme tamamlanmazsa {#rollback}

| Gördüğünüz | Ne yapmalı |
|---|---|
| **Güncelleme kurulu sürümü değiştirmeden durdu** | Hiçbir şey değişmedi. İletinin söylediği nedeni giderin; yeniden denemek güvenlidir. |
| **Güncelleme tamamlanmadı; önceki sürüm geri yüklendi** | Sunucu önceki sürüme döndü. Düzeltilmiş bir sürümü bekleyin. |
| Sonuç henüz bilinmiyor | Sayfa sunucunun kaydını okumayı sürdürür. Bu sırada hiçbir şey başlatmayın. |

Geri alma, panelin önceki sürümünü ve panel veritabanının güncellemeden hemen önceki hâlini geri getirir. Güncellemeden sonra panelde yapılanlar kaybolur; onları yeniden gözden geçirin. Siteler, posta, veritabanları ve DNS geri almanın parçası değildir.

## Bakım {#maintenance}

**Yeniden başlatmalar ve yeniden açılış**, güncellemenin değiştirdiği kodu hâlâ çalıştıran servisleri her birinin **Yeniden başlat…** düğmesiyle ve sunucunun yeniden açılmayı bekleyip beklemediğini gösterir. Hiçbir şey kendiliğinden yeniden başlamaz; sunucuyu siz yeniden açarsınız.

**Sistem SQLite veritabanları**, **Bütünlüğü denetle**, **Optimize et** ve **Anlık görüntüyü indir** düğmelerini sunar. Anlık görüntü hesap verisi içerir: onu bir parola gibi saklayın.
