# alpha.127 case 3 fix check: the joined server updated (panel Update API) to a lab 128 label of release/alpha127
# 20957e440 (the PostgreSQL restore fix); then the database backup's restore again, and a backup made on 127. Lab only.
export CELL=${CELL:-r7} Z=${Z:-r7.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191} IP=${IP:-192.0.3.191}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
N=backup
psq() { grun $N g_pg.sh sql $1 ${3:-postgres} $(printf %s "$2" | base64 -w0); }
bash $A/portalF.sh latest v0.1.0-alpha.128
CELL=r7 IP=192.0.3.191 bash $A/upd.sh backup > $A/c3fix-upd.out 2>&1
exec > >(tee -a $LOGS/C3fix-$CELL-$N.log) 2>&1
grep -E "^  [0-9]+s |schema_version|^\(end\)|unverified" $A/c3fix-upd.out | cut -c1-250
DID=$(cat $T/pg7.id); DBN=pg7_test_shop127
echo "== restore the backup made on 127 (before the fix) into the non-empty database $(ts)"
admn $N GET /api/v1/domains/$DID/backups --label backups > $T/bks.json; sbrief 800 < $T/bks.json
BN=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); l=[x for x in b["backups"] if x.get("type")=="database" and x.get("origin")=="manual"]; print(sorted(l,key=lambda x:x["created_at"])[0]["name"])' $T/bks.json)
echo "backup $BN"
psq 5433 "select v from lab_rows; select tablename||' '||tableowner from pg_tables where schemaname='public';" $DBN
admn $N POST /api/v1/domains/$DID/backups/restore "{\"backup_name\":\"$BN\"}" --label restore-db-fix --timeout 900 | sbrief 900
psq 5433 "select v from lab_rows; select tablename||' '||tableowner from pg_tables where schemaname='public';" $DBN
echo "== a new backup, a change, a restore $(ts)"
admn $N POST /api/v1/domains/$DID/backups '{"type":"database","database_id":1}' --label backup-db2 --timeout 600 | sbrief 500
admn $N GET /api/v1/domains/$DID/backups --label backups > $T/bks.json
BN2=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); l=[x for x in b["backups"] if x.get("type")=="database" and x.get("origin")=="manual"]; print(sorted(l,key=lambda x:x["created_at"])[-1]["name"])' $T/bks.json)
psq 5433 "update lab_rows set v='changed-after-second-backup'; insert into lab_rows values ('extra-row'); select v from lab_rows;" $DBN
admn $N POST /api/v1/domains/$DID/backups/restore "{\"backup_name\":\"$BN2\"}" --label restore-db-fix2 --timeout 900 | sbrief 900
psq 5433 "select v from lab_rows; select tablename||' '||tableowner from pg_tables where schemaname='public';" $DBN
grun $N g_jr.sh -6min now "psql|restore|backup" 12
echo "== c3fix done $(ts)"
