# Uzak yedekler

*alpha.118 · sahibin isteği "uzak yedek" · [English](REMOTE-BACKUPS.md)*

Sunucunun kendi diskindeki yedek, sunucu kaybedilince onunla birlikte gider. **Hedef**, sizin
denetiminizdeki başka bir yerdeki depolamadır. CelikPanel yedekleri oraya yalnız sizin
bildiğiniz bir parola cümlesiyle şifreleyip gönderebilir ve geri getirebilir.

**Yapılanlar.** İki tür hedef desteklenir:

- **S3 uyumlu nesne depolama:** AWS S3, Backblaze B2 (S3 API), Wasabi, MinIO ve Cloudflare R2.
- **SFTP sunucuları.**

Bunların üzerine şunlar gelir:

- Kaydetmeden önce bağlantı denemesi: küçük bir dosya yazılır, okunur ve silinir.
- Elle gönderme ("Ayrıca gönder…").
- Site başına zamanlama ve yönetici için tüm siteler zamanlaması; basit günlük ve haftalık
  saklama ile.
- Hedefteki yedeklerin listesi.
- Mevcut geri yükleme yolundan geri yükleme.

Hepsi panelin içinde, Go ile yazılmıştır. CelikPanel rclone, restic ya da başka bir dış araç
çalıştırmaz.

**Yapılmayanlar.**

- **Artımlı yedek.** Her uzak kopya tam bir pakettir.
- **Panel yeniden başlayınca yarım kalan yüklemeyi kaldığı yerden sürdürmek.** İşlem aynı
  kimlikle yeniden çalışır ve paketi baştan gönderir.
- **Panel üzerinden başka bir sunucuya ya da başka bir siteye geri yükleme.** Bunun için
  [CelikPanel olmadan geri yükleme](#celikpanel-olmadan-geri-yükleme) adımlarını kullanın.

## Sahibin bağımsızlığı

- **Yerel yedekleriniz değişmez.** Onlar ve zamanlamaları önceki gibi çalışır. Uzak kopya ek bir
  özelliktir.
- **Zamanlanmış gönderim panelin otomasyonudur.** Panel kaldırılırsa artık bir şey gönderilmez.
  Gönderilmiş olanlar hedefinizde kalır ve CelikPanel olmadan açılır: dosyalar OpenPGP
  biçimindedir ve dağıtımın kendi `gpg` aracı onları açar.
- **CelikPanel uzak bir kopyayı yalnız sizin belirlediğiniz saklama kuralıyla siler.** Bir
  zamanlamanın saklama kuralı yalnız o zamanlamanın eski kopyalarını siler. Elle gönderilenlere ve
  başka bir zamanlamanın kopyalarına dokunmaz. Bir hedefi panelden kaldırmak, orada saklananları
  yerinde bırakır.

## Kim kullanabilir

| Kim | Ne yapabilir |
|---|---|
| Yönetici | **Sunucu geneli** hedef ekler. Bu hedefleri her site için kullanır. Her birini hesaplara **sunabilir**. Tüm siteler zamanlamasını kurar. |
| Bayi, müşteri | **Kendi** hedefini yalnız **Kendi uzak yedek hedefleri** yetkisiyle (`remote_backup_destinations`, D-042) ekler. Yöneticinin sunduğu hedefi her zaman seçebilir. |
| Ek kullanıcılar (ekip) | Hedef yönetmez. |

- **Her kuralı sunucu uygular.** Ekranlar yalnız izin verilmeyeni gizler.
- **Bir yetkinin kaldırılması çalışan zamanlamayı durdurmaz (D-042 P5).** Yalnız yeni seçim
  reddedilir.
- **Abonelik devrinden sonra önceki sahibin hedefi bu sitenin yedeklerini tutmaz.** Sonraki
  çalışma "hedef kullanılamıyor" diye durur; yeni sahibin verisi başkasının depolamasına gitmez.

## Şifreleme ve parola cümlesi

Her nesne gönderilmeden önce bu sunucuda şifrelenir. Biçim OpenPGP simetrik şifrelemedir
(RFC 4880):

- AES-256 şifre.
- Yinelemeli ve tuzlu SHA-256 anahtar türetme.
- Bütünlük korumalı veri paketi (MDC).

Bu, `gpg --symmetric --cipher-algo AES256` komutunun yazdığının aynısıdır. `gpg` desteklenen her
sistemin resmi deposunda vardır: Debian 12/13 ve Ubuntu 22.04/24.04 main ile Arch core'da
`gnupg` paketi. Çoğu zaman zaten kuruludur.

- **Parola cümlesini hedefi eklerken siz seçersiniz.** En az 12 karakter olmalıdır; ekran sizin
  için güçlü bir tane de üretebilir. **Bir yere yazın ve bu sunucudan uzakta saklayın.** O
  olmadan bu yedekleri kimse açamaz; sunucu kaybolursa CelikPanel de onu geri getiremez.
- **Panel parola cümlesini mühürlü saklar** (diğer panel kimlik bilgileri gibi, panelin gizli
  anahtarıyla AES-256-GCM). Böylece zamanlanmış yedekleri şifreleyebilir ve geri yüklemede
  çözebilir. Parola cümlesi bir daha gösterilmez, günlüklere yazılmaz ve hiçbir ekrana
  gönderilmez.
- **Bir hedefin parola cümlesi değiştirilemez,** çünkü oradaki yedekler ona bağlıdır. Yeni parola
  cümlesi için yeni bir hedef ekleyin.
- **Her hedef kısa bir denetim etiketi gösterir,** örneğin `3F2A-91C0`. Aynı etiket her yedeğin
  yan dosyasına da yazılır. Parola cümlesini açığa çıkarmadan bir dosyayı hangisinin açtığını
  söyler.

Depolamanın kimlik bilgileri de aynı biçimde mühürlenir: erişim anahtarı sırrı, SFTP parolası ya
da özel anahtar. Ekranlarda erişim anahtarı kimliğinin yalnız başı ve sonu görünür.

## SFTP: sunucunun anahtarı

**İlk bağlantıda bir kişinin onayı gerekir.** Panel SFTP sunucusunun anahtar parmak izini
gösterir, örneğin `SHA256:…`. Onaylamadan önce bunu sunucunun kendi parmak iziyle
karşılaştırın. SFTP sunucusunda şunu çalıştırın:

```sh
ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub
```

Yalnız ikisi aynıysa onaylayın. CelikPanel o anahtarı sabitler.

**Sunucu sonradan başka bir anahtar gösterirse hiçbir şey gönderilmez ve işlem durur.** Ekranda
"SFTP sunucusu başka bir anahtar gösteriyor" yazar. Sunucunun sahibine anahtarın gerçekten
değişip değişmediğini sorun. Yeni parmak izini yalnız değiştiyse hedefte onaylayın.

## Nerede saklanır

Yollar bucket klasörüne (önek) ya da SFTP klasörüne göredir:

```
<sunucu>/sites/<alanadı>-d<alan adı kimliği>/manual/<yedek>.cpbak.gpg
<sunucu>/sites/<alanadı>-d<alan adı kimliği>/manual/<yedek>.cpbak.gpg.json
<sunucu>/sites/<alanadı>-d<alan adı kimliği>/schedule-<n>/<yedek>.cpbak.gpg
```

- `<sunucu>` bu sunucunun adıdır ve ilk uzak yedekte sabitlenir. Böylece birden çok sunucu aynı
  bucket'ı paylaşabilir.
- **`.json` yan dosyası** düz metindir. İçinde şunlar bulunur:
  - site, yedek türü ve oluşturulma zamanı;
  - şifreli ve çözülmüş dosyanın boyutları ve SHA-256 özetleri;
  - parola cümlesi denetim etiketi;
  - çözme komutu.

  Hiçbir gizli bilgi içermez.

## Denetimler

- **S3'e yükleme.** Her istek verisinin SHA-256 ve MD5 özetini taşır; bozuk veriyi depolamanın
  kendisi reddeder. Büyük yedekler en az 16 MiB'lik parçalarla gider. Başarısız bir parça
  bellekten yeniden gönderilir. Başarısız bir yükleme iptal edilir; yarım nesne kalmaz. Yüklemeden
  sonra boyut geri okunur. Depolama bir özet (ETag) veriyorsa gönderilenin MD5'iyle
  karşılaştırılır.
- **SFTP'ye yükleme.** Dosya geçici, gizli bir adla yazılır ve tümüyle geri okunur. Boyutu ve
  SHA-256 özeti karşılaştırılır; ancak ondan sonra asıl adına taşınır.
- **Geri yükleme.** Nesnenin tamamı indirilir ve şu yollarla denetlenir:
  - yan dosyadaki SHA-256 özetiyle;
  - OpenPGP dosyasının içindeki bütünlük denetimiyle;
  - çözülmüş dosyanın SHA-256 özetiyle.

  Ardından Agent, hiçbir şey geri yüklenmeden önce paketi bir kez daha denetler: boyutunu ve
  SHA-256 özetini, bu siteye ve aboneliğe ait olduğunu ve her parçasının SHA-256 özetini. Geri
  yükleme sonra yerel geri yüklemeyle aynı yoldan gider: önce o anki durumun güvenlik yedeği
  alınır ve disk kotası uygulanır.

## İşlemler ve size söyledikleri

- **Bir seferde tek işlem.** Her gönderme ve geri yükleme kalıcı kimliği (`rb-…`) olan bir
  işlemdir ve panel bunları sırayla çalıştırır.
- **İzlemek hiçbir şey başlatmaz.** Bir işlemi izleyen sayfa onu yalnız okur; ikinci bir yükleme
  başlatamaz. Aynı istek iki kez gelirse aynı işlem döner.
- **Yeniden başlatma aynı işlemi sürdürür.** Panel bir işlem sırasında yeniden başlarsa aynı
  işlem denetlenir ve sürdürülür.
- **Her işlem nedeni, kimin bir şey yapacağını, sonraki adımı ve işin nasıl süreceğini söyler
  (D-024).** Her hata şu sınıflardan birine girer:

  - **Eksik önkoşul.** Önce bir şeyin düzeltilmesi gerekir. Örneğin:
    - kimlik bilgileri reddedildi;
    - bucket'ta yetki yok;
    - bucket ya da klasör yok;
    - sunucu anahtarı değişti;
    - depolama dolu;
    - sunucu saati yanlış;
    - disk kotası doldu;
    - hedef kaldırıldı ya da bu site için kullanılamıyor.
  - **Doğrulanmış hata.** Örneğin:
    - bütünlük denetimi tutmadı;
    - parola cümlesi dosyayı açmıyor;
    - paket bu sunucuca reddedildi.
  - **Sonucu bilinmiyor.** Depolamaya ulaşılamadı ya da panel yeniden başladı. CelikPanel kendisi
    yeniden dener: en çok 3 deneme, aralar giderek uzar. Yeniden denerken son hata saklanır ve
    gösterilir.

## Zamanlama ve saklama

- **Bir zamanlamanın belirledikleri:** günlük ya da haftalık, dosya ya da tam yedek ve kaç kopya
  saklanacağı.
  - **N günlük sakla:** en yeni N günün her birinin en yeni kopyası.
  - **M haftalık sakla:** en yeni M takvim haftasının her birinin en yeni kopyası.
- **Hiç silinmeyenler:** en yeni kopya her zaman kalır; zamanı okunamayan kopya hiç silinmez.
- **Ayrı yerel kopya.** Bir zamanlama her çalışmada kendi yerel yedeğini alır, gönderir, denetler
  ve sonra o yerel kopyayı siler. Sizin yerel yedeklerinize ve onların zamanlamasına dokunmaz.
- **Tüm siteler zamanlaması (yönetici):** sunucudaki her site için aynı biçimde çalışır.

## CelikPanel olmadan geri yükleme

Gerekenler: `.cpbak.gpg` dosyası, `.json` yan dosyası (isteğe bağlı, denetim için), parola
cümlesi ve `gpg` ile `tar`. Bu ikisi dağıtımın resmi paketlerinden gelir (`gnupg`, `tar`).

Sunucudaki yerel bir yedek (`/var/backups/celikpanel/subscriptions/<abonelik>/domains/<alan adı no>/*.cpbak`,
yalnız root) şifreli değildir: 4. adımdan başlayın. Silinip aynı adla yeniden oluşturulan bir
sitenin yolu da budur: yeni sitenin alan adı numarası yenidir; panel yalnız onun kendi
yedeklerini listeler ve önceki sitenin paketini başka bir sitenin yedeği olarak reddeder.

1. **İki dosyayı bucket'tan ya da SFTP klasöründen indirin.** İstediğiniz aracı kullanabilirsiniz:
   sağlayıcının web konsolu, `sftp`, `scp` ya da zaten kullanıyorsanız AWS CLI.
2. **İsteğe bağlı: indirmeyi denetleyin.** Çıktıyı `.json` dosyasındaki `encrypted_sha256` ile
   karşılaştırın:

   ```sh
   sha256sum full-20261103T020000.000000000Z-0123456789abcdef.cpbak.gpg
   ```

3. **Çözün.** Parola cümlesini yalnız sizin okuyabileceğiniz bir dosyaya yazın, sonra:

   ```sh
   install -m 600 /dev/null pass.txt && nano pass.txt
   gpg --batch --pinentry-mode loopback --passphrase-file pass.txt \
       --output site.cpbak --decrypt full-20261103T020000.000000000Z-0123456789abcdef.cpbak.gpg
   shred -u pass.txt
   ```

   Parola cümlesi yanlışsa ya da dosya bozulmuş veya değiştirilmişse `gpg` hata vererek durur.
   İsterseniz `sha256sum site.cpbak` çıktısını `plain_sha256` ile karşılaştırın.
4. **Paketi açın:**

   ```sh
   mkdir restore && tar -xzf site.cpbak -C restore
   ```

   İçinden şunlar çıkar:
   - `manifest.json`: paketin içeriği ve her parçanın SHA-256 özeti.
   - `payload/files.tar.gz`: sitenin dosyaları (dosya ve tam yedeklerde).
   - `payload/databases/<kimlik>.sql.gz`: her veritabanı için sıkıştırılmış bir döküm (veritabanı
     ve tam yedeklerde).
   - `payload/home.tar.gz` (yalnız `manifest.json` içinde `"version": 3` yazıyorsa): site ev
     dizinindeki program ya da Python uygulaması klasörü `app` ve her hizmet uygulamasının
     klasörü `apps/<uygulama>`; `manifest.json` bunları `areas` altında listeler (dosya ve tam
     yedeklerde).
5. **Dosyaları sitenin belge köküne geri koyun** (örnek yol):

   ```sh
   tar -xzf restore/payload/files.tar.gz -C /var/www/example.com/public_html
   ```

   **Program ve uygulama klasörleri** (yalnız sürüm 3): önce programı ya da uygulamayı
   durdurun ve şimdiki klasörünü kenara taşıyın; sonra arşivi site ev dizinine (`public_html`
   klasörünü içeren klasör) açın ve dosyaları sitenin kendi hesabına ve o hesabın kendi
   grubuna verin, çünkü arşiv sahip bilgisi tutmaz (her `apps/<uygulama>` klasörü için de
   aynısı). Ev dizininin grubunu asla vermeyin: o grup web sunucusunundur ve web sunucusu
   programın klasörüne ulaşmamalıdır. İki noktalı `chown "$owner:"` hesabın kendi grubunu
   alır. Örnek yol:

   ```sh
   site=/var/www/celikpanel/subscriptions/3/sites/7
   tar -xzf restore/payload/home.tar.gz -C "$site"
   owner=$(stat -c %U "$site")
   chown -hR "$owner:" "$site/app"
   ```

   Ortam dosyaları (gizli değerler) hiçbir yedekte yoktur: programın okuduğu yere yeniden girin.

6. **Her veritabanını yükleyin.**
   - MariaDB ya da MySQL:

     ```sh
     gunzip -c restore/payload/databases/12.sql.gz | mariadb veritabani_adi
     ```

   - PostgreSQL:

     ```sh
     gunzip -c restore/payload/databases/12.sql.gz | psql veritabani_adi
     ```

   Her kimliğin veritabanı adı `manifest.json` dosyasında yazar.

Bu adımlar `gpg` ve `tar` bulunan her sunucuda çalışır; CelikPanel olsun ya da olmasın.

## Ayarlar

- **S3:**
  - uç nokta, örneğin:
    - `https://s3.eu-central-1.amazonaws.com`
    - `https://s3.us-west-004.backblazeb2.com`
    - `https://s3.eu-central-1.wasabisys.com`
    - `https://<hesap>.r2.cloudflarestorage.com`
    - MinIO adresiniz
  - bölge: boşsa `us-east-1`; R2 için `auto`.
  - bucket.
  - klasör (önek).
  - erişim anahtarı kimliği ve sırrı.
  - varsayılan olarak yol biçimli (path-style) adresleme; istenirse sanal makine biçimli
    (virtual-hosted).

  Yalnız `https://` kabul edilir. Anahtarın bucket'ta şu yetkileri olmalıdır: nesne yazma, okuma,
  listeleme ve silme; çok parçalı yüklemeyi başlatma, tamamlama ve iptal etme. Sağlayıcıda,
  bitmemiş çok parçalı yüklemeleri birkaç gün sonra silen bir yaşam döngüsü kuralı iyi bir ek
  güvencedir.
- **SFTP:**
  - sunucu ve kapı (port).
  - kullanıcı.
  - parola ya da özel anahtar (şifresiz PEM).
  - var olan, tam yolu yazılmış bir klasör.

  Panel alt klasörleri bu klasörün içinde kendisi açar.

## Açık konular

Şunlar yapılmadı:

- artımlı ya da tekilleştirilmiş yedek;
- yeniden başlatmadan sonra yüklemeyi kaldığı yerden sürdürme;
- panel üzerinden yeniden kurulmuş bir sunucuya ya da başka bir siteye geri yükleme;
- yöneticinin sunulan bir hedefi hangi hesapların göreceğini seçmesi (bugün tüm hesaplar görür);
- bayinin kendi hedefini müşterilerine sunması;
- yerel ağdaki MinIO için düz `http://` uç noktaları ve özel sertifika yetkilileri.
