# alpha.119 A item 2: Go from Components (admin), customer refused. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R7go-$CELL.log) 2>&1
row() { adm GET /api/v1/managed-services --label ms | python3 -c '
import json,sys
d=json.load(sys.stdin); b=json.loads(d["body"]); rows=b if isinstance(b,list) else (b.get("services") or b.get("items") or [])
for r in rows:
    if r.get("id") in sys.argv[1:]: print("  row", json.dumps({k:r.get(k) for k in ("id","is_installed","status","kind","category","admin_only","package_version","package_version_short","packages")})[:600])' "$@"; }
echo "== R7go $(ts)"; row go
R=$(rid); svcstart "{\"service_id\":\"go\",\"request_id\":\"$R\"}" /api/v1/service/install && svcwait $R 900
row go; gsh1 'go version; dpkg -l golang-go | tail -1'
echo "-- customer: install Go (expect refusal)"; as cust119a POST /api/v1/service/install "{\"service_id\":\"go\",\"request_id\":\"$(rid)\"}" | sbrief 300
echo "-- customer: components listing (expect refusal)"; as cust119a GET /api/v1/managed-services | sbrief 200
echo "== R7go done $(ts)"
