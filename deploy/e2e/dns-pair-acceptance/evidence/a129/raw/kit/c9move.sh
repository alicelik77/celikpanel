# alpha.127 case 9: the administrator's move of the 126 Node.js site (Components -> Node.js "Versions" drawer, panel
# window, EN), then the state after it. Lab only.
export CELL=${CELL:-r7} Z=${Z:-r7.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191} IP=${IP:-192.0.3.190}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
N=main
exec > >(tee -a $LOGS/C9post-$CELL-$N.log) 2>&1
OLD=$(cat $T/node.main.id)
via() { lab ssh beta1-lab labnet -- "curl -sS -m 10 -H 'Host: $1' http://$MIP/"; }
fromb() { gshn backup "for p in $*; do printf \"%s \" \"from boston to $MIP:\$p:\"; curl -sS -m 5 http://$MIP:\$p/ 2>&1 | head -c 120; echo; done"; }
echo "== B. the administrator's move (Components, Node.js row, Versions drawer, panel window) $(ts)"
python3 - $T/move.json <<'P'
import json, sys
OPWIN = """JSON.stringify({ host: (document.querySelector('[data-confirm-host]') || {dataset: {}}).dataset.confirmHost || null, running: !!document.querySelector('[data-operation-running]'),
 end: (document.querySelector('[data-operation-end]') || {dataset: {}}).dataset.operationEnd || null, text: ((document.querySelector('#operation-progress') || document.querySelector('#confirm-action') || {}).innerText || '').replace(/\\s+/g, ' ').slice(0, 900),
 steps: [...document.querySelectorAll('[data-operation-step]')].map(e => e.dataset.operationStep + ':' + e.dataset.stepState), tour: !!document.querySelector('[data-tour-overlay],[data-tour-offer]') })"""
OPEN = """(() => { const bs = [...document.querySelectorAll('button')].filter(b => /^(Versions|Sürümler)/.test(b.innerText.trim()));
 const b = bs.find(b => { let e = b; for (let i = 0; i < 12 && e; i++) { e = e.parentElement; if (e && e.innerText.trim().startsWith('Node.js')) return true; if (e && /PHP-FPM|Python/.test(e.innerText)) return false; } return false; });
 window.__opened = bs.length + ' toggles, node row ' + !!b; if (b) b.click(); })()"""
CARD = "JSON.stringify({opened: window.__opened, card: [...document.querySelectorAll('div,section')].filter(e => /earlier release|site sandbox|önceki bir sürüm/.test(e.innerText) && e.innerText.length < 2500).slice(-1).map(e => e.innerText.replace(/\\s+/g, ' ').slice(0, 1500))})"
items = [{"name": "node-move", "path": "/components", "wait": 4000, "actions": [
  {"eval": OPEN, "after": 2500}, {"probe": CARD, "label": "card before"}, {"shot": "before"},
  {"clickText": ["button", "into the sandbox|korumaya taşı"], "after": 800}, {"waitFor": "#confirm-action-confirm"},
  {"probe": OPWIN, "label": "question"}, {"shot": "question"},
  {"eval": "(() => { const b = document.getElementById('confirm-action-confirm'); b.click(); b.click(); })()", "after": 400},
  {"probe": OPWIN, "label": "locked window (confirm pressed twice)"}, {"shot": "progress"},
  {"waitJs": ["!!document.querySelector('[data-operation-close]')", 600000]}, {"probe": OPWIN, "label": "end"}, {"shot": "end"},
  {"click": "[data-operation-close]", "after": 3000}, {"probe": CARD, "label": "card after (no reload)"}]}]
json.dump(items, open(sys.argv[1], "w"))
P
timeout 1200 python3 -I $H/shots.py $CELL $N c9-move $T/move.json --langs en --widths desktop | python3 $A/shotsum.py
echo "== C. after the move $(ts)"
admn $N GET /api/v1/runtimes/node/sandbox --label sandbox | sbrief 1500
admn $N GET /api/v1/domains/$OLD/app/status --label app-status | sbrief 900
grun $N g_nodechk.sh $OLD; echo "via nginx: $(via node7.test)"; fromb 3001 3002
gshn $N "journalctl -u celikpanel-agent --since -10min -o cat --no-pager | grep -iE sandbox | tail -8 | cut -c1-300"
echo "== c9move done $(ts)"
