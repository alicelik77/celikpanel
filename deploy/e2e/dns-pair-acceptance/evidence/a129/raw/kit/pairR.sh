set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/env.sh
source $H/common.sh
SH="python3 -I $H/shots.py"
P=${CELL}
MNAME=${MNAME:-frankfurt}; BNAME=${BNAME:-boston}
exec > >(tee -a $LOGS/pairR-$CELL.log) 2>&1
W=$B/wiz-$CELL
echo "== start main $(ts)"
for i in $(seq 1 20); do b1 start $CELL main --limit 3000 > $LOGS/start-$CELL-main.json; st start-main 600 < $LOGS/start-$CELL-main.json; grep -q server_setup_distribution_updates_unread $LOGS/start-$CELL-main.json || break; echo "   distribution updates not read yet: wait, press Start again"; sleep 30; done
[ "${SHOTS:-0}" = 1 ] && $SH $CELL main $P-main-progress '[{"name":"setup-progress","path":"/setup","wait":4000}]'
CODEF=/var/tmp/cp-beta1/state/$CELL/main.last-join-code
for i in $(seq 1 60); do
  b1 api $CELL main POST /api/v1/setup/join-codes '{}' --label join-code --limit 300 | st join-code 60
  [ -s $CODEF ] && break
  opnow $CELL main | head -2
  sleep 15
done
[ "${SHOTS:-0}" = 1 ] && $SH $CELL main $P-main-joincode '[{"name":"setup-progress-joincode","path":"/setup","wait":3000}]'
MPORT=$(b1 sh $CELL main "sed -n 's/^CELIKPANEL_LISTEN=.*://p' /etc/celikpanel/panel.env" | python3 -c 'import json,sys; print(json.load(sys.stdin)["stdout"].strip())')
echo "first server's panel port: $MPORT"
python3 -c 'import json,sys; print(json.dumps({"address":"https://"+sys.argv[2]+":"+sys.argv[3],"code":open(sys.argv[1]).read().strip()}))' $CODEF $MIP $MPORT > $B/join-$CELL.json
chmod 600 $B/join-$CELL.json
b1 api $CELL backup POST /api/v1/setup/peer/connect @$B/join-$CELL.json --label peer-connect --limit 3000 | st connect 300
rm -f $B/join-$CELL.json
for s in roles name names dnshome; do b1 save $CELL backup @$W/b/$s.json --label $s --joined --limit 300 | st backup-$s 0; done
b1 records $CELL backup > /dev/null
if [ -f $W/b/names2.json ]; then
  echo "== joined server: the first server's mail name must be refused (check 3a) $(ts)"
  b1 plan $CELL backup --limit 60000 > $LOGS/plan-$CELL-backup-taken.json; python3 $H/plansum2.py $LOGS/plan-$CELL-backup-taken.json "plan backup with mail.$Z"
  if [ "${SHOTS:-0}" = 1 ]; then
    b1 api $CELL backup GET /api/v1/setup --label snap --limit 400000 | python3 $H/wizitems.py $W/items-bt.json names review
    $SH $CELL backup $P-backup-taken $W/items-bt.json
  fi
  b1 save $CELL backup @$W/b/names2.json --label names-own-mail --joined --limit 300 | st backup-names2 0
fi
if [ "${SHOTS:-0}" = 1 ]; then
  b1 api $CELL backup GET /api/v1/setup --label snap --limit 400000 | python3 $H/wizitems.py $W/items-b.json domain name roles components names dnshome mail review
  $SH $CELL backup $P-backup $W/items-b.json
fi
b1 plan $CELL backup --limit 60000 > $LOGS/plan-$CELL-backup.json; python3 $H/plansum2.py $LOGS/plan-$CELL-backup.json "plan backup"
echo "== start backup $(ts)"
for i in $(seq 1 20); do b1 start $CELL backup --limit 3000 > $LOGS/start-$CELL-backup.json; st start-backup 600 < $LOGS/start-$CELL-backup.json; grep -q server_setup_distribution_updates_unread $LOGS/start-$CELL-backup.json || break; echo "   distribution updates not read yet: wait, press Start again"; sleep 30; done
echo "== first server's DNS page DURING the joined server's setup (check 8) $(ts)"
last=""
for i in $(seq 1 45); do
  ov=$(b1 api $CELL main GET /api/v1/dns/overview --label dns-overview-during --limit 400000 2>/dev/null | python3 $H/dnswatch.py 2>&1 | cut -c1-600)
  cap=$(b1 api $CELL main GET /api/v1/hosting/capabilities --label caps-during --limit 100000 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print(b.get("dns_server_state"), b.get("dns_server_peer"))' 2>&1)
  bop=$(opnow $CELL backup | head -1 | cut -c1-70)
  line="caps=$cap | $ov | backup: $bop"
  [ "$line" != "$last" ] && { echo "$(ts) $line"; last=$line; }
  if [ "${SHOTS:-0}" = 1 ] && [ $((i % 5)) -eq 1 ]; then $SH $CELL main $P-main-during-$i '[{"name":"dns-during-join","path":"/dns-infrastructure","wait":2500}]' --widths desktop --langs en > /dev/null; fi
  case "$bop" in succeeded*|failed*) break;; esac
  sleep 20
done
TRIES=120 wait_status $CELL backup succeeded failed
TRIES=90 wait_status $CELL main succeeded failed
echo "== after both finished: the DNS page settles by itself $(ts)"
for i in $(seq 1 16); do
  ov=$(b1 api $CELL main GET /api/v1/dns/overview --label dns-overview-after --limit 400000 2>/dev/null | python3 $H/dnswatch.py 2>&1 | cut -c1-500)
  cap=$(b1 api $CELL main GET /api/v1/hosting/capabilities --label caps-after --limit 100000 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print(b.get("dns_server_state"))' 2>&1)
  echo "$(ts) caps=$cap | $ov"
  [ "$cap" = ready ] && echo "$ov" | grep -q '^dns_overview_ok' && break
  sleep 30
done
echo "-- the joined server's DNS page"
b1 api $CELL backup GET /api/v1/dns/overview --label dns-overview-backup --limit 400000 2>/dev/null | python3 $H/dnswatch.py | cut -c1-500
for n in main backup; do opnow $CELL $n | head -3; done
inspection $CELL main; inspection $CELL backup
python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); [print("main_site step:", s.get("status"), json.dumps(s.get("main_site"))) for s in b.get("steps",[]) if s.get("id")=="main_site" or s.get("kind")=="main_site"]' < <(b1 api $CELL main GET /api/v1/setup/operation --label op-final --limit 400000)
if [ "${SHOTS:-0}" = 1 ]; then
  echo "== ready screens $(ts)"
  $SH $CELL main $P-main-ready '[{"name":"setup-ready","path":"/setup","wait":2500},{"name":"dashboard","path":"/","wait":3000},{"name":"dns-page","path":"/dns-infrastructure","wait":3000},{"name":"domains","path":"/domains","wait":2500},{"name":"server-setup","path":"/server-setup","wait":2500}]'
  $SH $CELL backup $P-backup-ready '[{"name":"setup-ready","path":"/setup","wait":2500},{"name":"dashboard","path":"/","wait":3000},{"name":"dns-page","path":"/dns-infrastructure","wait":3000},{"name":"domains","path":"/domains","wait":2500},{"name":"components","path":"/services","wait":3000}]'
fi
echo "== case 2: machine names resolve after setup $(ts)"
for ip in $MIP $BIP; do echo "  @$ip A ${MHOST:-host1.$Z}: $(dig1 $ip ${MHOST:-host1.$Z} A)  A ${BHOST:-host2.$Z}: $(dig1 $ip ${BHOST:-host2.$Z} A)  A $MNAME.$Z: $(dig1 $ip $MNAME.$Z A)"; done
echo "== pair done $(ts)"
