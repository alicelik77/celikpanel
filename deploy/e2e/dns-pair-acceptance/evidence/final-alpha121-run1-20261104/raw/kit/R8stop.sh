# alpha.119 run 2 item 4: stop of a never-deployed program -> 200 not_deployed, no raw systemd text. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R8stop-$CELL.log) 2>&1
DID=$(cat $T/id-prog119.test)
echo "== R8stop $(ts)"
adm GET /api/v1/admin/runtime-apps --label rta > $T/rta.json
DG=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print([k for k in b["kinds"] if k["kind"]=="program"][0]["risk_digest"])' $T/rta.json)
adm POST /api/v1/admin/runtime-apps/program/offer "{\"risk_digest\":\"$DG\"}" --label offer-prog | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["status"])'
as cust119a POST /api/v1/domains/$DID/runtime-apps/create '{"kind":"program","entry":"goprobe","args":["-serve","-port={port}"],"health_path":"/"}' | sbrief 300
echo "-- stop (never deployed)"; as cust119a POST /api/v1/domains/$DID/runtime-apps/control '{"action":"stop"}' | sbrief 600
echo "-- start (never deployed)"; as cust119a POST /api/v1/domains/$DID/runtime-apps/control '{"action":"start"}' | sbrief 600
as cust119a POST /api/v1/domains/$DID/runtime-apps/remove "{\"operation_id\":\"rm-stop-$(rid | cut -c1-12)\",\"remove_files\":false,\"confirm_domain\":\"prog119.test\"}" | sbrief 200
echo "== R8stop done $(ts)"
