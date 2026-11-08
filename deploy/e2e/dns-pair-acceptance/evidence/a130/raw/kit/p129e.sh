# alpha.129 phase 3c: case 6 (b), Fail2ban, case 4 on the main (MariaDB, PostgreSQL), reboot/flush. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit
source $A/env.sh
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source $A/lib.sh; source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
bash $A/k6b.sh > /dev/null 2>&1; echo "case 6b done $(ts)"
exec > >(tee -a $LOGS/C4remote-r9-mysql.log) 2>&1; echo "== Fail2ban from Components (the rule named it) $(ts)"; installc main fail2ban; exec > /dev/tty 2>&1 || true
bash $A/k4remote.sh mysql restart checks fail2ban pass suspend plan last > /dev/null 2>&1; echo "case 4 my done $(ts)"
bash $A/k4remote.sh postgresql setup open restart checks fail2ban pass last > /dev/null 2>&1; echo "case 4 pg done $(ts)"
bash $A/k4remote.sh mysql reboot flush > /dev/null 2>&1; echo "case 4 reboot/flush done $(ts)"
