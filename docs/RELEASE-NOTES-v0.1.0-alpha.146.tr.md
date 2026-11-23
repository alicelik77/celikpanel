# v0.1.0-alpha.146

[English](RELEASE-NOTES-v0.1.0-alpha.146.md)

**Yalnız herkese açık site için bir sürüm.** Yeni celikpanel.net'i bu sürümün
indirme portalıyla getirir. Panelin ve ajanın kodunda hiçbir şey değişmez;
panelin veritabanı düzeni de değişmez.
**Kurulu panelleri yalnız sahibi, panelin kendi güncelleme ekranından
günceller.**

## alpha.145'ten güncellemek

1. Her zamanki gibi panelin kendi güncelleme ekranından güncelleyin.
2. **Panelin veritabanı düzeni değişmez** (şema 81 olarak kalır).
   alpha.145'e dönmek olağan geri almadır; bir anlık görüntünün geri
   yüklenmesi gerekmez.
3. Güncellemeden sonra basılacak bir düğme ya da elle değiştirilecek bir şey
   yoktur.

## Yeni celikpanel.net

Site, Türkçe ve İngilizce, her sayfası iki dilde olan bir teknik bilgi sayfası
olarak yeniden yapıldı:

- **Ana sayfa:** CelikPanel'in ne olduğu ("Linux sunucular için hosting
  kontrol paneli"), panelin örnek verili gerçek bir ekranının yanında özellik
  listesi, kurulum komutu, desteklenen sistemler, yayımlanmış sürüm notlarının
  söylediği biçimiyle müşterileri birbirinden ayırma (henüz yapılmamış
  olanlarla), panel durduğunda neyin çalıştığı, tarihli ölçümler, bugünkü ve
  1.0 sürümünden itibaren lisans, henüz sunulmayanlar.
- **Kurulum:** adımlar, komut, kurulum sihirbazının DNS seçimleri ve
  sunucudaki bir değişikliğin uyduğu kurallar.
- **Sistemler:** bugün desteklenen Linux sistemleri, dağıtımların kendi
  bitiş tarihleriyle; her birinde neler olduğu ve olmadığı, güncellemelerin
  nasıl işlediği.
- **Lisans:** lisansın bugün nasıl işlediği, 14 Kasım 2026'da belirlenen
  1.0 sürümünden itibaren geçerli koşullar, henüz belirlenmeyenler ve lisansa
  hiçbir zaman bağlı olmayanlar.
- **Sürümler:** son sürüm ve dosyaları; indirdiğinizi elle nasıl
  denetleyeceğiniz (artık imzalı manifest'teki arşiv özetini paketin özetiyle
  karşılaştıran adımla).
- **Değişiklikler** (yeni): v0.1.0-alpha.141'den itibaren her sürümün
  değiştirdikleri, birkaç sade satırda, notlarının tamamına bağlantıyla.
- **Bilinen sorunlar** (yeni): numaralı vakalar (CP-2026-1'den itibaren),
  kendi sözlerimizle; durumları, o sırada ne yapılacağı ve onları düzelten
  sürümle; bugün bir sorunun nasıl bildirileceği (güvenlik sorunları gizli
  olarak, diğerleri herkese açık geri bildirim deposunda). Bildirenin adı,
  sunucusu ya da sözleri hiçbir zaman gösterilmez.

Sitedeki her sayı ve her ifade depodan alınır; sitenin kendi testleri, kaynak
gösterilen her satırın sitenin dayandığı şeyi hâlâ söylediğini denetler.

## Sitenin dışında değişenler

Bu sürüm, panelin ya da ajanın çalıştırdığı hiçbir dosyayı değiştirmez.
alpha.145'in sürüm commit'iyle karşılaştırıldığında, `site/` ve
`download-portal/` içindeki üretilen sayfalar dışında farklı olan dosyalar:

- `download-portal/get.sh`: sabitlenen sürüm, artık alpha.146 (her sürümde
  olduğu gibi).
- `deploy/release-sequence-policy`, `README.md`, `README.tr.md`: bu sürümün
  sürüm sabitlemeleri.
- `deploy/build-download-portal.sh`, `deploy/promote-download-portal.py`,
  `deploy/verify-download-portal-public.py`: indirme portalının nasıl
  hazırlandığı, yayımlandığı ve dışarıdan denetlendiği. celikpanel.net
  yayımlanırken çalışırlar; CelikPanel çalıştıran bir sunucuda hiçbir zaman
  çalışmazlar.
- `deploy/test-*` (portal, komut, yayımlama, doğrulayıcı, sürüm sırası ve
  imzalı manifest sözleşmeleri): testlerdir; sürüm arşivine girmezler.
- `web/tools/browser-inspect/`: sitenin ekranlarını alan araç; yalnız bir
  geliştiricinin makinesinde çalışır.
- `DESIGN.md` ve `.gitattributes`: sitenin yazı boyutları ve git'in sitenin
  listesini ve ekranlarını nasıl sakladığı.
- `ROADMAP.md`, `ROADMAP.tr.md`, `docs/RELEASE-NOTES-v0.1.0-alpha.145*.md` ve
  `internal/recoveryruntime/dns_application_compatibility_linux.go` yalnız
  alpha.145'in main'de zaten bulunan yayım kaydı kadar farklıdır.

## Güncellemeden sonra ne beklenir

- Panel yeni sürümü gösterir; şema adımı yoktur.
- Her güncellemenin yaptığının dışında hiçbir hizmet yeniden başlatılmaz.
- alpha.145'e dönmek olağan geri almadır.

## Bilinen sınırlar

- alpha.145'in notlarının bilinen sınır olarak listelediği her şey olduğu gibi
  kalır ([alpha.145 sürüm notları](RELEASE-NOTES-v0.1.0-alpha.145.tr.md)).
- Ubuntu'da güncellemeye basıldığında sistemin kendi otomatik paket
  güncellemesi çalışıyorsa güncelleme hiçbir şeyi değiştirmeden reddedilir;
  o iş bitince yeniden basılabilir (kart, paket yöneticisi gerçekten boşa
  çıkmadan biraz önce sunucunun hazır olduğunu söyleyebilir).

## Laboratuvarda denenenler

23 Kasım 2026'da (UTC yaklaşık 10:30–11:10) iki deneme sunucusunda bir
deneme yapıldı: PowerDNS, posta ve web postası olan bir Debian 13 ana DNS
sunucusu ve BIND'li bir Ubuntu 24.04 yedek DNS sunucusu. İkisi de alpha.145'in
koduyla başladı ve yalnız laboratuvara ait v0.1.0-alpha.154 etiketiyle
derlenen bu sürümün koduna panelin kendi güncelleme sayfasından güncellendi.
Laboratuvarın indirme portalı bu sürümün yeni sitesini sundu. Ağır hata yok.

- **İki güncelleme de kendiliğinden tamamlandı.** Ana sunucu: 10:35:54'te
  basıldı, 10:41:22'de bütün adımları işaretli "güncellendi" ile bitti. Yedek
  sunucu: 10:43:18'deki ilk basış 10:45:17'de reddedildi, çünkü Ubuntu'nun
  kendi günlük paket işi 10:44:05'te başlamıştı; pencere paket yöneticisinin
  meşgul olduğunu ve hiçbir şeyin değiştirilmediğini söyledi, sunucu
  alpha.145'in koduyla çalışmayı sürdürdü. O iş 10:47:55'te bittikten sonra
  10:50:22'deki ikinci basış 10:53:59'da "güncellendi" ile bitti.
- **Şema değişmedi:** iki sunucu da şema 81'de kaldı, yeni bir adım yok.
- **Web postasına ve web sunucusuna dokunulmadı:** iki sunucuda da web
  postasının yapılandırması ve yerel kapısı içeriğini ve zamanını korudu, web
  sunucusu yeniden yüklenmedi, PHP yeniden başlatılmadı.
- **Panelin açılışında yenilenenler, her güncellemede olduğu gibi:** panel
  yeniden açılırken her sunucuda Postfix iki kez yenilendi, Dovecot bir kez
  yeniden yüklendi; alpha.145'in laboratuvar denemesinde panelin açılışlarında
  olanın aynısı.
- **Siteler, DNS ve posta yanıt vermeyi sürdürdü:** ana sunucunun
  güncellemesi sırasında dışarıdan üç siteye gönderilen 756 isteğin hepsi
  yanıt aldı; iki güncellemeden sonra siteler, iki panel ve web postası
  sayfası önceki gibi yanıt verdi, posta 25, 587 ve 993 numaralı portlarda
  yanıt verdi, iki sunucunun DNS yanıtları öncekiyle aynıydı. Panelin olağan
  sayfaları iki sunucuda da yeni sürümü gösterdi.
- **Laboratuvar portalının sunduğu biçimiyle yeni site:** ana sayfa, kurulum,
  sistemler, lisans, sürümler, değişiklikler ve bilinen sorunlar; İngilizce ve
  Türkçe, masaüstü ve telefon genişliğinde: her sayfa kendi dilinde, sürüm ve
  tarih doldurulmuş, doldurulmamış yer tutucu kalmadan açıldı; stil
  dosyaları, iki yazı tipi ve ana sayfanın ekran görüntüleri yüklendi;
  yüklenemeyen dosya, tarayıcı hatası ve telefonda yana kayma olmadı.
  Sayfalar arasındaki her bağlantı ve her çapa doğru yere çıktı.
  Değişiklikler sayfası bu sürümü 23 Kasım 2026 tarihiyle gösteriyor. Sürümler
  sayfasının denetim komutları, sayfanın kendi bağlantılarıyla indirilen
  dosyalar üzerinde yazıldığı gibi elle çalıştırıldı (İngilizce ve Türkçe):
  sağlama toplamı tuttu, imza doğrulandı, imzalı manifest'teki arşiv özeti
  arşivinkine eşitti. (Laboratuvarın portalı laboratuvarın imza anahtarını
  sunduğu için anahtarın özeti sayfanın yazdığından farklıydı; beklenen
  buydu.)
- **Önceki portalla karşılaştırma:** `latest.txt` aynı biçimde ve yeni sürümü
  adlandırıyor, imza anahtarı dosyası bayt bayt aynı, `get.sh` yalnız sürümü
  adlandıran iki satırda farklı. Üye alanının seçici dosyaları (`account/`)
  alpha.145'inkilerle aynı ve aynı üye uygulaması sürümünü adlandırıyor.

**Laboratuvarda denenmeyenler:** üye giriş sayfalarının kendisi
(laboratuvarın portalı PHP çalıştırmaz; bu sürüm yayımlanırken canlı sitede
salt-okur denetlenir); sayfadaki komutla sıfırdan kurulum; başka sitelere
giden bağlantılar; Arch Linux'ta hiçbir şey.

## Kanıt

Şema değişikliği yok: `internal/db/migrations/` altında hiçbir dosya
değişmez; düzen 81 olarak kalır. Panelin hiçbir ajan çağrısı, yolu, denetim
kaydı ya da ekranı değişmez.

Sitenin testleri (`go test ./site/...`): depodaki sayfalar üreticinin
yazdığıyla aynıdır; iki dilde aynı metinler vardır ve her metin kullanılır;
sayfalar portalın içerik kuralına uygun ve düzgün biçimlidir; her bağlantı ve
çapa bir yere çıkar; her sayının, alıntılanan ifadesi kaynak gösterilen
satırda bulunan bir kaynağı vardır; başka bir kontrol panelinin adı geçmez;
başlıklar "Linux sunucular" der; isteğe bağlı bir özellik isteğe bağlı
olduğunu söyler; stillerde süs yoktur; her değişiklik satırı ve vaka bir sürüm
notu satırına dayanır; alpha.141'den itibaren yayımlanmış her sürümün
değişiklik dosyası vardır; vakalarda bildirene ait alan yoktur. Portal
sözleşme testleri hazırlanan portalı denetler: listelenen her sayfa ve dosya
gözden geçirilmiş baytlarıyla gönderilir, sürüm değerleri doldurulur, kurulum
komutu ve dil bağlantıları doğrudur.
