source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/env.sh
BUMP=129 PREVC=$C128 bash $A/build.sh refs/remotes/origin/fix/a129-lab v0.1.0-alpha.129 > $B/out-build129.out 2>&1
grep -E "^commit|^lab bump|EXIT|budget|FAILED|\"commit\"|\"sha256\"" $B/out-build129.out
C9=$(sed -n "s/^lab bump commit //p" $B/out-build129.out | head -1); echo $C9 > $B/c129
bash $A/portal.sh $C9 > $B/portal129.out 2>&1; echo "portal129 exit $?"; grep -E "^(version|commit|sequence)=" $B/portal129.out
bash $A/portalF.sh add v0.1.0-alpha.129 | tail -2
