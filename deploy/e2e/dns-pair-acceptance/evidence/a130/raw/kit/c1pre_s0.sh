# alpha.130 round, before (published alpha.129): the owner's content on the first server (Debian 13): PostgreSQL from
# Components next to MariaDB, an admin PHP site with a mailbox and a MariaDB database, a plan "Basic" with a customer,
# the customer's domain with a MariaDB and a PostgreSQL database and two mailboxes; a PHP site on the joined server.
# Then read-only hashes of every native web/PHP/mail file. Lab only.
export CELL=s0 Z=s0.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/C1pre-$CELL.log) 2>&1
echo "== versions $(ts)"; ver main; ver backup
echo "-- PostgreSQL from Components on the first server (as the window does it) $(ts)"
installc main postgresql
gshn main 'pg_lsclusters 2>&1 | cat; systemctl is-active mariadb postgresql 2>&1 | tr "\n" " "'
echo "-- admin PHP site site1.$Z $(ts)"
S1=$(mksite main site1.$Z php); echo "site1 id $S1"; echo $S1 > $T/site1.id
mkcred box1
admn main POST /api/v1/domains/$S1/mail/accounts "{\"address\":\"box1@site1.$Z\",\"password\":\"$(pw box1)\",\"quota_mb\":200}" --label mbox-create --timeout 300 | sbrief 500
mkcred db1
admn main POST /api/v1/domains/$S1/databases "{\"name\":\"shop\",\"type\":\"mysql\",\"password\":\"$(pw db1)\"}" --label db-create --timeout 300 | sbrief 600
echo "-- plan Basic + customer cust1 + the customer's domain $(ts)"
admn main POST /api/v1/plans '{"name":"Basic","max_domains":3,"max_databases":5,"max_email_accounts":10,"disk_quota_mb":2048,"bandwidth_quota_mb":0}' --label plan-create | sbrief 400
PID=$(admn main GET /api/v1/plans --label plans | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("plans") if isinstance(b,dict) else b; print([p["id"] for p in l if p.get("name")=="Basic"][0])'); echo "plan id $PID"; echo $PID > $T/planBasic.id
mkcred cust1
admn main POST /api/v1/users "{\"username\":\"cust1\",\"email\":\"cust1@lab.test\",\"password\":\"$(pw cust1)\",\"role\":\"customer\",\"plan_id\":$PID}" --label user-create | sbrief 400
SUB=$(sub_of cust1); echo "cust1 subscription $SUB"; echo $SUB > $T/sub-cust1.id
infra delegate cust1.$Z ns1.$Z $MIP ns2.$Z $BIP > /dev/null
as cust1 POST /api/v1/domains/create "{\"domain\":\"cust1.$Z\",\"project_type\":\"php\",\"ssl_type\":\"none\",\"subscription_id\":$SUB}" | sbrief 400
C1=$(domain_id cust1.$Z main); echo "cust1 site id $C1"; echo $C1 > $T/cust1.id
mkcred cdbm; mkcred cdbp; mkcred cbox1; mkcred cbox2
as cust1 POST /api/v1/domains/$C1/databases "{\"name\":\"shopm\",\"type\":\"mysql\",\"password\":\"$(pw cdbm)\"}" | sbrief 600
as cust1 POST /api/v1/domains/$C1/databases "{\"name\":\"shopp\",\"type\":\"postgresql\",\"password\":\"$(pw cdbp)\"}" | sbrief 600
as cust1 POST /api/v1/domains/$C1/mail/accounts "{\"address\":\"info@cust1.$Z\",\"password\":\"$(pw cbox1)\",\"quota_mb\":200}" | sbrief 500
as cust1 POST /api/v1/domains/$C1/mail/accounts "{\"address\":\"sales@cust1.$Z\",\"password\":\"$(pw cbox2)\",\"quota_mb\":200}" | sbrief 500
as cust1 GET /api/v1/domains/$C1/databases | sbrief 900
echo "-- joined server: PHP site bsite.$Z $(ts)"
BS=$(mksite backup bsite.$Z php); echo "bsite id $BS"; echo $BS > $T/bsite.id
sleep 20
for n in main backup; do echo "-- snapshot $n"; grun $n g_snap.sh pre129; done
gshn main 'ls /etc/nginx/sites-enabled | head -30; systemctl is-active postfix dovecot nginx mariadb postgresql 2>&1 | tr "\n" " "'
echo "== c1pre done $(ts)"
