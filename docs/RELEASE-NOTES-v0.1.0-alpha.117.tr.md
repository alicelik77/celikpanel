# v0.1.0-alpha.117

[English](RELEASE-NOTES-v0.1.0-alpha.117.md)

> **Yayımlandı**, 3 Kasım 2026 (UTC): sürüm commit'i `b22348ff8`, GitHub ön sürümü 02:08:57Z, indirme portalı yaklaşık 02:13Z (dış denetim 27/27).

Sahibin 2 Kasım 2026'da onayladığı planlar ve haklar (D-042) ile güvenlik acil
durumu (D-043) sürümü. **Kurulu panelleri yalnız sahibi, panelin kendi
güncelleme ekranından günceller.**

## Planlar, haklar ve hareket alanı (D-042)

- **Kurulu olmak izin vermez.** Bir hesabın kullanabildiği; kurulu olan,
  yöneticinin sunduğu, bayinin hakları ve hesabın haklarının kesişimidir. Bayi
  yalnız daraltır.
- **Her hesabın durumu adıyla görünür:** Sınırsız, Plan: <ad> ya da Özel. Plana
  bağlı hesapta tek bir şey değişince hesap Özel olur (önce sorulur). Hazır iki
  plan, Sınırsız ve Sınırsız bayi, sunulan her yeni şeyi alır.
- **Hakların üç bölümü var:** miktarlar (alan adı, veritabanı, posta kutusu,
  disk), yetkiler (DNS düzenleme, DNSSEC, posta, veritabanı, zamanlanmış
  görevler, dosya yöneticisi, yedek, ücretsiz ve kendi sertifikası, korumalı
  klasör ve yönlendirme, vekil site, uygulama kurucu, VPN, ekip üyeleri, dosya
  erişimi, SSH) ve yazılım (PHP ve Node.js sürümleri, veritabanı motorları, web
  postası, sertifika sağlayıcıları). Sunucu zorlar; ekranlar hesabın
  kullanamadığını gizler.
- **Bayi havuzu gerçek kullanımı sınırlar**, dağıtılan toplamı değil; yüzde 80
  ve 100'de uyarı.
- **Hesap oluştururken** Haklar bölümü her zaman görünür (adıyla seçili
  Sınırsız, bir plan ya da özel). Daraltan plan düzenlemesi önce etkilenen
  hesapları gösterir.
- **Hak kaldırılınca çalışan kalır:** var olan siteler, posta kutuları ve
  veritabanları çalışır; o türden yenisi oluşturulamaz.

## Abonelikler

- **Askıya alma.** Yönetici ya da bayi bir aboneliği askıya alınca ya da bitiş
  tarihi geçince: siteler "Bu site askıya alınmıştır" gösterir, posta geçici
  olarak reddedilir (450, gönderen yeniden dener); posta kutusu girişleri,
  veritabanı kullanıcıları, VPN cihazları, zamanlanmış görevler, Node.js
  uygulamaları ve dosya erişimi durur. Hiçbir şey silinmez; açılınca aynen geri
  gelir. Bayi askıya alınınca müşterileri de alınır. Askıdakiler Silinebilir
  listesinde görünür; silme elle yapılır.
- **Bitiş tarihi** girilebilir ya da boşaltılabilir (boş = bitmez); ileride
  gelecek ödeme sistemine hazırdır.
- **Devir.** Yönetici her aboneliği devreder; bayi kendi müşterileri arasında;
  müşteri istek gönderir, bayisi ya da yönetici onaylar. Alan tarafın hakları ve
  havuzu önce denetlenir. "Tüm şifreleri ve anahtarları değiştir" varsayılan
  olarak açıktır: site hesapları, SSH anahtarları, posta kutusu ve veritabanı
  şifreleri (panelin kurduğu WordPress güncellenir), korumalı klasör
  kullanıcıları ve VPN cihazları.

## Dosya erişimi

- Site başına: TLS üzerinden FTP ve SFTP (tek yetki) ile SSH (ayrı yetki),
  sitenin kendi klasörüne kapatılmış, az araçlı bir ortamda. Düz FTP hiç
  sunulmaz. Yeni site kullanıcılarının giriş kabuğu yoktur.

## Güvenlik acil durumu (D-043)

- Bir PHP ya da Node.js sürümü veya veritabanı motoru için: geri çek (yeni
  kullanım yok), acil durdur (hizmet kilitlenir, siteler bakım sayfasında),
  başka sürüme toplu taşı, acil kaldır. Güçlü onay (etkilenen liste, neden, adı
  elle yazma, parola). Etkilenen müşteri ve bayilere panel bildirimi ve tek
  tıkla geçişli e-posta gider. Müşteri verisi asla silinmez. Yeniden açma geri
  getirir.

## Diğer düzeltmeler

- Kurulum Postfix ve Dovecot'u ilk andan güvenilir posta sertifikasıyla açar;
  paketler kurulum sırasında kendi varsayılanlarıyla başlamaz (FTP sunucusu
  dahil).
- Alan adı henüz bu sunucuyu göstermiyorsa "Sertifika al" nedeni ve çözümüyle
  kapalıdır; "Cloudflare gibi bir vekilin arkasında — yine de iste" seçeneği
  vardır ve denetim kaydına yazılır.

## Bilinen sınırlar

- Trafik ölçülmez, disk işletim sistemiyle sınırlanmaz; sınırlar yeni
  oluşturmada işler.
- alpha.116'ya geri dönüş, güncellemeden önce alınan veritabanı kopyasını geri
  koyar: güncellemeden sonra panelde yapılanlar kaybolur; o sürede sunucuda
  uygulanmış bir askıya alma ya da acil durum geri dönüşle geri alınmaz.

- SSH üzerinden etkileşimli giriş olmadan gönderilen bir komut (örneğin
  `ssh sunucu 'pwd'`) kapalı ortamın listelenemeyen kökünde başlar;
  etkileşimli giriş sitenin kendi klasöründe başlar.

## Kanıt

Linux Go testleri (82 paket; deneme ortamına özgü bilinen Agent hataları
main'de de var), 2305 web testi, schema 56'ya genişletilmiş release-recovery
fikstürleri ve yayın kaydına yazılan bir laboratuvar denemesi.
