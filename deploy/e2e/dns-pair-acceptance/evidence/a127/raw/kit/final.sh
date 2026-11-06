export CELL=r7 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a127lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/final-r7.log) 2>&1
for n in main; do echo "== $n $(ts)"; grun $n g_eg2.sh; done
