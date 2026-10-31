# Yönlendirmeli sunucu kurulumu

*Onaylanan ürün yönü: 10 Eylül 2026 · [English](SERVER-SETUP-PLAN.md)*

**Durum: onaylanan kaynak uygulaması ve yerel doğrulama tamamlandı; henüz yayımlanmadı.**
Geçerli sınır için [uygulama durumu ve kanıtlara](SERVER-SETUP-STATUS.tr.md) bakın.
Kullanıcı aktivasyon, Dashboard ve Components ekranlarını değerlendirdikten sonra
bu yönü onayladı. Bu belge anlaşmayı ve uygulamanın kabul ölçütlerini kaydeder.
Asistana kurulu panel güncelleme yetkisi vermez; [AGENTS.md](../AGENTS.md) gereği
kurulu panel güncellemelerini kullanıcı CelikPanel'in kendi arayüzünden başlatır.

## 31 Ekim ek kararı: koda katılan sunucuda posta (alpha.114 kaynağı)

Sahip alpha.113'te canlı buldu: Frankfurt'a kodla katılan Boston'da posta
hiç kurulamıyordu; Frankfurt ana alan adının postasını zaten aldığı için
inceleme `server_setup_peer_mail_both` ile reddediyordu. Barındırma
şirketinin her sunucusu kendi müşterilerinin web, veritabanı ve postasını
barındırır.

- **Katılan sunucu, üzerinde açılan alan adları için posta çalıştırabilir.**
  Katılmanın ortak yanıtı ana alan adının postasını alan tek sunucuyu
  söylemeye devam eder. Bu ilk sunucuysa katılan sunucunun postası onu
  sahiplenmez: ana alan adının MX, SPF, DMARC ya da DKIM kaydı oradan yazılmaz
  ve gönderilmez. Katılan sunucu yalnız kendi posta adını (adresine A kaydı)
  yayımlar: ilk kurulumunda adlarıyla ilk sunucuya, sonrasında erişim DNS
  adımıyla yetkili DNS bağlantısı üzerinden. Kendi posta sertifikası,
  Postfix/Dovecot/Rspamd ve Roundcube'u olur. Üzerinde açılan müşteri alan
  adlarının MX, SPF, DKIM ve DMARC kayıtları kendi bölgelerinde, her
  barındırılan bölge gibi ana DNS sunucusuna yayımlanır.
- **Postayı sonradan eklemek.** Tamamlanmış bir katılan kurulum (örneğin
  alpha.113'te yalnız web ve DNS ile kurulup sahibince güncellenen) Sunucu ->
  Sunucu kurulumu ile yeniden açılır ve posta eklenir. Genişletmenin adlar
  adımı hiçbir şey göndermez (bağlantı kurulum tamamlanınca bitti, ilk sunucu
  adları zaten biliyor); bitmiş bağlantıdaki ilk kurulum
  `server_setup_peer_link_revoked` yönlendirmesini korur.
- **Adlar.** Katılan sunucuda posta adı `mail.<bu sunucunun panel adı>`
  olarak önerilir. İlk sunucunun, ana alan adının ya da bir ad sunucusunun
  kullandığı posta adı incelemede `server_setup_peer_mail_hostname_taken` ile
  reddedilir (siz, sunucu sahibi: Bu sunucunun adları adımına dönüp bu
  sunucuya kendi posta adını verin; hiçbir şey değiştirilmedi). Panel adı da
  posta adı olabilir: panel kendi bağlantı noktasında kendi sertifika
  soyuyla dinler, posta sertifikası aynı 80 numaralı kontrol ile verilen ayrı
  bir soydur, web postası 443'te nginx'tir; tek A kaydı iki ada da yeter.
- **Metinler.** Katılan sunucuda Adlar adımı ve inceleme, ana alan adının
  kayıtlarının ilk sunucuda kaldığını ve bu kurulumun onları değiştirmediğini
  söyler; onları aktarmayı istemez.

Kalan: yalnız bileşen testleriyle kanıtlandı; iki sunuculu laboratuvar koşusu
(postalı ilk sunucu, web ve DNS ile katılan ikinci, güncelleme, sonradan
eklenen posta, bir müşteri alan adı için iki yönde posta) borçtur.

## 29 Ekim ek kararı: üçüncü hedefli Beta-1 yeniden koşusunun düzeltmeleri (alpha.115 kaynağı)

cfcadd3f1 ürün kaydının üçüncü hedefli yeniden koşusu aşağıdaki kusurları
buldu. Her düzeltmenin bileşen testi var ve hiçbiri şemayı değiştirmez;
kanıtlanmış sayılmadan önce yeni bir hedefli VM koşusu gerekir.

- **Açılan sunucu her adım için bir beklemedir.** Yeniden başlatmadan sonra
  çalıştırıcı, systemd hâlâ "starting" bildirirken DNS adımını sürdürdü;
  Agent'ın sunucu kimliği okunamadı ve adım genel `server_setup_dns_failed`
  ile düştü. Artık tek bir sınıflandırıcı, systemd açılırken (ya da Agent
  henüz yanıt vermezken) sunucu kimliği okunamadığı için gönderilmeden
  reddedilen Agent çağrısını `server_setup_host_starting` sayar: hiçbir şey
  gönderilmedi, kimse bir şey yapmaz ve çalıştırıcı aynı adımı yeni bir
  değişiklik yapmadan yeniden sorar. Kurulumun alt işlemi ancak sunucu kimliği
  okunduktan sonra oluşturulur. 10 dakika sonra hâlâ açılan sunucu
  `server_setup_host_not_started` (systemd hâlâ açılıyor; denetlenecek
  komutlar) ya da `server_setup_agent_not_answering` (Agent hiç yanıt vermedi;
  `sudo systemctl status celikpanel-agent`) ile düşer. Her açılış ayrı bir
  dönemdir: başka her sonuç öncekini bitirir.
- **Sunucudaki başka bir değişiklik başlatmayı tutar, çalışma bekler.**
  Bileşenler'den bir kurulum sürerken başlatılan çalışmanın ilk bileşenini
  Agent kabulde reddetti (`HOST_MUTATION_BUSY`) ve çalışma düştü. Başlatma
  artık çalışma oluşmadan `server_setup_host_busy` ile reddedilir (başka bir
  değişiklik sürüyor; bitene kadar bekleyin, sonra başlatın). Buna rastlayan
  çalışma aynı kodla bekler: başka bir işlem sürerken hiçbir şey oluşturulmaz.
  Alt işlem ancak hiçbir şeyin çalışmadığı kanıtlanırsa yeniden kabule sunulur
  (sunucu meşgul olduğu için ilk kirasında düştü ve Agent defterinde işi yok):
  aynı alt işlem ve satırı, denemeler adımda tutulur ve her deneme için denetim
  satırı yazılmaz; en çok 30 dakika, sonra `server_setup_host_stayed_busy`
  sunucuyu meşgul tutanı adlandırır. Sürdürülen alt işlem ya da sonraki posta
  TLS alt adımı reddedilen posta profili (kendi kodu, `mail_tls_sync_host_busy`)
  asla yeniden kabule sunulmaz.
- **Sertifika yetkilisinin gerisindeki saat her sertifika için söylenir.**
  Saat 200 sn geriyken panel sertifikası adımı yalnız
  `panel_certificate_issue_failed`, hiç beklemesi olmayan posta sertifikası
  ise genel `server_setup_step_failed` dedi. Artık ortak bir Agent işlevi,
  kurulumun verdiği her sertifikanın (panel ve posta sunucusu) bu saate göre
  geçerli olmasını en çok iki dakika bekler; daha büyük sapma saniyesiyle
  kodlanıp işlemden adıma taşınır: `server_setup_certificate_clock_skew`
  (saat N saniye geride; siz, sunucu sahibi: saat eşitlemesini açın,
  `timedatectl set-ntp true`, ya da saati düzeltin; sonra yeniden vermek için
  düzeltilmiş planı inceleyin).
- **Biten ana sunucu, açık bağlantı üzerinden yedeğin DKIM kaydını yine alır.**
  A seçeneğinde ana sunucu önce bitti; yedeğin DKIM yayımı sonra sonsuza dek
  `server_setup_peer_dkim_not_running` ile reddedildi. Ana sunucunun bağlantısı
  açık oldukça (yedek hazır olduğunu bildirene kadar sürer), ana sunucu kendi
  çalışması sürse de bitse de bağlı katılmanın DKIM kaydını alır ve aynı
  incelenmiş DNS yolundan yazıp yayımlar. Kapanmış bağlantı onu reddeder.
- **Ana alan adının postasını tek sunucu alır.** A seçeneğinde postayı iki
  sunucuda da seçince iki plan da ana alan adının postasını üstlendi: katılma
  belgesi postayı ilk sunucunun (kodu oluşturanın) aldığını söylüyordu, ama ana
  sunucunun taslağı kendi postası olduğu için bu yanıtı düşürdü ve yedeğin genel
  bakışı kendini alıcı gösterdi. Katılmanın ortak yanıtı artık tek doğruluk
  kaynağıdır: katılmış taslakta korunur, ana alan adının postasını ayrıca
  alacak katılmış sunucu incelemede `server_setup_peer_mail_both` ile reddedilir
  (siz, sunucu sahibi: postayı bu sunucunun planından çıkarın ya da öteki
  sunucuda kaldırıp yeniden katılın; sonra yeniden inceleyin) ve DNS genel
  bakışı ortak yanıtın adlandırdığı alıcıyı gösterir. Bu sürümde yalnız
  sunucunun kendi ana bilgisayar adı için posta yoktur; bu yüzden ikinci
  sunucu yanında posta tutamaz.
- **Kurulum bağlantısının metinleri sunucuları rolüyle adlandırır.** A
  seçeneğinde yedek DNS sunucusu katılma kodunu oluşturan sunucudur ve
  `server_setup_peer_link_revoked` ona yeni kodu "ilk sunucuda" oluşturmasını
  söylüyordu. Bağlantının İngilizce ve Türkçe metinleri artık "katılma kodunu
  oluşturan sunucu", "öteki sunucu" ya da "ana DNS sunucusu" der; kapanan
  bağlantı iletisi her tarafa yeni kodun nerede oluşturulacağını söyler.
Yanıtı kaybolan bir sertifika adımı da yetkiliden geri kalan saati söyler: alt işin kaydı saat kodunu ve cümlesini tutar; panel ve posta sertifikası adımları kaydı yeniden okurken bunu tekrar eşler.

## 29 Ekim ek kararı: hedefli Beta-1 yeniden koşusunun düzeltmeleri (alpha.115 kaynağı)

b86ab6fa4 ürün kaydının hedefli yeniden koşusu aşağıdaki kusurları buldu. Her
düzeltmenin bileşen testi var ve hiçbiri şemayı değiştirmez; kanıtlanmış
sayılmadan önce yeni bir hedefli VM koşusu gerekir.

- **Okunamayan güncelleme listesi başlatmayı tutar.** İnceleme, planın
  kurulumlarının kurulu dağıtım paketlerinden hangilerini ayrıca
  güncelleyeceğini okuyamadığı sürece (`package_updates_state: unknown`,
  örneğin paket yöneticisi meşgul) kurulum başlamaz: onay düğmesi nedeniyle
  birlikte kapalıdır ve başlatma isteği `server_setup_distribution_updates_unread`
  ile reddedilir. İncelemeyi yenile listeyi yeniden okur ve sunucuda hiçbir
  şey değiştirmez; plan kimliğini korur. Başlatma, sayfanın gösterdiği listenin
  özetini gönderir; arada başka bir incelemenin değiştirdiği liste
  `server_setup_distribution_updates_changed_before_start` ile reddedilir
  (incelemeyi yeniden okuyun, sonra onaylayın); saklanan liste yalnız
  okunmamışken değiştirilir. `server_setup_distribution_updates_changed`
  (ya da `_unread`) ile bekleyen bir bileşen adımı Güncellemeleri incele'yi
  sunar: çalışmanın kalan bileşen kurulumları için listenin okunması, ardından
  Onayla ve sürdür. Onaylanan liste o çalışma için kaydedilir ve aynı çalışma
  bekleyen adımdan sürer; yapılmış hiçbir şey yinelenmez ve plan yeniden
  açılmaz. Çalışan bir adım ya da bilinen bir hata asla değiştirilmez.
- **Hiçbir şey yerleştirmeyen DNS kurulumu hemen doğrulanmış hatadır.** DNS
  yazılımının ilk kurulumu herhangi bir paket yerleştirmeden başarısız olursa
  (örneğin APT'de bind9 için kurulum adayı yoksa) Agent bu geri almayı da
  kanıtlar: tam kurulum makbuzu, eksik bulduğu paketlerin hiçbirinin kurulu
  olmaması, o yazılımın hiçbir biriminin çalışmaması, hatalı ya da maskeli
  olmaması ve genel 53 numaralı bağlantı noktasında hizmet veren olmaması; iki
  kez okunur. Kurulum adımı sonraki geçişinde iki sonraki eylemiyle
  (yazılımı koruyup yeniden denemek ya da düzeltilmiş planda diğerini seçmek)
  `server_setup_dns_rolled_back` gösterir ve panelin yeniden başlatılması
  gerekmez. Önceden başlangıç kurtarması bırakana kadar
  `server_setup_dns_agent_failed` durumunda kalıyordu.
- **Kurulum bağlantısı ana DNS sunucusunun kendi kurulumundan sonra da
  sürer.** Ana sunucu kendi kurulumu hazır olduktan sonra bağlantının kendi
  tarafını, yedek sunucu kurulumunun hazır olduğunu bağlantı üzerinden
  bildirene kadar (`POST /api/v1/setup/peer/ready`; rolünün ad alanındaki
  bağlantı kimliğiyle doğrulanır, ilk sunucuda katılmadaki adresten gelir ve
  aynı katılmayı adlandırır: başka bir bağlama için bildirim reddedilir) ya da
  sahip ana sunucuda beklemeyi bırakana kadar (Diğer sunucuyu beklemeyi
  bırak, `POST /api/v1/setup/peer/retire`, onaylı) açık tutar. Bağlantıyı
  ancak o zaman kapatır. Katılma kodu yine ana sunucunun kurulumuyla biter.
  Yedek sunucu kalıcı olarak hazır olduktan sonra bildirir ve ana sunucuya
  bildirilene kadar bağlantının kendi tarafını tutar (`peer_setup: reporting`); ana sunucuya
  ulaşamazsa bekler (`server_setup_peer_ready_unreachable`) ve kendiliğinden
  yeniden bildirir. Yalnız alındı yanıtı ya da ana sunucunun
  `server_setup_peer_link_ended` yanıtı bildirilmiş sayılır: kapanan taraf kabul
  ettiği kimliğin anahtarlı özetini saklar (değerini asla) ve ana sunucu
  bağlantının kapandığını yalnız tam o kimlik için ve katılmadaki adresten
  söyler. Başka her ret `server_setup_peer_ready_refused` beklemesidir (siz,
  sunucu sahibi: ana sunucuda kurulumu açın, gerekirse Diğer sunucuyu
  beklemeyi bırak'ı seçin); başka TLS anahtarı
  `server_setup_peer_ready_identity_changed` olur (ana sunucuda yeni katılma
  kodu oluşturun). Bildirim yalnız kimliğinin doğrulandığı ad alanının
  kaydına işler; durumu okunamayan bildirim hatadır ve yedek yeniden bildirir. Böylece ana sunucu bittikten sonra başarısız olan yedek
  sunucu yeniden deneyip bitirebilir. Bu arada ana sunucu kendi hizmetleri
  için hazırdır ve kurulum sayfası ikinci sunucunun bitirmediğini söyler
  (`peer_setup: pending`; siz, ikinci sunucuda: kurulumu orada açın; sonra
  ikisi de hazır olur).
- **Yeni verilmiş panel sertifikası beklenir, hata sayılmaz.** Koşudaki
  başarılı verilişten sonraki `panel_certificate_issue_failed` ad çözümlemesi
  değildi: ACME sunucusu notBefore'u veriliş anıyla damgaladı ve yedek
  sunucunun saati yaklaşık 1,5 sn gerideydi; Agent certbot'tan hemen sonra
  okuduğunda yaprağı "henüz geçerli değil" buldu. Veriliş artık en çok iki
  dakika ve yalnız okuyarak, yaprak bu sunucunun saatine göre geçerli olana
  kadar bekler; sonra her zamanki gibi okur ve denetler. Daha büyük sapma yine
  başarısız olur ve nedeni artık saati adlandırır (`timedatectl`).
- **Yeniden katılan yedek sunucu yayımlama adımından geçer.** Yedek DNS
  sunucusu ilk sunucuysa (ana sunucu ona katıldıysa) planı ana sunucu
  katılmadan incelenir; bu yüzden yayımlama adımı henüz bir uç nokta
  adlandırmaz, uç noktayı bağlama verir. Adımın kapısı incelenmiş boş uç
  noktayı bağlanmış olanla karşılaştırıyor ve bu tür her çalışmayı genel
  `server_setup_step_failed` ile düşürüyordu (koşuda P4 yeniden katılmasından
  hemen sonra). Kapı artık bu adım için bağlanmış uç noktayı alır; yerine
  geçen katılmanın bağlamasını benimseyen çalışma sürer. Bağlamasını
  kullanmış bir çalışmanın altında katılma değişseydi çalışma
  `server_setup_plan_changed_by_rejoin` ile durur (siz, sunucu sahibi:
  Düzeltilmiş planı incele; kurulum yeni katılmayla sürer; tamamlanan sunucu
  değişiklikleri korunur), asla genel hatayla değil.
- **Kurulumun kendi DNS yazılımı "mevcut DNS" değildir.** Bir çalışmadan
  sonra (örneğin P4 yeniden katılması) yeni plan incelemesi, önceki
  çalışmanın kurduğu PowerDNS yüzünden
  `server_setup_existing_dns_requires_migration` ile reddediliyordu. Artık bu
  panelin bir çalışması kaydettiyse etkin motor kurulumun kendisinindir:
  isteği bu paneldeki bir kurulum çalışmasının başarılı DNS adımı olan,
  kaynak motoru olmayan, işlenmiş bir ilk kurulum; motorun güncel
  döneminde (sonradan değiştirilmemiş) ve bekleyen değişim ya da işaret yok.
  Böyle bir motor asla "taşıma gerektiren mevcut DNS" sayılmaz: plana
  uyuyorsa çalışmanın DNS adımı hiçbir şey kurmadan onu benimser; uymuyorsa
  inceleme bunu söyler (`server_setup_dns_installed_differs`: planda o motoru
  seçin ya da Sunucu → DNS'te değiştirin); incelemeden sonra bağlanan ikinci
  sunucu DNS adımında aynı kodla karşılaştırılır. Bu kaydı olmayan motor önceki gibi
  taşıma ister.
- **Üretici PowerDNS derlemesi nereden geldiğini söyler.** Kaynak sürümden
  önce denetlenir: repo.powerdns.com (örneğin 4.9.18-1pdns.trixie), backports
  ya da başka bir depodan gelen derleme belgelendiği gibi
  `server_setup_pdns_main_not_debian_archive` ile reddedilir,
  `server_setup_pdns_main_untested_version` ile değil.
- **Değişen katılma çalışmanın güncel durumudur.** İlk sunucunun çalışması
  henüz hiçbir şey başlatmamış bir adımda beklerken (örneğin DNS kurulumundan
  önce primary_dns) ikinci sunucunun yanıtları değişirse, artık yalnız eş
  kartı değil çalışmanın kendisi en üstte `server_setup_peer_join_changed`
  der. İşlemi süren bir adım ya da bilinen bir hata kendi durumunu korur.

## 29 Ekim ek kararı: Beta-1 sanal makine koşusunun düzeltmeleri (alpha.115 kaynağı)

Kurulum sihirbazı v2'nin temiz Debian 13 ve Ubuntu 24.04 konuklarındaki gerçek
koşusu aşağıdaki kusurları buldu; her düzeltmenin bileşen testi var, hiçbiri
şemayı değiştirmez ve kanıtlanmış sayılmadan önce hedefli bir VM koşusu gerekir.

- **Dağıtım yama güncellemeleri listelenir, reddedilmez.** Bir bileşen kurulumu
  kurulu paketleri aynı sürüm içinde de güncelliyorsa temiz kurulum artık
  durmaz. Yama güncellemesi üçünü birden ister: aynı epoch ve üst sürüm,
  dpkg sırasına göre daha yeni sürüm ve APT'nin gösterdiği her arşivin
  dağıtımın kendi yayımlanmış sürümü ya da güvenlik/güncelleme cebi olması
  (örneğin Debian-Security'den perl-base 5.40.1-6 → 5.40.1-6+deb13u1).
  İnceleme kurulum benzetimini bir kez okur ve "Ayrıca güncellenecek dağıtım
  paketleri" altında listeler; kurulumu başlatmak tam olarak bunları
  (paket=sürüm) onaylar; bunları Agent'a yalnız kurulumun kendi bileşen
  adımları gönderir ve Agent yalnız birebir eşleşeni kabul eder. Okunamayan
  inceleme kurulumu başlatmaz. Her bileşenin ilk kurulumundan önce adım
  yeniden sorar: incelemenin listelemediği güncelleme
  `server_setup_distribution_updates_changed` olarak bekler (sahip o bekleyişten
  inceleyip onaylar), okunamayan yanıt
  `server_setup_distribution_updates_unread` olarak bekler; hiçbir şey
  kurulmaz. PPA, üretici ya da üçüncü taraf depo, backports, sürüm düşürme,
  yeni üst sürüm, veritabanı sunucusu, Apache, başka PHP sürümü ya da başka
  bileşen bugünkü onayı korur.
- **Kendi DNS'ini tutan tek sunucu bölgesini bitirir.** Plan ve her bölge
  eşitlemesi bölge türünü tek kuralla seçer (tek başına `NATIVE`, çiftte
  `MASTER`); incelenen özet artık `infrastructure_dns_changed` döngüsüne
  girmez.
- **Posta DNS kimliğinden sonra gelir.** Katılma akışında katılmadan önce
  yalnız DNS gerektirmeyen bileşenler çalışır; posta profili ve posta sunucusu
  gerektiren her şey (SpamAssassin) iki sunucuda ve her düzende DNS adımları ve
  beklemelerinden sonra çalışır.
- **Ana DNS sunucusu olarak PowerDNS: tek denenmiş seri.** Sihirbazın sunduğu,
  incelemenin kabul ettiği ve DNS adımının kurduğu, denenmiş PowerDNS
  serisinin (4.9.x, epoch'suz, Debian arşiv revizyonlu, Debian arşivinden;
  D-028 eki) Debian'ın kendi arşiv yapısıdır. Bunun dışındaki aday sunulmaz
  (`server_setup_pdns_main_untested_version`, ya da repo.powerdns.com,
  backports veya başka depo için `server_setup_pdns_main_not_debian_archive`)
  ve incelemede reddedilir, çalışma ortasında değil.
- **İkinci sunucunun değişen yanıtları yeni kodla toparlanır.** Bağlama yine
  tek çalışmaya aittir. Onunla hiçbir çalışma başarıyla bitmemişken ikinci
  sunucunun sonradan değişen yanıtları (örneğin DNS yazılımı) katılmayı
  değişmiş olarak işaretler: ilk sunucunun çalışması
  `server_setup_peer_join_changed` der (sahip orada yeni katılma kodu oluşturup
  ikinci sunucuya girer), yeni koda izin verilir ve çalışma yeni yanıtları
  bağlayıp sürer. Değişmeyen yanıtlar eskisi gibi yeniden bağlanır; önceki
  yanıtlarla ilerlemiş bir çalışma planın yeniden açılmasını ister
  (`server_setup_peer_join_reopen_required`). Bir çalışma başarıyla bittikten
  sonra yanıtlar kesindir ve yeni kod reddedilir; ikisi de açıkça söylenir
  (`server_setup_peer_names_final`, `server_setup_join_finished`).
- **Başarılı bir çiftten sonra genişletme** onun bağlamasını kullanır; yeni
  katılma adımı yoktur. İkinci sunucunun aldığını (ortak yanıtlar, adlar,
  posta) değiştirecek genişletme incelemede reddedilir
  (`server_setup_extension_needs_second_server`): ortak yanıtları koruyun ya da
  iki sunucuyu yeniden kurun.
- **Katılma kodu denemeleri** her yerde aynı söylenir: bir adresten en çok 5
  yanlış deneme ve kodun toplamda kalan hakkı (oluşturmada 50); yeniden
  okumada ve retlerde aynıdır.
- **Sahibin yerel DNS düzenlemesi** DNS yazılımı kartında ve DNS sayfasında tek
  bildirimdir (`engine_owner_changed` / `dns_overview_engine_owner_changed`):
  CelikPanel dışında değişti, sahip karar verir, korur (yapılacak bir şey yok;
  asla üzerine yazılmaz); asla sakin bir "Yapılacak bir şey yok" ve asla
  yabancı DNS sunucusu değildir. Bilinen sınır: değişiklik durdukça CelikPanel
  orada DNS değişikliği yayımlamaz (kayıt düzenleyicisi de yayımlayamaz) ve
  yerel düzenlemeden sonra DNS yazılımını geri vermek bu kaynakta yok; bildirim
  bunu söyler.

## 29 Ekim ek kararı: kurulum sihirbazı v2, 5. aşama (v0.1.0-alpha.115 kaynağı)

- **DNS sayfası tek bir resim okur.** `GET /api/v1/dns/overview` (yalnız
  yönetici, salt-okur, şema değişikliği yok) bütün DNS durumunu tek yanıtta
  verir: tek D-024 cümlesi (durum `ok`, `waiting`, `prerequisite`, `failed`
  ya da `unknown`; sabit kod, neden, kim yapar, sıradaki adım, sonra ne olur),
  bu sunucu ve bağlanmış ikinci sunucu (rol: ana, yedek, tek ya da yok; kısa
  ad, panel adı, DNS yazılımı; doğrudan sorulduğunda ana alan adı için yanıt
  verip vermediği: veriyor, veriyor ama alan adı için değil, sorulamadı,
  bağlanmadı), adlar ve genel çözücülerin yanıtı (panel ve posta adları, bölge
  içindeki ad sunucusu adları, alan adı firması kontrolünden kayıt kuruluşunun
  ad sunucuları ve IP kayıtları (glue)), ana alan adının MX, SPF, DKIM (seçici
  `celik`) ve DMARC kayıtları ile postayı kimin aldığı, sahibin kendi
  kayıtlarını nerede düzenlediği ve ikinci sunucunun burada bağlanıp
  bağlanamayacağı. Liste bir sağlayıcıdaysa hiçbir sunucu DNS yazılımı
  göstermez ve alan adı firması kontrolü yapılmaz. Başarısız sorgu
  "bilinmiyor"dur, asla "eksik" değil; önce doğrulanmış hata, sonra birinin
  yapması gereken, sonra bakılamayan, sonra yayılan değişiklik söylenir. Panel
  başına sınırlı tek yanıt bir dakika paylaşılır (bir kısmı bilinmiyorsa 15
  saniye) ve çağırandan bağımsız hesaplanır; yoklama hiçbir şeyi değiştirmez.
  Bağlantı kimlik bilgisi, kayıt ve katılma kodları yanıtta yer almaz.
- **Sayfa.** Sunucu → DNS bu cümleyle açılır; ardından sunucular ("Bu sunucu
  (ad)", "İkinci sunucu (ad)"; tahmin edilmiş ad yok) ya da listenin nerede
  tutulduğu, "Sunucu bağla" girişi, satır satır internet kontrollü ad tablosu,
  posta kayıtları ve mevcut kayıt düzenleyiciye bağlantılı "Sizin eklediğiniz
  kayıtlar" gelir. Bugünkü DNS ayarları ve motor kartı Gelişmiş altında,
  kapalı ve yalnız açılınca yüklenerek olduğu gibi durur.
- **DNS sayfasından bağlanma.** Kurulum ikinci sunucuyu beklerken giriş mevcut
  katılma kartıdır. Aksi hâlde burada neden bağlanılamadığını söyler: kurulum
  bitmedi; BIND'li tek sunucu (çalışan bir BIND'e bu sürümde yedek DNS
  sunucusu eklenemez); PowerDNS Gelişmiş'te eşleşir; liste bir sağlayıcıda ya
  da başka bir CelikPanel'de (ikinci sunucu kendi kurulumunu yapar); bu sunucu
  yedektir.
- **Postayı taşıma.** Sahip eski posta sunucusunu tuttuysa (MX "sonra") ve
  liste bu paneldeyse, "Postayı şuraya al" düğmesi ne değişeceğini söyleyen
  sayfa içi onaydan sonra mevcut kayıt düzenleyiciyle tek kök MX kaydını
  değiştirir (yeni posta bu sunucuya gelir; önce posta kutularını oluşturun;
  eski sunucudaki posta orada kalır). Birden çok MX kaydı düzenleyiciye
  bırakılır; yanıtı yiten değişiklik asla yeniden gönderilmez, sayfa durumu
  yeniden okur.
- **İlk site.** Web siteleri kurulan tamamlanmış kurulum "İlk sitenizi
  ekleyin" önerir: mevcut Alan adı ekle formu ana alan adı dolu açılır ve
  sahip göndermeden hiçbir şey oluşturmaz; "Sonra" öneriyi gizler.

İnceleme düzeltmeleri (aynı aşama):

- **Paylaşılan kontrol.** Süre hesabı kilidin içinde yapılır (`-race` ile bir
  veri yarışı ölçülmüştü); çöken bir kontrol bekleyenleri uyandırır, onlar da
  sonucun bilinmediğini söyler. "Şimdi bak" (`?fresh=1`) panel başına 15
  saniyede en çok bir kez gerçek yeni kontrol başlatır; bu süre içinde süren
  kontrolü ya da son yanıtı döndürür, `fresh_after` sonrakinin ne zaman
  mümkün olduğunu söyler. Sayfa paylaşılan yanıtın süresi dolunca
  (`next_check_after`) yeniden okur, sekme gizliyken bekler. Ana alan adı
  bölgesine her yazma (kayıtlarının özeti anahtarın parçasıdır) yeni bir
  kontroldür; düzenleyicideki ya da "Postayı al" ile yapılan değişiklik hemen
  görünür.
- **Doğru durumlar.** Bitmemiş ve sürmeyen kurulum internete hiçbir şey sormaz
  ve Sunucu kurulumu'nu gösterir. Okunamayan DKIM anahtarı bilinmiyordur;
  postalı tamamlanmış kurulumda anahtarın olmaması bir iştir. İlk kopyası
  kanıtlanmadan (anada eş hazır, yedekte katalog tüketimi) yanıt veren ama
  alan adı için vermeyen yedek DNS sunucusu sayfada da alan adı firması
  kartında da bekliyordur (tek kural). İnternetin yanlış gösterdiği sunucu adı
  önce kendi DNS sunucumuzun yanıtıyla karşılaştırılır: listemizde başka değer
  varsa bu sahibin değişikliğidir, gecikme değil. Kurulum düzenlendiğinde
  (incelenen genişletme) resim son tamamlanan çalışmayı ve ikinci sunucusunu
  korur. Yalnız cümlenin başlığı canlı bölgedir; yalnız durum doğrulanmış
  hataya dönünce uyarıdır.
- **Posta.** MX bu panelin kendi listesine göre değerlendirilir. "Postayı al"
  yalnız liste hâlâ kurulumun aktardığı eski MX'i aynen taşıyorsa önerilir;
  sahibin ayarladığı MX asla eski posta sunucusu sayılmaz. Kayıt güncelleme
  API'si isteğe bağlı `expected_content` alır ve uyuşmazlığı reddeder
  (`DNS_RECORD_CHANGED`); onay kaydın gerçek TTL'sini söyler. Kurulumun
  yazdığı ve sahibin silip değiştirdiği kayıt sonraki plan ya da genişletmede
  geri eklenmez.
- **Altyapı bölgesinde ilk site.** Ana alan adı zaten altyapı bölgesiyken site
  olarak eklenince yalnız eksik kök ve www adresleri eklenir (A, küresel IPv6
  varsa AAAA); başka yeri gösteren ad değiştirilmez, kimin karar vereceğiyle
  bildirilir.

Bu aşamada yok: "Ana DNS'i taşı" (alpha.116–.118); mevcut kurulum eşlemesi
sahibin 2026-10-26 kuralıyla düşürüldü (gerçek bir kurulum yok). Kalan
sınırlar: diğer sunucunun DKIM anahtarı yalnız yayımlanmış olarak kontrol
edilir (değeri o sunucunundur); ters DNS kurulumun açık işi olarak kalır, bu
sayfanın satırı değildir; kayıt düzenleyicinin kendisi değişmedi. Yalnız
yerel geri döngü sunucularında bileşen testleriyle ölçüldü; kurulu
sunucularda değil.

## 28 Ekim ek kararı: kurulum sihirbazı v2, 4. aşama (v0.1.0-alpha.114 kaynağı)

- **Tek sunucu, kendi DNS'i.** Kendi DNS'ini tutan tek sunucu bağımsızdır: iki
  ad sunucusu adı da onu gösterir, yedek DNS sunucusu yoktur ve sihirbaz BIND
  için sonradan yedek eklemeyi vaat etmez (çalışan BIND bağımsızdan çifte
  geçemez). Hazırlık için iki adın da bu sunucuya yayımlanması gerekir; alan adı
  firması kartı bu sunucuya bir kez sorar.
- **İkinci sunucu başlangıçta kodla katılır.** İki sunucu ve sahibin kendi
  DNS'inde ilk sunucunun planı bileşenlerini (hizmetler, çalışma ortamları,
  posta profilleri) kurar, sonra "ikinci sunucu katılır" adımında bekler; ikinci
  sunucunun adresi hiç yazılmaz. Yöneticisi orada bir katılma kodu oluşturur: 24
  karakter; ilk 16'sı ilk panelin TLS anahtar parmak izinin (SubjectPublicKeyInfo
  SHA-256) 80 bitini, son 8'i 40 bitlik tek kullanımlık gizli değeri taşır; 24
  saat ve 5 yanlış deneme geçerlidir, yalnız tuzlu özeti tutulur, gizli değer
  olmadan denetim kaydına yazılır, bir kez gösterilir. İkinci sunucuda ilk adım
  "İlk sunucuya kodla katıl"ı sunar: adres ve kod. İkinci sunucu parmak izini TLS
  el sıkışmasında, hiçbir şey yazılmadan önce denetler; yanlış adres gizli değeri
  asla almaz. İlk sunucu gizli değeri sabit sürede karşılaştırır, çağıranın
  kaynak adresini bildirdiği adresle, bir hız sınırını (dakikada 20, bir
  adresten 5) denetler ve kodu bir kez tüketir. Yanıtlar: katıldı; nedeniyle
  reddedildi (kalan denemelerle yanlış kod, kullanılmış, süresi dolmuş, tükenmiş,
  beklemiyor, kaynak uyuşmazlığı); yanlış sunucu (başka anahtar; hiçbir şey
  gönderilmedi); ulaşılamadı (hiçbir şey gönderilmedi; kod kullanılmadı);
  bilinmiyor (yanıt kayboldu: istek saklanır, sonraki deneme aynı istektir; ilk
  sunucu aynı katılmayı yeni gizli değerlerle yanıtlar, asla ikinci bir katılma
  olmaz).
- **Katılma belgesi.** İkinci sunucunun ortak yanıtları ilkinden gelir: ana alan
  adı, iki sunucu, kendi DNS'i, hangi sunucunun ana DNS sunucusu olduğu, ns1
  (hep ana) ve ns2, ilk sunucunun adresi, kısa adı ve adları; ayrıca ya ilk
  sunucunun yayımlama adresi ve tek kullanımlık kayıt kodu (ana ilk sunucuysa) ya
  da ilk sunucunun hazırladığı liste (ana ikinci sunucuysa). İkinci sunucunun
  incelemesi ortak yanıtları farklı taslağı reddeder ("değiştirmek için yeniden
  katılın").
- **Adlar bağlantıdan gelir, bir kez bağlanır.** İkinci sunucunun planı adlarını
  (panel, posta, ana alan adının postasını alıyorsa posta seçimi, adresi ve
  anahtar parmak izi) bağlantı üzerinden göndererek başlar; ilk sunucu onları
  kendi adları ve kayıtlarıyla hemen karşılaştırır (çakışma hiçbir şey
  yazılmadan reddedilir) ve uyanır. Katılma adımı ikinci sunucuyu o çalışmaya bir
  kez ve değişmez biçimde bağlar (DNS yayımlayıcı bağlamasındaki gibi INSERT OR
  IGNORE); bölge kayıtları bağlanma anında bölgeye göre hesaplanır. Sonraki her
  adım bağlamanın tamamladığı planla çalışır; incelenen plan hiç değişmez. Bekleme
  sırasında yeniden başlayan çalıştırıcı aynı biçimde bağlar; bağlamadan sonra
  değişen adlar nedeniyle reddedilir.
- **Ana DNS sunucusunu seçmek (Seçenek A).** Ana ikinci sunucuysa ilk sunucu
  listeyi (kayıtlar, posta seçimi) hazırlar ve ikinci katılana kadar DNS yazılımı
  kurmaz; liste katılmayla gider; ikinci sunucu bölgeyi ilk sunucunun adlarıyla
  tutar, ilk sunucu da ikincinin adlarıyla verdiği kayıt koduyla yedek olarak
  kurulur.
- **Yöneticisiz yayımlama bağlantısı.** Katılmış yedek DNS sunucusu yayımlama
  bağlantısını devredilen kayıt koduyla kendisi kurar (katılma ana yöneticinin
  onayıydı, Q5); ana DNS sunucusu kabul edene kadar (çifti hazır, ad sunucuları
  genel) adım bu nedenle bekler. Bağlantının kendisi mevcut kuralları korur
  (HTTPS adı, genel adresler, genel bir kuruluşun sertifikası).
- **Diğer sunucunun postası.** Yedek DNS sunucusu ana alan adının postasını
  alıyorsa (ananın postası yoksa) MX, SPF ve DMARC onun yanıtlarıyla anada yazılır,
  DKIM kaydı bağlantıdan gider; "elle ekleyin" kalktı. Ana alan adının postasını tam
  olarak bir sunucu alır ve hangisi olduğunu katılmanın ortak yanıtı söyler
  (29 Ekim 2026'da değişti: iki sunucuda da seçilen posta incelemede
  `server_setup_peer_mail_both` ile reddedilir).
- **Bağlantı.** Bağlantı eşin IPv4 adresine bağlanır ve yalnız anahtar parmak
  izine (katılmadan sonra tam SHA-256 SPKI) güvenir, asla sertifika adına güvenmez.
  Panel, alan adı sertifikalarından sonra da IP adresinde ilk kendinden imzalı
  kimliğini sunar; bu yüzden kurulum boyunca iz geçerli kalır. İlk sunucu kodun
  gizli değerinin ve ikinci sunucunun kimlik bilgisinin yalnız anahtarlı özetini
  (veritabanı dışındaki panel anahtar dosyasıyla HMAC) tutar; ikinci sunucu kabul
  ettiği kimlik bilgisini türetir, yazılan kodu da yalnız katılmanın sonucu
  bilinmezken anahtar dosyasıyla mühürlü tutar (bilinen her sonuçta silinir).
  Bir sunucunun yeniden kullanması gereken her gizli değer kendi anahtar
  dosyasıyla (AES-GCM, `secret.key`) mühürlüdür: ikinci sunucuda bağlantı kimlik
  bilgisi, ilk sunucunun devrettiği kayıt kodu ve ana DNS sunucusu olarak kendi
  verdiği kod; ilk sunucuda ikinci sunucuda kullandığı kimlik bilgisi ve Seçenek
  A'da ananın adlarıyla gönderdiği kayıt kodu. Bu yüzden iki veritabanından
  birinin kopyası tek başına ne bir erişim kimliği, ne kod, ne kayıt kodu verir.
  Özetler ve mühürlü değerler yapıldıkları anahtar kuşağını taşır (`k1:`); böylece
  sonraki bir anahtar değişimi eski değerleri yenilerinden ayırabilir. Anahtar
  değişimi henüz yoktur; başka kuşaktan bir değer okunmaz. Bağlantı kimliği ve
  katılma kodu iki sunucuda da kurulum tamamlanınca kaldırılır. Gizli hiçbir şey
  yönetici API'sinden, günlükten ya da denetim kaydından çıkmaz.

Kalan sınırlar: kaynak için yalnız IPv4 adresi denetlenir; yedeğin kendi anahtar
parmak izi adlarıyla bir kez gönderilir; bağlantı kaydını kaybeden ikinci sunucu
yeni kodla yeniden katılır. Yalnız geri döngü sunucularındaki bileşen testleriyle
ölçüldü; kurulu sunucularda ölçülmedi.

## 28 Ekim ek kararı: kurulum sihirbazı v2, 3. aşama (v0.1.0-alpha.113 kaynağı)

Alan adı firması kartı kendini kontrol eder. `GET /api/v1/setup/delegation?zone=`
(yalnız sahibin kendi DNS sunucularındaki ana alan adı; yalnız yönetici, salt-okur,
bir dakika paylaşılan sınırlı yanıt) üst bölgeyi SOA kaydıyla bulur (`.com.tr`
gibi uzantılar için liste gerekmez), üst bölgenin kendi sunucularına özyinelemesiz
olarak hangi ad sunucularını ve IP kayıtlarını (glue) verdiklerini sorar, bizim
her sunucumuza bölge için yetkili yanıt verip vermediğini (SOA seri numarasıyla)
sorar ve aktarılan kayıtları sunucularımızın yanıtıyla karşılaştırır. Her madde
doğru, henüz değil, bekliyor (kurulmadan önce yedek DNS sunucusu) ya da
bakılamadı olur; okunamayan yanıt asla "henüz değil" sayılmaz. İki sunucuda kart,
yedek yanıt verince ya da kurulum panel adını beklerken tam açılır; "Yine de şimdi
yap" ana DNS sunucusu yanıt verene kadar nedeniyle reddedilir ve `.tr` kayıt
kuruluşunun ikisi de yanıt vermeden değişikliği reddettiği uyarısını verir. Glue
yalnız alan adının içindeki ad sunucuları için istenir; IPv6 glue isteğe bağlıdır.
Kurulu bir sunucuda ölçülmedi.

İnceleme düzeltmeleri: karşılaştırma ham aktarılan kayıtlarla değil, bölgenin
yayımladığı kayıtlarla yapılır (aktarılan kayıtlar ve posta kayıtları: birleşik
SPF, sahibin seçtiği MX ve DMARC); sahibin bizimkinin yanına eklediği değer
(yedek MX) kabul edilir. Yalnız yayımlanan bir değeri içermeyen yetkili NOERROR
ya da NXDOMAIN yanıtı "henüz değil"dir; yanıt yoksa, yanıt yetkili değilse ya da
REFUSED/SERVFAIL ise "bakılamadı" olur. Üst bölge yürüyüşü ve sunucularımızın
yanıtları kendi süre sınırlarıyla yan yana çalışır. Yetkisiz bir üst bölge
sunucusu (yetkisiz NXDOMAIN, alan adının ad sunucularını içermeyen yanıt)
atlanır ve sıradakine sorulur; üst bölge sunucularına yalnız IPv4 ile ulaşılır,
yalnız IPv6 sunucusu olan bir üst bölge "bakılamadı" olur. Bilinmeyen maddesi
olan yanıt paylaşılmaz, ikinci bakış süren kontrolü bekler (ya da kendi isteği
bitince vazgeçer) ve her şeyi doğru olan kart, adım yeniden açılana kadar
sormayı bırakır.

## 28 Ekim ek kararı: kurulum sihirbazı v2, 2. aşama (v0.1.0-alpha.112 kaynağı)

- **Mevcut kayıtlar.** Ana DNS sunucusunda DNS adımından sonraki adım, ana alan
  adının bilinen adları (alan adının kendisi, www, mail, webmail, autodiscover,
  _dmarc) ve sahibin yazdığı adlar için internetin şu anki yanıtını okur
  (`GET /api/v1/setup/public-records`: iki sabit genel çözümleyici, salt-okur,
  bir dakika paylaşılır; okunamayan ad bilinmiyor sayılır). İnternet tam liste
  vermez ve ekran bunu söyler. Her kayıt aktarılır ya da atlanır; atlanan,
  ad sunucusu değişikliği yayılınca çalışmayı bırakır. Kurulumun bir sunucuya
  verdiği ad asla devralınmaz; eski posta sunucusu `mail.<alan>` adını
  tutuyorsa tek düğme bu sunucuya `mail2` verir.
- **Posta kayıtları.** E-posta seçiliyse ana alan adının MX, SPF ve DMARC
  kayıtları bölgeye yazılır; DKIM posta profilinden sonraki `mail_dns` adımında
  (anahtar, imzalama, TXT kaydı; tekrarlanabilir). MX yalnız eski MX aktarılmadıysa
  ya da sahip "şimdi" dediyse değişir; SPF birleştirilir (bu sunucu eklenir,
  hiçbir şey çıkarılmaz); eski DMARC seçime göre korunur ya da değiştirilir.
  Aktarılan ve posta kayıtları bölgede sahibindir (Alan adları'ndan
  düzenlenir); yalnız kurulumun kendi sunucu adresleri kilitli kalır.
  İnceleme düzeltmesi: kayıtlar bölge yazıcısının kendi biçiminde karşılaştırılır.
  Kurulum yazdığı aktarılan ve posta kayıtlarının listesini tutar
  (`panel_settings`, şema değişikliği yok); gözden geçirilmiş planda kurulumun
  yazdığı ad başına tek kayıt (posta adı değişince birleşik SPF, DMARC, posta
  "şimdi" taşınınca aktarılan eski MX) tam saklı satırıyla değiştirilir, asla
  reddedilmez. Kurulumun yazmadığı (ya da sahibin Alan adları'nda değiştirdiği)
  farklı bir SPF veya DMARC planı reddeder; sahibin eklediği başka bir MX (yedek
  MX) bizimkinin yanında kalır, çünkü bir adın birden çok MX'i olabilir. Kurulumun
  yazdığı ama yeni planın artık istemediği kayıtlar (atlanan aktarım, "şimdi"
  "sonra"ya dönünce bizim MX) Alan adları'nda sahibine bırakılır.
- **Harici sağlayıcı.** Sağlayıcı listesine MX, birleştirilmiş SPF, DMARC ve
  DKIM satırları genel durumlarıyla eklenir (sahibin koruduğu eski MX için
  "korunuyor", DKIM anahtarı oluşana kadar "bekliyor").
- **Ters DNS.** Posta adımı sunucu sağlayıcısına verilecek IPv4 ve IPv6 PTR
  satırlarını gösterir. IPv4 ve genel IPv6 varsa IPv6 için ters DNS, kurulumun
  beklemediği açık bir iştir (sahibin 2026-10-28 kararı): posta onsuz da çalışır
  ama istenmeyen sayılabilir; internet doğru adı gösterene kadar posta adımında,
  kurulum ekranında ve tamamlandıktan sonra listelenir (yanıtsız sorgu
  bilinmiyor sayılır).
- **Yardımcı birimler.** SpamAssassin ve ClamAV yeniden sihirbaz seçeneğidir:
  tarayıcı artık yardımcı birimlerini (spamass-milter, clamav-freshclam) görür;
  bunlar servisin kendisi sayılmaz.

Şema değişikliği yok. Kurulu bir sunucuda ölçülmedi.

## 26 Ekim ek kararı: kurulum sihirbazı v2, 1. aşama (v0.1.0-alpha.111 kaynağı)

Sahip yeniden tasarlanan ilk kurulum sihirbazını onayladı (önce ana alan adı,
görevler, ana alan adından önerilen adlar, açık DNS yeri). 1. aşama bunu
bugünkü DNS altyapısı üzerinde sunar; ikinci sunucunun kodla bağlanması, ikinci
sunucunun ana seçilmesi, mevcut kayıtların aktarılması, posta kayıtları, alan
adı firmasının canlı kontrolü ve ana DNS'in taşınması sonraki sürümlerdedir.

- **Adımlar.** Ana alan adı (bir kez, ilk) → sunucu sayısı (iki önerilir) →
  sunucunun kısa adı (yalnız makine adı insan seçimi gibiyse önerilir; panel ad
  uydurmaz) → görevler (Web siteleri, E-posta, DNS; birleşebilir; her biri ne
  kurduğunu yazar) → bileşenler (isteğe bağlı; Bileşenler sayfasıyla aynı
  katalog; kurulu sürüm, önerilen dağıtım sürümü, üretici sürümleri yalnız açık
  depodan, sihirbazın kurmadığı her bileşen için neden) → ana alan adı ve kısa
  addan önerilen adlar (sahibin değiştirdiği ad korunur) → DNS listesi nerede →
  özet → kurulum.
- **DNS yeri.** *Kendi sunucularımda*: bu sunucu ana DNS sunucusudur (bu sürümde
  tek ana sunucu seçeneği) ya da zaten kurulu bir ana sunucunun yedek DNS
  sunucusudur; BIND ya da PowerDNS, incelemenin kendi kapı gerekçeleriyle. Ana
  alan adı bölgedir; artık işaretsiz bir seçenek değildir. İki sunucuda ikinci
  sunucunun ad sunucusu adı ve IPv4'ü hâlâ yazılır ve ekran bunu söyler. Tek
  sunucu henüz kendi DNS'ini tutamaz (`server_setup_local_dns_requires_two_servers`
  ile reddedilir). *Harici sağlayıcıda*: DNS yazılımı kurulmaz, 53 açılmaz;
  başlatınca eklenecek kayıtlar canlı genel durumlarıyla gösterilir
  (`GET /api/v1/setup/provider-records`, salt-okur, dakikada yaklaşık bir
  kontrol, başarısız sorgu bilinmiyor sayılır). *Başka bir CelikPanel'de*:
  bugünkü bağlantı.
- **Kurulum ekranı.** Panel, adı çözülene kadar `https://IP:2083` adresinde geçici
  sertifikayla açılır ve bu açıkça söylenir. Alan adı firması kartı ad
  sunucularını ve ana alan adının içindeki adlar için IP kayıtlarını (glue)
  bugünkü zamanlamayla verir: yedek DNS sunucusu hazır olduktan sonra.
- **Arka uç.** Ek taslak alanları `main_domain`, `server_count`, `short_name`,
  `main_dns_server` ve `customization.versions` (seçilen üretici paketi hizmet
  adımının paketi olur; kurulum ön kontrolü deponun açık olmasını yine ister).
  `GET /api/v1/setup/components` her katalog bileşenini sihirbazın onunla ne
  yaptığıyla ve görev başına DNS yazılımı teklifiyle listeler. Yardımcı birimini
  kurulumun son kontrolünün göremediği servisler (SpamAssassin, ClamAV) ve
  Netdata (üretici deposu gerekir) kurulumdan sonra Bileşenler'den kurulur.
  Şema değişikliği yok.

Kurulu bir sunucuda ölçülmedi; ilk gerçek çalışma Beta 1 yeniden kurulumudur.
Kurulu paneller yine yalnız sahiplerince güncellenir.

## 11 Eylül ek kararı: açık sihirbaz tercihi (Alpha66 sürüm kaynağı)

Alpha65 sonrasında ilk yönlendirmede **Sihirbazla kur** ve **Kendim
yapılandıracağım** seçenekleri sunulur. Tercih sunucuda saklanır. Manuel tercih
otomatik sihirbazı ve Dashboard davetini kaldırır; **Sunucu → Sunucu kurulumu**
(v0.1.0-alpha.100'e dek Ayarlar → Sunucu kurulumu)
üzerinden geri dönülebilir. Güncellenen sunucu boş kabul edilmez; tamamlanmış
ve devam eden kurulumlar korunur. Manuel tercih kurulumu tamamlamaz, güvenlik
kontrollerini veya lisans denetimini kaldırmaz. Etkin/bekleyen işlem varken
tercih değiştirilemez. [Uygulama ve doğrulama kaydı](SERVER-SETUP-GUIDANCE.md).

## 13 Eylül uygulama notu: erişim DNS kayıtları sihirbazın içinde

Çalışma ağacındaki düzeltme, açıkça incelenen altyapı DNS kayıtlarını sihirbaz
içinde, herkese açık ad doğrulaması ve sertifika isteğinden önce hazırlar. Bir
müşteri alan adı oluşturmak için kurulumdan çıkmak gerekmez. Yerel DNS seçmek,
otomatik çıkarılan bir üst bölgenin yönetimini devralma izni değildir: sunucu
sahibi tam bölgeyi seçer ve gerekli kayıt eklemelerini inceler. Mevcut DNS
sahipliği ve kayıtlar korunur; ikincil sunucu doğal bölge aktarımı alır.
Uygulama barındırması onu yerel birincil DNS sunucusuna dönüştürmez.

Bu ek, aşağıdaki mevcut sonuç ve sahiplik gereksinimlerini uygular; onay
kapsamını değiştirmez. Önceden başlatılan planların adımları ve işlem kimlikleri
korunur; yeni altyapı değişiklikleri yeniden inceleme gerektirir. Sıralama,
kurtarma, yerel test kanıtları ve destek sınırları için
[erişim DNS uygulama notuna](SERVER-SETUP-ACCESS-DNS.tr.md) bakın. Bu düzeltme
henüz yayımlanmadı ve kurulu Frankfurt/Boston çiftinde doğrulanmadı.

## 1. Sonuç ve hedef kullanıcı

Sunucu yöneticisi bileşen kataloğunu yorumlamadan ve ilgisiz ayar sayfaları arasında
dolaşmadan kullanılabilir, güvenli bir ortama ulaşmalıdır. İlk soru sunucunun
amacıdır. Mevcut CelikPanel görsel dili, Türkçe ve İngilizce, mobil uyumlu yerleşim
ve erişilebilir kontroller korunur. Bayi, müşteri ve ek kullanıcılara sunucu
kurulum kontrolleri verilmez.

Lisans aktivasyonu ile sunucu kurulumu bağımsız durumlardır:

| Gözlenen sunucu durumu | Geçerli lisanstan sonra açılacak ekran |
|---|---|
| Yeni kurulum olduğu doğrulanmış | Kurulum rehberi |
| Kaydedilmiş yarım kurulum | Geçerli adımdan devam |
| Önceden hazırlanmış kurulum | Dashboard |
| Kurulum kaydı bulunmayan mevcut sunucu | Erişimi koru; salt-okur değerlendir ve yeni olduğunu varsaymadan kurulum öner |

Anahtar değişimi, yeniden giriş, yeniden başlatma veya güncelleme kurulumu
sıfırlamamalıdır. Alan adı listesinin boş olması yeni kurulum kanıtı değildir.
Hazır bir sunucuda hazırlık kontrolünün başarısız olması yeni kurulum dayatmamalı
ve mevcut işleri durdurmamalıdır.

## 2. Amaç profilleri

| Seçenek | Hedeflenen sonuç |
|---|---|
| Web hosting — önerilen | Web ve PHP hosting ile gerekli veritabanı altyapısı; varsayılan olarak posta kurulmaz |
| Web ve e-posta hosting | Web hosting, posta kutuları, webmail ve posta koruması |
| Uygulama sunucusu | Seçilen uygulamanın çalışma ortamı, web erişimi ve gerekli bağımlılıkları |
| DNS sunucusu | Yeni yetkili DNS altyapısı veya mevcut yapıya secondary olarak katılım |

Profiller gerçek sunucuda desteklenen, birbiriyle uyumlu bileşenlere çözülür.
Özelleştirme ikincildir; kullanıcı her paketi seçmek veya ilgisiz araçları kurmak
zorunda kalmaz. Bütün yaşam döngüsü uygulanıp doğrulanmadan bir profil çalıştırılabilir
olarak sunulmaz. Profiller müşterinin ilk sitesini veya posta kutusunu değil,
altyapıyı hazırlar. Bunlar altyapı hazır olduktan sonraki adımdır.

## 3. DNS politikası

Hosting kurulumunda DNS yönetim modeli seçilir. Yerel yetkili DNS motoru her
sunucu için zorunlu değildir:

1. **DNS'i burada yönet:** desteklenen yerel motoru, nameserver kimliğini ve gerekli
   topolojiyi yapılandır. Bu modelde üretime hazırlık bağımsız sunucularda yetkili
   DNS yedekliliğini içerir; aynı makinedeki iki ad bunu sağlamaz.
2. **Mevcut CelikPanel DNS altyapısını kullan:** yetkili yayıncı/tüketici ilişkilerini
   kur ve uzak otoriteyi doğrula. Yalnızca eş IP adresi yönetim izni vermez.
   Uzak DNS kullanan web sunucusuna DNS motoru kurmak veya onu secondary yapmak gerekmez.
3. **Harici DNS sağlayıcısını kullan:** eklenecek kayıtları tam olarak göster ve
   doğrula. Elle kayıt yönetimi tasarımın parçasıdır; sağlayıcı API bilgileri
   önkoşul değil, isteğe bağlı otomasyondur.

Altyapı/varsayılan model ilk kurulumda seçilir; her alan adının gerekli kayıtları
o alan adı yayımlanırken doğrulanır. İşletim sisteminin DNS çözümleyicisi, yetkili
DNS hazırlığına kanıt değildir. Secondary aktarılan DNS zone'larını sunar;
siteleri yedeklemez ve yeni yerel alan adlarının yayıncısı sayılmaz.

Bu onaylı yön, D-009'un her durumda yerel DNS gerektiren ürün kuralının yerini
alır. Yerel otoritede motor sahipliği, yayınlama, topoloji, DNSSEC ve kurtarma
kuralları geçerliliğini korur. Kontroller gevşetilmeden açık bir harici DNS modu
uygulanmalıdır; yalnız ön yüzdeki engeli kaldırmak bu özellik değildir.
Desteklenen açık bir taşıma işlemi yapılana kadar mevcut alan adlarının DNS
sahipliği korunur.

## 4. Güvenli erişim ve zorunlu kontroller

İlk uygulama `panel.example.com` gibi bir genel panel FQDN'si, güvenilir sertifika
ve doğrulanmış otomatik yenileme kullanır. Yeni domain satın almak gerekmez.
Başlangıçtaki kendinden imzalı sertifika giriş/kurtarma aracıdır; genel panel
kurulumunun tamamlandığına kanıt değildir. Panel adresi, işletim sistemi hostname'i,
nameserver adları ve posta kimliği ayrıdır; birini almak diğerlerini sessizce
değiştirmemelidir.

IP sertifikaları sonraki bir yol olabilir; mevcut CelikPanel yeteneği değildir.
Yalnız özel ağda çalışan kurulumlar ve güven modeli ayrıca açıkça tasarlanmalıdır;
genel hosting profilinin denetimsiz atlama yolu olmamalıdır.

| Gereklilik | Uygulanacağı sınır |
|---|---|
| Güvenilir panel HTTPS'i ve çalışan yenileme | İlk genel kurulumu tamamlandı saymadan önce |
| Panel/SSH erişimini koruyan ve gerekli servislere izin veren güvenlik duvarı | İlk kurulumu tamamlandı saymadan önce |
| Kurulu, sağlıklı ve uyumlu bağımlılıklar | İlgili özelliği açmadan önce |
| Doğru alan adı DNS'i ve site sertifikası | İlgili HTTPS sitesini yayında saymadan önce |
| Posta kimliği, TLS, gerekli DNS/doğrulama kayıtları ve gönderim önkoşulları | İlgili posta hizmetini/alan adını hazır saymadan önce |

Posta hazırlığı SPF, DKIM ve DMARC içerir; ilgili durumda ters DNS ve bağlantı
kontrolleri yapılır. Sunucu hazırlığı ile alan adı hazırlığı ayrılır. Panelin
yetkilendirilmiş entegrasyonu yoksa harici PTR veya kayıt kuruluşu değişikliği
açık bir kullanıcı görevi olarak kalır.

Kontroller yalnız düğme devre dışı bırakılarak değil, sunucuda uygulanır. Eksik
posta önkoşulları web hosting'i engellemez. Başarısız kontrolde düzeltme/yapılandırma
erişimi açık kalır. Hiçbir kontrol mevcut siteleri, postayı, veritabanlarını,
zamanlanmış işleri veya sertifika yenilemeyi kendiliğinden durdurmaz.

## 5. Kullanıcı akışı ve işlem sözleşmesi

**Amaç → Panel adresi ve DNS yöntemi → Planı incele → Kurulumu başlat → Sonucu doğrula**

- Mevcut durumu sınırlı, salt-okur kontrollerle otomatik oku. Sayfa açıldı diye
  yazılım kurma, ayar değiştirme veya yapılandırma yan etkisi olan tarama başlatma.
- Her adımda tek bir ana eylem sun. Yalnız ilgili girdileri göster. Gelişmiş ayrıntılar
  erişilebilir kalsın; ilk ekran Components kataloğuna dönüşmesin.
- Değişiklikten önce somut bileşenleri, ayar değişikliklerini, erişim kurallarını
  ve bekleyen dış görevleri göster. İncelenen planı kullanıcı başlatır.
- Tam plan kimliği ve bağımlılık sırasıyla kalıcı, sürdürülebilir işlemler kullan.
  Sayfa yenileme, yeniden bağlantı veya panel/agent yeniden başlatma işlemi
  kaybettirmemeli ya da iki kez çalıştırmamalıdır. Çakışan işlemler eşzamanlı yürümez.
  HTTP yanıtının kaybolması başarısızlık kanıtı değildir; tekrar denemeden önce
  kaydedilen işlem uzlaştırılır. Otomatik kurtarma yönetilmeyen yapılandırmaların
  üzerine yazmaz veya mevcut müşteri verisini silmez.
- Mevcut denetlenmiş servis/DNS/sertifika işlemlerini kullan. Kurulum, yetkili
  agent'a yeni bir serbest komut çalıştırma yolu kazandırmaz.
- Gerçek aşamaları, sonuçları ve uygulanabilir hataları göster. Harici DNS yayılımı
  bekleme durumudur; tahmini başarı veya uydurma yüzde sayacı değildir.
- Önemli girdi değişikliği önceki incelemeyi geçersiz kılar. Kurtarma tamamlanan,
  başlamayan ve sonucu belirsiz adımları ayırır; paket kurulumunun her zaman
  tamamen geri alınabileceği vaat edilmez.
- Yalnız güncel kanıtlarla tamamlandı de. Sonunda amaca uygun tek sonraki adımı sun:
  ilk siteyi ekle, uygulama yayımla veya DNS bağla/yönet.

Sonrasında Dashboard seçilen görevi, mevcut kaynakları ve işlem gerektiren
sorunları özetler. Kurulum tamamlandığında yeni sunucu kontrol listesini sürekli
göstermemelidir. Components sonraki yönetim işleri için erişilebilir kalır.

## 6. Uygulama sırası

1. **Kalıcı durum ve yönlendirme:** yeni kurulum kanıtı, korumacı yükseltme davranışı,
   yöneticiye özel kurulum API'leri, sürdürülebilir taslaklar ve bağımsız lisans kapısı.
2. **Hazırlık ve DNS modelleri:** kanıt şeması; yerel/mevcut/harici sahiplik;
   alan adı, yetenek ve posta yollarında D-009 kontrollerinin tutarlı yenilenmesi.
3. **Güvenli erişim akışı:** mevcut işlem altyapısıyla panel adresi, güvenilir
   TLS/yenileme ve güvenli güvenlik duvarı değişiklikleri.
4. **Amaç planları ve yürütme:** desteklenen bileşen çözümü, tam plan incelemesi,
   kalıcı iş yürütme, kurtarma ve amaca uygun doğrulama.
5. **Rehber arayüzü ve Dashboard:** iki dilde adımlar, ilerleme, dış görevler,
   devam/hata durumları ve tek faydalı tamamlanma eylemi.
6. **Doğrulama ve yayın:** aşağıdaki kabul durumlarını karşıla, desteklenen yolları
   belgele ve incelenmiş sürümü yayımla. Panellerini kullanıcılar günceller.

Bunlar uygulama bağımlılıklarıdır; desteklenmeyen profilleri vaat eden veya eksik
kontrolleri hazır sayan bir sihirbaz yayımlama izni değildir.

## 7. Kabul durumları

- Yeni, devam eden, hazırlanmış ve belirsiz eski kurulumlar doğru yönlenir;
  yeniden aktivasyon kurulum tekrarı veya mevcut kurulum sıfırlaması yapmaz.
- Yönetici dışındaki roller sunucu kurulum API'lerinden reddedilir; lisans bağımsız
  uygulanır ve lisans kaybı mevcut çalışan işleri durdurmaz.
- Sunulan her amaç desteklenen sunucu yetenekleriyle tamamlanır; gerekli bağımlılıklar
  ve aynı anda bulunamayan servisler doğru ele alınır.
- Yerel DNS, doğrulanmış mevcut DNS ve elle yönetilen harici DNS için tam çalışan
  yollar vardır. Harici DNS seçimi yerel DNS kurulumu başlatmaz.
- Primary/secondary görevleri, yetki, yedeklilik ve yayın doğrulanır;
  yapılandırılmış fakat ulaşılamayan veya güncel olmayan eş hazır sayılmaz.
- Geçersiz/uyumsuz/süresi dolmuş panel sertifikası, başarısız yenileme, eski kanıt,
  yanlış DNS ve eksik posta önkoşulları sahte tamamlanma üretemez.
- Güvenlik duvarı uygulaması mevcut yönetim erişimini korur. Uygulanabilir
  [OPERATIONS.tr.md](OPERATIONS.tr.md) gerçek VM yeniden başlatma/kurtarma kapıları geçer.
- Yenileme, tekrar tıklama, çıkış, eşzamanlı oturumlar, yeniden başlatma ve kesinti
  kurulumu tekrarlamaz veya kaydedilmiş işlemi kaybettirmez.
- Mevcut işler/yapılandırma hazırlık değerlendirmesinden ve ilgisiz hatalardan
  etkilenmez. Profil değişikliği canlı servisleri sessizce kaldırmaz veya sıfırlamaz.
- Türkçe/İngilizce, masaüstü/mobil, klavye erişimi ve uygulanabilir hatalar gerçek
  arayüzde doğrulanır. Başarı gizli bir ek düğmeye basmayı gerektirmez.

- Her kurulum işlemi [D-024 uygulanabilir yönlendirme](OPERATION-GUIDANCE.tr.md)
  kuralına uyar: mevcut neden, eylemi yapacak kişi, sonraki eylem ve devam davranışı
  uzun ilerleme listesinden önce gösterilir. Önkoşul, gözlenmiş hata ve bilinmeyen
  sonuç ayrılır. İki DNS başlatma sırası, seçilen DNS modu, ilgili lisans/kimlik
  bilgisi/sağlayıcı hataları, uzlaştırmada hatanın korunması, gizli bilgi içermeyen
  tanılar ve yenilemenin işlemi tekrarlamaması kapsanır.
## 8. Onay sırasındaki başlangıç durumu

Onay sırasında Dashboard içinde `StartGuide` vardı; kalıcı genel amaç sihirbazı
yoktu. Posta profilleri mevcut plan inceleme/yürütme temelini sağlar.
Alan adı ekleme D-009 altında yerel DNS gerektirir; harici nameserver tespiti,
harici sağlayıcı yönetim modu değildir. Panel sertifikası girdisi `CanonicalFQDN`
kullanır ve IP adreslerini reddeder.

Bu tarihsel başlangıç kaydı geçerli çalışma ağacını anlatmaz;
[uygulama durumuna](SERVER-SETUP-STATUS.tr.md) bakın.

İlgili kaynaklar: `web/src/components/StartGuide.tsx`,
`web/src/components/AddDomainModal.tsx`, `cmd/panel/mail_profiles.go`,
`cmd/panel/dns_engine.go`, `cmd/panel/domain_connection.go`,
`cmd/panel/panel_cert_handler.go`, `internal/hostname/hostname.go`.

## Not, 2026-09-30: Arch'ta e-posta ve bileşen kurulum hataları (upd1 P2)

`web_mail` amacı (ve e-posta bileşeni içeren her plan) Arch Linux'ta inceleme
aşamasında `server_setup_service_unsupported:dovecot` ile reddedilir: Arch'ın
Dovecot 2.4 paketi posta, giriş ve TLS ayarlarını panelin yönetmediği tek bir
`dovecot.conf` içinde tutar; kabul edilen plan önceden kurulumun ortasında e-posta
adımında duruyordu. Bileşen kurulum hatası artık bileşeni, başarısız adımı ve
makinenin kendi satırını adlandırır; sihirbaz kimin ne yapacağını ve kurulumun
nasıl süreceğini söyler. Ayrıntı ve kalan iş: [işlem yönlendirmesi](OPERATION-GUIDANCE.tr.md).
Yalnız bileşen testleri; gerçek sistem kanıtı bekliyor.
