---
id: plans-rights
title: Planlar, haklar ve sunulanlar
roles: [admin, reseller]
topics: [store]
keywords: [plan, haklar, izinler, kota, sınır, sınırsız, havuz, disk, trafik, sunmak, mağaza]
---

Bir hesabın neyi kullanabileceği iç içe dört katmanla belirlenir: sunucuda kurulu olan, yöneticinin sunduğu, bayinin sahip olduğu ve hesabın sahip olduğu. Bunu sunucu uygular; ekranlar hesabın kullanamadığını gizler.

## Haklar {#rights}

- **Sayılar:** müşteri (bayi için), alan adı, veritabanı, posta kutusu, disk ve aylık trafik. `0` **Sınırsız** demektir.
- **İzinler:** örneğin **DNS kayıtlarını düzenleme**, **Komut satırı (SSH)**, **VPN**.
- **Yazılımlar:** PHP ve Node.js sürümleri, veritabanı motorları, web postası, uygulamalar.

Bir hakkı geri almak çalışanı durdurmaz; yalnız o türden yenisi oluşturulamaz.

## Sunulanlar {#offered}

**Kullanıcılar** sayfasında yöneticinin **Sunulanlar** sekmesi her izni ve kurulu her programı **Sunuluyor** ya da **Gizli** olarak gösterir. WordPress gibi uygulamalar **Uygulamalar (uygulama kurucu)** altındadır. Yerleşik **Sınırsız** planlar bu sekmeyi hemen izler.

## Planlar {#plans}

**Servis planları** ve yönetici için **Bayi planları**: **Plan ekle**, **Plan adı** yazın, bölümleri ayarlayın, **Kaydet**. Bir şeyi geri alan düzenleme, önce planı izleyen hesapları gösterir. **Sınırsız** ve **Sınırsız bayi** yerleşiktir, düzenlenemez.

Hesap satırındaki **Haklar**, **Sınırsız**, **Bir plan** ya da **Özel** seçeneklerini sunar. Plana bağlı bir hesapta tek bir maddeyi değiştirmek, sorulduktan sonra hesabı **Özel** yapar. **Bitiş tarihi** gelince abonelik askıya alınır.

## Bayi olarak {#reseller}

- Yalnız daraltabilirsiniz. Kendi haklarınızda olmayanlar soluk görünür; onları yönetici ekleyebilir.
- Müşteriniz için **Sınırsız**, kendi haklarınızın tamamı demektir.
- **Havuz**, planlarda ne dağıtmış olursanız olun, bütün müşterilerinizin gerçekte birlikte kullanabileceği üst sınırdır. **Planım** kartı **Bayi havuzunuz** bölümünü gösterir; %80 ve %100'de bildirim gelir.

## Gerçek kullanım {#usage}

**Planım** her ölçüyü gösterir. Disk; site dosyalarını, postayı ve veritabanlarını sayar. Trafik aylık sayılır ve birkaç dakika geriden gelir.

| Sınıra gelince | Ne olur |
|---|---|
| Bir sayı | O türden yenisi reddedilir. |
| Trafik | Ayın 1'ine ya da sınır yükseltilene kadar yalnız web siteleri durur. Posta ve dosyalar çalışır. |
| Disk | Yeni bir şey oluşturulamaz, yüklemeler durur. 7 gün sonra abonelik askıya alınır; kullanım sınırın altına inince kendiliğinden açılır. |

%80 ve %100 bildirimleri müşteriye, bayisine ve yöneticiye gider.

## Mağaza {#store}

**Mağaza**, tek bir aboneliğe **Erişim ver** ve **Erişimi kaldır** ile ürün hakkı verir; hiçbir şey kurmaz. Yönetici, **Katalog yönetimi** sekmesinde riskleri okuduktan sonra **Yapay zekâ ve otomasyon uygulamaları** listesindekileri sunar.
