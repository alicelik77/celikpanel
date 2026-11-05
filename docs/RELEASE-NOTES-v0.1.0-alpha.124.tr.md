# v0.1.0-alpha.124

[English](RELEASE-NOTES-v0.1.0-alpha.124.md)

Bileşen kurulumu artık bileşen düzeyinde ve bileşenin ihtiyaç duyduklarını
aynı işlemde kuruyor; panelin veritabanı meşgulken denetim kayıtları
kaybolmuyor; aynı anda yalnız bir Agent çalışabiliyor. **Kurulu panelleri
yalnız sahibi, panelin kendi güncelleme ekranından günceller.**

## Bileşen kurma

- Kurulum penceresi bileşenin **gerektirdiklerini** paket paket değil, bileşen
  bileşen listeler: her biri kurulu (sürümüyle), onunla birlikte kurulacak ya
  da burada kurulamaz olarak, nedeniyle gösterilir (örneğin: posta motorları
  posta kurulumuyla gelir; bu sistemde sunulmuyor; o görevi başka bir bileşen
  üstlenmiş). Gereken bir bileşen burada kurulamıyorsa **Kur** kapalıdır ve
  pencere kimin ne yapacağını söyler.
- **Kur**'a basınca sunucu önce gerekenleri, sonra bileşeni tek işlemde kurar.
  Sayfa kapansa ya da panel yeniden başlasa da işlem sürer; bu sırada açılan
  sayfa aynı ilerlemeyi gösterir. Bir adım başarısız olursa o ana dek kurulan
  kurulu kalır ve pencere bunları adıyla söyler.
- Pencere açıldıktan sonra Kur'a basılana dek plan değiştiyse (örneğin başka
  bir yönetici bir şey kurduysa) hiçbir şey başlamaz ve pencere güncel planı
  gösterir.
- phpMyAdmin, phpPgAdmin ve Roundcube hangi kurulu PHP sürümüyle çalışacağını
  sorar; varsayılan olarak dağıtımın kendi PHP'si (yoksa en yenisi) seçilidir.
- Paket adları artık yalnız sahibin onay vermesi gereken yerde görünür: depo
  değişikliği depoyu açıkça adıyla söyler (Debian 12'de PostgreSQL 18 artık
  "üretici" yazmıyor).
- Sitelerin kullandığı bir PHP sürümünü kaldırma eskisi gibi reddedilir; ret
  artık o sürümle çalışan phpMyAdmin, phpPgAdmin ya da Roundcube'u da kendi
  sayfasının bağlantısıyla adıyla söyler.

## Panel veritabanı meşgul

- Başka bir program panelin veritabanını bir süre tuttuğunda denetim kayıtları
  artık kaybolmuyor. Henüz yazılamayan kayıt diskte de saklanır ve sonra kendi
  sırasıyla ve kendi zamanıyla yazılır. Denetim günlüğü sayfası kaç kaydın
  beklediğini ve nedenini söyler; kimsenin bir şey yapması gerekmez. Yaklaşık
  on dakika sonra hâlâ bekliyorlarsa sayfa çalıştırılacak tek komutu verir.
- Bileşen işlemi artık "çalışıyor"da kalmak yerine **panelin veritabanını
  bekliyor** der ve kendiliğinden devam eder. Sunucu kurulumu da aynı bekleme
  durumunu gösterir.

## Yalnız bir Agent (5 Kasım Frankfurt olayı)

**Ne oldu.** Frankfurt'ta Agent'ın sürümünü görmek için `agent --version`
yazıldı. Agent bu seçeneği tanımadı, yok saydı ve ikinci bir tam Agent
başlattı. İkinci Agent, panelin Agent'a bağlandığı noktayı (soketi) panelin
kullanamadığı izinlerle yeniden oluşturdu. Yaklaşık 40 dakika panel Agent'a
ulaşamadı ve alan adının DNS sekmesi yalnız "iç hata" dedi. İkinci süreç
durdurulup Agent yeniden başlatılarak düzeldi.

**Artık ne önlüyor.**

- `agent --version` (panelde ve kurtarma aracında da) sürümü yazar ve durur.
  Tanınmayan başka her seçenek, hiçbir şey başlatılmadan reddedilir.
- Çalışan Agent ömrü boyunca bir kilit tutar. İkinci bir Agent bunu görür,
  çalışanın nasıl denetlenip yeniden başlatılacağını söyler ve hiçbir şeye
  dokunmadan durur.
- Soketin grubu, Agent'ı systemd de başlatsa bir kişi de başlatsa hep aynıdır.
- Panel Agent'a ulaşamazsa ekran bunu söyler ve tek güvenli komutu verir:
  önce durumu denetle, sonra `sudo systemctl restart celikpanel-agent`.

## DNS ve sunucu kurulumu

- DNSSEC: PowerDNS'in ana sunucu, başka bir sunucunun yedek olduğu çiftte
  (Frankfurt ve Boston'daki gibi) bölgeyi imzalamak **henüz desteklenmiyor**.
  **Bölgeyi imzala** düğmesi kapalıdır ve yanında nedeni düz bir dille yazar.
  Önceden düğme açıktı; basınca hiçbir şey değişmeden genel bir iç hata
  çıkıyordu. DNSSEC kartının metni de sadeleşti.
- İlk sunucu kurulumu artık makinenin kendi adını (örneğin
  `frankfurt.example.com`) oluşturduğu alana A kaydı olarak ekler. O adda
  zaten bir kayıt varsa korunur. Bu sürümden önce kurulan sunucular
  değişmez; ana sayfaları **Kaydı ekle** önerisini göstermeye devam eder.

## Parolasız gönderilen posta (ilk adım)

- Postfix sayfası, parolasız gönderilen postanın **bir sonraki sürümde**
  kapanacağını söyler ve son 30 günde böyle posta gönderen siteleri listeler
  (Postfix'in kendi günlüğünden okunur). **Bu sürüm sunucuda hiçbir şeyi
  değiştirmez**: bu siteler eskisi gibi göndermeye devam eder.
- Listelenen her sitenin **Gönderim posta kutusu oluştur** düğmesi vardır:
  `noreply@<alan adı>` kutusunu oluşturur ve sitenin posta ayarlarına
  yazılacak sunucu, bağlantı noktası, kullanıcı adı ve parolayı bir kez
  gösterir. Liste ardından kendiliğinden güncellenir.

## Diğer

- Agent, kendi önceki kendini güncelleme denemeleri için systemd'nin tuttuğu
  "başarısız" işaretini, sonraki bir deneme başarılı olduğunda temizler;
  bunlar artık `systemctl --failed` altında görünmez. Çözülmemiş denemeler
  işaretini korur.
- Kullanıcıların yüklediği programlar, Python uygulamaları ve hizmet
  uygulamaları Ubuntu 22.04'te Ubuntu 24.04'tekiyle birebir aynı davranır;
  bunu artık bir test güvenceye alıyor.

## Laboratuvar sonuçları

Laboratuvar turu bekliyor. Bu bölüm yayımlanmadan önce sonuçlarıyla
güncellenecek.

## Bilinen sınırlar

- Yedek sunucusu olan PowerDNS ana sunucusunda DNSSEC imzalama yok; düğme
  nedeniyle birlikte kapalı kalır.
- Parolasız posta bu sürümde yalnız raporlanır; kapatma alpha.125'te gelir.
- Bir işlemin durumunu kaydeden her yer henüz meşgul veritabanını
  beklemiyor. Sırada olanlar: çalışma ortamı ve hizmet uygulamaları, uzak
  yedekler, güvenlik acil durumu ve askıya alma, eş sunucunun ana makine
  kaydını ekleme sonucu.
- Sonraya planlananlar: ClamAV, vsftpd ve Python uygulamaları için bağımlılık
  kuralları; Redis, Valkey ve Memcached için kullanım kayıtları; tek bir
  PostgreSQL ana sürümünü kaldırma.
- Makinenin kendi adı için AAAA (IPv6) kaydı kurulumda eklenmez.

## Kanıt

Şema değişmedi, geçiş yok. Sürüm sırası 123 → 124.
Go testleri (bütün modül) ve web testleri geçiyor; web paketleri bütçeleri
içinde. Laboratuvar turu bekliyor.
