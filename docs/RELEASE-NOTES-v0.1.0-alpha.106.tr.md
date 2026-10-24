# v0.1.0-alpha.106

[English](RELEASE-NOTES-v0.1.0-alpha.106.md)

Bu, v0.1.0-alpha.105'i izleyen adaydır. **Yayımlanmadı ve hiçbir parçası kurulu bir
sunucuda değil.** 24 Ekim 2026'da `fix/alpha106-clear-removal` dalında, alpha.105'in
yayım kaydından sonraki `main`'den yazıldı (alpha.105 sürüm commit'i
`0a51195cd294be5f009f2a2b72e1dd372094e524`).

## Kim güncellemeli, neden

Bileşen kaldıran herkes. Frankfurt'ta sahip PHP-FPM satırının kaldır düğmesine bastı
(8.6, 8.5 ve 8.4 kurulu) ve sordu: "kaldır dersem hangi versiyonu kaldıracak?"
Pencere bunu başlığında söylemiyordu ve reddi ortaya gömülmüştü (D-037 değişikliği,
24 Ekim 2026).

Bir sunucuyu yalnız sahibi, panelin kendi güncelleme ekranından günceller. Sahibin
gördükleri [işlem yönlendirmesinde](OPERATION-GUIDANCE.tr.md).

## Hangi sürüm

- PHP-FPM'in birden çok sürümü varken kaldır düğmesi önce **"PHP-FPM: hangi sürüm
  kaldırılsın?"** diye sorar: her sürüm için kimin kullandığıyla bir seçim ("Kullanan:
  phpPgAdmin, Roundcube") ve altta ayrı **Tüm sürümler (8.6, 8.5, 8.4)**. Orada
  hiçbir şey değişmez.
- Bir sürüm o sürümün kendi kaldırmasına geçer (yalnız araçlar engelse "önce taşı,
  sonra kaldır" ile); Tüm sürümler bütün bileşenin kaldırmasına.
- Tek sürümde soru yoktur; başlık onu adlandırır ("PHP-FPM 8.4 kaldırılsın mı?").
  Her başlık neyin gideceğini söyler ("PHP-FPM — tüm sürümler (8.6, 8.5, 8.4)
  kaldırılsın mı?").

## Pencere yanıtla başlar

- Önce: **"PHP-FPM şu an kaldırılamaz"** (kaldırılabiliyorsa "Kaldırılacak: …"),
  sonra her neden bir kez, sade sözle.
- Sonra **Ne yapabilirsiniz** ve nasıl yapılacağı: "Önce kaldırın: phpPgAdmin" o
  kaldırmayı açar; siteler için Alan adları; "Ya da PHP-FPM kalsın: bu pencereyi
  kapatın, hiçbir şey değişmez."
- Paket listesi, apt ya da pacman'ın yaptığı ve paket yöneticisinin kendi açıklaması
  **Ayrıntılar** altındadır. Reddedilen kaldırma için asla paket "götürür" denmez.
- Reddedilen kaldırma **Kapat** sunar; tehlike düğmesi **"Kaldırılamaz"** der ve
  görünür biçimde kullanılamaz. Bu artık siteleri ya da veritabanları barındıran
  bileşen için de geçerlidir (Panel onları önceden de reddediyordu).

## Sınırlar

- Node.js sürümleri yalnız sürüm listesinden kaldırılır; PostgreSQL için Panel'de
  sürüm başına kaldırma yoktur.
- Yalnız web: Agent ya da Panel değişikliği yok; sıra 106, şema 44 değişmedi.
- Kurulu bir sunucuda ölçülmedi. Web 1805 test geçti ve üretim derlemesi
  bütçelerinin içinde; hiçbir Go paketi değişmedi.
