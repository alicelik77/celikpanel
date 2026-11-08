# v0.1.0-alpha.130

[English](RELEASE-NOTES-v0.1.0-alpha.130.md)

Posta, yol haritasında söz verilen işlere kavuşuyor: kendiliğinden ayarlanan
posta programları, postayı başka bir sunucu üzerinden gönderme, alt
adresleme, sunucu genelinde gönderen listesi ve parolası unutulan posta
kutusu için kurtarma adresiyle sıfırlama bağlantısı. Veritabanına dışarıdan
bağlanmada alpha.129'un açık bıraktığı noktalar kapanıyor. Sahibin 8 Kasım'da
Boston ve Frankfurt'ta bulduğu her şey de düzeltildi: yanlış "Postfix
çalışmıyor" uyarısı, geçerli panel sertifikasının başarısız gösterilmesi,
Türkçe ekranlarda İngilizce biçimli tarihler, Sunulanlar sekmesinde hiçbir
şeyi değiştirmeyen işaretler, sahibin panelden kapatamadığı SSH parola
girişi ve ayarlarını geride bırakan kaldırılmış PostgreSQL sürümü.
Kaldırmalar artık CelikPanel'in kendi kalıntılarını da götürüyor. **Kurulu
panelleri yalnız sahip, panelin kendi güncelleme ekranından günceller.**

## Posta

- **Posta programları kendiliğinden ayarlanıyor.** Thunderbird (autoconfig)
  ve Outlook (Autodiscover), postası bu sunucuda olan her üst alan adı için
  sunucuyu, portları ve şifrelemeyi yalnız adresten buluyor. Yanıtlar sitenin
  kendi nginx yapılandırmasındaki sabit dosyalardır: hiçbir zaman parola
  içermez, sorulan adresi geri yazmaz. HTTPS yalnız sitenin sertifikası
  `autoconfig.` ve `autodiscover.` adlarını kapsıyorsa kullanılır; sertifikaya
  bu adlar yalnız genel DNS onları zaten bu sunucuya gösteriyorsa eklenir.
  Yeni bölgeler kayıtları kendiliğinden alır; var olan bir bölgeye eksik
  kayıtlar posta kutusu ekranında tek tıkla eklenir, var olan bir kayıt asla
  değiştirilmez. Posta kutusu ekranı neyin eksik olduğunu ve kimin yapacağını
  söyler (docs/MAIL-AUTOCONFIG.md).
- **Postayı başka bir sunucu üzerinden gönderme (smarthost), sunucu
  genelinde.** Yönetici aktarma sunucusunun adresini, portunu, kullanıcı
  adını ve parolasını Postfix sayfasına girer; parola mühürlü saklanır ve bir
  daha gösterilmez. Postfix zorunlu şifreli bağlantı üzerinden oturum açar.
  "Oturum açmayı dene" bağlanır, oturum açar ve hiçbir şey göndermeden
  çıkar. Sahibin kendi aktarma ayarı fark edilir ve asla üzerine yazılmaz;
  kapatınca yalnız CelikPanel'in yazdıkları kaldırılır (docs/MAIL-RELAY.md).
- **Alt adresleme (`kullanici+etiket@alanadi`), sunucu genelinde.** Debian ve
  Ubuntu'nun kendi `main.cf` dosyası bunu zaten açar; ekran bunu söyler ve o
  satırı korur. Dovecot'un ayarı yalnız okunur, hiç yazılmaz.
- **Sunucu genelinde gönderen izin ve engel listesi.** Engellenen adres ya da
  alan adı 550 ile reddedilir; "izin" yalnız bir engelin istisnasıdır, bu
  yüzden liste hiçbir zaman açık aktarma yaratamaz ya da istenmeyen posta
  denetimini atlatamaz. Bu sunucudaki alan adları listeye eklenemez
  (docs/MAIL-SERVER-SETTINGS-DESIGN.md).
- **Posta kutusu için kurtarma adresi ve sıfırlama bağlantısı.** Kutunun
  parolasını değiştirebilen kişi ona bir kurtarma adresi verebilir; adres,
  kendisine gönderilen bağlantı açıldıktan sonra geçerli olur. Kutu
  kullanıcısı panelin "parolamı unuttum" sayfasından sıfırlama bağlantısı
  ister; bağlantı yalnız doğrulanmış adrese gider, 30 dakika geçerlidir, bir
  kez kullanılır. Sıfırlama posta kutusu parola kuralını uygular, açık
  IMAP/POP3 oturumlarını kapatır ve kutuya bilgi postası gönderir. İsteğe
  verilen yanıt her adres için aynıdır. Alan adının devri ya da silinip
  yeniden eklenmesi kurtarma adresini geçersiz kılar. Önce yazılı bir
  güvenlik incelemesi yapıldı (docs/MAILBOX-RECOVERY-REVIEW.md).
- **Elle durdurulan gönderim parola değişikliğiyle açılmıyor.** Şimdiye dek
  her parola değişikliği durdurulmuş bir kutuyu yeniden açıyordu; yöneticinin
  ya da bayinin elle durdurduğu kutuyu da. Artık parola değişikliği yalnız
  güvenlik durdurmasını (ani artış, retler) kaldırır; ekran elle durdurulan
  gönderimi kimin açacağını söyler.

## Veritabanına dışarıdan bağlanma

- **MariaDB ana makine adları artık Fail2ban'dan kaçamıyor.** Kural yalnız
  adresleri yasaklar. MariaDB istemci adlarını çözdüğü sürece MariaDB
  kuralları bekler; yönetici onaylarsa CelikPanel ad çözmeyi kendi seçenek
  dosyasında kapatır (sahibin ayarının üzerine ya da bir hesap ana makine
  adı taşırken asla) ve son kuralla birlikte kaldırır.
- **PostgreSQL için Fail2ban**, `auth_delay`'in yanında ek bir katman olarak;
  yalnız paketin varsayılan günlük önekiyle çalışır. Son kural ya da Fail2ban
  gidince geride hiçbir şey kalmaz.
- **Süreli erişim:** yeni bir adres varsayılan olarak 1 gün açık kalır; 7 gün
  ya da süresiz de seçilebilir. Süresi biten adres bir dakika içinde, elle
  kaldırılmış gibi kaldırılır; süreyi uzatmak tek tıktır.
- **Sunucu henüz açılırken kapatma:** kaldırılan adres güvenlik duvarı
  süzgecinden hemen çıkarılır; geri kalanı sunucu hazır olunca gelir ve ekran
  bunu söyler.
- **`nft flush ruleset` sonrasında** Ajan panelin güvenlik duvarını yaklaşık
  15 saniye içinde kayıtlı kurallardan geri kurar; önce veritabanı süzgeci
  gelir. Açılışta geri yükleme kapatılmışsa ya da nftables maskelenmişse
  hiçbir şeye dokunmaz.
- **Veritabanları sayfasında yöneticiye ayrı bir bölüm:** sunucu anahtarı ve
  her motor için hâlâ eksik olan şey, yanında yapılacak adımla. Bu anahtar
  ile Sunulanlar sekmesi aynı kayıtlı sunumu yazar.
- Müşterinin gördüğü ekran sorunu söyler ama sunucunun ayrıntılarını
  göstermez (docs/DATABASE-REMOTE-ACCESS.md).

## Planlar ve haklar

- **Her sunum okunduğu yere yazılıyor.** Kullanıcılar → Sunulanlar'da
  "Veritabanına dışarıdan bağlanma"yı (ya da nginx yönergelerini) işaretlemek
  "kaydedildi" diyor ama hiçbir şeyi değiştirmiyordu; artık kendi anahtarıyla
  aynı kayıt ve etkiyle sunuyor. Yüklenen programlar, Python sürümleri ve
  yapay zekâ uygulamaları salt okunur gösterilir, yanında Katalog yönetimine
  bağlantı vardır; güvenlik nedeniyle geri çekilmiş bir sürüm bunu söyler.
  Önceki sürümlerin yanlış yere yazdığı girdiler ilk okumada geri çekilmiş
  sunum olarak taşınır; hiçbir şey açılmaz.
- Her yanıt, öğenin denetimlerin okuduğu hâlini taşır; ekran istenenden
  farklıysa bunu söyler.
- İzinler her haklar ekranında aynı başlıklar altında, okuyanın dilinde
  sıralanmış olarak durur.
- "Müşteri değiştirebilir" yalnız müşterinin gerçekten değer seçtiği
  ayarlarda (PHP değerleri) sunulur; bölümün tek bir Kaydet düğmesi vardır,
  hatalar kendi satırının yanında görünür.
- Uygulama kataloğunda açıklamalar, indirme doğrulaması ve risk notu artık
  Türkçe (docs/PLANS-AND-RIGHTS.md §15).

## PostgreSQL ve kalıntılar

Sahibin 8 Kasım kuralı: hiçbir yerde kalıntı yok; kullanıcı verisi asla
sessizce silinmez.

- **Bir PostgreSQL sürümünü kaldırmak geride hiçbir şey bırakmıyor** (Debian
  ve Ubuntu). Paketler, sürümün istemci paketiyle birlikte tamamen
  temizlenir; kümenin yapılandırması, veri klasörü ve günlükleri de ardından
  silinir. Yeni kurulumun oluşturduğundan fazlasını içeren bir küme önce
  `/var/backups/celikpanel/postgresql/` altına arşivlenir. Bir küme
  veritabanı taşıdığı sürece kaldırma yine reddedilir. Şimdiye dek eski
  sürümün ayarları kalıyor, yeni kurulan sürüm de bu yüzden 5433 portunu
  alıyordu (Boston, 3 Kasım).
- **Önceki sürümlerden kalan kümeler:** yalnız yeni kurulumun oluşturduğunu
  içeren küme, panel açıldıktan sonra bir kez kendiliğinden kaldırılır;
  kaydı tutulur ve yalnız root'un okuyabildiği bir arşiv 30 gün saklanır
  (`…-auto.tar.gz`). Diğerleri "Kullanılmıyor – eski sürüm N" olarak, neden
  korunduğuyla ve **Arşivle ve kaldır** düğmesiyle gösterilir (panelin kendi
  onay penceresi, sunucunun adımları, sonunda arşivin adı). Panelin üzerinde
  veritabanı kaydı tuttuğu bir küme asla kaldırılmaz.
- **Ayarlar yalnız çalışan kümelerde düzenleniyor.** Birden çok küme
  çalışıyorsa PostgreSQL sayfasında yönetici, düzenleyicilerin hangi
  kümenin `postgresql.conf` ve `pg_hba.conf` dosyasını açacağını seçer;
  kaldırılmış sürümün dosyaları artık listelenmez.
- **Reddedilen yapılandırma kaydı nedenini en üstte söylüyor.** Uyarı,
  kuralı (ekrandaki numarası ve satırıyla) ya da ayarı adıyla söyler, nedeni
  ve çözümü düz sözlerle verir, PostgreSQL'in kendi iletisini altında tutar,
  sunucudaki dosyanın değişmediğini söyler; "N. kuralı göster" işaretli
  satıra götürür. MariaDB ayarlarında da aynısı.
- **Posta ve Node.js kaldırmaları CelikPanel'in kendi dosyalarını da
  götürüyor:** Dovecot'un kota uyarısı zamanlayıcısı ve ek ayar dosyaları,
  Postfix'teki CelikPanel eşlemeleri (aktarma parolası dosyası dahil) ve
  dizinleri, postfwd'nin ek ayarı, kural dosyası, listeleri ve kayıtlı
  sayaçları, son sürümden sonra boş kalan Node.js klasörü. Bir dosya yalnız
  hâlâ CelikPanel'inse silinir; sahibin değiştirdiği dosya korunur ve adı
  söylenir (docs/LEFTOVERS-AUDIT.md).

## Sunucu ve güvenlik

- **SSH parola girişi**, sshd'nin gerçekte nasıl karar verdiğiyle gösterilir
  (kazanan dosya ve satır, örneğin cloud-init'in `50-cloud-init.conf`
  dosyası) ve yönetici bunu Güvenlik sayfasından kapatabilir. CelikPanel
  yalnız kendi `00-celikpanel.conf` dosyasını yazar, `sshd -t` ve `sshd -T`
  ile denetler, yeniden yükler (açık oturum asla kesilmez) ve anahtarı olan
  bir yönetici hesabı yoksa reddeder. Onay penceresi oturumu açık tutmayı ve
  önce anahtarla girişi denemeyi söyler; paneli kullanmadan geri almanın
  komutu da gösterilir.
- **Panel TLS denetimi:** geçerli bir Let's Encrypt sertifikası başarısız
  görünüyordu, çünkü denetim ad vermeden soruyor ve açılış sertifikasını
  alıyordu. Artık sertifikanın kendi adıyla soruyor, kapsadığı adları
  listeliyor ve panelin açıldığı adresi kapsayıp kapsamadığını söylüyor.
- **Güvenlik güncellemeleri kartı:** güncelleme kaynakları ham kalıplar
  yerine düz sözlerle ("Debian'dan güvenlik güncellemeleri"); yeniden
  başlatma bekleyen bir güncelleme bağlantısıyla birlikte söylenir.
- **İzleme:** Debian ve Ubuntu'da yanlış "Postfix çalışmıyor" uyarısı yok
  (canlı okuma artık hizmetin gerçekten çalıştığını gösteren birimi okuyor;
  WireGuard gibi tek seferlik birimler için de aynısı). Yalnız eski bir
  taramanın anlattığı hizmet "denetlenmedi" sayılır, şu anki sorun sayılmaz.
- **Tarih ve saat her yerde arayüz dilinde** (tek biçimleyici; dil verilmeden
  biçimlenen bir tarihi test reddeder).

## Simgeler

- Bileşenler sayfasındaki her bileşenin kendi simgesi var; hiçbiri başka
  bileşenle paylaşılmıyor.
- PostgreSQL ve MariaDB, motor renklerinde kendi çizdiğimiz fil ve deniz
  aslanıyla gösteriliyor (projelerin logoları değil); Veritabanları
  sayfasındaki sunucu kartlarında ve Bileşenler sayfasında. Emoji yok.

## Laboratuvarda bulunan düzeltmeler

- **Güncellemenin kendiliğinden geri alınması artık veritabanına dışarıdan
  bağlanmayı beklemiyor.** Bilerek başarısız bırakılan bir 129 → 130
  güncellemesinde ilk geri alma denemesi reddedildi; çünkü yeni panelin
  açılışta dışarıdan bağlanma ayarlarını uygulaması bir hizmet kilidi
  tutuyordu (panel yaklaşık 80 saniye kullanılamadı). Panel artık açılışta
  yalnız Ajan'ın en son uyguladığını okuyor, aynıysa hiçbir şey göndermiyor;
  açılıştan sonraki 3 dakika boyunca ve sunucu meşgulken tam uygulama
  yapılmıyor. Süresi biten ya da geri çekilen bir adres yine hemen, kilit
  gerektirmeden CelikPanel'in kendi güvenlik duvarı süzgecinden çıkarılarak
  kapatılıyor.
- **Kendiliğinden kalıntı temizliği kaldırılan PostgreSQL sürümünün
  paketlerini de temizliyor.** Boston benzeri bir Ubuntu sunucusunda eski
  kümenin dosyaları gitmişti ama `dpkg` hâlâ `rc postgresql-16` ve kurulu
  bir `postgresql-client-16` gösteriyordu. Aynı iş artık bunları her zamanki
  güvenli kaldırma yoluyla temizliyor (başka bir şeyi de kaldıracak bir
  benzetim işi reddeder; başka bir paketin hâlâ ihtiyaç duyduğu istemci
  kalır ve adı söylenir) ve kaydını tutuyor. İş panel açıldıktan 10 dakika
  sonra ve yalnız sunucu sakinken çalışıyor; böylece bir güncellemeyi ya da
  geri almayı asla geciktirmiyor.
- **Tek bir PostgreSQL sürümü yeniden kaldırılabiliyor.** Tek sürümü
  kaldırmak reddediliyordu ("paket kaldırılabilir bir sürüm adı
  taşımıyor"); iki sürüm kuruluyken PostgreSQL sayfasındaki her küme
  satırında artık "PostgreSQL N'yi kaldır" var.
- **PostgreSQL'i tamamen kaldırmak geride hiçbir şey bırakmıyor:** `rc`
  durumunda kalan paketler, her sürümün istemci ve belge paketleri,
  `postgresql-common` ve `postgresql-client-common`, boş PostgreSQL
  klasörleri, başka bir şey ihtiyaç duymuyorsa `libpq5` ve hiçbir PGDG
  paketi kalmadıysa CelikPanel'in PGDG kaynağı, anahtarı ve önceliği.
  Veri taşıyor olabilecek kalıntı küme kaldırmayı durdurur ve adı söylenir.
- **Sahibin dosyaları tam temizlemeden sağ çıkıyor** (Debian ve Ubuntu).
  Temizlemeden önce Ajan, kaldırılan hiçbir paketin sahibi olmadığı
  dosyaları (örneğin sahibin yazdığı bir systemd ek ayarı) ve değiştirilmiş
  paket yapılandırma dosyalarını kenara kopyalar; ardından silinen sahip
  dosyası sahibi ve izinleriyle geri konur, değiştirilmiş yapılandırma
  dosyaları `/var/backups/celikpanel/kept/` altında saklanır. Yanıt bunları
  adıyla söyler. Kalıntı küme kaldırılınca Bileşenler hemen yeniden okunur.
- **Katılmış BIND sunucusunda SSH parola girişi kapatılabiliyor.**
  CelikPanel'in kendi `Match User celikpeer` bloğu (yalnız DNS eş hesabı
  için parola girişini kapatır) değişikliği engelliyordu; artık yalnız
  parola girişini açabilecek bir Match bloğu engeller. CelikPanel'in ek
  ayar dosyası artık yalnız root'a ait; böylece durum okunuyor ve "yeniden
  aç" çalışıyor (önceden yalnız elle geri alınabiliyordu).
- **Posta programları için "Şimdi ayarla" çalışıyor.** Posta kutusu
  ekranındaki düğme her alan adı için "404 page not found" yanıtı alıyordu;
  bu yüzden eksik autoconfig ve Autodiscover kayıtları ve var olan bir alan
  adının site dosyası panelden eklenemiyordu. İstek artık doğru yere
  ulaşıyor; diğer posta değişiklikleriyle aynı posta iznine bağlı.
- **MariaDB'nin ad çözmesi yeniden başlatmadan sonra da kapalı kalıyor.**
  Son dış kural kaldırılıp MariaDB yeniden başlamadan yeni bir kural
  eklendiğinde CelikPanel'in seçenek dosyası onaylanan `skip-name-resolve`
  satırı olmadan yazılıyordu; yeniden başlatmadan sonra bütün MariaDB
  kuralları yeniden bekliyordu. Yönetici onay verdiyse satır artık her
  zaman yazılıyor; sahibin kendi ayarının üzerine yine asla yazılmıyor.
- **Debian 13'te de yanlış "Postfix çalışmıyor" uyarısı yok.** Debian
  13'ün Postfix'i doğrudan `postfix.service` içinde çalışır; Ubuntu'nunki
  ise bir sarmalayıcının arkasındaki `postfix@-` biriminde. İlk düzeltme
  yalnız bu birimi okuduğu için Debian 13'te uyarı veriyordu. İzleme artık
  ikisini de okuyor ve sarmalayıcı kendisi çalışıyorsa onun okumasını
  alıyor.

## Laboratuvar sonuçları

_Laboratuvar turundan sonra doldurulacak._

## Bilinen sınırlar

- **Bu sürümde yapılmadı (yalnız tasarım notu):**
  - DNS üzerinden joker sertifika (`*.alanadi`): yenileme paneli gerektirmeden
    çalışmalı; doğrulama kaydını panelin bölgesine yazmak bölge kayıtlarının
    tutarlılık kanıtını ve ikincil sunucuların aktarımını bozar. Önce sahibin
    kararı gerekiyor. Sertifika isteği yolu (alpha.128) bugün de herhangi bir
    sertifika sağlayıcısından joker sertifika almayı sağlıyor
    (docs/WILDCARD-CERTIFICATE-DESIGN.md);
  - DKIM anahtarı değişimi, Dovecot bağlantı sınırları ve müşterinin alan
    adına özel gönderen listesi (docs/MAIL-SERVER-SETTINGS-DESIGN.md);
  - yalnız bir alan adı için aktarma sunucusu (docs/MAIL-RELAY.md).
- **Posta programları:** alt alan adları ayarlanmaz; bir site yanıtları bir
  sonraki kaydında (ya da "Şimdi ayarla" ile) alır; sahibin koruduğu site
  dosyası asla değiştirilmez.
- **Aktarma:** Postfix'in SASL oturum açma modülüne ihtiyacı var
  (`libsasl2-modules`, Arch'ta `libsasl`); yoksa kayıt komutuyla birlikte
  reddedilir, hiçbir şey kurulmaz.
- **Posta kutusu kurtarma:** kutu kullanıcıları kendi kurtarma adreslerini
  giremez (panele girişleri yok); Roundcube giriş sayfasında "parolamı
  unuttum" bağlantısı yok; gönderimi elle durduran bir ekran henüz yok (kural
  yine de geçerli).
- **Veritabanına dışarıdan bağlanma:**
  - motor ağda dinlemeye ve MariaDB'nin ad çözmesi kapanmaya ancak yönetici
    motoru yeniden başlattığında başlar;
  - MariaDB'de ad çözmeyi kapatmak yöneticinin onayını ister (127.0.0.1'e
    `user@localhost` olarak bağlanan programlar artık eşleşmez); o zamana dek
    MariaDB kuralları bekler;
  - PostgreSQL Fail2ban katmanı yalnız paketin varsayılan `log_line_prefix`
    değeriyle, `logging_collector` olmadan ve İngilizce iletilerle çalışır;
  - süre bitimini panel uygular: panel durduğu sürece süresi biten adres
    izinli kalır (motorlar parolayı yine denetler);
  - sunucu açılışını bitirmemişken bir kaldırma adresi yalnız süzgeçte
    kapatır;
  - `nft flush ruleset` sonrasında yalnız canlı açılmış ("Yeniden başlatma
    için kaydet" yapılmamış) güvenlik duvarı geri kurulmaz;
  - izni geri çekmek erişimi bir sonraki uygulamada kapatır (yaklaşık 35
    saniyeye kadar); MariaDB yalnız tekil IPv6 adresi kabul eder; uzak
    MariaDB hesabına elle eklenen tablo, sütun ya da rol yetkileri
    kaldırılmaz; MariaDB günlüğünü journal'a yazmalıdır.
- **Karar bekleyen kalıntılar (docs/LEFTOVERS-AUDIT.md):** MariaDB
  kaldırılınca `/var/lib/mysql` ve `/etc/mysql` kalır (paketler `rc`
  durumunda); Roundcube kaldırılınca veritabanı (kullanıcıların kişileri ve
  ayarları) arşivlenmeden silinir; WireGuard anahtarları ve eşleri
  `/etc/wireguard/wg0.conf` içinde kalır; posta verisi, posta kutusu parola
  özetleri ve DKIM anahtarları kalır; Arch'ta siteler `mysqli` isteyebileceği
  için phpMyAdmin'in `celikpanel-phpmyadmin.ini` dosyası ve
  `/var/lib/postgres/data` kalır; işaretsiz genel Postfix eşlemeleri, ortak
  posta sertifikası kopyaları ve eski DNS motorunun dosyaları kalır. Her
  biri için önce sahibin kararı gerekiyor.
- **Sahip dosyaları ve tam temizleme:** sahibin dosyalarını temizleme
  sırasında korumak yalnız apt ile (Debian ve Ubuntu) çalışır, Arch'ın
  pacman'ında çalışmaz. Panelin yazdığı genel Postfix eşlemeleri
  (`vmailbox`, `virtual`, `vmailbox_domains`) CelikPanel işareti taşımadığı
  için sahip dosyası sayılır ve geri konur.
- **Kalıntı PostgreSQL kümeleri:** bir küme yalnız dosyalarına bakılarak
  "yeni" sayılır; `postgres` ya da `template1` içinde oluşturulan işlev,
  görünüm ya da açıklamalar bu yolla görülemez, bu yüzden kendiliğinden
  kaldırma 30 günlük bir arşiv saklar. Okumanın karar veremediği hiçbir şey
  kendiliğinden kaldırılmaz. Arch tek kümesini eskisi gibi korur.
- **Bileşenler:** Bileşenler taraması Debian ve Ubuntu'da Postfix'i hâlâ
  sarmalayıcı biriminden değerlendiriyor; İzleme ve "Şu anki sorunlar"
  gerçek birimi okuyor.
- **Haklar:** veritabanına dışarıdan bağlanma kendi anahtarında açık ama
  Sunulanlar sekmesinde gizliyse (seyrek bir durum), artık kapalı okunur
  (müşteriler için zaten öyleydi); yönetici tek tıkla yeniden açar.
- Yeni özelliklerin hiçbiri henüz gerçek bir sistemde çalıştırılmadı;
  laboratuvar adımları docs/A130-OPS-LAB.md ve özellik belgelerinde.
- Önceki sınırlar geçerliliğini koruyor (bkz. alpha.129).

## Kanıt

Geçişler (hepsi eklemeli):

- **075 `database_remote_rule_expiry`:** `database_remote_rules` üzerinde
  `expires_at` (var olan her kural süresiz kalır).
- **076 `mailbox_recovery`:** `mailbox_recovery_addresses` ve
  `mailbox_recovery_links` tabloları.
- **077 `mail_relay`:** `mail_relay_settings`, `mail_server_options` ve
  `mail_sender_rules` tabloları.

Her geçişte olduğu gibi alpha.129 bu geçişleri içeren bir veritabanını
reddeder; geri dönmek, güncellemeden önce alınan anlık görüntüyü geri
yüklemek demektir. Sürüm sırası 129 → 130.
