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
  `app/src` içindeki kodu değil.

## Kanıt

<!-- INTEGRATION -->

<!-- LAB -->
