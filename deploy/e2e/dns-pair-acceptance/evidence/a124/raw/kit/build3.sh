source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a124lab/kit/env.sh
echo "start $(ts)"
bash $A/build.sh refs/heads/release/alpha124 v0.1.0-alpha.124 > $B/out-build124c.out 2>&1
grep -E "^commit|EXIT|budget|\"commit\"|\"sha256\"" $B/out-build124c.out
C=$(sed -n 's/^commit //p' $B/out-build124c.out | head -1)
mv $B/portal-v0.1.0-alpha.124 $B/portal-v0.1.0-alpha.124-9561b2df9 2>/dev/null
bash $A/portal.sh $C > $B/portal124c.out 2>&1; echo "portal124c exit $?"; grep -E "^(version|commit|sequence)=" $B/portal124c.out
echo "done $(ts)"
