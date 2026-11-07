A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit
export CELLX=d12v S=d12 IPU=192.0.3.224
bash $A/upd128.sh > /dev/null 2>&1
grep -q "\"version\": \"v0.1.0-alpha.128\"" /var/tmp/a129o/logs/C2upd-d12.log || { echo "update did not reach 128"; exit 1; }
bash $A/runK.sh d12 192.0.3.224
