# alpha.124 case 1: Debian 13 installed at the lab copy of alpha.123 (u123), a failed self-update attempt, then the
# update to the 124 candidate through the panel's own update API, then the Agent checks. Lab only.
export CELL=u123 IP=192.0.3.192 Z=u123.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a124lab/kit/lib.sh
exec > >(tee -a $LOGS/C1-$CELL.log) 2>&1
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
echo "== C1.0 on 123 $(ts)"; ver; c1
echo "== C1.1 attempt with the package manager busy (dpkg lock held, lab load) $(ts)"
lab ssh $CELL main -- "sudo -n systemd-run --unit=lab-dpkg-hold python3 -c \"import fcntl,time; f=open('/var/lib/dpkg/lock-frontend','w'); fcntl.lockf(f, fcntl.LOCK_EX); g=open('/var/lib/dpkg/lock','w'); fcntl.lockf(g, fcntl.LOCK_EX); time.sleep(300)\"; sleep 1; sudo -n systemctl is-active lab-dpkg-hold"
read RID1 rest < <(upbody); echo "  request $RID1 $rest"
adm POST /api/v1/panel/update/start @$T/upd.json --label update-start-busy --timeout 120 | sbrief 600
sleep 20; adm GET "/api/v1/panel/update/status?request_id=$RID1" --label upd-status-busy | sbrief 600
lab ssh $CELL main -- "sudo -n systemctl stop lab-dpkg-hold; sudo -n systemctl reset-failed lab-dpkg-hold 2>/dev/null; echo lock released"
c1 | sed -n '/failed self-update/,/attempt records/p'
echo "== C1.2 attempt the installer refuses: the owner left an unsupported key in panel.env (lab load) $(ts)"
gsh1 'echo CELIKPANEL_LAB_ROLLBACK_PROBE=1 >> /etc/celikpanel/panel.env; tail -1 /etc/celikpanel/panel.env'
read RID2 rest < <(upbody); echo "  request $RID2 $rest"
adm POST /api/v1/panel/update/start @$T/upd.json --label update-start-probe --timeout 120 | sbrief 600
upwait $RID2 90
ver; c1
echo "== C1.3 the owner removes the line and presses Update once $(ts)"
gsh1 'sed -i "/^CELIKPANEL_LAB_ROLLBACK_PROBE=/d" /etc/celikpanel/panel.env; grep -c LAB_ROLLBACK /etc/celikpanel/panel.env'
sleep 20
for k in 1 2 3 4 5 6; do read RID3 rest < <(upbody); echo "  request $RID3 $rest"; r=$(adm POST /api/v1/panel/update/start @$T/upd.json --label update-start --timeout 120 | sbrief 600); echo "  $r"; case "$r" in 409*BUSY*) sleep 30;; *) break;; esac; done
upwait $RID3 120
sleep 20; b1 login $CELL main > /dev/null 2>&1; ver
echo "== C1.4 after the update: version, second Agent, socket, failed units $(ts)"
c1 full
echo "-- panel still reaches the Agent"; adm GET /api/v1/managed-services --label ms-after | sbrief 200
echo "== C1 done $(ts)"
