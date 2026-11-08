# alpha.130 round, item 2: remote access to databases on A (Debian 13). usage: k2.sh PART...
# Listed client: labnet 192.0.3.1 (MariaDB resolves it to a name, '_gateway'). Lab only.
export CELL=s0 Z=s0.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/K2remote-$CELL.log) 2>&1
C1=$(cat $T/cust1.id); PID=$(cat $T/planBasic.id); SUB=$(cat $T/sub-cust1.id)
nup n_rdb.sh >/dev/null; b1 login $CELL main > /dev/null 2>&1
R=/api/v1/database-remote
g() { grun main g_k2.sh "$@"; }
dbst() { as cust1 GET $R/databases/$(cat $T/rdb-$1.id) | python3 -c '
import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]) if d["body"].startswith("{") else {}
rest={k:v for k,v in b.items() if k not in ("rules","users","access")}
print(" ", d["status"], json.dumps(rest)[:1000])
for r in b.get("rules") or []: print("   rule", json.dumps(r)[:500])'; }
srv() { adm GET $R/server --label rs | sbrief ${1:-1500}; }
conn() { local e=$1 m=$2 p=my; [ $e = postgresql ] && p=pg; pw rdb-$e | timeout 40 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_rdb.sh $p $MIP $(cat $T/rdb-$e.user) $(cat $T/rdb-$e.name) $m"; }
ruleid() { as cust1 GET $R/databases/$(cat $T/rdb-$1.id) | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=[r for r in (b.get("rules") or []) if r.get("address")==sys.argv[1]]; print(l[0]["id"] if l else "")' $2; }
addrule() { as cust1 POST $R/databases/$(cat $T/rdb-$1.id)/rules "{\"user_id\":$(cat $T/rdb-$1.uid),\"address\":\"$2\",\"duration\":\"${3:-1d}\"}" | sbrief 500; }
delrule() { local id=$(ruleid $1 $2); echo "   delete rule $id ($1 $2)"; as cust1 DELETE $R/rules/$id | sbrief 300; }
pollc() { local e=$1 want=$2 secs=$3 t0=$(date +%s) r; while :; do r=$(conn $e tls); if echo "$r" | grep -q " ok "; then [ $want = open ] && { echo "  $e open after $(( $(date +%s)-t0 )) s: $r"; return 0; }; else [ $want = closed ] && { echo "  $e closed after $(( $(date +%s)-t0 )) s: $r"; return 0; }; fi
  [ $(( $(date +%s)-t0 )) -ge $secs ] && { echo "  $e still not $want after $secs s: $r"; return 1; }; sleep 5; done; }
for part in "$@"; do echo; echo "######## $part $(ts)"; case $part in
setup)
  srv
  adm PUT $R/server '{"offered":true}' --label rs-on | sbrief 400
  adm GET /api/v1/plans --label plans > $T/plans.json
  python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); l=b.get("plans") if isinstance(b,dict) else b; p=[x for x in l if x.get("name")=="Basic"][0]; r=p.get("rights") or {}
out={"name":"Basic","max_domains":5,"max_databases":8,"max_email_accounts":10,"disk_quota_mb":4096,"bandwidth_quota_mb":0,"rights":{"all_permissions":True,"all_software":bool(r.get("all_software",True)),"permissions":r.get("permissions") or [],"software":r.get("software") or []},"confirm_narrowing":True}
json.dump(out,open(sys.argv[2],"w"))' $T/plans.json $T/planrights.json
  adm PUT /api/v1/plans/$PID @$T/planrights.json --label plan-all | sbrief 200
  for e in mysql postgresql; do E=my; [ $e = postgresql ] && E=pg; [ -s $T/rdb-$e.id ] && continue
    mkcred rdb-$e
    as cust1 POST /api/v1/domains/$C1/databases "{\"name\":\"r$E\",\"type\":\"$e\",\"password\":\"$(pw rdb-$e)\"}" | sbrief 300
    as cust1 GET /api/v1/domains/$C1/databases > $T/cdbs.json
    python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); l=b.get("databases") if isinstance(b,dict) else b; x=[d for d in l if d["name"].endswith("_r"+sys.argv[2])][0]; u=(x.get("users") or [{}])[0]; print(x["id"], x["name"], x.get("user"), u.get("id")); open(sys.argv[3]+".id","w").write(str(x["id"])); open(sys.argv[3]+".name","w").write(x["name"]); open(sys.argv[3]+".user","w").write(x.get("user") or ""); open(sys.argv[3]+".uid","w").write(str(u.get("id") or ""))' $T/cdbs.json $E $T/rdb-$e
  done
  srv ;;
pgrestart)
  admn main POST /api/v1/service/action '{"name":"postgresql","action":"restart"}' --label svc-restart-pg --timeout 300 | sbrief 300
  sleep 30; dbst postgresql; pollc postgresql open 120; g pg
  for i in 1 2 3; do conn postgresql wrong; done ;;
pgopen)
  g f2b; addrule postgresql 192.0.3.1; sleep 20; dbst postgresql; pollc postgresql open 120; g pg
  for i in 1 2 3; do conn postgresql wrong; done ;;
myname)
  addrule mysql 192.0.3.1; sleep 25; dbst mysql; srv 2500; g skip; conn mysql tls ;;
myskip)
  adm PUT $R/server '{"mariadb_skip_name_resolve":true}' --label rs-skip | sbrief 600
  sleep 5; g skip
  admn main POST /api/v1/service/action '{"name":"mariadb","action":"restart"}' --label svc-restart --timeout 300 | sbrief 300
  sleep 25; g skip; dbst mysql; pollc mysql open 150 ;;
f2binst)
  installc main fail2ban; sleep 30; g f2b; g files ;;
myban)
  echo "-- wrong MariaDB passwords from 192.0.3.1"; for i in 1 2 3 4 5 6; do conn mysql wrong; done; sleep 10; g f2b
  conn mysql tls; g unban 192.0.3.1; sleep 3; conn mysql tls ;;
pgf2b)
  echo "-- the PostgreSQL rule with Fail2ban running: prefix + jail"; adm PUT $R/server '{"apply":true}' --label rs-apply | sbrief 300; sleep 40; g pg; g files; g f2b; dbst postgresql
  echo "-- wrong PostgreSQL passwords"; for i in 1 2 3 4 5 6; do conn postgresql wrong; done; sleep 10; g f2b
  conn postgresql tls; g unban 192.0.3.1; sleep 3; conn postgresql tls ;;
mylast)
  delrule mysql 192.0.3.1; pollc mysql closed 60; sleep 30; g skip; g files; g f2b ;;
pglast)
  delrule postgresql 192.0.3.1; pollc postgresql closed 60; sleep 30; g pg; g files; g f2b ;;
expiry)
  addrule mysql 192.0.3.1 1d; addrule postgresql 192.0.3.1 7d; sleep 30; dbst mysql; dbst postgresql; pollc mysql open 90; pollc postgresql open 60
  g ruletab
  RM=$(ruleid mysql 192.0.3.1); echo "-- expire MariaDB rule $RM in the database (lab)"; g expire $RM; T0=$(date +%s)
  pollc mysql closed 150; echo "   expiry closed in $(( $(date +%s)-T0 )) s"; g nft; g ruletab; g skip
  RP=$(ruleid postgresql 192.0.3.1); echo "-- extend PostgreSQL rule $RP to no end"; as cust1 PUT $R/rules/$RP '{"duration":"permanent"}' | sbrief 400; dbst postgresql ;;
flush)
  addrule mysql 192.0.3.1 1d; sleep 40; pollc mysql open 90; g nft
  echo "-- nft flush ruleset $(ts)"; gshn main "nft flush ruleset; date -u +%T"
  T0=$(date +%s); while [ $(( $(date +%s)-T0 )) -lt 75 ]; do echo "  +$(( $(date +%s)-T0 ))s tables: $(gshn main 'nft list tables 2>&1 | xargs')"; sleep 5; done
  g nft; pollc mysql open 60; gshn main 'nft list table inet celikpanel_fw 2>/dev/null | grep -E "3306|5432" | head -6' ;;
flushoff)
  gshn main 'systemctl disable celikpanel-firewall-restore.service 2>&1 | tail -1; nft flush ruleset; date -u +%T'
  sleep 60; echo "  tables after 60 s: $(gshn main 'nft list tables 2>&1 | xargs')"; srv 3000
  gshn main 'journalctl -u celikpanel-agent -u celikpanel-panel --since -3min -o cat --no-pager | grep -iE "nft|firewall|restore" | tail -6 | cut -c1-260'
  echo "-- the owner turns the restore back on and runs it"; gshn main 'systemctl enable celikpanel-firewall-restore.service 2>&1 | tail -1; systemctl start celikpanel-firewall-restore.service; nft list tables | xargs'
  sleep 40; g nft ;;
starting)
  g hang; echo "-- reboot with a unit holding the boot $(ts)"; lab reboot $CELL main > /dev/null 2>&1; lab wait-ssh $CELL main 300 > /dev/null 2>&1
  for i in $(seq 1 12); do b1 login $CELL main > /dev/null 2>&1 && break; sleep 10; done
  g state; dbst mysql
  T0=$(date +%s); delrule mysql 192.0.3.1
  for i in $(seq 1 16); do s=$(g state); echo "  +$(( $(date +%s)-T0 ))s $(echo "$s" | tr '\n' ' ' | cut -c1-200)"; echo "$s" | grep -q "192.0.3" || break; sleep 5; done
  dbst mysql; srv 2000
  echo "-- wait for the boot to finish (up to 6 min)"; for i in $(seq 1 40); do s=$(g state | head -1); echo "$s" | grep -qE "running|degraded" && break; sleep 10; done; echo "  $s $(ts)"
  sleep 40; dbst mysql; g skip; g nft; g unhang ;;
esac; done
echo "== k2 $* done $(ts)"
