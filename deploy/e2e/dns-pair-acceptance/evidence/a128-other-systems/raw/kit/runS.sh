# the three 127 servers with setup and content at once. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit
S=d12 OS=debian12 IPU=192.0.3.220 bash $A/setup127.sh > /dev/null 2>&1 &
sleep 20; S=u22 OS=ubuntu2204 IPU=192.0.3.221 bash $A/setup127.sh > /dev/null 2>&1 &
sleep 20; S=ar OS=arch IPU=192.0.3.222 bash $A/setup127.sh > /dev/null 2>&1 &
wait
for s in d12 u22 ar; do echo "######## $s"; cat /var/tmp/a129o/logs/setup127-$s.log; done
