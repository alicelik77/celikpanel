pkill -f Bupd.sh; sleep 1
CELL=u117 IP=192.0.3.191 Z=u117.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
date -u +%T
adm GET "/api/v1/service/operation?active=1" --label active | sbrief 800
adm GET /api/v1/setup/operation --label op | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); op=b.get("operation",b); print("setup", op.get("status"), op.get("phase"))'
adm GET /api/v1/panel/update/check --label chk | sbrief 300
