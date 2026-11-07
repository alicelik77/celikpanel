# stop one lab guest and remove its disk and state. usage: rmcell.sh CELL [NODE]. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/env.sh
python3 -I $L/lab.py stop $1 ${2:-main} 2>/dev/null | tr -d '\n '; echo
rm -rf /var/tmp/cp-beta1/state/$1 /var/tmp/cp-beta1/lab/cells/$1 /var/tmp/cp-beta1/evidence/$1
echo "qemu: $(pgrep -af qemu-system | grep -o 'name [^ ]*' | tr '\n' ' ')"; free -m | sed -n 2p
