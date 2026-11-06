---
id: monitoring-audit
title: İzleme, denetim günlüğü ve bildirimler
roles: [admin]
topics: [monitoring, audit]
keywords: [işlemci, bellek, disk, yük, grafik, denetim, günlük, geçmiş, kim ne yaptı, bildirim, uyarı, çalışma süresi, hizmetler, e-posta]
---

## İzleme {#monitoring}

**İzleme** önce **Şu anki sorunlar** ve **Uyarılar**ı, sonra **İşlemci**, **Bellek**, **Disk (/)** ve **Yük (1dk)** grafiklerini gösterir: her nokta 30 saniyenin ortalamasıdır ve 30 gün saklanır; **1 saat** ile **30 gün** arasında seçersiniz. Disk kartı disk okuma ve yazma hızını, yük kartı gelen ve giden ağ trafiğini de gösterir. İşlemci ya da bellek %90'a gelince o andaki en yoğun beş işlem grafiğin altında görünür.

**Hizmetler** kurulu her hizmeti listeler: ne zamandan beri çalıştığı, son bir saatteki yeniden başlatmaları, işlemci ve bellek kullanımı. **Web siteleri** her sitenin ana sayfasını 5 dakikada bir bu sunucudan ister; yanıt verip vermediğini ve sertifikasının kaç günü kaldığını gösterir.

**Uyarı ayarları**ndaki süreden (varsayılan 10 dakika) uzun süren sorun uyarı olur. Uyarılar yalnız adresini **Hesabım**'da onaylamış yöneticilere, saatte en çok dört e-postayla gider. Panel ya da sunucu dururken uyarı yoktur: önemli siteler için ayrıca dış bir çalışma süresi denetleyicisi kullanın.

## Denetim günlüğü {#audit}

**Denetim günlüğü** kimin ne yaptığını en yeniden eskiye **Zaman**, **Kullanıcı**, **Eylem**, **Ayrıntı** ve **IP** sütunlarıyla listeler. **Olarak giriş yap** ile yapılan işler iki adı birlikte gösterir. Sayfa son 200 kaydı gösterir; henüz süzgeç ya da arama yoktur. En yenileri ana sayfada **Son etkinlik** altında da görünür.

Kullanılmış bir sunucuda günlük boş görünüyorsa sayfayı yenileyin: liste okunamamış olabilir.

## Bildirimler {#notices}

| Nerede | Ne |
|---|---|
| Ana sayfa, **İlgi istiyor** | Duran bir servis, kapalı ya da kaydedilmemiş güvenlik duvarı, sertifika yenileme sorunları, spam süzgeci olmayan posta. |
| Ana sayfa | Yeni panel sürümü, sistem güncellemeleri, lisans sorunları. |
| **Kullanıcılar**, **Trafik ve disk bildirimleri** | %80 ya da %100'e gelen hesaplar ve bayi havuzları. |
| **İzleme**, **Uyarılar** | Uyarı süresinden uzun süren sorunlar; ana sayfa kaç tanesinin yeni olduğunu gösterir. |
| E-posta | Onaylı yöneticilere izleme uyarıları, kullanım bildirimleri, zararlı yazılım bulguları, müşterilere giden güvenlik acil durumu bildirimleri. |
