# alpha.129 phase 3a: cases 3, 6, 5, 4 on the pair. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit
source $A/env.sh
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source $A/lib.sh; source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
echo "== PostgreSQL on the main from Components $(ts)"; installc main postgresql
bash $A/k3dbu.sh mysql > /dev/null 2>&1; bash $A/k3dbu.sh postgresql > /dev/null 2>&1; bash $A/k3wp.sh > /dev/null 2>&1; echo "case 3 done $(ts)"
bash $A/k6tok.sh > /dev/null 2>&1; echo "case 6 done $(ts)"
bash $A/k5repause.sh > /dev/null 2>&1; echo "case 5 done $(ts)"
bash $A/k4remote.sh mysql setup open restart checks fail2ban pass suspend plan last > /dev/null 2>&1; echo "case 4 my done $(ts)"
bash $A/k4remote.sh postgresql setup open restart checks fail2ban pass last > /dev/null 2>&1; echo "case 4 pg done $(ts)"
bash $A/k4remote.sh mysql reboot flush > /dev/null 2>&1; echo "case 4 reboot/flush done $(ts)"
