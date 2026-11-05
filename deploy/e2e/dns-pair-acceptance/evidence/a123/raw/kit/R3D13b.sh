# alpha.123 case 3 part 2 on Debian 13 (d13): Node.js 24 with n8n, removal refused; then Fail2ban without the
# firewall engine (the lab plays an owner who removes nftables outside the panel - last step). Lab only.
export CELL=d13 IP=192.0.3.192 Z=d13.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a123/kit/lib.sh
source $A/lib11.sh; source $A/lib123.sh
exec > >(tee -a $LOGS/R3b-$CELL.log) 2>&1
SUB=$(cat $T/sub)
if [ -z "${SKIPN8N:-}" ]; then
echo "== 3.7 Node.js 24 + n8n $(ts)"
adm GET /api/v1/runtimes/node/lts --label node-lts > $T/lts.json
NV=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); v=[r["version"].lstrip("v") for r in b.get("releases",[]) if r["version"].lstrip("v").startswith("24.")]; print(v[0] if v else "")' $T/lts.json); echo "node $NV"; echo $NV > $T/nv
R=$(rid); svcstart "{\"version\":\"$NV\",\"request_id\":\"$R\"}" /api/v1/runtimes/node && svcwait $R 900
adm GET /api/v1/admin/service-apps --label sa > $T/sa.json
DG=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print([a for a in b["apps"] if a["id"]=="n8n"][0]["risk_digest"])' $T/sa.json)
python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print("  sandbox", json.dumps(b.get("sandbox"))[:400])' $T/sa.json
adm POST /api/v1/admin/service-apps/n8n/offer "{\"risk_digest\":\"$DG\"}" --label offer-n8n | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print("offer n8n", d["status"], [(a["id"],a.get("offered"),a.get("available"),a.get("reason")) for a in b.get("apps",[]) if a["id"]=="n8n"])'
infra delegate n8n123.test ns1.$Z $IP ns2.$Z $IP > /dev/null
NID=$(newdomain cust123 $SUB n8n123.test letsencrypt); echo "n8n123.test id=$NID"; echo $NID > $T/nid
c=$(as cust123 GET /api/v1/domains/$NID/service-apps | jget certificate); echo "  certificate after create: $c"
if ! echo "$c" | grep -qiE 'true'; then AS_TIMEOUT=300 as cust123 POST /api/v1/domains/$NID/ssl/letsencrypt '{"auto_renew":true}' | sbrief 300; fi
mkcred n8nowner
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"app":"n8n","owner_email":"owner@n8n123.test","owner_password":c["password"]+"Zz9","confirm_replace":True},open(sys.argv[2],"w"))' $T/cred-n8nowner.json $T/n8n.json
echo "-- install n8n $(ts)"; as cust123 POST /api/v1/domains/$NID/service-apps/install @$T/n8n.json | sbrief 500; rm -f $T/n8n.json
last=""
for i in $(seq 1 100); do
  s=$(as cust123 GET /api/v1/domains/$NID/service-apps | python3 $A/sastat.py)
  [ "$s" != "$last" ] && { echo "  $(ts) $s"; last="$s"; }
  case "$s" in *"install=running active=True"*|*"install=failed"*|*"op=failed"*) break ;; esac
  sleep 15
done
fi
NV=$(cat $T/nv)
echo "== 3.8 remove Node.js $NV while n8n runs on it $(ts)"
impact "service_id=node&version=$NV"
adm DELETE /api/v1/runtimes/node/$NV --label node-remove --timeout 300 | sbrief 1500
gsh1 "systemctl list-units --no-pager --plain 'celiksvc*' 'celikrt*' | head -5; ls /opt/celikpanel-runtimes/node 2>/dev/null || ls /opt | head"
echo "== 3.9 Fail2ban without the firewall engine $(ts)"
echo "-- LAB: the owner removes the nftables package outside the panel (apt-get remove nftables), as an owner change"
gsh1 'DEBIAN_FRONTEND=noninteractive apt-get remove -y -q nftables 2>&1 | grep -E "^(Removing|The following)" ; dpkg -l nftables fail2ban 2>/dev/null | grep -E "^(ii|rc)" | awk "{print \$1, \$2, \$3}"'
adm POST /api/v1/managed-services/scan '{}' --label scan --timeout 300 | python3 -c 'import json,sys; d=json.load(sys.stdin); print("scan", d["status"])'
comps fail2ban,nftables
python3 -c '
import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); L=b if isinstance(b,list) else (b.get("services") or [])
for s in L:
  if s.get("id")=="fail2ban": print("  fail2ban row", json.dumps(s)[:1500])' $T/ms.json
echo "== R3 part 2 done $(ts)"
