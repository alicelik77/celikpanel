# alpha.127 case 3 (after the update to 127): the PostgreSQL database is created on the running 18 cluster (5433);
# a database backup and restore; phpPgAdmin names "PostgreSQL 18 (port 5433)". Lab only.
export CELL=${CELL:-r7} Z=${Z:-r7.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191} IP=${IP:-192.0.3.190}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
N=backup
exec > >(tee -a $LOGS/C3post-$CELL-$N.log) 2>&1
psq() { grun $N g_pg.sh sql $1 ${3:-postgres} $(printf %s "$2" | base64 -w0); }
echo "== versions $(ts)"; ver $N
DID=$(cat $T/pg7.id); echo "pg7.test id $DID"
pgrow
admn $N GET /api/v1/system-databases --label sysdb | sbrief 1500
echo "-- create a PostgreSQL database on 127 (the Databases tab's request) $(ts)"
PW=$(python3 -c 'import secrets; print(secrets.token_urlsafe(14)+"-Aa1")')
admn $N POST /api/v1/domains/$DID/databases "{\"name\":\"shop127\",\"type\":\"postgresql\",\"password\":\"$PW\"}" --label db-create-127 --timeout 300 | sbrief 900
admn $N GET /api/v1/domains/$DID/databases --label dbs > $T/dbs.json; sbrief 1500 < $T/dbs.json
DBN=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); d=[x for x in b["databases"] if x.get("type")=="postgresql"]; print(d[0]["name"] if d else "")' $T/dbs.json)
DBI=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); d=[x for x in b["databases"] if x.get("type")=="postgresql"]; print(d[0]["id"] if d else "")' $T/dbs.json)
echo "database $DBN id $DBI"
echo "-- placement: 5433 (18, running) vs 5432 (16, kept, no binaries)"
psq 5433 "select datname from pg_database where datname='$DBN'"
gshn $N "pg_lsclusters | cat"
echo "-- backup + restore of the PostgreSQL database $(ts)"
psq 5433 'create table lab_rows(v text); insert into lab_rows values ('"'before-backup'"'); DO $x$ BEGIN EXECUTE format('"'alter table lab_rows owner to %I'"', (select pg_get_userbyid(datdba) from pg_database where datname=current_database())); END $x$; select tableowner from pg_tables where tablename='"'lab_rows'"';' $DBN
admn $N POST /api/v1/domains/$DID/backups "{\"type\":\"database\",\"database_id\":$DBI}" --label backup-db --timeout 600 | sbrief 900
admn $N GET /api/v1/domains/$DID/backups --label backups > $T/bks.json; sbrief 1500 < $T/bks.json
BN=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); l=b.get("backups") if isinstance(b,dict) else b; l=[x for x in l if "db" in json.dumps(x).lower() or x.get("type")=="database"]; print(l[0].get("name") or l[0].get("file") if l else "")' $T/bks.json)
echo "backup $BN"
grun $N g_pg.sh find $BN
psq 5433 "update lab_rows set v='changed-after-backup'; select v from lab_rows;" $DBN
admn $N POST /api/v1/domains/$DID/backups/restore "{\"backup_name\":\"$BN\"}" --label restore-db --timeout 900 | sbrief 900
psq 5433 "select v from lab_rows;" $DBN
echo "-- phpPgAdmin $(ts)"
installc $N phppgadmin
pgrow
grun $N g_pg.sh ppa
admn $N GET /api/v1/managed-services --label ms > $T/ms.json; python3 -c '
import json,sys
b=json.loads(json.load(open(sys.argv[1]))["body"]); rows=b if isinstance(b,list) else (b.get("services") or b.get("items") or [])
for r in rows:
    if r.get("id") in ("phppgadmin","postgresql"): print(json.dumps(r)[:1500])' $T/ms.json
echo "== c3post done $(ts)"
