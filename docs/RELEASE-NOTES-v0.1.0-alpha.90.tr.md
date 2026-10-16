# v0.1.0-alpha.90 (16 Ekim 2026'da yayımlandı)

[English](RELEASE-NOTES-v0.1.0-alpha.90.md)

Bu, v0.1.0-alpha.89'u izleyen sürümdür. v0.1.0-alpha.90 numarasını sahibin
13 Ekim 2026 tarihli kalıcı talimatı uyarınca sürümü hazırlayan seçti.
16 Ekim 2026'da (UTC) `release/alpha90` dalında, `main`'in
`d935170818f315d7c6c94e1e7324417bb5a38f8e` noktasından (alpha.89 sürüm
commit'i `ac36fccf4e4238a8bc4624356a478ba8dab20b34` sonrasında) yazıldı.

## Ne değişti

Bir düzeltme (D-033; sahibin 16 Ekim 2026'da Frankfurt'ta gördüğü durum):
"Güncellemeyi kontrol et" bir şey bulmadığında Ayarlar, CelikPanel
güncellemeleri sayfası "Bu kanal için yeni güncelleme yok." metnini bir uyarı
üçgeniyle gösteriyordu. İyi bir durum uyarı gibi çiziliyordu. Artık normal
yüzeyde onay işaretiyle **"En son sürümü kullanıyorsunuz."** ("You are on the
latest version.") yazar. Kart kanalları ayırt etmediği için cümle kanal adı
vermez.

Aynı kartta: bulunan bir güncelleme ("İmzalı bir güncelleme bulundu.") indirme
işaretli nötr bir bilgidir; sunucunun hazır olup olmadığı okunurken kutu
nötrdür. Uyarı işareti yalnız bir sorunun ya da bilinmeyen bir durumun olduğu
yerde kalır: okunamayan bir denetim, hazır olmayan sunucu, başlatılamayan
güncelleme, daha önce başarısız olan deneme ve alpha yedek uyarısı.

## Neden var

Sahip 16 Ekim 2026'da istedi ("90'ı çıkar"). Güncelleme, ondan önce kurulu
olan sürümün sayfasında çalışır; bu yüzden adım adım güncelleme görünümü ilk
kez bir sunucu **alpha.89'dan alpha.90'a** güncellenirken canlı görülür.
Sahibin talimatıyla gerçek sistem kaydı ve uzun laboratuvar testi yoktur.

## alpha.89'dan alpha.90'a güncellerken ne göreceksiniz

- Birbiri ardına dolan beş adım çubuğu: güncelleme paketini indirme; imzayı ve
  sağlama toplamlarını denetleme; kurmadan önce sunucuyu denetleme; yeni sürümü
  kurma ve paneli yeniden başlatma; yeni sürümü doğrulama.
- Tek sakin cümleli planlı yeniden başlama: "Panel yeniden başlıyor; bu adım
  birkaç dakika sürebilir. Bu sayfa kendiliğinden yeniden bağlanır."
- Sonda "v0.1.0-alpha.90 sürümüne güncellendi".

## Sınırlar

- Sahibin talimatıyla gerçek sistem kaydı yok.
- Canlı adım görünümünün bu güncelleme sırasında gözlenmesi hedeflenmişti;
  görüldüğü bildirilmedi (bkz. "Yayımlama kaydı").
- Yeniden başlama süresi bir sunucuda ilk kez bu güncellemede ölçülür (ölçümün
  kendisi bildirilmedi); daha
  önce ölçüm olmadığından yeniden başlama çubuğu 90 saniye bekler (koddan
  okunmuştur, gözlenmemiştir).
- P0.1-P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez;
  yayımlanma kabul olmaz.
- Şema 44, geçiş yok; kurtarma davranışı değişmez.

## Yayımlama kaydı

Aşağıdaki her şey 16 Ekim 2026 tarihlidir (saat dilimi belirtilmedikçe UTC).
Birleştirmeyi, depo değişkenini, etiketi, denetimleri, portal yayımını ve genel
sitenin okunmasını sürümü hazırlayan kişi yaptı, sahibin 15 Ekim 2026 tarihli
talimatı uyarınca; kurulu panel güncellemelerini sahip, panelin kendi güncelleme
ekranından başlattı. Sahibin talimatıyla gerçek sistem kaydı yok. Bu girdi,
aşağıdaki sonuçların hiçbirini depoda bir dosya olarak saklamaz.

**Ne oldu.**

1. #220 numaralı çekme isteği hazırlayan kişi tarafından `main`'e squash olarak
   birleştirildi: commit `1743ac69edb0e5723dc60d992b53094dcc4b1431`, ağaç
   `bbfa5305cc8c5b6bb4a34a2e640633582dc7534b`. Üzerindeki koşu başarıyla
   sonuçlandı.
2. Hazırlayan kişi `CELIKPANEL_RELEASE_SEQUENCE` değişkenini 89'dan 90'a
   çıkardı ve `v0.1.0-alpha.90` açıklamalı etiketini gönderdi (nesne
   `9c98483d71b4a747f522f87c476f22125868bca7`). Etiketin koşusu başarıyla
   sonuçlandı; sürüm 2026-10-16T14:42:18Z'de altı dosyayla yayımlandı.
3. Bir indirmeden denetlendi (hazırlayan kişi; burada bir dosyada kayıtlı
   değil): arşivler bayt bayt aynı, 66.194.918 bayt, SHA-256
   `3719aba4f60b3eb3da0762b2d1c57302b628cdb6c958d72c2a5ac0430e2ff3e1`; manifest
   sıra 90, `published_at` 2026-10-16T13:58:20Z der (SHA-256 `0f27855c…`), imzası
   doğrulanır (anahtar `7eadeb0b…`); 237 üye; `libexec/get.sh` 90'ı sabitler
   (SHA-256 `eb55a90102fe21be27483ceaaf2fcb1bcf7846a1285f0253ad7a06d333c41bd3`);
   politika 90 der, önceki 89 (`ac36fccf4…`); `release.commit` ve
   `release.tree` etiketinkilerle eşittir. Arayüz paketi `app-HNXfBLHb.js`,
   248.218 bayt, SHA-256
   `b743c25f66e712623593ae333e28a8b82bc9d764eba66830976e3e5c4b21609c`.
4. Portal paketi: 132.762.586 bayt, SHA-256
   `b2a31a43d2b1e497b7dc9da53f405d79fa70d7193cb86f518f5d367e74b3acef`, 54 üye,
   iki kez aynı derlendi; terfi aracı kabul etti; testler geçti. Hazırlayan kişi
   yayımladı (önceki v0.1.0-alpha.89): committed, yedek
   `httpdocs.before-v0.1.0-alpha.90.20261016T145144Z…`, doğrulama ok (26 istek),
   `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`.
5. 14:51:50Z'de dışarıdan okuma: `releases/latest.txt` v0.1.0-alpha.90, `get.sh`
   90'ı sabitliyor, `releases/latest.json` sıra 90, commit `1743ac69…`, SHA-256
   `3719aba4…`.

**Sahibin bildirdikleri (16 Ekim 2026, ekran görüntüleri; alpha.89'dan
güncelleme).** Sahip iki güncellemeyi de panelin kendi güncelleme ekranından
başlattı.

- **Boston:** temiz bir güncelleme. Kart "v0.1.0-alpha.90 sürümüne güncellendi",
  build `1743ac69edb0`, beş adım tamam der; "Güncellemeyi kontrol et"ten sonra
  yeşil "En son sürümü kullanıyorsunuz." (bu sürümün D-033 düzeltmesi, görüldü).
- **Frankfurt:** T+01:23'te kırmızı çarpı, "Son kaydedilen adım: kurtarma
  sürüyor" ve sunucunun satırı "reviewed updater failed: exit status 1: Certbot
  renewal scheduler state could not be restored"; T+01:39'da aynı işlem için
  (`13193201ba7a3d832bb64cc6d972f8a5`) "Güncelleme doğrulandı". Tarayıcı, sahip
  Ctrl+Shift+R'ye basana kadar alpha.89 arayüzünü korudu: kenar çubuğu alpha.89,
  "Geçerli sürüm" alpha.90 gösteriyordu; eski uyarı üçgenli "yeni güncelleme yok"
  satırı da oradaydı. Yeniden yüklemeden sonra sayfa "En son sürümü
  kullanıyorsunuz." der ama "güncellendi" kartı yoktur, çünkü güncelleme kurtarma
  kaydı üzerinden bitti. Hatanın nedeni ve düzeltmesi [alpha.92
  notlarında](RELEASE-NOTES-v0.1.0-alpha.92.tr.md). Tarayıcının eski arayüzü
  neden korduğu burada araştırılmadı.
- **Boston'un kartı kopyalanınca** yapı satırı "build
  1743ac69edb01743ac69edb0e5723…" olarak yapıştı (kısa ve tam kimlik birbirine
  yapışık); alpha.92'de düzeltildi.
- Bu güncelleme sırasındaki canlı adım görünümü görüldü diye **bildirilmedi**:
  güncelleme alpha.89'un sayfasında çalıştı, Frankfurt'ta ise eski iletişim
  kutusuna düşerek başarısız göründü.

**Yapılmayanlar ve yayımlamanın değiştirmedikleri.**

- Kurulu sunucuları sürümü hazırlayan kişi güncellemedi.
- Bildirilmeyen ve kaydedilmeyenler: Frankfurt'un Alan adları'ndaki "Altyapı
  bölgesi" satırı, Boston'un eş sunucu kaydıyla posta genişletmesi (ikisi de
  alpha.91 kodundadır, alpha.92 ile gelir), lisans sayfası, Agent'ın sürümü,
  `postconf -n >/dev/null`.
- Önceki yayımlama kayıtlarının sınırları ([alpha.88
  notları](RELEASE-NOTES-v0.1.0-alpha.88.tr.md)) kalkmış olarak bildirilmedi.
- Yayımlanma kabul değildir. P0.1–P0.5 kısmi kalır; hiçbir P0 işi kapanmaz ya da
  ilerlemez.

Bu sürümü yalnız CelikPanel'in güncelleme arayüzünden kurun. Yayımlanması
kurulu sunucuları güncellemez; bir sunucunun güncellemesini yalnız sahibi,
panelin kendi güncelleme ekranından başlatır.
