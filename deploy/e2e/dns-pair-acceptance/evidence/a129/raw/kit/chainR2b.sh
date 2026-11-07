A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit
source $A/env.sh
pkill -f "a129lab/kit/chainR2.sh"; pkill -f "a129lab/kit/runP_r9.sh"; pkill -f "a129lab/kit/pairX.sh"; sleep 2
CELL=r9 MIP=192.0.3.190 BIP=192.0.3.191 Z=r9.test MKIND=pair-main BKIND=pair-backup-web BENGINE=bind bash $A/pairR.sh > $A/pairR_r9.out 2>&1
sleep 20
bash $A/c1pre_r9.sh > $A/c1pre_r9.out 2>&1
bash $A/c1upd_r9.sh > $A/c1upd_r9.out 2>&1
for c in r2a r2mail r2git r2b r2sftp; do bash $A/$c.sh > $A/$c.out 2>&1; done
bash $A/r2idle.sh > $A/r2idle.out 2>&1
bash $A/r2reboot.sh > $A/r2reboot.out 2>&1
echo chainR2b done
