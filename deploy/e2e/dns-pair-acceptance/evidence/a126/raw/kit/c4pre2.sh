# case 4 prep (125): the reseller's "all software but one" plan, without the items outside its own rights. Lab only.
export CELL=p6 Z=p6.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a126lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh
exec > >(tee -a $LOGS/C1pre-$CELL.log) 2>&1
echo "== case 4 prep, plan 2 without the items outside the reseller's rights $(ts)"
python3 - $T/plan-minus.json <<'P'
import json,sys
p=json.load(open(sys.argv[1])); out={"app:drupal","app:joomla","app:mediawiki","app:n8n","app:nextcloud","app:open-webui","app:prestashop","runtime:program"}
p["rights"]["software"]=[i for i in p["rights"]["software"] if i not in out]; json.dump(p,open(sys.argv[1],"w")); print("software", len(p["rights"]["software"]), p["rights"]["software"])
P
as res6 POST /api/v1/plans @$T/plan-minus.json | sbrief 400
as res6 GET /api/v1/plans > $T/plans125.json; python3 $A/softcount.py $T/plans125.json "125 reseller plans"
grun main g_rights.sh
