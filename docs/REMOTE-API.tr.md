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

   Jeton (`cpk_…`) **bir kez** gösterilir. O anda kopyalayın ve bir parola
   gibi saklayın. CelikPanel yalnız SHA-256 özetini tutar.

Ek kullanıcılar jeton sahibi olamaz. "Farklı giriş yap" ile girilmişken
jeton oluşturulamaz.

## Kullanmak

Jetonu yalnız `Authorization: Bearer <jeton>` başlığıyla gönderin. Jeton
sorgu dizesinde asla kabul edilmez. Hem oturum çerezi hem jeton taşıyan
istek reddedilir.

Örnekler ve rota listesi İngilizce sayfadadır: [REMOTE-API.md](REMOTE-API.md).

**Yalnız okuma** jetonu şunları okuyabilir:

- alan adı listesi;
- barındırma ayarları;
- kullanım;
- DNS kayıtları;
- posta kutuları;
- veritabanları;
- yedekler.

**Okuma ve değiştirme** jetonu bunlara ek olarak şunları yapabilir:

- alan adı ekleyip silebilir;
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

Hesabın kendi hakları bu listenin üstünde yine geçerlidir. Bir müşterinin
jetonu başka bir hesabın alan adına asla ulaşmaz.

## Sınırlar ve kayıt

- Bir jeton dakikada en çok 120 istek gönderebilir.
- Bir adresten dakikada en çok 10 hatalı jeton girişi kabul edilir. Sonra o
  adres dakikanın geri kalanında reddedilir.
- Jeton oluşturmak, kaldırmak ve jetonla yapılan her değişiklik denetim
  kaydına yazılır. Kayıtta jetonun açık kimliği (`cpk_<id>`) bulunur, gizli
  kısmı asla.
- Jetonun son kullanımı (zaman ve adres) Hesabım'da görünür. Dakikada en
  çok bir kez yazılır.
- Parola sıfırlama ya da değişikliği, iki faktörlü giriş değişikliği ya da
  hesap değişikliği o hesabın bütün jetonlarını bitirir. Hesabın oturumları
  da aynı biçimde biter.
- Sunucu anahtarını kapatmak her jetonu hemen durdurur.
