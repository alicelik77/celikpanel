# topology A line 2b (customer B): WordPress install + tools, then ClamAV. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/L2w-$CELL.log) 2>&1
SUBB=$(cat $T/subb); DR=${DOCDIR:-/}
wpop() {
  local id=$1 op=$2 s i
  for i in $(seq 1 90); do
    s=$(as cust118b GET /api/v1/wordpress/$id/operations/$op | python3 $A/wpop.py)
    case "$s" in
      queued*|backing_up*|updating*|restoring*|cloning*) sleep 10 ;;
      *) echo "  wp op $op: $s ($(ts))"; return ;;
    esac
  done
  echo "  wp op still $s"
}
if [ -z "${SKIPWP:-}" ]; then
echo "== WordPress $(ts)"
WID=$(newdomain cust118b $SUBB wp118.test); echo "wp118.test id=$WID"; echo $WID > $T/wid
AS_TIMEOUT=900 as cust118b POST /api/v1/domains/$WID/apps/install '{"app":"wordpress"}' | sbrief 500
mkcred wpadm
nup labchk.sh >/dev/null; pw wpadm | timeout 120 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/labchk.sh wpfinish $IP wp118.test"
lc get - wp118.test ""
echo "-- status"; as cust118b GET /api/v1/wordpress/$WID > $T/wps.json; python3 $A/wpstat.py $T/wps.json
PF=$(python3 $A/wpstat.py $T/wps.json pick)
if [ -n "$PF" ]; then
  echo "-- update plugin $PF"
  as cust118b POST /api/v1/wordpress/$WID/update "{\"kind\":\"plugin\",\"items\":[\"$PF\"]}" > $T/wpu.json; sbrief 400 < $T/wpu.json
  OP=$(jget operation_id < $T/wpu.json); [ -n "$OP" ] && wpop $WID $OP
  as cust118b GET /api/v1/wordpress/$WID > $T/wps2.json; python3 $A/wpstat.py $T/wps2.json
else
  echo "-- no plugin with an update available: plugin update NOT exercised"
fi
echo "-- clone to staging"
SID=$(newdomain cust118b $SUBB staging.wp118.test); echo "staging.wp118.test id=$SID"
as cust118b GET /api/v1/wordpress/$WID/clone-targets | sbrief 300
as cust118b POST /api/v1/wordpress/$WID/clone "{\"target_domain_id\":$SID}" > $T/wpc.json; sbrief 400 < $T/wpc.json
OP=$(jget operation_id < $T/wpc.json); [ -n "$OP" ] && wpop $WID $OP
lc get - staging.wp118.test ""
fi
echo "== ClamAV $(ts)"
[ -z "${SKIPINST:-}" ] && R=$(rid) && svcstart "{\"service_id\":\"clamav\",\"request_id\":\"$R\"}" /api/v1/service/install && svcwait $R 900
for i in $(seq 1 30); do s=$(adm GET /api/v1/malware/scanner --label scanner | sbrief 600); echo "$s" | grep -qiE '"state": *"ready"' && break; sleep 20; done; echo "  scanner: $s"
AID=$(newdomain cust118b $SUBB av118.test); echo "av118.test id=$AID"; echo $AID > $T/aid
python3 $A/mkup.py $T/eicar.json eicar-test.txt eicar
as cust118b POST "/api/v1/domains/$AID/files?path=$DR" @$T/eicar.json | sbrief 200
as cust118b POST /api/v1/malware/domains/$AID/scan | sbrief 300
for i in $(seq 1 40); do
  sleep 15
  s=$(as cust118b GET /api/v1/malware/domains/$AID | python3 $A/mwscan.py)
  case "$s" in queued*|running*|scanning*) ;; *) break ;; esac
done
echo "  scan: $s"
as cust118b GET /api/v1/malware/domains/$AID > $T/mw.json; python3 $A/mwscan.py findings < $T/mw.json
FID=$(jget findings.0.id < $T/mw.json)
if [ -n "$FID" ]; then
  GQT=60 gq gav.sh
  echo "-- quarantine"; as cust118b POST /api/v1/malware/findings/$FID/quarantine | sbrief 300; GQT=60 gq gav.sh
  echo "-- restore"; as cust118b POST /api/v1/malware/findings/$FID/restore | sbrief 300; GQT=60 gq gav.sh
fi
echo "== L2w done $(ts)"
