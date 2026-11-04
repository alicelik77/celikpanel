for spec in "u120 192.0.3.191 u120.test" "d121 192.0.3.190 d121.test"; do set -- $spec; export CELL=$1 IP=$2 Z=$3
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
echo "== $CELL database servers"; adm GET /api/v1/database-servers --label dbs | python3 -c 'import json,sys; j=json.loads(json.load(sys.stdin)["body"]); [print(" ", x["name"], x["type_name"], x.get("version"), x.get("port"), x.get("status"), x.get("is_default")) for x in j]'
adm GET /api/v1/service/postgresql/clusters --label clusters | sbrief 900
done
