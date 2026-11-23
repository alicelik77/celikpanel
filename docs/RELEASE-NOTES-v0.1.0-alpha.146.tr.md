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

## Laboratuvarda denenenler

AÇIK.

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
