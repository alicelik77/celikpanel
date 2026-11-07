A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit
S=$1 IPU=$2 bash $A/upd128.sh > /dev/null 2>&1
cat /var/tmp/a129o/logs/C2upd-$1.log
