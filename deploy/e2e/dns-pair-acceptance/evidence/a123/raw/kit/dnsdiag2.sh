export CELL=${CELL:-u22} IP=${IP:-192.0.3.191} Z=${Z:-u22.test}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a123/kit/lib.sh
exec > >(tee -a $LOGS/dnsdiag-$CELL.log) 2>&1
echo "== $(ts) setup operation (API read)"
b1 api $CELL main GET /api/v1/setup/operation --label diag --limit 200000 2>/dev/null | python3 -c '
import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); op=b.get("operation",b)
print(d["status"], op.get("status"), op.get("phase"))
for s in (op.get("steps") or [])[:3]: print(json.dumps(s)[:3000])
for k in ("error","guidance","problem","next_action"):
  if op.get(k): print(k, json.dumps(op.get(k))[:2500])'
echo "== guest journal"; GQT=90 gq g_dnsdiag2.sh
