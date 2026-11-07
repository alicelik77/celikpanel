source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/env.sh
mv $LOGS/C6tok-r9.log $LOGS/C6tok-r9-run1-named-stopped.log
bash $A/k6tok.sh > /dev/null 2>&1; cat $LOGS/C6tok-r9.log | cut -c1-500
