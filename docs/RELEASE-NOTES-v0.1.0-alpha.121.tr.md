# v0.1.0-alpha.121

[English](RELEASE-NOTES-v0.1.0-alpha.121.md)

Sahibin Boston ve Frankfurt'u alpha.119 ve 120'ye güncelledikten sonra
karşılaştığı sorunların düzeltildiği küçük bir sürüm. **Kurulu panelleri yalnız
sahibi, panelin kendi güncelleme ekranından günceller.**

## Ubuntu 24.04'te PostgreSQL 18

- PostgreSQL (PGDG) deposundan PostgreSQL 18, Ubuntu 24.04'e kurulamıyordu. Bu
  sürüm, Ubuntu'nun sunduğundan daha yeni bir `postgresql-common` (ve
  `postgresql-client-common`) ister; deponun sınırlaması ise Ubuntu'nunkileri
  tutuyordu. Kurulum artık bu iki paketin PGDG sürümlerini adıyla ister; bunu
  yalnız Ubuntu'nun sürümü yetersizse yapar. Kurulum penceresi onları `libpq5`
  ile birlikte, güncellemeleri artık PGDG'den gelecek paketler olarak listeler.
  Önce siz onaylarsınız. Hiçbir paket kaldırılmaz; PHP'nin PostgreSQL eklentisi
  çalışmaya devam eder.
- Debian 13'te ve Ubuntu'da PostgreSQL 17 ya da 16 kurulumu tam önceki gibidir.

## Daha açık iletiler

- Kurulum penceresi bir kurulumun neler getireceğini denetleyemediğinde artık
  nedenini (örneğin "karşılanamayan bağımlılık: postgresql-common"), kimin bir şey
  yapması gerektiğini ve sonraki adımı söyler. Etkin bir deponun sunması gereken
  bir paket bulunamazsa panel o deponun paket listelerini bir kez tazeler ve
  yeniden denetler.
- Güncellemenin hemen ardından, sunucu açılış işlerini bitirmediği için reddedilen
  tek tıkla kurulum artık tam olarak bunu söyler: bir şey yapmanız gerekmez, bir
  iki dakika içinde kendiliğinden biter.

## Güncelleme ekranı

- Meşgul paket yöneticisi yüzünden duran bir güncellemeden sonra
  "… sürümüne güncelle" düğmesi eski bildirimi kapatır ve güncellemeyi tek basışta
  başlatır. Önce sunucu yeniden denetlenir. Hiçbir şey kendiliğinden başlamaz.

## Sunucunun ana bilgisayar adı

- Ana sayfa, bu sunucunun kendi adı (kenar çubuğunda görünen) genel DNS'te
  çözülmüyorsa bunu ve kimin bir şey yapması gerektiğini söyler. O bölge bu
  panelde ise **Kaydı ekle…** eklenecek adres kaydını tam metniyle gösterir ve
  bölgenin olağan kayıt düzenleyicisi yoluyla ekler. Denetim yalnız okur ve
  önbelleklidir.

## Bilinen sınırlar

- Bu yolla yalnız PGDG'nin `postgresql-common` ve `postgresql-client-common`
  paketleri alınır. Dağıtımın çok eski sunduğu başka bir paket yine bildirilir,
  değiştirilmez.
- Ad bildirimi, başka yerde barındırılan bir bölge için tek tık önermez;
  başka bir adresi gösteren ad için yalnız açıklama yapar.

## Kanıt

Yeni Ajan ve Panel kodu için Go testleri geçiyor. 2.444 web testi paket
bütçeleri içinde geçiyor. Sürüm sırası 120 → 121. apt'ın davranışı yalnız
`apt-get -s` ile, sınırlanmış bir test deposuyla denetlendi (apt 3.0.3, klasik
ve 3.0 çözücüleri): Ubuntu benzeri bir sunucu Boston gibi başarısız olur, iki
ortak paket adıyla istenince başarılı olur (3 yükseltme, 2 yeni, 0 kaldırma).
Debian benzeri bir sunucuda yalnız `libpq5` gerekir.

Laboratuvar (`final-alpha121-run1-20261104`, laboratuvar dalı `31fb2aec0`): Ubuntu'nun PostgreSQL 16'sı, phpPgAdmin ve bir veritabanı olan, PGDG'si açık alpha.120 Ubuntu 24.04, 84 saniyede 121'e güncellendi. PGDG 18 önizlemesi libpq5 (16.15 → 18.6), postgresql-common ve postgresql-client-common (257build1.1 → 293) paketlerini onaya sundu; 18 kuruldu ve 16'nın yanında çalışıyor; iki küme de listeleniyor; PHP'nin PostgreSQL eklentisi duruyor; 16'nın verisi bozulmadı. PGDG 17 yalnız libpq5'i gösterdi. PGDG paket listeleri silinince önizleme onları yeniledi ve normal denetledi. Debian 13: PGDG 18 yalnız libpq5'i gösterdi ve kuruldu. Paket yöneticisi tutulurken güncelleme başlatma geride hiçbir şey bırakmadan reddedildi, boşalınca tek basışla başladı. Gerçek sistemde denenmeyenler: sunucu adı uyarısı (laboratuvarın sunucu adı alan adı değil) ve açılış meşguliyeti mesajı (güncellemeden sonraki ilk kurulum meşgul değildi).
