---
id: files-access
title: Dosyalar: dosya yöneticisi, FTP, SFTP ve SSH
roles: [customer]
topics: [domain.files#file-manager, domain.access#ftp-sftp]
keywords: [ftp, sftp, ssh, filezilla, yükleme, indirme, public_html, anahtar, komut satırı, terminal]
---

Sitenizin dosyalarına üç yoldan ulaşırsınız: paneldeki dosya yöneticisi, FTP ya da SFTP üzerinden FileZilla gibi bir program ve SSH üzerinden bir komut satırı. Üçü de yalnız kendi aboneliğinizin klasörünü görür; sunucunun geri kalanı kapalı kalır.

## Nerede bulunur {#where}

**Alan Adları** sayfasını açın ve alan adını seçin. Sekmelerinden ikisi dosyalarla ilgilidir:

- **Dosyalar** dosya yöneticisidir.
- **Erişim** FTP, SFTP ve SSH'yi açar ve nasıl bağlanacağınızı gösterir.

Web sitenizin dosyaları `public_html` klasöründedir. Bir ekip üyesi (kullanıcı), müşteri kendisine açtıysa **Dosyalar** sekmesini görür; **Erişim** sekmesini hiçbir zaman görmez.

## Dosya yöneticisi {#file-manager}

**Dosyalar** sekmesi sitenin klasörlerini ve dosyalarını listeler. Burada:

- bir klasörü açar, **Yukarı git** ile geri dönersiniz;
- **Yükle** ile bilgisayarınızdan dosya yüklersiniz;
- **Yeni klasör** ya da **Yeni dosya** oluşturursunuz;
- **Düzenle** ile bir ayar dosyası gibi 1 MB'a kadar metin dosyalarını açıp kaydedersiniz;
- **İndir** ile bir dosyayı indirir, **Sil** ile silersiniz.

Resimler, arşivler ve diğer ikili (binary) dosyalar yüklenip indirilebilir ama panelde düzenlenemez. Çok sayıda dosya için FTP ya da SFTP daha hızlıdır.

:::warning
Silinen dosya geri gelmez. Önemli bir şeyi değiştirmeden ya da silmeden önce bir kopyasını indirin ya da alan adının **Barındırma** sekmesinde **Yedekler** altından yedek alın.
:::

## FTP ve SFTP {#ftp-sftp}

**Erişim** sekmesinde:

1. **Dosya erişimi (FTP ve SFTP)** seçeneğini açın.
2. **Parola belirleyin** altında 8 ile 128 karakter arasında bir parola seçin ya da bir SSH anahtarı ekleyin (bkz. [SSH anahtarları](#keys)).
3. **Değişikliği gözden geçir** düğmesine basın. Panel sunucuda neyin değişeceğini gösterir.
4. **Değişikliği uygula** düğmesine basın.

Ardından **FileZilla ile bağlanın** kutusu sunucu adınızı, kullanıcı adınızı ve klasörünüzü adım adım gösterir. FTP her zaman şifrelidir (TLS üzerinden FTP); düz FTP sunulmaz. SFTP aynı kullanıcı adını ve parolayı kullanır.

CelikPanel parolanızı saklamaz ve size bir daha gösteremez. Unutursanız aynı yerden yenisini belirleyin.

:::note
FTP ile ilk bağlandığınızda FileZilla sunucunun sertifikasına güvenip güvenmeyeceğinizi sorabilir. Yalnız SHA-256 parmak izi **Erişim** sekmesinde yazanla aynıysa güvenin.
:::

## SSH anahtarları {#keys}

Anahtar, paroladan daha güvenlidir. **SSH anahtarları** altına tek bir açık anahtar, yani `.pub` dosyanızın içeriğini yapıştırın; örneğin:

```
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA… siz@dizustu
```

Özel anahtarı, yani `.pub` uzantısı olmayan dosyayı asla yapıştırmayın. Bazı sunucular SFTP ve SSH için yalnız anahtar kabul eder; böyle olduğunda **Erişim** sekmesi bunu söyler ve FTP yine parolayı kullanır.

## Komut satırı (SSH) {#ssh}

**Komut satırı (SSH)**, aboneliğinizin klasörüne kapatılmış ve yalnız temel dosya araçlarını içeren bir terminal açar. Yalnız gerekiyorsa açın; dosya erişimiyle aynı yoldan: işaretleyin, sonra **Değişikliği gözden geçir** ve **Değişikliği uygula**. Aynı kullanıcı adıyla, parolanız ya da anahtarınızla bağlanın:

```
ssh -p 22 kullanici@sunucu.example.com
```

**Erişim** sekmesinin gösterdiği sunucu adını, portu ve kullanıcı adını kullanın; bu örnekten farklı olabilirler. **Komut satırı (SSH)** aboneliğinizin planında yoksa sekme bunu söyler. Dosya erişimi bundan etkilenmez; sağlayıcınız komut satırını planınıza ekleyebilir.

## SSH henüz açılamıyorsa {#ssh-tools}

Komut satırı araçlarını sunucudan alır. Sunucuda bunların bir kısmı yoksa **Erişim** sekmesi hangilerinin eksik olduğunu söyler ve SSH seçeneği onlar kurulana kadar görünmez. Bu sırada FTP ve SFTP çalışmaya devam eder.

- **Müşteriler:** araçları sunucu yöneticisinin kurması gerekir. Sağlayıcınıza yazın; araçlar kurulunca seçenek sekmede görünür.
- **Yönetici**, eksik araçları resmî paket adlarıyla ve **Araçları kur** düğmesiyle görür. Araçlar kurulur kurulmaz seçenek görünür.

Komut satırı zaten açık olan bir hesap, bu arada eksik araçlar olmadan komut satırını kullanmaya devam eder.

## Sık görülen iletiler {#messages}

| Sekmenin söylediği | Ne yapmalı |
|---|---|
| Dosya erişimi bu aboneliğin planında yok | Dosya yöneticisini kullanın ya da sağlayıcınızdan dosya erişimini eklemesini isteyin. |
| Bu abonelik etkin değil | Erişim şimdilik yalnız kapatılabilir. Aboneliği sağlayıcınız yeniler. |
| Bu sunucu yalnız SSH anahtarı kabul ediyor | **SSH anahtarları** altına bir anahtar ekleyin. FTP yine parolanızı kullanır. |
| Bu sunucuda FTP kurulu değil | SFTP kullanın; aynı kullanıcı adı ve parolayla çalışır. |
| Bu alan adının web sitesi dosyaları yok | Önce **Barındırma** sekmesinde alan adına bir web sitesi verin. |

Başka bir ileti görürseniz onu [Durumu okumak ve yardım almak](guide:reading-states#problem) bölümünün anlattığı gibi okuyun: kimin bir şey yapması gerektiğini ve sıradaki adımı söyler.
