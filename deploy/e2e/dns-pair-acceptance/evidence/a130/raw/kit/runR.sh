# alpha.130 retest cells u1 (Ubuntu 24.04) and d1 (Debian 13), both at the published 129. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit
source $A/env.sh
bash $A/portalF.sh latest v0.1.0-alpha.129 > /dev/null
S=u1 OS=ubuntu2404 IPU=192.0.3.200 bash $A/setup129s.sh > /dev/null 2>&1 &
sleep 30; S=d1 OS=debian13 IPU=192.0.3.201 bash $A/setup129s.sh > /dev/null 2>&1 &
wait
for s in u1 d1; do echo "######## $s"; cut -c1-300 /var/tmp/a130o/logs/setup129-$s.log | tail -25; done
