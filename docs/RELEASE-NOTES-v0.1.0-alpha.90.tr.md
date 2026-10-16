# v0.1.0-alpha.90 (aday, yayımlanmadı)

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
- Canlı adım görünümü kurulu bir sunucuda ilk kez bu güncelleme sırasında
  gözlenir.
- Yeniden başlama süresi bir sunucuda ilk kez bu güncellemede ölçülür; daha
  önce ölçüm olmadığından yeniden başlama çubuğu 90 saniye bekler (koddan
  okunmuştur, gözlenmemiştir).
- P0.1-P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez;
  yayımlanma kabul olmaz.
- Şema 44, geçiş yok; kurtarma davranışı değişmez.

## Yayımlama kaydı

Yayımlanmadı.

Bu sürümü yalnız CelikPanel'in güncelleme arayüzünden kurun. Yayımlanması
kurulu sunucuları güncellemez; bir sunucunun güncellemesini yalnız sahibi,
panelin kendi güncelleme ekranından başlatır.
