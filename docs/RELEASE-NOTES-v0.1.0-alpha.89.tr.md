# v0.1.0-alpha.89 (aday, yayımlanmadı)

[English](RELEASE-NOTES-v0.1.0-alpha.89.md)

Bu, v0.1.0-alpha.88'i izleyen adaydır. Yayımlanmadı (bkz. "Yayımlama kaydı").
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
- Adım görünümünün kendisi, bu güncelleme sırasında kurulu bir sunucuda ilk kez
  gözlenir.
- P0.1-P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez;
  yayımlanma kabul olmaz.
- Geri kalan her şey (tasarım, beş adım, testler, diğer sınırlar)
  [v0.1.0-alpha.88 notlarında](RELEASE-NOTES-v0.1.0-alpha.88.tr.md).

Bu sürümün yayımlanması hiçbir kurulu sunucuyu güncellemez; her güncellemeyi
sahibi, panelin kendi güncelleme ekranından başlatır.

## Yayımlama kaydı

Yayımlanmadı.
