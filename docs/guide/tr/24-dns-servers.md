---
id: dns-servers
title: DNS ve eş sunucular
roles: [admin]
topics: [dns.servers, dns.infrastructure#infrastructure-zone]
keywords: [dns, ad sunucusu, ns1, ns2, bind, powerdns, ikincil, yedek, glue, alan adı firması, bölge aktarımı]
---

**DNS ve eş sunucular**, adlarınızı hangi sunucunun yanıtladığını ve internetin onları nasıl gördüğünü gösterir. DNS, BIND ya da PowerDNS üzerinde çalışır ve panel durduğunda da yanıt vermeyi sürdürür.

## Sayfayı okumak {#reading}

En üstteki kart internetten okur ve **Neden**, **Kim yapar**, **Sıradaki adım** ve **Sonra** satırlarını gösterir. **Şimdi bak** yeniden sorar, hiçbir şeyi değiştirmez.

**DNS sunucularınız** her sunucuyu **Ana DNS sunucusu** (değişiklikler burada yapılır), **Yedek DNS sunucusu** (kopyayı tutar, o da yanıt verir) ya da **Tek DNS sunucusu** olarak gösterir. "Şu an sorulamadı" bozuk değil, bilinmiyor demektir: birkaç dakika sonra yeniden bakın.

## İki sunucu {#pair}

Ana DNS sunucusu alan adlarını standart bölge aktarımıyla yedeğe gönderir; yedek onları kendi başına yanıtlar. İkisi de diğerinin paneline ihtiyaç duymaz. Yedek sunucu, sunucu kurulumu sırasında katılma koduyla bağlanır (bkz. [İlk kurulum](guide:server-setup#join)). Tek başına kurulmuş bir PowerDNS sunucusu **Gelişmiş** altında eşleştirilir; tek başına çalışan bir BIND sunucusu bu yolla yedek kazanamaz.

## Alan adı firmanızda {#registrar}

"Alan adı firmanızda bir işiniz var", internetin ad sunucularınızı ya da onların IP kayıtlarını (glue) henüz görmediği anlamına gelir. Kartın gösterdiği değerleri alan adı firmanızın panelinde girin. Panel değişikliği kendisi fark eder; firmaların yayımlaması saatler sürebilir.

## Panel dışında yapılan değişiklikler {#outside}

"CelikPanel dışında değiştirildi", DNS'in doğrudan sunucuda, örneğin `rndc` ile değiştirildiğini söyler. Panel değişikliğinizi korur. O değişiklik durdukça panel bu sunucuda hiçbir DNS değişikliği yayımlamaz; böyle bir düzenlemeden sonra DNS'i panele geri vermek henüz mümkün değil.

## Gelişmiş {#advanced}

**Gelişmiş** bölümünde DNS yazılımı (**Yetkili DNS sunucusu**), eşleşme ve buraya kayıt yayımlayan paneller için **DNS sunucularına bağlantılar** durur. **DNS değişiklik iznini iptal et** böyle bir bağlantıyı bitirir; mevcut bölgeler yanıt vermeyi sürdürür.

## Altyapı bölgesi {#infrastructure-zone}

Kurulum, panelin kendi adları için bir bölge oluşturur: panel adresleri, posta sunucuları ve ad sunucuları. **Altyapı bölgesi · yalnız DNS** olarak işaretlidir; web sitesi ya da posta kutusu yoktur. **Sunucu kurulumu yönetiyor** işaretli kayıtlar yalnız kurulum genişletilerek değişir; diğer kayıtlar sizindir.
