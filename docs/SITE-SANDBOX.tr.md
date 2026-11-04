# Site koruması (sandbox)

*alpha.119 · Kaynakta var, bileşen testlerinden geçti; WSL Debian 13 (systemd 257) üzerinde bir kez kısa deneme yapıldı. Desteklenen beş sistemde kaçış matrisi laboratuvarda bekliyor.*

İngilizcesi: [SITE-SANDBOX.md](SITE-SANDBOX.md).

Müşterinin her programı tek bir systemd korumasında çalışır: yapay zekâ ve otomasyon uygulamaları
(`celiksvc-<id>` ve kurulum adımı `celiksvcinst-<id>`), Python uygulamaları ve yüklenen
programlar (`celikrt-<id>`). Her program rastgele kod sayılır. Yüklenen bir Go programı Python
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
- **Yüklenen programlar:** yazılabilir belleği çalıştıramaz. Yalnız kendi klasöründen ve paylaşılan kitaplıklardan program çalıştırabilir: `/bin/sh`, `python3` ve `ld.so /usr/bin/python3` başarısız olur. Bu ek bir derinlik katmanıdır; asıl sınır değildir.
- **Hizmet uygulamaları:** alpha.118'deki tam `/proc` görünümünü korur. Node.js `/proc/cpuinfo` dosyasını okur ve bu görünüm n8n ile Open WebUI üzerinde henüz ölçülmedi.

**systemd 257 ve sonrasında:** program kendi ad alanında 1 numaralı süreçtir (`PrivatePIDs`) ve cgroup ağacı sıkı biçimde salt okunurdur.

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
| SMTP çıkış engeli | nft `socket cgroupv2` + cgroup v2 | ölçülecek (nft 1.0.2) | var (beklenen) | var (beklenen) | var (beklenen) | var (beklenen) |

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
çalışırlar ve yönetici sayfası bu düzeyi gösterir.

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
`ExecStartPre=+… sandbox-helper egress-add --unit=<birim>` tabloyu yükler ve birimin cgroup'unu
kümeye ekler; `ExecStopPost=-+… egress-remove` durduktan sonra çıkarır. nft boş bir ortamla
çalışır. nft tabloyu yükleyemezse ya da birimin cgroup'u
`/system.slice/<birim>` değilse birim başlamaz; engel kapalı kalacağına program hiç çalışmaz.

`celikpanel-egress-guard.timer` 30 saniyede bir çalışır. Biri `nft flush ruleset`
çalıştırdıysa ya da `nftables.service` yeniden başlatıldıysa tabloyu yeniden yükler ve çalışan
korumalı birimleri yeniden ekler. Bu arada engel en çok 30 saniye yok olabilir. Önceki bir
sürümün yazdığı birim kümeye asla sessizce eklenmez.

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

## 7. Kaçış denemeleri

Deneme takımı `deploy/e2e/sandbox-escape/` içindedir. Statik bir Go yoklaması ve bir Python
yoklaması, korumalı birimler altında matrisin 1–18. maddelerini dener. Her deneme başarısız
olursa takım PASS bildirir. Desteklenen beş sistemde laboratuvar turu hâlâ açıktır.
