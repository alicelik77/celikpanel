source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/env.sh
BUMP=131 bash $A/build.sh refs/heads/fix/a128-lab v0.1.0-alpha.131 > $B/out-build131.out 2>&1
grep -E "^commit|^lab bump|EXIT|budget|FAILED|\"commit\"|\"sha256\"" $B/out-build131.out
C=$(sed -n 's/^lab bump commit //p' $B/out-build131.out | head -1); echo $C > $B/c131
bash $A/portal.sh $C > $B/portal131.out 2>&1; echo "portal131 exit $?"; grep -E "^(version|commit|sequence)=" $B/portal131.out

