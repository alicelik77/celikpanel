# v0.1.0-alpha.122

[English](RELEASE-NOTES-v0.1.0-alpha.122.md)

> **Yayımlandı**, 4 Kasım 2026 (UTC): sürüm commit'i `f4f6252ff`, GitHub ön sürümü 20:54:41Z, indirme portalı yaklaşık 20:58Z (dış denetim 27/27).

Sahibin Boston'daki (alpha.121) sorusundan sonra tek Bileşenler listesiyle
küçük bir sürüm: "phpPgAdmin ve phpMyAdmin neden ayrı ayrı kurulamıyor?"
**Kurulu panelleri yalnız sahibi, panelin kendi güncelleme ekranından
günceller.**

## Bileşenler

- **Kurulu** ve **Katalog** görünümlerinin yerine tek liste. Sunucunun sunduğu
  her bileşen kendi bölümünde, kurulu olanlar önce gösterilir; ilk ziyarette
  her bölüm açıktır. **Tümünü aç**, **Tümünü kapat** ve bölüm başlıkları
  listeyi yine katlar; tarayıcı neyin katlandığını hatırlar. Arama her bileşeni
  bulur.
- Kurulu olmayan bileşen **Kurulu değil** yazar ve **Kur** düğmesi kendi kurulum
  penceresini açar. phpPgAdmin, phpMyAdmin, Fail2ban, ClamAV, vsftpd, Redis,
  Valkey, Memcached, Netdata, Node.js, Go ve Rust artık gizli değil.
- Burada kurulamayan bileşen nedenini tek satırda söyler: Apache'nin yanında
  **Nginx ile çakışır** (Postfix'in yanında Exim, BIND'in yanında PowerDNS) ya
  da bu sistemde sunulmuyor. Postfix, Dovecot, Rspamd ve Roundcube e-posta
  bölümünün başındaki posta kurulumuna, DNS motorları DNS sayfasına yönlendirir.
- Parçalarından bazıları kurulu olan hazır paket neyi ekleyeceğini adıyla
  söyler, örneğin **PostgreSQL + phpPgAdmin · Ekle: phpPgAdmin**. Hiçbir parçası
  kurulu olmayan paket yine **Kur** der.
- Başka bir sayfa bir bileşenin eksik olduğunu söylediğinde (dosya erişimi:
  vsftpd ve Fail2ban; virüs taraması: ClamAV; yapay zekâ uygulamaları: Node.js;
  Python uygulamaları: Python; VPN: WireGuard; Güvenlik: Fail2ban), yönetici o
  bileşenin kurulum penceresini açan bir **Kur** bağlantısı görür. Onaylanmadan
  hiçbir şey başlamaz.

## Bilinen sınırlar

- Site oluşturma ve barındırma türü ekranları ile WordPress araçları hâlâ
  doğrudan bağlantı olmadan "Bileşenler'den kurun" der.
- Python kurulu ama `venv` modülü eksikse, Python bağlantısı kurulum açmak
  yerine Python'un kurulu olduğunu söyleyebilir.

## Kanıt

Şema değişmedi. Web testleri paket bütçeleri içinde geçiyor; sürüm sırası
121 → 122.
Yalnız ekran sürümü: Agent ya da Panel sunucu tarafında değişiklik yok, bu yüzden laboratuvar turu yapılmadı; otomatik denetimlerle denetlendi. Bu denetimler de GitHub'ın `/run` bağlama bayraklarından bağımsız yapıldı: `/run` altına çalıştırılabilir dosya koyan üç sözleşme testi artık kendi özel tmpfs'ini kullanıyor; bu durum alpha.121'in paket derlemesini bir kez, alpha.122'nin denetimlerini bir kez durdurmuştu.
