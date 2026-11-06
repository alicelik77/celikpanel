source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a127lab/kit/env.sh
BUMP=130 bash $A/build.sh refs/heads/release/alpha127 v0.1.0-alpha.130 > $B/out-build130fix.out 2>&1
grep -E "^commit|^lab bump|EXIT|budget|FAILED|\"commit\"|\"sha256\"" $B/out-build130fix.out
C=$(sed -n 's/^lab bump commit //p' $B/out-build130fix.out | head -1); echo $C > $B/c130fix
bash $A/portal.sh $C > $B/portal130fix.out 2>&1; echo "portal130fix exit $?"; grep -E "^(version|commit|sequence)=" $B/portal130fix.out
bash $A/portalF.sh add v0.1.0-alpha.130 | tail -2
