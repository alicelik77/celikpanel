source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/env.sh
exec > >(tee -a $LOGS/r2-observe.log) 2>&1
echo "== read-only observation after the stop $(ts)"
echo "-- a118d setup (D1 recheck)"; opnow a118d main | head -2 | cut -c1-400
lab ssh a118d main -- "sudo -n cat /var/lib/celikpanel-agent-private/service-mutations.json" | python3 -c 'import json,sys; d=json.load(sys.stdin); print("jobs", len(d.get("jobs",{})), [(j["kind"],j["target"],j["status"]) for j in d["jobs"].values()])'
cat $LOGS/d1watch-a118d.log | cut -c1-300 | head -24
echo "-- a118 n8n state"
CELL=a118 IP=192.0.3.190 Z=f118.test source $A/lib.sh
as cust118n GET /api/v1/domains/$(cat $T/nid)/service-apps | python3 $A/sastat.py
