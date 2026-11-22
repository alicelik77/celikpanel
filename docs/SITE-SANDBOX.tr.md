# Site koruması (sandbox)

*alpha.119 · Kaynakta var, bileşen testlerinden geçti; WSL Debian 13 (systemd 257) üzerinde bir kez kısa deneme yapıldı. Desteklenen beş sistemde kaçış matrisi laboratuvarda bekliyor.*

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
- Bu programların 25, 465 ve 587 numaralı portlara açtığı bağlantılar reddedilir. Bu her adres
  için geçerlidir, 127.0.0.1 ve ::1 dahil. Reddedilen her deneme `smtp_rejects` sayacına
  yazılır.
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
korumalı birimleri yeniden ekler. Bu arada engel en çok 30 saniye yok olabilir. Önceki bir
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

*21 Kasım 2026, ikinci düzeltme turu 22 Kasım 2026 · Kaynakta var ve bileşen testleriyle
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
| sitenin PHP-FPM havuzu, zamanlanmış görevleri | | henüz değil: PHP ve görevler sonraki adımlarda dilime girer |
| `celikpanel-sandbox@…`, `celikpanel-ftp@…` | root'un bağlama birimleri | hayır: müşteri kodu çalıştırmazlar |

**Bu sürümden sonra henüz sınırlanmayanlar.** Açıkça: bir sitenin diliminin dışında çalışan
programları (bu sürümden önce yazılmış her program, yönetici siteyi taşıyana kadar; böyle bir
program, taşınana kadar sunucunun her yeniden başlatılmasından sonra yine), sitenin PHP'si (PHP-FPM
havuzu), zamanlanmış görevleri ve bir site hesabının açık tutabildiği kullanıcı yöneticileri
("lingering", polkitd olan Ubuntu) sitenin sınırlarıyla sınırlanmaz. PHP R3'tür; zamanlanmış
görevler ve açık kalan kullanıcı yöneticileri R2b'dir.

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
  kendi değişiklik kilidiyle** (programın her başka değişikliğinin aldığı kilit; her programın kendi
  kilidi, başka hiçbir program ya da siteyle paylaşılmaz): dosya taşımanın planladığı metin olmalıdır
  (değilse arada yapılmış bir değişikliktir: korunur, "değiştirildi"), CelikPanel'in değişmemiş metni
  `Slice=celiksite_<kimlik>.slice` alır (yeniden mühürlenir), systemd yeniden yüklenir ve çalışan birim
  bir kez yeniden başlar; **kendi başlama süresi içinde** dilimde çalışmalı ve önceden dinlediği her
  bağlantı noktasını yeniden dinlemelidir: birimin kendi `TimeoutStartUSec` değeri (bu birimler koymaz,
  yani systemd'nin 90 saniyesi), hiçbir zaman 20 saniyeden az, 300 saniyeden çok değil. Ne neyi
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
  "denenmedi" olur ve bir sonraki taşıma onlarla sürer.
- Bir birim başarısız olursa o ve sitenin ondan önce taşınan her birimi taşıma öncesi baytlarına
  geri konur, ama yalnız taşımanın kendi metninin üstüne: arada değiştirilen ya da silinen dosya
  olduğu gibi bırakılır. Diğer sitelere hiç dokunulmaz.

**Günlük** (Ajan'ın özel klasöründe `site-slice-move.json`) **her yazmadan önce** yazılır ve diske
işlenir: ilk yazmadan önce taşımanın değiştireceği her birim dosyasının taşıma öncesi baytları ve
kipi, dilim dosyasının baytları ve kipi (ya da olmadığı) ile kayıt defterindeki özeti ve her birimin
adımı; bir birim dosyası yazılmadan önce adımı "yazılıyor" olur; dilim dosyası yazılır yazılmaz yeni
baytları. Yazılamayan günlük başarısız bir adımdır: başka hiçbir şey yazılmaz ve yazılan geri konur.
Ajan başlarken yalnız adımın adına hiç güvenilmez: her birimin dosyası kaydedilen taşıma öncesi
baytlarla ve taşımanın yazmak istediği metinle karşılaştırılır:

| Birim dosyasında olan | Ajan'ın bir sonraki başlangıcında |
|---|---|
| taşıma öncesi baytlar, adım "planlandı" ya da "yazılıyor" | dokunulmaz ("denenmedi") |
| taşıma öncesi baytlar, daha sonraki bir adım | zaten geri kondu; önceden çalışıyorsa ve şimdi sistem diliminde çalışmıyorsa yeniden başlatılır ("geri kondu") |
| taşımanın metni, durdurulmuş birim | korunur: dosyası dilimi adlandırır, orada başlar ("yeniden yazıldı") |
| taşımanın metni, çalışan birim, adım "taşındı" (dilimde çalıştı ve yanıt verdi) | korunur ("taşındı") |
| taşımanın metni, çalışan birim, başka her adım | taşıma öncesi baytlar geri, daemon-reload ve hâlâ sistem diliminde çalışmıyorsa yeniden başlatma ("geri kondu") |
| başka bir şey ya da dosya yok | olduğu gibi bırakılır ("değiştirildi") |

Dilim dosyası: bundan sonra hiçbir birim dilimi adlandırmıyorsa ve dosya tam olarak taşımanın
yazdığı baytları tutuyorsa kaydedilen asıl geri gelir (önceki baytları ve kayıt defteri özeti, ya
da önceden yoksa dosya yok); yoksa kalır. Geri koyması tamamlanmayan taşıma günlüğünü korur; bir
sonraki başlangıç ya da bir sonraki taşıma onu aynı yolla bitirir (o zamana kadar yeni taşıma
reddedilir). Ajan'ın ne yaptığı sitenin bir sonraki taşımasına kadar yöneticinin site kartında
gösterilir ("son taşıma yarıda kesildi … her program tamamlandı ya da geri kondu").

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
  ne zaman olduğunu, geçerli sınırı (ulaşılan sayı: systemd'nin uyguladığı; sahibin eklemesi,
  değiştirilmiş dosya ya da henüz yazılmamış bir değişiklik başkasını koymadıkça planın değeri) ve
  planınki ise kimin yükseltebileceğini söyler: müşteriye "hizmet sağlayıcınız", bayiye "kendi
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
  uygulayamayan sunucu) kart "sınırlarının içinde kaldı" yerine bunu söyler.

**Henüz taşınacak siteler.** Şu anki sorunlar'da yalnız yönetici için bir satır vardır: kendi
programları sınırlarının dışında çalışan ya da orada başlayacak olan ve taşımanın alabileceği siteler
(sayıları ve ilk beş adı). İlk siteyi açar, bir uyarıdır (yapılacak iş) ve hiç site kalmayınca
kendiliğinden kaybolur (`panel_settings` `site_limit_outside_v1`, yalnız liste değişince yazılır).

### 8.7 Silinen sitenin dilimi

Bir site silinince CelikPanel'in değişmemiş dilim dosyası gider ve kayıt defteri onu unutur; sahibin
değiştirdiği dosya ve sahibin ekleme klasörü systemd'nin yüklemediği adlarla kenara konur
(`<dosya>.celikpanel-aside-<UTC zamanı>`) ve denetim kaydında adlandırılır (`site.limits_file_aside`).
**Bir site yaratılırken** (henüz hiçbir şeyi yok: ev klasörü ve hesabı yeni), onun kimliğine ait olup
Ajan'ın kayıt defterinin tanımadığı dilim dosyası (önceki bir kurulumda aynı kimliğe sahip bir site)
**ekleme klasörüyle birlikte** aynı biçimde kenara konur; yeni müşterinin sitesi ikisini de devralmaz;
Ajan'ın kaydında adlandırılır. Var olan bir site için sonraki bir yazma sahibin dosyasını ya da
eklemesini hiçbir zaman kenara koymaz. Kayıt defterinin tanıdığı dilim dosyası (aynı kurulumun,
silinmesi çalışmamış önceki bir sitesi) olduğu gibi bırakılır. **Okunamayan kayıt defteri** bir
hatadır, hiçbir zaman boş bir kayıt defteri değildir: o okumayla hiçbir şey yazılmaz, silinmez ya da
kenara konmaz ve çağıran bunu söyler (sınır değişikliğinin Şu anki sorunlar satırı, taşımanın yanıtı,
silmenin başarısız adımı, bir programın kendi yazması için Ajan'ın kaydı).

**Sahibin bağımsızlığı (D-022).** Dilimler yerel birimlerdir. Panel durdurulsa, kaldırılsa ya da
lisanssız kalsa da onlar ve sınırları çalışmaya devam eder. Bir sitenin sınırlarını panel olmadan
kaldırmak ya da değiştirmek için sahip bir ekleme dosyası yazar
(`/etc/systemd/system/celiksite_<kimlik>.slice.d/owner.conf`, örneğin `[Slice]` ve
`MemoryMax=infinity` ile) ve `systemctl daemon-reload` çalıştırır (systemd 257'de yeni değer çalışan
programlara onlardan biri başladığında ya da `systemctl set-property --runtime` ile ulaşır);
CelikPanel onu hiçbir zaman yazmaz ya da silmez.
