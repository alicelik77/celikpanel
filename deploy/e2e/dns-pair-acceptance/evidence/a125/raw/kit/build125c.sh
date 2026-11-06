# rebuild the alpha.125 candidate (1600949e8, both lab fixes) and its lab 126 label; refresh the network portal. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a125acc/kit/env.sh
echo "start $(ts)"
mv $B/portal-v0.1.0-alpha.125 $B/portal-v0.1.0-alpha.125-3f58cd02c; mv $B/portal-v0.1.0-alpha.126 $B/portal-v0.1.0-alpha.126-64c22f5de
bash $A/build.sh refs/remotes/origin/release/alpha125 v0.1.0-alpha.125 > $B/out-build125c.out 2>&1
grep -E "^commit|EXIT|budget|\"sha256\"" $B/out-build125c.out
bash $A/portal.sh $C125 > $B/portal125c.out 2>&1; echo "portal125c exit $?"; grep -E "^(version|commit|sequence)=" $B/portal125c.out
BUMP=1 bash $A/build.sh refs/remotes/origin/release/alpha125 v0.1.0-alpha.126 > $B/out-build126c.out 2>&1
grep -E "^commit|^lab bump|EXIT|\"sha256\"" $B/out-build126c.out
C=$(sed -n 's/^lab bump commit //p' $B/out-build126c.out | head -1)
bash $A/portal.sh $C > $B/portal126c.out 2>&1; echo "portal126c exit $?"; grep -E "^(version|commit|sequence)=" $B/portal126c.out
PREVV=v0.1.0-alpha.124 NEXTV=v0.1.0-alpha.125 bash $A/portalF.sh up v0.1.0-alpha.125 > $A/portalnet-up3.out 2>&1; tail -1 $A/portalnet-up3.out
# the broken Debian 12 guest (panel down, four-row ledger left by the old helper) is replaced by a fresh one
python3 -I $L/lab.py stop d5 main | tr -d '\n '; echo
rm -rf /var/tmp/cp-beta1/state/d5 /var/tmp/cp-beta1/lab/cells/d5 $B/t-d5 $B/wiz-d5
echo "done $(ts)"
