---
id: security
title: Güvenlik: güvenlik duvarı, Fail2ban, acil durumlar ve zararlı yazılım
roles: [admin]
topics: [security]
keywords: [güvenlik duvarı, nftables, fail2ban, yasak kaldırma, denetim, acil durum, geri çekme, clamav, zararlı yazılım, karantina, bekleme, güncellemeler]
---

**Güvenlik** sayfası güvenlik duvarını, sistemin güvenlik güncellemelerini, Fail2ban'ı ve salt okunur **Güvenlik denetimi** kartını gösterir.

## Güvenlik duvarı {#firewall}

Güvenlik duvarı gelen bağlantılarda panelin, SSH'nin ve çalışan servislerin portları dışındaki her portu kapatır; bir servis kurmak ya da kaldırmak bu portları kendisi değiştirir. **Aç** önce sorar, sonra hemen uygular. **Yeniden başlatma için kaydet** kuralların yeniden başlatmadan sonra da geçerli kalmasını sağlar. **Kapat** her portu sistemin varsayılanına döndürür.

SSH hiçbir zaman kapatılmaz. SSH kurulu olup dinlemiyorsa siz onu başlatana kadar açma reddedilir. SSH yoksa, paneli kaybederseniz sağlayıcınızın konsoluna ihtiyacınız olacağını onaylamanız istenir.

## Fail2ban ve denetim {#fail2ban}

Fail2ban, girişte sürekli başarısız olan adresleri engeller. **Fail2ban'ı yönet**, **Hapishaneler** ve **Banlı IP'ler** sekmelerini **Yasağı kaldır** düğmesiyle gösterir; yapılandırması burada okunabilir ama henüz düzenlenemez.

**Güvenlik denetimi** güvenlik duvarını, SSH girişini, bekleyen bir yeniden başlatmayı, güncelleme imzalarını ve panelin sertifikasını denetler. Yalnız okur; **Yeniden çalıştır** denetimi yineler.

## Güvenlik acil durumu {#emergency}

Bir PHP ya da Node.js sürümünde, MariaDB'de ya da PostgreSQL'de güvenlik açığı varsa ve henüz yaması yoksa onu **Bileşenler** sayfasında açın. **Güvenlik acil durumu araçları** en hafifinden başlayan dört adımdır:

1. Geri çekme (1. adım): kimse onu yeni bir iş için seçemez; onu kullanan siteler çalışmaya devam eder. Geri alınabilir.
2. **Acil durdurma (2. adım)**: durdurulur ve kilitlenir; etkilenen siteler bakım sayfası gösterir.
3. **Siteleri taşı (3. adım)**: siteler başka bir sürüme taşınır; taşınamayanlar listelenir.
4. **Acil kaldırma (4. adım)**: paket kaldırılır. Dosyalar, veritabanları ve posta kutuları kalır.

2–4. adımlar etkilenenleri listeler; bir neden, adın elle yazılmasını ve parolanızı ister. Müşteriler ve bayiler panelde ve e-postayla bilgilendirilir. Yama kurulunca **Yeniden aç (yama kuruldu)** her şeyi geri getirir. Hiçbir adım müşteri verisi silmez.

**Mağaza** üzerinden sunulan **Yapay zekâ ve otomasyon uygulamaları** için **Güvenlik beklemesi**, veriyi silmeden bütün kurulumları hemen durdurur. **Beklemeyi kaldır** sonrasında her sahip kendi uygulamasını yeniden başlatır.

## Zararlı yazılım taraması {#malware}

ClamAV'ı **Bileşenler** sayfasından kurun. Alan adının **Zararlı yazılım taraması** bölümü **Bu siteyi tara** ve **Aboneliğin tüm sitelerini tara** düğmelerini sunar; yönetici orada **Her siteyi haftada bir tara (tüm abonelikler, düşük öncelikle)** seçeneğini de işaretleyebilir.

Hiçbir şey kendiliğinden silinmez. **Karantinaya al** bulunan dosyayı kenara alır, **Geri koy** yerine döndürür, **Kalıcı olarak sil** yalnız karantinadan çalışır. Müşteri, bayisi ve yöneticiler bilgilendirilir. İmzalar 7 günden eskiyse "bir şey bulunmadı" tam bir temiz raporu değildir.

## Sistem güncellemeleri {#os-updates}

**Güvenlik güncellemeleri** bekleyen güncellemeleri listeler ve yalnız okur. Debian ve Ubuntu'da **Otomatik veya elle güncelleme**, yazacağı tek dosyayı önce gösterir; **Dosyayı yaz** ile onaylarsınız. Sunucuyu asla yeniden başlatmaz. Arch'ta bunun yerine adım adım bir rehber verir.
