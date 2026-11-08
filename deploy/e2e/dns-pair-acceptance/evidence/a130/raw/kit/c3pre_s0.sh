# alpha.130 round, before (published alpha.129): Boston's PostgreSQL history on the joined Ubuntu 24.04 server, through
# the panel as on Boston: PostgreSQL 16 (distribution) installed, removed with its data kept, PGDG lane enabled,
# PostgreSQL 18 installed as the Components window does it (lands on 5433; 16/main stays down,binaries_missing). Lab only.
export CELL=s0 Z=s0.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
N=backup
exec > >(tee -a $LOGS/C3pre-$CELL-$N.log) 2>&1
clusters() { gshn $N 'pg_lsclusters 2>&1 | cat; dpkg -l "postgresql*" 2>/dev/null | grep -E "^(ii|rc)" | awk "{print \$1, \$2, \$3}"'; }
echo "== versions $(ts)"; ver $N
pgrow
echo "-- install PostgreSQL (distribution version) $(ts)"; installc $N postgresql
clusters
echo "-- remove PostgreSQL (data kept: the removal's default) $(ts)"
admn $N GET "/api/v1/service/removal-impact?service_id=postgresql" --label impact | sbrief 600
admn $N POST /api/v1/service/uninstall '{"service_id":"postgresql"}' --label pg-remove --timeout 900 | sbrief 600
sleep 5; clusters; pgrow
echo "-- enable the PostgreSQL project's repository (PGDG lane) $(ts)"
admn $N GET "/api/v1/repo/lane?service_id=postgresql" --label lane > $T/lane.json; sbrief 400 < $T/lane.json
SHA=$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); p=b.get("plan") or b; print(p.get("sha256",""))' $T/lane.json)
admn $N POST /api/v1/repo/lane "{\"service_id\":\"postgresql\",\"expected_sha256\":\"$SHA\"}" --label lane-enable --timeout 600 | sbrief 500
echo "-- install PostgreSQL 18 (Components window: preview's packages confirmed) $(ts)"; installc $N postgresql postgresql-18
clusters; pgrow
echo "-- a site and a PostgreSQL database on 129 $(ts)"
DID=$(mksite $N pgb.s0.test php); echo "pgb id $DID"; echo $DID > $T/pgb.id
mkcred bdbp
admn $N POST /api/v1/domains/$DID/databases "{\"name\":\"shopb\",\"type\":\"postgresql\",\"password\":\"$(pw bdbp)\"}" --label db-create-129 --timeout 300 | sbrief 700
admn $N GET /api/v1/domains/$DID/databases --label dbs | sbrief 700
grun $N g_snap.sh pre129pg >/dev/null; echo "snapshot pre129pg taken"
echo "== c3pre done $(ts)"
