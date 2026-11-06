#!/bin/bash
# alpha.124 pre-release round: fresh VM through get.sh from the lab portal of VER, one-server setup (web + mail,
# main site here) through the panel API. HOSTFQDN: the owner's machine name (set before install, lab load).
# DNSENGINE=bind picks BIND. TRUST=1 trusts the network lab portal (update path). Lab only.
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a125acc/kit/env.sh
source $H/common.sh
CELL=${CELL:-x}; IP=${IP:-192.0.3.190}; Z=${Z:-x.test}; VER=${VER:-v0.1.0-alpha.124}; MEM=${MEM:-4096}; SHORT=${SHORT:-frankfurt}; OS=${OS:-debian13}
HOSTFQDN=${HOSTFQDN:-host1.$Z}
exec > >(tee -a $LOGS/single-$CELL.log) 2>&1
cat_() { python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); print("host", b.get("host"), "inventory", b.get("inventory_state")); [print("  dns_engine", json.dumps(e)) for e in b.get("dns_engines") or []]' $1; }
echo "== provision $(ts) $OS $IP zone=$Z version=$VER host=$HOSTFQDN"
b1 provision $CELL main $OS $IP $SHORT $MEM > $LOGS/prov-$CELL-main.log 2>&1
tail -c 200 $LOGS/prov-$CELL-main.log; echo
echo "== the owner's machine name (lab load: hostnamectl + /etc/hosts) $(ts)"; sethost $CELL main $HOSTFQDN
[ -n "${TRUST:-}" ] && bash $A/portalF.sh trust $CELL main
echo "== install through get.sh in a root terminal $(ts)"
A114_PORTAL=$B/portal-$VER python3 -I $H/getsh_install.py $L $CELL main > $LOGS/install-$CELL-main.log 2>&1
python3 $H/insum.py $LOGS/install-$CELL-main.log main
[ -n "${TRUST:-}" ] && bash $A/portalF.sh trust $CELL main
b1 login $CELL main | python3 -c 'import json,sys; d=json.load(sys.stdin); print("license", d["license_after"].get("state"), d["license_after"].get("license_kind"), "can_use_panel", d["access"].get("can_use_panel"))'
b1 sh $CELL main "cat /opt/celikpanel/release.commit 2>/dev/null; cat /etc/os-release | grep PRETTY; grep CELIKPANEL_LISTEN /etc/celikpanel/panel.env; hostname -f" --label pkgs | python3 -c 'import json,sys; print(json.load(sys.stdin)["stdout"])'
b1 api $CELL main GET /api/v1/setup/components --label components --limit 400000 > $LOGS/components-$CELL.json; cat_ $LOGS/components-$CELL.json
W=$B/wiz-$CELL; mkdir -p $W
python3 $H/wizdata.py $W single $Z $IP $SHORT here
infra delegate $Z ns1.$Z $IP ns2.$Z $IP > /dev/null
b1 api $CELL main PUT /api/v1/setup/guidance '{"revision":0,"guidance":"guided"}' --label guidance --limit 50 | st guidance
for s in domain count name roles names; do b1 save $CELL main @$W/$s.json --label $s --limit 300 | st $s 0; done
[ -n "${DNSENGINE:-}" ] && python3 -c 'import json,sys; p=sys.argv[1]; d=json.load(open(p)); d["dns_engine"]=sys.argv[2]; json.dump(d,open(p,"w"))' $W/dnshome.json $DNSENGINE
b1 save $CELL main @$W/dnshome.json --label dnshome --limit 300 | st dnshome 0
b1 records $CELL main > /dev/null
b1 save $CELL main @$W/mail.json --label mail --limit 300 | st mail 0
b1 api $CELL main GET /api/v1/setup --label setup-snap --limit 400000 > $LOGS/setup-$CELL.json
echo "== case 2: the setup DNS plan names the machine $HOSTFQDN $(ts)"; hostrec $LOGS/setup-$CELL.json $HOSTFQDN
for i in $(seq 1 20); do
  b1 plan $CELL main --limit 60000 > $LOGS/plan-$CELL-main.json; python3 $H/plansum2.py $LOGS/plan-$CELL-main.json plan | cut -c1-900
  echo "== start $(ts) (try $i)"
  b1 start $CELL main --limit 3000 > $LOGS/start-$CELL.json; st start 600 < $LOGS/start-$CELL.json
  grep -q 'server_setup_distribution_updates_unread' $LOGS/start-$CELL.json || break
  echo "   distribution updates not read yet: the owner waits and presses Start again"; sleep 30
done
hostrec $LOGS/plan-$CELL-main.json $HOSTFQDN
EVERY=20 TRIES=120 wait_status $CELL main succeeded failed
echo "== end $(ts)"
opnow $CELL main | head -3
b1 api $CELL main GET /api/v1/setup/operation --label op-final --limit 400000 > $LOGS/op-$CELL.json; echo "== case 2: the setup operation's infrastructure DNS records name the machine"; hostrec $LOGS/op-$CELL.json $HOSTFQDN
echo "== case 2: $HOSTFQDN resolves after setup (labnet dig @$IP) $(ts)"
for t in A SOA; do echo "  $t $HOSTFQDN: $(dig1 $IP $HOSTFQDN $t)"; done
echo "  A $SHORT.$Z: $(dig1 $IP $SHORT.$Z A)  A mail.$Z: $(dig1 $IP mail.$Z A)"
echo "== single done $(ts)"
