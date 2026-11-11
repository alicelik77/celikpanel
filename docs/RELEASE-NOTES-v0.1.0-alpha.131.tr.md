# v0.1.0-alpha.131

[English](RELEASE-NOTES-v0.1.0-alpha.131.md)

Tek bir düzeltme içeren küçük bir sürüm. Sahip Boston'da PostgreSQL'i 5433
portundan 5432'ye taşıdı ve veritabanına dışarıdan bağlanma çalışmaz oldu.
Güvenli tarafta kaldı: hiçbir şey dışarıya açılmadı, ama hiçbir müşteri
dışarıdan bağlanamıyordu. **Kurulu panelleri yalnız sahip, panelin kendi
güncelleme ekranından günceller.**

## Neden durdu

CelikPanel'in dışarıdan erişim için kurduklarına dair kaydı eski porta bağlı
kaldı. Yeni portta CelikPanel'in kendi ayarları (ağda dinleme,
`auth_delay`) sahibinkiler gibi göründü; bu yüzden her kural reddedildi ve
güvenlik duvarı süzgecinde eski porta ait bir giriş kaldı.

## Ne değişti

- **Kayıt kümeyi izliyor.** Bir PostgreSQL girdisi artık kümenin veri
  klasörünü de taşıyor (kayıt sürümü 2). Sürüm 1 kaydı okunurken
  dönüştürülüyor ve ilk uygulama baştan sona yapılıyor.
- **Her uygulama, portu kümesini kaybetmiş bir kaydı onarıyor:** veri
  klasöründen ya da eski bir kayıtta CelikPanel'in kendi ayarlarının tam
  olarak tek bir kümede bulunmasından. Hangi kümenin kastedildiği açık
  değilse hiçbir şey açılmıyor.
- **Port değişikliği kaydı ve süzgeci birlikte taşıyor** (kümenin eski
  Fail2ban kuralını da); ardından panel tam bir uygulama istiyor.
- **Sunucu anahtarı açıkken müşteri adresleri için yeniden başlatma yok.**
  Son adres kaldırıldığında motor TLS ve `auth_delay` ile ağda dinlemeyi
  sürdürüyor (MariaDB seçenek dosyasını koruyor); böylece sonraki müşteri
  için yeniden başlatma gerekmiyor. Bu ayarları yalnız sunucu anahtarını
  kapatmak geri alıyor.
- **MariaDB etkilenmiyor** (panel MariaDB için port değişikliği sunmuyor).
- **Müşteri kartı:** satırlar düzgün yerleşiyor; yeni bitiş tarihi yalnız
  açıkça Uygula'ya basılınca kaydediliyor (Vazgeç ile bırakılıyor); istemci
  ayarlarında posta sunucusunun adı yerine sunucunun kendi adı görünüyor;
  sahibin ayarı sakin bir dille anlatılıyor; aralıklar normal; Veritabanları
  listesindeki elle yenile düğmesi kaldırıldı (liste kendiliğinden
  yenileniyor).

## Laboratuvar sonuçları

Laboratuvar (QEMU; gerçek sunucu yok): Boston'daki sorun Ubuntu 24.04
üzerinde 130 ile yeniden üretildi. 131'e güncellemeden sonra dışarıdan
erişim, yeniden başlatma olmadan 404 saniyede kendiliğinden düzeldi.
Şifreli bağlantı çalıştı, şifresiz bağlantı reddedildi. Son adres
kaldırıldığında motor dinlemeyi sürdürdü; adresi yeniden eklemek 1 saniye
içinde çalıştı. 5432'den 5434'e port değişikliği 6 saniyede izlendi.

Kanıt: laboratuvar dalı `lab/alpha114-acceptance` (commit `e52d8c641`)
üzerinde [`deploy/e2e/dns-pair-acceptance/evidence/a131/`](https://github.com/alicelik77/celikpanel/tree/e52d8c641/deploy/e2e/dns-pair-acceptance/evidence/a131).

## Bilinen sınırlar

- 130'da port değişikliğinden sonra `hostssl` satırı `pg_hba.conf` içinde
  kalıyordu; 131 bunu onarıyor.
- Yönetici kartı, listede hiç adres yokken de motoru "açık" gösteriyor.
- Daha büyük yeniden tasarım (sunucu anahtarı açılırken her şeyin tek bir
  onaylı yeniden başlatmayla hazırlanması) ve her ekranın yalnız açık bir
  eylemle kaydetmesi kuralı alpha.132'de geliyor.
- Önceki sınırlar geçerliliğini koruyor (bkz. alpha.130).

## Kanıt

Geçiş yok (şema 77'de kalıyor). Ajan'ın diskteki kaydı sürüm 1'den sürüm
2'ye geçiyor. 130 Ajanı sürüm 2 kaydını reddediyor ve hiçbir şeyi
değiştirmiyor (güvenli tarafta kalıyor); bu yüzden 130'a geri dönüldüğünde
dışarıdan erişim, 131 yeniden kurulana dek kapalı kalıyor. Sürüm sırası
130 → 131.
