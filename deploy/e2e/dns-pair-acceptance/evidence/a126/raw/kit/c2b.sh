# case 4 UI: the reseller's Service plans tab (126), EN and TR. Lab only.
export CELL=p6 Z=p6.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a126lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh
exec > >(tee -a $LOGS/C4ui-$CELL.log) 2>&1
echo "== case 4 UI: res6 opens Service plans, presses Add plan $(ts)"
timeout 600 python3 -I $H/shots.py $CELL main c4-plans3 $A/planui.json --langs ${LANGS:-en,tr} --widths desktop --user res6 --pwfile $T/cred-res6.json | python3 $A/shotsum.py
