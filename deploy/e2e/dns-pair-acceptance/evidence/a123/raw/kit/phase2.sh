# Rebuild for 894fd63c9 (after the Ubuntu 22.04 BIND fix): stop wave-1 VMs, build dist, portal. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a123/kit/env.sh
for c in u22 d12; do python3 -I $L/lab.py stop $c main | tr -d '\n '; echo; done
cp -a $B/portal-v0.1.0-alpha.123/get.sh.diff $LOGS/get.sh.diff-e99 2>/dev/null
cp /var/tmp/cp-pair-accept/dist/$C0-acceptance-license/dist.json $LOGS/dist-e99.json
bash $A/build.sh refs/heads/release/alpha123 v0.1.0-alpha.123 > $B/out-build123b.out 2>&1; grep -E "^commit|EXIT|budget|\"commit\"|\"sha256\"" $B/out-build123b.out
bash $A/portal.sh $C118 > $B/portal123b.out 2>&1; echo "portal exit $?"; grep -E "^(version|commit|sequence)=" $B/portal123b.out; sed -n 1,6p $B/portal123b.out
rm -rf /var/tmp/cp-pair-accept/dist/$C0-acceptance-license $B/repo-v0.1.0-alpha.123/.git/objects/pack/*.keep 2>/dev/null
free -g | head -2; echo "phase2 done $(ts)"
