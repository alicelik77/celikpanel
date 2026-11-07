# alpha.129 phase 3d: resume cust1, then case 4 plan/last on MariaDB, PostgreSQL on the main, reboot/flush. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit
source $A/env.sh
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source $A/lib.sh; source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec >> $LOGS/C4remote-r9-mysql.log 2>&1
echo "== 4.6b the subscription resumed (lab: the first script asked unsuspend) $(ts)"; adm POST /api/v1/subscriptions/$(cat $T/sub-cust1.id)/suspension "{\"action\":\"resume\",\"reason\":\"lab\"}" --label resume --timeout 300 | sbrief 300
sleep 30
bash $A/k4remote.sh mysql checks plan last
bash $A/k4remote.sh postgresql setup open restart checks fail2ban pass last
bash $A/k4remote.sh mysql reboot flush
echo "== p129g done $(ts)"
