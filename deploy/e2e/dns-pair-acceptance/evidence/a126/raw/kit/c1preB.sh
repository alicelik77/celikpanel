# alpha.126 case 1, other order (pair p7 at 125): the incident reproduced on inc7.test, then main first. Lab only.
export CELL=p7 Z=p7.test MIP=192.0.3.192 BIP=192.0.3.193 IP=192.0.3.192
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a126lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh
exec > >(tee -a $LOGS/C1pre-$CELL.log) 2>&1
echo "== versions $(ts)"; ver main; ver backup
grun backup g_dnstool.sh | sed -n '1,2p;/named control/,/site include/p'
IID=$(mkstatic backup inc7.test); echo "inc7.test id on backup: $IID"; echo $IID > $T/inc-id
sleep 20; digz inc7.test
echo "== DELETE inc7.test on the joined server $(ts)"
admn backup DELETE /api/v1/domains/$IID --label delete-inc7 --timeout 600 | sbrief 500
EVERY=10 delwatch backup $IID 240
for n in main; do echo "-- $n"; grun $n g_peerlog.sh -5min "" 4; done
digz inc7.test
echo "== c1preB done $(ts)"
