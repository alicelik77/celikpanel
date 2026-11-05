#!/bin/bash
# alpha.123 pre-release round: fresh VM through get.sh from the lab portal of the candidate e99fa018e,
# one-server setup (web + mail, main site here) through the panel API. PDNSFIRST=1 first saves PowerDNS
# (the wizard default) to read the refusal, then BIND. HOLDDB=1 holds the panel DB write lock briefly
# during setup (lab load) and watches the setup state. Lab only.
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a123/kit/env.sh
CELL=${CELL:-x}; IP=${IP:-192.0.3.190}; Z=${Z:-x.test}; VER=${VER:-v0.1.0-alpha.123}; MEM=${MEM:-4096}; SHORT=${SHORT:-frankfurt}; OS=${OS:-debian13}
exec > >(tee -a $LOGS/single-$CELL.log) 2>&1
cat_() { python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); print("host", b.get("host"), "inventory", b.get("inventory_state")); [print("  dns_engine", json.dumps(e)) for e in b.get("dns_engines") or []]' $1; }
echo "== provision $(ts) $OS $IP zone=$Z version=$VER"
b1 provision $CELL main $OS $IP $SHORT $MEM > $LOGS/prov-$CELL-main.log 2>&1
tail -c 200 $LOGS/prov-$CELL-main.log; echo
echo "== install through get.sh in a root terminal $(ts)"
A114_PORTAL=$B/portal-$VER python3 -I $H/getsh_install.py $L $CELL main > $LOGS/install-$CELL-main.log 2>&1
python3 $H/insum.py $LOGS/install-$CELL-main.log main
echo "== systemd private socket (read-only) $(ts)"; lab ssh $CELL main -- "sudo -n tee /root/g_sock.sh >/dev/null" < $A/g_sock.sh; lab ssh $CELL main -- "sudo -n bash /root/g_sock.sh"
b1 login $CELL main | python3 -c 'import json,sys; d=json.load(sys.stdin); print("license", d["license_after"].get("state"), d["license_after"].get("license_kind"), "can_use_panel", d["access"].get("can_use_panel"))'
b1 sh $CELL main "cat /opt/celikpanel/release.commit 2>/dev/null; cat /etc/os-release | grep PRETTY; grep CELIKPANEL_LISTEN /etc/celikpanel/panel.env" --label pkgs | python3 -c 'import json,sys; print(json.load(sys.stdin)["stdout"])'
echo "== setup DNS engine catalogue (GET /api/v1/setup/components, read-only) $(ts)"
b1 api $CELL main GET /api/v1/setup/components --label components --limit 400000 > $LOGS/components-$CELL.json; cat_ $LOGS/components-$CELL.json
W=$B/wiz-$CELL; mkdir -p $W
python3 $H/wizdata.py $W single $Z $IP $SHORT here
infra delegate $Z ns1.$Z $IP ns2.$Z $IP > /dev/null
b1 api $CELL main PUT /api/v1/setup/guidance '{"revision":0,"guidance":"guided"}' --label guidance --limit 50 | st guidance
for s in domain count name roles names; do b1 save $CELL main @$W/$s.json --label $s --limit 300 | st $s 0; done
if [ -n "${PDNSFIRST:-}" ]; then
  echo "== owner picks PowerDNS (wizard default) $(ts)"
  b1 save $CELL main @$W/dnshome.json --label dnshome-pdns --limit 3000 | python3 -c 'import json,sys; d=json.load(sys.stdin); s=d.get("save",d); print("save dnshome(pdns)", s.get("status"), (s.get("body") or "")[:1500])'
  b1 records $CELL main > /dev/null
  b1 save $CELL main @$W/mail.json --label mail-pdns --limit 300 | st mail 0
  b1 plan $CELL main --label plan-pdns --limit 60000 > $LOGS/plan-pdns-$CELL.json; python3 $H/plansum2.py $LOGS/plan-pdns-$CELL.json plan-pdns
  python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=d["body"]; j=json.loads(b) if b.startswith("{") else {}; print("  refusal", d["status"], json.dumps({k:j.get(k) for k in ("code","error","reason","reason_code","next_action","who","message")})[:1500]); print("  body head", b[:1200])' $LOGS/plan-pdns-$CELL.json
  echo "== owner switches to BIND $(ts)"
  python3 -c 'import json,sys; p=sys.argv[1]; d=json.load(open(p)); d["dns_engine"]="bind"; json.dump(d,open(p,"w"))' $W/dnshome.json
fi
[ -n "${DNSENGINE:-}" ] && python3 -c 'import json,sys; p=sys.argv[1]; d=json.load(open(p)); d["dns_engine"]=sys.argv[2]; json.dump(d,open(p,"w"))' $W/dnshome.json $DNSENGINE
b1 save $CELL main @$W/dnshome.json --label dnshome --limit 300 | st dnshome 0
b1 records $CELL main > /dev/null
b1 save $CELL main @$W/mail.json --label mail --limit 300 | st mail 0
b1 api $CELL main GET /api/v1/setup/components --label components2 --limit 400000 > $LOGS/components2-$CELL.json; cat_ $LOGS/components2-$CELL.json
for i in $(seq 1 20); do
  b1 plan $CELL main --limit 60000 > $LOGS/plan-$CELL-main.json; python3 $H/plansum2.py $LOGS/plan-$CELL-main.json plan | cut -c1-900
  echo "== start $(ts) (try $i)"
  b1 start $CELL main --limit 3000 > $LOGS/start-$CELL.json; st start 600 < $LOGS/start-$CELL.json
  grep -q 'server_setup_distribution_updates_unread' $LOGS/start-$CELL.json || break
  echo "   distribution updates not read yet: the owner waits and presses Start again"; sleep 30
done
if [ -n "${HOLDDB:-}" ]; then
  sleep ${HOLDAFTER:-8}
  echo "== case 4: lab load holds the panel database write lock for ${HOLDSEC:-60} s $(ts)"
  if [ -n "${HOLDPAT:-}" ]; then lab ssh $CELL main -- "sudo -n tee /root/g_holddb2.sh >/dev/null" < $A/g_holddb2.sh; lab ssh $CELL main -- "sudo -n bash /root/g_holddb2.sh $HOLDPAT"
  else lab ssh $CELL main -- "sudo -n tee /root/g_holddb.sh >/dev/null" < $A/g_holddb.sh
  lab ssh $CELL main -- "sudo -n bash /root/g_holddb.sh ${HOLDSEC:-60}"; fi
  for i in $(seq 1 ${WATCHN:-45}); do echo "-- $(ts) $(opnow $CELL main 2>/dev/null | head -1 | cut -c1-700)"; sleep ${WATCHS:-3}; done > $LOGS/holddb-watch-$CELL.log 2>&1
  lab ssh $CELL main -- "sudo -n bash -c 'systemctl status lab-dbhold --no-pager 2>&1 | tail -4; journalctl -u lab-dbhold -o cat --no-pager | tail -5'"
  uniq -f2 $LOGS/holddb-watch-$CELL.log | cut -c1-500
fi
EVERY=20 TRIES=120 wait_status $CELL main succeeded failed
echo "== end $(ts)"
opnow $CELL main | head -1
echo "== single done $(ts)"
