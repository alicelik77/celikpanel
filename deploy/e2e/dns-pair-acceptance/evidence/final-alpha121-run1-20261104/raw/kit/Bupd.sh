# topology B: data on alpha.117 (Ubuntu 24.04), update to the candidate through the panel API, checks. Lab only.
CELL=u117 IP=192.0.3.191 Z=u117.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/Bupd-$CELL.log) 2>&1
if [ -z "${SKIPDATA:-}" ]; then
echo "== B data on 117 $(ts)"
mkcred custb
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"customer"}, open(sys.argv[2],"w"))' $T/cred-custb.json $T/u.json
adm POST /api/v1/users @$T/u.json --label create-custb | sbrief 200; rm -f $T/u.json
SUB=$(sub_of custb); echo $SUB > $T/sub
infra delegate cb117.test ns1.$Z $IP ns2.$Z $IP > /dev/null
DID=$(newdomain custb $SUB cb117.test); echo "cb117.test id=$DID"; echo $DID > $T/did
mkcred mbx-q
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"address":"q@cb117.test","password":c["password"],"quota_mb":1},open(sys.argv[2],"w"))' $T/cred-mbx-q.json $T/m.json
as custb POST /api/v1/domains/$DID/mail/accounts @$T/m.json | sbrief 200; rm -f $T/m.json
GQT=60 gq gB.sh
lc site - cb117.test
fi
SUB=$(cat $T/sub); DID=$(cat $T/did)
echo "== B update through the panel API $(ts)"
CHK=$(adm GET /api/v1/panel/update/check --label update-check --timeout 120); echo "$CHK" | python3 -c 'import json,sys; d=json.load(sys.stdin); print("check", d["status"], d["body"][:700].replace("\n"," "))'
echo "$CHK" | python3 -c '
import json,sys,secrets
chk=json.loads(json.load(sys.stdin)["body"])
body={"request_id":secrets.token_hex(16),"confirmed":True,"current_version":chk["current_version"],"current_commit":chk["current_commit"],**chk["target"]}
json.dump(body,open(sys.argv[1],"w")); print(body["request_id"])' $T/upd.json > $T/upd.rid
RID=$(cat $T/upd.rid)
adm POST /api/v1/panel/update/start @$T/upd.json --label update-start --timeout 120 | sbrief 300
UPT=$(date +%s); last=""
for i in $(seq 1 120); do sleep 10
  s=$(adm GET "/api/v1/panel/update/status?request_id=$RID" --label update-status --timeout 30 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d.get("body") or "{}") if d.get("status")==200 else {}; print(d.get("status"), b.get("status"), b.get("phase"))' 2>/dev/null)
  case "$s" in 401*) b1 login $CELL main >/dev/null 2>&1;; esac
  [ "$s" != "$last" ] && { echo "  $(( $(date +%s) - UPT ))s $s"; last="$s"; }
  case "$s" in "200 succeeded "*|"200 failed"*|"200 rolled_back"*|"200 recovered"*|"200 abandoned"*) [ "${s##* }" != verifying ] && break;; esac
done
echo "== B after update $(ts) ($(( $(date +%s) - UPT ))s)"
GQT=90 gq gB.sh
lc site - cb117.test
echo "-- usage status"; adm GET /api/v1/usage/status --label usage-status | sbrief 1200
echo "-- traffic: fetch twice"; lc fetch - cb117.test ""
echo "-- mail quota q@cb117.test: 950 KB then 300 KB"; lc smtpsize - q@cb117.test 950000; sleep 30; lc smtpsize - q@cb117.test 300000
echo "== B wait one usage round $(ts)"; waitround
subusage custb
GQT=90 gq gB.sh
echo "== B done $(ts)"
