#!/bin/bash
# Check 8 (run 2): a web-only candidate first server + a candidate joined server that adds mail in its setup,
# so the first server finishes while the joined server is still being set up. The first server's DNS page is
# read and photographed throughout; the state must clear by itself. Lab only.
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab3/env.sh
CELL=a4-f; MIP=192.0.3.180; BIP=192.0.3.181; Z=f114.test; MNAME=fmain; BNAME=fjoin
SH="python3 -I $H/shots.py"
exec > >(tee -a $LOGS/pair-$CELL.log) 2>&1
ovl() { b1 api $CELL main GET /api/v1/dns/overview --label dns-overview-during --limit 400000 2>/dev/null | python3 $H/dnswatch.py 2>&1 | cut -c1-500; }
echo "== provision $(ts)"
b1 provision $CELL main debian13 $MIP $MNAME 2048 > $LOGS/prov-$CELL-main.log 2>&1 &
b1 provision $CELL backup debian13 $BIP $BNAME 2560 > $LOGS/prov-$CELL-backup.log 2>&1 &
wait
A114_PORTAL=/var/tmp/a114r3/portal-v0.1.0-alpha.114 python3 -I $H/getsh_install.py $L $CELL main > $LOGS/install-$CELL-main.log 2>&1 &
A114_PORTAL=/var/tmp/a114r3/portal-v0.1.0-alpha.114 python3 -I $H/getsh_install.py $L $CELL backup > $LOGS/install-$CELL-backup.log 2>&1 &
wait
for n in main backup; do python3 $H/insum.py $LOGS/install-$CELL-$n.log $n | head -1; b1 login $CELL $n > /dev/null; done
W=/var/tmp/a114r3/wiz-$CELL; mkdir -p $W/m $W/b
python3 $H/wizdata.py $W/m pair-main-web $Z $MIP $MNAME elsewhere
python3 $H/wizdata.py $W/b pair-backup-mail $Z $BIP $BNAME
infra delegate $Z ns1.$Z $MIP ns2.$Z $BIP > /dev/null
for n in main backup; do b1 api $CELL $n PUT /api/v1/setup/guidance '{"revision":0,"guidance":"guided"}' --label guidance --limit 50 | st guidance-$n; done
for s in domain count name roles names dnshome; do [ -f $W/m/$s.json ] && b1 save $CELL main @$W/m/$s.json --label $s --limit 300 | st main-$s 0; done
b1 records $CELL main > /dev/null
b1 plan $CELL main --limit 60000 > $LOGS/plan-$CELL-main.json; python3 $H/plansum2.py $LOGS/plan-$CELL-main.json "plan main"
echo "== start main $(ts)"
b1 start $CELL main --limit 200 | st start-main 100
CODEF=/var/tmp/cp-beta1/state/$CELL/main.last-join-code
for i in $(seq 1 60); do b1 api $CELL main POST /api/v1/setup/join-codes '{}' --label join-code --limit 300 | st join-code 40; [ -s $CODEF ] && break; sleep 15; done
python3 -c 'import json,sys; print(json.dumps({"address":"https://"+sys.argv[2]+":1923","code":open(sys.argv[1]).read().strip()}))' $CODEF $MIP > /var/tmp/a114r3/join-$CELL.json; chmod 600 /var/tmp/a114r3/join-$CELL.json
b1 api $CELL backup POST /api/v1/setup/peer/connect @/var/tmp/a114r3/join-$CELL.json --label peer-connect --limit 3000 | st connect 200
rm -f /var/tmp/a114r3/join-$CELL.json
for s in roles name names names2 dnshome; do b1 save $CELL backup @$W/b/$s.json --label $s --joined --limit 300 | st backup-$s 0; done
b1 records $CELL backup > /dev/null
b1 plan $CELL backup --limit 60000 > $LOGS/plan-$CELL-backup.json; python3 $H/plansum2.py $LOGS/plan-$CELL-backup.json "plan backup"
echo "== start backup $(ts)"
b1 start $CELL backup --limit 300 | st start-backup 100
last=""; lastcode=""; mdone=""
for i in $(seq 1 400); do
  bop=$(opnow $CELL backup 2>/dev/null | head -1 | cut -c1-60); mop=$(opnow $CELL main 2>/dev/null | head -1 | cut -c1-60)
  ov=$(ovl)
  line="$ov | main: $mop | backup: $bop"
  [ "$line" != "$last" ] && { echo "$(ts) check8 main: $line"; last=$line; }
  case "$mop" in succeeded*) [ -z "$mdone" ] && { mdone=1; echo "$(ts) the first server finished while the joined server is: $bop"; };; esac
  code=$(echo "$ov" | cut -d' ' -f1-2)
  [ "$code" != "$lastcode" ] && { $SH $CELL main a3-main-during-$i '[{"name":"dns-during-join","path":"/dns-infrastructure","wait":2500},{"name":"dashboard","path":"/","wait":2500}]' > /dev/null; lastcode=$code; }
  case "$bop" in succeeded*|failed*) break;; esac
  sleep 5
done
TRIES=60 wait_status $CELL main succeeded failed
echo "== after: the DNS page settles by itself $(ts)"
for i in $(seq 1 16); do ov=$(ovl); echo "$(ts) $ov"; echo "$ov" | grep -q '^dns_overview_ok' && break; sleep 30; done
$SH $CELL main a3-main-after '[{"name":"dns-page","path":"/dns-infrastructure","wait":3000}]'
for n in main backup; do lab stop $CELL $n | tr -d '\n '; echo; done
echo "== pair F done $(ts)"
