# alpha.119 run 5: data on alpha.117, then the panel's own update straight to the 119 candidate, then checks. Lab only.
# usage (env): CELL IP Z  (the guest is installed at 117 and set up)
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R9-$CELL.log) 2>&1
D=site9.test
echo "== data on 117 $(ts) ($CELL)"
gsh1 'cat /opt/celikpanel/release.commit 2>/dev/null; ls /opt/celikpanel'
PID=$(adm POST /api/v1/plans '{"name":"Plan117","max_domains":5,"max_databases":3,"max_email_accounts":3,"disk_quota_mb":2000,"bandwidth_quota_mb":0}' --label plan | jget id); echo "plan $PID"
RP=$(adm POST /api/v1/reseller-plans '{"name":"RPlan117","max_customers":5,"max_domains":10,"max_databases":5,"max_email_accounts":5,"disk_quota_mb":5000,"rights":{"all_permissions":true,"all_software":true,"permissions":[],"software":[]}}' --label rplan | jget id); echo "reseller plan $RP"
mkcred cust9; mkcred res9
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"customer","plan_id":int(sys.argv[2])}, open(sys.argv[3],"w"))' $T/cred-cust9.json $PID $T/u.json
adm POST /api/v1/users @$T/u.json --label cust | sbrief 200
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"reseller","rights":{"state":"plan","plan_id":int(sys.argv[2])}}, open(sys.argv[3],"w"))' $T/cred-res9.json $RP $T/u.json
adm POST /api/v1/users @$T/u.json --label res | sbrief 200; rm -f $T/u.json
SUB=$(sub_of cust9); echo $SUB > $T/sub
infra delegate $D ns1.$Z $IP ns2.$Z $IP > /dev/null
DID=$(newdomain cust9 $SUB $D); echo "$D id=$DID"; echo $DID > $T/did
python3 $A/mkup.py $T/t.json hello.txt text:hello-from-117 2>/dev/null; as cust9 POST "/api/v1/domains/$DID/files?path=/" @$T/t.json | sbrief 100
mkcred mbx9
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"address":"box","password":c["password"],"quota_mb":100},open(sys.argv[2],"w"))' $T/cred-mbx9.json $T/m.json
as cust9 POST /api/v1/domains/$DID/mail/accounts @$T/m.json | sbrief 200; rm -f $T/m.json
mkcred db9
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"name":"app9","type":"mysql","password":c["password"]},open(sys.argv[2],"w"))' $T/cred-db9.json $T/d.json
as cust9 POST /api/v1/domains/$DID/databases @$T/d.json > $T/dbr.json; sbrief 300 < $T/dbr.json; rm -f $T/d.json
as cust9 GET /api/v1/domains/$DID/databases > $T/dbl.json; DBU=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); L=b if isinstance(b,list) else (b.get("databases") or []); print(L[0]["user"] if L else "")' $T/dbl.json); echo "db user $DBU"; echo $DBU > $T/dbu
as cust9 POST /api/v1/domains/$DID/backups '{"type":"full"}' > $T/bk.json; sbrief 300 < $T/bk.json
python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print((b.get("backup") or {}).get("name",""))' $T/bk.json > $T/bn
echo "-- baseline"; lc get - $D hello.txt; lc smtp - box@$D; dbt db9 $DBU
echo "== update through the panel's own update path $(ts)"
CHK=$(adm GET /api/v1/panel/update/check --label update-check --timeout 120); echo "$CHK" | python3 -c 'import json,sys; d=json.load(sys.stdin); print("check", d["status"], d["body"][:600].replace("\n"," "))'
echo "$CHK" | python3 -c '
import json,sys,secrets
chk=json.loads(json.load(sys.stdin)["body"])
body={"request_id":secrets.token_hex(16),"confirmed":True,"current_version":chk["current_version"],"current_commit":chk["current_commit"],**chk["target"]}
json.dump(body,open(sys.argv[1],"w")); print(body["request_id"])' $T/upd.json > $T/upd.rid
RID=$(cat $T/upd.rid)
for k in 1 2 3 4 5 6; do r=$(adm POST /api/v1/panel/update/start @$T/upd.json --label update-start --timeout 120 | sbrief 300); echo "$r"; case "$r" in 409*PANEL_UPDATE_BUSY*) sleep 30;; *) break;; esac; done
UPT=$(date +%s); last=""
for i in $(seq 1 120); do sleep 10
  s=$(adm GET "/api/v1/panel/update/status?request_id=$RID" --label update-status --timeout 30 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d.get("body") or "{}") if d.get("status")==200 else {}; print(d.get("status"), b.get("status"), b.get("phase"))' 2>/dev/null)
  case "$s" in 401*) b1 login $CELL main >/dev/null 2>&1;; esac
  [ "$s" != "$last" ] && { echo "  $(( $(date +%s) - UPT ))s $s"; last="$s"; }
  case "$s" in "200 succeeded "*|"200 failed"*|"200 rolled_back"*|"200 recovered"*|"200 abandoned"*) [ "${s##* }" != verifying ] && break;; esac
done
echo "== after update $(ts)"
GQT=90 gq g_r9.sh
lc get - $D hello.txt
echo "-- mail"; lc smtpsize - box@$D 2000; sleep 5; gsh1 "doveadm fetch -u box@$D hdr.subject mailbox INBOX ALL 2>&1 | tail -2"
echo "-- database"; dbt db9 $DBU
echo "-- backup restore"; AS_TIMEOUT=600 as cust9 POST /api/v1/domains/$DID/backups/restore "{\"backup_name\":\"$(cat $T/bn)\"}" | sbrief 300; lc get - $D hello.txt
echo "-- plans and users"; adm GET /api/v1/plans --label plans | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); L=b if isinstance(b,list) else (b.get("plans") or []); [print("  plan", p.get("id"), p.get("name"), "disk", p.get("disk_quota_mb"), "bw", p.get("bandwidth_quota_mb")) for p in L]'
adm GET /api/v1/reseller-plans --label rplans | python3 -c 'import json,sys; d=json.load(sys.stdin); print("  reseller-plans", d["status"], d["body"][:300].replace("\n"," "))'
adm GET /api/v1/users --label users | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("  user", u["username"], u["role"], json.dumps(u.get("rights"))[:120]) for u in b["users"]]'
as cust9 GET /api/v1/my/subscription | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); s=b["subscriptions"][0]; u=s["usage"]; print("  cust9 limits: traffic_limit_bytes", u.get("traffic_limit_bytes"), "disk_limit_bytes", u.get("disk_limit_bytes"), "domains_limit", u.get("domains_limit"))'
echo "-- notices"; adm GET /api/v1/usage-notices --label notices | sbrief 600; adm GET /api/v1/rights/notices --label rnotices | sbrief 400
echo "-- guide"; gsh1 'curl -sk -o /dev/null -w "/help %{http_code}\n" https://127.0.0.1:1923/help'; adm GET /api/v1/me/guide-state --label gs | sbrief 100
if [ -n "${SSHCHECK:-}" ]; then
  echo "== Access tab SSH tools $(ts)"; CU=cust9 SUBF=sub bash $A/R4ssh.sh 2>&1 | grep -vE "^##" | tail -20
fi
echo "== R9 done $(ts)"
