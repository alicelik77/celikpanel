---
id: customers
title: Müşteriler ve abonelikler
roles: [admin, reseller]
topics: [users]
keywords: [hesap, müşteri, bayi, davet, askıya alma, engelleme, devir, silme, olarak giriş]
---

**Kullanıcılar** yönettiğiniz hesapları tutar: yönetici için bayiler ve müşteriler, bayi için müşteriler.

## Hesap oluşturmak {#create}

**Hesap ekle** düğmesine basın; **Kullanıcı adı**, **E-posta** ve **Rol** alanlarını doldurun, bir **Parola** belirleyin ya da **Parola belirlemek yerine davet gönder** seçeneğini işaretleyin (bağlantı 7 gün geçerlidir). **Haklar** bölümü **Sınırsız** ile başlar; plan ya da özel hakları orada seçin (bkz. [Planlar ve haklar](guide:plans-rights#plans)). **Oluştur** düğmesine basın.

## Girişi engellemek ya da askıya almak {#suspend}

| Düğme | Ne durur |
|---|---|
| **Girişi engelle** | Yalnız panele giriş. Siteler ve posta çalışmaya devam eder. |
| **Hizmetleri askıya al** | Siteler askıda olduklarını gösterir; posta bekletilir, gönderenler yeniden dener; posta kutusu girişleri, veritabanları, VPN, zamanlanmış görevler ve dosya erişimi durur. |

Askıya alırken abonelik sahibinin göreceği bir neden istenir; bayiyi askıya almak müşterilerini de askıya alır. Hiçbir şey silinmez; **Hizmetleri yeniden aç** önceden çalışanı aynen geri getirir. Bitiş tarihinin gelmesi ya da diskin 7 gün sınırı aşması da askıya alır; disk durumu, kullanım sınırın altına inince kendiliğinden kalkar.

## Aboneliği devretmek {#transfer}

**Aboneliği devret**: **Yeni sahip** seçin, **Devri gözden geçir**, sonra **Devret**. Önceki sahibin erişimi kalmasın diye **Tüm parola ve anahtarları değiştir** açık kalsın. Alan tarafın hakları ve havuzu önce denetlenir.

Yönetici herhangi iki hesap arasında, bayi kendi müşterileri arasında devreder. Müşterinin **Devir iste** ile gönderdiği istekler **Devir istekleri** altında görünür.

## Silmek {#delete}

**Sil** yalnız hesapta canlı bir şey kalmadığında çalışır; yoksa panel nelerin kaldığını söyler. **Silinebilir** sekmesi askıdaki abonelikleri ve hâlâ tuttuklarını listeler. Önce alan adlarını ve veritabanlarını silin.

:::warning
Alan adını silmek geri alınamaz. Önce yedek alın ya da silmek yerine askıya alın.
:::

**Olarak giriş yap**, bir sorunu birlikte görmek için paneli o müşterinin gözünden açar. Denetim günlüğü bunu iki adla birlikte kaydeder.
