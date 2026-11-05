A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a123/kit
( bash $A/runW2b.sh > $A/runW2b.out 2>&1 ) &
( bash $A/R3D13r.sh > $A/R3D13r.out 2>&1 ) &
wait; echo "run4 done $(date -u +%T)"
