---
id: mail
title: E-posta ve web postası
roles: [admin, reseller, customer]
topics: [domain.mail#mailboxes]
keywords: [e-posta, posta kutusu, kota, dolu, yönlendirme, yönlendirici, web postası, webmail, roundcube, imap, smtp, outlook, thunderbird, otomatik yanıt, catch-all]
---

Posta kutuları bir alan adına aittir. Alan adını, sonra **E-posta** bölümünü açın. Bölüm, sunucuda posta hizmeti varsa görünür. Planınız e-postayı kapsamıyorsa soluk görünür ve kimin ekleyebileceğini söyler.

## Posta kutuları {#mailboxes}

**Hesaplar** sekmesinde **Adres oluştur** düğmesine basın:

1. `@` işaretinden önceki kısmı **Kullanıcı adı** olarak yazın.
2. Bir **Parola** belirleyin.
3. Posta kutusunun boyutunu **Kota (MB)** olarak girin. 1024 ile başlar.
4. **Oluştur** düğmesine basın.

Her satır posta kutusunun **Kullanım** ve **Kota** bilgisini gösterir. Yanındaki düğmelerle parolayı değiştirir (en az 8 karakter), **Kotayı düzenle** ile boyutu değiştirir, otomatik yanıt ayarlar ya da posta kutusunu silersiniz.

## Posta kutusu dolunca {#full}

- **%90** dolulukta posta kutusuna Türkçe ve İngilizce tek bir uyarı iletisi gelir.
- **%100** dolulukta yeni postalar reddedilir; gönderen "posta kutusu dolu" iadesi alır. Kutudaki hiçbir ileti kaybolmaz.

Kim yapar: posta kutusunun sahibi eski iletileri siler ya da siz **Kotayı düzenle** ile boyutu büyütürsünüz. Posta hemen yeniden kabul edilir. Bunu posta sunucusunun kendisi yapar; panel dururken de çalışır.

E-posta bölümü Dovecot kota eklentisinin etkin olmadığını söylüyorsa boyutlar kayıtlıdır ama henüz uygulanmaz; eklentiyi yönetici açar.

## Yönlendirme {#forwarding}

**Yönlendirme** sekmesinde **Yönlendirici ekle** düğmesine basın, **Kaynak adres** ve **Hedef adres** yazıp **Oluştur** düğmesine basın. Kaynağa gelen posta hedefe teslim edilir.

**Ayarlar** sekmesindeki **Catch-all adresi**, alan adınızda olmayan bir ada gelen her iletiyi tek bir posta kutusuna teslim eder. Spam de toplar.

## Otomatik yanıt {#autoreply}

Posta kutusunun otomatik yanıt düğmesini açın, **Otomatik yanıt gönder** kutusunu işaretleyin, **Konu** ve **İleti** yazın; isterseniz **İlk gün** ve **Son gün** seçin ve **Yanıtı kaydet** düğmesine basın. Yanıtı posta sunucusu kendisi gönderir; aynı gönderene belirlediğiniz gün sayısında en çok bir kez yanıt gider, posta listelerine hiç gitmez.

## Web postası {#webmail}

E-posta bölümündeki **Web posta** kutusunda **Webmail’i Aç** düğmesi vardır. Tam e-posta adresi ve posta kutusunun parolasıyla giriş yapın; panel hesabı gerekmez. Web postasının bu sunucuda henüz kullanılamadığı yazıyorsa yönetici Roundcube'u kurar ve ona bir adres verir.

## Posta programı ayarları {#mail-programs}

**Ayarlar** sekmesindeki **Posta istemcisi kurulumu** Thunderbird, Outlook, Apple Mail ya da telefon için gerekenleri gösterir. `example.com` için değerler:

| | Sunucu | Port | Güvenlik |
|---|---|---|---|
| Gelen (IMAP) | `mail.example.com` | 993 | SSL/TLS |
| Gelen (POP3) | `mail.example.com` | 995 | SSL/TLS |
| Giden (SMTP) | `mail.example.com` | 587 | STARTTLS |

**Kullanıcı adı** her zaman tam e-posta adresidir, örneğin `ayse@example.com`; parola posta kutusunun parolasıdır. Giden sunucu da aynı girişi ister.

## Spam'e düşen postalar {#authentication}

**Kimlik doğrulama** sekmesi SPF, DKIM ve DMARC kayıtlarını gösterir; bunlar postanızın gerçek olduğunu kanıtlar. DNS bu sunucudan sunuluyorsa sizin için yayımlanır. Başka bir sağlayıcıdaysa gösterilen kayıtları kopyalayıp oraya ekleyin.
