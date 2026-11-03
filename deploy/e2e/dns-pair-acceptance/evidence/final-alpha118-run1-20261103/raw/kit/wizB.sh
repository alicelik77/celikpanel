# topology B: re-run the one-server wizard on the installed alpha.117 Ubuntu 24.04 with BIND as the DNS engine
# (PowerDNS main is refused on Ubuntu by design: server_setup_pdns_main_untested_system). Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/env.sh
CELL=u117; W=$B/wiz-$CELL
exec > >(tee -a $LOGS/single-$CELL.log) 2>&1
bash $A/portalF.sh up v0.1.0-alpha.118 > $A/portalnet-up.out 2>&1; tail -n 1 $A/portalnet-up.out
python3 -c 'import json,sys; p=sys.argv[1]; d=json.load(open(p)); d["dns_engine"]="bind"; json.dump(d,open(p,"w"))' $W/dnshome.json
b1 save $CELL main @$W/dnshome.json --label dnshome-bind --limit 300 | st dnshome 300
b1 records $CELL main > /dev/null
b1 plan $CELL main --limit 60000 > $LOGS/plan-$CELL-main.json; python3 $H/plansum2.py $LOGS/plan-$CELL-main.json plan
echo "== start $(ts)"
b1 start $CELL main --limit 300 | st start 160
EVERY=20 TRIES=120 wait_status $CELL main succeeded failed
echo "== wizB done $(ts)"
