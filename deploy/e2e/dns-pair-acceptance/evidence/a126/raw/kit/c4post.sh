# alpha.126 case 4 after the update: reseller catalog, the plans saved on 125, a plan still works. Lab only.
export CELL=${CELL:-p6} Z=${Z:-p6.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191}; export IP=${IP:-$MIP}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a126lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh
exec > >(tee -a $LOGS/C4-$CELL.log) 2>&1
echo "== versions $(ts)"; ver main
echo "-- 125 (before the update), kept from c1pre:"; python3 $A/softcount.py $T/cat125.json "125 reseller catalog"; python3 $A/softcount.py $T/plans125.json "125 plans"
as res6 GET /api/v1/rights/catalog > $T/cat126.json; python3 $A/softcount.py $T/cat126.json "126 reseller catalog"
as res6 GET /api/v1/plans > $T/plans126.json; python3 $A/softcount.py $T/plans126.json "126 plans"
echo "-- saved rows in the database (read-only, kept as they were)"
grun main g_rights.sh
PID=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print([p["id"] for p in b["plans"] if p["name"]=="all-software-125"][0])' $T/plans126.json); echo "plan all-software-125 id $PID"
echo "== the plan still works: res6 creates customer c6plan on it; the customer creates a PHP site $(ts)"
mkcred c6plan
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"customer","plan_id":int(sys.argv[3])}, open(sys.argv[2],"w"))' $T/cred-c6plan.json $T/u.json $PID
as res6 POST /api/v1/users @$T/u.json | sbrief 400; rm -f $T/u.json
as c6plan GET /api/v1/rights/catalog > $T/catc6.json; python3 $A/softcount.py $T/catc6.json "customer on all-software-125"
as c6plan GET /api/v1/my/subscription | sbrief 600
SUB=$(as c6plan GET /api/v1/my/subscription | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); s=b.get("subscription") or b; print(s.get("id") or s.get("subscription_id"))')
infra delegate plan6.test ns1.$Z $MIP ns2.$Z $BIP > /dev/null
as c6plan POST /api/v1/domains/create "{\"domain\":\"plan6.test\",\"project_type\":\"php\",\"ssl_type\":\"none\",\"subscription_id\":$SUB}" | sbrief 400
echo "== c4post done $(ts)"
