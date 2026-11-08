# lab: puppeteer-core for owner-path screenshots with the Arch chromium (lab only)
set -e
command -v chromium || pacman -S --noconfirm --needed chromium >/dev/null 2>&1; command -v chromium
mkdir -p /var/tmp/a130o/shots-tool /var/tmp/a130o/shots
cp /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/shot130.js /var/tmp/a130o/shot.js
cd /var/tmp/a130o/shots-tool && [ -d node_modules/puppeteer-core ] || npm install --no-audit --no-fund puppeteer-core@23.11.1 >/dev/null 2>&1; ls node_modules | grep -c . ; echo setup-ok
