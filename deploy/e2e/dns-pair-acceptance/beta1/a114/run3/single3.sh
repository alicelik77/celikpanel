#!/bin/bash
# run 3: one Arch server through get.sh from the lab portal of VER, the one-server wizard (mail) through the
# panel API; optional names-step screens (S-new-4) before the start. Lab only.
# env: CELL IP Z VER MEM [TRUST=1] [SHOTS=1] [MAIN_SITE=here|none]
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab3/env.sh
SHORT=arch; OS=arch
SH="python3 -I $H/shots.py"
exec > >(tee -a $LOGS/single-$CELL.log) 2>&1
echo "== provision $(ts) $OS $IP zone=$Z version=$VER"
b1 provision $CELL main $OS $IP $SHORT $MEM > $LOGS/prov-$CELL-main.log 2>&1
tail -c 300 $LOGS/prov-$CELL-main.log; echo
[ "${TRUST:-0}" = 1 ] && { echo "-- lab portal trust (update topology)"; bash $H/portalnet.sh trust $CELL main; }
echo "== install through get.sh in a root terminal $(ts)"
A114_PORTAL=/var/tmp/a114r3/portal-$VER python3 -I $H/getsh_install.py $L $CELL main > $LOGS/install-$CELL-main.log 2>&1
python3 $H/insum.py $LOGS/install-$CELL-main.log main
[ "${TRUST:-0}" = 1 ] && { echo "-- lab portal name again after the install restart"; bash $H/portalnet.sh trust $CELL main; }
b1 login $CELL main | python3 -c 'import json,sys; d=json.load(sys.stdin); print("license", d["license_after"].get("state"), d["license_after"].get("license_kind"), "can_use_panel", d["access"].get("can_use_panel"))'
b1 sh $CELL main "cat /opt/celikpanel/release.commit; grep CELIKPANEL_LISTEN /etc/celikpanel/panel.env; pacman -Q dovecot pigeonhole 2>&1" --label pkgs | python3 -c 'import json,sys; print(json.load(sys.stdin)["stdout"])'
W=/var/tmp/a114r3/wiz-$CELL; mkdir -p $W
python3 $H/wizdata.py $W single $Z $IP $SHORT ${MAIN_SITE:-here}
infra delegate $Z ns1.$Z $IP ns2.$Z $IP > /dev/null
b1 api $CELL main PUT /api/v1/setup/guidance '{"revision":0,"guidance":"guided"}' --label guidance --limit 50 | st guidance
for s in domain count name roles names dnshome; do b1 save $CELL main @$W/$s.json --label $s --limit 300 | st $s 0; done
b1 records $CELL main > /dev/null
b1 save $CELL main @$W/mail.json --label mail --limit 300 | st mail 0
if [ "${SHOTS:-0}" = 1 ]; then
  echo "-- S-new-4: the one-server names step (EN/TR, desktop and 390 px) $(ts)"
  b1 api $CELL main GET /api/v1/setup --label snap --limit 400000 | python3 $H/wizitems.py $W/items.json names
  python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); d[0]["actions"]=[{"probe":"(document.querySelector(\"main\")||document.body).innerText.match(/[^.\n]*(will join|katılacak|katilacak)[^.\n]*/gi)","label":"mentions of the joining server"},{"probe":"(document.querySelector(\"main\")||document.body).innerText.match(/[^\n]*mail\.[a-z0-9.-]+[^\n]*/gi)","label":"mail name help"}]; json.dump(d,open(sys.argv[1],"w"))' $W/items.json
  $SH $CELL main snew4-$CELL $W/items.json
fi
b1 plan $CELL main --limit 60000 > $LOGS/plan-$CELL-main.json; python3 $H/plansum2.py $LOGS/plan-$CELL-main.json plan
echo "== start $(ts)"
b1 start $CELL main --limit 300 | st start 160
TRIES=120 wait_status $CELL main succeeded failed
echo "== end $(ts)"
opnow $CELL main | head -3
b1 sh $CELL main "pacman -Q dovecot pigeonhole 2>&1; dovecot --version; systemctl is-active dovecot postfix" --label pkgs-after | python3 -c 'import json,sys; print(json.load(sys.stdin)["stdout"])'
echo "== single done $(ts)"
