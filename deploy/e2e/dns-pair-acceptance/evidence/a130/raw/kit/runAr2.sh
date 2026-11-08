A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit
export S=ar IPU=192.0.3.222
bash $A/k5b.sh > /dev/null 2>&1
bash $A/k7sftp.sh add anna-nokey > /dev/null 2>&1
bash $A/k7sftp.sh vsftpd > /dev/null 2>&1
bash $A/k7sftp.sh add anna-up > /dev/null 2>&1
cat /var/tmp/a130o/logs/C5web-ar.log | sed -n "/5b.1/,\$p" | cut -c1-500; cat /var/tmp/a130o/logs/C7sftp-ar.log | sed -n "/7.add anna-nokey/,\$p" | cut -c1-900
