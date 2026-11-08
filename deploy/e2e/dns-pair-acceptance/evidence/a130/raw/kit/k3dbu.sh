# alpha.129 case 3: database users on one engine of the main (site1.$Z): password change, read-only and read-write
# users, removal keeps the database, DATABASE_NAME_TAKEN for a name made outside the panel. usage: k3dbu.sh mysql|postgresql. Lab only.
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
ENG=$1; E=my; [ $ENG = postgresql ] && E=pg
exec > >(tee -a $LOGS/C3dbu-$CELL-$ENG.log) 2>&1
S1=$(cat $T/site1.id)
q() { grun main g_db.sh $E $1 $(printf %s "$2" | base64 -w0); }               # as the engine's administrator
qu() { pw $3 | timeout 120 python3 -I $L/lab.py ssh $CELL main -- "sudo -n bash /root/g_dbu.sh $E $1 $2 $(printf %s "$4" | base64 -w0)"; }  # as a user: USER DB CRED SQL
gput g_dbu.sh main; gput g_db.sh main
dbjson() { admn main GET /api/v1/domains/$S1/databases --label dbs > $T/dbs.json; }
findid() { python3 - "$T/dbs.json" "$1" <<'P'
import json,sys
d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); want=sys.argv[2]
for x in (b.get("databases") if isinstance(b,dict) else b) or []:
    for u in x.get("users") or []:
        if u.get("username")==want: print(u["id"]); sys.exit()
def walk(x):
    if isinstance(x,dict):
        if want == x.get("name") and isinstance(x.get("id"),int): print(x["id"]); sys.exit()
        for v in x.values(): walk(v)
    if isinstance(x,list):
        for v in x: walk(v)
walk(b)
P
}
echo "== 3.0 a $ENG database of site1 $(ts)"
mkcred dbu-$ENG
admn main POST /api/v1/domains/$S1/databases "{\"name\":\"${DBS:-u}$E\",\"type\":\"$ENG\",\"password\":\"$(pw dbu-$ENG)\"}" --label dbcreate --timeout 300 | sbrief 400
dbjson; DBN=site1_r9_test_${DBS:-u}$E; DBID=$(findid $DBN); OWNER=${DBN}_user; [ $E = pg ] && OWNER=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); l=b.get("databases") if isinstance(b,dict) else b; print([x for x in l if x["name"]==sys.argv[2]][0].get("user",""))' $T/dbs.json $DBN)
UID0=$(findid $OWNER); echo "database $DBN id $DBID owner user $OWNER id $UID0"
python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); l=b.get("databases") if isinstance(b,dict) else b; [print("  ", json.dumps(x)[:600]) for x in l if x["name"]==sys.argv[2]]' $T/dbs.json $DBN
echo "-- the owner user makes a table"; qu $OWNER $DBN dbu-$ENG "create table t1(id int primary key, v varchar(20)); insert into t1 values (1,'one'); select count(*) from t1;"
P=/api/v1/domains/$S1/databases/$DBID
echo "== 3.1 password change of the owner user $(ts)"
cp $T/cred-dbu-$ENG.json $T/cred-dbu-$ENG-old.json; rm -f $T/cred-dbu-$ENG.json; mkcred dbu-$ENG
python3 -c 'import json,os,sys; p=json.load(open(sys.argv[1]))["password"]; fd=os.open(sys.argv[2],os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600); os.write(fd,json.dumps({"password":p}).encode())' $T/cred-dbu-$ENG.json $T/pwbody.json
admn main POST $P/users/$UID0/password @$T/pwbody.json --label dbu-pass --timeout 300 | sbrief 900; rm -f $T/pwbody.json
echo "   old password:"; qu $OWNER $DBN dbu-$ENG-old "select count(*) from t1;"
echo "   new password:"; qu $OWNER $DBN dbu-$ENG "select count(*) from t1;"
echo "   a weak password is refused:"; admn main POST $P/users/$UID0/password '{"password":"[REDACTED]"}' --label dbu-weak | sbrief 400
echo "== 3.2 read-only and read-write users $(ts)"
mkcred ro-$ENG; mkcred rw-$ENG
admn main POST $P/users "{\"name\":\"ro\",\"password\":\"$(pw ro-$ENG)\",\"access\":\"read\"}" --label dbu-ro --timeout 300 | sbrief 500
admn main POST $P/users "{\"name\":\"rw\",\"password\":\"$(pw rw-$ENG)\",\"access\":\"write\"}" --label dbu-rw --timeout 300 | sbrief 500
dbjson; RO=${DBN}_ro; RW=${DBN}_rw; ROID=$(findid $RO); RWID=$(findid $RW); echo "ro $RO id $ROID, rw $RW id $RWID"
[ -z "$ROID" ] && { python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["body"][:2500])' $T/dbs.json; }
echo "-- rw makes a table later and writes"; qu $RW $DBN rw-$ENG "create table t2(id int primary key); insert into t2 values (7); insert into t1 values (2,'rw'); select count(*) from t2;"
echo "-- ro reads both, cannot write"; qu $RO $DBN ro-$ENG "select (select count(*) from t1), (select count(*) from t2);"
qu $RO $DBN ro-$ENG "insert into t1 values (3,'ro');"
qu $RO $DBN ro-$ENG "create table t3(id int);"
echo "-- the owner makes a table after ro exists; ro reads it"; qu $OWNER $DBN dbu-$ENG "create table t4(id int); insert into t4 values (4);"
qu $RO $DBN ro-$ENG "select count(*) from t4;"
echo "== 3.3 remove ro and rw: the database stays $(ts)"
admn main DELETE $P/users/$ROID --label dbu-ro-del --timeout 300 | sbrief 400
admn main DELETE $P/users/$RWID --label dbu-rw-del --timeout 300 | sbrief 400
qu $OWNER $DBN dbu-$ENG "select (select count(*) from t1), (select count(*) from t2), (select count(*) from t4);"
echo "   ro after removal:"; qu $RO $DBN ro-$ENG "select 1;"
echo "   removing the database's own user is refused:"; admn main DELETE $P/users/$UID0 --label dbu-own-del | sbrief 400
echo "== 3.4 a name made outside the panel $(ts)"
if [ $E = my ]; then q mysql "create database site1_r9_test_taken; create user 'site1_r9_test_taken2_user'@'localhost' identified by 'Lab-outside-1x';"
else q postgres "create role site1_r9_test_${DBS:-u}taken2_user login password 'Lab-outside-1x';"; q postgres "create database site1_r9_test_${DBS:-u}taken;"; fi
mkcred tk-$ENG
admn main POST /api/v1/domains/$S1/databases "{\"name\":\"${DBS:-u}taken\",\"type\":\"$ENG\",\"password\":\"$(pw tk-$ENG)\"}" --label db-taken --timeout 300 | sbrief 600
admn main POST /api/v1/domains/$S1/databases "{\"name\":\"${DBS:-u}taken2\",\"type\":\"$ENG\",\"password\":\"$(pw tk-$ENG)\"}" --label db-taken2 --timeout 300 | sbrief 600
if [ $E = my ]; then q mysql "show databases like 'site1_r9_test_taken%'; select user,host from mysql.user where user like 'site1_r9_test_taken%';"
else q postgres "select datname from pg_database where datname like 'site1_r9_test_%taken%'; select rolname from pg_roles where rolname like 'site1_r9_test_%taken%';"; fi
echo "== k3dbu $ENG done $(ts)"
