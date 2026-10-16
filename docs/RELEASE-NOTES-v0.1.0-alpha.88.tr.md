# v0.1.0-alpha.88 (aday, yayımlanmadı)

[English](RELEASE-NOTES-v0.1.0-alpha.88.md)

Bu, v0.1.0-alpha.87'yi izleyen adaydır. Yayımlanmadı (sondaki "Yayımlama
kaydı"na bakın). v0.1.0-alpha.88 numarasını, sahibin 13 Ekim 2026 tarihli
kalıcı talimatı uyarınca ("hem sürüm numaraları hem de commit işlemleri için
bana sormana gerek yok": sürüm numaraları ve commit'ler için sahibe sorulmaz)
sürümü hazırlayan seçti; sahibin bu sürüm için adını koyduğu bir numara
değildir. Sürüm hazırlığı `7231afdbf` commit'idir (87'den sonra sıra 88;
önceki olarak alpha.87 sürüm commit'i
`a8cddcb546302daa0f5eb65b18fe6b9566b69e66`). Aday, `main`'in `89995199f`
noktasından (alpha.87 yayımlama kaydı) açılan `feat/alpha88-update-steps`
dalıdır. Bu notlar 16 Ekim 2026'da (UTC) yazıldı.

Bu sürümün tek konusu var: panelin kendi güncellemesini nasıl gösterdiği.

## Sahibin gözlemi ve tasarımı (16 Ekim 2026)

Sahip 16 Ekim 2026'da ikinci kurulu sunucuyu (Frankfurt) v0.1.0-alpha.85'ten
v0.1.0-alpha.87'ye güncelledi. Bu güncelleme alpha.85'in kendi güncelleme
ekranında çalıştı; o ekran, ortada bir sorun yokken bağlantının kesildiğini ve
panelin yeniden başlıyor olabileceğini söyleyen eski uyarıyı ve yanında köşe
bildirimini hâlâ gösteriyordu. Sahibin istediği, özüyle: güncelleme sürerken
her işlem için bir ilerleme çubuğu gösterilsin; her biri bittikçe bir
sonrakinin çubuğu altında belirsin; güncelleme gerçekten bitip panel yeniden
başladığında güncel durum gösterilsin; "bağlantı kesildi, güncelleniyor
olabilir" gibi ürkütücü hiçbir şey gösterilmesin; daha kullanıcı dostu bir yol
olmalı. Aynı gün sahiple üzerinde anlaşılan tasarım bu sürümün şartnamesidir.
D-024, D-032 ve D-033 uygulanır; yeni karar kaydedilmedi. Tasarım, koddan
okunan adımlar ve kanıt [OPERATION-GUIDANCE](OPERATION-GUIDANCE.tr.md) içinde,
"Güncelleme her adım için bir çubukla, yeniden başlaması planlı bir adım olarak
(2026-10-16)" başlığı altında kayıtlıdır.

## Ne göreceksiniz

Önce sunucu sahibi için düz sözlerle. Güncelleme sırasında güncelleme sayfası
her adım için bir tane olmak üzere beş kısa satır gösterir: güncelleme paketini
indirme, imzayı ve sağlama toplamlarını denetleme, kurmadan önce sunucuyu
denetleme, yeni sürümü kurma ve paneli yeniden başlatma, yeni sürümü doğrulama.
Biten adımda onay işareti, geçerli adımda bir çubuk vardır; sonraki adımlar
altta sessizce bekler. Panel yeniden başlarken o adım yalnız şunu söyler:
"Panel yeniden başlıyor; bu adım birkaç dakika sürebilir. Bu sayfa
kendiliğinden yeniden bağlanır." Yeni sürüm yanıt verip doğrulanınca sayfa
"{sürüm} sürümüne güncellendi" der, sonra yeniden yüklenir. Tek bildirim vardır:
güncelleme sayfası açıkken hiçbir köşe bildirimi onu yinelemez. Güncelleme sürerken
diğer sayfalar kilitli kalır ve sayfa bunu tek cümleyle söyler. Uyarı yalnız
panel güncellemenin başlangıcından altı dakika sonra hâlâ yanıt vermiyorsa ya
da bir hata doğrulanmışsa görünür.

**Bunu ilk ne zaman göreceksiniz.** Bir güncelleme her zaman ondan önce kurulu
olan sürümün ekranında çalışır. Bu yüzden bir sunucuyu alpha.88'e *doğru*
güncellemek hâlâ alpha.87'nin ekranını (alpha.86'nın sakin yeniden başlaması
ve tek bildirimi) gösterir. Adım çubukları ilk kez alpha.88'*den* sonraki bir
sürüme güncellemede görülür; v0.1.0-alpha.89'un aynı kodla gelmesinin nedeni
budur. Bu sürümün yayımlanması hiçbir kurulu sunucuyu güncellemez; bunu siz,
panelin kendi güncelleme ekranından başlatırsınız.

## Ne değişti

Ürün commit'leri: `b1d2ee381` (Agent), `22cdc2c4a` (Panel), `8b96ca5ce` ve
`b63da83e0` (sayfa), `07aaa50d5` (kilit cümlesi). Belgeler: `78174fc12`.

- **Sunucu beş adım bildirir** (`22cdc2c4a`;
  `cmd/panel/system_update_plan.go`, yeni; `system_update_handlers.go`;
  `server_lifecycle.go`). Başlatma ve durum yanıtları eklemeli bir `plan`
  taşır: `steps` (`download`, `verify`, `prepare`, `restart`, `confirm`; her
  birinde `id`, `state`, `started_at`, `finished_at`, `elapsed_seconds`,
  `progress`), `expected_restart_seconds` ve `restart_bound_seconds`. Bir adım
  yalnız kanıtla bitmiş sayılır (`downloaded_at`, `verified_at`, bu Panel'in
  hedef derleme olması, `succeeded`). **Kurma ve yeniden başlatma tek
  adımdır**, çünkü güncelleyici hiçbir şeyi kurmadan önce Panel'i durdurur:
  dışarıdan ikisi ayırt edilemez, bu yüzden sayfa ayırt edemeyeceği iki adım
  çizmez. Testler: `cmd/panel/system_update_plan_test.go`.
- **Agent güncelleyicinin ilerlemesini okur** (`b1d2ee381`;
  `cmd/agent/system_update_progress_linux.go`, yeni; `system_update_rpc.go`;
  `system_update_store_linux.go`; `internal/transport/system_update_contracts.go`,
  eklemeli). İndirme dizinini ve sürüm tabanını yalnız `Lstat` ile okur, sahibi
  ve izinleri denetler, bu isteğe ait olmayanı yok sayar ve durum RPC'sine
  `downloaded_bytes`, `downloaded_at` ve `verified_at` ekler. Salt okurdur;
  kabulün, kurtarmanın ya da kaydın durumunun girdisi asla değildir. Testler:
  `cmd/agent/system_update_progress_linux_test.go`; okumanın dayandığı `get.sh`
  satırlarını sabitleyen bir sözleşme testi dahil.
- **Yeniden başlama ölçülür ve hatırlanır** (`22cdc2c4a`). Yeni Panel kendi
  güncellemesinin yeniden başlamasını bir kez, `verified_at`'ten yönetiminin
  açıldığı ana kadar ölçer ve `panel_settings` içinde
  `panel_update_last_restart` anahtarında (`{"request_id", "seconds"}`) tutar.
  Beklenen yeniden başlama süresi en çok (son × 1,5, 90 sn), en fazla 360
  sn'dir. Ölçülen süre güncelleyicinin Panel'i durdurmadan önceki kendi
  denetimlerini de içerir; yani en az sayfanın gördüğü kesinti kadardır.
- **Her adım için bir çubuk** (`8b96ca5ce`; `web/src/lib/systemUpdateSteps.ts`
  ve `web/src/components/SystemUpdateSteps.tsx`, yeni;
  `SystemUpdateOperation.tsx`; `PanelUpdateCard.tsx`;
  `lib/systemUpdateTracking.ts`; `index.css`). Pencere, güncelleme kartı ve
  küçük köşe bildirimi adımları çizer; ölçülen indirme yüzdesini, ölçülmeyen
  adım kayan bir çubuğu gösterir.
- **Planlı bir adım olarak yeniden başlama.** Sayfa güncelleme etkinken
  Panel'e ulaşamazsa ya da `PANEL_STARTING` okursa, yeniden başlama çubuğu
  beklenen süreye göre zamanla ilerler (o sürede %90'a kadar doğrusal, sonra
  %99'a yaklaşır; kendiliğinden asla bitmez) ve yukarıdaki tek sakin cümle
  gösterilir. Yeniden başlama sırasında açılan bir sayfa aynı adımı çizer.
- **"{version} sürümüne güncellendi"**, yeni sürüm doğrulanınca yeni derleme
  kimliğiyle; sonra sayfanın eskisi gibi kendi yeniden yüklenmesi.
- **Gecikme ve hata metni yalnız doğru olduğunda** (`b63da83e0`). alpha.86'nın
  gecikme metni yalnız 6 dakikalık pencereden sonra ya da doğrulanmış bir
  hatada görünür; pencere aşılınca yeniden başlama adımı çubuksuz, yalnız
  etiketiyle kalır (tarayıcı turu 17, gecikme metninin yanında %98'de duran bir
  çubuk buldu; neredeyse bitti gibi okunuyordu).
- **Tek bildirim.** Güncelleme sayfasında başarıda da tek bildirim karttır;
  hata, köşe bildirimini ve kendi metnini korur.
- **Kilit cümlesi sadeleşti** (`07aaa50d5`). Adımlar gösterilirken pencerenin
  kilit cümlesi artık "Güncelleme sürerken diğer sayfalar kilitli kalır."
  (yeni anahtar `panelUpdate.steps.locked`). Sunucu planı olmayan sayfalar
  önceki iki dakika/altı dakika metnini korur (`panelUpdate.interactionLocked`);
  gezinme kilidinin kendisi değişmedi. Test:
  `web/tests/update-steps-mounted.test.mjs`.

Yeniden başlama boyunca gezinme kilidi (set17b F1), lisans ve oturum
kapılarının toleransı (set17 D1), D-032 ve D-033 korunur.

## Ne ölçüldü ve nasıl

- **Web:** `npm test` 1523'te 1523, `npx tsc --noEmit` temiz ve `npm run build`
  paket bütçesiyle geçiyor (giriş dosyası 242.40 KiB ham, 74.25 KiB gzip);
  hepsi `07aaa50d5`'te yeniden çalıştırıldı.
  `web/tests/update-steps-mounted.test.mjs` 24 test içerir; her yeni test,
  modüllerin ve alanların bulunmadığı önceki kaynağa karşı başarısız olur.
- **Go** (OPERATION-GUIDANCE girdisinden; `b63da83e0`'da WSL konuğu
  `CelikPanel-S2-Debian` içinde ölçüldü; ondan sonra hiçbir Go dosyası
  değişmedi, bu notlar için yeniden çalıştırılmadı):
  `go test ./cmd/panel/... ./internal/transport/...` 1602 test ve 1695 alt
  test geçti, 6 atlandı, 0 başarısız; `go test ./cmd/agent/...` 1805 test ve
  2838 alt test geçti, 29 atlandı, 0 başarısız; 14 Ekim 2026 denetim
  girdisinin temiz bağlama ad alanında.
- **Tarayıcı turu 17** (`web/tools/browser-inspect/scenarios-batch17.mjs`,
  sahte sunucuya karşı): 8 yapılandırmada 9 durum (dizüstü 1280 ve telefon
  390, EN ve TR, açık ve koyu); bir kusur bulundu ve düzeltildi (pencereden
  sonra duran çubuk), bir kez yeniden çekildi: pencereden önceki 64 durumun
  hiçbirinde ürkütücü cümle yok, sonraki 8'inin hepsinde gecikme metni var,
  yatay taşma yok, kilit yeniden başlama ve doğrulama boyunca açık. Ekran
  görüntüleri depoda değildir. `07aaa50d5`'ten önce çekildiler, yani önceki
  kilit cümlesini gösterirler; yeni cümle çekilmedi.
- **Sürüm hazırlığı:** `bash deploy/test-release-sequence-policy.sh` Windows
  Git Bash çalışma kopyasında geçiyor ("87 -> 88; historical bootstrap sequence
  87").

## Bu sürümün sınırları

Bunlar bilinir. Gizli kusur değildir.

- **Sahibin talimatıyla gerçek sistem kaydı yok.** Sahip 16 Ekim 2026'da bu
  sürümün gerçek sistem kaydı olmadan çıkmasını istedi. QEMU konuğunda ya da
  kurulu bir sunucuda hiçbir şey ölçülmedi: Agent'ın ilerleme okuması, tabanın
  zaman damgası ve yeniden başlama ölçümü gerçek bir güncelleme sırasında
  gözlenmedi. 90 saniyelik varsayılan set16'ya dayanır (Panel en çok 32 sn
  yanıtsız).
- **Adım görünümü ilk kez alpha.88'den güncellemede görülür.** alpha.88'e
  güncelleme hâlâ önceki sürümün sayfasında çalışır; adımların görülebilmesi
  için v0.1.0-alpha.89 aynı kodla gelir.
- **Aynı sürümün yeniden denenmesi doğrulama sınırını bildirmez.** Sıra
  değişmediğinde `get.sh` tabanı yeniden yazmaz; yeniden başlamadan önceki
  adımlar Panel kaybedildiğinde ya da yeni derleme yanıt verdiğinde bitmiş
  gösterilir.
- **`prepare` adımı yalnız birkaç saniye sürebilir;** taban ile Panel'in durması
  arasından daha ayrıntılısı okunamaz.
- **Bitmiş durumda kilit cümlesi.** Sayfa yeniden yüklenmeden önceki kısa anda
  pencere "{sürüm} sürümüne güncellendi" yanında hâlâ "Güncelleme sürerken
  diğer sayfalar kilitli kalır." der.
- **Sağlayıcı bir testte bağlanmış değildir** (IndexedDB, Web Locks, kimlik
  doğrulama sinyali); bağlantıları kaynaktan ve sahte tarayıcıda denetlenir.
- **Portal ve imzalı manifest sözleşme testleri** bu hazırlık için
  çalıştırılmadı; Windows çalışma kopyasında portal sözleşmesi izlenen açık
  anahtarın kanonik PEM denetiminde durur (incelenmedi; alpha.87 hazırlığı bu
  sözleşmeleri WSL konuğunda çalıştırmıştı).
- **Açık kabul işleri.** [Dayanıklılık sözleşmesinin](RESILIENCE-CONTRACT.tr.md)
  beş temel işinin hepsi kısmi kalır; bu sürüm hiçbirini kapatmaz ve
  ilerletmez. P0.1–P0.5 **kısmi** kalır. Bu sürümün yayımlanması onun kabulü
  olmaz.
- **Hiçbir kurulu sunucu ölçülmedi.** [alpha.87 notlarının](RELEASE-NOTES-v0.1.0-alpha.87.tr.md)
  açık saydığı her şey açık kalır.

## Yayımlama kaydı

Yayımlanmadı. Yayımlanana kadar burada hiçbir şey kaydedilmez. Yayımlamadan
önce açık olanlar:

1. Sürüm hazırlığı: `7231afdbf` commit'inde yapıldı; `cmd/`, `internal/` ya da
   `web/` altında hiçbir dosyayı değiştirmez: önceki olarak
   `a8cddcb546302daa0f5eb65b18fe6b9566b69e66` ile 87'den sonra 88 sürüm sırası
   politikası, `download-portal/get.sh` sabitlemeleri (sıra 88 ve sürüm
   v0.1.0-alpha.88; açık anahtar sabitlemesi değişmedi), README sürüm satırları
   ve üç sözleşme testi. Hâlâ açık: etiketten önce ayarlanan depo değişkeni
   `CELIKPANEL_RELEASE_SEQUENCE=88`. Gerçek arşivi CI etikette derler ve
   imzalar.
2. Dal için bir çekme isteği ve onun CI koşusu.
3. Birleştirme, ana commit üzerindeki koşu, etiket, altı imzalı varlığın
   denetimi ve indirme portalının yayımı.
4. Sürüm yayımlandıktan sonra her kurulu sunucunun sahibi o sunucuyu panelin
   kendi güncelleme ekranından günceller. Bu, yukarıdaki her adımdan ayrı bir
   adımdır; başka kimse başlatmaz.

Bu sürümü yalnız CelikPanel'in güncelleme arayüzünden kurun. Yayımlanması kurulu
sunucuları güncellemez; bir sunucunun güncellemesini yalnız sahibi, panelin
kendi güncelleme ekranından başlatır.
