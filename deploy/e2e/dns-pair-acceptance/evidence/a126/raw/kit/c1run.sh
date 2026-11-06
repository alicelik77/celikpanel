# alpha.126 case 1 (part 2) + case 7: the owner updates the joined server, then the main, each by one Update press
# (125 -> 126 candidate); the stuck deletion is watched throughout. ORDER=joined-first|main-first. Lab only.
export CELL=${CELL:-p6} Z=${Z:-p6.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191}; export IP=${IP:-$MIP}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a126lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh
exec > >(tee -a $LOGS/C1run-$CELL.log) 2>&1
IID=$(cat $T/inc-id); D=${D:-inc6.test}; ORDER=${ORDER:-joined-first}
case $ORDER in joined-first) FIRST=backup SECOND=main;; *) FIRST=main SECOND=backup;; esac
echo "== $D (id $IID on the joined server) before any update $(ts)"; delst backup $IID
bash $A/portalF.sh latest v0.1.0-alpha.126
echo "== 1st update: $FIRST $(ts)"
IP=$([ $FIRST = main ] && echo $MIP || echo $BIP) bash $A/upd.sh $FIRST > $LOGS/updrun-$FIRST.out 2>&1; sed -n '/== press/,$p' $LOGS/C1-$CELL-$FIRST.log | grep -vE "^--- attempt|\['self-update'" | head -40
T1=$(date +%s); echo "  $FIRST updated $(ts)"
grun backup g_dnstool.sh $D
echo "== watch the deletion with only $FIRST updated (up to 7 min) $(ts)"
EVERY=10 delwatch backup $IID 420 && echo "  ENDED with only $FIRST updated, $(( $(date +%s) - T1 ))s after its update"
for n in main backup; do echo "-- $n"; grun $n g_peerlog.sh -8min "" 8; done
echo "== 2nd update: $SECOND $(ts)"
IP=$([ $SECOND = main ] && echo $MIP || echo $BIP) bash $A/upd.sh $SECOND > $LOGS/updrun-$SECOND.out 2>&1; sed -n '/== press/,$p' $LOGS/C1-$CELL-$SECOND.log | grep -vE "^--- attempt|\['self-update'" | head -40
T2=$(date +%s); echo "  $SECOND updated $(ts)"
EVERY=10 delwatch backup $IID 1200 && echo "  ENDED $(( $(date +%s) - T2 ))s after the $SECOND update ($(( $(date +%s) - T1 ))s after the $FIRST update)"
echo "== after $(ts)"
for n in main backup; do echo "-- $n"; ver $n; grun $n g_peerlog.sh -25min "" 12; done
grun backup g_dnstool.sh $D
echo "-- domain lists"; for n in backup main; do admn $n GET /api/v1/domains --label domains-$n | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); s=json.dumps(b); print("  ", sys.argv[1], "lists", sys.argv[2], sys.argv[2] in s, "|", s[:300])' $n $D; done
digz $D
echo "== Users page traffic warning on the joined server after a usage round $(ts)"; waitround
admn backup GET /api/v1/usage/status --label usage-after | python3 $A/usagesum.py
echo "== c1run done $(ts)"
