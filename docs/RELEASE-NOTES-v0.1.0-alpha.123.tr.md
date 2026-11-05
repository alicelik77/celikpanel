# v0.1.0-alpha.123

[English](RELEASE-NOTES-v0.1.0-alpha.123.md)

> **Yayımlandı**, 5 Kasım 2026 (UTC): sürüm commit'i `d7b5f1ab5`, GitHub ön sürümü 07:19:47Z, indirme portalı yaklaşık 07:50Z (dış denetim 27/27).

Bir bileşenin neye ihtiyaç duyduğu için tek kural, önce kaldırmaya
uygulandı; ayrıca Debian 12 ve Ubuntu 22.04'te kurulum düzeltmeleri.
**Kurulu panelleri yalnız sahibi, panelin kendi güncelleme ekranından
günceller.**

## Bileşen kaldırma

- Kaldırma tek bir soru sorar: bu bileşene ne ihtiyaç duyuyor? Kaldırma
  penceresi onu kullanan her bileşeni, siteyi, veritabanını, posta kutusunu,
  uygulamayı, sertifikayı, VPN eşini ve karantinadaki dosyayı adıyla sayar;
  her birinin, taşınacağı ya da kaldırılacağı yere bağlantısı vardır. Bu
  liste boşalana dek kaldırma reddedilir. "Zorla" seçeneği yoktur; müşteri
  verisi bir bileşenle birlikte asla kaldırılmaz.
- Başka hiçbir şeyin kullanmadığı ve onsuz çalışamayan küçük bileşenler
  (örneğin PostgreSQL ile phpPgAdmin, posta sunucusuyla Roundcube)
  **Birlikte kaldırılacaklar** altında işaretli gelir. Sunucu önce onları,
  sonra bileşeni tek işlemde kaldırır. Sayfa kapansa ya da panel yeniden
  başlasa da işlem sürer; bu sırada açılan sayfa bağlantı koptu uyarısı
  yerine aynı ilerlemeyi gösterir. İşaret kaldırılırsa bileşen kaldırılmaz.
- Ona hiçbir şey ihtiyaç duymuyorsa pencere neyin duracağını ve neyin
  çalışmaya devam edeceğini söyler (örneğin Roundcube gidince web postası
  olmaz, posta yine teslim edilir).
- n8n gibi bir uygulamanın üzerinde çalıştığı Node.js sürümü kaldırılmaz;
  pencere uygulamanın adını verir. Kaldır'a basıldığında sunucu yeniden
  denetler.
- Kaldırılan bileşeni ya da sürümü sunan planlar, bayi sınırları ve hesaba
  özel haklar o kaydı kendiliğinden kaybeder.
- Güvenlik acil durumu hızını korur, ama kaldırma adımından önce aynı
  listeyi gösterir.
- Bileşenler sayfasındaki **Birlikte kaldır** işlemi artık "CelikPanel
  güncellenmeden önce açılmış" gerekçesiyle reddedilmiyor: sayfa artık
  sunucunun beklediği istek kimliğini gönderiyor.
- Gerektirdiği bileşen eksik olan kurulu bir bileşen bunu kendi satırında
  söyler: **Fail2ban** eksik **Güvenlik duvarı motoru**nu adıyla verir ve
  onun **Kur** düğmesini gösterir. Önce başka bileşenlere ihtiyaç duyan bir
  bileşen, kurulum penceresinde **Önce … kurulsun** seçeneğini sunar.

## Debian 12 ve Ubuntu 22.04

- Ubuntu 22.04'te kurulum artık sistem yöneticisi denetiminde durmuyor:
  oradaki eski systemd bir soketi daha geniş dosya izinleriyle oluşturur;
  systemd'nin kendisi o soketi yine yalnız root kullanıcısına açar.
- BIND, hizmet dosyasının hâlâ `/lib/systemd/system` altında bulunduğu
  Debian 12 ve Ubuntu 22.04'te tanınıyor.
- BIND, Ubuntu 22.04'te de başlıyor: oradaki BIND hizmet dosyası ad
  sunucusunu arka planda (eski yöntemle) çalıştırır; orada yalnız o dosyanın
  birebir kendisi kabul edilir.
- PowerDNS yalnız denendiği yerde sunulur: ana DNS sunucusu olarak Debian 13
  ve Arch Linux'ta, yedek DNS sunucusu olarak ayrıca Ubuntu 24.04'te
  (alpha.122'den beri aynı: Ubuntu 24.04'te ana DNS sunucusu BIND kullanır).
  Diğer sistemlerde kurulum, kodla katılma ve DNS sayfası bunu söyler ve
  BIND kullanılır;
  sunucuda hiçbir şey değiştirilmez.

## Sunucu kurulumu

- Panelin kendi veritabanı kısa bir süre meşgul olduğunda kurulum adımı artık
  hata bildirmek yerine bekler ve kendiliğinden yeniden dener. Hiçbir şey iki
  kez başlatılmaz. Veritabanı yaklaşık on dakika meşgul kalırsa kurulum o
  adımı durdurur ve sahibe çalıştıracağı tek komutu ve nasıl devam edeceğini
  söyler; tamamlanan adımlar korunur.
- Bileşenler sayfasından bileşen kurarken de panelin veritabanı kısa süre
  meşgulse işlem hata vermek yerine bekler.

## Laboratuvar sonuçları

5 Kasım 2026 laboratuvar turu: sahibin `get.sh` yoluyla kurulan yeni Debian
12, Ubuntu 22.04, Debian 13 ve Ubuntu 24.04 makinelerinde sunucu kurulumu
dördünde de tamamlandı. DNS sunucusu Debian 12, Ubuntu 22.04 ve Ubuntu
24.04'te BIND, Debian 13'te PowerDNS oldu. Ubuntu 24.04 PowerDNS'i yalnız
yedek DNS sunucusu olarak sunar; alpha.122'den beri böyle. Tur şunları buldu:
Ubuntu 22.04'te BIND'in başlaması, Bileşenler sayfasında birlikte kaldırma
reddi ve bileşen kurulumunda meşgul veritabanı beklemesi; üçü de bu sürümde
düzeltildi.

Ölçülen site yalıtımı düzeyi: Debian 13'te tam; Debian 12, Ubuntu 22.04 ve
Ubuntu 24.04'te azaltılmış (bu sistemlerin systemd'sinde tam düzeyin
gerektirdiği BPF desteği yok). Ubuntu 22.04, Ubuntu 24.04'ten zayıf değil.

## Bilinen sınırlar

- Kurulum penceresi hâlâ kurulacak paketleri listeler. Bunun yerine
  bileşenleri gösteren görünüm alpha.124 için planlandı.
- Bir bileşeni kurmak, ihtiyaç duyduklarını henüz aynı işlemde kurmuyor;
  pencere her biri için **Önce … kurulsun** seçeneğini sunar.
- Bileşenler sayfası bu turda tarayıcıda denenmedi.
- Panel yeniden başlatıldıktan sonra kaldırma işlemine yeniden bağlanma
  kaldırma sürerken gözlenmedi: açılış kaldırmanın kendisinden uzun sürdü ve
  kaldırma tamamlandı.
- Bir PHP sürümünü kaldırma reddedilir ve siteleri adıyla söyler, ama ret
  metni phpMyAdmin'i saymaz; kaldırma penceresi sayar.
- PostgreSQL 18 kurulum önizlemesi PGDG deposunu adıyla değil "vendor" olarak
  etiketler.
- Panel veritabanı kesintisiz yaklaşık 5 saniyeden uzun kilitli kalırsa bir
  bileşen işleminin ekranı "bekliyor" yerine "çalışıyor" durumunda kalabilir;
  bu laboratuvar durumunda bir denetim kaydı da kayboldu. Düzeltmeler
  alpha.124 için planlandı.

## Kanıt

Şema değişmedi, geçiş yok. Sürüm sırası 122 → 123.
Go testleri (bütün modül) ve web testleri geçiyor; web paketleri bütçeleri
içinde. 5 Kasım 2026 laboratuvar turu yukarıda anlatılmıştır.
