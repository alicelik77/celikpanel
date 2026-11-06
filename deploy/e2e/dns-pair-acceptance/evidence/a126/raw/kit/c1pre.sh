# alpha.126 case 1 (part 1, pair at the published alpha.125): the owner's incident reproduced. A static site on
# the joined server (zone published on the main), deleted on the joined server; the deletion stays pending.
# Case 4 preparation: a reseller and two plans saved on 125. Lab only.
export CELL=${CELL:-p6} Z=${Z:-p6.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191}; export IP=${IP:-$MIP}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a126lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh
exec > >(tee -a $LOGS/C1pre-$CELL.log) 2>&1
echo "== versions $(ts)"; ver main; ver backup
echo "== backup: named's control sockets and the DNS check tool at 125 $(ts)"; grun backup g_dnstool.sh
grun main g_dnstool.sh | sed -n '1,3p'
echo "== case 4 prep: reseller res6 and two plans saved on 125 $(ts)"
mkcred res6
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"reseller"}, open(sys.argv[2],"w"))' $T/cred-res6.json $T/u.json
adm POST /api/v1/users @$T/u.json --label create-res6 | sbrief 300; rm -f $T/u.json
as res6 GET /api/v1/rights/catalog > $T/cat125.json; python3 $A/softcount.py $T/cat125.json "125 reseller catalog"
python3 - $T/cat125.json $T/plan-all.json $T/plan-minus.json <<'P'
import json,sys
sys.path.insert(0, sys.argv[0].rsplit('/',1)[0])
d=json.load(open(sys.argv[1])); b=json.loads(d["body"])
def soft(x, out):
    if isinstance(x,dict):
        for k,v in x.items():
            if k in ("software","offerings","installed") and isinstance(v,list):
                for e in v:
                    i = e.get("id") if isinstance(e,dict) else e
                    if isinstance(i,str): out.append(i)
            soft(v,out)
    elif isinstance(x,list):
        for v in x: soft(v,out)
ids=[]; soft(b,ids); ids=sorted(set(ids))
base={"max_domains":5,"max_databases":5,"max_email_accounts":5,"disk_quota_mb":1024,"bandwidth_quota_mb":10240}
json.dump(dict(base,name="all-software-125",rights={"all_permissions":True,"all_software":True,"permissions":[],"software":[]}),open(sys.argv[2],"w"))
minus=[i for i in ids if not i.startswith("component:php")][:] or ids
take=[i for i in ids if i.startswith("component:php")][:1]
json.dump(dict(base,name="all-but-one-125",rights={"all_permissions":True,"all_software":False,"permissions":[],"software":[i for i in ids if i not in take]}),open(sys.argv[3],"w"))
print("software ids", len(ids), ids[:40], "taken out:", take)
P
as res6 POST /api/v1/plans @$T/plan-all.json | sbrief 400
as res6 POST /api/v1/plans @$T/plan-minus.json | sbrief 400
as res6 GET /api/v1/plans > $T/plans125.json; python3 $A/softcount.py $T/plans125.json "125 reseller plans"
grun main g_rights.sh
echo "== the incident site: inc6.test on the joined server (static), zone published on the main $(ts)"
IID=$(mkstatic backup inc6.test); echo "inc6.test id on backup: $IID"; echo $IID > $T/inc-id
sleep 20; digz inc6.test
admn backup GET /api/v1/domains --label domains-backup | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print("  backup domains:", json.dumps(b)[:400])'
grun backup g_dnstool.sh inc6.test | sed -n '/site include/,$p'
echo "== a usage round so the traffic check has measured inc6.test $(ts)"; waitround
admn backup GET /api/v1/usage/status --label usage-before | python3 $A/usagesum.py
echo "== DELETE inc6.test on the joined server (once, as the owner presses Delete) $(ts)"
admn backup DELETE /api/v1/domains/$IID --label delete-inc6 --timeout 600 | sbrief 900
EVERY=10 delwatch backup $IID 420
echo "== pending? journal on both servers $(ts)"
for n in main backup; do echo "-- $n"; grun $n g_peerlog.sh -10min "" 14; done
digz inc6.test
echo "== Users page traffic warning on the joined server while pending $(ts)"; waitround
admn backup GET /api/v1/usage/status --label usage-pending | python3 $A/usagesum.py
grun backup g_dnstool.sh inc6.test | sed -n '/site include/,$p'
echo "== c1pre done $(ts)"
