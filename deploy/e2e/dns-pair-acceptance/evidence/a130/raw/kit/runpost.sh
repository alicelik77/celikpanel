A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit
bash $A/c3post.sh > $A/c3post.out 2>&1 &
bash $A/c9post.sh > $A/c9post.out 2>&1
wait
bash $A/c9esc.sh > $A/c9esc.out 2>&1
echo "runpost done $(date -u +%T)"
