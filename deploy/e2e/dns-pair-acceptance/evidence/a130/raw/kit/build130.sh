# alpha.130 round: the candidate release/alpha130 with the release identity bump (lab label 130). Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/env.sh
REF=${REF:-refs/remotes/origin/release/alpha130}
BUMP=130 PREVC=$C129 bash $A/build.sh $REF v0.1.0-alpha.130 > $B/out-build130.out 2>&1
grep -E "^commit|^lab bump|EXIT|budget|FAILED|\"commit\"|\"sha256\"" $B/out-build130.out
C10=$(sed -n "s/^lab bump commit //p" $B/out-build130.out | head -1); echo $C10 > $B/c130
bash $A/portal.sh $C10 > $B/portal130.out 2>&1; echo "portal130 exit $?"; grep -E "^(version|commit|sequence)=" $B/portal130.out
bash $A/portalF.sh add v0.1.0-alpha.130 | tail -2
echo "build130 done $(ts)"
