# alpha.129 phase 2 (again for the pair after the harness flake): pair at 128, content, updates. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit
source $A/env.sh
bash $A/runP129.sh > $LOGS/runP129.out 2>&1; echo "pair installed $(ts)"; python3 $H/insum.py $LOGS/install-r9-main.log main | head -2 | cut -c1-160; python3 $H/insum.py $LOGS/install-r9-backup.log backup | head -1 | cut -c1-160; tail -3 $LOGS/runP129.out
bash $A/c1pre129.sh > /dev/null 2>&1; tail -10 $LOGS/C1pre-r9.log | cut -c1-220
CELLX=u22u S=u22 IPU=192.0.3.221 bash $A/upd128.sh > /dev/null 2>&1 &
bash $A/c1upd129.sh > /dev/null 2>&1; wait
echo "######## pair update"; grep -E "s 200|schema_version|differences|^<|^>|failed|: [0-9]{3}$|snapshot|== " $LOGS/C1upd-r9.log | cut -c1-260
echo "######## u22 update"; grep -E "s 200|schema_version|differences|^<|^>|failed|: [0-9]{3}$|snapshot|== " $LOGS/C2upd-u22.log | cut -c1-260
echo "phase2 done $(ts)"
