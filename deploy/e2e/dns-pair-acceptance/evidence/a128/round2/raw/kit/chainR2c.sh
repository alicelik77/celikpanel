A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit
bash $A/c1pre_r9.sh > $A/c1pre_r9.out 2>&1
bash $A/c1upd_r9.sh > $A/c1upd_r9.out 2>&1
for c in r2a r2mail r2git r2b r2sftp; do bash $A/$c.sh > $A/$c.out 2>&1; done
bash $A/r2idle.sh > $A/r2idle.out 2>&1
bash $A/r2reboot.sh > $A/r2reboot.out 2>&1
echo chainR2c done
