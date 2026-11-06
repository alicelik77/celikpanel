# alpha.125 Debian 12 fresh install at 125 (d5): smoke (components, a site, a panel-written crontab), then case 2:
# a forced rollback (125 -> lab 126 label refused by the installer), then Update pressed at once while the panel's
# start-up mail work runs. Lab only.
export CELL=p5 IP=192.0.3.190 Z=p5.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a125acc/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/C2-$CELL.log) 2>&1
N=main
upbody() { adm GET /api/v1/panel/update/check --label update-check --timeout 120 > $T/chk.json; python3 -c '
import json,sys,secrets
d=json.load(open(sys.argv[1])); chk=json.loads(d["body"])
body={"request_id":secrets.token_hex(16),"confirmed":True,"current_version":chk["current_version"],"current_commit":chk["current_commit"],**chk["target"]}
json.dump(body,open(sys.argv[2],"w")); print(body["request_id"], chk["current_version"], "->", chk["target"].get("version"))' $T/chk.json $T/upd.json; }
stline() { adm GET "/api/v1/panel/update/status?request_id=$1" --label update-status --timeout 30 2>/dev/null | python3 -c '
import json,sys; d=json.load(sys.stdin); b=json.loads(d.get("body") or "{}") if d.get("status")==200 else {}
steps=" ".join("%s:%s"%(s.get("id"),s.get("state")) for s in (b.get("steps") or []) if isinstance(s,dict))
print(d.get("status"), b.get("status"), b.get("phase"), (b.get("summary") or b.get("error") or "")[:200], "|", steps)' 2>/dev/null; }
upwait() { local RID=$1 UPT=$(date +%s) last="" s i
  for i in $(seq 1 ${2:-200}); do sleep ${3:-4}
    s=$(stline $RID); case "$s" in 401*) b1 login $CELL $N >/dev/null 2>&1;; esac
    [ "$s" != "$last" ] && { echo "  $(date -u +%T) +$(( $(date +%s) - UPT ))s $s"; last="$s"; }
    case "$s" in "200 succeeded "*|"200 failed"*|"200 rolled_back"*|"200 recovered"*|"200 abandoned"*|"200 recovery_required"*) case "$s" in *verifying*) ;; *) break;; esac;; esac
  done; }
echo "== case 2: the lab 126 label of the same candidate on the portal $(ts)"
bash $A/portalF.sh add v0.1.0-alpha.126 | tail -3; bash $A/portalF.sh latest v0.1.0-alpha.126
echo "== 2.1 an attempt the installer refuses: the owner left an unsupported key in panel.env (lab load) $(ts)"
gsh1 'echo CELIKPANEL_LAB_ROLLBACK_PROBE=1 >> /etc/celikpanel/panel.env; tail -1 /etc/celikpanel/panel.env'
read RID1 rest < <(upbody); echo "  request $RID1 $rest"
adm POST /api/v1/panel/update/start @$T/upd.json --label update-start-probe --timeout 120 | sbrief 600
upwait $RID1 200 3
echo "== 2.2 the owner removes the line; as soon as the panel answers, Update is pressed (start-up mail work running) $(ts)"
gsh1 'sed -i "/^CELIKPANEL_LAB_ROLLBACK_PROBE=/d" /etc/celikpanel/panel.env; grep -c LAB_ROLLBACK /etc/celikpanel/panel.env'
for i in $(seq 1 60); do adm GET /api/v1/panel/version --label ver 2>/dev/null | grep -q '"status": 200' && break; b1 login $CELL main >/dev/null 2>&1; sleep 1; done
echo "  panel answers $(ts)"
read RID2 rest < <(upbody); echo "  request $RID2 $rest"
echo "  pressed $(ts)"; adm POST /api/v1/panel/update/start @$T/upd.json --label update-start --timeout 120 | sbrief 600
upwait $RID2 300 1
sleep 10; b1 login $CELL main > /dev/null 2>&1; echo "== after $(ts)"; ver
adm GET "/api/v1/panel/update/status?request_id=$RID2" --label update-status-final > $LOGS/update-status-$CELL.json; sbrief 2500 < $LOGS/update-status-$CELL.json
grun main g_upd.sh -20min
echo "-- the panel's start-up mail work and the update (journal, both units)"
gshn main 'journalctl -u celikpanel-panel -u celikpanel-agent --since -20min -o short-iso --no-pager | grep -iE "mail_tls_sync|mail_filter|filter wir|start-up mail|deferred|admitted|self-update.*(admit|start|queued)|yield|waits? for" | cut -c1-260 | tail -25'
echo "== case2 done $(ts)"
