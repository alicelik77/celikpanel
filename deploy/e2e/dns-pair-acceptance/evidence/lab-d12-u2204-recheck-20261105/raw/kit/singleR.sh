#!/bin/bash
# alpha.118 run 1 topology A: fresh Debian 13 through get.sh from the lab portal of the candidate,
# one-server wizard (PowerDNS local, web + mail, main site here) through the panel API. Lab only.
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/lab/kit/env.sh
CELL=${CELL:-a118}; IP=${IP:-192.0.3.190}; Z=${Z:-f118.test}; VER=${VER:-v0.1.0-alpha.118}; MEM=${MEM:-5120}; SHORT=${SHORT:-frankfurt}; OS=${OS:-debian13}
exec > >(tee -a $LOGS/single-$CELL.log) 2>&1
echo "== provision $(ts) $OS $IP zone=$Z version=$VER"
b1 provision $CELL main $OS $IP $SHORT $MEM > $LOGS/prov-$CELL-main.log 2>&1
tail -c 200 $LOGS/prov-$CELL-main.log; echo
[ -n "${TRUST:-}" ] && bash $A/portalF.sh trust $CELL main
echo "== install through get.sh in a root terminal $(ts)"
A114_PORTAL=$B/portal-$VER python3 -I $H/getsh_install.py $L $CELL main > $LOGS/install-$CELL-main.log 2>&1
python3 $H/insum.py $LOGS/install-$CELL-main.log main
[ -n "${TRUST:-}" ] && bash $A/portalF.sh trust $CELL main
b1 login $CELL main | python3 -c 'import json,sys; d=json.load(sys.stdin); print("license", d["license_after"].get("state"), d["license_after"].get("license_kind"), "can_use_panel", d["access"].get("can_use_panel"))'
b1 sh $CELL main "cat /opt/celikpanel/release.commit 2>/dev/null; cat /etc/os-release | grep PRETTY; grep CELIKPANEL_LISTEN /etc/celikpanel/panel.env" --label pkgs | python3 -c 'import json,sys; print(json.load(sys.stdin)["stdout"])'
echo "== setup DNS engine catalogue (GET /api/v1/setup/components, read-only) $(ts)"
b1 api $CELL main GET /api/v1/setup/components --label components --limit 400000 > $LOGS/components-$CELL.json; python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); print("host", b.get("host"), "inventory", b.get("inventory_state")); [print("  dns_engine", json.dumps(e)) for e in b.get("dns_engines") or []]' $LOGS/components-$CELL.json
W=$B/wiz-$CELL; mkdir -p $W
python3 $H/wizdata.py $W single $Z $IP $SHORT here
[ -n "${DNSENGINE:-}" ] && python3 -c 'import json,sys; p=sys.argv[1]; d=json.load(open(p)); d["dns_engine"]=sys.argv[2]; json.dump(d,open(p,"w"))' $W/dnshome.json $DNSENGINE
infra delegate $Z ns1.$Z $IP ns2.$Z $IP > /dev/null
b1 api $CELL main PUT /api/v1/setup/guidance '{"revision":0,"guidance":"guided"}' --label guidance --limit 50 | st guidance
for s in domain count name roles names dnshome; do b1 save $CELL main @$W/$s.json --label $s --limit 300 | st $s 0; done
b1 records $CELL main > /dev/null
b1 save $CELL main @$W/mail.json --label mail --limit 300 | st mail 0
echo "== catalogue again before review $(ts)"; b1 api $CELL main GET /api/v1/setup/components --label components2 --limit 400000 > $LOGS/components2-$CELL.json; python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); print("host", b.get("host"), "inventory", b.get("inventory_state")); [print("  dns_engine", json.dumps(e)) for e in b.get("dns_engines") or []]' $LOGS/components2-$CELL.json
b1 plan $CELL main --limit 60000 > $LOGS/plan-$CELL-main.json; python3 $H/plansum2.py $LOGS/plan-$CELL-main.json plan
if [ -n "${HOLDLOCK:-}" ]; then
  echo "== D1 recheck: lab load holds the dpkg frontend lock for 90 s $(ts)"
  lab ssh $CELL main -- "sudo -n systemd-run --unit=lab-dpkg-hold python3 -c \"import fcntl,time; f=open('/var/lib/dpkg/lock-frontend','w'); fcntl.lockf(f, fcntl.LOCK_EX); g=open('/var/lib/dpkg/lock','w'); fcntl.lockf(g, fcntl.LOCK_EX); time.sleep(90)\"; sleep 1; sudo -n systemctl is-active lab-dpkg-hold"
  ( for i in $(seq 1 30); do echo "-- $(ts) $(opnow $CELL main 2>/dev/null | head -1 | cut -c1-400)"; echo "   ledger: $(lab ssh $CELL main -- 'sudo -n cat /var/lib/celikpanel-agent-private/service-mutations.json' 2>/dev/null | head -c 600)"; sleep 8; done ) > $LOGS/d1watch-$CELL.log 2>&1 &
fi
echo "== start $(ts)"
b1 start $CELL main --limit 300 | st start 160
EVERY=20 TRIES=120 wait_status $CELL main succeeded failed
echo "== end $(ts)"
opnow $CELL main | head -1
echo "== single done $(ts)"
