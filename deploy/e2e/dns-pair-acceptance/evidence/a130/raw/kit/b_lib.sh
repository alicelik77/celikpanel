# B lane helpers (sourced). Lab only.
export CELL=s0 Z=s0.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.191
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
N=backup
audit() { admn $N GET "/api/v1/audit-logs?limit=${2:-60}" --label audit > $T/audit-b.json; python3 -c '
import json,sys,re; b=json.loads(json.load(open(sys.argv[1]))["body"]); l=b.get("logs") or b.get("items") or b.get("entries") or (b if isinstance(b,list) else [])
pat=sys.argv[2]
for e in l:
    a=e.get("action","")
    if re.search(pat,a): print("  audit", e.get("created_at") or e.get("timestamp"), a[:220], json.dumps(e.get("details"),ensure_ascii=False)[:300])' $T/audit-b.json "$1"; }
