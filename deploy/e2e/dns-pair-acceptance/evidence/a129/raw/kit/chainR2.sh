A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit
source $A/env.sh
( bash $A/run7_f9.sh > $A/run7_f9.out 2>&1; bash $A/c8rb_f9.sh > $A/c8rb_f9.out 2>&1; sleep 5; bash $A/c8b_f9.sh > $A/c8b_f9.out 2>&1; python3 -I $L/lab.py stop f9 main > /dev/null 2>&1; echo "f9 stopped" ) &
bash $A/runP_r9.sh > $A/runP_r9.out 2>&1
wait
sleep 20
bash $A/c1pre_r9.sh > $A/c1pre_r9.out 2>&1
bash $A/c1upd_r9.sh > $A/c1upd_r9.out 2>&1
for c in r2a r2mail r2git r2b r2sftp; do bash $A/$c.sh > $A/$c.out 2>&1; done
bash $A/r2idle.sh > $A/r2idle.out 2>&1
bash $A/r2reboot.sh > $A/r2reboot.out 2>&1
echo chainR2 done
