pkill -f p129b.sh; pkill -f runP129.sh; pkill -f pairX.sh; sleep 2; pgrep -af "setupV|pairX|runP129|p129b" | grep -v pgrep | cut -c1-120
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/env.sh
cp $LOGS/pair-r9.log $LOGS/pair-r9-attempt1-harness.log; cp $LOGS/install-r9-main.log $LOGS/install-r9-main-attempt1-harness.log; rm -f $LOGS/pair-r9.log
bash $A/rmcell.sh r9 main; bash $A/rmcell.sh r9 backup; rm -rf /var/tmp/cp-beta1/state/r9 $B/t-r9 $B/wiz-r9
