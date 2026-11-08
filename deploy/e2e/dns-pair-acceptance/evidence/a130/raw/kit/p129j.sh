A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit
source $A/env.sh
echo "== 4.7 third run: systemd ready again (cloud-init job stopped)" >> $LOGS/C4remote-r9-mysql.log
bash $A/k4remote.sh mysql plan > /dev/null 2>&1
echo "== 4.7 second run: systemd ready again" >> $LOGS/C4remote-r9-postgresql.log
bash $A/k4remote.sh postgresql plan > /dev/null 2>&1
sed -n "/4.7 third run/,\$p" $LOGS/C4remote-r9-mysql.log | cut -c1-250; sed -n "/4.7 second run/,\$p" $LOGS/C4remote-r9-postgresql.log | cut -c1-250
