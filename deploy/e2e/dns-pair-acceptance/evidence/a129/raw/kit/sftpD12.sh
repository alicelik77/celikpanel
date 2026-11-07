A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit
export CELLX=d12v S=d12 IPU=192.0.3.224
bash $A/k7sftp.sh add dora-nokey > /dev/null 2>&1
bash $A/k7sftp.sh pw yes > /dev/null 2>&1
bash $A/k7sftp.sh add dora-up > /dev/null 2>&1
bash $A/k7sftp.sh pw clear > /dev/null 2>&1
bash $A/k7sftp.sh state > /dev/null 2>&1
echo sftp d12 done
