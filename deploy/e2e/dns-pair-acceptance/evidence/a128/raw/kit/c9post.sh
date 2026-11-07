# alpha.127 case 9 (after the update to 127): the 126 Node.js site is not rewritten silently; the administrator's move
# (panel window, EN) moves it; a new Node.js site runs sandboxed; the app's own port from another guest. Lab only.
export CELL=${CELL:-r7} Z=${Z:-r7.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191} IP=${IP:-192.0.3.190}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
N=main
exec > >(tee -a $LOGS/C9post-$CELL-$N.log) 2>&1
OLD=$(cat $T/node.main.id); V=$(cat $T/nodev.main); echo "old site node7.test id $OLD, node $V"
via() { lab ssh beta1-lab labnet -- "curl -sS -m 10 -H 'Host: $1' http://$MIP/"; }
fromb() { gshn backup "for p in $*; do printf \"%s \" \"from boston to $MIP:\$p:\"; curl -sS -m 5 http://$MIP:\$p/ 2>&1 | head -c 120; echo; done"; }
echo "== versions $(ts)"; ver $N
echo "== A. after the update, before any move $(ts)"
admn $N GET /api/v1/domains/$OLD/app/status --label app-status | sbrief 900
grun $N g_nodechk.sh $OLD
echo "  (126 render header sha was 7111967da67f..., the unit is unchanged when the same header and sha are shown)"
admn $N GET /api/v1/runtimes/node/sandbox --label sandbox | sbrief 1500
echo "-- the site owner's app listens on all addresses (Express default) and restarts (lab load)"
grun $N g_nodewild.sh $OLD
admn $N POST /api/v1/domains/$OLD/app/restart '{}' --label app-restart | sbrief 300; sleep 4
grun $N g_nodechk.sh $OLD | tail -3; echo "via nginx: $(via node7.test)"; fromb 3001
echo "== B. the administrator's move (Components -> Node.js, panel window) $(ts)"
python3 - $T/move.json <<'P'
import json, sys
OPWIN = """JSON.stringify({ host: (document.querySelector('[data-confirm-host]') || {dataset: {}}).dataset.confirmHost || null, running: !!document.querySelector('[data-operation-running]'),
 end: (document.querySelector('[data-operation-end]') || {dataset: {}}).dataset.operationEnd || null, text: ((document.querySelector('#operation-progress') || document.querySelector('#confirm-action') || {}).innerText || '').replace(/\\s+/g, ' ').slice(0, 900),
 steps: [...document.querySelectorAll('[data-operation-step]')].map(e => e.dataset.operationStep + ':' + e.dataset.stepState) })"""
CARD = "JSON.stringify([...document.querySelectorAll('section,div')].filter(e => /earlier release|site sandbox|önceki bir sürüm/.test(e.innerText) && e.innerText.length < 2500).slice(-1).map(e => e.innerText.replace(/\\s+/g, ' ').slice(0, 1500)))"
items = [{"name": "node-move", "path": "/components/node", "wait": 3500, "actions": [
  {"probe": CARD, "label": "card before"}, {"shot": "before"},
  {"clickText": ["button", "into the sandbox|korumaya taşı"], "after": 800}, {"waitFor": "#confirm-action-confirm"},
  {"probe": OPWIN, "label": "question"}, {"shot": "question"},
  {"eval": "(() => { const b = document.getElementById('confirm-action-confirm'); b.click(); b.click(); })()", "after": 400},
  {"probe": OPWIN, "label": "locked window"}, {"shot": "progress"},
  {"waitJs": ["!!document.querySelector('[data-operation-close]')", 600000]}, {"probe": OPWIN, "label": "end"}, {"shot": "end"},
  {"click": "[data-operation-close]", "after": 3000}, {"probe": CARD, "label": "card after (no reload)"}]}]
json.dump(items, open(sys.argv[1], "w"))
P
timeout 1200 python3 -I $H/shots.py $CELL $N c9-move $T/move.json --langs en --widths desktop | python3 $A/shotsum.py
echo "== C. after the move $(ts)"
admn $N GET /api/v1/runtimes/node/sandbox --label sandbox | sbrief 1500
admn $N GET /api/v1/domains/$OLD/app/status --label app-status | sbrief 900
grun $N g_nodechk.sh $OLD; echo "via nginx: $(via node7.test)"; fromb 3001
echo "== D. a new Node.js site on 127 $(ts)"
NEW=$(mksite $N node7n.test static); echo "node7n.test id $NEW"; echo $NEW > $T/node7n.id
grun $N g_nodeapp.sh $NEW $V new-127 | tail -3; grun $N g_nodewild.sh $NEW
admn $N PUT /api/v1/domains/$NEW/hosting "{\"project_type\":\"node\",\"start_command\":\"node server.js\",\"runtime_version\":\"$V\"}" --label to-node-127 --timeout 600 | sbrief 700
sleep 8; admn $N GET /api/v1/domains/$NEW/app/status --label app-status | sbrief 900
grun $N g_nodechk.sh $NEW; echo "via nginx: $(via node7n.test)"
P2=$(gshn $N "grep -h ^Environment=PORT= /etc/systemd/system/celikapp-$NEW.service | cut -d= -f3"); echo "new port $P2"
fromb 3001 $P2
echo "-- the Hosting tab's install packages and build (127), in the sandbox"
admn $N POST /api/v1/domains/$NEW/app/packages "{\"request_id\":\"node-$(rid)\",\"build\":true}" --label app-packages --timeout 900 | sbrief 900
sleep 20; admn $N GET /api/v1/domains/$NEW/app/packages --label app-packages-st | sbrief 900
gshn $N "ls -la /var/www/celikpanel/subscriptions/*/sites/$NEW/public_html/build-done.txt 2>&1; systemctl list-units --all --no-legend 'celikappinst-*' | cat"
echo "== c9post done $(ts)"
