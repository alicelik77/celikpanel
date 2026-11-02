# v0.1.0-alpha.115

[English](RELEASE-NOTES-v0.1.0-alpha.115.md)

v0.1.0-alpha.114'ten sonra, sahibin ilk sunucuyu yeniden kurarken karşılaştığı
sorun için küçük bir düzeltme sürümü. **Kurulu panelleri yalnız sahibi, panelin
kendi güncelleme ekranından günceller.**

## Düzeltmeler

- **Sunucu kurulumu: ana alan adını değiştirmek.** Bir alan adıyla mevcut
  kayıtlar adımına gelip geri dönerek başka bir alan adı yazınca taslak ilk
  alan adının içe aktarılan kayıtlarını tutuyordu; sunucu taslağı reddediyor,
  sihirbaz "bağlantıyı kontrol edin" diyordu. Artık ana alan adı değişince eski
  alan adının kayıt cevapları ve onun için önerilen mail2 adı düşülür; kayıtlar
  adımı yeni alan adının kendi kayıtlarını okur.
- **Sunucu kurulumu: reddedilen cevap nedenini söyler.** Sunucu bir taslağı
  kabul etmezse sihirbaz bağlantı metni yerine sunucunun nedenini gösterir.
- **Panel sekme simgesi.** Tarayıcı sekmesinde Vite'ın başlangıç simgesi
  görünüyordu; artık CelikPanel işareti görünür.
- Arka plandaki DKIM işiyle aynı anda veritabanı kilidine takılabilen bir test
  artık o işi başlatmıyor.

## Kapsanmayanlar

Bu sürüm için yalnız otomatik kontroller koşuldu (web, Go, sürüm ve dağıtım
kontrolleri); sahibin tercihiyle yeni bir gerçek sunucu denemesi yapılmadı.
alpha.114 notlarında denenmedi olarak yazılanlar denenmemiş kalır.
