# v0.1.0-alpha.119

[English](RELEASE-NOTES-v0.1.0-alpha.119.md)

Panelin içinde kullanım kılavuzu, yeni site korumasında çalışan yüklenen
programlar ve Python web uygulamaları, yönetici için geliştirici araçları ve
alpha.118'in yapay zekâ uygulamaları için daha güçlü bir koruma. Yüklenen her
program herhangi bir kod sayılır: sınır programlama dillerinin listesi değil,
korumanın kendisidir. **Kurulu panelleri yalnız sahibi, panelin kendi güncelleme
ekranından günceller.**

## Kullanım kılavuzu

- Her rol (yönetici, bayi, müşteri) için bir kılavuz artık panelin parçası.
  Üst çubuktaki "?" bulunduğunuz ekranın bölümünü açar; /help bütün kılavuzu
  arama ile gösterir.
- İlk girişte kısa bir tur ekranın ana bölümlerini gösterir. Atlanabilir ve
  istendiğinde yeniden izlenebilir ("Turu başlat"); asla zorunlu değildir.
- Yönetici kılavuzu bir bayinin ya da müşterinin gördüğü gibi görebilir. Rol
  yalnız neyin ilgili olduğunu seçer; bir güvenlik sınırı değildir.
- Aynı kılavuz Türkçe ve İngilizce olarak indirme sitesinde de yayımlanır
  (`/guide/`).

## Yüklenen programlar

- Müşteri kendi derlenmiş programını (Go, Rust ya da sunucunun işlemcisi için
  derlenmiş başka bir program) sitesi olarak çalıştırabilir. Program dosya
  yöneticisi ya da SFTP ile sitenin `app/bin` klasörüne yüklenir; CelikPanel
  kaynaktan hiçbir şey derlemez.
- Program sitenin kendi hesabıyla, site korumasında, yalnız sunucunun
  ulaşabildiği tek bir portta çalışır. Ziyaretçiler ona yalnız HTTPS ile, site
  üzerinden ulaşır.
- Yayımlama programı denetler, başlatır, systemd'nin gerçekten yüklediği
  korumayı kanıtlar, programın dinlemesini ve yanıt vermesini bekler; site
  ancak bundan sonra programa geçer. Her hata nedenini, kimin ne yapacağını ve
  sonraki adımı söyler.
- Yönetici risk özetini okuyup yüklenen programları sunana kadar hiçbir şey
  açılmaz; yönetici onları güvenlik beklemesine de alabilir.

## Python web uygulamaları

- Django, Flask, FastAPI ve diğer WSGI ya da ASGI uygulamaları aynı altyapıda
  çalışır. Kod `requirements.txt` dosyasıyla birlikte `app/src` klasörüne
  yüklenir.
- Yalnız dağıtımın kendi Python'u kullanılır (Debian 12: 3.11, Debian 13: 3.13,
  Ubuntu 22.04: 3.10, Ubuntu 24.04: 3.12, Arch: 3.14). Yönetici onu
  Bileşenler'den kurar ve her Python sürümü için ayrı sunar.
- Her yayımlama, çalışan uygulama çalışmaya devam ederken, sitenin hesabıyla ve
  korumalı bir kurulum biriminde PyPI'den yeni bir ortam kurar. gunicorn ve
  uvicorn parmak iziyle sabitlenmiştir. Yeni sürüm açılmazsa önceki ortam geri
  konur.
- Başka yerden kod getirebilecek `requirements.txt` satırları (`-e`,
  bağlantılar, yerel yollar, başka paket dizinleri) reddedilir; ekran satırı
  adıyla gösterir.

## Geliştirici araçları (yalnız yönetici)

- Go ve Rust, Bileşenler'de "Geliştirici araçları (yalnız yönetici)" altında,
  işletim sisteminin resmi paketlerinden kurulabilir.
- Bunlar yöneticinin kendi araçlarıdır: bayilere ya da müşterilere asla
  sunulamaz ve SFTP/SSH kafesinde bulunmaz.

## Yapay zekâ uygulamaları için daha güçlü koruma

- n8n ve Open WebUI yeni site korumasına geçer: panelin kendi soketi ve diğer
  sitelerin PHP soketleri gizlenir, bulut üst veri adresi engellenir, uygulama
  yalnız kendi portunda dinleyebilir ve 25, 465 ya da 587 portlarından, yerel
  posta sunucusu dahil, posta gönderemez.
- Uygulamanın klasörü her başlatmadan önce korumaya sunucunun kendisi
  tarafından bağlanır; sitenin koyabileceği bir bağlantı asla izlenmez.
  Uygulamanın ayarları alpha.118 güvenlik düzeltmesinin getirdiği korumalı
  sunucu klasöründe kalır.
- alpha.118 ile kurulmuş uygulamalar olduğu gibi çalışmaya devam eder. Yönetici
  onları yapay zekâ uygulamaları sayfasındaki **Yeni korumayı uygula** düğmesiyle
  bir kez taşır; çalışan her uygulama bir kez yeniden başlar, denetlenir ve geri
  gelir. Sahibin elle değiştirdiği bir birim korunur ve listelenir.

## Yeni haklar

Yüklenen programlar (`program_apps`) ve Python uygulamaları (`python_apps`) yeni
yetkilerdir; `runtime:program` ve `component:python:<sürüm>` yeni sunumlardır.
Hepsi yönetici sunana kadar kapalı başlar. Geliştirici araçları asla bir hak
değildir.

## Bilinen sınırlar

- Programın kendi dışa giden trafiği aboneliğin trafiğine sayılmaz.
- Yalnız 25, 465 ve 587 posta portları engellenir; 2525 portu ve HTTP posta
  servisleriyle gönderilen posta engellenmez.
- Ubuntu 22.04 eksik koruma düzeyinde çalışabilir. Panel o zaman her eksik
  korumayı adıyla söyler ve yönetici orada program sunmadan önce bunu kabul
  etmelidir.
- Python uygulamaları varsayılan olarak yalnız hazır paketleri (wheel)
  kullanır; paketleri kaynaktan derlemek her sunum için yöneticinin açacağı bir
  seçenektir.
- Go ve Rust dağıtımın sürümleridir; en yeni sürümden eski olabilirler
  (Debian 12: Go 1.19, Ubuntu 22.04: Go 1.18).
- Bir Python yayımlaması başarısız olursa geri alma önceki ortamı geri koyar,
  `app/src` içindeki kodu değil. Yüklenen kodun kendisi bozuksa site, çalışan kod
  yeniden yüklenene kadar yanıt vermez; ekran bunu söyler. Önceki kodun bir
  kopyasını saklayın.
- Durdurma sinyalini yok sayan bir program 30 saniyeye kadar geç durabilir.
- Laboratuvar Debian 13, Ubuntu 24.04 (alpha.118'den güncelleme) ve Arch'ı denedi.
  Debian 12 ve Ubuntu 22.04 bu sürümde gerçek sistemde denenmedi.

## Kanıt

Birleştirilmiş ağaç (`release/alpha119`): Go derlemesi, vet ve gofmt temiz;
`internal` ve `cmd/panel` testleri geçiyor; Ajan testlerinde yalnız alpha.118'de
de başarısız olan, bilinen 19 konuk-ortamı DNS ve posta hatası var, yenisi yok;
2424 web testi geçiyor, kılavuzun üretilen dosyaları kaynaklarıyla aynı ve
derleme paket bütçelerinin içinde; sürüm kurtarma örnekleri şema 63'e
genişletildi; indirme sitesi ve sürüm sırası sözleşmeleri geçiyor (118 → 119).

Laboratuvar (QEMU konukları, `lab/alpha114-acceptance` dalı, `792b8a58f`, kanıt
`final-alpha119-run1..run4-20261104`). 1–3. turlar yukarıda düzeltilen Python
kurulum hatalarında durdu. Son aday üzerindeki 4. tur geçti:

- Sıfırdan Debian 13: hazır paketlerle Flask (HTTPS) ve FastAPI; bozuk bir
  yeniden yayımlama belgelendiği gibi önceki ortama döndü; n8n yeni korumada
  kuruldu, çalıştı, sahibi girdi; hiç yayımlanmamış programı durdurmak düz bir
  yanıt veriyor.
- Kaçma denemeleri, yüklenen Go programı ve Python uygulaması, Debian 13: her
  deneme reddedildi. Bunlar arasında başka sitelerin ve root dosyalarının
  okunması, panelin soketleri, yetki değişimi, başka süreçler, ham soketler,
  25, 465 ve 587 posta portları (internete ve sunucunun kendi postasına), bulut
  üst verisi, uygulama klasörü dışına yazma ve uygulama klasörünü bir bağla
  değiştirme var. Başka port açma değerlendirilemedi: sunucunun güvenlik duvarı
  iki dinleyiciyi de düşürüyor.
- Sıfırdan Arch: Python 3.14 Flask; yüklenen programda posta portu ve klasör bağı
  denemeleri reddedildi.
- Bu kodla önceki turlar: kılavuz ve ilk giriş turu, yalnız yöneticiye Go,
  yüklenen programın yayımlanması, askıya alınması, sürdürülmesi ve kaldırılması;
  Ubuntu 24.04 alpha.118'den 61 saniyede güncellendi ve n8n "Yeni korumayı
  uygula" ile yeni korumaya geçti, ardından posta portları reddedildi.
- Denenmeyenler: Debian 12, Ubuntu 22.04 (laboratuvar görüntüsü yok), geri dönüş.
