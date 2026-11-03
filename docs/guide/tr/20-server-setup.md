---
id: server-setup
title: Sunucunun ilk kurulumu
roles: [admin]
topics: [server-setup]
keywords: [sihirbaz, kurulum, ilk sunucu, katılma kodu, ikinci sunucu, ad sunucusu, roller]
---

Kurulum sihirbazı yeni bir sunucuyu web siteleri, e-posta ve DNS için hazırlar. Önce planını gösterir; siz başlatana kadar hiçbir şeyi değiştirmez. Sunucudaki mevcut veriler korunur.

## Sihirbazla ya da elle {#choice}

İlk girişte panel sunucunun nasıl hazırlanacağını sorar. Olağan yol **Sihirbazla kur** seçeneğidir. Kendiniz yapılandırmayı seçerseniz sihirbazı sonradan **Sunucu kurulumu** sayfasından açabilirsiniz.

## Adımlar {#steps}

1. **Bu sunucu:** **İlk sunucu** ya da ikinci bir sunucu için **Mevcut bir sunucuya katıl**. İlk sunucu ana alan adınızı sorar.
2. **Sunucu sayısı:** **Bir sunucu** ya da **İki sunucu**. İki sunucuda ikincisi DNS'in bir kopyasını tutar.
3. **Ne yapacak?** **Web siteleri**, **E-posta** ve **DNS (alan adı listesi)** arasından seçin. Her biri neleri kuracağını gösterir. SSH ve panel erişimini açık tutan bir güvenlik duvarı ile sertifika yenileme her zaman kurulur.
4. **Adlar:** panelin adresi ve e-posta sunucusunun adresi.
5. **DNS listesi nerede:** bkz. [DNS seçenekleri](#dns).
6. **Özet:** nelerin kurulacağı ve nelere dokunulmayacağı. Onay kutusunu işaretleyip **İncelenen kurulumu başlat** düğmesine basın.

Başka adımlar yalnız gerektiğinde görünür; örneğin devralınacak mevcut kayıtlar. Kurulumu engelleyen bir şey varsa en başta, düzeltme ya da yeniden kontrol düğmesiyle birlikte yazar.

## DNS seçenekleri {#dns}

| Seçenek | Ne olur |
|---|---|
| **Kendi sunucularımda** | Bu sunucu BIND ya da PowerDNS çalıştırır. İki sunucuda biri ana DNS sunucusu olur, diğeri kopyasını tutar. |
| **Harici bir DNS sağlayıcısında** | DNS yazılımı kurulmaz. Panel, sağlayıcınızda eklenecek kayıtları gösterir; sağlayıcıya kendisi yazmaz. |
| **Başka bir CelikPanel’de** | Alan adı listesi başka bir panelde durur; tek kullanımlık bağlantı koduyla bağlanılır. |

## Kurulum sürerken {#progress}

Her adım **Bekliyor**, **Devam ediyor**, **Tamamlandı**, **Başarısız** ya da **Sizi bekliyor** gösterir. Panel kendi adresini güvenceye alırken bir kez yeniden başlar. Sayfayı kapatabilirsiniz; kurulum sunucuda sürer.

**Alan adı firmanızda girilecek bilgiler**, ad sunucularını ve onların IP kayıtlarını (glue) listeler. Bunları alan adı firmanızda yalnız siz girebilirsiniz; internet onları görünce kart kendiliğinden doğruya döner.

## İkinci sunucuyu bağlamak {#join}

1. İlk sunucuda **Katılma kodu oluştur** düğmesine basın. Kod bir kez gösterilir ve süresi dolar.
2. İkinci sunucuda **Mevcut bir sunucuya katıl** seçeneğini seçin, ilk sunucunun adresini ve kodu yazıp **Katıl** düğmesine basın.

## Kurulum bitince {#done}

**Sunucunuz hazır** ekranı size kalan işleri, her birinin düğmesiyle listeler. Ardından ilk web sitenizi ekleyin. **Kurulumu genişlet** sonradan yeni görevler ekler; hiçbir şeyi kaldırmaz. DNS'in ayrıntıları: [DNS ve eş sunucular](guide:dns-servers).
