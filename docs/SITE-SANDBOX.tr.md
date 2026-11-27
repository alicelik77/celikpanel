# Site koruması (sandbox)

*alpha.119 · Kaynakta var, bileşen testlerinden geçti; WSL Debian 13 (systemd 257) üzerinde bir kez kısa deneme yapıldı. Desteklenen beş sistemde kaçış matrisi laboratuvarda bekliyor.*

*S4 R2b (22 Kasım 2026) · Bir sitenin zamanlanmış görevleri kendi diliminde zamanlayıcı olarak çalışır (§9); SMTP engeli üyelerinin bu sunucunun kendi gönderim hizmetine sunucunun kendi adreslerinde, yalnız o hizmet CelikPanel'in kendisiyken ulaşmasına izin verir (§3).*

*alpha.127 · Node.js siteleri korumaya katıldı (§1, §6a, §7). Kaynakta var, bileşen testlerinden geçti; deneme takımının Node.js maddeleri (19, 20 ve Node.js sitesinin birimi altında matris) WSL Debian 13 (systemd 257, Node.js 22.23.4) üzerinde kısa denendi. Beş sistemde laboratuvar turu hâlâ açık.*

İngilizcesi: [SITE-SANDBOX.md](SITE-SANDBOX.md).

Müşterinin her programı tek bir systemd korumasında çalışır: yapay zekâ ve otomasyon uygulamaları
(`celiksvc-<id>` ve kurulum adımı `celiksvcinst-<id>`), Python uygulamaları ve yüklenen
programlar (`celikrt-<id>`), alpha.127'den beri de Node.js siteleri (`celikapp-<site no>` ve
kurulum adımı `celikappinst-<site no>`). Her program rastgele kod sayılır. Yüklenen bir Go programı Python
başlatabilir; bu yüzden güvenlik sınırı bu korumadır, sunucuda hangi dillerin kurulu olduğu
değildir.

Kod `internal/sitesandbox` içindedir. Hizmet uygulamaları birimlerini bununla yazar; çalışma
ortamı işi de Python uygulamaları ve yüklenen programlar için bunu kullanır. Yüklenen programlar
[RUNTIME-APPS.tr.md](RUNTIME-APPS.tr.md) belgesinde anlatılır.

## 1. Her programın aldığı koruma

| Alan | Ayarlar |
|---|---|
| Hesap | Sitenin kendi hesabı, asla root değil; `NoNewPrivileges`, boş yetki kümesi, `RestrictSUIDSGID`. |
| Dosyalar | `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp` (/tmp ve /var/tmp birime özeldir, durunca silinir), `PrivateDevices`, `UMask=0077`. Barındırma ağacı boş ve salt okunurdur; içine yalnız programın kendi klasörü bağlanır. Yazılabilen tek yer o klasördür. |
| Gizlenen yollar | `/run/celikpanel` (ajan soketi), `/run/php` ve `/run/php-fpm` (bütün sitelerin PHP-FPM soketleri), `/etc/celikpanel`, `/var/lib/celikpanel*`, `/var/log/celikpanel`, `/root`, `/opt/celikpanel/toolchains`, `/var/spool/postfix` (`sendmail` yolu dahil). |
| Çekirdek | Çekirdek ayarları, modüller, günlükler, saat, makine adı, cgroup ağacı ve ad alanları korunur. `PrivateIPC` açıktır. |
| Sistem çağrıları | `@system-service` grubu kullanılır. Hata ayıklama, bağlama (mount), ham giriş/çıkış, yeniden başlatma, takas, modül, eski çağrılar ve çekirdek anahtarlığı bu gruptan çıkarılır. `process_vm_readv`/`writev` çağrıları da kapalıdır. |
| Süreçler | Başka kullanıcıların süreçleri görünmez. |
| Ağ | Ham soket ve paket soketi açılamaz. Bulut üst veri adresleri (169.254.169.254 ve benzerleri) ile bağlantı yerel (link-local) adresler kapalıdır. Program yalnız kendi döngü (loopback) portlarına bağlanabilir. Başka port, 0 numaralı port ve UDP kapalıdır. |
| Posta | SMTP çıkış engeli (§3). |
| Sınırlar | Bellek (takas yok), işlemci, süreç ve açık dosya sınırları uygulanır. Günlük hızı sınırlıdır: 30 saniyede en çok 1000 satır. Program 5 dakikada 5 kez çökerse yeniden başlatılmaz. |

**Türe göre eklenenler:**

- **Python uygulamaları ve yüklenen programlar:** `/proc` içinde yalnız süreçleri görür (`ProcSubset=pid`).
- **Python uygulamaları bundan fazlasını almaz:** temel küme ve `ProcSubset=pid`; `MemoryDenyWriteExecute` ve `NoExecPaths`/`ExecPaths` yok. Yorumlayıcı ortamındaki kendi eklenti modüllerini çalıştırır, bazı paketler çalışırken kod üretir; bu ayarlar sıradan uygulamaları bozardı. Bu yüzden bir Python uygulaması sunucunun başka programlarını (`/bin/sh` dahil) başlatabilir; neye ulaşacakları bu korumanın izin verdiğidir. Ortamının kurulumu geçici `celikrtbuild-<no>` biriminde çalışır: aynı hesap ve koruma, dinlenen port yok, PyPI için dışa bağlantı açık, posta portu engeli yerinde ([RUNTIME-APPS.tr.md §18](RUNTIME-APPS.tr.md)).
- **Yüklenen programlar:** yazılabilir belleği çalıştıramaz. Yalnız kendi klasöründen ve paylaşılan kitaplıklardan program çalıştırabilir: `/bin/sh`, `python3` ve `ld.so /usr/bin/python3` başarısız olur. Bu ek bir derinlik katmanıdır; asıl sınır değildir.
- **Hizmet uygulamaları:** alpha.118'deki tam `/proc` görünümünü korur. Node.js `/proc/cpuinfo` dosyasını okur ve bu görünüm n8n üzerinde henüz ölçülmedi.
- **Node.js siteleri** (alpha.127, kod `internal/nodesite`): temel küme ve tam `/proc` görünümü
  (hizmet uygulamaları gibi: `os.cpus()` işlemci göremese, işçi sayısını ondan hesaplayan kod hiç
  işçi başlatmazdı; `ProtectProc=invisible` başka hesapların süreçlerini yine gizler).
  `MemoryDenyWriteExecute` yok (V8 JavaScript'i çalışırken derler), `NoExecPaths` yok (npm proje
  betiklerini `/bin/sh` ile çalıştırır). Ayrıca:
  - **Tek Node.js sürümü.** `/opt/celikpanel/runtimes` boş ve salt okunur bir
    `TemporaryFileSystem`; içine yalnız sitenin seçtiği sürüm salt okunur bağlanır. Planın izin
    vermediği bir sürüm tam yoluyla da başlatılamaz; kaynak root'undur, barındırma ağacı dışındadır.
  - **Kendi klasörü.** Uygulama klasörü sitenin belge köküdür (`<site evi>/public_html`) ve
    yardımcı onu her uygulama klasörü gibi bağlar (§1a). `HOME`, `<belge kökü>/.home`'dur (npm
    önbelleği `HOME/.npm`). `PATH` önce sürümün `bin` klasörü, sonra `/usr/bin:/bin`.
  - **Kendi portu.** Siteye verilen port (eskisi gibi 1024–65535; Panel diğer sitelerinkiyle
    karşılaştırır); `SocketBindAllow` yalnız onu açtırır. `PORT`, `HOST=127.0.0.1` ve
    `NODE_ENV=production` verilir.
  - **Başlatma komutu** müşterinin kabuk komut satırı olarak kalır ve sitenin hesabıyla, koruma
    içinde `ExecStart=/bin/sh -c "<komut>"` olarak çalışır. Kabuk tam yazılanı alacak biçimde
    yazılır (`%` → `%%`, `$` → `$$`, `\` ve `"` kaçışlı; satır sonu reddedilir).
  - **Sınırlar** (bu sürümde sabit; önceden Node.js sitesinin sınırı yoktu): `MemoryMax=1536M`,
    `CPUQuota=200%`, `TasksMax=512`; kurulum adımı `MemoryMax=2048M` alır.
  - **Kurulum adımı** `celikappinst-<site no>` (Barındırma sekmesindeki "Paketleri kur ve
    derle"): geçici birim; aynı koruma, port yok, npm deposu için dışa bağlantı açık, posta
    engeli yerinde. `npm ci` (`package-lock.json` yoksa `npm install`) `--no-audit --no-fund
    --foreground-scripts`, ardından `npm run build --if-present` ayrı komut satırları olarak
    sürümün kendi node'uyla çalışır. Yalnız birimi zaten korumada olan sitede çalışır; çalışan
    uygulamaya dokunmaz.

**systemd 257 ve sonrasında:** program kendi ad alanında 1 numaralı süreçtir (`PrivatePIDs`) ve cgroup ağacı sıkı biçimde salt okunurdur.

**Geçici birimler** (`systemd-run --property` ile başlatılan Python kurulumu
`celikrtbuild-<no>` ve hizmet uygulaması kurulumu `celiksvcinst-<no>`) aynı
ayarları alır, ama systemd-run'ın okuyabildiği biçimde. systemd 257'nin
systemd-run'ı `ProtectControlGroups=strict` değerini kabul etmez; birim dosyası
eder. Bu yüzden geçici birimde aynı etkiyi veren `ProtectControlGroupsEx=strict`
kullanılır. alpha.119 deneme turu 2'de her Python kurulumu "Failed to parse
ProtectControlGroups: strict" hatasıyla başlayamamıştı. systemd-run'a verilen
her ad, onu okuyan en eski systemd ile birlikte
`sitesandbox.TransientPropertyMinimums` içinde yazılıdır. Testler her listeyi
systemd 249, 252, 255, 256, 257, 258 ve 262 için denetler.

## 1a. App klasörü korumaya nasıl girer

systemd bir birimin yollarını root olarak, ana sistemde çözer ve bağlantıları izler:
`BindPaths=` kaynağı ve `EnvironmentFile=`. Site klasörü sitenin hesabınındır. Hesap,
CelikPanel'in denetimiyle başlatma arasında ya da sonraki bir yeniden başlatmadan önce app
klasörünün adını değiştirip yerine bir bağlantı koyabilir. Birim klasörü adıyla anarsa systemd
bağlantının gösterdiği yeri bağlar: başka bir sitenin klasörünü ya da `/etc/celikpanel`'i. Unix
izinleri yine geçerli olurdu, ama koruma onlara dayanmamalı.

Bu yüzden hiçbir birim ana sistemde site klasörünün içindeki bir yolu anmaz. Her korumalı birim
bir **bağlama birimi** gerektirir: `celikpanel-sandbox@<birim adı>.service` (`Requires=` ve
`After=`); şablonu `/etc/systemd/system/celikpanel-sandbox@.service`. Bağlama biriminde hiçbir
koruma ayarı yoktur; **koruma yardımcısını** root olarak ana sistemin bağlama ad alanında çalıştırır
(ayrı birim gerekir: systemd `BindPaths=` ve `TemporaryFileSystem=` ayarlarını birimin her
komutu için, `ExecStartPre=+` dahil, kurar). Korumalı birim başlamadan önce yardımcı:

1. Korumalı birimin kendi `User=`, `BindPaths=` ve `EnvironmentFile=` ayarlarını systemd'den
   okur (`systemctl show`). Hangi klasörü bağlayacağını başka hiçbir şey söylemez.
2. App klasörünü `/var/www/celikpanel/subscriptions` içinden, her seferinde bir klasör olmak
   üzere `openat2(RESOLVE_BENEATH | RESOLVE_NO_SYMLINKS | RESOLVE_NO_MAGICLINKS)` ile açar.
   Yoldaki her bağlantı reddedilir. Site klasörünün altında bağlama noktası da reddedilir
   (`RESOLVE_NO_XDEV`).
3. Sahipleri denetler: abonelik klasörü ve onun `sites` klasörü root'undur, başkası yazamaz.
   Site klasörü ve altındaki her klasör sitenin hesabınındır. Hesabın ev klasörü o site
   klasörü olmalı ve hesap olağan bir hesap olmalı (uid 1000 ya da üstü).
4. Açtığı klasörü adıyla değil tanımlayıcısıyla `/run/celikpanel-sandbox/<birim>/app` noktasına
   `nosuid` ve `nodev` olarak, alt bağlamalar olmadan bağlar (`open_tree`, `mount_setattr`,
   `move_mount`; bunlar olmayan çekirdekte `/proc/self/fd/<n>` ile `mount`). Sonra bağlama
   noktasının tam o klasörü gösterdiğini denetler. `/run/celikpanel-sandbox` ve her birimin
   klasörü root'undur, kipi 0700.

Yardımcı hiçbir ortam dosyası okumaz. Bir birimin değerleri yalnız barındırma ağacının dışındaki,
root'a ait ve Ajan'ın bağlantı izlemeden yazdığı dosyadan gelir: hizmet uygulaması için
`/etc/celikpanel-svcapps/celiksvc-<id>.env` (alpha.118 güvenlik düzeltmesi,
[AI-APPS.tr.md](AI-APPS.tr.md) §5), çalışma zamanı uygulaması için `/etc/celikpanel-runtime/`
([RUNTIME-APPS.tr.md](RUNTIME-APPS.tr.md)). Site klasöründeki hiçbir dosya ve onun hiçbir kopyası
bir birimin ortamı olmaz.

Korumalı birimde `BindPaths=/run/celikpanel-sandbox/<birim>/app:<app klasörü>` bulunur: program
klasörünü olağan yolunda, bunun dışında boş olan barındırma ağacında görür.
`WorkingDirectory=`, `ExecPaths=` ve `ExecStart=` de bu yolu anar, ama systemd onları koruma
içinde çözer; orada bu yol bağlanan klasördür. Bağlama klasörün kendisini tutar: klasörün adı
sonradan değişse ya da yerine bağlantı konsa çalışan program etkilenmez.

Bağlama birimi korumalı birimin parçasıdır (`PartOf=`): korumalı birimin her başlatılması,
durdurulması ve yeniden başlatılması (panelden, sahipten ya da açılışta) onu da başlatır,
durdurur ya da yeniden başlatır; her başlatma yeniden denetler. Korumalı birim artık çalışmıyorsa
bağlama birimi kendiliğinden durur (`StopWhenUnneeded=`); çıkmış bir program sonraki başlatmada
yeniden denetlenir. Bir çökmeden sonraki otomatik
yeniden başlatma (`Restart=on-failure`) zaten denetlenmiş bağlamayı korur.

Yardımcı kapalı kalır. Bir denetim ya da bağlama başarısız olursa bağlama birimi başarısız olur
ve korumalı birim başlamaz ("bir bağımlılık işi başarısız oldu"). Bağlama biriminin günlüğü
klasörü ve nedeni İngilizce ve Türkçe yazar (`journalctl -u celikpanel-sandbox@<birim adı>`).
Panel `app_folder_not_real` (bağlantı, klasör değil, bağlama noktası: müşteri gerçek klasörü
geri koyar) ya da `app_folder_not_owned` (yönetici sahibini düzeltir) gösterir. Ajan da aynı
yürüyüşü, bağlamadan, bir birimi yazmadan ya da başlatmadan önce ve başlatma başarısız olduğunda
yapar. Bağlama birimini durdurmak bağlamayı kaldırır.

**Hiçbir root komutu müşterinin ortamını dinamik yükleyiciyle almaz.** systemd bir birimin her
komutuna birimin ortamını verir; bu ortamda müşterinin girdiği değerler vardır. Korumalı
birimin root komutları yalnız yardımcının `egress-add`/`egress-remove` çağrılarıdır (§3):
yardımcı statik bir Go programıdır, ortamını yok sayar ve nft'yi boş bir ortamla başlatır.
3. kuşaktan önce birimler nft'yi bu ortamla doğrudan root olarak çalıştırıyordu.

**Yardımcı**, `sandbox-helper` adıyla başlatılan ajan programıdır. Ajan kendini
`/usr/lib/celikpanel-sandbox/sandbox-helper` olarak kopyalar (§4); böylece birimler CelikPanel
kaldırılsa da başlar.

| Sistem | Çekirdek | `openat2` (5.6) | `open_tree`/`move_mount` (5.2) | `mount_setattr` (5.12) | systemd |
|---|---|---|---|---|---|
| Ubuntu 22.04 | 5.15 (GA), 6.x (HWE) | var | var | var | 249 |
| Debian 12 | 6.1 | var | var | var | 252 |
| Ubuntu 24.04 | 6.8 | var | var | var | 255 |
| Debian 13 | 6.12 | var | var | var | 257 |
| Arch | güncel | var | var | var | 258+ |

Yardımcı üç tür uygulama klasörü kabul eder: çalışma ortamı uygulamasının `<site evi>/app`,
hizmet uygulamasının `<site evi>/apps/<uygulama>` klasörü ve alpha.127'den beri Node.js sitesinin
belge kökü `<site evi>/public_html`.

systemd tarafı (şablon birim, `Requires=`/`After=`/`PartOf=`, `BindPaths=kaynak:hedef`,
`RemainAfterExit=` ile `Type=oneshot`) desteklenen her sürümden eskidir. `openat2` olmayan
çekirdekte yardımcı hiçbir şey başlatmaz (`app_folder_unreadable`); ada göre yürüyüşe asla geri
dönmez. WSL Debian 13 üzerinde (systemd 257, çekirdek 6.18) bir kez çalıştırıldı: bağlama,
sitenin hesabından yazma, çalışırken bağlantıyla değiştirme (program kendi klasörünü korudu),
reddedilen yeniden başlatma ve temiz durdurma. Beş sistem laboratuvarda hâlâ ölçülecek.

## 2. Sistemlere göre ve eksik (reduced) düzey

Her ayar yalnız sunucudaki systemd onu tanıyorsa yazılır. BPF olmadan kabul edilip sessizce
uygulanmayacak bir ayar hiç yazılmaz.

| Ayar | Gerekir | Ubuntu 22.04 (249) | Debian 12 (252) | Ubuntu 24.04 (255) | Debian 13 (257) | Arch (258+) |
|---|---|---|---|---|---|---|
| ProtectProc, ProcSubset | 247 | var | var | var | var | var |
| PrivateIPC, ExecPaths/NoExecPaths | 248 | var | var | var | var | var |
| SocketBind | 249 + BPF | BPF varsa | BPF varsa | BPF varsa | BPF varsa | BPF varsa |
| IPAddressDeny | 235 + BPF | BPF varsa | BPF varsa | BPF varsa | BPF varsa | BPF varsa |
| PrivatePIDs, ProtectControlGroups=strict | 257 | — | — | — | var | var |
| SMTP çıkış engeli | nft `socket cgroupv2` + cgroup v2 | yoklama tabloyu kabul etti (nft 1.0.2, alpha.123 laboratuvarı) | var (beklenen) | var (beklenen) | var (beklenen) | var (beklenen) |

Ubuntu 22.04 ile Ubuntu 24.04 aynı davranır (sahibin kararı, 5 Kasım 2026). İkisi de alpha.123
laboratuvarında tek eksikle (`bpf_missing`; systemd 249 ve 255 BPF desteği olmadan derlenmiş) **azaltılmış**
ölçüldü; yüklenen programlar, Python uygulamaları ve hizmet uygulamaları ikisinde de aynı ayarları, aynı
kanıtı ve aynı sunumu alır; yalnız dağıtımın Python sürümü farklıdır (3.10 ve 3.12). Debian 12 de
azaltılmış, Debian 13 tamdır. Kodda dağıtıma bağlı bir ayrım yoktur: düzey ölçülen yeteneklerden gelir.

Ajanın salt okuyan yoklaması (`SandboxCapsV1`) dört şeye bakar: systemd sürümü ve BPF desteği,
cgroup v2 ve nft'nin çıkış tablosunu `--check` ile kabul edip etmediği. `--check` hiçbir şeyi
uygulamaz; yoklama sunucuda hiçbir şeyi değiştirmez.

Bunların hepsi tutarsa düzey **tam**, biri tutmazsa **eksik** olur. Ekran her eksiği adıyla gösterir:

- `systemd_old` ya da `systemd_unknown`: systemd 249 sürümünden eski ya da sürümü okunamadı.
- `bpf_missing`: port bağlama kuralı ve üst veri engeli uygulanmıyor. Dinleme denetimi,
  dışarıya açılan programı yine durdurur.
- `cgroup_v2_missing`: sunucu cgroup v2 kullanmıyor.
- `nft_cgroup_missing`: posta engeli yok. Bu sunucuda programlar yerel posta sunucusu
  üzerinden e-posta gönderebilir.

Eksik düzeyde bir çalışma ortamı yalnız yönetici o eksiği kabul ederse sunulur. Hizmet
uygulamaları düzeyler çıkmadan önce sunulmuştu. Sunucu hangi düzeyi uyguluyorsa o düzeyde
çalışırlar ve yönetici sayfası bu düzeyi gösterir. Node.js siteleri de düzeylerden önce vardı ve
aynı biçimde çalışır: sunucunun uyguladığı düzeyde; düzey ve adı konmuş eksikler Bileşenler →
Node.js altında gösterilir. Ubuntu 22.04 ve 24.04 onlara aynı birimi verir
(`TestNodeUbuntu2204IsUbuntu2404`).

## 3. SMTP çıkış engeli

Postfix, 127.0.0.1'den gelen postayı parola istemeden iletir. Engel olmasaydı sunucudaki her
program sunucu adına e-posta gönderebilirdi.

Engel ayrı bir nftables tablosundadır: `inet celikpanel_egress`. Tablo yerel bir dosyada
tanımlıdır: `/usr/lib/celikpanel-sandbox/egress.nft`. Güvenlik duvarının `inet celikpanel_fw`
tablosuna asla girmez.

- Korumalı birimlerin cgroup'ları `smtp_blocked` kümesinde tutulur.
- Bu programların 25, 465 ve 587 numaralı portlara her adrese (127.0.0.1 ve ::1 dahil) açtığı
  bağlantılar, aşağıdaki tek ayrık dışında reddedilir. Reddedilen her deneme `smtp_rejects`
  sayacına yazılır.
- **Bu sunucuya gönderim, yalnız güvenli olduğu yerde (S4 R2b, 22 Kasım 2026, düzeltme turunda
  daraltıldı; kümedeki her birim için geçerlidir, bir sitenin programları da zamanlanmış görevleri
  de).** Reddetme kuralından önceki bir kural, yerel arabirimden (`oifname "lo"`) çıkan 465 ve 587
  numaralı port bağlantılarını kabul eder: bu sunucunun kendi adreslerinden herhangi birine giden
  trafik oradan çıkar; 127.0.0.1, ::1 ve sunucunun genel adresleri aynı. Böylece sunucunun adına
  bağlanan bir görev, sertifikanın taşıdığı ada ulaşır. Ajan bu kuralı **yalnız** `master.cf`
  içindeki iki gönderim kaydı da CelikPanel'in kendi, değişmemiş kayıtlarıyken
  (`submission/inet` ve `smtps/inet`, `cmd/agent/mail_submission_rpc.go`'nun yazdığı biçimde) ve
  yalnız bir posta kutusu girişini kabul ederken (`smtpd_sasl_auth_enable=yes`,
  `smtpd_client_restrictions=permit_sasl_authenticated,reject`) ve girişi yapmış bir posta kutusunun
  yalnız kendi adresleriyle göndermesine izin verirken (herhangi bir `permit`'ten önce
  `reject_sender_login_mismatch` ve bir `smtpd_sender_login_maps`; ikinci düzeltme turu: eksik ya da
  okunamayan bir ayar, ayrık yok demektir), başka hiçbir `master.cf` kaydı 465
  ya da 587'de dinlemiyorken ve orada dinleyen her soket Postfix'in ana sürecine aitken yazar
  (`cmd/agent/site_tasks_mail.go`). Bu portlarda sahibin kendi kaydı ya da başka bir program (bir
  msmtpd, bir akıllı sunucuya giden stunnel) varsa ya da hiç gönderim hizmeti yoksa dosyada ayrık
  yoktur ve sitenin Genel Bakış kartı, sitelerin görevlerinin ve programlarının bu sunucu üzerinden
  e-posta gönderemediğini ve nedenini söyler. Karar her Panel açılışında ve posta gönderim adımından
  sonra yeniden verilir; değişen kural takımı hemen çalışan tabloya yüklenir. **Karar, Panel'in bir
  sonraki açılışına kadar son verildiği gibi kalır; panel olmadan da kalıcı olarak öyle kalır**:
  sonradan 465 ya da 587'ye başka bir program koyan sahip kuralı elle kaldırır (dosyanın kendi başlığı
  nasıl yapılacağını söyler). Sunucu sahibi engelin dosyasını değiştirdiyse dosya korunur ve kart,
  kararı korunan dosyanın verdiğini söyler (CelikPanel'in yazacağını değil). 25 numaralı port her
  yerde kapalı kalır: Postfix'in aktarma listesi alpha.125'ten beri bu sunucunun kendi adreslerine
  parolasız izin vermiyor, engel de onu yine kapalı tutar.
- **Bilinen sınır: bu yoldan parola tahmini (erişim dilimi R4'ün ön koşulu).** CelikPanel henüz kendi
  sınırlayıcısını eklemiyor. Bugün hâlâ engelin dışında çalışan bir sitenin PHP'si aynı biçimde tahmin
  edebiliyor, yani adım küçük; R3 PHP'yi engelin içine aldığında tek kapı bu yol olur ve R4 başka bir
  şey açmadan önce onu sınırlamalıdır. Bugün olan: Postfix'in `smtpd_client_auth_rate_limit` değeri
  varsayılanda 0'dır (sınır yok) ve olay sınırları varsayılanda `$mynetworks`'ü (yerel arabirim)
  dışarıda tutar (`smtpd_client_event_limit_exceptions`); bir oturum 20 hatadan sonra kapatılır
  (`smtpd_hard_error_limit`), onuncudan sonraki her hatada bir saniye beklenir; Dovecot her
  başarısız girişi ancak `auth_failure_delay` sonra yanıtlar (varsayılan 2 saniye; belgeleri yerel
  istemciler için bir ayrık saymıyor). Bir sitenin kodu aynı anda çok oturum açabilir; bu tahmini
  yavaşlatır ama durdurmaz.
- Program kendi portunda 0.0.0.0 açsa bile dışarıdan gelen yeni bağlantı düşürülür.
- Dosyayı yeniden yüklemek zararsızdır ve tek seferde (atomik) olur. Yalnız iki zincir
  boşaltılıp yeniden doldurulur; kümedeki üyeler ve sayaç korunur. Bu, nft 1.1.3 üzerinde bir
  ağ ad alanında ölçüldü.

Engeli tutabilen sunucuda her korumalı birim yardımcıyı (§1a) çalıştırır:
`ExecStartPre=+… sandbox-helper egress-add --unit=<birim>` tabloyu yükler ve birimin gerçek
cgroup'unu (`'{ "<dilim>/<birim>" }'`) kümeye ekler; `ExecStopPost=-+… egress-remove` durduktan
sonra çıkarır. nft boş bir ortamla çalışır. Yardımcı cgroup'u şu sırayla bulur: birimin içinde
çalıştığı için önce kendi `/proc/self/cgroup` satırı (tam olarak `/system.slice/<birim>` ya da
`/celiksite_<id>.slice/<birim>` ise); değilse systemd'nin söylediği `ControlGroup`; systemd'ye
sorulamazsa ya da bir yer söylemezse eskisi gibi sistem dilimi. Kendi cgroup satırı
kullanılmadığında birimin günlüğüne tek satır yazılır: nedeni, hangi adımın karar verdiği ve alınan
yer. Böylece posta engeli yardımcısı systemd'nin yanıt vermesine bağlı kalmaz; bağlama biriminin
önceki sürümlerden beri aynı kalan kendi okuması ise systemd yanıt vermediği sürece başlatmayı yine
durdurur. Birim şu durumlarda başlamaz: nft tabloyu yükleyemezse,
systemd başka bir yer söylerse (örneğin sahibin bir ek ayar dosyası birimi başka bir dilime
taşıdıysa) ya da birimin yeri belirlenemezse. nft var olmayan bir cgroup yolunu kabul etmediği
için, başka yerde çalışan bir birime sistem diliminin öğesi verilemez; engel kapalı kalacağına
program hiç çalışmaz. Günlükte neyin belirlenemediği, sunucu yöneticisinin ne yapacağı ve yeri
gösteren komut yazar: `systemctl show -p ControlGroup,Slice <birim>`.

Sitenin dilimi, bu sürümün bir sitenin programlarını koyduğu yerdir (§8: çalışmıyorken yazılan
program dilime girer; çalışan program, yönetici siteyi taşıyana kadar çalıştığı yerde kalır). Bir
programın birim dosyası sitenin sınırları uygulanamazken yazılırsa (bu sunucu sınırları tutamıyorsa,
§8, ya da istekteki sınırlar aralık dışındaysa), zaten kendi sitesinin dilimini gösteren birim
dosyası `Slice=celiksite_<id>.slice` satırını bir önceki sürümdeki gibi yalnız şu durumda korur: ana
birim dosyasındaki son `Slice=` satırıdır (ek ayar dosyaları okunmaz), birimin kendi sitesinin
dilimidir ve `/etc/systemd/system/celiksite_<id>.slice` sıradan bir dosyadır (bağlantı ya da klasör
değil). Yoksa böyle bir yazma birime eskisi gibi `Slice=system.slice` verir.

`celikpanel-egress-guard.timer` 30 saniyede bir çalışır. Biri `nft flush ruleset`
çalıştırdıysa ya da `nftables.service` yeniden başlatıldıysa tabloyu yeniden yükler ve çalışan
korumalı birimleri (programların ya da görevlerin kendi yardımcı kopyasını çalıştıranları, §9.4),
bir de biten bir görevin çalışır bıraktığı bir süreci hâlâ tutan, bir sitenin dilimindeki her görev
cgroup'unu yalnız yoluna bakarak (`celiksite_<id>.slice` altındaki `celikcron-<id>-<n>.service`;
görevin birim dosyası kaldırılıp systemd onun `ExecStartPre`'sini artık göstermediğinde de, §9.2)
yeniden ekler. Bu arada engel en çok 30 saniye yok olabilir. Önceki bir
sürümün yazdığı birim kümeye asla sessizce eklenmez. alpha.127'den beri koruyucu `celikapp-*` ve
`celikappinst-*` birimlerini de kapsar; önceki sürümün yazdığı Node.js birimi yardımcı satırı
taşımadığı için dışarıda kalır.

**Kapsam dışı:** 2525 numaralı port, web posta API'leri üzerinden gönderilen e-posta ve
programın kendi genel dış trafiği. Programın dış trafiği aboneliğin trafiğine de sayılmaz.

## 4. Yerel dosyalar ve sahibin bağımsızlığı (D-022, D-031)

Ajan bu dört dosyayı D-031 kuralıyla yazar. Her dosyanın başında içeriğin özetini taşıyan bir
başlık satırı bulunur:

- `/usr/lib/celikpanel-sandbox/egress.nft`
- `/usr/lib/celikpanel-sandbox/egress-guard.sh`
- `celikpanel-egress-guard.service`
- `celikpanel-egress-guard.timer`

Ajan yardımcıyla birlikte bağlama birimlerinin şablonunu
(`/etc/systemd/system/celikpanel-sandbox@.service`) da aynı kuralla, her sunucuda yazar.

Altıncı dosya yardımcıdır: `/usr/lib/celikpanel-sandbox/sandbox-helper`, ajan programının
kopyası (0755, root'un). Ajan onu yoksa yazar ve bir güncellemeden sonra kendi önceki kopyasını
yeniler. Sahibin yerine koyduğu dosya korunur ve olduğu gibi kullanılır. Yardımcı CelikPanel
olmadan da çalışır.

Her yazma `/var/lib/celikpanel-agent-private/sandbox/native-files.json` dosyasına kaydedilir.

Ajan eksik dosyayı yazar ve CelikPanel'in değişmemiş metnini yeniler. Sahibin değiştirdiği
dosya korunur; CelikPanel kendi metnini yanına `.celikpanel-pending` adıyla koyar. Sahibin
dosyası yüklenmezse korumalı birimler başlamaz.

Hepsi CelikPanel olmadan da çalışır. Engeli kaldırmak için:
`systemctl disable --now celikpanel-egress-guard.timer` ve
`nft delete table inet celikpanel_egress`. Ajan bu dosyaları yalnız bir değişiklik işleminde
yazar; bir durum okuması bunları asla yazmaz.

## 5. Yayımlamadan önce kanıt

İşlem `verify` adımına gelince ajan çalışan birimi profiliyle karşılaştırır. Üç şey denetlenir:

- `systemctl show` profille uyuşuyor; bağlama biriminin `Requires=`/`After=` satırları, onun
  bağlama noktasından gelen `BindPaths=` ve yardımcının çıkış satırları dahil.
- Hiçbir bağlama kaynağı ve ortam dosyası ana sistemde barındırma ağacında değil (örneğin
  sahibin bir ek dosyasından: `site_path_on_host`).
- Birim SMTP kümesinde ve engel kuralı yüklü.
- Birim yalnız döngü (loopback) adresinde dinliyor.

**Node.js siteleri**, ajanın yaptığı her başlatmadan sonra (yeni site, ayar değişikliği, §6a'daki
taşıma) birim çalışıp portunu dinlediğinde (en çok 20 saniye) kanıtlanır. Program `listen(port)`
çağrısını adres vermeden yaparsa Node.js her adresi dinler. Çıkış engelini tutan sunucuda giriş
zinciri korumalı bir sokete dışarıdan gelen her yeni bağlantıyı düşürür; bu yüzden sitenin kendi
portundaki joker dinleyici döngü adresi sayılır (`sitesandbox.ProveNodeListening`). Başka bir
porttaki joker ya da adı verilmiş açık bir adres `listens_elsewhere` olur. Engel yoksa joker her
tür için olduğu gibi açıkta sayılır. Henüz kodu olmayan yeni site (çalışmıyor, dinlemiyor)
bildirilir, reddedilmez. Zaten korumada olup kanıtı tutmayan birim durdurulur ve yayımlanmaz.

Uyuşmayan ya da okunamayan her şey kodlu bir rettir. Panel uygulamayı durdurur ve yayımlamaz
(`sandbox_proof`).

## 6. Çalışan uygulamaları yeni korumaya geçirmek

Önceki bir sürümün yazdığı birim (kuşak işareti olmayan ya da 3'ten küçük) sessizce yeniden
yazılmaz. 3. kuşak, bağlama birimi ve uygulama klasörünün root'a ait bağlamasıdır (§1a); ortam dosyası yine root'un `/etc/celikpanel-svcapps/celiksvc-<id>.env` dosyasıdır. Alan adı sayfası "önceki sürümün
korumasıyla çalışıyor" der. Yönetici sayfası kaç uygulamanın beklediğini gösterir ve tek bir
düğme sunar: "Yeni korumayı N uygulamaya uygula".

- Düğme sayfada görülen sayıyı gönderir. Sayı o arada değiştiyse istek reddedilir.
- Panel'de aynı anda yalnız bir uygulama işlemi çalışır.
- Kaybolan bir yanıtın tekrarı ilk yanıtı döndürür; hiçbir şeyi ikinci kez başlatmaz.
- Çalışan her uygulama kalıcı bir işlemle bir kez yeniden başlatılır. Koruması kanıtlanır, sonra
  yeniden yayımlanır.
- Durdurulmuş bir uygulamanın birimi yeniden yazılır; uygulama durmuş kalır.
- Sahibin değiştirdiği birime dokunulmaz.
- İşlem denetim kaydına `service_app.sandbox_refresh` olarak yazılır.

### 6a. Node.js siteleri (alpha.127)

Node.js sitesinin birimi adını korur: `celikapp-<site no>`. Ajanın hangi metni yazacağı istekten ve
sunucudaki dosyadan anlaşılır:

- yeni site (henüz birim dosyası yok) ve zaten korumada olan birim korumayı alır;
- önceki bir sürümün yazdığı birim (`# celikpanel-sandbox-generation` işareti yok) her sıradan
  yazmada (ayar değişikliği, Node.js sürümü taşıma, site yapılandırma sayfası) o sürümün metnini
  korur; asla sessizce yeniden yazılmaz. Sitenin Barındırma sekmesi "önceki bir sürümün
  korumasında çalışıyor" der ve kimin taşıyacağını söyler;
- hâlâ ortak web hesabıyla çalışan uygulama dışarıda kalır: koruma sitenin kendi hesabını ister,
  önce "Sitenin kendi kullanıcısıyla çalıştır" (D-038 §6) gelir.

Bileşenler → Node.js sunucunun düzeyini ve eksiklerini, korumada kaç site olduğunu gösterir ve
tek bir işlem sunar:

`POST /api/v1/runtimes/node/sandbox-refresh {request_id, expected}`

- `expected` sayfada görülen sayıdır; sayı değiştiyse istek reddedilir.
- Panel genelinde aynı anda tek taşıma; çalışırken ikinci basış reddedilir, aynı `request_id`
  ilk yanıtı döndürür.
- Siteler sırayla taşınır (`ApplyAppUnit`, `sandbox_move`). Çalışan uygulama korumada bir kez
  yeniden başlar ve kanıtlanır (§5). Kanıt taşımayı çürütürse ya da önceden portunda yanıt veren
  uygulama artık vermiyorsa ajan önceki birimi geri yazar ve uygulamayı onunla yeniden başlatır
  (geri alma; dayanıklılık sözleşmesi ilke 4). Durdurulmuş uygulama yeni birimi alır ve durmuş
  kalır (`no_start`).
- Sahibin değiştirdiği birimler korunur ve listelenir, taşınmaz. Ortak web hesabındaki siteler
  sonraki adımlarıyla listelenir.
- Yazılan her dosya site dosyası defterine kaydedilir; işlem denetim kaydına
  `site.node_sandbox_refresh` olarak yazılır.

Kurulum adımı (§1) yalnız site korumaya girince sunulur:
`POST /api/v1/domains/{id}/app/packages {request_id, build}` başlatır (aynı `request_id` ilk
başlatmayı yanıtlar; biri çalışırken ikincisi reddedilir), `GET` yalnız okur ve hiçbir şey
başlatmaz. Denetim kaydına `site.node_install` olarak yazılır.

## 7. Kaçış denemeleri

Deneme takımı `deploy/e2e/sandbox-escape/` içindedir. Statik bir Go yoklaması ve bir Python
yoklaması, korumalı birimler altında matrisin 1–18. maddelerini dener. Her deneme başarısız
olursa takım PASS bildirir. Desteklenen beş sistemde laboratuvar turu hâlâ açıktır.

**Node.js siteleri (alpha.127).** `NODE_VERSION=<sürüm>` verilirse (`/opt/celikpanel/runtimes/node`
altında bir Node.js) takım bir Node.js sitesinin birimini ajanın kendi üreticisiyle yazar
(`render -what node-unit`) ve Go yoklamasını sitenin kodu olarak başlatma komutuyla çalıştırır;
1–16. maddeler Node.js profili altında değerlendirilir (14. madde SKIP: npm `/bin/sh` ister). İki
madde eklenir:

| # | Deneme | Nasıl değerlendirilir |
|---|---|---|
| 19 | Başka bir Node.js sürümü; çalışma ortamına yazma; araç takımları | Sitenin sürümünün yanındaki sahte bir sürüm listelenmemeli; sitenin kendi `bin/node`'u ulaşılabilir olmalı; çalışma ortamına yazma ve `/opt/celikpanel/toolchains` listesi başarısız olmalı |
| 20 | Kurulum adımı | Üretilen kurulum bağımsız değişkenleriyle (`render -what node-install`) `systemd-run` kabul etmeli; npm, `postinstall` betiği yoklamayı bir kez çalıştıran ve `build` betiği node çalıştıran bir projeyi kurar; hiçbir deneme kaçmamalı, adım `success` ile bitmeli ve derleme dosyasını yazmış olmalı |

`sudo NODE_VERSION=<s> ./run.sh node` yalnız bunları çalıştırır; `matrix` ve `quick` bunları da
içerir. WSL Debian 13 üzerinde (systemd 257; çekirdekte nft `socket cgroupv2` yok, düzey
`nft_cgroup_missing` ile eksik) 1–8, 10, 12–16, 19 ve 20. maddeler geçti; 9. madde adı konmuş
eksiğin söylediği gibi başarısız oldu (o sunucuda posta engeli yok). Laboratuvar için açık
kalanlar: beş sistem, çıkış tablosu yüklüyken joker Node.js dinleyicisine dışarıdan ulaşılamadığı
(10. madde) ve her sistemde yöneticinin işlemiyle taşınan gerçek bir Node.js uygulaması (Express,
Next.js).

## 8. Sitenin dilimi: bir sitenin bütün programları için tek sınır takımı

*21 Kasım 2026, ikinci düzeltme turu 22 Kasım 2026, üçüncüsü 23 Kasım 2026 · Kaynakta var ve bileşen testleriyle
denendi; aşağıdaki gerçekler geçici Debian 13 ve Ubuntu 24.04 makinelerinde ölçüldü; panelin iki
sistemdeki laboratuvar turu açık (adımlar çekme isteğinde).*

CelikPanel'in bir site için systemd birimi olarak başlattığı her program o sitenin dilimi
`celiksite_<site kimliği>.slice` içinde çalışır. Dilimin `MemoryMax=`, `MemorySwapMax=0`,
`TasksMax=` ve `CPUQuota=` değerleri bu programların **hepsini birlikte** sınırlar:

| Birim | Ne | Dilimde mi |
|---|---|---|
| `celikapp-<site kimliği>` | Node.js uygulaması | evet |
| `celikrt-<uygulama kimliği>` | yüklenmiş bir program ya da Python uygulaması | evet |
| `celiksvc-<kurulum kimliği>` | bir hizmet uygulaması (n8n) | evet |
| `celikappinst-`, `celikgit-<site kimliği>` | Node.js kurulum adımı, Git ile dağıtım | evet (her seferinde yeni birim) |
| `celikrtbuild-`, `celiksvcinst-<kimlik>` | Python derlemesi, bir hizmet uygulamasının kurulumu | evet (her seferinde yeni birim) |
| `celikcron-<site kimliği>-<n>` | taşınmış bir sitenin zamanlanmış bir görevi (§9) | evet (S4 R2b) |
| sitenin PHP-FPM havuzu | | henüz değil: PHP dilime R3'te girer |
| `celikpanel-sandbox@…`, `celikpanel-ftp@…` | root'un bağlama birimleri | hayır: müşteri kodu çalıştırmazlar |

**Henüz sınırlanmayanlar.** Açıkça: bir sitenin diliminin dışında çalışan programları (R2a'dan
önce yazılmış her program, yönetici siteyi taşıyana kadar; böyle bir program, taşınana kadar
sunucunun her yeniden başlatılmasından sonra yine), sitenin PHP'si (PHP-FPM havuzu; R3), henüz
taşınmamış bir sitenin crontab'ı (§9) ve bir site hesabının R2b'den önce açık tuttuğu kullanıcı
yöneticisi ("kalıcı oturum", §9.5) sitenin sınırlarıyla sınırlanmaz.

**Ad.** systemd için dilim adındaki tire iç içe geçme demektir (`celiksite-7.slice`,
`celiksite.slice`'ın alt dilimi olurdu). Tire olmayınca dilim kökün hemen altındadır ve içindeki
birim `/celiksite_7.slice/<birim>` olur: `/system.slice/<birim>` gibi ikinci düzey. Bu yüzden SMTP
engelinin kümesi (`typeof socket cgroupv2 level 2`, §3) birimi iki yerde de tabloda hiçbir
değişiklik olmadan yakalar. Yardımcının `egress-add` komutu birimin gerçekten çalıştığı cgroup'un
öğesini ekler; bu yeri §3'teki sırayla bulur (önce kendi cgroup satırı, sonra systemd'nin
`ControlGroup=` değeri, sonra sistem dilimi; birim başka yerde çalışıyorsa nft bu yolu kabul etmez).
Yalnız sistem dilimini ya da bir sitenin dilimini kabul eder ve başka her yeri reddeder; sahibin
eklemesiyle başka yere taşınan birim (önceden olduğu gibi) başlamaz. Ajan bir programın nerede
çalıştığını aynı kuralla okur (`sitesandbox.UnitControlGroupAllowed`). Bekçi zamanlayıcısı birimleri
iki yerde de yeniden ekler.

**Sınırlar.** Dilim sitenin etkin `site.memory_mb`, `site.tasks` ve `site.cpu_percent` değerlerini
alır (docs/SITE-LIMITS.md §4a). systemd bir birimle üst dilimlerinin en sıkı sınırını uygular
("EffectiveMemoryMax= … is the most stringent limit of the unit and parent slices",
systemd.resource-control(5), systemd 257 ve 255). Birim kendi sınırlarını dosyasında tutar (Node.js
uygulaması 1536 MB, n8n 1536 MB); dilim dış sınırdır. Hiçbir düzey sınır koymamışsa dilim 512
işlem, %200 ve bellek için **3072 MB ile sunucu belleğinin %75'inden küçük olanı** alır
(`/proc/meminfo` içindeki `MemTotal`, tam MB'ye aşağı yuvarlanır, hiçbir zaman 64 MB'den az değil);
böylece tek bir site varsayılanla bütün sunucuyu alamaz. Bu bellek varsayılanı **ilk
hesaplandığında sabitlenir** (panelin bu sürümle ilk açılışında; hesaplandığı sunucu belleğiyle
birlikte `panel_settings` `site_memory_default_v1` içine yazılır). Sunucu belleği sonradan
değişirse kendiliğinden hiçbir şey değişmez: yönetici Şu anki sorunlar'da tek satır okur ("varsayılan
önceki belleğe göre konmuştu", iki büyüklükle) ve o satırın tek işlemi, düşürülen her sınırda olduğu
gibi, daha düşük bir varsayılanın durduracağı siteleri önce gösterir ve ancak o liste onaylanınca
uygular.

**Bir bayi varsayılanla sunucuyu tüketemez.** Yönetici bir `site.*` anahtarı için tavan koymadıysa
(ne sunucunun sınırları ne bayinin kendi planı ya da Özel takımı), bayinin planları, müşterilerinin
Özel takımları ve altındaki her site o anahtarın sunucu varsayılanıyla sınırlanır (bellek için
sabitlenmiş, sunucuya bağlı varsayılan). Bayinin formu bu sınırı "sınırı koyan: sunucu yöneticisi"
diye gösterir; yönetici bayiye (bayi planına ya da Özel takımına) bir tavan vererek onu yükseltir.
Yöneticinin kendi planları varsayılanla sınırlanmaz.

**Sınır uygulayamayan sunucu.** Birleşik cgroup düzeni (`/sys/fs/cgroup/cgroup.controllers`) ya da
onun `memory`, `pids` ve `cpu` denetleyicileri yoksa sınırlıyormuş gibi görünen hiçbir şey yazılmaz:
dilim dosyası yazılmaz, hiçbir birim bir dilimi adlandırmaz, taşıma reddedilir, her sitenin kartı
sınırların geçerli olmadığını söyler ve Şu anki sorunlar'da yönetici için eksik olanı adlandıran
tek satır olur.

### 8.1 Gerçek sistemlerde ölçülen gerçekler (21 Kasım 2026)

Temel imajlardan başlatılan iki geçici makinede düz `systemctl` ve cgroup dosyaları: Debian 13
(çekirdek 6.12.105, systemd 257.13) ve Ubuntu 24.04.4 (çekirdek 6.8.0-138, systemd 255.4); ikisi
de `cpu memory pids` ile birleşik düzeni kullanıyor.

| | Soru | Debian 13 / systemd 257 | Ubuntu 24.04 / systemd 255 |
|---|---|---|---|
| F1 | Çalışan bir birimin dosyası `Slice=system.slice`'tan `Slice=celiksite_1.slice`'a değişir; `daemon-reload`, sonra `systemctl restart` | yeniden yüklemeden sonra `Slice=` yeni dilimi gösterir ama süreç `/system.slice/u.service`'te kalır; yeniden başlatmadan sonra `/celiksite_1.slice/u.service`'te çalışır | aynı |
| F1 | Bir ekleme dosyası `Slice=` koyar | ekleme iki yönde de dosyanın önüne geçer (etkin yer `systemctl show -p Slice`) | aynı |
| F2 | Dilimin `TasksMax=30` sınırına ulaşılır (tek birim ya da dilimi paylaşan iki birim) | dilimin `pids.events.local` dosyası `max 1`; birimin `max 0` | `pids.events.local` yok; dilimin `pids.events` dosyası `max 0` kalır; çatallanan birimin `pids.events` dosyası `max 1`, `pids.peak` değeri (31, 11) kendi `pids.max` değerinin altında |
| F2 | Birimin kendi `TasksMax=10` sınırına dilimin içinde ulaşılır | dilimin `pids.events` dosyası `max 1` ama `pids.events.local` `max 0`; birimin yerel dosyası `max 1` | birimin `pids.events` dosyası `max 1`, `pids.peak` 10 = kendi `pids.max` 10; dilimin `pids.peak` değeri 30'da 10 |
| F3 | Dilimin `MemoryMax=128M` sınırı aşılır | dilimin `memory.events.local` dosyası `max 25 oom 1 oom_kill 0`; `memory.events` dosyası `oom_kill 1` | aynı (`max 35 oom 1`) |
| F3 | 512M dilim içinde birimin kendi `MemoryMax=96M` sınırı | dilimin `memory.events` dosyası `max 36 oom 1 oom_kill 1`; `memory.events.local` hep 0 | aynı (`max 42`) |
| F3 | Bütün sunucuda bellek darlığı (2G makinede 8G dilim sınırı, çekirdek kaydında `global_oom`) | dilimin `memory.events` dosyası `oom 0 oom_kill 1`; `memory.events.local` hep 0 | aynı |
| F4 | İçinde birim çalışırken dilim dosyası değişir (512M → 300M); `daemon-reload` | **çekirdek 512M'de kalır** (altı saniye sonra `memory.max` 536870912, `daemon-reexec` ile de); yeni değer, dilimin bir birimi başladığında ya da `systemctl set-property --runtime` ile cgroup'a ulaşır | yeniden yükleme hemen uygular |
| F4 | Sahip dilim dosyasını siler; `daemon-reload` | eski sınırlar dilimin bir birimi yeniden başlayana kadar kalır, sonra hiç sınır yok | sınırlar hemen kalkar (`memory.max` `max`) |
| F4 | 200M kullanım varken 512M → 100M düşürme | bir sonraki başlangıca kadar hiçbir şey | program hemen durdurulur (`oom` 10576) |

Yani sitenin kendi bellek sınırını dilimin `memory.events.local` dosyasındaki `oom` kanıtlar;
kendi işlem sınırını, varsa (Debian'ın 6.12'si, 6.13 ve sonrası) dilimin `pids.events.local`
dosyasındaki `max`, yoksa (6.8) bir birimin `pids.events` dosyasındaki `max` kanıtlar — o birimin
`pids.peak` değeri kendi `pids.max` değerinin altında kaldıysa (sınırı olan tek diğer düzey
dilimdir). `memory.peak` durumları ayırt etmez.

### 8.2 Dosya ve bir değişikliğin çalışan programlara ulaşması

`/etc/systemd/system/celiksite_<kimlik>.slice`, D-031 kuralıyla ve CelikPanel başlığıyla yazılır:
olmayan dosya yazılır, CelikPanel'in değişmemiş metni değiştirilir, sahibin değiştirdiği dosya
korunur ve CelikPanel'in metni yanına `<dosya>.celikpanel-pending` olarak bırakılır (geçerli olan
dosyanın kendi değerleridir). Sahibin eklemeleri `celiksite_<kimlik>.slice.d/*.conf` içine gider;
CelikPanel onları hiçbir zaman yazmaz ya da silmez.

Değişen sınır dilimlere yazılır (`Agent.ApplySiteSlicesV1`, çağrı başına en çok 100 site; büyük bir
değişiklik parti parti gider, böylece bir müşterinin program değişikliği en fazla bir parti bekler;
sürerken yöneticinin site kartı "Yeni sınırlar sitelere yazılıyor: N siteden n tanesi" der).
systemd 257 değişen dilim dosyasını çalışan dilime uygulamadığı için (F4) Ajan sonra çekirdeğin
uyguladığını (`memory.max`, `memory.swap.max`, `pids.max`, `cpu.max`) systemd'nin dilim için
bildirdiği **etkin** değerle (`systemctl show`: dosya ve bütün eklemeler; sahibin eklemesi kendi
sayısını korur) karşılaştırır ve yalnız farklı olan anahtar için `systemctl set-property --runtime
<dilim> <anahtar>=<o değer>` çalıştırır. systemd bunu bir denetim eklemesi olarak yazar
(`/run/systemd/system.control/<dilim>.d/50-<anahtar>.conf`); bu `/etc`'nin önüne geçer. Ajan o
çağrının yaptığı dosyaları tam olarak siler ve yeniden yükler; geçerli ayarlar yine yalnız dilim
dosyası ve sahibin eklemeleri olur. Denetim eklemesi zaten var olan anahtara (sahibin kendi anlık
ayarı) hiç dokunulmaz. systemd 255'te yeniden yükleme değerleri zaten uygulamıştır ve hiçbir şey
yapılmaz. Hiçbir şey yeniden başlatılmaz ve müşterinin diliminde hiçbir süreç başlatılmaz.
Yapılacak dosyalar önce Ajan'ın özel klasörüne kaydedilir ve aynı adımda, adımdan her çıkışta
silinir. Yalnız çağrı ile silme arasında Ajan durursa kalırlar: o anda dilimin etkin değerlerini
taşıdıkları için sahip için o an hiçbir şey değişmez; ama **silinene kadar o anahtarlar için
sahibin kendi eklemelerinin** (`/etc/systemd/system/<dilim>.d/*.conf`) **önüne geçerler**: sahibin
bu arada böyle bir anahtar için eklediği ya da değiştirdiği ekleme ancak Ajan'ın bir sonraki
başlangıcından sonra (Ajan onları örnek kilidini alır almaz, sınırları okuyan hiçbir şeyden önce
siler) ya da bir sonraki açılıştan sonra (`/run` boşalır) geçerli olur. Sahibin o arada aynı anahtar
için kendisinin çalıştırdığı `systemctl set-property --runtime` de onlarla birlikte silinir.
**Düşürülen sınır bu yüzden iki sistemde de hemen geçerli olur**; bu nedenle önce sorulur (§8.5).

**Her siteye ulaşmayan değişiklik.** Sınır değişiklikleri birer birer yazılır (ikinci değişiklik
bekler, sonra sınırları o anki haliyle okur; iki değişiklik hiç iç içe geçmez); 100 sitelik her
partinin kendi süre sınırı vardır. Ajan'ın yanıt vermediği parti (ve ondan sonrakiler), reddettiği
ya da systemd'ye aldıramadığı parti (başarısız `daemon-reload`, okunamayan kayıt defteri) ve dosyası
yazılamayan site önceki sınırlarını korur ve bu kaydedilir (`panel_settings` `site_limit_apply_v1`):
yönetici Şu anki sorunlar'da değişikliği, kaç sitede hâlâ önceki sınırların geçerli olduğunu ve
nedenini söyleyen TEK satır okur; panel her saat o siteleri sınırların o anki haliyle yeniden yazar
ve her site yenilerini alınca satır kaybolur. Sitenin kartı, ikisi farklı olduğunda geçerli sınırı
(systemd'nin uyguladığını) planın değerinin yanında gösterir.

**"Geçerli" çekirdeğin söylediğidir** (üçüncü düzeltme turu). Durum okuması çalışan dilimin
`memory.max`, `pids.max` ve `cpu.max` dosyalarını da okur ve systemd'nin uyguladığıyla karşılaştırır
(bellekte çekirdeğin tam sayfaya yuvarlamasına izin vererek). Kart sınırların geçerli olduğunu ancak
bunlar eşitse ve systemd diskteki dosyayı okumuşsa (`FragmentPath` dolu, `NeedDaemonReload=no`)
söyler. Değilse yönetici "yazıldı, henüz geçerli değil" okur ve geçerli olanlar olarak çekirdeğin
değerleri gösterilir: systemd 257'de değişen dosya çalışan dilime yalnız CelikPanel'in kendi
uygulamasıyla ya da bir programın başlamasıyla ulaşır; sahibin elle değişikliği ise bir
`daemon-reload` ve biraz zaman sonra. `sudo systemctl daemon-reload` sonrasında sahibin değişikliği
çalışan programlara Ubuntu 24.04'te hemen, Debian 13'te hiçbir program başlamadan yaklaşık bir buçuk
dakika sonra ulaşır; bir programın başlaması onu hemen uygular (ölçüldü, A7/N4 gerçekleri).
Okunamayan cgroup
dosyası "bilinmiyor"dur, hiçbir zaman "geçerli" değil; çalışmayan dilim dosyayı bir program
başladığında alır. Uygulamanın kendisi, çalışan bir dilimin sınırlarını neden tutmadığını site site
bildirir (systemd okunamadı, uygulamanın kaydı yazılamadı, `set-property` başarısız oldu ya da
çekirdeğin dosyaları sonrasında hâlâ farklı): böyle bir site değişikliğin başarısız sonucudur, hiçbir
zaman "yazıldı" değil; bu yüzden yukarıdaki kayda, onun Şu anki sorunlar satırına ve dosya
değişmemiş olsa da yeniden uygulayan saatlik tura girer. Bir programın kendi yazmasından sonraki ya
da taşımanın içindeki uygulama yalnız kayda geçer: dilimde başlayan her program systemd'nin dosyayı
uygulamasını sağlar ve kart çekirdeği okur.

**Eksik dosya.** Dosyası olmayan dilimin sınırı yoktur (F4). Kart üç durumu ayırır. Güncellemeden
hemen sonra var olan her sitenin dilim dosyası yoktur, çünkü programları hiç taşınmadı: hiçbir birim
dilimi adlandırmaz; kart programların sınırların dışında çalıştığını söyler ve taşımayı sunar. Bir
birim dilimi adlandırıyor ya da içinde çalışıyorken dosya gitmişse (sahip sildi) kart dosyanın eksik
olduğunu ve sınırların bir sonraki değişikliğinin onu yeniden yazacağını söyler; site "dışarıda"
satırına sayılır ve yöneticinin kartı dosyayı yazan taşımayı sunar. CelikPanel'in yazdığı ve sahibin
geri vermek için sildiği dosya, hiçbir birim dilimi adlandırmasa da bir sonraki değişiklikle yeniden
yazılır.

**Yöneticinin kartı** sahibin eklemelerini dosya adıyla ve "sunucu sahibinin eklemeleri geçerli"
diyerek gösterir; değiştirilmiş dosyayı korunuyor diye anar.

### 8.3 Bir birim yazıldığında nereye gider

Değişen `Slice=` çalışan bir birime ancak bir sonraki başlangıcında ulaşır (F1). Bu yüzden:

- çalışmayan bir birim (yeni, durdurulmuş, her kurulum ya da derleme adımı) sitesinin dilimine
  yazılır. alpha.149'dan beri (alpha.148 laboratuvarının O2'si) buna çalışmayan bir programın
  **panelden başlatılması** da girer (yüklenen program, Node.js sitesi, yapay zekâ ya da otomasyon
  uygulaması ve çalışmayan birinin Yeniden başlat'ı): birimi önce dilime yazılır (sitenin dilim
  dosyası yerine konur, CelikPanel'in değişmemiş kendi metni `Slice=celiksite_<id>.slice` alır,
  systemd yeniden yüklenir), sonra orada başlar. Sahibin değiştirdiği ya da kendi eklemesiyle
  yerleştirdiği, önceki bir korumadaki, tutulan bir taşıma kaydının adını verdiği birim yazılmaz;
  systemd çalışıp çalışmadığını söyleyemediğinde hiçbir şey yazılmaz. Bu yazmadan önce Başlat, taşıma
  gibi, bu sürümün koruma yardımcısını ve SMTP engelini yerine koyar (düzeltme turu F2: alpha.147'den
  önceki yardımcı SMTP engeline yalnız sistem dilimindeki denetim gruplarını ekliyordu, bu yüzden
  sitesinin dilimindeki bir birim `ExecStartPre` adımında reddediliyordu); bu yapılamazsa hiçbir şey
  yazılmaz ve program önceden başladığı yerde, sınırların dışında başlar;
- çalışan bir birim her sıradan yazmada (ayar değişikliği, güncelleme, müşterinin yeniden
  başlatması) çalıştığı dilimde kalır;
- dosyası zaten sitesinin dilimini adlandıran bir birim, o dilimin dosyası durdukça, systemd
  nerede çalıştığını söyleyemese de (yanıt vermez ya da boş veya bilinmeyen bir yer gösterir;
  alpha.148 düzeltme turu) her yazmada o satırı korur: yapılamayan bir okuma bir programı hiçbir
  zaman sınırlarının dışına çıkarmaz, içine de almaz;
- yöneticinin Node.js sitesi için koruma taşıması (§6a) zaten yeniden başlatır ve onu kanıtı ve geri
  koymasıyla dilime alır.

**Sunucu yeniden başladıktan sonra.** systemd açılışta her birimi kendi dosyasından, `Slice=`
satırının adlandırdığı dilimde başlatır. Bu sürümden önce yazılmış her programın ve çalışırken
yazılmış her programın dosyası `Slice=system.slice` der. **Bu yüzden sitenin var olan programı
yeniden başlatmadan sonra, önceden olduğu gibi, sitenin sınırlarının dışında başlar ve yöneticinin
taşımasına kadar dışarıda kalır.** Dosyası çalışmıyorken yazılan ya da taşınan program sitesinin
dilimini adlandırır ve onun içinde başlar.

### 8.4 Yöneticinin taşıması

Sitenin Genel Bakış sayfası yöneticiye her programın nerede çalıştığını ve tek bir işlemi gösterir:
`POST /api/v1/domains/{id}/site-limits/move {request_id}` (`Agent.MoveSiteSliceV1`). Panel genelinde
aynı anda bir taşıma; yinelenen `request_id` ilk yanıtı alır; denetim kaydına `site.limits_move`
olarak yazılır. Panelin kendi güncellemesi kabul edilmiş ya da sürüyorken taşıma (ve Tekrar dene)
panel ve Ajan tarafından reddedilir ("panelin güncellemesi sürüyor; siteyi güncelleme bittikten sonra
taşıyın"; düzeltme turu F6) ve bir taşıma sürerken güncelleme reddedilir (§8.6a). Bunu yalnız gerçekten
süren bir taşıma yapar: Ajan taşımanın bütün süresince kendi işaretini tutar; bir durum okumasının bir
anlığına aldığı kilit hiçbir zaman güncellemeyi ya da bir taşımayı reddettirmez (düzeltme turu F5).
Ajan bunun için güncellemenin kendi kayıtlarını yalnız okur: sırada ya da süren bir kayıt ancak hâlâ
canlı bir güncelleme olabildiği sürece, yani 31 dakikadan kısa süre önce oluşturulduysa sayılır
(güncellemenin işçisi ve kurtarması ayrı ayrı 15 dakikayla sınırlıdır; güncellemenin kendi uzlaştırması
daha eski kaydı başarısız sayar); böylece ölmüş bir işçinin "sürüyor" bıraktığı kayıt taşımaları hiçbir
zaman kalıcı olarak reddettirmez (ikinci düzeltme turu, R2). Okunamayan bir kayıt taşımayı güvenli
tarafta kalarak kendi sözleriyle ("panelin güncellemesinin sürüp sürmediği bilinmiyor") reddeder ve son
değişmesinden 31 dakika sonra kendiliğinden reddetmeyi bırakır. Güncellemenin kapısı yalnız bu denetim
ve taşımanın işaretinin konması için tutulur, taşıma kaydının kilidini beklerken hiçbir zaman (R7):
güncellemenin kabulü hiçbir zaman bir durum okumasının arkasında beklemez.

- Taşıma sitenin kilidini beklemeden alır: tam o sırada silinen ya da değiştirilen site taşınmaz
  ("site şu an değiştiriliyor; bir dakika sonra sayfasını yeniden okuyun") ve taşıma silinmiş bir
  sitenin hiçbir şeyini yeniden yaratmaz.
- Önce her birimi okur: dosyasını sahibin değiştirdiği birim, eski bir korumadaki birim ve sahibin
  eklemesinin yerleştirdiği birim (eklemedeki `Slice=`, bir şey yeniden başlamadan önce okunur)
  taşınmaz ve yeniden başlatılmaz; kart onları kimin ne yapacağıyla adlandırır ve "dışarıda" satırına
  sayılmazlar.
- İlk yazmadan önce günlüğü kaydeder (aşağıda). Sonra dilim, sonra sırayla her birim, **o programın
  kendi değişiklik kilidiyle** (programın her başka değişikliğinin aldığı kilit; her programın kendi
  kilidi, başka hiçbir program ya da siteyle paylaşılmaz): dosya taşımanın planladığı metin olmalıdır
  (değilse arada yapılmış bir değişikliktir: korunur, "değiştirildi"), CelikPanel'in değişmemiş metni
  `Slice=celiksite_<kimlik>.slice` alır (yeniden mühürlenir), systemd yeniden yüklenir ve çalışan birim
  bir kez yeniden başlar; **kendi başlama süresi içinde** dilimde çalışmalı ve önceden dinlediği her
  bağlantı noktasını yeniden dinlemelidir: birimin kendi `TimeoutStartUSec` değeri (bu birimler koymaz,
  yani systemd'nin 90 saniyesi), hiçbir zaman 20 saniyeden az, 300 saniyeden çok değil. Yeniden
  başlatmanın kendisi de, birimin kendi `TimeoutStartSec` değeri ne derse desin (`infinity` bile),
  aynı süreyle sınırlıdır: o sırada hâlâ süren başlangıç "hâlâ başlıyor" olur (systemd başlatma
  işini sürdürür; yalnız taşıma beklemeyi bırakır) ve birim geri konur. Ne neyi
  bekler: taşınan programın bir değişikliği onun adımını bekler (en fazla başlama süresinin iki katı:
  yeniden başlatma ve bir geri koyma); taşıma siteyi tutarken aynı sitenin bir değişikliği reddedilir
  ("değiştiriliyor"), site değiştirilirken taşımanın reddedildiği gibi; başka programlar ve başka
  siteler hiç beklemez.
- **Taşımanın süre kuralı.** Taşımanın 11 dakikası vardır. Çalışan bir program ancak kalan süre onun
  kendi başlama süresini, kendi geri koymasını (o süreyi yeniden alabilen bir yeniden başlatma),
  ondan önce taşınan her programın geri koymasını ve 5 saniyeyi tutuyorsa yeniden başlatılır; böylece
  ilk program, en uzun başlama süresi olan 300 saniyeyle bile her zaman sığar ve geri koymalar dahil
  bütün taşıma panelin çağrı için koyduğu 12 dakikanın altında kalır. Süre yüzünden duran taşıma bunu
  söyler: taşıdığı programlar sınırların içinde kalır ve tamamlandı olarak işaretlenir, başlatmadıkları
  "denenmedi" olur ve bir sonraki taşıma onlarla sürer. **Bir basış, bir son süre** (ikinci düzeltme
  turu, R4): taşıma önce kendi sitesinin korunan kaydını sonuçlandırıyorsa, bu ve taşıma basıştan
  itibaren sayılan aynı 11 dakikayı paylaşır; artık sığmayan başlatılmaz ve bu söylenir.
- Bir birim başarısız olursa o ve sitenin ondan önce taşınan her birimi taşıma öncesi baytlarına
  geri konur, ama yalnız taşımanın kendi metninin üstüne: arada değiştirilen ya da silinen dosya
  olduğu gibi bırakılır. Diğer sitelere hiç dokunulmaz. Yanıt "geri kondu" demeyi yalnız geri
  konduğu doğrulanan programlar için söyler; geri koyması tamamlanmayanları adıyla sayar.

**Günlük** (Ajan'ın özel klasöründe `site-slice-move.json`) **her yazmadan önce** yazılır ve diske
işlenir: ilk yazmadan önce taşımanın değiştireceği her birim dosyasının taşıma öncesi baytları ve
kipi, dilim dosyasının baytları ve kipi (ya da olmadığı) ile kayıt defterindeki özeti ve her birimin
adımı; bir birim dosyası yazılmadan önce adımı "yazılıyor" olur; dilim dosyası yazılır yazılmaz yeni
baytları. Yazılamayan günlük başarısız bir adımdır: başka hiçbir şey yazılmaz ve yazılan geri konur.
Ajan başlarken yalnız adımın adına hiç güvenilmez: her birimin dosyası kaydedilen taşıma öncesi
baytlarla ve taşımanın yazmak istediği metinle karşılaştırılır. **Ajan'ın açılışı bir sitenin
programını hiçbir zaman başlatmaz, yeniden başlatmaz ve beklemez** (düzeltme turu F1): bir güncelleme
yeni Ajan'ı yalnız sınırlı bir süre bekler (alpha.149'dan beri yaklaşık bir dakika, önceden yaklaşık on
iki saniye), bu yüzden Ajan'ın açılış yolunda hiçbir şey bir müşterinin programını bekleyemez. Açılışta
kayıt yalnız okunur ve dosyalarla uzlaştırılır:

| Birim dosyasında olan | Ajan'ın bir sonraki başlangıcında |
|---|---|
| taşıma öncesi baytlar, adım "planlandı" ya da "yazılıyor" | dokunulmaz ("denenmedi") |
| taşıma öncesi baytlar, daha sonraki bir adım | zaten geri kondu: çalışsın ya da çalışmasın tam olduğu gibi bırakılır (sahibinin durdurduğu program durmuş kalır); sonucu bu taşımanın kendi kaydının söylediğidir, geri koyma yarıda kaldıysa tek bir okuma: sistem diliminde başlama süresi boyunca kesintisiz çalışıyorsa (ve portu yoksa: açılış port okumaz) "geri kondu", orada başka türlü çalışıyorsa "henüz bilinmiyor", çalışmıyorsa "henüz yeniden başlatılmadı" |
| taşımanın metni, durdurulmuş birim | korunur: dosyası dilimi adlandırır, orada başlar ("yeniden yazıldı") |
| taşımanın metni, çalışan birim, adım "taşındı" (dilimde çalıştı ve yanıt verdi) | korunur ("taşındı") |
| taşımanın metni, çalışan birim, başka her adım | taşıma öncesi baytlar geri, hepsi için TEK daemon-reload ve hâlâ sistem diliminde çalışmıyorsa hepsi için işleri beklenmeden istenen TEK bir yeniden başlatma (`systemctl restart --no-block b1 … bN`): "henüz bilinmiyor" |
| başka bir şey ya da dosya yok | olduğu gibi bırakılır ("değiştirildi") |

Böylece açılışın systemd çağrıları program sayısı ne olursa olsun en fazla dörttür: yeri ne olacağına
karar veren programların tek bir okuması, bir daemon-reload, bir reset-failed ve taşıma başladığında
çalışan her programı adlandıran tek bir yeniden başlatma isteği. Yoklama, port okuması ve bekleme
yoktur. Bu çağrılar **15 saniyelik tek bir toplam sınırı** paylaşır (ikinci düzeltme turu, R5); süre
dolunca başka hiçbir şey istenmez, yanıt vermeyen okuma ya da yeniden yükleme "bilinmiyor", gönderilmeyen
istek "henüz yeniden başlatılmadı" bırakır ve Ajan açılmayı sürdürür. Tek isteğin çıkış kodu ne olursa
olsun adlandırdığı her program "henüz bilinmiyor"dur ve kayıt kalır. **"Henüz bilinmiyor" açılışın
dışında, yalnız okunarak çözülür** (düzeltme turu F3, ikinci düzeltme turu R1/R3): sitenin durum okuması
(kart ve Şu anki sorunlar'ın arkasındaki beş dakikalık arka plan okuması) "henüz bilinmiyor", "çalışmıyor",
"henüz yeniden başlatılmadı" ya da "hâlâ başlıyor" bırakılan her programı denetler. Satırı yalnız TEK bir
okuma programı sistem diliminde etkin ve çalışır, en az başlama süresi boyunca (hiçbir zaman 20 saniyeden
az değil; systemd'nin `ActiveEnterTimestampMonotonic` değeri) kesintisiz açık gösterdiğinde, dosyası
önceki baytlarını tuttuğunda ve taşımadan önce portlarda dinlediyse şimdi bir portta dinlediğinde
kapatılır. systemd'nin yeniden başlatma döngüsündeki bir program tek bir şanslı okumayla hiçbir zaman
kapatılmaz. "Henüz bilinmiyor" bırakılan ve durmuş, başarısız ya da yeniden başlatma döngüsünde olan
program "çalışmıyor" olur ve kartta ve Şu anki sorunlar'da yöneticinin yapacağı işle adlandırılır; başka
her durum olduğu gibi kalır. Böylece yöneticinin sayfasından başlattığı ya da systemd'nin geçici bir
andan sonra yeniden kaldırdığı program, başlama süresi boyunca çalıştıktan sonraki ilk okumada kapatılır
ve Şu anki sorunlar'daki satır kendiliğinden gider. Hiçbir şey kalmadığında kayıt kapatılır ve dilim
dosyası aşağıdaki gibi geri gelir; bu yalnız dilim kilidi hemen alınabiliyorsa yapılır (R6; değilse bir
sonraki okuma kapatır). Bir okuma hiçbir şeyi başlatmaz ya da yeniden başlatmaz. Tekrar
dene ve aynı sitenin bir taşıma girişimi aynı tabloyu kullanır, ama önceden çalışan ve çalıştığı yerde
çalışmayan programı yeniden başlatır ve başlama süresi içinde denetler; bunlar taşımanın kendi 11
dakikasıyla sınırlıdır (düzeltme turu F4: yeniden başlatma ancak kalan süre başlama süresini ve 5 saniyeyi
tutuyorsa gönderilir; kalanlar "henüz yeniden başlatılmadı" olur). **Başka** bir sitenin taşıması kaydı
bulur ve ilk sitenin hiçbir şeyi yeniden başlatılmadan hemen reddedilir.

**"Yanıt vermiyor"un bir çıkışı var ve o yöneticinin basışıdır** (ikinci düzeltme turu, R8). Taşımanın
kendi geri koyması bugünkü gibi "yanıt vermiyor, bakın" demeyi sürdürür. Tekrar dene böyle bir programı
bir kez daha yeniden başlatır (yönetici istedi), başlama süresi kadar bekler ve program o süre boyunca
sistem diliminde kesintisiz çalışırsa, hiçbir portta dinlemese de satırı geri kondu olarak kapatılır;
yanıt onu adıyla söyler ("çalışıyor, hiçbir portta dinlemiyor"), böylece yöneticiye iki kez söylenmiş
olur. O süre içinde durup yeniden kalkan program "yanıt vermiyor" kalır. Okunamayan dinleyici tablosu
(`ss` başarısız) "henüz bilinmiyor"dur, hiçbir zaman "hiçbir portta dinlemiyor" değil.

Birim dosyaları, yönetilen her başka dosyanın kullandığı yazıcıyla yazılır ve geri konur (metin
yeniden adlandırmadan önce, klasör ondan sonra diske işlenir); böylece bir yazmanın hemen ardından
elektrik kesilse de boş bir birim kalmaz.

Dilim dosyası: her birim sonuçlanmışsa, bundan sonra hiçbir birim dilimi adlandırmıyorsa ve dosya tam
olarak taşımanın yazdığı baytları tutuyorsa kaydedilen asıl geri gelir (önceki baytları ve kayıt
defteri özeti, ya da önceden yoksa dosya yok); yoksa kalır. Bu, sınır değişikliklerinin aldığı dilim
kilidi altında, taşıma kaydının kilidinden sonra yapılır (ikinci düzeltme turu, R6).

**Tamamlanmayan geri koyma günlüğü korur** (üçüncü düzeltme turu). Bunlar: yazılamayan taşıma öncesi
dosya; systemd'nin yerini söyleyemediği birim (başarısız durum okuması "bilinmiyor"dur: buna göre
hiçbir şey yeniden başlatılmaz, hiçbir zaman "çalışmıyor, yeniden başlat" sayılmaz); başarısız
`daemon-reload` (systemd hâlâ taşınmış metni tutar, bu yüzden hiçbir birim yeniden başlatılmaz);
başarısız yeniden başlatma (program durmuştur) ya da başlama süresinin sonunda hâlâ süren başlangıç;
yeniden başlatılan ama yeri söylenemeyen program ("henüz bilinmiyor", hiçbir zaman "yeniden
başlatılmadı" değil); Ajan'ın açılışında, istediği ("henüz bilinmiyor", yukarıda) ya da sınırı
dolmadan gönderemediği ("henüz yeniden başlatılmadı") her yeniden başlatma.
Günlük korunduğu sürece hiçbir site taşınmaz: yeni taşıma, taşıması bitmeyen site adıyla reddedilir.
Sitenin kartı (yönetici) son taşımanın bitmediğini ve kalan her program için durumunu (geri kondu ama
çalışmıyor; yeri bilinmiyor, yeniden başlatılmadı; hâlâ başlıyor; henüz yeniden başlatılmadı; yeniden
başlatıldı, henüz bilinmiyor; çalışıyor ama önceden dinlediği yerden artık yanıt vermiyor), nedeni için
`journalctl -u <birim>` komutunu ve sonraki adımı söyler: çalışmayan ya da yeniden başlatılmayan program
için kendi sayfasından ya da `systemctl start <birim>` ile başlatmak; henüz bilinmeyen ya da yanıt
vermeyen için önce bakmak (`systemctl status <birim>` ve günlüğü), hiçbir zaman "başlatın" değil
(düzeltme turu W3); **Tekrar dene** düğmesi (`POST …/site-limits/move {request_id, recover: true}`,
denetim kaydına `recover` ile `site.limits_move` olarak yazılır) geri koymayı hemen çalıştırır ve
yeni hiçbir şey taşımaz. Bu arada elle başlatılan program sistem diliminde çalışır bulunur ve günlük
gider; bir program durmuşken günlüğü unutturan bir düğme yoktur. Günlük korunduğu sürece Şu anki
sorunlar'da yönetici için TEK satır vardır (`site_move_kept`, siteyi açar). Durum okuması okuyarak
görebildiğini sonuçlandırır; Tekrar dene ve o sitenin bir sonraki taşıma girişimi aynı geri koymayı
dener; Ajan'ın açılışı yalnız kaydı uzlaştırır (yukarıda). Tamamlanınca kart, sitenin bir
sonraki taşımasına kadar son taşımanın yarıda kesildiğini ve her programın tamamlandığını, geri
konduğunu ya da arada değiştirildiyse olduğu gibi bırakıldığını söyler.

### 8.5 Sınırı sitenin kullandığının altına düşürmek

`site.memory_mb` değerini sitenin programlarının şu an kullandığının altına düşürmek çekirdeğin
onlardan birini hemen durdurmasına yol açar (F4); `site.tasks` değerini tuttukları işlem sayısının
altına düşürmek yeni işlemleri reddeder, çalışanlardan hiçbirini durdurmaz. Bu yüzden plan formu,
bayi planı ve bir hesabın kendi sınırları, değişiklik onaylanmadan önceki önizlemesinde (panelin
kendi onayı) programları yeni değerden fazlasını kullanan siteleri gösterir ve tam olarak şunu söyler:
bellek sınırını aşan program hemen durdurulur; işlem sınırında yeni işlem başlamaz, çalışanlar sürer.
**Önizlemeyi ekran değil sunucu uygular:** sınırı sitelerin şu an kullandığının altına düşüren istek
ancak sunucunun o anda verdiği listenin işaretiyle (`confirm_stopped`, önizlemenin `stopped_token`
değeri) kaydedilir; API'den bu işaret olmadan ya da önceki bir listenin işaretiyle gelen istek
(sitelerin kullanımı bu arada değişti) güncel listeyle reddedilir. Önceki belleğe göre sabitlenmiş
varsayılanın işlemi için de aynısı geçerlidir (§8 "Sınırlar"). Sınırların uygulanmasında başka hiçbir
şey değişmez. Sunucunun kendi sınırlar ekranı onları listelemez (bilinen bir açık: bir `site.*`
anahtarının düşürülen sunucu tavanı ya da varsayılanı önizlemesiz uygulanır).

### 8.6 Bir sınıra ulaşıldığında insanların okuduğu (D-024)

Ajan dilimin sayaçlarını sitenin durum okumasıyla ve program çalıştıran bütün siteler için arka
planda en fazla beş dakikada bir okur (en çok 1000 site ve 4000 programlık sayfalarla; bir sitenin
istediği kadar programı olabilir; hiçbir okuma site ya da program sayısı yüzünden reddedilmez; kayıt
defteri her okumada bir kez okunur). Panel bir sayacın ne zaman arttığını kaydeder (`panel_settings`
`site_limit_events_v1`; bir sitenin ilk okuması başlangıç noktasıdır; yalnız bir sınıra
ulaşıldığında, bir sayaç yeniden başladığında ya da bir site gittiğinde yazılır). Kayıt sınırlıdır:
her tür için beş dakikada en fazla bir ulaşma anı ve hiçbir zaman bir günlükten (288) fazlası tutulur;
kart ne kadar sık okunursa okunsun kayıt bununla büyümez; sayılar eksiksiz kalır.

- "Sitenin kendi sınırı" (bellek, işlem) yalnız §8.1'in kanıtladığıdır. Kart o zaman ne olduğunu ve
  ne zaman olduğunu, o zaman geçerli olan sınırı (alpha.149, alpha.148 laboratuvarının CHECK 2'si:
  olayla birlikte o okumadaki çekirdek değerinden kaydedilir; daha önce kaydedilmiş bir olay sayısız
  gösterilir, hiçbir zaman bugünkü değerle değil) ve geçerli sınır planınki ise kimin
  yükseltebileceğini söyler: müşteriye "hizmet sağlayıcınız", bayiye "kendi
  sınırlarınız içinde müşterinin planında siz", yöneticiye planın ya da bayinin adı. Bir günde iki ya
  da daha fazla okumada kendi bellek ya da işlem sınırına ulaşan site Şu anki sorunlar'da bir satırdır.
- **Ne sayılır.** Sitenin kendi bellek sınırına bir kez ulaşılması, o sınıra ulaşılmışken çekirdeğin
  durdurduğu bir programdır: dilimin kendi `memory.events.local` `oom` değerinin arttığı bir okumada
  dilimin `oom_kill` artışı (en az bir); her başarısız geri kazanmayı sayan ham `oom` sayısı hiçbir
  zaman sayılmaz (F4: durdurulan tek program için `oom 10576`). Sitenin sınırında reddedilen işlemde
  her reddedilen başlatma sayılır.
- Sitenin sınırı olmadan bellek yetmediği için durdurulan (kendi daha küçük sınırı ya da bütün
  sunucunun darlığı) ya da çekirdeğin sitenin sınırına bağlamadığı bir işlem reddi yaşayan (programın
  kendi sınırı ya da 6.8'de birimin bunu söyleyecek `pids.peak` değeri olmadığında) program yalnız bu
  kadarıyla söylenir ("bu sitenin bir programı yeni işlem başlatamadı; çekirdek bunun sitenin sınırı
  olduğunu göstermiyor"): plan hiçbir zaman suçlanmaz ve Şu anki sorunlar satırı çıkmaz.
- Sınırların geçerli olmadığı yerde (bütün programlar dışarıda, dilim dosyası yok, sınırları
  uygulayamayan sunucu, yazıldı ama henüz geçerli değil, bilinmiyor) kart "sınırlarının içinde
  kaldı" yerine bunu söyler. **Müşteri ya da bayi bunların hiçbiri için bir şey yapamaz**; bu
  yüzden tek ve yansız bir cümle okurlar ("bu sınırlar henüz uygulanmıyor; bununla sunucunun
  yöneticisi ilgilenir; sizden bir şey gerekmez"); yönetici nedeni ve adımı okur.

**Henüz taşınacak siteler.** Şu anki sorunlar'da yalnız yönetici için bir satır vardır: kendi
programları sınırlarının dışında çalışan ya da orada başlayacak olan ve taşımanın alabileceği siteler
(sayıları ve ilk beş adı). İlk siteyi açar, bir uyarıdır (yapılacak iş) ve hiç site kalmayınca
kendiliğinden kaybolur (`panel_settings` `site_limit_outside_v1`, yalnız liste değişince yazılır).

### 8.6a alpha.149'un değiştirdikleri (alpha.148 laboratuvarı)

- **Geri koyma, taşıma gibi denetlenir** (D6). Geri konan program yeniden başlatıldıktan sonra kendi
  başlama süresi içinde çalıştığı yerde (etkin, sistem diliminde) çalışmalıdır; taşımanın kendi adımı
  dilimi böyle denetler. Portları olan program yeniden bir portta dinlemelidir (düzeltme turu F10:
  çalıştığı yere geri konup değişmiş bir porttan yanıt veren program çalışıyordur ve kayıt bırakmaz;
  "yanıt vermiyor" yalnız portları olup başlama süresinden sonra hiçbirinde dinlemeyen programdır;
  portu olmayan program yerinde etkin olur olmaz çalışıyordur; Tekrar dene aynı denetimi kullanır).
  Yeniden başlatması başarılı olduğu hâlde orada çalışmayan program (`Type=simple` birimin yeniden
  başlatması süreci başlar başlamaz başarılı sayılır) "geri kondu" **sayılmaz**: kayıt onunla birlikte
  durmuş, hâlâ başlıyor ya da yanıt vermiyor olarak tutulur. Ajan'ın açılışı bu denetimi yapmaz
  (düzeltme turu F1): yalnız yeniden başlatmayı ister ve programı "henüz bilinmiyor" (ne hata ne başarı)
  olarak tutar; bunu durum okuması okuyarak çözer (§8.4: yalnız başlama süresi boyunca kesintisiz
  çalışan program için, ikinci düzeltme turu R1). Okunamayan dinleyici tablosu "henüz bilinmiyor"dur,
  hiçbir zaman "hiçbir portta dinlemiyor" değil (R8). Kayıt her programın portlarını da yazar
  (önceki bir kayıtta yoktur, onlarsız denetlenir). Kart ve Şu anki sorunlar programın şu an
  çalışmadığını, yöneticinin işlem yapacağını, sonraki adımı (kendi günlüğü, `sudo journalctl -u
  <birim>`, sonra **Tekrar dene** ya da başlatmak) ve başka hiçbir şeyin değiştirilmediğini söyler.
  Taşımanın penceresi "çalıştıkları yere geri kondu, yeniden çalışıyor" sözünü yalnız taşımadan önce
  çalışan programlar için, ötekiler için "oldukları gibi geri kondu" der (düzeltme turu W1).
- **systemd'nin kendiliğinden yeniden başlatma döngüsündeki program** (`activating`/`auto-restart`,
  denetim grubu yok) çalışmıyor olarak gösterilir, hiçbir zaman sunucu sahibinin koyduğu yerde
  olarak değil; taşıma için sayılmaz.
- **Nedenler sayfanın dilinde** (D7, O3). Ajan taşımanın hatasını, geri koymasını ve hiç
  yapmadığı bir taşımayı değerleriyle bir kod olarak bildirir (`not_running`, `still_starting`,
  `not_in_slice`, portlarıyla `not_listening`, `restart_failed`, `another_move`, `site_busy`…);
  sayfa bunları Türkçe ya da İngilizce söyler; tanımadığı bir kod, İngilizce ayrıntının yazıldığı
  Ajan günlüğünü gösteren genel bir cümledir. Plan önizlemesinin site satırları ("kullanılan bellek
  215 MB, yeni sınır 128 MB") değer olarak gönderilir ve sayfa tarafından söylenir; bir site ve bir
  hesap tekil söylenir.
- **Uygulanmayan sınırlar satırı doğrudur** (O3). Ajan bir itmenin her çıkış yolunda yanıtlamadan
  önce çalışan dilimin çekirdek değerlerini yeniden okur: çekirdeği etkin değerleri tutan site, hangi
  adım başarısız olursa olsun sınırlarına sahiptir; değerleri karşılaştırılamayan site "bilinmiyor"
  olur (Şu anki sorunlar'da kendi sözüyle), hiçbir zaman "sınırlara sahip değil" değil. Kaydedilen
  neden bir koddur (`kernel_differs`, `unchecked`, `write_failed`, …) ve abonelik başına her biri için
  site sayısıyla tutulur (düzeltme turu F9): tek satır her siteyi kendi nedeniyle söyler. Yalnız sınır
  dosyası yazılamayan ya da çekirdeği hâlâ önceki değerleri tutan siteler için önceki sınırların
  sürdüğü söylenir; ötekiler için (okunamadı, Ajan yanıt vermedi ya da değişikliği almadı) bilinmediği
  söylenir.
- **Ubuntu 24.04'te ulaşılan işlem sınırı** (D3). `pids.events.local` olmayan çekirdekte dilimin
  kendi `pids.peak` değeri okunduğu andaki `pids.max` değeriyle birlikte okunur. `pids.peak` en yüksek
  değeri tutar ve değişen sınır onu sıfırlamaz; bu yüzden kart "işlem sınırına ulaştı" sözünü yalnız
  sınır önceki okumadan beri değişmediyse, o okumada tepe değer vardıysa ve sınırın altındaysa ve tepe
  şimdi sınıra ulaştıysa söyler (düzeltme turu F7): önceki bir tepenin altına düşürülen sınır ya da
  güncellemeden sonraki ilk okuma ulaşılan sınır değildir. Oradaki çekirdek kaç kez olduğunu ve yeni
  bir işlemin reddedilip reddedilmediğini saymaz; kart da bunu söyler (yeni işlemlerin başlayamadığını
  hiçbir zaman söylemez).
- **O zamanki sınır** (düzeltme turu F8). Bir sınır değişikliği itilmeden önce panel ilgili sitelerin
  sayaçlarını bir kez okur; böylece değişiklikten önce olanlar o zaman geçerli olan sınırla kaydedilir.
  Bu okuma yapılamazsa bir sonraki okumanın bulduğu olaylar sınır değeri taşımaz (değişiklikten önce
  olmuş olabilirler), hiçbir zaman yenisini değil.
- **Bayinin sınırı** (CHECK 2). Ne sunucu ne bayi bir tavan koymadığında sunucunun varsayılanı bayinin
  müşterisini sınırlar; kart onu o varsayılan olarak adlandırır ("bayiyi de sınırlar"), yöneticinin
  koyduğu bir sınır olarak değil.
- **Taşıma sürerken güncelleme yok** (D5). Bir sitenin programları taşınırken panelin kendi
  güncellemesi başlatılmaz: panel ve Ajan ikisi de reddeder, güncelleme sayfası taşımanın penceresini
  bekleyip güncellemeye yeniden basmayı söyler. Bunu yalnız gerçekten süren bir taşıma yapar (düzeltme
  turu F5: Ajan'ın kendi "taşıma sürüyor" işareti, hiçbir zaman bir durum okumasının aldığı kilit değil).
  Tersine, güncelleme kabul edilmiş ya da sürüyorken taşıma ve Tekrar dene reddedilir (düzeltme turu
  F6); eskimiş bir güncelleme kaydı reddetmeyi kendiliğinden bırakır (ikinci düzeltme turu, R2, §8.4).
  Ajan'ın açılışı her açılış adımı için süresiyle bir günlük satırı yazar (`agent start: <adım>
  took …`).

### 8.7 Silinen sitenin dilimi

Bir site silinince CelikPanel'in değişmemiş dilim dosyası gider ve kayıt defteri onu unutur; sahibin
değiştirdiği dosya ve sahibin ekleme klasörü systemd'nin yüklemediği adlarla kenara konur
(`<dosya>.celikpanel-aside-<UTC zamanı>`) ve denetim kaydında adlandırılır (`site.limits_file_aside`).
**Bir site yaratılırken** (henüz hiçbir şeyi yok: ev klasörü ve hesabı yeni), onun kimliğine ait olup
Ajan'ın kayıt defterinin tanımadığı dilim dosyası (önceki bir kurulumda aynı kimliğe sahip bir site)
**ekleme klasörüyle birlikte** aynı biçimde kenara konur; yeni müşterinin sitesi ikisini de devralmaz;
Ajan'ın kaydında adlandırılır. Böyle bir dosya ya da klasör varsa ve sınıflandırılamıyorsa (kayıt
defteri okunamıyor) ya da kenara konamıyorsa **site reddedilir**: hiçbir şeyi yapılmadan, dosya ve
nedeniyle ("bu ad için sunucuda zaten bir yapılandırma dosyası var"; sunucu sahibi onu kaldırır,
sonra site yeniden yaratılır); böyle bir dosya yoksa yaratma kayıt defterinden bağımsız sürer. Var
olan bir site için sonraki bir yazma sahibin dosyasını ya da eklemesini hiçbir zaman kenara koymaz. Kayıt defterinin tanıdığı dilim dosyası (aynı kurulumun,
silinmesi çalışmamış önceki bir sitesi) olduğu gibi bırakılır. **Okunamayan kayıt defteri** bir
hatadır, hiçbir zaman boş bir kayıt defteri değildir: o okumayla hiçbir şey yazılmaz, silinmez ya da
kenara konmaz ve çağıran bunu söyler (sınır değişikliğinin Şu anki sorunlar satırı, taşımanın yanıtı,
silmenin başarısız adımı, bir programın kendi yazması için Ajan'ın kaydı).

**Sahibin bağımsızlığı (D-022).** Dilimler yerel birimlerdir. Panel durdurulsa, kaldırılsa ya da
lisanssız kalsa da onlar ve sınırları çalışmaya devam eder. Bir sitenin sınırlarını panel olmadan
kaldırmak ya da değiştirmek için sahip bir ekleme dosyası yazar
(`/etc/systemd/system/celiksite_<kimlik>.slice.d/owner.conf`, örneğin `[Slice]` ve
`MemoryMax=infinity` ile) ve `systemctl daemon-reload` çalıştırır. `sudo systemctl daemon-reload`
sonrasında sahibin değişikliği çalışan programlara Ubuntu 24.04'te hemen, Debian 13'te hiçbir
program başlamadan birkaç saniye ile yaklaşık bir buçuk dakika arasında ulaşır (alpha.148
laboratuvarı: 15 saniye içinde); bir programın başlaması onu hemen uygular.
CelikPanel onu hiçbir zaman yazmaz ya da silmez.

**Panel kalırken tek bir programı sitesinin sınırlarının dışına almak.** Panelde bunun için bir
işlem yoktur. Sahip programın biriminin yanına bir ekleme dosyası yazar; Debian 13'te ve Ubuntu
24.04'te aynıdır: `/etc/systemd/system/<birim>.service.d/slice.conf` içinde `[Service]` ve
`Slice=system.slice`, sonra `sudo systemctl daemon-reload` ve `sudo systemctl restart <birim>`.
CelikPanel bir birimin ekleme dosyalarını hiçbir zaman yazmaz ya da silmez ve sonrasında panelin
hiçbir yeri programı reddetmez: durum okuması ekleme dosyasının `Slice=` satırını `DropInPaths`
üzerinden okur (`siteUnitDropInSlice`); yöneticinin kartı programı sınırların dışında, "sunucu
sahibi onu kendi eklemesiyle yerleştirdi; orada kalır" diye gösterir ve "dışarıda" satırı onu
saymaz; taşıma onu yeniden başlatmadan bırakır (`owner_slice`); bir kaydetme CelikPanel'in kendi
birim dosyasını her zamanki gibi yazar (§8.3'ün kuralıyla korunan `Slice=` satırını ekleme dosyası
geçersiz kılar), bu yüzden ne `unit_edited` ne de korunan bir dosya oluşur; posta engelinin
yardımcısı ve kanıtı sistem dilimini kabul eder. Ekleme dosyasını silip aynı iki komutu çalıştırmak
programı yeniden içeri alır. Bunun yerine CelikPanel'in birim dosyasını değiştirmek onu sahibinin
yapar (D-031): bir hizmet uygulamasının bir sonraki etkinleştirmesi `unit_edited` ile reddedilir,
bir programın ya da Node.js sitesinin yazmaları yanında `.celikpanel-pending` olarak korunur;
böylece panel o birimi artık yönetmez.

**Panel kaldırılınca sunucuda kalanlar.** Sınırlar yeniden başlatmalardan sonra da çalışmaya devam
eder: systemd her açılışta dilim dosyasını, birimi onu adlandıran programlara uygular (dosya
yerindeyken bir yeniden başlatmada ölçüldü, N4 gerçekleri). Geride kalanlar:

- `/etc/systemd/system/celiksite_<kimlik>.slice`, sahibin `celiksite_<kimlik>.slice.d/`
  klasörleri ve varsa `celiksite_<kimlik>.slice.celikpanel-pending` ile
  `….celikpanel-aside-<zaman>` kopyaları;
- taşınan her programın birim dosyasındaki `Slice=celiksite_<kimlik>.slice` satırı;
- Ajan'ın özel klasöründeki kendi kayıtları (kayıt defteri, `site-slice-move.json`,
  `site-slice-move-outcomes.json`, `site-slice-runtime.json`): Ajan olmadan hiçbir şey yapmazlar;
- yalnız Ajan bir uygulamanın ortasında durduysa: `/run/systemd/system.control/celiksite_<kimlik>.slice.d/`
  içindeki denetim eklemeleri; bir sonraki açılışta giderler.

Panel kaldırıldıktan sonra sınırları elle kaldırmak için: her programa yukarıdaki ekleme dosyasını
yazın (`[Service]` ve `Slice=system.slice` içeren `<birim>.service.d/slice.conf`) ya da birim
dosyasında `Slice=celiksite_<kimlik>.slice` satırını `Slice=system.slice` yapın,
`celiksite_*.slice` dosyalarını ve `.d` klasörlerini
silin, `systemctl daemon-reload` çalıştırın ve her programı yeniden başlatın. Programları oldukları
yerde bırakıp yalnız bir sınırı kaldırmak için yukarıdaki ekleme dosyasını kullanın.

### 8.8 Önceki bir sürüme geri dönmek

**Başarısız bir güncelleme kendiliğinden geri döner; o anda bu bölümün hiçbir şeyi yoktur.**
Güncellemenin kendiliğinden geri dönüşü, yeni panelin salt okur açılış denetimine kadarki bir
hatayı kapsar; güncellemenin tamamlandı işareti bu denetimden sonra, yeni Ajan ve panel
başlatılmadan önce konur. Bu sürüm güncellemenin içinde hiçbir şey yapmaz: veritabanı geçişi yoktur
(yapı şema 81'de kalır), güncelleyici hiçbir programın birimini yeniden yazmaz ve sürüm arşivi
yardımcı kurmaz (Ajan, koruma yardımcısının kendi kopyasını ve bekçiyi açılıştan sonraki ilk
program yazmasında yerine koyar). Bu yüzden kendiliğinden geri dönüş ne dilim dosyası, ne taşınmış
program, ne bu sürümün yazdığı bir `site.*` ya da `db.*` değeri, ne de Ajan'ın bu bölümdeki
kayıtlarından birini bulur.

**Tamamlanmış bir güncelleme geri alınmaz.** Yeni Ajan ve panel başlatıldığı andan sonra
güncelleme sonuna kadar götürülür; o başlatma başarısız olsa da. Panel önceki sürüme dönmek için
bir yol sunmaz; sürümün `rollback.sh` betiği tamamlanmış bir güncellemeyi hiçbir şeyi durdurmadan
reddeder:
`!! a new rollback cannot reuse an earlier transaction's recovery material; preserve the snapshot and use a supported recovery plan`;
`sudo /usr/libexec/celikpanel/recovery recover` ise yalnız yarıda kalmış bir güncellemeyi ya da
geri almayı bitirir. Bu yüzden ürünün hiçbir yolu, bu sürümü çalıştırmış bir sunucuya önceki
sürümü (ya da daha eskisini) geri getirmez. Önceki sürüm alpha.147 sitesinin diliminde çalışan bir
programı zaten kabul eder (yardımcısı, bekçisi ve kanıtı), bir planı ya da hesabı kaydederken
tanımadığı sınır anahtarlarını korur ve o dilimin dosyası durdukça bir birimin
`Slice=celiksite_<kimlik>.slice` satırını korur; bu bir hazırlıktır, geri dönüş yolu değildir.

**Bu sürümün yaptıkları bu sürümde geri alınır:**

- Bir sitenin sınırları panel olmadan sahibin ek ayar dosyasıyla kaldırılır ya da değiştirilir
  (§8.7, "Sahibin bağımsızlığı"); planda, hesapta ya da sunucunun sınırlarında bir `site.*`
  anahtarını boşaltmak siteyi varsayılana döndürür.
- Bir program sitesinin diliminden sahibin ekleme dosyasıyla çıkarılır (§8.7, "Panel kalırken tek
  bir programı sitesinin sınırlarının dışına almak"): panelde bunun için bir işlem yoktur ve panel
  programı yönetmeyi sürdürür.
- Bir veritabanı kullanıcısının süre ve geçici dosya sınırları: `db.*` anahtarını konduğu yerde
  boşaltmak, panelin kendi yazdığı değeri (ve yalnız onu) kaldırmasını sağlar; sunucu sahibinin
  elle koyduğu bir değeri sahibi kaldırır (`ALTER ROLE … RESET …`, MariaDB için
  `ALTER USER … WITH MAX_STATEMENT_TIME 0`).

## 9. Sitenin zamanlanmış görevleri kendi sınırları içinde (S4 R2b)

*22 Kasım 2026; 26 Kasım 2026'da alpha.149'un üstüne getirildi (programların taşımasındaki gibi
tek taşıma kaydı, güncelleme kapısı ve her basışa tek süre, §8.4) · Kaynakta var ve bileşen
testleriyle denendi; aşağıdaki gerçekler geçici Debian 13 ve Ubuntu 24.04 makinelerinde ölçüldü;
panelin iki sistemdeki laboratuvar turu açık (adımlar çekme isteğinde).*

Taşınmış bir sitenin zamanlanmış görevleri artık cron ile çalışmaz. Etkin her görev bir
systemd zamanlayıcı ve hizmet çiftidir: `celikcron-<site kimliği>-<n>.timer` ve `.service`.
Hizmet sitenin diliminde (§8), sitenin öteki programlarının ağ kurallarıyla çalışır. **Yeni
siteler baştan taşınmıştır. Var olan bir site, yönetici onu taşıyana kadar cron'da kalır:**
programlarıyla aynı "Sınırların içine taşı" işlemiyle (§8.4, §9.3) ya da hâlâ dışarıda olan
bütün siteler için "Hepsini taşı" ile.

**Bu adımdan sonra hâlâ sınırların dışında çalışanlar.** Açıkça: sitenin PHP'si (PHP-FPM
havuzu; R3), henüz taşınmamış bir sitenin programları ve crontab'ı, R2a'dan önce yazılmış bir
program (taşınana kadar sunucunun her yeniden başlatılmasından sonra yine, §8.3), taşınmış bir
sitenin hesabı için yeniden yapılan bir crontab (bildirilir, §9.4) ve bu adımdan önce kalıcı oturum
açmış bir site hesabının programları (§9.5).

### 9.1 Gerçek sistemlerde ölçülen gerçekler (22 Kasım 2026)

Temel görüntülerden başlatılan iki geçici makinede düz `crontab`, `systemctl` ve `loginctl`:
Debian 13 (çekirdek 6.12.105, systemd 257.13, cron 3.0pl1-197, polkitd 126 kurulu ve gerektikçe
başlıyor) ve Ubuntu 24.04.4 (çekirdek 6.8.0-138, systemd 255.4, cron 3.0pl1-184ubuntu2, polkitd 124
çalışıyor). Bir hücre başka demedikçe ikisi de aynı yanıtı verdi.

| | Soru | Yanıt |
|---|---|---|
| F1 | `/etc/cron.allow` bir hesabı dışarıda bırakıyor | hesabın **var olan crontab'ı her dakika çalışmayı sürdürür**; `crontab -u <hesap> -l`, `-r` ve dosya yükleme root için bile reddedilir ("The user … cannot use this program (crontab)"); root `/var/spool/cron/crontabs/<hesap>` dosyasını yine okuyabilir, silebilir ve yerinde yeniden yazabilir (yenisini oluşturamaz: `fs.protected_regular=2`); cron silinen ya da değişen dosyayı sinyal olmadan bir sonraki dakikada fark eder |
| F2 | Aynı satır cron ile ve zamanlayıcı + hizmetle (`User=`, `Slice=`) | aynı hesap, gruplar, ev klasörü, çalışma klasörü, `LANG`, yetenekler; farklı olanlar: `PATH` (cron `/usr/bin:/bin`, Ubuntu'da `/etc/environment` içindeki), `SHELL` (cron `/bin/sh` koyar), umask (cron 0002), oturumun giriş kimliği, systemd'nin ek değişkenleri, standart girdi (boş bir boru, `/dev/null`) ve standart hata (günlük) |
| F2 | Komutta `%`; iki `OnCalendar=` satırı; `AccuracySec=` | standart girdi bayt bayt aynı; iki satır da kabul edildi ve ikisinin sonraki zamanı listelendi; varsayılan doğruluk bir dakikadır, bu yüzden `AccuracySec=1s` cron'un dakikasını korur |
| F2 | Aralığından uzun süren bir iş | cron her seferinde yeni bir kopya başlatır (üst üste biner); zamanlayıcı hiçbir zaman ikinci bir kopya başlatmaz, iş çalışırken saati gelen bir zaman onu **bittikten hemen sonra bir kez daha** başlatır (`Type=oneshot` ve `Type=simple` aynı); günlük atlanan zaman için bir şey söylemez |
| F3 | Oturumu olmayan bir site hesabı (bir sitenin PHP'si gibi) `loginctl enable-linger <kendisi>` çalıştırır | dağıtımın polkitd'siyle **izin verilir** (`set-self-linger` herkes için "evet"); sonra kendi kullanıcı hizmetlerini diliminin dışında, `user@<uid>.service` içinde çalıştırır; polkitd olmayan Debian: root dışında her hesaba "Access denied" |
| F3 | Çağıran yönetici grubunda değilse yönetici parolası isteyen bir kural dosyası | polkit 124 ve 126'da çalışır: site hesabı "Interactive authentication required" alır, bir yönetici ve root yine başarır; var olan bir `/var/lib/systemd/linger/<hesap>` kalır (kalıcı oturum sürer, `systemd-logind` yeniden başlasa da) |
| F4 | Giden posta engeli | bir cron işi `/system.slice/cron.service` içinde çalışır ve engellenmez; sitenin dilimindeki bir hizmet ancak kendi cgroup'u eklenince engellenir (yardımcının `egress-add` adımı), dilimde olduğu için değil |

### 9.2 Dosyalar ve bir görevin çalışması

| Dosya | Ne |
|---|---|
| `/etc/celikpanel-cron/<site kimliği>.crontab` | sitenin görevleri crontab biçiminde, tek kaynak (0711 bir klasörde 0640 root:root; CelikPanel'in oluşturma başlığı ve altı satırlık bir `#|` girişi) |
| `/etc/celikpanel-cron/<site kimliği>.d/<n>.sh`, `<n>.in` | n numaralı görevin komutu, cron'un kabuğa verdiği haliyle, ve standart girdisi (`%`); sitenin grubuyla root'a ait 0640, o grubun 0750 klasöründe |
| `/etc/celikpanel-cron/<site kimliği>.d/env` | crontab'ın `AD=değer` ayarları (systemd'nin root olarak okuduğu bir ortam dosyası) |
| `/etc/systemd/system/celikcron-<site kimliği>-<n>.service`, `.timer` | birimler, D-031 kuralıyla |

Komut ve girdisi hiçbir zaman bir birimin kendi ayarlarında durmaz: systemd bir birimin
ayarlarını her yerel hesaba söyler (`systemctl show`); bir crontab ise yalnız kendi hesabınca
okunur. Bir görev, metni aynı kaldıkça numarasını korur (hizmeti onu `# celikpanel-task:`
satırıyla adlandırır); böylece son çalışması kendisinin kalır.

**Hizmet** (`Type=oneshot`, `KillMode=process`, `NoNewPrivileges=yes`): sitenin dilim dosyasına
`ConditionPathExists=` (aşağıda), `User=` ve `Group=` sitenin hesabı, `Slice=`
sitenin dilimi, `WorkingDirectory=` ev klasörü, `UMask=0002`, `Environment=PATH=/usr/bin:/bin
SHELL=/bin/sh`, ardından `EnvironmentFile=-/etc/environment`, `-/etc/default/locale` ve sitenin
kendi ayar dosyası (sonraki dosya üstün gelir), görevin girdisi varsa `StandardInput=file:<n>.in`, `ExecStart=<crontab'ın SHELL'i>
<n>.sh`, SMTP engelinin yardımcı satırları (§3; görevlerin kendi yardımcı kopyasıyla,
`/usr/lib/celikpanel-sandbox/sandbox-task-helper`, §9.4), `RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
AF_NETLINK`, programların günlük hız sınırı (`LogRateLimitIntervalSec=30s`, `LogRateLimitBurst=1000`)
ve `LimitCORE=0`, ayrıca systemd uygulayabiliyorsa yerel bağlantı ve meta veri adreslerine
`IPAddressDeny=`. Programların dosya sistemi korumasını (§1) **almaz**: bir cron işinin hiç olmadı,
olsaydı sitenin dosyalarını yazan görevleri durdururdu. PHP ve dosya sistemi görünümü R3'te gelir.

**Bir görevin içinde ek yetki yok (ikinci düzeltme turu).** Sitenin programlarındaki gibi
`NoNewPrivileges=yes` konur. systemd 257 ve 255'te ölçüldü: `User=` ile birlikte
`RestrictAddressFamilies=` bunu **getirmez** (o olmadan set-group-id bir `crontab` görevin içinde
grubunu aldı). Bu yüzden ek yetki isteyen bir program bir görevde çalışmaz: `sudo`, `su`, `crontab`,
`at`, dosya yetenekli `ping`, `dotlockfile` ve sunucunun kendi `sendmail` komutu; dolayısıyla bir
komut satırı betiğinden çağrılan ve iletiyi `sendmail`'e veren PHP `mail()` işlevi de. Bir görev
e-postayı bir posta kutusunun adı ve parolasıyla SMTP üzerinden gönderir (§3). Taşımanın ön
izlemesi böyle bir programı komut konumunda (komutun başında ya da `;`, `&&`, `||`, `|`'den sonra)
çağıran her görevi programın adına göre adlandırır.

**Bir görevin çalışır bıraktığı şey, sınırların içinde çalışmayı sürdürür (düzeltme turu).**
`KillMode=process` ile systemd, görev bitince yalnız görevin kendi kabuğunu durdurur; görevin arka
planda başlattığı bir süreç (`nohup worker &`, `setsid ./queue &`) birimin cgroup'unda kalır, yani
sitenin diliminde (bellek, süreç ve işlemci sınırları) ve SMTP engelinin altında: yardımcının
`egress-remove` adımı, içinde kendisinden başka bir süreç oldukça cgroup'u kümede tutar; koruyucu da
bir boşaltmadan sonra böyle cgroup'ları yeniden ekler (§3). İkisi de engel yönünde yanılır: yardımcı
görevin nerede çalıştığını ya da cgroup'unun süreçlerini okuyamazsa cgroup kümede kalır; koruyucu bir
görev cgroup'unun süreçlerini okuyamazsa onu içinde süreç varmış gibi ekler, yardımcı satırı
okunamayan çalışan bir birimi de yardımcısı engeli yüklüyormuş gibi ekler (alpha.150). Bir sonraki
çalışma yine başlar. systemd
çalışma bitince "Unit process <pid> (<ad>) remains running after unit stopped.", bir sonraki çalışma
başlarken "Found left-over process <pid> (<ad>) in control group while starting unit. Ignoring."
yazar; ikisi de beklenen satırlardır. Görevin hizmetini `systemctl stop` ile durdurmak onları
bitirmez; `systemctl kill <hizmet>` bitirir. Bir görevi kaldırmak (ya da sitenin cron'a geri
konması) da onları bitirmez, cron'un bitirmediği gibi: bir görevin geride bıraktığı süreç,
**yönetici bitirene kadar sitenin sınırları içinde çalışmayı sürdürür** (`systemctl kill
celikcron-<kimlik>-<n>.service`, görev kaldırıldıktan sonra da). Ölçüldü: birim dosyası kaldırıldıktan
sonra da birimin cgroup'unda, dilimin sınırları ve adres kuralı altında kalır; koruyucu onu cgroup'unun
yoluna bakarak SMTP engelinin altında tutar (§3). Hep çalışması gereken bir program sitenin
Programlar bölümüne aittir; müşteri onu orada görür, yeniden başlatır ve durdurur. Görev, biten işler
içindir.

**Önce sitenin sınır dosyası (düzeltme turu).** Sitenin dilim dosyası yerinde ve systemd'ce
yüklenmiş değilse sitenin hiçbir zamanlayıcısı açılmaz: yeni bir sitenin dosyası siteyle birlikte
yazılır, her Zamanlanmış görevler isteği sitenin sınırlarını taşır ve eksik dosyayı yazar, her
Panel açılışından sonraki yan kapılar adımı da eksik dosyayı Panel'in gönderdiği sınırlardan geri
koyar. Yazılamayan bir dosya, bir günlük satırı değil, kimin ne yapacağını söyleyen bir yanıttır:
bir kayıt, görevin sitenin sınırlarını beklediğini söyler (görev dosyası kaydedilir, zamanlayıcıları
henüz yazılmaz), taşıma hiçbir şey değişmeden reddedilir; açılışta dosyasını yerinde bulamayan
çalışan zamanlayıcılar bekletilir (durdurulur ve hatırlanır; dosyayı yerinde bulan ilk açılışta
yeniden başlar) ve Şu anki sorunlar'da bir satır çıkar.

**Panel olmadan da bekletilir (ikinci düzeltme turu).** Her görev hizmeti
`ConditionPathExists=/etc/systemd/system/celiksite_<kimlik>.slice` taşır. İki sistemde, iki yeniden
başlatmayla ölçüldü: dosya yokken systemd her çalışmayı **atlar** (bir bilgi satırı, "skipped, unmet
condition check"; birim başarısız sayılmaz, `systemctl --failed` boştur), sunucu yeniden başlasa da ve
hiç panel olmasa da. Zamanlanmış görevler ekranı böyle bir çalışmayı başarısız değil, atlandı diye
gösterir. **Sınır dosyası yokken çalışmazlar; dosya geri gelince bir sonraki saatlerinde yeniden
çalışırlar.** Ölçülen bir açık: `systemctl daemon-reload` **olmadan** geri konan bir dilim dosyası,
systemd'nin dosyasız yaptığı dilimi yerinde bırakır; sonraki çalışmalar bir yeniden yüklemeye ya da
yeniden başlatmaya kadar **sınırsız** çalışır. CelikPanel'in dosyayı kendi geri koyuşu (aboneliğin
sınırlarının kaydı, açılış adımı) onu yazar, systemd'yi yeniden yükler ve sınırları çalışan dilime
iletir; bekletilen zamanlayıcılar ancak ondan sonra yeniden başlar. Dosyayı elle geri koyan sunucu
sahibi hemen ardından `systemctl daemon-reload` çalıştırır.

**Zamanlayıcı**: satırdan çevrilen `OnCalendar=` (`internal/sitecron`): her değer listesi
systemd'nin listesi olarak, cron'un iki gün alanı için VEYA'sı (ikisi de `*` ile başlamıyorsa) iki
`OnCalendar=` satırı olarak, haftanın günü için `7` ve adlar, ay adları, aralıklarda adımlar ve
özel sözcükler; `@reboot` `OnBootSec=1min` olur, **etkinleştirilir ama başlatılmaz** (cron'un
`@reboot` satırlarını kendisi başlarken çalıştırması gibi, sunucunun her açılışından sonra bir kez
çalışır); `AccuracySec=1s`; `Persistent=false` (cron kaçırılanı yetiştirmez). Çevirinin bilmediği
bir satır (`L`, `W`, `#`, `?`, aralık dışında bir değer, geriye giden bir aralık, 0 adımı)
çevrilmez.

**Bir satır tam cron 3.0pl1'in okuduğu gibi okunur ya da reddedilir (düzeltme turu; Debian'ın
`entry.c`, `env.c` ve `cron.h` dosyaları).** Alanları yalnız bir ASCII boşluk ya da sekme ayırır;
görev dosyasında da ekranın zamanlamayı denetlemesinde de: bölünmez boşluk ya da başka bir Unicode
boşluğu alanın parçasıdır ve alan, cron'un reddettiği gibi reddedilir. Komut sondaki boşluklarını
korur. 999 bayt ya da daha uzun bir komut reddedilir (cron onu çok uzun diye reddeder); ekran da
kaydetmez. Bir ayar satırı `load_env`'i izler: `AD=` boş değerdir, `A="x` ve `A="x"y` ayar değildir
(cron onları iş olarak okur ve reddeder), tırnaklar ve sondaki boşluklar cron'un kaldırdığı gibi
kalkar, 1000 bayt ya da daha uzun bir ayar satırı reddedilir (cron onu sessizce keserdi). systemd'nin
bir ortam dosyasında okuyamayacağı bir değer (geçerli UTF-8 olmayan, U+FEFF, karakter olmayan bir kod
noktası, tırnak işareti, ters eğik çizgi, denetim karakteri) çevrilemeyen bir satırdır: taşımada
crontab'ı cron'da tutar, ayar dosyasına asla yazılmaz ve böyle bir satır taşıyan görev dosyasının
kaydı reddedilir. cron'un kabul edip çevirinin reddettiği birkaç biçim (`007` gibi ikiden çok basamaklı
sayılar, 99'dan büyük adımlar, `fri-sun` gibi geriye giden bir gün aralığı) nedeniyle gösterilir.
**Bazı cron'lar için zamanlamanın parçası olan bir ayar reddedilir (alpha.150 düzeltme turu):**
`CRON_TZ=`, `RANDOM_DELAY=` ve `START_HOURS_RANGE=` cronie'de (ve systemd-cron'da) işlerin ne zaman
çalıştığını değiştirir; Debian'ın ve Ubuntu'nun cron 3.0pl1'i ise onları sıradan ayar olarak tutar
(programında böyle bir ad yoktur). Bir zamanlayıcı yalnız ayarı tutardı, yani zamanlama sessizce
değişebilirdi; bir sunucunun hangi cron'u çalıştırdığı güvenilir biçimde ayırt edilemediği için böyle
bir satır taşımada bütün crontab'ı cron'da tutar (adı ve nedeni söylenerek) ve böyle bir satır tutan
görev dosyasının kaydı her sunucuda reddedilir. Sunucu sahibi satırı siler ve saati her görevin
kendi zamanlamasında verir. systemd-cron'un bir işin zamanını değiştiren kendi adları
(`PERSISTENT=`, `BATCH=`, `DELAY=`) **reddedilmez** (düzeltme turu 4, olduğu gibi): desteklenen
sistemler kendi dağıtımlarının cron'unu çalıştırır, onun için bunlar sıradan ayardır; onun yerine
systemd-cron çalıştıran bir sunucu böyle bir satırı yalnız ayar olarak tutar. **Go çalışma ortamının okuduğu adlar da öyledir (alpha.150 düzeltme
turu):** `GODEBUG`, `GOGC`, `GOMAXPROCS`, `GOMEMLIMIT`, `GOTRACEBACK` ve `GORACE`. Sitenin ayar
dosyası görevin root olarak çalışan yardımcı satırlarının (`ExecStartPre=+`) da ortamıdır; yardımcı,
ortamını ilk iş olarak temizleyen durağan bir Go programıdır, ama çalışma ortamı bunları ondan önce
okur. Böyle bir satır reddedilir (taşımada crontab'ı cron'da tutar ve çalışmıyor diye gösterilir),
ayar dosyasına hiçbir zaman yazılmaz; buna ihtiyaç duyan bir görev onu komutunda verir
(`GOGC=off ./is`).

**Bir cron işi ile bir zamanlayıcı görevi: farklar.**

| | cron işi | zamanlayıcı görevi |
|---|---|---|
| hesap, gruplar, ev klasörü, çalışma klasörü, `LANG`, kabuk | aynı | aynı |
| `SHELL`, umask | cron'un koyduğu gibi | aynı yapıldı (yukarıda) |
| `/etc/environment` bir `PATH` koyduğunda `PATH` | Debian: yine `/usr/bin:/bin`; Ubuntu: `/etc/environment`'taki | ikisinde de `/etc/environment`'taki (`Environment=`'tan sonra okunur; `/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin` ile ölçüldü); crontab'ın kendi `PATH=` satırı ikisinden de üstündür |
| hem `/etc/environment`'ta hem crontab'da konan bir değişken | `/etc/environment`'taki (PAM en son koyar) | crontab'ınki (sitenin kendi ayar dosyası en son gelir) |
| `/etc/environment` içinde `export AD=değer` satırları ve noktalı ya da tireli adlar (`BENIM.AD`, `BENIM-AD`) | `export` satırları alınır; noktalı ve tireli adlar yok sayılır | üçü de yok sayılır (systemd "Ignoring invalid environment assignment" yazar); görev yine başlar |
| `/etc/environment` içinde `AD = değer` (`=` çevresinde boşluk) ve girintili satırlar | yok sayılır; girinti atılır | `AD=değer` olarak alınır; girintili satır öndeki boşlukları adında taşır |
| `$0` | `/bin/sh` | görevin dosyası (`<n>.sh`) |
| ek ortam | — | `USER`, `INVOCATION_ID`, `JOURNAL_STREAM`, `MEMORY_PRESSURE_*`, `SYSTEMD_EXEC_PID`, `TRIGGER_*` |
| giriş kimliği (denetim) | PAM koyar | konmaz |
| `%` olmadan standart girdi | boş bir boru | `/dev/null` (ikisi de hemen biter) |
| çıktı | e-postayla gönderilmez (alpha.125'ten beri `MAILTO=""`) | çalışmayla birlikte günlükte saklanır |
| açık dosya üst sınırı (Ubuntu) | 1048576 | 524288 |
| saati geldiğinde hâlâ çalışıyorsa | ikinci bir kopya başlar | yeniden başlatılmaz; bittikten hemen sonra bir kez daha başlar |
| arka planda çalışır bırakılan bir süreç | çalışmayı sürdürür | sitenin diliminde ve §3'ün kuralı altında çalışmayı sürdürür; systemd bunu yazar (yukarıda) |
| set-user-id ya da set-group-id bir program (`sudo`, `crontab`, dosya yetenekli `ping`, `dotlockfile`, `at`, sunucunun `sendmail`'i ve dolayısıyla bir betikten PHP `mail()`) | yetkisini alır | yetkisi olmadan çalışır (`NoNewPrivileges=yes` açıkça konur) ve başarısız olabilir; e-posta bir posta kutusu girişiyle SMTP üzerinden gider |
| sitenin sınır dosyası yok | — | çalışma atlanır (başarısız sayılmaz), yeniden başlatmadan sonra da, panel olmadan da |
| sunucu sahibinin giriş başına sınırları (`/etc/security/limits.conf`, `pam_limits`) | geçerlidir (cron bir PAM oturumu açar) | geçerli değildir (PAM oturumu yok); sitenin dilim sınırları geçerlidir |
| aynı saniyede başlayacak çok görev | hepsi o saniyede başlar (cron rastgele gecikme koymaz) | aynı: rastgele gecikme eklenmez |
| sınırlar, giden posta | yok; her porta posta | sitenin dilimi; §3'ün kuralı |

**İnsanların okuduğu.** Zamanlanmış görevler ekranı taşınmış bir site için uzun süren bir
çalışmanın iki yarısını tek cümlede ve bir sitenin fark edebileceği cron farklarını (arka planda
başlattığı şey sınırların içinde çalışmayı sürdürür; çıktı saklanır, e-postayla gönderilmez; giriş
oturumu yok, bu yüzden giriş başına sınırlar geçerli değil; set-user-id bir program yetkisi olmadan
çalışır; `$0`) söyler; taşımanın ön izlemesi yaygın bir set-user-id programı çağıran her görevi
adlandırır ve bir görevin nasıl e-posta gönderebileceğini (ya da bu sunucu üzerinden
gönderemeyeceğini, §3) söyler; her görev için son çalışmasını (ne zaman, ne kadar,
çıkış durumu ve systemd'nin onun için sözcüğü, kendi günlük kayıtlarından yazdığı son on satır),
ne zaman yeniden çalışacağını; sunucu sahibinin zamanlayıcısını kapattığı, CelikPanel'in
zamanlayıcısını yazıp henüz açamadığı (ikinci düzeltme turu: kendi durumu, asla "kapatıldı" değil;
bir kayıt ancak her yeni zamanlayıcı açıldığında uygulanmış sayılır ve bir sonraki açılış ya da kayıt
yeniden dener), birimi sunucuda değiştirilen, henüz zamanlanmamış ("yeniden kaydedin"), son çalışması
sitenin sınır dosyası olmadığı için atlanan ya da satırı çalışmayan (nedeniyle) görevleri de söyler. Sunucu yeniden başladıktan sonra o zamandan beri çalışmamış bir görev
"Sunucu açıldığından beri çalışmadı" der (systemd son çalışmayı o zaman unutur; günlük tutar).
Müşteri yalnız kendi sitesinin görevlerini görür. Biten bir çalışmanın son satırları günlükten bir
kez okunur ve saklanır (sınırlı bir önbellek); listenin bir okuması en çok sekiz günlük okuyucu
başlatır, henüz okunmamış çalışmaların satırları bir sonraki okumada görünür.

### 9.3 Var olan bir sitenin crontab'ını taşımak ve geri almak

Yöneticinin sitenin Genel Bakış sayfasındaki "Sınırların içine taşı" işlemi önce bir **ön izleme**
okur (neyin neye dönüştüğü, birlikte gelen ayarlar ve duraklatılmış görevler, set-user-id bir
program çağıran bir görev ve e-posta gönderen bir görev: taşımadan sonra yalnız bu sunucunun kendi
gönderim hizmetiyle ya da bu sunucu üzerinden hiç gönderemez, §3). Çevrilemeyen bir satır ve
CelikPanel'de yapılmamış, yöneticinin tutmadığı bir satır (S4 R1) **bütün** crontab'ı cron'da tutar
ve adı söylenir; hiçbir şey yarım taşınmaz. Taşıma, programların taşınmasıyla aynı kilitleri (aynı
anda tek taşıma; sitenin kilidi beklemeden, böylece silinmekte olan bir site taşınmaz) ve sitenin
görevlerinin her değişikliğinin kilidini tutar; hiçbir şeyi yeniden başlatmaz.

**Hesabı hâlâ iş kurabilen taşınmış bir site, CelikPanel'in yarattığı bir durum değildir (düzeltme
turu).** cron'un erişim dosyaları taşımadan sonra da hesaba (ya da uid'ini paylaşan bir girişe)
`crontab` kullandıracaksa, taşıma hiçbir şey değişmeden ve kimin ne yapacağı söylenerek reddedilir:
sunucu sahibinin kendi `/etc/cron.allow` dosyası onu adlandırıyorsa (sahip adı çıkarır ya da
CelikPanel kendi dosyasını yazsın diye dosyayı siler), CelikPanel'in dosyasını sahip sildiyse
(yönetici Şu anki sorunlar'da "Yeniden kapat"a basar) ya da hesaplar bir dizin hizmetinden geliyorsa
(sahip hesabı kendi dosyasıyla dışarıda tutar). Hesabı adlandıran bir `/etc/cron.deny` yeter. Ön
izleme de bunu söyler; hesabı izinli kalacak yeni bir site cron'da kalır - CelikPanel'in
`cron.allow` dosyası güncellendikten sonra yeniden denetlenir, böylece başarısız bir yazma da yeni
siteyi cron'da tutar (alpha.150 düzeltme turu).

**Kayıt** (taşımanın tek kaydı, Ajan'ın özel klasöründe `site-slice-move.json`: `tasks` bölümüyle
sürüm 3; programların taşıması sürüm 2 yazar): crontab'ın tam baytları (`crontab -l`'nin yazdığı
gibi) ve her adım, bir sonraki adım başlamadan yazılır. `source_written` adımından sonra aynı
crontab görev dosyasında da durur.

| Ulaşılan adım | Anlamı | Ajan'ın açılışında |
|---|---|---|
| planned | crontab'ın baytları kaydedildi | hiçbir şey ("denenmedi"); kaydından önce yazılmış bir görev dosyası kaldırılır |
| source_written | görev dosyası crontab'ı tutuyor | geri konur (aşağıda) |
| units_written | her zamanlayıcı ve hizmet yazıldı ve systemd'ce denetlendi, hiçbiri başlatılmadı | geri konur |
| cron_closed | `/etc/cron.allow` hesabı dışarıda bırakıyor (denetlendi) ve uid'inin bundan önce başlamış her `crontab` süreci sonlandırıldı (crontab'ın denetiminden daha önce geçmişti, yine de bir dosya kurabilirdi); bu süreçler listelenemezse taşıma burada başarısız olur ve geri konur, hiçbir şey silinmez (ikinci düzeltme turu); programı okunamayan ya da sonlandırılamayan bir uid süreci de böyledir (alpha.150 düzeltme turu: hiçbir zaman atlanmaz) ve liste hiçbir şey değişmeden önce bir kez okunur, böylece okunamayan bir liste taşımayı o anda reddeder. Bir sürecin programı bağlantının metniyle değil dosyasıyla tanınır (`/proc/<pid>/exe`'nin götürdüğü dosya `crontab` programınınkiyle karşılaştırılır; düzeltme turu 4): çekirdek, bir yolun olabileceğinden derin bir klasörden başlatılmış programın yolunu yazamaz, ama böyle bir program sitenin taşınmasını reddettiremez. Dosyası okunamayan bir süreç yine reddeder ve kendi koduyla söylenir: yönetici hesabın süreçlerine bakar ve ajanın günlüğünün adını verdiği süreci sonlandırır. Bir `crontab` süreci SIGKILL ile sonlandırılır ve beklenmez (olduğu gibi): biriktirme dosyası ondan sonra, bir sonraki adımda okunur ve sonlanmadan önce kurduğu bir crontab taşımayı geri koymaya çevirir | geri konur |
| crontab_emptied | kaydedilen crontab'ı tuttuğu denetlenen biriktirme dosyası doğrudan silindi (root yapabilir; `crontab -u` artık reddedilir) ve gittiği denetlendi | önce hesabın biriktirme dosyası okunur (aşağıda); crontab yoksa tamamlanır: zamanlayıcılar açılır ("taşındı"). Açılamazlarsa kayıt Tekrar dene için kalır ve açılışta geri konmaz (bu, açılışın yaptığından çok çağrı isterdi); zamanlayıcılar tek çağrıyla yeniden kapatılır - her zaman, sitenin sınır dosyası yok görünse de (Ajan durmadan önce gönderilmiş bir açma bazılarını açmış olabilir) - böylece sitenin görevlerinin Tekrar dene'ye kadar çalışmadığı bilinir |
| enabled | zamanlayıcılar etkinleştirildi ve başlatıldı | önce hesabın biriktirme dosyası okunur (aşağıda); crontab yoksa tamamlanır ("taşındı"). Crontab varsa (ya da dosya okunamıyorsa) zamanlayıcılar kapatılmadan önce kayıt `crontab_emptied` olarak yazılır (alpha.150 düzeltme turu 4); böylece crontab kalkınca sonraki geçiş zamanlayıcıları üstteki satırdaki gibi yeniden açar, kapalıyken asla "taşındı" demez |

**Taşıma anında bir çalışma kaçabilir, hiçbiri iki kez çalışmaz** (alpha.150 düzeltme turu):
zamanlayıcılar ancak crontab gittikten sonra başlar ve arada yeni bir dakika başlayabilir (cron'da
satır artık yok, zamanlayıcı henüz açık değil); o dakikada çalışacak bir görev bir sonraki zamanında
çalışır.

Sitenin kendi kodunun crontab'ı geri koyabileceği bir an yoktur: hesap, crontab'ı gitmeden önce
cron'un dışına çıkar. Crontab giderken hâlâ kaydedilen crontab olmalıdır: arada yapılan bir
değişiklik hiçbir zaman kaybolmaz (taşıma geri konur ve bunu söyler). **Geri koyma** taşımanın
zamanlayıcılarını kapatır, hesaba yeniden cron izni verir, kaydedilen crontab'ı yoksa geri yazar,
`crontab -l` ile denetler ve ancak ondan sonra zamanlayıcıları, dosyaları ve kaydı kaldırır. Crontab
ancak zamanlayıcıları kapatan çağrı başarı yanıtı verdikten sonra geri yazılır: yarıda başarısız
olan toplu bir `systemctl enable --now` bazılarını açık bırakmış olabilir ve yanlarındaki bir crontab
her görevi iki kez çalıştırırdı. Bu doğrulanamazsa ya da crontab geri yazılamazsa kayıt, görev
dosyası ve birimler kalır (kaydın adlandırdığı zamanlayıcılar sitenin sınırları izin veriyorsa
yeniden açılır); hiçbir şey kaybolmaz, hiçbir şey iki kez çalışmaz. Geri koyma, yazdığı ama aynı
biçimde geri okuyamadığı bir crontab'ın yanında zamanlayıcıları hiçbir zaman yeniden açmaz (alpha.150
düzeltme turu): biriktirme dosyasındaki kayıtlı crontab geri gelmiş sayılır; orada başka bir şey ya
da okunamayan bir dosya zamanlayıcıları kapalı ve kaydı yerinde tutar (görevler cron'dan çalışır).
**Bu bir geçiş için geçerlidir** (bilinen sınır, düzeltme turu 4): `source_written`, `units_written`
ya da `cron_closed` adımında tutulan bir kaydın sonraki geçişi (Tekrar dene, Ajan'ın açılışı) o anda
bulduğu her crontab'ı geri gelmiş sayar ve geri koymayı bitirir - zamanlayıcılar, dosyaları, görev
dosyası ve kayıt gider, crontab olduğu gibi kalır; kayıttaki crontab o zaman yalnız o crontab'ın
tuttuğu kadardır. `crontab_emptied` adımında aşağıdaki biriktirme dosyası denetimi kaydı tutar.
Taşıma, hesap için bir crontab varken hiçbir zaman "taşındı" demez.

**Tek kayıt, tek satır, Tekrar dene (alpha.150, alpha.149'un taşıma kaydının üstünde).** Görevlerin
taşıması ve geri alması programların taşımasıyla aynı tek kaydı tutar (§8.4). Kayıt dururken:

- **Her yeni taşıma ve geri alma** hiçbir şey değişmeden **reddedilir** (`kept_earlier`): başka bir
  sitenin kaydı hemen; aynı sitenin kaydı önce basışın kendi süresi içinde çözülür ve basış ancak
  çözülürse sürer. Panel, görevler bölümü "bilinmiyor" diyen ya da başlamadan reddedilen bir basışın
  programlar bölümünü başlatmaz ve "Hepsini taşı" orada durur.
- **Şu anki sorunlar'da tek satır** ("Sınırlara taşıma bitmedi") siteyi açar. Görev kaydı için
  sitenin zamanlanmış görevlerinin şu an nasıl çalıştığını Ajan'ın kaydettiği gibi söyler: yönetici
  Tekrar dene'ye basana dek **çalışmıyor** (crontab boşaltıldı, zamanlayıcılar kapalı); sınırlar
  içinde zamanlayıcı olarak çalışıyor; ya da geride kapalı zamanlayıcı dosyalarıyla cron'dan
  çalışıyor. Yalnız Ajan bunu bilemediyse çalışmıyor olabileceklerini söyler.
- **Sitenin kartı** aynısını, kaydın ulaştığı adımı, **Tekrar dene**'yi ve Tekrar dene çözemezse
  sunucu sahibinin görevleri elle cron'a geri koyma yolunu söyler (aşağıda).
- **Tekrar dene** kaydı çözer: adımı tamamlar ya da taşımayı geri koyar, basışın süresi içinde.
  Ajan'ın açılışı da kendi sınırı içinde dener (aşağıda).

**Kayıt dururken bir crontab (alpha.150 düzeltme turu).** `crontab_emptied` ve `enabled`
adımlarında, Ajan'ın açılışında da Tekrar dene'de de, önce hesabın biriktirme dosyası okunur (bir
dosya okuması, systemd çağrısı değil). Orada bir crontab - sunucu sahibi görevleri elle geri
koymuştur - ya da okunamayan bir biriktirme dosyası varsa **yanında zamanlayıcılar hiçbir zaman
açılmaz**: tek çağrıyla kapatılırlar ve kayıt kalır; kart ve Şu anki sorunlar görevlerin cron'dan
çalıştığını (dosya okunamıyorsa bunun bilinmediğini) söyler. Tekrar dene'de kayıttaki görevleri
tutan bir crontab - taşımadan önce `crontab -l`'nin yazdığı gibi ya da görev dosyasının bütünüyle
kurulmuş hâli - geri koymayı bitirir: zamanlayıcılar, dosyaları ve görev dosyası gider, cron hesaba
yeniden izin verir. Başka bir şey tutan crontab olduğu gibi bırakılır ve kayıt kalır (karşılaştırma
baytı baytınadır, yalnız "tutulur" yönüne yanılabilir); sunucu sahibi onu görev dosyasıyla
karşılaştırır ve birini tutar, kart da nasıl yapılacağını söyler (düzeltme turu 4), sonra yönetici
Tekrar dene'ye basar: **zamanlayıcıları tutmak için** sahip o crontab'ı siler (`sudo rm
/var/spool/cron/crontabs/<hesap>`, cronie'de `/var/spool/cron/<hesap>`; hesap için `crontab -r`
reddedilir) ve Tekrar dene zamanlayıcıları yeniden açar (`crontab_emptied` satırı); **kayıttaki
görevlerle cron'u tutmak için** sahip onu aynı biçimde siler ve aşağıdaki elle dönüş yolunu
çalıştırır, Tekrar dene de geri koymayı bitirir. Öteki adımlarda görevleri cron'dan çalışan bir
kaydı Tekrar dene crontab ne tutarsa tutsun bitirir (geri koymanın ve geri almanın kendi
satırları) ve kart yalnız bunu söyler.

**Elle cron'a geri dönüş yolu** (Tekrar dene kaydı çözemezse kart onu sitenin kimliği ve hesabıyla
verir; systemd 257'de atılacak birimlerle gerçekten çalıştırıldı, zaten var olan bir crontab ve her
adımda durduktan sonra ikinci bir çalıştırma da dahil, düzeltme turu 4; her adım yalnız bir
öncekisi başarılıysa çalışır):

```sh
{ [ ! -e /etc/cron.allow.celikpanel-before ] || sudo mv /etc/cron.allow.celikpanel-before /etc/cron.allow; } && { sudo test ! -e /var/spool/cron/crontabs/<hesap> && sudo test ! -e /var/spool/cron/<hesap> || { echo 'Stopped, nothing changed: <hesap> has a crontab. Press Try again.' >&2; false; }; } && sudo systemctl disable --now $(systemctl list-unit-files --no-legend 'celikcron-<kimlik>-*.timer' | awk '{print $1}') && sudo cp -p /etc/cron.allow /etc/cron.allow.celikpanel-before && echo <hesap> | sudo tee -a /etc/cron.allow >/dev/null && { sudo crontab -u <hesap> /etc/celikpanel-cron/<kimlik>.crontab; ok=$?; sudo mv /etc/cron.allow.celikpanel-before /etc/cron.allow && [ $ok = 0 ]; }
```

Sırasıyla: (1) daha önce yarıda kalmış bir çalıştırmanın bıraktığı `/etc/cron.allow` yedeği geri
konur; (2) **hesabın bir crontab'ı varsa hiçbir şeyi değiştirmeden durur** - biriktirme dosyasına
doğrudan bakılır (Debian ve Ubuntu'da `/var/spool/cron/crontabs/<hesap>`, cronie'de
`/var/spool/cron/<hesap>`), çünkü `/etc/cron.allow` dosyasının dışarıda bıraktığı bir hesap için
`crontab -l` root'a da reddedilir - böylece hiçbir crontab'ın üzerine yazılmaz; (3) sitenin
zamanlayıcıları adlarıyla listelenip kapatılır (`systemctl disable` desen almaz:
`'celikcron-<kimlik>-*.timer'` birim adı olarak reddedilir; hiçbiri listelenmezse bu adım başarısız
olur ve başka hiçbir şey çalışmaz); (4) `/etc/cron.allow` bir kenara kopyalanır; (5) hesaba o tek
`crontab` komutu için izin verilir (`/etc/cron.allow` dosyasının dışarıda bıraktığı bir hesap için
`crontab -u` root'a da reddedilir); (6) görev dosyası **şimdiki hâliyle** hesabın crontab'ı olarak
kurulur ve kopya - kurulum başarısız olsa da - baytı baytına geri konur; böylece CelikPanel'in
dosyası CelikPanel'in kalır ve başka hiçbir hesaba izin verilmez. Sunucuda kapatılmış bir görev
görev dosyasındaki gibi geri gelir (bir geri alma kaydı için de: adımlar hangi zamanlayıcıların
kapalı olduğunu okumaz). Sonra yönetici **Tekrar dene**'ye basar: crontab'ı yerinde bulur, geride
kalan zamanlayıcı dosyalarını ve kaydı kaldırır ve hesabın yeniden cron'u kullanmasına izin verir;
böylece siteler yeniden taşınabilir.

**Durduktan sonra yeniden çalıştırma** (ölçüldü): (1) ya da (2)'den sonra durduysa hiçbir şey
değişmemiştir, ikinci çalıştırma baştan başlar (crontab varken (2)'de yine durur); (3)'ten sonra
zamanlayıcılar zaten kapalıdır, ikinci çalıştırma onları yeniden kapatır ve sonuna kadar gider;
(4)'ten sonra (1) değişmemiş kopyayı geri koyar ve çalıştırma sonuna kadar gider; (5)'ten sonra (1)
hesabı yeniden çıkarır ve çalıştırma sonuna kadar gider (hesap bir kez listelenir, dosya
CelikPanel'inki olarak biter); kurulumdan sonra ama kopya geri konmadan durduysa (1)
`/etc/cron.allow`'u geri koyar ve crontab orada olduğu için (2) durur: yönetici Tekrar dene'ye
basar, o da crontab'ı yerinde bulur. Sonuna kadar giden her çalıştırma `/etc/cron.allow`'u baytı
baytına olduğu gibi ve yanında kopya olmadan bırakır. **`/etc/cron.allow` yoksa** (hesabı
`/etc/cron.deny` adlandıran bir sunucu) komut zamanlayıcılar kapandıktan sonra (4)'te durur, sonraki
her çalıştırmada da; sunucu sahibi o zaman hesabın adını `/etc/cron.deny`'den çıkarır ve `sudo
crontab -u <hesap> /etc/celikpanel-cron/<kimlik>.crontab` komutunu çalıştırır, yönetici de Tekrar
dene'ye basar. Crontab kurulmadan basılırsa Tekrar dene kaydı adımına göre çözer (`crontab_emptied`
adımından sonra: zamanlayıcılar yeniden açılır, yukarıdaki tablo).

**Ajan'ın açılışı sınırlıdır.** Bir görev kaydı Ajan'ın açılışında tek bir 15 saniyelik sınır
altında en çok **dört systemd çağrısıyla** çözülür (dilimin okunması, zamanlayıcıların açılıp
kapanması, bir yeniden yükleme); bu, her adım ve her hata için bir testle sayılır; açılış hiçbir
görevi ya da zamanlayıcıyı başlatmaz, durdurmaz, beklemez. Zamanında yanıt vermeyen bir çağrı
systemd'ye bırakılır ve bilinmiyor sayılır; ardından hiçbir şey gönderilmez. Açılışın çözemediği
kayıt Tekrar dene için kalır. **Sınırda bırakılan bir çağrı iptal edilmez** (alpha.150 düzeltme
turu, olduğu gibi): systemd onu tutar ve kayıt yazıldıktan sonra da yerine getirebilir - orada
bırakılan bir açma zamanlayıcıları sonradan açabilir ve sonraki bir Tekrar dene'nin kapatmasını
geçebilir. Bunun için systemd'nin sınırı aşacak kadar takılması gerekir; kayıt o zaman görevlerin
nasıl çalıştığının bilinmediğini söyler ve systemd yeniden yanıt verdiğinde bir Tekrar dene durumu
yeniden okur. Aynı sınır yalnız systemd çağrılarını kapsar: `crontab` komutu ve `cron.allow`
yazımları onunla kesilmez.

**Panelin güncellemesi.** Görevlerin taşıması, geri alması ve "Görev dosyasını yeniden kur" (§9.4)
programların taşımasıyla aynı hakkı alır: panelin güncellemesi sırada ya da sürüyorken
(`update_running`), bir güncelleme kaydı okunamazken (`update_unreadable`) ya da başka bir taşıma
sürerken (`another_move`) aynı sözlerle reddedilir; biri sürerken de güncelleme reddedilir (§8.4).

**Tek basış, tek süre.** Bir basış önce sitenin zamanlanmış görevlerini, sonra programlarını
taşımanın 11 dakikası içinde taşır: programlar bölümü görevler bölümünün bıraktığını alır, hiçbir
zaman fazlasını değil. Bir saniyeden az kaldıysa başlatılmaz; basış sürenin dolduğunu söyler ve bir
sonraki basış için her programı adlandırır. **Görevler bölümünün kendisi bu süreyle kesilmez**
(alpha.150 düzeltme turu): adımları, systemd çağrıları ve yazılan her birimin denetimi
(`systemd-analyze verify`, görev başına iki) sonuna kadar çalışır. Tek süre görevler bölümünden
sonra gelenler için geçerlidir. Görevler bölümü tek başına panelin basış için beklediğinden
(12 dakika; buna yalnız yavaş bir systemd'de çok büyük bir crontab ulaşır) uzun sürerse basış
sonucun bilinmediğini söyler, Ajan ise görevler bölümünü bitirmeyi sürdürür ve taşımanın hakkını
tutar: sayfa yeniden basmadan önce yeniden okunmasını söyler; o arada bir basış başka bir taşıma
sürüyor diye reddedilir.

**Geri alma** ("Görevleri cron'a geri koy", yönetici): sunucu sahibinin kendi `/etc/cron.allow`
dosyası hesabı dışarıda bırakıyorsa (ya da `/etc/cron.deny` onu adlandırıyorsa), görev dosyası elle
değiştirilmişse (ya da yoksa), hesabın yeniden bir crontab'ı varsa ya da sitenin zamanlayıcılarının
durumu okunamıyorsa (sunucu sahibinin hangi görevleri kapattığı bilinemezdi), hiçbir şey değişmeden
reddedilir. **Geri alma, sunucu sahibinin kapattığını yeniden başlatmaz (alpha.150 düzeltme turu,
D-022):** geri alma çalıştığında zamanlayıcısı etkin olmayan bir görev - sunucuda kapatılmış
(`systemctl disable --now`), CelikPanel'in yazıp henüz açamadığı değil - crontab'a CelikPanel'in
duraklatılmış satırı olarak (`# DISABLED: <satır>`), tek ve sabit bir yorum satırının, `# CelikPanel:
the task below stays paused, as its timer was switched off on the server.`, altında geri gelir
(düzeltme turu 4: bir satırın üstüne iki kez yazılmaz); yanıt bu görevleri ve zamanlayıcılarını
adlandırır, Zamanlanmış görevler ekranı birini yeniden başlatır. Sayılan, zamanlayıcının
**etkin** olup olmadığıdır: etkin olup yalnız durdurulmuş (`systemctl stop`) bir zamanlayıcının
görevi cron'a çalışır bir satır olarak geri gelir (düzeltme turu 4, olduğu gibi); yalnız bir sonraki
açılışa kadar etkin (`enable --runtime`) ya da maskelenmiş olan kapatılmış sayılır ve duraklatılmış
geri gelir. Ekran bu yorumu hiçbir zaman bir
görevin açıklaması olarak göstermez (üstündeki açıklama görevin açıklaması kalır) ve görevi
duraklatılmış gösterir; ekrandaki her kaydetme yorumu dışarıda bırakır, görev orada yeniden
başlatılınca yorum gider. Ekranda duraklatılmış bir görev görev dosyasında zaten böyle bir
satırdır ve olduğu gibi geri gelir. Bunun dışında crontab baytı baytına görev dosyasınınkidir. Geri
alma adım adım günlüğe yazılır; crontab ile zamanlayıcılar hiçbir zaman ikisi birden açık ya da
ikisi birden yok olmaz:

| Ulaşılan adım | Anlamı | Ajan'ın açılışında |
|---|---|---|
| undo_planned | yazılacak crontab (görev dosyasının baytları, sunucuda kapatılmış görevler duraklatılmış) ve açık zamanlayıcılar kaydedildi | kaydedilen zamanlayıcılar yeniden açılır, cron yeniden kapatılır ("denenmedi") |
| timers_disabled | bu zamanlayıcılar kapatıldı (çalışan bir görev durdurulmaz) | aynısı |
| cron_opened | CelikPanel'in `cron.allow` dosyası hesaba yeniden izin veriyor | biriktirme dosyası kaydedilen crontab'ı tutuyorsa tamamlanır ("geri alındı"); yoksa yukarıdaki gibi geri döner |
| crontab_written | crontab yazıldı ve denetlendi: `crontab -l` taşımadan önce verdiği baytların tam aynısını verir | tamamlanır: zamanlayıcılar ve dosyalar kaldırılır ("geri alındı") |

Cron açıkken sitenin kendi kodunun yaptığı bir crontab hiçbir zaman silinmez: geri dönüş onu bırakır
ve sınırların dışında çalışıyor diye bildirilir (§9.4). Geri almanın yazdığı crontab doğrulanamazsa
yalnız tam olarak geri almanın yazdığını tutan biriktirme dosyası yeniden kaldırılır; başka her dosya
olduğu gibi bırakılır ve yanıtta adı söylenir (ikinci düzeltme turu). Bir geri dönüşte zamanlayıcılar
yeniden açılamazsa ya da crontab geri geldikten sonra zamanlayıcı dosyaları kaldırılamazsa kayıt
yukarıdaki gibi kalır; kart ve Şu anki sorunlar görevlerin nasıl çalıştığını söyler (çalışmıyor ya
da cron'dan).

### 9.4 Sunucu sahibi ve görev dosyaları (D-022)

- **Ekranda.** Yönetici taşınmış bir sitenin görevlerini Zamanlanmış görevler ekranında
  değiştirir; CelikPanel görev dosyasını yazar ve yalnız değişen zamanlayıcıları oluşturur.
- **Elle, görev dosyasında.** CelikPanel sahibin dosyasını korur ve uygular: zamanlayıcıları onu
  ekrandaki bir sonraki kayıtta ve panelin her açılışından sonra izler (yan kapılar adımı, görev
  dosyası zamanlayıcıları yapıldığından beri değişen siteyi yeniden oluşturur). CelikPanel'in sonraki
  kendi değişiklikleri `<dosya>.celikpanel-pending` olarak yanında bekler ve ekran bunu söyler.
  Dosyayı geri vermek için sahip bekleyen dosyayı onun üzerine taşır.
- **Kaybolan bir görev dosyası (düzeltme turu).** Sitenin zamanlayıcıları dururken eksik bir görev
  dosyası bildirilecek bir durumdur, tek bir satırdan yeniden kurulacak bir durum değil: ekrandaki bir
  değişiklik nedeniyle reddedilir (öteki bütün görevleri silerdi). Yönetici ekranda tek bir işlem
  alır, "Görev dosyasını yeniden kur": önce dosyanın ne tutacağını gösterir — CelikPanel'in
  zamanlayıcıları en son neyden yaptığının, Ajan'ın özel klasöründe saklanan kopyası — sonra yazar.
  **Hesabın bir crontab'ı varken** (elle yarım kalmış bir geri alma ya da sahibin değiştirdiği bir
  birim dosyasını bırakan bir geri koyma) görevler o crontab'dan çalışır: yeniden kurma ve ön izlemesi
  reddedilir (her görevi iki kez çalıştırırdı); ekran görevlerin yeniden crontab'dan çalıştığını
  söyler ve geride kalan zamanlayıcı dosyalarını adlandırır; sunucu sahibi onları kaldırır
  (`sudo systemctl disable --now $(systemctl list-unit-files --no-legend 'celikcron-<kimlik>-*.timer'
  | awk '{print $1}') && sudo rm -f <adı söylenen dosyalar> && sudo systemctl daemon-reload`;
  ekranın yanıtı dosyaları adlandırır).
  Yeniden kurma birim yazar ve systemd'yi yeniden yükler; bu yüzden panelin güncellemesi ya da bir
  taşıma sürerken reddedilir, o sürerken de güncelleme (§9.3); sitenin bir taşıma kaydı dururken de
  kendisi ve ön izlemesi reddedilir: o kaydın Tekrar dene'si önce sitenin görevlerini çözer
  (alpha.150 düzeltme turu). Okunamayan bir taşıma kaydı her sitenin yeniden kurulmasını reddeder
  ve sözler bunun bu sitenin değil, bu sunucudaki bir taşımanın bitmediğini söyler (düzeltme turu 4).
- **Yerel olarak, birimlerde.** Değiştirilmiş bir `celikcron-*` birimi korunur (D-031) ve ekran onu
  adlandırır. Sahibin durdurduğu ya da kapattığı bir zamanlayıcı (`systemctl disable --now`) öyle
  kalır: değişen bir görev zamanlayıcısını yalnız çalışıyorsa yeniden başlatır; geri alma da
  kapatılmış birinin görevini cron'a duraklatılmış olarak geri koyar (§9.3).
- **Yeniden bir crontab.** Taşınmış bir hesap için var olan bir crontab her açılışta ve ekranın ve
  kartın her okumasında doğrudan biriktirme klasöründen okunur (Debian ve Ubuntu'da
  `/var/spool/cron/crontabs/<hesap>`, Arch'ta cronie ile `/var/spool/cron/<hesap>`) ve satırları
  sınırların dışında çalışıyor diye adlandırılır (yalnız yöneticiye). Yalnız hesap onu kendisi
  kuramayacaksa sunucu sahibinin diye söylenir: CelikPanel'in kendi, değişmemiş `cron.allow` dosyası,
  taşıma hesabın crontab'ı olmadığını denetlediğinden beri hesabı dışarıda tutuyorsa ve dosya ondan
  sonra yazıldıysa. Yoksa sözler sitenin kendi kodunun yapmış olabileceğini ve nedenini (cron hesaba
  izin verdi) söyler. Varken geri alma reddedilir.
- **CelikPanel olmadan.** Zamanlayıcılar, hizmetler ve dosyalar yereldir: panel durdurulsa,
  kaldırılsa ya da lisanssız kalsa da çalışmayı sürdürürler. Panel gittikten sonra
  `/etc/celikpanel-cron/<kimlik>.crontab` dosyasını düzenlemek **hiçbir** zamanlayıcıyı değiştirmez
  (o dosyayı birimlere yalnız panel çevirir); sahip bir görevi kendi dosyalarında değiştirir: saatini
  `/etc/systemd/system/celikcron-<kimlik>-<n>.timer` içinde, komutunu
  `/etc/celikpanel-cron/<kimlik>.d/<n>.sh` içinde, ardından `systemctl daemon-reload` (yeni bir görev
  yeni bir zamanlayıcı ve hizmet çiftidir, `systemctl enable --now <zamanlayıcı>` ile açılır). Her
  görevin çalışması önce `/usr/lib/celikpanel-sandbox/sandbox-task-helper`'ı çağırır (SMTP engeli,
  §3); CelikPanel'in dosyalarını kaldırmak görevleri de durdurur: onları kaldıran sahip önce siteyi
  cron'a geri koyar. Bir siteyi elle cron'a geri koymak için **önce zamanlayıcılar kapatılır, sonra
  crontab yazılır** (ikisi asla birlikte açık olmaz), her adım yalnız bir öncekisi başarılıysa,
  hesabın zaten bir crontab'ı varsa hiçbir şey yapılmaz (düzeltme turu 4: üzerine yazılırdı):
  `sudo test ! -e /var/spool/cron/crontabs/<hesap> && sudo test ! -e /var/spool/cron/<hesap> &&
  sudo systemctl disable --now $(systemctl list-unit-files --no-legend 'celikcron-<kimlik>-*.timer'
  | awk '{print $1}') && echo <hesap> | sudo tee -a /etc/cron.allow >/dev/null && sudo crontab -u
  <hesap> /etc/celikpanel-cron/<kimlik>.crontab` (`crontab -u`'nun root için de çalışması için
  `/etc/cron.allow` hesaba izin vermelidir ve panel artık cron'u kapatmadığı için sonra da izin
  verir; ilk altı `#|` satırı ve başlık satırı CelikPanel'in yorumlarıdır; `/etc/cron.allow` yoksa
  sahip hesabı onun yerine `/etc/cron.deny`'den çıkarır), sonra birimleri, klasörü ve görev
  dosyasını kaldırın ve `systemctl daemon-reload` çalıştırın.
- **Panel kaldırıldıktan sonra kalanlar.** Taşınmış sitelerin zamanlayıcıları, bekletilenler de
  (sitenin sınır dosyası yokken bekletilmiş kalırlar, §9.2); SMTP engeli, gönderim ayrığıyla son
  verildiği gibi (§3); taşınmış hesapları ve sonradan açılan her hesabı, sahip adını ekleyene ya da
  dosyayı silene kadar dışarıda tutan `/etc/cron.allow`; kalıcı oturum kuralı; ve site hesaplarının
  hizmet yöneticisi maskeleri (§9.5).
- **alpha.149'a dönmek (sürüm geçişi).** Şema değişmez (yerleşim 81).
  - **Başarısız bir güncelleme, bu sürümün Ajan'ı başlamadan kendiliğinden geri döner** (§8.8):
    güncelleme bu bölümün hiçbir görev birimini, görev dosyasını, `/etc/cron.allow` dosyasını,
    kalıcı oturum kuralını, maskesini, görev yardımcısını ya da kaydını kurmaz (her birini Ajan
    sonra yazar); geri dönüş bunların hiçbirini bulmaz.
  - **Tamamlanmış bir güncelleme geri alınmaz** (§8.8): panel geri dönüş yolu sunmaz, sürümün
    `rollback.sh` betiği tamamlanmış bir güncellemeyi reddeder ve kurtarma aracı yalnız yarıda
    kalmış olanı bitirir. Aşağıdakiler, alpha.149 buna rağmen bu sürümün sitelerin görevlerini
    taşıdığı bir sunucuya geri getirilseydi yapacaklarıdır.
  - **Görevler sitelerin sınırları içinde zamanlayıcı olarak çalışmayı sürdürür.** alpha.149 hiçbir
    `celikcron-*` birimini, `/etc/celikpanel-cron` dosyasını ya da görev kaydını tanımaz; onları ne
    kaldırır ne değiştirir. Kendi sitesinin diliminde duran bir görev hizmeti alpha.149 için o dilimi
    adlandırmış sayılır; dilim dosyası ve sınırları kalır. Görev birimleri kendi yardımcılarını
    çağırır: `/usr/lib/celikpanel-sandbox/sandbox-task-helper`; alpha.149 onu ne yönetir ne kaldırır,
    bu yüzden görevler SMTP engelinin altında başlar.
  - **SMTP engeli ve koruyucusu.** alpha.149 ilk program yazışında kendi SMTP engeli dosyasını ve
    koruyucusunu geri yazar: gönderim ayrığı gider (görevler bu sunucu üzerinden e-posta
    gönderemez); koruyucusu bir `nft flush ruleset`'ten sonra görevleri ve bıraktıklarını yine ekler,
    ama süreçlerini okuyamadığı bir görev cgroup'unu atlar.
  - **Zamanlanmış görevler ekranı.** `/etc/cron.allow` (CelikPanel'in dosyası; alpha.149 onu ne
    yazar ne siler) taşınmış hesapları ve sonra açılan her hesabı dışarıda tutar: alpha.149'un ekranı
    `/etc/cron.allow` hesabı listelemediği için crontab'ın okunamadığını söyler, zamanlayıcıları
    göstermez ve bu hesaplara görev ekleyemez. `/etc/celikpanel-cron/<kimlik>.crontab` dosyasını
    düzenlemek artık hiçbir zamanlayıcıyı değiştirmez.
  - **Kalıcı oturum kuralı** (`/etc/polkit-1/rules.d/49-celikpanel-linger.rules`) ve **site
    hesaplarının hizmet yöneticisi maskeleri** kalır; alpha.149 bunları tanımaz.
  - **Geri almanın duraklatılmış olarak geri koyduğu görevin üstündeki yorum** (§9.3) alpha.149
    için sıradan bir yorumdur: Zamanlanmış görevler ekranı onu İngilizce olarak o görevin açıklaması
    diye gösterir (üstüne yazılmış açıklama artık görünmez) ve görev orada yeniden başlatılınca da
    tutar. Görevin kendisi duraklatılmış kalır ve eskisi gibi yeniden başlatılır; yorum satırını
    sunucu sahibi elle siler.
  - **O anda tutulan bir görev kaydını** alpha.149'un açılışı programsız bir taşıma olarak okur:
    çözülmüş sayar ve siler. Crontab kaybolmaz - `source_written` adımından sonra görev dosyası onu
    tutar, daha önce de biriktirme dosyası tutuyordu - ama sitenin taşımasının bitmediğini artık
    hiçbir şey söylemez; crontab boşaltıldıktan sonra durmuş bir taşıma o sitenin görevlerini
    çalışmaz hâlde bırakır.
  - **Elle, taşınmış her site için** (root olarak; `<kimlik>` sitenin kimliği, `<hesap>` hesabı):
    önce zamanlayıcılar, sonra crontab, hiçbir zaman ikisi birden açık değil, her adım yalnız bir
    öncekisi başarılıysa ve hesabın zaten bir crontab'ı varsa hiçbir şey yapılmadan: `sudo test !
    -e /var/spool/cron/crontabs/<hesap> && sudo test ! -e /var/spool/cron/<hesap> && sudo systemctl
    disable --now $(systemctl list-unit-files --no-legend 'celikcron-<kimlik>-*.timer' | awk
    '{print $1}') && echo <hesap> | sudo tee -a /etc/cron.allow >/dev/null && sudo crontab -u
    <hesap> /etc/celikpanel-cron/<kimlik>.crontab` (ilk başlık satırı
    ve altı `#|` satırı CelikPanel'in yorumlarıdır; `/etc/cron.allow` yoksa sahip hesabı onun yerine
    `/etc/cron.deny`'den çıkarır). Önerilen sıra yine önce bu sürümde her sitenin görevlerini
    cron'a geri koymaktır (her sitenin Genel Bakış sayfasında "Görevleri cron'a geri koy").

### 9.5 Bir sitenin kendi işleri için son kapılar

**`/etc/cron.allow`** taşınmış sitelerin hesaplarının kendi crontab'larını kurmasını engeller. R1'in
`at.allow` kurallarıyla yazılır (`cmd/agent/side_doors_cron.go`): yalnız yoksa ve yalnız bir site
taşındıktan sonra; önce root, sonra bugün crontab kullanabilen her hesap (`/etc/cron.deny` içinde
adı geçmeyen), taşınmış sitelerin hesapları, CelikPanel'in araç hesapları ve bir sitenin kimliğini
paylaşan her giriş hariç (crontab, çağıranın uid'inin passwd'deki ilk adına göre karar verir).
Henüz taşınmamış bir site izinli kalır ve R1'in CelikPanel'de yapılmamış satırları işaretlemesi onun
için çalışmayı sürdürür. CelikPanel'in dosyası, CelikPanel'in yazdığı gibi kaldıkça her açılışta ve
her taşıma ya da geri almadan sonra güncellenir; sahibin dosyası bayt bayt korunur ve içinde adı
geçen taşınmış bir hesap Şu anki sorunlar'da bir satır olur; sahibin CelikPanel'in dosyasını silmesi
geçerli kalır (Şu anki sorunlar, "Yeniden kapat" ile); hesaplar bir dizin hizmetinden geliyorsa
hiçbir şey yazılmaz ve bu söylenir. Dosya bir hesabı dışarıda bıraktıktan sonra da hesabın var olan
crontab'ı çalışır (F1): taşıma onu kendisi boşaltır. Sahibin sonradan açtığı bir hesap, bir sonraki
Panel açılışından itibaren `crontab` kullanabilir (CelikPanel'in dosyası, `at.allow` gibi, o zaman
güncellenir); o zamana kadar ya da panel olmadan sahip adını dosyaya ekler ya da dosyayı siler —
dosyanın başlığı bunu söyler.

**Kalıcı oturum** polkitd kurulu olan yerde kapatılır: `/etc/polkit-1/rules.d/49-celikpanel-linger.rules`,
çağıran sistemin sahip olduğu bir yönetici grubunda değilse (`sudo`, `admin` ve `wheel` içinden;
Debian 13 ve Ubuntu 24.04'te `sudo`) `org.freedesktop.login1.set-self-linger` için "bir yöneticinin
parolası" yanıtını verir; CelikPanel'in oluşturma başlığını taşır ve ne yaptığını, nasıl geri
alınacağını kendi sözleriyle söyler (dosyayı silin; polkit değişikliği hemen okur). Yalnız bu üç
grup adı, `/etc/group`'un adlandırdığı haliyle sayılır: yöneticileri başka bir gruptan (örneğin bir
LDAP ya da SSSD grubundan) gelen bir sahip onlar için `sudo loginctl enable-linger <ad>` çalıştırır
ya da o grubu kurala ekler (dosya o zaman sahibinin olur ve CelikPanel onu korur); kuralın başlığı
bunu söyler. Sahibin değişikliği korunur; sahibin silmesi geçerli kalır (Şu anki sorunlar, "Yeniden kapat"). polkitd
olmadan systemd-logind zaten kalıcı oturumu yalnız root'a açtırır (F3). Ubuntu 22.04'te (polkit
0.105) kural dosyası okunmaz: adım kalıcı oturumu kapatılmamış diye bildirir. **Zaten kalıcı
oturumda olan hesaplar öyle kalır**: Şu anki sorunlar onları bitiren komutla adlandırır
(`loginctl disable-linger <hesap>`); hiçbir şey kendiliğinden kaldırılmaz.

**Bir site hesabının SFTP girişindeki kendi hizmet yöneticisi kapatılır (ikinci düzeltme turu).**
Debian 13 ve Ubuntu 24.04'te ölçüldü: bir site hesabının SFTP girişi `UsePAM yes` ve `pam_systemd`
ile sshd'den geçer; bu, `user@<uid>.service`'i `user.slice` içinde, sitenin diliminin ve SMTP
engelinin dışında başlatır; sitenin kendi kodu orada, bir oturum açık kaldıkça kendi birimlerini
çalıştırabilirdi (`~/.config/systemd/user` altına bırakılan birimler ya da oturum açıkken
`systemd-run --user`). `user@<uid>.service` maskelendiğinde SFTP girişi yine çalışır,
`/run/user/<uid>` yine yapılır ve hiçbir yönetici başlamaz. Bu yüzden her site hesabının uid'i bir
maske alır: `/dev/null`'a bağlanan `/etc/systemd/system/user@<uid>.service`. Bu, CelikPanel'in kendi
bağlantısıdır; bu bölümün öteki dosyaları gibi kaydedilir, bir site açılırken ve var olan siteler
için her Panel açılışından sonraki adımda yazılır (ardından `systemctl daemon-reload`) ve uid artık
bir site hesabına ait olmadığında (sitesi silindiğinde) kaldırılır. Maske yazıldığında açık olan bir
oturum yöneticisini bitene kadar tutar. root'a ya da bir yönetici hesabının (`sudo`, `admin` ya da
`wheel` üyesi) paylaştığı bir uid'e asla konmaz. O adda CelikPanel'in olmayan bir dosya ya da bağlantı
sunucu sahibinindir: korunur ve Şu anki sorunlar'da onu kapatan komutla adlandırılır; sahibin
CelikPanel'in bağlantısını kaldırması geçerli kalır ve aynı biçimde adlandırılır; yazılamayan bir maske
de adlandırılır. Panelin yapılandırdığı FTP hiçbir yönetici başlatmaz (ölçüldü). **Panel kaldırıldıktan
sonra maskeler kalır**; sahip birini `systemctl unmask user@<uid>.service` ile kaldırır.

**Bir site hesabının bu adımdan sonra hâlâ çalışır tutabildikleri**: sitesi taşındıktan ve polkitd
(ya da hiç polkitd olmaması) yukarıdaki gibi olduktan sonra kendisine ait yeni hiçbir şey —
crontab yok, `at` işi yok (R1), kalıcı oturumda kullanıcı yöneticisi yok, SFTP girişinde kullanıcı
yöneticisi yok. PHP'si (R3'e kadar) ve programları CelikPanel'in başlattığı yerde çalışır; daha önce
kalıcı oturum açmış bir kullanıcı yöneticisi sahip bitirene kadar, biten bir görevin geride bıraktığı
bir süreç de yönetici bitirene kadar sitenin sınırları içinde çalışmayı sürdürür (§9.2).
