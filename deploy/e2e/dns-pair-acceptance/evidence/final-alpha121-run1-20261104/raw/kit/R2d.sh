# run 2, second fresh Debian 13 (a118d): D1 recheck with apt's own (fcntl) dpkg locks held, then the WordPress refusal. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab
CELL=a118d IP=192.0.3.193 Z=d118.test SHORT=berlin MEM=3072 HOLDLOCK=1 bash $A/singleA.sh > $A/singleD.out 2>&1
export CELL=a118d IP=192.0.3.193 Z=d118.test
source $A/lib.sh
exec > >(tee -a $LOGS/R2d-$CELL.log) 2>&1
echo "== WordPress refusal on a fresh server $(ts)"
mkcred custd
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"customer"}, open(sys.argv[2],"w"))' $T/cred-custd.json $T/u.json
adm POST /api/v1/users @$T/u.json --label create-custd | sbrief 200; rm -f $T/u.json
SUB=$(sub_of custd); adm POST /api/v1/subscriptions/$SUB/entitlements '{"product_id":"app_installer"}' --label ent | sbrief 200
W=$(newdomain custd $SUB wpd118.test); echo "wpd118.test id=$W"
sleep 5
as custd GET /api/v1/domains/$W/apps | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("  customer view", x.get("id"), json.dumps(x.get("requirements"))[:800]) for x in b.get("apps",[]) if x.get("id")=="wordpress"]'
adm GET /api/v1/domains/$W/apps --label admin-apps | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("  admin view", x.get("id"), json.dumps(x.get("requirements"))[:800]) for x in b.get("apps",[]) if x.get("id")=="wordpress"]'
AS_TIMEOUT=600 as custd POST /api/v1/domains/$W/apps/install '{"app":"wordpress"}' | sbrief 900
lc code - wpd118.test ""
echo "== R2d done $(ts)"
