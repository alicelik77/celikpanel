#!/bin/bash
# C + D: installs on alpha.113 (the owner's current release), the mixed pair (alpha.113 first server on
# 2083 + fresh alpha.114 joined server on 1923), then the owner's Update to the candidate through the
# panel's own update API, then mail added in place on a joined server (check 3b). Lab only.
set -u
H=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab
source $H/env.sh
exec > >(tee -a $LOGS/cd2.log) 2>&1
( export CELL=a4-d MIP=192.0.3.170 BIP=192.0.3.171 Z=d114.test MOS=debian13 BOS=ubuntu2404 MVER=v0.1.0-alpha.113 BVER=v0.1.0-alpha.114 MKIND=pair-main-web BKIND=pair-backup-web MMEM=2048 BMEM=2304 SHOTS_M=0 SHOTS_B=1 MAIN_SITE=none TRUST_NODES="main" MNAME=dmain BNAME=djoin
  timeout 5400 bash $H/pair.sh > /var/tmp/a114r1/pairD.out 2>&1 )
echo "== C and D set up $(ts)"
tail -n 12 /var/tmp/a114r1/pairC.out | cut -c1-300
tail -n 12 /var/tmp/a114r1/pairD.out | cut -c1-300
echo "== D mixed pair: ports and DNS from both $(ts)"
for ip in 192.0.3.170 192.0.3.171; do for p in 1923 2083; do timeout 5 bash -c "echo > /dev/tcp/$ip/$p" 2>/dev/null && echo "$ip:$p open" || echo "$ip:$p closed"; done; done
source $H/common.sh
for ip in 192.0.3.170 192.0.3.171; do echo "   @$ip SOA d114.test: $(rcode $ip d114.test) | A dmain.d114.test=$(dig1 $ip dmain.d114.test A) djoin=$(dig1 $ip djoin.d114.test A)"; done
echo "== the owner's Update, latest = alpha.114 $(ts)"
bash $H/portalnet.sh latest v0.1.0-alpha.114
python3 -I $H/shots.py a4-c main c-before-update '[{"name":"dashboard","path":"/","wait":4000},{"name":"settings","path":"/settings","wait":3000}]' --langs en --widths desktop
python3 -I $H/owner.py a4-d main update
python3 -I $H/owner.py a4-c main update
python3 -I $H/owner.py a4-c backup update
for c in a4-c:main a4-c:backup a4-d:main; do b1 login ${c%%:*} ${c##*:} | python3 -c 'import json,sys; d=json.load(sys.stdin); print(sys.argv[1], "login after update, license", d["license_after"].get("state"))' $c; done
echo "== after the update: setup state, DNS overview, pair $(ts)"
for c in a4-c:main a4-c:backup a4-d:main a4-d:backup; do
  cell=${c%%:*}; n=${c##*:}
  echo "-- $c: $(b1 api $cell $n GET /api/v1/setup --label setup-after-update --limit 100000 | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print(b.get("status"), "extension", b.get("extension"))')"
  b1 api $cell $n GET /api/v1/dns/overview --label dns-after-update --limit 400000 2>/dev/null | python3 $H/dnswatch.py | cut -c1-400
done
python3 -I $H/shots.py a4-c main c-after-update '[{"name":"dashboard","path":"/","wait":4000},{"name":"dns-page","path":"/dns-infrastructure","wait":3500},{"name":"settings-panel-port","path":"/settings?section=panel","wait":3000}]' --langs en,tr --widths desktop
python3 -I $H/shots.py a4-c backup c-backup-after-update '[{"name":"dashboard","path":"/","wait":4000},{"name":"dns-page","path":"/dns-infrastructure","wait":3500},{"name":"components","path":"/services","wait":3500}]' --langs en,tr --widths desktop,phone
echo "== check 3b on the upgraded joined server (Boston's case): Components -> mail in place, by API $(ts)"
for ip in 192.0.3.160 192.0.3.161; do echo "   before @$ip c114.test MX=[$(dig1 $ip c114.test MX)] SPF=[$(dig1 $ip c114.test TXT | cut -c1-60)] DMARC=[$(dig1 $ip _dmarc.c114.test TXT | cut -c1-50)] DKIM=[$(dig1 $ip celik._domainkey.c114.test TXT | cut -c1-30)]"; done
python3 -I $H/owner.py a4-c backup mailext_taken mail.c114.test
python3 -I $H/owner.py a4-c backup mailext mail.boston.c114.test
for ip in 192.0.3.160 192.0.3.161; do echo "   after  @$ip c114.test MX=[$(dig1 $ip c114.test MX)] SPF=[$(dig1 $ip c114.test TXT | cut -c1-60)] DMARC=[$(dig1 $ip _dmarc.c114.test TXT | cut -c1-50)] DKIM=[$(dig1 $ip celik._domainkey.c114.test TXT | cut -c1-30)] mail.boston A=[$(dig1 $ip mail.boston.c114.test A)]"; done
echo "== check 3b in the browser on the fresh joined server of D (first server alpha.114 now) $(ts)"
python3 -I $H/shots.py a4-d backup d-mail '[{"name":"components-mail-dialog","path":"/services","wait":4000,"actions":[{"shot":"page"},{"eval":"(()=>{const cards=[...document.querySelectorAll(\"section,article,li,div\")].filter(e=>e.innerText&&e.innerText.includes(\"Spam-Protected Mail\")&&[...e.querySelectorAll(\"button\")].some(x=>/Install|Set up|Add/.test(x.innerText))); cards.sort((a,b)=>a.innerText.length-b.innerText.length); const b=[...cards[0].querySelectorAll(\"button\")].find(x=>/Install|Set up|Add/.test(x.innerText)); b.click();})()"},{"wait":3000},{"shot":"dialog-name"},{"clickText":["button","Show the steps"]},{"wait":6000},{"shot":"dialog-steps"},{"eval":"document.querySelector(\"[role=dialog] input[type=checkbox]\")?.click()"},{"wait":800},{"clickText":["button","Start"]},{"wait":60000},{"shot":"progress-1m"},{"wait":180000},{"shot":"progress-4m"},{"wait":240000},{"shot":"end-8m"}]}]' --langs en --widths desktop
b1 api a4-d backup GET /api/v1/setup/operation --label mail-ext-op --limit 400000 | python3 $H/opsum.py | head -2
for ip in 192.0.3.170 192.0.3.171; do echo "   @$ip mail.djoin.d114.test A=[$(dig1 $ip mail.djoin.d114.test A)] d114.test MX=[$(dig1 $ip d114.test MX)]"; done
python3 -I $H/shots.py a4-d backup d-after-mail '[{"name":"components","path":"/services","wait":4000},{"name":"dns-page","path":"/dns-infrastructure","wait":3500},{"name":"setup","path":"/setup","wait":3000}]' --langs en,tr --widths desktop
echo "== C/D done $(ts)"
