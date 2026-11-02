# Planlar, haklar ve hareket alanı — TASLAK

*2 Kasım 2026 · Durum: **TASLAK, sahibin onayını bekliyor.** Onaylanmadan hiçbir
madde uygulanmaz. Bu belge yapılmış bir şeyi anlatmaz; ne yapılacağını önerir.*

Bu taslak, sahibin 2 Kasım 2026'da verdiği yönü ve kararları tek yerde toplar.
Temmuz'da alınan iki kararı genişletir ama değiştirmez:
D-014 ("yönetici kurar, bayi sunar, müşteri kullanır; birim sürümdür") ve
D-017 (her seçim tek bir teklif kimliği alanından geçer). Sürüm ilkesine (D-038:
kullanılan şey kaldırılmaz, kimse sessizce taşınmaz), sahip bağımsızlığına
(D-022) ve uygulanabilir yönlendirme kuralına (D-024) uyar.

---

## 1. Sahibin cümleleri (2 Kasım 2026)

- "Reseller ya da müşteriye neleri sunabileceğini veya kullanabileceğini admin
  belirlemeli. Reseller da müşterisinin neleri kullanabileceğini seçebilmeli."
- "Sunucuda admin'in Node.js bileşenini kurması reseller veya müşterinin
  kullanabileceği anlamına gelmez. Reseller ve müşteriler adminin hazırladığı
  oyun alanında oynayabilirler."
- "Bir müşteri bir domainine Node.js tanımlar, PHP 8.5 tanımlar. Ya da Node.js
  yetkisi vermez, PHP'de 8.3 verir."
- "Her şeyin sınırsız olması da bir plandır. Belki de temel plandır."
- "Plan bir şablon aslında. Herkes illa planda olmak zorunda değil. Plan
  seçmeden de teker teker seçip belirleyebilirsin."
- "Bağlı olduğu plandan tek bir şey farklı yapılırsa plandan çıkar, kendine
  göre özel olur. Plan + bilmem ne olmaz."
- "Reseller, ona verdiğim yetki çerçevesinde kendi planlarını belirleyebilir."
- "Bunu çok dikkatli ve hassas tasarla. Her şeyi onaylamadan işe başlama."

---

## 2. Bugün ne var, ne yok (koddan, 2 Kasım 2026)

**Var:**
- Planlar (`service_plans`): yalnız sayılar. Alan adı, veritabanı, posta
  kutusu ve disk sınırı var. Trafik alanı da var ama ölçülmüyor. 0 sınırsız
  demek.
- Abonelik (`subscriptions`): planın sayılarını kopyalar.
- Bayi havuzu (`reseller_pools`): bayinin dağıtabileceği toplamlar.
- Bayi kendi planlarını yapabiliyor ama yalnız sayı olarak.
- Yöneticinin bütün sunucu için tek bir "müşterilere gösterilen sürümler"
  listesi var; yalnız PHP ve Node.js için.
- İki ürün izni var: VPN ve uygulama kurucu.
- Ek kullanıcılar için ayrıntılı izin modeli var; bu taslak ona dokunmaz.

**Yok:**
- Planda hangi işin (DNS düzenleme, zamanlanmış görev, yedek…) açık olduğu.
- Planda hangi yazılımın ve sürümün açık olduğu.
- Abonelik devri.
- Kurulu olmayan ya da izin verilmeyen bir özelliğin menüden gizlenmesi.

**Bu çalışmada bulunan hatalar** (ayrı küçük sürümde düzeltilecek, bu taslağa
bağlı değil):
1. Bayi hiçbir şey sorulmadan sınırsız oluşuyor.
2. Alan adı sayfasından veritabanı oluşturmada kota denetimi yok.
3. Plan düzenlenince kullanımın altına inme denetimi yok.
4. Vekil (proxy) sitenin hedef adresi kısıtsız. Bu bir güvenlik açığı:
   sunucunun yalnız içeriye açık hizmetleri bir müşterinin alan adından
   dışarı açılabilir.
5. VPN sayfası bayiye ve müşteriye yanlış görünüyor.

---

## 3. İlkeler

**İ1 — Kurulu olmak izin vermez.** Yönetici bir bileşeni kurduğunda onu
yalnız *verilebilir* yapar. Bir hesabın bir şeyi kullanabilmesi için o şeyin
hesabın **hak setinde** açık olması gerekir.

**İ2 — Alanı yönetici çizer, bayi yalnız daraltır.** Bir müşterinin
kullanabildiği her şey dört katmanın kesişimidir:

```
kurulu  ∩  yöneticinin sunduğu  ∩  bayinin hak seti  ∩  aboneliğin hak seti
```

Bayi kendisinde olmayan bir hakkı hiçbir müşterisine veremez. Ne planla ne
özel ayarla ne de devirle.

**İ3 — Her hesabın hakları ya bir plandan gelir ya da o hesaba özel seçilir.**
(Sahip, 2 Kasım 2026; ROLES.md'deki "bayi ve müşteriye elle izin seti yok"
kuralının yerine geçer.) Üç durum vardır ve her ekranda adıyla yazar:
- **Sınırsız:** Hiçbir plan seçilmemiş ve hiçbir şey özel olarak
  belirlenmemişse hesap Sınırsız plandadır. Sahip: "Bir plana bağlı olmayanlar
  Sınırsız plana dahil olurlar."
- **Plana bağlı:** Hak seti, bağlı olduğu planınkiyle birebir aynıdır. Plan
  değişince hesap da değişir.
- **Özel:** Hak seti hesaba aittir. Ya plan seçilmeden tek tek seçilmiştir ya
  da bir plana bağlıyken tek bir şey bile değiştirilmiştir. O anda hesap
  plandan çıkar ve "Özel" olur. "Plan + ek" diye bir ara durum yoktur.

**Planın amacı hız.** Sahip: "Plan seçimi, bayiye ya da müşteriye haklarını
hızlı bir şekilde şablon olarak yapıştırmak için. Yoksa her bayi ya da müşteri
için her şeyi tek tek seçmek gerekir." Plan bir zorunluluk değil, hazır bir
şablondur.

**İ4 — "Sınırsız" da bir plandır ve adı davranışını söyler.** Panel iki hazır
planla gelir: müşteriler için **Sınırsız** ve bayiler için **Sınırsız bayi**.
Sınırsız plan, yöneticinin sunduğu her şeyi açar ve hiçbir sayı sınırı koymaz.
Yönetici yeni bir şey sunduğunda (yeni bir PHP sürümü, yeni bir yetki, yeni bir
yazılım) Sınırsız plan onu **kendiliğinden** alır. Sahip, 2 Kasım 2026: "Yeni
bir şey ya da özellik gelince bu plana eklenmiyorsa adı Sınırsız olmaz, A planı
olur." Bu yüzden Sınırsız planın kalemleri tek tek seçilemez. Daraltılmak
istenirse o plan Sınırsız olmaktan çıkar: yönetici ondan yeni adlı bir plan
türetir. Sınırsız plan formda adıyla görünür; gizli bir varsayılan değildir.

**İ5 — Daraltma çalışanı bozmaz** (D-038). Bir kalem kapatıldığında, onu
kullanan site ya da hizmet çalışmaya devam eder. Yalnız yeni kullanım
reddedilir. Ekranda kimin ne yapması gerektiği yazar. Hiçbir site sessizce
başka bir sürüme taşınmaz.

**İ6 — Kural sunucuda zorlanır, ekran yalnız gösterir.** Ekranda gizlenen her
şey sunucu tarafında da reddedilir. cPanel'deki "PHP sürümü ekranda kısıtlı
ama dosyayla aşılabiliyor" hatası tekrarlanmaz.

**İ7 — Sessiz değişiklik yoktur.** Plan değiştirmek, aboneliği özele çevirmek,
bir plana bağlamak ve devretmek birer değişikliktir. Her biri önce neyin
değişeceğini gösterir, sonra onay ister ve denetim kaydına yazılır.

---

## 4. Hak setinin içeriği

Hak setinin üç bölümü var. Plan da özel hak seti de aynı üç bölümden oluşur.

### 4.1 Miktarlar

| Kalem | Not |
|---|---|
| Alan adı sayısı | Alt alan adları da sayılır (bugünkü gibi) |
| Veritabanı sayısı | Bütün oluşturma yollarında denetlenir (bugün bir yol eksik) |
| Posta kutusu sayısı | |
| Disk | Bugün oluşturma anında yumuşak bir kapı; işletim sistemi kotası ayrı iş |
| Trafik | Ölçülene kadar planda **gösterilmez** (dürüstlük kuralı) |
| Müşteri sayısı | Yalnız bayi hak setinde |

0 ya da boş bırakılan kalem **sınırsız** demektir ve ekranda "sınırsız" diye
yazılır.

### 4.2 Yetkiler (hangi işler)

| Yetki | Kapalıysa |
|---|---|
| DNS kayıtlarını düzenleme | Kayıtlar görünür, değiştirilemez |
| DNSSEC | Bölüm görünmez |
| Posta (kutular, yönlendirme, otomatik yanıt) | Posta sekmesi görünmez |
| Veritabanı | Veritabanları menüsü ve sekmesi görünmez |
| Zamanlanmış görevler | Sekme görünmez |
| Dosya yöneticisi | Sekme görünmez |
| Yedek | Sekme görünmez |
| Sertifika: ücretsiz (ACME) | Sertifika alınamaz |
| Sertifika: kendi sertifikasını yükleme | Yükleme sekmesi görünmez |
| Korumalı klasörler, yönlendirmeler | Sekmeler görünmez |
| Site türü: vekil (proxy) | Seçenek listede yok |
| Uygulama kurucu (WordPress) | Bugünkü ürün izninin yerine geçer |
| VPN | Menü görünmez; bugünkü ürün izninin yerine geçer |
| Ek kullanıcılar (ekip) | Menü görünmez |

### 4.3 Yazılım (hangi bileşen, hangi sürüm)

D-017'nin teklif kimlikleri kullanılır. Birim sürümdür (D-014).

| Kalem | Örnek kimlik |
|---|---|
| PHP sürümleri | `component:php-fpm:8.3`, `component:php-fpm:8.5` |
| Node.js sürümleri | `component:node:22` |
| Veritabanı motorları | `component:mariadb`, `component:postgresql:16` |
| Web postası | `component:roundcube` |
| Sertifika sağlayıcıları | `integration:acme:letsencrypt` |

Bir kalem yalnız sunucuda kurulu ve yönetici tarafından sunuluyorsa hak
setine eklenebilir. Bir yazılım kalemi seçilmezse o yazılım kapalıdır.
Örneğin Node.js sürümü seçilmemişse "Node.js" site türü listede görünmez.
Sınırsız plan, yöneticinin sunduğu bütün kalemleri kendiliğinden kapsar;
sunucuya yeni bir PHP sürümü kurulup sunulduğunda Sınırsız plandakiler onu
hemen görür.

**Örnek iki plan:**

| | Temel | Geliştirici |
|---|---|---|
| PHP | 8.3 | 8.3, 8.5 |
| Node.js | yok | 20, 22 |
| Veritabanı | MariaDB | MariaDB, PostgreSQL 16 |
| DNS düzenleme | yok | var |
| Zamanlanmış görev | var | var |

### 4.4 Üçüncü taraf yazılımlar (ileride)

Sahip, 2 Kasım 2026: "Daha sonra üçüncü parti yazılımlara da izin vereceğiz;
bunlar da bu haklarda olacak veya olmayacak. Güvenlik önemli, dikkat edelim."

Mağazadan ya da bir üreticiden gelen her yazılım, hak setinde ayrı bir kalem
olur. Aynı kesişim kuralı geçerlidir; üçüncü taraf yazılım için ayrı bir izin
sistemi kurulmaz (D-017: tek teklif kimliği alanı). Kimlik örneği:
`extension:<üretici>:<ad>`. Güvenlik için ek kurallar:

- **Kurulmak sunulmak değildir.** Üçüncü taraf yazılım kurulduğunda hiçbir
  hesaba açık değildir. Yönetici onu önce **sunulan** katmana alır; bunu
  yaparken yazılımın istediği izinleri görür ve onaylar. Sunulduğu anda
  Sınırsız plan onu kendiliğinden alır (İ4); diğer planlara ve özel hak
  setlerine yönetici ya da bayi bilerek ekler. Güvenlik kapısı böylece
  "sunma" adımındadır ve Sınırsız planın adı doğru kalır.
- **Neye dokunacağını önceden söyler.** Üçüncü taraf yazılım, ihtiyaç duyduğu
  izinleri bir bildirimle açıkça listeler; örneğin "müşterinin dosyalarını
  okur", "veritabanı oluşturur", "dışarıya bağlantı kurar". Yönetici kurmadan
  önce bu listeyi görür ve onaylar. Listede olmayan bir şeyi yapamaz.
- **Hak setinin dışına çıkamaz.** Üçüncü taraf yazılım bir müşteri adına
  çalışırken o müşterinin hak setiyle sınırlıdır. Kendi başına hak veremez,
  hak genişletemez, plan değiştiremez.
- **Kapı panelde.** İzin denetimini üçüncü taraf yazılım değil panel yapar.
  Yazılımın kendi ekranı "izin var" dese bile panel reddederse işlem olmaz.
- **İmzalı ve kayıtlı.** Yalnız imzası doğrulanan paketler kurulabilir (bugünkü
  sürüm imzası düzeninin aynısı). Kurulum, izin onayı ve her plan değişikliği
  denetim kaydına yazılır.
- **Kaldırma çalışanı bozmaz, ama güvenlik önce gelir.** Kullanılan bir üçüncü
  taraf yazılım normalde kaldırılamaz (D-038). Ancak bir güvenlik sorunu
  bildirildiğinde yönetici onu hemen **durdurabilir**; neyin etkileneceği
  ekranda yazar.

Bu bölüm bir ilke koyar; mağaza ve üçüncü taraf paket düzeni ayrı bir
tasarımdır.

---

## 5. Roller ve hareket alanı

**Yönetici**
- Sunucuya neyin kurulacağını seçer.
- Kurulu olanlardan neyin **sunulduğunu** seçer. Bugünkü "müşterilere
  gösterilen sürümler" listesi bu katmana dönüşür ve PHP/Node.js dışındaki
  kalemleri de kapsar.
- Müşteri ve bayi planları yapar. Her hesaba bir plan bağlar ya da özel hak
  seti verir.
- Kendi çalışma alanı sınırsızdır (bugünkü gibi).

**Bayi**
- Kendi hak seti, yöneticinin ona bağladığı bayi planından ya da ona verdiği
  özel hak setinden gelir. Bayi hak seti, bugünkü havuzu (toplamlar ve müşteri
  sayısı) ve bayinin dağıtabileceği yetki ve yazılım kalemlerini birlikte
  tutar.
- Kendi planlarını yapar. Bir bayi planı, bayinin kendi hak setinin dışına
  çıkamaz.
- Müşterilerine kendi planlarından birini ya da yöneticinin bayiye açtığı
  genel planlardan birini bağlar, ya da özel hak seti verir.
- **Havuz gerçek kullanımı sınırlar, dağıtımı değil.** Sahip: "Bayiye 1 TB
  veri transferi izni verirsin, bayi 10 müşterisine her biri için 500 GB
  verir. Toplam teoride geçer ama pratikte geçememeli." Yani bayi
  müşterilerine havuzundan fazlasını dağıtabilir; ama bütün müşterilerinin
  **gerçek** toplam kullanımı havuzu aşamaz. Toplam havuza ulaşınca o bayinin
  hiçbir müşterisi daha fazlasını kullanamaz; yeni kullanım, kimin ne yapması
  gerektiği yazılarak reddedilir. Yüzde 80'de ve 100'de bayiye ve yöneticiye
  uyarı gider. Bu, bugünkü kuralın tersidir: bugün dağıtılan toplam
  sınırlanıyor.
- Sayı kalemlerinde (alan adı, veritabanı, posta kutusu, müşteri) gerçek
  kullanım zaten sayıdır: oluşturulmuş olanlar sayılır.
- Trafik ölçülmeye başlayana kadar trafik havuzu uygulanmaz ve ekranda böyle
  yazar.

**Müşteri**
- Yalnız kendi hak setinde açık olanı görür ve kullanır.
- "Planım" kartında planının adını ya da "Özel" yazısını, sınırlarını ve
  kullanımını görür. Daha fazlası için kime başvuracağı da yazar.

**Ek kullanıcı** (bugünkü model): Müşterinin hak setinin alt kümesini alır.
Müşterinin kendisinde olmayan bir yetkiyi ek kullanıcı hiç alamaz.

---

## 6. Plan ve abonelik davranışı

**6.1 Hesap oluşturma.** Formda "Haklar" bölümü her zaman görünür. Üç yol
var:
- **Sınırsız:** Formda adıyla seçili gelir. Hiçbir şey değiştirilmezse hesap
  Sınırsız olur. Bu bilerek görülen bir seçimdir; bugünkü gibi sessiz
  değildir.
- **Plan seç:** Liste, oluşturanın verebileceği planları gösterir.
- **Özel belirle:** Üç bölüm tek tek doldurulur. Yalnız oluşturanın kendi hak
  setindeki kalemler seçilebilir.

Bayi oluştururken de aynı bölüm görünür: Sınırsız bayi, bir bayi planı ya da
özel bayi hak seti. Bir bayinin oluşturduğu müşteri için "Sınırsız", o bayinin
kendi hak setinin tamamı demektir; bayinin alanının dışına çıkmaz.

**6.2 Plandan özel olmaya geçiş.** Plana bağlı bir abonelikte tek bir kalem
değiştirildiğinde, kaydetmeden önce şu sorulur:

> Bu değişiklikle hesap "Temel" planından çıkar ve "Özel" olur. Plana sonradan
> yapılan değişiklikler bu hesaba gitmez. Devam edilsin mi?

**6.3 Özelden plana dönüş.** Özel bir abonelik bir plana bağlanabilir. Önce
farklar gösterilir; kullanımın plandan büyük olduğu kalemler varsa bugünkü
"kullanım planın üstünde" kuralı geçerli olur. Hiçbir şey silinmez; yalnız yeni
kullanım engellenir.

**6.4 Plan düzenleme.** Kaydetmeden önce şu gösterilir:
- Bu plana bağlı kaç abonelik var.
- Daralan kalemler hangi aboneliklerde kullanılıyor. Örneğin: "PHP 8.5 kaldırılıyor;
  bu plandaki 3 sitede kullanılıyor. Siteler çalışmaya devam edecek, yeni
  sitede seçilemeyecek."
- Miktarı kullanımın altına inen abonelikler.

Özel abonelikler bu ekranda yer almaz, çünkü plandan çıkmışlardır.

**6.5 Plan silme.** Plana bağlı bir abonelik varsa reddedilir (bugünkü gibi).
Hazır gelen iki Sınırsız plan silinemez. Yönetici sınırlarını ve kalemlerini
değiştirebilir.

**6.6 Bayi hak seti daralırsa.** Yönetici bir bayinin hak setinden bir kalemi
kaldırırsa, bayinin planları ve müşterileri o kalemi kullanmayı sürdürebilir
(İ5). Ama hiçbiri o kalemi yeniden seçemez. Bayinin ekranında, hangi plan ve
müşterilerin etkilendiği listelenir.

---

## 7. Sunucudan kaldırma

Bugün bir PHP ya da Node.js sürümünü kaldırma, onu kullanan siteler varsa
reddediliyor. Bu kural genişler: kaldırma ekranı o kalemi açık tutan **planları
ve özel hak setlerini** de listeler. Bunlar kaldırmayı engellemez; yalnız
"bu kalem şu planlardan da düşecek" diye gösterilir. Engelleyen yine yalnız
gerçek kullanımdır: o sürümü kullanan siteler.

---

## 8. Devir

**Birim.** Devredilen şey bir **aboneliktir**: içindeki alan adları, siteler,
posta kutuları, veritabanları, sertifikalar ve zamanlanmış görevler birlikte
gider. Tek bir alan adını başka bir aboneliğe taşımak ayrı bir iştir ve bu
taslağın dışındadır.

**Kim devredebilir.**
- Yönetici: herkesten herkese.
- Bayi: yalnız kendi müşterileri arasında.
- Müşteri: kendisi devredemez (bkz. Karar K3).
- İki farklı bayinin müşterileri arasında devri yalnız yönetici yapar.

**Ön denetim** (hiçbir şey değişmeden önce):
1. **Alanın tavanı:** Abonelik hak setiyle birlikte gider. Alan taraf bir
   bayiye bağlıysa, o bayinin havuzunda bu aboneliğin verdiği miktarlar kadar
   boş yer olmalıdır. Bayinin hak setinde, aboneliğin açık tuttuğu her yetki ve
   yazılım kalemi bulunmalıdır.
2. **Uymazsa devir başlamaz.** Ekran, eksik olan her kalemi ve yolları yazar:
   - Yönetici bayinin hak setini genişletir.
   - Ya da abonelik önce daraltılır. Bu, yalnız gerçek kullanım daralan hak
     setine sığıyorsa mümkündür.
   - Ya da abonelik olduğu yerde kalır.
3. **Ek kullanıcılar:** Eski sahibin ek kullanıcılarının bu abonelikteki
   izinleri devirde kaldırılır. Ekranda önceden yazılır.
4. **Önizleme:** Neyin kime geçeceği, hangi izinlerin kalkacağı ve havuzlarda
   neyin değişeceği gösterilir, sonra onay istenir.

**Sunucu tarafı.** Devrin sunucuda dosya, sistem kullanıcısı ya da hizmet
yapılandırması değiştirmesi gerekip gerekmediği, uygulama tasarımında
doğrulanacak (bkz. Bölüm 11). Gerekiyorsa devir, D-025'teki değişiklik
kurallarına uyan bir işlem olur: önizleme, kalıcı kontrol noktaları, geri alma.

---

## 9. Mevcut kurulumlara geçiş

Hiçbir site, posta, veritabanı ya da hizmet değişmez. Değişen yalnız panelin
kayıtlarıdır.

| Bugün | Geçişten sonra |
|---|---|
| Plana bağlı abonelik | Aynı plana bağlı. Plan bugünkü sayılarını korur; yetki ve yazılım bölümleri "yöneticinin sunduğu her şey" olarak doldurulur. Davranış değişmez. |
| Plansız abonelik (elle sayılar) | Özel. Bugünkü sayılar korunur; yetki ve yazılım "sunulan her şey". |
| Aboneliği olmayan müşteri | Sınırsız (İ3: plana bağlı olmayan Sınırsız'dadır). Bugün bu hesap hiçbir şey oluşturamıyor; geçişten sonra oluşturabilir. **Bu bir davranış değişikliğidir;** geçişten sonra yöneticiye bu hesapların listesi gösterilir. |
| Havuzu olmayan bayi | Sınırsız bayi. Davranış değişmez. |
| Havuzu olan bayi | Özel bayi hak seti: bugünkü havuz sayıları; yetki ve yazılım "sunulan her şey". |
| VPN ve uygulama kurucu ürün izinleri | Aboneliğin hak setinde yetki olarak. İzni olmayanda kapalı. |
| "Müşterilere gösterilen sürümler" listesi | Yöneticinin "sunulan" katmanı. |

Geçişten sonra yönetici her şeyi bugünkü gibi bulur, ama artık hangi hesabın
hangi planda ya da özel olduğunu görür ve daraltmaya başlayabilir.

**Geri dönüş.** Yeni tablolar eklenir, eski sütunlar silinmez. Önceki sürüme
geri dönülürse, eski panel yeni tabloları görmez ve bugünkü gibi çalışır.
Uygulama aşamasında bu, D-025'in istediği gibi bir geri dönüş denemesiyle
kanıtlanır.

---

## 9A. Abonelik bitince: askıya alma

Sahip, 2 Kasım 2026: "Müşterinin ya da bayinin aboneliği biterse ilgili tüm
siteler askıya alınmalı. Bir şey askıya alındıysa 'bu site askıya alınmıştır'
denir. Hiçbir servis, VPN olsun veritabanı olsun, hizmet vermez. Belki hosting
değil sadece veritabanı olarak kullanıyor olabilir; izin vermemek lazım."
Önceki cevap: "Belli bir süre sonra elle silinir."

**Ne zaman.** Aboneliğin bitiş tarihi geçtiğinde ya da yönetici veya bayi
aboneliği bitirdiğinde. Bir bayinin aboneliği bitince, o bayinin bütün
müşterilerinin abonelikleri de askıya alınır.

**Ne olur.** Askıya alınan aboneliğin hiçbir hizmeti çalışmaz:

| Hizmet | Askıdayken |
|---|---|
| Web siteleri | Ziyaretçi "Bu site askıya alınmıştır" sayfasını görür |
| Posta | Gelen posta kabul edilmez; posta kutularına giriş yapılamaz |
| Veritabanları | Veritabanı kullanıcıları bağlanamaz |
| VPN | Cihazlar bağlanamaz |
| Zamanlanmış görevler | Çalışmaz |
| Dosya erişimi (FTP/SFTP) | Giriş yapılamaz |
| Panel | Hesap sahibi giriş yapınca yalnız "Aboneliğiniz askıda; şu kişiye başvurun" ekranını görür |

**Hiçbir şey silinmez.** Dosyalar, posta kutuları, veritabanları ve
yapılandırma olduğu gibi durur. Abonelik yeniden başlatılınca her şey aynen
geri gelir.

**Silme elle yapılır.** Askıdaki bir abonelik belirli bir süre sonra
yöneticinin ya da bayinin listesinde "silinebilir" olarak görünür. Silmeyi
yalnız bir kişi başlatır; panel kendiliğinden silmez. Bekleme süresi K6'da.

**Güvenlik ve dayanıklılık.**
- Askıya alma ve geri alma birer değişikliktir (D-025): önce neyin
  değişeceği gösterilir, sunucuda kontrol noktalarıyla yapılır, yarıda kalırsa
  aynı işlem kaldığı yerden sürer.
- Askıya alma, her hizmetin kendi standart ayarıyla yapılır (D-022). Panel
  kaldırılsa bile askıdaki site askıda, açık site açık kalır.
- Bugünkü panelde siteyi askıya alma yok; hesap dondurulabiliyor ama siteler
  çalışmaya devam ediyor. Bu bölüm yeni bir parçadır.

---

## 10. Plesk ve cPanel'den alınan dersler

Kaynaklar: Plesk Obsidian ve cPanel/WHM belgeleri ile destek makaleleri
(2 Kasım 2026 araştırması). Doğrulanamayan noktalar belirsiz olarak kaldı.

| Onlarda görülen sorun | Bu tasarımdaki karşılığı |
|---|---|
| Plesk: plandan koparılan abonelik sessizce kopuk kalır, plan değişikliği ona gitmez ve bu fark edilmez | Durum yalnız ikidir ve her ekranda yazar: "Plan: Temel" ya da "Özel". Kopma bir onay sorusuyla olur. |
| Plesk: plan eşitleme hatası geç ve rehbersiz çıkar ("limitations exceeded") | Plan düzenleme kaydetmeden önce etkilenenleri gösterir; ret mesajı kimin ne yapacağını söyler (D-024). |
| Plesk: kaldırılan PHP sürümünü kullanan siteler 503 verir; planlarda eski referans kalır | Kullanılan sürüm kaldırılamaz; kaldırma ekranı planları da gösterir. |
| cPanel: PHP sürüm kısıtı yalnız ekranda, `.htaccess` ile aşılır | Kural sunucuda zorlanır (İ6). |
| cPanel: hesap paketinden kopabilir, paket değişikliğinin hesaplara gidip gitmediği belirsiz | Plana bağlı hesap her zaman planla aynıdır; özel hesap hiç etkilenmez. Ara durum yoktur. |
| cPanel: bayi sınırları varsayılan olarak kapalı ve görünmez | Her bayinin durumu adıyla yazar: Sınırsız bayi, bir plan ya da Özel. Sınırsızlık görünür bir seçimdir. |
| Plesk: devirde alan bayinin kapasitesi denetlenir (iyi örnek) | Aynısı, yetki ve yazılım kalemleri de eklenerek. |

---

## 11. Uygulamada doğrulanacaklar

Bu taslak, kodu okuyarak hazırlandı. Uygulama başlamadan önce şunlar
doğrulanacak:
- Bir aboneliğin siteleri sunucuda hangi sistem kullanıcısıyla çalışıyor;
  devirde dosya sahipliğinin değişmesi gerekiyor mu?
- Ek kullanıcı tetikleyicileri (`029_additional_users.sql`) devirde sahibin
  değişmesine nasıl izin verecek?
- Her oluşturma ve değiştirme yolu tek bir denetimden mi geçiyor? Bugün bir
  veritabanı yolu kota denetimini atlıyor; bu tür yollar tek tek listelenecek.
- Sınırsız planın "yeni kurulan sürümü kendiliğinden kapsaması" ile özel hak
  setlerinin "yalnız seçilenleri kapsaması" aynı kesişim kuralıyla nasıl
  saklanacak?

---

## 12. Sahibin vermesi gereken kararlar

- **K1 — Rol kuralı. KARARLANDI:** "Her hesabın hakları ya bir plandan gelir
  ya da o hesaba özel seçilir." ROLES.md buna göre güncellenecek (İ3).
- **K2 — Fazla satış. KARARLANDI:** Dağıtım havuzu aşabilir; gerçek toplam
  kullanım aşamaz (Bölüm 5, Bayi).
- **K3 — Müşterinin devir isteği. KARARLANDI:** Müşteri istek gönderir; bayisi
  ya da yönetici onaylar.
- **K4 — Plansız hesaplar. KARARLANDI:** Bir plana bağlı olmayan ve özel hakkı
  olmayan hesap Sınırsız'dadır (İ3). Abonelik bitince askıya alınır, belirli
  bir süre sonra elle silinir (Bölüm 9A).
- **K5 — Sınırsız plan ve yeni gelenler. KARARLANDI (sahip, 2 Kasım 2026):**
  Sınırsız plan yönetici tarafından sunulan her yeni şeyi kendiliğinden alır;
  almıyorsa adı Sınırsız olamaz. Üçüncü taraf yazılım için güvenlik kapısı
  "sunma" adımıdır (Bölüm 4.4).
- **K6 — Askıdaki abonelik ne zaman "silinebilir" olur?** Öneri: 30 gün;
  yönetici bu süreyi değiştirebilir. Silme her zaman elle yapılır.
- **K7 — Abonelik bitiş tarihi kimde?** Bugün panelde fiyat ve ödeme yok.
  Öneri: yönetici ve bayi her aboneliğe isteğe bağlı bir bitiş tarihi girer;
  tarih yoksa abonelik kendiliğinden bitmez, yalnız elle bitirilir.

---

## 13. Önerilen uygulama sırası (onaydan sonra)

Her adım ayrı, küçük bir sürümdür; her biri kendi testleriyle çıkar.
1. **Veri modeli ve geçiş:** Hak seti tabloları, iki Sınırsız plan, mevcut
   kayıtların taşınması. Ekranda yalnız "Plan: X / Özel" görünür. Gerçek
   sunucuda bir güncelleme ve bir geri dönüş denemesi.
2. **Sunucu tarafı kapılar:** Her seçim ve oluşturma yolu kesişim kuralını
   uygular; reddetme mesajları kimin ne yapacağını söyler.
3. **Ekranlar:** Hesap oluşturma formu, plan düzenleme önizlemesi, abonelik
   sayfası, menü ve sekme gizleme, "Planım" kartı.
4. **Devir:** Ön denetim, önizleme ve gerekiyorsa sunucu tarafı işlem.

**Kapsam dışı:** fiyat, dönem ve ödeme; trafik ölçümü; işletim sistemi disk
kotası; tek bir alan adının abonelikler arasında taşınması.
