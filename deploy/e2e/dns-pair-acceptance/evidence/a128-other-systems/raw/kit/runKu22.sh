A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit
for f in C3php C5web C6db C7sftp C4mail; do for g in /var/tmp/a129o/logs/$f-u22*.log; do [ -f "$g" ] && mv "$g" "${g%.log}-at127-killed.log"; done; done
bash $A/runK.sh u22 192.0.3.221
