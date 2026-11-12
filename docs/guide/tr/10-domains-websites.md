---
id: domains-websites
title: Alan adları ve web siteleri
roles: [admin, reseller, customer]
topics: [domains#domain-list, domain.overview#overview, domain.site#hosting, domain.security#https]
keywords: [alan adı, domain, web sitesi, alt alan adı, php sürümü, sertifika, https, ssl, lets encrypt, ad sunucusu, statik, node.js]
---

Barındırdığınız her şey bir alan adının altında durur: web sitesi, e-postası, veritabanları ve DNS'i. Bu bölüm alan adı eklemeyi, alan adının sayfasını, site ayarlarını ve sertifikayı anlatır.

## Alan adı listesi {#domain-list}

Menüdeki **Alan Adları** sayfası alan adlarınızı türü, disk kullanımı, trafiği ve durumuyla listeler. Sayfasını açmak için alan adını ya da **Yönet** düğmesini seçin. **Siteyi ziyaret et** siteyi açar; **Sil** alan adını sitesi, e-postası ve veritabanlarıyla birlikte kalıcı olarak kaldırır. Silmeden önce yedek alın (bkz. [Yedekler](guide:backups#backups)).

## Alan adı eklemek {#adding}

1. **Alan adı ekle** düğmesine basın.
2. **Alan adı** kutusuna adı `www` olmadan yazın, örneğin `example.com`. Alt alan adı da aynı yoldan eklenir: `magaza.example.com`.
3. **Bu alan adı ne için?** sorusunda **Web sitesi** seçin; site başka yerde barındırılıyorsa **Yalnız DNS (web sitesi yok)** seçin.
4. Alan adı bu sunucuyu zaten gösteriyorsa **SSL etkinleştir (Let’s Encrypt)** işaretli kalsın.
5. **Alan adı oluştur** düğmesine basın.

Sertifika istediyseniz pencere bir dakikaya kadar onu bekler. Pencereyi açık bırakın; hiçbir şey iki kez gönderilmez. Sertifika alınamazsa site yine oluşturulur ve HTTPS olmadan çalışır; sertifikayı sonra [Sertifika ve HTTPS](#https) bölümündeki gibi isteyin.

## Genel bakış {#overview}

Alan adının sayfasında tek satırda bölümler vardır: **Genel Bakış**, **Site**, **Dosyalar**, **E-posta**, **Veritabanları**, **Alan adı ve DNS**, **Güvenlik**, **Yedekler** ve **Gelişmiş**. Yalnız alan adına ve size uyan bölümleri görürsünüz. Birkaç parçası olan bölüm onları **Bu sayfada** satırında listeler. Planınızda olmayan bölüm kendi yerinde soluk ve kilitli görünür; onu kimin ekleyebileceğini söyler.

Alan adının altındaki satır **Tür**, **PHP sürümü**, **SSL**, **Disk kullanımı** ve bu ayki **Trafik** bilgisini gösterir.

**Genel Bakış**'ta ve **Alan adı ve DNS** bölümünde duran **Bu alan adının bağlantısı** kartı adın bu sunucuyu gösterip göstermediğini söyler. Göstermedikçe burada barındırılan hiçbir şey o ad için cevap vermez. Kart iki yol sunar; değerleri alan adını aldığınız firmada girin:

- **A yolu** DNS'i bu sunucunun ad sunucularına verir. Posta kayıtları ve sertifikalar sizin yerinize yönetilir.
- **B yolu** DNS'i bugünkü sağlayıcınızda bırakır; yalnız gösterilen adres kayıtlarını eklersiniz.

Kayıt firmasındaki bir değişikliğin yayılması birkaç dakikadan birkaç saate kadar sürebilir. Kart 30 dakika boyunca yarım dakikada bir kendiliğinden kontrol eder; **Tekrar kontrol et** hemen kontrol eder. Değişiklik yayılıyor derken kayıtları yeniden değiştirmeyin.

## Site ayarları {#hosting}

**Site** bölümü alan adında neyin, nasıl çalıştığını toplar:

- **Site türü**: **Proje tipi** olarak **PHP**, **Statik**, **Node.js**, **Vekil** ya da **Yönlendirme** seçip **Uygula** düğmesine basın.
- Planınızda varsa **Uygulamalar**, **Yapay zekâ ve otomasyon** ve **Programlar**.
- Aşağıda anlatılan **PHP ayarları** ve planınızda varsa **Yönlendirmeler**.

**Alan adı ve DNS** bölümünün **Adlar** parçası **Belge kök dizini**, **www'ye yönlendir** ve aynı siteyi gösteren ek adlar için **Alan adı takma adları** ayarlarını tutar. **Korumalı klasörler** **Güvenlik** bölümündedir.

### PHP sürümü {#php}

Yeni web sitesi statik başlar. PHP çalıştırmak için **Site** bölümünün **Site türü** parçasında **PHP** seçin, bir sürüm belirleyin ve **Uygula** düğmesine basın. Sürümü sonra değiştirmek için aynı bölümdeki **PHP ayarları** parçasına gidin, **PHP sürümü** listesinden seçin ve **Uygula** düğmesine basın. Listede yalnız sunucuda kurulu sürümler görünür; **PHP-FPM** yoksa yönetici onu **Bileşenler** sayfasından kurar.

## Sertifika ve HTTPS {#https}

**Güvenlik** bölümünü açın; ilk parçası **Sertifika (HTTPS)**:

1. **Ücretsiz sertifika (ACME)** sekmesinde **E-posta adresi** bilgisini kontrol edin; **Otomatik yenilemeyi aç** açık kalsın.
2. **Sertifika al** düğmesine basın.

Sertifika sağlayıcısı alan adına bu sunucuda ulaşabilmelidir; önce **Bu alan adının bağlantısı** kartındaki adımı bitirin. Ücretsiz sertifika kendiliğinden yenilenir. Satın aldığınız bir sertifika için **Özel sertifika** sekmesinde **Sertifika (PEM)** ve **Özel anahtar (PEM)** alanlarını doldurup **Sertifika yükle** düğmesine basın.

**HTTPS zorla** her ziyaretçiyi `https://` adresine gönderir. **HSTS aç** seçeneğini yalnız HTTPS kalıcı olarak çalışıyorsa açın.

**SSL** satırı **Yapılandırıldı — ilgilenmeniz gerekiyor** diyorsa sertifika vardır ama web sunucusu ya da posta sunucusu onu henüz kullanmıyordur. **Güvenlik** bölümünü açıp **Etkinleştirmeyi yeniden dene** düğmesine basın.
