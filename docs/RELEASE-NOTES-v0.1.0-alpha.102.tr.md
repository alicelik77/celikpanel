# v0.1.0-alpha.102 (aday, yayımlanmadı)

[English](RELEASE-NOTES-v0.1.0-alpha.102.md)

Bu, v0.1.0-alpha.101'den sonraki sürüm adayıdır. Yayımlanmadı; hiçbir parçası
kurulu bir sunucuda değil. 23 Ekim 2026'da, alpha.101'in yayım kaydından sonraki
`main` üzerinden açılan `fix/alpha102-install-email` dalında yazıldı.

## Kim güncellemeli ve neden

Yayımlandıktan sonra CelikPanel'i yeni bir sunucuya kuran herkes. Mevcut bir
kurulumda görünür bir değişiklik yoktur: değişiklik kurucunun ilk yönetici
adımındadır.

alpha.99'dan beri açık bir iş, yöneticinin geçerli bir e-posta adresi yokken
Sunucu kurulumunun hiçbir planı başlatmadığını söylüyordu. Sahip 23 Ekim 2026'da
bunun böyle kalmasına karar verdi: "iletişim epostası zorunlu olarak gerekiyor ise
zorunlu olmalı. kurulumda eposta belirtiyoruz". Gerçekten gerekiyor: ilk kurulum
her zaman panelin HTTPS sertifikasını ister ve sertifika yetkilisi yöneticinin
adresini iletişim adresi olarak ister. Sertifika adımı olmayan tek plan zaten
geçerli sertifikası olan bir sunucunun planıdır; o durumda adres zaten kayıtlıdır.
Ardından sahip adresin ilk yazıldığı yerde denetlenmesini onayladı ("ok. yap");
böylece `ali` ya da `ali@` gibi bir yazım hatası Sunucu kurulumu ekranında
sonradan değil, kurulum sırasında yakalanır (D-021 değişikliği).

Bir sunucuyu yalnız sahibi, panelin kendi güncelleme ekranından günceller.

## Kurulumda ne değişir

**Yöneticinin e-postası sertifika isteğinin kuralıyla denetlenir.** Kurucu,
kurulum planının, panel sertifikası isteğinin ve Hesabım'daki kendi e-posta
değişikliğinin kullandığı denetimi kullanır: yazdırılabilir ASCII, tam bir `@`,
en çok 64 karakterlik yerel bölüm, tam bir alan adı (`localhost` değil, IP adresi
değil, sonda nokta yok), toplam en çok 254 karakter. Örneğin `ali`, `ali@`,
`@x.com`, `a b@x.com` ve `ali@localhost` reddedilir; `siz@ornek.com` kabul edilir.

- **İstemde** ("Admin email / Yönetici e-posta:") reddedilen adres şu satırı yazar

  > That is not a valid email address (example: you@example.com). The certificate
  > authority needs it as a contact. Please try again. / Bu geçerli bir e-posta
  > adresi değil (örnek: siz@ornek.com). Sertifika yetkilisi bunu iletişim adresi
  > olarak ister. Lütfen yeniden deneyin.

  ve aynı soru yeniden sorulur. Yazılan değer yazdırılmaz. Boş yanıtın kendi
  satırı kalır ("E-posta boş olamaz…"). İstem yine paketler kurulduktan sonra
  gelir; durmak yerine yeniden sorduğu için bir yazım hatası kurulumu durdurmaz.
- **Kimlik bilgisi dosyasıyla** (`CELIKPANEL_ADMIN_CREDENTIALS_FILE`) kurucu,
  önceden olduğu gibi hiçbir şey kurmadan dosyayı indirilen panelle denetler; bu
  denetim artık böyle bir adresi reddeder. Kurucu İngilizce satırı ve şu Türkçe
  satırla durur:

  > Yönetici kimlik bilgileri geçersiz; girdi dosyasına dokunulmadı ve hiçbir şey
  > kurulmadı. Dosyayı düzeltip kurucuyu yeniden çalıştırın: panelin kabul ettiği
  > bir kullanıcı adı, siz@ornek.com gibi gerçek bir e-posta adresi (sertifika
  > yetkilisi bunu iletişim adresi olarak ister) ve en az 8 karakterlik bir parola

  Dosyayı düzelttikten sonra aynı komutu yeniden çalıştırın. Panelin kendi reddi
  (panel programı elle çalıştırıldığında görülür) e-posta alanını adlandırır,
  hiçbir değeri yazmaz.
- **Adres, kendi e-posta değişikliğinin sakladığı gibi saklanır:** alan adı küçük
  harfle (`Owner@Example.COM`, `Owner@example.com` olarak saklanır).
- **Yönetici oluşturma komutunun mevcut bir kullanıcı adı için yeniden
  çalıştırılması** da aynı kurala bağlıdır; reddedilen adres ne adresi ne parolayı
  değiştirir.

## Değişmeyenler

- **Mevcut kurulumlar.** Göç yok; saklanan adresler yeniden denetlenmez ya da
  değiştirilmez. Sertifika isteğinin reddedeceği eski bir adresi yine Sunucu
  kurulumunun iletişim engeli yakalar; engelin "Hesabım’ı aç" düğmesi kendi
  e-posta değişikliğine götürür.
- **Sunucu kurulumunun iletişim engeli** her planda kalır.
- **Sertifika yetkilisinin kendi kararı.** Panel adresin biçimini denetler; yetkili
  genel olmayan bir alan adındaki adresi (`.test`, `.invalid`, `example.com`) yine
  reddedebilir.
- **Ekip üyeleri ve diğer hesaplar.** Başka hiçbir yol yönetici oluşturmaz: hesap
  ekranları yönetici hesabı oluşturmayı ve yönetmeyi reddeder, ekip üyeleri Sunucu
  kurulumunu açamayan müşteri rollü ek kullanıcılardır ve kontrol düzlemi geri
  yüklemesi arşivdeki yöneticileri değiştirmeden getirir. Onların adres
  denetimleri değişmez.
- **İmzalı giriş betiği** (`get.sh`) kendi daha dar biçim denetimini korur; yalnız
  sürüm sabitleri değişir. Yetki indirilen panelin denetimindedir.
- Arayüz, roller, Agent, güncelleyici, sürüm kurtarma, şema (44, göç yok) ve
  sorgulama. Sahibin 16 Ekim 2026 talimatıyla gerçek sistem kaydı yapılmaz
  (güncelleyicinin değişiklik yolu değişmedikçe gerçek sistem kaydı yok).

## Bilinen açıklar

- Kurulu bir sunucuda ya da gerçek bir ilk kurulumda hiçbir şey ölçülmedi.
- **Yaklaşan kabul, iddia değil:** Beta 1'de sahip temiz ilk kurulum denemesi için
  iki sunucuyu da silip yeniden kuracak; bu denetim gerçek bir kurulumda ilk kez
  orada görülecek.

## Kanıt

Aday ağaçta:

- Go: istem, sertifika iletişim doğrulayıcısının kabul ettiğini tam olarak kabul
  eder, diğerlerinde yeniden sorar (`ali`, `ali@`, `@x.com`, `a b@x.com`,
  `ali@localhost`, bir IP adresi, görünen ad; kabul edilenler kanonik biçimde
  döner); katı kimlik bilgisi ayrıştırıcısı aynı adresleri ve boşluklu bir adresi
  reddeder; oluşturma yolu veritabanında hiçbir değişiklik yapmadan reddeder,
  kanonik biçimi saklar ve reddedilen adreste mevcut yöneticinin adresini ve
  parolasını değiştirmez; yalnız-doğrulama kipi e-posta alanını adlandıran, değer
  içermeyen bir hatayla çıkar; kimlik bilgisi dosyasıyla oluşturma yolu hiçbir
  kullanıcı yazmaz. Son ağaçta `CelikPanel-S2-Debian` konuğunda geçti (Panel ve iç paketler, 45 dakika sınırı, CGO kapalı): 64 pakette 7.219 geçti, 44 atlandı, 0 başarısız (2.921 üst düzey test); gofmt ve go vet temiz.
- Kabuk: `archlinux` konuğunda taze bir klonda sürüm sırası politikası ("101 -> 102; historical bootstrap sequence 101"), indirme portalı, imzalı sürüm manifesti, önyükleme güncellemesi, ilk kurulum imzalı güven, yeni kurulum giriş sınırı ve kurulum yerelleştirme sözleşmeleri geçti; `install.sh` değiştiği için çalıştırılan çevrimdışı sürüm kurtarma düzeneği de geçti (1.109 test, 1 atlandı); Go araç zinciri sözleşmesi `CelikPanel-S2-Debian` konuğunda geçti (yalnız sabitlenmiş 1.26.5 kabul edildiğinden `archlinux` konuğunun Go 1.27'sini tasarım gereği reddeder). Hiçbir test verisinin değişmesi gerekmedi: kabuk ve düzenek verilerindeki her örnek yönetici adresi kuralı zaten geçiyor.
- Arayüz değişmediği için arayüz derlemesi ya da arayüz testleri gerekmedi.

## Nasıl güncellenir

Yayımlandıktan sonra: sunucunun sahibi tarafından panelin kendi güncelleme
ekranından (Ayarlar → CelikPanel güncellemeleri). Öncesinde ya da sonrasında
yapılacak bir şey yoktur. Yeni bir kurulum kurucuyu eskisi gibi kullanır.
