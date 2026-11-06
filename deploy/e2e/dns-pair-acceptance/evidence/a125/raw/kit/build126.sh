source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a125acc/kit/env.sh
echo "start $(ts)"
BUMP=1 bash $A/build.sh refs/remotes/origin/release/alpha125 v0.1.0-alpha.126 > $B/out-build126.out 2>&1
grep -E "^commit|^lab bump|EXIT|budget|\"commit\"|\"sha256\"" $B/out-build126.out
C=$(sed -n 's/^lab bump commit //p' $B/out-build126.out | head -1)
bash $A/portal.sh $C > $B/portal126.out 2>&1; echo "portal126 exit $?"; grep -E "^(version|commit|sequence)=" $B/portal126.out
echo "done $(ts)"
