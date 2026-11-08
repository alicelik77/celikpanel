# alpha.127 case 3 (before, on 126): Boston's PostgreSQL history on the joined Ubuntu 24.04 server: PostgreSQL 16
export CELL=${CELL:-r7} Z=${Z:-r7.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191} IP=${IP:-192.0.3.190}
# (distribution) installed, removed with its data kept, PGDG lane enabled, PostgreSQL 18 installed (5433); a PostgreSQL
# database created from the domain's Databases tab request. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
N=backup
exec > >(tee -a $LOGS/C3pre-$CELL-$N.log) 2>&1
clusters() { gshn $N 'pg_lsclusters 2>&1 | cat; dpkg -l "postgresql*" 2>/dev/null | grep -E "^(ii|rc)" | awk "{print \$1, \$2, \$3}"'; }
echo "== versions $(ts)"; ver $N
pgrow
echo "-- install PostgreSQL (distribution version) $(ts)"; installn $N postgresql
clusters
echo "-- remove PostgreSQL (data kept: the removal's default) $(ts)"
admn $N GET "/api/v1/service/removal-impact?service_id=postgresql" --label impact | sbrief 800
admn $N POST /api/v1/service/uninstall '{"service_id":"postgresql"}' --label pg-remove --timeout 900 | sbrief 800
sleep 5; clusters; pgrow
echo "-- enable the PostgreSQL project's repository (PGDG lane) $(ts)"
admn $N GET "/api/v1/repo/lane?service_id=postgresql" --label lane > $T/lane.json; sbrief 900 < $T/lane.json
SHA=$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); p=b.get("plan") or b; print(p.get("sha256",""))' $T/lane.json)
admn $N POST /api/v1/repo/lane "{\"service_id\":\"postgresql\",\"expected_sha256\":\"$SHA\"}" --label lane-enable --timeout 600 | sbrief 800
echo "-- install PostgreSQL 18 $(ts)"; installn $N postgresql postgresql-18
clusters; pgrow
echo "-- a site and a PostgreSQL database on 126 $(ts)"
DID=$(mksite $N pg7.test php); echo "pg7.test id $DID"; echo $DID > $T/pg7.id
PW=$(python3 -c 'import secrets; print(secrets.token_urlsafe(14)+"-Aa1")')
admn $N POST /api/v1/domains/$DID/databases "{\"name\":\"shop126\",\"type\":\"postgresql\",\"password\":\"$PW\"}" --label db-create-126 --timeout 300 | sbrief 900
admn $N GET /api/v1/domains/$DID/databases --label dbs | sbrief 900
gshn $N 'journalctl -u celikpanel-panel --since -5min -o cat --no-pager | grep -iE "postgres|database" | tail -8 | cut -c1-300'
echo "== c3pre done $(ts)"
