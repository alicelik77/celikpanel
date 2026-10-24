# v0.1.0-alpha.107

[English](RELEASE-NOTES-v0.1.0-alpha.107.md)

> **Yayımlandı.** Bu sürüm 24 Ekim 2026'de (UTC) yayımlandı: sürüm commit'i `2e68e6039`, GitHub ön sürümü 21:06:26Z, yapıtlar ve imza doğrulanıp dış denetim geçtikten (26/26) sonra indirme portalı yaklaşık 21:13Z. Hiçbir parçası henüz kurulu bir sunucuda değil; kurulu panelleri yalnız sahip günceller. Aşağıdaki aday metni yazıldığı gibi korunur.

Bu, v0.1.0-alpha.106'yı izleyen sürümdür. **24 Ekim 2026'da yazılmış bir adaydır;
yayımlanmadı ve hiçbir parçası kurulu bir sunucuda değil. Kurulu panelleri yalnız
sahip, panelin kendi güncelleme ekranından günceller.** D-038 kaydından sonraki
`main`'den `feat/alpha107-d038` dalında yazıldı (alpha.106 sürüm commit'i
`a8f90616f6472300fe6bc29868fa962247da51ad`).

## Kim güncellemeli, neden

Her sunucu, hem de biri PostgreSQL ya da MariaDB'yi kaldırmadan önce. 24 Ekim
2026'da sahip, güvenlik yamalarının zorlanması gerekip gerekmediğini ve tek bir eski
sürümün bütün sunucuyu riske sokup sokmadığını sordu ve her bileşen için tek bir ilke
istedi (D-038). Salt-okur bir denetim şunları buldu: Sury deposundan gelen PHP'yi
Frankfurt'un otomatik güncellemeleri hiç kurmuyordu; bir üretici deposu dağıtımın
kendi paketlerinin yerine geçebiliyordu; PGDG varken yalın bir PostgreSQL kurulumu
varsayılanı izleyip yeni bir ana sürüme geçiyordu; bir veritabanı motorunu kaldırmak
onu purge ediyordu; web postasının güncelleme yolu yoktu ve hangi servislerin yeniden
başlatma beklediğini hiçbir şey söylemiyordu. Bu sürüm bu açıkları kapatır. Sahibin
gördükleri [işlem yönlendirmesinde](OPERATION-GUIDANCE.tr.md).

## Güvenlik güncellemeleri işletim sisteminindir ve artık görünür

- **Güvenlik → Güvenlik güncellemeleri**: otomatik güncellemelerin açık olup
  olmadığı, son çalışma, bekleyen güncellemeler ve güvenlik güncellemeleri, kurulu
  hangi paketlerin otomatik güncellemelerin kapsamadığı kaynaklardan geldiği ve
  CelikPanel'in paket yöneticisinin dışında kurdukları (Roundcube, Node.js). Kapalı
  olması, açmak için gereken komutla bir uyarıdır; CelikPanel onu asla kapatmaz.
- **Otomatik güncellemelere ekle…**, metnini onayladıktan sonra tek bir dosya yazar:
  `/etc/apt/apt.conf.d/52celikpanel-vendor-origins`; sizin dosyalarınız asla
  düzenlenmez. Geri almak için onu silin. Yalnız sizin sınırladığınız bir depo
  (aşağıda) eklenebilir: sınırlanmamış bir depo, otomatik güncellemelerin dağıtımın
  paketlerini değiştirmesine yol açardı.

## Üretici depoları kendi alanlarında kalır

- Depo kartındaki (PHP-FPM ve PostgreSQL sayfaları) **Sınırla…** sizin
  eyleminizdir: önce tam apt kural dosyasını ve SHA-256'sını, depoyu izlemeye devam
  eden paketleri ve `apt upgrade`'in neyi farklı yapacağını gösterir; onayladıktan
  sonra (**Bu kuralı yaz**) yalnız o metin yazılır. Hiçbir şey bir depoyu
  kendiliğinden sınırlamaz. Geri almak için dosyayı silin; kart sonra bunu söyler ve
  onu asla kendiliğinden yeniden yazmaz (kuralı geri isterseniz **Yeniden sınırla…**).
  Sahibin kendi kuralı korunur ve adlandırılır.
- Bir dağıtım paketinin yerine üretici sürümünü koyacak bir kurulum adıyla
  reddedilir. Yalnız üreticinin sağladığı bir sürüm, üreticinin `php-common`'ına
  (Sury) ya da `libpq5`'ine (PGDG) ihtiyaç duyabilir: kurulum onu sürümleriyle
  adlandırır ve onayınızı bekler. İstemediğiniz bir paketi yükseltecek bir kurulum onu adlandırır ve onayınızı
  bekler (örneğin postgresql-17, Debian'ın perl-base güvenlik güncellemesini ister).
- Üretici deposu varken yalın bir kurulum, dağıtımın varsayılanının getirdiği tam
  sürümü alır (`php8.4-fpm`, `postgresql-17`); varsayılanı izleyen paketi asla
  almaz.
- Depo kartı onu kimin ne zaman açtığını, imza anahtarını ve bitiş tarihini,
  listenin tarihini ve apt'nin ne bildirdiğini söyler.

## Kaldırmak veriyi korur; web postası ve Node.js güncellemelerini alır

- **PostgreSQL ve MariaDB purge edilmez, kaldırılır:** pencere verinin nerede
  kaldığını söyler ve sonraki bir kurulum veriyi bulduğunu ve onu kullandığını
  söyler.
- **Web postası (Roundcube)**: "1.6.x sürümüne güncelle…" yayını indirir, imzasını
  Roundcube'un anahtarıyla denetler, yapılandırmayı, eklentileri ve veritabanını
  korur, şimdiki web postasını arşivler, tek adımda yerine koyar ve web postası yanıt
  vermezse onu geri koyar. Hiçbir servis yeniden başlamaz. Bu sürüm yeni kurulumlar
  için 1.6.20'yi kurar.
- **Node.js**: kurulu her hat daha yeni yamasını söyler; onu kurun, sonra her siteyi
  istenince taşıyın: uygulama yeni sürümle yeniden başlar ve yanıt vermelidir,
  vermezse geri konur.

## Yeniden başlatmalar, destek tarihleri, hesaplar ve işletim sistemi

- **Bakım → Yeniden başlatmalar ve yeniden açılış**: hâlâ değiştirilmiş kodla
  çalışan servisler ve bir yeniden açılışın bekleyip beklemediği; her yeniden
  başlatma önce neyi keseceğini söyler.
- **Destek tarihleri**: tarihli bir tablo (24 Ekim 2026'da denetlendi), onu kim
  sağlıyorsa ondan artık güvenlik düzeltmesi almayan sürümleri işaretler; bilinmeyen
  bilinmeyen kalır. Bir site için böyle bir PHP seçmek onay ister.
- **Node.js uygulamaları** bundan sonra sitenin kendi kullanıcısıyla çalışır; var olan
  bir uygulama paylaşılan web kullanıcısıyla çalıştığını söyler ve yalnız istenince
  taşınır, kanıtlanır ya da geri konur. Paylaşılan yalıtım (tek bir PHP havuzunda
  web postası ve veritabanı araçları, tek bir kullanıcı altında posta kutuları) açıkça
  söylenir.
- **Sunucu kurulumu → İşletim sistemi**: bu sunucunun desteklenen listedeki yeri.
  Sahip işletim sistemini yükselttikten sonra, gözden geçirildi olarak işaretlenene
  kadar neyin değiştiğini ve ne yapılacağını söyler. CelikPanel o yükseltmeyi asla
  başlatmaz.
- **Denetim günlüğü**: satırlar ayrıntı taşır (bileşen, eski ve yeni sürümler,
  onaylar, sonuç); sitenin PHP ve Node.js değişiklikleri kaydedilir.

## Sınırlar

- Şema 44'ten 45'e (iki ekleyici sütun); daha eski bir sürüm yeni veritabanını
  açamaz, bu yüzden geri dönüş güncelleme öncesi anlık görüntünün geri yüklenmesidir.
- Kurulu bir sunucuda ölçülmedi. pacman ve DNF yolları yalnız bileşen testleriyle.
  Roundcube'u başka bir hatta taşımak, ana veritabanı yükseltme akışı, sürümleri
  müşterilerden gizlemek ve bileşen başına "denenmiş" işaretleri yapılmadı.
- Aday ağacındaki kanıt: Agent 4771 geçti, 29 atlandı (temiz bağlama ad alanı);
  Panel ve iç paketler 7925 geçti, 45 atlandı, 0 başarısız; sürüm kurtarma düzeneği
  geçiş 45'i sabitler ve testleri geçti; web 1844 geçti ve üretim derlemesi
  bütçelerinin içinde (yeni "lifecycle" sözcük parçasının kendi tavanı var); tarayıcı
  grubu 30 (EN ve TR, 1440 ve 390, açık); atılabilir bir Debian 13 sanal makinesinde
  bir gerçek sistem denemesi
  ([kanıt](../deploy/e2e/d038/alpha107-debian13-20261024/README.md), arşiv SHA-256
  `1a317d7b…9266913`). Önceki üç denemenin bulduğu ve bu sürümün düzelttikleri: tam
  bir kurulumdan sonra PostgreSQL'in küme biriminin bulunamaması ve kaldırmada bir
  şablon adının birim sanılması, ve Sury PHP sürümlerinin sınırlama yüzünden
  engellenmesi (artık böyle bir kurulumda üreticinin `php-common`'ı, PGDG'nin
  `libpq5`'i adlandırılır). Debian'ın kendi `postgresql-17` purge'ü, sanal makinede
  elle çalıştırıldığında kümeyi sildi; CelikPanel purge etmeden kaldırır.
