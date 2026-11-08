source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/env.sh
BUMP=128 bash $A/build.sh refs/heads/release/alpha127 v0.1.0-alpha.128 > $B/out-build128fix.out 2>&1
grep -E "^commit|^lab bump|EXIT|budget|FAILED|\"commit\"|\"sha256\"" $B/out-build128fix.out
C=$(sed -n 's/^lab bump commit //p' $B/out-build128fix.out | head -1); echo $C > $B/c128fix
bash $A/portal.sh $C > $B/portal128fix.out 2>&1; echo "portal128fix exit $?"; grep -E "^(version|commit|sequence)=" $B/portal128fix.out
bash $A/portalF.sh add v0.1.0-alpha.128 | tail -2
