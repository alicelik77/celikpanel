export CELL=a118 IP=192.0.3.190 Z=f118.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/r2-n8nfail.log) 2>&1
echo "== n8n install log (API) $(ts)"
as cust118n GET "/api/v1/domains/$(cat $T/nid)/service-apps/logs?lines=60&install=1" | python3 -c 'import json,sys; d=json.load(sys.stdin); b=d["body"]
try:
  j=json.loads(b); L=j.get("lines") or j.get("log") or j
  print(d["status"]); print("\n".join(L) if isinstance(L,list) else json.dumps(L)[:8000])
except Exception: print(d["status"], b[:8000])'
