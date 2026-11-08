A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit
source $A/env.sh
mv $LOGS/C3dbu-r9-postgresql.log $LOGS/C3dbu-r9-postgresql-run1-scriptbug.log
DBS=v bash $A/k3dbu.sh postgresql > /dev/null 2>&1; cat $LOGS/C3dbu-r9-postgresql.log | cut -c1-260
