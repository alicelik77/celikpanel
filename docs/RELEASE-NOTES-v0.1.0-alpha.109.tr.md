# v0.1.0-alpha.109

[English](RELEASE-NOTES-v0.1.0-alpha.109.md)

Bu, v0.1.0-alpha.108'i izleyen sürümdür. **26 Ekim 2026'da yazılmış bir adaydır;
yayımlanmadı ve hiçbir parçası kurulu bir sunucuda değil. Kurulu panelleri yalnız
sahip, panelin kendi güncelleme ekranından günceller.** alpha.108'in yayım
kaydından sonraki `main`'den `fix/alpha109-webmail-update` dalında yazıldı
(alpha.108 sürüm commit'i `f99ae9f3ec045ea45c7eada690d052c0767848f5`).

## Kim güncellemeli, neden

Web postası olan ve kurulum onaylanan her sunucu. Frankfurt'ta Roundcube'un 1.6.20
güncellemesi başarısız oldu ve geri alındı; ardından kilit "Başka bir CelikPanel
işlemi hâlâ sürüyor" dedi, satır ne olduğunu söylemeden yeniden "Güncelleme var"
gösterdi ve web postası düz kutularla açıldı. Sahip ayrıca web postasını nasıl
açacağını bilmiyordu ve kurulum onayını birden fazla paket adı geçince kafa
karıştırıcı buldu. Sahibin her sunucuda yapacakları
[işlem rehberinde](OPERATION-GUIDANCE.tr.md#frankfurtun-web-postası-güncellemesi-web-postası-bağlantısı-ve-kurulum-onayları-26-ekim-2026).

## Eski sürümlerle kurulmuş sunucularda da çalışan web postası güncellemesi

- **Frankfurt'taki neden.** Web postasının web sunucusu yapılandırmasını eski bir
  sürüm yazmıştı; CelikPanel'in PHP denetimi için konum yoktu. Bu yüzden takastan
  sonraki kanıt HTTP 404 okudu ve çalışan 1.6.20 geri alındı (doğru biçimde, bayt
  bayt). Artık bir güncellemeden ve her PHP aracı kanıtından önce denetim ve konumu
  yerine konur: yalnız CelikPanel'in kendi biçimindeki dosyaya, dinleme adresi, kök
  ve soketleri korunarak; nginx denetler ve yeniden yükler, reddederse önceki dosya
  geri konur. Sahibin değiştirdiği dosyaya dokunulmaz; güncelleme Roundcube
  değişmeden durur ve kimin ne yapacağını söyler.
- **Güncelleme tarayıcıda değil sunucuda sürer.** Onay bir işlem kaydeder ve hemen
  döner; sayfa kapansa ya da bağlantı kopsa da güncelleme sürer; ekran onu okuyarak
  izler, yeniden yüklenince yeniden bağlanır ve asla ikinci bir güncelleme göndermez.
  Güncelleme sırasında panel yeniden başlarsa sonuç kurulu sürümden salt okunarak
  uzlaştırılır.
- **Sonuç söylenmeye devam eder.** Roundcube satırı ve Yönet kartı, sunucu o sürümü
  çalıştırana dek son güncellemenin ne yaptığını söyler: güncellendi; başarısız oldu
  ve nedeniyle geri kondu; değiştirilmedi (gerekçesiyle); geri kondu ama yanıt
  vermiyor; geri konamadı; yarıda kaldı ya da bilinmiyor — her biri kimin ne
  yapacağıyla.
- **Kanıt web postasının stil dosyasını da yükler** ve CSS olarak gelmesini ister;
  düzeni olmadan açılacak bir web postası asla geçmez.

## Web postası arayüzüyle açılır

Panelin arkasındaki web postası, panelin kendi güvenlik kuralını alıyordu: betiklere
yalnız dosyadan izin verir, çerçeveleri yasaklar. Roundcube kendini satır içi bir
betikle başlatır ve iletileri aynı kökenli çerçevelerde gösterir; bu yüzden arayüzü
hiç başlamadı: giriş sayfasında düz kutular, girişten sonra da çalışan bir posta
kutusu yok. Bunu geri alma yapmadı; web postası panel üzerinden sunulduğundan beri
böyleydi. Web postasının yanıtları artık kendi kuralını taşır (çerçeveler yalnız
aynı kökenden, Roundcube'un kendi `X-Frame-Options` değeri); nosniff, yönlendiren
kuralı ve HSTS kalır.

## Web postasını aç

Roundcube satırı ve Yönet sayfası, web postası orada yanıt verirken tam adresiyle
**Web postasını aç** gösterir ve posta kutusu kullanıcılarının e-posta adresi ve
parolasıyla girdiğini, panel hesabı gerekmediğini söyler. Alan adının posta ekranı
da aynı adresi söyler.

## Okunabilen kurulum onayları

Her kurulum penceresi (bir bileşen, sürüm listesi, paket grubu, Onar) artık her
satırda bir paketle şunları söyler: **Değiştirilecek paketler** (dağıtımın sürümü ve
deponun sürümü, nedeni ve sonra güncellemelerinin nereden geleceği) ve gerekçeleri
altında **Ek olarak kurulacak paketler**; dağıtımın kendi sürümü varsa onun nasıl
seçileceği. Sürümler kısa gösterilir (`2:93ubuntu2` değil, `2:93`); tam sürümler
ipucunda ve sunucunun denetlediği bilgide kalır. Paket sayan retler aynı yapıyı
kullanır.

## Yükseltme ve kurtarma

Şema değişmez (şema 46): güncelleme var olan hizmet işlemleri tablosunu kullanır
(tür `webmail_update`). alpha.108'e kurtarma olağan yoldur; alpha.108 Paneli biten
bir `webmail_update` satırını geçmiş bir işlem olarak gösterir. Bugünkü biçime
yeniden yazılan bir web postası yapılandırması alpha.108 için de CelikPanel'in
kendi biçimidir.

## Kanıt

- Agent 4827 geçti, 29 atlandı (temiz bir bağlama ad alanında root olarak); Agent
  paketi CI'daki gibi yetkisiz `nobody` kullanıcısıyla da geçer; Panel 3448 geçti,
  6 atlandı; her iç paket geçer; web 1920 geçti ve üretim derlemesi bütçelerinde.
  Sürüm kurtarma düzeneği değişmedi (şema 46) ve bu adayın kanıtına girmez. Yarış
  bölümleri yerelde koşturulamadı (o konukta C derleyicisi yok); CI koşturur.
- Saklanan temel görüntüden atılabilir bir Debian 13 sanal makinesinde, Agent
  biriminin umask değeriyle bir gerçek sistem denemesi: Frankfurt'un HTTP 404'ü eski
  yapılandırmadan kelimesi kelimesine yeniden üretildi; kanıtı bilerek başarısız
  kılınan bir güncelleme 1.6.15'i geri koydu ve yanıt verdi; 1.6.20'ye gerçek
  güncelleme denetim, sayfa ve stil dosyasıyla (`text/css`) kanıtlandı
  ([kanıt](../deploy/e2e/d038/alpha109-debian13-20261026/README.md), arşiv SHA-256 `e77d1ab4…960f7ee`).
- Sahte veriyle tarayıcı bakışı (EN ve TR, 1440 ve 390, açık; grup 32): gruplu onay,
  web postası satırı ve kartı; yana kayma yok, ham anahtar yok.

Kurulu bir sunucuda ölçülmedi. Aday kabul değildir; P0.1–P0.5 **kısmi** kalır;
hiçbir P0 işi kapanmaz ya da ilerlemez.
