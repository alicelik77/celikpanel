---
id: without-panel
title: Sunucuyu CelikPanel olmadan işletmek
roles: [admin]
topics: []
keywords: [bağımsızlık, yerel, systemd, nginx, postfix, bind, powerdns, cron, certbot, lisans, panel dışı]
---

CelikPanel sunucuyu sizin adınıza işletir; hizmetlerinizin sahibi değildir. Hizmetler, işletim sisteminin olağan yapılandırma dosyalarıyla çalışan sıradan servisleridir; onlara standart araçlarla kendiniz bakabilirsiniz.

## Çalışmaya devam edenler {#keeps-running}

Panel durduğunda, ulaşılamadığında ya da lisansını beklerken şunlar çalışır:

- web siteleri (nginx, PHP-FPM) ve Node.js uygulamaları;
- posta (Postfix, Dovecot) ve web postası;
- yedek DNS sunucusuna giden kopya dahil DNS (BIND ya da PowerDNS);
- veritabanları (MariaDB, PostgreSQL);
- her sitenin kendi sistem kullanıcısının sıradan crontab'ı olan zamanlanmış görevler;
- sistemin kendi certbot zamanlayıcısıyla site sertifikalarının yenilenmesi.

## Hâlâ panele bağlı olanlar {#needs-panel}

- panelin ve posta sunucusunun kendi sertifikalarının, eski sürümlerin aldığı site sertifikalarının da yeniden alınana kadar yenilenmesi;
- açılışta güvenlik duvarı kurallarının geri yüklenmesi;
- zamanlanmış uzak yedeklerin gönderilmesi.

CelikPanel'i bir sunucudan kaldırmak henüz desteklenmiyor. Dosyalarını elle silmeyin.

## Kendi yaptığınız değişiklikler {#own-changes}

CelikPanel kendi düzenlemelerinizi fark eder ve sessizce üzerine yazmaz:

- Bir sitenin yapılandırma dosyası, **Yapılandırma dosyası** sayfasında **CelikPanel dışında düzenlendi** olarak görünür: **CelikPanel’in metnini al**, **Benimkini koru** ya da **Elle birleştir** seçeneklerinden birini seçin.
- Sayfa okuduktan sonra değişen bir bileşen dosyasının üzerine yazılmaz; dosyayı yeniden yükleyip karar verin.
- Sunucuda değiştirilen DNS korunur (bkz. [DNS ve eş sunucular](guide:dns-servers#outside)).

Panelin kendisi iki servis olarak çalışır; bunları yeniden başlatmak barındırılan hizmetleri durdurmaz:

```
sudo systemctl status celikpanel-panel celikpanel-agent
```
