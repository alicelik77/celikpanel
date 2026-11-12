---
id: getting-started
title: Başlarken
roles: [admin, reseller, customer]
topics: [dashboard, account#your-account, help#help]
keywords: [giriş, oturum açma, menü, ilk adımlar, dil, tema, parola, iki faktörlü]
---

CelikPanel web sitelerinizi, e-postanızı, alan adlarınızı ve veritabanlarınızı bir sunucuda çalıştırır. Bu bölüm ekranda neyin nerede olduğunu, rolünüzün size neler yaptırdığını ve nereden başlayacağınızı anlatır.

## Giriş yapmak {#signing-in}

Panelin adresini tarayıcınızda açın. Hesabınızı kim verdiyse bu adresi, **Kullanıcı adı** bilginizi ve bir parolayı ya da parola belirleme bağlantısını o gönderir.

- İki faktörlü doğrulamayı açtıysanız panel, parolanızdan sonra doğrulama uygulamanızdaki altı haneli kodu sorar.
- Parolanızı unuttuysanız giriş sayfasında **Parolamı unuttum** bağlantısına basın. Sıfırlama bağlantısı hesabınızdaki e-posta adresine gider.

:::note
CelikPanel parolanızı hiçbir zaman e-postayla ya da telefonla istemez. Parolanızı yalnız kendi panelinizin giriş sayfasına yazın.
:::

## Ekran {#the-screen}

- Soldaki **menü**, yönettiğiniz her şeyi **Barındırma** başlığı altında, yöneticiyse **Sunucu** başlığı altında da toplar. **Menüyü daralt** ile yalnız simgelere indirilebilir. Telefonda sol üstteki **Menü** düğmesiyle açılır.
- Ortadaki **sayfa** bir seferde tek bir şeyi gösterir: bir liste, bir alan adı, bir bileşen.
- **Sağ üstte** yardım düğmesi (**?**), dil, panelin görünümü ve adınızın yazdığı kişisel menünüz durur.

## Kim ne yapabilir {#roles}

Her hesabın tek bir rolü vardır. Menü yalnız rolünüzün kullanabildiklerini gösterir; ekran ne gösterirse göstersin sunucu her değişikliği ayrıca denetler.

| Rol | Yönettikleri |
|---|---|
| Yönetici | Sunucunun tamamı: bileşenler, güvenlik, güncellemeler, bütün hesaplar. |
| Bayi | Kendi müşterileri, sattığı planlar ve kendi siteleri. |
| Müşteri | Planının içinde kendi alan adları, siteleri, e-postası, veritabanları ve yedekleri. |
| Kullanıcı | Bir müşterinin eklediği ekip üyesi. Yalnız müşterinin kendisine açtığı alan adlarını ve bölümleri görür. |

Planınızda nelerin olduğunu, örneğin SSH kullanıp kullanamayacağınızı ya da kaç posta kutusu açabileceğinizi hesabınızı veren kişi ya da firma belirler.

## İlk adımlar {#first-steps}

- **Yönetici:** önce **Sunucu kurulumu** sayfasını bitirin, **Bileşenler** sayfasına bakın, sonra ilk alan adını ekleyin. Sunucu kurulumu ekranı neyin eksik kaldığını ve nedenini söyler.
- **Bayi:** ilk planınızı ve ilk müşterinizi oluşturmak için **Kullanıcılar** sayfasını açın.
- **Müşteri:** **Alan Adları** sayfasını açıp alan adınızı seçin ve bölümlerine bakın: web sitesi için **Site**, posta kutuları için **E-posta**, sitenin dosyaları için **Dosyalar**.
- **Kullanıcı:** **Alan Adları** sayfasını açın. Ekibinizin size açtığı alan adlarını orada görürsünüz.

## Hesabınız {#your-account}

Sağ üstteki kişisel menünüz **Hesabım** sayfasını açar. Parolanızı ve e-posta adresinizi orada değiştirir, iki faktörlü doğrulamayı orada açarsınız. Ekip üyeleri parolalarını aynı menüdeki **Parolayı değiştir** ile değiştirir. **Çıkış yap** menünün en altındadır.

## Yardım almak {#help}

- Sağ üstteki **?**, kılavuzu bulunduğunuz sayfayı anlatan yerden açar.
- Aynı menüdeki **Kullanım kılavuzu**, rolünüze ait bütün bölümleri arama kutusuyla birlikte gösterir.
- **Turu başlat**, ekranın ana bölümlerini birkaç adımda yeniden gösterir.

Bir ekran sizi durdurduğunda nedenini, kimin bir şey yapması gerektiğini ve sıradaki adımı söyler. Bu iletilerin nasıl okunacağını [Durumu okumak ve yardım almak](guide:reading-states) bölümü anlatır.
