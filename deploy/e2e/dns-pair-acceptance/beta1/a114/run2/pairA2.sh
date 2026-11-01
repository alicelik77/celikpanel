#!/bin/bash
# A/C/D pair: first server (MOS) + a server joined by code (BOS), both installed through the owner's
# get.sh in a root terminal from the lab portal of their version, then the setup wizard through the
# panel API exactly as web/src calls it. Wizard screens are photographed at each step from the saved
# draft (SHOTS=1). Lab only.
# env: CELL MIP BIP Z MOS BOS MVER BVER MKIND BKIND MMEM BMEM SHOTS MNAME BNAME
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab2/env.sh
SH="python3 -I $H/shots.py"
P=${CELL}
MNAME=${MNAME:-frankfurt}; BNAME=${BNAME:-boston}
exec > >(tee -a $LOGS/pair-$CELL.log) 2>&1
mkdir -p /var/tmp/a114r2/e1
echo "== provision $(ts) main=$MOS $MIP ($MVER) backup=$BOS $BIP ($BVER) zone=$Z"
b1 provision $CELL main $MOS $MIP $MNAME $MMEM > $LOGS/prov-$CELL-main.log 2>&1 &
b1 provision $CELL backup $BOS $BIP $BNAME $BMEM > $LOGS/prov-$CELL-backup.log 2>&1 &
wait
tail -c 200 $LOGS/prov-$CELL-main.log; echo; tail -c 200 $LOGS/prov-$CELL-backup.log; echo
echo "== install through get.sh in a root terminal $(ts)"
A114_PORTAL=/var/tmp/a114r2/portal-$MVER python3 -I $H/getsh_install.py $L $CELL main > $LOGS/install-$CELL-main.log 2>&1 &
A114_PORTAL=/var/tmp/a114r2/portal-$BVER python3 -I $H/getsh_install.py $L $CELL backup > $LOGS/install-$CELL-backup.log 2>&1 &
wait
for n in main backup; do python3 $H/insum.py $LOGS/install-$CELL-$n.log $n; done
echo "== port (check 4): fresh alpha.114 1923 only; alpha.113 2083 $(ts)"
for ip in $MIP $BIP; do for p in 1923 2083; do timeout 5 bash -c "echo > /dev/tcp/$ip/$p" 2>/dev/null && echo "$ip:$p open (from the lab bridge)" || echo "$ip:$p closed (from the lab bridge)"; done; done
for n in main backup; do b1 sh $CELL $n "grep -E '^CELIKPANEL_LISTEN' /etc/celikpanel/panel.env; ss -ltn | grep -E ':(1923|2083) ' | sed 's/  */ /g'; nft list ruleset 2>/dev/null | grep -E '1923|2083' | sed 's/^ *//' | head -5; cat /opt/celikpanel/release.commit" --label port-$n | python3 -c 'import json,sys; print(sys.argv[1], json.load(sys.stdin)["stdout"].replace(chr(10)," | "))' $n; done
for n in main backup; do b1 login $CELL $n | python3 -c 'import json,sys; d=json.load(sys.stdin); print(sys.argv[1], "license", d["license_after"].get("state"), d["license_after"].get("license_kind"), "can_use_panel", d["access"].get("can_use_panel"))' $n; done
if [ "${SHOTS:-0}" = 1 ]; then
  echo "== screens: login, first screen, first/join choice $(ts)"
  $SH $CELL main $P-main '[{"name":"setup-first-screen","path":"/setup"},{"name":"setup-kind-first","path":"/setup","actions":[{"click":"input[name=setup-server_kind][value=first]"}]}]' --shot-login
  $SH $CELL backup $P-backup '[{"name":"setup-kind-join","path":"/setup","actions":[{"click":"input[name=setup-server_kind][value=join]"}]}]'
fi
W=/var/tmp/a114r2/wiz-$CELL; mkdir -p $W/m $W/b
python3 $H/wizdata.py $W/m $MKIND $Z $MIP $MNAME ${MAIN_SITE:-here}
python3 $H/wizdata.py $W/b $BKIND $Z $BIP $BNAME
infra delegate $Z ns1.$Z $MIP ns2.$Z $BIP > /dev/null
for n in main backup; do b1 api $CELL $n PUT /api/v1/setup/guidance '{"revision":0,"guidance":"guided"}' --label guidance --limit 50 | st guidance-$n; done
for s in domain count name roles names dnshome; do [ -f $W/m/$s.json ] && b1 save $CELL main @$W/m/$s.json --label $s --limit 300 | st main-$s 0; done
b1 records $CELL main > $LOGS/records-$CELL-main.json; python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print("records", len(d["imported_records"]), d["save"]["status"])' $LOGS/records-$CELL-main.json
[ -f $W/m/mail.json ] && b1 save $CELL main @$W/m/mail.json --label mail --limit 300 | st main-mail 0
if [ "${SHOTS:-0}" = 1 ]; then
  b1 api $CELL main GET /api/v1/setup --label snap --limit 400000 | python3 $H/wizitems.py $W/items-main.json domain count name roles components names dnshome records mail review
  $SH $CELL main $P-main $W/items-main.json
fi
b1 plan $CELL main --limit 60000 > $LOGS/plan-$CELL-main.json; python3 $H/plansum2.py $LOGS/plan-$CELL-main.json "plan main"
echo "== start main $(ts)"
b1 start $CELL main --limit 200 | st start-main 160
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
python3 -c 'import json,sys; print(json.dumps({"address":"https://"+sys.argv[2]+":"+sys.argv[3],"code":open(sys.argv[1]).read().strip()}))' $CODEF $MIP $MPORT > /var/tmp/a114r2/join-$CELL.json
chmod 600 /var/tmp/a114r2/join-$CELL.json
b1 api $CELL backup POST /api/v1/setup/peer/connect @/var/tmp/a114r2/join-$CELL.json --label peer-connect --limit 3000 | st connect 300
rm -f /var/tmp/a114r2/join-$CELL.json
echo "== joined wizard with mail chosen (S9 screen: the mail step must keep the main domain's records on the first server) $(ts)"
mkdir -p $W/bm; python3 $H/wizdata.py $W/bm pair-backup-mail $Z $BIP $BNAME
b1 save $CELL backup @$W/bm/roles.json --label roles-mail --joined --limit 300 | st backup-roles-mail 0
b1 save $CELL backup @$W/b/name.json --label name --joined --limit 300 | st backup-name 0
b1 save $CELL backup @$W/b/names.json --label names --joined --limit 300 | st backup-names-first 0
b1 save $CELL backup @$W/bm/names2.json --label names-mail --joined --limit 300 | st backup-names-mail 0
b1 save $CELL backup @$W/b/dnshome.json --label dnshome --joined --limit 300 | st backup-dnshome 0
b1 records $CELL backup > /dev/null
b1 api $CELL backup GET /api/v1/setup --label snap --limit 400000 | python3 $H/wizitems.py $W/items-bmail.json names mail
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); [i.update(name=i["name"]+"-mailchosen") for i in d]; json.dump(d,open(sys.argv[1],"w"))' $W/items-bmail.json
$SH $CELL backup $P-backup $W/items-bmail.json
echo "-- back to web only (the joined server takes mail later from Components: E8)"
b1 save $CELL backup @$W/b/roles.json --label roles --joined --limit 300 | st backup-roles 0
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); d["mail_hostname"]=""; json.dump(d,open(sys.argv[2],"w"))' $W/b/names.json $W/b/names-clear.json
b1 save $CELL backup @$W/b/names-clear.json --label names --joined --limit 300 | st backup-names 0
b1 records $CELL backup > /dev/null
b1 api $CELL backup GET /api/v1/setup --label snap --limit 400000 | python3 $H/wizitems.py $W/items-b.json domain name roles components names dnshome review
$SH $CELL backup $P-backup $W/items-b.json
b1 plan $CELL backup --limit 60000 > $LOGS/plan-$CELL-backup.json; python3 $H/plansum2.py $LOGS/plan-$CELL-backup.json "plan backup"
echo "   main still in its own setup: $(opnow $CELL main | head -1 | cut -c1-80)"
echo "== start backup $(ts)"
b1 start $CELL backup --limit 300 | st start-backup 160
echo "== E1 + check 8: the joined server's publisher step and the first server's DNS page while both run $(ts)"
last=""; lastpub=""; lastcode=""; pubshot=0; lastshotpub=""
for i in $(seq 1 240); do
  b1 api $CELL backup GET /api/v1/setup/operation --label op-watch --limit 400000 > /var/tmp/a114r2/e1/op.json 2>/dev/null
  pub=$(python3 $H/e1watch.py < /var/tmp/a114r2/e1/op.json 2>&1 | cut -c1-1500)
  mop=$(opnow $CELL main | head -1 | cut -c1-60)
  if [ "$pub" != "$lastpub" ]; then echo "$(ts) E1 backup: $pub"; echo "         main: $mop"; lastpub=$pub; cp /var/tmp/a114r2/e1/op.json /var/tmp/a114r2/e1/op-$i.json; fi
  pst=$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d.get("body") or "{}"); op=b.get("operation",b); p=[s for s in op.get("steps",[]) if s.get("id")=="dns_publisher"]; print(p[0].get("status") if p else "none")' /var/tmp/a114r2/e1/op.json 2>/dev/null)
  if [ $pubshot -lt 3 ] && [ "$pub" != "$lastshotpub" ] && [ -n "$pst" ] && [ "$pst" != pending ] && [ "$pst" != succeeded ] && [ "$pst" != none ]; then
    echo "   publisher step $pst while main is: $mop -> photographing the joined server's setup page"; pubshot=$((pubshot+1)); lastshotpub=$pub
    $SH $CELL backup $P-backup-publisher-$pubshot '[{"name":"setup-progress","path":"/setup","wait":3000}]' > /dev/null
  fi
  ov=$(b1 api $CELL main GET /api/v1/dns/overview --label dns-overview-during --limit 400000 2>/dev/null | python3 $H/dnswatch.py 2>&1 | cut -c1-600)
  cap=$(b1 api $CELL main GET /api/v1/hosting/capabilities --label caps-during --limit 100000 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print(b.get("dns_server_state"), b.get("dns_server_peer"))' 2>&1)
  bop=$(echo "$pub" | cut -d' ' -f1)
  line="caps=$cap | $ov"
  [ "$line" != "$last" ] && { echo "$(ts) check8 main: $line | backup: $bop | main: $mop"; last=$line; }
  code=$(echo "$ov" | cut -d' ' -f1-2)
  if [ "$code" != "$lastcode" ]; then $SH $CELL main $P-main-during-$i '[{"name":"dns-during-join","path":"/dns-infrastructure","wait":2500}]' --widths desktop,phone > /dev/null; lastcode=$code; fi
  case "$bop" in succeeded|failed) break;; esac
  sleep 10
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
echo "== pair done $(ts)"
