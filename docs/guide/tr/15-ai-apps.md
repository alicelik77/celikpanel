---
id: ai-apps
title: Yapay zekâ ve otomasyon uygulamaları
roles: [admin, reseller, customer]
topics: [domain.ai#ai-apps]
keywords: [yapay zekâ, yapay zeka, n8n, open webui, otomasyon, iş akışı, sohbet, openai, api anahtarı, güvenlik beklemesi]
---

**Yapay zekâ ve otomasyon** sekmesi bir alan adında n8n (iş akışı otomasyonu) ya da Open WebUI (yapay zekâ modelleri için sohbet ekranı) gibi bir uygulama çalıştırır. Uygulama sitenin kendi hesabıyla kapalı bir korumalı alanda çalışır, yalnız sunucunun içinde dinler ve alan adının adresinde HTTPS ile açılır.

## Kurmadan önce {#ai-apps}

Sekme hâlâ neyin gerektiğini ve onu kimin sağlayacağını yazar:

- **Web sitesi ve sertifika.** Uygulamalar yalnız HTTPS ile açılır. Alan adının sertifikası yoksa önce **Barındırma** sekmesinde **SSL/TLS** bölümünden alın.
- **Ayrı bir alt alan adı**, örneğin `n8n.example.com`, en iyisidir: alan adı web sitesi yerine uygulamayı gösterir. Sitenin dosyaları yerinde kalır ve uygulama kaldırılınca geri gelir.
- **Sunulmuş olması.** Yönetici uygulamayı sunmadan hiçbir şey kurulamaz; planınız da onu kapsamalıdır. Kapsamıyorsa satır kime başvuracağınızı yazar.
- **Sunucu.** n8n, yöneticinin kuracağı Node.js 24'ü ister. Open WebUI yalnız bazı sistemlerde çalışır; burada çalışamıyorsa satır bunu söyler.

## Kurmak {#installing}

1. Uygulamanın yanındaki **Kur** düğmesine basın.
2. **Uygulama yöneticisinin e-posta adresi** ve **Uygulama yöneticisinin parolası** (12 ile 128 karakter) girin. Uygulamaya bunlarla girersiniz; böylece uygulamayı sizden önce kimse üstlenemez. CelikPanel parolayı saklamaz.
3. Open WebUI için isterseniz **Yapay zekâ sağlayıcı anahtarları (isteğe bağlı)** ekleyin. Anahtarlar yalnız uygulamanın kendi ayar dosyasında tutulur ve bir daha gösterilmez.
4. Uygulamanın neler yapabildiğini ve hangi risklerin kaldığını okuduğunuzu işaretleyip onaylayın.

Kurulum sunucuda çalışır ve birkaç dakika sürebilir; Open WebUI birkaç GB'tır. Sayfayı kapatabilirsiniz; döndüğünüzde sekme her adımı gösterir. Bir adım başarısız olursa ileti nedenini ve kimin ne yapacağını yazar; ayrıntılar kurulum günlüğündedir.

## Uygulamayı kullanmak {#running}

**Uygulamayı aç** onu yeni bir sekmede açar. Satır uygulamanın **Çalışıyor**, **Durdu** ya da **Çalışıyor ama henüz yanıt vermiyor** durumunu ve ne kadar bellek kullandığını gösterir. **Başlat**, **Durdur**, **Yeniden başlat** düğmeleri ve **Günlük** buradadır. **Anahtarları değiştir** sağlayıcı anahtarlarını değiştirir ya da yeni yönetici parolası belirler.

Bellek, işlemci ve işlem sınırlarını her kurulum için yönetici belirler. Bir panel sürümüyle daha yeni bir sürüm gelince **… sürümüne güncelle** onu çalışanın yanına kurar ve yalnız hazır olunca geçiş yapar.

## Güvenlik beklemesi {#hold}

Yönetici bir uygulamayı, örneğin bir güvenlik açığı duyurulduğunda, güvenlik beklemesine alabilir. O uygulamanın bütün kurulumları durur; bekleme kaldırılana kadar başlatılamaz ve kurulamaz. Verileriniz kalır. Beklemenin kaldırılması hiçbir şeyi başlatmaz: uygulamayı kendiniz yeniden başlatın.

## n8n neden bayi üzerinden kullanılamıyor {#n8n-resellers}

n8n'in lisansı onu başkalarına barındırılan hizmet olarak sunmayı kısıtlıyor. Bu yüzden bayilere ve onların müşterilerine kapalıdır; yalnız yöneticinin kendi müşterileri kullanabilir. Satır bunu yazar; yapmanız gereken bir şey yok. Open WebUI bundan etkilenmez.

## Uygulamayı kaldırmak {#removing}

**Uygulamayı kaldır** hizmeti ve program dosyalarını kaldırır; alan adı yeniden web sitesini gösterir. Uygulamanın verileri (iş akışları, sohbetler, veritabanı) siz **Uygulamanın verilerini de sil** seçeneğini işaretleyip alan adını yazmadıkça kendi klasöründe kalır.

Uygulama panele bağlı değildir. Sunucuda sıradan bir sistem hizmetidir; sekme adını yazar ve sunucunun sahibi onu `sudo systemctl stop` ve ardından o adla durdurabilir.
