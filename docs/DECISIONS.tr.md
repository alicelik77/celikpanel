# Stratejik Kararlar

*Proje kaydı · [English](DECISIONS.md)*

Büyük yön kararlarının **neden**inin kalıcı kaydı — soru her yeniden
gündeme geldiğinde sıfırdan türetmek istemediğimiz gerekçeler. Kod kararları
git'te yaşar; bu dosya strateji içindir. En yeni en üstte.

> **Tarihler üzerine düzeltme notu (9 Ekim 2026'da kaydedildi).** D-029'un tarih satırı
> ("10 Ekim 2026") ve içindeki "2026-10-10 kaydı" atfı takvim tarihi değildir. D-029,
> 2026-10-09 günü saat 05:57'de işlendi (`faa5ef085`, yerel saat, UTC+3). İş turları,
> saatten okunmadan her turda bir artırılarak 2026-10-10, 2026-10-11 ve 2026-10-12
> diye etiketlendi; turların hepsi 2026-10-09 günü işlendi (2026-10-10: 00:45-05:57;
> 2026-10-11: 08:05-09:12; 2026-10-12: 09:12-13:06). Kayıtlar birbirine bu etiketlerle
> atıf yaptığı için etiketler kalır. Eşleşme, işlemler ve aynı etiketleri taşıyan üç kanıt
> dizini (`set1-20261010`, `set2-20261011`, `set3-20261012`; 2026-10-08 ve 2026-10-09
> günlerinde çalıştılar) [OPERATION-GUIDANCE.tr.md](OPERATION-GUIDANCE.tr.md) başındaki
> düzeltme notundadır. Bu dosyada etkilenen tek kayıt D-029'dur. Bu nottan sonra yazılan
> kayıtlar saat tarihini kullanır; D-029 dışında 10, 11 veya 12 Ekim 2026 tarihli bir
> kayıt gerçektir.

## D-040 · Web sitesi sertifikasını Panel değil, işletim sisteminin certbot'u yeniler

*30 Ekim 2026 (saat tarihi) · Kaynakta bulunan bir ilke ihlalinin düzeltmesi; `fix/site-cert-native-renewal` dalında kaynakta, bileşen testli ve yerel bir test CA'ya karşı atılabilir bir chroot denemesi; yayımlanmadı, kurulu sunucuda ölçülmedi. Ayrıntı: [D-040 (İngilizce)](DECISIONS.md)*

**Yanlış olan.** D-022 ve anayasa (§2), panel kesintisinin, lisans kaybının ya da
kaldırılmasının sertifika yenilemesini durdurmamasını ister. Her ACME web sitesi
sertifikası CelikPanel'in yalıtılmış certbot deposunda tutuluyor, nginx'in
adlandırdığı değişmez bir kopyaya (`/etc/ssl/celikpanel/<alan>/sha256-…/`) alınıyor ve
yalnız çalışan Panel'in 12 saatlik döngüsüyle yenileniyordu. İşletim sisteminin
`certbot.timer`'ı bu lineage'ları görmüyordu. Panel durursa ya da kaldırılırsa her site
sertifikası en geç 90 günde bitiyordu; başarısız yenileme yalnız veritabanına
`failed` yazıyor, kimseye söylenmiyordu.

**Karar.** Bu sürümün verdiği her web sitesi sertifikası standart depoda
(`/etc/letsencrypt`) kendi lineage'ı olur (`celikpanel-site-<alan kimliği>-<24 hex>`),
webroot doğrulaması sitenin root'a ait doğrulama kökünden yapılır; yenileme
yapılandırmasını certbot kendisi yazar. nginx certbot'un live dosyalarını adlandırır;
yenilemeyi işletim sisteminin zamanlayıcısı yapar; CelikPanel programı gerektirmeyen
düz sh dağıtım kancası
(`/etc/letsencrypt/renewal-hooks/deploy/celikpanel-site-certificates`) `nginx -t`
sonrası nginx'i yeniden yükler ve sonucu kaydeder. Kanca yalnız yoksa yazılır;
sahibin değiştirdiği kanca korunur ve sayfada söylenir. EAB bilgileri komut satırına
ve yenileme yapılandırmasına girmez (0600 geçici `--config` dosyası). Panel yerel bir
lineage'ı asla yenilemez; yalnız okur ve kaydeder; sertifika kartı nedeni, kimin
yapacağını (sunucu yöneticisi), somut sonraki adımı ve nasıl devam edileceğini
söyler; Pano her sorunu gün sayısından bağımsız listeler. Önceki sürümlerin
sertifikaları yeniden alınana kadar çalışan Panel tarafından eskisi gibi yenilenir;
kart bunu ve bir kez yeniden almanın onları işletim sistemine taşıdığını söyler.

**Posta SNI.** Site sertifikası `mail.<alan>` için de kullanılıyorsa SNI girdisi
certbot'un live dosyalarını adlandırır; kanca CelikPanel'in Postfix SNI haritasını
diskteki hâliyle yeniden derler (`postmap -F`) ve Postfix ile Dovecot'u yeniden
yükler; yapılandırma yazmaz; sahip haritayı başka yere yönelttiyse dokunmaz. Posta
sunucusu sertifikasının kendi bağımsız yenilemesi ayrı lineage'larla sürer.

**Açık kalanlar.** Systemd ile açılmış bir sistemde zamanlayıcının
kendisi, Ubuntu (snap), Arch ve Dovecot 2.3, gerçek bir CA, yeniden başlatma ölçülmedi;
önceki sürüme dönüşte o sürüm yerel lineage adlandıran posta TLS planını okuyamaz
(Postfix ve Dovecot sunmaya devam eder, posta TLS değişikliği ileri dönene kadar
yapılamaz). Şema
değişikliği yok (46). P0.5 yalnız web siteleri için ilerledi; kısmi kalır.

### Ek (30 Ekim 2026, saat tarihi): önceki sürümün sertifikası bir sonraki yenilemesinde taşınır; Dovecot 2.3, Ubuntu 24.04 ve Arch ölçüldü

*`release/alpha113`'ten `fix/cert-mail-known-gaps` dalı; yayımlanmadı; kurulu sunucuda
ölçülmedi.* Önceki sürümün sertifikası artık yalnız yeniden alınana kadar çalışan
Panel'ce yenilenmez: bir sonraki yenilemesinde Panel onu, Yeniden al'ın kullandığı aynı
Ajan çağrısıyla yerel bir lineage'a taşır (`cmd/panel/site_cert_legacy_move.go`); eski
sertifika, yenisi etkin olup nginx'in denenmiş yapılandırması onu adlandırana kadar
sunulur. Başarısız taşıma `move_failed_<tür>` kaydeder, her çalışmada yeniden denenir;
bitimine 14 gün kala eski yolla yenilenir ve hata kartta kalır. Yanıt gelmezse hiçbir
şey kaydedilmez (bilinmiyor). EAB isteyen sağlayıcıların sertifikası kendiliğinden
taşınmaz (bilgiler saklanmaz); kart "bir kez yeniden alın" der. Dağıtım yapılandırma
matrisi gerçek Dovecot ve Postfix'te her SNI adının sertifikasını, kancalı yenilemeden
sonra yenisini denetler; Arch Dovecot ve MariaDB artık "uygulanamaz" değil, ölçülür.
Ubuntu 24.04 ve Arch, QEMU'da gerçek açılışla ölçüldü. Kanıt:
[cert-mail-20261030](../deploy/e2e/dns-pair-acceptance/evidence/cert-mail-20261030/).
P0.5 kısmi kalır.

---

### Değişiklik (26 Ekim 2026, saat tarihi): web postası kendi adresinde, sunucunun kendi web sunucusuyla; v0.1.0-alpha.110 adayı için kaynakta uygulandı

Sahip 26 Ekim 2026'da onayladı: web postası panelin kökeninde çalışmamalı; orada
Roundcube'da çalışan bir betik (geçmiş XSS açıkları CVE-2023-5631, 2024-37383,
2024-42009) oturum açmış yönetici olarak `/api/v1` çağırabilir. alpha.109
`/webmail/`'i panelin sıkı kuralının arkasında tuttu; Roundcube'un arayüzü orada
başlayamadı. Web postası kendi adresini alır ve sunucunun kendi nginx'i sunar;
böylece panel durduğunda da yanıt verir (D-022). Adlar için sahibin sözleri:
"https://mail.celikhost.com/ diyeyim … frankfurt ya da boston diye prefixleri
kullanmak istemiyorsam". v0.1.0-alpha.110 adayı için kaynakta (yayımlanmadı; kurulu
bir sunucuda hiçbir şey ölçülmedi).

1. **Sunucu başına, yöneticinin seçtiği adres.** Herhangi bir ad (varsayılan öneri:
   Sunucu kurulumundaki posta sunucusu adı; panelin kendi adı asla, çünkü tarayıcı
   panelin oturum çerezini aynı adın her portuna gönderir). `panel_settings`
   içinde (`webmail_address`); şema değişikliği yok.
2. **Önce salt okunur denetim.** Genel DNS (sabit genel çözümleyicilerin hepsi)
   yalnız bu sunucunun adreslerini yanıtlamalı. Henüz değilse: eklenecek kaydın
   kendisi; bölge bu panelde ise onaydan sonra bölgenin kendi kayıt düzenleyicisiyle
   eklenir. Başka yeri gösteriyorsa: beklenen ve gerçek adreslerle reddedilir;
   diğer DNS sunucusuysa adıyla söylenir. DNS'e sorulamadıysa bilinmez. Güvenlik
   duvarında 80 ve 443 kapalıysa nerede açılacağı söylenir; CelikPanel portları
   kendiliğinden asla açmaz.
3. **Tek onaylı işlem** (`webmail_address`, kalıcı, sunucu yeniden denetler): ad
   için certbot sertifikası (nginx üzerinden 80 portunda HTTP-01, panelin denetim
   kökü, posta sertifikasıyla aynı kurallar; işletim sisteminin certbot
   zamanlayıcısı yeniler, nginx'i certbot'un kendi kancası yeniden yükler); tek
   yerel dosya `/etc/nginx/conf.d/celikpanel-webmail-origin-<ad>.conf` (80: denetim
   ve HTTPS'e yönlendirme; 443: Roundcube'un genel ağacı, web postasının bağlı
   PHP-FPM soketi, kendi kuralı `frame-ancestors 'self'`, HSTS, nosniff,
   Roundcube'un satır içi başlangıç betiği o kökende serbest; CelikPanel'in PHP
   denetimi yalnız bu sunucudan); `nginx -t`, yeniden yükleme, ad üzerinden HTTPS
   kanıtı (sunulan sertifika, PHP sürümü ve uzantıları, kuralıyla giriş sayfası,
   text/css stil dosyası). Her hatada dosya eski haline döner, sertifika kalır.
   Adı zaten yanıtlayan ya da 80/443'te açık bir genel adresi dinleyen başka bir
   sunucu bloğu dosyasıyla reddedilir; posta sertifikasının kendi 80 sunucusu
   paylaşılabilir (o zaman `http://` 404 verir, söylenir). Sahibin değiştirdiği
   dosyalar asla üzerine yazılmaz ya da silinmez.
4. **Roundcube'un kendi ayarları** (`use_https`, `session_samesite` Strict)
   yalnız CelikPanel biçimindeki yapılandırmasına eklenir; sahibin değiştirdiği
   yapılandırma olduğu gibi kalır ve söylenir. IMAP ve SMTP yerelde kalır.
5. **Panelin `/webmail/` yolu** `https://<adres>/`'e yönlendirir (302); adres
   yoksa panelin sıkı kuralı altında, adresin nerede ayarlandığını söyleyen kısa
   bir sayfadır. Roundcube vekili panelin kökeninden kaldırıldı. Bağlantılar yalnız
   adrese döner; posta kutusu kullanıcılarının panel hesabı gerekmez.
6. **Değiştirme ve kaldırma.** Ad değişince yenisi kurulur, eskisinin dosyası
   yalnız yenisi kanıtlanınca kaldırılır. Kaldırma ayrı onaylı bir işlemdir;
   yönetici istemedikçe sertifika kalır. Roundcube kaldırılınca adres dosyaları da
   kaldırılır (sertifikalar kalır).

**Yapılmadı (sonraki adımlar, kayıtlı).** Birden çok sunucudaki posta kutuları için
tek adres (Roundcube'un alan adına göre IMAP yönlendirmesi); müşterilerin kendi
`webmail.<alanadı>` adları. Eş sunucunun kendi etkin adresi okunmaz (bunun için
uzak API çağrılmaz); denetim DNS yanıtını ve bilinen eş adresini karşılaştırır.

**Kanıt.** Bileşen ve bağlanmış testler (Agent, Panel, web); saklanan temel
imajdan atılabilir bir Debian 13 sanal makinesinde, Let's Encrypt yerine atılabilir
bir test CA'sıyla tek gerçek sistem koşusu
([deploy/e2e/d038/alpha110-debian13-20261026](../deploy/e2e/d038/alpha110-debian13-20261026/README.md)).
Kurulu bir sunucuda ölçülmedi. Bu değişiklik hiçbir P0 işini kapatmaz.

---

## D-039 · Bir başarılı lisans kontrolü yönetimi çevrimdışı en çok yedi gün açık tutar; kullanımdaki panel yaklaşık saatte bir kontrol eder

*30 Ekim 2026 (saat tarihi) · Sahibin 2026-10-29 onayı (lisans doğrulaması çevrimdışı yaklaşık 7 gün geçerli, yaklaşık saatte bir kontrol: "ok") · Kaynak değişikliği `feat/license-offline-grace` dalında; yayımlanmadı, kurulu panel güncellenmedi*

**Neden.** #134'ten (v0.1.0-alpha.64) beri hizmet bir dakikalık süre imzalıyor,
panel de her kaydı `issued_at + 60 sn` ile sınırlıyordu. celikpanel.net'in bir
dakikadan uzun tek bir kesintisi bu yüzden kurulu bütün panellerin yönetim
ekranlarını aynı anda kapatıyordu (siteler, e-posta ve DNS çalışmaya devam
ediyordu). Bu bütün müşteriler için tek hata noktasıdır ve D-022'deki sahip
bağımsızlığıyla çelişir.

**Karar.**
- Üyelik hizmeti `refresh_after = şimdi + 3600` ve
  `offline_until = min(şimdi + 7 gün, yıllık bitiş)` imzalar; v1 biçimi yedi güne
  zaten izin veriyordu, biçim değişmez.
- Panel `min(offline_until, issued_at + 7 gün, bitiş)` sınırını uygular; `refresh_after`
  ile bir saatin erken olanında, sunucuya özgü belirlenimci sapmayla beşte bire kadar
  erken kontrol eder. Başarısızlıkta 1→30 dakika bekler (o da sapmalı), hiçbir zaman
  kaydın geçersizleşeceği andan sonra değil; geçersizleşince kullanımda 30 saniyede
  bir. Hizmete yalnız kimliği doğrulanmış kullanım ya da "Tekrar kontrol et" ulaşır;
  boştaki sunucuda zamanlayıcı yoktur.
- Zamanı gelen kontrol, kayıt geçerliyken en çok 3 sn bekler (tek ortak yenileme);
  böylece erişilebilen hizmetin açık reddi (kaldırma, taşıma ya da anahtar
  değiştirme sonrası `invalid_activation`; `invalid_license`, `license_in_use`,
  `license_expired`) yönetimi onu saptayan istekte kapatır; erişilemeyen hizmet her
  bekleme aralığında en çok bir isteği 3 sn geciktirir.
- Saat güvenliği: süre hizmetin `issued_at` değerine bağlıdır; ondan 5 dakikadan
  fazla önceki, kalıcı yerel üst işaret `seen_at`'ın 5 dakikadan fazla altındaki ya
  da çalışırken geri alınan saat (duvar saati ile monoton zaman) bir kontrol başarılı
  olana kadar `verification_unavailable` / `clock_behind` verir.
- D-024 yönlendirmesi: lisans sayfası ve Pano "lisans hizmetine ulaşılamıyor;
  yapmanız gereken bir şey yok; … tarihine kadar çalışır" der, son 48 saatte kimin ne
  yapacağını söyler (sunucu yöneticisi: celikpanel.net'e giden HTTPS'e izin verip
  Tekrar kontrol et), süre dolarsa kurtarma ekranı nedeni adıyla söyler.

**Uyumluluk.** Eski paneller (alpha.64 … bu sürüm) yedi günlük kaydı kabul eder ve
kendi 60 saniyelik sınırlarını ve 45 saniyelik yenilemelerini korur; eski hizmetin
60 saniyelik kaydını alan yeni panel tam o dakikayı uygular. alpha.64 öncesi
paneller imzalı süreyi zaten uyguluyordu. Yerel `seen_at` alanını eski paneller
geri dönüşte yok sayar.

**Dayanıklılık sözleşmesi (D-025).** Etkilenen: erişim kapısı ve kalıcı lisans
kanıtı (`license.json` isteğe bağlı yerel `seen_at` alanını kazanır; imzalı içerik
değişmez). Kurtarma: süre dolması ya da saat sorunu bir sonraki başarılı kontrolle,
kullanımda ya da "Tekrar kontrol et" ile düzelir; doğrulanmış yanıt olmadan hiçbir
şey yeniden yazılmaz. Kanıt: bileşen testleri (licensing, cmd/panel, web, PHP hizmeti
ve HTTP akışı, PHP↔Go birlikte çalışma, değiştirilmemiş alpha.112 licensing paketinin
yeni hizmet kaydıyla denenmesi). Kalan açıklar: gerçek bir kurulumda çok günlük
kesinti denemesi yapılmadı; sunucuda root olan kişi her yerel kontrolü yine aşabilir
(eski dosyayı geri koyup saati geri almak), önceden olduğu gibi; üyelik hizmeti
değişikliğini sahip yayımlar.

## D-038 · Sürüm, kullanım ve güvenlik her bileşen için tek ilkeye bağlıdır: sahibi seçer, panel önerir ve uyarır, kullanımda olan kaldırılmaz, hiçbir şey sessizce taşınmaz, yamalar işletim sisteminin işidir

*24 Ekim 2026 (saat tarihi) · Sahibin 24 Ekim 2026 yönü, aynı gün onaylandı ("onaylıyorum") · Bir ilkedir, teslim edilmiş bir yeteneğin beyanı değildir: sondaki "Bugün ne var" bölümüne bakın*

24 Ekim 2026'da, Frankfurt'ta PHP-FPM 8.4'ün kaldırma penceresine
(v0.1.0-alpha.106) bakarken sahip şöyle yazdı: "müşterilerin websiteleri de
farklı versiyonlarda php kullanıyor olabilirler. Bunu da kontrol etmelisin.
belki de müşteri illa o sürümü kullanacaktır. php 8.4 zorunlu iken php 8.5'e
zorlamak olmaz. fakat güvenlik yamalarını zorlamalı mıyız. tüm sunucuyu riske
sokar mı" Ardından: "php dışında da diğer componentler için de geçerli bu
düşünceler, planlar. program özgü çalışmıyoruz. prensip oluşturmaya
çalışıyoruz." Sahip, boşluklar için ikinci bir gözden geçirmeden sonra aşağıdaki
on yedi maddeyi onayladı.

Bu kayıt, alpha.100 ve alpha.104–alpha.106 işlerinde (D-037 değişiklikleri)
PHP ve panelin kendi PHP araçları için verilen kararları her bileşen için tek
ilkeye dönüştürür: web sunucuları, PHP, Node.js, veritabanları, önbellekler,
posta, DNS, güvenlik araçları ve panelin kendi araçları için aynı biçimde.
D-002'yi (dürüst sürümler), D-022'yi (sahibin denetimindeki altyapı), D-024'ü
(uygulanabilir yönlendirme), D-034'ü (paketler önerir, dayatmaz) ve D-036'yı
(gösterilen sürüm paket veritabanının yanıtıdır) genişletir.

**İlke.**

1. **Bir şeyin sahibi sürümünü seçer.** Bir sitenin, veritabanının ya da
   uygulamanın hangi sürümle çalışacağına onun sahibi karar verir. Panel önerir
   ve uyarır; hiçbir zaman sürüm dayatmaz ve kimsenin işini kendiliğinden başka
   bir sürüme taşımaz.
2. **Kullanımda olan kaldırılmaz.** Bir bileşen ya da sürüm kaldırılmadan önce
   panel onu kullanan her şeyi adıyla sayar: siteler, veritabanları, posta
   kutuları, başka bileşenler ve panelin kendi araçları. Kullanan varsa
   kaldırma reddedilir. Taşıma yalnız güvenle doğrulanabilen yerde ve yalnız
   kişi isteyince önerilir; müşterinin sitesi ya da verisi asla kendiliğinden
   taşınmaz.
3. **Önce göster, sonra kanıtla, olmazsa geri al.** Her kurma, kaldırma ve
   taşıma işleminden önce panel sunucuda tam olarak neyin değişeceğini
   gösterir; beklenmeyen her şey kişinin onayını ister. İşlemden sonra panel
   sonucun çalıştığını kanıtlar; çalışmıyorsa önceki duruma döner ve bunu
   söyler. Paylaşılan bir servisi yeniden başlatan her işlem, başlamadan önce
   bunu ve beklenen kesintiyi söyler; hiçbir işlem başka bir servisi söylemeden
   yan etki olarak yeniden başlatmaz.
4. **Sürüm içindeki güvenlik yamaları işletim sisteminin işidir.** Bunlar
   sürümü değiştirmez ve uygulanmalıdır. Onları işletim sisteminin kendi
   otomatik güncellemesi, panelden bağımsız olarak uygular: panel yokken de
   sunucu yamalanmaya devam eder. Panel durumu tek yerde gösterir — otomatik
   güncelleme açık mı, bekleyen güvenlik güncellemesi var mı, en son ne zaman
   çalıştı — ve otomatik güncellemenin kapalı olmasını uyarı sayar. Panel bu
   ayarı asla sessizce kapatmaz.
5. **Desteği bitmiş sürüm açıkça işaretlenir, asla zorlanmaz.** Artık güvenlik
   düzeltmesi almayan bir sürüm göründüğü her yerde belirgin biçimde
   işaretlenir ve onu kullanan yerler uyarı alır. Böyle bir sürümü kurmak ya da
   seçmek onay ister. Taşıma kararı sahibinde kalır.
6. **Her iş kendi alanında çalışır.** Her site, veritabanı ve posta alanı
   kendi kullanıcısıyla, diğerlerinden ayrı çalışır; böylece eski sürümdeki bir
   iş diğerlerini tehlikeye atmaz. Bu ayrımın olmadığı yerde panel bunu söyler.
7. **Veriyi dönüştüren sürüm değişikliği geri alınamaz ve asla yan etki
   değildir.** Bir veritabanının ana sürüm yükseltmesi (PostgreSQL 16'dan 17'ye,
   MariaDB'de bir ana adım) diskteki veriyi dönüştürür ve çoğu zaman geri dönüş
   yoktur; Debian'da yeni PostgreSQL paketi eski kümeyi devralmaz, yanına boş
   bir küme açar. Böyle bir değişiklik asla başka bir işlemin parçası olarak
   yapılmaz: panel önce yedeğin var olup olmadığını gösterir, geri dönüş
   olmadığını açıkça söyler ve onay ister.
8. **Bir bileşeni kaldırmak verisini silmek değildir.** Bir bileşen
   kaldırıldığında veritabanları, posta kutuları ve site dosyaları yerinde
   kalır ve panel nerede olduklarını söyler. Veri silme ayrı, açıkça istenen
   bir işlemdir. Sistem yapılandırma dosyaları purge ile gider; sahip birini
   elle düzenlediyse panel bunu önceden söyler. Yeniden kurulum eski veri
   bulursa ne yapılacağını sorar.
9. **Paket yöneticisinin kurmadığını işletim sistemi yamalamaz.** Roundcube
   panel tarafından kendi paketinden kurulur; işletim sistemi onu hiç
   güncellemez. Böyle bileşenlerde güvenlik güncellemesini bildirmek ve kişinin
   onayıyla, aynı göster-ve-onayla akışıyla uygulamak panelin işidir. Panel
   hangi bileşenlerin işletim sistemi güncellemelerinin dışında kaldığını
   açıkça listeler.
10. **Yamalanmış, yeniden başlatılmış demek değildir.** OpenSSL gibi bir
    kitaplık yamalanınca onu kullanan servisler yeniden başlatılana kadar eski
    kodla çalışır; çekirdek güncellemesi yeniden başlatma ister. Panel "yeniden
    başlatma bekleyen servisler" ve "yeniden başlatma bekleyen sunucu" durumunu
    gösterir, beklenen kesintiyi söyler ve zamanı sahibine bırakır.
11. **"Desteği bitti" doğru tanımlanır.** Ubuntu 22.04'teki PHP 8.1 üreticisinin
    desteği dışındadır ama Ubuntu tarafından 2027'ye kadar yamalanır. Yanlış bir
    uyarı insanları gereksiz taşımalara iter. Tanım şudur: bu paketi sağlayan
    kim ise artık onun için güvenlik düzeltmesi göndermiyor. Bu bilgi tarihli
    bir kaynaktan gelir; bilinmediğinde panel "bilinmiyor" der, tahmin etmez.
12. **Üçüncü taraf depolar sınırlandırılır.** PHP 8.5 ve 8.6'yı getiren depo
    Frankfurt'ta sistemin temel PHP paketlerinin yerine kendininkileri koymaya
    adaydı. Bir üçüncü taraf depo yalnız onu gerektiren bileşen için açılır,
    dağıtımın öteki paketlerinin yerine geçemeyecek biçimde sabitlenir; panel
    hangi deponun kim tarafından ne zaman açıldığını ve imza anahtarının
    durumunu gösterir. Anahtar eskidiğinde güncellemeler sessizce durmaz; panel
    bunu söyler.
13. **Panel yalnız bildiğini sayar ve bunu söyler.** Sahip panel dışında bir
    şey kurabilir; bu tasarımın parçasıdır (D-022). "Hiçbir şey kullanmıyor"
    yerine panel "CelikPanel'in bildiği hiçbir şey kullanmıyor" der. Kaldırmadan
    önce elinden geldiğince sistemin kendi izlerine de bakar: o sokete işaret
    eden web sunucusu yapılandırması, bağımlı servisler, açık bağlantılar.
14. **Müşteri sunulanlar arasından seçer; seçtiği sessizce değiştirilmez.**
    Yönetici müşterilere hangi sürümlerin sunulacağına karar verebilir
    (örneğin desteği bitmişleri gizler). Müşterinin zaten seçtiği sürüm asla
    kendiliğinden değiştirilmez; uyarıyı müşteri kendi sayfasında görür.
15. **Panel "denenmiş" ile "izin verilen"i ayırır.** Panelin test edildiği
    sürümler işaretlenir; ötekiler "kurulabilir, denenmemiş" diye sunulur. Yine
    engel yok, yalnız bilgi var.
16. **Her karar kayda geçer.** Sürüm seçimi, beklenmeyen paket onayı, desteği
    bitmiş sürüm onayı, taşıma: kimin neyi ne zaman onayladığı denetim
    günlüğüne yazılır.
17. **İşletim sistemi sürüm yükseltmesi sahibinindir.** Debian 13'ten 14'e
    geçiş bütün bileşen sürümlerini birden değiştirir. Panel bunu asla
    başlatmaz; sahip yapar. Panel desteklediği işletim sistemi sürümlerini
    açıkça söyler, yükseltmeden önce neyin değişeceğini listeler ve yükseltmeden
    sonra ayakta kalır (D-001: sunucu asla yeniden imajlanmaz).

**Bugün ne var (24 Ekim 2026).** 2. ve 3. maddeler PHP sürümleri, panelin PHP
araçları ve bileşen kaldırma için büyük ölçüde yerinde (alpha.100, alpha.104,
alpha.105, alpha.106): kaldırma benzetimi ve adıyla ret, dikkate değer
paketlerin onayıyla kurulum benzetimi, kanıt ve geri almayla araç başına PHP
bağlama, PHP sürümlerinde "kim kullanıyor", önce-sonuç kaldırma penceresi. 1.,
5. ve 14. maddeler sitelerin PHP sürümleri için var. 4., 6. (havuz ve hesap
tasarımının bir sonucu olarak var, bir denetim olarak yok), 7., 8. (kısmen,
alpha.100 kaldırma metinlerinde), 9., 10., 11., 12., 13. (kısmen), 15., 16.
(kısmen, mevcut denetim günlüğüyle) ve 17. maddeler bileşenler genelinde
uygulanmış değil. Her bileşenin bu on yedi maddeye göre salt-okur denetimi bu
kaydı izler; bulunan açıklar parça parça değil tek sürümde kapatılır (sahibin
23 Ekim 2026 kuralı: sonraki sürüme bir şey bırakma).

**Bunun izin vermedikleri.** Kurulu sunucularda canlı yapılandırma yok, panelin
başlattığı işletim sistemi yükseltmesi yok, lisans politikasında değişiklik yok
ve asistanın başlattığı kurulu panel güncellemesi yok: her kurulu panel
güncellemesini sahip, panelin kendi güncelleme ekranından başlatır (AGENTS.md).

### Değişiklik (24 Ekim 2026, saat tarihi): v0.1.0-alpha.107 adayı için kaynakta uygulandı

Sahip, `main`'in (703f386da) salt-okur denetiminin bulduğu on iki açığın tek
sürümde kapatılmasını 24 Ekim 2026'da onayladı ("ok"). v0.1.0-alpha.107 adayı için
kaynakta (yayımlanmadı; kurulu bir sunucuda hiçbir şey ölçülmedi). Madde madde,
şimdi ne var ve ne kalıyor:

1. **Sahibi seçer.** Bir üretici deposu açıkken (Sury PHP, PGDG) yalın bir kurulum,
   dağıtımın varsayılanının getirdiği tam paketi alır (`php8.4-fpm`,
   `postgresql-17`); varsayılanı izleyen paketi asla almaz. Zaten kurulu olan,
   varsayılanı izleyen paketler depo kartında sahibin isteğe bağlı komutuyla
   bildirilir, asla kaldırılmaz. Bir sitenin Node.js sürümü yalnız istenince taşınır
   (aşağıda, 9. madde). *Kalan:* bu bileşenler için hiçbir şey.
2. **Kullanımda olan kaldırılmaz.** alpha.104–alpha.106'dan beri değişmedi; metin
   artık "CelikPanel'in bildiği hiçbir şey kullanmıyor" der (13. madde).
3. **Göster, kanıtla, geri al.** Kurulum benzetimi yükseltmeleri
   (`Inst name [old] (new)`) yeni kurulumlardan ayırır, onay için adlandırır ve
   bir dağıtım paketinin yerine bir üretici deposunun sürümünü koyacak adımı — ne
   onaylanmış olursa olsun — adıyla reddeder. Bir bağımlılığın dağıtım güvenlik
   güncellemesi (Debian 13'te `postgresql-17`, `perl-base`'inkini ister)
   adlandırılır ve onay bekler. Web postası güncellemeleri, Node.js taşımaları ve
   bir uygulamanın hesap taşıması kanıtlanır, olmazsa geri konur. Burada sunulan her
   yeniden başlatma, başlamadan önce neyi keseceğini söyler; web postası
   güncellemesi hiçbir servisin yeniden başlatılmadığını söyler.
4. **Yamalar işletim sisteminin işidir.** Güvenlik → Güvenlik güncellemeleri
   işletim sisteminin kendi durumunu okur: otomatik güncellemeler açık mı kapalı mı
   (apt'de unattended-upgrades; pacman ve DNF'de doğru olan), son çalışma, bekleyen
   güncellemeler ve güvenlik güncellemeleri, otomatik güncellemelerin kapsamadığı
   üretici kaynakları ve CelikPanel'in paket yöneticisinin dışında kurdukları
   (Roundcube, Node.js). Kapalı olması, sahibin komutuyla bir uyarıdır. Onayla, ayrı
   tek bir ek dosya yazar: sınırlanmış üretici kaynaklarını ekleyen
   `/etc/apt/apt.conf.d/52celikpanel-vendor-origins`. Sahibin dosyaları asla
   düzenlenmez; değiştirilmiş bir ek dosya olduğu gibi bırakılır. Panel otomatik
   güncellemeleri asla kapatmaz.
5. **Desteği bitmiş olan işaretlenir, asla zorlanmaz.** Tarihli destek tablosu
   (11. madde) sürümleri bileşen satırlarında, sürüm listelerinde, paket şeridinde
   ve müşterinin kendi sayfası da dahil sitenin PHP ve Node.js seçicilerinde
   işaretler. Bir site için artık güvenlik düzeltmesi almayan bir PHP sürümü seçmek
   ya da böyle birini kurmak onay ister (`SUPPORT_CONFIRMATION_REQUIRED`); onay
   denetim ayrıntılarına yazılır.
6. **Her iş kendi alanında.** Yeni bir Node.js uygulaması sitesinin kendi
   kullanıcısıyla çalışır (şema 45, `sites.app_run_as`). Önceden yapılmış bir
   uygulama paylaşılan web kullanıcısını korur ve bunu söyler; açıkça seçilen
   "Sitenin kendi kullanıcısıyla çalıştır" uygulamayı durdurur, dosyaların sahibini
   yalnız onun dizininde değiştirir (dizin kök olarak açılır, dışarıya çıkan bir
   bağlantı asla izlenmez, her yol değiştirilirken yeniden denetlenir), birimi
   yeniden yazar, bir yanıt bekler ve olmazsa ikisini de geri koyar. Paylaşılan yalıtım bir gerçek olarak söylenir: web postası,
   phpMyAdmin ve phpPgAdmin PHP-FPM'in varsayılan havuzunu paylaşır; bütün posta
   kutuları tek bir vmail kullanıcısının altındadır. *Kalan:* bu paylaşılan alanlar
   söylenir, ayrılmaz.
7. **Veriyi dönüştüren bir değişiklik asla yan etki değildir.** PGDG açıkken yalın
   bir PostgreSQL kurulumu artık PGDG'nin varsayılanını izleyip yeni bir ana sürüme
   geçmez; sınırlama elle yapılan bir `apt upgrade`'in de bunu yapmasını önler. Bir
   işletim sistemi yükseltmesinden sonra yeni bir küme bildirilir (17. madde).
   *Kalan:* CelikPanel bir ana sürüm yükseltme akışı (yedek denetimi ve onayla)
   sunmaz; ana sürüm yükseltmesi sahibin kendi adımı olarak kalır.
8. **Kaldırmak silmek değildir.** PostgreSQL ve MariaDB purge ile değil
   `apt-get remove` ile kaldırılır; pencere verinin nerede kaldığını söyler
   (`/var/lib/postgresql/<version>/<cluster>`, `/etc/postgresql`,
   `/var/lib/mysql`) ve böyle bir veri bulan kurulum onu olduğu gibi kullanacağını
   söyler. Gerçek sistem denemesi ikisini de var olan bir kümeyle kaydetti:
   `remove`'dan sonra küme yerinde kaldı ve yeniden kurulum onu veritabanıyla
   başlattı; Debian'ın kendi `postgresql-17` `purge`'ü, elle çalıştırıldığında,
   kümenin verisini ve yapılandırmasını sildi. Bu yüzden CelikPanel `remove`'da
   kalır. *Kalan:* yeniden kurulum ne bulduğunu söyler ama bir seçim sunmaz; başka
   bileşenler için purge'ün yapılandırma dosyası uyarısı yapılmadı.
9. **Paket yöneticisinin kurmadıkları.** Roundcube: güncelleme denetimi
   Roundcube'un resmî yayınlarını okur, satır "Güncelleme var" der ve (onaylanan)
   güncelleme yayını indirir, imzasını Roundcube'un sabitlenmiş imza anahtarıyla
   doğrular, yapılandırmayı, eklentileri, temaları ve SQLite veritabanını taşır,
   Roundcube'un kendi veritabanı güncellemesini çalıştırır, şimdiki ağacı arşivler,
   atomik olarak değiştirir, web postasını kendi soketi üzerinden kanıtlar ve
   olmazsa önceki ağacı bayt bayt geri koyar. Node.js: kurulu her hat daha yeni
   yamasını söyler; onu kurmak var olan doğrulanmış kurulumdur; eski yamadaki
   siteler listelenir ve her biri yalnız istenince taşınır, kanıtlanır ya da geri
   konur. *Kalan:* Roundcube'u başka bir hatta (1.6'dan 1.7'ye) taşımak sunulmaz.
10. **Yamalanmış, yeniden başlatılmış demek değildir.** Bakım → Yeniden
    başlatmalar ve yeniden açılış, hâlâ değiştirilmiş kodla çalışan servisleri
    (kuruluysa needrestart, değilse süreçlerin kendi bellek eşlemleri, sınırlı
    olarak) ve sunucunun yeniden açılış bekleyip beklemediğini, her birini
    kesintisiyle listeler; bir bileşen istenince kendi yeniden başlatma eylemiyle
    yeniden başlar, başka her şey için komut verilir. İşletim sisteminin
    needrestart'ı servisleri kendiliğinden yeniden başlatıyorsa bunu söyler.
11. **"Desteği bitti" doğru tanımlanır.** `internal/supportdata/support-table.json`
    (24 Ekim 2026'da denetlendi, kaynaklarıyla) PHP, PostgreSQL, MariaDB ve Node.js
    için üst kaynağın bitiş tarihlerini ve desteklenen her dağıtım sürümü için
    hangi sürümü dağıtıp yamaladığını verir. Bir dağıtım sürümü, dağıtım sürümü
    yaşadıkça yamalanır; bir üretici ya da resmî sürüm kendi üst kaynağını izler;
    "yakında bitiyor" 90 gündür; bilinmeyen bilinmeyen kalır; bir tarih, kaynağı onu
    ne kadar kesin veriyorsa o kesinlikte saklanır (Ubuntu'nunkiler aydır). Koda
    gömülü PHP eşiği kaldırıldı.
12. **Üretici depoları sahibi tarafından sınırlanabilir.** Sınırlama sahibin depo
    kartından yaptığı bir eylemdir, asla kendiliğinden olmaz (ne depo açılırken ne
    Panel başlarken; 24 Ekim 2026 incelemesi): **Sınırla…** önce planı gösterir —
    `/etc/apt/preferences.d/celikpanel-<id>.pref` dosyasının tam metni ve SHA-256'sı,
    üreticiyi izlemeye devam eden kurulu paketler ve `apt-get -s upgrade`'in onunla
    ve onsuz nasıl farklılaştığı — ve yalnız SHA-256'sıyla onaylanan o metin yazılır.
    Kaynak bundan sonra yalnız dağıtımın sağlamadığını sağlar; kurulu sürümü
    üreticiden gelen her paket onu izlemeye devam eder (liste paket kurulu kaldıkça
    korunur, böylece üreticiden gelen hiçbir şey güncellemelerinden yoksun kalmaz).
    Sahibin sildiği bir dosyayı kart sahibin kaldırdığı dosya olarak okur ("…
    kural dosyası sunucuda silindi. CelikPanel onu kendiliğinden yeniden yazmaz")
    ve dosya istenmedikçe asla yeniden yazılmaz; değiştirilmiş bir dosya ya da depoyu
    adlandıran herhangi bir sahip kuralı (kaynak sunucu adıyla ya da `o=`, `l=`,
    `n=`, `a=`, `c=` ile) korunur ve adlandırılır. Yalnız üreticinin sağladığı bir
    sürüm (dağıtımda adayı yok), ihtiyaç duyduğu üretici ortak paketini (Sury'nin
    `php-common`'ı, PGDG'nin `libpq5`'i) yalnız sürümleriyle adlandırılmış, onaylanmış
    bir öğe olarak alır; aksi hâlde asla. Depo kartı onu kimin ne zaman açtığını, imza anahtarının
    parmak izini ve bitiş tarihini (60 gün kala uyarı), listenin geçerliliğini ve
    Agent'ın liste yenilemelerinden apt'nin hatalarını ve uyarılarını söyler.
13. **Yalnız bildiğini sayar.** Kaldırma pencereleri "CelikPanel'in bildiği hiçbir
    şey …" der. *Kalan:* PHP araçlarının nginx satırlarının ötesinde sistemin kendi
    izlerine (bir sokete işaret eden yapılandırma, açık bağlantılar) bakmak
    yapılmadı.
14. **Müşterinin seçimi.** İşaretler ve onay müşterinin kendi PHP seçicisine ulaşır.
    *Kalan:* yönetici henüz sürümleri müşterilerden gizleyemez.
15. **Denenmiş ve izin verilen.** Desteklenen işletim sistemi listesi denenmiş
    sürümleri kabul edilenlerden ayırır. *Kalan:* alpha.105'in PHP uyarısının
    ötesinde bileşen başına "denenmiş" işaretleri.
16. **Her karar kayda geçer.** Şema 45 `audit_logs.details` (sınırlı JSON) ekler:
    bileşen, paket, eski ve yeni sürüm, onaylanan dikkat çeken paketler, desteği
    bitmiş bir sürümün onayı, taşımalar ve sonuçlar; sitenin PHP ve Node.js sürüm
    değişiklikleri, uygulama hesap taşımaları, depo sınırlaması, güncellemeler ek
    dosyası ve gözden geçirildi olarak işaretlenen bir işletim sistemi değişikliği
    denetlenir. Denetim sayfası ayrıntıları gösterir.
17. **İşletim sistemi sürümü sahibinindir.**
    `internal/supportdata/os-releases.json` desteklenen sürümleri listeler
    (Debian 13 ve Ubuntu 24.04 denenmiş; Debian 12 ve Ubuntu 22.04 kabul edilmiş,
    denenmemiş; geliştirme için Arch rolling; RHEL 9 ailesi kurulumu reddeden bir
    önizleme); Sunucu kurulumu bu sunucunun buradaki yerini ve yama tarihini
    gösterir. Panel sürümü ve bir yükseltmenin altında neyi değiştirdiğini kaydeder;
    sürüm farklı olduğunda salt-okur bir karşılaştırma neyin değiştiğini (eski kod
    adını anan bir depo, yeni bir PostgreSQL kümesi, PHP hatları, MariaDB'nin sürümü)
    sonraki eylemle birlikte söyler; bu, yönetici onu gözden geçirildi olarak
    işaretleyene kadar sürer. CelikPanel asla bir yükseltme başlatmaz. *Kalan:*
    değişiklikler yükseltmeden sonra listelenir, öncesinde öngörülmez.

**Şema ve kurtarma.** Geçiş 045 boş bırakılabilir iki sütun ekler
(`audit_logs.details`, `sites.app_run_as`); önceden yazılmış satırlar NULL kalır ve
eskisi gibi okunur. Daha eski bir sürüm tasarım gereği 45 numaralı geçiş defterini reddeder; bu
yüzden geri dönüş yerinde sürüm düşürme değil, güncelleme öncesi anlık görüntünün
geri yüklenmesidir (D-025). Sınırlama dosyaları, ek dosya ve Agent'ın kayıtları
ekleyici dosyalardır; sahip onları `rm` ve `apt-get update` ile kaldırır ve
CelikPanel değiştirilmiş olanı olduğu gibi bırakır.

**Kanıt.** Yukarıdaki her madde için bileşen ve bağlanmış testler; atılabilir
Debian 13 sanal makinelerinde gerçek sistem denemeleri, ikincisi incelemeden sonra
sahibin sınırlaması için
([deploy/e2e/d038/alpha107-debian13-20261024](../deploy/e2e/d038/alpha107-debian13-20261024/README.md)).
Kurulu bir sunucuda ölçülmedi. Bu değişiklik hiçbir P0 işini kapatmaz.

### Değişiklik (25 Ekim 2026, saat tarihi): iki yol, yan yana PostgreSQL ana sürümleri ve kalan maddeler; v0.1.0-alpha.108 adayı için kaynakta uygulandı

Sahip 25 Ekim 2026'da iki yönü onayladı. Depolar üzerine: "güvenlik ve
stabilite her zaman daha önemli. özgürlük vereceğiz diye sistemi ölüme
göndermeyelim". PostgreSQL üzerine: "postgresql 16 başka
17 başka 18 başka; farklı portlara kurulmalı; portlar düzenlenebilmeli; 16 varken
17'yi başka porta kurar, veritabanlarını taşır, sonra 16'yı kaldırır".
v0.1.0-alpha.108 adayı için kaynakta (yayımlanmadı; kurulu bir sunucuda hiçbir şey
ölçülmedi).

**İki yol (1., 4. ve 12. maddeleri inceltir).**

1. **Varsayılan yol işletim sisteminin kendi paketleridir.** Hiçbir üretici
   deposunun açık olmadığı bir sunucuda PHP-FPM ve PostgreSQL sayfaları bunu söyler
   ("işletim sisteminin paketleri"); sürüm listeleri, yalnız bir üretici deposunun
   getirdiği bakımı süren sürümleri kilitli olarak adlandırır: "<depo> deposundan
   gelir, <işletim sistemi>'nden değil" ve onları kimin derleyip yamaladığı
   (tarihli destek tablosundan: bu sürümde sağlayıcısı üst kaynak olan sürüm).
2. **Üretici yolu, yöneticinin bilgili seçimidir; bileşen başınadır** ve kısa,
   bilinen bir listeden gelir: PHP için Sury, PostgreSQL için PGDG; her birinin
   katalogdaki Release kaynağı vardır. Node.js, panelin zaten yamaladığı (9. madde)
   resmî, sağlama toplamı doğrulanmış derlemelerini korur. Bir üretici deposunu
   açmak, gösterilen ve onaylanan tek bir işlemdir (Agent.VendorLanePlanV1,
   Agent.EnableVendorLaneV1; kendi `vendor_lane` işinde): kaynak satırı ve
   sabitlenmiş anahtarı, CelikPanel'in sınırlama kuralı (alpha.107 metni) ve
   deponun kaynağının otomatik güncellemeler ek dosyasındaki yeri (alpha.107
   dosyası); her biri kendi dosyası, tam metni ve SHA-256'sıyla, tek bir plan
   özetiyle onaylanır. Önce sınırlama, ikinci olarak otomatik güncellemelerdeki
   yeri, en son kaynak satırı yazılır; böylece kaynak hiçbir zaman sınırsız ya da
   yamasız var olmaz; başarısız
   bir ilk açma yazdıklarını geri alır. İşletim sisteminin otomatik güncellemeleri
   kapalıyken ya da kurulu değilken ilk açma, yöneticinin komutuyla reddedilir.
   Bir üretici yolunun yalın açılması reddedilir (`REPO_LANE_REQUIRED`); yalnız
   zaten açık olan bir yol için "Güncel anahtarını al" bunun dışındadır. Deponun
   zaten açık olduğu bir sunucuda (iki sahip sunucusunda da) hiçbir şey kapatılmaz
   ya da yeniden yazılmaz: depo kartı eksik adımları adlandırır ve yalnız onları
   yazan tek bir **Tamamla…** sunar; sahibin kendi tercih kuralı geçerlidir,
   düzenlenmiş bir CelikPanel dosyası ya da düzenlenmiş bir ek dosya korunur ve
   adlandırılır, onun engellediği bir adım için yöneticinin ekleyeceği satır
   söylenir. Güvenlik güncellemeleri kartı, kapsanmamış ya da sınırlanmamış bir
   üretici deposu için aynı eylemi sunar.
3. **Panelin kendi araçları varsayılan olarak dağıtımın PHP'sinde kalır.** Bir
   aracın PHP seçimi önceden bir dağıtım sürümünü seçili getirir; yalnız
   üreticinin getirdiği bir sürüm bir notla "siteler için" diye işaretlenir. Bir
   aracın kurulumunun varsayılanları değişmez.
4. **Müşteriler hiçbir zaman depo açamaz** (`/api/v1/repo` ve yol yönetici
   yollarıdır; testler bir müşterinin reddedildiğini kanıtlar). **Kapatmak hiçbir
   şeyi kaldırmaz:** Agent hangi kurulu paketlerin depodan geldiğini kaydeder; sürüm
   listesi ve bileşenin sayfası onları tarihle "artık güncellenmiyor" diye işaretler.

**Yan yana PostgreSQL ana sürümleri (7. maddenin "ana sürüm yükseltme akışı"nın yerine geçer).**
Yerinde dönüştürme yapılmadı. Debian ve Ubuntu'da (postgresql-common), başka bir
ana sürüm kendi portunda kendi kümesi olarak kurulur: PostgreSQL sayfasındaki
kümeler kartı her kümeyi gösterir (sürüm, port, durum, veri dizini, kullanıcı
veritabanları, panelin ondaki kayıtları) ve paket adayı olan ana sürümleri,
yeni kümenin alacağı portla birlikte sunar (5432'den başlayarak ilk boş port;
her kümenin yapılandırmasından ve dinlenen portlardan okunur); kurulum önizlemesi
üzerinden gider ve sonuç kümenin gerçekte aldığı portu söyler. Bir kümenin portu,
değişen tek satır ve SHA-256'sı, şu an kimin bağlı olduğu, panelin kayıtları ve
yeniden başlatmanın keseceği gösterildikten sonra düzenlenir; küme yeniden başlar,
yeni port kanıtlanır, aksi halde eski dosya geri yazılır, yeniden başlatılır ve
kanıtlanır. Güvenlik duvarı yalnız yerel kalır. Bir veritabanı kopyalanarak
taşınır: kök dışında kimsenin okuyamadığı bir dosyaya pg_dump, eksik roller
kaynağın kendi tanımlarından oluşturulur (parolalar taşınır, asla gösterilmez),
pg_restore, sonra her tablonun satır sayısı iki tarafta karşılaştırılır; kaynak
kalır. Panelin kaydını ve bir sitenin saklı sunucusunu ve portunu geçirmek, geri
geçirmek ve kaynağı bırakmak ayrı, onaylı adımlardır; kaynağı bırakmak, bağlı bir
şey varken reddedilir. Bir ana sürümü kaldırmak, kümesi kullanıcı veritabanı
tuttuğu (adlandırılır) ya da okunamadığı sürece reddedilir; hâlâ `apt-get remove`
olarak kalır, asla purge değil. **Şema 45'ten 46'ya:** `database_servers.cluster`
(her eski satırda NULL: o kaydın portundaki küme; ilk kümeler okumasında dolar) ve
`postgresql_database_moves` tablosu (kanıt ve geçiş durumu). **MariaDB:** paketlerle
yan yana çalıştırmak mümkün değildir ve sayfası bunu söyler; yerinde ana sürüm
değişikliği 7. maddenin arkasında kalır ve yapılmadı.

**alpha.107 değişikliğinin kalan maddeleri.**

- **14. madde, müşterinin seçimi:** yönetici, her PHP ve Node.js sürüm hattı için
  müşterilere sunulup sunulmayacağını seçer (bir `panel_settings` satırı;
  varsayılan: hepsi sunulur). Bir müşterinin gizlenmiş bir sürümü seçmesi sonraki
  eylemi söyleyerek (yöneticiye sorun) reddedilir (`VERSION_NOT_OFFERED`); bir yönetici
  asla kısıtlanmaz; gizlenmiş bir sürümdeki site onu korur (bir Node.js sitesi
  kendi hattının başka bir yamasına yine geçebilir) ve sahibi artık yeni seçimler
  için sunulmadığını görür.
- **15. madde, denenmiş ve izin verilmiş:** `internal/supportdata/tested-versions.json`
  her dağıtım sürümü için bu sürümün gerçek sistem kanıtının denediği sürümleri
  adlandırır (Debian 13: PHP 8.4 ve 8.5, PostgreSQL 17 ve 18); sürüm listeleri
  "denendi" ya da "denenmedi" (kurulabilir) diye işaretler. Bilgidir, asla engel
  değildir.
- **13. madde, sistemin kendi izleri:** bir kaldırmadan önce, sınırlı ve salt-okur
  olarak, Agent bileşenin soketini ya da bir loopback portunu adlandıran web
  sunucusu yapılandırmasını (nginx ve Apache), /etc/systemd/system içinde onun
  birimine bağlı birimleri ve ona şu an bağlı yerel programları okur; her biri
  kaldırma penceresinde bir uyarı olarak adlandırılır (CelikPanel'in kendi
  dosyaları, birimleri ve siteleri, zaten adlandırıldıkları için hariç); aksi
  halde "CelikPanel'in bildiği hiçbir şey …".

**Sahibin alpha.107 bulguları, düzeltildi.** Her kurulum yolu kurulum önizlemesini
ve onayı gösterir (sürüm listesi ve Onar göstermiyordu; reddetme metni artık
yeniden Kur'a basmayı söyler). Sury'nin anahtarı "17 Mart 2021'de doldu" diyordu,
çünkü openpgp paketi dört kendi imzasından en eskisini tutuyordu; artık apt ve gpg
gibi, geçerli olan en yeni imza karar verir ve "apt artık kabul etmiyor" yalnız apt
bildirdiğinde söylenir. Boston'da phpPgAdmin'in ilk kurulum kanıtı, Agent'ın
`UMask=0027` ile çalışması yüzünden başarısız oldu: CelikPanel'in denetim dosyası
PHP-FPM'e okunamazdı (0750/0640); artık 0755/0644 yazılır ve bir kanıtla onarılır
(yeni neden `check_unreadable`); "çalışmıyor" yalnız soket yokken ya da birim
durmuşken söylenir; aynı sürümde "PHP x kullan" yalnız kanıtlar; kaybolan bir
yanıt okuyarak uzlaştırılır. Yerleşim: kurulum reddinin metni artık sütunlara
bölünmez, Bileşenler satırları orta genişliklerde adlarını bütün tutar, kenar
çubuğu simgelere daralır (tarayıcı başına hatırlanır), Roundcube'un satırında
doğrudan bir Güncelle vardır, sahip komutları `sudo` ile başlar ve "root olarak
oturum açtıysanız sudo yazmayın" der, anahtar satırının sözcükleri ayrıdır.

**Kalan.** Kurulu bir sunucuda hiçbir şey ölçülmedi. DNF ve pacman ailelerinin
üretici yolu yoktur (yalnız apt) ve yolları bileşen testleriyle kapsanır. Node.js'in
dağıtım yolu yoktur. Kapatılan bir üretici deposu kaynağını ek dosyadan kaldırmaz
(zararsızdır; kaynak gitmiştir). PostgreSQL taşıması rol üyeliklerini yalnız kopyanın
oluşturduğu roller için taşır; hedefte var olan bir rol kendi parolasını korur
(kanıt bunu söyler). Paylaşılan PHP-FPM havuzu ve tek vmail kullanıcısı (6. madde)
hâlâ söylenir, ayrılmaz. Purge'ün yapılandırma dosyası uyarısı (8. madde) ve
Roundcube'u başka bir hatta taşımak (9. madde) yapılmadı. Bir işletim sistemi
yükseltmesinin değişiklikleri sonrasında listelenir, önceden tahmin edilmez (17.
madde).

**Sürüm incelemesinden düzeltmeler (25 Ekim 2026).** Bir taşımanın kopyası satırları
yalnız kopyalandığı anda kanıtlar: kaynağın yazma sayaçları (kullanıcı tablolarına
eklenen, güncellenen ve silinen satırlar ile istatistiklerinin sıfırlandığı an)
kanıtla saklanır, geçişten ve kaynak kaldırılmadan önce yeniden okunur; kaynağa
kopyadan sonra yazıldıysa (yeniden kopyalayın ya da önce uygulamayı kopyaya
yöneltin) ya da bu kanıtlanamıyorsa ikisi de reddedilir; kopyalanırken yazılan bir
kaynak kopyayı başarısız kılar. Bir üretici adresi için sahibin kendi kaynağına
(sources.list, `*.list` ya da `*.sources`) CelikPanel ikinci kaynak eklemez: yol onu
sahibinin olarak adlandırır ve yalnız sınırlamasını ve otomatik güncellemelerini
sunar; ikisi birden varsa çift kaynak adlandırılır ve engeller; ilk açma, hazırlanan
kaynağı mevcut tüm kaynaklarla birlikte doğrular. Yol, otomatik güncellemelerin
zamanlayıcısını da okur (kapalı ya da maskeli kapalıdır, okunamayan bilinmiyordur) ve
reddi, sahibin depoyu kendisinin de ekleyebileceğini söyler. pg_lsclusters yokluğu
dışında herhangi bir nedenle başarısız olursa kaldırma bilinmiyor olarak reddedilir;
bir kümenin `postgres` veritabanındaki kullanıcı tabloları da onun tuttuğu veri
sayılır. PHP'ye geri geçirilen bir müşteri sitesi sunulan PHP sürümlerine bağlıdır.
Kanıt: birim ve bağlanmış testler ile aynı temel kalıpta PostgreSQL ve üretici yolu
testlerinin kısa ikinci gerçek sistem denemesi
([kanıt](../deploy/e2e/d038/alpha108-debian13-20261025/README.md)).

**Şema ve kurtarma.** Geçiş 046 ekleyicidir (boş bırakılabilen bir sütun ve yeni bir
tablo). alpha.107 Panel'i 46 numaralı geçiş defterini reddeder; bu yüzden geri
dönüş, güncelleme öncesi anlık görüntünün geri yüklenmesidir (D-025); sürüm
kurtarma düzeneği geçiş 46'yı sabitler (defter `6ae06313…`, şema `b5b65789…`).
Yolun dosyaları alpha.107 dosyalarıdır; sahip onları `rm` ve `apt-get update` ile
kaldırır.

**Kanıt.** Yukarıdaki her madde için bileşen ve bağlanmış testler; saklanan temel
imajdan atılabilir bir Debian 13 sanal makinesinde bir gerçek sistem denemesi
([deploy/e2e/d038/alpha108-debian13-20261025](../deploy/e2e/d038/alpha108-debian13-20261025/README.md)).
Kurulu bir sunucuda ölçülmedi. Bu değişiklik hiçbir P0 işini kapatmaz.

## D-037 · Her sayfa adının söylediği yerdedir: Ayarlar ayarları tutar; araçlar, güvenlik ve bakım Sunucu altındadır; kişinin kendi hesabı kişisel menüdedir

*22 Ekim 2026 (saat tarihi) · Sahibin 21 Ekim 2026 tarihli isteği ve 22 Ekim 2026 tarihli onayı; v0.1.0-alpha.98 adayı için kaynakta, 22 Ekim 2026'da yayımlandı; 22 Ekim 2026'da sahibin kendi güncellemesinden sonra sahibin ekranları yeni menüyü iki kurulu sunucuda da gösterdi · 22 Ekim 2026'da değiştirildi: kişinin kendi e-postasını değiştirmesi, v0.1.0-alpha.99 adayı için kaynakta; değişikliğin hiçbir parçası kurulu bir sunucuda ölçülmedi · 22 Ekim 2026'da yeniden değiştirildi: Sunucu kurulumu Sunucu altına taşınır, kaldırma penceresi neyin etkileneceğini söyler, ilk ziyaret Kurulu görünümü açar; v0.1.0-alpha.100 adayı için kaynakta · 23 Ekim 2026'da yeniden değiştirildi: bir kaldırma adını söylemediği hiçbir şeyi kaldırmaz ve her PHP aracı sahibin seçtiği PHP sürümünde çalışır; v0.1.0-alpha.104 adayı için kaynakta; kurulu bir sunucuda hiçbir şey ölçülmedi*

Sahip 21 Ekim 2026'da şunu sordu: "ayarlardaki bu alt
menüdeki her şey ayarlar ile mi ilgili?" ve sürdürdü: "her işlemin yeri doğru
olmalı. saçma sapan alakasız yerlerde olmamalılar. user friendly yaklaşım önemli.
herkes aradığını kolayca bulsun." Yeni düzenin bir taslağı gösterildi, bir ikinci
düşünceyle gözden geçirildi (aşağıda) ve 22 Ekim 2026'da sahip şöyle yanıt verdi:
"menu düzeni taslağı ok."

Soru yerindeydi. Ayarlar'da sekiz bölüm vardı: Sunucu kurulumu, Hesap güvenliği,
Panel HTTPS, Lisans, CelikPanel güncellemeleri, Sunucu güvenliği, DNS altyapısı ve
Sistem veritabanları. Bunların dördü panelin ayarı değildi: dışa açık hizmet denetimi
sunucunun güvenliğiyle ilgilidir, DNS altyapısı sunucunun DNS rolü ve eş
sunucusudur, Sistem veritabanları SQLite dosyalarının bakımıdır ve Hesap güvenliği
giriş yapmış kişinin kendi iki faktörlü doğrulama panelidir. Panelin ayarlarına
dokunmayan bir bayi ya da müşteri, tek bölümü kendi hesabı olan bir Ayarlar görüyordu.
D-019 boş yönetim sayfalarının yasak olduğunu zaten söyler; yalnız birinin kendi
hesabını tutmak için var olan bir Ayarlar, aynı kusurun menüden görünüşüdür.

Benimsenen kurallar:

1. **Bir sayfa adının söylediği yere gider.** Ayarlar, bir kez yapılan ve panelin
   nasıl davranacağını belirleyen seçimler içindir. Araçlar, sunucunun güvenliği ve
   bakımı Sunucu altına aittir. Sunucuya değil kişiye ait olan (parolası ve iki
   faktörlü doğrulaması) kişisel menüye aittir.
2. **Son düzen (yönetici).**
   - Panel (ana sayfa), değişmedi.
   - **Barındırma:** Alan Adları, Veritabanları, Kullanıcılar, Mağaza, VPN;
     değişmedi.
   - **Sunucu:** Bileşenler · **DNS ve eş sunucular** (Ayarlar → DNS altyapısı'ndan
     taşındı) · İzleme · **Güvenlik** (yeni sayfa) · **Bakım** (yeni sayfa) · İçe
     aktar · Denetim günlüğü.
   - **Ayarlar** (menünün sonunda) dört şey tutar: **Panel adresi ve HTTPS**
     (öğenin adı "Panel HTTPS" idi), Lisans, CelikPanel güncellemeleri ve Sunucu
     kurulumu.
   - **Kişisel menü** (sağ üstteki kullanıcı adı): ilk sırada **Hesabım**,
     telefonda dil seçici (eskisi gibi), Çıkış. Tema ve dil üst çubukta kalır.
3. **Taşınan her şey nedir ve neden orada.**
   - *DNS ve eş sunucular* (`/dns-infrastructure`): bugünkü DNS altyapısı içeriği,
     olduğu gibi: ortak ad sunucuları, bu sunucunun rolü, eş sunucusu ve DNS
     çoğaltması. Bu, bir panel seçimi değil, sunucunun DNS rolüdür.
   - *Güvenlik* (`/security`): "bu sunucu güvende mi?" sorusunu tek yerde yanıtlar.
     (1) Ayarlarıyla güvenlik duvarı (aç/kapat, yeniden başlatma için kaydet, açık
     portlar, SSH satırı); eskiden Bileşenler sayfasındaki tek satırlık güvenlik
     duvarı çubuğundan açılan tam kutu. (2) Fail2ban'ın durumu, saklanan bileşen
     kaydından okunur (çalışıyor; kurulu ama çalışmıyor; kurulu, çalışma durumu
     henüz okunmadı; kurulu değil; henüz denetlenmedi; başarısız okuma "okunamadı"
     ve "Tekrar dene"dir), bileşen sayfasına "Fail2ban'ı yönet" bağlantısıyla, kurulu
     değilse "Bileşenler'i aç" ile. (3) Ayarlar → Sunucu güvenliği olan salt-okur
     dışa açık hizmet denetimi, olduğu gibi taşındı.
   - *Bakım* (`/maintenance`): Ayarlar → Sistem veritabanları, olduğu gibi taşındı
     (CelikPanel'in ve kurulu servislerin tuttuğu SQLite dosyalarını inceleyin ve
     güvenle bakımını yapın). Sayfa yedekleme gibi gelecekteki hiçbir özelliği vaat
     etmez.
   - *Hesabım* (`/account`): Ayarlar → Hesap güvenliği olan iki faktörlü doğrulama
     paneli, olduğu gibi taşındı, ve menünün eskiden açtığı parola penceresini açan
     bir Parola kartı. Roller: yönetici, bayi ve müşteri; yani Ayarlar → Hesap
     güvenliği'ne sahip olan roller.
   - *Ayarlar*, panelin nasıl davranacağını belirleyen ve seyrek değiştirilenleri
     tutar: sunulduğu adres ve HTTPS sertifikası, lisansı, kendi güncellemeleri ve
     sunucunun amacı ile kurulumu.
4. **Onaydan önce alınan ikinci düşünceler.** İlk taslak İçe aktar'ı Barındırma'ya,
   Sunucu kurulumu'nu da Ayarlar'ın dışına taşımıştı; ikisi de geri alındı. *İçe
   aktar* Sunucu altında kalır: seyrek kullanılan bir araçtır ve sunucu araçları
   arasında aranır. *Sunucu kurulumu* Ayarlar'da kalır: bir kez, sonra "genişletmek"
   için seyrek çalıştırılır; sunucunun amacı bir yapılandırma seçimidir; yılda bir
   yapılan bir işin menüde kalıcı kendi yeri olmaz. *Mağaza* ve *VPN* Barındırma
   altında kalır, çünkü bayiler ve müşteriler onları kullanır.
5. **Eski adresler çalışmaya devam eder.** Her eski `/settings?section=` adresi yeni
   sayfasına yerine koyma yönlendirmesiyle (ek geçmiş girdisi olmaz) yönlendirilir:
   `?section=account` → `/account`, `?section=security` → `/security`,
   `?section=dns` → `/dns-infrastructure`, `?section=system-databases` →
   `/maintenance`. Ayarlar'ı boş olacak bir bayi ya da müşteri için `/settings`,
   adres hangi bölümü söylerse söylesin `/account`'a gider (Ayarlar eskiden
   gösteremediği her bölüm için onlara hesap bölümünü açardı) ve menüde artık
   Ayarlar'ı görmezler. Ek kullanıcı (ekip üyesi) hiç Ayarlar'a ya da hesap
   bölümüne sahip olmadı: rota koruması onu hâlâ ana sayfaya gönderir ve kişisel
   menüde "Parolayı değiştir"i korur. Yer imleri, eski yönlendirme bağlantıları ve
   eski bir Panel'in bağlantıları (tarayıcı yönlendirir) bu yüzden doğru yere ulaşır.
6. **Roller değişmez.** Bir sayfayı görebilen ya da bir API'yi çağırabilen rol hâlâ
   görebilir ve çağırabilir; yalnız yer taşındı. API, Agent, güncelleyici, şema,
   yoklama ya da bileşen davranışında hiçbir değişiklik yok. Barındırılan hiçbir şey
   bir sayfanın nerede olduğuna bağlı değildir (D-022).

Bunu kurarken alınan uygulama kararları (sahip kararı sanılmasın diye böyle
belirtilir):

- **Ayarlar'ın dört bölümü var.** Taslak, yöneticilerin tümü için bir politika
  ayarı varsa beşinci bir bölüme, yönetici güvenlik kurallarına, izin veriyordu.
  Böyle bir ayar yok (Ayarlar → Hesap güvenliği yalnız giriş yapmış kişinin kendi
  iki faktörlü doğrulama panelini tutuyordu), bu yüzden hiçbiri uydurulmadı.
- **Parola değiştirme Hesabım'a taşındı**, aynı pencereyi açar. Menü, Hesabım'ı olan
  roller için onu artık yinelemez; ek kullanıcılar onu menüde korur. Pencere her sayfa
  yüklenişinde değil, açıldığında getirilir; sözleri kabuk kataloğunda kalır
  (`profile.*`), bu yüzden asla ham anahtar göstermez.
- **Simgeler** lucide kümesinden gelir: DNS ve eş sunucular için ağ, Güvenlik için
  onay işaretli kalkan, Bakım için bir anahtar, Hesabım için onay işaretli bir kişi
  (UserCheck, açılış paketinde zaten var).
- **Güvenlik duvarı kutusu, dikkat gerektirdiğinde Bileşenler'de hâlâ görünür**
  (kapalı, okunamıyor, motor yok, bir SSH notu, kalıcılık), çünkü önemli olduğu anda
  sahip oradadır; yalnız sağlıklı durum tek sakin satırdır ve onun bağlantısı
  `/security#firewall`'u açar ve bunu söyler ("Güvenlik'te yönet"; önceden
  "Ayarlar" diyordu, bu ad artık güvenlik duvarı ayarı tutmayan bir yeri
  gösteriyor). Tarayıcı denetimi 23 bu etiketi ve çalışan bir Fail2ban'ın "kurulu
  ama çalışmıyor" diye çizildiğini buldu (kayıtlı kayıt systemd'nin "active
  (running)" sözünü taşır); ikisi de bu kayıttan önce düzeltildi. Kurulu olmayan,
  isteğe bağlı Fail2ban amber değil sakin çizilir.
- **Güvenlik sayfası Fail2ban'ı yalnız saklanan kayıttan okur**; yönetimi bileşen
  sayfasında kalır. Açıkken ana makine değişiklik hazırlığını Bileşenler gibi her 5
  saniyede okur ve sunucu meşgulken aynı "değişiklikler bekletiliyor" satırını
  ("Sunucu değişiklikleri geçici olarak kullanılamıyor.") gösterir (D-024; D-033).
- **Ürün içindeki her bağlantı doğrudan yeni yeri gösterir:** Panel sayfasının
  başlangıç rehberi ve hesap kartı, Alan Adları'nın "DNS gerekli"si, Alan adı ekle penceresi,
  Bileşenler'in DNS düğmeleri ve güvenlik duvarı satırı, sunucu kurulumunun sonraki
  adımı ("DNS ve eş sunucuları aç"), kurulum kurtarma bağlantıları ve başlangıç
  rehberi. DNS çifti için "Ayarlar > DNS altyapısı", "Ayarlar > Sunucu güvenliği" ya
  da "Ayarlar'da" diyen yönlendirme metinleri artık "Sunucu → DNS ve eş sunucular" ya
  da "Sunucu → Güvenlik" der. Sunucu tarafında `/settings?section=dns` gösteren her
  yönlendirme eylemi (DNS motoru iş akışı, DNSSEC, yalnız-DNS çocukları, içe aktarma,
  uzak DNS yetkisi, DNS kurulumu) artık `/dns-infrastructure`'ı gösterir; Panel
  sertifikasının eksik iletişim e-postası eylem taşımaz (aşağıdaki inceleme
  düzeltmelerine bakın; v0.1.0-alpha.99 adayından başlayarak `/account#email`'i
  gösterir, sondaki değişikliğe bakın).
- **Kurulum geçidi dört adresi erişilebilir tutar** kurulum gerekirken
  (`/account`, `/security`, `/dns-infrastructure`, `/maintenance`), `/settings`
  gibi; çevrimdışı kurtarma kabuğunun rota listesi de onları içerir.
- **Kişisel menü bir açılır bölümdür:** `aria-expanded`, Escape onu kapatır ve odağı
  düğmesine geri verir.

Bunun yetkilendirmediği: yeni bir özellik, kimin neyi görüp yapabileceğinde bir
değişiklik, bir sayfayı doldurmak için uydurulmuş bir politika ayarı ve sahibin
kurulu sunucularda görmesinden önce onlar hakkında hiçbir iddia. Kurulu paneller
hâlâ yalnız sahipleri tarafından, panelin kendi güncelleme ekranından güncellenir.

Geri kalanla ilişkisi. D-019 (boş yönetim sayfaları yasak): bir bayi ya da müşteri
için hiçbir şeyi olmayan Ayarlar artık yok. D-022 (sahibin altyapısı panele bağlı
değildir): yalnız bir ekranın yeri taşındı; sunucuda çalışan hiçbir şey ona bağlı
değildir. D-024 (her işlem mevcut durumu ve sonraki eylemi açıklar): Güvenlik
sayfasının Fail2ban durumları ve "değişiklikler bekletiliyor" satırı onu izler;
yönlendirme metinleri ve eylem bağlantıları yeni yeri adlandırır. D-033 (sahibi
bekleten bir ekran bunu söyler): Güvenlik sayfasının hazırlık okuması, Bileşenler'in
zaten sahip olduğu görünür ve sınırlı okumadır. D-035: CelikPanel güncellemeleri,
güncelleme kartının ve menüdeki noktasının bulunduğu Ayarlar'da kalır.

**İnceleme düzeltmeleri (22 Ekim 2026, herhangi bir yayından önce).** Adayın bağımsız
incelemesi bunları istedi; kaynaktadırlar:

- **Açılış yolu.** Yalnız ekranların gösterdiği kodlu retler (dil başına 126: DNS,
  posta, alan adları, siteler, bileşenler, depolar, kotalar, güvenlik duvarı) kabuk
  kataloğundan "errors" adlı bir ekran parçasına taşındı; bu parça diğerleriyle
  birlikte, hiçbir ekran çizilmeden önce yüklenir. Kabuğun ondan önce
  gösterebileceği kodlar (giriş ve erişim, lisans, güncelleme izleyicisi, işlem
  katmanı, istek kimliği) kabukta kalır. Sözleri henüz gelmemiş bir ret, sunucunun
  kendi cümlesini ya da genel hatayı gösterir, asla anahtarını. Kritik açılış yolu
  361 KiB'de 360,75'ten 329,16'ya indi; yeni parçanın en az %25 paylı kendi tavanı
  var. Parola penceresinin sözleri kabukta kalır (`profile.*`); menünün ek kullanıcı
  için açtığı pencere asla ham anahtar göstermez.
- **Güvenlik duvarı kutusunun kendi modülü var** (`FirewallPanel.tsx`): Güvenlik
  sayfası onu çizmek için artık Bileşenler sayfasını yüklemez. Uzak durum cırcırı
  taşınmayı kaydeder; toplamları değişmedi.
- **Tek çalışma kuralı** (`lib/serviceStatus.ts`) Panel, Bileşenler, bir bileşenin
  sayfası ve Güvenlik için: "running" ya da "(running)" diyen durum. "active
  (exited)" bitmiş tek seferlik birimdir, çalışmıyordur; önceden Panel ve bir
  kopyanın satırı her "active ..." durumunu çalışıyor sayıyordu. Fail2ban kaydı
  olmayan bilinen bir bileşen listesi "kurulu değil" der.
- **Kalan metinlerdeki eski yer adları** artık yeni yeri adlandırır: bir alan adının
  ad sunucusu adımı, yönetilen bir DNS kaydı, başlangıç rehberinin DNS adımı,
  kurulumun geçiş engeli, "DNS sunucusuna bağlan", kurulum kurtarma bağlantısında ve
  yenileme engelinde "Panel adresi ve HTTPS" ve Panel'in iki kurulum iletisi.
- **Sertifikanın iletişim e-postası.** Ret Ayarlar → Hesap güvenliği'ni, bu sürümün
  ilk taslağında da Hesabım'ı gösteriyordu; ikisi de onu değiştiremez. Yöneticinin
  kendi e-postasını düzenleyen bir ekran yok (yönetici yalnız sunucuda oluşturulurken
  yönetilir); bu yüzden ret artık eylem taşımaz ve bunun için henüz bir ekran
  olmadığını söyler. **Açık iş** (alpha.98'de açık): kendi e-postasını Hesabım'dan
  değiştirmenin bir yolu. *Artık v0.1.0-alpha.99 adayı için kaynakta yapıldı*
  (aşağıdaki değişiklik); alpha.98 yayımlandığında açıktı.
- Taslağın Hesabım tanımında geçen **"açık oturumlar"** yapılmadı: üründe oturum
  listesi yok (yol haritası "Aktif oturumlar"ı sonraki bir iş olarak tutar).
  Yönetici kurallarında olduğu gibi hiçbir şey uydurulmadı.
- Kişisel menü odak ondan çıkınca kapanır ve panelini yalnız açıkken adlandırır;
  Bakım tek alt başlık tutar (kartınınkini); ayarlar katalog parçasının blok
  işaretleri düzeltildi; iki tarayıcı denetimi betiği hâlâ var olan adresleri
  kullanır.

alpha.98'de yapılmadı: yazıldığında kurulu bir sunucuda hiçbir şey ölçülmemişti
(22 Ekim 2026'da, sahip onları güncelledikten sonra, sahibin Boston ve Frankfurt
ekranları yeni menüyü gösterdi); yöneticinin kendi e-postasını değiştiren bir ekran
yoktu (yukarıdaki açık iş; alpha.98'de açıktı, aşağıdaki değişiklikle v0.1.0-alpha.99
adayı için kaynakta yapıldı); açık oturumlar yapılmadı.

### Değişiklik (22 Ekim 2026, saat tarihi): kişinin kendi e-postasını değiştirmesi v0.1.0-alpha.99 adayı için kaynakta yapıldı

*Sahibin, alpha.98'in bıraktığı açık iş için 22 Ekim 2026 tarihli onayı ("ok"); v0.1.0-alpha.99 adayı için kaynakta; kurulu bir sunucuda hiçbir şey ölçülmedi*

Neden. Panel sertifikası, giriş yapmış yöneticinin e-posta adresini sertifika
yetkilisine iletişim adresi olarak verir. alpha.99'a kadar bu adres yalnız yönetici
sunucuda oluşturulurken verilirdi ve hiçbir ekran onu değiştirmezdi; kullanılabilir
bir adresi olmayan bir yönetici alpha.98'den beri yalnız bunun için henüz bir ekran
olmadığını okuyabiliyordu. alpha.98, "kendi e-postasını Hesabım'dan değiştirmenin bir
yolu"nu açık iş olarak kaydetti. 22 Ekim 2026'da, alpha.98 yayımından sonra, sahip
bunun yapılmasını onayladı ("ok").

Benimsenen kurallar:

1. **Yalnız kişinin kendi adresi; oturumu olan her rol.** Giriş yapmış kişi kendi
   e-posta adresini değiştirir, başkasınınkini değil. Rota `PUT /api/v1/auth/email`'dir,
   `POST /api/v1/auth/password`'ün yanında; kendi parolasını değiştirebilen rollere
   açıktır: yönetici, bayi, müşteri ve ek kullanıcı (ekip üyesi). Ne yalnız yöneticiye
   özeldir ne de ek kullanıcılara kapalı rotalar listesindedir.
2. **Mevcut parola yeniden istenir.** İstek yeni adresi ve mevcut parolayı taşır;
   başka bir alan ya da birden çok JSON nesnesi taşıyan bir gövde reddedilir;
   anahtarlar tam olarak böyle yazılmalıdır. Her deneme, parola değiştirmeyle
   paylaşılan tek bir yeniden kimlik doğrulama sınırına (`reauth`, hesap ve istemci
   adresi başına) sayılır. Yanlış ya da eksik parola
   `403 account_email_wrong_password`; çok fazla deneme `429
   account_email_too_many_attempts`'tir.
3. **Her eylemin tek kimliği; bir yineleme asla ikinci kez değiştirmez (D-029).** İstek
   kimliği koruması artık bu rotayı korur; koruduğu ilk `PUT`'tur (önceden yalnız
   `POST` rotalarını koruyordu). İstek `X-CelikPanel-Request-Id`'yi taşır (32 küçük
   harfli onaltılık karakter). Aynı kimlik, ilk gelişin saklanan yanıtıyla
   (`X-CelikPanel-Request-Replayed: 1` ile) yanıtlanır ve adresi asla ikinci kez
   değiştirmez; başka bir adresle aynı kimlik `409 REQUEST_ID_REUSED`; kimliksiz bir
   istek `428 REQUEST_ID_REQUIRED`'dır. Rotanın süre sınırı 30 saniyedir. Korumanın
   sakladığı özet yalnız yeni adresin yeniden kodlanmış hâlidir (rota başına bir özet
   kancası) ve saklanan yanıt adres taşımaz; bu yüzden `request_identities` tablosu
   ne parolayı ne de adresi tutar. İşleyici,
   korumadan geçmeden ulaşılırsa çalışmayı reddeder.
4. **Tek doğrulayıcı.** Bir adres yalnız `mutationpayload.CanonicalContactEmail` ile
   kabul edilir; bu, panel sertifikasının var olan iletişim denetimidir, dışa açılıp
   yeniden kullanıldı: baştaki ve sondaki boşluklar atılır; yalnız yazdırılabilir
   ASCII; tam bir `@`; en çok 64 baytlık bir yerel kısım ve en çok 254 baytlık bir
   bütün; tam nitelikli bir DNS adı olan, küçük harfle saklanan bir alan adı; görünen ad
   yok. Burada kabul edilen bir adres bu yüzden sertifika isteğinin de her zaman kabul
   ettiği bir adrestir. Başka her şey `400 account_email_invalid`; okunamayan bir istek
   `400 account_email_request_invalid`'dir.
5. **Bir adres, bir hesap.** Başka bir hesabın kullandığı adres, büyük-küçük harf
   gözetmeden karşılaştırılarak `409 account_email_taken`'dır. `users.email` üzerindeki,
   `001_full_schema.sql`'den beri şemada olan `UNIQUE` kısıtı son sözü söyler. Şema
   değişikliği yok, yeni dizin yok. Adresin zaten kişinin kendisinin mi (`409
   account_email_unchanged`) yoksa başka bir hesabın mı olduğu ancak parola
   kanıtlandıktan sonra söylenir; rota bir adresi kimin kullandığını açığa vurmaz.
6. **Başka yerde yapılan bir değişiklik bildirilir, asla üstüne yazılmaz.** Yazım,
   doğrulanan adres ve parola özeti üzerinde karşılaştır-ve-yaz biçimindedir. Arada
   biri değiştiyse hiçbir şey yazılmaz ve yanıt `409 account_email_changed_elsewhere`'dir.
7. **Oturumlar sürer.** Giriş kullanıcı adıyla yapılır; adres bir giriş bilgisi
   değildir, bu yüzden açık oturumlar sonlandırılmaz.
8. **Denetim izi adresi değil hesabı adlandırır.** Kullanıcı üzerinde tek bir
   `account.email.changed` kaydı, eski ya da yeni adres olmadan (iz hesapları
   adlandırır, asla adresleri). Panel sayfasındaki "Son etkinlik" "kendi e-posta
   adresini değiştirdi" ("changed their own email address") der.
9. **Kendiliğinden sertifika işlemi yok.** Bir adresi değiştirmek hiçbir sertifika için
   hiçbir şey istemez, yenilemez ya da yeniden denemez. Panel sertifikası isteği giriş
   yapmış yöneticinin adresini istek yapıldığında veritabanından okur (önbellek yok).
   `panel_certificate_contact_email_invalid` reti artık `/account#email` eylemini taşır
   ve e-postanın Hesabım'da (sağ üst menü) ayarlanıp sertifikanın yeniden istenmesini
   söyler. Sunucu kurulum planı, incelendiği iletişim adresini planın kimliğinin
   parçası olarak tutar: bir e-posta değişikliğinden sonra zaten incelenmiş bir planı
   başlatmak "gereksinimler değişti, güncel planı inceleyin" diye reddedilir ve yeni
   inceleme yeni adresi alır; zaten çalışan bir kurulum, başladığı planın adresini
   korur.

Bunu kurarken alınan uygulama kararları (sahip kararı sanılmasın diye böyle
belirtilir):

- **Lisans kapısı değişmedi.** E-posta değiştirme, lisansı kilitli bir panelin hâlâ
  kabul ettiği rotalara *eklenmedi* (parola değiştirme onlardan biridir). Panel
  lisans nedeniyle kilitliyken e-posta değiştirme diğer rotalar gibi reddedilir.
  Lisansı kilitli bir panelin neyi kabul ettiği lisans politikasıdır ve bu sürümün onu
  değiştirme yetkisi yoktur.
- **Nerede.** Hesabım, Parola ile İki adımlı giriş arasında üçüncü bir kart kazanır:
  "E-posta adresim" ("My email", çapa `id="email"`; bu yüzden `/account#email` ona
  kaydırır). `/api/v1/auth/me`'den okunan güncel adresi gösterir (okurken
  denetlendiğini söyler; okuma başarısızsa adresin okunamadığını söyler ve "Tekrar
  dene" sunar). "E-postayı değiştir" düğmesi, yeni adres ve mevcut parola için küçük bir pencere
  açar. Hesabım sayfası olmayan ek kullanıcılar, kişisel menüde "Parolayı
  değiştir"in yanında aynı pencereyi açan "E-postayı değiştir" öğesini alır.
- **Bekleme görünür ve sınırlıdır (D-033).** Kaydederken düğme döner bir gösterge
  gösterir, alanlar ve düğmeler kilitlenir ve bir satır değişikliğin kaydedildiğini
  ve başka bir şey gerekmediğini söyler. 20 saniyeden sonra pencere, değişikliğin
  kaydedilip kaydedilmediğinin bilinmediğini, bunun bir hata olmadığını söyler ve kart
  adresi yeniden okur.
- **Kaybolan bir yanıt bir kez daha istenir.** Her tıklama tek kimlik; bağlantı
  koptuğunda ya da bir ağ geçidi yanıt verdiğinde pencere aynı kimlikle bir kez daha
  sorar. Panel'in kendisinin gönderdiği bir ret kaybolan yanıt değildir.
- **Retler D-024 sırasında.** Yukarıdaki her kod pencerede ne olduğu, hiçbir şeyin
  değiştirilmediği ve sonra ne yapılacağı olarak gösterilir. Başarı "E-posta adresiniz
  değiştirildi." der ve kart yeni adresi gösterir.
- **Kurulum planının iletişim engeli ve panel sertifikası reti** "E-posta adresinizi
  Hesabım'da (sağ üst menü) ayarlayın" der ve `/account#email`'e giden, Hesabım'ı
  açan bir düğme ("Hesabım’ı aç") gösterir.
- **Eski ya da yeni adrese ileti gönderilmez.** Panel yeni adresi doğrulamak için
  posta göndermez; kişi kim olduğunu parolasıyla kanıtlar.

Bunun yetkilendirmediği: bu rotadan başka bir hesabın adresini değiştirmek; e-posta
adresiyle giriş; herhangi bir sertifika isteği, yenilemesi ya da yeniden denemesi;
herhangi bir lisans politikası, şema, Agent ya da güncelleyici değişikliği ve sahibin
orada görmesinden önce kurulu sunucular hakkında hiçbir iddia. Kurulu paneller hâlâ
yalnız sahipleri tarafından, panelin kendi güncelleme ekranından güncellenir.

Geri kalanla ilişkisi. D-024: her ret ne olduğunu, kimin işlem yapacağını ve ne
yapılacağını söyler; bilinmeyen bir sonuca hata denmez. D-029: istek kimliği
korumasının altındaki ilk `PUT`, yalnız yeni adresin özetiyle. D-033:
pencerenin beklemesi görünür ve sınırlıdır. D-022: değişiklik yalnız panelin kendi
kullanıcı kaydını yazar; barındırılan hiçbir hizmet yeniden yapılandırılmaz ve panel
sertifikası adresi yalnız biri sertifika istediğinde okur.

**Güvenlik incelemesi düzeltmeleri (22 Ekim 2026, hiçbir yayımdan önce).** Adayın
bağımsız bir güvenlik incelemesi bunları istedi; kaynaktadırlar:

- **İki iletişim kapısı da tek doğrulayıcıyı kullanır.** Panel sertifikası isteği
  ve kurulum planının iletişim engeli yöneticinin adresini Go'nun
  `mail.ParseAddress`'iyle denetliyordu; bu, sertifika isteğinin kendi denetiminin
  reddettiği `admin@localhost`, `root@myhost` ya da ASCII dışı bir adresi kabul
  eder. Böyle bir adres (`--create-admin` verilen adresi denetlemediği için
  mümkündür) kapıdan geçiyor ve sonra Hesabım yönlendirmesi olmadan başarısız
  oluyordu. İki kapı da artık `CanonicalContactEmail` kullanır: sertifikanın
  reddedeceği her adres `/account#email` eylemli kodlu reddi ve kurulum engelini alır.
- **Saklanan özet yalnız yeni adresi tutar.** Go'nun JSON çözümü anahtarları
  büyük-küçük harf gözetmeden eşler (ve `ſ`'yi `s`'ye katlar); bu yüzden
  `"Current_Password"` taşıyan bir gövde parola alanına ulaşıyor, özet ise yalnız tam
  `current_password` anahtarını çıkarıyordu. Gövdenin anahtarları artık tam olarak
  `email` ve `current_password` olmalıdır; başka her anahtar (`Current_Password`,
  `user_id`, `id`, …) `400 account_email_request_invalid`'dır. Özet yalnız yeni
  adresin yeniden kodlanmış hâlidir; okunamayan bir gövde sabit bir özet alır.
- **Başarı yanıtı adres taşımaz** (koruma yanıtları 24 saat saklar); sayfa adresi
  yeniden okur.
- **Tek yeniden kimlik doğrulama sınırı.** E-posta değiştirme ve parola değiştirme,
  kullanıcı ve istemci adresi başına tek bir sınırlayıcı anahtarını (`reauth`)
  paylaşır; çalınmış bir oturum ikisi için tek bir tahmin bütçesi alır (5 dakikada
  10), rota başına bir değil.
- **Sonucu bilinmeyen bir gönderiden sonra aynı adresin sonraki Kaydet'i aynı
  kimliği taşır**, kesin bir yanıt gelene dek (Panel'in o kimlik için kesin olan `REQUEST_OUTCOME_UNKNOWN` yanıtından sonra değil: sonraki Kaydet o zaman yeni bir eylemdir); başka bir adres
  yeni bir eylemdir. Gövdede iki kez verilen bir anahtar, işleyicide ve özette aynı
  biçimde son değerini korur.
- **Kullanılamayan adres reddi**, İngilizce dışı harfler içeren bir alan adının ASCII
  biçimiyle (`xn--…`) yazıldığını söyler.
- Eklenen testler: `admin@localhost`, `root@myhost` ve ASCII dışı bir adresle iki
  kapı; `Current_Password`, `current_paſsword`, `user_id` ve `id` taşıyan gövdeler;
  "kullanılıyor" ve "zaten sizin" yanıtlarından önce söylenen yanlış parola; bir
  müşterinin yerine geçen yöneticinin müşterinin parolası olmadan reddi; başka
  parolayla gelen aynı kimliğin saklanan yanıtla yanıtlanması; iki rotanın paylaştığı
  sınır; bilinmeyen sonuçtan sonra aynı kimlik (web).

Kaydedilen bilinen sınırlar: yalnız büyük-küçük harfle ayrılan iki adrese aynı anda
yapılan iki değişiklik, harf gözetmeyen denetimden birlikte geçebilir, çünkü şemanın
`UNIQUE` kısıtı tam karşılaştırır; bu sürümde harf gözetmeyen bir dizin eklenmez.
Sertifika yetkilisi herkese açık olmayan bir alan adındaki (`.test`, `.local`,
`.internal`, `example.com`) adresi yine reddedebilir: panel biçimi denetler, alan
adına yetkili karar verir. `--create-admin` verilen adresi hâlâ denetlemez; panelin
kapıları artık onu yakalar.

Yapılmadı: kurulu bir sunucuda hiçbir şey ölçülmedi; yeni adres bir iletiyle
doğrulanmaz; açık oturumlar hâlâ yapılmadı.

### Değişiklik (22 Ekim 2026, saat tarihi): Sunucu kurulumu Sunucu altına taşınır; kaldırma penceresi neyin etkileneceğini söyler; geniş Bileşenler yeniden tasarımı geri çekilir

*Sahibin 22 Ekim 2026 kararları ("1 evet, 2 b, 3 evet"); v0.1.0-alpha.100 adayı için kaynakta; hiçbir parçası kurulu bir sunucuda ölçülmedi*

Neden. alpha.99 yayımlandıktan sonra sahip, Bileşenler sayfası için önerilen bir
yeniden tasarıma baktı (her satırda bir "⋯" menüsü, tek bir eylem sütunu ve
kataloğun başında tek satır olarak Sunucu kurulumu) ve bunun neden daha iyi
olacağını sordu. Dürüst yanıt, büyük kısmının daha iyi olmayacağıydı: sayfa
sahibin seçtiği görünümü (Kurulu ya da Katalog) zaten hatırlıyor ve kataloğa
geçerken onu katlıyor; bu yüzden geniş yeniden tasarım **geri çekildi**. O
incelemeden üç şey kaldı ve sahip tam olarak bu üçünü onayladı: taslağın, bir
kaldırmanın bu sunucuda neyi etkileyeceğini adıyla söyleyen kaldırma penceresi
(1, "evet"); ilk ziyaretin D-010'un zaten söz verdiği Kurulu görünümü açması (2,
b seçeneği); Sunucu kurulumunun Ayarlar'dan çıkması (3, "evet").

Kabul edilen kurallar:

1. **Sunucu kurulumu bir işlemdir ve Sunucu altında yer alır.** Yukarıdaki 4.
   kuralın ikinci düşüncesini ("Sunucu kurulumu Ayarlar'da kalır") tersine
   çevirir. Sahibin okuması: sunucunun ne yapacağını seçmek ve genişletmek,
   bileşen kurmak gibi sunucuda yapılan bir iştir; panelin nasıl davranacağına
   dair bir ayar değildir. Sunucu grubunda, **Bileşenler'in hemen altında**,
   "Sunucu kurulumu" / "Server setup" adlı kendi öğesidir; adresi
   `/server-setup`. Ayarlar → Sunucu kurulumu'nun gösterdiğini gösterir:
   kurulumun durumu, sunucunun kullanım amacı, "Bu sunucunun kurulumunu
   genişletin" düğmesi ve kurulum sihirbazının bağlantısı (`/setup`, değişmedi). Pano'daki
   "Sunucunun kullanım amacı" kartı ona bağlanır. `/settings?section=setup`,
   alpha.98 taşımaları gibi `/server-setup` ile değiştirilir (fazladan geçmiş
   kaydı yok); eski bölüme giden her ürün içi bağlantı ve Panel'in sunucu
   kurulumunun yönettiği bir kayıt için verdiği yönlendirme eylemi yeni sayfayı
   gösterir. **Ayarlar'da üç bölüm kalır:** Panel adresi ve HTTPS, Lisans,
   CelikPanel güncellemeleri. **Roller değişmez:** önceki gibi yalnız
   yöneticiler.
2. **İlk ziyaret Kurulu görünümü açar (D-010).** Hiçbir şey saklı değilken
   Bileşenler sayfası tam kataloğu açıyordu. Artık kataloğu yalnız hatırlanan bir
   "katalog" seçimi açar; olmayan, bilinmeyen ya da okunamayan seçim Kurulu
   görünümü açar ve saklı bir seçim yine geçerlidir. Sayfada başka hiçbir şey
   değişmez: satırlar, düğmeleri (yeniden başlat, durdur, Yönet ve kaldır görünür
   kalır; "⋯" menüsü yok), hazır paket şeritleri, rozetler ve katlama olduğu
   gibidir.
3. **Kaldırma penceresi neyin etkileneceğini adıyla ve neyin yerinde kaldığını
   söyler.** Taslağın onaylanan tek parçasıdır. Kuralları:
   - **Önce doğruluk.** Neyin silindiğine ya da kaldığına dair her cümle taslağı
     değil koddaki kaldırma yolunu izler (taslaktaki "yapılandırma dosyaları
     yerinde kalır" yanlıştı: purge onları siler). Panel, kendi kaldırmasının
     kullandığı küçük koşullarla yanıt verir; böylece pencere kaldırmanın
     yapmadığı bir şeyi vaat edemez.
   - **Panel'in zaten tuttuğundan.** `GET /api/v1/service/removal-impact` (yalnız
     yöneticiler, salt-okunur: yeni Agent çağrısı ve değişiklik yok; sunucu
     profili önbellekteki Agent bilgilerinden gelebilir, güvenlik duvarı motoru
     için duvarın durumu var olan okumayla, sınırlı biçimde okunur) kataloğu,
     saklanan bileşen kaydını, Panel veritabanını ve panel sertifikası dosyasını
     okur. Şunları adlandırır: Panel'in bileşen üzerinde barındırdıkları (web
     sunucusu için siteler; bir çalışma ortamı için PHP ya da Node.js siteleri;
     bir motor için veritabanları; Postfix ve Dovecot için e-posta adresleri ve
     yönlendirmeler; bir spam süzgeci için posta alan adları; Roundcube için posta
     kutuları; certbot için Panel'in onunla yenilediği, yenilenmesi duracak
     sertifikalar — sahip kendi sertifikasını sunmuyorsa panelinki, sunucu kurulumunun verildiğini kaydettiği posta sunucusu sertifikaları ve otomatik yenilemesi açık her Let's Encrypt site
     sertifikası —; güvenlik duvarı motoru için panel güvenlik duvarının
     durumu (açık ve N açık port, kapalı ya da bilinmiyor) ve CelikPanel'in onu
     artık uygulayamayacağı, değiştiremeyeceği ve geri yükleyemeyeceği, yüklenmiş
     kuralların sunucu yeniden başlayana dek kalabileceği; WireGuard için Panel'in
     etkin tuttuğu, artık bağlanamayacak VPN eşleri; vsftpd için kaydettiği FTP
     hesapları; Redis, Valkey
     ve Memcached için yalnız CelikPanel'in onları hangi sitelerin kullandığını
     kaydetmediği), sunucu kurulumunun certbot'u ve güvenlik duvarı motorunu
     hazırladığı her sunucuda gerektirdiği, ona ihtiyaç duyan kurulu bileşenler,
     eksik bıraktığı hazır paketler ve güvenlik duvarından istediği portlar;
     yalnız bunların hiçbiri yoksa "CelikPanel bu sunucuda X bileşenine bağlı başka hiçbir şey kaydetmiyor". Barındırılan adlardan en çok beşi, ardından "+N tane daha"
     gösterilir.
   - **Bir kez, görünür biçimde okunur ve asla engel olmaz.** Pencere açılınca
     okur, okuduğunu gösterir; 10 saniye sonra, bir retle ya da okuyamadığı bir
     yanıtla etkilerin okunamadığını, hiçbir şeyin değiştirilmediğini,
     kaldırmanın yine izinli olduğunu ve önce nereye bakılacağını söyler (Tekrar
     dene yine yalnız okur). Başarısız bir okuma kaldırmayı asla tutmaz; okuma
     asla bir değişiklik başlatmaz.
   - **Var olan korumalar kalır.** Panel, bildiği siteler, veritabanları ya da
     e-posta adresleri bir bileşene bağlıyken onu kaldırmayı yine reddeder ve
     pencere bunu o adların yanında söyler; çakışma ve gereksinim retleri, DNS
     motorlarının kendi akışı (BIND ve PowerDNS bu pencerede yok) ve Kaldır'dan
     sonraki işlem yönlendirmesi değişmedi. Kaldır düğmesi çerçeveli tehlike
     düğmesi olarak kalır.

Kaldırma yolunun yaptığı, pencerenin söylediği biçimde (bu adayın kodundan):

| Bileşen sınıfı | Silinenler | Yerinde kalanlar |
| --- | --- | --- |
| Paketli bileşen (hizmetler ve araçlar) | Katalog birim adlandırıyorsa birimleri durdurulur ve devre dışı bırakılır (`systemctl disable --now`); birimi olmayan bir araçta (certbot, nftables, phpMyAdmin, phpPgAdmin) durdurulacak bir şey yoktur. Ardından katalog paketleri kaldırılır. **apt** (`apt-get purge -y --auto-remove`): paketler, yapılandırma dosyaları ve artık hiçbir şeyin ihtiyaç duymadığı, otomatik kurulmuş her paket — yalnız bu bileşen için kurulanlar değil, sistemin tamamında. **pacman** (`pacman -Rns --noconfirm`): paketler, yapılandırma dosyaları ve başka hiçbir şeyin ihtiyaç duymadığı bağımlılıkları. Güvenlik duvarı açıksa yalnız onun istediği portlar kapanır (güvenlik duvarı kurulu kalanlara göre yeniden ayarlanır). | CelikPanel kendisi başka hiçbir şey silmez (site dosyaları, posta kutuları, veritabanı dosyaları). Bir paketin soru sormadan çalışan kendi kaldırma betiği o paketin oluşturduğu verileri silebilir; hangilerinin silineceği CelikPanel'in değil dağıtım paketinin kararıdır ve iddia edilmez. |
| apt'ta PHP-FPM ve PostgreSQL | Yukarıdaki gibi; ayrıca katalogdaki üretici desenine uyan her kurulu sürüm paketi (`php8.3-fpm`, `postgresql-17`, …) ve yol arkadaşları. pacman'da yalnız katalog paketleri. | Yukarıdaki gibi. Bir veritabanı motoru için pencere, silinenlerin altında kendi uyarı satırıyla, veritabanı dosyalarının paketin kaldırma betiğince silinebileceğini ve önce yedek alınmasını söyler. |
| Spam süzgeci (Rspamd, SpamAssassin) | Paketli bileşen gibi; ayrıca Postfix'in süzgeç zinciri onsuz yeniden kurulur. | Yukarıdaki gibi; posta kalır, artık spam denetiminden geçmez. |
| phpMyAdmin, phpPgAdmin | Paketli bileşen gibi; ayrıca veritabanı araçlarının sunucusu onsuz yeniden ayarlanır. | Yukarıdaki gibi. |
| certbot | Paketli bileşen gibi (birimi yok). Panel'in onunla yenilediği sertifikaların (sahip kendi sertifikasını sunmuyorsa panelinki, kaydedilmiş posta sunucusu sertifikaları, otomatik yenilenen site sertifikaları) yenilenmesi durur. | Yukarıdaki gibi. |
| Güvenlik duvarı motoru (nftables) | Paketli bileşen gibi (birimi yok). CelikPanel panel güvenlik duvarını artık uygulayamaz, değiştiremez ve geri yükleyemez; yüklenmiş kurallar sunucu yeniden başlayana dek kalabilir. | Yukarıdaki gibi. |
| Bu paket yöneticisi için katalog paketi olmayan bileşen (pacman'da Redis, SpamAssassin, phpPgAdmin) | Hiçbir şey: Agent reddeder ("bu sistemde henüz otomatik olarak kaldırılamaz") ve pencere purge yerine bunu söyler. | Her şey. |
| Roundcube | Paket değildir: `/var/lib/celikpanel-webmail` altındaki uygulama ağacı (ayarları ve adres defteri veritabanı dahil) ve `/webmail/` adresini sunan web sunucusu kaydı. | Posta kutuları ve Maildir verileri, diğer siteler ile Nginx ve PHP-FPM paketleri. |
| Reddedilen kaldırma (Panel'in bildiği siteler, veritabanları ya da e-posta adresleri) | Hiçbir şey: Panel adlarıyla reddeder. | Her şey. |

Bunu kurarken alınan uygulama kararları (sahip kararı sanılmasın diye
belirtilir):

- **Roundcube kalıntı teklifi görünür kalır.** Bu teklif kurulu olmayan Roundcube
  satırındadır. İlk görünüm Kurulu olunca düğmenin arkasında saklanırdı; bu
  yüzden Panel'in yarım kalmış bir kurulum ya da kaldırma kaydı teklifi
  yaptıkça Kurulu görünüm de Roundcube satırını gösterir.
- **Engelleyen barındırma türleri var olan bekçinin kendi yanıtıdır**
  (`serviceDependents`); spam süzgeci için posta alan adları ve Roundcube için
  posta kutuları yalnız pencere için sayılır ve asla engellemez. Hiçbir bekçi
  eklenmedi ya da gevşetilmedi.
- **Kaldırmanın reddedileceğini söyleyen bir okuma Kaldır düğmesini kapatmaz:**
  yetki Panel'dedir, gerçekler okuma ile basma arasında değişebilir ve ret,
  önceden olduğu gibi pencerede gösterilir.
- **Paket yöneticisinin komutu** silinenler cümlesinin altında adlandırılır (yalnız
  apt ve pacman için); böylece sahip neyin çalıştığını tanır.

**İnceleme düzeltmeleri (22 Ekim 2026, her yayından önce).** Adayın bağımsız
incelemesi kaldırma davranışını değişmemiş buldu ve şunları istedi; kaynaktadırlar:

- "Bu sunucuda başka hiçbir şey X bileşenine bağlı değil" cümlesi, sunucu
  kurulumunun gerektirdiği ve Panel'in her sertifikayı onunla yenilediği ve
  güvenlik duvarını onunla uyguladığı certbot ile güvenlik duvarı motoru için
  yanlıştı; pencere artık o sertifikaları ve güvenlik duvarının durumunu
  adlandırır ve ikisini de sunucu kurulumunun gerektirdiğini söyler. Redis,
  Valkey ve Memcached için yalnız CelikPanel'in onları hangi sitelerin
  kullandığını kaydetmediğini söyler.
- apt'ın `--auto-remove`'u sistemin tamamını kapsar; pencere bunu paket ailesine
  göre söyler.
- Veritabanı uyarısı "yerinde kalanlar" değil, silinenlerin altında kendi
  satırıdır.
- Birimi olmayan bir araç için durdurulduğu söylenmez.
- Katalog paketi olmayan bir paket ailesinde, Agent'ın reddettiği gibi kaldırmanın
  reddedileceği söylenir; purge ve komut yoktur.
- Roundcube'un kalanlar cümlesi Nginx ve PHP-FPM paketlerini adlandırır
  (`/webmail/` kaydı silinir); Roundcube için başarısız bir okuma, paket purge'ünü
  değil kaldırmanın sildiğini gösterir.
- Agent'ın kaldırma ve güncelleme testleri değiştiği için Agent testleri
  (Agent paketinin kendi testleri, temiz bir mount ad alanında) koşturulur ve kaydedilir.

**İkinci inceleme turu (22 Ekim 2026, her yayından önce).** Yeniden denetim
şunları istedi; kaynaktadırlar: kendi hatasını ya da hata kodunu taşıyan bir
güvenlik duvarı yanıtı ya da okunamayan bir motor "bilinmiyor"dur, asla bir durum
değildir; güvenlik duvarı motorunun cümlesi `nft -f` ile yüklenmiş bir kural
kümesi için doğru olanı söyler (CelikPanel panel güvenlik duvarını artık
uygulayamaz, değiştiremez ve geri yükleyemez; yüklenmiş kurallar sunucu yeniden
başlayana dek kalabilir); WireGuard, Panel'in etkin tuttuğu ve artık
bağlanamayacak VPN eşlerini, vsftpd kaydettiği FTP hesaplarını adlandırır;
certbot listesi yalnız yenileme işinin yenilediği site sertifikalarını sayar
(`letsencrypt`, `auto_renew = 1`, `status = 'active'`), sahip kendi sertifikasını
sunuyorsa (`CELIKPANEL_TLS_CERT`/`KEY` ya da yönetilmeyen bir TLS dizini) panel
sertifikasını dışarıda bırakır ve bir posta sunucusu sertifikasını yalnız sunucu
kurulumu onun sertifika adımını başarılı olarak kaydettiyse adlandırır; güvenlik
duvarı okuması bir güvenlik duvarı değişikliğini asla beklemez (kilit tutulurken
hemen "bilinmiyor" der); boş durum CelikPanel'in kaydettikleriyle daraltıldı:
"CelikPanel bu sunucuda X bileşenine bağlı başka hiçbir şey kaydetmiyor".

Bunun yetkilendirmedikleri: Bileşenler sayfasında başka hiçbir değişiklik
(satırlar, düğmeler, şeritler, rozetler ve katlama olduğu gibi kalır), "⋯" menüsü,
yeni Agent çağrısı, kimin neyi görüp yapabileceğinde değişiklik ve sahip kurulu
sunucularda görmeden önce onlar hakkında iddia yok. Kurulu paneller yine yalnız
sahipleri tarafından, panelin kendi güncelleme ekranından güncellenir.

Diğerleriyle ilişkisi. D-010 (önce kurulu) artık ilk ziyaretin yaptığıdır. D-024:
pencere ne olacağını ve kimin hareket edeceğini söyler; doğrulanmış bir ret,
karşılanmamış bir önkoşul ve bilinmeyen bir okuma ayrı tutulur. D-033: okuma
görünür ve sınırlıdır. D-034: bir kaldırmanın eksik bıraktığı hazır paket
adlandırılır. D-022: kaldırma cümleleri paket yöneticisinin ve CelikPanel'in
yaptığını söyler; sahibin kendi yapılandırmasının yaptığı hakkında hiçbir şey
söylemez.

Yapılmayan: hiçbir şey kurulu bir sunucuda ölçülmedi; bir dağıtım paketinin kendi
kaldırma betiğinin neyi sildiği okunmaz ve iddia edilmez.

### Değişiklik (23 Ekim 2026, saat tarihi): bir kaldırma adını söylemediği hiçbir şeyi kaldırmaz; her PHP aracı sahibin seçtiği PHP sürümünde çalışır

*Frankfurt olayından sonra sahibin 23 Ekim 2026 kararları ("Tüm düzeltmeleri 104'te yap. 105'e bir şey bırakma."); v0.1.0-alpha.104 adayı için kaynakta; kurulu bir sunucuda hiçbir şey ölçülmedi*

Ne oldu. 23 Ekim 2026'da Frankfurt (Debian 13, alpha.103, PHP-FPM 8.6, 8.5 ve 8.4
kurulu; 8.4 Debian 13'ün kendi PHP'si) phpMyAdmin, phpPgAdmin, Roundcube ve Web
postası paketini kurulu gösteriyordu. Sahip sürüm listesinden PHP-FPM 8.4'ü kaldırdı.
Pencere yalnızca şunu söyledi: "Bu sürümü ve yol arkadaşı paketlerini sunucudan
kaldırır. Üzerinde çalışan bir site varsa panel reddeder ve onları listeler." İki
dakika sonra phpMyAdmin "Kurulu değil", web postası "doğrulanmadı — Roundcube kurulu,
ama Unix soketi çalışma ortamı hazır değil" durumundaydı. Arada başka hiçbir şey
yapılmamıştı. Sunucunun kendi apt geçmişi nedenini gösterdi: Agent, PHP 8.4
paketlerinde `apt-get purge -y --auto-remove` çalıştırdı ve apt sormadan phpmyadmin'i,
dağıtımın php-fpm, php-mysql, php-mbstring, php-pgsql ve php-xml paketlerini,
php8.4-readline'ı, dbconfig-common'ı, dbconfig-mysql'i ve yaklaşık 35 kütüphaneyi de
sildi — silinene bağlı olan her şeyi ve yalnızca onların ihtiyaç duyduklarını.
Roundcube, phpMyAdmin ve phpPgAdmin `php8.4-fpm.sock` üzerinden sunuluyordu
(CelikPanel'in onlar için yazdığı yapılandırma tek bir PHP-FPM soketini adlandırır);
8.4 gidince web postası ve phpPgAdmin yanıt vermez oldu, phpPgAdmin yine de "Kurulu"
görünüyordu. Kaldırma, hiçbir şey söylemeden başka bir bileşeni götürmüş ve iki
bileşeni daha bozmuştu; bu, yukarıdaki alpha.100 değişikliğinin 3. kuralına ("önce
gerçek; etkilenen, adıyla") ve D-022'ye aykırıdır.

Benimsenen kurallar:

1. **Bir kaldırma adını söylemediği hiçbir şeyi kaldırmaz; kararı paket
   yöneticisinin kendi benzetimi verir ve beklenen kümenin dışındaki her şey
   kaldırmayı reddeder.** Herhangi bir paket kaldırmadan önce (her bileşen, her
   sürüm, apt ve pacman; DNF önizlemesi erişilmez kalır) Agent, hiçbir şeyi
   değiştirmeden paket yöneticisine kaldırmanın ne yapacağını sorar. Beklenen küme:
   kaldırmanın adını söylediği paketler; o çalışma ortamı sürümünün onsuz
   çalışamayan paketleri (php8.4-common yanında "php8.4-…", postgresql-17 yanında
   "postgresql-17-…"); ve yalnızca bu kaldırmanın kullanılmaz hâle getirdiği,
   otomatik kurulmuş paketler. Paket yöneticisinin kaldıracağı başka her şey —
   başka bir katalog bileşeninin paketi (phpMyAdmin), kaldırılan bileşenin kendi
   dağıtım paketi (php-fpm; bu, bu sürümün sistemin varsayılanı olduğu anlamına
   gelir), kaldırılana bağlı başka her paket (php-mysql) ya da kuracağı her şey —
   kaldırmayı adlarıyla reddeder ve hiçbir şey değişmez: hiçbir hizmet durdurulmaz.
   Kaldırmadan önce zaten kullanılmayan paketlere (eski bir çekirdek) dokunulmaz.
   Gerçek kaldırma benzetimi yapılan tam listeyi adlandırır, hemen öncesinde
   yinelenen son benzetimle aynı argümanlarla ve aynı paket kilidi altında çalışır;
   arada yanıt değiştiyse (başka bir paket işlemi çalıştıysa) reddedilir.
2. **Her PHP aracı tek bir PHP sürümünde çalışır ve onu sahip seçer.** Web postası
   (Roundcube), phpMyAdmin ve phpPgAdmin'in her biri kurulu bir PHP-FPM sürümüne
   bağlıdır. Bağ, aracın üretilen nginx yapılandırmasının adını verdiği sürümdür
   (sahibin o satırda yaptığı değişikliğe saygı gösterilir); Agent bunu hatırlar,
   böylece kaldırılıp yeniden kurulan araç oraya döner. Yeniden yapılandırma aracı,
   o sürüm kurulu kaldıkça orada tutar ve aracın PHP eklentilerinin o sürüm için var
   olmasını sağlar; aracı bir daha asla en yeni sürüme taşımaz (eski seçim alfabetik
   olarak son soketti; bu, 8.9'u da 8.10'dan sonraya koyuyordu). Henüz bağı olmayan
   araç dağıtımın varsayılan PHP'sini alır (kurulu php-* paketlerinin bağlı olduğu
   sürüm, sonra sistemin php-fpm ve php seçenekleri), yoksa eklentileri olan en yeni
   sürümü. Sahip aracı **aracın kendi Yönet penceresinde** ("PHP sürümü") taşır;
   tıpkı bir sitenin ayarlarındaki PHP sürümü gibi; yalnız kurulu sürümler sunulur.
   Taşıma, eksikse aracın eklentilerini o sürüm için kurar, o PHP'ye eklentileri
   sorar, aracın nginx yapılandırmasını yazar, denetler, nginx'i yeniden yükler ve
   sonra aracın kendi soketi üzerinden kendisi olarak yanıt vermesini ister (gerçek
   sayfası; geçit hatası, ham PHP dosyası ya da kendi "eklenti eksik" sayfası değil);
   olmazsa önceki yapılandırma geri konur ve pencere bunu söyler: neden, aracın
   yeniden önceki sürümünde çalıştığı ve ne yapılacağı. Bir seferde tek değişiklik,
   yalnız yöneticiler, görünür bekleme; yinelenen istek yanıtlanır, iki kez
   çalışmaz (D-029).
3. **PHP-FPM sürüm listesi her sürümü kimin kullandığını söyler**: "Kullanan: 2 site,
   phpPgAdmin, Roundcube" ya da hiçbir şeyin kullanmadığı; Panel'in site kayıtlarından
   ve Agent'ın bağlarından.
4. **Sitelerin ya da araçların kullandığı bir PHP sürümünü kaldırmak onları
   adlandırır ve hiçbir şey değişmeden reddedilir.** Siteler her sitenin ayarlarından
   taşınır (pencere onları adlandırır ve Alan Adları'na bağlantı verir). Araçlar için
   pencere "PHP [sürüm] sürümüne taşı, sonra kaldır" seçeneğini sunar: her araç tek
   tek taşınır ve orada kanıtlanır; sürüm ancak her taşıma çalıştığında (ve paket
   yöneticisinin benzetimi kaldırmayı hâlâ kabul ettiğinde) kaldırılır; ilk başarısız
   taşımada hiçbir şey kaldırılmaz ve pencere tam olarak hangi araçların artık yeni
   sürümde çalıştığını, hangisinin nereye geri konduğunu ve hangilerinin denenmediğini
   söyler.
5. **Her kaldırma penceresi paket yöneticisinin yanıtını düğmeden önce söyler:**
   kaldıracağı tam paketleri; ayrıca neyi alacağını, paket ve bileşen adıyla ve
   sonraki adımla ("önce phpMyAdmin'i kaldırın ya da PHP-FPM 8.4'ü tutun"); sürüm
   sistemin varsayılanı olduğunda bunu; ve denetim çalışamadığında (başka bir paket
   işlemi sürüyordu ya da 30 saniyede yanıt gelmedi) kaldırmanın basıldığında yeniden
   denetleneceğini ve başka bir şey alacaksa reddedileceğini. Panel'in reddedeceğini
   zaten bildiği kaldırma sunulmaz; başarısız bir okuma kaldırmayı asla tutmaz.
6. **PHP sürümü eksik olan bir PHP aracı düz "kurulu" gösterilmez**: Bileşenler
   satırı nedeniyle (sürüm çalışmıyor ya da artık kurulu değil) ve çaresiyle (Yönet'te
   bir PHP sürümü seçin) "Hazır değil" der.

Bu, kaldırmalar için alpha.100'ün apt temizliğinin "artık hiçbir şeyin ihtiyaç
duymadığı otomatik kurulmuş her paketi (sistem genelinde)" aldığı sözünün yerine
geçer; ayrıca o değişikliğin, Kaldır düğmesinin okumayla asla devre dışı
bırakılmadığı uygulama notunun da yerine geçer: benzetimin zaten gösterdiği bir ret
Kaldır'ı kullanılamaz yapar, başarısız ya da bilinmeyen bir okuma yine asla yapmaz.
alpha.100 değişikliğinin diğer kuralları geçerlidir.

Bunun izin vermedikleri: bir aracın başka bir PHP sürümüne otomatik taşınması (yalnız
sahibin açık seçimi taşır), sitelerin PHP sürümü seçme biçiminde değişiklik, sahibin
kurmadığı bir PHP sürümünün kurulması, güncelleyicide değişiklik, sahip görmeden
kurulu sunucular hakkında iddia. Kurulu paneller hâlâ yalnız sahipleri tarafından,
panelin kendi güncelleme ekranından güncellenir.

Diğerleriyle ilişkisi. D-022: yerel yapılandırma gerçek kalır ve sahibin değişikliği
kazanır. D-024: retler nedeni, kimin harekete geçeceğini ve sonraki adımı söyler;
doğrulanmış ret, karşılanmamış önkoşul ve bilinmeyen yanıt ayrı tutulur. D-025:
Agent'ta değişen bir kaldırma yolu; kanıtı kaydedilir (RESILIENCE-CONTRACT,
alpha.104). D-029: taşıma yolu bir istek kimliği taşır. D-036: Debian 13'ün phpMyAdmin
sürümü `4:5.2.2-really+dfsg-1+deb13u1` artık 5.2.2 okunur (alpha.101 kuralı
"really"den sonra bir rakam bekliyordu).

İnceleme düzeltmeleri (2026-10-23, aynı aday, bağımsız bir incelemeden sonra;
sahibin kuralı: her şey alpha.104'te). Yukarıdaki kuralları keskinleştirir: (a) "onları
taşı, sonra kaldır" herhangi bir aracı taşımadan önce paket yöneticisi kaldırmayı
başka bir şey almadan kabul etmiş olmalıdır, aksi hâlde hiçbir şey taşınmaz; araçlar
taşındıysa yanıt onları retle ya da "bilinmiyor" ile birlikte adlandırır; (b) bir
kaldırma başka PHP sürümlerinin eklenti paketlerine ve zaten kullanılmayan paketlere
dokunmaz, hizmetleri durmadan önce ve durduktan sonra yeniden denetlenir ve yanıt
değiştiyse onları yeniden başlatır; (c) bir kurulum asla paket kaldırmaz; (d) bir
aracın CelikPanel dışında, kendi üretilen biçiminin ötesinde değiştirilmiş
yapılandırması yeniden üretilmez (D-022) ve gitmiş hatırlanan sürüm kullanılmaz;
(e) PHP eklenti paketleri sürümü için eksik olan araç "Hazır değil"dir (paket
varlığı, yüklendiğinin kanıtı değil); (f) kurulumdan sonra başka bir aracın PHP
sorunu uyarıdır, başarısız kurulum değil; (g) başarısız taşımanın nedeni okurun
dilinde söylenir, beklenmeyen biri alıntılanır; (h) kaldırma yolu bir istek kimliği
taşır (D-029).

Yapılmayanlar: kurulu bir sunucuda hiçbir şey ölçülmedi; gerçek sistem kanıtı atılabilir
iki Debian 13 makinesidir (RESILIENCE-CONTRACT, alpha.104); pacman yolu yalnız bileşen
testleriyle kapsanır; DNF yolu erişilmez kalır.

### Değişiklik (24 Ekim 2026, saat tarihi): kaldırma hangi sürüm olduğunu söyler; pencere yanıtla başlar

*Sahibin 24 Ekim 2026 Frankfurt gözlemi (alpha.105); v0.1.0-alpha.106 adayı için kaynakta; yalnız web, kurulu bir sunucuda hiçbir şey ölçülmedi*

Sahip PHP-FPM satırının kaldır düğmesine bastı (8.6, 8.5 ve 8.4 kurulu) ve hangi
sürümün kaldırılacağını sordu: başlık "PHP-FPM kaldırılsın mı?" diyordu, bütün
sürümlerin gideceğini yalnız ortadaki bir cümle söylüyordu. Panel haklı olarak
reddetti (phpPgAdmin ve Roundcube PHP-FPM üzerinde çalışıyor; apt phppgadmin'i de
götürürdü), ama ret listenin ortasındaydı; pencere yine de kaldırmanın neyi
"götürdüğünü" söylüyor, aynı araçları iki kez sayıyor, paket yöneticisi diliyle
konuşuyor ve kullanılamaz görünmeyen bir Kaldır düğmesi gösteriyordu. Kurallar:
(1) birden çok sürümü olan çalışma ortamı önce "Hangi sürüm kaldırılsın?" diye
sorar; her sürüm için kimin kullandığıyla bir seçim ve altta ayrı "Tüm sürümler
(8.6, 8.5, 8.4)"; sürüm kendi kaldırmasına, Tüm sürümler bütün bileşenin
kaldırmasına geçer; tek sürümde soru yoktur ve başlık onu adlandırır; her başlık
neyin gideceğini söyler. (2) Her kaldırma penceresi önce yanıtı söyler (D-024):
şimdi kaldırılabilir mi, nedenler her biri bir kez ve sade sözle, sonra sahibin
yapabilecekleri ve nasıl yapacağı (adı geçen bileşenleri kaldırmak, siteleri
taşımak, tutmak); paket listesi, apt ya da pacman'ın yaptığı ve paket
yöneticisinin açıklaması katlanmış Ayrıntılar altındadır; reddedilen kaldırma için
asla bir şeyi "götürür" denmez. (3) Reddedilen kaldırma Kapat sunar; tehlike
düğmesi görünür biçimde kullanılamaz ve "Kaldırılamaz" der. Bu, alpha.100
değişikliğinin Panel'in yanıtına bıraktığı barındırılanlar (siteler, veritabanları)
reddini de kapsar. Node.js sürümleri yalnız sürüm listesinden kaldırılır;
PostgreSQL için Panel'de sürüm başına kaldırma yoktur.

---


## D-036 · Gösterilen sürüm paket veritabanının kendi yanıtıdır; bilinmeyen, bilinmeyen olarak kalır

*21 Ekim 2026 (saat tarihi) · Sahibin 17 Ekim 2026 tarihli alpha.94 ekranlarının ve D-034 değişikliğindeki açık işin ardından; v0.1.0-alpha.97 adayı için kaynakta; kurulu bir sunucuda hiçbir şey ölçülmedi*

Sahibin alpha.94 ekran görüntüleri, Bileşenler sayfasında tek sürümlü her bileşen
için (Nginx, PostgreSQL, MariaDB, Postfix, Dovecot, Certbot, Redis…) çizgilerden
oluşan bir sürüm sütunu gösteriyordu. alpha.95 sütunu kaldırdı ve "her bileşen için
kurulu paket sürümünü okumak"ı açık iş olarak kaydetti (D-034 değişikliği): Panel
sürümleri yalnız çok sürümlü çalışma ortamları (PHP-FPM, Node.js) için biliyordu,
Agent'ın birim başına ikili yoklaması bir paket sürümü değildir ve güvenilmezdir,
Certbot gibi araçların ise hiç birimi yoktur.

Benimsenen kurallar:

1. **Kaynak, salt-okur sorulan sistem paket veritabanıdır.** Yeni bir Agent
   okuması, `Agent.InstalledPackageVersionsV1`, mevcut bileşen taraması sırasında,
   tarama başına bir kez, tüm katalog için tek bir sorgu olarak çalışır: Debian ve
   Ubuntu'da sabit biçimli (paket, durum, sürüm) `dpkg-query -W`, Arch'ta
   `pacman -Q`. Sabit argv, kabuk yok, ağ yok, 10 saniyelik zaman aşımı ve 8 MiB
   çıktı sınırı. Hangi paketlerin sayılacağı Agent'ın kendi derlenmiş
   kataloğudur (bu ailede bileşenin paketleri; PostgreSQL'in `postgresql-17`
   gibi sürümlü bir depo kalıbı olan bir bileşen için, herhangi biri kuruluysa
   sürümlü paketler, çünkü o zaman katalog adı sürümü sunucunun sürümü olmayan bir
   üst pakettir). Okuma hiçbir paket yöneticisi kilidi almaz.
2. **Bir paket işlemi sürerken asla.** Sorgudan önce Agent süreç tablosunu ve
   (pacman ve apt/dpkg araçları dahil) çekirdeğin kilit tablosunu apt/dpkg kilit
   dosyaları için okur; kendisi, bir an için bile, asla kilit almaz. Öldürülmüş bir
   pacman'in geride bıraktığı kilit dosyası, canlı bir pacman süreci yoksa sayılmaz;
   yoksa sürümler bir daha hiç okunmazdı. Meşgul bir sunucu ya da
   durumu okunamayan bir sunucu "meşgul" yanıtı verir: hiçbir şey sorgulanmaz ve
   Panel önceki taramanın sürümlerini, korunmuş olarak ve okundukları zamanla
   birlikte işaretleyerek tutar.
3. **Bilinmeyen, bilinmeyen olarak kalır.** Kurulu katalog paketi olmayan bir bileşen
   listelenmez. Yalnız veritabanının kurulu dediği paket sayılır: dpkg'de, istenen
   işlem ne olursa olsun ("ii", tutulan "hi", kaldırma ya da temizleme için
   işaretlenen "ri"/"pi") mevcut durumu "i" ve hata bayrağı boş olan paket; "yeniden
   kurulum gerekli" ("iiR"), yarım kurulu, açılmış ya da yalnız yapılandırması kalmış
   ("rc") paketin sürümü yoktur. 64 karakterden
   uzun ya da bir paket veritabanının yazmayacağı karakterler taşıyan bir sürüm
   metni atılır, asla kısaltılmaz; bu durumda bileşenin tüm sürümü bilinmez olur.
   Paketleri farklı sürümler bildiren bir bileşen "birden çok sürüm"dür, asla
   bunlardan biri değildir. Başarısız ya da desteklenmeyen bir okuma her sürümü
   bilinmez bırakır. Tahmin edilmiş sürüm yok, çizgi yok, "varsayılan" yok.
4. **Ne gösterilir.** Basıldığı gibi tam sürüm, epoch ve revizyon korunarak
   (`1:11.8.3-0+deb13u1`), bileşenin kendi sayfasında kaynağıyla birlikte
   ("Sistemin paket veritabanından okundu."; korunan bir okuma için, ne zaman
   okunduğu ve son denetimde paket yöneticisinin meşgul olduğu), her bileşen
   sayfasının paylaştığı başlıkta. Bileşenler satırlarında Panel'in kısa biçimi,
   yani yukarı akış kısmı (`11.8.3`: epoch, son tireden sonraki paketleme
   revizyonu ve `+dfsg`/`~deb` gibi bir sonek çıkarılmış), mono yazı tipinde,
   ipucunda tam sürüm ve kaynağıyla. Masaüstü sürüm sütunu yalnız sayfadaki kurulu
   bileşenlerden en az birinin paket sürümü biliniyorsa geri gelir; aksi hâlde
   alpha.95'in açıklamanın altındaki satırı kalır. Telefonda sürüm açıklamanın
   altındadır. PHP-FPM ve Node.js kopya başına sürümlerini korur ve onlara bir
   paket sürümü verilmez.
5. **Yetkilendirme ve depolama mevcut taramayı izler.** Panel yöntemi bir okuma
   olarak kabul eder (hızlı okuma zaman aşımı, ana makine yetkisi yok), diğer
   tarama okumaları gibi; tarama başına bir kez çağırır ve bir hata taramayı asla
   başarısız kılmaz. Yanıt, mevcut tarama önbelleğiyle birlikte her gözlemin
   eklemeli bir JSON alanı olarak taşınır; alpha.97'den önce yazılmış bir önbellek
   satırında bu alan yoktur ve bilinmeyen olarak okunur. Şema değişikliği yok (44).

Bunun yetkilendirmediği: hiçbir türden paket işlemi, tarama dışında sürüm
okuması, sürümler için ağ okuması, bu sürümlerden "en yeni sunulan" ya da güncelleme
önerisi ve sahibin kurulu sunucularda görmesinden önce onlar hakkında hiçbir
iddia. Kurulu paneller hâlâ yalnız sahipleri tarafından, panelin kendi güncelleme
ekranından güncellenir.

Geri kalanla ilişkisi. D-024 (bilinmeyen yok demek değildir; kimin işlem yaptığını
ve nasıl devam edildiğini söyle): korunan bir okuma bunu söyler, hiçbir şeyin
bilinmediği yerde hiçbir şey söylenmez. D-032 (panel kendiliğinden bakar): okuma
mevcut koşullu ve elle taramalara biner ve zamanlayıcı eklemez. D-034 değişikliği:
"her bileşen için kurulu paket sürümünü okumak" açık işi bu kararla kaynakta
uygulanmıştır; kurulu bir sunucuda ölçülmemiştir.

*Adayın bağımsız incelemesinden sonra aynı gün düzeltildi:* ilk biçim yalnız dpkg'nin
"ii" durumunu sayıyordu (kural 3 artık her kurulu durumu sayar); çıktı tavanını
aşan yanıt genel bir hata olarak bildiriliyor ve sorgu çalışmayı sürdürüyordu (artık
sorgunun süreç grubu tavanda öldürülür ve yanıt "okuma tavanından büyük"tür); sorgu
Agent'ın ortamını devralıyordu (artık `LC_ALL=C`, `LANG=C` ve sabit bir `PATH`,
başka hiçbir şey); kilit tablosu okuması sınırsızdı ve inode'u olmayan bir kilidi
reddediyordu (artık en çok 1 MiB ve böyle bir satır atlanır); bayat bir pacman kilit
dosyası sonsuza dek meşgul demekti (artık canlı bir pacman süreci gerekir); bir günden
uzun korunan okumanın yönlendirmesi yoktu (artık sayfa şunu ekler: "Paket yöneticisi
bir günden uzun süredir her denetimde meşgul. Sunucuda bir kurulum ya da güncelleme
çalışmıyorsa sunucu yöneticisi takılı kalmış bir paket işlemi olup olmadığına
bakmalı."); ve sayfanın kendi taraması paket veritabanı yanıt verirken Panel'in
değişiklik kirasını tutuyordu (artık sürümleri kirayı almadan önce okur; bir
işlemin sonundaki taze tarama onları yine o işlemin kirası içinde okur).

*22 Ekim 2026 notu (alpha.101 adayı), sahibin alpha.100'deki Frankfurt ekranından
sonra:* phpMyAdmin satırı "5.2.2-really…" okunuyordu. Debian'daki bir "really"
sürümü, daha eski bir yukarı akış sürümüne geri dönmek zorunda kalan bir pakete
aittir: paket yöneticisi yine bir yükseltme görsün diye "really" sözcüğünden önce
daha yüksek bir numara taşır; gerçekte dağıtılan sürüm "really"den sonra gelir.
4. maddenin kısa biçimi epoch'u ve paketleme revizyonunu çıkarıp yalnız "+" ya da
"~" işaretinde kesiyordu; bu yüzden "-really5.2.x" içinde kalıyor, dar sütun da onu
kırpıyordu. Kısa biçim artık, epoch ve revizyon önceki gibi çıkarıldıktan sonra,
"really" bir sürüm ayracından ("+", "-", "~" ya da ".") sonra geliyor ve ardından
bir rakam geliyorsa "really"den sonraki gerçek sürümdür (`5.2.2-really5.2.1-1`
5.2.1, `2:4.1.0+really4.0.9-2` 4.0.9 okunur; `4:5.2.2+dfsg-1` yine 5.2.2, `3:7.2`
7.2 okunur). İpucundaki ve bileşen sayfasındaki tam sürüm değişmedi. Kısa biçim
Panel yanıtını oluştururken türetilir; bu yüzden saklanmış bir tarama da hemen
doğru okunur; Agent ve bildirdikleri değişmedi. Bir satırdaki tek çalışma ortamı
sürümü de artık üç noktayla biter ve ipucunda tam metni vardır; böylece hiçbir
sürüm hücresi sessizce kırpılmaz. Kurulu bir sunucuda hiçbir şey ölçülmedi.

---

## D-035 · Panel güncellemeleri kendiliğinden arar ve bunu sakin bir dille söyler; güncellemeyi yalnız bir yönetici başlatır

*17 Ekim 2026 (saat tarihi) · v0.1.0-alpha.95'e güncellenirken sahibin isteği; v0.1.0-alpha.96 adayı için kaynakta; kurulu bir sunucuda hiçbir şey ölçülmedi*

Sahip kurulu sunucuları v0.1.0-alpha.95'e güncellerken güncelleme ekranında iki
şey buldu. Panel daha yeni bir sürümün var olduğunu yalnız biri "Güncellemeyi
kontrol et"e bastığında öğreniyordu; bu yüzden bir sunucu kimse uyarılmadan eski
sürümde kalabiliyordu. Ayrıca güncelleme kartının kendisi zor okunuyordu: iki
sarı kutu (alfa uyarısı ve "Bu sürüme yapılan önceki güncelleme…"), yeşil bir
"İmzalı bir güncelleme bulundu." ve Başlat düğmesinin altında aynısının ikinci bir satırı.
Sahibin sözleriyle: "belirli periodlarda güncellemeleri kontrol etmeli ve
güncelleme var diye uyarmalı. güncelleme konusunda yardımcı olmalı."

Benimsenen kurallar:

1. **Panelin kendisi zamanlanmış olarak bakar ve yalnız okur.** Panel
   işleminde (tarayıcıda değil) çalışan bir döngü, "Güncellemeleri denetle"
   düğmesiyle aynı salt-okur keşfi tek bir ortak işlev üzerinden yapar: Agent'ın
   erişilebilirliği ve yapı çifti, güncellemeyi başlatmak için sunucunun kendi
   yetkisi, Agent'ın celikpanel.net'teki imzalı sürümü denetlemesi, kimlik
   eşleşmesi ve hedefin doğrulanması. İlk denetim Panel başladıktan yaklaşık 2
   dakika sonra (artı eksi 30 saniye), sonra her 12 saatte bir (artı eksi 30
   dakika) yapılır. Bir denetim 40 saniyeyle sınırlıdır. Sunucuda hiçbir şeyi
   değiştirmez ve güncelleme kaydı yazmaz. Hazır olma okuması, hazır olma
   uç noktasınınkiyle aynıdır: Panel'in kendi değişiklik kilidini `TryLock` ile
   yalnız süren bir işlemin süreç içi denetimi için alır ve danışma Agent
   okumasından önce bırakır.
2. **Başarısız denetim geri çekilir, yeniden deneme fırtınası çıkarmaz.**
   Başarısızlıktan sonra bir sonraki deneme 1 saat, sonra 3 saat sonradır; sonra
   yine 12 saatlik dönem gelir. Başarılı bir denetim bunu sıfırlar.
3. **Bazı durumlar denetimi atlatır; atlanma bir başarısızlık değildir.**
   Denetim yapılmaz ve 1 saat sonra yeniden denenir: Panel son bir saat içinde
   bir güncelleme başlatmayı kabul ettiyse, Agent'ın danışma niteliğindeki
   hazır olma okuması süren bir değişiklik ya da doğrulayamadığı bir durum
   bildiriyorsa, bir sunucu kurulumu sunucuyu değiştiriyorsa (bir DNS kaydı
   bekleyen kurulum denetimi tutmaz) ve Agent yanıt vermiyorsa. Meşgul bir paket
   yöneticisi denetimi atlatmaz, çünkü sürüm okuması paketlere dokunmaz. Atlanma
   yalnız bellekte tutulur: asla yazılmaz ve bilinen bir sonucun yerini almaz;
   kart onu o sonucun yanında söyler. Denetim, Panel kapanırken asla çalışmaz;
   yarıda kalan bir denetim hiçbir şey tutmaz. Yeniden açılan zamanlama, iki
   dakika sonra değil, son tamamlanan denetimden 12 saat sonra sürer.
4. **Sahip kapatabilir.** Ayarlar → CelikPanel güncellemeleri içinde
   "Güncellemeleri otomatik denetle" vardır; varsayılan olarak açıktır ve
   şunu söyler: "Panel 12 saatte bir celikpanel.net'e daha yeni imzalı bir sürüm
   olup olmadığını sorar. Hiçbir şey kendiliğinden kurulmaz." Kapalı olması,
   zamanlamanın hiçbir ağ okuması yapmaması demektir; elle denetim yine çalışır.
5. **Sonuç saklanır ve kuralla temizlenir; güncelleme sırasında asla.** Son
   tamamlanan sonuç (çıktı, zaman, otomatik olup olmadığı, başarısızlık için bir
   neden kodu ve bulunan sürüm ile bulunma zamanı) Panel'in ayarlar tablosunda
   tutulur; şema değişmez. Başarısız bir denetim ondan önce bulunan sürümü
   korur; böylece geçici bir kesinti hâlâ bekleyen bir güncellemeyi gizlemez.
   **Zamanlanmış denetim, bir güncelleme kabul edilmiş ya da sürüyorken asla
   yazmaz**; düğme de yazmaz: yazmadan hemen önce Panel, hiçbir kilit tutmadan
   Agent'ın hazır olma durumunu okur; sonra değişiklik kilidini `TryLock` ile
   yalnız kendi tuttuğu bilgileri yeniden okumak (kabul edilmiş güncelleme yok,
   süren Panel işlemi yok) ve ayarı yazmak için alır; aksi hâlde sonuç bellekte
   tutulur, gösterilir ve bir sonraki boşta geçişte yazılır. (Güncelleyicinin sabitlenmiş salt-okur ön
   denetimi sırasında Panel veritabanına yazmak o denetimi durdurur.) Aynı
   sürümün başka commit'li yeniden derlemesi, Agent'ın imzalı yanıtı ne diyorsa
   odur. Başlangıçta, yönetim açılmadan önce, başka bir yapının yazdığı ya da bu
   yapının zaten olduğu ya da ondan eski olan bir sürümü adlandıran kayıt
   temizlenir; güncellemeden sonra bildirim kaybolur.
6. **Sahip nerede görür, nerede görmez.** Yalnız yöneticiler: kenar çubuğunda
   sürüm satırının altında küçük bir bağlantı ("Güncelleme var · v0.1.0-alpha.96");
   Pano içeriğinin en üstünde tek, sessiz bir satır ("İlgi istiyor" kutusunun
   içinde değil, çünkü güncelleme bir sorun değildir); "İncele ve güncelle" ve
   "Sonra hatırlat" ile (o sürüm için bu tarayıcıda 24 saat gizlenir); Ayarlar
   gezinmesinde küçük bir nokta ve aynı sözler. Güncelleme kartı, kayıtlı sonucu
   bilerek açılır. Başarısız ya da bilinmeyen bir otomatik denetim, güncelleme
   kartının dışında hiçbir şey göstermez.
7. **Asla alarm değil.** Bulunan bir güncellemenin sinyal rengi yoktur: sarı yok,
   kırmızı yok, rozet rengi yok. Başarısız denetim sakindir; neyin bilindiğini
   ve bilinmediğini, kimin işlem yapacağını (kimse ya da sahip şimdi
   denetleyebilir), sonraki denetimin ne zaman yapılacağını ve panelle barındırılan
   hizmetlerin eskisi gibi çalıştığını söyler.
8. **Güncelleme asla kendiliğinden başlamaz.** Güncellemeyi yalnız bir yönetici,
   panelin kendi güncelleme ekranından, tek dolu düğme "<sürüm> sürümüne güncelle"
   ile başlatır. Başlat, keşfi yeniden çalıştırır ve aynı hedefi ister. Kart,
   sahibe basmadan önce ne olacağını söyler (yaklaşık N saniyelik bir yeniden
   başlatma, barındırılan web siteleri, posta, DNS ve veritabanları çalışmaya
   devam eder, alfa sürümü için güncel bir yedek önerilir, hiçbir şey kendiliğinden
   başlamaz).

Sonuçları. Yeni bir okuma, `GET /api/v1/panel/update/discovery` (yalnız
yöneticiler; Agent çağrısı ve ağ okuması yok), kayıtlı sonucu, anahtarı,
zamanlanmış sonraki denetime kalan süreyi ve sunucunun saatiyle ölçülen yaşları
döndürür; anahtar aynı yola `PUT` ile kaydedilir. İlk plan bu okuma için
`/api/v1/panel/update/status` yolunu adlandırmıştı, ama o yol zaten tek bir güncelleme
isteğinin durumunu istek kimliğiyle yanıtlıyor; bu yüzden ayrı bir yol
kullanıldı. Mevcut `/update/check`, `/update/start` ve `/update/status` biçimlerini
korur; denetim artık sonucunu da kaydeder. Agent değişikliği yok, şema değişikliği
yok (44), geçiş yok. Güncelleyicinin değiştirme yolu değişmedi. Tarayıcı, kendi
kopyası 5 dakikadan eskiyse kayıtlı sonucu okur, sonra her 30 dakikada bir ve
sekme o kadar süre sonra gösterildiğinde yeniden okur; yalnız görünürken.
Sözler, durumlar ve başarısız bir denetimin nasıl sürdüğü
[OPERATION-GUIDANCE](OPERATION-GUIDANCE.tr.md) içinde, "Güncelleme keşfi ve
güncelleme ekranı" bölümündedir.

Bunun yetkilendirmediği şeyler:

- Otomatik güncelleme yok. Panel bulur ve söyler; asla kurmaz, hiçbir zamanlama,
  yoklama ya da bildirim güncelleme başlatmaz.
- Değişiklik başlatan yoklama yok. Zamanlanmış denetim ve tarayıcının her yeniden
  okuması okumadır; okuma ikinci bir değişiklik başlatmaz.
- Bulunan güncelleme için alarm rengi yok, bağlantısında rozet rengi yok.
- Başarısız bir denetim için güncelleme kartının dışında bildirim yok. Başarısız
  ya da bilinmeyen bir denetim Pano'da, kenar çubuğunda ya da Ayarlar
  gezinmesinde gösterilmez.
- Kurulu bir sunucuda ölçülmüş iddia yok. Zamanlanmış denetim gerçek bir Agent'a
  ya da gerçek sürüm sunucusuna karşı çalışmadı; Go testleri Agent'ı taklit eder,
  tarayıcı turu loopback üzerinde bir taklit kullanır.
- Hiçbir P0 işi kapanmaz ya da ilerlemez.
- Kurulu panelleri kimin güncelleyeceği değişmez: yalnız sahipleri, panelin kendi
  güncelleme ekranından.

Gerisiyle ilişkisi. D-022 (panel portal olmadan çalışır; hiçbir şey denetime
bağlı değildir), başarısız bir denetimin sakin olmasının ve hiçbir şeyi
engellememesinin nedenidir. D-024 nedeni, kimin işlem yapacağını, sonraki eylemi
ve işin nasıl süreceğini; doğrulanmış başarısızlık, karşılanmamış önkoşul ve
bilinmeyen sonuç ayrımını ister: durum satırı bir başarısızlığı (doğrulanmış
yanıt yok), bilinmeyen bir sonucu (zaman aşımı) ve bir atlanmayı (karşılanmamış
koşul) ayrı tutar. D-032 panelin kendiliğinden baktığını ve kimse bakmıyorken
çalışan bir arka plan zamanlayıcısı olmadığını söylemişti; bunu bileşen
taramasına uygulamıştı. D-035, güncelleme keşfi için sahibin açık ve dar
istisnasıdır: sunucuda 12 saatte bir tek salt-okur sürüm okuması, sınırlı,
kapatılabilir, asla değiştirme değil. D-032'nin saat kuralı korunur: tarayıcı
saatleri asla karşılaştırmaz, yaşları sunucu söyler. D-033: kartın bekleme
durumları görünürdür (düğmenin denetimi sürerken hareketli bir "denetleniyor"
satırı) ya da yapılacak bir şey olmadığını söyler.

*Aynı gün, adayın bağımsız incelemesinden sonra düzeltildi:* 5. kuralın ilk
biçimi atlanmayı yazıyordu ve bir güncelleme kabul edilirken bir sonuç
yazabiliyordu; bir DNS kaydı bekleyen kurulum denetimi tutuyordu; kabul penceresi
6 dakikaydı. Yukarıdaki kurallar düzeltilmiş olanlardır.

### Değişiklik (23 Ekim 2026, saat tarihi): güncelleme ekranı açılınca yeniden denetler; durdurulan bir güncelleme teklifi korur ve nedenini söyler

*Sahibin 23 Ekim 2026'da Frankfurt ve Boston'da (ikisi de alpha.100) gözlemleri;
v0.1.0-alpha.103 adayı için kaynakta; kurulu bir sunucuda hiçbir şey ölçülmedi*

Neden. Frankfurt'un kenar çubuğu "Update available · v0.1.0-alpha.101" diyordu;
açılan güncelleme ekranı, alpha.102 yaklaşık 10:02Z'den beri celikpanel.net'te
olduğu hâlde "Update to v0.1.0-alpha.101" sundu; alpha.102'yi ancak "Check for
updates" gösterdi. Sahibin sözleriyle: "güncellemede her zaman en son versiyonu
göstermedi … sol alt kısımda 101 nolu güncellemeyi gösterdiğinde ve onu
tıkladığımda güncelleme ekranında en son güncelleme görünmesi gerekmez miydi".
Haklıdır: kart 12 saatlik zamanlamanın saklanan sonucunu gösteriyordu ve ekranın
açılması yeniden bakmıyordu. Başlat değişmiş hedefi reddederdi
(PANEL_UPDATE_TARGET_CHANGED), yani güvensiz bir şey olamazdı; ama sahip en yeni
sürüm yerine bir hatayla karşılaşırdı. Boston'da alpha.102 güncellemesi paket
yöneticisi meşgul olduğu için hiçbir şeyi değiştirmeden durdu; bu bildirim
açıkken güncelleme kartı yalnız kurulu sürümü, "Teknik ayrıntılar"ı ve anahtarı
gösterdi; kenar çubuğu ve Ayarlar öğesi ise "Güncelleme var · v0.1.0-alpha.102"
diyordu ve bildirimin kendisi kartın paket yöneticisinin ne zaman boşaldığını
söyleyeceğini vaat ediyordu. Neden: kart, tarayıcının güncelleme kilidi
tutulduğunda durum satırını, teklifi, önceki denemeyi ve hazırlık okumasını
gizliyordu; onaylanmayı bekleyen başarısız bir sonuç da bu kilidi tutar.

Eklenen kurallar:

1. **Güncelleme ekranı açılınca, bilinen sonuç on dakikadan eskiyse ve yalnız
   otomatik denetim açıkken yeniden denetler.** Tek bir okuma,
   `GET /api/v1/panel/update/discovery/refresh` (yalnız yöneticiler), kartın her
   açılışında, saklanan sonuç okunduktan sonra bir kez. Panel yeniden karar verir:
   düğmenin ve zamanlamanın aynı keşfini, zamanlamanın atlama kurallarıyla (son
   bir saatte bir güncelleme başlatmayı kabul ettiyse, sunucu süren bir
   değişiklik bildiriyorsa, kurulum sunucuyu değiştiriyorsa ya da Agent yanıt
   vermiyorsa denetim yok) ve yazma kurallarıyla çalıştırır; tüm istek 50
   saniyeyle (denetimin kendisi 40 saniyeyle) sınırlıdır; denetim ve kaydı
   tarayıcı ayrılsa da sürer, tamamlanan okuma saklanır. On dakika: ekranı bir
   oturumda birkaç kez açan biri her seferinde celikpanel.net'e sormasın diye (kim
   açarsa açsın, ekran açılışları on dakikada en çok bir tamamlanmış sürüm okuması
   yapar; atlanan ya da sınırı içinde bitmeyen denetim hiçbir şey saklamaz, bu
   yüzden sonraki açılış yeniden deneyebilir), ama daha yeni bir sürüm çıktığında
   ve denetim çalışabildiğinde ekran 12 saatlik bir sonucu sunmasın diye.
2. **Aynı anda tek sürüm denetimi.** Zamanlama, düğme ve açılış tek bir kapıdan
   birbirini bekler. Son on dakika içinde tamamlanan bir denetimi bekleyen açılış
   yeniden okumaz, o sonucu kullanır; zamanlama, bekledikten sonra son on dakika
   içinde tamamlanan bir denetim varsa yeniden okumaz ve bu arada bir başlatma
   kabul edildiyse atlar. Düğme, beklediği denetimden sonra her zaman kendi
   okumasını yapar; zamanlamanınki gibi 40 saniyeyle sınırlıdır.
3. **Bekleme görünürdür ve bilineni korur.** Hareketli "celikpanel.net'te en yeni
   sürüm denetleniyor…", "Bu denetimden önce bilinen: …" ve "Güncelleme düğmesi
   bu yanıtı bekler, en çok bir dakika kadar. Hiçbir işlem kendiliğinden
   başlamaz."; teklif görünür kalır ve Başlat bekler. Başarısız bir denetim
   teklifi yaşıyla ve mevcut hata satırıyla korur; atlanan, zamanında yanıt
   vermeyen (sonucu bilinmeyen) ve yanıtı okunamayan (panel bir hatayla ya da
   okunamayan bir yanıtla döndü) denetim bunu ve gösterilenin ne kadar eski
   olduğunu söyler.
4. **Otomatik denetim kapalıyken hiçbir yere sorulmaz.** Ekranın açılması hiçbir
   şey sormaz; teklif "Son denetim: …" der ve sonraki denetim satırı "Güncellemeleri
   denetle"yi gösterir.
5. **Durdurulan güncelleme teklifi korur ve nedenini söyler.** Hiçbir şeyi
   değiştirmeden biten (ya da doğrulanarak geri dönen) bir güncellemenin bildirimi
   açıkken kart durum satırını, teklifi, nedeniyle önceki denemeyi ("Sunucunun
   paket yöneticisi meşguldü …"), kendi hazırlık okumasından ve sessiz yeniden
   okumasından paket yöneticisinin durumunu ("hâlâ meşgul" / "işini bitirdi") ve
   en üstte şunu gösterir: "Önce durdurulan güncellemeyle ilgili bildirimi kapatın
   ("Bildirimi kapat"); ardından bu düğme v0.1.0-alpha.102 güncellemesini
   başlatır. Hiçbir işlem kendiliğinden başlamaz." Başlat bildirimin kapanmasını
   bekler; güncelleme kilidinin kendisi değişmedi. Bildirim kilidi tutarken
   açılışta denetim yapılmaz; bildirim kapanınca, bilinen sonuç on dakikadan
   eskiyse kart denetler (bildirim açık kalırken daha yeni bir sürüm yayımlanmış
   olabilir). Başlatmadan sonraki bir saat içinde Panel bu denetimi atlanmış
   olarak yanıtlar, çünkü bir güncelleme başlatmayı kabul etmiştir; teklif o
   zaman yaşı ve bu nedenle kalır ve "Güncellemeleri denetle" sürümü hemen okur.
6. **Kenar çubuğu, Ayarlar öğesi ve Pano satırı** açılış denetiminin yanıtını
   izler: aynı ortak kopyayı okurlar ve yanıt bu kopyanın yerine geçer.

Değişmeyenler: yukarıdaki 1–8. kurallar (sakin bildirim, hiçbir şey kendiliğinden
kurulmaz, güncellemeyi yalnız bir yönetici başlatır), başlatma işleyicisinin tam
hedefi yeniden denetlemesi, Agent, güncelleyicinin değiştirme yolu ve şema (44).
Sahibin 16 Ekim 2026 talimatı gereği gerçek sistem kaydı yok. Tarayıcı incelemesinin
26. grubu yerel bir sahte sunucu kullanır; kurulu bir sunucuda hiçbir şey ölçülmedi.

---

## D-034 · Hazır paketler birer kısayoldur, ürün değildir: genel bir kayıt, her Bileşenler kategorisinin başında gösterilir, Pano'da tek bir kapı

*17 Ekim 2026 (saat tarihi) · Bileşenler sayfasının tasarım incelemesinden (alpha.92 kaynağı, 16 Ekim 2026) ve sahibin 16 Ekim 2026 tarihli taslağından sonra sahip kararı; v0.1.0-alpha.94 adayı için kaynakta; kurulu bir sunucuda hiçbir şey ölçülmedi · 17 Ekim 2026'da değiştirildi: her sunucuda aynı liste (alpha.95) · 22 Ekim 2026'da yeniden değiştirildi: paket, bir hizmet ile ona eşlik eden araçlardır (alpha.101) · 24 Ekim 2026'da yeniden değiştirildi: paketler sürümleri gösterir ve sahibe seçtirir, araçlar ilk kurulumda kanıtlanır, beklenmeyen kurulumlar önce gösterilir; v0.1.0-alpha.105 adayı için kaynakta; kurulu bir sunucuda hiçbir şey ölçülmedi*

"Tek tıkla posta paketleri" başlıklı üç kart Bileşenler'in en üstünde, bütün
bileşen satırlarının üstünde duruyordu. Telefonda ilk satır yaklaşık 1.300
pikselik kartlardan sonra geliyordu; kartlar doğrulanmış bir Webmail'i sarı
renkle "Kısmen kurulu" diye çiziyordu; ve arkasındaki fikir, bilinen sağlam bir
bileşen grubunu tek seferde kurmak, yalnız posta için vardı. Sahip aynısının
genel olmasını istedi: tek tıkla paketler gibi, ama posta, veritabanı ve web için
aynı biçimde ve bileşenlerin kendisinin durduğu yerde. WordPress ve diğer site
uygulamaları ayrı, daha sonraki bir konudur. Mağaza, edinilen ya da lisanslanan
şeylerin yeri olarak kalır (D-015 2. madde korunur: katalog tek kurulum adresidir).

Benimsenen kurallar:

1. **Hazır paket, katalogdaki mevcut bileşenlerin üzerinde bir kısayoldur.**
   Mağaza ürünü değildir, kendi başına bir bileşen değildir ve bir sayfa
   değildir. Panel'deki genel bir kayıt (`cmd/panel/bundles.go`) her paketin
   adını, kategorisini, bileşenlerini kurulum sırasıyla ve ondan sonra sunulan
   paketleri belirtir. İlk girdiler: Temel Posta, Webmail ve Spam Korumalı Posta
   (mevcut posta profilleri üzerinden); her biri isteğe bağlı bir eklentili
   PostgreSQL paketi ve MariaDB paketi (phpPgAdmin, phpMyAdmin; ikisinin de gerek
   duyduğu Nginx ve PHP-FPM ile); Nginx + PHP-FPM.
2. **Nerede.** Her Bileşenler kategorisi, başlığı ile satırları arasına "Hazır
   paketler" adlı tek bir satır çizer. Kurulu ve doğrulanmış paket "… kurulu ve
   doğrulandı" satırına daralır (Onar, varsa, bir metin bağlantısıdır); kurulu
   ama doğrulanmamış olan gri renkle "kurulu, doğrulanmadı" der ve Onar (posta)
   ya da Doğrula (veritabanı ve web) düğmesini taşır; kurulu bir paket kapsadığı
   daha küçük paketleri içerir (Webmail, Temel Posta'yı kapsar) ve en çok bir
   öneri daha izler (önce kaydın eklentisi). Engellenen paket nedenini gri
   renkle söyler, düğmesi yoktur; ana makine genelindeki bir posta reddi (DNS
   kimliği) kategori için tek satırdır ve kurulu bir posta paketi onun yanında
   görünür kalır. Bir posta paketinin son başarısız ya da süren denemesi ve
   Panel'in uyarısı altında yeniden söylenir (D-024). Posta paketleri ürünün
   kendi posta profili adlarını taşır. Pano'da her zaman gösterilen tek bir
   sakin "Hazır paketler" kartı vardır: neyin kurulu olduğunu, kaç paketin
   kurulabileceğini ve başarısız ya da süren her posta denemesini nedeniyle
   birlikte sıralar; "denetlenmedi", "denetlenemedi", "tutuluyor" ve "yakın
   zamanda değil" durumlarının her birini kendi sözleriyle söyler (hiç
   denetlenmemiş bir ana makinede denetimin kendisiyle birlikte); `/services#bundles`
   adresine açılan bir kapıdır; hiçbir şey kurmaz.
3. **Durumlar ve eylemler Panel'indir.** Bilinmiyor (taze tarama yok), kurulabilir,
   kısmi, kurulu ve doğrulanmadı, kurulu ve doğrulandı, engelli; posta
   profillerinin tipli nedenleriyle. Eylemler: kur, devam et, doğrula, onar,
   hiçbiri. Posta paketleri durumlarını posta profillerinden alır; bütün
   bileşenleri var ama kanıtı eksik olan bir posta profili kısmi değil, "kurulu,
   doğrulanmadı"dır. **Onar ve Doğrula, yola göre farklı şeyler anlatır.** Bir
   posta kurulumunun salt okunur doğrulaması yoktur: posta paketinde eylem
   Onar'dır ve profil kurulumunu çalıştırır; eksik olanı tamamlar ve kurulumu
   doğrular, kurulu hiçbir şey kaldırılmaz (pencere bunu, düğmeye basılmadan önce
   söyler). Bir veritabanı ya da web paketinde Doğrula yalnız bileşenleri yeniden
   okur ve hiçbir şeyi değiştirmez; bu yüzden sayfa değişikliklerini tutarken de
   kullanılabilir kalır. Bir veritabanı ya da web paketi asla kısmi değildir:
   bütün bileşenleri taze taramada hazırsa doğrulanmıştır, değilse Kur ile sunulur
   ve Kur yalnız eksik parçaları kurar (var olan parçalar korunan olarak,
   rolü zaten dolu olan bir parça, örneğin web'e Apache hizmet ederken Nginx,
   gerekmeyen olarak sıralanır).
4. **Nasıl kurulur.** Bir posta paketi mevcut posta kurulum penceresini açar ve
   bugünkü yolu aynen çalıştırır (kurulum uzantısı ya da profil kurulumu). Bir
   veritabanı ya da web paketi bir kez sorar, sonra eksik her bileşen için sırayla
   tek bir olağan bileşen kurulumu gönderir; her biri, ancak öncekinin işlemi,
   yani bu adımın kendi bileşen kurulumu taze bir taramayla bittikten sonra
   (sunucu başına tek etkin işlem) ve ancak sayfanın değişiklik kapısı (doğrulanmamış
   bir sayfa durumu, değişikliğe hazır olmayan bir ana makine) yeniden okunduktan
   sonra gönderilir. İlk hata, bu adımın kendisi olmayan bir işlem ya da kapalı
   bir kapı sırayı nedeniyle durdurur ve yalnız eksik olanı kuran bir Devam et
   sunar; başarısız bileşenin kendi yönlendirmesi (D-024) ekranda kalır. Bu Devam et sayfada
   yaşar: bir yoklama asla kurulum göndermez ve yeniden yüklemeden sonra paket
   yalnızca yeniden sunulur. Şema değişikliği yok (şema 44) ve yeni Agent iş türü
   yok.

Sonuçlar. Aynı incelemenin Bileşenler renk düzeltmeleri yeni kurallar olarak
değil, mevcut DESIGN.md kuralları olarak uygulandı: satır eylem simgeleri
dinlenirken nötrdür ve tonlarını yalnız imleç ya da odak altında alır (Durum
Rengi Kuralı, Bitişik Olmama Kuralı); durum noktasının dört tonu vardır, durmuş
bir hizmet sözüyle birlikte kırmızıdır ve "kurulu değil" içi boş bir halkadır
(Durum Rengi Kuralı); sarı Katmanlar karosu ve sarı "kısmi" cümleleri kartlarla
birlikte gitti, şerit Kehribar Mürekkep'i yalnız kurulu ama hazır olmayan bir
webmail çalışma ortamı için kullanır (İki Sarı Kuralı); "Webmail'i onar" sunucu
bir webmail sorunu bildirmedikçe nötr kalır; sağlıklı bir güvenlik duvarı tek
satırdır; bileşen simgeleri emoji yerine tek bir çizgi simgeleri haritasından
gelir; satır sabit sütunlu bir ızgaradır ve telefonda satır eylemleri 44 piksel
olup adları yazılıdır.

Bunun izin vermediği şeyler: kurulu bir sunucuda değişiklik, yeni mağaza
öğesi, lisans politikası değişikliği, WordPress ya da site uygulaması paketi
yok. Kurulu panelleri yine yalnız sahipleri, panelin kendi güncelleme
ekranından günceller.

### Değişiklik 17 Ekim 2026 (alpha.95): her sunucuda aynı liste, her öğede durum

*Sahibin Boston ve Frankfurt'ta (alpha.94) Bileşenler ve Pano'ya baktıktan sonraki 17 Ekim 2026 kararı: "ok. sorunları tekrar değerlendir ve düzelt"; v0.1.0-alpha.95 adayı için kaynakta; kurulu bir sunucuda hiçbir şey ölçülmedi*

Sahibin gördüğü. Boston'da MariaDB paketi kurulabilir olduğu halde hiç
çizilmiyordu ve Pano onu yine de sayıyordu ("5 paket daha kurulabilir"); görünen
phpPgAdmin eklentisini ise saymıyordu. Temel Posta Boston'da bir öğeydi,
Frankfurt'ta yoktu. İki sunucu iki ayrı liste gösteriyordu ve Pano'daki sayı
altındaki listeyle uyuşmuyordu. Nedeni yukarıdaki 2. maddeydi: "kurulu satır ve
en çok bir öneri daha" kardeş paketleri gizliyordu. Bu kural **geri çekildi**.
Sahibin bir gereksinimi değil, sürümü hazırlayanın hatasıydı.

Yerine geçen kural:

1. **Tek liste.** Her Bileşenler kategorisi, her sunucuda aynı hazır paketleri
   aynı kayıt sırasıyla gösterir. Yalnız her çipin durumu değişir. Şerit, kurulu bir
   satır artı bir öneri değil, sola yaslı ve satır kıran bir çip dizisidir.
   Eklentiler her zaman görünür.
2. **Durum sözle ve biçimle söylenir, asla yalnız renkle.** Kurulu ve doğrulanmış:
   bir onay işareti ve ad (erişilebilir ad "…, kurulu ve doğrulandı"); doğrulanmış
   bir posta paketi onu içeriyorsa ve içerilen paketin kendisi de kurulu ve
   doğrulanmışsa, kendi başarısız ya da süren bir denemesi yoksa "Temel Posta ·
   dahil" (aksi halde nedeniyle "doğrulanmadı"); gri bir nokta ve
   "doğrulanmadı"; çerçeveli "Ad · Kur" (eklentilerde "· Ekle"); denemesi olan
   posta için "n/m bileşen kurulu · Devam et"; engelli: kısa bir nedenle soluk
   metin ("phpMyAdmin · MariaDB gerekiyor"), sessiz bir düğmede: basınca (klavye ya
   da dokunmayla) tam neden çiplerin altındaki bir satırda söylenir (ipucu kalır
   ama tek yol değildir); çalışan paket hareket eden bir göstergeyle "2/3 · PHP-FPM
   kuruluyor" ve diğer çipler devre dışıdır. Eski bir tarama tek bir soluk satırdır.
3. **Tek sayma kuralı.** Pano kartı ve çipler aynı biçimde sayar: eklentiler dahil
   kaydın her girdisi; "kurulabilir" kurulabilir ya da kısmi demektir (engelli
   hariç); "içinde" kurulu ve doğrulanmış sayılır. Kart tek bir özet satırdır
   ("7 kurulu ve doğrulandı · 1 kurulabilir"), sonra yalnız sorun satırları
   (nedeniyle başarısız ya da süren bir posta denemesi, kurulu ama doğrulanmamış
   bir paket; ikisi birden olan paket, nedenle tek satırdır), sonra "Bileşenler'de
   aç". Kendi son denemesi başarısız olan ya da süren bir paket asla kurulu ve
   doğrulanmış sayılmaz. Hiç denetlenmedi, denetleniyor, tutuluyor, denetlenemedi ve
   yakın zamanda değil cümleleri korunur.
4. **Kurulu çip Onar ve Doğrula'yı taşır.** Kurulu bir çipe basmak bir pencere
   açar: posta paketi için Onar (profil kurulumu; eksik olanı tamamlar ve kurulumu
   doğrular, kurulu hiçbir şey kaldırılmaz), bileşen paketi için Doğrula
   (bileşenleri yeniden okur, hiçbir şeyi değiştirmez). Şeritte Onar bağlantısı ve
   ipucu cümlesi yoktur. Roundcube'un "Webmail'i onar" düğmesi, bileşen satırında
   yalnız sunucu bir webmail sorunu bildirdiğinde durur; aksi halde onarım Webmail
   çipinin penceresindedir.
5. **alpha.94'ün üç inceleme maddesi kapandı.** Bir paketin adımları arasındaki
   kapı, doğrulanmamış durum bayrağını değiştiği anda ayarlanan bir referans
   üzerinden, hazırlık okumasından önce ve sonra okur. Bir bileşen paketinin
   Doğrula'sı, bir bileşen işlemi sürerken (kilitli izleyici, etkin bir paket sırası
   ya da ana makinenin `panel_operation_active` / `agent_mutation_active`
   bildirmesi) nedeni gösterilerek tutulur; değişiklikler başka nedenlerle
   tutulduğunda kullanılabilir kalır, çünkü yalnız okur (`host_lock_busy` ve
   `package_manager_active` dahil: yeniden tarama ucu yalnız çalışan bir sunucu
   kurulumunda, etkin bir hizmet işleminde ya da etkin bir Agent değişikliğinde
   reddeder). Çip, kurulum isteği kabul
   edilene kadar "X kurulmadan önce sunucu denetleniyor…", kabulden sonra yalnızca
   "X kuruluyor" der.

Şema değişikliği yok (44), Agent değişikliği yok, yeni Agent iş türü yok. D-034'ün
geri kalanı (katalog bileşenleri üzerinde kısayollar, birer birer sıra, Devam et
yalnız sahibin eylemiyle, yeniden yüklemeden sonra devam yok) geçerli kalır.

**Sürümler: bu sürümde yeni Agent RPC'si yok.** Sahibin ekranları çizgilerden
oluşan bir sürüm sütunu gösteriyordu. İnceleme: Panel, yönetilen bir hizmetin
`Versions` alanını yalnız çalışma ortamı örneklerinden doldurur
(`Agent.ListServiceInstances`, yani yalnız PHP-FPM ve Node.js). `Agent.GetServices`
her systemd birimi için bir ikili yoklama `Version` döndürür
(`internal/services/version_detector.go`); Panel bunu yok sayar. Bu bir paket
sürümü değildir, birkaç birim için güvenilmezdir (Apache, `named` ve PostgreSQL
birim adını ikili olarak çalıştırır) ve birimi olmayan araçları (Certbot) kapsamaz.
Gerçek paket sürümleri yeni bir Agent RPC'si ya da genişletilmiş `dpkg`/`pacman`
sorguları gerektirir; bu sürüm bunları eklemez. Karar: masaüstü sürüm sütunu
kaldırıldı; sürüm yalnız bilindiği yerde açıklamanın altında küçük tek aralıklı
metin olarak gösterilir; tarayıcı sahtesi artık sürüm uydurmaz. **Açık yol haritası
işi:** her bileşen için kurulu paket sürümünü okumak.

**Durdur simgesi.** Durdur düğmesi boş bir kareydi ve bir onay kutusu gibi
okunuyordu. Bileşenler satırlarında, bileşen sayfasında ve barındırma türü
panelinde Lucide `StopCircle` (daire içinde bir dur işareti) kullanılır; yeniden
başlat, durdur, başlat, onar ve sil tek bir çerçeveli simge düğmesini paylaşır.
Görünümler DESIGN.md'de, "Ready bundle chips, row actions and versions (alpha.95)"
bölümündedir.

Aynı incelemenin sahibin istediği temizlikleri, yeni kural değil: kenar çubuğu
sayıları (Bileşenler, Alan adları) dolu hap değil, sade soluk sayılardır (kimsenin
almadığı bir sayı yine çizgidir, asla 0 değil); Pano'nun "dikkat gerektiren bir şey
yok" cümlesi sade dildir ve hâlâ her okumanın yanıtlanmasını ve taze bir bileşen
taramasını ister; Pano'nun Alan adları sayısı, Alan adları sayfasının listelediği
bağımsız altyapı DNS bölgelerini ekler (sahibin 16 Ekim 2026 tarihli alpha.91
kararı); Pano kutusu, kenar çubuğundaki Alan adları rozeti ve Alan adları sayfası
artık uyuşur, çünkü kutu ve rozet tek bir yardımcıyı kullanır (`domainsCount`,
`web/src/lib/infrastructureZones.ts`): alan adları artı Alan adları sayfasının
listelediği yalnız DNS altyapı bölgeleri (siteye dönüşen bir bölge, o site olarak
bir kez sayılır); sayı yalnız iki okuma da yanıtlandığında; biri okunurken yükleme
işareti; biri okunamadığında bilinmeyen, asla "alan adları + 0" (önceki sayı yalnız
iki okumanın da sayısı varsa korunur); bölge okuması olmayan yönetici olmayanlar
yalnız alan adlarını sayar.

**Adayın bağımsız incelemesi (17 Ekim 2026).** Bağımsız bir inceleme on sorun
buldu; aday içinde düzeltildi ve bu değişiklik buna göre
düzeltildi. (1) Roundcube kalıntıları: Bileşenler satırı soluk notu ve nötr
"Kalıntıları temizle…" düğmesini yalnız Panel'in kendi kaydı yarım kalmış bir
Roundcube kurulumu ya da kaldırması gösterdiğinde (Panel denetim kaydındaki en yeni
Roundcube kurulum ya da kaldırma girdisi başarısız bir kurulum ya da kısmi ya da
başarısız bir kaldırma; ek yük alanı `webmail_cleanup` ile gönderilir) ya da webmail
profilinin son denemesi başarısız olduğunda gösterir; sonraki temiz bir kurulum ya
da kaldırma sunumu bitirir; kayıt yoksa satır bir şey söylemez. Roundcube'un kendi
bileşen sayfasında aynı onayı açan (`/services?cleanup=roundcube`) bir
"Kalıntıları temizle…" bağlantısı her zaman vardır. Onay değişmedi. Kalan bilinen
açık: Panel dosyaların diskte kalıp kalmadığını hâlâ göremez (Agent okuması yok);
kendi kaydına dayanır. (2) Kurulu paket penceresi "… kurar" yerine paketi betimler
("Sitelerin veritabanları için PostgreSQL."). (3) Çiplerin altında: yalnız
başarısız ya da süren denemeler ve doğrulanmamış ya da kısmen kurulu çiplerdeki
tipli uyarılar; "bazı parçalar zaten var" notu asla. (4) "içinde" yalnız, kendisi de
kurulu ve doğrulanmış olan ve kendi başarısız ya da süren denemesi bulunmayan içerilen
paket için; aksi halde nedeniyle "doğrulanmadı" ve Pano onu saymaz. (5) Kutu ve rozet
için tek Alan adları sayısı (yukarıda). (6) Doğrulanmamış ve başarısız ya da süren
denemesi olan paket tek Pano satırıdır. (7) Engelli çip, tam nedeni çiplerin altında
gösteren sessiz bir düğmedir. (8) Doğrula, `host_lock_busy` ve
`package_manager_active` durumunda kullanılabilir kalır (yukarıda). (9) İfade: EN
"2/3 · installing PHP-FPM"; "n of m installed" (Türkçe "2/3 bileşen kurulu"). (10)
"Kur" diyen bir posta çipi (kurulu bir Webmail'in yanında Spam Korumalı Posta; Panel
onu yalnız parçalar paylaşıldığı için kısmi sayar) posta penceresini "devam et"
değil, kurulum biçiminde açar. Şema değişikliği yok (44), Agent değişikliği yok,
gerçek sistem kaydı yok: yeni yük alanı, var olan denetim kaydından okunan eklemeli
bir JSON alanıdır.

Bunun izin vermediği şeyler: kurulu bir sunucuda değişiklik ve lisans politikası
değişikliği yok. Kurulu panelleri yine yalnız sahipleri, panelin kendi güncelleme
ekranından günceller.

### Değişiklik 22 Ekim 2026 (alpha.101): paket, bir hizmet ile ona eşlik eden araçlardır

*Sahibin v0.1.0-alpha.100'de Frankfurt'a baktıktan sonraki 22 Ekim 2026 kararı; v0.1.0-alpha.101 adayı için kaynakta (kod 22 Ekim 2026'da, bu kayıt 23 Ekim 2026'da; saat tarihleri); kurulu bir sunucuda hiçbir şey ölçülmedi*

Sahibin gördüğü. Frankfurt'ta, alpha.100'de, veritabanı kategorisi her biri tek
bileşen olan bir "PostgreSQL" paketi ve bir "MariaDB" paketi sunuyordu; bu
bileşenlerin şeridin hemen altında kendi satırları zaten vardı. Tarayıcı araçları
phpPgAdmin ve phpMyAdmin ise ayrı eklentilerdi. Sahibin sözleri: "bundle diyip
tek servisi kurmak nedir. saçmalık. postgresql ve phpPgAdmin bundle dır mariadb ve
phpMyAdmin bundle dır. Diğer tüm bundle lar da böyle dir." Yukarıdaki 1. madde
"her biri isteğe bağlı bir eklentiyle bir PostgreSQL paketi ve bir MariaDB paketi"
sayıyordu; araç artık paketin parçasıdır.

Benimsenen kurallar:

1. **Paket, bir hizmet ile ona eşlik eden araçlardır.** Tek bileşenli bir paket
   yalnız bileşeninin satırını tekrarlar; bu yüzden artık yoktur. Her bileşen
   paketinde en az iki bileşen bulunur ve bunlardan en az biri yalnız başka birinin
   önkoşulu olarak değil, kendisi için oradadır; bir posta paketi tam olarak kendi
   posta profilidir. Go'daki bir kayıt sözleşmesi testi bunu bozan bir kaydı ve
   emekli edilen kimlikleri ve ad kodlarını reddeder.
2. **Kayıt, sırasıyla.** Adlar İngilizce ve Türkçede aynıdır; Türkçesi farklı
   olanlar ayrıca yazılmıştır.

   | Kimlik | Kategori | Ad | Kurulum sırasıyla bileşenler | Not |
   | --- | --- | --- | --- | --- |
   | `mail.core` | posta | Temel Posta (Core Mail) | Postfix, Dovecot | posta profili `core-mail`; ardından Spam Korumalı Posta'yı sunar |
   | `mail.webmail` | posta | Webmail | Postfix, Dovecot, Nginx, PHP-FPM, Roundcube | posta profili `webmail`; ardından Spam Korumalı Posta'yı sunar |
   | `mail.protected` | posta | Spam Korumalı Posta (Spam-Protected Mail) | Postfix, Dovecot, Rspamd | posta profili `protected-mail` |
   | `db.postgresql` | veritabanı | PostgreSQL + phpPgAdmin | PostgreSQL, Nginx, PHP-FPM, phpPgAdmin | Nginx ve PHP-FPM önkoşuldur (4. madde) |
   | `db.mariadb` | veritabanı | MariaDB + phpMyAdmin | MariaDB, Nginx, PHP-FPM, phpMyAdmin | Nginx ve PHP-FPM önkoşuldur (4. madde) |
   | `web.nginx_php` | web | Nginx + PHP-FPM | Nginx, PHP-FPM | |

   `db.postgresql.admin` ve `db.mariadb.admin` eklenti paketleri kaldırıldı;
   kurdukları şeyler artık veritabanı paketlerinin içindedir. Posta paketleri,
   onların Spam Korumalı Posta eklentisi ve Nginx + PHP-FPM değişmedi. Kaydın her
   girdisi önceki gibi çizilir ve sayılır (alpha.95 değişikliği, 1. ve 3. madde);
   artık altı girdi vardır.
3. **Kimlikler korunur, adlar değişir.** `db.postgresql` ve `db.mariadb`
   kimliklerini korur; bu yüzden onlara giden bir bağlantı yine aynı pakete varır.
   Ad kodları değişti (`db_postgresql_phppgadmin`, `db_mariadb_phpmyadmin`); böylece
   bir sayfa, daha eski bir Panel'in tek bileşenli paketini asla araçlı bir paket
   diye adlandırmaz: sayfada emekli kodların sözü yoktur ve böyle bir yanıtı
   bileşenleriyle adlandırır.
4. **Önkoşullar bir rolü doldurur.** Bir veritabanı aracının üzerinde çalıştığı
   web sunucusu ve PHP-FPM pakette kendileri için değil, doldurdukları rol için
   bulunur. O rolü kurulu başka bir bileşen dolduruyorsa (Nginx yerine Apache), o
   kullanılır ve pencere Nginx'i çakışma olarak değil, "gerekmiyor" olarak sıralar.
   Nginx + PHP-FPM ise Nginx'in kendisini adlandırır ve önceki gibi Apache'nin
   yanında engelli kalır.
5. **Sistemin sunmadığı bir parça paketin tamamını engeller.** Paket var olan tipli
   nedenle engellenir ve hiçbir zaman bir alt kümesi kurulmaz. phpPgAdmin'in pacman
   paketi yoktur; bu yüzden Arch'ta "PostgreSQL + phpPgAdmin" engellidir ve çipi
   "phpPgAdmin bu sistemde kullanılamıyor" der ("phpPgAdmin is not available on this
   system"); MariaDB + phpMyAdmin orada kurulabilir kalır.
6. **Kurulu paketin penceresi her parçanın kendi durumunu söyler; Doğrula geri
   çekilir.** Bu, yukarıdaki 3. ve 4. maddeyi ve alpha.95 değişikliğinin 4.
   maddesini ("Kurulu çip Onar ve Doğrula'yı taşır") değiştirir. Her parça, "Zaten
   kurulu" yerine satırının kendi durumunu ve sürümünü söyler ("Çalışıyor · 1.26.3",
   "Kurulu · 5.2.1", "Durdu"; İngilizcesi "Running · …", "Installed · …",
   "Stopped"); bunlar satırların kullandığı aynı veriden gelir ve ipucunda tam paket
   sürümü vardır. Doğrula düğmesi ve cümlesi kaldırıldı: yalnız bileşenleri yeniden
   okuyordu; bunu sayfanın Yeniden tara düğmesi yapar. Kapat kalır. Posta paketleri,
   gerçek iş yapan Onar'ı (profil kurulumu) korur. Doğrulanmamış kurulu bir bileşen
   paketi, çalışmayan parçayı satırın kırmızısıyla gösterir ve onu kendi satırından
   başlatmayı ya da sayfanın üstündeki Yeniden tara'yı kullanmayı, pencerenin
   sunucuda hiçbir şeyi değiştirmediğini sözle söyler; kendi düğmesi yoktur. Panel
   artık `verify` eylemini göndermez (`none` gönderir); sayfa daha eski bir
   Panel'den gelen `verify`'ı yine kabul eder ve onun için düğme çizmez. Sayfa
   yanıtı okurken, yanıtın taşımadığı bir kimliği artık bütün listeyi reddetmek
   yerine dışarıda bırakır (bilinmeyen bir pakete eklenti göndermesi atılır, var
   olmayan bir paketin eklentisi çizilmez).
7. **Bir paketin nasıl kurulduğu değişmedi** (yukarıdaki 4. madde): eksik her
   parça için sırayla, her biri öncekinin taze bir taramayla bitmesinden sonra tek
   bir olağan bileşen kurulumu; ilk hata sırayı nedeniyle ve yalnız eksik olanı
   kuran bir Devam et ile durdurur; kurulu hiçbir şey kaldırılmaz.

Sahibin sunucularının gösterdiği, Panel'in türettiği biçimde (alpha.100
ekranlarından; alpha.101'de ölçülmedi). **Boston** (PostgreSQL ve Nginx kurulu,
PHP-FPM yok, phpPgAdmin yok): "PostgreSQL + phpPgAdmin · Kur"; penceresi PostgreSQL
ve Nginx'i korur, PHP-FPM'i, ardından phpPgAdmin'i birer birer kurar. Pano'daki
"Hazır paketler" kartı aynı çipleri sayar: "Henüz kurulu yok · 6 kurulabilir".
**Frankfurt** (her şey kurulu): iki veritabanı paketi de kurulu ve doğrulanmıştır ve
kart "6 kurulu ve doğrulandı" der.

**Paketlerin tanımlandığı yer.** Kodda, `cmd/panel/bundles.go` içindeki
`bundleDefinitions`'ta; Panel'e derlenir. Bir dosyadan ya da veritabanından
okunmaz ve panel çalışırken değiştirilemez; bir paketi değiştirmek bir kod
değişikliğidir ve bir sürüm gerektirir. Sahip isterse ileride bir veri kataloğuna
taşınabilir; bu burada karara bağlanmadı.

Şema değişikliği yok (44), Agent değişikliği yok, yeni Agent iş türü ya da RPC
yok, güncelleyicide ve sürüm kurtarmada değişiklik yok; bu yüzden gerçek sistem
kaydı yok.

Bunun izin vermediği şeyler: kurulu bir sunucuda değişiklik, lisans politikası
değişikliği, yeni mağaza öğesi, WordPress ya da site uygulaması paketi ve
paketlerin çalışırken düzenlenmesi yok. Kurulu panelleri yine yalnız sahipleri,
panelin kendi güncelleme ekranından günceller.

Diğerleriyle ilişkisi. D-015 2. madde geçerlidir: katalog hâlâ tek kurulum
adresidir ve bir paket yalnız katalog bileşenlerini kurar. D-024: doğrulanmamış bir
paket hangi parçanın, kimin işlem yapacağını (yönetici), sonraki eylemi (parçayı
satırından başlatmak ya da Yeniden tara) ve işin nasıl süreceğini söyler. D-033:
kurulum sırası ve görünür beklemesi değişmedi. D-036: penceredeki sürümler
satırların sürümleridir. D-037 değişikliğinin kaldırma penceresi birleşen paketleri
adlandırır (Nginx'i kaldırmak artık iki veritabanı paketini de eksik bırakır).

Yapılmayan: kurulu bir sunucuda hiçbir şey ölçülmedi; paketlerin bir veri
kataloğuna taşınıp taşınmayacağı sahibe bırakıldı.

### Değişiklik (24 Ekim 2026, saat tarihi): paketler sürümleri gösterir ve seçtirir; araçlar ilk kurulumda kanıtlanır; beklenmeyen kurulumlar önce gösterilir

*Sahibin 24 Ekim 2026 kararları ("ok"); v0.1.0-alpha.105 adayı için kaynakta; kurulu bir sunucuda hiçbir şey ölçülmedi*

Sahibin sorusu. Bir paket kuracağı sürümleri nasıl seçer, uyumlu olanları zorlamalı
mı? Sahip, CelikPanel'in sahibin istediğini kurduğu ilkesini hatırlattı: **öner,
uyar, asla dayatma**.

Benimsenen kurallar (yapıldığı gibi):

1. **Paket penceresi her sürümü gösterir ve sahibin değiştirmesine izin verir.**
   Pencere kuracağı her bileşen için gelecek sürümü söyler ("8.4 · dağıtımın sürümü
   · önerilen"); **Değiştir**, o bileşenin kendi kurulumunun zaten sunduğu sürümleri
   listeler (yönetilen deposu: PHP-FPM için üreticinin PHP deposu, PostgreSQL için
   PGDG; yeni depo yok). Zaten kurulu bir parça sürümünü söyler ("PostgreSQL 16.15 ·
   zaten kurulu") ve değiştirilmez. Önerilen sürüm dağıtımın kendi sürümüdür;
   işaretlenir, asla zorlanmaz. PHP-FPM'e bir sürüm seçildiğinde paketin veritabanı
   aracı o PHP için kurulur ve ona bağlanır ("PHP 8.3 üzerinde çalışır"). Node.js
   hiçbir pakette yok.
2. **Bilinen uyumsuzlukta uyar; yalnız çalışamayacak olanı reddet.** Bir aracın
   eklenti paketinin (ya da FPM paketinin) kurulabilir adayı olmayan bir PHP sürümü
   seçilirse, hiçbir şey değişmeden reddedilir; paketler ve sonraki eylem söylenir:
   "phpPgAdmin burada PHP 8.6 üzerinde çalışamaz: php8.6-pgsql bu sunucunun
   depolarında yok. PHP-FPM için başka bir PHP sürümü seçin." Dağıtımdan gelen bir
   aracı dağıtımınkinden başka bir PHP'ye koymak ve güvenlik desteği bitmiş bir PHP
   (8.2'nin altı) yalnız uyarıdır ("… PHP 8.5 genellikle çalışır, ama dağıtım
   bunu sınamaz. Devam edebilirsiniz."); sahip devam edebilir.
3. **Araçlar ilk kurulumda kanıtlanır.** phpMyAdmin, phpPgAdmin ya da Roundcube
   kurulup yapılandırıldıktan sonra alpha.104'teki taşımayla aynı kanıttan geçer
   (aracın kendi soketi üzerinden CelikPanel'in PHP denetimi, sonra aracın kendi
   sayfası, sınırlı yeniden deneme). İlk kurulum kanıtı geçemezse, aracın eklenti
   paketleri zaten kurulu olan diğer PHP sürümleri sırayla denenir (önce dağıtımın
   varsayılanı, sonra en yenisi); her biri, başarısız olunca önceki yapılandırmayı
   geri koyan doğrulanmış taşımayla. Hiçbiri çalışmazsa araç kurulu kalır ve nedeni
   (örneğin PHP'de hangi eklentinin eksik olduğu) ve sonraki eylemle (Yönet'te bir
   PHP sürümü seçin) "Hazır değil" okunur. Zaten kurulu bir araç (onarım) kanıtlanır
   ama asla taşınmaz; alpha.104 kuralı geçerlidir. Sonradan geçen bir kanıt
   (Yönet'te bir taşıma) "Hazır değil"i kaldırır.
4. **Beklenmeyen kurulumlar hiçbir şey değişmeden önce gösterilir.** Paket
   yöneticisinin kendi kurulum benzetimi (APT `-s`; pacman `--print`; bir pakette
   bütün adımları birlikte) istenen paketlerin ötesinde neyi kuracağını söyler.
   Beklenen: istenen paketler, kurulumun adını verdiği ya da mevcut bir PHP
   sürümünün paketleri, sıradan kütüphaneler ve sunucuda zaten olan bir bileşenin
   her şeyi. Dikkat çeken: yeni bir PHP sürümünün paketleri, Apache (apache2,
   apache2-bin, libapache2-mod-php*), başka bir katalog bileşeninin paketi, bir
   veritabanı sunucusu. Dikkat çeken paketler paket penceresinde ve bir bileşenin
   kendi kurulum penceresinde adlarıyla ve düz bir gerekçeyle listelenir ("Bu,
   seçmediğiniz paketleri de kurar:") ve sahip "Okudum; bu paketleri de kur."
   kutusunu işaretleyene kadar Kur'u tutar. Okuma sürerken gösterilir. Başarısız
   olan ya da paket yöneticisini meşgul bulan bir okuma "denetlenemedi" der ve Kur'u
   tutmaz, çünkü her kurulum çalışmadan hemen önce sunucuda, paket kilidi altında
   yeniden denetlenir ve sahibin onaylamadığı dikkat çeken bir paketi getirecekse
   reddedilir (hiçbir şey kurulmaz).
5. **Sürekli tümleştirme.** "Panel race — Server setup" işi test adına göre "(1/2)"
   ve "(2/2)" olarak bölündü (54 ve 70 test); zaman sınırı yükseltilmedi.

Uygulama notları (sahip kararı değildir). Yeni salt-okur
`Agent.ServiceInstallPreviewV1` (adım yok; paket kilidini asla beklemez) ve
`Agent.ProvePHPToolV1` (aracın kendi kurulum işinin içinde);
`GET /api/v1/service/install-preview`; kurulum isteğinde `php_version`,
`check_installs` ve `confirmed_installs` alanları. İnceleme düzeltmesi (aynı gün):
Panel, tarayıcı ne gönderirse göndersin her kurulumda denetim ister (bir bileşenin
kendi sayfası, Sunucu kurulumu, posta profilleri, eklenen bir sürüm, Onar, eski bir
sekme); tarayıcı yalnız onaylananları verir. PHP sürümü ve onaylanan adlar kurulumun
işlem kaydında tutulur ve kesilen kurulum yeniden sürerken geri konur. Başarısız bir
kanıt Yönet penceresinde de hazır değil sayılır; aynı sürüm yeniden seçilebilir. Gerçek sistem denemesi, PHP 8.3 seçilmiş bir paketin başka
PHP getirmediğini (aracın 8.3 eklentileri bağımlılıklarını karşılar), yalın PHP
8.3 üzerinde tek başına phpMyAdmin'in ise varsayılan PHP 8.4'ün paketlerini
çekeceğini gösterdi; bu söylendi ve onaysızken reddedildi.

Bunun izin vermediği şeyler: kurulu bir sunucuda değişiklik yok. Kurulu panelleri
yine yalnız sahipleri, panelin kendi güncelleme ekranından günceller. Paketleri
listeleyen bir pencerenin olmadığı yerlerde (Sunucu kurulumu, posta profilleri,
PHP-FPM sürüm listesinden eklenen bir PHP sürümü, Onar) dikkat çeken bir kurulum
hiçbir şey kurulmadan reddedilir; yönlendirmesi paketleri adlandırır ve
Bileşenler'i gösterir.

Diğerleriyle ilişkisi. D-022: yerel yapılandırma gerçeğin kendisi kalır ve seçim
sahibindir. D-024: her ret nedeni, kimin işlem yapacağını ve sonraki eylemi söyler;
doğrulanmış bir ret ile bilinmeyen bir okuma ayrı tutulur. D-025: Agent'ın kurulum
yolu değişti, bu yüzden kanıtı kaydedildi (RESILIENCE-CONTRACT, alpha.105). D-029:
kurulum istekleri kimliklerini korur. D-033: okuma beklerken gösterilir. D-036:
sunulan sürümler paket veritabanının yanıtıdır.

Yapılmayan: kurulu bir sunucuda hiçbir şey ölçülmedi; pacman önizlemesi yalnız
bileşen testleriyle kapsanır ve gerçek `-Syu` işlemi değildir; Apache kuralı yalnız
Debian ve Ubuntu paket adlarını bilir; DNF ulaşılamaz kalır. Dal koruması eski "Panel race —
Server setup" denetim adını istiyorsa, sahibin onu iki yeni adla değiştirmesi
gerekir.


---

## D-033 · Sahibi bekleten bir ekran beklediğini gösterir ya da yapılacak bir şey olmadığını söyler

*14 Ekim 2026 (saat tarihi) · Kurulu ikinci sunucudaki (Frankfurt, v0.1.0-alpha.84) kurulumun son adımından sonra sahip kararı; web arayüzünün okunmasıyla bulunan her bekleme durumuna (37 durum) kaynakta, bağlanan testler ve API taklidine karşı tarayıcıyla uygulandı (14 Ekim 2026); kısmen on sekizinci gerçek sistem kaydında ölçüldü (set16, 14 Ekim 2026 UTC: Dashboard'ın Posta paketleri kartı ve kenar çubuğundaki yapı kimliği Debian 13 ve Ubuntu 24.04'te; Agent durmuşken Bileşenler güvenlik duvarı şeridi ve "İlgi istiyor" kutusu yalnız Debian 13'te); kurulumun tamamlanmasına ve güncelleme kartının meşgul durumuna gerçek sistemde ulaşılamadı; kurulu bir sunucuda hiçbir şey ölçülmedi*

Sahip 13 ve 14 Ekim 2026'da kurulu ikinci sunucuda kurulumun son adımı "Verify
the prepared server" ile uğraştı. Son kontrol, sunucu adresinin ters DNS (PTR)
adını bekliyordu. Sahip bu adı VPS sağlayıcısında çoktan değiştirmişti;
sağlayıcı değişikliği hâlâ yayıyordu ve sorulan genel çözümleyici önbelleğindeki
eski adla yanıt veriyordu. Sahibin payına düşen iş bitmiş olduğu hâlde adım
"Waiting for you" diyordu; sağlayıcıdaki bir değişikliğin genel DNS'e ulaşmasının
zaman aldığını ya da CelikPanel'de başka bir şey gerekmediğini söyleyen bir şey
yoktu. "Check requirements again" bütün kontrolleri geçirdikten sonra adım,
hareketsiz bir cümlenin ("These checks passed. Waiting for the server to confirm
setup completion.") altında "Waiting for requirements" dedi; düğmeler etkindi,
hareket yoktu; çalışma ancak bir sonraki 20 saniyelik geçişinde tamamlandı.
Sahibin sözleri, özünde: bu insanların kafasını karıştırır; ne yapacaklarını
bilmezler ve sabırsızlıkla bir şeylere basarlar; beklemeleri gerekiyorsa
bekletilmeli ya da açıkça beklemeleri söylenmeli; işlem yapıyor görünmüyorken
kimse iki dakika beklemez. Sahip ardından kuralı her ekran için koydu.

Benimsenen kurallar:

1. Kullanıcıyı bekleten bir ekran ya görünür biçimde bekler (hareketle; araya
   girebilecek denetimler devre dışıdır) ya da yapılacak bir şey olmadığını ve
   sayfanın kendiliğinden güncelleneceğini açıkça söyler. Yapabildiği yerde
   ikisini birden yapar.
2. İşi çabuk bitirir. Sahibin kendi eyleminin bitirebileceği bir bekleme (geçen
   bir kontrol) bir sonraki zamanlayıcıda değil, o eylemle biter.
3. Her beklemenin bir sınırı vardır. Sınırdan sonra ekran hareketini durdurur ve
   dürüstçe sonucun henüz bilinmediğini, bunun bir hata olmadığını, kimin işlem
   yapacağını ve neyin denetleneceğini, işin nasıl süreceğini söyler; bilinen bir
   hata kendi sözlerini korur.
4. Etkin düğmelerin yanında duran hareketsiz bir bekleme cümlesi kusurdur.
5. Sahibin payına düşen iş bitmişken ekran hiçbir zaman "Sizi bekliyor" demez;
   örneğin sağlayıcıda yapılan bir değişiklik yayılırken. Neyi beklediğini (ör.
   "Ters DNS bekleniyor") ve bunun ne kadar sürebileceğini söyler.
6. Okuyabileceği bir durumu bekleyen ekran, sahibe bir düğmeye bastırmak yerine
   görünür olduğu sürece onu kendiliğinden yeniden okur (yalnız okuma).

Sonuçları. Web arayüzü, bir sayfanın sunucuyu, bir arka plan işlemini, bir
yoklamayı, bir sağlayıcıyı, lisansı ya da dış bir denetimi beklediği her durum
için kaynaktan okundu: 37 durum. 18'i kurala zaten uyuyordu, 16'sı 14 Ekim
2026'da değiştirildi; 3'ü (bağlantı kesildikten sonra bileşen işlem kilidi,
panel güncelleme kartının hazır olma durumu ve bir alan adının yayılan DNS
değişikliği) için sahip aynı gün karar verdi ve ardından onlar da değiştirildi.
Seçilen sınırlar: kurulumun tamamlanması, kurulumun yeniden bağlanması ve
bileşen kilidi için 60 saniye; güncelleme kartı sunucunun meşgul olduğunu
söylerken hazır olma durumunun 5 saniyede bir yeniden okunması; yayılan bir alan
adının 30 saniyede bir, en çok 60 kez kontrol edilmesi. Kurulumun son adımı
artık sahibin geçen kontrolüyle tamamlanır. Satırlar, commit'ler ve testler
[OPERATION-GUIDANCE](OPERATION-GUIDANCE.tr.md) içindeki 2026-10-14 tarihli iki
kayıttadır; hangi denetimin hangi koda dayandığını v0.1.0-alpha.85 sürüm notları
söyler.

Bunun yetkilendirmediği:

- Bir değişiklik başlatan ya da kabul eden yoklama. Bu kararın eklediği her
  kendiliğinden yeniden okuma bir okumadır (GET); bir yeniden okuma asla ikinci
  bir değişiklik başlatmaz. Güncelleme kartının ve yayılan alan adının yeniden
  okumaları aynı anda tektir ve yalnız sayfa görünürken çalışır; bileşen kurulum
  penceresinin 5 saniyelik yeniden okuması ve kurulum sayfasının okumaları
  sekmenin görünürlüğüne bakmaz (açık; sürüm notlarına bakın).
- Kimse bakmazken çalışan yeni bir zamanlayıcı ya da hareket olarak gösterilen
  sınırsız bir bekleme: sınır hareketi bitirir ve bildirim sonucun bilinmediğini
  söyler (kurulumun ve bileşen kilidinin okumaları ondan sonra da sürer). Kurulum
  penceresinin yeniden okuması, sekme ne olursa olsun pencere açıkken çalışır;
  bu bir açıktır, bir yetki değil.
- Doğrulanmış bir hata ya da karşılanmamış bir önkoşul yerine "henüz
  bilinmiyor": üçü ayrı kalır (D-024).
- Her ekranın ölçüldüğü iddiası. Denetim koddan okundu; gerçek sistem kaydı bir
  kısmına ulaştı; kurulu bir sunucuda hiçbir şey ölçülmedi.
- Kurulu panellerde değişiklik. Panel güncellemeleri sahibin kalır, panelin
  kendi güncelleme ekranından; hiçbir P0 işi kapanmaz ya da ilerlemez.

Gerisiyle ilişkisi. D-024 her işlemin mevcut nedenini, kimin işlem yapacağını,
sonraki eylemi ve işin nasıl süreceğini göstermesini ve doğrulanmış hata,
karşılanmamış önkoşul ve bilinmeyen sonucu ayırmasını ister; bu karar onu
beklemeye uygular ve bir beklemenin bekleme gibi görünmesi ve bitmesi gerektiğini
ekler. D-032 (panel kendiliğinden bakar; okuyabileceği bir durumu öğrenmek için
sahibe bir düğmeye bastırmaz), 6. kuralın sormak yerine yeniden okumasının
nedenidir. Bilinen-durum kuralı, sınırın olumsuz bir durumla değil "henüz
bilinmiyor" ile bitmesinin nedenidir. D-022 (sunucuyu sahibi işletir), sınırlı
bildirimlerin bir şeyi yeniden başlatmak yerine sahibe paneli sunucuda nasıl
denetleyeceğini söylemesinin nedenidir.

Durum. v0.1.0-alpha.85 adayının dalında (15 Ekim 2026'daki yayımından önce
yazıldı) kaynakta, bağlanan testlerle. Bileşenler güvenlik duvarı şeridi, Dashboard'ın ilgi bildirimi ve
Posta paketleri kartı ile kenar çubuğundaki yapı kimliği on sekizinci gerçek
sistem kaydında ölçüldü. Kurulumun tamamlanmasına, güncelleme kartının meşgul
durumuna, bileşen kilidinin yeniden bağlanmasına ve yayılan alan adı
kontrollerine gerçek sistemde ulaşılamadı. Bir gerçek sistem çalıştırması, kurulu
bir sunucu ve kurulum penceresinin ile kurulum sayfasının okumalarının
görünürlüğe bağlanması için açık.

*15 Ekim 2026 tarihli not (saat tarihi): v0.1.0-alpha.86'da uygulandı (aday
iken yazıldı; 15 Ekim 2026'da yayımlandı).* O aday için yeni bir karar kaydedilmez; yaptıkları bu kararı ve
D-032'yi izler ve yukarıdaki metin değişmedi.

- 1. ve 3. kurallar, sahibin 15 Ekim 2026'da ikinci kurulu sunucunun alpha.85'e
  gerçek güncellemesi sırasındaki gözleminden sonra: güncellemenin kendi
  planladığı yeniden başlama ilerleme olarak gösterilir ("Panel, güncellemenin
  bir parçası olarak yeniden başlıyor…", dönen simge, T+ sayacı); uyarı ve
  "bilinmiyor" metni yalnız bir sınırdan sonra (güncellemenin başlangıcından 6
  dakika ve ilk kopan okumadan en az 2 dakika) ya da kendi metnini koruyan
  doğrulanmış bir hatayla gelir. Güncelleme kartı ve köşe bildirimi artık aynı
  şeyi iki kez söylemez. Sınır iki laboratuvar güncellemesine (set16) ve o tek
  sahip gözlemine dayanır.
- 1. ve 3. kurallar, set17 ve set17b gerçek sistem kayıtlarından sonra: aynı
  sınır içinde ne lisans kapısı ne oturum kapısı o ilerlemenin yerine kendi
  tutmasını koyar (set17 D1) ve güncellemenin gezinti kilidi, yeniden başlama
  sinyali etkinken yeniden başlamayı en geç başlangıçtan 6 dakikaya dek
  kapsar (set17b F1; sürümü hazırlayanın 15 Ekim 2026 kararı, sahibin 13 Ekim
  2026 tarihli kalıcı talimatı altında, bu kuralın bir uygulaması; sahibin
  kararı değil ve yeni bir karar değil).
- "Bunun yetkilendirmediği" altında adı geçen açık kaynakta kapandı: bileşen kurma
  penceresinin yeniden okuması ve kurulum sayfasının okumaları artık yalnız
  sekme görünürken çalışır (`web/src/lib/visibleReads.ts`) ve bu kararın sekme
  gizliyken zaten duran iki yeniden okuması, güncelleme kartının hazırlık
  yeniden okuması ve yayılan alan adının yeniden kontrolü, sekme yeniden
  gösterilince hemen bir kez okur (6. kural); bu kararın diğer yeniden okumaları
  bunun için denetlenmedi.
- D-032 ile: hiçbir sunucu ya da sayfa cümlesi sahipten yeniden tarama istemez;
  doğrulanamayan kayıtlı bir bileşen kaydını Dashboard, Bileşenler sayfası ve
  bir bileşenin kendi sayfası kendileri kontrol eder; bir değişiklik yüzünden
  tutulan kontrol sekme görünürken kendiliğinden sürer (yalnız okuma, tek
  izleme, döngü yok).
- Kendi başına bir karar değil: `system` kullanıcı adı ayrıldı
  (`USERNAME_RESERVED`); çünkü denetim günlüğü artık o aktörü "Sistem" diye
  söyler ve bir kişi sistem olarak gösterilmemelidir (D-024: ekran doğru olanı
  söyler). Bu bir politika değişikliği değil, bir doğrulama kuralıdır; var olan
  satıra dokunulmaz.

Notun durumu: kaynakta, bileşen testleri ve taklit tarayıcıyla (13.–15.
tarayıcı inceleme turları). `deploy/e2e/release-recovery/evidence/` altındaki
set17 (`313f3eea2`'de) ve set17b (`ecd2a5f08`'de) gerçek sistem kayıtları
sakin yeniden başlamayı ölçtü; onlardan sonra yapılan kilit değişikliği
(`7265d3c99`) ve oturum kapısının `PANEL_STARTING` yolu gerçek sistemde
ölçülmedi ve kurulu bir sunucuda hiçbir şey ölçülmedi. Hangi kontrolün hangi koda
dayandığını [alpha.86 sürüm notları](RELEASE-NOTES-v0.1.0-alpha.86.tr.md)
söyler. Hiçbir P0 işi kapanmaz ya da ilerlemez.

*16 Ekim 2026 tarihli not (saat tarihi): v0.1.0-alpha.87 adayında (yayımlanmadı)
uygulandı.* O aday için yeni bir karar kaydedilmez; yukarıdaki metin değişmedi.

- Kurulum genişletmesi için 1. kural ve D-024: hazır ekranı ve Ayarlar, bir şeye
  basılmadan önce "Bu sunucunun kurulumunu genişletin"in neyi yapıp neyi
  yapmadığını söyler ve incelemeden önce hiçbir şey başlamaz; bir genişletme
  çalışırken Dashboard ve Ayarlar, "Genişletmeye devam et"in yanında "bir şey
  yapmanız gerekmiyor" demek yerine ilerlemesine bağlantı verir ("İlerlemesini
  göster").
- set18 gerçek sistem kaydından sonra 1. ve 4. kurallar: başlayan bir kurulum
  çalışması kendi ilk adımını adlandırır ve sayfanın kendiliğinden
  güncellendiğini söyler (O1); basılan "Kayıtlı işleme devam et", bir okuma
  işlemin ilerlediğini gösterene kadar, en çok 15 saniye, "Aynı işlem
  denetleniyor" olarak kapalı kalır (O3; yalnız okuma, sunucu aynı isteğin
  tekrarını ikinci işlem başlatmadan yanıtlar); taslaktaki bir genişletme artık
  tamamlanmış çalışmanın "Tamamlanma doğrulanamadı" ekranı olarak gösterilmez
  (D1, hazır olan bir sunucuda yanlış alarm).
- İkincil DNS sunucusu için 5. kural: posta kutusu, sahibi kaydı oluşturamayan
  bir DNS sayfasına göndermek yerine neyi beklediğini (birincildeki ya da DNS
  sağlayıcısındaki kayıt, tam kaydıyla) adlandırır ve profil kartları "Posta DNS
  kaydı bekleniyor" der. "Gereksinimleri tekrar kontrol et" yanıtı yönlendirmeyle
  aynı türlü nedeni söyler.
- Bileşenler posta penceresi kurulum sorulurken kilitlidir (sessiz çıkış yok, 1.
  kural) ve bir reddi görünür alanda tutar.

Notun durumu: kaynakta, bileşen testleri ve taklit tarayıcıyla (16. tur);
set18b (`6b363ac46`'da) bir laboratuvarda D1, O1 ve O3'ün düzeldiğini gözledi;
ikincil DNS kutularına gerçek sistemde ulaşılmadı; kurulu bir sunucuda hiçbir şey
ölçülmedi. Hiçbir P0 işi kapanmaz ya da ilerlemez.

---

## D-032 · Panel kendiliğinden bakar; okuyabileceği bir durumu öğrenmek için sahibe bir düğmeye basmasını asla söylemez

*12 Ekim 2026 (saat tarihi) · v0.1.0-alpha.83 güncellemesinden sonra sahip kararı; kaynakta, bileşen testleri ve API taklidine karşı tarayıcıyla; ilk adım on dördüncü gerçek sistem kaydında ölçüldü (Debian 13, Ubuntu 24.04, 12 Ekim 2026 UTC); tarama yaşı saat kuralı ile tarama ve kabul denetimindeki tipli Agent yanıtı on beşinci kayıtta ölçüldü (Debian 13, 13 Ekim 2026 UTC); lisans denetiminin kalan süresi, ortak hata yazıcısındaki Agent yanıtı ve okuma şeridi (13 Ekim 2026) bileşen testli, sonra on altıncı kayıtta Debian 13'te ölçüldü (set14c, 13 Ekim 2026 UTC); on beşinci kayıtta bulunan lisans denetimi kusuru orada yok*

Dashboard'ın "Sistem servisleri" kartı, saklı tarama beş dakikadan eskiyse
"Bilinmiyor. Bileşen durumu eski. Yeniden taramak için Bileşenleri açın."
diyordu. Bileşenler sayfası, Panel'in sunduğu önbellekli, koşullu taramayı
(`POST /api/v1/managed-services/scan?max_age_seconds=N`: aynı anda tek tarama,
önbellekli, bir değişiklik sunucunun kilidini tutarken reddedilen) zaten
çalıştırıyordu; Dashboard çalıştırmıyordu. Böylece sahip, Panel'in kendi
yapabileceği bir okumayı elle yapmak zorunda kalıyordu. v0.1.0-alpha.83
güncellemesinden sonra sahip 12 Ekim 2026'da karar verdi: Dashboard kaynak
harcamadan kendiliğinden kontrol etmeli.

Benimsenen kurallar:

1. Başarılı bir okumadan sonra, saklı tarama yoksa ya da 300 saniyeden eskiyse
   Dashboard koşullu taramayı çalıştırır; yanıt ne olursa olsun beş dakikalık
   pencere başına en çok bir kez.
2. Yalnız sekme görünürken. Gizli sekme hiçbir şey çalıştırmaz; yeniden
   gösterilen sekme, kontrol zamanı gelmişse bir kez kontrol eder.
3. Başka bir kontrol sürerken asla. Ne olursa olsun tarayan aralıklı bir
   zamanlayıcı yok; reddedilmeden sonra yeniden deneme döngüsü yok.
4. Bir değişiklik sürdüğü için gelen ret (`server_setup_busy`,
   `service_operation_busy`) "bir değişiklik sürüyor" diye gösterilir ve başka
   bir şey göndermez. Mevcut hazırlık yoklaması sunucunun meşgulden hazıra
   geçtiğini görünce bir takip çalışır (yalnız paket işlemleri için; kurulum
   retinden sonra bir sonraki kontrol pencereyi bekler).
5. Başarısız bir kontrol yerinde düğmeyle "Kontrol edilemedi: <neden>" der.
   Sahibin basışı koşulsuz kalır.
6. Kart dört ayrı durum çizer (kontrol ediliyor, sonuç, kontrol edilemedi,
   tutuluyor) ve "İlgi istiyor" cümlesi onları izler.

Kaynak sınırı kararın parçasıdır, sonradan eklenmiş bir şey değil: bakılan bir
Dashboard için beş dakikada en çok bir koşullu tarama, kimsenin bakmadığı için
hiç. İki laboratuvar konuğunda ölçüldü (on dördüncü kayıt, ilk adım): bayat bir
yükleme tam bir tarama gönderdi, taze bir yükleme hiç, görünür tutulan bir sayfa
300 saniyeden önce olmamak üzere bir tarama daha, gizli bir sekme beş dakika
hiç ve gösterilince bir; bir kontrol Agent'a yaklaşık 0,2 ile 0,4 saniye CPU'ya
mal oldu (47 saniyelik iki pencereden bir tahmin).

Saat kuralı. Sunucu bir taramanın yaşını söyler (`scan_age_seconds`, üç
yönetilen-servisler yanıtında, yanıt yazılırken sunucu saatiyle ölçülür); sayfa
taramayı yanıtı aldığı an eksi bu yaş olarak tarihler. Tarayıcı kendi saatini
sunucununkiyle asla karşılaştırmaz. Kuralın iki adımı vardır, çünkü laboratuvar
kusuru ilk sürümde buldu: konuğun saati bir iki saniye ileriyken kart her
kontrolden sonra 20 ile 29 saniye "Son 5 dakikada kontrol edilmedi" gösterdi.
Birinci adım (damga ile alınma anının erken olanı) ileride olan sunucu için
kusuru kaldırdı; ikincisi (sunucunun yaşı) beş dakikadan fazla geride olan
sunucu için de kaldırır. Alanı olmayan eski bir sunucu birinci kuralda kalır.
Eksi bir yaş ya da sonlu olmayan bir sayı taze sayılmaz. Ölçüldü (on beşinci
kayıt, Debian 13, konuk 1,5 sn ileride): bayat yüklemede tek koşullu tarama ve
kartın doğrudan sonuca geçmesi, "Son 5 dakikada kontrol edilmedi" karesi
olmadan. Sunucu saatinin geride olduğu durum sonradan, on altıncı kayıtta
ölçüldü (set14c, Debian 13, 13 Ekim 2026, saat yedi dakika geride): tek koşullu
tarama, ardından 90 saniyenin sonuna dek sonuç.

Aynı kural, onu bozduğu laboratuvarda görülen lisans denetimi için de geçerlidir.
`/api/v1/license/access` erişimin sonunu (`valid_until`) sunucunun saatiyle bir an
olarak veriyordu ve sayfa, sonu tarayıcının saatinden sonra olmayan olumlu bir
yanıtı reddediyordu; sunucunun saati yaklaşık bir dakikadan fazla geride olduğunda
hiçbir Panel sayfası açılmadı (on beşinci kayıt, D2 kusuru: 418 sn geride, denetim
90 sn'de 19 kez okundu). Sunucu artık kalan süreyi söyler (`valid_for_seconds`:
yanıt yazılırken `valid_until` eksi sunucunun saati, sıfırın altına inmez;
`valid_until` eski sayfalar için kalır) ve tarayıcı sonu, isteği gönderdiği an artı
bu süre olarak alır; böylece iki saati hiç karşılaştırmaz ve sunucunun izin
verdiğinden geç bitmez. Alan yoksa mutlak kural sürer. Düzeltme Debian 13'te
on altıncı kayıtta ölçüldü (13 Ekim 2026): konuk saati tarayıcıdan yedi dakika
geride ve yedi dakika ilerideyken sayfa açıldı ve erişimi 90 saniyede dört kez
okudu; doğru saatle olduğu gibi.

Tipli hata kuralı. Kendi Agent'ının yerel soketini açamayan bir Panel `503
AGENT_UNREACHABLE` yanıtlar: nedeni, hiçbir şeyin değiştirilmediğini, kimin
işlem yapacağını, sonraki eylemi ve işin nasıl süreceğini söyler (D-024). Yanıt
ortak sunucu hata yazıcısında tek yerde yazılır; bu yüzden o yazıcıya, Agent
soketine başarısız bağlantı kurmanın tek neden olduğu bir durumla ulaşan her
işleyici için geçerlidir (tarama, değiştirici işleyicilerin kabul denetimi
(`beginServiceMutation`; 68b79b372 commit'inde tarama işleyicisi dışında dokuz
çağrı yeri), güvenlik duvarı okuması, sistem denetimi), işleyici işleyici değil.
Yalnız başarısız bağlantı kurma içindir. İstek gönderildikten sonra kopan
bağlantı bilinmeyen sonuçtur ve genel yolda kalır; bilinmeyen sonuç asla
doğrulanmış bir başarısızlığa çevrilmez. Birleşik hata da çevrilmez; örneğin bir
hata ve onun telafisi: önceki bir parça bir şeyi değiştirmiş olabilir ve "hiçbir
şey değiştirilmedi" yanlış olurdu. Kabul edilen tek birleşim, platform kimliği
işaretiyle nedenidir; çünkü o okuma koruduğu çağrıdan önce koşar. DNSSEC durumu,
güncelleme durumu ve başlatması, alan adı silme ve posta parolası döndürme kendi
tipli yollarında kalır. Sıradan bir hata yine `500` yanıtlar. Tarama ve kabul
denetiminde (on beşinci kayıt) ve güvenlik duvarı okuması, sistem denetimi ile
bir yeniden başlatma isteğinde ölçüldü (on altıncı kayıt; `503`, yalnız `code`
ve `error`, nginx değişmedi). Açık: bir şeyi değiştirip sonra, önceki sonuca
birleşmemiş ayrı bir bağlantıda başarısız olan bir işleyici yine de hiçbir şeyin
değiştirilmediğini söylerdi.

Okuma şeridi kuralı. Başarısız bir okuma asla bir işlem iddia etmez. Bir durumu
okuyamayan ekran, okuyamadığını ve nedenini, sonraki eylemle söyler; bir işlem
gönderilmediyse işlemin tamamlanmış olabileceğini söylemez. Neden saklanır:
gönderilmiş ve doğrulanmamış bir işlem "tamamlanmış olabilir" sözünü korur ve
sonradan başarısız olan bir okuma onun yerini asla almaz. Laboratuvarda, Agent
durmuşken Bileşenler sayfasında bulundu (on beşinci kayıt, O2 gözlemi) ve orada
uygulandı; on altıncı kayıtta ölçüldü (okuma şeridi İngilizce ve Türkçe, işlem
ifadesi yok, Agent dönünce tek Yeniden tara ile gitti). Bu kuralla başka hiçbir
ekran dönüştürülmez.

Bunun yetkilendirmediği:

- Kimse bakmazken çalışan bir arka plan zamanlayıcısı. Bu karar, taze bir kart
  uğruna zamanlanmış bir tarama eklemez; Dashboard, bir kişi onu açık ve görünür
  tuttuğunda bakar.
- Bir değişiklik başlatan yoklama. Kontrol, Bileşenler sayfasının zaten sunduğu
  salt-okur taramadır; bir durumu gören yoklama asla değişiklik başlatmaz ve
  hazırlık yoklamasının tek takibi de aynı taramadır.
- Bu kararla başka hiçbir kart, sayfa ya da ekran dönüştürülmez. Dashboard'daki
  posta yığını kartı, kendi değişikliğine kadar kendi bayat metnini ve "Yeniden
  taramak için Bileşenleri aç" düğmesini korur; sunucuyu hâlâ eski biçimde
  okuyan ekranlar sürüm notundaki listede kalır.
- Temelin bittiği iddiası yok: hiçbir P0 işi kapanmaz ya da ilerlemez.

Gerisiyle ilişkisi. D-024 her işlemin nedenini, kimin işlem yapacağını, sonraki
eylemi ve işin nasıl süreceğini göstermesini ve doğrulanmış başarısızlık,
karşılanmamış önkoşul ve bilinmeyen sonucu ayırmasını ister; bu karar onu bileşen
kontrolüne ve ulaşılamayan Agent'a uygular. Bilinen-durum kuralı (başarısız ya da
eksik bir okuma boş ya da olumsuz bir durum gibi çizilmez) kartın neden
"kontrol edilemedi" ve "tutuluyor" durumlarına ayrı ayrı sahip olduğunu ve bayat
durumun "Bilinmiyor"a indirgenmediğini açıklar. D-022 (sunucuyu sahibi işletir;
Panel saptar ve bildirir) Panel'in Agent'ı yeniden başlatmak yerine sahibe onu
nasıl denetleyeceğini söylemesinin nedenidir. Panel güncellemeleri sahibin
kalır, panelin kendi güncelleme ekranından.

---

## D-031 · Sahibin değiştirdiği site yapılandırma dosyası korunur ve adlandırılır; üretim onun üstüne yazmaz

*10 Ekim 2026 (saat tarihi) · Salt-okur denetimin bulgusu üzerine sahip kararı ([SITE-CONFIG-OWNER-EDITS-2026-10-10](audit/SITE-CONFIG-OWNER-EDITS-2026-10-10.md)); tasarım kararlaştırıldı; ilk adım onuncu gerçek sistem kaydında ölçüldü (iki bulgu 11 Ekim 2026'da düzeltildi); “şimdi yaz” eylemi ile tek öncelik sırası (11 Ekim 2026) ve ikinci adım (havuz, uygulama birimi; 11 Ekim 2026) bileşen testli ve sahte tarayıcıyla denendi, gerçek sistem ölçümü bekliyor*

Denetim, yalnız kaynaktan okuyarak şunu buldu: Panelin açılışı barındırılan
her sitenin nginx yapılandırmasını yeniden üretir ve karşılaştırmasız,
özetsiz, yedeksiz yazar (`cmd/panel/cert_startup_reconcile.go`,
`internal/services/nginx_generator.go`); diğer her üretim yolu (sertifika,
ayar, barındırma türü, PHP sürümü, içe aktarma) aynısını yapar. Bu okumaya
göre sahibin eklediği `location`, değiştirdiği yönerge, kendi include satırı
ya da değiştirdiği dosya tek söz edilmeden ezilir; kaldırılan site dosyası bir
sonraki açılışta yeniden yaratılır. Yayımlanmış v0.1.0-alpha.81 ve
v0.1.0-alpha.82 bunu yapar. Bu, D-022'nin yasakladığı davranıştır: Panel
sahibin değişikliğini algılar, üstüne yazmaz. Karar alındığında bunların
hiçbiri gerçek sistemde ölçülmemişti; sekizinci gerçek sistem kaydı önce
yayımlanmış davranışı ölçer.

Sahip 10 Ekim 2026'da düzeltmenin bir sonraki sürüme girmesine karar verdi:

1. Panelin bir site için yazdığı her dosya, gövdesinin özetini taşıyan bir
   başlık alır; Panel dosya başına bir defter satırı tutar (şema geçişi
   43 → 44, göç 044, tablo `managed_site_files`; D-025 bunu adlandırır).
2. Her üretimden önce Agent diskteki dosyayı sınıflandırır: yok, yönetilen ve
   değişmemiş, sahip düzenlemiş, yabancı, okunamaz, kökeni bilinmiyor. Yalnız
   yok ya da yönetilen-ve-değişmemiş dosya yazılır; aynı baytlar yeniden
   yazılmaz ve onlar için nginx yeniden yüklenmez.
3. Diğer her dosya korunur. Site "yapılandırması sahibi tarafından
   düzenlenmiş" diye gösterilir, fark gösterilir; Panelin bekleyen metni
   dosyanın yanında tutulur; sahip "benimkini koru", "CelikPanel'inkini al"
   (tarihli yedekle) ya da elle birleştirmeyi seçer. Sahibin kaldırdığı site
   dosyası açılışta yeniden yaratılmaz; Panel yeniden yaratmayı önerir.
4. Şablon, sahibin eklemeleri için site başına bir include dizini kazanır;
   sonraki üretimlerin onlara dokunması gerekmez. (Adım 1b, 2026-10-10,
   planlayıcı kararı:) Şablon ayrıca Panel'e ait ikinci bir include dizini
   (`/etc/nginx/celikpanel-managed.d/<alan adı>/`) kazanır; ACME HTTP-01
   konumu oraya yazılır, sanal konağa değil. Korunmuş bir dosyanın doğrulamaya
   izin verip vermediği metninden okunmaz, ölçülür (aynı gün, ikinci tur):
   Agent doğrulama köküne rastgele adlı ve içerikli bir yoklama dosyası koyar,
   127.0.0.1'in 80 numaralı bağlantı noktasındaki nginx'e (2026-10-11'den beri:
   adın korunan dosyadaki 80 bloklarının dinlediği her adreste) onu
   sertifikanın doğruladığı her adla sorar ve dosyayı kaldırır; hazır, her adın onu
   sunması demektir. Hazır (ve doğrulama dosyası değişmemiş) korunmuş bir
   dosyada sertifika dosyaya dokunmadan alınır ya da yenilenir ve sahip
   CelikPanel'in metnini alana ya da sertifika satırlarını değiştirene dek
   dosyanın gösterdiği sertifika, varsa, sunulur (`waiting_for_owner`); hazır
   olmayan korunmuş dosyada alım hiçbir şey istenmeden tipli bir retle
   durur, yenileme başarısız değil "sahibi bekliyor" olur; nginx'in yanıt
   vermediği yoklama bilinmiyordur, asla "hazır değil" değildir. Takma ad
   sertifikası yolu da aynı iki yolu izler.
5. Yazma kip ve sahibi korur, sembolik bağları reddeder; bir sitenin hatası
   diğerlerini düşürmez; açılış satırı yazılan, değişmeyen, korunan, yabancı
   ve okunamayanı ayrı ayrı sayar.
6. alpha.81 ve alpha.82'nin yazdığı dosyalarda özet yoktur. Göç her siteyi o
   sürümlerin dondurulmuş şablonlarıyla bellekte üretip karşılaştırır: bayt
   bayt eşit dosya benimsenir, diğer her dosya "kökeni bilinmiyor"dur ve asla
   üstüne yazılmaz. Güncellemeden sonraki dürüst ilk durum "N benimsendi, M
   bilinen hiçbir CelikPanel metniyle örtüşmediği için dokunulmadı"dır ve
   Panel bunu gösterir. (Onuncu gerçek sistem kaydı, 2026-10-11, planlayıcı
   kararı:) benimsenen dosya bu sürümün metni olarak yeniden üretilir ve
   başlığı alır; eski baytlara eklenmiş bir başlık değildir. Sahibin eklemeleri
   onda olamazdı, çünkü dosya CelikPanel'in kendi metniyle bayt bayt eşitti.
   `adopted_from`, metni eşleşen en yeni sürümü adlandırır (ikisi de eşleşirse
   alpha.82). Onuncu gerçek sistem kaydında ölçüldü.
7. Eski bir sürüme otomatik dönüş korumayı kaldırır; sürüm notu bunu söyler.
8. (İkinci adım, 2026-10-11; kaynakta, bileşen testli ve sahte tarayıcıyla
   denendi; gerçek sistemde ölçülmedi.) Sitenin PHP-FPM havuz dosyası (tür
   `php_pool`) ve uygulama birimi (tür `app_unit`) 1-7. maddelere uyar. Havuz
   INI'dir; başlığı `;` yorum satırıdır (`; celikpanel-render v2 sha256=…`);
   birimin başlığı `# celikpanel-render v2 sha256=…`. Her biri her yazmadan
   önce sınıflandırılır (havuz: kaydetme, PHP sürümü değişikliği, oluşturma,
   silme; birim: barındırma değişikliği ve onun telafisi, kaldırma; ikisi de:
   sayfanın seçimleri); aynı baytlar yeniden yazılmaz ve onlar için hiçbir
   şey yeniden yüklenmez; korunan dosyanın yanına CelikPanel'in metni konur ve
   işlem `409 SITE_CONFIG_OWNER_EDITED` verir (`reason` tür, `detail` durum).
   Yazma geçici dosya ve tek yeniden adlandırmayla olur; kip, sahip ve grup
   korunur; sembolik bağlantı reddedilir. PHP-FPM'in havuz için ekleme düzeni
   yoktur; sahibin yönergeleri dosyanın içinde desteklenir: CelikPanel'in
   başlığını ve CelikPanel'in üretmediği satırları taşıyan havuz sahibin
   düzenlemesidir, korunur; hiçbir kaydetme ya da sürüm değişikliği onları
   düşürmez. Birimin sahip için genişletme yeri systemd drop-in dizinidir
   (`<birim>.service.d/*.conf`); CelikPanel oraya hiç yazmaz, onu silmez.
   Havuzun süreç yöneticisi değerlerini tutan bir veritabanı sütunu yoktur:
   CelikPanel'in havuz metni onları kaydetmeden, yoksa dosyanın yanındaki
   kendi bekleyen metninden, yoksa dosyanın o anahtarlar için taşıdığı
   değerlerden, yoksa varsayılanlardan alır; kimlik satırları her zaman
   sitenindir. Benimseme: sitenin kimliği ve taşıdığı değerlerle dondurulmuş
   alpha.81/82 biçimine bayt bayt eşit başlıksız havuz ya da aynı istekle
   alpha.81/82 birimine bayt bayt eşit başlıksız birim benimsenir
   (`adopted_from` eşleşen en yeni sürüm) ve bir sonraki yazmada bu sürümün
   metni olarak yazılır; başka her dosya bilinmeyen kökenlidir. Havuz ve birim
   Panelin açılışında üretilmez (açılışta yeni yazma yok); açılış satırı
   yalnız vhost'ları sayar. Şema değişmez: 44. göçün `kind` sütunu
   `php_pool` ve `app_unit` adlarını zaten taşır. Yapılandırma dosyası sayfası
   sitenin her dosyasını kendi seçimleriyle listeler; Alan Adları satırı
   dosyalar arasında en kötü durumu gösterir (eşitlikte vhost önce gelir).
   Sınırlar, açıkça (2026-10-11): Panel'in iki havuz çağrısı Panel'in kendi
   derlemesine bağlıdır; onlara sahip olmayan bir Agent hiçbir şey yazılmadan hata
   verir (eski havuz çağrılarına geri dönüş yok; `MigratePHPPool` Panel'in
   politikasından çıkarıldı). Birimin iki çağrısı da Panel'in derlemesine aynı
   biçimde bağlıdır (2026-10-11 düzeltmesi). Site oluşturma havuz ya da birim
   defter satırı yazmaz: satır sayfanın ilk okumasında ya da ilk kaydetme,
   sürüm değişikliği veya birim değişikliğinde başlar; bu yüzden kimsenin
   okumadığı bir havuz Alan Adları listesinde yoktur. 2026-10-11 düzeltmesi
   (bileşen testli ve sahte tarayıcıda denendi; gerçek sistemde ölçülmedi):
   Agent'ın okumadığı dosya (`unknown`) en-kötü-durum kuralında CelikPanel'in
   değişmemiş metninin üstünde sıralanır (gözlenen her sorun yine onun
   üstündedir), yani Alan Adları satırı bir yapılandırma dosyasının
   okunamadığını söyler; bir hizmet reddi `restore: confirmed | unconfirmed`
   taşır ve doğrulanmamış geri koyma (Agent'ın “rollback incomplete” demesi ya
   da hiç geri koyma bilgisi vermemesi) dosyanın geri konduğunu söylemez,
   sunucu yöneticisinin dosyayı sunucuda kontrol edeceğini söyler. İki benimseme
   tabanı farklıdır: alpha.81'in havuzu `/var/run/php/php<s>-fpm-<havuz>.sock`
   soketini `www-data` sahibiyle dinler, alpha.82'nin havuzu bu sunucunun
   soketini web sunucusu hesabı sahibiyle; alpha.81'in birimi `User=` satırını
   istendiği gibi yazar, alpha.82'ninki sunucuda var olan web sunucusu hesabını.

Debian 13, Ubuntu 24.04 ve Arch'ta gerçek sistemde ölçülmeden bitmiş
sayılmaz: denetimin §9 hücreleri (sahip düzenlemesi açılışta ve her üretim
yolunda korunup adlandırılıyor; değişmez dosya diğer siteleri durdurmuyor;
güncelleme, dönüş ve yeniden ileri; başlıktan önceki veritabanı geri
yüklemesi; benimkini-koru sonra sertifika alma; CelikPanel'inkini-al ve
yedeği; sahibin include dizini yenileme ve geçişlerden sağ çıkıyor;
kaldırılan dosya yeniden yaratılmıyor; havuz yönergeleri korunuyor). PHP
havuzu ve uygulama birimi aynı kural altında ikinci adımda gelir (8. madde,
11 Ekim 2026'da kaynakta; hücreleri set10 düzeneğinde m, n ve o bölümleri
olarak var, henüz bir konukta koşulmadı).
Onuncu gerçek sistem kaydı (set10, 2026-10-10, `cd46ca595`, Debian 13, Ubuntu
24.04, Arch): iki hücre dışında hepsi tuttu. 2026-10-11 düzeltmesi (bileşen
testli; ölçülmedi): sahibin ekleme dizini açıkça 0755 oluşturulur (Agent'ın
`UMask=0027` ayarıyla `root:celikpanel 0750` ölçülmüştü), CelikPanel'in kendi
dizini de öyle; kilitli bir dosya (`chattr +i`, EPERM) alan adının sayfasında,
liste satırında ve rozetinde aynı biçimde `unwritable` (değiştirilemiyor)
olarak yanıtlanır: sistem hatası, salt okunur ölçülen koruma, CelikPanel'in
yazmak istediği metin ve "Yeniden kontrol et" ile (sayfa nedensiz
`managed_unchanged` yanıtlıyordu). Defter göç 044'ün durum kümesini korur
(`unreadable` + `write_refused`); şema değişmedi. Yeni ve yeniden oluşturulan
sanal konaklar `root:celikpanel 0644`; korunan ve alınan dosyalar sahibin
kipini, sahibini ve grubunu korur (ölçüldü).
İkinci düzeltme (2026-10-11; bileşen testli ve sahte tarayıcıyla denendi;
gerçek sistemde ölçülmedi): sayfanın okuması “yeniden denetle”dir ve reddedilen
bir değiştirmenin defter satırına bulduğunu yazar. Aynı baytlarda ölçülen
koruma yoksa CelikPanel'in kendi metni `managed_unchanged` olur, nedeni
`protection_removed`; sayfa “CelikPanel'in metnini şimdi yaz” önerir
(`POST …/site-config/apply`, iki özet, istek kimliği, yalnız yönetici). Bu
siteye ait bekleyen metni yazar, tarihli kopya tutmaz (dosya CelikPanel'in kendi
önceki metnidir; sahibin hiçbir şeyi içinde yok) ve nginx reddederse önceki
baytlar geri konur. Koruma sürüyorsa `unwritable` kalır; baytlar farklıysa dosya
olduğu gibi kaydedilir ve asla yazılmaz. İki olgu aynı baytlar için geçerliyse
satırın, rozetin ve sayfanın ne göstereceğini tek sıra belirler: okunamıyor,
değiştirilemiyor, yok, korunan (sahip düzenlemesi, yabancı, bilinmeyen köken),
sonra CelikPanel'in değişmemiş metni (`services.SiteFileShownState`). “Al”ı
reddedilen sahip düzenlemesi iki olguyu birlikte gösterir ve koruma sürerken
“al” hiçbir şey yazılmadan `409 SITE_CONFIG_UNWRITABLE` ile yanıtlanır ve
reddedilen bir “al” tarihli kopya bırakmaz (Agent korumayı kopyadan önce ölçer,
değiştirme yine de reddedilirse kopyayı kaldırır; sanal konak ve havuz
testli, birim havuzun koduyla aynı kodu kullanır, set11 hücre B4; bileşen
testli; gerçek sistemde ölçülmedi). Şema değişmez.
İkinci turun yoklamasında açık kalanlar (2026-10-10), ilki yanıtlandı: (1) tek
bir adrese bağlı 80 numaralı bağlantı noktası bloğuna geri döngüden
ulaşılmıyordu — 2026-10-11'de yanıtlandı (set11 hücre A6; bileşen testli;
gerçek sistemde ölçülmedi): yoklama, her ad için korunan dosyanın 80 numaralı
bloklarının dinlediği adreslere sorar
(`internal/services/managed_vhost_listen.go`); bu sunucunun olmayan bir adres
bilinmeyendir ve sertifika isteği sürer, `validation_probe` `not_run`, yalnız
günlük, şema değişmez, 503 yerel nedenlerde kalır; geri kalanlar hâlâ açıktır ve
hiçbiri giderilmedi: Yapılandırma dosyası sayfasının okumasında başarısız bir `nginx -t`
ya da yeniden yükleme `challenge_failed` olarak bildirilir; eksik ya da
okunamayan dosya neden yazmaz, bu yüzden durdurduğu yenileme bir sonraki
tamamlanan yenilemeye dek bekler; (2026-10-11'de giderildi: SSL sekmesi eksi
gün yerine "süresi N gün önce doldu" yazar); gerçek bir barındırma müşterisi oturumu çizilmedi; hazır bir
yoklama, DNS'in ya da güvenlik duvarının sertifika otoritesini içeri
aldığını göstermez. Yoklama hiç gerçek bir nginx'e sormadı.

On ikinci yerel kayıttan düzeltmeler (set12, 2026-10-11; iki bulgu on ikinci
kayıtta ölçüldü, `816b8dd60` işlemesinde düzeltildi, yeni kod bileşen testli;
gerçek sistemde ölçülmedi): Agent'ın
yanıt vermediği havuz ya da birim, defter satırına bir gözlem olarak yazılır
(`unknown`, `not_read` ya da `agent_does_not_report` ve hata sınıfı, hiçbir özet
değişmez); böylece yanıt veren bir sonraki okumaya dek Alan Adları satırı
okunamadığını söyler; bilinen bir sorunu olan satır onu korur. Geri koymadan
sonra önceden çalışmayan hizmete dokunulmaz (`service: not_running`); aksi hâlde
hizmetin kendi denetimi (`php-fpm -t`, `nginx -t`, birim için
`systemd-analyze verify`) önce geri konan yapılandırmada çalışır: reddederse
hizmet yeniden yüklenmez ve önceden yüklediği yapılandırmayla çalışır
(`restore: confirmed`, `service: not_reloaded`, `config_refused_elsewhere`,
yönetici için satır); geçerse yeniden yüklenir; bu yanıtların her biri
`service_active` taşır. Uygulama biriminin başlatma komutu artık giriş kabuğuna
bağlı değildir (Panel'in Node dizinini adlandıran açık bir
`Environment=PATH=` ile `/bin/sh -c`; gözlenen: PATH satırı orada olduğu hâlde
`/bin/sh -lc` altında çıplak `node` 127 ile çıktı; giriş profilinin onu
sıfırlaması bir çıkarımdır ve yeni birim gerçek bir sistemde çalışmadı);
önceki sürümlerin birim metinleri benimseme için donmuş kalır. PHP sürümü
değiştirmenin kaynak havuzu silmesi ve temizliği hâlâ denetimsiz yeniden
yükler. Sayfanın bir seçime yanıtı görünür alana kayar. Şema değişikliği yok.

---

## D-030 · Panelin güvenli bağlantı kuralı yalnız Panelin kendi ana makine adını kapsar

*10 Ekim 2026 (saat tarihi) · Sahip kararı; kaynakta ve bir bileşen testiyle; çalışan Panelde üç platformda ve bildirildiğine göre kurulu iki sunucuda okundu (aşağıdaki 11 Ekim 2026 notu); tarayıcının eski kuralı değiştirmesi ölçülmedi*

Panel, HTTPS üzerinde `Strict-Transport-Security: max-age=31536000;
includeSubDomains` gönderiyordu. Paneli bir adda açmış olan tarayıcı bu yüzden
o adın altındaki her adda bir yıl boyunca düz HTTP'yi reddediyordu. O adlar
Panelin değildir: sahibi onları başka yerde, sertifikasız ya da hiç CelikPanel
üzerinden olmadan sunuyor olabilir (D-022). Barındırılan siteler hiçbir zaman
`includeSubDomains` ya da `preload` taşımadı; bir test bunu orada zaten
yasaklıyordu. Panelin kendi başlığını kapsayan bir karar kaydı yoktu.

Sahip 10 Ekim 2026'da kuralın Panelin kendi ana makine adıyla sınırlanmasına ve
değişikliğin v0.1.0-alpha.82'ye girmesine karar verdi. Panel artık HTTPS
üzerinde `Strict-Transport-Security: max-age=31536000` gönderir, düz HTTP
üzerinde hiçbir şey göndermez. `preload` eklenmez.

Sahibin gördüğü: Panelin kendisinde hiçbir şey değişmez. Tarayıcı, Panelin ana
makinesi için tuttuğu kuralı Panele bir sonraki girişinde okuduğuyla
değiştirir; yani geniş kural o tarayıcı için o girişte sona erer. Paneli bir
daha açmayan tarayıcı eski kuralı bir yılı dolana kadar tutar; sahibi onu o
tarayıcının kendi ayarlarından silebilir. Bu bir tarayıcıda ölçülmedi; başlığın
tanımı böyledir (RFC 6797, bölüm 8.1).

Değişmeyenler: başlığın süresi, Panelin diğer güvenlik başlıkları, barındırılan
sitelerin başlıkları.

*Not, 11 Ekim 2026 (o günden bu yana okunanlar).* (1) Altıncı gerçek sistem kaydı
(`deploy/e2e/release-recovery/evidence/set6-20261009/README.md` ve
`part-b-headers.md`) başlığı çalışan Panelden Debian 13, Ubuntu 24.04 ve Arch'ta
okudu: aday, okunan her HTTPS yanıtında (sayfalar, varlıklar, herkese açık ve
oturumlu API yanıtları, 401/403/404 yanıtları, ön kontrol isteği) tam olarak
`max-age=31536000` yanıtladı; güncellemeden önce yayımlanmış v0.1.0-alpha.81 ise
`max-age=31536000; includeSubDomains` yanıtladı; bunlar atılabilir deneme
konuklarında konuğun içinden yapılan okumalardır. (2) Sahip kurulu iki sunucuyu
10 Ekim 2026'da güncelledikten sonra her birinin dıştan okunması daraltılmış
başlığı gösterdi; bu, planlayıcının kendi okumasıyla bildirilmiştir ve kurulu
sunucularda gözlenmiştir, burada bir dosyada kayıtlı değildir
(`docs/RESILIENCE-CONTRACT.tr.md` içindeki “v0.1.0-alpha.82'nin yayımlanması …”
girdisine bakın). **Ölçülmedi:** bir tarayıcının elindeki kuralı bir sonraki
girişte daraltılmışıyla değiştirmesi (yukarıdaki paragraf hâlâ başlığın tanımıdır,
RFC 6797 bölüm 8.1, bir gözlem değildir); eski kuralı tutan bir tarayıcı; telefon
ya da başka bir tarayıcı; Panelin üçüncü taraf bir istemcisi. Bu notla hiçbir
karar, kod ya da şema değişmedi.

---

## D-029 · Durum değiştiren her istek tek bir kimlik taşır; yineleme asla iki kez çalışmaz

*10 Ekim 2026 · Sahibin yürürlükteki yönü üzerine planlayıcı kararı (D-024: kaybolan yanıt asla ikinci bir değişiklik başlatmaz; D-025 ilke 4); birinci grup kaynakta ve bileşen testleriyle, gerçek sistem denemesi bekliyor*

Gerçek tarayıcıda yapılan bir inceleme şunu ölçtü: durum değiştiren bir istek
gönderilirken bağlantı sıfırlanırsa Chrome POST'u kendiliğinden yeniden
gönderir; tek tıklama Panel'e üç kez ulaştı. Ardından `7a64bda91` kaynağının
salt-okur dökümü, durum değiştiren yaklaşık 115 rotayı sınıflandırdı: 12'si iki
kez çalıştığında zararlı, 7'si sınıflandırılamadı, yaklaşık 38'i durumu doğru
bırakıp yanlış bildiriyor, yaklaşık 58'i güvenli. Gövdesinde istek kimliği
taşıyan ve işlem satırı tutan alt sistemler (bileşen kurulumu, posta profili
kurulumu, panel sertifikası, sunucu kurulumu, DNS motoru geçişi, panel
güncellemesi) güvenliydi.

Birincinin altında ikinci bir tehlike vardı. İşleyicilerin çoğu bağlantının
bağlamını Agent çağrısına ve sonrasındaki veritabanı yazımlarına veriyordu.
Sıfırlama bu bağlamı iptal eder: Panel dönerken Agent çalışmayı sürdürüyor,
sunucu değişiyor ama Panel bunu kaydetmiyordu; yineleme de bu yarım kalmış ilk
denemeye karşı çalışıyordu.

**Karar.** Kendini zaten adlandırmayan her durum değiştiren istek için tek
düzenek.

- *Kimlik.* `X-CelikPanel-Request-Id` başlığı: 32 küçük harfli onaltılık
  karakter; ürünün mevcut işlem kimliği biçimi. Web arayüzünün tek fetch
  yakalayıcısı bunu, gövdesinde `request_id` taşımayan, GET olmayan her `/api/`
  çağrısına ekler; kullanıcı eylemi başına tek kimlik, yeni tıklama yeni
  kimliktir. Başka bir kökene asla gönderilmez.
- *Koruma.* Kimlik doğrulama zincirinin içinde, yönlendiricinin önünde;
  `request_identities` tablosuna dayanır (göç 43). Satır; kimliği, işlemi yapan
  kullanıcıyı, yöntemi ve rota kalıbını, yöntem + yol + sorgu + gövdenin
  SHA-256'sını (gövdenin kendisini asla), `running`, `done` ya da `interrupted`
  durumunu, yanıtın durum kodunu ve saklanıyorsa içerik türüyle gövdesini,
  oluşturulma ve bitiş zamanlarını tutar. Satırlar 24 saat yaşar ve saatlik
  süpürmeyle silinir.
- *İlk geliş.* Satır `running` olarak yazılır ve işleyici, bağlantının iptal
  edemeyeceği, rotanın kendi süre sınırıyla sınırlı bir bağlamda çalışır.
  Yanıtı önce saklanır, sonra gönderilir. Satırı yazılamayan işleyici hiç
  başlamaz.
- *Yineleme.* Aynı kullanıcı ve aynı özetle gelen aynı kimlik, satırdan baytı
  baytına yanıtlanır. İlk geliş hâlâ sürüyorsa yineleme onu en çok 20 saniye
  bekler; sonra aynı kimlikle `409 REQUEST_IN_PROGRESS` yanıtını alır.
- *Yeniden kullanım.* Aynı kimlik başka gövde, yol, sorgu ya da kullanıcıyla
  gelirse `409 REQUEST_ID_REUSED`; hiçbir şey çalışmaz ve ilk yanıt
  gösterilmez.
- *Yarıda kalan.* Panel açılırken `running` bulunan ya da işleyicisi
  beklenmedik biçimde duran satır `interrupted` olur. Yinelemesi, sonraki
  adımla birlikte (mevcut durumu kontrol edin) `409 REQUEST_OUTCOME_UNKNOWN`
  alır; istek o kimlikle bir daha asla çalıştırılmaz.
- *Gizli bilgiler.* Tek seferlik gizli bilgi taşıyan yanıt ve 64 KiB'tan büyük
  her yanıt saklanmaz: yalnızca durum kodu saklanır. Yinelemesi `409
  REQUEST_COMPLETED_RESULT_NOT_RETAINED` alır (değişiklik yapıldı; tek seferlik
  sonucu yalnızca bir kez gösterildi); ilk deneme hatayla bittiyse aynı kod
  `failed` gerekçesiyle gelir.
- *Sarılmayanlar.* Gövdesinde `request_id` taşıyan ve işlem satırı tutan
  rotalar; bu grupta, ayar sürümü taşıyan rotalar.

**Birinci grubun kapsamı.** Zararlı rotalardan tam sekizi; her biri doğrulanmış
sonucuyla:

| Rota (POST) | İki kez çalışınca ya da sıfırlamayla kesilince | Süre sınırı |
|---|---|---|
| `domains/{id}/backups/restore` | Agent'ta kilit yoktu: iki ya da üç geri yükleme aynı belge köküne açılıyor ve aynı veritabanını yan yana içe aktarıyordu | 40 dk |
| `import/cpanel/apply` | ilk içe aktarım iki adım arasında, site yarı aktarılmış hâlde duruyordu (alan adı oluşturulmuş; dosyalar, posta, DNS ya da veritabanları değil); yineleme de "alan adı zaten var" diye reddediliyordu | 2 sa |
| yeniden düzenlemeli `domains/{id}/ssl/letsencrypt` | her geliş, sertifika otoritesinin yinelenen sertifika payından bir düzenleme daha zorluyordu | 25 dk |
| `domains/{id}/backups` | iş anahtarı gönderilmiyordu; Agent iş kilidi almıyor ve her geliş için bir arşiv üretiyordu | 35 dk |
| `database-servers/{id}/admin-account` | her gelişte yeni parola; iç içe geçen iki geliş ya da motorla kayıt arasındaki bir sıfırlama, motoru bir parolada, Panel'i ötekinde bırakıyordu | 12 dk |
| `vpn/peers` | her gelişte yeni anahtarlar ve yeni adres | 10 dk |
| `database-servers/{id}/databases` | MariaDB ikinci `CREATE DATABASE IF NOT EXISTS` komutunu kabul eder; yineleme sonra mevcut kayıtta başarısız oluyor ve telafisi, ilk isteğin oluşturup kaydettiği veritabanını siliyordu | 15 dk |
| `domains/{id}/databases` | Agent'ın oluşturması ile Panel'in kaydı arasındaki bir sıfırlama, motorda Panel'in listelemediği bir veritabanı bırakıyordu | 15 dk |

Korumanın yanında: Agent, bir alan adının geri yüklemesi sürerken ikincisini
hiçbir şey okumadan ve yazmadan reddeder (`409 BACKUP_RESTORE_IN_PROGRESS`);
böylece iki geri yükleme başlık olmadan da iç içe geçemez. Elle alınan yedek,
istek kimliğini Agent'ın iş anahtarı olarak taşır. Bir veritabanı sunucusunun
kendi hesabındaki değişiklikler sırayla çalışır. Yönetici hesabı ve VPN eşi
yanıtları hiç saklanmaz; veritabanı rotasının yanıtı, o isteğin ürettiği bir
parola taşıyorsa saklanmaz.

Bu sekiz rotada başlıksız istek, işleyiciden önce `428 REQUEST_ID_REQUIRED` ile
reddedilir: güncellemeden önce açılmış bir sayfa bunları korumasız
çalıştıramamalıdır. Bu sekizinde web arayüzü kaybolan yanıtı kısa bir
beklemeden sonra aynı kimlikle bir kez daha ister ve o yanıtı kullanır. Panel'in
kendisinin gönderdiği ret, durum kodu ne olursa olsun, kaybolan yanıt değildir.
İkinci soru da yanıt getirmezse ekran sonucun bilinmediğini ve yanıtın yeniden
sorulduğunu söyler, durumu yeniden okur ve o okuma yanıtlanana dek değiştiren
denetimlerini kapalı tutar. Diğer her rota, başlık olsun olmasın, eskisi gibi
davranır.

**Bilerek henüz kapsanmayanlar.** Dökümün bulduğu diğer zararlı rotalar (hizmet
ve uygulama yeniden başlatma, planlar, kayıtlar); durumu doğru bırakıp yanlış
bildiren yaklaşık 38 rota; dökümün sınıflandıramadığı 7 rota; sürüm belirteci
taşıyan rotalar. Koruma geneldir; bunların her biri, kötü sonucun kendi
doğrulamasıyla sonraki bir gruptur.

**Sonuçlar.**

- Güncelleme sırasında açık olan sayfa, yeniden yüklenene dek sekiz rotada
  reddedilir. Sürüm notları bunu söyler. Web arayüzü olmayan bir istemci
  başlığı kendisi gönderir.
- Sayfadan ayrılmak ya da sayfayı kapatmak bu sekiz değişikliği artık iptal
  etmez: kabul edilen bir geri yükleme, içe aktarım, sertifika düzenleme, yedek
  ya da veritabanı değişikliği sonuna ya da süre sınırına kadar çalışır.
- Tek seferlik gizli bilgiler saklanmaz; bu yüzden gizli bilgi taşıyan kayıp
  bir yanıt yeniden gösterilemez: yapılandırması ulaşmayan VPN eşi mevcut
  teslim alındısıyla iptal edilir ve yeni bir eş oluşturulur; üretilmiş
  veritabanı parolası yeniden belirlenir. Ekranlar bunu yerinde söyler. Panel'de
  bir veritabanı kullanıcısının parolasını belirleyen bir denetim henüz yoktur;
  o yüzden bu, motorda yapılır.
- Tablo sahibin verisi değildir. Eski bir Panel, 43. girdiyi taşıyan defterde
  açılmayı reddeder; geri dönüş, önceki her göçte olduğu gibi güncelleme
  öncesi anlık görüntünün geri yüklenmesidir ve tablonun geri yüklemede
  kaybolması zararsızdır: unutulmuş bir kimliğin yinelemesi o zaman ilk
  geliştir.
- Elle alınan yedek artık, zamanlanmış yedeğin hep yaptığı gibi, bu işin daha
  önce yayımladığı arşivi bulmak için alan adının mevcut arşivlerinin
  bildirimlerini okur.

**Kanıt durumu.** Korumanın, sekiz rotanın her birinin ürünün yönlendiricisi
üzerinden sahte bir Agent ile (art arda üç ve aynı anda üç gönderim: tek etki,
aynı yanıt), bağlantıdan ayrılmış bağlamın, Agent'ın geri yükleme kilidinin ve
yedek iş anahtarının bileşen testleri; başlığın, ikinci sormanın ve metinlerin
web testleri. Korumanın sözleşmesini tutan yerel bir sahte sunucuya karşı
gerçek bir Chrome: bağlantı gerçekten sıfırlandığında tek tıklama tek kimlikle
en çok 10 kez ulaştı ve her değişiklik bir kez yapıldı (dayanıklılık
sözleşmesi, 2026-10-10 kaydı). Henüz ölçülmedi: gerçek bir Panel'e karşı isteği
yeniden gönderen gerçek bir tarayıcı, sekizden birinin ortasında Panel'in
yeniden başlaması ve gerçek hizmetler. Kurulu sunuculara dokunulmadı; karar onların güncellenmesine
yetki vermez.

---

## D-028 · Boş çift PowerDNS birincili, ölçülen kapsam içinde sunuluyor

*1 Ekim 2026 · Sahibin yürürlükteki yönü üzerine planlayıcı kararı (D-026 eki: 6. satırın gerçek sistem kanıtı gelince kapıyı aç)*

Bugüne kadar Panel ve Agent, DNS motoru olmayan bir sunucuya PowerDNS'in
çift birincili olarak ilk kurulumunu reddediyordu
(`pdns_primary_switch_paused`); çünkü V3 günlüğünün gerçek sistem kurtarma
kanıtı yoktu. 30 Eylül 2026'dan beri her kabul koşusu iki kapı sabitini
çeviren bir dalda yapıldı; ana hat kapalı kaldı.

**Kanıt.** Kesinti matrisinin 8r, 9 ve 12. grupları (ilk başlatmadan önce ve
sonra kesintiler, tek bölgeyle ve sıfır bölgeyle, ileri kurtarma, başlangıç
öncesi geri alma, tutulan sahip düzenlemeleri, sahip kaydından sonra
sürdürülen ebeveynsiz silme, yönetim kapalıyken yeniden açılışlar) ve 5. ile
7. çift koşuları (iki CelikPanel sunucusunda ürün akışının tamamı: kurulum,
bölge ekleme, kayıt düzenleme, sahip kaydından sonra ürünün kanıtıyla bölge
silme, yeniden ekleme, panel kapalıyken yeniden açılış, yönetimin dönüşü)
kabul dalında düzenek geçici çözümü olmadan geçti. Aynı koşular yol boyunca
on bir ürün kusuru bulup kapattı; her biri kanıtıyla kabul kütüğünde.

**Karar.** İki kapı sabiti ana hatta `true`. Sunulan, tam olarak ölçülen
şeydir: DNS motoru olmayan bir sunucuda çift birincil olarak PowerDNS,
Debian 13 amd64, ölçülmüş PowerDNS paket sürümü; Agent'ın sunucu profili ve
paket sürümü ön denetimleriyle zorlanır. Diğer her PowerDNS çift birincil
bildirimi reddedilmeye devam eder; hizmet veren BIND kaynağı D-026 reddini
(`bind_source_pdns_switch_unsupported`) korur; BIND için ve PowerDNS ikincili
için hiçbir şey değişmez.

**Belirtilen sınırlar.** Hücre ya da topoloji başına tek çalıştırma, geçici
konuklar, düzenli yeniden açılışlar, yalnızca test amaçlı lisans derlemesi
(D-027), dizüstü bir ana makine. Daemon yeniden damgalama kabulü, kanıt süre
aşımı kodu ve bileşik sahip-düzenleme kodu kaynakta bileşen testleriyle var
ve gerçek sistemde ortaya çıkmadı. Yerel kanıtın adım süresi sınırları tek
örneklerdir. Kurulu sunuculara dokunulmadı; karar onların güncellenmesine
yetki vermez (her panel güncellemesini sahibi panelin kendi güncelleme
ekranından başlatır).

**Ek, 29 Ekim 2026 (Beta-1 sanal makine koşusundan sonra sahip kararı).**
Kapı, sunulanın ölçülen olması için tam ölçülen yapıyı (4.9.17-0+deb13u1 ve
daemon özeti) sabitliyordu. Temiz bir Debian 13'te dağıtımın adayı bir
güvenlik yama sürümü olan 4.9.18-0+deb13u1'e geçmişti: sihirbaz PowerDNS'i
ana DNS sunucusu olarak sundu, DNS adımı reddedip geri aldı. Kapı artık yamayı
değil, **Debian'ın kendi arşiv yapısının** denenmiş **serisini** (major.minor,
4.9) tutar. Sihirbazın sunduğunu, incelemenin kabul ettiğini, Agent ön
denetiminin kurduğunu (tek aday için `ad=sürüm`), kurulum sonrası denetimin
ve bağımsız kurtarma doğrulamasının kabul ettiğini tek ortak kural belirler:
epoch'suz ve Debian arşiv revizyonlu bir sürüm (backport, repo.powerdns.com
ya da üretici yapısı her yerde sürümünden reddedilir), iki paket tek bir 4.9
sürümünde. Kaynak kanıtı arşiv denetimidir: kurulumdan önce APT'nin aday için
gösterdiği her arşiv Debian'ın yayımlanmış sürümü ya da onun güvenlik/
güncelleme cebi olmalıdır; okunamayan dizin reddedilir. Kurulumdan sonra kaynak
artık okunamaz; kurulum sonrası denetim ve kurtarma sürüm kuralına dayanır.
Daemon, dpkg'nin kurulu paket için kaydettiği özetle (md5sums) karşılaştırılır:
bu, diskte değişikliği ya da bozulmayı bulan bir bütünlük denetimidir, kaynak
kanıtı değildir. İki denetim de sunucudaki root'u (dosyaları ve kayıtları
birlikte değiştirebilir) ya da geçerli imzalı arşiv sunan ele geçirilmiş bir
yansıyı kapsamaz. Kural dışındaki aday gerekçesiyle sunulmaz ve incelemede
reddedilir. Sunucu profili, topoloji ve yukarıdaki diğer sınırlar değişmez;
yeni major/minor sürüm yeni ölçüm ister. Seri içi yama sürümleri yeni gerçek
koşu olmadan kabul edilir; davranışı değiştiren bir yama sürümü bilinen
sınırdır.

**Ek, 30 Ekim 2026 (sahip kararı: "Arch'ta PowerDNS kullanacağım").**
Denenmiş seri artık **paket ailesine göre** tutulur. Debian ve Ubuntu için
yukarıdaki her şey aynen kalır (Debian arşivinin 4.9 yapısı, dpkg md5sums).
Arch Linux x86_64 kendi ölçülen yapısını alır: yalnız Arch'ın resmi `extra`
deposundan `powerdns` 5.1 (AUR ya da üçüncü taraf depo asla), daemon
`/usr/bin/pdns_server`, kullanıcı ve grup `powerdns`, `pdns.conf` 0644
root:root. Sihirbazın sunduğunu, incelemenin kabul ettiğini, Agent'ın
kurduğunu, kurulum sonrası denetimin ve bağımsız kurtarma doğrulamasının
kabul ettiğini yine tek kural belirler: epoch'suz, yama ve pkgrel'i sayı olan,
5.1 serisinde bir pacman sürümü. Kaynak kanıtı, pacman'ın aday için gösterdiği
depodur (`extra`): her etkiden önce mevcut veritabanından, tek `pacman -Su`
işleminden hemen önce de paket kilidi altında tazelenmiş veritabanından
yeniden okunur; başka seri ya da başka depo ise hiçbir şey kurulmaz. Dosya
kimliği pacman'ın kendi yerel veritabanından gelir: daemon ve hizmet birimi,
kurulu paketin kaydettiği SHA-256 ile (mtree) aynı olmalıdır. Böylece 5.1
içindeki olağan bir `pacman -Syu` dosyayı ve kaydı birlikte değiştirir ve
Debian yama sürümü gibi kabul edilir; elle değişmiş dosya kabul edilmez.
Yukarıdaki sınırlar aynen geçerlidir (sunucudaki root, ele geçirilmiş yansı).
Arch kurulumunun günlükleri ve durum belgeleri kendi işaretini taşır
(`pdns-fresh-paired-primary/arch-5.1/v1`); bir durum kendi günlüğünün
işaretini taşımak zorundadır ve bu sürümden eski bir okuyucu Arch belgesini
yanlış okumak yerine reddeder. Deneme sanal makinesinde ölçüldü: 5.1.5
üreticinin açılıştaki SQL değişikliği ve 5.1.5 tüketicinin üye satırları
4.9'unkiyle aynıdır.

Arch'ta PowerDNS yalnız **DNS yazılımı olmayan bir sunucuya ilk kurulum**
olarak kurulur: kendi DNS'ini tutan tek sunucu, eşli ana ya da eşli yedek
sunucu (`pdns_arch_setup_only` ve Agent başka her şeyi her etkiden önce
reddeder: var olan DNS sunucusunu PowerDNS'e çevirmek, yeniden kurmak, yeniden
ayarlamak, sahibin kurduğu PowerDNS'i devralmak). Arch sürekli güncellenen bir
dağıtımdır ve `pacman -Syu`'yu sahip çalıştırır (CelikPanel Arch'ı kendiliğinden
yükseltmez). Aday 5.1'in dışına çıkınca sihirbaz PowerDNS'i ana DNS sunucusu
olarak gerekçesiyle sunmaz. Kurulu PowerDNS sahibin güncellemesiyle denenmiş
serinin dışına çıkarsa PowerDNS kendi yapılandırmasıyla yanıt vermeye devam
eder (D-022); CelikPanel ona DNS değişikliği yayımlamayı bekletir (değişiklik
her etkiden önce gerekçesiyle reddedilir), DNS ekranlarında
`engine_series_untested` ile kimin ne yapacağını söyler (D-024) ve kurulu
sürüm yeniden denenmiş bir seride olunca (örneğin onu adlandıran bir CelikPanel
sürümünden sonra) kendiliğinden sürer. D-026'nın ilk kurulum kurtarma sözleşmesi
değişmez. Kanıt:
[arch-powerdns-20261030](../deploy/e2e/dns-pair-acceptance/evidence/arch-powerdns-20261030/).

---

## D-027 · Kabul koşuları yalnız deneyde geçerli lisans kullanır; lisans politikası değişmez

*30 Eylül 2026 · Kullanıcı kararı*

Geçici deney sunucularında ürün akışının kabulü gerçek Panel'i kullanmak
zorunda: kurulum sihirbazı, alan adı ekranları, iki sunucuda da CelikPanel.
Panel, lisans etkin değilken neredeyse hiçbir işleme izin vermiyor ve etkin
lisansı yaklaşık dakikada bir lisans hizmetinden doğruluyor; deney sunucuları
ise internete kapalı. Kullanıcı, deneylerde gerçek anahtar kullanmak ya da
ürün akışını atlamak yerine yalnız deneyde geçerli bir derlemeyi seçti.

**Karar.** `acceptance_license` derleme etiketi, yalnız "ACCEPTANCE FIXTURE —
NOT FOR PRODUCTION" etiketli tek bir deney lisansını kabul eden bir dikiş
derler. Bu lisans yalnız deney düzeneğinin işaretini taşıyan sunucuda
etkinleşir, yerelde doğrulanır; lisans hizmetine her bağlantı denemesi
reddedilir ve sayılır. Sıradan derlemede deney lisansı, başka bir doğrulayıcı
ya da ortam/dosya okuması bulunmayan boş bir gövde derlenir; sıradan bir
ikili çalışma anında kabul kipine geçirilemez. Bu, kaynaklardan ve derlenmiş
ikililerden testlerle kanıtlanır. Sürüm paketleme, etiketle derlenmiş ya da
deney metnini içeren her dosyayı reddeder. Kabul arşivi yalnız adı açıkça
belirtilmiş bir betikle üretilir ve sürüm değildir.

Bu karar lisans politikasını, doğrulama süresini ya da müşteri derlemesinin
herhangi bir davranışını değiştirmez. Deney lisansıyla yapılan koşu lisans
davranışı için kanıt sayılmaz. Bilinen sınır: Lisans ekranı yalnız durum ve
bitiş tarihini gösterdiği için deney lisansı ekranda ayırt edilemez; API
yanıtı, günlükler, kanıt ve panel sürüm adı ayırt eder.

---

## D-026 · DNS motoru kurtarması: kurtarılamayan geçişi reddet, ilk kurulumda aynı işlemle kurtarmayı kabul et

*29 Eylül 2026 · Devirdeki 1. madde (DNS kurtarma sözleşmesi) için kullanıcı kararı*

`e9d1019d` kaynağının salt-okur denetimi, ürünün başlatabildiği her DNS motoru
değişikliğini üç durumdan birine bağladı: gerçek sistem kanıtıyla geçti, adı
belli eksik, desteklenmiyor. Bir yol kabul edilemez dördüncü duruma düştü:
**çalışan BIND** kaynağını tek sunucuda veya çiftin ikincilinde **PowerDNS**
hedefine çevirmek motor kartından hâlâ başlatılabiliyordu. Bu yol eski V1
günlüğünü yazıyor, tek kurtarması Agent'ın kendi ters işlemi, V4 başlangıç
öncesi ters işlem ve sahip komutu üreticiye bağlı değil ve gerçek sistemde
kesinti denemesi yok. Mevcut kapı yalnız çiftli birinciyi kapatıyordu.

**Karar 1.** BIND→PowerDNS motor geçişi bu sürümde her topolojide
desteklenmiyor. Panel önizleme/onay, sunucu kurulumu ve Agent RPC'si bunu
değişiklik başlamadan `bind_source_pdns_switch_unsupported` engeliyle reddeder;
ekran nedeni, BIND'in hizmete devam ettiğini ve sonraki adımı (PowerDNS'i DNS
motoru olmayan bir sunucuya kurmak ya da geçişi destekleyen sürümü beklemek)
söyler. Boş sunucuya PowerDNS kurulumu, PowerDNS→BIND geçişi, PowerDNS
devralma/yeniden yapılandırma, BIND yeniden kurulumu ve kurulu sunucular
değişmez. Kapıyı yeniden açmak için üreticinin bağlanması, başlangıç öncesi ve
sonrası Agent'tan bağımsız ters işlem ve gerçek kesinti kanıtı gerekir;
denetimi kaldırmak çözüm değildir.

**Karar 2.** İlk motor kurulumunda (BIND veya PowerDNS, her rol) önceki durum
"DNS motoru yok"tur; geri yüklenecek sahip verisi yoktur. Sonraki açılışta
Agent'ın aynı işlem kimliğiyle kurtarması, salt-okur durum komutu ve ekranın
sahibe sonraki adımı söylemesi bu sürümün kabul edilen sözleşmesidir. İlk
kurulumlar için Agent'tan bağımsız sahip komutu şart değildir. 1. madde için
kalan iş kod değil kanıttır: boş tek sunucu PowerDNS, boş çift ikincil (BIND ve
PowerDNS) ve boş BIND'de başlangıç sonrası kesinti hücreleri.

**Ek, aynı gün.** Gerçek sistem hücreleri, bileşen testlerinin bulamadığı iki
ürün kusurunu (ilk kurulum geri alması ve V2 BIND geçişi ters işlemi, hiç
başlamamış hedefi reddediyordu) ve bir kurtarma çıkmazını (sahip komutları
Agent'ın bilerek bıraktığı işi reddediyordu) ortaya çıkardı. Üçü de düzeltildi
ve gerçek sistemde yeniden koşuldu. Sahip, kapatmadan önce kaynak durdurulduktan
sonraki PowerDNS→BIND hücrelerinin koşulmasını seçti; geçtiler ve yol
haritasının 1. maddesi [kabul kütüğünde](DNS-RECOVERY-ACCEPTANCE.tr.md) adı
belli sınırlarla kapandı. Bırakılmış bir işte sahip komutu bitmiş defter
kaydını yeniden yazmaz; sahibe gösterilen metin okuma anında hesaplanır.
1. maddenin kapanması P0.4'ü kapatmaz.

Karar 1 vaat edilen kapsamı daraltır ve sürüm notlarında açıkça desteklenmeyen
işlem olarak yazılır; sessizce yapılmaz. İki karar da kurulu sunucuyu, lisans
politikasını, sahibin kendi kurtarmasını ve panel güncellemesini yalnız
kullanıcının başlatması kuralını değiştirmez.

---

## D-025 · Dayanıklılık, olay yaması değil temel çalışma sözleşmesidir

*14 Eylül 2026 · Kullanıcının istediği anayasa incelemesi; bağlayıcı yön, uygulama açık*

Kullanıcı, Frankfurt'ta tekrar eden hatalardan sonra sistemin anayasasının
incelenmesini istedi. Kaynak incelemesi; birbirine bağlı arıza sınırları, anlamı
karışmış kanıt rolleri, uyumsuz çıktı okuyucuları ve kurtaracağı aday sürüme
bağımlı kurtarma mekanizması buldu. Benzer hatalar 26 Ağustos'ta da kaydedilmişti.
Tek hatayı yeniden üreten testin geçmesi bütün yaşam döngüsünün güvenilirliğini
kanıtlamamıştı.

**Yön.** Panel/Agent yetki ayrımı korunur; normal değişiklik yetkisi, durum
gözlemi ve kurtarma erişimi ayrılır. Güvensiz eylem etkilenen sınırda durdurulur;
çalışan yerel hizmetler ve desteklenen sunucu arıza modeli içinde dar, kimlik
doğrulamalı kurtarma yolu korunur. Bilinmeyen durum; lisans süresi doldu, hizmet
yok veya iş tamamlandı şeklinde yanlış bir teşhise dönüştürülemez.

Her kalıcı çıktının sürümlü tek bir üretim/okuma/geri yükleme sözleşmesi gerekir.
Metadata normalleştirmesi dahil her değişiklik; salt-okur keşif, açık kalıcı
kontrol noktaları, sınırlı devam/telafi ve sonuç kanıtı ister. Kurtarma, normal
Agent ve aday uygulama başlayamadığında da çalışabilmelidir. Ayrı sürümlenen
asgari yürütücü ve manifest protokolü önerilir; yerine geçecek mekanizma uygulanıp
doğrulanana kadar mevcut tam eşleşen saklı sürüm geri alma kuralları sürer.
Sınırsız kabuk veya yapay zekâyla kanıt düzeltme yolu açılmaz.

[Dayanıklılık sözleşmesi](RESILIENCE-CONTRACT.tr.md); kaynak kanıtını, ilkeleri,
açık P0 kabul kaydını ve uygulama sırasını içerir. [Anayasa](../ROADMAP.tr.md)
artık sürekliliği, gerçeği yansıtan durumu ve arıza kanıtını zorunlu tutar. PR
incelemesi etkilenen sözleşmeleri, taşıma/kurtarma davranışını ve tam test kanıtını
adlandırmalıdır. Tam destek; kurtarmanın kendisindeki arızalar dahil, geçici gerçek
sistem ortamında güncelleme ve otomatik geri yükleme denemeleri ister. Acil olay
düzeltmesi kapsam ve sınırları belirtilerek yayımlanabilir; mimari işi kapatmaz
ve ilgisiz özellik genişlemesini haklı çıkarmaz.

Bu karar kurulu sunucuyu değiştirmez; yeni yürütücüyü, kurtarma arayüzünü, şema
ayrımını veya bağımsız hizmet yenilemesini uygulamaz. Alpha80 dar kapsamlı BIND ve
kurtarma düzeltmesidir. D-021, D-022, D-024, kullanıcının yürüttüğü kurtarma ve
kurulu paneli yalnız kullanıcının güncellemesi kuralları geçerlidir. Eski tek
binary, yalnız-panel ve tarihsel bootstrap ifadeleri bu sınırları geçersiz kılamaz.

---

## D-024 · Her işlem mevcut durumu ve sonraki eylemi açıklar

*13 Eylül 2026 · Kullanıcının onayladığı gereksinim; ürün genelindeki uygulama ve inceleme tamamlanmadı*

İlerleme, bekleme, hata ve kurtarma yönlendirmesi; yönetilen her program, hizmet,
çalışma ortamı ve entegrasyon için geçerlidir. Kullanıcı uzun adım listesinden
önce mevcut nedeni, kimin işlem yapacağını, somut sonraki eylemi ve işlemin nasıl
devam edeceğini görmelidir. DNS topolojisi, lisans, kimlik bilgileri, yetkiler ve
sağlayıcı erişimi sessiz bekleme yerine özel yönlendirme gerektiren örneklerdir.

Doğrulanmış hata, eksik önkoşul ve bilinmeyen sonuç ayrılır. Sonuç kontrol
edilirken bilinen hata korunur; kabul edilen plan ve tam işlem kimliği korunur;
gizli bilgi gösterilmez. Sayfa yenilendi veya yanıt kayboldu diye ikinci değişiklik
işlemi başlatılmaz. Desteklenen her adaptör kendi durumları için sabit durum/hata
kodları ve uygulanabilir çevrilmiş mesajlar sağlamalıdır.

Mevcut kaynak çalışması kurulum DNS rol bağlamını, panel lisansı yönlendirmesini
ve genel hizmet hatası gösterimini sağlar. Bu karar üçüncü taraf lisans
adaptörlerini veya bütün işlem ekranlarının eksiksiz incelemesini uygulamaz.
[İşlem yönlendirme sözleşmesi](OPERATION-GUIDANCE.tr.md) kabul ölçütlerini ve
sınırları tanımlar. D-021 ve D-022 geçerlidir; kurulu panelleri yalnız kullanıcı
günceller.

---

## D-022 · CelikPanel'den bağımsız, sahibinin yönetebildiği altyapı

*12 Eylül 2026 · Kullanıcının onayladığı gereksinim; ilk inceleme kaydedildi; kaldırma desteği henüz doğrulanmadı*

CelikPanel sunucu sahibinin talimatlarını yerine getiren bir çalışandır.
Sunucunun sahibi panel olmadan da hizmetleri yapılandırıp yönetebilmeli, paneli
kaldırdığında işleri devam etmelidir. Hizmetlerin çalışması panelin erişilebilir
olmasına, lisansına veya yönetim ajanının kurulu kalmasına bağlanamaz.
DNS, web, e-posta, veritabanları, zamanlanmış işler ve sertifika yenilemeleri
standart protokoller ve hizmetlerin kendi yapılandırmalarıyla bağımsız çalışmalıdır.

Yönetim yazılımının kaldırılması hizmet paketlerini, verileri, kimlikleri,
yapılandırmaları ve gerekli yenileme/zamanlama mekanizmalarını korumalıdır.
Mevcut bağımlılıklar incelenip güvenle ayrıştırılmadan bu kaldırma davranışının
desteklendiği söylenemez. Sahibinin doğrudan yaptığı değişiklikler algılanmalı;
üzerlerine sessizce yazmak yerine çakışmalar açıkça çözümlenmelidir.

Standart birincil/ikincil DNS aktarımı, karşı tarafta CelikPanel veya panel HTTPS
adresi gerektirmeden panelden yapılandırılabilmelidir. Uzaktan DNS kaydı oluşturma
ve değiştirme ayrıca yetkilendirilen, isteğe bağlı bir otomasyondur. Mevcut merkezi
kayıt yönetimi bağlantısı DNS zorunluluğu değil, uygulama tercihidir. Mimari
geliştirilirken mevcut kurulumlar korunmalıdır.

Bu yön kararı asistana canlı sunucuyu değiştirme, paneli kaldırma veya kurulu paneli
güncelleme izni vermez. Güncellemeyi kullanıcı panelden kendisi başlatır.

[Kaynak uygulaması ve kalan bağımlılıklar](OWNER-INDEPENDENCE.tr.md).

*Değişiklik (26 Ekim 2026, alpha.110 adayı):* web postası kendi adresinde, sunucunun
kendi nginx'i ve PHP-FPM'iyle çalışır; sertifikasını işletim sisteminin certbot
zamanlayıcısı yeniler; panel dururken de yanıt verir (D-038, 26 Ekim 2026
değişikliği).

---

## D-021 · Amaca göre, kaldığı yerden süren kurulum ve açık DNS sahipliği

*10 Eylül 2026 · Onaylanan yön; [çalışma ağacındaki uygulama durumu](SERVER-SETUP-STATUS.tr.md)*

**Karar.** Yeni olduğu doğrulanmış sunucu, lisans aktivasyonundan sonra yöneticiye
özel kurulum rehberine girer. Yarım kurulum devam eder; hazırlanmış sunucu
Dashboard'a döner. Lisans, kurulum geçmişi ve güncel sağlık bağımsızdır.
Önerilen varsayılan web hosting'dir; posta kendiliğinden kurulmaz, açıkça seçilir.
Diğer amaçlar web ve posta hosting, uygulama sunucusu ve DNS altyapısıdır.

Hosting için yerel DNS, yetkilendirilmiş mevcut CelikPanel DNS altyapısı veya
kayıtları elle doğrulanabilen, API otomasyonu isteğe bağlı harici DNS seçilir.
Bu, D-009'un her durumda yerel DNS motoru gerektiren kuralının yerini alır.
Yerel DNS sahipliği, topoloji ve kurtarma kurallarını gevşetmez; yalnız mevcut
engeller silinerek uygulanamaz. Açık ve desteklenen bir taşıma yapılana kadar
mevcut DNS sahipliği korunur.

İlk genel kurulum panel FQDN'si, güvenilir ve yenilenebilir HTTPS ve yönetim
erişimini koruyan güvenlik duvarı gerektirir. Diğer hazırlık koşulları ilgili
özelliğe uygulanır. Kullanıcı somut, kalıcı planı inceleyip başlatır; salt-okur
keşif yapılandırmayı değiştirmez. Mevcut işler korunur. Kurulu panel güncellemelerini
kullanıcı panelin içinden başlatır.

**Kapsam ve kanıt.** Onaylanan sıra, mevcut uygulama eksikleri, kabul durumları ve
uygulama sırası [Yönlendirmeli sunucu kurulumu](SERVER-SETUP-PLAN.tr.md) belgesindedir.
Bu karar ürün niyetini kaydeder; yeni sihirbazın veya harici DNS modunun bugün
kullanılabildiği anlamına gelmez.

*16 Ekim 2026 tarihli not (saat tarihi): v0.1.0-alpha.87 adayında (yayımlanmadı)
uygulandı.* O aday için yeni bir karar kaydedilmez; yukarıdaki metin değişmedi.
Tamamlanmış bir kurulum artık genişletilebilir (`POST /api/v1/setup/extend`):
sihirbaz tamamlanmış değerlerle yeniden açılır, inceleme neyin değiştiğini,
neyin kurulacağını ve neyin kaldığını söyler ve çalıştırma var olan incelenmiş
çalıştırıcıdır. Bu, kararın kendi kurallarını uygular:

- **Mevcut işler korunur.** Bir genişletme yalnız ekleneni kurar ve
  yapılandırır; kurulu bir bileşeni asla kaldırmaz, durdurmaz ya da
  değiştirmez (bir seçimi temizlemenin kurulu bir hizmeti asla kaldırmadığı
  özelleştirme kuralı genişletme için de geçerlidir) ve "Genişletmeden vazgeç",
  kısmen çalışmış bir genişletmenin kurduğu hiçbir şeyi kaldırmadan tamamlanmış
  taslağı geri getirir.
- **Kurulum geçmişi ve güncel sağlık bağımsız kalır.** Bir genişletme tamamlanma
  kaydını (`completed_at`) korur ve zorunlu değildir; böylece hazırlanmış bir
  sunucu asla kuruluma geri yönlendirilmez; Dashboard sunucunun genişletildiğini
  ve çalışmaya devam ettiğini söyler.
- **Posta açıkça seçilir, kurulumun denetimleriyle.** Kurulumu bitmiş bir
  sunucuda Bileşenler sayfasından posta profili eklemek kurulumu yeniden
  kullanır: profil bir genişletmeye eklenir ve kurulumun incelemesinden ve
  adımlarından geçer (posta sunucusu adının genel DNS'i, güvenilir sertifika ve
  yenilenmesi, ters DNS, 25 numaralı bağlantı noktası). Kurulumu bitmemiş bir
  sunucu, geçici kendinden imzalı bir sertifikayla bittiğini söyleyen doğrudan
  kurulumu korur.
- **DNS sahipliği değişmez.** Bir DNS çiftinin ikincilinde panel, posta sunucusu
  adının kaydının birincilde ya da DNS sağlayıcısında yapıldığını söyler ve tam
  kaydı gösterir; hiçbir kayıt oluşturmaz. Birincilin bir eşin kayıtlarını
  kendisinin yayımlayıp yayımlamayacağı, altyapı bölgesi için bir düzenleyici
  ve ana makine kayıtlarının uzak DNS bağlantısıyla kendiliğinden aktarılması,
  sahibin açık üç kararıydı (İŞLEM YÖNLENDİRMESİ, 16 Ekim 2026 girdisi). Sahip
  üçüne de aynı gün karar verdi ("önerin gibi olacak"): birincilin planı eşin
  posta sunucusu adını taşıyabilir, altyapı bölgesi mevcut düzenleyiciyle
  Alan adlarında yalnız DNS girdisidir ve ikincil ad kayıtlarını yetkili
  bağlantı üzerinden birincilden ister, elle giriş de mümkün kalır; kopyalama
  yerel bölge aktarımı olarak kalır (D-022). Kaynakta
  `feat/alpha91-second-server-part2` dalında; sahibin talimatıyla gerçek
  sistemde ölçülmedi (İŞLEM YÖNLENDİRMESİ, ikinci bölüm).

Notun durumu: kaynakta, bileşen testleri ve taklit tarayıcıyla (16. tur);
`deploy/e2e/release-recovery/evidence/` altındaki set18 (`5d54aba1e`'de;
genişletme tamamlanmış çalışmayla açıldı, D1) ve set18b (`6b363ac46`'da; D1'in
düzeldiği gözlendi, genişletme hazır duruma ulaştı) gerçek sistem kayıtlarında,
kendi DNS'i ve ACME'si olan bir laboratuvarda ölçüldü; yalnız Debian 13; kurulu
bir sunucuda hiçbir şey ölçülmedi. Hangi kontrolün hangi koda dayandığını
[alpha.87 sürüm notları](RELEASE-NOTES-v0.1.0-alpha.87.tr.md) söyler. Hiçbir P0
işi kapanmaz ya da ilerlemez.

### Değişiklik (2026-10-23): yöneticinin e-postası zorunlu kalır ve yönetici oluşturulurken denetlenir

*Sahibin 2026-10-23 kararları; v0.1.0-alpha.102 adayının kaynağında (yayımlanmadı);
kurulu bir sunucuda hiçbir şey ölçülmedi*

Neden. alpha.99'dan beri açık bir iş, kurulum planının iletişim e-postası
engelinin her plana uygulandığını söylüyordu. Sahip karar verdi: "iletişim
epostası zorunlu olarak gerekiyor ise zorunlu olmalı. kurulumda eposta
belirtiyoruz". Gerçekten gerekiyor: ilk genel kurulum panelin adı için güvenilir
ve yenilenebilir HTTPS ister (yukarıda), bu yüzden ilk kurulum her zaman panelin
sertifikasını ister ve sertifika yetkilisi yöneticinin adresini iletişim adresi
olarak alır. Sertifika adımı olmayan tek plan zaten geçerli sertifikası olan bir
sunucunun planıdır; o durumda adres zaten kayıtlıdır. Engel bu yüzden her planda
kalır. Ardından sahip adresin ilk verildiği yerde denetlenmesini onayladı ("ok.
yap"); böylece yazım hatası Sunucu kurulumu ekranında sonradan değil, kurulumda
yakalanır.

Karar.

- **Tek kural.** Kurucu ilk yöneticinin e-postasını kurulum planının, panel
  sertifikası isteğinin ve kişinin kendi e-posta değişikliğinin zaten kullandığı
  doğrulayıcıyla denetler (`mutationpayload.CanonicalContactEmail`): yazdırılabilir
  ASCII, tam bir `@`, en çok 64 baytlık yerel bölüm, tam nitelikli bir alan adı
  (`localhost` değil, IP adresi değil, sonda nokta yok) ve toplam en çok 254 bayt.
  `ali`, `ali@`, `@x.com`, `a b@x.com` ve `ali@localhost` reddedilir.
- **Etkileşimli istem.** Reddedilen adreste kurucu "That is not a valid email
  address (example: you@example.com). The certificate authority needs it as a
  contact. Please try again. / Bu geçerli bir e-posta adresi değil (örnek:
  siz@ornek.com). Sertifika yetkilisi bunu iletişim adresi olarak ister. Lütfen
  yeniden deneyin." yazar ve yeniden sorar. Yazılan değer hiçbir zaman yazdırılmaz.
  İstem her zamanki yerinde, paketler kurulduktan sonra gelir; durmak yerine
  yeniden sorduğu için oradaki bir yazım hatası kurulumu durdurmaz.
- **Kimlik bilgisi dosyası.** `CELIKPANEL_ADMIN_CREDENTIALS_FILE` ile verilen dosya,
  önceden olduğu gibi hiçbir şey kurulmadan aday panelin yalnız-doğrulama kipiyle
  denetlenir; bu denetim artık aynı kuralı uygular. Kurucu ardından girdi dosyasına
  dokunulmadığını ve hiçbir şey kurulmadığını, neyin düzeltileceğini söyler:
  panelin kabul ettiği bir kullanıcı adı, siz@ornek.com gibi gerçek bir e-posta
  adresi (sertifika yetkilisi bunu iletişim adresi olarak ister) ve en az 8
  karakterlik bir parola. Panelin kendi reddi e-posta alanını adlandırır, hiçbir
  değeri yazmaz. İmzalı giriş betiğinin kendi biçim denetimi değişmez; yetki
  indirilen paneldedir.
- **Saklanan biçim.** Adres, kişinin kendi e-posta değişikliğinin sakladığı gibi
  doğrulayıcının kanonik biçiminde (alan adı küçük harfle) saklanır.
- **Mevcut yönetici yolu.** Yönetici oluşturma komutunun mevcut bir kullanıcı adı
  için yeniden çalıştırılması da aynı kurala bağlıdır; reddedilen adres ne adresi
  ne parolayı değiştirir.
- **Mevcut kurulumlar etkilenmez.** Göç yok, saklanan adresler yeniden
  denetlenmez. Eski geçersiz bir adresi yine Sunucu kurulumu engeli yakalar;
  engelin "Hesabım’ı aç" bağlantısı kişinin kendi e-posta değişikliğine götürür.
- **Başka hiçbir yol yönetici oluşturmaz.** Hesap API'si yönetici rollü hesap
  oluşturmayı ve yönetmeyi reddeder; ekip üyeleri müşteri rollü ek kullanıcılardır
  ve sunucu kurulumunu açamaz; kontrol düzlemi geri yüklemesi arşivdeki
  yöneticileri değiştirmeden getirir; demo kipinin hesapları yalnız demodur. Bu
  yüzden ekip üyesi adres denetimi değiştirilmez.

Değişmeyen. Sertifika yetkilisi genel olmayan bir alan adındaki adresi (`.test`,
`.invalid`, `example.com`) yine reddedebilir: panel biçimi denetler, alan adına
yetkili karar verir. Bu, D-037'nin alpha.99 değişikliğiyle kaydedilen "yönetici
oluşturma komutu adresi denetlemiyor" sınırını kapatır; alpha.102 kaynağında
denetliyor.

Yaklaşan kabul, iddia değil. Beta 1'de sahip temiz ilk kurulum denemesi için iki
sunucuyu da silip yeniden kuracak; bu denetim gerçek bir kurulumda ilk kez orada
görülecek. Kurulu paneller yine yalnız sahipleri tarafından panelin kendi
güncelleme ekranından güncellenir.


### Ek (2026-10-26): ana alan adı kurulumun bölgesidir ve sihirbaz bu sürümün yapamadığını söyler

*Sahibin onayladığı kurulum sihirbazı v2, 1. aşama; v0.1.0-alpha.111 adayının
kaynağında (yayımlanmadı); kurulu bir sunucuda ölçülmedi*

Sahip yeniden tasarlanan sihirbazı 2026-10-26'da onayladı ([Yönlendirmeli
sunucu kurulumu](SERVER-SETUP-PLAN.tr.md), o tarihli giriş). İki ürün kuralı
değişir:

- **Ana alan adı, altyapı bölgesi için açık niyettir.** Bir kez ve ilk sorulur.
  Bu sunucu yerel ana DNS sunucusuysa kurulum o bölgenin sunucu adresi
  kayıtlarını hazırlar (inceleme bunları gösterir); eski işaretsiz "bu kayıtları
  burada hazırla" seçeneği kalktı. Yedek DNS sunucusu ve harici ya da uzak DNS
  yeri bölge hazırlamaz. Kurulumun kiracı, site, posta kutusu, MX, SPF ya da
  apex kaydı oluşturmadığı kural değişmez.
- **Sihirbazın bileşen listesi katalogdan gelir.** Her katalog bileşeni,
  sihirbazın onunla ne yaptığıyla görünür; kurulumla kurulamayan, nedenini ve
  nerede kurulacağını söyler.

Sürüm sınırlarını da ekranda söyler: tek sunucu henüz kendi DNS'ini tutamaz ve
iki sunucuda ikinci sunucunun IP adresi hâlâ yazılır. Mevcut DNS sahipliği,
topoloji ve kurtarma kuralları değişmez.


### Ek (2026-10-28): kurulum, sahibin aktardığı kayıtları ve ana alan adının posta kayıtlarını yazar

*Sahibin onayladığı kurulum sihirbazı v2, 2. aşama; v0.1.0-alpha.112 adayının
kaynağında (yayımlanmadı); kurulu bir sunucuda ölçülmedi*

Kurulumun apex, `www`, MX, SPF ya da DMARC kaydı oluşturmadığı önceki kural
yalnız ana alan adı için ve yalnız sahibin sihirbazdaki açık seçimleriyle
değişir: sahibin aktardığı mevcut genel kayıtlar ana alan adının bölgesine
yazılır; e-posta seçiliyse MX, SPF, DMARC ve DKIM yazılır. MX sahip olmadan
değişmez (aktarılan eski MX, sahip "şimdi" diyene kadar kalır), SPF
birleştirilir, eski DMARC değiştirilmedikçe korunur. Bunlar sahibin
kayıtlarıdır (Alan adları'ndan düzenlenir); kurulumun kendi sunucu adresleri
kilitli kalır. Kurulum yine kiracı, site ya da posta kutusu oluşturmaz. Ters
DNS sunucu sağlayıcısında kalır: kurulum IPv4 ve varsa IPv6 için tam satırları
gösterir; yanlış ya da eksik PTR'yi beklemediği açık bir iş olarak tutar (sahibin
2026-10-28 kararı; posta onsuz da çalışır ama istenmeyen sayılabilir). Bu,
IPv4 ters DNS'ini bekleyen önceki son kontrolü değiştirir; yanlış posta adı ya
da adres yine engeller.

### Ek (2026-10-28): ikinci sunucu kodla katılır; tek sunucu kendi DNS'ini tutabilir

*Sahibin onayladığı kurulum sihirbazı v2, 4. aşama (2026-10-26 planının Q1, Q4,
Q5, Q7 maddeleri); v0.1.0-alpha.114 adayının kaynağında (yayımlanmadı); kurulu
bir sunucuda ölçülmedi*

Ekranda söylenen önceki sınırlar kalktı. Tek sunucu kendi DNS'ini bağımsız sunucu
olarak tutabilir (iki ad sunucusu adı da onda; BIND için sonradan yedek
eklenebileceği söylenmez). İki sunucuda ikinci sunucunun adresi hiç yazılmaz:
ilk panelin anahtar parmak izini ve bir gizli değeri taşıyan tek kullanımlık kodla
ilkine katılır; adları, adresi ve ana alan adı posta seçimi bu bağlantıdan gelir
ve ilk sunucunun çalışmasına bir kez bağlanır. Katılma, ana yöneticinin ikinci
sunucunun adlarına ve yayımlama bağlantısına onayıdır (Q5). Ana DNS sunucusu iki
sunucudan biri olabilir (Q1, Seçenek A): o zaman ilk sunucu ikinci katılana kadar
DNS yazılımı kurmaz ve hazırladığı listeyi devreder. Kurulum anında ns1 ana DNS
sunucusudur (Q2). DNS çifti yine yerel birincil/ikincil aktarımdır; bağlantı
yalnız kurulum yanıtlarını ve yedeğin kayıtlarını taşır, hiçbir barındırılan iş
ona bağlı değildir. Mevcut DNS sahipliği, topoloji ve kurtarma ilkeleri
değişmez; şema değişikliği yoktur.

---

## D-020 · Yönetilen sunucu desteği dağıtım adına değil kanıtlanmış yeteneğe bağlanır

*11 Ağustos 2026*

**Karar.** ÇelikPanel Agent'ın yönetilen sunucu hedefi yalnız Linux'tur. Platform
seçimi dağıtım adı, sürümü veya kod adı izin listesine değil, kanıtlanmış sunucu
yeteneklerine dayanır. Güncel paket adaptörleri `apt`, `dnf` ve `pacman`dır;
yalnız paket, depo, servis, güvenlik politikası veya yol gerçekten ayrışıyorsa
dar bir yerleşim override'ı eklenir.

**Algılama ve güven sınırı.** `/etc/os-release` içindeki `ID`, `ID_LIKE` ve
`VERSION_ID` çalıştırılmadan, salt veri olarak ayrıştırılır. Tek ve çelişkisiz
bir `ID`/`ID_LIKE` aile ipucu hangi eksiksiz araç zincirinin doğrulanacağını
seçebilir; böylece operatörün sonradan kurduğu ilgisiz paket yöneticisi bootstrap
yolunu değiştiremez. Kullanılabilir ipucu yoksa veya ipuçları çelişiyorsa tam
olarak bir eksiksiz güvenilir araç zinciri bulunmalıdır. APT sabit `apt-get` +
`apt-cache` + `dpkg-query`, pacman sabit `pacman`, DNF ise sabit `dnf` + `rpm`
takımıdır. Sabit `systemctl` ile `timeout`; root sahipli ve yazılamaz
`/run/systemd` zinciri; doğrudan `/run/systemd/private` Unix soketi ve zaman
sınırlandırılmış `systemctl is-system-running` çağrısıyla `running` veya
`degraded` kanıtı her durumda zorunludur. Var olan her aday aracın kendisi,
hedefi ve dizin zinciri root sahipli, yazılamaz, sabit-yol sözleşmesini
sağlamalıdır. Eksik, belirsiz, canlı olmayan, güvenli olmayan veya sabitlenmiş
vendor alternatifi dışına çıkan sembolik yetenek mutasyondan önce reddedilir.
Böylece ad ve sürüm sunucuyu tanımlar; adaptör yetkisini doğrulanmış yetenekler
verir.

**Sertifikasyon sınırı.** Paket adaptörünün seçilmesi genel özellik desteği
iddiası değildir. Arayüz yalnız paket, yol, güvenlik politikası, yaşam döngüsü ve
hazırlık önkoşullarını kendi içinde kanıtlayan yetenekleri gösterir. Bir dağıtım
istediği ad ve sürümle kanıtlanmış adaptörü yeniden kullanabilir; desteklenmeyen
paket ekosistemi veya kanıtlanmamış özellik, dağıtım etiketi listede olmadığı için
değil gerekli yetenek kanıtı bulunmadığı için engellenir.

**Güncel DNF önizleme kapısı.** Testlerle kapsanan DNF transaction, RPM envanteri,
sunucu kimliği ve dış paket kilidi temelleri herhangi bir DNF yeteneği açılmadan
önce geliştirilebilir ve sınanabilir. Doğrulanmış araç zinciri DNF seçen her
sunucuda normal kurulum yine ilk mutasyondan önce durur; çünkü SELinux Enforcing
panel/agent yaşam döngüsü henüz uygulanmamıştır. Bu engel dağıtım adı veya
sürümünden bağımsız olarak eksik güvenlik yeteneğini izler.
Aynı canlı SELinux kontrolü her APT veya pacman bootstrap ve rollback
mutasyonundan önce çalışır. Bu adaptörler yalnız SELinux etkin değilken ilerler;
etkin permissive veya enforcing durum, etiket yaşam döngüsü uygulanana kadar
fail-closed reddedilir. Paket yöneticisi yeteneği güvenlik politikası yeteneği
anlamına gelmez.

**Varsayılan-red kesin-yöntem ön filtresi.** Panel, her DNF önizleme sunucusu için
doğrulanmış tam host kimliğini tek ham dispatcher'ın hemen önünde kesin Agent
RPC yöntemiyle anahtarlanan bir ön filtreye taşır. Bu fail-closed bir temeldir;
açıkça aktivasyon yüzeyi değildir. Kesin yöntem ile host kimliği, parametreli
bir RPC'yi yetkilendirmeye yetmez: aynı yöntem farklı servis, paket veya hedef
argümanları taşıyabilir. Hiçbir DNF yetki girdisi, RPC argümanları kalıcı lease
kaydının kesin `Kind`, `Target` ve `PackageName` değerlerine bağlanmadan ve bu
eksiksiz yaşam döngüsü gerçek makine uçtan uca sertifikasyonundan geçmeden
eklenemez. DNF yetki kümesi boş kalır; dolayısıyla DNF sunucularındaki bütün
host mutasyonları reddedilmeye devam eder. Algılama, kimlik çözümleme, çapraz
derleme ve smoke kapsamı destek veya yetenek iddiası değildir.

**Kapsam dışı.** Uygulanmış adaptörü olmayan bir paket ekosistemi APT, DNF veya
pacman gibi davranarak destek kazanmaz; kanıtlanmış talep halinde ayrı adaptör
eklenebilir. Linux dağıtımları yalnız adları yüzünden reddedilmez; tek ve eksiksiz
güvenilir araç zinciri ile özelliğe özgü yerleşim/güvenlik kanıtı yoksa fail-closed
reddedilir. Windows ve FreeBSD yönetilen sunucu hedefi değildir. `!linux`
dosyaları yalnız derleme taşınabilirliği ve mutasyonsuz fail-closed sözleşmesini
korur; ürün desteği vaat etmez.

**Kurulum kaynağı değişmez.** Paket adaptörü D-018'i uygular: sistem servisleri
`apt`/`dnf`/`pacman` üzerinden dağıtım paketidir; taşınabilir uygulama ve
runtime'lar doğrulanmış resmi sürümdür. Zorunlu bir kaynak derlemesi müşteri
sunucusunda yapılmaz; merkezi CI'da üretilip ilgili ekosistemin imzalı paketi olarak
dağıtılır. Böylece yetenek kapsamı büyürken güvenlik güncellemesi, dosya sahipliği,
kaldırma ve geri alma paket yöneticisinde kalır.

**Neden.** Dağıtım başına kopyalanmış installer sayısı kaliteyi bölerek
büyütür; her şeyi kaynaktan derlemek ise yalnız binary üretimini ortaklaştırır,
systemd/SELinux/AppArmor, kullanıcılar, yollar, firewall ve güncelleme
entegrasyonunu ortaklaştırmaz. Paket-yeteneği adaptörü doğru tekrar-kullanım sınırıdır.

---

## D-019 · Panel korkutmaz: her yönetim sayfası kendi yardımını taşır; boş sayfa yasaktır

*25 Temmuz 2026*

**Karar.** Operatör istedi: "Kullanıcı dostu olması önemli. Korkutmasın,
yardımcı olsun... yönetim sayfasında bir yardım butonu olsun ve sayfa ile
ilgili yardım alınabilsin." Üç kural kalıcıdır:

1. **Her yönetim sayfasında Yardım düğmesi vardır.** Tek kabuktan gelir
   (ServiceShell); yeni sayfa yazan kimse yardımı ayrıca hatırlamak zorunda
   değildir. İçerik üç bölümdür: *Bu nedir?* (düz dille 2-3 cümle),
   *İpuçları* (panelden yapılacak pratik işler), *Bir şey mi ters
   görünüyor?* (belirti → çözüm). Dil teknik değildir: "arka plan süreci",
   "veritabanında çöp kutusu yoktur", "okumak her zaman güvenlidir".
2. **Yardım içeriği chrome değil İÇERİKtir.** i18n anahtar dosyalarına
   yüzlerce `help.x.tip3` anahtarı olarak dağıtılmaz; bileşen başına tek
   kayıtta iki dil yan yana durur (`web/src/help/serviceHelp.ts`). Tek dili
   güncelleyip öbürünü unutmak yapısal olarak imkânsızdır. Özel kaydı olmayan
   bileşen türünün genel kaydını alır — yardımsız sayfa var olamaz.
3. **Boş yönetim sayfası yasaktır.** Panel bir bileşenin durumunu, birimini,
   sürümlerini, paketlerini, portlarını, ayar dosyalarını ve günlüğünü zaten
   biliyor; "yakında" yazan bir sayfa, bilineni göstermeyi reddetmektir.
   Türetilmiş paneller (ComponentPanels) her sayfanın tabanıdır; özel
   sayfalar bunun ÜSTÜNE koyar, yerine geçmez.

**Neden.** Korkutan sayfa kullanılmaz; kullanılmayan panel güvenilmez. Yardım
düğmesinin sayfanın okuyucusunu korkuttuğu anda orada olması gerekir — kurulu
olmayan, düşmüş, yarım servislerde de görünür. Elle yazılmış sayfa listeleri
(9 sayfalık switch) tarayıcının elle unit listesi gibi çürüdü; türetilmiş
taban bu sınıfı kapatır.

---

## D-018 · Kurulum kaynağı cinse göre sabittir: sistem servisi = dağıtım paketi, uygulama/runtime = resmi sürüm, Docker temel değildir

*24 Temmuz 2026*

**Karar.** Operatör sordu: "her zaman git'ten çekip sisteme göre derlesek ya da
Docker kullansak daha iyi olmaz mı?" Cevap "her şeye tek yöntem" değildir —
kaos onu değil, kuralsızlığı doğurur. Kaynak, kalemin CİNSİNE göre seçilir ve
her cins için kural sabittir:

1. **Sistem servisi** (nginx, PostgreSQL, MariaDB, Postfix, Dovecot, Redis…)
   → **dağıtımın paket deposu** (apt/pacman/dnf). Neden: güvenlik yamasını
   dağıtımın güvenlik ekibi üstlenir (`apt upgrade` yamalıyor); systemd
   entegrasyonu ve config yolları hazır. Kaynaktan derlemek CVE takibini bize
   yükler; Docker'lamak systemd üzerinden başlat/durdur/durum okumayı kırar.
2. **Runtime + taşınabilir uygulama** (Node.js, Roundcube, ileride
   Python/Ruby, webmail alternatifleri) → **üreticinin resmi sürümü**
   (indir + sabit SHA-256 doğrula + aç). Neden: dağıtım tek sürüm dondurur,
   biz çok+güncel isteriz; ve dağıtım paketi dağıtıma özgüdür (Roundcube'da
   canlı kanıt: apt `roundcube` / pacman `roundcubemail` farklı yerleşim →
   Debian'da çalışıp Arch'ta çalışmıyordu). Resmi sürüm her Linux'ta TEK yol
   (D-004). Bu "git'ten çek + derle" DEĞİL "imzalı/donmuş sürüm" — composer/
   derleme adımının kırılganlığı yok.
3. **Docker temel DEĞİLDİR.** Paylaşımlı barındırma modeli sistemde doğrudan
   çalışır (nginx vhost + site başına FPM havuzu); 100 site = 100 konteyner
   israfı + port/ağ/volume kaosu + systemd entegrasyonu kaybı. Docker ileride
   bir CİNS olabilir (müşterinin kendi konteyner uygulaması için proje tipi,
   ya da izole/ağır bir yığın için mağazada seçenek) ama altyapının zemini
   değil — Plesk'te de eklentidir, ana model değil.
4. **Kaynaktan derleme**: yalnız gerçekten gerekince (dağıtımda yok VE resmi
   binary yok — nadir); o zaman derleme araçları geçici kurulur, sonra
   kaldırılır (temiz-sunucu).

**Dürüst sınır (aynı dilimde kanıtlandı).** "Her Linux'ta tek yol" ideali
bedava değildir: taşınabilir bir tarball bile ALT-bağımlılıklarda dağıtım
gerçeklerine değer — Roundcube'un SQLite'ı PHP'nin pdo_sqlite'ına muhtaçtı
(iki dağıtımda ayrı paket, Arch'ta üstelik elle enable), FPM soket yolu
farklıydı, nginx conf yerleşimi Debian-only'ydi (A10). Doğru mimari saf değil
HİBRİTTİR: taşınabilir gövde + dağıtım-farkındalıklı alt-detaylar (paket adı,
soket yolu, uzantı enable) — ve o alt-detayların dağıtıma özgü tek gerçeği
agent'ta yaşar, panel/katalog dağıtım bilmez.

**Reddedilen alternatifler.**
- **Her şeyi kaynaktan derlemek:** çoğu şey (PHP/script) derlenmez; sistem
  servislerinde CVE-yama sorumluluğunu üstlenmek; "temiz sunucu"ya derleme
  araçları dolar.
- **Her şeyi Docker'a koymak:** shared-hosting DNA'sıyla çatışır; systemd
  temelli tüm mekanizmalar (durum okuma, başlat/durdur) kırılır.
- **Her şeyi dağıtım paketiyle kurmak:** runtime'da tek-sürüm hapsi;
  taşınabilir uygulamada dağıtıma özgü yerleşim (Roundcube tuzağı).
- **"Duruma göre karar" (yazılı kural yok):** her yeni kalemde aynı tartışma;
  ve sınıf-atlaması riski (bir uygulamayı sistem servisi gibi paketlemek).

---

## D-017 · Sunum evrenseldir: seçilebilen her şey — bileşen, sürüm, entegrasyon, ürün — tek teklif uzayında zincirden geçer

*24 Temmuz 2026*

**Karar.** Operatörün cümlesi kanun maddesi niteliğinde: *"müşteri SSL seçerken
veya bayi müşterilerine hangi SSL'leri seçme izni vereceğini düşünürken,
esasında bizim onlara sunduğumuz seçenekler içinde hareket edecekler."*

1. **Zincirin öznesi genelleşir.** D-014'ün `kurulu ⊇ sunulan ⊇ kullanılan`
   zinciri yalnız kurulu bileşenler için değil, SEÇİLEBİLEN her şey için
   geçerlidir — sunucuya hiçbir şey kurmayan entegrasyonlar (SSL CA'ları,
   ileride Cloudflare, yedek hedefleri) dahil. Müşteri deneyiminde "hangi
   CA" seçimi, "hangi PHP sürümü" seçiminden farksız bir tekliftir.
2. **Tek teklif-kimliği uzayı.** Her sunulabilir kalem kanonik bir kimlik
   taşır: `component:<id>` (bütün) · `component:<id>:<sürüm>` (D-014: birim
   sürümdür) · `integration:acme:<id>` · `product:<id>` (D-012'nin
   `subscription_entitlements.product_id`'si bu uzayın ilk sakinidir).
   Teklif türü başına ayrı tablo/model YOKTUR — "aynı bilginin iki sahibi"
   hastalığının teklif katmanındaki karşılığı olurdu.
3. **Üç katman, üç ayrı soru; etkin küme = kesişim.**
   *Mevcut* (admin): kurulu bileşenler + kayıt defterinde sunucu genelinde
   AÇIK bırakılanlar — admin bir CA'yı bütün panelde kapatabilmelidir.
   *Sunulan* (bayi): plan içeriği — D-014'ün "eksik halka"sı burada
   `plan_offerings` (plan ↔ teklif-kimliği) olarak doğar.
   *Kullanılan* (müşteri): kullanım anındaki seçim (site başına sürüm,
   sertifika başına CA).
4. **Uygulama deseni her seçici uçta AYNIDIR.** Listeleme uçları (ssl
   sağlayıcıları, php sürümleri, node sürümleri…) çağıranın etkin kümesine
   süzülür; eylem uçları sunucu tarafında yeniden doğrular ve kodlu retle
   döner (`NOT_OFFERED` ailesi). UI yalnız sunulanı çizer — D-012 #7'nin
   ("görünürlük hakkı izler") genel hâli.
5. **Varsayılan cömert, daraltma bilinçli.** Plan hiçbir kısıt bildirmiyorsa
   her şey sunulur (bugünkü davranış aynen). Mekanizma geriye dönük hiçbir
   şeyi kırmaz; bayi daraltmak İSTEDİĞİNDE daraltır — D-014'teki
   "sadeleştirme hakkı"nın uygulanması.
6. **Zamanlama dürüstlüğü.** Tam mekanizma v0.3 kiracı/plan dilimiyle gelir
   (plan UI'sı olmadan davranışsız iskelet kurmak tiyatro olur). Bugünden
   geçerli olan tek yükümlülük: yeni yazılan her seçici uç bu deseni bilerek
   yazılır ve teklif kimlikleri şimdiden kanonik adlandırılır.

**Neden.** Bugün üç ayrı seçici doğdu (PHP sürümü, Node sürümü, SSL CA'sı) ve
üçü de "listeyi kim daraltır?" sorusuna cevapsız. Operatör, cevabın üçü için
de AYNI olması gerektiğini gördü. Üç ayrı daraltma mekanizması yazmak, üç ayrı
yetki modeli bakımı demekti.

**Reddedilen alternatifler.**
- **Entegrasyonları zincir dışında tutmak** ("kurulmuyor ki"): müşterinin
  gözünde CA seçimi de bir tekliftir; dışarıda tutmak ikinci bir yetki
  modeli doğurur.
- **Teklif türü başına ayrı tablo:** tek uzay + tek sunum defteri; D-012
  dersi.
- **Şimdi tam uygulama:** plan arayüzü yokken davranış değiştirmeyen
  iskelet — bekleyen karar, yarım kod'dan iyidir.

---

## D-016 · Sayfanın adı Bileşenler'dir; sözlük sabitlenir; servis tanımı incelir

*23 Temmuz 2026 — 5 danışmanlı panel (pazar, TR-UX, bilgi mimarisi, şeytanın avukatı, küresel/yerelleştirme) + hakem sentezi; operatör onayı*

**Karar.**
1. **Sayfa ŞİMDİ yeniden adlandırılır:** EN **Components** · TR **Bileşenler**.
   Rota `/services` kalır. Beş danışmandan dördü bağımsız aynı sonuca vardı:
   ad bugün zaten yanlış (sektörde Services = daemon; sayfada PHP, phpMyAdmin,
   nftables var) ve değiştirme maliyeti kullanıcı/doküman biriktikçe büyür.
2. **Sözlük** (mağaza bu kelimelere doğar; şemsiye yeniden tartışılmaz):
   şemsiye **component/bileşen** · cinsler: **Service/Servis** ·
   **Runtime/Çalışma Ortamı** (rozetlerde kısa biçim: "Ortam") · **Tool/Araç** ·
   gelecekte **Module/Modül**, **Task/Görev**, **Integration/Entegrasyon** ·
   mağaza: EN **Store**, TR **Mağaza** (sayfa başlığı "Bileşen Mağazası").
   "Servis" varsayılan cins-filtresi olarak yaşar — destek dili ("servisi
   yeniden başlat") bozulmaz.
3. **Servis tanımı incelir** (operatörün "start/stop yapılan = servis" testi
   yön olarak doğru, harfiyen değil): *servis, kendi yerleşik süreci olan ve
   o sürecin çökmesi başlı başına bir olay sayılan bileşendir.* İki karşı
   örnek kataloğun kendisinden: nftables oneshot unit'iyle start/stop kabul
   eder ama yerleşik süreç bırakmaz (doğru sınıfı araç; oneshot'a
   "uygulandı/uygulanmadı" durumu yakışır, koşuyor noktası asla) ve php-fpm
   gerçek daemon'lar taşır ama runtime'dır çünkü birincil işi site-başına
   sürüm seçimi. "Denetlenebilir unit'i var" ise cins değil DİK bir
   yetenektir: başlat/durdur düğmeleri her satırda — sürüm çekmecesi dahil —
   o yetenekten çizilir.
4. **Kayda geçen karşı görüş** (küresel danışman, dürüst haliyle): hiçbir
   büyük panel nav'ında "Components" kullanmıyor (cPanel şemsiyesi
   "Software", aaPanel "App Store"); Components, ESL-ağır büyüme pazarları
   için en soyut aday ve Joomla'nın Components/Modules/Plugins üçlüsüyle
   çakışıyor. Alternatifi "Software/Yazılımlar"dı. Hakem reddetti çünkü
   "Software", entegrasyon cinsi (sunucuda yazılım olmayan Cloudflare
   bağlantısı) doğduğu gün planlı tarihte yalanlaşır — "Servisler"i bugün
   yanlış yapan hastalığın aynısı — ve satır/doküman için sayılabilir birim
   adı vermez. Bulunabilirlik ya da destek-dili sorunu ÇIKARSA geri dönüş
   adayı Software'dir; bu cümle o gün için yazıldı.

**Neden şimdi.** Kalem sayısı yalnız artacak (modül/görev/entegrasyon yolda,
mağaza yolda); yanlış adın üstüne alışkanlık ve doküman biriktirmek, düzeltme
gününü pahalılaştırmaktan başka işe yaramaz. Şeytanın avukatının kendi çöküş
koşulu da buydu: "servis olmayan satırlar çoğaldığında teslim olurum" — yol
haritası tam bunu vaat ediyor.

---

## D-015 · Eklenti pakettir, cins doğadır: mağaza hak verir kurmaz, çekirdek kataloğun sınırı para değil sorumluluktur, cins listesi açıktır

*23 Temmuz 2026*

**Karar.** Operatörün araba benzetmesi (standart/opsiyonel donanım, motor değişimi)
ve getirdiği danışman analizi (Plesk/cPanel eklenti dökümü) üzerine altı bağlı karar:

1. **İki eksen kesindir ve karışmaz.** *Kalem/eklenti* = edinme-dağıtım birimi
   (ücretsiz/paralı, 1./3. taraf — D-012'nin ekseni). *Cins* = kurulan şeyin
   mekanik doğası (service/runtime/tool — D-010'un ekseni). Danışmanın özeti
   modelin kendisidir: *"eklentiler çoğu zaman servisleri yönetir; kendileri
   servis değildir."* Bir eklenti 0..N parça getirebilir (Plesk Email Security
   tek eklenti → postfix+dovecot+clamav yönetir); her parça KENDİ cinsiyle
   görünür, eklenti kendisi bir cins değildir.
2. **Mağaza hak verir, kurmaz.** Satın alma / lisans anahtarı / koltuk havuzu
   mağazada; edinilen kalem Servisler kataloğunda belirir ve oradan kurulur.
   Kurulumun tek adresi katalogdur — ikinci Kur düğmesi, B3b'de silinen
   AdminNodeInstall'ın (iki-adres hatası) tekrarı olurdu.
3. **Çekirdek katalog ile mağazanın sınırı PARA değil SORUMLULUKTUR.**
   Çekirdek katalog, panelin garanti verdiği küçük küratörlü kümedir (iki
   dağıtımda paket adları, güvenlik duvarı portları, kaldırma, test —
   hepsi bizim borcumuz). Nadir kullanılan ÜCRETSİZ kalemler (örn. niş bir
   veritabanı) çekirdeğe şişkinlik ve veremeyeceğimiz garanti demektir —
   onlar da mağazadan gelir. (Bu, 22 Tem'deki "ücretsiz kalem mağazaya
   girmez" ifadesinin düzeltmesidir: ölçüt fiyat değil, garanti sınırı.)
4. **Cins listesi açıktır ama cins, İLK kalemiyle doğar.** Bugün üç cins üç
   gerçek kalem ailesini taşıyor. Danışman dökümündeki kalan biçimlerin
   haritası hazırdır ve günü gelince açık cins olarak eklenir:
   *modül* (host servisin içinde yaşar, kendi unit'i yok — ModSecurity,
   Brotli; "hangi servisin içinde" alanı gerekir) · *görev/timer* (çalışır ve
   biter — yedekleme, sertifika yenileme) · *entegrasyon* (sunucuya hiçbir
   şey kurulmaz; durumu "bağlı mı", sırrı mühürlü API anahtarı — Cloudflare,
   S3 yedek hedefi, UptimeRobot). *Agent* ayrı cins DEĞİLDİR: mekanikçe
   servistir (Imunify/Acronis agent'ı başlatılır/durdurulur); farkı ticari.
5. **Derlenen dil için kurulacak şey yoktur.** Go/Rust "desteği" katalog
   kalemi olamaz çünkü çoğalma testi (D-011) N der: runtime, müşterinin
   binary'sinin İÇİNDE gelir. Uygulama modu (ters vekil + unit) herhangi bir
   binary'yi bugün zaten çalıştırır — Go/Rust desteği fiilen vardır, satır
   gerekmez. Yorumlanan diller (Python, Ruby) Node ile aynı sınıftır: günü
   gelince runtime kalemi olurlar. (İleride "Git'ten çek + sunucuda derle"
   yapılırsa derleyici zinciri bir *tool* kalemi olur — o gün gelirse.)
6. **İki sayfa iki sorudur.** Servisler (adı ileride "Bileşenler"
   olabilir) = "sunucumda ne var, nasıl işliyor?" Mağaza = "ne edinebilirim,
   kimden, hangi hakla?" Aynı ürün ikisinde de görünebilir; çelişki değil,
   iki farklı an.

**Neden.** Operatör üç somut soruyla modeli zorladı (nadir-ücretsiz kalem
nereye; Go/Rust ne olacak; danışman dökümü 7 çalışma biçimi sayıyor) ve model
üçüne de mevcut eksenlerle cevap verdi — yalnız çekirdek/mağaza sınırının
gerekçesi yanlış ifade edilmişti (fiyat değil sorumluluk). Danışmanın
"gerçek sunucuda kontrol" bölümü (list-unit-files, ss -lntup) taramanın
bugün yaptığının aynısıdır — dış doğrulama olarak kayda geçer.

**Ek (24 Tem — operatörün sınır bekçiliği üzerine): çekirdeğin ölçüsü.**
Operatör sordu: "hayati servisler dışındaki her şey önce mağazada durup
istenince kataloğa geçmeyecek miydi? Yüzlerce program Components'e girerse
kargaşa olmaz mı?" Doğru hatırlıyor ve sınır şimdi sayıya bağlanıyor:
- **Çekirdek = rol başına varsayılan + 1-2 adlandırılmış alternatif.**
  (web: nginx+Apache · DNS: PowerDNS+BIND · SMTP: Postfix+Exim · spam:
  SpamAssassin+Rspamd · webmail: Roundcube …) Hedef ONLARCA kalem; asla
  yüzlerce. 23-24 Tem eklemeleri (rspamd, exim) bu sınıfa girer — uzun
  kuyruk değil, koltuk üyesi.
- **Uzun kuyruk mağazada yaşar** (ücretsizse tek tık "ekle" ile edinilir ve
  kataloğda belirir — 2. madde zaten böyle diyordu; sayı disiplini artık
  açık). SOGo gibi büyük grupware çekirdeğe GİRMEZ, mağazayı bekler; netdata
  sınırdadır (izleme rolünü panelin kendi sayfası karşılıyor) ve mağaza
  doğduğunda oraya taşınmaya adaydır.
- Kargaşanın bugünkü freni de kayıtta kalsın: varsayılan görünüm yalnız
  kuruluları gösterir (D-010, kurulu-önce); katalog düğmenin arkasındadır.
  Ama ölçek sigortası görünüm değil, mağaza kapısıdır.

**Reddedilen alternatifler.**
- **Danışmanın 7 biçimini bugün 7 cins yapmak:** elimizde modül, timer ve
  entegrasyon KALEMI yokken şema spekülatif olurdu; cins ilk kalemiyle
  doğar, yoksa D-010'un çözdüğü sorunun tersine döner (boş kategoriler).
- **Go/Rust için "destek satırı":** kavramsal hata — D-011'in "Go satırı"
  reddiyle aynı; kurulacak şey olmayan yerde satır yalan söyler.
- **Sınırı ücretsiz/ücretli çizmek:** nadir ücretsiz kalem çekirdekte
  garanti yükü, mağazada doğal dağıtım bulur; para yanlış eksendir.
- **Cins başına ayrı sayfa:** "kurulu olan tek listede" ilkesini yedi
  sayfaya bölerdi; cins satırın çizimini değiştirir, adresini değil.

---

## D-014 · Yetki zinciri daralan bir süzgeçtir: admin kurar, bayi sunar, müşteri kullanır — birim sürümdür

*21 Temmuz 2026*

**Karar.** Bir yeteneğin kime ulaşacağı üç ayrı kararla belirlenir ve her katman
yalnızca **daraltır**, asla genişletemez:

    kurulu (admin)  ⊇  sunulan (bayi)  ⊇  kullanılan (müşteri)

- **Admin** yalnız şuna karar verir: bu servis/eklenti **sunucuda var mı**.
  Kurmuş olması kimseye verdiği anlamına gelmez.
- **Bayi** yalnız şuna karar verir: adminin kurduklarından hangilerini
  **müşterilerime sunuyorum**.
- **Müşteri** yalnız şuna karar verir: bayinin sunduklarından hangilerini
  **kullanıyorum** — ve bu karar site başınadır, hesap başına değil.

**Zincirin birimi servis değil, sürümdür.** "PHP sunuyorum" bir şey ifade etmez;
bayi **PHP 8.3'ü** sunar ya da sunmaz, müşteri site başına sürüm seçer. Bu,
D-010 ile çelişmez, ona ikinci bir eksen ekler: katalogda **görüntü** birimi
runtime'dır (tek satır, sürümler çekmecede), **hak** birimi sürümün kendisidir.

**Ayarlar da aynı zincirden geçer**, yalnız varlık değil: admin sunucu
varsayılanını ve tavanını koyar, bayi hangi düğmeye dokunulabileceğine karar
verir, müşteri o sınırlar içinde ayarlar.

**Neden.** Operatörün somut vakası: admin yalnız PHP 8.4 kurdu; site sahibinin
8.3'e ihtiyacı olabilir ve 8.4'ü hiç kullanmayacak. Tek bir "kurulu mu" bayrağı
bu üç farklı soruyu aynı anda cevaplayamaz. Bugün kodda tek bağış katmanı var
(`hasEntitlement`: abonelik ürünü tutuyor mu); admin→bayi ile bayi→müşteri
ayrımı yok, yani bayinin "sunmuyorum" deme hakkı temsil edilemiyor.

**Katmanların kodda yeri** (üçünden ikisi hazır):
- Admin katmanı = kurulu set (`InstalledServiceIDs` + katalog). ✅ var
- **Bayi katmanı = servis planları** (`plan_handlers.go`, `004_accounts.sql`).
  Yeni bir yapı gerekmez: "sunuyorum" kararı planın içeriğidir — *"bu plan PHP
  8.2 ve 8.3 içerir, 8.4 içermez"*. Plesk de aynısını yapar (Service Plans).
  ⬜ eksik halka budur
- Müşteri katmanı = site başına seçim (`site.php_version`, `CreatePool(siteID,
  username, phpVersion)`). ✅ var

**Sonuçlar.**
- **Kaldırma zincir boyu bir olaydır.** Admin bir sürümü silmek isterse, onu
  sunan planlar ve kullanan siteler engeller. D-010'daki `RUNTIME_IN_USE` reddi
  "kullanımda" demekle yetinmemeli, *kimin* engellediğini saymalı (kaç plan, kaç
  site) — aksi hâlde admin neyi kırdığını göremeden karar verir.
- **Sunucu-geneli PHP ayarı bu modelle bağdaşmaz.** Bugünkü PHP-FPM sayfası
  uzantı anahtarlarını ve php.ini değerlerini sunucu genelinde tutuyor:
  `disable_functions` bir güvenlik denetimidir, bir müşterinin ihtiyacı herkesin
  politikası olur; `memory_limit` sunucu genelindeyse tek bir ağır site herkesin
  tavanını belirler; `ffi`yi bir müşteri isterken diğerinin istememesi mümkün
  değildir. Sayfa kendini **sunucu varsayılanı** olarak adlandırmalı ve site
  altlığı ayrıca var olmalıdır. Altyapı hazır: her site zaten kendi FPM havuzunu
  alıyor, havuz kendi `php_admin_value` yönergelerini taşıyabilir.

**Önkoşul (dürüstlük notu).** Bu model PHP için şu an **boştur**: Sury deposu
olmadığı için admin ikinci bir sürümü isteseydi bile kuramaz (bkz. D-002
düzeltmesi, D-010). Verecek tek şey varken "kimin neyi sunacağı" tartışması
teoriktir. Bu yüzden B3c (Sury + çoklu sürüm) sıranın önüne alınır — süs değil,
zinciri anlamlı kılan önkoşuldur.

**Reddedilen alternatifler.**
- **Bayi katmanı için ayrı bir "teklif" tablosu**: planlar zaten tam olarak bu
  soruyu cevaplıyor; ikinci bir yapı, aynı bilginin iki sahibi olurdu (B3'ün
  kapatmaya çalıştığı hatanın aynısı).
- **Adminin kurduğu her şeyin otomatik sunulması**: en az koddur ve yanlıştır —
  bayinin sadeleştirme hakkını yok eder (bir bayi bilerek yalnız 8.3 sunmak,
  destek yükünü azaltmak isteyebilir).
- **Hak birimini servis düzeyinde tutmak** ("PHP hakkı var/yok"): operatörün
  vakasını hiç çözmez; 8.4 kurulu diye 8.3 gerektiren müşteriye "PHP'n var"
  demek olurdu.
- **Sunucu-geneli ayarları yeterli saymak**: çok kiracılı bir panelde bir
  müşterinin `disable_functions` ihtiyacını herkese dayatmak demektir.

---

## D-013 · Alan adı oluşturmak runtime seçmek değildir: PHP bir site anahtarıdır, kurulum kararı değil

*21 Temmuz 2026*

**Durum: UYGULANDI (24 Tem).** Ekran artık yalnız amacı soruyor (Web sitesi /
Yalnız DNS); PHP sürüm seçici oluşturmadan tamamen kalktı ve web sitesi için
PHP ön koşul olmaktan çıktı — PHP'siz sunucu da site barındırabiliyor. Kabloda
proje tipi "static" gider (PHP'nin kapalı hâli); sunucu sözleşmesi değişmedi.
Canlı doğrulama: Boston'da site oluşturuldu ve anında sunuldu (HTTP 200).

**Karar.** "Alan adı ekle" ekranı **"ne çalışacak?"** sorusunu sormayı bırakır.
Sorduğu tek şey amaçtır: **web sitesi · yalnız posta · yalnız DNS**. Runtime
(PHP açık/kapalı + sürüm, Node/proxy modu, port, başlangıç dosyası) oluşturmadan
sonra **sitenin ayarı** olur — açılıp kapanabilen, geri alınabilir bir özellik.

Bunun doğrudan sonucu: **`static` ile `php` ayrı proje tipi değildir.** İkisi
aynı şeyin PHP anahtarı kapalı ve açık hâlidir; şablon farkı üç satırdır
(`index.php` dizin sayılır mı · eşleşmeyen URL `/index.php`'ye mi düşer yoksa
404 mü · `.php` fastcgi'ye gider mi). `node`/`proxy` ise gerçekten ayrı bir
moddur — doküman kökü sunulmaz, her şey ters vekile gider.

**PHP-FPM isteğe bağlı kalır** (operatör kararı, 21 Tem). Anayasanın "kurulum
anında hiçbir şey kurulmaz, kurulu olmayan görünmez" maddesi PHP için de
geçerlidir: yalnız-DNS ya da yalnız-Node sunan bir makine PHP taşımaz. PHP
kurulu değilse site ayarındaki anahtar bunu dürüstçe söyler ve yöneticiye kurma
yolunu gösterir — ama bunu **oluşturma ekranında** değil, kendi yerinde yapar.

**Neden.** Operatörün sorusu şuydu: "PHP kullanmayacak birine neden PHP
zorluyoruz, gereksiz ve kafa karıştırıcı değil mi? Herkes WordPress kurmak
zorunda değil." Haklıydı ve teşhis şuydu: ekran, kurulu olmayan bir yeteneği
turuncu bir çağrıyla reklam ediyordu — Servisler sayfasında daha yeni
uyguladığımız "kurulu olmayan görünmez" kuralının tam tersi. Statik site kuracak
birine sunucusu eksikmiş gibi hissettiriyordu; sunucu eksik değildi, o kişi PHP
istemiyordu.

Plesk'in gerçek akışı incelendi (operatörün 9 ekran görüntüsü, canlı kurulum) ve
şu çıktı: **Plesk'in oluşturma diyaloğu bir runtime seçici değildir.** İlk ekran
"içerik buraya nasıl gelecek" diye sorar (boş sayfa · dosya yükle · WordPress ·
Git'ten çek · Node.js · yalnız posta). İkinci ekranda runtime hiç sorulmaz.
Üçüncü ekranda "Configuring PHP" **koşulsuz** bir adımdır. PHP sonradan sitenin
ayar sayfasında bir onay kutusudur (sürüm listesi + site başına php.ini).
Kritik ayrıntı: **"Node.js" seçeneği oluşturmayı hiç değiştirmez** — form "Blank
website" ile birebir aynıdır ve oluşturma bitince Node **hâlâ etkin değildir**,
"Enable Node.js" düğmesine basmak gerekir. Yani Plesk'te bile o ilk ekran bir
yönlendirme menüsüdür; gerçek karar sitenin ayarında verilir.

**Plesk'ten şekli alıyoruz, paketlemeyi almıyoruz.** Plesk'te soru hiç doğmaz
çünkü PHP'yi kendisi getirir (`/opt/plesk/php/8.3`); PHP asla "kurulu değil"
olamaz. Bizde olabilir. Bu yüzden "her sitede PHP varsayılan açık" doğrudan
kopyalanamaz — kopyalansaydı D-003 ve anayasa maddesi çiğnenirdi.

**Yan bulgu (kapatıldı).** Bu tartışma gerçek bir açığı ortaya çıkardı: statik
vhost'ta PHP işleyicisi yoktu ama `.php` için ret kuralı da yoktu, bu yüzden
nginx dosyayı `application/octet-stream` olarak veriyor ve tarayıcı **kaynak
kodu indiriyordu**. A6 ailesi. Canlı nginx'te ölçülüp kapatıldı (`b1a6aac`).
Ders kayda geçsin: *"burada X çalışmaz", "burada X'in kaynağı yayınlanır"
demek olmamalı.*

**Reddedilen alternatifler.**
- **PHP-FPM temel kurulumun parçası olsun** (Plesk modeli): soruyu kökten
  bitirirdi ve pazar beklentisiyle birebir örtüşürdü, ama "kurulu olmayan
  görünmez" ilkesinden PHP için feragat demekti. Operatör isteğe bağlıyı seçti.
- **Kurulum sırasında operatöre sorulsun** ("bu sunucu PHP sunacak mı?"):
  makine başına tek karar cazipti, ama install.sh'a kalıcı bir dallanma ekler ve
  sonradan fikir değiştireni yarı-yolda bırakır.
- **Dile göre seçenek çoğaltmak** ("Node.js sitesi", "Python sitesi", "Go
  sitesi"): nginx'in ve panelin gözünde üçü de aynı şeydir —
  `127.0.0.1:PORT`'a ters vekil + bir unit. D-010'un "Go satırı kavramsal hata
  olurdu" gerekçesinin aynısı. Uygulama modu **tek** seçenektir, runtime onun
  içinde bir alandır.
- **Oluşturma ekranındaki turuncu uyarıyı yumuşatmak/kısaltmak**: semptomu
  tedavi ederdi. Sorun uyarının tonu değil, sorunun oraya ait olmamasıydı.
- **Uyuşmazlık tespitini tek başına yeterli saymak** (statik sitede `.php`
  görünce uyar): iyi bir özellik ve yapılacak, ama yanlış soruyu sormaya devam
  edip arkasından temizlik yapmak olurdu. Önce soru düzelir, tespit emniyet
  ağıdır.

**Uygulama sırası.** (1) ✅ statik `.php` sızıntısı kapandı — güvenlik, tasarım
tartışmasından bağımsız · (2) Add Domain "ne için?" sorusuna iner, turuncu
dayatma kalkar, metinler markadan davranışa çevrilir ("WordPress, Laravel…"
gider) · (3) site ayarında PHP anahtarı + sürüm; `static`/`php` tek moda birleşir
· (4) uyuşmazlık tespiti + tek tıkla düzelt · (5) Uygulama modu (Node/proxy)
oluşturma sonrası ayarda görünür hâle gelir — arka uç (`hosting_handlers.go`,
şablonun vekil dalı, `HostingTypePanel`) zaten hazırdır, eksik olan tek dil.

---

## D-012 · Lisanslı ürünler ve satış zinciri: hak bir havuzdur, uyum satıcıyla operatör arasındadır

*20 Temmuz 2026*

**Karar.** Operatörün modeli ("her şey bir kalem; bazıları bedava bazıları paralı;
admin satın alır, bayiye ya da doğrudan müşteriye satar") şu yapıyla karşılanır —
ve **üçüncü-taraf ticari ürünler baştan kapsam içindedir** (operatör kararı):

1. **"Şey" ile "kullanma hakkı" ayrılır.** Ürünün kendisi sunucuya bir kez kurulur
   (çoğalma testi = 1 → katalog kalemi, D-011). **Hak** ise abonelik başınadır ve
   ağaçta aşağı akar. Bu ikisi bağımsız eksenlerdir: kurulum ikili bir durumdur,
   hak sayılabilir bir kaynaktır.
2. **Hak, disk/domain gibi bir havuzdur.** Yeni mekanizma yok: v0.3'ün
   `reseller_pools` deseni ürünlere genişler. Admin'in kontenjanı → bayiye tahsis →
   müşteriye tahsis; aşımda kodlu 409 + kalan-havuz. Admin **doğrudan müşteriye** de
   satabilir (abonelik sahiplik ağacı bunu zaten taşır).
3. **Lisans modeli ürün tanımına girer:** `license_model ∈ {server, seat}`.
   *server* → tek fiyat, sınırsız kullanıcı; admin havuzu sonsuz, satış saf marj.
   *seat* → admin satıcıya adet başına öder; **fazla tahsis gerçek paradır ve lisans
   ihlalidir**, bu yüzden havuz sertçe uygulanır (uyarı değil, ret).
4. **`seat_unit` zorunludur** (`mailbox | site | subscription | server`). Satıcılar
   farklı şey sayar; birimi yazmazsak yanlış şeyi sayarız ve panelin sayısı
   satıcının faturasını tutmaz — bu, en sinsi tuzaktır.
5. **Lisans anahtarı sır olarak saklanır** — A4'ün `enc:v1` mekanizması (mühürle,
   kullanım anında çöz). Ticari ürünlerin çoğu kurulumda anahtar ister; anahtarsız
   kurulum yarım kurulumdur.
6. **Fiyat tek sayı değildir.** Zincirde üç fiyat vardır: satıcı→admin (maliyet),
   admin→bayi, bayi→müşteri. Bugünkü tek `MonthlyPriceCents` alanı yetmez; desen
   v0.3'teki "bayiye ait plan" (`service_plans.owner_id`) ile aynıdır.
7. **Görünürlük hakkı izler.** Bayi ürünü almadıysa müşterileri onu **hiç görmez**
   ("kurulu olmayan görünmez"in haklara uygulanmış hâli). İnce nokta: bayi isterse
   "satın al" tanıtımını açabilir — görünürlük bayinin kararıdır, varsayılan gizli.
8. **Kurulu olmayan ürün satılamaz.** Sunucuda kurulu olmayan bir ürüne hak satmak
   müşteriye hiçbir şey satmaktır → kodlu ret.
9. **Geri alma, abonelik askısıyla AYNI kuraldır.** Bayi ödemeyi kestiğinde:
   ağacında yeni tahsis 403, mevcut kullanım grace sonuna dek yaşar, veri silinmez.
   İki farklı "kesildi" davranışı olmaz.

**Neden bu yük kabul edildi.** Zinciri sonradan üçüncü-tarafa açmak, tahsis ve
fiyat modelini yeniden yazmak demekti; `license_model`/`seat_unit` alanlarını
baştan koymanın maliyeti iki alandır. Operatör bilinçli olarak zor yolu seçti.

**DÜRÜSTLÜK SINIRI — panel neyi garanti ETMEZ.** Panel yalnız **kendi tahsis
kayıtlarını** uygular; satıcının lisans şartlarına uyumu **garanti etmez**:
- Satıcı farklı sayabilir (ör. kutu yerine alan adı); panel mutabakat ekranı
  gösterir ("satıcı: 50 · tahsis: 47 · boşta: 3") ama **fatura satıcının
  gerçeğidir**, panelin değil.
- **Üçüncü-taraf ürünü alt-lisanslama (bayiye/müşteriye satma) hakkı operatörle
  satıcı arasındadır**; çoğu satıcı bunu ortaklık sözleşmesine bağlar. Panel bu
  hakkı doğrulayamaz ve doğruladığını iddia etmez. Ekranda ve dokümanda açıkça
  yazar — aksi hâlde kullanıcıyı lisans ihlaline teşvik eden bir özellik yapmış
  oluruz.

**Reddedilen alternatifler.** Sessiz fazla-tahsiz (uyarıp geçmek — *seat* modelinde
operatöre borç yazdırır) · satıcı başına API entegrasyonunu çekirdeğe koymak (her
satıcı ayrı entegrasyon, dış bağımlılık yasağına ve sadeliğe aykırı; başlangıç
**elle anahtar + elle kontenjan**, otomasyon ancak tek bir satıcıda gerçek talep
doğarsa) · hakları plana gömmek (plan değişince hak kaybı sessiz olur; hak ayrı
kayıttır, plan yalnız varsayılan verir) · üçüncü-tarafı sonraya bırakmak (operatör
reddetti: zincir sonradan açılırsa tahsis modeli yeniden yazılır).

---

## D-011 · Framework servis değildir: çoğalma testi, preset bütçesi ve sürüm sahipliğinin sınırı

*20 Temmuz 2026*

**Karar.** Dört bağlı karar, tek kayıtta:

1. **Sınır kuralı sayısallaşır — çoğalma testi.** Bir şeyin katalog kalemi mi
   site-içi mi olduğunu tek soru belirler: *sunucuda N site açıldığında bu şeyin
   diskteki örnek sayısı 1'de mi kalır, N'e mi çıkar?* 1 → katalog kalemi (sürümü
   dağıtım/vendor deposu yönetir); N → site-içi (sürümü müşterinin lock dosyası ya
   da uygulamanın kendi güncelleyicisi yönetir). **Katalog = L1 (servisler) + L2
   (çok-sürümlü runtime'lar), başka hiçbir şey.** Laravel, Symfony, Django,
   Next.js katalogda **yoktur ve olmayacaktır**; Composer, Node ve certbot ise
   katalog kalemidir (tek örnek). Sınıfı yazılım değil **konuşlandırma biçimi**
   belirler: aynı phpMyAdmin her müşterinin docroot'una kopyalansaydı N olur ve
   katalogdan düşerdi. Test tartışmaya kapalıdır çünkü sayısaldır.

2. **Panel, N örnekli hiçbir şeyin sürümünü sahiplenmez.** Panel bir güncelleyici
   **yazmaz**; uygulamanın kendi güncelleyicisini site kullanıcısı kimliğiyle
   **tetikler**. Sürüm sabitlemez, dosya yamalamaz, `DISALLOW_FILE_MODS` yazmaz,
   "desteklenen sürüm" göstermez. Bu, yol haritasındaki "WordPress Toolkit
   derinliği (güncelleme, sertleştirme)" maddesini bağlayıcı olarak sınırlar:
   sertleştirme presetlerinden otomatik güncellemeyi kapatan hiçbir madde
   varsayılan açık olamaz.

3. **Framework desteği = site ilkelleri (L3) + preset; asla tip, asla kurucu.**
   Panelin Laravel'e dair sahip olabileceği tek şey bir *varsayılanlar seti*dir
   (docroot `public` önerisi, cron satırı metni, queue komut derleyicisi) — bir
   kurucu değil. Provizyonda tek bir `if type == laravel` dalı yoktur.

4. **`appCatalog` bir tip ekseni değil, geri alınabilir kurulum eylemidir.** Site
   tipi PHP'dir; WordPress domain oluşturma akışında bir seçenek **değildir**,
   kurulduktan sonra çalıştırılan ve **kaldırılabilen** bir eylemdir.

**Neden.** Operatörün sorusu: "Laravel servis mi, katalogda mı durmalı — ve bir
kez açarsak bu böyle gider mi?" Beş rakip panelin gerçek kaydı incelendi:

- **Kaymanın motoru katalog değil, preset'tir.** Katalog girişi pahalıdır (paket
  adı, iki dağıtım ailesi, kaldırma yolu, init adımı); preset'in maliyeti üç
  dizedir ve reddetmenin argümanı yoktur. "Symfony bedava gelir" cümlesi Django,
  Ghost, Strapi için de kurulur; sekizinci preset'te ekran bir framework
  seçicisidir. **cPAddons da "tek tık kurulum" değil, "yalnızca yapılandırma
  reçetesi" olarak başladı.**
- **Kaymanın ikinci motoru sürüm sahipliğidir.** cPAddons'ı öldüren kurulum
  değildi: N örnekli WordPress'in sürümünü panel üstlendi, `update.php`'yi
  yamaladı, kullanıcılar WP 3.9'da çakılı kaldı. Aynı desen aaPanel'de (Laravel
  5.4/PHP 7.4'te donmuş one-click; kendi personeli "ürettiğimiz dosyaları silin"
  diyor) ve Plesk'te (iskelet üreteci upstream değişiminde kırıldı; APS kataloğu
  18.0.77'de **tamamen kaldırıldı**) tekrarlandı. Sektör bu modelden geri çekildi.

**Kuralı ihlal etmeyi pahalı kılan üç mekanizma** (kural yalnız Markdown'da
yaşarsa iki yıl dayanmaz — bu karar D-003/D-010'un aksine kodda zorlayıcı ister):

- **Yapısal saflık şartı (preset testi).** Bir preset ancak *zaten var olan
  jenerik alanların ön-doldurulmuş değerleri* olarak ifade edilebiliyorsa kabul
  edilir. Tek bir yeni alan ya da tek bir `if framework ==` dalı gerekiyorsa
  preset **reddedilir**. Preset'ler salt-veri tek dosyada yaşar; UI'da seçilebilir
  bir "tip" değil, formun üstündeki "formu doldur" düğmesidir. Preset sayısı 3'ü
  aşarsa bu bir kod değişikliği değil **strateji değişikliğidir**.
- **Framework adı yasağı + CI kapısı.** Framework adı hiçbir enum sabitinde, DB
  kolonunda, API alan değerinde veya systemd unit adında geçemez; yalnız i18n
  dizelerinde geçer. Docroot açılır kutusu framework adı değil **yol değeri**
  listeler (`(kök) | public | public_html`). `validProjectTypes`
  (php/static/node/proxy/forwarding) *taşıma biçimini* tanımlar, uygulamayı değil —
  **kilitli sayılır**. CI'da Go kaynaklarında `laravel|symfony|django|nextjs|ghost`
  grep'i sıfır eşleşme vermeli (i18n hariç).
- **Girişi olan her kalemin çıkışı baştan yazılır.** `appCatalog` girişleri zorunlu
  karar-kaydı ID'si + bakımcı + kaynak sağlaması taşır — "üç satırda giriş eklemek"
  imkânsızlaşır. Kabul kararına girişi kaldıran yol ve "kurulmuş siteler ne olur"
  cevabı (her zaman: **dosyalar müşterinindir, panel yalnız düğmeyi kaldırır**)
  birlikte yazılır.

**Ticari basınç da kapatılır.** `app_installer` girişi bugün "One-click installs
for WordPress **and other apps**" diyor — tek girişli bir listeyi çoğul bir plan
özelliği olarak satıyor. İki zararı var: satış tarafı doldurulacak boş bir kova
görür (kaymanın en güçlü motoru mühendislik değil **satılmış bir vaattir**), ve
liste plan özelliğine bağlandığı an giriş silmek sözleşme ihlaline döner. Ürün
gerçekliğe indirilir: "WordPress tek-tık kurulumu"; çoğul ifade kaldırılır.

**Konumlandırma (satış ve ürün aynı cümleyi kullanır).**
*"CelikPanel uygulamanızı kurmaz; uygulamanızın altındaki sunucuyu yönetir.
Kodunuzun sürümü sizindir — ve bunu bir sınır olarak değil, bir garanti olarak
veriyoruz."* Teknik alıcı için: *"Panelin sahiplendiği her şeyin sunucuda tek bir
örneği vardır; sitenizde N kopya halinde duran hiçbir şeyin sürümüne karışmayız."*
Rakibin veremeyeceği bir garantidir. **Not:** "bizde de tek tık var, adı
`composer create-project`" cümlesi ancak site-kullanıcısı kimliğiyle çalışan komut
ucu **yayınlandıktan sonra** kullanılabilir; bugün yoktur.

**Asıl iş katalogda değil.** Bu karar neyi yapmayacağımızı sabitler; boşluk
L3'tedir — bugün Laravel'i katalogsuz da barındıramıyoruz:
- docroot `site_orchestrator.go`'da `public_html`'e sabit (Laravel `public/` ister)
- ~~vhost `.env`'i düz metin sunuyor~~ → **kapatıldı** (commit `0088c67`, aynı gün;
  ayrıca statik sitelerin ACME kırığı da bu düzeltmeyle giderildi)
- `composer`/`artisan` kod tabanında sıfır kez geçiyor (site kullanıcısı kimliğiyle
  komut çalıştıran uç yok)
- queue worker üç bağımsız engelle imkânsız (`RunAsUser: "www-data"` sabiti,
  `req.Port <= 0` reddi, `project_type == "node"` kilidi)

**Reddedilen alternatifler.** "Laravel" proje tipi (enum bir kez framework adı
taşıdığında katalog örtük doğar ve DB'de kalıcılaştığı için geri alınamaz) ·
panelin ürettiği iskelet/tarball (aaPanel donmuş sürümde, Plesk upstream
kırılmasında) · Softaculous tarzı "Frameworks" kategorisi (15 giriş: Bootstrap bir
CSS framework'ü, Kohana 2016'dan beri ölü, Symfony 2.3.42 EOL 2017 — hâlâ
kurulabiliyor) · `.env`'i alan alan şemalaştırmak (çok satırlı değerlerde
müşterinin dosyasını sessizce tahrip eder; Forge da Ploi de opak blob tutar —
meşru "akıllılık" diskteki gerçekliğe bakmaktır: `.env.example` varsa kopyala) ·
scheduler'ı birinci sınıf kavram yapmak (sıradan bir crontab satırıdır) ·
preset'i otomatik uygulamak (onay kullanıcıdadır; yoksa hayalet cron/unit doğar).

---

## D-010 · Katalogda tür ayrımı (servis/runtime/araç) + "kurulu-önce" varsayılan; PHP çoklu-sürüm Sury ile gerçek olur

*20 Temmuz 2026*

**Karar.** Üç bağlı karar, tek kayıtta:

1. **Katalog tek kalır, ama `ManagedService`'e `Kind` girer:** `service` (systemd
   daemon'ı), `runtime` (sürümlü yorumlayıcı), `tool` (daemon'suz web aracı).
   PHP-FPM ve Node.js aynı türün iki üyesi olur; phpMyAdmin/phpPgAdmin "servis"
   olmaktan çıkar. Satır çizimi `Kind`'e dallanır ve bugünkü
   `Daemonless = len(SystemNames)==0` sezgisi silinir — o bayrak bugün üç ayrı
   şeyi işaretliyor. **Sürümler satır değil, satırın içindedir** (sürüm çekmecesi).
   Ayrı `/runtimes` ya da `/apps` sayfası **açılmaz**.
2. **Servisler sayfası "kurulu-önce" olur:** "kurulu olmayanı gizle" varsayılan
   AÇIK. Katalog = aynı listenin süzgeci kapalı hâli (ikinci ekran, ikinci arama
   kutusu yok). *Uygulama düzeltmesi (20 Tem, aynı gün): katlama sabit varsayılan
   değil, görünümü izler* — kurulu görünümde kategoriler AÇIK (liste zaten kısa;
   katlamak 3 servisi görmek için 3 tık demekti), katalog açılınca KATLI (liste
   uzun, yönetilmek ister). ✅ uygulandı: commit `312e378`.
3. **PHP çoklu-sürüm Sury vendor deposuyla gerçek olur** (D-007'nin PHP'ye
   uygulanması): Debian/Ubuntu'da yan yana `php8.x-fpm` paketleri panelden
   kurulabilir. Arch'ta temiz yol yoktur (yalnız AUR) — orada dürüstçe "tek
   dağıtım sürümü" denir (D-004: apt birinci sınıf, pacman geliştirme-test).

**Neden.** Operatörün sorusu şuydu: "Node.js/Go gibi seçenekler yok; buraya mı
eklenmeli, sayfa çok uzamaz mı?" Beş rakip panelin gerçek dokümantasyonu
incelendi ve iki şey çıktı:

- **Sayfa uzunluğu bir taksonomi sorunu değil, varsayılan sorunudur.** Sayfa
  uzunluğu katalog boyutuyla değil **kurulu kalem sayısıyla** ölçülmeli; süzgeç
  varsayılan açık olunca temiz sunucuda ekran 3-4 satırdır, katalog 19 da olsa
  40 da olsa. Bu, anayasanın "kurulu olmayan servis arayüzde görünmezdir"
  maddesini bir onay kutusu olmaktan çıkarıp ürün mekaniğine çevirir.
- **Liste patlamasının kaynağı sürümleri satır yapmaktır.** Plesk'in "PHP
  interpreter versions (2 of 12 selected)" ağacı bunun kanıtı. Dil başına tek
  satır + içinde sürüm yöneticisi bu patlamayı kaynağında keser.

Kaçındığımız somut hatalar: **cPanel**'in Service Manager / Application Manager
ikiliği (kullanıcı hangi ekranın hangi soruyu cevapladığını ezberler);
**Plesk**'in tutarsız runtime modeli (Node Linux'ta eklenti, Windows'ta bileşen;
Ruby CLI'da; Python onay kutusu); **HestiaCP**'nin 5050 numaralı hatası (N sürüm
satırının "Edit"i sürümsüz tek sayfaya gider — sürüm uçlarımız parametreli olur);
**aaPanel**'in PM2 çiftlemesi (tek yetenek, iki giriş noktası — bizdeki karşılığı
`AdminNodeInstall`'dır, silinir: runtime kurulumunun tek adresi Servisler'dir).

**Go/Java runtime olarak hedef-dışıdır.** Go derlenip tek binary olur; kurulacak
runtime yoktur, doğru destek zaten `proxy` proje tipi + systemd unit'tir.
Katalogda "Go" satırı kavramsal hata olurdu. Java/Tomcat ayrıca WAR dağıtım
modeliyle panelin geri kalanına uymaz. Ruby, PM2, Supervisor, Docker,
Elasticsearch, MongoDB/RabbitMQ/Varnish de hedef-dışı kalır.

**Bağlı üç alt karar** (aynı deseni 5 dilde tekrarlayacağımız için burada sabitlenir):
- **"Sistem yorumlayıcısı" kaçağı kaldırılır:** panel yalnız kendi kurduğu
  runtime'la çalışır (PHP'deki disiplinin aynısı). PATH'teki `node`u kurmadan,
  güncellemeden, kaldıramadan çalıştırmayı kabul etmek görünmez bağımlılık üretir.
- **Web sunucusuz Node projesi reddedilir** (php/static ile simetrik, B1 hata
  sözleşmesiyle kodlu ret + eylem düğmesi): bugün unit kalkıyor ama reverse proxy
  olmadığı için alan adı açılmıyor — kullanıcı yeşil durumla bozuk siteyi aynı
  anda görüyor.
- **Kullanımdaki sürüm/servis kaldırılamaz:** kodlu ret + engelleyen site listesi
  (`RUNTIME_IN_USE`, `SERVICE_HAS_DEPENDENTS`). Toplu sürüm taşıma ayrı iştir (v0.3).

**D-002 düzeltmesi.** D-002 "PHP — yan yana paketler, site başına seçilir ✅
yapıldı" diyordu; kod bunu yalnız yarısıyla karşılıyor: **tespit ve site başına
seçim yapıldı, çoklu sürüm KURULUMU yapılmadı** — `php-fpm` katalog girdisinde
`Repo` yok, kod tabanında `sury`/`ondrej` geçen tek satır yok. Yani panel çoklu-PHP
arayüzü taşıyor ama tek sürüm sunabiliyor: "seçici var, seçenek yok". Bu karar o
boşluğu kapatır.

**Uygulama düzeltmeleri (21 Tem).** Kodu yazmak kararın iki yerini
inceltti; ikisi de kayda geçer:

- **`Kind` durumu değil, DENETİMİ belirler.** "Satır çizimi Kind'e dallanır"
  ilk okunuşta "runtime'ın da çalışıyor/durdu durumu yok" demeye geliyordu.
  Yanlış: `php-fpm`'in gerçek unit'leri (`php8.3-fpm`…) vardır ve tarama onları
  toplar. Ölü bir php-fpm her PHP sitesini kırar; panodan alarmı kaldırmak
  körlük olurdu. Doğru ayrım ikilidir: **durumdan yalnız `tool` muaftır**
  (bize ait daemon'ı yoktur); **satır içi başlat/durdur ise yalnız
  `service`indir** — runtime'ın sürüm başına unit'i olduğu için tek bir
  "Durdur" düğmesi "hangi PHP?" sorusuna yalan söylerdi; onun denetimi sürüm
  çekmecesindedir. Saha kanıtı: `nftables` (tool) unit'i "inactive (dead)"
  görünürken güvenlik duvarı iki sunucuda da açık ve 12 kural yüklüydü — eski
  davranış çalışan bir firewall için "1 servis durdu" yanlış alarmı üretiyordu.
- **Katalog gerçeği önbelleğe alınmaz** (A7). Tür ayrımı ilk denemede
  `kind:""` olarak yayına gitti: `service_scan_cache` tüm API yanıtını, katalog
  alanları dahil saklıyordu. Kural artık açıktır: **önbellek yalnız taramanın
  keşfettiğini saklar** (kurulu mu / unit ayakta mı / sürümler / config'ler);
  ad, açıklama, ikon, kategori, paket adları ve `Kind` her okumada koddan
  birleştirilir. Bu, D-010'un "katalog tek kalır" cümlesinin çalışma-zamanı
  karşılığıdır — tek sahiplik, saklanan bir kopyayla birlikte var olamaz.

**Reddedilen alternatifler.** Ayrı `/runtimes` sayfası (ikinci ekran = cPanel'in
ikiliği) · ayrı `/apps` sayfası (Node satırının altındaki sayaç+liste yeterli;
30 uygulamayı aşarsa yeniden tartılır) · sunucu rolü/profil UI'ı (`Role` alanı
veri modeline şimdi girer ama filtre 25 kalemden sonra tartılır) · toplu onay
planlayıcısı (bağımlılık zinciri 2 kalemden derinleşmedikçe yazılmaz) ·
**"tür ayrımından sonra bir kez Tara'ya basılsın"** (A7'yi düzeltmek yerine
üstünü örterdi: hata `kind`'e özgü değildi, her katalog düzeltmesinde geri
gelirdi).

---

## D-009 · DNS sunucusu yoksa domain de yok: panel, domain'lerinin otoritesidir

*9 Temmuz 2026*

**Karar.** Bir domain ancak DNS sunucusu (PowerDNS/BIND) kuruluyken
eklenebilir ve domain'ler varken DNS sunucusu kaldırılamaz. CelikPanel
sunucusundaki her domain'in DNS'ini o sunucunun kendisi sunar — "ya da DNS'i
dış sağlayıcıda yönet" dalı yoktur.

**Neden.** Operatörün ya/ya-da mesajlarıyla yaşadıktan sonraki hükmü: "dns
yoksa domain de olmamalı — kafa karıştırıyor." Hiçbir şeyin sunmadığı bir
kayıt listesi, özellik kılığında bir tuzaktır; tek pencerede iki ayrı zihinsel
model, bir fazladır. Tek kural anında okunur: önce DNS kur, sonra domain. Bu,
D-008'in ilk-gün notlarındaki "DNS dışarıda da olabilir" duruşunu bilinçli
olarak geçersiz kılar — dış DNS, gerçek bir kullanıcı isterse ileride açık bir
gelişmiş mod olarak dönebilir; alfa tutarlılık için eniyilenir.

**Nasıl duruyor.** Ön-denetim her domain tipinin oluşturulmasını eyleme dönük
bir 409 ile engeller (pencere de tüm seçenekleri tek ve net bir bantla
kapatır); ayna koruması, herhangi bir domain varken dns-server grubu üyesinin
kaldırılmasını reddeder — yoksa her domain sessizce kararırdı; kuralın
önlediği tuzağın ta kendisi. Tüm "ya da dışarıda yönet" metinleri kaldırıldı.
Canlı kanıt: DNS'siz oluşturma → 409.

**Ek (26 Temmuz 2026; 18 Ağustos'ta geçersiz kılındı — iki sunuculu yetki).**
Eski, yalnız-PowerDNS ve zone-başına çift-yazarlı model reddedildi; çünkü karışık
BIND/PowerDNS eşinde yönlü ikincilde oluşturulan bir zone birincile geri
yakınsayamaz. **Eşli** artık makine yönlüdür: bir birincil paneldeki tüm yerel DNS
mutasyonlarının sahibidir ve motor-bağımsız Catalog Zone v2 yayınlar; bir ikincil
bu kataloğu tüketir ve yerel yazılara kapalıdır. Oluşan zone'lar için iki
nameserver da yetkili cevap verir. İki panel aynı ortak ad çiftini taşır; adlardan
tam biri yerel IP'ye, diğeri eş IP'ye çözülür. Glue yalnız bu adların sahibi olan
üst domain'de bir kez kaydedilir; müşteri domain'i altında child nameserver
üretilmez. Kesin NS adları, adresleri ve yönlü roller motor kurulmadan önce
hazırlanır ve ilk motor etkinleştirildiğinde değişmez olur.

**Ek (18 Ağustos 2026 — doğrudan yönlü DNS motoru sahipliği).** PowerDNS ile BIND,
operatörün bağımsız olarak başlatıp durdurduğu veya kaldırdığı sıradan bileşen
satırları değil, panelin sahip olduğu tek DNS-motoru yaşam döngüsünün iki
seçeneğidir. TCP ve UDP 53 numaralı portun sahibi aynı anda yalnız bir motordur.
Etkin olmayan alternatif kurulu kalabilir; geri dönüş için durmuş bekleme
kopyasıdır.

Motor seçimi bilinçli olarak iki adımdır. İlk eylem; kesin işlem türünü,
topolojiyi, zone ve DNSSEC sayılarını, beklenen kesintiyi, etkileri ve kodlu
engelleri gösteren salt-okunur bir önizleme alır. Ayrı onay aynı tek-kullanımlık
önizleme yetkisini, kaynağı, hedefi ve durum revizyonunu sunmak zorundadır; bayat
veya engelli önizleme sunucuyu değiştiremez. Canlı motor geçişinde ayrıca anlamlı
bir kesinti onayı gerekir; sürüm ya da ürün adını yazdıran bir parola oyunu yoktur.

PowerDNS↔BIND değişimi hem **Tek sunucu** hem doğrulanmış **Eşli** topolojide
çift yönlüdür. Topoloji ile motor seçimi bağımsızdır: birincil ve ikincil
PowerDNS/PowerDNS, BIND/BIND, BIND/PowerDNS veya PowerDNS/BIND kullanabilir ve
iki rolde de ilk etkin motor doğrudan ikisinden biri olabilir. Önce PowerDNS ile
bootstrap ya da geçici motor adımı yasaktır. Kayıtlı NS kimliği değişmez birincil
ve ikincil rolleri kesin olarak belirler. Birincil motor-bağımsız Catalog Zone v2
yayınlar; ikincil kesin kataloğu ve her üye zone'u standart AXFR/NOTIFY ile
aktarır.

Birincil motorun eş henüz yokken etkinleşmesine izin verilir; fakat sahiplik ile
yayına hazır olma ayrı gerçeklerdir: motor kesin yönetilen ve çalışır halde
**eş-bekliyor** kalır, paneldeki tüm yerel zone yazıları engellenir. PairReady
ancak şu bağımsız otoritelerin tümü uyuştuğunda yayını açar:

- yerel katalog AXFR'si kalıcı seri ve sıralı üyelikle birebir eşleşir;
- yapılandırılmış yerel eş adresine kaynak olarak bağlanan eş katalog AXFR'si
  aynı seri ve üyeliği döndürür;
- eş kataloğu bu yetkili SOA serisini hem UDP hem TCP üzerinden döndürür; ve
- her üye zone kalıcı beklenen yetkili SOA serisini yerelde ve eşte UDP ve TCP
  üzerinden döndürür.

Bu kanıt üye zone sayısı sıfırken de boşuna doğru olamaz. Silme ancak eşteki zone
AXFR'si bulunmadığında tamamlanır; aktarım hâlâ başarılıysa eş bayat kopya
sunuyordur ve PairReady başarısız olur. İkincile hiçbir zaman panelde yerel yazma
yetkisi verilmez.

Yönetilen BIND genel seçenekleri varsayılan olarak `allow-transfer { none; };`
kullanır. Yönlü BIND birincilin üretilen katalog ve üye zone'ları AXFR'ye yalnız
güvenilir yerel kanıt için kesin `LocalIP` ile kesin eşten izin verir ve yalnız
o eşi bilgilendirir. BIND ikincil yalnız kesin birincil `/32` adresini kabul eder.
Yönlü PowerDNS birincil exact `allow-axfr-ips=LocalIP,PeerIP` ile
`also-notify=PeerIP` kullanır; yönlü ikincil yalnız eş AXFR'sine izin verir ve
`also-notify` içermez. Yayımlanmış eski eşli politika yalnız dar eski sürüm
kanıtında bayt-birebir peer-only-plus-notify uyumluluğudur. İlk başarılı BIND V3
yayını bunu nesil pointer'ı/durum geri dönüş işlemiyle birlikte taşır. Eski
PowerDNS sessizce taşınmaz; açıkça incelenmiş switch ya da yeniden yapılandırmaya
kadar uyumluluk kanıtında kalır.
Eski V2 mutasyon girişleri yalnız exact tuple'sız üretici/standalone
uyumluluğuna açıktır. Yönlü receipt'ler ile tuple'sız consumer'lar salt
okunurdur ve incelenmiş V3 switch/yeniden yapılandırma yolunu gerektirir.
Yayımlanmış dolu bir consumer, PowerDNS'ten ancak exact katalog-bağlı üye kümesi
ile yerel/eş SOA serileri kanıtlandıktan sonra ayrılabilir.
Sabit ve özel eş IPv4 zorunludur. TSIG henüz
yoktur; paylaşılan veya dinamik NAT uçları desteklenmez.

Katalog serisi motora yerel durum değil, topoloji otoritesidir. Birincil
BIND↔PowerDNS geçişi bu değeri birebir korur. Üyelik ekleme, silme veya yeniden
ekleme seriyi bir kez ilerletir; yalnız kayıt güncellemesi ilerletmez. En büyük
seriyi ilerletmek bayat durumu güncelmiş gibi gösterecek biçimde saracağı için
taşma yayından önce fail-closed reddedilir. Yayımlanmış
`v0.1.0-alpha.27` kaynak receipt'i seri alanından eskidir. Böyle bir receipt
yalnız kesin kalıcı durum ile canlı kaynak backend bağımsız olarak aynı pozitif
katalog serisini ve üyeliği verdiğinde yükseltilebilir; türetilen değer yeni
geçiş günlüğüne ve hedef receipt'ine bağlanır. Eksik veya farklı kanıt geçişi
engeller.

DNSSEC zone'ları, bekleyen zone yayını, panel dışı DNS, 53 portu çakışması, bozuk
kaynak veya başka bir işlem değişimi ya da yeniden yapılandırmayı engeller.
İşlem; motor ve etkinleştirme dönemine bağlanmış tam zone görüntüsünü dondurur,
kaynağı durdurmadan önce hedefi hazırlar ve doğrular, uygulanabilir her zone'u
hem UDP hem TCP üzerinden sınar ve eski paketi durmuş bekleme kopyası olarak
tutar.

Eski sürümden kalan ve motor durumu henüz çözümlenmemiş bir host için dar
**mevcut panel-yönetimli PowerDNS otoritesini yalnız kayıt amacıyla devralma**
yolu kalır. Devralma; kesin config baytları ve kipleri, unit durumu, SQLite
verisi, sahip olunan zone'lar, topoloji ve TCP/UDP otoritesini değiştirmeden
doğrular; BIND için devralma kestirmesi yoktur. Sabit Boston akışı daha sonra
mevcut **boş, yönetilen Tek-sunucu PowerDNS** durumunu inceler ve aynı motoru
doğrudan yönlü ikincil olarak yeniden yapılandırır. Kesin veritabanı/config/unit
anlık görüntüsünü alır, katalog-tüketici hedefini yazar, sınırlı yeniden
başlatmayı yapar, hedefi kanıtlayıp tamamlar veya anlık görüntüyü geri getirip
kanıtlar. Geçici BIND otoritesi kurmaz veya etkinleştirmez. Boş olmayan, DNSSEC
taşıyan, yönetilmeyen ya da farklı eski durum sessizce yeniden amaçlandırılmak
yerine fail-closed reddedilir.

Hem panel defteri hem agent'ın sahip olduğu host günlüğü işlem kimliğini,
değişmez eş kimliğini, anlık görüntüyü ve aşamayı taşır. Kesinti sonrasında
kurtarma kesin hedefi kanıtlayıp tamamlar veya işlem öncesi dosyaları,
veritabanını, nesil işaretçisini ve unit durumunu birebir geri getirip kanıtlar.
İkisi de kanıtlanamazsa DNS mutasyonları kurtarma için kilitli kalır; panel hangi
daemon'ın otorite olduğunu tahmin etmez.

---

## D-008 · Alfa: paneli operatör sürer; her boşluk bir ürün özelliğine dönüşür

*9 Temmuz 2026*

**14 Eylül açıklaması.** Panelden ilerleme kuralı ürün kabul disiplinidir;
sunucu sahibinin kendi araçlarıyla yönetim yetkisini sınırlamaz (D-022). Kurulu
sunucuda 13 Eylül kullanıcı kurtarma kuralı geçerlidir: önce panel, panel yeterli
değilse kısa ve doğrulanmış kullanıcı komutları. Bu, gizli asistan müdahalesine
veya kurulu paneli asistanın güncellemesine izin vermez. Elle kurtarma, ilgili
ürün akışının tamamlandığının kanıtı değildir. Aşağıdaki ilk karar tarihsel
bağlamıyla ve bu açıklamayla okunmalıdır.

**Karar.** Debian 13 yeniden kurulumundan itibaren operatör, CelikPanel'i
gerçek bir müşteri gibi kullanır: her kurulum, her ayar, her domain panelden
ve kendi eliyle geçer. Geliştirici sunucuyu asla yapılandırmaz — izinle bile.
Operatör bir duvara çarptığında suç ürünündür: eksik yetenek panele eklenir
(ya da bozuk olan düzeltilir) ve ürün güncellemesi olarak yayınlanır. SSH ile
teşhis salt-okunur kalır.

**Neden.** Sunucuyu SSH ile elle onarmak, ürünü olduğundan bitmiş gösterir —
boşluk üründen değil görüş alanından kaybolur. Bu, D-005 ile (hiçbir şey
kurma) başlayıp "sormadan asla kurma" ile keskinleşen çizginin son
basamağıdır: geliştirici artık *hiçbir şey* kurmaz ve ayarlamaz. Alfa
kalitesi duvarlara bilerek yürümekten gelir.

**İlk günde nasıl işledi.** Bir öğleden sonralık gerçek operatör kullanımı
sırayla şunları çıkardı ve düzelttirdi: domain'in yanlışlıkla PHP zorunlu
sayması (oluşturma yolu tip sisteminden eskiydi) → php/statik/**yalnız-DNS**
tipli, canlı önkoşul denetimli rol-farkında Alan Adı Ekle (Plesk'in "no web
hosting" karşılığı); kurulu olmayan servislerin hayalet ayar sayfaları → her
sekme, motor listesi ve sürüm açılır kutusu artık sunucunun gerçek yetenek
kümesini okur (tek uç nokta: `/api/v1/hosting/capabilities`); ölü
phpMyAdmin/pgAdmin bağlantıları → genel bir `Requires` katalog kavramı
(çakışma gruplarının tersi): bağımlı araçlar üst servisleri kurulana dek
kilitlidir, kurulunca yalnız-loopback'te, admin-kapılı panel vekilinin
arkasında sunulur; panele gerçek sertifika almanın yolu yoktu → Ayarlar'da
tek tık Let's Encrypt + otomatik yenileme; ve en sessiz, en büyüğü — panel
`index.html`'i önbellek başlıksız veriyordu; tarayıcılar her güncellemeden
sonra ESKİ arayüzü göstermeye devam etti. Operatör, çoktan düzeltilmiş
hayaletleri gördü; artık giriş noktası `no-cache`, parmak izli asset'ler
immutable.

---

## D-007 · Sürüm seçimi, dağıtımı fork'lamadan yönetilen vendor depolarıyla

*9 Temmuz 2026*

**Karar.** Bir servis resmi bir yukarı-akış deposu bildirebilir (PostgreSQL →
PGDG). Onu açmak, kurulumda sürüm seçimini açan birinci-sınıf bir panel
eylemidir: dağıtımın dondurduğu tek major yerine, yönetici vendor'ın sunduğu
tüm güncel major'lardan seçer. Opt-in, seçilmiş, imzalı ve geri alınabilirdir —
asla varsayılan açık değil.

**Neden.** Bir dağıtım sürümü her veritabanı/çalışma zamanının tek major'unu
sabitler (Ubuntu noble yalnız PostgreSQL 16; Debian bookworm yalnız 15).
Müşteriler belirli bir sürüme inip orada kalmak ister — bu, operatörün açık
kaygısıydı ("PG'nin bir sürü sürümü var; install hangisini kurar, bizi tek
sürüme kilitler mi?"). İki kötü yanıt: *dağıtımı fork'la* (tüm paket ağacına
sonsuza dek sahip ol) ya da *OS karar versin* (hiç seçim yok). Sektör-standardı
orta yol, tüm güncel major'ları yan yana taşıyan vendor'ın kendi imzalı apt
deposudur.

**Minimal kurulumla (D-005/D-006) neden tutarlı.** Üçüncü-parti depo güven
sınırını genişletir; bu yüzden servis kurmakla aynı disipline tabidir: yalnız
katalogda tanımlı depolar açılabilir (UI asla URL geçirmez); imza anahtarı depo
başına apt `signed-by=` ile sabitlenir (küresel `apt-key` güveni yok); kapatmak
kaynağı + anahtarı temizce kaldırır. Zırhlı anahtar doğrudan kullanılır, böylece
**`gpg` çekilmez** — minimal ayak izi korunur. Sürüm seçimi deponun paket
desenine bağlıdır; asla keyfi paket kurulumuna dönüşemez.

**Nasıl doğrulandı.**
- Katalog: `ManagedRepo` (id, ad, anahtar URL'si, `{codename}` yer tutuculu
  kaynak şablonu, paket deseni); `postgresql` PGDG taşır. Agent `EnableRepo`/
  `DisableRepo`/`RepoStatus`/`RepoPackages`; hangi sürümlerin var olduğunun
  kaynağı kodumuz değil depodur (`apt-cache` ile keşfedilir, en yeni major önce).
- Panel `GET/POST /api/v1/repo` (yalnız admin, audit'li); izin listesi burada
  (depoyu katalogdan servis id'siyle çöz). Kurulum, bir sürüm paketini yalnız o
  servisin depo desenine uyuyorsa kabul eder.
- Üretim VPS'te kanıtlandı: baseline **yalnız `postgresql-16`** sunuyordu; PGDG
  açılınca **9 major (10–18)** açıldı ve `postgresql-17` seçimi PGDG'den
  `17.10-1.pgdg24.04+1`'e çözüldü; kapatmak baseline'a döndürdü.
- Testte yakalanan hata: agent `UMask=0027` ile koşar; `0644` keyring
  `0640 root:celikpanel` olarak indi — apt'ın yetkisiz `_apt` doğrulayıcısı
  okuyamadı, "imzasız" hatası verdi. Yazdıktan sonra `chmod 0644` ile düzeltildi.

**Düzeltme (21 Tem) — ikinci vendor, tek-vendor varsayımlarını ortaya çıkardı.**
Bu karar tek bir depoyla (PGDG) yazıldı ve uygulandı; PHP için Sury eklenince
üç varsayımın PGDG'ye özgü olduğu görüldü. Genel kural: *bir mekanizma tek
örnekle doğrulandığında, örneğe özgü olanı genel sanır.*

1. **"KeyURL zırhlı `.asc`'dir" yanlıştı.** Yukarıdaki metin bunu bir gerçek
   gibi yazıyor ("Zırhlı anahtar doğrudan kullanılır"). PGDG zırhlı yayınlar,
   **Sury ikili keyring yayınlar** (`0x99` = OpenPGP açık anahtar paketi;
   sunucuda ölçüldü) ve `.asc` varyantı yok (418). `fetchArmoredKey` onu
   reddederdi — yani karar, kendi kapsamındaki ikinci vendor'ı dışarıda
   bırakıyordu. Artık iki biçim de kabul edilir ve dosya **içeriğine göre**
   adlandırılır (`.asc`/`.gpg`); apt ikisini de doğrudan okur, gpg bağımlılığı
   yine çekilmez. Kabul, paket tag'i (tag 6) denetlenerek sınırlıdır — bir HTML
   hata sayfası güvenilir keyring olamaz.
2. **Sürüm sıralaması sondaki tam sayıyı okuyordu.** `postgresql-17` için 17
   verir; `php8.3-fpm` için 0 ("fpm" sayı değil). Tek vendor'da doğru görünen
   kod, PHP'de tüm sürümleri berabere bırakıyordu — çekmece sırası apt-cache ne
   bastıysa o oluyordu. Artık `(major, minor)` okunur.
3. **"Zaten kurulu" duvarı.** Kurulum, sürüm seçimine bakmadan servisin kurulu
   olup olmadığını soruyordu. PostgreSQL'de zararsızdı (tek küme); PHP'de
   ölümcül: 8.4 varken 8.3 isteği "PHP-FPM zaten kurulu" diye reddediliyordu.
   Düz kurulumda soru "bu servis burada mı", sürüm seçiminde "**bu sürüm**
   burada mı" olmalıdır (D-014'ün zincirini fiilen engelleyen şey buydu).

Ayrıca **`VersionCompanions`** eklendi: bir sürüm seçimi çıplak kurulur
(`php8.3-fpm`in tek başına veritabanı sürücüsü, mbstring'i, curl'ü yoktur) ve
panel, site sunamayacak bir runtime için başarı bildirirdi. PGDG'de bu ihtiyaç
yoktu çünkü `postgresql-17` tek başına çalışır — tek-vendor körlüğünün dördüncü
yüzü. Companion'lar en-iyi-çabadır ve atlanan **raporlanır** (Sury
`php8.5-opcache` yayınlamıyor; katı kurulum tek uzantı yüzünden 8.5'i tümüyle
reddederdi).

---

## D-006 · Saldırı yüzeyi yönetimi: her kurulum geri alınabilir

*8 Temmuz 2026*

**Karar.** Panel neyi kurabiliyorsa, onu kaldırabilir de. Servis kaldırma
(durdur + devre dışı + `apt purge --auto-remove`) kurulumun yanında birinci
sınıf bir eylemdir, elle SSH işi değil. Bu, üç parçalı "saldırı yüzeyi"
hattının ilkidir: **geri alınabilir kurulumlar**, otomatik güvenlik yaması ve
yalnız kullanılan portları açan bir güvenlik duvarı.

**Neden.** Operatörün ilkesi, açıkça: *kurulu her servis ya da paket bir
güvenlik riskidir — bekleyen bir CVE, açık bir port.* Minimalizm derli
topluluk değil, daha küçük saldırı yüzeyidir. Ama minimalizm ancak yüzey hem
büyüyüp hem küçülebiliyorsa gerçektir: nginx ekleyip asla kaldıramıyorsan,
yanlış ya da artık gereksiz bir kurulum kalıcı risktir. O yüzden kurulumun bir
aynası var.

**Nasıl korunuyor.**
- `Agent.UninstallService`: her unit'i durdurur + devre dışı bırakır, sonra
  `removePackages` purge eder (config gider, `--auto-remove` öksüz bağımlılığı
  düşürür). `InstallService`'in aynası; aynı whitelist, aynı dürüst "bu
  dağıtımda henüz yok".
- Panel `POST /service/uninstall` (yalnız admin, audit'li); UI tam paket
  listesini ve bağımlı site/postanın duracağı uyarısını içeren yıkıcı-onay
  modalı gösterir.
- Üretim VPS'inde kanıtlı: SpamAssassin kuruldu (dpkg var) sonra kaldırıldı
  (dpkg yok, unit gitti) — yüzey ölçülebilir şekilde küçüldü.

**Bu hattın ilerlemesi (tamam):** geri alınabilir kurulumlar ✅ · otomatik
güvenlik güncellemeleri ✅ · güvenlik duvarı ✅. Duvar varsayılan-reddet
gelen; panel tam olarak panel portu + kurulu her servisin bildirdiği portları
açar, agent ise SSH'ı (canlı dinleyicilerden tespit) + loopback + kurulu
bağlantıları daima açık tutar; bir kural operatörü asla kilitleyemez.
Kurulum/kaldırma yeniden senkronlar; açık-port kümesi daima koşan-servis
kümesine eşittir. VPS'te kanıtlı: açınca SSH, panel, web ve DNS erişilebilir
kaldı, politika gerisini düşürdü.

---

## D-005 · Kurulum panelden başka hiçbir şey kurmaz; sunucu yalnız-panel olabilir

*8 Temmuz 2026*

**Karar.** `install.sh` yalnız panel + agent'ı (kendine yeten Go binary'leri +
SQLite) ve dört minik indirme aracını (tar, xz, curl, ca-certificates) kurar —
barındırma için **hiçbir şey**. nginx, PHP, MariaDB, Postfix, PowerDNS, posta:
hepsi sonradan, panelden, talep üzerine eklenir. Bir sunucu yalnız-panel
kalabilir (örn. sadece yerleşik VPN isteyen biri) ve hiç domain olmadan IP
üzerinden yönetilebilir.

**Neden.** Anayasa: "kurulu olmayan görünmez." Taze kurulmuş bir sunucu, panel
dışında sıfır saldırı yüzeyi ve sıfır fazlalık taşır. Operatör tam istediği
sunucuyu kurar — tam bir barındırma kutusu, ya da sadece VPN ucu, ya da sadece
DNS. İstenmeyen hiçbir şey asla bulunmaz.

**Nasıl korunuyor.**
- Doğrulandı: `install.sh` yalnız `apt-get install tar xz-utils curl
  ca-certificates` koşar. Üretim VPS'inde nginx/PHP/vb. yalnız golden path'i
  kanıtlamak için *panelden* kurulduğu için var — çıplak kurulumda hiçbiri yok.
- Domainsiz erişim tasarım gereği çalışır: panelin self-signed sertifikası
  makinenin IP'lerini SAN'a koyar (`tmpl.IPAddresses = hostIPs()`), böylece IP
  erişimi eşleşen bir sertifika sunar (yalnız self-signed uyarısı, asla isim
  uyuşmazlığı). Domain + Let's Encrypt bir yükseltmedir, asla zorunluluk değil.

---

## D-004 · İşletim sistemi: önce Ubuntu LTS, BSD seçeneği korunur (asla fork)

*8 Temmuz 2026*

**Karar.** Ubuntu 24.04 LTS birinci sınıf, yalnızca test edilen hedeftir. BSD
**şimdilik** bilinçli bir hedef-dışıdır — ama ileride desteklemek seçeneği
mimariyle korunur ve **asla** ayrı bir ürün olmaz.

**Neden Ubuntu, BSD değil.**
- Ürünün tamamı Linux'a özgü parçaların üstünde: systemd (agent'ın kalbi —
  servisler, `celikapp-*`, `wg-quick@`), apt + paket whitelist'leri, nftables
  (VPN NAT) ve postfix/dovecot/opendkim'in Ubuntu yerleşimi. OS değiştirmek =
  agent'ın "ellerini" yeniden yazmak, kullanıcıya sıfır fark için aylarca iş.
- Kullanıcının beğendiği kararlılık ("versiyonlar kilitli, kararlı") zaten
  **Ubuntu LTS'nin kendisi**: 5 yıl yalnız güvenlik yaması. BSD, elimizde
  olmayan bir kararlılık katmaz.
- Pazar gerçeği: hiçbir rakip BSD'de değil (cPanel = RHEL ailesi, Plesk =
  Linux+Windows, aaPanel = Linux). VPS imajları, müşteri alışkanlıkları,
  WordPress/PHP ekosistemi, Let's Encrypt araçları hep Linux-öncelikli.
  Müşteri "WordPress'im çalışıyor mu, mailim spam'e düşüyor mu?" diye sorar —
  "BSD'de misiniz?" diye değil.

**Seçeneği ucuz tutan şey — ve neden fork DEĞİL.**
- Panel↔agent RPC ayrımı (güvenlik için kuruldu) aynı zamanda OS
  bağımsızlığıdır. **Panel** (HTTP, SQLite, UI, iş mantığı) taşınabilir Go;
  OS'e dokunan tek katman **agent**. Panel *ne* yapılacağını der ("site kur");
  *nasıl*ını yalnız agent bilir (systemctl/apt/nftables).
- İddia değil kanıt: bu tarih itibarıyla `make freebsd-cross` **hem** paneli
  hem agent'ı derliyor. Tek bir taşınabilirlik düzeltmesi gerekti (Statfs_t
  alan tipleri Linux ile BSD'de farklı — `cmd/panel/system_stats_disk_unix.go`'da açık
  `uint64` dönüşümleri) ve yapıldı. Kod tabanının tamamı bugün FreeBSD'ye
  çapraz derleniyor.
- İki CelikPanel, kullanıcının korktuğu hantallığın ta kendisi olurdu: her
  özellik iki kez, her hata iki kez, iki vasat ürün. Bunun yerine olası bir
  BSD geleceği = aynı RPC yüzeyinin arkasına bir **BSD agent arka ucu** (rc.d,
  pkg, pf). Tahmin: yıllar değil haftalar — tüm yığın (nginx, postfix,
  dovecot, opendkim, PowerDNS, MariaDB, WireGuard) FreeBSD ports'ta var;
  yalnız "eller" değişir. Panel, UI, DB, iş mantığı: sıfır değişiklik.

**Bugün ne yapıyoruz.** Neredeyse hiçbir şey — ve asıl nokta bu. Tek bedel
disiplin: yeni agent özellikleri *ne*yi (RPC yüzeyi) *nasıl*dan (exec
çağrıları) ayrı tutar, ki kod zaten böyle yazılıyor. Spekülatif soyutlama
katmanı yok, "belki lazım olur" kodu yok. Karar, gerçek talep doğduğunda
verilir (örn. Linux'ta bir güven/güvenlik krizi barındırıcıları BSD'ye
iterse).

**Diğer dağıtımlar.** `detectPkgFamily` apt / dnf / pacman tanır. Bugün yalnız
apt (Ubuntu/Debian) tam destekli ve test edilmiş; dnf/pacman tanınır ama
kurulum tahmin etmek yerine dürüstçe "bu dağıtımda henüz otomatik kurulamıyor"
der. Not: dnf module stream'leri (`postgresql:16` vs `:18`) orada gerçek bir
sürüm seçiciyi anlamlı kılar — dnf desteği eklersek UI buna hazır.

*Ek (16 Tem, operatör kararı):* Arch artık bir **geliştirme-test hedefi** —
taşınabilirlik sözünü dürüst tutmak için ikinci test sunucusu bilerek Arch'ta.
`install.sh` pacman'i destekler (ön gereksinimler, araç zinciri mimarisi
`uname -m` ile, dürüst "güvenlik-yalnız kanal yok" notu).

*Ek 2 (16 Tem, aynı gün genişletildi):* Operatör iki sunucuda aktif test
yaparken "apt'ye özgü katalog" duvarına üç kez çarptı (certbot kurulumu, bind
kaldırma, paket adı `bind9` gösteren onay penceresi). Karar: **agent'ın paket
katmanı pacman'i de sürer** (kur `-S --needed`, kaldır `-Rns`, varlık `-Q`;
asla `-Syu` — sürpriz sistem yükseltmesi yok). Katalogda pacman paket adları
yalnız eşlemenin kesin olduğu VE dağıtıma özgü init adımı istemeyen servisler
için dolu; MariaDB/PostgreSQL (initdb ister), phpPgAdmin (AUR-only) ve Redis
(Arch'ta Valkey çatalı) bilerek boş — dürüst "henüz desteklenmiyor" derler.
UI, paket adlarını agent'ın bildirdiği aileden gösterir (`Agent.PkgFamily`).

*Ek 3 (3 Ağu 2026):* Debian 13, Ubuntu 24.04'ün yanında açık bir apt kabul
hedefidir. Bu, çalıştırılmamış bir testi geçmiş saymaz: sürüm kabul edilmeden
önce etiketli, önceden derlenmiş ürün tam belgelenmiş operatör akışıyla temiz
Debian 13 kurulumundan geçmelidir. Kaynak checkout'ları, geliştiriciye özel
bundle'lar ve kurulum sonrası SSH düzeltmeleri kanıt sayılmaz.

*Ek 4 (23 Ağu 2026):* D-020, bu eski karardaki dağıtım/sürüm kapısının yerini
alır. Ubuntu 24.04 ile Debian 13 kabul örnekleri olarak kalır; yetkilendirme izin
listesi değildir. Eksiksiz sabit-yol yetenek sözleşmesi kanıtlandığında herhangi
bir Linux adı/sürümü APT veya pacman seçebilir. DNF yalnız SELinux yaşam döngüsü
yeteneği eksik olduğu için önizleme olarak kalır.

---

## D-003 · Servis kataloğu: her şeyi göster, çakışmayla engelle — "yönetilmiyor" ile değil

*8 Temmuz 2026*

**Karar.** Servisler sayfası (yalnız admin) tüm kataloğu listeler, kurulu olsun
olmasın, tek tıkla Kur ile. Derinlemesine yönetmediğimiz servisleri
**gizlemeyiz** ve "yönetilmiyor" etiketleriyle **karmaşıklaştırmayız**. Bir
kurulumu engelleyen tek şey **gerçek bir çakışma**dır.

**Neden.** Kullanıcının sezgisi: "yönetiyor muyuz?" üzerinden kapılamak
gereksiz karmaşıklık. Asıl kısıt, iki servisin yan yana çalışıp
çalışamayacağıdır.
- **Çakışma grupları** (katalogda `ConflictGroup`): aynı rol/portu tutanlar
  karşılıklı dışlar — :80'de web sunucusu (Nginx ↔ Apache), :53'te etkin DNS
  otoritesi (BIND ↔ PowerDNS). Genel bileşen kurulumu bu çakışmayı yine
  reddeder. Dar istisna olan özel DNS-motoru akışı, alternatifi başlamama
  bekçisi altında kurup doğrulanmış geri dönüş kopyası olarak durmuş tutabilir;
  iki motorun 53 numaralı portu birlikte sunmasına asla izin vermez.
- **Grup yoksa = yan yana**: MariaDB + PostgreSQL birlikte koşar (farklı
  portlar), ikisi de kurulabilir. Redis + Memcached de öyle.

**Sonuç.** Her kurulum admin'in açık, tek-tıklık onayıdır ("yalnız izinle kur"
ilkesine uygun). Katalog kategoriye göre akordiyonla gruplu (tümünü aç/kapat);
düzinelerce servise doğru büyüdükçe taranabilir kalır.

DNS motoru kurulumu ve değişimi bu genel kuraldan bilinçli olarak daha güçlüdür:
ilk tıklama salt-okunur incelemedir; kalıcı işlemi ayrı açık onay başlatır.
Bkz. D-009.

---

## D-002 · Servis sürümleri: dürüst tek sürüm, gerçek çoklu-sürüm yalnız var olan yerde

*8 Temmuz 2026*

**Karar.** Kurulum modalı apt'ın kuracağı **gerçek** sürümü gösterir (canlı
`apt-cache policy` ile okunur), sürüm *seçici* değil. Seçici yalnız birden
fazla sürümün gerçekten var olduğu yerde sunulur.

**Neden.** Bir Debian/Ubuntu sürümü her paketin tam olarak tek sürümünü verir
ve dondurur (yalnız güvenlik yaması) — kararlılık modeli bu, bilerek. Tek
seçenekli bir açılır liste, kullanıcıyı yanıltan bir yalandır. Dolayısıyla:
- **Tek-sürümlü servisler** (PostgreSQL, MariaDB, nginx…): ne ineceğini göster
  ("Kurulacak sürüm: 16"), seçici yok.
- **Gerçekten çoklu-sürüm, hosting-kritik** → dağıtımdan bağımsız, gerçek
  seçiciyle:
  - **PHP** — yan yana paketler (`php8.1-fpm`…`php8.4-fpm`), site başına
    seçilir. ⚠️ *Düzeltme (20 Tem, bkz. D-010): yalnız YARISI yapılmış.* Tespit
    (`DetectInstalledPHPVersions`) ve site başına seçim çalışıyor; ama katalogda
    `php-fpm` için `Repo` tanımlı olmadığından panel çoklu sürüm **kuramıyor** —
    dağıtımın tek sürümüyle sınırlı. Sury vendor deposu D-010 ile eklenir.
  - **Node.js** — dağıtımdan bağımsız resmi tarball'lar, proje başına seçilir.
    ✅ yapıldı.
- **Gelecek seçeneği**: Ubuntu 24.04'te örn. PostgreSQL 18 isteyen müşteri,
  üreticinin kendi apt deposunu (PGDG) bilinçli bir özellik olarak eklemek
  demektir — gerçek depo yönetimi, asla sahte seçici. Backlog'da, yapılmadı.

Desen: **dağıtımın tek sürümü = güvenli taban; gerçek çoklu-sürüm (PHP, Node)
kendi mekanizmamızla, dağıtımdan bağımsız.** cPanel/Plesk gibi (PHP'yi
kendileri derler, gerisini dağıtıma bırakır).

---

## D-001 · Güncelleme ve geri alma: sunucuyu asla sıfırdan kurma

*8 Temmuz 2026 · 28 Temmuz 2026'da değiştirildi*

**Güncel kurulu panel sınırı — 14 Eylül açıklaması.** Aşağıdaki bootstrap
komutları iç/tarihsel sürüm mekanizmasını belgeler; kurulu paneli SSH üzerinden
güncelleme izni vermez. 10 Eylül'den beri bütün kurulu panel güncellemelerini
kullanıcı CelikPanel'in güncelleme arayüzünden başlatır. Yayınlama ve kurma ayrı
eylemlerdir. D-025'in yeni yürütücüsü uygulanıp doğrulanana kadar, tam eşleşen
saklı sürümün geri alma yolu desteklenen kullanıcı kurtarması olarak kalır;
başka sürümün rollback scripti yerine konulamaz.

**Karar.** Üretim güncellemeleri yalnız
`sudo /bin/bash ./bootstrap-update.sh --normal` ile veya bir kerelik
`sudo /bin/bash ./bootstrap-update.sh --bootstrap-pre-ledger` geçişiyle başlar.
Bootstrap temiz ve incelenmiş commit'i dışa aktarır;
`/var/backups/celikpanel/releases/<commit>-<nonce>/` altında değişmez, root
sahipli bir release yayımlar. Güncellemeyi yalnız o release'in açık modla
çağrılan `update.sh` dosyası yapabilir. Ayrıcalıklı güncelleme `git pull`
çalıştırmaz ve değişebilir checkout içindeki updater veya installer'ı
çalıştırmaz.

Her güncelleme önce bir geri-alma snapshot'ı alır ve doğrular. Kurtarma yalnız
update çıktısındaki kesin komutu ve `VERIFIED_SNAPSHOT` değerini kullanır:
`sudo /bin/bash /var/backups/celikpanel/releases/<commit>-<nonce>/rollback.sh "$VERIFIED_SNAPSHOT"`.
Checkout'taki `rollback.sh` veya başka bir release'in
rollback scripti kurtarma yolu değildir. Servis mutasyon ledger'ı asla elle
oluşturulmaz, değiştirilmez, boşaltılmaz veya migrate edilmez; yalnız ürünün
kontrollü tek seferlik initializer'ı pre-ledger kurulum akışında onu
oluşturabilir.

**Neden.** Kutuda müşteri olması, verinin her güncellemeden sağ çıkması
gerektiği anlamına gelir. Müşteri verisi (SQLite DB, site dosyaları, posta,
DNS, sertifikalar, DKIM anahtarları) değişen yolların (`bin/`, `web/`) dışında
yaşar. Anlık görüntüden sonra güncelleme, şema geçişlerini iki koordinatör de
dururken staged panelin `--migrate-only` modu üzerinden çevrimdışı çalıştırır;
bu adım tamamlanmadan hiçbir unit yeniden başlatılamaz. Anlık görüntü (panel DB +
binary'ler + unit dosyaları + kaynak commit'i) "bir güncelleme işleri bozdu"yu
felaket değil, kurtarılabilir bir olay yapar. Düzeltme akışı: dev kutusunda
(VPS'in aynası) yeniden üret → kanıtla → push → VPS'te
[`bootstrap-update.sh`](../bootstrap-update.sh) ile değişmez release'i stage
edip doğrula → gerekirse yalnız çıktıda verilen root tarafından güvenilen
rollback komutunu kullan. İç release ve snapshot sözleşmeleri için
[`update.sh`](../update.sh) ve [`rollback.sh`](../rollback.sh) dosyalarına bakın.
