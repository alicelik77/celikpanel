# topology A line 2b rerun after the owner installed php8.4-mysql: finish WordPress setup, tools, clone. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/L2w2-$CELL.log) 2>&1
SUBB=$(cat $T/subb); WID=$(cat $T/wid)
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
echo "== WordPress rerun $(ts) (wp118.test id $WID)"
lc get - wp118.test ""
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
  echo "-- no plugin with an update available (bundled akismet/hello current): plugin update NOT exercised; core update offered: see status"
fi
echo "-- clone to a fresh staging subdomain"
SID=$(newdomain cust118b $SUBB stage2.wp118.test); echo "stage2.wp118.test id=$SID"
as cust118b POST /api/v1/wordpress/$WID/clone "{\"target_domain_id\":$SID}" > $T/wpc.json; sbrief 400 < $T/wpc.json
OP=$(jget operation_id < $T/wpc.json); [ -n "$OP" ] && wpop $WID $OP
lc get - stage2.wp118.test ""
as cust118b GET /api/v1/wordpress/$SID > $T/wps3.json; python3 $A/wpstat.py $T/wps3.json
echo "== L2w2 done $(ts)"
