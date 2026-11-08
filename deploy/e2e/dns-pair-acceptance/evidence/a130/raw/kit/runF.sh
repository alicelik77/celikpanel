# the three fresh installs at once (3 guests + labnet). Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit
S=d12 OS=debian12 IPF=192.0.3.210 bash $A/fresh.sh > /dev/null 2>&1 &
S=u22 OS=ubuntu2204 IPF=192.0.3.211 bash $A/fresh.sh > /dev/null 2>&1 &
S=ar OS=arch IPF=192.0.3.212 bash $A/fresh.sh > /dev/null 2>&1 &
wait
for s in d12 u22 ar; do echo "######## $s"; cat /var/tmp/a130o/logs/C1fresh-$s.log; done
