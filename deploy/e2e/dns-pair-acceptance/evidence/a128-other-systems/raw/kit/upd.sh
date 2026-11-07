# alpha.127 case 2: the owner presses Update once on NODE (126 -> 127) through the panel's own update API. Lab only.
# usage: upd.sh NODE
export CELL=${CELL:-r9} IP=${IP:-192.0.3.190} Z=${Z:-r9.test}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh
N=${1:-main}
exec > >(tee -a $LOGS/C2-$CELL-$N.log) 2>&1
upbody() { admn $N GET /api/v1/panel/update/check --label update-check --timeout 120 > $T/chk-$N.json; python3 -c '
import json,sys,secrets
d=json.load(open(sys.argv[1])); chk=json.loads(d["body"])
body={"request_id":secrets.token_hex(16),"confirmed":True,"current_version":chk["current_version"],"current_commit":chk["current_commit"],**chk["target"]}
json.dump(body,open(sys.argv[2],"w")); print(body["request_id"], chk["current_version"], "->", chk["target"].get("version"))' $T/chk-$N.json $T/upd-$N.json; }
upwait() { local RID=$1 UPT=$(date +%s) last="" s i
  for i in $(seq 1 ${2:-120}); do sleep 4
    s=$(admn $N GET "/api/v1/panel/update/status?request_id=$RID" --label update-status --timeout 30 2>/dev/null | python3 -c '
import json,sys; d=json.load(sys.stdin); b=json.loads(d.get("body") or "{}") if d.get("status")==200 else {}
steps=" ".join("%s:%s"%(s.get("id"),s.get("state")) for s in (b.get("steps") or []) if isinstance(s,dict))
print(d.get("status"), b.get("status"), b.get("phase"), (b.get("summary") or b.get("error") or "")[:240], "|", steps)' 2>/dev/null)
    case "$s" in 401*) b1 login $CELL $N >/dev/null 2>&1;; esac
    [ "$s" != "$last" ] && { echo "  $(( $(date +%s) - UPT ))s $s"; last="$s"; }
    case "$s" in "200 succeeded "*|"200 failed"*|"200 rolled_back"*|"200 recovered"*|"200 abandoned"*|"200 recovery_required"*) case "$s" in *verifying*) ;; *) break;; esac;; esac
  done; }
echo "== C2 $N before $(ts)"; ver $N; grun $N g_upd.sh -5min | sed -n '1,6p'
read RID rest < <(upbody); echo "  request $RID $rest"
echo "== press Update once $(ts)"
admn $N POST /api/v1/panel/update/start @$T/upd-$N.json --label update-start --timeout 120 | sbrief 600
upwait $RID 150
sleep 10; b1 login $CELL $N > /dev/null 2>&1
echo "== after $(ts)"; ver $N
admn $N GET "/api/v1/panel/update/status?request_id=$RID" --label update-status-final --timeout 30 > $LOGS/update-status-$CELL-$N.json; sbrief 1500 < $LOGS/update-status-$CELL-$N.json
grun $N g_upd.sh -15min
echo "== C2 $N done $(ts)"
