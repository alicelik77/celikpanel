# Rebuild for 9b69021a9 and rerun case 4 on a fresh Ubuntu 24.04 (BIND; intermittent panel DB lock hold). Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a123/kit/env.sh
for c in d13 u24; do python3 -I $L/lab.py stop $c main | tr -d '\n '; echo; done
cp /var/tmp/cp-pair-accept/dist/$C1-acceptance-license/dist.json $LOGS/dist-894fd63c9.json
bash $A/build.sh refs/heads/release/alpha123 v0.1.0-alpha.123 > $B/out-build123c.out 2>&1; grep -E "^commit|EXIT|budget|\"commit\"|\"sha256\"" $B/out-build123c.out
bash $A/portal.sh $C118 > $B/portal123c.out 2>&1; echo "portal exit $?"; grep -E "^(version|commit|sequence)=" $B/portal123c.out
rm -rf /var/tmp/cp-pair-accept/dist/$C1-acceptance-license
echo "phase3 build done $(ts)"
CELL=u24b IP=192.0.3.193 Z=u24b.test OS=ubuntu2404 SHORT=boston DNSENGINE=bind HOLDDB=1 HOLDPAT="12 7 1" HOLDAFTER=3 WATCHN=60 WATCHS=2 bash $A/singleX.sh > $A/singleU24b.out 2>&1
lab ssh u24b main -- "sudo -n journalctl -u celikpanel-panel --since '-25min' -o short-precise --no-pager" 2>/dev/null | grep -iE 'busy|locked|asked again' | cut -c1-260 | head -30 > $LOGS/holddb2-journal-u24b.txt
echo "phase3 done $(ts)"
