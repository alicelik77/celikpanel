# alpha.130 round: lab label 132 = fix/a130-pg-lab2 6fc1bbba1 (retest) with a release identity bump. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/env.sh
BUMP=132 PREVC=899b40b1f34331dc42b0e96a0de030c93e9d01d2 bash $A/build.sh refs/remotes/origin/fix/a130-pg-lab2 v0.1.0-alpha.132 > $B/out-build132.out 2>&1
grep -E "^commit|^lab bump|EXIT|budget|FAILED|\"commit\"|\"sha256\"" $B/out-build132.out
C=$(sed -n "s/^lab bump commit //p" $B/out-build132.out | head -1); echo $C > $B/c132
bash $A/portal.sh $C > $B/portal132.out 2>&1; echo "portal132 exit $?"; grep -E "^(version|commit|sequence)=" $B/portal132.out
bash $A/portalF.sh add v0.1.0-alpha.132 | tail -1
echo "build132 done $(ts)"
