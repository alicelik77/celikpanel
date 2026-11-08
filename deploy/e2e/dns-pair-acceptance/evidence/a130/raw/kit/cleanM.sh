source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/env.sh
E="/mnt/c/CELIKBROS PROJECTS/celikpanel/.claude/worktrees/lab114/deploy/e2e/dns-pair-acceptance/evidence/a128-other-systems/raw"
cp $LOGS/C4mail-d12.log "$E/D4-mail-round-d12-lab129.log"; cp $LOGS/setup127-d12.log "$E/D4-setup-d12-lab129.log"; grep -E "^commit|^lab bump|\"sha256\"" $B/out-build129.out > "$E/D4-build129.txt"; cp $A/runM.sh $A/setupV.sh "$E/kit/"
bash $A/portalF.sh down | tail -1
python3 -I $L/lab.py stop m12 main | tr -d "\n "; echo; rm -rf /var/tmp/cp-beta1/state/m12 /var/tmp/cp-beta1/lab/cells/m12 /var/tmp/cp-beta1/evidence/m12
python3 -I $L/lab.py stop beta1-lab labnet --keep | tr -d "\n "; echo
C9=$(cat $B/c129); rm -rf /var/tmp/cp-pair-accept/dist/$C9-acceptance-license /var/tmp/a130o; echo "qemu: $(pgrep -c qemu-system)"
