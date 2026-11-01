#!/bin/bash
# B (run 2). Arch standalone (PowerDNS, web + mail) through get.sh; the first-server wizard photographed from
# its very first screen at desktop and 390 px in EN and TR (S1, S2, S14, S15), both main-site answers; setup
# runs with the "No" answer (ready page No path, Dashboard no-website card). Lab only.
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4/env.sh
CELL=a4-b; IP=192.0.3.152; Z=archa114.test; OS=arch; SHORT=arch; MEM=3072
SH="python3 -I $H/shots.py"
P=$CELL
exec > >(tee -a $LOGS/single-$CELL.log) 2>&1
echo "== provision $(ts) $OS $IP zone=$Z"
b1 provision $CELL main $OS $IP $SHORT $MEM > $LOGS/prov-$CELL-main.log 2>&1
tail -c 300 $LOGS/prov-$CELL-main.log; echo
echo "== install through get.sh in a root terminal $(ts)"
A114_PORTAL=/var/tmp/a114r4/portal-v0.1.0-alpha.114 python3 -I $H/getsh_install.py $L $CELL main > $LOGS/install-$CELL-main.log 2>&1
python3 $H/insum.py $LOGS/install-$CELL-main.log main
for p in 1923 2083; do timeout 5 bash -c "echo > /dev/tcp/$IP/$p" 2>/dev/null && echo "$IP:$p open" || echo "$IP:$p closed"; done
b1 login $CELL main | python3 -c 'import json,sys; d=json.load(sys.stdin); print("license", d["license_after"].get("state"), d["license_after"].get("license_kind"), "can_use_panel", d["access"].get("can_use_panel"))'
echo "== first screens, as the owner clicks them $(ts)"
$SH $CELL main $P-w '[{"name":"setup-guidance","path":"/setup","wait":2500}]' --shot-login
$SH $CELL main $P-w '[{"name":"setup-kind-pick","path":"/setup","wait":2500,"actions":[{"clickText":["button","Set up with the wizard|Sihirbazla kur"]},{"wait":2000}]}]' --langs en --widths desktop
$SH $CELL main $P-w '[{"name":"setup-kind-pick","path":"/setup","wait":2500},{"name":"setup-kind-first","path":"/setup","actions":[{"click":"input[name=setup-server_kind][value=first]"}]},{"name":"setup-kind-join","path":"/setup","actions":[{"click":"input[name=setup-server_kind][value=join]"}]}]'
W=/var/tmp/a114r4/wiz-$CELL; mkdir -p $W
python3 $H/wizdata.py $W single $Z $IP $SHORT here
infra delegate $Z ns1.$Z $IP ns2.$Z $IP > /dev/null
b1 api $CELL main PUT /api/v1/setup/guidance '{"revision":0,"guidance":"guided"}' --label guidance --limit 50 | st guidance
for s in domain count name roles names dnshome; do b1 save $CELL main @$W/$s.json --label $s --limit 300 | st $s 0; done
b1 records $CELL main > /dev/null
b1 save $CELL main @$W/mail.json --label mail --limit 300 | st mail 0
echo "-- every wizard step, main site Yes (the default) $(ts)"
b1 api $CELL main GET /api/v1/setup --label snap --limit 400000 | python3 $H/wizitems.py $W/items.json domain count name roles components names dnshome records mail review
$SH $CELL main $P-w $W/items.json
echo "-- the main-site answer No"
b1 save $CELL main '{"main_site":"elsewhere"}' --label main-site-no --limit 300 | st main-site-no 0
b1 api $CELL main GET /api/v1/setup --label snap --limit 400000 | python3 $H/wizitems.py $W/items-no.json roles review
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); [i.update(name=i["name"]+"-site-no") for i in d]; json.dump(d,open(sys.argv[1],"w"))' $W/items-no.json
$SH $CELL main $P-w $W/items-no.json
echo "-- the Summary as the owner reaches it"
b1 api $CELL main GET /api/v1/setup --label snap --limit 400000 | python3 $H/wizitems.py $W/items-m.json mail
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); d[0]["name"]="wizard-summary-reached"; d[0]["actions"]=[{"click":"form button[type=submit]"},{"wait":6000}]; json.dump(d,open(sys.argv[1],"w"))' $W/items-m.json
$SH $CELL main $P-w $W/items-m.json
b1 plan $CELL main --limit 60000 > $LOGS/plan-$CELL-main.json; python3 $H/plansum2.py $LOGS/plan-$CELL-main.json plan
echo "== start $(ts)"
b1 start $CELL main --limit 300 | st start 160
TRIES=120 wait_status $CELL main succeeded failed
echo "== end $(ts)"
opnow $CELL main | head -3
b1 api $CELL main GET /api/v1/dns/overview --label dns-overview --limit 400000 2>/dev/null | python3 $H/dnswatch.py | cut -c1-500
$SH $CELL main $P-ready '[{"name":"setup-ready","path":"/setup","wait":2500},{"name":"dashboard","path":"/","wait":3500},{"name":"domains","path":"/domains","wait":2500},{"name":"dns-page","path":"/dns-infrastructure","wait":3000},{"name":"security","path":"/security","wait":3000},{"name":"settings","path":"/settings?section=panel","wait":3000}]'
echo "== single done $(ts)"
