A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a124lab/kit
source $A/env.sh
bash $A/R6.sh > $A/R6.out 2>&1
python3 -I $L/lab.py stop p124 backup | tr -d '\n '; echo " backup stopped $(date -u +%T)"
bash $A/R5.sh > $A/R5.out 2>&1
bash $A/R3.sh > $A/R3.out 2>&1
bash $A/R4.sh > $A/R4.out 2>&1
bash $A/R7.sh > $A/R7.out 2>&1
echo "runM done $(date -u +%T)"
