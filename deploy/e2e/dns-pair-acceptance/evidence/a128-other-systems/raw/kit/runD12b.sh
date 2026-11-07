A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit
export CELLX=d12v S=d12 IPU=192.0.3.224
bash $A/k4b.sh > /dev/null 2>&1
bash $A/sftpD12.sh
sed -n "/4.4 the pause/,\$p" /var/tmp/a129o/logs/C4mail-d12.log | grep -v "235 2.7.0" | cut -c1-300
