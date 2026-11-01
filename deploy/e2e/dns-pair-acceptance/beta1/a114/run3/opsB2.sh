#!/bin/bash
# B (run 2, Arch standalone): run-6 ops, Dashboard no-website card -> Add domain with SSL ticked (no reload),
# E5 redirects, autoresponder on Arch, OS updates guide, nginx -t, panel address screens, port change
# confirm + reboot, trial rolled back by a restart, cancel. Lab only.
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab3/env.sh
CELL=a4-b; IP=192.0.3.152; Z=archa114.test; S=shopb114.test
SH="python3 -I $H/shots.py"
OW="python3 -I $H/owner.py $CELL main"
exec > >(tee -a $LOGS/opsB.log) 2>&1
lab ssh beta1-lab labnet -- "sudo -n tee /root/mailx.py >/dev/null" < $H/mailx.py
echo "== run-6 ops on Arch $(ts)"
CELL=$CELL MIP=$IP BIP=$IP Z=$Z S=$S PANEL=arch timeout 2400 bash $H/ops4.sh 2>&1 | grep -vE '^\{|^ +"' | cut -c1-400
echo "== check 2 No path: Dashboard card 'no website' -> Add domain with SSL ticked, the row converts without reload $(ts)"
$SH $CELL main b-host '[{"name":"dashboard-card","path":"/","wait":3500,"actions":[{"shot":"card"},{"probe":"(document.querySelector(\"[data-dashboard-no-website]\")||{}).innerText","label":"card text"},{"click":"[data-dashboard-no-website=\"archa114.test\"] button"},{"wait":3000},{"shot":"dialog"},{"probe":"(()=>{const d=document.querySelector(\"[role=dialog]\"); if(!d) return \"no dialog\"; const dom=d.querySelector(\"input[placeholder=\\\"example.com\\\"]\"); return {domain: dom&&dom.value, checks:[...d.querySelectorAll(\"input[type=checkbox]\")].map(i=>((i.closest(\"label\")||{}).innerText||i.id||\"\").replace(/\\s+/g,\" \").slice(0,60)+\"=\"+i.checked)}})()","label":"dialog fields"},{"click":"[role=dialog] form button[type=submit]"},{"waitGone":["No website for this name yet",240000]},{"wait":5000},{"shot":"after"},{"clickIf":"nav a[href=\"/\"]"},{"wait":4000},{"shot":"dashboard-after"},{"probe":"!!document.querySelector(\"[data-dashboard-no-website]\")","label":"card still shown after SPA navigation"}]}]' --langs en --widths desktop
echo "-- the main domain now opens as a website (https, apex and www)"
lab ssh beta1-lab labnet -- "sudo -n tee /root/q-$CELL.sh >/dev/null" < $H/webcheck.sh
lab ssh beta1-lab labnet -- "sudo -n bash /root/q-$CELL.sh $Z" | grep -E '^resolves|^---|code=|issuer|Verify'
echo "== E5: folder redirect with a query $(ts)"
$OW e5 $S
echo "== autoresponder on Arch $(ts)"
$OW autoresp_e4 $S mail.$Z basic
b1 sh $CELL main "ls -la /var/mail/vhosts/$S/bob/ | grep -i sieve; doveconf -n >/dev/null && echo doveconf-clean; dovecot --version; journalctl -u dovecot -b --no-pager | grep -c 'Not a directory'" --label arch-sieve | python3 -c 'import json,sys; print(json.load(sys.stdin)["stdout"])'
echo "== check 5: OS updates on Arch (guide only) $(ts)"
$OW osupdates
echo "== check 7: nginx -t $(ts)"
b1 sh $CELL main "nginx -t 2>&1 | tail -2" --label nginx-t | python3 -c 'import json,sys; print(json.load(sys.stdin)["stdout"])'
$SH $CELL main b-sec '[{"name":"security","path":"/security","wait":3000},{"name":"settings-panel-port","path":"/settings?section=panel","wait":2500,"actions":[{"probe":"(document.querySelector(\"main\")||document.body).innerText.match(/https:\\/\\/[^\\s]+/g)","label":"addresses shown"}]}]'
echo "== check 4: panel address and port trial screens (S16, S17) $(ts)"
$OW port_trial_shots 2089
echo "== check 4: port change confirm path, survives a reboot $(ts)"
$OW port_change 2087
lab reboot $CELL main 900 | tr -d '\n '; echo
sleep 20
for p in 1923 2087; do timeout 5 bash -c "echo > /dev/tcp/$IP/$p" 2>/dev/null && echo "$IP:$p open" || echo "$IP:$p closed"; done
b1 login $CELL main | python3 -c 'import json,sys; d=json.load(sys.stdin); print("login after reboot, license", d["license_after"].get("state"))'
$SH $CELL main b-port '[{"name":"settings-panel-port-after","path":"/settings?section=panel","wait":2500}]' --langs en,tr --widths desktop
echo "-- a restart during a trial rolls it back $(ts)"
$OW port_trial_restart 2088
echo "-- cancel a trial $(ts)"
$OW port_trial 2090 cancel
echo "== B ops done $(ts)"
