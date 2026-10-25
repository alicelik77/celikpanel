# v0.1.0-alpha.108

[English](RELEASE-NOTES-v0.1.0-alpha.108.md)

Bu, v0.1.0-alpha.107'yi izleyen sürümdür. **25 Ekim 2026'da yazılmış bir adaydır;
yayımlanmadı ve hiçbir parçası kurulu bir sunucuda değil. Kurulu panelleri yalnız
sahip, panelin kendi güncelleme ekranından günceller.** alpha.107'nin yayım
kaydından sonraki `main`'den `feat/alpha108-safe-lanes` dalında yazıldı (alpha.107
sürüm commit'i `2e68e6039e5d21160c28944ad9fd74b83747ec4d`).

## Kim güncellemeli, neden

Her sunucu. Boston ve Frankfurt'u alpha.107'ye güncelledikten sonra sahip şunları
buldu: bir üretici deposu hâlâ sınırlaması ve otomatik güncellemeleri olmadan
açılabiliyordu; PHP-FPM sürüm listesi neyi getirdiğini göstermeden kuruyordu;
Boston'un phpPgAdmin'i yanıt verdiği hâlde yanıt vermiyor diye bildirildi; Sury'nin
anahtarı "2021'de doldu" diyordu; ve PostgreSQL 16, 17 ve 18'i yan yana çalıştırmanın
bir yolu yoktu. Sahip şunu belirledi: **güvenlik ve stabilite her zaman özgürlükten
önce gelir** ([D-038 değişikliği](DECISIONS.tr.md)). Sahibin her sunucuda
yapacakları [işlem yönlendirmesinde](OPERATION-GUIDANCE.tr.md#sahibin-bulguları-iki-yol-ve-yan-yana-postgresql-d-038-25-ekim-2026).

## İki yol: varsayılan olarak işletim sisteminin paketleri, üretici deposu yalnız bilgili seçimle

- PHP-FPM ve PostgreSQL sayfaları, bileşen işletim sisteminden geldiğinde bunu
  söyler. Yalnız bir üretici deposunun getirdiği sürümler **kilitli** olarak,
  nereden geldikleri ve onları kimin yamaladığıyla listelenir.
- **Sury PHP kullan… / PGDG kullan…**, üç adımı birlikte gösteren tek bir pencere
  açar — depo ve sabitlenmiş anahtarı, sınırlama kuralı, kaynağının işletim
  sisteminin otomatik güncellemelerindeki yeri — her biri kendi dosyası, metni ve
  SHA-256'sıyla; tek bir onay bunları yazar, kaynak satırı en son. Otomatik
  güncellemeler kapalıyken, önce onları açacak komutu söyler.
- **Zaten açık olanlar (Boston, Frankfurt):** hiçbir şey kapatılmaz. Depo kartı neyin
  eksik olduğunu adlandırır ve **Tamamla…** yalnız onu yazar. Güvenlik güncellemeleri
  kartında, kapsanmamış bir deponun yanında aynı düğme vardır.
- Müşteriler hiçbir zaman depo açamaz. Birini kapatmak hiçbir şeyi kaldırmaz;
  ondan gelenler **artık güncellenmiyor** diye işaretlenir.
- phpMyAdmin, phpPgAdmin ve web postası varsayılan olarak dağıtımın PHP'sinde kalır;
  üretici sürümü siteler için diye işaretlenir.

## PostgreSQL 16, 17 ve 18 yan yana

- **PostgreSQL → Kümeler**, her kümeyi portu, durumu, veri dizini, veritabanları ve
  panelin kayıtlarıyla listeler.
- **PostgreSQL NN kur…**, kümesinin alacağı portu önce gösterir ve kurulum
  önizlemesinden geçer.
- **Portu değiştir…**, değişen satırı, kimin bağlı olduğunu ve kesintiyi gösterir,
  yeniden başlatır, yeni portu kanıtlar ve olmazsa eskisini geri koyar.
- **Bir veritabanını taşı…**, onu kopyalar (roller dahil, parolalar asla gösterilmez),
  her tablonun satır sayısını iki tarafta kanıtlar ve kaynağı korur. **Kopyaya
  geçir…**, **Geri geçir…** ve **Kaynağı kaldır…** ayrı, onaylı adımlardır.
- Bir ana sürümü kaldırmak, veritabanı tuttuğu sürece reddedilir; verisi korunur.
- MariaDB paketlerle yan yana çalışamaz; sayfası bunu söyler.

## Sahibin bulguları, düzeltildi

- Her kurulum yolu önizlemeyi gösterir ve dikkat çeken paketlerin onayını ister;
  PHP-FPM sürüm listesi ve Onar da dahil. Reddetme, yeniden Kur'a basmayı söyler.
- **Boston'da phpPgAdmin:** olası neden, Agent'ın `UMask=0027` ayarıdır; bu,
  CelikPanel'in denetim dosyasını PHP-FPM'e okunamaz yapar. Atılabilir bir Debian 13
  sanal makinesinde yeniden üretildi, Boston'da ölçülmedi: güncellemeden sonra
  **Yeniden denetle**'ye basmak bunu orada doğrular (neden başkaysa onu söyler). Bu düğme
  onu onarır ve aracı kanıtlar; "çalışmıyor" yalnız PHP-FPM gerçekten durmuşken
  söylenir; aynı PHP sürümünü kullanmak yalnız kanıtlar; kaybolan bir yanıt geri
  okunur, asla yeniden gönderilmez.
- **Sury'nin anahtarı:** bitiş tarihi en yeni imzasından okunur (2028 başı); "apt
  artık kabul etmiyor" yalnız apt öyle dediğinde görünür. Bir anahtarın süresi
  dolmak üzereyken kartta **Güncel anahtarını al…** vardır.
- Kurulum reddi artık bir sütuna sıkışmaz; Bileşenler satırları orta genişliklerde
  adlarını bütün tutar; kenar çubuğu simgelere daralır (bu tarayıcıda hatırlanır,
  klavyeyle erişilir, telefon değişmez); Roundcube'un satırında doğrudan bir
  **Güncelle** vardır; sahip komutları `sudo` ile başlar ve "root olarak oturum
  açtıysanız sudo yazmayın" der.

## Kalan D-038 maddeleri

- **Müşterilere sunulan** (PHP-FPM ve Node.js sayfaları): yönetici her sürüm hattını
  gizler ya da sunar; gizlenmiş bir sürümdeki site onu korur ve sahibi bir not görür.
- Sürüm listeleri bu sürümle **denendi** ya da **denenmedi** (kurulabilir) diye
  işaretler.
- Kaldırma pencereleri sunucunun kendi izlerinin gösterdiğini adlandırır: soketi ya
  da portu adlandıran web sunucusu satırları, ona bağlı servisler, şu an bağlı
  programlar.

## Yükseltme ve kurtarma

- Şema 45'ten **46**'ya (ekleyici: `database_servers.cluster`, tablo
  `postgresql_database_moves`). alpha.107 Panel'i şema 46'yı reddeder; bu yüzden geri
  dönüş, güncelleme öncesi anlık görüntünün geri yüklenmesidir. Sürüm kurtarma
  düzeneği geçiş 46'yı sabitler.
- Sıra 107'den 108'e. Güncellemeden sonra sunucuda hiçbir şey kendiliğinden
  değişmez: her yol adımı, port değişikliği ve taşıma yöneticiyi bekler.

## Kanıt

- Agent 4817 geçti, 29 atlandı (temiz bağlama ad alanında); Panel ve iç paketler
  7944 geçti, 45 atlandı, 0 başarısız; web 1904 geçti ve üretim derlemesi
  bütçelerinin içinde (iki yeni ekran parçası, `databaseClusters` ve `vendorLanes`,
  her birinin kendi tavanı var). Sürüm kurtarma düzeneği şema 46'yı sabitler; dört
  veritabanı takımı geçer, denetleyici takımları o konukta alpha.107'dekiyle aynı
  yalnız ortama bağlı hataları gösterir (orada git yok).
- Saklanan temel imajdan atılabilir bir Debian 13 sanal makinesinde bir gerçek sistem
  denemesi: dört test geçti (yan yana PostgreSQL, Agent'ın umask'i altında PHP aracı
  kanıtı, kaldırma izleri, Sury üretici yolu). İlk denemesi, dağıtımın
  postgresql-common'ı üzerine kurulan bir PGDG ana sürümünün kümesiz geldiğini buldu;
  kurulum artık onu gösterilen portta oluşturur
  ([kanıt](../deploy/e2e/d038/alpha108-debian13-20261025/README.md), arşiv
  SHA-256 `effaf7ce…a1604698`).
- Bir sahte sunucu üzerinde tarayıcı bakışı (EN ve TR, 1440 ve 390, açık) (grup 31).

Kurulu bir sunucuda ölçülmedi. Aday kabul değildir; P0.1–P0.5 **kısmi** kalır; hiçbir
P0 işi kapanmaz ya da ilerlemez.
