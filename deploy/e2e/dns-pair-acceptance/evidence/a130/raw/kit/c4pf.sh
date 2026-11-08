export CELL=r8 Z=r8.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/C4mail-$CELL.log) 2>&1
echo "== 4.2' postfwd on Debian 13 (read-only diagnosis) $(ts)"; grun main g_pf4.sh
