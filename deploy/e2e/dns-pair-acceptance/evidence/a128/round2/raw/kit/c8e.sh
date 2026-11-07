# alpha.127 case 8 (part 3): with the administrator's address confirmed, nginx stopped outside the panel (lab load):
# the service alert and the site checks; the alert email in the lab mail capture (alerts@r7.test on this server). Lab only.
export CELL=r7 IP=192.0.3.190 Z=r7.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/lib.sh
source $A/lib5.sh
N=main; SVC=nginx
exec > >(tee -a $LOGS/C8-r7-main.log) 2>&1
alerts() { admn $N GET /api/v1/metrics/alerts --label malerts | python3 -c '
import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print("  settings", b.get("settings"), "email", b.get("email"), "unseen", b.get("unseen"))
for a in b.get("alerts") or []: print("  ", {k:a.get(k) for k in ("id","kind","subject","severity","state","first_seen","opened_at","resolved_at","mail_state","mail_detail")})'; }
echo "== before $(ts)"; alerts
echo "== stop $SVC outside the panel (lab load) $(ts)"; T0=$(date +%s)
gshn $N "systemctl stop $SVC; systemctl is-active $SVC"
for i in $(seq 1 24); do sleep 30
  s=$(admn $N GET /api/v1/metrics/alerts --label malerts | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(" ".join("%s/%s:%s:%s" % (a.get("kind"), a.get("subject"), a.get("state"), a.get("mail_state")) for a in b.get("alerts") or [] if a.get("kind")!="security_updates"))')
  echo "  +$(( $(date +%s) - T0 ))s $s"
  echo "$s" | grep -q "service_stopped/nginx:open:" && echo "$s" | grep -qvE "service_stopped/nginx:open:(pending|none)" && break
done
alerts
echo "-- site checks"; admn $N GET /api/v1/metrics/sites --label msites | sbrief 1500
echo "== start $SVC again $(ts)"; gshn $N "systemctl start $SVC; systemctl is-active $SVC"
echo "== the alert email in the lab mail capture $(ts)"; sleep 30; grun $N g_mail.sh alerts@$Z list 3
gshn $N "journalctl -u postfix@- -u postfix --since -10min -o cat --no-pager | grep -E alerts@ | tail -4 | cut -c1-250"
echo "== c8e done $(ts)"
