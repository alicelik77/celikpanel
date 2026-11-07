A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit
bash $A/c3pre.sh > $A/c3pre.out 2>&1 &
bash $A/c9pre.sh > $A/c9pre.out 2>&1 &
wait
echo "runpre done $(date -u +%T)"
