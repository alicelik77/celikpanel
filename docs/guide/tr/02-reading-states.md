---
id: reading-states
title: Durumu okumak ve yardım almak
roles: [admin, reseller, customer]
topics: [states]
keywords: [hata, sorun, bekleme, tekrar dene, tekrar kontrol et, bilinmiyor, takıldı, başarısız, destek]
---

CelikPanel'de bir şey ilginizi gerektirdiğinde her ekran size aynı dört şeyi söyler: nedeni, kimin bir şey yapması gerektiğini, sıradaki adımı ve işin ardından nasıl süreceğini. Bu bölüm bu iletilerin nasıl okunacağını anlatır; böylece beklemeniz mi, bir şey yapmanız mı, yoksa birine sormanız mı gerektiğini bilirsiniz.

## Kontrol ediliyor, kontrol edilemedi, biliniyor {#three-states}

Bir ekranın sunucudan okuduğu her şey üç biçimden biriyle gösterilir ve bu üçü hiçbir zaman birbirine benzemez.

| Gördüğünüz | Anlamı | Ne yapmalı |
|---|---|---|
| Küçük bir dönen işaret ve "…" ile biten bir cümle | Panel şu anda sunucuya soruyor. Ters giden bir şey yok. | Biraz bekleyin. |
| **Tekrar dene** düğmeli sarı bir uyarı | Panel yanıtı okuyamadı. Bu, o şeyin olmadığı ya da bozuk olduğu anlamına gelmez ve hiçbir şey değiştirilmedi. | **Tekrar dene** düğmesine basın. Sürekli oluyorsa [Panelin kendisi yanıt vermediğinde](#panel-not-answering) bölümüne bakın. |
| Yanıtın kendisi | Sunucu yanıt verdi. Ekran "yok", "kapalı", "boş" demeyi ya da bir sayı göstermeyi ancak şimdi yapar. | Okuyun, isterseniz işlem yapın. |

## Bir sorun size ne söyler {#problem}

Sizi durduran ileti, engellediği şeyin en üstünde durur. Şu sırayla okuyun:

1. **Ne yanlış**: alan adı, posta kutusu ya da bileşen adıyla, tek ve sade bir cümle.
2. **Kimin bir şey yapması gerekiyor**: siz, sunucu yöneticisi, sağlayıcınız, DNS sağlayıcınız ya da alan adı firmanız, ya da başka bir sunucunun sahibi.
3. **Sıradaki adım**: panelin yapabildiği kadar somut. Adımı panel kendisi atabiliyorsa cümlenin yanında bir düğme vardır.
4. **Nasıl sürecek**: panel kendiliğinden yeniden bakar, siz **Tekrar kontrol et** gibi bir düğmeye basarsınız ya da başka bir şeye gerek kalmaz.

Basamadığınız bir düğmenin nedeni hemen yanında yazar. Sorunun iç kodu **Teknik ayrıntılar** altında durur; ona yalnız birinden yardım isterken ihtiyacınız olur.

## Başarısız oldu, bir şey bekleniyor, ya da bilinmiyor {#failed-waiting-unknown}

Panel bu üç durumu birbirinden ayırır, çünkü her biri sizden başka bir şey ister.

- **Başarısız oldu.** Sunucu denedi ve olmadı. İleti neyin denendiğini ve şimdi neyin yerinde olduğunu söyler; çoğunlukla önceki durum geri gelmiştir. İletinin söylediği nedeni giderin, sonra yeniden deneyin.
- **Önce bir şeyin hazır olması gerekiyor.** Hiçbir şey denenmedi, çünkü önce başka bir şey hazır olmalı: o özelliği içeren bir plan, sunucuda bir bileşen, alan adı firmanızda bir DNS kaydı. İleti o şeyi ve onu kimin sağlayacağını söyler.
- **Sonuç bilinmiyor.** Değişiklik gönderildi ama yanıtı gelmedi; örneğin bağlantı koptu. Panel değişikliği ikinci kez göndermez. Güncel durumu yeniden okur ve değişikliğin yapılıp yapılmadığını size söyler. Bunun için **Tekrar kontrol et** düğmesini kullanın; bu düğme yalnız okur.

:::warning
Sonuç bilinmiyorken aynı değişikliği başka bir sekmeden ya da tarayıcıdan yinelemeyin. Önce **Tekrar kontrol et** ne olduğunu göstersin.
:::

## İş nasıl sürer {#resume}

Bir bileşen kurmak, yedek almak ya da sertifika almak gibi uzun işler tarayıcınızda değil, sunucuda çalışır. Sayfayı kapatsanız ya da bağlantınız kopsa da iş sürer. Geri döndüğünüzde sayfa işin nerede olduğunu gösterir. Bekleyen bir sayfa ya hareket eder ya da sizin bir şey yapmanız gerekmediğini söyler; sonucu görmek için sayfayı yenilemeniz hiçbir zaman gerekmez.

## Yardım istemek {#asking}

Bir sorunu kendiniz çözemiyorsanız iletinin adını verdiği kişiye yazın: müşteri ya da kullanıcı hesabınız için sağlayıcınıza, sunucudaki her şey için sunucu yöneticisine. Şunları gönderin:

- sayfayı ve sorunun ilgili olduğu alan adını, posta kutusunu ya da bileşeni;
- iletinin cümlesini ve **Teknik ayrıntılar** altındaki kodu;
- olayın saatini.

Parola ya da özel anahtar asla göndermeyin. CelikPanel bunları size bir daha göstermez; size yardım etmek için de kimsenin bunlara ihtiyacı yoktur.

## Panelin kendisi yanıt vermediğinde {#panel-not-answering}

Panele ulaşılamıyorsa ya da panel "başlatılıyor" diyorsa web siteleriniz, e-postanız, DNS, veritabanlarınız, zamanlanmış işleriniz ve sertifika yenilemeniz çalışmaya devam eder. Bunlar sunucunun sıradan hizmetleridir ve panele bağlı değildir. Birkaç dakika sonra yeniden deneyin. Panele hâlâ ulaşılamıyorsa sunucu yöneticisine haber verin.
