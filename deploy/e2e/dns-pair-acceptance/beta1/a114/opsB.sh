#!/bin/bash
# B (Arch standalone, alpha.114) checks: run-6 site/mail ops, main-domain "No" path + Host a website in
# the browser without reload (checks 2, 9), OS updates guide (5), panel port change confirm / reboot /
# cancel / timeout (4), nginx -t (7), screens. Lab only.
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab/env.sh
CELL=a4-b; IP=192.0.3.152; Z=archa114.test
SH="python3 -I $H/shots.py"
OW="python3 -I $H/owner.py $CELL main"
exec > >(tee -a $LOGS/opsB.log) 2>&1
lab ssh beta1-lab labnet -- "sudo -n tee /root/mailx.py >/dev/null" < $H/mailx.py
echo "== run-6 ops on Arch $(ts)"
CELL=$CELL MIP=$IP BIP=$IP Z=$Z S=shopb114.test PANEL=arch timeout 2400 bash $H/ops4.sh 2>&1 | grep -vE '^\{|^ +"' | cut -c1-400
echo "== check 2 (No path) + check 9: Host a website from the Domains row, no reload $(ts)"
$SH $CELL main b-host '[{"name":"setup-ready-no-site","path":"/setup","wait":2500},{"name":"dashboard","path":"/","wait":3000}]'
$SH $CELL main b-host '[{"name":"host-website","path":"/domains","wait":2500,"actions":[{"shot":"before"},{"click":"[data-host-website=\"archa114.test\"]"},{"wait":2000},{"shot":"dialog"},{"click":"form button[type=submit]"},{"waitGone":["No website for this name yet",180000]},{"wait":3000},{"shot":"after"}]}]' --langs en --widths desktop
echo "-- the main domain now opens as a website"
lab ssh beta1-lab labnet -- "sudo -n tee /root/q-$CELL.sh >/dev/null" < $H/webcheck.sh
lab ssh beta1-lab labnet -- "sudo -n bash /root/q-$CELL.sh $Z" | grep -E '^resolves|^---|code=|issuer|Verify'
echo "== check 5: OS updates on Arch (guide only) $(ts)"
$OW osupdates
$SH $CELL main b-sec '[{"name":"security","path":"/security","wait":3000},{"name":"settings-panel-port","path":"/settings?section=panel","wait":2500}]'
echo "== check 7: nginx -t $(ts)"
b1 sh $CELL main "nginx -t 2>&1 | tail -2" --label nginx-t | python3 -c 'import json,sys; print(json.load(sys.stdin)["stdout"])'
echo "== check 4: panel port change (standalone) $(ts)"
$OW port_change 2087
echo "-- reboot: the new port survives $(ts)"
lab reboot $CELL main 900 | tr -d '\n '; echo
sleep 20
for p in 1923 2087; do timeout 5 bash -c "echo > /dev/tcp/$IP/$p" 2>/dev/null && echo "$IP:$p open" || echo "$IP:$p closed"; done
b1 login $CELL main | python3 -c 'import json,sys; d=json.load(sys.stdin); print("login after reboot, license", d["license_after"].get("state"))'
$SH $CELL main b-port '[{"name":"settings-panel-port-after","path":"/settings?section=panel","wait":2500}]' --langs en,tr --widths desktop
echo "-- cancel a trial $(ts)"
$OW port_trial 2088 cancel
echo "-- let a trial time out (10 minutes) $(ts)"
(sleep 25; $SH $CELL main b-port-trial '[{"name":"settings-panel-port-trial","path":"/settings?section=panel","wait":2500}]' --langs en,tr --widths desktop > /dev/null) &
$OW port_trial 2089 timeout
wait
echo "== B ops done $(ts)"
