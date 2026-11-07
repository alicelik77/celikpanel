# alpha.127 case 4: the domain page's reads on the joined server (pg7.test), after warm-up. Lab only.
export CELL=r7 IP=192.0.3.191
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
exec > >(tee -a $LOGS/C4-r7-backup.log) 2>&1
echo "== joined server (boston), pg7.test $(date -u +%T)"
python3 -I $A/perf7.py $L r7 backup 1 joined
echo "== main (frankfurt), site7.test, for comparison $(date -u +%T)"
python3 -I $A/perf7.py $L r7 main 4 main
