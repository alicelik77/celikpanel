A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit
( export CELLX=d12v S=d12 IPU=192.0.3.224; bash $A/k4c.sh > /dev/null 2>&1 ) &
( export S=u22 IPU=192.0.3.221; bash $A/k4c.sh > /dev/null 2>&1 ) &
wait
for s in d12 u22; do echo "## $s"; sed -n "/4.9 box2/,\$p" /var/tmp/a130o/logs/C4mail-$s.log | grep -v "235 2.7.0" | cut -c1-260; done
