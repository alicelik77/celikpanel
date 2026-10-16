# v0.1.0-alpha.89 (16 Ekim 2026'da yayımlandı)

[English](RELEASE-NOTES-v0.1.0-alpha.89.md)

Bu, v0.1.0-alpha.88'i izleyen sürümdür. 16 Ekim 2026'da yayımlandı (bkz.
"Yayımlama kaydı").
v0.1.0-alpha.89 numarasını sahibin 13 Ekim 2026 tarihli kalıcı talimatı
uyarınca sürümü hazırlayan seçti; sahibin adını koyduğu bir numara değildir.
16 Ekim 2026'da (UTC) `release/alpha89` dalında, `main`'in
`1cb3628ff1969f3fc2e9f0d848b7adf629a815b0` noktasından (alpha.88 sürüm
commit'i) yazıldı.

## v0.1.0-alpha.88 ile aynı kod

Bu sürüm **v0.1.0-alpha.88 ile aynı ürün koduna** sahiptir.
`git diff --stat 1cb3628ff HEAD -- cmd internal web` denetimi boştur. Yalnız
sürüm sıra ilkesi (88'den sonra 89), kurucunun önyükleme sabitlemeleri,
README sürüm satırları, sabitlenmiş üç sözleşme testi ve bu belgeler farklıdır.

## Neden var

Sahip 16 Ekim 2026'da arka arkaya iki sürüm istedi ("iki sürüm birden
çıkart"). Güncelleme her zaman ondan önce kurulu olan sürümün sayfasında
çalışır; bu yüzden v0.1.0-alpha.88'e *doğru* güncelleme hâlâ alpha.87'nin
sayfasını gösterir. Adım adım yeni güncelleme görünümü ilk kez alpha.88'*den*
bu sürüme güncellerken görülür. Sahibin talimatıyla gerçek sistem kaydı ve uzun
laboratuvar testi yoktur.

## alpha.88'den alpha.89'a güncellerken ne göreceksiniz

- Beş adım çubuğu: indirme, imza ve sağlama toplamlarını denetleme, sunucuyu
  denetleme, paneli kurma ve yeniden başlatma, yeni sürümü doğrulama. Biten
  adımda onay işareti vardır; sonrakiler altta bekler.
- Tek sakin cümleli planlı yeniden başlama: "Panel yeniden başlıyor; bu adım
  birkaç dakika sürebilir. Bu sayfa kendiliğinden yeniden bağlanır."
- Sonda "v0.1.0-alpha.89 sürümüne güncellendi"; sonra sayfa yeniden yüklenir.

Yeniden başlama, alpha.88'e güncellemede bir kez ölçülür; bu güncellemenin
yeniden başlama çubuğu o ölçümü kullanır (beklenen süre
`max(son x 1,5; 90 sn)`, en çok 360 sn). Bu koddan okunmuştur; gözlenmemiştir.

## Sınırlar

- Sahibin talimatıyla gerçek sistem kaydı yok.
- Adım görünümünün ilk kez alpha.88'den güncellemede gözlenmesi hedeflenmişti;
  hiçbir sunucu alpha.88'i çalıştırmadığı için bu olmadı (bkz. "Yayımlama
  kaydı").
- P0.1-P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez;
  yayımlanma kabul olmaz.
- Geri kalan her şey (tasarım, beş adım, testler, diğer sınırlar)
  [v0.1.0-alpha.88 notlarında](RELEASE-NOTES-v0.1.0-alpha.88.tr.md).

Bu sürümün yayımlanması hiçbir kurulu sunucuyu güncellemez; her güncellemeyi
sahibi, panelin kendi güncelleme ekranından başlatır.

## Yayımlama kaydı

Aşağıdaki her şey 16 Ekim 2026 tarihlidir (saat dilimi belirtilmedikçe UTC).
Birleştirmeyi, depo değişkenini, etiketi, denetimleri, portal yayımını ve genel
sitenin okunmasını sürümü hazırlayan kişi yaptı, sahibin 15 Ekim 2026 tarihli
talimatı uyarınca; kurulu panel güncellemelerini yalnız sahip yapar. Sahibin 16
Ekim 2026 talimatıyla gerçek sistem kaydı yok. Bu girdi, aşağıdaki sonuçların
hiçbirini depoda bir dosya olarak saklamaz.

**Ne oldu.**

1. #218 numaralı çekme isteği hazırlayan kişi tarafından `main`'e squash olarak
   birleştirildi: commit `ac36fccf4e4238a8bc4624356a478ba8dab20b34`, ağaç
   `29f08b6bef1ca6e19c4f02890f83cfeb3f678433`. Üzerindeki koşu başarıyla
   sonuçlandı.
2. Hazırlayan kişi `CELIKPANEL_RELEASE_SEQUENCE` değişkenini 88'den 89'a
   çıkardı ve `v0.1.0-alpha.89` açıklamalı etiketini gönderdi. Etiketin koşusu
   (38874797963) başarıyla sonuçlandı; sürüm 2026-10-16T08:57:23Z'de altı
   dosyayla yayımlandı.
3. Bir indirmeden denetlendi (hazırlayan kişi; burada bir dosyada kayıtlı
   değil): arşivler bayt bayt aynı, 66.195.105 bayt, SHA-256
   `58c6911be7a1b640b0bafb595a8c2b54451d70d8238d51b1c726a76bb7f59526`; manifest
   sıra 89, `published_at` 2026-10-16T08:14:24Z der, imzası aynı anahtarla
   doğrulanır; 237 üye; `libexec/get.sh` 89'u sabitler (SHA-256
   `baaae2dce46465ba3198b8f3a6662e0590117e85586b81450149305b52aeaf08`);
   politika 89 der, önceki 88 (`1cb3628ff…`); `release.commit` ve
   `release.tree` etiketinkilerle eşittir. Arayüz paketi alpha.88'inkiyle bayt
   bayt aynıdır (`app-DAi4coqL.js`, aynı özet; doğrudan karşılaştırıldı); ürün
   kodu değişmediği için beklenen budur.
4. Portal paketi: 132.763.175 bayt, SHA-256
   `bb36ebce7b52261988651d9df234ebe9c26b45ec605feacb5bda9f2da1649f1e`, 54 üye,
   iki kez aynı derlendi; terfi aracı kabul etti; testler geçti. Hazırlayan kişi
   yayımladı (önceki v0.1.0-alpha.88): committed, yedek
   `httpdocs.before-v0.1.0-alpha.89.20261016T090510Z…`, doğrulama ok (26 istek),
   `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`.
5. 09:05:19Z'de dışarıdan okuma: `releases/latest.txt` v0.1.0-alpha.89, HSTS
   var, `get.sh` 89'u sabitliyor, `releases/latest.json` sıra 89, commit
   `ac36fccf…`, SHA-256 `58c6911b…`.

**Sahibin bildirdikleri (16 Ekim 2026, iki ekran görüntüsü, yaklaşık 09:30Z).**
Sahip iki güncellemeyi de panelin kendi güncelleme ekranından başlattı. İki
sunucu da önceki sürümünden **doğrudan v0.1.0-alpha.89'a** güncellendi ("doğrudan
89'a güncelledi"; sıra tabanı buna izin verir): Frankfurt alpha.87'den, Boston
çalıştırdığı sürümden (bildirilmedi; alpha.85 ya da sonrası). İkisinde de
Ayarlar, CelikPanel güncellemeleri sayfası "Geçerli sürüm v0.1.0-alpha.89" ve
"Geçerli commit ac36fccf4e4238a8bc4624356a478ba8dab20b34" gösterir; kenar çubuğu
"CelikPanel · v0.1.0-alpha.89 · build ac36fccf4e42" der; ve "v0.1.0-alpha.89
sürümüne güncellendi — build ac36fccf4e42" kartı beş tamamlanmış adım sayar
(güncelleme paketini indir; imzayı ve sağlama toplamlarını denetle; kurmadan önce
sunucuyu denetle; yeni sürümü kur ve paneli yeniden başlat; yeni sürümü doğrula).
Yani yeniden yüklemeden sonra alpha.89 arayüzü, tamamlanan güncellemenin
sunucudaki adım planını çizer. Her güncelleme önceki sürümün sayfasında
çalıştığı için yeniden başlama sırasındaki canlı adım görünümü **görülmedi**; ilk
kez bir sonraki güncellemede, alpha.89'dan görülür. Frankfurt'un sayfası ayrıca
"Bu kanalda yeni güncelleme yok." metnini bir uyarı üçgeni simgesiyle gösterir:
iyi bir durum uyarı simgesiyle çizilmiş; sonraki sürüm için D-033 maddesi olarak
kaydedildi (bir onay gibi okunmalı, örneğin "En son sürümdesiniz", nötr ya da
başarı simgesiyle); burada bunun için hiçbir şey değiştirilmedi.

**Yapılmayanlar ve yayımlamanın değiştirmedikleri.**

- Kurulu sunucuları sürümü hazırlayan kişi güncellemedi.
- Bildirilmeyen ve kaydedilmeyenler: lisans sayfası, Agent'ın sürümü,
  Frankfurt'un kurulum son adımı, Boston'un "Bu sunucunun kurulumunu
  genişletin" seçeneği, `postconf -n >/dev/null`.
- Sınırlar, [alpha.88 notlarının](RELEASE-NOTES-v0.1.0-alpha.88.tr.md) yayımlama
  kaydındaki gibidir: etiket dosyaları Windows'ta `git archive` ile, terfi
  aracı provası yok, önceki paketle dosya dosya karşılaştırma yok, yayımlayıcının
  kendi testi çalıştırılmadı.
- Yayımlanma kabul değildir. P0.1–P0.5 kısmi kalır; hiçbir P0 işi kapanmaz ya da
  ilerlemez.

Bu sürümü yalnız CelikPanel'in güncelleme arayüzünden kurun. Yayımlanması kurulu
sunucuları güncellemez; bir sunucunun güncellemesini yalnız sahibi, panelin
kendi güncelleme ekranından başlatır.
