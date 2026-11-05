A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a124lab/kit
bash $A/R7.sh > $A/R7r.out 2>&1
bash $A/R3b.sh > $A/R3b.out 2>&1
bash $A/R5b.sh > $A/R5b.out 2>&1
echo "runM2 done $(date -u +%T)"
