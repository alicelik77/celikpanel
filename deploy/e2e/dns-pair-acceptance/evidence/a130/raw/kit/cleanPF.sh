source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/env.sh
E="/mnt/c/CELIKBROS PROJECTS/celikpanel/.claude/worktrees/lab114/deploy/e2e/dns-pair-acceptance/evidence/a128-other-systems/raw"
cp $LOGS/D4-pf135-old.log $LOGS/D4-pf135-new.log "$E/"; cp $A/g_pf135.sh $A/runPF.sh "$E/kit/"
python3 -I $L/lab.py stop pf12 main | tr -d "\n "; echo; rm -rf /var/tmp/cp-beta1/state/pf12 /var/tmp/cp-beta1/lab/cells/pf12 /var/tmp/cp-beta1/evidence/pf12
python3 -I $L/lab.py stop beta1-lab labnet --keep | tr -d "\n "; echo
rm -rf /var/tmp/a130o /var/tmp/a129-pf; echo "qemu: $(pgrep -c qemu-system)"
