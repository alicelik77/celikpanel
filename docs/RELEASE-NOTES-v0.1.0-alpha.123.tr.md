# v0.1.0-alpha.123

[English](RELEASE-NOTES-v0.1.0-alpha.123.md)

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
- PowerDNS yalnız denendiği sistemlerde sunulur: Debian 13, Ubuntu 24.04
  ve Arch Linux; hem ana hem yedek DNS sunucusu olarak. Diğer sistemlerde
  kurulum, kodla katılma ve DNS sayfası bunu söyler ve BIND kullanılır;
  sunucuda hiçbir şey değiştirilmez.

## Sunucu kurulumu

- Panelin kendi veritabanı kısa bir süre meşgul olduğunda kurulum adımı artık
  hata bildirmek yerine bekler ve kendiliğinden yeniden dener. Hiçbir şey iki
  kez başlatılmaz. Veritabanı yaklaşık on dakika meşgul kalırsa kurulum o
  adımı durdurur ve sahibe çalıştıracağı tek komutu ve nasıl devam edeceğini
  söyler; tamamlanan adımlar korunur.

## Bilinen sınırlar

- Kurulum penceresi hâlâ kurulacak paketleri listeler. Bunun yerine
  bileşenleri gösteren görünüm alpha.124 için planlandı.
- Bir bileşeni kurmak, ihtiyaç duyduklarını henüz aynı işlemde kurmuyor;
  pencere her biri için **Önce … kurulsun** seçeneğini sunar.
- Debian 12 ve Ubuntu 22.04: bu düzeltmeler alpha.121 üzerindeki
  laboratuvar turundan geliyor; ancak bu notlar yazılırken bu sürümle iki
  sistemde de baştan sona kurulum ve sunucu kurulumu henüz yinelenmemişti.
  Sıradaki iş o laboratuvar turu.

## Kanıt

Şema değişmedi, geçiş yok. Sürüm sırası 122 → 123.
Go testleri (bütün modül) ve web testleri geçiyor; web paketleri bütçeleri
içinde. Yeni Debian 12, Ubuntu 22.04 ve olağan sistemlerdeki laboratuvar turu
henüz bekliyor.
