# cases 3, 5, 6, 7 (state), 4 on one updated server. usage: runK.sh S IPU. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit
export S=$1 IPU=$2
bash $A/k3php.sh > /dev/null 2>&1
bash $A/k5web.sh > /dev/null 2>&1; bash $A/k5b.sh > /dev/null 2>&1
bash $A/k6db.sh main site1.id mysql > /dev/null 2>&1
bash $A/k7sftp.sh state > /dev/null 2>&1
bash $A/k4mail.sh > /dev/null 2>&1
echo "runK $1 done"
