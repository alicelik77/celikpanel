---
id: databases
title: Veritabanları
roles: [admin, reseller, customer]
topics: [domain.databases#domain-databases, databases#databases-page]
keywords: [veritabanı, mysql, mariadb, postgresql, kullanıcı, parola, phpmyadmin, phppgadmin, izin]
---

WordPress gibi bir site içeriğini veritabanında tutar. CelikPanel sunucuda MariaDB (MySQL) ve PostgreSQL veritabanları ve her biri için bir kullanıcı oluşturur. MariaDB yerine çalışan yerel bir MySQL sunucusu yönetilmez: CelikPanel onun veritabanı hesaplarını değiştirmez; bunu sunucunun yöneticisi sunucunun kendisinde yapar.

## Alan adının Veritabanları bölümünde {#domain-databases}

En kısa yol alan adının kendisidir. Alan adını, sonra **Veritabanları** bölümünü açıp **Veritabanı oluştur** düğmesine basın:

1. Bir **Veritabanı adı** yazın. Panel adın önüne alan adını ekler: `example.com` ve `magaza` için tam ad `example_com_magaza` olur.
2. **Veritabanı motoru** seçin.
3. Bir **Parola** belirleyin.
4. **Veritabanı oluştur** düğmesine basın.

Aynı adın sonuna `_user` eklenmiş, tam yetkili bir kullanıcı da oluşturulur; burada `example_com_magaza_user`. Uygulamanız `localhost` adresine bu ad, kullanıcı ve parolayla bağlanır.

## Veritabanları sayfası {#databases-page}

Menüdeki **Veritabanları** sayfası görebildiğiniz her veritabanını listeler: yönetici sunucudaki bütün veritabanlarını **Sahibi** ile, bayi müşterilerinin veritabanlarını, müşteri kendi veritabanlarını alan adına göre gruplanmış görür. Parolalar ve veritabanının içeriği burada hiç gösterilmez.

- **Aç**, veritabanının alan adına, **Veritabanları** bölümüne götürür.
- **Diğer › Yeni parola…** veritabanının kullanıcılarından birine yeni parola verir. Eski parolayı kullanan programlar yenisini alana dek bağlanamaz; CelikPanel'in kurduğu uygulama yenisini kendiliğinden alır.
- **Diğer › Sil…** adını yazdığınızda veritabanını bütün verisiyle siler. Kullanıcıları silinmez; veritabanını alan adının **Veritabanları** bölümünden silerseniz onunla birlikte açılan kullanıcı da silinir.
- **Veritabanı ekle** **Hangi alan adı için?** diye sorar ve o alan adının **Veritabanları** bölümünü formuyla açar.

Sunucunun kendi işleri bu sayfada değildir. Yönetici dışarıdan bağlantıları **Güvenlik** sayfasında, **Dışarıya açık olanlar** altında açıp kapatır; CelikPanel'in MariaDB'ye ya da PostgreSQL'e bağlandığı hesabı **Bileşenler**'de o hizmetin sayfasında bulur.

## phpMyAdmin ve phpPgAdmin {#web-tools}

Alan adının **Veritabanları** bölümündeki **Veritabanı web araçları** kutusunda **phpMyAdmin aç** (MariaDB) ya da **phpPgAdmin aç** (PostgreSQL) düğmesi bulunur. Araç panel üzerinden açılır, internete hiçbir zaman açık değildir. Veritabanı kullanıcısı ve parolasıyla giriş yapın; yalnız kendi veritabanlarınızı görürsünüz.

Düğme aracın **Bileşenler** sayfasından kurulmasını söylüyorsa onu yönetici kurar.

## Parolalar {#passwords}

CelikPanel veritabanı parolalarını saklamaz ve yeniden gösteremez. Birini kaybettiyseniz kullanıcıya yeni parola verin ve onu uygulamanızın yapılandırma dosyasına yazın. Planınızdaki veritabanı sayısı ya da disk dolunca yeni veritabanı reddedilir; bkz. [Sınırlar ve askıya alma](guide:limits#disk).

:::warning
Veritabanını silmek bütün verisini siler. Önce alan adının **Yedekler** bölümünden yedek alın.
:::

## Tabloları olan kullanıcı {#user-owns-objects}

PostgreSQL'de bir veritabanına eklediğiniz kullanıcıyı kaldırmak reddedilebilir: kullanıcının bu veritabanında tabloları ya da başka nesneleri vardır, bu yüzden kaldırılmamış ve hiçbir şey değiştirilmemiştir. Bu, CelikPanel'in kendi veritabanı hesabına ait veritabanlarında olur; o hesap bir kullanıcının yaptıklarını hiçbir zaman devralmaz.

Sunucunun yöneticisi önce bu nesneleri sunucuda tek bir komutla veritabanının kendi kullanıcısına, yani veritabanıyla birlikte açılan kullanıcıya (`example_com_magaza` için `example_com_magaza_user`) devreder, sonra kullanıcıyı panelden yeniden kaldırır. `example_com_magaza_okur` kullanıcısı için:

```
sudo -u postgres psql -d example_com_magaza -c 'REASSIGN OWNED BY "example_com_magaza_okur" TO "example_com_magaza_user";'
```

Müşteri ya da bayi bunu sunucunun yöneticisinden ister.
