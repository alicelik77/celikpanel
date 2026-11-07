# alpha.129 case 4: remote access to a customer database (cust1, plan Basic) on one engine of a server.
# usage: k4remote.sh mysql|postgresql [part...]   parts: setup open checks fail2ban pass suspend plan last reboot flush
# env: CELL Z MIP (defaults: the pair's main). Listed client: labnet 192.0.3.1; unlisted: the lab host 192.0.3.2. Lab only.
export CELL=${CELL:-r9} Z=${Z:-r9.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191} IP=${IP:-192.0.3.190}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
ENG=$1; shift; E=my; PORT=3306; SVC=mariadb; [ $ENG = postgresql ] && { E=pg; PORT=5432; SVC=postgresql; }
exec > >(tee -a $LOGS/C4remote-$CELL-$ENG.log) 2>&1
C1=$(cat $T/cust1.id); PID=$(cat $T/planBasic.id); SUB=$(cat $T/sub-cust1.id)
nup n_rdb.sh >/dev/null
R=/api/v1/database-remote
st() { as cust1 GET $R/databases/$(cat $T/rdb-$ENG.id) | python3 -c '
import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]) if d["body"].startswith("{") else {}
print(d["status"], "access", json.dumps(b.get("access"))[:200]); e=b.get("engine") or b.get("instance") or {}
print("   engine", json.dumps(e)[:700])
for r in b.get("rules") or []: print("   rule", json.dumps(r)[:400])'; }
conn() { pw rdb-$ENG | timeout 40 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_rdb.sh $E $MIP $(cat $T/rdb-$ENG.user) $(cat $T/rdb-$ENG.name) $1"; }
connold() { pw rdb-$ENG-old | timeout 40 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_rdb.sh $E $MIP $(cat $T/rdb-$ENG.user) $(cat $T/rdb-$ENG.name) tls"; }
unlisted() { T0=$(date +%s%N); if timeout 8 bash -c "</dev/tcp/$MIP/$PORT" 2>/dev/null; then echo "  from 192.0.3.2 (unlisted): port OPEN [$(( ($(date +%s%N)-T0)/1000000 )) ms]"; else echo "  from 192.0.3.2 (unlisted): no connection [$(( ($(date +%s%N)-T0)/1000000 )) ms]"; fi; }
plan_perm() { adm GET /api/v1/plans --label plans > $T/plans.json
  python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); l=b.get("plans") if isinstance(b,dict) else b; p=[x for x in l if x.get("name")=="Basic"][0]; r=p.get("rights") or {}
perms=set(r.get("permissions") or []) if sys.argv[3]=="on" else set("dns_edit dnssec mail databases cron file_manager backups ssl_acme ssl_custom site_protect_redirects site_proxy app_installer vpn team_members file_access ssh remote_backup_destinations program_apps python_apps php_settings php_version".split())
out={"name":"Basic","max_domains":5,"max_databases":8,"max_email_accounts":10,"disk_quota_mb":4096,"bandwidth_quota_mb":0,"rights":{"all_permissions":sys.argv[3]=="on","all_software":bool(r.get("all_software",True)),"permissions":sorted(perms),"software":r.get("software") or []},"confirm_narrowing":True}
json.dump(out,open(sys.argv[2],"w"))' $T/plans.json $T/planrights.json $1
  adm PUT /api/v1/plans/$PID @$T/planrights.json --label plan-dbremote-$1 | sbrief 200; }
pollconn() { local want=$1 secs=$2 t0=$(date +%s) r; while :; do r=$(conn tls); if echo "$r" | grep -q " ok "; then [ $want = open ] && { echo "  open after $(( $(date +%s)-t0 )) s: $r"; return 0; }; else [ $want = closed ] && { echo "  closed after $(( $(date +%s)-t0 )) s: $r"; return 0; }; fi
  [ $(( $(date +%s)-t0 )) -ge $secs ] && { echo "  still not $want after $secs s: $r"; return 1; }; sleep 5; done; }
for part in "$@"; do case $part in
setup)
  echo "== 4.0 the offer, plan permission, a $ENG database of cust1 $(ts)"
  [ $E = pg ] && { admn main GET /api/v1/managed-services --label ms | grep -q '"postgresql"' ; installc main postgresql; }
  adm GET $R/server --label rs | sbrief 900
  adm PUT $R/server '{"offered":true}' --label rs-on | sbrief 600
  plan_perm on
  mkcred rdb-$ENG
  as cust1 POST /api/v1/domains/$C1/databases "{\"name\":\"r$E\",\"type\":\"$ENG\",\"password\":\"$(pw rdb-$ENG)\"}" | sbrief 400
  as cust1 GET /api/v1/domains/$C1/databases > $T/cdbs.json
  python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); l=b.get("databases") if isinstance(b,dict) else b; x=[d for d in l if d["name"].endswith("_r"+sys.argv[2])][0]; print(x["id"], x["name"], x.get("user") or ""); open(sys.argv[3]+".id","w").write(str(x["id"])); open(sys.argv[3]+".name","w").write(x["name"]); open(sys.argv[3]+".user","w").write(x.get("user") or "")' $T/cdbs.json $E $T/rdb-$ENG
  st ;;
open)
  echo "== 4.1 a rule for the lab client 192.0.3.1, then the refusals $(ts)"
  UIDR=$(as cust1 GET $R/databases/$(cat $T/rdb-$ENG.id) | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(((b.get("users") or [{}])[0]).get("id",""))')
  echo "   user id $UIDR"; echo $UIDR > $T/rdb-$ENG.uid
  as cust1 POST $R/databases/$(cat $T/rdb-$ENG.id)/rules "{\"user_id\":$UIDR,\"address\":\"192.0.3.1\"}" | sbrief 600
  for a in 0.0.0.0/0 10.0.0.0/8 127.0.0.1 169.254.1.1; do echo "   refused $a:"; as cust1 POST $R/databases/$(cat $T/rdb-$ENG.id)/rules "{\"user_id\":$UIDR,\"address\":\"$a\"}" | sbrief 250; done
  st; conn tls; unlisted ;;
restart)
  echo "== 4.2 the administrator restarts $SVC in Components $(ts)"
  admn main POST /api/v1/service/action "{\"name\":\"$SVC\",\"action\":\"restart\"}" --label svc-restart --timeout 300 | sbrief 300
  sleep 20; adm PUT $R/server '{"apply":true}' --label rs-apply | sbrief 300; sleep 15; st ;;
checks)
  echo "== 4.3 listed TLS / listed plain / unlisted $(ts)"
  conn tls; conn plain; unlisted
  grun main g_rstate.sh $PORT ;;
fail2ban)
  echo "== 4.4 wrong passwords from the listed address $(ts)"
  for i in 1 2 3 4 5 6 7; do conn wrong; done
  conn tls
  grun main g_f2b.sh status
  if [ $E = my ]; then echo "   lab: unban the lab client"; grun main g_f2b.sh unban; sleep 3; conn tls; fi ;;
pass)
  echo "== 4.5 password change: the old one fails remotely $(ts)"
  cp $T/cred-rdb-$ENG.json $T/cred-rdb-$ENG-old.json; rm -f $T/cred-rdb-$ENG.json; mkcred rdb-$ENG
  UIDP=$(as cust1 GET /api/v1/domains/$C1/databases | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("databases") if isinstance(b,dict) else b; x=[d for d in l if str(d["id"])==sys.argv[1]][0]; u=(x.get("users") or [{}])[0]; print(u.get("id") or x.get("user_id") or "")' $(cat $T/rdb-$ENG.id))
  python3 -c 'import json,os,sys; p=json.load(open(sys.argv[1]))["password"]; fd=os.open(sys.argv[2],os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600); os.write(fd,json.dumps({"password":p}).encode())' $T/cred-rdb-$ENG.json $T/pwbody.json
  as cust1 POST /api/v1/domains/$C1/databases/$(cat $T/rdb-$ENG.id)/users/$UIDP/password @$T/pwbody.json | sbrief 500; rm -f $T/pwbody.json
  sleep 5; echo "   old:"; connold; echo "   new:"; conn tls ;;
suspend)
  echo "== 4.6 suspension blocks remote login at once $(ts)"
  T0=$(date +%s); adm POST /api/v1/subscriptions/$SUB/suspension '{"action":"suspend","reason":"lab"}' --label susp --timeout 300 | sbrief 400
  pollconn closed 60
  adm POST /api/v1/subscriptions/$SUB/suspension '{"action":"resume","reason":"lab"}' --label unsusp --timeout 300 | sbrief 400
  pollconn open 120 ;;
plan)
  echo "== 4.7 plan permission withdrawn: closes within ~35 s $(ts)"
  plan_perm off; pollconn closed 90; st
  plan_perm on; pollconn open 120 ;;
last)
  echo "== 4.8 the last rule removed closes at once $(ts)"
  RID=$(as cust1 GET $R/databases/$(cat $T/rdb-$ENG.id) | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(((b.get("rules") or [{}])[0]).get("id",""))')
  as cust1 DELETE $R/rules/$RID | sbrief 400; pollconn closed 30; unlisted
  grun main g_rstate.sh $PORT
  echo "   the rule back:"; as cust1 POST $R/databases/$(cat $T/rdb-$ENG.id)/rules "{\"user_id\":$(cat $T/rdb-$ENG.uid),\"address\":\"192.0.3.1\"}" | sbrief 300; pollconn open 60 ;;
reboot)
  echo "== 4.9 reboot keeps the filter $(ts)"
  lab reboot $CELL main > /dev/null 2>&1; lab wait-ssh $CELL main 300 > /dev/null 2>&1; sleep 30; b1 login $CELL main > /dev/null 2>&1
  grun main g_rstate.sh $PORT
  unlisted; pollconn open 120 ;;
flush)
  echo "== 4.10 nft flush ruleset: the guard restores the filter; is the port open meanwhile? $(ts)"
  gshn main "nft flush ruleset; date -u +%T"
  T0=$(date +%s); while [ $(( $(date +%s)-T0 )) -lt 45 ]; do echo "  +$(( $(date +%s)-T0 ))s $(unlisted | sed 's/^ *//') | filter: $(grun main g_nftq.sh 2>/dev/null | tail -1)"; sleep 3; done
  grun main g_rstate.sh $PORT ;;
esac; done
echo "== k4remote $ENG $* done $(ts)"
