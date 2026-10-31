#!/bin/bash
# W: a fresh alpha.114 server used only to photograph the first-server wizard from its very first
# screen (guidance choice, first/join, every step, both main-site answers). Setup is never started.
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab/env.sh
CELL=a4-w; IP=192.0.3.155; Z=wiz114.test
SH="python3 -I $H/shots.py"
exec > >(tee -a $LOGS/wiz.log) 2>&1
echo "== provision + install $(ts)"
b1 provision $CELL main debian13 $IP frankfurt 2048 > $LOGS/prov-$CELL.log 2>&1
A114_PORTAL=/var/tmp/a114r1/portal-v0.1.0-alpha.114 python3 -I $H/getsh_install.py $L $CELL main > $LOGS/install-$CELL.log 2>&1
python3 $H/insum.py $LOGS/install-$CELL.log main
b1 login $CELL main | python3 -c 'import json,sys; d=json.load(sys.stdin); print("license", d["license_after"].get("state"))'
echo "== first screens (the owner's clicks) $(ts)"
$SH $CELL main w '[{"name":"setup-guidance","path":"/setup","wait":2500}]' --shot-login
$SH $CELL main w '[{"name":"setup-kind-pick","path":"/setup","wait":2500,"actions":[{"clickText":["button","Set up with the wizard|Sihirbazla kur"]},{"wait":2000}]}]' --langs en --widths desktop
$SH $CELL main w '[{"name":"setup-kind-pick","path":"/setup","wait":2500},{"name":"setup-kind-first","path":"/setup","actions":[{"click":"input[name=setup-server_kind][value=first]"}]},{"name":"setup-kind-join","path":"/setup","actions":[{"click":"input[name=setup-server_kind][value=join]"}]}]'
W=/var/tmp/a114r1/wiz-$CELL; mkdir -p $W
python3 $H/wizdata.py $W pair-main $Z $IP frankfurt here
for s in domain count name roles names dnshome; do b1 save $CELL main @$W/$s.json --label $s --limit 300 | st $s 0; done
b1 records $CELL main > /dev/null
b1 save $CELL main @$W/mail.json --label mail --limit 300 | st mail 0
b1 api $CELL main GET /api/v1/setup --label snap --limit 400000 | python3 $H/wizitems.py $W/items.json domain count name roles components names dnshome records mail review
$SH $CELL main w $W/items.json
echo "-- the main-site answer No"
b1 save $CELL main '{"main_site":"elsewhere"}' --label main-site-no --limit 300 | st main-site-no 0
b1 api $CELL main GET /api/v1/setup --label snap --limit 400000 | python3 $H/wizitems.py $W/items-no.json roles review
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); [i.update(name=i["name"]+"-site-no") for i in d]; json.dump(d,open(sys.argv[1],"w"))' $W/items-no.json
$SH $CELL main w $W/items-no.json
echo "-- review with the plan computed (the Summary as the owner reaches it)"
b1 save $CELL main '{"main_site":"here"}' --label main-site-yes --limit 300 | st main-site-yes 0
b1 api $CELL main GET /api/v1/setup --label snap --limit 400000 | python3 $H/wizitems.py $W/items-m.json mail
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); d[0]["name"]="wizard-summary-reached"; d[0]["actions"]=[{"click":"form button[type=submit]"},{"wait":6000}]; json.dump(d,open(sys.argv[1],"w"))' $W/items-m.json
$SH $CELL main w $W/items-m.json
echo "== W done $(ts)"
