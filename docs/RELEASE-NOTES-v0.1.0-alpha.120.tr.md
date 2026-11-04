# v0.1.0-alpha.120

[English](RELEASE-NOTES-v0.1.0-alpha.120.md)

Sahibin Boston ve Frankfurt'u alpha.119'a güncelledikten sonra istediği
Bileşenler düzeltmeleriyle küçük bir sürüm. **Kurulu panelleri yalnız sahibi,
panelin kendi güncelleme ekranından günceller.**

## Bileşenler

- Henüz kurulu olmayan çekirdek sunucu programının (Nginx, PHP-FPM, MariaDB,
  PostgreSQL) artık kuruluların gibi **Kur** düğmeli kendi satırı var. Kurulum
  penceresi sürümleri gösterir. Küçük "ya da tek başına kurun" bağlantısı
  kalktı; hazır paketler duruyor.
- Güvenlik acil durumu adımları listeden çıktı. Artık her bileşenin kendi
  sayfasında (PHP, Node.js, MariaDB, PostgreSQL) ve Güvenlik sayfasındaki tek bir
  "Güvenlik acil durumu" bölümünde. Bu bölüm neden yalnız bu dördü olduğunu
  söyler: müşteri sitelerinin doğrudan üzerinde çalıştığı programlar bunlar.
  Satırda kırmızı "Güvenlik acil durumu açık" rozeti yalnız bir acil durum
  açıkken görünür.
- Python, işletim sisteminin verdiği sürümü gösterir.
- Sayfanın adresi artık `/components`. Eski `/services` bağlantıları ve yer
  imleri çalışmaya devam eder: aynı yeri açar.

## Bilinen sınırlar

- Önceki sürümlerin kaydettiği Mağaza öğeleri hâlâ `/services/…` adresine
  bağlanır; yönlendirmeyle çalışmaya devam eder.
- Posta ve DNS motorları "kurulu değil" satırı olarak listelenmez; bunlar posta
  kurulumunda ve DNS altyapısında seçilir.

## Kanıt

Panel için Go testleri geçiyor; 2432 web testi paket bütçeleri içinde geçiyor;
sürüm sırası 119 → 120. <!-- LAB -->
