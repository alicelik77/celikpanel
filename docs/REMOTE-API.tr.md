# Uzak API

*alpha.128 (G15) · [English](REMOTE-API.md)*

Uzak API, bir hesabın kendi programlarının ve betiklerinin tarayıcı olmadan o
hesapla çalışmasını sağlar. Alan adlarını, DNS kayıtlarını, posta kutularını,
veritabanlarını ve yedekleri listeleyip değiştirebilirler. Bu, isteğe bağlı
panel otomasyonudur (D-022). Barındırılan hizmetler ona hiç bağlı değildir;
panel dursa da çalışmayı sürdürürler.

## Açmak

1. **Sunucu anahtarı.** Uzak API yeni bir sunucuda **kapalıdır**. Yönetici
   **Ayarlar → Panel → Uzak API** bölümünden açar. Kapalıyken hiçbir jeton
   çalışmaz. Jetonlar saklanır ve yeniden açılınca yine çalışır.
2. **Plan izni `remote_api`** (plan haklarında "Uzak API"). Yöneticide her
   zaman vardır. Bayi için yöneticinin sunması ve bayinin kendi haklarında
   bulunması gerekir. Müşteri için hesabın **her** aboneliğinde bulunması
   gerekir, çünkü jeton bütün hesap adına çalışır. "Tüm izinler" içeren
   planlar (Sınırsız planlar) bu izni içerir. Bir hesabı API dışında tutmak
   için izni planına koymayın.
3. **Jeton.** **Hesabım → API jetonları** bölümünde şunları seçin:
   - bir ad;
   - bir kapsam: **Yalnız okuma** ya da **Okuma ve değiştirme**;
   - bir bitiş: 7, 30, 90 ya da 365 gün. Her jetonun bir sonu vardır.

   Jeton oluştururken geçerli parolanız yeniden sorulur. İki faktörlü giriş
   açıksa iki faktörlü kodunuz da sorulur. Jeton (`cpk_…`) **bir kez**
   gösterilir. O anda kopyalayın ve bir parola
   gibi saklayın. CelikPanel yalnız SHA-256 özetini tutar.

Ek kullanıcılar jeton sahibi olamaz. "Farklı giriş yap" ile girilmişken
jeton oluşturulamaz.

## Kullanmak

Jetonu yalnız `Authorization: Bearer <jeton>` başlığıyla gönderin. Jeton
sorgu dizesinde asla kabul edilmez. Hem oturum çerezi hem jeton taşıyan
istek reddedilir.

Yalnız `Authorization: Bearer cpk_…` biçimindeki başlık jeton sayılır. Başka
her `Authorization` başlığını jeton katmanı yok sayar. Panelin önündeki bir
kimlik vekilinin taşıyıcı jetonu ya da bir vekilin Basic girişi buna
örnektir. Panel o başlık hiç yokmuş gibi davranır.

Örnekler ve rota listesi İngilizce sayfadadır: [REMOTE-API.md](REMOTE-API.md).

**Yalnız okuma** jetonu şunları okuyabilir:

- alan adı listesi;
- barındırma ayarları;
- kullanım;
- ziyaretçi istatistikleri;
- DNS kayıtları;
- posta kutuları;
- veritabanları;
- yedekler.

**Okuma ve değiştirme** jetonu bunlara ek olarak şunları yapabilir:

- alan adı ekleyip silebilir; silerken `confirm_domain` parametresi alan
  adının kendisi olmalıdır, yoksa `400 API_TOKEN_CONFIRM_DOMAIN` döner ve
  hiçbir şey silinmez;
- DNS kayıtlarını ekleyip değiştirip silebilir;
- posta kutusu ekleyip silebilir, kotasını ve parolasını değiştirebilir;
- veritabanı ekleyip silebilir;
- yedek alıp silebilir.

Başka her adres `403 API_TOKEN_ROUTE_NOT_ALLOWED` yanıtını verir. Bunlara
şunlar da dahildir:

- jeton yönetimi;
- parola, iki faktörlü giriş ve e-posta değişikliği;
- gizli bilgi gösteren her rota;
- VPN, lisans, kurulum ve güncelleme;
- her sunucu ve yönetici rotası.

**Posta kutusu parolaları.** Okuma ve değiştirme jetonu bir posta kutusunun
parolasını değiştirebilir. Parolayı değiştirmek o posta kutusuna erişim
demektir. Bu yüzden böyle bir jeton, hesabın her posta kutusuna erişim kadar
güçlüdür. Jetonla yapılan her değişiklik, jetonun açık kimliğiyle denetim
kaydına yazılır. Değişiklik yapması gerekmeyen betiklere Yalnız okuma jetonu
verin.

Hesabın kendi hakları bu listenin üstünde yine geçerlidir. Bir müşterinin
jetonu başka bir hesabın alan adına asla ulaşmaz.

## Sınırlar ve kayıt

- Bir jeton dakikada en çok 120 istek gönderebilir.
- Bir adresten dakikada en çok 10 hatalı jeton girişi kabul edilir. Sonra o
  adresten gelen ve jetonu doğrulanmayan istekler dakikanın geri kalanında
  reddedilir. Aynı adresten gelen doğru jeton çalışmayı sürdürür; tek bir
  ortak adresin arkasındaki bir ofis buna örnektir.
- Uzak API kapalıyken çalışmayan bir jeton, geçersiz jetonla aynı yanıtı
  alır. Böylece çalışan jetonu olmayan biri API'nin açık olup olmadığını
  öğrenemez.
- Başarısız jeton girişleri denetim kaydına yazılır. Her jeton kimliği ve
  adres için dakikada en çok bir satır yazılır ve satırda neden bulunur.
- Jeton oluşturmak, kaldırmak ve jetonla yapılan her değişiklik denetim
  kaydına yazılır. Kayıtta jetonun açık kimliği (`cpk_<id>`) bulunur, gizli
  kısmı asla.
- Jetonun son kullanımı (zaman ve adres) Hesabım'da görünür. Dakikada en
  çok bir kez yazılır.
- Şunlar o hesabın bütün jetonlarını bitirir:
  - parola sıfırlama ya da değişikliği;
  - hesabın e-posta adresinin değişmesi;
  - iki faktörlü giriş değişikliği;
  - yöneticinin hesabı değiştirmesi.
- Sunucu anahtarını kapatmak her jetonu hemen durdurur.
