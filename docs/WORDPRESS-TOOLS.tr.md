# WordPress araçları

*Özellik notu · [English](WORDPRESS-TOOLS.md)*

*alpha.118 (sahibin isteği: "WordPress araçları"). Kaynak değişikliği; yalnız odaklı birim
testleriyle denendi, henüz gerçek sunucuda denenmedi. Sürümün deneme turu açık.*

Panelin WordPress kurduğu ya da web klasöründe WordPress bulduğu her PHP sitesinde,
Uygulamalar sekmesinde uygulama kataloğundan ayrı bir **WordPress araçları** bölümü vardır.

## Ne yapar

- **Durum:** WordPress sürümü, eklentiler ve temalar, sürümleri ve wordpress.org'un güncelleme
  sunup sunmadığı. Sürümler, panelin getirdiği küçük bir PHP betiğiyle **sitenin kendi
  hesabıyla ve kendi PHP'siyle** okunur; betik yalnız sürüm başlıklarını okur, WordPress,
  eklenti ya da tema kodu çalıştırmaz. Güncellemeler yalnız **wordpress.org'un resmî
  API'sine** (`api.wordpress.org`) sorulur; başka hiçbir yere bağlanılmaz.
- **Önce yedek, sonra güncelleme:** her güncellemeden önce panel, var olan yedekleme motoruyla
  sitenin yedeğini alır (kaynak "WordPress güncellemesi öncesi"; dosyalar, çekirdek
  güncellemesinde sitenin veritabanları da). Sonra:
  - **Çekirdek:** sürüm wordpress.org'dan indirilir ve kullanılmadan önce yayımlanan SHA-1
    değeriyle doğrulanır; dosyalar sitenin kendi hesabıyla kopyalanır (`wp-content` ve
    `wp-config.php` asla); WordPress'in kendi `wp_upgrade()` adımı, eklenti ve temalar
    yüklenmeden, site hesabıyla çalışır.
  - **Eklentiler:** her paket indirilir ve site değiştirilmeden önce wordpress.org'un o
    eklenti sürümü için yayımladığı SHA-256 listesine karşı **dosya dosya** doğrulanır. Eski
    eklenti klasörü kenara konur, bir şey ters giderse geri konur.
  - **Temalar:** wordpress.org tema paketleri için sağlama değeri yayımlamıyor; bu yüzden
    CelikPanel tema güncellemesi kurmaz (güvenlik özgürlükten önce). Ekran yeni sürümü
    gösterir ve yedek aldıktan sonra WordPress'in kendi Başlangıç > Güncellemeler ekranından
    güncellemeyi söyler.
  - Değişiklik sırasında WordPress'in kendi bakım sayfası görünür; sitenin ana sayfası önce
    ve sonra yerelden sorulur. Önce yanıt verip sonra hata veriyorsa güncelleme başarısız
    sayılır.
- **Geri alma:** güncelleme dosyaları değiştirip başarılı olmadıysa panel önce aldığı yedeği
  kendiliğinden geri yükler. Ajan eski eklenti klasörlerini kendisi geri koyduysa geri
  yüklemeye gerek kalmaz. Güncellemenin yanıtı yitirildiyse ya da panel bu sırada yeniden
  başladıysa sonuç **bilinmez**: hiçbir şey tahmin edilmez ya da tekrarlanmaz; ekran, site
  çalışmıyorsa Gelişmiş > Yedekler'den geri yüklenecek yedeği adıyla söyler.
- **Küçük ve güvenlik sürümleri kendiliğinden (site başına anahtar):** panel günde bir kez
  kurulu dalın yeni sürümünü arar (6.6.1 → 6.6.2; asla 6.6 → 6.7) ve aynı yolla, önce yedek
  alarak kurar. Büyük sürümler her zaman kişinin seçimidir. WordPress'in kendi arka plan
  güncelleyicisine panel dokunmaz (sahibin bağımsızlığı): sahip onu `wp-config.php` içinde
  kapatmadıysa WordPress küçük sürümleri kendisi de kurar.
- **Deneme sitesi kopyası:** siteyi aynı aboneliğin boş başka bir PHP sitesine kopyalar:
  dosyalar (yedekleme motorunun güvenli arşivi), yeni bir veritabanı (veritabanı sınırından
  düşer; önce disk sınırı denetlenir) ve adres. Adres veritabanı dökümünde
  **serileştirmeye dayanıklı** biçimde değiştirilir: PHP serileştirilmiş değerlerin uzunlukları
  yeniden yazılır, iç içe değerler dolaşılır, JSON-kaçışlı ve URL-kodlu biçimler de kapsanır,
  `www.` da eşlenir; çözülemeyen değer bozulmaz, olduğu gibi kalır (sayılır). Kopyanın
  `wp-config.php` dosyasında yalnız dört veritabanı satırı ve adres değişir. Başarısız kopya,
  yalnız kendi oluşturduğu veritabanını (sahiplik belirteciyle) siler.

## Karar: WP-CLI yok

WP-CLI, Arch Linux'un resmî `extra` deposunda var; ama Debian 12/13 ve Ubuntu 22.04/24.04'te
**yok** (packages.debian.org ve packages.ubuntu.com, 2026-11-03'te bakıldı). CelikPanel
desteklenen her sistemde yalnız resmî dağıtım paketleri kurar; bu yüzden WP-CLI kullanılmaz.
Gereken işler sitenin PHP'siyle çalışan iki küçük betikle
(`/var/lib/celikpanel-wordpress-tools/inventory.php`, `upgrade.php`), WordPress'in kendi
`wp_upgrade()` adımıyla, site hesabıyla çalışan standart dosya araçlarıyla ve var olan yedekleme
motoruyla yapılır.

## Haklar

`wordpress_tools` izni (D-042, [PLANS-AND-RIGHTS.tr.md](PLANS-AND-RIGHTS.tr.md) bölüm 4.2).
Sunucu her istekte uygular; izin yoksa ekran bölümü gizler. Yönetici her zaman kullanabilir.
Sınırsız planlar ve "tüm izinler" içeren kümeler onu içerir; Özel küme ancak biri eklerse
(sessiz değişiklik yok). Ek kullanıcılar (ekip üyeleri) bu araçları almaz. Uygulama kurucunun
kendi izni sürer.

## Ekrandaki yönlendirme

Her durum nedeni, kimin yapacağını, sonraki adımı ve nasıl devam edileceğini söyler:
sunucunun tek işlem yerini bekleme, yedek alma, güncelleme, geri yükleme, tamam; doğrulanmış
hata ("hiçbir şey değişmedi"); geri yüklendi ("site eski hâline getirildi"); bilinmiyor
("siteyi kontrol edin; X yedeğini geri yükleyin"); önkoşullar (PHP komut satırı yok →
yönetici; sitenin veritabanı alan adına bağlı değil → yedek onu içermeyeceği için çekirdek
güncellemesi reddedilir). Ekran işlem bitene dek onu okur, ikinci bir işlem başlatmaz; sunucu
aynı site için ikinci açık işlemi reddeder.

## Sınırlar ve açık işler

- **Deneme sitesini canlıya aktarma** yok (bu sürümün kapsamı dışında).
- Tema güncellemelerini CelikPanel yapmaz (yayımlanmış sağlama değeri yok).
- wordpress.org'da olmayan, tek dosyalı ve zorunlu (mu-plugins) eklentiler listelenir ama
  panelce güncellenmez.
- Çekirdek güncellemesi, yeni sürümün artık getirmediği eski dosyaları yerinde bırakır
  (WordPress'in kendi güncelleyicisi bir eski dosya listesini siler; panel henüz silmez).
- Kopya için `wp-config.php` web klasöründe olmalı, veritabanı satırları düz `define()`
  olmalı ve sitenin veritabanı panelde alan adına bağlı olmalı.
- Sunucuda bir seferde bir WordPress işlemi çalışır.
