---
id: components
title: Bileşenler: yazılım kurmak ve sunmak
roles: [admin]
topics: [components]
keywords: [kurulum, kaldırma, php, node.js, sürümler, hazır paket, depo, nginx, mariadb, postgresql]
---

**Bileşenler**, sunucudaki servisleri, PHP ve Node.js gibi çalışma ortamlarını ve phpMyAdmin gibi araçları listeler. Yazılımlar işletim sisteminin kendi paketlerinden gelir.

## Tek liste {#views}

Sunucunun sunduğu her bileşen kendi bölümünde listelenir; kurulu olanlar önce gelir. Kurulu satır durumunu **Başlat**, **Durdur**, **Yeniden başlat**, **Onar ve başlat** ve **Yönet** düğmeleriyle gösterir. **Kurulu değil** yazan satırda **Kur** düğmesi vardır ya da satır neden burada kurulamadığını tek satırda söyler; örneğin Apache'nin yanında **Nginx ile çakışır** yazar. Postfix ve Dovecot gibi posta motorları, e-posta bölümünün başındaki posta kurulumuyla kurulur: satırlarında **Posta kurulumuyla kur** yazar. DNS motorları DNS sayfasında seçilir.

İlk ziyarette her bölüm açıktır. **Tümünü kapat**, **Tümünü aç** ya da bölümün kendi başlığı bölümleri kapatır ve açar; bu tarayıcı neyi katladığınızı hatırlar. Arama her bileşeni bulur. **Yeniden tara**, örneğin bir şeyi kendiniz kurduktan sonra, sunucuyu baştan okur.

Başka bir sayfa Fail2ban, ClamAV, vsftpd, Python, Node.js ya da WireGuard gibi bir bileşenin eksik olduğunu söylediğinde, oradaki **Kur** bağlantısı o bileşenin kurulum penceresini burada açar. Siz onaylamadan hiçbir şey başlamaz.

## Kurmak ve kaldırmak {#install}

**Kur**, hiçbir şey değişmeden önce neyin kurulacağını gösterir. **Kaldır**, **Silinenler**, **Bu sunucuda etkilenecekler** ve **Yerinde kalanlar** bölümlerini gösterir; bileşene hâlâ bağlı bir şey varsa reddedilir. Bir veritabanı motorunu kaldırmak verisini silmez.

**Hazır paketler**, **Temel Posta** ya da **Nginx + PHP-FPM** gibi birkaç bileşeni art arda kurar. Henüz hiçbir parçası kurulu olmayan paket **Kur** der; parçalarından bazıları kurulu olan paket neyi ekleyeceğini adıyla söyler, örneğin **Ekle: phpPgAdmin**. Bir adım başarısız olursa **Devam et** yalnız eksik kalanları kurar; hiçbir şey kaldırılmaz.

## Sürümler {#versions}

PHP ve Node.js'in birkaç sürümü yan yana durur; her site kendi sürümünü seçer. Satırdaki **Sürümler** ile **Sürüm kur** ya da **Sürümü kaldır**. phpMyAdmin gibi araçlar bir sürümde çalışıyorsa panel önce onları taşımayı önerir.

Sistemin sunmadığı PHP ya da PostgreSQL sürümleri için üreticinin deposu, yalnız o bileşene sınırlı olarak **Sınırlama ve güncellemeleriyle aç** ile eklenebilir. Sonradan kapatmak hiçbir şeyi kaldırmaz.

## Yazılım sunmak {#offering}

Kurmak, yazılımı kimseye vermez. Müşteri bir sürümü ya da programı ancak siz sunduğunuzda ve planı içerdiğinde kullanır:

- PHP ve Node.js sayfalarındaki **Müşterilere sunulan**: her sürümü ayrı işaretleyin. Birini gizlemek çalışan hiçbir siteyi değiştirmez.
- **Kullanıcılar** sayfasının **Sunulanlar** sekmesi: bütün izinler, programlar ve uygulamalar. Bkz. [Planlar ve haklar](guide:plans-rights#offered).

Ayrı bir geliştirici araç kutusu henüz yok; geliştiriciler PHP ve Node.js sürümlerini, planları içeriyorsa SSH'yi kullanır.

## Bir şey ters gittiğinde {#problems}

Duran bir servis kendi satırında ve ana sayfada **İlgi istiyor** altında görünür. **Onar ve başlat** onu onarıp başlatmayı dener. Başarısız bir kurulum ya da değişiklik neyin denendiğini ve şimdi neyin yerinde olduğunu söyler.
