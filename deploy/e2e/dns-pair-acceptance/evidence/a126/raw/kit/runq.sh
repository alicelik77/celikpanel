export CELL=p6 Z=p6.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a126lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh
exec > >(tee -a $LOGS/C2-$CELL.log) 2>&1
echo "== case 2: what the three DELETE presses of api6.test ran on the joined server $(ts)"
grun ${NODE:-backup} "$@"
