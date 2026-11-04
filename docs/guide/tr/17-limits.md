---
id: limits
title: Sınırlar ve askıya alma
roles: [reseller, customer]
topics: []
keywords: [sınır, kota, trafik, bant genişliği, disk, dolu, askıya alındı, askı, plan, yükseltme, %80, %100]
---

Planınız ne kadar kullanabileceğinizi belirler: alan adları, posta kutuları, veritabanları, disk alanı ve aylık trafik. Bu bölüm bir sınır dolunca ne olduğunu, kimin ne yapacağını ve her şeyin nasıl yeniden açıldığını anlatır. Bir sınır yüzünden hiçbir şey silinmez.

## Kullanımınızı nerede görürsünüz {#my-plan}

**Panel** sayfasındaki ve **Hesabım** sayfasındaki **Planım** kartı her sınırı kullanılan / izin verilen olarak, diski site dosyaları, posta ve veritabanları diye ayırarak ve **Bu ayki trafik** bilgisiyle gösterir. %80 ve %100 dolulukta siz, sağlayıcınız ve yönetici **Trafik ve disk bildirimleri** altında bir bildirim alırsınız.

## Trafik {#traffic}

Trafik, web sitelerinizin bir takvim ayında ziyaretçilerine gönderdiğidir. 5 dakikada bir sayılır; bu yüzden bir site, o sürede gönderdiği kadar sınırı aşabilir.

Ayın trafiği dolunca:

- yalnız web siteleri "trafik sınırı doldu" gösterir;
- posta, veritabanları ve dosya erişimi çalışmaya devam eder;
- web siteleri gelecek ayın 1'inde ya da sağlayıcınız sınırı yükselttikten birkaç dakika sonra kendiliğinden açılır.

Sağlayıcınız bir bayiyse, bayinin ortak trafiği dolduğunda da siteleriniz kapanabilir; bildirim bunu söyler.

## Disk alanı {#disk}

Disk kullanımı site dosyalarını, postayı ve veritabanlarını sayar; birkaç saatte bir ölçülür. Yedekler sayılmaz.

Disk dolunca:

- yeni bir şey oluşturulamaz: alan adı, posta kutusu, veritabanı, uygulama, elle yedek ya da geri yükleme yapılamaz;
- yüklemeler durur: dosya yöneticisi reddeder, FTP ve SFTP salt okunur olur (indirme çalışır), PHP siteleri yüklenen dosya kabul etmez;
- web siteleri çalışmaya devam eder; posta, her posta kutusunda yer oldukça teslim edilir.

Kim yapar: gerekmeyen dosya, posta ya da veritabanlarını siz silersiniz veya daha büyük plan istersiniz. Sınırın altındaki bir sonraki ölçümden sonra, birkaç saat içinde her şey kendiliğinden açılır.

Kullanım sınırın üstünde kalırsa 5 gün sonra bir uyarı gelir, 7 gün sonra abonelik askıya alınır.

## Aboneliğiniz askıya alındıysa {#suspended}

Askıdaki bir abonelikte giriş yapınca tek bir ekran görünür: **Aboneliğiniz askıya alındı**. Web siteleriniz, postanız, veritabanlarınız ve diğer hizmetleriniz kapalıdır. Hiçbir şey silinmemiştir.

Ekran şunları söyler:

- **Neden:** sağlayıcınızın yazdığı neden, diskin 7 gün sınırın üstünde kalması ya da aboneliğin bitiş tarihinin geçmesi;
- **Bunu kim değiştirebilir:** bayiniz ya da sunucu yöneticisi;
- **Ne yapmalısınız:** onlarla iletişime geçip aboneliği yeniden açmalarını isteyin;
- **Nasıl devam eder:** açtıklarında yeniden giriş yapın ya da sayfayı yenileyin.

Disk nedeniyle askıya alma, ölçüm sınırın altına inince kendiliğinden kalkar. Sağlayıcınız sınırı yükseltir ya da yer açmanız için aboneliği 7 gün daha açar.

## Özet {#summary}

| Dolan | Ne durur | Ne zaman açılır |
|---|---|---|
| Trafik %100 | Yalnız web siteleri | Ayın 1'inde ya da sınır yükseltildikten dakikalar sonra |
| Disk %100 | Yeni öğeler ve yüklemeler | Sınırın altına inince, birkaç saat içinde kendiliğinden |
| Disk 7 gün sınırın üstünde | Bütün hizmetler (askı) | Kullanım sınırın altına inince ya da sağlayıcı açınca |
| Posta kutusu dolu | O kutuya yeni posta | Yer açılır açılmaz ([ayrıntı](guide:mail#full)) |
