---
id: recovery
title: Yönetici için kurtarmanın temelleri
roles: [admin]
topics: []
keywords: [kurtarma, bilinmiyor, takıldı, yanıt vermiyor, tekrar kontrol et, journalctl, geri alma, yardım]
---

Önce panelin kendi ekranlarını kullanın: nedeni, kimin bir şey yapacağını, sıradaki adımı ve işin nasıl süreceğini söylerler. Terminale yalnız bir ekran bunu söylediğinde ya da panele hiç ulaşılamadığında geçin.

## Sonuç bilinmediğinde {#unknown}

"Bilinmiyor", "başarısız" demek değildir: yanıt kaybolmuştur ve panel değişikliği yeniden göndermez.

1. **Tekrar kontrol et** düğmesine basın. Bu düğme yalnız okur.
2. Değişikliğin yapıldığını, yapılmadığını ya da hâlâ sürdüğünü gösterir.
3. Yalnız yapılmadıysa yeniden başlatın.

:::warning
Sonuç bilinmiyorken değişikliği başka bir sekmeden, terminalden ya da bir betikle yinelemeyin. Aynı anda iki değişiklik, ikisinin de istemediği bir durum bırakabilir.
:::

## Panel yanıt vermediğinde {#not-answering}

Panelin yanıt vermediğini söyleyen uyarı çoğunlukla kendiliğinden kalkar; örneğin bir güncelleme paneli yeniden başlatırken. Sürerse **Panel erişimini kontrol et** düğmesine basın. Barındırılan hizmetler çalışmaya devam eder (bkz. [Sunucuyu CelikPanel olmadan işletmek](guide:without-panel)); panelin kendi servislerinin çalışıp çalışmadığını `sudo systemctl status celikpanel-panel celikpanel-agent` gösterir.

Tamamlanamayan bir güncellemeden sonra **Güncelleme ve kurtarma durumu** ne olduğunu gösterir. Otomatik kurtarma duraklarsa aynı ekran, kaydına bakmak için tam ve salt okunur komutu verir; o komutu çalıştırın. Bkz. [Güncelleme tamamlanmazsa](guide:panel-settings#rollback).

## Elle değişiklik yapmadan önce {#by-hand}

- Önce **Teknik ayrıntılar** bölümünü okuyun; oradaki kod sorunu tam olarak adlandırır.
- Her seferinde tek bir şeyi değiştirin, sonra sayfanın durumu yeniden okumasını bekleyin.
- Size yardım edecek kişi için neyi değiştirdiğinizi not edin.
- Parolayı, özel anahtarı ya da lisans anahtarını kimseye vermeyin.
