source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/env.sh
python3 -I $L/lab.py stop a118 main | tr -d '\n '; echo
rm -rf /var/tmp/cp-beta1/state/a118 /var/tmp/cp-beta1/evidence/a118 /var/tmp/cp-beta1/lab/cells/a118
bash $A/singleA.sh > $A/singleA2.out 2>&1
