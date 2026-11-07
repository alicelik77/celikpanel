source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/env.sh
pkill -f "a129lab/kit/runP.sh"; pkill -f "a129lab/kit/pairX.sh"; pkill -f getsh_install.py; sleep 2
for n in main backup; do python3 -I $L/lab.py stop r8 $n 2>/dev/null | tr -d '\n '; echo; done
rm -rf /var/tmp/cp-beta1/state/r8 /var/tmp/cp-beta1/lab/cells/r8 /var/tmp/cp-beta1/evidence/r8 $B/logs/* $B/t-r8 $B/wiz-r8
pgrep -af qemu-system | grep -o "name [^ ]*"
