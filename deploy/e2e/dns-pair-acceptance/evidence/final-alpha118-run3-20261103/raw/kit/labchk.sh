# labnet (root): file access / mail / site probes against the lab server. Secret on stdin (first line). Lab only.
# usage: labchk.sh MODE IP USER [HOST]   modes: sftp ssh ftps ftpplain imap smtp site
MODE=$1; IP=$2; U=$3; HN=${4:-}
read -r PW
export SSHPASS="$PW"
for t in sshpass lftp; do command -v $t >/dev/null || DEBIAN_FRONTEND=noninteractive apt-get install -y -q $t >/dev/null 2>&1; done
O="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o PubkeyAuthentication=no -o PreferredAuthentications=password,keyboard-interactive -o ConnectTimeout=10 -o LogLevel=ERROR"
K="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o IdentitiesOnly=yes -o PreferredAuthentications=publickey -o ConnectTimeout=10 -o LogLevel=ERROR"
case $MODE in
sftp) printf 'pwd\n-ls -la\n-ls /etc\n-cd /etc\n-pwd\n-ls /\n' | timeout 40 sshpass -e sftp $O -oBatchMode=no -b - $U@$IP 2>&1 | head -30; echo "sftp rc=${PIPESTATUS[1]}" ;;
sftpk) printf 'pwd
-ls -la
-ls /etc
-cd /etc
-pwd
-ls /
' | timeout 40 sftp -i /root/fa117key $K -b - $U@$IP 2>&1 | head -30; echo "sftp(key) rc=${PIPESTATUS[1]}" ;;
sshk) timeout 25 ssh -i /root/fa117key $K -T $U@$IP 'id; echo SHELL-OK' 2>&1 | head -6; echo "ssh(key) rc=${PIPESTATUS[0]}"; timeout 25 ssh -i /root/fa117key $K -tt $U@$IP 2>&1 </dev/null | head -4 ;;
sshk2) timeout 25 ssh -i /root/fa117key $K -T $U@$IP 'id; echo "pwd=$(pwd)"; echo "--ls /"; ls -la /; echo "--ls /etc"; ls /etc 2>&1 | head -5; cat /etc/hostname 2>&1; echo SHELL-OK' 2>&1 | head -40; echo "ssh(key) rc=${PIPESTATUS[0]}" ;;
sshk3) timeout 40 ssh -i /root/fa117key $K -T $U@$IP 'echo "## id"; id; echo "## pwd"; pwd; echo "## ls -la /"; ls -la /; echo "ls-root rc=$?"; echo "## cat /etc/passwd"; cat /etc/passwd; echo "passwd rc=$?"; echo "## ls /var/www"; ls /var/www; echo "rc=$?"; echo "## ls /home"; ls /home; echo "rc=$?"; echo "## ps aux"; ps aux; echo "ps rc=$?"; echo "## ls /proc"; ls /proc 2>&1 | head -5; echo "## sudo"; sudo -n true; echo "sudo rc=$?"; echo "## su"; su -c true </dev/null; echo "su rc=$?"; echo "## links"; ls -ld /bin /sbin /lib /lib64 2>&1; echo "## hostname file"; cat /etc/hostname 2>&1; echo SHELL-OK' 2>&1 | head -90; echo "ssh(key) rc=${PIPESTATUS[0]}"
      echo "## interactive (-tt) login banner/prompt"; timeout 15 ssh -i /root/fa117key $K -tt $U@$IP 2>&1 </dev/null | head -4 ;;
sftpk3) printf 'pwd
-ls -la
-ls -la /
-ls /etc
-ls /etc/ssh
-ls /var/www
-ls /home
-get /etc/shadow /tmp/r3-shadow
-cd /etc
-pwd
' | timeout 40 sftp -i /root/fa117key $K -b - $U@$IP 2>&1 | head -60; echo "sftp(key) rc=${PIPESTATUS[1]}"; rm -f /tmp/r3-shadow ;;
sshk4) timeout 30 ssh -i /root/fa117key $K -T $U@$IP 'echo "HOME=$HOME"; echo "start pwd=$(pwd)"; cd; echo "cd-home rc=$?"; cd /2 && echo "cd /2 ok pwd=$(pwd)" && ls -la; echo "## id -Gn"; id -Gn 2>&1' 2>&1 | head -20; echo "ssh(key) rc=${PIPESTATUS[0]}" ;;
keygen) [ -f /root/fa117key ] || ssh-keygen -q -t ed25519 -N '' -C fa117-lab -f /root/fa117key; cat /root/fa117key.pub ;;
ssh)  timeout 25 sshpass -e ssh $O -T $U@$IP 'id; cat /etc/hostname; echo SHELL-OK' 2>&1 | head -8; echo "ssh rc=${PIPESTATUS[0]}" ;;
ftps) timeout 40 lftp -c "set ftp:ssl-force true; set ftp:ssl-protect-data true; set ssl:verify-certificate no; set net:max-retries 1; set net:timeout 10; open -u $U,$PW ftp://$IP; pwd; ls; cd /etc; pwd; ls; bye" 2>&1 | sed "s#:[^:@/]*@#:[REDACTED]@#g" | head -20; echo "ftps rc=$?"
      echo Q | timeout 15 openssl s_client -connect $IP:21 -starttls ftp -CAfile /root/beta1/pebble-root.pem ${HN:+-verify_hostname $HN -servername $HN} 2>/dev/null > /tmp/ftps.out; openssl x509 -in /tmp/ftps.out -noout -subject -issuer 2>/dev/null | tr '\n' ' '; grep -E 'Verify return code' /tmp/ftps.out | head -1 ;;
ftpplain) timeout 30 lftp -c "set ftp:ssl-allow false; set net:max-retries 1; set net:timeout 10; open -u $U,$PW ftp://$IP; ls; bye" 2>&1 | head -6; echo "plain ftp rc=$?"
      (printf 'USER %s\r\n' "$U"; sleep 2; printf 'QUIT\r\n') | timeout 10 nc $IP 21 2>&1 | head -4 ;;
imap) python3 - "$IP" "$U" "$PW" <<'P'
import imaplib, ssl, sys
ip, u, pw = sys.argv[1:4]; ctx = ssl.create_default_context(); ctx.check_hostname = False; ctx.verify_mode = ssl.CERT_NONE
try:
    m = imaplib.IMAP4_SSL(ip, 993, ssl_context=ctx, timeout=20); r = m.login(u, pw); print("imap login OK", r[0]); m.logout()
except Exception as e: print("imap login FAILED:", type(e).__name__, str(e)[:200])
P
;;
smtp) python3 - "$IP" "$U" <<'P'
import smtplib, sys
ip, rcpt = sys.argv[1:3]
s = smtplib.SMTP(ip, 25, timeout=30); s.ehlo("labnet.beta1-lab.test"); print("MAIL", s.mail("probe@beta1-lab.test")); print("RCPT", rcpt, s.rcpt(rcpt)); s.quit()
P
;;
site) c=$(timeout 20 curl -s -o /tmp/site.out -w '%{http_code}' -H "Host: $U" http://$IP/); echo "site http $c body: $(head -c 300 /tmp/site.out | tr '\n' ' ' | sed 's/<[^>]*>/ /g' | tr -s ' ')" ;;
sftpput) printf 'put /etc/hostname sftp-probe.txt\nls -l sftp-probe.txt\nrm sftp-probe.txt\n' | timeout 40 sshpass -e sftp $O -oBatchMode=no -b - $U@$IP 2>&1 | head -8; echo "sftp put rc=${PIPESTATUS[1]}" ;;
phpup) c=$(timeout 20 curl -s -o /tmp/phpup.out -w '%{http_code}' -F f=@/etc/hostname -H "Host: $U" http://$IP/up.php); echo "php upload http $c: $(head -c 200 /tmp/phpup.out)" ;;
fetch) for i in 1 2 3; do timeout 60 curl -s -o /dev/null -w "fetch $HN http %{http_code} bytes %{size_download}\n" -H "Host: $U" http://$IP/$HN; done ;;
get) c=$(timeout 30 curl -sL -o /tmp/get.out -w '%{http_code} %{url_effective}' -H "Host: $U" "http://$IP/$HN"); echo "get $U/$HN http $c title: $(grep -oiE '<title>[^<]*' /tmp/get.out | head -1 | cut -c8-120) | $(head -c 200 /tmp/get.out | tr '\n' ' ' | sed 's/<[^>]*>/ /g' | tr -s ' ' | cut -c1-160)" ;;
https) c=$(timeout 30 curl -s --cacert /root/beta1/pebble-root.pem --resolve $U:443:$IP -o /tmp/https.out -w '%{http_code}' "https://$U/$HN"); echo "https $U/$HN http $c verify=$? title: $(grep -oiE '<title>[^<]*' /tmp/https.out | head -1 | cut -c8-120) bytes=$(wc -c < /tmp/https.out)"; grep -c -i executeCommand /tmp/https.out | sed 's/^/executeCommand mentions: /' ;;
smtpsize) python3 - "$IP" "$U" "$HN" <<'PY'
import smtplib, sys, os
ip, rcpt, size = sys.argv[1], sys.argv[2], int(sys.argv[3])
body = ("Subject: lab quota probe %d\r\nFrom: probe@beta1-lab.test\r\nTo: %s\r\n\r\n" % (size, rcpt)) + "\r\n".join(["x" * 998] * (size // 1000))
s = smtplib.SMTP(ip, 25, timeout=60); s.ehlo("labnet.beta1-lab.test")
print("MAIL", s.mail("probe@beta1-lab.test", ["SIZE=%d" % len(body)]))
r = s.rcpt(rcpt); print("RCPT", rcpt, r)
if r[0] == 250:
    s.data(body); print("DATA ok", len(body))
s.quit()
PY
;;
wpfinish) c=$(timeout 120 curl -s -o /tmp/wpf.out -w '%{http_code}' -H "Host: $U" --data-urlencode "weblog_title=Lab WP" --data-urlencode "user_name=labadmin" --data-urlencode "admin_password=$PW" --data-urlencode "admin_password2=$PW" --data-urlencode "pw_weak=1" --data-urlencode "admin_email=labadmin@$U" --data-urlencode "blog_public=0" --data-urlencode "Submit=Install WordPress" --data-urlencode "language=" "http://$IP/wp-admin/install.php?step=2"); echo "wp finish http $c: $(grep -oiE '<h1>[^<]*' /tmp/wpf.out | head -2 | tr '
' ' ')" ;;
code) c=$(timeout 30 curl -s -o /tmp/code.out -w '%{http_code} -> %{redirect_url}' -H "Host: $U" "http://$IP/$HN"); echo "code $U/$HN http $c title: $(grep -oiE '<title>[^<]*' /tmp/code.out | head -1 | cut -c8-100)" ;;
n8nlogin) python3 - "$IP" "$U" "$PW" <<'PY'
import json, ssl, sys, http.client
ip, host, pw = sys.argv[1:4]
ctx = ssl.create_default_context(cafile="/root/beta1/pebble-root.pem")
class C(http.client.HTTPSConnection):
    def connect(self):
        import socket
        self.sock = ctx.wrap_socket(socket.create_connection((ip, 443), 20), server_hostname=host)
c = C(host, 443, timeout=30)
c.request("POST", "/rest/login", json.dumps({"emailOrLdapLoginId": "owner@" + host, "password": pw}), {"Content-Type": "application/json", "Host": host})
r = c.getresponse(); b = r.read().decode(errors="replace")
try: j = json.loads(b); d = j.get("data") or {}; print("n8n login http", r.status, "user:", d.get("email"), d.get("role"), "cookie:", "n8n-auth" in (r.getheader("set-cookie") or ""))
except Exception: print("n8n login http", r.status, b[:300])
PY
;;
esac
