# Kullanıcı Rolleri ve Yetkiler

*Tasarım belgesi · 3 Temmuz 2026 · [English](ROLES.md)*

Bu bir **tasarım şartnamesidir**, henüz tam olarak uygulanmadı. Faz 0.2 kimlik doğrulama çalışmasının hedeflediği taslaktır. [Anayasaya](../ROADMAP.tr.md) sadık kalır: dört net rol, tek bariz yol, varsayılan olarak güvenli.

---

## Temel fikir

CelikPanel'de **dört rol** bir hiyerarşi içinde dizilir. Her katman yalnızca kendisinin üstündeki katmanın verdiği kadarını görebilir ve o kadarına müdahale edebilir. Bu tek, öngörülebilir sahiplik zinciri; tek kişilik bir web sitesinden, birçok bayi işleten bir dağıtıcıya kadar her senaryoyu kapsar — Plesk ve cPanel'i korkutucu yapan o iç içe geçmiş yetki ekranları labirenti olmadan.

```
Yönetici                 sunucunun kendisi — tam yetki
   │  oluşturur & kaynak verir
   ├── Bayi              kaynak havuzundan hosting satar
   │      │  oluşturur & kaynak verir
   │      └── Müşteri    aboneliklere sahiptir, sitelerini işletir
   │             │  devreder
   │             └── Ek Kullanıcı   tek bir müşterinin sınırlı alt kümesi
   │
   └── Müşteri           (Yönetici doğrudan müşteri de oluşturabilir)
```

Basitliği sağlayan kural: **bir kaynağın her zaman tam olarak bir sahibi vardır ve yalnızca sahip olduğunu ya da sana devredileni yönetebilirsin.** Ortak sahiplik yok, muğlak çapraz bağlar yok.

---

## Dört rol

### 1. Yönetici (`admin`)
Sunucu sahibi. Normalde **bir tanedir**. cPanel'deki *root/WHM* ve Plesk'teki *Administrator* karşılığı.

- Servisleri kurar, başlatır, durdurur, yapılandırır (Nginx, PHP, MariaDB, PostgreSQL, mail, DNS, …)
- Sunucu geneli ayarları, IP adreslerini, panel SSL'ini, güncellemeleri yönetir
- **Bayi** ve **müşteri** oluşturur ve yönetir
- Sunucudaki her şeyi görür; işletim sistemi katmanına (Agent aracılığıyla) dokunan tek roldür

### 2. Bayi (`reseller`)
Yöneticinin verdiği bir **kaynak havuzundan** (örn. "toplam 100 GB disk, 50 domain, istediğin gibi dağıt") kendi müşterilerine hosting satar. cPanel'deki *Reseller (sınırlı WHM)* ve Plesk'teki *Reseller* karşılığı.

- Havuzu dahilinde **müşteri** ve aboneliklerini oluşturur ve yönetir
- Her müşterinin kotasını belirler (havuzda kalandan fazlasını asla veremez)
- Kendi müşterilerini askıya alır/açar, parolalarını sıfırlar
- Sunucuya, servislere, diğer bayilere ya da kendi oluşturmadığı müşterilere **dokunamaz**
- Kendi markasına sahip olabilir (gelecekte: beyaz etiket giriş ekranı)

### 3. Müşteri (`customer`)
Asıl web sitelerini işleten kişi. Bir *cPanel hesabı* / Plesk *Customer* karşılığı. Bu tek rol, tasarım gereği senaryolarınızdan ikisini birden kapsar:

- **Tek web sitesi:** bir abonelik, bir domain. Panel yalnızca ihtiyacı olanı gösterir.
- **Çok web sitesi:** bir abonelik birçok domain tutabilir ya da müşteri birden fazla aboneliğe sahip olabilir. Aynı rol, daha çok kaynak — öğrenilecek yeni bir kavram yok.

Müşteri, **kotası dahilinde** şunları yönetir: domainler ve siteler, DNS kayıtları, e-posta hesapları, veritabanları, dosyalar, yedekler, cron işleri, SSL, PHP ayarları, loglar ve istatistikler — ama yalnızca kendi abonelikleri için.

### 4. Ek Kullanıcı (etkin rol `additional_user`)
Tek bir müşteriye ait, işin bir kısmını ana parolayı paylaşmadan devretmek için kullanılan **sınırlandırılmış** bir giriş. cPanel *alt hesaplar / User Manager* ve Plesk *Ek Kullanıcılar* karşılığı.

Şema uyumluluğu için bu kimlik `users.role = 'customer'` ve değiştirilemez `users.account_type = 'additional_user'` işaretiyle saklanır. Kimlik doğrulama, etkin `additional_user` rolünü bu ikiliden ve sahip `parent_id` değerinden türetir; tutarsız birleşimler varsayılan olarak reddedilir.

Örnekler:
- Dosya ve veritabanlarını düzenleyebilen ama e-posta ya da faturaya dokunamayan bir geliştirici
- Yalnızca e-posta hesaplarını yöneten bir ofis sorumlusu
- Yalnızca istatistikleri gören, salt-okunur bir muhasebeci

Bir ek kullanıcı, ait olduğu tek müşterinin dışındaki kaynakları asla görmez ve yalnızca o müşterinin devrettiği yetkilere sahiptir (aşağıdaki yetki modeline bakın).

---

## Senaryolarınız için hangi rol

| Senaryo | Rol |
|---|---|
| Tek web sitesi olan bir kişi | **Müşteri** (bir abonelik, bir domain) |
| Birçok web sitesini yöneten bir kişi | **Müşteri** (çok domain / çok abonelik) — ya da bir kısmını **Ek Kullanıcılara** devret |
| Birçok müşterisi olan bir hosting satıcısı | **Bayi** |
| Birden fazla bayiye sahip / onları denetleyen biri | **Yönetici** — bayiler yöneticiye bağlanır. Bayilerin üzerinde ayrı bir "dağıtıcı" katmanı şimdilik bilinçli bir *yapılmayacaklardan* (aşağıya bakın). |

---

## Yetki modeli (Ek Kullanıcılar için)

Yönetici, bayi ve müşterinin yetkileri rolleri tarafından sabitlenmiştir — paneli basit tutan şey budur. **Ayrıntılı yetkilendirme tam olarak tek bir yerde vardır: ek kullanıcı**, yani müşterinin *kendi* yetkilerinin bir alt kümesini devrettiği yer.

Yetkiler kaynağa göre gruplanır. Ek kullanıcı tanımlayan müşteri, **kendi** setinden seçer:

| Yetki grubu | Görüntüle | Yönet |
|---|:---:|:---:|
| Dosyalar (domain başına ya da hepsi) | ☐ | ☐ |
| Veritabanları | ☐ | ☐ |
| E-posta hesapları | ☐ | ☐ |
| DNS kayıtları | ☐ | ☐ |
| SSL / sertifikalar | ☐ | ☐ |
| Cron işleri | ☐ | ☐ |
| Yedekler | ☐ | ☐ |
| PHP ayarları | ☐ | ☐ |
| İstatistik ve loglar | ☐ | ☐ |

İki özellik bunu hem güvenli hem basit tutar:
- **Kapsam aşağı indikçe daralır, asla genişlemez.** Bir ek kullanıcı, ait olduğu müşterinin sahip olmadığı bir yetkiyi asla kazanamaz. Müşteri aboneliğini aşamaz. Bayi havuzunu aşamaz.
- **Devir domain başına seçilebilir.** Bir yetki tek bir domain için ya da müşterinin tüm domainleri için verilebilir — yetki labirentinden kaçınmak için bundan daha ince değil.

---

## Uygulama — kuralların gerçekte yaşadığı yer

Yetkiler bir **arayüz** meselesi değildir. "Önce API" ilkesi gereği her kural sunucuda, tek bir yerde uygulanır:

1. **Kimlik doğrulama** (Faz 0.2): kimsin? → bir `user`'a bağlı oturum.
2. **Sahiplik çözümleme:** her kaynak (domain, veritabanı, …) bir sahip aboneliğe → sahip müşteriye → (varsa) bayiye → yöneticiye çözülür. Bir istek, yalnızca çağıran bu zincirin içindeyse ya da yöneticiyse kabul edilir.
3. **Yetki kontrolü:** ek kullanıcılar için, sahipliğin üstüne o belirli yetki de kontrol edilir.

Arayüz yalnızca API'nin reddedeceği şeyi gizler — "kurmadığın servis görünmez" ile "yalnızca sahip olduğunu görürsün" böylece aynı mekanizma haline gelir.

---

## Veri modeli sonuçları (0.2 sprinti için)

Mevcut şema omurgayı zaten içeriyor: `users(role)` ve `subscriptions(owner_id)`. Bu tasarımı hayata geçirmek için:

- Değiştirilemez bir `users.account_type` işareti (`account` veya `additional_user`) eklenir. Ek kullanıcıların saklanan rolü `customer` kalır; yetkilendirme iki sütundan birine tek başına güvenmek yerine türetilen etkin rolü kullanır.
- Kullanıcıyı kimin oluşturduğunu/sahiplendiğini göstermek için `users.parent_id` (nullable, `users.id`'ye referans) kullanılır. Ek kullanıcı için bu alan etkin, gerçek bir müşteri hesabını göstermelidir.
- Bayinin dağıttığı kaynak havuzu için bir `reseller_pools` kavramı (ya da bayinin kendi kaydında sütunlar) eklenir.
- Yetkiler iki açık kapsam tablosunda tutulur: `additional_user_subscription_permissions` ve `additional_user_domain_permissions`. İkisinde de kapalı bir yetenek kümesi ile `view|manage` modu vardır. Bir domain için etkin erişim, doğrudan-domain ve abonelik yetkilerinin eklemeli birleşimidir; `manage`, `view` değerinden baskındır.
- Her veri erişim repository'sine bir sahiplik filtresi eklenir; hiçbir handler sahiplik kontrolü olmadan ham ID ile sorgu yapmaz.

Bu değişiklikler **eklemeli** olup Faz 0.2'de kimlik doğrulamayla birlikte gelir; böylece model ve onu uygulayan kurallar birlikte yayınlanır — arkasındaki kurallar olmadan asla bir giriş ekranı çıkmaz.

---

## Uygulanmış olanlar (v0.1.0-alpha.114)

Bu bölüm kodun bugün ne yaptığını söyler; yukarıdaki tasarım hedef olarak kalır.

- **Roller ve kapılar.** `admin`, `reseller`, `customer` ve türetilmiş `additional_user` sunucuda uygulanır. Rota × rol testi (`cmd/panel/route_role_matrix_test.go`) `cmd/panel/main.go` içindeki her rotayı okur ve kapısını anonim ziyaretçi ve her rol için denetler; satırı olmayan yeni rota testi düşürür. Hesap rotaları ayrıca gerçek işleyicileriyle sürülür (`TestAccountRoutesRoleMatrix`).
- **Planlar.** Yöneticinin planları geneldir. Bayi kendi planlarını oluşturur (`service_plans.owner_id`); genel planları ve kendininkileri görür, yalnız kendininkileri düzenler; planları havuzundan fazlasını vaat edemez. Bayinin planı yalnız o bayinin müşterilerine verilir.
- **0 sınırsız demektir:** domain, veritabanı, posta kutusu ve disk için; plan formunda, plan doğrulamasında, her kota denetiminde ve her kullanım yanıtında.
- **Oluşturmadan sonra plan değişikliği** (`PUT /api/v1/users/{id}/plan`; yönetici, bayi kendi müşterileri için). Sınırlar plandan yeniden kopyalanır. Mevcut kullanımın altındaki plan, aşan kalemlerin listesiyle (`PLAN_BELOW_USAGE`) operatör onaylayana dek reddedilir; onaydan sonra hiçbir şey silinmez, aşan türlerde yenisi yönlendirmeyle reddedilir (`QUOTA_*_EXCEEDED`, Planım'a işaret eder).
- **Bayi havuzu** (`reseller_pools`; yönetici `/api/v1/users/{id}/pool` adresinden ya da bayiyi oluştururken belirler): bayi ağacı üzerinde toplam domain, veritabanı, posta kutusu, disk ve müşteri sayısı. Havuz *dağıtımı* (verilen plan sınırlarının toplamını) sınırlar; aşırı satılamaz. Aşım `RESELLER_POOL_EXCEEDED` / `RESELLER_CUSTOMER_LIMIT` ile reddedilir. Dağıtılanın altına indirilen havuz uyarıyla kaydedilir; hiçbir şey geri alınmaz. Havuz satırı olmayan bayinin sınırı yoktur (alpha.114 öncesindeki her bayi).
- **Askıya alma/sürdürme.** Bayi kendi müşterilerini, yönetici her hesabı askıya alır ve sürdürür; ikisi de denetim izine yazılır.
- **Planım** (`GET /api/v1/my/subscription`, Pano'da ve Hesabım'da bir kart): plan, sınırlar, canlı kullanım (kota denetimlerinin kullandığı aynı sayılar), daha büyük plan için kime başvurulacağı ve bayi için havuzu. Ek kullanıcılar görmez.
- **"Farklı giriş yap" hesap verir.** Oturum operatörünü kaydeder (`sessions.impersonator_id`). İçinde yazılan her denetim satırı gerçek kişiyi `user_id`, hesabı `acting_as` ile adlandırır; `impersonate.start` ve `impersonate.stop` kaydedilir. Operatör askıya alınınca, parolasını değiştirince ya da hesabı artık yönetemeyince oturum biter. İç içe geçiş reddedilir. Kalıcı bir şerit "X olarak görüntülüyorsunuz" der ve dönüş yolunu gösterir.
- **Parola değişikliği** o kişinin bütün oturumlarını, operatör olarak açtığı "Farklı giriş yap" oturumlarıyla birlikte bitirir.

**Henüz uygulanmayanlar:** trafik (bant genişliği) saklanır ama **ölçülmez**; bu yüzden trafik sınırı uygulanmaz ve ekranlar bunu söyler; abonelik askıya alma/süre bitimi etkileri, fiyatlar, dönemler ve ödemeler; bayi askıya alma "zincir kuralı"; işletim sistemi düzeyinde disk kotası (disk, oluşturmada önbellekli ölçümlerle yumuşak bir kapıdır); mevcut ekip üyesi izinlerinin ötesindeki yetenek modeli.

---

## Bilinçli yapılmayacaklar (sadelik)

- ❌ **Sınırsız iç içe geçme** (bayinin altında bayi, onun altında bayi). Zincir dört katmanda sabittir. Bayilerin üzerine bir "dağıtıcı" katmanı ancak gerçek talep doğarsa *sonra* eklenebilir — o zamana kadar yönetici bu rolü doldurur.
- ❌ **Müşteri/bayiler için elle kurulan özel yetki setleriyle özel roller.** Yalnızca ek kullanıcılar ayrıntılıdır; diğer herkes kendi rolüdür. Bu, cPanel/Plesk kafa karışıklığının en büyük kaynağıdır ve bilerek reddediyoruz.
- ❌ **"Tek domain"den daha ince kayıt-başı ACL'ler.** En küçük kapsam bir domaindir.
