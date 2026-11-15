# v0.1.0-alpha.136

[English](RELEASE-NOTES-v0.1.0-alpha.136.md)

Tek başına çıkan bir dayanıklılık düzeltmesi. Panel bazı işlere başlamadan
önce "şu anda bu sunucuda bir şey değişiyor mu?" diye sorar. Yoğun bir
sunucuda bu denetim, aslında bir değişiklik sürerken "hayır" diyebiliyordu.
Artık cevap vermeden önce her sürece bakar.
**Kurulu panelleri yalnız sahip, panelin kendi güncelleme ekranından
günceller.**

## alpha.135'ten güncelleme

1. Her zamanki gibi panelin kendi güncelleme ekranından güncelleyin. Bu
   sürüm panelin veritabanı düzenini değiştirmez (alpha.135'teki hâlinde
   kalır); sonradan alpha.135'e dönmek veritabanı geri yüklemesi
   gerektirmez.
2. Güncellemeden sonra basılacak ya da ayarlanacak bir şey yok. Değişiklik
   denetimin içinde; hiçbir ekran değişmez.

## Sizin için ne değişiyor

### Sorun neydi

Panel bir güncellemeden önce, kurtarma aracı yeni sürümüne geçirilmeden
önce, bir sayfa sunucunun bir değişikliğe hazır olup olmadığını sorduğunda
ve posta kurulumu işçisine devredilmeden önce, başka bir değişikliğin
sunucunun değişiklik kilidini tutup tutmadığına bakar.

Cevap için sistemin bütün dosya kilitlerinin listesini okuyordu. Pek çok
programın sürekli kilit alıp bıraktığı bir sunucuda (veritabanları, posta,
başka hizmetler) bu liste, bir satırın atlandığı bir anda okunabilir.
Atlanan satır süren değişikliğin satırıysa denetim, bir şey değişirken "hiçbir
şey değişmiyor" diyordu. Bu kusur, denetimin kilidi bir an almayı bırakıp
listeyi okumaya başladığı alpha.113'ten beri her sürümde vardı. CelikPanel'in
kendi otomatik denetimlerinden biri bu yüzden ara sıra başarısız olduğu için
bulundu.

### Neye yol açabilirdi

Denetimin "boş" demesi kendi başına hiçbir şeyi değiştirmez ve çoğu
değişiklik ardından kilidi kendisi almaya çalışır, tutuluyorsa durur. Ama
bazı adımlar yalnız bu denetime dayanır:

- güncellemenin ilk denetimi, bir Agent işlemi hâlâ sürerken güncellemenin
  devam etmesine izin verebilir, güncelleme de sonra bekler ya da dururdu;
- güncelleme sırasında kurtarma aracının yeni sürümüne geçişi bu denetime
  dayanır (orada hâlâ açık olan için "Bilinen sınırlar"a bakın);
- kilidi kimin tuttuğunun kaydı, tutan hâlâ tutarken "adı alınamadan
  bıraktı" diyebilirdi.

### Ne değişiyor

- Kilit listesi bir tutucunun adını verirse cevap önceki gibi "meşgul".
- Kilit listesi kimseyi göstermezse panel artık "boş" demeden önce her
  sürecin açık dosyalarına da bakar. Bu okuma kilidi almaz, bu yüzden
  kendisi gerçek bir değişikliği bekletemez. Dört bin süreçli bir sunucuda
  yarım saniye kadar sürer ve en çok beş saniyede durur.
- Bu tam okuma yapılamıyorsa, örneğin panele başka süreçlere bakma hakkı
  vermeyen bir sistemde, panel alpha.113'ten önceki sürümler gibi cevap verir:
  kilidi bir an alır ve bırakır. Cevap yine kesindir. "Bilinen sınırlar"a
  bakın.
- Kilidi kimin tuttuğunun kaydı "adı alınamadan bıraktı" sözünü yalnız tam
  okuma kimseyi bulamadığında söyler; aksi hâlde tutanın belirlenemediğini
  söyler.

### Ne görürsünüz

Yeni bir şey yok. Gerçekten bir şey sürüyorsa güncelleme, Agent'ın başka bir
sunucu işlemini bitirdiğini söyleyerek en çok 60 saniye bekler; o zaman hâlâ
sürüyorsa mevcut açıklamayla ve "Güncellemeyi yeniden başlatmak güvenlidir"
sözüyle hiçbir şeyi değiştirmeden durur. Bu iletiler değişmedi; artık yalnız
doğru olduklarında gösterilir.

## Bilinen sınırlar

- **Başka bir süreç ad alanındaki tutucu.** Kilidi panelin göremediği ayrı
  bir süreç ad alanından tutan bir program iki okumayla da bulunmaz.
  CelikPanel'in kendi programları panelin gördüğü yerde çalışır.
- **Açık dosya olmadan tutulan kilit.** Dosyayı artık açık tutmayan bir
  programın canlı tuttuğu kilit (seyrek bir programlama yöntemi) önceki gibi
  yalnız kilit listesinde görünür.
- **Tam okumanın yapılamadığı sistemler.** Orada denetim kilidi bir an alır.
  Tam o anda başlayan gerçek bir değişiklik "meşgul" cevabı alır ve bekler ya
  da yeniden denenir; aynı anda iki denetim de birinin "meşgul" demesine yol
  açabilir. İkisi de kilit tutulurken denetimin "boş" demesine yol açamaz.
  CelikPanel'in Agent'ı tam okumanın gerektirdiği haklarla çalışır.
- **Kendi değişikliği için açık: kurtarma aracının geçişi kilidi tutmaz.**
  Bir güncelleme kurtarma aracını yeni sürümüne geçirirken hiçbir şeyin
  değişmediğini denetler ama geçiş boyunca değişiklik kilidini tutmaz. Arada
  başlayan bir panel işlemi (DNS motoru geçişi, posta kurulumu, güvenlik
  duvarı değişikliği) durdurulmaz. Sonraki bir kurtarma, yeni kurtarma
  aracının hiç denetlenmediği kayıtlarla karşılaşıp reddedebilir. Bu sürüm
  denetimin kendisini kesinleştirir; denetimle geçiş arasındaki boşluk kalır
  ve kendi değişikliği olarak planlandı.
- Başlatılamayan bir kurtarma denetimi hâlâ "desteklenmeyen protokol"
  olarak bildiriliyor (alpha.137 listesinde).

## Laboratuvarda denenenler

**Laboratuvar denetiminden sonra doldurulacak.** Bu sürümde henüz
laboratuvar denemesi yapılmadı.

## Kanıt

Şema değişikliği yok: şema 079, alpha.135'teki gibi. Kalıcı hiçbir kayıt
değişmez. Değişiklik `internal/hostmutationlock` içinde
(`descriptors_linux.go`, `holders_linux.go`, `lock_linux.go`) ve
`cmd/agent/mail_enrollment_worker_linux.go` içinde bir açıklama satırı.
Bileşen testleri: `internal/hostmutationlock/descriptors_linux_test.go`
(kilit listesinde olmayan tutucu bulunur; tamamlanamayan okuma anlık kilitle
karar verir; yetki olmadan da cevap kesindir; anlık kilit sırasında bir
yazıcının karşılaştığı) ve her denetimin tutucuyu adlandırması gerekirken
bilerek kilit alıp bırakan bir gerileme testi. alpha.135'in kodunda bu test
root ve yetkisiz kullanıcıyla 20 denemenin 20'sinde, yarış denetleyicisiyle
5'in 5'inde başarısız oldu; bu sürümde her denemede geçti. CelikPanel'in
başka bir süreci adlandırma testi, değiştirilmeden, aynı tür yük altında
alpha.135'te 300 denemenin 47 ve 38'inde, bu sürümde hiçbirinde başarısız
olmadı. Yeni bir CI adımı bu testleri root olarak çalıştırır. Gerçek bir
sunucuda henüz deneme yok ("Laboratuvarda denenenler"e bakın).
**Hiçbir P0 işi kapanmadı.**
