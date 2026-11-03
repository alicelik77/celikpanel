# v0.1.0-alpha.118

[English](RELEASE-NOTES-v0.1.0-alpha.118.md)

Sahibin 3 Kasım 2026'da verdiği kararlarla (D-044): gerçekten işleyen plan
sınırları, uygulama kataloğu, WordPress araçları, zararlı yazılım taraması, uzak
yedek ve ilk yapay zekâ ve otomasyon uygulamaları. **Kurulu panelleri yalnız
sahibi, panelin kendi güncelleme ekranından günceller.**

## İşleyen plan sınırları (D-044)

- **Trafik.** Her sitenin web trafiği takvim ayına göre sayılır (ziyaretçiye
  giden veri). Aboneliğin ya da bayisinin havuzunun sınırı dolunca yalnız
  siteler "trafik sınırı doldu" gösterir; posta, veritabanı ve dosya erişimi
  çalışmaya devam eder. Ayın 1'inde ya da sınır artırılınca birkaç dakika içinde
  yeniden açılır. Sayım birkaç dakika geriden gelebilir; ekran bunu söyler.
- **Disk.** Gerçek kullanım ölçülür: site dosyaları, posta ve veritabanları.
  Dolunca yeni bir şey oluşturulamaz ve yükleme durur (dosya yöneticisi, FTP,
  SFTP ve PHP yüklemeleri); siteler çalışır. 7 gün sınırın üstünde kalırsa
  abonelik askıya alınır; sınırın altına inince kendiliğinden açılır.
- **Posta kutuları.** Dolu posta kutusu yeni postayı reddeder (Dovecot'un kendi
  kotası); kutuya %90'da bir kez uyarı iletisi gider.
- %80 ve %100 bildirimleri müşteriye, bayisine ve yöneticiye gider.
- Daha önce kaydedilen ama hiç uygulanmayan trafik sınırları sınırsız olarak
  başlar ve yöneticiye bir kez bildirilir; kimsenin görmediği eski bir sayı
  yüzünden site kapanmaz.

## Uygulama kataloğu

- Uygulamalar sekmesi artık bir katalog: WordPress'e PrestaShop, Nextcloud,
  Drupal, Joomla, MediaWiki ve phpBB eklendi. Her biri resmi sürümdür ve açılmadan
  önce CelikPanel'de sabitlenmiş parmak iziyle denetlenir.
- Hangi uygulamaların sunulacağını yönetici seçer (Planlar ve haklar →
  Sunulanlar). WordPress sunulmaya devam eder; diğerleri kapalı başlar.
- Kurulumdan önce ekran neyin eksik olduğunu söyler (PHP sürümü, PHP paketleri,
  boş site, disk alanı). Uygulama kendini kurar ve yönetici parolası bir kez
  gösterilir. Başarısız kurulum yeniden denenebilir; kaldırırken veritabanı
  yalnız işaretlenirse silinir.

## WordPress araçları

- WordPress sürümü, eklentiler ve temalar, bekleyen güncellemelerle görünür.
- Çekirdek ve eklentiler önce yedek alınarak güncellenir; güncelleme başarısız
  olursa yedek geri konur. İsteğe bağlı bir anahtar WordPress güvenlik ve bakım
  sürümlerini kendiliğinden kurar.
- WordPress sitesi aynı aboneliğin boş bir sitesine deneme için kopyalanabilir.

## Zararlı yazılım taraması

- ClamAV ile (Bileşenler'den kurulur): bir siteyi, tüm aboneliği ya da haftada
  bir bütün siteleri tarayın. Virüslü dosyalar karantinaya alınır ve geri
  konabilir; hiçbir şey kendiliğinden silinmez. Müşteri, bayisi ve yöneticiler
  bilgilendirilir. Eski imzalar taramayı durdurmaz, uyarı verir.

## Uzak yedek

- Yedekler sizin denetiminizdeki bir depolamaya da gönderilebilir: S3 uyumlu
  depolama (AWS, Backblaze B2, Wasabi, MinIO, Cloudflare R2) ya da SFTP sunucusu.
- Her kopya sizin parola cümlenizle şifrelenir ve CelikPanel olmadan `gpg` ile
  açılır (adımlar docs/REMOTE-BACKUPS.tr.md'de).
- Bağlantı denemesi, "Ayrıca gönder…", saklamalı günlük ve haftalık zamanlama,
  hedeften geri yükleme. Hakları izin veriyorsa hesaplar kendi hedeflerini
  ekleyebilir.

## Yapay zekâ ve otomasyon uygulamaları

- n8n ve Open WebUI bir sitede kapalı bir hizmet olarak çalışır: kendi hesabı,
  bellek ve işlemci sınırı var; yalnız site üzerinden HTTPS ile erişilir.
  Yönetici risk özetini okuyup sunmadan hiçbiri açılmaz; yönetici bir uygulamayı
  güvenlik beklemesine alabilir.
- n8n, komut çalıştıran ve sunucu dosyalarını okuyan parçaları kapalı başlar.
- n8n'in lisansı onu barındırma hizmeti olarak satmayı kısıtlar; bu yüzden şimdilik
  yönetici n8n'i yalnız kendi müşterilerine sunabilir, bayiler üzerinden
  sunamaz. Ekran nedenini söyler.
- Kendi başına komut çalıştıran uygulamalar (OpenClaw, Hermes Agent, Paperclip)
  listede görünür ama henüz sunulmaz; katalog nedenini söyler.

## Düzeltilenler

- İlk kurulum DNS'i kurmaya başladığında sunucu meşgulse, kurulum artık "sonuç
  hâlâ bilinmiyor" durumunda kalmıyor. Bekliyor, hiçbir şeyin değişmediğini
  söylüyor ve kendiliğinden sürüyor. Sunucu 30 dakikadan uzun meşgul kalırsa
  kurulum duruyor ve düzeltilmiş planı incelemenizi istiyor.
- Bileşenler'de Debian ya da Ubuntu'nun kendi PHP sürümü seçilince, uygulama
  kataloğunun istediği PHP eklentileriyle birlikte dağıtımın paketlerinden kurulur;
  Sury deposu gerekmez.
- FTP/SFTP/SSH erişimi kaydedilince uyarılar artık sayfanın düğmelerini örten
  açılır kutular yerine sayfada görünüyor; sayfanın zaten söylediği tekrarlanmıyor,
  eksik komut satırı araçları adıyla ve kimin kuracağıyla yazılıyor.
- Büyük arşivlerin açılmasını ve aynı anda yapılan dosya okumalarını
  bozabilen bir dosya tanıtıcı hatası giderildi.

## Yeni haklar

WordPress araçları, zararlı yazılım taraması sonuçları, uzak yedek hedefleri ve
yapay zekâ ve otomasyon uygulamaları yeni haklardır. Sınırsız planlar alır; özel
hak listesi olan hesaplar yalnız biri seçerse alır.

## Bilinen sınırlar

- Yedekler disk kullanımına sayılmaz; disk sınırını aşan abonelikte zamanlanmış
  yedekler yine çalışır. Karantinadaki dosyalar disk sayımından çıkar.
- Disk dolunca SSH erişimi salt-okur yapılamaz.
- Trafik yalnız web sitelerini sayar (posta ve FTP değil).
- Matomo ve Moodle henüz sunulmuyor. PrestaShop'ta kolay adresler kapalı;
  Nextcloud arka plan işleri zamanlanmış görev olarak kurulmuyor.
- WordPress tema güncellemeleri WordPress'in kendi ekranına bırakıldı
  (wordpress.org temalar için denetim değeri yayımlamıyor).
- n8n ve Open WebUI'de yalnız ana paket sabitlenir; kurulumda çektikleri paketler
  tek tek denetlenmez. Open WebUI birkaç GB'tır ve yalnız Debian 12 ile Ubuntu
  24.04'te çalışır; n8n panelin Node.js 24 ortamını kullanır.
- Arch Linux'ta eksik PHP eklentileri adlandırılır ama panel henüz açamaz (sahip
  `/etc/php/conf.d` içine `extension=` ekler). Dağıtımınkinden başka bir PHP
  sürümü hâlâ Sury deposunu ister.
- Daha önce logrotate olmadan kurulmuş sunucular, yönetici kurana dek
  (`sudo apt install logrotate` ya da `sudo pacman -Syu --needed logrotate`)
  "logrotate kurulu değil" gösterir; trafik yine sayılır. Arch'ta ilk ClamAV imza
  indirmesi başarısız olursa ClamAV kurulumu hâlâ hata verir.
- Uzak yedekler tam kopyadır (artımlı yedek yok); panelden geri yükleme aynı
  siteye yapılır.
- alpha.117'ye geri dönüş güncellemeden önce alınan veritabanı kopyasını geri
  koyar; güncellemeden sonra panelde yapılanlar kaybolur.

## Kanıt

Birleştirilmiş ağaç: Go derleme ve vet temiz; `internal` ve `cmd/panel` testleri
geçti; Agent testlerinde yalnız alpha.117'de de düşen, deneme ortamına özgü
bilinen hatalar var; 2348 web testi paket sınırları içinde geçti;
release-recovery fikstürleri şema 61'e genişletildi. <!-- LAB --> Laboratuvar
denemesi yayın kaydına yazıldı.
