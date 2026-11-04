# run 2 D6: ClamAV from Components never reports a failure; Running once signatures land; EICAR scan. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R2av-$CELL.log) 2>&1
SUBB=$(cat $T/subb)
echo "== R2av $(ts)"
R=$(rid); svcstart "{\"service_id\":\"clamav\",\"request_id\":\"$R\"}" /api/v1/service/install && svcwait $R 900
adm GET "/api/v1/service/operation?request_id=$R" --label clam-op | sbrief 1500
for i in $(seq 1 30); do
  s=$(adm GET /api/v1/malware/scanner --label scanner | sbrief 400); u=$(gsh1 'systemctl is-active clamav-daemon clamav-freshclam | tr "\n" " "')
  echo "  $(ts) scanner: $s | units: $u"; echo "$u" | grep -q "^active" && break; sleep 20
done
adm GET /api/v1/services --label services | python3 -c 'import json,sys; d=json.load(sys.stdin); t=d["body"]; i=t.find("clamav"); print(d["status"], t[max(0,i-200):i+700])' 2>&1 | cut -c1-900
AID=$(newdomain cust118b $SUBB av.apps118.test); echo "av.apps118.test id=$AID"
python3 $A/mkup.py $T/eicar.json eicar-test.txt eicar 2>/dev/null
as cust118b POST "/api/v1/domains/$AID/files?path=/" @$T/eicar.json | sbrief 200
as cust118b POST /api/v1/malware/domains/$AID/scan | sbrief 300
for i in $(seq 1 40); do sleep 15; s=$(as cust118b GET /api/v1/malware/domains/$AID | python3 $A/mwscan.py); case "$s" in queued*|running*|scanning*) ;; *) break ;; esac; done
echo "  scan: $s"; as cust118b GET /api/v1/malware/domains/$AID | python3 $A/mwscan.py findings
echo "== R2av done $(ts)"
