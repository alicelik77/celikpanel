# alpha.127 case 3 (before, on 126), continued: PostgreSQL 18 installed as the Components window does it (the preview's
# packages confirmed), then a PostgreSQL database from the domain's Databases tab request. Lab only.
export CELL=${CELL:-r7} Z=${Z:-r7.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191} IP=${IP:-192.0.3.190}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
N=backup
exec > >(tee -a $LOGS/C3pre-$CELL-$N.log) 2>&1
clusters() { gshn $N 'pg_lsclusters 2>&1 | cat; dpkg -l "postgresql*" 2>/dev/null | grep -E "^(ii|rc)" | awk "{print \$1, \$2, \$3}"'; }
echo "-- install preview for PostgreSQL 18 $(ts)"
admn $N GET "/api/v1/service/install-preview?step=postgresql:postgresql-18" --label prev18 --timeout 150 > $T/p18.json; sbrief 700 < $T/p18.json
CONF=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print(json.dumps(b.get("installs") or []))' $T/p18.json)
echo "confirmed installs: $CONF"
R=$(rid); svcstartn $N "{\"service_id\":\"postgresql\",\"package\":\"postgresql-18\",\"check_installs\":true,\"confirmed_installs\":$CONF,\"request_id\":\"$R\"}" /api/v1/service/install; svcwaitn $N $R 1200
clusters; pgrow
DID=$(cat $T/pg7.id); echo "pg7.test id $DID"
PW=$(python3 -c 'import secrets; print(secrets.token_urlsafe(14)+"-Aa1")')
echo "-- create a PostgreSQL database on 126 (the Databases tab's request) $(ts)"
admn $N POST /api/v1/domains/$DID/databases "{\"name\":\"shop126\",\"type\":\"postgresql\",\"password\":\"$PW\"}" --label db-create-126 --timeout 300 | sbrief 900
admn $N GET /api/v1/domains/$DID/databases --label dbs | sbrief 900
gshn $N 'journalctl -u celikpanel-panel -u celikpanel-agent --since -3min -o cat --no-pager | grep -iE "postgres|database|5432|5433" | tail -8 | cut -c1-300'
echo "== c3pre2 done $(ts)"
