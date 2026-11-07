source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/env.sh
BUMP=129 bash $A/build.sh refs/heads/fix/a128-lab v0.1.0-alpha.129 > $B/out-build129fix.out 2>&1
grep -E "^commit|^lab bump|EXIT|budget|FAILED|\"commit\"|\"sha256\"" $B/out-build129fix.out
C=$(sed -n 's/^lab bump commit //p' $B/out-build129fix.out | head -1); echo $C > $B/c129fix
bash $A/portal.sh $C > $B/portal129fix.out 2>&1; echo "portal129fix exit $?"; grep -E "^(version|commit|sequence)=" $B/portal129fix.out
bash $A/portalF.sh add v0.1.0-alpha.129 | tail -2
