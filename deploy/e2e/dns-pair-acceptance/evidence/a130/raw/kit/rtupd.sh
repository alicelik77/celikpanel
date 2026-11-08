# alpha.130 retest: 129 -> lab 132 (new head) through the panel's update, pressed once on u1u and d1u. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit
source $A/env.sh
bash $A/portalF.sh latest v0.1.0-alpha.132
CELL=u1u IP=192.0.3.200 Z=u1.test bash $A/upd.sh main > /dev/null 2>&1 &
CELL=d1u IP=192.0.3.201 Z=d1.test bash $A/upd.sh main > /dev/null 2>&1 &
wait
for c in u1u d1u; do echo "######## $c"; grep -E "^  [0-9]+s|== after|schema_version|failed" /var/tmp/a130o/logs/C2-$c-main.log | cut -c1-260 | tail -8; done
