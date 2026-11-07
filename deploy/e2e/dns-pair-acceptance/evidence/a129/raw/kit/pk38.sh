source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/env.sh
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source $A/lib.sh; source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/C4ban-r9.log) 2>&1
echo "== 133 retry $(ts): the 192.0.3.2 rule now (5 min after the 502):"
as cust1 GET /api/v1/database-remote/databases/$(cat $T/rdb-mysql.id) | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("   rule", r.get("address"), r.get("status")) for r in b.get("rules") or []]'
grun main g_fwr.sh
