export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
pacman -Q chromium 2>/dev/null || timeout 900 pacman -S --noconfirm --needed chromium 2>&1 | tail -3
pacman -Q chromium; command -v chromium
mkdir -p /var/tmp/a114r4/shots-tool && cd /var/tmp/a114r4/shots-tool
[ -f package.json ] || echo '{"name":"a114shots","private":true}' > package.json
PUPPETEER_SKIP_DOWNLOAD=1 timeout 600 npm install --no-audit --no-fund puppeteer-core@23.11.1 2>&1 | tail -2
node -e "console.log(require('puppeteer-core/package.json').version)"
fc-list | wc -l
