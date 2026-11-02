# v0.1.0-alpha.116

[English](RELEASE-NOTES-v0.1.0-alpha.116.md)

v0.1.0-alpha.115'ten sonra bir düzeltme sürümü: bir güvenlik düzeltmesi ve
sahibin yeniden kurulan sunucuları kullanırken bulduğu sorunlar. **Kurulu
panelleri yalnız sahibi, panelin kendi güncelleme ekranından günceller.**

## Güvenlik

- **Vekil (proxy) siteler artık sunucunun içini gösteremez.** Vekil türüne
  çevrilen bir site hedef olarak 127.0.0.1, iç ağ adresleri, bulut
  sağlayıcının özel adresi ya da sunucunun kendi adresleri dahil her adresi
  verebiliyor, böylece yalnız içeriye açık hizmetleri bir müşterinin alan
  adından dışarı açabiliyordu. Böyle bir hedef artık nedeni söylenerek
  reddedilir (`PROXY_TARGET_NOT_ALLOWED`); adlar çözülür ve her adres
  denetlenir. Var olan vekil sitelere dokunulmaz. Bilinen sınır: adresi
  kayıttan sonra iç adrese çevrilen bir ad yakalanmaz (nginx onu sonradan
  yeniden çözer).

## Düzeltmeler

- **Kurulum desteklenmeyen sistemi en başta reddeder.** Kali, Linux Mint,
  eski Debian/Ubuntu sürümleri, openSUSE/SLES ve Red Hat ailesi hiçbir şey
  değiştirilmeden, sistemi adlandıran ve desteklenen listeyi (Debian 12/13,
  Ubuntu 22.04/24.04, Arch Linux) veren iki dilli bir mesajla reddedilir.
- **Kurulum posta sertifikasını postadan önce alır.** Postası olmayan bir
  sunucuda kurulum önce güvenilir posta sertifikasını alır; Postfix ve Dovecot
  onunla başlar. Postası zaten olan sunucular eski sırayı korur.
- **Kurulumda yanıltıcı sertifika uyarısı yok.** "Kendi imzalı yedek
  sertifika" uyarısı kurulum adımlarında artık gösterilmez; kurulum dışında
  metni yalnız kurulumun kanıtladığını söyler.
- **Bayi ve müşteri için VPN sayfası.** Menüde VPN yalnız o hesaba sunuluyor ve
  WireGuard kuruluysa görünür; değilse tek bir düz cümle. Çelişkili "dikkat
  istiyor / 0 bekleyen, 0 hatalı" kutusu ve yöneticiye ait ayrıntılar
  onlardan kaldırıldı.
- **Kota açıkları kapandı.** Alan adı sayfasından veritabanı oluşturmak artık
  plana sayılır; cPanel içe aktarması posta kutusu ya da veritabanı sınırını
  aşamaz; bir plan mevcut kullanımın altına düşürülürken etkilenen hesaplar
  gösterilir ve onay istenir (hiçbir şey silinmez).

## Kapsanmayanlar

Otomatik kontroller (web, Go, sürüm ve dağıtım) ve postalı yeni bir Debian 13
sunucusunun bir kısa laboratuvar kurulumu. Başka gerçek sunucu denemesi yok.
