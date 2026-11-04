# alpha.121 U (Ubuntu 24.04 at 120, Boston-like): PG16 + phpPgAdmin + DB, remove PG, PGDG lane, busy-stop update UX,
# update to 121, busy wording, PGDG 17/18 previews, stale lists, PG18 install. Lab only.
export CELL=u120 IP=192.0.3.191 Z=u120.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
source $A/lib11.sh
exec > >(tee -a $LOGS/R11U-$CELL.log) 2>&1
echo "== 1 on 120 $(ts)"; gsh1 'cat /opt/celikpanel/release.commit 2>/dev/null; grep PRETTY /etc/os-release'
installc postgresql
installc phppgadmin
gsh1 'php -m 2>/dev/null | grep -i pgsql; pg_lsclusters'
mkcred cust11
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"customer"}, open(sys.argv[2],"w"))' $T/cred-cust11.json $T/u.json
adm POST /api/v1/users @$T/u.json --label cust | sbrief 100; rm -f $T/u.json
SUB=$(sub_of cust11); DID=$(newdomain cust11 $SUB pg11.test); echo "pg11.test id=$DID"
mkcred pgdb
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"name":"keep16","type":"postgresql","password":c["password"]},open(sys.argv[2],"w"))' $T/cred-pgdb.json $T/d.json
as cust11 POST /api/v1/domains/$DID/databases @$T/d.json | sbrief 300; rm -f $T/d.json
adm GET /api/v1/database-servers --label dbs | sbrief 600
echo "-- remove PostgreSQL"; adm GET "/api/v1/service/removal-impact?service_id=postgresql" --label impact | sbrief 400
adm POST /api/v1/service/uninstall '{"service_id":"postgresql"}' --label pg-remove --timeout 900 | sbrief 500
gsh1 'pg_lsclusters 2>&1; ls -la /var/lib/postgresql/ 2>&1; ls /var/lib/postgresql/16/main 2>&1 | head -3'
echo "-- enable PGDG from Components (vendor lane)"; lane
adm GET "/api/v1/repo?service_id=postgresql" --label repo | sbrief 500
echo "== 6 update UX: dpkg lock held, start the update from 120 $(ts)"
lab ssh $CELL main -- "sudo -n systemd-run --unit=lab-dpkg-hold python3 -c \"import fcntl,time; f=open('/var/lib/dpkg/lock-frontend','w'); fcntl.lockf(f, fcntl.LOCK_EX); g=open('/var/lib/dpkg/lock','w'); fcntl.lockf(g, fcntl.LOCK_EX); time.sleep(600)\"; sleep 1; sudo -n systemctl is-active lab-dpkg-hold"
upbody() { adm GET /api/v1/panel/update/check --label update-check --timeout 120 | python3 -c '
import json,sys,secrets
chk=json.loads(json.load(sys.stdin)["body"])
body={"request_id":secrets.token_hex(16),"confirmed":True,"current_version":chk["current_version"],"current_commit":chk["current_commit"],**chk["target"]}
json.dump(body,open(sys.argv[1],"w")); print(body["request_id"], chk["target"].get("version"))' $T/upd.json; }
upbody > $T/upd.rid; echo "  request $(cat $T/upd.rid)"
adm POST /api/v1/panel/update/start @$T/upd.json --label update-start-busy --timeout 120 | sbrief 500
sleep 30; RID=$(cut -d' ' -f1 $T/upd.rid)
adm GET "/api/v1/panel/update/status?request_id=$RID" --label upd-status-busy | sbrief 700
adm GET /api/v1/host-mutation-readiness --label readiness-held | sbrief 300
echo "-- release the lock $(ts)"; lab ssh $CELL main -- "sudo -n systemctl stop lab-dpkg-hold; sleep 2; sudo -n systemctl is-active lab-dpkg-hold"
adm GET /api/v1/host-mutation-readiness --label readiness | sbrief 300
echo "-- one press of Update (new request id) $(ts)"
upbody > $T/upd.rid; RID=$(cut -d' ' -f1 $T/upd.rid)
adm POST /api/v1/panel/update/start @$T/upd.json --label update-start --timeout 120 | sbrief 300
UPT=$(date +%s); last=""
for i in $(seq 1 120); do sleep 5
  s=$(adm GET "/api/v1/panel/update/status?request_id=$RID" --label update-status --timeout 30 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d.get("body") or "{}") if d.get("status")==200 else {}; print(d.get("status"), b.get("status"), b.get("phase"))' 2>/dev/null)
  case "$s" in 401*) b1 login $CELL main >/dev/null 2>&1;; esac
  [ "$s" != "$last" ] && { echo "  $(( $(date +%s) - UPT ))s $s"; last="$s"; }
  case "$s" in "200 succeeded "*|"200 failed"*|"200 rolled_back"*|"200 recovered"*|"200 abandoned"*) [ "${s##* }" != verifying ] && break;; esac
done
echo "== 2 busy wording right after the update $(ts)"
for k in 1 2 3 4 5 6; do r=$(adm POST /api/v1/service/install "{\"service_id\":\"go\",\"request_id\":\"$(rid)\"}" --label go-after | sbrief 500); echo "  $(ts) $r"; case "$r" in 409*) sleep 5;; *) break;; esac; done
gsh1 'cat /opt/celikpanel/release.commit; systemctl is-active celikpanel-panel celikpanel-agent nginx postfix dovecot mariadb named | tr "\n" " "; echo; nginx -t 2>&1 | tail -1; echo "panics: $(journalctl -u celikpanel-panel -u celikpanel-agent --no-pager | grep -ciE "panic:|goroutine [0-9]+ \[")"'
sleep 30
echo "== 4 preview PGDG 17 (18 not installed) $(ts)"; adm GET "/api/v1/service/install-preview?step=postgresql:postgresql-17" --label prev17 --timeout 150 > $T/p17.json; prevsum $T/p17.json
echo "== 5 stale lists: delete PGDG list files (lab load), then preview 18 $(ts)"
gsh1 'ls /var/lib/apt/lists/ | grep -i postgresql; rm -f /var/lib/apt/lists/*apt.postgresql.org*; ls /var/lib/apt/lists/ | grep -ci postgresql'
adm GET "/api/v1/service/install-preview?step=postgresql:postgresql-18" --label prev18-stale --timeout 150 > $T/p18s.json; prevsum $T/p18s.json
echo "== 3 PGDG 18 $(ts)"; installc postgresql postgresql-18
gsh1 'pg_lsclusters; systemctl is-active postgresql "postgresql@18-main" 2>&1 | tr "\n" " "; echo; php -m | grep -i pgsql; dpkg -l postgresql-common postgresql-client-common libpq5 | grep ^ii | awk "{print \$2, \$3}"; ls -la /var/lib/postgresql/; ls /var/lib/postgresql/16/main 2>&1 | head -3'
adm GET /api/v1/database-servers --label dbs2 | sbrief 800
echo "== R11U done $(ts)"
