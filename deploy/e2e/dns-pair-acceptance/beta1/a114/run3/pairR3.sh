#!/bin/bash
# A third install (run 2): fresh candidate pair for the remaining A items; E1/check 8 were proven before, the first server finishes its setup first; then the joined server's
# setup runs. Check 8 (first server's DNS page while the backup is being set up) and E1 forced: the first
# server's panel is stopped for 90 s while the joined server reaches its publisher step (as in run 1, where
# the first server's panel restarted). Lab only.
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab3/env.sh
CELL=a4-a; MIP=192.0.3.150; BIP=192.0.3.151; Z=fa114.test; MNAME=frankfurt; BNAME=boston
SH="python3 -I $H/shots.py"
exec > >(tee -a $LOGS/pair-$CELL.log) 2>&1
mkdir -p /var/tmp/a114r3/e1e; rm -f /var/tmp/a114r3/e1e/op-*.json
echo "== provision $(ts)"
b1 provision $CELL main debian13 $MIP $MNAME 4096 > $LOGS/prov-$CELL-main.log 2>&1 &
b1 provision $CELL backup ubuntu2404 $BIP $BNAME 3072 > $LOGS/prov-$CELL-backup.log 2>&1 &
wait
A114_PORTAL=/var/tmp/a114r3/portal-v0.1.0-alpha.114 python3 -I $H/getsh_install.py $L $CELL main > $LOGS/install-$CELL-main.log 2>&1 &
A114_PORTAL=/var/tmp/a114r3/portal-v0.1.0-alpha.114 python3 -I $H/getsh_install.py $L $CELL backup > $LOGS/install-$CELL-backup.log 2>&1 &
wait
for n in main backup; do python3 $H/insum.py $LOGS/install-$CELL-$n.log $n | head -1; b1 login $CELL $n > /dev/null; done
W=/var/tmp/a114r3/wiz-$CELL; mkdir -p $W/m $W/b
python3 $H/wizdata.py $W/m pair-main $Z $MIP $MNAME here
python3 $H/wizdata.py $W/b pair-backup-web $Z $BIP $BNAME
infra delegate $Z ns1.$Z $MIP ns2.$Z $BIP > /dev/null
for n in main backup; do b1 api $CELL $n PUT /api/v1/setup/guidance '{"revision":0,"guidance":"guided"}' --label guidance --limit 50 | st guidance-$n; done
for s in domain count name roles names dnshome; do [ -f $W/m/$s.json ] && b1 save $CELL main @$W/m/$s.json --label $s --limit 300 | st main-$s 0; done
b1 records $CELL main > /dev/null
b1 save $CELL main @$W/m/mail.json --label mail --limit 300 | st main-mail 0
b1 plan $CELL main --limit 60000 > $LOGS/plan-$CELL-main.json; python3 $H/plansum2.py $LOGS/plan-$CELL-main.json "plan main"
echo "== start main $(ts)"
b1 start $CELL main --limit 200 | st start-main 100
CODEF=/var/tmp/cp-beta1/state/$CELL/main.last-join-code
for i in $(seq 1 60); do b1 api $CELL main POST /api/v1/setup/join-codes '{}' --label join-code --limit 300 | st join-code 40; [ -s $CODEF ] && break; sleep 15; done
python3 -c 'import json,sys; print(json.dumps({"address":"https://"+sys.argv[2]+":1923","code":open(sys.argv[1]).read().strip()}))' $CODEF $MIP > /var/tmp/a114r3/join-$CELL.json; chmod 600 /var/tmp/a114r3/join-$CELL.json
b1 api $CELL backup POST /api/v1/setup/peer/connect @/var/tmp/a114r3/join-$CELL.json --label peer-connect --limit 3000 | st connect 200
rm -f /var/tmp/a114r3/join-$CELL.json
for s in roles name names dnshome; do b1 save $CELL backup @$W/b/$s.json --label $s --joined --limit 300 | st backup-$s 0; done
b1 records $CELL backup > /dev/null
b1 plan $CELL backup --limit 60000 > $LOGS/plan-$CELL-backup.json; python3 $H/plansum2.py $LOGS/plan-$CELL-backup.json "plan backup"
echo "== start backup $(ts)"
b1 start $CELL backup --limit 300 | st start-backup 100
TRIES=90 wait_status $CELL backup succeeded failed
TRIES=90 wait_status $CELL main succeeded failed
b1 api $CELL main GET /api/v1/dns/overview --label dns-overview --limit 400000 2>/dev/null | python3 $H/dnswatch.py | cut -c1-300
echo "== pair done $(ts)"
