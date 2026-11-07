# case 6: database tools. usage: k6db.sh NODE DOMAINIDFILE ENGINE(mysql|postgresql) [PGPORT]. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/hdr.sh
N=$1; DID=$(cat $T/$2); ENG=$3; PGP=${4:-5432}
exec > >(tee -a $LOGS/C6db-$S-$ENG.log) 2>&1
E=my; [ "$ENG" = postgresql ] && E=pg
q() { grun $N g_db.sh $E $1 $(printf %s "$2" | base64 -w0) $PGP; }
raw() { python3 -I $A/raw.py $L $CELL $N "$@"; }
dbs() { admn $N GET /api/v1/domains/$DID/databases --label dbs > $T/dbs.json; python3 -c '
import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); l=b.get("databases") if isinstance(b,dict) else b
for x in l:
    if x.get("type")==sys.argv[2]: print(x["id"], x["name"], x.get("user") or x.get("username") or "")' $T/dbs.json $ENG; }
echo "== 5.0 a $ENG database with rows $(ts)"
if [ -z "$(dbs)" ]; then mkcred db-$N-$ENG; admn $N POST /api/v1/domains/$DID/databases "{\"name\":\"tools\",\"type\":\"$ENG\",\"password\":\"$(pw db-$N-$ENG)\"}" --label dbcreate --timeout 300 | sbrief 500; fi
dbs; read DBI DBN DBU < <(dbs | head -1); echo "database $DBN id $DBI user $DBU"
R="set role \"$DBU\"; "; [ $E = my ] && R=""
q $DBN "${R}create table lab_items(id int primary key, v varchar(40)); insert into lab_items values (1,'one'),(2,'two'),(3,'three'); select count(*) from lab_items;"
P=/api/v1/domains/$DID/databases/$DBI
echo "== 5.1 tools view $(ts)"; admn $N GET $P/tools --label tools | sbrief 800
echo "== 5.2 export + download $(ts)"
admn $N POST $P/export '{}' --label export --timeout 600 > $T/exp.json; sbrief 400 < $T/exp.json
EID=$(python3 -c 'import json,sys; print(json.loads(json.load(open(sys.argv[1]))["body"]).get("export_id",""))' $T/exp.json)
raw GET "$P/export?id=$EID" --out $T/export-$ENG.sql.gz
zcat $T/export-$ENG.sql.gz | grep -ciE "lab_items" | sed 's/^/   lines naming lab_items in the dump: /'
echo "   second download:"; raw GET "$P/export?id=$EID" --show 300
echo "== 5.3 import with a safety copy $(ts)"
if [ $E = my ]; then printf 'CREATE TABLE lab_imported (id INT PRIMARY KEY, note VARCHAR(40));\nINSERT INTO lab_imported VALUES (1,"imported");\nUPDATE lab_items SET v="changed-by-import" WHERE id=1;\n' > $T/imp-ok.sql
else printf 'CREATE TABLE lab_imported (id INT PRIMARY KEY, note VARCHAR(40));\nINSERT INTO lab_imported VALUES (1,%s);\nUPDATE lab_items SET v=%s WHERE id=1;\n' "'imported'" "'changed-by-import'" > $T/imp-ok.sql; fi
up() { local U=$(rid); raw POST "$P/import/upload?id=$U&offset=0" --body-file $1 --show 200 >&2; echo $U; }
U=$(up $T/imp-ok.sql); SZ=$(stat -c %s $T/imp-ok.sql)
raw POST $P/import --json "{\"upload_id\":\"$U\",\"size\":$SZ,\"empty_first\":false}" --show 600
q $DBN "select id, v from lab_items order by id; select note from lab_imported;"
echo "== 5.4 an import that fails half way rolls back $(ts)"
if [ $E = my ]; then printf 'UPDATE lab_items SET v="half-way" WHERE id=2;\nINSERT INTO lab_items VALUES (1,"duplicate key");\n' > $T/imp-bad.sql
else printf 'UPDATE lab_items SET v=%s WHERE id=2;\nINSERT INTO lab_items VALUES (1,%s);\n' "'half-way'" "'duplicate key'" > $T/imp-bad.sql; fi
U=$(up $T/imp-bad.sql); SZ=$(stat -c %s $T/imp-bad.sql)
raw POST $P/import --json "{\"upload_id\":\"$U\",\"size\":$SZ,\"empty_first\":false}" --show 900
q $DBN "select id, v from lab_items order by id;"
echo "   a file with a client command:"; printf 'select 1;\n\\! id\n' > $T/imp-cmd.sql; U=$(up $T/imp-cmd.sql); SZ=$(stat -c %s $T/imp-cmd.sql)
raw POST $P/import --json "{\"upload_id\":\"$U\",\"size\":$SZ,\"empty_first\":false}" --show 500
echo "== 5.5 copy $(ts)"
mkcred dbcopy-$N-$ENG
admn $N POST $P/copy "{\"name\":\"copy1\",\"password\":\"$(pw dbcopy-$N-$ENG)\"}" --label copy --timeout 600 | sbrief 500
dbs; CN=$(dbs | awk '$2 ~ /copy1/ {print $2}' | head -1); [ -n "$CN" ] && q $CN "select count(*) from lab_items; select note from lab_imported;"
echo "== 5.6 check (and repair for MariaDB) $(ts)"
admn $N POST $P/check '{}' --label check | sbrief 600
[ $E = my ] && admn $N POST $P/check '{"repair":true}' --label repair | sbrief 600
[ $E = pg ] && admn $N POST $P/check '{"repair":true}' --label repair-pg | sbrief 400
echo "== c5db $N $ENG done $(ts)"
