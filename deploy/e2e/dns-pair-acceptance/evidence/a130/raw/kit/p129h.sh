# alpha.129: case 4.7 again (lab script fixed: the plan held "all permissions", so the first run withdrew nothing), then p129f. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit
source $A/env.sh
echo "== 4.7 rerun: the first run left all_permissions on (lab script), nothing was withdrawn" >> $LOGS/C4remote-r9-mysql.log
bash $A/k4remote.sh mysql plan > /dev/null 2>&1
echo "== 4.7 run with the explicit permission list" >> $LOGS/C4remote-r9-postgresql.log
bash $A/k4remote.sh postgresql plan > /dev/null 2>&1
echo "== p129h plan done $(date -u +%T)" >> $LOGS/C4remote-r9-mysql.log
bash $A/p129f.sh > $LOGS/p129f.out 2>&1
echo "== p129h done $(date -u +%T)" >> $LOGS/p129f.out
