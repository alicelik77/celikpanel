---
id: apps-wordpress
title: Uygulamalar, WordPress araçları ve zararlı yazılım taraması
roles: [admin, reseller, customer]
topics: [domain.site.apps#catalogue, domain.security.malware#malware]
keywords: [wordpress, prestashop, nextcloud, drupal, joomla, mediawiki, kurulum, eklenti, güncelleme, deneme sitesi, zararlı yazılım, virüs, clamav, karantina]
---

Bir PHP sitesinin **Site** bölümündeki **Uygulamalar** parçası hazır bir uygulama kurar ve WordPress için araçlar sunar. Zararlı yazılım taraması **Güvenlik** bölümündedir.

## Uygulama kurmak {#catalogue}

Katalogda WordPress, PrestaShop, Nextcloud, Drupal, Joomla ve MediaWiki bulunabilir; hangilerinin sunulacağına yönetici karar verir. Her biri resmî sürümdür ve açılmadan önce doğrulanır.

Bir sitede tek uygulama olur ve yalnız boş siteye kurulur; böylece hiçbir dosyanızın üzerine yazılmaz. Sayfa her uygulamanın altında sitenin hazır olup olmadığını ya da neyin eksik olduğunu yazar:

- **PHP … gerekir**: sitenin PHP sürümünü aynı **Site** bölümündeki **PHP ayarları** parçasından değiştirin.
- PHP uzantıları eksik: adı yazılan paketleri yönetici kurar.
- Sitenin genel klasöründe zaten dosyalar var: onları taşıyın ya da silin veya `magaza.example.com` gibi yeni bir alt alan adı kullanın.
- Bu sitede kullanılamaz: sağlayıcınızdan uygulamayı sunmasını isteyin.

**Kur** düğmesine basıp onaylayın. Panel veritabanını oluşturur ve uygulamanın kendi kurulumunu çalıştırır.

## Bir kez gösterilen yönetici parolası {#password-once}

Kurulum bitince sayfa uygulamanın **Yönetici** ve **Parola** bilgisini gösterir. Parolayı hemen bir yere kaydedin, sonra **Kaydettim** düğmesine basın: CelikPanel ondan sonra parolayı saklamaz. **Yönetimi aç** uygulamanın giriş sayfasını açar. Parolayı görmeden sayfa kapandıysa **Parolayı göster (bir kez)** onu bir kez daha gösterir.

WordPress farklıdır: kendi kurulum sayfasını açar; sitenin adını ve yöneticinizi orada belirlersiniz.

## Kurulum bitmezse {#setup-failed}

- **Kurulumu yeniden dene**: dosyalar yerinde ama uygulamanın kurulumu yarıda kalmış. Kaldığı yerden devam eder.
- Sonuç bilinmiyor (bir yanıt kayboldu): temizlemek için **Kaldır** düğmesine basın, sonra yeniden kurun.
- Kurulum başarısız oldu: sitede hiçbir şey kalmadı; nedeni okuyup yeniden deneyin.

## Uygulamayı kaldırmak {#removing}

**Kaldır** sizden alan adını yazmanızı ister. Uygulamanın dosyaları gider ve site yer tutucu sayfasını gösterir. **veritabanını da sil** seçeneğini işaretlemezseniz veritabanı kalır; Nextcloud'un veri klasörü de öyle.

## WordPress araçları {#wordpress}

**WordPress araçları** WordPress sürümünü, eklentileri ve temaları, wordpress.org'daki güncellemeleriyle gösterir.

- **… sürümüne güncelle** WordPress'i, **Seçili eklentileri güncelle** işaretli eklentileri günceller.
- Her güncellemeden önce yedek alınır. Güncelleme başarısız olursa site eski hâline getirilir.
- Sonuç bilinmiyorsa siteyi açın. Çalışmıyorsa iletide adı geçen yedeği **Yedekler** bölümünden geri yükleyin.
- Temaları yedek aldıktan sonra WordPress'in kendi yönetim ekranından güncelleyin. wordpress.org temalar için sağlama değeri yayımlamadığından CelikPanel temaları kurmaz.
- **WordPress’in güvenlik ve bakım sürümlerini kendiliğinden kur** günde bir kez bakar. Büyük sürümler her zaman sizin seçiminizdir.

**Başka bir siteye kopyala (deneme sitesi)**, dosyaları ve veritabanını aynı aboneliğin boş bir PHP sitesine kopyalar ve içlerindeki adresi değiştirir. **Hedef site** seçip **Kopyala** düğmesine basın. Kopya sınırlarınızdan düşer; kopyayı canlı siteye geri aktarma yoktur.

## Zararlı yazılım taraması {#malware}

**Güvenlik** bölümündeki **Zararlı yazılım taraması** sitenin dosyalarını ClamAV ile tarar. **Bu siteyi tara** ya da **Aboneliğin tüm sitelerini tara** düğmesine basın. Sunucuda bir seferde tek tarama, düşük öncelikle çalışır; sayfa ilerlemeyi gösterir.

Hiçbir şey kendiliğinden silinmez. Her virüslü dosya **Karantinaya al** ile taşınabilir, **Geri koy** ile geri getirilebilir ya da **Kalıcı olarak sil** ile silinebilir. Bir şey bulunduysa sitenin parolalarını değiştirin ve yazılımını güncelleyin.

ClamAV kurulu değilse ya da henüz imzası yoksa tarama çalışamaz; imzalar eskiyse çalışır ama sonuç daha yeni tehditlerin kaçabileceğini söyler. İki durumda da yönetici **Bileşenler** sayfasında gerekeni yapar, siz taramayı yeniden başlatırsınız.
