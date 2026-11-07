# guest (root): lab load - the site owner uploads a small Express app into the site's document root and runs
# npm install with the panel's Node.js runtime, as the owner would over SFTP/SSH. $1 = domain id, $2 = node version, $3 = label. Lab only.
DID=$1; V=$2; LBL=${3:-a127}
D=$(ls -d /var/www/celikpanel/subscriptions/*/sites/$DID/public_html 2>/dev/null | head -1)
[ -d "$D" ] || { echo "no docroot for $DID"; ls /var/www/celikpanel/subscriptions/*/sites/ 2>&1 | head; exit 1; }
U=$(stat -c %U "$D"); echo "docroot $D owner $U"
NB=/opt/celikpanel/runtimes/node/$V/bin
ls $NB 2>&1 | tr '\n' ' '; echo
cat > $D/package.json <<J
{"name":"lab-express","version":"1.0.0","private":true,"main":"server.js","scripts":{"start":"node server.js","build":"node -e \"require('fs').writeFileSync('build-done.txt', new Date().toISOString())\""},"dependencies":{"express":"4.21.2"}}
J
cat > $D/server.js <<J
const express = require('express');
const app = express();
app.get('/', (req, res) => res.send('lab express ok $LBL pid=' + process.pid + ' uid=' + process.getuid() + ' node=' + process.version + '\n'));
app.get('/whoami', (req, res) => res.json({ uid: process.getuid(), cwd: process.cwd(), port: process.env.PORT, host: process.env.HOST }));
const port = process.env.PORT || 3000;
app.listen(port, process.env.HOST || '127.0.0.1', () => console.log('listening', port));
J
chown $U: $D/package.json $D/server.js
cd $D && runuser -u $U -- env HOME=/tmp/npmhome-$DID PATH=$NB:/usr/bin:/bin npm install --no-audit --no-fund 2>&1 | tail -3
ls $D/node_modules | head -3; ls $D/node_modules | wc -l
