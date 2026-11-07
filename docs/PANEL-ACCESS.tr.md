# Panel oturumları ve adrese göre yönetici erişimi

Durum: alpha.128 (açık madde G4). İngilizce: [PANEL-ACCESS.md](PANEL-ACCESS.md).

## Ne yapar

- **Açık oturumlar** (Hesabım): hesabın açık olduğu her tarayıcı, son kullandığı adres ve
  tarayıcıyla; tek oturum için "Sonlandır" ve "Diğer bütün oturumları sonlandır". Parola
  değişikliği zaten hesabın bütün oturumlarını kapatır.
- **Bütün hesapların oturumları** (Güvenlik, yönetici): şu an kim giriş yapmış, "Sonlandır" ile.
- **Boşta kalma süresi** (Güvenlik → Panel erişimi): 15, 30, 60 (varsayılan), 120, 240 ya da
  480 dakika kullanılmayan oturum bir sonraki isteğinde kapanır. Yazmak, tıklamak, sayfaya
  dokunmak ve her değişiklik kullanım sayılır; kendiliğinden yenilenen sayfalar sayılmaz.
- **Yönetici adresleri** (Güvenlik → Panel erişimi): liste boş değilse yönetici girişleri ve
  yönetici oturumları (bir yöneticinin açtığı "Farklı giriş yap" oturumları dahil) yalnız
  listedeki adres ve ağlardan kabul edilir. Müşteriler ve bayiler hiç etkilenmez. Liste
  panelin kendisinindir: SSH, posta, web siteleri ve güvenlik duvarı değişmez.

## Dışarıda kalmaya karşı

1. Boş olmayan listeyi kaydeden adres listede olmalıdır; değilse panel kaydetmez ve nedenini
   söyler. Ekran "Adresimi ekle" düğmesini sunar.
2. Sunucunun kendisi (127.0.0.1 ve ::1) her zaman izinlidir; SSH tüneli her zaman çalışır:

   ```
   ssh -L 1923:127.0.0.1:1923 root@sunucunuz
   ```

   sonra `https://localhost:1923` adresini açın (kendi panel portunuzu kullanın).
3. **Yerel kurtarma komutu.** Sunucuda root olarak:

   ```
   sudo touch /etc/celikpanel/panel-access-open
   ```

   Birkaç saniye içinde yöneticiler her adresten giriş yapabilir. Listeyi Güvenlik → Panel
   erişimi altında düzeltin, sonra dosyayı kaldırın:

   ```
   sudo rm /etc/celikpanel/panel-access-open
   ```

   Dosya durdukça Panel erişimi kartı bunu söyler. Bu dosyayı başka hiçbir şey okumaz.

## Nerede saklanır

`panel_settings` satırları `panel.sessions.idle_minutes` ve `panel.access.allowlist` (JSON
liste). Oturumlar `last_seen_at`, `client_ip` ve `user_agent` alır (göç 069); `last_seen_at`
oturum başına dakikada en çok bir kez yazılır. Adres bağlantının kendi adresidir
(`RemoteAddr`); panele vekil sunucu üzerinden değil doğrudan ulaşılır.

## Denetim kaydı

`auth.session.end`, `auth.session.end_others`, `panel.access.settings` ve listenin reddettiği
yönetici girişi için `auth.login.address_refused`.
