# Yapay zekâ ve otomasyon uygulamaları

*alpha.118 · [English](AI-APPS.md) · Haklar: [PLANS-AND-RIGHTS.tr.md §4.6](PLANS-AND-RIGHTS.tr.md)*

Sahip, 3 Kasım 2026: "hermes, openclaw gibi daha nice AI ile ilgili yazılımlar da olsun",
"paperclip, n8n gibi programlar da seçeneklerde olabilir" ve "güvenlik önemli". Bir müşteri,
alan adlarından birinde, PHP uygulama kataloğunun yanında bir yapay zekâ ya da otomasyon
uygulaması çalıştırabilir. Güvenlik her şeyden önce gelir: bir uygulama, yönetici onu sunana
kadar kapalıdır; sitenin kendi hesabıyla bir systemd korumalı alanında çalışır, yalnızca
sunucunun içinde dinler ve yalnızca HTTPS üzerinden açılır.

Bu belge alpha.118'de yapılanı anlatır. Uygulamaların desteklenen her sistemde denendiği
iddiası değildir: sürüm için bir laboratuvar turu daha yapılacaktır.

---

## 1. Katalog

Sürümler ve parmak izleri 3 Kasım 2026'da resmî kayıt depolarından okundu. Her uygulama tek
bir kesin sürüme sabitlenmiştir; güncelleme, yeni bir sabitlemeyle gelen yeni bir CelikPanel
sürümüdür ve açık bir "X sürümüne güncelle" düğmesi ister. Hiçbir şey kendi kendine
güncellenmez.

| Uygulama | Ne olduğu | Kaynak | Lisans | Sabitlenmiş sürüm | Çalışma ortamı |
|---|---|---|---|---|---|
| **n8n** | İş akışı otomasyonu (hizmetleri bağlar, yapay zekâ adımları, web kancaları) | npm `n8n`, github.com/n8n-io/n8n | Sustainable Use License 1.0 | 2.46.4, bütünlük değeri `sha512-Ek/AlWCx…Xr1Wg==` | Node.js 24 |
| **Open WebUI** | Yapay zekâ modelleri için bir sohbet arayüzü (OpenAI uyumlu sağlayıcılar) | PyPI `open-webui`, github.com/open-webui/open-webui | Open WebUI License (marka ibaresi maddeli BSD-3) | 0.12.5, wheel sha256 `74fafb4e…19d3fd` | Python 3.11 ya da 3.12 |
| OpenClaw | Yapay zekâ ajanı ağ geçidi | npm `openclaw`, MIT | — | sunulmuyor (bkz. §6) | Node.js 24/26 |
| Hermes Agent | Nous Research'ün yapay zekâ ajanı | PyPI `hermes-agent`, MIT | — | sunulmuyor (bkz. §6) | Python |
| Paperclip | Yapay zekâ ajanı şirket platformu | npm `paperclipai`, MIT | — | sunulmuyor (bkz. §6) | Node.js 24 + PostgreSQL |

**Yöneticinin sunmadan önce okuması gereken lisanslar:**
- n8n'in Sustainable Use License'ı kullanıma "yalnızca kendi iç ticari amaçlarınız ya da ticari
  olmayan ya da kişisel kullanım için" izin verir; n8n'in kendisini başkalarına ücretli hizmet
  olarak sunmak kısıtlıdır. Yönetici kaydı bu notu gösterir. **n8n bayilere kapalıdır** (sahibin
  3 Kasım 2026 kararı, lisans sorusu yanıtlanana dek): bkz. §3.
- Open WebUI'ın lisansı, kurulumda 30 günde en çok 50 son kullanıcı bulunmadıkça, yazılı izin
  ya da kurumsal lisans olmadıkça markasını yerinde tutmayı şart koşar.

## 2. Nerede çalışabilir

CelikPanel hiçbir paket kaynağı eklemez (NodeSource yok, deadsnakes yok, Docker yok,
`curl | sh` yok).

| Sistem | Node.js (dağıtımın) | Python 3 | n8n | Open WebUI |
|---|---|---|---|---|
| Debian 12 | 18 | 3.11 | panelin Node.js 24'üyle | evet (`python3-venv` gerekir) |
| Debian 13 | 20 | 3.13 | panelin Node.js 24'üyle | **hayır** (3.11/3.12 gerekir) |
| Ubuntu 22.04 | 12 | 3.10 | panelin Node.js 24'üyle | **hayır** |
| Ubuntu 24.04 | 18 | 3.12 | panelin Node.js 24'üyle | evet (`python3-venv` gerekir) |
| Arch | 26 / lts 24 | 3.14 | evet | **hayır** |

n8n, Node.js 24 ister (`engines: >=24.0.0`; bağımlılığı isolated-vm de öyle). Arch dışında
desteklenen hiçbir dağıtım bunu sunmaz; bu yüzden n8n, yöneticinin Hizmetler → Node.js
altından kurduğu Node.js çalışma ortamını kullanır: resmî nodejs.org arşivi, açılmadan önce
resmî SHASUMS256.txt ile denetlenir (Node.js sitelerinin kullandığı çalışma ortamının
aynısı). Node.js 24 kurulu değilse ekran bunu ve kimin kuracağını söyler. Sistemin Python'u
3.11-3.12 dışındaysa Open WebUI, bulunan sürümle birlikte o sistemde kullanılamıyor olarak
gösterilir.

## 3. Haklar ve sunum

- **Yapay zekâ ve otomasyon uygulamaları** izni (`service_apps`) ve her uygulama için bir
  sunum, `app:<id>`.
- **Sunulana kadar kapalı.** Yönetici Eklentiler → katalog sekmesi → *Yapay zekâ ve otomasyon
  uygulamaları* sayfasını açar, risk özetini okur, okuduğunu işaretler ve uygulamayı her kurulum
  için bellek, işlemci ve süreç sınırlarıyla birlikte sunar. İstek, özetin parmak izini taşır;
  bunu değiştiren bir sürüm, yeniden okunup bir kez daha sunulana kadar sunumu yine kapatır.
- Sınırsız planlar sunulan uygulamayı alır; bayi ya da plan onu yalnızca dışarıda bırakabilir.
  Sunucu, bir uygulama kurulurken ya da güncellenirken bütün katmanlara bakar. Durdurma, günlük
  ve kaldırma hiçbir zaman reddedilmez; böylece daraltılmış bir hak, çalışan bir uygulamayı
  ortada bırakmaz.
- **Lisans nedeniyle bayilere kapalı (n8n).** Katalog kaydında `reseller_closed` bulunan
  uygulama hiçbir zaman bayi üzerinden kullanılamaz. Yönetici onu yine sunabilir ve kendi
  müşterilerine verebilir. Sunucu; uygulama bir bayinin haklarına, bir bayi planına, bir bayinin
  sahip olduğu plana ya da bayi ağacındaki bir aboneliğin haklarına yazılmak istendiğinde ve böyle
  bir abonelikte kurulmak ya da güncellenmek istendiğinde, isteyen kim olursa olsun (o hesap adına
  işlem yapan yönetici de) lisansı neden göstererek reddeder (`SERVICE_APP` / `licence_reseller`,
  HTTP 409). "Sunulan her şey"den bir öğe çıkarmak onu bayi için hiçbir zaman eklemez. Durdurma,
  günlük ve kaldırma hiçbir zaman reddedilmez. Bayi ve müşterileri nedeni uygulamanın satırında
  okur (lisans; karar yöneticinin). Open WebUI etkilenmez.
- **Güvenlik beklemesi** (yönetici): her kurulumu hemen durdurur, açılışta başlamalarını
  engeller, kurulumları, güncellemeleri ve başlatmaları reddeder. Beklemeyi kaldırmak hiçbir
  şey başlatmaz. Veri asla silinmez.
- Ek kullanıcılar (ekip üyeleri) sekmeyi görmez.

## 4. Bir kurulum nasıl yürür

Müşteri, web sitesi olan ve etkin sertifikası bulunan bir alan adı seçer (ayrı bir alt alan
adı önerilir: bu durumda alan adı web sitesi yerine uygulamayı gösterir; sitenin dosyaları
kalır ve uygulama kaldırılınca geri gelir). Uygulamanın ilk yöneticisini girer (e-posta ve
12-128 karakterlik bir parola; böylece hiçbir ziyaretçi uygulamayı önce ele geçiremez) ve Open
WebUI için isteğe bağlı olarak OpenAI uyumlu bir anahtar ve adres girer.

| Adım | Kim | Ne yapılır |
|---|---|---|
| prepare (hazırlık) | Aracı (root) | Sabitlenmiş dosyayı kendi özel durum klasörüne indirir ve sha512/sha256 değerini sabitlemeyle karşılaştırır; uyuşmazsa dosya atılır. |
| | site kullanıcısı | `<site home>/apps/<app>/{dist,data,config}` klasörlerini oluşturur, doğrulanmış dosyayı ve `config/app.env` dosyasını (0600) yazar. Root, sitenin klasörüne asla yazmaz. |
| installing (kurulum) | site kullanıcısı, geçici birim `celiksvcinst-<id>` | Doğrulanmış arşivin `npm install` işlemi (ardından `npm audit signatures`) ya da doğrulanmış wheel dosyasının `python -m venv` + `pip install` işlemi; uygulamayla aynı korumalı alanın içinde. |
| activate (etkinleştirme) | Aracı | `/etc/systemd/system/celiksvc-<id>.service` dosyasını yazar, etkinleştirir ve başlatır. |
| verify (doğrulama) | Aracı → Panel | `127.0.0.1:<port>` adresinde ve başka hiçbir adreste **bu birimin** bir sürecinin dinlediğini kanıtlar. Başka yerde dinleyen bir birim ya da portu başka bir süreç tutuyorsa birim durdurulur ve yayımlanmaz. |
| publish (yayım) | Panel | Alan adının sitesi `service_app` türüne geçer: nginx `127.0.0.1:<port>` adresine vekil olur, HTTPS zorunludur. Etkin sertifika yoksa alan adı uygulamayı hiçbir zaman HTTP üzerinden sunmaz. |
| finalize (tamamlama) | site kullanıcısı | Önceki sürümlerin klasörleri silinir; Open WebUI'ın ilk parolası ortam dosyasından çıkarılır. |

Her adım kalıcı bir işlemdir (D-025): Panel yeniden başlarsa işlem kaldığı adımdan sürer; aynı
işlem kimliği kurulumu iki kez başlatmaz; ekranın yoklaması yalnızca okur. Ekran her hatayı
doğrulanmış hata, karşılanmamış önkoşul ya da bilinmeyen sonuç olarak adlandırır ve kimin
işlem yapacağını ve sonraki adımı söyler (D-024).

## 5. Korumalı alan

Her uygulamanın birimi (ve kurulum birimi) şu ayarlarla çalışır:

- `User=` sitenin kendi hesabı (kanıtlanır: ev klasörü tam olarak sitenin ev klasörüdür); asla
  root değil, ek grup yok, Docker yok.
- `NoNewPrivileges`, boş `CapabilityBoundingSet`, `RestrictSUIDSGID`, `LockPersonality`,
  `RestrictNamespaces`, `RestrictRealtime`, `SystemCallFilter=@system-service`,
  `SystemCallArchitectures=native`.
- `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`, `PrivateDevices`, `ProtectKernel*`,
  `ProtectControlGroups`, `ProtectClock`, `ProtectHostname`, `ProtectProc=invisible` (başka
  kullanıcıların süreçleri görünmez).
- `TemporaryFileSystem=/var/www/celikpanel:ro`; yalnızca uygulamanın kendi klasörü geri
  bağlanır: diğer bütün siteler ve abonelikler görünmez olur. `ReadWritePaths` yalnızca
  uygulamanın klasörüdür. CelikPanel'in kendi yapılandırma ve durum klasörlerine erişilemez.
- `MemoryMax`, `MemorySwapMax=0`, `CPUQuota`, `TasksMax` yöneticinin sınırlarından gelir;
  `UMask=0077`.
- `RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX AF_NETLINK`: internete çıkabilir
  (uygulamaların yapay zekâ sağlayıcılarına ihtiyacı var) ve herhangi bir sitenin PHP kodu
  gibi, sunucunun 127.0.0.1 adresinde dinleyen hizmetlere ulaşabilir (§7).

CelikPanel'in her uygulamanın kendi desteklenen ayarlarıyla kapattıkları:

- **n8n:** `NODES_EXCLUDE` = Execute Command ve Local File Trigger (sunucunun kendisi üzerinde
  işlem yapan düğümler); topluluk paketleri kapalı; Code düğümünde Python kapalı;
  `N8N_BLOCK_ENV_ACCESS_IN_NODE`; dosya düğümleri `data/files` ile sınırlı; Git kancaları
  kapalı; telemetri ve sürüm denetimleri kapalı; 127.0.0.1 üzerindeki görev aracısı ve kendi
  portu; sahip ortam değişkeniyle belirlenir (`N8N_INSTANCE_OWNER_MANAGED_BY_ENV`, Panel'in
  hesapladığı bir bcrypt özeti - parolanın kendisi sunucuya hiç ulaşmaz). JavaScript Code
  düğümü kalır; n8n'in kendi görev çalıştırıcısı korumalı alanında, yerleşik ya da harici modül
  olmadan çalışır.
- **Open WebUI:** `--host 127.0.0.1` üzerinde dinler; kod çalıştırma ve kod yorumlayıcı kapalı;
  `SAFE_MODE` (işlevler kapalı); kayıt kapalı ve yeni kullanıcılar beklemede; topluluk
  paylaşımı, Ollama, güncelleme denetimleri ve telemetri kapalı. Bunlar "kalıcı" ayarlardır:
  uygulamanın kendi yöneticisi (müşteri) bunları uygulamanın içinden yeniden açabilir; yine de
  yalnızca korumalı alanın içinde çalışırlar. Kalanlar: anahtarı girilen sağlayıcılarla
  sohbet, belgeler, istek üzerine web okuma, bir Open WebUI yöneticisinin eklediği Python
  "araçları" (uygulamanın sürecinde, korumalı alanın içinde çalışır).

Gizli bilgiler (sağlayıcı anahtarları, Open WebUI'ın ilk parolası, n8n'in şifreleme anahtarı)
yalnızca `config/app.env` dosyasında durur; 0600, sahibi site kullanıcısı. Panel bunları
saklamaz, denetim günlüğüne yazmaz (yalnızca hangi ayarların değiştiği yazılır), bir daha
gösterilmez; panelin gösterdiği her günlük satırı da önce sunucuda gizlenir (dosyanın kendi
değerleri ve anahtar, jeton, parola ya da bcrypt özeti gibi görünen her şey).

## 6. Bu sürümde sunulmayanlar

Paylaşımlı bir barındırma sunucusunda, kabuk komutları çalıştıran ve güvenilmeyen içerik okuyan
bir ajan (istem enjeksiyonu), korumalı alanın içinde bile diğer müşteriler için risktir.
Sürüm hazırlayıcısı (sahibi bilgilendirerek) şunların alpha.118'de bayilere ya da müşterilere
sunulamayacağına karar verdi:

- **OpenClaw** (MIT, npm `openclaw` 2026.10.10): araçları "sunucunun kendisinde çalışan" bir
  ağ geçidi; kurulum betikleri ister (yerel `node-pty`), kendini beta olarak tanımlar.
  Yalnızca yöneticiye açık, tek sahipli bir kip gerektirir.
- **Hermes Agent** (MIT): belgelenmiş tek kurucu `curl … | bash`'tir ve CelikPanel bunu
  çalıştırmaz; PyPI sürümü (0.19.0) GitHub sürümünün (0.22.6) gerisinde ve PyPI'daki sahibi
  doğrulanamadı; API sunucusu tam uçbirim erişimi verir.
- **Paperclip** (MIT, npm `paperclipai` 2026.1023.0): sunucuda Claude Code, Codex ve bash
  çalıştırır; varsayılan kipinde oturum açma yoktur; gömülü bir PostgreSQL içerir (çözüm,
  hesabın PostgreSQL hakkı olurdu).

Bunlar katalogta bu gerekçeyle listelenir; sunucu onları sunmayı ya da kurmayı reddeder.

## 7. Bilinen sınırlar

- **Bağımlılıklar parmak iziyle sabitlenmemiştir.** En üstteki dosya sabitlenmiştir ve iki kez
  denetlenir (Aracı tarafından ve kurulum adımında yeniden). n8n bir kilit dosyası yayımlamaz
  (`_hasShrinkwrap: false`): yaklaşık 165 bağımlılığı, kurulum anında npm tarafından n8n'in
  aralıkları içinde çözülür; npm her arşivi kayıt deposunun bütünlük değeriyle, `npm audit
  signatures` ise kayıt deposunun imzalarıyla denetler. Open WebUI doğrudan bağımlılıklarının
  çoğunu kesin olarak (`==`) sabitler, ancak pip bunları `--require-hashes` olmadan PyPI'dan
  çözer (her Python sürümü için tam bir parmak izi kilidi sonraki adımdır).
- **Kurulum betikleri çalışır.** `--ignore-scripts` iki uygulamayı da bozardı (n8n: sqlite3 ve
  isolated-vm yerel modülleri; sqlite3'ün betiği, GitHub sürümünden sabitlenmiş parmak izi
  olmayan hazır bir ikili dosya indirir). Betikler site kullanıcısı olarak korumalı alanın
  içinde çalışır. Hazır ikili dosya yoksa derleme araçları (python3, make, g++) gerekir; bunları
  yönetici kurar.
- **Boyut.** Open WebUI, PyTorch ve başka büyük paketleri çeker: kurulum başına birkaç GB,
  aboneliğin diskinden sayılır. İlk açılışında Hugging Face'ten bir gömme modeli indirir.
- **Yerel ağ arabirimi.** Bir uygulama 127.0.0.1 üzerindeki hizmetlere ulaşabilir (PHP gibi).
  Port çakışması algılanır ve uygulamayı durdurur; başka bir kiracının sürecinin boş bir portta
  dinlemesini engellemez.
- **Plan sınırları.** Bellek, işlemci ve süreç sınırları, henüz bir plandan değil,
  yöneticinin uygulama başına varsayılanlarından gelir.
- **D-043.** Yöneticinin güvenlik beklemesi uygulamaları kapsar; tam güvenlik acil durumu akışı
  (etkilenen müşterilere bildirimler, parolayı yeniden girme, sürümü geri çekme) uygulamalara
  henüz bağlanmadı.
- **Sahip bağımsızlığı.** Birim ve ortam dosyası düz systemd yapılandırmasıdır ve Panel
  olmadan çalışmayı sürdürür. n8n, `/opt/celikpanel/runtimes` altındaki Panel'in Node.js çalışma
  ortamını kullanır (Node.js siteleri gibi); bu klasörü kaldırmak n8n'i durdurur.
- **WebSocket**'ler nginx'ten geçer; `proxy_read_timeout` 60 sn'dir ve varsayılan yükleme
  sınırı nginx'in 1 MB'ıdır; daha büyük yüklemeler için sitenin nginx içerme klasöründe sahibin
  bir içerme dosyası gerekir.
- Uygulama kuruluyken abonelik silme: uygulama kaldırılana kadar alan adının silinmesi
  reddedilir; abonelik düzeyindeki silme yolları değiştirilmedi.

## 8. Panel olmadan çalıştırma ve durdurma

Sunucuda, root olarak (`<id>` uygulamanın sekmesinde gösterilir):

```
systemctl status celiksvc-<id>       # çalışıyor mu
systemctl stop celiksvc-<id>         # şimdi durdur
systemctl disable celiksvc-<id>      # açılışta başlatma
systemctl start celiksvc-<id>        # yeniden başlat
journalctl -u celiksvc-<id> -n 100   # günlüğü
```

Kendi ayarlarınız `/etc/systemd/system/celiksvc-<id>.service.d/*.conf` içine yazılır
(CelikPanel oraya asla yazmaz). Birim dosyasının kendisinde yapılan bir değişiklik korunur:
CelikPanel onun üzerine yazmayı reddeder ve bunu söyler. Uygulamanın dosyaları
`/var/www/celikpanel/subscriptions/<sub>/sites/<domain id>/apps/<app>/` altındadır.
