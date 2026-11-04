# run 3 item 1: ClamAV from Components ends Running (or waits, then Running by itself); EICAR scan + quarantine. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R3av-$CELL.log) 2>&1
SUBB=$(cat $T/subb)
echo "== R3av $(ts)"
R=$(rid); svcstart "{\"service_id\":\"clamav\",\"request_id\":\"$R\"}" /api/v1/service/install && svcwait $R 900
adm GET "/api/v1/service/operation?request_id=$R" --label clam-op | sbrief 1500
for i in $(seq 1 30); do
  s=$(adm GET /api/v1/malware/scanner --label scanner | sbrief 400); u=$(gsh1 'systemctl is-active clamav-daemon clamav-daemon.socket clamav-freshclam | tr "\n" " "')
  echo "  $(ts) scanner: $s | daemon socket freshclam: $u"; echo "$u" | grep -q "^active" && break; sleep 20
done
GQT=60 gq g_clam3.sh
AID=$(newdomain cust118b $SUBB av118.test); echo "av118.test id=$AID"
python3 $A/mkup.py $T/eicar.json eicar-test.txt eicar 2>/dev/null
as cust118b POST "/api/v1/domains/$AID/files?path=/" @$T/eicar.json | sbrief 200
as cust118b POST /api/v1/malware/domains/$AID/scan | sbrief 300
for i in $(seq 1 40); do sleep 15; s=$(as cust118b GET /api/v1/malware/domains/$AID | python3 $A/mwscan.py); case "$s" in queued*|running*|scanning*) ;; *) break ;; esac; done
echo "  scan: $s"; as cust118b GET /api/v1/malware/domains/$AID > $T/mw.json; python3 $A/mwscan.py findings < $T/mw.json
FID=$(jget findings.0.id < $T/mw.json)
if [ -n "$FID" ]; then GQT=60 gq gav.sh; echo "-- quarantine"; as cust118b POST /api/v1/malware/findings/$FID/quarantine | sbrief 300; GQT=60 gq gav.sh; as cust118b GET /api/v1/malware/domains/$AID | python3 $A/mwscan.py findings; fi
echo "== R3av done $(ts)"
