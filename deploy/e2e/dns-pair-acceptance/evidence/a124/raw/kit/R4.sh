# alpha.124 case 4 on p124 main: the panel database write lock held ~30 s during a component install (lab load:
# one extra SQLite writer, BEGIN IMMEDIATE, rolls back). Screen state, finish after release, audit rows. Lab only.
export CELL=p124 IP=192.0.3.190 Z=p124.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a124lab/kit/lib.sh
source $A/lib11.sh; source $A/lib123.sh; source $A/lib124.sh
exec > >(tee -a $LOGS/R4-$CELL.log) 2>&1
SVC=${SVC:-redis}
auditpage() { adm GET "/api/v1/audit-logs?limit=$1" --label "audit-$2" | python3 -c '
import json,sys
d=json.load(sys.stdin); b=d.get("body") or ""
j=json.loads(b) if b.startswith("{") else {}
L=j.get("logs") or j.get("items") or j.get("entries") or []
print("  ", d.get("status"), json.dumps({k:v for k,v in j.items() if k not in ("logs","items","entries")})[:500])
for x in L[:int(sys.argv[1])]: print("    ", x.get("action"), x.get("created_at"), str(x.get("details") or x.get("detail") or "")[:160])' $1; }
echo "== 4.0 audit rows before $(ts)"; auditcount 5
comps $SVC
prepare $SVC; R=$(cat $T/rid-$SVC)
lab ssh $CELL main -- "sudo -n tee /root/g_holddb.sh >/dev/null" < $A/g_holddb.sh
echo "== 4.1 start the install, hold the lock 30 s two seconds in $(ts)"
adm POST /api/v1/service/install @$T/body-$SVC.json --label install-held | sbrief 600
sleep 2
lab ssh $CELL main -- "sudo -n bash /root/g_holddb.sh 30"
last=""
for i in $(seq 1 40); do
  s=$(opread "request_id=$R"); [ "$s" != "$last" ] && { echo "  $(ts)$s"; last="$s"; }
  if [ $i = 6 ]; then echo "  -- audit page while held"; auditpage 1 held; gsh1 'ls -la /var/lib/celikpanel/audit-pending.json 2>&1 | tail -1'; fi
  case "$s" in *'"status": "succeeded"'*|*'"status": "failed"'*) break;; esac
  sleep 2
done
followr "request_id=$R" 200
lab ssh $CELL main -- "sudo -n bash -c 'journalctl -u lab-dbhold -o cat --no-pager | tail -3'"
echo "== 4.2 audit rows after (drain allowed 40 s) $(ts)"; sleep 40; auditcount 8
auditpage 8 after
gsh1 'journalctl -u celikpanel-panel --since "-6min" -o cat --no-pager | grep -iE "audit|busy|locked" | cut -c1-260 | tail -12'
comps $SVC
echo "== R4 done $(ts)"
