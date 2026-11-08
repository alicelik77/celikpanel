# alpha.130 round: lab label 131 = release/alpha130 899b40b1f (final candidate) with a release identity bump. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/env.sh
BUMP=131 PREVC=2739ae3e9e2755676b4508002d0ed641d04dbdc5 bash $A/build.sh refs/remotes/origin/release/alpha130 v0.1.0-alpha.131 > $B/out-build131.out 2>&1
grep -E "^commit|^lab bump|EXIT|budget|FAILED|\"commit\"|\"sha256\"" $B/out-build131.out
C=$(sed -n "s/^lab bump commit //p" $B/out-build131.out | head -1); echo $C > $B/c131
bash $A/portal.sh $C > $B/portal131.out 2>&1; echo "portal131 exit $?"; grep -E "^(version|commit|sequence)=" $B/portal131.out
bash $A/portalF.sh add v0.1.0-alpha.131 | tail -1
echo "build131 done $(ts)"
