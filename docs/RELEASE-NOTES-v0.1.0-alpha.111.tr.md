# v0.1.0-alpha.111 (aday)

[English](RELEASE-NOTES-v0.1.0-alpha.111.md)

Bu sürüm v0.1.0-alpha.110'un ardından gelir. **29 Ekim 2026'da yazılmış bir
adaydır; yayımlanmadı ve hiçbir parçası kurulu bir sunucuda değil. Kurulu panelleri
yalnız sahibi, panelin kendi güncelleme ekranından günceller.** alpha.110'un yayın
kaydı (`79059a644`; alpha.110'un sürüm commit'i
`3abeb40d137e2664988a2a3735f2a44361f5774b`) üzerindeki `main`'den
`feat/setup-wizard-v2` dalında yazıldı.

Bu tek sürüm, alpha.111'den alpha.115'e kadar planlanan her şeyi getirir: yeni
kurulum sihirbazı, ikinci sunucunun kodla katılması, yenilenen DNS sayfası, iki
sunucu arasındaki kurulum bağlantısı ve dört sanal makine koşusundan çıkan
düzeltmeler. Beta 1 kurulumu budur.

## Kim kurmalı, nasıl

**Yeni sunucular.** Test sunucuları bu sürümle yeniden kurulur. Önceki alfa
kurulumlarından bir yükseltme yolu vaat edilmez: önceki bir alfanın kurulumuyla
kurulmuş sunucu, yeni sihirbazı kullanmak için güncellenmez, yeniden kurulur. Temiz
kurulum yolu:

1. Her sunucuya temiz bir işletim sistemi kurun (denenenler Debian 13 ve Ubuntu
   24.04).
2. Her sunucuda celikpanel.net'teki kurulum komutunu root olarak çalıştırın
   (indirme sayfasındaki komut).
3. İlk sunucuda paneli açın ve sihirbazı izleyin: ana alan adı, bir ya da iki
   sunucu, görevler, adlar ve DNS'in nerede tutulacağı.
4. Kendi DNS'inizde iki sunucuyla ilk sunucu ikinciyi bekler. İlk sunucuda bir
   katılma kodu oluşturun; sonra ikinci sunucuda "İlk sunucuya kodla katıl"ı seçip
   ilk sunucunun adresini ve kodu girin. İkinci sunucu ortak yanıtları ilkinden
   alır.
5. Kayıt kuruluşu kartı söylediğinde alan adının ad sunucularını kayıt
   kuruluşunuzda, alan adının içindeki ad sunucusu adları için de IP kayıtlarını
   (glue) girin. Kart kayıt merkezini kendisi denetler ve doğru olduğunda söyler.

## Ne değişti

- **Yeni kurulum sihirbazı.** Önce ana alan adı; sonra bir ya da iki sunucu,
  sunucunun kısa adı, görevler (web siteleri, e-posta, DNS), isteğe bağlı
  bileşenler, ana alan adından önerilen adlar (sizin değişiklikleriniz korunur),
  DNS'in nerede tutulacağı, özet ve kurulum. Her adım neyi kuracağını söyler.
- **DNS nerede tutulur.** *Kendi çiftiniz*: iki sunucu alan adının DNS'ini tutar;
  biri ana DNS sunucusu, diğeri yedek, BIND ya da PowerDNS ile; ana sunucu
  ikisinden biri olabilir. *Dış sağlayıcı*: DNS yazılımı kurulmaz; ekran eklenecek
  kayıtları ve her biri için internetin ne yanıt verdiğini gösterir. *Tek başına*:
  bir sunucu kendi DNS'ini tutar, iki ad sunucusu adı da onu gösterir.
- **Kodla katılma.** İkinci sunucunun adresi ilk sunucuda hiç yazılmaz. Kod bir
  kez gösterilir, 24 saat geçerlidir, bir adresten en çok 5 yanlış denemeye izin
  verir (toplam 50) ve hiçbir şey gönderilmeden önce ilk sunucunun anahtarına
  karşı denetlenir; böylece yanlış adres kodu hiç almaz. Kaybolan yanıt aynı
  istekle yeniden denenir ve tam olarak bir katılma olur.
- **Kayıt kuruluşu kartı.** Girilecek ad sunucularını ve glue kayıtlarını adlandırır,
  kayıt merkezinin kendi sunucularına ve bizim sunucularımıza sorar; doğru, henüz
  değil, yedek DNS sunucusu bekleniyor ya da denetlenemedi der. Okunamayan yanıt
  asla "henüz değil" sayılmaz.
- **DNS sayfası.** Sunucu → DNS, bütün tabloyu anlatan tek bir cümleyle açılır
  (kim ne yapacak, sonra ne olacak); ardından sunucular, adlar ve internetin
  yanıtı, posta kayıtları ve sizin eklediğiniz kayıtlar gelir. Önceki ayarlar
  Gelişmiş altında korunur. Eski posta sunucusunu tuttuysanız "Postayı taşı",
  onayınızdan sonra tek MX kaydını değiştirir.
- **Var olan kayıtlar ve posta kayıtları.** Kurulum, alan adının olağan adları için
  internetin verdiği yanıtları okur; her kaydı içe alır ya da atlarsınız. E-postayla
  MX, SPF (birleştirilmiş), DMARC ve DKIM yazılır. Ters DNS, sunucu sağlayıcınız
  için bir görev olarak listelenir; kurulum onu beklemez.
- **Posta DNS'ten sonra.** Posta adımları iki sunucuda da DNS adımlarından sonra
  çalışır; böylece postanın DNS kimliği her zaman hazırdır.
- **Tek posta alıcısı.** Ana alan adının postasını tam olarak bir sunucu alır. İki
  sunucuda da posta seçmek incelemede, neyin değişeceği söylenerek reddedilir.
- **Bekleme ve kurtarma metinleri.** Her bekleme ve hata nedenini, kimin işlem
  yapacağını ve sıradaki adımı söyler. Yeniden başlatmadan sonra hâlâ açılan bir
  sunucu hata değil, beklemedir. Sunucuda başka bir değişiklik çalışırken başlatma
  bekletilir, çalışan kurulum da onu bekler. Sertifika yetkilisinin gerisinde kalan
  saat, saniyesiyle ve `timedatectl set-ntp true` ile söylenir. Kurulum bağlantısı
  yedek hazır olduğunu bildirene kadar kalır; metinleri sunucuları göreviyle
  adlandırır ("diğer sunucu", "ana DNS sunucusu"). Kendi yerel DNS
  değişiklikleriniz fark edilir, korunur ve bildirilir; asla üzerine yazılmaz.

## Bilinen sınırlar

- Katılmadaki kaynak denetimi yalnız IPv4'tür.
- Bağlantısını yitiren ikinci sunucu yeni bir kodla yeniden katılmalıdır.
- Kendi yerel DNS değişikliğinizden sonra henüz devralma ya da geri verme yok:
  değişiklik korunur ve bildirilir, bu arada CelikPanel orada DNS değişikliği
  yayımlamaz.
- Kurulumdan sonra ve kurtarmada PowerDNS'in kaynağı yalnız sürüm kuralıyla
  denetlenir (denenen 4.9 serisinin Debian'ın kendi arşivindeki yapısı).
- Henüz "ikinci sunucuyu sonra ekle" ve "Ana DNS'i taşı" yok.
- Bir alan adı için iki sunucuda da posta reddedilir.
- Meşgul diye reddedilen, sürdürülmüş bir kurulum doğrulanmış bir hatadır; yeniden
  sürdürülmez.
- Bir posta adımı sırasında saatin geriye atlaması o adımı düşürebilir; Planı
  düzenle ve yeniden başlat onu kurtarır.
- Gerçek sistemlerde ölçülmedi: lisans davranışı (koşular kabul yapısıyla yapıldı),
  bir koşunun ortasında meşgul bekleme, büyük saat kaymasında posta sertifikası,
  yalın HTTP 401 olan bir bağlantı reddi, IPv6 ve ters DNS.

## Yükseltme ve kurtarma

Şema değişikliği yok (şema 46). Sihirbazın yanıtları, katılma kodları (yalnız
anahtarlı özet olarak), kurulum bağlantısının mühürlü değerleri ve kurulumun yazdığı
kayıtların listesi `panel_settings` ile var olan kurulum ve DNS tablolarında tutulur.
Bir sunucunun yeniden kullanması gereken her gizli değer kendi anahtar dosyasıyla
mühürlenir; veritabanının tek başına bir kopyası kod, kimlik bilgisi ya da kayıt
vermez. Bağlantı kimlik bilgisi ve katılma kodu, kurulum bitince iki sunucudan da
silinir.

İki sunucu arasındaki DNS çoğaltması DNS yazılımının kendi bölge aktarımıdır ve
panele gerek duymaz (D-022). İlk koşuda panel ve ajan durdurulmuşken sahibin yerel
`rndc` değişikliği aktarımla yedeğe ulaştı; sonra panel onu fark etti ve korudu.

Yarıda kalan bir adım, yeniden başlatmadan ya da panel yeniden başladıktan sonra
aynı işlem olarak sürer; durum sorgusu hiçbir değişikliği başlatmaz. Panelin kendisi
olağan sürüm kurallarıyla kurtarılır (şema değişikliği yok). Bir kurulumun ortasında
alpha.110'a dönüş ölçülmedi; bu temiz test sunucuları için geri dönüş yolu yeniden
kurulumdur. Kurulu panelleri yalnız sahipleri, panelin kendi güncelleme ekranından
günceller.

## Kanıt

- Bir laboratuvar ağında, atılabilir sanal makinelerde dört koşu; her biri temiz
  işletim sisteminden müşteri kurulum komutu ve sihirbazla: laboratuvar DNS kökü ve
  üst bölgesi, bir laboratuvar çözümleyicisi ve bir laboratuvar sertifika yetkilisi
  (pebble). Gerçek bir sunucuya, kayıt kuruluşuna ya da sertifika yetkilisine
  bağlanılmadı.
  - İlk koşu (ürün commit'i `c2d60e574`): Debian 13 + Ubuntu 24.04 BIND çifti ve
    dış sağlayıcı düzeni geçti; katılma retleri, yineleme, yeniden başlatmalar, ad
    çakışmaları ve kayıt kuruluşu kartı yazıldığı gibi davrandı. Beş kusur buldu
    (temiz kalıplarda reddedilen MariaDB, tek başına bölge türü, DNS'ten önce posta,
    PowerDNS sürümü, kurtarma yolu olmayan yazılım değişikliği); hepsi düzeltildi.
  - İkinci koşu (`b86ab6fa4`): tek başına BIND ve ana sunucu olarak PowerDNS geçti,
    yazılım değişikliğinden kurtarma da geçti; beş kusur daha bulundu ve düzeltildi.
  - Üçüncü koşu (`cfcadd3f1`): bu düzeltmeler geçti; altı kusur daha (açılmakta
    olan sunucu, saat nedeni, bağlantı üzerinden DKIM, posta sertifikası beklemesi,
    yedeğin kendini posta alıcısı göstermesi, bir metin) bulundu ve düzeltildi.
  - Dördüncü koşu (`68a9915a0`, bu adayın ürün kodu): iki sunuculu iki düzen de
    temiz kurulumdan uçtan uca hazır oldu. Gerçek bir DNS beklemesi sırasında, ana
    sunucunun MariaDB adımında ve sonra erişim DNS'i sırasında yeniden başlatma,
    hiçbir şey iki kez yapılmadan sürdü; Bileşenler kurulumu sırasında başlatma
    `server_setup_host_busy` ile reddedildi; 200 sn geride kalan saat adlandırıldı
    ve düzeltmeden sonra bitti; iki sunucuda posta incelemede reddedildi; A
    seçeneğinde yedekteki posta DKIM'ini bağlantı üzerinden aldı. Önceki denetimler
    (katılma kodu, kayıt kuruluşu kartı, sahip bağımsızlığı, tek başına düzen,
    yeniden başlatmadan sonra ana PowerDNS, Postayı taşı) yeniden geçti.
  - Özetler ve arşivler:
    [birinci](../deploy/e2e/dns-pair-acceptance/evidence/beta1-20261029/SUMMARY.md),
    [ikinci](../deploy/e2e/dns-pair-acceptance/evidence/beta1b-20261029/SUMMARY.md),
    [üçüncü](../deploy/e2e/dns-pair-acceptance/evidence/beta1c-20261029/SUMMARY.md),
    [dördüncü](../deploy/e2e/dns-pair-acceptance/evidence/beta1d-20261029/SUMMARY.md).
    Arşivler commit edilmeden önce anahtar, kod, kimlik bilgisi, parola ve çerez
    için tarandı: temiz.
- Aday ağaçta testler: Ajan 4870 geçti, 29 atlandı (temiz bir bağlama ad alanında root olarak), bu test konuğunun ortamından gelen bilinen bir hatayla (`TestTheReal1620ArchiveVerifiesWithThePinnedKey`); Panel 3601 geçti, 6 atlandı; her iç paket geçer; yarış parçaları yerelde çalıştırılmadı (CI çalıştırır); web 1980 geçti, tür denetimi ve üretim derlemesi
  bütçeleri içinde; sürüm sırası, indirme portalı ve imzalı bildirim sözleşmeleri
  geçer.

Anayasa: ilke 1 (sahibin hizmetleri ve yerel değişiklikleri), 2 (bilinmeyen asla
"henüz değil" ya da hata değildir), 4 (her adım aynı işlem olarak sürer) ve 6
(bekleme ve ilerleme erişimden ayrıdır; durum sorgusu hiçbir şeyi değiştirmez);
D-021, D-022, D-024 ve D-025 ile. Kurulu bir sunucuda hiçbir şey ölçülmedi. Aday
kabul değildir; P0.1–P0.5 **kısmi** kalır; hiçbir P0 işi kapanmaz ya da ilerlemez.
