# Yüklenen programlar ve Python uygulamaları (çalışma ortamı uygulamaları)

*alpha.119 · Kaynakta var, bileşen testlerinden geçti. Desteklenen beş sistemde laboratuvar turu
bekliyor.*

İngilizcesi: [RUNTIME-APPS.md](RUNTIME-APPS.md) · Koruma: [SITE-SANDBOX.tr.md](SITE-SANDBOX.tr.md) ·
Haklar: [PLANS-AND-RIGHTS.tr.md §4.8–4.9](PLANS-AND-RIGHTS.tr.md)

Müşteri kendi derlenmiş programını (Go, Rust ya da bu sunucunun işlemcisi için derlenmiş başka
bir program) sitesi olarak çalıştırabilir. Program sitenin kendi hesabıyla, site koruması
(sandbox) içinde, yalnız sunucunun kendisinin erişebildiği tek bir portta çalışır. Ziyaretçiler
ona yalnız HTTPS ile, sitenin nginx'i üzerinden ulaşır.

Müşteri aynı altyapı üzerinde bir Python web uygulaması (Django, Flask, FastAPI) da
çalıştırabilir; uygulama dağıtımın Python'uyla, PyPI'dan kendi ortamına kurulur (§18).

CelikPanel yüklenen her programı rastgele kod sayar. Yüklenen bir Go programı Python
başlatabilir. Bu yüzden güvenlik sınırı izin verilen diller değil, korumanın kendisidir.

Bu belge alpha.119'daki kodun yaptığını anlatır. Özelliğin desteklenen her sistemde denendiğini
söylemez; laboratuvar turu daha yapılacak (§17).

---

## 1. Türler

| Tür | alpha.119'daki durumu |
|---|---|
| `program` | Hazır: yüklenen çalıştırılabilir dosya. |
| `python` | Hazır: dağıtımın Python'uyla kurulan sanal ortamda çalışan bir WSGI ya da ASGI web uygulaması; kendi izni (`python_apps`) ve dağıtımın her Python sürümü için ayrı sunumu (`component:python:<sürüm>`) vardır. Bkz. §18. |

Bir alan adında en çok bir program olabilir.

## 2. Dosyalar nereye gider

```
<site klasörü>/app/
  bin/     yüklenen program (başlangıç dosyası ve gerek duyduğu her şey)
  data/    programın kendi verisi; HOME klasörü
  config/  programın kendi ayar dosyaları
```

`<site klasörü>` şudur: `/var/www/celikpanel/subscriptions/<abonelik>/sites/<alan adı no>`.
`app` klasörü `public_html`'in içinde değil, yanındadır. Dosyaları sitenin hesabınındır,
aboneliğin diskine sayılır ve nginx onları hiçbir zaman dışarıya sunmaz.

Müşteri programı panelin dosya yöneticisiyle ya da SFTP ile `app/bin` içine yükler. SFTP
oturumu site klasöründe açılır; hedef `app/bin/` olur. `data/` ve `config/` klasörlerini
CelikPanel yayımlama sırasında sitenin hesabıyla kendisi açar (`mkdir` kabuksuz, doğrudan
çalışır). root sitenin klasörüne hiçbir şey yazmaz.

Ortam değişkenleri dosyası bu klasörde **değildir** (§5).

## 3. Başlangıç dosyası

Başlangıç dosyası systemd'nin çalıştırdığı dosyadır. Müşteri onu `app/bin`'e göre adlandırır:
örneğin `server` ya da `release/api`. Baştaki `bin/` atılır. Ad harf, rakam, `.`, `_` ve `-`
içerir; `/` ile ayrılmış en çok dört parçadır; üst klasörü gösteremez.

Ajan dosyayı her yayımlamadan önce ve her durum okumasında denetler. Dosyayı yalnız **okur**,
asla çalıştırmaz. `app` klasörünü site klasörünün altında, başlangıç dosyasını da `app`
klasörünün altında `openat2` ile açar (`RESOLVE_BENEATH | RESOLVE_NO_SYMLINKS |
RESOLVE_NO_MAGICLINKS | RESOLVE_NO_XDEV`, `O_NOFOLLOW`, `O_NONBLOCK`). Yolun herhangi bir
yerindeki bağlantı (link) reddedilir. Başlangıç dosyası yerine konmuş bir FIFO ajanı
bekletemez.

| Kural | Ret kodu | Ne yapılır |
|---|---|---|
| `app` klasörü ve barındırma ağacından ona giden her klasör gerçek bir klasördür (bağlantı ya da bağlama noktası değildir) | `app_folder_not_real` | Bağlantıyı dosya yöneticisi veya SFTP ile gerçek klasörle değiştirin. |
| `app` klasörü ve site klasörü sitenin hesabınındır; site klasörünün üstündekiler root'undur ve başkaları yazamaz | `app_folder_not_owned` | Yönetici sahibini düzeltir. |
| Dosya vardır | `entry_missing` | `app/bin/<dosya>` olarak yükleyin. |
| Bağlantı değildir | `entry_link` | Programın kendisini yükleyin. |
| Sıradan bir dosyadır | `entry_not_regular` | Klasör ya da özel dosya değil, program dosyasını yükleyin. |
| Sitenin hesabınındır | `entry_not_owned` | Dosya yöneticisiyle ya da SFTP ile yeniden yükleyin. |
| setuid ya da setgid biti yoktur | `entry_setid` | `chmod u-s,g-s` |
| Başkaları yazamaz | `entry_writable` | `chmod o-w` |
| Sahibi çalıştırabilir | `entry_not_executable` | `chmod u+x` (dosya yöneticisi ya da SFTP programı bunu yapabilir) |
| Bu sunucunun işlemcisi için ELF programıdır (x86-64 ya da AArch64) | `entry_wrong_arch` | Bu sunucu için yeniden derleyin; örneğin `GOARCH=amd64` ya da `GOARCH=arm64`. |
| Ya da yorumlayıcısı `app` klasörünün içinde olan bir `#!` betiğidir | `entry_interpreter` | Derlenmiş program yükleyin ya da yorumlayıcıyı `app/` içine koyun. |
| Başka her şey | `entry_unknown_format` | Derlenmiş programı yükleyin. |

**Sistem yorumlayıcıları neden reddedilir.** `/bin/sh` ya da `python3` başlatan bir betik daha
çalışmadan reddedilir. Program türünün koruması, programın yalnız kendi klasöründen ve
paylaşılan kitaplıklardan bir şey çalıştırmasına izin verir (`NoExecPaths=/`,
`ExecPaths=<app klasörü> /usr/lib /usr/lib64 /lib /lib64`). Böyle bir betik zaten hiç
çalışamazdı; panel bunu başlatma hatası göstermek yerine en baştan söyler.

## 4. Argümanlar

Argümanlar bir komut satırı değil, tek tek parçalardan oluşan bir listedir. Her parça
`ExecStart`'ın bir sözcüğü olur. Kabuk yoktur.

- En çok 32 parça; her biri 1–256 karakter.
- İzin verilen karakterler: harf, rakam ve `. _ @ : / + = , -`.
- Tek yer tutucu `{port}`'tur. Uygulamanın portuyla değiştirilir; örneğin
  `-addr=127.0.0.1:{port}`.

**`$`, `%`, boşluk, tırnak ve ters bölü neden reddedilir.** systemd bir birimin komut
satırındaki `$` ve `%` işaretlerini açar; boşluk, tırnak ve ters bölüde sözcükleri böler ya da
tırnakları kaldırır. Her durumda doğru olan tek bir kaçış yolu yoktur. Bu yüzden bunlardan
birini içeren parça kaçışlanmaz, reddedilir. Son bir güvence olarak yazılan birim yeniden
denetlenir: hiçbir satırda `$` yoktur, tek `%` `CPUQuota`'nınkidir ve `ExecStart` asla bir
kabuk başlatmaz, ayrıcalık öneki (`+`, `!`, `@`, `-`) taşımaz.

Sağlık yolu (varsayılan `/`) `/` ile başlar; yalnız harf, rakam ve `. _ ~ / -` içerir; `..` ve
sorgu içermez.

## 5. Ortam değişkenleri

Müşteri en çok 64 değer girebilir. Her değer tek satırlık yazdırılabilir metindir, 1–4096 bayt.
Anahtar büyük harf, rakam ve `_` içerir, harfle başlar, en çok 64 karakterdir.

**Reddedilen anahtarlar:**

| Anahtarlar | Neden |
|---|---|
| `PATH`, `HOME`, `PORT`, `HOST` | CelikPanel bunları birimde kendisi verir. Program nerede dinleyeceğini `PORT` ve `HOST`'tan okur. |
| `LD_*` | Programın içine başka kod yükletir. |
| `PYTHONPATH`, `PYTHONHOME`, `PYTHONSTARTUP` | Aynısını Python için yapar. |
| `USER`, `LOGNAME`, `SHELL` | systemd hesaba göre verir. |
| `NOTIFY_SOCKET`, `LISTEN_FDS`, `LISTEN_PID`, `LISTEN_FDNAMES` | systemd'nin kendi iletişimidir. |
| `CELIKPANEL_*`, `SYSTEMD_*` | CelikPanel'e ya da systemd'ye aittir. |

**Açık ve gizli değerler.** Açık değeri Panel saklar ve yeniden gösterir. Gizli değer yalnız
yazılır: Panel yalnız anahtarını saklar. Değer sunucuya bir kez gider, ortam dosyasına yazılır
ve bir daha gösterilmez. Denetim kaydı değişen anahtarları yazar, değeri asla yazmaz. Gizli
değeri olduğu gibi bırakmak için anahtarı değersiz gönderin. Yeni bir gizli değerin ilk
seferde en az 8 karakterlik bir değeri olmalıdır; böylece günlükte sıradan sözcükler
gizlenmeden o değer gizlenebilir.

Her kayıt listenin **tamamını** gönderir. Listede olmayan anahtar silinir. Program yayımlanmış
ve çalışıyorsa yeni değerleri okusun diye yeniden başlatılır (`systemctl try-restart`).

**Ortam dosyası** şudur: `/etc/celikpanel-runtime/celikrt-<no>.service.env`. root'undur, izni
0600'dür ve izni 0700 olan bir root klasöründedir. Bilerek sitenin klasörüne konmaz. systemd
`EnvironmentFile=` dosyasını koruma başlamadan önce root olarak okur. Sitenin hesabının
yönettiği bir dosya, root'un okuyabildiği bir dosyaya (örneğin başka bir hizmetin anahtarına)
bağlantıyla değiştirilebilirdi ve systemd onu okurdu. Birim dosyayı başında `-` ile anar;
program dosya olmadan da başlar.

Dosya panelden her kayıtta baştan yazılır, ama sahibin değişikliğinin üzerine asla yazılmaz:
dosyada CelikPanel'in yazmadığı ve kaydın da anmadığı bir anahtar varsa hiçbir şey değişmez,
kayıt `env_edited` ile reddedilir ve o anahtarlar adıyla söylenir. Korumak için onları
Programlar sekmesinde ekleyin ya da dosyadan silin (D-022).

**Günlük sunucuda gizlenir.** Panel bir günlük satırını göstermeden önce ajan, ortam
dosyasındaki her gizli değeri (gizli değerler en az 8 karakterdir) ve anahtar, jeton, parola,
bearer jetonu, bcrypt özeti ya da adresteki `kullanıcı:parola@` gibi görünen her şeyi gizler.

## 6. Port, ağ ve birim

**Port.** Her uygulama 30000–34999 arasından boştaki en küçük portu alır. Hizmet uygulamaları
21000–29999'u kullanır; iki aralık hiç çakışmaz. Birim `PORT=<port>` ve `HOST=127.0.0.1`
verir. Program `127.0.0.1:$PORT` üzerinde (ya da `{port}` argümanında) dinlemelidir. Koruma
yalnız bu porta bağlanmasına izin verir. Yayımlama sırasında başka bir adreste dinleyen
program dinleme denetiminde görülür, durdurulur ve yayımlanmaz. Sonraki okumalar bunu
Programlar sekmesinde gösterir (`listens_elsewhere`).

**Yayımlama.** Başarılı yayımlamadan sonra sitenin türü `runtime_app` olur. nginx
`127.0.0.1:<port>`'a iletir ve HTTPS'i zorunlu tutar. Etkin sertifika yoksa alan adı programı
asla düz HTTP ile sunmaz; site o zaman kendi klasörünü sunar.

**Birim** `/etc/systemd/system/celikrt-<no>.service` dosyasıdır; `<no>` uygulamanın
numarasıdır ve Programlar sekmesinde görünür. İçinde şunlar vardır:

- CelikPanel'in yazma başlığı (`# celikpanel-render v2 sha256=…`, D-031), CelikPanel olmadan
  da çalışan komutları anan açıklama satırları, koruma kuşağı işareti ve
  `# celikpanel-runtime-kind: program`.
- `[Unit]`: `After=`/`Wants=network-online.target`, `StartLimitIntervalSec=300`,
  `StartLimitBurst=5`.
- `[Service]`: `Type=simple`; `User=`/`Group=` sitenin hesabı; `WorkingDirectory=<app klasörü>`
  (koruma içinde çözülür; orada bu yol bağlanan klasördür);
  `EnvironmentFile=-<ortam dosyası>`; `HOME=<app>/data`, `PATH=<app>/bin`, `PORT`, `HOST`;
  `ExecStart=<app>/bin/<dosya> <argümanlar>`; `Restart=on-failure`, `RestartSec=5`,
  `TimeoutStopSec=30`; ve koruma ayarları (§7).
- Bağlama birimi `celikpanel-sandbox@celikrt-<id>.service` için `Requires=` ve `After=`
  ([SITE-SANDBOX.tr.md §1a](SITE-SANDBOX.tr.md)): her başlatmadan önce app klasörünü denetler ve
  onu `/run/celikpanel-sandbox/celikrt-<id>.service/app` noktasına bağlar. `BindPaths=` korumaya
  yalnız root'a ait bu yolu, klasörün olağan yolunda bağlar.
- Çıkış engelini uygulayan sunucuda `ExecStartPre=+… sandbox-helper egress-add` birimin
  cgroup'unu `smtp_blocked` kümesine ekler, `ExecStopPost=-+… egress-remove` çıkarır
  ([SITE-SANDBOX.tr.md §3](SITE-SANDBOX.tr.md)).
- `[Install]`: `WantedBy=multi-user.target`.

5 dakikada 5 kez duran program bir daha başlatılmaz (`restart_loop`). Başlat ya da yeni bir
yayımlama sayacı sıfırlar (`systemctl reset-failed`) ve gerçekten yeniden dener.

**Sahibin değişiklikleri (D-031).** Sahibin kendi ayarları
`/etc/systemd/system/celikrt-<no>.service.d/*.conf` içine gider. CelikPanel bu klasöre asla
yazmaz, onu silmez; Programlar sekmesi içindeki dosyaları listeler. Sahip birim dosyasının
kendisini değiştirirse CelikPanel değişikliği korur. Yayımlama `unit_edited` ile durur ve
CelikPanel kendi metnini yanına `celikrt-<no>.service.celikpanel-pending` adıyla koyar.
Ayarlarınızı ek ayar dosyasına taşıyın ya da CelikPanel'in metnini geri koyun, sonra yeniden
yayımlayın.

Koruma kanıtı (§8) ek ayar dosyaları dahil yüklenmiş birimi okur. Karşılaştırdığı
özelliklerden biri profille uyuşmazsa yayımlamayı reddeder. Kanıtın hangi özellikleri
karşılaştırdığı §17'dedir.

## 7. Koruma

Her program ortak site korumasında çalışır. Ayrıntısı [SITE-SANDBOX.tr.md](SITE-SANDBOX.tr.md)
belgesindedir: sitenin kendi hesabı, hiçbir ayrıcalık yok; içine yalnız `app` klasörü bağlanmış
boş ve salt okunur bir barındırma ağacı; gizlenmiş ajan, PHP-FPM ve CelikPanel yolları; sistem
çağrısı süzgeci; ham soket yok; yalnız kendi döngü (loopback) portuna bağlanma; üst veri
engeli; SMTP çıkış engeli; sunumun bellek, işlemci, süreç ve dosya sınırları.

Program türü bunlara üç ayar ekler:

- `MemoryDenyWriteExecute=yes`: yazılabilir bellek çalıştırılabilir yapılamaz.
- `NoExecPaths=/` ve `ExecPaths=<app klasörü> /usr/lib /usr/lib64 /lib /lib64`: program yalnız
  kendi klasöründen ve paylaşılan kitaplıklardan bir şey çalıştırabilir. `/bin/sh`, `python3`
  ve `ld.so /usr/bin/python3` başarısız olur.
- `ProcSubset=pid`: `/proc` yalnız süreçleri gösterir (Python uygulamalarında olduğu gibi).

Bunlar ek derinlik katmanlarıdır; asıl sınır değildir.

## 8. Yayımlama

Yayımlama tek bir kalıcı işlemdir (D-025). Adımları bu sırayla çalışır:

| Adım | Ne olur | Hata ve anlamı |
|---|---|---|
| `validate` | Panel güvenlik beklemesine, sunuma ve askıya almaya yeniden bakar. Ajan sitenin hesabını, sunucunun koruma düzeyini kabul edilen düzeyle ve başlangıç dosyasını (§3) denetler. Hiçbir şey değişmez. | `held`, `not_offered`, `suspended`, `sandbox_reduced`, bir başlangıç dosyası kodu, `site_account`: eksik önkoşul. `entry_unreadable` ve `agent_unreachable`: bilinmeyen sonuç. |
| `render` | Ajan `data/` ve `config/` klasörlerini sitenin hesabıyla açar, birimi hazırlar, çıkış tablosunu denetler, birimi D-031 kuralıyla yazar ve systemd'yi yeniden yükler. Hiçbir şey başlatmaz. | `unit_edited` (önkoşul: sahip birimi değiştirdi). `site_folder` ve `unit_render` (doğrulanmış hata). `unit_write`, `unit_dir`, `sandbox_helper` ve `sandbox_egress` (bilinmeyen). `build_mismatch` (önkoşul: Panel ve ajan farklı yapımlardan; güncelleme bitince düzelir). |
| `restart` | `systemctl reset-failed`, `enable` ve `restart`. | `systemctl` (doğrulanmış hata), `unit_missing` (önkoşul). |
| `proof` | Panel en çok 2 dakika, **bu birime ait** bir sürecin `127.0.0.1:<port>`'ta dinlemesini bekler; sonra yüklenen korumayı profiliyle karşılaştırır ([SITE-SANDBOX.tr.md §5](SITE-SANDBOX.tr.md)). | `listens_elsewhere`, `port_taken`, `oom`, `restart_loop`, `sandbox_proof`: program durdurulur, açılışta başlaması kapatılır, hiçbir şey yayımlanmaz. `start_failed`: birim başlarken çöktü. `not_listening`: çalıştı ama zamanında dinlemedi. |
| `health` | En çok 90 saniye, 3 saniyede bir `http://127.0.0.1:<port><sağlık yolu>` adresine bir GET; `Host` sitenin adı, `X-Forwarded-Proto: https`, yönlendirmeler izlenmez. 500'ün altındaki her yanıt sağlıklı sayılır. | `health_failing`: günlüğü ve yanıtları okunabilsin diye program çalışmaya devam eder, ama site ona geçirilmez. |
| `publish` | Site, uygulamanın portu ve zorunlu HTTPS ile `runtime_app` türüne geçer; nginx dosyası yeniden yazılır. Uygulama `running` ve `deployed` olur. | `publish` (bilinmeyen): program çalışıyor ama alan adı henüz ona geçirilemedi. Yeniden yayımlayın. |

**Yeniden yayımlamada önceki yüklemeye geri dönüş yoktur.** Dosyalar müşterinindir; CelikPanel
kopyasını tutmaz. Zaten yayımlanmış bir uygulama yeniden yayımlanırken site hâlâ o porta
bakar. Kanıt başarısız olursa program durdurulur ve site bir sonraki başarılı yayımlamaya
kadar hata verir. Sağlık denetimi başarısız olursa yeni sürüm yanıt vermeye devam eder.

Her hata sınıfını (doğrulanmış hata, eksik önkoşul ya da bilinmeyen sonuç), neden kodunu ve
sunucunun gizli bilgileri ayıklanmış kendi sözlerini korur. Ekran bunları kimin ne yapacağıyla
birlikte gösterir (§12).

## 9. Günlük işler

| İş | Ne yapar | Ne zaman reddedilir |
|---|---|---|
| Ayarlar (`PUT …/settings`) | Başlangıç dosyasını, argümanları ve sağlık yolunu değiştirir. Bir sonraki yayımlamada geçerli olur. | Yeni kullanımın her kapısında: haklar, bekleme, askıya alma, sunucu düzeyi, dolu disk (§10, §11). |
| Başlat / Yeniden başlat | `reset-failed`, `enable`, `restart`. | Henüz yayımlanmadıysa; beklemedeyse; askıdaysa; sunucu düzeyi kabul edilmediyse; başka bir işlem sürüyorsa (`busy`). |
| Durdur | `stop` ve `disable`; program yeniden açılışta da durmuş kalır. systemd'nin tanımadığı birimde (hiç yayımlanmamış ya da birimi silinmiş) çalışan bir şey yoktur: durdurma `systemctl` çağrılmadan başarılı olur. Hiç yayımlanmamış uygulamada yanıt `reason: not_deployed` ile “durdurulacak bir şey yok; başlatmak için yayına alın” der, durumu değişmez. | Haklar yüzünden asla. Yalnız uygulamanın yayımlaması ya da kaldırılması sürerken (`busy`). |
| Ortam değişkenleri | §5. | Yalnız bir işlem sürerken (`busy`). |
| Günlük | Son satırlar (varsayılan 200, en çok 500), sunucuda ayıklanmış. | Haklar yüzünden asla. Günlük okunamazsa `logs_unavailable`. |
| Kaldır | Kalıcı işlem: `unpublish` → `remove` → `done`. | Haklar yüzünden asla. Yalnız bir işlem sürerken (`busy`). |

**Kaldırma.** `unpublish` siteyi programdan önceki türüne ve portuna döndürür, nginx dosyasını
yeniden yazar. `remove` birimi durdurur, açılışta başlamasını kapatır, sıfırlar; sonra birim
dosyasını ve ortam dosyasını siler ve systemd'yi yeniden yükler. Sahip birimi değiştirmişse
bir kopyası `celikrt-<no>.service.celikpanel-removed-<UTC zamanı>` adıyla saklanır. Sahibin ek
ayar klasörüne dokunulmaz. Ajan sonunda birimin artık çalışmadığını doğrular (`verify`).

**`app` klasörü korunur**; müşteri "dosyaları da sil" kutusunu işaretleyip alan adını yazmadıkça
silinmez (yazmazsa `confirm_files`). Silme de sitenin kendi hesabıyla yapılır.

Bir alan adında program varken sitenin türü değiştirilemez ve alan adı silinemez
(`app_created`). Önce programı kaldırın. Ek kullanıcılar (ekip üyeleri) bu özelliği kullanamaz.

## 10. Haklar (D-042)

- **Yüklenen programlar** izni (`program_apps`) ve **`runtime:program`** sunumu.
- **Sunulana kadar kapalı.** Yönetici sunumu Eklentiler sayfasında risk özetini okuduktan sonra
  açar: programın neler yapabildiği, korumanın neyi engellediği, ağ ve geriye kalan riskler.
  Yönetici bellek (64–65536 MB, varsayılan 512), işlemci (yüzde 10–1600, varsayılan 100) ve
  süreç (16–8192, varsayılan 256) sınırlarını da belirler.
- **Sunum özetin parmak izini kaydeder.** Parmak izi türü, koruma kuşağını, varsayılan
  sınırları ve risk listelerini kapsar. Bunlardan birini değiştiren bir sürüm, yönetici yeniden
  okuyup sunana kadar sunumu kapatır (`offer_stale`). Eski parmak iziyle gelen istek
  `risk_changed` ile reddedilir.
- **Eksik düzey.** Tam korumayı uygulamayan sunucuda sunum, yönetici her eksiği adıyla tek tek
  kabul ederse olur (`systemd_old`, `systemd_unknown`, `bpf_missing`, `cgroup_v2_missing`,
  `nft_cgroup_missing`). Eksik düzey asla sessizce geçmez. Sunucuda sonradan kabul edilmemiş bir
  eksik görünürse yayımlama ve başlatma, yönetici onu kabul edene ya da sunucuyu düzeltene
  kadar reddedilir (`sandbox_reduced`). Ajan da bunu kendisi yeniden denetler.
- **Zincir yalnız daraltır.** Sınırsız planlar sunulan `runtime:program`'ı kendiliğinden alır.
  Bayi ya da plan yalnız dışarıda bırakabilir. Sunucu oluşturmada, ayarlarda ve yayımlamada her
  katmana bakar.
- **Durdurma, günlük, ortam değişkenleri ve kaldırma haklar yüzünden asla reddedilmez** (İ5);
  daralan bir hak çalışan programı ortada bırakmaz. Sunumu geri çekmek yalnız yeni kullanımı
  durdurur. Çalışan çalışmaya devam eder; yayımlanmış program yine başlatılabilir.
- Sınırlar her birime yayımlama sırasında yazılır. Yeni sınırlar bir uygulamaya bir sonraki
  yayımlamasında geçer.

## 11. Güvenlik beklemesi, askıya alma ve dolu disk

- **Güvenlik beklemesi** (yönetici, 1–500 karakterlik bir gerekçeyle): yayımlanmış her program
  hemen durdurulur ve açılışta başlaması kapatılır. Yanıt durdurulan ve durdurulamayan
  programları nedeniyle birlikte listeler. Bekleme kalkana kadar oluşturma, ayarlar, yayımlama,
  başlatma ve yeniden başlatma reddedilir. Beklemeyi kaldırmak hiçbir şeyi başlatmaz; her
  müşteri kendi programını yeniden başlatır. Dosyalar asla silinmez. Süren bir yayımlama güvenlik
  beklemesine, sunuma ve askıya almaya `validate` adımında, program başlatılmadan önce ve site
  ona çevrilmeden önce yeniden bakar ve orada `held` ile durur (başlattığı programı durdurur).
  Bekleme ve askıya alma, hâlâ süren ilk yayımlamayı da kapsar.
- **Askıya alma** aboneliğin yayımlanmış programlarını durdurur ve açılışta başlamalarını
  kapatır; programlar yeniden açılışta ve panel olmadan da durmuş kalır. Geri açma yalnız
  önceden çalışanları başlatır (karşılaştırarak, D-022). Sunumu beklemede olan program geri
  açılışta durmuş kalır.
- **Dolu disk** (D-044): abonelik diski yüzde 100'deyken oluşturma, ayarlar ve yayımlama
  reddedilir. Çalışan program bunun için durdurulmaz.

## 12. Ekranın söyledikleri (D-024)

Her durum nedeni, kimin işlem yapacağını, sonraki adımı ve işin nasıl süreceğini söyler.
Durumlar en çok engelleyenden başlayarak sıralanır. **Bilinmeyen** asla hata olarak
gösterilmez.

| Durum | Sınıf | Neden | Kim | Sonraki adım | Nasıl sürer |
|---|---|---|---|---|---|
| `agent_unreachable` | bilinmeyen | Sunucunun ajanı durum okumasına yanıt vermedi; programın gerçek durumu bilinmiyor. | Önce kimse; sürerse yönetici | Bir dakika sonra yenileyin. Yönetici `systemctl status celikpanel-agent` ile bakar. | Sonraki okuma gerçek durumu gösterir. Program systemd altında çalışmayı sürdürür. |
| `suspended` | önkoşul | Abonelik askıda; program durduruldu. | Yönetici ya da bayi | Aboneliği geri açın. | Geri açma, önceden çalışıyorsa onu yeniden başlatır. |
| `held` | önkoşul | Yönetici yüklenen programları güvenlik beklemesine aldı. | Yönetici | Uyarı çözülünce beklemeyi kaldırın. | Müşteri Başlat'a basar ya da yayımlar. Beklemeyi kaldırmak hiçbir şeyi başlatmaz. |
| `not_offered` | önkoşul | Sunum ya da izin bu hesaba açık değil (sunulmadı, geri çekildi, özeti değişti ya da plan dışarıda bıraktı). | Ekranın adını verdiği hak veren | Sunmasını isteyin. | Oluşturma, ayarlar ve yayımlama yeniden açılır. Çalışan program bu arada çalışır. |
| `sandbox_reduced` | önkoşul | Bu sunucu tam korumayı uygulamıyor ve adı konan eksikler kabul edilmedi. | Yönetici | Sunucuyu düzeltin (systemd, BPF, cgroup v2, nft) ya da eksikleri Eklentiler sayfasında kabul edin. | Yayımlama ve başlatma yeniden çalışır. |
| `no_certificate` | önkoşul | Alan adının sertifikası yok; program yalnız HTTPS ile sunulur. | Müşteri | SSL sayfasından ücretsiz sertifika alın. | Yayımlayın. |
| `disk_full` | önkoşul | Aboneliğin diski dolu. | Müşteri; daha büyük sınır için sağlayıcı | Yer açın ya da sınırı büyütün. | Oluşturma ve yayımlama yeniden kabul edilir. |
| `entry_missing` | önkoşul | `app/bin/<dosya>` sunucuda yok. | Müşteri | Dosya yöneticisiyle ya da SFTP ile yükleyin. | Yayımlayın. |
| `entry_not_executable` | önkoşul | Başlangıç dosyası çalıştırılamıyor. | Müşteri | `chmod u+x`. | Yayımlayın. |
| `entry_link` | önkoşul | Dosya ya da yolundaki bir klasör bağlantı. | Müşteri | Programın kendisini yükleyin. | Yayımlayın. |
| `entry_not_regular` | önkoşul | Dosya bir klasör ya da özel dosya. | Müşteri | Program dosyasını yükleyin. | Yayımlayın. |
| `entry_not_owned` | önkoşul | Dosya başka bir hesabın. | Müşteri; root'unsa yönetici | Dosya yöneticisiyle ya da SFTP ile yeniden yükleyin. | Yayımlayın. |
| `entry_setid` | önkoşul | setuid ya da setgid biti var. | Müşteri | `chmod u-s,g-s`. | Yayımlayın. |
| `entry_writable` | önkoşul | Sunucudaki herkes dosyayı değiştirebilir. | Müşteri | `chmod o-w`. | Yayımlayın. |
| `entry_unknown_format` | önkoşul | Ne ELF ne `#!`. | Müşteri | Derlenmiş programı yükleyin. | Yayımlayın. |
| `entry_wrong_arch` | önkoşul | Başka bir işlemci için derlenmiş. | Müşteri | Bu sunucunun işlemcisi için derleyin. | Yayımlayın. |
| `entry_interpreter` | önkoşul | `#!` betiğinin yorumlayıcısı `app` klasörünün dışında. | Müşteri | Derlenmiş program yükleyin. | Yayımlayın. |
| `app_folder_not_real` | önkoşul | `app` ya da yolundaki bir klasör bağlantı veya bağlama noktası; koruma yardımcısı programı başlatmaz. | Müşteri | Bağlantıyı gerçek klasörle değiştirin. | Yayımlayın. |
| `app_folder_not_owned` | önkoşul | `app` sitenin kendi klasörü değil ya da site klasörünün üstündeki bir klasör yalnız root'un değil. | Yönetici | Sahibini düzeltin. | Yayımlayın. |
| `restart_loop` | doğrulanmış hata | Program 5 dakikada 5 kez durdu; systemd vazgeçti. | Müşteri | Günlüğü okuyun, programı düzeltin, yeniden yükleyin. | Başlatın ya da yayımlayın (sayaç sıfırlanır). |
| `oom` | doğrulanmış hata | Sınırından fazla bellek kullandı; çekirdek durdurdu. | Müşteri; daha yüksek sınır için yönetici | Daha az bellek kullanın ya da sunumun sınırını büyütün. | Başlatın (yeni sınır yayımlama ister). |
| `listens_elsewhere` | doğrulanmış hata | 127.0.0.1'in dışında da dinledi; durduruldu. | Müşteri | Ortamdaki `HOST:PORT` üzerinde dinleyin. | Yayımlayın. |
| `port_taken` | doğrulanmış hata | Uygulamanın portunu başka bir süreç tutuyor. | Yönetici | Süreci bulun (`ss -tlnp 'sport = :<port>'`) ve durdurun. | Yayımlayın. |
| `not_listening` | doğrulanmış hata | Çalışıyor ama `127.0.0.1:<port>`'ta dinlemiyor. | Müşteri | `PORT` ve `HOST`'u okuyun ya da `{port}` kullanın. | Yayımlayın. |
| `health_failing` | doğrulanmış hata | Sağlık yolu 500 ve üstüyle yanıt verdi ya da hiç yanıt vermedi. | Müşteri | Programı düzeltin ya da başka bir sağlık yolu seçin. | Yayımlayın. |

Sağlık okuması yalnız yayımlama sırasında ve kişi taze okuma istediğinde (`?health=1`)
yapılır; ekranın durum sorgusu onu asla çalıştırmaz.

## 13. CelikPanel olmadan (D-022)

Birim, ortam dosyası ve `app` klasörü düz systemd ve dosya sistemi durumudur. Panel durursa,
lisansını kaybederse ya da kaldırılırsa da çalışır. Sunucuda root olarak:

```sh
systemctl status celikrt-<no>          # çalışıyor mu
systemctl stop celikrt-<no>            # şimdi durdur
systemctl disable celikrt-<no>         # açılışta başlamasın
systemctl start celikrt-<no>           # yeniden başlat
journalctl -u celikrt-<no> -n 100      # günlüğü
```

Programı elle kaldırmak için:

```sh
systemctl disable --now celikrt-<no>.service
rm /etc/systemd/system/celikrt-<no>.service
rm /etc/celikpanel-runtime/celikrt-<no>.service.env
rm -r /etc/systemd/system/celikrt-<no>.service.d   # yalnız ek ayar dosyası yazdıysanız
systemctl daemon-reload
```

nginx, sitenin türü değişene kadar alan adını o porta iletmeyi sürdürür. Panel hâlâ kuruluysa
programı Programlar sekmesinden kaldırmak site türünü geri alır (birim zaten silinmiş olsa da
çalışır). Panel yoksa sahip sitenin nginx dosyasını düzenler. `app` klasörü biri silene kadar
kalır.

SMTP çıkış engeli panel olmadan da çalışır ([SITE-SANDBOX.tr.md §4](SITE-SANDBOX.tr.md)).

## 14. Kapsam dışı

- 2525 numaralı port ve web posta API'leri üzerinden gönderilen e-posta (yalnız 25, 465 ve 587
  kapalıdır).
- Programın kendi dış trafiği aboneliğin trafiğine sayılmaz.
- CelikPanel programı kaynak koddan derlemez. Müşteri hazır programı yükler.

## 15. Denetim kaydı

İşlemler denetim kaydına şu adlarla yazılır: `runtime.create`, `runtime.settings`,
`runtime.deploy`, `runtime.start`, `runtime.stop`, `runtime.restart`, `runtime.env` (yalnız
anahtarlar), `runtime.remove`; yönetici için `runtime.offer`, `runtime.withdraw`,
`runtime.hold` ve `runtime.release`.

## 16. Dayanıklılık (D-025)

Bu bölüm [RESILIENCE-CONTRACT.tr.md](RESILIENCE-CONTRACT.tr.md) belgesinin 1. (sahip yetkisini ve
çalışan hizmetlerini korur) ve 4. (her değişiklik bir kurtarma sözleşmesi taşır) ilkelerine
dokunur.

- **Şema.** 063 numaralı göç yalnız ekler: üç yeni tablo (`runtime_offerings`, `runtime_apps`,
  `runtime_app_operations`) ve dizinleri. Var olan hiçbir tablo, tetikleyici ya da değer
  değişmez. alpha.119'dan önceki sürüm kaydında 63 numaralı girdi olan veritabanını reddeder.
  Geri dönmek, her göçte olduğu gibi, güncelleme öncesi anlık görüntüyü geri yüklemektir.
- **Kalıcı işlemler.** Bir uygulamada aynı anda en çok bir işlem çalışır (tekil dizin). Panel
  yeniden başlarsa çalışan işlem kaydedilmiş adımından sürer. Ajanın her adımı güvenle
  tekrarlanabilir. Aynı işlem kimliği yeniden gelirse ilk yanıt döner, hiçbir şey ikinci kez
  başlamaz (kaybolan yanıt koruması). Gizli hiçbir şey yalnız bellekte tutulmaz: ortam dosyası
  zaten sunucudadır.
- **Durum sorgusu hiçbir şeyi değiştirmez.** Ekranın durum okuması ajanın yalnız okuyan
  yöntemlerini çağırır; adım başlatmaz, sağlık GET'ini çalıştırmaz.
- **Yerel durum sunucunundur.** Panel'in satırları birimin, ortam dosyasının ve klasörün
  kaydıdır; programın yaşam döngüsünün kaynağı değildir.

Bileşen testleri yukarıdaki kuralları kapsar. Bunlar laboratuvar kanıtı değildir: bu
tablolarla gerçek bir güncelleme ve otomatik geri alma henüz denenmedi.

## 17. Bilinen açıklar (laboratuvar turu için)

- Kaçış matrisi ([SITE-SANDBOX.tr.md §7](SITE-SANDBOX.tr.md)) desteklenen beş sistemde henüz
  çalıştırılmadı. `deploy/e2e/sandbox-escape/panel-runtime.sh`, Go yoklamasını panel üzerinden
  gerçek bir program olarak çalıştırabilir (yalnız deneme sanal makinesinde).
- `ProcSubset=pid`, `MemoryDenyWriteExecute` ve `NoExecPaths`/`ExecPaths` gerçek Go ve Rust
  programlarıyla ölçülmedi. Bir `/proc` dosyasına ya da JIT'e gerek duyan program başlayamayabilir.
- Kaynakta kapatıldı (alpha.119): `BindPaths=` artık site klasörünü adlandırmıyor. Bağlama birimi her
  başlatmada klasörü bağlantı izlemeden açar ve tanımlayıcısıyla bağlar; klasör sonradan bir
  bağlantıyla değiştirilirse çalışan program bundan etkilenmez, sonraki başlatma reddedilir
  (`app_folder_not_real`). Kaçış setinin 18. maddesi bunu gerçek sistemde kanıtlar; laboratuvar
  turu hâlâ açık.
- Koruma kanıtı sabit bir özellik listesini karşılaştırır (hesap, cgroup, dosya ve çekirdek
  korumaları, bellek, süreç, IP ve bağlanma engeli, çıkış satırları). Bu listenin dışındaki bir
  şeyi değiştiren ek ayar dosyası (örneğin `NoExecPaths=`, `SystemCallFilter=` ya da
  `CPUQuota=`) Programlar sekmesinde listelenir ama yayımlamayı durdurmaz.
- Sağlık GET'i istek üzerine ve yayımlama sırasında çalışır; henüz 5 dakikada bir değil.
- cgroup olaylarından (bellek aşımı, süreç sınırı) kötüye kullanım bildirimi henüz
  gönderilmiyor. Sayılar Programlar sekmesinde görünür.
- Ubuntu 22.04'te nft'nin `socket cgroupv2` eşleşmesi (SMTP engeli) kanıtlanmadı.

## 18. Python uygulamaları

Müşteri aynı altyapı üzerinde bir Python web uygulaması (Django, Flask,
FastAPI ya da herhangi bir WSGI veya ASGI uygulaması) çalıştırabilir. §5–16'nın
hepsi geçerlidir: sitenin kendi hesabı, site koruması, tek yerel port, yalnız
HTTPS, kalıcı yayımlama işlemi, güvenlik beklemesi, askıya alma, D-024
durumları ve yerel dosyalar. Bu bölüm yalnız farkları yazar.

Yüklenen kod rastgele koddur; pip'in kurduğu her paket de öyledir. Sınır dil
değil, korumanın kendisidir.

### 18.1 Yalnız dağıtımın kendi Python'u

| Sistem | Python | Paketler |
|---|---|---|
| Debian 12 | 3.11 | `python3`, `python3-venv` |
| Debian 13 | 3.13 | `python3`, `python3-venv` (`python3.13-venv`'i çeker) |
| Ubuntu 22.04 | 3.10 | `python3`, `python3-venv` (`python3.10-venv`'i çeker) |
| Ubuntu 24.04 | 3.12 | `python3`, `python3-venv` (`python3.12-venv`'i çeker) |
| Arch | 3.14 (sürekli güncellenen) | `python` (`ensurepip` içinde) |

4 Kasım 2026'da dağıtımların paket dizinlerinden doğrulandı. deadsnakes, pyenv
ya da python.org derlemeleri kullanılmaz.

Yönetici **Python**'u Bileşenler altından kurar (Web bölümü, çalışma ortamı
türü). Ajan sürümü `/usr/bin/python3` bağlantısından okur;
`/usr/bin/python3.<sürüm>` ve `/usr/lib/python3.<sürüm>/ensurepip/__init__.py`
dosyalarını denetler. Debian ve Ubuntu'da sistemin kendi sürümü olmayan bir
`python3` reddedilir. Arch'ta `python3` hangi sürümü gösteriyorsa dağıtımınkidir;
yeter ki bu sürüm o Python için bir uygulama sunucusu sabitlemiş olsun
(3.10–3.14). Daha yeni bir Arch Python'u (3.15) bir CelikPanel güncellemesini
bekler; ekran bunu söyler.

Python bileşenini kaldırmak, `python3`'e başka paketler gerek duydukça
reddedilir (kaldırma benzetimi onları adıyla yazar).

### 18.2 Haklar ve sunum

- İzin **`python_apps`** (Python uygulamaları).
- Sunucunun Python'u için sunum **`component:python:<sürüm>`**, örneğin
  `component:python:3.13`. Diğer bileşen sürümlerinden farklı olarak yönetici
  Eklentiler sayfasında risk özetini (§18.9) okuyup sunana dek **kapalıdır**.
  Python'u kurmak hiçbir şeyi sunmaz.
- Yönetici bu sunum için **kaynaktan derlemeye** de izin verebilir. Varsayılan
  olarak kapalıdır ve yöneticinin onayladığı risk özetini değiştirir (özete
  “kaynaktan derleme” eklenir). Saklanan risk özeti bu seçimi kaydeder; şema
  değişmedi (Python türü 063 tablolarına sığar, 064 kullanılmadı).
- Yolun kendisi bir izin adı taşımaz: uygulamanın türüne göre `program_apps`
  ya da `python_apps` içeride denetlenir.
- Bir sistem yükseltmesinden sonra (örneğin Debian 12'den 13'e) uygulama hâlâ
  eski sunumu taşır. Yayımlama `python_moved` ile reddedilir; uygulamanın
  ayarlarını bir kez kaydetmek onu, o sunumun kapılarından geçirerek
  sunucunun şu anki Python'una taşır.

### 18.3 Dosyalar

```
<site klasörü>/app/
  src/              yüklenen kod ve requirements.txt
  venv-<n>/         n numaralı kurulumun ortamı (çalışan ve bir öncekisi)
  venv-<n>.partial  süren bir kurulum (başarısız olursa silinir)
  .pip-cache/       pip'in indirme önbelleği (sitenin diskine sayılır)
  .build-tmp/       kurulum sırasında pip'in geçici klasörü
  data/ config/     §2'deki gibi
```

Birim `app/src` içinde çalışır (`WorkingDirectory`); `modül:nesne` oradan içe
aktarılır.

### 18.4 Ayarlar

| Ayar | Kural |
|---|---|
| Uygulama sunucusu | `wsgi` (gunicorn: Django, Flask) ya da `asgi` (uvicorn: FastAPI, Starlette). |
| Uygulama | `modül:nesne`, örneğin `mysite.wsgi:application` ya da `main:app`. Noktalı adlardan oluşan bir modül ve bir ad; fabrika çağrısı ya da ifade yok. |
| İşçi sayısı | 1 ile `min(8, bellek sınırı / 128 MB)` arası. |
| Sağlık yolu | §8'deki gibi. |

Kayıt, nesneyi `runtime_apps.entry` içinde, sunucuyu ve işçi sayısını
`runtime_apps.args` içinde iki argüman olarak (`server=wsgi`, `workers=2`)
tutar.

### 18.5 requirements.txt

Kurulumdan önce ajan `app/src/requirements.txt` dosyasını ve eklediği her
dosyayı, hepsi `app/src` altında ve hiçbir bağlantıyı izlemeden (openat2
`RESOLVE_BENEATH | RESOLVE_NO_SYMLINKS`) okur. Şunları dosya ve satır adıyla
reddeder:

| Reddedilen | Kod |
|---|---|
| `-e`, `--editable` | `editable` |
| `git+…`, `hg+…`, `svn+…`, `bzr+…`, `ad @ git+…` | `vcs` |
| başka her adres, `ad @ https://…` | `url` |
| `./x`, `../x`, `/x`, `x/y`, `*.whl`, `*.tar.gz` ve başka arşivler | `path` |
| `-i`, `--index-url`, `--no-index` | `index_url` |
| `--extra-index-url` | `extra_index_url` |
| `--trusted-host` | `trusted_host` |
| `-f`, `--find-links` | `find_links` |
| `app/src` dışını gösteren `-c` | `constraint_outside` |
| `app/src` dışını gösteren `-r` | `nested_outside` |
| olmayan dosyayı gösteren `-r`/`-c` | `nested_missing` |
| başka her seçenek (`--no-binary`, `--global-option`, …) | `option` |
| `$`, ters tırnak ya da `ad[ekler] sürüm ; koşul` olarak okunamayan satır | `syntax` |
| 16'dan çok dosya, dosya başına 256 KiB ya da toplam 4000 satır | `too_large` |
| dört düzeyden derin `-r` | `nested_too_deep` |

İzin verilenler: paket satırları, bir paketin ardından `--hash=sha256:…`
(sha384/sha512 de), `--require-hashes`, `--pre`, `--prefer-binary` ve
`app/src` içindeki `-r`/`-c`. Reddedilen satırdaki kimlik bilgileri
gösterilmeden önce gizlenir.

pip, müşterinin dosyasını değil, ajanın root olarak
`/run/celikpanel-rtbuild/<uygulama no>/` altına yazdığı **düzleştirilmiş
kopyayı** okur (klasör 0750, dosyalar 0640, `root:<site grubu>`). Bir satırda
özet varsa (ya da dosya `--require-hashes` diyorsa) pip `--require-hashes` ile
çalışır.

Bu, sunumun tedarik zinciri kuralıdır, güvenlik sınırı değildir: müşteri zaten
`app/src` içine her kodu yükleyebilir.

### 18.6 Kurulum (yayımlama adımı `build`)

Yayımlama: `validate → build → render → restart → proof → health → publish`.

Kurulum geçici bir birimdir, `celikrtbuild-<no>.service`: sitenin kendi
hesabı, uygulamanın koruması ama dinlenen port yok (PyPI için dışa bağlantı
açık, posta portu engeli yerinde, `Type=oneshot`, en çok 30 dakika). Uygulamanın
kendi birimi gibi `celikpanel-sandbox@celikrtbuild-<no>` bağlama birimini ister; bu birim
uygulama klasörünü root'a ait bir bağlama noktasından bağlar. Root komutları yalnız statik
yardımcının adımlardan önce gelen `egress-add`/`egress-remove` çağrılarıdır
([SITE-SANDBOX.tr.md](SITE-SANDBOX.tr.md) §1a). Adımlar
ayrı komut satırlarıdır; kabuk yoktur (komutlar İngilizce belgede, §18.6).
Birimin ana komutu (`ExecStart`) sistemin kendi Python'udur ve ortamı kurar.
`systemd-run`, ana komutun programını birim başlamadan önce sunucuda arar; bu
yüzden ana komut, kurulumun daha sonra oluşturacağı bir yol olamaz (alpha.119
deneme turu 1'de her kurulum bu nedenle başlayamadı). Yeni ortamın python'u ile
çalışan pip adımları `ExecStartPost` satırlarıdır; sırayla çalışır, ilk başarısız
adım kurulumu bitirir ve sonrakiler çalışmaz.
`--isolated` ve `PIP_CONFIG_FILE=/dev/null`, bir `pip.conf` dosyasının ya da
`PIP_*` değişkeninin paket dizinini değiştirmesini engeller. Müşterinin
adımından `--only-binary=:all:` yalnız sunum kaynaktan derlemeye izin
veriyorsa çıkarılır; sabitlenmiş paketler her zaman hazır pakettir.

Kuşaklar (`internal/siteruntime/venv.go`):

1. Kalmış `venv-*.partial` klasörleri silinir; `n`, klasörün ya da birimin
   andığı her kuşaktan bir fazladır.
2. Kurulum `venv-<n>.partial` içine yazar. **Çalışan uygulamaya dokunulmaz.**
3. Başarı: sitenin hesabıyla tek bir yeniden adlandırma (`mv -T`) ile
   `venv-<n>`. `venv-<n>` ve birimin çalıştırdığı kuşak dışındaki her kuşak
   silinir.
4. Başarısızlık: `venv-<n>.partial` silinir; işlem nedenini söyler.
5. Birim `venv-<n>` ile yazılır ve uygulama yeniden başlatılır.
6. Daha önce yayımlanmış bir uygulama açılmazsa (dinlemiyor, başlarken
   durdu, sürekli yeniden başlıyor, bellek aşımı, dış adreste dinliyor ya da
   sağlık yolu yanıt vermiyor) birim önceki kuşakla yeniden yazılır ve
   yeniden başlatılır. İşlem yine başarısız sayılır ve bunu söyler. `app/src`
   içindeki kod müşterinin yüklediğidir; yalnız ortam geri döner.

Ajan her kurulumun kaydını tutar (`runtime-apps/<no>/build.json`, root'un):
işlem, kurulan kuşak ve öncekisi. Panel yeniden başlarsa aynı kurulum sürer;
aynı işlemin tekrarı yeni bir şey başlatmaz. systemd'nin artık tanımadığı bir
kurulum (yeniden başlatma) bilinmeyen sonuçtur, asla başarı sayılmaz.

Neden `venv/bin/gunicorn` değil de `python -m gunicorn`: bir komut betiği
kurulduğu klasörün adını `#!` satırında tutar; ortam `venv-<n>.partial` olarak
kurulup yeniden adlandırılır. Yorumlayıcı bağlantısı ve `pyvenv.cfg` klasör
adına bağlı değildir.

### 18.7 Özetle sabitlenmiş uygulama sunucuları

CelikPanel önce bu paketleri `--require-hashes --no-deps` ile kurar. Hepsi
`py3-none-any`'dir; aynı sabitler desteklenen her Python sürümüne yeter.
`typing_extensions` yalnız 3.10'da kurulur.

| Paket | Sürüm | PyPI'a yüklenme | Paketin sha256 özeti |
|---|---|---|---|
| gunicorn | 26.2.0 | 2026-08-24 | `bd249d0b3f7972f7432f0a6b6ff3b3ee2d129f70cd1ff6c09a9dd9e29a2b88e3` |
| uvicorn | 0.54.0 | 2026-09-25 | `505bdb0f318731d45f1f712071fc781a8981f6847a31c902c9f5e652d4f67faf` |
| click | 8.5.0 | 2026-08-26 | `255bc9599cf7748b4b1a446ccc735421bd08a2ae529a8b88597d3de5664ee360` |
| h11 | 0.16.0 | 2025-04-24 | `63cf8bbe7522de3bf65932fda1d9c2772064ffb3dae62d55932da54b31cb6c86` |
| typing_extensions | 4.16.0 | 2026-07-02 | `481caa481374e813c1b176ada14e97f1f67a4539ce9cfeb3f350d78d6370c2e8` |

Kaynak: PyPI'ın JSON arayüzü (`https://pypi.org/pypi/<ad>/<sürüm>/json`,
`bdist_wheel` dosyasının `digests.sha256` alanı), 4 Kasım 2026'da okundu. Her
sürüm en az iki haftalıktı ve geri çekilmemişti. uvicorn `[standard]` eki
olmadan kurulur; derlenmiş bir şey gelmez. Yeni bir sabit,
`internal/siteruntime/pins.go` dosyasının incelenmiş bir değişikliğidir.

gunicorn ya da uvicorn'u adıyla anan bir gereksinim satırı kurulu kopyayı
değiştirebilir; uygulama sunucusu içe aktarılabilir kalmalıdır.

### 18.8 Birim

ExecStart sabittir: `<app>/venv-<n>/bin/python -m gunicorn --bind
127.0.0.1:<port> --workers <N> <modül:nesne>` ya da `… -m uvicorn --host
127.0.0.1 --port <port> --workers <N> <modül:nesne>`. Her sözcük denetlenir;
kabuk yoktur, metne `$` ya da `%` girmez. `WorkingDirectory=<app>/src`,
`PATH=<app>/venv-<n>/bin:/usr/bin:/bin`, `PYTHONUNBUFFERED=1`,
`PYTHONNOUSERSITE=1`.

**Koruma:** [SITE-SANDBOX.tr.md §1](SITE-SANDBOX.tr.md) temel kümesi ve
`ProcSubset=pid`. Python uygulaması `MemoryDenyWriteExecute` ya da
`NoExecPaths`/`ExecPaths` **almaz**: yorumlayıcı ortamdaki kendi eklenti
modüllerini çalıştırır, bazı paketler çalışırken kod üretir. Bu yüzden bir
Python uygulaması sunucunun başka programlarını (`/bin/sh` dahil)
başlatabilir; neye ulaşabilecekleri yalnız korumanın izin verdiğidir.

### 18.9 Risk özeti

Yöneticinin sunmadan önce, müşterinin yayımlamadan önce okuduğu:

- **Yapabilir:** her kodu çalıştırır, kendi klasörünü okur ve yazar, tek yerel
  portta dinler, dışa bağlantı açar, PyPI'dan kendi ortamına paket kurar.
- **Koruma engeller:** diğer siteler, CelikPanel dosyaları, yetki yükseltme,
  posta portları, bulut bilgi adresi, diğer portlar.
- **Risk olarak kalır:** dışa dönük kötüye kullanım; posta arayüzleri; işlemci
  ve belleğin paylaşılması; kabul edilmişse eksik koruma düzeyi; müşterinin
  kendi kodu; **PyPI paketleri başkalarının kodudur — CelikPanel yalnız
  dosyanın PyPI'ın yayımladığı özetle eşleştiğini denetler, ne yaptığını
  denetlemez;** yorumlayıcı başka programlar başlatıp kod üretebilir; izin
  verilmişse **kaynaktan derleme**, yayımlama sırasında her paketin kendi
  derleme kodunu aynı korumada çalıştırır.

### 18.10 Durumlar (D-024)

| Kod | Tür | Kim yapar | Anlamı |
|---|---|---|---|
| `runtime_missing` | önkoşul | yönetici | Sunucunun Python'u ya da venv modülü hazır değil (ya da başka sürüm, ya da sabit yok). |
| `src_missing` | önkoşul | müşteri | `app/src` yok ya da bağlantı veya dosya. |
| `venv_missing` | önkoşul | müşteri | Birimin andığı ortam silinmiş; yayımlama yenisini kurar. |
| `requirements_refused` | önkoşul | müşteri | Bir satır reddedildi; ekran dosyayı, satırı ve nedeni yazar. |
| `wheel_unavailable` | doğrulanmış hata | müşteri | Bir paketin PyPI'da bu Python için hazır paketi yok (ya da paket yok); paket adıyla yazılır. |
| `build_failed` | doğrulanmış hata | müşteri | pip ya da venv adımı başarısız oldu; pip'in son satırları, gizli bilgiler gizlenmiş. |
| `pypi_unreachable` | bilinmiyor | önce müşteri, sonra yönetici | Kurulum pypi.org'a ulaşamadı. |

Her kurulum hatası çalışan uygulamanın değişmediğini söyler. Durum sorgusu
yalnız işlemi okur; asla kurulum başlatmaz.

### 18.11 CelikPanel olmadan

Birim, ortam dosyası ve `app/venv-<n>` sıradan dosyalardır. Sunucu sahibi
`systemctl restart celikrt-<no>` çalıştırabilir ya da bir ortamı sitenin
hesabıyla elle yeniden kurabilir (`python3 -m venv`, `pip install -r
app/src/requirements.txt`). Sahibin değiştirdiği birim korunur (§13).

### 18.12 Bilinen açıklar

- Laboratuvar denemesi yok: beş sistemde Django (gunicorn), FastAPI (uvicorn)
  ve kaçış kitinin Python sondası, `ProcSubset=pid` ve kurulumun dışa erişimi
  laboratuvar turuna kaldı.
- Geri dönüş ortamı geri getirir, `app/src` içindeki kodu değil.
- pip'in kendisi (`ensurepip`'in getirdiği, dağıtımın paketlediği) CelikPanel
  tarafından sabitlenmez.
- Kurulumun dışa trafiği yalnız pypi.org ile sınırlı değildir.
