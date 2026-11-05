# u24 (Ubuntu 24.04): PowerDNS main is refused there (since alpha.122), so the owner picks BIND; setup with an intermittent
# panel DB write-lock hold (lab load, 7 s held / 1 s free x 12) and a 2 s watch of the setup state (case 4). Lab only.
export CELL=u24 IP=192.0.3.193 Z=u24.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a123/kit/env.sh
exec > >(tee -a $LOGS/R24b-$CELL.log) 2>&1
W=$B/wiz-$CELL
python3 -c 'import json,sys; p=sys.argv[1]; d=json.load(open(p)); d["dns_engine"]="bind"; json.dump(d,open(p,"w"))' $W/dnshome.json
echo "== owner switches to BIND $(ts)"
b1 save $CELL main @$W/dnshome.json --label dnshome-bind --limit 300 | st dnshome 0
b1 records $CELL main > /dev/null
b1 save $CELL main @$W/mail.json --label mail --limit 300 | st mail 0
b1 plan $CELL main --limit 60000 > $LOGS/plan-$CELL-bind.json; python3 $H/plansum2.py $LOGS/plan-$CELL-bind.json plan | cut -c1-400
b1 start $CELL main --limit 3000 > $LOGS/start-$CELL.json; st start 300 < $LOGS/start-$CELL.json
sleep 3
echo "== case 4 (intermittent): lab load on the panel database $(ts)"
lab ssh $CELL main -- "sudo -n tee /root/g_holddb2.sh >/dev/null" < $A/g_holddb2.sh; lab ssh $CELL main -- "sudo -n bash /root/g_holddb2.sh 12 7 1"
for i in $(seq 1 60); do echo "-- $(ts) $(opnow $CELL main 2>/dev/null | head -1 | cut -c1-700)"; sleep 2; done > $LOGS/holddb2-watch-$CELL.log 2>&1
uniq -f2 $LOGS/holddb2-watch-$CELL.log | cut -c1-600
lab ssh $CELL main -- "sudo -n journalctl -u celikpanel-panel --since '-4min' -o short-precise --no-pager" 2>/dev/null | grep -iE 'busy|locked' | cut -c1-260 | head -20 | tee $LOGS/holddb2-journal-$CELL.txt
EVERY=20 TRIES=60 wait_status $CELL main succeeded failed
echo "== end $(ts)"; opnow $CELL main | head -1
