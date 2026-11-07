# alpha.127 case 10: a forced failed update rolls back by itself, the next press succeeds. Fresh 127 (case 1 guest f7)

# -> lab 128 label of the candidate, refused by the installer after services stopped (an unsupported panel.env key). Lab only.
export CELL=${CELL:-f7} Z=${Z:-f7.test} MIP=${MIP:-192.0.3.194} BIP=${BIP:-192.0.3.194}; export IP=${IP:-$MIP}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh
N=${N:-main}
exec > >(tee -a $LOGS/C10-$CELL-$N.log) 2>&1
upbody() { admn $N GET /api/v1/panel/update/check --label update-check --timeout 120 > $T/chk.json; python3 -c '
import json,sys,secrets
d=json.load(open(sys.argv[1])); chk=json.loads(d["body"])
body={"request_id":secrets.token_hex(16),"confirmed":True,"current_version":chk["current_version"],"current_commit":chk["current_commit"],**chk["target"]}
json.dump(body,open(sys.argv[2],"w")); print(body["request_id"], chk["current_version"], "->", chk["target"].get("version"))' $T/chk.json $T/upd.json; }
stline() { admn $N GET "/api/v1/panel/update/status?request_id=$1" --label update-status --timeout 30 2>/dev/null | python3 -c '
import json,sys; d=json.load(sys.stdin); b=json.loads(d.get("body") or "{}") if d.get("status")==200 else {}
steps=" ".join("%s:%s"%(s.get("id"),s.get("state")) for s in ((b.get("plan") or {}).get("steps") or b.get("steps") or []) if isinstance(s,dict))
print(d.get("status"), b.get("status"), b.get("phase"), (b.get("summary") or b.get("error") or "")[:260], "|", steps)' 2>/dev/null; }
upwait() { local RID=$1 UPT=$(date +%s) last="" s i
  for i in $(seq 1 ${2:-200}); do sleep ${3:-3}
    s=$(stline $RID); case "$s" in 401*) b1 login $CELL $N >/dev/null 2>&1;; esac
    [ "$s" != "$last" ] && { echo "  $(date -u +%T) +$(( $(date +%s) - UPT ))s $s"; last="$s"; }
    case "$s" in "200 succeeded "*|"200 failed"*|"200 rolled_back"*|"200 recovered"*|"200 abandoned"*|"200 recovery_required"*) case "$s" in *verifying*) ;; *) break;; esac;; esac
  done; }
echo "== versions $(ts)"; ver $N
echo "== lab 128 label is the portal's latest $(ts)"; bash $A/portalF.sh latest v0.1.0-alpha.128
gsh1 'echo CELIKPANEL_LAB_ROLLBACK_PROBE=1 >> /etc/celikpanel/panel.env; tail -1 /etc/celikpanel/panel.env'
read RID1 rest < <(upbody); echo "  request $RID1 $rest"; T0=$(date -u +%T)
admn $N POST /api/v1/panel/update/start @$T/upd.json --label update-start-probe --timeout 120 | sbrief 400
upwait $RID1 200 3
for i in $(seq 1 60); do admn $N GET /api/v1/panel/version --label ver 2>/dev/null | grep -q '"status": 200' && break; b1 login $CELL $N >/dev/null 2>&1; sleep 2; done
echo "== after the refused attempt $(ts)"; ver $N
admn $N GET "/api/v1/panel/update/status?request_id=$RID1" --label update-status-probe > $LOGS/update-status-probe-$CELL.json; sbrief 1600 < $LOGS/update-status-probe-$CELL.json
echo "-- wait for the recovery runner to finish (up to 3 min)"
for i in $(seq 1 36); do s=$(gsh1 'journalctl -u celikpanel-release-recovery.service --since '"$T0"' -o cat --no-pager | grep -c "Rollback complete"'); [ "${s:-0}" -ge 1 ] && break; sleep 5; done
gshn $N "echo '--- systemctl --failed (direct)'; systemctl list-units --failed --all --no-legend --plain | cat; echo '(failed end)'; journalctl --since $T0 -o short-iso --no-pager | grep -iE 'rollback|rolled back|restor|recovery_required|release recovery|CELIKPANEL_UPDATE|Recovery dispatch|Verified snapshot' | cut -c1-280 | tail -30; cat /opt/celikpanel/release.commit"
echo "== the owner removes the line and presses Update again $(ts)"
gsh1 'sed -i "/^CELIKPANEL_LAB_ROLLBACK_PROBE=/d" /etc/celikpanel/panel.env; grep -c LAB_ROLLBACK /etc/celikpanel/panel.env'
read RID2 rest < <(upbody); echo "  request $RID2 $rest"; T2=$(date +%s)
admn $N POST /api/v1/panel/update/start @$T/upd.json --label update-start --timeout 120 | sbrief 400
upwait $RID2 300 2
echo "  press-to-end $(( $(date +%s) - T2 )) s"
sleep 10; b1 login $CELL $N > /dev/null 2>&1; echo "== after $(ts)"; ver $N
grun $N g_upd.sh -15min
gshn $N "echo '--- systemctl --failed (direct, after the second press)'; systemctl list-units --failed --all --no-legend --plain | cat; echo '(failed end)'"
bash $A/portalF.sh latest v0.1.0-alpha.126
echo "== c10 done $(ts)"
