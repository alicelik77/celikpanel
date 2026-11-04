# run 4 item 5: PostgreSQL on its own (distribution version), phpPgAdmin, removal keeping the tool -> orphan notice. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R4pg-$CELL.log) 2>&1
row() { adm GET /api/v1/managed-services --label ms | python3 -c '
import json,sys
d=json.load(sys.stdin); b=json.loads(d["body"]); rows=b if isinstance(b,list) else (b.get("services") or b.get("items") or [])
for r in rows:
    if r.get("id") in sys.argv[1:]:
        print("  row", json.dumps({k:r.get(k) for k in ("id","is_installed","status","kind","not_offered","conflict_with","requires_missing","needs_missing","versions","packages")})[:700])' "$@"; }
echo "== R4pg $(ts)"
row postgresql phppgadmin
adm GET "/api/v1/service/candidate?id=postgresql" --label cand | sbrief 300
adm GET "/api/v1/repo?service_id=postgresql" --label repo | sbrief 400
echo "-- install PostgreSQL (distribution version, no package)"; R=$(rid); svcstart "{\"service_id\":\"postgresql\",\"request_id\":\"$R\"}" /api/v1/service/install && svcwait $R 900
adm GET "/api/v1/service/operation?request_id=$R" --label pg-op | sbrief 800
echo "-- install phpPgAdmin"; R=$(rid); svcstart "{\"service_id\":\"phppgadmin\",\"request_id\":\"$R\"}" /api/v1/service/install && svcwait $R 900
adm GET "/api/v1/service/operation?request_id=$R" --label ppa-op | sbrief 800
row postgresql phppgadmin
echo "-- removal review for PostgreSQL"; adm GET "/api/v1/service/removal-impact?service_id=postgresql" --label impact | sbrief 1200
echo "-- remove PostgreSQL, keep phpPgAdmin (unticked)"; adm POST /api/v1/service/uninstall '{"service_id":"postgresql"}' --label pg-remove --timeout 900 | sbrief 800
row postgresql phppgadmin
echo "-- remove phpPgAdmin"; adm GET "/api/v1/service/removal-impact?service_id=phppgadmin" --label impact2 | sbrief 600
adm POST /api/v1/service/uninstall '{"service_id":"phppgadmin"}' --label ppa-remove --timeout 900 | sbrief 800
row postgresql phppgadmin
gsh1 'dpkg -l "postgresql*" phppgadmin 2>/dev/null | grep -E "^(ii|rc)" | awk "{print \$1, \$2, \$3}"'
echo "== R4pg done $(ts)"
