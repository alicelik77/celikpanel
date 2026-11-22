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
  `smtpd_client_restrictions=permit_sasl_authenticated,reject`), başka hiçbir `master.cf` kaydı 465
  ya da 587'de dinlemiyorken ve orada dinleyen her soket Postfix'in ana sürecine aitken yazar
  (`cmd/agent/site_tasks_mail.go`). Bu portlarda sahibin kendi kaydı ya da başka bir program (bir
  msmtpd, bir akıllı sunucuya giden stunnel) varsa ya da hiç gönderim hizmeti yoksa dosyada ayrık
  yoktur ve sitenin Genel Bakış kartı, sitelerin görevlerinin ve programlarının bu sunucu üzerinden
  e-posta gönderemediğini ve nedenini söyler. Karar her Panel açılışında ve posta gönderim adımından
  sonra yeniden verilir; değişen kural takımı hemen çalışan tabloya yüklenir. 25 numaralı port her
  yerde kapalı kalır: Postfix'in aktarma listesi alpha.125'ten beri bu sunucunun kendi adreslerine
  parolasız izin vermiyor, engel de onu yine kapalı tutar.
- **Bilinen sınır: bu yoldan parola tahmini (erişim dilimi R4 için).** CelikPanel henüz kendi
  sınırlayıcısını eklemiyor. Bugün olan: Postfix'in `smtpd_client_auth_rate_limit` değeri
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
`ExecStartPre=+… sandbox-helper egress-add --unit=<birim>` tabloyu yükler ve birimin systemd'den
okunan gerçek cgroup'unu (`'{ "<dilim>/<birim>" }'`) kümeye ekler; `ExecStopPost=-+… egress-remove`
durduktan sonra çıkarır. nft boş bir ortamla çalışır. nft tabloyu yükleyemezse ya da birimin
cgroup'u ne `/system.slice/<birim>` ne de sitesinin `/celiksite_<id>.slice/<birim>` dilimiyse
(§8; örneğin sahibin bir ek ayar dosyası birimi başka bir dilime taşıdıysa) birim başlamaz; engel
kapalı kalacağına program hiç çalışmaz.

`celikpanel-egress-guard.timer` 30 saniyede bir çalışır. Biri `nft flush ruleset`
çalıştırdıysa ya da `nftables.service` yeniden başlatıldıysa tabloyu yeniden yükler ve çalışan
korumalı birimleri, bir de biten bir görevin çalışır bıraktığı bir süreci hâlâ tutan, bir sitenin
dilimindeki her görev cgroup'unu (§9.2) yeniden ekler. Bu arada engel en çok 30 saniye yok olabilir. Önceki bir
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

*21 Kasım 2026 · Kaynakta var ve bileşen testleriyle denendi; aşağıdaki gerçekler geçici
Debian 13 ve Ubuntu 24.04 makinelerinde ölçüldü; panelin iki sistemdeki laboratuvar turu açık
(adımlar çekme isteğinde).*

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
(`ControlGroup=`) öğesini ekler, yalnız sistem dilimini ya da bir sitenin dilimini kabul eder ve
başka her yeri reddeder; sahibin eklemesiyle başka yere taşınan birim (önceden olduğu gibi)
başlamaz. Bekçi zamanlayıcısı birimleri iki yerde de yeniden ekler.

**Sınırlar.** Dilim sitenin etkin `site.memory_mb`, `site.tasks` ve `site.cpu_percent` değerlerini
alır (docs/SITE-LIMITS.md §4a). systemd bir birimle üst dilimlerinin en sıkı sınırını uygular
("EffectiveMemoryMax= … is the most stringent limit of the unit and parent slices",
systemd.resource-control(5), systemd 257 ve 255). Birim kendi sınırlarını dosyasında tutar (Node.js
uygulaması 1536 MB, n8n 1536 MB); dilim dış sınırdır. Hiçbir düzey sınır koymamışsa dilim 512
işlem, %200 ve bellek için **3072 MB ile sunucu belleğinin %75'inden küçük olanı** alır
(`/proc/meminfo` içindeki `MemTotal`, tam MB'ye aşağı yuvarlanır, hiçbir zaman 64 MB'den az değil);
böylece tek bir site varsayılanla bütün sunucuyu alamaz.

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
Yapılacak dosyalar önce Ajan'ın özel klasörüne kaydedilir; çağrı ile silme arasında Ajan durursa
`/run` içinde aynı etkin değerler kalır (zararsız, bir sonraki açılışta zaten gider) ve Ajan'ın
bir sonraki başlangıcı onları siler. **Düşürülen sınır bu yüzden iki sistemde de hemen geçerli
olur**; bu nedenle önce sorulur (§8.5).

**Eksik dosya.** Dosyası olmayan dilimin sınırı yoktur (F4). Bir birim dilimi adlandırıyor ya da
içinde çalışıyorsa sınırların bir sonraki değişikliği olmayan dosyayı yeniden yazar, yöneticinin
taşıması da yazar; o zamana kadar sitenin kartı dosya eksik olduğu için sınırların geçerli olmadığını
söyler ve Şu anki sorunlar hiçbir şeyi sınırlanmış saymaz. Sahibin CelikPanel'e geri vermek için
sildiği dosya da aynı yolla yeniden yazılır.

**Yöneticinin kartı** sahibin eklemelerini dosya adıyla ve "sunucu sahibinin eklemeleri geçerli"
diyerek gösterir; değiştirilmiş dosyayı korunuyor diye anar.

### 8.3 Bir birim yazıldığında nereye gider

Değişen `Slice=` çalışan bir birime ancak bir sonraki başlangıcında ulaşır (F1). Bu yüzden:

- çalışmayan bir birim (yeni, durdurulmuş, her kurulum ya da derleme adımı) sitesinin dilimine
  yazılır;
- çalışan bir birim her sıradan yazmada (ayar değişikliği, güncelleme, müşterinin yeniden
  başlatması) çalıştığı dilimde kalır;
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
olarak yazılır.

- Taşıma sitenin kilidini beklemeden alır: tam o sırada silinen ya da değiştirilen site taşınmaz
  ("site şu an değiştiriliyor; bir dakika sonra sayfasını yeniden okuyun") ve taşıma silinmiş bir
  sitenin hiçbir şeyini yeniden yaratmaz.
- Önce her birimi okur: dosyasını sahibin değiştirdiği birim, eski bir korumadaki birim ve sahibin
  eklemesinin yerleştirdiği birim (eklemedeki `Slice=`, bir şey yeniden başlamadan önce okunur)
  taşınmaz ve yeniden başlatılmaz; kart onları kimin ne yapacağıyla adlandırır ve "dışarıda" satırına
  sayılmazlar.
- İlk yazmadan önce günlüğü kaydeder (aşağıda). Sonra dilim, sonra sırayla her birim, **o programın
  kendi değişiklik kilidiyle** (programın her başka değişikliğinin aldığı kilit): dosya taşımanın
  planladığı metin olmalıdır (değilse arada yapılmış bir değişikliktir: korunur, "değiştirildi"),
  CelikPanel'in değişmemiş metni `Slice=celiksite_<kimlik>.slice` alır (yeniden mühürlenir), systemd
  yeniden yüklenir ve çalışan birim bir kez yeniden başlar; **kendi başlama süresi içinde** dilimde
  çalışmalı ve önceden dinlediği her bağlantı noktasını yeniden dinlemelidir: birimin kendi
  `TimeoutStartUSec` değeri (bu birimler koymaz, yani systemd'nin 90 saniyesi), hiçbir zaman 20
  saniyeden az, 300 saniyeden çok değil. Bütün taşıma 10 dakika içinde kalır: beklemesi artık
  sığmayan program başlatılmaz ve yanıt yeniden basmayı söyler.
- Bir birim başarısız olursa o ve sitenin ondan önce taşınan her birimi taşıma öncesi baytlarına
  geri konur, ama yalnız taşımanın kendi metninin üstüne: arada değiştirilen ya da silinen dosya
  olduğu gibi bırakılır. Diğer sitelere hiç dokunulmaz.

**Günlük** (Ajan'ın özel klasöründe `site-slice-move.json`): taşımanın değiştireceği her birim
dosyasının taşıma öncesi baytları ve kipi, dilim dosyasının var olup olmadığı ve her birimin
ulaştığı adım. Ajan başlarken:

| Ulaşılan adım | Ajan'ın bir sonraki başlangıcında |
|---|---|
| planlandı (hiçbir şey yazılmadı) | dokunulmaz ("denenmedi") |
| yazıldı, durdurulmuş birim | korunur: dosyası dilimi adlandırır, orada başlar ("yeniden yazıldı") |
| yazıldı, çalışan birim (yeniden başlatılmadı) | taşıma öncesi baytlar geri, daemon-reload; hiç ayrılmadı ("geri kondu") |
| yeniden başlatılıyor (sonuç bilinmiyor) | taşıma öncesi baytlar geri, daemon-reload, yeniden başlatma ("geri kondu") |
| taşındı (dilimde çalıştı ve yanıt verdi) | korunur ("taşındı") |
| geri kondu ya da arada değiştirildi | başka bir şey yapılmaz |

Taşımanın yazdığını artık tutmayan dosya olduğu gibi bırakılır ("değiştirildi"). Taşımanın
yarattığı dilim dosyası, hiçbir birim onu adlandırmıyorsa yeniden silinir. Ajan'ın ne yaptığı
sitenin bir sonraki taşımasına kadar yöneticinin site kartında gösterilir ("son taşıma yarıda
kesildi … her program tamamlandı ya da geri kondu").

### 8.5 Sınırı sitenin kullandığının altına düşürmek

`site.memory_mb` değerini sitenin programlarının şu an kullandığının altına düşürmek çekirdeğin
onlardan birini hemen durdurmasına yol açar (F4); `site.tasks` değerini tuttukları işlem sayısının
altına düşürmek yeni işlemleri durdurur. Bu yüzden plan formu, bayi planı ve bir hesabın kendi
sınırları, değişiklik onaylanmadan önceki önizlemesinde (panelin kendi onayı) programları yeni
değerden fazlasını kullanan siteleri ve değişiklik kaydedilince sistemin onları hemen durduracağını
gösterir. Sınırların uygulanmasında başka hiçbir şey değişmez. Sunucunun kendi sınırlar ekranı
onları listelemez.

### 8.6 Bir sınıra ulaşıldığında insanların okuduğu (D-024)

Ajan dilimin sayaçlarını sitenin durum okumasıyla ve program çalıştıran bütün siteler için arka
planda en fazla beş dakikada bir okur (1000 sitelik sayfalarla; hiçbir okuma site sayısı yüzünden
reddedilmez; kayıt defteri her okumada bir kez okunur). Panel bir sayacın ne zaman arttığını kaydeder
(`panel_settings` `site_limit_events_v1`; bir sitenin ilk okuması başlangıç noktasıdır; yalnız bir
sınıra ulaşıldığında, bir sayaç yeniden başladığında ya da bir site gittiğinde yazılır).

- "Sitenin kendi sınırı" (bellek, işlem) yalnız §8.1'in kanıtladığıdır. Kart o zaman ne olduğunu ve
  ne zaman olduğunu, planın neye izin verdiğini ve kimin yükseltebileceğini söyler: müşteriye "hizmet
  sağlayıcınız", bayiye "kendi sınırlarınız içinde müşterinin planında siz", yöneticiye planın ya da
  bayinin adı. Bir günde iki ya da daha fazla okumada kendi bellek ya da işlem sınırına ulaşan site
  Şu anki sorunlar'da bir satırdır.
- Sitenin sınırı olmadan bellek yetmediği için durdurulan (kendi daha küçük sınırı ya da bütün
  sunucunun darlığı) ya da kendi sınırında işlemi reddedilen program yalnız bu kadarıyla söylenir
  ("bu sitenin bir programı bellek yetmediği için durduruldu"): plan hiçbir zaman suçlanmaz ve Şu
  anki sorunlar satırı çıkmaz.
- Sınırların geçerli olmadığı yerde (bütün programlar dışarıda, dilim dosyası yok, sınırları
  uygulayamayan sunucu) kart "sınırlarının içinde kaldı" yerine bunu söyler.

**Henüz taşınacak siteler.** Şu anki sorunlar'da yalnız yönetici için bir satır vardır: kendi
programları sınırlarının dışında çalışan ya da orada başlayacak olan ve taşımanın alabileceği siteler
(sayıları ve ilk beş adı). İlk siteyi açar, bir uyarıdır (yapılacak iş) ve hiç site kalmayınca
kendiliğinden kaybolur (`panel_settings` `site_limit_outside_v1`, yalnız liste değişince yazılır).

### 8.7 Silinen sitenin dilimi

Bir site silinince CelikPanel'in değişmemiş dilim dosyası gider ve kayıt defteri onu unutur; sahibin
değiştirdiği dosya ve sahibin ekleme klasörü systemd'nin yüklemediği adlarla kenara konur
(`<dosya>.celikpanel-aside-<UTC zamanı>`) ve denetim kaydında adlandırılır (`site.limits_file_aside`).
Yeni bir sitenin kimliği için bulunup Ajan'ın kayıt defterinin tanımadığı dilim dosyası (önceki bir
kurulumda aynı kimliğe sahip bir site) yenisi yazılmadan önce aynı biçimde ele alınır.

**Sahibin bağımsızlığı (D-022).** Dilimler yerel birimlerdir. Panel durdurulsa, kaldırılsa ya da
lisanssız kalsa da onlar ve sınırları çalışmaya devam eder. Bir sitenin sınırlarını panel olmadan
kaldırmak ya da değiştirmek için sahip bir ekleme dosyası yazar
(`/etc/systemd/system/celiksite_<kimlik>.slice.d/owner.conf`, örneğin `[Slice]` ve
`MemoryMax=infinity` ile) ve `systemctl daemon-reload` çalıştırır (systemd 257'de yeni değer çalışan
programlara onlardan biri başladığında ya da `systemctl set-property --runtime` ile ulaşır);
CelikPanel onu hiçbir zaman yazmaz ya da silmez.

## 9. Sitenin zamanlanmış görevleri kendi sınırları içinde (S4 R2b)

*22 Kasım 2026 · Kaynakta var ve bileşen testleriyle denendi; aşağıdaki gerçekler geçici
Debian 13 ve Ubuntu 24.04 makinelerinde ölçüldü; panelin iki sistemdeki laboratuvar turu açık
(adımlar çekme isteğinde).*

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

**Hizmet** (`Type=oneshot`, `KillMode=process`): `User=` ve `Group=` sitenin hesabı, `Slice=`
sitenin dilimi, `WorkingDirectory=` ev klasörü, `UMask=0002`, `Environment=PATH=/usr/bin:/bin
SHELL=/bin/sh`, ardından `EnvironmentFile=-/etc/environment`, `-/etc/default/locale` ve sitenin
kendi ayar dosyası (sonraki dosya üstün gelir; cron'da da crontab'ın ayarları PAM'inkilerden
üstündür), görevin girdisi varsa `StandardInput=file:<n>.in`, `ExecStart=<crontab'ın SHELL'i>
<n>.sh`, SMTP engelinin yardımcı satırları (§3), `RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
AF_NETLINK`, programların günlük hız sınırı (`LogRateLimitIntervalSec=30s`, `LogRateLimitBurst=1000`)
ve `LimitCORE=0`, ayrıca systemd uygulayabiliyorsa yerel bağlantı ve meta veri adreslerine
`IPAddressDeny=`. Programların dosya sistemi korumasını (§1) **almaz**: bir cron işinin hiç olmadı,
olsaydı sitenin dosyalarını yazan görevleri durdururdu. PHP ve dosya sistemi görünümü R3'te gelir.

**Bir görevin çalışır bıraktığı şey, sınırların içinde çalışmayı sürdürür (düzeltme turu).**
`KillMode=process` ile systemd, görev bitince yalnız görevin kendi kabuğunu durdurur; görevin arka
planda başlattığı bir süreç (`nohup worker &`, `setsid ./queue &`) birimin cgroup'unda kalır, yani
sitenin diliminde (bellek, süreç ve işlemci sınırları) ve SMTP engelinin altında: yardımcının
`egress-remove` adımı, içinde kendisinden başka bir süreç oldukça cgroup'u kümede tutar; koruyucu da
bir boşaltmadan sonra böyle cgroup'ları yeniden ekler (§3). Bir sonraki çalışma yine başlar. systemd
çalışma bitince "Unit process <pid> (<ad>) remains running after unit stopped.", bir sonraki çalışma
başlarken "Found left-over process <pid> (<ad>) in control group while starting unit. Ignoring."
yazar; ikisi de beklenen satırlardır. Görevin hizmetini `systemctl stop` ile durdurmak onları
bitirmez; `systemctl kill <hizmet>` bitirir. Bir görevi kaldırmak (ya da sitenin cron'a geri
konması) da onları bitirmez, cron'un bitirmediği gibi.

**Önce sitenin sınır dosyası (düzeltme turu).** Sitenin dilim dosyası yerinde ve systemd'ce
yüklenmiş değilse sitenin hiçbir zamanlayıcısı açılmaz: yeni bir sitenin dosyası siteyle birlikte
yazılır, her Zamanlanmış görevler isteği sitenin sınırlarını taşır ve eksik dosyayı yazar, her
Panel açılışından sonraki yan kapılar adımı da eksik dosyayı Panel'in gönderdiği sınırlardan geri
koyar. Yazılamayan bir dosya, bir günlük satırı değil, kimin ne yapacağını söyleyen bir yanıttır:
bir kayıt, görevin sitenin sınırlarını beklediğini söyler (görev dosyası kaydedilir, zamanlayıcıları
henüz yazılmaz), taşıma hiçbir şey değişmeden reddedilir; açılışta dosyasını yerinde bulamayan
çalışan zamanlayıcılar bekletilir (durdurulur ve hatırlanır; dosyayı yerinde bulan ilk açılışta
yeniden başlar) ve Şu anki sorunlar'da bir satır çıkar.

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

**Bir cron işi ile bir zamanlayıcı görevi: farklar.**

| | cron işi | zamanlayıcı görevi |
|---|---|---|
| hesap, gruplar, ev klasörü, çalışma klasörü, `LANG`, kabuk | aynı | aynı |
| `PATH`, `SHELL`, umask | cron'un koyduğu gibi | aynı yapıldı (yukarıda) |
| `$0` | `/bin/sh` | görevin dosyası (`<n>.sh`) |
| ek ortam | — | `USER`, `INVOCATION_ID`, `JOURNAL_STREAM`, `MEMORY_PRESSURE_*`, `SYSTEMD_EXEC_PID`, `TRIGGER_*` |
| giriş kimliği (denetim) | PAM koyar | konmaz |
| `%` olmadan standart girdi | boş bir boru | `/dev/null` (ikisi de hemen biter) |
| çıktı | e-postayla gönderilmez (alpha.125'ten beri `MAILTO=""`) | çalışmayla birlikte günlükte saklanır |
| açık dosya üst sınırı (Ubuntu) | 1048576 | 524288 |
| saati geldiğinde hâlâ çalışıyorsa | ikinci bir kopya başlar | yeniden başlatılmaz; bittikten hemen sonra bir kez daha başlar |
| arka planda çalışır bırakılan bir süreç | çalışmayı sürdürür | sitenin diliminde ve §3'ün kuralı altında çalışmayı sürdürür; systemd bunu yazar (yukarıda) |
| set-user-id ya da set-group-id bir program (`sudo`, `crontab`, dosya yetenekli `ping`, `dotlockfile`, `at`) | yetkisini alır | yetkisi olmadan çalışır (`RestrictAddressFamilies=` ile `User=` birlikte `NoNewPrivileges=yes` demektir) ve başarısız olabilir |
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
ne zaman yeniden çalışacağını; sunucu sahibinin zamanlayıcısını kapattığı, birimi sunucuda
değiştirilen, henüz zamanlanmamış ("yeniden kaydedin") ya da satırı çalışmayan (nedeniyle)
görevleri de söyler. Sunucu yeniden başladıktan sonra o zamandan beri çalışmamış bir görev
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
izleme de bunu söyler; hesabı izinli kalacak yeni bir site cron'da kalır.

**Günlük** (programların taşıma günlüğü, `tasks` bölümüyle sürüm 2): crontab'ın tam baytları
(`crontab -l`'nin yazdığı gibi) ve her adım, bir sonraki adım başlamadan yazılır.

| Ulaşılan adım | Anlamı | Ajan'ın bir sonraki açılışında |
|---|---|---|
| planned | crontab'ın baytları kaydedildi | hiçbir şey ("denenmedi"); kaydından önce yazılmış bir görev dosyası kaldırılır |
| source_written | görev dosyası crontab'ı tutuyor | geri konur (aşağıda) |
| units_written | her zamanlayıcı ve hizmet yazıldı ve systemd'ce denetlendi, hiçbiri başlatılmadı | geri konur |
| cron_closed | `/etc/cron.allow` hesabı dışarıda bırakıyor (denetlendi) ve uid'inin bundan önce başlamış her `crontab` süreci sonlandırıldı (crontab'ın denetiminden daha önce geçmişti, yine de bir dosya kurabilirdi) | geri konur |
| crontab_emptied | kaydedilen crontab'ı tuttuğu denetlenen biriktirme dosyası doğrudan silindi (root yapabilir; `crontab -u` artık reddedilir) ve gittiği denetlendi | tamamlanır: zamanlayıcılar etkinleştirilir ("taşındı"); bu başarısız olursa geri konur |
| enabled | zamanlayıcılar etkinleştirildi ve başlatıldı | tamamlanır ("taşındı") |

Sitenin kendi kodunun crontab'ı geri koyabileceği bir an yoktur: hesap, crontab'ı gitmeden önce
cron'un dışına çıkar. Crontab giderken hâlâ kaydedilen crontab olmalıdır: arada yapılan bir
değişiklik hiçbir zaman kaybolmaz (taşıma geri konur ve bunu söyler). **Geri koyma** taşımanın
zamanlayıcılarını kapatır, hesaba yeniden cron izni verir, kaydedilen crontab'ı yoksa geri yazar,
`crontab -l` ile denetler ve ancak ondan sonra zamanlayıcıları, dosyaları ve günlüğü kaldırır. Geri
yazılamayan bir crontab günlüğü, görev dosyasını ve birimleri korur (sitenin sınırları izin
veriyorsa yeniden açılır); hiçbir şey kaybolmaz, yanıt bunu söyler ve Ajan'ın bir sonraki açılışı
yeniden dener. Taşıma, hesap için bir crontab varken hiçbir zaman "taşındı" demez. Yöneticinin kartı
yarıda kesilen bir taşımanın sonucunu gösterir.

**Geri alma** ("Görevleri cron'a geri koy", yönetici): sunucu sahibinin kendi `/etc/cron.allow`
dosyası hesabı dışarıda bırakıyorsa (ya da `/etc/cron.deny` onu adlandırıyorsa), görev dosyası elle
değiştirilmişse (ya da yoksa) veya hesabın yeniden bir crontab'ı varsa, hiçbir şey değişmeden
reddedilir. Yoksa adım adım günlüğe yazılır; crontab ile zamanlayıcılar hiçbir zaman ikisi birden
açık ya da ikisi birden yok olmaz:

| Ulaşılan adım | Anlamı | Ajan'ın bir sonraki açılışında |
|---|---|---|
| undo_planned | yazılacak crontab (görev dosyasının baytları, hiçbir şey eklenmeden) ve açık zamanlayıcılar kaydedildi | kaydedilen zamanlayıcılar yeniden açılır, cron yeniden kapatılır ("denenmedi") |
| timers_disabled | bu zamanlayıcılar kapatıldı (çalışan bir görev durdurulmaz) | aynısı |
| cron_opened | CelikPanel'in `cron.allow` dosyası hesaba yeniden izin veriyor | biriktirme dosyası kaydedilen crontab'ı tutuyorsa tamamlanır ("geri alındı"); yoksa yukarıdaki gibi geri döner |
| crontab_written | crontab yazıldı ve denetlendi: `crontab -l` taşımadan önce verdiği baytların tam aynısını verir | tamamlanır: zamanlayıcılar ve dosyalar kaldırılır ("geri alındı") |

Cron açıkken sitenin kendi kodunun yaptığı bir crontab hiçbir zaman silinmez: geri dönüş onu bırakır
ve sınırların dışında çalışıyor diye bildirilir (§9.4).

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
- **Yerel olarak, birimlerde.** Değiştirilmiş bir `celikcron-*` birimi korunur (D-031) ve ekran onu
  adlandırır. Sahibin durdurduğu ya da kapattığı bir zamanlayıcı (`systemctl disable --now`) öyle
  kalır: değişen bir görev zamanlayıcısını yalnız çalışıyorsa yeniden başlatır.
- **Yeniden bir crontab.** Taşınmış bir hesap için var olan bir crontab her açılışta ve ekranın ve
  kartın her okumasında doğrudan biriktirme klasöründen okunur (Debian ve Ubuntu'da
  `/var/spool/cron/crontabs/<hesap>`, Arch'ta cronie ile `/var/spool/cron/<hesap>`) ve satırları
  sınırların dışında çalışıyor diye adlandırılır (yalnız yöneticiye). Yalnız hesap onu kendisi
  kuramayacaksa sunucu sahibinin diye söylenir: CelikPanel'in kendi, değişmemiş `cron.allow` dosyası,
  taşıma hesabın crontab'ı olmadığını denetlediğinden beri hesabı dışarıda tutuyorsa ve dosya ondan
  sonra yazıldıysa. Yoksa sözler sitenin kendi kodunun yapmış olabileceğini ve nedenini (cron hesaba
  izin verdi) söyler. Varken geri alma reddedilir.
- **CelikPanel olmadan.** Zamanlayıcılar, hizmetler ve dosyalar yereldir: panel durdurulsa,
  kaldırılsa ya da lisanssız kalsa da çalışmayı sürdürürler. Her görevin çalışması önce
  `/usr/lib/celikpanel-sandbox/sandbox-helper`'ı çağırır (SMTP engeli, §3); CelikPanel'in dosyalarını
  kaldırmak görevleri de durdurur: onları kaldıran sahip önce siteyi cron'a geri koyar. Bir siteyi
  elle cron'a geri koymak için: `crontab -u <hesap> /etc/celikpanel-cron/<kimlik>.crontab`
  (`/etc/cron.allow` izin verdikten sonra; ilk altı `#|` satırı ve başlık satırı CelikPanel'indir,
  önce silinebilir), ardından `systemctl disable --now 'celikcron-<kimlik>-*.timer'` ve birimleri ve
  klasörü kaldırın.

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

**Bir site hesabının bu adımdan sonra hâlâ çalışır tutabildikleri**: sitesi taşındıktan ve polkitd
(ya da hiç polkitd olmaması) yukarıdaki gibi olduktan sonra kendisine ait yeni hiçbir şey —
crontab yok, `at` işi yok (R1), kalıcı oturumda kullanıcı yöneticisi yok. PHP'si (R3'e kadar) ve
programları CelikPanel'in başlattığı yerde çalışır; daha önce kalıcı oturum açmış bir kullanıcı
yöneticisi, sahip bitirene kadar çalışmayı sürdürür.
