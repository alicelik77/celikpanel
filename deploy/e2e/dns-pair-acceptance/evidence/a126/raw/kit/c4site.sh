# case 4: the customer on the plan saved on 125 ("all software") creates a PHP site on 126. Lab only.
export CELL=p6 Z=p6.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a126lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh
exec > >(tee -a $LOGS/C4-$CELL.log) 2>&1
echo "== the plan still works: c6plan (on all-software-125) creates plan6.test with PHP $(ts)"
SUB=$(as c6plan GET /api/v1/my/subscription | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(b["subscriptions"][0]["id"])'); echo "subscription $SUB"
as c6plan POST /api/v1/domains/create "{\"domain\":\"plan6.test\",\"project_type\":\"php\",\"ssl_type\":\"none\",\"subscription_id\":$SUB}" | sbrief 400
sleep 10; as c6plan GET /api/v1/domains | python3 -c 'import json,sys; d=json.load(sys.stdin); print("  customer domains", d["status"], d["body"][:300])'
