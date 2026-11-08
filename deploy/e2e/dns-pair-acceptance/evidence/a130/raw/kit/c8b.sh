export CELL=f8 Z=f8.test IP=192.0.3.194
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/C8rb-f8-main.log) 2>&1
echo "== c8b: the journal of the refused attempt and its rollback (read-only, captured after) $(ts)"
grun main g_rb.sh 04:25:40 04:28:14
grun main g_upd.sh -10min | head -4
