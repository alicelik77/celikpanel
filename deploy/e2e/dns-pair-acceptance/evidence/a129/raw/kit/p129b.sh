# alpha.129 round phase 2: pair (Debian 13 main PowerDNS+mail, Ubuntu 24.04 joined BIND) at 128 and Ubuntu 22.04 at 127; content; updates to 129 pressed once. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit
source $A/env.sh
bash $A/runP129.sh > $LOGS/runP129.out 2>&1 &
sleep 60; CELLX=u22u S=u22 OS=ubuntu2204 IPU=192.0.3.221 VER=v0.1.0-alpha.127 MEM=3072 bash $A/setupV.sh > /dev/null 2>&1 &
wait; echo "pair+u22 installed $(ts)"; tail -4 $LOGS/runP129.out; tail -12 $LOGS/pair-r9.log | cut -c1-200; tail -6 $LOGS/setup127-u22.log | cut -c1-200
bash $A/c1pre129.sh > /dev/null 2>&1; tail -12 $LOGS/C1pre-r9.log | cut -c1-220
CELLX=u22u S=u22 IPU=192.0.3.221 bash $A/upd128.sh > /dev/null 2>&1 &
bash $A/c1upd129.sh > /dev/null 2>&1; wait
echo "######## pair update"; grep -E "s 200|schema_version|differences|^<|^>|failed|: [0-9]{3}$|snapshot|== " $LOGS/C1upd-r9.log | cut -c1-260
echo "######## u22 update"; grep -E "s 200|schema_version|differences|^<|^>|failed|: [0-9]{3}$|snapshot|== " $LOGS/C2upd-u22.log | cut -c1-260
echo "phase2 done $(ts)"
