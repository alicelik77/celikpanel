#!/bin/bash
# E (run 2 extra): fresh candidate pair, the first server finishes its setup first; then the joined server's
# setup runs. Check 8 (first server's DNS page while the backup is being set up) and E1 forced: the first
# server's panel is stopped for 90 s while the joined server reaches its publisher step (as in run 1, where
# the first server's panel restarted). Lab only.
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab3/env.sh
CELL=a4-e; MIP=192.0.3.180; BIP=192.0.3.181; Z=e114.test; MNAME=emain; BNAME=ejoin
SH="python3 -I $H/shots.py"
exec > >(tee -a $LOGS/pair-$CELL.log) 2>&1
mkdir -p /var/tmp/a114r3/e1e
echo "== provision $(ts)"
b1 provision $CELL main debian13 $MIP $MNAME 2048 > $LOGS/prov-$CELL-main.log 2>&1 &
b1 provision $CELL backup debian13 $BIP $BNAME 2048 > $LOGS/prov-$CELL-backup.log 2>&1 &
wait
A114_PORTAL=/var/tmp/a114r3/portal-v0.1.0-alpha.114 python3 -I $H/getsh_install.py $L $CELL main > $LOGS/install-$CELL-main.log 2>&1 &
A114_PORTAL=/var/tmp/a114r3/portal-v0.1.0-alpha.114 python3 -I $H/getsh_install.py $L $CELL backup > $LOGS/install-$CELL-backup.log 2>&1 &
wait
for n in main backup; do python3 $H/insum.py $LOGS/install-$CELL-$n.log $n | head -1; b1 login $CELL $n > /dev/null; done
W=/var/tmp/a114r3/wiz-$CELL; mkdir -p $W/m $W/b
python3 $H/wizdata.py $W/m pair-main-web $Z $MIP $MNAME elsewhere
python3 $H/wizdata.py $W/b pair-backup-web $Z $BIP $BNAME
infra delegate $Z ns1.$Z $MIP ns2.$Z $BIP > /dev/null
for n in main backup; do b1 api $CELL $n PUT /api/v1/setup/guidance '{"revision":0,"guidance":"guided"}' --label guidance --limit 50 | st guidance-$n; done
for s in domain count name roles names dnshome; do [ -f $W/m/$s.json ] && b1 save $CELL main @$W/m/$s.json --label $s --limit 300 | st main-$s 0; done
b1 records $CELL main > /dev/null
b1 plan $CELL main --limit 60000 > $LOGS/plan-$CELL-main.json; python3 $H/plansum2.py $LOGS/plan-$CELL-main.json "plan main"
echo "== start main $(ts)"
b1 start $CELL main --limit 200 | st start-main 100
TRIES=90 wait_status $CELL main succeeded failed
b1 api $CELL main GET /api/v1/dns/overview --label dns-overview --limit 400000 2>/dev/null | python3 $H/dnswatch.py | cut -c1-300
CODEF=/var/tmp/cp-beta1/state/$CELL/main.last-join-code
b1 api $CELL main POST /api/v1/setup/join-codes '{}' --label join-code --limit 300 | st join-code 40
python3 -c 'import json,sys; print(json.dumps({"address":"https://"+sys.argv[2]+":1923","code":open(sys.argv[1]).read().strip()}))' $CODEF $MIP > /var/tmp/a114r3/join-$CELL.json; chmod 600 /var/tmp/a114r3/join-$CELL.json
b1 api $CELL backup POST /api/v1/setup/peer/connect @/var/tmp/a114r3/join-$CELL.json --label peer-connect --limit 3000 | st connect 200
rm -f /var/tmp/a114r3/join-$CELL.json
for s in roles name names dnshome; do b1 save $CELL backup @$W/b/$s.json --label $s --joined --limit 300 | st backup-$s 0; done
b1 records $CELL backup > /dev/null
b1 plan $CELL backup --limit 60000 > $LOGS/plan-$CELL-backup.json; python3 $H/plansum2.py $LOGS/plan-$CELL-backup.json "plan backup"
echo "== start backup $(ts)"
b1 start $CELL backup --limit 300 | st start-backup 100
last=""; lastpub=""; lastcode=""; stopped=0; shots=0
for i in $(seq 1 300); do
  b1 api $CELL backup GET /api/v1/setup/operation --label op-watch --limit 400000 > /var/tmp/a114r3/e1e/op.json 2>/dev/null
  pub=$(python3 $H/e1watch.py < /var/tmp/a114r3/e1e/op.json 2>&1 | cut -c1-1500)
  cur=$(echo "$pub" | grep -o 'cur=[^ ]*' | head -1)
  if [ "$pub" != "$lastpub" ]; then echo "$(ts) E1 backup: $pub"; lastpub=$pub; cp /var/tmp/a114r3/e1e/op.json /var/tmp/a114r3/e1e/op-$i.json; fi
  if [ $stopped = 0 ] && echo "$cur" | grep -qE 'cur=09-|cur=10-dns_publisher'; then
    echo "$(ts) LAB: the first server's panel is stopped for 90 s (as in run 1, a restart while the joined server publishes)"
    b1 sh $CELL main "systemctl stop celikpanel-panel; systemctl is-active celikpanel-panel" --label lab-stop-panel --limit 200 > /dev/null
    stopped=$(date +%s)
  fi
  if [ $stopped -gt 1 ] && [ $(( $(date +%s) - stopped )) -ge 90 ]; then
    b1 sh $CELL main "systemctl start celikpanel-panel; sleep 3; systemctl is-active celikpanel-panel" --label lab-start-panel --limit 200 > /dev/null
    echo "$(ts) LAB: the first server's panel started again"; stopped=1
  fi
  if [ $shots -lt 2 ] && echo "$pub" | grep -q '"status": "\(waiting\|running\|blocked\|retrying\)"' && [ $stopped -gt 1 ] && [ $(( $(date +%s) - stopped )) -ge 30 ]; then
    $SH $CELL backup a-e-backup-publisher-wait-$shots '[{"name":"setup-progress","path":"/setup","wait":3000}]' > /dev/null; shots=$((shots+1))
  fi
  if [ $stopped -le 1 ]; then
    ov=$(b1 api $CELL main GET /api/v1/dns/overview --label dns-overview-during --limit 400000 2>/dev/null | python3 $H/dnswatch.py 2>&1 | cut -c1-500)
    [ "$ov" != "$last" ] && { echo "$(ts) check8 main: $ov"; last=$ov; }
    code=$(echo "$ov" | cut -d' ' -f1-2)
    if [ "$code" != "$lastcode" ]; then $SH $CELL main a-e-main-during-$i '[{"name":"dns-during-join","path":"/dns-infrastructure","wait":2500}]' > /dev/null; lastcode=$code; fi
  fi
  case "$pub" in succeeded*|failed*) break;; esac
  sleep 5
done
[ $stopped -gt 1 ] && b1 sh $CELL main "systemctl start celikpanel-panel" --label lab-start-panel --limit 200 > /dev/null
echo "== after the joined server finished: the first server's DNS page settles $(ts)"
for i in $(seq 1 12); do
  ov=$(b1 api $CELL main GET /api/v1/dns/overview --label dns-overview-after --limit 400000 2>/dev/null | python3 $H/dnswatch.py 2>&1 | cut -c1-400)
  echo "$(ts) $ov"; echo "$ov" | grep -q '^dns_overview_ok' && break; sleep 30
done
$SH $CELL main a-e-main-after '[{"name":"dns-page","path":"/dns-infrastructure","wait":3000}]'
$SH $CELL backup a-e-backup-after '[{"name":"setup","path":"/setup","wait":3000},{"name":"dns-page","path":"/dns-infrastructure","wait":3000}]' --widths desktop
grep -l '_refused\|new join code\|create a new' /var/tmp/a114r3/e1e/op-*.json 2>/dev/null | head -3 | sed 's/^/RAW REFUSAL WORDING IN: /'
lab stop $CELL main | tr -d '\n '; echo; lab stop $CELL backup | tr -d '\n '; echo
echo "== pair E done $(ts)"
