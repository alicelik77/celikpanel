A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit
source $A/env.sh
mv $LOGS/setup127-d12.log $LOGS/setup127-d12-attempt2-harness.log; cp $LOGS/install-d12v-main.log $LOGS/install-d12v-attempt2-harness.log
pkill -f "setup127.sh" ; bash $A/rmcell.sh d12v
CELLX=d12v S=d12 OS=debian12 IPU=192.0.3.224 bash $A/setup127.sh > /dev/null 2>&1
tail -25 $LOGS/setup127-d12.log
