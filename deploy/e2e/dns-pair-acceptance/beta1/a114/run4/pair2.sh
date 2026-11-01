#!/bin/bash
# A/C/D pair: first server (MOS) + a server joined by code (BOS), both installed through the owner's
# get.sh in a root terminal from the lab portal of their version, then the setup wizard through the
# panel API exactly as web/src calls it. Wizard screens are photographed at each step from the saved
# draft (SHOTS=1). Lab only.
# env: CELL MIP BIP Z MOS BOS MVER BVER MKIND BKIND MMEM BMEM SHOTS MNAME BNAME
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4/env.sh
SH="python3 -I $H/shots.py"
SM=${SHOTS_M:-${SHOTS:-0}}; SB=${SHOTS_B:-${SHOTS:-0}}
P=${CELL}
MNAME=${MNAME:-frankfurt}; BNAME=${BNAME:-boston}
exec > >(tee -a $LOGS/pair-$CELL.log) 2>&1
echo "== provision $(ts) main=$MOS $MIP ($MVER) backup=$BOS $BIP ($BVER) zone=$Z"
b1 provision $CELL main $MOS $MIP $MNAME $MMEM > $LOGS/prov-$CELL-main.log 2>&1 &
b1 provision $CELL backup $BOS $BIP $BNAME $BMEM > $LOGS/prov-$CELL-backup.log 2>&1 &
wait
tail -c 200 $LOGS/prov-$CELL-main.log; echo; tail -c 200 $LOGS/prov-$CELL-backup.log; echo
for n in ${TRUST_NODES:-}; do echo "-- lab portal trust on $n (update topology)"; bash $H/portalnet.sh trust $CELL $n; done
echo "== install through get.sh in a root terminal $(ts)"
A114_PORTAL=/var/tmp/a114r4/portal-$MVER python3 -I $H/getsh_install.py $L $CELL main > $LOGS/install-$CELL-main.log 2>&1 &
A114_PORTAL=/var/tmp/a114r4/portal-$BVER python3 -I $H/getsh_install.py $L $CELL backup > $LOGS/install-$CELL-backup.log 2>&1 &
wait
for n in main backup; do python3 $H/insum.py $LOGS/install-$CELL-$n.log $n; done
echo "== port (check 4): fresh alpha.114 1923 only; alpha.113 2083 $(ts)"
for ip in $MIP $BIP; do for p in 1923 2083; do timeout 5 bash -c "echo > /dev/tcp/$ip/$p" 2>/dev/null && echo "$ip:$p open (from the lab bridge)" || echo "$ip:$p closed (from the lab bridge)"; done; done
for n in main backup; do b1 sh $CELL $n "grep -E '^CELIKPANEL_LISTEN' /etc/celikpanel/panel.env; ss -ltn | grep -E ':(1923|2083) ' | sed 's/  */ /g'; nft list ruleset 2>/dev/null | grep -E '1923|2083' | sed 's/^ *//' | head -5; cat /opt/celikpanel/release.commit" --label port-$n | python3 -c 'import json,sys; print(sys.argv[1], json.load(sys.stdin)["stdout"].replace(chr(10)," | "))' $n; done
for n in main backup; do b1 login $CELL $n | python3 -c 'import json,sys; d=json.load(sys.stdin); print(sys.argv[1], "license", d["license_after"].get("state"), d["license_after"].get("license_kind"), "can_use_panel", d["access"].get("can_use_panel"))' $n; done
if [ "$SB" = 1 ]; then
  echo "== screens: guidance choice, first/join choice, join path $(ts)"
  $SH $CELL backup $P-backup '[{"name":"setup-guidance-screen","path":"/setup"},{"name":"setup-kind-pick","path":"/setup","actions":[{"clickText":["button","Set up with the wizard"]},{"wait":1500}]},{"name":"setup-kind-join","path":"/setup","actions":[{"click":"input[name=setup-server_kind][value=join]"}]}]' --shot-login --langs en
  $SH $CELL backup $P-backup '[{"name":"setup-kind-join","path":"/setup","actions":[{"click":"input[name=setup-server_kind][value=join]"}]}]' --langs tr
fi
W=/var/tmp/a114r4/wiz-$CELL; mkdir -p $W/m $W/b
python3 $H/wizdata.py $W/m $MKIND $Z $MIP $MNAME ${MAIN_SITE:-here}
python3 $H/wizdata.py $W/b $BKIND $Z $BIP $BNAME
infra delegate $Z ns1.$Z $MIP ns2.$Z $BIP > /dev/null
for n in main backup; do b1 api $CELL $n PUT /api/v1/setup/guidance '{"revision":0,"guidance":"guided"}' --label guidance --limit 50 | st guidance-$n; done
for s in domain count name roles names dnshome; do [ -f $W/m/$s.json ] && b1 save $CELL main @$W/m/$s.json --label $s --limit 300 | st main-$s 0; done
b1 records $CELL main > $LOGS/records-$CELL-main.json; python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print("records", len(d["imported_records"]), d["save"]["status"])' $LOGS/records-$CELL-main.json
[ -f $W/m/mail.json ] && b1 save $CELL main @$W/m/mail.json --label mail --limit 300 | st main-mail 0
if [ "$SM" = 1 ]; then
  b1 api $CELL main GET /api/v1/setup --label snap --limit 400000 | python3 $H/wizitems.py $W/items-main.json domain count name roles components names dnshome records mail review
  $SH $CELL main $P-main $W/items-main.json
fi
b1 plan $CELL main --limit 60000 > $LOGS/plan-$CELL-main.json; python3 $H/plansum2.py $LOGS/plan-$CELL-main.json "plan main"
echo "== start main $(ts)"
b1 start $CELL main --limit 200 | st start-main 160
[ "$SM" = 1 ] && $SH $CELL main $P-main-progress '[{"name":"setup-progress","path":"/setup","wait":4000}]'
CODEF=/var/tmp/cp-beta1/state/$CELL/main.last-join-code
for i in $(seq 1 60); do
  b1 api $CELL main POST /api/v1/setup/join-codes '{}' --label join-code --limit 300 | st join-code 60
  [ -s $CODEF ] && break
  opnow $CELL main | head -2
  sleep 15
done
[ "$SM" = 1 ] && $SH $CELL main $P-main-joincode '[{"name":"setup-progress-joincode","path":"/setup","wait":3000}]'
MPORT=$(b1 sh $CELL main "sed -n 's/^CELIKPANEL_LISTEN=.*://p' /etc/celikpanel/panel.env" | python3 -c 'import json,sys; print(json.load(sys.stdin)["stdout"].strip())')
echo "first server's panel port: $MPORT"
python3 -c 'import json,sys; print(json.dumps({"address":"https://"+sys.argv[2]+":"+sys.argv[3],"code":open(sys.argv[1]).read().strip()}))' $CODEF $MIP $MPORT > /var/tmp/a114r4/join-$CELL.json
chmod 600 /var/tmp/a114r4/join-$CELL.json
b1 api $CELL backup POST /api/v1/setup/peer/connect @/var/tmp/a114r4/join-$CELL.json --label peer-connect --limit 3000 | st connect 300
rm -f /var/tmp/a114r4/join-$CELL.json
for s in roles name names dnshome; do b1 save $CELL backup @$W/b/$s.json --label $s --joined --limit 300 | st backup-$s 0; done
b1 records $CELL backup > /dev/null
if [ -f $W/b/names2.json ]; then
  echo "== joined server: the first server's mail name must be refused (check 3a) $(ts)"
  b1 plan $CELL backup --limit 60000 > $LOGS/plan-$CELL-backup-taken.json; python3 $H/plansum2.py $LOGS/plan-$CELL-backup-taken.json "plan backup with mail.$Z"
  if [ "$SB" = 1 ]; then
    b1 api $CELL backup GET /api/v1/setup --label snap --limit 400000 | python3 $H/wizitems.py $W/items-bt.json names review
    $SH $CELL backup $P-backup-taken $W/items-bt.json
  fi
  b1 save $CELL backup @$W/b/names2.json --label names-own-mail --joined --limit 300 | st backup-names2 0
fi
if [ "$SB" = 1 ]; then
  b1 api $CELL backup GET /api/v1/setup --label snap --limit 400000 | python3 $H/wizitems.py $W/items-b.json domain name roles components names dnshome mail review
  $SH $CELL backup $P-backup $W/items-b.json
  last=dnshome; [ "$BKIND" = pair-backup-mail ] && last=mail
  b1 api $CELL backup GET /api/v1/setup --label snap --limit 400000 | python3 $H/wizitems.py $W/items-bs.json $last
  python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); d[0]["name"]="wizard-summary-reached"; d[0]["actions"]=[{"click":"form button[type=submit]"},{"wait":6000}]; json.dump(d,open(sys.argv[1],"w"))' $W/items-bs.json
  $SH $CELL backup $P-backup $W/items-bs.json
fi
b1 plan $CELL backup --limit 60000 > $LOGS/plan-$CELL-backup.json; python3 $H/plansum2.py $LOGS/plan-$CELL-backup.json "plan backup"
echo "== start backup $(ts)"
b1 start $CELL backup --limit 300 | st start-backup 160
echo "== first server's DNS page DURING the joined server's setup (check 8) $(ts)"
last=""
for i in $(seq 1 45); do
  ov=$(b1 api $CELL main GET /api/v1/dns/overview --label dns-overview-during --limit 400000 2>/dev/null | python3 $H/dnswatch.py 2>&1 | cut -c1-600)
  cap=$(b1 api $CELL main GET /api/v1/hosting/capabilities --label caps-during --limit 100000 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print(b.get("dns_server_state"), b.get("dns_server_peer"))' 2>&1)
  bop=$(opnow $CELL backup | head -1 | cut -c1-70)
  line="caps=$cap | $ov | backup: $bop"
  [ "$line" != "$last" ] && { echo "$(ts) $line"; last=$line; }
  if [ "$SM" = 1 ] && [ $((i % 5)) -eq 1 ]; then $SH $CELL main $P-main-during-$i '[{"name":"dns-during-join","path":"/dns-infrastructure","wait":2500}]' --widths desktop --langs en > /dev/null; fi
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
if [ "$SM" = 1 ]; then
  echo "== ready screens $(ts)"
  $SH $CELL main $P-main-ready '[{"name":"setup-ready","path":"/setup","wait":2500},{"name":"dashboard","path":"/","wait":3000},{"name":"dns-page","path":"/dns-infrastructure","wait":3000},{"name":"domains","path":"/domains","wait":2500},{"name":"server-setup","path":"/server-setup","wait":2500}]'
fi
if [ "$SB" = 1 ]; then
  $SH $CELL backup $P-backup-ready '[{"name":"setup-ready","path":"/setup","wait":2500},{"name":"dashboard","path":"/","wait":3000},{"name":"dns-page","path":"/dns-infrastructure","wait":3000},{"name":"domains","path":"/domains","wait":2500},{"name":"components","path":"/services","wait":3000}]'
fi
echo "== pair done $(ts)"
