# alpha.119 B items 6-7: after the update, n8n installed on 118 needs the new sandbox; apply it. Lab only.
export CELL=u118 IP=192.0.3.191 Z=u118.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/B7ref-$CELL.log) 2>&1
NID=$(cat $T/nid)
sav() { adm GET /api/v1/admin/service-apps --label sa | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print("  admin sandbox:", json.dumps(b.get("sandbox"))[:700])'; }
dom() { as custb GET /api/v1/domains/$NID/service-apps | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); i=b.get("install") or {}; print("  domain install:", json.dumps({k:i.get(k) for k in ("state","active","serving","unit","needs_sandbox","sandbox_generation")}))'; }
echo "== B7 sandbox refresh $(ts)"
sav; dom; GQT=90 gq g_n8n7.sh celiksvc-1 | head -16
N=$(adm GET /api/v1/admin/service-apps --label sa | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print((b.get("sandbox") or {}).get("needs_refresh",0))')
echo "-- Apply the new sandbox (expected=$N)"; adm POST /api/v1/admin/service-apps/sandbox-refresh "{\"request_id\":\"lab-refresh-$(rid | cut -c1-12)\",\"expected\":$N}" --label refresh --timeout 600 | sbrief 800
sleep 30; sav; dom; GQT=90 gq g_n8n7.sh celiksvc-1 | head -24
lc https - n8nb118.test ""
echo "-- owner login"; nup labchk.sh >/dev/null; echo "$(pw n8nowner)Zz9" | timeout 60 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/labchk.sh n8nlogin $IP n8nb118.test"
echo "== B7ref done $(ts)"
