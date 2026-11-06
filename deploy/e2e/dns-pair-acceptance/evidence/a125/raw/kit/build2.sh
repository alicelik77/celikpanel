# rebuild the alpha.125 candidate (3f58cd02c, the lab fix) and its lab 126 label; refresh the network portal. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a125acc/kit/env.sh
echo "start $(ts)"
mv $B/portal-v0.1.0-alpha.125 $B/portal-v0.1.0-alpha.125-bf0a5b9ee; mv $B/portal-v0.1.0-alpha.126 $B/portal-v0.1.0-alpha.126-3b25dad42
bash $A/build.sh refs/remotes/origin/release/alpha125 v0.1.0-alpha.125 > $B/out-build125b.out 2>&1
grep -E "^commit|EXIT|budget|\"commit\"|\"sha256\"" $B/out-build125b.out
bash $A/portal.sh $C125 > $B/portal125b.out 2>&1; echo "portal125b exit $?"; grep -E "^(version|commit|sequence)=" $B/portal125b.out
BUMP=1 bash $A/build.sh refs/remotes/origin/release/alpha125 v0.1.0-alpha.126 > $B/out-build126b.out 2>&1
grep -E "^commit|^lab bump|EXIT|\"sha256\"" $B/out-build126b.out
C=$(sed -n 's/^lab bump commit //p' $B/out-build126b.out | head -1)
bash $A/portal.sh $C > $B/portal126b.out 2>&1; echo "portal126b exit $?"; grep -E "^(version|commit|sequence)=" $B/portal126b.out
PREVV=v0.1.0-alpha.124 NEXTV=v0.1.0-alpha.125 bash $A/portalF.sh up v0.1.0-alpha.125 > $A/portalnet-up2.out 2>&1; tail -1 $A/portalnet-up2.out
echo "done $(ts)"
