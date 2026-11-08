# alpha.127 case 9 (before, on 126): a Node.js site with a small Express app on the Debian 13 main. Lab only.
export CELL=${CELL:-r7} Z=${Z:-r7.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191} IP=${IP:-192.0.3.190}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
N=${N:-main}; D=${D:-node7.test}
exec > >(tee -a $LOGS/C9pre-$CELL-$N.log) 2>&1
echo "== versions $(ts)"; ver $N
admn $N GET /api/v1/runtimes/node/lts --label lts > $T/lts.json; sbrief 400 < $T/lts.json
V=$(admn $N GET /api/v1/runtimes/node --label nodes | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print((b.get("installed") or [""])[0])')
if [ -z "$V" ]; then
  V=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); r=b["releases"]; print(([x["version"] for x in r if x["version"].startswith("22.")] or [r[0]["version"]])[0])' $T/lts.json)
  echo "-- install Node.js $V $(ts)"
  admn $N POST /api/v1/runtimes/node "{\"version\":\"$V\",\"request_id\":\"$(rid)\"}" --label node-install --timeout 900 | sbrief 500
  for i in $(seq 1 60); do admn $N GET /api/v1/runtimes/node --label nodes | grep -q "$V" && break; sleep 10; done
fi
echo "node $V"; echo $V > $T/nodev.$N
admn $N GET /api/v1/runtimes/node --label nodes | sbrief 300
DID=$(mksite $N $D static); echo "$D id $DID"; echo $DID > $T/node.$N.id
grun $N g_nodeapp.sh $DID $V before-$N
echo "-- switch the site to Node.js (Hosting type) $(ts)"
admn $N PUT /api/v1/domains/$DID/hosting "{\"project_type\":\"node\",\"start_command\":\"node server.js\",\"runtime_version\":\"$V\"}" --label to-node --timeout 600 | sbrief 700
sleep 8
admn $N GET /api/v1/domains/$DID/app/status --label app-status | sbrief 900
gshn $N "systemctl cat celikapp-$DID.service 2>&1 | head -60; systemctl is-active celikapp-$DID.service; ss -ltnp | grep node"
echo "-- through nginx from the lab network"
lab ssh beta1-lab labnet -- "curl -sS -m 10 -H 'Host: $D' http://$MIP/ ; curl -sS -m 10 -H 'Host: $D' http://$MIP/whoami"
echo "== c9pre done $(ts)"
