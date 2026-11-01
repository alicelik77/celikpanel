#!/bin/bash
# delwatch.sh CELL NODE ID DOMAIN SECONDS : poll deletion-status every 3 s (as the Domains screen re-reads),
# print each change with the full typed answer; stop at 204. Read only. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4/env.sh
C=$1; N=$2; ID=$3; D=$4; SECS=$5; t0=$(date +%s); last=""; unknown=0
while [ $(( $(date +%s) - t0 )) -lt $SECS ]; do
  s=$(b1 api $C $N GET /api/v1/domains/$ID/deletion-status --label delstatus-$D --limit 4000 2>/dev/null | python3 -c 'import json,sys
d=json.load(sys.stdin); b=d.get("body") or ""
try: j=json.loads(b); print(d.get("status"), json.dumps({k:j.get(k) for k in ("status","stage","reason","detail","error_line","message") if j.get(k)}, ensure_ascii=False)[:900])
except Exception: print(d.get("status"), b[:200])' 2>&1)
  echo "$s" | grep -qi '"unknown"' && unknown=$((unknown+1))
  [ "$s" != "$last" ] && echo "   $(date -u +%T) +$(( $(date +%s) - t0 ))s $s"; last=$s
  case "$s" in 204*) echo "   DELSTATUS-204 after $(( $(date +%s) - t0 ))s (unknown answers seen: $unknown)"; exit 0;; esac
  sleep 3
done
echo "   DELSTATUS-NOT-204 within ${SECS}s (unknown answers seen: $unknown)"; exit 1
