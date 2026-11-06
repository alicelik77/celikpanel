# case 1 defect reproduction on p5 main (124 -> 125): a package task appears while the update prepares its recovery runtime. Lab only.
export CELL=p5 IP=192.0.3.190 Z=p5.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a125acc/kit/lib.sh
source $A/lib5.sh
N=main
exec > >(tee -a $LOGS/C1${TAG:-repro}-$CELL.log) 2>&1
b1 login $CELL main >/dev/null 2>&1
echo "== arm the package-task load $(ts)"; grun main g_pkgwatch.sh ${HOLD:-20}
admn main GET /api/v1/panel/update/check --label update-check --timeout 120 > $T/chk.json
python3 -c '
import json,sys,secrets
d=json.load(open(sys.argv[1])); chk=json.loads(d["body"])
body={"request_id":secrets.token_hex(16),"confirmed":True,"current_version":chk["current_version"],"current_commit":chk["current_commit"],**chk["target"]}
json.dump(body,open(sys.argv[2],"w")); print(body["request_id"], chk["current_version"], "->", chk["target"].get("version"))' $T/chk.json $T/upd.json > $T/rid
read RID rest < $T/rid; echo "  request $RID $rest"
echo "== press Update $(ts)"; admn main POST /api/v1/panel/update/start @$T/upd.json --label update-start --timeout 120 | sbrief 300
last=""; for i in $(seq 1 150); do sleep 3
  s=$(admn main GET "/api/v1/panel/update/status?request_id=$RID" --label st --timeout 30 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d.get("body") or "{}") if d.get("status")==200 else {}; print(d.get("status"), b.get("status"), (b.get("summary") or b.get("error") or "")[:260])' 2>/dev/null)
  case "$s" in 401*) b1 login $CELL main >/dev/null 2>&1;; esac
  [ "$s" != "$last" ] && { echo "  $(ts) $s"; last="$s"; }
  case "$s" in "200 succeeded"*|"200 failed"*|"200 rolled_back"*) break;; esac; done
sleep 8; b1 login $CELL main >/dev/null 2>&1; ver main
gshn main 'journalctl -u lab-pkgwatch -o cat --no-pager | tail -4; systemctl reset-failed lab-pkgwatch 2>/dev/null; rm -f /root/dpkg'
grun main g_upd.sh -15min
echo "== repro done $(ts)"
