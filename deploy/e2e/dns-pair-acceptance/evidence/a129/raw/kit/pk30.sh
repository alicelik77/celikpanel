source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/env.sh
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source $A/lib.sh; source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
echo "== lab: cloud-init hang after the 4.9 reboot keeps systemd starting; the Agent refused every apply (systemd is not ready)" >> $LOGS/C4remote-r9-mysql.log; grun main g_drj.sh 19:14:00 19:28:00 >> $LOGS/C4remote-r9-mysql.log; grun main g_ciun.sh | tee -a $LOGS/C4remote-r9-mysql.log
