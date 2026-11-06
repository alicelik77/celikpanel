# alpha.126 cases 2 + 3 on the joined server (both servers at 126): fresh deletions through the Domains page
# (headless Chromium, EN and TR), a repeated DELETE through the API, the served bundle searched. Lab only.
export CELL=${CELL:-p6} Z=${Z:-p6.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191}; export IP=${IP:-$MIP}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a126lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh
exec > >(tee -a $LOGS/C2-$CELL.log) 2>&1
echo "== versions $(ts)"; ver main; ver backup
echo "== three static sites on the joined server $(ts)"
for d in delen6.test deltr6.test api6.test; do echo "$d id $(mkstatic backup $d)"; done
sleep 30; for d in delen6.test deltr6.test api6.test; do digz $d; done
grun backup g_dnstool.sh | sed -n '/site include/,/agent journal/p'
for x in en tr; do
  echo "== case 2 ($x): Delete on the Domains page, Delete pressed twice in the panel's own window $(ts)"
  python3 $A/delui.py del${x}6.test $T/items-$x.json
  timeout 1500 python3 -I $H/shots.py $CELL backup c2-$x $T/items-$x.json --langs $x --widths desktop | python3 $A/shotsum.py
  grun backup g_dnstool.sh del${x}6.test | sed -n '/site include/,/agent journal/p'
  digz del${x}6.test
done
echo "== case 2: a repeated DELETE through the API (two requests 0.3 s apart) $(ts)"
AID=$(domain_id api6.test backup); echo "api6.test id $AID"
( admn backup DELETE /api/v1/domains/$AID --label delete-api6-1 --timeout 600 | sbrief 500 | sed 's/^/  first: /' ) &
sleep 0.3
admn backup DELETE /api/v1/domains/$AID --label delete-api6-2 --timeout 600 | sbrief 500 | sed 's/^/  second: /'
wait
admn backup DELETE /api/v1/domains/$AID --label delete-api6-3 --timeout 600 | sbrief 500 | sed 's/^/  third (after the first answered): /'
EVERY=3 delwatch backup $AID 600
echo "-- the delete stages the joined server ran (journal)"; grun backup g_peerlog.sh -15min "api6|delete|deletion" 30
echo "== case 3: the served bundle $(ts)"; grun backup g_bundle.sh
echo "== c2 done $(ts)"
