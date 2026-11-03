pkill -f Bupd.sh; sleep 1
export CELL=u117 IP=192.0.3.191 Z=u117.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/Bbusy-$CELL.log) 2>&1
echo "== $(ts) why PANEL_UPDATE_BUSY"; adm GET "/api/v1/service/operation?active=1" --label active | sbrief 600
lab ssh u117 main -- "sudo -n cat /var/lib/celikpanel-agent-private/service-mutations.json" | python3 -c 'import json,sys; d=json.load(sys.stdin); print("active", d.get("active_request_id"), [(j["kind"],j["target"],j["status"],j.get("updated_at")) for j in d["jobs"].values()][-4:])'
