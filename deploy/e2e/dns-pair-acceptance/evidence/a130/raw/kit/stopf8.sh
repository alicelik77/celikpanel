source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/env.sh
python3 -I $L/lab.py stop f8 main | tr -d '\n '; echo
pgrep -af qemu-system | grep -o "name [^ ]*"; free -m | sed -n 2p
