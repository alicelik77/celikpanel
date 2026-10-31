#!/bin/bash
# B. Standalone server (Arch, PowerDNS, web + mail) through get.sh (alpha.114 portal), wizard through
# the panel API. MAIN_SITE=elsewhere takes the "No" path of the main-domain website question. Lab only.
# env: CELL IP Z OS SHORT MEM MAIN_SITE SHOTS
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab/env.sh
SH="python3 -I $H/shots.py"
P=$CELL
exec > >(tee -a $LOGS/single-$CELL.log) 2>&1
echo "== provision $(ts) $OS $IP zone=$Z main_site=$MAIN_SITE"
b1 provision $CELL main $OS $IP $SHORT $MEM > $LOGS/prov-$CELL-main.log 2>&1
tail -c 300 $LOGS/prov-$CELL-main.log; echo
echo "== install through get.sh in a root terminal $(ts)"
A114_PORTAL=/var/tmp/a114r1/portal-v0.1.0-alpha.114 python3 -I $H/getsh_install.py $L $CELL main > $LOGS/install-$CELL-main.log 2>&1
python3 $H/insum.py $LOGS/install-$CELL-main.log main
for p in 1923 2083; do timeout 5 bash -c "echo > /dev/tcp/$IP/$p" 2>/dev/null && echo "$IP:$p open" || echo "$IP:$p closed"; done
b1 sh $CELL main "grep -E '^CELIKPANEL_LISTEN' /etc/celikpanel/panel.env; ss -ltn | grep -E ':(1923|2083) ' | sed 's/  */ /g'; nft list ruleset 2>/dev/null | grep -E '1923|2083' | sed 's/^ *//' | head -5" --label port | python3 -c 'import json,sys; print(json.load(sys.stdin)["stdout"].replace(chr(10)," | "))'
b1 login $CELL main | python3 -c 'import json,sys; d=json.load(sys.stdin); print("license", d["license_after"].get("state"), d["license_after"].get("license_kind"), "can_use_panel", d["access"].get("can_use_panel"))'
W=/var/tmp/a114r1/wiz-$CELL; mkdir -p $W
python3 $H/wizdata.py $W single $Z $IP $SHORT $MAIN_SITE
infra delegate $Z ns1.$Z $IP ns2.$Z $IP > /dev/null
b1 api $CELL main PUT /api/v1/setup/guidance '{"revision":0,"guidance":"guided"}' --label guidance --limit 50 | st guidance
for s in domain count name roles names dnshome; do b1 save $CELL main @$W/$s.json --label $s --limit 300 | st $s 0; done
b1 records $CELL main > /dev/null
b1 save $CELL main @$W/mail.json --label mail --limit 300 | st mail 0
if [ "${SHOTS:-0}" = 1 ]; then
  b1 api $CELL main GET /api/v1/setup --label snap --limit 400000 | python3 $H/wizitems.py $W/items.json roles names review
  $SH $CELL main $P $W/items.json --widths desktop
fi
b1 plan $CELL main --limit 60000 > $LOGS/plan-$CELL-main.json; python3 $H/plansum2.py $LOGS/plan-$CELL-main.json plan
echo "== start $(ts)"
b1 start $CELL main --limit 300 | st start 160
TRIES=120 wait_status $CELL main succeeded failed
echo "== end $(ts)"
opnow $CELL main | head -3
python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); [print("main_site step:", s.get("status"), json.dumps(s.get("main_site"))) for s in b.get("steps",[]) if s.get("id")=="main_site" or s.get("kind")=="main_site"]' < <(b1 api $CELL main GET /api/v1/setup/operation --label op-final --limit 400000)
b1 api $CELL main GET /api/v1/dns/overview --label dns-overview --limit 400000 2>/dev/null | python3 $H/dnswatch.py | cut -c1-500
if [ "${SHOTS:-0}" = 1 ]; then
  $SH $CELL main $P-ready '[{"name":"setup-ready","path":"/setup","wait":2500},{"name":"dashboard","path":"/","wait":3000},{"name":"domains","path":"/domains","wait":2500},{"name":"dns-page","path":"/dns-infrastructure","wait":3000},{"name":"security","path":"/security","wait":3000}]'
fi
echo "== single done $(ts)"
