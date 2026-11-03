---
id: monitoring-audit
title: İzleme, denetim günlüğü ve bildirimler
roles: [admin]
topics: [monitoring, audit]
keywords: [işlemci, bellek, disk, yük, grafik, denetim, günlük, geçmiş, kim ne yaptı, bildirim, uyarı]
---

## İzleme {#monitoring}

**İzleme**, 30 saniyede bir örneklenip 48 saat saklanan **İşlemci**, **Bellek**, **Disk (/)** ve **Yük (1dk)** grafiklerini çizer; **1sa** ile **48sa** arasında seçim yaparsınız. Yeni bir sunucuda ilk örnekler birkaç dakika sürer. Servislerin durumu ana sayfada **Sistem servisleri** altındadır. Panel sunucunun sağlığıyla ilgili e-posta uyarısı göndermez.

## Denetim günlüğü {#audit}

**Denetim günlüğü** kimin ne yaptığını en yeniden eskiye **Zaman**, **Kullanıcı**, **Eylem**, **Ayrıntı** ve **IP** sütunlarıyla listeler. **Olarak giriş yap** ile yapılan işler iki adı birlikte gösterir. Sayfa son 200 kaydı gösterir; henüz süzgeç ya da arama yoktur. En yenileri ana sayfada **Son etkinlik** altında da görünür.

Kullanılmış bir sunucuda günlük boş görünüyorsa sayfayı yenileyin: liste okunamamış olabilir.

## Bildirimler {#notices}

| Nerede | Ne |
|---|---|
| Ana sayfa, **İlgi istiyor** | Duran bir servis, kapalı ya da kaydedilmemiş güvenlik duvarı, sertifika yenileme sorunları, spam süzgeci olmayan posta. |
| Ana sayfa | Yeni panel sürümü, sistem güncellemeleri, lisans sorunları. |
| **Kullanıcılar**, **Trafik ve disk bildirimleri** | %80 ya da %100'e gelen hesaplar ve bayi havuzları. |
| E-posta | Kullanım bildirimleri, zararlı yazılım bulguları, müşterilere giden güvenlik acil durumu bildirimleri. |
