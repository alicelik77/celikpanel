# v0.1.0-alpha.105 (aday)

[English](RELEASE-NOTES-v0.1.0-alpha.105.md)

Bu, v0.1.0-alpha.104'ü izleyen adaydır. **Yayımlanmadı.** 24 Ekim 2026'da aday
olarak, alpha.104'ün yayın kaydından sonraki `main` üzerinden
`feat/alpha105-bundle-versions` dalında yazıldı (alpha.104 sürüm commit'i
`98c4750b4a5d4ad67447468bcfdde3a3b848465a`).

## Kim güncellemeli ve neden

Bileşenler'den hazır paket ya da veritabanı ve web postası araçları kuran herkes.
Sahip, bir paketin sürümlerini nasıl seçtiğini ve uyumlu olanları zorlaması gerekip
gerekmediğini sordu ve CelikPanel'in ilkesini hatırlattı: **öner, uyar, asla
dayatma** (D-034 değişikliği, 24 Ekim 2026).

Bir sunucuyu yalnız sahibi, panelin kendi güncelleme ekranından günceller. Sahibin
her durumda ne gördüğü [işletim
rehberinde](OPERATION-GUIDANCE.tr.md#pakette-sürüm-seçmek-seçtiğinizden-fazlasını-getiren-kurulumlar-kurulumdan-sonra-hazır-olmayan-bir-araç-d-034-değişikliği-24-ekim-2026).

## Paket sürümlerini gösterir, siz değiştirebilirsiniz

- Paketin kurduğu her parça gelecek sürümü söyler, örneğin "8.4 · dağıtımın sürümü
  · önerilen". **Değiştir**, o parçanın kendi kurulumunun zaten sunduğu sürümleri
  listeler (üreticinin PHP deposu; PostgreSQL'in kendi deposu); yeni depo eklenmez.
  Zaten kurulu bir parça sürümünü söyler ve olduğu gibi bırakılır.
- Önerilen sürüm dağıtımın kendi sürümüdür. İşaretlenir, asla zorlanmaz.
- Bir PHP sürümü seçtiğinizde paketin veritabanı aracı onun üzerinde çalışır ("PHP
  8.3 üzerinde çalışır").
- Çalışamayacak bir PHP sürümü hiçbir şey değişmeden reddedilir, örneğin
  "phpPgAdmin burada PHP 8.6 üzerinde çalışamaz: php8.6-pgsql bu sunucunun
  depolarında yok. PHP-FPM için başka bir PHP sürümü seçin." Genellikle çalışan ama
  sınanmamış bir sürüm ya da eski bir PHP yalnız uyarıdır ("… Devam
  edebilirsiniz.").

## Bir kurulumun ne getireceğini başlamadan görürsünüz

- Önce paket yöneticisine kurulumun ne getireceği sorulur. Başka bir PHP sürümü,
  Apache, başka bir bileşen ya da bir veritabanı sunucusu getirecekse pencere "Bu,
  seçmediğiniz paketleri de kurar:" der, adları ve düz bir gerekçeyi verir; siz
  "Okudum; bu paketleri de kur." kutusunu işaretleyene kadar Kur bekler. Bu, paket
  penceresinde ve bir bileşenin kendi kurulum penceresinde vardır.
- Bu denetim yapılamazsa (paket yöneticisi meşgulse) pencere bunu söyler ve sizi
  durdurmaz: sunucu kurmadan hemen önce yeniden denetler ve onaylamadığınız bir şey
  gelecekse hiçbir şey kurmadan reddeder.

## Araçlar ilk kurulduklarında denetlenir

- phpMyAdmin, phpPgAdmin ve Roundcube kurulumlarından hemen sonra, alpha.104'teki
  taşımayla aynı biçimde denetlenir (CelikPanel'in PHP denetimi, sonra aracın kendi
  sayfası). Denetim geçmezse aracın eklentileri olan diğer kurulu PHP sürümleri
  denenir; başarısız olan her biri geri konur.
- Hiçbiri çalışmazsa araç kurulu kalır ve nedeni ile sonraki eylemle "Hazır değil"
  okunur: aracın Yönet penceresinde bir PHP sürümü seçin. Zaten kurulu bir araç
  denetlenir ama asla taşınmaz.

## Altta

- Bir kurulumun ne getireceğini soran yeni salt-okur Agent denetimi ve bir aracı
  kurulumunun içinde kanıtlayan yeni Agent adımı; kurulumlar çalışmadan hemen önce
  paket kilidi altında yeniden denetlenir.
- Panel: `GET /api/v1/service/install-preview`; her kurulum sunucuda denetlenir
  (bir bileşenin kendi sayfası, Sunucu kurulumu, posta profilleri, sürüm listesinden
  eklenen bir PHP sürümü, Onar ve açık kalmış eski bir sekme dahil); tarayıcı yalnız
  neyi onayladığınızı söyler. Bir kurulum seçilen PHP sürümünü ve onaylananları
  kaydında tutar; kesilip yeniden süren kurulum istediğiniz gibi çalışır.
- Sürekli tümleştirme: "Panel race — Server setup" işi ikiye bölündü ("(1/2)" ve
  "(2/2)"); zaman sınırı yükseltilmedi.

## Değişmeyenler

Siteler PHP sürümlerini yine kendi ayarlarında seçer. Güncelleyici, şema (44) ve
her kurulu sunucu değişmedi; yalnız sahip günceller. Her kurulum denetlenir ve
alpha.104'ün bir kurulumun asla paket kaldırmadığı kuralı sürer. Paketleri listeleyen
bir pencerenin olmadığı yerlerde (Sunucu kurulumu, bir posta profili, sürüm
listesinden eklenen bir PHP sürümü, Onar) dikkat çeken bir paket getirecek kurulum
hiçbir şey kurulmadan reddedilir; yönlendirmesi paketleri adlandırır ve o bileşeni,
paketlerin onayınız için listelendiği Bileşenler'den kurmanızı söyler.

## Aday üzerindeki kanıt

- Agent: 4723 geçti, 29 atlandı (temiz bir bağlama ad alanında).
- Panel ve iç paketler: 7243 geçti, 44 atlandı.
- Web: 1796 geçti; üretim derlemesi bütçelerinin içinde.
- Gerçek sistem, atılabilir bir Debian 13 sanal makinesi (QEMU/KVM, denemeden
  sonra silindi): yalın PHP 8.3 üzerinde tek başına phpMyAdmin'in PHP 8.4
  paketlerini getireceği görüldü ve onaysız kurulumu hiçbir şey kurulmadan
  reddedildi; PostgreSQL + PHP-FPM 8.3 + phpPgAdmin başka PHP ve Apache getirmedi,
  phpPgAdmin 8.3'te kanıtlandı; FPM paketi olmayan bir PHP sürümü reddedildi; 8.3'te
  pgsql kapalıyken zaten kurulu bir aracın denetimi nedeniyle başarısız oldu ve
  taşınmadı, ilk kurulum ise PHP 8.4'e geçti ve kanıtlandı
  ([kanıt](../deploy/e2e/bundle-versions/alpha105-debian13-20261024/README.md),
  arşiv SHA-256 `183e4275…61f2f8`). Makine, sonraki sürümler için saklanan yeniden
  kullanılabilir bir Debian 13 temel kalıbından başladı (SHA-256
  `e1c1173b…53caec`).
- Sürüm kurtarma düzeneği koşturulmadı: hiçbir güncelleme dosyası değişmedi.

## Bilinen eksikler

- Kurulu bir sunucuda hiçbir şey ölçülmedi.
- pacman denetimi yalnız bileşen testleriyle kapsanır; DNF yolu ulaşılamaz kalır.
- Panel'in denetim yolu, tarayıcı penceresi ve paket sırası sanal makine
  denemesinin parçası değildi (bileşen ve bağlı testler kapsar); beklenmeyen kurulum
  olarak Apache yalnız bileşen testleriyle kapsanır.
- Apache kuralı yalnız Debian ve Ubuntu adlarını bilir (apache2, apache2-bin,
  libapache2-mod-php*), Arch'ınkileri (apache, php-apache) bilmez; Arch'ta denetim
  (`pacman -S --print`) sistemi de yükselten gerçek `-Syu` işlemi değildir.
- Dal koruması eski "Panel race — Server setup" denetim adını istiyorsa, sahibin onu
  iki yeni adla değiştirmesi gerekir.
- P0.1–P0.5 kısmi kalır; hiçbir P0 işi kapanmaz ya da ilerlemez.
