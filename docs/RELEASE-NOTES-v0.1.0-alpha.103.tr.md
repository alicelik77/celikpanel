# v0.1.0-alpha.103 (aday)

[English](RELEASE-NOTES-v0.1.0-alpha.103.md)

Bu, v0.1.0-alpha.102'den sonra gelen adaydır. **Yayımlanmadı.** 23 Ekim 2026'da,
aday olarak, `fix/alpha103-fresh-update-check` dalında, alpha.102'nin yayım
kaydından sonraki `main`'den yazıldı (alpha.102 sürüm commit'i
`085cc400131072f3bcda81b560bfcf98dad47f03`).

## Kim güncellemeli ve neden

Yayımlandığında, CelikPanel'i kendi güncelleme ekranından güncelleyen herkes.
Sahibin 23 Ekim 2026'da, iki sunucu da alpha.100'deyken gördüğü iki şey:

- **Frankfurt.** Kenar çubuğu "Update available · v0.1.0-alpha.101" diyordu.
  Açılan güncelleme ekranı, alpha.102 yaklaşık 10:02Z'den beri celikpanel.net'te
  olduğu hâlde "Update to v0.1.0-alpha.101" sundu; alpha.102'yi yalnız "Check for
  updates" gösterdi. Sahibin sözleriyle: "güncellemede her zaman en son versiyonu
  göstermedi … sol alt kısımda 101 nolu güncellemeyi gösterdiğinde ve onu
  tıkladığımda güncelleme ekranında en son güncelleme görünmesi gerekmez miydi".
  Ardından Frankfurt'u kendisi alpha.102'ye güncelledi.
- **Boston.** alpha.102 güncellemesi, sunucunun paket yöneticisi meşgul olduğu
  için hiçbir şeyi değiştirmeden durdu. Bu bildirim açıkken "CelikPanel
  güncellemeleri" kartı yalnız kurulu sürümü, "Teknik ayrıntılar"ı ve otomatik
  denetim anahtarını gösterdi; kenar çubuğu ve Ayarlar öğesi hâlâ "Güncelleme var
  · v0.1.0-alpha.102" diyordu ve bildirim, kartın paket yöneticisinin ne zaman
  boşaldığını söyleyeceğini yazıyordu.

Bir sunucuyu yalnız sahibi, panelin kendi güncelleme ekranından günceller.

## Güncelleme ekranında değişenler

**Ekran açılınca en yeni sürüm denetlenir**: bilinen sonuç on dakikadan eskiyse ve
otomatik denetim açıksa (D-035 değişikliği). Denetim sürerken kart hareketli
"celikpanel.net'te en yeni sürüm denetleniyor…", "Bu denetimden önce bilinen:
v0.1.0-alpha.101 sürümü var (13 saat önce bulundu)." ve "Güncelleme düğmesi bu
yanıtı bekler, en çok bir dakika kadar. Hiçbir işlem kendiliğinden başlamaz." der;
bilinen teklif görünür kalır ve güncelleme düğmesi bekler. Ardından en yeni sürüm
sunulur; kenar çubuğu bağlantısı, Ayarlar öğesi ve Pano satırı aynı sürümü
gösterir.

- On dakika: ekranı bir oturumda birkaç kez açmak her seferinde celikpanel.net'e
  sormasın, ama daha yeni bir sürüm çıktığında ekranın sunduğu şey 12 saatlik
  zamanlamanın sonucu olmasın diye.
- "Güncellemeleri denetle" ve zamanlamayla aynı denetimdir, onlarla aynı anda
  değil sırayla ve aynı kurallarla çalışır: bu sunucuda son bir saatte bir
  güncelleme başlatıldıysa, sunucu süren bir değişiklik bildiriyorsa, sunucu
  kurulumu sunucuyu değiştiriyorsa ya da Agent yanıt vermiyorsa denetim yapılmaz.
  Meşgul paket yöneticisi onu durdurmaz.
- Denetleyemezse teklif yaşı ve nedeniyle kalır: başarısız bir denetim kendi
  satırını korur ("Son otomatik denetim: az önce — sürüm sunucusundan
  (celikpanel.net) doğrulanmış bir yanıt alınamadı."), karşılanmayan bir koşul bunu
  söyler ("Açılışta yeniden denetlenmedi, çünkü bu sunucuda bir saatten kısa süre
  önce bir güncelleme başlatıldı."; "Açılışta yeniden denetlenmedi: sunucuda başka
  bir değişiklik sürüyordu."), zamanında yanıt gelmezse "Açılışta yapılan denetim
  zamanında yanıt vermedi; bu yüzden sonucu bilinmiyor." der; her birinin ardından
  "Gösterilen, son denetimin sonucudur (13 saat önce); şimdi de
  denetleyebilirsiniz." gelir.
- **Otomatik denetim kapalıyken ekranın açılması hiçbir yere sormaz.** Teklif "Son
  denetim: 13 saat önce." der, sonraki satır "Otomatik denetim kapalı. Şimdi bakmak
  için "Güncellemeleri denetle"ye basın." der.

**Durdurulan bir güncelleme kartı boş bırakmaz.** Hiçbir şeyi değiştirmeden duran
(ya da doğrulanarak geri dönen) bir güncellemenin bildirimi açıkken kart teklifi,
"Bu sürüme yapılan önceki güncelleme kurulu sürümü değiştirmeden durdu" başlığını
"Sunucunun paket yöneticisi meşguldü, örneğin otomatik güvenlik güncellemeleri
çalışıyordu. Hiçbir şey değiştirilmedi; …" ile, kartın kendi okumasından paket
yöneticisinin durumunu ("Paket yöneticisi hâlâ meşgul. Bu kart kendiliğinden
yeniden denetler ve güncellemenin ne zaman başlatılabileceğini söyler; hiçbir işlem
kendiliğinden başlamaz.", ardından "Paket yöneticisi işini bitirdi. Güncelleme
yeniden başlatılabilir.") ve kartın köşe bildiriminin örtmediği en üstünde şunu
gösterir: "Önce durdurulan güncellemeyle ilgili bildirimi kapatın ("Bildirimi
kapat"); ardından bu düğme v0.1.0-alpha.102 güncellemesini başlatır. Hiçbir işlem
kendiliğinden başlamaz." Güncelleme düğmesi bildirim kapanana kadar bekler.

Telefonda hazırlık kutusunun "Tekrar dene" düğmesi artık 44 px yüksekliğinde.

## Değişmeyenler

- Hiçbir şey kendiliğinden kurulmaz; ekran, zamanlama ve her yeniden okuma yalnız
  okur. Başlat, bir şey kurulmadan önce tam hedefi yine yeniden denetler ve
  değişmiş bir hedefi reddeder.
- Agent, güncelleyici (`update.sh`, sistem güncelleme işçisi, geri alma ve
  kurtarma), tarayıcıdaki güncelleme kilidi, roller, şema (44, geçiş yok). Sahibin
  16 Ekim 2026 talimatı gereği gerçek sistem kaydı yapılmadı.
- 12 saatlik zamanlama, geri çekilmesi ve anahtarı.

## Bilinen eksikler

- Kurulu bir sunucuda hiçbir şey ölçülmedi. Açılış denetimi gerçek bir Agent'a ya
  da gerçek sürüm sunucusuna karşı çalışmadı; Go testleri Agent'ı taklit eder,
  tarayıcı grubu yerel bir sahte sunucu kullanır.
- Durdurulan güncellemenin bildirimi açıkken köşe bildirimi geniş ekranda kartın
  bir kısmını hâlâ örter; ne yapılacağını söyleyen cümle kartın en üstündedir ve
  görünür kalır.
- Bir duruştan sonra sayfa açılışta yeniden denetlemez (duruş sunulacak sürümü
  değiştirmez); bildirim kapandıktan sonra "Güncellemeleri denetle" yine çalışır.

## Kanıt

Aday ağaçta:

- Go: açılış denetimi (eski sonuç yeniden denetlenir, on dakika içindeki taze sonuç denetlenmez, otomatik denetim kapalıyken Agent çağrısı yapılmaz, bir güncelleme kabul edilmişken ya da sunucu kilidi tutulurken denetim yapılmaz, meşgul paket yöneticisi onu tutmaz, başarısız denetim teklifi korur, yanıt vermeyen Agent atlama sayılır, beklerken biten istek hiçbir şey saklamaz, düğme, açılışlar ve zamanlama arasında aynı anda tek sürüm denetimi, yalnız yöneticiler, lisans kilidinde okunabilir); son Go ağacında `CelikPanel-S2-Debian` konuğunda geçti (Panel ve iç paketler, 45 dakika sınırı, CGO kapalı): 7.226 geçti, 44 atlandı, 0 başarısız (2.970 üst düzey test), başarısız paket yok; gofmt ve go vet temiz
- Web: 1.755 test geçti, 0 başarısız; aralarında eski bir saklı teklif ve daha
  yeni bir sürüm (bekleme, ardından en yeni sürümün sunulması ve kenar çubuğunun
  izlemesi), taze bir sonuç (denetim yok), otomatik denetim kapalı (istek yok, yaş
  gösterilir), başarısız, atlanan ya da yanıtsız denetim (teklif nedeniyle kalır),
  bir güncelleme izlenirken denetim yapılmaması ve Boston'ın durdurulmuş hâli
  (teklif, nedeniyle önceki deneme, "hâlâ meşgul" ve "işini bitirdi", bildirimi
  bekleyen Başlat, aynı sürümü söyleyen kenar çubuğu) için bağlanmış testler var;
  üretim derlemesi paket bütçeleri içinde geçti (hiçbir sınır yükseltilmedi).
- Tarayıcı incelemesi, 26. grup (yerel sahte sunucu; Frankfurt'un eski teklifi ve
  Boston'ın meşgul duruşu; İngilizce ve Türkçe, açık ve koyu, 1440 ve 390 px; 40
  durum): bir kez bakıldı, düzeltildi ("önce bildirimi kapatın" cümlesi köşe
  bildirimi onu örttüğü için kartın en üstüne taşındı, içindeki sürüm bölünmez
  tutuldu, telefonda Tekrar dene 44 px), yeniden bakıldı; ham anahtar yok, Türkçe
  sayfada İngilizce yok, yana kaydırma yok, telefonda 44 px'ten küçük hedef yok.
- Kabuk: `archlinux` konuğunda temiz bir kopyada çevrimdışı sürüm kurtarma
  düzeneği geçti (1.109 test, 1 atlandı; yansıttığı sonuç metinleri ve mantığı
  değişmediği için içinde güncellenecek bir şey yoktu), ve sürüm sırası politikası
  ("102 -> 103; historical bootstrap sequence 102"), indirme portalı ve imzalı
  sürüm manifesti sözleşmeleri geçti.

## Nasıl güncellenir

Yayımlandığında: sunucunun sahibi tarafından, panelin kendi güncelleme ekranından
(Ayarlar → CelikPanel güncellemeleri). Öncesinde ya da sonrasında yapılacak bir şey
yok.
