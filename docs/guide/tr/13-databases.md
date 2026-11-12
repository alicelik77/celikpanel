---
id: databases
title: Veritabanları
roles: [admin, reseller, customer]
topics: [domain.databases#domain-databases, databases#databases-page]
keywords: [veritabanı, mysql, mariadb, postgresql, kullanıcı, parola, phpmyadmin, phppgadmin, izin]
---

WordPress gibi bir site içeriğini veritabanında tutar. CelikPanel sunucuda MariaDB (MySQL) ve PostgreSQL veritabanları ve her biri için bir kullanıcı oluşturur.

## Alan adının Veritabanları bölümünde {#domain-databases}

En kısa yol alan adının kendisidir. Alan adını, sonra **Veritabanları** bölümünü açıp **Veritabanı oluştur** düğmesine basın:

1. Bir **Veritabanı adı** yazın. Panel adın önüne alan adını ekler: `example.com` ve `magaza` için tam ad `example_com_magaza` olur.
2. **Veritabanı motoru** seçin.
3. Bir **Parola** belirleyin.
4. **Veritabanı oluştur** düğmesine basın.

Aynı adın sonuna `_user` eklenmiş, tam yetkili bir kullanıcı da oluşturulur; burada `example_com_magaza_user`. Uygulamanız `localhost` adresine bu ad, kullanıcı ve parolayla bağlanır.

## Veritabanları sayfası {#databases-page}

Menüdeki **Veritabanları** sayfası hesabınızın bütün veritabanlarını ve kullanıcılarını **Veritabanları** ve **Kullanıcılar** sekmelerinde gösterir. **Veritabanı oluştur** şunları sorar:

- **Veritabanı adı**: yalnız harf, rakam ve alt çizgi;
- bağlı olduğu **Web sitesi** ya da **Web sitesi yok (bağımsız veritabanı)**. Bir siteye bağlı veritabanı site silinince onunla silinir;
- **Veritabanı kullanıcısı**: **Yeni kullanıcı** ya da **Var olan kullanıcı**. **Güçlü bir parola üret** sizin için parola yazar;
- **İzinler**: **ALL (tam erişim)** ya da **SELECT (yalnızca okuma)**.

**Kullanıcı oluştur** tek başına bir kullanıcı ekler. Bir kullanıcıya yeni parola vermek için anahtar düğmesini kullanın. Eski parolayı kullanan programlar yenisini alana dek bağlanamaz; CelikPanel'in kurduğu uygulama yenisini kendiliğinden alır.

## phpMyAdmin ve phpPgAdmin {#web-tools}

Alan adının **Veritabanları** bölümündeki **Veritabanı web araçları** kutusunda **phpMyAdmin aç** (MariaDB) ya da **phpPgAdmin aç** (PostgreSQL) düğmesi bulunur. Araç panel üzerinden açılır, internete hiçbir zaman açık değildir. Veritabanı kullanıcısı ve parolasıyla giriş yapın; yalnız kendi veritabanlarınızı görürsünüz.

Düğme aracın **Bileşenler** sayfasından kurulmasını söylüyorsa onu yönetici kurar.

## Parolalar {#passwords}

CelikPanel veritabanı parolalarını saklamaz ve yeniden gösteremez. Birini kaybettiyseniz kullanıcıya yeni parola verin ve onu uygulamanızın yapılandırma dosyasına yazın. Planınızdaki veritabanı sayısı ya da disk dolunca yeni veritabanı reddedilir; bkz. [Sınırlar ve askıya alma](guide:limits#disk).

:::warning
Veritabanını silmek bütün verisini siler. Önce alan adının **Yedekler** bölümünden yedek alın.
:::
