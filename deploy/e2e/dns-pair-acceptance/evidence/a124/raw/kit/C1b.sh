# alpha.124 case 1: Debian 13 installed at the lab copy of alpha.123 (u123), a failed self-update attempt, then the
# update to the 124 candidate through the panel's own update API, then the Agent checks. Lab only.
export CELL=u123 IP=192.0.3.192 Z=u123.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a124lab/kit/lib.sh
exec > >(tee -a $LOGS/C1b-$CELL.log) 2>&1
c1() { lab ssh $CELL main -- "sudo -n tee /root/g_c1.sh >/dev/null" < $A/g_c1.sh; timeout 200 python3 -I $L/lab.py ssh $CELL main -- "sudo -n bash /root/g_c1.sh $*"; }
upbody() { adm GET /api/v1/panel/update/check --label update-check --timeout 120 > $T/chk.json; python3 -c '
import json,sys,secrets
d=json.load(open(sys.argv[1])); chk=json.loads(d["body"])
body={"request_id":secrets.token_hex(16),"confirmed":True,"current_version":chk["current_version"],"current_commit":chk["current_commit"],**chk["target"]}
json.dump(body,open(sys.argv[2],"w")); print(body["request_id"], chk["current_version"], "->", chk["target"].get("version"))' $T/chk.json $T/upd.json; }
upwait() { local RID=$1 UPT=$(date +%s) last="" s i
  for i in $(seq 1 ${2:-90}); do sleep 8
    s=$(adm GET "/api/v1/panel/update/status?request_id=$RID" --label update-status --timeout 30 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d.get("body") or "{}") if d.get("status")==200 else {}; print(d.get("status"), b.get("status"), b.get("phase"), (b.get("summary") or b.get("error") or "")[:300])' 2>/dev/null)
    case "$s" in 401*) b1 login $CELL main >/dev/null 2>&1;; esac
    [ "$s" != "$last" ] && { echo "  $(( $(date +%s) - UPT ))s $s"; last="$s"; }
    case "$s" in "200 succeeded "*|"200 failed"*|"200 rolled_back"*|"200 recovered"*|"200 abandoned"*) case "$s" in *verifying*) ;; *) break;; esac;; esac
  done; }
ver() { adm GET /api/v1/panel/version --label version | sbrief 300; }
echo "== C1b.0 the 123 Agent after the hand-started test (123 ignored --version and replaced the socket) $(ts)"; c1 | head -5
echo "-- the owner runs sudo systemctl restart celikpanel-agent $(ts)"
gsh1 'systemctl restart celikpanel-agent; sleep 10; systemctl is-active celikpanel-agent; stat -c "%A %U:%G %n" /run/celikpanel/agent.sock'
sleep 60
adm GET /api/v1/host-mutation-readiness --label readiness | sbrief 300
echo "== C1b.1 the owner presses Update again $(ts)"
for k in 1 2 3 4 5 6; do read RID3 rest < <(upbody); echo "  request $RID3 $rest"; r=$(adm POST /api/v1/panel/update/start @$T/upd.json --label update-start2 --timeout 120 | sbrief 600); echo "  $r"; case "$r" in 409*BUSY*) sleep 30;; *) break;; esac; done
upwait $RID3 120
sleep 30; b1 login $CELL main > /dev/null 2>&1; ver
echo "== C1b.2 after the update: version, second Agent, socket, failed units $(ts)"
c1 full
echo "-- panel still reaches the Agent"; adm GET "/api/v1/service/install-plan?id=redis" --label plan-after | sbrief 300
gsh1 'journalctl -u celikpanel-agent --since "-10min" -o cat --no-pager | grep -iE "reset|failed unit|self-update|instance" | tail -8'
echo "== C1b done $(ts)"
