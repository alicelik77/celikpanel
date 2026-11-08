# labnet (root): a lab Git server for Git deployment: bare repository /srv/git/app.git served read-only over HTTPS
# (dumb protocol, git.lab.test on 192.0.2.62 with a lab-CA leaf) and over SSH (user git, git-shell). usage: setup | addkey | log. Lab only.
set -u
case $1 in
setup)
  ip addr add 192.0.2.62/24 dev lab0 2>/dev/null
  id git >/dev/null 2>&1 || useradd -m -d /srv/git -s /usr/bin/git-shell git
  rm -rf /srv/git/app.git /root/gitwork; mkdir -p /root/gitwork; cd /root/gitwork
  cat > package.json <<'J'
{"name":"lab-git-app","version":"1.0.0","private":true,"main":"server.js","scripts":{"start":"node server.js","build":"node -e \"require('fs').writeFileSync('built.txt','built '+new Date().toISOString())\""}}
J
  cat > server.js <<'J'
const http = require('http');
const fs = require('fs');
http.createServer((req, res) => {
  const built = fs.existsSync('built.txt') ? fs.readFileSync('built.txt', 'utf8') : 'not built';
  res.end(JSON.stringify({ app: 'lab-git-app', rev: 'REV', node_env: process.env.NODE_ENV || null, lab_public: process.env.LAB_PUBLIC || null, lab_secret_len: (process.env.LAB_SECRET || '').length, built }) + '\n');
}).listen(process.env.PORT || 3000, process.env.HOST || '127.0.0.1');
J
  sed -i "s/REV/https-1/" server.js
  git config --global --add safe.directory "*"; git init -q -b main . && git -c user.name=lab -c user.email=lab@lab.test add . && git -c user.name=lab -c user.email=lab@lab.test commit -qm "lab app 1"
  git clone -q --bare /root/gitwork /srv/git/app.git && (cd /srv/git/app.git && git update-server-info && git config core.sharedRepository group)
  mkdir -p /srv/git/.ssh; touch /srv/git/.ssh/authorized_keys; chown -R git: /srv/git; chmod 700 /srv/git/.ssh; chmod 600 /srv/git/.ssh/authorized_keys
  cat > /root/git-serve.py <<'P'
import http.server, ssl, functools, time
class H(http.server.SimpleHTTPRequestHandler):
    def log_message(self, f, *a):
        open("/root/git-requests.log", "a").write(time.strftime("%H:%M:%S ") + self.client_address[0] + " " + (f % a) + "\n")
srv = http.server.ThreadingHTTPServer(("192.0.2.62", 443), functools.partial(H, directory="/srv/git"))
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER); ctx.load_cert_chain("/root/git-tls.pem", "/root/git-tls-key.pem")
srv.socket = ctx.wrap_socket(srv.socket, server_side=True); srv.serve_forever()
P
  systemctl stop a128-git 2>/dev/null; systemd-run --unit a128-git -p RuntimeMaxSec=28800 python3 -I /root/git-serve.py >/dev/null 2>&1; sleep 1
  systemctl is-active a128-git; git ls-remote /srv/git/app.git | head -2
  ;;
commit)  # a second revision (for the SSH deploy)
  cd /root/gitwork && sed -i "s/rev: '[^']*'/rev: '$2'/" server.js && git -c user.name=lab -c user.email=lab@lab.test commit -qam "lab $2" && git push -q /srv/git/app.git main && (cd /srv/git/app.git && git update-server-info) && chown -R git: /srv/git && git -C /srv/git/app.git log --oneline -1
  ;;
addkey) cat >> /srv/git/.ssh/authorized_keys; chown git: /srv/git/.ssh/authorized_keys; wc -l < /srv/git/.ssh/authorized_keys ;;
log) tail -${2:-10} /root/git-requests.log 2>/dev/null; journalctl -u ssh -u sshd --since -15min -o cat --no-pager 2>/dev/null | grep -E "git|Accepted|Failed" | tail -5 ;;
esac
