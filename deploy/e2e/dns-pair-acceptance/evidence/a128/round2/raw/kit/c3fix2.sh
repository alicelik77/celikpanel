# alpha.127 case 3 fix check 2: joined server updated (panel Update API) to a lab 129 label of release/alpha127
# 32124ee20 (restore in one transaction). A table of another role blocks a restore without changing anything; without
# it the restore works; a backup made on 127 restores too. Lab only.
export CELL=${CELL:-r7} Z=${Z:-r7.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191} IP=${IP:-192.0.3.191}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
N=backup
psq() { grun $N g_pg.sh sql $1 ${3:-postgres} $(printf %s "$2" | base64 -w0); }
bash $A/portalF.sh latest v0.1.0-alpha.129
CELL=r7 IP=192.0.3.191 bash $A/upd.sh backup > $A/c3fix2-upd.out 2>&1
exec > >(tee -a $LOGS/C3fix-$CELL-$N.log) 2>&1
echo "== update 128 -> 129 (lab label of 32124ee20)"; grep -E "^  [0-9]+s |schema_version|^\(end\)|unverified" $A/c3fix2-upd.out | cut -c1-250
DID=$(cat $T/pg7.id); DBN=pg7_test_shop127
state() { psq 5433 "select string_agg(v, ',') from lab_rows; select tablename||' '||tableowner from pg_tables where schemaname='public' order by 1;" $DBN; }
newbk() { admn $N POST /api/v1/domains/$DID/backups '{"type":"database","database_id":1}' --label backup-db --timeout 600 | sbrief 300 >&2
  admn $N GET /api/v1/domains/$DID/backups --label backups | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=[x for x in b["backups"] if x.get("type")=="database" and x.get("origin")=="manual"]; print(sorted(l,key=lambda x:x["created_at"])[-1]["name"])'; }
restore() { admn $N POST /api/v1/domains/$DID/backups/restore "{\"backup_name\":\"$1\"}" --label restore-db --timeout 900 | sbrief 500; }
echo "== 1. a table of another role (lab: made by postgres) in the database and in the backup $(ts)"
psq 5433 "update lab_rows set v='before-backup-1'; create table lab_other(x int);" $DBN
B1=$(newbk); echo "backup $B1"
psq 5433 "update lab_rows set v='changed-1';" $DBN
restore $B1; echo "-- after (expected: unchanged, changed-1, both tables there)"; state
echo "== 2. without the other role's table $(ts)"
psq 5433 "drop table lab_other; update lab_rows set v='before-backup-2';" $DBN
B2=$(newbk); echo "backup $B2"
psq 5433 "update lab_rows set v='changed-2'; insert into lab_rows values ('extra-2');" $DBN
restore $B2; echo "-- after (expected: before-backup-2 only, owner kept)"; state
echo "== 3. the backup made on 127 (before any fix) $(ts)"
B0=$(admn $N GET /api/v1/domains/$DID/backups --label backups | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=[x for x in b["backups"] if x.get("type")=="database" and x.get("origin")=="manual"]; print(sorted(l,key=lambda x:x["created_at"])[0]["name"])')
echo "backup $B0"; restore $B0; state
grun $N g_jr.sh -8min now "restore failed|already exists" 8
echo "== c3fix2 done $(ts)"
