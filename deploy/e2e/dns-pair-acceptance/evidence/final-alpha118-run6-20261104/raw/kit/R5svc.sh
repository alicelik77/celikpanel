# run 5 items 1-2: n8n env file location; malicious link swap, then change keys + restart through the panel. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R5svc-$CELL.log) 2>&1
NID=$(cat $T/nid)
echo "== item 1 env file $(ts)"; GQT=60 gq g_env5.sh
echo "== item 2 attack setup (lab as malicious customer) $(ts)"; GQT=60 gq g_attack5.sh n8n118_test
echo "-- change the app's keys through the panel (owner password)"
mkcred n8nowner2
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"secrets":{},"clear":[],"owner_password":c["password"]+"Zz9"},open(sys.argv[2],"w"))' $T/cred-n8nowner2.json $T/sec.json
as cust118n PUT /api/v1/domains/$NID/service-apps/secrets @$T/sec.json | sbrief 700; rm -f $T/sec.json
echo "-- restart through the panel"; as cust118n POST /api/v1/domains/$NID/service-apps/control '{"action":"restart"}' | sbrief 700
for i in $(seq 1 12); do s=$(as cust118n GET /api/v1/domains/$NID/service-apps | python3 $A/sastat.py); echo "  $(ts) $s"; case "$s" in *"active=True serving=True"*|*failed*) break;; esac; sleep 10; done
sleep 15; GQT=60 gq g_env5.sh
lc https - n8n118.test ""
echo "-- owner login with the new password"; nup labchk.sh >/dev/null; echo "$(pw n8nowner2)Zz9" | timeout 60 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/labchk.sh n8nlogin $IP n8n118.test"
echo "== R5svc done $(ts)"
